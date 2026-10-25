/* 系统管理接口：用户、角色、审计导出、系统信息（仅平台管理员） */
'use strict';
const express = require('express');
const os = require('node:os');
const fs = require('node:fs');
const path = require('node:path');
const { db, parseJson, DB_FILE, DATA_DIR } = require('../db');
const auth = require('../auth');
const { nowSec } = require('../util');

const router = express.Router();
const BACKUP_DIR = path.join(DATA_DIR, 'backups');

/* 权限矩阵可配置的模块（与前端菜单 key 一致） */
const MODULE_KEYS = ['dashboard', 'workbench', 'collab', 'scenarios', 'script-editor', 'projects', 'screen',
  'evidence', 'replay', 'evaluation', 'report', 'ledger', 'opinion', 'knowledge', 'admin'];

function adminOnly(req, res, next) {
  if (req.user.role.id !== 'platformadmin') {
    return res.status(403).json({ error: '仅平台管理员可访问系统管理功能' });
  }
  next();
}

/* 全部用户角色映射 */
function roleMapOfUsers() {
  const map = {};
  db.prepare(
    'SELECT ur.user_id, r.id, r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id ORDER BY r.seq'
  ).all().forEach(function (x) {
    (map[x.user_id] = map[x.user_id] || []).push({ id: x.id, name: x.name });
  });
  return map;
}

/* 规整角色列表：去空、去重、校验存在、至少一个 */
function normalizeRoles(input) {
  const list = (Array.isArray(input) ? input : [input]).map(function (x) { return String(x || '').trim(); })
    .filter(Boolean).filter(function (v, i, a) { return a.indexOf(v) === i; });
  if (!list.length) return { error: '请至少选择一个角色' };
  for (const rid of list) {
    if (!db.prepare('SELECT id FROM roles WHERE id=?').get(rid)) return { error: '角色不存在：' + rid };
  }
  return { list: list };
}

router.get('/users', auth.requireAuth, adminOnly, function (req, res) {
  const rows = db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,u.created_at,r.name AS role_name ' +
    'FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id'
  ).all();
  const rmap = roleMapOfUsers();
  res.json(rows.map(function (u) {
    const roleList = rmap[u.id] || [{ id: u.role_id, name: u.role_name }];
    return {
      id: u.id, username: u.username, name: u.name, role: u.role_id, roleName: u.role_name,
      roles: roleList.map(function (r) { return r.id; }), roleList: roleList,
      org: u.org, status: u.status, createdAt: u.created_at
    };
  }));
});

router.post('/users', auth.requireAuth, adminOnly, function (req, res) {
  const b = req.body || {};
  const username = String(b.username || '').trim();
  const name = String(b.name || '').trim();
  if (!/^[A-Za-z0-9_.-]{3,32}$/.test(username)) return res.status(400).json({ error: '账号需为 3-32 位字母、数字或 _ . -' });
  if (!name) return res.status(400).json({ error: '姓名不能为空' });
  const nr = normalizeRoles(b.roles !== undefined ? b.roles : b.role);
  if (nr.error) return res.status(400).json({ error: nr.error });
  if (db.prepare('SELECT id FROM users WHERE username=?').get(username)) return res.status(409).json({ error: '账号已存在' });

  const pwd = String(b.password || auth.INITIAL_PASSWORD);
  const p = auth.hashPassword(pwd);
  const r = db.prepare('INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?,?,?,?)')
    .run(username, name, nr.list[0], String(b.org || '盘州市人民医院'), p.salt, p.hash, '启用', 1, nowSec());
  const uid = Number(r.lastInsertRowid);
  const ur = db.prepare('INSERT INTO user_roles (user_id,role_id) VALUES (?,?)');
  nr.list.forEach(function (rid) { ur.run(uid, rid); });
  auth.audit(req, '新建账号 ' + username + '（' + name + '，角色 ' + nr.list.join('/') + '）');
  res.json({ ok: true, id: uid });
});

