/* 初始化数据库：建表 + 从 public/mock.js 导入初始演练数据 + 创建账号
 * 用法：node src/seed.js            首次初始化（已有数据则跳过）
 *       node src/seed.js --reset    清空并重新初始化（谨慎使用）
 */
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

process.env.DRILL_DB = process.env.DRILL_DB || path.join(__dirname, '..', 'data', 'drill.db');
const { db, migrate, isSeeded, tx } = require('./db');
const { hashPassword, INITIAL_PASSWORD } = require('./auth');
const { nowSec } = require('./util');
/* 专项预案 / 上报流程 / 兜底流程正文（依据 2025 版院发正式预案） */
const KB_CONTENT = require('./kb-content');

const RESET = process.argv.indexOf('--reset') >= 0;

/* 账号映射：姓名 → 登录账号 */
const USERNAME = {
  '张副院长': 'yuanzhang',
  '李明': 'liming',
  '王强': 'wangqiang',
  '赵工': 'zhaogong',
  '陈主任': 'chendongzhu',
  '刘主任': 'liuzhuren',
  '孙护士长': 'sunhushizhang',
  '周科员': 'zhoukeyuan',
  '吴处长': 'wuchuzhang',
  '郑警官': 'zhengjingguan',
  '平台管理员': 'admin'
};

const ROLE_ICON = {
  commander: 'shield', duty: 'bell', netsec: 'net', sysadmin: 'db', itlead: 'user',
  medlead: 'hospital', clinical: 'building', healthbureau: 'hospital',
  cac: 'opinion', cyberpolice: 'evidence', platformadmin: 'admin'
};

/* 从 mock.js 载入初始数据（保持单一数据源） */
function loadMock() {
  const file = path.join(__dirname, '..', 'public', 'mock.js');
  const code = fs.readFileSync(file, 'utf8');
  const sandbox = { window: {}, console: console };
  vm.runInNewContext(code, sandbox, { filename: 'mock.js' });
  const M = sandbox.window.MOCK;
  if (!M) throw new Error('mock.js 未导出 window.MOCK');
  return M;
}

