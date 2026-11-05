#!/usr/bin/env bash
# =============================================================================
# Cyber-Drill（应急演练指挥系统）0.5.1 —— Linux 一键安装脚本
#
# 功能：
#   1) 检测 Docker / Docker Compose v2；未装则按发行版给出安装命令（需确认）
#   2) 选择源码来源：GitHub / Gitee / 已有本地目录（含离线包）
#   3) 选择镜像来源：GHCR（公开）/ SWR 华为云（公开或 AK 登录）/ 本地镜像
#   4) 生成 drill.env（缺失的密钥自动随机生成，已有文件绝不覆盖）
#   5) docker compose up -d，轮询 /api/health 直到 200 或超时
#
# 用法：
#   bash install-linux.sh                 # 交互式
#   bash install-linux.sh --yes           # 全默认、免确认（自动化）
#   bash install-linux.sh --dry-run       # 只打印命令，不做任何变更
#
#   常用参数：
#     --source github|gitee|local     源码来源
#     --registry ghcr|swr|local       镜像来源
#     --dir <路径>                    源码目录（默认 ~/cyber-drill）
#     --port <端口>                   应用宿主端口（默认 8090）
#     --mysql-port <端口>             MySQL 宿主端口（默认 3306）
#     --project <名称>                compose 项目名（默认 drill）
#     --env-file <文件名>             环境文件名（默认 drill.env）
#     --image-tar <文件>              本地镜像 tar（docker load）
#     --bundle <tar.gz>               离线源码包（按已发布 SHA256 校验）
#
# 环境变量等价参数：DRILL_SOURCE / DRILL_REGISTRY / DRILL_DIR / DRILL_PORT
#
# 安全约定：
#   - 脚本内零硬编码凭据；SWR 口令只经 docker login --password-stdin 传入
#   - drill.env 权限 600；已有文件默认沿用，不覆盖、不删数据卷
# =============================================================================
set -u

VERSION="0.5.1"
# 安装器随 main 分支发布（v0.5.1 标签与安装器同源），
# 源码版本固定为 0.5.1；可用 DRILL_REF 覆盖分支/标签。
REF="${DRILL_REF:-main}"
GITHUB_REPO="https://github.com/chenjinxiang1980/cyber-drill.git"
GITEE_REPO="https://gitee.com/habeas/cyber-drill.git"
GITHUB_ARCHIVE="https://github.com/chenjinxiang1980/cyber-drill/archive/refs/heads/${REF}.tar.gz"
GITEE_ARCHIVE="https://gitee.com/habeas/cyber-drill/repository/archive/${REF}.tar.gz"
GHCR_IMAGE="ghcr.io/chenjinxiang1980/cyber-drill:${VERSION}"
SWR_HOST_DEFAULT="swr.cn-east-3.myhuaweicloud.com"
SWR_ORG_DEFAULT="chenjinxiang"
BUNDLE_SHA256="BB448855A4BB9F3F00ACD7803F4A0364C7D9091B936349BE8BB0D16FBA459B71"

DRY_RUN=0
ASSUME_YES=0
SOURCE="${DRILL_SOURCE:-}"
REGISTRY="${DRILL_REGISTRY:-}"
SRC_DIR="${DRILL_DIR:-$HOME/cyber-drill}"
PORT="${DRILL_PORT:-8090}"
MYSQL_PORT="3306"
PROJECT="drill"
ENV_FILE="drill.env"
IMAGE_TAR=""
BUNDLE=""
UPDATE_POLICY="${DRILL_UPDATE:-check}"
FORCE_RESET=0
GENERATED_ADMIN_PW=""
DOCKER_STATE="unknown"
COMPOSE_FILE=""
ENV_PATH=""
SOURCE_URL=""
SWR_HOST="$SWR_HOST_DEFAULT"
SWR_ORG="$SWR_ORG_DEFAULT"
SWR_AK=""

# ---------- 输出 ----------
info()  { printf '[信息] %s\n' "$*"; }
warn()  { printf '[提示] %s\n' "$*" >&2; }
err()   { printf '[错误] %s\n' "$*" >&2; }
step()  { printf '\n=== %s ===\n' "$*"; }

usage() {
    cat <<'EOF'
Cyber-Drill 0.5.1 Linux 一键安装

用法：
  bash install-linux.sh [参数]

参数：
  --dry-run              只打印将执行的命令，不安装/不拉取/不启动
  --yes（或 -y）         跳过交互确认（自动化）
  --source  github|gitee|local
  --registry ghcr|swr|local
  --dir <路径>           源码目录（默认 ~/cyber-drill）
  --port <端口>          应用端口（默认 8090）
  --mysql-port <端口>    MySQL 端口（默认 3306）
  --project <名称>       compose 项目名（默认 drill）
  --env-file <文件名>    环境文件名（默认 drill.env）
  --image-tar <文件>     本地镜像 tar（docker load）
  --bundle <tar.gz>      离线源码包（按已发布 SHA256 校验）
  --update               检查远端 ref，一致则跳过下载，不一致增量更新（默认）
  --no-update            只用本地源码，不联网检查
  --force-reset          本地非 git / 有未提交改动时仍强制更新（会先备份 drill.env）
EOF
}

# ---------- 参数解析 ----------
while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run)     DRY_RUN=1 ;;
        --yes|-y)      ASSUME_YES=1 ;;
        --source)      SOURCE="${2:-}"; shift ;;
        --registry)    REGISTRY="${2:-}"; shift ;;
        --dir)         SRC_DIR="${2:-}"; shift ;;
        --port)        PORT="${2:-}"; shift ;;
        --mysql-port)  MYSQL_PORT="${2:-}"; shift ;;
        --project)     PROJECT="${2:-}"; shift ;;
        --env-file)    ENV_FILE="${2:-}"; shift ;;
        --image-tar)   IMAGE_TAR="${2:-}"; shift ;;
        --bundle)      BUNDLE="${2:-}"; shift ;;
        --update)      UPDATE_POLICY="check" ;;
        --no-update)   UPDATE_POLICY="never" ;;
        --force-reset) UPDATE_POLICY="check"; FORCE_RESET=1 ;;
        -h|--help)     usage; exit 0 ;;
        *) err "未知参数：$1（--help 查看用法）"; exit 1 ;;
    esac
    shift
