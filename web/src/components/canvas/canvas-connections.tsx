import React, { useEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";
import { STORYBOARD_HEADER_HEIGHT, STORYBOARD_ROW_HEIGHT, storyboardTableHeight } from "@/lib/canvas/canvas-storyboard-layout";
import { batchReferenceHandleY } from "@/lib/canvas/canvas-batch-table";
import type { CanvasConnection, CanvasNodeData, ConnectionHandle, Position } from "@/types/canvas";

export const ConnectionPath = React.memo(function ConnectionPath({
    connection,
    from,
    to,
    fromScrollTop = 0,
    toScrollTop = 0,
    active,
    visualMode = "full",
    hideVisual = false,
    onSelect,
    onContextMenu,
}: {
    connection: CanvasConnection;
    from: CanvasNodeData;
    to: CanvasNodeData;
    fromScrollTop?: number;
    toScrollTop?: number;
    active: boolean;
    visualMode?: "full" | "hover-only";
    hideVisual?: boolean;
    onSelect: () => void;
    onContextMenu?: (event: ReactMouseEvent<SVGPathElement>) => void;
}) {
    const theme = canvasThemes[useActiveTheme()];
    const [hovered, setHovered] = useState(false);
    const { pathD, startX, startY, endX, endY } = canvasConnectionPath(connection, from, to, fromScrollTop, toScrollTop);
    const emphasized = active || hovered;
    // The public read-only LibTV overview uses hairline, low-contrast graph
    // wires at 10% zoom. Keep the fixture's wires visually faithful without
    // changing the normal interactive connection treatment.
    const denseReadonlyWire = connection.id.startsWith("libtv-dense-");
    const showVisual = shouldShowConnectionVisual({ visualMode, hovered, hideVisual });
    // 选中节点的那一刻沿连线跑一次光点，交代从哪连到哪；悬停不触发，平移时重新挂载的连线也不重播。
    const [traceRun, setTraceRun] = useState(0);
    const wasActive = useRef(active);
    useEffect(() => {
        if (active && !wasActive.current) setTraceRun((run) => run + 1);
        if (!active) setTraceRun(0);
        wasActive.current = active;
    }, [active]);
    const showTrace = !hideVisual && active && traceRun > 0 && !denseReadonlyWire;
    // 强调态只换颜色和粗细，120ms 过渡；不再常驻流动虚线、流星和模糊光晕（每帧重绘，选中后一直在余光里动）。
    const emphasisTransition = "stroke var(--motion-instant) var(--ease-out), stroke-width var(--motion-instant) var(--ease-out), stroke-opacity var(--motion-instant) var(--ease-out)";

    return (
        <g data-canvas-connection-id={connection.id}>
            <path
                data-connection-id={connection.id}
                data-connection-active={active ? "true" : "false"}
                d={pathD}
                stroke="transparent"
                strokeWidth="16"
                vectorEffect="non-scaling-stroke"
                fill="none"
                style={{ cursor: "pointer", pointerEvents: "stroke" }}
                onMouseEnter={() => setHovered(true)}
                onMouseLeave={() => setHovered(false)}
                onClick={(event) => {
                    event.stopPropagation();
                    onSelect();
                }}
                onContextMenu={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    onContextMenu?.(event);
                }}
            />
            {showVisual ? <path
                d={pathD}
                stroke={theme.node.muted}
                strokeWidth={denseReadonlyWire ? 0.75 : emphasized ? 5 : 2.5}
                vectorEffect="non-scaling-stroke"
                strokeOpacity={denseReadonlyWire ? 0.12 : emphasized ? 0.18 : 0.1}
                fill="none"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeDasharray={denseReadonlyWire ? "2 12" : undefined}
                style={{ pointerEvents: "none", transition: emphasisTransition }}
            /> : null}
            {showVisual ? <path
                d={pathD}
                stroke={emphasized ? theme.accent.primary : theme.node.muted}
                strokeWidth={denseReadonlyWire ? 0.75 : emphasized ? 2.8 : 1.5}
                vectorEffect="non-scaling-stroke"
                strokeOpacity={denseReadonlyWire ? 0.16 : emphasized ? 0.95 : 0.62}
                fill="none"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeDasharray={denseReadonlyWire ? "2 12" : undefined}
                style={{ pointerEvents: "none", transition: emphasisTransition }}
            /> : null}
            {showVisual ? <>
                <circle cx={startX} cy={startY} r={denseReadonlyWire ? 0.35 : emphasized ? 3.5 : 2} fill={emphasized ? theme.accent.primary : theme.node.muted} fillOpacity={denseReadonlyWire ? 0.12 : emphasized ? 0.9 : 0.56} vectorEffect="non-scaling-stroke" style={{ pointerEvents: "none" }} />
                <circle cx={endX} cy={endY} r={denseReadonlyWire ? 0.35 : emphasized ? 3.5 : 2} fill={emphasized ? theme.accent.primary : theme.node.muted} fillOpacity={denseReadonlyWire ? 0.12 : emphasized ? 0.9 : 0.56} vectorEffect="non-scaling-stroke" style={{ pointerEvents: "none" }} />
            </> : null}
            {showTrace ? <path
                key={traceRun}
                className="canvas-connection-trace"
                d={pathD}
                pathLength={100}
                stroke={theme.accent.primary}
                strokeWidth="3"
                vectorEffect="non-scaling-stroke"
                fill="none"
                strokeLinecap="round"
                style={{ pointerEvents: "none" }}
                onAnimationEnd={() => setTraceRun(0)}
                // 平移时路径被隐藏会取消动画：同样算这次播完，停下后不再从头重播。React 没有 animationcancel 的合成事件。
                ref={(path) => {
                    if (!path) return;
                    const finish = () => setTraceRun(0);
                    path.addEventListener("animationcancel", finish);
                    return () => path.removeEventListener("animationcancel", finish);
                }}
            /> : null}
        </g>
    );
}, (previous, next) => previous.connection === next.connection && previous.from === next.from && previous.to === next.to && previous.active === next.active && previous.visualMode === next.visualMode && previous.hideVisual === next.hideVisual && previous.fromScrollTop === next.fromScrollTop && previous.toScrollTop === next.toScrollTop);

