#!/usr/bin/env bash
# =============================================================================
# Drill System 一键升级工具（容器部署）
#
#   备份 → 数据库迁移 → 拉取镜像 → 重建 app → 校验；失败自动回滚。
#
# 用法：
#   sudo ./deploy/upgrade/upgrade.sh --to 1.0.0
#   sudo ./deploy/upgrade/upgrade.sh --from 0.9.0 --to 1.0.0
#   sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --dry-run
#   sudo ./deploy/upgrade/upgrade.sh --status
#
# 规格见 docs/upgrade-tool-design.md；用户手册见 docs/upgrade.md。
#
# ⚠ 破坏性操作纪律（禁止绕过）：
#   1. 任何删除目录/容器/数据卷的动作，只允许作用于
#      com.docker.compose.project.working_dir == 目标安装目录 的容器，
#      以及 com.docker.compose.project == 目标项目名 且属于本工具声明卷名的数据卷；
#      禁止按固定容器名（drill-app / drill-mysql）或固定卷名（drill-deploy_*）删除。
#   2. 回滚/清理必须使用目标 compose 文件 + 目标项目名，不得跨项目清理。
#   3. 测试升级链路必须使用独立目录 + 独立项目名，禁止复用现网目录或项目。
# =============================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=lib/00-common.sh
. "$SCRIPT_DIR/lib/00-common.sh"
# shellcheck source=lib/10-manifest.sh
. "$SCRIPT_DIR/lib/10-manifest.sh"
# shellcheck source=lib/20-backup.sh
. "$SCRIPT_DIR/lib/20-backup.sh"
# shellcheck source=lib/30-migrate.sh
. "$SCRIPT_DIR/lib/30-migrate.sh"
# shellcheck source=lib/40-compose.sh
. "$SCRIPT_DIR/lib/40-compose.sh"
# shellcheck source=lib/50-rollback.sh
. "$SCRIPT_DIR/lib/50-rollback.sh"

FROM_OPT=""
TO_OPT=""
DIR_OPT=""
MODE="upgrade"
CHAIN_LINES=""
CHAIN_TOTAL=""
CHAIN_THROUGH=""
NEED_DOWNTIME=0
RESUMING=0
RUN_TS="$(date +%Y%m%d-%H%M%S)"
RUN_ID=""
RUN_STATUS="running"
RUN_STARTED_AT=""
LAST_SEEN_VERSION=""
MANIFEST_OVERRIDE="${UPGRADE_MANIFEST:-}"
TARGET_IS_LOCAL=0

usage() {
  cat <<'EOF'
用法: upgrade.sh --to <版本> [选项]

必填（--status / --recover-last 除外）：
  --to <ver>              目标版本，例如 1.0.0

选项：
  --from <ver>            当前版本（默认自动探测：容器 /api/health → schema_migrations 行数）
  --dir <path>            部署目录（含 docker-compose.yml），默认自动查找
  --compose <file>        compose 文件，默认 <dir>/docker-compose.yml
  --env-file <file>       环境文件，默认 <dir>/.env
  --registry ghcr|swr     镜像仓库，默认 ghcr（当前 image 无仓库前缀时保持本地 tag 写法）
  --image-ref <ref>       直接指定目标镜像引用（本地构建场景）
  --offline               不访问 registry（需本地已有镜像或配合 --image-tar）
  --image-tar <file>      docker load -i 的镜像包
  --pin-digest            写 tag@sha256:…（默认跟随 compose 原写法）
  --dry-run               只检查与打印计划，不做任何变更
  --yes                   跳过交互确认
  --skip-backup           复用已有同版本备份（必须与 --yes 同时使用）
  --fast                  跳过备份的恢复演练校验
  --keep-backups <n>      保留最近 n 份成功备份，默认 3；0 = 不清理
  --timeout <sec>         健康检查等待上限，默认 180
  --log-dir <path>        日志目录，默认 <dir>/logs
  --allow-internal        允许目标为 channel=internal 的版本
  --force-chain           链不完整时强行继续（需 --yes，日志留痕）
  --status                只打印当前状态
  --recover-last          自动回滚失败后的人工补救入口
  -h, --help              显示本帮助

退出码：0 成功 / 10 前置检查 / 11 版本链或库状态 / 12 磁盘 / 13 备份 /
        14 迁移或重建 / 15 健康检查 / 16 版本号不符 / 20 回滚失败 /
        30 锁冲突 / 40 镜像缺失或 digest 不符
EOF
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --to) TO_OPT="${2:-}"; shift 2 ;;
      --from) FROM_OPT="${2:-}"; shift 2 ;;
      --dir) DIR_OPT="${2:-}"; shift 2 ;;
      --compose) COMPOSE_FILE="${2:-}"; shift 2 ;;
      --env-file) ENV_FILE="${2:-}"; shift 2 ;;
      --registry) REGISTRY="${2:-}"; REGISTRY_GIVEN=1; shift 2 ;;
      --image-ref) IMAGE_REF_OPT="${2:-}"; shift 2 ;;
      --offline) OFFLINE=1; shift ;;
      --image-tar) IMAGE_TAR="${2:-}"; shift 2 ;;
      --pin-digest) PIN_DIGEST=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --yes) ASSUME_YES=1; shift ;;
      --skip-backup) SKIP_BACKUP=1; shift ;;
      --fast) FAST_MODE=1; shift ;;
      --keep-backups) KEEP_BACKUPS="${2:-3}"; shift 2 ;;
      --timeout) HEALTH_TIMEOUT="${2:-180}"; shift 2 ;;
      --log-dir) LOG_DIR="${2:-}"; shift 2 ;;
      --allow-internal) ALLOW_INTERNAL=1; shift ;;
      --force-chain) FORCE_CHAIN=1; shift ;;
      --status) MODE="status"; shift ;;
      --recover-last) MODE="recover"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) printf '未知参数：%s（--help 查看用法）\n' "$1" >&2; exit "$E_PRECHECK" ;;
    esac
  done

  if [ -n "$TO_OPT" ] && ! printf '%s' "$TO_OPT" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    printf '版本号格式非法：%s（应为 x.y.z）\n' "$TO_OPT" >&2
    exit "$E_PRECHECK"
  fi
  if [ -n "$FROM_OPT" ] && ! printf '%s' "$FROM_OPT" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    printf '版本号格式非法：%s（应为 x.y.z）\n' "$FROM_OPT" >&2
    exit "$E_PRECHECK"
  fi
  if [ -n "$IMAGE_REF_OPT" ] && ! printf '%s' "$IMAGE_REF_OPT" | grep -Eq '^[a-zA-Z0-9._/:@-]+$'; then
    printf '镜像引用含非法字符：%s\n' "$IMAGE_REF_OPT" >&2
    exit "$E_PRECHECK"
  fi
  if [ "$MODE" = "upgrade" ] && [ -z "$TO_OPT" ]; then
    printf '缺少 --to <版本>（--help 查看用法）\n' >&2
    exit "$E_PRECHECK"
  fi
  if [ "$MODE" = "upgrade" ]; then
    TO_VER="$TO_OPT"
  fi
  if [ "$SKIP_BACKUP" = 1 ] && [ "$ASSUME_YES" != 1 ]; then
    printf '--skip-backup 必须与 --yes 同时使用\n' >&2
    exit "$E_PRECHECK"
  fi
  if [ "$FORCE_CHAIN" = 1 ] && [ "$ASSUME_YES" != 1 ]; then
    printf '--force-chain 必须与 --yes 同时使用\n' >&2
    exit "$E_PRECHECK"
  fi
}

