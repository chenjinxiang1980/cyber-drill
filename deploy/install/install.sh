#!/usr/bin/env bash
# =============================================================================
# Cyber-Drill（应急演练系统）一键安装脚本 —— Linux / Docker
#
# 全新安装 = 恢复基线快照（结构 + 参照数据 + 快照内的 schema_migrations），
#            不在空库逐条重放迁移链；迁移链只服务 upgrade.sh 的升级路径。
#
# 口径（定稿）：不得为通过历史迁移守卫在空库伪造数据；新增迁移在基线快照
#            同步前不会进入全新安装（需重跑 build-baseline.sh 并更新
#            manifest.env / SHA256SUMS / EXPECT_* 后才会生效）。
#
# 一条命令完成：
#   Docker 引擎与 compose → 导入镜像 → 建目录 → 生成随机强口令 .env(600)
#   → 启动 MySQL → 恢复基线快照 → 校验口径 → 启动应用 → 生成管理员
#   → 健康检查 → 打印访问地址与一次性初始口令
#
# 已安装检测：${INSTALL_DIR} 已存在且非空时直接停止，给出两条明确路径：
#   A. --reinstall 删除原目录/容器/数据卷后重装（必须输入确认词 DELETE）
#   B. 保留数据，改用 deploy/upgrade/upgrade.sh 升级
#
# ⚠ 破坏性操作纪律（禁止绕过）：
#   1. 任何删除目录/容器/数据卷的动作，只允许作用于
#      com.docker.compose.project.working_dir == 当前 INSTALL_DIR 的容器，
#      以及 com.docker.compose.project == 当前 PROJECT 且卷名为本安装器声明的数据卷；
#      禁止按固定容器名（drill-app / drill-mysql）或固定卷名（drill-deploy_*）删除。
#   2. 测试安装器必须使用独立安装目录 + 独立 compose 项目名（--install-dir/--project），
#      不得指向现网目录或现网项目，避免误删同机其他实例。
#   3. 删除前先打印将删除的对象清单与数量，便于核对是否命中错误目标。
#
# 用法：
#   sudo bash deploy/install/install.sh                       # 在线安装（自动选镜像源）
#   sudo bash deploy/install/install.sh --image-source swr    # 指定国内华为云 SWR
#   sudo bash deploy/install/install.sh --offline-dir /opt/cyber-drill-offline
#   sudo bash deploy/install/install.sh --reinstall           # 已装过时删除重装
#   sudo bash deploy/install/install.sh --help
#
# 退出码：
#   10 前置检查失败   20 Docker 安装失败   21 镜像导入/拉取失败
#   23 MySQL 未就绪   25 应用健康检查失败
#   26 检测到已有库且非本安装器创建（拒绝覆盖）
#   27 基线快照缺失/恢复/校验失败
#   28 检测到已有安装目录（需选择删除重装或升级）
# =============================================================================
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

DRILL_VERSION=""            # 由 resolve_version 解析，禁止在别处写死版本号
VERSION_OPT=""
APP_IMAGE=""
MYSQL_IMAGE="mysql:8.0"
INSTALL_DIR="/opt/cyber-drill"
PROJECT="drill"
APP_PORT="8090"
MYSQL_HOST_PORT="3306"
OFFLINE_DIR=""
SNAPSHOT=""
MODE="auto"
SHOW_SECRETS=0
SKIP_FIREWALL=0
FORCE_ENV=0
REINSTALL=0
CONFIRM_WORD=""
IMAGE_SOURCE="auto"       # auto|ghcr|swr
TLS_CERT=""
TLS_KEY=""
TLS_APP_PORT="8443"

MYSQL_CONTAINER="drill-mysql"
APP_CONTAINER="drill-app"
COMPOSE_FILE=""
ENV_FILE=""
RESTORE_LOG=""
CURRENT_STEP="初始化"

# 包管理器：openEuler / UOS Server 20 用 dnf，老版本 RHEL 系可能只有 yum
PKG_MGR=""
if command -v dnf >/dev/null 2>&1; then
    PKG_MGR="dnf"
elif command -v yum >/dev/null 2>&1; then
    PKG_MGR="yum"
fi

GHCR_HOST="ghcr.io"
GHCR_REPO="chenjinxiang1980/cyber-drill"
SWR_HOST="swr.cn-east-3.myhuaweicloud.com"
SWR_REPO="chenjinxiang/cyber-drill"
GHCR_REGISTRY="${GHCR_HOST}/${GHCR_REPO}"
SWR_REGISTRY="${SWR_HOST}/${SWR_REPO}"
GHCR_IMAGE=""
SWR_IMAGE=""
CHOSEN_REGISTRY=""
CHOSEN_IMAGE=""
FALLBACK_IMAGE=""
MYSQL_MIRRORS=(
    "docker.m.daocloud.io/library/mysql:8.0"
    "docker.1ms.run/library/mysql:8.0"
)
COMPOSE_URLS=(
    "https://ghproxy.net/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://ghfast.top/https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
    "https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64"
)

# ---------- 输出 ----------
if [ -t 1 ]; then
    C_INFO=$'\033[1;34m'; C_OK=$'\033[1;32m'; C_WARN=$'\033[1;33m'; C_ERR=$'\033[1;31m'; C_END=$'\033[0m'
else
    C_INFO=""; C_OK=""; C_WARN=""; C_ERR=""; C_END=""
fi
info()  { printf '%s[信息]%s %s\n' "$C_INFO" "$C_END" "$*"; }
ok()    { printf '%s[完成]%s %s\n' "$C_OK" "$C_END" "$*"; }
warn()  { printf '%s[提示]%s %s\n' "$C_WARN" "$C_END" "$*" >&2; }
err()   { printf '%s[错误]%s %s\n' "$C_ERR" "$C_END" "$*" >&2; }
step()  { CURRENT_STEP="$*"; printf '\n%s==> %s%s\n' "$C_INFO" "$*" "$C_END"; }

die() {
    local code="$1"; shift
    err "$*"
    err "失败步骤：${CURRENT_STEP}"
    exit "$code"
}

on_err() {
    local rc=$?
    trap - ERR
    set +e
    err "命令执行失败（exit=${rc}），失败步骤：${CURRENT_STEP}"
    if [ -n "${RESTORE_LOG}" ] && [ -f "${RESTORE_LOG}" ]; then
        err "快照恢复日志：${RESTORE_LOG}（末尾 20 行）"
        tail -n 20 "${RESTORE_LOG}" >&2 || true
    fi
    exit "$rc"
}
trap on_err ERR

