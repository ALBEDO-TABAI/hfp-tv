package bootstrap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/projectsnapshot"
)

type memoryBlobs map[string][]byte

func (m memoryBlobs) Put(body io.Reader) (string, int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	m[sha] = data
	return sha, int64(len(data)), nil
}

func (m memoryBlobs) Open(sha string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m[sha])), nil
}

func pngBytes(t *testing.T, width int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, 8))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// seedProject builds a two-canvas project the way the editor stores one: an
// image node bound to an asset, a timeline clip, a drawing with a preview and
// a sibling canvas sharing the root's workspaceProjectId.
func seedProject(t *testing.T, runtime *Runtime) (ownerID, rootID string) {
	t.Helper()
	owner, err := runtime.service.LocalWorkspaceOwner()
	if err != nil {
		t.Fatal(err)
	}
	store := func(name string, data []byte) string {
		key := "seed:" + name
		resource, _, err := runtime.service.ResourceService().Store(owner.ID, "image", name, "image/png", int64(len(data)), 8, 8, 0, bytes.NewReader(data), &key)
		if err != nil {
			t.Fatal(err)
		}
		return resource.ID
	}
	imageID, previewID := store("shot.png", pngBytes(t, 8)), store("drawing.png", pngBytes(t, 12))
	asset := map[string]any{"id": "asset-seed", "kind": "image", "category": "material", "status": "confirmed", "title": "镜头一",
		"coverUrl": "/api/resources/" + imageID + "/file", "tags": []string{}, "data": map[string]any{"storageKey": "resource:" + imageID,
			"dataUrl": "/api/resources/" + imageID + "/file", "mimeType": "image/png", "bytes": 1, "width": 8, "height": 8}}
	raw, _ := json.Marshal(asset)
	if _, err := runtime.service.UpsertUserAsset(owner.ID, raw); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"id": "root-canvas", "title": "雨夜追车", "revision": 0, "workspaceProjectId": "root-canvas",
		"chatSessions": []any{map[string]any{"id": "chat", "messages": []any{"secret plan"}}},
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "镜头一", "position": map[string]any{"x": 0, "y": 0},
				"metadata": map[string]any{"storageKey": "resource:" + imageID, "content": "/api/resources/" + imageID + "/file", "assetId": "asset-seed", "taskId": "task-1", "prompt": "雨夜，追车"}},
			map[string]any{"id": "d1", "type": "drawing", "title": "分镜草图", "position": map[string]any{"x": 400, "y": 0},
				"metadata": map[string]any{"drawingId": "drawing-1"}},
		},
		"connections": []any{},
		"timeline": map[string]any{"version": 2, "durationMs": 1000, "tracks": []any{}, "clips": []any{map[string]any{"id": "clip", "kind": "image",
			"directMedia": map[string]any{"id": "clip-media", "kind": "image", "storageKey": "resource:" + imageID, "assetId": "asset-seed"}}}},
	}
	sibling := map[string]any{"id": "sibling-canvas", "title": "角色设定", "canvasTitle": "角色设定", "revision": 0, "workspaceProjectId": "root-canvas",
		"nodes":       []any{map[string]any{"id": "n2", "type": "text", "title": "设定", "position": map[string]any{"x": 0, "y": 0}, "metadata": map[string]any{"content": "主角是一名出租车司机"}}},
		"connections": []any{}}
	for _, canvas := range []map[string]any{root, sibling} {
		raw, _ := json.Marshal(canvas)
		if _, err := runtime.service.UpsertUserCanvasProject(owner.ID, raw); err != nil {
			t.Fatalf("seed canvas %s: %v", canvas["id"], err)
		}
	}
	drawing, _ := json.Marshal(map[string]any{"revision": 0, "engine": "excalidraw", "snapshot": map[string]any{"elements": []any{}, "appState": map[string]any{}, "files": map[string]any{}},
		"shapeCount": 1, "pageCount": 1, "previewResourceId": previewID})
	if _, err := runtime.service.UpsertUserCanvasDrawing(owner.ID, "root-canvas", "drawing-1", drawing); err != nil {
		t.Fatal(err)
	}
	return owner.ID, "root-canvas"
}

