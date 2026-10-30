import { useEffect, useState } from "react";
import { Alert, Button, Skeleton, Tabs } from "antd";
import { useQuery } from "@tanstack/react-query";
import { EmptyState } from "@/components/ui/product/empty-state";
import { WorkshopCanvasViewer } from "./workshop-canvas-viewer";
import { WorkshopFileList } from "./workshop-file-list";
import { getWorkshopSnapshot, workshopQueryKey } from "@/services/api/workshop";
import { platformErrorMessage } from "@/lib/platform-account";

export function WorkshopSnapshotView({ versionId, snapshotSha256, allowDownload, filesOpen, onCloseFiles, onReady }: {
    versionId: string; snapshotSha256?: string; allowDownload: boolean; filesOpen: boolean; onCloseFiles: () => void; onReady?: (ready: boolean) => void;
}) {
    const [canvasKey, setCanvasKey] = useState<string>();
    const [focus, setFocus] = useState<{ canvasKey: string; nodeId: string }>();
    const snapshot = useQuery({ queryKey: [...workshopQueryKey, "snapshot", versionId, snapshotSha256], queryFn: ({ signal }) => getWorkshopSnapshot(versionId, signal), refetchOnWindowFocus: false });
    useEffect(() => {
        onReady?.(snapshot.isSuccess && !snapshot.isFetching);
        return () => onReady?.(false);
    }, [onReady, snapshot.isSuccess, snapshot.isFetching]);
    if (snapshot.isError) return <Alert type="error" title={platformErrorMessage(snapshot.error)} action={<Button onClick={() => void snapshot.refetch()}>重试</Button>} />;
    if (!snapshot.data) return <Skeleton active />;
    const canvas = snapshot.data.canvases.find((item) => item.key === canvasKey) || snapshot.data.canvases[0];
    return <>
        {snapshot.data.canvases.length > 1 && <Tabs activeKey={canvas?.key} onChange={(key) => { setCanvasKey(key); setFocus(undefined); }} items={snapshot.data.canvases.map((item) => ({ key: item.key, label: item.title }))} />}
        {canvas ? <WorkshopCanvasViewer key={`${versionId}:${canvas.key}`} canvas={canvas} versionId={versionId} allowDownload={allowDownload} focus={focus?.canvasKey === canvas.key ? focus : undefined} /> : <EmptyState description="这份作品没有画布" />}
        <WorkshopFileList open={filesOpen} onClose={onCloseFiles} snapshot={snapshot.data} versionId={versionId} allowDownload={allowDownload} onLocate={(location) => { setCanvasKey(location.canvasKey); setFocus(location); }} />
    </>;
}