setup_paths() {
  if [ -n "$DIR_OPT" ]; then
    DEPLOY_DIR="$(cd "$DIR_OPT" 2>/dev/null && pwd)" || {
      printf '部署目录不存在：%s\n' "$DIR_OPT" >&2; exit "$E_PRECHECK"; }
  elif [ -f "$SCRIPT_DIR/../docker-dist/docker-compose.yml" ]; then
    DEPLOY_DIR="$(cd "$SCRIPT_DIR/../docker-dist" && pwd)"
  elif [ -f "$PWD/docker-compose.yml" ]; then
    DEPLOY_DIR="$PWD"
  elif [ -f "$SCRIPT_DIR/docker-compose.yml" ]; then
    DEPLOY_DIR="$SCRIPT_DIR"
  else
    printf '找不到 docker-compose.yml，请用 --dir 指定部署目录\n' >&2
    exit "$E_PRECHECK"
  fi

  [ -n "$COMPOSE_FILE" ] || COMPOSE_FILE="$DEPLOY_DIR/docker-compose.yml"
  case "$COMPOSE_FILE" in /*) ;; *) COMPOSE_FILE="$DEPLOY_DIR/$COMPOSE_FILE" ;; esac
  [ -n "$ENV_FILE" ] || ENV_FILE="$DEPLOY_DIR/.env"
  case "$ENV_FILE" in /*) ;; *) ENV_FILE="$DEPLOY_DIR/$ENV_FILE" ;; esac
  [ -n "$LOG_DIR" ] || LOG_DIR="$DEPLOY_DIR/logs"
}

setup_log() {
  mkdir -p "$LOG_DIR" 2>/dev/null || {
    printf '无法创建日志目录：%s\n' "$LOG_DIR" >&2; exit "$E_PRECHECK"; }
  LOG_FILE="$LOG_DIR/upgrade-${FROM_VER:-auto}-to-${TO_VER:-none}-${RUN_TS}.log"
  : >>"$LOG_FILE" || { printf '无法写日志：%s\n' "$LOG_FILE" >&2; exit "$E_PRECHECK"; }
  chmod 600 "$LOG_FILE" 2>/dev/null || true
}

# ---------------- 版本与链 ----------------
resolve_from() {
  local v n
  if [ -n "$FROM_OPT" ]; then
    FROM_VER="$FROM_OPT"
    info "当前版本（--from 指定）：$FROM_VER"
    return 0
  fi
  if [ -n "$APP_C" ] && v="$(health_once "$APP_C" "/api/health")" && [ -n "$v" ]; then
    FROM_VER="$v"
    info "当前版本（容器 /api/health）：$FROM_VER"
    return 0
  fi
  n="$(schema_count)"
  v="$(mf_version_by_total "$n")"
  if [ -n "$v" ]; then
    FROM_VER="$v"
    warn "容器不可读，改由 schema_migrations 行数（$n）推断当前版本：$FROM_VER"
    return 0
  fi
  fail "$E_CHAIN" "P0-precheck" "探测当前版本" \
    "无法判定当前版本（容器 health 不可用，且 schema_migrations 行数 $n 与清单任何版本都不匹配）" \
    "手工指定 --from <当前版本> 后重跑"
}

build_chain() {
  local errf="$RUN_DIR/.chain.err"
  mkdir -p "$RUN_DIR"
  if ! CHAIN_LINES="$(mf_chain "$FROM_VER" "$TO_VER" 2>"$errf")"; then
    cat "$errf" >>"$LOG_FILE"
    cat "$errf" >&2
    fail "$E_CHAIN" "P0-precheck" "版本链校验 $FROM_VER → $TO_VER" "版本链不完整或清单缺版本（详见上方说明）" "补齐清单/工具后重跑；不要用 --force-chain 绕过未知风险"
  fi
  local last_line
  last_line="$(printf '%s\n' "$CHAIN_LINES" | tail -1)"
  CHAIN_TOTAL="${last_line#*|}"; CHAIN_TOTAL="${CHAIN_TOTAL%%|*}"
  CHAIN_THROUGH="$(printf '%s\n' "$last_line" | cut -d'|' -f3)"
  if chain_requires_downtime; then NEED_DOWNTIME=1; fi
}

run_actions() {
  local line ver typ key text
  while IFS='|' read -r ver _rest; do
    [ -n "$ver" ] || continue
    while IFS=$'\t' read -r typ key text; do
      [ -n "$typ" ] || continue
      case "$typ" in
        env_require)
          if [ -z "$(env_get "$key")" ]; then
            fail "$E_PRECHECK" "P0-precheck" "$ver.actions env_require $key" \
              "目标版本链要求环境变量 $key，但 $ENV_FILE 里没有" "补齐 $key 后重跑"
          fi
          ;;
        warn) warn "[$ver] $text" ;;
      esac
    done < <(python3 - "$MANIFEST" "$ver" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
r=[x for x in d["releases"] if x["version"]==sys.argv[2]]
if not r: sys.exit(0)
for a in (r[0].get("actions") or []):
    t=a.get("type","")
    if t=="env_require":
        print("%s\t%s\t%s" % (t, a.get("key",""), a.get("text","")))
    elif t=="warn":
        print("%s\t\t%s" % (t, a.get("text","")))
PY
)
  done <<<"$CHAIN_LINES"
}

# ---------------- 状态目录 ----------------
select_run_dir() {
  local runs="$DEPLOY_DIR/.upgrade-runs" d st f t
  mkdir -p "$runs"
  for d in $(ls -1d "$runs"/*/ 2>/dev/null | sort -r); do
    [ -d "$d" ] || continue
    [ -f "$d/STATE.json" ] || continue
    t="$(state_get_file "$d/STATE.json" to)"
    [ "$t" = "$TO_VER" ] || continue
    st="$(state_get_file "$d/STATE.json" status)"
    case "$st" in
      running|failed|rolling_back)
        # 没有备份就说明还没动过数据（P0/P1 就失败了），按新升级处理；回滚中的记录例外
        if [ "$st" != "rolling_back" ] && [ -z "$(state_get_file "$d/STATE.json" artifacts.backup_dir)" ]; then
          continue
        fi
        f="$(state_get_file "$d/STATE.json" from)"
        [ -n "$f" ] && FROM_VER="$f"
        RUN_DIR="$d"; RESUMING=1; RESUMING_STATUS="$st"
        info "检测到未完成的升级记录，将从断点继续：$RUN_DIR（$FROM_VER → $TO_VER，状态 $st）"
        return 0
        ;;
    esac
  done
  return 1
}

