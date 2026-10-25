/* 数据库连接与初始化（使用 Node 内置 node:sqlite，无需编译原生模块） */
'use strict';
const path = require('node:path');
const fs = require('node:fs');
const { DatabaseSync } = require('node:sqlite');
const { now, nowSec } = require('./util');

const DATA_DIR = path.join(__dirname, '..', 'data');
const DB_FILE = process.env.DRILL_DB || path.join(DATA_DIR, 'drill.db');

if (!fs.existsSync(DATA_DIR)) fs.mkdirSync(DATA_DIR, { recursive: true });

const db = new DatabaseSync(DB_FILE);

/* 建表（幂等） */
function migrate() {
  const sql = fs.readFileSync(path.join(__dirname, 'schema.sql'), 'utf8');
  db.exec(sql);
  const alertNew = !db.prepare('PRAGMA table_info(scenarios)').all().some(function (c) { return c.name === 'alert'; });
  addColumn('scenarios', 'alert', 'TEXT');
  /* 旧库升级：为已存在的剧本补充初始故障通报（可在剧本编排中修改） */
  if (alertNew) {
    try {
      db.exec("UPDATE scenarios SET alert='T+0 监测发现'||COALESCE(NULLIF(type,''),'网络安全')||'类异常：'||COALESCE(NULLIF(descr,''),'请立即按预案开展研判与处置。') WHERE alert IS NULL OR alert=''");
    } catch (e) {
      console.warn('[migrate] 故障通报回填跳过：', e.message);
    }
  }
  addColumn('projects', 'mode', "TEXT DEFAULT '基础版'");
  /* 旧库升级：原「跨部门联合会商组」并入「院外上报沟通」频道 */
  try {
    db.prepare("UPDATE chat_messages SET grp='external' WHERE grp='joint'").run();
  } catch (e) {
    console.warn('[migrate] 会商频道升级跳过：', e.message);
  }
  /* 旧库升级：聊天时间统一为「YYYY-MM-DD HH:mm」完整格式 */
  try {
    const today = now().slice(0, 10);
    const rows = db.prepare("SELECT id,time FROM chat_messages WHERE time IS NOT NULL AND length(time)<=5").all();
    const up = db.prepare('UPDATE chat_messages SET time=? WHERE id=?');
    rows.forEach(function (r) { up.run(today + ' ' + String(r.time).slice(0, 5), r.id); });
  } catch (e) {
    console.warn('[migrate] 聊天时间格式升级跳过：', e.message);
  }
  addColumn('roles', 'status', 'TEXT DEFAULT \'启用\'');
  addColumn('sessions', 'role_id', 'TEXT');
  /* 旧库升级：按原单角色回填用户多角色表 */
  try {
    db.prepare(
      'INSERT INTO user_roles (user_id,role_id) ' +
      'SELECT id,role_id FROM users WHERE role_id IS NOT NULL AND NOT EXISTS ' +
      '(SELECT 1 FROM user_roles ur WHERE ur.user_id=users.id AND ur.role_id=users.role_id)'
    ).run();
  } catch (e) {
    console.warn('[migrate] 用户角色回填跳过：', e.message);
  }
  addColumn('script_nodes', 'guide', 'TEXT');
  addColumn('script_nodes', 'keywords', 'TEXT');
  addColumn('script_nodes', 'action', 'TEXT');
  addColumn('script_nodes', 'required', 'INTEGER DEFAULT 1');
  addColumn('script_nodes', 'wrong', 'TEXT');
  addColumn('tasks', 'completed_at', 'TEXT');
  addColumn('tasks', 'accepted_at', 'TEXT');
  addColumn('tasks', 'node_id', 'TEXT');
  addColumn('drill_steps', 'choice', 'TEXT');
  addColumn('drill_steps', 'correct', 'INTEGER DEFAULT 1');
  addColumn('drill_steps', 'found_at', 'TEXT');
  addColumn('drill_steps', 'found', 'TEXT');
  addColumn('drill_steps', 'measure', 'TEXT');
  addColumn('drill_steps', 'result', 'TEXT');
  addColumn('drill_steps', 'evidence', 'TEXT');
  addColumn('knowledge', 'file_path', 'TEXT');
  addColumn('evidence', 'note', 'TEXT');
  addColumn('evidence', 'node_id', 'TEXT');
  addColumn('evidence', 'req_key', 'TEXT');
  addColumn('script_nodes', 'evidence_req', 'TEXT');
  addColumn('script_nodes', 'flow_type', "TEXT DEFAULT 'correct'");
  addColumn('script_nodes', 'score', 'INTEGER DEFAULT 0');
  addColumn('drill_steps', 'forms', 'TEXT');
  addColumn('drill_steps', 'flow_type', "TEXT DEFAULT 'correct'");
  addColumn('drill_steps', 'score', 'REAL DEFAULT 0');
  addColumn('script_nodes', 'receiver', 'TEXT');
  addColumn('script_nodes', 'deps', 'TEXT');
  addColumn('script_nodes', 'pos_x', 'INTEGER');
  addColumn('script_nodes', 'pos_y', 'INTEGER');
  addColumn('drill_steps', 'receiver', 'TEXT');
  addColumn('drill_steps', 'receiver_ok', 'INTEGER DEFAULT 1');
  addColumn('drill_steps', 'skipped', 'INTEGER DEFAULT 0');
  addColumn('notifs', 'target', "TEXT DEFAULT 'all'");
  /* 旧库升级：停用「计划时刻」，超时判定改为按上一节点真实完成时间 + 处置时长 */
  try {
    db.prepare("UPDATE script_nodes SET time='' WHERE COALESCE(time,'')<>''").run();
    db.prepare("UPDATE tasks SET deadline='' WHERE COALESCE(deadline,'')<>''").run();
  } catch (e) {
    console.warn('[migrate] 计划时刻清理跳过：', e.message);
  }
  /* 演练真实计时：实际启动/暂停/结束时刻与累计暂停时长 */
  const startedNew = !db.prepare('PRAGMA table_info(projects)').all().some(function (c) { return c.name === 'started_at'; });
  addColumn('projects', 'started_at', 'TEXT');
  addColumn('projects', 'paused_at', 'TEXT');
  addColumn('projects', 'paused_sec', 'INTEGER DEFAULT 0');
  addColumn('projects', 'ended_at', 'TEXT');
  /* 旧库升级：原「演练中」项目没有真实开始时刻，以升级时刻作为计时起点 */
  if (startedNew) {
    db.prepare("UPDATE projects SET started_at=? WHERE status='演练中' AND (started_at IS NULL OR started_at='')").run(nowSec());
  }
  /* 旧库升级：为未配置取证要求的剧本节点按剧本内容自动生成模拟取证材料清单 */
  try {
    const plan = require('./evidence_plan');
    const rows = db.prepare("SELECT * FROM script_nodes WHERE type!='end' AND (evidence_req IS NULL OR evidence_req='')").all();
    const up = db.prepare('UPDATE script_nodes SET evidence_req=? WHERE id=?');
    rows.forEach(function (n) {
      up.run(JSON.stringify(plan.autoEvidenceReq({
        title: n.title, target: n.target, action: n.action, guide: n.guide, desc: n.descr, form: n.form
      })), n.id);
    });
  } catch (e) {
    console.warn('[migrate] 取证材料清单生成跳过：', e.message);
  }
  /* 旧库升级：为监控/核查类节点补充「界面截图」取证材料（已人工配置的清单不覆盖） */
  try {
    const plan = require('./evidence_plan');
    const up = db.prepare('UPDATE script_nodes SET evidence_req=? WHERE id=?');
    db.prepare("SELECT * FROM script_nodes WHERE type!='end'").all().forEach(function (n) {
      const list = plan.normalizeReq(plan.parseJsonSafe(n.evidence_req, []));
      if (!list.length || list.some(function (x) { return x.type === '附件截图'; })) return;
      const auto = plan.autoEvidenceReq({
        title: n.title, target: n.target, action: n.action, guide: n.guide, desc: n.descr, form: n.form
      });
      const shot = auto.filter(function (x) { return x.type === '附件截图'; })[0];
      if (!shot || list.length >= 6) return;
      up.run(JSON.stringify(list.concat([shot])), n.id);
    });
  } catch (e) {
    console.warn('[migrate] 界面截图取证材料补充跳过：', e.message);
  }
  /* 旧库升级：平台管理员需具备「演练报告导出」与「时序回放（整体复盘分析）」权限 */
  grantNav('platformadmin', 'report');
  grantNav('platformadmin', 'replay');
  /* 「演练监视大屏」：指挥部、平台管理员、信息科负责人 */
  grantNav('commander', 'screen');
  grantNav('platformadmin', 'screen');
  grantNav('itlead', 'screen');
  /* 旧库升级：任务执行已并入角色工作台、剧本编排已并入场景剧本库，移除旧菜单键 */
  try {
    db.prepare('SELECT id FROM roles').all().forEach(function (r) { revokeNav(r.id, 'tasks'); revokeNav(r.id, 'script-editor'); });
  } catch (e) {
    console.warn('[migrate] 菜单升级跳过：', e.message);
  }
  /* 旧库升级：补齐正确流程分值（合计 100）并按责任角色生成实战版混淆流程 */
  try {
    const r = require('./drill_plan').backfillFlowScores(db);
    if (r.scored || r.distractors) console.log('[migrate] 流程分值/混淆流程：评分节点 ' + r.scored + ' 个，混淆流程 ' + r.distractors + ' 条');
  } catch (e) {
    console.warn('[migrate] 流程分值/混淆流程生成跳过：', e.message);
  }
  /* 旧库升级：为存量剧本建立串行前置依赖（保持原顺序行为，可在并行编排画布中改为并行） */
  try {
    const n = backfillDeps(db);
    if (n) console.log('[migrate] 剧本节点前置依赖回填 ' + n + ' 个');
  } catch (e) {
    console.warn('[migrate] 前置依赖回填跳过：', e.message);
  }
  /* 旧库升级：按剧本顺序补齐「下一节点接收人」（取后继正确流程的责任角色，末尾为院应急指挥部） */
  try {
    const n = require('./drill_plan').backfillReceivers(db);
    if (n) console.log('[migrate] 下一节点接收人补齐 ' + n + ' 个节点');
  } catch (e) {
    console.warn('[migrate] 接收人补齐跳过：', e.message);
  }
  /* 旧库升级：基础版项目的任务按剧本基线重建，工作台展示具体处置步骤 */
  try {
    require('./drill_plan').backfillBasicTasks(db);
  } catch (e) {
    console.warn('[migrate] 基础版任务回填跳过：', e.message);
  }
  /* 系统品牌设置：补齐默认值（系统名称 / 单位名称 / Logo / 版本 / 项目地址） */
  try {
    require('./brand').ensureBrand(db);
  } catch (e) {
    console.warn('[migrate] 系统设置补齐跳过：', e.message);
  }
  /* 表单模板：升级内置上报文书字段说明并补齐《网络和数据安全事件反馈单》 */
  try {
    require('./forms').ensureForms(db);
    require('./forms').ensureFormControls(db);
  } catch (e) {
    console.warn('[migrate] 表单模板升级跳过：', e.message);
  }
  /* 剧本节点：信息科负责人「上报审批」绑定反馈单，并同步基础版任务表单 */
  try {
    require('./drill_plan').backfillFeedbackForm(db);
  } catch (e) {
    console.warn('[migrate] 反馈单绑定跳过：', e.message);
  }
}

