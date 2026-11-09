package modelcatalog

import "testing"

func TestYSXXAISeedanceModelCapabilities(t *testing.T) {
	for _, tc := range []struct {
		model, resolution string
		references        bool
	}{
		{"doubao-seedance-2-5-260628-2-720P", "720p", true},
		{"doubao-seedance-2-5-260628-2-1080P", "1080p", true},
		{"doubao-seedance-2-0-260128-2-720P", "720p", true},
		{"doubao-seedance-2-0-260128-2-1080P", "1080p", true},
		{"doubao-seedance-2-0-260128-2-4k", "4k", true},
		{"doubao-seedance-2-0-fast-260128-2-720P", "720p", false},
		{"doubao-seedance-2-0-mini-260615-2-720P", "720p", false},
	} {
		profile := DefaultModelCapabilityConfigForModel("video-generations-json", tc.model).Video
		if profile.Duration.Min != 4 || profile.Duration.Max != 15 || len(profile.Resolutions) != 1 || profile.Resolutions[0] != tc.resolution || !profile.GenerateAudio.Supported || !profile.Watermark.Supported {
			t.Fatalf("%s: %#v", tc.model, profile)
		}
		if hasReferences := profile.References.MaxImages > 0 && profile.References.MaxVideos > 0 && profile.References.MaxAudios > 0; hasReferences != tc.references {
			t.Fatalf("%s: references = %#v", tc.model, profile.References)
		}
	}
}

func TestYSXXAIKlingAndWanCapabilities(t *testing.T) {
	kling := DefaultModelCapabilityConfigForModel("video-generations-json", "kling-4.0").Video
	if kling.Duration.Min != 3 || kling.Duration.Max != 30 || kling.GenerateAudio.Supported || kling.References.MaxAudios != 0 || kling.References.MaxImages != 10 || kling.References.MaxVideos != 5 || kling.References.MaxVideoTotalDuration != 30 {
		t.Fatalf("kling-4.0: %#v", kling)
	}
	for _, ratio := range kling.Ratios {
		if ratio == "adaptive" || ratio == "4:3" {
			t.Fatalf("kling-4.0 text-to-video does not take %s: %v", ratio, kling.Ratios)
		}
	}
	if flash := DefaultModelCapabilityConfigForModel("video-generations-json", "kling-4.0-flash").Video; flash.Duration.Max != 20 {
		t.Fatalf("kling-4.0-flash: %#v", flash.Duration)
	}
	wan := DefaultModelCapabilityConfigForModel("video-generations-json", "wan3.0-video").Video
	if wan.Duration.Min != 2 || wan.Duration.Max != 30 || !wan.GenerateAudio.Supported || wan.References.MaxAudios != 5 || wan.References.MaxVideoTotalDuration != 15 || len(wan.Resolutions) != 3 || wan.Resolutions[0] != "480p" {
		t.Fatalf("wan3.0-video: %#v", wan)
	}
	// Same model names on other protocols keep their own contracts.
	if other := DefaultModelCapabilityConfigForModel("newapi-channel-2", "wan3.0-video").Video; other.Duration.Max == 30 && other.References.MaxAudios == 5 {
		t.Fatalf("newapi-channel-2 wan3.0-video picked up the YSXXAI profile: %#v", other)
	}
}
