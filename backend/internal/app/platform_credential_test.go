package app

import (
	"testing"

	"infinite-canvas/backend/internal/modelcatalog"
)

func TestPlatformChannelKeyIsFilledServerSideOnlyForItsOrigin(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[
		{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","apiKey":"sk-member-relay","pinned":true},
		{"id":"mine","baseUrl":"https://api.example.com/v1","apiKey":"sk-mine"}]}`)); err != nil {
		t.Fatal(err)
	}

	// Outside the team server no channel is managed: requests pass through.
	untouched, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "company-relay", "apiKey": ""}})
	if err != nil || untouched["config"].(map[string]any)["apiKey"] != "" {
		t.Fatalf("unmanaged workspace resolved a key: %v %v", untouched, err)
	}

	s.SetPlatformChannels([]string{"company-relay"})
	resolved, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{
		"channelId": "company-relay", "apiKey": "", "baseUrl": "http://attacker.example/v1", "model": "gpt-image-2"}})
	if err != nil {
		t.Fatal(err)
	}
	config := resolved["config"].(map[string]any)
	if config["apiKey"] != "sk-member-relay" || config["baseUrl"] != "http://10.10.10.45:3000/v1" || config["model"] != "gpt-image-2" {
		t.Fatalf("task config = %v", config)
	}
	// Admission treats any channel ID as a system-catalog channel.
	if _, kept := config["channelId"]; kept || !modelcatalog.TaskInputUsesCustomChannel(resolved) {
		t.Fatalf("resolved task is not a custom-channel task: %v", config)
	}

	other, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "mine", "apiKey": "typed"}})
	if err != nil || other["config"].(map[string]any)["apiKey"] != "typed" {
		t.Fatalf("member channel changed: %v %v", other, err)
	}

	models := ChannelModelsRequest{ChannelID: "company-relay", BaseURL: "http://attacker.example/v1"}
	if err := s.resolveChannelModelsRequest(&models); err != nil || models.APIKey != "sk-member-relay" || models.BaseURL != "http://10.10.10.45:3000/v1" {
		t.Fatalf("models request = %+v %v", models, err)
	}

	for _, check := range []struct{ target, incoming, want string }{
		{"http://10.10.10.45:3000/v1/images/generations", "", "sk-member-relay"},
		{"http://10.10.10.45:3000/v1/images/generations", "sk-own", "sk-own"}, // a member's own key is kept
		{"http://10.10.10.45:3001/v1/images/generations", "", ""},             // other port, other origin
		{"https://10.10.10.45:3000/v1/images/generations", "", ""},            // other scheme
		{"https://api.example.com/v1/images/generations", "", ""},
	} {
		got, err := s.resolveCustomRelayKey(check.target, check.incoming)
		if err != nil || got != check.want {
			t.Errorf("relay key for %s (incoming %q) = %q, want %q (%v)", check.target, check.incoming, got, check.want, err)
		}
	}
}

func TestPlatformChannelWithoutBoundKeyFailsClearly(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"https://relay.example.com/v1","apiKey":"","pinned":true}]}`)); err != nil {
		t.Fatal(err)
	}
	s.SetPlatformChannels([]string{"company-relay"})
	if _, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "company-relay"}}); err == nil {
		t.Fatal("task without a bound key was accepted")
	}
	models := ChannelModelsRequest{ChannelID: "company-relay"}
	if err := s.resolveChannelModelsRequest(&models); err == nil {
		t.Fatal("model listing without a bound key was accepted")
	}
}

func TestPlatformChannelModelEndpointStaysOnTheRelayOrigin(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","apiKey":"sk-member-relay","pinned":true,
		"modelProfiles":[
			{"model":"doubao-seedance-2-0-260128","capability":"video","protocol":"volcengine-ark-video","baseUrl":"http://10.10.10.45:3000/doubao/api/v3"},
			{"model":"elsewhere","capability":"video","baseUrl":"http://203.0.113.9/doubao/api/v3"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	s.SetPlatformChannels([]string{"company-relay"})
	for model, want := range map[string]string{
		"doubao-seedance-2-0-260128": "http://10.10.10.45:3000/doubao/api/v3",
		"elsewhere":                  "http://10.10.10.45:3000/v1", // another host never receives the key
		"gpt-image-2":                "http://10.10.10.45:3000/v1",
	} {
		resolved, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "company-relay", "model": model}})
		if err != nil {
			t.Fatal(err)
		}
		if got := resolved["config"].(map[string]any)["baseUrl"]; got != want {
			t.Errorf("%s baseUrl = %v, want %s", model, got, want)
		}
	}
	models := ChannelModelsRequest{ChannelID: "company-relay"}
	if err := s.resolveChannelModelsRequest(&models); err != nil || models.BaseURL != "http://10.10.10.45:3000/v1" {
		t.Fatalf("catalog must stay on the channel endpoint: %+v %v", models, err)
	}
}
