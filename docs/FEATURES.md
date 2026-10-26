# BeefTV → hfp-tv 全量功能盘点清单 (Feature Inventory)

本文档对上游开源项目 **BeefTV** (`https://github.com/glanderness/BeefTV`) 的所有页面、交互流程、节点工具、状态机、设置项与后端 HTTP API 进行 100% 完整盘点。在重命名为 **hfp-tv** 并以「藏书阁 (Ready Library)」设计语言重构视觉与交互后，以下全部能力必须保持完整可用。

---

## 1. 架构与运行模式概览

- **本地优先架构 (Local-First Workspace)**：
  - 前端基于 React 19 + TypeScript + Vite + Zustand + TanStack Query + Tailwind CSS + Ant Design + Leafer UI + Three.js + Vidstack + Excalidraw + TipTap。
  - 后端基于 Go (`backend/cmd/server` 与 `backend/cmd/desktop` Wails 桌面宿主) + SQLite + 本地文件资源存储。
  - 默认以单用户本地工作区运行 (`isLocalWorkspaceMode() === true`)，无需注册登录即可使用全部画布、项目、素材、技能、模型配置与诊断能力；同时保留完整的本地 HTTP API 同步、IndexedDB 降级缓存与 CAS 版本冲突保护。
- **启动自检与外观引导 (`startup-preflight` & `appearance-bootstrap`)**：
  - 启动前执行运行环境检查、本地存储水合 (`useCanvasStore.persist.rehydrate()`、`hydrateLocalCanvasProjectsFromBackend()`) 与品牌外观初始化 (`bootstrapAppearance()`)。

---

## 2. 路由与页面信息架构 (Routes & Pages)

