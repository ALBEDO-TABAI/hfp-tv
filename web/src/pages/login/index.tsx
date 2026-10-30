import { useState } from "react";
import { Alert, Button, Form, Input } from "antd";
import { useSearchParams } from "react-router";
import { PlatformAuthCard } from "@/components/platform/auth-card";
import { platformErrorMessage, platformPasswordRules, platformUsernameRules } from "@/lib/platform-account";
import { safeNextPath } from "@/lib/platform-navigation";
import { loginPlatform } from "@/services/api/platform";

export default function LoginPage() {
    const [search] = useSearchParams();
    const [pending, setPending] = useState(false);
    const [error, setError] = useState("");
    const submit = async (values: { username: string; password: string }) => {
        setPending(true);
        setError("");
        try {
            await loginPlatform(values);
            window.location.assign(safeNextPath(search.get("next")));
        } catch (cause) {
            setError(platformErrorMessage(cause));
            setPending(false);
        }
    };
    return <PlatformAuthCard title="登录团队工作区">
        {error && <Alert type="error" showIcon title={error} className="mb-4" />}
        <Form layout="vertical" onFinish={submit} disabled={pending}>
            <Form.Item name="username" label="用户名" rules={platformUsernameRules}><Input autoComplete="username" autoFocus /></Form.Item>
            <Form.Item name="password" label="密码" rules={platformPasswordRules}><Input.Password autoComplete="current-password" /></Form.Item>
            <Button block type="primary" htmlType="submit" loading={pending}>登录</Button>
        </Form>
    </PlatformAuthCard>;
}
