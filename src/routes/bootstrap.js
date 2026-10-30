/* 全量数据引导接口：登录后一次性下发页面渲染所需的演练数据（按角色权限过滤） */
'use strict';
const express = require('express');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { nowSec } = require('../util');
const notify = require('../notify');
const clock = require('../drill_clock');

const router = express.Router();

/* 信息科负责人可统筹的信息科条线角色 */
const MANAGED_ROLES = ['itlead', 'duty', 'netsec', 'sysadmin'];

/* 外部单位可见的取证类型（仅医院主动报送的材料） */
const EXTERNAL_EVIDENCE_TYPES = ['表单文书', '上报材料', '报案材料'];

/* 项目动态进度计算（与 content.js 的 projectProgress 口径一致） */
function projectProgress(r) {
  if (!r) return 0;
  if (r.status === '已结束' || r.status === '已归档') return 100;
  if (r.status === '未开始') return 0;
  const mode = r.mode || '基础版';
  if (mode === '基础版') {
    const ts = db.prepare('SELECT COUNT(*) AS total, SUM(CASE WHEN status=? THEN 1 ELSE 0 END) AS done FROM tasks WHERE project_id=?')
      .get('已完成', r.id);
    if (!ts || !ts.total) return 0;
    return Math.round((ts.done || 0) / ts.total * 100);
  } else {
    if (!r.scenario_id) return 0;
    const correct = db.prepare("SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor'")
      .get(r.scenario_id).c;
    if (!correct) return 0;
    const doneRow = db.prepare(
      'SELECT COUNT(DISTINCT ds.matched_node) AS c FROM drill_steps ds '
      + "JOIN script_nodes n ON n.id=ds.matched_node AND n.scenario_id=? AND COALESCE(n.flow_type,'correct')!='distractor' AND n.type!='end' "
      + 'WHERE ds.project_id=?'
    ).get(r.scenario_id, r.id);
    const done = doneRow ? doneRow.c : 0;
    return Math.round(done / correct * 100);
  }
}

function mapProject(r) {
  return {
    id: r.id, name: r.name, type: r.type, date: r.date, scenarioId: r.scenario_id,
    level: r.level, status: r.status, target: r.target,
    units: parseJson(r.units, []), members: parseJson(r.members, []), observers: parseJson(r.observers, []),
    mode: r.mode || '基础版',
    startedAt: r.started_at || null, pausedAt: r.paused_at || null,
    pausedSec: Number(r.paused_sec) || 0, endedAt: r.ended_at || null,
    elapsedSec: clock.elapsedSec(r),
    progress: projectProgress(r)
  };
}

function mapScenario(r) {
  return {
    id: r.id, name: r.name, type: r.type, level: r.level,
    status: r.status, version: r.version, updated: r.updated, nodes: r.nodes, desc: r.descr,
    alert: r.alert || ''
  };
}

function mapNode(r) {
  return {
    id: r.id, scenarioId: r.scenario_id, type: r.type, title: r.title, target: r.target,
    timeout: r.timeout, form: r.form || undefined, att: r.att || undefined,
    desc: r.descr, branches: parseJson(r.branches, []), parallelDesc: r.parallel_desc || undefined,
    guide: r.guide || '', keywords: (r.keywords || '').split(',').filter(Boolean),
    action: r.action || '', required: r.required === undefined ? true : !!r.required,
    wrong: parseJson(r.wrong, []),
    evidenceReq: parseJson(r.evidence_req, []),
    flowType: r.flow_type === 'distractor' ? 'distractor' : 'correct',
    score: Number(r.score || 0) || 0,
    receiver: r.receiver || '',
    deps: parseJson(r.deps, []),
    posX: r.pos_x === null || r.pos_x === undefined ? null : Number(r.pos_x),
    posY: r.pos_y === null || r.pos_y === undefined ? null : Number(r.pos_y)
  };
}

function mapTask(r, atts) {
  return {
    id: r.id, projectId: r.project_id, role: r.role_id, title: r.title, desc: r.descr,
    deadline: r.deadline, status: r.status, level: r.level, nodeId: r.node_id || null,
    completedAt: r.completed_at || null,
    forms: parseJson(r.forms, []), atts: atts || []
  };
}

