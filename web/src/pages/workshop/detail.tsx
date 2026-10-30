import { useState } from "react";
import { Alert, Button, Collapse, Skeleton, Tag } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { EmptyState } from "@/components/ui/product/empty-state";
import { WorkshopOrigin } from "@/components/workshop/workshop-card";
import { WorkshopCopyDialog } from "@/components/workshop/workshop-copy-dialog";
import { WorkshopSnapshotView } from "@/components/workshop/workshop-snapshot-view";
import { platformErrorMessage } from "@/lib/platform-account";
import { ApiError } from "@/services/api/request";
import { getWorkshopPublication, workshopQueryKey } from "@/services/api/workshop";

export default function WorkshopDetailPage() {
    const { id } = useParams();
    const [copyOpen, setCopyOpen] = useState(false);
    const [filesOpen, setFilesOpen] = useState(false);
    const publication = useQuery({
        queryKey: [...workshopQueryKey, "publication", id],
        queryFn: ({ signal }) => { if (!id) throw new Error("缺少作品编号"); return getWorkshopPublication(id, signal); },
        retry: false,
    });
    const work = publication.data;
    return <main className="app-workspace-page h-full space-y-4 overflow-y-auto p-4 text-foreground sm:p-6">
        <Link to="/workshop" className="inline-block text-sm focus-visible:outline-2 focus-visible:outline-ring">← 返回创意工坊</Link>
        {publication.isError ? publication.error instanceof ApiError && (publication.error.status === 410 || publication.error.reason === "publication_unlisted") ? <EmptyState title="该作品已下架" />
            : <Alert type="error" title={publication.error instanceof ApiError && publication.error.status === 404 ? "作品不存在" : platformErrorMessage(publication.error)} action={<Button onClick={() => void publication.refetch()}>重试</Button>} />
            : !work ? <Skeleton active /> : <>
                <header className="flex flex-wrap items-center justify-between gap-4">
                    <div className="min-w-0 space-y-2"><h1 className="break-words text-xl font-medium">{work.title} {work.official && <Tag>官方</Tag>}</h1><p className="text-sm text-muted-foreground">v{work.versionNo} · {work.author.displayName || work.author.username} · {new Date(work.updatedAt).toLocaleString("zh-CN")} 更新</p></div>
                    <div className="flex flex-wrap gap-2"><Button onClick={() => setFilesOpen(true)}>文件清单</Button><Button type="primary" onClick={() => setCopyOpen(true)}>复制到我的工作区</Button></div>
                </header>
                <Collapse defaultActiveKey={["intro"]} items={[{ key: "intro", label: "只读预览 · 复制后可以编辑和重新生成", children: <div className="space-y-3"><p className="whitespace-pre-wrap break-words">{work.summary || "暂无简介"}</p><div>{work.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</div><WorkshopOrigin origin={work.derivedFrom} /></div> }]} />
                <WorkshopSnapshotView key={work.currentVersionId} versionId={work.currentVersionId} allowDownload={work.allowDownload || work.canManage} filesOpen={filesOpen} onCloseFiles={() => setFilesOpen(false)} />
                {copyOpen && <WorkshopCopyDialog publication={work} onClose={() => setCopyOpen(false)} />}
            </>}
    </main>;
}
