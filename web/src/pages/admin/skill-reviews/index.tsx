import { useState } from "react";
import { Alert, App, Button, Form, Input, Popconfirm, Select, Skeleton, Space, Table, Tabs } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AppDrawer } from "@/components/ui/product/app-drawer";
import { AppModal } from "@/components/ui/product/app-modal";
import { platformErrorMessage } from "@/lib/platform-account";
import { diffMarketFiles } from "@/lib/skill-market";
import { requirementLabel } from "@/pages/skills/market-copy";
import { ApiError } from "@/services/api/request";
import {
    approveSkillSubmission, getSkillSubmission, getSkillSubmissionFile, listAdminMarketSkills,
    listSkillSubmissions, rejectSkillSubmission, relistMarketSkill, unlistMarketSkill,
    type MarketSkillSummary, type SkillSubmissionSummary,
} from "@/services/api/platform";

const submissionsKey = ["platform", "skill-submissions"];
const skillsKey = ["platform", "market-skills"];
const time = (value: string | null) => value ? new Date(value).toLocaleString() : "—";
const statuses = { draft: "草稿", ready_to_submit: "待送审", in_review: "待审核", published: "已发布", rejected: "已驳回" };
const changes = { added: "新增", modified: "修改", deleted: "删除", unchanged: "未变" };
const reasons = ["素材或内容授权不明", "内容不完整或无法使用", "与已有技能重复", "存在安全风险的指令"];

export default function AdminSkillReviewsPage() {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [tab, setTab] = useState("pending");
    const [selected, setSelected] = useState<string | null>(null);
    const status = tab === "processed" ? "processed" : "pending";
    const submissions = useQuery({ queryKey: [...submissionsKey, status], queryFn: ({ signal }) => listSkillSubmissions(status, signal), enabled: tab !== "skills" });
    const skills = useQuery({ queryKey: skillsKey, queryFn: ({ signal }) => listAdminMarketSkills(signal), enabled: tab === "skills" });
    const listing = useMutation({
        mutationFn: (skill: MarketSkillSummary) => skill.listed ? unlistMarketSkill(skill.skillId) : relistMarketSkill(skill.skillId),
        onSuccess: async () => { message.success("已保存"); await client.invalidateQueries({ queryKey: skillsKey }); },
        onError: (error) => message.error(platformErrorMessage(error)),
    });
    return <main className="app-workspace-page flex h-full min-h-0 flex-col overflow-y-auto p-6" style={{ color: "var(--ink)" }}>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <h1 className="text-xl font-medium">技能审核</h1>
            <Button onClick={() => void (tab === "skills" ? skills.refetch() : submissions.refetch())}>刷新</Button>
        </div>
        <Tabs activeKey={tab} onChange={setTab} items={[{ key: "pending", label: "待审核" }, { key: "processed", label: "已处理" }, { key: "skills", label: "已发布技能" }]} />
        {tab === "skills" ? <>
            {skills.isError && <Alert type="error" showIcon title={platformErrorMessage(skills.error)} className="mb-4" />}
            <Table<MarketSkillSummary> rowKey="skillId" dataSource={skills.data?.items} loading={skills.isFetching} scroll={{ x: 900 }} pagination={{ pageSize: 20 }} columns={[
                { title: "名称", dataIndex: "name" },
                { title: "作者", dataIndex: "ownerUsername" },
                { title: "版本", dataIndex: "versionLabel" },
                { title: "发布时间", dataIndex: "publishedAt", render: time },
                { title: "官方", key: "official", render: (_, skill) => skill.official ? "是" : "否" },
                { title: "状态", key: "listed", render: (_, skill) => skill.listed ? "已上架" : "已下架" },
                { title: "操作", key: "actions", render: (_, skill) => <Popconfirm title={`确定${skill.listed ? "下架" : "重新上架"}“${skill.name}”？`} onConfirm={() => listing.mutate(skill)}>
                    <Button size="small" disabled={listing.isPending}>{skill.listed ? "下架" : "重新上架"}</Button>
                </Popconfirm> },
            ]} />
        </> : <>
            {submissions.isError && <Alert type="error" showIcon title={platformErrorMessage(submissions.error)} className="mb-4" />}
            <Table<SkillSubmissionSummary> rowKey="id" dataSource={submissions.data?.items} loading={submissions.isFetching} scroll={{ x: 1000 }} pagination={{ pageSize: 20 }} onRow={(submission) => ({ onClick: () => setSelected(submission.id) })} columns={[
                { title: "标题", key: "name", render: (_, submission) => <Button type="link" onClick={() => setSelected(submission.id)}>{submission.metadata.name}</Button> },
                { title: "作者", key: "author", render: (_, submission) => `${submission.author.displayName} (${submission.author.username})` },
                { title: "投稿目标", key: "target", render: (_, submission) => submission.target === "new" ? "新技能" : `更新 · ${submission.currentSkillName || submission.skillId}` },
                { title: "版本", key: "version", render: (_, submission) => submission.metadata.versionLabel },
                { title: "投稿时间", dataIndex: "submittedAt", render: time },
                { title: "状态", key: "status", render: (_, submission) => statuses[submission.status] },
                { title: "审核意见", dataIndex: "reviewReason" },
            ]} />
        </>}
        {selected && <SubmissionDrawer key={selected} id={selected} onClose={() => setSelected(null)} />}
    </main>;
}