done

# ---------- 工具函数 ----------
is_dry() { [ "$DRY_RUN" = "1" ]; }

run() {
    if is_dry; then
        printf '  [dry-run] %s\n' "$*"
        return 0
    fi
    printf '  + %s\n' "$*"
    "$@"
}

run_root() {
    if [ "$(id -u)" -eq 0 ]; then
        run "$@"
    elif command -v sudo >/dev/null 2>&1; then
        if is_dry; then
            printf '  [dry-run] sudo %s\n' "$*"
        else
            printf '  + sudo %s\n' "$*"
            sudo "$@"
        fi
    else
        err "该操作需要 root 权限，但系统没有 sudo；请用 root 账户执行。"
        return 1
    fi
}

run_shell() {
    if is_dry; then
        printf '  [dry-run] %s\n' "$1"
        return 0
    fi
    printf '  + %s\n' "$1"
    sh -c "$1"
}

confirm() {
    if [ "$ASSUME_YES" = "1" ]; then
        printf '%s -> 自动确认（--yes）\n' "$1"
        return 0
    fi
    printf '%s [y/N]: ' "$1"
    read -r ans || ans=""
    case "$ans" in
        y|Y|yes|YES|Yes) return 0 ;;
        *) return 1 ;;
    esac
}

choose() {
    printf '\n%s\n  1) %s\n  2) %s\n  3) %s\n' "$1" "$2" "$3" "$4"
    printf '请选择 [%s]: ' "$5"
    read -r a || a=""
    [ -z "$a" ] && a="$5"
    printf '%s' "$a"
}

gen_hex() {
    n="$1"
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex "$n"
    elif [ -r /dev/urandom ]; then
        od -An -tx1 -N "$n" /dev/urandom | tr -d ' \n'
    else
        err "无法生成随机密钥（缺少 openssl 且 /dev/urandom 不可读）"
        return 1
    fi
}

validate_args() {
    case "$PORT" in
        ''|*[!0-9]*) err "--port 必须是数字：$PORT"; return 1 ;;
    esac
    case "$MYSQL_PORT" in
        ''|*[!0-9]*) err "--mysql-port 必须是数字：$MYSQL_PORT"; return 1 ;;
    esac
    if [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then err "端口越界：$PORT"; return 1; fi
    if [ "$MYSQL_PORT" -lt 1 ] || [ "$MYSQL_PORT" -gt 65535 ]; then err "端口越界：$MYSQL_PORT"; return 1; fi
    if [ "$PORT" = "$MYSQL_PORT" ]; then err "应用端口与 MySQL 端口不能相同（$PORT）"; return 1; fi
    if [ -n "$SOURCE" ]; then
        case "$SOURCE" in github|gitee|local) : ;; *) err "--source 只能是 github / gitee / local"; return 1 ;; esac
    fi
    if [ -n "$REGISTRY" ]; then
        case "$REGISTRY" in ghcr|swr|local) : ;; *) err "--registry 只能是 ghcr / swr / local"; return 1 ;; esac
    fi
    [ -n "$SRC_DIR" ] || { err "--dir 不能为空"; return 1; }
    [ -n "$ENV_FILE" ] || { err "--env-file 不能为空"; return 1; }
    [ -n "$PROJECT" ] || { err "--project 不能为空"; return 1; }
    return 0
}

# ---------- 1. Docker 检测与引导 ----------
read_os_release() {
    OS_ID=""
    OS_LIKE=""
    if [ -r /etc/os-release ]; then
        OS_ID=$(sed -n 's/^ID=//p' /etc/os-release | tr -d '"' | head -n 1)
        OS_LIKE=$(sed -n 's/^ID_LIKE=//p' /etc/os-release | tr -d '"' | head -n 1)
    fi
    printf '%s %s\n' "$OS_ID" "$OS_LIKE"
}

docker_family() {
    line=$(read_os_release)
    id=${line%% *}
    like=${line#* }
    case "$id" in
        debian|ubuntu) printf 'debian'; return ;;
    esac
    case "$id" in
        rhel|centos|fedora|rocky|almalinux|openeuler|uos|kylin|anolis|cclinux|tencentos) printf 'rhel'; return ;;
    esac
    case "$like" in
        *debian*|*ubuntu*) printf 'debian'; return ;;
        *rhel*|*fedora*|*centos*) printf 'rhel'; return ;;
    esac
    printf 'unknown'
}

