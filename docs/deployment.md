# 部署指引

本页说明 hfp - all in one 该用哪种方式运行，以及团队服务器的部署、升级、备份和日常运维。信任边界、接口和环境变量的完整说明见[团队服务器](content/docs/backend/team-server.mdx)。

## 选哪种方式

| 场景 | 用法 | 说明 |
| --- | --- | --- |
| 企业内多人使用 | **团队服务器**（`deploy/teamserver/`） | 账号登录、每人一个独立工作区、公司中转站渠道、创意工坊、站内技能市场。唯一推荐的服务器部署方式 |
| 一个人在自己电脑上用 | 桌面应用 `hfp-allinone.app` / `hfp-allinone.exe` | 见[快速开始](../QUICKSTART.md)与[桌面发布](desktop-release.md) |
| 开发调试 | 单工作区开发服务 + Vite | 见[本地开发](local-development.md) |

根目录的 `docker-compose.yml` 是早期的单工作区、无登录形态，不要用于服务器部署：能访问端口的人就能使用工作区；画布保存要求本机 owner 凭据，经 nginx 访问时会被拒绝（按代码推断，未在服务器上实测）。

## 团队服务器：首次部署

### 准备

- 一台 Linux 服务器，Docker 24 以上（Compose 插件 2.24 以上），建议 4 核 8 GB 内存起步。每个正在使用的成员占一个工作区内核，用过创作助手的再多一个 Node 进程；工作区空闲 30 分钟后自动关闭释放内存，下次访问自动打开。
- 访问方式二选一：
  - **只在公司内网用 IP 访问**（默认，最简单）：不需要域名和证书，只占一个端口，浏览器用 `http://服务器IP:端口` 打开。账号密码在内网以明文传输，适合可信的办公网络。
  - **有域名**：解析到这台服务器并开放 80 和 443 端口，Caddy 自动申请 HTTPS 证书。
- 公司中转站的地址和可用模型清单，以及给每个成员发的中转站密钥。

### 步骤

```bash
git clone https://github.com/ALBEDO-TABAI/hfp-tv.git
cd hfp-tv/deploy/teamserver
cp .env.example .env                                             # 部署设置，见第 2 步
cp config/relay-channel.example.json config/relay-channel.json   # 按中转站实际情况修改
cp config/users.example.txt config/users.txt                     # 成员账号，见第 3 步
cp config/user-keys.example.txt config/user-keys.txt             # 成员的中转站密钥
```

