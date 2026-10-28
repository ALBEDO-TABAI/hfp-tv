import { expect, test } from "bun:test";
import { renderToString } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router";
import AuthPage from "../src/pages/auth";
import { TeamAccountMenu } from "../src/components/layout/team-account-menu";
import { WorkspaceAccountCard } from "../src/components/layout/workspace-account-card";
import { useUserStore } from "../src/stores/use-user-store";
import { usePlatformIdentityStore } from "../src/stores/use-platform-identity-store";

test("login and invitation forms render before workspace startup", () => {
    for (const [path, title] of [["/login", "登录工作区"], ["/invite/test-token", "设置密码"]]) {
        const markup = renderToString(<MemoryRouter initialEntries={[path]}><Routes>
            <Route path="/login" element={<AuthPage />} />
            <Route path="/invite/:token" element={<AuthPage />} />
        </Routes></MemoryRouter>);
        expect(markup).toContain(title);
        expect(markup).toContain("用户名");
        expect(markup).toContain('type="password"');
        if (path.startsWith("/invite/")) {
            expect(markup).toContain("显示名");
            expect(markup).toContain("再次输入密码");
        }
    }
});

test("team account UI shows the platform name without labelling the kernel owner a platform admin", () => {
    const previousMode = process.env.VITE_DEPLOYMENT_MODE;
    // Server rendering reads Zustand's initial snapshot rather than live state.
    const kernel = useUserStore.getInitialState();
    const platform = usePlatformIdentityStore.getInitialState();
    const previousUser = kernel.user;
    const previousIdentity = platform.identity;
    kernel.user = {
        id: "local", username: "local", displayName: "本地工作区", role: "admin", status: "active",
    };
    platform.identity = {
        user: { id: "member-a", username: "platform-member", display_name: "平台成员", role: "member", status: "active", last_active_at: null, created_at: "" },
        workspace_id: "workspace-a",
    };
    try {
        process.env.VITE_DEPLOYMENT_MODE = "team";
        const menu = renderToString(<TeamAccountMenu collapsed={false} />);
        expect(menu).toContain("平台成员");
        expect(menu).not.toContain("本地工作区");
        const card = () => renderToString(<MemoryRouter><WorkspaceAccountCard onNavigate={() => undefined} /></MemoryRouter>);
        expect(card()).not.toContain("管理员");
        process.env.VITE_DEPLOYMENT_MODE = "local";
        expect(card()).toContain("管理员");
    } finally {
        if (previousMode === undefined) delete process.env.VITE_DEPLOYMENT_MODE;
        else process.env.VITE_DEPLOYMENT_MODE = previousMode;
        kernel.user = previousUser;
        platform.identity = previousIdentity;
    }
});
