/* 初始化数据库：建表 + 从种子数据导入初始演练数据
 * 种子数据优先从 seed_baseline 表读取（已入库的基线版本），
 * 数据库为空时回退到 seed_data.js + scenario_baseline.js 文件，
 * 首次文件加载后自动 snapshot 入库，固化种子数据避免文件丢失/修改。
 * 用法：node src/seed.js            首次初始化（已有数据则跳过）
 *       node src/seed.js --reset    清空并重新初始化（谨慎使用）
 */
'use strict';
const path = require('node:path');
const fs = require('node:fs');

process.env.DRILL_DB = process.env.DRILL_DB || path.join(__dirname, '..', 'data', 'drill.db');
const { db, migrate, isSeeded, tx } = require('./db');
const { hashPassword, INITIAL_PASSWORD } = require('./auth');
const { nowSec } = require('./util');

const RESET = process.argv.indexOf('--reset') >= 0;

/* 延迟加载：只有当 seed_baseline 表不可用时才尝试从文件读取 */
let _SD = null;
let _SBM = null;
function getSD() {
  if (!_SD) _SD = require('./seed_data.js');
  return _SD;
}
function getSBM() {
  if (!_SBM) _SBM = require('./scenario_baseline');
  return _SBM;
}

/* ========================================================================
 * 种子数据来源解析：数据库优先，文件兜底
 * ======================================================================== */

/* 从数据库 seed_baseline 表读取最新的一套种子数据 */
function loadSeedFromDb() {
  try {
    const rows = db.prepare('SELECT version, component, data FROM seed_baseline ORDER BY id DESC').all();
    if (!rows || rows.length < 2) return null;
    // 提取最最新版本的每个 component
    const byComp = {};
    rows.forEach(function (r) {
      if (!byComp[r.component]) {
        try { byComp[r.component] = JSON.parse(r.data); } catch (e) {}
      }
    });
    if (byComp.seed_data && byComp.scenario_baseline) {
      return {
        version: rows[0].version,
        seedData: byComp.seed_data,
        scenarioBaseline: { SCEN: byComp.scenario_baseline.SCEN || byComp.scenario_baseline,
          MAIN_DEPS: byComp.scenario_baseline.MAIN_DEPS,
          PROJECT_MODE: byComp.scenario_baseline.PROJECT_MODE }
      };
    }
  } catch (e) { /* seed_baseline 表不存在或为空 */ }
  return null;
}

/* 从文件兜底加载种子数据 */
function loadSeedFromFile() {
  const SD = getSD();
  const SBM = getSBM();
  return {
    version: 'file',
    seedData: SD,
    scenarioBaseline: {
      SCEN: SBM.SCEN,
      MAIN_DEPS: SBM.MAIN_DEPS,
      PROJECT_MODE: SBM.PROJECT_MODE
    }
  };
}

/* 统一入口：返回 { seedData, scenarioBaseline, version, fromDb } */
function loadSeedSource() {
  const fromDb = loadSeedFromDb();
  if (fromDb) {
    fromDb.fromDb = true;
    return fromDb;
  }
  const fromFile = loadSeedFromFile();
  fromFile.fromDb = false;
  return fromFile;
}

/* 将当前种子数据 snapshot 存入 seed_baseline 表（幂等） */
function snapshotSeed(seedData, scenarioBaseline, version, note) {
  const ts = nowSec();
  try {
    const ins = db.prepare(
      'INSERT OR REPLACE INTO seed_baseline (version, component, data, note, created_at) VALUES (?,?,?,?,?)'
    );
    ins.run(version || 'initial', 'seed_data', JSON.stringify(seedData), note || '首次部署自动入库', ts);
    ins.run(version || 'initial', 'scenario_baseline', JSON.stringify(scenarioBaseline), note || '首次部署自动入库', ts);
    console.log('[seed] 种子数据已 snapshot 入 seed_baseline（version=' + (version || 'initial') + '）');
    return true;
  } catch (e) {
    console.warn('[seed] snapshotSeed 失败（可能是表尚未创建）：', e.message);
    return false;
  }
}

/* 检查 seed_baseline 表是否有种子数据 */
function hasBaseline() {
  try {
    const row = db.prepare('SELECT COUNT(*) c FROM seed_baseline').get();
    return row.c >= 2;
  } catch (e) { return false; }
}

