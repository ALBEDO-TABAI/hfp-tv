import { useState } from "react";
import { useParams, useSearchParams } from "react-router";
import { Alert, Button, Form, Input } from "antd";
import { HfpLogoMark, HfpWordmark } from "@/components/brand/brand-logo";
import { safeLoginRedirect } from "@/lib/team-auth";
import { ApiError } from "@/services/api/request";
import { PLATFORM_ERROR_REASONS as reasons } from "@/services/api/error-codes";
import { acceptPlatformInvitation, loginPlatform, type InvitationAcceptance } from "@/services/api/platform";
import { notifyTeamSessionChanged } from "@/services/team-session";
import "./auth.css";

export default function AuthPage() {
    const { token } = useParams();
    const [searchParams] = useSearchParams();
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [accepted, setAccepted] = useState(false);

    const submit = async (values: InvitationAcceptance) => {
        setBusy(true);
        setError("");
        try {
            if (token && !accepted) {
                await acceptPlatformInvitation(token, { username: values.username, display_name: values.display_name, password: values.password });
                setAccepted(true);
            }
            // 接受邀请只创建账号；登录接口才会签发 HttpOnly Cookie。
            await loginPlatform({ username: values.username, password: values.password });
            notifyTeamSessionChanged();
            window.location.replace(token ? "/" : safeLoginRedirect(searchParams.get("redirect")));
        } catch (cause) {
            const reason = cause instanceof ApiError ? cause.reason : undefined;
            if (reason === reasons.rateLimited || (cause instanceof ApiError && cause.status === 429)) setError("尝试次数过多，请稍后再试");
            else if (token && reason === reasons.invalidInvitation) setError("邀请无效或已过期，请联系管理员");
            else if (token && reason === reasons.usernameTaken) setError("用户名已被使用，请换一个用户名");
            else if (token && reason === reasons.invalidPassword) setError("密码不符合要求，请使用至少 12 个字符，且不超过 1024 字节");
            else if (token && reason !== reasons.invalidCredentials) setError("暂时无法进入工作区，请稍后重试");
            else setError("用户名或密码不正确");
        } finally {
            setBusy(false);
        }
    };

    return <main className="team-auth-page">
        <section className="team-auth-card" aria-labelledby="team-auth-title">
            <div className="team-auth-brand"><HfpLogoMark size={48} /><HfpWordmark /></div>
            <h1 id="team-auth-title">{token ? "设置密码" : "登录工作区"}</h1>
            {accepted ? <p role="status">账号已创建，点击下方按钮继续登录。</p> : null}
            {error ? <Alert type="error" title={error} showIcon role="alert" /> : null}
            <Form layout="vertical" onFinish={submit} requiredMark={false} disabled={busy}>
                <Form.Item name="username" label="用户名" rules={[{ required: true, whitespace: true, message: "请输入用户名" }]}>
                    <Input autoComplete="username" autoFocus disabled={accepted} />
                </Form.Item>
                {token ? <Form.Item name="display_name" label="显示名" rules={[{ required: true, whitespace: true, message: "请输入显示名" }]}>
                    <Input autoComplete="nickname" disabled={accepted} />
                </Form.Item> : null}
                <Form.Item name="password" label="密码" rules={[
                    { required: true, message: "请输入密码" },
                    ...(token ? [{ validator: (_: unknown, value: string) => !value || Array.from(value).length >= 12 ? Promise.resolve() : Promise.reject(new Error("密码至少需要 12 个字符")) }] : []),
                ]}>
                    <Input.Password autoComplete={token ? "new-password" : "current-password"} />
                </Form.Item>
                {token ? <Form.Item name="confirmPassword" label="再次输入密码" dependencies={["password"]} rules={[
                    { required: true, message: "请再次输入密码" },
                    ({ getFieldValue }) => ({ validator: (_, value) => !value || getFieldValue("password") === value ? Promise.resolve() : Promise.reject(new Error("两次输入的密码不一致")) }),
                ]}>
                    <Input.Password autoComplete="new-password" />
                </Form.Item> : null}
                <Button type="primary" htmlType="submit" block loading={busy}>{token && !accepted ? "创建账号并进入工作区" : "登录"}</Button>
            </Form>
        </section>
    </main>;
}
