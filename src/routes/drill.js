/* 演练模式与自动考评：
 * 两种模式共用同一套剧本基线，模式在新建演练项目时选定：
 * 基础版 = 按剧本节点自动下发任务并给出引导提示，仅对操作时限打分；
 * 实战版 = 任务中心只展示“已发现的系统故障”，参演人员在角色工作台自主选择处置方案，
 *          与剧本基线偏离扣分、超时扣分、未处置扣分。
 */
'use strict';
const express = require('express');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { now, hhmm, nowSec } = require('../util');
const { canHandle } = require('../drill_plan');
const notify = require('../notify');
const clock = require('../drill_clock');
const drillDict = require('../drill_dict');

const router = express.Router();

/* 节点处置时长展示文案 */
function planText(n) {
  return n && Number(n.timeout) > 0 ? ('处置时长 ' + n.timeout + ' 分钟') : '未设置处置时长';
}

/* 节点起算时刻：取所有前置节点中最晚的实际完成时间；无前置时取演练真实开始时间 */
function nodeStartMs(p, node) {
  let best = null;
  (node.deps || []).forEach(function (depId) {
    const s = db.prepare('SELECT created_at FROM drill_steps WHERE project_id=? AND matched_node=? ORDER BY id LIMIT 1')
      .get(p.id, depId);
    if (!s) return;
    const ms = clock.parseLocal(s.created_at);
    if (isFinite(ms) && (best === null || ms > best)) best = ms;
  });
  if (best !== null) return best;
  const st = clock.parseLocal(p.started_at);
  return isFinite(st) ? st : Date.now();
}

/* 是否超时：真实当前时间 - 节点起算时间 > 处置时长 */
function isLate(p, node, atMs) {
  if (!node || !(Number(node.timeout) > 0)) return false;
  const start = nodeStartMs(p, node);
  const end = (typeof atMs === 'number') ? atMs : Date.now();
  return (end - start) / 60000 > Number(node.timeout);
}

/* 参演人员判定「不执行」跳过混淆流程时的少量奖励分 */
const SKIP_BONUS = 2;

/* 接收人支持多选：字符串（顿号/逗号分隔）或数组，统一转为去重排序的名称数组 */
function parseReceivers(v) {
  let arr = [];
  if (Array.isArray(v)) arr = v;
  else arr = String(v || '').split(/[、,，;；]/);
  const out = [];
  arr.forEach(function (x) {
    const s = String(x || '').trim().slice(0, 40);
    if (s && out.indexOf(s) < 0) out.push(s);
  });
  return out.sort();
}

/* 接收人判定：集合完全一致才算正确（多选、漏选均视为错误） */
function sameReceivers(a, b) {
  return a.length === b.length && a.every(function (x, i) { return x === b[i]; });
}

function nodesOf(scenarioId) {
  return db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(scenarioId).map(function (n) {
    return {
      id: n.id, type: n.type, title: n.title, target: n.target, timeout: n.timeout || 0,
      desc: n.descr || '', guide: n.guide || '', keywords: (n.keywords || '').split(',').filter(Boolean),
      action: n.action || '', required: !!n.required, wrong: parseJson(n.wrong, []),
      form: n.form || '', flowType: n.flow_type === 'distractor' ? 'distractor' : 'correct',
      score: Number(n.score || 0) || 0, receiver: n.receiver || '', deps: parseJson(n.deps, [])
    };
  });
}

/* 可选接收角色（全部角色中文名，供实战版参演人员选择） */
function receiverOptions() {
  return db.prepare('SELECT name FROM roles ORDER BY seq').all().map(function (r) { return r.name; });
}

/* 取证材料完成情况（全场景，用于「完成演练」前置校验） */
function evidenceSummary(project) {
  const archived = {};
  db.prepare('SELECT req_key FROM evidence WHERE project_id=? AND req_key IS NOT NULL').all(project.id)
    .forEach(function (e) { archived[e.req_key] = true; });
  const pending = [];
  let total = 0;
  db.prepare("SELECT * FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq").all(project.scenario_id)
    .forEach(function (n) {
      parseJson(n.evidence_req, []).forEach(function (it, i) {
        total++;
        const key = n.id + '#' + i;
        if (archived[key]) return;
        pending.push({ key: key, nodeId: n.id, nodeTitle: n.title, target: n.target, type: it.type, name: it.name });
      });
    });
  return { total: total, done: total - pending.length, pending: pending.length, pendingItems: pending };
}

/* 表单模板字段（实战版处置表单按节点关联表单展示） */
function formFields(formId) {
  if (!formId) return [];
  const f = db.prepare('SELECT * FROM form_templates WHERE id=?').get(String(formId));
  if (!f) return [];
  return { name: f.name || formId, fields: require('../forms').normalizeFields(parseJson(f.fields, [])) };
}

function projectOf(projectId) {
  return db.prepare('SELECT * FROM projects WHERE id=?').get(projectId);
}

