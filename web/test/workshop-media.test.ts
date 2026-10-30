import { describe, expect, test } from "bun:test";
import { findWorkshopFileNode, groupWorkshopFiles, resolveWorkshopMedia, workshopCanvasNodes, workshopFileUrl, workshopNodeParameters, workshopSubmissionStatus } from "../src/lib/workshop-media";
import type { Snapshot } from "../src/services/api/workshop";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";

const sha = "a".repeat(64);
const otherSha = "b".repeat(64);
const version = "version-one";
const node: CanvasNodeData = { id: "image-one", type: CanvasNodeType.Image, title: "图片", position: { x: 10, y: 20 }, width: 320, height: 180, metadata: { storageKey: `workshop:${sha}`, videoPreview: { content: `workshop://${otherSha}` } } };
const canvas: Snapshot["canvases"][number] = { key: "canvas-one", title: "主画布", document: { nodes: [node], connections: [] }, drawings: [] };
const snapshot: Snapshot = { format: "hfp-workshop-snapshot", version: 1, title: "测试作品", stats: { canvases: 1, nodes: 1, files: 1, bytes: 100, drawings: 0 }, canvases: [canvas], files: [{ sha256: sha, size: 100, mimeType: "image/png", kind: "image", name: "image.png" }] };

describe("workshop media", () => {
    test("resolves both whole-string placeholder forms with version-bound file URLs", () => {
        expect(resolveWorkshopMedia(`workshop:${sha}`, version)).toBe(`/api/community/files/${sha}?v=version-one`);
        expect(resolveWorkshopMedia(`workshop://${sha}`, version)).toBe(workshopFileUrl(sha, version));
        expect(workshopFileUrl(sha, "v/a?&", true)).toBe(`/api/community/files/${sha}?v=v%2Fa%3F%26&download=1`);
    });

    test("leaves ordinary strings, URLs, malformed placeholders and scalar values unchanged", () => {
        for (const value of ["https://example.com/image.png", "resource:private", "/api/resources/private/file", "workshop:invalid", `prompt workshop:${sha}`, "<script>alert(1)</script>", "", 42, false, null, undefined]) {
            expect(resolveWorkshopMedia(value, version)).toBe(value);
        }
    });

    test("resolves nested media and arrays without mutating the snapshot", () => {
        const source = { nodes: [node], timeline: { clips: [{ url: `workshop://${sha}` }] }, empty: null };
        const before = JSON.stringify(source);
        const result = resolveWorkshopMedia(source, version);
        expect(result.nodes[0].metadata?.storageKey).toBe(workshopFileUrl(sha, version));
        expect(result.nodes[0].metadata?.videoPreview?.content).toBe(workshopFileUrl(otherSha, version));
        expect(result.timeline.clips[0].url).toBe(workshopFileUrl(sha, version));
        expect(result.empty).toBeNull();
        expect(JSON.stringify(source)).toBe(before);
    });

    test("renders storage-only media and drawing previews without workspace lookups", () => {
        const drawing: CanvasNodeData = { ...node, id: "drawing-one", type: CanvasNodeType.Drawing, metadata: { drawingId: "drawing-data" } };
        const drawingCanvas = { ...canvas, document: { ...canvas.document, nodes: [node, drawing] }, drawings: [{ drawingId: "drawing-data", engine: "tldraw", snapshot: {}, shapeCount: 1, pageCount: 1, preview: otherSha }] };
        const result = workshopCanvasNodes(drawingCanvas, version);
        expect(result[0].metadata?.content).toBe(workshopFileUrl(sha, version));
        expect(result[1].type).toBe(CanvasNodeType.Drawing);
        expect(result[1].metadata?.content).toBe(workshopFileUrl(otherSha, version));
        expect(result[1].metadata?.drawingPreviewUrl).toBe(workshopFileUrl(otherSha, version));
        expect(drawing.metadata?.content).toBeUndefined();
        const previewOnly = workshopCanvasNodes({ ...canvas, document: { ...canvas.document, nodes: [{ ...node, metadata: { previewContent: `workshop://${sha}` } }] } }, version);
        expect(previewOnly[0].metadata?.content).toBe(workshopFileUrl(sha, version));
    });

    test("finds files across canvases, nested references, drawing previews and renders", () => {
        expect(findWorkshopFileNode(snapshot, sha)).toEqual({ canvasKey: canvas.key, nodeId: node.id });
        expect(findWorkshopFileNode(snapshot, otherSha)).toEqual({ canvasKey: canvas.key, nodeId: node.id });
        const second = { ...canvas, key: "second", document: { ...canvas.document, nodes: [{ ...node, id: "draw-node", metadata: { drawingId: "drawing" } }] }, drawings: [{ drawingId: "drawing", engine: "tldraw", snapshot: {}, shapeCount: 1, pageCount: 1, preview: sha, render: { sha256: otherSha } }] };
        const multi = { ...snapshot, canvases: [{ ...canvas, document: { ...canvas.document, nodes: [] } }, second] };
        expect(findWorkshopFileNode(multi, sha)).toEqual({ canvasKey: "second", nodeId: "draw-node" });
        expect(findWorkshopFileNode(multi, otherSha)).toEqual({ canvasKey: "second", nodeId: "draw-node" });
        expect(findWorkshopFileNode(snapshot, "c".repeat(64))).toBeUndefined();
    });

    test("groups drawing images separately and retains unknown attachments", () => {
        const base = snapshot.files[0];
        const files = [base, { ...base, kind: "video", mimeType: "video/mp4" }, { ...base, kind: "audio", mimeType: "audio/mpeg" }, { ...base, kind: "drawing-preview" }, { ...base, kind: "document", mimeType: "text/html" }];
        expect(groupWorkshopFiles(files).map(([group, items]) => [group, items.length])).toEqual([["图片", 1], ["视频", 1], ["音频", 1], ["画板", 1], ["其他", 1]]);
        expect(groupWorkshopFiles([])).toEqual([]);
    });

    test("parameters show scalar model data and omit media, prompts, ids and credentials", () => {
        expect(workshopNodeParameters({ model: "test-model", width: 1024, prompt: "prompt", content: "file", taskId: "task", storageKey: "secret", apiKey: "secret", parameters: { steps: 30, seed: 10, userId: "private" } })).toEqual([["model", "test-model"], ["width", 1024], ["parameters.steps", 30], ["parameters.seed", 10]]);
        expect(workshopNodeParameters(undefined)).toEqual([]);
    });
});

test("submission status keeps a new pending or rejected revision visible over an older publication", () => {
    expect(workshopSubmissionStatus({ status: "listed" }, { reviewStatus: "pending" })).toBe("待审核");
    expect(workshopSubmissionStatus({ status: "listed" }, { reviewStatus: "rejected" })).toBe("已驳回");
    expect(workshopSubmissionStatus({ status: "draft" }, { reviewStatus: "withdrawn" })).toBe("已撤回");
    expect(workshopSubmissionStatus({ status: "listed" }, { reviewStatus: "approved" })).toBe("已发布");
    expect(workshopSubmissionStatus({ status: "unlisted" }, { reviewStatus: "approved" })).toBe("已下架");
});
