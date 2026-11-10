#!/usr/bin/env bash
# =============================================================================
# 30-migrate.sh —— 用「目标镜像」的迁移器在一次性容器里执行迁移
#
# 为什么用临时容器：不触发 entrypoint 的「等 MySQL + seed」，且迁移失败时
# 旧版容器仍在（含 DDL 的版本会先停 app，见 upgrade.sh P3）。
# =============================================================================

image_migrations_count() { # <image-ref>
  docker run --rm --entrypoint sh "$1" -c 'ls /opt/drill/migrations/*.up.sql 2>/dev/null | wc -l' 2>>"${LOG_FILE:-/dev/null}" | tr -d '[:space:]'
}

image_has_migration() { # <image-ref> <migration-name>
  docker run --rm --entrypoint sh "$1" -c "test -f /opt/drill/migrations/$2.up.sql" >/dev/null 2>&1
}

migrate_run() {
  local log_hint="一次性容器执行 drill-migrate up（镜像 $TARGET_REF）"
  run_sh "$log_hint" \
    "docker run --rm --network \"$NET_NAME\" --env-file \"$ENV_FILE\" -e DB_HOST=\"$DB_HOST_APP\" --entrypoint /opt/drill/bin/drill-migrate \"$TARGET_REF\" -root=/opt/drill up"
}

verify_schema_target() { # <expect_total> <expect_through>
  local total="$1" through="$2" n last
  n="$(schema_count)"
  last="$(schema_last)"
  SCHEMA_AFTER="$n"
  if [ "$n" != "$total" ]; then
    warn "schema_migrations 行数 $n，期望 $total"
    return 1
  fi
  if [ -n "$through" ] && [ "$last" != "$through" ]; then
    warn "schema_migrations 末条 $last，期望 $through"
    return 1
  fi
  return 0
}
