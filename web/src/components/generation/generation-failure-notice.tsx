import { Check, Copy, Eye, RefreshCw, TriangleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { formatGenerationDiagnostics, type GenerationFailureContext, type GenerationFailureExplanation } from "@/lib/generation-error";
import { getDiagnosticTraceId } from "@/services/diagnostics/client-diagnostics";

type GenerationFailureNoticeProps = {
    explanation: GenerationFailureExplanation;
    context?: GenerationFailureContext;
    compact?: boolean;
    onRetry?: () => void;
    retryLabel?: string;
    onOpenDetails?: () => void;
    retryDisabled?: boolean;
    summaryOnly?: boolean;
};

const copyFeedback = {
    idle: { label: "复制排查信息", Icon: Copy },
    copied: { label: "已复制", Icon: Check },
    failed: { label: "复制失败", Icon: TriangleAlert },
};

export function GenerationFailureNotice({ explanation, context, compact, onRetry, retryLabel, onOpenDetails, retryDisabled, summaryOnly }: GenerationFailureNoticeProps) {
    const [copyState, setCopyState] = useState<keyof typeof copyFeedback>("idle");
    const copyTimer = useRef<number | undefined>(undefined);
    const copyAttempt = useRef(0);
    const diagnostics = formatGenerationDiagnostics(explanation, { ...context, traceId: context?.traceId || getDiagnosticTraceId(context?.taskId || explanation.taskId) });
    const showRetry = Boolean(onRetry) && !explanation.blockManualRetry && !retryDisabled;
    useEffect(() => () => {
        copyAttempt.current++;
        window.clearTimeout(copyTimer.current);
    }, []);
    const copyDiagnostics = async () => {
        const attempt = ++copyAttempt.current;
        let result: "copied" | "failed" = "copied";
        try {
            if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
                await navigator.clipboard.writeText(diagnostics);
            } else {
                const copied = fallbackCopy(diagnostics);
                if (!copied) throw new Error("copy failed");
            }
        } catch {
            result = "failed";
        }
        if (attempt !== copyAttempt.current) return;
        window.clearTimeout(copyTimer.current);
        setCopyState(result);
        copyTimer.current = window.setTimeout(() => setCopyState("idle"), 1500);
    };
    return (
        <div className={`generation-failure-notice${compact ? " is-compact" : ""}`}>
            <p className="generation-failure-reason">{explanation.reason}</p>
            {!summaryOnly && explanation.action ? <p className="generation-failure-action">{explanation.action}</p> : null}
            <div className="generation-failure-actions">
                {onOpenDetails ? (
                    <button
                        type="button"
                        className="generation-failure-button"
                        onClick={(event) => {
                            event.stopPropagation();
                            onOpenDetails();
                        }}
                        onMouseDown={(event) => event.stopPropagation()}
                    >
                        <Eye className="size-3.5" />
                        {explanation.uncertain ? "查看原任务" : "查看详情"}
                    </button>
                ) : null}
                {!summaryOnly && diagnostics ? <>
                    <button
                        type="button"
                        className="generation-failure-button"
                        aria-label={copyFeedback[copyState].label}
                        onClick={(event) => {
                            event.stopPropagation();
                            void copyDiagnostics();
                        }}
                        onMouseDown={(event) => event.stopPropagation()}
                    >
                        <span className="inline-grid" aria-hidden="true">
                            {Object.entries(copyFeedback).map(([state, { label, Icon }]) => (
                                <span key={state} className={`col-start-1 row-start-1 grid grid-cols-[auto_1fr] items-center gap-1.5 whitespace-nowrap text-center transition-opacity duration-(--motion-state) ease-(--ease-out) ${state === copyState ? "opacity-100" : "opacity-0"}`}>
                                    <Icon className="size-3.5" />{label}
                                </span>
                            ))}
                        </span>
                    </button>
                    <span role="status" className="sr-only">{copyState === "idle" ? "" : copyFeedback[copyState].label}</span>
                </> : null}
                {showRetry ? (
                    <button
                        type="button"
                        className="generation-failure-button"
                        onClick={(event) => {
                            event.stopPropagation();
                            onRetry?.();
                        }}
                        onMouseDown={(event) => event.stopPropagation()}
                    >
                        <RefreshCw className="size-3.5" />
                        {retryLabel || "重新生成"}
                    </button>
                ) : null}
            </div>
        </div>
    );
}

function fallbackCopy(value: string) {
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "true");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    try {
        return document.execCommand("copy");
    } finally {
        area.remove();
    }
}
