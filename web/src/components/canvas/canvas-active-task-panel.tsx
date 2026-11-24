import { AnimatePresence, LayoutGroup, motion, useReducedMotionConfig } from "motion/react";
import { ChevronDown, ChevronUp, Clock3, ListTodo, LoaderCircle, XCircle } from "lucide-react";
import { useEffect, useState } from "react";

import { motionSpring, motionTransition } from "@/lib/motion-tokens";
import { formatTaskKind, generationTaskShowsProgress, generationTaskStageLabel, generationTaskStatusLabel } from "@/lib/generation-task-display";
import { canvasThemes } from "@/lib/canvas-theme";
import type { GenerationTask } from "@/services/api/task-center";
import { useActiveTheme } from "@/stores/canvas/use-canvas-theme-store";

const taskDisclosureMotion = {
    initial: { opacity: 0 },
    animate: { opacity: 1 },
    exit: { opacity: 0, transition: motionTransition.panelExit },
    transition: motionTransition.panelEnter,
};

// 顶栏是绝对定位浮层，面板必须按调用方传入的 topInset 避让；专注模式隐藏顶栏时传小间距。
export function CanvasActiveTaskPanel({ tasks, align = "right", topInset = "var(--canvas-topbar-offset)", onCancelTask }: { tasks: GenerationTask[]; align?: "left" | "right"; topInset?: string; onCancelTask?: (task: GenerationTask) => void }) {
    const theme = canvasThemes[useActiveTheme()];
    const [open, setOpen] = useState(false);
    const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null);

    useEffect(() => {
        if (expandedTaskId && !tasks.some((task) => task.id === expandedTaskId)) setExpandedTaskId(null);
    }, [expandedTaskId, tasks]);

    return (
        <AnimatePresence>
            {tasks.length > 0 ? (
                <motion.div
                    key="canvas-active-task-panel"
                    data-canvas-no-zoom
                    initial={{ opacity: 0, y: -4 }}
                    animate={{ opacity: 1, y: 0, transition: motionSpring.smooth }}
                    exit={{ opacity: 0, transition: motionTransition.panelExit }}
                    className={`pointer-events-none absolute z-[var(--z-panel-floating)] w-[var(--canvas-panel-width)] ${align === "left" ? "left-3" : "right-3"}`}
                    style={{ top: topInset }}
                >
                    <LayoutGroup id="canvas-active-tasks">
                        <motion.section
                            layout
                            className="pointer-events-auto overflow-hidden rounded-[var(--panel-radius)] border backdrop-blur-2xl"
                            style={{ background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.node.text, boxShadow: `0 24px 72px ${theme.spatial.shadow}` }}
                            aria-label="当前画布生成任务"
                        >
                            <motion.button
                                type="button"
                                layout
                                className="flex min-h-12 w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-white/[0.04] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px]"
                                onClick={() => setOpen((value) => !value)}
                                aria-expanded={open}
                                aria-controls="canvas-active-task-list"
                            >
                                <span className="flex min-w-0 items-center gap-2">
                                    <span className="grid size-8 shrink-0 place-items-center rounded-[var(--dock-item-radius)]" style={{ background: theme.accent.primarySoft, color: theme.accent.primary }}>
                                        <ListTodo className="size-4" />
                                    </span>
                                    <span className="min-w-0">
                                        <span className="block text-sm font-semibold leading-5">生成任务</span>
                                        <span className="block truncate text-[var(--fs-label)]" style={{ color: theme.node.muted }} aria-live="polite">
                                            当前画布 · {tasks.length} 个进行中
                                        </span>
                                    </span>
                                </span>
                                <span className="flex shrink-0 items-center gap-2" style={{ color: theme.accent.primary }}>
                                    <LoaderCircle className="size-4 animate-spin opacity-70 motion-reduce:animate-none" />
                                    {open ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
                                </span>
                            </motion.button>

                            <AnimatePresence initial={false}>
                                {open ? (
                                    <motion.div
                                        id="canvas-active-task-list"
                                        layout
                                        {...taskDisclosureMotion}
                                        className="thin-scrollbar max-h-[min(70vh,520px)] space-y-2 overflow-y-auto px-2.5 pb-2.5"
                                    >
                                        {tasks.map((task) => (
                                            <ActiveTaskCard
                                                key={task.id}
                                                task={task}
                                                theme={theme}
                                                expanded={expandedTaskId === task.id}
                                                onToggle={() => setExpandedTaskId((current) => (current === task.id ? null : task.id))}
                                                onCancelTask={onCancelTask}
                                            />
                                        ))}
                                    </motion.div>
                                ) : null}
                            </AnimatePresence>
                        </motion.section>
                    </LayoutGroup>
                </motion.div>
            ) : null}
        </AnimatePresence>
    );
}

