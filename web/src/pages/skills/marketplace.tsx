import { App, Alert, Button, Input, Select, Skeleton, Tabs, Tag } from "antd";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ComponentRef } from "react";
import { Link, useSearchParams } from "react-router";
import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { AppModal, useAppModal } from "@/components/ui/product/app-modal";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { motionPreset } from "@/lib/motion-tokens";
import { ApiError } from "@/services/api/request";
import { platformErrorMessage } from "@/lib/platform-account";
import { addSkill, deleteSkill, getSkill, listSkills, removeSkill, type Skill } from "@/services/api/skills";
import { forkSkill, getInstalledVersions, getMarketRelease, installMarketRelease, rollbackSkill, searchSkillMarket, type InstalledVersion, type MarketItem, type MarketRelease } from "@/services/api/skill-market";
import { SkillEditorDrawer } from "./skill-editor-drawer";
import { SkillInstallModal } from "./skill-install-modal";
import { SkillDetailModal } from "./skill-detail-drawer";
import { SkillPublishingPanel } from "./publishing-panel";
import { MarketFilePreview } from "./market-file-preview";
import { MarketReviewSummary } from "./market-review-summary";
import { capabilityOptions, requirementLabel } from "./market-copy";
import "./marketplace.css";

const skillListsKey = ["skills", "marketplace"];
const skillTabs = [{ key: "market", label: "技能市场" }, { key: "mine", label: "我的技能" }, { key: "publish", label: "我的投稿" }];
type SkillResults = { page: number; total: number } & ({ tab: "market"; items: MarketItem[] } | { tab: "mine"; items: Skill[] });

