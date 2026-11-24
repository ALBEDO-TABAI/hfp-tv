/**
 * hfp - all in one Liquid Glass Runtime
 * Adapted from Ready Library (藏书阁) / Glass-HQ/liquid-glass (MIT License, © 2026 Glass HQ;
 * see licenses/ui/LICENSE-liquid-glass.txt).
 */

type GlassMaterial = "clear" | "regular" | "thick";

const MAT: Record<GlassMaterial, { blur: number; sat: number }> = {
    clear: { blur: 4, sat: 1.7 },
    regular: { blur: 14, sat: 1.8 },
    thick: { blur: 22, sat: 1.85 },
};

const sat = (x: number) => (x < 0 ? 0 : x > 1 ? 1 : x);

function sdRect(px: number, py: number, hw: number, hh: number, r: number): [number, number, number] {
    const qx = Math.abs(px) - (hw - r);
    const qy = Math.abs(py) - (hh - r);
    let d: number;
    let nx: number;
    let ny: number;
    if (qx > 0 && qy > 0) {
        const l = Math.hypot(qx, qy) || 1e-5;
        d = l - r;
        nx = (qx / l) * Math.sign(px);
        ny = (qy / l) * Math.sign(py);
    } else if (qx > qy) {
        d = qx - r;
        nx = Math.sign(px) || 1;
        ny = 0;
    } else {
        d = qy - r;
        nx = 0;
        ny = Math.sign(py) || 1;
    }
    return [d, nx, ny];
}

const ovCache = new Map<string, string>();

function overlayMap(w: number, h: number, r: number, dark: boolean): string {
    const dpr = Math.min(2, typeof window !== "undefined" ? window.devicePixelRatio || 1 : 1);
    const W = Math.max(2, Math.round(w * dpr));
    const H = Math.max(2, Math.round(h * dpr));
    const c = document.createElement("canvas");
    c.width = W;
    c.height = H;
    const ctx = c.getContext("2d");
    if (!ctx) return "";
    const img = ctx.createImageData(W, H);
    const a = img.data;
    const aa = 1 / dpr;
    const cs = Math.cos(1.08);
    const cd = Math.cos(1.08 * 0.65);
    const cside = Math.cos(0.98);
    const mix = (p: number, q: number, t: number) => p + (q - p) * t;
    const dk = dark ? 1 : 0;

    for (let y = 0; y < H; y++) {
        const py = (y + 0.5) / dpr - h / 2;
        for (let x = 0; x < W; x++) {
            const px = (x + 0.5) / dpr - w / 2;
            const [d, nx, ny] = sdRect(px, py, w / 2, h / 2, r);
            if (d < -6) continue;
            const coverage = sat(0.5 - d / aa);
            const directional = Math.pow(sat((Math.abs(ny) - cs) / (1 - cs)), 1.25);
            const diffuseDir = Math.pow(sat((Math.abs(ny) - cd) / (1 - cd)), 1.25);
            const stroke = sat((d + 0.9) / aa + 0.5) * sat(0.5 - d / aa);
            let spec = directional * stroke;
            spec = spec / (1 + 0.5 * (1 - spec));
            const diffuse = diffuseDir * 0.18 * Math.pow(1 - sat(-d / 4.5), 2) * coverage;
            const hl = sat(spec * mix(1, 0.82, dk) + diffuse * mix(1, 0.4, dk));
            const adj = d - 0.5;
            const falloff = 1 - sat(-adj / 0.5);
            const ramp = mix(falloff > 0 ? 1 : 0, falloff, 0.75);
            const outline = ramp * sat((adj + 0.5) / aa + 0.5) * sat(-adj / aa + 0.5);
            const w1 = sat((nx - cside) / (1 - cside)) * outline;
            const w2 = sat((-nx - cside) / (1 - cside)) * outline;
            const shade = (w1 / (1 + 0.4 * (1 - w1)) + w2 / (1 + 0.4 * (1 - w2))) * mix(0.48, 0.6, dk);
            const alpha = hl + shade * (1 - hl);
            if (alpha <= 0.002) continue;
            const light = (mix(1, 0.82, dk) * hl) / Math.max(alpha, 1e-5);
            const i = (y * W + x) * 4;
            const v = Math.round(light * 255);
            a[i] = v;
            a[i + 1] = v;
            a[i + 2] = v;
            a[i + 3] = Math.round(alpha * 255);
        }
    }
    ctx.putImageData(img, 0, 0);
    return c.toDataURL("image/png");
}

