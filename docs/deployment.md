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

1. **编辑公司中转站渠道** `config/relay-channel.json`：`baseUrl`、`apiFormat`、`models` 和 `modelProfiles` 与模型配置页里的渠道格式相同，不要写密钥。最省事的做法是先在桌面版里把中转站配好、能出图，再按那个渠道的字段抄过来。这个渠道会出现在每个成员的“模型配置”里，请求都发往这个地址，各自用自己的密钥；`baseUrl` 是内网地址（如 `http://10.10.10.45:3000/v1`）时会自动加入内网放行名单。管理员自己在“模型配置”里添加的服务只属于管理员本人的工作区，不会分发给其他成员。

   每个模型在 `modelProfiles` 里写明能力和接口协议，否则视频模型会按旧的通用视频接口发送：

   | 模型 | `capability` | `protocol` | 其他 |
   | --- | --- | --- | --- |
   | 文本 / 助手 | `text` | `chat-completion` | |
   | OpenAI 兼容生图 | `image` | `openai-image` | |
   | Seedance（经 New API 的豆包任务插件） | `video` | `volcengine-ark-video` | `baseUrl` 写中转站的豆包入口，例如 `http://10.10.10.45:3000/doubao/api/v3`；`referenceAssetService` 写公司参考图服务，例如 `http://10.10.10.45:8787/v1/reference-images` |

   模型自己的 `baseUrl` 必须与渠道 `baseUrl` 同协议、同主机和端口，否则启动时报错；成员密钥只会发往这个中转站。模型名以中转站 `/v1/models` 实际返回的为准。

   **Seedance 参考图自动登记**：Seedance 只接受已登记的德云素材编号（`asset://…`），不接受图片地址。配置了 `referenceAssetService` 后，成员用画布或素材库里的图片做参考时，团队服务器在提交视频前把图片发给公司参考图服务（旧项目的 `services/reference-upload-relay`，8787 端口）：它用成员自己的中转站密钥鉴权，把图片存进公司对象存储，登记进预设的德云素材组，等素材可用后返回素材编号；团队服务器按图片缓存这个编号，同一张图下次直接复用。参考图服务的地址会自动加入内网放行名单。服务自身的对象存储、德云密钥和素材组编号仍在它的 `.env` 里配置，见旧项目该目录的 README。纯文生视频不需要它。中转站没有取消接口，取消 Seedance 任务时会提示结果不明确。

   **参考图中转**：同一个参考图服务也提供“临时公网链接”模式。配置了 `referenceAssetService` 后，凡是只接受图片网址的模型协议（例如 NewAPI 视频），团队服务器会自动把成员的参考图经它换成临时 HTTPS 链接，不再弹出“需要在线素材链接”；成员自己添加、指向公司中转站的模型服务也一样，用的是该成员绑定的中转站密钥。成员自己添加的服务若用 `volcengine-ark-video` 协议直连中转站的豆包入口，参考图同样会先登记为德云素材。参考视频和参考音频不经这个服务；需要在线链接时仍要成员提供，或另配下面的对象存储。
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

   日志里应该看到：“公司模型服务：<地址>，N 个模型”、“允许访问的内网地址：…”、“账号文件已同步”、“内置技能市场”和 “team server listening”。看到“公司模型服务：未配置”说明 `.env` 里的 `TEAMSERVER_RELAY_CHANNEL_FILE` 没有生效。

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
cd hfp-tv && git pull
cd deploy/teamserver && docker compose up -d --build
```

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
