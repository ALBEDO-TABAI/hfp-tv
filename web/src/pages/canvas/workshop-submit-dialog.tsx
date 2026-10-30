import { useState } from "react";
import { Alert, App, Button, Checkbox, Input, Select, Skeleton, Space, Steps, Switch } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { AppModal } from "@/components/ui/product/app-modal";
import { WorkshopOrigin } from "@/components/workshop/workshop-card";
import { platformErrorMessage } from "@/lib/platform-account";
import { ApiError } from "@/services/api/request";
import { resourceFileUrl } from "@/services/api/resources";
import { getWorkshopOrigin, preflightWorkshop, submitWorkshop, workshopQueryKey } from "@/services/api/workshop";

export function WorkshopSubmitDialog({ projectId, onClose }: { projectId: string; onClose: () => void }) {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [step, setStep] = useState(0);
    const [title, setTitle] = useState<string>();
    const [summary, setSummary] = useState("");
    const [tags, setTags] = useState<string[]>([]);
    const [coverResourceId, setCoverResourceId] = useState<string>();
    const [allowDownload, setAllowDownload] = useState(false);
    const [rights, setRights] = useState(false);
    const preflight = useQuery({ queryKey: [...workshopQueryKey, "preflight", projectId], queryFn: ({ signal }) => preflightWorkshop(projectId, signal), staleTime: 0, refetchOnMount: "always", refetchOnWindowFocus: false });
    const origin = useQuery({ queryKey: [...workshopQueryKey, "origin", projectId], queryFn: async ({ signal }) => {
        try { return await getWorkshopOrigin(projectId, signal); }
        catch (error) { if (error instanceof ApiError && error.reason === "origin_not_found") return null; throw error; }
    } });
    const workTitle = title ?? preflight.data?.title ?? "";
    const submit = useMutation({
        retry: false,
        mutationFn: () => {
            if (!rights || !workTitle.trim() || tags.length > 8) throw new Error("请完成作品信息和授权确认");
            return submitWorkshop({ projectId, title: workTitle.trim(), summary, tags, coverResourceId, allowDownload, rightsConfirmed: true });
        },
        onSuccess: () => { void client.invalidateQueries({ queryKey: workshopQueryKey }); },
        onError: async (error) => {
            message.error(platformErrorMessage(error));
            if (error instanceof ApiError && error.reason === "preflight_blocked") {
                setStep(0); setCoverResourceId(undefined); setRights(false);
                await preflight.refetch();
            }
        },
    });
    const blocked = !preflight.data || preflight.isError || preflight.isFetching || preflight.data.issues.some((issue) => issue.level === "error");
    return <AppModal flush open width={720} footer={null} onCancel={() => { if (!submit.isPending) onClose(); }} maskClosable={!submit.isPending} keyboard={!submit.isPending} closable={!submit.isPending}>
        <div className="space-y-5 p-6">
            <h2 className="text-lg font-medium">投稿到创意工坊</h2>
            {submit.isSuccess ? <div className="space-y-4"><p role="status">已提交，等待审核</p><Space><Link to="/workshop?tab=mine" onClick={onClose}>查看我的投稿 →</Link><Button onClick={onClose}>完成</Button></Space></div> : <>
                <Steps size="small" current={step} items={[{ title: "内容检查" }, { title: "作品信息" }, { title: "确认公开内容" }]} />
                <div className="max-h-[60vh] space-y-4 overflow-y-auto py-2">
                    {step === 0 && <>
                        {preflight.isFetching ? <Skeleton active /> : preflight.isError ? <Alert type="error" title={platformErrorMessage(preflight.error)} /> : preflight.data && <>
                            <p>{preflight.data.issues.some((issue) => issue.level === "error") ? "待处理" : "✓"} {preflight.data.stats.canvases} 张画布 · {preflight.data.stats.nodes} 个节点 · {preflight.data.stats.files} 个文件（{(preflight.data.stats.bytes / 1024 / 1024).toFixed(1)} MB）</p>
                            {preflight.data.issues.map((issue, index) => <Alert key={`${issue.code}:${index}`} type={issue.level === "error" ? "error" : "warning"} title={issue.message} action={issue.canvasId ? <Link to={`/canvas/${issue.canvasId}`} target="_blank" rel="noreferrer">定位</Link> : undefined} />)}
                        </>}
                        <Button loading={preflight.isFetching} onClick={() => void preflight.refetch()}>重新检查</Button>
                    </>}
                    {step === 1 && <>
                        <label className="block space-y-2"><span>标题</span><Input value={workTitle} onChange={(event) => setTitle(event.target.value)} placeholder="为作品起个名字" /></label>
                        <label className="block space-y-2"><span>简介</span><Input.TextArea value={summary} onChange={(event) => setSummary(event.target.value)} rows={3} /></label>
                        <label className="block space-y-2"><span>标签（最多 8 个）</span><Select className="w-full" mode="tags" value={tags} onChange={setTags} maxCount={8} tokenSeparators={[",", "，"]} /></label>
                        <fieldset className="space-y-2"><legend className="mb-2">封面</legend>
                            <Button aria-pressed={!coverResourceId} onClick={() => setCoverResourceId(undefined)}>不指定封面</Button>
                            <div className="grid grid-cols-3 gap-2">{preflight.data?.covers.map((cover) => <button key={`${cover.resourceId}:${cover.nodeId}`} type="button" aria-pressed={coverResourceId === cover.resourceId} className={`overflow-hidden rounded-[var(--r-sm)] border p-1 focus-visible:outline-2 focus-visible:outline-ring ${coverResourceId === cover.resourceId ? "border-primary bg-primary/10" : "border-border"}`} onClick={() => setCoverResourceId(cover.resourceId)}>
                                <img src={resourceFileUrl(cover.resourceId)} alt={cover.title || "作品封面"} className="aspect-video w-full object-contain" /><span className="block truncate text-xs">{cover.title}</span>
                            </button>)}</div>
                        </fieldset>
                        <label className="flex items-center gap-3"><Switch checked={allowDownload} onChange={setAllowDownload} />允许下载原文件</label>
                        {origin.isError ? <Alert type="error" title={platformErrorMessage(origin.error)} action={<Button onClick={() => void origin.refetch()}>重试来源查询</Button>} /> : origin.data && <WorkshopOrigin origin={origin.data} />}
                    </>}
                    {step === 2 && <>
                        <div className="grid gap-4 sm:grid-cols-2"><section><h3 className="mb-2 font-medium">会公开</h3><ul className="list-inside list-disc"><li>提示词</li><li>媒体文件</li><li>时间线</li><li>画板</li></ul></section><section><h3 className="mb-2 font-medium">不会公开</h3><ul className="list-inside list-disc"><li>助手对话</li><li>生成记录</li><li>模型密钥</li></ul></section></div>
                        <Checkbox checked={rights} disabled={submit.isPending} onChange={(event) => setRights(event.target.checked)}>我拥有或有权分享这些素材</Checkbox>
                        {submit.isError && <Alert type="error" title={platformErrorMessage(submit.error)} />}
                    </>}
                </div>
                <Space>{step > 0 && <Button disabled={submit.isPending} onClick={() => setStep(step - 1)}>上一步</Button>}
                    {step < 2 ? <Button type="primary" disabled={step === 0 ? blocked : !workTitle.trim() || tags.length > 8 || origin.isPending || origin.isError} onClick={() => setStep(step + 1)}>下一步</Button>
                        : <Button type="primary" loading={submit.isPending} disabled={!rights} onClick={() => submit.mutate()}>提交审核</Button>}
                </Space>
            </>}
        </div>
    </AppModal>;
}