new_run_dir() {
  local runs="$DEPLOY_DIR/.upgrade-runs"
  mkdir -p "$runs"
  RUN_DIR="$runs/${FROM_VER}-to-${TO_VER}-${RUN_TS}"
  mkdir -p "$RUN_DIR"
  chmod 700 "$RUN_DIR"
}

state_get_file() { # <file> <dot.path>
  [ -f "$1" ] || return 0
  python3 -c 'import json,sys
try:
    cur=json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
for p in sys.argv[2].split("."):
    if isinstance(cur,dict) and p in cur: cur=cur[p]
    else: sys.exit(0)
print("" if cur is None else cur)' "$1" "$2"
}

load_state() {
  [ "$RESUMING" = 1 ] || return 0
  BACKUP_DIR="$(state_get_file "$RUN_DIR/STATE.json" artifacts.backup_dir)"
  DUMP_ROWS="$(state_get_file "$RUN_DIR/STATE.json" artifacts.dump_db_rows)"
  SCHEMA_BEFORE="$(state_get_file "$RUN_DIR/STATE.json" artifacts.schema_before)"
  CURRENT_REF="$(state_get_file "$RUN_DIR/STATE.json" artifacts.old_image_ref)"
  OLD_IMAGE_ID="$(state_get_file "$RUN_DIR/STATE.json" artifacts.old_image_id)"
  APP_STOPPED="$(state_get_file "$RUN_DIR/STATE.json" artifacts.app_stopped)"
  COMPOSE_CHANGED="$(state_get_file "$RUN_DIR/STATE.json" artifacts.compose_changed)"
  RUN_STARTED_AT="$(state_get_file "$RUN_DIR/STATE.json" started_at)"
  RUN_EXIT_CODE="$(state_get_file "$RUN_DIR/STATE.json" exit_code)"
  local dp
  dp="$(state_get_file "$RUN_DIR/STATE.json" done)"
  if [ -n "$dp" ] && [ "$dp" != "[]" ]; then
    dp="${dp#[}"; dp="${dp%]}"; dp="${dp//\"/}"; dp="${dp//\'/}"
    IFS=',' read -ra DONE_PHASES <<<"$dp"
  fi
}

# ---------------- P0 ----------------
detect_project() {
  local p="" c
  c="$(docker ps -aq --filter "label=com.docker.compose.service=app" \
        --filter "label=com.docker.compose.project.working_dir=$DEPLOY_DIR" 2>/dev/null | head -1)"
  [ -n "$c" ] && p="$(docker inspect "$c" --format '{{index .Config.Labels "com.docker.compose.project"}}' 2>/dev/null)"
  [ -n "$p" ] || p="$(env_get COMPOSE_PROJECT_NAME)"
  [ -n "$p" ] || p="$(basename "$DEPLOY_DIR")"
  PROJECT="$p"
}

pick_containers() {
  APP_C="$(app_container)"
  MYSQL_C="$(mysql_container)"
  if [ -z "$MYSQL_C" ]; then
    MYSQL_C="$(docker ps -aq --filter "label=com.docker.compose.service=mysql" \
      --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null | head -1)"
  fi
  [ -n "$MYSQL_C" ] || fail "$E_PRECHECK" "P0-precheck" "docker compose ps -q mysql" \
    "找不到 mysql 容器（compose 里必须有 mysql 服务且已创建）" "先 docker compose up -d，再重跑升级"
}

