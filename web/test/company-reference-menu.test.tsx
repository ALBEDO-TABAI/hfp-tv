import assert from "node:assert/strict";
import test from "node:test";
import { createElement, type ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { CanvasNodeContextMenu } from "../src/components/canvas/canvas-context-menu";
import { usePlatformSession } from "../src/stores/use-platform-session";
import { CanvasNodeType } from "../src/types/canvas";

test("only hosted image nodes offer virtual portrait registration", () => {
    const noop = () => {};
    const props: ComponentProps<typeof CanvasNodeContextMenu> = {
        menu: { type: "node", x: 10, y: 10, nodeId: "image" },
        node: { id: "image", type: CanvasNodeType.Image, title: "头像", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { content: "image.png" } },
        canUndo: false, canRedo: false, canPaste: false,
        onClose: noop, onAddNode: noop, onAddFolder: noop, onChooseStyle: noop, onOpenDirector: noop,
        onUpload: noop, onOpenAssets: noop, onOpenProjectCharacters: noop, onOpenGenerationHistory: noop,
        onUndo: noop, onRedo: noop, onPaste: noop, onCopyNode: noop, onCreateGenerationCopy: noop,
        onDuplicate: noop, onDelete: noop, onSaveAsset: noop, onViewMedia: noop, onEditText: noop,
        onOpenDrawing: noop, onGenerateImage: noop, onCopyContent: noop, onCopyMediaUrl: noop,
        onUploadToArkPrivateAsset: noop, onSetAssetCategory: noop, onToggleFrame: noop,
    };
    try {
        usePlatformSession.setState({ mode: "hosted" });
        assert.match(renderToStaticMarkup(createElement(CanvasNodeContextMenu, props)), /登记虚拟人像素材/);
        usePlatformSession.setState({ mode: "local" });
        assert.doesNotMatch(renderToStaticMarkup(createElement(CanvasNodeContextMenu, props)), /登记虚拟人像素材/);
    } finally {
        usePlatformSession.setState({ mode: "unknown" });
    }
});