router.patch('/users/:id', auth.requireAuth, adminOnly, function (req, res) {
  const u = db.prepare('SELECT * FROM users WHERE id=?').get(req.params.id);
  if (!u) return res.status(404).json({ error: '账号不存在' });
  const b = req.body || {};

  let newUsername = u.username;
  if (b.username !== undefined) {
    newUsername = String(b.username || '').trim();
    if (!/^[A-Za-z0-9_.-]{3,32}$/.test(newUsername)) return res.status(400).json({ error: '账号需为 3-32 位字母、数字或 _ . -' });
    const dup = db.prepare('SELECT id FROM users WHERE username=? AND id<>?').get(newUsername, u.id);
    if (dup) return res.status(409).json({ error: '账号已存在' });
  }
  if (b.name !== undefined && !String(b.name).trim()) return res.status(400).json({ error: '姓名不能为空' });

  let newRoles = null;
  if (b.roles !== undefined || b.role !== undefined) {
    const nr = normalizeRoles(b.roles !== undefined ? b.roles : b.role);
    if (nr.error) return res.status(400).json({ error: nr.error });
    newRoles = nr.list;
    const hadAdmin = !!db.prepare('SELECT 1 AS ok FROM user_roles WHERE user_id=? AND role_id=?').get(u.id, 'platformadmin');
    if (hadAdmin && newRoles.indexOf('platformadmin') < 0) {
      if (u.id === req.user.id) return res.status(400).json({ error: '不能移除自己的平台管理员角色' });
      const admins = db.prepare('SELECT COUNT(DISTINCT user_id) AS c FROM user_roles WHERE role_id=?').get('platformadmin').c;
      if (admins <= 1) return res.status(400).json({ error: '至少保留一个平台管理员账号' });
    }
  }
  if (b.status !== undefined) {
    if (u.id === req.user.id && String(b.status) !== '启用') return res.status(400).json({ error: '不能停用当前登录账号' });
  }

  db.exec('BEGIN');
  try {
    db.prepare('UPDATE users SET username=?, name=?, org=?, status=?, role_id=? WHERE id=?').run(
      newUsername,
      b.name !== undefined ? String(b.name).trim() : u.name,
      b.org !== undefined ? String(b.org).trim() : u.org,
      b.status !== undefined ? (String(b.status) === '停用' ? '停用' : '启用') : u.status,
      newRoles ? newRoles[0] : u.role_id,
      u.id
    );
    if (newRoles) {
      db.prepare('DELETE FROM user_roles WHERE user_id=?').run(u.id);
      const ur = db.prepare('INSERT INTO user_roles (user_id,role_id) VALUES (?,?)');
      newRoles.forEach(function (rid) { ur.run(u.id, rid); });
      /* 已登录会话中不再持有的角色：强制该会话重新登录 */
      const del = db.prepare('DELETE FROM sessions WHERE user_id=? AND role_id IS NOT NULL AND role_id NOT IN (' +
        newRoles.map(function () { return '?'; }).join(',') + ')');
      del.run.apply(del, [u.id].concat(newRoles));
    }
    db.exec('COMMIT');
  } catch (e) {
    try { db.exec('ROLLBACK'); } catch (e2) {}
    return res.status(500).json({ error: '保存失败：' + e.message });
  }

  const unameMsg = newUsername !== u.username ? ('（账号 ' + u.username + ' → ' + newUsername + '）') : '';
  auth.audit(req, '修改账号 ' + newUsername + unameMsg + (newRoles ? ' 角色 ' + newRoles.join('/') : ''));
  res.json({ ok: true, username: newUsername, roles: newRoles || undefined });
});

