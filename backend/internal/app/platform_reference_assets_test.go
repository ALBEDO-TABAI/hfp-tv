package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
