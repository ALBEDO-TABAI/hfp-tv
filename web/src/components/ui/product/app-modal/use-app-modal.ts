import { App } from "antd";

import { APP_MODAL_TRANSITION } from "./app-modal-motion";

type ModalApi = ReturnType<typeof App.useApp>["modal"];
type ModalOpener = ModalApi["confirm"];
type ModalConfig = Parameters<ModalOpener>[0];
type Handler = (...args: unknown[]) => unknown;

// 按 AntD 给的 modal 对象缓存包装结果：同一个 modal 永远拿到同一个包装对象，
// 依赖它的回调不会每次渲染都变；也不额外调用 React hook。
const wrappedModals = new WeakMap<ModalApi, ModalApi>();

/** AntD 按处理函数的参数个数决定是否传入 close：包装后保留原来的参数个数。 */
function keepArity(original: Handler, run: (close?: unknown) => unknown): Handler {
    return original.length > 0 ? (close) => run(close) : () => run();
}

const withMotion = (open: ModalOpener): ModalOpener => (config: ModalConfig) => {
    // 取消（按钮、Esc、点遮罩）之后离场还要播一会儿，焦点可能还停在确认按钮上：这时按回车不再执行确认。
    let dismissed = false;
    const onOk = config.onOk as Handler | undefined;
    const onCancel = config.onCancel as Handler | undefined;
    return open({
        transitionName: APP_MODAL_TRANSITION,
        ...config,
        onOk: onOk ? keepArity(onOk, (close) => (dismissed ? undefined : close === undefined ? onOk() : onOk(close))) : undefined,
        onCancel: onCancel
            ? keepArity(onCancel, (close) => {
                dismissed = true;
                return close === undefined ? onCancel() : onCancel(close);
            })
            : () => {
                dismissed = true;
            },
        afterClose: () => {
            dismissed = true;
            config.afterClose?.();
        },
    });
};

/**
 * 命令式确认框（confirm / info / success / error / warning）和 AppModal 用同一套进出场：
 * 朝点击处轻微放大淡入，而不是 AntD 默认的从 20% 弹出。用法同 App.useApp().modal。
 */
export function useAppModal(): ModalApi {
    const { modal } = App.useApp();
    let wrapped = wrappedModals.get(modal);
    if (!wrapped) {
        wrapped = {
            ...modal,
            confirm: withMotion(modal.confirm),
            info: withMotion(modal.info),
            success: withMotion(modal.success),
            error: withMotion(modal.error),
            warning: withMotion(modal.warning),
        };
        wrappedModals.set(modal, wrapped);
    }
    return wrapped;
}
