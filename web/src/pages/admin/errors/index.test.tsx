import test from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { chromium } from "playwright";
import { build } from "vite";
import AdminErrorsPage from "./index";
import type { PlatformErrorEvent } from "@/services/api/platform";

const event: PlatformErrorEvent = {
    id: "error-1", createdAt: "2026-11-03T09:02:00Z", startedAt: "2026-11-03T09:00:00Z",
    workspaceId: "workspace-1", source: "task", taskId: "task-1", taskType: "canvas_video",
    model: "video-model", stage: "任务失败", category: "provider_unavailable", message: "视频生成失败",
    providerTaskId: "task_Cdcy123", httpStatus: 503, upstreamDetail: "provider unavailable",
    log: ["17:52:56 INFO 上游任务状态：IN_PROGRESS，进度 50%", "17:53:56 ERROR " + "diagnostic".repeat(80)],
    member: { id: "member-1", username: "alice", displayName: "小白" },
};
const unknown: PlatformErrorEvent = { id: "error-2", createdAt: "2026-11-03T08:00:00Z", workspaceId: "workspace-2", source: "workspace", message: "工作区打开失败", member: null };
const cursor = "2026-11-03T08:00:00Z";

test("error page renders records, unknown members, loading, empty and retry states", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false } } });
    const queryKey = ["platform", "errors", "", ""];
    const render = () => renderToStaticMarkup(<QueryClientProvider client={client}><AdminErrorsPage /></QueryClientProvider>);
    try {
        assert.ok(render().includes("ant-skeleton"));
        client.setQueryData(queryKey, { pages: [{ events: [event, unknown], nextBefore: cursor }], pageParams: [""] });
        const html = render();
        for (const text of ["错误记录", "小白", "未知成员", "任务失败", "工作区无法打开", "video-model", "视频生成失败", "加载更多"]) assert.ok(html.includes(text), text);
        assert.ok(html.indexOf("视频生成失败") < html.indexOf("工作区打开失败"));
        client.setQueryData(queryKey, { pages: [{ events: [], nextBefore: "" }], pageParams: [""] });
        assert.ok(render().includes("暂无错误记录"));
        client.removeQueries({ queryKey });
        await assert.rejects(client.fetchInfiniteQuery({ queryKey, initialPageParam: "", queryFn: () => Promise.reject(new Error("offline")) }), /offline/);
        assert.ok(render().includes("重试"));
        assert.ok(render().includes("操作失败，请稍后重试"));
    } finally { client.clear(); }
});

