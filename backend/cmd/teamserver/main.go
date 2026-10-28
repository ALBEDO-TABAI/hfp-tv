package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"infinite-canvas/backend/internal/accounts"
)

type config struct {
	addr     string
	dataRoot string
	http     accounts.HTTPConfig
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("用法：teamserver serve | admin create --username <name> [--display-name <name>]")
	}
	admin := len(args) >= 2 && args[0] == "admin" && args[1] == "create"
	if args[0] != "serve" && !admin {
		return fmt.Errorf("仅支持 serve 或 admin create")
	}
	remaining := args[1:]
	if admin {
		remaining = args[2:]
	}
	flags := flag.NewFlagSet("teamserver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var cfg config
	flags.StringVar(&cfg.addr, "addr", env("TEAMSERVER_ADDR", "127.0.0.1:43181"), "HTTP 监听地址")
	flags.StringVar(&cfg.dataRoot, "data-dir", os.Getenv("TEAMSERVER_DATA_DIR"), "平台数据根目录（必填）")
	origins := flags.String("allowed-origins", os.Getenv("TEAMSERVER_ALLOWED_ORIGINS"), "允许的 Origin，以逗号分隔")
	insecure, err := strconv.ParseBool(env("TEAMSERVER_INSECURE_COOKIE", "false"))
	if err != nil {
		return fmt.Errorf("TEAMSERVER_INSECURE_COOKIE 必须是 true 或 false")
	}
	flags.BoolVar(&cfg.http.InsecureCookie, "insecure-cookie", insecure, "允许 HTTP 调试 Cookie")
	var username, displayName string
	if admin {
		flags.StringVar(&username, "username", "", "管理员用户名")
		flags.StringVar(&displayName, "display-name", "", "管理员显示名")
	}
	if err := flags.Parse(remaining); err != nil {
		return fmt.Errorf("命令参数不正确；密码只能从 stdin 读取")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("不接受位置参数；密码只能从 stdin 读取")
	}
	for _, origin := range strings.Split(*origins, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.http.AllowedOrigins = append(cfg.http.AllowedOrigins, origin)
		}
	}
	if admin && strings.TrimSpace(username) == "" {
		return fmt.Errorf("admin create 必须指定 --username")
	}
	if strings.TrimSpace(cfg.dataRoot) == "" {
		return fmt.Errorf("必须通过 --data-dir 或 TEAMSERVER_DATA_DIR 指定平台数据目录")
	}
	service, err := accounts.Open(cfg.dataRoot)
	if err != nil {
		return fmt.Errorf("打开平台库失败: %w", err)
	}
	defer service.Close()
	if admin {
		password, err := readPassword(stdin)
		if err != nil {
			return err
		}
		identity, err := service.CreateAdmin(ctx, accounts.NewUser{Username: username, DisplayName: displayName, Password: password})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "已创建管理员 %s（工作区 %s）\n", identity.User.Username, identity.WorkspaceID)
		return err
	}
	handler, err := accounts.NewHandler(service, cfg.http)
	if err != nil {
		return err
	}
	return serve(ctx, cfg.addr, handler)
}

func readPassword(stdin io.Reader) (string, error) {
	// 有界读取一行；不要求管道关闭，也不把输入写到输出或错误中。
	scanner := bufio.NewScanner(io.LimitReader(stdin, 1027))
	if !scanner.Scan() {
		return "", fmt.Errorf("必须从 stdin 输入一行密码")
	}
	password := scanner.Text()
	if len(password) < 12 || len(password) > 1024 {
		return "", accounts.ErrPassword
	}
	return password, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func serve(ctx context.Context, addr string, handler http.Handler) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	log.Printf("teamserver listening on %s", listener.Addr())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return errors.Join(err, server.Close())
		}
		return nil
	}
}
