import { useIsPresent } from "motion/react";
import { cloneElement, type ReactElement } from "react";

/**
 * 放在 AnimatePresence 里、包住要离场的浮层：离场动画播放期间给它加 inert，
 * 点击和键盘（Tab、回车）都不再落到正在消失的内容上。离场动画照常由里面的 motion 元素负责。
 *   <AnimatePresence>{open ? <PresenceInert key="menu"><motion.div {...motionPreset.popover()} /></PresenceInert> : null}</AnimatePresence>
 */
export function PresenceInert({ children }: { children: ReactElement<{ inert?: boolean }> }) {
    const isPresent = useIsPresent();
    return cloneElement(children, { inert: !isPresent || undefined });
}
