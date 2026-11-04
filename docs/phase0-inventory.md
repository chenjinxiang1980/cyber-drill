# Phase 0 · 接口契约与迁移清单

> 只读盘点产物。基准版本：2026-11-02，Node/Express + SQLite 后端。Go + Gin 复刻前请以本文档为唯一契约源。

---

## 0. 摘要（Exec Summary）

| 指标 | 数值 |
| --- | --- |
| 路由文件 | 8 个（auth / bootstrap / content / drill / tasks / admin / evidence / system） |
| HTTP 接口 | ~78 个（GET ~40 / POST ~25 / PATCH ~6 / DELETE ~5 / PUT ~2） |
| SQLite 表 | 28 张 |
| migrate() 运行时补列 | 32 列（含 DROP+CREATE impact 联合主键） |
| 内置角色 | 10 类（platformadmin / commander / itlead / duty / netsec / sysadmin / medlead / clinical / healthbureau / cac / cyberpolice） |
| 前端 API 方法数 | 90+（全部走 /api/* 统一前缀） |

**最高风险 5 项**（Go 复刻时最容易悄悄丢掉或改变语义）：

1. **`score()` 扣分制 + 混淆节点下发时机（before/after）**：`src/routes/drill.js` 的核心评分函数 ~400 行，`isDistractorReleased()` 控制混淆节点可见性，`nodeLock()` 锁定依赖。三函数口径必须完全一致，Go 版若任何一处改判定条件都会导致评分错误。
2. **migrate() 的 impact 联合主键 DROP+CREATE**：把全局 `project_id=''` 模板复制到每个项目，Go 版建表必须直接按最终联合主键设计，不能等迁移再补；但补数据的逻辑需要保留（新建项目时复制模板）。
3. **`/content/monitor`（大屏）和 `/content/dashboard` 都调用 `require('./drill').score(p.id)` 复用评分**：Go 版若分别实现会导致两处结果不一致。必须抽成一个纯函数。
4. **session 令牌来源三重**：`Authorization: Bearer X` / `X-Drill-Token: X` / query `?t=X`（文件下载场景用 query token 绕过 header 限制）。Go 中间件要同时支持这三种。
5. **F1/F7 表单提交自动联动 impact 状态**：tasks.js 里的两处联动 + 别名映射字典是硬编码的业务规则，Go 版必须保留。

**Go 复刻前必须先确认的前提**（否则写一半返工）：

- SQLite 迁移路径：**Go 版是从零建全新数据库（直接按最终 schema），还是需要兼容现有 .db 文件做升级？** 这决定了 migrate() 里 30+ 补列是全部写进 DDL，还是只保留数据迁移脚本。
- 密码散列算法是否保持 `scrypt`（Go 有 golang.org/x/crypto/scrypt，纯 Go 实现，兼容 Node 输出）。
- 上传目录结构 `uploads/{kind}/{YYYYMM}/xxx`、禁止扩展名列表、MAX_MB=20 是否保持一致。
- 时间格式统一 `YYYY-MM-DD HH:mm[:ss]`，本地时区（不做 UTC），Go 版要用 `time.Local`。

---

## 1. 数据模型

### 1.1 表清单（28 张，schema.sql 权威）

| # | 表名 | 主键 | 关键字段 | 说明 |
| --- | --- | --- | --- | --- |
| 1 | roles | `id TEXT PK` | name, short, icon, color, descr, nav(JSON), external(0/1), status, seq | 角色定义，nav 为 JSON 数组（模块权限） |
| 2 | users | `id INTEGER PK AUTOINCREMENT` | username UNIQUE, name, role_id, org, pwd_salt, pwd_hash, must_change(0/1), status | 主角色 role_id 冗余（user_roles 表才是真源） |
| 3 | sessions | `token TEXT PK` | user_id, role_id, created_at, expires_at | 令牌会话，480min 过期 |
| 4 | user_roles | `(user_id, role_id) PK` | | 多角色绑定 |
| 5 | projects | `id TEXT PK` (P2026-001) | name, type, date, scenario_id, level, status(6态), target, units(JSON), members(JSON), observers(JSON), mode(基础版/实战版), started_at, paused_at, paused_sec, ended_at | 演练项目 |
| 6 | scenarios | `id TEXT PK` (S01-S08) | name, type, level, difficulty, status, version, updated, nodes, descr, alert | 剧本场景 |
| 7 | script_nodes | `id TEXT PK` (S01-01, DS01-duty-1) | scenario_id, type, title, short_title(≤6字), target, timeout(分钟), form, att, descr, branches(JSON), seq, guide, keywords, action, wrong(JSON), evidence_req(JSON), required(0/1), flow_type(correct/distractor), score, receiver, deps(JSON), pos_x, pos_y, anchor_id, release_timing(before/after), **parallel_desc** | 剧本节点，correct 为主流，distractor 为混淆 |
| 8 | drill_steps | `id INTEGER PK` | project_id, scenario_id, role_id, user, action, title, descr, step_time(HH:mm), matched_node, matched(0/1), ontime(0/1), choice, correct(0/1), found_at, found, measure, result, evidence, forms(JSON), flow_type, score, receiver, receiver_ok(0/1), skipped(0/1), **skip_reason** | 实战版处置记录 |
| 9 | drill_node_state | `(project_id, node_id) PK` | role_id, user, started_at | 打开表单即标记为处置中 |
| 10 | tasks | `id TEXT PK` (T... ) | project_id, role_id, title, descr, deadline, status(5态), level, forms(JSON), node_id, seq, **completed_at**, **accepted_at** | 基础版任务 |
| 11 | task_attachments | `id INTEGER PK` | task_id, name, type, size, url, file_path, uploader, uploader_id, time, auto(0/1) | 任务附件 |
| 12 | form_templates | `id TEXT PK` (F1-F8+) | name, fields(JSON) | 表单模板 |
| 13 | form_submissions | `id INTEGER PK` | task_id, form_id, form_name, data(JSON), submitter, user_id, role_id, time | 表单提交 |
| 14 | messages | `id TEXT PK` | time, level, source, text, targets(JSON), seq, project_id | 指令类消息 |
| 15 | notifs | `id TEXT PK` | time, level, source, title, descr, seq, target(all/internal), project_id | 站内消息 |
| 16 | notif_reads | `(user_id, notif_id) PK` | project_id | 未读状态 |
| 17 | chat_messages | `id INTEGER PK` | grp(inner/external), sender, role_id, user_id, time(YYYY-MM-DD HH:mm), text, att, file_path, project_id | 会商频道消息 |
| 18 | evidence | `id TEXT PK` (E... ) | type, name, size, time, role_id, uploader, user_id, auto(0/1), task_id, project_id, url, file_path, created_at, note, **node_id**, **req_key** | 取证材料 |
| 19 | snapshots | `id TEXT PK` | project_id, name, time, by_user, note | 状态快照 |
| 20 | timeline | `id INTEGER PK` | project_id, t(HH:mm), actor, role_id, action, detail, level, seq | 时序台账 |
| 21 | seq_flow | `id INTEGER PK` | project_id, name, time, role, state, seq | 处置时序图节点（当前被 monitor/dashboard 动态计算替代） |
| 22 | score_items | `id TEXT PK` | name, weight, rule, seq | 评分指标模板 |
| 23 | scores | `item_id TEXT PK` | project_id, score, max, note, auto(0/1), updated_at | 实际评分结果 |
| 24 | ledger | `id TEXT PK` | project_id, name, date, type, event, level, result, score, org | 演练台账 |
| 25 | opinion | `id TEXT PK` | project_id, time, channel, title, heat, risk, status, source | 舆情记录 |
| 26 | knowledge | `id TEXT PK` (KB...) | name, cat, updated, size, pages, content, **file_path** | 预案知识库 |
| 27 | audit_logs | `id INTEGER PK` | time, user, role_id, user_id, action, ip | 审计日志（只追加） |
| 28 | impact | `(sys, project_id) PK` | state, pct, seq | 业务影响（**联合主键**） |
| 29 | dict | `key TEXT PK` | value(JSON) | 流程字典 / 站内消息模板 |
| 30 | sim_data | `type TEXT PK` | data(JSON) | 场景模拟数据 |
| 31 | settings | `key TEXT PK` | value | 系统设置 + 迁移 flag |
| 32 | seed_baseline | `id INTEGER PK` | version, component, data, note, created_at | 种子基线版本 |

### 1.2 索引

```
idx_drill_steps_proj      drill_steps(project_id)
idx_tasks_project         tasks(project_id)
idx_tasks_role            tasks(role_id)
idx_att_task              task_attachments(task_id)
idx_evidence_project      evidence(project_id)
idx_timeline_project      timeline(project_id)
idx_audit_time            audit_logs(time)
idx_chat_grp              chat_messages(grp)
```

SQLite `PRAGMA foreign_keys=ON` 在 server.js（历史路径：现为 node/server.js）启动时没有显式执行，但 schema.sql 里写了。Go 版要在连接建立后显式开启。

### 1.3 migrate() 补列清单（Go 版 DDL 必须包含）

**项目相关：**

| 表 | 列 | 类型 | 说明 |
| --- | --- | --- | --- |
| scenarios | alert | TEXT | 故障通报 |
| projects | mode | TEXT DEFAULT '基础版' | 演练模式 |
| projects | started_at | TEXT | 真实计时起点 |
| projects | paused_at | TEXT | 暂停时刻 |
| projects | paused_sec | INTEGER DEFAULT 0 | 累计暂停秒数 |
| projects | ended_at | TEXT | 结束时刻 |
| roles | status | TEXT DEFAULT '启用' | |
| sessions | role_id | TEXT | 会话角色 |
| script_nodes | guide | TEXT | 基础版引导提示 |
| script_nodes | keywords | TEXT | 实战版判定关键词 |
| script_nodes | action | TEXT | 动作类型 |
| script_nodes | required | INTEGER DEFAULT 1 | 是否必经 |
| script_nodes | wrong | TEXT | JSON 干扰项 |
| script_nodes | evidence_req | TEXT | JSON 取证要求 |
| script_nodes | flow_type | TEXT DEFAULT 'correct' | correct/distractor |
| script_nodes | score | INTEGER DEFAULT 0 | 分值 |
| script_nodes | receiver | TEXT | 下一节点接收人 |
| script_nodes | deps | TEXT | JSON 前置依赖 |
| script_nodes | pos_x | INTEGER | 画布坐标 |
| script_nodes | pos_y | INTEGER | 画布坐标 |
| script_nodes | **anchor_id** | TEXT | 混淆锚定主流节点 |
| script_nodes | **release_timing** | TEXT DEFAULT 'before' | before/after |
| script_nodes | **short_title** | TEXT | 节点简称 ≤6 字 |
| tasks | **completed_at** | TEXT | |
| tasks | **accepted_at** | TEXT | |
| tasks | node_id | TEXT | 关联剧本节点 |
| drill_steps | choice | TEXT | 处置方案选择 |
| drill_steps | correct | INTEGER DEFAULT 1 | 是否正确 |
| drill_steps | found_at | TEXT | 发现时间 |
| drill_steps | found | TEXT | 发现情况 |
| drill_steps | measure | TEXT | 处置措施 |
| drill_steps | result | TEXT | 处置结果 |
| drill_steps | evidence | TEXT | 证据说明 |
| drill_steps | forms | TEXT | JSON 表单填报 |
| drill_steps | flow_type | TEXT DEFAULT 'correct' | |
| drill_steps | score | REAL DEFAULT 0 | 本次分值变化 |
| drill_steps | receiver | TEXT | 下一节点接收人 |
| drill_steps | receiver_ok | INTEGER DEFAULT 1 | 接收人是否正确 |
| drill_steps | skipped | INTEGER DEFAULT 0 | 是否跳过 |
| drill_steps | **skip_reason** | TEXT | 混淆节点跳过原因 |
| knowledge | file_path | TEXT | |
| evidence | note | TEXT | 取证说明 |
| evidence | node_id | TEXT | 关联剧本节点 |
| evidence | req_key | TEXT | 剧本取证项标识 |
| notifs | target | TEXT DEFAULT 'all' | all/internal |
| notifs | project_id | TEXT | |
| notif_reads | project_id | TEXT | |
| messages | project_id | TEXT | |
| chat_messages | project_id | TEXT | |

### 1.4 migrate() 回填逻辑（Go 版初始化数据阶段执行一次即可）

| 函数 | 作用 |
| --- | --- |
| `UPDATE chat_messages SET grp='external' WHERE grp='joint'` | 旧频道名合并 |
| 聊天时间补全日期 | 只 HH:mm 时补演练日期 |
| 回填 user_roles | 按 users.role_id 初始化 |
| 回填 script_nodes.deps | 串行链（第 1 空，其余依赖前一个） |
| `backfillAnchorForDistractors` | 混淆节点自动锚定同角色最近主流节点 |
| `backfillFlowScores` | 分值均分 100 + 为未配置场景生成混淆流程 |
| `backfillReceivers` | 按依赖关系推导接收人 |
| `backfillBasicTasks` | 基础版项目任务重建 |
| `backfillFeedbackForm` | 信息科负责人上报审批节点绑定 F8 |
| `backfillTaskForms` | script_nodes.form 与 tasks.forms 对齐 |
| impact 联合主键 DROP+CREATE | 按项目隔离业务系统状态 |
| ensureBrand / ensureForms / ensureFormControls | 系统设置 + 表单模板 |
| notify.ensureTemplates | 站内消息模板写入 |

### 1.5 impact 联合主键 DROP+CREATE 细节（必须复刻）

```sql
CREATE TABLE impact_old ...   -- 旧单列主键
CREATE TABLE impact (sys TEXT NOT NULL, state TEXT, pct INTEGER, seq INTEGER,
                     project_id TEXT NOT NULL DEFAULT '',
                     PRIMARY KEY (sys, project_id))
-- 迁移数据，NULL project_id → '' 作全局模板
-- 为每个已有项目从 '' 模板复制一份独立 impact 行
```

**Go 版建表直接用最终联合主键，跳过 DROP+CREATE。** 但新项目创建时仍需要从全局模板复制 → 这部分逻辑要保留。

---

## 2. 接口契约（Go 版必须逐一对齐）

统一前缀 `/api`，统一错误格式 `{error: "string", ...}`，鉴权中间件 `requireAuth` 从三重来源读取 token。

### 2.1 /api/auth — 认证

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /accounts | 公开 | — | — | `[{username, name, org, role, roleName, short, color, external, roles:[{id,name,short,color,external}]}]` | — |
| GET | /roles | 公开 | — | — | `[{id, name, short, icon, color, desc, nav:[], external}]`（仅 status=启用） | — |
| POST | /login | 公开 | — | body: `{username, password, role?}` | 200: `{token, user:{username,name,org,mustChange}, role:{id,name,short,icon,color,desc,nav:[],external}}`；400/401/403: `{error}` | 写 sessions |
| POST | /logout | 是 | — | — | `{ok:true}` | 删 sessions |
| GET | /me | 是 | — | — | `{username,name,org,mustChange,role,roles:[]}` | — |
| POST | /change-password | 是 | — | body: `{oldPassword, newPassword(≥8位且含字母+数字)}` | `{ok:true,time}` | 写 users.pwd_salt/hash, must_change=0 |

### 2.2 /api — 启动引导（bootstrap.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /bootstrap | 是 | — | — | 巨型聚合（见 2.2.1） | — |
| POST | /projects/:id/activate | 是 | — | — | `{ok:true, project}` | 写 settings.active_project |
| PATCH | /projects/:id/status | 是 | — | body: `{status}` ∈ {未开始,演练中,暂停,已结束,已归档} | `{ok:true, project}` | **真实计时状态机**：更新 started_at/paused_at/paused_sec/ended_at；触发 notify.push(drill_start/drill_end) |

#### 2.2.1 /bootstrap 返回 DTO（Go 版必须保持字段名一致）

```
serverTime, activeProjectId,
currentUser:{id,username,name,org,roleId},
roles:[{id,name,short,icon,color,desc,nav,external,status}],
users:[...](仅 admin/commander 可见),
projects:[{id,name,type,date,scenarioId,level,status,target,units,members,observers,mode,startedAt,pausedAt,pausedSec,endedAt,elapsedSec,progress}],
scenarios:[...],
scriptNodes:[...],
tasks:[...](实战版项目不返回任务),
formTemplates:[...],
messages:[{id,time,level,source,text,targets}],
notifs:[{id,time,level,source,title,desc,read}],
chat:{inner:[],external:[]}(按角色频道权限过滤),
evidence:[...](外部仅见报送类),
snapshots:[...],
timeline:[...](外部仅见上报/接收类),
seqFlow:[...],
scoreItems:[...],
scores:[...],
ledger:[...],
opinion:[...],
knowledge:[...],
auditLogs:[...](仅 admin),
impact:[...](全局 + active),
dict:{key:[...]},
simData:{type:{...}}
```

### 2.3 /api/system — 系统设置（system.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /brand | 公开 | — | — | `{name,unitName,logo,version,...}` | 读 settings |
| PUT | /brand | 是 | platformadmin | body: `{name, unitName, logo...}` | `{ok:true, brand}` | 写 settings（复用 brand.writeBrand） |
| POST | /logo | 是 | platformadmin | multipart: file | `{ok:true, url, name}` | 上传 `/uploads/system/{YYYYMM}/`（限 PNG/JPG/WebP/GIF, ≤2MB） |
| GET | /status | 是 | platformadmin | — | seed.status() | — |
| POST | /reset | 是 | platformadmin | — | `{ok:true, userCount, resetAt}` | 调用 seed.resetAll() 清空重建（**危险操作，有 settings.reset_lock 互斥锁**） |
| GET | /dict/:key | 是 | platformadmin | — | `{key, value}` | DRILL_DICT_KEYS 白名单 |
| PUT | /dict/:key | 是 | platformadmin | body: `{value}` | `{ok:true}` | 写 dict 表 + drillDict.invalidate() 清缓存 |
| GET | /seed/baselines | 是 | platformadmin | — | seed_baseline 去重列表 | — |
| POST | /seed/export | 是 | platformadmin | body: `{version, note}` | `{ok:true, version, created_at}` | 导出当前数据库为种子基线 |

### 2.4 /api/admin — 系统管理（admin.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /users | 是 | platformadmin | — | `[{id,username,name,role,roleName,roles,roleList,org,status,createdAt}]` | — |
| POST | /users | 是 | platformadmin | body: `{username, name, roles, org, password?}` | `{ok:true, id}` | hashPassword(scrypt)；新建用户初始密码见 .env 的 DRILL_INIT_PWD |
| PATCH | /users/:id | 是 | platformadmin | body: `{username?, name?, roles?, org?, status?}` | `{ok:true, username?, roles?}` | 删除 sessions（已被移除角色） |
| POST | /users/:id/reset-password | 是 | platformadmin | body: `{password(≥8位+字母+数字)}` | `{ok:true}` | 删除该用户全部 sessions |
| GET | /roles | 是 | platformadmin | — | — | — |
| PATCH | /roles/order | 是 | platformadmin | body: `{ids:[]}` | `{ok:true}` | 更新 role.seq |
| PATCH | /roles/:id/status | 是 | platformadmin | body: `{status: 启用/停用}` | `{ok:true, status}` | 停用删所有该 role_id sessions |
| DELETE | /roles/:id | 是 | platformadmin | — | `{ok:true}` | 有账号绑定时 409 禁止 |
| POST | /roles | 是 | platformadmin | body: `{name, nav, desc, external}` | `{ok:true, role}` | nav 限制在 MODULE_KEYS 内 |
| PUT | /roles/:id | 是 | platformadmin | body: `{name?, nav?, desc?}` | `{ok:true, role}` | platformadmin 必须保留 admin 权限 |
| GET | /forms | 是 | platformadmin | — | normalizeTemplate 列表 | — |
| POST | /forms | 是 | platformadmin | body: `{name, fields}` | `{ok:true, id, name, fields}` | F1→... 自动编号 |
| PUT | /forms/:id | 是 | platformadmin | body: `{name, fields}` | `{ok:true, template}` | — |
| POST | /backup | 是 | platformadmin | — | `{ok:true, file, path, size, time}` | WAL checkpoint → 复制 drill.db 到 data/backups/ |
| GET | /backups | 是 | platformadmin | — | `[{file, size, time}]` | — |
| POST | /restore | 是 | platformadmin | body: `{file}` | `{ok:true, file, preBackup, size, time}` | 安全网自动备份当前 DB → integrity_check → reload + migrate |
| POST | /restore-upload | 是 | platformadmin | multipart: file(.db, 1KB-100MB) | 同上 | 先落盘到 data/backups/，再恢复 |
| DELETE | /backups/:file | 是 | platformadmin | — | `{ok:true}` | 文件名安全校验 |
| GET | /backups/:file | 是 | platformadmin | — | 文件下载 | — |
| GET | /report/:projectId | 是 | platformadmin | — | 报告聚合 DTO | — |
| POST | /report/:projectId/export | 是 | platformadmin | body: `{format}` | `{ok:true, time}` | — |
| GET | /audit/export | 是 | platformadmin | — | CSV + BOM | — |
| GET | /sysinfo | 是 | platformadmin | — | `{node, platform, hostname, uptimeMin, dbFile, serverTime, counts}` | — |

---

> （文档继续）接口清单第二部分（content.js ~ tasks.js ~ drill.js ~ evidence.js）、状态机与调度逻辑、前端耦合点、大屏、静态资源与上传、移动端、重构风险清单 — 见 docs/phase0-inventory.md 后续章节。


### 2.5 /api/content — 内容/场景/项目/大屏（content.js · 最大的路由文件）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /messages | 是 | — | — | [{id,time,level,source,text,targets}] | — |
| POST | /messages | 是 | commander/itlead/platformadmin | body: {level, source, text, targets} | {ok:true, id} | 写 messages，project_id=active_project |
| GET | /notifs | 是 | — | — | [{id,time,level,source,title,desc,read}]（外部过滤 internal 目标） | — |
| POST | /notifs/read | 是 | — | body: {ids:[...]} (不传则全部) | {ok:true} | 写 notif_reads |
| GET | /chat/channels | 是 | — | — | [{key, label, count, last}] 按 chat_channels 字典过滤 | — |
| GET | /chat/:group | 是 | — | group ∈ dict | [{id, sender, role, userId, time, text, att, url}] | — |
| POST | /chat/:group/upload | 是 | — | multipart: file | {ok:true, name, size, url, path} | 上传 uploads/chat/{YYYYMM}/ |
| POST | /chat/:group | 是 | — | body: {text?, att?, filePath?, attSize} | {ok:true, id} | 写 chat_messages；有 filePath 时同时写 evidence(type=聊天记录) |
| GET | /drill-progress | 是 | — | query: channel=inner/external | {project, mode, scenario, nodes:[{id,seq,title,target,action,planTime,duration,scope,status(done/doing/wrong),actor,opTime,detail,correct,ontime}], updatedAt} | 从 script_nodes + drill_steps + drill_node_state 动态计算；实战版按 deps 依赖链过滤可见性 |
| GET | /scenarios | 是 | — | — | 场景列表 | — |
| PATCH | /scenarios/:id | 是 | platformadmin/commander/itlead | body: {name?, type?, level?, status?, desc?, alert?, version?, nodes?} | {ok:true} | 写 scenarios |
| POST | /scenarios | 是 | platformadmin/commander/itlead | body: {name, type?, level?, desc?, alert?} | {ok:true, id} | SXX 编号，status=草稿，version=v1.0 |
| GET | /script-nodes | 是 | — | — | 全部 script_nodes mapNode DTO（含 anchorId/releaseTiming/shortTitle） | — |
| POST | /script-nodes | 是 | platformadmin/commander/itlead | body: {scenarioId, nodes:[...], alert?, name?, versionBump: none/major/minor/patch} | {ok:true, count, scenarioId, version, updated, alert, name} | **事务**：重编号主流节点(S{n}-{m})和混淆节点(DS{SID}-{role}-{n})→同步 deps/anchorId→依赖清洗（排除混淆/end/同场景）→topoSort 校验无环→删旧写新→推导 receiver→算 evidence_req→版本号递增 |
| GET | /projects | 是 | — | — | projectDto 列表 | — |
| POST | /projects | 是 | platformadmin/commander/itlead | body: {name, date, scenarioId?, mode(基础版/实战版), type?, level?, target?, members[]} | {ok:true, id, taskCount, project} | P{year}-{n} 编号；ensureBasicTasks（基础版）；为新项目复制全局 impact；notify.push(project_created)；写 timeline |
| PATCH | /projects/:id/members | 是 | platformadmin/commander/itlead | body: {members:[{role}]} | {ok:true, project} | 写 projects.members |
| DELETE | /projects/:id | 是 | platformadmin | — | {ok:true, id, tasks, files} | **级联清理**：task_attachments + form_submissions + evidence + tasks + drill_steps + drill_node_state + timeline + seq_flow + scores + snapshots + ledger + opinion + notifs/messages/chat_messages 按项目名 LIKE 兜底 |
| GET | /projects/:id/script | 是 | — | — | buildProjectScript DTO（基础版有完整 guide/desc/score；实战版 guide 固定话术） | — |
| GET | /projects/:id/script/export | 是 | — | query: mt=word(default)/html | 伪装 .doc 或 HTML 下载 | — |
| POST | /projects/:id/notify | 是 | platformadmin/commander/itlead | — | {ok:true, projectId, msg} | notify.push(project_created) + timeline |
| GET | /projects/:id/notify/export | 是 | — | query: mt=word/html | 下载 | — |
| GET | /snapshots | 是 | — | — | 快照列表 | — |
| POST | /snapshots | 是 | — | body: {name?, note} | {ok:true, id} | 写 snapshots，project_id=active |
| GET | /timeline | 是 | — | — | 时序列表（外部仅看上报类） | — |
| GET | /seqflow | 是 | — | — | seq_flow 列表 | — |
| POST | /seqflow | 是 | — | body: {items:[{name,time,role,state}]} | {ok:true} | 先删 active 项目旧的再写新的（**当前 monitor/dashboard 已动态计算替代，此表基本废弃**） |
| GET | /evaluation | 是 | — | — | {items:score_items[], scores:scores[]} | active 项目 |
| POST | /evaluation/scores | 是 | commander/itlead/platformadmin | body: {scores:[{itemId, score, max, note}]} | {ok:true, time} | INSERT OR REPLACE scores(auto=1)；notify.push(auto_eval) |
| GET | /ledger | 是 | — | — | 台账列表 | — |
| GET | /opinion | 是 | — | — | 舆情列表 | — |
| POST | /opinion | 是 | cac/commander/itlead/platformadmin | body: {title, channel?, heat?, risk?} | {ok:true, id} | O{n} 编号 |
| PATCH | /opinion/:id | 是 | — | body: {status?, risk?, heat?} | {ok:true} | — |
| GET | /knowledge | 是 | — | — | 知识库列表（含 url= /uploads/...） | — |
| POST | /knowledge | 是 | platformadmin/commander/itlead | multipart: file + body: {name?, cat?} | {ok:true, id, doc} | 上传 uploads/knowledge/；KB{n} 编号 |
| DELETE | /knowledge/:id | 是 | platformadmin/commander/itlead | — | {ok:true, id} | 同时删除物理文件 |
| GET | /audit | 是 | platformadmin/commander | query: limit≤2000 | [{time,user,role,action,ip}] | — |
| POST | /audit | 是 | — | body: {action} | {ok:true} | 追加审计 |
| GET | /impact | 是 | — | — | active 项目或全局模板 | — |
| PATCH | /impact/:sys | 是 | commander/itlead/sysadmin/platformadmin | body: {state?, pct?} | {ok:true} | active 项目优先，否则全局 |
| GET | /users | 是 | — | — | 用户只读列表 | — |
| GET | /roles | 是 | — | — | 角色只读列表 | — |
| POST | /audit | 是 | — | body: {action} | {ok:true} | 前端操作留痕 |
| GET | /dict | 是 | — | — | dict 全表 JSON（key→value） | — |
| GET | /sim-data | 是 | — | — | sim_data 全表 | — |
| **GET** | **/monitor** | **公开** | — | query: projectId? | **大屏聚合 DTO（见 2.5.1）** | **只读** |
| **GET** | **/dashboard** | **是** | — | query: projectId? | **态势总看板聚合 DTO（见 2.5.2）** | **只读** |

