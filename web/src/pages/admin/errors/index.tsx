import { Alert, Button, Select, Skeleton } from "antd";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import dayjs from "dayjs";
import { useRef, useState } from "react";
import { AppDrawer } from "@/components/ui/product/app-drawer";
import { platformErrorMessage } from "@/lib/platform-account";
import { listPlatformErrors, listPlatformUsers, type PlatformErrorEvent } from "@/services/api/platform";

const sourceLabels = { task: "任务失败", workspace: "工作区无法打开" };
const time = (value?: string) => value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "—";

function ErrorDetail({ event, onClose }: { event: PlatformErrorEvent; onClose: () => void }) {
    const [copyStatus, setCopyStatus] = useState("");
    const detailRef = useRef<HTMLDivElement>(null);
    const copyTaskId = async () => {
        const value = event.providerTaskId;
        if (!value) return;
        try {
            if (navigator.clipboard?.writeText) {
                try {
                    await navigator.clipboard.writeText(value);
                    setCopyStatus("已复制");
                    return;
                } catch { /* 权限受限时继续使用页面内复制。 */ }
            }
            const textarea = document.createElement("textarea");
            const focused = document.activeElement;
            textarea.value = value;
            textarea.style.cssText = "position:fixed;top:0;left:0;opacity:0;pointer-events:none";
            // 留在抽屉内，避免焦点约束打断 HTTP 页面的复制。
            detailRef.current!.appendChild(textarea);
            try {
                textarea.select();
                if (!document.execCommand("copy")) throw new Error("copy failed");
            } finally {
                textarea.remove();
                if (focused instanceof HTMLElement) focused.focus();
            }
            setCopyStatus("已复制");
        } catch {
            setCopyStatus("复制失败，请手动选择上游任务号复制");
        }
    };
    const duration = event.startedAt ? Math.max(0, Math.round(dayjs(event.createdAt).diff(dayjs(event.startedAt), "second", true))) : undefined;
    const fields = [
        ["记录 ID", event.id], ["时间", time(event.createdAt)],
        ["成员", event.member ? `${event.member.displayName}（${event.member.username}）` : "未知成员"],
        ["成员 ID", event.member?.id], ["工作区 ID", event.workspaceId],
        ["类型", sourceLabels[event.source]], ["任务 ID", event.taskId], ["任务类型", event.taskType],
        ["模型", event.model], ["阶段", event.stage], ["错误类别", event.category],
        ["上游任务号", event.providerTaskId], ["HTTP 状态", event.httpStatus],
        ["上游详情", event.upstreamDetail], ["开始时间", time(event.startedAt)],
        ["耗时", duration === undefined ? "—" : `${duration} 秒`],
    ];

    return <AppDrawer open title="错误详情" size="min(640px, 100vw)" onClose={onClose}>
        <div ref={detailRef} className="min-w-0 space-y-5" style={{ color: "var(--ink)", overflowWrap: "anywhere" }}>
            <dl className="space-y-3">
                {fields.map(([label, value]) => <div key={label}>
                    <dt className="text-sm" style={{ color: "var(--ink-2)" }}>{label}</dt>
                    <dd className="m-0 whitespace-pre-wrap"><span>{value ?? "—"}</span>
                        {label === "上游任务号" && event.providerTaskId && <div className="mt-1 flex flex-wrap items-center gap-2">
                            <Button size="small" onClick={() => void copyTaskId()}>复制上游任务号</Button>
                            <span role="status">{copyStatus}</span>
                        </div>}
                    </dd>
                </div>)}
            </dl>
            <section><h2 className="mb-2 font-medium">完整错误信息</h2><p className="whitespace-pre-wrap">{event.message}</p></section>
            <section><h2 className="mb-2 font-medium">任务日志</h2>
                <pre className="m-0 max-w-full whitespace-pre-wrap rounded-[var(--r-sm)] p-3 text-sm" style={{ fontFamily: "var(--font-mono)", background: "var(--fill)" }}>{event.log?.length ? event.log.join("\n") : "暂无日志"}</pre>
            </section>
        </div>
    </AppDrawer>;
}