/* 幂等为指定角色补充菜单权限（角色尚未导入时跳过） */
function grantNav(roleId, key) {
  const row = db.prepare('SELECT nav FROM roles WHERE id=?').get(roleId);
  if (!row) return;
  let nav = parseJson(row.nav, []);
  if (!Array.isArray(nav)) nav = [];
  if (nav.indexOf(key) >= 0) return;
  nav.push(key);
  db.prepare('UPDATE roles SET nav=? WHERE id=?').run(JSON.stringify(nav), roleId);
}

/* 幂等移除指定角色的菜单权限 */
function revokeNav(roleId, key) {
  const row = db.prepare('SELECT nav FROM roles WHERE id=?').get(roleId);
  if (!row) return;
  let nav = parseJson(row.nav, []);
  if (!Array.isArray(nav) || nav.indexOf(key) < 0) return;
  db.prepare('UPDATE roles SET nav=? WHERE id=?').run(JSON.stringify(nav.filter(function (k) { return k !== key; })), roleId);
}

/* 兼容旧库：补齐新增列 */
function addColumn(table, col, def) {
  const cols = db.prepare('PRAGMA table_info(' + table + ')').all();
  if (!cols.some(function (c) { return c.name === col; })) {
    db.exec('ALTER TABLE ' + table + ' ADD COLUMN ' + col + ' ' + def);
  }
}

