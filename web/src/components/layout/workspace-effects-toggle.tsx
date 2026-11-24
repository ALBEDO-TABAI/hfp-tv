import { Gauge } from "lucide-react";

import { cn } from "@/lib/utils";
import { useThemeStore } from "@/stores/use-theme-store";

/** 流畅模式的说明，侧栏按钮的提示和画布「画布外观」里的开关共用。 */
export const FLUID_MODE_COPY = {
    name: "流畅模式",
    description: "关掉毛玻璃、光影和装饰动画，画布和长列表更跟手",
    short: "关掉毛玻璃和装饰动画",
    turnOn: "开启流畅模式",
    turnOff: "关闭流畅模式",
} as const;

/** 和昼夜切换并列的「流畅模式」开关，持久状态用 aria-pressed 表达。 */
export function WorkspaceEffectsToggle({ className }: { className?: string }) {
    const lite = useThemeStore((state) => state.effects === "lite");
    const setEffects = useThemeStore((state) => state.setEffects);
    return (
        <button
            type="button"
            className={cn("app-workspace-theme-action app-workspace-effects-action", className)}
            aria-pressed={lite}
            aria-label={lite ? FLUID_MODE_COPY.turnOff : FLUID_MODE_COPY.turnOn}
            title={`${FLUID_MODE_COPY.name}：${FLUID_MODE_COPY.description}`}
            onClick={() => setEffects(lite ? "full" : "lite")}
        >
            <Gauge className="size-4" />
        </button>
    );
}
