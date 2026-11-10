#!/usr/bin/env bash
# =============================================================================
# 10-manifest.sh —— 版本清单读取与链校验
#
# 依赖：python3（清单是 JSON；不做脆弱的自研解析）
# 清单是唯一事实来源，随 deploy/upgrade/ 一起发布；脚本不联网发现版本。
# =============================================================================

MANIFEST=""

manifest_init() {
  MANIFEST="${MANIFEST_OVERRIDE:-$SCRIPT_DIR/manifest.json}"
  [ -f "$MANIFEST" ] || fail "$E_PRECHECK" "P0-precheck" "读取清单 $MANIFEST" "版本清单不存在" "确认 deploy/upgrade/manifest.json 与 upgrade.sh 一起被拷贝"
  command -v python3 >/dev/null 2>&1 || fail "$E_PRECHECK" "P0-precheck" "python3 -V" "缺少 python3，无法解析版本清单" "安装 python3（Debian/Ubuntu: apt install python3；RHEL: dnf install python3）后重跑"
  local sv
  sv="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["schema_version"])' "$MANIFEST" 2>/dev/null)" \
    || fail "$E_PRECHECK" "P0-precheck" "解析 $MANIFEST" "清单 JSON 语法错误" "修正 manifest.json 后重跑"
  [ "$sv" = "1" ] || fail "$E_PRECHECK" "P0-precheck" "schema_version=$sv" "清单版本不受支持（需要 1）" "升级 upgrade.sh 与 manifest.json 到同一版本"
}

mf_has() {
  python3 - "$MANIFEST" "$1" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
sys.exit(0 if any(r.get("version")==sys.argv[2] for r in d["releases"]) else 1)
PY
}

# mf_get <version> <dot.path>；不存在输出空
mf_get() {
  python3 - "$MANIFEST" "$1" "$2" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
r=[x for x in d["releases"] if x.get("version")==sys.argv[2]]
if not r:
    sys.exit(0)
cur=r[0]
for p in sys.argv[3].split("."):
    if p=="":
        continue
    if not isinstance(cur,dict) or p not in cur:
        sys.exit(0)
    cur=cur[p]
if cur is None:
    print("null")
elif isinstance(cur,bool):
    print("true" if cur else "false")
elif isinstance(cur,(dict,list)):
    print(json.dumps(cur,ensure_ascii=False))
else:
    print(cur)
PY
}

mf_versions() {
  python3 -c 'import json,sys
for r in json.load(open(sys.argv[1]))["releases"]:
    print(r["version"])' "$MANIFEST"
}

# 由 schema_migrations 行数反查版本（用于无法从 health 探测时）
mf_version_by_total() {
  python3 - "$MANIFEST" "$1" <<'PY'
import json,sys
n=int(sys.argv[2])
for r in json.load(open(sys.argv[1]))["releases"]:
    if int(r.get("schema_migrations_total",-1))==n:
        print(r["version"]); break
PY
}

# 注册表基础地址（不含 tag）
mf_registry_base() {
  python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["registries"].get(sys.argv[2],""))' "$MANIFEST" "$1"
}

# mf_digest <version> <registry>：打印 sha256:xxx 或空
mf_digest() {
  python3 - "$MANIFEST" "$1" "$2" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
r=[x for x in d["releases"] if x.get("version")==sys.argv[2]]
if not r: sys.exit(0)
dg=(r[0].get("image") or {}).get("digest") or {}
v=dg.get(sys.argv[3])
print(v if v else "")
PY
}

# mf_chain <from> <to>
# 输出：每行 "<version>|<schema_total>|<through>|<from_exclusive>|<count>|<ddl>|<compat>|<channel>|<best_effort>"
# 失败：stderr 打中文原因，退出 11
mf_chain() {
  python3 - "$MANIFEST" "$1" "$2" "$FORCE_CHAIN" <<'PY'
import json,sys
path, frm, to, force = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]=="1"
d=json.load(open(path))
rel={r["version"]:r for r in d["releases"]}

