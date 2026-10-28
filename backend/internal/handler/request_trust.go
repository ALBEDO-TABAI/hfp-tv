package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"infinite-canvas/backend/internal/agentops"
	"infinite-canvas/backend/internal/app"
	"infinite-canvas/backend/internal/kernel"
	httptransport "infinite-canvas/backend/internal/transport/http"
	"infinite-canvas/backend/internal/workspace"
)

type RequestTrust struct {
	dataDir, workspaceID, launchToken, hostToken, uiBootstrapToken string
	hosted                                                         bool
}

type RequestIdentity struct {
	Hosted, SourceAllowed, EntryAllowed, Owner, UI, DevBootstrap, AssistantHost, RequireUI bool
}

func NewLocalRequestTrust(dataDir, launchToken, uiBootstrapToken string) *RequestTrust {
	return &RequestTrust{dataDir: dataDir, launchToken: launchToken, uiBootstrapToken: uiBootstrapToken}
}

func NewHostedRequestTrust(workspaceID, launchToken, hostToken string) *RequestTrust {
	return &RequestTrust{hosted: true, workspaceID: workspaceID, launchToken: launchToken, hostToken: hostToken}
}

type assistantCallbackKey struct{}

func (t *RequestTrust) Resolve(r *http.Request) RequestIdentity {
	if t.hosted {
		owner := workspace.IsAuthenticatedOwner(r.Context(), t.workspaceID)
		callback, _ := r.Context().Value(assistantCallbackKey{}).(*RequestTrust)
		return RequestIdentity{Hosted: true, SourceAllowed: owner || callback == t,
			EntryAllowed: owner, Owner: owner, AssistantHost: callback == t, RequireUI: true}
	}
	identity := RequestIdentity{SourceAllowed: isLoopbackRequest(r), RequireUI: t.launchToken != "",
		EntryAllowed: t.launchToken == "" || tokenMatches(t.launchToken, r.Header.Get(httptransport.LaunchTokenHeader)),
		DevBootstrap: strings.TrimSpace(os.Getenv("BEEFTV_UI_BOOTSTRAP")) == "1"}
	if t.dataDir != "" {
		if owner := strings.TrimSpace(r.Header.Get("X-Beeftv-Owner")); owner != "" {
			identity.Owner = agentops.OwnerTokenMatches(t.dataDir, owner)
		}
		identity.AssistantHost = agentops.HostTokenMatches(t.dataDir, r.Header.Get(assistantHostTokenHeader))
	}
	identity.UI = identity.SourceAllowed && identity.RequireUI && identity.EntryAllowed && tokenMatches(t.uiBootstrapToken, r.Header.Get("X-Beeftv-UI-Bootstrap"))
	return identity
}

func requestIdentity(c *gin.Context, svc *app.Service) RequestIdentity {
	if dependencies, ok := runtimeDependencies(c); ok && dependencies.Trust != nil {
		return dependencies.Trust.Resolve(c.Request)
	}
	dataDir := ""
	if svc != nil {
		dataDir = svc.DataDir()
	}
	return NewLocalRequestTrust(dataDir, "", "").Resolve(c.Request)
}

func (t *RequestTrust) GatewayHandler(next http.Handler) http.Handler {
	if !t.hosted {
		if t.launchToken != "" {
			return httptransport.RequireLaunchToken(t.launchToken)(next)
		}
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !t.Resolve(r).Owner {
			unauthorizedWorkspace(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AssistantCallbackHandler is mounted only on the kernel's private listener.
// Even valid host credentials cannot create an owner identity at the gateway.
func (t *RequestTrust) AssistantCallbackHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tokenMatches(t.launchToken, r.Header.Get(httptransport.LaunchTokenHeader)) ||
			!tokenMatches(t.hostToken, r.Header.Get(assistantHostTokenHeader)) {
			unauthorizedWorkspace(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), assistantCallbackKey{}, t)))
	})
}

func tokenMatches(expected, presented string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

func unauthorizedWorkspace(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": kernel.CodeUnauthorized, "data": nil,
		"msg": "工作区身份未认证", "reason": kernel.ReasonUnauthorized})
}
