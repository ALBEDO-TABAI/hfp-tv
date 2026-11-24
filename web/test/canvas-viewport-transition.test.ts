import { expect, spyOn, test } from "bun:test";
import { MotionConfig } from "motion/react";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { interpolateViewport, viewportTransitionDuration } from "@/lib/canvas/canvas-viewport";
import { motionDuration } from "@/lib/motion-tokens";
import { useCanvasViewportController } from "@/pages/canvas/use-canvas-viewport-controller";
import type { ViewportTransform } from "@/types/canvas";

const size = { width: 800, height: 600 };
const center = { x: 400, y: 300 };
const zoomDuration = motionDuration.state * 1000;

test("puts half of a zoom at the geometric midpoint of the two scales", () => {
    const from = { x: 0, y: 0, k: 0.2 };
    const to = { x: 0, y: 0, k: 3.2 };
    expect(interpolateViewport(from, to, 0.5, center).k).toBeCloseTo(0.8, 8);
});

test("keeps a fixed world point within half a pixel of the screen focus throughout zooming", () => {
    const focus = { x: 563, y: 193 };
    const world = { x: 617.2, y: -916.7 };
    const from = { x: 408.7, y: 422.175, k: 0.25 };
    const to = { x: -3140.2, y: 5693.2, k: 6 };
    for (let step = 0; step <= 100; step++) {
        const frame = interpolateViewport(from, to, step / 100, focus);
        const drift = Math.hypot(frame.x + world.x * frame.k - focus.x, frame.y + world.y * frame.k - focus.y);
        expect(drift).toBeLessThan(0.5);
    }
});

test("pans the world point at the screen center linearly while zooming", () => {
    const from = { x: 390, y: 325, k: 0.5 };
    const to = { x: 0, y: 100, k: 2 };
    expect(interpolateViewport(from, to, 0.5, center)).toEqual({ x: 290, y: 275, k: 1 });
});

test("camera duration accounts for both visible travel and the zoom ratio", () => {
    const from = { x: 150, y: -80, k: 1 };
    const zoom = { x: -100, y: -460, k: 2 };
    expect(viewportTransitionDuration(from, from, size, center)).toBe(motionDuration.panel);
    expect(viewportTransitionDuration(from, zoom, size, center)).toBe(motionDuration.stage);
    expect(viewportTransitionDuration(from, { ...zoom, x: zoom.x - 1400 }, size, center)).toBeGreaterThan(motionDuration.stage);
    expect(viewportTransitionDuration(from, { ...zoom, x: zoom.x - 10000 }, size, center)).toBeCloseTo(0.4, 8);
});

function withAnimationFrames(run: (advance: (ms: number) => void, dataset: DOMStringMap) => void) {
    let now = 0;
    let frameId = 0;
    const frames = new Map<number, FrameRequestCallback>();
    const clock = spyOn(performance, "now").mockImplementation(() => now);
    const dataset: DOMStringMap = {};
    const browser = {
        document: { documentElement: { dataset } },
        requestAnimationFrame: (callback: FrameRequestCallback) => {
            frames.set(++frameId, callback);
            return frameId;
        },
        cancelAnimationFrame: (id: number) => frames.delete(id),
    };
    const original = Object.keys(browser).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)] as const);
    for (const [key, value] of Object.entries(browser)) Object.defineProperty(globalThis, key, { configurable: true, value });
    try {
        run((ms) => {
            now += ms;
            const pending = [...frames.values()];
            frames.clear();
            pending.forEach((callback) => callback(now));
        }, dataset);
    } finally {
        clock.mockRestore();
        for (const [key, descriptor] of original) {
            if (descriptor) Object.defineProperty(globalThis, key, descriptor);
            else Reflect.deleteProperty(globalThis, key);
        }
    }
}

function renderController(scale: number, reducedMotion: "user" | "always" = "user") {
    const viewport = { x: 150, y: -80, k: scale };
    const viewportRef = { current: viewport };
    let committed = viewport;
    let controller!: ReturnType<typeof useCanvasViewportController>;
    const noop = () => {};
    function Harness() {
        controller = useCanvasViewportController({
            size, viewport, viewportRef,
            containerRef: { current: null },
            nodesRef: { current: [] },
            selectedNodeIdsRef: { current: new Set() },
            setViewport: (update) => { committed = typeof update === "function" ? update(committed) : update; },
            setVirtualizationViewport: noop, setSelectedNodeIds: noop, setSelectedConnectionId: noop,
            setContextMenu: noop, setDialogNodeId: noop, setToolbarNodeId: noop,
        });
        return null;
    }
    renderToStaticMarkup(createElement(MotionConfig, { reducedMotion }, createElement(Harness)));
    return { ...controller, viewportRef, committed: (): ViewportTransform => committed };
}

test("rapid zoom clicks build on the pending target and continue from the visible frame", () => {
    withAnimationFrames((advance) => {
        const canvas = renderController(1);
        canvas.zoomCanvasIn();
        canvas.zoomCanvasIn();
        expect(canvas.viewportRef.current.k).toBe(1);
        advance(60);
        const visible = canvas.viewportRef.current;
        expect(visible.k).toBeGreaterThan(1);
        expect(visible.k).toBeLessThan(1.44);
        canvas.zoomCanvasIn();
        expect(canvas.viewportRef.current).toEqual(visible);
        advance(zoomDuration - 1);
        expect(canvas.committed().k).toBe(1);
        advance(1);
        expect(canvas.committed().k).toBeCloseTo(1.728, 8);
    });
});

test("multiplicative steps clamp at both zoom limits", () => {
    withAnimationFrames((advance) => {
        const upper = renderController(7);
        const lower = renderController(0.055);
        upper.zoomCanvasIn();
        lower.zoomCanvasOut();
        advance(zoomDuration / 2);
        upper.zoomCanvasIn();
        lower.zoomCanvasOut();
        advance(zoomDuration / 2);
        expect(upper.committed().k).toBe(8);
        expect(lower.committed().k).toBe(0.05);
    });
});

test("a wheel gesture interrupts a zoom, but its delayed commit leaves the next zoom running", () => {
    withAnimationFrames((advance) => {
        const canvas = renderController(1);
        canvas.zoomCanvasIn();
        const wheelPreview = { x: 120, y: -80, k: 1 };
        canvas.handleViewportPreviewChange(wheelPreview);
        advance(zoomDuration);
        expect(canvas.viewportRef.current).toEqual(wheelPreview);

        canvas.zoomCanvasIn();
        advance(motionDuration.instant * 1000);
        canvas.handleViewportChange(canvas.viewportRef.current);
        advance(zoomDuration);
        expect(canvas.committed().k).toBeCloseTo(1.2, 8);
    });
});

test("camera commands commit immediately in reduced motion and lite modes", () => {
    withAnimationFrames((_advance, dataset) => {
        const reduced = renderController(2, "always");
        reduced.zoomToActualSize();
        expect(reduced.committed().k).toBe(1);

        dataset.effects = "lite";
        const lite = renderController(2);
        lite.zoomCanvasIn();
        expect(lite.committed().k).toBe(2.4);
    });
});
