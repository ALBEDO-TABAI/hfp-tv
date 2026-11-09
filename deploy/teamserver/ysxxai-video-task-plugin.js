const models = ["kling-4.0", "kling-4.0-flash", "wan3.0-video"];

export const meta = {
  apiVersion: 1,
  key: "ysxxai-video",
  name: "YSXXAI Video",
  version: "1.1.1",
  author: { name: "HFP" },
  description: { en: "YSXXAI Kling and Wan video tasks", zh: "YSXXAI 可灵与万相视频任务" },
  baseUrl: "https://www.ysxxai.com",
  models,
  fetchMode: "per_task",
  usageSchema: {
    cost: { type: "number", unit: "credit", description: { en: "Upstream cost in CNY", zh: "上游人民币费用" } },
  },
  routes: [
    { method: "POST", path: "/ysxxai-video/v1/video/generations", type: "submit", decode: "create", render: "created" },
    { method: "GET", path: "/ysxxai-video/v1/video/generations/:task_id", type: "query", render: "status" },
  ],
  protocols: ["openai_video"],
};

function requestIntent(body, model, format, files) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("JSON object required");
  if (!models.includes(model)) throw new Error("unsupported YSXXAI video model");
  const payload = { ...body, model };
  const duration = payload.seconds === undefined ? payload.duration : payload.seconds;
  if (duration !== undefined && typeof duration !== "string" && typeof duration !== "number") throw new Error("invalid video duration");
  if (payload.seconds !== undefined && payload.duration !== undefined && Number(payload.seconds) !== Number(payload.duration)) {
    throw new Error("seconds and duration must match");
  }
  const seconds = duration === undefined ? (model === "wan3.0-video" ? 4 : 5) : Number(duration);
  const maxSeconds = model === "kling-4.0-flash" ? 20 : 30;
  const minSeconds = model === "wan3.0-video" ? 2 : 3;
  if (!Number.isInteger(seconds) || seconds < minSeconds || seconds > maxSeconds) throw new Error("invalid video duration");
  if (duration === undefined) {
    if (format === "native" && model !== "wan3.0-video") payload.duration = seconds;
    else payload.seconds = String(seconds);
  }
  const material = payload.image || payload.images || payload.input_reference || (Array.isArray(payload.contents) && payload.contents.length) || (payload.metadata && payload.metadata.input) || (files || []).length;
  return {
    kind: "submit", model, action: material ? "image_to_video" : "text_to_video",
    requestBody: { format, payload, seconds },
  };
}

// hfp - all in one sends one body for every "video-generations-json" model:
// images [{url, role}], videos [url], audios [url], duration, ratio,
// resolution, generate_audio and watermark. YSXXAI takes Kling material in
// `contents` and Wan material in image/images/metadata, and refuses Kling
// requests that carry generate_audio, so that body is rewritten here. Native
// YSXXAI bodies never carry top-level generate_audio/watermark or image
// objects and pass through unchanged.
function isWorkbenchRequest(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return false;
  if (["contents", "metadata", "image", "image_tail", "elements", "video_url", "aspect_ratio", "seconds"].some((key) => body[key] !== undefined)) return false;
  const imageObjects = Array.isArray(body.images) && body.images.some((item) => item && typeof item === "object");
  return imageObjects || body.generate_audio !== undefined || body.watermark !== undefined || Array.isArray(body.videos) || Array.isArray(body.audios);
}

function workbenchList(body, field) {
  if (body[field] === undefined || body[field] === null) return [];
  if (!Array.isArray(body[field])) throw new Error(field + " must be an array");
  return body[field];
}

function workbenchMedia(value, needsLink) {
  const url = typeof value === "string" ? value.trim() : "";
  if (!url) throw new Error("reference media URL is missing");
  if (needsLink && !/^https?:\/\/[^\s/]+/i.test(url)) throw new Error("Kling needs reference media as a public http(s) link");
  return url;
}

// Callers without the model's capability profile send the bare height ("720").
function workbenchResolution(value) {
  const text = typeof value === "number" ? String(value) : typeof value === "string" ? value.trim() : "";
  return /^\d+$/.test(text) ? text + "p" : text;
}

function workbenchImages(body, needsLink) {
  return workbenchList(body, "images").map((item) => {
    const role = (item && item.role) || "reference_image";
    if (!["first_frame", "last_frame", "reference_image"].includes(role)) throw new Error("unsupported image role " + role);
    return { role, url: workbenchMedia(item && item.url, needsLink) };
  });
}

