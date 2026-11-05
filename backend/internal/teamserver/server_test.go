package teamserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/errorlog"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/kernelpool"
	"infinite-canvas/backend/internal/relaychannel"
	workspacecfg "infinite-canvas/backend/internal/workspace"
)

type harness struct {
	t      *testing.T
	server *Server
	users  *accounts.Service
}

func newHarness(t *testing.T, configure ...func(*Options)) *harness {
	t.Helper()
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "false")
	root := t.TempDir()
	users, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	relayTemplate := map[string]any{"name": "公司中转站", "baseUrl": "https://relay.example.com/v1", "apiFormat": "openai", "models": []any{"relay-model", "relay-text"},
		"modelProfiles": []any{map[string]any{"model": "relay-model", "capability": "image"}, map[string]any{"model": "relay-text", "capability": "text", "protocol": "chat-completion"}}}
	pool := kernelpool.New(kernelpool.Options{
		RelayTemplate:   relayTemplate,
		ShutdownTimeout: 5 * time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = pool.CloseAll(ctx)
		_ = users.Close()
	})
	relayChannels, err := relaychannel.Open(users.DB())
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Accounts: users, Pool: pool, DataRoot: root, CookieSecure: CookieSecureAlways, RelayChannels: relayChannels, RelayTemplates: pool,
		Status: DeploymentStatus{RelayFile: "/config/relay-channel.json", RelayFileTemplate: relayTemplate, IdleTimeout: 30 * time.Minute}}
	for _, change := range configure {
		change(&opts)
	}
	server, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, server: server, users: users}
}

type call struct {
	method, path, body, cookie, origin string
	headers                            map[string]string
}

func (h *harness) do(request call) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(request.method, "https://team.example.com"+request.path, strings.NewReader(request.body))
	req.RemoteAddr = "203.0.113.5:40000"
	if request.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	origin := request.origin
	if origin == "" {
		origin = "https://team.example.com"
	}
	if origin != "-" {
		req.Header.Set("Origin", origin)
	}
	if request.cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: request.cookie})
	}
	for key, value := range request.headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, req)
	return recorder
}

func (h *harness) login(username, password string) string {
	h.t.Helper()
	response := h.do(call{method: http.MethodPost, path: "/api/platform/login", body: `{"username":"` + username + `","password":"` + password + `"}`})
	if response.Code != http.StatusOK {
		h.t.Fatalf("login %s = %d %s", username, response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == SessionCookie {
			if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
				h.t.Fatalf("session cookie flags wrong: %+v", cookie)
			}
			return cookie.Value
		}
	}
	h.t.Fatal("login did not set the session cookie")
	return ""
}

func reason(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &envelope)
	return envelope.Reason
}

