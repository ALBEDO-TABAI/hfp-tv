import { strict as assert } from "node:assert";
import { test } from "node:test";
import { meta, native, protocols, buildSubmitRequest, parseSubmitResponse, extractUsage, extractUsageOnComplete, buildQueryRequest, parseTaskResult, listArtifacts, buildContentRequest } from "./ysxxai-video-task-plugin.js";

const baseUrl = "https://www.ysxxai.com";
const create = (body) => native.create({ body: { kind: "json", value: body } });

test("the three supported models keep the provider's JSON request and task identity", () => {
  for (const model of meta.models) {
    const original = { model, prompt: "A paper boat", duration: 5, size: "1280x720" };
    const intent = create(original);
    const request = buildSubmitRequest({ requestBody: intent.requestBody, baseUrl, apiKey: "test-key" });
    assert.equal(request.url, baseUrl + "/v1/video/generations");
    assert.deepEqual(request.body, original);
    assert.equal(request.headers.Authorization, "Bearer test-key");
    assert.equal(parseSubmitResponse({}, { body: { id: "task-upstream" } }).taskId, "task-upstream");
  }
  assert.throws(() => create({ model: "unknown", prompt: "test" }), /unsupported/);
  assert.throws(() => create({ model: "kling-4.0-flash", prompt: "test", duration: 21 }), /duration/);
  assert.throws(() => create({ model: "kling-4.0", prompt: "test", duration: 2 }), /duration/);
  assert.throws(() => create({ model: "kling-4.0", prompt: "test", duration: 5, seconds: 6 }), /match/);
  assert.throws(() => create({ model: "kling-4.0", prompt: "test", seconds: [5] }), /duration/);
  assert.equal(create({ model: "wan3.0-video", prompt: "test", duration: 2 }).requestBody.seconds, 2);
  assert.equal(create({ model: "kling-4.0-flash", prompt: "test", duration: 3 }).requestBody.seconds, 3);
  assert.throws(() => parseSubmitResponse({}, { body: {} }), /task ID/);
});

test("OpenAI video requests use the provider's video endpoint without losing files", () => {
  const json = protocols.openai_video.decodeRequest({ model: "wan3.0-video", body: { kind: "json", value: { model: "wan3.0-video", prompt: "waves", seconds: "4" } } });
  const request = buildSubmitRequest({ requestBody: json.requestBody, baseUrl, apiKey: "test-key" });
  assert.equal(request.url, baseUrl + "/v1/videos");
  assert.equal(request.body.seconds, "4");
  const file = { field: "input_reference", ref: "file-ref", filename: "reference.png", mimeType: "image/png" };
  const multipart = protocols.openai_video.decodeRequest({ model: "kling-4.0", body: { kind: "multipart", fields: { prompt: ["move"], seconds: ["5"] }, files: [file] } });
  const upload = buildSubmitRequest({ requestBody: multipart.requestBody, baseUrl, apiKey: "test-key", files: [file] });
  assert.equal(upload.bodyType, "multipart");
  assert.deepEqual(upload.parts.find((part) => part.fileRef), { name: "input_reference", fileRef: "file-ref", filename: "reference.png" });
  assert.deepEqual(protocols.openai_video.render({}, { status: "SUCCESS", data: { id: "private-upstream-id" } }), {});
});

test("reservation covers the published per-second prices and reference-video boundary", () => {
  const estimate = (model, extra = {}) => extractUsage({ requestBody: create({ model, prompt: "test", duration: 5, ...extra }).requestBody }).cost;
  assert.equal(estimate("kling-4.0"), 6.15);
  assert.equal(estimate("kling-4.0-flash"), 2.46);
  assert.ok(Math.abs(estimate("wan3.0-video") - 1.65) < 1e-9);
  assert.ok(estimate("kling-4.0", { videos: ["https://example.com/ref.mp4"] }) >= (5 + 30) * 0.738);
  assert.ok(estimate("kling-4.0-flash", { videos: ["https://example.com/ref.mp4"] }) >= (5 + 30) * 0.328);
});

test("completion waits for the upstream charge and preserves the original video", () => {
  const ctx = { baseUrl, taskId: "task-upstream", apiKey: "test-key" };
  assert.equal(buildQueryRequest(ctx).url, baseUrl + "/v1/videos/task-upstream");
  assert.equal(parseTaskResult(ctx, { status: "cancelled" }).status, "FAILURE");
  globalThis.utils = { fetch: () => ({ status: 200, body: { code: "success", data: { status: "SUCCESS", billing: { settlement_status: "settled", final_amount: 4.1 } } } }) };
  try {
    const pending = parseTaskResult(ctx, { status: "completed" });
    assert.equal(pending.status, "IN_PROGRESS");
    assert.deepEqual(pending.state, { finalCost: 4.1 });
    const settled = { ...ctx, state: pending.state };
    assert.equal(parseTaskResult(settled, { status: "completed" }).status, "SUCCESS");
    assert.deepEqual(extractUsageOnComplete(settled, {}, { status: "completed" }), { cost: 4.1 });
    assert.equal(listArtifacts({ status: "SUCCESS" })[0].key, "video");
    const result = native.status({}, { task_id: "task-local", status: "SUCCESS" });
    assert.equal(result.data[0].url, "/v1/tasks/task-local/artifacts/video/content");
    assert.equal(buildContentRequest({ artifactKey: "video", upstreamTaskId: "task-upstream", baseUrl, apiKey: "test-key", clientRequest: { method: "GET" } }).url, baseUrl + "/v1/videos/task-upstream/content");
    globalThis.utils = { fetch: () => ({ status: 200, body: { code: "success", data: { status: "SUCCESS", billing: { settlement_status: "pending" } } } }) };
    assert.equal(parseTaskResult(ctx, { status: "completed" }).status, "IN_PROGRESS");
  } finally {
    delete globalThis.utils;
  }
});

