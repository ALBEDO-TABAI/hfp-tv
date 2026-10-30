package teamserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/kernelpool"
)

type harness struct {
	t      *testing.T
	server *Server
	users  *accounts.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "false")
	root := t.TempDir()
	users, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	pool := kernelpool.New(kernelpool.Options{
		RelayTemplate:   map[string]any{"name": "公司中转站", "baseUrl": "https://relay.example.com/v1", "apiFormat": "openai", "models": []any{"relay-model"}},
		ShutdownTimeout: 5 * time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = pool.CloseAll(ctx)
		_ = users.Close()
	})
	server, err := New(Options{Accounts: users, Pool: pool, DataRoot: root, CookieSecure: true})
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
	// A member save that drops or rewrites the managed channel is restored.
	tamper := h.do(call{method: http.MethodPut, path: "/api/workspace/model-config", cookie: aliceCookie,
		body: `{"config":{"channels":[{"id":"company-relay","name":"x","baseUrl":"http://127.0.0.1:9","apiKey":"stolen","apiFormat":"openai","models":[]},{"id":"mine","name":"mine","baseUrl":"https://api.example.com","apiKey":"sk-mine","apiFormat":"openai","models":["m"]}]}}`})
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
