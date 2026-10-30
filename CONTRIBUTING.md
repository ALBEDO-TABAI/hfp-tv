# 贡献指南

本仓库是 hfp - all in one，基于 BeefTV 独立修改。日常开发按本页执行；AI 协作还需遵守 [AGENTS.md](AGENTS.md)。保留上游及第三方许可和署名。

## 开始前

1. 用一句话描述要修的问题、预期行为和完成标准；不确定的需求先说清。
2. 检查 `git status --short`，识别已有修改；不要覆盖或顺手整理其他人的工作。
3. 从[仓库地图](docs/repository-map.md)定位入口、调用者、数据所有者和相邻测试，再决定改动位置。先判断改动影响哪种运行形态：桌面、单工作区开发服务、团队服务器，或三者共用的工作区内核。
4. 按[本地开发](docs/local-development.md)启动。仅安装前端依赖不足以运行模型插件和创作助手。`bun` 不在系统 PATH 时，用仓库内的 `.local/node_modules/.bin/bun`。

当前仓库没有 `.env.example` 或独立文档站的 `package.json`。环境变量以实际入口、Vite 配置和启动说明为准，不沿用已经缺失的文件或命令。

## 改动边界

| 改动内容 | 应遵守的规则 |
| --- | --- |
| 页面与交互 | 页面协调业务；公共组件、状态与算法各归原目录。先使用已有主题变量，遵守[设计规范](docs/DESIGN.md)。 |
| HTTP 请求 | 业务 JSON 统一走 `services/api/request.ts` 的 `http`；保留错误码、取消和超时语义。 |
| 保存 | 浏览器草稿与服务端确认分开。保存失败必须可见；版本冲突保留用户修改。 |
| Go 业务 | `handler → localapp/领域接口 → app/领域实现 → repository/model`；handler 不直接查库，领域不反向依赖巨型入口。 |
| 模型调用 | 沿用 generation / protocol / outbound；参数由所选接口定义，不能仅凭模型名推断地址。 |
| 生成与恢复 | 提交、结果下载、资产登记与画布绑定分别确认；未知状态不自动重发付费任务。 |
| 创作助手 | Go 管授权与业务，官方 Pi Durable 管执行；旧会话只读，不新增另一套模型循环。详见[复用矩阵](docs/refactor/pi-official-reuse.md)。 |
| 身份与数据 | 继续校验资源归属、来源和权限；开发身份只用于本机，不借修复问题关闭验证。 |
| 团队服务器 | 平台（`teamserver`、`accounts`、`workshop`、`skillmarket`）和工作区内核之间只走窄接口：网关转交请求、`ProjectSnapshots` 导出导入。平台不直接读写工作区数据库，内核不感知平台账号与角色。 |
| 托管内核 | 新增的内核接口如果会读服务器文件、启动进程、安装可执行组件或签发本机凭据，必须加进 `bootstrap/hosted.go` 的拒绝清单并补测试。本机可信判断统一认 `httptransport.HostedOwner`，不要新写只看回环地址的检查。 |
| 团队模式前端 | 本地模式（`/api/platform/me` 返回 404）行为保持不变；团队专属入口按 `capabilities` 显示。错误按 `ApiError.reason` 处理，401 交给统一的登录失效处理。 |

优先修正负责该问题的原实现。只有真实重复或明确协议边界才引入封装；不做无关重构、格式化、依赖升级或预防性兼容层。

## 验证

按变化选择最小充分检查。纯文档修改检查路径、链接和命令；无需因此运行全仓测试。代码问题先复现，再运行关联测试；下面列出可选入口，不要求每次全跑。

| 改动范围 | 命令 / 实机检查 |
| --- | --- |
| 前端类型与构建 | `cd web && bun run build`；仅类型检查为 `bun run typecheck` |
| 本地基础合同 | `cd web && bun run pretest` |
| 画布交互 | `cd web && bun run test:canvas`，另验证实际拖拽、缩放、弹窗和保存 |
| 指定前端模块 | `cd web && bun test test/<相关文件>` |
| 后端领域 | `cd backend && go test ./internal/<相关包>`；跨领域或发布时按需要扩大范围 |
| 助手生命周期 | `cd backend && go test ./internal/assistantruntime` |
| 助手宿主 | `cd agent-host && bun test durable-session-owner.test.mjs`（由测试启动 Node 验证 SQLite 与恢复） |
| 团队服务器 | `cd backend && go test ./internal/teamserver ./internal/accounts ./internal/workshop ./internal/skillmarket ./internal/projectsnapshot ./internal/kernelpool`，以及 `go test ./internal/bootstrap -run 'Hosted\|ProjectSnapshot'`；这些测试会打开真实托管内核，覆盖隔离、权限、复制与签名 |
| 团队模式界面 | 构建前端后用 `teamserver` 二进制在临时数据目录和非开发端口托管 `web/dist`，用无头浏览器走登录与相关页面；不要连正式数据 |
| 文档 | 确认相对链接对应真实文件；启动命令在根目录与新终端的上下文中能成立 |

