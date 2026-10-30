package bootstrap

import (
	"errors"
	"net/http"
	"net/textproto"
	"os"
	goruntime "runtime"
	"strings"

	"infinite-canvas/backend/internal/assistantruntime"
	httptransport "infinite-canvas/backend/internal/transport/http"

	"github.com/gin-gonic/gin"
)

// HostedConfig configures a workspace kernel opened by the team server. The
// kernel is reached only through GatewayHandler; its own listener stays on
// loopback for the assistant child and requires the per-kernel launch token.
type HostedConfig struct {
	// AssistantHostCommand pins the assistant child executable. Members cannot
	// change it because PUT /assistant/host/config is not served.
	AssistantHostCommand string
	AssistantHostArgs    []string
	// ManagedChannelIDs are model channels written by the team server; saves
	// from the browser cannot change or remove them.
	ManagedChannelIDs []string
}

// hostedDeniedRoute matches gin route patterns (c.FullPath()).
type hostedDeniedRoute struct {
	method string // empty matches every method
	path   string
	prefix bool
}

// hostedDeniedRoutes are server-local capabilities: they start processes,
// read server files, install executables, or issue loopback credentials.
// Desktop keeps them; a hosted kernel answers as if they were not registered.
var hostedDeniedRoutes = []hostedDeniedRoute{
	{method: http.MethodPut, path: "/api/assistant/host/config"},
	{path: "/api/agent-clients", prefix: true},
	{path: "/api/ops/clients"},
	{path: "/api/plugins/eagle/", prefix: true},
	{method: http.MethodPost, path: "/api/plugins"},
	{method: http.MethodDelete, path: "/api/plugins/:id"},
	{method: http.MethodPost, path: "/api/depth-captures"},
	{path: "/api/diagnostics/", prefix: true},
	// The BeefAPI device flow opens a browser on the machine running the kernel.
	{method: http.MethodPost, path: "/api/beefapi/connection/start"},
	{method: http.MethodPost, path: "/api/beefapi/connection/cancel"},
}

func hostedRouteDenied(method, fullPath string) bool {
	for _, route := range hostedDeniedRoutes {
		if route.method != "" && route.method != method {
			continue
		}
		if route.prefix && strings.HasPrefix(fullPath, route.path) {
			return true
		}
		if !route.prefix && fullPath == route.path {
			return true
		}
	}
	return false
}

func hostedRouteGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if hostedRouteDenied(c.Request.Method, c.FullPath()) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "reason": "hosted_unavailable", "msg": "团队服务器不提供该功能"})
			return
		}
		c.Next()
	}
}

// hostedInboundStrippedHeaders lists credentials only in-process or loopback
// callers may present. The UI session header stays: the kernel issues it to
// this owner's browser and verifies it itself.
func stripHostedInboundHeaders(header http.Header) {
	for key := range header {
		canonical := textproto.CanonicalMIMEHeaderKey(key)
		if canonical == httptransport.LaunchTokenHeader {
			delete(header, key)
			continue
		}
		if strings.HasPrefix(canonical, "X-Beeftv-") && canonical != "X-Beeftv-Ui-Session" {
			delete(header, key)
		}
	}
}

// IsHostedDeniedPath lets the gateway refuse the same capabilities before a
// kernel is even resolved. The path is a request path, so parameterised
// patterns are matched by their static prefix.
func IsHostedDeniedPath(method, path string) bool {
	for _, route := range hostedDeniedRoutes {
		if route.method != "" && route.method != method {
			continue
		}
		pattern := route.path
		if index := strings.Index(pattern, "/:"); index >= 0 {
			if strings.HasPrefix(path, pattern[:index+1]) && path != strings.TrimSuffix(pattern[:index+1], "/") {
				return true
			}
			continue
		}
		if route.prefix && strings.HasPrefix(path, pattern) || path == pattern || path == strings.TrimSuffix(pattern, "/") {
			return true
		}
	}
	return false
}

// GatewayHandler serves browser requests that the team server has bound to this
// kernel's owner. It bypasses the loopback launch token and external-client
// routing on purpose: the gateway is the authentication boundary.
func (r *Runtime) GatewayHandler() http.Handler {
	if r == nil || r.gateway == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "workspace kernel is not hosted", http.StatusServiceUnavailable)
		})
	}
	return r.gateway
}

func hostedGateway(router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		forwarded := request.Clone(request.Context())
		stripHostedInboundHeaders(forwarded.Header)
		router.ServeHTTP(w, httptransport.WithHostedOwner(forwarded))
	})
}

// hostedAssistantCommand never falls back to the workspace's agent_config.json
// command: an empty deployment value means the bundled layout or nothing.
func hostedAssistantCommand(config HostedConfig) *assistantruntime.HostConfig {
	if command := strings.TrimSpace(config.AssistantHostCommand); command != "" {
		return &assistantruntime.HostConfig{HostCommand: command, HostArgs: append([]string(nil), config.AssistantHostArgs...)}
	}
	executable, err := os.Executable()
	if err != nil {
		return &assistantruntime.HostConfig{}
	}
	bundled := assistantruntime.BundledConfig(executable, goruntime.GOOS)
	return &bundled
}

// hostedForbiddenEnvironment are process-wide overrides that would make every
// hosted kernel share one credential, model key or trust bypass.
var hostedForbiddenEnvironment = []string{
	"BEEFTV_AGENT_HOST_TOKEN", "BEEFTV_OWNER_TOKEN", "BEEFTV_UI_BOOTSTRAP", "BEEFTV_ALLOWED_ORIGINS",
	"BEEFTV_AGENT_BASE_URL", "BEEFTV_AGENT_API_KEY", "BEEFTV_AGENT_MODEL", "BEEFTV_AGENT_PROTOCOL",
	"BEEFTV_AGENT_HOST_URL",
}

// CheckHostedEnvironment refuses process settings that break isolation between
// hosted kernels. Private upstream access is process-wide (read per dial in
// internal/outbound), so the team server must run with it disabled.
func CheckHostedEnvironment(getenv func(string) string) error {
	var problems []string
	for _, key := range hostedForbiddenEnvironment {
		if strings.TrimSpace(getenv(key)) != "" {
			problems = append(problems, key+" 会让所有工作区共用同一凭据或信任，团队服务器不能设置")
		}
	}
	switch strings.ToLower(strings.TrimSpace(getenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS"))) {
	case "0", "false", "no":
	default:
		problems = append(problems, "CANVAS_ALLOW_PRIVATE_UPSTREAMS 必须为 false；内网中转站请用 CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS 精确放行")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "；"))
	}
	return nil
}

// hostedAssistantEnvironKeys is what a hosted assistant child may inherit. The
// team server's own secrets (session keys, relay keys, cloud credentials) stay out.
var hostedAssistantEnvironKeys = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LANG": true, "TZ": true, "TMPDIR": true, "TMP": true, "TEMP": true,
	"SYSTEMROOT": true, "NODE_EXTRA_CA_CERTS": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
}

func hostedAssistantEnviron() []string {
	inherited := os.Environ()
	out := make([]string, 0, len(hostedAssistantEnvironKeys))
	for _, entry := range inherited {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		upper := strings.ToUpper(key)
		if hostedAssistantEnvironKeys[upper] || strings.HasPrefix(upper, "LC_") {
			out = append(out, entry)
		}
	}
	return out
}

func hostedManagedChannels(cfg Config) []string {
	if cfg.Profile != ProfileHosted {
		return nil
	}
	return append([]string(nil), cfg.Hosted.ManagedChannelIDs...)
}