install_docker_linux() {
    family=$(docker_family)
    info "未检测到 Docker（docker 命令不存在）。"
    info "将按发行版（$(read_os_release)）给出安装命令；安装需要 root 权限。"
    printf '\n[离线/手工安装参考]\n'
    printf '  Debian/Ubuntu：curl -fsSL https://get.docker.com | sh\n'
    printf '  RHEL/CentOS/UOS/Kylin：dnf install -y docker（或 yum install -y docker）\n'
    printf '  官网文档：https://docs.docker.com/engine/install/\n\n'

    case "$family" in
        debian)
            info "计划执行的命令："
            printf '  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh\n'
            printf '  sh /tmp/get-docker.sh\n'
            printf '  systemctl enable --now docker\n'
            printf '  usermod -aG docker %s\n' "$(id -un)"
            ;;
        rhel)
            info "计划执行的命令（优先发行版自带 docker 包）："
            printf '  dnf install -y docker || yum install -y docker\n'
            printf '  dnf install -y docker-compose-plugin || yum install -y docker-compose-plugin\n'
            printf '  systemctl enable --now docker\n'
            printf '  usermod -aG docker %s\n' "$(id -un)"
            ;;
        *)
            err "无法识别发行版，请按 https://docs.docker.com/engine/install/ 手工安装后重跑本脚本。"
            exit 1
            ;;
    esac

    if is_dry; then
        printf '\n[dry-run] 不执行安装，继续演示后续步骤。\n'
        DOCKER_STATE="absent"
        return 0
    fi

    if ! confirm "确认按上述命令安装 Docker 吗？"; then
        err "用户取消安装。安装 Docker 后重新运行本脚本即可。"
        exit 1
    fi

    case "$family" in
        debian)
            run_shell "curl -fsSL https://get.docker.com -o /tmp/get-docker.sh" || { err "下载安装脚本失败"; exit 1; }
            run_root sh /tmp/get-docker.sh || { err "Docker 安装脚本执行失败"; exit 1; }
            ;;
        rhel)
            if ! run_root dnf install -y docker; then
                warn "dnf 安装失败，尝试 yum。"
                run_root yum install -y docker || { err "Docker 安装失败，请手工安装后重跑。"; exit 1; }
            fi
            run_root dnf install -y docker-compose-plugin || warn "docker-compose-plugin 安装失败，稍后单独检查 compose。"
            ;;
    esac

    run_root systemctl enable --now docker || warn "systemctl enable --now docker 失败，请手工启动。"
    run_root usermod -aG docker "$(id -un)" || warn "加入 docker 组失败，可能仍需要 sudo 运行 docker。"

    info "Docker 已安装。请注销并重新登录（或重新打开终端）使 docker 组生效，然后重新运行本脚本。"
    exit 0
}

ensure_docker() {
    step "1/5 检测 Docker 环境"
    if ! command -v docker >/dev/null 2>&1; then
        DOCKER_STATE="absent"
    elif docker info >/dev/null 2>&1; then
        DOCKER_STATE="ready"
    else
        DOCKER_STATE="stopped"
    fi

    case "$DOCKER_STATE" in
        ready)
            info "Docker 已安装，守护进程可用。"
            ;;
        stopped)
            warn "检测到 docker 命令，但 docker info 失败（已安装但未启动，或当前用户无权限）。"
            if command -v systemctl >/dev/null 2>&1; then
                if confirm "是否执行 systemctl enable --now docker 启动 Docker？"; then
                    run_root systemctl enable --now docker || true
                fi
            fi
            if is_dry; then
                printf '  [dry-run] systemctl enable --now docker（如未启动）\n'
            elif ! docker info >/dev/null 2>&1; then
                warn "Docker 仍不可用：请确认已启动，且当前用户已加入 docker 组（注销后重新登录）。"
                err "Docker 不可用，安装中止。"
                exit 1
            fi
            ;;
        absent)
            install_docker_linux
            ;;
    esac

    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        info "Docker Compose v2 可用：$(docker compose version 2>/dev/null | head -n 1)"
        return 0
    fi
    warn "未检测到 Docker Compose v2 插件（docker compose version 失败）。"
    info "安装参考：dnf/apt install docker-compose-plugin，或 https://docs.docker.com/compose/install/"
    if is_dry; then
        printf '  [dry-run] docker compose version 校验（缺失时提示安装）\n'
        return 0
    fi
    if confirm "是否尝试自动安装 docker-compose-plugin？"; then
        if command -v dnf >/dev/null 2>&1; then
            run_root dnf install -y docker-compose-plugin || true
        elif command -v yum >/dev/null 2>&1; then
            run_root yum install -y docker-compose-plugin || true
        elif command -v apt-get >/dev/null 2>&1; then
            run_root apt-get update && run_root apt-get install -y docker-compose-plugin || true
        fi
    fi
    if ! docker compose version >/dev/null 2>&1; then
        err "缺少 Docker Compose v2，无法继续。请安装后重跑。"
        exit 1
    fi
}

# ---------- 2. 源码来源 ----------
ENV_REL="deploy/docker-dist/$ENV_FILE"

backup_env_file() {
    ep="$SRC_DIR/$ENV_REL"
    [ -f "$ep" ] || return 0
    ts=$(date +%Y%m%d-%H%M%S)
    cp -p "$ep" "$ep.bak-$ts" && info "已备份环境文件：$ep.bak-$ts"
}

source_is_dirty() {
    [ -d "$SRC_DIR/.git" ] || return 1
    # 只看已跟踪文件的改动；drill.env 等安装期生成文件是 untracked，不算“脏”
    [ -n "$(git -C "$SRC_DIR" status --porcelain --untracked-files=no 2>/dev/null | head -n 1)" ]
}

clone_repo() {
    url="$1"
    dest="$2"
    if is_dry; then
        printf '  [dry-run] git clone --depth 1 --branch %s %s %s\n' "$REF" "$url" "$dest"
        return 0
    fi
    run git clone --depth 1 --branch "$REF" "$url" "$dest"
}

download_archive() {
    url="$1"
    out="$2"
    if is_dry; then
        printf '  [dry-run] 下载归档：%s -> %s\n' "$url" "$out"
        return 0
    fi
    if command -v curl >/dev/null 2>&1; then
        run curl -fL --retry 2 --connect-timeout 20 "$url" -o "$out"
    else
        run wget -O "$out" "$url"
    fi
}

extract_archive() {
    tar_file="$1"
    dest="$2"
    if is_dry; then
        printf '  [dry-run] tar -xzf %s 并移动到 %s\n' "$tar_file" "$dest"
        return 0
    fi
    tmpd=$(mktemp -d 2>/dev/null || echo "/tmp/drill-extract-$$")
    mkdir -p "$tmpd"
    tar -xzf "$tar_file" -C "$tmpd" || return 1
    first=$(ls -1 "$tmpd" | head -n 1)
    mkdir -p "$(dirname "$dest")"
    mv "$tmpd/$first" "$dest" || return 1
    rm -rf "$tmpd"
    return 0
}

