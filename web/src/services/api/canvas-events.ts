import { apiBaseURL } from "./request";
import { teamSessionSignal } from "@/services/team-session";

// EventSource 不暴露 401，team 使用 fetch 以复用登录失效处理和会话取消。
export function subscribeTeamCanvasUpdates(projectId: string, onUpdate: () => void) {
    const controller = new AbortController();
    const signal = teamSessionSignal(controller.signal)!;
    let retry: ReturnType<typeof setTimeout> | undefined;
    const connect = async () => {
        try {
            const response = await fetch(`${apiBaseURL}/canvas-projects/${encodeURIComponent(projectId)}/events`, {
                headers: { Accept: "text/event-stream" }, credentials: "include", signal,
            });
            if (!response.ok || !response.body) throw new Error("画布事件流连接失败");
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = "";
            try {
                while (!signal.aborted) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    signal.throwIfAborted();
                    buffer += decoder.decode(value, { stream: true });
                    const frames = buffer.split(/\r?\n\r?\n/u);
                    buffer = frames.pop() || "";
                    for (const frame of frames) if (/^event:\s*canvas\.updated\s*$/mu.test(frame)) onUpdate();
                }
            } finally {
                await reader.cancel().catch(() => undefined);
                reader.releaseLock();
            }
        } catch {
            // 暂时断网时重连；会话取消后不再重试。
        }
        if (!signal.aborted) retry = setTimeout(() => void connect(), 1000);
    };
    void connect();
    return () => { controller.abort(); clearTimeout(retry); };
}
