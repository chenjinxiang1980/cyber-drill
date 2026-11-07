#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# drill-server —— 幂等安装脚本（systemd + 非 root 用户运行）
#
# 用法：sudo bash install.sh
#   - 自动创建 drill 用户、目录结构、拷贝二进制、安装 systemd 单元
#   - 自动 daemon-reload，但 **不** 自动 start（等你放好 .env 和证书再启动）
#   - 脚本可重复执行（用户/目录/二进制已存在时会覆盖更新、不报错）
#
# 前置：
#   - 本脚本需 root 权限执行（创建用户、写 /etc/systemd/system、/usr/local/bin）
#   - 同目录下需有 drill-server 二进制（ELF64 x86_64）
# -----------------------------------------------------------------------------
set -euo pipefail

# 固定 umask，避免 cp/mkdir 在严格 umask（027）下生成 640/750：
# 静态资源需对容器 uid 10001 可读（bind mount 时容器用户不是文件属主）。
umask 022

# ---------- 可调参数（需要换路径/用户时改这里）----------
APP_NAME="drill-server"
APP_USER="drill"
APP_GROUP="drill"
APP_HOME="/opt/drill"
BIN_SRC_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="${BIN_SRC_DIR}/${APP_NAME}"
BIN_DST="${APP_HOME}/bin/${APP_NAME}"
SYSTEMD_UNIT_NAME="${APP_NAME}.service"
SYSTEMD_UNIT_SRC="${BIN_SRC_DIR}/${SYSTEMD_UNIT_NAME}"
SYSTEMD_UNIT_DST="/etc/systemd/system/${SYSTEMD_UNIT_NAME}"
ENV_FILE="/etc/drill/drill.env"
CERTS_DIR="/etc/drill/certs"
LOG_DIR="${APP_HOME}/logs"
RUN_DIR="${APP_HOME}/run"

# ---------- 颜色输出 --------
info()  { printf '\033[1;34m[INFO]\033[0m  %s\n' "$*"; }
warn()  { printf '\033[1;33m[WARN]\033[0m  %s\n' "$*"; }
error() { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; }

# ---------- root 检查 --------
if [[ $EUID -ne 0 ]]; then
    error "本脚本需要 root 权限，请用 sudo bash install.sh 执行"
    exit 1
fi

# ---------- 二进制检查 --------
if [[ ! -f "${BIN_SRC}" ]]; then
    error "找不到 ${BIN_SRC}，请确认 install.sh 与 drill-server 在同一目录"
    exit 1
fi
if ! file "${BIN_SRC}" | grep -qi 'ELF'; then
    warn "源文件 ${BIN_SRC} 看起来不是 ELF，继续但请确认架构正确"
fi

info "==> 安装目标: ${APP_NAME}  →  ${APP_HOME}  (用户: ${APP_USER})"

# ---------- 1. 创建非 root 用户 --------
if ! id -u "${APP_USER}" &>/dev/null; then
    info "创建系统用户 ${APP_USER}（no-login、无家目录）"
    useradd --system --no-create-home --shell /usr/sbin/nologin \
            --user-group "${APP_USER}"
else
    info "用户 ${APP_USER} 已存在，跳过创建"
fi

# ---------- 2. 目录结构 --------
info "创建目录结构"
mkdir -p "${APP_HOME}/bin" "${APP_HOME}/public" "${APP_HOME}/data" \
         "${APP_HOME}/uploads" "${APP_HOME}/migrations" \
         "${LOG_DIR}" "${RUN_DIR}" \
         "$(dirname "${ENV_FILE}")" "${CERTS_DIR}"

# ---------- 3. 拷贝二进制 --------
info "拷贝 ${APP_NAME} → ${BIN_DST}"
cp -f "${BIN_SRC}" "${BIN_DST}"
chmod 0755 "${BIN_DST}"

# ---------- 4. 可选：同步 public/ / migrations（如果当前目录有）----------
PUBLIC_SRC="${BIN_SRC_DIR}/../../public"
MIG_SRC="${BIN_SRC_DIR}/../../migrations"

