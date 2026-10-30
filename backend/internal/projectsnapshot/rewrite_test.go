package projectsnapshot

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotRewriteRoundTripsEveryReferenceForm(t *testing.T) {
	sha := strings.Repeat("a", 64)
	document := map[string]any{
		"nodes": []any{map[string]any{"id": "n", "metadata": map[string]any{
			"storageKey": "resource:r1", "content": "/api/resources/r1/file?x=1", "assetId": "a1", "taskId": "t1",
			"referenceResourceIds": []any{"r1"}, "prompt": "keep /api/resources/r1/file in prose",
			"videoPreview": map[string]any{"storageKey": "resource:r1"},
		}}},
	}
	exported := walkStrings(document, "", func(field, value string) (string, bool) {
		return toSnapshotValue(field, value, map[string]string{"r1": sha})
	}).(map[string]any)
	raw, _ := json.Marshal(exported)
	text := string(raw)
	for _, want := range []string{`"storageKey":"workshop:` + sha, `"content":"workshop://` + sha, `"referenceResourceIds":["workshop:` + sha, "keep /api/resources/r1/file in prose"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
	if strings.Contains(text, "assetId") || strings.Contains(text, "taskId") {
		t.Fatalf("workspace-local fields survived: %s", text)
	}
	imported := walkStrings(exported, "", func(field, value string) (string, bool) {
		return fromSnapshotValue(field, value, map[string]string{sha: "new"})
	})
	raw, _ = json.Marshal(imported)
	text = string(raw)
	for _, want := range []string{`"storageKey":"resource:new"`, `"content":"/api/resources/new/file"`, `"referenceResourceIds":["new"]`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

func TestSnapshotValidateRejectsForeignFormats(t *testing.T) {
	if Validate(&Snapshot{Format: "other", Version: 1}) == nil {
		t.Fatal("foreign format accepted")
	}
	if Validate(&Snapshot{Format: Format, Version: Version}) == nil {
		t.Fatal("empty snapshot accepted")
	}
	ok := &Snapshot{Format: Format, Version: Version, Canvases: []CanvasSnapshot{{Key: "c0"}}, Files: []File{{SHA256: strings.Repeat("b", 64)}}}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	ok.Files[0].SHA256 = "../../etc/passwd"
	if Validate(ok) == nil {
		t.Fatal("unsafe file id accepted")
	}
}
