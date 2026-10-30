/* 内容类接口：消息、通知、会商、场景、项目、快照、时序、评估、台账、舆情、知识库、审计、影响、字典 */
'use strict';
const express = require('express');
const path = require('node:path');
const fs = require('node:fs');
const { db, tx, parseJson } = require('../db');
const auth = require('../auth');
const { makeUploader, handleUploadError } = require('../upload');
const { now, hhmm, fmtSize, nowSec } = require('../util');
const { ensureBasicTasks, parseDeps, targetToRoleId } = require('../drill_plan');
const notify = require('../notify');
const clock = require('../drill_clock');

const router = express.Router();
const kbUpload = makeUploader('knowledge');
const chatUpload = makeUploader('chat');

const MODES = ['基础版', '实战版'];

/* 下一节点接收人支持多选：数组或分隔字符串统一为「、」拼接的名称串 */
function receiverStr(v) {
  const arr = Array.isArray(v) ? v : String(v || '').split(/[、,，;；]/);
  const out = [];
  arr.forEach(function (x) {
    const s = String(x || '').trim().slice(0, 40);
    if (s && out.indexOf(s) < 0) out.push(s);
  });
  return out.join('、');
}

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
  db.prepare('INSERT INTO messages (id,time,level,source,text,targets,seq,project_id) VALUES (?,?,?,?,?,?,?,?)')
    .run(id, hhmm(), String(b.level || 'P2'), String(b.source || (req.user.name + '（人工触发）')),
      String(b.text || ''), JSON.stringify(b.targets || ['*']), seq, activeProjectId());
  auth.audit(req, '发布消息：' + String(b.text || '').slice(0, 60));
  res.json({ ok: true, id: id });
});

/* ---------- 通知 ---------- */
router.get('/notifs', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT * FROM notifs ORDER BY seq').all().filter(function (n) {
    /* 外部单位仅可见公开通知，演练内部消息按权限隔离 */
    return !req.user.role.external || (n.target || 'all') !== 'internal';
  });
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

/* ---------- 演练导调中心：频道权限真源为 dict 表 chat_channels ---------- */
/* 流程字典页维护 chat_channels，每个频道 { label, roles:[...] }；不存在或为空视为无频道可用 */
function loadChatAccess() {
  try {
    const row = db.prepare("SELECT value FROM dict WHERE key='chat_channels'").get();
    if (!row || !row.value) return {};
    const v = JSON.parse(row.value);
    if (v && typeof v === 'object' && Object.keys(v).length) {
      Object.keys(v).forEach(function (k) {
        if (!v[k].label) v[k].label = k;
        if (!Array.isArray(v[k].roles)) v[k].roles = [];
      });
      return v;
    }
  } catch (e) { /* malformed JSON → 返回空对象 */ }
  return {};
}

function canAccessChat(roleId, key) {
  const ch = loadChatAccess()[key];
  return !!ch && Array.isArray(ch.roles) && ch.roles.indexOf(roleId) >= 0;
}

function channelOfKey(req, res, key) {
  const access = loadChatAccess();
  const ch = access[key];
  if (!ch) {
    res.status(400).json({ error: '会商频道不存在' });
    return null;
  }
  if (!canAccessChat(req.user.role.id, key)) {
    res.status(403).json({ error: '当前角色不具备「' + ch.label + '」频道权限' });
    return null;
  }
  return { key: key, meta: ch };
}

function channelOf(req, res) {
  return channelOfKey(req, res, String(req.params.group || ''));
}

function fullTime(t) {
  const s = String(t || '');
  if (!s) return '';
  if (/^\d{4}-\d{2}-\d{2}/.test(s)) return s.slice(0, 16);
  return now().slice(0, 10) + ' ' + s.slice(0, 5);
}

function chatDto(c) {
  return {
    id: c.id, sender: c.sender, role: c.role_id, userId: c.user_id, time: fullTime(c.time), text: c.text || '',
    att: c.att || null, url: c.file_path ? ('/uploads/' + c.file_path) : null
  };
}

router.get('/chat/channels', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  const access = loadChatAccess();
  const out = Object.keys(access).filter(function (k) {
    return canAccessChat(rid, k);
  }).map(function (k) {
    const last = db.prepare('SELECT text,time,sender FROM chat_messages WHERE grp=? ORDER BY id DESC LIMIT 1').get(k) || null;
    if (last) last.time = fullTime(last.time);
    return {
      key: k, label: access[k].label,
      count: db.prepare('SELECT COUNT(*) AS n FROM chat_messages WHERE grp=?').get(k).n,
      last: last
    };
  });
  res.json(out);
});

router.get('/chat/:group', auth.requireAuth, function (req, res) {
  const ch = channelOf(req, res);
  if (!ch) return;
  const rows = db.prepare('SELECT * FROM chat_messages WHERE grp=? ORDER BY id').all(ch.key);
  res.json(rows.map(chatDto));
});

