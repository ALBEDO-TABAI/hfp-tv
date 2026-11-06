package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/generation"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestSeedanceReferenceImagesAreRegisteredThroughTheCompanyService(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	var calls atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer sk-member-relay" || r.Header.Get("X-HFP-Reference-Mode") != "seedance-asset" ||
			r.Header.Get("Content-Type") != "image/png" || r.Header.Get("X-HFP-Request-ID") == "" || !bytes.Equal(body, png) {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"url":"asset://asset-20261102-abc12","requestId":"r"}`))
	}))
	defer service.Close()

	s, _, _, _ := creationTestService(t)
	resource, err := s.UploadLocalResourceFile("user", "ref.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	newInput := func() *generation.Input {
		return &generation.Input{Mode: "video", Config: generation.Config{InterfaceType: "volcengine-ark-video", APIKey: "sk-member-relay", ReferenceAssetService: service.URL + "/v1/reference-images"},
			ReferenceImages: []generation.Media{{StorageKey: "resource:" + resource.ID, DataURL: "data:image/png;base64," + onePixelPNG}, {URL: "asset://asset-kept"}}}
	}
	input := newInput()
	if err := s.prepareArkPrivateAssetReferences(context.Background(), "user", input); err != nil {
		t.Fatal(err)
	}
	if input.ReferenceImages[0].URL != "asset://asset-20261102-abc12" || input.ReferenceImages[0].DataURL != "" || input.ReferenceImages[1].URL != "asset://asset-kept" {
		t.Fatalf("references = %+v", input.ReferenceImages)
	}
	// A second task reuses the registered asset without calling the service.
	input = newInput()
	if err := s.prepareArkPrivateAssetReferences(context.Background(), "user", input); err != nil || input.ReferenceImages[0].URL != "asset://asset-20261102-abc12" || calls.Load() != 1 {
		t.Fatalf("cached registration: %v %+v calls=%d", err, input.ReferenceImages, calls.Load())
	}
	// Images that are not library resources cannot be registered.
	inline := &generation.Input{Mode: "video", Config: newInput().Config, ReferenceImages: []generation.Media{{DataURL: "data:image/png;base64," + onePixelPNG}}}
	if err := s.prepareArkPrivateAssetReferences(context.Background(), "user", inline); err == nil || !strings.Contains(err.Error(), "素材库") {
		t.Fatalf("inline reference = %v", err)
	}
}

func TestYSXXAIRegistersLibraryImagesBeforeGeneration(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	var calls atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-HFP-Reference-Mode") != "seedance-asset" {
			t.Errorf("mode = %q", r.Header.Get("X-HFP-Reference-Mode"))
		}
		_, _ = w.Write([]byte(`{"url":"asset://asset-virtual","requestId":"r"}`))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	registered, _ := s.UploadLocalResourceFile("user", "registered.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	fresh, _ := s.UploadLocalResourceFile("user", "fresh.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	serviceURL := service.URL + "/v1/ysxxai-reference-images"
	if _, err := s.ensureReferenceServiceAsset(context.Background(), "user", registered.ID, serviceURL, "sk-member-relay"); err != nil {
		t.Fatal(err)
	}
	newInput := func() *generation.Input {
		return &generation.Input{Mode: "video", Config: generation.Config{InterfaceType: "video-generations-json", Model: "doubao-seedance-2-0-260128-2-720P", ReferenceAssetService: serviceURL, APIKey: "sk-member-relay"},
			ReferenceImages: []generation.Media{{StorageKey: "resource:" + registered.ID, URL: "https://example.com/registered.png"}, {StorageKey: "resource:" + fresh.ID, URL: "https://example.com/fresh.png"}, {URL: "https://example.com/linked.png"}}}
	}
	input := newInput()
	if err := s.prepareArkPrivateAssetReferences(generation.WithTaskExecutionID(context.Background(), "task-1"), "user", input); err != nil {
		t.Fatal(err)
	}
	if input.ReferenceImages[0].URL != "asset://asset-virtual" || input.ReferenceImages[1].URL != "asset://asset-virtual" || input.ReferenceImages[2].URL != "https://example.com/linked.png" || calls.Load() != 2 {
		t.Fatalf("references=%+v calls=%d", input.ReferenceImages, calls.Load())
	}
	logs, err := s.repo.TaskLogs("user", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, log := range logs {
		lines = append(lines, log.Message+"："+log.Payload)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"参考图 1：使用已登记的虚拟人像素材 asset-virtual", "参考图 2：首次使用，正在登记", "参考图 2：审核通过，使用素材编号 asset-virtual", "参考图 3：不是画布或素材库里的图片"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("task log lacks %q:\n%s", want, joined)
		}
	}
	// The second run reuses both registrations without calling the service.
	if err := s.prepareArkPrivateAssetReferences(context.Background(), "user", newInput()); err != nil || calls.Load() != 2 {
		t.Fatalf("second run err=%v calls=%d", err, calls.Load())
	}
	if err := s.prepareArkPrivateAssetReferences(context.Background(), "another-user", newInput()); err == nil {
		t.Fatal("another user reused the registered reference")
	}
}

func TestYSXXAIWaitsWhileTheReviewIsStillRunning(t *testing.T) {
	previous := referenceAssetReviewPause
	referenceAssetReviewPause = time.Millisecond
	defer func() { referenceAssetReviewPause = previous }()
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	var calls atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"YSXXAI素材还在审核中","code":"YSXXAI_ASSET_TIMEOUT","stage":"ysxxai-asset"}`))
			return
		}
		_, _ = w.Write([]byte(`{"url":"asset://asset-reviewed"}`))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	image, _ := s.UploadLocalResourceFile("user", "portrait.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	input := &generation.Input{Mode: "video", Config: generation.Config{InterfaceType: "video-generations-json", Model: "doubao-seedance-2-5-260628-2-720P", ReferenceAssetService: service.URL + "/v1/ysxxai-reference-images", APIKey: "sk-member-relay"},
		ReferenceImages: []generation.Media{{StorageKey: "resource:" + image.ID}}}
	if err := s.prepareArkPrivateAssetReferences(generation.WithTaskExecutionID(context.Background(), "task-wait"), "user", input); err != nil {
		t.Fatal(err)
	}
	if input.ReferenceImages[0].URL != "asset://asset-reviewed" || calls.Load() != 3 {
		t.Fatalf("reference=%+v calls=%d", input.ReferenceImages[0], calls.Load())
	}
}