func TestProjectSnapshotRoundTripsBetweenWorkspaces(t *testing.T) {
	author, copier := openHostedKernel(t, t.TempDir()), openHostedKernel(t, t.TempDir())
	_, rootID := seedProject(t, author)
	snapshots, err := author.ProjectSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := snapshots.Preflight(rootID)
	if err != nil || preflight.Blocked() || preflight.Stats.Canvases != 2 || preflight.Stats.Files != 2 || preflight.Stats.Drawings != 1 || len(preflight.Covers) != 1 {
		t.Fatalf("preflight = %+v, %v", preflight, err)
	}
	blobs := memoryBlobs{}
	exported, err := snapshots.Export(rootID, blobs)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(exported.Snapshot)
	text := string(encoded)
	for _, leaked := range []string{"resource:", "/api/resources/", "asset-seed", "task-1", "secret plan", "chatSessions", "root-canvas"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("snapshot leaks %q: %s", leaked, text)
		}
	}
	if len(exported.Snapshot.Canvases) != 2 || exported.Snapshot.Canvases[0].Title != "雨夜追车" || len(exported.Snapshot.Canvases[0].Drawings) != 1 || len(blobs) != 2 {
		t.Fatalf("snapshot shape: %s", text)
	}

	importer, err := copier.ProjectSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	var restored projectsnapshot.Snapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(&restored, blobs, projectsnapshot.ImportOptions{Title: "雨夜追车（副本）"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.CanvasIDs) != 2 || result.ProjectID != result.CanvasIDs[0] {
		t.Fatalf("import result %+v", result)
	}
	copierOwner, _ := copier.service.LocalWorkspaceOwner()
	raw, err := copier.service.UserCanvasProject(copierOwner.ID, result.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var canvas struct {
		Title              string `json:"title"`
		WorkspaceProjectID string `json:"workspaceProjectId"`
		Nodes              []struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"nodes"`
	}
	_ = json.Unmarshal(raw, &canvas)
	if canvas.Title != "雨夜追车（副本）" || canvas.WorkspaceProjectID != result.ProjectID {
		t.Fatalf("copied root canvas = %s", raw)
	}
	key, _ := canvas.Nodes[0].Metadata["storageKey"].(string)
	resourceID := strings.TrimPrefix(key, "resource:")
	if resource, err := copier.service.ResourceService().Resource(copierOwner.ID, resourceID); err != nil || resource == nil {
		t.Fatalf("copied node points at a resource the copier does not own: %s %v", key, err)
	}
	if canvas.Nodes[0].Metadata["assetId"] == "" || canvas.Nodes[0].Metadata["prompt"] != "雨夜，追车" {
		t.Fatalf("copied node metadata = %+v", canvas.Nodes[0].Metadata)
	}
	drawing, err := copier.service.UserCanvasDrawing(copierOwner.ID, result.ProjectID, "drawing-1")
	if err != nil || drawing.PreviewResourceID == "" {
		t.Fatalf("drawing not restored: %+v %v", drawing, err)
	}
	sibling, err := copier.service.UserCanvasProject(copierOwner.ID, result.CanvasIDs[1])
	if err != nil || !strings.Contains(string(sibling), `"workspaceProjectId":"`+result.ProjectID+`"`) {
		t.Fatalf("sibling canvas not grouped: %s %v", sibling, err)
	}
	// Copying the same snapshot again reuses the stored files.
	again, err := importer.Import(&restored, blobs, projectsnapshot.ImportOptions{})
	if err != nil || again.ProjectID == result.ProjectID {
		t.Fatalf("second copy: %+v %v", again, err)
	}
}

func TestProjectSnapshotPreflightBlocksBrowserOnlyMedia(t *testing.T) {
	runtime := openHostedKernel(t, t.TempDir())
	owner, _ := runtime.service.LocalWorkspaceOwner()
	raw, _ := json.Marshal(map[string]any{"id": "local-only", "title": "本地", "revision": 0, "connections": []any{},
		"nodes": []any{map[string]any{"id": "n", "type": "text", "title": "x", "position": map[string]any{"x": 0, "y": 0},
			"metadata": map[string]any{"storageKey": "image:scope:client-1", "prompt": "联系 13812345678"}}}})
	if _, err := runtime.service.UpsertUserCanvasProject(owner.ID, raw); err != nil {
		t.Fatal(err)
	}
	snapshots, _ := runtime.ProjectSnapshots()
	preflight, err := snapshots.Preflight("local-only")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]string{}
	for _, issue := range preflight.Issues {
		codes[issue.Code] = issue.Level
	}
	if codes["browser_only_media"] != "error" || codes["privacy_hint"] != "warning" || !preflight.Blocked() {
		t.Fatalf("issues = %+v", preflight.Issues)
	}
	if _, err := snapshots.Export("local-only", memoryBlobs{}); err == nil {
		t.Fatal("export ignored blocking issues")
	}
	if _, err := snapshots.Preflight("missing"); err == nil {
		t.Fatal("unknown project accepted")
	}
}
