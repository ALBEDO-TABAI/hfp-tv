import { expect, test } from "bun:test";
import { ApiError, apiClient } from "../src/services/api/request";
import { approveWorkshopVersion, copyWorkshopPublication, getWorkshopSnapshot, listWorkshopPublications, submitWorkshop } from "../src/services/api/workshop";
import { platformErrorMessage } from "../src/lib/platform-account";

test("workshop requests keep session auth, exact contract fields, cancellation and long copy timeouts", async () => {
    const original = apiClient.defaults.adapter;
    const requests: { url: string | undefined; body: unknown; timeout: number | undefined; params: unknown; signal: unknown; withCredentials: boolean | undefined }[] = [];
    apiClient.defaults.adapter = async (config) => {
        requests.push({ url: config.url, body: typeof config.data === "string" ? JSON.parse(config.data) : config.data, timeout: config.timeout, params: config.params, signal: config.signal, withCredentials: config.withCredentials });
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data: { accepted: true }, msg: "ok" } };
    };
    try {
        const controller = new AbortController();
        await listWorkshopPublications({ q: "", tag: "", source: "colleague", sort: "copies", page: 2, pageSize: 20 }, controller.signal);
        await getWorkshopSnapshot("version/one", controller.signal);
        await copyWorkshopPublication("publication/one", { requestId: "same_request", title: "作品（副本）" });
        await approveWorkshopVersion("version/one", { snapshotSha256: "exact-viewed-snapshot", official: true });
        await submitWorkshop({ projectId: "root-canvas", title: "作品", summary: "简介", tags: [], allowDownload: false, rightsConfirmed: true });
        expect(requests.map((request) => request.url)).toEqual(["/community/publications", "/community/versions/version%2Fone/snapshot", "/community/publications/publication%2Fone/copies", "/platform/admin/workshop/versions/version%2Fone/approve", "/community/submissions"]);
        expect(requests[0].params).toEqual({ source: "colleague", sort: "copies", page: 2, pageSize: 20 });
        expect(requests[0].signal).toBe(controller.signal);
        expect(requests[1].signal).toBe(controller.signal);
        expect(requests[2].body).toEqual({ requestId: "same_request", title: "作品（副本）" });
        expect(requests[2].timeout).toBe(0);
        expect(requests[3].body).toEqual({ snapshotSha256: "exact-viewed-snapshot", official: true });
        expect(requests[4].body).toEqual({ projectId: "root-canvas", title: "作品", summary: "简介", tags: [], allowDownload: false, rightsConfirmed: true });
        expect(requests.every((request) => request.withCredentials)).toBe(true);
    } finally { apiClient.defaults.adapter = original; }
});

test("workshop failures preserve the machine-readable reason regardless of server prose", async () => {
    const original = apiClient.defaults.adapter;
    apiClient.defaults.adapter = async () => { throw { isAxiosError: true, response: { status: 409, data: { code: 409, reason: "version_changed", msg: "unrelated opaque message" } } }; };
    try {
        let failure: unknown;
        try { await approveWorkshopVersion("version", { snapshotSha256: "old", official: false }); }
        catch (error) { failure = error; }
        expect(failure).toBeInstanceOf(ApiError);
        expect(failure instanceof ApiError && failure.reason).toBe("version_changed");
        expect(platformErrorMessage(failure)).toBe("作者已更新或撤回稿件，请刷新后重新审核");
    } finally { apiClient.defaults.adapter = original; }
    for (const reason of ["rights_required", "title_invalid", "tags_invalid", "cover_invalid", "preflight_blocked", "publication_unlisted", "publication_not_listed", "copy_in_progress", "origin_not_found", "project_not_found"]) {
        expect(platformErrorMessage(new ApiError("opaque", { reason }))).not.toBe("操作失败，请稍后重试");
    }
});
