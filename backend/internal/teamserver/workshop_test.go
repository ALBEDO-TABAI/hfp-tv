package teamserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/kernelpool"
	"infinite-canvas/backend/internal/workshop"
)

func newWorkshopHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "false")
	root := t.TempDir()
	users, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	shop, err := workshop.Open(users.DB(), root, nil, users, 0)
	if err != nil {
		t.Fatal(err)
	}
	pool := kernelpool.New(kernelpool.Options{ShutdownTimeout: 5 * time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = pool.CloseAll(ctx)
		_ = users.Close()
	})
	server, err := New(Options{Accounts: users, Pool: pool, DataRoot: root, CookieSecure: true, Workshop: shop})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, server: server, users: users}
}

// uploadImage stores a PNG in the caller's workspace through the gateway.
func (h *harness) uploadImage(cookie string) string {
	h.t.Helper()
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 16, 9)))
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "frame.png")
	_, _ = part.Write(picture.Bytes())
	_ = writer.WriteField("kind", "image")
	_ = writer.WriteField("width", "16")
	_ = writer.WriteField("height", "9")
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "https://team.example.com/api/resources", &body)
	request.RemoteAddr = "203.0.113.5:40000"
	request.Header.Set("Origin", "https://team.example.com")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, request)
	var uploaded struct {
		Resource struct {
			ID string `json:"id"`
		} `json:"resource"`
	}
	decodeData(h.t, recorder, &uploaded)
	return uploaded.Resource.ID
}

func (h *harness) seedWorkshopProject(cookie, projectID string) string {
	h.t.Helper()
	resourceID := h.uploadImage(cookie)
	url := "/api/resources/" + resourceID + "/file"
	asset, _ := json.Marshal(map[string]any{"asset": map[string]any{"id": "asset-" + projectID, "kind": "image", "category": "material", "status": "confirmed",
		"title": "镜头", "coverUrl": url, "tags": []string{}, "data": map[string]any{"storageKey": "resource:" + resourceID, "dataUrl": url,
			"mimeType": "image/png", "bytes": 1, "width": 16, "height": 9}}})
	decodeData(h.t, h.do(call{method: http.MethodPut, path: "/api/assets/asset-" + projectID, cookie: cookie, body: string(asset)}), nil)
	canvas, _ := json.Marshal(map[string]any{"project": map[string]any{"id": projectID, "title": "雨夜追车", "revision": 0, "workspaceProjectId": projectID,
		"connections": []any{}, "nodes": []any{map[string]any{"id": "n1", "type": "image", "title": "镜头", "position": map[string]any{"x": 0, "y": 0},
			"metadata": map[string]any{"storageKey": "resource:" + resourceID, "content": url, "assetId": "asset-" + projectID, "prompt": "雨夜追车"}}}}})
	decodeData(h.t, h.do(call{method: http.MethodPut, path: "/api/canvas-projects/" + projectID, cookie: cookie, body: string(canvas)}), nil)
	return resourceID
}

