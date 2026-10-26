/**
 * hfp-tv Liquid Glass Runtime
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

function applyRefraction(el: HTMLElement) {
    const off =
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

let initialized = false;

export function initHfpGlassRuntime() {
    if (typeof window === "undefined" || initialized) return;
    initialized = true;

    let lx = 0.3;
    let ly = 0.2;
    let raf = 0;

    window.addEventListener(
        "pointermove",
        (e) => {
            lx = e.clientX / Math.max(1, window.innerWidth) - 0.5;
            ly = e.clientY / Math.max(1, window.innerHeight) - 0.5;
            if (!raf) {
                raf = requestAnimationFrame(() => {
                    raf = 0;
                    const angle = (Math.atan2(ly, lx) * 180) / Math.PI + 270;
                    document.documentElement.style.setProperty("--la", `${angle.toFixed(1)}deg`);
                });
            }
        },
        { passive: true },
    );

    document.addEventListener(
        "pointermove",
        (e) => {
            const target = e.target instanceof Element ? e.target.closest<HTMLElement>(".hl") : null;
            if (!target) return;
            const r = target.getBoundingClientRect();
            target.style.setProperty("--hx", `${e.clientX - r.left}px`);
            target.style.setProperty("--hy", `${e.clientY - r.top}px`);
        },
        { passive: true },
    );

    document.addEventListener(
        "pointerdown",
        (e) => {
            const target = e.target instanceof Element ? e.target.closest<HTMLElement>(".press, .hfp-press") : null;
            if (!target) return;
            const r = target.getBoundingClientRect();
            if (r.width > 0 && r.height > 0) {
                target.style.setProperty("--ox", `${(((e.clientX - r.left) / r.width) * 100).toFixed(1)}%`);
                target.style.setProperty("--oy", `${(((e.clientY - r.top) / r.height) * 100).toFixed(1)}%`);
            }
            const up = () => {
                target.classList.remove("jelly");
                void target.offsetWidth;
                target.classList.add("jelly");
                window.setTimeout(() => target.classList.remove("jelly"), 580);
                window.removeEventListener("pointerup", up);
            };
            window.addEventListener("pointerup", up);
        },
        { passive: true },
    );

    const ro = new ResizeObserver((entries) => {
        for (const entry of entries) {
            if (entry.target instanceof HTMLElement) applyRefraction(entry.target);
        }
    });

    const scan = () => {
        document.querySelectorAll<HTMLElement>("[data-refract], .lg").forEach((el) => {
            if (!(el as HTMLElement & { __lgObserved?: boolean }).__lgObserved) {
                (el as HTMLElement & { __lgObserved?: boolean }).__lgObserved = true;
                ro.observe(el);
            }
            applyRefraction(el);
        });
    };

    let scanFrame = 0;
    const mo = new MutationObserver(() => {
        if (!scanFrame) {
            scanFrame = requestAnimationFrame(() => {
                scanFrame = 0;
                scan();
            });
        }
    });
    if (document.body) {
        mo.observe(document.body, { childList: true, subtree: true });
        scan();
    }
}