#### 2.5.1 /content/monitor DTO（免登录，大屏专用）

`
serverTime,
project: {id,name,date,status,mode,level,scenarioId,startedAt,pausedAt,pausedSec,endedAt,elapsedSec},
projects:[...],
stats: {total, done, doing, pending, overdue},
tasks:[...],
timeline:[...],
seqFlow:[{id,seq,name,shortTitle,role,time,state,isOverdue,timeout,deps,type,parallelDesc}],
chainNodes:[同上完整三态],
score: {score, grade, total, hit, ontime, late, missed, pending, completeness, timeRate, deductions:[...]},
impact:[{sys,state,pct}],
messages:站内+院内会商合并去重,
roles:[{id,name,color,external}],
roleAgg: [{roleId, total, done, overtime}]
`

**重要：** 本接口内部调用 equire('./drill').score(projectId) 复用评分结果，Go 版必须把 score() 抽成独立 service。

#### 2.5.2 /content/dashboard DTO（态势总看板）

`
serverTime,
project: {id,name,status,mode,level,startedAt,date},
stats: {totalTasks, doneTasks, doingTasks, pendingTasks, overdueTasks, wrongTasks, onlineActors, totalActors, score, grade, mode},
seqFlow:[{id,title,role,flowType,state,completedAt,score,deduct}],   // 实战版正确流程 + 被执行的混淆
roles:[{roleId,roleName,total,done,overtime,wrong}],
docs:[去重证据(task_id+type+name)],
msgs:[timeline 近 40 条],
impact:[...],
`

