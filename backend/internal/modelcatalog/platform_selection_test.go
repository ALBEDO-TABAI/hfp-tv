package modelcatalog

import "testing"

func TestRemovedAssistantModelIsReplacedOnlyByOneTheAssistantCanUse(t *testing.T) {
	channel := map[string]any{"id": "company-relay", "baseUrl": "https://relay.invalid/v1", "models": []any{"gemini-text", "chat-text"},
		"modelProfiles": []any{
			map[string]any{"model": "gemini-text", "capability": "text", "protocol": "gemini-generate-content"},
			map[string]any{"model": "chat-text", "capability": "text", "protocol": "chat-completion"},
		}}
	config := map[string]any{"assistantModel": "company-relay::removed", "textModel": "company-relay::removed", "imageModel": "mine::keep"}
	if err := CorrectPlatformChannelSelections(config, channel); err != nil {
		t.Fatal(err)
	}
	if config["assistantModel"] != "company-relay::chat-text" {
		t.Fatalf("assistant got %v", config["assistantModel"])
	}
	// Ordinary text generation can use the first text model.
	if config["textModel"] != "company-relay::gemini-text" || config["imageModel"] != "mine::keep" {
		t.Fatalf("text=%v image=%v", config["textModel"], config["imageModel"])
	}
}
