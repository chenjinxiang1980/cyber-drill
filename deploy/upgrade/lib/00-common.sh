#!/usr/bin/env bash
# =============================================================================
# 00-common.sh —— 日志、错误、锁、状态文件、脱敏
#
# 原则：
#   - 任何失败都带「阶段 / 命令 / 原因 / 下一步 / 日志路径」，不吞错误；
#   - 口令只从文件读，禁止出现在命令行与日志；
#   - 状态文件 STATE.json 每完成一个阶段写一次，供断点续跑。
# =============================================================================

# 退出码（与 docs/upgrade-tool-design.md 5.3 一致）
readonly E_OK=0
readonly E_PRECHECK=10
readonly E_CHAIN=11
readonly E_DISK=12
readonly E_BACKUP=13
readonly E_MIGRATE=14
readonly E_HEALTH=15
readonly E_VERSION=16
readonly E_ROLLBACK_FAIL=20
readonly E_LOCK=30
readonly E_IMAGE=40

# 运行期全局
DEPLOY_DIR=""
COMPOSE_FILE=""
ENV_FILE=""
LOG_DIR=""
LOG_FILE=""
RUN_DIR=""
BACKUP_DIR=""
STATE_FILE=""
RESULT_FILE=""
LOCK_FILE=""
FROM_VER=""
TO_VER=""
CURRENT_REF=""
TARGET_REF=""
IMAGE_TAG=""
PROJECT=""
APP_C=""
MYSQL_C=""
NET_NAME=""
DB_NAME=""
DB_HOST_APP=""
APP_STOPPED=0
COMPOSE_CHANGED=0
SCHEMA_BEFORE=""
SCHEMA_AFTER=""
DUMP_ROWS=""
OLD_IMAGE_ID=""
MYCNF=""
DRY_RUN=0
ASSUME_YES=0
FAST_MODE=0
SKIP_BACKUP=0
OFFLINE=0
PIN_DIGEST=0
FORCE_CHAIN=0
ALLOW_INTERNAL=0
REGISTRY_GIVEN=""
REGISTRY="ghcr"
IMAGE_TAR=""
IMAGE_REF_OPT=""
KEEP_BACKUPS=3
HEALTH_TIMEOUT=180
PHASE="init"
ROLLBACK_DONE=0
RESUMING_STATUS=""
RUN_EXIT_CODE=""
SECRET_VALUES=()

declare -a DONE_PHASES=()

# ---------- 日志 ----------
ts() { date '+%F %T'; }

mask() {
  local s="$1" v
  for v in ${SECRET_VALUES[@]+"${SECRET_VALUES[@]}"}; do
    [ -n "$v" ] && s="${s//$v/***}"
  done
  # 兜底：常见的口令型参数
  # 注意 -p 必须独立出现，否则 "--pin-digest" 这类参数会被误伤
  s="$(printf '%s' "$s" | sed -E 's/(^|[[:space:]])-p[^[:space:]"]*/\1-p***/g; s/(--password[= ])[^[:space:]"]*/\1***/g; s/(MYSQL_PWD=)[^[:space:]"]+/\1***/g; s/(PASSWORD|SECRET|TOKEN)[=:][^[:space:]"]*/\1=***/g')"
  printf '%s' "$s"
}

log() { # log <LEVEL> <msg...>
  local level="$1"; shift
  local line="[$(ts)] [$level] $(mask "$*")"
  printf '%s\n' "$line"
  [ -n "$LOG_FILE" ] && printf '%s\n' "$line" >>"$LOG_FILE"
  return 0
}
info() { log INFO "$@"; }
warn() { log WARN "$@"; }
step() { log STEP "$@"; }

# 记录并执行一条 shell 片段：输出同时进日志与终端
# 用法：run_sh "<命令说明>" "<shell 片段>"
run_sh() {
  local desc="$1" script="$2"
  log CMD "$desc"
  printf '$ %s\n' "$(mask "$script")" >>"${LOG_FILE:-/dev/null}"
  local rc=0
  bash -o pipefail -c "$script" 2>&1 | tee -a "${LOG_FILE:-/dev/null}" || rc=${PIPESTATUS[0]}
  return "$rc"
}

# 统一失败出口：打印可执行信息后退出
# fail <exitcode> <阶段> <失败命令> <原因> <下一步>
fail() {
  local code="$1" phase="$2" cmd="$3" reason="$4" next="$5"
  RUN_STATUS="failed"
  RUN_EXIT_CODE="$code"
  {
    printf '\n'
    printf '[ERROR] 卡在阶段：%s\n' "$phase"
    printf '        失败命令：%s\n' "$(mask "$cmd")"
    printf '        原因：%s\n' "$reason"
    printf '        下一步：%s\n' "$next"
    printf '        日志：%s\n' "${LOG_FILE:-（未初始化）}"
    [ -n "$BACKUP_DIR" ] && printf '        备份：%s\n' "$BACKUP_DIR"
  } | tee -a "${LOG_FILE:-/dev/null}" >&2
  write_result "failed" "$code" "$phase" "$reason"
  state_write
  exit "$code"
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/\t/\\t/g'
}

