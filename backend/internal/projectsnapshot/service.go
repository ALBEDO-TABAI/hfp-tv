package projectsnapshot

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"infinite-canvas/backend/internal/assets"
	"infinite-canvas/backend/internal/kernel"
)

// ResourceInfo is the metadata a snapshot keeps for one media file.
type ResourceInfo struct {
	ID         string
	Kind       string
	MimeType   string
	Size       int64
	Width      int
	Height     int
	DurationMs int64
	Name       string
	Ready      bool
}

type Drawing struct {
	Engine            string
	Snapshot          json.RawMessage
	ShapeCount        int
	PageCount         int
	PreviewResourceID string
	Render            *DrawingRender // SHA256 holds the render resource ID here
}

// Workspace is the slice of a workspace kernel the snapshot needs. The app
// layer implements it; all calls act on one owner.
type Workspace interface {
	Canvases(userID string) ([]json.RawMessage, error)
	Drawing(userID, canvasID, drawingID string) (*Drawing, error)
	Resource(userID, id string) (*ResourceInfo, error)
	OpenResource(userID, id string) (io.ReadCloser, error)
	StoreResource(userID string, file File, body io.Reader, uploadKey string) (string, error)
	UpsertAsset(userID string, raw json.RawMessage) error
	DeleteAsset(userID, id string) error
	CreateCanvas(userID string, raw json.RawMessage) error
	DeleteCanvas(userID, id string) error
	PutDrawing(userID, canvasID, drawingID string, raw json.RawMessage) error
}

type Service struct {
	ws     Workspace
	userID string
	now    func() time.Time
}

func New(ws Workspace, userID string) *Service {
	return &Service{ws: ws, userID: userID, now: time.Now}
}

// ErrBlocked is returned by Export when preflight found blocking issues.
type ErrBlocked struct{ Preflight Preflight }

func (e *ErrBlocked) Error() string { return "项目还有需要处理的问题，不能投稿" }

var ErrProjectNotFound = errors.New("项目不存在")

type canvasDoc struct {
	id      string
	title   string
	created string
	body    map[string]any
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}

// projectCanvases returns the project's canvases, root first, then by
// creation time — the order the canvas page shows them.
func (s *Service) projectCanvases(projectID string) ([]canvasDoc, error) {
	projectID = strings.TrimSpace(projectID)
	raws, err := s.ws.Canvases(s.userID)
	if err != nil {
		return nil, err
	}
	var docs []canvasDoc
	rootFound := false
	for _, raw := range raws {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		id := stringField(body, "id")
		owner := stringField(body, "workspaceProjectId")
		if owner == "" {
			owner = id
		}
		if owner != projectID {
			continue
		}
		if id == projectID {
			rootFound = true
		}
		docs = append(docs, canvasDoc{id: id, title: stringField(body, "title"), created: stringField(body, "createdAt"), body: body})
	}
	if !rootFound {
		return nil, ErrProjectNotFound
	}
	sort.SliceStable(docs, func(i, j int) bool {
		if (docs[i].id == projectID) != (docs[j].id == projectID) {
			return docs[i].id == projectID
		}
		if docs[i].created != docs[j].created {
			return docs[i].created < docs[j].created
		}
		return docs[i].id < docs[j].id
	})
	if len(docs) > maxCanvases {
		return nil, fmt.Errorf("项目画布超过 %d 张", maxCanvases)
	}
	return docs, nil
}