/* 真实附件上传：先上传到服务器，再随消息发送 */
router.post('/chat/:group/upload', auth.requireAuth, function (req, res, next) {
  const ch = channelOf(req, res);
  if (!ch) return;
  chatUpload.single('file')(req, res, function (err) {
    if (err) return handleUploadError(err, req, res, next);
    if (!req.file) return res.status(400).json({ error: '未接收到文件' });
    const rel = (req._uploadRel || 'chat') + '/' + req.file.filename;
    let orig = req.file.originalname || 'file';
    try { orig = Buffer.from(orig, 'latin1').toString('utf8'); } catch (e) {}
    const display = String(orig).replace(/[\\/:*?"<>|]/g, '_').slice(0, 120);
    auth.audit(req, '上传会商附件：' + display);
    res.json({ ok: true, name: display, size: fmtSize(req.file.size), url: '/uploads/' + rel, path: rel });
  });
});

router.post('/chat/:group', auth.requireAuth, function (req, res) {
  const ch = channelOf(req, res);
  if (!ch) return;
  const b = req.body || {};
  const text = String(b.text || '').trim().slice(0, 2000);
  const att = b.att ? String(b.att).slice(0, 200) : null;
  const filePath = b.filePath ? String(b.filePath).slice(0, 300) : null;
  if (!text && !att) return res.status(400).json({ error: '消息内容不能为空' });
  const r = db.prepare('INSERT INTO chat_messages (grp,sender,role_id,user_id,time,text,att,file_path,project_id) VALUES (?,?,?,?,?,?,?,?,?)')
    .run(ch.key, req.user.name, req.user.role.id, req.user.id, now().slice(0, 16), text, att, filePath, activeProjectId());
  /* 会商附件同步归档至取证模块，便于复盘追溯 */
  if (att && filePath) {
    const id = 'E' + Date.now() + Math.floor(Math.random() * 100);
    const activeProj = (db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value || null;
    db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
      .run(id, '聊天记录', att, String(b.attSize || '-'), hhmm(), req.user.role.id, req.user.name, req.user.id, 0, null, activeProj, null, filePath, now());
  }
  auth.audit(req, '导调中心发言（' + ch.meta.label + '）：' + (text || '[附件] ' + att).slice(0, 80));
  res.json({ ok: true, id: Number(r.lastInsertRowid) });
});

/* ---------- 演练节点操作情况（按频道权限过滤：院内看全部，院外仅上报沟通阶段） ---------- */
/* 院外单位名称出现在责任角色或节点描述中，才归入院外上报沟通阶段；院内上报指挥部不计入 */
const EXT_ORG_RE = /卫生健康局|卫健局|卫健委|网信办|网安大队|公安网安/;
const EXT_EVENT_RE = /报案|舆情|对外口径/;

function isExternalNode(n) {
  if (EXT_ORG_RE.test(String(n.target || ''))) return true;
  const text = String(n.title || '') + ' ' + String(n.descr || '');
  return EXT_ORG_RE.test(text) || EXT_EVENT_RE.test(text);
}

function opStamp(v, date) {
  const s = String(v || '').trim();
  if (!s) return '';
  if (/^\d{4}-\d{1,2}-\d{1,2}/.test(s)) return s.slice(0, 16);
  return date + ' ' + s.slice(0, 5);
}

function nodeProgress(project, channel) {
  const nodes = db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(project.scenario_id)
    .filter(function (n) { return n.type !== 'end'; });
  const mode = project.mode || '基础版';
  const date = /^\d{4}-\d{2}-\d{2}/.test(project.date || '') ? String(project.date).slice(0, 10) : now().slice(0, 10);
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(project.id);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=?').all(project.id);
  /* 时序中按任务号反查最近一次操作人，用于基础版节点操作情况 */
  const tlByTask = {};
  db.prepare('SELECT * FROM timeline WHERE project_id=? ORDER BY seq,id').all(project.id).forEach(function (r) {
    const m = /任务\s+([A-Za-z0-9-]+)/.exec(r.detail || '');
    if (m) tlByTask[m[1]] = r;
  });
  const stepByNode = {}, doneMap = {}, doingMap = {};
  steps.forEach(function (s) {
    if (!s.matched_node) return;
    stepByNode[s.matched_node] = s;
    doneMap[s.matched_node] = true;
  });
  /* 打开处置表单即记为处置中 */
  db.prepare('SELECT node_id,user,started_at FROM drill_node_state WHERE project_id=?').all(project.id).forEach(function (r) {
    doingMap[r.node_id] = r;
  });
  /* 实战版按前置依赖放出环节：前置全部完成（或本环节已产生处置/处置中记录）才展示，避免提前泄露后续剧本 */
  const visible = {};
  if (mode === '实战版') {
    const nodeById = {};
    nodes.forEach(function (n) { nodeById[n.id] = n; });
    nodes.forEach(function (n) {
      const recorded = !!(doneMap[n.id] || doingMap[n.id]);
      if (n.flow_type === 'distractor') {
        visible[n.id] = recorded;
        return;
      }
      const deps = parseJson(n.deps, []).filter(function (id) { return !!nodeById[id]; });
      visible[n.id] = recorded || deps.every(function (id) { return !!doneMap[id]; });
    });
  }
  const out = [];
  nodes.forEach(function (n, i) {
    const scope = isExternalNode(n) ? 'external' : 'inner';
    if (channel === 'external' && scope !== 'external') return;
    if (mode === '实战版' && scope !== 'external' && !visible[n.id]) return;

    const task = tasks.filter(function (t) { return t.node_id === n.id; })[0] || null;
    const step = stepByNode[n.id] || null;
    const doing = doingMap[n.id] || null;
    let status = 'pending', actor = '', opTime = '', detail = '';
    if (step) {
      status = step.correct ? 'done' : 'wrong';
      actor = step.user || '';
      opTime = step.created_at
        ? String(step.created_at).slice(0, 16)
        : (step.found_at || (date + ' ' + String(step.step_time || '').slice(0, 5)));
      detail = step.descr || step.measure || '';
    } else if (task && task.status === '已完成') {
      status = 'done';
      const tl = tlByTask[task.id];
      actor = tl ? (tl.actor || '') : '';
      opTime = tl ? opStamp(tl.t, date) : opStamp(task.completed_at, date);
      detail = (tl && tl.detail) || task.descr || '';
    } else if ((task && task.status === '处理中') || doing) {
      status = 'doing';
      actor = (doing && doing.user) || '';
      opTime = doing && doing.started_at
        ? String(doing.started_at).slice(0, 16)
        : opStamp(task && task.accepted_at, date);
      detail = task ? (task.descr || '') : (doing ? '已打开处置表单，正在处置' : '');
    } else {
      status = task && task.status === '超时' ? 'overdue' : 'pending';
      const tl = task ? tlByTask[task.id] : null;
      if (tl) { actor = tl.actor || ''; opTime = opStamp(tl.t, date); detail = tl.detail || ''; }
    }
    /* 导调中心仅展示处置中与已完成的节点，待处置/超时未处置不显示 */
    if (status !== 'done' && status !== 'doing' && status !== 'wrong') return;
    out.push({
      id: n.id, seq: i + 1, title: n.title, target: n.target, action: n.action || '',
      planTime: '',
      duration: Number(n.timeout) || null,
      scope: scope, status: status, actor: actor, opTime: opTime, detail: detail,
      correct: step ? !!step.correct : null, ontime: step ? !!step.ontime : null
    });
  });
  return out;
}

/* ---------- 大屏专用：动态计算节点进度（仅返回已完成/处置中） ----------
 * 基础版按 tasks.status 判定，实战版按 drill_steps + drill_node_state 判定
 * 实战版保留 deps 依赖链可见性控制，避免提前泄露后续剧本节点
 */
/* ---------- 大屏专用：动态计算节点状态 ----------
 * 基础版按 tasks.status 判定，实战版按 drill_steps + drill_node_state 判定
 * 用 script_nodes.timeout 阈值判定超时（分钟），基准为 project.started_at
 * 返回 {all:三态完整节点[], filtered:仅done/doing[], overdue:超时节点数}
 */
function computeMonitorNodes(project) {
  if (!project || !project.scenario_id) return { all: [], filtered: [], overdue: 0 };
  const nodes = db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? AND type!=\'end\' AND COALESCE(flow_type,\'correct\')!=\'distractor\' ORDER BY seq')
    .all(project.scenario_id);
  if (!nodes.length) return { all: [], filtered: [], overdue: 0 };
  const mode = project.mode || '基础版';

  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(project.id);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=?').all(project.id);
  const nodeStates = db.prepare('SELECT node_id,user,started_at FROM drill_node_state WHERE project_id=?').all(project.id);

  const taskByNode = {};
  tasks.forEach(function (t) { if (t.node_id) taskByNode[t.node_id] = t; });
  const stepInfo = {};
  steps.forEach(function (s) { if (s.matched_node) stepInfo[s.matched_node] = s; });
  const doingNode = {};
  nodeStates.forEach(function (r) { doingNode[r.node_id] = r; });

  // 演练开始时间（秒），用于超时判定
  let startedSec = 0;
  if (project.started_at) {
    startedSec = Math.floor(new Date(project.started_at.replace(' ', 'T')).getTime() / 1000);
  }

  // 实战版 deps 依赖链可见性
  let visible = {};
  if (mode === '实战版') {
    const nodeById = {};
    nodes.forEach(function (n) { nodeById[n.id] = n; });
    nodes.forEach(function (n) {
      const recorded = !!(stepInfo[n.id] || doingNode[n.id]);
      const deps = parseDeps(n.deps).filter(function (id) { return !!nodeById[id]; });
      visible[n.id] = recorded || deps.every(function (id) { return !!stepInfo[id]; });
    });
  }

  const allOut = [];
  const filteredOut = [];
  let overdueCnt = 0;

  nodes.forEach(function (n) {
    if (mode === '实战版' && !visible[n.id]) return;

    const task = taskByNode[n.id];
    const doingRec = doingNode[n.id];
    let state = 'pending', time = '', role = n.target || '', finishSec = 0;

    if (stepInfo[n.id]) {
      const step = stepInfo[n.id];
      state = 'done';
      time = step.step_time || step.created_at || '';
      if (time && /^\d{4}-\d{1,2}-\d{1,2}/.test(time)) {
        finishSec = Math.floor(new Date(String(time).replace(' ', 'T')).getTime() / 1000);
        time = String(time).slice(5, 16);
      }
    } else if (task && task.status === '已完成') {
      state = 'done';
      time = task.completed_at || '';
      if (time && /^\d{4}-\d{1,2}-\d{1,2}/.test(time)) {
        finishSec = Math.floor(new Date(String(time).replace(' ', 'T')).getTime() / 1000);
        time = String(time).slice(5, 16);
      }
      if (!time) {
        const tl = db.prepare("SELECT t FROM timeline WHERE project_id=? AND detail LIKE ? ORDER BY seq DESC LIMIT 1")
          .all(project.id, '%' + task.id + '%')[0];
        if (tl) time = tl.t;
      }
    } else if (doingRec) {
      state = 'doing';
      time = doingRec.started_at || '';
      if (time && /^\d{4}-\d{1,2}-\d{1,2}/.test(time)) time = String(time).slice(5, 16);
    } else if (task && task.status === '处理中') {
      state = 'doing';
      time = task.accepted_at || '';
      if (time && /^\d{4}-\d{1,2}-\d{1,2}/.test(time)) time = String(time).slice(5, 16);
    }

    // 超时判定：直接读取演练自动评分系统的结果
    // 基础版: tasks.status='超时'
    // 实战版: drill_steps.correct=1 AND ontime=0
    let isOverdue = false;
    if (stepInfo[n.id]) {
      // 实战版：读自动评分系统
      const s = stepInfo[n.id];
      if (Number(s.correct) === 1 && Number(s.ontime) === 0) isOverdue = true;
    } else if (task && task.status === '超时') {
      // 基础版：读 tasks 表状态
      isOverdue = true;
    }
    if (isOverdue) overdueCnt++;

    const item = {
      id: n.id, seq: n.seq, name: n.title, role: role, time: time,
      state: state, isOverdue: isOverdue, timeout: Number(n.timeout) || 0,
      deps: parseDeps(n.deps),
      type: n.type || 'task',
      parallelDesc: n.parallel_desc || n.parallelDesc || ''
    };
    allOut.push(item);
    if (state !== 'pending') filteredOut.push(item);
  });
  return { all: allOut, filtered: filteredOut, overdue: overdueCnt };
}

router.get('/drill-progress', auth.requireAuth, function (req, res) {
  const ch = channelOfKey(req, res, String(req.query.channel || 'inner'));
  if (!ch) return;
  const pid = activeProjectId();
  const p = pid ? db.prepare('SELECT * FROM projects WHERE id=?').get(pid) : null;
  if (!p) return res.json({ project: null, mode: '', scenario: null, nodes: [], updatedAt: now() });
  const sc = db.prepare('SELECT id,name,type,alert FROM scenarios WHERE id=?').get(p.scenario_id) || null;
  res.json({
    project: { id: p.id, name: p.name, mode: p.mode || '基础版', status: p.status, date: p.date, scenarioId: p.scenario_id },
    mode: p.mode || '基础版',
    scenario: sc,
    nodes: nodeProgress(p, ch.key),
    updatedAt: now()
  });
});

/* ---------- 场景剧本 ---------- */
router.get('/scenarios', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM scenarios').all().map(function (s) {
    return { id: s.id, name: s.name, type: s.type, level: s.level, status: s.status, version: s.version, updated: s.updated, nodes: s.nodes, desc: s.descr, alert: s.alert || '' };
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
  const fields = { name: 'name', type: 'type', level: 'level', status: 'status', desc: 'descr', alert: 'alert', version: 'version', nodes: 'nodes' };
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
  db.prepare('INSERT INTO scenarios (id,name,type,level,status,version,updated,nodes,descr,alert) VALUES (?,?,?,?,?,?,?,?,?,?)')
    .run(id, String(b.name), String(b.type || ''), String(b.level || '一般'),
      '草稿', 'v1.0', now().slice(0, 10), 0, String(b.desc || ''), String(b.alert || ''));
  auth.audit(req, '新建剧本《' + b.name + '》');
  res.json({ ok: true, id: id });
});

/* ---------- 剧本节点保存（可视化编排） ---------- */
router.get('/script-nodes', auth.requireAuth, function (req, res) {
  res.json(db.prepare('SELECT * FROM script_nodes ORDER BY scenario_id,seq').all().map(function (n) {
    return { id: n.id, scenarioId: n.scenario_id, type: n.type, title: n.title, target: n.target, timeout: n.timeout, form: n.form || undefined, att: n.att || undefined, desc: n.descr, branches: parseJson(n.branches, []), parallelDesc: n.parallel_desc || undefined, guide: n.guide || '', keywords: n.keywords || '', action: n.action || '', required: n.required !== 0, wrong: parseJson(n.wrong, []), evidenceReq: parseJson(n.evidence_req, []), flowType: n.flow_type === 'distractor' ? 'distractor' : 'correct', score: Number(n.score || 0), receiver: n.receiver || '', deps: parseDeps(n.deps), posX: n.pos_x === null || n.pos_x === undefined ? null : Number(n.pos_x), posY: n.pos_y === null || n.pos_y === undefined ? null : Number(n.pos_y) };
  }));
});

function bumpVersion(v) {
  const m = /^v(\d+)\.(\d+)$/.exec(String(v || 'v1.0'));
  return m ? 'v' + m[1] + '.' + (Number(m[2]) + 1) : 'v1.1';
}
function resolveVersion(req, currentV) {
  /* 支持两种传参方式：version（直接指定新版本）或 versionBump（'none'/'major'/'minor'/'patch'） */
  const body = req.body || {};
  const incoming = typeof body.version === 'string' ? body.version.trim() : '';
  if (incoming && /^v?\d+\.\d+$/.test(incoming)) {
    return incoming.startsWith('v') ? incoming : ('v' + incoming);
  }
  const bump = (typeof body.versionBump === 'string' ? body.versionBump : 'minor').toLowerCase();
  const m = /^v(\d+)\.(\d+)$/.exec(String(currentV || 'v1.0'));
  if (!m) return 'v1.0';
  const major = Number(m[1]), minor = Number(m[2]);
  if (bump === 'none' || bump === 'keep') return 'v' + major + '.' + minor;
  if (bump === 'major') return 'v' + (major + 1) + '.0';
  /* minor / patch 以及默认 */
  return 'v' + major + '.' + (minor + 1);
}

/* 依赖图拓扑排序（稳定，按数组原顺序出队）：无环返回节点 ID 顺序，有环返回 null */
function topoSortNodes(list, depMap) {
  const ids = list.map(function (n) { return String(n.id); });
  const present = {};
  const indeg = {}, children = {};
  ids.forEach(function (id) { present[id] = true; indeg[id] = 0; children[id] = []; });
  ids.forEach(function (id) {
    depMap[id].forEach(function (d) {
      if (!present[d]) return;
      indeg[id]++;
      children[d].push(id);
    });
  });
  const queue = ids.filter(function (id) { return !indeg[id]; });
  const out = [];
  while (queue.length) {
    const id = queue.shift();
    out.push(id);
    children[id].forEach(function (c) {
      indeg[c]--;
      if (!indeg[c]) queue.push(c);
    });
  }
  return out.length === ids.length ? out : null;
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
  /* 只保存本场景节点：调用方可能提交全量节点，其他场景节点直接忽略 */
  const sceneNodes = nodes.filter(function (n) { return !n.scenarioId || String(n.scenarioId) === scenario; });

  /* 节点 ID 唯一性校验（依赖关系以节点 ID 为键） */
  let byId = {}, seen = {};
  for (let i = 0; i < sceneNodes.length; i++) {
    const id = String(sceneNodes[i].id || '');
    if (!id) return res.status(400).json({ error: '节点 ID 不能为空' });
    if (seen[id]) return res.status(400).json({ error: '节点 ID 重复：' + id });
    const owner = db.prepare('SELECT scenario_id FROM script_nodes WHERE id=?').get(id);
    if (owner && owner.scenario_id !== scenario) {
      return res.status(400).json({ error: '节点 ID 与其他剧本冲突：' + id });
    }
    seen[id] = true;
    byId[id] = sceneNodes[i];
  }

  /* ---------- 重编号：确保主流节点为 {SID}-{N}，混淆为 DS{SID}-{role}-{n} ----------
   * 空号优先复用：如 S01 已有 1-7,9-12,14-15，则新增节点先占 8、13，再从 16 起递增
   * 同时更新 deps 引用，生成 idMap[oldId]=newId 给后续流程使用
   */
  const correctNodes_pre = sceneNodes.filter(function (n) { return n.flowType !== 'distractor'; });
  const distractors_pre = sceneNodes.filter(function (n) { return n.flowType === 'distractor'; });
  const MAIN_ID_RE = new RegExp('^' + scenario.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '-(\\d+)$');

  /* Step 1: 收集已有规范主流编号，计算空号和 max */
  const existingMainNums = [];
  correctNodes_pre.forEach(function (n) {
    const m = MAIN_ID_RE.exec(String(n.id));
    if (m) existingMainNums.push(Number(m[1]));
  });
  existingMainNums.sort(function (a, b) { return a - b; });
  const mainMax = existingMainNums.length ? Math.max.apply(null, existingMainNums) : 0;
  const mainGaps = [];
  if (existingMainNums.length) {
    for (let i = 1; i <= mainMax; i++) {
      if (existingMainNums.indexOf(i) < 0) mainGaps.push(i);
    }
  }

  /* Step 2: 为主流节点分配编号（已规范的保留，非规范的优先填空号） */
  let gapIdx = 0, nextNew = mainMax + 1;
  const idMap = {};
  correctNodes_pre.forEach(function (n) {
    const oldId = String(n.id);
    const m = MAIN_ID_RE.exec(oldId);
    if (m) {
      idMap[oldId] = oldId; /* 已规范，保留 */
    } else {
      const num = (gapIdx < mainGaps.length) ? mainGaps[gapIdx++] : nextNew++;
      idMap[oldId] = scenario + '-' + num;
    }
  });

  /* Step 3: 混淆节点重编号（已有 DS{SID}-{role}-{n} 的 role-n 组合保留，否则重新分配） */
  const DIST_ID_RE = new RegExp('^DS' + scenario.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '-(.+?)-(\\d+)$');
  /* role → 已使用编号集合 */
  const distRoleUsed = {};
  distractors_pre.forEach(function (n) {
    const m = DIST_ID_RE.exec(String(n.id));
    if (m) {
      const role = m[1];
      distRoleUsed[role] = distRoleUsed[role] || [];
      distRoleUsed[role].push(Number(m[2]));
    }
  });
  distractors_pre.forEach(function (n) {
    const oldId = String(n.id);
    const m = DIST_ID_RE.exec(oldId);
    if (m) {
      idMap[oldId] = oldId; /* 已规范 */
    } else {
      /* 按 target 角色命名，同角色递增编号 */
      const roleName = String(n.target || 'other').replace(/[^a-zA-Z0-9\\u4e00-\\u9fa5]/g, '').slice(0, 6) || 'other';
      distRoleUsed[roleName] = distRoleUsed[roleName] || [];
      let num = 1;
      while (distRoleUsed[roleName].indexOf(num) >= 0) num++;
      distRoleUsed[roleName].push(num);
      idMap[oldId] = 'DS' + scenario + '-' + roleName + '-' + num;
    }
  });

  /* Step 4: 应用 idMap 到 sceneNodes 的 id 和 deps */
  sceneNodes.forEach(function (n) {
    const oldId = String(n.id);
    if (idMap[oldId] && idMap[oldId] !== oldId) {
      n.id = idMap[oldId];
    }
    if (Array.isArray(n.deps)) {
      n.deps = n.deps.map(function (d) {
        const s = String(d || '');
        return (idMap[s] && idMap[s] !== s) ? idMap[s] : s;
      });
    }
  });
  /* 更新 byId（后续依赖清洗要用） */
  const byId_new = {};
  sceneNodes.forEach(function (n) { byId_new[String(n.id)] = n; });
  byId = byId_new;
  /* ---------- 重编号结束 ---------- */

  /* 依赖清洗：仅允许引用同场景、非自身、非混淆流程、非结束节点的前置 */
  const correctNodes = sceneNodes.filter(function (n) { return n.flowType !== 'distractor'; });
  const depMap = {};
  correctNodes.forEach(function (n) {
    const deps = [];
    (Array.isArray(n.deps) ? n.deps : []).forEach(function (d) {
      const id = String(d || '');
      if (!id || id === String(n.id) || deps.indexOf(id) >= 0) return;
      const pre = byId[id];
      if (!pre || pre.flowType === 'distractor' || pre.type === 'end') return;
      deps.push(id);
    });
    depMap[String(n.id)] = deps;
  });
  const order = topoSortNodes(correctNodes, depMap);
  if (!order) return res.status(400).json({ error: '剧本存在循环依赖，请检查节点前置关系' });

  /* 接收人推导：后继正确流程的责任角色并集，无后继流转至院应急指挥部 */
  const succTargets = {};
  correctNodes.forEach(function (n) {
    const t = String(n.target || '').trim();
    depMap[String(n.id)].forEach(function (d) {
      succTargets[d] = succTargets[d] || [];
      if (t && succTargets[d].indexOf(t) < 0) succTargets[d].push(t);
    });
  });

  const distractors = sceneNodes.filter(function (n) { return n.flowType === 'distractor'; });
  const ordered = order.map(function (id) { return byId[id]; }).concat(distractors);
  const name = req.body && typeof req.body.name === 'string' ? String(req.body.name).trim().slice(0, 60) : '';

  const del = db.prepare('DELETE FROM script_nodes WHERE scenario_id=?');
  const ins = db.prepare('INSERT INTO script_nodes (id,scenario_id,type,title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,action,wrong,evidence_req,required,flow_type,score,receiver,deps,pos_x,pos_y,parallel_desc) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)');
  const plan = require('../evidence_plan');
  tx(function () {
    del.run(scenario);
    ordered.forEach(function (n, i) {
      const id = String(n.id);
      const flowType = n.flowType === 'distractor' ? 'distractor' : 'correct';
      const wrong = Array.isArray(n.wrong) ? n.wrong.map(function (x) { return String(x || '').trim(); }).filter(Boolean).slice(0, 5) : [];
      let receiver = flowType === 'distractor' ? '' : receiverStr(n.receiver);
      if (!receiver && flowType !== 'distractor') {
        const targets = succTargets[id] || [];
        receiver = targets.length ? targets.join('、') : '院应急指挥部';
      }
      /* 混淆流程不产生取证要求，不参与逐项取证提醒 */
      let evReq = flowType === 'distractor' ? [] : plan.normalizeReq(n.evidenceReq);
      if (!evReq.length && flowType !== 'distractor' && String(n.type || 'task') !== 'end') {
        evReq = plan.autoEvidenceReq({ title: n.title, target: n.target, action: n.action, guide: n.guide, desc: n.desc, form: n.form });
      }
      const rawScore = Math.round(Number(n.score || 0));
      const scoreVal = Math.max(0, Math.min(100, isNaN(rawScore) ? 0 : rawScore));
      const px = n.posX === null || n.posX === undefined || n.posX === '' || isNaN(Number(n.posX)) ? null : Math.round(Number(n.posX));
      const py = n.posY === null || n.posY === undefined || n.posY === '' || isNaN(Number(n.posY)) ? null : Math.round(Number(n.posY));
      ins.run(id, scenario, String(n.type || 'task'), String(n.title || ''), String(n.target || ''),
        '', n.timeout || null, String(n.form || ''), String(n.att || ''), String(n.desc || ''),
        JSON.stringify(n.branches || []), i, String(n.guide || ''), String(n.keywords || ''),
        String(n.action || ''), JSON.stringify(wrong), JSON.stringify(evReq), n.required === false ? 0 : 1,
        flowType, scoreVal, receiver, JSON.stringify(flowType === 'distractor' ? [] : depMap[id]), px, py,
        String(n.parallel_desc || ''));
    });
    /* 同步场景剧本库：名称、节点数、版本号、更新时间、故障通报 */
    const version = resolveVersion(req, sc.version);
    const updated = now().slice(0, 10);
    const alertText = (req.body && typeof req.body.alert === 'string') ? String(req.body.alert).slice(0, 1000) : (sc.alert || '');
    const finalName = name || sc.name;
    db.prepare('UPDATE scenarios SET name=?, nodes=?, version=?, updated=?, alert=? WHERE id=?')
      .run(finalName, correctNodes.length, version, updated, alertText, scenario);
    return { version: version, updated: updated, alert: alertText, name: finalName };
  });
  /* 事务内已完成版本号递增，重新读取最新版本用于响应 */
  const nowSc = db.prepare('SELECT name,version,updated,alert,nodes FROM scenarios WHERE id=?').get(scenario);
  auth.audit(req, '保存剧本编排《' + nowSc.name + '》' + correctNodes.length + ' 个流程节点，版本 ' + sc.version + '→' + nowSc.version);
  res.json({
    ok: true, count: correctNodes.length, scenarioId: scenario,
    version: nowSc.version, updated: nowSc.updated, alert: nowSc.alert || '', name: nowSc.name
  });
});