json_array_of() {
  local out='[' sep='' x
  for x in "$@"; do out+="$sep\"$(json_escape "$x")\""; sep=','; done
  printf '%s]' "$out"
}

# ---------- 状态文件 ----------
state_write() {
  [ -n "$STATE_FILE" ] || return 0
  cat >"$STATE_FILE" <<EOF
{
  "run_id": "$(json_escape "${RUN_ID:-}")",
  "from": "$(json_escape "$FROM_VER")",
  "to": "$(json_escape "$TO_VER")",
  "phase": "$(json_escape "$PHASE")",
  "status": "$(json_escape "${RUN_STATUS:-running}")",
  "exit_code": ${RUN_EXIT_CODE:-0},
  "done": $(json_array_of ${DONE_PHASES[@]+"${DONE_PHASES[@]}"}),
  "artifacts": {
    "backup_dir": "$(json_escape "$BACKUP_DIR")",
    "dump_file": "db.sql.gz",
    "dump_db_rows": "$(json_escape "$DUMP_ROWS")",
    "schema_before": "$(json_escape "$SCHEMA_BEFORE")",
    "old_image_ref": "$(json_escape "$CURRENT_REF")",
    "old_image_id": "$(json_escape "$OLD_IMAGE_ID")",
    "new_image_ref": "$(json_escape "$TARGET_REF")",
    "app_stopped": $APP_STOPPED,
    "compose_changed": $COMPOSE_CHANGED
  },
  "started_at": "$(json_escape "${RUN_STARTED_AT:-}")",
  "updated_at": "$(ts)"
}
EOF
  chmod 600 "$STATE_FILE" 2>/dev/null || true
}

phase_done() {
  local p
  for p in ${DONE_PHASES[@]+"${DONE_PHASES[@]}"}; do [ "$p" = "$1" ] && return 0; done
  return 1
}

mark_done() {
  phase_done "$1" || DONE_PHASES+=("$1")
  state_write
}

write_result() { # status exit_code phase reason
  [ -n "$RESULT_FILE" ] || return 0
  cat >"$RESULT_FILE" <<EOF
{
  "status": "$(json_escape "$1")",
  "exit_code": $2,
  "phase": "$(json_escape "$3")",
  "reason": "$(json_escape "$4")",
  "from": "$(json_escape "$FROM_VER")",
  "to": "$(json_escape "$TO_VER")",
  "backup_dir": "$(json_escape "$BACKUP_DIR")",
  "log_file": "$(json_escape "$LOG_FILE")",
  "finished_at": "$(ts)"
}
EOF
  chmod 600 "$RESULT_FILE" 2>/dev/null || true
  # 备份目录里也留一份，供保留策略判定「成功/失败」
  if [ -n "$BACKUP_DIR" ] && [ -d "$BACKUP_DIR" ]; then
    cp -f "$RESULT_FILE" "$BACKUP_DIR/RESULT.json" 2>/dev/null || true
    chmod 600 "$BACKUP_DIR/RESULT.json" 2>/dev/null || true
  fi
}

# ---------- 锁 ----------
acquire_lock() {
  LOCK_FILE="$DEPLOY_DIR/.upgrade.lock"
  mkdir -p "$DEPLOY_DIR" 2>/dev/null || true
  # 用 >> 打开：抢锁失败时不要清掉持锁进程写下的信息
  exec 9>>"$LOCK_FILE" || fail "$E_PRECHECK" "P0-precheck" "exec 9>>$LOCK_FILE" "无法创建锁文件" "检查 $DEPLOY_DIR 写权限"
  if ! flock -n 9; then
    local holder
    holder="$(cat "$LOCK_FILE" 2>/dev/null | tr -d '\n')"
    fail "$E_LOCK" "P0-precheck" "flock $LOCK_FILE" \
      "已有另一个升级进程在运行（持锁信息：${holder:-未知}）" \
      "等前一个升级结束后重跑；若确认无进程在跑，请检查是否有残留进程持锁"
  fi
  printf '%s pid=%s since=%s\n' "$(basename "$0")" "$$" "$(ts)" >"$LOCK_FILE"
}

# ---------- 环境文件读取 ----------
env_get() { # env_get <KEY> [file]
  local key="$1" file="${2:-$ENV_FILE}"
  [ -f "$file" ] || return 0
  awk -v k="$key" '
    $0 ~ /^[[:space:]]*#/ { next }
    {
      line=$0
      sub(/[[:space:]]*#.*$/, "", line)
      p=index(line, "=")
      if (p <= 0) next
      kk=substr(line, 1, p-1); gsub(/^[[:space:]]+|[[:space:]]+$/, "", kk)
      if (kk != k) next
      vv=substr(line, p+1); gsub(/^[[:space:]]+|[[:space:]]+$/, "", vv)
      gsub(/^["\x27]|["\x27]$/, "", vv)
      print vv; exit
    }' "$file"
}

secret_register() {
  local v
  for v in "$@"; do [ -n "$v" ] && SECRET_VALUES+=("$v"); done
}
