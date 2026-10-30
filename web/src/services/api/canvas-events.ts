import { apiBaseURL, handleUnauthenticated } from "./request";

// 团队服务器走 HTTPS，且 EventSource 拿不到 401：改用 fetch 读取画布事件流，
// 会话失效时交给统一的登录失效处理，网络中断时 1 秒后重连。
export function subscribeHostedCanvasUpdates(projectId: string, onUpdate: () => void) {
    const controller = new AbortController();
    let retry: ReturnType<typeof setTimeout> | undefined;
    const connect = async () => {
        try {
            const response = await fetch(`${apiBaseURL}/canvas-projects/${encodeURIComponent(projectId)}/events`, {
                headers: { Accept: "text/event-stream" }, credentials: "same-origin", signal: controller.signal,
            });
            if (response.status === 401) {
                const payload = await response.json().catch(() => null) as { reason?: string } | null;
                handleUnauthenticated(401, payload?.reason);
                return;
            }
            if (!response.ok || !response.body) throw new Error("画布事件流连接失败");
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = "";
            try {
                while (!controller.signal.aborted) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    buffer += decoder.decode(value, { stream: true });
                    const frames = buffer.split(/\r?\n\r?\n/u);
                    buffer = frames.pop() || "";
                    for (const frame of frames) if (/^event:\s*canvas\.updated\s*$/mu.test(frame)) onUpdate();
                }
            } finally {
                await reader.cancel().catch(() => undefined);
            }
        } catch {
            // 断网或服务重启时重连；页面卸载后不再重试。
        }
        if (!controller.signal.aborted) retry = setTimeout(() => void connect(), 1000);
    };
    void connect();
    return () => { controller.abort(); clearTimeout(retry); };
}