/* 重置密码 */
router.post('/users/:id/reset-password', auth.requireAuth, adminOnly, function (req, res) {
  const u = db.prepare('SELECT * FROM users WHERE id=?').get(req.params.id);
  if (!u) return res.status(404).json({ error: '账号不存在' });
  const pwd = String((req.body && req.body.password) || auth.INITIAL_PASSWORD);
  if (pwd.length < 8 || !/[A-Za-z]/.test(pwd) || !/[0-9]/.test(pwd)) {
    return res.status(400).json({ error: '密码需至少 8 位，且同时包含字母与数字' });
  }
  const p = auth.hashPassword(pwd);
  db.prepare('UPDATE users SET pwd_salt=?, pwd_hash=?, must_change=1 WHERE id=?').run(p.salt, p.hash, u.id);
  db.prepare('DELETE FROM sessions WHERE user_id=?').run(u.id);
  auth.audit(req, '重置账号 ' + u.username + ' 密码');
  res.json({ ok: true });
});

router.get('/roles', auth.requireAuth, adminOnly, function (req, res) {
  res.json(db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return { id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color, desc: r.descr, nav: parseJson(r.nav, []), external: !!r.external, status: r.status || '启用' };
  }));
});

/* 调整角色显示顺序（同步登录页角色卡片顺序） */
router.patch('/roles/order', auth.requireAuth, adminOnly, function (req, res) {
  const ids = Array.isArray(req.body && req.body.ids) ? req.body.ids.map(String) : [];
  if (!ids.length) return res.status(400).json({ error: '请提供角色顺序' });
  const all = db.prepare('SELECT id FROM roles').all().map(function (r) { return r.id; });
  if (ids.some(function (id) { return all.indexOf(id) < 0; })) return res.status(400).json({ error: '角色不存在' });
  const up = db.prepare('UPDATE roles SET seq=? WHERE id=?');
  db.exec('BEGIN');
  try {
    ids.forEach(function (id, i) { up.run(i + 1, id); });
    db.exec('COMMIT');
  } catch (e) {
    try { db.exec('ROLLBACK'); } catch (e2) {}
    return res.status(500).json({ error: '排序保存失败：' + e.message });
  }
  auth.audit(req, '调整角色显示顺序');
  res.json({ ok: true });
});

/* 停用 / 启用角色（停用后登录页与账号角色选择不再显示） */
router.patch('/roles/:id/status', auth.requireAuth, adminOnly, function (req, res) {
  const r = db.prepare('SELECT * FROM roles WHERE id=?').get(req.params.id);
  if (!r) return res.status(404).json({ error: '角色不存在' });
  const st = String((req.body && req.body.status) || '');
  if (st !== '启用' && st !== '停用') return res.status(400).json({ error: '状态不正确' });
  if (st === '停用') {
    if (r.id === 'platformadmin') return res.status(400).json({ error: '平台管理员角色不可停用' });
    if (r.id === req.user.role.id) return res.status(400).json({ error: '不能停用当前登录角色' });
    /* 已用该角色登录的会话强制失效 */
    db.prepare('DELETE FROM sessions WHERE role_id=?').run(r.id);
  }
  db.prepare('UPDATE roles SET status=? WHERE id=?').run(st, r.id);
  auth.audit(req, (st === '停用' ? '停用角色：' : '启用角色：') + r.name);
  res.json({ ok: true, status: st });
});

/* 删除角色（仍有账号绑定时禁止删除） */
router.delete('/roles/:id', auth.requireAuth, adminOnly, function (req, res) {
  const r = db.prepare('SELECT * FROM roles WHERE id=?').get(req.params.id);
  if (!r) return res.status(404).json({ error: '角色不存在' });
  if (r.id === 'platformadmin') return res.status(400).json({ error: '平台管理员角色不可删除' });
  if (r.id === req.user.role.id) return res.status(400).json({ error: '不能删除当前登录角色' });
  const cnt = db.prepare('SELECT COUNT(*) AS c FROM user_roles WHERE role_id=?').get(r.id).c;
  if (cnt) {
    return res.status(409).json({
      error: '该角色仍有 ' + cnt + ' 个账号绑定，请先在「用户账号」中解除绑定，或改用停用'
    });
  }
  db.prepare('DELETE FROM roles WHERE id=?').run(r.id);
  auth.audit(req, '删除角色：' + r.name);
  res.json({ ok: true });
});