function ActiveTaskCard({
    task,
    theme,
    expanded,
    onToggle,
    onCancelTask,
}: {
    task: GenerationTask;
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    expanded: boolean;
    onToggle: () => void;
    onCancelTask?: (task: GenerationTask) => void;
}) {
    const reducedMotion = useReducedMotionConfig();
    const showsProgress = generationTaskShowsProgress(task);
    const progress = showsProgress && typeof task.progress === "number" ? Math.max(0, Math.min(100, Math.round(task.progress))) : showsProgress && task.status === "queued" ? 0 : undefined;
    const statusTone = task.status === "running" ? theme.accent.primary : theme.node.muted;

    return (
        <motion.article layout layoutId={`canvas-active-task-${task.id}`} className="overflow-hidden rounded-xl border" style={{ background: theme.spatial.surface, borderColor: theme.toolbar.border }}>
            <button type="button" className="block w-full p-3 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px]" onClick={onToggle} aria-expanded={expanded}>
                <div className="flex min-w-0 items-start gap-2">
                    <motion.span layout="position" className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-md" style={{ background: `${statusTone}18`, color: statusTone }}>
                        <ListTodo className="size-3.5" />
                    </motion.span>
                    <span className="min-w-0 flex-1">
                        <span className="flex items-center justify-between gap-2">
                            <span className="truncate text-xs font-semibold" title={formatTaskKind(task)}>
                                {formatTaskKind(task)}
                            </span>
                            <span className="shrink-0 rounded-full border px-1.5 py-0.5 text-[var(--fs-tiny)] font-medium" style={{ borderColor: `${statusTone}44`, color: statusTone }}>
                                {generationTaskStatusLabel(task)}
                            </span>
                        </span>
                        <span className="mt-1 block truncate text-[var(--fs-label)]" style={{ color: theme.node.muted }} title={generationTaskStageLabel(task)}>
                            {generationTaskStageLabel(task)}
                        </span>
                    </span>
                    {expanded ? <ChevronUp className="mt-0.5 size-3.5 shrink-0" style={{ color: theme.node.muted }} /> : <ChevronDown className="mt-0.5 size-3.5 shrink-0" style={{ color: theme.node.muted }} />}
                </div>

                {showsProgress ? (
                    <div className="mt-3 h-1.5 overflow-hidden rounded-full" style={{ background: theme.toolbar.itemHover }}>
                        {progress !== undefined ? (
                            <div
                                className={`h-full origin-left rounded-full${reducedMotion ? "" : " transition-transform duration-(--motion-state) ease-(--ease-out)"}`}
                                style={{ background: statusTone, transform: `scaleX(${progress / 100})` }}
                            />
                        ) : (
                            <div
                                aria-hidden
                                className={reducedMotion ? "h-full rounded-full bg-current opacity-45" : "canvas-task-progress-shimmer h-full w-1/2 bg-linear-to-r from-transparent via-current to-transparent"}
                                style={{ color: statusTone }}
                            />
                        )}
                    </div>
                ) : null}

                {/* 进度百分比显示（对应 #98 决策3：列表视图）*/}
                {progress !== undefined && task.status === "running" ? (
                    <div className="mt-1 flex items-center justify-between text-[var(--fs-micro)]" style={{ color: theme.node.muted }}>
                        <ActiveTaskElapsedTime task={task} />
                        <span className="font-medium tabular-nums" style={{ color: statusTone }}>
                            {progress}%
                        </span>
                    </div>
                ) : (
                    <div className="mt-3 grid grid-cols-1 gap-2 text-[var(--fs-tiny)]" style={{ color: theme.node.muted }}>
                        <ActiveTaskElapsedTime task={task} withIcon />
                    </div>
                )}
            </button>

            <AnimatePresence initial={false}>
                {expanded ? (
                    <motion.div
                        {...taskDisclosureMotion}
                        className="border-t px-3 pb-3 pt-2 text-[var(--fs-label)]"
                        style={{ borderColor: theme.toolbar.border, color: theme.node.muted }}
                    >
                        <div className="flex items-center justify-between gap-2">
                            <span>当前阶段</span>
                            <span className="max-w-[200px] truncate text-right" style={{ color: theme.node.text }}>
                                {generationTaskStageLabel(task)}
                            </span>
                        </div>
                        {onCancelTask && (task.status === "queued" || task.status === "running") ? (
                            <button
                                type="button"
                                className="mt-3 inline-flex h-7 items-center gap-1 rounded-[var(--r-sm)] px-2 text-[var(--fs-tiny)] font-medium transition-colors"
                                style={{ background: `${theme.accent.danger}16`, color: theme.accent.danger }}
                                onClick={(event) => {
                                    event.stopPropagation();
                                    onCancelTask(task);
                                }}
                                onMouseDown={(event) => event.stopPropagation()}
                            >
                                <XCircle className="size-3" />
                                取消任务
                            </button>
                        ) : null}
                    </motion.div>
                ) : null}
            </AnimatePresence>
        </motion.article>
    );
}

function ActiveTaskElapsedTime({ task, withIcon = false }: { task: GenerationTask; withIcon?: boolean }) {
    const [now, setNow] = useState(Date.now);

    useEffect(() => {
        const timer = window.setInterval(() => setNow(Date.now()), 1_000);
        return () => window.clearInterval(timer);
    }, []);

    const elapsedMs = Math.max(0, now - parseTime(task.startedAt || task.createdAt));
    const label = `${task.status === "queued" ? "已等待" : "已运行"} ${formatDuration(elapsedMs)}`;
    if (!withIcon) return <span>{label}</span>;
    return (
        <span className="inline-flex min-w-0 items-center gap-1 truncate" title={label}>
            <Clock3 className="size-3 shrink-0" />
            {label}
        </span>
    );
}

function parseTime(value?: string) {
    if (!value) return Date.now();
    const time = new Date(value).getTime();
    return Number.isFinite(time) ? time : Date.now();
}

function formatDuration(value: number) {
    const totalSeconds = Math.floor(value / 1_000);
    const hours = Math.floor(totalSeconds / 3_600);
    const minutes = Math.floor((totalSeconds % 3_600) / 60);
    const seconds = totalSeconds % 60;
    return hours ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}` : `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}
