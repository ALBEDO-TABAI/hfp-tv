import { bootstrapAppearance } from "@/services/appearance-bootstrap";
import { bootstrapDesktopRuntime } from "@/services/desktop-runtime";
import { probePlatformSession } from "@/services/api/platform";
import { setUnauthenticatedHandler } from "@/services/api/request";
import { usePlatformSession } from "@/stores/use-platform-session";
import { setActiveUserScope } from "@/lib/user-scope";
import { isPlatformPublicPath, platformLoginPath } from "@/lib/platform-navigation";
import { appHref, appPathname } from "@/lib/app-routing";
import { resetAgentUiSession } from "@/services/api/agent-assistant";
import { appQueryClient } from "@/lib/query-client";
import { useUserStore } from "@/stores/use-user-store";

export async function startApplication() {
    await bootstrapDesktopRuntime();
    const session = await probePlatformSession();
    usePlatformSession.getState().setSession(session);
    if (session.mode === "local" && isPlatformPublicPath(appPathname())) {
        window.location.replace(appHref("/"));
        return;
    }
    if (session.mode === "hosted") {
        setActiveUserScope(session.me ? `${session.me.user.id}:${session.me.workspace.id}` : null);
        let redirecting = false;
        setUnauthenticatedHandler(() => {
            if (redirecting) return;
            usePlatformSession.getState().setMe(null);
            resetAgentUiSession();
            appQueryClient.clear();
            useUserStore.getState().clearSession();
            if (window.location.pathname === "/login") return;
            redirecting = true;
            window.location.assign(platformLoginPath(window.location.pathname, window.location.search));
        });
    }
    if (!isPlatformPublicPath(appPathname()) && (session.mode === "local" || session.me)) {
        const { useCanvasStore } = await import("@/stores/canvas/use-canvas-store");
        const { hydrateLocalCanvasProjectsFromBackend } = await import("@/services/local-workspace-repository");
        // 浏览器缓存晚于后端读入完成时，会用旧连接数组盖掉刚合并的画布。
        await useCanvasStore.persist.rehydrate();
        await hydrateLocalCanvasProjectsFromBackend();
    }
    await bootstrapAppearance().catch(() => console.warn("外观初始化未完成，继续打开工作区"));
    await import("./application");
}