function klingRequest(body) {
  if (workbenchList(body, "audios").length) throw new Error("Kling 4.0 does not take reference audio on this channel");
  const types = { first_frame: "first_frame", last_frame: "last_frame", reference_image: "refer_image" };
  const contents = workbenchImages(body, true).map((image) => ({ type: types[image.role], url: image.url }));
  for (const url of workbenchList(body, "videos")) contents.push({ type: "feature_video", url: workbenchMedia(url, true) });
  if (contents.some((item) => item.type === "last_frame") && !contents.some((item) => item.type === "first_frame")) throw new Error("a last frame needs a first frame");
  const request = { model: body.model, prompt: body.prompt, duration: body.duration };
  const resolution = workbenchResolution(body.resolution);
  if (resolution) request.resolution = resolution.toLowerCase();
  // Frame-only requests take the picture's own aspect ratio.
  const framesOnly = contents.length > 0 && contents.every((item) => item.type === "first_frame" || item.type === "last_frame");
  if (body.ratio && !framesOnly) request.aspect_ratio = body.ratio;
  if (contents.length) request.contents = contents;
  if (body.watermark === true) request.watermark_info = { enabled: true };
  return request;
}

function wanRequest(body) {
  const images = workbenchImages(body, false);
  const first = images.filter((image) => image.role === "first_frame");
  const last = images.filter((image) => image.role === "last_frame");
  if (first.length > 1 || last.length > 1) throw new Error("Wan takes at most one first frame and one last frame");
  if (last.length && !first.length) throw new Error("a last frame needs a first frame");
  const media = images.filter((image) => image.role === "reference_image").map((image) => ({ type: "reference_image", url: image.url }));
  for (const url of workbenchList(body, "videos")) media.push({ type: "reference_video", url: workbenchMedia(url, false) });
  for (const url of workbenchList(body, "audios")) media.push({ type: "reference_audio", url: workbenchMedia(url, false) });
  if (first.length && media.length) throw new Error("Wan cannot combine first/last frames with reference images, videos or audio");
  const request = { model: body.model, prompt: body.prompt, duration: body.duration };
  const resolution = workbenchResolution(body.resolution);
  if (resolution) request.resolution = resolution.toUpperCase();
  // Frame requests take the picture's own aspect ratio.
  if (body.ratio && !first.length) request.ratio = body.ratio;
  if (first.length && last.length) request.images = [first[0].url, last[0].url];
  else if (first.length) request.image = first[0].url;
  const parameters = {};
  if (body.generate_audio !== undefined) parameters.audio = body.generate_audio === true;
  if (body.watermark !== undefined) parameters.watermark = body.watermark === true;
  const metadata = {};
  if (media.length) metadata.input = { media };
  if (Object.keys(parameters).length) metadata.parameters = parameters;
  if (Object.keys(metadata).length) request.metadata = metadata;
  return request;
}

function workbenchRequest(body) {
  if (!isWorkbenchRequest(body) || !models.includes(body.model)) return body;
  return body.model.startsWith("kling-") ? klingRequest(body) : wanRequest(body);
}

function multipartBody(body) {
  const fields = body.fields || {};
  const payload = {};
  for (const name of Object.keys(fields)) {
    if (!Array.isArray(fields[name]) || fields[name].length !== 1) throw new Error(name + " must be provided once");
    payload[name] = fields[name][0];
  }
  return payload;
}

export const native = {
  create(ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
    const body = workbenchRequest(ctx.body.value);
    return requestIntent(body, body && body.model, "native", []);
  },
  created(ctx, task) {
    return { id: task.task_id, status: "queued" };
  },
  status(ctx, task) {
    const result = { id: task.task_id, status: task.status, progress: task.progress, fail_reason: task.fail_reason };
    if (task.status === "SUCCESS") {
      result.result_url = "/v1/tasks/" + encodeURIComponent(task.task_id) + "/artifacts/video/content";
      result.data = [{ url: result.result_url }];
    }
    return result;
  },
  error(ctx, error) {
    return { error: { code: error.code, message: error.message } };
  },
};

export const protocols = {
  openai_video: {
    decodeRequest(ctx) {
      if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
      const payload = ctx.body.kind === "json" ? ctx.body.value : multipartBody(ctx.body);
      return requestIntent(payload, ctx.model, ctx.body.kind, ctx.body.files);
    },
    render(ctx, task) {
      return task.status === "FAILURE" ? { error: { code: "video_generation_failed", message: task.fail_reason || "upstream task failed" } } : {};
    },
  },
};

