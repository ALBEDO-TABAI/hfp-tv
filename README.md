# hfp - all in one

[中文](README.md) · [English](README.en.md)

**High-Performance · Local-First · AI-Native Video Screening Room & Infinite Canvas Studio**

**hfp - all in one** 是基于开源项目 [BeefTV](https://github.com/glanderness/BeefTV) 构建的 AI 视频与短剧创作工作台，并以 **「Ready Library」** 的艺术指导（Art Direction）与设计系统进行了全面重构：采用 Inter / Inter Tight / Instrument Serif / Geist Mono / Noto Sans & Serif SC 混排体系、点阵放映桌面、液态玻璃光学表面（Liquid Glass）、信号橙（Print Signal `#ff5a1f` / `#ff6a2c`）焦点系统以及严格的 12 列响应式栅格。

---

## 非官方衍生版本声明 · Unofficial Derivative Notice

**中文说明**
- **非官方独立修改版**：`hfp - all in one`（仓库地址：`https://github.com/ALBEDO-TABAI/hfp-tv`）是基于上游公开项目 [glanderness/BeefTV](https://github.com/glanderness/BeefTV) 独立修改而成的衍生版本，未获得 BeefTV 官方背书。
- **主要变更摘要**：
  1. 产品品牌与用户界面全面重命名为 `hfp - all in one`（含全新胶片书脊标志、斜体衬线字标与海报式启动幕布）。
  2. 保留上游 100% 功能与本地优先架构（详见 [`docs/FEATURES.md`](docs/FEATURES.md)）。
  3. 依据「Ready Library」设计语言对全部视觉、UI 组件、液态玻璃材质、双主题色板、12 列响应式栅格与交互动效进行了彻底重做（详见设计规范 [`docs/DESIGN.md`](docs/DESIGN.md)）。
- **开源与源码可用许可**：本仓库免费公开分发，并严格遵循与上游相同的许可证条款——即 [`LICENSE`](LICENSE)（**Part A: `BeefTV Source Available License 1.0`** 与 **Part B: 历史 `MIT` 许可证**），完整保留 [`NOTICE`](NOTICE)、[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) 及 [`licenses/`](licenses/) 下的全部第三方与字体/光学算法许可文件。

**English Notice**
- **Unofficial, Independently Modified Version**: `hfp - all in one` (`https://github.com/ALBEDO-TABAI/hfp-tv`) is an unofficial, independently modified derivative based on the open-source project [glanderness/BeefTV](https://github.com/glanderness/BeefTV). **It is not endorsed by BeefTV.**
- **Summary of Changes**:
  1. Rebranded user-facing surfaces and package metadata to `hfp - all in one` with a new film-spine emblem, serif-paired wordmark, and poster-sheet splash screen.
  2. Preserved 100% of upstream features and local-first workspace capabilities (see [`docs/FEATURES.md`](docs/FEATURES.md)).
  3. Completely redid the art direction, visual design, liquid-glass material surfaces, typography pairing, 12-column responsive grid, and interactions following the Ready Library design system (see [`docs/DESIGN.md`](docs/DESIGN.md)).
- **License & Free Distribution**: Distributed free of charge under the exact same [`LICENSE`](LICENSE) (**Part A: `BeefTV Source Available License 1.0`** + **Part B: historical `MIT` License**), with [`NOTICE`](NOTICE), [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md), and [`licenses/`](licenses/) retained in full.

---

## 核心特性

1. **100% 完整保留上游能力**：
   - **无限画布工作台 (`/canvas/:id`)**：自由画布与短剧流水线双起点，支持图片、视频、音频、文本、分镜表、批量创作表、3D 导演台、AI 超分（Topaz）、Excalidraw 绘图、文件夹与可复用工作流节点。
   - **专业媒体工具**：Vidstack 视频播放器（悬停静音预览、H.265 按需转 H.264 代理）、行内视频裁剪、视频截帧、音频提取、多视频合并、图片 3D 多角度机位与光影预设、4/9/16/25 及 5×5 自定义宫格切分、Seedream 图层拆分。
   - **hfp - all in one Agent 创作助手与可复用工作流**：支持多轮对话分镜提案、整片批量生成确认、多模态结果审查，以及将画布节点封装为带版本管理的可复用工作流模板。
   - **独立多模态创作室 (`/create`)**：支持视频、图片、文本三模式生成，`@` 智能引用素材与技能、内置 AI 提示词优化器、分镜轨预览与历史会话管理。
   - **时间线成片剪辑器 (`/projects/:id/editor`)**：200 步撤销/重做命令状态机、8 个内置片段/字幕插件、本地 `whisper.cpp` / 云端语音转写字幕、本机 FFmpeg 成片渲染导出。
   - **个人资产库 (`/assets`)、技能市场 (`/skills`)、模型配置与诊断 (`/settings`)、MCP 外部工具接入 (`/agents`)、插件中心 (`/plugins`)**。
2. **「Ready Library」艺术指导设计系统**：
   - 详见 [`docs/DESIGN.md`](docs/DESIGN.md) 与全量功能盘点 [`docs/FEATURES.md`](docs/FEATURES.md)。

---

## 本地运行指南

### 环境要求
- **Bun** `>= 1.4.4`（安装前端与助手依赖）
- **Node.js** `>= 24.15`（运行本地创作助手）
- **Go** `>= 1.25`（编译并启动本地 SQLite 与资源服务后端）

按[本地开发指南](docs/local-development.md)完成前端依赖、模型插件、助手依赖与启动配置，再依次启动后端和前端。浏览器入口为 `http://127.0.0.1:43123`，开发数据保存在被 Git 忽略的 `.local/project-workbench-debug`。

## 开发文档

- [仓库地图](docs/repository-map.md)：目录职责、主链路与常用修改入口。
- [开发规范](CONTRIBUTING.md)：改动边界、验证与提交要求。
- [本地开发与重启](docs/local-development.md)：可执行步骤、数据保护和身份配置。
- [故障排查](docs/troubleshooting.md)：保存失败、插件缺失、助手与模型错误。
- [部署指引](docs/deployment.md)：桌面版、开发服务与团队服务器怎么选，团队服务器的部署、升级与备份。
- [团队服务器](docs/content/docs/backend/team-server.mdx)：企业内多人部署——账号登录、每人一个独立工作区、创意工坊、站内技能市场（尚未在生产环境验收）。
- [完整文档索引](docs/index.md)与 [AI 协作约定](AGENTS.md)。

---

## 许可证与致谢 (License & Attribution)

- 本项目基于 [glanderness/BeefTV](https://github.com/glanderness/BeefTV) 衍生开发，完整保留上游 [`LICENSE`](LICENSE)（Part A: `BeefTV Source Available License 1.0` 与 Part B: 历史 `MIT` 许可）、[`NOTICE`](NOTICE) 与 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。
- 视觉设计语言、UI 字体打包（Inter、Inter Tight、Instrument Serif、Geist Mono、Noto Sans SC、Noto Serif SC，SIL OFL 1.1）与液态玻璃折射贴图算法（源自 `Glass-HQ/liquid-glass`，MIT）承袭自「Ready Library」，相关许可证完整保留于 [`licenses/ui/`](licenses/ui/)。
