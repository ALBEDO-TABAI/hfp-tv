// Package teamserver is the HTTP composition of the team server: platform
// routes (login, accounts, admin), the gateway that binds every other /api
// request to the caller's own workspace kernel, and optional SPA hosting.
package teamserver

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/kernelpool"
	"infinite-canvas/backend/internal/skillmarket"
	"infinite-canvas/backend/internal/workshop"

	"github.com/gin-gonic/gin"
)

const SessionCookie = "hfp_session"

// WorkspacePool is the kernel lifecycle the gateway needs.
type WorkspacePool interface {
	Get(ctx context.Context, spec kernelpool.Spec) (kernelpool.Kernel, error)
	Reprovision(spec kernelpool.Spec) error
	Close(ctx context.Context, id string) error
}

type Options struct {
	Accounts *accounts.Service
	Pool     WorkspacePool
	// DataRoot resolves the relative workspace directories stored in platform.db.
	DataRoot string
	// WebDir serves the built SPA when set; otherwise a reverse proxy does.
	WebDir string
	// CookieSecure must stay true behind HTTPS; false only for local HTTP trials.
	CookieSecure bool
	// TrustedProxies are the reverse proxies whose X-Forwarded-For is honoured.
	TrustedProxies []string
	// AllowedOrigins are extra browser origins (besides the request host) allowed
	// to send unsafe requests, e.g. a separate dev server.
	AllowedOrigins []string
	// Market is the in-house skill market; nil disables its platform routes.
	Market *skillmarket.Service
	// Workshop is the project workshop; nil disables its routes.
	Workshop *workshop.Service
}

type Server struct {
	opts    Options
	engine  *gin.Engine
	origins map[string]bool
}

func New(opts Options) (*Server, error) {
	if opts.Accounts == nil || opts.Pool == nil {
		return nil, errors.New("团队服务器缺少账号服务或内核池")
	}
	root, err := filepath.Abs(opts.DataRoot)
	if err != nil {
		return nil, err
	}
	opts.DataRoot = root
	engine := gin.New()
	engine.Use(gin.Recovery())
	if err := engine.SetTrustedProxies(opts.TrustedProxies); err != nil {
		return nil, err
	}
	server := &Server{opts: opts, engine: engine, origins: make(map[string]bool)}
	for _, origin := range opts.AllowedOrigins {
		if normalized := normalizeOrigin(origin); normalized != "" {
			server.origins[normalized] = true
		}
	}
	engine.Use(server.requireSameOrigin())
	if opts.Workshop != nil {
		opts.Workshop.UseKernels(server.snapshotKernel)
	}
	server.routes()
	return server, nil
}

func (s *Server) Handler() http.Handler { return s.engine }

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data, "msg": "ok"})
}

func fail(c *gin.Context, err error) {
	typed := accounts.AsError(err)
	c.AbortWithStatusJSON(typed.Status, gin.H{"code": typed.Status, "data": nil, "msg": typed.Message, "reason": typed.Reason})
}

func failWith(c *gin.Context, status int, reason, message string) {
	c.AbortWithStatusJSON(status, gin.H{"code": status, "data": nil, "msg": message, "reason": reason})
}

func normalizeOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

// requireSameOrigin is the CSRF guard. The session cookie is SameSite=Lax, and
// every unsafe request must also carry an Origin (or Sec-Fetch-Site) proving it
// came from this site.
func (s *Server) requireSameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			if c.GetHeader("Sec-Fetch-Site") == "same-origin" {
				c.Next()
				return
			}
			failWith(c, http.StatusForbidden, "origin_required", "请求来源无法确认")
			return
		}
		normalized := normalizeOrigin(origin)
		if normalized != "" {
			parsed, _ := url.Parse(normalized)
			if strings.EqualFold(parsed.Host, s.requestHost(c)) || s.origins[normalized] {
				c.Next()
				return
			}
		}
		failWith(c, http.StatusForbidden, "origin_forbidden", "不允许的请求来源")
	}
}

// requestHost honours X-Forwarded-Host only from a trusted proxy.
func (s *Server) requestHost(c *gin.Context) string {
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwarded != "" && s.fromTrustedProxy(c) {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return c.Request.Host
}

func (s *Server) fromTrustedProxy(c *gin.Context) bool {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return false
	}
	for _, proxy := range s.opts.TrustedProxies {
		if strings.Contains(proxy, "/") {
			if _, network, err := net.ParseCIDR(proxy); err == nil && network.Contains(remote) {
				return true
			}
			continue
		}
		if ip := net.ParseIP(proxy); ip != nil && ip.Equal(remote) {
			return true
		}
	}
	return false
}

func (s *Server) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", Expires: expires,
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode})
}

