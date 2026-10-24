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

function adminOnly(req, res, next) {
  if (req.user.role.id !== 'platformadmin') {
    return res.status(403).json({ error: '仅平台管理员可访问系统管理功能' });
  }
  next();
}

router.get('/users', auth.requireAuth, adminOnly, function (req, res) {
  const rows = db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,u.created_at,r.name AS role_name ' +
    'FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id'
  ).all();
  res.json(rows.map(function (u) {
    return { id: u.id, username: u.username, name: u.name, role: u.role_id, roleName: u.role_name, org: u.org, status: u.status, createdAt: u.created_at };
  }));
});

router.post('/users', auth.requireAuth, adminOnly, function (req, res) {
  const b = req.body || {};
  const username = String(b.username || '').trim();
  const name = String(b.name || '').trim();
  const roleId = String(b.role || '').trim();
  if (!/^[A-Za-z0-9_.-]{3,32}$/.test(username)) return res.status(400).json({ error: '账号需为 3-32 位字母、数字或 _ . -' });
  if (!name) return res.status(400).json({ error: '姓名不能为空' });
  if (!db.prepare('SELECT id FROM roles WHERE id=?').get(roleId)) return res.status(400).json({ error: '角色不存在' });
  if (db.prepare('SELECT id FROM users WHERE username=?').get(username)) return res.status(409).json({ error: '账号已存在' });

  const pwd = String(b.password || auth.INITIAL_PASSWORD);
  const p = auth.hashPassword(pwd);
  const r = db.prepare('INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?,?,?,?)')
    .run(username, name, roleId, String(b.org || '盘州市人民医院'), p.salt, p.hash, '启用', 1, nowSec());
  auth.audit(req, '新建账号 ' + username + '（' + name + '）');
  res.json({ ok: true, id: Number(r.lastInsertRowid) });
});

router.patch('/users/:id', auth.requireAuth, adminOnly, function (req, res) {
  const u = db.prepare('SELECT * FROM users WHERE id=?').get(req.params.id);
  if (!u) return res.status(404).json({ error: '账号不存在' });
  const b = req.body || {};
  if (b.name !== undefined) db.prepare('UPDATE users SET name=? WHERE id=?').run(String(b.name), u.id);
  if (b.org !== undefined) db.prepare('UPDATE users SET org=? WHERE id=?').run(String(b.org), u.id);
  if (b.role !== undefined) {
    if (!db.prepare('SELECT id FROM roles WHERE id=?').get(String(b.role))) return res.status(400).json({ error: '角色不存在' });
    db.prepare('UPDATE users SET role_id=? WHERE id=?').run(String(b.role), u.id);
  }
  if (b.status !== undefined) {
    if (u.id === req.user.id && String(b.status) !== '启用') return res.status(400).json({ error: '不能停用当前登录账号' });
    db.prepare('UPDATE users SET status=? WHERE id=?').run(String(b.status) === '停用' ? '停用' : '启用', u.id);
  }
  auth.audit(req, '修改账号 ' + u.username);
  res.json({ ok: true });
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
    return { id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color, desc: r.descr, nav: parseJson(r.nav, []), external: !!r.external };
  }));
});

/* 表单模板：列表与新建 */
router.get('/forms', auth.requireAuth, adminOnly, function (req, res) {
  res.json(db.prepare('SELECT * FROM form_templates ORDER BY id').all().map(function (f) {
    return { id: f.id, name: f.name, fields: parseJson(f.fields, []) };
  }));
});

router.post('/forms', auth.requireAuth, adminOnly, function (req, res) {
  const b = req.body || {};
  const name = String(b.name || '').trim();
  const fields = Array.isArray(b.fields) ? b.fields.map(String).filter(Boolean) : [];
  if (!name) return res.status(400).json({ error: '模板名称不能为空' });
  if (!fields.length) return res.status(400).json({ error: '字段不能为空' });
  const maxId = db.prepare("SELECT MAX(CAST(SUBSTR(id,2) AS INTEGER)) AS n FROM form_templates").get().n || 0;
  const id = 'F' + (maxId + 1);
  db.prepare('INSERT INTO form_templates (id,name,fields) VALUES (?,?,?)').run(id, name, JSON.stringify(fields));
  auth.audit(req, '新建表单模板《' + name + '》');
  res.json({ ok: true, id: id, name: name, fields: fields });
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