function mapAtt(r) {
  return {
    id: r.id, name: r.name, type: r.type, size: r.size,
    url: r.url, by: r.uploader, time: r.time, auto: !!r.auto, filePath: r.file_path || null
  };
}

function mapEvidence(r) {
  return {
    id: r.id, type: r.type, name: r.name, size: r.size, time: r.time,
    role: r.role_id, uploader: r.uploader, auto: !!r.auto,
    taskId: r.task_id || null, projectId: r.project_id || null,
    note: r.note || '', hasFile: !!r.file_path,
    url: r.file_path ? ('/uploads/' + r.file_path) : null
  };
}

/* 任务可见范围（与角色工作台口径一致） */
function visibleTasks(user, all) {
  const rid = user.role.id;
  if (rid === 'commander') return all;
  /* 仅信息科负责人可统筹处置本科室条线任务，其余角色只能看到本角色任务 */
  if (rid === 'itlead') {
    return all.filter(function (t) { return MANAGED_ROLES.indexOf(t.role_id) >= 0; });
  }
  return all.filter(function (t) { return t.role_id === rid; });
}

router.get('/bootstrap', auth.requireAuth, function (req, res) {
  const u = req.user;
  const rid = u.role.id;
  const isExternal = !!u.role.external;
  const isAdmin = rid === 'platformadmin';
  const isCommander = rid === 'commander';

  const activeId = (db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value;

  const roles = db.prepare('SELECT * FROM roles ORDER BY seq').all().map(function (r) {
    return {
      id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color,
      desc: r.descr, nav: parseJson(r.nav, []), external: !!r.external, status: r.status || '启用'
    };
  });

  const roleMap = {};
  db.prepare('SELECT ur.user_id, r.id, r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id ORDER BY r.seq')
    .all().forEach(function (x) {
      (roleMap[x.user_id] = roleMap[x.user_id] || []).push({ id: x.id, name: x.name });
    });
  const users = db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,r.name AS role_name ' +
    'FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id'
  ).all().map(function (r) {
    const roleList = roleMap[r.id] || [{ id: r.role_id, name: r.role_name }];
    return {
      id: r.id, username: r.username, name: r.name, role: r.role_id, roleName: r.role_name,
      roles: roleList.map(function (x) { return x.id; }), roleList: roleList,
      org: r.org, status: r.status
    };
  });

  const projects = db.prepare('SELECT * FROM projects').all().map(mapProject);
  const scenarios = db.prepare('SELECT * FROM scenarios').all().map(mapScenario);
  const nodes = db.prepare('SELECT * FROM script_nodes ORDER BY scenario_id,seq').all().map(mapNode);

  const practical = {};
  db.prepare("SELECT id FROM projects WHERE mode='实战版'").all().forEach(function (p) { practical[p.id] = true; });
  /* 实战版不发布任务提示：任务中心与首页待办均不展示该项目的任务 */
  const allTaskRows = db.prepare('SELECT * FROM tasks ORDER BY seq').all()
    .filter(function (t) { return !practical[t.project_id]; });
  const visTasks = visibleTasks(u, allTaskRows);

  const attRows = db.prepare('SELECT * FROM task_attachments ORDER BY id').all();
  const attByTask = {};
  attRows.forEach(function (a) {
    (attByTask[a.task_id] = attByTask[a.task_id] || []).push(mapAtt(a));
  });

  const tasks = visTasks.map(function (t) { return mapTask(t, attByTask[t.id] || []); });
  const allTasks = isCommander ? allTaskRows.map(function (t) { return mapTask(t, attByTask[t.id] || []); }) : tasks;

  /* 取证材料：外部单位仅可见报送类材料 */
  let evidenceRows = db.prepare('SELECT * FROM evidence ORDER BY created_at, id').all();
  if (isExternal) {
    evidenceRows = evidenceRows.filter(function (e) {
      return EXTERNAL_EVIDENCE_TYPES.indexOf(e.type) >= 0 ||
        ['itlead', 'commander'].indexOf(e.role_id) >= 0;
    });
  }

  /* 时序：外部单位仅可见报送与接收节点 */
  let timelineRows = db.prepare('SELECT * FROM timeline ORDER BY seq').all();
  if (isExternal) {
    timelineRows = timelineRows.filter(function (t) {
      return /上报|报送|接收|督办|报案|审批/.test(t.action || '');
    });
  }

  /* 审计日志：仅平台管理员可见 */
  const auditLogs = isAdmin
    ? db.prepare('SELECT * FROM audit_logs ORDER BY id DESC LIMIT 500').all().map(function (a) {
        return { time: a.time, user: a.user, role: a.role_id, action: a.action, ip: a.ip };
      })
    : [];

  /* 导调中心频道权限：院内角色仅院内会商，外部单位仅院外上报，信息科负责人/指挥部可同时使用 */
  const CHAT_ROLE_ACCESS = {
    duty: ['inner'], netsec: ['inner'], sysadmin: ['inner'], medlead: ['inner'], clinical: ['inner'],
    healthbureau: ['external'], cac: ['external'], cyberpolice: ['external'],
    itlead: ['inner', 'external'], commander: ['inner', 'external']
  };
  const allowedChat = CHAT_ROLE_ACCESS[u.role.id] || [];
  const chatRows = db.prepare('SELECT * FROM chat_messages ORDER BY id').all();
  const chat = {};
  chatRows.forEach(function (c) {
    if (allowedChat.indexOf(c.grp) < 0) return;
    (chat[c.grp] = chat[c.grp] || []).push({
      id: c.id, sender: c.sender, role: c.role_id, time: c.time, text: c.text, att: c.att || null,
      url: c.file_path ? '/uploads/' + c.file_path : null
    });
  });

  const readRows = db.prepare('SELECT notif_id FROM notif_reads WHERE user_id=?').all(u.id);

  const dict = {};
  db.prepare('SELECT * FROM dict').all().forEach(function (d) { dict[d.key] = parseJson(d.value, []); });

  const simData = {};
  db.prepare('SELECT * FROM sim_data').all().forEach(function (s) { simData[s.type] = parseJson(s.data, {}); });

  const scores = db.prepare('SELECT * FROM scores WHERE project_id=?').all(activeId).map(function (s) {
    return { itemId: s.item_id, score: s.score, max: s.max, note: s.note, auto: !!s.auto };
  });

  res.json({
    serverTime: nowSec(),
    activeProjectId: activeId || (projects[0] && projects[0].id),
    currentUser: { id: u.id, username: u.username, name: u.name, org: u.org, roleId: rid },
    roles: roles,
    users: isAdmin || isCommander ? users : [],
    projects: projects,
    scenarios: scenarios,
    scriptNodes: nodes,
    tasks: allTasks,
    formTemplates: db.prepare('SELECT * FROM form_templates ORDER BY id').all()
      .map(function (f) { return require('../forms').normalizeTemplate(f); }),
    messages: db.prepare('SELECT * FROM messages ORDER BY seq').all().map(function (m) {
      return { id: m.id, time: m.time, level: m.level, source: m.source, text: m.text, targets: parseJson(m.targets, []) };
    }),
    notifs: db.prepare('SELECT * FROM notifs ORDER BY seq').all().filter(function (n) {
      /* 外部单位仅可见公开通知，演练内部消息按权限隔离 */
      return !isExternal || (n.target || 'all') !== 'internal';
    }).map(function (n) {
      return {
        id: n.id, time: n.time, level: n.level, source: n.source, title: n.title, desc: n.descr,
        read: readRows.some(function (r) { return r.notif_id === n.id; })
      };
    }),
    chat: chat,
    evidence: evidenceRows.map(mapEvidence),
    snapshots: db.prepare('SELECT * FROM snapshots ORDER BY id').all().map(function (s) {
      return { id: s.id, name: s.name, time: s.time, by: s.by_user, note: s.note };
    }),
    timeline: timelineRows.map(function (t) {
      return { projectId: t.project_id, t: t.t, actor: t.actor, role: t.role_id, action: t.action, detail: t.detail, level: t.level };
    }),
    seqFlow: db.prepare('SELECT * FROM seq_flow ORDER BY seq').all().map(function (s) {
      return { projectId: s.project_id, name: s.name, time: s.time, role: s.role, state: s.state };
    }),
    scoreItems: db.prepare('SELECT * FROM score_items ORDER BY seq').all().map(function (k) {
      return { id: k.id, name: k.name, weight: k.weight, rule: k.rule };
    }),
    scores: scores,
    ledger: db.prepare('SELECT * FROM ledger').all().map(function (l) {
      return { id: l.id, name: l.name, date: l.date, type: l.type, event: l.event, level: l.level, result: l.result, score: l.score, org: l.org };
    }),
    opinion: db.prepare('SELECT * FROM opinion ORDER BY id').all().map(function (o) {
      return { id: o.id, time: o.time, channel: o.channel, title: o.title, heat: o.heat, risk: o.risk, status: o.status, source: o.source };
    }),
    knowledge: db.prepare('SELECT * FROM knowledge ORDER BY id').all().map(function (k) {
      return {
        id: k.id, name: k.name, cat: k.cat, updated: k.updated, size: k.size, pages: k.pages,
        content: k.content, filePath: k.file_path || null,
        url: k.file_path ? '/uploads/' + k.file_path : null
      };
    }),
    auditLogs: auditLogs,
    impact: db.prepare('SELECT * FROM impact ORDER BY seq').all().map(function (i) {
      return { sys: i.sys, state: i.state, pct: i.pct };
    }),
    dict: dict,
    simData: simData
  });
});

