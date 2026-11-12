#!/usr/bin/env bash
# =============================================================================
# 离线交付打包（在“有网 + 有 docker”的 Linux 上执行一次；优先 openEuler 24.03 LTS-SP1）
#
# 默认产出（三件套放同一目录即可离线安装）：
#   <out>/install.sh                   已内嵌基线快照的安装脚本
#   <out>/cyber-drill-<版本>.tar.gz     应用镜像
#   <out>/mysql-8.0.tar.gz              MySQL 镜像
#   <out>/SHA256SUMS                    校验清单
#
# 可选：
#   --embed-installer   把 install.sh 的基线段重写为内嵌快照（先剥旧段再写，可重复执行）
#   --with-rpms         追加 rpms/（Docker 引擎及依赖）与 bin/docker-compose
#
# 用法：
#   sudo bash pack-offline.sh --out ./offline --version 1.0.6 --embed-installer
#   sudo bash pack-offline.sh --out ./offline --version 1.0.6 --embed-installer --with-rpms
#   sudo bash pack-offline.sh --out ./offline --version 1.0.6
#       # 不带 --embed-installer 时输出 install.sh + baseline/ 目录（兼容旧 --offline-dir 用法）
#
# 说明：
#   - 目标机只需 Docker；未装 Docker 时用 --with-rpms 一并打包引擎与 compose v2
#   - 基线快照来自 deploy/install/baseline/（build-baseline.sh 生成），打包前强制校验 SHA256
#   - 交付物就是 install.sh + 两个 tar（+ SHA256SUMS），用户无需解包目录结构
# =============================================================================
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

OUT="./offline"
VERSION=""
COMPOSE_BIN=""
BASELINE_DIR=""
INSTALLER="${SCRIPT_DIR}/install.sh"
MYSQL_SOURCE="docker.m.daocloud.io/library/mysql:8.0"
APP_SOURCE=""
EMBED_INSTALLER=0
WITH_RPMS=0
TOTAL=5
STEP=0

COMPOSE_URLS=(
    "https://ghproxy.net/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://ghfast.top/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
)

info() { printf '[信息] %s\n' "$*"; }
ok()   { printf '[完成] %s\n' "$*"; }
die()  { printf '[错误] %s\n' "$*" >&2; exit 1; }
say_step() { STEP=$((STEP + 1)); info "${STEP}/${TOTAL} $*"; }

usage() { sed -n '2,25p' "$0"; }

while [ $# -gt 0 ]; do
    case "$1" in
        --out)              OUT="${2:-}"; shift ;;
        --version)          VERSION="${2:-}"; shift ;;
        --installer)        INSTALLER="${2:-}"; shift ;;
        --compose-bin)      COMPOSE_BIN="${2:-}"; shift ;;
        --baseline-dir)     BASELINE_DIR="${2:-}"; shift ;;
        --mysql-source)     MYSQL_SOURCE="${2:-}"; shift ;;
        --app-source)       APP_SOURCE="${2:-}"; shift ;;
        --embed-installer)  EMBED_INSTALLER=1 ;;
        --with-rpms)        WITH_RPMS=1 ;;
        -h|--help)          usage; exit 0 ;;
        *) die "未知参数：$1（--help 查看用法）" ;;
    esac
    shift
done

[ "$(id -u)" -eq 0 ] || die "请用 root 执行（--with-rpms 需要 dnf 下载 RPM）"
command -v docker >/dev/null 2>&1 || die "需要本机 docker 用于拉取/导出镜像"
command -v base64 >/dev/null 2>&1 || die "需要 base64（coreutils）用于内嵌基线"

if [ -z "${VERSION}" ] && [ -f "${REPO_ROOT}/VERSION" ]; then
    VERSION="$(tr -d ' \t\r\n\357\273\277' < "${REPO_ROOT}/VERSION")"
fi
if [ -z "${VERSION}" ]; then
    VERSION="$(sed -n 's/^BASELINE_VERSION=//p' "${SCRIPT_DIR}/baseline/manifest.env" 2>/dev/null | head -n1 | tr -d ' \t\r\n"')"
fi
[ -n "${VERSION}" ] || die "无法确定版本号：请用 --version 指定"
case "${VERSION}" in
    *[!0-9A-Za-z._-]*) die "版本号含非法字符：${VERSION}" ;;
esac

[ "${EMBED_INSTALLER}" -eq 1 ] && TOTAL=$((TOTAL + 1))
[ "${WITH_RPMS}" -eq 1 ] && TOTAL=$((TOTAL + 1))

mkdir -p "${OUT}"

# ---------------------------------------------------------------------------
say_step "敏感内容硬闸"
stray_sql="$(find "${OUT}" -type f -name '*.sql' -print -quit)"
[ -z "${stray_sql}" ] || die "打包目录存在 .sql 文件（疑似生产库 dump），拒绝打包：${stray_sql}"
leaked_ip="$(grep -RIlE '192\.168\.[0-9]{1,3}\.[0-9]{1,3}' "${OUT}" \
    --include='*.md' --include='*.txt' --include='*.env' --include='*.yml' \
    --include='*.yaml' --include='*.conf' --include='*.service' 2>/dev/null | head -n1 || true)"
