import { useState } from "react";

/**
 * 弹窗、抽屉第一次打开后保持挂载：关闭时组件还在，才能播放离开动效，而不是瞬间消失。
 * 从没打开过的不挂载，按需加载的代码也就不会提前下载。
 */
export function useOpenedOnce(open: boolean) {
    const [opened, setOpened] = useState(open);
    if (open && !opened) setOpened(true);
    return opened || open;
}
