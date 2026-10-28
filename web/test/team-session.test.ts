import { expect, test } from "bun:test";
import axios, { type InternalAxiosRequestConfig } from "axios";
import { apiClient } from "../src/services/api/request";
import { installTeamFetchRuntime, teamSessionSignal } from "../src/services/team-session";
import { appQueryClient } from "../src/lib/query-client";
import { getActiveUserScope, setActiveUserScope } from "../src/lib/user-scope";
import { teamWorkspaceScope } from "../src/lib/team-auth";
import { installLocalForageStoreFactoryForTests } from "../src/lib/localforage-storage";
import { useUserStore } from "../src/stores/use-user-store";
import { usePlatformIdentityStore } from "../src/stores/use-platform-identity-store";
import type { PlatformIdentity } from "../src/services/api/platform";
import type { WorkspaceBootstrapPayload } from "../src/services/api/workspace";

const scope = teamWorkspaceScope("member-a", "workspace-a");
const identity: PlatformIdentity = {
    user: { id: "member-a", username: "platform-member", display_name: "平台成员", role: "member", status: "active", last_active_at: null, created_at: "" },
    workspace_id: "workspace-a",
};

type TeamEnvironment = { storedValues: string[]; requests: RequestInit[]; redirect: () => string };

async function withTeamEnvironment(run: (env: TeamEnvironment) => Promise<void>) {
    const previousMode = process.env.VITE_DEPLOYMENT_MODE;
    const previousWindow = globalThis.window;
    const previousFetch = globalThis.fetch;
    const previousUserState = useUserStore.getState();
    const previousIdentity = usePlatformIdentityStore.getState().identity;
    const storedValues: string[] = [];
    const requests: RequestInit[] = [];
    let redirect = "";
    process.env.VITE_DEPLOYMENT_MODE = "team";
    globalThis.window = {
        location: { origin: "https://hfp.test", pathname: "/canvas/a", search: "?view=nodes", hash: "#selected", replace: (url: string) => { redirect = url; } },
        addEventListener() {},
        localStorage: { getItem: () => null, setItem: (_key: string, value: string) => { storedValues.push(value); } },
    } as unknown as Window & typeof globalThis;
    installLocalForageStoreFactoryForTests(() => ({
        ready: async () => undefined, getItem: async () => null,
        setItem: async (_key: string, value: unknown) => { storedValues.push(JSON.stringify(value)); return value; },
        keys: async () => [], iterate: async () => undefined,
    }) as unknown as LocalForage);
    globalThis.fetch = (async (_input, init) => {
        requests.push(init || {});
        return new Response("data: waiting\n\n", { headers: { "content-type": "text/event-stream" } });
    }) as typeof fetch;
    try {
        await run({ storedValues, requests, redirect: () => redirect });
    } finally {
        if (previousMode === undefined) delete process.env.VITE_DEPLOYMENT_MODE;
        else process.env.VITE_DEPLOYMENT_MODE = previousMode;
        globalThis.window = previousWindow;
        globalThis.fetch = previousFetch;
        installLocalForageStoreFactoryForTests();
        useUserStore.setState(previousUserState);
        usePlatformIdentityStore.getState().setIdentity(previousIdentity);
        setActiveUserScope(null);
    }
}

// Bun cannot undo mock.module; when another file replaced these stores earlier in the same run, real hydration is unavailable here.
const realHydration = await Promise.all([
    import("../src/lib/user-session"),
    import("../src/services/api/resources"),
    import("../src/stores/use-asset-store"),
    import("../src/stores/canvas/use-canvas-store"),
    import("../src/stores/canvas/use-canvas-history-store"),
]).then(([session, resources, assets, canvas, history]) => (
    [assets.useAssetStore, canvas.useCanvasStore, history.useCanvasHistoryStore].every((store) => typeof store?.persist?.rehydrate === "function")
        ? { applyUserSession: session.applyUserSession, resourceFileUrl: resources.resourceFileUrl, playbackVariantUrl: resources.playbackVariantUrl }
        : null
)).catch(() => null);

