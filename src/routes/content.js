/* 内容类接口：消息、通知、会商、场景、项目、快照、时序、评估、台账、舆情、知识库、审计、影响、字典 */
'use strict';
const express = require('express');
const path = require('node:path');
const fs = require('node:fs');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { makeUploader, handleUploadError } = require('../upload');
const { now, hhmm, fmtSize, nowSec } = require('../util');

const router = express.Router();
const kbUpload = makeUploader('knowledge');

const MODES = ['基础版', '实战版'];

function activeProjectId() {
  const r = db.prepare("SELECT value FROM settings WHERE key='active_project'").get();
  return r ? r.value : null;
}

/* ---------- 消息流 ---------- */
router.get('/messages', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT * FROM messages ORDER BY seq').all();
  res.json(rows.map(function (m) {
    return { id: m.id, time: m.time, level: m.level, source: m.source, text: m.text, targets: parseJson(m.targets, []) };
  }));
});

router.post('/messages', auth.requireAuth, function (req, res) {
  const b = req.body || {};
  const rid = req.user.role.id;
  if (rid !== 'commander' && rid !== 'itlead' && rid !== 'platformadmin') {
    return res.status(403).json({ error: '仅指挥部/信息科负责人可发布指令类消息' });
  }
  const seq = db.prepare('SELECT COALESCE(MAX(seq),0)+1 AS n FROM messages').get().n;
  const id = 'M' + Date.now();
  db.prepare('INSERT INTO messages (id,time,level,source,text,targets,seq) VALUES (?,?,?,?,?,?,?)')
    .run(id, hhmm(), String(b.level || 'P2'), String(b.source || (req.user.name + '（人工触发）')),
      String(b.text || ''), JSON.stringify(b.targets || ['*']), seq);
  auth.audit(req, '发布消息：' + String(b.text || '').slice(0, 60));
  res.json({ ok: true, id: id });
});

/* ---------- 通知 ---------- */
router.get('/notifs', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT * FROM notifs ORDER BY seq').all();
  const read = db.prepare('SELECT notif_id FROM notif_reads WHERE user_id=?').all(req.user.id)
    .map(function (r) { return r.notif_id; });
  res.json(rows.map(function (n) {
    return { id: n.id, time: n.time, level: n.level, source: n.source, title: n.title, desc: n.descr, read: read.indexOf(n.id) >= 0 };
  }));
});

router.post('/notifs/read', auth.requireAuth, function (req, res) {
  const ids = (req.body && Array.isArray(req.body.ids)) ? req.body.ids : db.prepare('SELECT id FROM notifs').all().map(function (n) { return n.id; });
  const st = db.prepare('INSERT OR IGNORE INTO notif_reads (user_id,notif_id) VALUES (?,?)');
  ids.forEach(function (id) { st.run(req.user.id, String(id)); });
  res.json({ ok: true });
});

/* ---------- 会商 ---------- */
router.get('/chat/:group', auth.requireAuth, function (req, res) {
  const g = String(req.params.group);
  const rows = db.prepare('SELECT * FROM chat_messages WHERE grp=? ORDER BY id').all(g);
  res.json(rows.map(function (c) {
    return { sender: c.sender, role: c.role_id, time: c.time, text: c.text, att: c.att || null, url: c.file_path ? '/uploads/' + c.file_path : null };
  }));
});

router.post('/chat/:group', auth.requireAuth, function (req, res) {
  const g = String(req.params.group);
  if (['inner', 'joint'].indexOf(g) < 0) return res.status(400).json({ error: '会商组不存在' });
  const b = req.body || {};
  const text = String(b.text || '').slice(0, 2000);
  if (!text && !b.att) return res.status(400).json({ error: '消息内容不能为空' });
  const r = db.prepare('INSERT INTO chat_messages (grp,sender,role_id,user_id,time,text,att) VALUES (?,?,?,?,?,?,?)')
    .run(g, req.user.name, req.user.role.id, req.user.id, hhmm(), text, b.att ? String(b.att) : null);
  res.json({ ok: true, id: Number(r.lastInsertRowid) });
});

/* ---------- 场景剧本 ---------- */
router.get('/scenarios', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM scenarios').all().map(function (s) {
    return { id: s.id, name: s.name, type: s.type, level: s.level, difficulty: s.difficulty, status: s.status, version: s.version, updated: s.updated, nodes: s.nodes, desc: s.descr };
  }));
});

