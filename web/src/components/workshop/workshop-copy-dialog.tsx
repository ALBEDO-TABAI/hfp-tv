import { useRef, useState } from "react";
import { Alert, Button, Input, Space } from "antd";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AppModal } from "@/components/ui/product/app-modal";
import { appHref } from "@/lib/app-routing";
import { platformErrorMessage } from "@/lib/platform-account";
import { ApiError } from "@/services/api/request";
import { copyWorkshopPublication, getWorkshopCopy, workshopQueryKey, type Publication } from "@/services/api/workshop";

// Mount once per dialog opening so retries keep the same request identity.
export function WorkshopCopyDialog({ publication, onClose }: { publication: Publication; onClose: () => void }) {
    const [requestId] = useState(() => crypto.randomUUID().replaceAll("-", ""));
    const [title, setTitle] = useState(`${publication.title}（副本）`);
    const attempted = useRef(false);
    const client = useQueryClient();
    const copy = useMutation({
        retry: false,
        mutationFn: async () => {
            if (attempted.current) {
                try { return await getWorkshopCopy(requestId); }
                catch (error) { if (!(error instanceof ApiError && error.status === 404)) throw error; }
            }
            attempted.current = true;
            try { return await copyWorkshopPublication(publication.id, { requestId, title: title.trim() }); }
            catch (error) {
                if (error instanceof ApiError && error.reason === "copy_in_progress") {
                    try { return await getWorkshopCopy(requestId); }
                    catch (lookupError) { if (!(lookupError instanceof ApiError && lookupError.status === 404)) throw lookupError; }
                }
                throw error;
            }
        },
        onSuccess: () => { void client.invalidateQueries({ queryKey: workshopQueryKey }); },
    });
    return <AppModal flush open footer={null} onCancel={() => { if (!copy.isPending) onClose(); }} closable={!copy.isPending} maskClosable={!copy.isPending} keyboard={!copy.isPending}>
        <div className="space-y-5 p-6">
            <h2 className="text-lg font-medium">复制到我的工作区</h2>
            {copy.data ? <>
                <p role="status">已复制到你的项目</p>
                <Space><Button type="primary" onClick={() => window.location.assign(appHref(`/canvas/${copy.data.copy.targetProjectId}`))}>打开</Button><Button onClick={onClose}>留在此页</Button></Space>
            </> : <>
                <label className="block space-y-2"><span>名称</span><Input value={title} onChange={(event) => setTitle(event.target.value)} disabled={copy.isPending} /></label>
                <p className="text-sm text-muted-foreground">将复制 {publication.stats.canvases} 张画布、{publication.stats.files} 个文件（约 {(publication.stats.bytes / 1024 / 1024).toFixed(1)} MB）。复制后和原作品互不影响。</p>
                {copy.isPending && <p role="status">正在复制，较大的作品需要一点时间</p>}
                {copy.isError && <Alert type="error" title={platformErrorMessage(copy.error)} description="重试会继续确认本次复制结果。" />}
                <Space><Button type="primary" loading={copy.isPending} disabled={!title.trim()} onClick={() => copy.mutate()}>{copy.isError ? "重试" : "复制"}</Button><Button disabled={copy.isPending} onClick={onClose}>取消</Button></Space>
            </>}
        </div>
    </AppModal>;
}
