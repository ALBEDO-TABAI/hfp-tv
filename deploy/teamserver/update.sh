#!/bin/sh
# 升级团队服务器：拉取代码、带上提交号重建镜像，等新容器通过健康检查后再提示可以测试。
# 构建期间旧容器继续服务，这段时间提交的任务仍按旧版本执行，所以要等本脚本结束再测试。
set -eu
cd "$(dirname "$0")"

git pull --ff-only
BUILD_COMMIT="$(git rev-parse --short HEAD)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
export BUILD_COMMIT BUILD_TIME

echo "开始构建 ${BUILD_COMMIT}（构建期间旧版本继续服务）"
docker compose up -d --build

container="$(docker compose ps -q teamserver)"
waited=0
while :; do
  health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || echo unknown)"
  case "$health" in
    healthy|running) break ;;
    unhealthy|exited|dead) echo "新容器状态为 ${health}，请查看 docker compose logs teamserver" >&2; exit 1 ;;
  esac
  if [ "$waited" -ge 180 ]; then
    echo "新容器 3 分钟内没有通过健康检查（当前 ${health}），请查看 docker compose logs teamserver" >&2
    exit 1
  fi
  sleep 3
  waited=$((waited + 3))
done

echo "团队服务器已运行 ${BUILD_COMMIT}（构建于 ${BUILD_TIME}），现在可以测试。"
echo "之后的“复制排查信息”里“构建提交”应为 ${BUILD_COMMIT}，“运行版本”带 -${BUILD_COMMIT}。"
