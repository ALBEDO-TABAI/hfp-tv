import { Modal, type ModalProps } from "antd";
import type { CSSProperties } from "react";

import { APP_MODAL_TRANSITION } from "./app-modal-motion";

// 用途：产品弹窗壳。消费 token 由内容自己负责。接管 AntD Modal 的重复 flush 内边距。
const FLUSH_STYLES = {
    container: { padding: 0, overflow: "hidden" },
    body: { padding: 0 },
} as const;

export type AppModalProps = ModalProps & {
    /** 内容自带外壳时去掉 AntD 默认内边距，避免每个业务弹窗复制同一套 styles。 */
    flush?: boolean;
};

export function AppModal({ flush = false, destroyOnHidden = true, centered = true, transitionName = APP_MODAL_TRANSITION, styles, ...props }: AppModalProps) {
    const mergeResolvedStyles = (resolved: unknown) => {
        const base = resolved && typeof resolved === "object" ? (resolved as Record<string, unknown>) : {};
        if (!flush) return base;
        const container = base.container && typeof base.container === "object" ? (base.container as CSSProperties) : {};
        const body = base.body && typeof base.body === "object" ? (base.body as CSSProperties) : {};
        return {
            ...base,
            container: { ...container, ...FLUSH_STYLES.container },
            body: { ...body, ...FLUSH_STYLES.body },
        };
    };
    const mergedStyles =
        typeof styles === "function"
            ? (info: { props: ModalProps }) => mergeResolvedStyles(styles(info))
            : mergeResolvedStyles(styles);

    // 关闭后离场还要播约 180ms：这段时间整个弹窗设为 inert，点击和键盘（例如焦点还停在「删除」上时按 Enter）都不再生效。
    // 外壳 inert 后不再接住点击，由遮罩接住（app-modal.css），点击不会穿透到背后的页面。
    const wrapProps = { ...props.wrapProps, inert: props.open ? undefined : true };
    const rootClassName = props.rootClassName ? `app-modal-root ${props.rootClassName}` : "app-modal-root";
    return <Modal destroyOnHidden={destroyOnHidden} centered={centered} transitionName={transitionName} {...props} rootClassName={rootClassName} wrapProps={wrapProps} styles={mergedStyles as ModalProps["styles"]} />;
}