func TestTeamServerIsolatesWorkspacesBehindSessions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	admin, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	alice, _ := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass"})
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"}); err != nil {
		t.Fatal(err)
	}

	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects"}); response.Code != http.StatusUnauthorized || reason(t, response) != "unauthenticated" {
		t.Fatalf("anonymous workspace request = %d %s", response.Code, response.Body.String())
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/platform/login", body: `{"username":"alice","password":"wrong-pass"}`}); response.Code != http.StatusUnauthorized || reason(t, response) != "invalid_credentials" {
		t.Fatalf("bad login = %d %s", response.Code, response.Body.String())
	}
	aliceCookie := h.login("alice", "alice-pass")
	bobCookie := h.login("bob", "bob-pass1")
	adminCookie := h.login("admin", "admin-pass")

	me := h.do(call{method: http.MethodGet, path: "/api/platform/me", cookie: aliceCookie})
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"username":"alice"`) || !strings.Contains(me.Body.String(), `"hosted":true`) {
		t.Fatalf("me = %d %s", me.Code, me.Body.String())
	}

	save := h.do(call{method: http.MethodPut, path: "/api/canvas-projects/alice-canvas", cookie: aliceCookie,
		body: `{"project":{"id":"alice-canvas","title":"A","revision":0,"nodes":[],"connections":[]}}`})
	if save.Code != http.StatusOK {
		t.Fatalf("alice save = %d %s", save.Code, save.Body.String())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/alice-canvas", cookie: aliceCookie}); response.Code != http.StatusOK {
		t.Fatalf("alice read own canvas = %d", response.Code)
	}
	// Forged loopback credentials and workspace hints cannot redirect the request.
	forged := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/alice-canvas", cookie: bobCookie,
		headers: map[string]string{"X-Beeftv-Owner": "forged", "X-Desktop-Token": "forged", "X-Workspace-Id": alice.ID}})
	if forged.Code == http.StatusOK {
		t.Fatal("bob read alice's canvas")
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/ops/canvas.get", cookie: aliceCookie, body: `{"params":{"canvasId":"alice-canvas"}}`}); response.Code != http.StatusOK {
		t.Fatalf("ops through gateway = %d %s", response.Code, response.Body.String())
	}

	// CSRF: unsafe requests need this site's Origin.
	if response := h.do(call{method: http.MethodPost, path: "/api/ops/canvas.get", cookie: aliceCookie, origin: "-", body: `{}`}); response.Code != http.StatusForbidden {
		t.Fatalf("request without Origin = %d", response.Code)
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/ops/canvas.get", cookie: aliceCookie, origin: "https://evil.example", body: `{}`}); response.Code != http.StatusForbidden {
		t.Fatalf("cross-site request = %d", response.Code)
	}

	if response := h.do(call{method: http.MethodPut, path: "/api/assistant/host/config", cookie: aliceCookie, body: `{"hostCommand":"/bin/sh"}`}); response.Code != http.StatusNotFound || reason(t, response) != "hosted_unavailable" {
		t.Fatalf("host command write = %d %s", response.Code, response.Body.String())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/platform/admin/users", cookie: aliceCookie}); response.Code != http.StatusForbidden {
		t.Fatalf("member reached admin API: %d", response.Code)
	}
	users := h.do(call{method: http.MethodGet, path: "/api/platform/admin/users", cookie: adminCookie})
	if users.Code != http.StatusOK || strings.Contains(users.Body.String(), "argon2") {
		t.Fatalf("admin users = %d %s", users.Code, users.Body.String())
	}

	bind := h.do(call{method: http.MethodPut, path: "/api/platform/admin/users/" + alice.ID + "/relay-key", cookie: adminCookie, body: `{"key":"sk-alice-relay-9876"}`})
	if bind.Code != http.StatusOK || strings.Contains(bind.Body.String(), "sk-alice-relay") || !strings.Contains(bind.Body.String(), "…9876") {
		t.Fatalf("bind relay = %d %s", bind.Code, bind.Body.String())
	}
	config := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie})
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), `"id":"company-relay"`) || !strings.Contains(config.Body.String(), "https://relay.example.com/v1") {
		t.Fatalf("relay channel not provisioned: %d %s", config.Code, config.Body.String())
	}
	if strings.Contains(config.Body.String(), "sk-alice-relay") || !strings.Contains(config.Body.String(), `"hasApiKey":true`) {
		t.Fatalf("relay key must stay on the server: %s", config.Body.String())
	}
	if key := storedRelayKey(t, h, alice.ID); key != "sk-alice-relay-9876" {
		t.Fatalf("stored relay key = %q", key)
	}
	// A member save that drops or rewrites the managed channel is restored.
	var current struct {
		Data struct {
			Revision int64 `json:"revision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(config.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	tamper := h.do(call{method: http.MethodPut, path: "/api/workspace/model-config", cookie: aliceCookie,
		body: `{"expectedRevision":` + strconv.FormatInt(current.Data.Revision, 10) + `,"config":{"channels":[{"id":"company-relay","name":"x","baseUrl":"http://127.0.0.1:9","apiKey":"stolen","apiFormat":"openai","models":[]},{"id":"mine","name":"mine","baseUrl":"https://api.example.com","apiKey":"sk-mine","apiFormat":"openai","models":["m"]}]}}`})
	if tamper.Code != http.StatusOK {
		t.Fatalf("member save = %d %s", tamper.Code, tamper.Body.String())
	}
	after := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie}).Body.String()
	if strings.Contains(after, "127.0.0.1:9") || strings.Contains(after, `"stolen"`) || !strings.Contains(after, "https://relay.example.com/v1") || !strings.Contains(after, `"id":"mine"`) {
		t.Fatalf("managed relay channel was modified by a member: %s", after)
	}
	bobConfig := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: bobCookie}).Body.String()
	if strings.Contains(bobConfig, "sk-alice-relay") {
		t.Fatal("alice's relay key reached bob's workspace")
	}

	disable := h.do(call{method: http.MethodPatch, path: "/api/platform/admin/users/" + alice.ID, cookie: adminCookie, body: `{"status":"disabled"}`})
	if disable.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", disable.Code, disable.Body.String())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects", cookie: aliceCookie}); response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled member still served: %d", response.Code)
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/platform/logout", cookie: bobCookie, body: `{}`}); response.Code != http.StatusOK {
		t.Fatalf("logout = %d", response.Code)
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/platform/me", cookie: bobCookie}); response.Code != http.StatusUnauthorized {
		t.Fatalf("session survived logout: %d", response.Code)
	}
}

