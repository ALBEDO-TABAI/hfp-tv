package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

// The company reference-image service (the old project's
// services/reference-upload-relay) stores an image in the company object
// storage, registers it in the preset Deyun asset group and answers with an
// asset:// ID. It authenticates the member's own relay key and keeps a content
// ledger, so re-sending the same image never creates a second asset.
const (
	referenceServiceMode        = "seedance-asset"
	referenceServiceMaxBytes    = 30 << 20
	referenceServiceTimeout     = 4 * time.Minute
	referenceServiceBindingKind = "reference-service:"
)

var referenceAssetIDPattern = regexp.MustCompile(`^asset://[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)

var referenceServiceImageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// registerReferenceServiceAssets replaces the local reference images of a
// video task with asset:// IDs before the request is built. Images that are
// already assets are kept.
func (s *Service) registerReferenceServiceAssets(ctx context.Context, userID string, input *canvasGenerationInput) error {
	service := strings.TrimSpace(input.Config.ReferenceAssetService)
	if service == "" || input.Mode != "video" || !isArkPrivateAssetVideoConfig(input.Config) || len(input.ReferenceImages) == 0 {
		return nil
	}
	if taskID := taskExecutionID(ctx); taskID != "" {
		_ = s.repo.UpdateTaskProgress(taskID, "登记参考图素材", 36)
	}
	for index := range input.ReferenceImages {
		reference := &input.ReferenceImages[index]
		if strings.HasPrefix(strings.TrimSpace(reference.URL), "asset://") {
			continue
		}
		resourceID := arkPrivateAssetResourceID(*reference)
		if resourceID == "" {
			return fmt.Errorf("参考图 %d 不是素材库里的图片，无法登记为 Seedance 素材，请先保存到画布或素材库", index+1)
		}
		assetURL, err := s.ensureReferenceServiceAsset(ctx, userID, resourceID, service, input.Config.APIKey)
		if err != nil {
			return fmt.Errorf("参考图 %d：%w", index+1, err)
		}
		reference.URL = assetURL
		reference.DataURL = ""
	}
	if taskID := taskExecutionID(ctx); taskID != "" {
		_ = s.repo.UpdateTaskProgress(taskID, "调用生成模型", 40)
	}
	return nil
}

// ensureReferenceServiceAsset returns the cached asset for a resource or
// registers it once. Only successful registrations are cached; a failed or
// uncertain call is safe to repeat because the service deduplicates by content.
func (s *Service) ensureReferenceServiceAsset(ctx context.Context, userID, resourceID, service, apiKey string) (string, error) {
	bindingKey := referenceServiceBindingKind + service
	if binding, err := s.repo.ArkPrivateAssetBinding(resourceID, bindingKey); err == nil && binding.Status == arkPrivateAssetStatusLive && binding.ArkAssetID != "" {
		return "asset://" + binding.ArkAssetID, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("没有可用的中转站密钥，无法登记参考图")
	}
	resource, body, err := s.OpenResource(userID, resourceID)
	if err != nil {
		return "", fmt.Errorf("读取参考图失败：%w", err)
	}
	defer body.Close()
	if resource.Status != model.ResourceStatusReady {
		return "", errors.New("参考图尚未上传完成")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resource.MimeType, ";")[0]))
	if !referenceServiceImageTypes[contentType] {
		return "", errors.New("Seedance 参考图只支持 PNG、JPEG 或 WebP")
	}
	data, err := io.ReadAll(io.LimitReader(body, referenceServiceMaxBytes))
	if err != nil {
		return "", fmt.Errorf("读取参考图失败：%w", err)
	}
	if len(data) >= referenceServiceMaxBytes {
		return "", errors.New("Seedance 参考图必须小于 30MB")
	}
	assetURL, err := callReferenceService(ctx, service, apiKey, contentType, data)
	if err != nil {
		return "", err
	}
	// Caching is an optimization: a concurrent task may have cached it first,
	// and the service returns the same asset for the same image anyway.
	_, _ = s.repo.CreateArkPrivateAssetBinding(&model.ArkPrivateAssetBinding{ID: newID(), UserID: userID, ResourceID: resourceID,
		ProjectName: bindingKey, ArkAssetID: strings.TrimPrefix(assetURL, "asset://"), Status: arkPrivateAssetStatusLive})
	return assetURL, nil
}

func callReferenceService(ctx context.Context, service, apiKey, contentType string, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, referenceServiceTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("参考图服务地址无效：%w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-HFP-Reference-Mode", referenceServiceMode)
	request.Header.Set("X-HFP-Request-ID", newID())
	response, err := OutboundHTTPClient(referenceServiceTimeout).Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("参考图素材登记超时；素材可能仍在处理，稍后重试会复用同一素材")
		}
		return "", fmt.Errorf("无法连接参考图服务：%w", err)
	}
	defer response.Body.Close()
	var payload struct {
		URL   string `json:"url"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	_ = json.Unmarshal(raw, &payload)
	if response.StatusCode == http.StatusOK && referenceAssetIDPattern.MatchString(strings.TrimSpace(payload.URL)) {
		return strings.TrimSpace(payload.URL), nil
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return "", errors.New("参考图服务不接受你的中转站密钥，请联系管理员核对")
	case response.StatusCode == http.StatusOK:
		return "", errors.New("参考图服务没有返回可用的素材编号")
	case strings.TrimSpace(payload.Error) != "":
		return "", fmt.Errorf("参考图素材登记失败：%s", strings.TrimSpace(payload.Error))
	default:
		return "", fmt.Errorf("参考图素材登记失败（HTTP %d）", response.StatusCode)
	}
}

// platformReferenceService is the reference-image service of any company relay
// model, used for registering a library image ahead of generation.
func (s *Service) platformReferenceService() (service, apiKey string, ok bool) {
	for _, credential := range s.platformChannelCredentials() {
		for _, candidate := range credential.modelReferenceServices {
			if candidate != "" && credential.apiKey != "" {
				return candidate, credential.apiKey, true
			}
		}
	}
	return "", "", false
}
