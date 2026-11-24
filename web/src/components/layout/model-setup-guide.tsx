import { AnimatePresence, motion, useIsPresent, useReducedMotionConfig } from "motion/react";
import { ArrowRight, Sparkles, X } from "lucide-react";
import { useState } from "react";

import { motionEase, motionTransition } from "@/lib/motion-tokens";
import { navigateToSettings } from "@/lib/settings-navigation";
import { useConfigStore } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { isLocalWorkspaceMode } from "@/services/workspace-mode";

export function ModelSetupGuide({ hidden = false }: { hidden?: boolean }) {
    const [dismissed, setDismissed] = useState(false);
    const registrationGuide = typeof window !== "undefined" && window.sessionStorage.getItem("infinite-canvas:model-setup-guide") === "1";
    const hydrated = useUserStore((state) => state.hydrated);
    const user = useUserStore((state) => state.user);
    const storageMode = useUserStore((state) => state.storageMode);
    const localMode = isLocalWorkspaceMode() || storageMode === "local" || user?.username === "local";
    const models = useConfigStore((state) => state.config.models);
    // BeefTV 本地工作区不显示阻塞式模型引导浮层；模型配置入口已经固定在侧边栏。
    const visible = !hidden && !dismissed && hydrated && user && !localMode && user.role !== "admin" && (registrationGuide || models.length === 0);

    const close = () => {
        window.sessionStorage.removeItem("infinite-canvas:model-setup-guide");
        setDismissed(true);
    };

    return <AnimatePresence>{visible ? <ModelSetupGuideCard onClose={close} /> : null}</AnimatePresence>;
}

function ModelSetupGuideCard({ onClose }: { onClose: () => void }) {
    const isPresent = useIsPresent();
    const reducedMotion = useReducedMotionConfig();
    const openModels = () => {
        onClose();
        navigateToSettings({ section: "models" });
    };

    return (
        <motion.aside
            initial={{ opacity: 0, y: 14, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1, transition: motionTransition.panelEnter }}
            exit={{
                opacity: 0,
                // 减弱模式会直接到达位移终点，离场时让卡片留在原位。
                y: reducedMotion ? 0 : 8,
                transition: reducedMotion ? motionTransition.panelExit : { ...motionTransition.panelExit, ease: motionEase.in },
            }}
            className="fixed bottom-5 right-5 z-[var(--z-toast)] w-[min(360px,calc(100vw-32px))] overflow-hidden rounded-2xl border border-border/70 bg-background/92 shadow-(--shadow-3) backdrop-blur-2xl"
            inert={!isPresent}
        >
            <div className="flex items-start gap-3 p-4">
                <span className="grid size-9 shrink-0 place-items-center rounded-xl border border-amber-400/25 bg-amber-400/10 text-amber-600 dark:text-amber-300"><Sparkles className="size-4" /></span>
                <button type="button" className="min-w-0 flex-1 text-left" onClick={openModels}>
                    <span className="block text-sm font-semibold">先选择创作模型</span>
                    <span className="mt-1 block text-xs leading-5 text-foreground/55">配置生图、视频和文本的默认模型，价格会在选择时显示。</span>
                    <span className="mt-3 inline-flex items-center gap-1.5 text-xs font-semibold text-foreground">打开模型选择 <ArrowRight className="size-3.5" /></span>
                </button>
                <button type="button" className="grid size-7 shrink-0 place-items-center rounded-full text-foreground/40 hover:bg-muted hover:text-foreground" onClick={onClose} aria-label="关闭模型配置引导"><X className="size-3.5" /></button>
            </div>
        </motion.aside>
    );
}