// The generic body hfp - all in one sends for every "video-generations-json" model.
const workbench = (model, extra = {}) => ({ model, prompt: "walk", duration: 5, ratio: "16:9", resolution: "720p", generate_audio: false, watermark: false, ...extra });
const submitted = (body) => buildSubmitRequest({ requestBody: create(body).requestBody, baseUrl, apiKey: "test-key" }).body;

test("workbench Kling requests become contents without the refused audio switch", () => {
  assert.deepEqual(submitted(workbench("kling-4.0")), { model: "kling-4.0", prompt: "walk", duration: 5, resolution: "720p", aspect_ratio: "16:9" });
  const frames = submitted(workbench("kling-4.0-flash", { resolution: "1080P", watermark: true, images: [{ url: "https://cdn.example.com/a.png", role: "first_frame" }, { url: "https://cdn.example.com/b.png", role: "last_frame" }] }));
  assert.deepEqual(frames.contents, [{ type: "first_frame", url: "https://cdn.example.com/a.png" }, { type: "last_frame", url: "https://cdn.example.com/b.png" }]);
  assert.equal(frames.aspect_ratio, undefined, "frames keep the picture's own aspect ratio");
  assert.equal(frames.resolution, "1080p");
  assert.deepEqual(frames.watermark_info, { enabled: true });
  const refs = submitted(workbench("kling-4.0", { images: [{ url: "https://cdn.example.com/c.png", role: "reference_image" }, { url: "https://cdn.example.com/d.png" }], videos: ["https://cdn.example.com/move.mp4"] }));
  assert.deepEqual(refs.contents.map((item) => item.type), ["refer_image", "refer_image", "feature_video"]);
  assert.equal(refs.aspect_ratio, "16:9");
  for (const field of ["generate_audio", "watermark", "images", "videos", "audios", "ratio"]) assert.equal(refs[field], undefined, field);
  assert.throws(() => create(workbench("kling-4.0", { audios: ["https://cdn.example.com/a.mp3"] })), /reference audio/);
  assert.throws(() => create(workbench("kling-4.0", { images: [{ url: "https://cdn.example.com/b.png", role: "last_frame" }] })), /first frame/);
  assert.throws(() => create(workbench("kling-4.0", { images: [{ url: "data:image/png;base64,aGVsbG8=", role: "reference_image" }] })), /public http/);
  assert.equal(create(workbench("kling-4.0", { images: [{ url: "https://cdn.example.com/a.png" }] })).action, "image_to_video");
  assert.equal(create(workbench("kling-4.0")).action, "text_to_video");
});

test("workbench Wan requests use image/images for frames and metadata for references", () => {
  assert.deepEqual(submitted(workbench("wan3.0-video", { generate_audio: true })), { model: "wan3.0-video", prompt: "walk", duration: 5, resolution: "720P", ratio: "16:9", metadata: { parameters: { audio: true, watermark: false } } });
  const first = submitted(workbench("wan3.0-video", { images: [{ url: "https://cdn.example.com/a.png", role: "first_frame" }] }));
  assert.equal(first.image, "https://cdn.example.com/a.png");
  assert.equal(first.ratio, undefined);
  const both = submitted(workbench("wan3.0-video", { images: [{ url: "https://cdn.example.com/b.png", role: "last_frame" }, { url: "https://cdn.example.com/a.png", role: "first_frame" }] }));
  assert.deepEqual(both.images, ["https://cdn.example.com/a.png", "https://cdn.example.com/b.png"]);
  const refs = submitted(workbench("wan3.0-video", { images: [{ url: "https://cdn.example.com/c.png", role: "reference_image" }], videos: ["https://cdn.example.com/v.mp4"], audios: ["https://cdn.example.com/a.mp3"] }));
  assert.deepEqual(refs.metadata.input.media, [{ type: "reference_image", url: "https://cdn.example.com/c.png" }, { type: "reference_video", url: "https://cdn.example.com/v.mp4" }, { type: "reference_audio", url: "https://cdn.example.com/a.mp3" }]);
  assert.equal(refs.ratio, "16:9");
  assert.equal(refs.image, undefined);
  assert.throws(() => create(workbench("wan3.0-video", { images: [{ url: "https://cdn.example.com/a.png", role: "first_frame" }, { url: "https://cdn.example.com/c.png", role: "reference_image" }] })), /cannot combine/);
  assert.throws(() => create(workbench("wan3.0-video", { duration: 1 })), /duration/);
  assert.equal(submitted(workbench("wan3.0-video", { resolution: "1080" })).resolution, "1080P", "a bare height gets its unit");
  assert.equal(submitted(workbench("kling-4.0", { resolution: "720" })).resolution, "720p");
});

test("reservation reads reference videos inside contents and metadata", () => {
  const estimate = (body) => extractUsage({ requestBody: create(body).requestBody }).cost;
  assert.equal(estimate(workbench("kling-4.0", { images: [{ url: "https://cdn.example.com/c.png" }] })), 6.15, "reference images add no billed seconds");
  assert.ok(estimate(workbench("kling-4.0", { videos: ["https://cdn.example.com/v.mp4"] })) >= (5 + 30) * 0.738);
  assert.ok(Math.abs(estimate(workbench("wan3.0-video", { images: [{ url: "https://cdn.example.com/c.png" }] })) - 1.65) < 1e-9);
  assert.ok(estimate(workbench("wan3.0-video", { videos: ["https://cdn.example.com/v.mp4"] })) > 1.65);
});
