import { beforeEach, expect, mock, test } from "bun:test";

let failure = false;
let response: unknown;
let issued = 0;
let staleToken = "";
const sentTokens: string[] = [];
class FixtureApiError extends Error {
    constructor(message: string, readonly status: number, readonly reason: string) { super(message); }
}
mock.module("@/services/api/request", () => ({
    ApiError: FixtureApiError,
    apiBaseURL: "/api",
    handleUnauthenticated: () => {},
    http: {
        get: async (_path: string, config?: { headers?: Record<string, string> }) => {
            const token = config?.headers?.["X-Beeftv-Ui-Session"] ?? "";
            sentTokens.push(token);
            if (token && token === staleToken) throw new FixtureApiError("内置 UI 会话已失效", 403, "unauthenticated");
            if (failure) throw new Error("offline");
            return response;
        },
        post: async (path: string) => {
            if (path === "/assistant/ui-session") return { token: `fixture-ui-${++issued}`, expiresAt: new Date(Date.now() + 3_600_000).toISOString() };
            if (failure) throw new Error("offline");
            return response;
        },
    },
}));
const { createAssistantSession, activateAssistantSession, getAssistantHistory, listAssistantSessions, resetAgentUiSession } = await import("@/services/api/agent-assistant");
beforeEach(() => { failure = false; response = undefined; staleToken = ""; sentTokens.length = 0; resetAgentUiSession(); });

test("C02: rejected create/activate cannot resolve as a new empty conversation", async () => {
    failure = true;
    await expect(createAssistantSession("a")).rejects.toThrow("offline");
    await expect(activateAssistantSession("a", "s2")).rejects.toThrow("offline");
});
test("C05: unavailable history/list are failures, not empty success", async () => {
    failure = true;
    await expect(getAssistantHistory("a")).rejects.toThrow("offline");
    await expect(listAssistantSessions("a")).rejects.toThrow("offline");
});
test("C02: malformed successful session response does not erase the current session", async () => {
    response = {};
    await expect(createAssistantSession("a")).rejects.toThrow();
    await expect(activateAssistantSession("a", "s2")).rejects.toThrow();
});
test("C05: malformed history is distinguished from genuine zero turns", async () => {
    response = {};
    await expect(getAssistantHistory("a")).rejects.toThrow();
    response = { sessionId: "s1", turns: [] };
    expect(await getAssistantHistory("a")).toEqual({ sessionId: "s1", turns: [], active: undefined, plan: null });
});
test("a UI session lost by a kernel restart is replaced once instead of failing every read", async () => {
    response = { sessionId: "s1", turns: [] };
    await getAssistantHistory("a");
    staleToken = sentTokens[0];
    expect(await getAssistantHistory("a")).toEqual({ sessionId: "s1", turns: [], active: undefined, plan: null });
    expect(sentTokens).toHaveLength(3);
    expect(sentTokens[1]).toBe(staleToken);
    expect(sentTokens[2]).not.toBe(staleToken);
});