/* 导出当前运行时数据库为基线（供 /api/system/seed/export 使用） */
function exportBaseline(version, note) {
  version = version || 'v-custom-' + nowSec().slice(0, 10);
  note = note || '运行时导出';
  const ts = nowSec();

  /* 从当前数据库反导出 seed_data 格式 */
  const seedData = {};
  seedData.ROLES = db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return { id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color,
      descr: r.descr, nav: JSON.parse(r.nav || '[]'), external: !!r.external, seq: r.seq };
  });
  seedData.USERS = db.prepare('SELECT username,name,role_id AS role,org FROM users').all();
  seedData.SCENARIOS = db.prepare('SELECT * FROM scenarios ORDER BY id').all().map(function (s) {
    return { id: s.id, name: s.name, type: s.type, level: s.level, status: s.status,
      version: s.version, descr: s.descr, alert: s.alert };
  });
  seedData.KNOWLEDGE = db.prepare('SELECT id,name,cat,updated,size,pages FROM knowledge').all();
  seedData.KB_CONTENT = {};
  db.prepare("SELECT id,content FROM knowledge WHERE content IS NOT NULL AND content<>''").all().forEach(function (k) {
    try { seedData.KB_CONTENT[k.id] = JSON.parse(k.content); } catch (e) {}
  });
  const dictRows = db.prepare('SELECT key,value FROM dict').all();
  seedData.DICT = {};
  dictRows.forEach(function (r) {
    try { seedData.DICT[r.key] = JSON.parse(r.value); } catch (e) { seedData.DICT[r.key] = r.value; }
  });
  const simRows = db.prepare('SELECT type,data FROM sim_data').all();
  seedData.SIM_DATA = {};
  simRows.forEach(function (r) {
    try { seedData.SIM_DATA[r.type] = JSON.parse(r.data); } catch (e) { seedData.SIM_DATA[r.type] = r.data; }
  });

  /* scenario_baseline：优先从 seed_baseline 数据库读，没有再兜底从文件 */
  let scenarioBL = null;
  try {
    const sbRow = db.prepare("SELECT data FROM seed_baseline WHERE component='scenario_baseline' ORDER BY id DESC LIMIT 1").get();
    if (sbRow) scenarioBL = JSON.parse(sbRow.data);
  } catch (e) {}
  if (!scenarioBL) {
    const SBM = getSBM();
    scenarioBL = { SCEN: SBM.SCEN, MAIN_DEPS: SBM.MAIN_DEPS, PROJECT_MODE: SBM.PROJECT_MODE };
  }

  snapshotSeed(seedData, scenarioBL, version, note);
  return { version: version, note: note, created_at: ts };
}

/* ---------- 剧本节点快照备份 ---------- */
const BACKUP_DIR = path.join(__dirname, '..', 'data', 'backups');
function ensureBackupDir() {
  if (!fs.existsSync(BACKUP_DIR)) fs.mkdirSync(BACKUP_DIR, { recursive: true });
}
/* 备份 script_nodes 到 JSON（在 seed/apply 覆盖前自动调用） */
function snapshotScriptNodes(db, label) {
  ensureBackupDir();
  const rows = db.prepare('SELECT * FROM script_nodes ORDER BY scenario_id, seq').all();
  if (!rows.length) return null;
  const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
  const fname = 'script-nodes-' + ts + (label ? '-' + label : '') + '.json';
  const fpath = path.join(BACKUP_DIR, fname);
  const pack = {
    meta: { label: label || 'auto', at: new Date().toISOString(), count: rows.length },
    nodes: rows.map(function (r) {
      const o = {};
      Object.keys(r).forEach(function (k) { o[k] = r[k]; });
      return o;
    })
  };
  fs.writeFileSync(fpath, JSON.stringify(pack, null, 2), 'utf-8');
  // 只留最近 20 份
  const all = fs.readdirSync(BACKUP_DIR).filter(function (f) { return f.startsWith('script-nodes-'); });
  if (all.length > 20) {
    all.sort().slice(0, all.length - 20).forEach(function (f) {
      try { fs.unlinkSync(path.join(BACKUP_DIR, f)); } catch (e) {}
    });
  }
  console.log('[seed] script_nodes 已备份 → data/backups/' + fname + '（' + rows.length + '条）');
  return fpath;
}

/* 将 scenario baseline 中的剧本节点写入数据库
 * 接受 SBLx = { SCEN, MAIN_DEPS, PROJECT_MODE }，兼容老格式（元组数组）和新格式（完整对象）
 * 此函数是自包含的，不依赖 scenario_baseline.js，确保 seed_baseline 可独立完成初始化
 *
 * 安全机制：
 *   - 默认行为：script_nodes 表已有数据则**跳过**（不会覆盖用户编辑）
 *   - force=true：无条件覆盖（会先自动备份）
 *   - 备份文件保存在 data/backups/ 目录，保留最近 20 份
 */