全量 `go test ./...` 在装了 Homebrew ffmpeg 8.1 的 Mac 上，`internal/editing`、`internal/app`、`internal/handler` 的字幕渲染相关用例和 `internal/generation` 的一个 DNS 计时用例会失败，与业务改动无关；判断时先在未改动的提交上复现。

测试通过只证明其覆盖的条件。用户可见修复需要真实页面验证；本地就绪不等于外部模型成功，构建成功不等于已安装应用可用。验证所需的真实模型调用可能产生费用，按用户授权范围执行。

## 数据与诊断

开发数据库、素材、运行配置、缓存和日志使用 `.local/`。不要提交 `.env`、数据库、上传或生成文件、编辑器配置、密钥和已安装依赖。调试时不打印渠道配置全文、认证头或用户素材正文。

现有数据迁移或清理前先证明备份可恢复；普通修复不删库。错误先按[排查指南](docs/troubleshooting.md)定位到具体阶段，同一失败不盲目重复。

## 提交要求

- 保持改动聚焦，沿用现有目录职责、命名和错误处理风格。
- 写路径必须明确失败；不可用默认值掩盖保存、权限、生成或删除错误。
- 后端对象读取必须同时校验当前用户和资源归属。
- 涉及上传、外部请求或模型调用时，必须说明大小、频率、费用和 SSRF 边界。
- 用户可见文案使用中文；核心业务入口和安全边界使用简短中文注释说明原因。

本项目由负责人一人维护：改动直接提交并推送到 `ALBEDO-TABAI/hfp-tv` 的 `main`，不需要 Pull Request 和交叉 review，也不推送到上游 BeefTV 仓库。AI 代理只在负责人明确要求时提交和推送，发版仍需单独授权。

- 提交格式：`fix(canvas): 画布保存 - 修复具体问题`，类型使用 `feat|fix|refactor|perf|docs|test|build|ci|chore|revert`。一组相关改动按模块拆成几个提交（例如后端、前端、文档），提交说明写清改了什么、为什么、如何验证。
- 按明确文件暂存，检查 `git diff --cached` 与 `git diff --cached --check`；不对混有其他工作或本机文件的目录使用 `git add -A`。
- 推送前 `git fetch` 确认远端 `main` 没有新提交，只做快进推送；推送后读回远端提交，不能把本地提交成功当作已经同步。
- 改接口、数据结构或运行边界时更新相应专题；改开发入口时同步中英文 README。历史验收记录不代替当前验证。

## 贡献许可

自引入新许可的提交起，首次提交的新增原创内容默认按 [`LICENSE` 的 Part A（BeefTV Source Available License 1.0）](LICENSE)贡献。你应有权授予这些权限，保留已有署名和许可证通知，并标明第三方或历史 MIT 材料的来源与适用条款。不要将他人的代码、素材或雇主所有的成果作为无权授权的原创贡献提交。

按 Part A 有意提交的原创贡献，还依其第 8 条向官方 `glanderness/BeefTV` 仓库维护者授予复制、修改、分发，以及按社区许可或单独商业条款再许可该贡献的非独占、永久、不可撤销、全球、免版税著作权许可。贡献者保留版权；这不是版权转让，也不追溯适用于历史贡献或扩大至第三方材料。若不接受此项授权，应在接受贡献前明确提出，由维护者先确认处理方式。

已按 MIT 发布的贡献继续保留 MIT 权利；因 GPL 等条款需要使用兼容许可的内容不能强行套用 Part A，贡献中应明确标识并说明兼容性。场景和范围说明见[许可说明](docs/content/docs/overview/licensing.mdx)。