test("filters, pagination, drawer and HTTP copying work at phone width in both themes", { timeout: 120_000 }, async () => {
    const root = fileURLToPath(new URL("../../../../", import.meta.url));
    const bundle = await build({
        configFile: false, root, publicDir: false, logLevel: "silent",
        resolve: { alias: { "@": `${root}src` } },
        build: { write: false, minify: false, rolldownOptions: { input: fileURLToPath(new URL("../../../../test/fixtures/admin-errors-harness.tsx", import.meta.url)) } },
    });
    assert.ok(!Array.isArray(bundle) && "output" in bundle);
    const assets = bundle.output;
    const entry = assets.find(asset => asset.type === "chunk" && asset.isEntry);
    assert.ok(entry);
    const css = assets.filter(asset => asset.fileName.endsWith(".css")).map(asset => `<link rel="stylesheet" href="/${asset.fileName}">`).join("");
    const executablePath = [process.env.CHROME_PATH, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"].find((path): path is string => Boolean(path && existsSync(path)));
    const browser = await chromium.launch({ headless: true, executablePath });
    try {
        for (const dark of [false, true]) {
            const page = await browser.newPage({ viewport: { width: 375, height: 812 } });
            const requests: URL[] = [];
            const pageErrors: string[] = [];
            let failNextPage = false;
            page.on("pageerror", error => pageErrors.push(error.message));
            await page.route("**/*", async route => {
                const url = new URL(route.request().url());
                if (url.pathname === "/api/platform/admin/users") return route.fulfill({ json: { code: 0, data: { items: [event.member] } } });
                if (url.pathname === "/api/platform/admin/errors") {
                    requests.push(url);
                    if (url.searchParams.has("before") && failNextPage) {
                        failNextPage = false;
                        return route.fulfill({ status: 503, json: { code: 503, msg: "offline" } });
                    }
                    const events = url.searchParams.has("before") ? [{ ...unknown, id: "older", message: "更早的错误" }] : url.searchParams.has("source") || url.searchParams.has("member") ? [event] : [event, unknown];
                    return route.fulfill({ json: { code: 0, data: { events, nextBefore: url.searchParams.has("before") ? "" : cursor } } });
                }
                const asset = assets.find(item => `/${item.fileName}` === url.pathname);
                if (asset) return route.fulfill({ contentType: asset.type === "chunk" ? "text/javascript" : "text/css", body: asset.type === "chunk" ? asset.code : Buffer.from(asset.source) });
                if (url.pathname !== "/") return route.fulfill({ status: 404, body: "" });
                return route.fulfill({ contentType: "text/html", body: `<html><head>${css}</head><body><div id="root" style="height:100vh"></div><script type="module" src="/${entry.fileName}"></script></body></html>` });
            });
            await page.goto(`http://error-log.test/${dark ? "?dark" : ""}`);
            await page.getByText("未知成员", { exact: true }).waitFor();
            await page.getByRole("combobox", { name: "类型", exact: true }).click();
            await page.locator(".ant-select-item-option").filter({ hasText: "任务失败" }).click();
            await page.waitForFunction(() => !document.body.textContent?.includes("未知成员"));
            assert.equal(requests.at(-1)?.searchParams.get("source"), "task");
            assert.equal(requests.at(-1)?.searchParams.has("before"), false);
            failNextPage = true;
            await page.getByRole("button", { name: "加载更多" }).click();
            await page.getByRole("button", { name: /^重\s*试$/ }).click();
            await page.getByText("更早的错误", { exact: true }).waitFor();
            assert.equal(requests.at(-1)?.searchParams.get("before"), cursor);
            assert.equal(requests.at(-1)?.searchParams.get("source"), "task");
            await page.getByRole("combobox", { name: "成员", exact: true }).click();
            await page.locator(".ant-select-item-option").filter({ hasText: "小白（alice）" }).click();
            await page.getByText("更早的错误", { exact: true }).waitFor({ state: "hidden" });
            assert.equal(requests.at(-1)?.searchParams.get("member"), "member-1");
            assert.equal(requests.at(-1)?.searchParams.has("before"), false);
            await page.getByRole("button", { name: "加载更多" }).click();
            await page.getByText("更早的错误", { exact: true }).waitFor();
            await page.getByRole("button", { name: /^刷\s*新$/ }).click();
            await page.getByText("更早的错误", { exact: true }).waitFor({ state: "hidden" });
            assert.equal(requests.at(-1)?.searchParams.has("before"), false);
            await page.getByRole("button").filter({ hasText: "视频生成失败" }).click();
            const drawer = page.getByRole("dialog");
            await drawer.getByText("task_Cdcy123", { exact: true }).waitFor();
            assert.ok((await drawer.locator("pre").innerText()).includes(event.log![0]));
            assert.ok((await drawer.innerText()).includes("120 秒"));
            assert.equal(await page.evaluate(() => navigator.clipboard), undefined);
            await drawer.getByRole("button", { name: "复制上游任务号" }).click();
            await drawer.getByRole("status").filter({ hasText: "已复制" }).waitFor();
            await page.evaluate(() => { document.execCommand = () => false; });
            await drawer.getByRole("button", { name: "复制上游任务号" }).click();
            await drawer.getByRole("status").filter({ hasText: "复制失败" }).waitFor();
            assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
            assert.equal(await drawer.evaluate(element => element.scrollWidth <= element.clientWidth), true);
            assert.equal(await drawer.locator("pre").evaluate(element => element.scrollWidth <= element.clientWidth), true);
            assert.deepEqual(pageErrors, []);
            await page.close();
        }
    } finally { await browser.close(); }
});
