#!/usr/bin/env bash
# =============================================================================
# 基线快照构建（全新安装用）
#
# 作用：从指定 1.0.0 库导出“结构 + 参照数据”，脱敏后打包成可恢复的快照，
#       并生成固定哈希与 manifest；不含任何运营数据和现网账号口令。
#
# 迁移与安装口径（定稿）：
#   全新安装只恢复本脚本产出的基线快照，不重放 migrations/ 迁移链；
#   迁移链只服务 deploy/upgrade 的升级路径。
#   → 新增迁移不会自动进入全新安装：必须重跑本脚本生成新基线，
#     并同步 manifest.env（BASELINE_MIGRATIONS / BASELINE_SCRIPT_NODES 等）、
#     SHA256SUMS 与本脚本内的 EXPECT_* 常量，新装库才会包含该迁移的结果。
#   → 不得为了通过历史迁移守卫在空库/快照里补造假数据。
#
# 快照内容：
#   结构      : 全部表（CREATE TABLE，去掉 AUTO_INCREMENT 计数器）
#   参照数据  : settings / roles / scenarios / script_nodes / form_templates /
#               dict / knowledge / score_items / schema_migrations
#   明确排除  : users / user_roles / projects / tasks / form_submissions / evidence /
#               audit_logs / sessions / messages / notifs / 及所有上传文件
#
# 用法：
#   # 1) 本地能连源库
#   bash build-baseline.sh --mysql-host <source-host> --mysql-user root --mysql-password '***' \
#        --mysql-db drill_system --out ./baseline --version 1.0.0
#
#   # 2) 已在别处导出合并 SQL（本脚本只做脱敏/校验/打包）
#   bash build-baseline.sh --raw-sql ./drill-1.0.0-raw.sql --out ./baseline --version 1.0.0
#
#   # 打印在源库主机上生成 raw SQL 的命令（容器部署场景）
#   bash build-baseline.sh --print-dump-cmd --mysql-db drill_system
#
# 产物：
#   baseline/drill-baseline-<版本>.sql.gz
#   baseline/SHA256SUMS
#   baseline/manifest.env
# =============================================================================
set -Eeuo pipefail

VERSION="1.0.0"
OUT="./baseline"
RAW_SQL=""
MYSQL_HOST="127.0.0.1"
MYSQL_PORT="3306"
MYSQL_USER="root"
MYSQL_PASSWORD=""
MYSQL_DB="drill_system"
KEEP_SQL=0
PRINT_DUMP_CMD=0

REF_TABLES=(settings roles scenarios script_nodes form_templates dict knowledge score_items schema_migrations)
FORBIDDEN_TABLES=(users user_roles projects tasks form_submissions evidence audit_logs sessions messages notifs chat_messages timeline ledger impact impact_events scores score_snapshots drill_steps drill_node_state snapshots script_revisions project_core_evidence project_end_exception project_end_seal opinion notif_reads task_attachments sim_data)

EXPECT_MIGRATIONS=88
EXPECT_BRAND="v1.0.0"
EXPECT_SCENARIOS=20
EXPECT_SCENARIOS_ACTIVE=20
EXPECT_ROLES=18
EXPECT_SCRIPT_NODES=1127
EXPECT_FORM_TEMPLATES=22
EXPECT_DICT=12
EXPECT_EVENT_TYPES=20

info() { printf '[信息] %s\n' "$*"; }
ok()   { printf '[完成] %s\n' "$*"; }
die()  { printf '[错误] %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="${2:-}"; shift ;;
        --out) OUT="${2:-}"; shift ;;
        --raw-sql) RAW_SQL="${2:-}"; shift ;;
        --mysql-host) MYSQL_HOST="${2:-}"; shift ;;
        --mysql-port) MYSQL_PORT="${2:-}"; shift ;;
        --mysql-user) MYSQL_USER="${2:-}"; shift ;;
        --mysql-password) MYSQL_PASSWORD="${2:-}"; shift ;;
        --mysql-db) MYSQL_DB="${2:-}"; shift ;;
        --keep-sql) KEEP_SQL=1 ;;
        --print-dump-cmd) PRINT_DUMP_CMD=1 ;;
        -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
        *) die "未知参数：$1" ;;
    esac
    shift
done

if [ "${PRINT_DUMP_CMD}" -eq 1 ]; then
    cat <<EOF
# 在源库主机（或能 docker exec 到 MySQL 容器的主机）执行，把输出保存为 raw.sql：
(
  echo "SET NAMES utf8mb4;"
  echo "SET FOREIGN_KEY_CHECKS=0;"
  mysqldump -h${MYSQL_HOST} -P${MYSQL_PORT} -u${MYSQL_USER} -p'<口令>' \\
    --single-transaction --skip-lock-tables --no-tablespaces --default-character-set=utf8mb4 \\
    --skip-comments --set-gtid-purged=OFF --no-create-db --skip-triggers --no-data ${MYSQL_DB}
  mysqldump -h${MYSQL_HOST} -P${MYSQL_PORT} -u${MYSQL_USER} -p'<口令>' \\
    --single-transaction --skip-lock-tables --no-tablespaces --default-character-set=utf8mb4 \\
    --skip-comments --set-gtid-purged=OFF --no-create-db --skip-triggers --no-create-info \\
    --skip-extended-insert --order-by-primary ${MYSQL_DB} ${REF_TABLES[*]}
  echo "SET FOREIGN_KEY_CHECKS=1;"
) > raw.sql
EOF
    exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# ---------- 取原始 SQL ----------
