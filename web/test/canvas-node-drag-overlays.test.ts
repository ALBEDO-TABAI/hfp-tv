import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, test } from "bun:test";

const projectSource = readFileSync(resolve(import.meta.dir, "../src/pages/canvas/project.tsx"), "utf8");
const selectionControllerSource = readFileSync(resolve(import.meta.dir, "../src/pages/canvas/use-canvas-selection-controller.ts"), "utf8");
const flat = (text: string) => text.replace(/\s+/g, " ");

describe("canvas node drag overlays", () => {
    test("hides floating editors and selection controls for the whole drag preview", () => {
        expect(projectSource).toContain("const isCanvasNodeMoving = isNodeDragging || Boolean(dragPreview?.nodeIds.size);");
        // 面板浮层的守卫必须同时成立：类型排除、框选、拖拽预览。断言整段条件而不是某一行的
        // 字面量，避免上游拆行或新增类型排除（Panorama 等）后把排版变化报成契约失效。
        const flatSource = flat(projectSource);
        const overlayStart = flatSource.indexOf("{dialogNode &&");
        const overlayCondition = flatSource.slice(overlayStart, flatSource.indexOf("<CanvasNodePanelOverlay", overlayStart));
        expect(overlayCondition).toContain("dialogNode.type !== CanvasNodeType.Drawing");
        expect(overlayCondition).toContain("!selectionBox");
        expect(overlayCondition).toContain("!isCanvasNodeMoving");
        expect(projectSource).toContain("angleNode?.metadata?.content ? (");
        expect(projectSource).toContain("dragOffset={dragPreview?.nodeIds.has(angleNode.id)");
        expect(projectSource).toContain("emotionNode?.metadata?.content && !isCanvasNodeMoving");
        // 多选工具条在拖动期间保持挂载但立即隐藏、不可交互，松手后淡回，而不是卸载再重新弹出。
        expect(projectSource).toContain("inert={isCanvasNodeMoving}");
        expect(projectSource).toContain('${isCanvasNodeMoving ? "opacity-0" :');
        expect(projectSource).toContain("selectedNodeBounds && !selectionBox ? (");
        expect(projectSource).toContain(
            "node={isCanvasNodeMoving || nodeImageSettingsOpen || annotationNodeId || maskEditNodeId || emotionNodeId || angleNodeId || (dialogNode && !isCanvasMediaResultNode(dialogNode)) || textEditorNodeId ? null : toolbarNode}",
        );
        expect(projectSource).toContain("onNodeDragEnd: handleNodeDragEnd");
        expect(projectSource).toContain("setDialogNodeId(node.id);");
        expect(selectionControllerSource).toContain("if (clickedNodeId) onNodeDragEnd?.(clickedNodeId);");
    });
});
