import { useState } from "react";
import { Alert, Button, Input, Segmented, Select, Skeleton, Tabs } from "antd";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router";
import { WorkshopCard, WorkshopEmptyState } from "@/components/workshop/workshop-card";
import { WorkshopCopyDialog } from "@/components/workshop/workshop-copy-dialog";
import { platformErrorMessage } from "@/lib/platform-account";
import { listWorkshopPublications, workshopQueryKey, type BrowseFilters, type Publication } from "@/services/api/workshop";
import { MySubmissions } from "./my-submissions";

export default function WorkshopPage() {
    const [params, setParams] = useSearchParams();
    const tab = params.get("tab") === "mine" ? "mine" : "browse";
    return <main className="app-workspace-page h-full overflow-y-auto p-6 text-foreground">
        <h1 className="mb-4 text-xl font-medium">创意工坊</h1>
        <Tabs activeKey={tab} onChange={(key) => setParams(key === "mine" ? { tab: "mine" } : {})} items={[{ key: "browse", label: "浏览" }, { key: "mine", label: "我的投稿" }]} />
        {tab === "mine" ? <MySubmissions /> : <BrowseWorkshop />}
    </main>;
}

function BrowseWorkshop() {
    const [q, setQuery] = useState("");
    const [tag, setTag] = useState("");
    const [source, setSource] = useState<BrowseFilters["source"]>("all");
    const [sort, setSort] = useState<BrowseFilters["sort"]>("featured");
    const [copying, setCopying] = useState<Publication | null>(null);
    const publications = useInfiniteQuery({
        queryKey: [...workshopQueryKey, "browse", { q, source, sort, tag }], initialPageParam: 1,
        queryFn: ({ signal, pageParam }) => listWorkshopPublications({ q, source, sort, tag, page: pageParam, pageSize: 20 }, signal),
        getNextPageParam: (last, pages) => pages.reduce((count, page) => count + page.items.length, 0) < last.total ? pages.length + 1 : undefined,
    });
    const items = publications.data?.pages.flatMap((page) => page.items) || [];
    return <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-3">
            <Input.Search aria-label="搜索作品" placeholder="搜索作品" allowClear onSearch={setQuery} className="max-w-sm" />
            <Segmented value={source} onChange={setSource} options={[{ label: "全部", value: "all" }, { label: "官方", value: "official" }, { label: "同事", value: "colleague" }]} />
            <Select aria-label="排序" value={sort} onChange={setSort} className="min-w-28" options={[{ label: "精选", value: "featured" }, { label: "最新", value: "latest" }, { label: "最多复制", value: "copies" }]} />
            <Input.Search aria-label="按标签筛选" placeholder="标签" allowClear onSearch={setTag} className="max-w-48" />
        </div>
        {publications.isError && <Alert type="error" title={platformErrorMessage(publications.error)} action={<Button onClick={() => void (publications.isFetchNextPageError ? publications.fetchNextPage() : publications.refetch())}>重试</Button>} />}
        {publications.isPending ? <Skeleton active /> : items.length ? <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">{items.map((publication) => <WorkshopCard key={publication.id} publication={publication} onCopy={setCopying} />)}</div> : !publications.isError && <WorkshopEmptyState />}
        {publications.hasNextPage && <div className="text-center"><Button loading={publications.isFetchingNextPage} onClick={() => void publications.fetchNextPage()}>加载更多</Button></div>}
        {copying && <WorkshopCopyDialog publication={copying} onClose={() => setCopying(null)} />}
    </div>;
}