# 首次下载（目录不存在时）
download_fresh() {
    kind="$1"
    case "$kind" in
        github) SOURCE_URL="$GITHUB_REPO" ;;
        gitee)  SOURCE_URL="$GITEE_REPO" ;;
    esac

    if command -v git >/dev/null 2>&1; then
        info "使用 git 浅克隆分支 $REF（版本 $VERSION）：$SOURCE_URL"
        if clone_repo "$SOURCE_URL" "$SRC_DIR"; then
            return 0
        fi
        warn "git clone 失败，回退为归档下载。"
    else
        warn "未找到 git，回退为归档下载。"
    fi

    archive="/tmp/cyber-drill-${VERSION}-$$.tar.gz"
    if [ "$kind" = "gitee" ]; then
        if download_archive "$GITEE_ARCHIVE" "$archive"; then
            if tar -tzf "$archive" >/dev/null 2>&1; then
                extract_archive "$archive" "$SRC_DIR" || return 1
                rm -f "$archive"
                return 0
            fi
            warn "Gitee 归档不可用（可能要求登录），回退 GitHub 归档。"
        fi
        download_archive "$GITHUB_ARCHIVE" "$archive" || { err "归档下载失败"; return 1; }
    else
        download_archive "$GITHUB_ARCHIVE" "$archive" || { err "归档下载失败"; return 1; }
    fi
    extract_archive "$archive" "$SRC_DIR" || { err "解压失败：$archive"; return 1; }
    rm -f "$archive"
    return 0
}

# 已有目录：比较本地 HEAD 与远端目标 ref，一致跳过，不一致增量更新
update_existing() {
    kind="$1"

    if [ "$UPDATE_POLICY" = "never" ]; then
        info "--no-update：跳过联网检查，使用本地源码 $SRC_DIR"
        return 0
    fi

    if [ ! -d "$SRC_DIR/.git" ]; then
        warn "目录不是 git 仓库：$SRC_DIR（默认不覆盖）"
        if [ "$FORCE_RESET" = "1" ]; then
            replace_non_git "$kind"
            return $?
        fi
        if [ "$ASSUME_YES" = "1" ]; then
            info "非交互模式：保留本地目录，不覆盖（如需强更请加 --force-reset）"
            return 0
        fi
        if ! confirm "是否备份并强制重新下载源码目录？"; then
            info "已保留本地目录：$SRC_DIR"
            return 0
        fi
        replace_non_git "$kind"
        return $?
    fi

    info "检查远端更新：git -C $SRC_DIR fetch origin $REF"
    if ! git -C "$SRC_DIR" fetch --quiet origin "$REF"; then
        warn "git fetch 失败（网络不通？），继续使用本地源码"
        return 0
    fi

    local_head=$(git -C "$SRC_DIR" rev-parse HEAD 2>/dev/null)
    remote_head=$(git -C "$SRC_DIR" rev-parse FETCH_HEAD 2>/dev/null)
    short_local=$(git -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null)

    if [ -n "$local_head" ] && [ "$local_head" = "$remote_head" ]; then
        info "源码已是最新（$short_local），跳过下载"
        return 0
    fi

    info "检测到远端有更新：本地 ${short_local} → 远端 $(printf '%s' "$remote_head" | cut -c1-8)"
    if source_is_dirty; then
        warn "本地有未提交改动（默认不覆盖）"
        if [ "$FORCE_RESET" != "1" ]; then
            if [ "$ASSUME_YES" = "1" ]; then
                warn "非交互模式：保留本地改动，跳过本次更新（如需强更请加 --force-reset）"
                return 0
            fi
            if ! confirm "是否丢弃本地已跟踪文件的改动并更新到 $REF？"; then
                info "已保留本地源码与改动"
                return 0
            fi
        fi
    fi

    backup_env_file
    if git -C "$SRC_DIR" reset --hard origin/"$REF" >/dev/null 2>&1; then
        :
    elif git -C "$SRC_DIR" reset --hard FETCH_HEAD >/dev/null 2>&1; then
        :
    else
        err "增量更新失败，保留原有源码（未改动）"
        return 1
    fi
    new_short=$(git -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null)
    info "源码已增量更新：$short_local → $new_short（drill.env 与数据卷均保留）"
    return 0
}

# 非 git 目录强更：先整体备份，下载成功后再删除备份
replace_non_git() {
    kind="$1"
    ts=$(date +%Y%m%d-%H%M%S)
    bk="${SRC_DIR}.bak-$ts"
    info "备份原目录：$bk"
    mv "$SRC_DIR" "$bk" || { err "备份原目录失败，已中止"; return 1; }
    if ! download_fresh "$kind"; then
        err "重新下载失败，恢复原目录"
        rm -rf "$SRC_DIR"
        mv "$bk" "$SRC_DIR"
        return 1
    fi
    # 恢复旧 drill.env 与备份文件
    old_env="$bk/$ENV_REL"
    if [ -f "$old_env" ]; then
        mkdir -p "$(dirname "$SRC_DIR/$ENV_REL")"
        cp -p "$old_env" "$SRC_DIR/$ENV_REL"
        for b in "$bk/$ENV_REL".bak-*; do
            [ -f "$b" ] && cp -p "$b" "$(dirname "$SRC_DIR/$ENV_REL")/"
        done
        info "已恢复原 drill.env（并保留备份文件）"
    fi
    rm -rf "$bk"
    info "源码已强制重新下载：$SRC_DIR"
    return 0
}