---

### 2.6 /api/tasks — 任务协同（tasks.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | / | 是 | — | — | 任务列表（实战版项目自动过滤掉所有任务） | 含 available/blockedBy |
| POST | /:id/accept | 是 | 见 loadTask | — | {ok:true, status} | status=处理中，accepted_at=hhmm |
| POST | /:id/submit | 是 | 见 loadTask | body: {note} | {ok:true, evidenceId} | 写 evidence(处置记录) + timeline |
| POST | /:id/complete | 是 | 见 loadTask | — | {ok:true, status} | status=已完成，completed_at=now；写 evidence + timeline；notify.push(node_done) |
| DELETE | /:id | 是 | commander/itlead | — | {ok:true, id, attachments, evidence} | 级联清理 tasks + task_attachments + form_submissions + evidence |
| POST | /reset-all | 是 | platformadmin/commander | — | {ok:true} | 已完成→待处理 |
| POST | /:id/attachments | 是 | loadTask | multipart: files[] (最多10) | {ok:true, attachments:[...]} | 同时登记 evidence(同 task_id) |
| GET | /:id/attachments | 是 | loadTask | — | 附件列表 | — |
| DELETE | /:id/attachments/:attId | 是 | 上传者本人或 platformadmin | — | {ok:true} | 同时删除物理文件 |
| POST | /:id/forms | 是 | loadTask | body: {formId, data} | {ok:true, submissionId} | 写 form_submissions + evidence(表单文书) + timeline；**F1→impact 自动 state=故障（含系统名别名归一化字典）；F7→impact 逐系统更新 state，全已恢复时写 timeline 业务全部恢复** |
| GET | /:id/forms | 是 | loadTask | — | 表单提交记录列表 | — |

