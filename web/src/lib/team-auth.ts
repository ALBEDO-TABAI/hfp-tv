export function safeLoginRedirect(value: string | null) {
    if (!value?.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u0020\u007f]/u.test(value)) return "/";
    return value;
}

export function teamWorkspaceScope(userId: string, workspaceId: string) {
    if (!userId || !workspaceId) throw new Error("账号或工作区信息不完整");
    return `team-${encodeURIComponent(JSON.stringify([userId, workspaceId]))}`;
}

export function isTeamEntryPath(pathname: string) {
    return pathname === "/login" || /^\/invite\/[^/]+\/?$/u.test(pathname);
}

export function isSameOriginApi(url: string, origin: string) {
    const target = new URL(url, origin);
    return target.origin === origin && (target.pathname === "/api" || target.pathname.startsWith("/api/"));
}

export function isWriteMethod(method = "GET") {
    return !["GET", "HEAD"].includes(method.toUpperCase());
}