/* 新增角色（权限矩阵可扩展） */
router.post('/roles', auth.requireAuth, adminOnly, function (req, res) {
  const b = req.body || {};
  const name = String(b.name || '').trim();
  if (!name) return res.status(400).json({ error: '角色名称不能为空' });
  if (name.length > 20) return res.status(400).json({ error: '角色名称不能超过 20 个字' });
  if (db.prepare('SELECT id FROM roles WHERE name=?').get(name)) return res.status(409).json({ error: '角色名称已存在' });
  if (b.nav !== undefined && !Array.isArray(b.nav)) return res.status(400).json({ error: '权限列表格式不正确' });
  const nav = (b.nav || []).map(String).filter(function (k) { return MODULE_KEYS.indexOf(k) >= 0; })
    .filter(function (v, i, a) { return a.indexOf(v) === i; });
  const desc = String(b.desc || '').slice(0, 200);
  const id = 'role_' + Date.now().toString(36) + Math.random().toString(36).slice(2, 5);
  const seq = (db.prepare('SELECT MAX(seq) AS m FROM roles').get().m || 0) + 1;
  db.prepare('INSERT INTO roles (id,name,short,icon,color,descr,nav,external,status,seq) VALUES (?,?,?,?,?,?,?,?,?,?)')
    .run(id, name, name.slice(0, 8), 'user', '#4f7cff', desc, JSON.stringify(nav), b.external ? 1 : 0, '启用', seq);
  auth.audit(req, '新增角色：' + name + '（模块 ' + nav.length + ' 个）');
  res.json({
    ok: true,
    role: { id: id, name: name, short: name.slice(0, 8), icon: 'user', color: '#4f7cff', desc: desc, nav: nav, external: !!b.external, status: '启用' }
  });
});

/* 修改角色说明与模块权限（平台管理员「系统管理」权限不可移除） */
router.put('/roles/:id', auth.requireAuth, adminOnly, function (req, res) {
  const r = db.prepare('SELECT * FROM roles WHERE id=?').get(req.params.id);
  if (!r) return res.status(404).json({ error: '角色不存在' });
  const b = req.body || {};
  let nav = parseJson(r.nav, []);
  if (b.nav !== undefined) {
    if (!Array.isArray(b.nav)) return res.status(400).json({ error: '权限列表格式不正确' });
    nav = b.nav.map(String).filter(function (k) { return MODULE_KEYS.indexOf(k) >= 0; })
      .filter(function (v, i, a) { return a.indexOf(v) === i; });
    if (r.id === 'platformadmin' && nav.indexOf('admin') < 0) nav.push('admin');
  }
  const desc = b.desc !== undefined ? String(b.desc || '').slice(0, 200) : r.descr;
  let name = r.name;
  if (b.name !== undefined) {
    name = String(b.name || '').trim();
    if (!name) return res.status(400).json({ error: '角色名称不能为空' });
    if (name.length > 20) return res.status(400).json({ error: '角色名称不能超过 20 个字' });
  }
  db.prepare('UPDATE roles SET name=?, nav=?, descr=? WHERE id=?').run(name, JSON.stringify(nav), desc, r.id);
  auth.audit(req, '更新角色' + (name !== r.name ? '（' + r.name + ' → ' + name + '）' : '权限：' + r.name) + '，模块 ' + nav.length + ' 个');
  res.json({
    ok: true,
    role: { id: r.id, name: name, short: r.short, icon: r.icon, color: r.color, desc: desc, nav: nav, external: !!r.external }
  });
});

/* 表单模板：列表、新建与编辑（内置模板同样可编辑） */
router.get('/forms', auth.requireAuth, adminOnly, function (req, res) {
  res.json(db.prepare('SELECT * FROM form_templates ORDER BY id').all().map(function (f) {
    return require('../forms').normalizeTemplate(f);
  }));
});

