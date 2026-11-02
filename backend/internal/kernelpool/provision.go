package kernelpool

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"infinite-canvas/backend/internal/referencestorage"
	"infinite-canvas/backend/internal/workspace"
)

// RelayChannelID is the fixed ID of the company relay channel the team server
// writes into every workspace's model configuration.
const RelayChannelID = "company-relay"

// LoadRelayTemplate reads the deployment's channel definition (a ModelChannel
// JSON object without apiKey). The admin exports it once from a configured
// workspace; the server fills in each member's own key.
func LoadRelayTemplate(path string) (map[string]any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取公司中转站渠道定义失败：%w", err)
	}
	var template map[string]any
	if err := json.Unmarshal(body, &template); err != nil {
		return nil, fmt.Errorf("公司中转站渠道定义不是 JSON 对象：%w", err)
	}
	if baseURL, _ := template["baseUrl"].(string); strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("公司中转站渠道定义缺少 baseUrl")
	}
	for _, secret := range []string{"apiKey", "secretKey", "credentialRef"} {
		if value, _ := template[secret].(string); strings.TrimSpace(value) != "" {
			return nil, fmt.Errorf("公司中转站渠道定义不能包含 %s，密钥按成员单独绑定", secret)
		}
	}
	if err := checkModelBaseURLs(template); err != nil {
		return nil, err
	}
	return template, nil
}

// checkModelBaseURLs allows a model its own endpoint on the relay (for example
// Seedance under /doubao/api/v3) but never another host: members' keys are
// sent to whatever URL the model resolves to.
func checkModelBaseURLs(template map[string]any) error {
	base, err := url.Parse(strings.TrimSpace(fmt.Sprint(template["baseUrl"])))
	if err != nil || base.Host == "" {
		return errors.New("公司中转站渠道定义的 baseUrl 无效")
	}
	profiles, _ := template["modelProfiles"].([]any)
	for _, raw := range profiles {
		profile, _ := raw.(map[string]any)
		modelBase, _ := profile["baseUrl"].(string)
		if strings.TrimSpace(modelBase) == "" {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(modelBase))
		if err != nil || !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
			return fmt.Errorf("公司中转站渠道里模型 %v 的 baseUrl 必须与渠道 baseUrl 同协议、同主机和端口", profile["model"])
		}
	}
	for _, service := range RelayReferenceServices(template) {
		parsed, err := url.Parse(service)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("公司中转站渠道里的 referenceAssetService 必须是 http:// 或 https:// 开头的完整地址：%s", service)
		}
	}
	return nil
}

// RelayReferenceServices lists the reference-image service URLs named by the
// template's model profiles, deduplicated.
func RelayReferenceServices(template map[string]any) []string {
	var services []string
	seen := map[string]bool{}
	profiles, _ := template["modelProfiles"].([]any)
	for _, raw := range profiles {
		profile, _ := raw.(map[string]any)
		service, _ := profile["referenceAssetService"].(string)
		if service = strings.TrimSpace(service); service != "" && !seen[service] {
			seen[service] = true
			services = append(services, service)
		}
	}
	return services
}

func cloneJSONMap(value map[string]any) (map[string]any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		return nil, err
	}
	if clone == nil {
		clone = map[string]any{}
	}
	return clone, nil
}

// applyRelayChannel upserts the company relay channel with this member's key.
// Without a key the channel stays visible but disabled ("等待管理员绑定").
func applyRelayChannel(dataDir string, template map[string]any, apiKey string) error {
	if template == nil {
		return nil
	}
	store, err := workspace.NewProviderConfig(dataDir)
	if err != nil {
		return err
	}
	effective, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		return err
	}
	config, err := cloneJSONMap(effective.Config)
	if err != nil {
		return err
	}
	channel, err := cloneJSONMap(template)
	if err != nil {
		return err
	}
	channel["id"] = RelayChannelID
	if name, _ := channel["name"].(string); strings.TrimSpace(name) == "" {
		channel["name"] = "公司中转站"
	}
	channel["apiKey"] = apiKey
	channel["enabled"] = apiKey != ""
	channel["pinned"] = true
	channels, _ := config["channels"].([]any)
	next := make([]any, 0, len(channels)+1)
	next = append(next, channel)
	for _, raw := range channels {
		if existing, ok := raw.(map[string]any); ok && existing["id"] == RelayChannelID {
			continue
		}
		next = append(next, raw)
	}
	config["channels"] = next
	// The assistant reads the stored text model; the page only displays a
	// fallback. Give a member who never picked one the relay's text model.
	if current, _ := config["textModel"].(string); strings.TrimSpace(current) == "" {
		if model := firstRelayModel(template, "text"); model != "" {
			config["textModel"] = RelayChannelID + "::" + model
		}
	}
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return store.SaveLocalModelConfig(body)
}

func applyReferenceStorage(dataDir string, config *referencestorage.Config) error {
	if config == nil {
		return nil
	}
	_, err := referencestorage.Save(dataDir, *config)
	return err
}

func firstRelayModel(template map[string]any, capability string) string {
	profiles, _ := template["modelProfiles"].([]any)
	for _, raw := range profiles {
		profile, _ := raw.(map[string]any)
		if value, _ := profile["capability"].(string); strings.EqualFold(strings.TrimSpace(value), capability) {
			if model, _ := profile["model"].(string); strings.TrimSpace(model) != "" {
				return strings.TrimSpace(model)
			}
		}
	}
	return ""
}