function applyScenarioNodes(db, SBLx, today, opts) {
  opts = opts || {};
  const force = !!opts.force;
  const _cnt = db.prepare('SELECT COUNT(*) c FROM script_nodes').get();
  if (!force && _cnt.c > 0) {
    console.log('[seed] script_nodes 已有 ' + _cnt.c + ' 条数据，跳过 apply（如需强制覆盖请显式 force=true）');
    return { skipped: true, reason: 'has_data', count: _cnt.c };
  }
  if (force && _cnt.c > 0) {
    snapshotScriptNodes(db, 'pre-force-apply');
  }
  const _SCEN = (SBLx && SBLx.SCEN) || {};
  const _MAIN_DEPS = (SBLx && SBLx.MAIN_DEPS) || {};
  const _PROJECT_MODE = (SBLx && SBLx.PROJECT_MODE) || {};
  const del = db.prepare('DELETE FROM script_nodes WHERE scenario_id=?');
  const ins = db.prepare('INSERT INTO script_nodes (id,scenario_id,type,title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,action,required,deps,flow_type,score,pos_x,pos_y,parallel_desc) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)');
  Object.keys(_SCEN).forEach(function (sid) {
    const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(sid);
    if (!sc) return;
    const all = _SCEN[sid];
    const isFull = all && all._mode === 'full';
    let main, dist;
    if (isFull) {
      main = all.main.slice();
      dist = all.dist.slice();
    } else {
      main = all.filter(function (r) { return r[5] === 1; });
      dist = all.filter(function (r) { return r[5] === 0; });
    }
    del.run(sid);
    if (isFull) {
      main.forEach(function (o, i) {
        const px = (o.posX !== undefined && o.posX !== null) ? o.posX : null;
        const py = (o.posY !== undefined && o.posY !== null) ? o.posY : null;
        ins.run(o.id, sid, o.type, o.title, o.target, '', o.timeout || null, '', '', o.desc || '',
          JSON.stringify([]), i, o.guide || '', o.keywords || '', o.action || '', o.required,
          JSON.stringify(o.deps || []), o.flow_type || 'correct', (o.score !== undefined && o.score !== null) ? o.score : 0, px, py, o.parallel_desc || '');
      });
      dist.forEach(function (o, i) {
        const sc_ = (o.score !== undefined && o.score !== null) ? o.score : 10;
        const ft = o.flow_type || 'distractor';
        const px = (o.posX !== undefined && o.posX !== null) ? o.posX : null;
        const py = (o.posY !== undefined && o.posY !== null) ? o.posY : null;
        ins.run(o.id, sid, o.type, o.title, o.target, '', o.timeout || null, '', '', o.desc || '',
          JSON.stringify([]), main.length + i, o.guide || '', o.keywords || '', o.action || '', o.required,
          JSON.stringify(o.deps || []), ft, sc_, px, py, o.parallel_desc || '');
      });
    } else {
      const mainCount = main.length;
      const remainCount = mainCount - 1;
      const mainBase = remainCount > 0 ? Math.floor(100 / remainCount) : 0;
      const mainRemainder = remainCount > 0 ? 100 % remainCount : 0;
      main.forEach(function (r, i) {
        const id = sid + '-' + (i + 1);
        const deps = _MAIN_DEPS[sid] && _MAIN_DEPS[sid][i]
          ? _MAIN_DEPS[sid][i].map(function (di) { return sid + '-' + (di + 1); })
          : [];
        let score;
        if (i === 0) score = 0;
        else if ((i - 1) < mainRemainder) score = mainBase + 1;
        else score = mainBase;
        ins.run(id, sid, r[0], r[1], r[2], '', r[4] || null, '', '', r[6] || '',
          JSON.stringify([]), i, r[7] || '', r[8] || '', r[9] || '', 1,
          JSON.stringify(deps), 'correct', score, null, null, '');
      });
      const roleByTarget = {};
      const roles = db.prepare('SELECT id, name, short FROM roles').all();
      roles.forEach(function (role) { roleByTarget[role.name] = role.id; roleByTarget[role.short] = role.id; });
      let roleCounters = {};
      let seq = main.length;
      dist.forEach(function (r, i) {
        const roleId = roleByTarget[r[2]] || 'other';
        roleCounters[roleId] = (roleCounters[roleId] || 0) + 1;
        const id = 'DS' + sid + '-' + roleId + '-' + roleCounters[roleId];
        ins.run(id, sid, r[0], r[1], r[2], '', r[4] || null, '', '', r[6] || '',
          JSON.stringify([]), seq + i, r[7] || '', r[8] || '', r[9] || '', 0,
          JSON.stringify([]), 'distractor', 10, null, null, '');
      });
    }
    db.prepare('UPDATE scenarios SET nodes=?, updated=? WHERE id=?').run(main.length, today, sid);
  });
  try {
    Object.keys(_PROJECT_MODE).forEach(function (pid) {
      const p = db.prepare('SELECT id FROM projects WHERE id=?').get(pid);
      if (p) db.prepare('UPDATE projects SET mode=? WHERE id=?').run(_PROJECT_MODE[pid], pid);
    });
  } catch (e) {}
  db.prepare("UPDATE projects SET mode='基础版' WHERE mode IS NULL OR mode=''").run();
}