router.patch('/scenarios/:id', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '仅平台管理员/指挥部/信息科负责人可维护剧本' });
  }
  const s = db.prepare('SELECT * FROM scenarios WHERE id=?').get(req.params.id);
  if (!s) return res.status(404).json({ error: '剧本不存在' });
  const b = req.body || {};
  const fields = { name: 'name', type: 'type', level: 'level', difficulty: 'difficulty', status: 'status', desc: 'descr', version: 'version', nodes: 'nodes' };
  Object.keys(fields).forEach(function (k) {
    if (b[k] !== undefined) db.prepare('UPDATE scenarios SET ' + fields[k] + '=? WHERE id=?').run(String(b[k]), s.id);
  });
  db.prepare("UPDATE scenarios SET updated=? WHERE id=?").run(now().slice(0, 10), s.id);
  auth.audit(req, '更新剧本《' + s.name + '》');
  res.json({ ok: true });
});

router.post('/scenarios', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '仅平台管理员/指挥部/信息科负责人可新建剧本' });
  }
  const b = req.body || {};
  if (!b.name) return res.status(400).json({ error: '剧本名称不能为空' });
  const maxId = db.prepare("SELECT MAX(CAST(SUBSTR(id,2) AS INTEGER)) AS n FROM scenarios").get().n || 0;
  const id = 'S' + String(maxId + 1).padStart(2, '0');
  db.prepare('INSERT INTO scenarios (id,name,type,level,difficulty,status,version,updated,nodes,descr) VALUES (?,?,?,?,?,?,?,?,?,?)')
    .run(id, String(b.name), String(b.type || ''), String(b.level || '一般'), String(b.difficulty || '实战版'),
      '草稿', 'v1.0', now().slice(0, 10), 0, String(b.desc || ''));
  auth.audit(req, '新建剧本《' + b.name + '》');
  res.json({ ok: true, id: id });
});

/* ---------- 剧本节点保存（可视化编排） ---------- */
router.get('/script-nodes', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM script_nodes ORDER BY scenario_id,seq').all().map(function (n) {
    return { id: n.id, scenarioId: n.scenario_id, type: n.type, title: n.title, target: n.target, time: n.time, timeout: n.timeout, form: n.form || undefined, att: n.att || undefined, desc: n.descr, branches: parseJson(n.branches, []) };
  }));
});

function bumpVersion(v) {
  const m = /^v(\d+)\.(\d+)$/.exec(String(v || 'v1.0'));
  return m ? 'v' + m[1] + '.' + (Number(m[2]) + 1) : 'v1.1';
}

router.post('/script-nodes', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '无剧本编排权限' });
  }
  const nodes = Array.isArray(req.body && req.body.nodes) ? req.body.nodes : null;
  if (!nodes) return res.status(400).json({ error: '节点数据格式错误' });
  const scenario = String((req.body && req.body.scenarioId) || 'S01');
  const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(scenario);
  if (!sc) return res.status(404).json({ error: '场景剧本不存在' });
  const del = db.prepare('DELETE FROM script_nodes WHERE scenario_id=?');
  const ins = db.prepare('INSERT INTO script_nodes (id,scenario_id,type,title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,action,required) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)');
  del.run(scenario);
  nodes.forEach(function (n, i) {
    ins.run(String(n.id || ('N' + (i + 1))), scenario, String(n.type || 'task'), String(n.title || ''), String(n.target || ''),
      String(n.time || ''), n.timeout || null, String(n.form || ''), String(n.att || ''), String(n.desc || ''),
      JSON.stringify(n.branches || []), i, String(n.guide || ''), String(n.keywords || ''),
      String(n.action || ''), n.required === false ? 0 : 1);
  });
  /* 同步场景剧本库：节点数、版本号、更新时间、难度（版本号在保存时递增，保证剧本库版本一致） */
  const version = bumpVersion(sc.version);
  const updated = now().slice(0, 10);
  const difficulty = String((req.body && req.body.difficulty) || sc.difficulty || '');
  db.prepare('UPDATE scenarios SET nodes=?, version=?, updated=?, difficulty=? WHERE id=?')
    .run(nodes.length, version, updated, difficulty, scenario);
  auth.audit(req, '保存剧本编排《' + sc.name + '》' + nodes.length + ' 个节点，版本 ' + sc.version + '→' + version);
  res.json({ ok: true, count: nodes.length, scenarioId: scenario, version: version, updated: updated, difficulty: difficulty });
});

