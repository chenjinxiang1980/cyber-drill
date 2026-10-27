/* 初始化数据库：建表 + 从 seed_data.js 导入初始演练数据
 * 用法：node src/seed.js            首次初始化（已有数据则跳过）
 *       node src/seed.js --reset    清空并重新初始化（谨慎使用）
 */
'use strict';
const path = require('node:path');

process.env.DRILL_DB = process.env.DRILL_DB || path.join(__dirname, '..', 'data', 'drill.db');
const { db, migrate, isSeeded, tx } = require('./db');
const { hashPassword, INITIAL_PASSWORD } = require('./auth');
const { nowSec } = require('./util');

/* 种子数据中心（从运行时 DB 反向导出） */
const SD = require('./seed_data.js');

/* 专项预案正文 */
const KB_CONTENT = require('./kb-content');

const RESET = process.argv.indexOf('--reset') >= 0;

function clearAll() {
  const tables = ['notif_reads','notifs','messages','chat_messages','task_attachments','form_submissions',
    'tasks','script_nodes','sim_data','scores','score_items','evidence','snapshots','timeline','seq_flow',
    'ledger','knowledge','audit_logs','impact','dict','scenarios','projects','form_templates',
    'users','user_roles','roles','sessions','settings'];
  db.exec('PRAGMA foreign_keys = OFF');
  tables.forEach(function (t) { try { db.exec('DELETE FROM ' + t); } catch (e) {} });
  db.exec('PRAGMA foreign_keys = ON');
}

