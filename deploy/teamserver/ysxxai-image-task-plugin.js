// YSXXAI image models on the company relay (New API task plugin).
// The supplier's contract differs from OpenAI Images in three ways, all
// translated here so callers keep sending a standard OpenAI Images request:
//   - the resolution tier is part of the model name (gpt-image-2.5-4k);
//   - `size` is an aspect ratio ("4:3"), pixel sizes are refused;
//   - edits take public links in `image_urls`, uploaded files are refused.
const models = ["gemini-nano-banana-2.1", "gpt-image-2.5"];
const resolutionTiers = {
  [models[0]]: ["1k", "2k", "4k"],
  [models[1]]: ["2k", "4k"],
};
const ratios = {
  [models[0]]: ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "5:4", "4:5", "21:9", "4:1", "1:4", "8:1", "1:8"],
  [models[1]]: ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "5:4", "4:5", "21:9"],
};

function usageSchema(tiers) {
  return {
    resolution: { enum: tiers, description: { en: "Output resolution", zh: "输出分辨率" } },
    image_count: { type: "number", unit: "count", description: { en: "Image generation unit price", zh: "图片生成单价" } },
  };
}

export const meta = {
  apiVersion: 1,
  key: "ysxxai-image",
  name: "YSXXAI Image",
  version: "1.1.0",
  author: { name: "HFP" },
  description: { en: "YSXXAI image generation", zh: "YSXXAI 图片生成" },
  baseUrl: "https://os.ysxxai.com",
  models,
  fetchMode: "per_task",
  protocols: ["openai_image"],
  routes: [],
  usageSchema: usageSchema(["1k", "2k", "4k"]),
  usageProfiles: models.map(model => ({ models: [model], schema: usageSchema(resolutionTiers[model]) })),
};

function nearestRatio(model, width, height) {
  const target = Math.log(width / height);
  let best = "";
  let distance = Infinity;
  for (const ratio of ratios[model]) {
    const [w, h] = ratio.split(":").map(Number);
    const gap = Math.abs(Math.log(w / h) - target);
    if (gap < distance) {
      best = ratio;
      distance = gap;
    }
  }
  return best;
}

// readSize accepts what callers send in `size`, `resolution` or
// `aspect_ratio`: a tier (4k), a pixel size (3264x2448), a ratio (4:3) or auto.
function readSize(model, name, value) {
  if (value === undefined || value === null || value === "") return {};
  if (typeof value !== "string") throw new Error(name + " must be a string");
  const text = value.trim().toLowerCase();
  if (text === "" || text === "auto") return {};
  if (["1k", "2k", "4k"].includes(text)) return { tier: text };
  const pixels = /^(\d+)x(\d+)$/.exec(text);
  if (pixels) {
    const width = Number(pixels[1]);
    const height = Number(pixels[2]);
    if (!width || !height || width > 4096 || height > 4096) throw new Error("invalid image size");
    const area = width * height;
    return { tier: area <= 2000000 ? "1k" : area <= 4300000 ? "2k" : "4k", ratio: nearestRatio(model, width, height) };
  }
  if (/^\d+:\d+$/.test(text)) {
    if (!ratios[model].includes(text)) throw new Error(model + " supports ratios " + ratios[model].join(", "));
    return { ratio: text };
  }
  throw new Error(name + " must be 1k, 2k, 4k, WIDTHxHEIGHT, W:H or auto");
}

function agreed(name, values) {
  const given = values.filter(Boolean);
  if (given.some(value => value !== given[0])) throw new Error("size, resolution and aspect_ratio must agree on " + name);
  return given[0] || "";
}

function imageLinks(value) {
  const list = value === undefined || value === null || value === "" ? [] : Array.isArray(value) ? value : [value];
  for (const link of list) {
    if (typeof link !== "string" || !/^https?:\/\/\S+$/i.test(link.trim())) throw new Error("image_urls must be http(s) links");
  }
  return list.map(link => link.trim());
}