/* 项目切换 / 状态更新（写入会话级当前项目，全局共享） */
router.post('/projects/:id/activate', auth.requireAuth, function (req, res) {
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  db.prepare("INSERT INTO settings (key,value) VALUES ('active_project',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").run(p.id);
  auth.audit(req, '切换演练项目至 ' + p.id + '《' + p.name + '》');
  res.json({ ok: true, project: mapProject(p) });
});

router.patch('/projects/:id/status', auth.requireAuth, function (req, res) {
  const status = String((req.body && req.body.status) || '');
  const allow = ['未开始', '演练中', '暂停', '已结束', '已归档'];
  if (allow.indexOf(status) < 0) return res.status(400).json({ error: '非法的项目状态' });
  const p = db.prepare('SELECT * FROM projects WHERE id=?').get(req.params.id);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  const prev = p.status;
  /* 真实计时：启动记录起点，暂停冻结，继续扣除暂停时长，结束/归档冻结 */
  const ts = nowSec();
  let started = p.started_at, pausedAt = p.paused_at;
  let pausedSec = Number(p.paused_sec) || 0, endedAt = p.ended_at;
  if (status === '未开始') {
    started = null; pausedAt = null; pausedSec = 0; endedAt = null;
  } else if (status === '演练中') {
    if (!started) started = ts;
    if (prev === '暂停' && pausedAt) {
      pausedSec += clock.secBetween(pausedAt, ts);
      pausedAt = null;
    }
    endedAt = null;
  } else if (status === '暂停' && prev !== '暂停') {
    pausedAt = ts;
  } else if ((status === '已结束' || status === '已归档') && prev !== status) {
    if (pausedAt) {
      pausedSec += clock.secBetween(pausedAt, ts);
      pausedAt = null;
    }
    endedAt = ts;
  }
  db.prepare('UPDATE projects SET status=?, started_at=?, paused_at=?, paused_sec=?, ended_at=? WHERE id=?')
    .run(status, started, pausedAt, pausedSec, endedAt, p.id);
  auth.audit(req, '更新项目 ' + p.id + ' 状态为「' + status + '」');
  /* 站内消息：演练启动/继续、演练结束（模板驱动） */
  const mode = p.mode || '基础版';
  if (status === '演练中' && prev !== '演练中') {
    const resume = prev === '暂停';
    notify.push({
      event: 'drill_start',
      vars: {
        action: resume ? '演练继续' : '演练开始',
        project: p.name,
        resumed: resume ? '已恢复' : '已启动',
        mode: mode,
        level: p.level || '较大'
      },
      level: p.level === '重大' ? 'P0' : 'P1'
    });
  } else if (status === '已结束' && prev !== '已结束') {
    notify.push({
      event: 'drill_end',
      vars: { project: p.name, mode: mode }
    });
  }
  res.json({ ok: true, project: mapProject(db.prepare('SELECT * FROM projects WHERE id=?').get(p.id)) });
});

module.exports = router;