/* ---------- 项目 ---------- */
function projectDto(p) {
  return {
    id: p.id, name: p.name, type: p.type, date: p.date, scenarioId: p.scenario_id, level: p.level,
    status: p.status, target: p.target, mode: p.mode || '基础版',
    units: parseJson(p.units, []),
    members: parseJson(p.members, []), observers: parseJson(p.observers, [])
  };
}

router.get('/projects', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM projects ORDER BY id').all().map(projectDto));
});

router.post('/projects', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '仅平台管理员/指挥部/信息科负责人可新建演练项目' });
  }
  const b = req.body || {};
  const name = String(b.name || '').trim();
  if (!name) return res.status(400).json({ error: '项目名称不能为空' });
  if (name.length > 80) return res.status(400).json({ error: '项目名称不能超过 80 个字符' });
  const date = String(b.date || '').trim() || now();
  if (!/^\d{4}-\d{2}-\d{2}/.test(date)) return res.status(400).json({ error: '计划日期格式应为 YYYY-MM-DD' });
  const scenarioId = String(b.scenarioId || '');
  if (scenarioId && !db.prepare('SELECT id FROM scenarios WHERE id=?').get(scenarioId)) {
    return res.status(400).json({ error: '关联剧本不存在：' + scenarioId });
  }
  const sc = scenarioId ? db.prepare('SELECT * FROM scenarios WHERE id=?').get(scenarioId) : null;
  const mode = String(b.mode || '基础版');
  if (MODES.indexOf(mode) < 0) return res.status(400).json({ error: '演练模式应为 基础版 或 实战版' });

  /* 项目编号按当年最大序号递增（P2026-013），避免与已有编号冲突 */
  const year = new Date().getFullYear();
  let maxSeq = 0;
  db.prepare('SELECT id FROM projects').all().forEach(function (r) {
    const m = new RegExp('^P' + year + '-(\\d+)$').exec(r.id);
    if (m) maxSeq = Math.max(maxSeq, Number(m[1]));
  });
  const id = 'P' + year + '-' + String(maxSeq + 1).padStart(3, '0');

  const units = Array.isArray(b.units) && b.units.length ? b.units : ['盘州市人民医院'];
  const members = Array.isArray(b.members) ? b.members : [];
  const observers = Array.isArray(b.observers) ? b.observers : [];
  db.prepare('INSERT INTO projects (id,name,type,date,scenario_id,level,status,target,units,members,observers,mode) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(id, name, String(b.type || '实战协同演练'), date,
      scenarioId, String(b.level || (sc && sc.level) || '一般'), '未开始', String(b.target || ''),
      JSON.stringify(units), JSON.stringify(members), JSON.stringify(observers), mode);
  auth.audit(req, '新建演练项目 ' + id + '《' + name + '》');
  res.json({ ok: true, id: id, project: projectDto(db.prepare('SELECT * FROM projects WHERE id=?').get(id)) });
});

/* ---------- 快照 ---------- */
router.get('/snapshots', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM snapshots ORDER BY id').all().map(function (s) {
    return { id: s.id, name: s.name, time: s.time, by: s.by_user, note: s.note };
  }));
});

router.post('/snapshots', auth.requireAuth, function (req, res) {
  const b = req.body || {};
  const id = 'SP' + Date.now();
  db.prepare('INSERT INTO snapshots (id,project_id,name,time,by_user,note) VALUES (?,?,?,?,?,?)')
    .run(id, activeProjectId(), String(b.name || '手动快照'), hhmm(), req.user.name, String(b.note || ''));
  auth.audit(req, '创建状态快照：' + String(b.name || ''));
  res.json({ ok: true, id: id });
});

/* ---------- 时序 / 处置时序图 ---------- */
router.get('/timeline', auth.requireAuth, function (req, res) {
  let rows = db.prepare('SELECT * FROM timeline ORDER BY seq').all();
  if (req.user.role.external) {
    rows = rows.filter(function (t) { return /上报|报送|接收|督办|报案|审批/.test(t.action || ''); });
  }
  res.json(rows.map(function (t) {
    return { t: t.t, actor: t.actor, role: t.role_id, action: t.action, detail: t.detail, level: t.level };
  }));
});

router.get('/seqflow', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM seq_flow ORDER BY seq').all().map(function (s) {
    return { name: s.name, time: s.time, role: s.role, state: s.state };
  }));
});

