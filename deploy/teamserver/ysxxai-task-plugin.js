const models = [
  "doubao-seedance-2-5-260628-2-720P",
  "doubao-seedance-2-5-260628-2-1080P",
  "doubao-seedance-2-0-260128-2-720P",
  "doubao-seedance-2-0-260128-2-1080P",
  "doubao-seedance-2-0-260128-2-4k",
  "doubao-seedance-2-0-fast-260128-2-720P",
  "doubao-seedance-2-0-mini-260615-2-720P",
];
const estimatedYuanPerSecond = [1.134, 3.0873, 0.7452, 2.061, 2.1426, 0.5994, 0.3726];

export const meta = {
  apiVersion: 1,
  key: "ysxxai-seedance",
  name: "YSXXAI Seedance",
  version: "1.6.0",
  author: { name: "HFP" },
  description: { en: "YSXXAI Seedance video tasks", zh: "YSXXAI Seedance 视频任务" },
  baseUrl: "https://www.ysxxai.com",
  models,
  fetchMode: "per_task",
  usageSchema: {
    cost: { type: "number", unit: "credit", description: { en: "Upstream cost in CNY", zh: "上游人民币费用" } },
  },
  routes: [
    { method: "POST", path: "/ysxxai-seedance/v1/video/generations", type: "submit", decode: "create", render: "created" },
    { method: "GET", path: "/ysxxai-seedance/v1/video/generations/:task_id", type: "query", render: "status" },
  ],
};

function validate(body) {
  if (!body || typeof body !== "object" || Array.isArray(body) || !models.includes(body.model)) throw new Error("unsupported Seedance model");
  if (typeof body.prompt !== "string" || !body.prompt.trim()) throw new Error("prompt is required");
  const metadata = body.metadata;
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata is required");
  if (!Number.isInteger(metadata.duration) || metadata.duration < 4 || metadata.duration > 15) throw new Error("duration must be 4 to 15 seconds");
  const resolution = body.model.split("-").pop().toLowerCase();
  if (String(metadata.resolution || "").toLowerCase() !== resolution) throw new Error("resolution must match the model");
  if (metadata.content !== undefined && !Array.isArray(metadata.content)) throw new Error("content must be an array");
  const content = metadata.content || [];
  if (content.length && (body.model.includes("-fast-") || body.model.includes("-mini-"))) throw new Error("Fast and Mini models do not support references on this channel");
  for (const item of content) {
    const roles = { image_url: ["reference_image", "first_frame", "last_frame"], video_url: ["reference_video"], audio_url: ["reference_audio"] };
    const url = item && item[item.type]?.url;
    if (!item || !roles[item.type] || !roles[item.type].includes(item.role) || typeof url !== "string" || !(/^https:\/\/[^\s/]+/i.test(url) || /^asset:\/\/[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$/.test(url))) {
      throw new Error("reference media must have a supported role and a public HTTPS URL or trusted asset ID");
    }
  }
  if (body.model.includes("seedance-2-5-") && content.some((item) => item.role === "first_frame" || item.role === "last_frame")) {
    return { ...body, metadata: { ...metadata, ratio: "adaptive" } };
  }
  return body;
}

function workbenchRequest(body) {
  if (!body || body.metadata !== undefined) return body;
  const content = [];
  for (const [field, type, role] of [["images", "image_url", "reference_image"], ["videos", "video_url", "reference_video"], ["audios", "audio_url", "reference_audio"]]) {
    if (body[field] !== undefined && !Array.isArray(body[field])) throw new Error(field + " must be an array");
    for (const item of body[field] || []) {
      const url = field === "images" ? item && item.url : item;
      content.push({ type, [type]: { url }, role: field === "images" && item ? item.role || role : role });
    }
  }
  const metadata = { duration: body.duration, ratio: body.ratio, resolution: body.resolution, generate_audio: body.generate_audio, watermark: body.watermark };
  if (content.length) metadata.content = content;
  return { model: body.model, prompt: body.prompt, metadata };
}

export const native = {
  create(ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
    const body = validate(workbenchRequest(ctx.body.value));
    return { kind: "submit", model: body.model, action: body.metadata.content?.length ? "image_to_video" : "text_to_video", requestBody: body };
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

export function buildSubmitRequest(ctx) {
  const body = validate(ctx.requestBody);
  return {
    url: ctx.baseUrl + "/v1/video/generations", method: "POST",
    headers: { "Content-Type": "application/json", Authorization: "Bearer " + ctx.apiKey },
    body: { ...body, model: ctx.upstreamModel || body.model },
  };
}

export function parseSubmitResponse(ctx, response) {
  if (!response.body || typeof response.body.id !== "string" || !response.body.id.trim()) throw new Error("upstream task ID missing");
  return { taskId: response.body.id, taskData: response.body };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const body = validate(ctx.requestBody);
  const hasVideo = (body.metadata.content || []).some((item) => item.type === "video_url");
  const extraSeconds = hasVideo ? (body.model.includes("seedance-2-5-") ? 30 : 15) : 0;
  return { cost: (body.metadata.duration + extraSeconds) * estimatedYuanPerSecond[models.indexOf(body.model)] };
}

function upstreamTask(ctx) {
  const response = utils.fetch({
    url: ctx.baseUrl + "/v1/video/generations/" + encodeURIComponent(ctx.taskId),
    method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey }, responseType: "json",
  });
  if (response.status !== 200 || !response.body || response.body.code !== "success" || !response.body.data) throw new Error("upstream task details unavailable");
  return response.body.data;
}

function settledCost(task) {
  const billing = task && task.billing;
  if (!billing || billing.settlement_status !== "settled" || typeof billing.final_amount !== "number" || !Number.isFinite(billing.final_amount) || billing.final_amount < 0) {
    throw new Error("upstream final cost is unavailable");
  }
  return billing.final_amount;
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
  if (body.status === "queued") return { status: "QUEUED", progress: "0%" };
  if (body.status === "in_progress" || body.status === "processing") return { status: "IN_PROGRESS", progress: Number.isFinite(body.progress) ? body.progress + "%" : "50%" };
  if (body.status === "completed") {
    if (ctx.state && typeof ctx.state.finalCost === "number" && Number.isFinite(ctx.state.finalCost) && ctx.state.finalCost >= 0) return { status: "SUCCESS", progress: "100%" };
    const task = upstreamTask(ctx);
    if (task.status !== "SUCCESS") return { status: "UNKNOWN", reason: "upstream task status is inconsistent" };
    if (task.billing && task.billing.settlement_status === "pending") return { status: "IN_PROGRESS", progress: "99%" };
    return { status: "IN_PROGRESS", progress: "99%", state: { finalCost: settledCost(task) } };
  }
  if (body.status === "failed") {
    const task = upstreamTask(ctx);
    return { status: "FAILURE", progress: "100%", reason: task.fail_reason || "upstream task failed without a reason" };
  }
  return { status: "UNKNOWN", reason: "unrecognized upstream task status" };
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video" || !ctx.upstreamTaskId) throw new Error("video artifact is unavailable");
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.upstreamTaskId) + "/content", method: ctx.clientRequest.method, headers: { Authorization: "Bearer " + ctx.apiKey } };
}