if [[ -d "${PUBLIC_SRC}" ]]; then
    info "同步 public/ → ${APP_HOME}/public"
    # 只更新更新时间比目标新的文件，保留用户可能的本地修改
    cp -ru "${PUBLIC_SRC}/." "${APP_HOME}/public/"
    # 固化静态资源权限：容器 uid 10001 非文件属主，需 other-read 才能返回页面。
    find "${APP_HOME}/public" -type d -exec chmod 0755 {} +
    find "${APP_HOME}/public" -type f -exec chmod 0644 {} +
else
    warn "未找到 public/ 源目录（${PUBLIC_SRC}），请手动把前端拷到 ${APP_HOME}/public"
fi

if [[ -d "${MIG_SRC}" ]]; then
    info "同步 migrations/ → ${APP_HOME}/migrations"
    cp -ru "${MIG_SRC}/." "${APP_HOME}/migrations/"
else
    warn "未找到 migrations/ 源目录（${MIG_SRC}），请手动拷贝"
fi

# ---------- 5. 修正目录属主（让 drill 用户能写 data/uploads）----------
info "修正目录属主（drill:drill）"
chown -R "${APP_USER}:${APP_GROUP}" "${APP_HOME}"
# /etc/drill 由 root 拥有，但 drill 需要读 env 和 certs
chown -R root:root "$(dirname "${ENV_FILE}")" "${CERTS_DIR}"

# ---------- 6. 安装 systemd 单元 --------
if [[ -f "${SYSTEMD_UNIT_SRC}" ]]; then
    info "安装 systemd 单元 → ${SYSTEMD_UNIT_DST}"
    cp -f "${SYSTEMD_UNIT_SRC}" "${SYSTEMD_UNIT_DST}"
    chmod 0644 "${SYSTEMD_UNIT_DST}"
    systemctl daemon-reload
    info "systemd daemon-reload 完成"
else
    warn "未找到 ${SYSTEMD_UNIT_SRC}，跳过 unit 安装。请手动放置后执行 daemon-reload"
fi

# ---------- 7. 权限安全提示 --------
info ""
info "========================================== 安装完成 =========================================="
info ""
info "下一步（按顺序）："
info "  1) 放环境变量文件："
info "       sudo cp drill.env.example ${ENV_FILE}"
info "       sudo ${EDITOR:-vi} ${ENV_FILE}"
info "     ⚠️  务必替换所有 __REPLACE_WITH__ 占位符，尤其是："
info "        DB_USER / DB_PASSWORD / DB_ROOT_* / JWT_SECRET / SESSION_SECRET"
info "     ⚠️  证书文件也放 ${CERTS_DIR}/，确保 drill 用户可读："
info "        sudo chown -R root:${APP_GROUP} ${CERTS_DIR}"
info "        sudo chmod 0750 ${CERTS_DIR}"
info "        sudo chmod 0640 ${CERTS_DIR}/*"
info ""
info "  2) 初始化数据库（首次部署）："
info "       # 先起 MySQL（容器或物理机都行），然后在项目根执行："
info "       go run ./server/cmd/bootstrap -root=/opt/drill"
info "       # 或用我们提供的 Dockerfile 里的 bootstrap 阶段镜像"
info ""
info "  3) 启动服务："
info "       sudo systemctl enable --now ${APP_NAME}"
info "       sudo systemctl status ${APP_NAME}"
info "       sudo journalctl -u ${APP_NAME} -f"
info ""
info "  4) 健康检查："
info "       curl http://127.0.0.1:8090/api/health"
info ""
info "  5) （可选）配置 Nginx / Caddy 反向代理 + HTTPS："
info "       nginx-drill.conf / Caddyfile.example 同目录可用"
info ""
info "  6) 权限说明："
info "       ${ENV_FILE}    : root:root 0640  — 含口令，不能让其他用户读"
info "       ${CERTS_DIR}/  : root:drill 0750 / 文件 0640 — drill 进程需读"
info "       ${APP_HOME}/data/uploads : drill:drill 0755 — 运行时写"
info "       ${APP_HOME}/logs    : drill:drill 0755 — systemd journal 替代"
info ""
info "================================ 脚本已全部幂等，可重复执行 ==================================="