/* ========================================================================
 * 初始化主体
 * ======================================================================== */

function clearAll() {
  const tables = ['notif_reads','notifs','messages','chat_messages','task_attachments','form_submissions',
    'tasks','script_nodes','sim_data','scores','score_items','evidence','snapshots','timeline','seq_flow',
    'ledger','knowledge','audit_logs','impact','dict','scenarios','projects','form_templates',
    'users','user_roles','roles','sessions','settings'];
  db.exec('PRAGMA foreign_keys = OFF');
  tables.forEach(function (t) { try { db.exec('DELETE FROM ' + t); } catch (e) {} });
  db.exec('PRAGMA foreign_keys = ON');
}

function run(opts) {
  opts = opts || {};
  const source = loadSeedSource();
  const SDx = source.seedData;
  const SBLx = source.scenarioBaseline;

  const ts = nowSec();

  tx(function () {
    /* ---------- 角色 ---------- */
    const ri = db.prepare('INSERT INTO roles (id,name,short,icon,color,descr,nav,external,seq) VALUES (?,?,?,?,?,?,?,?,?)');
    SDx.ROLES.forEach(function (r) {
      const nav = (r.nav || []).filter(function (k) { return k !== 'tasks'; });
      if (r.id === 'platformadmin' && nav.indexOf('report') < 0) nav.push('report');
      if (r.id === 'platformadmin' && nav.indexOf('replay') < 0) nav.push('replay');
      if (['commander', 'platformadmin', 'itlead'].indexOf(r.id) >= 0 && nav.indexOf('screen') < 0) nav.push('screen');
      ri.run(r.id, r.name, r.short || '', (SDx.ROLE_ICON && SDx.ROLE_ICON[r.id]) || r.icon || 'user', r.color || '',
        r.descr || '', JSON.stringify(nav), r.external ? 1 : 0, r.seq || 0);
    });

    /* ---------- 用户 + user_roles ---------- */
    const ui = db.prepare('INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?,?,?,?)');
    const ur = db.prepare('INSERT INTO user_roles (user_id,role_id) VALUES (?,?)');
    SDx.USERS.forEach(function (u) {
      const p = hashPassword(INITIAL_PASSWORD);
      const r = ui.run(u.username, u.name, u.role, u.org || '', p.salt, p.hash, '启用', 1, ts);
      ur.run(Number(r.lastInsertRowid), u.role);
    });

    /* ---------- 场景 ---------- */
    const si = db.prepare('INSERT INTO scenarios (id,name,type,level,status,version,updated,nodes,descr,alert) VALUES (?,?,?,?,?,?,?,?,?,?)');
    SDx.SCENARIOS.forEach(function (s) {
      si.run(s.id, s.name, s.type || '', s.level || '一般', s.status || '启用', s.version || 'v1.0',
        ts.slice(0, 10), 0, s.descr || '', s.alert || '');
    });

    /* ---------- 剧本节点（从 scenario_baseline 基线） ---------- */
    applyScenarioNodes(db, SBLx, ts.slice(0, 10));

    /* ---------- 表单模板 ---------- */
    try {
      const forms = require('./forms');
      (forms.DEFAULT_TEMPLATES || SDx.FORM_TEMPLATES || []).forEach(function (f) {
        db.prepare('INSERT OR REPLACE INTO form_templates (id,name,fields) VALUES (?,?,?)').run(f.id, f.name, JSON.stringify(f.fields || []));
      });
    } catch (e) { /* forms 模块不存在时跳过 */ }

    /* ---------- 知识库 ---------- */
    (SDx.KNOWLEDGE || []).forEach(function (k) {
      const body = SDx.KB_CONTENT && SDx.KB_CONTENT[k.id]
        ? JSON.stringify(SDx.KB_CONTENT[k.id])
        : '';
      db.prepare('INSERT INTO knowledge (id,name,cat,updated,size,pages,content,file_path) VALUES (?,?,?,?,?,?,?,?)')
        .run(k.id, k.name, k.cat || '预案', k.updated || ts.slice(0, 10), k.size || '', k.pages || 0, body, null);
    });

    /* ---------- 字典 ---------- */
    Object.keys(SDx.DICT || {}).forEach(function (k) {
      db.prepare('INSERT INTO dict (key,value) VALUES (?,?)').run(k, JSON.stringify(SDx.DICT[k]));
    });

    /* ---------- 场景模拟数据 ---------- */
    Object.keys(SDx.SIM_DATA || {}).forEach(function (k) {
      db.prepare('INSERT INTO sim_data (type,data) VALUES (?,?)').run(k, JSON.stringify(SDx.SIM_DATA[k]));
    });

    /* ---------- 系统设置 ---------- */
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('active_project', '');
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?)').run('initialized_at', ts);
    require('./brand').ensureBrand(db);
  });

  /* 文件兜底加载后自动 snapshot 入库，固化种子 */
  if (!source.fromDb && !opts.skipSnapshot) {
    try { snapshotSeed(SDx, SBLx, 'initial', '首次部署自动固化'); } catch (e) {}
  }

  console.log('[seed] 初始化完成（来源：' + (source.fromDb ? 'seed_baseline@' + source.version : 'seed_data.js 文件') + '）');
  console.log('[seed] 数据库文件：' + require('./db').DB_FILE);
  console.log('[seed] 初始密码：' + INITIAL_PASSWORD);
  console.log('[seed] 账号列表：');
  const rows = db.prepare('SELECT u.username,u.name,r.name AS role FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id').all();
  rows.forEach(function (r) { console.log('        ' + r.username.padEnd(16) + r.name + ' · ' + r.role); });
  const counts = {};
  ['users','roles','scenarios','script_nodes','knowledge','dict','sim_data'].forEach(function (t) {
    try { counts[t] = db.prepare('SELECT COUNT(*) c FROM ' + t).get().c; } catch (e) {}
  });
  console.log('[seed] 数据概览：' + JSON.stringify(counts));
}

