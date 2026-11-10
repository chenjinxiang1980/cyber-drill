#!/usr/bin/env bash
# =============================================================================
# 20-backup.sh —— 数据库访问、整库备份、备份校验（含恢复演练）、保留策略
#
# 安全约定：
#   - 口令只写进 0600 的 .my.cnf，再用 docker cp 放进 mysql 容器；
#   - 容器内临时文件用完即删；
#   - 宿主机命令行、日志里都不出现口令。
# =============================================================================

DB_USER=""
DB_PASSWORD=""
DB_ROOT_USER="root"
DB_ROOT_PASSWORD=""

db_init() {
  DB_NAME="$(env_get DB_NAME)"
  DB_NAME="${DB_NAME:-drill_system}"
  if ! printf '%s' "$DB_NAME" | grep -Eq '^[A-Za-z0-9_]+$'; then
    fail "$E_PRECHECK" "P0-precheck" "DB_NAME=$DB_NAME" "库名含非法字符" "检查 .env 的 DB_NAME"
  fi
  DB_HOST_APP="$(env_get DB_HOST)"
  DB_HOST_APP="${DB_HOST_APP:-mysql}"
  # 容器里的 DB_HOST 才是迁移容器能用的值（.env 里常写 127.0.0.1）
  if [ -n "$APP_C" ]; then
    local ch
    ch="$(docker inspect "$APP_C" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | sed -n 's/^DB_HOST=//p' | head -1)"
    [ -n "$ch" ] && DB_HOST_APP="$ch"
  fi
  DB_USER="$(env_get DB_USER)"
  DB_PASSWORD="$(env_get DB_PASSWORD)"
  DB_ROOT_USER="$(env_get DB_ROOT_USER)"
  DB_ROOT_USER="${DB_ROOT_USER:-root}"
  DB_ROOT_PASSWORD="$(env_get DB_ROOT_PASSWORD)"
  [ -n "$DB_ROOT_PASSWORD" ] || DB_ROOT_PASSWORD="$(env_get MYSQL_ROOT_PASSWORD)"
  secret_register "$DB_PASSWORD" "$DB_ROOT_PASSWORD"
}

# 生成 0600 客户端配置，并复制到 mysql 容器内部（口令不出现在命令行）
mycnf_install() { # [exit_code] [phase]
  local code="${1:-$E_BACKUP}" phase="${2:-P1-backup}"
  [ -n "$DB_ROOT_PASSWORD" ] || fail "$code" "$phase" "读取 root 口令" \
    "环境文件里没有 DB_ROOT_PASSWORD / MYSQL_ROOT_PASSWORD" \
    "在 $ENV_FILE 里补上 DB_ROOT_PASSWORD（与 mysql 服务一致），或加 --fast 跳过恢复演练（仍需要 root 口令做整库备份）"
  MYCNF="${TMPDIR:-/tmp}/.drill-upgrade-my-$$.cnf"
  umask 077
  cat >"$MYCNF" <<EOF
[client]
user=$DB_ROOT_USER
password=$DB_ROOT_PASSWORD
host=127.0.0.1
port=3306
default-character-set=utf8mb4
EOF
  chmod 600 "$MYCNF"
  umask 022
  docker cp "$MYCNF" "$MYSQL_C:/tmp/.upgrade-my.cnf" >>"${LOG_FILE:-/dev/null}" 2>&1 \
    || fail "$code" "$phase" "docker cp .my.cnf" "无法把客户端配置放进 mysql 容器" "确认容器 $MYSQL_C 在运行且有写 /tmp 权限"
  docker exec "$MYSQL_C" chmod 600 /tmp/.upgrade-my.cnf >>"${LOG_FILE:-/dev/null}" 2>&1 || true
}

mycnf_remove() {
  docker exec "$MYSQL_C" rm -f /tmp/.upgrade-my.cnf >>"${LOG_FILE:-/dev/null}" 2>&1 || true
  [ -n "$MYCNF" ] && rm -f "$MYCNF"
}

sql_exec() { # sql_exec <sql> [db]
  local sql="$1" db="${2:-$DB_NAME}"
  docker exec "$MYSQL_C" mysql --defaults-extra-file=/tmp/.upgrade-my.cnf --batch -e "$sql" "$db" \
    >>"${LOG_FILE:-/dev/null}" 2>&1
}

