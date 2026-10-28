import { expect, test } from "bun:test";
import { isTeamDeployment } from "../src/lib/deployment-mode";
import { isSameOriginApi, isTeamEntryPath, isWriteMethod, safeLoginRedirect, teamWorkspaceScope } from "../src/lib/team-auth";

test("only the explicit team build enables platform login", () => {
    for (const mode of ["", "local", "unknown"]) expect(isTeamDeployment(mode)).toBe(false);
    expect(isTeamDeployment("team")).toBe(true);
    expect(isTeamEntryPath("/login")).toBe(true);
    expect(isTeamEntryPath("/invite/token")).toBe(true);
    expect(isTeamEntryPath("/invite/token/private")).toBe(false);
});

test("login redirects stay on site, including browser URL normalization", () => {
    for (const path of [null, "https://evil.test", "//evil.test", "/\\evil.test", "/\n/evil.test", "/\t/evil.test", "javascript:alert(1)"]) expect(safeLoginRedirect(path)).toBe("/");
    expect(safeLoginRedirect("/canvas/a?tab=nodes#selected")).toBe("/canvas/a?tab=nodes#selected");
});

test("cache namespaces include both identifiers without delimiter collisions", () => {
    expect(teamWorkspaceScope("a", "b")).not.toBe(teamWorkspaceScope("a", "c"));
    expect(teamWorkspaceScope("a", "b")).not.toBe(teamWorkspaceScope("c", "b"));
    expect(teamWorkspaceScope("a:b", "c")).not.toBe(teamWorkspaceScope("a", "b:c"));
    expect(teamWorkspaceScope("a:b", "c")).not.toContain(":");
    expect(() => teamWorkspaceScope("", "b")).toThrow();
    expect(() => teamWorkspaceScope("a", "")).toThrow();
});

test("CSRF headers only target same-origin API writes", () => {
    for (const path of ["/api/resources", "https://hfp.test/api/ai/custom"]) expect(isSameOriginApi(path, "https://hfp.test")).toBe(true);
    for (const path of ["https://provider.test/api/generate", "//provider.test/api", "/api-other", "blob:https://hfp.test/id"]) expect(isSameOriginApi(path, "https://hfp.test")).toBe(false);
    for (const method of ["GET", "head"]) expect(isWriteMethod(method)).toBe(false);
    for (const method of ["POST", "put", "PATCH", "DELETE"]) expect(isWriteMethod(method)).toBe(true);
});