export default function SkillMarketplace() {
    const { message } = App.useApp();
    const modal = useAppModal();
    const client = useQueryClient();
    const [params, setParams] = useSearchParams();
    const tab = skillTabs.find(({ key }) => key === params.get("tab"))?.key ?? "market";
    const tabsRef = useRef<ComponentRef<typeof Tabs>>(null);
    const [indicator, setIndicator] = useState({ left: 0, width: 0 });
    const [query, setQuery] = useState("");
    const [page, setPage] = useState(1);
    const [showSkeleton, setShowSkeleton] = useState(false);
    const [marketNotice, setMarketNotice] = useState("");
    const [actionError, setActionError] = useState("");
    const [busy, setBusy] = useState(false);
    const [release, setRelease] = useState<MarketRelease | null>(null);
    const [detail, setDetail] = useState<Skill | null>(null);
    const [edit, setEdit] = useState<Skill | null>(null);
    const [editorOpen, setEditorOpen] = useState(false);
    const [installOpen, setInstallOpen] = useState(false);
    const [versions, setVersions] = useState<{ skill: Skill; items: InstalledVersion[] } | null>(null);
    const [versionId, setVersionId] = useState("");
    const filters = useMemo(() => ({ tab, page, query }), [tab, page, query]);
    const appliedFilters = useDebouncedValue(filters, 200);
    const list = useQuery({
        queryKey: [...skillListsKey, appliedFilters],
        enabled: tab !== "publish" && appliedFilters.tab !== "publish",
        placeholderData: keepPreviousData,
        queryFn: async (): Promise<SkillResults> => {
            const { tab, page, query } = appliedFilters;
            if (tab === "market") {
                const result = await searchSkillMarket(query, page);
                return { tab, page, items: result.items || [], total: result.total };
            }
            const result = await listSkills({ scope: "mine", page, pageSize: 20, search: query });
            return { tab: "mine", page, items: result.skills, total: result.totalCount };
        },
    });
    const replacing = list.isPlaceholderData || filters !== appliedFilters;
    const listError = filters === appliedFilters ? list.error : null;
    const listNotice = listError instanceof ApiError && listError.reason === "market_not_configured" ? platformErrorMessage(listError) : "";
    const notice = marketNotice || listNotice;
    const results = listError || (notice && tab === "market") ? undefined : list.data;
    const pendingList = tab !== "publish" && !notice && !list.data && (list.isPending || replacing);
    const reload = async () => {
        setMarketNotice("");
        setActionError("");
        // 首次读取还没返回时，invalidate 会直接沿用那次旧请求；先取消再重读，写入之后拿到的才是最新列表。
        await client.cancelQueries({ queryKey: skillListsKey });
        await client.invalidateQueries({ queryKey: skillListsKey });
    };
    const changeTab = (next: string) => { setParams({ tab: next }); setPage(1); setQuery(""); setMarketNotice(""); setActionError(""); };
    useLayoutEffect(() => {
        const active = tabsRef.current?.nativeElement?.querySelector('[role="tab"][aria-selected="true"]')?.closest<HTMLElement>(".skill-market-tab");
        if (!active) return;
        const measure = () => setIndicator({ left: active.offsetLeft, width: active.offsetWidth });
        measure();
        const observer = new ResizeObserver(measure);
        observer.observe(active);
        if (active.parentElement) observer.observe(active.parentElement);
        return () => observer.disconnect();
    }, [tab]);
    useEffect(() => {
        if (!pendingList) { setShowSkeleton(false); return; }
        const timer = window.setTimeout(() => setShowSkeleton(true), 200);
        return () => window.clearTimeout(timer);
    }, [pendingList]);
    useEffect(() => {
        const skillId = params.get("skillId"), releaseId = params.get("releaseId");
        if (!skillId || !releaseId) return;
        let stale = false;
        setBusy(true); setRelease(null); setActionError("");
        void getMarketRelease(skillId, releaseId).then(value => { if (!stale) setRelease(value); })
            .catch(e => {
                if (stale) return;
                if (e instanceof ApiError && e.reason === "market_not_configured") setMarketNotice(platformErrorMessage(e));
                else setActionError(e instanceof Error ? e.message : "该技能版本暂时不可用");
            })
            .finally(() => { if (!stale) setBusy(false); });
        return () => { stale = true; };
    }, [params]);
    // 弹窗里的操作把错误留在弹窗里；页面上的操作（例如长列表底部的启停）用全局提示，滚到哪里都看得见。
    const showActionError = (text: string) => { if (release || versions) setActionError(text); else message.error(text); };
    // waitForList：启停这类行内操作要等列表刷新完再解锁，否则按钮还显示旧状态，再点会按旧状态重复提交；
    // 安装会切到「我的技能」，列表在后台刷新，不让上一页的慢请求拖住按钮。
    const action = async (work: () => Promise<unknown>, done?: string, waitForList = true) => {
        setBusy(true);
        setActionError("");
        try {
            await work();
            if (done) message.success(done);
            if (waitForList) await reload();
            else void reload();
        } catch (e) {
            if (e instanceof ApiError && e.reason === "market_not_configured") { setMarketNotice(platformErrorMessage(e)); setRelease(null); }
            else showActionError(e instanceof Error ? e.message : "操作未完成");
        } finally { setBusy(false); }
    };
    const openLocal = async (skill: Skill) => action(async () => { setDetail((await getSkill(skill.skillId)).skill); }, "已读取技能文件");
    const openEditor = (skill?: Skill) => { setDetail(null); setEdit(skill || null); setEditorOpen(true); };
    const openRelease = (skillId: string, releaseId: string) => setParams({ tab: "market", skillId, releaseId });
    const deleteLocal = (skill: Skill) => modal.confirm({ title: `删除本机的“${skill.skillName}”？`, content: "本机技能文件和历史版本会被删除。公开市场中的版本不会改变。", okText: "删除本机技能", cancelText: "取消", okButtonProps: { danger: true }, onOk: async () => { await deleteSkill(skill.skillId); setDetail(null); reload(); } });
    const showVersions = async (skill: Skill) => action(async () => { const value = await getInstalledVersions(skill.skillId); setVersions({ skill, items: value.versions }); setVersionId(skill.versionId); }, "已读取本机版本");
    const install = () => { if (release) void action(async () => { await installMarketRelease(release.skillId, release.releaseId); setRelease(null); changeTab("mine"); }, "已安装，可在画布助手中选择使用", false); };
    const actionFeedback = <AnimatePresence>
        {actionError && <motion.div key="action-error" {...motionPreset.fade} className="mb-4"><Alert type="error" showIcon title={actionError} /></motion.div>}
    </AnimatePresence>;
    return <WorkspacePage className="skill-market-page">
        <PageHeader title="技能" description="把创作方法带进助手。安装和私人改写无需审核，公开分享时再投稿。" actions={<><Button onClick={() => openEditor()}>创建技能</Button><Button onClick={() => setInstallOpen(true)}>导入文件</Button></>} />
        <Tabs ref={tabsRef} activeKey={tab} onChange={changeTab} items={skillTabs} animated={false}
            classNames={{ item: "skill-market-tab", indicator: "skill-market-tab-indicator" }}
            styles={{ indicator: { left: 0, right: "auto", width: indicator.width, transform: `translateX(${indicator.left}px)` } }} />
        {tab === "publish" ? <SkillPublishingPanel planId={params.get("planId") || undefined} /> : <>
            {notice ? <Alert type="info" title={notice} className="mb-4" /> : null}
            <div className="skill-market-toolbar"><Input.Search aria-label="搜索技能" placeholder="搜索名称或用途" allowClear value={query} onChange={e => { setQuery(e.target.value); setPage(1); setMarketNotice(""); }} /><Button icon={<RefreshCw className="size-[1em]" />} loading={list.isRefetching} onClick={reload}>刷新</Button></div>
            {!release && !versions && actionFeedback}
            {listError && !listNotice ? <Alert type="error" showIcon title={listError.message || "暂时无法读取技能"} action={<Button size="small" onClick={() => void reload()}>重试</Button>} /> : null}
            <div className="grid" aria-busy={pendingList || replacing}>
                <AnimatePresence>
                    {pendingList && showSkeleton && <motion.div key="skeleton" {...motionPreset.fade} className="pointer-events-none col-start-1 row-start-1" aria-hidden="true"><Skeleton paragraph={{ rows: 6 }} /></motion.div>}
                </AnimatePresence>
                <div className="skill-market-results col-start-1 row-start-1" data-ready={Boolean(results)} data-updating={Boolean(results) && replacing} inert={replacing}>
                    {results?.tab === "market" ? results.items.map(item => <article className="skill-market-row" key={`${item.skillId}/${item.releaseId}`}>
                        <div className="skill-market-copy"><h2>{item.name} <span>{item.versionLabel}</span></h2><p>{item.description}</p><div className="skill-market-meta"><span>{item.author?.replace(/BeefTV/g, "hfp - all in one")}</span><span>{item.license}</span><span>{item.verification === "tested" ? "有实测记录" : "效果尚未验证"}</span></div>{item.compatibility?.status === "unsupported" ? <p className="skill-market-warning">当前环境不满足：{item.compatibility.reasons?.join("、")}</p> : null}</div>
                        <Button onClick={() => openRelease(item.skillId, item.releaseId)} loading={busy}>查看与安装</Button>
                    </article>) : results?.items.map(skill => <article className="skill-market-row" key={skill.skillId}>
                        <div className="skill-market-copy"><h2>{skill.skillName} <span>{skill.version}</span> {!skill.isAdded ? <Tag>已停用</Tag> : null}</h2><p>{skill.description}</p><div className="skill-market-meta"><span>{skill.sourceType === "market" ? "市场固定版本" : "本机技能"}</span><span>{skill.fileCount} 个文件</span></div>
                            <div className="skill-market-actions">
                                <Button size="small" onClick={() => void openLocal(skill)}>查看文件</Button>
                                <Button size="small" disabled={busy} onClick={() => void action(() => skill.isAdded ? removeSkill(skill.skillId) : addSkill(skill.skillId))}>
                                    <span className="skill-market-toggle-label"><span aria-hidden={skill.isAdded}>启用</span><span aria-hidden={!skill.isAdded}>停用</span></span>
                                </Button>
                                {skill.sourceType === "market" ? <><Button size="small" onClick={() => { changeTab("market"); setQuery(skill.skillName); }}>检查更新</Button><Button size="small" onClick={() => void showVersions(skill)}>历史版本</Button><Button size="small" onClick={() => void action(async () => openEditor((await forkSkill(skill.skillId)).skill), "已创建私人副本")}>私人改写</Button></> : <><Button size="small" onClick={() => void action(async () => openEditor((await getSkill(skill.skillId)).skill), "已读取技能")}>编辑</Button><Button size="small" onClick={() => setParams({ tab: "publish", skillId: skill.skillId })}>准备投稿</Button></>}
                                {skill.isOwner ? <Button size="small" danger onClick={() => deleteLocal(skill)}>删除</Button> : null}
                            </div>
                        </div>
                    </article>)}
                    {results && <>
                        {results.total === 0 ? <p className="skill-market-empty">{results.tab === "mine" ? "还没有技能。可以从市场安装，或导入自己的 SKILL.md。" : "没有找到匹配的技能。"}</p> : null}
                        <div className="skill-market-actions"><Button disabled={results.page <= 1} onClick={() => setPage(n => n - 1)}>上一页</Button><span>第 {results.page} 页 · 共 {results.total} 项</span><Button disabled={results.page * 20 >= results.total} onClick={() => setPage(n => n + 1)}>下一页</Button></div>
                        {results.tab === "mine" ? <p className="skill-market-help">打开项目画布，在助手输入框选择“技能”，再选本次要用的版本。使用过程中产生的模型费用按你的模型配置计费。<Link to="/project">打开项目</Link></p> : null}
                    </>}
                </div>
            </div>
        </>}
        <AppModal open={Boolean(release)} title="检查技能并安装" onCancel={() => { setRelease(null); setActionError(""); setParams({ tab: "market" }); }} footer={<Button type="primary" loading={busy} disabled={release?.requirements.some(value => !capabilityOptions.some(item => item.value === value))} onClick={install}>安装此版本</Button>} width={720}>
            {release ? <div className="skill-market-preview">
                {actionFeedback}
                <h2>{release.name} · {release.versionLabel}</h2><p>{release.description}</p><p>作者：{release.author?.replace(/BeefTV/g, "hfp - all in one")} · 许可：{release.license}</p>
                <p>使用条件：{release.requirements.length ? release.requirements.map(requirementLabel).join("、") : "没有额外工具要求"}</p>{release.requirementNotes ? <p>{release.requirementNotes}</p> : null}
                {release.requirements.some(value => value.startsWith("model.")) ? <Alert type="info" title="使用前需要在模型配置中选择支持所需理解能力的模型。安装不代表这些条件已满足。" /> : null}
                <p>模型与费用请按技能说明确认。安装只保存文件，使用时由你选择模型。</p><p>安装后不会自动运行，也不会替你切换模型。后续更新需要再次确认。</p>
                <h3>包含的文件</h3><ul>{release.files.map(file => <li key={file.path}>{file.path} · {file.size} 字节</li>)}</ul><p className="skill-market-meta">内容摘要：{release.contentHash}</p>
                <MarketReviewSummary review={release.review} /><MarketFilePreview key={release.releaseId} skillId={release.skillId} releaseId={release.releaseId} />
            </div> : null}
        </AppModal>
        <AppModal open={Boolean(versions)} title="恢复本机历史版本" onCancel={() => { setVersions(null); setActionError(""); }} onOk={() => { if (versions) void action(async () => { await rollbackSkill(versions.skill.skillId, versionId); setVersions(null); }, "已恢复指定版本"); }} okText="恢复此版本" okButtonProps={{ disabled: !versions || versionId === versions.skill.versionId, loading: busy }}>
            {versions && actionFeedback}
            <p>恢复后新对话可选择此版本。已有对话保留原先选定的版本。</p><Select className="w-full" value={versionId} onChange={setVersionId} options={versions?.items.map(v => ({ value: v.id, label: `${v.versionLabel} · ${v.createdAt.slice(0, 10)}${v.id === versions.skill.versionId ? "（当前）" : ""}` }))} />
        </AppModal>
        <SkillDetailModal fileOnly skill={detail} loading={false} mutating={busy} categories={[]} onClose={() => setDetail(null)} onEdit={openEditor} onAdd={() => {}} onLike={() => {}} onSync={() => {}} />
        <SkillEditorDrawer open={editorOpen} skill={edit} onClose={() => setEditorOpen(false)} onSaved={() => { setEditorOpen(false); changeTab("mine"); reload(); }} />
        <SkillInstallModal open={installOpen} onClose={() => setInstallOpen(false)} onManualCreate={() => { setInstallOpen(false); openEditor(); }} onInstalled={() => { setInstallOpen(false); changeTab("mine"); reload(); }} />
    </WorkspacePage>;
}
