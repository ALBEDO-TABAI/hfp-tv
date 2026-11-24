import { AnimatePresence, motion } from "motion/react";
import { forwardRef, useEffect, useState, type CSSProperties, type MouseEvent, type ReactNode } from "react";

import { cn } from "@/lib/utils";
import { motionPreset, motionSpring } from "@/lib/motion-tokens";

export type FloatingDockCommand = {
    kind?: "command";
    id: string;
    label: string;
    displayLabel?: string;
    icon: ReactNode;
    wide?: boolean;
    quiet?: boolean;
    onClick?: (event: MouseEvent<HTMLButtonElement>) => void;
    active?: boolean;
    disabled?: boolean;
    danger?: boolean;
    /** 主操作强调态：用于 LibTV 风格 Dock 中始终突出的添加入口。 */
    prominent?: boolean;
    /** 面板展开型工具——使用 aria-expanded 而非 aria-pressed */
    expands?: boolean;
};

export type FloatingDockSwitchOption = {
    id: string;
    label: string;
    icon: ReactNode;
    value: string;
};

export type FloatingDockSwitch = {
    kind: "switch";
    id: string;
    label: string;
    value: string;
    options: FloatingDockSwitchOption[];
    onChange: (value: string) => void;
};

export type FloatingDockEntry = FloatingDockCommand | FloatingDockSwitch | { kind: "separator"; id: string };

type FloatingDockProps = {
    items: FloatingDockEntry[];
    size?: "default" | "compact" | "libtv";
    embedded?: boolean;
    className?: string;
    style?: CSSProperties;
    ariaLabel?: string;
    showLabels?: boolean;
};

type DockMetrics = {
    base: number;
    icon: number;
};

const DOCK_METRICS: Record<NonNullable<FloatingDockProps["size"]>, DockMetrics> = {
    default: { base: 30, icon: 15 },
    compact: { base: 26, icon: 13 },
    libtv: { base: 32, icon: 18 },
};

const TOUCH_DOCK_METRICS: Record<NonNullable<FloatingDockProps["size"]>, DockMetrics> = {
    default: { base: 40, icon: 18 },
    compact: { base: 36, icon: 16 },
    libtv: { base: 32, icon: 18 },
};