router.post('/seqflow', auth.requireAuth, function (req, res) {
  const items = Array.isArray(req.body && req.body.items) ? req.body.items : null;
  if (!items) return res.status(400).json({ error: '数据格式错误' });
  const pid = activeProjectId();
  db.prepare('DELETE FROM seq_flow WHERE project_id=?').run(pid);
  const ins = db.prepare('INSERT INTO seq_flow (project_id,name,time,role,state,seq) VALUES (?,?,?,?,?,?)');
  items.forEach(function (n, i) { ins.run(pid, String(n.name || ''), String(n.time || ''), String(n.role || ''), String(n.state || 'pending'), i); });
  res.json({ ok: true });
});

/* ---------- 评估（自动考评结果） ---------- */
router.get('/evaluation', auth.requireAuth, function (req, res) {
  const pid = activeProjectId();
  res.json({
    items: db.prepare('SELECT * FROM score_items ORDER BY seq').all().map(function (k) {
      return { id: k.id, name: k.name, weight: k.weight, rule: k.rule };
    }),
    scores: db.prepare('SELECT * FROM scores WHERE project_id=?').all(pid).map(function (s) {
      return { itemId: s.item_id, score: s.score, max: s.max, note: s.note, auto: !!s.auto };
    })
  });
});

router.post('/evaluation/scores', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'commander' && rid !== 'itlead' && rid !== 'platformadmin') {
    return res.status(403).json({ error: '仅指挥部/信息科负责人可执行自动考评' });
  }
  const items = Array.isArray(req.body && req.body.scores) ? req.body.scores : null;
  if (!items) return res.status(400).json({ error: '评分数据格式错误' });
  const pid = activeProjectId() || 'P2026-012';
  const up = db.prepare('INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,1,?) ' +
    'ON CONFLICT(item_id) DO UPDATE SET project_id=excluded.project_id,score=excluded.score,max=excluded.max,note=excluded.note,auto=1,updated_at=excluded.updated_at');
  const ts = now();
  items.forEach(function (s) {
    up.run(String(s.itemId), pid, Number(s.score) || 0, Number(s.max) || 10, String(s.note || ''), ts);
  });
  auth.audit(req, '执行系统自动考评（' + items.length + ' 项指标）');
  res.json({ ok: true, time: ts });
});

/* ---------- 台账 ---------- */
router.get('/ledger', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM ledger').all().map(function (l) {
    return { id: l.id, name: l.name, date: l.date, type: l.type, event: l.event, level: l.level, result: l.result, score: l.score, org: l.org };
  }));
});

/* ---------- 舆情 ---------- */
router.get('/opinion', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM opinion ORDER BY id').all().map(function (o) {
    return { id: o.id, time: o.time, channel: o.channel, title: o.title, heat: o.heat, risk: o.risk, status: o.status, source: o.source };
  }));
});

router.post('/opinion', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (['cac', 'commander', 'itlead', 'platformadmin'].indexOf(rid) < 0) {
    return res.status(403).json({ error: '仅网信办/指挥部/信息科负责人可发布舆情记录' });
  }
  const b = req.body || {};
  if (!b.title) return res.status(400).json({ error: '舆情标题不能为空' });
  const maxId = db.prepare("SELECT MAX(CAST(SUBSTR(id,2) AS INTEGER)) AS n FROM opinion").get().n || 0;
  const id = 'O' + (maxId + 1);
  db.prepare('INSERT INTO opinion (id,project_id,time,channel,title,heat,risk,status,source) VALUES (?,?,?,?,?,?,?,?,?)')
    .run(id, activeProjectId(), hhmm(), String(b.channel || '人工录入'), String(b.title),
      String(b.heat || '-'), String(b.risk || '一般'), '待研判', '人工发布（演练模拟）');
  auth.audit(req, '新增舆情记录：' + String(b.title).slice(0, 50));
  res.json({ ok: true, id: id });
});

router.patch('/opinion/:id', auth.requireAuth, function (req, res) {
  const o = db.prepare('SELECT * FROM opinion WHERE id=?').get(req.params.id);
  if (!o) return res.status(404).json({ error: '舆情记录不存在' });
  const b = req.body || {};
  ['status', 'risk', 'heat'].forEach(function (k) {
    if (b[k] !== undefined) db.prepare('UPDATE opinion SET ' + k + '=? WHERE id=?').run(String(b[k]), o.id);
  });
  auth.audit(req, '更新舆情记录 ' + o.id + ' 状态为「' + String(b.status || o.status) + '」');
  res.json({ ok: true });
});

