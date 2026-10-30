import { useState } from "react";
import { Alert, App, Button, Popconfirm, Skeleton, Space, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AppDrawer } from "@/components/ui/product/app-drawer";
import { WorkshopOrigin } from "@/components/workshop/workshop-card";
import { WorkshopSnapshotView } from "@/components/workshop/workshop-snapshot-view";
import { platformErrorMessage } from "@/lib/platform-account";
import { ApiError } from "@/services/api/request";
import { approveWorkshopVersion, getWorkshopReview, rejectWorkshopVersion, workshopQueryKey, type Version } from "@/services/api/workshop";
import { ReasonDialog } from "./reason-dialog";

export function ReviewDetail({ id, onClose }: { id: string; onClose: () => void }) {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [filesOpen, setFilesOpen] = useState(false);
    const [ready, setReady] = useState(false);
    const [rejecting, setRejecting] = useState<Version | null>(null);
    const [conflict, setConflict] = useState(false);
    const detail = useQuery({ queryKey: [...workshopQueryKey, "review", id], queryFn: ({ signal }) => getWorkshopReview(id, signal), refetchOnWindowFocus: false });
    const review = useMutation({
        mutationFn: (input: { version: Version } & ({ kind: "approve"; official: boolean } | { kind: "reject"; reason: string })) => input.kind === "approve"
            ? approveWorkshopVersion(input.version.id, { snapshotSha256: input.version.snapshotSha256, official: input.official })
            : rejectWorkshopVersion(input.version.id, input.reason),
        onSuccess: async () => {
            setRejecting(null); setConflict(false); message.success("已完成审核");
            await client.invalidateQueries({ queryKey: workshopQueryKey });
        },
        onError: async (error) => {
            message.error(platformErrorMessage(error));
            if (error instanceof ApiError && error.status === 409 && error.reason === "version_changed") {
                setRejecting(null); setConflict(true);
                await client.invalidateQueries({ queryKey: workshopQueryKey });
            }
        },
    });
    const item = detail.data;
    const busy = review.isPending || detail.isFetching || !ready || detail.isError || conflict;
    return <AppDrawer open title="作品审核" width="min(1400px, 96vw)" onClose={() => { if (!review.isPending) onClose(); }}>
        {conflict && <Alert className="mb-4" type="warning" title="作者已更新或撤回稿件，请刷新后重新审核" action={<Button onClick={async () => { const result = await detail.refetch(); if (result.isSuccess) setConflict(false); }}>刷新</Button>} />}
        {detail.isError ? <Alert type="error" title={platformErrorMessage(detail.error)} action={<Button onClick={() => void detail.refetch()}>重试</Button>} /> : !item ? <Skeleton active /> : <div className="space-y-4">
            <header className="flex flex-wrap items-center justify-between gap-3"><h2 className="break-words text-lg font-medium">{item.version.title} · v{item.version.versionNo}</h2><Button onClick={() => setFilesOpen(true)}>文件清单</Button></header>
            <p className="text-sm">作者：{item.author.displayName || item.author.username} · 投稿于 {new Date(item.version.createdAt).toLocaleString("zh-CN")}</p>
            <p className="whitespace-pre-wrap break-words">{item.version.summary || "暂无简介"}</p>
            <div>{item.version.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}<Tag>{item.version.allowDownload ? "允许下载原文件" : "不允许下载原文件"}</Tag></div>
            <WorkshopOrigin origin={item.version.derivedFrom} />
            {item.previous && <p className="text-sm">相比 v{item.previous.versionNo}：{([ ["画布", item.version.stats.canvases - item.previous.stats.canvases], ["文件", item.version.stats.files - item.previous.stats.files], ["节点", item.version.stats.nodes - item.previous.stats.nodes] ] as const).map(([label, delta]) => `${label} ${delta >= 0 ? "+" : ""}${delta}`).join("，")}</p>}
            {item.version.reviewNote && <Alert type="info" title="审核意见" description={item.version.reviewNote} />}
            <WorkshopSnapshotView key={`${item.version.id}:${item.version.snapshotSha256}`} versionId={item.version.id} snapshotSha256={item.version.snapshotSha256} allowDownload filesOpen={filesOpen} onCloseFiles={() => setFilesOpen(false)} onReady={setReady} />
            {item.version.reviewStatus === "pending" && <Space wrap>
                <Popconfirm title="通过并发布当前版本？" onConfirm={() => review.mutate({ kind: "approve", version: item.version, official: false })}><Button type="primary" disabled={busy}>通过</Button></Popconfirm>
                <Popconfirm title="通过并设为官方作品？" onConfirm={() => review.mutate({ kind: "approve", version: item.version, official: true })}><Button disabled={busy}>通过并设为官方</Button></Popconfirm>
                <Button danger disabled={busy} onClick={() => { review.reset(); setRejecting(item.version); }}>驳回</Button>
            </Space>}
        </div>}
        {rejecting && <ReasonDialog title="驳回稿件" presets busy={review.isPending} error={review.isError ? platformErrorMessage(review.error) : undefined} onClose={() => setRejecting(null)} onConfirm={(reason) => review.mutate({ kind: "reject", version: rejecting, reason })} />}
    </AppDrawer>;
}
