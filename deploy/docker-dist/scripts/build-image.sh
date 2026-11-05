#!/usr/bin/env bash
# =============================================================================
# build-image.sh —— Linux / macOS 本地构建脚本
#
# 用法：
#   ./scripts/build-image.sh                       # 默认 amd64
#   ./scripts/build-image.sh --arch arm64          # 跨架构（buildx）
#   ./scripts/build-image.sh --arch amd64 --version 1.0.0
#
# 注意：**绝不** push（红线）。push 命令只在文件末尾作为注释示例。
# =============================================================================
set -euo pipefail

ARCH="amd64"
TAG="drill-server:0.5.1"
VERSION="0.5.1"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch|-a)   ARCH="$2"; shift 2 ;;
        --tag|-t)    TAG="$2";  shift 2 ;;
        --version|-v) VERSION="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKERFILE="$REPO_ROOT/deploy/docker-dist/Dockerfile"

echo "============================================================"
echo " Drill Server Image Builder (*nix)"
echo "   Architecture : $ARCH"
echo "   Image        : $TAG"
echo "   Version      : $VERSION"
echo "   Context      : $REPO_ROOT"
echo "============================================================"

BUILD_START=$(date +%s)

export DOCKER_BUILDKIT=1

echo ""
echo "[1/3] docker buildx build --platform linux/$Arch ..."
echo ""

docker buildx build \
    --platform "linux/$ARCH" \
    -f "$DOCKERFILE" \
    -t "$TAG" \
    --build-arg "VERSION=$VERSION" \
    --build-arg "BUILD_TIME=$(date '+%Y-%m-%d %H:%M:%S')" \
    --load \
    "$REPO_ROOT"

BUILD_END=$(date +%s)
ELAPSED=$((BUILD_END - BUILD_START))
echo ""
echo "✅ 构建完成，耗时 ${ELAPSED}s"

echo ""
echo "[2/3] 镜像信息"
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}" | grep -i drill-server || true

IMAGE_ID=$(docker images -q --filter "reference=$TAG" | head -n1 || true)
if [[ -n "$IMAGE_ID" ]]; then
    echo ""
    echo "Image ID   : $IMAGE_ID"
    DIGEST=$(docker inspect --format '{{index .RepoDigests 0}}' "$IMAGE_ID" 2>/dev/null || true)
    [[ -n "$DIGEST" ]] && echo "Image Digest: $DIGEST"
fi

echo ""
echo "[3/3] 本地冒烟"
SMOKE_OUT=$(docker run --rm --entrypoint /opt/drill/bin/drill-migrate "$TAG" 2>&1 | head -n5 || true)
echo "  drill-migrate 输出（前 5 行）:"
echo "$SMOKE_OUT" | sed 's/^/    /'

echo ""
echo "============================================================"
echo "  构建完成！镜像：$TAG  架构：linux/$ARCH  耗时：${ELAPSED}s"
echo "============================================================"

# =============================================================================
# 如何 push（**需要用户自行 docker login 并确认授权后执行**）：
#
#   docker login ghcr.io
#   docker tag $TAG ghcr.io/your-org/drill-server:$VERSION
#   docker push ghcr.io/your-org/drill-server:$VERSION
#
#   # 多架构 manifest（buildx 推荐）
#   docker buildx build --platform linux/amd64,linux/arm64 \
#       -t ghcr.io/your-org/drill-server:$VERSION \
#       -f deploy/docker-dist/Dockerfile --push .
# =============================================================================
