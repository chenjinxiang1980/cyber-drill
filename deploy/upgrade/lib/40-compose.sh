#!/usr/bin/env bash
# =============================================================================
# 40-compose.sh —— compose 解析、image 改写、重建 app、健康检查
#
# 只改 app 服务的 image 行；其余内容（build / environment / volumes）一律不动。
# 改写流程：生成 .new → compose config -q 校验 → 原子 mv 覆盖。
# =============================================================================

COMPOSE_ARGS=()

compose() {
  docker compose "${COMPOSE_ARGS[@]}" "$@"
}

compose_init() {
  COMPOSE_ARGS=(--project-directory "$DEPLOY_DIR" -f "$COMPOSE_FILE")
  [ -n "$PROJECT" ] && COMPOSE_ARGS+=(-p "$PROJECT")
  # 让 compose 读到的变量与用户手工执行时一致
  if [ -f "$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE" >/dev/null 2>&1 || true
    set +a
  fi
}

compose_config_json() {
  compose config --format json 2>>"${LOG_FILE:-/dev/null}"
}

compose_app_image() {
  compose_config_json | python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit(1)
s=d.get("services",{}).get("app")
if not s: sys.exit(2)
print(s.get("image",""))'
}

compose_has_service() {
  compose_config_json | python3 -c 'import json,sys
d=json.load(sys.stdin); sys.exit(0 if sys.argv[1] in d.get("services",{}) else 1)' "$1" 2>/dev/null
}

# 用 awk 只替换 app 服务块里的 image 行；替换不到就返回 1
rewrite_compose_image() { # <new-ref> <src> <dst>
  awk -v newref="$1" '
    /^[[:space:]]*#/ { print; next }
    /^[[:space:]]*[A-Za-z0-9_.-]+:[[:space:]]*$/ {
      key=$0; sub(/:.*/,"",key); gsub(/^[[:space:]]+|[[:space:]]+$/,"",key)
      inservice=(key=="app")
    }
    inservice && !done && /^[[:space:]]+image:[[:space:]]*/ {
      indent=$0; sub(/image:.*/,"",indent)
      print indent "image: " newref
      done=1
      next
    }
    { print }
    END { if (!done) exit 1 }
  ' "$2" >"$3"
}

compose_rewrite_and_check() { # <new-ref>；成功则原文件已原子替换
  local newref="$1" tmp="$COMPOSE_FILE.new"
  if ! rewrite_compose_image "$newref" "$COMPOSE_FILE" "$tmp"; then
    rm -f "$tmp"
    return 1
  fi
  # 只用 .new 一个文件校验，避免多个 -f 被 compose 合并
  local cargs=(--project-directory "$DEPLOY_DIR" -f "$tmp")
  [ -n "$PROJECT" ] && cargs+=(-p "$PROJECT")
  if ! docker compose "${cargs[@]}" config -q >>"${LOG_FILE:-/dev/null}" 2>&1; then
    rm -f "$tmp"
    return 1
  fi
  mv -f "$tmp" "$COMPOSE_FILE" || { rm -f "$tmp"; return 1; }
  return 0
}

app_container() {
  compose ps -q app 2>/dev/null | head -1
}

mysql_container() {
  compose ps -q mysql 2>/dev/null | head -1
}

recreate_app() {
  local q="" a
  for a in "${COMPOSE_ARGS[@]}"; do q+="$(printf '%q' "$a") "; done
  run_sh "只重建 app 容器（不动 mysql）" "docker compose ${q}up -d --no-deps app"
}

app_http_port() {
  local addr
  addr="$(docker inspect "$1" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | sed -n 's/^HTTP_ADDR=//p' | head -1)"
  [ -n "$addr" ] || addr="0.0.0.0:8090"
  printf '%s' "${addr##*:}"
}

# 在容器内自检 /api/health；打印 version
health_once() { # <container> <path>
  local c="$1" path="$2" port body
  port="$(app_http_port "$c")"
  body="$(docker exec "$c" wget -qO- --timeout=3 "http://127.0.0.1:${port}${path}" 2>/dev/null)" || return 1
  printf '%s' "$body" | python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
except Exception:
    sys.exit(1)
print(d.get("version",""))' 2>/dev/null
}

wait_healthy() { # <container> <path> <expect_version> <timeout>
  local c="$1" path="$2" expect="$3" timeout="$4" deadline v
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if v="$(health_once "$c" "$path")" && [ -n "$v" ]; then
      if [ "$v" = "$expect" ]; then
        printf '%s' "$v"
        return 0
      fi
      LAST_SEEN_VERSION="$v"
    fi
    sleep 2
  done
  return 1
}

brand_version() {
  sql_scalar "SELECT value FROM settings WHERE \`key\`='brand_version'" 2>/dev/null
}

app_restart_count() {
  docker inspect "$1" --format '{{.RestartCount}}' 2>/dev/null
}

app_log_scan() { # 只做告警，不作为回滚依据（业务日志里也有 ERROR 字样）
  docker logs --tail 200 "$1" 2>&1 | grep -E 'PANIC|FATAL|\[ERROR\]' | head -20
}

# 由当前 compose 里的 image 推出目标引用。三种写法都保持用户原风格：
#   cyber-drill:0.8.9                        → cyber-drill:<to>（本地）
#   ghcr.io/acme/cyber-drill:0.8.9           → <清单仓库>:<to>（registry）
#   ghcr.io/acme/cyber-drill:0.8.9@sha256:…  → 同写法 + 清单里该版本的 digest

# 由当前镜像名推断它属于清单里的哪个仓库，用于取对应仓库的 digest
registry_for_ref() { # <repo-without-tag>
  local name="$1" g s
  g="$(mf_registry_base ghcr)"
  s="$(mf_registry_base swr)"
  if [ -n "$g" ] && [ "$name" = "$g" ]; then printf '%s' ghcr; return 0; fi
  if [ -n "$s" ] && [ "$name" = "$s" ]; then printf '%s' swr; return 0; fi
  printf '%s' "$REGISTRY"
}

compute_target_ref() {
  local cur="${1:-$CURRENT_REF}" ref dg="" name tag last base
  ref="$cur"
  case "$ref" in
    *@sha256:*) dg="${ref#*@}"; ref="${ref%%@*}" ;;
  esac
  name="$ref"; last="${ref##*/}"
  if [ "${last#*:}" != "$last" ]; then name="${ref%:*}"; tag="${ref##*:}"; fi

  local is_registry=0
  case "$name" in
    */*) case "${name%%/*}" in *.*|*:*) is_registry=1 ;; esac ;;
  esac

  if [ -n "$IMAGE_REF_OPT" ]; then
    TARGET_REF="$IMAGE_REF_OPT"
    TARGET_IS_LOCAL=0
    case "${IMAGE_REF_OPT%%@*}" in
      */*) case "$(printf '%s' "${IMAGE_REF_OPT%%@*}" | cut -d/ -f1)" in *.*|*:*) TARGET_IS_LOCAL=0 ;; *) TARGET_IS_LOCAL=1 ;; esac ;;
      *) TARGET_IS_LOCAL=1 ;;
    esac
    info "目标镜像（--image-ref 指定）：$TARGET_REF"
    return 0
  fi

  if [ -n "$REGISTRY_GIVEN" ]; then
    # 用户显式 --registry：按清单里的仓库地址改写（切到官方仓库/镜像站）
    base="$(mf_registry_base "$REGISTRY")"
    [ -n "$base" ] || fail "$E_PRECHECK" "P0-precheck" "--registry $REGISTRY" "清单里没有该仓库" "改用 --registry ghcr 或 swr"
    TARGET_REF="$base:$TO_VER"
    TARGET_IS_LOCAL=0
  elif [ "$is_registry" = 1 ]; then
    # 保留用户自己的仓库地址（可能是私有镜像站），只换 tag
    TARGET_REF="$name:$TO_VER"
    TARGET_IS_LOCAL=0
  else
    TARGET_REF="$name:$TO_VER"
    TARGET_IS_LOCAL=1
  fi

  if [ -n "$dg" ] || [ "$PIN_DIGEST" = 1 ]; then
    local td reg
    if [ -n "$REGISTRY_GIVEN" ]; then reg="$REGISTRY"; else reg="$(registry_for_ref "$name")"; fi
    td="$(mf_digest "$TO_VER" "$reg")"
    if [ -n "$td" ] && [ "$td" != "null" ]; then
      TARGET_REF="$TARGET_REF@$td"
      info "目标镜像按 $reg 清单 digest 固定"
    elif [ -n "$dg" ]; then
      # 当前 compose 是 digest 固定写法：必须剥离旧 digest，否则改 tag 后仍钉死旧镜像
      warn "当前 compose 用 @sha256 固定，但清单里 $TO_VER 没有 $reg 的 digest"
      warn "已剥离旧 digest，目标改为 tag 写法：$TARGET_REF（本次升级后不再固定 digest）"
    else
      warn "清单里 $TO_VER 没有 $reg 的 digest，忽略 --pin-digest"
    fi
  fi
  info "目标镜像：$TARGET_REF"
}
