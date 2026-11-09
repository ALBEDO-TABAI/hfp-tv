package modelcatalog

import "strings"

// IsYSXXAIVideoModel reports YSXXAI Kling 4.0 and Wan 3.0 on the company relay's "ysxxai-video" plugin.
// The plugin rewrites the generic video-generations-json body into each
// model's own request (deploy/teamserver/ysxxai-video-task-plugin.js); the
// limits below are the YSXXAI documents' published ones. Keep the same
// cases as web/src/lib/model-capabilities.ts.
func IsYSXXAIVideoModel(protocol, modelName string) bool {
	if strings.TrimSpace(protocol) != "video-generations-json" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "kling-4.0", "kling-4.0-flash", "wan3.0-video":
		return true
	}
	return false
}

func applyYSXXAIVideoCapability(video *VideoCapabilityConfig, modelName string) {
	name := strings.ToLower(strings.TrimSpace(modelName))
	video.Operations = []string{"text_to_video", "image_to_video", "reference_to_video"}
	video.Watermark = VideoBooleanConfig{Supported: true, Default: false}
	refs := &video.References
	refs.MaxImages = 10
	refs.MaxVideos = 5
	refs.MaxVideoBytes = 200 * 1024 * 1024
	if strings.HasPrefix(name, "kling-") {
		maxSeconds := 30
		if name == "kling-4.0-flash" {
			maxSeconds = 20
		}
		video.Duration = VideoDurationConfig{Selection: "range", Min: 3, Max: maxSeconds, Step: 1, Default: 5}
		video.Ratios, video.DefaultRatio = []string{"16:9", "9:16", "1:1", "21:9"}, "16:9"
		video.Resolutions, video.DefaultResolution = []string{"720p", "1080p"}, "720p"
		// Kling refuses generate_audio; its voices are voice IDs, not audio files.
		video.GenerateAudio = VideoBooleanConfig{Supported: false, Default: false}
		refs.MinVideoDuration, refs.MaxVideoDuration, refs.MaxVideoTotalDuration = 3, 30, 30
		refs.MaxAudios, refs.MaxAudioBytes, refs.MaxAudioDuration = 0, 0, 0
		return
	}
	video.Duration = VideoDurationConfig{Selection: "range", Min: 2, Max: 30, Step: 1, Default: 5}
	video.Ratios, video.DefaultRatio = []string{"adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}, "16:9"
	video.Resolutions, video.DefaultResolution = []string{"480p", "720p", "1080p"}, "720p"
	video.GenerateAudio = VideoBooleanConfig{Supported: true, Default: true}
	refs.MaxVideoDuration, refs.MaxVideoTotalDuration = 15, 15
	refs.MaxAudios = 5
	refs.MaxAudioBytes = 15 * 1024 * 1024
	refs.MaxAudioDuration = 15
}
