import { useState } from "react";
import { Alert, Button, Segmented, Skeleton } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { WorkshopCard, WorkshopEmptyState } from "@/components/workshop/workshop-card";
import { WorkshopCopyDialog } from "@/components/workshop/workshop-copy-dialog";
import { platformErrorMessage } from "@/lib/platform-account";
import { listWorkshopPublications, workshopQueryKey, type Publication } from "@/services/api/workshop";

export function HomeWorkshop() {
    const [selection, setSelection] = useState("featured");
    const [copying, setCopying] = useState<Publication | null>(null);
    const works = useQuery({ queryKey: [...workshopQueryKey, "home", selection], queryFn: ({ signal }) => listWorkshopPublications({ source: selection === "official" ? "official" : "all", sort: selection === "latest" ? "latest" : "featured", page: 1, pageSize: 4 }, signal) });
    return <section className="hfp-shelf-section space-y-4" aria-label="创意工坊">
        <header className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-medium">创意工坊 · 官方与同事分享的画布</h2><div className="flex flex-wrap items-center gap-4">
            <Segmented value={selection} onChange={setSelection} options={[{ label: "精选", value: "featured" }, { label: "官方", value: "official" }, { label: "最新", value: "latest" }]} />
            <Link to="/workshop" className="hfp-section-link">查看全部 →</Link>
        </div></header>
        {works.isError ? <Alert type="error" title={platformErrorMessage(works.error)} action={<Button onClick={() => void works.refetch()}>重试</Button>} /> : works.isPending ? <Skeleton active /> : works.data.items.length ? <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">{works.data.items.slice(0, 4).map((publication) => <WorkshopCard key={publication.id} publication={publication} onCopy={setCopying} />)}</div> : <WorkshopEmptyState />}
        {copying && <WorkshopCopyDialog publication={copying} onClose={() => setCopying(null)} />}
    </section>;
}
