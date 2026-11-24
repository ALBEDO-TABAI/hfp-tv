let region: HTMLElement | null = null;
let pending = 0;
let queued: string[] = [];

/**
 * 给读屏软件的礼貌播报：画面上已经用动效交代了、但读屏用户看不到的状态变化（例如生成完成）。
 * 全页只放一个 live region；先清空再写入，同一句话连续出现也会再读一次。
 * 短时间内的多条（例如几个节点同时完成、先失败后成功）合并成一句一起读，不互相覆盖。
 */
export function announce(message: string) {
    if (typeof document === "undefined" || !message) return;
    if (!region || !region.isConnected) {
        region = document.createElement("div");
        region.setAttribute("role", "status");
        region.setAttribute("aria-live", "polite");
        Object.assign(region.style, { position: "fixed", width: "1px", height: "1px", overflow: "hidden", clip: "rect(0 0 0 0)", clipPath: "inset(50%)", whiteSpace: "nowrap", margin: "-1px", padding: "0", border: "0" });
        document.body.appendChild(region);
    }
    if (!queued.includes(message)) queued.push(message);
    region.textContent = "";
    window.clearTimeout(pending);
    pending = window.setTimeout(() => {
        if (region) region.textContent = queued.join("；");
        queued = [];
    }, 60);
}
