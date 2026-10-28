import { ArrowRight, Plus, RefreshCw } from "lucide-react";
import { Link, useNavigate } from "react-router";
import type { CanvasLibrarySummary } from "@/services/api/workspace-data";
import { ProjectPreview } from "@/components/canvas/canvas-project-card";
import { beefTVCapabilityItems } from "./home-data";

function formatDate(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "刚刚更新";
    return new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).format(date).replaceAll("/", "-");
}

export function HomeDashboard({ projects, loading, error, onRetry }: { projects: CanvasLibrarySummary[]; loading: boolean; error: boolean; onRetry: () => void }) {
    const navigate = useNavigate();
    const latestProject = projects[0];

    return (
        <main className="beeftv-home hfp-home" aria-label="hfp - all in one 首页">
            <div className="hfp-home-container">
                {/* Editorial Masthead — Ready Library Hero Grammar */}
                <header className="hfp-hero-masthead">
                    <div className="hfp-hero-copy">
                        <div className="caption hfp-hero-caption">
                            <span className="ledg" aria-hidden="true" />
                            <span>LOCAL-FIRST SCREENING ROOM</span>
                            <span className="dot" />
                            <span><b>{projects.length}</b> 部工程在架</span>
                            <span className="dot" />
                            <span><b>{beefTVCapabilityItems.length}</b> 项制片能力</span>
                        </div>
                        <h1 className="lib-title hfp-hero-title">
                            hfp - <em>all in one</em>
                        </h1>
                        <p className="hfp-hero-subtitle">
                            面向 AI 时代的电影感视频创作工作台——在自由画布中串联分镜、模型、素材与时间线成片。
                        </p>
                    </div>

                    <div className="hfp-hero-actions">
                        {latestProject ? (
                            <Link to={`/canvas/${latestProject.id}`} className="lg continue hfp-continue-card" data-refract>
                                <div className="continue-cover hfp-continue-preview">
                                    <ProjectPreview project={{ id: latestProject.id, nodes: latestProject.previewNodes }} emptyVariant="libtv" />
                                </div>
                                <div className="continue-txt">
                                    <span className="lbl">接着剪辑 · CONTINUE</span>
                                    <b>{latestProject.title || "未命名项目"}</b>
                                    <div className="continue-bar">
                                        <i style={{ width: `${Math.min(100, Math.max(18, (latestProject.nodeCount || 1) * 14))}%` }} />
                                    </div>
                                    <span className="continue-pct mono">{latestProject.nodeCount || 0} 节点</span>
                                </div>
                            </Link>
                        ) : null}
                    </div>
                </header>

                {/* Primary Hero Stage — Dotted Projection Desk + Ink Glass Pill with Print-Signal LED */}
                <section className="beeftv-home-hero hfp-projection-stage lg" data-refract aria-label="新建画布">
                    <span className="beeftv-home-dot-base" aria-hidden />
                    <span className="beeftv-home-dot-flow" aria-hidden />
                    <span className="beeftv-home-dot-flow beeftv-home-dot-flow-delay" aria-hidden />

                    <div className="hfp-stage-frame-marks" aria-hidden="true">
                        <span className="mono">SCENE 01 / INFINITE CANVAS</span>
                        <span className="mono">24 FPS · 16:9 · 4PT GRID</span>
                    </div>

                    <button type="button" className="beeftv-home-create hfp-hero-draw-btn lg ink draw press" onClick={() => navigate("/canvas?mode=new")} aria-label="新建画布创作">
                        <span className="led busy" aria-hidden="true" />
                        <span className="beeftv-home-create-icon hfp-draw-plus"><Plus /></span>
                        <span className="beeftv-home-create-title">新建画布创作</span>
                        <kbd className="kbd">↵</kbd>
                    </button>

                    <p className="hfp-stage-hint mono">
                        从空白画布、短剧分镜、参考图像或与 hfp - all in one Agent 对话开始
                    </p>
                </section>

                {/* Studio Capability Shelf — 12-Col Responsive Grid */}
                <section className="hfp-shelf-section" aria-label="创作工坊架">
                    <header className="beeftv-section-heading hfp-section-head">
                        <div className="hfp-section-title-group">
                            <span className="lbl">STUDIOS &amp; TOOLS</span>
                            <h2>创作工坊</h2>
                        </div>
                        <Link to="/create" className="hfp-section-link">
                            进入独立创作室 <ArrowRight />
                        </Link>
                    </header>

                    <nav className="beeftv-capabilities hfp-capabilities-grid" aria-label="创作能力">
                        {beefTVCapabilityItems.map(({ id, label, detail, to, icon: Icon, disabled }, idx) => (
                            disabled ? (
                                <span key={id} className="beeftv-capability hfp-capability-card is-disabled" aria-disabled="true" title={`${label}：${detail}`}>
                                    <span className="hfp-capability-index mono">0{idx + 1}</span>
                                    <span className="beeftv-capability-icon"><Icon /></span>
                                    <strong>{label}</strong>
                                    <small>{detail}</small>
                                </span>
                            ) : (
                                <Link key={id} to={to} className="beeftv-capability hfp-capability-card lg press" data-refract title={detail}>
                                    <span className="hfp-capability-index mono">0{idx + 1}</span>
                                    <span className="beeftv-capability-icon"><Icon /></span>
                                    <strong>{label}</strong>
                                    <small>{detail}</small>
                                </Link>
                            )
                        ))}
                    </nav>
                </section>

                {/* Recent Projects Screening Reels */}
                <section className="beeftv-home-section beeftv-recents hfp-recents-section">
                    <header className="beeftv-section-heading hfp-section-head">
                        <div className="hfp-section-title-group">
                            <span className="lbl">ON YOUR SHELF</span>
                            <h2>最近项目</h2>
                        </div>
                        <Link to="/project" className="hfp-section-link">
                            查看全部 <ArrowRight />
                        </Link>
                    </header>
                    {error ? (
                        <button className="beeftv-home-error hfp-error-card lg" type="button" onClick={onRetry}>
                            <RefreshCw />画布读取失败，点击重试
                        </button>
                    ) : (
                        <div className="beeftv-recent-grid hfp-recent-grid">
                            {loading
                                ? Array.from({ length: 4 }, (_, index) => <div className="beeftv-recent-card is-loading" key={index} />)
                                : projects.length
                                  ? projects.map((project, idx) => (
                                        <Link to={`/canvas/${project.id}`} className="beeftv-recent-card hfp-recent-card lg press" data-refract key={project.id}>
                                            <span className="hfp-spine-accent" aria-hidden="true" />
                                            <span className="beeftv-recent-preview">
                                                <ProjectPreview project={{ id: project.id, nodes: project.previewNodes }} emptyVariant="libtv" />
                                            </span>
                                            <span className="beeftv-recent-copy">
                                                <span className="hfp-recent-title-row">
                                                    <strong>{project.title || "未命名项目"}</strong>
                                                    <span className="hfp-recent-kicker mono">REEL 0{idx + 1}</span>
                                                </span>
                                                <span className="hfp-recent-rule" aria-hidden="true" />
                                                <small className="hfp-recent-meta mono">
                                                    <span>{formatDate(project.updatedAt)}</span>
                                                    <span className="hfp-meta-dot" aria-hidden="true" />
                                                    <span>{project.nodeCount || 0} 节点</span>
                                                </small>
                                            </span>
                                            <ArrowRight className="beeftv-recent-arrow" />
                                        </Link>
                                    ))
                                  : (
                                        <button type="button" className="beeftv-recent-card is-empty hfp-recent-empty lg press" data-refract onClick={() => navigate("/canvas?mode=new")}>
                                            <span className="hfp-spine-accent" aria-hidden="true" />
                                            <span className="beeftv-recent-preview"><Plus /></span>
                                            <span className="beeftv-recent-copy">
                                                <span className="hfp-recent-title-row">
                                                    <strong>创建第一个画布</strong>
                                                    <span className="hfp-recent-kicker mono">REEL 01</span>
                                                </span>
                                                <span className="hfp-recent-rule" aria-hidden="true" />
                                                <small className="hfp-recent-meta">让灵感在放映室与自由画布中落成第一镜</small>
                                            </span>
                                            <ArrowRight className="beeftv-recent-arrow" />
                                        </button>
                                    )}
                        </div>
                    )}
                </section>
            </div>
        </main>
    );
}