usage() {
    cat <<'EOF'
Cyber-Drill 一键安装（Docker / Linux，基线快照模式）

用法：
  sudo bash install.sh [参数]

参数：
  --offline-dir <目录>   离线资源目录（rpms/、images/、baseline/、bin/docker-compose）
  --snapshot <文件>      基线快照 .sql.gz（默认在离线目录 baseline/ 或脚本同级 baseline/ 查找）
  --mode auto|offline|online
                         安装来源，默认 auto（有离线资源时优先离线）
  --version <版本>       覆盖版本号（默认读取仓库 VERSION，镜像 tag 与之一致）
  --image-source auto|ghcr|swr
                         应用镜像来源，默认 auto（探测连通性后选择）
                           ghcr = ghcr.io/chenjinxiang1980/cyber-drill:${版本}
                           swr  = swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:${版本}
  --registry <ghcr|swr>  --image-source 的别名
  --install-dir <目录>   安装目录，默认 /opt/cyber-drill
  --port <端口>          应用对外端口，默认 8090
  --mysql-port <端口>    MySQL 宿主机回环端口，默认 3306
  --project <名称>       compose 项目名，默认 drill
  --force-env            重新生成 .env（旧文件备份；不删数据卷）
  --show-secrets         结束时再次打印初始口令（默认仅首次安装打印一次）
  --skip-firewall        不修改 firewalld
  --tls-cert <证书>      启用应用内 HTTPS（需与 --tls-key 成对提供，PEM 全链）
  --tls-key <私钥>       应用内 HTTPS 私钥（PEM）；对外交付必须上 TLS
  --reinstall            安装目录已存在时，删除原目录/容器/数据卷后重装
  --confirm DELETE       非交互环境下 --reinstall 的确认词（区分大小写）

已安装检测：
  安装目录已存在且非空 → 直接停止（退出码 28），给两条路径：
    A. sudo bash install.sh --reinstall      # 交互输入 DELETE 确认
    B. sudo bash deploy/upgrade/upgrade.sh --to <版本>   # 保留数据升级

空库判定与保护：
  * drill_system 无表 + 无本安装器标记 → 恢复快照
  * 有本安装器的 .restored 标记     → 视为断点续装，跳过恢复
  * 有表但无标记                    → 拒绝覆盖，提示改用 upgrade.sh 或先清空数据卷

示例：
  sudo bash deploy/install/install.sh
  sudo bash deploy/install/install.sh --image-source swr
  sudo bash deploy/install/install.sh --offline-dir /opt/cyber-drill-offline
  sudo bash deploy/install/install.sh --reinstall
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --offline-dir)  OFFLINE_DIR="${2:-}"; shift ;;
        --snapshot)     SNAPSHOT="${2:-}"; shift ;;
        --mode)         MODE="${2:-}"; shift ;;
        --version)      VERSION_OPT="${2:-}"; shift ;;
        --image-source|--registry) IMAGE_SOURCE="${2:-}"; shift ;;
        --install-dir)  INSTALL_DIR="${2:-}"; shift ;;
        --port)         APP_PORT="${2:-}"; shift ;;
        --mysql-port)   MYSQL_HOST_PORT="${2:-}"; shift ;;
        --project)      PROJECT="${2:-}"; shift ;;
        --force-env)    FORCE_ENV=1 ;;
        --show-secrets) SHOW_SECRETS=1 ;;
        --skip-firewall) SKIP_FIREWALL=1 ;;
        --reinstall)    REINSTALL=1 ;;
        --confirm)      CONFIRM_WORD="${2:-}"; shift ;;
        --tls-cert)     TLS_CERT="${2:-}"; shift ;;
        --tls-key)      TLS_KEY="${2:-}"; shift ;;
        -h|--help)      usage; exit 0 ;;
        *) err "未知参数：$1（--help 查看用法）"; exit 10 ;;
    esac
    shift
done

case "${IMAGE_SOURCE}" in
    auto|ghcr|swr) ;;
    *) err "--image-source 只支持 auto|ghcr|swr，收到：${IMAGE_SOURCE}"; exit 10 ;;
esac

# 容器名跟随项目名：默认 drill 仍是 drill-mysql / drill-app，其它项目名自动隔离为 ${PROJECT}-mysql / ${PROJECT}-app，
# 避免同一台机器上第二套安装因容器名冲突而失败
MYSQL_CONTAINER="${PROJECT}-mysql"
APP_CONTAINER="${PROJECT}-app"

# 版本号单一来源：--version > 仓库 VERSION > 离线包 VERSION > 基线清单 > 源码默认值
resolve_version() {
    local v=""
    if [ -n "${VERSION_OPT}" ]; then
        v="${VERSION_OPT}"
    elif [ -f "${REPO_ROOT}/VERSION" ]; then
        v="$(tr -d ' \t\r\n\357\273\277' < "${REPO_ROOT}/VERSION")"
    elif [ -n "${OFFLINE_DIR}" ] && [ -f "${OFFLINE_DIR}/VERSION" ]; then
        v="$(tr -d ' \t\r\n\357\273\277' < "${OFFLINE_DIR}/VERSION")"
    elif [ -f "${SCRIPT_DIR}/baseline/manifest.env" ]; then
        v="$(sed -n 's/^BASELINE_VERSION=//p' "${SCRIPT_DIR}/baseline/manifest.env" | head -n1 | tr -d ' \t\r\n"')"
    elif [ -f "${REPO_ROOT}/server/internal/version/version.go" ]; then
        v="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' "${REPO_ROOT}/server/internal/version/version.go" | head -n1)"
    fi
    [ -n "${v}" ] || die 10 "无法确定版本号：请提供 --version，或在仓库根目录放置 VERSION 文件"
    case "${v}" in
        *[!0-9A-Za-z._-]*) die 10 "版本号含非法字符：${v}" ;;
    esac
    DRILL_VERSION="${v}"
    APP_IMAGE="cyber-drill:${DRILL_VERSION}"
    GHCR_IMAGE="${GHCR_REGISTRY}:${DRILL_VERSION}"
    SWR_IMAGE="${SWR_REGISTRY}:${DRILL_VERSION}"
}

if [ -z "${OFFLINE_DIR}" ] && [ -d "${SCRIPT_DIR}/offline" ]; then
    OFFLINE_DIR="${SCRIPT_DIR}/offline"
fi
if [ -z "${OFFLINE_DIR}" ] && [ -d "/opt/cyber-drill-offline" ]; then
    OFFLINE_DIR="/opt/cyber-drill-offline"
fi

resolve_version

COMPOSE_FILE="${INSTALL_DIR}/docker-compose.yml"
ENV_FILE="${INSTALL_DIR}/.env"
RESTORE_LOG="${INSTALL_DIR}/logs/restore-$(date +%Y%m%d-%H%M%S).log"
MARKER_RESTORED="${INSTALL_DIR}/.restored"
MARKER_INSTALLED="${INSTALL_DIR}/.installed"
MARKER_PWD_PRINTED="${INSTALL_DIR}/.password_printed"

