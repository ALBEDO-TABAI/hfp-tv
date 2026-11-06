package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"infinite-canvas/backend/internal/generation"
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

// The company reference-image service (the old project's
// services/reference-upload-relay) stores an image in the company object
// storage and answers, depending on the requested mode, with a temporary
// public HTTPS link (public-url) or, after registering it in the preset Deyun
// asset group, an asset:// ID (seedance-asset). It authenticates the member's
// own relay key and keeps a content ledger, so re-sending the same image never
// creates a second asset.
const (
	referenceServiceAssetMode   = "seedance-asset"
	referenceServicePublicMode  = "public-url"
	referenceServiceMaxBytes    = 30 << 20
	referenceServiceTimeout     = 4 * time.Minute
	referenceServiceBindingKind = "reference-service:"
	// A cached public link is reused only while the provider still has this
	// long to fetch it.
	referenceLinkReuseMargin = 30 * time.Minute
)

// A task waits this long in total for an asset review; the service itself
// answers "still reviewing" after about three minutes per call.
var (
	referenceAssetReviewBudget = 12 * time.Minute
	referenceAssetReviewPause  = 5 * time.Second
)

var referenceAssetIDPattern = regexp.MustCompile(`^asset://[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)

var referenceServiceImageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

func isYSXXAIReferenceService(service string) bool {
	endpoint, err := url.Parse(strings.TrimSpace(service))
	return err == nil && endpoint.Path == "/v1/ysxxai-reference-images"
}

// registerYSXXAIReferenceAssets gives every library image of a YSXXAI Seedance
// task an approved asset ID before the model is called, as the old Deyun line
// did: Seedance refuses realistic faces sent as links. Registered images are
// reused; a new one is registered and the task waits for the provider's review.
// A refused registration fails the task before anything is billed. Each step
// goes to the task log, so a rejected task shows what the provider received.
func (s *Service) registerYSXXAIReferenceAssets(ctx context.Context, userID string, input *canvasGenerationInput) error {
	service := strings.TrimSpace(input.Config.ReferenceAssetService)
	taskID := taskExecutionID(ctx)
	note := func(index int, message string) {
		if taskID != "" {
			_ = s.log(userID, taskID, "info", fmt.Sprintf("参考图 %d", index+1), message)
		}
	}
	registering := false
	for index := range input.ReferenceImages {
		reference := &input.ReferenceImages[index]
		if strings.HasPrefix(strings.TrimSpace(reference.URL), "asset://") {
			note(index, "已是素材编号，直接使用")
			continue
		}
		resourceID := arkPrivateAssetResourceID(*reference)
		if resourceID == "" {
			note(index, "不是画布或素材库里的图片，无法登记为虚拟人像素材，按原链接提交；含写实人脸会被拒")
			continue
		}
		binding, err := s.repo.ArkPrivateAssetBinding(resourceID, referenceServiceBindingKind+service)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && binding.UserID != userID {
			return errors.New("参考素材不属于当前用户")
		}
		if err == nil && binding.Status == arkPrivateAssetStatusLive && binding.ArkAssetID != "" {
			reference.URL, reference.DataURL = "asset://"+binding.ArkAssetID, ""
			note(index, "使用已登记的虚拟人像素材 "+binding.ArkAssetID)
			continue
		}
		if !registering && taskID != "" {
			_ = s.repo.UpdateTaskProgress(taskID, "登记虚拟人像素材，等待审核", 36)
		}
		registering = true
		note(index, "首次使用，正在登记为虚拟人像素材并等待审核（通常 1～3 分钟）")
		assetURL, err := s.awaitReferenceServiceAsset(ctx, userID, resourceID, service, input.Config.APIKey, func() {
			note(index, "素材仍在审核，继续等待")
		})
		if err != nil {
			return fmt.Errorf("参考图 %d：%w", index+1, err)
		}
		reference.URL, reference.DataURL = assetURL, ""
		note(index, "审核通过，使用素材编号 "+strings.TrimPrefix(assetURL, "asset://"))
	}
	if registering && taskID != "" {
		_ = s.repo.UpdateTaskProgress(taskID, "调用生成模型", 40)
	}
	return nil
}

// awaitReferenceServiceAsset keeps asking while the service reports the review
// is still running; its content ledger turns every repeat into a status query.
func (s *Service) awaitReferenceServiceAsset(ctx context.Context, userID, resourceID, service, apiKey string, waiting func()) (string, error) {
	deadline := time.Now().Add(referenceAssetReviewBudget)
	for {
		assetURL, err := s.ensureReferenceServiceAsset(ctx, userID, resourceID, service, apiKey)
		var reviewing referenceAssetReviewing
		if err == nil || !errors.As(err, &reviewing) || time.Now().After(deadline) {
			return assetURL, err
		}
		waiting()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(referenceAssetReviewPause):
		}
	}
}

// referenceAssetReviewing is the service's "review still running" answer.
type referenceAssetReviewing struct{ message string }

func (e referenceAssetReviewing) Error() string { return e.message }

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
	if _, err := s.Resource(userID, resourceID); err != nil {
		return "", err
	}
	bindingKey := referenceServiceBindingKind + service
	if binding, err := s.repo.ArkPrivateAssetBinding(resourceID, bindingKey); err == nil && binding.Status == arkPrivateAssetStatusLive && binding.ArkAssetID != "" {
		return "asset://" + binding.ArkAssetID, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	contentType, data, err := s.readReferenceServiceImage(userID, resourceID)
	if err != nil {
		return "", err
	}
	reply, err := callReferenceService(ctx, service, apiKey, referenceServiceAssetMode, contentType, data)
	if err != nil {
		return "", err
	}
	assetURL := strings.TrimSpace(reply.URL)
	if !referenceAssetIDPattern.MatchString(assetURL) {
		return "", relayReplyError("参考图服务没有返回可用的素材编号")
	}
	// Caching is an optimization: a concurrent task may have cached it first,
	// and the service returns the same asset for the same image anyway.
	_, _ = s.repo.CreateArkPrivateAssetBinding(&model.ArkPrivateAssetBinding{ID: newID(), UserID: userID, ResourceID: resourceID,
		ProjectName: bindingKey, ArkAssetID: strings.TrimPrefix(assetURL, "asset://"), Status: arkPrivateAssetStatusLive})
	return assetURL, nil
}

type publicReferenceLink struct {
	url       string
	expiresAt time.Time
}

// referenceServicePublicURL is the company image relay: it gives an owned
// image a temporary public HTTPS link for model protocols that only accept
// URLs. Links are reused while they stay valid long enough.
func (s *Service) referenceServicePublicURL(ctx context.Context, userID, resourceID, service, apiKey string) (string, error) {
	cacheKey := service + "\x00" + userID + "\x00" + resourceID
	if cached, ok := s.publicReferenceLinks.Load(cacheKey); ok {
		if link := cached.(publicReferenceLink); time.Until(link.expiresAt) > referenceLinkReuseMargin {
			return link.url, nil
		}
		s.publicReferenceLinks.Delete(cacheKey)
	}
	contentType, data, err := s.readReferenceServiceImage(userID, resourceID)
	if err != nil {
		return "", err
	}
	reply, err := callReferenceService(ctx, service, apiKey, referenceServicePublicMode, contentType, data)
	if err != nil {
		return "", err
	}
	link := strings.TrimSpace(reply.URL)
	if parsed, err := url.Parse(link); err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return "", relayReplyError("参考图服务没有返回可用的 HTTPS 链接")
	}
	if expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(reply.ExpiresAt)); err == nil {
		s.publicReferenceLinks.Store(cacheKey, publicReferenceLink{url: link, expiresAt: expiresAt})
	}
	return link, nil
}

// readReferenceServiceImage reads an owned, ready image the service accepts.
func (s *Service) readReferenceServiceImage(userID, resourceID string) (string, []byte, error) {
	resource, body, err := s.OpenResource(userID, resourceID)
	if err != nil {
		return "", nil, fmt.Errorf("读取参考图失败：%w", err)
	}
	defer body.Close()
	if resource.Status != model.ResourceStatusReady {
		return "", nil, errors.New("参考图尚未上传完成")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resource.MimeType, ";")[0]))
	if !referenceServiceImageTypes[contentType] {
		return "", nil, errors.New("参考图服务只接受 PNG、JPEG 或 WebP 图片")
	}
	data, err := io.ReadAll(io.LimitReader(body, referenceServiceMaxBytes))
	if err != nil {
		return "", nil, fmt.Errorf("读取参考图失败：%w", err)
	}
	if len(data) >= referenceServiceMaxBytes {
		return "", nil, errors.New("参考图必须小于 30MB")
	}
	return contentType, data, nil
}

type referenceServiceReply struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
	Error     string `json:"error"`
	Code      string `json:"code"`
	Stage     string `json:"stage"`
}

// callReferenceService posts one image. Failures of the service itself are
// generation.ReferenceRelayError: the model was not called, and the request ID
// lets an administrator find the call in the service's log.
func callReferenceService(ctx context.Context, service, apiKey, mode, contentType string, data []byte) (referenceServiceReply, error) {
	requestID := newID()
	fail := func(unauthorized bool, message string, args ...any) (referenceServiceReply, error) {
		return referenceServiceReply{}, referenceServiceFailure(requestID, apiKey, unauthorized, false, fmt.Errorf(message, args...))
	}
	if strings.TrimSpace(apiKey) == "" {
		return fail(true, "没有可用的中转站密钥，无法上传参考图")
	}
	callCtx, cancel := context.WithTimeout(ctx, referenceServiceTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(callCtx, http.MethodPost, service, bytes.NewReader(data))
	if err != nil {
		return fail(false, "参考图服务地址无效：%w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-HFP-Reference-Mode", mode)
	request.Header.Set("X-HFP-Request-ID", requestID)
	response, err := OutboundHTTPClient(referenceServiceTimeout).Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return referenceServiceReply{}, ctx.Err() // the task was cancelled
		}
		if callCtx.Err() != nil {
			return fail(false, "参考图服务处理超时；稍后重试会复用同一张图")
		}
		return fail(false, "无法连接参考图服务：%w", err)
	}
	defer response.Body.Close()
	var reply referenceServiceReply
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	_ = json.Unmarshal(raw, &reply)
	switch {
	case response.StatusCode == http.StatusOK:
		return reply, nil
	case response.StatusCode == http.StatusUnauthorized:
		return fail(true, "参考图服务不接受你的中转站密钥，请联系管理员核对")
	case strings.HasSuffix(strings.TrimSpace(reply.Code), "_ASSET_TIMEOUT"):
		return referenceServiceReply{}, referenceServiceFailure(requestID, apiKey, false, false, referenceAssetReviewing{"参考图服务处理失败：" + strings.TrimSpace(reply.Error)})
	case strings.HasSuffix(strings.TrimSpace(reply.Stage), "-asset"):
		// Registration or review failed after the upload: retrying alone will not help.
		message := strings.TrimSpace(reply.Error)
		if message == "" {
			message = fmt.Sprintf("HTTP %d", response.StatusCode)
		}
		return referenceServiceReply{}, referenceServiceFailure(requestID, apiKey, false, true, fmt.Errorf("参考图服务登记素材失败：%s", message))
	case strings.TrimSpace(reply.Error) != "":
		return fail(false, "参考图服务处理失败：%s", strings.TrimSpace(reply.Error))
	default:
		return fail(false, "参考图服务处理失败（HTTP %d）", response.StatusCode)
	}
}

func referenceServiceFailure(requestID, apiKey string, unauthorized, assetStage bool, cause error) error {
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" && strings.Contains(cause.Error(), apiKey) {
		cause = errors.New(strings.ReplaceAll(cause.Error(), apiKey, "[REDACTED]"))
	}
	return generation.ReferenceRelayError{RequestID: requestID, Unauthorized: unauthorized, AssetStage: assetStage, Cause: cause}
}

// relayReplyError reports a successful call whose answer cannot be used.
func relayReplyError(message string) error {
	return generation.ReferenceRelayError{Cause: errors.New(message)}
}

// platformVirtualPortraitService is where a member's explicit "register
// virtual portrait" goes: the YSXXAI asset endpoint whose bindings Seedance
// generation reuses. Other services are skipped even when listed first,
// otherwise the image would land in a group generation never reads.
func (s *Service) platformVirtualPortraitService() (service, apiKey string, ok bool) {
	for _, credential := range s.platformChannelCredentials() {
		if credential.apiKey == "" {
			continue
		}
		for _, service := range credential.referenceServices {
			if isYSXXAIReferenceService(service) {
				return service, credential.apiKey, true
			}
		}
	}
	return "", "", false
}

// platformReferenceService is the reference-image service of any company relay
// model with the member's company key, used as the workspace's image relay.
func (s *Service) platformReferenceService() (service, apiKey string, ok bool) {
	for _, credential := range s.platformChannelCredentials() {
		if service := credential.anyReferenceService(); service != "" && credential.apiKey != "" {
			return service, credential.apiKey, true
		}
	}
	return "", "", false
}
