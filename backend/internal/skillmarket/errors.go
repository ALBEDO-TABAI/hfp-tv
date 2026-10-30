package skillmarket

import (
	"errors"
	"net/http"
)

// Error carries an HTTP status and machine-readable reason. The SkillHub
// handler renders it as {success:false,reason,message}; platform routes use
// the usual {code,reason,msg} envelope.
type Error struct {
	Status  int
	Reason  string
	Message string
}

func (e *Error) Error() string { return e.Message }

func newError(status int, reason, message string) *Error {
	return &Error{Status: status, Reason: reason, Message: message}
}

var (
	ErrUnauthorized         = newError(http.StatusUnauthorized, "publishing_authorization_required", "请先连接技能投稿账号")
	ErrNotFound             = newError(http.StatusNotFound, "not_found", "技能或版本不存在")
	ErrSubmissionNotFound   = newError(http.StatusNotFound, "submission_not_found", "稿件不存在")
	ErrSkillNotOwned        = newError(http.StatusForbidden, "skill_not_owned", "只能更新自己发布的技能")
	ErrRevisionConflict     = newError(http.StatusConflict, "revision_conflict", "稿件已更新，请刷新后重试")
	ErrSubmissionLocked     = newError(http.StatusConflict, "submission_locked", "稿件正在审核或已发布，不能再修改")
	ErrContentMissing       = newError(http.StatusConflict, "content_missing", "稿件还没有上传内容")
	ErrPreviewChanged       = newError(http.StatusConflict, "preview_changed", "公开预览已变化，请重新检查后提交")
	ErrSubmissionChanged    = newError(http.StatusConflict, "submission_changed", "作者已更新稿件，请刷新后重新审核")
	ErrReasonRequired       = newError(http.StatusBadRequest, "reason_required", "请填写驳回原因")
	ErrAuthorizationInvalid = newError(http.StatusNotFound, "authorization_invalid", "授权请求无效或已过期")
	ErrAuthorizationPending = newError(http.StatusConflict, "authorization_pending", "等待在授权页确认")
	ErrAuthorizationExpired = newError(http.StatusBadRequest, "expired_token", "授权请求已过期，请重新连接")
	ErrIdempotencyConflict  = newError(http.StatusConflict, "idempotency_conflict", "重复请求的内容不一致")
)

func invalid(reason, message string) *Error { return newError(http.StatusBadRequest, reason, message) }

// AsError maps any error to a market error; unknown errors become 500.
func AsError(err error) *Error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return newError(http.StatusInternalServerError, "internal_error", "技能市场内部错误")
}
