import { useState } from "react";
import { Alert, App, Button, Form, Input, InputNumber, Popconfirm, Select, Space, Table, Tabs, Tag, Typography } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AppModal } from "@/components/ui/product/app-modal";
import { platformErrorMessage, platformPasswordRules, platformUsernameRules } from "@/lib/platform-account";
import {
    bindPlatformRelayKey, clearPlatformRelayKey, createPlatformInvitation, createPlatformUser,
    listPlatformAudit, listPlatformUsers, resetPlatformUserPassword, updatePlatformUser,
    type AdminUser, type PlatformRole,
} from "@/services/api/platform";
import { usePlatformSession } from "@/stores/use-platform-session";

type Editor = { kind: "create" | "invite" } | { kind: "password" | "relay"; user: AdminUser };
type FormValues = { username: string; displayName: string; password: string; role: PlatformRole; relayKey?: string; key: string; expiresInHours: number };
const roles = [{ value: "member", label: "成员" }, { value: "admin", label: "管理员" }];
const usersKey = ["platform", "users"];
const auditKey = ["platform", "audit"];
const time = (value: string | null) => value ? new Date(value).toLocaleString() : "尚未登录";

export default function AdminUsersPage() {
    const { message } = App.useApp();
    const client = useQueryClient();
    const me = usePlatformSession((state) => state.me);
    const [tab, setTab] = useState("users");
    const [editor, setEditor] = useState<Editor | null>(null);
    const [invitation, setInvitation] = useState<{ url: string; expiresAt: string } | null>(null);
    const [form] = Form.useForm<FormValues>();
    const users = useQuery({ queryKey: usersKey, queryFn: listPlatformUsers });
    const audit = useQuery({ queryKey: auditKey, queryFn: listPlatformAudit, enabled: tab === "audit" });
    const refresh = async () => {
        await Promise.all([client.invalidateQueries({ queryKey: usersKey }), client.invalidateQueries({ queryKey: auditKey })]);
    };
    const close = () => { setEditor(null); setInvitation(null); form.resetFields(); };
    const mutation = useMutation({
        mutationFn: (operation: () => Promise<unknown>) => operation(),
        onSuccess: async () => { message.success("已保存"); await refresh(); },
        onError: (error) => message.error(platformErrorMessage(error)),
    });
    const formMutation = useMutation({
        mutationFn: async (values: FormValues) => {
            if (!editor) throw new Error("请选择操作");
            switch (editor.kind) {
                case "create":
                    await createPlatformUser({ username: values.username, displayName: values.displayName, password: values.password, role: values.role, ...(values.relayKey?.trim() ? { relayKey: values.relayKey.trim() } : {}) });
                    return null;
                case "password":
                    await resetPlatformUserPassword(editor.user.id, values.password);
                    return null;
                case "relay":
                    await bindPlatformRelayKey(editor.user.id, values.key.trim());
                    return null;
                case "invite":
                    return createPlatformInvitation({ role: values.role, expiresInHours: values.expiresInHours });
            }
        },
        onSuccess: async (result) => {
            form.resetFields();
            if (result) setInvitation({ url: window.location.origin + result.url, expiresAt: result.expiresAt });
            else { close(); message.success("已保存"); }
            await refresh();
        },
        onError: (error) => message.error(platformErrorMessage(error)),
    });
    const busy = mutation.isPending || formMutation.isPending;
    const openEditor = (next: Editor) => { form.resetFields(); setInvitation(null); setEditor(next); };
    const title = editor?.kind === "create" ? "创建用户" : editor?.kind === "invite" ? "创建邀请" : editor?.kind === "password" ? `重置密码 · ${editor.user.username}` : editor?.kind === "relay" ? `中转密钥 · ${editor.user.username}` : "";

    return <main className="app-workspace-page flex h-full min-h-0 flex-col overflow-y-auto p-6" style={{ color: "var(--ink)" }}>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-medium">用户管理</h1>
            <Space><Button onClick={() => openEditor({ kind: "invite" })} disabled={busy}>创建邀请</Button><Button type="primary" onClick={() => openEditor({ kind: "create" })} disabled={busy}>创建用户</Button></Space>
        </div>
        <Tabs activeKey={tab} onChange={setTab} items={[
            { key: "users", label: "用户", children: <>
                {users.isError && <Alert type="error" showIcon title={platformErrorMessage(users.error)} action={<Button onClick={() => void users.refetch()}>重试</Button>} className="mb-4" />}
                <Table<AdminUser> rowKey="id" dataSource={users.data?.items} loading={users.isFetching} scroll={{ x: 1050 }} pagination={{ pageSize: 20 }} columns={[
                    { title: "用户名", key: "username", render: (_, user) => <Space size={6}>{user.username}{user.source === "file" && <Tag title="密码、密钥和启停由服务器上的账号文件管理">账号文件</Tag>}</Space> },
                    { title: "显示名称", dataIndex: "displayName" },
                    { title: "角色", key: "role", render: (_, user) => user.role === "admin" ? "管理员" : "成员" },
                    { title: "状态", key: "status", render: (_, user) => user.status === "active" ? "启用" : "停用" },
                    { title: "中转密钥", key: "relay", render: (_, user) => user.relay.bound ? user.relay.hint || "等待绑定" : "未绑定" },
                    { title: "最近活跃", key: "lastActiveAt", render: (_, user) => time(user.lastActiveAt) },
                    { title: "操作", key: "actions", render: (_, user) => {
                        // Account-file users change password, key and status only in the file.
                        const fileManaged = user.source === "file";
                        const fileHint = fileManaged ? "请修改服务器上的账号文件" : undefined;
                        return <Space wrap size="small">
                        <Button size="small" title={fileHint} disabled={busy || fileManaged} onClick={() => openEditor({ kind: "password", user })}>重置密码</Button>
                        <Button size="small" title={fileHint} disabled={busy || fileManaged} onClick={() => openEditor({ kind: "relay", user })}>{user.relay.bound ? "更换密钥" : "绑定密钥"}</Button>
                        <Popconfirm title={`确定${user.status === "active" ? "停用" : "启用"} ${user.username}？`} onConfirm={() => mutation.mutateAsync(() => updatePlatformUser(user.id, { status: user.status === "active" ? "disabled" : "active" }))}>
                            <Button size="small" title={fileHint} disabled={busy || fileManaged || user.id === me?.user.id}>{user.status === "active" ? "停用" : "启用"}</Button>
                        </Popconfirm>
                        <Popconfirm title={`将 ${user.username} 改为${user.role === "admin" ? "成员" : "管理员"}？`} onConfirm={() => mutation.mutateAsync(() => updatePlatformUser(user.id, { role: user.role === "admin" ? "member" : "admin" }))}>
                            <Button size="small" disabled={busy || user.id === me?.user.id}>改为{user.role === "admin" ? "成员" : "管理员"}</Button>
                        </Popconfirm>
                    </Space>;
                    } },
                ]} />
            </> },
            { key: "audit", label: "操作记录", children: <>
                {audit.isError && <Alert type="error" showIcon title={platformErrorMessage(audit.error)} action={<Button onClick={() => void audit.refetch()}>重试</Button>} className="mb-4" />}
                <Table rowKey="id" dataSource={audit.data?.items} loading={audit.isFetching} scroll={{ x: 700 }} pagination={{ pageSize: 20 }} columns={[
                    { title: "时间", dataIndex: "createdAt", render: time },
                    { title: "操作人", dataIndex: "actorUsername" },
                    { title: "操作", dataIndex: "action" },
                    { title: "对象", dataIndex: "targetUsername" },
                    { title: "详情", dataIndex: "detail" },
                ]} />
            </> },
        ]} />
        <AppModal flush open={editor !== null} footer={null} onCancel={() => { if (!busy) close(); }}>
            <div className="p-6">
                <h2 className="mb-5 text-lg font-medium">{title}</h2>
                {invitation ? <div className="space-y-4">
                    <p>有效至 {time(invitation.expiresAt)}</p>
                    <Typography.Paragraph copyable={{ text: invitation.url }} style={{ overflowWrap: "anywhere" }}>{invitation.url}</Typography.Paragraph>
                    <Button onClick={close}>完成</Button>
                </div> : <Form form={form} layout="vertical" initialValues={{ role: "member", expiresInHours: 24 }} disabled={busy} onFinish={(values) => formMutation.mutate(values)}>
                    {editor?.kind === "create" && <>
                        <Form.Item name="username" label="用户名" rules={platformUsernameRules}><Input autoComplete="off" /></Form.Item>
                        <Form.Item name="displayName" label="显示名称" rules={[{ required: true, whitespace: true, message: "请输入显示名称" }]}><Input /></Form.Item>
                    </>}
                    {(editor?.kind === "create" || editor?.kind === "password") && <Form.Item name="password" label={editor.kind === "create" ? "初始密码" : "新密码"} rules={platformPasswordRules}><Input.Password autoComplete="new-password" /></Form.Item>}
                    {(editor?.kind === "create" || editor?.kind === "invite") && <Form.Item name="role" label="角色" rules={[{ required: true }]}><Select options={roles} /></Form.Item>}
                    {editor?.kind === "create" && <Form.Item name="relayKey" label="中转密钥（选填）"><Input.Password autoComplete="off" visibilityToggle={false} /></Form.Item>}
                    {editor?.kind === "invite" && <Form.Item name="expiresInHours" label="有效时长（小时）" rules={[{ required: true, type: "integer", min: 1, message: "请输入至少 1 小时的整数" }]}><InputNumber min={1} precision={0} /></Form.Item>}
                    {editor?.kind === "relay" && <>
                        <p className="mb-4">当前：{editor.user.relay.bound ? editor.user.relay.hint || "等待绑定" : "未绑定"}</p>
                        <Form.Item name="key" label="新密钥" rules={[{ required: true, whitespace: true, message: "请输入密钥" }]}><Input.Password autoComplete="off" visibilityToggle={false} /></Form.Item>
                    </>}
                    <Space>
                        <Button type="primary" htmlType="submit" loading={formMutation.isPending}>{editor?.kind === "invite" ? "生成邀请链接" : "保存"}</Button>
                        {editor?.kind === "relay" && editor.user.relay.bound && <Popconfirm title="确定清除中转密钥？" onConfirm={() => mutation.mutateAsync(() => clearPlatformRelayKey(editor.user.id)).then(close)}><Button danger disabled={busy}>清除密钥</Button></Popconfirm>}
                        <Button onClick={close}>取消</Button>
                    </Space>
                </Form>}
            </div>
        </AppModal>
    </main>;
}
