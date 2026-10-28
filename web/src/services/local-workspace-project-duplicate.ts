import { listCanvasWorkspaceProjectCanvases } from "@/lib/canvas/canvas-workspace-project";
import { assertUserScope, captureUserScope } from "@/lib/user-scope-guard";
import { getCanvasDrawing, putCanvasDrawing } from "@/services/api/workspace-data";
import { beginLocalCanvasProject, deleteLocalCanvasProjects, syncLocalCanvasProjectToBackend } from "@/services/local-workspace-repository";
import { useCanvasStore } from "@/stores/canvas/use-canvas-store";

export async function duplicateLocalWorkspaceProject(sourceCanvasId: string): Promise<{ id: string; canvasCount: number }> {
    const scope = captureUserScope();
    const sourceCanvases = listCanvasWorkspaceProjectCanvases(useCanvasStore.getState().projects, sourceCanvasId);
    const copiedCanvasIds: string[] = [];
    let copiedWorkspaceProjectId: string | undefined;
    try {
        for (const [index, source] of sourceCanvases.entries()) {
            assertUserScope(scope);
            const content = structuredClone({
                folderId: source.folderId,
                canvasTitle: source.canvasTitle,
                nodes: source.nodes,
                connections: source.connections,
                chatSessions: source.chatSessions,
                activeChatId: source.activeChatId,
                starterMode: source.starterMode,
                appearance: source.appearance,
                backgroundMode: source.backgroundMode,
                showImageInfo: source.showImageInfo,
                viewport: source.viewport,
                directorScenes: source.directorScenes,
                timeline: source.timeline,
            });
            // Retain the new ID before awaiting persistence so failed saves can also be rolled back.
            const copy = beginLocalCanvasProject(index === 0 ? `${source.title || "未命名项目"} 副本` : source.title, source.projectId, content, copiedWorkspaceProjectId);
            copiedCanvasIds.push(copy.id);
            copiedWorkspaceProjectId ||= copy.id;
            await copy.saved;
            assertUserScope(scope);
            const drawingIds = new Set(content.nodes.filter((node) => node.type === "drawing" && node.metadata?.drawingId).map((node) => node.metadata!.drawingId!));
            for (const drawingId of drawingIds) {
                const { drawing } = await getCanvasDrawing(source.id, drawingId, { expectedScope: scope });
                if (drawing.snapshot === undefined) throw new Error("画板内容缺失，无法创建副本");
                // Drawing IDs are scoped by canvas; reuse media references within this workspace.
                const { drawing: saved } = await putCanvasDrawing(copy.id, drawingId, {
                    drawingId,
                    engine: drawing.engine,
                    revision: 0,
                    snapshot: drawing.snapshot,
                    shapeCount: drawing.shapeCount,
                    pageCount: drawing.pageCount,
                    previewResourceId: drawing.previewResourceId,
                    render: drawing.render,
                }, { expectedScope: scope });
                content.nodes = content.nodes.map((node) => node.type === "drawing" && node.metadata?.drawingId === drawingId ? {
                    ...node,
                    metadata: { ...node.metadata, drawingEngine: saved.engine, drawingRevision: saved.revision, drawingUpdatedAt: saved.updatedAt, drawingShapeCount: saved.shapeCount, drawingPageCount: saved.pageCount },
                } : node);
            }
            assertUserScope(scope);
            if (drawingIds.size) {
                useCanvasStore.getState().updateProject(copy.id, { nodes: content.nodes });
                await syncLocalCanvasProjectToBackend(copy.id, scope);
                assertUserScope(scope);
            }
        }
    } catch (error) {
        if (copiedCanvasIds.length) {
            try {
                assertUserScope(scope);
                await deleteLocalCanvasProjects(copiedCanvasIds, { recordHistory: false });
            } catch (cleanupError) {
                throw new AggregateError([error, cleanupError], `${error instanceof Error ? error.message : "创建副本失败"}；部分副本未能清理，请重试删除`);
            }
        }
        throw error;
    }
    if (!copiedWorkspaceProjectId) throw new Error("项目不存在");
    return { id: copiedWorkspaceProjectId, canvasCount: sourceCanvases.length };
}