/* ---------- 项目 ---------- */

/* 计算单个项目的演练进度百分比（0-100）
 * 基础版：基于 tasks 表的完成比例
 * 实战版：基于 drill_steps 中正确流程节点完成数 / 剧本正确流程节点总数 */
function projectProgress(p) {
  if (!p) return 0;
  if (p.status === '已结束' || p.status === '已归档') return 100;
  if (p.status === '未开始') return 0;
  const mode = p.mode || '基础版';
  if (mode === '基础版') {
    const ts = db.prepare('SELECT COUNT(*) AS total, SUM(CASE WHEN status=? THEN 1 ELSE 0 END) AS done FROM tasks WHERE project_id=?')
      .get('已完成', p.id);
    if (!ts || !ts.total) return 0;
    return Math.round((ts.done || 0) / ts.total * 100);
  } else {
    /* 实战版：剧本正确流程节点总数（不含混淆） */
    if (!p.scenario_id) return 0;
    const correct = db.prepare("SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor'")
      .get(p.scenario_id).c;
    if (!correct) return 0;
    /* 已提交处置记录的正确流程节点数 */
    const doneRow = db.prepare(
      'SELECT COUNT(DISTINCT ds.matched_node) AS c FROM drill_steps ds '
      + "JOIN script_nodes n ON n.id=ds.matched_node AND n.scenario_id=? AND COALESCE(n.flow_type,'correct')!='distractor' AND n.type!='end' "
      + 'WHERE ds.project_id=?'
    ).get(p.scenario_id, p.id);
    const done = doneRow ? doneRow.c : 0;
    return Math.round(done / correct * 100);
  }
}

