package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/agentops"
)

func TestHostedAssistantCallbackTrustIsSeparateFromGateway(t *testing.T) {
	env := newAssistantTestEnv(t, nil)
	trust := NewHostedRequestTrust("workspace-A", "launch-A", "host-A")
	router := gin.New()
	router.Use(RuntimeDependenciesMiddleware(RuntimeDependencies{Trust: trust}))
	RegisterAgentOpsRoutes(router.Group("/api"), env.service, agentops.NewStore(env.service.Database()), env.clients)
	for _, tc := range []struct {
		name, launch, host string
		handler            http.Handler
		want               int
	}{
		{"host callback", "launch-A", "host-A", trust.AssistantCallbackHandler(router), 200},
		{"missing", "", "", trust.AssistantCallbackHandler(router), 401},
		{"launch only", "launch-A", "", trust.AssistantCallbackHandler(router), 401},
		{"host only", "", "host-A", trust.AssistantCallbackHandler(router), 401},
		{"other kernel", "launch-B", "host-B", trust.AssistantCallbackHandler(router), 401},
		{"gateway rejects host", "launch-A", "host-A", trust.GatewayHandler(router), 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "https://callback.example/api/ops", nil)
			req.RemoteAddr = "203.0.113.42:12345"
			req.Header.Set("Origin", "https://untrusted.example")
			req.Header.Set("X-Desktop-Token", tc.launch)
			req.Header.Set("X-Beeftv-Agent-Token", tc.host)
			w := httptest.NewRecorder()
			tc.handler.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
