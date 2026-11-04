package teamserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/accounts"
)

// relayChannelOf reads the company relay channel from a member's model
// configuration as the browser receives it.
func relayChannelOf(t *testing.T, h *harness, cookie string) (map[string]any, string) {
	t.Helper()
	response := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: cookie})
	if response.Code != http.StatusOK {
		t.Fatalf("model config = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Config struct {
				Channels []map[string]any `json:"channels"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, channel := range envelope.Data.Config.Channels {
		if channel["id"] == "company-relay" {
			return channel, response.Body.String()
		}
	}
	t.Fatalf("no company relay channel: %s", response.Body.String())
	return nil, ""
}

func relayModelNames(channel map[string]any) []string {
	var names []string
	models, _ := channel["models"].([]any)
	for _, model := range models {
		names = append(names, model.(string))
	}
	return names
}

func TestAdminEditsTheCompanyRelayForEveryMember(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	admin, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass", RelayKey: "sk-alice-relay-7777"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"}); err != nil {
		t.Fatal(err)
	}
	aliceCookie, adminCookie := h.login("alice", "alice-pass"), h.login("admin", "admin-pass")
	if channel, _ := relayChannelOf(t, h, aliceCookie); len(relayModelNames(channel)) != 2 {
		t.Fatalf("file definition not applied: %v", channel["models"])
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/platform/admin/relay-channel", cookie: aliceCookie}); response.Code != http.StatusForbidden {
		t.Fatalf("member read the relay definition: %d", response.Code)
	}
	state := h.do(call{method: http.MethodGet, path: "/api/platform/admin/relay-channel", cookie: adminCookie}).Body.String()
	if !strings.Contains(state, `"source":"file"`) || !strings.Contains(state, `"fileAvailable":true`) || !strings.Contains(state, "relay.example.com") {
		t.Fatalf("initial state = %s", state)
	}

	put := func(channel string) (int, string) {
		response := h.do(call{method: http.MethodPut, path: "/api/platform/admin/relay-channel", cookie: adminCookie, body: `{"channel":` + channel + `}`})
		return response.Code, response.Body.String()
	}
	for name, channel := range map[string]string{
		"a key":                   `{"name":"公司中转站","baseUrl":"https://relay.example.com/v1","apiKey":"sk-leak","models":["relay-text"]}`,
		"a model on another host": `{"name":"公司中转站","baseUrl":"https://relay.example.com/v1","models":["video"],"modelProfiles":[{"model":"video","baseUrl":"https://elsewhere.example.com/api/v3"}]}`,
		"no models":               `{"name":"公司中转站","baseUrl":"https://relay.example.com/v1","models":[]}`,
	} {
		if code, body := put(channel); code != http.StatusBadRequest || !strings.Contains(body, "relay_channel_invalid") {
			t.Fatalf("saving %s = %d %s", name, code, body)
		}
	}

	// The page sends its whole editor state; only the definition is kept.
	code, body := put(`{"id":"company-relay","enabled":true,"pinned":true,"hasApiKey":true,"apiKey":"","headers":[{"name":"X-Debug","value":"1"}],
		"name":"公司中转站","baseUrl":"https://relay.example.com/v1/","apiFormat":"openai",
		"models":["relay-model","relay-text","seedance-fast"],
		"modelProfiles":[{"model":"relay-model","capability":"image","protocol":"openai-image"},{"model":"relay-text","capability":"text","protocol":"chat-completion"},
			{"model":"seedance-fast","displayName":"Seedance Fast","capability":"video","protocol":"volcengine-ark-video","baseUrl":"https://relay.example.com/doubao/api/v3","referenceAssetService":"https://relay.example.com:8787/v1/reference-images"},
			{"model":"not-listed","capability":"text"}]}`)
	if code != http.StatusOK || !strings.Contains(body, `"source":"web"`) || !strings.Contains(body, `"applied":3`) || !strings.Contains(body, `"failed":0`) {
		t.Fatalf("save = %d %s", code, body)
	}
	for _, unwanted := range []string{"X-Debug", "not-listed", "hasApiKey", `"pinned"`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("saved definition kept %q: %s", unwanted, body)
		}
	}

	// Alice's open workspace has the new models and still never sees her key.
	channel, raw := relayChannelOf(t, h, aliceCookie)
	if names := relayModelNames(channel); len(names) != 3 || names[2] != "seedance-fast" || channel["hasApiKey"] != true || strings.Contains(raw, "sk-alice") {
		t.Fatalf("alice after save: %v %s", names, raw)
	}
	if !strings.Contains(raw, "/doubao/api/v3") || strings.Contains(raw, "X-Debug") {
		t.Fatalf("alice's channel lost the model endpoint or kept editor state: %s", raw)
	}
	// Bob had no open workspace; his first open gets the saved definition.
	if channel, _ := relayChannelOf(t, h, h.login("bob", "bob-pass1")); len(relayModelNames(channel)) != 3 || channel["hasApiKey"] == true {
		t.Fatalf("bob after save: %v", channel)
	}
	status := h.do(call{method: http.MethodGet, path: "/api/platform/admin/service-status", cookie: adminCookie}).Body.String()
	if !strings.Contains(status, `"source":"web"`) || !strings.Contains(status, "seedance-fast") {
		t.Fatalf("service status after save = %s", status)
	}
	audit := h.do(call{method: http.MethodGet, path: "/api/platform/admin/audit", cookie: adminCookie}).Body.String()
	if !strings.Contains(audit, "relay_channel.update") {
		t.Fatalf("audit = %s", audit)
	}

	// Reverting brings back the server file for everyone.
	reset := h.do(call{method: http.MethodDelete, path: "/api/platform/admin/relay-channel", cookie: adminCookie})
	if reset.Code != http.StatusOK || !strings.Contains(reset.Body.String(), `"source":"file"`) {
		t.Fatalf("reset = %d %s", reset.Code, reset.Body.String())
	}
	if channel, _ := relayChannelOf(t, h, aliceCookie); len(relayModelNames(channel)) != 2 {
		t.Fatalf("alice after reset: %v", channel["models"])
	}
}

func TestCompanyRelayCanBeDefinedWithoutAServerFile(t *testing.T) {
	h := newHarness(t, func(opts *Options) {
		opts.Status.RelayFile, opts.Status.RelayFileTemplate = "", nil
		opts.RelayTemplates.SetRelayTemplate(nil)
	})
	ctx := context.Background()
	admin, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass", RelayKey: "sk-alice-relay-8888"}); err != nil {
		t.Fatal(err)
	}
	adminCookie := h.login("admin", "admin-pass")
	if state := h.do(call{method: http.MethodGet, path: "/api/platform/admin/relay-channel", cookie: adminCookie}).Body.String(); !strings.Contains(state, `"source":"none"`) || !strings.Contains(state, `"channel":null`) {
		t.Fatalf("state without a file = %s", state)
	}
	if response := h.do(call{method: http.MethodDelete, path: "/api/platform/admin/relay-channel", cookie: adminCookie}); response.Code != http.StatusConflict {
		t.Fatalf("reset without a file = %d", response.Code)
	}
	response := h.do(call{method: http.MethodPut, path: "/api/platform/admin/relay-channel", cookie: adminCookie,
		body: `{"channel":{"baseUrl":"https://relay.example.com/v1","models":["relay-text"],"modelProfiles":[{"model":"relay-text","capability":"text","protocol":"chat-completion"}]}}`})
	if response.Code != http.StatusOK {
		t.Fatalf("first definition = %d %s", response.Code, response.Body.String())
	}
	channel, raw := relayChannelOf(t, h, h.login("alice", "alice-pass"))
	if channel["name"] != "公司中转站" || channel["hasApiKey"] != true || strings.Contains(raw, "sk-alice") {
		t.Fatalf("alice's channel = %s", raw)
	}
}
