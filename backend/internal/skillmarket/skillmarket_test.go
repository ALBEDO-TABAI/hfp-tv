package skillmarket

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/skillhub"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte(body))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

const validSkill = "---\nname: storyboard\ndescription: Split a script into shots\n---\nDo it."

func TestCanonicalizeRejectsScriptsAndAcceptsDataFiles(t *testing.T) {
	if _, err := canonicalize(zipOf(t, map[string]string{"SKILL.md": validSkill, "scripts/run.sh": "rm -rf /"})); err == nil {
		t.Fatal("script accepted")
	}
	if _, err := canonicalize(zipOf(t, map[string]string{"SKILL.md": "no front matter"})); err == nil {
		t.Fatal("SKILL.md without front matter accepted")
	}
	first, err := canonicalize(zipOf(t, map[string]string{"SKILL.md": validSkill, "references/rules.md": "rules", "__MACOSX/._x": "junk"}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalize(first.Data)
	if err != nil || !bytes.Equal(first.Data, second.Data) {
		t.Fatal("canonical archive is not stable")
	}
	if len(first.Files) != 2 || first.Files[0].Path != "SKILL.md" || first.ContentHash == "" {
		t.Fatalf("unexpected files %+v", first.Files)
	}
}

func TestSignedReleaseVerifiesAndTamperingFails(t *testing.T) {
	signer, err := LoadSigner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEEFTV_SKILLHUB_TRUSTED_KEYS", signer.TrustedKeysJSON())
	t.Setenv("BEEFTV_SKILLHUB_REQUIRE_SIGNATURE", "true")
	release := skillhub.Release{Metadata: skillhub.Metadata{SkillID: "s_1", Name: "分镜", Description: "d", License: "MIT", VersionLabel: "1.0.0",
		Requirements: []string{}, Showcase: []skillhub.Showcase{}}, ReleaseID: "r_1", Author: "alice", Files: []skillhub.File{}, DownloadURL: "skills/s_1/releases/r_1/archive"}
	payload, attestation, err := signer.Sign(release)
	if err != nil {
		t.Fatal(err)
	}
	// Serve it the way the market does: stored JSON plus attestation.
	stored := Release{ReleaseJSON: string(payload), AttestationJSON: `{"algorithm":"` + attestation.Algorithm + `","keyId":"` + attestation.KeyID + `","payload":"` + attestation.Payload + `","signature":"` + attestation.Signature + `"}`}
	served, err := (&Service{}).releaseOf(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := skillhub.VerifyRelease(&served); err != nil {
		t.Fatalf("market-signed release rejected: %v", err)
	}
	served.Author = "mallory"
	if err := skillhub.VerifyRelease(&served); err == nil {
		t.Fatal("tampered release accepted")
	}
	again, err := LoadSigner(strings.TrimSuffix(t.TempDir(), "/"))
	if err != nil || again.KeyID() == signer.KeyID() {
		t.Fatal("separate data roots must not share a signing key")
	}
}

func TestCompatibilityFollowsDeclaredCapabilities(t *testing.T) {
	if got := Compatibility([]string{"beeftv.canvas"}, []string{"beeftv.canvas"}); got.Status != "supported" {
		t.Fatal(got)
	}
	got := Compatibility([]string{"beeftv.canvas", "model.video"}, []string{"beeftv.canvas"})
	if got.Status != "unsupported" || len(got.Reasons) != 1 || got.Reasons[0] != "missing_capability:model.video" {
		t.Fatal(got)
	}
}
