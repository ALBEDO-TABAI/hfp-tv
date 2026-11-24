import { CanvasNodeType, type CanvasMediaPerformanceMode, type CanvasNodeData } from "@/types/canvas";

const STORAGE_KEY = "canvas-media-performance-mode";

// Overview cost is bounded by the LOD canvas (canvas-lod.ts), not a DOM node
// count budget: a count cap hid visible nodes.
export const CANVAS_MAX_RENDERED_CONNECTIONS = 5000;

export function readCanvasMediaPerformanceMode(): CanvasMediaPerformanceMode {
    try {
        const stored = window.localStorage.getItem(STORAGE_KEY);
        return stored === "quality" || stored === "performance" ? stored : "auto";
    } catch {
        return "auto";
    }
}

export function persistCanvasMediaPerformanceMode(mode: CanvasMediaPerformanceMode) {
    try {
        window.localStorage.setItem(STORAGE_KEY, mode);
    } catch {
        // 浏览器禁用本地存储时保留当前会话内的选择。
    }
}

/** fluidMode：应用的「流畅模式」开着时，「自动性能」按性能优先处理；用户明确选了画质优先则照办。 */
export function shouldReduceCanvasMediaEffects(mode: CanvasMediaPerformanceMode, nodes: CanvasNodeData[], fluidMode = false) {
    if (mode === "performance") return true;
    if (mode === "quality") return false;
    if (fluidMode) return true;
    const mediaCount = nodes.filter((node) => node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video || node.type === CanvasNodeType.Audio).length;
    return nodes.length >= 80 || mediaCount >= 32;
}

export function canvasNodeRenderPadding(reduceMediaEffects: boolean, previouslyRendered: boolean) {
    if (previouslyRendered) return reduceMediaEffects ? 640 : 384;
    return reduceMediaEffects ? 128 : 192;
}

export function resolveActiveCanvasMediaNodeId(selectedNodeIds: ReadonlySet<string>, nodeById: ReadonlyMap<string, CanvasNodeData>) {
    if (selectedNodeIds.size !== 1) return null;
    const nodeId = selectedNodeIds.values().next().value;
    if (!nodeId) return null;
    const type = nodeById.get(nodeId)?.type;
    return type === CanvasNodeType.Video || type === CanvasNodeType.Audio ? nodeId : null;
}