[ -z "${leaked_ip}" ] || die "打包目录存在真实内网地址，拒绝打包：${leaked_ip}"
ok "敏感内容检查通过（无 stray .sql、无 192.168.x.x）"

# ---------------------------------------------------------------------------
say_step "校验基线快照"
[ -n "${BASELINE_DIR}" ] || BASELINE_DIR="${SCRIPT_DIR}/baseline"
[ -d "${BASELINE_DIR}" ] || die "基线快照目录不存在：${BASELINE_DIR}（先运行 build-baseline.sh 生成）"
SNAP_FILE="${BASELINE_DIR}/drill-baseline-${VERSION}.sql.gz"
[ -f "${SNAP_FILE}" ] || die "基线目录没有 drill-baseline-${VERSION}.sql.gz：${BASELINE_DIR}"
[ -f "${BASELINE_DIR}/manifest.env" ] || die "基线目录缺少 manifest.env：${BASELINE_DIR}"
[ -f "${BASELINE_DIR}/SHA256SUMS" ] || die "基线目录缺少 SHA256SUMS：${BASELINE_DIR}"
grep -q "^BASELINE_VERSION=${VERSION}$" "${BASELINE_DIR}/manifest.env" \
    || die "manifest.env 的 BASELINE_VERSION 与 --version ${VERSION} 不一致"
grep -q '^BASELINE_MIGRATIONS=[0-9][0-9]*$' "${BASELINE_DIR}/manifest.env" \
    || die "manifest.env 缺少 BASELINE_MIGRATIONS（必须绑定该版本的迁移条数）"

# 容忍 Windows 侧产物携带 BOM/CRLF，否则 sha256sum -c 会解析失败
SUMS_CLEAN="$(mktemp)"
MANIFEST_CLEAN="$(mktemp)"
trap 'rm -f "${SUMS_CLEAN}" "${MANIFEST_CLEAN}"' EXIT
sed '1s/^\xEF\xBB\xBF//' "${BASELINE_DIR}/SHA256SUMS" | tr -d '\r' > "${SUMS_CLEAN}"
sed '1s/^\xEF\xBB\xBF//' "${BASELINE_DIR}/manifest.env" | tr -d '\r' > "${MANIFEST_CLEAN}"
(cd "${BASELINE_DIR}" && sha256sum -c "${SUMS_CLEAN}" --ignore-missing) || die "基线快照校验失败（SHA256SUMS 不匹配）"

SNAP_SHA="$(sha256sum "${SNAP_FILE}" | awk '{print $1}')"
SNAP_SIZE="$(wc -c < "${SNAP_FILE}" | tr -d ' \t\r\n')"
ok "基线校验通过：$(basename "${SNAP_FILE}")（SHA256 ${SNAP_SHA}，${SNAP_SIZE} 字节）"

# ---------------------------------------------------------------------------
say_step "导出应用镜像 ${VERSION}"
if docker image inspect "cyber-drill:${VERSION}" >/dev/null 2>&1; then
    info "本地已有 cyber-drill:${VERSION}，跳过拉取"
else
    [ -n "${APP_SOURCE}" ] || APP_SOURCE="ghcr.io/chenjinxiang1980/cyber-drill:${VERSION}"
    info "拉取应用镜像：${APP_SOURCE}"
    docker pull "${APP_SOURCE}" || die "应用镜像拉取失败（本机已有该镜像时可直接复用，或用 --app-source 指定）"
    docker tag "${APP_SOURCE}" "cyber-drill:${VERSION}"
fi
docker save "cyber-drill:${VERSION}" | gzip -1 > "${OUT}/cyber-drill-${VERSION}.tar.gz"
ok "应用镜像：${OUT}/cyber-drill-${VERSION}.tar.gz"

# ---------------------------------------------------------------------------
say_step "导出 MySQL 镜像"
if docker image inspect mysql:8.0 >/dev/null 2>&1; then
    info "本地已有 mysql:8.0，跳过拉取"
else
    docker pull "${MYSQL_SOURCE}" || die "MySQL 镜像拉取失败（本机已有 mysql:8.0 时可直接复用）"
    docker tag "${MYSQL_SOURCE}" mysql:8.0
fi
docker save mysql:8.0 | gzip -1 > "${OUT}/mysql-8.0.tar.gz"
ok "MySQL 镜像：${OUT}/mysql-8.0.tar.gz"

