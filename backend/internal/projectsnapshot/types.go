// Package projectsnapshot exports a workspace project (all canvases sharing a
// workspaceProjectId, their drawings and media) as an immutable snapshot whose
// media are content-addressed, and imports such a snapshot into another
// workspace with fresh IDs, resources, asset records and drawings.
package projectsnapshot

import (
	"encoding/json"
	"io"
)

const (
	Format  = "hfp-workshop-snapshot"
	Version = 1

	// Placeholders in snapshot documents. "workshop:<sha256>" replaces a
	// resource key or bare resource ID; "workshop://<sha256>" replaces a
	// resource file URL.
	KeyPrefix = "workshop:"
	URLPrefix = "workshop://"

	maxCanvases = 64
	maxFiles    = 4000
)

// Snapshot is the published, immutable form of a project.
type Snapshot struct {
	Format   string           `json:"format"`
	Version  int              `json:"version"`
	Title    string           `json:"title"`
	Canvases []CanvasSnapshot `json:"canvases"`
	Files    []File           `json:"files"`
	Stats    Stats            `json:"stats"`
}

// CanvasSnapshot holds one canvas document with workspace-specific identity,
// runtime state and assistant conversations removed.
type CanvasSnapshot struct {
	Key      string            `json:"key"`
	Title    string            `json:"title"`
	Document json.RawMessage   `json:"document"`
	Drawings []DrawingSnapshot `json:"drawings"`
}

type DrawingSnapshot struct {
	DrawingID  string          `json:"drawingId"`
	Engine     string          `json:"engine"`
	Snapshot   json.RawMessage `json:"snapshot"`
	ShapeCount int             `json:"shapeCount"`
	PageCount  int             `json:"pageCount"`
	Preview    string          `json:"preview,omitempty"`
	Render     *DrawingRender  `json:"render,omitempty"`
}

type DrawingRender struct {
	SHA256     string `json:"sha256"`
	PageID     string `json:"pageId,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Background string `json:"background,omitempty"`
}

type File struct {
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	MimeType   string `json:"mimeType"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

type Stats struct {
	Canvases int   `json:"canvases"`
	Nodes    int   `json:"nodes"`
	Files    int   `json:"files"`
	Bytes    int64 `json:"bytes"`
	Drawings int   `json:"drawings"`
}

// Issue is a preflight finding. Errors block submission; warnings do not.
type Issue struct {
	Level    string `json:"level"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	CanvasID string `json:"canvasId,omitempty"`
	NodeID   string `json:"nodeId,omitempty"`
}

// CoverCandidate is an image the author may pick as the publication cover.
type CoverCandidate struct {
	ResourceID string `json:"resourceId"`
	CanvasID   string `json:"canvasId"`
	NodeID     string `json:"nodeId"`
	Title      string `json:"title"`
}

type Preflight struct {
	ProjectID string           `json:"projectId"`
	Title     string           `json:"title"`
	Stats     Stats            `json:"stats"`
	Issues    []Issue          `json:"issues"`
	Covers    []CoverCandidate `json:"covers"`
}

func (p Preflight) Blocked() bool {
	for _, issue := range p.Issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

// ExportResult carries the snapshot and the author-side resource → file map,
// so the platform can resolve the chosen cover.
type ExportResult struct {
	Snapshot  Snapshot
	Resources map[string]string
}

type ImportOptions struct {
	Title    string
	FolderID string
}

type ImportResult struct {
	ProjectID string   `json:"projectId"`
	CanvasIDs []string `json:"canvasIds"`
	Files     int      `json:"files"`
}

// BlobSink stores exported bytes content-addressed and returns their SHA-256.
type BlobSink interface {
	Put(body io.Reader) (sha256 string, size int64, err error)
}

// BlobSource opens previously stored bytes by SHA-256.
type BlobSource interface {
	Open(sha256 string) (io.ReadCloser, error)
}
