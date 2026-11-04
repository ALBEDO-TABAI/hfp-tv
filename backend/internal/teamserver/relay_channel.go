package teamserver

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"infinite-canvas/backend/internal/kernelpool"

	"github.com/gin-gonic/gin"
)

// RelayTemplates holds the company relay definition the kernel pool writes
// into every workspace. *kernelpool.Pool implements it.
type RelayTemplates interface {
	RelayTemplate() map[string]any
	SetRelayTemplate(template map[string]any)
}

// maxRelayChannelBody bounds a saved definition; hundreds of models with
// their capability settings fit well within it.
const maxRelayChannelBody = 2 << 20

type relayChannelMember struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type relayChannelState struct {
	Channel       map[string]any      `json:"channel"`
	Source        string              `json:"source"`
	File          string              `json:"file"`
	FileAvailable bool                `json:"fileAvailable"`
	UpdatedAt     *time.Time          `json:"updatedAt,omitempty"`
	UpdatedBy     *relayChannelMember `json:"updatedBy,omitempty"`
	Applied       *int                `json:"applied,omitempty"`
	Failed        *int                `json:"failed,omitempty"`
}

func (s *Server) relayChannelRoutes(admin *gin.RouterGroup) {
	admin.GET("/relay-channel", s.getRelayChannel)
	admin.PUT("/relay-channel", s.saveRelayChannel)
	admin.DELETE("/relay-channel", s.resetRelayChannel)
}

func (s *Server) relayEditingAvailable(c *gin.Context) bool {
	if s.opts.RelayChannels == nil || s.opts.RelayTemplates == nil {
		failWith(c, http.StatusNotFound, "relay_channel_unavailable", "当前部署不支持在网页上编辑公司中转站")
		return false
	}
	return true
}

func (s *Server) getRelayChannel(c *gin.Context) {
	if !s.relayEditingAvailable(c) {
		return
	}
	state, err := s.relayChannelState(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, state)
}

func (s *Server) saveRelayChannel(c *gin.Context) {
	if !s.relayEditingAvailable(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRelayChannelBody)
	var body struct {
		Channel map[string]any `json:"channel"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Channel == nil {
		failWith(c, http.StatusBadRequest, "relay_channel_invalid", "请求里缺少公司中转站定义")
		return
	}
	template, err := kernelpool.NormalizeRelayTemplate(body.Channel)
	if err != nil {
		failWith(c, http.StatusBadRequest, "relay_channel_invalid", err.Error())
		return
	}
	principal, _ := s.principal(c)
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if _, err := kernelpool.TrustRelayHosts(template); err != nil {
		failWith(c, http.StatusBadRequest, "relay_channel_invalid", err.Error())
		return
	}
	if _, err := s.opts.RelayChannels.Save(principal.User.ID, template); err != nil {
		fail(c, err)
		return
	}
	s.opts.RelayTemplates.SetRelayTemplate(template)
	s.respondApplied(c)
}

func (s *Server) resetRelayChannel(c *gin.Context) {
	if !s.relayEditingAvailable(c) {
		return
	}
	if s.opts.Status.RelayFileTemplate == nil {
		failWith(c, http.StatusConflict, "relay_channel_no_file", "服务器没有配置公司中转站文件，无法恢复；请直接修改网页上的定义")
		return
	}
	principal, _ := s.principal(c)
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if _, err := kernelpool.TrustRelayHosts(s.opts.Status.RelayFileTemplate); err != nil {
		fail(c, err)
		return
	}
	if err := s.opts.RelayChannels.Delete(principal.User.ID); err != nil {
		fail(c, err)
		return
	}
	s.opts.RelayTemplates.SetRelayTemplate(s.opts.Status.RelayFileTemplate)
	s.respondApplied(c)
}

// respondApplied rewrites every member's company relay channel with the new
// definition. Open kernels read the channel from their configuration on each
// call, so the change takes effect without a restart; a workspace that cannot
// be rewritten now gets it the next time it opens.
func (s *Server) respondApplied(c *gin.Context) {
	ctx := c.Request.Context()
	workspaces, err := s.opts.Accounts.ActiveWorkspaces(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	applied, failed := 0, 0
	for _, workspace := range workspaces {
		spec, err := s.workspaceSpec(workspace)
		if err == nil {
			err = s.opts.Pool.Reprovision(spec)
		}
		if err != nil {
			failed++
			log.Printf("apply company relay workspace=%s: %v", workspace.ID, err)
			continue
		}
		applied++
	}
	state, err := s.relayChannelState(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	state.Applied, state.Failed = &applied, &failed
	ok(c, state)
}

func (s *Server) relayChannelState(ctx context.Context) (relayChannelState, error) {
	state := relayChannelState{Source: "none", File: s.opts.Status.RelayFile, FileAvailable: s.opts.Status.RelayFileTemplate != nil}
	saved, err := s.opts.RelayChannels.Load()
	if err != nil {
		return state, err
	}
	state.Channel = s.opts.RelayTemplates.RelayTemplate()
	switch {
	case saved != nil:
		state.Source = "web"
		updatedAt := saved.UpdatedAt
		state.UpdatedAt = &updatedAt
		state.UpdatedBy = s.relayEditor(ctx, saved.UpdatedBy)
	case state.Channel != nil:
		state.Source = "file"
	}
	return state, nil
}

func (s *Server) relayEditor(ctx context.Context, userID string) *relayChannelMember {
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	users, err := s.opts.Accounts.ListUsers(ctx)
	if err != nil {
		return nil
	}
	for _, user := range users {
		if user.ID == userID {
			return &relayChannelMember{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
		}
	}
	return nil
}

// relaySource reports where the definition in effect came from, for the
// service-status page.
func (s *Server) relaySource() (string, *time.Time) {
	if s.opts.RelayChannels != nil {
		if saved, err := s.opts.RelayChannels.Load(); err == nil && saved != nil {
			updatedAt := saved.UpdatedAt
			return "web", &updatedAt
		}
	}
	if s.currentRelayTemplate() != nil {
		return "file", nil
	}
	return "none", nil
}

// currentRelayTemplate is the definition in effect: the pool's when the
// deployment can edit it, otherwise the file loaded at startup.
func (s *Server) currentRelayTemplate() map[string]any {
	if s.opts.RelayTemplates != nil {
		return s.opts.RelayTemplates.RelayTemplate()
	}
	return s.opts.Status.RelayFileTemplate
}

var _ RelayTemplates = (*kernelpool.Pool)(nil)