/* 旧版自由步骤匹配基线节点（仅用于历史数据兼容） */
function matchNode(step, nodes) {
  const pool = nodes.filter(function (n) { return n.required && n.type !== 'end' && n.flowType !== 'distractor'; });
  const text = (step.title || '') + ' ' + (step.descr || '') + ' ' + (step.action || '');
  let best = null, bestScore = 0;
  pool.forEach(function (n) {
    let s = 0;
    if (step.action && n.action && step.action === n.action) s += 3;
    (n.keywords || []).forEach(function (k) {
      if (k && text.indexOf(k) >= 0) s += 2;
    });
    if (n.target && step.roleId && n.target === step.roleId) s += 0.5;
    if (s > bestScore) { bestScore = s; best = n; }
  });
  return { node: best, score: bestScore };
}

/* 评分（每个流程独立分值，所有正确流程合计满分 100）
 * 基础版：正确流程完成得分；超时完成扣该流程分值的 10%；演练结束时未完成扣该流程全部分值
 * 实战版：额外选择混淆流程直接扣该流程分值；混淆流程不参与得分
 */
function score(projectId) {
  const p = projectOf(projectId);
  if (!p) return null;
  const mode = p.mode || '基础版';
  const ended = p.status === '已结束' || p.status === '已归档';
  const all = nodesOf(p.scenario_id).filter(function (n) { return n.type !== 'end'; });
  const correct = all.filter(function (n) { return n.flowType !== 'distractor'; });
  const distractors = all.filter(function (n) { return n.flowType === 'distractor'; });
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(projectId);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(projectId);

  /* 基础版任务与剧本节点对应关系（历史任务可能未回填 node_id，按标题/描述兜底匹配） */
  const taskByNode = {};
  tasks.forEach(function (t) {
    let node = t.node_id ? correct.filter(function (n) { return n.id === t.node_id; })[0] : null;
    if (!node) node = matchNode({ title: t.title, descr: t.descr, roleId: t.role_id }, correct).node;
    if (node && !taskByNode[node.id]) taskByNode[node.id] = t;
  });
  /* 基础版任务完成时刻：优先完整日期时间；历史数据只有 HH:mm 时按完成先后滚动到次日 */
  const startMs = clock.parseLocal(p.started_at);
  const baseStart = isFinite(startMs) ? startMs : clock.parseLocal(String(p.date || now()));
  const doneCache = {};
  function resolveStamp(raw, afterMs) {
    const s = String(raw || '').trim();
    if (!s) return NaN;
    if (/^\d{4}-\d{1,2}-\d{1,2}/.test(s)) return clock.parseLocal(s);
    const m = /^(\d{1,2}):(\d{2})/.exec(s);
    if (!m) return NaN;
    const anchor = isFinite(afterMs) ? afterMs : baseStart;
    if (!isFinite(anchor)) return NaN;
    const d = new Date(anchor);
    const at = new Date(d.getFullYear(), d.getMonth(), d.getDate(), Number(m[1]), Number(m[2])).getTime();
    return at < anchor ? at + 86400000 : at;
  }
  /* 基础版节点起算时刻：前置任务最晚完成时间；无前置时取演练真实开始时间 */
  function basicStartMs(node, seen) {
    seen = seen || {};
    if (seen[node.id]) return NaN;
    seen[node.id] = true;
    let best = null;
    (node.deps || []).forEach(function (depId) {
      const dep = taskByNode[depId];
      if (!dep) return;
      const ms = taskDoneMs(dep, seen);
      if (isFinite(ms) && (best === null || ms > best)) best = ms;
    });
    if (best !== null) return best;
    return isFinite(baseStart) ? baseStart : null;
  }
  function taskDoneMs(t, seen) {
    if (!t || t.status !== '已完成' || !t.completed_at) return NaN;
    if (doneCache[t.id] !== undefined) return doneCache[t.id];
    let node = t.node_id ? correct.filter(function (n) { return n.id === t.node_id; })[0] : null;
    if (!node) node = matchNode({ title: t.title, descr: t.descr, roleId: t.role_id }, correct).node;
    const after = node ? basicStartMs(node, seen) : baseStart;
    const ms = resolveStamp(t.completed_at, after);
    doneCache[t.id] = ms;
    return ms;
  }

  /* 已完成流程：基础版取已完成任务，实战版取已提交且判定为正确流程的记录 */
  const hitMap = {};
  if (mode === '基础版') {
    tasks.forEach(function (t) {
      if (t.status !== '已完成') return;
      let node = t.node_id ? correct.filter(function (n) { return n.id === t.node_id; })[0] : null;
      if (!node) node = matchNode({ title: t.title, descr: t.descr, roleId: t.role_id }, correct).node;
      if (!node || hitMap[node.id]) return;
      const done = taskDoneMs(t);
      const start = basicStartMs(node);
      const duration = (isFinite(done) && start !== null && done >= start)
        ? Math.round((done - start) / 60000) : null;
      const stamp = String(t.completed_at || '').trim();
      hitMap[node.id] = {
        stepTime: stamp ? (/^\d{4}/.test(stamp) ? stamp.slice(11, 16) : stamp.slice(0, 5)) : null,
        late: duration !== null && Number(node.timeout) > 0 ? duration > Number(node.timeout) : false,
        duration: duration
      };
    });
  } else {
    steps.forEach(function (s) {
      if (!s.matched_node || s.correct === 0 || s.skipped === 1) return;
      const node = correct.filter(function (n) { return n.id === s.matched_node; })[0];
      if (!node || hitMap[node.id]) return;
      hitMap[node.id] = { stepTime: s.step_time, late: Number(s.ontime) === 0, step: s };
    });
  }
  /* 被选中的混淆流程 */
  const wrongPicked = {};
  if (mode !== '基础版') {
    steps.forEach(function (s) {
      if (!s.matched_node || s.correct !== 0 || s.skipped === 1) return;
      if (distractors.some(function (n) { return n.id === s.matched_node; })) wrongPicked[s.matched_node] = s;
    });
  }
  /* 参演人员判定「不执行」跳过的流程 */
  const skipMap = {};
  if (mode !== '基础版') {
    steps.forEach(function (s) {
      if (!s.matched_node || s.skipped !== 1) return;
      skipMap[s.matched_node] = s;
    });
  }

  const nodeResults = [];
  const deductions = [];
  const bonuses = [];
  let earned = 0;
  let skippedCorrect = 0, skippedDistract = 0;
  correct.forEach(function (n) {
    const sc = n.score;
    const hit = hitMap[n.id];
    const skip = skipMap[n.id];
    if (!hit && skip) {
      skippedCorrect++;
      const missed = ended;
      if (missed) {
        earned -= sc;
        deductions.push({ nodeId: n.id, title: n.title, type: '正确流程未完成', point: -sc, skipped: true });
      }
      nodeResults.push({
        id: n.id, title: n.title, role: n.target,
        plan: planText(n),
        actual: missed ? '已跳过（演练结束，按未完成计）' : '已跳过（未执行）',
        required: true, submitted: false, hit: false, ontime: false, wrong: false, skipped: true,
        score: sc, earned: 0, late: false, state: missed ? '遗漏' : '已跳过'
      });
      return;
    }
    if (hit) {
      /* 超时完成扣本流程 10%；下一节点接收人选择错误再扣本流程 10% */
      const recvBad = !!(hit.step && hit.step.receiver_ok === 0);
      const got = Math.round(sc * (hit.late ? 0.9 : 1) * (recvBad ? 0.9 : 1) * 10) / 10;
      earned += got;
      if (hit.late) deductions.push({ nodeId: n.id, title: n.title, type: '超时完成（扣10%）', point: -(Math.round(sc * 0.1 * 10) / 10) });
      if (recvBad) deductions.push({ nodeId: n.id, title: n.title, type: '接收人选择错误（扣10%）', point: -(Math.round(sc * 0.1 * 10) / 10) });
      nodeResults.push({
        id: n.id, title: n.title, role: n.target,
        plan: planText(n),
        actual: hit.stepTime
          ? ('完成于 ' + hit.stepTime + (hit.duration !== null && hit.duration !== undefined ? '（用时 ' + hit.duration + ' 分钟）' : ''))
          : '已完成',
        required: true, submitted: true, hit: true, ontime: !hit.late, wrong: false,
        score: sc, earned: got, late: !!hit.late, receiver: (hit.step && hit.step.receiver) || '', receiverOk: !recvBad,
        state: recvBad ? (hit.late ? '超时且接收人错误' : '接收人错误') : (hit.late ? '超时完成' : '已完成')
      });
      return;
    }
    const missed = ended;
    if (missed) {
      earned -= sc;
      deductions.push({ nodeId: n.id, title: n.title, type: '正确流程未完成', point: -sc });
    }
    nodeResults.push({
      id: n.id, title: n.title, role: n.target,
      plan: planText(n),
      actual: missed ? '未完成（演练结束）' : '待处置',
      required: true, submitted: false, hit: false, ontime: false, wrong: false,
      score: sc, earned: 0, late: false, state: missed ? '遗漏' : '待处置'
    });
  });
  Object.keys(wrongPicked).forEach(function (nid) {
    const n = distractors.filter(function (x) { return x.id === nid; })[0];
    const s = wrongPicked[nid];
    earned -= n.score;
    deductions.push({ nodeId: n.id, title: n.title, type: '选择混淆流程', point: -n.score, distract: true });
    nodeResults.push({
      id: n.id, title: n.title, role: n.target, plan: planText(n),
      actual: '已执行（' + (s.step_time || '—') + '）', required: false, submitted: true,
      hit: false, ontime: false, wrong: true, distract: true, score: 0, earned: -n.score,
      late: false, state: '决策偏离'
    });
  });

  /* 跳过混淆流程：判定不执行可获得少量奖励分 */
  Object.keys(skipMap).forEach(function (nid) {
    const n = distractors.filter(function (x) { return x.id === nid; })[0];
    if (!n) return;
    skippedDistract++;
    earned += SKIP_BONUS;
    bonuses.push({ nodeId: n.id, title: n.title, type: '跳过混淆流程（+' + SKIP_BONUS + '）', point: SKIP_BONUS, bonus: true });
    nodeResults.push({
      id: n.id, title: n.title, role: n.target, plan: planText(n),
      actual: '已跳过（未执行）', required: false, submitted: false,
      hit: false, ontime: false, wrong: false, skipped: true, score: 0, earned: SKIP_BONUS,
      late: false, state: '已跳过'
    });
  });

  const total = correct.length;
  const hitCnt = nodeResults.filter(function (x) { return x.required && x.hit; }).length;
  const ontimeCnt = nodeResults.filter(function (x) { return x.required && x.hit && x.ontime; }).length;
  const wrongCnt = nodeResults.filter(function (x) { return x.distract; }).length;
  const lateCnt = nodeResults.filter(function (x) { return x.required && x.hit && !x.ontime; }).length;
  const missedCnt = nodeResults.filter(function (x) { return x.required && !x.submitted && ended; }).length;
  const pendingCnt = nodeResults.filter(function (x) { return x.required && !x.submitted && !x.skipped && !ended; }).length;
  const maxScore = correct.reduce(function (a, n) { return a + n.score; }, 0);
  const sc = Math.max(0, Math.min(100, Math.round(earned * 10) / 10));
  const completeness = total ? Math.round(hitCnt / total * 1000) / 10 : 0;
  const timeRate = total ? Math.round(ontimeCnt / total * 1000) / 10 : 0;
  return {
    projectId: projectId, mode: mode, scenarioId: p.scenario_id, ended: ended,
    total: total, hit: hitCnt, ontime: ontimeCnt, missed: missedCnt, pending: pendingCnt,
    wrong: wrongCnt, late: lateCnt, maxScore: maxScore, distractors: distractors.length,
    skipped: skippedCorrect + skippedDistract, skippedCorrect: skippedCorrect, skippedDistract: skippedDistract,
    completeness: completeness, timeRate: timeRate,
    score: sc, grade: gradeOf(sc), deductions: deductions, bonuses: bonuses,
    formula: (mode === '实战版' ? '实战版' : '基础版')
      + '：正确流程满分 ' + maxScore + ' 分'
      + (hitCnt ? '，已完成 ' + hitCnt + '/' + total + ' 项（超时按 90% 计）' : '')
      + (skippedDistract ? '，跳过混淆流程 ' + skippedDistract + ' 项 +' + (skippedDistract * SKIP_BONUS) + ' 分' : '')
      + (skippedCorrect ? '，跳过正确流程 ' + skippedCorrect + ' 项' : '')
      + (wrongCnt ? '，误选混淆流程 ' + wrongCnt + ' 项扣 ' + deductions.filter(function (d) { return d.distract; }).reduce(function (a, d) { return a - d.point; }, 0) + ' 分' : '')
      + (missedCnt ? '，未完成 ' + missedCnt + ' 项扣 ' + deductions.filter(function (d) { return d.type === '正确流程未完成'; }).reduce(function (a, d) { return a - d.point; }, 0) + ' 分' : '')
      + ' = ' + sc + ' 分',
    nodes: nodeResults, steps: steps, correctTotal: maxScore
  };
}

