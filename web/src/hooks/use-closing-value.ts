import { useState } from "react";

/**
 * 「有值才渲染」的弹窗和抽屉关闭时也要播完离场：值清空后先保留上一次的内容、把 open 设成 false，
 * 等 AntD 的 afterClose（抽屉是 afterOpenChange(false)）回调后再卸载。
 * 每次从关闭到打开（包括离场途中又打开）key 都加一，组件内部状态（例如请求幂等键、步骤）从头开始；
 * 打开期间值从 A 换成 B 不换 key，需要时由调用方自己按值设 key。
 */
export function useClosingValue<T>(value: T | null | undefined | false) {
    const current = value === false || value == null ? null : value;
    const [state, setState] = useState<{ shown: T | null; open: boolean; key: number }>({ shown: current, open: current !== null, key: current === null ? 0 : 1 });
    if (current !== null && (current !== state.shown || !state.open)) {
        setState({ shown: current, open: true, key: state.open ? state.key : state.key + 1 });
    } else if (current === null && state.open) {
        setState({ ...state, open: false });
    }
    return {
        shown: current ?? state.shown,
        open: current !== null,
        key: state.key,
        // 已经重新打开时不清空：旧实例迟到的回调不会把新打开的内容卸掉。
        afterClose: () => setState((latest) => (latest.open ? latest : { ...latest, shown: null })),
    };
}
