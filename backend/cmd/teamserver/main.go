// Command teamserver serves several users from one process: each account gets
// its own workspace kernel (SQLite, resources, model config, assistant child)
// under TEAMSERVER_DATA_DIR/workspaces, behind a session-cookie gateway.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/kernelpool"
	"infinite-canvas/backend/internal/referencestorage"
	"infinite-canvas/backend/internal/teamserver"

	"github.com/gin-gonic/gin"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		if err := runAdmin(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envList(key string) []string {
	var out []string
	for _, item := range strings.Split(os.Getenv(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func dataRoot() (string, error) {
	return filepath.Abs(env("TEAMSERVER_DATA_DIR", "/data"))
}

// enforceOutboundPolicy makes private-network upstreams opt-in per host.
// internal/outbound reads this per dial, so it is process-wide by design.
func enforceOutboundPolicy() error {
	if strings.TrimSpace(os.Getenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS")) == "" {
		if err := os.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "false"); err != nil {
			return err
		}
	}
	return bootstrap.CheckHostedEnvironment(os.Getenv)
}

func loadReferenceStorage() (*referencestorage.Config, error) {
	path := strings.TrimSpace(os.Getenv("TEAMSERVER_REFERENCE_STORAGE_FILE"))
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取参考素材对象存储配置失败：%w", err)
	}
	var config referencestorage.Config
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("参考素材对象存储配置格式错误：%w", err)
	}
	if config.Enabled {
		if err := config.Validate(); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func agentHostArgs() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("TEAMSERVER_AGENT_HOST_ARGS"))
	if raw == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, errors.New(`TEAMSERVER_AGENT_HOST_ARGS 必须是 JSON 字符串数组，例如 ["/app/agent-host/server.mjs"]`)
	}
	return args, nil
}

func run(ctx context.Context) error {
	gin.SetMode(gin.ReleaseMode)
	if err := enforceOutboundPolicy(); err != nil {
		return err
	}
	root, err := dataRoot()
	if err != nil {
		return err
	}
	users, err := accounts.Open(root)
	if err != nil {
		return err
	}
	defer users.Close()
	if hasUsers, err := users.HasUsers(ctx); err != nil {
		return err
	} else if !hasUsers {
		log.Printf("还没有任何账号：先运行 teamserver admin create --username <name> 创建管理员")
	}
	relayTemplate, err := kernelpool.LoadRelayTemplate(os.Getenv("TEAMSERVER_RELAY_CHANNEL_FILE"))
	if err != nil {
		return err
	}
	referenceStorage, err := loadReferenceStorage()
	if err != nil {
		return err
	}
	args, err := agentHostArgs()
	if err != nil {
		return err
	}
	shutdownTimeout := 2 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("CANVAS_SHUTDOWN_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return errors.New("CANVAS_SHUTDOWN_TIMEOUT 必须是正数时长，例如 2m")
		}
		shutdownTimeout = parsed
	}
	pool := kernelpool.New(kernelpool.Options{
		Hosted:           bootstrap.HostedConfig{AssistantHostCommand: os.Getenv("TEAMSERVER_AGENT_HOST_COMMAND"), AssistantHostArgs: args},
		RelayTemplate:    relayTemplate,
		ReferenceStorage: referenceStorage,
		ShutdownTimeout:  shutdownTimeout,
	})
	server, err := teamserver.New(teamserver.Options{
		Accounts: users, Pool: pool, DataRoot: root, WebDir: os.Getenv("TEAMSERVER_WEB_DIR"),
		CookieSecure:   env("TEAMSERVER_COOKIE_SECURE", "true") != "false",
		TrustedProxies: envListOrDefault("TEAMSERVER_TRUSTED_PROXIES", []string{"127.0.0.1", "::1"}),
		AllowedOrigins: envList("TEAMSERVER_ALLOWED_ORIGINS"),
	})
	if err != nil {
		return err
	}

	// Open every active workspace in the background so queued generation tasks
	// resume after a restart without waiting for their owner to log in.
	go openActiveWorkspaces(ctx, users, server, pool)
	go purgeSessions(ctx, users)

	addr := env("TEAMSERVER_ADDR", ":8080")
	httpServer := &http.Server{Addr: addr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("team server listening on %s (data %s)", addr, root)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()
	var failure error
	select {
	case <-ctx.Done():
	case err, ok := <-serveErr:
		if ok {
			failure = err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout+30*time.Second)
	defer cancel()
	// Streams (SSE, assistant chat) end when kernels close; do not wait on them first.
	httpCtx, httpCancel := context.WithTimeout(shutdownCtx, 10*time.Second)
	if err := httpServer.Shutdown(httpCtx); err != nil {
		_ = httpServer.Close()
	}
	httpCancel()
	return errors.Join(failure, pool.CloseAll(shutdownCtx))
}

func envListOrDefault(key string, fallback []string) []string {
	if values := envList(key); len(values) > 0 {
		return values
	}
	return fallback
}

func openActiveWorkspaces(ctx context.Context, users *accounts.Service, server *teamserver.Server, pool *kernelpool.Pool) {
	workspaces, err := users.ActiveWorkspaces(ctx)
	if err != nil {
		log.Printf("列出工作区失败：%v", err)
		return
	}
	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, workspace := range workspaces {
		spec, err := server.WorkspaceSpec(workspace)
		if err != nil {
			log.Printf("工作区 %s 登记无效：%v", workspace.ID, err)
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			if _, err := pool.Get(ctx, spec); err != nil {
				log.Printf("预先打开工作区 %s 失败：%v", spec.ID, err)
			}
		}()
	}
	wg.Wait()
}

func purgeSessions(ctx context.Context, users *accounts.Service) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := users.PurgeExpiredSessions(ctx); err != nil {
				log.Printf("清理过期会话失败：%v", err)
			}
		}
	}
}

