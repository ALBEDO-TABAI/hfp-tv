package generation

import (
	"regexp"
	"strings"
)

var imageTierPattern = regexp.MustCompile(`supports [124]k( and [124]k)*\b`)

func persistedTaskConstraintCopy(message string) (categoryCopy, bool) {
	for _, input := range []string{"video_first_frame_ratio_unreadable", "video_first_frame_ratio_unsupported", "video_first_frame_ratio_mismatch", "视频画幅仅支持 当前传入 adaptive", "image_urls is required", "pixel sizes are not supported", "gpt-image supports 2k and 4k", "TaskTypeConstraint ratio first-frame", "TaskTypeConstraint duration", "TaskTypeConstraint ratio", "TaskTypeConstraint"} {
		copy, _ := taskConstraintCopy(input)
		if strings.HasPrefix(strings.TrimSpace(message), copy.Reason+"。"+copy.Action) {
			return copy, true
		}
	}
	return categoryCopy{}, false
}

func taskConstraintCopy(message string) (categoryCopy, bool) {
	m := strings.ToLower(message)
	if strings.Contains(m, "video_first_frame_ratio_unreadable") {
		return categoryCopy{Reason: "无法读取首帧图片的尺寸", Action: "请重新上传 PNG、JPEG 或 WebP 图片后再生成"}, true
	}
	if strings.Contains(m, "video_first_frame_ratio_unsupported") {
		return categoryCopy{Reason: "当前渠道无法保持这张首帧图的比例", Action: "请将首帧调整为 1:1、4:3、3:4、9:16、16:9 或 21:9，或更换模型"}, true
	}
	if strings.Contains(m, "video_first_frame_ratio_mismatch") {
		return categoryCopy{Reason: "输出比例需要与首帧一致", Action: "请使用随首帧比例，或先调整首帧图片的尺寸"}, true
	}
	if strings.Contains(m, "视频画幅仅支持") && strings.Contains(m, "当前传入 adaptive") {
		return categoryCopy{Reason: "当前渠道暂不支持自适应画幅", Action: "首帧生成请更换模型；文生视频请指定固定比例后再提交"}, true
	}
	// Image models whose supplier differs from OpenAI Images (see
	// deploy/teamserver/ysxxai-image-task-plugin.js for the YSXXAI ones).
	if strings.Contains(m, "image_urls") && (strings.Contains(m, "required") || strings.Contains(m, "public links")) {
		return categoryCopy{Reason: "这个模型只接收参考图的链接", Action: "请管理员在模型设置的“参考素材传输”里，把图片改为“上传后以链接提交”后重试"}, true
	}
	if strings.Contains(m, "pixel sizes are not supported") || strings.Contains(m, "unsupported image aspect ratio") {
		return categoryCopy{Reason: "这个模型只接受画面比例，不接受像素尺寸", Action: "请联系管理员更新中转站的图片插件；也可以先把尺寸改为自动再生成"}, true
	}
	if imageTierPattern.MatchString(m) {
		return categoryCopy{Reason: "这个模型不支持所选的分辨率档位", Action: "请换一个分辨率档位（如 2K 或 4K）后重试"}, true
	}
	if !strings.Contains(m, "tasktypeconstraint") {
		return categoryCopy{}, false
	}
	if strings.Contains(m, "ratio") && (strings.Contains(m, "first-frame") || strings.Contains(m, "first_frame")) {
		return categoryCopy{Reason: "首尾帧模式的画面比例需跟随首帧", Action: "请选择自适应比例后重新生成；如需指定比例，请改用参考生成模式"}, true
	}
	if strings.Contains(m, "duration") {
		return categoryCopy{Reason: "当前任务模式不支持指定的视频时长", Action: "编辑视频时请使用跟随原视频的时长；如需生成指定时长，请改用参考生成模式"}, true
	}
	if strings.Contains(m, "ratio") {
		return categoryCopy{Reason: "当前任务模式的画面比例需跟随输入素材", Action: "请选择自适应比例后重新生成；如需指定比例，请改用参考生成模式"}, true
	}
	return categoryCopy{Reason: "生成参数与当前任务模式不兼容", Action: "请检查所选模式；首尾帧和延长需跟随素材比例，编辑还需跟随原视频时长"}, true
}
