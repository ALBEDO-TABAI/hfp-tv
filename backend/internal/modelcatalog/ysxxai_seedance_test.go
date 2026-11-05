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
