# hfp - all in one 本地开发

本页是当前浏览器开发方式的操作入口。前端固定使用 `127.0.0.1:43123`，Go 服务使用 `127.0.0.1:43180`，数据保存到仓库内被忽略的 `.local/project-workbench-debug`。

## 环境

需要 Go 1.25 或更新版本、Bun 1.4.4 或更新版本、Node.js 24.15 或更新版本，以及编译 SQLite 所需的 C 编译器。插件打包使用 `sh`、`zip` 和 Node。macOS 的 C 编译器来自 Xcode Command Line Tools。

在仓库根目录检查：

```bash
go version
bun --version
node --version
cc --version
```

助手依赖清单使用 Bun lockfile v2；Bun 1.3.9 无法读取。Bun 用于安装依赖，助手实际由 Node 运行。不要用 npm/pnpm 重建项目的 `node_modules` 或替换锁文件。

## 首次准备

以下代码块从仓库根目录执行；`HFPTV_ROOT` 保存绝对路径，Go 的缓存目录不能使用相对路径。

```bash
export HFPTV_ROOT="$PWD"
mkdir -p .local/project-workbench-debug .local/cache/go-build .local/cache/go-mod .local/cli .local/logs
export GOCACHE="$HFPTV_ROOT/.local/cache/go-build"
export GOMODCACHE="$HFPTV_ROOT/.local/cache/go-mod"

(cd web && bun install --frozen-lockfile)
(cd agent-host && bun install --frozen-lockfile)
./plugin-packages/build-packages.sh

node <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const file = '.local/project-workbench-debug/agent_config.json';
if (!fs.existsSync(file)) {
  fs.writeFileSync(file, JSON.stringify({
    hostCommand: process.execPath,
    hostArgs: [path.resolve('agent-host/server.mjs')]
  }, null, 2), { mode: 0o600 });
}
NODE

(cd backend && go build -o ../.local/beeftv-server ./cmd/server)
(cd backend && go build -o ../.local/cli/beeftv ./cmd/beeftv)
```

已有助手配置会保留。移动仓库或 Node 后，需核对该文件里的启动位置。插件打包脚本会同步嵌入的接口说明，执行后检查 `git diff`，不要把无关的生成改动混入提交。

## 启动

先运行后端。终端一，从仓库根目录执行：

```bash
cd backend
GIN_MODE=release \
BEEFTV_UI_BOOTSTRAP=1 \
BEEFTV_ALLOWED_ORIGINS=http://127.0.0.1:43123 \
CANVAS_CORS_ORIGINS=http://127.0.0.1:43123 \
CANVAS_BACKEND_ADDR=127.0.0.1:43180 \
CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug \
../.local/beeftv-server 2>&1 | tee ../.local/logs/backend.log
```

确认后端已监听，再运行前端。终端二，从仓库根目录执行：

```bash
cd web
VITE_API_PROXY_TARGET=http://127.0.0.1:43180 \
BEEFTV_DEV_OWNER_TOKEN_FILE=../.local/project-workbench-debug/agent_owner_token \
bun run dev -- --host 127.0.0.1 --port 43123 --strictPort 2>&1 | tee ../.local/logs/web.log
```

打开 **http://127.0.0.1:43123/**。不要改用 `localhost` 或其他端口：开发身份转发限定在当前 `127.0.0.1` 同源页面；`--strictPort` 避免端口占用时悄悄换端口。

此开发方式只用于本机。Go 识别现有工作区 owner 凭据，Vite 仅对符合本机地址、来源与操作路径限制的请求附加身份；凭据不交给页面。`BEEFTV_UI_BOOTSTRAP=1` 允许本地开发页面取得助手会话，不等于画布保存身份，也不能用于对外部署。

## 启动验收

```bash
curl --fail --silent --show-error http://127.0.0.1:43123/api/health/ready
curl --fail --silent --show-error http://127.0.0.1:43123/api/assistant/status
```

还必须检查界面：

1. 首页和已有项目能打开；在专用测试画布添加文字节点，等待“已保存在本地”，刷新后仍能读取。
2. 模型设置的接口目录包含所需插件；只看模型名称不能证明其接口可用。
3. 配置文本模型后，助手面板能启动并读取对话；`available: true` 只证明本地助手就绪。
4. 获得真实调用授权后，用一条简短对话验证模型回复；生成验收还需确认结果已下载、显示和保存。中转站连通性与本地启动分别判断。

## 数据与重启

- 先保存页面，确认没有运行中的生成和助手回合；再在启动终端按 `Ctrl+C`，等待进程退出。
- 数据库、资源、密钥、插件状态和助手会话都在 `.local/project-workbench-debug`。重启时继续使用同一目录，不删库、不换空目录。
- 关闭服务后完整复制数据目录才是便于恢复的备份；运行中不要只复制 SQLite 主文件，尚未合并的写入可能仍在 WAL 中。
- 改 Go 源码需要重新编译后端并重启；改前端源码通常由 Vite 自动更新；改助手源码需要重启助手或后端；修改插件后需要重新打包并重启后端。
- 日志保存在 `.local/logs/`，上述 `tee` 命令每次启动覆盖同名日志；需要保留时先另存。分享诊断前检查并移除凭据、提示词与私人素材内容。

## 原生桌面与其他运行方式

Wails 桌面入口是 `backend/cmd/desktop`，会通过原生绑定提供启动身份；不能把浏览器开发代理当作正式桌面认证。

`scripts/beeftv-shared-dev.sh` 当前启动 Vite 的端口为 `3000`，而 `backend/cmd/desktop/wails.json` 的前端地址为 `43123`。原生联调前必须对齐，不能直接套用旧说明中的已验证结论。桌面构建、安装和打包依赖见[桌面开发](desktop-local-development.md)与[桌面发布](desktop-release.md)。

仓库还保留 Compose 配置；它们与本页的浏览器开发入口不同。使用前核对 Dockerfile、数据卷、插件与助手的装配，不把已有配置文件视为当前环境的成功部署证明。

原生桌面的正式安装位置是 `/Applications/hfp-allinone.app`，更新使用 `scripts/update-local-beeftv-app.sh`。默认数据目录使用系统配置目录下的 `hfp-allinone`；新目录不存在且旧 `BeefTV` 目录存在时沿用旧目录，不搬迁数据。浏览器开发继续使用 `.local/project-workbench-debug`。