function projectDto(p) {
  return {
    id: p.id, name: p.name, type: p.type, date: p.date, scenarioId: p.scenario_id, level: p.level,
    status: p.status, target: p.target, mode: p.mode || '基础版',
    startedAt: p.started_at || null, pausedAt: p.paused_at || null,
    pausedSec: Number(p.paused_sec) || 0, endedAt: p.ended_at || null,
    elapsedSec: clock.elapsedSec(p),
    progress: projectProgress(p),
    units: parseJson(p.units, []),
    members: parseJson(p.members, []), observers: parseJson(p.observers, [])
  };
}

/* 大屏项目 DTO：透出真实计时字段，供大屏按秒本地推算 */
function monitorProjectDto(p) {
  return {
    id: p.id, name: p.name, date: p.date, status: p.status, mode: p.mode || '基础版',
    level: p.level, scenarioId: p.scenario_id,
    startedAt: p.started_at || null, pausedAt: p.paused_at || null,
    pausedSec: Number(p.paused_sec) || 0, endedAt: p.ended_at || null,
    elapsedSec: clock.elapsedSec(p)
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

  const brandRow = db.prepare("SELECT value FROM settings WHERE key='brand_unit_name'").get();
  const defaultUnit = (brandRow && brandRow.value) || '盘州市人民医院';
  const units = Array.isArray(b.units) && b.units.length ? b.units : [defaultUnit];
  const members = Array.isArray(b.members) ? b.members : [];
  const observers = Array.isArray(b.observers) ? b.observers : [];
  db.prepare('INSERT INTO projects (id,name,type,date,scenario_id,level,status,target,units,members,observers,mode) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(id, name, String(b.type || '实战协同演练'), date,
      scenarioId, String(b.level || (sc && sc.level) || '一般'), '未开始', String(b.target || ''),
      JSON.stringify(units), JSON.stringify(members), JSON.stringify(observers), mode);
  const created = db.prepare('SELECT * FROM projects WHERE id=?').get(id);
  const taskCount = ensureBasicTasks(db, created);
  auth.audit(req, '新建演练项目 ' + id + '《' + name + '》' + (taskCount ? '，自动下发 ' + taskCount + ' 项节点任务' : ''));

  /* 自动生成演练通知（站内消息）：通知参演角色演练项目已创建，可查看演练脚本 */
  notify.push({
    event: 'project_created',
    vars: {
      project: name,
      projectId: id,
      mode: mode,
      scenario: sc ? sc.name : (scenarioId || '未关联剧本'),
      level: created.level || '一般',
      date: date,
      target: String(b.target || '按剧本预案完成全流程处置').slice(0, 40)
    },
    level: created.level === '重大' ? 'P0' : 'P1'
  });

  /* 自动写入一条审计级 timeline 事件：演练项目创建 */
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(id, hhmm(), req.user.name, rid, '创建演练项目',
      '项目「' + name + '」（' + mode + '，关联剧本 ' + (sc ? sc.name : '无') + '，计划日期 ' + date + '）已创建',
      '', null);

  res.json({ ok: true, id: id, taskCount: taskCount, project: projectDto(created) });
});

/* 共享：构建项目演练脚本数据（供 JSON / Word / TXT 三种输出共用）
 * 无论基础版还是实战版，都过滤 end 终止节点 + distractor 混淆节点，
 * 两模式节点数完全一致，仅提示信息不同：
 *   基础版 → 完整 guide + desc + score（真实处置提示）
 *   实战版 → guide 替换为通用"本节点包括正确的和不正确的处置流程若干，请自行判定执行"，隐藏分值 */
function buildProjectScript(p) {
  if (!p || !p.scenario_id) return null;
  const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(p.scenario_id);
  const nodes = db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(p.scenario_id);
  const mode = p.mode || '基础版';
  const roleMap = {};
  db.prepare('SELECT id,name FROM roles').all().forEach(function (r) { roleMap[r.id] = r.name; });
  const targetToRole = function (target) {
    const t = String(target || '').trim();
    if (!t) return 'duty';
    let row = db.prepare("SELECT id FROM roles WHERE name=? OR short=?").get(t, t);
    if (row) return row.id;
    return targetToRoleId(db, t);
  };
  /* 两模式统一过滤：end + distractor 一律屏蔽，节点数一致 */
  const list = nodes.filter(function (n) {
    if (n.type === 'end') return false;
    if (n.flow_type === 'distractor') return false;
    return true;
  }).map(function (n) {
    const roleId = targetToRole(n.target);
    const roleName = roleMap[roleId] || n.target;
    const base = {
      id: n.id, type: n.type, title: n.title,
      role: roleName, roleId: roleId,
      action: n.action || '',
      deps: parseDeps(n.deps),
      timeout: Number(n.timeout) || 0
    };
    if (mode === '基础版') {
      base.desc = n.descr || '';
      base.guide = n.guide || '';
      base.keywords = (n.keywords || '').split(',').filter(Boolean);
      base.score = Number(n.score || 0) || 0;
      base.receiver = n.receiver || '';
      base.evidenceReq = parseJson(n.evidence_req, []);
      base.form = n.form || '';
      base.required = !!n.required;
    } else {
      /* 实战版：节点数量与基础版一致，仅提示信息改为通用话术 */
      base.guide = '本节点包括正确的和不正确的处置流程若干，请自行判定执行。';
      base.desc = '';
    }
    return base;
  });
  const members = parseJson(p.members, []).map(function (m) {
    const r = roleMap[m.role];
    return { roleId: m.role, name: r || m.role };
  });
  return {
    projectId: p.id, projectName: p.name,
    mode: mode, status: p.status, level: p.level || '一般',
    scenario: sc ? { id: sc.id, name: sc.name, type: sc.type, desc: sc.descr || '', alert: sc.alert || '' } : null,
    members: members,
    script: list,
    instructions: mode === '基础版'
      ? [
          '本脚本为基础版演练完整脚本，包含全部主流节点的详细处置步骤与提示信息。',
          '参演角色按任务中心提示，依次完成各自负责的节点处置。',
          '系统按剧本时限自动评分，超时完成扣本节点 10% 分值。'
        ]
      : [
          '本脚本为实战版演练主流节点流程（节点数量与基础版一致）。',
          '参演角色进入角色工作台后，需在多个处置选项中自主选择正确方案。',
          '选错混淆流程扣对应全部分值；超时完成扣本节点 10% 分值；正确流程未完成扣全部分值。'
        ]
  };
}

