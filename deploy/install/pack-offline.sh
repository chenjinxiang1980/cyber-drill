#!/usr/bin/env bash
# =============================================================================
# 离线资源打包（在“有网 + 有 docker + dnf 系”的 Linux 上执行一次；优先 openEuler 24.03 LTS-SP1）
#
# 产出：
#   <out>/rpms/*.rpm                        Docker 引擎与全部依赖
#   <out>/bin/docker-compose                compose v2 独立二进制
#   <out>/images/cyber-drill-<版本>.tar.gz  应用镜像
#   <out>/images/mysql-8.0.tar.gz           MySQL 镜像
#   <out>/baseline/*.sql.gz + SHA256SUMS + manifest.env
#                                           基线快照（结构 + 参照数据，不含账号与运营数据）
#   <out>/SHA256SUMS                        校验清单
#
# 用法：
#   sudo bash pack-offline.sh --out /opt/cyber-drill-offline --version 1.0.0
#   sudo bash pack-offline.sh --out ./offline --baseline-dir ./baseline
#   sudo bash pack-offline.sh --out ./offline --compose-bin /path/docker-compose
#
# 说明：
#   - Docker Hub 在部分内网不可达，默认经 daocloud 镜像拉取 mysql:8.0 后重命名为 mysql:8.0
#   - 基线快照由 build-baseline.sh 生成（导出→脱敏→校验），本脚本只负责打包与校验哈希
#   - 打包完成后可整体拷贝到离线机器，用 install.sh --offline-dir <目录> 安装
# =============================================================================
set -Eeuo pipefail

OUT="./offline"
VERSION="1.0.2"
COMPOSE_BIN=""
BASELINE_DIR=""
MYSQL_SOURCE="docker.m.daocloud.io/library/mysql:8.0"

COMPOSE_URLS=(
    "https://ghproxy.net/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://ghfast.top/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
)

info() { printf '[信息] %s\n' "$*"; }
die()  { printf '[错误] %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --out)          OUT="${2:-}"; shift ;;
        --version)      VERSION="${2:-}"; shift ;;
        --compose-bin)  COMPOSE_BIN="${2:-}"; shift ;;
        --baseline-dir) BASELINE_DIR="${2:-}"; shift ;;
        --mysql-source) MYSQL_SOURCE="${2:-}"; shift ;;
        -h|--help)      sed -n '2,25p' "$0"; exit 0 ;;
        *) die "未知参数：$1" ;;
    esac
    shift
done

[ "$(id -u)" -eq 0 ] || die "请用 root 执行（需要 dnf 下载 RPM）"
command -v dnf >/dev/null 2>&1 || die "仅支持 dnf 系发行版（openEuler / 统信 UOS Server 20 / RHEL / CentOS）"
command -v docker >/dev/null 2>&1 || die "需要本机 docker 用于拉取/导出镜像"

mkdir -p "${OUT}"/{rpms,images,bin,baseline}

info "0/5 敏感内容硬闸"
stray_sql="$(find "${OUT}" -type f -name '*.sql' -print -quit)"
[ -z "${stray_sql}" ] || die "打包目录存在 .sql 文件（疑似生产库 dump），拒绝打包：${stray_sql}"
leaked_ip="$(grep -RIlE '192\.168\.[0-9]{1,3}\.[0-9]{1,3}' "${OUT}" \
    --include='*.md' --include='*.txt' --include='*.env' --include='*.yml' \
    --include='*.yaml' --include='*.conf' --include='*.service' 2>/dev/null | head -n1 || true)"
[ -z "${leaked_ip}" ] || die "打包目录存在真实内网地址，拒绝打包：${leaked_ip}"
info "敏感内容检查通过（无 stray .sql、无 192.168.x.x）"