func TestYSXXAIRefusedRegistrationFailsBeforeTheModel(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"素材审核未通过","code":"YSXXAI_ASSET_FAILED","stage":"ysxxai-asset"}`))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	image, _ := s.UploadLocalResourceFile("user", "portrait.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	input := &generation.Input{Mode: "video", Config: generation.Config{InterfaceType: "video-generations-json", Model: "doubao-seedance-2-0-260128-2-720P", ReferenceAssetService: service.URL + "/v1/ysxxai-reference-images", APIKey: "sk-member-relay"},
		ReferenceImages: []generation.Media{{StorageKey: "resource:" + image.ID}}}
	err := s.prepareArkPrivateAssetReferences(context.Background(), "user", input)
	failure := generation.ClassifyError(err)
	if err == nil || !strings.Contains(err.Error(), "素材审核未通过") || failure.ProviderCode != "reference_asset_failed" || !strings.Contains(failure.UserMessage(), "没能登记为虚拟人像素材") {
		t.Fatalf("err=%v failure=%+v", err, failure)
	}
}

func TestYSXXAIModelWithoutTheAssetServiceSaysSo(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	input := &generation.Input{Mode: "video", Config: generation.Config{InterfaceType: "video-generations-json", Model: "doubao-seedance-2-0-260128-2-720P", ReferenceAssetService: "http://10.0.0.1:8787/v1/reference-images"},
		ReferenceImages: []generation.Media{{StorageKey: "resource:any", URL: "https://example.com/a.png"}}}
	if err := s.prepareArkPrivateAssetReferences(generation.WithTaskExecutionID(context.Background(), "task-misconfigured"), "user", input); err != nil {
		t.Fatal(err)
	}
	logs, _ := s.repo.TaskLogs("user", "task-misconfigured")
	if len(logs) != 1 || !strings.Contains(logs[0].Payload, "没有配置虚拟人像素材服务") || input.ReferenceImages[0].URL != "https://example.com/a.png" {
		t.Fatalf("logs=%+v reference=%+v", logs, input.ReferenceImages[0])
	}
}

func TestReferenceServiceFailuresAreExplained(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	status, body := http.StatusUnauthorized, `{"error":"无效或已失效的应用凭据"}`
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	resource, err := s.UploadLocalResourceFile("user", "ref.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	register := func() error {
		_, err := s.ensureReferenceServiceAsset(context.Background(), "user", resource.ID, service.URL, "sk-member-relay")
		return err
	}
	if err := register(); err == nil || !strings.Contains(err.Error(), "密钥") {
		t.Fatalf("401 = %v", err)
	}
	status, body = http.StatusBadGateway, `{"error":"参考图登记到德云素材服务失败","code":"DEYUN_ASSET_REGISTRATION_FAILED"}`
	if err := register(); err == nil || !strings.Contains(err.Error(), "参考图登记到德云素材服务失败") {
		t.Fatalf("502 = %v", err)
	}
	status, body = http.StatusOK, `{"url":"https://example.com/not-an-asset.png"}`
	if err := register(); err == nil {
		t.Fatal("a public URL was accepted as an asset ID")
	}
}

func TestReferenceServiceComesOnlyFromTheCompanyChannel(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","apiKey":"sk-member-relay","pinned":true,
		"modelProfiles":[{"model":"doubao-seedance-2-0-260128","protocol":"volcengine-ark-video","baseUrl":"http://10.10.10.45:3000/doubao/api/v3","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	s.SetPlatformChannels([]string{"company-relay"})
	resolved, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "company-relay", "model": "doubao-seedance-2-0-260128", "referenceAssetService": "http://attacker.example/upload"}})
	if err != nil || resolved["config"].(map[string]any)["referenceAssetService"] != "http://10.10.10.45:8787/v1/reference-images" {
		t.Fatalf("platform model service = %v %v", resolved, err)
	}
	own, err := s.resolvePlatformChannelSecrets(map[string]any{"config": map[string]any{"channelId": "", "model": "x", "referenceAssetService": "http://attacker.example/upload"}})
	if _, kept := own["config"].(map[string]any)["referenceAssetService"]; err != nil || kept {
		t.Fatalf("member channel kept a client-supplied service: %v %v", own, err)
	}
	if service, key, ok := s.platformReferenceService(); !ok || service != "http://10.10.10.45:8787/v1/reference-images" || key != "sk-member-relay" {
		t.Fatalf("library pre-registration service = %q %q %v", service, key, ok)
	}
}

// The company image relay gives a library image a temporary public link for
// protocols that accept only URLs, using the member's company key.
func TestCompanyImageRelayActsAsReferenceStorage(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	var calls atomic.Int32
	link := "https://cdn.example.com/hfp-reference/20261103/a.png?sig=1"
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer sk-member-relay" || r.Header.Get("X-HFP-Reference-Mode") != "public-url" {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"url":"` + link + `","expiresAt":"` + time.Now().Add(6*time.Hour).UTC().Format(time.RFC3339) + `"}`))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	resource, err := s.UploadLocalResourceFile("user", "ref.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	storage := appReferenceStoragePort{service: s}
	if enabled, err := storage.Enabled("image"); err != nil || enabled {
		t.Fatalf("relay enabled without a company service = %v %v", enabled, err)
	}
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","apiKey":"sk-member-relay","pinned":true,
		"modelProfiles":[{"model":"doubao-seedance-2-0-260128","protocol":"volcengine-ark-video","referenceAssetService":"` + service.URL + `/v1/reference-images"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	s.SetPlatformChannels([]string{"company-relay"})
	for kind, want := range map[string]bool{"image": true, "video": false, "audio": false} {
		if enabled, err := storage.Enabled(kind); err != nil || enabled != want {
			t.Fatalf("Enabled(%s) = %v %v", kind, enabled, err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		got, err := storage.Upload(context.Background(), "user", resource.ID)
		if err != nil || got != link {
			t.Fatalf("upload %d = %q %v", attempt, got, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("a still-valid link was uploaded again: %d calls", calls.Load())
	}
}

func TestReferenceServiceFailuresAreNotSentFailures(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString(onePixelPNG)
	status, body := http.StatusUnauthorized, `{"error":"无效或已失效的应用凭据"}`
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer service.Close()
	s, _, _, _ := creationTestService(t)
	resource, err := s.UploadLocalResourceFile("user", "ref.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ensureReferenceServiceAsset(context.Background(), "user", resource.ID, service.URL, "sk-member-relay")
	failure := generation.ClassifyError(fmt.Errorf("参考图 1：%w", err))
	if failure.ProviderCode != "reference_relay_unauthorized" || failure.RequestID == "" || failure.BlocksManualRetry() || !failure.BlocksAutomaticRetry() {
		t.Fatalf("401 failure = %+v", failure)
	}
	// The persisted message is recognized again as a not-sent failure.
	if again := generation.ClassifyText(failure.UserMessage()); again.ProviderCode != "reference_relay_unauthorized" || again.BlocksManualRetry() {
		t.Fatalf("persisted 401 failure = %+v (%s)", again, failure.UserMessage())
	}
	service.Close()
	_, err = s.referenceServicePublicURL(context.Background(), "user", resource.ID, service.URL, "sk-member-relay")
	if failure := generation.ClassifyError(err); failure.ProviderCode != "reference_relay_failed" || failure.BlocksManualRetry() {
		t.Fatalf("unreachable service failure = %+v", failure)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.referenceServicePublicURL(ctx, "user", resource.ID, service.URL, "sk-member-relay"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled task = %v", err)
	}
}

// A member's own Ark video channel on the company relay origin registers its
// references through the company service too; other origins never do.
func TestMemberSeedanceOnTheCompanyRelayUsesTheCompanyService(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	if err := s.SaveLocalModelConfig([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","apiKey":"sk-member-relay","pinned":true,
		"modelProfiles":[{"model":"doubao-seedance-2-0-260128","protocol":"volcengine-ark-video","referenceAssetService":"http://10.10.10.45:8787/v1/reference-images"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	s.SetPlatformChannels([]string{"company-relay"})
	resolve := func(config map[string]any) any {
		resolved, err := s.resolvePlatformChannelSecrets(map[string]any{"config": config})
		if err != nil {
			t.Fatal(err)
		}
		return resolved["config"].(map[string]any)["referenceAssetService"]
	}
	if got := resolve(map[string]any{"model": "doubao-seedance-2-0-fast-260128", "interfaceType": "volcengine-ark-video", "baseUrl": "http://10.10.10.45:3000/doubao/api/v3", "referenceAssetService": "http://attacker.example/upload"}); got != "http://10.10.10.45:8787/v1/reference-images" {
		t.Fatalf("own channel on the relay = %v", got)
	}
	for _, config := range []map[string]any{
		{"model": "doubao-seedance-2-0-260128", "interfaceType": "volcengine-ark-video", "baseUrl": "https://ark.cn-beijing.volces.com/api/v3", "referenceAssetService": "http://attacker.example/upload"},
		{"model": "doubao-seedance-2-0-260128", "interfaceType": "newapi-video", "baseUrl": "http://10.10.10.45:3000/v1"},
	} {
		if got := resolve(config); got != nil {
			t.Fatalf("%v got a reference service: %v", config, got)
		}
	}
}