function clearAll() {
  const tables = ['notif_reads','notifs','messages','chat_messages','task_attachments','form_submissions',
    'tasks','script_nodes','sim_data','scores','score_items','evidence','snapshots','timeline','seq_flow',
    'ledger','opinion','knowledge','audit_logs','impact','dict','scenarios','projects','form_templates',
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

  const M = loadMock();
  const ts = nowSec();

  tx(function () {
    /* 角色 */
    const ri = db.prepare('INSERT INTO roles (id,name,short,icon,color,descr,nav,external,seq) VALUES (?,?,?,?,?,?,?,?,?)');
    M.ROLES.forEach(function (r, i) {
      const nav = (r.nav || []).filter(function (k) { return k !== 'tasks'; });
      /* 演练报告导出归入「复盘与报送」栏目，仅平台系统管理员可见；平台管理员同时具备时序回放（整体复盘分析）权限 */
      if (r.id === 'platformadmin' && nav.indexOf('report') < 0) nav.push('report');
      if (r.id === 'platformadmin' && nav.indexOf('replay') < 0) nav.push('replay');
      /* 演练监视大屏：指挥部、平台管理员、信息科负责人 */
      if (['commander', 'platformadmin', 'itlead'].indexOf(r.id) >= 0 && nav.indexOf('screen') < 0) nav.push('screen');
      ri.run(r.id, r.name, r.short || '', ROLE_ICON[r.id] || 'user', r.color || '',
        r.desc || '', JSON.stringify(nav), r.external ? 1 : 0, i);
    });

    /* 用户（统一初始密码） */
    const ui = db.prepare('INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?,?,?,?)');
    const ur = db.prepare('INSERT INTO user_roles (user_id,role_id) VALUES (?,?)');
    M.USERS.forEach(function (u) {
      const uname = USERNAME[u.name] || ('user' + Math.floor(Math.random() * 9000 + 1000));
      const p = hashPassword(INITIAL_PASSWORD);
      const r = ui.run(uname, u.name, u.role, u.org || '', p.salt, p.hash, '启用', 1, ts);
      ur.run(Number(r.lastInsertRowid), u.role);
    });

    /* 场景 */
    const si = db.prepare('INSERT INTO scenarios (id,name,type,level,status,version,updated,nodes,descr,alert) VALUES (?,?,?,?,?,?,?,?,?,?)');
    M.SCENARIOS.forEach(function (s) {
      si.run(s.id, s.name, s.type, s.level, s.status, s.version, s.updated, s.nodes || 0, s.desc || '', s.alert || '');
    });

    /* 项目 */
    const pi = db.prepare('INSERT INTO projects (id,name,type,date,scenario_id,level,status,target,units,members,observers,started_at,paused_sec) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)');
    M.PROJECTS.forEach(function (p) {
      /* 初始即为「演练中」的项目以初始化时刻作为真实计时起点 */
      pi.run(p.id, p.name, p.type, p.date, p.scenarioId, p.level, p.status, p.target || '',
        JSON.stringify(p.units || []), JSON.stringify(p.members || []), JSON.stringify(p.observers || []),
        p.status === '演练中' ? ts : null, 0);
    });

    /* 任务 */
    const ti = db.prepare('INSERT INTO tasks (id,project_id,role_id,title,descr,deadline,status,level,forms,seq) VALUES (?,?,?,?,?,?,?,?,?,?)');
    M.TASKS.forEach(function (t, i) {
      ti.run(t.id, t.projectId, t.role, t.title, t.desc || '', '', t.status, t.level,
        JSON.stringify(t.forms || []), i);
      /* 原型中已存在的字符串附件同步为附件记录 */
      (t.atts || []).forEach(function (a) {
        db.prepare('INSERT INTO task_attachments (task_id,name,type,size,url,uploader,time,auto) VALUES (?,?,?,?,?,?,?,?)')
          .run(t.id, String(a), '附件', '-', null, '历史归档', hhmmOf(t.deadline), 0);
      });
    });

    /* 剧本节点：改用 8 类场景的标准处置基线（含基础版引导与实战版判定规则） */
    require('./scenario_baseline').apply(db, { bump: false, date: ts.slice(0, 10) });

    /* 表单模板（使用内置定义，新库中迁移已写入时覆盖为同一份数据） */
    const fi = db.prepare('INSERT OR REPLACE INTO form_templates (id,name,fields) VALUES (?,?,?)');
    require('./forms').DEFAULT_TEMPLATES.forEach(function (f) { fi.run(f.id, f.name, JSON.stringify(f.fields || [])); });

    /* 消息 */
    const mi = db.prepare('INSERT INTO messages (id,time,level,source,text,targets,seq) VALUES (?,?,?,?,?,?,?)');
    M.MESSAGES.forEach(function (m, i) { mi.run(m.id, m.time, m.level, m.source, m.text, JSON.stringify(m.targets || []), i); });

    /* 通知 */
    const noi = db.prepare('INSERT INTO notifs (id,time,level,source,title,descr,seq) VALUES (?,?,?,?,?,?,?)');
    M.NOTIFS.forEach(function (n, i) { noi.run(n.id, n.time, n.level, n.source || '系统', n.title, n.desc, i); });

    /* 会商（时间统一为完整日期时间格式） */
    const todayStr = nowSec().slice(0, 10);
    const ci = db.prepare('INSERT INTO chat_messages (grp,sender,role_id,time,text,att) VALUES (?,?,?,?,?,?)');
    Object.keys(M.CHAT).forEach(function (g) {
      M.CHAT[g].forEach(function (c) { ci.run(g, c.sender, c.role, /^\d{4}-/.test(c.time) ? c.time : (todayStr + ' ' + c.time), c.text, c.att || null); });
    });

    /* 取证 */
    const ei = db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,auto,task_id,project_id,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)');
    M.EVIDENCE.forEach(function (e) {
      ei.run(e.id, e.type, e.name, e.size || '-', e.time, e.role || 'sys', e.uploader || '',
        e.auto ? 1 : 0, null, activeProjectId(M), null, ts);
    });

    /* 快照 */
    const spi = db.prepare('INSERT INTO snapshots (id,project_id,name,time,by_user,note) VALUES (?,?,?,?,?,?)');
    M.SNAPSHOTS.forEach(function (s) { spi.run(s.id, activeProjectId(M), s.name, s.time, s.by, s.note); });

    /* 时序 */
    const tli = db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)');
    M.TIMELINE.forEach(function (t, i) {
      tli.run(activeProjectId(M), t.t, t.actor, t.role, t.action, t.detail || '', t.level || '', i);
    });

    /* 处置时序图 */
    const sfi = db.prepare('INSERT INTO seq_flow (project_id,name,time,role,state,seq) VALUES (?,?,?,?,?,?)');
    M.SEQ_FLOW.forEach(function (s, i) { sfi.run(activeProjectId(M), s.name, s.time, s.role, s.state, i); });

    /* 评分指标与结果 */
    const kii = db.prepare('INSERT INTO score_items (id,name,weight,rule,seq) VALUES (?,?,?,?,?)');
    M.SCORE_ITEMS.forEach(function (k, i) { kii.run(k.id, k.name, k.weight, k.rule || '', i); });
    const kri = db.prepare('INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,?,?)');
    M.SCORES.forEach(function (s) { kri.run(s.itemId, activeProjectId(M), s.score, s.max, s.note || '', 1, ts); });

    /* 台账 */
    const li = db.prepare('INSERT INTO ledger (id,project_id,name,date,type,event,level,result,score,org) VALUES (?,?,?,?,?,?,?,?,?,?)');
    M.LEDGER.forEach(function (l) { li.run(l.id, l.id, l.name, l.date, l.type, l.event, l.level, l.result, l.score, l.org); });

    /* 舆情 */
    const oi = db.prepare('INSERT INTO opinion (id,project_id,time,channel,title,heat,risk,status,source) VALUES (?,?,?,?,?,?,?,?,?)');
    M.OPINION.forEach(function (o) {
      oi.run(o.id, activeProjectId(M), o.time, o.channel, o.title, String(o.heat), o.risk, o.status, o.source);
    });

    /* 知识库 */
    const kbi = db.prepare('INSERT INTO knowledge (id,name,cat,updated,size,pages,content,file_path) VALUES (?,?,?,?,?,?,?,?)');
    M.KNOWLEDGE.forEach(function (k) {
      const body = KB_CONTENT[k.id] ? JSON.stringify(KB_CONTENT[k.id]) : '（预案正文请由平台管理员上传替换，原型阶段为示意条目）';
      kbi.run(k.id, k.name, k.cat, k.updated, k.size, k.pages || 0, body, null);
    });

    /* 审计日志 */
    const ai = db.prepare('INSERT INTO audit_logs (time,user,role_id,action,ip) VALUES (?,?,?,?,?)');
    M.AUDIT_LOGS.forEach(function (a) { ai.run(a.time, a.user, a.role, a.action, a.ip); });

    /* 业务影响 */
    const ii = db.prepare('INSERT INTO impact (sys,state,pct,seq) VALUES (?,?,?,?)');
    M.IMPACT.forEach(function (m, i) { ii.run(m.sys, m.state, m.pct, i); });

    /* 字典 */
    const di = db.prepare('INSERT INTO dict (key,value) VALUES (?,?)');
    Object.keys(M.DICT).forEach(function (k) { di.run(k, JSON.stringify(M.DICT[k])); });

    /* 场景模拟数据 */
    const sdi = db.prepare('INSERT INTO sim_data (type,data) VALUES (?,?)');
    Object.keys(M.SIM_DATA || {}).forEach(function (k) { sdi.run(k, JSON.stringify(M.SIM_DATA[k])); });

    /* 系统设置 */
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('active_project', activeProjectId(M));
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('initialized_at', ts);
    require('./brand').ensureBrand(db);
  });

  console.log('[seed] 初始化完成');
  console.log('[seed] 数据库文件：' + require('./db').DB_FILE);
  console.log('[seed] 初始密码：' + INITIAL_PASSWORD);
  console.log('[seed] 账号列表：');
  const rows = db.prepare('SELECT u.username,u.name,r.name AS role FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id').all();
  rows.forEach(function (r) { console.log('        ' + r.username.padEnd(16) + r.name + ' · ' + r.role); });
}

/* 当前演练中的项目 id */
function activeProjectId(M) {
  const p = (M.PROJECTS || []).filter(function (x) { return x.status === '演练中'; })[0];
  return p ? p.id : (M.PROJECTS && M.PROJECTS[0] ? M.PROJECTS[0].id : '');
}

function hhmmOf(t) { return t || '00:00'; }

run();