router.post('/forms', auth.requireAuth, adminOnly, function (req, res) {
  const b = req.body || {};
  const name = String(b.name || '').trim().slice(0, 40);
  const fields = require('../forms').normalizeFields(b.fields);
  if (!name) return res.status(400).json({ error: '模板名称不能为空' });
  if (!fields.length) return res.status(400).json({ error: '字段不能为空' });
  const maxId = db.prepare("SELECT MAX(CAST(SUBSTR(id,2) AS INTEGER)) AS n FROM form_templates").get().n || 0;
  const id = 'F' + (maxId + 1);
  db.prepare('INSERT INTO form_templates (id,name,fields) VALUES (?,?,?)').run(id, name, JSON.stringify(fields));
  auth.audit(req, '新建表单模板《' + name + '》（字段 ' + fields.length + ' 个）');
  res.json({ ok: true, id: id, name: name, fields: fields });
});

router.put('/forms/:id', auth.requireAuth, adminOnly, function (req, res) {
  const tpl = db.prepare('SELECT * FROM form_templates WHERE id=?').get(req.params.id);
  if (!tpl) return res.status(404).json({ error: '表单模板不存在' });
  const b = req.body || {};
  const name = String(b.name || '').trim().slice(0, 40);
  const fields = require('../forms').normalizeFields(b.fields);
  if (!name) return res.status(400).json({ error: '模板名称不能为空' });
  if (!fields.length) return res.status(400).json({ error: '字段不能为空' });
  const seen = {};
  for (let i = 0; i < fields.length; i++) {
    if (seen[fields[i].label]) return res.status(400).json({ error: '字段名称重复：' + fields[i].label });
    seen[fields[i].label] = 1;
  }
  db.prepare('UPDATE form_templates SET name=?, fields=? WHERE id=?').run(name, JSON.stringify(fields), tpl.id);
  auth.audit(req, '更新表单模板《' + name + '》（字段 ' + fields.length + ' 个）');
  res.json({ ok: true, template: { id: tpl.id, name: name, fields: fields } });
});

/* 数据备份：将 SQLite 单文件复制到 data/backups/（WAL 检查点后复制） */
router.post('/backup', auth.requireAuth, adminOnly, function (req, res) {
  if (!fs.existsSync(BACKUP_DIR)) fs.mkdirSync(BACKUP_DIR, { recursive: true });
  try { db.exec('PRAGMA wal_checkpoint(TRUNCATE)'); } catch (e) {}
  const ts = nowSec().replace(/[-: ]/g, '');
  const name = 'drill-' + ts + '.db';
  const target = path.join(BACKUP_DIR, name);
  fs.copyFileSync(DB_FILE, target);
  const size = fs.statSync(target).size;
  auth.audit(req, '执行数据备份：' + name);
  res.json({ ok: true, file: name, path: target, size: size, time: nowSec() });
});

router.get('/backups', auth.requireAuth, adminOnly, function (req, res) {
  if (!fs.existsSync(BACKUP_DIR)) return res.json([]);
  res.json(fs.readdirSync(BACKUP_DIR).filter(function (f) { return /\.db$/.test(f); }).map(function (f) {
    const st = fs.statSync(path.join(BACKUP_DIR, f));
    const d = st.mtime;
    const p2 = function (n) { return n < 10 ? '0' + n : String(n); };
    return { file: f, size: st.size, time: d.getFullYear() + '-' + p2(d.getMonth() + 1) + '-' + p2(d.getDate()) +
      ' ' + p2(d.getHours()) + ':' + p2(d.getMinutes()) + ':' + p2(d.getSeconds()) };
  }).sort(function (a, b) { return a.file < b.file ? 1 : -1; }));
});