func nodesOf(body map[string]any) []map[string]any {
	raw, _ := body["nodes"].([]any)
	nodes := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if node, ok := item.(map[string]any); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func metadataOf(node map[string]any) map[string]any {
	metadata, _ := node["metadata"].(map[string]any)
	return metadata
}

// scan walks one canvas without changing it and reports references and issues.
func (s *Service) scan(doc canvasDoc, preflight *Preflight, resources map[string]*reference, drawings map[string]bool) {
	for _, node := range nodesOf(doc.body) {
		nodeID := stringField(node, "id")
		metadata := metadataOf(node)
		if stringField(node, "type") == "drawing" && metadata != nil {
			if drawingID := stringField(metadata, "drawingId"); drawingID != "" {
				drawings[drawingID] = true
			}
		}
		if metadata != nil {
			switch stringField(metadata, "status") {
			case "generating", "pending", "queued", "running":
				preflight.Issues = append(preflight.Issues, Issue{Level: "warning", Code: "generation_running", Message: "节点仍在生成，投稿只包含生成前的内容", CanvasID: doc.id, NodeID: nodeID})
			}
		}
		walkStrings(cloneValue(node), "", func(field, value string) (string, bool) {
			trimmed := strings.TrimSpace(value)
			if id := resourceRef(field, trimmed); id != "" {
				if resources[id] == nil {
					resources[id] = &reference{resourceID: id, nodeID: nodeID}
				}
				return value, true
			}
			if locatorFields[field] {
				switch {
				case strings.HasPrefix(trimmed, "blob:") || browserOnlyKey.MatchString(trimmed):
					preflight.Issues = append(preflight.Issues, Issue{Level: "error", Code: "browser_only_media", Message: "素材只保存在浏览器里，请先重新上传到服务器", CanvasID: doc.id, NodeID: nodeID})
				case strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://"):
					preflight.Issues = append(preflight.Issues, Issue{Level: "warning", Code: "external_link", Message: "引用了外部链接，复制后仍指向原地址", CanvasID: doc.id, NodeID: nodeID})
				}
				return value, true
			}
			if hint := privacyHint(value); hint != "" {
				preflight.Issues = append(preflight.Issues, Issue{Level: "warning", Code: "privacy_hint", Message: hint + "，公开前请确认", CanvasID: doc.id, NodeID: nodeID})
			}
			return value, true
		})
	}
	// Timeline and director scenes carry media without being nodes.
	for _, key := range []string{"timeline", "directorScenes"} {
		if value, ok := doc.body[key]; ok {
			walkStrings(cloneValue(value), key, func(field, value string) (string, bool) {
				if id := resourceRef(field, strings.TrimSpace(value)); id != "" && resources[id] == nil {
					resources[id] = &reference{resourceID: id}
				}
				return value, true
			})
		}
	}
}

func cloneValue(value any) any {
	raw, _ := json.Marshal(value)
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

func dedupeIssues(issues []Issue) []Issue {
	seen := map[string]bool{}
	out := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		key := issue.Level + "\x00" + issue.Code + "\x00" + issue.CanvasID + "\x00" + issue.NodeID
		if !seen[key] {
			seen[key] = true
			out = append(out, issue)
		}
	}
	return out
}

type plan struct {
	preflight Preflight
	docs      []canvasDoc
	resources map[string]*ResourceInfo
	drawings  map[string]map[string]*Drawing // canvas → drawing
}

func (s *Service) build(projectID string) (*plan, error) {
	docs, err := s.projectCanvases(projectID)
	if err != nil {
		return nil, err
	}
	p := &plan{docs: docs, resources: map[string]*ResourceInfo{}, drawings: map[string]map[string]*Drawing{}}
	p.preflight = Preflight{ProjectID: projectID, Title: docs[0].title, Issues: []Issue{}, Covers: []CoverCandidate{}}
	refs := map[string]*reference{}
	for _, doc := range docs {
		drawingIDs := map[string]bool{}
		s.scan(doc, &p.preflight, refs, drawingIDs)
		p.preflight.Stats.Nodes += len(nodesOf(doc.body))
		p.drawings[doc.id] = map[string]*Drawing{}
		for drawingID := range drawingIDs {
			drawing, err := s.ws.Drawing(s.userID, doc.id, drawingID)
			if err != nil || drawing == nil {
				p.preflight.Issues = append(p.preflight.Issues, Issue{Level: "error", Code: "drawing_missing", Message: "画板内容还没保存到服务器", CanvasID: doc.id})
				continue
			}
			p.drawings[doc.id][drawingID] = drawing
			p.preflight.Stats.Drawings++
			for _, id := range []string{drawing.PreviewResourceID, renderResource(drawing)} {
				if id != "" && refs[id] == nil {
					refs[id] = &reference{resourceID: id}
				}
			}
		}
		for _, node := range nodesOf(doc.body) {
			metadata := metadataOf(node)
			if stringField(node, "type") != "image" || metadata == nil {
				continue
			}
			if id := assets.ResourceID(stringField(metadata, "storageKey")); id != "" {
				title := stringField(node, "title")
				p.preflight.Covers = append(p.preflight.Covers, CoverCandidate{ResourceID: id, CanvasID: doc.id, NodeID: stringField(node, "id"), Title: title})
			}
		}
	}
	ids := make([]string, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		info, err := s.ws.Resource(s.userID, id)
		if err != nil || info == nil || !info.Ready {
			p.preflight.Issues = append(p.preflight.Issues, Issue{Level: "error", Code: "missing_file", Message: "引用的文件不存在或还没上传完成", NodeID: refs[id].nodeID})
			continue
		}
		p.resources[id] = info
		p.preflight.Stats.Bytes += info.Size
	}
	p.preflight.Stats.Canvases = len(docs)
	p.preflight.Stats.Files = len(p.resources)
	if len(p.resources) > maxFiles {
		p.preflight.Issues = append(p.preflight.Issues, Issue{Level: "error", Code: "too_many_files", Message: fmt.Sprintf("文件超过 %d 个", maxFiles)})
	}
	p.preflight.Issues = dedupeIssues(p.preflight.Issues)
	return p, nil
}

func renderResource(drawing *Drawing) string {
	if drawing.Render == nil {
		return ""
	}
	return drawing.Render.SHA256
}

func (s *Service) Preflight(projectID string) (*Preflight, error) {
	p, err := s.build(projectID)
	if err != nil {
		return nil, err
	}
	return &p.preflight, nil
}

// Export copies every referenced file into sink and returns the snapshot.
// Nothing is exported when preflight reports a blocking issue.
func (s *Service) Export(projectID string, sink BlobSink) (*ExportResult, error) {
	p, err := s.build(projectID)
	if err != nil {
		return nil, err
	}
	if p.preflight.Blocked() {
		return nil, &ErrBlocked{Preflight: p.preflight}
	}
	shaOf := map[string]string{}
	files := []File{}
	for id, info := range p.resources {
		body, err := s.ws.OpenResource(s.userID, id)
		if err != nil {
			return nil, err
		}
		sha, size, err := sink.Put(body)
		_ = body.Close()
		if err != nil {
			return nil, err
		}
		if size != info.Size {
			return nil, fmt.Errorf("文件 %s 大小与记录不一致", id)
		}
		shaOf[id] = sha
		files = append(files, File{SHA256: sha, Size: size, MimeType: info.MimeType, Kind: info.Kind, Name: info.Name, Width: info.Width, Height: info.Height, DurationMs: info.DurationMs})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].SHA256 < files[j].SHA256 })
	files = uniqueFiles(files)
	snapshot := Snapshot{Format: Format, Version: Version, Title: p.preflight.Title, Files: files, Stats: p.preflight.Stats}
	snapshot.Stats.Files = len(files)
	for index, doc := range p.docs {
		kept := map[string]any{}
		for _, key := range keptCanvasFields {
			if value, ok := doc.body[key]; ok {
				kept[key] = value
			}
		}
		rewritten := walkStrings(kept, "", func(field, value string) (string, bool) {
			return toSnapshotValue(field, value, shaOf)
		})
		document, err := json.Marshal(rewritten)
		if err != nil {
			return nil, err
		}
		canvas := CanvasSnapshot{Key: fmt.Sprintf("c%d", index), Title: doc.title, Document: document, Drawings: []DrawingSnapshot{}}
		drawingIDs := make([]string, 0, len(p.drawings[doc.id]))
		for drawingID := range p.drawings[doc.id] {
			drawingIDs = append(drawingIDs, drawingID)
		}
		sort.Strings(drawingIDs)
		for _, drawingID := range drawingIDs {
			drawing := p.drawings[doc.id][drawingID]
			entry := DrawingSnapshot{DrawingID: drawingID, Engine: drawing.Engine, Snapshot: drawing.Snapshot, ShapeCount: drawing.ShapeCount, PageCount: drawing.PageCount, Preview: shaOf[drawing.PreviewResourceID]}
			if drawing.Render != nil && shaOf[drawing.Render.SHA256] != "" {
				render := *drawing.Render
				render.SHA256 = shaOf[drawing.Render.SHA256]
				entry.Render = &render
			}
			canvas.Drawings = append(canvas.Drawings, entry)
		}
		snapshot.Canvases = append(snapshot.Canvases, canvas)
	}
	return &ExportResult{Snapshot: snapshot, Resources: shaOf}, nil
}