/* ---------- 知识库 ---------- */
/* 可维护知识库的角色：平台管理员、院应急指挥部、信息科负责人 */
const KB_EDIT_ROLES = ['platformadmin', 'commander', 'itlead'];
function canEditKnowledge(roleId) { return KB_EDIT_ROLES.indexOf(roleId) >= 0; }
/* 先校验权限再接收文件，避免无权用户触发落盘 */
function requireKbEditor(req, res, next) {
  if (!canEditKnowledge(req.user.role.id)) {
    return res.status(403).json({ error: '仅平台管理员/院应急指挥部/信息科负责人可维护知识库文档' });
  }
  next();
}

router.get('/knowledge', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM knowledge ORDER BY id').all().map(function (k) {
    return {
      id: k.id, name: k.name, cat: k.cat, updated: k.updated, size: k.size, pages: k.pages,
      content: k.content, filePath: k.file_path || null,
      url: k.file_path ? '/uploads/' + k.file_path : null
    };
  }));
});

/* 上传知识库文档（multipart：name / cat / file） */
router.post('/knowledge', auth.requireAuth, requireKbEditor, kbUpload.single('file'), handleUploadError, function (req, res) {
  const f = req.file;
  if (!f) return res.status(400).json({ error: '请选择要上传的文档' });
  let orig = f.originalname || '文档';
  try { orig = Buffer.from(orig, 'latin1').toString('utf8'); } catch (e) {}
  const cleanName = orig.replace(/^\d+-[a-z0-9]+-/, '');
  const name = String((req.body && req.body.name) || '').trim() || cleanName.replace(/\.[^.]+$/, '');
  if (name.length > 100) return res.status(400).json({ error: '文档名称不能超过 100 个字符' });
  const cat = String((req.body && req.body.cat) || '').trim() || '上传文档';

  let maxSeq = 0;
  db.prepare("SELECT id FROM knowledge WHERE id LIKE 'KB%'").all().forEach(function (r) {
    const n = Number(String(r.id).replace(/^KB/, ''));
    if (!isNaN(n)) maxSeq = Math.max(maxSeq, n);
  });
  const id = 'KB' + (maxSeq + 1);
  const relPath = (req._uploadRel || 'knowledge') + '/' + f.filename;
  db.prepare('INSERT INTO knowledge (id,name,cat,updated,size,pages,content,file_path) VALUES (?,?,?,?,?,?,?,?)')
    .run(id, name, cat, now().slice(0, 10), fmtSize(f.size), 0, '', relPath);
  auth.audit(req, '上传知识库文档 ' + id + '《' + name + '》');
  res.json({
    ok: true, id: id,
    doc: {
      id: id, name: name, cat: cat, updated: now().slice(0, 10), size: fmtSize(f.size), pages: 0,
      content: '', filePath: relPath, url: '/uploads/' + relPath
    }
  });
});

/* 删除知识库文档（同时清理已上传的原文件） */
router.delete('/knowledge/:id', auth.requireAuth, requireKbEditor, function (req, res) {
  const k = db.prepare('SELECT * FROM knowledge WHERE id=?').get(req.params.id);
  if (!k) return res.status(404).json({ error: '文档不存在' });
  if (k.file_path) {
    const abs = path.join(__dirname, '..', '..', 'uploads', k.file_path);
    try { fs.unlinkSync(abs); } catch (e) {}
  }
  db.prepare('DELETE FROM knowledge WHERE id=?').run(k.id);
  auth.audit(req, '删除知识库文档 ' + k.id + '《' + k.name + '》');
  res.json({ ok: true, id: k.id });
});

/* ---------- 审计日志（只读，仅平台管理员/指挥部） ---------- */
router.get('/audit', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander') {
    return res.status(403).json({ error: '仅平台管理员/指挥部可查看审计日志' });
  }
  const limit = Math.min(Number(req.query.limit) || 300, 2000);
  res.json(db.prepare('SELECT * FROM audit_logs ORDER BY id DESC LIMIT ?').all(limit).map(function (a) {
    return { time: a.time, user: a.user, role: a.role_id, action: a.action, ip: a.ip };
  }));
});

/* ---------- 业务影响 ---------- */
router.get('/impact', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM impact ORDER BY seq').all().map(function (i) {
    return { sys: i.sys, state: i.state, pct: i.pct };
  }));
});