detect_network() {
  NET_NAME=""
  if [ -n "$APP_C" ]; then
    NET_NAME="$(docker inspect "$APP_C" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' 2>/dev/null | awk '{print $1}')"
  fi
  [ -n "$NET_NAME" ] || NET_NAME="${PROJECT}_drill-net"
}

check_disk() {
  local avail_kb need avail
  avail_kb="$(df -Pk "$DEPLOY_DIR" | awk 'NR==2{print $4}')"
  need="$(mf_get "$TO_VER" min_free_bytes)"
  [ -n "$need" ] || need=0
  [ "$need" = "null" ] && need=0
  local size; size="$(mf_get "$TO_VER" image.size_bytes)"
  [ -n "$size" ] || size=0
  [ "$size" = "null" ] && size=0
  avail=$((avail_kb * 1024))
  if [ "$avail" -lt "$need" ]; then
    local need_mb=$((need / 1048576)) avail_mb=$((avail / 1048576))
    fail "$E_DISK" "P0-precheck" "df $DEPLOY_DIR" \
      "可用空间 ${avail_mb} MiB，低于该版本要求的 ${need_mb} MiB" \
      "清理旧备份（$DEPLOY_DIR/backups）或旧镜像（docker image prune）后重跑"
  fi
  if [ "$size" -gt 0 ] && [ "$avail" -lt $((size * 3)) ]; then
    warn "可用空间 $((avail / 1048576)) MiB 小于镜像大小×3（$((size * 3 / 1048576)) MiB），可能不够拉取与解包"
  fi
}

print_plan() {
  local line ver total through cnt ddl compat ch be client
  info "── 升级计划 ────────────────────────────────"
  info "部署目录：$DEPLOY_DIR"
  info "compose：$COMPOSE_FILE"
  info "环境文件：$ENV_FILE"
  info "当前版本：$FROM_VER    目标版本：$TO_VER"
  info "当前镜像：$CURRENT_REF"
  info "目标镜像：$TARGET_REF"
  info "迁移链（只跑目标镜像里的迁移）："
  while IFS='|' read -r ver total through _from cnt ddl compat ch be; do
    [ -n "$ver" ] || continue
    client="stable"
    [ "$ch" = "internal" ] && client="internal"
    [ "$be" = "true" ] && client="$client,best-effort"
    info "  → $ver  迁移 +$cnt 条（累计 $total，末条 $through）DDL=$ddl 兼容旧二进制=$compat [$client]"
  done <<<"$CHAIN_LINES"
  if [ "$NEED_DOWNTIME" = 1 ]; then
    info "停机策略：链上有 DDL 或不向后兼容的版本 → 先停 app 再迁移（停机时长取决于迁移耗时）"
  else
    info "停机策略：无需提前停机（迁移向后兼容；仅在重建 app 的秒级窗口不可用）"
  fi
  info "备份：$DEPLOY_DIR/backups/upgrade-${FROM_VER}-to-${TO_VER}-<时间戳>/"
  info "日志：$LOG_DIR/"
  info "────────────────────────────────────────────"
}

