package teamserver

import (
	"fmt"
	"strings"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/outbound"

	"github.com/gin-gonic/gin"
)

// DeploymentStatus is what the team server loaded at startup. The admin
// service-status page shows it so a missed setting is visible in the browser,
// not only in the container log.
type DeploymentStatus struct {
	RelayFile     string
	RelayTemplate map[string]any
	// RelayHost is the company relay's intranet host trusted automatically.
	RelayHost   string
	UsersFile   string
	KeysFile    string
	IdleTimeout time.Duration
}

type relayModelView struct {
	Model                 string `json:"model"`
	Capability            string `json:"capability"`
	Protocol              string `json:"protocol"`
	BaseURL               string `json:"baseUrl"`
	ReferenceAssetService string `json:"referenceAssetService"`
}

func relayModels(template map[string]any) []relayModelView {
	text := func(profile map[string]any, key string) string {
		value, _ := profile[key].(string)
		return strings.TrimSpace(value)
	}
	byModel := map[string]relayModelView{}
	profiles, _ := template["modelProfiles"].([]any)
	for _, raw := range profiles {
		if profile, ok := raw.(map[string]any); ok {
			model := text(profile, "model")
			byModel[model] = relayModelView{Model: model, Capability: text(profile, "capability"), Protocol: text(profile, "protocol"),
				BaseURL: text(profile, "baseUrl"), ReferenceAssetService: text(profile, "referenceAssetService")}
		}
	}
	models := []relayModelView{}
	listed, _ := template["models"].([]any)
	for _, raw := range listed {
		if model := strings.TrimSpace(fmt.Sprint(raw)); model != "" {
			view, ok := byModel[model]
			if !ok {
				view = relayModelView{Model: model}
			}
			models = append(models, view)
		}
	}
	return models
}

func (s *Server) serviceStatus(c *gin.Context) {
	ctx := c.Request.Context()
	users, err := s.opts.Accounts.ListUsers(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	workspaces, err := s.opts.Accounts.ActiveWorkspaces(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	active, withKey := 0, 0
	for _, user := range users {
		if user.Status == accounts.StatusActive {
			active++
			if user.Relay.Bound {
				withKey++
			}
		}
	}
	status := s.opts.Status
	template := status.RelayTemplate
	text := func(key string) string {
		value, _ := template[key].(string)
		return strings.TrimSpace(value)
	}
	relay := gin.H{"configured": template != nil, "name": text("name"), "baseUrl": text("baseUrl"), "apiFormat": text("apiFormat"),
		"models": relayModels(template), "file": status.RelayFile, "autoAllowedHost": status.RelayHost}
	if template != nil && text("name") == "" {
		relay["name"] = "公司中转站"
	}
	hosts := outbound.AllowedPrivateUpstreamHosts()
	if hosts == nil {
		hosts = []string{}
	}
	ok(c, gin.H{
		"relay":               relay,
		"accountFiles":        gin.H{"usersFile": status.UsersFile, "keysFile": status.KeysFile},
		"allowedPrivateHosts": hosts,
		"cookieSecure":        string(s.opts.CookieSecure),
		"idleTimeoutSeconds":  int64(status.IdleTimeout / time.Second),
		"workspaces":          gin.H{"open": s.opts.Pool.OpenCount(), "total": len(workspaces)},
		"members":             gin.H{"active": active, "withRelayKey": withKey},
	})
}
