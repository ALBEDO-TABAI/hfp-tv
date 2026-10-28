import { http } from "./request";

export type PlatformUser = {
    id: string;
    username: string;
    display_name: string;
    role: "member" | "admin";
    status: "active" | "disabled";
    last_active_at: string | null;
    created_at: string;
};

export type PlatformIdentity = { user: PlatformUser; workspace_id: string };
export type PlatformCredentials = { username: string; password: string };
export type InvitationAcceptance = PlatformCredentials & { display_name: string };

export const getPlatformIdentity = (signal?: AbortSignal) => http.get<PlatformIdentity>("/platform/me", { signal });
export const loginPlatform = (input: PlatformCredentials) => http.post<PlatformIdentity>("/platform/login", input);
export const logoutPlatform = () => http.post<null>("/platform/logout");
export const acceptPlatformInvitation = (token: string, input: InvitationAcceptance) => http.post<PlatformIdentity>(`/platform/invitations/${encodeURIComponent(token)}/accept`, input);
