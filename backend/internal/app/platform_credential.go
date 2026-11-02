package app

import (
	"encoding/json"
	"slices"
	"strings"
)

// SetPlatformChannels names the model channels a team server writes into this
// workspace. Their keys never reach the browser: the config is returned
// redacted and requests that name the channel get the stored key here.
func (s *Service) SetPlatformChannels(ids []string) {
	if s == nil {
		return
	}
	s.platformChannels = append([]string(nil), ids...)
}

func (s *Service) IsPlatformChannel(channelID string) bool {
	channelID = strings.TrimSpace(channelID)
	return s != nil && channelID != "" && slices.Contains(s.platformChannels, channelID)
}

type platformChannelCredential struct {
	apiKey  string
	baseURL string
	// modelBaseURLs are per-model endpoints on the same origin, e.g. a relay
	// that serves Seedance under /doubao/api/v3 next to its OpenAI /v1.
	modelBaseURLs map[string]string
	// modelReferenceServices register a model's reference images as assets.
	modelReferenceServices map[string]string
}

func platformModelKey(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), "models/")
}

func (c platformChannelCredential) referenceServiceFor(model string) string {
	return c.modelReferenceServices[platformModelKey(model)]
}

// baseURLFor returns the model's own endpoint when it shares the channel's
// origin, otherwise the channel base URL.
func (c platformChannelCredential) baseURLFor(model string) string {
	if base := c.modelBaseURLs[platformModelKey(model)]; base != "" && sameCredentialOrigin(base, c.baseURL) {
		return base
	}
	return c.baseURL
}

func (s *Service) platformChannelCredentials() []platformChannelCredential {
	return s.platformChannelCredentialsMatching(func(string) bool { return true })
}

func (s *Service) platformChannelCredential(channelID string) (platformChannelCredential, bool) {
	found := s.platformChannelCredentialsMatching(func(id string) bool { return id == channelID })
	if len(found) == 0 {
		return platformChannelCredential{}, false
	}
	return found[0], true
}

func (s *Service) platformChannelCredentialsMatching(match func(id string) bool) []platformChannelCredential {
	if s == nil || len(s.platformChannels) == 0 {
		return nil
	}
	body, err := s.ReadLocalModelConfig()
	if err != nil || len(body) == 0 {
		return nil
	}
	var config struct {
		Channels []struct {
			ID            string `json:"id"`
			APIKey        string `json:"apiKey"`
			BaseURL       string `json:"baseUrl"`
			ModelProfiles []struct {
				Model                 string `json:"model"`
				BaseURL               string `json:"baseUrl"`
				ReferenceAssetService string `json:"referenceAssetService"`
			} `json:"modelProfiles"`
		} `json:"channels"`
	}
	if json.Unmarshal(body, &config) != nil {
		return nil
	}
	var out []platformChannelCredential
	for _, channel := range config.Channels {
		if s.IsPlatformChannel(channel.ID) && match(channel.ID) {
			credential := platformChannelCredential{apiKey: strings.TrimSpace(channel.APIKey), baseURL: strings.TrimSpace(channel.BaseURL),
				modelBaseURLs: map[string]string{}, modelReferenceServices: map[string]string{}}
			for _, profile := range channel.ModelProfiles {
				model := platformModelKey(profile.Model)
				if base := strings.TrimSpace(profile.BaseURL); model != "" && base != "" {
					credential.modelBaseURLs[model] = base
				}
				if service := strings.TrimSpace(profile.ReferenceAssetService); model != "" && service != "" {
					credential.modelReferenceServices[model] = service
				}
			}
			out = append(out, credential)
		}
	}
	return out
}

// resolvePlatformChannelSecrets completes a task that names a team-server
// channel with the stored key. The stored base URL (or the model's own
// same-origin endpoint) always wins, so the key is only ever sent to the
// origin the administrator configured. Like the managed
// BeefAPI channel, the channel ID is then dropped: admission treats any task
// with a channel ID as a system-catalog channel.
func (s *Service) resolvePlatformChannelSecrets(input map[string]any) (map[string]any, error) {
	config, _ := input["config"].(map[string]any)
	if config == nil {
		return input, nil
	}
	channelID := strings.TrimSpace(stringValue(config["channelId"]))
	if !s.IsPlatformChannel(channelID) {
		// Members' own channels never reach the reference-image service.
		delete(config, "referenceAssetService")
		return input, nil
	}
	credential, found := s.platformChannelCredential(channelID)
	if !found {
		return nil, BadAuthRequest("公司模型服务暂不可用，请联系管理员")
	}
	if credential.apiKey == "" {
		return nil, BadAuthRequest("管理员还没有为你绑定公司模型服务的密钥")
	}
	next := make(map[string]any, len(config)+2)
	for key, value := range config {
		if key != "channelId" {
			next[key] = value
		}
	}
	next["apiKey"] = credential.apiKey
	next["baseUrl"] = credential.baseURLFor(stringValue(config["model"]))
	next["secretKey"] = ""
	if service := credential.referenceServiceFor(stringValue(config["model"])); service != "" {
		next["referenceAssetService"] = service
	} else {
		delete(next, "referenceAssetService")
	}
	input["config"] = next
	return input, nil
}

// platformRelayKey returns the stored key for a relay request that carries no
// key of its own and targets a team-server channel's origin.
func (s *Service) platformRelayKey(targetURL string) string {
	for _, credential := range s.platformChannelCredentials() {
		if credential.apiKey != "" && sameCredentialOrigin(targetURL, credential.baseURL) {
			return credential.apiKey
		}
	}
	return ""
}
