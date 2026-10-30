package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func openHostedKernel(t *testing.T, dataDir string) *Runtime {
	t.Helper()
	runtime, err := Open(context.Background(), Config{
		Profile: ProfileHosted, DataDir: dataDir, AutoMigrate: true, ShutdownTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	if err := runtime.Start(); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func hostedCall(t *testing.T, runtime *Runtime, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "https://team.example.com/api"+path, strings.NewReader(body))
	request.RemoteAddr = "203.0.113.7:51000"
	request.Header.Set("Origin", "https://team.example.com")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	runtime.GatewayHandler().ServeHTTP(recorder, request)
	return recorder
}

func hostedWorkspaceID(t *testing.T, runtime *Runtime) string {
	t.Helper()
	response := hostedCall(t, runtime, http.MethodGet, "/workspace/bootstrap", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap through gateway = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Data.Workspace.ID == "" {
		t.Fatalf("bootstrap payload: %v %s", err, response.Body.String())
	}
	return envelope.Data.Workspace.ID
}

func TestHostedKernelsStayIsolatedInOneProcess(t *testing.T) {
	first := openHostedKernel(t, t.TempDir())
	second := openHostedKernel(t, t.TempDir())
	if hostedWorkspaceID(t, first) == hostedWorkspaceID(t, second) {
		t.Fatal("two hosted kernels share one workspace identity")
	}

	saved := hostedCall(t, first, http.MethodPut, "/canvas-projects/hosted-a", `{"project":{"id":"hosted-a","title":"A","revision":0,"nodes":[],"connections":[]}}`, nil)
	if saved.Code != http.StatusOK {
		t.Fatalf("gateway owner could not save a canvas: %d %s", saved.Code, saved.Body.String())
	}
	if response := hostedCall(t, first, http.MethodGet, "/canvas-projects/hosted-a", "", nil); response.Code != http.StatusOK {
		t.Fatalf("owner cannot read own canvas: %d", response.Code)
	}
	if response := hostedCall(t, second, http.MethodGet, "/canvas-projects/hosted-a", "", nil); response.Code == http.StatusOK {
		t.Fatal("second workspace read the first workspace's canvas")
	}

	// The gateway principal replaces the loopback same-origin rule for the ops layer.
	if response := hostedCall(t, first, http.MethodGet, "/ops", "", nil); response.Code != http.StatusOK {
		t.Fatalf("ops discovery rejected the hosted owner: %d %s", response.Code, response.Body.String())
	}
	if response := hostedCall(t, first, http.MethodPost, "/assistant/ui-session", `{}`, nil); response.Code != http.StatusOK {
		t.Fatalf("hosted owner cannot obtain an assistant UI session: %d %s", response.Code, response.Body.String())
	}
}

func TestHostedKernelRefusesServerLocalCapabilities(t *testing.T) {
	runtime := openHostedKernel(t, t.TempDir())
	cases := []struct{ method, path string }{
		{http.MethodPut, "/assistant/host/config"},
		{http.MethodGet, "/agent-clients"},
		{http.MethodPost, "/agent-clients"},
		{http.MethodGet, "/ops/clients"},
		{http.MethodPost, "/ops/clients"},
		{http.MethodGet, "/plugins/eagle/library"},
		{http.MethodPost, "/plugins"},
		{http.MethodDelete, "/plugins/some-plugin"},
		{http.MethodPost, "/depth-captures"},
		{http.MethodPost, "/diagnostics/export"},
		{http.MethodPost, "/beefapi/connection/start"},
	}
	for _, item := range cases {
		response := hostedCall(t, runtime, item.method, item.path, `{}`, nil)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "hosted_unavailable") {
			t.Fatalf("%s %s = %d %s, want hosted_unavailable", item.method, item.path, response.Code, response.Body.String())
		}
		if !IsHostedDeniedPath(item.method, "/api"+item.path) {
			t.Fatalf("gateway deny list misses %s %s", item.method, item.path)
		}
	}
	for _, allowed := range []struct{ method, path string }{
		{http.MethodGet, "/api/plugins"}, {http.MethodPost, "/api/plugins/x/enable"}, {http.MethodGet, "/api/assistant/host/config"},
		{http.MethodGet, "/api/canvas-projects"}, {http.MethodPost, "/api/ops/canvas.document.commit"},
	} {
		if IsHostedDeniedPath(allowed.method, allowed.path) {
			t.Fatalf("gateway deny list blocks ordinary route %s %s", allowed.method, allowed.path)
		}
	}
}

func TestHostedKernelLoopbackListenerRequiresItsLaunchToken(t *testing.T) {
	runtime := openHostedKernel(t, t.TempDir())
	other := openHostedKernel(t, t.TempDir())
	if !strings.HasPrefix(runtime.BaseURL(), "http://127.0.0.1:") {
		t.Fatalf("hosted kernel must listen on loopback, got %q", runtime.BaseURL())
	}
	get := func(token string) int {
		request, err := http.NewRequest(http.MethodGet, runtime.BaseURL()+"/workspace/bootstrap", nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			request.Header.Set("X-Desktop-Token", token)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if code := get(""); code != http.StatusForbidden {
		t.Fatalf("loopback listener without token = %d", code)
	}
	if code := get(other.LaunchToken()); code != http.StatusForbidden {
		t.Fatalf("another kernel's token opened this kernel: %d", code)
	}
	if code := get(runtime.LaunchToken()); code != http.StatusOK {
		t.Fatalf("own launch token rejected: %d", code)
	}
}

func TestHostedKernelLocksItsDataDir(t *testing.T) {
	dir := t.TempDir()
	openHostedKernel(t, dir)
	if _, err := Open(context.Background(), Config{Profile: ProfileHosted, DataDir: dir, AutoMigrate: true}); err == nil {
		t.Fatal("a second hosted kernel opened the same data directory")
	}
}

func TestHostedGatewayStripsLoopbackCredentials(t *testing.T) {
	header := http.Header{}
	for _, key := range []string{"X-Beeftv-Owner", "X-Desktop-Token", "X-Beeftv-UI-Bootstrap", "X-Beeftv-Client", "X-Beeftv-Agent-Token", "X-Beeftv-Agent-Turn"} {
		header.Set(key, "forged")
	}
	header.Set("X-Beeftv-Ui-Session", "kept")
	header.Set("Authorization", "Bearer upstream-key")
	stripHostedInboundHeaders(header)
	for key := range header {
		if key != "X-Beeftv-Ui-Session" && key != "Authorization" {
			t.Fatalf("gateway forwarded loopback credential %s", key)
		}
	}
	if header.Get("X-Beeftv-Ui-Session") != "kept" || header.Get("Authorization") == "" {
		t.Fatal("gateway dropped a header the kernel needs")
	}
}
