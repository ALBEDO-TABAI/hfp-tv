import { create } from "zustand";
import { persist } from "zustand/middleware";

export type ThemeName = "light" | "dark";
/** full：完整特效；lite：流畅模式，关掉毛玻璃、追光和装饰循环，脚本动画只保留淡入淡出。 */
export type EffectsLevel = "full" | "lite";

export function normalizeTheme(value: unknown): ThemeName {
    return value === "light" ? "light" : "dark";
}

export function normalizeEffects(value: unknown): EffectsLevel {
    return value === "lite" ? "lite" : "full";
}

type ThemeStore = {
    theme: ThemeName;
    effects: EffectsLevel;
    setTheme: (theme?: string) => void;
    setEffects: (effects?: string) => void;
};

export const useThemeStore = create<ThemeStore>()(
    persist(
        (set) => ({
            theme: "dark",
            effects: "full",
            setTheme: (theme) => set({ theme: normalizeTheme(theme) }),
            setEffects: (effects) => set({ effects: normalizeEffects(effects) }),
        }),
        {
            name: "infinite-canvas:theme_store",
            // 持久化恢复校验：旧版本/坏 session 写入的非法值回退到默认，
            // 避免 canvasThemes[非法值] = undefined 触发 "reading 'node'" 崩溃
            merge: (persisted, current) => {
                const saved = persisted as Partial<ThemeStore> | null;
                return { ...current, theme: normalizeTheme(saved?.theme), effects: normalizeEffects(saved?.effects) };
            },
        },
    ),
);