sql_scalar() { # sql_scalar <sql> [db]
  local sql="$1" db="${2:-$DB_NAME}"
  docker exec "$MYSQL_C" mysql --defaults-extra-file=/tmp/.upgrade-my.cnf --batch --skip-column-names -e "$sql" "$db" \
    2>>"${LOG_FILE:-/dev/null}"
}

table_exists() {
  local n
  n="$(sql_scalar "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME' AND table_name='$1'" information_schema 2>/dev/null)"
  [ "${n:-0}" -gt 0 ]
}

schema_count() {
  if ! table_exists schema_migrations; then echo 0; return 0; fi
  local n
  n="$(sql_scalar "SELECT COUNT(*) FROM schema_migrations")"
  printf '%s' "${n:-0}"
}

schema_last() {
  if ! table_exists schema_migrations; then return 0; fi
  sql_scalar "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1"
}

db_collation() {
  local c
  c="$(sql_scalar "SELECT default_collation_name FROM information_schema.schemata WHERE schema_name='$DB_NAME'" information_schema 2>/dev/null)"
  printf '%s' "${c:-utf8mb4_unicode_ci}"
}

# ---------------- 备份 ----------------
backup_prepare() {
  BACKUP_DIR="$DEPLOY_DIR/backups/upgrade-${FROM_VER}-to-${TO_VER}-${RUN_TS}"
  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  export BACKUP_DIR
  info "备份目录：$BACKUP_DIR"
  mycnf_install

  SCHEMA_BEFORE="$(schema_count)"
  DUMP_ROWS="$SCHEMA_BEFORE"
  OLD_IMAGE_ID="$(docker image inspect "$CURRENT_REF" --format '{{.Id}}' 2>/dev/null | cut -c8-19)"
  state_write
}

backup_run() {
  step "P1 备份：整库 dump + 环境文件 + compose + 元信息"
  local dump="$BACKUP_DIR/db.sql.gz"

  if run_sh "mysqldump 整库 → db.sql.gz" \
    "docker exec \"$MYSQL_C\" sh -c \"mysqldump --defaults-extra-file=/tmp/.upgrade-my.cnf --single-transaction --quick --routines --triggers --events --no-tablespaces --default-character-set=utf8mb4 '$DB_NAME'\" | gzip -1 > \"$dump\""; then
    [ -s "$dump" ] || return 1
  else
    return 1
  fi

  cp -p "$ENV_FILE" "$BACKUP_DIR/env.bak" || return 1
  cp -p "$COMPOSE_FILE" "$BACKUP_DIR/compose.bak" || return 1
  chmod 600 "$BACKUP_DIR/env.bak" "$BACKUP_DIR/compose.bak" "$dump" 2>/dev/null || true

  cat >"$BACKUP_DIR/META.json" <<EOF
{
  "run_id": "$(json_escape "$RUN_ID")",
  "from": "$(json_escape "$FROM_VER")",
  "to": "$(json_escape "$TO_VER")",
  "created_at": "$(ts)",
  "operator": "$(id -un 2>/dev/null || echo unknown)",
  "host": "$(hostname 2>/dev/null || echo unknown)",
  "deploy_dir": "$(json_escape "$DEPLOY_DIR")",
  "compose_file": "$(json_escape "$COMPOSE_FILE")",
  "env_file": "$(json_escape "$ENV_FILE")",
  "db_name": "$(json_escape "$DB_NAME")",
  "db_collation": "$(json_escape "$(db_collation)")",
  "schema_rows": "$(json_escape "$SCHEMA_BEFORE")",
  "old_image_ref": "$(json_escape "$CURRENT_REF")",
  "old_image_id": "$(json_escape "$OLD_IMAGE_ID")",
  "target_image_ref": "$(json_escape "$TARGET_REF")"
}
EOF
  ( cd "$BACKUP_DIR" && sha256sum db.sql.gz env.bak compose.bak META.json >SHA256SUMS ) || return 1
  chmod 600 "$BACKUP_DIR/SHA256SUMS" "$BACKUP_DIR/META.json"
  return 0
}

