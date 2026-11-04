import { afterAll, beforeAll, expect, test } from "bun:test";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { chromium, type Browser } from "playwright";
import postcss from "postcss";
import tailwind from "@tailwindcss/postcss";
import type { RelayChannelTemplate } from "../src/services/api/platform";

let browser: Browser;
let server: ReturnType<typeof Bun.serve>;
let generationRequests = 0;
beforeAll(async () => {
    const build = await Bun.build({ entrypoints: [import.meta.dir + "/fixtures/external-video-settings-harness.tsx"], target: "browser", define: {
        "import.meta.env.DEV": "false", "import.meta.env.PROD": "true", "import.meta.env.MODE": '"production"', "import.meta.env.VITE_CANVAS_LOCAL_MODE": '"true"', "import.meta.env.VITE_CANVAS_BACKEND_URL": '"/api"', "process.env.NODE_ENV": '"production"',
    }, plugins: [{ name: "source", setup(builder) {
        builder.onResolve({ filter: /^@\/components\/model-logo$/ }, () => ({ path: "model-logo", namespace: "test-icons" }));
        builder.onLoad({ filter: /.*/, namespace: "test-icons" }, () => ({ contents: "export function ModelLogo() { return null; }", loader: "js" }));
        builder.onResolve({ filter: /^@\// }, (args) => ({ path: Bun.resolveSync("../src/" + args.path.slice(2), import.meta.dir) }));
    } }] });
    if (!build.success) throw new Error(build.logs.join("\n"));
    const js = await build.outputs.find((output) => output.path.endsWith(".js"))!.text();
    const cssPath = import.meta.dir + "/../src/styles/globals.css";
    const css = (await postcss([tailwind({ base: import.meta.dir + "/.." })]).process(readFileSync(cssPath, "utf8"), { from: cssPath })).css
        + readFileSync(import.meta.dir + "/../src/styles/beeftv-local-overrides.css", "utf8")
        + readFileSync(import.meta.dir + "/../src/styles/workspace-product.css", "utf8")
        + await build.outputs.find((output) => output.path.endsWith(".css"))!.text();
    const providers = [
        { id: "newapi", name: "OpenAI Videos / Sora", create: "POST /v1/videos", poll: "GET /v1/videos/{task_id}", contentType: "multipart/form-data" },
        { id: "video-generations-json", name: "视频生成 JSON · ratio / duration", create: "POST /v1/video/generations", poll: "GET /v1/video/generations/{task_id}", contentType: "application/json" },
    ].map((provider) => ({ ...provider, version: "1.0.0", vendor: "BeefTV", categories: ["video"], scopes: ["user.custom-channel"], enabled: true }));
    server = Bun.serve({ port: 0, fetch(request) {
        const url = new URL(request.url);
        if (url.pathname === "/harness.js") return new Response(js, { headers: { "Content-Type": "application/javascript" } });
        if (url.pathname === "/harness.css") return new Response(css, { headers: { "Content-Type": "text/css" } });
        if (url.pathname === "/api/plugins/catalog") return Response.json({ code: 0, data: { providers }, msg: "ok" });
        if (url.pathname.startsWith("/api/")) { generationRequests++; return Response.json({ code: 0, data: {}, msg: "ok" }); }
        const dark = url.searchParams.get("theme") === "dark";
        return new Response(`<html class="${dark ? "dark" : ""}"><meta name="viewport" content="width=device-width"><link rel="stylesheet" href="/harness.css"><body class="app-product-overlays"><div id="root" class="app-product-workspace"></div><script type="module" src="/harness.js"></script></body></html>`, { headers: { "Content-Type": "text/html" } });
    } });
    const executablePath = [process.env.CHROME_PATH, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"].find((path): path is string => Boolean(path && existsSync(path)));
    browser = await chromium.launch({ executablePath, headless: true });
}, 60000);
afterAll(async () => { server?.stop(true); await browser?.close(); }, 30000);

test("an unconfigured model displays, tests and saves its explicitly selected channel protocol", async () => {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
        await page.goto(server.url.toString() + "?inherited=1");
        await page.getByRole("button", { name: "高级模型设置与测试", exact: true }).click();
        await page.getByText("POST /v1/video/generations", { exact: true }).waitFor();
        await page.getByRole("button", { name: "配置使用", exact: true }).click();
        expect(await page.locator('input[type="radio"][value="video-generations-json"]').isChecked()).toBe(true);
        expect(await page.getByRole("button", { name: "生成测试", exact: true }).isEnabled()).toBe(true);
        expect(await page.getByTestId("saved-profile").textContent()).toBe("unsaved");
        await page.getByRole("button", { name: /^完\s*成$/ }).click();
        await page.getByRole("button", { name: "保存并使用", exact: true }).click();
        await page.waitForFunction(() => document.querySelector('[data-testid="saved-profile"]')?.textContent === "video-generations-json");
        expect(generationRequests).toBe(0);
    } finally { await page.close(); }
}, 60000);

for (const [width, dark] of [[1280, false], [390, true]] as const) {
    test(`external video requires a chosen protocol and saves it (${width}, dark=${dark})`, async () => {
        const page = await browser.newPage({ viewport: { width, height: 900 } });
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        try {
            await page.goto(server.url.toString() + (dark ? "?theme=dark" : ""));
            await page.getByRole("textbox", { name: "模型 ID", exact: true }).fill("seedance-2.5");
            await page.getByRole("combobox", { name: "模型类型", exact: true }).click();
            await page.locator('.ant-select-item-option[title="视频"]').click();
            await page.getByRole("button", { name: /^添\s*加$/ }).click();
            await page.getByRole("button", { name: "保存并使用", exact: true }).click();
            await page.getByText("seedance-2.5 还需要选择请求协议，请打开高级模型设置", { exact: true }).waitFor();
            expect(await page.getByTestId("saved-profile").textContent()).toBe("unsaved");
            await page.getByRole("button", { name: "配置使用", exact: true }).click();
            expect(await page.getByRole("button", { name: "生成测试", exact: true }).isDisabled()).toBe(true);
            expect(await page.locator('.choice-browser input[type="radio"]:checked').count()).toBe(0);
            await page.locator('input[type="radio"][value="video-generations-json"]').check();
            await page.getByText("application/json", { exact: true }).waitFor();
            await page.getByText("异步轮询", { exact: true }).waitFor();
            expect(await page.getByRole("button", { name: "生成测试", exact: true }).isEnabled()).toBe(true);
            const dialog = page.getByRole("dialog").last();
            const box = (await dialog.boundingBox())!;
            expect(box.x).toBeGreaterThanOrEqual(0);
            expect(box.x + box.width).toBeLessThanOrEqual(width + 1);
            expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
            const output = process.env.BEEFTV_TEST_SCREENSHOT_DIR;
            if (output) { mkdirSync(output, { recursive: true }); await page.screenshot({ path: `${output}/external-video-${width}.png`, fullPage: true, animations: "disabled" }); }
            await page.getByRole("button", { name: /^完\s*成$/ }).click();
            await page.getByRole("button", { name: "保存并使用", exact: true }).click();
            await page.waitForFunction(() => document.querySelector('[data-testid="saved-profile"]')?.textContent === "video-generations-json");
            expect(errors).toEqual([]);
            expect(generationRequests).toBe(0);
        } finally { await page.close(); }
    }, 60000);
}

test("company edit and setup entries require hosted userAdmin capability", async () => {
    const page = await browser.newPage();
    try {
        for (const role of ["admin", "member", "local"]) {
            await page.goto(`${server.url}?company=${role}`);
            await page.getByRole("button", { name: "添加模型服务", exact: true }).first().waitFor();
            expect(await page.getByRole("button", { name: "编辑（所有成员）", exact: true }).count()).toBe(role === "admin" ? 1 : 0);
            await page.goto(`${server.url}?company=${role}&empty=1`);
            await page.getByRole("button", { name: "添加模型服务", exact: true }).first().waitFor();
            expect(await page.getByRole("button", { name: "配置公司中转站", exact: true }).count()).toBe(role === "admin" ? 1 : 0);
        }
        await page.goto(`${server.url}?company=admin&empty=1`);
        await page.getByRole("button", { name: "配置公司中转站", exact: true }).click();
        await page.getByRole("heading", { name: "配置公司中转站", exact: true }).waitFor();
        expect(await page.getByRole("textbox", { name: "名称", exact: true }).inputValue()).toBe("公司中转站");
        expect(await page.getByLabel("API Key", { exact: true }).count()).toBe(0);
    } finally { await page.close(); }
}, 60000);

test("company editor keeps shared endpoints, saves without keys, displays errors and reloads the card", async () => {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const template: RelayChannelTemplate = {
        name: "公司中转站", baseUrl: "http://10.10.10.45:3000/v1", apiFormat: "openai", models: ["seedance-2.5"],
        modelProfiles: [{ model: "seedance-2.5", capability: "video", protocol: "volcengine-ark-video", baseUrl: "http://10.10.10.45:3000/doubao/api/v3", referenceAssetService: "http://10.10.10.45:8787/v1/reference-images" }],
    };
    const submitted: RelayChannelTemplate[] = [];
    let rejectSave = true;
    try {
        await page.route("**/api/platform/admin/relay-channel", async route => {
            if (route.request().method() === "PUT") {
                submitted.push(route.request().postDataJSON().channel);
                if (rejectSave) return route.fulfill({ status: 400, json: { code: 400, reason: "relay_channel_invalid", msg: "模型接口地址必须与中转站同主机" } });
            }
            await route.fulfill({ json: { code: 0, data: { channel: template, source: "web", file: "", fileAvailable: false, applied: 3, failed: 1 } } });
        });
        await page.route("**/api/workspace/model-config", route => route.fulfill({ json: { code: 0, data: { revision: 2, config: { channels: [{ ...submitted.at(-1), id: "company-relay", pinned: true, apiKey: "", hasApiKey: false }] } } } }));
        await page.route("**/api/ai/models", async route => {
            expect(route.request().postDataJSON()).toMatchObject({ channelId: "company-relay", apiKey: "" });
            await route.fulfill({ status: 403, json: { code: 403, msg: "denied", reason: "relay_key_missing" } });
        });
        await page.goto(`${server.url}?company=admin`);
        await page.getByRole("button", { name: "编辑（所有成员）", exact: true }).click();
        await page.getByRole("heading", { name: "管理公司中转站", exact: true }).waitFor();
        await page.getByRole("button", { name: "连接信息", exact: true }).click();
        expect(await page.getByLabel("API Key", { exact: true }).count()).toBe(0);
        expect(await page.getByLabel("选择服务商", { exact: true }).count()).toBe(0);
        expect(await page.getByLabel("火山方舟（豆包）接口地址（可选）", { exact: true }).inputValue()).toBe(template.modelProfiles[0].baseUrl!);
        expect(await page.getByLabel("参考图服务地址（可选）", { exact: true }).inputValue()).toBe(template.modelProfiles[0].referenceAssetService!);
        await page.getByRole("button", { name: "高级设置", exact: true }).click();
        expect(await page.getByLabel("Secret Key", { exact: true }).count()).toBe(0);
        expect(await page.getByLabel("素材服务地址", { exact: true }).count()).toBe(0);
        expect(await page.getByRole("combobox", { name: "目录接口格式", exact: true }).count()).toBe(1);
        await page.getByRole("button", { name: "读取模型", exact: true }).click();
        await page.getByText("读取模型目录要用你自己绑定的中转站密钥：请先在「用户管理」为自己绑定密钥，或直接手动添加模型 ID。", { exact: true }).waitFor();
        await page.getByRole("textbox", { name: "名称", exact: true }).fill("公司新模型服务");
        await page.getByRole("button", { name: "手动添加模型", exact: true }).click();
        await page.getByRole("button", { name: "保存到公司中转站", exact: true }).click();
        await page.getByText("模型接口地址必须与中转站同主机", { exact: true }).waitFor();
        expect(submitted[0]).toMatchObject({ ...template, name: "公司新模型服务" });
        expect(Object.keys(submitted[0]).sort()).toEqual(["apiFormat", "baseUrl", "modelProfiles", "models", "name"]);
        rejectSave = false;
        await page.getByRole("button", { name: "保存到公司中转站", exact: true }).click();
        await page.getByRole("heading", { name: "公司新模型服务", exact: true }).waitFor();
        await page.getByText("公司中转站已保存，已更新 3 位成员的工作区；其他成员刷新页面后可见", { exact: true }).waitFor({ state: "attached" });
        await page.getByText("1 个工作区暂未更新，会在成员下次打开时更新", { exact: true }).waitFor({ state: "attached" });
        await page.getByRole("dialog").waitFor({ state: "hidden" });
    } finally { await page.close(); }
}, 60000);