function overlayFor(w: number, h: number, r: number, dark: boolean): string {
    const key = [Math.round(w), Math.round(h), Math.round(r), dark ? 1 : 0].join("x");
    if (!ovCache.has(key)) ovCache.set(key, overlayMap(Math.round(w), Math.round(h), Math.round(r), dark));
    return ovCache.get(key) || "";
}

/** 「流畅模式」：不画折射贴图，也不跟随指针改高光角度。 */
const effectsLite = () => document.documentElement.dataset.effects === "lite";

function applyRefraction(el: HTMLElement) {
    const off =
        effectsLite() ||
        document.documentElement.classList.contains("reduce-transparency") ||
        window.matchMedia?.("(prefers-reduced-transparency: reduce)").matches;
    const w = el.offsetWidth;
    const h = el.offsetHeight;
    if (w < 8 || h < 8 || w > 1800 || h > 1400) return;
    const cs = getComputedStyle(el);
    const rad = Math.min(parseFloat(cs.borderTopLeftRadius) || 0, h / 2, w / 2);
    const dark = document.documentElement.classList.contains("dark") || document.documentElement.dataset.theme === "dark";
    const matKey = (el.dataset.mat as GlassMaterial) || "regular";
    const mat = MAT[matKey] || MAT.regular;

    if (!off) {
        const url = overlayFor(w, h, rad, dark);
        if (url) el.style.setProperty("--lg-ov", `url(${url})`);
    } else {
        el.style.removeProperty("--lg-ov");
    }
    void mat;
}

// The rim highlight follows the pointer in coarse steps; finer steps are not
// visible but each one restyles every glass element.
const LIGHT_ANGLE_STEP = 15;
const GLASS_SELECTOR = "[data-refract], .lg";

/**
 * 首页玻璃卡片的运行时：指针改变高光角度、按元素尺寸生成折射贴图、记录按压原点。
 * 玻璃卡片只在首页，所以只在首页挂载期间运行，并且只观察首页这棵子树；流畅模式下不启动。
 * 返回清理函数：移除监听、断开观察，并去掉已经画上的贴图和高光角度。
 */