func uniqueFiles(files []File) []File {
	out := files[:0]
	for index, file := range files {
		if index > 0 && file.SHA256 == files[index-1].SHA256 {
			continue
		}
		out = append(out, file)
	}
	return out
}

// Validate checks a snapshot's shape before any import write.
func Validate(snapshot *Snapshot) error {
	if snapshot.Format != Format || snapshot.Version != Version {
		return errors.New("快照格式不受支持")
	}
	if len(snapshot.Canvases) == 0 || len(snapshot.Canvases) > maxCanvases || len(snapshot.Files) > maxFiles {
		return errors.New("快照画布或文件数量无效")
	}
	for _, file := range snapshot.Files {
		if _, err := hex.DecodeString(file.SHA256); err != nil || len(file.SHA256) != 64 || file.Size < 0 {
			return errors.New("快照文件清单无效")
		}
	}
	return nil
}

func mediaKind(kind string) string {
	switch kind {
	case "image", "video", "audio":
		return kind
	}
	return ""
}

// Import recreates a snapshot as a new project in this workspace. On failure
// every created canvas and asset is removed; stored resources stay and are
// reused by a retry through their upload keys.
func (s *Service) Import(snapshot *Snapshot, source BlobSource, options ImportOptions) (result *ImportResult, importErr error) {
	if err := Validate(snapshot); err != nil {
		return nil, err
	}
	var createdCanvases, createdAssets []string
	defer func() {
		if importErr == nil {
			return
		}
		for index := len(createdCanvases) - 1; index >= 0; index-- {
			_ = s.ws.DeleteCanvas(s.userID, createdCanvases[index])
		}
		for _, id := range createdAssets {
			_ = s.ws.DeleteAsset(s.userID, id)
		}
	}()

	resources := map[string]string{}
	files := map[string]File{}
	for _, file := range snapshot.Files {
		body, err := source.Open(file.SHA256)
		if err != nil {
			return nil, err
		}
		// The upload key makes a repeated copy reuse the same resource. The blob
		// store verified each file's SHA-256 when it was written.
		id, err := s.ws.StoreResource(s.userID, file, body, KeyPrefix+file.SHA256)
		_ = body.Close()
		if err != nil {
			return nil, err
		}
		resources[file.SHA256] = id
		files[id] = file
	}

	now := s.now().UTC()
	assetFor := map[string]string{}
	ensureAsset := func(resourceID, title string) (string, error) {
		if id, ok := assetFor[resourceID]; ok {
			return id, nil
		}
		file := files[resourceID]
		kind := mediaKind(file.Kind)
		if kind == "" {
			kind = mediaKind(strings.SplitN(file.MimeType, "/", 2)[0])
		}
		if kind == "" {
			return "", nil
		}
		raw, err := assetPayload(kernel.NewID(), kind, resourceID, file, title, now)
		if err != nil {
			return "", err
		}
		var identity struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &identity)
		if err := s.ws.UpsertAsset(s.userID, raw); err != nil {
			return "", err
		}
		createdAssets = append(createdAssets, identity.ID)
		assetFor[resourceID] = identity.ID
		return identity.ID, nil
	}

	rootID := ""
	canvasIDs := []string{}
	for index, canvas := range snapshot.Canvases {
		var document map[string]any
		if err := json.Unmarshal(canvas.Document, &document); err != nil {
			return nil, errors.New("快照画布内容无效")
		}
		document = walkStrings(document, "", func(field, value string) (string, bool) {
			return fromSnapshotValue(field, value, resources)
		}).(map[string]any)
		// Media nodes and timeline clips must point at an asset record that
		// references their resource; the canvas save guard rejects them otherwise.
		for _, node := range nodesOf(document) {
			metadata := metadataOf(node)
			if metadata == nil || mediaKind(stringField(node, "type")) == "" {
				continue
			}
			resourceID := assets.ResourceID(stringField(metadata, "storageKey"))
			if resourceID == "" {
				resourceID = assets.ResourceID(stringField(metadata, "content"))
			}
			if resourceID == "" {
				continue
			}
			assetID, err := ensureAsset(resourceID, stringField(node, "title"))
			if err != nil {
				return nil, err
			}
			if assetID != "" {
				metadata["assetId"] = assetID
			}
		}
		if timeline, ok := document["timeline"].(map[string]any); ok {
			clips, _ := timeline["clips"].([]any)
			for _, raw := range clips {
				clip, _ := raw.(map[string]any)
				media, _ := clip["directMedia"].(map[string]any)
				if media == nil || mediaKind(stringField(media, "kind")) == "" {
					continue
				}
				resourceID := ""
				for _, key := range []string{"storageKey", "url", "dataUrl", "content"} {
					if resourceID = assets.ResourceID(stringField(media, key)); resourceID != "" {
						break
					}
				}
				if resourceID == "" {
					continue
				}
				assetID, err := ensureAsset(resourceID, stringField(media, "name"))
				if err != nil {
					return nil, err
				}
				if assetID != "" {
					media["assetId"] = assetID
				}
			}
		}
		canvasID := kernel.NewID()
		if index == 0 {
			rootID = canvasID
		}
		title := canvas.Title
		if index == 0 && strings.TrimSpace(options.Title) != "" {
			title = strings.TrimSpace(options.Title)
		}
		if strings.TrimSpace(title) == "" {
			title = snapshot.Title
		}
		document["id"] = canvasID
		document["title"] = title
		document["revision"] = 0
		document["workspaceProjectId"] = rootID
		if options.FolderID != "" {
			document["folderId"] = options.FolderID
		}
		raw, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}
		if err := s.ws.CreateCanvas(s.userID, raw); err != nil {
			return nil, err
		}
		createdCanvases = append(createdCanvases, canvasID)
		canvasIDs = append(canvasIDs, canvasID)
		for _, drawing := range canvas.Drawings {
			payload := map[string]any{"drawingId": drawing.DrawingID, "engine": drawing.Engine, "revision": 0, "snapshot": drawing.Snapshot,
				"shapeCount": drawing.ShapeCount, "pageCount": drawing.PageCount}
			if drawing.Preview != "" {
				payload["previewResourceId"] = resources[drawing.Preview]
			}
			if drawing.Render != nil && resources[drawing.Render.SHA256] != "" {
				payload["render"] = map[string]any{"resourceId": resources[drawing.Render.SHA256], "pageId": drawing.Render.PageID, "width": drawing.Render.Width,
					"height": drawing.Render.Height, "mimeType": drawing.Render.MimeType, "background": drawing.Render.Background}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			if err := s.ws.PutDrawing(s.userID, canvasID, drawing.DrawingID, raw); err != nil {
				return nil, err
			}
		}
	}
	return &ImportResult{ProjectID: rootID, CanvasIDs: canvasIDs, Files: len(resources)}, nil
}