/* 获取演练脚本 JSON */
router.get('/projects/:id/script', auth.requireAuth, function (req, res) {
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if (!p.scenario_id) return res.status(400).json({ error: '该项目未关联剧本' });
  const data = buildProjectScript(p);
  res.json(Object.assign({ ok: true }, data));
});

/* 演练脚本导出：word(默认) | html */
router.get('/projects/:id/script/export', auth.requireAuth, function (req, res) {
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if (!p.scenario_id) return res.status(400).json({ error: '该项目未关联剧本' });
  const data = buildProjectScript(p);
  const fmt = String(req.query.fmt || 'word').toLowerCase();
  let body, ext, mime;
  if (fmt === 'html') { body = renderScriptWord(data, true); ext = '.html'; mime = 'text/html; charset=utf-8'; }
  else { body = renderScriptWord(data, false); ext = '.doc'; mime = 'application/msword'; }
  const fname = (data.projectName + '_演练脚本').replace(/[\\/:*?"<>|]/g, '');
  res.setHeader('Content-Type', mime);
  res.setHeader('Content-Disposition', 'attachment; filename="script_' + p.id + ext + '"; filename*=UTF-8\'\'' + encodeURIComponent(fname + ext));
  res.send(body);
  auth.audit(req, '导出演练脚本（' + fmt + '）：' + p.id + '《' + p.name + '》');
});

/* 手动生成演练通知 */
router.post('/projects/:id/notify', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (['platformadmin', 'commander', 'itlead'].indexOf(rid) < 0) {
    return res.status(403).json({ error: '仅平台管理员/指挥部/信息科负责人可生成演练通知' });
  }
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  const sc = p.scenario_id ? db.prepare('SELECT name FROM scenarios WHERE id=?').get(p.scenario_id) : null;
  notify.push({
    event: 'project_created',
    vars: {
      project: p.name, projectId: p.id, mode: p.mode || '基础版',
      scenario: sc ? sc.name : '未关联剧本', level: p.level || '一般',
      date: p.date, target: p.target || '按剧本预案完成全流程处置'
    },
    level: p.level === '重大' ? 'P0' : 'P1'
  });
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(p.id, hhmm(), req.user.name, rid, '生成演练通知', '演练通知已推送至所有参演角色的站内消息', '', null);
  auth.audit(req, '手动生成演练通知：' + p.id + '《' + p.name + '》');
  res.json({ ok: true, projectId: p.id, msg: '演练通知已推送至站内消息' });
});

/* 演练通知导出：word(默认) | html */
router.get('/projects/:id/notify/export', auth.requireAuth, function (req, res) {
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  const sc = p.scenario_id ? db.prepare('SELECT * FROM scenarios WHERE id=?').get(p.scenario_id) : null;
  const members = parseJson(p.members, []);
  const roleMap = {};
  db.prepare('SELECT id,name FROM roles').all().forEach(function (r) { roleMap[r.id] = r.name; });
  const names = members.map(function (m) { return roleMap[m.role] || m.role; });
  const fmt = String(req.query.fmt || 'word').toLowerCase();
  let body, ext, mime;
  if (fmt === 'html') { body = renderNotifyWord(p, sc, names, true); ext = '.html'; mime = 'text/html; charset=utf-8'; }
  else { body = renderNotifyWord(p, sc, names, false); ext = '.doc'; mime = 'application/msword'; }
  const fname = (p.name + '_演练通知').replace(/[\\/:*?"<>|]/g, '');
  res.setHeader('Content-Type', mime);
  res.setHeader('Content-Disposition', 'attachment; filename="notify_' + p.id + ext + '"; filename*=UTF-8\'\'' + encodeURIComponent(fname + ext));
  res.send(body);
  auth.audit(req, '导出演练通知（' + fmt + '）：' + p.id + '《' + p.name + '》');
});

/* ============ Word 导出渲染（Office HTML mso-header 伪装 .doc） ============ */
function wordHeader(title) {
  return '<html xmlns:o="urn:schemas-microsoft-com:office:office" '
    + 'xmlns:w="urn:schemas-microsoft-com:office:word" '
    + 'xmlns="http://www.w3.org/TR/REC-html40">'
    + '<head><meta charset="UTF-8">'
    + '<!--[if gte mso 9]><xml><w:WordDocument><w:View>Print</w:View><w:Zoom>100</w:Zoom></w:WordDocument></xml><![endif]-->'
    + '<title>' + escapeHtml(title) + '</title>'
    + '<style>' + wordCss() + '</style></head><body>';
}
function wordFooter() { return '</body></html>'; }
function wordCss() {
  return `
@page{mso-page-orientation:portrait;margin:2.54cm 2.8cm}
body{font-family:"宋体",SimSun,"Microsoft YaHei";font-size:12pt;color:#000;line-height:1.7}
h1{font-family:"黑体",SimHei;font-size:22pt;text-align:center;margin:12pt 0 18pt;color:#1a2b52}
h2{font-family:"黑体",SimHei;font-size:14pt;border-left:4pt solid #4f7cff;padding-left:6pt;margin:16pt 0 8pt;color:#1a2b52}
h3{font-family:"黑体",SimHei;font-size:12pt;margin:12pt 0 6pt;color:#2c3e50}
.km{width:100%;border-collapse:collapse;margin:4pt 0 10pt;font-size:11pt}
.km td{border:1pt solid #999;padding:5pt 8pt}
.km td.k{background:#f0f2f5;color:#666;width:110pt;font-weight:bold}
.kv{font-size:11.5pt;margin:3pt 0}
.kv b{color:#1a2b52}
.inst{margin:6pt 0 12pt;font-size:11.5pt;line-height:1.9}
.inst li{margin:2pt 0}
.chip{display:inline-block;background:#eef2ff;color:#2c47a8;padding:2pt 8pt;margin:2pt 3pt;border:1pt solid #c5ceff;border-radius:3pt;font-size:10.5pt}
.node{border:1pt solid #d0d4dc;border-left:3pt solid #4f7cff;margin:8pt 0;padding:8pt 10pt;background:#fff}
.node-head{font-weight:bold;font-size:12pt;color:#1a2b52;margin-bottom:3pt}
.node-idx{color:#4f7cff}
.node-meta{font-size:10.5pt;color:#777;margin-bottom:3pt}
.node-meta span{margin-right:10pt;background:#f4f6fa;padding:1pt 6pt;border:1pt solid #e1e5ee;border-radius:2pt}
.sn-guide{margin:5pt 0 3pt;padding:5pt 8pt;background:#ebf5fb;border-left:2pt solid #3498db;font-size:11pt;color:#2c3e50}
.sn-guide b{color:#2980b9}
.sn-desc{font-size:11pt;color:#444;margin:3pt 0}
.sn-score{font-size:10.5pt;color:#e67e22;font-weight:bold}
.hero{text-align:center;background:#4f7cff;color:#fff;padding:16pt 10pt;margin:10pt 0}
.hero .htitle{font-size:18pt;font-weight:bold;margin-bottom:3pt}
.hero .hsub{font-size:11pt;opacity:.85}
.level-badge{display:inline-block;padding:3pt 10pt;border:2pt solid;border-radius:10pt;font-weight:bold;font-size:11pt;margin-top:6pt}
.level-P0{background:#fce4e4;color:#c0392b;border-color:#e74c3c}
.level-P1{background:#fef5e7;color:#d4890d;border-color:#f39c12}
.attention{background:#fef9e7;border:1pt solid #f9e79f;padding:10pt 12pt;margin:10pt 0}
.attention h2{border-left-color:#f39c12;color:#7d6608}
.attention ul{margin:4pt 0;padding-left:18pt;font-size:11.5pt;color:#5d4e00}
.attention li{margin:2pt 0}
footer{text-align:center;margin-top:20pt;padding-top:8pt;border-top:1pt solid #999;color:#888;font-size:9.5pt}
`;
}
/* Word 版脚本 */
function renderScriptWord(d, isHtml) {
  const typeLabel = { alert: '告警节点', parallel: '并行节点', task: '处置节点', report: '上报节点', decision: '决策节点', end: '终止节点' };
  const nodeHtml = d.script.map(function (n, i) {
    const extras = [];
    if (d.mode === '基础版') {
      if (n.desc) extras.push('<div class="sn-desc">处置描述：' + escapeHtml(n.desc) + '</div>');
      if (n.guide) extras.push('<div class="sn-guide"><b>演练提示：</b>' + escapeHtml(n.guide) + '</div>');
      if (n.score) extras.push('<div class="sn-score">分值 ' + n.score + ' 分</div>');
      if (n.receiver) extras.push('<div class="kv" style="font-size:11pt">下一节点接收人：<b>' + escapeHtml(n.receiver) + '</b></div>');
      if (n.evidenceReq && n.evidenceReq.length) extras.push('<div class="kv" style="font-size:11pt">需取证材料：<b>' + n.evidenceReq.map(function (e) { return escapeHtml(e.name || e.type); }).join('、') + '</b></div>');
    } else {
      extras.push('<div class="sn-guide"><b>演练提示：</b>' + escapeHtml(n.guide) + '</div>');
    }
    return '<div class="node">'
      + '<div class="node-head"><span class="node-idx">(' + (i + 1) + ') </span>' + escapeHtml(n.title) + '</div>'
      + '<div class="node-meta">'
      +   (typeLabel[n.type] || n.type) + '　责任角色：' + escapeHtml(n.role) + '　处置时长：' + (n.timeout ? n.timeout + ' 分钟' : '未设置') + (n.action ? '　动作：' + escapeHtml(n.action) : '')
      + '</div>'
      + extras.join('')
      + '</div>';
  }).join('');
  const instHtml = '<div class="inst"><ol>' + d.instructions.map(function (s) { return '<li>' + escapeHtml(s) + '</li>'; }).join('') + '</ol></div>';
  const membersJoined = d.members.length ? d.members.map(function (m) { return escapeHtml(m.name); }).join('、') : '（未添加）';
  const overviewRows = [
    ['项目编号', d.projectId], ['项目名称', d.projectName],
    ['演练模式', d.mode], ['关联剧本', d.scenario ? d.scenario.name : '—'],
    ['剧本类型', d.scenario ? d.scenario.type : '—'],
    ['事件等级', d.level], ['当前状态', d.status]
  ];
  if (d.scenario && d.scenario.alert) overviewRows.push(['故障描述', d.scenario.alert]);
  const ovHtml = '<table class="km">' + overviewRows.map(function (r) { return '<tr><td class="k">' + r[0] + '</td><td>' + escapeHtml(r[1]) + '</td></tr>'; }).join('') + '</table>';

  const h = isHtml ? '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>' + escapeHtml(d.projectName + '演练脚本') + '</title><style>' + wordCss() + '</style></head><body>' : wordHeader(d.projectName + '演练脚本');
  const f = isHtml ? '</body></html>' : wordFooter();

  return h
    + '<h1>' + escapeHtml(d.projectName + '演练脚本') + '</h1>'
    + '<h2>一、演练概览</h2>' + ovHtml
    + '<h2>二、演练说明</h2>' + instHtml
    + '<h2>三、参演角色（' + d.members.length + ' 人）</h2><div style="margin:6pt 0 12pt;font-size:11.5pt">' + membersJoined + '</div>'
    + '<h2>四、演练流程（共 ' + d.script.length + ' 个主流节点）</h2>' + nodeHtml
    + '<footer>生成时间：' + now() + ' · ' + (isHtml ? '' : '演练系统自动生成') + '</footer>'
    + f;
}
/* Word 版通知 */
function renderNotifyWord(p, sc, memberNames, isHtml) {
  const modeText = (p.mode || '基础版') === '实战版'
    ? '实战版（自主选择处置方案，选错/超时扣分）'
    : '基础版（按剧本提示逐步操作，仅时限评分）';
  const membersJoined = memberNames.length ? memberNames.join('、') : '（未添加）';
  const title = p.name + '通知';

  const h = isHtml ? '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>' + escapeHtml(title) + '</title><style>' + wordCss() + '</style></head><body>' : wordHeader(title);
  const f = isHtml ? '</body></html>' : wordFooter();

  return h
    + '<h1>' + escapeHtml(title) + '</h1>'
    + '<h2>一、演练项目</h2>'
    + '<table class="km">'
    +   '<tr><td class="k">项目编号</td><td>' + escapeHtml(p.id) + '</td></tr>'
    +   '<tr><td class="k">项目名称</td><td>' + escapeHtml(p.name) + '</td></tr>'
    +   '<tr><td class="k">演练模式</td><td>' + escapeHtml(modeText) + '</td></tr>'
    +   '<tr><td class="k">演练类型</td><td>' + escapeHtml(p.type || '实战协同演练') + '</td></tr>'
    +   '<tr><td class="k">关联剧本</td><td>' + (sc ? escapeHtml(sc.name) : '—') + '</td></tr>'
    +   '<tr><td class="k">参演单位</td><td>' + escapeHtml(parseJson(p.units, []).join('、') || '盘州市人民医院') + '</td></tr>'
    +   '<tr><td class="k">计划日期</td><td>' + escapeHtml(p.date) + '</td></tr>'
    +   '<tr><td class="k">演练目标</td><td>' + escapeHtml(p.target || '按剧本预案完成全流程处置') + '</td></tr>'
    + '</table>'
    + '<h2>二、参演角色（' + memberNames.length + ' 人）</h2><div style="margin:6pt 0 12pt;font-size:11.5pt">' + membersJoined + '</div>'
    + '<div class="attention">'
    +   '<h2>三、注意事项</h2>'
    +   '<ul>'
    +     '<li>所有参演角色请登录系统后查看完整演练脚本，熟悉各自负责的处置节点。</li>'
    +     '<li>演练开始时间以系统推送的「演练开始」消息为准，未启动前请勿提前操作。</li>'
    +     '<li>本演练为' + escapeHtml(modeText) + '。</li>'
    +     '<li>演练全程不操作真实生产网络与业务系统，所有操作均为模拟记录。</li>'
    +     '<li>演练过程中产生的模拟数据不得作为真实工作依据。</li>'
    +     '<li>脚本/通知等资料以系统导出为准。</li>'
    +   '</ul>'
    + '</div>'
    + '<footer>生成时间：' + now() + ' · 请妥善保存此通知并提前通知参演人员</footer>'
    + f;
}

/* ============ TXT 导出渲染 ============ */
function renderScriptTxt(d) {
  const typeLabel = { alert: '告警节点', parallel: '并行节点', task: '处置节点', report: '上报节点', decision: '决策节点', end: '终止节点' };
  const lines = [];
  lines.push('====================================================================');
  lines.push('  演练脚本');
  lines.push('====================================================================');
  lines.push('');
  lines.push('一、演练概览');
  lines.push('  项目编号：' + d.projectId);
  lines.push('  项目名称：' + d.projectName);
  lines.push('  演练模式：' + d.mode);
  lines.push('  关联剧本：' + (d.scenario ? d.scenario.name : '—'));
  lines.push('  剧本类型：' + (d.scenario ? d.scenario.type : '—'));
  lines.push('  事件等级：' + d.level);
  lines.push('  当前状态：' + d.status);
  if (d.scenario && d.scenario.alert) lines.push('  故障描述：' + d.scenario.alert);
  lines.push('');
  lines.push('二、演练说明');
  d.instructions.forEach(function (s) { lines.push('  - ' + s); });
  lines.push('');
  lines.push('三、参演角色（' + d.members.length + ' 人）');
  if (d.members.length) d.members.forEach(function (m) { lines.push('  - ' + m.name); });
  else lines.push('  （未添加）');
  lines.push('');
  lines.push('四、演练流程（共 ' + d.script.length + ' 个主流节点）');
  lines.push('--------------------------------------------------------------------');
  d.script.forEach(function (n, i) {
    lines.push('');
    lines.push('(' + (i + 1) + ') ' + n.title);
    lines.push('    类型：' + (typeLabel[n.type] || n.type) + '    责任角色：' + n.role + '    处置时长：' + (n.timeout ? n.timeout + ' 分钟' : '未设置'));
    if (n.action) lines.push('    动作：' + n.action);
    if (d.mode === '基础版') {
      if (n.score) lines.push('    分值：' + n.score + ' 分');
      if (n.desc) lines.push('    处置描述：' + n.desc);
      if (n.guide) lines.push('    演练提示：' + n.guide);
      if (n.receiver) lines.push('    下一节点接收人：' + n.receiver);
      if (n.evidenceReq && n.evidenceReq.length) lines.push('    需取证材料：' + n.evidenceReq.map(function (e) { return e.name || e.type; }).join('、'));
    } else {
      lines.push('    演练提示：' + n.guide);
    }
  });
  lines.push('');
  lines.push('====================================================================');
  lines.push('  生成时间：' + now() + ' · 演练系统自动生成');
  lines.push('====================================================================');
  return lines.join('\r\n');
}
function renderNotifyTxt(p, sc, memberNames) {
  const level = (p.level || '一般') === '重大' ? 'P0 · 重大事件' : ((p.level || '一般') + '级');
  const modeText = (p.mode || '基础版') === '实战版'
    ? '实战版（自主选择处置方案，选错/超时扣分）'
    : '基础版（按剧本提示逐步操作，仅时限评分）';
  const lines = [];
  lines.push('====================================================================');
  lines.push('  演练通知');
  lines.push('====================================================================');
  lines.push('');
  lines.push('  演练项目：' + p.name);
  lines.push('  项目编号：' + p.id);
  lines.push('  事件等级：' + level);
  lines.push('  演练模式：' + modeText);
  lines.push('');
  lines.push('--------------------------------------------------------------------');
  lines.push('一、演练项目');
  lines.push('  演练类型：' + (p.type || '实战协同演练'));
  lines.push('  关联剧本：' + (sc ? sc.name : '—'));
  lines.push('  参演单位：' + (parseJson(p.units, []).join('、') || '盘州市人民医院'));
  lines.push('  计划日期：' + p.date);
  lines.push('  演练目标：' + (p.target || '按剧本预案完成全流程处置'));
  lines.push('');
  lines.push('二、参演角色（' + memberNames.length + ' 人）');
  if (memberNames.length) memberNames.forEach(function (m) { lines.push('  - ' + m); });
  else lines.push('  （未添加）');
  lines.push('');
  lines.push('三、注意事项');
  lines.push('  - 所有参演角色请登录系统后查看完整演练脚本，熟悉各自负责的处置节点。');
  lines.push('  - 演练开始时间以系统推送的"演练开始"消息为准，未启动前请勿提前操作。');
  lines.push('  - 本演练为' + modeText + '。');
  lines.push('  - 演练全程不操作真实生产网络与业务系统，所有操作均为模拟记录。');
  lines.push('  - 演练过程中产生的模拟数据不得作为真实工作依据。');
  lines.push('  - 脚本/通知等资料以系统导出为准。');
  lines.push('');
  lines.push('====================================================================');
  lines.push('  生成时间：' + now() + ' · 请妥善保存此通知并提前通知参演人员');
  lines.push('====================================================================');
  return lines.join('\r\n');
}

function escapeHtml(s) {
  return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}



/* 参演编组：按角色添加（不指定具体人员，由该角色演练账号登录参演） */
router.patch('/projects/:id/members', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '仅平台管理员/指挥部/信息科负责人可维护参演编组' });
  }
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  const raw = Array.isArray(req.body && req.body.members) ? req.body.members : null;
  if (!raw) return res.status(400).json({ error: '参演角色数据格式错误' });
  const seen = [];
  const members = [];
  raw.forEach(function (m) {
    const roleId = String((m && m.role) || m || '').trim();
    if (!roleId || seen.indexOf(roleId) >= 0) return;
    const r = db.prepare('SELECT id FROM roles WHERE id=?').get(roleId);
    if (!r) return;
    seen.push(roleId);
    members.push({ role: roleId });
  });
  if (!members.length) return res.status(400).json({ error: '请至少选择一个参演角色' });
  db.prepare('UPDATE projects SET members=? WHERE id=?').run(JSON.stringify(members), p.id);
  auth.audit(req, '更新演练项目 ' + p.id + ' 参演编组：' + members.map(function (m) { return m.role; }).join('、'));
  res.json({ ok: true, project: projectDto(db.prepare('SELECT * FROM projects WHERE id=?').get(p.id)) });
});