fetch_source() {
    kind="$1"
    if is_dry; then
        if [ -d "$SRC_DIR/deploy/docker-dist" ]; then
            if [ "$UPDATE_POLICY" = "never" ]; then
                printf '  [dry-run] --no-update：跳过 git fetch，直接使用 %s\n' "$SRC_DIR"
                return 0
            fi
            if [ ! -d "$SRC_DIR/.git" ]; then
                printf '  [dry-run] 检测到非 git 目录：默认不覆盖；加 --force-reset 将“备份目录 + 重新下载 + 恢复 drill.env”\n'
                return 0
            fi
            printf '  [dry-run] git -C %s fetch origin %s\n' "$SRC_DIR" "$REF"
            local_short=$(git -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null)
            remote_peek=$(git -C "$SRC_DIR" ls-remote origin "$REF" 2>/dev/null | awk '{print $1}' | head -n1)
            if [ -n "$local_short" ] && [ -n "$remote_peek" ]; then
                local_full=$(git -C "$SRC_DIR" rev-parse HEAD 2>/dev/null)
                if [ "$local_full" = "$remote_peek" ]; then
                    printf '  [dry-run] 判定：源码已是最新（%s），将跳过下载\n' "$local_short"
                else
                    printf '  [dry-run] 判定：检测到更新（%s -> %s），将备份 drill.env 后 git reset --hard origin/%s\n' "$local_short" "$(printf '%s' "$remote_peek" | cut -c1-8)" "$REF"
                fi
            else
                printf '  [dry-run] 判定：git fetch 后比较 HEAD 与 origin/%s；一致跳过，不一致备份 drill.env 后增量更新\n' "$REF"
            fi
            if source_is_dirty; then
                printf '  [dry-run] 检测到未提交改动：默认不覆盖，需 --force-reset 才强更\n'
            fi
            return 0
        fi
        printf '  [dry-run] 校验/下载源码到 %s（来源 %s）\n' "$SRC_DIR" "$kind"
        printf '  [dry-run] git clone --depth 1 --branch %s <repo> %s（失败则下载归档）\n' "$REF" "$SRC_DIR"
        return 0
    fi

    if [ -d "$SRC_DIR/deploy/docker-dist" ]; then
        info "检测到已有源码目录：$SRC_DIR"
        update_existing "$kind"
        return $?
    fi
    if [ -e "$SRC_DIR" ] && [ ! -d "$SRC_DIR" ]; then
        err "目标路径已存在且不是目录：$SRC_DIR"
        return 1
    fi

    download_fresh "$kind"
}

ensure_source() {
    step "2/5 选择源码来源"
    if [ -z "$SOURCE" ]; then
        if [ "$ASSUME_YES" = "1" ]; then
            SOURCE="github"
        else
            c=$(choose "请选择源码来源：" "GitHub（境外）" "Gitee（国内）" "已有本地目录 / 离线包" "1")
            case "$c" in
                1) SOURCE="github" ;;
                2) SOURCE="gitee" ;;
                3) SOURCE="local" ;;
                *) err "无效选择：$c"; exit 1 ;;
            esac
        fi
    fi

    case "$SOURCE" in
        github|gitee)
            info "源码来源：$SOURCE，目录：$SRC_DIR"
            fetch_source "$SOURCE" || exit 1
            ;;
        local)
            info "源码来源：本地目录 $SRC_DIR（沿用，不覆盖）"
            if [ -n "$BUNDLE" ]; then
                info "离线包：$BUNDLE（校验已发布 SHA256）"
                if is_dry; then
                    printf '  [dry-run] 校验 SHA256 期望 %s\n' "$BUNDLE_SHA256"
                else
                    [ -f "$BUNDLE" ] || { err "离线包不存在：$BUNDLE"; exit 1; }
                    actual=$(sha256sum "$BUNDLE" | awk '{print toupper($1)}')
                    if [ "$actual" != "$BUNDLE_SHA256" ]; then
                        err "离线包 SHA256 不匹配：$actual（期望 $BUNDLE_SHA256），已中止。"
                        exit 1
                    fi
                    info "SHA256 校验通过。"
                    if ! tar -tzf "$BUNDLE" 2>/dev/null | grep -q 'deploy/docker-dist/docker-compose.yml'; then
                        err "该离线包不含 deploy/docker-dist（面向 systemd 安装），请参考 docs/install.md 离线包章节。"
                        exit 1
                    fi
                    extract_archive "$BUNDLE" "$SRC_DIR" || { err "离线包解压失败"; exit 1; }
                fi
            fi
            if is_dry; then
                printf '  [dry-run] 检查 %s/deploy/docker-dist/docker-compose.yml\n' "$SRC_DIR"
            elif [ ! -f "$SRC_DIR/deploy/docker-dist/docker-compose.yml" ]; then
                err "本地目录中没有 deploy/docker-dist/docker-compose.yml：$SRC_DIR"
                err "请指向仓库目录或解压后的源码包（或用 --bundle 指定离线包）。"
                exit 1
            fi
            ;;
    esac
}

# ---------- 3. 镜像来源 ----------
swr_image() {
    printf '%s/%s/cyber-drill:%s' "$SWR_HOST" "$SWR_ORG" "$VERSION"
}

