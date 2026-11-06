# syntax=docker/dockerfile:1.7
# 团队服务器单镜像：Go 网关与工作区内核、Node 助手宿主、ffmpeg、官方插件与前端静态文件。
# 构建上下文是仓库根目录：docker build -f deploy/teamserver/Dockerfile .

FROM oven/bun:1.3.13 AS web-build
WORKDIR /app/web
ARG BUILD_VERSION
ENV CANVAS_BUILD_VERSION=${BUILD_VERSION}
COPY web/package.json web/bun.lock ./
COPY web/patches ./patches
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --cache-dir=/root/.bun/install/cache
COPY VERSION /app/VERSION
COPY CHANGELOG.md README.md LICENSE NOTICE THIRD_PARTY_NOTICES.md /app/
COPY licenses /app/licenses
COPY scripts/distribution-notices.mjs /app/scripts/distribution-notices.mjs
COPY assets /app/assets
COPY backend/internal/providerpreset/image_contracts.json backend/internal/providerpreset/video_contracts.json /app/backend/internal/providerpreset/
COPY web ./
# 构建上下文不含 .git，提交号由 update.sh 经 Compose 传入；排查信息里的“构建提交”来自这里。
ARG BUILD_COMMIT=unknown
ENV VITE_BUILD_COMMIT=${BUILD_COMMIT}
RUN bun --bun ./node_modules/vite/bin/vite.js build

# 助手锁文件是 Bun lockfile v2，Bun 1.3.x 无法解析，版本与本地开发要求的 1.4.4 对齐；前端锁文件仍是 v1，上面的阶段不受影响。
FROM oven/bun:1.4.4 AS agent-host-deps
WORKDIR /app/agent-host
COPY agent-host/package.json agent-host/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --production --backend copyfile --cache-dir=/root/.bun/install/cache

FROM node:24-bookworm-slim AS plugin-build
RUN apt-get update && apt-get install -y --no-install-recommends zip && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY plugin-packages ./plugin-packages
RUN ./plugin-packages/build-packages.sh

FROM golang:1.25-bookworm AS backend-build
# 先走国内镜像；它出任何错（含断流、超时）都换官方源再试一次，官方源没有的模块直接从源码仓库取。
ARG GOPROXY=https://goproxy.cn|https://proxy.golang.org,direct
ARG BUILD_VERSION
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
ENV GOPROXY=$GOPROXY
WORKDIR /src/backend
COPY VERSION /src/VERSION
COPY backend ./
# 依赖下载和编译必须挂载同一个模块缓存：以前只有下载步骤挂载，编译步骤看不到已下载的模块，每次构建都重新联网拉全部依赖。
# 缓存在失败后也保留已下完的模块，网络断续时重试和下次构建都从断点继续；模块齐全时 go mod download 不联网。
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build set -eu; \
    attempt=1; \
    until go mod download; do \
        if [ "$attempt" -ge 4 ]; then echo "Go 依赖连续 ${attempt} 次下载失败（GOPROXY=${GOPROXY}）。网络不稳时在 .env 设置 GOPROXY 后重新构建，见 .env.example。" >&2; exit 1; fi; \
        echo "Go 依赖第 ${attempt} 次下载失败，$((attempt * 15)) 秒后继续" >&2; \
        sleep $((attempt * 15)); \
        attempt=$((attempt + 1)); \
    done; \
    version="${BUILD_VERSION:-$(tr -d '\r\n' </src/VERSION)}"; \
    ldflags="-s -w -X infinite-canvas/backend/internal/buildinfo.Version=${version} -X infinite-canvas/backend/internal/buildinfo.Commit=${BUILD_COMMIT} -X infinite-canvas/backend/internal/buildinfo.BuildTime=${BUILD_TIME}"; \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="$ldflags" -o /out/teamserver ./cmd/teamserver

FROM node:24-bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata ffmpeg wget \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 app && useradd --system --uid 10001 --gid app --home-dir /data app \
    && mkdir -p /data && chown app:app /data && chmod 0750 /data
WORKDIR /app
COPY --from=backend-build /out/teamserver /usr/local/bin/teamserver
COPY --from=plugin-build /src/plugin-packages /app/plugin-packages
COPY agent-host/*.mjs agent-host/*.md agent-host/*.json /app/agent-host/
COPY --from=agent-host-deps /app/agent-host/node_modules /app/agent-host/node_modules
COPY --from=web-build /app/web/dist /app/web

ENV TEAMSERVER_DATA_DIR=/data \
    TEAMSERVER_ADDR=:8080 \
    TEAMSERVER_WEB_DIR=/app/web \
    TEAMSERVER_AGENT_HOST_COMMAND=/usr/local/bin/node \
    TEAMSERVER_AGENT_HOST_ARGS='["/app/agent-host/server.mjs"]' \
    CANVAS_OFFICIAL_PLUGIN_DIR=/app/plugin-packages \
    CANVAS_ALLOW_PRIVATE_UPSTREAMS=false

USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/api/health/live >/dev/null || exit 1
CMD ["teamserver"]
