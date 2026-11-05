package modelcatalog

import "testing"

// A relay can serve a text model under its own path; the assistant must call
// that path, but never a different host with the channel key.
func TestAssistantUsesAModelsOwnEndpointOnlyOnTheChannelOrigin(t *testing.T) {
	snapshot := func(modelBase string) AssistantConfigSnapshot {
		return AssistantConfigSnapshot{
			AssistantModel: "company-relay::relay-text",
			Channels: []AssistantChannel{{
				ID: "company-relay", Name: "公司中转站", Enabled: true, Pinned: true,
				BaseURL: "http://10.10.10.45:3000/v1", APIKey: "fixture-relay-key",
				Models:        []string{"relay-text"},
				ModelProfiles: []AssistantModelProfile{{Model: "relay-text", Capability: "text", Protocol: "chat-completion", BaseURL: modelBase}},
			}},
		}
	}
	for _, tc := range []struct{ name, modelBase, want string }{
		{"no own endpoint", "", "http://10.10.10.45:3000/v1"},
		{"same origin", "http://10.10.10.45:3000/alternate/v1", "http://10.10.10.45:3000/alternate/v1"},
		{"other host", "http://elsewhere.example:3000/v1", "http://10.10.10.45:3000/v1"},
		{"other port", "http://10.10.10.45:4000/v1", "http://10.10.10.45:3000/v1"},
		{"credentials in URL", "http://user:pass@10.10.10.45:3000/v1", "http://10.10.10.45:3000/v1"},
	} {
		provider, err := ResolveAssistantProvider(snapshot(tc.modelBase), nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if provider.BaseURL != tc.want || provider.APIKey != "fixture-relay-key" {
			t.Fatalf("%s: base=%q", tc.name, provider.BaseURL)
		}
	}
	// The endpoint is part of the fingerprint, so changing it restarts the host.
	same, _ := ResolveAssistantProvider(snapshot(""), nil)
	moved, _ := ResolveAssistantProvider(snapshot("http://10.10.10.45:3000/alternate/v1"), nil)
	if same.Fingerprint() == moved.Fingerprint() {
		t.Fatal("endpoint change did not change the provider fingerprint")
	}
}