func TestWorkshopSubmitReviewBrowseCopy(t *testing.T) {
	h := newWorkshopHarness(t)
	ctx := context.Background()
	admin, _ := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-pass", Role: accounts.RoleAdmin})
	_, _ = h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "alice", Password: "alice-pass"})
	_, _ = h.users.CreateUser(ctx, admin.ID, accounts.CreateUserInput{Username: "bob", Password: "bob-pass1"})
	adminCookie, aliceCookie, bobCookie := h.login("admin", "admin-pass"), h.login("alice", "alice-pass"), h.login("bob", "bob-pass1")

	coverID := h.seedWorkshopProject(aliceCookie, "alice-project")
	var preflight struct {
		Issues []struct{ Level string } `json:"issues"`
		Stats  struct{ Files int }      `json:"stats"`
		Covers []struct {
			ResourceID string `json:"resourceId"`
		} `json:"covers"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/submissions/preflight", cookie: aliceCookie, body: `{"projectId":"alice-project"}`}), &preflight)
	if preflight.Stats.Files != 1 || len(preflight.Covers) != 1 || preflight.Covers[0].ResourceID != coverID {
		t.Fatalf("preflight = %+v", preflight)
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/community/submissions", cookie: aliceCookie,
		body: `{"projectId":"alice-project","title":"雨夜追车分镜","rightsConfirmed":false}`}); response.Code != http.StatusBadRequest {
		t.Fatalf("submission without rights confirmation = %d", response.Code)
	}
	var submitted struct {
		Publication workshop.PublicationView `json:"publication"`
		Version     workshop.VersionView     `json:"version"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/submissions", cookie: aliceCookie,
		body: `{"projectId":"alice-project","title":"雨夜追车分镜","summary":"三张画布的分镜","tags":["分镜"],"coverResourceId":"` + coverID + `","allowDownload":true,"rightsConfirmed":true}`}), &submitted)
	if submitted.Version.ReviewStatus != workshop.ReviewPending || submitted.Version.CoverSHA256 == "" {
		t.Fatalf("submission = %+v", submitted)
	}
	versionID, publicationID := submitted.Version.ID, submitted.Publication.ID

	// Invisible to others until approved.
	var listing struct {
		Items []workshop.PublicationView `json:"items"`
	}
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/community/publications", cookie: bobCookie}), &listing)
	if len(listing.Items) != 0 {
		t.Fatal("pending work is listed")
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/community/versions/" + versionID + "/snapshot", cookie: bobCookie}); response.Code != http.StatusNotFound {
		t.Fatalf("pending snapshot readable by another member: %d", response.Code)
	}

	var queue struct {
		Items []workshop.ReviewItem `json:"items"`
	}
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/platform/admin/workshop/versions?status=pending", cookie: adminCookie}), &queue)
	if len(queue.Items) != 1 || queue.Items[0].Author.Username != "alice" {
		t.Fatalf("queue = %+v", queue.Items)
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/community/versions/" + versionID + "/snapshot", cookie: adminCookie}); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "workshop:") || strings.Contains(response.Body.String(), "resource:") {
		t.Fatalf("reviewer snapshot = %d %s", response.Code, response.Body.String())
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/platform/admin/workshop/versions/" + versionID + "/approve", cookie: adminCookie,
		body: `{"snapshotSha256":"stale"}`}); response.Code != http.StatusConflict {
		t.Fatalf("stale approval = %d", response.Code)
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/platform/admin/workshop/versions/" + versionID + "/approve", cookie: adminCookie,
		body: `{"snapshotSha256":"` + queue.Items[0].Version.SnapshotSHA256 + `","official":true}`}), nil)

	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/community/publications?sort=latest", cookie: bobCookie}), &listing)
	if len(listing.Items) != 1 || !listing.Items[0].Official || listing.Items[0].CurrentVersionID != versionID {
		t.Fatalf("listing after approval = %+v", listing.Items)
	}
	cover := listing.Items[0].CoverSHA256
	file := h.do(call{method: http.MethodGet, path: "/api/community/files/" + cover + "?v=" + versionID, cookie: bobCookie})
	if file.Code != http.StatusOK || file.Header().Get("Content-Type") != "image/png" || file.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("cover file = %d %v", file.Code, file.Header())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/community/files/" + cover + "?v=v_unknown", cookie: bobCookie}); response.Code == http.StatusOK {
		t.Fatal("file served through an unknown version")
	}

	var copied struct {
		Copy workshop.CopyView `json:"copy"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/publications/" + publicationID + "/copies", cookie: bobCookie,
		body: `{"requestId":"bob-copy-0001","title":"我的雨夜追车"}`}), &copied)
	if copied.Copy.Status != workshop.CopyCompleted || copied.Copy.TargetProjectID == "" {
		t.Fatalf("copy = %+v", copied)
	}
	target := copied.Copy.TargetProjectID
	canvas := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/" + target, cookie: bobCookie})
	if canvas.Code != http.StatusOK || !strings.Contains(canvas.Body.String(), "我的雨夜追车") || strings.Contains(canvas.Body.String(), coverID) {
		t.Fatalf("copied canvas = %d %s", canvas.Code, canvas.Body.String())
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/" + target, cookie: aliceCookie}); response.Code == http.StatusOK {
		t.Fatal("copy landed in the author's workspace")
	}
	var again struct {
		Copy workshop.CopyView `json:"copy"`
	}
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/publications/" + publicationID + "/copies", cookie: bobCookie,
		body: `{"requestId":"bob-copy-0001"}`}), &again)
	if again.Copy.TargetProjectID != target {
		t.Fatal("same request ID produced another copy")
	}

	// Bob's edited copy is submitted as a derived work.
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/submissions", cookie: bobCookie,
		body: `{"projectId":"` + target + `","title":"雨夜追车·改","rightsConfirmed":true}`}), &submitted)
	if submitted.Version.DerivedFrom == nil || submitted.Version.DerivedFrom.PublicationID != publicationID {
		t.Fatalf("derived work not labelled: %+v", submitted.Version)
	}
	var origin workshop.DerivedView
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/community/origins/" + target, cookie: bobCookie}), &origin)
	if origin.Title != "雨夜追车分镜" || !origin.Available {
		t.Fatalf("origin = %+v", origin)
	}

	// Unlisting hides the work; the copy stays.
	decodeData(t, h.do(call{method: http.MethodPost, path: "/api/community/publications/" + publicationID + "/unlist", cookie: aliceCookie, body: `{}`}), nil)
	decodeData(t, h.do(call{method: http.MethodGet, path: "/api/community/publications", cookie: bobCookie}), &listing)
	if len(listing.Items) != 0 {
		t.Fatal("unlisted work still listed")
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/community/publications/" + publicationID, cookie: bobCookie}); response.Code != http.StatusGone {
		t.Fatalf("unlisted detail = %d", response.Code)
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects/" + target, cookie: bobCookie}); response.Code != http.StatusOK {
		t.Fatal("unlisting removed an existing copy")
	}
	if response := h.do(call{method: http.MethodPost, path: "/api/community/publications/" + publicationID + "/unlist", cookie: bobCookie, body: `{}`}); response.Code != http.StatusForbidden {
		t.Fatalf("another member unlisted the work: %d", response.Code)
	}
}
