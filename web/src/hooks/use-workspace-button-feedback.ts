import { useEffect } from "react";

import { motionDuration, prefersReducedMotionNow } from "@/lib/motion-tokens";

const PRESSABLE = 'button, [role="button"], a[href]';
/** 画布世界里的节点、连接点和手柄跟着指针走，不做按压缩放；.press 自带按压规则。 */
const EXCLUDED = "[data-canvas-world-layer], .press, .hfp-press, [data-press-feedback='none']";

/** 按下时整体内收约 2px：小按钮约 0.97，大卡片只收一点，不会显得整块在缩。 */
function pressedScale(element: HTMLElement) {
    const size = Math.max(element.offsetWidth, element.offsetHeight, 1);
    return 1 - Math.min(0.03, 4 / size);
}

/**
 * 按压反馈：按下立即内收，松开按弹簧回到原位；中途再次按下从当前大小接着走。
 * 只表达「按到了」，不表达请求成功，也不接管业务回调或画布事件。用独立的 scale 属性，
 * 不覆盖组件自己的 transform 和 transition。
 */
export function useWorkspaceButtonFeedback() {
    useEffect(() => {
        // origin 是指针在元素里的位置：以它为中心内收，指针下面那一点不动，原地松手一定还落在按钮里。
        let pressed: { element: HTMLElement; animation: Animation; origin: string } | null = null;
        const springEasing = () => getComputedStyle(document.documentElement).getPropertyValue("--ease-spring").trim() || "cubic-bezier(0.3, 1.25, 0.5, 1)";
        const currentScale = (element: HTMLElement) => {
            const value = getComputedStyle(element).scale;
            return value && value !== "none" ? value.split(" ")[0] : "1";
        };

        const release = () => {
            if (!pressed) return;
            const { element, animation, origin } = pressed;
            pressed = null;
            const from = currentScale(element);
            animation.cancel();
            if (!element.isConnected || typeof element.animate !== "function") return;
            element.animate([{ scale: from, transformOrigin: origin }, { scale: "1", transformOrigin: origin }], { duration: motionDuration.state * 1000, easing: springEasing() });
        };

        const press = (event: PointerEvent) => {
            if (event.button !== 0 || prefersReducedMotionNow() || !(event.target instanceof Element)) return;
            const element = event.target.closest<HTMLElement>(PRESSABLE);
            if (!element || typeof element.animate !== "function" || element.closest(EXCLUDED)) return;
            if (element.matches(':disabled, [aria-disabled="true"], .ant-btn-loading')) return;
            // 正文里的文字链接不缩放，只有做成按钮、卡片或导航项的链接才有按压反馈。
            if (element.tagName === "A" && getComputedStyle(element).display === "inline") return;
            release();
            const from = currentScale(element);
            const rect = element.getBoundingClientRect();
            const origin = `${event.clientX - rect.left}px ${event.clientY - rect.top}px`;
            const animation = element.animate([{ scale: from, transformOrigin: origin }, { scale: String(pressedScale(element)), transformOrigin: origin }], {
                duration: motionDuration.press * 1000,
                easing: "cubic-bezier(0.22, 1, 0.36, 1)",
                fill: "forwards",
            });
            pressed = { element, animation, origin };
        };

        document.addEventListener("pointerdown", press, true);
        window.addEventListener("pointerup", release, true);
        window.addEventListener("pointercancel", release, true);
        window.addEventListener("blur", release);
        return () => {
            document.removeEventListener("pointerdown", press, true);
            window.removeEventListener("pointerup", release, true);
            window.removeEventListener("pointercancel", release, true);
            window.removeEventListener("blur", release);
            pressed?.animation.cancel();
            pressed = null;
        };
    }, []);
}
