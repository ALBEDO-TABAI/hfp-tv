import { useCallback, useEffect, useRef, type Dispatch, type MouseEvent, type SetStateAction } from "react";

import { isNodeHiddenByCollapsedFrame } from "@/lib/canvas/canvas-frame";
import { isHiddenBatchChild } from "@/lib/canvas/canvas-project-domain";
import { applyCanvasLiveViewport } from "@/lib/canvas/canvas-live-viewport";
import { canOpenCanvasNodePromptPanel, isCanvasMediaResultNode } from "@/lib/canvas/canvas-node-semantics";
import { getCanvasNodesBounds, viewportAtScale, viewportForBounds, type CanvasViewportSize } from "@/lib/canvas/canvas-viewport";
import { shouldRefreshCanvasVirtualization } from "@/lib/canvas/canvas-viewport-render-sync";
import { shouldRebaseCanvasRaster } from "@/lib/canvas/canvas-leafer-viewport";
import { motionDuration } from "@/lib/motion-tokens";
import { CanvasNodeType, type CanvasNodeData, type ContextMenuState, type Position, type ViewportTransform } from "@/types/canvas";
import { useCanvasViewportTransition } from "./use-canvas-viewport-transition";

type UseCanvasViewportControllerOptions = {
    containerRef: { current: HTMLDivElement | null };
    size: CanvasViewportSize;
    viewport: ViewportTransform;
    viewportRef: { current: ViewportTransform };
    nodesRef: { current: CanvasNodeData[] };
    selectedNodeIdsRef: { current: Set<string> };
    setViewport: Dispatch<SetStateAction<ViewportTransform>>;
    setVirtualizationViewport: Dispatch<SetStateAction<ViewportTransform | null>>;
    setSelectedNodeIds: Dispatch<SetStateAction<Set<string>>>;
    setSelectedConnectionId: Dispatch<SetStateAction<string | null>>;
    setContextMenu: Dispatch<SetStateAction<ContextMenuState | null>>;
    setDialogNodeId: Dispatch<SetStateAction<string | null>>;
    setToolbarNodeId: Dispatch<SetStateAction<string | null>>;
    /** True while a renderer culls per frame itself (overview LOD); previews then skip React refreshes. */
    virtualizationPausedRef?: { current: boolean };
};

