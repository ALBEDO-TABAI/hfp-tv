package kernelpool

import (
	"encoding/json"
	"infinite-canvas/backend/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRelayTemplateKeepsModelEndpointsOnTheRelay(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		path := filepath.Join(dir, "relay.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ok := write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.45:3000/doubao/api/v3"}]}`)
	if _, err := LoadRelayTemplate(ok); err != nil {
		t.Fatalf("same-origin model endpoint rejected: %v", err)
	}
	for name, body := range map[string]string{
		"other host":   `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.46:3000/doubao/api/v3"}]}`,
		"other port":   `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"http://10.10.10.45:8787/v1"}]}`,
		"other scheme": `{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"seedance","baseUrl":"https://10.10.10.45:3000/v1"}]}`,
	} {
		if _, err := LoadRelayTemplate(write(body)); err == nil || !strings.Contains(err.Error(), "同协议") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

type concurrentRelayConfigStore struct {
	*workspace.ProviderConfig
	t     *testing.T
	wrote bool
}

func (s *concurrentRelayConfigStore) LoadEffectiveModelConfig() (workspace.EffectiveModelConfig, workspace.ConfigHealth, error) {
	got, health, err := s.ProviderConfig.LoadEffectiveModelConfig()
	if err == nil && !s.wrote {
		s.wrote = true
		if err := s.ProviderConfig.SaveLocalModelConfig([]byte(`{"textModel":"personal::latest","channels":[{"id":"personal","apiKey":"SYNTHETIC_PERSONAL"}]}`)); err != nil {
			s.t.Fatal(err)
		}
	}
	return got, health, err
}

func TestRelayProvisionPreservesConcurrentMemberSave(t *testing.T) {
	store, err := workspace.NewProviderConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := updateRelayChannel(&concurrentRelayConfigStore{ProviderConfig: store, t: t}, map[string]any{"baseUrl": "https://relay.invalid"}, "SYNTHETIC_COMPANY"); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Config["textModel"] != "personal::latest" {
		t.Fatal("provision overwrote concurrent member selection")
	}
	found := false
	for _, raw := range got.Config["channels"].([]any) {
		c := raw.(map[string]any)
		if c["id"] == "personal" && c["apiKey"] == "SYNTHETIC_PERSONAL" {
			found = true
		}
	}
	if !found {
		t.Fatal("provision erased concurrent personal channel")
	}
}

func TestRelayMediaReviewDefaultUsesSupportedModel(t *testing.T) {
	dir := t.TempDir()
	store, err := workspace.NewProviderConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLocalModelConfig([]byte(`{"mediaReviewModel":"company-relay::removed"}`)); err != nil {
		t.Fatal(err)
	}
	template := map[string]any{"baseUrl": "https://relay.invalid", "models": []any{"ordinary", "wand-vega-vl-1.0-flash"}, "modelProfiles": []any{map[string]any{"model": "ordinary", "capability": "text"}, map[string]any{"model": "wand-vega-vl-1.0-flash", "capability": "text", "protocol": "chat-completion"}}}
	if err := applyRelayChannel(dir, template, "SYNTHETIC_KEY"); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.LoadEffectiveModelConfig()
	if err != nil || got.Config["mediaReviewModel"] != "company-relay::wand-vega-vl-1.0-flash" {
		t.Fatal("review default did not select a supported model")
	}
}

func TestRelayReplacesOnlyInvalidCompanyDefaults(t *testing.T) {
	for _, capability := range []string{"text", "image", "video", "audio"} {
		t.Run(capability, func(t *testing.T) {
			dir := t.TempDir()
			store, err := workspace.NewProviderConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			config := map[string]any{"channels": []any{map[string]any{"id": "personal", "apiKey": "SYNTHETIC_PERSONAL"}}, "textModel": "personal::keep", "assistantModel": "company-relay::removed", "mediaReviewModel": "company-relay::removed"}
			config[capability+"Model"] = "company-relay::removed"
			body, _ := json.Marshal(config)
			if err := store.SaveLocalModelConfig(body); err != nil {
				t.Fatal(err)
			}
			template := map[string]any{"baseUrl": "https://relay.invalid/v1", "models": []any{"new"}, "modelProfiles": []any{map[string]any{"model": "ghost", "capability": capability}, map[string]any{"model": "new", "capability": capability}}}
			if err := applyRelayChannel(dir, template, "SYNTHETIC_COMPANY"); err != nil {
				t.Fatal(err)
			}
			got, _, err := store.LoadEffectiveModelConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got.Config[capability+"Model"] != "company-relay::new" {
				t.Errorf("deleted default was not replaced: %v", got.Config[capability+"Model"])
			}
			wantText := ""
			if capability == "text" {
				wantText = "company-relay::new"
			} else if got.Config["textModel"] != "personal::keep" {
				t.Error("personal default was changed")
			}
			if got.Config["assistantModel"] != wantText {
				t.Errorf("assistantModel = %v", got.Config["assistantModel"])
			}
			if got.Config["mediaReviewModel"] != "" {
				t.Error("ordinary text model cannot be selected for media review")
			}
			if err := applyRelayChannel(dir, map[string]any{"baseUrl": "https://relay.invalid/v1"}, "SYNTHETIC_COMPANY"); err != nil {
				t.Fatal(err)
			}
			got, _, err = store.LoadEffectiveModelConfig()
			if err != nil || got.Config[capability+"Model"] != "" {
				t.Fatal("removed last model must clear company default")
			}
		})
	}
}

func TestLoadRelayTemplateChecksReferenceServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"a","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"},{"model":"b","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"}]}`)
	template, err := LoadRelayTemplate(path)
	if err != nil {
		t.Fatal(err)
	}
	if services := RelayReferenceServices(template); len(services) != 1 || services[0] != "http://10.10.10.45:8787/v1/reference-images" {
		t.Fatalf("services = %v", services)
	}
	write(`{"baseUrl":"http://10.10.10.45:3000/v1","modelProfiles":[{"model":"a","referenceAssetService":"10.10.10.45:8787"}]}`)
	if _, err := LoadRelayTemplate(path); err == nil || !strings.Contains(err.Error(), "referenceAssetService") {
		t.Fatalf("bad service accepted: %v", err)
	}
}

// A selection is replaced only when its model left the relay. A listed model
// whose profile omits the capability is still usable and stays selected.
func TestRelayKeepsSelectionsOfListedModelsWithoutCapability(t *testing.T) {
	dir := t.TempDir()
	store, err := workspace.NewProviderConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"textModel": "company-relay::plain", "assistantModel": "company-relay::plain"})
	if err := store.SaveLocalModelConfig(body); err != nil {
		t.Fatal(err)
	}
	template := map[string]any{"baseUrl": "https://relay.invalid/v1", "models": []string{"plain", "other"},
		"modelProfiles": []any{map[string]any{"model": "plain", "protocol": "chat-completion"}, map[string]any{"model": "other", "capability": "text"}}}
	if err := applyRelayChannel(dir, template, "SYNTHETIC_COMPANY"); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Config["textModel"] != "company-relay::plain" || got.Config["assistantModel"] != "company-relay::plain" {
		t.Fatalf("usable selection replaced: text=%v assistant=%v", got.Config["textModel"], got.Config["assistantModel"])
	}
}

func TestRelayTemplateWithOnlyProfilesKeepsSelections(t *testing.T) {
	dir := t.TempDir()
	store, err := workspace.NewProviderConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"assistantModel": "company-relay::profiled"})
	if err := store.SaveLocalModelConfig(body); err != nil {
		t.Fatal(err)
	}
	template := map[string]any{"baseUrl": "https://relay.invalid/v1", "modelProfiles": []any{map[string]any{"model": "profiled", "capability": "text"}}}
	if err := applyRelayChannel(dir, template, "SYNTHETIC_COMPANY"); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.LoadEffectiveModelConfig()
	if err != nil || got.Config["assistantModel"] != "company-relay::profiled" {
		t.Fatalf("profiles-only selection replaced: %v %v", got.Config["assistantModel"], err)
	}
}
