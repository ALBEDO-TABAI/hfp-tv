import { strict as assert } from "node:assert";
import { test } from "node:test";
import { meta, native, buildSubmitRequest, parseSubmitResponse, buildQueryRequest, parseTaskResult, listArtifacts, buildContentRequest, extractUsage, extractUsageOnComplete } from "./ysxxai-task-plugin.js";

test("Seedance relay preserves the documented request and task identity", () => {
  const body = {
    model: "doubao-seedance-2-5-260628-2-720P",
    prompt: "Follow image1 and video1",
    metadata: {
      duration: 5, resolution: "720p", ratio: "9:16", generate_audio: false, watermark: false,
      content: [
        { type: "image_url", image_url: { url: "https://example.com/first.jpg" }, role: "first_frame" },
        { type: "video_url", video_url: { url: "https://example.com/ref.mp4" }, role: "reference_video" },
        { type: "audio_url", audio_url: { url: "https://example.com/ref.mp3" }, role: "reference_audio" },
      ],
    },
  };
  assert.equal(meta.models.length, 7);
  const intent = native.create({ body: { kind: "json", value: body } });
  const request = buildSubmitRequest({ requestBody: intent.requestBody, baseUrl: "https://www.ysxxai.com", apiKey: "test-key", upstreamModel: body.model });
  assert.equal(request.url, "https://www.ysxxai.com/v1/video/generations");
  assert.deepEqual(request.body, { ...body, metadata: { ...body.metadata, ratio: "adaptive" } });
  assert.equal(parseSubmitResponse({}, { body: { id: "task-upstream", status: "queued" } }).taskId, "task-upstream");
  assert.equal(buildQueryRequest({ baseUrl: "https://www.ysxxai.com", taskId: "task-upstream", apiKey: "test-key" }).url, "https://www.ysxxai.com/v1/videos/task-upstream");
});

test("the current workbench can send an uploaded reference through the same channel", () => {
  const intent = native.create({ body: { kind: "json", value: {
    model: "doubao-seedance-2-5-260628-2-720P", prompt: "move", duration: 4, ratio: "9:16", resolution: "720p", generate_audio: true, watermark: false,
    images: [{ url: "https://example.com/reference.jpg", role: "first_frame" }],
  } } });
  assert.deepEqual(intent.requestBody.metadata, {
    duration: 4, ratio: "adaptive", resolution: "720p", generate_audio: true, watermark: false,
    content: [{ type: "image_url", image_url: { url: "https://example.com/reference.jpg" }, role: "first_frame" }],
  });
});

test("only 2.5 keyframes use their image ratio", () => {
  for (const [model, role, ratio] of [
    ["doubao-seedance-2-5-260628-2-720P", "first_frame", "adaptive"],
    ["doubao-seedance-2-5-260628-2-720P", "last_frame", "adaptive"],
    ["doubao-seedance-2-5-260628-2-720P", "reference_image", "16:9"],
    ["doubao-seedance-2-0-260128-2-720P", "first_frame", "16:9"],
  ]) {
    const { requestBody } = native.create({ body: { kind: "json", value: { model, prompt: "test", duration: 4, ratio: "16:9", resolution: "720p", images: [{ url: "https://example.com/image.png", role }] } } });
    assert.equal(requestBody.metadata.ratio, ratio);
  }
});

test("trusted assets returned by the provider remain valid references", () => {
  const { requestBody } = native.create({ body: { kind: "json", value: {
    model: "doubao-seedance-2-0-260128-2-720P", prompt: "move", duration: 4, ratio: "16:9", resolution: "720p",
    images: [{ url: "asset://asset-20261105-example", role: "reference_image" }],
  } } });
  assert.equal(requestBody.metadata.content[0].image_url.url, "asset://asset-20261105-example");
});

test("Seedance relay reports real task states and downloads the original result", () => {
  const ctx = { baseUrl: "https://www.ysxxai.com", taskId: "task-upstream", apiKey: "test-key" };
  globalThis.utils = { fetch: () => ({ status: 200, body: { code: "success", data: { status: "SUCCESS", billing: { settlement_status: "settled", final_amount: 4.590442 } } } }) };
  assert.equal(parseTaskResult(ctx, { status: "in_progress", progress: 50 }).status, "IN_PROGRESS");
  const awaitingSettlement = parseTaskResult(ctx, { status: "completed" });
  assert.equal(awaitingSettlement.status, "IN_PROGRESS");
  assert.deepEqual(awaitingSettlement.state, { finalCost: 4.590442 });
  const settledContext = { ...ctx, state: awaitingSettlement.state };
  assert.equal(parseTaskResult(settledContext, { status: "completed" }).status, "SUCCESS");
  assert.equal(parseTaskResult(ctx, { status: "unexpected" }).status, "UNKNOWN");
  const completed = { task_id: "task-local", status: "SUCCESS", data: { data: { status: "SUCCESS" } } };
  assert.equal(native.status({}, completed).result_url, "/v1/tasks/task-local/artifacts/video/content");
  assert.equal(native.status({}, completed).data[0].url, "/v1/tasks/task-local/artifacts/video/content");
  assert.equal(listArtifacts({ status: "SUCCESS" })[0].key, "video");
  const download = buildContentRequest({ artifactKey: "video", upstreamTaskId: "task-upstream", baseUrl: "https://www.ysxxai.com", apiKey: "test-key", clientRequest: { method: "GET" } });
  assert.equal(download.url, "https://www.ysxxai.com/v1/videos/task-upstream/content");
  assert.equal(download.headers.Authorization, "Bearer test-key");
  delete globalThis.utils;
  assert.deepEqual(extractUsageOnComplete(settledContext, { status: "SUCCESS" }, { status: "completed" }), { cost: 4.590442 });
  globalThis.utils = { fetch: () => ({ status: 200, body: { code: "success", data: { status: "SUCCESS", billing: { settlement_status: "pending", final_amount: 0 } } } }) };
  assert.equal(parseTaskResult(ctx, { status: "completed" }).status, "IN_PROGRESS");
  delete globalThis.utils;
});

test("Seedance relay rejects unsupported options before creating a task", () => {
  const create = (model, metadata) => native.create({ body: { kind: "json", value: { model, prompt: "test", metadata } } });
  assert.throws(() => create("doubao-seedance-2-0-fast-260128-2-720P", { duration: 5, resolution: "720p", content: [{ type: "image_url", image_url: { url: "https://example.com/ref.jpg" }, role: "reference_image" }] }));
  assert.throws(() => create("doubao-seedance-2-5-260628-2-720P", { duration: 16, resolution: "720p" }));
  assert.throws(() => create("doubao-seedance-2-5-260628-2-720P", { duration: 5, resolution: "1080p" }));
  assert.throws(() => create("doubao-seedance-2-5-260628-2-720P", { duration: 5, resolution: "720p", content: [{ type: "image_url", image_url: { url: "http://example.com/ref.jpg" }, role: "reference_image" }] }));
  assert.throws(() => create("doubao-seedance-2-5-260628-2-720P", { duration: 5, resolution: "720p", content: [{ type: "image_url", image_url: { url: "asset://asset-one/extra" }, role: "reference_image" }] }));
  assert.equal(extractUsage({ requestBody: { model: "doubao-seedance-2-5-260628-2-720P", prompt: "test", metadata: { duration: 4, resolution: "720p" } } }).cost, 4.536);
});
