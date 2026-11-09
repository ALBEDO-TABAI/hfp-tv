import { expect, test } from "bun:test";
import { companyRelayChannelFromTemplate, companyRelayModelEndpoint, companyRelaySharedEndpoints, companyRelayTemplateFromChannel } from "../src/lib/company-relay-template";
import { testChannelModelConnection } from "../src/lib/model-connection-test";
import { missingGenerationCredentialMessage } from "../src/stores/use-config-store";
import { getPlatformRelayChannel, resetPlatformRelayChannel, savePlatformRelayChannel, type RelayChannelTemplate } from "../src/services/api/platform";
import { apiClient, ApiError } from "../src/services/api/request";

const template: RelayChannelTemplate = {
    name: "公司中转站", baseUrl: "http://10.10.10.45:3000/v1", apiFormat: "openai",
    models: ["seedance-2.5", "text"],
    modelProfiles: [
        { model: "seedance-2.5", capability: "video", protocol: "volcengine-ark-video", baseUrl: "http://10.10.10.45:3000/doubao/api/v3", referenceAssetService: "http://10.10.10.45:8787/v1/reference-images" },
        { model: "text", capability: "text", protocol: "chat-completion", baseUrl: "http://10.10.10.45:3000/text", referenceAssetService: "http://10.10.10.45:8787/text" },
    ],
};

test("empty company draft has a managed identity, no credentials and no models", () => {
    const draft = companyRelayChannelFromTemplate(null);
    expect(draft).toMatchObject({ id: "company-relay", pinned: true, name: "公司中转站", baseUrl: "", apiFormat: "openai", apiKey: "", models: [], modelProfiles: [] });
    expect(draft).not.toHaveProperty("secretKey");
    expect(draft).not.toHaveProperty("headers");
    expect(companyRelaySharedEndpoints(draft)).toEqual({ arkBaseUrl: "", referenceAssetService: "", ysxxaiVideoBaseUrl: "" });
});

test("Seedance endpoints survive the editor round trip and new Ark models inherit shared settings", () => {
    const draft = companyRelayChannelFromTemplate(template);
    const shared = companyRelaySharedEndpoints(draft);
    expect(shared).toEqual({ arkBaseUrl: template.modelProfiles[0].baseUrl!, referenceAssetService: template.modelProfiles[0].referenceAssetService!, ysxxaiVideoBaseUrl: "" });
    expect(companyRelayTemplateFromChannel(draft, shared)).toEqual(template);
    draft.models.push("new-video", "seedream");
    draft.modelProfiles!.push({ model: "new-video", capability: "video", protocol: "volcengine-ark-video" }, { model: "seedream", capability: "image", protocol: "volcengine-ark-image" }, { model: "removed", capability: "text", protocol: "chat-completion" });
    const result = companyRelayTemplateFromChannel(draft, shared);
    expect(result.modelProfiles.find(profile => profile.model === "new-video")).toMatchObject({ baseUrl: shared.arkBaseUrl, referenceAssetService: shared.referenceAssetService });
    expect(result.modelProfiles.find(profile => profile.model === "seedream")).toMatchObject({ baseUrl: shared.arkBaseUrl });
    expect(result.modelProfiles.find(profile => profile.model === "seedream")).not.toHaveProperty("referenceAssetService");
    expect(result.modelProfiles.find(profile => profile.model === "text")).toEqual(template.modelProfiles[1]);
    expect(result.modelProfiles.some(profile => profile.model === "removed")).toBe(false);
    expect(template.models).toEqual(["seedance-2.5", "text"]);
});