export default function AdminErrorsPage() {
    const client = useQueryClient();
    const [member, setMember] = useState("");
    const [source, setSource] = useState<PlatformErrorEvent["source"] | "">("");
    const [selected, setSelected] = useState<PlatformErrorEvent | null>(null);
    const users = useQuery({ queryKey: ["platform", "users"], queryFn: listPlatformUsers });
    const queryKey = ["platform", "errors", member, source];
    const errors = useInfiniteQuery({
        queryKey,
        initialPageParam: "",
        queryFn: ({ pageParam, signal }) => listPlatformErrors({ member: member || undefined, source: source || undefined, before: pageParam || undefined }, signal),
        getNextPageParam: (page) => page.nextBefore || undefined,
        gcTime: 0,
        refetchOnWindowFocus: false,
    });
    const events = errors.data?.pages.flatMap(page => page.events) ?? [];
    const refresh = () => client.resetQueries({ queryKey, exact: true });
    const changeFilter = (nextMember: string, nextSource: typeof source) => {
        void client.resetQueries({ queryKey: ["platform", "errors", nextMember, nextSource], exact: true });
        setMember(nextMember);
        setSource(nextSource);
    };

    return <main className="app-workspace-page flex h-full min-h-0 min-w-0 flex-col overflow-y-auto p-4 sm:p-6" style={{ color: "var(--ink)", overflowWrap: "anywhere" }}>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <div><h1 className="text-xl font-medium">错误记录</h1>
                <p className="mt-2 text-sm" style={{ color: "var(--ink-2)" }}>汇总成员失败的生成任务和无法打开的工作区，按时间从新到旧排列，不包含密钥或结果地址。</p>
            </div>
            <Button loading={errors.isFetching} onClick={() => void refresh()}>刷新</Button>
        </div>
        <div className="mb-4 flex flex-wrap gap-3">
            <Select aria-label="成员" value={member} loading={users.isPending} onChange={value => changeFilter(value, source)} className="w-full min-w-0 sm:w-64" options={[
                { value: "", label: "全部成员" }, ...(users.data?.items.map(user => ({ value: user.id, label: `${user.displayName}（${user.username}）` })) ?? []),
            ]} />
            <Select aria-label="类型" value={source} onChange={value => changeFilter(member, value)} className="w-full min-w-0 sm:w-52" options={[
                { value: "", label: "全部" }, { value: "task", label: sourceLabels.task }, { value: "workspace", label: sourceLabels.workspace },
            ]} />
        </div>
        {users.isError && <Alert type="error" showIcon title="成员列表加载失败" action={<Button onClick={() => void users.refetch()}>重试成员列表</Button>} className="mb-4" />}
        {errors.isError && <Alert type="error" showIcon title={platformErrorMessage(errors.error)} action={<Button onClick={() => void (errors.isFetchNextPageError ? errors.fetchNextPage() : refresh())}>重试</Button>} className="mb-4" />}
        {errors.isPending ? <Skeleton active /> : errors.data && <>
            {events.length ? <ul className="m-0 min-w-0 list-none space-y-2 p-0">
                {events.map(event => <li key={event.id}>
                    <button type="button" onClick={() => setSelected(event)} className="w-full min-w-0 cursor-pointer rounded-[var(--r-md)] border border-[var(--hair)] p-4 text-left hover:bg-[var(--fill)]">
                        <span className="flex min-w-0 flex-wrap gap-x-4 gap-y-1 text-sm" style={{ color: "var(--ink-2)" }}>
                            <time dateTime={event.createdAt}>{time(event.createdAt)}</time>
                            <span>{event.member?.displayName ?? "未知成员"}</span><span>{sourceLabels[event.source]}</span>
                            <span>{event.model || event.taskType || "—"}</span>
                        </span>
                        <span className="mt-2 line-clamp-2">{event.message}</span>
                    </button>
                </li>)}
            </ul> : <div className="py-12 text-center"><p>暂无错误记录</p><p className="mt-2 text-sm" style={{ color: "var(--ink-2)" }}>当前筛选下没有记录，可调整筛选或稍后刷新。</p></div>}
            {errors.hasNextPage && <div className="mt-4 text-center"><Button loading={errors.isFetchingNextPage} disabled={errors.isFetching && !errors.isFetchingNextPage} onClick={() => void errors.fetchNextPage()}>加载更多</Button></div>}
        </>}
        {selected && <ErrorDetail key={selected.id} event={selected} onClose={() => setSelected(null)} />}
    </main>;
}
