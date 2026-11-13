import { strict as assert } from "node:assert";
import { test } from "node:test";
import { meta, protocols, buildSubmitRequest, extractUsage, parseTaskResult } from "./ysxxai-image-task-plugin.js";

const baseUrl = "https://os.ysxxai.com";
const decode = (model, operation, value) => protocols.openai_image.decodeRequest({ model, operation, body: { kind: "json", value } });
const submit = (model, operation, value) => buildSubmitRequest({ model, baseUrl, apiKey: "test-key", requestBody: decode(model, operation, value).requestBody });

test("a pixel size becomes the supplier's tier model name and ratio", () => {
  const request = submit("gpt-image-2.5", "generate", { model: "gpt-image-2.5", prompt: "a cat", n: 1, size: "3264x2448", response_format: "b64_json", quality: "high", output_format: "png" });
  assert.equal(request.url, baseUrl + "/v1/images/generations");
  assert.deepEqual(request.body, { prompt: "a cat", model: "gpt-image-2.5-4k", n: 1, size: "4:3" });
  assert.equal(submit("gemini-nano-banana-2.1", "generate", { prompt: "p", size: "1024x1024" }).body.model, "gemini-nano-banana-2.1-1k");
  assert.equal(submit("gemini-nano-banana-2.1", "generate", { prompt: "p", size: "1536x2720" }).body.size, "9:16");
  assert.equal(submit("gemini-nano-banana-2.1", "generate", { prompt: "p", size: "2048x1536" }).body.model, "gemini-nano-banana-2.1-2k");
});

test("tiers, ratios and auto can also be sent directly", () => {
  const tiered = submit("gemini-nano-banana-2.1", "generate", { prompt: "p", size: "16:9", resolution: "4k", n: "2" });
  assert.deepEqual(tiered.body, { prompt: "p", model: "gemini-nano-banana-2.1-4k", n: 2, size: "16:9" });
  const auto = submit("gpt-image-2.5", "generate", { prompt: "p", size: "auto" });
  assert.deepEqual(auto.body, { prompt: "p", model: "gpt-image-2.5-2k", n: 1 });
  assert.equal(submit("gpt-image-2.5", "generate", { prompt: "p", size: "4k", aspect_ratio: "3:2" }).body.size, "3:2");
  assert.throws(() => decode("gpt-image-2.5", "generate", { prompt: "p", size: "1024x1024" }), /supports 2k and 4k/);
  assert.throws(() => decode("gpt-image-2.5", "generate", { prompt: "p", size: "8:1" }), /supports ratios/);
  assert.throws(() => decode("gpt-image-2.5", "generate", { prompt: "p", size: "2048x1536", resolution: "4k" }), /agree/);
  assert.throws(() => decode("gpt-image-2.5", "generate", { prompt: "p", size: "big" }), /size must be/);
  assert.throws(() => decode("unknown", "generate", { prompt: "p" }), /unsupported image model/);
  assert.throws(() => decode("gpt-image-2.5", "generate", { size: "4k" }), /prompt/);
});

test("edits pass public links and refuse uploaded files", () => {
  const links = ["https://files.example/a.png", "https://files.example/b.png"];
  const request = submit("gpt-image-2.5", "edit", { prompt: "merge", size: "3264x2448", image_urls: links });
  assert.equal(request.url, baseUrl + "/v1/images/edits");
  assert.equal(request.headers["Content-Type"], "application/json");
  assert.deepEqual(request.body, { prompt: "merge", model: "gpt-image-2.5-4k", n: 1, size: "4:3", image_urls: links });
  assert.throws(() => decode("gpt-image-2.5", "edit", { prompt: "merge" }), /image_urls/);
  assert.throws(() => decode("gpt-image-2.5", "edit", { prompt: "merge", image_urls: ["data:image/png;base64,AA"] }), /http/);
  assert.throws(() => decode("gpt-image-2.5", "generate", { prompt: "p", image_urls: links }), /only for image edits/);
  const upload = { kind: "multipart", fields: { prompt: ["merge"] }, files: [{ field: "image", ref: "f1", filename: "a.png" }] };
  assert.throws(() => protocols.openai_image.decodeRequest({ model: "gpt-image-2.5", operation: "edit", body: upload }), /image_urls/);
  const form = { kind: "multipart", fields: { prompt: ["merge"], size: ["4k"], image_urls: links }, files: [] };
  const viaForm = protocols.openai_image.decodeRequest({ model: "gpt-image-2.5", operation: "edit", body: form });
  assert.deepEqual(viaForm.requestBody.imageUrls, links);
});

test("billing follows the tier and the image count", () => {
  const intent = decode("gemini-nano-banana-2.1", "generate", { prompt: "p", size: "4k", n: 3 });
  assert.deepEqual(extractUsage({ requestBody: intent.requestBody }), { resolution: "4k", image_count: 3 });
  assert.deepEqual(extractUsage({ requestBody: intent.requestBody, usagePurpose: "billing_ratios" }), { image_count: 3 });
  assert.equal(meta.version, "1.1.0");
  assert.equal(parseTaskResult({}, { status: "completed", data: [{ url: "https://files.example/out.jpg" }] }).status, "SUCCESS");
  assert.equal(parseTaskResult({}, { status: "failed", error: { message: "blocked" } }).reason, "blocked");
});
