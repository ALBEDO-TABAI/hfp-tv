import { App, Button, Select } from "antd";
import { Maximize, Minus, Plus } from "lucide-react";
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { CanvasNode } from "@/components/canvas/canvas-node";
import { CanvasFrameNode } from "@/components/canvas/canvas-frame-node";
import { ConnectionPath } from "@/components/canvas/canvas-connections";
import { InfiniteCanvas } from "@/components/canvas/infinite-canvas";
import { CanvasNodeActionContext } from "@/components/canvas/canvas-node-action-context";
import { CanvasNodeGraphContext } from "@/components/canvas/canvas-node-graph-context";
import { EmptyState } from "@/components/ui/product/empty-state";
import { normalizeCanvasAppearance } from "@/lib/canvas/canvas-appearance";
import { isFrameNode, isNodeHiddenByCollapsedFrame, resolveFrameConnection } from "@/lib/canvas/canvas-frame";
import { getCanvasNodesBounds, viewportAtScale, viewportForBounds } from "@/lib/canvas/canvas-viewport";
import { getNodeDefinition } from "@/lib/canvas/node-registry";
import { workshopCanvasNodes, workshopNodeParameters } from "@/lib/workshop-media";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";
import type { Snapshot } from "@/services/api/workshop";
import type { CanvasNodeData, ViewportTransform } from "@/types/canvas";

const noAction = () => undefined;
const readOnlyActions = {};
const previewId = (id: string) => `workshop-preview:${id}`;
const FALLBACK_NODE_SIZE = { width: 320, height: 240 };

// Documents written by the assistant or CLI may omit node sizes; the editor never does.
function withNodeSize(node: CanvasNodeData): CanvasNodeData {
    if (Number.isFinite(node.width) && Number.isFinite(node.height)) return node;
    const size = getNodeDefinition(node.type)?.defaultSize || FALLBACK_NODE_SIZE;
    return { ...node, width: Number.isFinite(node.width) ? node.width : size.width, height: Number.isFinite(node.height) ? node.height : size.height };
}

