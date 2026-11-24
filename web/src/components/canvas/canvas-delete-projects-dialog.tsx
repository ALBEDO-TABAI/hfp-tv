import { App } from "antd";
import { useMemo } from "react";
import { CanvasDeleteConfirmation } from "./canvas-delete-confirmation";

import { useAssetStore } from "@/stores/use-asset-store";
import { useCanvasUiStore } from "@/stores/canvas/use-canvas-ui-store";
import { deleteWorkspaceCanvasProjects } from "@/services/workspace-project-repository";
import { isLocalWorkspaceMode } from "@/services/workspace-mode";
import { useClosingValue } from "@/hooks/use-closing-value";

export function CanvasDeleteProjectsDialog() {
    const { message } = App.useApp();
    const ids = useCanvasUiStore((state) => state.deleteProjectIds);
    const scope = useCanvasUiStore((state) => state.deleteProjectScope);
    const setDeleteIds = useCanvasUiStore((state) => state.setDeleteProjectIds);
    const removeSelectedIds = useCanvasUiStore((state) => state.removeSelectedProjectIds);
    const cleanupImages = useAssetStore((state) => state.cleanupImages);
    const deletionLabel = isLocalWorkspaceMode() ? "本地画布" : "画布";
    const confirm = async (assertCurrent: () => void) => {
        const selected = [...useCanvasUiStore.getState().deleteProjectIds];
        if (!selected.length) return;
        await deleteWorkspaceCanvasProjects(selected);
        assertCurrent();
        void cleanupImages();
        removeSelectedIds(selected);
        message.success(`已删除 ${selected.length} 个${deletionLabel}`);
    };
    const request = useMemo(() => (scope && ids.length ? { scope, count: ids.length } : null), [scope, ids.length]);
    const dialog = useClosingValue(request);
    if (!dialog.shown) return null;
    return <CanvasDeleteConfirmation key={dialog.key} open={dialog.open} afterClose={dialog.afterClose} scope={dialog.shown.scope} title={`删除${deletionLabel}？`}
        description={`将删除 ${dialog.shown.count} 个${deletionLabel}，里面的节点和连线也会一起移除。`}
        onCancel={() => setDeleteIds([])} onConfirm={confirm} />;
}
