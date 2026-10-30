import { ApiError, apiBaseURL, compactApiParams, http, type BackendEnvelope } from "./request";
import type { SkillMetadata } from "./skill-market";

export type PlatformRole = "admin" | "member";
export type PlatformMe = {
    user: { id: string; username: string; displayName: string; role: PlatformRole; source?: PlatformAccountSource };
    workspace: { id: string; status: "active" };
    relay: { bound: boolean; hint: string };
    capabilities: {
        hosted: true;
        externalAgents: false;
        storageSettings: false;
        eagle: false;
        depthCapture: false;
        pluginInstall: false;
        diagnostics: false;
        userAdmin: boolean;
        skillReview: boolean;
        workshop: boolean;
        workshopReview: boolean;
    };
};
/** "file": password, relay key and status come from the server-side account files. */
export type PlatformAccountSource = "" | "file";
export type AdminUser = {
    id: string; username: string; displayName: string; role: PlatformRole; status: "active" | "disabled"; source?: PlatformAccountSource;
    createdAt: string; lastActiveAt: string | null;
    relay: { bound: boolean; hint: string; updatedAt: string | null };
};
export type PlatformAuditItem = { id: string; actorUsername: string; action: string; targetUsername: string; detail: string; createdAt: string };
export type PlatformSession = { mode: "local" } | { mode: "hosted"; me: PlatformMe | null };

// 记住本站上次识别出的模式：本地部署在后端暂不可用时仍按原样降级到浏览器缓存，
// 团队服务器站点则不能降级成共享的本地身份。
const platformModeKey = "hfp:platform-mode";

function rememberedHostedSite() {
    try { return localStorage.getItem(platformModeKey) === "hosted"; } catch { return false; }
}

function rememberPlatformMode(mode: PlatformSession["mode"]) {
    try { localStorage.setItem(platformModeKey, mode); } catch { /* 偏好写入失败不影响启动 */ }
}

export async function probePlatformSession(): Promise<PlatformSession> {
    const desktop = typeof window !== "undefined" && (Boolean(window.go?.main?.DesktopApp) || window.location.protocol === "wails:");
    const canDegrade = desktop || !rememberedHostedSite();
    let response: Response;
    try {
        response = await fetch(`${apiBaseURL}/platform/me`, { credentials: "same-origin", signal: AbortSignal.timeout(10_000) });
    } catch (error) {
        if (canDegrade) return { mode: "local" };
        throw error;
    }
    if (response.status === 404) {
        rememberPlatformMode("local");
        return { mode: "local" };
    }
    if (response.status >= 500 && canDegrade) return { mode: "local" };
    if (response.status === 401) {
        rememberPlatformMode("hosted");
        return { mode: "hosted", me: null };
    }
    if (!response.ok) throw new ApiError("无法连接团队服务器，请重试", { status: response.status });
    const payload: BackendEnvelope<PlatformMe> = await response.json();
    if (payload.code !== 0 || !payload.data?.user?.id || !payload.data.workspace?.id || payload.data.capabilities?.hosted !== true) {
        throw new ApiError("无法读取登录状态，请重试", { reason: payload.reason, code: payload.code });
    }
    rememberPlatformMode("hosted");
    return { mode: "hosted", me: payload.data };
}

export const getPlatformMe = () => http.get<PlatformMe>("/platform/me");
export const loginPlatform = (input: { username: string; password: string }) => http.post<PlatformMe>("/platform/login", input);
export const logoutPlatform = () => http.post<{ loggedOut: true }>("/platform/logout", {});
export const changePlatformPassword = (input: { currentPassword: string; newPassword: string }) => http.post<{ changed: true }>("/platform/password", input);
export const getPlatformInvitation = (token: string) => http.get<{ role: PlatformRole; expiresAt: string }>(`/platform/invitations/${encodeURIComponent(token)}`);
export const acceptPlatformInvitation = (token: string, input: { username: string; displayName: string; password: string }) => http.post<PlatformMe>(`/platform/invitations/${encodeURIComponent(token)}/accept`, input);
export const listPlatformUsers = () => http.get<{ items: AdminUser[] }>("/platform/admin/users");
export const createPlatformUser = (input: { username: string; displayName: string; password: string; role: PlatformRole; relayKey?: string }) => http.post<{ user: AdminUser }>("/platform/admin/users", input);
export const updatePlatformUser = (id: string, input: { displayName?: string; role?: PlatformRole; status?: AdminUser["status"] }) => http.patch<{ user: AdminUser }>(`/platform/admin/users/${encodeURIComponent(id)}`, input);
export const resetPlatformUserPassword = (id: string, password: string) => http.post<{ changed: true }>(`/platform/admin/users/${encodeURIComponent(id)}/password`, { password });
export const bindPlatformRelayKey = (id: string, key: string) => http.put<{ user: AdminUser }>(`/platform/admin/users/${encodeURIComponent(id)}/relay-key`, { key });
export const clearPlatformRelayKey = (id: string) => http.delete<{ user: AdminUser }>(`/platform/admin/users/${encodeURIComponent(id)}/relay-key`);
export const createPlatformInvitation = (input: { role: PlatformRole; expiresInHours: number }) => http.post<{ token: string; url: string; expiresAt: string }>("/platform/admin/invitations", input);
export const listPlatformAudit = () => http.get<{ items: PlatformAuditItem[] }>("/platform/admin/audit", { params: { limit: 100 } });