export function canvasConnectionPath(connection: CanvasConnection, from: CanvasNodeData, to: CanvasNodeData, fromScrollTop = 0, toScrollTop = 0) {
    if (connection.pathD) return { pathD: connection.pathD, startX: from.position.x + from.width, startY: from.position.y + from.height / 2, endX: to.position.x, endY: to.position.y + to.height / 2 };
    const startX = from.position.x + from.width;
    const startY = connectionHandleY(from, connection.fromHandleId, fromScrollTop);
    const endX = to.position.x;
    const endY = connectionHandleY(to, connection.toHandleId, toScrollTop);
    const dx = Math.abs(endX - startX);
    const curvature = Math.max(dx * 0.5, 50);
    return { pathD: `M ${startX} ${startY} C ${startX + curvature} ${startY}, ${endX - curvature} ${endY}, ${endX} ${endY}`, startX, startY, endX, endY };
}

export function shouldShowConnectionVisual({ visualMode, hovered, hideVisual }: { visualMode: "full" | "hover-only"; hovered: boolean; hideVisual: boolean }) {
    return !hideVisual && (visualMode === "full" || hovered);
}

export function activeConnectionPath(node: CanvasNodeData | undefined, handle: ConnectionHandle, mouseWorld: Position, target?: CanvasNodeData, nodeScrollTop = 0) {
    if (!node) return "";
    const startX = handle.handleType === "source" ? node.position.x + node.width : mouseWorld.x;
    const startY = handle.handleType === "source" ? connectionHandleY(node, handle.handleId, nodeScrollTop) : mouseWorld.y;
    const endX = handle.handleType === "source" ? mouseWorld.x : node.position.x;
    const endY = handle.handleType === "source" ? mouseWorld.y : connectionHandleY(node, handle.handleId, nodeScrollTop);
    const snappedStartX = handle.handleType === "target" && target ? target.position.x + target.width : startX;
    const snappedStartY = handle.handleType === "target" && target ? connectionHandleY(target) : startY;
    const snappedEndX = handle.handleType === "source" && target ? target.position.x : endX;
    const snappedEndY = handle.handleType === "source" && target ? connectionHandleY(target) : endY;
    const distance = Math.abs(snappedEndX - snappedStartX);
    return `M ${snappedStartX} ${snappedStartY} C ${snappedStartX + distance * 0.5} ${snappedStartY}, ${snappedEndX - distance * 0.5} ${snappedEndY}, ${snappedEndX} ${snappedEndY}`;
}

/**
 * 连线在节点上的接入 Y。
 *
 * 单端口的一侧（左右各一个出入口，除分镜脚本外都是）**永远取边的正中**：
 * 之前按鼠标落点比例取 Y，多条线接同一个端口就会沿着边散开，视觉上像节点有很多端口。
 * 真正的多端口只存在于分镜脚本的 `row:` 句柄；普通节点始终连接到边缘
 * 垂直中心，避免同一个节点因鼠标落点产生漂移的“伪端口”。
 */
export function connectionHandleY(node: CanvasNodeData, handleId?: string, scrollTop = 0) {
    const batchY = batchReferenceHandleY(node, handleId);
    if (batchY !== undefined) return batchY;
    if (handleId === "storyboard:context") return node.position.y + node.height - (node.metadata?.storyboardComposerHeight || 104) / 2;
    if (!handleId?.startsWith("row:")) return node.position.y + node.height / 2;
    const rowId = handleId.slice(4);
    const index = (node.metadata?.storyboard?.rows || []).findIndex((row) => row.id === rowId);
    if (index < 0) return node.position.y + node.height / 2;
    const tableHeight = storyboardTableHeight(node.height, node.metadata?.storyboardComposerHeight);
    const localY = Math.min(Math.max(index * STORYBOARD_ROW_HEIGHT + STORYBOARD_ROW_HEIGHT / 2 - scrollTop, 4), tableHeight - 4);
    return node.position.y + STORYBOARD_HEADER_HEIGHT + localY;
}
