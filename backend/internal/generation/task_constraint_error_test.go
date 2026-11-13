package generation

import "testing"

func TestFirstFrameChannelErrorsSurvivePersistence(t *testing.T) {
	for _, code := range []string{"video_first_frame_ratio_unreadable", "video_first_frame_ratio_unsupported", "video_first_frame_ratio_mismatch", "视频画幅仅支持：[1:1]，当前传入 adaptive"} {
		f := ClassifyText(code)
		if f.Category != CategoryInvalidParams || f.Reason == "模型不接受当前参数" {
			t.Fatalf("unspecific error: %#v", f)
		}
		persisted := ClassifyText(f.UserMessage())
		if persisted.Reason != f.Reason || persisted.Action != f.Action {
			t.Fatalf("persistence lost copy: %#v", persisted)
		}
	}
}

func TestNestedTaskTypeConstraint(t *testing.T) {
	raw := `{"error":{"code":"upstream_error","message":"{\"code\":\"fail_to_fetch_task\",\"message\":\"InvalidParameter.TaskTypeConstraint: The parameter ratio specified in the request is not valid. For first-frame or first-last-frame generation, the output ratio follows the first-frame image\"}"}}`
	got := ClassifyText(raw)
	got.RequestID = "req_task_constraint_123"
	if got.Category != CategoryInvalidParams || got.Reason != "首尾帧模式的画面比例需跟随首帧" {
		t.Fatalf("unexpected classification: %#v", got)
	}
	readback := ClassifyText(got.UserMessage())
	if readback.Category != got.Category || readback.Reason != got.Reason || readback.RequestID != got.RequestID {
		t.Fatalf("persisted copy lost: %#v", readback)
	}
}

// The real replies the company relay returned for YSXXAI image models.
func TestImageSupplierConstraintsExplainThemselves(t *testing.T) {
	cases := map[string]string{
		`status_code=400, {"error":{"message":"image_urls is required for image edits (request id: 2026)","type":"gateway_error","code":"invalid_request"}}`:                                         "这个模型只接收参考图的链接",
		`{"error":{"code":"fail_to_fetch_task","message":"{\"error\":{\"message\":\"size must be auto or one of 1:1, 16:9; pixel sizes are not supported\"}}"}}`:                                     "这个模型只接受画面比例，不接受像素尺寸",
		`{"error":{"code":"fail_to_fetch_task","message":"{\"error\":{\"message\":\"unsupported image aspect ratio \\\"1024x1024\\\", gemini-nano-banana-2.1-ext supported ratios: 16:9, 1:1\"}}"}}`: "这个模型只接受画面比例，不接受像素尺寸",
		`{"error":{"message":"gpt-image-2.5 supports 2k and 4k (request id: 2026)","type":"new_api_error"}}`:                                                                                         "这个模型不支持所选的分辨率档位",
	}
	for raw, reason := range cases {
		got := ClassifyText(raw)
		if got.Category != CategoryInvalidParams || got.Reason != reason {
			t.Fatalf("%s => %#v", raw, got)
		}
		if readback := ClassifyText(got.UserMessage()); readback.Reason != reason || readback.Action != got.Action {
			t.Fatalf("persistence lost copy: %#v", readback)
		}
	}
}
