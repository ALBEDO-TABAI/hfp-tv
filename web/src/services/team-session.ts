import { isTeamDeployment } from "@/lib/deployment-mode";
import { appQueryClient } from "@/lib/query-client";
import { isSameOriginApi, isTeamEntryPath, isWriteMethod } from "@/lib/team-auth";
import { setActiveUserScope } from "@/lib/user-scope";
import { useUserStore } from "@/stores/use-user-store";
import { usePlatformIdentityStore } from "@/stores/use-platform-identity-store";
import { cancelAndDrainGenerationConsumers } from "@/services/generation-consumer-lifecycle";

const lifetime = new AbortController();
let fetchInstalled = false;

export function teamSessionSignal(signal?: AbortSignal | null) {
    if (!isTeamDeployment()) return signal ?? undefined;
    return signal ? AbortSignal.any([signal, lifetime.signal]) : lifetime.signal;
}

export function leaveTeamSession(redirect = true) {
    if (!isTeamDeployment() || lifetime.signal.aborted) return;
    lifetime.abort();
    usePlatformIdentityStore.getState().setIdentity(null);
    void cancelAndDrainGenerationConsumers();
    void appQueryClient.cancelQueries();
    appQueryClient.clear();
    setActiveUserScope(null);
    useUserStore.getState().clearSession();
    useUserStore.getState().setHydrated(false);
    const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    window.location.replace(redirect && !isTeamEntryPath(window.location.pathname) ? `/login?redirect=${encodeURIComponent(current)}` : "/login");
}

export function notifyTeamSessionChanged() {
    const channel = new BroadcastChannel("hfp-team-session");
    channel.postMessage("changed");
    channel.close();
}

export function handleTeamUnauthorized(url: string, status?: number) {
    if (!isTeamDeployment() || status !== 401 || !isSameOriginApi(url, window.location.origin)) return;
    const path = new URL(url, window.location.origin).pathname;
    // 登录失败留在表单中显示，不能触发刷新循环。
    if (path === "/api/platform/login" || path.startsWith("/api/platform/invitations/")) return;
    leaveTeamSession();
}

// 与桌面启动入口相同，集中覆盖原生 fetch（包含媒体读取和长连接）。
export function installTeamFetchRuntime() {
    if (!isTeamDeployment() || fetchInstalled) return;
    fetchInstalled = true;
    const nativeFetch = globalThis.fetch.bind(globalThis);
    window.addEventListener("pageshow", (event) => { if (event.persisted) window.location.reload(); });
    globalThis.fetch = async (input, init) => {
        const url = input instanceof Request ? input.url : String(input);
        const internal = isSameOriginApi(url, window.location.origin);
        const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
        const method = init?.method ?? (input instanceof Request ? input.method : "GET");
        if (internal && isWriteMethod(method)) headers.set("X-Requested-With", "hfp");
        const signal = teamSessionSignal(init?.signal ?? (input instanceof Request ? input.signal : undefined));
        signal?.throwIfAborted();
        const response = await nativeFetch(input, { ...init, headers, signal, ...(internal ? { credentials: "include" as const, cache: "no-store" as const } : {}) });
        signal?.throwIfAborted();
        handleTeamUnauthorized(url, response.status);
        return response;
    };
}
