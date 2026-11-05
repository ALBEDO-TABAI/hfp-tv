package skillhub

import (
	"strings"
	"testing"
)

const colonHeader = "---\nname: xhs-seed-shot\ndescription: Generate prompts for seeding photos: natural iPhone realism, soft light.\ncategory: media\nsymbolName: camera.fill\nseedVersion: 1\n---\n\n# xhs-seed-shot\n\nBody: keeps its colon.\n"

func TestFrontMatterWithAColonInAValueIsRepairedByQuotingOnly(t *testing.T) {
	if _, err := SkillName([]byte(colonHeader)); err == nil || !strings.Contains(err.Error(), "[2:") || !strings.Contains(err.Error(), "双引号") {
		t.Fatalf("invalid header error = %v", err)
	}
	repaired, changed := RepairFrontMatter([]byte(colonHeader))
	if !changed {
		t.Fatal("header was not repaired")
	}
	name, err := SkillName(repaired)
	if err != nil || name != "xhs-seed-shot" {
		t.Fatalf("repaired header = %q %v\n%s", name, err, repaired)
	}
	want := "description: \"Generate prompts for seeding photos: natural iPhone realism, soft light.\"\n"
	if !strings.Contains(string(repaired), want) || !strings.Contains(string(repaired), "category: media\n") || !strings.HasSuffix(string(repaired), "Body: keeps its colon.\n") {
		t.Fatalf("repair changed more than the quoting:\n%s", repaired)
	}
}

func TestValidOrUnrepairableFrontMatterIsLeftAlone(t *testing.T) {
	valid := "---\nname: ok-skill\ndescription: \"Already: quoted\"\nmetadata:\n  version: 2\n---\nbody\n"
	if out, changed := RepairFrontMatter([]byte(valid)); changed || string(out) != valid {
		t.Fatal("valid header was rewritten")
	}
	broken := "---\nname: [unclosed\ndescription: fine\n---\nbody\n"
	if _, changed := RepairFrontMatter([]byte(broken)); changed {
		t.Fatal("a value that is not a plain scalar was rewritten")
	}
	if _, err := SkillName([]byte(broken)); err == nil || !strings.Contains(err.Error(), "不是有效的 YAML") {
		t.Fatalf("broken header error = %v", err)
	}
}
