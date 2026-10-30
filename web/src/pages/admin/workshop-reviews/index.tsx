import { useState } from "react";
import { Alert, App, Button, Popconfirm, Space, Table, Tabs } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { EmptyState } from "@/components/ui/product/empty-state";
import { platformErrorMessage } from "@/lib/platform-account";
import { listAdminWorkshopPublications, listWorkshopReviews, manageWorkshopPublication, workshopQueryKey, type Publication, type PublicationAction, type ReviewItem } from "@/services/api/workshop";
import { ReviewDetail } from "./review-detail";
import { ReasonDialog } from "./reason-dialog";

export default function WorkshopReviewsPage() {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [tab, setTab] = useState("pending");
    const [selected, setSelected] = useState<string | null>(null);
    const [unlisting, setUnlisting] = useState<Publication | null>(null);
    const status = tab === "processed" ? "processed" : "pending";
    const reviews = useQuery({ queryKey: [...workshopQueryKey, "reviews", status], queryFn: ({ signal }) => listWorkshopReviews(status, signal), enabled: tab !== "publications" });
    const publications = useQuery({ queryKey: [...workshopQueryKey, "admin-publications"], queryFn: ({ signal }) => listAdminWorkshopPublications(signal), enabled: tab === "publications" });
    const manage = useMutation({
        mutationFn: ({ publication, action, reason }: { publication: Publication; action: PublicationAction; reason?: string }) => manageWorkshopPublication(publication.id, action, reason),
        onSuccess: async () => { setUnlisting(null); message.success("已保存"); await client.invalidateQueries({ queryKey: workshopQueryKey }); },
        onError: (error) => message.error(platformErrorMessage(error)),
    });
    return <main className="app-workspace-page flex h-full min-h-0 flex-col overflow-y-auto p-6 text-foreground">
        <header className="mb-5 flex items-center justify-between gap-3"><h1 className="text-xl font-medium">作品审核</h1><Button onClick={() => void (tab === "publications" ? publications.refetch() : reviews.refetch())}>刷新</Button></header>
        <Tabs activeKey={tab} onChange={setTab} items={[{ key: "pending", label: "待审核" }, { key: "processed", label: "已处理" }, { key: "publications", label: "已发布作品" }]} />
        {tab === "publications" ? <>
            {publications.isError && <Alert className="mb-4" type="error" title={platformErrorMessage(publications.error)} />}
            <Table<Publication> rowKey="id" dataSource={publications.data?.items} loading={publications.isFetching} scroll={{ x: 1000 }} pagination={{ pageSize: 20 }} locale={{ emptyText: <EmptyState description="暂无已发布作品" /> }} columns={[
                { title: "作品", key: "title", render: (_, work) => <Link to={`/workshop/${work.id}`}>{work.title}</Link> },
                { title: "作者", key: "author", render: (_, work) => work.author.displayName || work.author.username },
                { title: "版本", dataIndex: "versionNo" },
                { title: "状态", key: "status", render: (_, work) => work.status === "listed" ? "已发布" : work.status === "unlisted" ? "已下架" : "草稿" },
                { title: "下架原因", dataIndex: "unlistReason" },
                { title: "操作", key: "actions", render: (_, work) => <Space wrap>
                    {work.status === "listed" ? <Button size="small" disabled={manage.isPending} onClick={() => { manage.reset(); setUnlisting(work); }}>下架</Button> : work.status === "unlisted" && <Popconfirm title="恢复发布这份作品？" onConfirm={() => manage.mutate({ publication: work, action: "relist" })}><Button size="small" disabled={manage.isPending}>恢复</Button></Popconfirm>}
                    <Button size="small" disabled={manage.isPending} onClick={() => manage.mutate({ publication: work, action: work.featured ? "unfeature" : "feature" })}>{work.featured ? "取消精选" : "设为精选"}</Button>
                    <Button size="small" disabled={manage.isPending} onClick={() => manage.mutate({ publication: work, action: work.official ? "unofficial" : "official" })}>{work.official ? "取消官方" : "设为官方"}</Button>
                </Space> },
            ]} />
        </> : <>
            {reviews.isError && <Alert className="mb-4" type="error" title={platformErrorMessage(reviews.error)} />}
            <Table<ReviewItem> rowKey={(item) => item.version.id} dataSource={reviews.data?.items} loading={reviews.isFetching} scroll={{ x: 900 }} pagination={{ pageSize: 20 }} locale={{ emptyText: <EmptyState description={tab === "pending" ? "暂无待审核作品" : "暂无已处理作品"} /> }} columns={[
                { title: "作品", key: "title", render: (_, item) => <Button type="link" onClick={() => setSelected(item.version.id)}>{item.version.title}</Button> },
                { title: "作者", key: "author", render: (_, item) => item.author.displayName || item.author.username },
                { title: "版本", key: "version", render: (_, item) => `版本 ${item.version.versionNo}` },
                { title: "投稿时间", key: "time", render: (_, item) => new Date(item.version.createdAt).toLocaleString("zh-CN") },
                { title: "状态", key: "status", render: (_, item) => ({ pending: "待审核", approved: "已通过", rejected: "已驳回", withdrawn: "已撤回" })[item.version.reviewStatus] },
                { title: "审核意见", key: "note", render: (_, item) => item.version.reviewNote },
            ]} />
        </>}
        {selected && <ReviewDetail key={selected} id={selected} onClose={() => setSelected(null)} />}
        {unlisting && <ReasonDialog title={`下架《${unlisting.title}》`} busy={manage.isPending} error={manage.isError ? platformErrorMessage(manage.error) : undefined} onClose={() => setUnlisting(null)} onConfirm={(reason) => manage.mutate({ publication: unlisting, action: "unlist", reason })} />}
    </main>;
}