export function useCanvasViewportController({
    containerRef,
    size,
    viewport,
    viewportRef,
    nodesRef,
    selectedNodeIdsRef,
    setViewport,
    setVirtualizationViewport,
    setSelectedNodeIds,
    setSelectedConnectionId,
    setContextMenu,
    setDialogNodeId,
    setToolbarNodeId,
    virtualizationPausedRef,
}: UseCanvasViewportControllerOptions) {
    const commitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const renderedViewportRef = useRef(viewportRef.current);
    const lastVirtualizationRefreshAtRef = useRef(0);

    const applyViewportPreview = useCallback((next: ViewportTransform) => {
        viewportRef.current = next;
        const container = containerRef.current;
        if (container && container.dataset.canvasViewportInteracting !== "true") container.dataset.canvasViewportInteracting = "true";
        applyCanvasLiveViewport(container, next);
    }, [containerRef, viewportRef]);

    const commitViewport = useCallback((next: ViewportTransform) => {
        if (commitTimerRef.current) {
            clearTimeout(commitTimerRef.current);
            commitTimerRef.current = null;
        }
        viewportRef.current = next;
        renderedViewportRef.current = next;
        lastVirtualizationRefreshAtRef.current = performance.now();
        delete containerRef.current?.dataset.canvasViewportInteracting;
        setViewport((current) => current.x === next.x && current.y === next.y && current.k === next.k ? current : next);
    }, [containerRef, setViewport, viewportRef]);

    useEffect(() => () => {
        if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
        delete containerRef.current?.dataset.canvasViewportInteracting;
    }, [containerRef]);

    // 程序化镜头每帧也要更新可见范围，否则缩小时画面外的节点要等镜头结束才出现。
    // 逐帧循环拿到的是开始时的回调，所以经 ref 调用最新的刷新函数。
    const refreshVirtualizationRef = useRef<(next: ViewportTransform) => void>(() => {});
    const previewTransitionViewport = useCallback((next: ViewportTransform) => {
        applyViewportPreview(next);
        refreshVirtualizationRef.current(next);
    }, [applyViewportPreview]);
    const { cancelViewportTransition, isViewportTransitionActive, transitionViewportTo, stepViewportZoom } = useCanvasViewportTransition(viewportRef, size, previewTransitionViewport, commitViewport);

    useEffect(() => {
        const interrupt = (event: PointerEvent) => {
            const container = containerRef.current;
            if (container && event.composedPath().includes(container) && cancelViewportTransition()) {
                commitViewport(viewportRef.current);
            }
        };
        document.addEventListener("pointerdown", interrupt, true);
        return () => document.removeEventListener("pointerdown", interrupt, true);
    }, [cancelViewportTransition, commitViewport, containerRef, viewportRef]);

    const previewViewport = useCallback((next: ViewportTransform) => {
        cancelViewportTransition();
        applyViewportPreview(next);
    }, [applyViewportPreview, cancelViewportTransition]);

    const screenToCanvas = useCallback((clientX: number, clientY: number): Position => {
        const rect = containerRef.current?.getBoundingClientRect();
        const viewport = viewportRef.current;
        const localX = clientX - (rect?.left || 0);
        const localY = clientY - (rect?.top || 0);
        return { x: (localX - viewport.x) / viewport.k, y: (localY - viewport.y) / viewport.k };
    }, [containerRef, viewportRef]);

    const getCanvasCenter = useCallback(() => {
        const rect = containerRef.current?.getBoundingClientRect();
        return screenToCanvas((rect?.left || 0) + (rect?.width || size.width) / 2, (rect?.top || 0) + (rect?.height || size.height) / 2);
    }, [containerRef, screenToCanvas, size.height, size.width]);

    const focusNodesInView = useCallback((targetNodes: CanvasNodeData[], maxScale = 1) => {
        const bounds = getCanvasNodesBounds(targetNodes);
        if (!bounds) return false;
        transitionViewportTo(viewportForBounds(bounds, size, { padding: 88, maxScale }));
        setContextMenu(null);
        return true;
    }, [setContextMenu, size, transitionViewportTo]);

    const fitCanvasContent = useCallback(() => {
        const nodes = nodesRef.current;
        return focusNodesInView(nodes.filter((node) => !isHiddenBatchChild(node, nodes) && !isNodeHiddenByCollapsedFrame(node, nodes)));
    }, [focusNodesInView, nodesRef]);

    const fitCanvasSelection = useCallback(() => {
        const nodes = nodesRef.current;
        return focusNodesInView(nodes.filter((node) => selectedNodeIdsRef.current.has(node.id) && !isHiddenBatchChild(node, nodes) && !isNodeHiddenByCollapsedFrame(node, nodes)), 1.25);
    }, [focusNodesInView, nodesRef, selectedNodeIdsRef]);

    const handleCanvasDoubleClick = useCallback((event: MouseEvent<HTMLDivElement>) => {
        event.preventDefault();
        event.stopPropagation();
        setSelectedNodeIds(new Set());
        setSelectedConnectionId(null);
        setDialogNodeId(null);
        setToolbarNodeId(null);
        setContextMenu({ type: "canvas", x: event.clientX, y: event.clientY, position: screenToCanvas(event.clientX, event.clientY), createOpen: true });
    }, [screenToCanvas, setContextMenu, setDialogNodeId, setSelectedConnectionId, setSelectedNodeIds, setToolbarNodeId]);

    const selectFocusedNode = useCallback((nodeId: string) => {
        const selection = new Set([nodeId]);
        selectedNodeIdsRef.current = selection;
        setSelectedNodeIds(selection);
        setSelectedConnectionId(null);
        setContextMenu(null);
    }, [selectedNodeIdsRef, setContextMenu, setSelectedConnectionId, setSelectedNodeIds]);

    const focusCanvasImageNode = useCallback((nodeId: string) => {
        const node = nodesRef.current.find((item) => item.id === nodeId && item.type === CanvasNodeType.Image);
        if (!node) return;
        const scale = Math.min(1.25, Math.max(viewportRef.current.k, 0.78));
        transitionViewportTo({ x: size.width / 2 - (node.position.x + node.width / 2) * scale, y: size.height / 2 - (node.position.y + node.height / 2) * scale, k: scale });
        selectFocusedNode(node.id);
        setDialogNodeId(null);
        setToolbarNodeId(node.id);
    }, [nodesRef, selectFocusedNode, setDialogNodeId, setToolbarNodeId, size.height, size.width, transitionViewportTo, viewportRef]);

    const focusCanvasNode = useCallback((nodeId: string) => {
        const node = nodesRef.current.find((item) => item.id === nodeId);
        if (!node) return;
        const scale = Math.min(1.18, Math.max(viewportRef.current.k, 0.72));
        transitionViewportTo({ x: size.width / 2 - (node.position.x + node.width / 2) * scale, y: size.height / 2 - (node.position.y + node.height / 2) * scale, k: scale });
        selectFocusedNode(node.id);
        setDialogNodeId(canOpenCanvasNodePromptPanel(node) ? node.id : null);
        setToolbarNodeId(isCanvasMediaResultNode(node) ? node.id : null);
    }, [nodesRef, selectFocusedNode, setDialogNodeId, setToolbarNodeId, size.height, size.width, transitionViewportTo, viewportRef]);

    // 缩放菜单的预设档位和输入的百分比：和其他程序化镜头一样过渡过去，任何输入立即接管。
    const setZoomScale = useCallback((scale: number) => {
        transitionViewportTo(viewportAtScale(viewportRef.current, size, scale));
        setContextMenu(null);
    }, [setContextMenu, size, transitionViewportTo, viewportRef]);

    const zoomCanvas = useCallback((direction: "in" | "out") => {
        stepViewportZoom(direction);
        setContextMenu(null);
    }, [setContextMenu, stepViewportZoom]);
    const zoomCanvasIn = useCallback(() => zoomCanvas("in"), [zoomCanvas]);
    const zoomCanvasOut = useCallback(() => zoomCanvas("out"), [zoomCanvas]);

    const zoomToActualSize = useCallback(() => {
        transitionViewportTo(viewportAtScale(viewportRef.current, size, 1));
    }, [size, transitionViewportTo, viewportRef]);

    const handleViewportChange = useCallback((next: ViewportTransform) => {
        // 滚轮收尾可能晚于新镜头指令，实际手势已在输入时接管视口。
        if (isViewportTransitionActive()) return;
        commitViewport(next);
        setContextMenu(null);
    }, [commitViewport, isViewportTransitionActive, setContextMenu]);

    const refreshVirtualization = useCallback((next: ViewportTransform) => {
        // The overview canvas redraws every frame from the live camera and
        // only leaves LOD after the gesture commits.
        if (virtualizationPausedRef?.current) return;
        const now = performance.now();
        if (!shouldRefreshCanvasVirtualization(renderedViewportRef.current, next, lastVirtualizationRefreshAtRef.current, now)) return;
        renderedViewportRef.current = next;
        lastVirtualizationRefreshAtRef.current = now;
        // Keep culling current without relaying every camera tick as a new
        // scale prop to all mounted nodes. Rebase with the graphics layer's
        // existing scale limits, and commit the final camera on gesture end.
        if (shouldRebaseCanvasRaster(next, viewport)) setViewport(next);
        else setVirtualizationViewport(next);
    }, [setViewport, setVirtualizationViewport, viewport, virtualizationPausedRef]);
    useEffect(() => {
        refreshVirtualizationRef.current = refreshVirtualization;
    }, [refreshVirtualization]);

    const handleViewportPreviewChange = useCallback((next: ViewportTransform) => {
        cancelViewportTransition();
        viewportRef.current = next;
        refreshVirtualization(next);
    }, [cancelViewportTransition, refreshVirtualization, viewportRef]);

    return {
        fitCanvasContent,
        fitCanvasSelection,
        focusCanvasImageNode,
        focusCanvasNode,
        getCanvasCenter,
        handleCanvasDoubleClick,
        handleViewportChange,
        handleViewportPreviewChange,
        previewViewport,
        screenToCanvas,
        setZoomScale,
        zoomCanvasIn,
        zoomCanvasOut,
        zoomToActualSize,
    };
}