function SubmissionDrawer({ id, onClose }: { id: string; onClose: () => void }) {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [path, setPath] = useState<string | null>(null);
    const [rejecting, setRejecting] = useState<SkillSubmissionSummary | null>(null);
    const [conflict, setConflict] = useState(false);
    const [form] = Form.useForm<{ preset?: string; reason?: string }>();
    const detailKey = [...submissionsKey, "detail", id];
    const detail = useQuery({ queryKey: detailKey, queryFn: ({ signal }) => getSkillSubmission(id, signal), refetchOnWindowFocus: false });
    const submission = detail.data?.submission;
    const file = useQuery({
        queryKey: [...detailKey, "file", submission?.revision, submission?.previewDigest, path],
        queryFn: ({ signal }) => {
            if (!path) throw new Error("请选择文件");
            return getSkillSubmissionFile(id, path, signal);
        },
        enabled: Boolean(path && submission) && !detail.isFetching && !detail.isError,
        refetchOnWindowFocus: false,
    });
    type Review = { submission: SkillSubmissionSummary } & ({ kind: "approve"; official: boolean } | { kind: "reject"; reason: string });
    const review = useMutation({
        mutationFn: (input: Review) => input.kind === "approve"
            ? approveSkillSubmission(input.submission.id, { revision: input.submission.revision, previewDigest: input.submission.previewDigest, official: input.official })
            : rejectSkillSubmission(input.submission.id, { revision: input.submission.revision, reason: input.reason }),
        onSuccess: async () => {
            setRejecting(null); form.resetFields(); setConflict(false); setPath(null);
            message.success("已完成审核");
            await Promise.all([client.invalidateQueries({ queryKey: submissionsKey }), client.invalidateQueries({ queryKey: skillsKey })]);
        },
        onError: async (error) => {
            message.error(platformErrorMessage(error));
            if (error instanceof ApiError && error.status === 409 && error.reason === "submission_changed") {
                setRejecting(null); form.resetFields(); setPath(null); setConflict(true);
                await client.invalidateQueries({ queryKey: submissionsKey });
            }
        },
    });
    const busy = review.isPending || detail.isFetching;
    const metadata = submission?.metadata;
    const files = detail.data ? diffMarketFiles(detail.data.files, detail.data.previousRelease?.files || []) : [];
    return <AppDrawer open title="稿件详情" width={760} onClose={() => { if (!review.isPending) onClose(); }}>
        {conflict && <Alert type="warning" title="作者已更新稿件，请刷新后重新审核" className="mb-4" />}
        {detail.isError ? <Alert type="error" title={platformErrorMessage(detail.error)} action={<Button onClick={() => void detail.refetch()}>刷新</Button>} /> : detail.isPending ? <Skeleton active /> : submission && metadata && <div className="space-y-4" style={{ overflowWrap: "anywhere" }}>
            <h2 className="text-lg font-medium">{metadata.name}</h2>
            <p className="whitespace-pre-wrap">{metadata.description}</p>
            <p>作者：{submission.author.displayName} ({submission.author.username})</p>
            <p>版本：{metadata.versionLabel} · 许可：{metadata.license}</p>
            <p>使用条件：{metadata.requirements.map(requirementLabel).join("、") || "无额外要求"}</p>
            <p className="whitespace-pre-wrap">使用说明：{metadata.requirementNotes || "无"}</p>
            <p>来源：{metadata.sourceUrl || "无"}</p>
            <div>展示链接：{metadata.showcase.length ? <ul>{metadata.showcase.map((item, index) => <li key={index}>{item.url} · {item.type} · {item.caption}</li>)}</ul> : "无"}</div>
            <p>状态：{statuses[submission.status]} · 投稿时间：{time(submission.submittedAt)}</p>
            {submission.reviewedAt && <p>审核时间：{time(submission.reviewedAt)} · 审核人：{submission.reviewer || "—"}</p>}
            {submission.reviewReason && <Alert type="info" title="审核意见" description={submission.reviewReason} />}
            <h3 className="font-medium">文件（{submission.fileCount} 个，共 {submission.archiveBytes} 字节）</h3>
            {detail.data?.previousRelease && <p>对比上一版：{detail.data.previousRelease.versionLabel}</p>}
            <ul>{files.map(item => <li key={item.path} className="mb-2 flex flex-wrap items-center gap-2">
                <Button size="small" disabled={item.change === "deleted" || busy} onClick={() => setPath(item.path)}>{item.path}</Button>
                <span>{item.size} 字节{detail.data?.previousRelease ? ` · ${changes[item.change]}` : ""}</span>
            </li>)}</ul>
            {path && <section aria-label="文件内容" className="space-y-3">
                <h3 className="font-medium">{path}</h3>
                {file.isError ? <Alert type="error" title={platformErrorMessage(file.error)} action={<Button onClick={() => void file.refetch()}>重试</Button>} /> : file.isFetching || detail.isFetching ? <Skeleton active /> : file.data ? (
                    file.data.encoding === "utf-8" ? <pre className="max-h-96 overflow-auto whitespace-pre-wrap p-4" style={{ fontFamily: "var(--font-mono)", background: "var(--fill)", borderRadius: "var(--r-sm)" }}>{file.data.content}</pre>
                        : /^image\/[a-z0-9.+-]+$/i.test(file.data.mimeType) ? <img className="max-h-96 max-w-full object-contain" src={`data:${file.data.mimeType};base64,${file.data.content}`} alt={file.data.path} />
                            : <p>此文件不支持预览。</p>
                ) : null}
            </section>}
            {submission.status === "in_review" && <Space wrap>
                <Popconfirm title="通过此稿件并发布？" onConfirm={() => review.mutate({ kind: "approve", submission, official: false })}>
                    <Button type="primary" disabled={busy}>通过</Button>
                </Popconfirm>
                <Popconfirm title="通过此稿件并设为官方技能？" onConfirm={() => review.mutate({ kind: "approve", submission, official: true })}>
                    <Button disabled={busy}>通过并设为官方</Button>
                </Popconfirm>
                <Button danger disabled={busy} onClick={() => { form.resetFields(); setRejecting(submission); }}>驳回</Button>
            </Space>}
        </div>}
        <AppModal flush open={rejecting !== null} footer={null} onCancel={() => { if (!review.isPending) setRejecting(null); }}>
            <div className="p-6">
                <h2 className="mb-5 text-lg font-medium">驳回稿件</h2>
                <Form form={form} layout="vertical" disabled={review.isPending} onFinish={(values) => {
                    const reason = [values.preset, values.reason?.trim()].filter(Boolean).join("；");
                    if (rejecting) review.mutate({ kind: "reject", submission: rejecting, reason });
                }}>
                    <Form.Item name="preset" label="常见原因"><Select allowClear options={reasons.map(value => ({ value, label: value }))} /></Form.Item>
                    <Form.Item name="reason" label="具体说明" dependencies={["preset"]} rules={[({ getFieldValue }) => ({ validator: (_, value?: string) => getFieldValue("preset") || value?.trim() ? Promise.resolve() : Promise.reject(new Error("请选择原因或填写具体说明")) })]}><Input.TextArea rows={4} /></Form.Item>
                    <Space><Button type="primary" danger htmlType="submit" loading={review.isPending}>确认驳回</Button><Button disabled={review.isPending} onClick={() => setRejecting(null)}>取消</Button></Space>
                </Form>
            </div>
        </AppModal>
    </AppDrawer>;
}