test("shared settings read the first Ark endpoint and first Ark video reference service independently", () => {
    const draft = companyRelayChannelFromTemplate({ ...template, modelProfiles: [
        { model: "seedream", capability: "image", protocol: "volcengine-ark-image", baseUrl: "http://10.10.10.45:3000/image", referenceAssetService: "ignored" },
        ...template.modelProfiles,
    ] });
    expect(companyRelaySharedEndpoints(draft)).toEqual({ arkBaseUrl: "http://10.10.10.45:3000/image", referenceAssetService: template.modelProfiles[0].referenceAssetService!, ysxxaiVideoBaseUrl: "" });
});

test("a model with its own Ark endpoint keeps it; the shared field moves only the models that followed it", () => {
    const mixed: RelayChannelTemplate = { ...template, models: ["seedream", "seedance-2.5"], modelProfiles: [
        { model: "seedream", capability: "image", protocol: "volcengine-ark-image", baseUrl: "http://10.10.10.45:3000/image" },
        template.modelProfiles[0],
    ] };
    const draft = companyRelayChannelFromTemplate(mixed, { hasApiKey: true });
    const opening = companyRelaySharedEndpoints(draft);
    expect(opening.arkBaseUrl).toBe("http://10.10.10.45:3000/image");
    // Unchanged: saving keeps both endpoints, so nothing is reported as an unsaved address change.
    const unchanged = companyRelayTemplateFromChannel(draft, opening, opening);
    expect(unchanged).toEqual(mixed);
    for (const model of mixed.models) expect(companyRelayModelEndpoint(unchanged, model)).toBe(companyRelayModelEndpoint(mixed, model));
    // Changing the field moves the model that used it and new Ark models, not the one with its own endpoint.
    draft.models.push("new-video");
    draft.modelProfiles!.push({ model: "new-video", capability: "video", protocol: "volcengine-ark-video" });
    const moved = companyRelayTemplateFromChannel(draft, { ...opening, arkBaseUrl: "http://10.10.10.45:3000/ark" }, opening);
    expect(moved.modelProfiles.map(profile => profile.baseUrl)).toEqual(["http://10.10.10.45:3000/ark", "http://10.10.10.45:3000/doubao/api/v3", "http://10.10.10.45:3000/ark"]);
    expect(moved.modelProfiles[2].referenceAssetService).toBe(template.modelProfiles[0].referenceAssetService);
});

test("clearing shared endpoints removes only matching fields and output contains only template fields", () => {
    const draft = { ...companyRelayChannelFromTemplate(template), name: "   ", baseUrl: " http://10.10.10.45:3000/v1/// ", apiKey: "do-not-send", secretKey: "do-not-send", hasApiKey: true, headers: [{ name: "Authorization", value: "do-not-send" }] };
    const result = companyRelayTemplateFromChannel(draft, { arkBaseUrl: " ", referenceAssetService: "", ysxxaiVideoBaseUrl: "" });
    expect(Object.keys(result).sort()).toEqual(["apiFormat", "baseUrl", "modelProfiles", "models", "name"]);
    expect(result.name).toBe("公司中转站");
    expect(result.baseUrl).toBe(template.baseUrl);
    expect(result.modelProfiles[0]).not.toHaveProperty("baseUrl");
    expect(result.modelProfiles[0]).not.toHaveProperty("referenceAssetService");
    expect(result.modelProfiles[1]).toEqual(template.modelProfiles[1]);
    expect(JSON.stringify(result)).not.toContain("do-not-send");
    expect(companyRelaySharedEndpoints(draft).arkBaseUrl).toBe(template.modelProfiles[0].baseUrl!);
    expect(companyRelayTemplateFromChannel({ ...draft, name: " 公司模型 ", apiFormat: "gemini" }, { arkBaseUrl: "", referenceAssetService: "", ysxxaiVideoBaseUrl: "" })).toMatchObject({ name: "公司模型", apiFormat: "gemini" });
});

