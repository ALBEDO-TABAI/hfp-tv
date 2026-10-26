import { useEffect, useState } from "react";

import { HfpLogoMark, HfpWordmark } from "@/components/brand/brand-logo";
import { cn } from "@/lib/utils";

type FullScreenLoaderProps = {
    label?: string;
    detail?: string;
    className?: string;
};

export function FullScreenLoader({ label = "正在恢复工作区", detail = "同步账号、模型和项目数据", className }: FullScreenLoaderProps) {
    return (
        <div
            data-full-screen-loader
            role="status"
            aria-live="polite"
            aria-label={`${label}，${detail}`}
            className={cn("full-screen-loader flex flex-col items-center justify-center gap-3", className)}
        >
            <div className="flex items-center gap-2.5">
                <HfpLogoMark size={32} />
                <HfpWordmark />
            </div>
            <span className="mono text-xs opacity-60">{label}</span>
        </div>
    );
}

export function WorkspaceRouteLoader({ label = "正在打开页面" }: { label?: string }) {
    const [visible, setVisible] = useState(false);

    useEffect(() => {
        const timer = window.setTimeout(() => setVisible(true), 140);
        return () => window.clearTimeout(timer);
    }, []);

    return (
        <section data-workspace-route-loader className={cn("workspace-route-loader", visible && "is-visible")} role="status" aria-live="polite" aria-label={label}>
            <div className="workspace-route-loader-content flex items-center gap-2.5">
                <HfpLogoMark size={22} />
                <span className="mono text-xs">{label}</span>
            </div>
        </section>
    );
}
