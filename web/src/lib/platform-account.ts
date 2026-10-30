import { ApiError } from "@/services/api/request";

export const platformUsernameRules = [{ required: true, message: "请输入用户名" }, { pattern: /^[A-Za-z0-9_.-]{3,32}$/, message: "使用 3–32 位字母、数字、下划线、点或短横线" }];
export const platformPasswordRules = [{ required: true, message: "请输入密码" }, { min: 8, message: "密码至少 8 位" }];

export function platformErrorMessage(error: unknown) {
    const messages: Record<string, string> = {
        invalid_credentials: "用户名或密码不正确",
        too_many_attempts: "尝试次数过多，请稍后再试",
        unauthenticated: "请先登录",
        invitation_invalid: "邀请已失效或过期，请联系管理员重新邀请",
        username_taken: "用户名已被使用",
        invalid_username: "用户名须为 3–32 位字母、数字、下划线、点或短横线",
        invalid_password: "密码至少 8 位",
        cannot_modify_self: "不能停用自己或降低自己的权限",
        invalid_relay_key: "中转密钥无效，请检查后重试",
        forbidden: "没有操作权限",
        authorization_invalid: "授权码无效或已过期，请回到投稿页重新连接",
        submission_changed: "作者已更新稿件，请刷新后重新审核",
        submission_not_found: "稿件不存在，请刷新列表",
        reason_required: "请填写驳回原因",
        market_not_configured: "未配置技能市场。团队服务器部署会自动接入内置技能市场。",
    };
    return error instanceof ApiError && error.reason && messages[error.reason] || "操作失败，请稍后重试";
}
