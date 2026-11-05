package modelcatalog

import (
	"encoding/json"
	"slices"
	"strings"
)

// platformSelectionFields are the default selections a platform channel can
// back, with the capability a replacement must have.
var platformSelectionFields = []struct{ field, capability string }{
	{"textModel", "text"}, {"assistantModel", "text"}, {"mediaReviewModel", "text"},
	{"imageModel", "image"}, {"videoModel", "video"}, {"audioModel", "audio"},
}

// PlatformChannelListedModels is a platform channel's model list, read from
// decoded JSON or Go values; a definition that names its models only through
// profiles uses those.
func PlatformChannelListedModels(channel map[string]any) []string {
	var listed []string
	switch value := channel["models"].(type) {
	case []any:
		for _, item := range value {
			if model, ok := item.(string); ok && strings.TrimSpace(model) != "" {
				listed = append(listed, strings.TrimSpace(model))
			}
		}
	case []string:
		for _, model := range value {
			if strings.TrimSpace(model) != "" {
				listed = append(listed, strings.TrimSpace(model))
			}
		}
	}
	if len(listed) == 0 {
		for _, profile := range platformChannelProfiles(channel) {
			if model, _ := profile["model"].(string); strings.TrimSpace(model) != "" {
				listed = append(listed, strings.TrimSpace(model))
			}
		}
	}
	return listed
}

// PlatformChannelModels lists the channel's models that declare capability.
func PlatformChannelModels(channel map[string]any, capability string) []string {
	listed := PlatformChannelListedModels(channel)
	var models []string
	for _, profile := range platformChannelProfiles(channel) {
		value, _ := profile["capability"].(string)
		model, _ := profile["model"].(string)
		model = strings.TrimSpace(model)
		if strings.EqualFold(strings.TrimSpace(value), capability) && model != "" && slices.Contains(listed, model) && profile["enabled"] != false {
			models = append(models, model)
		}
	}
	return models
}

// CorrectPlatformChannelSelections replaces default selections that point at
// a model the platform channel no longer lists with the first listed model of
// the same capability, or clears them. Selections of other channels, and of
// models still listed, are left alone. Provisioning and every hosted save use
// it, so a browser holding an old choice cannot write a removed model back.
func CorrectPlatformChannelSelections(config map[string]any, channel map[string]any) error {
	id, _ := channel["id"].(string)
	if config == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	prefix := id + "::"
	listed := PlatformChannelListedModels(channel)
	for _, item := range platformSelectionFields {
		current, _ := config[item.field].(string)
		if !strings.HasPrefix(current, prefix) || slices.Contains(listed, strings.TrimSpace(strings.TrimPrefix(current, prefix))) {
			continue
		}
		candidates, err := platformSelectionCandidates(channel, id, item.field, item.capability)
		if err != nil {
			return err
		}
		config[item.field] = ""
		if len(candidates) > 0 {
			config[item.field] = prefix + candidates[0]
		}
	}
	return nil
}

func platformSelectionCandidates(channel map[string]any, id, field, capability string) ([]string, error) {
	models := PlatformChannelModels(channel, capability)
	if field != "mediaReviewModel" && field != "assistantModel" {
		return models, nil
	}
	var resolved AssistantChannel
	body, err := json.Marshal(channel)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &resolved); err != nil {
		return nil, err
	}
	// Selection validity is independent of whether a key is bound.
	resolved.Enabled = true
	if resolved.APIKey == "" {
		resolved.APIKey = "selection-check"
	}
	return slices.DeleteFunc(models, func(candidate string) bool {
		key := id + "::" + candidate
		if field == "assistantModel" {
			// Only a model the assistant can talk to may replace its choice.
			_, reason := ResolveAssistantChannelModel(AssistantConfigSnapshot{Channels: []AssistantChannel{resolved}}, key, nil)
			return reason != ""
		}
		_, reason := ResolveMediaReviewProvider(AssistantConfigSnapshot{MediaReviewModel: key, Channels: []AssistantChannel{resolved}}, nil)
		return reason != "" && reason != AssistantReasonCredentialMissing
	}), nil
}

func platformChannelProfiles(channel map[string]any) []map[string]any {
	var profiles []map[string]any
	switch value := channel["modelProfiles"].(type) {
	case []any:
		for _, raw := range value {
			if profile, ok := raw.(map[string]any); ok {
				profiles = append(profiles, profile)
			}
		}
	case []map[string]any:
		profiles = value
	}
	return profiles
}
