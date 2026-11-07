#!/bin/sh
# =============================================================================
# Drill System —— 容器入口脚本（干净版）
#
# 三步流水线：
#   1. 等待 MySQL 就绪（DB_HOST / DB_PORT，超时明确）
#   2. 执行 migrations（drill-migrate up，幂等）
#   3. exec 主程序（PID 1 由 tini 托管）
#
# 设计原则：
#   - 零 Go 运行时依赖 —— 只用 alpine busybox sh + wget
#   - 凭据零预置 —— migrate / drill-server 启动时各自严格校验
#     （缺失 DB_USER / JWT_SECRET / SESSION_SECRET 会 os.Exit(2)）
#   - 幂等 —— 任何阶段失败都可以重跑
# =============================================================================

set -e

APP_ROOT="${APP_ROOT:-/opt/drill}"
BIN_DIR="${APP_ROOT}/bin"

# ---------- 日志 ----------
log_info()  { printf '[entrypoint] INFO  %s\n' "$*"; }
log_warn()  { printf '[entrypoint] WARN  %s\n' "$*" >&2; }
log_error() { printf '[entrypoint] ERROR %s\n' "$*" >&2; }

# =============================================================================
# 阶段 1：等待 MySQL 就绪
# =============================================================================
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_WAIT_TIMEOUT="${DB_WAIT_TIMEOUT:-90}"

log_info "等待 MySQL ${DB_HOST}:${DB_PORT} 就绪（超时 ${DB_WAIT_TIMEOUT}s）…"

wait_mysql() {
    local deadline=$(( $(date +%s) + DB_WAIT_TIMEOUT ))
    local attempt=0

    while [ $(( $(date +%s) )) -lt $deadline ]; do
        attempt=$((attempt + 1))

        # 用 wget 探测 TCP 端口是否可达 —— MySQL 不是 HTTP server，
        # 所以 wget 会拿到 "HTTP 400 Bad Request" 后 exit，但 TCP 连接
        # 已经成功建立。如果 MySQL 根本没起来，exit code 会是 4
        # （Network failure / Connection refused）。
        #
        # 技巧：用 timeout 包裹防止偶尔卡死
        local rc=0
        (timeout 2 wget -q -O /dev/null --timeout=1 \
            "http://${DB_HOST}:${DB_PORT}/" >/dev/null 2>&1) || rc=$?

        # rc 含义：
        #   0  = 成功拿到 HTTP 响应（MySQL 会发握手给 HTTP client 后断开）
        #   4  = 网络层失败（TCP 连不上 / Connection refused / timeout）
        # 其他（如 8 = Server issued an error response 即 HTTP 4xx/5xx）
        #    代表 TCP 已经通了 —— 就把它当成 MySQL 已就绪
        if [ "$rc" -ne 4 ]; then
            log_info "MySQL ${DB_HOST}:${DB_PORT} TCP 连接可达（attempt=${attempt}, rc=${rc}）"
            return 0
        fi

        if [ $(( attempt % 5 )) -eq 0 ]; then
            log_info "  等待中…（attempt=${attempt}）"
        fi
        sleep 2
    done

    log_error "MySQL ${DB_HOST}:${DB_PORT} 在 ${DB_WAIT_TIMEOUT}s 内未就绪"
    log_error "请检查："
    log_error "  - docker compose 中 mysql 服务是否 healthy"
    log_error "  - DB_HOST / DB_PORT 环境变量是否正确"
    log_error "  - 网络是否可达（compose 内用 mysql 作主机名）"
    return 1
}

if ! wait_mysql; then
    exit 1
fi

# =============================================================================
# 阶段 2：执行 migrations（幂等）
# =============================================================================
MIGRATE_BIN="${BIN_DIR}/drill-migrate"

if [ -x "${MIGRATE_BIN}" ]; then
    log_info "执行 migrations（drill-migrate up）…"
    if "${MIGRATE_BIN}" -root="${APP_ROOT}" up; then
        log_info "migrations 完成"
    else
        rc=$?
        log_error "migrations 执行失败（exit=${rc}）"
        log_error "如果错误是 '配置缺失或未替换占位符' —— 请检查 .env / 环境变量"
        exit "${rc}"
    fi
else
    log_warn "迁移器缺失（${MIGRATE_BIN}），跳过迁移"
    log_warn "若为已有数据库，请确认 schema_migrations 已是最新；首次部署必须先补齐 drill-migrate，否则应用会因表不存在而启动失败"
fi

# =============================================================================
# 阶段 2.5：幂等补齐系统配置（角色 / 管理员账号）
#   - 只增不删：绝不 DELETE 角色、剧本、流程字典
#   - 单事务：失败回滚，旧数据原样保留
#   - 已存在账号不改密码，只补缺失
# =============================================================================
SEED_BIN="${BIN_DIR}/drill-seed"

if [ -x "${SEED_BIN}" ]; then
    log_info "执行配置种子（drill-seed，只增不删）…"
    if "${SEED_BIN}" -root="${APP_ROOT}"; then
        log_info "配置种子完成"
    else
        rc=$?
        log_error "配置种子执行失败（exit=${rc}，已回滚，旧数据保留）"
        log_error "首次部署请检查 drill.env 中的数据库/密钥配置与管理员初始口令"
        exit "${rc}"
    fi
else
    log_warn "${SEED_BIN} 不存在或不可执行，跳过配置种子"
fi

# =============================================================================
# 阶段 3：exec 主程序（tini 托管 → PID 1，正确转发 SIGTERM）
# =============================================================================
log_info "启动 Drill Server …"
log_info "APP_ROOT=${APP_ROOT}  HTTP 监听=${HTTP_ADDR:-0.0.0.0:8090}"

exec "$@"