func TestTeamServerServesSPAFallback(t *testing.T) {
	h := newHarness(t)
	web := t.TempDir()
	if err := writeFile(web+"/index.html", "<html>app</html>"); err != nil {
		t.Fatal(err)
	}
	h.server.opts.WebDir = web
	for _, path := range []string{"/", "/canvas/abc", "/login"} {
		response := h.do(call{method: http.MethodGet, path: path})
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "app") {
			t.Fatalf("%s = %d", path, response.Code)
		}
	}
	if response := h.do(call{method: http.MethodGet, path: "/static/missing.js"}); response.Code != http.StatusNotFound {
		t.Fatalf("missing asset = %d", response.Code)
	}
}

func writeFile(path, body string) error { return os.WriteFile(path, []byte(body), 0o600) }

// storedRelayKey reads the company relay key from the user's workspace config
// on disk, where kernels resolve it.
func storedRelayKey(t *testing.T, h *harness, userID string) string {
	t.Helper()
	workspace, err := h.users.WorkspaceForUser(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := h.server.WorkspaceSpec(*workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspacecfg.NewProviderConfig(spec.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	effective, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	channels, _ := effective.Config["channels"].([]any)
	for _, raw := range channels {
		if channel, ok := raw.(map[string]any); ok && channel["id"] == kernelpool.RelayChannelID {
			key, _ := channel["apiKey"].(string)
			return key
		}
	}
	return ""
}

func TestCookieSecureFollowsTheBrowserSchemeInAutoMode(t *testing.T) {
	h := newHarness(t, func(opts *Options) {
		opts.CookieSecure = CookieSecureAuto
		opts.TrustedProxies = []string{"10.0.0.0/8"}
	})
	if _, err := h.users.CreateUser(context.Background(), "", accounts.CreateUserInput{Username: "alice", Password: "alice-pass"}); err != nil {
		t.Fatal(err)
	}
	login := func(target, remote, proto string) *http.Cookie {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, target+"/api/platform/login", strings.NewReader(`{"username":"alice","password":"alice-pass"}`))
		req.RemoteAddr = remote
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", target)
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		recorder := httptest.NewRecorder()
		h.server.Handler().ServeHTTP(recorder, req)
		for _, cookie := range recorder.Result().Cookies() {
			if cookie.Name == SessionCookie {
				return cookie
			}
		}
		t.Fatalf("login via %s from %s: %d %s", target, remote, recorder.Code, recorder.Body.String())
		return nil
	}
	for _, check := range []struct {
		target, remote, proto string
		secure                bool
	}{
		{"https://team.example.com", "203.0.113.5:40000", "", true}, // direct TLS
		{"http://10.10.10.20", "10.0.0.2:40000", "https", true},     // Caddy terminated HTTPS
		{"http://10.10.10.20", "10.0.0.2:40000", "http", false},     // intranet HTTP through Caddy
		{"http://10.10.10.20", "203.0.113.5:40000", "https", false}, // untrusted peer cannot claim HTTPS
	} {
		if cookie := login(check.target, check.remote, check.proto); cookie.Secure != check.secure {
			t.Errorf("%s from %s proto=%q: Secure=%v", check.target, check.remote, check.proto, cookie.Secure)
		}
	}
}

func TestServiceStatusShowsDeploymentToAdminsOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	admin, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass", RelayKey: "sk-alice-status-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"}); err != nil {
		t.Fatal(err)
	}
	member := h.do(call{method: http.MethodGet, path: "/api/platform/admin/service-status", cookie: h.login("alice", "alice-pass")})
	if member.Code != http.StatusForbidden {
		t.Fatalf("member service status = %d", member.Code)
	}
	response := h.do(call{method: http.MethodGet, path: "/api/platform/admin/service-status", cookie: h.login("admin", "admin-pass")})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "sk-alice") {
		t.Fatalf("service status = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Relay struct {
				Configured bool   `json:"configured"`
				BaseURL    string `json:"baseUrl"`
				Models     []struct {
					Model      string `json:"model"`
					Capability string `json:"capability"`
				} `json:"models"`
				AutoAllowedHost string `json:"autoAllowedHost"`
			} `json:"relay"`
			CookieSecure       string `json:"cookieSecure"`
			IdleTimeoutSeconds int64  `json:"idleTimeoutSeconds"`
			Members            struct {
				Active       int `json:"active"`
				WithRelayKey int `json:"withRelayKey"`
			} `json:"members"`
			Workspaces struct {
				Total int `json:"total"`
			} `json:"workspaces"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	status := envelope.Data
	if !status.Relay.Configured || status.Relay.BaseURL != "https://relay.example.com/v1" || len(status.Relay.Models) != 2 || status.Relay.Models[0].Capability != "image" || status.Relay.AutoAllowedHost != "relay.example.com" {
		t.Fatalf("relay status = %+v", status.Relay)
	}
	if status.CookieSecure != "always" || status.IdleTimeoutSeconds != 1800 || status.Members.Active != 3 || status.Members.WithRelayKey != 1 || status.Workspaces.Total != 3 {
		t.Fatalf("status = %+v", status)
	}
}

// Members create generation tasks through the gateway; the kernel's desktop
// trust (which never matches without a desktop shell) must not block them.
func TestMembersCanCreateTasksThroughTheGateway(t *testing.T) {
	h := newHarness(t)
	if _, err := h.users.CreateUser(context.Background(), "", accounts.CreateUserInput{Username: "alice", Password: "alice-pass"}); err != nil {
		t.Fatal(err)
	}
	cookie := h.login("alice", "alice-pass")
	for _, path := range []string{"/api/tasks", "/api/canvas-workflows"} {
		response := h.do(call{method: http.MethodPost, path: path, cookie: cookie, body: `{"type":"canvas_text","prompt":"写一句话","input":{"config":{}}}`})
		if response.Code == http.StatusForbidden {
			t.Fatalf("%s rejected the workspace owner: %s", path, response.Body.String())
		}
	}
}

func TestProvisionedWorkspaceIsReadyAndNeverEchoesTheRelayKey(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	alice, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "alice", Password: "alice-pass", RelayKey: "sk-alice-relay-5555"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"})
	if err != nil {
		t.Fatal(err)
	}
	aliceCookie := h.login("alice", "alice-pass")
	body := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie}).Body.String()
	if !strings.Contains(body, `"textModel":"company-relay::relay-text"`) {
		t.Fatalf("a new member got no text model for the assistant: %s", body)
	}

	// An older build copied the selected channel's key into top-level fields.
	workspace, _ := h.users.WorkspaceForUser(ctx, alice.ID)
	spec, _ := h.server.WorkspaceSpec(*workspace)
	store, err := workspacecfg.NewProviderConfig(spec.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	effective, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	effective.Config["apiKey"] = "sk-alice-relay-5555"
	effective.Config["legacy"] = map[string]any{"nested": []any{"sk-alice-relay-5555"}}
	// Alice also added the relay as her own model service with the same key.
	channels, _ := effective.Config["channels"].([]any)
	effective.Config["channels"] = append(channels, map[string]any{"id": "mine", "name": "mine", "baseUrl": "http://relay.internal:3000/v1", "apiKey": "sk-alice-relay-5555", "apiFormat": "openai", "models": []any{"m"}})
	raw, _ := json.Marshal(effective.Config)
	if err := store.SaveLocalModelConfig(raw); err != nil {
		t.Fatal(err)
	}
	body = h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie}).Body.String()
	var returned struct {
		Data struct {
			Config struct {
				APIKey   string           `json:"apiKey"`
				Legacy   map[string]any   `json:"legacy"`
				Channels []map[string]any `json:"channels"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &returned); err != nil {
		t.Fatal(err)
	}
	keys := map[string]any{}
	for _, channel := range returned.Data.Config.Channels {
		keys[fmt.Sprint(channel["id"])] = channel["apiKey"]
	}
	if returned.Data.Config.APIKey != "" || strings.Contains(fmt.Sprint(returned.Data.Config.Legacy), "sk-alice-relay") || keys["company-relay"] != "" {
		t.Fatalf("stale copy of the relay key reached the browser: %s", body)
	}
	if keys["mine"] != "sk-alice-relay-5555" {
		t.Fatalf("the member's own channel lost its key: %s", body)
	}

	// Another member's canvas event stream is a plain 404, not a server error.
	_ = bob
	project := `{"project":{"id":"alice-canvas","revision":0,"title":"A","nodes":[],"connections":[]}}`
	if response := h.do(call{method: http.MethodPut, path: "/api/canvas-projects/alice-canvas", cookie: aliceCookie, body: project}); response.Code != http.StatusOK {
		t.Fatalf("save canvas = %d %s", response.Code, response.Body.String())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/alice-canvas/events", cookie: h.login("bob", "bob-pass1")}); response.Code != http.StatusNotFound {
		t.Fatalf("foreign canvas events = %d %s", response.Code, response.Body.String())
	}
}

