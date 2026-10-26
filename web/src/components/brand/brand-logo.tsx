import { useState, type CSSProperties, type ReactNode } from "react";

import { cn } from "@/lib/utils";
import { appearanceLogoURL, useAppearanceStore } from "@/stores/use-appearance-store";
import type { ThemeName } from "@/stores/use-theme-store";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";

/**
 * hfp-tv mark — adapted from Ready Library's shelf mark:
 * three film reels / storyboard spines, one leaning in motion, and the print-signal REC dot.
 */
export function HfpLogoMark({ size = 26, className }: { size?: number; className?: string }) {
    return (
        <svg width={size} height={size} viewBox="0 0 26 26" className={cn("hfp-logo-mark shrink-0", className)} aria-hidden="true">
            <rect x="1" y="1" width="24" height="24" rx="5.5" fill="var(--ink, currentColor)" />
            <rect x="4.8" y="5" width="4" height="16" rx="1.1" fill="var(--logo-ink, #ebebee)" />
            <rect x="10.4" y="6.8" width="4" height="14.2" rx="1.1" fill="var(--logo-ink, #ebebee)" opacity="0.58" />
            <rect x="15.8" y="9.2" width="4" height="11.8" rx="1.1" transform="rotate(-14 17.8 15)" fill="var(--logo-ink, #ebebee)" />
            <circle cx="19.6" cy="6.4" r="2.6" fill="var(--rl-accent, #ff5a1f)" />
        </svg>
    );
}

export function HfpWordmark({ className }: { className?: string }) {
    return (
        <span className={cn("hfp-wordmark", className)}>
            hfp<i>-tv</i>
        </span>
    );
}

type BrandLogoProps = {
    className?: string;
    fallback: ReactNode;
    alt?: string;
    theme?: ThemeName | "auto";
};

export function BrandLogo({ className, fallback, alt = "", theme = "auto" }: BrandLogoProps) {
    const appearance = useAppearanceStore((state) => state.appearance);
    const currentTheme = useActiveTheme();
    const source = appearanceLogoURL(appearance, theme === "auto" ? currentTheme : theme);
    const [failedSource, setFailedSource] = useState<string | null>(null);

    if (!appearance.logoConfigured) {
        return <HfpLogoMark size={26} className={className} />;
    }

    if (failedSource === source) return appearance.logoConfigured ? <span className={cn("block", className)} aria-hidden="true" /> : <>{fallback}</>;
    return (
        <img
            src={source}
            alt={alt}
            className={cn("block object-contain", className)}
            draggable={false}
            onError={(event) => {
                event.currentTarget.style.visibility = "hidden";
                setFailedSource(source);
            }}
        />
    );
}

export function BrandLogoFrame({ className, logoClassName, fallback, alt = "", theme = "auto" }: BrandLogoProps & { logoClassName?: string }) {
    const frameEnabled = useAppearanceStore((state) => state.appearance.logoFrameEnabled);
    const unframedStyle: CSSProperties | undefined = frameEnabled
        ? undefined
        : {
              background: "transparent",
              borderColor: "transparent",
              borderRadius: 0,
              boxShadow: "none",
              color: "inherit",
          };
    return (
        <span className={cn("brand-logo-frame", className)} data-logo-frame-enabled={frameEnabled} style={unframedStyle}>
            <BrandLogo className={logoClassName} fallback={fallback} alt={alt} theme={theme} />
        </span>
    );
}
