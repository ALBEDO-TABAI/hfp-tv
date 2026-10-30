import { Alert, Button, Skeleton } from "antd";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, Navigate, useSearchParams } from "react-router";
import { platformErrorMessage } from "@/lib/platform-account";
import { approvePlatformMarketAuthorization, getPlatformMarketAuthorization } from "@/services/api/platform";
import { usePlatformSession } from "@/stores/use-platform-session";

export default function SkillAuthorizePage() {
    const mode = usePlatformSession((state) => state.mode);
    const [params] = useSearchParams();
    if (mode === "local") return <Navigate to="/skills" replace />;
    return <Authorization key={params.get("code")} code={params.get("code") || ""} />;
}

function Authorization({ code }: { code: string }) {
    const authorization = useQuery({ queryKey: ["platform", "market-authorization", code], queryFn: ({ signal }) => getPlatformMarketAuthorization(code, signal), enabled: Boolean(code), retry: false });
    const approve = useMutation({ mutationFn: () => approvePlatformMarketAuthorization(code) });
    const approved = approve.isSuccess || authorization.data?.status === "approved";
    const error = approve.error || authorization.error;
    return <main className="app-workspace-page h-full overflow-y-auto p-6" style={{ color: "var(--ink)" }}>
        <div className="mx-auto max-w-xl space-y-5">
            <h1 className="text-xl font-medium">连接技能市场</h1>
            {approved ? <Alert type="success" showIcon title="已授权。回到技能页的「投稿」，点「检查授权状态」即可完成连接。" />
                : !code ? <Alert type="warning" title="缺少授权码，请回到投稿页重新连接。" />
                    : error ? <Alert type="warning" title={platformErrorMessage(error)} />
                        : authorization.isPending ? <Skeleton active />
                            : authorization.data && <>
                                <p>请求连接：{authorization.data.clientName}</p>
                                <p>确认码：<strong>{authorization.data.userCode}</strong></p>
                                <p>有效至：{new Date(authorization.data.expiresAt).toLocaleString()}</p>
                                <p>请核对这是你刚刚发起的连接。</p>
                                <Button type="primary" loading={approve.isPending} onClick={() => approve.mutate()}>授权连接</Button>
                            </>}
            <p><Link to="/skills?tab=publish">返回技能投稿</Link></p>
        </div>
    </main>;
}