info "1/5 下载 Docker 引擎 RPM（含依赖）"
rm -f "${OUT}"/rpms/*.rpm
dnf download --resolve --alldeps --destdir "${OUT}/rpms" moby-engine moby-client \
    >/tmp/pack-offline-dnf.log 2>&1 || die "RPM 下载失败，见 /tmp/pack-offline-dnf.log"
info "已下载 $(ls -1 "${OUT}"/rpms/*.rpm | wc -l) 个 RPM"

info "2/5 准备 compose v2 二进制"
if [ -n "${COMPOSE_BIN}" ]; then
    install -m 0755 "${COMPOSE_BIN}" "${OUT}/bin/docker-compose"
else
    # 优先复用本机已装的 compose 二进制：ghproxy 等镜像可能只有几十 KB/s，
    # 下载 60MB 会拖到数小时；本机版本与目标机无强绑定，直接打包即可。
    local_compose=""
    for p in /usr/local/lib/docker/cli-plugins/docker-compose \
             /usr/libexec/docker/cli-plugins/docker-compose \
             /usr/lib/docker/cli-plugins/docker-compose; do
        if [ -x "${p}" ]; then local_compose="${p}"; break; fi
    done
    if [ -n "${local_compose}" ]; then
        install -m 0755 "${local_compose}" "${OUT}/bin/docker-compose"
        info "复用本机 compose：${local_compose}（$("${OUT}/bin/docker-compose" version 2>/dev/null | head -n1)）"
    else
        tmp="$(mktemp)"
        ok=0
        for url in "${COMPOSE_URLS[@]}"; do
            if curl -fL --connect-timeout 10 --speed-limit 102400 --speed-time 60 -m 600 -o "${tmp}" "${url}"; then
                install -m 0755 "${tmp}" "${OUT}/bin/docker-compose"; ok=1; break
            fi
        done
        rm -f "${tmp}"
        [ "${ok}" -eq 1 ] || die "compose 下载失败，请用 --compose-bin 指定本地二进制"
    fi
fi

info "3/5 导出应用镜像 ${VERSION}"
if docker image inspect "cyber-drill:${VERSION}" >/dev/null 2>&1; then
    info "本地已有 cyber-drill:${VERSION}，跳过拉取"
else
    docker pull "ghcr.io/chenjinxiang1980/cyber-drill:${VERSION}" || die "应用镜像拉取失败（本机已有该镜像时可直接复用）"
    docker tag "ghcr.io/chenjinxiang1980/cyber-drill:${VERSION}" "cyber-drill:${VERSION}"
fi
docker save "cyber-drill:${VERSION}" | gzip -1 > "${OUT}/images/cyber-drill-${VERSION}.tar.gz"

info "4/5 导出 MySQL 镜像"
if docker image inspect mysql:8.0 >/dev/null 2>&1; then
    info "本地已有 mysql:8.0，跳过拉取"
else
    docker pull "${MYSQL_SOURCE}" || die "MySQL 镜像拉取失败（本机已有 mysql:8.0 时可直接复用）"
    docker tag "${MYSQL_SOURCE}" mysql:8.0
fi
docker save mysql:8.0 | gzip -1 > "${OUT}/images/mysql-8.0.tar.gz"

info "5/5 打包基线快照"
[ -n "${BASELINE_DIR}" ] || BASELINE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/baseline"
[ -d "${BASELINE_DIR}" ] || die "基线快照目录不存在：${BASELINE_DIR}（先运行 build-baseline.sh 生成）"
SNAP_FILE="$(ls "${BASELINE_DIR}"/drill-baseline-*.sql.gz 2>/dev/null | head -n1)"
[ -n "${SNAP_FILE}" ] || die "基线目录中没有 drill-baseline-*.sql.gz：${BASELINE_DIR}"
[ -f "${BASELINE_DIR}/manifest.env" ] || die "基线目录缺少 manifest.env：${BASELINE_DIR}"
[ "$(basename "${SNAP_FILE}")" = "drill-baseline-${VERSION}.sql.gz" ] \
    || die "基线快照版本与 --version ${VERSION} 不一致：$(basename "${SNAP_FILE}")"
grep -q "^BASELINE_VERSION=${VERSION}$" "${BASELINE_DIR}/manifest.env" \
    || die "manifest.env 的 BASELINE_VERSION 与 --version ${VERSION} 不一致"
grep -q '^BASELINE_MIGRATIONS=[0-9][0-9]*$' "${BASELINE_DIR}/manifest.env" \
    || die "manifest.env 缺少 BASELINE_MIGRATIONS（必须绑定该版本的迁移条数）"
if [ -f "${BASELINE_DIR}/SHA256SUMS" ]; then
    # 容忍 Windows 侧产物携带 BOM/CRLF，否则 sha256sum -c 会解析失败
    sums_clean="$(mktemp)"
    sed '1s/^\xEF\xBB\xBF//' "${BASELINE_DIR}/SHA256SUMS" | tr -d '\r' > "${sums_clean}"
    (cd "${BASELINE_DIR}" && sha256sum -c "${sums_clean}") || { rm -f "${sums_clean}"; die "基线快照校验失败（SHA256SUMS 不匹配）"; }
    rm -f "${sums_clean}"
    info "基线校验通过：$(basename "${SNAP_FILE}")"
else
    die "基线目录缺少 SHA256SUMS（发布产物必须固定哈希）：${BASELINE_DIR}"
fi
cp -f "${SNAP_FILE}" "${BASELINE_DIR}/SHA256SUMS" "${BASELINE_DIR}/manifest.env" "${OUT}/baseline/"
chmod 0644 "${OUT}/baseline/"*

(cd "${OUT}" && sha256sum images/*.tar.gz bin/docker-compose baseline/*.sql.gz baseline/SHA256SUMS baseline/manifest.env > SHA256SUMS)
echo "${VERSION}" > "${OUT}/VERSION"

info "打包完成：${OUT}"
du -sh "${OUT}"
ls -lh "${OUT}/images" "${OUT}/bin" "${OUT}/baseline"
