package app

import (
	"encoding/json"
	"io"
	"path"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/projectsnapshot"
)

// projectSnapshotWorkspace adapts the workspace services to the project
// snapshot port used by the team-server workshop.
type projectSnapshotWorkspace struct{ s *Service }

func (s *Service) ProjectSnapshotWorkspace() projectsnapshot.Workspace {
	return projectSnapshotWorkspace{s: s}
}

func (w projectSnapshotWorkspace) Canvases(userID string) ([]json.RawMessage, error) {
	return w.s.UserCanvasProjects(userID)
}

func (w projectSnapshotWorkspace) Drawing(userID, canvasID, drawingID string) (*projectsnapshot.Drawing, error) {
	document, err := w.s.UserCanvasDrawing(userID, canvasID, drawingID)
	if err != nil {
		return nil, err
	}
	drawing := &projectsnapshot.Drawing{Engine: document.Engine, Snapshot: document.Snapshot, ShapeCount: document.ShapeCount,
		PageCount: document.PageCount, PreviewResourceID: document.PreviewResourceID}
	if render := document.Render; render != nil && render.ResourceID != "" {
		drawing.Render = &projectsnapshot.DrawingRender{SHA256: render.ResourceID, PageID: render.PageID, Width: render.Width,
			Height: render.Height, MimeType: render.MimeType, Background: render.Background}
	}
	return drawing, nil
}

func (w projectSnapshotWorkspace) Resource(userID, id string) (*projectsnapshot.ResourceInfo, error) {
	resource, err := w.s.ResourceService().Resource(userID, id)
	if err != nil || resource == nil {
		return nil, err
	}
	return &projectsnapshot.ResourceInfo{ID: resource.ID, Kind: resource.Kind, MimeType: resource.MimeType, Size: resource.Size,
		Width: resource.Width, Height: resource.Height, DurationMs: resource.DurationMs, Name: path.Base(resource.ObjectKey),
		Ready: resource.Status == model.ResourceStatusReady && resource.Provider == "local"}, nil
}

func (w projectSnapshotWorkspace) OpenResource(userID, id string) (io.ReadCloser, error) {
	_, body, err := w.s.OpenResource(userID, id)
	return body, err
}

func (w projectSnapshotWorkspace) StoreResource(userID string, file projectsnapshot.File, body io.Reader, uploadKey string) (string, error) {
	kind := file.Kind
	if kind == "" {
		kind = "file"
	}
	resource, _, err := w.s.ResourceService().Store(userID, kind, file.Name, file.MimeType, file.Size, file.Width, file.Height, file.DurationMs, body, &uploadKey)
	if err != nil {
		return "", err
	}
	return resource.ID, nil
}

func (w projectSnapshotWorkspace) UpsertAsset(userID string, raw json.RawMessage) error {
	_, err := w.s.UpsertUserAsset(userID, raw)
	return err
}

func (w projectSnapshotWorkspace) DeleteAsset(userID, id string) error {
	return w.s.DeleteUserAsset(userID, id)
}

func (w projectSnapshotWorkspace) CreateCanvas(userID string, raw json.RawMessage) error {
	_, err := w.s.UpsertUserCanvasProject(userID, raw)
	return err
}

func (w projectSnapshotWorkspace) DeleteCanvas(userID, id string) error {
	return w.s.DeleteUserCanvasProject(userID, id)
}

func (w projectSnapshotWorkspace) PutDrawing(userID, canvasID, drawingID string, raw json.RawMessage) error {
	_, err := w.s.UpsertUserCanvasDrawing(userID, canvasID, drawingID, raw)
	return err
}
