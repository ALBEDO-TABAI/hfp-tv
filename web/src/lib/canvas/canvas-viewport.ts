import { motionDuration } from "@/lib/motion-tokens";
import type { CanvasNodeData, Position, ViewportTransform } from "@/types/canvas";

export type CanvasBounds = {
    left: number;
    top: number;
    right: number;
    bottom: number;
};

export type CanvasViewportSize = {
    width: number;
    height: number;
};

export function getCanvasNodesBounds(nodes: CanvasNodeData[]): CanvasBounds | null {
    if (!nodes.length) return null;
    let left = Number.POSITIVE_INFINITY;
    let top = Number.POSITIVE_INFINITY;
    let right = Number.NEGATIVE_INFINITY;
    let bottom = Number.NEGATIVE_INFINITY;

    nodes.forEach((node) => {
        left = Math.min(left, node.position.x);
        top = Math.min(top, node.position.y);
        right = Math.max(right, node.position.x + node.width);
        bottom = Math.max(bottom, node.position.y + node.height);
    });

    return { left, top, right, bottom };
}

export function viewportForBounds(bounds: CanvasBounds, viewportSize: CanvasViewportSize, options: { padding?: number; minScale?: number; maxScale?: number } = {}): ViewportTransform {
    const padding = options.padding ?? 96;
    const minScale = options.minScale ?? 0.05;
    const maxScale = options.maxScale ?? 1;
    const boundsWidth = Math.max(1, bounds.right - bounds.left);
    const boundsHeight = Math.max(1, bounds.bottom - bounds.top);
    const availableWidth = Math.max(1, viewportSize.width - padding * 2);
    const availableHeight = Math.max(1, viewportSize.height - padding * 2);
    const k = Math.min(maxScale, Math.max(minScale, Math.min(availableWidth / boundsWidth, availableHeight / boundsHeight)));
    const centerX = (bounds.left + bounds.right) / 2;
    const centerY = (bounds.top + bounds.bottom) / 2;

    return {
        x: viewportSize.width / 2 - centerX * k,
        y: viewportSize.height / 2 - centerY * k,
        k,
    };
}

/** 镜头能否参与插值：坐标都是有限数、缩放为正。窄视口算出的负缩放或 NaN 不能进对数插值。 */
export function isUsableViewport(viewport: ViewportTransform): boolean {
    return Number.isFinite(viewport.x) && Number.isFinite(viewport.y) && Number.isFinite(viewport.k) && viewport.k > 0;
}

export function viewportAtScale(viewport: ViewportTransform, viewportSize: CanvasViewportSize, scale: number): ViewportTransform {
    const k = Math.min(8, Math.max(0.05, scale));
    const center = { x: viewportSize.width / 2, y: viewportSize.height / 2 };
    const world = worldPointAt(viewport, center);
    return {
        x: center.x - world.x * k,
        y: center.y - world.y * k,
        k,
    };
}

export function interpolateViewport(from: ViewportTransform, to: ViewportTransform, progress: number, focus: Position): ViewportTransform {
    if (progress <= 0) return from;
    if (progress >= 1) return to;
    const k = from.k * Math.pow(to.k / from.k, progress);
    const start = worldPointAt(from, focus);
    const end = worldPointAt(to, focus);
    return {
        x: focus.x - (start.x + (end.x - start.x) * progress) * k,
        y: focus.y - (start.y + (end.y - start.y) * progress) * k,
        k,
    };
}

export function viewportTransitionDuration(from: ViewportTransform, to: ViewportTransform, size: CanvasViewportSize, focus: Position): number {
    const start = worldPointAt(from, focus);
    const end = worldPointAt(to, focus);
    const screenDistance = Math.hypot(end.x - start.x, end.y - start.y) * Math.sqrt(from.k * to.k);
    const panDistance = screenDistance / Math.max(1, Math.hypot(size.width, size.height));
    const zoomDistance = Math.log2(to.k / from.k);
    // 一次倍增或一个视口对角线的平移用 stage，最远加一档 panel 到 stage 的差值。
    const distance = Math.hypot(zoomDistance, panDistance);
    return motionDuration.panel + Math.min(distance, 2) * (motionDuration.stage - motionDuration.panel);
}

function worldPointAt(viewport: ViewportTransform, focus: Position): Position {
    return {
        x: (focus.x - viewport.x) / viewport.k,
        y: (focus.y - viewport.y) / viewport.k,
    };
}
