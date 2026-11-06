package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"infinite-canvas/backend/internal/app"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompanyReferenceAssetRouteRegistersOwnedImageOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/ysxxai-reference-images" || r.Header.Get("X-HFP-Reference-Mode") != "seedance-asset" || r.Header.Get("Authorization") != "Bearer member-key" {
			t.Errorf("unexpected relay request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"url":"asset://asset-virtual"}`))
	}))
	defer relay.Close()
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "workspace.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateLocalSchema(db); err != nil {
		t.Fatal(err)
	}
	svc := app.NewLocal(repository.New(db), dir)
	owner, err := svc.LocalWorkspaceOwner()
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:3000/v1","apiKey":"member-key","modelProfiles":[{"model":"doubao-seedance-2-0-260128-2-720P","protocol":"video-generations-json","referenceAssetService":%q}]}]}`, relay.URL+"/v1/ysxxai-reference-images")
	if err := svc.SaveLocalModelConfig([]byte(config)); err != nil {
		t.Fatal(err)
	}
	svc.SetPlatformChannels([]string{"company-relay"})
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	resource, err := svc.UploadLocalResourceFile(owner.ID, "virtual.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	foreign := model.Resource{ID: "foreign", UserID: "someone-else", Kind: "image", Status: model.ResourceStatusReady}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterCompanyReferenceAssetRoutes(router.Group("/api"), svc, svc)
	for _, id := range []string{resource.ID, resource.ID} {
		result := httptest.NewRecorder()
		router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/api/resources/"+id+"/ark-private-asset", nil))
		if result.Code != http.StatusOK || !bytes.Contains(result.Body.Bytes(), []byte(`"status":"active"`)) {
			t.Fatalf("response=%d %s", result.Code, result.Body.String())
		}
	}
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/api/resources/foreign/ark-private-asset", nil))
	if result.Code == http.StatusOK || calls.Load() != 1 {
		t.Fatalf("foreign response=%d, relay calls=%d", result.Code, calls.Load())
	}
}

func TestCompanyReferenceAssetRouteUsesTheVirtualPortraitService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var paths []string
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"url":"asset://asset-virtual"}`))
	}))
	defer relay.Close()
	for _, tc := range []struct {
		name     string
		profiles string
		wantPath string
	}{
		// The older Deyun endpoint is listed first, as a leftover model would be.
		{"ysxxai after an older service", fmt.Sprintf(`{"model":"doubao-seedance-2-0-260128","protocol":"volcengine-ark-video","referenceAssetService":%q},{"model":"doubao-seedance-2-0-260128-2-720P","protocol":"video-generations-json","referenceAssetService":%q}`, relay.URL+"/v1/reference-images", relay.URL+"/v1/ysxxai-reference-images"), "/v1/ysxxai-reference-images"},
		{"only the older service", fmt.Sprintf(`{"model":"doubao-seedance-2-0-260128","protocol":"volcengine-ark-video","referenceAssetService":%q}`, relay.URL+"/v1/reference-images"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths = nil
			dir := t.TempDir()
			db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "workspace.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateLocalSchema(db); err != nil {
				t.Fatal(err)
			}
			svc := app.NewLocal(repository.New(db), dir)
			owner, err := svc.LocalWorkspaceOwner()
			if err != nil {
				t.Fatal(err)
			}
			config := `{"channels":[{"id":"company-relay","baseUrl":"http://127.0.0.1:3000/v1","apiKey":"member-key","modelProfiles":[` + tc.profiles + `]}]}`
			if err := svc.SaveLocalModelConfig([]byte(config)); err != nil {
				t.Fatal(err)
			}
			svc.SetPlatformChannels([]string{"company-relay"})
			png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
			resource, err := svc.UploadLocalResourceFile(owner.ID, "virtual.png", int64(len(png)), "image", 1, 1, 0, bytes.NewReader(png))
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			RegisterCompanyReferenceAssetRoutes(router.Group("/api"), svc, svc)
			result := httptest.NewRecorder()
			router.ServeHTTP(result, httptest.NewRequest(http.MethodPost, "/api/resources/"+resource.ID+"/ark-private-asset", nil))
			if tc.wantPath == "" {
				if result.Code != http.StatusBadRequest || !bytes.Contains(result.Body.Bytes(), []byte("/v1/ysxxai-reference-images")) || len(paths) != 0 {
					t.Fatalf("response=%d %s, calls=%v", result.Code, result.Body.String(), paths)
				}
				return
			}
			if result.Code != http.StatusOK || len(paths) != 1 || paths[0] != tc.wantPath {
				t.Fatalf("response=%d %s, calls=%v", result.Code, result.Body.String(), paths)
			}
		})
	}
}
