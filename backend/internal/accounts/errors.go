package accounts

import (
	"errors"
	"net/http"
)

// Error carries the HTTP status and machine-readable reason the team server
// returns in its { code, data, msg, reason } envelope.
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
	ErrUnauthenticated    = newError(http.StatusUnauthorized, "unauthenticated", "请先登录")
	ErrInvalidCredentials = newError(http.StatusUnauthorized, "invalid_credentials", "用户名或密码不正确")
	ErrTooManyAttempts    = newError(http.StatusTooManyRequests, "too_many_attempts", "尝试次数过多，请稍后再试")
	ErrForbidden          = newError(http.StatusForbidden, "forbidden", "需要管理员权限")
	ErrUserNotFound       = newError(http.StatusNotFound, "user_not_found", "用户不存在")
	ErrUsernameTaken      = newError(http.StatusConflict, "username_taken", "用户名已被使用")
	ErrInvalidUsername    = newError(http.StatusBadRequest, "invalid_username", "用户名需为 3–32 位字母、数字、下划线、点或短横线")
	ErrInvalidPassword    = newError(http.StatusBadRequest, "invalid_password", "密码至少 8 位")
	ErrInvalidRole        = newError(http.StatusBadRequest, "invalid_role", "角色无效")
	ErrInvalidStatus      = newError(http.StatusBadRequest, "invalid_status", "账号状态无效")
	ErrCannotModifySelf   = newError(http.StatusBadRequest, "cannot_modify_self", "不能停用自己或取消自己的管理员身份")
	ErrInvitationInvalid  = newError(http.StatusNotFound, "invitation_invalid", "邀请链接无效或已过期")
	ErrInvalidRelayKey    = newError(http.StatusBadRequest, "invalid_relay_key", "中转站密钥无效")
	ErrWorkspaceMissing   = newError(http.StatusConflict, "workspace_unavailable", "工作区不可用")
	ErrManagedByFile      = newError(http.StatusConflict, "account_managed_by_file", "这个账号的密码、密钥和启停由服务器上的账号文件管理，请修改账号文件")
)

// AsError maps any error to an accounts.Error; unknown errors become 500.
func AsError(err error) *Error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return newError(http.StatusInternalServerError, "internal_error", "服务器内部错误")
}