if [ -n "${RAW_SQL}" ]; then
    [ -f "${RAW_SQL}" ] || die "raw SQL 不存在：${RAW_SQL}"
    info "使用已有合并 SQL：${RAW_SQL}"
    cp -f "${RAW_SQL}" "${TMP}/raw.sql"
else
    command -v mysqldump >/dev/null 2>&1 || die "缺少 mysqldump；或改用 --raw-sql"
    info "从 ${MYSQL_HOST}:${MYSQL_PORT}/${MYSQL_DB} 导出"
    {
        echo "SET NAMES utf8mb4;"
        echo "SET FOREIGN_KEY_CHECKS=0;"
        MYSQL_PWD="${MYSQL_PASSWORD}" mysqldump -h"${MYSQL_HOST}" -P"${MYSQL_PORT}" -u"${MYSQL_USER}" \
            --single-transaction --skip-lock-tables --no-tablespaces --default-character-set=utf8mb4 \
            --skip-comments --set-gtid-purged=OFF --no-create-db --skip-triggers --no-data "${MYSQL_DB}"
        MYSQL_PWD="${MYSQL_PASSWORD}" mysqldump -h"${MYSQL_HOST}" -P"${MYSQL_PORT}" -u"${MYSQL_USER}" \
            --single-transaction --skip-lock-tables --no-tablespaces --default-character-set=utf8mb4 \
            --skip-comments --set-gtid-purged=OFF --no-create-db --skip-triggers --no-create-info \
            --skip-extended-insert --order-by-primary "${MYSQL_DB}" "${REF_TABLES[@]}"
        echo "SET FOREIGN_KEY_CHECKS=1;"
    } > "${TMP}/raw.sql" || die "mysqldump 导出失败"
fi

# ---------- 脱敏与稳定化 ----------
info "脱敏与稳定化"
# 去掉视图/触发器 DEFINER（防跨实例权限失败）
sed -i -E 's/DEFINER=`[^`]*`@`[^`]*`//g' "${TMP}/raw.sql"
# 结构快照不保留自增计数器，保证同一份数据产物稳定、且不泄露运营规模
sed -i -E 's/ AUTO_INCREMENT=[0-9]+//g' "${TMP}/raw.sql"
# 安装时间属于运营信息，固定为常量
sed -i -E "s/'initialized_at','[^']*'/'initialized_at','1970-01-01 00:00:00'/g" "${TMP}/raw.sql"

{
    echo "-- Cyber-Drill baseline snapshot ${VERSION}"
    echo "-- generated by deploy/install/build-baseline.sh (reference data only)"
    cat "${TMP}/raw.sql"
} > "${TMP}/baseline.sql"

# ---------- 校验 ----------
info "校验快照内容"
for t in "${FORBIDDEN_TABLES[@]}"; do
    if grep -qE "^INSERT INTO \`${t}\`" "${TMP}/baseline.sql"; then
        die "快照包含禁止的运营表数据：${t}"
    fi
done
if grep -qE '\$2[aby]\$[0-9]{2}\$' "${TMP}/baseline.sql"; then
    die "快照包含 bcrypt 口令散列"
fi
if grep -qE "^INSERT INTO .* VALUES \(\(" "${TMP}/baseline.sql"; then
    die "快照存在非法嵌套括号的 INSERT（SQL 语法校验失败）"
fi
for t in "${REF_TABLES[@]}"; do
    grep -qE "^INSERT INTO \`${t}\`|^CREATE TABLE \`${t}\`" "${TMP}/baseline.sql" \
        || die "快照缺少参照表：${t}"
done