/* 当前演练中的项目 id */
function activeProjectId() {
  const SDx = loadSeedSource().seedData;
  const p = (SDx.PROJECTS || []).filter(function (x) { return x.status === '演练中'; })[0];
  return p ? p.id : ((SDx.PROJECTS && SDx.PROJECTS[0]) ? SDx.PROJECTS[0].id : '');
}

/* 系统初始化状态（供 /api/system/status 使用） */
function status() {
  try {
    const users = db.prepare('SELECT COUNT(*) c FROM users').get().c;
    const scenarios = db.prepare('SELECT COUNT(*) c FROM scenarios').get().c;
    const nodes = db.prepare('SELECT COUNT(*) c FROM script_nodes').get().c;
    const roleCount = db.prepare('SELECT COUNT(*) c FROM roles').get().c;
    const projCount = db.prepare('SELECT COUNT(*) c FROM projects').get().c;
    const brand = {};
    db.prepare('SELECT key,value FROM settings').all().forEach(function (r) { brand[r.key] = r.value; });
    let seedVersion = null;
    try {
      const b = db.prepare('SELECT version FROM seed_baseline ORDER BY id DESC LIMIT 1').get();
      if (b) seedVersion = b.version;
    } catch (e) {}
    return {
      initialized: isSeeded(),
      userCount: users,
      roleCount: roleCount,
      scenarioCount: scenarios,
      nodeCount: nodes,
      projectCount: projCount,
      brand_version: brand.brand_version || 'unknown',
      initialized_at: brand.initialized_at || null,
      seedVersion: seedVersion,
      hasBaseline: hasBaseline()
    };
  } catch (e) {
    return { initialized: false, error: e.message };
  }
}

/* 系统重置：清空并重新初始化（供 /api/system/reset 使用） */
function resetAll() {
  const ts = nowSec();
  migrate();
  clearAll();
  run();
  const users = db.prepare('SELECT COUNT(*) c FROM users').get().c;
  return { userCount: users, resetAt: ts };
}

/* 命令行执行 */
if (require.main === module) {
  migrate();
  if (RESET) {
    clearAll();
    run();
  } else if (isSeeded()) {
    console.log('[seed] 数据库已初始化，跳过。如需重建请执行：node src/seed.js --reset');
  } else {
    run();
  }
}

module.exports = { status: status, resetAll: resetAll, exportBaseline: exportBaseline, loadSeedSource: loadSeedSource, hasBaseline: hasBaseline };