func TestErrorLogIsShownToAdminsWithTheMember(t *testing.T) {
	var store *errorlog.Store
	h := newHarness(t, func(opts *Options) {
		var err error
		if store, err = errorlog.Open(opts.Accounts.DB()); err != nil {
			t.Fatal(err)
		}
		opts.ErrorLog = store
	})
	ctx := context.Background()
	admin, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	alice, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := h.users.WorkspaceForUser(ctx, alice.ID)
	if err := store.RecordTask(workspace.ID, kernel.FailureReport{TaskID: "task-1", TaskType: "canvas_video", Model: "doubao-seedance-2.0-fast",
		Message: "声明式协议任务失败：上游返回失败状态", ProviderTaskID: "task_Cdcy", Log: []string{"18:05:04 ERROR 任务处理失败"}}); err != nil {
		t.Fatal(err)
	}
	if member := h.do(call{method: http.MethodGet, path: "/api/platform/admin/errors", cookie: h.login("alice", "alice-pass")}); member.Code != http.StatusForbidden {
		t.Fatalf("member error log = %d", member.Code)
	}
	adminCookie := h.login("admin", "admin-pass")
	var envelope struct {
		Data struct {
			Events []struct {
				TaskID         string   `json:"taskId"`
				ProviderTaskID string   `json:"providerTaskId"`
				Log            []string `json:"log"`
				Member         *struct {
					Username string `json:"username"`
				} `json:"member"`
			} `json:"events"`
		} `json:"data"`
	}
	response := h.do(call{method: http.MethodGet, path: "/api/platform/admin/errors", cookie: adminCookie})
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK {
		t.Fatalf("error log = %d %s", response.Code, response.Body.String())
	}
	if events := envelope.Data.Events; len(events) != 1 || events[0].Member == nil || events[0].Member.Username != "alice" || events[0].ProviderTaskID != "task_Cdcy" || len(events[0].Log) != 1 {
		t.Fatalf("events = %s", response.Body.String())
	}
	filtered := h.do(call{method: http.MethodGet, path: "/api/platform/admin/errors?member=" + bob.ID, cookie: adminCookie})
	if !strings.Contains(filtered.Body.String(), `"events":[]`) {
		t.Fatalf("bob's error log = %s", filtered.Body.String())
	}
}