# ---------------------------------------------------------------------------
if [ "${WITH_RPMS}" -eq 1 ]; then
    say_step "下载 Docker 引擎与 compose（--with-rpms）"
    command -v dnf >/dev/null 2>&1 || die "仅支持 dnf 系发行版（openEuler / 统信 UOS Server 20 / RHEL / CentOS）"
    mkdir -p "${OUT}"/{rpms,bin}
    rm -f "${OUT}"/rpms/*.rpm
    dnf download --resolve --alldeps --destdir "${OUT}/rpms" moby-engine moby-client \
        >/tmp/pack-offline-dnf.log 2>&1 || die "RPM 下载失败，见 /tmp/pack-offline-dnf.log"
    info "已下载 $(ls -1 "${OUT}"/rpms/*.rpm | wc -l) 个 RPM"

    if [ -n "${COMPOSE_BIN}" ]; then
        install -m 0755 "${COMPOSE_BIN}" "${OUT}/bin/docker-compose"
    else
        # 优先复用本机已装的 compose 二进制：镜像站可能只有几十 KB/s，
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
fi

# ---------------------------------------------------------------------------
if [ "${EMBED_INSTALLER}" -eq 1 ]; then
    say_step "内嵌基线到 install.sh"
    [ -f "${INSTALLER}" ] || die "install.sh 不存在：${INSTALLER}（用 --installer 指定）"

    BEGIN_MARK='# >>> BEGIN EMBEDDED BASELINE >>>'
    END_MARK='# <<< END EMBEDDED BASELINE <<<'
    n_begin="$(grep -cF "${BEGIN_MARK}" "${INSTALLER}" || true)"
    n_end="$(grep -cF "${END_MARK}" "${INSTALLER}" || true)"
    [ "${n_begin}" = "1" ] && [ "${n_end}" = "1" ] \
        || die "install.sh 内嵌标记异常（BEGIN=${n_begin}，END=${n_end}）：${INSTALLER}"

    PAYLOAD="$(mktemp)"
    {
        printf '# 本段由 pack-offline.sh --embed-installer 生成；重复打包会先剥离旧段再整体重写。\n'
        printf 'EMBEDDED_BASELINE_VERSION="%s"\n' "${VERSION}"
        printf 'EMBEDDED_BASELINE_SHA256="%s"\n' "${SNAP_SHA}"
        printf 'EMBEDDED_BASELINE_SIZE="%s"\n' "${SNAP_SIZE}"
        printf "EMBEDDED_BASELINE_B64='\n"
        base64 -w 76 "${SNAP_FILE}"
        printf "'\n"
        printf "EMBEDDED_BASELINE_MANIFEST_B64='\n"
        base64 -w 76 "${MANIFEST_CLEAN}"
        printf "'\n"
        printf "EMBEDDED_BASELINE_SUMS_B64='\n"
        base64 -w 76 "${SUMS_CLEAN}"
        printf "'\n"
    } > "${PAYLOAD}"

    # 先剥离旧段（BEGIN 与 END 之间的所有行），再在 BEGIN 后写入新载荷
    awk -v pf="${PAYLOAD}" -v b="${BEGIN_MARK}" -v e="${END_MARK}" '
        $0 == b { print; while ((getline line < pf) > 0) print line; close(pf); skip = 1; next }
        $0 == e { skip = 0 }
        skip != 1 { print }
    ' "${INSTALLER}" > "${OUT}/install.sh"
    rm -f "${PAYLOAD}"
    chmod 0755 "${OUT}/install.sh"
    bash -n "${OUT}/install.sh" || die "内嵌后的 install.sh 语法检查失败：${OUT}/install.sh"

    installer_lines="$(grep -c '' "${OUT}/install.sh" || true)"
    ok "已内嵌基线：${OUT}/install.sh（${installer_lines} 行，$(du -h "${OUT}/install.sh" | awk '{print $1}')）"
else
    cp -f "${INSTALLER}" "${OUT}/install.sh"
    chmod 0755 "${OUT}/install.sh"
    mkdir -p "${OUT}/baseline"
    cp -f "${SNAP_FILE}" "${OUT}/baseline/"
    cp -f "${SUMS_CLEAN}" "${OUT}/baseline/SHA256SUMS"
    cp -f "${MANIFEST_CLEAN}" "${OUT}/baseline/manifest.env"
    chmod 0644 "${OUT}/baseline/"*
    info "未指定 --embed-installer：已输出 install.sh + baseline/（兼容旧 --offline-dir 用法）"
fi
echo "${VERSION}" > "${OUT}/VERSION"

# ---------------------------------------------------------------------------
say_step "生成校验清单"
(
    cd "${OUT}"
    if [ "${WITH_RPMS}" -eq 1 ]; then
        sha256sum install.sh cyber-drill-"${VERSION}".tar.gz mysql-8.0.tar.gz \
            bin/docker-compose rpms/*.rpm VERSION > SHA256SUMS
    elif [ -d "${OUT}/baseline" ] && [ "${EMBED_INSTALLER}" -eq 0 ]; then
        sha256sum install.sh cyber-drill-"${VERSION}".tar.gz mysql-8.0.tar.gz \
            baseline/*.sql.gz baseline/SHA256SUMS baseline/manifest.env VERSION > SHA256SUMS
    else
        sha256sum install.sh cyber-drill-"${VERSION}".tar.gz mysql-8.0.tar.gz VERSION > SHA256SUMS
    fi
)

info "打包完成：${OUT}"
du -sh "${OUT}"
ls -lh "${OUT}" | sed 's/^/  /'
