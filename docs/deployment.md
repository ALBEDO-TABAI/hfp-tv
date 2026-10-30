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

- 一台 Linux 服务器，Docker 24 以上（含 Compose 插件），建议 4 核 8 GB 内存起步；每个在线成员的创作助手会占用一个 Node 进程。
- 一个解析到这台服务器的域名，开放 80 和 443 端口。Caddy 会自动申请 HTTPS 证书；只在内网时，可以改用你们自己的证书或内网 CA。
- 公司中转站的地址和可用模型清单，以及给每个成员发的中转站密钥。

### 步骤

```bash
git clone https://github.com/ALBEDO-TABAI/hfp-tv.git
cd hfp-tv/deploy/teamserver
cp config/relay-channel.example.json config/relay-channel.json   # 按中转站实际情况修改
```

1. **编辑公司中转站渠道** `config/relay-channel.json`：`baseUrl`、`apiFormat`、`models` 和 `modelProfiles` 与模型配置页里的渠道格式相同，不要写密钥。最省事的做法是先在桌面版里把中转站配好、能出图，再按那个渠道的字段抄过来。
2. **写部署变量**：在 `deploy/teamserver/.env`（不要提交）里写：

   ```bash
   TEAMSERVER_DOMAIN=studio.example.com
   TEAMSERVER_RELAY_CHANNEL_FILE=/config/relay-channel.json
   # 中转站在内网时，只放行它这一个主机名，不要打开整个私网出站
   CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=relay.internal.example.com
   # 需要公网素材地址的模型（部分视频模型）要配置 S3 兼容存储，可选
   # TEAMSERVER_REFERENCE_STORAGE_FILE=/config/reference-storage.json
   ```

3. **构建并启动**：

   ```bash
   docker compose up -d --build
   docker compose logs -f teamserver   # 看到 “team server listening” 和 “内置技能市场” 两行即可
   ```

4. **创建首个管理员**（密码从标准输入读，不进 shell 历史）：

   ```bash
   read -rs PW; printf '%s\n' "$PW" | docker compose exec -T teamserver teamserver admin create --username admin --display-name 管理员
   ```

5. 用浏览器打开 `https://<域名>`，以管理员登录，在账号菜单的“用户管理”里创建成员或生成邀请链接，并为每个成员绑定中转站密钥。

### 上线前验收

健康检查只说明进程活着。请至少用两个成员账号实际走一遍：

- [ ] 登录、退出；A 退出后在同一浏览器登录 B，看不到 A 的项目。
- [ ] 新建画布、上传图片、刷新后内容还在。
- [ ] 用“公司中转站”渠道出一张图、一段视频，中转站后台能看到用量记在本人的密钥下。
- [ ] 创作助手能回复，并能在画布上新建节点。
- [ ] 创意工坊：A 投稿 → 管理员审核通过 → B 浏览并复制到自己的项目 → B 能继续编辑和生成。
- [ ] 技能市场：A 连接投稿账号、投稿 → 管理员在“技能审核”通过 → B 能安装。
- [ ] 停用一个账号后，他的浏览器立即回到登录页。

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

卷名以 `docker volume ls` 的实际输出为准。不停服时，可以对 `platform.db` 和每个 `workspaces/*/open_ai_canvas.db` 用 `sqlite3 <db> ".backup <目标>"` 做在线备份，再增量复制其余文件。恢复时先停服，把归档解回同一个卷，再启动。

## 日常运维

| 要做的事 | 怎么做 |
| --- | --- |
| 看日志 | `docker compose logs -f teamserver` |
| 重启 | `docker compose restart teamserver`；进行中的生成任务会在重启后继续 |
| 停用成员 | 管理员在“用户管理”里停用；会话立即失效、他的工作区内核关闭，数据保留；记得到中转站吊销他的密钥 |
| 忘记管理员密码 | `printf '%s\n' "新密码" \| docker compose exec -T teamserver teamserver admin reset-password --username admin` |
| 审核作品与技能 | 管理员账号菜单里的“作品审核”“技能审核” |
| 查谁做了什么 | “用户管理 → 操作记录” |

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
TEAMSERVER_AGENT_HOST_COMMAND=/usr/bin/node \
TEAMSERVER_AGENT_HOST_ARGS='["/path/to/hfp-tv/agent-host/server.mjs"]' \
CANVAS_OFFICIAL_PLUGIN_DIR=/path/to/hfp-tv/plugin-packages \
/opt/hfp/teamserver
```

前面放 Caddy 或 nginx 做 HTTPS 反向代理。用 nginx 时要对 `/api/` 关闭缓冲（`proxy_buffering off`）、把读超时设到 1 小时，否则任务事件流和助手对话会被截断；同时把 `TEAMSERVER_TRUSTED_PROXIES` 设为 nginx 的地址。用 systemd 托管时，以独立的低权限用户运行，并给数据目录设置 `0750`。

## 尚未验证的部分

镜像和 Compose 还没有在真实服务器上构建和压测；按人数调整的内存和并发上限、配额也还没有实现（见[方案第 9、12 节](team-server-and-workshop.md)）。首次部署建议先给少数人试用。
