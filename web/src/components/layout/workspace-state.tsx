import { Button, Skeleton } from "antd";
import { motion } from "motion/react";
import { useLayoutEffect, useState, type ReactNode } from "react";

import { WorkspaceSignalIcon, type WorkspaceSignalIconVariant } from "@/components/ui/aceternity/workspace-signal-icon";
import { motionPreset } from "@/lib/motion-tokens";
import { cn } from "@/lib/utils";

/**
 * null 表示正在保存。短暂保存（400ms 内）沿用上次状态，转圈一旦出现至少停留 600ms，避免提示一闪而过。
 * isFailure 判断为失败的结果不等这两段延迟，立即显示。
 */
export function useSaveIndicator<T>(settled: T | null, isFailure?: (value: T) => boolean) {
    const [previous, setPrevious] = useState(settled);
    const [shownAt, setShownAt] = useState<number | null>(null);
    const saving = settled === null;
    const failed = settled !== null && isFailure?.(settled) === true;
    const showSaving = shownAt !== null && !failed;
    if (!saving && !Object.is(settled, previous)) setPrevious(settled);

    useLayoutEffect(() => {
        if (failed) {
            if (shownAt !== null) setShownAt(null);
            return;
        }
        if (saving === showSaving) return;
        const delay = shownAt === null ? 400 : Math.max(0, shownAt + 600 - performance.now());
        const timer = window.setTimeout(() => setShownAt(saving ? performance.now() : null), delay);
        return () => window.clearTimeout(timer);
    }, [failed, saving, showSaving, shownAt]);

    return { showSaving, settled: saving ? previous : settled };
}

export function WorkspaceState({ icon = "empty", title, description, action, compact = false, className }: { icon?: WorkspaceSignalIconVariant; title: string; description?: string; action?: ReactNode; compact?: boolean; className?: string }) {
    return (
        <motion.section {...motionPreset.fade} className={cn("flex flex-col items-center justify-center text-center", compact ? "min-h-44 py-8" : "min-h-[320px] py-12", className)}>
            <WorkspaceSignalIcon variant={icon} size={compact ? "md" : "lg"} fadeIn={false} />
            <h2 className="mt-4 text-[var(--fs-body-lg)] font-semibold leading-6 text-foreground">{title}</h2>
            {description ? <p className="mt-1.5 max-w-[42ch] text-xs leading-5 text-foreground/58">{description}</p> : null}
            {action ? <div className="mt-5">{action}</div> : null}
        </motion.section>
    );
}

export function WorkspaceErrorState({ title = "暂时无法加载", description, actionLabel = "重新加载", onRetry, compact = false }: { title?: string; description?: string; actionLabel?: string; onRetry?: () => void; compact?: boolean }) {
    return <WorkspaceState icon="error" title={title} description={description || "请检查网络连接后重试，当前内容不会被覆盖。"} compact={compact} action={onRetry ? <Button onClick={onRetry}>{actionLabel}</Button> : undefined} />;
}

export function WorkspaceLoadingState({ label = "正在加载内容", detail, rows = 3, className }: { label?: string; detail?: string; rows?: number; className?: string }) {
    return (
        <section className={cn("workspace-loading-state py-8", className)} aria-busy="true" aria-live="polite">
            <div className="mb-5 flex items-center gap-3">
                <WorkspaceSignalIcon variant="loading" size="sm" />
                <div><div className="text-sm font-medium">{label}</div>{detail ? <div className="mt-0.5 text-xs text-foreground/50">{detail}</div> : null}</div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {Array.from({ length: rows }, (_, index) => <div key={index} className="rounded-md bg-surface-active p-4"><Skeleton active title={{ width: `${48 + index * 8}%` }} paragraph={{ rows: 3 }} /></div>)}
            </div>
        </section>
    );
}
