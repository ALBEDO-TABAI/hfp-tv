import test from "node:test";
import assert from "node:assert/strict";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import AdminServicePage from "./index";
import type { PlatformServiceStatus } from "@/services/api/platform";

test("service page renders configured values and the unconfigured guidance", () => {
    const data: PlatformServiceStatus = {
        relay: { configured: true, name: "公司服务", baseUrl: "http://relay.internal", apiFormat: "openai", models: [{ model: "sample-image", capability: "image" }, { model: "unknown-model", capability: "" }], file: "/config/relay.json", autoAllowedHost: "relay.internal" },
        accountFiles: { usersFile: "/config/users.json", keysFile: "/config/keys.json" }, allowedPrivateHosts: ["relay.internal"], cookieSecure: "auto",
        idleTimeoutSeconds: 1800, workspaces: { open: 2, total: 5 }, members: { active: 4, withRelayKey: 3 },
    };
    const client = new QueryClient();
    const render = () => renderToStaticMarkup(<QueryClientProvider client={client}><AdminServicePage /></QueryClientProvider>);
    client.setQueryData(["platform", "service-status"], data);
    const html = render();
    for (const text of ["公司服务", "sample-image", "图片", "unknown-model", "未标注能力", "/config/users.json", "/config/keys.json", "空闲 30 分钟", "2 / 5", "各自使用自己的中转密钥", "已自动放行此内网地址", "自动（HTTPS 访问时加 Secure）"]) assert.ok(html.includes(text), text);
    client.setQueryData(["platform", "service-status"], { ...data, relay: { ...data.relay, configured: false }, accountFiles: { usersFile: "", keysFile: "" }, allowedPrivateHosts: [], idleTimeoutSeconds: 0 });
    const unconfigured = render();
    for (const text of ["TEAMSERVER_RELAY_CHANNEL_FILE=/config/relay-channel.json", "未使用（在用户管理中维护账号）", "未放行任何内网地址", "未启用"]) assert.ok(unconfigured.includes(text), text);
    assert.ok(!unconfigured.includes("sample-image"));
    for (const [cookieSecure, label] of [["always", "始终 Secure（仅 HTTPS）"], ["never", "不加 Secure（HTTP）"]] as const) {
        client.setQueryData(["platform", "service-status"], { ...data, cookieSecure });
        assert.ok(render().includes(label));
    }
    client.clear();
});

test("service page shows loading and a retry after a failed request", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false } } });
    const render = () => renderToStaticMarkup(<QueryClientProvider client={client}><AdminServicePage /></QueryClientProvider>);
    assert.ok(render().includes("ant-skeleton"));
    await assert.rejects(client.fetchQuery({ queryKey: ["platform", "service-status"], queryFn: () => Promise.reject(new Error("offline")) }), /offline/);
    const html = render();
    assert.ok(html.includes("重试"));
    assert.ok(html.includes("操作失败，请稍后重试"));
    client.clear();
});