1. **编辑公司中转站渠道** `config/relay-channel.json`：`baseUrl`、`apiFormat`、`models` 和 `modelProfiles` 与模型配置页里的渠道格式相同，不要写密钥。最省事的做法是先在桌面版里把中转站配好、能出图，再按那个渠道的字段抄过来。这个渠道会出现在每个成员的“模型配置”里，请求都发往这个地址，各自用自己的密钥；`baseUrl` 是内网地址（如 `http://10.10.10.45:3000/v1`）时会自动加入内网放行名单。管理员自己在“模型配置”里添加的服务只属于管理员本人的工作区，不会分发给其他成员。上线后也可以不再改文件：管理员在“模型配置”里点公司中转站卡片的“编辑（所有成员）”增删模型，保存后所有成员刷新页面即可使用；网页保存的定义优先于这个文件。

   每个模型在 `modelProfiles` 里写明能力和接口协议，否则视频模型会按旧的通用视频接口发送：

   | 模型 | `capability` | `protocol` | 其他 |
   | --- | --- | --- | --- |
   | 文本 / 助手 | `text` | `chat-completion` | |
   | OpenAI 兼容生图 | `image` | `openai-image` | |
   | Seedance（YSXXAI，经公司 New API） | `video` | `video-generations-json` | `baseUrl` 写中转站的 `/ysxxai-seedance` 入口；参考图服务和各素材传输方式见示例配置 |
   | 可灵 4 / 万相 3（YSXXAI，经公司 New API） | `video` | `video-generations-json` | `baseUrl` 写中转站的 `/ysxxai-video` 入口；不配参考图服务 |
   | 火山方舟原生视频 | `video` | `volcengine-ark-video` | `baseUrl` 写实际支持方舟原生接口的地址 |

   模型自己的 `baseUrl` 必须与渠道 `baseUrl` 同协议、同主机和端口，否则启动时报错；成员密钥只会发往这个中转站。模型名以中转站 `/v1/models` 实际返回的为准。

   **YSXXAI Seedance**：先在公司 New API 安装 `deploy/teamserver/ysxxai-task-plugin.js`，创建该插件的渠道并在渠道里保存 YSXXAI 的 `seedance` 分组密钥；工作台仍使用成员自己的公司中转站密钥。七个模型名及其分辨率必须与上游完全一致，时长为 4～15 秒；Fast 和 Mini 不接收参考素材。普通图片由参考图服务转为临时公网 HTTPS 链接；视频和音频需要已有公网 HTTPS 链接或另配对象存储。画布或素材库里的参考图在提交前自动登记为虚拟人像素材并等待审核；真人素材需按上游要求认证和授权。素材组由管理员预先配置，不复用旧德云素材编号。

   YSXXAI 可信素材登记需先升级 8787 参考图服务并设置独立素材组，见 `hfp-allinone-tauri/services/reference-upload-relay/README.md`。七个公司模型的参考图服务地址统一改为 `/v1/ysxxai-reference-images`，生成前的自动登记和画布里的“登记虚拟人像素材”都只发往这个入口；Fast 和 Mini 仍不接受参考素材。网页保存的公司模型配置优先于配置文件，不能只修改示例文件。顺序必须是先升级 8787（`/health` 显示 `ysxxaiAssetReady: true`）再改地址：旧版 8787 没有这个入口，提前改地址会让普通参考图的临时链接也失败。YSXXAI 模型配成旧地址 `/v1/reference-images` 或没配地址时，工作区自动改用同一台参考图服务的 `/v1/ysxxai-reference-images` 并在任务日志注明；只有公司中转站完全没有参考图服务时，任务日志才会写“没有配置虚拟人像素材服务”，参考图按原链接提交。管理员页面的公共“参考图服务地址”也会套用到这七个模型。

   Seedance 2.x 不接受直接用写实人脸图做参考（上游提示 “may contain real person”），与图片放在哪里无关。所以 YSXXAI Seedance 任务在调用模型前，会把画布或素材库里还没登记过的参考图自动登记为虚拟人像素材：8787 把图片传到对象存储，用 YSXXAI 的 `CreateAsset` 登记进预设素材组，轮询 `GetAsset` 直到审核通过（`Active`），通常 1～3 分钟，任务最多等 12 分钟；之后以 `asset://` 素材编号提交，同一张图再用时直接复用。审核未通过或登记失败时任务在调用模型前结束，不产生生成费用，原因写在任务日志的“上游原始错误”。成员也可以在画布里右键图片“登记虚拟人像素材”提前登记。所有参考图（包括场景、产品图）都会入库，占用服务商的素材额度。真人只能由本人在火山方舟完成认证授权后入库，不能当虚拟人像登记。

   New API 的七个模型按插件的 `cost` 用量配置为每 credit 1 元：提交时按上游展示的价格预留，完成后按上游返回的最终人民币费用结算。上游尚未返回最终费用时任务会继续等待；长时间未完成时检查中转站日志，不要把预留额当成已确认的实际费用。价格变化时应更新插件中的预留估算。视频结果通过中转站使用原任务下载；不要因等待而重复提交。

   **YSXXAI 可灵与万相（中转站）**：`deploy/teamserver/ysxxai-video-task-plugin.js` 提供 `kling-4.0`、`kling-4.0-flash` 和 `wan3.0-video`；可灵与万相分别使用各自上游分组的密钥。插件支持 `/v1/videos` 和 `/ysxxai-video/v1/video/generations`，按上游价格预留额度，完成后按上游返回的人民币费用结算；为 YSXXAI Video 配置 `cost` 每 credit 1 元。中转站对这三个模型使用独立的严格会话规则，避免提交结果不明时自动重发。部署时应在获得授权后分别确认真实生成、结果取回与最终结算；模型列表连通和插件沙箱检查不能替代这些验证。

   工作台接这三个模型时，中转站插件须为 1.1.0 或更新版本：1.0.0 只原样转发，工作台发出的通用请求体（`images` 对象数组、`generate_audio` 等）会被可灵拒收。管理员在“模型配置 → 公司中转站 → 编辑（所有成员）”读取或手动添加这三个模型，协议会默认选“视频生成 JSON”；再在“可灵 / 万相接口地址”填 `http://<中转站>/ysxxai-video` 并保存。之前已经用别的协议加过这三个模型时，要在高级模型设置里改回“视频生成 JSON”，时长、画幅、参考数量等限制才会换成这三个模型的预设。参考图沿用公司参考图服务转成临时公网链接，不经过素材库审核，也不需要新的对象存储；参考视频和音频与 Seedance 一样需要公网链接。可灵时长 3～30 秒（Flash 3～20 秒），720P/1080P，最多 10 张图和 5 段视频，不接收参考音频；万相 2～30 秒，480P/720P/1080P，可开关声音，首尾帧不能与参考素材混用。

   **参考图中转**：公司参考图服务（旧项目的 `services/reference-upload-relay`，8787 端口）用成员自己的中转站密钥鉴权，将图片写入公司对象存储并返回临时 HTTPS 链接；参考图服务的地址会自动加入内网放行名单。旧的德云线路曾使用该服务登记 `asset://` 素材，不能把它作为 YSXXAI 线路的默认方式。参考视频和参考音频不经这个服务；需要在线链接时仍要成员提供，或另配下面的对象存储。