/* 删除演练项目（仅平台管理员）：清除全部关联数据与上传文件，不可恢复 */
router.delete('/projects/:id', auth.requireAuth, function (req, res) {
  if (req.user.role.id !== 'platformadmin') return res.status(403).json({ error: '仅平台管理员可删除演练项目' });
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });

  const taskCount = db.prepare('SELECT COUNT(*) AS c FROM tasks WHERE project_id=?').get(p.id).c;
  /* 汇总待物理删除的上传文件（任务附件 + 取证材料） */
  const files = [];
  db.prepare('SELECT file_path FROM task_attachments WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').all(p.id)
    .forEach(function (a) { if (a.file_path) files.push(a.file_path); });
  db.prepare('SELECT file_path FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?)').all(p.id, p.id)
    .forEach(function (e) { if (e.file_path) files.push(e.file_path); });

  const likeName = '%' + String(p.name || '').replace(/%/g, '\\%').replace(/_/g, '\\_') + '%';
  tx(function () {
    /* 全局表（无 project_id 列）仅按项目名 LIKE 兜底匹配，避免误删 */
    db.prepare('DELETE FROM notif_reads WHERE notif_id IN (SELECT id FROM notifs WHERE title LIKE ? OR descr LIKE ?)').run(likeName, likeName);
    db.prepare('DELETE FROM notifs WHERE title LIKE ? OR descr LIKE ?').run(likeName, likeName);
    db.prepare('DELETE FROM messages WHERE text LIKE ?').run(likeName);
    db.prepare('DELETE FROM chat_messages WHERE text LIKE ?').run(likeName);
    /* impact 为全局影响面快照，不按项目删除 */
    db.prepare('DELETE FROM task_attachments WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id);
    db.prepare('DELETE FROM form_submissions WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id);
    db.prepare('DELETE FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id, p.id);
    db.prepare('DELETE FROM tasks WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM drill_steps WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM drill_node_state WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM timeline WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM seq_flow WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM scores WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM snapshots WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM ledger WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM opinion WHERE project_id=?').run(p.id);
    db.prepare('DELETE FROM projects WHERE id=?').run(p.id);
  });

  files.forEach(function (f) {
    try { fs.unlinkSync(path.join(__dirname, '..', '..', 'uploads', f)); } catch (e) {}
  });

  /* 删除的是当前项目时，切换到剩余项目；无剩余项目则清空 */
  if (activeProjectId() === p.id) {
    const next = db.prepare('SELECT id FROM projects ORDER BY date DESC').get();
    if (next) db.prepare("UPDATE settings SET value=? WHERE key='active_project'").run(next.id);
    else db.prepare("DELETE FROM settings WHERE key='active_project'").run();
  }

  auth.audit(req, '删除演练项目 ' + p.id + '《' + p.name + '》并清理全部关联数据（任务 ' + taskCount + ' 项、上传文件 ' + files.length + ' 个）');
  res.json({ ok: true, id: p.id, tasks: taskCount, files: files.length });
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
  /* 站内消息：自动考评完成（模板驱动） */
  const sum = items.reduce(function (a, s) { return a + (Number(s.score) || 0); }, 0);
  const max = items.reduce(function (a, s) { return a + (Number(s.max) || 0); }, 0);
  notify.push({
    event: 'auto_eval',
    vars: { count: items.length, sum: sum, max: max ? '/' + max : '' }
  });
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
  const roleMap = {};
  db.prepare('SELECT ur.user_id, r.id, r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id ORDER BY r.seq')
    .all().forEach(function (x) {
      (roleMap[x.user_id] = roleMap[x.user_id] || []).push({ id: x.id, name: x.name });
    });
  res.json(db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,r.name AS role_name ' +
    'FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id'
  ).all().map(function (u) {
    const roleList = roleMap[u.id] || [{ id: u.role_id, name: u.role_name }];
    return {
      id: u.id, username: u.username, name: u.name, role: u.role_id, roleName: u.role_name,
      roles: roleList.map(function (x) { return x.id; }), roleList: roleList,
      org: u.org, status: u.status
    };
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
  const projects = db.prepare('SELECT * FROM projects ORDER BY date DESC').all().map(monitorProjectDto);
  const pid = String(req.query.projectId || '') || activeProjectId() || (projects[0] && projects[0].id);
  const p = projects.filter(function (x) { return x.id === pid; })[0] || projects[0];
  if (!p) {
    return res.json({ serverTime: nowSec(), project: null, projects: [], stats: { total: 0, done: 0, doing: 0, pending: 0, overdue: 0 }, tasks: [], timeline: [], seqFlow: [], impact: [], messages: [], roles: [] });
  }
  /* 从 projects 表取完整项目对象（含 scenario_id、mode），monitorProjectDto 只带部分字段 */
  const pFull = db.prepare('SELECT * FROM projects WHERE id=?').get(p.id);
  const mode = pFull.mode || '基础版';

  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=? ORDER BY seq').all(p.id);
  const timeline = db.prepare('SELECT * FROM timeline WHERE project_id=? ORDER BY seq').all(p.id).slice(-200);
  const drillSteps = db.prepare('SELECT * FROM drill_steps WHERE project_id=?').all(p.id);
  const nodeStates = db.prepare('SELECT node_id FROM drill_node_state WHERE project_id=?').all(p.id);

  /* ---------- seqFlow 动态计算（三态完整 + 过滤子集） ---------- */
  const seqData = computeMonitorNodes(pFull);
  const chainNodes = seqData.all;           // 完整剧本节点（三态），用于处置链路
  const seqFlow = seqData.filtered;         // 仅 done/doing，用于左侧处置进度

  // 用 drill.score() 返回的节点结果覆盖 isOverdue，与评分模块完全对齐
  try {
    const scr = require('./drill').score(p.id);
    const scoredNodes = {};
    (scr.nodes || []).forEach(function (sn) { scoredNodes[sn.id] = sn; });
    chainNodes.forEach(function (cn) {
      const sn = scoredNodes[cn.id];
      if (sn) {
        cn.isOverdue = !!sn.late || (sn.state === '遗漏');
        if (sn.late && cn.state !== 'done') cn.state = 'done';
      }
    });
  } catch (e) {}

  /* ---------- roles 表 ---------- */
  const roles = db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return { id: r.id, name: r.name, color: r.color, external: !!r.external };
  });

  /* ---------- stats 综合统计（基于 chainNodes 真实三态） ---------- */
  const total = chainNodes.length;
  let doneCnt = 0, doingCnt = 0, pendingCnt = 0;
  chainNodes.forEach(function (n) {
    if (n.state === 'done') doneCnt++;
    else if (n.state === 'doing') doingCnt++;
    else pendingCnt++;
  });
  // 超时：对齐角色工作台评分模块 drill.score() 的取数
  // late = 超时完成数，missed = 演练结束未完成数
  let overdue = 0;
  let scoreInfo = null;
  try {
    const scr = require('./drill').score(p.id);
    overdue = (scr.late || 0) + (scr.missed || 0);
    // 评分模块完整数据（供大屏评分模块 + 扣分原因消息使用）
    scoreInfo = {
      score: scr.score, grade: scr.grade,
      total: scr.total, hit: scr.hit, ontime: scr.ontime,
      late: scr.late, missed: scr.missed, pending: scr.pending,
      completeness: scr.completeness, timeRate: scr.timeRate,
      deductions: (scr.deductions || []).map(function (d) {
        return { nodeId: d.nodeId, title: d.title, type: d.type, point: d.point };
      })
    };
  } catch (e) {}

  /* ---------- 问题 4：角色聚合统一映射 ---------- */
  const roleAgg = {};
  const roleById = {};
  roles.forEach(function (r) { roleById[r.id] = r; });

  // 基础版从 tasks 聚合
  tasks.forEach(function (t) {
    const rid = t.role_id || '';
    if (!rid) return;
    roleAgg[rid] = roleAgg[rid] || { roleId: rid, total: 0, done: 0, overtime: 0 };
    roleAgg[rid].total++;
    if (t.status === '已完成') roleAgg[rid].done++;
    if (t.status === '超时') roleAgg[rid].overtime++;
  });
  // 实战版从 drill_steps 聚合，统一 targetToRoleId 映射
  drillSteps.forEach(function (s) {
    const rid = targetToRoleId(db, s.role_id || s.target || '');
    if (!rid) return;
    roleAgg[rid] = roleAgg[rid] || { roleId: rid, total: 0, done: 0, overtime: 0 };
    roleAgg[rid].total++;
    if (Number(s.correct) === 1 && Number(s.ontime) !== 0) roleAgg[rid].done++;
    if (Number(s.correct) === 1 && Number(s.ontime) === 0) roleAgg[rid].overtime++;
  });

  /* ---------- impact 关联 project_id ---------- */
  const impact = db.prepare('SELECT * FROM impact WHERE project_id=? OR project_id IS NULL ORDER BY seq').all(p.id)
    .map(function (i) { return { sys: i.sys, state: i.state, pct: i.pct }; });

  /* ---------- 问题 5：messages 合并站内 + 院内会商 ---------- */
  const msgs = db.prepare('SELECT * FROM messages WHERE project_id=? OR project_id IS NULL ORDER BY seq').all(p.id).slice(-16);
  const chatMsgs = db.prepare("SELECT * FROM chat_messages WHERE grp='inner' AND (project_id=? OR project_id IS NULL) ORDER BY id DESC LIMIT 16").all(p.id);
  const mergedMessages = [].concat(
    msgs.map(function (m) {
      return { time: m.time || '', level: m.level || 'P2', source: m.source || '', text: m.text || '' };
    }),
    chatMsgs.map(function (c) {
      // chat_messages.time 已是 YYYY-MM-DD HH:mm 格式，统一截取到 HH:mm 方便排序
      const t = String(c.time || '');
      return {
        time: t,
        level: 'P2',
        source: '院内会商·' + (c.sender || ''),
        text: c.text || ''
      };
    })
  ).sort(function (a, b) {
    // 时间格式兼容：messages.time 可能是 HH:mm，chat_messages.time 是 YYYY-MM-DD HH:mm
    return String(b.time).localeCompare(String(a.time));
  }).slice(0, 24);

  res.json({
    serverTime: nowSec(),
    project: p,
    projects: projects,
    stats: {
      total: total,
      done: doneCnt,
      doing: doingCnt,
      pending: pendingCnt,
      overdue: overdue
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
    seqFlow: seqFlow,
    chainNodes: chainNodes,
    score: scoreInfo,
    impact: impact,
    messages: mergedMessages,
    roles: roles,
    roleAgg: roleAgg
  });
});

/* ---------- 态势总看板聚合接口（替代 dashboard.html 5 个独立 API 调用） ---------- */
router.get('/dashboard', auth.requireAuth, function (req, res) {
  const projectId = String(req.query.projectId || '') || activeProjectId();
  if (!projectId) {
    return res.json({ serverTime: nowSec(), project: null, stats: {}, seqFlow: [], roles: [], docs: [], msgs: [], impact: [] });
  }
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(projectId);
  if (!p) {
    return res.json({ serverTime: nowSec(), project: null, stats: {}, seqFlow: [], roles: [], docs: [], msgs: [], impact: [] });
  }

  /* ---- sessions 清理 + 在线数 ---- */
  try { db.prepare('DELETE FROM sessions WHERE expires_at IS NOT NULL AND expires_at <= datetime(\'now\')').run(); } catch (e) {}
  const onlineActors = db.prepare('SELECT COUNT(DISTINCT user_id) AS c FROM sessions WHERE role_id IS NOT NULL').get().c;
  const totalActors = db.prepare("SELECT COUNT(*) AS c FROM users WHERE COALESCE(status,'启用')='启用'").get().c;

  /* ---- 评分：复用 drill.score() 取 overdue/wrong/score/grade ---- */
  let scr = null;
  try { scr = require('./drill').score(projectId); } catch (e) { scr = null; }
  const overdueCnt = scr ? (scr.late + scr.missed) : 0;
  const wrongCnt = scr ? scr.wrong : 0;
  const finalScore = scr ? scr.score : null;
  const finalGrade = scr ? scr.grade : null;

  /* ---- script_nodes + drill_steps → seqFlow ---- */
  const nodes = db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? AND type!=\'end\' ORDER BY CAST(SUBSTR(id, INSTR(id,\'-\')+1) AS INTEGER), id')
    .all(p.scenario_id || '');
  const correct = nodes.filter(function (n) { return n.flow_type !== 'distractor'; });
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY id').all(projectId);
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(projectId);

  const stepByNode = {};
  steps.forEach(function (s) { if (s.matched_node) stepByNode[s.matched_node] = s; });

  const seqFlow = [];
  correct.forEach(function (n) {
    const st = stepByNode[n.id];
    let state = '待处置';
    let completedAt = null;
    let deduct = 0;
    if (st) {
      if (st.skipped === 1) { state = '已跳过'; }
      else {
        state = Number(st.ontime) === 0 ? '超时完成' : '已完成';
        completedAt = st.step_time || null;
        if (Number(st.ontime) === 0) deduct += Math.round(Number(n.score || 0) * 0.1 * 10) / 10;
        if (Number(st.receiver_ok) === 0) deduct += Math.round(Number(n.score || 0) * 0.1 * 10) / 10;
      }
    } else {
      const t = tasks.filter(function (x) { return x.node_id === n.id; })[0];
      if (t) {
        if (t.status === '已完成') { state = '已完成'; completedAt = t.completed_at || null; }
        else if (t.status === '处理中') state = '处置中';
        else if (t.status === '超时') { state = '超时完成'; }
      }
    }
    seqFlow.push({
      id: n.id, title: n.title, role: n.target, flowType: n.flow_type || 'correct',
      state: state, completedAt: completedAt, score: Number(n.score || 0), deduct: deduct
    });
  });
  /* 混淆节点：仅在 drill_steps 中被执行后追加 */
  steps.forEach(function (s) {
    if (!s.matched_node || s.skipped === 1) return;
    if (s.correct !== 0) return;
    if (seqFlow.some(function (x) { return x.id === s.matched_node; })) return;
    const n = nodes.filter(function (x) { return x.id === s.matched_node; })[0];
    seqFlow.push({
      id: s.matched_node, title: s.title || (n && n.title) || '混淆流程',
      role: (n && n.target) || '', flowType: 'distractor',
      state: '已执行', completedAt: s.step_time,
      score: n ? Number(n.score || 10) : 10, deduct: n ? Number(n.score || 10) : 10
    });
  });

  /* ---- 角色聚合 ---- */
  const roleMap = {};
  db.prepare('SELECT id,name FROM roles').all().forEach(function (r) { roleMap[r.id] = r.name; });
  const roleAgg = {};
  tasks.forEach(function (t) {
    const rid = t.role_id || '';
    roleAgg[rid] = roleAgg[rid] || { roleId: rid, roleName: roleMap[rid] || rid, total: 0, done: 0, overtime: 0, wrong: 0 };
    roleAgg[rid].total++;
    if (t.status === '已完成') roleAgg[rid].done++;
    if (t.status === '超时') roleAgg[rid].overtime++;
  });
  steps.forEach(function (s) {
    if (!s.role_id) return;
    roleAgg[s.role_id] = roleAgg[s.role_id] || { roleId: s.role_id, roleName: roleMap[s.role_id] || s.role_id, total: 0, done: 0, overtime: 0, wrong: 0 };
    if (Number(s.ontime) === 0 && Number(s.correct) === 1) roleAgg[s.role_id].overtime++;
    if (Number(s.correct) === 0 && s.skipped !== 1) roleAgg[s.role_id].wrong++;
  });

  /* ---- docs (evidence) — 按 task_id+type+name 去重，保留最新一条 ---- */
  const rawDocs = db.prepare(
    'SELECT * FROM evidence WHERE project_id=? AND (type=\'表单文书\' OR type=\'处置记录\') ORDER BY id DESC LIMIT 50'
  ).all(projectId);
  const seenDocKey = {};
  const docs = rawDocs.filter(function (d) {
    const k = String(d.task_id || '') + '|' + String(d.type || '') + '|' + String(d.name || '');
    if (seenDocKey[k]) return false;
    seenDocKey[k] = true;
    return true;
  }).map(function (d) {
    let t = String(d.created_at || d.time || '').trim();
    if (t && /^\d{1,2}:\d{2}/.test(t)) {
      const base = (p.started_at || p.date || '').slice(0, 10);
      t = (base && /^\d{4}-\d{1,2}-\d{1,2}/.test(base)) ? (base + ' ' + t) : (nowSec().slice(0, 10) + ' ' + t);
    }
    return { id: d.id, name: d.name, time: t, uploader: d.uploader, type: d.type };
  });

  /* ---- msgs (timeline) ---- */
  const tl = db.prepare('SELECT * FROM timeline WHERE project_id=? ORDER BY id DESC LIMIT 40').all(projectId);
  const msgs = tl.map(function (t) {
    return { time: t.t, level: t.level || '', actor: t.actor, action: t.action, detail: t.detail };
  });

  /* ---- impact ---- */
  const impact = db.prepare('SELECT sys,state,pct FROM impact ORDER BY seq').all();

  /* ---- tasks 统计 ---- */
  const statOf = function (status) { return tasks.filter(function (t) { return t.status === status; }).length; };

  res.json({
    serverTime: nowSec(),
    project: { id: p.id, name: p.name, status: p.status, mode: p.mode, level: p.level, startedAt: p.started_at, date: p.date },
    stats: {
      totalTasks: tasks.length, doneTasks: statOf('已完成'), doingTasks: statOf('处理中'),
      pendingTasks: statOf('待处理'), overdueTasks: overdueCnt, wrongTasks: wrongCnt,
      onlineActors: onlineActors, totalActors: totalActors,
      score: finalScore, grade: finalGrade, mode: p.mode
    },
    seqFlow: seqFlow,
    roles: Object.keys(roleAgg).map(function (k) { return roleAgg[k]; }),
    docs: docs,
    msgs: msgs,
    impact: impact
  });
});

module.exports = router;