**loadTask 中间件**：校验存在 → 实战版项目 403 → taskAccess 权限矩阵（commander 全看，itlead 看 managed_roles_by_role 字典条目，其余仅自己）

### 2.7 /api/drill — 演练模式与考评（drill.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | /workbench/:projectId | 是 | — | — | {projectId, mode, scenarioId, nodes(仅主流), steps, evidence, score} | — |
| POST | /nodes/:nodeId/start | 是 | 非外部 | — | {ok:true, projectId, nodeId} | UPSERT drill_node_state（project_id+node_id 为主键） |
| GET | /flows/:projectId | 是 | 非外部，且实战版 | — | {projectId, mode, projectStatus, flowCount, flows:[nodeId,timeout,type,action,target,title,desc,formId,formName,fields,locked,blockedBy,submitted?], receivers, evidence, score} | 混淆节点按 isDistractorReleased 过滤；返回列表按 nodeId 哈希打乱顺序 |
| GET | /faults/:projectId | 是 | 非外部，且实战版 | — | {projectId, mode, projectStatus, count, faults:[nodeId,title,symptom,status,handled,handledBy,stepTime,correct,ontime], score} | 仅正确流程节点，status 枚举：待处置/已控制/已控制(超时)/处置偏差/已跳过 |
| POST | /skip | 是 | 非外部 | body: {projectId, nodeId, reason?} | {ok:true, nodeId, skipped:true, stepTime, delta(固定0), score} | **混淆节点跳过时 reason 必填（needReason:true）**；写 drill_steps + timeline；notify.push(node_skip) |
| POST | /decide | 是 | 非外部 | body: {projectId, nodeId, measure, result, evidence?, found?, forms(JSON), receiver(多选逗号拼接)} | {ok:true, id, nodeId, stepTime, correct, ontime, delta, receiver, receiverOk, score} | **核心决策提交**：写 drill_steps + evidence(处置记录) + timeline；notify.push(node_submit)；receiver_ok=0 时扣本流程 10% |
| POST | /finish | 是 | commander/platformadmin/itlead | body: {projectId} | {ok:true, projectId, score} | 前置校验 evidence 全部归档；status=已结束；notify.push(drill_end_score) |
| POST | /steps | 是 | 非外部，且实战版 | body: {projectId, title, action?, descr?, stepTime?} | {ok:true, id, matchedNode, matchedTitle, ontime, stepTime} | 旧版兼容入口（工作台已改为决策菜单） |
| DELETE | /steps/:id | 是 | 本人或 platformadmin/commander | — | {ok:true} | 删 drill_steps |
| POST | /score/:projectId/recalc | 是 | commander/platformadmin/itlead | — | 完整 score() DTO | INSERT OR REPLACE scores(item_id='MODE',...) |
| GET | /predecessor | 是 | 非外部 | query: 
odeId, projectId | {ok:true, mode, currentNode, predecessors:[..., distractorSkips:[{nodeId,title,user,skipTime,reason}]]} | 只读前置处置详情；实战版**绝不下发** script_nodes 的 flow_type/guide/keywords/wrong/score/receiver/required |