test("an admin with a bound key can test the draft; the server fills the key by channel id", async () => {
    const unbound = companyRelayChannelFromTemplate(template);
    await expect(testChannelModelConnection(unbound, "text", "text", "chat-completion", async () => ({ text: "OK" }))).rejects.toThrow("公司中转站还没有为你绑定密钥");
    expect(missingGenerationCredentialMessage(unbound)).not.toContain("API Key");

    const draft = companyRelayChannelFromTemplate(template, { hasApiKey: true });
    expect(draft.hasApiKey).toBe(true);
    expect(Object.keys(companyRelayTemplateFromChannel(draft, companyRelaySharedEndpoints(draft)))).not.toContain("hasApiKey");
    let sent: Record<string, unknown> = {};
    const result = await testChannelModelConnection(draft, "text", "text", "chat-completion", async ({ config }) => {
        sent = config as unknown as Record<string, unknown>;
        return { text: "OK" };
    });
    expect(result).toContain("文本响应");
    expect((sent.channels as { id: string; pinned: boolean; apiKey: string }[])[0]).toMatchObject({ id: "company-relay", pinned: true, apiKey: "" });
});

test("model endpoints mirror the server: own endpoint on the relay origin, otherwise the relay base", () => {
    expect(companyRelayModelEndpoint(template, "seedance-2.5")).toBe("http://10.10.10.45:3000/doubao/api/v3");
    expect(companyRelayModelEndpoint(template, "unknown")).toBe("http://10.10.10.45:3000/v1");
    expect(companyRelayModelEndpoint({ ...template, baseUrl: "http://10.10.10.45:3000/v1/" }, "unknown")).toBe("http://10.10.10.45:3000/v1");
    expect(companyRelayModelEndpoint({ ...template, modelProfiles: [{ model: "x", capability: "video", baseUrl: "http://other.example:3000/api/v3" }] }, "x")).toBe("http://10.10.10.45:3000/v1");
    expect(companyRelayModelEndpoint(null, "x")).toBe("");

    // A new Seedance model only gets its Ark endpoint once saved, so the editor must not test it before then.
    const draft = companyRelayChannelFromTemplate(template, { hasApiKey: true });
    const shared = companyRelaySharedEndpoints(draft);
    draft.models.push("new-video");
    draft.modelProfiles!.push({ model: "new-video", capability: "video", protocol: "volcengine-ark-video" });
    expect(companyRelayModelEndpoint(companyRelayTemplateFromChannel(draft, shared), "new-video")).toBe(shared.arkBaseUrl);
    expect(companyRelayModelEndpoint(template, "new-video")).toBe(template.baseUrl);
});

test("relay API unwraps GET, PUT and DELETE, forwards cancellation and preserves validation errors", async () => {
    const original = apiClient.defaults.adapter;
    const requests: { method?: string; url?: string; body: unknown; signal: unknown }[] = [];
    const state = { channel: template, source: "file", file: "/config/relay.json", fileAvailable: true };
    const signal = new AbortController().signal;
    try {
        apiClient.defaults.adapter = async request => {
            requests.push({ method: request.method, url: request.url, body: request.data ? JSON.parse(request.data) : undefined, signal: request.signal });
            return { config: request, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: state } };
        };
        expect(await getPlatformRelayChannel(signal)).toEqual(state);
        expect(await savePlatformRelayChannel(template)).toEqual(state);
        expect(await resetPlatformRelayChannel()).toEqual(state);
        expect(requests).toEqual([
            { method: "get", url: "/platform/admin/relay-channel", body: undefined, signal },
            { method: "put", url: "/platform/admin/relay-channel", body: { channel: template }, signal: undefined },
            { method: "delete", url: "/platform/admin/relay-channel", body: undefined, signal: undefined },
        ]);
        apiClient.defaults.adapter = async request => ({ config: request, status: 400, statusText: "Bad Request", headers: {}, data: { code: 400, reason: "relay_channel_invalid", msg: "模型接口地址必须与中转站同主机" } });
        await expect(savePlatformRelayChannel(template)).rejects.toMatchObject({ name: new ApiError("").name, status: 400, reason: "relay_channel_invalid", message: "模型接口地址必须与中转站同主机" });
    } finally { apiClient.defaults.adapter = original; }
});

