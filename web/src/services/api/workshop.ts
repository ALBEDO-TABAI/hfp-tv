import type { CanvasConnection, CanvasNodeData } from "@/types/canvas";
import { compactApiParams, http } from "./request";

export { resolveWorkshopMedia, workshopFileUrl } from "@/lib/workshop-media";

export type Author = { id: string; username: string; displayName: string };
export type Derived = { publicationId: string; versionId: string; title: string; authorName: string; available: boolean };
export type WorkshopStats = { canvases: number; nodes: number; files: number; bytes: number };
export type Publication = {
    id: string; title: string; summary: string; tags: string[]; author: Author; official: boolean; featured: boolean;
    status: "draft" | "listed" | "unlisted"; coverSha256: string; currentVersionId: string; versionNo: number;
    stats: WorkshopStats; copyCount: number; allowDownload: boolean; unlistReason: string; derivedFrom: Derived | null;
    publishedAt: string | null; updatedAt: string; canManage: boolean;
};
export type Version = {
    id: string; publicationId: string; versionNo: number; reviewStatus: "pending" | "approved" | "rejected" | "withdrawn";
    reviewNote: string; title: string; summary: string; tags: string[]; coverSha256: string; allowDownload: boolean;
    snapshotSha256: string; stats: WorkshopStats; derivedFrom: Derived | null; createdAt: string; reviewedAt: string | null;
};
export type Issue = { level: "error" | "warning"; code: string; message: string; canvasId?: string; nodeId?: string };
export type Preflight = {
    projectId: string; title: string; stats: WorkshopStats & { drawings: number }; issues: Issue[];
    covers: { resourceId: string; canvasId: string; nodeId: string; title: string }[];
};
export type CopyRecord = { id: string; requestId: string; status: "completed"; error: string; publicationId: string; versionId: string; targetProjectId: string; createdAt: string };
export type ReviewItem = { version: Version; publication: Publication; author: Author; previous: Version | null };
export type Snapshot = {
    format: "hfp-workshop-snapshot"; version: 1; title: string; stats: WorkshopStats & { drawings: number };
    canvases: {
        key: string; title: string;
        document: { title?: string; canvasTitle?: string; nodes: CanvasNodeData[]; connections: CanvasConnection[]; appearance?: unknown; backgroundMode?: unknown; showImageInfo?: boolean; directorScenes?: unknown; timeline?: unknown };
        drawings: { drawingId: string; engine: string; snapshot: unknown; shapeCount: number; pageCount: number; preview?: string; render?: { sha256: string; pageId?: string; width?: number; height?: number; mimeType?: string; background?: string } }[];
    }[];
    files: { sha256: string; size: number; mimeType: string; kind: string; name: string; width?: number; height?: number; durationMs?: number }[];
};
export type SubmissionInput = { projectId: string; title: string; summary: string; tags: string[]; coverResourceId?: string; allowDownload: boolean; rightsConfirmed: true };
export type BrowseFilters = { q?: string; source?: "all" | "official" | "colleague"; sort?: "featured" | "latest" | "copies"; tag?: string; page?: number; pageSize?: number };
export type PublicationAction = "unlist" | "relist" | "feature" | "unfeature" | "official" | "unofficial";
export const workshopQueryKey = ["workshop"];
const community = "/community";
const admin = "/platform/admin/workshop";
const id = encodeURIComponent;

export const preflightWorkshop = (projectId: string, signal?: AbortSignal) => http.post<Preflight>(`${community}/submissions/preflight`, { projectId }, { signal, timeout: 0 });
export const submitWorkshop = (input: SubmissionInput) => http.post<{ publication: Publication; version: Version }>(`${community}/submissions`, input, { timeout: 0 });
export const listWorkshopSubmissions = (signal?: AbortSignal) => http.get<{ items: { publication: Publication; latest: Version }[] }>(`${community}/submissions/mine`, { signal });
export const withdrawWorkshopVersion = (versionId: string) => http.post<{ withdrawn: true }>(`${community}/versions/${id(versionId)}/withdraw`, {});
export const unlistWorkshopPublication = (publicationId: string) => http.post<{ unlisted: true }>(`${community}/publications/${id(publicationId)}/unlist`, {});
export const listWorkshopPublications = (filters: BrowseFilters, signal?: AbortSignal) => http.get<{ items: Publication[]; total: number }>(`${community}/publications`, { params: compactApiParams(filters), signal });
export const getWorkshopPublication = (publicationId: string, signal?: AbortSignal) => http.get<Publication>(`${community}/publications/${id(publicationId)}`, { signal });
export const getWorkshopSnapshot = (versionId: string, signal?: AbortSignal) => http.get<Snapshot>(`${community}/versions/${id(versionId)}/snapshot`, { signal, timeout: 0 });
export const copyWorkshopPublication = (publicationId: string, input: { requestId: string; title: string }) => http.post<{ copy: CopyRecord }>(`${community}/publications/${id(publicationId)}/copies`, input, { timeout: 0 });
export const getWorkshopCopy = (requestId: string) => http.get<{ copy: CopyRecord }>(`${community}/copies/${id(requestId)}`);
export const getWorkshopOrigin = (projectId: string, signal?: AbortSignal) => http.get<Derived>(`${community}/origins/${id(projectId)}`, { signal });
export const listWorkshopReviews = (status: "pending" | "processed", signal?: AbortSignal) => http.get<{ items: ReviewItem[] }>(`${admin}/versions`, { params: compactApiParams({ status }), signal });
export const getWorkshopReview = (versionId: string, signal?: AbortSignal) => http.get<ReviewItem>(`${admin}/versions/${id(versionId)}`, { signal });
export const approveWorkshopVersion = (versionId: string, input: { snapshotSha256: string; official: boolean }) => http.post<ReviewItem>(`${admin}/versions/${id(versionId)}/approve`, input);
export const rejectWorkshopVersion = (versionId: string, reason: string) => http.post<ReviewItem>(`${admin}/versions/${id(versionId)}/reject`, { reason });
export const listAdminWorkshopPublications = (signal?: AbortSignal) => http.get<{ items: Publication[] }>(`${admin}/publications`, { signal });
export const manageWorkshopPublication = (publicationId: string, action: PublicationAction, reason?: string) => http.post<{ publication: Publication }>(`${admin}/publications/${id(publicationId)}/${action}`, reason ? { reason } : {});