# ---------- TLS（可选，对外交付必须开启）----------
TLS_ENABLED="false"
HTTP_REDIRECT_TO_HTTPS="false"
TLS_CERT_IN_CONTAINER=""
TLS_KEY_IN_CONTAINER=""
TLS_PORT_LINE=""
TLS_VOLUME_LINE=""
HEALTH_PROBE_URL="http://127.0.0.1:8090/api/health"
if [ -n "${TLS_CERT}" ] || [ -n "${TLS_KEY}" ]; then
    [ -n "${TLS_CERT}" ] && [ -n "${TLS_KEY}" ] || die 10 "--tls-cert 与 --tls-key 必须同时提供"
    [ -f "${TLS_CERT}" ] || die 10 "证书文件不存在：${TLS_CERT}"
    [ -f "${TLS_KEY}" ] || die 10 "私钥文件不存在：${TLS_KEY}"
    TLS_ENABLED="true"
    TLS_CERT_IN_CONTAINER="/opt/drill/tls/fullchain.pem"
    TLS_KEY_IN_CONTAINER="/opt/drill/tls/privkey.pem"
    TLS_PORT_LINE="      - \"${TLS_APP_PORT}:8443\""
    TLS_VOLUME_LINE="      - ${INSTALL_DIR}/tls:/opt/drill/tls:ro"
    HEALTH_PROBE_URL="https://127.0.0.1:${TLS_APP_PORT}/api/health"
fi

# ---------- 工具函数 ----------
rand_hex() { openssl rand -hex "$1"; }
rand_password() { printf 'A9%s' "$(openssl rand -hex 8)"; }

json_value() {
    sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" <<<"$2" | head -n1
}

container_health() {
    docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$1" 2>/dev/null || echo "missing"
}

wait_healthy() {
    local name="$1" timeout="${2:-180}" i=0
    while [ "$i" -lt "$timeout" ]; do
        local st; st="$(container_health "$name")"
        case "$st" in
            healthy) return 0 ;;
            exited|dead|missing) return 1 ;;
        esac
        sleep 2; i=$((i + 2))
    done
    return 1
}

db_query() {
    docker exec "${MYSQL_CONTAINER}" mysql -N -B --default-character-set=utf8mb4 \
        -uroot -p"${MYSQL_ROOT_PASSWORD}" "${DB_NAME}" -e "$1" 2>/dev/null
}

find_snapshot() {
    [ -n "${SNAPSHOT}" ] && return 0
    local d
    for d in "${OFFLINE_DIR:+${OFFLINE_DIR}/baseline}" "${SCRIPT_DIR}/baseline" "${INSTALL_DIR}/baseline"; do
        [ -n "${d}" ] || continue
        local f
        f="$(ls "${d}"/drill-baseline-"${DRILL_VERSION}".sql.gz 2>/dev/null | head -n1)"
        [ -z "${f}" ] && f="$(ls "${d}"/drill-baseline-*.sql.gz 2>/dev/null | head -n1)"
        if [ -n "${f}" ]; then SNAPSHOT="${f}"; return 0; fi
    done
    return 1
}

# ---------- 已有安装检测 ----------
reinstall_wipe() {
    case "${INSTALL_DIR}" in
        ""|"/"|"/opt"|"/usr"|"/etc"|"/var"|"/home") die 28 "安装目录不安全，拒绝删除：${INSTALL_DIR}" ;;
    esac
    if command -v docker >/dev/null 2>&1; then
        local cids vids v key proj
        # 容器：只认“compose 工作目录 == 本安装目录”，绝不按固定容器名删除
        cids="$(docker ps -aq --filter "label=com.docker.compose.project.working_dir=${INSTALL_DIR}" 2>/dev/null || true)"
        # 数据卷：compose 不给卷打 working_dir 标签，只能按“项目名 + 本安装器声明的卷名”精确匹配，
        # 且必须复核卷上的 project 标签，避免误删其他项目
        vids=""
        for key in drill-mysql-data drill-app-data drill-app-uploads; do
            v="${PROJECT}_${key}"
            docker volume inspect "${v}" >/dev/null 2>&1 || continue
            proj="$(docker volume inspect "${v}" --format '{{index .Labels "com.docker.compose.project"}}' 2>/dev/null || true)"
            [ "${proj}" = "${PROJECT}" ] && vids="${vids} ${v}"
        done
        vids="$(printf '%s\n' ${vids} | sed '/^$/d')"

        info "将清理以下资源（容器：工作目录=${INSTALL_DIR}；数据卷：项目名=${PROJECT}）"
        printf '  容器 %s 个\n' "$(printf '%s\n' "${cids}" | sed '/^$/d' | wc -l)"
        [ -z "${cids}" ] || printf '%s\n' "${cids}" | sed 's/^/    /'
        printf '  数据卷 %s 个\n' "$(printf '%s\n' "${vids}" | sed '/^$/d' | wc -l)"
        [ -z "${vids}" ] || printf '%s\n' "${vids}" | sed 's/^/    /'

        if [ -f "${COMPOSE_FILE}" ]; then
            docker compose --project-name "${PROJECT}" -f "${COMPOSE_FILE}" down -v --remove-orphans >/dev/null 2>&1 || true
        fi
        if [ -n "${cids}" ]; then
            # shellcheck disable=SC2086
            docker rm -f ${cids} >/dev/null 2>&1 || true
        fi
        if [ -n "${vids}" ]; then
            # shellcheck disable=SC2086
            docker volume rm ${vids} >/dev/null 2>&1 || true
        fi
    fi
    rm -rf "${INSTALL_DIR}"
    ok "已删除 ${INSTALL_DIR}，继续全新安装"
}

detect_existing_install() {
    [ -d "${INSTALL_DIR}" ] || return 0
    [ -n "$(ls -A "${INSTALL_DIR}" 2>/dev/null || true)" ] || return 0

    local upgrade_cmd="sudo bash ${REPO_ROOT}/deploy/upgrade/upgrade.sh --to ${DRILL_VERSION}"
    [ -f "${REPO_ROOT}/deploy/upgrade/upgrade.sh" ] || upgrade_cmd="sudo bash deploy/upgrade/upgrade.sh --to ${DRILL_VERSION}"

    warn "安装目录已存在且非空：${INSTALL_DIR}"
    cat >&2 <<EOF

检测到本机可能已经安装过 Cyber-Drill。为避免“旧库口令不同导致装完却登不上”，
安装器不会覆盖现有数据，请明确选择其中一条：

  A. 删除原目录并重装（会停止容器、删除数据卷和 ${INSTALL_DIR}，不可恢复）
       sudo bash $0 --reinstall
     执行后必须输入确认词 DELETE 才会真正删除。

  B. 保留数据，改用升级脚本
       ${upgrade_cmd}

EOF

    if [ "${REINSTALL}" -eq 0 ]; then
        die 28 "已存在安装目录，未做任何修改。请按上面的 A 或 B 选择后重新执行。"
    fi

    if [ -t 0 ]; then
        local ans=""
        printf '确认删除 %s 及其容器/数据卷？请输入 DELETE（区分大小写，其他输入取消）：' "${INSTALL_DIR}" >&2
        read -r ans || ans=""
        [ "${ans}" = "DELETE" ] || die 28 "确认词不匹配，已取消，未删除任何文件"
    else
        [ "${CONFIRM_WORD}" = "DELETE" ] || die 28 "非交互环境执行 --reinstall 必须同时提供 --confirm DELETE（区分大小写）"
    fi
    reinstall_wipe
}

