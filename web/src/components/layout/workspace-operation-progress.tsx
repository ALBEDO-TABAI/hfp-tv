import { useEffect, useState } from "react";
import { Check, ChevronDown, ChevronUp, LoaderCircle, TriangleAlert, X } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { motionTransition } from "@/lib/motion-tokens";
import { operationProgressView, type OperationProgress } from "@/lib/operation-progress";
import { getActiveUserScopeEpoch } from "@/lib/user-scope";
import { useOperationProgress } from "@/stores/use-operation-progress";
import "./workspace-operation-progress.css";

export function FileOperationProgress({ progress, now = Date.now() }: { progress: OperationProgress; now?: number }) {
    const view = operationProgressView(progress, now);
    return <div className="file-operation-progress" role="status" aria-live="polite">
        <div className="file-operation-stage"><span>{progress.detail}</span>{view.percent !== null ? <strong>{view.percent}%</strong> : null}</div>
        <div className="file-operation-bar" role="progressbar" aria-label={progress.title} aria-valuemin={0} aria-valuemax={100} aria-valuenow={view.percent ?? undefined} data-indeterminate={view.percent === null && progress.phase !== "error" || undefined}>
            <i style={view.percent === null ? undefined : { transform: `scaleX(${view.percent / 100})` }} />
        </div>
        <div className="file-operation-metrics">{view.size ? <span>{view.size}</span> : null}{view.speed ? <span>{view.speed}</span> : null}{view.count ? <span>{view.count}</span> : null}</div>
        {view.timing ? <div className="file-operation-time">{view.timing}</div> : null}
    </div>;
}

export function WorkspaceOperationProgress() {
    const entries = useOperationProgress(state => state.entries);
    const remove = useOperationProgress(state => state.remove);
    const [collapsed, setCollapsed] = useState(false);
    const [now, setNow] = useState(Date.now());
    const operations = Object.values(entries).filter(item => !item.target);
    const active = operations.filter(item => item.phase !== "done" && item.phase !== "error").length;
    useEffect(() => {
        if (!active) return;
        const timer = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(timer);
    }, [active]);
    return (
        <AnimatePresence key={getActiveUserScopeEpoch()}>
            {operations.length ? (
                <motion.aside
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0, transition: motionTransition.panelEnter }}
                    exit={{ opacity: 0, transition: motionTransition.panelExit }}
                    className="workspace-operation-tray"
                    aria-label="文件处理进度"
                    data-canvas-no-zoom
                >
                    <button className="workspace-operation-heading" type="button" aria-expanded={!collapsed} onClick={() => setCollapsed(value => !value)}>
                        <span>文件处理{active ? " · " + active + " 项进行中" : " · 已结束"}</span>{collapsed ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                    </button>
                    {!collapsed ? <div className="workspace-operation-list">{operations.map(item => {
                        const finished = item.phase === "done" || item.phase === "error";
                        return <section key={item.id} className="workspace-operation-item" data-phase={item.phase}>
                            <div className="workspace-operation-title">
                                <span className="workspace-operation-icon" aria-hidden="true">
                                    <span data-visible={!finished || undefined}><LoaderCircle size={15} className="animate-spin" /></span>
                                    <span data-visible={finished || undefined}>{item.phase === "error" ? <TriangleAlert size={15} /> : <Check size={15} />}</span>
                                </span>
                                <strong title={item.title}>{item.title}</strong>
                                {finished ? <button type="button" aria-label={"关闭" + item.title + "进度"} onClick={() => remove(item.id)}><X size={14} /></button> : null}
                            </div>
                            <FileOperationProgress progress={item} now={Math.max(now, Date.now())} />
                        </section>;
                    })}</div> : null}
                </motion.aside>
            ) : null}
        </AnimatePresence>
    );
}
