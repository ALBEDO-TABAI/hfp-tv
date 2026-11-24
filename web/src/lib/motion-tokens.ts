/**
 * 动效语言的脚本侧镜像，给 motion/react 和 Web Animations 用。
 *
 * 数值与 globals.css 第 7 节的 CSS 变量一一对应（这里单位是秒），规则见 docs/DESIGN.md 第 4 节：
 *   press   0.08s = --motion-press     按下那一下
 *   instant 0.12s = --motion-instant   悬停、颜色与透明度反馈；小元素离开
 *   state   0.18s = --motion-state     控件状态、图标互换、提示与气泡出现、松开回弹；浮层离开
 *   panel   0.24s = --motion-panel     菜单、弹窗、抽屉、浮层面板出现；展开收起；列表增删
 *   stage   0.32s = --motion-stage     侧栏折叠、程序化镜头、主题切换
 *
 * 曲线：进入、移动和纯淡出用 out；带位移的离场用 in；非用户直接触发、起止都在画面里的移动用 inOut。
 * 会被反复打断的位置和尺寸用弹簧，弹簧从当前速度接着走，不会在中途重新起步。
 *
 * 系统「减少动态效果」和设置里的「流畅模式」由根部的 MotionConfig 统一处理（见 app-providers）：
 * 组件里读减弱状态用 useReducedMotionConfig，不用只认系统设置的 useReducedMotion。
 */
export const motionDuration = {
    press: 0.08,
    instant: 0.12,
    state: 0.18,
    panel: 0.24,
    stage: 0.32,
} as const;

export const motionEase = {
    out: [0.22, 1, 0.36, 1],
    in: [0.32, 0, 0.67, 0],
    inOut: [0.65, 0, 0.35, 1],
} as const;

export const motionSpring = {
    /** 指示条、导航胶囊、跟随指针的数值：快，不回弹。 */
    snappy: { type: "spring", visualDuration: 0.18, bounce: 0 },
    /** 浮层面板、布局补位：不回弹。 */
    smooth: { type: "spring", visualDuration: 0.24, bounce: 0 },
    /** 拖放落位、强调：约 2.5% 的轻回弹，和 CSS 的 --ease-spring 同感。 */
    settle: { type: "spring", visualDuration: 0.32, bounce: 0.25 },
} as const;

/** useSpring 只收弹簧参数，不带 type。 */
export const motionSpringValue = {
    snappy: { visualDuration: motionSpring.snappy.visualDuration, bounce: motionSpring.snappy.bounce },
    smooth: { visualDuration: motionSpring.smooth.visualDuration, bounce: motionSpring.smooth.bounce },
} as const;

export const motionTransition = {
    /** 浮层、菜单、提示出现。 */
    enter: { duration: motionDuration.state, ease: motionEase.out },
    /** 浮层、菜单、提示离开：比出现快一档，纯淡出也用 out，第一帧就有反应。 */
    exit: { duration: motionDuration.instant, ease: motionEase.out },
    /** 面板、弹窗出现。 */
    panelEnter: { duration: motionDuration.panel, ease: motionEase.out },
    /** 面板、弹窗离开。 */
    panelExit: { duration: motionDuration.state, ease: motionEase.out },
} as const;

/**
 * 画布和工作区里自绘浮层的统一进出（下拉、弹出面板、Dock 弹层）：
 * 朝锚点方向移近 4px、轻微放大并淡入；离开只淡出。用法：
 *   <motion.div {...motionPreset.popover("above")} />
 * placement 是浮层相对锚点的位置：浮层在锚点上方时从下往上出现。
 */
export const motionPreset = {
    popover: (placement: "above" | "below" = "below") => ({
        initial: { opacity: 0, y: placement === "above" ? 4 : -4, scale: 0.98 },
        animate: { opacity: 1, y: 0, scale: 1, transition: motionTransition.enter },
        exit: { opacity: 0, transition: motionTransition.exit },
    }),
    /** 常驻在画布上的工具条、提示条首次出现；离开只淡出。 */
    bar: {
        initial: { opacity: 0, y: 4 },
        animate: { opacity: 1, y: 0, transition: motionTransition.enter },
        exit: { opacity: 0, transition: motionTransition.exit },
    },
    /** 只淡入淡出：反复出现的东西、减弱模式下的退化形态。 */
    fade: {
        initial: { opacity: 0 },
        animate: { opacity: 1, transition: motionTransition.enter },
        exit: { opacity: 0, transition: motionTransition.exit },
    },
} as const;

const reducedMotionQuery = typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia("(prefers-reduced-motion: reduce)") : null;

/**
 * 现在是否该减弱动效：每次实时读取系统「减少动态效果」和流畅模式（html[data-effects="lite"]），不缓存。
 * 脚本里临时起的动画（程序化镜头、按下反馈）开始前和逐帧都用它判断，运行中切换偏好也立即生效。
 */
export function prefersReducedMotionNow(): boolean {
    if (typeof document !== "undefined" && document.documentElement.dataset.effects === "lite") return true;
    return reducedMotionQuery?.matches === true;
}