probe_ghcr() {
    if is_dry; then
        printf '  [dry-run] 匿名探测 GHCR manifest（期望 200，公开仓库）\n'
        return 0
    fi
    token=$(curl -fsS --connect-timeout 15 "https://ghcr.io/token?service=ghcr.io&scope=repository:chenjinxiang1980/cyber-drill:pull" 2>/dev/null | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
    if [ -z "$token" ]; then
        warn "GHCR 匿名 token 获取失败（可能网络不通）。"
        return 1
    fi
    code=$(curl -s -o /dev/null -w '%{http_code}' --connect-timeout 15 \
        -H "Authorization: Bearer $token" \
        -H "Accept: application/vnd.oci.image.manifest.v1+json,application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.v2+json" \
        "https://ghcr.io/v2/chenjinxiang1980/cyber-drill/manifests/${VERSION}")
    info "GHCR 匿名 manifest 状态码：$code"
    [ "$code" = "200" ]
}

probe_swr() {
    if is_dry; then
        printf '  [dry-run] 匿名探测 SWR manifest（含匿名 token 协商）：https://%s/v2/%s/cyber-drill/manifests/%s\n' "$SWR_HOST" "$SWR_ORG" "$VERSION"
        return 0
    fi
    accept="application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json"
    hdr=$(mktemp 2>/dev/null || echo "/tmp/drill-swr-hdr-$$")
    code=$(curl -sS -D "$hdr" -o /dev/null -w '%{http_code}' --connect-timeout 15 --max-time 30 \
        -H "Accept: $accept" \
        "https://${SWR_HOST}/v2/${SWR_ORG}/cyber-drill/manifests/${VERSION}" 2>/dev/null)
    rc=$?
    SWR_PROBE_CODE="$code"
    if [ "$rc" -ne 0 ] || [ "$code" = "000" ]; then
        err "无法连接 SWR（网络不通或 DNS 失败）：https://$SWR_HOST"
        rm -f "$hdr"
        return 2
    fi
    case "$code" in
        200)
            info "SWR 匿名 manifest 返回 200：仓库已公开，可匿名拉取，无需 AK。"
            rm -f "$hdr"
            return 0
            ;;
        404)
            err "SWR 返回 404：镜像 tag ${VERSION} 不存在。"
            rm -f "$hdr"
            return 3
            ;;
        401|403)
            # 华为 SWR 对公开仓库也会先返回 401 + Bearer challenge，
            # 需用 realm 换取匿名 token 后再判定是否真的可匿名拉取。
            challenge=$(awk 'tolower($1)=="www-authenticate:"{sub(/^[^:]*:[ ]*/,""); print; exit}' "$hdr" 2>/dev/null)
            realm=$(printf '%s' "$challenge" | sed -n 's/.*realm="\([^"]*\)".*/\1/p')
            service=$(printf '%s' "$challenge" | sed -n 's/.*service="\([^"]*\)".*/\1/p')
            scope=$(printf '%s' "$challenge" | sed -n 's/.*scope="\([^"]*\)".*/\1/p')
            if [ -n "$realm" ] && [ -n "$scope" ]; then
                sep='?'
                case "$realm" in *\?*) sep='&' ;; esac
                tok_json=$(curl -sS --connect-timeout 15 --max-time 30 \
                    "${realm}${sep}service=${service}&scope=${scope}" 2>/dev/null)
                token=$(printf '%s' "$tok_json" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
                if [ -z "$token" ]; then
                    token=$(printf '%s' "$tok_json" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
                fi
                if [ -n "$token" ]; then
                    code2=$(curl -s -o /dev/null -w '%{http_code}' --connect-timeout 15 --max-time 30 \
                        -H "Authorization: Bearer $token" -H "Accept: $accept" \
                        "https://${SWR_HOST}/v2/${SWR_ORG}/cyber-drill/manifests/${VERSION}" 2>/dev/null)
                    SWR_PROBE_CODE="$code2"
                    if [ "$code2" = "200" ]; then
                        info "SWR 匿名 token 协商成功（manifest 200）：仓库已公开，可匿名拉取，无需 AK。"
                        rm -f "$hdr"
                        return 0
                    fi
                    if [ "$code2" = "404" ]; then
                        err "SWR 返回 404：镜像 tag ${VERSION} 不存在。"
                        rm -f "$hdr"
                        return 3
                    fi
                fi
            fi
            warn "SWR 匿名鉴权未通过（HTTP $code）：仓库未公开或需要私有凭据（AK）。"
            rm -f "$hdr"
            return 1
            ;;
        *)
            err "SWR 返回意外状态码 $code。"
            rm -f "$hdr"
            return 1
            ;;
    esac
}

swr_login() {
    printf '\nSWR 私有仓库需要登录（AK）。请按提示输入，口令不会出现在命令行或日志里。\n'
    printf '区域 endpoint [%s]: ' "$SWR_HOST_DEFAULT"
    read -r v || v=""
    [ -n "$v" ] && SWR_HOST="$v"
    printf '组织名 [%s]: ' "$SWR_ORG_DEFAULT"
    read -r v || v=""
    [ -n "$v" ] && SWR_ORG="$v"
    printf 'AK 用户名（如 cn-east-3@XXXX）: '
    read -r SWR_AK || SWR_AK=""
    if [ -z "$SWR_AK" ]; then
        err "AK 用户名不能为空。"
        return 1
    fi
    if [ -n "${SWR_PASS:-}" ]; then
        swr_pass="$SWR_PASS"
        info "检测到 SWR_PASS 环境变量，使用其作为登录口令。"
    else
        printf '登录口令（输入不回显）: '
        read -r -s swr_pass || swr_pass=""
        printf '\n'
    fi
    if [ -z "$swr_pass" ]; then
        err "登录口令为空，无法登录。"
        return 1
    fi
    if is_dry; then
        printf '  [dry-run] printf %%s | docker login %s -u %s --password-stdin\n' "$SWR_HOST" "$SWR_AK"
        return 0
    fi
    if printf '%s' "$swr_pass" | docker login "$SWR_HOST" -u "$SWR_AK" --password-stdin; then
        return 0
    fi
    err "docker login 失败（401/403 表示 AK 或权限不对；网络超时表示连接不到 $SWR_HOST）。"
    return 1
}

pull_image() {
    image="$1"
    if is_dry; then
        printf '  [dry-run] docker pull %s\n' "$image"
        printf '  [dry-run] docker tag %s drill-server:%s\n' "$image" "$VERSION"
        return 0
    fi
    if ! run docker pull "$image"; then
        err "镜像拉取失败：$image"
        err "排查顺序：① 网络是否可达 registry ② 是否需要登录 ③ tag 是否存在"
        return 1
    fi
    run docker tag "$image" "drill-server:$VERSION"
    return 0
}

