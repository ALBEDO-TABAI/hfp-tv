import type { CanvasNodeData, Position } from "@/types/canvas";

export type CanvasConnectionApproach = { nodeId: string; point: Position } | null;

/** Capture only entry into a target, not every pointer frame or snap adjustment. */
export function latchCanvasConnectionApproach(previous: CanvasConnectionApproach, nodeId: string | null, point: Position): CanvasConnectionApproach {
    if (!nodeId) return null;
    return previous?.nodeId === nodeId ? previous : { nodeId, point: { ...point } };
}

/** 连线靠近时节点朝来向轻微倾斜（最多 ±3°），交代「要连到这里」；再大就显得在晃。 */
const MAX_TILT_DEGREES = 3;

/** Canvas-space coordinates keep the approach direction stable at every zoom level. */
export function canvasConnectionTilt(node: Pick<CanvasNodeData, "position" | "width" | "height">, point?: Position) {
    if (!point || node.width <= 0 || node.height <= 0) return undefined;
    const x = Math.max(0, Math.min(1, (point.x - node.position.x) / node.width));
    const y = Math.max(0, Math.min(1, (point.y - node.position.y) / node.height));
    if (!Number.isFinite(x) || !Number.isFinite(y)) return undefined;
    return { rotateX: (0.5 - y) * 2 * MAX_TILT_DEGREES, rotateY: (x - 0.5) * 2 * MAX_TILT_DEGREES, origin: `${x * 100}% ${y * 100}%` };
}