precheck() {
  PHASE="P0-precheck"
  step "P0 预检"

  command -v docker >/dev/null 2>&1 || fail "$E_PRECHECK" "P0-precheck" "docker --version" "未安装 docker" "安装 Docker 后重跑"
  docker info >/dev/null 2>&1 || fail "$E_PRECHECK" "P0-precheck" "docker info" "无法连接 docker daemon" "用 root/sudo 运行，或检查 docker 服务与权限"
  local dcver
  dcver="$(docker compose version --short 2>/dev/null)" || fail "$E_PRECHECK" "P0-precheck" "docker compose version" "缺少 docker compose v2（compose 插件）" "安装 docker-compose-plugin 后重跑"
  if ! printf '2.20.0\n%s\n' "$dcver" | sort -V -C; then
    fail "$E_PRECHECK" "P0-precheck" "docker compose version --short = $dcver" "compose 版本低于 2.20" "升级 docker compose 后重跑"
  fi

  [ -f "$COMPOSE_FILE" ] || fail "$E_PRECHECK" "P0-precheck" "读取 $COMPOSE_FILE" "compose 文件不存在" "用 --dir/--compose 指定正确路径"
  [ -f "$ENV_FILE" ] || fail "$E_PRECHECK" "P0-precheck" "读取 $ENV_FILE" "环境文件不存在" "用 --env-file 指定，或先创建 .env"

  detect_project
  compose_init
  compose config -q >>"${LOG_FILE:-/dev/null}" 2>&1 || fail "$E_PRECHECK" "P0-precheck" "docker compose config -q" "compose 解析失败（语法或变量缺失）" "修正 compose/.env 后重跑"
  compose_has_service app || fail "$E_PRECHECK" "P0-precheck" "docker compose config" "compose 里没有 app 服务" "确认这是 Drill 的部署 compose"

  pick_containers
  detect_network
  if [ "$RESUMING" != 1 ] || [ -z "$CURRENT_REF" ]; then
    CURRENT_REF="$(compose_app_image)"
  fi
  [ -n "$CURRENT_REF" ] || fail "$E_PRECHECK" "P0-precheck" "读取 app.image" "app 服务没有 image" "在 compose 的 app 服务下补 image 后重跑"
  info "当前镜像（compose）：$CURRENT_REF"

  db_init
  mycnf_install "$E_PRECHECK" "P0-precheck"

  if [ "$RESUMING" = 1 ]; then
    info "断点续跑：沿用升级记录中的起始版本 $FROM_VER"
  else
    resolve_from
  fi
  mf_has "$TO_VER" || fail "$E_CHAIN" "P0-precheck" "目标版本 $TO_VER" "目标版本不在清单内" "检查版本号，或升级升级工具"
  local ch; ch="$(mf_get "$TO_VER" channel)"
  if [ "$ch" = "internal" ] && [ "$ALLOW_INTERNAL" != 1 ]; then
    fail "$E_CHAIN" "P0-precheck" "目标版本 $TO_VER 为 internal" "内部迭代版本默认不允许作为升级目标" "改用 stable 版本；确需测试时加 --allow-internal"
  fi

  if [ "$FROM_VER" = "$TO_VER" ]; then
    info "当前已是 $TO_VER，无需升级（未做任何变更）"
    mycnf_remove
    write_result "noop" "$E_OK" "P0-precheck" "already at $TO_VER"
    exit 0
  fi

  build_chain
  if [ "$(mf_get "$FROM_VER" best_effort)" = "true" ] || printf '%s' "$CHAIN_LINES" | grep -q '|true$'; then
    warn "本次升级路径包含 best-effort 版本（历史版本缺少发布时校验数据），升级后请人工抽查业务页面"
  fi
  run_actions

  local cnt f_total
  cnt="$(schema_count)"
  # 断点续跑时保留备份时刻的行数，避免回滚比较基准被覆盖
  [ "$RESUMING" = 1 ] || SCHEMA_BEFORE="$cnt"
  f_total="$(mf_get "$FROM_VER" schema_migrations_total)"
  if [ "$cnt" != "$f_total" ] && [ "$RESUMING" != 1 ]; then
    if [ "$cnt" -lt "$f_total" ]; then
      fail "$E_CHAIN" "P0-precheck" "schema_migrations=$cnt" \
        "数据库落后于当前版本（$FROM_VER 应为 $f_total 条），说明上次启动迁移没跑完" \
        "先用当前版本镜像跑一次迁移（docker compose up -d app 会自动补），确认 $f_total 条后再升级"
    else
      fail "$E_CHAIN" "P0-precheck" "schema_migrations=$cnt" \
        "数据库超前于当前版本（$FROM_VER 应为 $f_total 条），不支持降级" \
        "如需回退请恢复升级前的整库备份；不要直接降级镜像"
    fi
  fi
  info "数据库迁移记录：$cnt 条（末条 $(schema_last)）"

  check_disk
  compute_target_ref
  print_plan

  mkdir -p "$DEPLOY_DIR/backups" && chmod 700 "$DEPLOY_DIR/backups" 2>/dev/null || true
  mkdir -p "$DEPLOY_DIR/.upgrade-runs" && chmod 700 "$DEPLOY_DIR/.upgrade-runs" 2>/dev/null || true

  if [ "$DRY_RUN" = 1 ]; then
    info "--dry-run：预检通过，未做任何变更"
    write_result "dry_run" "$E_OK" "P0-precheck" "dry-run"
    mycnf_remove
    exit 0
  fi

  if [ "$ASSUME_YES" != 1 ]; then
    if [ -t 0 ]; then
      local ans
      read -r -p "确认按上述计划升级？输入 yes 继续：" ans
      [ "$ans" = "yes" ] || { info "已取消（未做任何变更）"; mycnf_remove; exit 0; }
    else
      fail "$E_PRECHECK" "P0-precheck" "交互确认" "非交互环境无法确认" "加 --yes 后重跑"
    fi
  fi

  RUN_ID="$(basename "$RUN_DIR")"
  RUN_STARTED_AT="$(ts)"
  STATE_FILE="$RUN_DIR/STATE.json"
  RESULT_FILE="$RUN_DIR/RESULT.json"
  state_write
  mark_done precheck
}

# ---------------- 各阶段 ----------------
do_backup() {
  PHASE="P1-backup"
  [ -n "$STATE_FILE" ] || { STATE_FILE="$RUN_DIR/STATE.json"; RESULT_FILE="$RUN_DIR/RESULT.json"; }
  if [ "$SKIP_BACKUP" = 1 ]; then
    reuse_backup || fail "$E_BACKUP" "P1-backup" "--skip-backup" "找不到可复用的同版本备份" "去掉 --skip-backup 重新备份"
    return 0
  fi
  if [ "$RESUMING" = 1 ] && [ -n "$BACKUP_DIR" ] && [ -f "$BACKUP_DIR/db.sql.gz" ] \
     && ( cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS >/dev/null 2>&1 ); then
    info "复用已完成并通过校验的备份：$BACKUP_DIR（不重复备份）"
    mark_done backup
    return 0
  fi

  step "P1 备份"
  backup_prepare
  if ! backup_run; then
    fail "$E_BACKUP" "P1-backup" "mysqldump 整库备份" "备份失败（未做任何变更）" "检查数据库连通性与磁盘空间；日志里有 mysqldump 原始报错"
  fi
  if ! backup_verify; then
    rm -rf -- "$BACKUP_DIR"
    fail "$E_BACKUP" "P1-backup" "备份校验（gzip/sha256/恢复演练）" "备份不可用，已删除坏备份（未做任何变更）" "检查磁盘与内存后重跑；慢机器可加 --fast 跳过恢复演练"
  fi
  mark_done backup
}

reuse_backup() {
  local d f
  for d in $(ls -1d "$DEPLOY_DIR"/backups/upgrade-*-to-* 2>/dev/null | sort -r); do
    [ -f "$d/META.json" ] || continue
    f="$(state_get_file "$d/META.json" from)"
    [ "$f" = "$FROM_VER" ] || continue
    if ( cd "$d" && sha256sum -c SHA256SUMS >/dev/null 2>&1 ); then
      BACKUP_DIR="$d"
      SCHEMA_BEFORE="$(state_get_file "$d/META.json" schema_rows)"
      DUMP_ROWS="$SCHEMA_BEFORE"
      mycnf_install "$E_BACKUP" "P1-backup"
      info "复用备份：$BACKUP_DIR"
      return 0
    fi
  done
  return 1
}