2. **改部署设置** `.env`（不要提交）：示例文件里已经写好内网部署需要的几项，通常只需要改端口：

   ```bash
   TEAMSERVER_HTTP_PORT=80                      # 80 被占用时改成别的，例如 180
   TEAMSERVER_RELAY_CHANNEL_FILE=/config/relay-channel.json
   TEAMSERVER_USERS_FILE=/config/users.txt
   TEAMSERVER_USER_KEYS_FILE=/config/user-keys.txt
   # 除公司中转站外还要访问的内网地址（主机名或 IP，逗号分隔，不写端口）
   # CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=10.10.10.46
   # 需要公网素材地址的模型：图片已由参考图服务中转；视频、音频也要中转时才配置 S3 兼容存储，可选
   # TEAMSERVER_REFERENCE_STORAGE_FILE=/config/reference-storage.json
   ```

   `.env` 里的每一项都会原样传进容器；对外端口只认 `TEAMSERVER_HTTP_PORT`，容器内的监听端口固定，不要再设 `TEAMSERVER_ADDR` 或 `TEAMSERVER_DOMAIN=:端口`。登录 Cookie 默认按访问方式自动处理（HTTP 访问不加 Secure、HTTPS 访问加 Secure），不需要设置 `TEAMSERVER_COOKIE_SECURE`。

   **有域名时**再加两行，Caddy 会改为自动 HTTPS 并开放 443（此时 `TEAMSERVER_HTTP_PORT` 保持 80）：

   ```bash
   COMPOSE_FILE=docker-compose.yml:docker-compose.https.yml
   TEAMSERVER_DOMAIN=studio.example.com
   ```

3. **写成员名单**：`config/users.txt` 一行一个 `用户名:密码`，`config/user-keys.txt` 一行一个 `用户名:中转站密钥`（格式说明在示例文件开头）。文件里有明文密码，收紧权限：

   ```bash
   sudo chown 10001 config/users.txt config/user-keys.txt && chmod 600 config/users.txt config/user-keys.txt
   ```

   以后加人、改密码、换密钥、停用都只改这两个文件，保存后约 5 秒生效，不用重启。文件写错时整份不生效，`docker compose logs teamserver` 会指出第几行。也可以不用文件，改由管理员在网页“用户管理”里开账号。