| 路由路径 | 页面组件 | 核心功能说明 |
| --- | --- | --- |
| `/` | `HomePage` (`pages/home/index.tsx`) | 首页工作台：主视觉新建画布入口、7 大核心创作能力快捷入口（自由画布、视频生成、图片生成、音频生成、3D 导演台、hfp-tv Agent、项目库）、最近更新的 4 个画布项目卡片（含缩略图预览、空状态、加载骨架、错误重试）。 |
| `/create` | `CreatePage` (`pages/create/index.tsx`) | 独立多模态创作室：支持图片、视频、文本三种生成模式切换，带模型选择、比例/时长/画质/数量参数、参考素材上传与素材库选择、按模型能力校验参考上限（如 Seedance 2.0 的 9 图 / 3 视频 / 3 音频）、`@` 智能引用（素材与技能）、内置 AI 提示词优化器、对话流与分镜轨道 (`CreationShotRail`)、共用全屏图片查看器、历史会话抽屉、版本冲突恢复、一键转入自由画布继续创作。支持 `?mode=image\|video\|text`、`?skill=<id>`、`?demo=conversation`。 |
| `/project`, `/projects`, `/canvas` | `CanvasPage` (`pages/canvas/index.tsx`) | 项目与画布库：支持网格浏览、关键词防抖搜索、排序（按更新时间 / 名称 / 节点数）、按所属项目或文件夹筛选、新建/重命名/删除文件夹（支持 6 种文件夹样式与 4 种主题）、新建画布项目、复制项目副本、多选批量删除、批量导出 `.canvas` 工程备份包、从备份包恢复、回收站 (`RecycleBinDialog`) 查看与彻底清空。 |
| `/canvas/:id` | `CanvasProjectPage` (`pages/canvas/project.tsx`) | 无限画布创作工作台：核心空间创作界面，包含顶部项目栏、画布主舞台（Leafer 图形层 + DOM 节点层 + LOD 概览层）、底部主工具栏 Dock、右下角缩放与自动整理控件、小地图 (`Minimap`)、左侧项目画布切换栏與素材托盘、右侧内置创作助手 (`CanvasAssistantSidebar`)、版本历史树 (`CanvasVersionHistory`)、可复用工作流抽屉与编辑器 (`CanvasWorkflowWorkspace`)。支持多种内置演示 Fixture (`?fixture=libtv-video`、`libtv-storyboard`、`libtv-readonly-dense` 等)。 |
| `/projects/:projectId/*` | `ProjectDetailPage` (`pages/projects/detail.tsx`) | 剧集/短剧项目详情工作台（非纯本地重定向模式下或剧集工程模式）：包含项目概览 (`overview`)、章节管理 (`chapters`)、项目专属资产库 (`assets`，支持角色卡/三视图/场景/道具提取)、分镜制作流水线 (`workflow`)、时间线成片剪辑器 (`editor`)、项目设置 (`settings`)。 |
| `/assets` | `AssetsPage` (`pages/assets/index.tsx`) | 个人资产库与生成历史：双栏页签（「个人素材」与「生成历史 `?tab=history`」），支持网格/列表视图切换、类型筛选（全部/文本/图片/视频/音频）、文件夹管理（新建/重命名/删除/移入）、排序、上传本地媒体文件、新建文本/图片/模型资产、全屏预览（图片缩放平移、视频播放、音频试听）、复制、下载、批量选择删除、资产包导入导出、存储空间用量条 (`AssetStorageUsage`)。 |
| `/skills` | `SkillMarketplace` (`pages/skills/marketplace.tsx`) | 技能中心：三大页签（「技能市场 `?tab=market`」「我的技能 `?tab=mine`」「我的投稿 `?tab=publish`」）。支持搜索市场技能、预览版本文件与哈希/审核摘要、按环境要求安装固定版本；启用/停用本机技能、查看文件、历史版本回退 (`rollbackSkill`)、复制为私人改写副本 (`forkSkill`)、在线编辑技能文件 (`SkillEditorDrawer`)、导入本地 `SKILL.md`/ZIP、创建并提交技能投稿。 |
| `/settings` | `SettingsPage` (`pages/settings/index.tsx`) | 系统与模型配置中心：包含「模型服务 (`?section=channels`)」「默认模型 (`?section=models`)」「存储设置 (`?section=storage`)」「问题诊断 (`?section=diagnostics`)」。 |
| `/agents` | `AgentsPage` (`pages/agents/index.tsx`) | hfp-tv MCP 与外部 Agent 连接中心：管理 Claude Code、Cursor、Codex 等外部智能体客户端连接，查看连接状态（未连接/等待连接/已验证连接）、生成连接命令、设置只读/读写权限、断开吊销客户端、复制 `hfp-tv` / `beeftv` CLI 安装命令。 |
| `/plugins`, `/plugins/eagle` | `PluginsPage`, `EagleLibraryPage` | 插件中心与 Eagle 外部素材库：按协议（文本/图片/视频/音频/应用插件）浏览与启停内置及扩展插件（AI 提示词优化器、Eagle 素材库等），查看插件权限与文档，配置 Eagle 本地服务地址并直接浏览导入 Eagle 文件夹与标签素材。 |
| `/test-voice-recording` | `TestVoiceRecording` | 语音录制测试工具页。 |

---

## 3. 全局工作区外壳与导航 (Workspace Shell & Navigation)

1. **左侧可折叠导航栏 (`WorkspaceSidebarNav`)**：
   - 品牌标识与名称展示，支持桌面端展开/收起 (`desktopSidebarCollapsed`) 与移动端抽屉展开 (`mobileSidebarExpanded`)。
   - 核心入口：`新建项目` (`/canvas?mode=new`)、`首页` (`/`)、`项目` (`/project`)、`资产` (`/assets`)、`技能` (`/skills`)、`模型配置` (`/settings?section=channels`)、`存储设置`（桌面端原生绑定可用时）、`hfp-tv MCP` (`/agents`)。
   - 底部区域：账号存储容量进度条 (`WorkspaceSidebarStorageMeter`，70% 警告、90% 危险)、首次创作引导入口 (`OnboardingEntry`)、桌面更新状态提示 (`WorkspaceSidebarUpdate`)、社交与文档链接、浅色/深色主题切换按钮 (`AnimatedThemeToggler`)。
2. **全局命令与搜索面板 (`WorkspaceCommandPalette`)**：
   - 快捷键 `⌘K` / `Ctrl+K` 呼出，支持跨页面跳转、搜索画布项目、快速执行新建操作。
3. **全局操作进度浮层 (`WorkspaceOperationProgress`)** 与 **首次创作引导 (`FirstCreationGuide`)**。

---

## 4. 无限画布工作台核心功能 (`/canvas/:id`)

