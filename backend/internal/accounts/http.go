package accounts

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const CookieName = "hfp_session"

type HTTPConfig struct {
	AllowedOrigins []string
	InsecureCookie bool
}

func NewHandler(service *Service, config HTTPConfig) (http.Handler, error) {
	origins := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		normalized, err := normalizeOrigin(origin)
		if err != nil {
			return nil, fmt.Errorf("允许的 Origin 必须是明确的 http/https 来源，不能使用通配符")
		}
		origins[normalized] = true
	}
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	// 不启用访问日志或默认 panic 请求转储，避免邀请 URL、Cookie 和密码进入日志。
	router.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				log.Printf("platform request panic: route=%s", c.FullPath())
				writeError(c, errors.New("request panic"))
			}
		}()
		c.Next()
	}, csrf(origins))
	platform := router.Group("/api/platform")
	platform.POST("/login", func(c *gin.Context) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !bind(c, &input) {
			return
		}
		ip, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
		identity, token, err := service.Login(c.Request.Context(), input.Username, input.Password, ip, c.Request.UserAgent())
		if err != nil {
			writeError(c, err)
			return
		}
		setCookie(c, token, config.InsecureCookie, int(SessionLifetime.Seconds()))
		writeOK(c, identity)
	})
	platform.POST("/logout", func(c *gin.Context) {
		if err := service.Logout(c.Request.Context(), sessionToken(c)); err != nil {
			writeError(c, err)
			return
		}
		setCookie(c, "", config.InsecureCookie, -1)
		writeOK(c, nil)
	})
	platform.GET("/me", func(c *gin.Context) {
		identity, err := service.Authenticate(c.Request.Context(), sessionToken(c))
		if err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, identity)
	})
	platform.POST("/invitations/:token/accept", func(c *gin.Context) {
		var input NewUser
		if !bind(c, &input) {
			return
		}
		identity, err := service.AcceptInvitation(c.Request.Context(), c.Param("token"), input)
		if err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, identity)
	})
	admin := platform.Group("/admin")
	admin.GET("/users", func(c *gin.Context) {
		users, err := service.ListUsers(c.Request.Context(), sessionToken(c))
		if err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, users)
	})
	admin.POST("/users", func(c *gin.Context) {
		var input NewUser
		if !bind(c, &input) {
			return
		}
		identity, err := service.CreateUser(c.Request.Context(), sessionToken(c), input)
		if err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, identity)
	})
	admin.POST("/invitations", func(c *gin.Context) {
		var input struct {
			Role      string    `json:"role"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		if !bind(c, &input) {
			return
		}
		invitation, token, err := service.CreateInvitation(c.Request.Context(), sessionToken(c), input.Role, input.ExpiresAt)
		if err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, gin.H{"invitation": invitation, "token": token})
	})
	admin.POST("/users/:id/disable", func(c *gin.Context) {
		if err := service.DisableUser(c.Request.Context(), sessionToken(c), c.Param("id")); err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, nil)
	})
	admin.POST("/users/:id/reset-password", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !bind(c, &input) {
			return
		}
		if err := service.ResetPassword(c.Request.Context(), sessionToken(c), c.Param("id"), input.Password); err != nil {
			writeError(c, err)
			return
		}
		writeOK(c, nil)
	})
	router.NoRoute(func(c *gin.Context) { writeError(c, ErrNotFound) })
	return router, nil
}

func bind(c *gin.Context, input any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.ShouldBindJSON(input); err != nil {
		writeError(c, ErrInvalidInput)
		return false
	}
	return true
}

func sessionToken(c *gin.Context) string {
	token, _ := c.Cookie(CookieName)
	return token
}

func setCookie(c *gin.Context, token string, insecure bool, maxAge int) {
	cookie := &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: !insecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, cookie)
}

func writeOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data, "msg": "ok", "reason": ""})
}

func writeError(c *gin.Context, err error) {
	public := &Error{http.StatusInternalServerError, "internal_error", "系统处理失败，请稍后重试"}
	var known *Error
	if errors.As(err, &known) {
		public = known
	} else {
		log.Printf("platform request failed: route=%s error_type=%T", c.FullPath(), err)
	}
	c.AbortWithStatusJSON(public.Status, gin.H{"code": public.Status, "data": nil, "msg": public.Message, "reason": public.Reason})
}

func normalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(parsed.Host, "*") {
		return "", ErrCSRF
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

func csrf(origins map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin, err := normalizeOrigin(c.GetHeader("Origin"))
		allowed := err == nil && origins[origin]
		c.Header("Vary", "Origin")
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			// 预检只有“将发送哪些头”，实际写请求仍必须携带指定值。
			hasRequestedWith := false
			for _, header := range strings.Split(c.GetHeader("Access-Control-Request-Headers"), ",") {
				switch strings.ToLower(strings.TrimSpace(header)) {
				case "x-requested-with":
					hasRequestedWith = true
				case "content-type":
				default:
					writeError(c, ErrCSRF)
					return
				}
			}
			if !allowed || !hasRequestedWith || c.GetHeader("Access-Control-Request-Method") != http.MethodPost {
				writeError(c, ErrCSRF)
				return
			}
			c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			if !allowed || c.GetHeader("X-Requested-With") != "hfp" {
				writeError(c, ErrCSRF)
				return
			}
		}
		c.Next()
	}
}