/* 是否已初始化（有角色数据即视为已初始化） */
function isSeeded() {
  try {
    const row = db.prepare('SELECT COUNT(*) AS c FROM roles').get();
    return row.c > 0;
  } catch (e) {
    return false;
  }
}

/* 事务包装 */
function tx(fn) {
  db.exec('BEGIN');
  try {
    const r = fn();
    db.exec('COMMIT');
    return r;
  } catch (e) {
    try { db.exec('ROLLBACK'); } catch (_) {}
    throw e;
  }
}

/* JSON 字段安全解析 */
function parseJson(s, fallback) {
  if (s === null || s === undefined || s === '') return fallback;
  try { return JSON.parse(s); } catch (e) { return fallback; }
}

/* 旧库升级：为未配置前置依赖的剧本节点建立串行链（第 1 个为空，其余依赖前一个）
 * 混淆流程不参与流转，统一写入空数组；已配置依赖的节点保持原样 */
function backfillDeps(database) {
  const up = database.prepare('UPDATE script_nodes SET deps=? WHERE id=?');
  let count = 0;
  database.prepare('SELECT id FROM scenarios').all().forEach(function (sc) {
    const nodes = database.prepare('SELECT id,flow_type,deps FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(sc.id);
    let prevId = null;
    nodes.filter(function (n) { return n.flow_type !== 'distractor'; }).forEach(function (n) {
      if (n.deps === null || n.deps === '') {
        up.run(JSON.stringify(prevId ? [prevId] : []), n.id);
        count++;
      }
      prevId = n.id;
    });
    nodes.filter(function (n) { return n.flow_type === 'distractor'; }).forEach(function (n) {
      if (n.deps === null || n.deps === '') {
        up.run('[]', n.id);
        count++;
      }
    });
  });
  return count;
}

module.exports = { db, migrate, isSeeded, tx, parseJson, DB_FILE, DATA_DIR };
