package generation

import "strings"

var referencePreparationCopies = map[string]struct{ reason, action string }{
	"image_reference_upload_failed": {"参考图片上传失败，本次尚未开始生成", "请稍后重试"},
	"reference_media_upload_failed": {"参考素材上传失败，本次尚未开始生成", "请检查对象存储配置和公开读取权限后重试"},
	"reference_preupload_failed":    {"参考素材上传失败，本次尚未开始生成", "请检查网络或稍后重试"},
	"reference_relay_failed":        {"参考图没能通过公司参考图服务上传，本次尚未开始生成", "请稍后重试；仍然失败时请把排查编号发给管理员"},
	"reference_relay_unauthorized":  {"公司参考图服务不接受你的中转站密钥，本次尚未开始生成", "请联系管理员核对为你绑定的密钥后重试"},
	"reference_asset_failed":        {"参考图没能登记为虚拟人像素材，本次尚未开始生成", "原因见任务日志“上游原始错误”：审核未通过请换图，人物可拆成正面半身特写和全身正面两张；真人需本人完成认证授权；服务未配置或密钥问题请联系管理员"},
}

// ReferenceRelayError is a failed call to the team server's company
// reference-image service. The model has not been called yet. RequestID is
// the ID sent to that service, so an administrator can find it in its log.
type ReferenceRelayError struct {
	RequestID    string
	Unauthorized bool
	// AssetStage: the upload worked but asset registration or review failed.
	AssetStage bool
	Cause      error
}

func (e ReferenceRelayError) Error() string { return e.Cause.Error() }
func (e ReferenceRelayError) Unwrap() error { return e.Cause }

func referenceRelayFailure(relay ReferenceRelayError) Failure {
	code := "reference_relay_failed"
	if relay.Unauthorized {
		code = "reference_relay_unauthorized"
	} else if relay.AssetStage {
		code = "reference_asset_failed"
	}
	copy := referencePreparationCopies[code]
	failure := normalizeFailure(Failure{Category: CategoryProviderUnavailable, ProviderCode: code, Reason: copy.reason, Action: copy.action, RequestID: relay.RequestID})
	failure.Retryable = false
	return failure
}

// Only the complete application-owned copy restores pre-submission evidence.
// A provider's upload error or a bare machine code does not prove it was unsent.
func persistedReferencePreparationFailure(raw string) (Failure, bool) {
	text := strings.TrimSpace(raw)
	requestID, taskID := persistedReferenceIDs(text)
	for code, copy := range referencePreparationCopies {
		failure := Failure{Category: CategoryProviderUnavailable, ProviderCode: code,
			Reason: copy.reason, Action: copy.action, RequestID: requestID, TaskID: taskID, FromCode: true}
		message := failure.UserMessage()
		if text == message || text == strings.TrimSuffix(message, "。") {
			return failure, true
		}
	}
	return Failure{}, false
}

// Manual retry is a deliberate new submission; preparation failures never
// authorize the automatic submission loop to send the same request again.
func (f Failure) BlocksManualRetry() bool {
	copy, ok := referencePreparationCopies[f.ProviderCode]
	if ok && !f.Uncertain && f.Category == CategoryProviderUnavailable && f.Reason == copy.reason && f.Action == copy.action {
		return false
	}
	return f.BlocksAutomaticRetry()
}
