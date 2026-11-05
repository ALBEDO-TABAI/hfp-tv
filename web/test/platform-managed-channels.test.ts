import { afterEach, expect, test } from "bun:test";
import { refreshOwnRelayBinding, refreshPlatformManagedChannels, resetPlatformManagedChannelRefresh } from "../src/services/platform-managed-channels";
import { apiClient } from "../src/services/api/request";
import { createModelChannel, defaultConfig, normalizeConfigSnapshot, useConfigStore } from "../src/stores/use-config-store";
import { usePlatformSession } from "../src/stores/use-platform-session";

const original = apiClient.defaults.adapter;
afterEach(() => {
    apiClient.defaults.adapter = original;
    resetPlatformManagedChannelRefresh();
    useConfigStore.getState().replaceConfig(defaultConfig);
    usePlatformSession.getState().setMe(null);
});

const relay = (hasApiKey: boolean, models: string[]) => ({ id: "company-relay", pinned: true, name: "公司中转站", baseUrl: "http://10.10.10.45:3000/v1", apiKey: "", hasApiKey, enabled: hasApiKey, models });

function serve(routes: Record<string, unknown>) {
    const calls: string[] = [];
    apiClient.defaults.adapter = async request => {
        calls.push(`${request.method} ${request.url}`);
        const data = routes[request.url || ""];
        return { config: request, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
    };
    return calls;
}

test("a key bound on the server reaches the page without touching the member's own channels", async () => {
    const own = createModelChannel({ id: "mine", name: "我的服务", baseUrl: "https://api.example.com/v1", apiKey: "typed-but-unsaved", models: ["m"] });
    useConfigStore.getState().replaceConfig({ ...defaultConfig, channels: [createModelChannel(relay(false, ["a"])), own] });
    const calls = serve({ "/workspace/model-config": { config: { ...defaultConfig, channels: [relay(true, ["a", "b"]), { ...own, apiKey: "stale-server-copy" }] }, revision: 3, health: "ready", source: "builtin+local" } });

    const fresh = await refreshPlatformManagedChannels();
    expect(fresh).toMatchObject({ id: "company-relay", hasApiKey: true, models: ["a", "b"] });
    const channels = useConfigStore.getState().config.channels;
    expect(channels.find(channel => channel.id === "company-relay")).toMatchObject({ hasApiKey: true, models: ["a", "b"] });
    expect(channels.find(channel => channel.id === "mine")?.apiKey).toBe("typed-but-unsaved");
    expect(calls).toEqual(["get /workspace/model-config"]);
});

test("an unchanged relay channel does not rewrite the configuration", async () => {
    const config = { ...defaultConfig, channels: [createModelChannel(relay(true, ["a"]))] };
    useConfigStore.getState().replaceConfig(config);
    serve({ "/workspace/model-config": { config, revision: 1, health: "ready", source: "builtin+local" } });
    const before = useConfigStore.getState().config;
    await refreshPlatformManagedChannels();
    expect(useConfigStore.getState().config).toBe(before);
});

test("a reply without a model configuration leaves the relay channel alone", async () => {
    const config = { ...defaultConfig, channels: [createModelChannel(relay(true, ["a"]))] };
    useConfigStore.getState().replaceConfig(config);
    apiClient.defaults.adapter = async request => ({ config: request, status: 200, statusText: "OK", headers: {}, data: "<html></html>" });
    await expect(refreshPlatformManagedChannels()).rejects.toThrow();
    serve({ "/workspace/model-config": { revision: 1 } });
    await expect(refreshPlatformManagedChannels()).rejects.toThrow("读取模型配置失败");
    expect(useConfigStore.getState().config.channels[0]).toMatchObject({ id: "company-relay", hasApiKey: true });
});

test("refreshing one's own binding updates the account state too", async () => {
    const me = { user: { id: "u1", username: "admin", displayName: "", role: "admin" }, workspace: { id: "w1", status: "active" }, relay: { bound: true, hint: "…0001" }, capabilities: {} };
    serve({ "/platform/me": me, "/workspace/model-config": { config: { ...defaultConfig, channels: [relay(true, ["a"])] }, revision: 1, health: "ready", source: "builtin+local" } });
    await refreshOwnRelayBinding();
    expect(usePlatformSession.getState().me?.relay.bound).toBe(true);
    expect(useConfigStore.getState().config.channels[0]).toMatchObject({ id: "company-relay", hasApiKey: true });
});

test("a selection of a removed company model follows the server's correction; other selections stay", async () => {
    const own = createModelChannel({ id: "mine", name: "我的服务", baseUrl: "https://api.example.com/v1", apiKey: "k", models: ["m"] });
    useConfigStore.getState().replaceConfig({ ...defaultConfig, channels: [createModelChannel(relay(true, ["old", "new"])), own], assistantModel: "company-relay::old", textModel: "mine::m" });
    serve({ "/workspace/model-config": { config: { ...defaultConfig, channels: [relay(true, ["new"]), own], assistantModel: "company-relay::new", textModel: "mine::m" }, revision: 7, health: "ready", source: "builtin+local" } });
    await refreshPlatformManagedChannels();
    const config = useConfigStore.getState().config;
    expect(config.assistantModel).toBe("company-relay::new");
    expect(config.textModel).toBe("mine::m");
});

test("a reply older than one already applied is ignored", async () => {
    useConfigStore.getState().replaceConfig({ ...defaultConfig, channels: [createModelChannel(relay(false, ["a"]))] });
    serve({ "/workspace/model-config": { config: { ...defaultConfig, channels: [relay(true, ["a"])] }, revision: 9, health: "ready", source: "builtin+local" } });
    await refreshPlatformManagedChannels();
    serve({ "/workspace/model-config": { config: { ...defaultConfig, channels: [relay(false, ["a"])] }, revision: 8, health: "ready", source: "builtin+local" } });
    await refreshPlatformManagedChannels();
    expect(useConfigStore.getState().config.channels[0]).toMatchObject({ id: "company-relay", hasApiKey: true });
});

test("a selection that cannot be corrected does not rewrite the configuration on every refresh", async () => {
    // The store always holds a normalized configuration (hydration normalizes it).
    const config = normalizeConfigSnapshot({ config: { ...defaultConfig, channels: [createModelChannel(relay(true, ["new"]))], assistantModel: "company-relay::old" } }).config;
    useConfigStore.getState().replaceConfig(config);
    // The server still reports the old choice (it was written back before the fix).
    serve({ "/workspace/model-config": { config: { ...config, channels: [relay(true, ["new"])] }, revision: 3, health: "ready", source: "builtin+local" } });
    const before = useConfigStore.getState().config;
    await refreshPlatformManagedChannels();
    expect(useConfigStore.getState().config).toBe(before);
});