const principalKey = "teamserver.principal"

func (s *Server) principal(c *gin.Context) (*accounts.Principal, error) {
	if value, exists := c.Get(principalKey); exists {
		return value.(*accounts.Principal), nil
	}
	cookie, err := c.Request.Cookie(SessionCookie)
	if err != nil {
		return nil, accounts.ErrUnauthenticated
	}
	principal, err := s.opts.Accounts.ResolveSession(c.Request.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}
	c.Set(principalKey, principal)
	return principal, nil
}

func (s *Server) requireSession(c *gin.Context) {
	if _, err := s.principal(c); err != nil {
		if errors.Is(err, accounts.ErrUnauthenticated) {
			s.clearSessionCookie(c)
		}
		fail(c, err)
		return
	}
	c.Next()
}

func (s *Server) requireAdmin(c *gin.Context) {
	principal, err := s.principal(c)
	if err != nil {
		fail(c, err)
		return
	}
	if !principal.IsAdmin() {
		fail(c, accounts.ErrForbidden)
		return
	}
	c.Next()
}

// workspaceSpec resolves the stored relative directory under the data root and
// refuses anything that would escape it.
func (s *Server) workspaceSpec(workspace accounts.Workspace) (kernelpool.Spec, error) {
	relative := filepath.Clean(filepath.FromSlash(workspace.DataDir))
	if relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
		return kernelpool.Spec{}, errors.New("工作区目录登记无效")
	}
	relayKey := func() (string, error) { return s.opts.Accounts.RelayKey(workspace) }
	return kernelpool.Spec{ID: workspace.ID, DataDir: filepath.Join(s.opts.DataRoot, relative), RelayKey: relayKey}, nil
}

// WorkspaceSpec exposes the same resolution to the process entrypoint.
func (s *Server) WorkspaceSpec(workspace accounts.Workspace) (kernelpool.Spec, error) {
	return s.workspaceSpec(workspace)
}

// gateway forwards an authenticated /api request to the caller's kernel. The
// target workspace comes only from the session, never from the request.
func (s *Server) gateway(c *gin.Context) {
	if bootstrap.IsHostedDeniedPath(c.Request.Method, c.Request.URL.Path) {
		failWith(c, http.StatusNotFound, "hosted_unavailable", "团队服务器不提供该功能")
		return
	}
	principal, err := s.principal(c)
	if err != nil {
		if errors.Is(err, accounts.ErrUnauthenticated) {
			s.clearSessionCookie(c)
		}
		fail(c, err)
		return
	}
	spec, err := s.workspaceSpec(principal.Workspace)
	if err != nil {
		failWith(c, http.StatusInternalServerError, "workspace_unavailable", "工作区不可用")
		return
	}
	kernel, err := s.opts.Pool.Get(c.Request.Context(), spec)
	if err != nil {
		if c.Request.Context().Err() == nil {
			log.Printf("open workspace=%s: %v", spec.ID, err)
		}
		failWith(c, http.StatusServiceUnavailable, "workspace_unavailable", "工作区暂时无法打开，请稍后重试")
		return
	}
	// Kernels never read cookies; keep the session token out of their logs and proxies.
	c.Request.Header.Del("Cookie")
	kernel.GatewayHandler().ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

func (s *Server) routes() {
	s.engine.GET("/api/health/live", func(c *gin.Context) { ok(c, gin.H{"live": true}) })
	platform := s.engine.Group("/api/platform")
	s.platformRoutes(platform)
	admin := platform.Group("/admin", s.requireAdmin)
	s.adminRoutes(admin)
	s.marketRoutes(platform, admin)
	s.workshopRoutes(admin)
	s.engine.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") {
			if strings.HasPrefix(requestPath, "/api/platform/") {
				failWith(c, http.StatusNotFound, "not_found", "请求不存在")
				return
			}
			s.gateway(c)
			return
		}
		s.serveWeb(c)
	})
}

// serveWeb serves the SPA: real files as-is, every other path gets index.html.
func (s *Server) serveWeb(c *gin.Context) {
	if s.opts.WebDir == "" || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
		failWith(c, http.StatusNotFound, "not_found", "请求不存在")
		return
	}
	cleaned := path.Clean("/" + c.Request.URL.Path)
	candidate := filepath.Join(s.opts.WebDir, filepath.FromSlash(cleaned))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		if strings.HasPrefix(cleaned, "/static/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.File(candidate)
		return
	}
	if path.Ext(cleaned) != "" && strings.HasPrefix(cleaned, "/static/") {
		failWith(c, http.StatusNotFound, "not_found", "文件不存在")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.File(filepath.Join(s.opts.WebDir, "index.html"))
}