4. **构建并启动**：

   ```bash
   docker compose up -d --build
   docker compose logs -f teamserver
   ```

   第一次构建要下载 Go 和前端依赖，耗时较长；之后的构建复用缓存，只编译改动的代码。Go 依赖默认先走国内镜像 goproxy.cn，公司有自己的 Go 代理时在 `.env` 里设置 `GOPROXY`（示例见 `.env.example`）。

   日志里应该看到：“公司模型服务：<地址>，N 个模型”、“允许访问的内网地址：…”、“账号文件已同步”、“内置技能市场”和 “team server listening”。看到“公司模型服务：未配置”说明 `.env` 里的 `TEAMSERVER_RELAY_CHANNEL_FILE` 没有生效（也可以先不写文件，登录管理员后在“模型配置”里点“配置公司中转站”）。

5. **创建管理员**（审核创意工坊作品和技能、查看操作记录需要；密码从标准输入读，不进 shell 历史）：

   ```bash
   read -rs PW; printf '%s\n' "$PW" | docker compose exec -T teamserver teamserver admin create --username admin --display-name 管理员
   ```

6. 用浏览器打开 `http://服务器IP:端口`（端口为 80 时可省略；有域名时为 `https://<域名>`），用账号文件里的账号或管理员登录。管理员在侧栏“管理 → 服务状态”能看到服务器实际读到的公司模型服务、成员密钥绑定情况、内网放行名单和工作区数量。

### 上线前验收

健康检查只说明进程活着。请至少用两个成员账号实际走一遍：

- [ ] 登录、退出；A 退出后在同一浏览器登录 B，看不到 A 的项目。
- [ ] 新建画布、上传图片、刷新后内容还在。
- [ ] 用“公司中转站”渠道出一张图、一段视频，中转站后台能看到用量记在本人的密钥下。
- [ ] 打开浏览器开发者工具的“网络”面板，生成和读取模型时请求里看不到公司中转站的密钥（密钥只在服务器上补上）。
- [ ] 创作助手能回复，并能在画布上新建节点。
- [ ] 创意工坊：A 投稿 → 管理员审核通过 → B 浏览并复制到自己的项目 → B 能继续编辑和生成。
- [ ] 技能市场：A 连接投稿账号、投稿 → 管理员在“技能审核”通过 → B 能安装。
- [ ] 从账号文件删掉一个人（或在“用户管理”停用）后，他的浏览器立即回到登录页；加回来后项目都还在。

## 升级

```bash
cd hfp-tv/deploy/teamserver && ./update.sh
```

`update.sh` 依次拉取代码、带上提交号重建镜像，并等新容器通过健康检查后才打印“现在可以测试”。构建和旧容器停止（最长 3 分钟）期间旧版本仍在服务，这段时间提交的任务按旧版本执行，验证新功能要等脚本结束。之后成员“复制排查信息”里的“构建提交”和“运行版本”会带上提交号，可据此确认任务由哪个版本执行。不用脚本时，手动执行 `BUILD_COMMIT=$(git rev-parse --short HEAD) docker compose up -d --build` 效果相同，但要自己确认新容器已启动。

数据都在 `teamserver-data` 卷里，升级不会清除。平台库和各工作区数据库在启动时自动迁移；迁移失败时进程会退出并在日志里说明原因，旧数据不会被改动。大版本升级前先做一次备份。

## 备份与恢复

数据卷的结构：

| 路径 | 内容 | 丢失后果 |
| --- | --- | --- |
| `platform.db` | 账号、会话、作品与审核记录、技能市场数据 | 所有账号和审核记录丢失 |
| `secrets/` | 中转站密钥的加密密钥、技能签名私钥 | 已绑定的中转站密钥无法解密需重新绑定；已发布技能需重新审核 |
| `workspaces/<ID>/` | 每个成员的工作区：数据库、资源文件、模型配置、助手数据 | 该成员的项目和素材丢失 |
| `community/`、`market/` | 创意工坊与技能市场的文件 | 已发布作品与技能的文件丢失 |

最简单的完整备份是停服后复制整个卷：

```bash
docker compose stop teamserver
docker run --rm -v teamserver_teamserver-data:/data -v "$PWD":/backup alpine tar czf /backup/teamserver-$(date +%F).tgz -C /data .
docker compose start teamserver
```