do_image() {
  PHASE="P2-image"
  step "P2 取镜像"
  [ "$NEED_DOWNTIME" = 0 ] && info "当前 app 保持运行"

  if [ -n "$IMAGE_TAR" ]; then
    [ -f "$IMAGE_TAR" ] || fail "$E_IMAGE" "P2-image" "读取 $IMAGE_TAR" "镜像包不存在" "确认 --image-tar 路径"
    local load_out loaded
    if ! load_out="$(docker load -i "$IMAGE_TAR" 2>&1)"; then
      printf '%s\n' "$load_out" >>"$LOG_FILE"
      fail "$E_IMAGE" "P2-image" "docker load -i $IMAGE_TAR" "镜像包加载失败" "确认文件完整（先比对 sha256）后重跑"
    fi
    printf '%s\n' "$load_out" >>"$LOG_FILE"
    loaded="$(printf '%s' "$load_out" | sed -n 's/^Loaded image: //p' | head -1)"
    info "已加载镜像：${loaded:-（未报告 tag）}"
    if ! docker image inspect "$TARGET_REF" >/dev/null 2>&1 && [ -n "$loaded" ]; then
      docker tag "$loaded" "$TARGET_REF" >>"$LOG_FILE" 2>&1 \
        || fail "$E_IMAGE" "P2-image" "docker tag $loaded $TARGET_REF" "无法把镜像包内的 tag 对应到目标引用" "用 --image-ref 指定镜像包里实际存在的引用"
      info "已打标签：$loaded → $TARGET_REF"
    fi
  elif [ "$TARGET_IS_LOCAL" = 1 ] && [ "$OFFLINE" != 1 ]; then
    # compose 用本地 tag（如 cyber-drill:0.9.0）：不访问 registry，要求本地已有镜像
    docker image inspect "$TARGET_REF" >/dev/null 2>&1 \
      || fail "$E_IMAGE" "P2-image" "docker image inspect $TARGET_REF" \
         "本地没有 $TARGET_REF（当前 compose 使用本地 tag 写法，工具不会去 registry 拉同名镜像）" \
         "先 docker load/构建出该镜像；或加 --registry ghcr 改用仓库镜像；或 --image-tar 指定镜像包"
    info "使用本地镜像：$TARGET_REF"
  elif [ "$OFFLINE" = 1 ]; then
    docker image inspect "$TARGET_REF" >/dev/null 2>&1 \
      || fail "$E_IMAGE" "P2-image" "docker image inspect $TARGET_REF" \
         "--offline 且本地没有目标镜像" "改用 --image-tar 导入镜像包，或去掉 --offline"
    info "使用本地镜像（--offline）：$TARGET_REF"
  else
    run_sh "docker pull $TARGET_REF" "docker pull \"$TARGET_REF\"" \
      || fail "$E_IMAGE" "P2-image" "docker pull $TARGET_REF" "镜像拉取失败（未做任何变更）" "检查网络/仓库；内网改用 --offline --image-tar"
  fi

  # digest 校验
  local dg digs
  dg="$(mf_digest "$TO_VER" "$REGISTRY")"
  if [ -n "$dg" ] && [ "$dg" != "null" ]; then
    digs="$(docker image inspect "$TARGET_REF" --format '{{json .RepoDigests}}' 2>/dev/null)"
    if printf '%s' "$digs" | grep -q "$dg"; then
      info "镜像 digest 校验通过：$dg"
    elif [ "$digs" = "[]" ] || [ -z "$digs" ]; then
      # 本地构建 / docker load 导入的镜像没有 RepoDigests，无法比对（不阻止本地 tag 用法）
      warn "镜像没有 RepoDigests（本地构建或导入），digest 无法校验，标记 UNVERIFIED 后继续"
    else
      fail "$E_IMAGE" "P2-image" "docker image inspect $TARGET_REF" \
        "镜像 digest 与清单不符（清单 $dg，实际 $digs）" "确认镜像来源；不要使用来源不明的镜像"
    fi
  else
    warn "清单里 $TO_VER 没有 digest，跳过 digest 校验（退回 tag + 健康检查）"
  fi

  # 镜像内迁移文件数必须与清单累计条数一致
  local img_cnt
  img_cnt="$(image_migrations_count "$TARGET_REF")"
  if [ "$img_cnt" != "$CHAIN_TOTAL" ]; then
    fail "$E_IMAGE" "P2-image" "镜像内 migrations 数量" \
      "镜像里有 $img_cnt 条迁移，清单要求 $CHAIN_TOTAL 条（镜像与清单不一致）" \
      "确认 tag 是否对应正确版本；必要时更新 manifest.json"
  fi
  info "镜像内迁移文件数：$img_cnt（与清单一致）"
  mark_done image
}

do_migrate() {
  PHASE="P3-migrate"
  step "P3 数据库迁移"
  if [ "$NEED_DOWNTIME" = 1 ] && [ "$APP_STOPPED" != 1 ]; then
    info "链上有 DDL 或不向后兼容版本，先停 app（停机开始）"
    compose stop app >>"${LOG_FILE:-/dev/null}" 2>&1 || fail "$E_MIGRATE" "P3-migrate" "docker compose stop app" "停止 app 失败，未执行迁移" "检查容器状态后重跑"
    APP_STOPPED=1
    state_write
  fi

  info "迁移前：$(schema_count) 条 / 目标 $CHAIN_TOTAL 条"
  if ! migrate_run; then
    rollback_apply "$E_MIGRATE" "数据库迁移失败（drill-migrate 返回非 0，日志见上）"
  fi
  if ! verify_schema_target "$CHAIN_TOTAL" "$CHAIN_THROUGH"; then
    rollback_apply "$E_MIGRATE" "迁移后 schema_migrations 与清单不符"
  fi
  info "迁移完成：$(schema_count) 条，末条 $(schema_last)"
  mark_done migrate
}

