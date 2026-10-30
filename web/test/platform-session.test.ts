import { afterAll, afterEach, describe, expect, spyOn, test } from "bun:test";
import { probePlatformSession, type PlatformMe } from "../src/services/api/platform";
import { safeNextPath } from "../src/lib/platform-navigation";
import { apiClient, http, setUnauthenticatedHandler } from "../src/services/api/request";
import { appQueryClient } from "../src/lib/query-client";
import { getActiveUserScope, setActiveUserScope } from "../src/lib/user-scope";
import { hostedCapability, usePlatformSession } from "../src/stores/use-platform-session";

const me: PlatformMe = {
    user: { id: "alice", username: "alice", displayName: "Alice", role: "admin" },
    workspace: { id: "workspace", status: "active" }, relay: { bound: false, hint: "" },
    capabilities: { hosted: true, externalAgents: false, storageSettings: false, eagle: false, depthCapture: false, pluginInstall: false, diagnostics: false, userAdmin: true, skillReview: true },
};
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
const originalScope = getActiveUserScope();
const fetchSpy = spyOn(globalThis, "fetch");
afterAll(() => fetchSpy.mockRestore());
afterEach(() => {
    fetchSpy.mockReset();
    if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
    else Reflect.deleteProperty(globalThis, "window");
    setUnauthenticatedHandler(undefined);
    setActiveUserScope(originalScope);
    usePlatformSession.setState({ mode: "unknown", me: null });
    appQueryClient.clear();
});

describe("platform mode detection", () => {
    test.each([404, 401, 200])("HTTP %s selects the correct mode", async (status) => {
        fetchSpy.mockResolvedValue(Response.json({ code: 0, data: me }, { status }));
        expect(await probePlatformSession()).toEqual(status === 404 ? { mode: "local" } : { mode: "hosted", me: status === 200 ? me : null });
        expect(fetchSpy.mock.calls[0]?.[0]).toBe("/api/platform/me");
        expect(fetchSpy.mock.calls[0]?.[1]?.credentials).toBe("same-origin");
    });
    test("a site known as a team server never degrades to a local identity", async () => {
        const store = new Map<string, string>();
        const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
        Object.defineProperty(globalThis, "localStorage", { configurable: true, value: { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, value: string) => store.set(key, value) } });
        try {
            fetchSpy.mockResolvedValueOnce(Response.json({ code: 401, reason: "unauthenticated" }, { status: 401 }));
            expect(await probePlatformSession()).toEqual({ mode: "hosted", me: null });
            fetchSpy.mockRejectedValueOnce(new Error("offline"));
            await expect(probePlatformSession()).rejects.toThrow("offline");
            fetchSpy.mockResolvedValueOnce(new Response("unavailable", { status: 503 }));
            await expect(probePlatformSession()).rejects.toThrow("无法连接团队服务器");
            store.clear();
            fetchSpy.mockResolvedValueOnce(new Response("unavailable", { status: 503 }));
            expect(await probePlatformSession()).toEqual({ mode: "local" });
        } finally {
            if (originalStorage) Object.defineProperty(globalThis, "localStorage", originalStorage);
            else Reflect.deleteProperty(globalThis, "localStorage");
        }
    });
    test("desktop keeps its local fallback on network and server errors", async () => {
        Object.defineProperty(globalThis, "window", { configurable: true, value: { go: { main: { DesktopApp: {} } }, location: { protocol: "http:" } } });
        fetchSpy.mockRejectedValueOnce(new Error("offline"));
        expect(await probePlatformSession()).toEqual({ mode: "local" });
        fetchSpy.mockResolvedValueOnce(new Response("unavailable", { status: 503 }));
        expect(await probePlatformSession()).toEqual({ mode: "local" });
    });
    test("unexpected status and malformed success are errors", async () => {
        fetchSpy.mockResolvedValueOnce(new Response(null, { status: 403 }));
        await expect(probePlatformSession()).rejects.toThrow();
        fetchSpy.mockResolvedValueOnce(Response.json({ code: 0, data: {} }));
        await expect(probePlatformSession()).rejects.toThrow("无法读取登录状态");
    });
});

test("next accepts only safe same-origin paths and avoids login loops", () => {
    expect(safeNextPath("/canvas/one?view=2#node")).toBe("/canvas/one?view=2#node");
    for (const next of [null, "https://evil.test", "//evil.test", "/\\evil.test", "/\tevil.test", "javascript:alert(1)", "/a/../login?next=/", " /canvas"]) expect(safeNextPath(next)).toBe("/");
});

test("scope changes clear query results without removing persisted data", () => {
    appQueryClient.setQueryData(["user-projects"], ["private"]);
    setActiveUserScope("other:workspace");
    expect(appQueryClient.getQueryData(["user-projects"])).toBeUndefined();
});

test("capabilities preserve local features and restrict hosted features", () => {
    usePlatformSession.getState().setSession({ mode: "local" });
    expect(hostedCapability("eagle")).toBe(true);
    usePlatformSession.getState().setSession({ mode: "hosted", me });
    expect(hostedCapability("eagle")).toBe(false);
    expect(hostedCapability("userAdmin")).toBe(true);
    usePlatformSession.getState().setMe(null);
    expect(hostedCapability("userAdmin")).toBe(false);
});

test("HTTP interceptor redirects only expired platform sessions", async () => {
    const original = apiClient.defaults.adapter;
    let redirected = 0;
    setUnauthenticatedHandler(() => { redirected++; });
    try {
        for (const reason of ["invalid_credentials", "unauthenticated"]) {
            apiClient.defaults.adapter = async () => { throw { isAxiosError: true, response: { status: 401, data: { code: 401, reason, msg: "denied" } } }; };
            await expect(http.get("/platform/me")).rejects.toThrow("denied");
        }
        expect(redirected).toBe(1);
    } finally { apiClient.defaults.adapter = original; }
});