export function buildSubmitRequest(ctx) {
  const request = ctx.requestBody;
  if (!request || !request.payload || !models.includes(ctx.upstreamModel || request.payload.model)) throw new Error("invalid video request");
  const payload = { ...request.payload, model: ctx.upstreamModel || request.payload.model };
  const headers = { Authorization: "Bearer " + ctx.apiKey };
  if (request.format === "multipart") {
    const parts = [];
    for (const name of Object.keys(payload)) {
      const value = payload[name];
      if (value === undefined || value === null) continue;
      parts.push({ name, value: typeof value === "object" ? JSON.stringify(value) : String(value) });
    }
    for (const file of ctx.files || []) parts.push({ name: file.field, fileRef: file.ref, filename: file.filename });
    return { url: ctx.baseUrl + "/v1/videos", method: "POST", headers, bodyType: "multipart", parts };
  }
  headers["Content-Type"] = "application/json";
  const path = request.format === "native" ? "/v1/video/generations" : "/v1/videos";
  return { url: ctx.baseUrl + path, method: "POST", headers, body: payload };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  if (!body || typeof body.id !== "string" || !body.id.trim()) throw new Error("upstream task ID missing");
  return { taskId: body.id, taskData: body };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const request = ctx.requestBody;
  if (!request || !request.payload || !Number.isInteger(request.seconds)) throw new Error("invalid video request");
  const safeFields = ["model", "prompt", "seconds", "duration", "size", "resolution", "ratio", "aspect_ratio", "mode", "image", "images", "watermark_info"];
  // contents and metadata are read item by item: only their video entries add billed input seconds.
  const payload = request.payload;
  const listed = [].concat(Array.isArray(payload.contents) ? payload.contents : [], payload.metadata && payload.metadata.input && Array.isArray(payload.metadata.input.media) ? payload.metadata.input.media : []);
  const metadataVideo = payload.metadata && payload.metadata.input && payload.metadata.input.video_url !== undefined;
  const mayHaveVideo = listed.some((item) => !item || ["feature_video", "base_video", "reference_video"].includes(item.type)) || metadataVideo ||
    Object.keys(payload).some((key) => !safeFields.includes(key) && key !== "contents" && key !== "metadata") || (ctx.files || []).length > 0;
  const seconds = request.seconds;
  if (request.payload.model === "wan3.0-video") return { cost: (seconds + (mayHaveVideo ? 30 : 0)) * 0.33 };
  // Reserve for the largest published resolution and up to 30 seconds of reference video.
  const isFlash = request.payload.model === "kling-4.0-flash";
  const noVideo = seconds * (isFlash ? 0.492 : 1.23);
  const withVideo = (seconds + Math.max(seconds, 30)) * (isFlash ? 0.328 : 0.738);
  return { cost: mayHaveVideo ? Math.max(noVideo, withVideo) : noVideo };
}

function upstreamTask(ctx) {
  const response = utils.fetch({
    url: ctx.baseUrl + "/v1/video/generations/" + encodeURIComponent(ctx.taskId),
    method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey }, responseType: "json",
  });
  if (response.status !== 200 || !response.body || response.body.code !== "success" || !response.body.data) throw new Error("upstream task details unavailable");
  return response.body.data;
}

export function extractUsageOnComplete(ctx, result, body) {
  if (!body || body.status !== "completed" || !ctx.state || typeof ctx.state.finalCost !== "number") return null;
  return { cost: ctx.state.finalCost };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object") return { status: "UNKNOWN", reason: "invalid upstream task response" };
  if (body.status === "queued" || body.status === "pending") return { status: "QUEUED", progress: "0%" };
  if (body.status === "in_progress" || body.status === "processing") return { status: "IN_PROGRESS", progress: Number.isFinite(body.progress) ? body.progress + "%" : "50%" };
  if (body.status === "completed") {
    if (ctx.state && typeof ctx.state.finalCost === "number" && Number.isFinite(ctx.state.finalCost) && ctx.state.finalCost >= 0) return { status: "SUCCESS", progress: "100%" };
    const task = upstreamTask(ctx);
    if (task.status !== "SUCCESS") return { status: "UNKNOWN", reason: "upstream task status is inconsistent" };
    const billing = task.billing;
    if (billing && billing.settlement_status === "pending") return { status: "IN_PROGRESS", progress: "99%" };
    if (!billing || billing.settlement_status !== "settled" || typeof billing.final_amount !== "number" || !Number.isFinite(billing.final_amount) || billing.final_amount < 0) {
      throw new Error("upstream final cost is unavailable");
    }
    return { status: "IN_PROGRESS", progress: "99%", state: { finalCost: billing.final_amount } };
  }
  if (body.status === "failed" || body.status === "cancelled" || body.status === "canceled") return { status: "FAILURE", progress: "100%", reason: (body.error && body.error.message) || "upstream task failed" };
  return { status: "UNKNOWN", reason: "unrecognized upstream task status" };
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video" || !ctx.upstreamTaskId) throw new Error("video artifact is unavailable");
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.upstreamTaskId) + "/content", method: ctx.clientRequest.method, headers: { Authorization: "Bearer " + ctx.apiKey } };
}