backup_verify() {
  local dump="$BACKUP_DIR/db.sql.gz"
  if ! gzip -t "$dump"; then
    return 1
  fi
  if ! ( cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS >/dev/null 2>&1 ); then
    return 1
  fi
  info "备份完整性校验通过（gzip -t + sha256sum -c）"

  if [ "$FAST_MODE" = 1 ]; then
    warn "已跳过恢复演练（--fast）：本次备份标记为 UNVERIFIED"
    return 0
  fi

  local vdb="drill_upgrade_verify_${RUN_TS}"
  vdb="${vdb//-/_}"
  vdb="${vdb}_$$"
  info "恢复演练：把 dump 导入临时库 $vdb"
  sql_exec "DROP DATABASE IF EXISTS \`$vdb\`" information_schema || return 1
  sql_exec "CREATE DATABASE \`$vdb\` DEFAULT CHARACTER SET utf8mb4 COLLATE $(db_collation)" information_schema || return 1
  docker cp "$dump" "$MYSQL_C:/tmp/.upgrade-verify.sql.gz" >>"${LOG_FILE:-/dev/null}" 2>&1 || return 1
  if ! run_sh "恢复演练：导入临时库" \
    "docker exec \"$MYSQL_C\" sh -c \"gunzip -c /tmp/.upgrade-verify.sql.gz | mysql --defaults-extra-file=/tmp/.upgrade-my.cnf '$vdb'\""; then
    sql_exec "DROP DATABASE IF EXISTS \`$vdb\`" || true
    docker exec "$MYSQL_C" rm -f /tmp/.upgrade-verify.sql.gz >>"${LOG_FILE:-/dev/null}" 2>&1 || true
    return 1
  fi

  local src_tables dst_tables src_rows dst_rows
  src_tables="$(sql_scalar "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME'" information_schema)"
  dst_tables="$(sql_scalar "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$vdb'" information_schema)"
  src_rows="$SCHEMA_BEFORE"
  dst_rows="$(sql_scalar "SELECT COUNT(*) FROM schema_migrations" "$vdb")"

  docker exec "$MYSQL_C" rm -f /tmp/.upgrade-verify.sql.gz >>"${LOG_FILE:-/dev/null}" 2>&1 || true
  sql_exec "DROP DATABASE IF EXISTS \`$vdb\`" || true

  if [ "$src_tables" != "$dst_tables" ] || [ "$src_rows" != "$dst_rows" ]; then
    warn "恢复演练结果不符：源库 ${src_tables} 张表/${src_rows} 条迁移；演练库 ${dst_tables} 张表/${dst_rows} 条迁移"
    return 1
  fi
  info "恢复演练通过：${dst_tables} 张表 / ${dst_rows} 条迁移记录"
  return 0
}

# 回滚时整库恢复（drop + create + 导入）
restore_db() {
  local dump="$BACKUP_DIR/db.sql.gz" coll
  [ -f "$dump" ] || return 1
  coll="$(db_collation)"   # 必须赶在 DROP 之前取，落库时查不到原排序规则
  info "整库恢复：DROP + CREATE + 导入（备份时 ${SCHEMA_BEFORE} 条迁移）"
  sql_exec "DROP DATABASE IF EXISTS \`$DB_NAME\`" information_schema || return 1
  sql_exec "CREATE DATABASE \`$DB_NAME\` DEFAULT CHARACTER SET utf8mb4 COLLATE $coll" information_schema || return 1
  docker cp "$dump" "$MYSQL_C:/tmp/.upgrade-restore.sql.gz" >>"${LOG_FILE:-/dev/null}" 2>&1 || return 1
  run_sh "恢复整库" \
    "docker exec \"$MYSQL_C\" sh -c \"gunzip -c /tmp/.upgrade-restore.sql.gz | mysql --defaults-extra-file=/tmp/.upgrade-my.cnf '$DB_NAME'\"" || return 1
  docker exec "$MYSQL_C" rm -f /tmp/.upgrade-restore.sql.gz >>"${LOG_FILE:-/dev/null}" 2>&1 || true
  return 0
}

# 保留最近 N 份「成功」备份；失败/回滚过的备份永不自动删除
backup_prune() {
  [ "$KEEP_BACKUPS" -gt 0 ] || return 0
  local root="$DEPLOY_DIR/backups"
  [ -d "$root" ] || return 0
  local keep=0 d
  while IFS= read -r d; do
    [ -d "$d" ] || continue
    if [ -f "$d/RESULT.json" ] && grep -q '"status": "success"' "$d/RESULT.json" 2>/dev/null; then
      keep=$((keep + 1))
      if [ "$keep" -gt "$KEEP_BACKUPS" ]; then
        info "清理过期备份：$d"
        rm -rf -- "$d"
      fi
    fi
  done < <(ls -1d "$root"/upgrade-*-to-* 2>/dev/null | sort -r)
  return 0
}
