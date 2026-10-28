import { useEffect, useRef, useState, type ReactNode } from "react";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { WorkspaceErrorState } from "@/components/layout/workspace-state";
import { getPlatformIdentity, type PlatformIdentity } from "@/services/api/platform";
import { leaveTeamSession } from "@/services/team-session";
import { teamWorkspaceScope } from "@/lib/team-auth";
import { usePlatformIdentityStore } from "@/stores/use-platform-identity-store";

export function TeamSessionGate({ children }: { children: (identity: PlatformIdentity) => ReactNode }) {
    const [identity, setIdentity] = useState<PlatformIdentity | null>(null);
    const [failed, setFailed] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const current = useRef<PlatformIdentity | null>(null);

    useEffect(() => {
        const controller = new AbortController();
        let checking = false;
        const check = async () => {
            if (checking || controller.signal.aborted) return;
            checking = true;
            try {
                const next = await getPlatformIdentity(controller.signal);
                if (controller.signal.aborted) return;
                const scope = teamWorkspaceScope(next.user.id, next.workspace_id);
                if (current.current && teamWorkspaceScope(current.current.user.id, current.current.workspace_id) !== scope) {
                    leaveTeamSession();
                    return;
                }
                current.current = next;
                usePlatformIdentityStore.getState().setIdentity(next);
                setIdentity(next);
                setFailed(false);
            } catch {
                if (!controller.signal.aborted && !current.current) setFailed(true);
            } finally {
                checking = false;
            }
        };
        const onFocus = () => { if (document.visibilityState === "visible") void check(); };
        const channel = new BroadcastChannel("hfp-team-session");
        channel.onmessage = () => leaveTeamSession();
        void check();
        window.addEventListener("focus", onFocus);
        document.addEventListener("visibilitychange", onFocus);
        return () => {
            controller.abort();
            channel.close();
            window.removeEventListener("focus", onFocus);
            document.removeEventListener("visibilitychange", onFocus);
        };
    }, [attempt]);

    if (failed) return <WorkspaceErrorState title="工作区暂时无法加载" description="无法确认登录状态，请稍后重试。" onRetry={() => { setFailed(false); setAttempt((value) => value + 1); }} />;
    return identity ? children(identity) : <FullScreenLoader label="正在确认登录状态" detail="连接你的工作区" />;
}
