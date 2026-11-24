import { Drawer, type DrawerProps } from "antd";
import type { CSSProperties, ReactNode } from "react";

import "./app-drawer.css";

// 用途：产品侧栏壳。消费 token 由内容自己负责。默认 destroyOnHidden，可选 flush 去内边距。
export type AppDrawerProps = DrawerProps & {
    flush?: boolean;
};

export function AppDrawer({ flush = false, destroyOnHidden = true, styles, className, rootClassName, drawerRender, ...props }: AppDrawerProps) {
    // 关闭后面板还要滑出约 240ms：这段时间整个面板设为 inert，点击和键盘都不再生效。
    // 外层是 display: contents 的 div，不改变面板布局。
    const open = props.open;
    const renderPanel = (panel: ReactNode) => <div className="app-drawer-interaction" inert={!open}>{drawerRender ? drawerRender(panel) : panel}</div>;
    const mergeResolvedStyles = (resolved: unknown) => {
        const base = resolved && typeof resolved === "object" ? (resolved as Record<string, unknown>) : {};
        if (!flush) return base;
        const body = base.body && typeof base.body === "object" ? (base.body as CSSProperties) : {};
        return { ...base, body: { ...body, padding: 0 } };
    };
    const mergedStyles =
        typeof styles === "function"
            ? (info: { props: DrawerProps }) => mergeResolvedStyles(styles(info))
            : mergeResolvedStyles(styles);

    return <Drawer destroyOnHidden={destroyOnHidden} className={className} rootClassName={rootClassName ? `app-drawer-root ${rootClassName}` : "app-drawer-root"} {...props} drawerRender={renderPanel} styles={mergedStyles as DrawerProps["styles"]} />;
}