// runAdmin: `teamserver admin create|reset-password --username NAME`. The
// password comes from TEAMSERVER_ADMIN_PASSWORD or an interactive prompt so it
// never lands in shell history.
func runAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("用法：teamserver admin create|reset-password --username NAME [--display-name NAME]")
	}
	command := args[0]
	flags := flag.NewFlagSet("admin "+command, flag.ContinueOnError)
	username := flags.String("username", "", "用户名")
	displayName := flags.String("display-name", "", "显示名称")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*username) == "" {
		return errors.New("需要 --username")
	}
	root, err := dataRoot()
	if err != nil {
		return err
	}
	users, err := accounts.Open(root)
	if err != nil {
		return err
	}
	defer users.Close()
	password, err := readPassword()
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch command {
	case "create":
		user, err := users.CreateUser(ctx, "", accounts.CreateUserInput{Username: *username, DisplayName: *displayName, Password: password, Role: accounts.RoleAdmin})
		if err != nil {
			return err
		}
		fmt.Printf("已创建管理员 %s（%s）\n", user.Username, user.ID)
	case "reset-password":
		if err := users.ResetPasswordByUsername(ctx, *username, password); err != nil {
			return err
		}
		fmt.Printf("已重置 %s 的密码，并使其所有会话失效\n", *username)
	default:
		return fmt.Errorf("未知子命令 %q", command)
	}
	return nil
}

// readPassword takes TEAMSERVER_ADMIN_PASSWORD or the first stdin line, e.g.
// `read -rs PW; printf '%s\n' "$PW" | teamserver admin create --username admin`.
func readPassword() (string, error) {
	if value := os.Getenv("TEAMSERVER_ADMIN_PASSWORD"); value != "" {
		return value, nil
	}
	fmt.Fprintln(os.Stderr, "从标准输入读取密码（一行）")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("请通过 TEAMSERVER_ADMIN_PASSWORD 或标准输入提供密码")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
