import type { Publication, Snapshot, Version } from "@/services/api/workshop";
import type { CanvasNodeData } from "@/types/canvas";

export function workshopFileUrl(sha: string, versionId: string, download = false) {
    return `/api/community/files/${encodeURIComponent(sha)}?v=${encodeURIComponent(versionId)}${download ? "&download=1" : ""}`;
}

// Snapshot JSON is copied, never changed in place; only whole placeholder strings are replaced.
export function resolveWorkshopMedia<T>(value: T, versionId: string): T;
export function resolveWorkshopMedia(value: unknown, versionId: string): unknown {
    if (typeof value === "string") {
        const match = /^workshop:(?:\/\/)?([a-f0-9]{64})$/i.exec(value);
        return match ? workshopFileUrl(match[1], versionId) : value;
    }
    if (Array.isArray(value)) return value.map((item: unknown) => resolveWorkshopMedia(item, versionId));
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, resolveWorkshopMedia(item, versionId)]));
    return value;
}

export function workshopCanvasNodes(canvas: Snapshot["canvases"][number], versionId: string): CanvasNodeData[] {
    return canvas.document.nodes.map((original) => {
        const node = resolveWorkshopMedia(original, versionId);
        const drawing = node.type === "drawing" ? canvas.drawings.find((item) => item.drawingId === node.metadata?.drawingId) : undefined;
        const preview = drawing?.preview ? workshopFileUrl(drawing.preview, versionId) : undefined;
        const storage = node.metadata?.storageKey;
        const content = preview || node.metadata?.content || (storage?.startsWith("/api/community/files/") ? storage : undefined) || (node.type === "image" ? node.metadata?.previewContent : undefined);
        return { ...node, metadata: { ...node.metadata, content, drawingPreviewUrl: preview } };
    });
}

function referencesSha(value: unknown, sha: string): boolean {
    if (typeof value === "string") return value === `workshop:${sha}` || value === `workshop://${sha}`;
    if (Array.isArray(value)) return value.some((item: unknown) => referencesSha(item, sha));
    return Boolean(value && typeof value === "object" && Object.values(value).some((item) => referencesSha(item, sha)));
}

export function findWorkshopFileNode(snapshot: Snapshot, sha: string): { canvasKey: string; nodeId: string } | undefined {
    for (const canvas of snapshot.canvases) {
        const drawings = canvas.drawings.filter((drawing) => drawing.preview === sha || drawing.render?.sha256 === sha || referencesSha(drawing.snapshot, sha));
        const node = canvas.document.nodes.find((item) => referencesSha(item.metadata, sha) || drawings.some((drawing) => drawing.drawingId === item.metadata?.drawingId));
        if (node) return { canvasKey: canvas.key, nodeId: node.id };
    }
}

export function groupWorkshopFiles(files: Snapshot["files"]) {
    const groups: Record<"图片" | "视频" | "音频" | "画板" | "其他", Snapshot["files"]> = { 图片: [], 视频: [], 音频: [], 画板: [], 其他: [] };
    for (const file of files) {
        const group = file.kind.startsWith("drawing") ? "画板" : file.mimeType.startsWith("image/") || file.kind === "image" ? "图片" : file.mimeType.startsWith("video/") || file.kind === "video" ? "视频" : file.mimeType.startsWith("audio/") || file.kind === "audio" ? "音频" : "其他";
        groups[group].push(file);
    }
    return Object.entries(groups).filter(([, items]) => items.length > 0);
}

export function workshopSubmissionStatus(publication: Pick<Publication, "status">, latest: Pick<Version, "reviewStatus">) {
    if (latest.reviewStatus === "pending") return "待审核";
    if (latest.reviewStatus === "rejected") return "已驳回";
    if (latest.reviewStatus === "withdrawn") return "已撤回";
    return publication.status === "unlisted" ? "已下架" : "已发布";
}

export function workshopNodeParameters(metadata: CanvasNodeData["metadata"]) {
    const rows: [string, string | number][] = [];
    const visit = (value: unknown, prefix = "") => {
        if (!value || typeof value !== "object" || Array.isArray(value)) return;
        for (const [key, item] of Object.entries(value)) {
            if (/id$|ids$|content|prompt|url|key|sha|token|secret|password|credential/i.test(key)) continue;
            const label = prefix ? `${prefix}.${key}` : key;
            if (typeof item === "string" || typeof item === "number") rows.push([label, item]);
            else if (key === "parameters" || key === "params" || key === "settings" || key === "generation") visit(item, label);
        }
    };
    visit(metadata);
    return rows;
}