MIG_ROWS="$(grep -cE "^INSERT INTO \`schema_migrations\`" "${TMP}/baseline.sql" || true)"
[ "${MIG_ROWS}" = "${EXPECT_MIGRATIONS}" ] || die "schema_migrations 行数期望 ${EXPECT_MIGRATIONS}，实际 ${MIG_ROWS}"
grep -q "'brand_version','${EXPECT_BRAND}'" "${TMP}/baseline.sql" \
    || die "settings.brand_version 不是 ${EXPECT_BRAND}"

SCEN_ROWS="$(grep -cE "^INSERT INTO \`scenarios\`" "${TMP}/baseline.sql" || true)"
[ "${SCEN_ROWS}" = "${EXPECT_SCENARIOS}" ] || die "scenarios 行数期望 ${EXPECT_SCENARIOS}，实际 ${SCEN_ROWS}"
# 停用剧本仍保留行（S20 并入 S01 后 status=停用），安装器只校验「在用」数量
SCEN_ACTIVE_ROWS=$((SCEN_ROWS - $(grep -E "^INSERT INTO \`scenarios\`" "${TMP}/baseline.sql" | grep -c "'停用'" || true)))
[ "${SCEN_ACTIVE_ROWS}" = "${EXPECT_SCENARIOS_ACTIVE}" ] || die "在用剧本数期望 ${EXPECT_SCENARIOS_ACTIVE}，实际 ${SCEN_ACTIVE_ROWS}"
ROLE_ROWS="$(grep -cE "^INSERT INTO \`roles\`" "${TMP}/baseline.sql" || true)"
[ "${ROLE_ROWS}" = "${EXPECT_ROLES}" ] || die "roles 行数期望 ${EXPECT_ROLES}，实际 ${ROLE_ROWS}"
FORM_ROWS="$(grep -cE "^INSERT INTO \`form_templates\`" "${TMP}/baseline.sql" || true)"
[ "${FORM_ROWS}" = "${EXPECT_FORM_TEMPLATES}" ] || die "form_templates 行数期望 ${EXPECT_FORM_TEMPLATES}，实际 ${FORM_ROWS}"
DICT_ROWS="$(grep -cE "^INSERT INTO \`dict\`" "${TMP}/baseline.sql" || true)"
[ "${DICT_ROWS}" = "${EXPECT_DICT}" ] || die "dict 行数期望 ${EXPECT_DICT}，实际 ${DICT_ROWS}"
NODE_ROWS="$(grep -cE "^INSERT INTO \`script_nodes\`" "${TMP}/baseline.sql" || true)"
# 1211 行含软删除；可见口径 1127 由安装器在恢复后用 SQL 校验
[ "${NODE_ROWS}" -ge "${EXPECT_SCRIPT_NODES}" ] || die "script_nodes 行数不足 ${EXPECT_SCRIPT_NODES}（实际 ${NODE_ROWS}）"

EVENT_TYPES="$(grep -E "^INSERT INTO \`dict\`" "${TMP}/baseline.sql" | grep "eventTypes" | head -n1 | grep -o '\\"' | wc -l)"
# 每项两侧各有转义引号，数量应为 2*N
EVENT_TYPES=$((EVENT_TYPES / 2))
[ "${EVENT_TYPES}" = "${EXPECT_EVENT_TYPES}" ] || die "dict.eventTypes 期望 ${EXPECT_EVENT_TYPES} 项，实际 ${EVENT_TYPES}"

# ---------- 打包 ----------
mkdir -p "${OUT}"
SQL_NAME="drill-baseline-${VERSION}.sql"
GZ_NAME="${SQL_NAME}.gz"
cp -f "${TMP}/baseline.sql" "${OUT}/${SQL_NAME}"
gzip -9 -n -c "${OUT}/${SQL_NAME}" > "${OUT}/${GZ_NAME}"
SHA="$(sha256sum "${OUT}/${GZ_NAME}" | awk '{print $1}')"

cat > "${OUT}/manifest.env" <<EOF
BASELINE_VERSION=${VERSION}
BASELINE_FILE=${GZ_NAME}
BASELINE_SHA256=${SHA}
BASELINE_MIGRATIONS=${MIG_ROWS}
BASELINE_BRAND_VERSION=${EXPECT_BRAND}
BASELINE_SCENARIOS=${SCEN_ACTIVE_ROWS}
BASELINE_SCENARIOS_ROWS=${SCEN_ROWS}
BASELINE_ROLES=${ROLE_ROWS}
BASELINE_SCRIPT_NODES=${EXPECT_SCRIPT_NODES}
BASELINE_FORM_TEMPLATES=${FORM_ROWS}
BASELINE_DICT=${DICT_ROWS}
BASELINE_EVENT_TYPES=${EVENT_TYPES}
BASELINE_SCRIPT_NODES_ROWS=${NODE_ROWS}
BASELINE_EXCLUDED=users,user_roles,projects,tasks,form_submissions,evidence,audit_logs,sessions,messages,notifs,impact,scores,sim_data
BASELINE_CREATED_AT="$(date '+%Y-%m-%d %H:%M:%S')"
EOF
(cd "${OUT}" && sha256sum "${GZ_NAME}" > SHA256SUMS)
if [ "${KEEP_SQL}" -eq 0 ]; then rm -f "${OUT}/${SQL_NAME}"; fi

ok "基线快照：${OUT}/${GZ_NAME}"
printf '  SHA256      : %s\n' "${SHA}"
printf '  迁移条数    : %s\n' "${MIG_ROWS}"
printf '  剧本/节点/表单/字典 : %s / %s(可见 %s) / %s / %s\n' \
    "${SCEN_ROWS}" "${NODE_ROWS}" "${EXPECT_SCRIPT_NODES}" "${FORM_ROWS}" "${DICT_ROWS}"
printf '  事件类型    : %s\n' "${EVENT_TYPES}"
