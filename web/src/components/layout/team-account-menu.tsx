import { useState } from "react";
import { App, Dropdown } from "antd";
import { ChevronDown, CircleUserRound, LogOut } from "lucide-react";
import { usePlatformIdentityStore } from "@/stores/use-platform-identity-store";
import { logoutPlatform } from "@/services/api/platform";
import { leaveTeamSession, notifyTeamSessionChanged } from "@/services/team-session";

export function TeamAccountMenu({ collapsed }: { collapsed: boolean }) {
    const user = usePlatformIdentityStore((state) => state.identity?.user);
    const [busy, setBusy] = useState(false);
    const { message } = App.useApp();
    if (!user) return null;

    const logout = async () => {
        setBusy(true);
        try {
            await logoutPlatform();
            notifyTeamSessionChanged();
            leaveTeamSession(false);
        } catch {
            message.error("退出登录失败，请稍后重试");
            setBusy(false);
        }
    };

    return <Dropdown trigger={["click"]} placement="topLeft" menu={{ items: [{ key: "logout", label: "退出登录", icon: <LogOut className="size-4" />, disabled: busy }], onClick: () => void logout() }}>
        <button type="button" disabled={busy} aria-label="账号菜单" title={user.display_name || user.username} className="flex w-full items-center gap-2 rounded-[var(--r-md)] p-2 text-[var(--ink)] hover:bg-[var(--fill-2)]">
            <CircleUserRound className="size-7 shrink-0" aria-hidden />
            {!collapsed ? <><span className="min-w-0 flex-1 truncate text-left">{user.display_name || user.username}</span><ChevronDown className="size-4 shrink-0" /></> : null}
        </button>
    </Dropdown>;
}