export type PlatformMarketAuthorization = { userCode: string; clientName: string; expiresAt: string; status: "pending" | "approved" };
export type SkillSubmissionStatus = "draft" | "ready_to_submit" | "in_review" | "published" | "rejected";
export type SkillSubmissionSummary = {
    id: string; status: SkillSubmissionStatus; revision: number; previewDigest: string;
    author: { id: string; username: string; displayName: string };
    target: "new" | "existing"; skillId: string; currentSkillName: string | null;
    metadata: SkillMetadata; fileCount: number; archiveBytes: number;
    submittedAt: string | null; reviewedAt: string | null; reviewReason: string; reviewer: string | null;
};
export type MarketFile = { path: string; size: number; sha256: string; mimeType: string };
export type SkillSubmissionDetail = {
    submission: SkillSubmissionSummary;
    files: MarketFile[];
    previousRelease: { releaseId: string; versionLabel: string; files: MarketFile[] } | null;
};
export type SkillSubmissionFile = { path: string; mimeType: string; encoding: "utf-8" | "base64"; content: string };
export type MarketSkillSummary = { skillId: string; name: string; ownerUsername: string; official: boolean; listed: boolean; currentReleaseId: string; versionLabel: string; publishedAt: string };

export const getPlatformMarketAuthorization = (userCode: string, signal?: AbortSignal) => http.get<PlatformMarketAuthorization>(`/platform/market/authorizations/${encodeURIComponent(userCode)}`, { signal });
export const approvePlatformMarketAuthorization = (userCode: string) => http.post<{ approved: true }>(`/platform/market/authorizations/${encodeURIComponent(userCode)}/approve`, {});
export const listSkillSubmissions = (status: "pending" | "processed", signal?: AbortSignal) => http.get<{ items: SkillSubmissionSummary[] }>("/platform/admin/skill-submissions", { params: compactApiParams({ status }), signal });
export const getSkillSubmission = (id: string, signal?: AbortSignal) => http.get<SkillSubmissionDetail>(`/platform/admin/skill-submissions/${encodeURIComponent(id)}`, { signal });
export const getSkillSubmissionFile = (id: string, path: string, signal?: AbortSignal) => http.get<SkillSubmissionFile>(`/platform/admin/skill-submissions/${encodeURIComponent(id)}/file`, { params: compactApiParams({ path }), signal });
export const approveSkillSubmission = (id: string, input: { revision: number; previewDigest: string; official: boolean }) => http.post<{ submission: SkillSubmissionSummary; release: { skillId: string; releaseId: string } }>(`/platform/admin/skill-submissions/${encodeURIComponent(id)}/approve`, input);
export const rejectSkillSubmission = (id: string, input: { revision: number; reason: string }) => http.post<{ submission: SkillSubmissionSummary }>(`/platform/admin/skill-submissions/${encodeURIComponent(id)}/reject`, input);
export const listAdminMarketSkills = (signal?: AbortSignal) => http.get<{ items: MarketSkillSummary[] }>("/platform/admin/market-skills", { signal });
export const unlistMarketSkill = (skillId: string) => http.post<{ skill: MarketSkillSummary }>(`/platform/admin/market-skills/${encodeURIComponent(skillId)}/unlist`, {});
export const relistMarketSkill = (skillId: string) => http.post<{ skill: MarketSkillSummary }>(`/platform/admin/market-skills/${encodeURIComponent(skillId)}/relist`, {});