function gradeOf(v) {
  return v >= 90 ? '优秀' : v >= 80 ? '良好' : v >= 70 ? '合格' : '不合格';
}

/* 实战版已完成处置的节点集合：提交或跳过均视为该前置已处理，避免流程永久锁死 */
function practicalDone(projectId) {
  const set = {};
  db.prepare('SELECT matched_node FROM drill_steps WHERE project_id=?').all(projectId).forEach(function (s) {
    if (s.matched_node) set[s.matched_node] = true;
  });
  return set;
}

/* 正确流程的前置依赖是否全部完成；混淆流程不参与依赖图，始终可执行 */
function nodeLock(node, byId, done) {
  if (!node || node.flowType === 'distractor') return { locked: false, blockedBy: [] };
  const pending = (node.deps || []).filter(function (id) { return byId[id] && !done[id]; });
  return {
    locked: pending.length > 0,
    blockedBy: pending.map(function (id) { return { id: id, title: byId[id].title }; })
  };
}

/* 实战版：本角色可执行的全部处置流程（正确流程与混淆流程外观一致，不暴露属性、分值、引导提示）
 * 与剧本基线相比多出的流程即为混淆流程，选中后按该流程分值扣分 */
function flowList(p, role) {
  const nodes = nodesOf(p.scenario_id).filter(function (n) { return n.type !== 'end'; });
  const byId = {};
  nodes.forEach(function (n) { byId[n.id] = n; });
  const done = practicalDone(p.id);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=?').all(p.id);
  const list = nodes.filter(function (n) { return canHandle(role, n.target); }).map(function (n) {
    const st = steps.filter(function (s) { return s.matched_node === n.id; })[0] || null;
    const f = formFields(n.form);
    const lock = nodeLock(n, byId, done);
    return {
      nodeId: n.id, timeout: n.timeout, type: n.type, action: n.action, target: n.target,
      title: n.title, desc: n.desc, formId: n.form || '',
      formName: f.name || '', fields: f.fields || [],
      locked: lock.locked, blockedBy: lock.blockedBy,
      submitted: st ? {
        stepTime: st.step_time, user: st.user, role: st.role_id,
        correct: st.correct === 0 ? 0 : 1, ontime: !!st.ontime, foundAt: st.found_at,
        found: st.found, measure: st.measure, result: st.result, evidence: st.evidence,
        forms: parseJson(st.forms, {}), delta: Number(st.score || 0),
        receiver: st.receiver || '', receiverOk: st.receiver_ok === 0 ? 0 : 1,
        skipped: st.skipped === 1 ? 1 : 0
      } : null
    };
  });
  /* 按节点 ID 哈希稳定混杂，避免混淆流程集中在一处 */
  const hash = function (s) {
    let h = 0;
    for (let i = 0; i < String(s).length; i++) h = (h * 31 + String(s).charCodeAt(i)) >>> 0;
    return h;
  };
  return list.sort(function (a, b) {
    return hash(a.nodeId) - hash(b.nodeId);
  });
}

