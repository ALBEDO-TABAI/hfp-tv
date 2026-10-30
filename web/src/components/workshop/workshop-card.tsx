import { Button, Tag } from "antd";
import { Link } from "react-router";
import { EmptyState } from "@/components/ui/product/empty-state";
import { workshopFileUrl, type Publication, type Derived } from "@/services/api/workshop";

export function WorkshopCard({ publication, onCopy }: { publication: Publication; onCopy: (publication: Publication) => void }) {
    return <article className="group overflow-hidden rounded-[var(--r-md)] border border-border bg-card text-card-foreground">
        <div className="relative aspect-video overflow-hidden bg-muted">
            <Link to={`/workshop/${publication.id}`} className="block h-full focus-visible:outline-2 focus-visible:outline-ring" aria-label={`预览 ${publication.title}`}>
                {publication.coverSha256 ? <img src={workshopFileUrl(publication.coverSha256, publication.currentVersionId)} alt={publication.title} loading="lazy" className="h-full w-full object-contain" /> : <span className="grid h-full place-items-center text-sm text-muted-foreground">{publication.title}</span>}
            </Link>
            <div className="absolute bottom-2 right-2 flex gap-2 rounded-[var(--r-sm)] bg-card p-1 md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100 [@media(hover:none)]:opacity-100">
                <Link to={`/workshop/${publication.id}`} className="inline-flex items-center px-3 text-sm focus-visible:outline-2 focus-visible:outline-ring">预览</Link>
                <Button size="small" onClick={() => onCopy(publication)}>复制</Button>
            </div>
        </div>
        <div className="space-y-2 p-4">
            <div className="flex items-center gap-2"><Link to={`/workshop/${publication.id}`} className="min-w-0 truncate font-medium focus-visible:outline-2 focus-visible:outline-ring">{publication.title}</Link>{publication.official && <Tag>官方</Tag>}</div>
            <p className="truncate text-sm text-muted-foreground">{publication.author.displayName || publication.author.username}</p>
            <p className="flex justify-between gap-2 text-xs text-muted-foreground"><span>{publication.stats.files} 个文件</span><span>复制 {publication.copyCount} 次</span></p>
        </div>
    </article>;
}

export function WorkshopEmptyState() {
    return <EmptyState description="还没有分享的画布。你可以在项目页把作品投稿到创意工坊。" action={<Link to="/canvas" className="text-primary underline focus-visible:outline-2 focus-visible:outline-ring">去投稿</Link>} />;
}

export function WorkshopOrigin({ origin }: { origin: Derived | null }) {
    if (!origin) return null;
    return <p className="text-sm text-muted-foreground">{origin.available ? <Link to={`/workshop/${origin.publicationId}`} className="underline focus-visible:outline-2 focus-visible:outline-ring">改编自《{origin.title}》· {origin.authorName}</Link> : "改编自（已下架作品）"}</p>;
}