/* 演练报告数据聚合：按项目汇总概况、时序、任务、评分、取证，供导出标准报告 */
router.get('/report/:projectId', auth.requireAuth, adminOnly, function (req, res) {
  const pid = String(req.params.projectId || '');
  const project = db.prepare('SELECT * FROM projects WHERE id=?').get(pid);
  if (!project) return res.status(404).json({ error: '演练项目不存在' });

  const scenario = project.scenario_id
    ? db.prepare('SELECT * FROM scenarios WHERE id=?').get(project.scenario_id) : null;
  const nodes = project.scenario_id
    ? db.prepare('SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=?').get(project.scenario_id).c : 0;

  const roleName = function (rid) {
    const r = db.prepare('SELECT name,short FROM roles WHERE id=?').get(rid);
    return r ? { name: r.name, short: r.short } : { name: rid, short: rid };
  };

  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=? ORDER BY seq,id').all(pid).map(function (t) {
    const r = roleName(t.role_id);
    return { id: t.id, title: t.title, descr: t.descr, role: t.role_id, roleName: r.name, level: t.level, deadline: t.deadline, status: t.status };
  });
  const total = tasks.length;
  const done = tasks.filter(function (t) { return t.status === '已完成'; }).length;
  const doing = tasks.filter(function (t) { return t.status === '处理中'; }).length;
  const pending = tasks.filter(function (t) { return t.status === '待处理'; }).length;
  const overdue = tasks.filter(function (t) { return t.status === '超时'; }).length;

  const byRole = {};
  tasks.forEach(function (t) {
    const k = t.roleName;
    byRole[k] = byRole[k] || { role: k, total: 0, done: 0, doing: 0, pending: 0, overdue: 0 };
    byRole[k].total++;
    if (t.status === '已完成') byRole[k].done++;
    else if (t.status === '处理中') byRole[k].doing++;
    else if (t.status === '超时') byRole[k].overdue++;
    else byRole[k].pending++;
  });

  const timeline = db.prepare('SELECT * FROM timeline WHERE project_id=? ORDER BY seq,id').all(pid).map(function (x) {
    return { t: x.t, actor: x.actor, role: roleName(x.role_id).name, action: x.action, detail: x.detail, level: x.level };
  });
  const seqFlow = db.prepare('SELECT * FROM seq_flow WHERE project_id=? ORDER BY seq,id').all(pid).map(function (x) {
    return { name: x.name, time: x.time, role: x.role, state: x.state };
  });

  const evidence = db.prepare(
    'SELECT * FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?) ORDER BY created_at,id'
  ).all(pid, pid).map(function (e) {
    return { id: e.id, type: e.type, name: e.name, size: e.size, time: e.time, role: e.role_id, taskId: e.task_id, auto: !!e.auto };
  });
  const evByType = {};
  evidence.forEach(function (e) { evByType[e.type] = (evByType[e.type] || 0) + 1; });
  const autoEv = evidence.filter(function (e) { return e.auto; }).length;

  /* 实战版处置决策记录（角色工作台提交） */
  const decisions = db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(pid).map(function (s) {
    const node = s.matched_node
      ? db.prepare('SELECT id,title,target,time,timeout FROM script_nodes WHERE id=?').get(s.matched_node) : null;
    return {
      nodeId: s.matched_node, title: node ? node.title : s.title, target: node ? node.target : '',
      role: s.role_id, roleName: roleName(s.role_id).name, choice: s.choice || s.title,
      correct: s.correct === 0 ? 0 : 1, ontime: !!s.ontime, stepTime: s.step_time,
      foundAt: s.found_at, found: s.found, measure: s.measure, result: s.result, user: s.user
    };
  });
  const baseline = project.scenario_id
    ? db.prepare("SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=? AND required=1 AND type<>'end'").get(project.scenario_id).c : 0;
  const decisionStats = {
    baseline: baseline,
    total: decisions.length,
    correct: decisions.filter(function (d) { return d.correct; }).length,
    wrong: decisions.filter(function (d) { return !d.correct; }).length,
    ontime: decisions.filter(function (d) { return d.ontime; }).length
  };

  const items = db.prepare('SELECT * FROM score_items ORDER BY seq,id').all();
  const scoreRows = db.prepare('SELECT * FROM scores WHERE project_id=?').all(pid);
  const scores = items.map(function (k) {
    const s = scoreRows.filter(function (x) { return x.item_id === k.id; })[0];
    return { itemId: k.id, name: k.name, weight: k.weight, rule: k.rule, max: s ? s.max : k.weight, score: s ? s.score : null, note: s ? s.note : '' };
  });
  const scored = scores.filter(function (s) { return s.score !== null; });
  const scoreSum = scored.reduce(function (a, s) { return a + Number(s.score || 0); }, 0);
  const scoreMax = scores.reduce(function (a, s) { return a + Number(s.max || s.weight || 0); }, 0);
  const scorePct = (scored.length && scoreMax) ? Math.round(scoreSum / scoreMax * 100) : null;
  /* 模式化自动考评（复盘页「重新自动考评」写入的 MODE 行） */
  const modeRow = scoreRows.filter(function (x) { return x.item_id === 'MODE'; })[0];

  const ledger = db.prepare('SELECT * FROM ledger WHERE project_id=?').get(pid) || null;

  res.json({
    project: {
      id: project.id, name: project.name, type: project.type, date: project.date, level: project.level,
      status: project.status, target: project.target, scenarioId: project.scenario_id,
      mode: project.mode || '基础版',
      units: parseJson(project.units, []), members: parseJson(project.members, []), observers: parseJson(project.observers, [])
    },
    scenario: scenario ? { id: scenario.id, name: scenario.name, type: scenario.type, level: scenario.level, version: scenario.version, nodes: nodes, descr: scenario.descr } : null,
    stats: { total: total, done: done, doing: doing, pending: pending, overdue: overdue, rate: total ? Math.round(done / total * 100) : 0 },
    byRole: Object.keys(byRole).map(function (k) { return byRole[k]; }),
    tasks: tasks, timeline: timeline, seqFlow: seqFlow,
    decisions: decisions, decisionStats: decisionStats,
    evidence: evidence, evidenceStats: { total: evidence.length, auto: autoEv, byType: evByType },
    scores: scores, scoreStats: { sum: scoreSum, max: scoreMax, pct: scorePct, graded: scored.length, items: scores.length },
    modeScore: modeRow ? { score: modeRow.score, max: modeRow.max, note: modeRow.note, updatedAt: modeRow.updated_at } : null,
    ledger: ledger,
    generatedAt: nowSec(),
    generatedBy: req.user.name
  });
});