# ---------- 镜像仓库连通性与来源选择 ----------
registry_reachable() {
    local base="$1" code
    code="$(curl -sS -o /dev/null -w '%{http_code}' --connect-timeout 5 -m 10 "${base}/v2/" 2>/dev/null || true)"
    case "${code}" in
        200|401|403) return 0 ;;
        *) return 1 ;;
    esac
}

select_image_source() {
    if [ "${MODE}" = "offline" ]; then
        info "离线安装，跳过镜像仓库探测"
        CHOSEN_REGISTRY="offline"
        return 0
    fi

    info "探测镜像仓库连通性（每个 10 秒上限）"
    local ghcr_ok=0 swr_ok=0 chosen=""
    registry_reachable "https://${GHCR_HOST}" && ghcr_ok=1
    registry_reachable "https://${SWR_HOST}" && swr_ok=1
    printf '  %-5s %-40s %s\n' "GHCR" "https://${GHCR_HOST}/v2/" "$([ "${ghcr_ok}" -eq 1 ] && echo 可达 || echo 不可达)"
    printf '  %-5s %-40s %s\n' "SWR" "https://${SWR_HOST}/v2/" "$([ "${swr_ok}" -eq 1 ] && echo 可达 || echo 不可达)"

    case "${IMAGE_SOURCE}" in
        ghcr)
            chosen="ghcr"
            [ "${ghcr_ok}" -eq 1 ] || warn "GHCR 探测不可达，仍按 --image-source ghcr 尝试"
            ;;
        swr)
            chosen="swr"
            [ "${swr_ok}" -eq 1 ] || warn "SWR 探测不可达，仍按 --image-source swr 尝试"
            ;;
        auto)
            if [ "${ghcr_ok}" -eq 1 ] && [ "${swr_ok}" -eq 0 ]; then
                chosen="ghcr"
            elif [ "${swr_ok}" -eq 1 ] && [ "${ghcr_ok}" -eq 0 ]; then
                chosen="swr"
            elif [ "${ghcr_ok}" -eq 1 ] && [ "${swr_ok}" -eq 1 ]; then
                if [ -t 0 ]; then
                    cat >&2 <<EOF

两个镜像仓库均可达，请选择：
  1) 华为云 SWR（国内推荐）  ${SWR_IMAGE}
  2) GHCR（官方，国外网络）   ${GHCR_IMAGE}
EOF
                    local ans=""
                    read -r -p "请输入序号 [1]: " ans || ans=""
                    case "${ans:-1}" in
                        2) chosen="ghcr" ;;
                        *) chosen="swr" ;;
                    esac
                else
                    case "$(date +%Z 2>/dev/null)" in
                        CST|HKT) chosen="swr" ;;
                        *) chosen="ghcr" ;;
                    esac
                    info "两个仓库均可达，非交互环境按本地时区默认 ${chosen}（可用 --image-source 覆盖）"
                fi
            else
                die 21 "GHCR 与 SWR 均不可达：请检查 DNS/代理，或改用 --offline-dir 离线安装"
            fi
            ;;
    esac

    if [ "${chosen}" = "swr" ]; then
        CHOSEN_REGISTRY="${SWR_REGISTRY}"
        CHOSEN_IMAGE="${SWR_IMAGE}"
        FALLBACK_IMAGE="${GHCR_IMAGE}"
    else
        CHOSEN_REGISTRY="${GHCR_REGISTRY}"
        CHOSEN_IMAGE="${GHCR_IMAGE}"
        FALLBACK_IMAGE="${SWR_IMAGE}"
    fi
    ok "应用镜像来源：${chosen}（${CHOSEN_IMAGE}）"
}


# =============================================================================
step "1/9 前置检查"
# =============================================================================
[ "$(id -u)" -eq 0 ] || die 10 "请用 root 执行：sudo bash install.sh"
[ -f /etc/os-release ] || die 10 "无法识别发行版（缺 /etc/os-release）"
# shellcheck disable=SC1091
. /etc/os-release
ARCH="$(uname -m)"
[ "$ARCH" = "x86_64" ] || die 10 "当前仅提供 x86_64 离线资源，本机架构：${ARCH}"

AVAIL_KB=$(df -Pk "$(dirname "${INSTALL_DIR}")" | awk 'NR==2{print $4}')
[ "${AVAIL_KB:-0}" -ge 3145728 ] || die 10 "磁盘可用空间不足 3GB（当前 $((${AVAIL_KB:-0}/1024/1024))GB）"