export function WorkshopCanvasViewer({ canvas, versionId, focus, allowDownload = false }: {
    canvas: Snapshot["canvases"][number]; versionId: string; focus?: { nodeId: string }; allowDownload?: boolean;
}) {
    const { message } = App.useApp();
    const containerRef = useRef<HTMLDivElement>(null);
    const theme = useActiveTheme();
    const [viewport, setViewport] = useState<ViewportTransform>({ x: 0, y: 0, k: 1 });
    const [selectedId, setSelectedId] = useState<string>();
    const nodes = useMemo(() => workshopCanvasNodes(canvas, versionId).map(withNodeSize), [canvas, versionId]);
    const selected = nodes.find((node) => node.id === selectedId);
    // A file inside a collapsed frame still needs to be reachable in this local preview.
    const visibleNodes = useMemo(() => nodes.filter((node) => node.id === selectedId || !isNodeHiddenByCollapsedFrame(node, nodes)), [nodes, selectedId]);
    const connections = useMemo(() => canvas.document.connections.flatMap((connection) => {
        const resolved = resolveFrameConnection(connection, nodes);
        return resolved ? [{ connection, ...resolved }] : [];
    }), [canvas, nodes]);
    const graph = useMemo(() => ({
        getUpstreamNodes: (id: string) => canvas.document.connections.filter((connection) => previewId(connection.toNodeId) === id)
            .flatMap((connection) => nodes.filter((node) => node.id === connection.fromNodeId)),
    }), [canvas, nodes]);
    const fitNodes = useCallback((items: CanvasNodeData[]) => {
        const rect = containerRef.current?.getBoundingClientRect();
        const bounds = getCanvasNodesBounds(items);
        if (rect && rect.width && bounds) setViewport(viewportForBounds(bounds, rect, { padding: 64, maxScale: 1 }));
    }, []);
    const selectNode = useCallback((id: string) => {
        setSelectedId(id);
        fitNodes(nodes.filter((node) => node.id === id));
    }, [fitNodes, nodes]);
    useLayoutEffect(() => {
        fitNodes(nodes.filter((node) => !isNodeHiddenByCollapsedFrame(node, nodes)));
    }, [nodes, fitNodes]);
    useLayoutEffect(() => {
        if (focus) selectNode(focus.nodeId);
    }, [focus, selectNode]);
    const zoom = (scale: number) => {
        const rect = containerRef.current?.getBoundingClientRect();
        if (rect) setViewport((current) => viewportAtScale(current, rect, scale));
    };
    const renderContent = useCallback((node: CanvasNodeData) => <WorkshopNodeContent node={node} allowDownload={allowDownload} />, [allowDownload]);
    const background = canvas.document.backgroundMode;
    return <div className="grid min-h-[32rem] grid-cols-1 overflow-hidden rounded-[var(--r-md)] border border-border bg-card text-foreground lg:grid-cols-[minmax(0,1fr)_18rem]" data-workshop-viewer>
        <div className="relative h-[60vh] min-h-[24rem]" aria-label="只读画布">
            <CanvasNodeActionContext.Provider value={readOnlyActions}>
                <CanvasNodeGraphContext.Provider value={graph}>
                    <InfiniteCanvas containerRef={containerRef} viewport={viewport} onViewportChange={setViewport}
                        appearance={normalizeCanvasAppearance(canvas.document.appearance, theme)}
                        backgroundMode={background === "dots" || background === "lines" || background === "blank" ? background : "dots"}
                        onContextMenu={(event) => event.preventDefault()} onDrop={(event) => event.preventDefault()} onCanvasDeselect={() => setSelectedId(undefined)}>
                        <svg className="pointer-events-none absolute overflow-visible" width={1} height={1}>
                            {connections.map(({ connection, from, to }) => <ConnectionPath key={connection.id} connection={{ ...connection, id: previewId(connection.id) }} from={from} to={to} active={false} onSelect={noAction} />)}
                        </svg>
                        {visibleNodes.map((node) => isFrameNode(node) ? <CanvasFrameNode key={node.id} data={{ ...node, id: previewId(node.id) }}
                            childNodes={nodes.filter((child) => child.parentId === node.id)} scale={viewport.k} isSelected={selectedId === node.id} isDropTarget={false} readOnly
                            onMouseDown={() => setSelectedId(node.id)} onResize={noAction} onToggleCollapsed={noAction} onFolderStyleChange={noAction} onTitleChange={noAction} onContextMenu={(event) => event.preventDefault()} />
                            : <CanvasNode key={node.id} data={{ ...node, id: previewId(node.id) }} scale={viewport.k} readOnly
                                isSelected={selectedId === node.id} mediaActive={selectedId === node.id} isRelated={false} isFocusRelated={false} isConnectionTarget={false}
                                showImageInfo={false} reduceMediaEffects renderReadOnlyContent={renderContent}
                                onMouseDown={() => setSelectedId(node.id)} onHoverStart={noAction} onHoverEnd={noAction} onConnectStart={noAction} onResize={noAction} onContentChange={noAction} onContextMenu={(event) => event.preventDefault()} />)}
                    </InfiniteCanvas>
                </CanvasNodeGraphContext.Provider>
            </CanvasNodeActionContext.Provider>
            {!nodes.length && <div className="pointer-events-none absolute inset-0 grid place-items-center"><EmptyState description="这张画布没有节点" /></div>}
            <div data-canvas-no-zoom className="absolute bottom-4 left-4 flex items-center gap-1 rounded-[var(--r-md)] border border-border bg-sidebar p-1">
                <Button type="text" aria-label="缩小预览" icon={<Minus size={16} />} onClick={() => zoom(viewport.k / 1.2)} />
                <span className="w-12 text-center text-xs">{Math.round(viewport.k * 100)}%</span>
                <Button type="text" aria-label="放大预览" icon={<Plus size={16} />} onClick={() => zoom(viewport.k * 1.2)} />
                <Button type="text" aria-label="适应画布" icon={<Maximize size={16} />} onClick={() => fitNodes(visibleNodes)} />
            </div>
        </div>
        <aside className="max-h-[60vh] space-y-4 overflow-y-auto border-t border-border p-4 lg:border-l lg:border-t-0" aria-label="节点详情">
            <Select className="w-full" aria-label="选择节点" placeholder="选择节点查看详情" value={selectedId} showSearch optionFilterProp="label"
                options={nodes.map((node) => ({ value: node.id, label: node.title || getNodeDefinition(node.type)?.label || node.type }))} onChange={selectNode} />
            {selected ? <>
                <h3 className="break-words font-medium">{selected.title}</h3>
                <p className="text-sm text-muted-foreground">类型：{getNodeDefinition(selected.type)?.label || selected.type}</p>
                <div className="space-y-2">
                    <div className="flex items-center justify-between gap-2"><strong className="text-sm">提示词</strong>
                        <Button size="small" disabled={!selected.metadata?.prompt} onClick={async () => {
                            try { await navigator.clipboard.writeText(selected.metadata?.prompt || ""); message.success("提示词已复制"); }
                            catch { message.error("复制失败，请选择文字手动复制"); }
                        }}>复制提示词</Button></div>
                    <p className="whitespace-pre-wrap break-words text-sm">{selected.metadata?.prompt || "无提示词"}</p>
                </div>
                <dl className="space-y-2 text-sm">{workshopNodeParameters(selected.metadata).map(([key, value]) => <div key={key} className="break-words"><dt className="text-muted-foreground">{key}</dt><dd>{value}</dd></div>)}</dl>
                <WorkshopNodeContent node={selected} allowDownload={allowDownload} />
            </> : <p className="text-sm text-muted-foreground">点选画布上的节点，查看提示词、参数和产出。</p>}
        </aside>
    </div>;
}

export function WorkshopNodeContent({ node, allowDownload }: { node: CanvasNodeData; allowDownload: boolean }) {
    const content = node.metadata?.content;
    const media = content?.startsWith("/api/community/files/") ? content : undefined;
    const poster = node.metadata?.videoPreview?.content || node.metadata?.videoPreview?.storageKey || node.metadata?.previewContent;
    if (media && (node.type === "image" || node.type === "drawing")) return <img src={media} alt={node.title} className="h-full max-h-96 w-full object-contain" draggable={false} />;
    if (media && node.type === "video") return <video src={media} poster={poster?.startsWith("/api/community/files/") ? poster : undefined} controls controlsList={allowDownload ? undefined : "nodownload"} preload="metadata" className="max-h-96 w-full" data-canvas-no-zoom />;
    if (media && node.type === "audio") return <audio src={media} controls controlsList={allowDownload ? undefined : "nodownload"} preload="metadata" className="w-full" data-canvas-no-zoom />;
    const rows = node.metadata?.storyboard?.rows;
    return <div data-canvas-wheel-scroll className="h-full w-full overflow-auto whitespace-pre-wrap break-words bg-card p-4 text-sm text-card-foreground">
        {rows?.length ? rows.map((row) => <div key={row.id} className="mb-3 border-b border-border pb-3"><p>镜头 {row.shotNumber} · {row.durationSeconds}s</p><p>{row.plotDescription}</p><p>{row.dialogue}</p></div>)
            : content || node.metadata?.composerContent || node.metadata?.prompt || "此节点没有可预览内容"}
    </div>;
}