function run() {
  migrate();
  if (RESET) {
    clearAll();
  } else if (isSeeded()) {
    console.log('[seed] 数据库已初始化，跳过。如需重建请执行：npm run reset');
    return;
  }

  const ts = nowSec();

  tx(function () {
    /* ---------- 角色 ---------- */
    const ri = db.prepare('INSERT INTO roles (id,name,short,icon,color,descr,nav,external,seq) VALUES (?,?,?,?,?,?,?,?,?)');
    SD.ROLES.forEach(function (r) {
      const nav = (r.nav || []).filter(function (k) { return k !== 'tasks'; });
      if (r.id === 'platformadmin' && nav.indexOf('report') < 0) nav.push('report');
      if (r.id === 'platformadmin' && nav.indexOf('replay') < 0) nav.push('replay');
      if (['commander', 'platformadmin', 'itlead'].indexOf(r.id) >= 0 && nav.indexOf('screen') < 0) nav.push('screen');
      ri.run(r.id, r.name, r.short || '', SD.ROLE_ICON[r.id] || r.icon || 'user', r.color || '',
        r.descr || '', JSON.stringify(nav), r.external ? 1 : 0, r.seq || 0);
    });

    /* ---------- 用户 + user_roles ---------- */
    const ui = db.prepare('INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?,?,?,?)');
    const ur = db.prepare('INSERT INTO user_roles (user_id,role_id) VALUES (?,?)');
    SD.USERS.forEach(function (u) {
      const p = hashPassword(INITIAL_PASSWORD);
      const r = ui.run(u.username, u.name, u.role, u.org || '', p.salt, p.hash, '启用', 1, ts);
      ur.run(Number(r.lastInsertRowid), u.role);
    });

    /* ---------- 场景 ---------- */
    const si = db.prepare('INSERT INTO scenarios (id,name,type,level,status,version,updated,nodes,descr,alert) VALUES (?,?,?,?,?,?,?,?,?,?)');
    SD.SCENARIOS.forEach(function (s) {
      si.run(s.id, s.name, s.type || '基础版', s.level || '一般', s.status || '启用', s.version || 'v1.0',
        ts.slice(0, 10), 0, s.descr || '', s.alert || '');
    });

    /* ---------- 项目 ---------- */
    const pi = db.prepare('INSERT INTO projects (id,name,mode,date,scenario_id,level,status,target,units,members,observers,started_at,paused_sec) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)');
    SD.PROJECTS.forEach(function (p) {
      pi.run(p.id, p.name, p.mode || '基础版', p.date || '', p.scenario_id, '', p.status || '进行中', '',
        '[]', '[]', '[]', null, 0);
    });

    /* ---------- 任务 ---------- */
    const ti = db.prepare('INSERT INTO tasks (id,project_id,role_id,title,descr,deadline,status,level,node_id,seq) VALUES (?,?,?,?,?,?,?,?,?,?)');
    (SD.TASKS || []).forEach(function (t) {
      ti.run(t.id, t.project_id, t.role_id, t.title, t.descr || '', t.deadline || '', t.status || '待办', t.level || '',
        t.node_id || null, t.seq || 0);
    });

    /* ---------- 剧本节点（从 scenario_baseline 基线） ---------- */
    require('./scenario_baseline').apply(db, { bump: false, date: ts.slice(0, 10) });

    /* ---------- 表单模板 ---------- */
    const fi = db.prepare('INSERT OR REPLACE INTO form_templates (id,name,fields) VALUES (?,?,?)');
    try {
      require('./forms').DEFAULT_TEMPLATES.forEach(function (f) { fi.run(f.id, f.name, JSON.stringify(f.fields || [])); });
    } catch (e) {
      /* forms 模块不存在时跳过 */
    }

    /* ---------- 消息 ---------- */
    const mi = db.prepare('INSERT INTO messages (id,time,level,source,text,targets,seq) VALUES (?,?,?,?,?,?,?)');
    (SD.MESSAGES || []).forEach(function (m) { mi.run(m.id, m.time, m.level, m.source, m.text, typeof m.targets === 'string' ? m.targets : JSON.stringify(m.targets || []), m.seq || 0); });

    /* ---------- 通知 ---------- */
    const noi = db.prepare('INSERT INTO notifs (id,time,level,source,title,descr,seq) VALUES (?,?,?,?,?,?,?)');
    (SD.NOTIFS || []).forEach(function (n) { noi.run(n.id, n.time, n.level, n.source || '系统', n.title, n.descr || '', n.seq || 0); });

    /* ---------- 会商（按 grp 分组） ---------- */
    const ci = db.prepare('INSERT INTO chat_messages (grp,sender,role_id,time,text,att) VALUES (?,?,?,?,?,?)');
    Object.keys(SD.CHAT || {}).forEach(function (g) {
      (SD.CHAT[g] || []).forEach(function (c) {
        ci.run(g, c.sender, c.role, c.time, c.text, c.att || null);
      });
    });

    /* ---------- 取证 ---------- */
    const ei = db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,auto,task_id,project_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)');
    (SD.EVIDENCE || []).forEach(function (e) {
      ei.run(e.id, e.type, e.name, e.size || '-', e.time, e.role_id || 'sys', e.uploader || '',
        e.auto ? 1 : 0, e.task_id || null, e.project_id || null, ts);
    });

    /* ---------- 快照 ---------- */
    const spi = db.prepare('INSERT INTO snapshots (id,project_id,name,time,by_user,note) VALUES (?,?,?,?,?,?)');
    (SD.SNAPSHOTS || []).forEach(function (s) { spi.run(s.id, s.project_id || activeProjectId(), s.name, s.time, s.by_user || '', s.note || ''); });

    /* ---------- 时序 ---------- */
    const tli = db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)');
    (SD.TIMELINE || []).forEach(function (t) {
      tli.run(t.project_id || activeProjectId(), t.t, t.actor, t.role_id || '', t.action, t.detail || '', t.level || '', t.seq || 0);
    });

    /* ---------- 处置时序图 ---------- */
    const sfi = db.prepare('INSERT INTO seq_flow (project_id,name,time,role,state,seq) VALUES (?,?,?,?,?,?)');
    (SD.SEQ_FLOW || []).forEach(function (s) { sfi.run(s.project_id || activeProjectId(), s.name, s.time, s.role, s.state, s.seq || 0); });

    /* ---------- 评分指标与结果 ---------- */
    const kii = db.prepare('INSERT INTO score_items (id,name,weight,rule,seq) VALUES (?,?,?,?,?)');
    (SD.SCORE_ITEMS || []).forEach(function (k) { kii.run(k.id, k.name, k.weight || 0, k.rule || '', k.seq || 0); });
    const kri = db.prepare('INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,?,?)');
    (SD.SCORES || []).forEach(function (s) { kri.run(s.item_id, activeProjectId(), s.score, s.max || 100, s.note || '', 1, ts); });

    /* ---------- 台账 ---------- */
    const li = db.prepare('INSERT INTO ledger (id,project_id,name,date,type,event,level,result,score,org) VALUES (?,?,?,?,?,?,?,?,?,?)');
    (SD.LEDGER || []).forEach(function (l) {
      const pid = l.project_id || l.id;
      li.run(l.id, pid, l.name, l.date, l.type, l.event, l.level, l.result, l.score, l.org || '');
    });

    /* ---------- 知识库 ---------- */
    const kbi = db.prepare('INSERT INTO knowledge (id,name,cat,updated,size,pages,content,file_path) VALUES (?,?,?,?,?,?,?,?)');
    (SD.KNOWLEDGE || []).forEach(function (k) {
      const body = SD.KB_CONTENT && SD.KB_CONTENT[k.id]
        ? JSON.stringify(SD.KB_CONTENT[k.id])
        : (KB_CONTENT[k.id] ? JSON.stringify(KB_CONTENT[k.id]) : '');
      kbi.run(k.id, k.name, k.cat || '预案', k.updated || ts.slice(0, 10), k.size || '', k.pages || 0, body, null);
    });

    /* ---------- 业务影响 ---------- */
    const ii = db.prepare('INSERT INTO impact (sys,state,pct,seq) VALUES (?,?,?,?)');
    (SD.IMPACT || []).forEach(function (m) { ii.run(m.sys, m.state, m.pct, m.seq || 0); });

    /* ---------- 字典 ---------- */
    const di = db.prepare('INSERT INTO dict (key,value) VALUES (?,?)');
    Object.keys(SD.DICT || {}).forEach(function (k) { di.run(k, JSON.stringify(SD.DICT[k])); });

    /* ---------- 场景模拟数据 ---------- */
    const sdi = db.prepare('INSERT INTO sim_data (type,data) VALUES (?,?)');
    Object.keys(SD.SIM_DATA || {}).forEach(function (k) { sdi.run(k, JSON.stringify(SD.SIM_DATA[k])); });

    /* ---------- 系统设置 ---------- */
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('active_project', activeProjectId());
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('initialized_at', ts);
    require('./brand').ensureBrand(db);
  });

  console.log('[seed] 初始化完成');
  console.log('[seed] 数据库文件：' + require('./db').DB_FILE);
  console.log('[seed] 初始密码：' + INITIAL_PASSWORD);
  console.log('[seed] 账号列表：');
  const rows = db.prepare('SELECT u.username,u.name,r.name AS role FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id').all();
  rows.forEach(function (r) { console.log('        ' + r.username.padEnd(16) + r.name + ' · ' + r.role); });
  const counts = {};
  ['users','roles','scenarios','script_nodes','knowledge','projects','tasks','messages','chat_messages'].forEach(function (t) {
    try { counts[t] = db.prepare('SELECT COUNT(*) c FROM ' + t).get().c; } catch (e) {}
  });
  console.log('[seed] 数据概览：' + JSON.stringify(counts));
}

/* 当前演练中的项目 id */
function activeProjectId() {
  const p = (SD.PROJECTS || []).filter(function (x) { return x.status === '演练中'; })[0];
  return p ? p.id : ((SD.PROJECTS && SD.PROJECTS[0]) ? SD.PROJECTS[0].id : '');
}

run();