账号文件、密钥文件和中转站渠道定义在 `deploy/teamserver/config/`，不在数据卷里，要一起备份。卷名以 `docker volume ls` 的实际输出为准。不停服时，可以对 `platform.db` 和每个 `workspaces/*/open_ai_canvas.db` 用 `sqlite3 <db> ".backup <目标>"` 做在线备份，再增量复制其余文件。恢复时先停服，把归档解回同一个卷，再启动。

## 日常运维

| 要做的事 | 怎么做 |
| --- | --- |
| 看日志 | `docker compose logs -f teamserver` |
| 重启 | `docker compose restart teamserver`；进行中的生成任务会在重启后继续 |
| 加人、改密码、换密钥 | 改 `config/users.txt` / `config/user-keys.txt`，约 5 秒生效；改密码会让此人重新登录 |
| 停用成员 | 从账号文件删掉他那一行（网页创建的账号在“用户管理”里停用）；会话立即失效、工作区内核关闭，数据保留；记得到中转站吊销他的密钥 |
| 忘记管理员密码 | `printf '%s\n' "新密码" \| docker compose exec -T teamserver teamserver admin reset-password --username admin` |
| 审核作品与技能 | 侧栏“管理”分组里的“作品审核”“技能审核” |
| 查谁做了什么 | “用户管理 → 操作记录” |
| 确认服务器读到的配置 | 侧栏“管理 → 服务状态”，或 `docker compose logs teamserver` 开头几行 |
| 成员报生成失败 | 侧栏“管理 → 错误记录”：按成员筛选，详情里有成员看到的消息、上游任务号（可到中转站后台对照）和任务日志里的上游状态变化 |
| 换端口 | 改 `.env` 的 `TEAMSERVER_HTTP_PORT`，再 `docker compose up -d` |
| 调整空闲回收 | `.env` 设置 `TEAMSERVER_IDLE_TIMEOUT`（默认 `30m`，`off` 表示不回收）；有任务在跑或助手在回复时不会回收 |

## 不用 Docker 部署

需要在主机上准备：Go 1.25（编译）、Bun（构建前端和助手依赖）、Node 24（运行助手）、ffmpeg、zip。

```bash
# 在仓库根目录
sh plugin-packages/build-packages.sh
(cd web && bun install --frozen-lockfile && bun run build)
(cd agent-host && bun install --frozen-lockfile --production)
(cd backend && CGO_ENABLED=1 go build -o /opt/hfp/teamserver ./cmd/teamserver)
```

运行时设置：

```bash
TEAMSERVER_DATA_DIR=/var/lib/hfp \
TEAMSERVER_ADDR=127.0.0.1:8080 \
TEAMSERVER_WEB_DIR=/path/to/hfp-tv/web/dist \
TEAMSERVER_USERS_FILE=/etc/hfp/users.txt \
TEAMSERVER_USER_KEYS_FILE=/etc/hfp/user-keys.txt \
TEAMSERVER_AGENT_HOST_COMMAND=/usr/bin/node \
TEAMSERVER_AGENT_HOST_ARGS='["/path/to/hfp-tv/agent-host/server.mjs"]' \
CANVAS_OFFICIAL_PLUGIN_DIR=/path/to/hfp-tv/plugin-packages \
/opt/hfp/teamserver
```

前面放 Caddy 或 nginx 做 HTTPS 反向代理。用 nginx 时要对 `/api/` 关闭缓冲（`proxy_buffering off`）、把读超时设到 1 小时，否则任务事件流和助手对话会被截断；同时把 `TEAMSERVER_TRUSTED_PROXIES` 设为 nginx 的地址，并转发 `X-Forwarded-Proto`（登录 Cookie 据此判断是否走 HTTPS）。用 systemd 托管时，以独立的低权限用户运行，并给数据目录设置 `0750`。

## 尚未验证的部分

镜像已在内网服务器上开始试部署，但还没有压测；按人数调整的并发上限和配额还没有实现（见[方案第 9、12 节](team-server-and-workshop.md)）。首次部署建议先给少数人试用。
