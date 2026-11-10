#!/usr/bin/env bash
# =============================================================================
# 50-rollback.sh —— 失败自动回滚
#
# 顺序不可颠倒：停 app → 还原 compose → 整库恢复 → 起旧版 → 校验旧版本。
# 迁移没有 down 脚本，DDL 已被 MySQL 隐式提交，唯一可靠的回滚是整库恢复。
# =============================================================================

rollback_manual_steps() {
  cat <<EOF
        手工恢复步骤（按顺序）：
          1) 还原 compose：
             cp -p "$BACKUP_DIR/compose.bak" "$COMPOSE_FILE"
          2) 整库恢复（会覆盖当前库，备份之后写入的业务数据将丢失）：
             docker cp "$BACKUP_DIR/db.sql.gz" "$MYSQL_C:/tmp/restore.sql.gz"
             docker exec "$MYSQL_C" sh -c 'gunzip -c /tmp/restore.sql.gz | mysql -uroot -p "$DB_NAME"'
          3) 启动旧版：
             cd "$DEPLOY_DIR" && docker compose up -d --no-deps app
          4) 校验：
             docker exec "$(app_container)" wget -qO- http://127.0.0.1:8090/api/health
             docker exec "$MYSQL_C" mysql -uroot -p -N -e 'select count(*) from schema_migrations' "$DB_NAME"
EOF
}

rollback_failed() {
  local why="$1"
  PHASE="P6-rollback"
  state_write
  {
    printf '\n[ERROR] 自动回滚失败：%s\n' "$why"
    printf '        当前状态：app 已停止，不再自动重试\n'
    printf '        备份目录：%s\n' "$BACKUP_DIR"
    printf '        校验备份：cd %s && sha256sum -c SHA256SUMS\n' "$BACKUP_DIR"
    printf '        日志：%s\n' "$LOG_FILE"
    rollback_manual_steps
  } | tee -a "$LOG_FILE" >&2
  write_result "rollback_failed" "$E_ROLLBACK_FAIL" "$PHASE" "$why"
  exit "$E_ROLLBACK_FAIL"
}

# rollback_apply <拟退出码> <失败原因>
rollback_apply() {
  local code="$1" reason="$2" cur
  PHASE="P6-rollback"
  RUN_STATUS="rolling_back"
  RUN_EXIT_CODE="$code"
  state_write
  step "开始自动回滚（退出码将为 $code）"

  # 1) 停掉可能半启动的新容器
  if ! compose stop app >>"${LOG_FILE:-/dev/null}" 2>&1; then
    warn "停止 app 容器失败，继续尝试回滚"
  fi

  # 2) 还原 compose 的 image 行
  if [ "$COMPOSE_CHANGED" = 1 ]; then
    if [ -f "$BACKUP_DIR/compose.bak" ]; then
      cp -p "$BACKUP_DIR/compose.bak" "$COMPOSE_FILE" || rollback_failed "还原 compose 失败"
      COMPOSE_CHANGED=0
      info "已还原 compose（image 回到 $CURRENT_REF）"
    else
      rollback_failed "compose 已被改写，但备份里没有 compose.bak"
    fi
  fi

  # 3) 迁移已推进 → 整库恢复
  cur="$(schema_count)"
  if [ -n "$SCHEMA_BEFORE" ] && [ "$cur" -gt "$SCHEMA_BEFORE" ]; then
    info "检测到迁移已推进（$SCHEMA_BEFORE → $cur），执行整库恢复"
    if ! restore_db; then
      rollback_failed "整库恢复失败"
    fi
    info "整库恢复完成"
  else
    info "迁移未推进（当前 $cur 条，备份时 $SCHEMA_BEFORE 条），跳过整库恢复"
  fi

  # 4) 起旧版
  if ! compose up -d --no-deps app >>"${LOG_FILE:-/dev/null}" 2>&1; then
    rollback_failed "旧版容器启动失败"
  fi
  APP_C="$(app_container)"
  [ -n "$APP_C" ] || rollback_failed "找不到 app 容器"

  # 5) 校验旧版本
  local v
  if ! v="$(wait_healthy "$APP_C" "$(mf_get "$FROM_VER" health.path)" "$FROM_VER" "$HEALTH_TIMEOUT")"; then
    rollback_failed "旧版健康检查未通过（期望 $FROM_VER）"
  fi
  local rows
  rows="$(schema_count)"
  if [ -n "$SCHEMA_BEFORE" ] && [ "$rows" != "$SCHEMA_BEFORE" ]; then
    rollback_failed "旧版已运行，但 schema_migrations 行数为 $rows，期望 $SCHEMA_BEFORE"
  fi

  ROLLBACK_DONE=1
  RUN_STATUS="rolled_back"
  PHASE="P6-rollback"
  state_write
  write_result "rolled_back" "$code" "$PHASE" "$reason"
  {
    printf '\n[WARN] 升级失败，已自动回滚到 %s\n' "$FROM_VER"
    printf '       失败原因：%s\n' "$reason"
    printf '       备份与日志：%s / %s\n' "$BACKUP_DIR" "$LOG_FILE"
    printf '       注意：若迁移已执行，备份之后写入的业务数据已被整库恢复覆盖\n'
    printf '       下一步：查看日志定位失败迁移，修复后重新升级；不要直接重试迁移\n'
  } | tee -a "$LOG_FILE" >&2
  exit "$code"
}