**导出（被 content.js 复用）**：

`js
module.exports.score(projectId)    // 完整扣分制评分
module.exports.projectOf(id)       // 读取项目
`

### 2.8 /api/evidence — 取证与归档（evidence.js）

| 方法 | 路径 | 鉴权 | 角色限制 | 入参 | 出参 | 副作用 |
| --- | --- | --- | --- | --- | --- | --- |
| GET | / | 是 | — | — | evidence 列表（外部仅见报送类 + itlead/commander 提交） | LEFT JOIN projects |
| POST | / | 是 | 非外部 | body: {type, name, size?, nodeId?, taskId?, projectId?, auto?, note?, dataUrl?} | {ok:true, id, attId?, url?, size?, deduped?} | dataUrl base64 自动落盘到 uploads/auto/{YYYYMM}/；asAttachment=true 时同时写 task_attachments |
| POST | /upload | 是 | 非外部 | multipart: files[] + body: {type?, nodeId?, note?, taskId?, reqKey?, projectId?} | {ok:true, files:[{id,name,size,url}]} | uploads/evidence/{YYYYMM}/ |
| POST | /prefill | 是 | 非外部 | body: {items:[{nodeId, index, name?, note?}], projectId?} | {ok:true, count, items:[], skipped:[{name,reason}]} | 批量导入剧本要求的模拟材料（auto=false，自动生成 SVG 截图或文本 MD 文件） |
| POST | /mock | 是 | 非外部 | body: {nodeId, index, projectId?} | {ok:true, id, name, size, url, deduped} | 一键导入单个模拟材料（幂等） |
| GET | /requirements | 是 | — | query: projectId?, mine=1 | {project, items:[{key,nodeId,nodeTitle,target,targetRoleId,index,type,name,note,mine,done,evidenceId,doneAt,hasFile,url}], done,total,pending,mineDone,mineTotal,minePending} | 按 script_nodes.evidence_req 逐项算归档状态 |
| GET | /:id/download | 是 | — | — | 文件下载 | 外部单位仅见报送类 |