do_recreate() {
  PHASE="P4-recreate"
  step "P4 切换镜像并重建 app"

  local cur
  cur="$(compose_app_image)"
  if [ "$cur" != "$TARGET_REF" ]; then
    if ! compose_rewrite_and_check "$TARGET_REF"; then
      rollback_apply "$E_MIGRATE" "compose 改写或校验失败（原文件未改动）"
    fi
    COMPOSE_CHANGED=1
    state_write
    info "compose 已更新：image: $TARGET_REF"
  else
    info "compose 已是目标镜像，跳过改写"
  fi

  if ! recreate_app; then
    rollback_apply "$E_MIGRATE" "docker compose up -d --no-deps app 失败"
  fi
  APP_C="$(app_container)"
  [ -n "$APP_C" ] || rollback_apply "$E_MIGRATE" "重建后找不到 app 容器"
  mark_done recreate
}

do_verify() {
  PHASE="P5-verify"
  step "P5 校验"
  local v path
  path="$(mf_get "$TO_VER" health.path)"
  path="${path:-/api/health}"

  if ! v="$(wait_healthy "$APP_C" "$path" "$TO_VER" "$HEALTH_TIMEOUT")"; then
    rollback_apply "$E_HEALTH" "健康检查 ${HEALTH_TIMEOUT}s 内未通过（最近版本号：${LAST_SEEN_VERSION:-无响应}）"
  fi
  info "健康检查通过：version=$v"

  local bv from_total added=0
  bv="$(brand_version)"
  from_total="$(mf_get "$FROM_VER" schema_migrations_total 2>/dev/null || true)"
  [ -n "$from_total" ] && added=$((CHAIN_TOTAL - from_total))
  if [ "$bv" != "v$TO_VER" ]; then
    # 补丁版本可能不带任何数据库迁移（清单 migrations.count=0）。
    # 这种情况下 brand_version 本来就不会被迁移推进，不能因此判升级失败。
    if [ "$added" -le 0 ]; then
      info "brand_version=$bv（本次无数据库迁移，跳过品牌版本校验）"
    else
      rollback_apply "$E_VERSION" "settings.brand_version=$bv，期望 v$TO_VER"
    fi
  else
    info "brand_version=$bv"
  fi

  if ! verify_schema_target "$CHAIN_TOTAL" "$CHAIN_THROUGH"; then
    rollback_apply "$E_VERSION" "升级后数据库迁移记录与清单不符"
  fi
  info "迁移记录：$(schema_count) 条，末条 $(schema_last)"

  local rc
  rc="$(app_restart_count "$APP_C")"
  [ "$rc" = "0" ] || warn "app 容器 RestartCount=$rc（存在重启，请查看日志）"

  local hits
  hits="$(app_log_scan "$APP_C")"
  if [ -n "$hits" ]; then
    warn "最近 200 行日志里有异常关键字（仅告警，不回滚）："
    printf '%s\n' "$hits" | while IFS= read -r l; do warn "  $l"; done
  fi
  mark_done verify
}

do_finish() {
  PHASE="P6-finish"
  step "P6 收尾"
  warn_public_mount
  RUN_STATUS="success"
  state_write
  write_result "success" "$E_OK" "$PHASE" "upgraded $FROM_VER -> $TO_VER"
  backup_prune
  mycnf_remove
  cleanup_tmp
  {
    printf '\n[OK] 升级完成：%s → %s\n' "$FROM_VER" "$TO_VER"
    printf '     镜像：%s\n' "$TARGET_REF"
    printf '     health：%s   brand_version：%s\n' "$TO_VER" "$(brand_version)"
    printf '     备份：%s\n' "$BACKUP_DIR"
    printf '     日志：%s\n' "$LOG_FILE"
  } | tee -a "$LOG_FILE"
}

warn_public_mount() {
  local json
  json="$(compose_config_json)" || return 0
  if printf '%s' "$json" | python3 -c 'import json,sys
d=json.load(sys.stdin)
s=d.get("services",{}).get("app",{})
for v in (s.get("volumes") or []):
    t=v.get("target","")
    if t.rstrip("/")=="/opt/drill/public":
        sys.exit(0)
sys.exit(1)' 2>/dev/null; then
    warn "检测到 app 把宿主机目录挂到 /opt/drill/public（bind mount）：本次前端更新不会生效"
    warn "如需同步，请用新镜像内的 public 覆盖宿主机目录（保持 755/644），工具不会自动改挂载"
  fi
}

cleanup_tmp() {
  [ -n "$RUN_DIR" ] && rm -f "$RUN_DIR/.chain.err" 2>/dev/null || true
  [ -n "$MYCNF" ] && rm -f "$MYCNF" 2>/dev/null || true
}

# ---------------- 模式 ----------------
show_status() {
  PHASE="status"
  setup_log
  info "清单：$MANIFEST（updated_at $(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("updated_at",""))' "$MANIFEST" 2>/dev/null)）"
  info "清单版本：$(mf_versions | tr '\n' ' ')"
  detect_project
  compose_init
  pick_containers
  detect_network

  local img health n last
  db_init
  img="$(compose_app_image)"
  info "compose app.image：$img"
  info "app 容器：$(docker inspect "$APP_C" --format '{{.Name}}' 2>/dev/null | sed 's#^/##')"
  health="$(health_once "$APP_C" "/api/health")" || health="（不可用）"
  info "health.version：$health"

  mycnf_install "$E_PRECHECK" "status"
  n="$(schema_count)"; last="$(schema_last)"
  info "schema_migrations：$n 条，末条 ${last:-无}"
  info "按清单推断的当前版本：$(mf_version_by_total "$n" || true)"

  local d
  d="$(ls -1dt "$DEPLOY_DIR"/.upgrade-runs/*/ 2>/dev/null | head -1)"
  if [ -n "$d" ] && [ -f "$d/RESULT.json" ]; then
    info "最近一次升级：$(basename "$d") → $(state_get_file "$d/RESULT.json" status)（$(state_get_file "$d/RESULT.json" finished_at)）"
  else
    info "最近一次升级：无记录"
  fi
  local cnt
  cnt="$(ls -1d "$DEPLOY_DIR"/backups/upgrade-*-to-* 2>/dev/null | wc -l)"
  info "升级备份目录数：$cnt"
  mycnf_remove
}