ensure_image() {
    step "3/5 选择镜像来源"
    if [ -z "$REGISTRY" ]; then
        if [ "$ASSUME_YES" = "1" ]; then
            REGISTRY="ghcr"
        else
            c=$(choose "请选择镜像来源：" "GHCR（境外，公开，免登录）" "SWR 华为云（国内，公开或需 AK）" "本地已导入镜像 / 离线 tar" "1")
            case "$c" in
                1) REGISTRY="ghcr" ;;
                2) REGISTRY="swr" ;;
                3) REGISTRY="local" ;;
                *) err "无效选择：$c"; exit 1 ;;
            esac
        fi
    fi

    case "$REGISTRY" in
        ghcr)
            info "镜像来源：GHCR（ghcr.io，公开仓库，匿名可拉）"
            probe_ghcr || warn "GHCR 探测未通过，仍将尝试拉取（失败时会给出原因）。"
            pull_image "$GHCR_IMAGE" || exit 1
            ;;
        swr)
            info "镜像来源：SWR 华为云（$SWR_HOST/$SWR_ORG/cyber-drill）"
            if is_dry; then
                probe_swr
                pull_image "$(swr_image)"
            else
                probe_swr
                probe_rc=$?
                if [ "$probe_rc" -eq 3 ]; then
                    exit 1
                elif [ "$probe_rc" -eq 2 ]; then
                    warn "网络不通，可检查 /etc/docker/daemon.json 代理或企业网络策略。"
                    exit 1
                elif [ "$probe_rc" -eq 0 ]; then
                    if ! pull_image "$(swr_image)"; then
                        warn "匿名拉取失败：可能是仓库仍未公开，或当前网络受限。"
                        if ! confirm "是否输入 AK 登录后重试？"; then
                            err "已取消。若仓库已公开，请确认 tag ${VERSION} 存在后重跑。"
                            exit 1
                        fi
                        swr_login || exit 1
                        pull_image "$(swr_image)" || exit 1
                    fi
                else
                    if ! confirm "匿名拉取失败，是否输入 AK 登录后重试？"; then
                        err "已取消。若仓库已公开，请确认 tag ${VERSION} 存在后重跑。"
                        exit 1
                    fi
                    swr_login || exit 1
                    pull_image "$(swr_image)" || exit 1
                fi
            fi
            ;;
        local)
            info "镜像来源：本地镜像 / 离线 tar"
            if [ -n "$IMAGE_TAR" ]; then
                info "将从 tar 导入镜像：$IMAGE_TAR"
                run docker load -i "$IMAGE_TAR" || { err "docker load 失败"; exit 1; }
            fi
            if is_dry; then
                printf '  [dry-run] 检查本地镜像 drill-server:%s\n' "$VERSION"
            elif ! docker image inspect "drill-server:$VERSION" >/dev/null 2>&1; then
                err "本地没有 drill-server:$VERSION；请先 docker load 导入镜像或用 --image-tar 指定 tar。"
                exit 1
            fi
            ;;
    esac
}

# ---------- 4. 生成 drill.env 与部署 ----------
write_env_file() {
    template="$1"
    target="$2"

    if is_dry; then
        printf '  [dry-run] 从 %s 生成 %s（随机生成 JWT/SESSION/DB/管理员密钥）\n' "$template" "$target"
        return 0
    fi
    [ -f "$template" ] || { err "缺少环境模板：$template"; return 1; }

    jwt=$(gen_hex 32) || return 1
    sess=$(gen_hex 32) || return 1
    dbpw=$(gen_hex 16) || return 1
    rootpw=$(gen_hex 16) || return 1

    if [ -n "${ADMIN_INIT_PASSWORD:-}" ]; then
        adminpw="$ADMIN_INIT_PASSWORD"
    elif [ "$ASSUME_YES" = "1" ]; then
        adminpw=$(gen_hex 16) || return 1
    else
        printf '管理员初始密码（留空则自动生成，输入不回显）: '
        read -r -s adminpw || adminpw=""
        printf '\n'
        [ -z "$adminpw" ] && adminpw=$(gen_hex 16)
    fi
    [ -n "$adminpw" ] || return 1
    GENERATED_ADMIN_PW="$adminpw"

    cp "$template" "$target" || return 1
    sed -i \
        -e "s/__REPLACE_WITH_DB_USER__/drill/g" \
        -e "s/__REPLACE_WITH_DB_PASSWORD__/${dbpw}/g" \
        -e "s/__REPLACE_WITH_ROOT_PW__/${rootpw}/g" \
        -e "s/__REPLACE_WITH_HEX32_JWT_SECRET__/${jwt}/g" \
        -e "s/__REPLACE_WITH_HEX32_SESSION_SECRET__/${sess}/g" \
        -e "s/__REPLACE_WITH_STRONG_ADMIN_PASSWORD__/${adminpw}/g" \
        -e "s/^VERSION=.*/VERSION=${VERSION}/" \
        "$target" || return 1

    {
        printf '\n# ---------- 一键安装器追加 ----------\n'
        printf 'APP_HOST_PORT=%s\n' "$PORT"
        printf 'MYSQL_HOST_PORT=%s\n' "$MYSQL_PORT"
        printf 'DRILL_ENV_FILE=%s\n' "$ENV_FILE"
        printf 'APP_CONTAINER_NAME=%s-app\n' "$PROJECT"
        printf 'MYSQL_CONTAINER_NAME=%s-mysql\n' "$PROJECT"
        printf 'PUBLIC_BASE_URL=http://localhost:%s\n' "$PORT"
    } >> "$target"

    chmod 600 "$target" 2>/dev/null || true
    return 0
}

