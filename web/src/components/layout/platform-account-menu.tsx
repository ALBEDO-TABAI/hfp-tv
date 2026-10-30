import { useState } from "react";
import { App, Avatar, Button, Dropdown, Form, Input } from "antd";
import { useNavigate } from "react-router";
import { AppModal } from "@/components/ui/product/app-modal";
import { platformErrorMessage, platformPasswordRules } from "@/lib/platform-account";
import { appQueryClient } from "@/lib/query-client";
import { changePlatformPassword, logoutPlatform } from "@/services/api/platform";
import { resetAgentUiSession } from "@/services/api/agent-assistant";
import { usePlatformSession } from "@/stores/use-platform-session";
import { useUserStore } from "@/stores/use-user-store";

export function PlatformAccountMenu({ collapsed }: { collapsed: boolean }) {
    const { mode, me } = usePlatformSession();
    const { message } = App.useApp();
    const navigate = useNavigate();
    const [open, setOpen] = useState(false);
    const [pending, setPending] = useState(false);
    const [form] = Form.useForm();
    if (mode !== "hosted" || !me) return null;
    const logout = async () => {
        setPending(true);
        try {
            await logoutPlatform();
            resetAgentUiSession();
            appQueryClient.clear();
            useUserStore.getState().clearSession();
            usePlatformSession.getState().setMe(null);
            window.location.assign("/login");
        } catch (error) {
            message.error(platformErrorMessage(error));
            setPending(false);
        }
    };
    const changePassword = async (values: { currentPassword: string; newPassword: string }) => {
        setPending(true);
        try {
            await changePlatformPassword({ currentPassword: values.currentPassword, newPassword: values.newPassword });
            message.success("密码已修改");
            setOpen(false);
            form.resetFields();
        } catch (error) {
            message.error(platformErrorMessage(error));
        } finally {
            setPending(false);
        }
    };
    return <>
        <Dropdown trigger={["click"]} menu={{ items: [
            { key: "password", label: "修改密码", onClick: () => setOpen(true) },
            ...(me.user.role === "admin" && me.capabilities.userAdmin ? [{ key: "users", label: "用户管理", onClick: () => navigate("/admin/users") }] : []),
            { key: "logout", label: "退出登录", disabled: pending, onClick: () => void logout() },
        ] }}>
            <button type="button" aria-label="账号菜单" disabled={pending} className="app-workspace-nav-link mt-3 flex min-h-11 w-full items-center gap-2 rounded-[var(--r-md)] p-2" title={me.user.displayName}>
                <Avatar size="small" style={{ background: "var(--fill-3)", color: "var(--ink)" }}>{(me.user.displayName || me.user.username).slice(0, 1).toUpperCase()}</Avatar>
                {!collapsed && <span className="truncate">{me.user.displayName || me.user.username}</span>}
            </button>
        </Dropdown>
        <AppModal flush open={open} footer={null} onCancel={() => { if (!pending) { setOpen(false); form.resetFields(); } }}>
            <div className="p-6">
                <h2 className="mb-5 text-lg font-medium">修改密码</h2>
                <Form form={form} layout="vertical" onFinish={changePassword} disabled={pending}>
                    <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true, message: "请输入当前密码" }]}><Input.Password autoComplete="current-password" /></Form.Item>
                    <Form.Item name="newPassword" label="新密码" rules={platformPasswordRules}><Input.Password autoComplete="new-password" /></Form.Item>
                    <Form.Item name="confirm" label="确认新密码" dependencies={["newPassword"]} rules={[{ required: true, message: "请再次输入新密码" }, ({ getFieldValue }) => ({ validator: (_, value: string) => value === getFieldValue("newPassword") ? Promise.resolve() : Promise.reject(new Error("两次密码不一致")) })]}><Input.Password autoComplete="new-password" /></Form.Item>
                    <Button type="primary" htmlType="submit" loading={pending}>保存密码</Button>
                </Form>
            </div>
        </AppModal>
    </>;
}