/* 当前项目模式 + 基线 + 步骤 + 评分（工作台/复盘页共用） */
router.get('/workbench/:projectId', auth.requireAuth, function (req, res) {
  const p = projectOf(req.params.projectId);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  res.json({
    projectId: p.id, mode: p.mode || '基础版', scenarioId: p.scenario_id,
    nodes: nodesOf(p.scenario_id).filter(function (n) { return n.flowType !== 'distractor'; }),
    steps: db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(p.id),
    evidence: evidenceSummary(p),
    score: score(p.id)
  });
});

/* 参演人员打开处置表单即视为「处置中」，仅记录状态供导调中心展示 */
router.post('/nodes/:nodeId/start', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位仅可查看医院上报材料（分级权限隔离）' });
  const pid = activeProjectId();
  const p = pid ? projectOf(pid) : null;
  if (!p) return res.status(404).json({ error: '当前没有进行中的演练项目' });
  if (p.status !== '演练中') return res.status(400).json({ error: '演练未开始或已结束' });
  const node = db.prepare('SELECT * FROM script_nodes WHERE id=? AND scenario_id=?').get(req.params.nodeId, p.scenario_id);
  if (!node) return res.status(404).json({ error: '演练节点不存在' });
  const mode = p.mode || '基础版';
  if (mode === '实战版' && !canHandle(req.user.role, node.target)) {
    return res.status(403).json({ error: '该环节不属于当前角色的处置范围' });
  }
  if (mode === '基础版') {
    const own = db.prepare('SELECT id FROM tasks WHERE project_id=? AND node_id=? AND role_id=?')
      .get(p.id, node.id, req.user.role.id);
    if (!own && !canHandle(req.user.role, node.target)) {
      return res.status(403).json({ error: '该环节不属于当前角色的处置范围' });
    }
  }
  db.prepare('INSERT INTO drill_node_state (project_id,node_id,role_id,user,started_at) VALUES (?,?,?,?,?) ' +
    'ON CONFLICT(project_id,node_id) DO UPDATE SET role_id=excluded.role_id, user=excluded.user, started_at=excluded.started_at')
    .run(p.id, node.id, req.user.role.id, req.user.name, now().slice(0, 16));
  res.json({ ok: true, projectId: p.id, nodeId: node.id });
});

