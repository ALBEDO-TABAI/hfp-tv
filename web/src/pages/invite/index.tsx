import { useState } from "react";
import { Alert, Button, Form, Input, Spin } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { PlatformAuthCard } from "@/components/platform/auth-card";
import { platformErrorMessage, platformPasswordRules, platformUsernameRules } from "@/lib/platform-account";
import { acceptPlatformInvitation, getPlatformInvitation } from "@/services/api/platform";
import { ApiError } from "@/services/api/request";

export default function InvitePage() {
    const { token = "" } = useParams();
    const invitation = useQuery({ queryKey: ["platform", "invitation", token], queryFn: () => getPlatformInvitation(token), retry: false });
    const [pending, setPending] = useState(false);
    const [error, setError] = useState("");
    const [invalid, setInvalid] = useState(false);
    const submit = async (values: { username: string; displayName: string; password: string }) => {
        setPending(true);
        setError("");
        try {
            await acceptPlatformInvitation(token, { username: values.username, displayName: values.displayName, password: values.password });
            window.location.assign("/");
        } catch (cause) {
            setInvalid(cause instanceof ApiError && cause.reason === "invitation_invalid");
            setError(platformErrorMessage(cause));
            setPending(false);
        }
    };
    const invitationInvalid = invalid || invitation.error instanceof ApiError && invitation.error.reason === "invitation_invalid";
    return <PlatformAuthCard title="加入团队工作区">
        {invitation.isPending ? <Spin /> : invitationInvalid ? <>
            <Alert type="warning" title="邀请已失效或过期，请联系管理员重新邀请" className="mb-4" />
            <Link to="/login">返回登录</Link>
        </> : invitation.isError ? <>
            <Alert type="error" title="无法读取邀请，请重试" className="mb-4" />
            <Button onClick={() => void invitation.refetch()}>重试</Button>
        </> : invitation.data && <>
            <p className="mb-4 text-sm" style={{ color: "var(--ink-2)" }}>{invitation.data.role === "admin" ? "管理员" : "成员"}邀请 · 有效至 {new Date(invitation.data.expiresAt).toLocaleString()}</p>
            {error && <Alert type="error" showIcon title={error} className="mb-4" />}
            <Form layout="vertical" onFinish={submit} disabled={pending}>
                <Form.Item name="username" label="用户名" rules={platformUsernameRules}><Input autoComplete="username" /></Form.Item>
                <Form.Item name="displayName" label="显示名称" rules={[{ required: true, whitespace: true, message: "请输入显示名称" }]}><Input autoComplete="name" /></Form.Item>
                <Form.Item name="password" label="密码" rules={platformPasswordRules}><Input.Password autoComplete="new-password" /></Form.Item>
                <Form.Item name="confirm" label="确认密码" dependencies={["password"]} rules={[{ required: true, message: "请再次输入密码" }, ({ getFieldValue }) => ({ validator: (_, value: string) => value === getFieldValue("password") ? Promise.resolve() : Promise.reject(new Error("两次密码不一致")) })]}><Input.Password autoComplete="new-password" /></Form.Item>
                <Button block type="primary" htmlType="submit" loading={pending}>创建账号并加入</Button>
            </Form>
        </>}
    </PlatformAuthCard>;
}