export function initHfpGlassRuntime(scope: HTMLElement) {
    if (typeof window === "undefined") return () => undefined;

    let lx = 0.3;
    let ly = 0.2;
    let raf = 0;
    let lightAngle = "";
    const glass = scope.getElementsByClassName("lg");
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)");

    const onPointerMove = (e: PointerEvent) => {
        // Drags and canvas pans move the pointer every frame; skip the highlight then.
        if (e.buttons !== 0 || reducedMotion?.matches || glass.length === 0 || effectsLite()) return;
        lx = e.clientX / Math.max(1, window.innerWidth) - 0.5;
        ly = e.clientY / Math.max(1, window.innerHeight) - 0.5;
        if (raf) return;
        raf = requestAnimationFrame(() => {
            raf = 0;
            const raw = (Math.atan2(ly, lx) * 180) / Math.PI + 270;
            const angle = `${Math.round(raw / LIGHT_ANGLE_STEP) * LIGHT_ANGLE_STEP}deg`;
            if (angle === lightAngle) return;
            lightAngle = angle;
            // --la is registered as non-inheriting: writing it on each glass element
            // restyles only those elements, not the whole document as on <html>.
            for (const element of glass) (element as HTMLElement).style.setProperty("--la", angle);
        });
    };

    // 按压从指针所在的位置内收（.press 的 transform-origin）。
    const onPointerDown = (e: PointerEvent) => {
        const target = e.target instanceof Element ? e.target.closest<HTMLElement>(".press, .hfp-press") : null;
        if (!target) return;
        const r = target.getBoundingClientRect();
        if (r.width > 0 && r.height > 0) {
            target.style.setProperty("--ox", `${(((e.clientX - r.left) / r.width) * 100).toFixed(1)}%`);
            target.style.setProperty("--oy", `${(((e.clientY - r.top) / r.height) * 100).toFixed(1)}%`);
        }
    };

    // ResizeObserver reports every observed element once on observe and again on
    // each size change, so it alone keeps the refraction overlay current. The first
    // report paints at once; later ones wait until the size settles, because a
    // sidebar or window resize changes the size every frame and each new size
    // costs a full per-pixel overlay plus a PNG encode. The old overlay is drawn
    // at 100% × 100%, so it stretches cleanly while the resize is in progress.
    const resized = new Set<HTMLElement>();
    let settleTimer = 0;
    const ro = new ResizeObserver((entries) => {
        for (const entry of entries) {
            if (!(entry.target instanceof HTMLElement)) continue;
            if (entry.target.style.getPropertyValue("--lg-ov")) resized.add(entry.target);
            else applyRefraction(entry.target);
        }
        if (!resized.size) return;
        window.clearTimeout(settleTimer);
        settleTimer = window.setTimeout(() => {
            for (const element of resized) if (element.isConnected) applyRefraction(element);
            resized.clear();
        }, 160);
    });
    const observed = new WeakSet<HTMLElement>();
    const prepare = (el: HTMLElement) => {
        if (!observed.has(el)) {
            observed.add(el);
            ro.observe(el);
        }
        if (lightAngle && el.classList.contains("lg")) el.style.setProperty("--la", lightAngle);
    };
    const scanTree = (root: Element) => {
        if (root instanceof HTMLElement && root.matches(GLASS_SELECTOR)) prepare(root);
        root.querySelectorAll<HTMLElement>(GLASS_SELECTOR).forEach(prepare);
    };

    // Only newly added subtrees are scanned; re-measuring every glass element on any
    // DOM change would force a layout per frame.
    const added = new Set<Element>();
    let scanFrame = 0;
    const mo = new MutationObserver((records) => {
        for (const record of records) {
            record.addedNodes.forEach((node) => {
                if (node instanceof Element) added.add(node);
            });
        }
        if (added.size && !scanFrame) {
            scanFrame = requestAnimationFrame(() => {
                scanFrame = 0;
                for (const element of added) if (element.isConnected) scanTree(element);
                added.clear();
            });
        }
    });
    // Theme, transparency and effects-level changes alter the overlay without resizing anything.
    // 逐像素重算折射贴图很重，放到空闲时做，不卡住主题揭示的开头。
    let themeRecompute = 0;
    const idle = typeof window.requestIdleCallback === "function";
    const themeObserver = new MutationObserver(() => {
        if (themeRecompute) return;
        const run = () => {
            themeRecompute = 0;
            scope.querySelectorAll<HTMLElement>(GLASS_SELECTOR).forEach(applyRefraction);
        };
        themeRecompute = idle ? window.requestIdleCallback(run, { timeout: 600 }) : window.setTimeout(run, 360);
    });

    window.addEventListener("pointermove", onPointerMove, { passive: true });
    scope.addEventListener("pointerdown", onPointerDown, { passive: true });
    mo.observe(scope, { childList: true, subtree: true });
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "data-theme", "data-effects"] });
    scanTree(scope);

    return () => {
        window.removeEventListener("pointermove", onPointerMove);
        scope.removeEventListener("pointerdown", onPointerDown);
        ro.disconnect();
        mo.disconnect();
        themeObserver.disconnect();
        cancelAnimationFrame(raf);
        cancelAnimationFrame(scanFrame);
        window.clearTimeout(settleTimer);
        if (themeRecompute) {
            if (idle) window.cancelIdleCallback(themeRecompute);
            else window.clearTimeout(themeRecompute);
        }
        scope.querySelectorAll<HTMLElement>(GLASS_SELECTOR).forEach((element) => {
            element.style.removeProperty("--lg-ov");
            element.style.removeProperty("--la");
        });
    };
}