function activeProjectId() {
  const r = db.prepare("SELECT value FROM settings WHERE key='active_project'").get();
  return r ? r.value : null;
}

/* 实战版处置流程列表（角色工作台：正确流程 + 混淆流程混合展示） */
router.get('/flows/:projectId', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位仅可查看医院上报材料（分级权限隔离）' });
  const p = projectOf(req.params.projectId);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if ((p.mode || '基础版') !== '实战版') return res.status(400).json({ error: '当前项目不是实战版' });
  const flows = flowList(p, req.user.role);
  res.json({
    projectId: p.id, mode: '实战版', projectStatus: p.status,
    flowCount: flows.length,
    flows: flows,
    /* 下一节点接收人候选角色（实战版需参演人员自行选择，选错扣本流程10%分值） */
    receivers: receiverOptions(),
    evidence: evidenceSummary(p),
    score: score(p.id)
  });
});

/* 实战版：角色工作台仅展示已发现的系统故障 */
router.get('/faults/:projectId', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位仅可查看医院上报材料（分级权限隔离）' });
  const p = projectOf(req.params.projectId);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if ((p.mode || '基础版') !== '实战版') return res.status(400).json({ error: '当前项目不是实战版' });
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=?').all(p.id);
  /* 剧本基线中的正确流程即为“已通报故障”，混淆流程不计入故障列表 */
  const faults = nodesOf(p.scenario_id).filter(function (n) {
    return n.type !== 'end' && n.flowType !== 'distractor';
  }).map(function (n) {
    const st = steps.filter(function (s) { return s.matched_node === n.id; })[0] || null;
    let status = '待处置';
    if (st) status = st.skipped === 1 ? '已跳过（未执行）' : (st.correct === 0 ? '处置偏差' : (st.ontime ? '已控制' : '已控制（超时）'));
    return {
      nodeId: n.id, title: n.title, symptom: n.desc,
      status: status, handled: !!(st && st.skipped !== 1), handledBy: st ? st.user : null,
      stepTime: st ? st.step_time : null, correct: st && st.skipped !== 1 ? (st.correct === 0 ? 0 : 1) : null,
      ontime: st ? !!st.ontime : null
    };
  });
  res.json({ projectId: p.id, mode: '实战版', projectStatus: p.status, count: faults.length, faults: faults, score: score(p.id) });
});

