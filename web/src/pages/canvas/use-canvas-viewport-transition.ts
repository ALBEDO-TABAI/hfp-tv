import { useCallback, useContext, useEffect, useRef, type MutableRefObject } from "react";
import { cubicBezier } from "motion";
import { MotionConfigContext } from "motion/react";

import { interpolateViewport, isUsableViewport, viewportAtScale, viewportTransitionDuration, type CanvasViewportSize } from "@/lib/canvas/canvas-viewport";
import { motionDuration, motionEase, prefersReducedMotionNow } from "@/lib/motion-tokens";
import type { Position, ViewportTransform } from "@/types/canvas";

const easeCamera = cubicBezier(...motionEase.inOut);
const easeZoom = cubicBezier(...motionEase.out);
const ZOOM_STEP = 1.2;

type ViewportTransition = {
    from: ViewportTransform;
    target: ViewportTransform;
    focus: Position;
    startedAt: number;
    duration: number;
    zoomStep: boolean;
    frameId: number;
};

export function useCanvasViewportTransition(
    viewportRef: MutableRefObject<ViewportTransform>,
    size: CanvasViewportSize,
    onPreview: (viewport: ViewportTransform) => void,
    onCommit: (viewport: ViewportTransform) => void,
) {
    const activeRef = useRef<ViewportTransition | null>(null);
    // 根部 MotionConfig 明确要求减弱（流畅模式时是 always）或系统此刻开着「减少动态效果」，就直接落到终点。
    // 系统偏好每次实时读（prefersReducedMotionNow），运行中打开或关掉都立即生效，不用挂载时的旧值。
    const configReduced = useContext(MotionConfigContext).reducedMotion === "always";

    const cancel = useCallback(() => {
        const active = activeRef.current;
        if (!active) return false;
        cancelAnimationFrame(active.frameId);
        activeRef.current = null;
        return true;
    }, []);

    useEffect(() => () => { cancel(); }, [cancel]);

    const step = useCallback((now: number): void => {
        const active = activeRef.current;
        if (!active) return;
        const elapsed = now - active.startedAt;
        // 运行中打开「减少动态效果」或流畅模式：直接落到终点。
        if (elapsed >= active.duration || prefersReducedMotionNow()) {
            activeRef.current = null;
            onPreview(active.target);
            onCommit(active.target);
            return;
        }
        const ease = active.zoomStep ? easeZoom : easeCamera;
        const progress = ease(Math.max(0, elapsed / active.duration));
        active.frameId = requestAnimationFrame(step);
        onPreview(interpolateViewport(active.from, active.target, progress, active.focus));
    }, [onCommit, onPreview]);

    const transitionTo = useCallback(
        (target: ViewportTransform, zoomStep = false) => {
            const from = viewportRef.current;
            if (!isUsableViewport(target)) return;
            const focus = { x: size.width / 2, y: size.height / 2 };
            const duration = zoomStep ? motionDuration.state : isUsableViewport(from) ? viewportTransitionDuration(from, target, size, focus) : Number.NaN;
            // 起点不合法（例如窄视口算出的负缩放）或时长算不出来时不做插值，直接落到终点，避免逐帧都结束不了。
            if (configReduced || prefersReducedMotionNow() || !isUsableViewport(from) || !Number.isFinite(duration) || (from.x === target.x && from.y === target.y && from.k === target.k)) {
                cancel();
                onPreview(target);
                onCommit(target);
                return;
            }

            // 继续用已排定的帧，从屏幕当前画面朝新目标移动。
            activeRef.current = {
                from, target, focus, zoomStep,
                startedAt: performance.now(),
                duration: duration * 1000,
                frameId: activeRef.current?.frameId ?? requestAnimationFrame(step),
            };
        },
        [cancel, configReduced, onCommit, onPreview, size, step, viewportRef],
    );

    const stepViewportZoom = useCallback((direction: "in" | "out") => {
        const active = activeRef.current;
        const base = active?.zoomStep ? active.target : viewportRef.current;
        const scale = direction === "in" ? base.k * ZOOM_STEP : base.k / ZOOM_STEP;
        const target = viewportAtScale(base, size, scale);
        if (target.k !== base.k) transitionTo(target, true);
    }, [size, transitionTo, viewportRef]);

    const isViewportTransitionActive = useCallback(() => activeRef.current !== null, []);

    return { cancelViewportTransition: cancel, isViewportTransitionActive, transitionViewportTo: transitionTo, stepViewportZoom };
}