### 4.1 画布起手式与空状态 (`CanvasShortDramaEntry`)
- 新建空白画布时提供起手引导：可选择「短剧流水线引导」或「自由空白画布」。
- 自由空白画布中央展示带 `+` 号的起始框，点击呼出与底部工具栏一致的节点创建菜单（文本、绘图、分镜、背板、文件夹、图片、视频、音频、工作流、AI 超分、上传文件、素材库）。

### 4.2 视口控制、主工具栏与画布外观
- **底部主工具栏 (`CanvasToolbar`)**：
  - 区域选择 (`box-select`) 与抓手平移 (`hand`) 胶囊切换开关（支持空格键或鼠标中键临时平移）。
  - 添加节点菜单（覆盖所有基础媒体节点、分镜、背板、文件夹、工作流、AI 超分、3D 导演台、插件节点）。
  - 上传本地文件、从个人素材库插入、打开画布外观面板。
- **画布外观 (`CanvasAppearanceControls`)**：
  - 4 种主题模式：跟随全局 (`follow`)、浅色 (`light`)、深色 (`dark`)、自定义 (`custom`，支持自定义背景 HEX 色码、OKLCH 明亮度调节、网格颜色与 0%–100% 网格强度）。
  - 3 种背景网格：点阵 (`dots`)、线稿 (`grid`)、空白 (`blank`)。
  - 支持将当前外观保存为本机新建画布默认值。
- **右下角视图控制 (`CanvasZoomControls` & `Minimap`)**：
  - 缩放比例显示与加减、适应屏幕 (`Fit View`)、连线显隐开关、对齐网格开关、自动整理节点 (`自动整理节点`：有连线时按左→右拓扑层级与媒体泳道排布，无连线时按媒体类型分网格排布，支持撤销)、小地图开关。
- **多选悬浮工具栏 (`CanvasProjectSelectionToolbar`)**：
  - 框选多个节点后提供左/中/右/顶/底对齐、水平/垂直等距分布、创建分镜组、批量连接、保存为可复用工作流 (`保存为工作流`)、右键「自适应整理画布 (`spreadCanvasNodes`)」。

### 4.3 节点类型与专业媒体工具
1. **图片节点 (`Image`)**：
   - 生成配置：模型选择、比例/尺寸 (`ImageSizePicker`)、画质、数量、透明背景、Seedream 模式（单图 / 图层拆分 / 关联组图）、参考图连接。
   - 图片工具栏 (`CanvasNodeToolbar`)：全屏预览 (`CanvasImagePreview`，支持 0.5x–12x 缩放与拖拽)、**多角度 3D 编辑器** (`CanvasNodeAnglePanel` + `CanvasAngleScene` 天空盒与机位箭头)、**光影预设** (`CanvasNodeLightingPanel`)、**宫格切分** (`CanvasGridSplitPicker`：4/9/16/25 预设与 5×5 自定义点选切分)、裁切、尺寸调整、局部重绘/涂抹、情绪画板 (`CanvasEmotionWorkspace`)、一键创建 **AI 超分** 节点、下载原图。
2. **视频节点 (`Video`)**：
   - Vidstack 播放器 (`CanvasVideoPlayer`)：支持鼠标悬停 350ms 静音预览前 3 秒（开启减弱动态效果时不自动预览）、H.265 原文件按需后台转码 H.264 预览、音量/全屏/进度控制。
   - 视频工具：行内裁剪片段 (`CanvasVideoInlineTrimOverlay`)、画面比例裁切 (`CanvasVideoCropDialog`)、视频截帧 (`CanvasVideoFrameDialog`)、提取音频、多视频合并 (`CanvasVideoMerge`)、字幕高亮与同步到时间线、深度动作捕捉 (`depth-captures`)、一键创建 **AI 超分** 节点。
3. **音频节点 (`Audio`)**：
   - 语音合成（MiniMax Speech 等，支持男女音色与语速）与音乐生成（MiniMax Music 3.0 MP3 流式生成）、波形/播放器预览与下载。
4. **文本与剧本节点 (`Text` / `Script` / `Storyboard`)**：
   - 富文本编辑 (TipTap)、分镜表节点 (`CanvasScriptNodeContent`，支持镜头序号、画面描述、动作、台词、时长、关联角色与首帧/视频生成)、章节资产提取（一次提取角色、场景、道具三类资产）。
5. **批量创作表 (`CanvasBatchTableNodeContent`)**：
   - 表格化批量生成管理：全局提示词覆盖、参考图列拖拽重排、跨行跨列交换缩略图、连线自动同步任务行。
6. **AI 超分处理节点 (`Upscale`)**：
   - 批量连接多个图片/视频输入，配置 Topaz 模型（Standard V2、Wonder 3.5、Proteus、Starlight Fast 2）与倍率（1x–4x），费用估算、批量提交与失败取回恢复 (`recover`)。
7. **3D 导演台节点 (`CanvasDirectorNodePanel`)**：
   - 3D 场景机位、角色动作姿态与参考素材联动生成。
8. **绘图节点 (`Excalidraw`)、文件夹节点 (`CanvasFolderCard`)、背板容器 (`Frame`)**。

### 4.4 提示词面板、智能引用与 AI 优化器
- **智能引用 (`AutoLink` & 一键引用全文)**：输入已连接素材名称、标签或 `图1`/`图片1`/`image 1` 自动匹配候选项，按 `Tab` 插入结构化引用芯片。
- **提示词放大编辑**：支持放大至 `min(1200px, 92vw)` 并拖拽右下角自由调整尺寸。
- **AI 提示词优化器 (`CanvasPromptOptimizerDrawer`)**：内置插件驱动，提供「扩展想法」「精修已有提示词」「强化风格」「适配模型」「结合参考」5 种优化策略，支持流式输出、正向/规避项预览与一键「采用」回填。

### 4.5 可复用工作流 (`CanvasWorkflowWorkspace` & `CanvasWorkflowEditor`)
- 从画布框选节点封装为可复用工作流模板，或从空白新建工作流。
- 独立「工作流设定」三栏编辑器：左侧添加输入/本地处理（图片裁切、尺寸调整、视频裁切、截帧、音频提取、剪辑、拼接、深度处理）/生成节点；中间画布连线与排布；右侧配置参数、`@[node:<ID>]` 引用与动态视频时长绑定 (`videoDurationSourceNodeId`)。
- 将工作流添加到画布后生成 `reusable-workflow` 容器与 `workflow-control` 控制器，支持拖入/上传替换输入素材、画布内直接运行/停止/重试、模板新版本实例升级。

### 4.6 内置创作助手 (`CanvasAssistantSidebar`)
- 围绕当前画布与 `@` 引用素材进行多轮对话规划。
- 支持分镜方案提案 (`canvas.generation.propose`)、整片制作确认卡片 (`CanvasTaskConfirmation`)、自动创建节点与连线、批量调用模型生成、多模态影音结果检查、自动接续轮次与成片时间线交付。

### 4.7 保存保护、强制覆盖与版本记录
- **顶部状态与菜单 (`CanvasTopBar` & `CanvasSyncStatus`)**：实时显示保存状态，提供手动「保存 (`⌘/Ctrl+S`)」与「强制覆盖保存 (`forceOverwriteRemoteCanvasSync`)」，以及导入 LibTV / TapNow 工程、导出当前画布备份。
- **版本记录 (`CanvasVersionHistory` & `CanvasVersionPreview`)**：按日期展开快照历史树，点击进入独立只读预览视口，支持一键恢复到任意历史版本（自动先备份当前草稿）。

---

## 5. 时间线成片编辑器 (`/projects/:projectId/editor`)

- 基于统一命令状态机 (`editor-commands.ts`)，支持轨道片段添加、删除、拆分、移动、修剪，具备 200 层历史撤销/重做 (`editor-history.ts`)。
- 左侧预设面板接入 8 个内置片段/字幕插槽插件 (`editor-slot-registry.ts`)，具备 `timeline.command` 与 `plugins.run` 失败关闭权限检查。
- 支持发起本地 `whisper.cpp` 或云端语音转写任务生成字幕轨 (`POST /timeline/transcriptions`)。
- 支持编译语义渲染计划 (`POST /timeline/render-plan`) 与发起本机 FFmpeg 成片渲染导出 (`POST /timeline/renders`)。
- 内置 AI 编辑助手：自然语言转时间线命令，超过 3 条命令时先展示差异预览再确认执行。

---

## 6. 模型服务、存储与问题诊断 (`/settings`)

1. **模型服务 (`ChannelSettingsPane` & `ModelServiceEditor`)**：
   - 内置官方渠道连接（OAuth 授权与目录刷新 `POST /beefapi/connection/models`）。
   - 添加自定义模型渠道（OpenAI、Google Gemini、火山方舟 Volcengine Ark 及兼容服务），配置名称、Base URL、API Key、自定义请求头、参考媒体传输方式 (`auto` / `inline` / `url` / `link`)。
   - 拉取远程模型目录 (`POST /ai/models`)，支持按用途（文本/图片/视频/音频）筛选、搜索或手动添加模型 ID。
   - 单模型能力与协议配置 (`ModelEditorModal`)，支持带费用提示的真实生成测试。
2. **默认模型 (`ModelDefaultGrid`)**：
   - 分别指定全局默认文本模型、图片模型、视频模型、音频模型。
3. **存储设置 (`StorageSettingsPane` & `ReferenceStoragePane` & `DepthComponentsPanel`)**：
   - 本地数据目录查看/迁移、参考素材对象存储配置、深度捕捉运行时组件状态检查。
4. **问题诊断 (`DiagnosticsPanel` & `EnvironmentChecks`)**：
   - 运行环境自检与临时目录修复、清理本地视频 H.264 预览缓存 (`DELETE /resources/playback-cache`)、复制排查信息、预览数据量并导出脱敏诊断 ZIP 包 (`/api/diagnostics/*`)。

---

## 7. 后端 HTTP API 完整路由表 (`backend/internal/handler/api.go`)

| 路由前缀 | 核心端点与职责 |
| --- | --- |
| `/api/health`, `/api/system/version` | 服务存活、数据库/Schema 就绪检查、构建版本信息。 |
| `/api/public/appearance*` | 启动外观配置 (`GET /api/public/appearance`) 与品牌静态资源槽位读取。 |
| `/api/workspace/*` | 本地工作区启动契约、模型渠道与偏好配置读写。 |
| `/api/beefapi/connection*` | 官方算力渠道连接开始、状态查询、取消、断开与模型目录刷新。 |
| `/api/canvas-projects/*`, `/api/projects/*` | 画布 CRUD、SSE 变更流 (`/events`)、快照历史、剧集项目、章节与分镜工作台。 |
| `/api/resources/*`, `/api/assets/*` | 本地媒体文件上传/分片上传、元数据补齐 (`?metadata=ensure`)、H.264 预览生成 (`/playback`)、预览缓存清理 (`/playback-cache`)、容量统计 (`/storage-usage`)、个人资产与文件夹 CRUD。 |
| `/api/tasks/*` | 异步生成任务创建、详情查询、取消、恢复与事件流。 |
| `/api/ai/*` | 外部模型目录探测 (`POST /api/ai/models`) 与自定义模型协议代理转发。 |
| `/api/media-upscale/*` | Topaz 图片/视频超分能力查询 (`/capabilities`)、费用估算 (`/estimate`)、任务提交 (`/tasks`) 与失败恢复 (`/tasks/:id/recover`)。 |
| `/api/canvas-workflows/*`, `/api/canvas-workflow-runs/*` | 可复用工作流模板 CRUD、编辑器投影 (`/editor`)、定义校验 (`/validate`)、可用操作目录 (`/canvas-workflow-operations`)、工作流运行/重试/取消。 |
| `/api/timeline/*`, `/api/depth-captures` | 时间线只读渲染计划编译 (`/render-plan`)、本机成片渲染 (`/renders`)、语音转写 (`/transcriptions`)、深度动作捕捉 (`/depth-captures`)。 |
| `/api/creation-conversations/*` | `/create` 创作室会话 CAS 同步、导入与删除。 |
| `/api/skills/*`, `/api/plugins/*` | 技能市场搜索、版本详情、安装、回退、私人改写、投稿计划；插件列表与状态管理、Eagle 素材库代理。 |
| `/api/ops*`, `/api/assistant/*` | 统一操作层 (`GET /api/ops`, `POST /api/ops/:op`)、外部 MCP/CLI 客户端管理 (`/api/ops/clients*`)、内置助手会话鉴权 (`/api/assistant/ui-session`)、对话流 (`/api/assistant/chat`) 与整片制作提案确认 (`/api/assistant/turns/:turnId/proposals/:proposalId/production`)。 |
| `/api/diagnostics/*` | 客户端错误上报、诊断包预览与脱敏 ZIP 导出。 |