/* 实战版：判定当前流程「不执行」并跳过
 * 混淆流程被正确跳过可获得少量奖励分；正确流程被跳过，结束演练时按该流程分值扣分 */
router.post('/skip', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不参与处置记录（分级权限隔离）' });
  const b = req.body || {};
  const p = projectOf(String(b.projectId || ''));
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if ((p.mode || '基础版') !== '实战版') return res.status(400).json({ error: '当前项目为基础版，请在角色工作台按正确流程逐步执行' });
  if (p.status !== '演练中') return res.status(400).json({ error: '演练' + p.status + '，暂不能跳过流程' });
  const all = nodesOf(p.scenario_id).filter(function (n) { return n.type !== 'end'; });
  const node = all.filter(function (n) { return n.id === String(b.nodeId || ''); })[0];
  if (!node) return res.status(404).json({ error: '处置流程不存在' });
  if (!canHandle(req.user.role, node.target)) {
    return res.status(403).json({ error: '该流程责任角色为「' + node.target + '」，当前角色无权处置' });
  }
  const byId = {};
  all.forEach(function (x) { byId[x.id] = x; });
  const lock = nodeLock(node, byId, practicalDone(p.id));
  if (lock.locked) {
    return res.status(400).json({
      error: '前置流程未完成：' + lock.blockedBy.map(function (x) { return x.title; }).join('、'),
      blockedBy: lock.blockedBy
    });
  }
  const dup = db.prepare('SELECT id,user FROM drill_steps WHERE project_id=? AND matched_node=?').get(p.id, node.id);
  if (dup) return res.status(409).json({ error: '该流程已由 ' + dup.user + ' 处理，不可重复跳过' });

  const isCorrect = node.flowType !== 'distractor';
  /* 不向前端下发流程属性，仅返回本次跳过带来的分值变化与最新得分 */
  const delta = isCorrect ? 0 : SKIP_BONUS;
  const stepTime = hhmm();
  db.prepare('INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,choice,correct,found_at,measure,result,forms,flow_type,score,receiver,receiver_ok,skipped,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(p.id, p.scenario_id, req.user.role.id, req.user.name, node.action || '', node.title,
      '参演人员判定该流程不执行（跳过）', stepTime, node.id, 1, 0, node.title, isCorrect ? 1 : 0,
      now().slice(0, 16), '参演人员判定不执行，跳过该流程', '已跳过（未执行）', '', node.flowType, delta, '', 1, 1, nowSec());
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(p.id, stepTime, req.user.name, req.user.role.id, '跳过处置流程',
      '流程「' + node.title + '」由参演人员判定不执行（跳过）', '', null);
  auth.audit(req, '实战版跳过流程 ' + node.id + '：' + node.title);
  const sc = score(p.id);
  /* 站内消息：节点跳过（模板驱动） */
  notify.push({
    event: 'node_skip',
    vars: {
      title: node.title,
      project: p.name,
      user: req.user.name,
      reason: isCorrect ? '正确流程跳过，结束演练时将按该流程分值扣分' : ('跳过混淆流程，奖励 +' + delta + ' 分'),
      score: sc.score
    },
    level: isCorrect ? 'P1' : 'P2'
  });
  res.json({ ok: true, nodeId: node.id, skipped: true, stepTime: stepTime, delta: delta, score: sc });
});