test.skipIf(!realHydration)("team keeps the kernel workspace user and stores the platform identity only in memory", async () => {
    const { applyUserSession, resourceFileUrl, playbackVariantUrl } = realHydration!;
    const payload: WorkspaceBootstrapPayload = {
        contractVersion: 1, profile: "local", capabilities: { localAssets: true, providerCalls: true },
        user: { id: "local", username: "local", displayName: "本地工作区", role: "admin", status: "active" },
        workspace: { id: "local", name: "本地工作区", owner: "local", storage: "sqlite" }, storageMode: "local",
    };
    await withTeamEnvironment(async ({ storedValues }) => {
        usePlatformIdentityStore.getState().setIdentity(identity);
        await applyUserSession(payload, identity);
        expect(useUserStore.getState().user).toEqual(payload.user);
        expect(useUserStore.getState().user?.role).toBe("admin");
        expect(usePlatformIdentityStore.getState().identity?.user.role).toBe("member");
        expect(storedValues.join("\n")).not.toContain("platform-member");
        expect(storedValues.join("\n")).not.toContain("平台成员");
        expect(getActiveUserScope()).toBe(scope);
        expect(new URL(resourceFileUrl("image"), "https://hfp.test").searchParams.get("_hfp_scope")).toBe(scope);
        expect(new URL(playbackVariantUrl("video"), "https://hfp.test").searchParams.get("variant")).toBe("playback");
    });
});

test("team requests carry the CSRF header only to the same-origin API and a 401 clears the session", async () => {
    const previousAdapter = apiClient.defaults.adapter;
    let sent: InternalAxiosRequestConfig | undefined;
    apiClient.defaults.adapter = async (config) => {
        sent = config;
        const response = { config, status: config.url?.includes("unauthorized") ? 401 : 200, statusText: "", headers: {}, data: { code: 0, data: {}, msg: "ok" } };
        if (response.status === 401) throw new axios.AxiosError("unauthorized", "ERR_BAD_REQUEST", config, undefined, response);
        return response;
    };
    try {
        await withTeamEnvironment(async ({ requests, redirect }) => {
            usePlatformIdentityStore.getState().setIdentity(identity);
            setActiveUserScope(scope);
            await apiClient.post("/resources", new FormData());
            expect(sent?.headers.get("X-Requested-With")).toBe("hfp");
            await apiClient.get("/workspace/bootstrap");
            expect(sent?.headers.get("X-Requested-With")).toBeUndefined();
            await apiClient.get("/resources/image/file?proxy=1", { responseType: "blob" });
            expect(sent?.params._hfp_scope).toBe(scope);
            await apiClient.post("https://provider.test/api/generate", {});
            expect(sent?.headers.get("X-Requested-With")).toBeUndefined();
            installTeamFetchRuntime();
            await fetch("/api/assistant/chat", { method: "POST" });
            await fetch("https://provider.test/api/generate", { method: "POST" });
            await fetch("/api/tasks/t/text-events");
            expect(new Headers(requests[0].headers).get("X-Requested-With")).toBe("hfp");
            expect(new Headers(requests[1].headers).has("X-Requested-With")).toBe(false);
            expect(new Headers(requests[2].headers).has("X-Requested-With")).toBe(false);
            expect(requests[0].credentials).toBe("include");
            const controller = new AbortController();
            const combined = teamSessionSignal(controller.signal)!;
            controller.abort();
            expect(combined.aborted).toBe(true);
            expect(requests[0].signal?.aborted).toBe(false);
            appQueryClient.setQueryData(["private"], "member-a data");
            await expect(apiClient.get("/unauthorized")).rejects.toMatchObject({ response: { status: 401 } });
            expect(requests.every((request) => request.signal?.aborted)).toBe(true);
            expect(appQueryClient.getQueryCache().getAll()).toHaveLength(0);
            expect(getActiveUserScope()).toBe("guest");
            expect(usePlatformIdentityStore.getState().identity).toBeNull();
            expect(redirect()).toBe("/login?redirect=%2Fcanvas%2Fa%3Fview%3Dnodes%23selected");
            await expect(fetch("/api/should-not-send", { method: "POST" })).rejects.toHaveProperty("name", "AbortError");
        });
    } finally {
        apiClient.defaults.adapter = previousAdapter;
    }
});
