package teamserver

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/errorlog"

	"github.com/gin-gonic/gin"
)

type errorMember struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type errorEventView struct {
	errorlog.Event
	Member *errorMember `json:"member"`
}

// errorEvents is the administrator's error log: failed tasks pushed by the
// workspace kernels and workspaces that could not be opened, newest first.
// Query: member (user ID), source (task|workspace), before (RFC 3339), limit.
func (s *Server) errorEvents(c *gin.Context) {
	if s.opts.ErrorLog == nil {
		ok(c, gin.H{"events": []errorEventView{}, "nextBefore": ""})
		return
	}
	users, err := s.opts.Accounts.ListUsers(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	members := make(map[string]*errorMember, len(users))
	query := errorlog.Query{Source: strings.TrimSpace(c.Query("source"))}
	memberFilter := strings.TrimSpace(c.Query("member"))
	for _, user := range users {
		if user.WorkspaceID == "" {
			continue
		}
		members[user.WorkspaceID] = &errorMember{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
		if memberFilter != "" && user.ID == memberFilter {
			query.WorkspaceIDs = append(query.WorkspaceIDs, user.WorkspaceID)
		}
	}
	if memberFilter != "" && len(query.WorkspaceIDs) == 0 {
		ok(c, gin.H{"events": []errorEventView{}, "nextBefore": ""})
		return
	}
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		before, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			failWith(c, http.StatusBadRequest, "invalid_before", "before 必须是 RFC 3339 时间")
			return
		}
		query.Before = before
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		query.Limit, _ = strconv.Atoi(raw)
	}
	events, err := s.opts.ErrorLog.List(query)
	if err != nil {
		fail(c, err)
		return
	}
	views := make([]errorEventView, 0, len(events))
	for _, event := range events {
		views = append(views, errorEventView{Event: event, Member: members[event.WorkspaceID]})
	}
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	nextBefore := ""
	if len(events) == limit {
		nextBefore = events[len(events)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	ok(c, gin.H{"events": views, "nextBefore": nextBefore})
}

func (s *Server) recordWorkspaceError(workspaceID string, cause error) {
	if s.opts.ErrorLog == nil {
		return
	}
	if err := s.opts.ErrorLog.RecordWorkspace(workspaceID, cause); err != nil {
		log.Printf("error log write failed: workspace=%s error=%v", workspaceID, err)
	}
}
