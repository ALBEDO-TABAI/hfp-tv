export function safeNextPath(next: string | null): string {
    if (!next?.startsWith("/") || next.startsWith("//") || /[\\\u0000-\u0020]/u.test(next)) return "/";
    const url = new URL(next, "https://workspace.invalid");
    if (url.origin !== "https://workspace.invalid" || url.pathname === "/login") return "/";
    return `${url.pathname}${url.search}${url.hash}`;
}

export function platformLoginPath(pathname: string, search: string) {
    return `/login?next=${encodeURIComponent(pathname + search)}`;
}

export function isPlatformPublicPath(pathname: string) {
    return pathname === "/login" || pathname.startsWith("/invite/");
}