test("the shared reference service also reaches the YSXXAI Seedance models", () => {
    const ysxxai: RelayChannelTemplate = {
        name: "公司中转站", baseUrl: "http://10.10.10.45:3000/v1", apiFormat: "openai",
        models: ["doubao-seedance-2-0-260128-2-720P", "doubao-seedance-2-5-260628-2-1080P", "gpt-image-2"],
        modelProfiles: [
            { model: "doubao-seedance-2-0-260128-2-720P", capability: "video", protocol: "video-generations-json", baseUrl: "http://10.10.10.45:3000/ysxxai-seedance" },
            { model: "doubao-seedance-2-5-260628-2-1080P", capability: "video", protocol: "video-generations-json", baseUrl: "http://10.10.10.45:3000/ysxxai-seedance", referenceAssetService: "http://10.10.10.45:8787/v1/reference-images" },
            { model: "gpt-image-2", capability: "image", protocol: "openai-image" },
        ],
    };
    const draft = companyRelayChannelFromTemplate(ysxxai);
    const opening = companyRelaySharedEndpoints(draft);
    expect(opening.referenceAssetService).toBe("http://10.10.10.45:8787/v1/reference-images");
    const saved = companyRelayTemplateFromChannel(draft, { ...opening, referenceAssetService: "http://10.10.10.45:8787/v1/ysxxai-reference-images" }, opening);
    for (const model of ["doubao-seedance-2-0-260128-2-720P", "doubao-seedance-2-5-260628-2-1080P"]) {
        expect(saved.modelProfiles.find(profile => profile.model === model)?.referenceAssetService).toBe("http://10.10.10.45:8787/v1/ysxxai-reference-images");
    }
    expect(saved.modelProfiles.find(profile => profile.model === "gpt-image-2")).not.toHaveProperty("referenceAssetService");
});

test("YSXXAI Kling and Wan models take the shared ysxxai-video entry, not the Seedance services", () => {
    const draft = companyRelayChannelFromTemplate(template);
    const opening = companyRelaySharedEndpoints(draft);
    draft.models.push("kling-4.0", "wan3.0-video", "kling-elsewhere");
    draft.modelProfiles!.push(
        { model: "kling-4.0", capability: "video", protocol: "video-generations-json" },
        { model: "wan3.0-video", capability: "video", protocol: "video-generations-json" },
        { model: "kling-elsewhere", capability: "video", protocol: "newapi" },
    );
    const entry = "http://10.10.10.45:3000/ysxxai-video";
    const result = companyRelayTemplateFromChannel(draft, { ...opening, ysxxaiVideoBaseUrl: ` ${entry} ` }, opening);
    for (const model of ["kling-4.0", "wan3.0-video"]) {
        const profile = result.modelProfiles.find(item => item.model === model)!;
        expect(profile.baseUrl).toBe(entry);
        expect(profile).not.toHaveProperty("referenceAssetService");
        expect(companyRelayModelEndpoint(result, model)).toBe(entry);
    }
    expect(result.modelProfiles.find(item => item.model === "kling-elsewhere")).not.toHaveProperty("baseUrl");
    expect(result.modelProfiles.find(item => item.model === "seedance-2.5")?.baseUrl).toBe(template.modelProfiles[0].baseUrl);
    const reopened = companyRelayChannelFromTemplate(result);
    expect(companyRelaySharedEndpoints(reopened).ysxxaiVideoBaseUrl).toBe(entry);
    const cleared = companyRelayTemplateFromChannel(reopened, { ...companyRelaySharedEndpoints(reopened), ysxxaiVideoBaseUrl: "" });
    expect(cleared.modelProfiles.find(item => item.model === "kling-4.0")).not.toHaveProperty("baseUrl");
});
