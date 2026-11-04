import { expect, test } from "bun:test";
import { companyRelayChannelFromTemplate, companyRelaySharedEndpoints, companyRelayTemplateFromChannel } from "../src/lib/company-relay-template";
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
    expect(companyRelaySharedEndpoints(draft)).toEqual({ arkBaseUrl: "", referenceAssetService: "" });
});

test("Seedance endpoints survive the editor round trip and new Ark models inherit shared settings", () => {
    const draft = companyRelayChannelFromTemplate(template);
    const shared = companyRelaySharedEndpoints(draft);
    expect(shared).toEqual({ arkBaseUrl: template.modelProfiles[0].baseUrl!, referenceAssetService: template.modelProfiles[0].referenceAssetService! });
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
    expect(companyRelaySharedEndpoints(draft)).toEqual({ arkBaseUrl: "http://10.10.10.45:3000/image", referenceAssetService: template.modelProfiles[0].referenceAssetService! });
});

test("clearing shared endpoints removes only matching fields and output contains only template fields", () => {
    const draft = { ...companyRelayChannelFromTemplate(template), name: "   ", baseUrl: " http://10.10.10.45:3000/v1/// ", apiKey: "do-not-send", secretKey: "do-not-send", hasApiKey: true, headers: [{ name: "Authorization", value: "do-not-send" }] };
    const result = companyRelayTemplateFromChannel(draft, { arkBaseUrl: " ", referenceAssetService: "" });
    expect(Object.keys(result).sort()).toEqual(["apiFormat", "baseUrl", "modelProfiles", "models", "name"]);
    expect(result.name).toBe("公司中转站");
    expect(result.baseUrl).toBe(template.baseUrl);
    expect(result.modelProfiles[0]).not.toHaveProperty("baseUrl");
    expect(result.modelProfiles[0]).not.toHaveProperty("referenceAssetService");
    expect(result.modelProfiles[1]).toEqual(template.modelProfiles[1]);
    expect(JSON.stringify(result)).not.toContain("do-not-send");
    expect(companyRelaySharedEndpoints(draft).arkBaseUrl).toBe(template.modelProfiles[0].baseUrl!);
    expect(companyRelayTemplateFromChannel({ ...draft, name: " 公司模型 ", apiFormat: "gemini" }, { arkBaseUrl: "", referenceAssetService: "" })).toMatchObject({ name: "公司模型", apiFormat: "gemini" });
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