/* 实战版：提交某个流程的实际处置情况（正确流程得分，混淆流程按分值扣分） */
router.post('/decide', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不参与处置记录（分级权限隔离）' });
  const b = req.body || {};
  const p = projectOf(String(b.projectId || ''));
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if ((p.mode || '基础版') !== '实战版') return res.status(400).json({ error: '当前项目为基础版，请在角色工作台按正确流程逐步执行' });
  if (p.status !== '演练中') return res.status(400).json({ error: '演练' + p.status + '，暂不能提交处置记录' });
  const all = nodesOf(p.scenario_id).filter(function (n) { return n.type !== 'end'; });
  const node = all.filter(function (n) { return n.id === String(b.nodeId || ''); })[0];
  if (!node) return res.status(404).json({ error: '处置流程不存在' });
  if (!canHandle(req.user.role, node.target)) {
    return res.status(403).json({ error: '该流程责任角色为「' + node.target + '」，当前角色无权处置' });
  }
  const byId = {};
  all.forEach(function (x) { byId[x.id] = x; });
  const lock = nodeLock(node, byId, practicalDone(p.id));
  if (lock.locked) {
    return res.status(400).json({
      error: '前置流程未完成：' + lock.blockedBy.map(function (x) { return x.title; }).join('、'),
      blockedBy: lock.blockedBy
    });
  }
  const dup = db.prepare('SELECT id,user FROM drill_steps WHERE project_id=? AND matched_node=?').get(p.id, node.id);
  if (dup) return res.status(409).json({ error: '该流程已由 ' + dup.user + ' 提交处置记录，不可重复执行' });

  const measure = String(b.measure || '').trim().slice(0, 1000);
  const result = String(b.result || '').trim().slice(0, 500);
  const evidence = String(b.evidence || '').trim().slice(0, 300);
  const found = String(b.found || '').trim().slice(0, 1000);
  const forms = (b.forms && typeof b.forms === 'object') ? b.forms : {};
  if (measure.length < 6) return res.status(400).json({ error: '请填写不少于 6 字的实际处置措施' });
  if (!result) return res.status(400).json({ error: '请填写处置结果' });
  const f = formFields(node.form);
  const missing = (f.fields || []).filter(function (fl) {
    const label = fl.label || fl.name || '';
    return label && fl.required !== false && !String(forms[label] || '').trim();
  });
  if (missing.length) return res.status(400).json({ error: '请完整填写表单：' + missing.map(function (x) { return x.label || x.name; }).join('、') });

  const stepTime = hhmm();
  const ontime = !isLate(p, node);
  const isCorrect = node.flowType !== 'distractor';
  /* 下一节点接收人：实战版由参演人员自行选择，支持多选；集合完全一致才正确，否则扣本流程 10% 分值 */
  const wantRecv = parseReceivers(node.receiver);
  const recv = parseReceivers(b.receiver);
  if (isCorrect && wantRecv.length && !recv.length) {
    return res.status(400).json({ error: '请选择下一节点接收人（可多选）' });
  }
  const receiver = recv.join('、');
  const receiverOk = !wantRecv.length ? 1 : (sameReceivers(wantRecv, recv) ? 1 : 0);
  const delta = !isCorrect
    ? -node.score
    : Math.round(node.score * (ontime ? 1 : 0.9) * (receiverOk ? 1 : 0.9) * 10) / 10;
  const formText = Object.keys(forms).map(function (k) { return k + '：' + String(forms[k]).slice(0, 80); }).join('；');
  const descr = (formText ? formText + '；' : '') + '措施：' + measure + '；结果：' + result + (found ? '；发现：' + found : '');
  const r = db.prepare('INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,choice,correct,found_at,found,measure,result,evidence,forms,flow_type,score,receiver,receiver_ok,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(p.id, p.scenario_id, req.user.role.id, req.user.name, node.action || '', node.title, descr.slice(0, 2000), stepTime,
      node.id, 1, ontime ? 1 : 0, node.title, isCorrect ? 1 : 0, now().slice(0, 16), found, measure, result, evidence,
      JSON.stringify(forms), node.flowType, delta, receiver, receiverOk, nowSec());

  /* 自动归档处置记录（模拟取证材料） */
  const evId = 'E' + String(Date.now()).slice(-8);
  const evName = '实战处置记录-' + node.id + '-' + req.user.name + (evidence ? '（' + evidence + '）' : '');
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(evId, '处置记录', evName.slice(0, 180), '表单', stepTime, req.user.role.id, req.user.name, req.user.id, 0, null, p.id, null, null, nowSec());
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(p.id, stepTime, req.user.name, req.user.role.id, isCorrect ? '执行正确处置流程' : '执行混淆处置流程',
      '流程「' + node.title + '」；' + (isCorrect ? '得分 +' + delta : '扣分 ' + delta)
        + (isCorrect && !receiverOk ? '；下一节点接收人选择错误（扣10%）' : '')
        + '；措施：' + measure.slice(0, 60),
      isCorrect && receiverOk ? '' : 'P1', null);
  auth.audit(req, '实战版处置流程 ' + node.id + '：' + (isCorrect ? '正确流程 +' + delta : '混淆流程 ' + delta) + ' 分'
    + (receiverOk ? '' : '（接收人选择错误）'));

  const sc = score(p.id);
  /* 站内消息：节点处置结果（模板驱动） */
  notify.push({
    event: 'node_submit',
    vars: {
      title: node.title,
      project: p.name,
      user: req.user.name,
      correct: (isCorrect ? ('正确流程，本节点得分 +' + delta) : ('混淆流程，扣分 ' + delta))
        + (isCorrect && !receiverOk ? '（接收人选择错误，扣10%）' : '')
        + '；当前累计得分 ' + sc.score + ' 分'
    },
    level: isCorrect && receiverOk ? 'P2' : 'P1'
  });

  res.json({
    ok: true, id: Number(r.lastInsertRowid), nodeId: node.id, stepTime: stepTime,
    correct: isCorrect ? 1 : 0, ontime: ontime, delta: delta,
    receiver: receiver, receiverOk: receiverOk,
    score: sc
  });
});

