# hfp - all in one 仓库地图

用于定位当前分支的代码。首次开发先读[开发规范](../CONTRIBUTING.md)，运行项目看[本地开发](local-development.md)，定位故障看[排查指南](troubleshooting.md)。目录存在不等于功能已通过实机验收。

## 项目组成

| 目录 / 文件 | 职责 | 首先查看 |
| --- | --- | --- |
| `web/` | React 界面、画布状态、媒体工具、请求与本地缓存 | [application.tsx](../web/src/application.tsx)、[router.tsx](../web/src/router.tsx) |
| `backend/` | Go HTTP 服务、SQLite、资源、任务及模型调用 | [server/main.go](../backend/cmd/server/main.go)、[bootstrap/runtime.go](../backend/internal/bootstrap/runtime.go) |
| `agent-host/` | Node 创作助手；官方 Pi Durable 管理模型和工具执行 | [server.mjs](../agent-host/server.mjs)、[durable-session-owner.mjs](../agent-host/durable-session-owner.mjs) |
| `plugin-packages/` | 各模型接口的声明式插件源码及打包脚本 | [build-packages.sh](../plugin-packages/build-packages.sh) |
| `plugins/beeftv-skills/` | 面向外部 Agent 的技能包 | [README](../plugins/beeftv-skills/README.md) |
| `scripts/` | 构建、发行和专项验收脚本 | [桌面发布说明](desktop-release.md) |
| `tools/depth-capture/` | 可选的深度捕捉工具 | [README](../tools/depth-capture/README.md) |
| `fixtures/`、`web/test/`、Go `*_test.go` | 合同样本、测试与回归用例 | 与改动模块相邻的测试 |
| `docs/` | Markdown / MDX 文档和历史证据；当前没有独立文档站构建工程 | [文档索引](index.md) |
| `assets/`、`licenses/` | 品牌、美术资源和第三方许可 | [NOTICE](../NOTICE)、[第三方声明](../THIRD_PARTY_NOTICES.md) |
| `.local/` | 被 Git 忽略的本机配置、数据库、缓存、日志和开发程序 | [数据保护](local-development.md#数据与重启) |

## 三条主要链路

```text
画布编辑 → use-canvas-store → local-workspace-repository
         → /api/ops/canvas.document.commit → operations → canvas / repository

图片、视频、音频生成 → generation-task → /api/tasks → task / app
                   → generation → 已安装的 protocol 插件 → 模型渠道
                   → taskdelivery → taskbinding → 资源与画布

创作助手面板 → /api/assistant/* → handler / assistantruntime
             → agent-host/server.mjs → durable-session-owner.mjs → 模型
             → /api/ops/* → Go 的业务权限与实际操作
```

保存成功、任务提交、模型生成成功、结果下载完成、绑定回画布是不同阶段，不能用前一阶段的成功替代后一阶段的验证。

## 前端入口与改动位置

路由以 [router.tsx](../web/src/router.tsx) 为准；页面模块加载见 [workspace-route-modules.ts](../web/src/lib/workspace-route-modules.ts)。

| 用户入口 / 需求 | 主要位置 | 关联边界 |
| --- | --- | --- |
| 首页 `/` | `web/src/pages/home/` | 公共外壳在 `web/src/layouts/user-layout.tsx` |
| 项目库 `/projects`、`/project`、`/canvas` | `web/src/pages/canvas/index.tsx` | 本地项目详情路由会转入画布 |
| 画布 `/canvas/:id` | `web/src/pages/canvas/project.tsx`、`web/src/components/canvas/` | 状态在 `web/src/stores/canvas/`；算法在 `web/src/lib/canvas/` |
| 画布保存与冲突 | `web/src/services/local-workspace-repository.ts`、`canvas-operation-journal.ts`、`canvas-revision-conflict.ts` | 服务端版本确认与浏览器草稿分开处理 |
| 独立创作室 `/create` | `web/src/pages/create/` | 使用统一生成任务与结果回收链路 |
| 资产 `/assets` | `web/src/pages/assets/`、`web/src/services/workspace-asset-repository.ts` | 本地资源与资产记录需一致 |
| 模型设置 `/settings` | `web/src/pages/settings/`、`web/src/stores/use-config-store.ts` | 保存入口 `model-config-repository.ts`；模型名称、渠道与接口类型不可混为一项 |
| 助手面板 | `web/src/pages/canvas/canvas-assistant-*`、`web/src/services/api/agent-assistant.ts` | UI 会话、流式输出、历史恢复与生成确认 |
| 技能 `/skills` | `web/src/pages/skills/` | 远程市场与本机已安装技能是不同数据源 |
| 外部 Agent `/agents` | `web/src/pages/agents/` | 外部客户端登记、命令与权限 |
| 插件 `/plugins` | `web/src/pages/plugins/`、`web/src/services/plugin-host.ts` | 页面受功能开关约束 |
| 主题、字体与组件外观 | `web/src/styles/`、`web/src/lib/app-theme.ts`、`web/src/components/ui/` | [设计规范](DESIGN.md)，不要增加无范围的全局样式覆盖 |

`/tasks` 当前跳转首页；生成记录通过画布内入口查看。开发实验页位于 `/dev/*`，只在开发构建注册。浏览器使用普通路径，原生桌面窗口使用 hash 路由。

## 后端职责

| 位置 | 负责 | 不应放入 |
| --- | --- | --- |
| `backend/cmd/server/` | 浏览器开发服务入口、环境变量和进程退出 | 页面业务逻辑 |
| `backend/cmd/desktop/` | Wails 原生窗口和桌面配置注入 | 另一套业务服务 |
| `backend/cmd/beeftv/` | CLI / MCP 入口 | 绕过业务接口直接改数据库 |
| `backend/internal/bootstrap/`、`localapp/` | 组合服务、生命周期和窄接口 | 新的巨型业务入口 |
| `backend/internal/handler/` | 路由、输入、身份和响应 | 数据库查询与供应商私有协议 |
| `backend/internal/operations/`、`agentops/` | 业务操作登记、调用者权限和重复请求处理 | 独立于页面的第二套业务实现 |
| `backend/internal/workspace/`、`canvas/`、`project/`、`asset/`、`task/` | 各领域的数据规则 | 页面状态或跨层依赖 |
| `backend/internal/app/` | 跨领域协调及尚未拆出的实现 | 无必要的新通用封装 |
| `backend/internal/generation/`、`protocol/`、`provider/`、`outbound/` | 模型请求、接口适配、错误、网络访问 | 绕过既有出口的临时 HTTP 客户端 |
| `backend/internal/plugins/` | 插件安装、启用和实际可用接口目录 | 把源码目录当成已安装插件 |
| `backend/internal/taskdelivery/`、`taskbinding/` | 结果落地与绑定回原目标 | 再次发起付费生成 |
| `backend/internal/assistantruntime/`、`assistantturns/` | 助手子进程、回合授权、确认与恢复 | 第二套模型执行循环 |
| `backend/internal/database/`、`repository/`、`model/` | 数据库结构、查询和数据类型 | UI 与请求策略 |

更细的领域说明见[后端代码地图](content/docs/backend/code-map.mdx)，请求入口以 [OpenAPI 文件](../backend/internal/handler/openapi.yaml) 和实际 handler 为准。

## 配置与依赖的来源

- 版本：根目录 [VERSION](../VERSION)；发行脚本注入前后端版本与提交信息。
- 前端依赖：`web/package.json` + `web/bun.lock`；助手依赖：`agent-host/package.json` + `agent-host/bun.lock`；Go 依赖：`backend/go.mod` + `backend/go.sum`。
- 浏览器转发与本地开发身份：[web/vite.config.ts](../web/vite.config.ts)。原生桌面身份：[desktop-runtime.ts](../web/src/services/desktop-runtime.ts) 与 `backend/internal/bootstrap/`。
- 模型配置与凭据：本地工作区管理，不能写入仓库、画布快照或日志。详见[模型配置持久化](local-model-config-persistence.md)。
- 画布、任务、素材元数据以本地服务为准；未确认的浏览器改动必须保留。详见[数据源说明](local-workspace-data-sources.md)。
- `*.beeftv-plugin` 是生成的运行文件，仓库只追踪插件源码。克隆后需要打包，不能只安装前端依赖。

## 文档维护

改目录职责或主链路时更新本页；改启动命令时更新[本地开发](local-development.md)；新故障的明确原因与恢复方法放入[排查指南](troubleshooting.md)。`refactor/`、`evidence/` 和 `release-evidence/` 中的记录只证明其描述的版本与环境，不自动代表当前分支已经验收。
