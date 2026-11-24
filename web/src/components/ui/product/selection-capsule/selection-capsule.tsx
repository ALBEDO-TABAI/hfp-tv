import { useEffect, useLayoutEffect, useRef, type ReactNode } from "react";
import { AnimatePresence, motion, useReducedMotionConfig } from "motion/react";

import { motionEase, motionTransition } from "@/lib/motion-tokens";
import "./selection-capsule.css";

function keepClearOfCornerOverlays(dock: HTMLDivElement) {
    const stage = dock.parentElement;
    const workspace = stage?.closest(".app-user-workspace");
    if (!stage || !workspace) return;

    let overlays: HTMLElement[] = [];
    const positionDock = () => {
        const dockRight = dock.getBoundingClientRect().right;
        const stageBottom = stage.getBoundingClientRect().bottom;
        let clearance = 0;
        for (const overlay of overlays) {
            // Fixed viewport offsets exclude the guide's entrance transform.
            if (dockRight > overlay.offsetLeft) clearance = Math.max(clearance, stageBottom - overlay.offsetTop);
        }
        dock.style.setProperty("--selection-capsule-clearance", `${clearance}px`);
    };
    const resize = new ResizeObserver(positionDock);
    const watchOverlays = () => {
        for (const overlay of overlays) resize.unobserve(overlay);
        overlays = Array.from(workspace.querySelectorAll<HTMLElement>(
            ':scope > .workspace-operation-tray, :scope > aside:has([aria-label="关闭模型配置引导"])',
        ));
        for (const overlay of overlays) resize.observe(overlay);
        positionDock();
    };

    resize.observe(stage);
    resize.observe(dock);
    const mutation = new MutationObserver(watchOverlays);
    mutation.observe(workspace, { childList: true });
    watchOverlays();
    return () => {
        mutation.disconnect();
        resize.disconnect();
    };
}

function focusPageControl(stage: HTMLElement | null) {
    const page = stage?.querySelector("main");
    const control = page?.querySelector<HTMLElement>('input[type="checkbox"]:checked')
        ?? page?.querySelector<HTMLElement>(".app-page-header button:not(:disabled)");
    control?.focus({ preventScroll: true });
}

export function SelectionCapsule({ open, label, summary, onClear, children }: {
    open: boolean;
    label: string;
    summary: string;
    onClear?: () => void;
    children: ReactNode;
}) {
    const reduceMotion = useReducedMotionConfig();
    const dockRef = useRef<HTMLDivElement>(null);

    useLayoutEffect(() => {
        const dock = dockRef.current;
        if (!dock) return;
        if (!open && dock.contains(document.activeElement)) focusPageControl(dock.parentElement);
        dock.inert = !open;
        if (open) return keepClearOfCornerOverlays(dock);
    }, [open]);

    useEffect(() => {
        const dock = dockRef.current;
        const stage = dock?.parentElement;
        if (!open || !onClear || !dock || !stage) return;

        const handleEscape = (event: KeyboardEvent) => {
            if (event.key !== "Escape" || event.defaultPrevented || event.isComposing) return;
            const target = event.target;
            if (!(target instanceof Element) || (target !== document.body && !stage.contains(target))) return;
            if (target.closest('input:not([type="checkbox"]), textarea, select, [contenteditable="true"], [role="dialog"], [role="alertdialog"]')
                || stage.querySelector(".ant-dropdown-open")) return;

            event.preventDefault();
            if (target === document.body || dock.contains(target)) focusPageControl(stage);
            onClear();
        };
        window.addEventListener("keydown", handleEscape);
        return () => window.removeEventListener("keydown", handleEscape);
    }, [onClear, open]);

    return (
        <div ref={dockRef} className="selection-capsule-dock">
            <AnimatePresence>
                {open ? (
                    <motion.div
                        className="selection-capsule"
                        role="toolbar"
                        aria-label={label}
                        initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }}
                        animate={{ opacity: 1, y: 0, transition: motionTransition.panelEnter }}
                        exit={{ opacity: 0, y: reduceMotion ? 0 : 4, transition: { ...motionTransition.panelExit, ease: motionEase.in } }}
                    >
                        <span className="selection-capsule-summary" role="status">{summary}</span>
                        <div className="selection-capsule-actions">{children}</div>
                    </motion.div>
                ) : null}
            </AnimatePresence>
        </div>
    );
}

/**
 * 放在滚动内容的最末尾：有选中项时占出胶囊的高度，滚到底时分页和最后一行不会被浮起的胶囊盖住。
 */
export function SelectionCapsuleSpacer({ active }: { active: boolean }) {
    return active ? <div className="selection-capsule-spacer" aria-hidden="true" /> : null;
}