prepare_env() {
    step "4/5 准备环境文件"
    ENV_PATH="$SRC_DIR/deploy/docker-dist/$ENV_FILE"
    COMPOSE_FILE="$SRC_DIR/deploy/docker-dist/docker-compose.yml"
    template="$SRC_DIR/deploy/docker-dist/.env.example"

    if is_dry; then
        printf '  [dry-run] 环境文件将写入：%s\n' "$ENV_PATH"
        if [ -f "$ENV_PATH" ]; then
            info "检测到已存在环境文件，将沿用（不覆盖）。"
        else
            info "环境文件不存在，将先生成随机密钥。"
        fi
        return 0
    fi

    if [ ! -f "$COMPOSE_FILE" ]; then
        err "缺少 compose 文件：$COMPOSE_FILE"
        exit 1
    fi

    if [ -f "$ENV_PATH" ]; then
        warn "检测到已有环境文件：$ENV_PATH"
        if [ "$ASSUME_YES" = "1" ]; then
            info "沿用已有环境文件（--yes 不覆盖）。"
        else
            printf '  1) 沿用（推荐）\n  2) 备份后重建（旧文件改名为 <文件名>.bak.<时间戳>）\n  3) 退出\n'
            printf '请选择 [1]: '
            read -r c || c=""
            [ -z "$c" ] && c="1"
            case "$c" in
                1) info "沿用已有环境文件。" ;;
                2)
                    bak="${ENV_PATH}.bak.$(date +%Y%m%d%H%M%S)"
                    cp "$ENV_PATH" "$bak" && chmod 600 "$bak" || { err "备份失败"; exit 1; }
                    info "已备份到：$bak"
                    write_env_file "$template" "$ENV_PATH" || exit 1
                    ;;
                *) info "用户退出，未做任何变更。"; exit 0 ;;
            esac
        fi
    else
        write_env_file "$template" "$ENV_PATH" || { err "生成环境文件失败"; exit 1; }
        info "已生成环境文件：$ENV_PATH（权限 600）"
    fi

    file_port=$(sed -n 's/^APP_HOST_PORT=//p' "$ENV_PATH" | tail -n 1)
    if [ -n "$file_port" ]; then
        PORT="$file_port"
        info "沿用环境文件中的 APP_HOST_PORT=$PORT 做健康检查。"
    fi
}

compose_cmd() {
    DRILL_ENV_FILE="$ENV_FILE" docker compose -p "$PROJECT" \
        -f "$COMPOSE_FILE" --env-file "$ENV_PATH" "$@"
}

deploy() {
    step "5/5 启动服务"
    info "compose 项目名：$PROJECT"
    info "容器名：${PROJECT}-app / ${PROJECT}-mysql"
    info "端口：应用 $PORT，MySQL $MYSQL_PORT"

    if is_dry; then
        printf '  [dry-run] docker compose -p %s -f %s --env-file %s up -d\n' "$PROJECT" "$COMPOSE_FILE" "$ENV_PATH"
        printf '  [dry-run] 轮询 http://127.0.0.1:%s/api/health 直到 200\n' "$PORT"
        return 0
    fi

    if ! compose_cmd up -d; then
        err "docker compose up -d 失败。"
        compose_cmd logs --tail=100 || true
        err "回滚建议：docker compose -p $PROJECT -f $COMPOSE_FILE --env-file $ENV_PATH down"
        exit 1
    fi

    info "等待 /api/health 就绪（最多 180 秒）…"
    deadline=$(( $(date +%s) + 180 ))
    body=""
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if body=$(curl -fsS --max-time 3 "http://127.0.0.1:$PORT/api/health" 2>/dev/null); then
            break
        fi
        sleep 3
    done

    if [ -n "$body" ]; then
        printf '\n[成功] 服务已启动。\n'
        printf '  健康检查：%s\n' "$body"
        printf '  访问地址：http://localhost:%s\n' "$PORT"
        printf '  管理员账号：admin\n'
        if [ -n "$GENERATED_ADMIN_PW" ]; then
            printf '  管理员初始密码（只显示这一次，请立即保存）：%s\n' "$GENERATED_ADMIN_PW"
        else
            printf '  管理员密码：沿用 drill.env 中的 ADMIN_INIT_PASSWORD（未重新生成）\n'
        fi
        printf '  环境文件：%s（权限 600，务必备份）\n' "$ENV_PATH"
        printf '\n注意：首次启动会自动创建 12 个角色、8 个场景/剧本节点与管理员账号（admin / platformadmin），无需手工 bootstrap。\n'
        printf '停止服务：docker compose -p %s -f %s down（保留数据卷）\n' "$PROJECT" "$COMPOSE_FILE"
        return 0
    fi

    err "健康检查超时（180 秒内 http://127.0.0.1:$PORT/api/health 未返回 200）。"
    printf '\n最近 100 行日志：\n'
    compose_cmd logs --tail=100 || true
    printf '\n回滚建议：docker compose -p %s -f %s --env-file %s down\n' "$PROJECT" "$COMPOSE_FILE" "$ENV_PATH"
    exit 1
}

# ---------- 主流程 ----------
main() {
    printf 'Cyber-Drill %s Linux 一键安装\n' "$VERSION"
    printf '仓库：%s\n      %s\n' "$GITHUB_REPO" "$GITEE_REPO"
    if is_dry; then printf '模式：dry-run（只打印命令，不做实际变更）\n'; fi
    if [ "$ASSUME_YES" = "1" ]; then printf '模式：--yes（跳过交互确认）\n'; fi

    if [ "$ASSUME_YES" != "1" ] && [ ! -t 0 ]; then
        warn "当前输入不是交互终端（例如 curl | bash）：将使用默认选项，且不会自动安装 Docker。"
        warn "管道模式建议显式传参：bash -s -- --source gitee --registry ghcr --yes"
    fi

    validate_args || exit 1
    ensure_docker
    ensure_source
    ensure_image
    prepare_env
    deploy

    if is_dry; then
        printf '\n[dry-run] 完成。未安装 Docker、未拉取镜像、未写文件、未启动容器。\n'
    fi
}

main
