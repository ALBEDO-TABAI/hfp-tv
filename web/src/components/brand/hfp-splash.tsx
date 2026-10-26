import { useEffect, useRef, useState } from "react";

import { useAssetStore } from "@/stores/use-asset-store";
import { useCanvasStore } from "@/stores/canvas/use-canvas-store";
import { useConfigStore } from "@/stores/use-config-store";
import { beefTVCapabilityItems } from "@/pages/home/home-data";

export type SplashStage = "start" | "shelf" | "type" | "covers";
export type SplashStat = [number | null, string, string];

const STAGE: Record<SplashStage, number> = { start: 0.18, shelf: 0.48, type: 0.76, covers: 0.94 };
const FILL_MS = 1650;
const HOLD_MS = 220;
const FADE_MS = 360;
const ease = (t: number) => (t >= 1 ? 1 : 0.5 - Math.cos(Math.PI * Math.max(0, t)) / 2);

function Count({ n }: { n: number | null }) {
    const [v, setV] = useState(n ?? 0);
    const from = useRef(n ?? 0);
    useEffect(() => {
        if (n == null) return;
        const a = from.current;
        const t0 = performance.now();
        let raf = 0;
        const tick = (now: number) => {
            const k = Math.min(1, (now - t0) / 800);
            const e = 1 - Math.pow(1 - k, 3);
            setV(Math.round(a + (n - a) * e));
            if (k < 1) raf = requestAnimationFrame(tick);
            else from.current = n;
        };
        raf = requestAnimationFrame(tick);
        return () => cancelAnimationFrame(raf);
    }, [n]);
    return <>{n == null ? "—" : v}</>;
}

/**
 * hfp-tv Poster-Sheet Splash Screen — adapted directly from Ready Library (藏书阁) `src/ui/Splash.tsx`.
 * Uses the 2200×1396 proportional typographic sheet with Instrument Serif + Geist Mono + 4 live studio counters.
 */
export function HfpSplash({ forceShow = false, onFinished }: { forceShow?: boolean; onFinished?: () => void }) {
    const [visible, setVisible] = useState(() => {
        if (forceShow) return true;
        if (typeof window === "undefined") return false;
        const params = new URLSearchParams(window.location.search);
        if (params.get("splash") === "1") return true;
        if (params.get("splash") === "0") return false;
        try {
            if (window.sessionStorage.getItem("hfp-tv:splash-shown") === "1") return false;
            window.sessionStorage.setItem("hfp-tv:splash-shown", "1");
            return true;
        } catch {
            return false;
        }
    });

    const [stage, setStage] = useState<SplashStage>("start");
    const [ready, setReady] = useState(false);
    const [out, setOut] = useState(false);

    const projectsCount = useCanvasStore((s) => s.projects.length);
    const assetsCount = useAssetStore((s) => s.assets.length);
    const channelsCount = useConfigStore((s) => s.config.channels.length);

    const bar = useRef<HTMLElement>(null);
    const live = useRef({ stage, ready, onFinished });
    live.current = { stage, ready, onFinished };

    useEffect(() => {
        if (!visible) return;
        let active = true;
        const t1 = window.setTimeout(() => { if (active) setStage("shelf"); }, 220);
        const t2 = window.setTimeout(() => { if (active) setStage("type"); }, 520);
        const t3 = window.setTimeout(() => {
            if (active) {
                setStage("covers");
                setReady(true);
            }
        }, 900);
        return () => {
            active = false;
            clearTimeout(t1);
            clearTimeout(t2);
            clearTimeout(t3);
        };
    }, [visible]);

    useEffect(() => {
        if (!visible) return;
        let raf = 0;
        let p = 0;
        let full = 0;
        let timer = 0;
        const t0 = performance.now();
        let last = t0;
        const tick = (now: number) => {
            const { stage: s, ready: r } = live.current;
            const goal = Math.min(r ? 1 : STAGE[s], ease((now - t0) / FILL_MS));
            p += (goal - p) * Math.min(1, (now - last) / 90);
            last = now;
            if (goal === 1 && p > 0.996) p = 1;
            if (bar.current) bar.current.style.transform = `scaleX(${p})`;
            if (p === 1 && !full) full = now;
            if (full && now - full >= HOLD_MS) {
                setOut(true);
                timer = window.setTimeout(() => {
                    setVisible(false);
                    live.current.onFinished?.();
                }, FADE_MS);
                return;
            }
            raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);
        return () => {
            cancelAnimationFrame(raf);
            clearTimeout(timer);
        };
    }, [visible]);

    if (!visible) return null;

    const stats: SplashStat[] = [
        [beefTVCapabilityItems.length, "项创作工坊", "Studios"],
        [Math.max(1, channelsCount), "条模型通道", "Channels"],
        [projectsCount, "部画布工程", "Canvases"],
        [assetsCount, "件在库素材", "Assets"],
    ];

    return (
        <div
            className={`splash${out ? " out" : ""}`}
            role="status"
            aria-label="hfp-tv 启动幕布"
            onClick={() => {
                setOut(true);
                window.setTimeout(() => {
                    setVisible(false);
                    onFinished?.();
                }, 220);
            }}
        >
            <div className="sp-art">
                <p className="sp-kicker">AI-Native Video Screening Room · Infinite Canvas · v2.0.3</p>
                <h1>
                    hfp<i>-tv</i>
                </h1>
                <p className="sp-zh">一句构想，在自由画布中化作分镜与成片。</p>
                <p className="sp-en">One spark on an infinite desk, cut and projected into cinema.</p>
                <div className="load-bar">
                    <i ref={bar} />
                </div>
                <ul className="sp-stats on">
                    {stats.map(([n, zh, en]) => (
                        <li key={en}>
                            <b>
                                <Count n={n} />
                            </b>
                            <span>{zh}</span>
                            <small>{en}</small>
                        </li>
                    ))}
                </ul>
                <p className="sp-repo">hfp-tv · Local-First Video Studio</p>
                <p className="sp-credit">Crafted with Ready Library Art Direction · Based on BeefTV</p>
            </div>
        </div>
    );
}