---

## 3. 状态机与调度逻辑

### 3.1 演练真实计时（drill_clock.js）

`
elapsedSec(project, at?) → 秒数
  未启动 → null
  status=暂停 → 取 paused_at
  status=已结束/已归档 → 取 ended_at
  否则 → 当前时间（或 at 参数）
  扣除 paused_sec
  负数归零

formatElapsed(sec) → HH:MM:SS

secBetween(a, b) → 两个 YYYY-MM-DD HH:mm 之间秒数（继续演练累加暂停时长）
`

**前端依赖**：api.js 里 elapsedOf(p) 用同一套算法本地推算，Go 版后端保持字段名 startedAt/pausedAt/pausedSec/endedAt 不变。

### 3.2 项目状态机（bootstrap.js PATCH /projects/:id/status）

`
未开始 → 演练中 → 暂停 ⇄ 演练中 → 已结束 → 已归档
              ↑ paused_sec 累加           ↑ 冻结 ended_at
`

- 从未开始 启动 → started_at=nowSec()
- 暂停时 paused_at=nowSec()；继续时 paused_sec += secBetween(paused_at, nowSec())，paused_at=null
- 结束时 ended_at=nowSec()，暂停期间先累加 paused_sec 再冻结
- 重置为未开始：started_at=null, paused_at=null, paused_sec=0, ended_at=null

### 3.3 混淆节点下发时机（isDistractorReleased）

`
flowType !== 'distractor' → 直接可见
releaseTiming === 'after'  → anchor_id 主流节点 drill_steps 有记录才可见
releaseTiming === 'before' → anchor_id 的 deps 全部完成（主流节点可处置）即可见
无 anchor_id 或锚点被删 → 兜底可见（避免老数据集体消失）
`

**调用点三处必须口径一致**：
1. drill.js flowList() 决定实战版节点列表
2. drill.js score() 决定 releasedDistractors 参与扣分/统计
3. drill.js skip() 里 skip_reason 校验前也要先过滤未下发的混淆节点

### 3.4 节点解锁（nodeLock）

`
flowType === 'distractor' && releaseTiming === 'after' && 锚点未完成 → locked
flowType === 'correct' → deps 全部完成（drill_steps 或 tasks.status=已完成）才可执行
返回 {locked:bool, blockedBy:[{id,title,reason}]}
`

### 3.5 扣分制评分（drill.js score）—— 最高风险

`
基准 100 分，只减不增

correct 节点（主流）:
  按时完成 + receiver 正确 → 0 扣分
  超时完成 → 扣本节点 score 的 10%（Math.round*10/10）
  receiver_ok=0 → 再扣本节点 score 的 10%
  演练结束仍未完成 → 扣本节点 score 的 100%
  被跳过 → 演练结束时按未完成计，扣 100%

distractor 节点（混淆）:
  被选执行(correct=0, skipped≠1) → 扣混淆节点 score 的 100%
  被跳过(skipped=1) → 0 扣分（未执行即无违规）
  未下发(isDistractorReleased=false) → 不计入任何统计
`

**前置起算时刻**：基础版取 	asks.completed_at，实战版取 drill_steps.created_at；无前置时取 projects.started_at。

**超时判定**：
- 基础版：	ask.status === '超时'
- 实战版：drill_steps.correct=1 && drill_steps.ontime=0

### 3.6 下一节点接收人选择

`
剧本预设 receiver（正确流程时必填）
  实战版由参演人员自行选择（多选逗号拼接）
  receiver_ok = sameReceivers(剧本预设排序后, 实际选择排序后)
  receiver_ok=0 → 扣本流程 10% 分值
`

### 3.7 站内消息模板渲染（notify.js）

模板存储位置：dict.key='notif_templates'，FALLBACK_TEMPLATES 内置 8 个 event：

| event | 触发点 |
| --- | --- |
| drill_start | 项目状态→演练中 |
| drill_end | 项目状态→已结束 |
| drill_end_score | /drill/finish |
| node_done | tasks/:id/complete |
| node_submit | /drill/decide |
| node_skip | /drill/skip |
| auto_eval | /evaluation/scores |
| project_created | POST /projects |

模板变量：{varname}，vars 对象传入替换，读不到模板时走 FALLBACK_TEMPLATES。

### 3.8 F1/F7 表单 → impact 自动联动

**F1《事件初步核实单》** 提交时：
- 解析字段 "受影响系统"（multiselect 顿号/逗号/空格分隔）
- 别名归一化字典（his收费→HIS收费系统，emr→电子病历系统，等等）
- 标准系统集合：HIS收费系统、医护工作站、电子病历系统、临床检验系统、医学影像系统、办公OA系统
- UPDATE impact SET state='故障', pct=30 WHERE sys=? AND project_id=?

**F7《业务恢复确认单》** 提交时：
- 6 个独立下拉：{HIS收费系统状态, 医护工作站状态, ...}
- 逐系统更新 impact.state
- 如果所有系统都是 '已恢复' 或 '正常'，写 timeline 业务全部恢复

**这是硬编码业务规则，Go 版必须原样保留，不要抽到配置里。**

---

## 4. 前端接口耦合点（public/api.js）

### 4.1 错误返回格式（Go 版必须保持）

- **通用错误**：{error: "中文消息"}，HTTP 4xx/5xx
- **skip 混淆节点**：{error, needReason: true} — 前端看到 
eedReason===true 会自动重开弹窗要求填原因
- **流程锁定**：{error, blockedBy:[{id,title,reason}]} — 前端用于展示"前置未完成"红色警示
- **演练结束前置校验**：{error, pending, done, total, pendingItems:[...]} — 前端弹窗展示未归档取证项
- **conflict 409**：账号已存在/账号已绑定等

### 4.2 必须保持的 DTO 字段名

| DTO | 字段 | 说明 |
| --- | --- | --- |
| project | id, name, type, date, scenarioId, level, status, target, mode, startedAt, pausedAt, pausedSec, endedAt, elapsedSec, progress, units, members, observers | **elapsedSec 是后端预计算好的秒数**，前端本地用 fmtElapsed 格式化 |
| scriptNode | id, scenarioId, type, title, shortTitle, target, timeout, form, desc, deps(JSON), flowType(correct/distractor), score, anchorId, releaseTiming(before/after), posX, posY, receiver | 时间格式化在前端 |
| task | id, projectId, role, title, desc, status, level, nodeId, forms(JSON), atts, available, blockedBy | **实战版项目 tasks 不返回**（由 flowList/faults 替代） |
| step (drill_steps) | id, nodeId?, roleId, user, action, title, descr, step_time, matched_node, correct(0/1), ontime(0/1), found_at, found, measure, result, evidence, forms, flow_type, score, receiver, receiver_ok(0/1), skipped, skip_reason, created_at | — |
| evidence | id, type, name, size, time, role, uploader, auto(0/1), taskId, projectId, nodeId, reqKey, note, hasFile, url(相对路径 /uploads/...) | — |
| notif | id, time, level(P0/P1/P2), source, title, desc, read | read 是布尔 |
| chat | id, sender, role, userId, time(YYYY-MM-DD HH:mm), text, att, url | — |