recover_last() {
  PHASE="P0-recover"
  local d last=""
  for d in $(ls -1dt "$DEPLOY_DIR"/.upgrade-runs/*/ 2>/dev/null); do
    [ -f "$d/STATE.json" ] || continue
    case "$(state_get_file "$d/STATE.json" status)" in
      rollback_failed|rolling_back) last="$d"; break ;;
    esac
  done
  if [ -z "$last" ]; then
    info "没有处于回滚中/回滚失败的升级记录，无需处理"
    exit 0
  fi

  RUN_DIR="$last"
  RUN_ID="$(basename "$RUN_DIR")"
  FROM_VER="$(state_get_file "$RUN_DIR/STATE.json" from)"
  TO_VER="$(state_get_file "$RUN_DIR/STATE.json" to)"
  BACKUP_DIR="$(state_get_file "$RUN_DIR/STATE.json" artifacts.backup_dir)"
  SCHEMA_BEFORE="$(state_get_file "$RUN_DIR/STATE.json" artifacts.schema_before)"
  CURRENT_REF="$(state_get_file "$RUN_DIR/STATE.json" artifacts.old_image_ref)"
  COMPOSE_CHANGED="$(state_get_file "$RUN_DIR/STATE.json" artifacts.compose_changed)"
  COMPOSE_CHANGED="${COMPOSE_CHANGED:-0}"
  APP_STOPPED="$(state_get_file "$RUN_DIR/STATE.json" artifacts.app_stopped)"
  STATE_FILE="$RUN_DIR/STATE.json"
  RESULT_FILE="$RUN_DIR/RESULT.json"
  [ -n "$BACKUP_DIR" ] && [ -f "$BACKUP_DIR/db.sql.gz" ] || fail "$E_ROLLBACK_FAIL" "P0-recover" "读取备份" "备份目录不可用" "按 docs/upgrade.md 的手工步骤恢复"

  detect_project
  compose_init
  pick_containers
  detect_network
  db_init
  mycnf_install "$E_ROLLBACK_FAIL" "P0-recover"
  info "继续回滚记录：$RUN_ID（$FROM_VER → $TO_VER）"
  rollback_apply "$E_ROLLBACK_FAIL" "人工触发 --recover-last"
}

on_exit() {
  local rc=$?
  [ "$rc" -eq 0 ] || true
  # 任何退出路径（含 dry-run / noop / 失败）都不留口令文件
  if [ -n "${MYSQL_C:-}" ]; then
    docker exec "$MYSQL_C" rm -f /tmp/.upgrade-my.cnf >/dev/null 2>&1 || true
  fi
  [ -n "${MYCNF:-}" ] && rm -f "$MYCNF" 2>/dev/null
  return 0
}

# ---------------- 主流程 ----------------
main() {
  parse_args "$@"
  setup_paths
  trap on_exit EXIT

  if [ "$MODE" = "status" ]; then
    setup_log
    MANIFEST_OVERRIDE="$MANIFEST_OVERRIDE" manifest_init
    show_status
    exit 0
  fi
  if [ "$MODE" = "recover" ]; then
    acquire_lock
    MANIFEST_OVERRIDE="$MANIFEST_OVERRIDE" manifest_init
    setup_log
    recover_last
    exit 0
  fi

  acquire_lock
  manifest_init

  # 先解析当前版本（日志文件名依赖它）；此阶段的输出只在终端
  detect_project
  compose_init
  pick_containers
  detect_network
  db_init
  mycnf_install "$E_PRECHECK" "P0-precheck"
  if [ "$DRY_RUN" = 1 ] || ! select_run_dir; then
    resolve_from
  fi
  mycnf_remove

  mkdir -p "$LOG_DIR" 2>/dev/null || fail "$E_PRECHECK" "P0-precheck" "mkdir $LOG_DIR" "无法创建日志目录" "检查目录权限或用 --log-dir 指定"
  LOG_FILE="$LOG_DIR/upgrade-${FROM_VER}-to-${TO_VER}-${RUN_TS}.log"
  : >>"$LOG_FILE" || fail "$E_PRECHECK" "P0-precheck" "写日志 $LOG_FILE" "无法写日志文件" "检查磁盘空间与权限"
  chmod 600 "$LOG_FILE" 2>/dev/null || true
  info "═══ Drill 一键升级 $FROM_VER → $TO_VER 开始 ═══"
  info "版本链与清单：$MANIFEST"
  info "日志：$LOG_FILE"

  if [ "$DRY_RUN" = 1 ]; then
    # 空跑不落任何运行记录，避免污染 .upgrade-runs 与断点续跑
    RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/drill-upgrade-dryrun-XXXXXX")"
    RESUMING=0
  elif [ "$RESUMING" = 1 ]; then
    load_state
  else
    new_run_dir
  fi
  RUN_ID="$(basename "$RUN_DIR")"
  STATE_FILE="$RUN_DIR/STATE.json"
  RESULT_FILE="$RUN_DIR/RESULT.json"
  state_write

  # 上次死在回滚中途：先把手尾收掉，不要继续往前走
  if [ "$RESUMING" = 1 ] && [ "$RESUMING_STATUS" = "rolling_back" ]; then
    info "上次升级终止于回滚阶段，继续执行回滚（不再重试升级）"
    rollback_apply "${RUN_EXIT_CODE:-$E_MIGRATE}" "断点续跑：继续上次未完成的回滚"
  fi

  precheck
  do_backup
  do_image
  do_migrate
  do_recreate
  do_verify
  do_finish
  exit 0
}

main "$@"