known="、".join(r["version"] for r in d["releases"])
def die(msg):
    sys.stderr.write(msg+"\n"); sys.exit(11)

if frm not in rel:
    die("[ERROR] 当前版本 %s 不在清单内\n        清单内已知版本：%s\n        可用做法：确认版本号是否正确；或升级升级工具到包含该版本的版本" % (frm, known))
if to not in rel:
    die("[ERROR] 目标版本 %s 不在清单内\n        清单内已知版本：%s\n        可用做法：检查 --to 拼写；或升级升级工具" % (to, known))

# 降级检测：from 沿 previous 往上能遇到 to，说明 to 是更老的版本
anc=frm
for _ in range(50):
    if anc==to:
        die("[ERROR] 不支持降级：%s 是 %s 的前置版本\n        可用做法：如需回退请恢复升级前的整库备份（--recover-last / 手工步骤见 docs/upgrade.md），不要直接降级镜像" % (to, frm))
    e=rel.get(anc)
    anc=e.get("previous") if e else None
    if not anc:
        break

chain=[]
cur=to
guard=0
while cur!=frm:
    guard+=1
    if guard>50:
        die("[ERROR] 版本链出现环（超过 50 跳），已停止")
    e=rel.get(cur)
    if e is None:
        if force:
            # 强行模式：跳过清单里没有的版本，沿用已知链继续
            chain_missing=cur
            cur=None
            break
        die("[ERROR] 版本链不完整，已停止（未做任何变更）\n        请求路径：%s → %s\n        缺失环节：%s\n        清单内已知版本：%s\n        可用做法：1) 升级升级工具；2) 联系支持，人工确认后使用 --force-chain（日志留痕）" % (frm,to,cur,known))
    prev=e.get("previous")
    if prev is None:
        die("[ERROR] 版本链在 %s 处断开（该条 previous=null）\n        请求路径：%s → %s\n        清单内已知版本：%s" % (cur,frm,to,known))
    chain.insert(0,e)
    cur=prev

if not force and chain:
    # 连续性：首跳的 from_exclusive 必须等于 from 的 through
    ft=rel[frm].get("migrations",{}).get("through","")
    if chain[0].get("migrations",{}).get("from_exclusive","")!=ft:
        die("[ERROR] 版本链不连续：%s 的区间起点是 %s，但 %s 的终点是 %s\n        可用做法：清单数据有误，请修正 manifest.json" % (
            chain[0]["version"], chain[0].get("migrations",{}).get("from_exclusive",""), frm, ft))
    for i in range(1,len(chain)):
        a=chain[i-1].get("migrations",{}).get("through","")
        b=chain[i].get("migrations",{}).get("from_exclusive","")
        if a!=b:
            die("[ERROR] 版本链缺环：%s 的终点是 %s，但 %s 的起点是 %s\n        清单内已知版本：%s" % (
                chain[i-1]["version"], a, chain[i]["version"], b, known))

for e in chain:
    m=e.get("migrations",{})
    print("%s|%s|%s|%s|%s|%s|%s|%s|%s" % (
        e["version"], e.get("schema_migrations_total"), m.get("through",""),
        m.get("from_exclusive",""), m.get("count",0),
        "true" if m.get("contains_ddl") else "false",
        "true" if m.get("backward_compatible") else "false",
        e.get("channel","stable"),
        "true" if e.get("best_effort") else "false"))
PY
}

chain_requires_downtime() { # 链上任一版本含 DDL 或声明不兼容 → 需要停服迁移
  local line
  while IFS='|' read -r _v _total _thru _from _cnt ddl compat _ch _be; do
    [ "$ddl" = "true" ] && return 0
    [ "$compat" = "false" ] && return 0
  done <<<"$CHAIN_LINES"
  return 1
}
