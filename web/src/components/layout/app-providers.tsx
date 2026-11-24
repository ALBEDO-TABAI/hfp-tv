import type { ReactNode } from "react";
import { lazy, Suspense, useLayoutEffect } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { App, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { MotionConfig } from "motion/react";

import { WorkspaceBootstrapHydrator } from "@/components/workspace/workspace-bootstrap-hydrator";
import { useWorkspaceButtonFeedback } from "@/hooks/use-workspace-button-feedback";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { appPathname } from "@/lib/app-routing";
import { getAntThemeConfig } from "@/lib/app-theme";
import { motionSpring } from "@/lib/motion-tokens";
import { applySkinTheme } from "@/lib/skin-themes";
import { appQueryClient } from "@/lib/query-client";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";
import { applyAppearanceMetadata, useAppearanceStore } from "@/stores/use-appearance-store";
import { useUserStore } from "@/stores/use-user-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { usePlatformSession } from "@/stores/use-platform-session";
import { isPlatformPublicPath } from "@/lib/platform-navigation";

const ClientRootInit = lazy(() => import("@/components/layout/client-root-init").then((module) => ({ default: module.ClientRootInit })));

function ClientRootBoundary({ children }: { children: ReactNode }) {
    const authenticated = useUserStore((state) => Boolean(state.user));
    if (!authenticated) return children;
    return <Suspense fallback={<FullScreenLoader label="正在打开工作区" detail="连接本地能力与模型配置" />}><ClientRootInit>{children}</ClientRootInit></Suspense>;
}

export function AppProviders({ children }: { children: ReactNode }) {
    const theme = useActiveTheme();
    const dark = theme === "dark";
    const appearance = useAppearanceStore((state) => state.appearance);
    const lite = useThemeStore((state) => state.effects === "lite");
    const platform = usePlatformSession();
    useWorkspaceButtonFeedback();

    useLayoutEffect(() => {
        document.documentElement.classList.toggle("dark", dark);
        document.documentElement.style.colorScheme = theme;
        // 加载画面底色跟着主题走，和 index.html 启动脚本里的取值一致（工作区页面底色 --stage）。
        document.documentElement.style.setProperty("--boot-background", dark ? "#0b0b0d" : "#ebebee");
        applySkinTheme(appearance.activeSkin, theme);
        applyAppearanceMetadata(appearance);
    }, [appearance, dark, theme]);

    useLayoutEffect(() => {
        if (lite) document.documentElement.dataset.effects = "lite";
        else delete document.documentElement.dataset.effects;
    }, [lite]);

    // DEV 复现台必须是同源本地确定性场景：WorkspaceBootstrapHydrator 会打 /api/workspace/bootstrap，
    // ClientRootInit 会打 /api/model-catalog，没有后端时产生真实 502，与导演台无关却会污染判据。
    // 只精确匹配该路径；生产构建中 import.meta.env.DEV 为 false，本分支被摇树删除。
    const isolateDevRepro = import.meta.env.DEV && typeof window !== "undefined" && ["/dev/director-repro", "/dev/assistant-panel"].includes(appPathname());

    return (
        // 系统「减少动态效果」与「流畅模式」在这里统一接管 motion 动画：只保留透明度和颜色变化。
        // AntD 的点击水波纹不是本项目的按压语言，按压反馈由各控件的 :active 状态负责。
        <MotionConfig reducedMotion={lite ? "always" : "user"} transition={motionSpring.smooth}>
        <ConfigProvider locale={zhCN} theme={getAntThemeConfig(dark, appearance.activeSkin)} modal={{ centered: true }} wave={{ disabled: true }}>
            <App message={{ duration: 3, maxCount: 3 }} notification={{ duration: 4.5, maxCount: 3, placement: "topRight" }}>
                <QueryClientProvider client={appQueryClient}>
                    {isolateDevRepro || isPlatformPublicPath(appPathname()) || (platform.mode === "hosted" && !platform.me) ? (
                        children
                    ) : (
                        <WorkspaceBootstrapHydrator>
                            <ClientRootBoundary>{children}</ClientRootBoundary>
                        </WorkspaceBootstrapHydrator>
                    )}
                </QueryClientProvider>
            </App>
        </ConfigProvider>
        </MotionConfig>
    );
}