/* 演练结束：未完成的正确流程按该流程分值扣分 */
router.post('/finish', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不可结束演练（分级权限隔离）' });
  const rid = req.user.role.id;
  if (['commander', 'platformadmin', 'itlead'].indexOf(rid) < 0) {
    return res.status(403).json({ error: '仅院应急指挥部、信息科负责人、平台管理员可结束演练' });
  }
  const p = projectOf(String((req.body && req.body.projectId) || ''));
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if (p.status !== '演练中') return res.status(400).json({ error: '演练' + p.status + '，无需重复结束' });
  /* 完成演练前置校验：取证材料必须全部归档，未完成不允许结束 */
  const ev = evidenceSummary(p);
  if (ev.pending > 0) {
    return res.status(400).json({
      error: '尚有 ' + ev.pending + ' 项取证材料未归档（已归档 ' + ev.done + '/' + ev.total + '），请完成取证后再结束演练',
      pending: ev.pending, done: ev.done, total: ev.total, pendingItems: ev.pendingItems.slice(0, 10)
    });
  }
  const s = score(p.id);
  db.prepare("UPDATE projects SET status='已结束' WHERE id=?").run(p.id);
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(p.id, hhmm(), req.user.name, rid, '演练结束',
      '结束演练：已完成 ' + (s ? s.hit : 0) + '/' + (s ? s.total : 0) + ' 项正确流程，最终得分 ' + (s ? s.score : 0), 'P1', null);
  auth.audit(req, '结束演练 ' + p.id + '，综合得分 ' + (s ? s.score : 0));
  const finalScore = score(p.id);
  /* 站内消息：演练结束综合得分（模板驱动） */
  notify.push({
    event: 'drill_end_score',
    vars: {
      project: p.name,
      mode: p.mode || '基础版',
      score: finalScore.score,
      hit: finalScore.hit,
      total: finalScore.total,
      grade: finalScore.grade
    }
  });
  res.json({ ok: true, projectId: p.id, score: finalScore });
});

/* 历史兼容：旧版自主添加步骤（当前工作台已改为决策菜单，不再展示该入口） */
router.post('/steps', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不参与处置步骤录入（分级权限隔离）' });
  const b = req.body || {};
  const p = projectOf(String(b.projectId || ''));
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if ((p.mode || '基础版') !== '实战版') return res.status(400).json({ error: '当前项目为基础版（诱导演练），不支持自主添加步骤' });
  const title = String(b.title || '').trim();
  if (!title) return res.status(400).json({ error: '请填写处置步骤标题' });
  const action = String(b.action || '其他');
  const descr = String(b.descr || b.desc || '').slice(0, 2000);
  const stepTime = /^\d{2}:\d{2}$/.test(String(b.stepTime || '')) ? b.stepTime : hhmm();
  const nodes = nodesOf(p.scenario_id);
  const m = matchNode({ title: title, descr: descr, action: action, roleId: req.user.role.id }, nodes);
  const ontime = !m.node ? false : !isLate(p, m.node);
  const r = db.prepare('INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(p.id, p.scenario_id, req.user.role.id, req.user.name, action, title, descr, stepTime,
      m.node ? m.node.id : null, m.score >= 2 ? 1 : 0, ontime ? 1 : 0, now());
  auth.audit(req, '新增处置步骤《' + title + '》' + (m.node ? '（匹配节点 ' + m.node.id + '）' : '（未匹配基线节点）'));
  res.json({
    ok: true, id: Number(r.lastInsertRowid), matchedNode: m.node ? m.node.id : null,
    matchedTitle: m.node ? m.node.title : null, ontime: ontime, stepTime: stepTime
  });
});

router.delete('/steps/:id', auth.requireAuth, function (req, res) {
  const s = db.prepare('SELECT * FROM drill_steps WHERE id=?').get(Number(req.params.id));
  if (!s) return res.status(404).json({ error: '步骤不存在' });
  if (s.role_id !== req.user.role.id && ['platformadmin', 'commander'].indexOf(req.user.role.id) < 0) {
    return res.status(403).json({ error: '仅本人或指挥部/平台管理员可删除该步骤' });
  }
  db.prepare('DELETE FROM drill_steps WHERE id=?').run(s.id);
  auth.audit(req, '删除处置步骤《' + s.title + '》');
  res.json({ ok: true });
});

/* 重新自动考评：计算并把主评分写入 scores（供报告与台账引用） */
router.post('/score/:projectId/recalc', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (['commander', 'platformadmin', 'itlead'].indexOf(rid) < 0) return res.status(403).json({ error: '无考评权限' });
  const s = score(req.params.projectId);
  if (!s) return res.status(404).json({ error: '演练项目不存在' });
  db.prepare('INSERT OR REPLACE INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,?,?)')
    .run('MODE', s.projectId, s.score, 100, s.formula, 1, now());
  auth.audit(req, '重新自动考评 ' + s.projectId + '（' + s.mode + '）：' + s.formula);
  res.json(s);
});

module.exports = router;
