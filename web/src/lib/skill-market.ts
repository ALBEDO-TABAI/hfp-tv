import type { MarketFile } from "@/services/api/platform";

export function skillWebsiteTarget(url: string): "app" | "external" {
    if (url.startsWith("/") && !url.startsWith("//") && !/[\\\u0000-\u0020]/u.test(url)) return "app";
    const parsed = new URL(url);
    if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password) throw new Error("授权链接无效");
    return "external";
}

export type MarketFileChange = MarketFile & { change: "added" | "modified" | "deleted" | "unchanged" };

export function diffMarketFiles(files: MarketFile[], previous: MarketFile[]): MarketFileChange[] {
    const before = new Map(previous.map(file => [file.path, file]));
    const paths = new Set(files.map(file => file.path));
    return [
        ...files.map((file): MarketFileChange => {
            const previousFile = before.get(file.path);
            return { ...file, change: !previousFile ? "added" : previousFile.sha256 !== file.sha256 ? "modified" : "unchanged" };
        }),
        ...previous.filter(file => !paths.has(file.path)).map((file): MarketFileChange => ({ ...file, change: "deleted" })),
    ];
}
