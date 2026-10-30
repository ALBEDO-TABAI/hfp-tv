package teamserver

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"infinite-canvas/backend/internal/accounts"

	"github.com/gin-gonic/gin"
)

// meView is the startup contract the web app uses to detect hosted mode and
// hide desktop-only capabilities.
func (s *Server) meView(principal *accounts.Principal) gin.H {
	relay := s.opts.Accounts.RelayStatus(principal.Workspace)
	return gin.H{
		"user":      gin.H{"id": principal.User.ID, "username": principal.User.Username, "displayName": principal.User.DisplayName, "role": principal.User.Role},
		"workspace": gin.H{"id": principal.Workspace.ID, "status": principal.Workspace.Status},
		"relay":     gin.H{"bound": relay.Bound, "hint": relay.Hint},
		"capabilities": gin.H{
			"hosted": true, "externalAgents": false, "storageSettings": false, "eagle": false,
			"depthCapture": false, "pluginInstall": false, "diagnostics": false, "userAdmin": principal.IsAdmin(),
			"skillReview": principal.IsAdmin() && s.opts.Market != nil,
			"workshop":    s.opts.Workshop != nil, "workshopReview": principal.IsAdmin() && s.opts.Workshop != nil,
		},
	}
}

// login issues the cookie and answers with the same payload as /me.
func (s *Server) startSession(c *gin.Context, user *accounts.User) {
	token, expires, err := s.opts.Accounts.IssueSession(c.Request.Context(), user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		fail(c, err)
		return
	}
	principal, err := s.opts.Accounts.ResolveSession(c.Request.Context(), token)
	if err != nil {
		fail(c, err)
		return
	}
	s.setSessionCookie(c, token, expires)
	ok(c, s.meView(principal))
}

func (s *Server) platformRoutes(r *gin.RouterGroup) {
	r.GET("/me", func(c *gin.Context) {
		principal, err := s.principal(c)
		if err != nil {
			s.clearSessionCookie(c)
			fail(c, err)
			return
		}
		ok(c, s.meView(principal))
	})
	r.POST("/login", func(c *gin.Context) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidCredentials)
			return
		}
		user, err := s.opts.Accounts.Authenticate(c.Request.Context(), body.Username, body.Password, c.ClientIP())
		if err != nil {
			fail(c, err)
			return
		}
		s.startSession(c, user)
	})
	r.POST("/logout", func(c *gin.Context) {
		if cookie, err := c.Request.Cookie(SessionCookie); err == nil {
			if err := s.opts.Accounts.RevokeSession(c.Request.Context(), cookie.Value); err != nil {
				fail(c, err)
				return
			}
		}
		s.clearSessionCookie(c)
		ok(c, gin.H{"loggedOut": true})
	})
	r.POST("/password", s.requireSession, func(c *gin.Context) {
		var body struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidPassword)
			return
		}
		principal, _ := s.principal(c)
		if err := s.opts.Accounts.ChangePassword(c.Request.Context(), principal, body.CurrentPassword, body.NewPassword); err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"changed": true})
	})
	r.GET("/invitations/:token", func(c *gin.Context) {
		invitation, err := s.opts.Accounts.Invitation(c.Request.Context(), c.Param("token"))
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"role": invitation.Role, "expiresAt": invitation.ExpiresAt})
	})
	r.POST("/invitations/:token/accept", func(c *gin.Context) {
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidUsername)
			return
		}
		user, err := s.opts.Accounts.AcceptInvitation(c.Request.Context(), c.Param("token"), body.Username, body.DisplayName, body.Password)
		if err != nil {
			fail(c, err)
			return
		}
		s.startSession(c, user)
	})
}

func (s *Server) adminRoutes(r *gin.RouterGroup) {
	r.GET("/users", func(c *gin.Context) {
		items, err := s.opts.Accounts.ListUsers(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
	r.POST("/users", func(c *gin.Context) {
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
			Role        string `json:"role"`
			RelayKey    string `json:"relayKey"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidUsername)
			return
		}
		principal, _ := s.principal(c)
		user, err := s.opts.Accounts.CreateUser(c.Request.Context(), principal.User.ID, accounts.CreateUserInput{
			Username: body.Username, DisplayName: body.DisplayName, Password: body.Password, Role: body.Role, RelayKey: body.RelayKey,
		})
		if err != nil {
			fail(c, err)
			return
		}
		s.respondUser(c, user.ID)
	})
	r.PATCH("/users/:id", func(c *gin.Context) {
		var body struct {
			DisplayName *string `json:"displayName"`
			Role        *string `json:"role"`
			Status      *string `json:"status"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidStatus)
			return
		}
		principal, _ := s.principal(c)
		view, err := s.opts.Accounts.UpdateUser(c.Request.Context(), principal, c.Param("id"), accounts.UserPatch{DisplayName: body.DisplayName, Role: body.Role, Status: body.Status})
		if err != nil {
			fail(c, err)
			return
		}
		if body.Status != nil && *body.Status == accounts.StatusDisabled && view.WorkspaceID != "" {
			// Sessions are already revoked; stop the kernel and its assistant child too.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := s.opts.Pool.Close(ctx, view.WorkspaceID); err != nil {
				log.Printf("close disabled workspace=%s: %v", view.WorkspaceID, err)
			}
		}
		ok(c, gin.H{"user": view})
	})
	r.POST("/users/:id/password", func(c *gin.Context) {
		var body struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidPassword)
			return
		}
		principal, _ := s.principal(c)
		if err := s.opts.Accounts.SetPassword(c.Request.Context(), principal, c.Param("id"), body.Password); err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"changed": true})
	})
	setRelay := func(c *gin.Context, key string) {
		principal, _ := s.principal(c)
		view, workspace, err := s.opts.Accounts.SetRelayKey(c.Request.Context(), principal, c.Param("id"), key)
		if err != nil {
			fail(c, err)
			return
		}
		spec, err := s.workspaceSpec(*workspace)
		if err == nil {
			err = s.opts.Pool.Reprovision(spec)
		}
		if err != nil {
			// The binding is saved; the next kernel open applies it.
			log.Printf("apply relay key workspace=%s: %v", workspace.ID, err)
		}
		ok(c, gin.H{"user": view})
	}
	r.PUT("/users/:id/relay-key", func(c *gin.Context) {
		var body struct {
			Key string `json:"key"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Key == "" {
			fail(c, accounts.ErrInvalidRelayKey)
			return
		}
		setRelay(c, body.Key)
	})
	r.DELETE("/users/:id/relay-key", func(c *gin.Context) { setRelay(c, "") })
	r.POST("/invitations", func(c *gin.Context) {
		var body struct {
			Role           string `json:"role"`
			ExpiresInHours int    `json:"expiresInHours"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, accounts.ErrInvalidRole)
			return
		}
		if body.Role == "" {
			body.Role = accounts.RoleMember
		}
		principal, _ := s.principal(c)
		token, expires, err := s.opts.Accounts.CreateInvitation(c.Request.Context(), principal, body.Role, time.Duration(body.ExpiresInHours)*time.Hour)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"token": token, "url": "/invite/" + token, "expiresAt": expires})
	})
	r.GET("/audit", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.Query("limit"))
		items, err := s.opts.Accounts.Audit(c.Request.Context(), limit)
		if err != nil {
			fail(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	})
}

func (s *Server) respondUser(c *gin.Context, userID string) {
	items, err := s.opts.Accounts.ListUsers(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	for _, item := range items {
		if item.ID == userID {
			ok(c, gin.H{"user": item})
			return
		}
	}
	failWith(c, http.StatusNotFound, "user_not_found", "用户不存在")
}
