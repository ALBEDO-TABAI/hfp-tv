import { Alert, Button, Descriptions, Skeleton, Tag } from "antd";
import { useQuery } from "@tanstack/react-query";
import { platformErrorMessage } from "@/lib/platform-account";
import { getPlatformServiceStatus } from "@/services/api/platform";

const capabilityLabels: Record<string, string> = { image: "图片", video: "视频", text: "文本", audio: "音频", "": "未标注能力" };
const cookieLabels = { auto: "自动（HTTPS 访问时加 Secure）", always: "始终 Secure（仅 HTTPS）", never: "不加 Secure（HTTP）" };

export default function AdminServicePage() {
    const status = useQuery({ queryKey: ["platform", "service-status"], queryFn: ({ signal }) => getPlatformServiceStatus(signal) });
    const data = status.data;

    return <main className="app-workspace-page flex h-full min-h-0 flex-col overflow-y-auto p-6" style={{ color: "var(--ink)" }}>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-medium">服务状态</h1>
            <Button loading={status.isFetching} onClick={() => void status.refetch()}>刷新</Button>
        </div>
        {status.isError && <Alert type="error" showIcon title={platformErrorMessage(status.error)} action={<Button onClick={() => void status.refetch()}>重试</Button>} className="mb-4" />}
        {status.isPending ? <Skeleton active /> : data && <div className="space-y-6" style={{ overflowWrap: "anywhere" }}>
            <section>
                <h2 className="mb-3 text-lg font-medium">公司模型服务</h2>
                {data.relay.configured ? <>
                    <Descriptions column={1} items={[
                        { key: "name", label: "名称", children: data.relay.name },
                        { key: "url", label: "服务地址", children: data.relay.baseUrl },
                        { key: "format", label: "接口格式", children: data.relay.apiFormat },
                        { key: "models", label: "模型", children: data.relay.models.length ? <div className="flex flex-wrap gap-2">{data.relay.models.map(({ model, capability }) => <Tag key={`${model}:${capability}`} className="whitespace-normal break-all">{model} · {capabilityLabels[capability] ?? capability}</Tag>)}</div> : "尚未配置模型" },
                        { key: "file", label: "渠道文件", children: data.relay.file },
                    ]} />
                    <p style={{ color: "var(--ink-2)" }}>成员的请求都发往这个地址，各自使用自己的中转密钥</p>
                    {data.relay.autoAllowedHost && <p className="mt-2">已自动放行此内网地址：{data.relay.autoAllowedHost}</p>}
                </> : <Alert type="info" showIcon title="服务器还没有配置公司模型服务" description={<>在 <code>deploy/teamserver/config/relay-channel.json</code> 写好渠道，并在 <code>.env</code> 设置 <code>TEAMSERVER_RELAY_CHANNEL_FILE=/config/relay-channel.json</code>，重启后生效。</>} />}
            </section>
            <section>
                <h2 className="mb-3 text-lg font-medium">成员与工作区</h2>
                <Descriptions column={1} items={[
                { key: "active", label: "启用成员数", children: data.members.active },
                { key: "keys", label: "已绑定密钥的成员数", children: data.members.withRelayKey },
                { key: "workspaces", label: "当前打开的工作区 / 总数", children: `${data.workspaces.open} / ${data.workspaces.total}` },
                { key: "idle", label: "空闲回收", children: data.idleTimeoutSeconds === 0 ? "未启用" : `空闲 ${Math.round(data.idleTimeoutSeconds / 60)} 分钟后自动关闭工作区，下次访问自动打开` },
                ]} />
            </section>
            <section>
                <h2 className="mb-3 text-lg font-medium">账号文件</h2>
                <Descriptions column={1} items={[
                { key: "users", label: "用户文件", children: data.accountFiles.usersFile || "未使用（在用户管理中维护账号）" },
                { key: "keys", label: "密钥文件", children: data.accountFiles.keysFile || "未使用（在用户管理中维护账号）" },
                ]} />
            </section>
            <section>
                <h2 className="mb-3 text-lg font-medium">内网访问</h2>
                {data.allowedPrivateHosts.length ? <div className="flex flex-wrap gap-2">{data.allowedPrivateHosts.map(host => <Tag key={host} className="whitespace-normal break-all">{host}</Tag>)}</div> : <p>未放行任何内网地址</p>}
            </section>
            <section>
                <h2 className="mb-3 text-lg font-medium">登录 Cookie</h2>
                <p>{cookieLabels[data.cookieSecure]}</p>
            </section>
        </div>}
    </main>;
}