func assetPayload(assetID, kind, resourceID string, file File, title string, now time.Time) (json.RawMessage, error) {
	url := assets.FileURL(resourceID)
	width, height := file.Width, file.Height
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	data := map[string]any{"storageKey": "resource:" + resourceID, "mimeType": file.MimeType, "bytes": file.Size, "width": width, "height": height}
	cover := url
	if kind == "image" {
		data["dataUrl"] = url
	} else {
		data["url"] = url
		if file.DurationMs > 0 {
			data["durationMs"] = file.DurationMs
		}
		if kind == "audio" {
			cover = ""
		}
	}
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(path.Base(file.Name), path.Ext(file.Name))
	}
	if strings.TrimSpace(title) == "" || title == "." {
		title = map[string]string{"image": "工坊图片", "video": "工坊视频", "audio": "工坊音频"}[kind]
	}
	stamp := now.Format(time.RFC3339Nano)
	return json.Marshal(map[string]any{"id": assetID, "kind": kind, "category": "material", "status": "confirmed", "title": title,
		"coverUrl": cover, "tags": []string{"创意工坊"}, "source": "创意工坊", "createdAt": stamp, "updatedAt": stamp, "data": data,
		"metadata": map[string]any{"source": "workshop-copy", "sha256": file.SHA256}})
}
