import { ApiError, apiBaseURL, http, type BackendEnvelope } from "./request";

export type PlatformRole = "admin" | "member";
export type PlatformMe = {
    user: { id: string; username: string; displayName: string; role: PlatformRole };
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
    };
};
export type AdminUser = {
    id: string; username: string; displayName: string; role: PlatformRole; status: "active" | "disabled";
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