router.patch('/impact/:sys', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (['commander', 'itlead', 'sysadmin', 'platformadmin'].indexOf(rid) < 0) {
    return res.status(403).json({ error: '当前角色无权更新业务影响状态' });
  }
  const b = req.body || {};
  const r = db.prepare('UPDATE impact SET state=?, pct=? WHERE sys=?')
    .run(String(b.state || '正常'), Number(b.pct) || 100, req.params.sys);
  if (!r.changes) return res.status(404).json({ error: '业务系统不存在' });
  auth.audit(req, '更新业务影响：' + req.params.sys + ' → ' + String(b.state || '正常'));
  res.json({ ok: true });
});

/* ---------- 用户 / 角色（只读列表，供页面展示接收人、审批人） ---------- */
router.get('/users', auth.requireAuth, function (req, res) {
  res.json(db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,r.name AS role_name ' +
    'FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id'
  ).all().map(function (u) {
    return { id: u.id, username: u.username, name: u.name, role: u.role_id, roleName: u.role_name, org: u.org, status: u.status };
  }));
});

router.get('/roles', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return { id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color, desc: r.descr, nav: parseJson(r.nav, []), external: !!r.external };
  }));
});

/* ---------- 前端操作留痕（仅追加） ---------- */
router.post('/audit', auth.requireAuth, function (req, res) {
  const action = String((req.body && req.body.action) || '').slice(0, 300);
  if (!action) return res.status(400).json({ error: '操作内容不能为空' });
  auth.audit(req, action);
  res.json({ ok: true });
});

/* ---------- 字典 / 场景模拟数据 ---------- */
router.get('/dict', auth.requireAuth, function (req, res) {
  const out = {};
  db.prepare('SELECT * FROM dict').all().forEach(function (d) { out[d.key] = parseJson(d.value, []); });
  res.json(out);
});

router.get('/sim-data', auth.requireAuth, function (req, res) {
  const out = {};
  db.prepare('SELECT * FROM sim_data').all().forEach(function (s) { out[s.type] = parseJson(s.data, {}); });
  res.json(out);
});

/* ---------- 演练监视大屏（只读聚合数据） ----------
 * 大屏用于投影演示：免登录公开只读，仅返回演练模拟态势统计，不含账号与审计明细
 */
router.get('/monitor', function (req, res) {
  const projects = db.prepare('SELECT * FROM projects ORDER BY date DESC').all().map(function (p) {
    return { id: p.id, name: p.name, date: p.date, status: p.status, mode: p.mode || '基础版', level: p.level, scenarioId: p.scenario_id };
  });
  const pid = String(req.query.projectId || '') || activeProjectId() || (projects[0] && projects[0].id);
  const p = projects.filter(function (x) { return x.id === pid; })[0] || projects[0];
  if (!p) {
    return res.json({ serverTime: nowSec(), project: null, projects: [], stats: { total: 0, done: 0, doing: 0, pending: 0, overdue: 0 }, tasks: [], timeline: [], seqFlow: [], impact: [], messages: [], roles: [] });
  }
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=? ORDER BY seq').all(p.id);
  const timeline = db.prepare('SELECT * FROM timeline WHERE project_id=? ORDER BY seq').all(p.id).slice(-200);
  const seqFlow = db.prepare('SELECT * FROM seq_flow WHERE project_id=? ORDER BY seq').all(p.id);
  const roles = db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return { id: r.id, name: r.name, short: r.short, color: r.color, external: !!r.external };
  });
  const statOf = function (status) { return tasks.filter(function (t) { return t.status === status; }).length; };
  res.json({
    serverTime: nowSec(),
    project: p,
    projects: projects,
    stats: {
      total: tasks.length,
      done: statOf('已完成'),
      doing: statOf('处理中'),
      pending: statOf('待处理'),
      overdue: statOf('超时')
    },
    tasks: tasks.map(function (t) {
      return {
        id: t.id, role: t.role_id, title: t.title, status: t.status, level: t.level,
        deadline: t.deadline, acceptedAt: t.accepted_at || null, completedAt: t.completed_at || null
      };
    }),
    timeline: timeline.map(function (t) {
      return { t: t.t, actor: t.actor, role: t.role_id, action: t.action, detail: t.detail, level: t.level };
    }),
    seqFlow: seqFlow.map(function (s) {
      return { name: s.name, time: s.time, role: s.role, state: s.state };
    }),
    impact: db.prepare('SELECT * FROM impact ORDER BY seq').all().map(function (i) {
      return { sys: i.sys, state: i.state, pct: i.pct };
    }),
    messages: db.prepare('SELECT * FROM messages ORDER BY seq').all().slice(-24).map(function (m) {
      return { time: m.time, level: m.level, source: m.source, text: m.text };
    }),
    roles: roles
  });
});

module.exports = router;