### 4.3 token 三种来源

`
1. Authorization: Bearer <token>       ← 绝大多数请求
2. X-Drill-Token: <token>               ← 同上（备用 header）
3. query param ?t=<token>               ← 文件下载（<a download> 无 header）
`

**Go 中间件必须按优先级依次尝试**。

### 4.4 DRILL.bootstrap 一次性下发全部数据

前端登录后只做一次 /api/bootstrap，后续只读 sessionStorage / MOCK 缓存。Go 版必须提供同样的聚合接口，或改造前端为增量拉取（工作量大，建议先保持聚合）。

### 4.5 响应字段类型细节

- elapsedSec、pausedSec：**整数秒**，前端 mtElapsed(sec) 输出 HH:MM:SS
- score（drill.step 里的）：**REAL 浮点**，可能为 -3.3 这种
- 布尔统一为整数 0/1：equired, skipped, ontime, correct, uto, eceiver_ok
- 时间字段：后端存 YYYY-MM-DD HH:mm[:ss] 字符串，前端本地 parse 或直接展示
- JSON 字段：数据库 TEXT 存字符串，后端返回时 parseJson，**Go 版用 JSON -> interface{} 反序列化**

---

## 5. 大屏模块依赖（public/screen.html）

### 5.1 页面

- **路径**：/screen.html（静态）
- **访问权限**：顶栏按钮仅对 platformadmin/commander/itlead 显示；页面本身免登录
- **跳转方式**：window.open('/screen.html','_blank','width=1680,height=960,menubar=no,toolbar=no') 独立弹出窗口

### 5.2 布局

`css
body { position: fixed; top:0; left:0; width:1920px; height:1080px; }
/* 整屏固定 1920×1080，不响应式，靠 transform-origin:top left + 前端 scale 适配不同分辨率 */
`

**Go 复刻决策**：保持 1920×1080 固定（投影场景确定），不做响应式手机端。

### 5.3 数据源

| 模块 | 接口 | 刷新频率 |
| --- | --- | --- |
| 项目切换 | GET /api/content/projects | 页面加载一次 |
| 核心数据 | GET /api/content/monitor?projectId=X | 每 3-5 秒轮询 |
| 时间/时钟 | 前端本地 Date() | 每秒 |

**无 WebSocket、无 SSE，纯 HTTP 轮询。**

### 5.4 monitor 关键计算函数

`
computeMonitorNodes(project)           → {all, filtered, overdue}
  基础版: tasks.status → done/doing/pending
  实战版: drill_steps + drill_node_state → done/doing/pending
  超时判定: 复用 drill.score() 的 late+missed
  seqFlow 只展示 state !== pending 的节点（done/doing）
  chainNodes 返回全部三态
  /dashboard 内部逻辑独立实现 seqFlow（也复用 drill.score()）
`

---

## 6. 静态资源与上传

### 6.1 uploads/ 目录结构

`
uploads/{kind}/{YYYYMM}/xxx
  kind ∈ system | tasks | evidence | chat | knowledge | backups | auto | mock
`