function decode(ctx) {
  if (!models.includes(ctx.model)) throw new Error("unsupported image model");
  if (ctx.operation !== "generate" && ctx.operation !== "edit") throw new Error("unsupported image operation");
  if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
  let payload;
  if (ctx.body.kind === "json") {
    if (!ctx.body.value || typeof ctx.body.value !== "object" || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    payload = { ...ctx.body.value };
  } else {
    // The supplier only reads public links; refusing here keeps the caller
    // from uploading megabytes that would be rejected one hop later.
    if ((ctx.body.files || []).length) throw new Error("uploaded image files are not supported by this model; send public links in image_urls");
    const fields = ctx.body.fields || {};
    payload = {};
    for (const name of Object.keys(fields)) {
      if (name === "image_urls" || name === "image_urls[]") {
        payload.image_urls = fields[name];
        continue;
      }
      if (fields[name].length !== 1) throw new Error(name + " must be provided once");
      payload[name] = fields[name][0];
    }
  }
  const size = readSize(ctx.model, "size", payload.size);
  const explicit = readSize(ctx.model, "resolution", payload.resolution || payload.image_size);
  const aspect = readSize(ctx.model, "aspect_ratio", payload.aspect_ratio);
  if (explicit.ratio && !/^\d+x\d+$/i.test(String(payload.resolution || payload.image_size).trim())) throw new Error("resolution must be 1k, 2k or 4k");
  if (aspect.tier) throw new Error("aspect_ratio must be W:H or auto");
  const resolution = agreed("resolution", [explicit.tier, size.tier]) || (ctx.model === models[0] ? "1k" : "2k");
  if (!resolutionTiers[ctx.model].includes(resolution)) throw new Error(ctx.model + " supports " + resolutionTiers[ctx.model].join(" and "));
  const ratio = aspect.ratio || size.ratio || explicit.ratio || "";
  const count = payload.n === undefined || payload.n === "" ? 1 : Number(payload.n);
  if (!Number.isInteger(count) || count < 1 || count > 10) throw new Error("n must be between 1 and 10");
  if (payload.stream !== undefined && payload.stream !== false && payload.stream !== "false") throw new Error("stream is not supported");
  const links = imageLinks(payload.image_urls);
  if (ctx.operation === "edit" && !links.length) throw new Error("image edits need public links in image_urls");
  if (ctx.operation === "generate" && links.length) throw new Error("image_urls is only for image edits");
  if (typeof payload.prompt !== "string" || !payload.prompt.trim()) throw new Error("prompt is required");
  // Only what the supplier documents is forwarded: OpenAI-only options such
  // as quality or output_format mean nothing to it (the tier is the quality).
  return { kind: "submit", model: ctx.model, action: ctx.operation === "edit" ? "image_to_image" : "text_to_image", requestBody: { prompt: payload.prompt, operation: ctx.operation, resolution, ratio, count, imageUrls: links } };
}

export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody;
  if (!req || !models.includes(ctx.model) || !["1k", "2k", "4k"].includes(req.resolution)) throw new Error("invalid image request");
  const body = { prompt: req.prompt, model: ctx.model + "-" + req.resolution, n: req.count };
  if (req.ratio) body.size = req.ratio;
  if (req.operation === "edit") body.image_urls = req.imageUrls;
  return {
    url: ctx.baseUrl + (req.operation === "edit" ? "/v1/images/edits" : "/v1/images/generations"),
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response && response.body;
  const taskId = body && (body.task_id || body.id);
  if (typeof taskId !== "string" || !taskId) throw new Error("upstream task ID missing");
  if (body.status === "completed" && Array.isArray(body.data) && body.data.length) return { taskId, taskData: body, immediate: { status: "SUCCESS", progress: "100%" } };
  return { taskId, taskData: body };
}

export function extractUsage(ctx) {
  if (!ctx.requestBody) throw new Error("invalid image request");
  const { resolution, count } = ctx.requestBody;
  return ctx.usagePurpose === "billing_ratios" ? { image_count: count } : { resolution, image_count: count };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/tasks/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object") return { status: "UNKNOWN" };
  if (["queued", "pending", "submitted"].includes(body.status)) return { status: "QUEUED", progress: "0%" };
  if (["in_progress", "processing", "running"].includes(body.status)) return { status: "IN_PROGRESS", progress: "50%" };
  if (body.status === "completed" && Array.isArray(body.data) && body.data.length && body.data.every(item => item && (item.url || item.b64_json))) return { status: "SUCCESS", progress: "100%" };
  if (["failed", "cancelled", "canceled"].includes(body.status)) return { status: "FAILURE", reason: (body.error && body.error.message) || "image generation failed" };
  return { status: "UNKNOWN" };
}

export function extractUsageOnComplete(ctx, result, body) {
  if (!body || body.status !== "completed" || !Array.isArray(body.data) || !body.data.length) return {};
  return { image_count: body.data.length };
}

export const protocols = {
  openai_image: {
    decodeRequest: decode,
    render(ctx, task) {
      const entries = task.data && task.data.data;
      if (!Array.isArray(entries) || !entries.length) throw new Error("image result missing");
      return { data: entries.map(item => item.url ? { url: item.url } : { b64_json: item.b64_json }) };
    },
  },
};
