package accounts

import "net/http"

type Error struct {
	Status  int
	Reason  string
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrCredentials     = &Error{http.StatusUnauthorized, "invalid_credentials", "用户名或密码不正确"}
	ErrRateLimited     = &Error{http.StatusTooManyRequests, "login_rate_limited", "用户名或密码不正确"}
	ErrUnauthenticated = &Error{http.StatusUnauthorized, "unauthenticated", "请先登录"}
	ErrForbidden       = &Error{http.StatusForbidden, "admin_required", "需要管理员权限"}
	ErrInvalidInput    = &Error{http.StatusBadRequest, "invalid_input", "输入内容不正确"}
	ErrPassword        = &Error{http.StatusBadRequest, "invalid_password", "密码长度须为 12 至 1024 字节"}
	ErrUsernameTaken   = &Error{http.StatusConflict, "username_taken", "用户名已被使用"}
	ErrInvitation      = &Error{http.StatusBadRequest, "invalid_invitation", "邀请无效或已过期"}
	ErrNotFound        = &Error{http.StatusNotFound, "not_found", "未找到该资源"}
	ErrCSRF            = &Error{http.StatusForbidden, "csrf_rejected", "请求来源或请求头不符合要求"}
)
