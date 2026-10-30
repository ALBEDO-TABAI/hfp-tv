package projectsnapshot

import (
	"regexp"
	"strings"

	"infinite-canvas/backend/internal/assets"
)

// locatorFields hold a resource key or resource URL. It extends the asset
// reference scanner with fields that scanner does not cover yet.
var locatorFields = map[string]bool{
	"storageKey": true, "content": true, "previewContent": true, "drawingPreviewStorageKey": true, "drawingPreviewUrl": true,
	"url": true, "dataUrl": true, "coverUrl": true, "imageUrl": true, "videoUrl": true, "audioUrl": true, "referenceUrl": true,
	"referenceUrls": true, "artifactRef": true, "providerArtifactRef": true,
	"directorCoverStorageKey": true, "directorCoverUrl": true, "characterCoverUrl": true, "maskStorageKey": true,
	"themeCover": true, "directImageUrl": true,
}

// bareIDFields hold a bare resource ID.
var bareIDFields = map[string]bool{
	"resourceId": true, "resourceIds": true, "sampleResourceId": true, "referenceResourceId": true, "referenceResourceIds": true,
}

// strippedFields are workspace-local identities and runtime state; they never
// travel to another workspace. Asset IDs are rebuilt on import.
var strippedFields = map[string]bool{
	"assetId": true, "taskId": true, "taskClientOperationId": true, "taskStatus": true, "generationSubmissionState": true,
	"agentGenerationContinuation": true, "fileUpload": true, "pendingBackendSession": true,
	"characterAssetId": true, "characterVersionId": true, "skillIds": true, "skillVersions": true,
}

// keptCanvasFields are the top-level canvas fields a snapshot keeps. Identity
// (id, revision, folder, project links), viewport and assistant conversations
// stay with the author.
var keptCanvasFields = []string{"title", "canvasTitle", "nodes", "connections", "starterMode", "appearance", "backgroundMode", "showImageInfo", "directorScenes", "timeline"}

// browserOnlyKey matches storage keys whose bytes live only in a browser.
var browserOnlyKey = regexp.MustCompile(`^(image|video|audio|file|model|video-reference|audio-reference):`)

// reference is one resource use found while walking a document.
type reference struct {
	resourceID string
	nodeID     string
}

// walkStrings visits every string with its field name; arrays inherit the
// parent field name, as in the asset reference scanner. visit returns the
// replacement value and whether to keep the field.
func walkStrings(value any, field string, visit func(field, value string) (string, bool)) any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if strippedFields[key] {
				delete(item, key)
				continue
			}
			if text, ok := child.(string); ok {
				next, keep := visit(key, text)
				if !keep {
					delete(item, key)
				} else {
					item[key] = next
				}
				continue
			}
			item[key] = walkStrings(child, key, visit)
		}
		return item
	case []any:
		out := item[:0]
		for _, child := range item {
			if text, ok := child.(string); ok {
				if next, keep := visit(field, text); keep {
					out = append(out, next)
				}
				continue
			}
			out = append(out, walkStrings(child, field, visit))
		}
		return out
	}
	return value
}

// resourceRef extracts the resource a locator or bare-ID field points at.
func resourceRef(field, value string) string {
	if locatorFields[field] {
		return assets.ResourceID(value)
	}
	if bareIDFields[field] {
		return assets.ValidID(value)
	}
	return ""
}

// toSnapshotValue rewrites a reference to its content-addressed placeholder.
func toSnapshotValue(field, value string, files map[string]string) (string, bool) {
	id := resourceRef(field, value)
	if id == "" {
		return value, true
	}
	sha, ok := files[id]
	if !ok {
		return "", false
	}
	if bareIDFields[field] || strings.HasPrefix(strings.TrimSpace(value), "resource:") {
		return KeyPrefix + sha, true
	}
	return URLPrefix + sha, true
}

// fromSnapshotValue rewrites a placeholder to the importing workspace's resource.
func fromSnapshotValue(field, value string, resources map[string]string) (string, bool) {
	if !locatorFields[field] && !bareIDFields[field] {
		return value, true
	}
	switch {
	case strings.HasPrefix(value, URLPrefix):
		id, ok := resources[strings.TrimPrefix(value, URLPrefix)]
		if !ok {
			return "", false
		}
		return assets.FileURL(id), true
	case strings.HasPrefix(value, KeyPrefix):
		id, ok := resources[strings.TrimPrefix(value, KeyPrefix)]
		if !ok {
			return "", false
		}
		if bareIDFields[field] {
			return id, true
		}
		return "resource:" + id, true
	}
	return value, true
}

var (
	secretPattern = regexp.MustCompile(`\b(sk|pk|ak|api[_-]?key)[-_][A-Za-z0-9]{16,}`)
	phonePattern  = regexp.MustCompile(`(^|\D)1[3-9]\d{9}(\D|$)`)
)

// privacyHint flags free text that looks like a credential or phone number.
func privacyHint(value string) string {
	switch {
	case secretPattern.MatchString(value):
		return "疑似包含密钥"
	case phonePattern.MatchString(value):
		return "疑似包含手机号"
	}
	return ""
}