/* 报告导出留痕 */
router.post('/report/:projectId/export', auth.requireAuth, adminOnly, function (req, res) {
  const pid = String(req.params.projectId || '');
  const p = db.prepare('SELECT name FROM projects WHERE id=?').get(pid);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  const fmt = String((req.body || {}).format || 'HTML');
  auth.audit(req, '导出演练报告《' + p.name + '》（' + fmt + '）');
  res.json({ ok: true, time: nowSec() });
});

/* 审计日志导出（CSV），仅追加不可修改 */
router.get('/audit/export', auth.requireAuth, adminOnly, function (req, res) {
  const rows = db.prepare('SELECT * FROM audit_logs ORDER BY id').all();
  const head = '时间,操作人,角色,操作内容,来源IP\n';
  const body = rows.map(function (a) {
    return [a.time, a.user, a.role_id, String(a.action).replace(/[",\n]/g, ' '), a.ip].join(',');
  }).join('\n');
  const ts = nowSec().replace(/[: ]/g, '-');
  auth.audit(req, '导出审计日志（' + rows.length + ' 条）');
  res.setHeader('Content-Type', 'text/csv; charset=utf-8');
  res.setHeader('Content-Disposition', 'attachment; filename="audit-' + ts + '.csv"');
  res.send('\ufeff' + head + body);
});

/* 系统信息 */
router.get('/sysinfo', auth.requireAuth, adminOnly, function (req, res) {
  const counts = {};
  ['users', 'roles', 'projects', 'tasks', 'evidence', 'timeline', 'audit_logs', 'knowledge'].forEach(function (t) {
    counts[t] = db.prepare('SELECT COUNT(*) AS c FROM ' + t).get().c;
  });
  res.json({
    node: process.version,
    platform: process.platform + ' ' + process.arch,
    hostname: os.hostname(),
    uptimeMin: Math.round(process.uptime() / 60),
    dbFile: DB_FILE,
    serverTime: nowSec(),
    counts: counts
  });
});

module.exports = router;