export const FloatingDock = forwardRef<HTMLDivElement, FloatingDockProps>(function FloatingDock({ items, size = "default", embedded = false, className, style, ariaLabel = "画布工具", showLabels = false }, forwardedRef) {
    const [coarsePointer, setCoarsePointer] = useState(() => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(pointer: coarse)").matches);
    // 窄屏或触屏时允许横向滚动，保证所有按钮可达。
    const [narrow, setNarrow] = useState(() => (typeof window !== "undefined" ? window.innerWidth < 768 : false));

    useEffect(() => {
        if (typeof window.matchMedia !== "function") return;
        const media = window.matchMedia("(pointer: coarse)");
        const update = () => setCoarsePointer(media.matches);
        update();
        media.addEventListener("change", update);
        return () => media.removeEventListener("change", update);
    }, []);

    useEffect(() => {
        const update = () => setNarrow(window.innerWidth < 768);
        window.addEventListener("resize", update);
        return () => window.removeEventListener("resize", update);
    }, []);

    const scrollable = coarsePointer || narrow;
    const metrics = coarsePointer ? TOUCH_DOCK_METRICS[size] : DOCK_METRICS[size];

    return (
        <div
            ref={forwardedRef}
            role="toolbar"
            aria-label={ariaLabel}
            className={cn(
                "aceternity-floating-dock flex",
                scrollable ? "overflow-x-auto" : "overflow-visible",
                showLabels ? "items-center" : "items-end",
                embedded ? "shadow-none" : "border backdrop-blur-2xl",
                showLabels
                    ? embedded
                        ? size === "compact"
                            ? "h-9 gap-0.5 px-0.5"
                            : "h-10 gap-0.5 px-0.5"
                        : size === "compact"
                          ? "h-10 gap-0.5 rounded-[var(--dock-radius-compact)] px-1.5"
                          : "h-11 gap-0.5 rounded-[var(--dock-radius-tight)] px-2"
                    : coarsePointer
                      ? embedded
                          ? size === "compact"
                              ? "h-10 gap-1 px-0.5"
                              : "h-11 gap-1 px-0.5"
                          : size === "compact"
                            ? "h-11 gap-1 rounded-[var(--dock-radius-tight)] px-1.5 pb-1"
                            : "h-12 gap-1 rounded-[var(--panel-radius)] px-2 pb-1"
                      : embedded
                        ? size === "compact"
                            ? "h-8 gap-0.5 px-0.5 pb-0.5"
                            : "h-9 gap-0.5 px-0.5 pb-0.5"
                        : size === "compact"
                          ? "h-8 gap-0.5 rounded-[var(--r-lg)] px-1 pb-1"
                          : "h-10 gap-0.5 rounded-[var(--dock-radius)] px-1.5 pb-1",
                className,
            )}
            style={style}
        >
            {renderDockItems(items, { metrics, scrollable, compact: size === "compact", showLabel: showLabels })}
        </div>
    );
});

type DockItemRenderProps = {
    metrics: DockMetrics;
    scrollable: boolean;
    compact: boolean;
    showLabel: boolean;
};

/**
 * 渲染 Dock 条目，将连续的 danger 命令包裹在 is-danger-group 容器中实现视觉隔离。
 * 满足"危险操作必须与常规按钮隔离"的硬约束。
 */
function renderDockItems(items: FloatingDockEntry[], props: DockItemRenderProps) {
    const result: ReactNode[] = [];
    let dangerGroup: FloatingDockCommand[] = [];
    let index = 0;

    const flushDangerGroup = () => {
        if (!dangerGroup.length) return;
        const groupKey = `danger-group-${index}`;
        result.push(
            <span key={groupKey} className="aceternity-dock-danger-group flex shrink-0 items-end gap-0.5 rounded-[calc(var(--dock-item-radius)+2px)] px-0.5">
                {dangerGroup.map((command) => (
                    <DockCommandButton key={command.id} command={command} {...props} />
                ))}
            </span>,
        );
        dangerGroup = [];
    };

    for (const item of items) {
        if (item.kind === "separator") {
            flushDangerGroup();
            result.push(<DockSeparator key={item.id} compact={props.compact} labeled={props.showLabel} />);
            continue;
        }
        if (item.kind === "switch") {
            flushDangerGroup();
            result.push(<DockSwitch key={item.id} entry={item} {...props} />);
            index += 1;
            continue;
        }
        if (item.danger) {
            dangerGroup.push(item);
            index += 1;
            continue;
        }
        flushDangerGroup();
        result.push(<DockCommandButton key={item.id} command={item} {...props} />);
        index += 1;
    }
    flushDangerGroup();
    return result;
}

function DockCommandButton({ command, metrics, scrollable, compact, showLabel }: DockItemRenderProps & { command: FloatingDockCommand }) {
    const [focused, setFocused] = useState(false);
    const [hovered, setHovered] = useState(false);
    // 鼠标点击产生的 focus 不能阻塞提示收起，只有键盘可见焦点才持续显示提示。
    const showTooltip = !scrollable && (hovered || focused) && !command.disabled && !(command.expands && command.active);
    // scrollable 场景自定义 tooltip 会被 overflow 裁剪，用原生 title 兜底
    const nativeTitle = scrollable ? command.label : undefined;

    if (showLabel) {
        return (
            <span className="relative block h-8 shrink-0">
                <button
                    type="button"
                    aria-label={command.label}
                    aria-expanded={command.expands ? command.active || undefined : undefined}
                    aria-pressed={command.expands ? undefined : command.active || undefined}
                    disabled={command.disabled}
                    className={cn(
                        "aceternity-dock-command is-labeled group inline-flex h-8 items-center justify-center gap-1.5 whitespace-nowrap rounded-[var(--dock-item-radius)] border-0 px-2.5 outline-none",
                        command.active && "is-active",
                        command.danger && "is-danger",
                    )}
                    onClick={command.onClick}
                >
                    <span className="grid size-3.5 shrink-0 place-items-center">{command.icon}</span>
                    <span className="inline-flex h-4 items-center text-[var(--fs-label)] font-medium leading-none">{command.displayLabel || command.label}</span>
                </button>
            </span>
        );
    }

    return (
        <span className={cn("relative block shrink-0", command.wide && "min-w-[var(--dock-precision-width)]")} style={{ width: metrics.base, height: metrics.base }}>
            <button
                type="button"
                aria-label={command.label}
                title={nativeTitle}
                aria-expanded={command.expands ? command.active || undefined : undefined}
                aria-pressed={command.expands ? undefined : command.active || undefined}
                disabled={command.disabled}
                    className={cn("aceternity-dock-command group relative grid size-full place-items-center rounded-[var(--dock-item-radius)] border outline-none", command.quiet && "is-quiet", command.active && "is-active", command.prominent && "is-prominent", command.danger && "is-danger")}
                onMouseEnter={() => setHovered(true)}
                onMouseLeave={() => setHovered(false)}
                onFocus={(event) => setFocused(event.currentTarget.matches(":focus-visible"))}
                onBlur={() => setFocused(false)}
                onMouseDown={() => setFocused(false)}
                onClick={command.onClick}
            >
                <span className={cn("grid place-items-center", command.wide && "w-full")} style={{ width: command.wide ? undefined : metrics.icon, height: metrics.icon }}>
                    {command.icon}
                </span>
                <AnimatePresence>
                    {showTooltip ? (
                        <motion.span
                            {...motionPreset.fade}
                            aria-hidden
                            className={cn(
                                "aceternity-dock-tooltip pointer-events-none absolute left-1/2 z-[var(--dock-tooltip-z)] -translate-x-1/2 whitespace-nowrap border font-medium shadow-xl backdrop-blur-xl",
                                compact ? "-top-7 rounded-md px-1.5 py-0.5 text-[var(--fs-micro)]" : "-top-8 rounded-md px-2 py-1 text-[var(--fs-tiny)]",
                            )}
                        >
                            {command.label}
                        </motion.span>
                    ) : null}
                </AnimatePresence>
            </button>
        </span>
    );
}

function DockSwitch({ entry, compact, showLabel, scrollable, metrics }: DockItemRenderProps & { entry: FloatingDockSwitch }) {
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [focusedId, setFocusedId] = useState<string | null>(null);
    const selectedIndex = Math.max(0, entry.options.findIndex((option) => option.value === entry.value));
    const touch = metrics.base >= 40;
    const slot = touch ? 32 : compact ? 24 : 26;
    const gap = touch ? 10 : compact ? 8 : 10;
    const padX = touch ? 7 : compact ? 6 : 7;

    return (
        <span
            role="radiogroup"
            aria-label={entry.label}
            className="aceternity-dock-switch relative flex shrink-0 self-end items-center"
            onKeyDown={(event) => {
                if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
                event.preventDefault();
                const direction = event.key === "ArrowRight" ? 1 : -1;
                const next = entry.options[(selectedIndex + direction + entry.options.length) % entry.options.length];
                if (next) entry.onChange(next.value);
            }}
        >
            <span
                className="aceternity-dock-switch-track relative inline-flex items-center"
                style={{ gap, padding: `${touch ? 4 : 3}px ${padX}px` }}
            >
                <motion.span
                    aria-hidden
                    className="aceternity-dock-switch-thumb pointer-events-none absolute top-1/2 -translate-y-1/2 rounded-full"
                    initial={false}
                    animate={{ x: selectedIndex * (slot + gap) }}
                    transition={motionSpring.snappy}
                    style={{ width: slot, height: slot, left: padX }}
                />
                {entry.options.map((option) => {
                    const checked = option.value === entry.value;
                    const showTooltip = !showLabel && !scrollable && (hoveredId === option.id || focusedId === option.id);
                    return (
                        <span key={option.id} className="relative">
                            <button
                                type="button"
                                role="radio"
                                aria-checked={checked}
                                aria-label={option.label}
                                title={scrollable ? option.label : undefined}
                                className="aceternity-dock-switch-option relative z-[1] grid place-items-center rounded-full border-0 outline-none"
                                style={{ width: slot, height: slot }}
                                onMouseEnter={() => setHoveredId(option.id)}
                                onMouseLeave={() => setHoveredId((current) => current === option.id ? null : current)}
                                onFocus={(event) => {
                                    if (event.currentTarget.matches(":focus-visible")) setFocusedId(option.id);
                                }}
                                onBlur={() => setFocusedId((current) => current === option.id ? null : current)}
                                onMouseDown={() => setFocusedId(null)}
                                onClick={() => {
                                    if (!checked) entry.onChange(option.value);
                                }}
                            >
                                <span className={cn("grid place-items-center", touch ? "[&_svg]:size-4" : "[&_svg]:size-[15px]")}>{option.icon}</span>
                            </button>
                            <AnimatePresence>
                                {showTooltip ? (
                                    <motion.span
                                        {...motionPreset.fade}
                                        aria-hidden
                                        className={cn(
                                            "aceternity-dock-tooltip pointer-events-none absolute left-1/2 z-[var(--dock-tooltip-z)] -translate-x-1/2 whitespace-nowrap border font-medium shadow-xl backdrop-blur-xl",
                                            compact ? "-top-7 rounded-md px-1.5 py-0.5 text-[var(--fs-micro)]" : "-top-8 rounded-md px-2 py-1 text-[var(--fs-tiny)]",
                                        )}
                                    >
                                        {option.label}
                                    </motion.span>
                                ) : null}
                            </AnimatePresence>
                        </span>
                    );
                })}
            </span>
        </span>
    );
}

function DockSeparator({ compact, labeled }: { compact: boolean; labeled: boolean }) {
    return <span aria-hidden className={cn("aceternity-dock-separator shrink-0 self-center", labeled ? "mx-1.5 h-6 w-px" : compact ? "mx-0.5 mb-0.5 h-3.5 w-px" : "mx-0.5 mb-0.5 h-4 w-px")} />;
}
