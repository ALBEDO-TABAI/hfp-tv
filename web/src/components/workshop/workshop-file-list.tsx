import { Button } from "antd";
import { AppDrawer } from "@/components/ui/product/app-drawer";
import { EmptyState } from "@/components/ui/product/empty-state";
import { findWorkshopFileNode, groupWorkshopFiles, workshopFileUrl } from "@/lib/workshop-media";
import type { Snapshot } from "@/services/api/workshop";

export function WorkshopFileList({ open, onClose, snapshot, versionId, allowDownload, onLocate }: {
    open: boolean; onClose: () => void; snapshot: Snapshot; versionId: string; allowDownload: boolean;
    onLocate: (location: { canvasKey: string; nodeId: string }) => void;
}) {
    return <AppDrawer open={open} title={`文件清单 · ${snapshot.files.length} 个文件`} width={440} onClose={onClose}>
        {!snapshot.files.length && <EmptyState description="这份作品没有文件" />}
        {groupWorkshopFiles(snapshot.files).map(([group, files]) => <section key={group} className="mb-6 space-y-3">
            <h3 className="font-medium">{group} · {files.length}</h3>
            <ul className="space-y-3">{files.map((file) => {
                const location = findWorkshopFileNode(snapshot, file.sha256);
                return <li key={file.sha256} className="flex items-center gap-3 rounded-[var(--r-sm)] border border-border p-3">
                    <button type="button" className="flex min-w-0 flex-1 items-center gap-3 text-left focus-visible:outline-2 focus-visible:outline-ring disabled:cursor-default" disabled={!location} title={location ? "定位到节点" : "此文件未直接关联节点"} onClick={() => {
                        if (location) { onLocate(location); onClose(); }
                    }}>
                        {file.mimeType.startsWith("image/") && <img src={workshopFileUrl(file.sha256, versionId)} alt="" loading="lazy" className="h-14 w-20 shrink-0 object-contain" />}
                        <span className="min-w-0"><span className="block break-words text-sm">{file.name}</span><small className="text-muted-foreground">{(file.size / 1024 / 1024).toFixed(1)} MB · {location ? "定位" : "附加文件"}</small></span>
                    </button>
                    {allowDownload && <Button size="small" href={workshopFileUrl(file.sha256, versionId, true)} download={file.name}>下载</Button>}
                </li>;
            })}</ul>
        </section>)}
    </AppDrawer>;
}