if [ "${MODE}" = "offline" ] && ! ls "${OFFLINE_DIR}"/images/*.tar* >/dev/null 2>&1; then
    die 10 "指定了离线模式，但 ${OFFLINE_DIR:-<未指定>}/images 下没有镜像包"
fi
if [ "${MODE}" = "auto" ]; then
    if [ -n "${OFFLINE_DIR}" ] && ls "${OFFLINE_DIR}"/images/*.tar* >/dev/null 2>&1; then MODE="offline"; else MODE="online"; fi
fi

HOST_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -n1)"
[ -n "${HOST_IP}" ] || HOST_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "${HOST_IP}" ] || HOST_IP="127.0.0.1"

info "发行版：${PRETTY_NAME:-unknown} (${ARCH})"
info "安装目录：${INSTALL_DIR}"
info "安装来源：${MODE}$([ "${MODE}" = offline ] && echo "（${OFFLINE_DIR}）")"
info "版本号：${DRILL_VERSION}（镜像 tag 与 VERSION 一致）"
info "访问地址：http://${HOST_IP}:${APP_PORT}"

detect_existing_install

if ss -lnt 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${APP_PORT}$"; then
    if [ "$(container_health "${APP_CONTAINER}")" = "missing" ]; then
        die 10 "端口 ${APP_PORT} 已被其他进程占用，请用 --port 指定其他端口"
    fi
    info "端口 ${APP_PORT} 已由本项目容器占用（重复执行场景）"
fi

# =============================================================================
step "2/9 安装 Docker 引擎与 Compose"
# =============================================================================
if ! command -v docker >/dev/null 2>&1; then
    [ -n "${PKG_MGR}" ] || die 20 "未找到 dnf/yum，无法自动安装 Docker；请先手工安装 Docker 后重试"
    if [ "${MODE}" = "offline" ]; then
        [ -d "${OFFLINE_DIR}/rpms" ] || die 20 "离线目录缺少 rpms/"
        info "离线安装 Docker RPM（$(ls -1 "${OFFLINE_DIR}"/rpms/*.rpm 2>/dev/null | wc -l) 个包）"
        "${PKG_MGR}" install -y --disablerepo='*' "${OFFLINE_DIR}"/rpms/*.rpm || die 20 "离线 RPM 安装失败"
    else
        # openEuler 源内是 moby-engine；UOS / RHEL 等发行版的源包名不同，按可用性回退
        if "${PKG_MGR}" -q list --available moby-engine >/dev/null 2>&1; then
            info "在线安装 moby-engine（openEuler 源）"
            "${PKG_MGR}" install -y moby-engine moby-client || die 20 "Docker 引擎安装失败"
        elif "${PKG_MGR}" -q list --available docker-ce >/dev/null 2>&1; then
            info "在线安装 docker-ce（发行版自带/自备源）"
            "${PKG_MGR}" install -y docker-ce docker-ce-cli containerd.io || die 20 "docker-ce 安装失败"
        else
            die 20 "当前软件源内未找到 moby-engine 或 docker-ce。请先按发行版文档安装 Docker，或用 --offline-dir 提供 RPM 离线包后重试。"
        fi
    fi
else
    info "Docker 已安装：$(docker --version 2>/dev/null | head -n1)"
fi

systemctl enable docker >/dev/null 2>&1 || true
systemctl start docker || die 20 "Docker 服务启动失败（journalctl -u docker -n 50 查看）"
docker info >/dev/null 2>&1 || die 20 "Docker 守护进程不可用"

install_compose() {
    local plugin_dir="/usr/local/lib/docker/cli-plugins" url tmp
    mkdir -p "${plugin_dir}"
    if [ -n "${OFFLINE_DIR}" ] && [ -f "${OFFLINE_DIR}/bin/docker-compose" ]; then
        install -m 0755 "${OFFLINE_DIR}/bin/docker-compose" "${plugin_dir}/docker-compose"
        info "已安装离线 compose 二进制"
        return 0
    fi
    tmp="$(mktemp)"
    for url in "${COMPOSE_URLS[@]}"; do
        info "下载 compose：${url%%/https*}"
        if curl -fL --connect-timeout 10 --speed-limit 102400 --speed-time 60 -m 600 -o "${tmp}" "${url}"; then
            install -m 0755 "${tmp}" "${plugin_dir}/docker-compose"
            rm -f "${tmp}"
            return 0
        fi
    done
    rm -f "${tmp}"
    return 1
}

if ! docker compose version >/dev/null 2>&1; then
    install_compose || die 20 "compose v2 安装失败；请把 docker-compose 二进制放入离线目录 bin/"
    mkdir -p /usr/libexec/docker/cli-plugins
    ln -sf /usr/local/lib/docker/cli-plugins/docker-compose /usr/libexec/docker/cli-plugins/docker-compose
fi
docker compose version >/dev/null 2>&1 || die 20 "compose v2 不可用"
ok "$(docker --version | head -n1)；$(docker compose version | head -n1)"

# =============================================================================
step "3/9 准备镜像"
# =============================================================================
select_image_source

load_or_pull() {
    local image="$1" tar_pattern="$2" pull_candidates=("${@:3}") c tar
    if docker image inspect "${image}" >/dev/null 2>&1; then
        info "镜像已存在：${image}"
        return 0
    fi
    if [ "${MODE}" = "offline" ]; then
        tar="$(ls ${tar_pattern} 2>/dev/null | head -n1)"
        [ -n "${tar}" ] || die 21 "离线镜像包缺失：${tar_pattern}"
        info "导入镜像：${tar}"
        gzip -dc "${tar}" | docker load || die 21 "镜像导入失败：${tar}"
        return 0
    fi
    for c in "${pull_candidates[@]}"; do
        info "拉取镜像：${c}"
        if docker pull "${c}" && docker tag "${c}" "${image}"; then return 0; fi
    done
    die 21 "镜像拉取失败：${image}"
}

load_or_pull "${APP_IMAGE}" "${OFFLINE_DIR:-/nonexistent}/images/cyber-drill-*.tar*" "${CHOSEN_IMAGE:-${GHCR_IMAGE}}" "${FALLBACK_IMAGE:-${SWR_IMAGE}}"
load_or_pull "${MYSQL_IMAGE}" "${OFFLINE_DIR:-/nonexistent}/images/mysql-*.tar*" "${MYSQL_MIRRORS[@]}"
docker image inspect "${APP_IMAGE}" >/dev/null 2>&1 || die 21 "应用镜像不存在：${APP_IMAGE}"

# =============================================================================
step "4/9 定位基线快照"
# =============================================================================
find_snapshot || die 27 "未找到基线快照。请用 --snapshot 指定 drill-baseline-${DRILL_VERSION}.sql.gz，或把 baseline/ 放进离线包。"
[ -f "${SNAPSHOT}" ] || die 27 "快照文件不存在：${SNAPSHOT}"
info "基线快照：${SNAPSHOT}（$(du -h "${SNAPSHOT}" | awk '{print $1}')）"
SNAPSHOT_SHA="$(sha256sum "${SNAPSHOT}" | awk '{print $1}')"
info "快照 SHA256：${SNAPSHOT_SHA}"
if [ -f "$(dirname "${SNAPSHOT}")/SHA256SUMS" ]; then
    # 基线产物可能在 Windows 环境生成，带 UTF-8 BOM 时 sha256sum 会拒绝解析；
    # 这里先清洗到临时文件再校验，兼容 BOM 与 CRLF。
    SUMS_CLEAN="$(mktemp)"
    sed '1s/^\xEF\xBB\xBF//' "$(dirname "${SNAPSHOT}")/SHA256SUMS" | tr -d '\r' > "${SUMS_CLEAN}"
    (cd "$(dirname "${SNAPSHOT}")" && sha256sum -c "${SUMS_CLEAN}" --ignore-missing >/dev/null 2>&1) \
        || { rm -f "${SUMS_CLEAN}"; die 27 "快照校验失败（SHA256SUMS 不匹配）"; }
    rm -f "${SUMS_CLEAN}"
    ok "SHA256SUMS 校验通过"
fi
BASELINE_MANIFEST="$(dirname "${SNAPSHOT}")/manifest.env"
if [ -f "${BASELINE_MANIFEST}" ]; then
    MANIFEST_CLEAN="$(mktemp)"
    sed '1s/^\xEF\xBB\xBF//' "${BASELINE_MANIFEST}" | tr -d '\r' > "${MANIFEST_CLEAN}"
    manifest_get() { sed -n "s/^$1=//p" "${MANIFEST_CLEAN}" | head -n1 | tr -d '"'; }
    BASELINE_MIGRATIONS="$(manifest_get BASELINE_MIGRATIONS)"
    BASELINE_BRAND_VERSION="$(manifest_get BASELINE_BRAND_VERSION)"
    BASELINE_SCENARIOS="$(manifest_get BASELINE_SCENARIOS)"
    BASELINE_SCRIPT_NODES="$(manifest_get BASELINE_SCRIPT_NODES)"
    BASELINE_FORM_TEMPLATES="$(manifest_get BASELINE_FORM_TEMPLATES)"
    BASELINE_DICT="$(manifest_get BASELINE_DICT)"
    BASELINE_EVENT_TYPES="$(manifest_get BASELINE_EVENT_TYPES)"
fi

# =============================================================================
step "5/9 创建目录与 compose 配置"
# =============================================================================
mkdir -p "${INSTALL_DIR}"/{logs,data,uploads}
chmod 0750 "${INSTALL_DIR}"
cp -f "${SNAPSHOT}" "${INSTALL_DIR}/baseline.sql.gz"
chmod 0644 "${INSTALL_DIR}/baseline.sql.gz"
if [ "${TLS_ENABLED}" = "true" ]; then
    mkdir -p "${INSTALL_DIR}/tls"
    install -m 0644 "${TLS_CERT}" "${INSTALL_DIR}/tls/fullchain.pem"
    install -m 0600 "${TLS_KEY}" "${INSTALL_DIR}/tls/privkey.pem"
    # 容器以 uid 10001 运行，证书与私钥必须归它所有
    chown -R 10001:10001 "${INSTALL_DIR}/tls" 2>/dev/null || warn "无法把 tls/ 属主改为 10001:10001，应用可能读不到证书"
    chmod 0750 "${INSTALL_DIR}/tls"
    ok "已启用应用内 HTTPS（证书 ${INSTALL_DIR}/tls/fullchain.pem）"
fi

cat > "${COMPOSE_FILE}" <<EOF
# 由 install.sh 生成，重复执行会覆盖；数据在命名卷中，不受本文件影响。
services:
  mysql:
    image: ${MYSQL_IMAGE}
    container_name: ${MYSQL_CONTAINER}
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: \${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: \${DB_NAME}
      MYSQL_USER: \${DB_USER}
      MYSQL_PASSWORD: \${DB_PASSWORD}
      TZ: Asia/Shanghai
    ports:
      - "127.0.0.1:${MYSQL_HOST_PORT}:3306"
    volumes:
      - drill-mysql-data:/var/lib/mysql
    command: >
      --character-set-server=utf8mb4
      --collation-server=utf8mb4_unicode_ci
      --default-authentication-plugin=mysql_native_password
      --max-connections=300
      --default-time-zone='+08:00'
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h localhost -uroot -p\$\$MYSQL_ROOT_PASSWORD --silent"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s
    networks:
      - drill-net

  app:
    image: ${APP_IMAGE}
    container_name: ${APP_CONTAINER}
    restart: unless-stopped
    env_file:
      - ${ENV_FILE}
    environment:
      DB_HOST: mysql
      APP_ENV: production
      HTTP_ADDR: 0.0.0.0:8090
      TRUSTED_PROXIES: "172.17.0.0/16,172.18.0.0/16,127.0.0.1/32"
      # 首次改密成功后，应用会把 .env 里的一次性初始口令就地清空并标注失效
      DRILL_ENV_FILE_PATH: /opt/drill/etc/deploy.env
    ports:
      - "${APP_PORT}:8090"
${TLS_PORT_LINE}
    volumes:
      - drill-app-data:/opt/drill/data
      - drill-app-uploads:/opt/drill/uploads
      - ${ENV_FILE}:/opt/drill/etc/deploy.env
${TLS_VOLUME_LINE}
    depends_on:
      mysql:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--no-check-certificate", "-qO-", "${HEALTH_PROBE_URL}"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
    networks:
      - drill-net

volumes:
  drill-mysql-data:
  drill-app-data:
  drill-app-uploads:

networks:
  drill-net:
    driver: bridge
EOF
chmod 0640 "${COMPOSE_FILE}"
ok "compose 配置：${COMPOSE_FILE}"

# =============================================================================
step "6/9 生成环境文件（随机强口令，权限 600）"
# =============================================================================
if [ "${TLS_ENABLED}" = "true" ]; then
    PUBLIC_BASE_URL="https://${HOST_IP}:${TLS_APP_PORT}"
else
    PUBLIC_BASE_URL="http://${HOST_IP}:${APP_PORT}"
fi
if [ -f "${ENV_FILE}" ] && [ "${FORCE_ENV}" -eq 0 ]; then
    info "${ENV_FILE} 已存在，沿用现有口令与密钥（不覆盖）"
    chmod 600 "${ENV_FILE}"
else
    if [ -f "${ENV_FILE}" ]; then
        cp -p "${ENV_FILE}" "${ENV_FILE}.bak.$(date +%Y%m%d-%H%M%S)"
        warn "已备份旧 .env"
    fi
    DB_USER="drill_app"
    DB_PASSWORD="$(rand_hex 16)"
    MYSQL_ROOT_PASSWORD="$(rand_hex 16)"
    ADMIN_INIT_PASSWORD="$(rand_password)"
    PLATFORMADMIN_INIT_PASSWORD="$(rand_password)"
    umask 077
    cat > "${ENV_FILE}" <<EOF
# 由 install.sh 生成；含明文口令，权限必须保持 600。
# 重复执行 install.sh 默认沿用本文件；--force-env 会备份后重建。
APP_ENV=production
DRILL_VERSION=${DRILL_VERSION}
HTTP_ADDR=0.0.0.0:8090
PUBLIC_BASE_URL=${PUBLIC_BASE_URL}

DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=drill_system
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASSWORD}
DB_SSLMODE=disable
MYSQL_ROOT_USER=root
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}

JWT_SECRET=$(rand_hex 32)
SESSION_SECRET=$(rand_hex 32)

ADMIN_INIT_PASSWORD=${ADMIN_INIT_PASSWORD}
PLATFORMADMIN_INIT_PASSWORD=${PLATFORMADMIN_INIT_PASSWORD}

UPLOAD_MAX_MB=20
RATE_LIMIT_GLOBAL=300
RATE_LIMIT_AUTH=30
RATE_LIMIT_CAPTCHA=20
ALLOW_ORIGINS=
LOGIN_MAX_FAILURES=5
LOGIN_LOCK_MINUTES=15
CAPTCHA_ENABLED=true
CAPTCHA_TTL_SECONDS=120
# 大屏匿名只读：保持 true（关闭会导致大屏打不开）；要收口请在 firewalld / 反代层限源
SCREEN_PUBLIC_VIEW=true
# 审计保留天数（0 = 不清理）
AUDIT_RETENTION_DAYS=365
# 备份默认 AES-256-GCM 加密；缺失该口令时备份接口直接拒绝（不会退化为明文）
BACKUP_PASSPHRASE=$(rand_hex 32)
# 恢复入口独立口令（与登录口令分离）；缺失时禁止恢复
RESTORE_PASSWORD=$(rand_password)
TLS_ENABLED=${TLS_ENABLED}
TLS_ADDR=:8443
HTTPS_CERT_FILE=${TLS_CERT_IN_CONTAINER}
HTTPS_KEY_FILE=${TLS_KEY_IN_CONTAINER}
HTTP_REDIRECT_TO_HTTPS=${HTTP_REDIRECT_TO_HTTPS}
TRUSTED_PROXIES=172.17.0.0/16,172.18.0.0/16,127.0.0.1/32
EOF
    umask 022
    ok "已生成 ${ENV_FILE}（权限 600，随机口令与密钥）"
fi
# 老 .env 补齐新增项：空值或缺失时生成，已有非空值一律不覆盖
ensure_env_value() {
    local key="$1" value="$2"
    if grep -qE "^${key}=.+" "${ENV_FILE}"; then
        return 0
    fi
    if grep -q "^${key}=" "${ENV_FILE}"; then
        sed -i "s|^${key}=.*|${key}=${value}|" "${ENV_FILE}"
    else
        printf '%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
    fi
}
ensure_env_value BACKUP_PASSPHRASE "$(rand_hex 32)"
ensure_env_value RESTORE_PASSWORD "$(rand_password)"
ensure_env_value AUDIT_RETENTION_DAYS "365"
ensure_env_value SCREEN_PUBLIC_VIEW "true"
ensure_env_value CAPTCHA_ENABLED "true"
if [ "${TLS_ENABLED}" = "true" ]; then
    ensure_env_value TLS_ENABLED "true"
    ensure_env_value TLS_ADDR ":8443"
    ensure_env_value HTTPS_CERT_FILE "${TLS_CERT_IN_CONTAINER}"
    ensure_env_value HTTPS_KEY_FILE "${TLS_KEY_IN_CONTAINER}"
    sed -i "s|^TLS_ENABLED=.*|TLS_ENABLED=true|" "${ENV_FILE}"
    sed -i "s|^HTTPS_CERT_FILE=.*|HTTPS_CERT_FILE=${TLS_CERT_IN_CONTAINER}|" "${ENV_FILE}"
    sed -i "s|^HTTPS_KEY_FILE=.*|HTTPS_KEY_FILE=${TLS_KEY_IN_CONTAINER}|" "${ENV_FILE}"
fi
chmod 600 "${ENV_FILE}"
# 应用容器以 uid 10001(drill) 运行；该文件会挂进容器用于“首登改密后清空一次性口令”，
# 因此属主交给容器用户，权限仍为 600（root 与容器用户可读写，其他用户不可读）。
if ! chown 10001:10001 "${ENV_FILE}" 2>/dev/null; then
    warn "无法把 ${ENV_FILE} 属主改为 10001:10001；首次改密后一次性口令不会自动从 .env 清除"
fi
# shellcheck disable=SC1090
set -a; . "${ENV_FILE}"; set +a

# =============================================================================
step "7/9 初始化数据库（基线快照）"
# =============================================================================
compose() { docker compose --project-name "${PROJECT}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" "$@"; }
compose up -d mysql || die 23 "MySQL 容器启动失败"
if ! wait_healthy "${MYSQL_CONTAINER}" 180; then
    compose logs --tail=40 mysql >&2 || true
    die 23 "MySQL 180 秒内未就绪"
fi
ok "MySQL 就绪（${MYSQL_CONTAINER}）"

TABLE_COUNT="$(db_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}'")"
TABLE_COUNT="${TABLE_COUNT:-0}"
info "当前库 ${DB_NAME} 表数量：${TABLE_COUNT}"

DB_INITIALIZED=0
if [ "${TABLE_COUNT}" -eq 0 ]; then
    info "空库，开始恢复基线快照"
    set +e
    gzip -dc "${SNAPSHOT}" | docker exec -i "${MYSQL_CONTAINER}" mysql \
        --default-character-set=utf8mb4 -uroot -p"${MYSQL_ROOT_PASSWORD}" "${DB_NAME}" 2>&1 | tee "${RESTORE_LOG}"
    RESTORE_RC=${PIPESTATUS[1]}
    set -e
    if [ "${RESTORE_RC}" -ne 0 ]; then
        err "基线快照恢复失败（exit=${RESTORE_RC}）。日志：${RESTORE_LOG}"
        exit 27
    fi
    DB_INITIALIZED=1
elif [ -f "${MARKER_RESTORED}" ]; then
    info "检测到本安装器的恢复标记，跳过快照恢复（断点续装）"
else
    die 26 "检测到已有数据库 ${DB_NAME}（${TABLE_COUNT} 张表），且不是本安装器创建的。为防误删已拒绝覆盖；升级请用 deploy/upgrade/upgrade.sh，确需重建请先 docker compose down -v 清空数据卷。"
fi

# --- 口径校验 ---
MIG_COUNT="$(db_query "SELECT COUNT(*) FROM schema_migrations")"
BRAND_VERSION="$(db_query "SELECT value FROM settings WHERE \`key\`='brand_version'")"
SCENARIO_COUNT="$(db_query "SELECT COUNT(*) FROM scenarios")"
NODE_COUNT="$(db_query "SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL")"
FORM_COUNT="$(db_query "SELECT COUNT(*) FROM form_templates")"
DICT_COUNT="$(db_query "SELECT COUNT(*) FROM dict")"
EVENT_TYPES="$(db_query "SELECT JSON_LENGTH(value) FROM dict WHERE \`key\`='eventTypes'")"
USER_COUNT="$(db_query "SELECT COUNT(*) FROM users")"

EXPECT_MIG="${BASELINE_MIGRATIONS:-76}"
EXPECT_BRAND="${BASELINE_BRAND_VERSION:-v1.0.0}"
EXPECT_SCEN="${BASELINE_SCENARIOS:-21}"
EXPECT_NODES="${BASELINE_SCRIPT_NODES:-1165}"
EXPECT_FORMS="${BASELINE_FORM_TEMPLATES:-22}"
EXPECT_DICT="${BASELINE_DICT:-12}"
EXPECT_EVTYPES="${BASELINE_EVENT_TYPES:-20}"

check_equal() {
    local name="$1" actual="$2" expect="$3"
    if [ "${actual}" != "${expect}" ]; then
        err "基线校验失败：${name} 期望 ${expect}，实际 ${actual}"
        exit 27
    fi
    info "校验通过：${name}=${actual}"
}
check_equal "schema_migrations" "${MIG_COUNT}" "${EXPECT_MIG}"
check_equal "brand_version" "${BRAND_VERSION}" "${EXPECT_BRAND}"
check_equal "scenarios" "${SCENARIO_COUNT}" "${EXPECT_SCEN}"
check_equal "script_nodes" "${NODE_COUNT}" "${EXPECT_NODES}"
check_equal "form_templates" "${FORM_COUNT}" "${EXPECT_FORMS}"
check_equal "dict" "${DICT_COUNT}" "${EXPECT_DICT}"
check_equal "dict.eventTypes" "${EVENT_TYPES}" "${EXPECT_EVTYPES}"

printf 'version=%s\nsha256=%s\nrestored_at=%s\n' "${DRILL_VERSION}" "${SNAPSHOT_SHA}" "$(date '+%Y-%m-%d %H:%M:%S')" > "${MARKER_RESTORED}"
ok "基线快照恢复并校验通过（应用内置迁移已全部标记为已执行：${MIG_COUNT} 条）"

# =============================================================================
step "8/9 启动应用并健康检查"
# =============================================================================
compose up -d app || die 25 "应用容器启动失败"
if ! wait_healthy "${APP_CONTAINER}" 240; then
    compose logs --tail=60 app >&2 || true
    die 25 "应用 240 秒内未通过健康检查"
fi
if [ "${TLS_ENABLED}" = "true" ]; then
    HEALTH="$(curl -fsSk --max-time 5 "https://127.0.0.1:${TLS_APP_PORT}/api/health" 2>/dev/null || true)"
else
    HEALTH="$(curl -fsS --max-time 5 "http://127.0.0.1:${APP_PORT}/api/health" 2>/dev/null || true)"
fi
[ -n "${HEALTH}" ] || die 25 "容器 healthy，但 /api/health 无响应"
HEALTH_VERSION="$(json_value version "${HEALTH}")"
[ -n "${HEALTH_VERSION}" ] || die 25 "健康接口返回异常：${HEALTH}"

# 管理员账号由应用启动时的 seed 创建（快照不含任何现网账号口令）
USER_COUNT_AFTER="$(db_query "SELECT COUNT(*) FROM users")"
MUST_CHANGE="$(db_query "SELECT COUNT(*) FROM users WHERE username IN ('admin','platformadmin') AND must_change=1")"
if [ "${DB_INITIALIZED}" -eq 1 ] && [ "${USER_COUNT_AFTER}" -ge 2 ]; then
    info "管理员账号已创建（admin / platformadmin，must_change=1）"
fi

# =============================================================================
step "9/9 防火墙与收尾"
# =============================================================================
if [ "${SKIP_FIREWALL}" -eq 0 ] && systemctl is-active --quiet firewalld 2>/dev/null; then
    firewall-cmd --permanent --add-port="${APP_PORT}/tcp" >/dev/null 2>&1 || true
    if [ "${TLS_ENABLED}" = "true" ]; then
        firewall-cmd --permanent --add-port="${TLS_APP_PORT}/tcp" >/dev/null 2>&1 || true
    fi
    firewall-cmd --reload >/dev/null 2>&1 || true
    info "firewalld 已放行 ${APP_PORT}/tcp"
    if [ "${TLS_ENABLED}" = "true" ]; then
        info "firewalld 已放行 ${TLS_APP_PORT}/tcp（HTTPS）"
    fi
fi

printf 'version=%s\nsha256=%s\ninstalled_at=%s\n' "${DRILL_VERSION}" "${SNAPSHOT_SHA}" "$(date '+%Y-%m-%d %H:%M:%S')" > "${MARKER_INSTALLED}"

printf '\n%s\n' "============================================================"
ok "安装完成（基线快照模式）"
printf '%s\n' "------------------------------------------------------------"
printf '  版本        : %s（镜像 %s）\n' "${HEALTH_VERSION}" "${APP_IMAGE}"
printf '  迁移条数    : %s\n' "${MIG_COUNT}"
printf '  brand_version: %s\n' "${BRAND_VERSION}"
printf '  参照数据    : 剧本 %s / 节点 %s / 表单 %s / 字典 %s（事件类型 %s）\n' \
    "${SCENARIO_COUNT}" "${NODE_COUNT}" "${FORM_COUNT}" "${DICT_COUNT}" "${EVENT_TYPES}"
printf '  访问地址    : %s\n' "${PUBLIC_BASE_URL}"
printf '  健康接口    : %s\n' "${HEALTH}"
printf '  安装目录    : %s\n' "${INSTALL_DIR}"
printf '  环境文件    : %s（权限 600）\n' "${ENV_FILE}"
printf '  基线快照    : %s（SHA256 %s）\n' "${INSTALL_DIR}/baseline.sql.gz" "${SNAPSHOT_SHA}"
printf '%s\n' "------------------------------------------------------------"
printf '  初始账号    : admin / platformadmin（首次登录必须立即修改口令）\n'
printf '  口令落盘    : %s（权限 600；请妥善保管，勿外发）\n' "${ENV_FILE}"
if [ ! -f "${MARKER_PWD_PRINTED}" ]; then
    printf '%s\n' "  ----------------------------------------------------------"
    printf '  ★ admin 初始口令         : %s\n' "${ADMIN_INIT_PASSWORD:-<未找到：可能已被首登改密清空>}"
    printf '  ★ platformadmin 初始口令 : %s\n' "${PLATFORMADMIN_INIT_PASSWORD:-<未找到：可能已被首登改密清空>}"
    printf '  ★ 以上口令仅显示一次，请立即记录并转交使用人；首次登录必须改密。\n'
    printf '%s\n' "  ----------------------------------------------------------"
    touch "${MARKER_PWD_PRINTED}"
    chmod 600 "${MARKER_PWD_PRINTED}"
elif [ "${SHOW_SECRETS}" -eq 1 ]; then
    printf '  ★ admin 初始口令         : %s\n' "${ADMIN_INIT_PASSWORD:-<已失效或已被首登改密清空>}"
    printf '  ★ platformadmin 初始口令 : %s\n' "${PLATFORMADMIN_INIT_PASSWORD:-<已失效或已被首登改密清空>}"
else
    printf '  初始口令    : 仅首次安装时打印一次；如已遗失，见 %s（权限 600）\n' "${ENV_FILE}"
fi
printf '  备份加密口令: 见 %s 的 BACKUP_PASSPHRASE（本脚本不打印；遗失将无法恢复旧备份）\n' "${ENV_FILE}"
if [ "${SHOW_SECRETS}" -eq 1 ]; then
    printf '  恢复口令    : %s\n' "${RESTORE_PASSWORD:-<未设置>}"
else
    printf '  恢复口令    : 见 %s 的 RESTORE_PASSWORD（权限 600，与登录口令分离）\n' "${ENV_FILE}"
fi
printf '  恢复风险    : 恢复会覆盖整库并清空全部登录会话，仅管理员本机操作\n'
printf '%s\n' "------------------------------------------------------------"
printf '  查看状态    : docker compose -p %s -f %s ps\n' "${PROJECT}" "${COMPOSE_FILE}"
printf '  查看日志    : docker logs -f %s\n' "${APP_CONTAINER}"
printf '  停止服务    : docker compose -p %s -f %s stop\n' "${PROJECT}" "${COMPOSE_FILE}"
printf '  重新执行    : bash %s（会检测到已安装并提示删除重装或升级）\n' "$0"
printf '%s\n' "============================================================"