| kind | 用途 | 大小限制 | 类型限制 |
| --- | --- | --- | --- |
| system | Logo | ≤2MB | PNG/JPG/WebP/GIF |
| tasks | 任务附件 | MAX_MB 默认 20MB | 禁可执行扩展名 |
| evidence | 取证材料 | MAX_MB | 同上 |
| chat | 会商附件 | MAX_MB | 同上 |
| knowledge | 知识库文档 | MAX_MB | 同上 |
| backups | 备份恢复 | 1KB-100MB | .db |
| auto | dataURL 截图（自动取证） | ≤8MB | image/* |
| mock | 剧本模拟材料（SVG/MD/TXT） | — | — |

### 6.2 全局上传限制

`js
MAX_MB = 20 (env DRILL_MAX_UPLOAD_MB)
BAD_EXT = /\.(exe|bat|cmd|vbs|scr|msi|com|pif|jar|ps1|sh|dll|sys|lnk)$/i
files per upload: 10 (multer 限制)
`

Go 版等价实现：用 gin-contrib/multipart 或手动 io.LimitReader，扩展名黑名单要保持一致（禁止真实恶意样本）。

### 6.3 文件名处理（中文兼容）

`
multer 以 latin1 解析原始文件名
→ Buffer.from(orig, 'latin1').toString('utf8') 转回
→ safeName 清理 \\/:*?"<>| → 替换为 _
→ multer storage.filename: {timestamp}-{rand}-{safename}
`

### 6.4 静态资源服务

`
/uploads  → express.static(UPLOAD_DIR, {index:false, Content-Disposition:inline})
/public   → express.static(PUBLIC_DIR, {index:'index.html'})
/api      → 业务路由
/X-Content-Type-Options: nosniff
/X-Frame-Options: SAMEORIGIN
/Referrer-Policy: same-origin
`

### 6.5 dataURL 图片自动落盘（evidence_plan）

前端 canvas.toDataURL → 后端 saveDataUrl(dataUrl) → 解码 base64 → 校验 ≤8MB → 存 uploads/auto/{YYYYMM}/，返回相对路径。

### 6.6 模拟材料生成

- 截图类：构建 SVG（CSS 渲染风格不变，Go 版直接字符串拼接即可，不用真解析）
- 文本类：MD/TXT 内容拼装

---

## 7. 移动端现状评估

| 页面 | 布局方式 | 手机可用 | 建议 |
| --- | --- | --- | --- |
| index.html（登录） | 居中卡片，flex | 是 | 可保持 |
| dashboard.html | flex 自适应 | 部分可用 | 保留响应式 |
| workbench.html | flex，有 role tabs | 部分可用 | 保留 |
| projects.html | flex | 部分可用 | 保留 |
| scenarios.html | flex + 可视化画布 | **否**（画布 1400+ 宽） | **必须适配** 或仅桌面 |
| content.js 剧本编排画布 | 1400px+ 固定最小宽 | **完全不行** | 桌面专用即可，**手机强制隐藏/提示** |
| screen.html | **固定 1920×1080** | **完全不行**（投影专用） | 不做 |
| admin.html（用户/角色/表单） | flex + table | 部分可用 | 保留 |
| evidence/knowledge/ledger 列表 | flex | 基本可用 | 保留 |

**结论**：必须明确声明"演练监视大屏 + 剧本可视化编排画布 仅桌面 1366 分辨率以上可用"；其余业务页面继续保持 flex 响应式，手机浏览器可正常访问（信息科值班员用手机接告警消息的场景可以支撑）。

---

## 8. 重构风险清单（按风险从高到低）

### R1 — 评分扣分制语义偏差

- **触发条件**：Go 版 score() 计算逻辑任何一处判断与 Node 版不同（比如混淆节点 anchor 未设置时的兜底、receiver_ok 的排序判定、超时起算时刻取错）
- **影响范围**：drill.score() 被 /monitor、/dashboard、/drill/finish、/drill/decide、/drill/recalc、前端复盘页等 **至少 7 处** 复用
- **Go 版做法**：把 Node 版 score() 整个移植为独立纯函数（不依赖 Express req/res），单测用 seed_data.js 的 S01 跑一遍比对输出

### R2 — migrate() 回填 / 补列顺序

- **触发条件**：Go 版按新 schema.sql 直接建表，但 impact 联合主键迁移的脚本没跑；或者 script_nodes.anchor_id 存在但所有老数据都空（backfillAnchorForDistractors 没执行）
- **影响范围**：混淆节点全部"无锚点兜底可见"，节点解锁逻辑失效
- **Go 版做法**：
  - 新库：DDL 直接按最终版（含 anchor_id/release_timing/skip_reason/impact 联合主键）
  - 初始化数据脚本单独实现 backfill 逻辑（backfillDeps/backfillAnchorForDistractors/backfillFlowScores/backfillReceivers/backfillTaskForms）
  - 如果要兼容现有 .db，就要保留完整 ALTER TABLE + 回填流程

### R3 — drill_steps.skip_reason 字段 + 混淆节点跳过 reason 必填

- **触发条件**：schema.sql 没加 skip_reason 列，或 /drill/skip 没校验 needReason
- **影响范围**：前端 skip 弹窗提交混淆节点时收到 500 或 reason 丢失
- **Go 版做法**：建表直接包含；/drill/skip handler 区分 correct 节点 reason 可选、distractor 节点 reason 必须非空且 ≤500 字

### R4 — impact 联合主键 DROP+CREATE + 新项目复制模板

- **触发条件**：新项目创建时忘了从全局 impact 模板复制到项目隔离行
- **影响范围**：所有 F1/F7 表单提交 → impact 更新 WHERE project_id=? 无匹配行 → 不报错但不生效
- **Go 版做法**：POST /projects 创建成功后立刻 INSERT OR IGNORE INTO impact SELECT * FROM impact WHERE project_id=''

### R5 — sessions 过期清理 + 角色移除强制失效

- **触发条件**：每次请求前没清理 expires_at 过期的 sessions；user_roles 解绑后没删 sessions.role_id 对应的行
- **影响范围**：停用角色账号还能继续操作；已解绑角色会话仍持有旧角色权限
- **Go 版做法**：认证中间件最后追加一段清理（或者定时 goroutine 每分钟扫一次）

### R6 — 令牌三重来源

- **触发条件**：只实现了 Bearer header，没实现 X-Drill-Token 或 query ?t=
- **影响范围**：文件下载（/api/evidence/:id/download, /api/admin/backups/:file）前端用 fetch 携带 Authorization，但另一些场景用 window.open + ?t=
- **Go 版做法**：中间件 	okenOf(c *gin.Context) 按 Authorization → X-Drill-Token → Query("t") 顺序返回第一个非空

### R7 — script_nodes 重编号算法 + deps/anchorId 引用同步

- **触发条件**：Go 版 POST /script-nodes 保存剧本后重编号，但漏更新某一处引用（deps 或 anchor_id）
- **影响范围**：流程解锁永远被 block；混淆节点锚点不存在 → 兜底可见
- **Go 版做法**：把 Node 版 topoSortNodes + 空号优先填充 + 生成 idMap 的算法逐行移植，单测覆盖"新增节点 + 删除中间节点 + 混淆锚点"三个场景

### R8 — /bootstrap 聚合接口字段顺序/命名差异

- **触发条件**：Go 版返回字段名与 api.js 假设不同（比如 camelCase vs snake_case）
- **影响范围**：前端整个界面崩溃（bootstrap 是所有页面的启动数据）
- **Go 版做法**：DTO 层严格按 Node 版 mapNode/mapTask/mapEvidence 输出字段名写 struct + json tag，单测比对 GET /bootstrap 的 JSON 完全一致

### R9 — notify.push 模板渲染读不到 dict.key='notif_templates'

- **触发条件**：ensureTemplates 没跑（Go 版 migrate 里漏调），或 dict 表初始化时没写入 8 个默认模板
- **影响范围**：所有 drill_* / node_* / auto_eval / project_created 事件 title/desc 变成空
- **Go 版做法**：启动时做一次 ensureTemplates（INSERT OR IGNORE 每个 event），同时 notify.push 里保留 FALLBACK_TEMPLATES 兜底

### R10 — F1/F7 → impact 硬编码联动

- **触发条件**：Go 版 tasks.js 里两处 IF formId==='F1' / formId==='F7' 的逻辑没移植
- **影响范围**：态势总看板业务影响模块永远显示"正常"状态
- **Go 版做法**：单独抽一个 service handleFormSubmissionEffects(formId, data, projectId)，里面硬编码 SYS_ALIAS 字典和 STANDARD_SYSS 集合

### R11 — upload 目录 /uploads/{kind}/{YYYYMM}/ 结构

- **触发条件**：Go 版把所有上传丢到 uploads/flat/，或 kind 目录命名不同（比如 evidence 写成 evidences）
- **影响范围**：老数据文件全部找不到；URL 相对路径不一致导致前端 404
- **Go 版做法**：完全沿用 Node 版目录结构，	ime.Now().Format("200601") 生成月份子目录

### R12 — drill.js score() 的时间解析

- **触发条件**：前端 step_time 是 "HH:mm"（不带日期），Go 版按 RFC3339 解析失败返回 NaN
- **影响范围**：超时判定永远 false；elapsedSec 返回 0；计时停滞
- **Go 版做法**：复制 Node 版 drill_clock.parseLocal 逻辑：正则 YYYY-MM-DD HH:mm，缺补演练日期

---

## 附录 A：环境变量

| 变量 | 默认 | 用途 |
| --- | --- | --- |
| DRILL_DB | data/drill.db | SQLite 数据库路径 |
| DRILL_PORT | 8080 | 监听端口 |
| DRILL_HOST | 0.0.0.0 | 监听地址 |
| DRILL_SESSION_MIN | 480 | 令牌过期分钟数 |
| DRILL_INIT_PWD | (见 .env) | 新建用户初始密码（占位：CHANGE_ME） |
| DRILL_MAX_UPLOAD_MB | 20 | 单个文件上传大小上限 |

## 附录 B：Go 复刻建议技术栈

| 层 | 选型 | 理由 |
| --- | --- | --- |
| Web 框架 | Gin | 生态成熟，路由快 |
| 数据库 | SQLite（纯 Go 驱动） | modernc.org/sqlite 或 glebarez/sqlite（兼容 CGO 环境） |
| ORM | GORM + 原生 SQL 混合 | 28 张表 + 复杂 join，ORM 不划算；但建表迁移用 GORM AutoMigrate 方便 |
| 密码散列 | golang.org/x/crypto/scrypt | 和 Node crypto.scryptSync 输出完全一致 |
| 会话 | 自实现（内存 + SQLite 落盘） | sessions 表已存在，Go 版直接复用 |
| 上传 | gin-contrib/multipart 或手动 multipart.Reader | 限制大小 + 扩展名黑名单 |
| 鉴权中间件 | 自实现（三重 token 来源 + requirePage 模块权限） | 简单明了 |
| 模板渲染 | Go html/template 伪装 .doc（同 Node 版逻辑） | Word 导出复用 |

---

*文档结束 · Phase 0 只读盘点完成 · 未经修改任何业务代码或数据库*

