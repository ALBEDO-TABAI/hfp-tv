import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import axios from "axios";

import { listVideoReferenceModels } from "@/lib/canvas/canvas-video-regeneration";
import { defaultModelCapabilityConfig } from "@/lib/model-capabilities";
import { channelValidationError, modelConfigChannelStatusLabel } from "@/pages/settings/channel-settings-pane";
import { assertAudioConfig } from "@/services/api/audio";
import { channelRequest } from "@/services/api/custom-channel-relay";
import { prepareBackendGenerationTask, prepareBackendToolGenerationTask } from "@/services/api/generation-task";
import { fetchChannelModels } from "@/services/api/image-models";
import { apiClient, ApiError } from "@/services/api/request";
import type { CreateTaskInput } from "@/services/api/task-center";
import { assertVideoConfig } from "@/services/api/video-validation";
import { channelHasGenerationCredential, configWithoutCachedSecrets, createModelChannel, defaultConfig, encodeChannelModel, resolveModelRequestConfig, useConfigStore } from "@/stores/use-config-store";

const videoCapabilities = defaultModelCapabilityConfig("newapi-channel-2", "video");
if (videoCapabilities.video) videoCapabilities.video.references.maxVideos = 3;
const channel = createModelChannel({
    id: "company-relay", pinned: true, hasApiKey: true, apiKey: "", baseUrl: "https://relay.example.com/v1",
    models: ["image", "video", "audio", "text"],
    modelProfiles: [
        { model: "image", capability: "image", protocol: "openai-image" },
        { model: "video", capability: "video", protocol: "newapi-channel-2", capabilityConfig: videoCapabilities },
        { model: "audio", capability: "audio", protocol: "openai-audio" },
        { model: "text", capability: "text", protocol: "chat-completion" },
    ],
});
const config = { ...defaultConfig, channels: [channel], videoSeconds: "4" };
const apiAdapter = apiClient.defaults.adapter;
const directAdapter = axios.defaults.adapter;
afterEach(() => {
    apiClient.defaults.adapter = apiAdapter;
    axios.defaults.adapter = directAdapter;
});

test("bound company channel is ready without a browser key; personal channels still require their own key", () => {
    assert.equal(channelHasGenerationCredential(channel), true);
    assert.equal(channelHasGenerationCredential({ ...channel, hasApiKey: false }), false);
    assert.equal(channelHasGenerationCredential({ ...channel, hasApiKey: undefined }), false);
    assert.equal(channelHasGenerationCredential({ ...channel, id: "personal" }), false);
    assert.equal(channelHasGenerationCredential({ ...channel, id: "personal", apiKey: "personal-key" }), true);
    for (const model of channel.models) {
        assert.equal(useConfigStore.getState().isAiConfigReady(config, encodeChannelModel(channel.id, model)), true);
    }
    const video = encodeChannelModel(channel.id, "video");
    const audio = encodeChannelModel(channel.id, "audio");
    assert.ok(listVideoReferenceModels(config).includes(video));
    assert.doesNotThrow(() => assertVideoConfig(resolveModelRequestConfig(config, video), video));
    assert.doesNotThrow(() => assertAudioConfig(resolveModelRequestConfig(config, audio), audio));
    assert.equal(channelValidationError(channel), "");
    const unbound = { ...channel, hasApiKey: false };
    assert.equal(channelValidationError(unbound), "等待管理员绑定中转站密钥");
    assert.equal(modelConfigChannelStatusLabel(unbound, { status: "idle", revision: 1, dirty: false, error: "" }), "等待管理员绑定中转站密钥");
});

test("company model discovery always uses the backend and sends only the channel identity with an empty key", async () => {
    axios.defaults.adapter = async () => { throw new Error("unexpected upstream request"); };
    const requests: unknown[] = [];
    apiClient.defaults.adapter = async (request) => {
        const body = JSON.parse(request.data);
        requests.push({ url: request.url, method: request.method, channelId: body.channelId, apiKey: body.apiKey });
        return { config: request, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { models: ["text", "image"] } } };
    };
    for (const apiFormat of ["openai", "gemini", "claude"] as const) {
        assert.deepEqual((await fetchChannelModels({ ...channel, apiFormat, apiKey: "must-not-send" })).models, ["image", "text"]);
    }
    assert.deepEqual(requests, Array.from({ length: 3 }, () => ({ url: "/ai/models", method: "post", channelId: channel.id, apiKey: "" })));
});

test("company model discovery preserves the backend missing-binding message", async () => {
    const message = "管理员还没有为你绑定公司模型服务的密钥";
    apiClient.defaults.adapter = async (request) => ({ config: request, status: 200, statusText: "OK", headers: {}, data: { code: 400, msg: message, reason: message } });
    await assert.rejects(fetchChannelModels({ ...channel, hasApiKey: false }), (error) => error instanceof ApiError && error.message === message);
});

test("company relay removes empty auth headers; personal relay still sends its own key", () => {
    const resolved = resolveModelRequestConfig(config, encodeChannelModel(channel.id, "text"));
    const request = channelRequest(resolved, "https://relay.example.com/v1/chat/completions", { Authorization: "Bearer ", "x-goog-api-key": "", "X-Canvas-Upstream-Headers": "old" });
    assert.equal(request.url, "/api/ai/custom");
    assert.equal(request.credentials, "include");
    assert.equal(request.headers.authorization, undefined);
    assert.equal(request.headers["x-goog-api-key"], undefined);
    assert.equal(request.headers["x-canvas-upstream-headers"], undefined);
    assert.equal(channelRequest({ ...resolved, channelId: "", apiKey: "personal-key" }, "https://relay.example.com/v1/models").headers.authorization, "Bearer personal-key");
});

test("all company generation tasks and tool text tasks retain the channel id and empty keys", async () => {
    for (const mode of ["image", "video", "audio", "text"] as const) {
        const selected = { ...config, model: encodeChannelModel(channel.id, mode) };
        const task = await prepareBackendGenerationTask({ config: selected, mode, prompt: "test" });
        assertTaskCredential(task, channel.id, "");
    }
    const task = prepareBackendToolGenerationTask({ config: { ...config, model: encodeChannelModel(channel.id, "text") }, prompt: "test", messages: [{ role: "user", content: "test" }], tools: [], toolChoice: "auto" });
    assertTaskCredential(task, channel.id, "");
    const personal = { ...config, channels: [{ ...channel, id: "personal", apiKey: "personal-key" }], model: "personal::text" };
    assertTaskCredential(await prepareBackendGenerationTask({ config: personal, mode: "text", prompt: "test" }), "", "personal-key");
    const cached = configWithoutCachedSecrets(config).channels[0];
    assert.deepEqual([cached.apiKey, cached.secretKey, cached.hasApiKey], ["", "", true]);
});

function assertTaskCredential(task: CreateTaskInput, channelId: string, apiKey: string) {
    const provider = task.input?.config;
    assert.ok(provider && typeof provider === "object" && "channelId" in provider && "apiKey" in provider && "secretKey" in provider);
    assert.deepEqual([provider.channelId, provider.apiKey, provider.secretKey], [channelId, apiKey, ""]);
}
