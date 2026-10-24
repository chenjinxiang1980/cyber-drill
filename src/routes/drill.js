/* 演练模式与自动考评：基础版（诱导演练，仅时限打分）/ 实战版（自主添加步骤，遗漏+时限评分）
 * 基线：script_nodes 中 scenario_id 对应的标准处置节点（required=1 为必经步骤，keywords 用于匹配）
 */
'use strict';
const express = require('express');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { now, hhmm } = require('../util');

const router = express.Router();

const MODES = ['基础版', '实战版'];

function mins(t) {
  const m = /^(\d{1,2}):(\d{2})/.exec(String(t || ''));
  return m ? Number(m[1]) * 60 + Number(m[2]) : null;
}

function nodesOf(scenarioId) {
  return db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(scenarioId).map(function (n) {
    return {
      id: n.id, type: n.type, title: n.title, target: n.target, time: n.time, timeout: n.timeout || 0,
      desc: n.descr || '', guide: n.guide || '', keywords: (n.keywords || '').split(',').filter(Boolean),
      action: n.action || '', required: !!n.required
    };
  });
}

function projectOf(projectId) {
  return db.prepare('SELECT * FROM projects WHERE id=?').get(projectId);
}

/* 步骤匹配基线节点：仅匹配必经节点（动作类型优先，关键词次之），避免误匹配"终止/复盘"等非必经节点 */
function matchNode(step, nodes) {
  const pool = nodes.filter(function (n) { return n.required && n.type !== 'end'; });
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

/* 评分：
 * 基础版 = 逐节点时限达成率（按时完成任务节点数 / 应完成节点总数 × 100）
 * 实战版 = 步骤遗漏 70% + 时限 30%（遗漏按必须节点未匹配计，时限按已匹配节点是否在 time+timeout 内）
 */
function score(projectId) {
  const p = projectOf(projectId);
  if (!p) return null;
  const nodes = nodesOf(p.scenario_id);
  const required = nodes.filter(function (n) { return n.required && n.type !== 'end'; });
  const mode = p.mode || '基础版';
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(projectId);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(projectId)
    .map(function (s) { return Object.assign({}, s); });

  const nodeResults = [];
  if (mode === '基础版') {
    /* 基础版以系统下发的任务节点为考评单元：只统计操作时限（接单→完成耗时 vs 节点时限） */
    tasks.forEach(function (t) {
      const m = matchNode({ title: t.title, descr: t.descr, roleId: t.role_id }, nodes);
      const limit = (m.node && m.node.timeout) || 30;
      const a = mins(t.accepted_at), d = mins(t.completed_at);
      const known = a !== null && d !== null && d >= a;
      const duration = known ? d - a : null;
      const done = t.status === '已完成';
      /* 历史已完成任务可能没有接单/完成时刻，无证据表明超时，按按时处理 */
      const ontime = done && (known ? duration <= limit : true);
      nodeResults.push({
        id: t.id, title: t.title, role: t.role_id,
        plan: '时限 ' + limit + ' 分钟' + (m.node ? '（基线 ' + m.node.id + '）' : ''),
        actual: done ? (known ? '耗时 ' + duration + ' 分钟' : '已完成（未记录耗时）') : t.status,
        required: true, hit: done, ontime: ontime,
        state: !done ? t.status : (ontime ? '按时' : '超时')
      });
    });
    const total = nodeResults.length;
    const ontimeCnt = nodeResults.filter(function (x) { return x.ontime; }).length;
    const doneCnt = nodeResults.filter(function (x) { return x.hit; }).length;
    const rate = total ? Math.round(ontimeCnt / total * 1000) / 10 : 0;
    return {
      projectId: projectId, mode: mode, scenarioId: p.scenario_id,
      total: total, hit: doneCnt, ontime: ontimeCnt, missed: total - doneCnt,
      completeness: total ? Math.round(doneCnt / total * 1000) / 10 : 0,
      timeRate: rate, score: Math.round(rate), grade: gradeOf(rate),
      formula: '基础版：按时完成节点 ' + ontimeCnt + ' / ' + total + '，时限达成率 ' + rate + '%',
      nodes: nodeResults, steps: []
    };
  }

  /* 实战版：先按步骤匹配基线必经节点，再判遗漏与时限 */
  const covered = {};
  steps.forEach(function (s) {
    const r = matchNode({ title: s.title, descr: s.descr, action: s.action, roleId: s.role_id }, nodes);
    s.matchedNode = r.node ? r.node.id : null;
    s.matched = r.score >= 2;
  });
  steps.forEach(function (s) { if (s.matched) covered[s.matchedNode] = s; });

  required.forEach(function (n) {
    const hit = covered[n.id];
    const limit = mins(n.time) === null ? null : mins(n.time) + (n.timeout || 0);
    const actualMin = hit ? mins(hit.step_time) : null;
    const ontime = !!(hit && actualMin !== null && limit !== null && actualMin <= limit);
    nodeResults.push({
      id: n.id, title: n.title, role: n.target, plan: n.time + (n.timeout ? ' ~ +' + n.timeout + 'min' : ''),
      actual: hit ? (hit.step_time + ' ' + hit.title) : '未录入', required: true,
      hit: !!hit, ontime: ontime, state: hit ? (ontime ? '按时' : '超时') : '遗漏'
    });
  });
  const total = required.length;
  const hitCnt = nodeResults.filter(function (x) { return x.hit; }).length;
  const ontimeCnt = nodeResults.filter(function (x) { return x.ontime; }).length;
  const completeness = total ? hitCnt / total * 100 : 0;
  const timeRate = hitCnt ? ontimeCnt / hitCnt * 100 : 0;
  const raw = completeness * 0.7 + timeRate * 0.3;
  const sc = Math.round(raw * 10) / 10;
  return {
    projectId: projectId, mode: mode, scenarioId: p.scenario_id,
    total: total, hit: hitCnt, ontime: ontimeCnt, missed: total - hitCnt,
    completeness: Math.round(completeness * 10) / 10, timeRate: Math.round(timeRate * 10) / 10,
    score: sc, grade: gradeOf(sc),
    formula: '实战版：完整性 ' + Math.round(completeness * 10) / 10 + '% × 70% + 时限达成 ' + Math.round(timeRate * 10) / 10 + '% × 30% = ' + sc,
    extraSteps: steps.filter(function (s) { return !s.matched; }).length,
    nodes: nodeResults, steps: steps
  };
}

function gradeOf(v) {
  return v >= 90 ? '优秀' : v >= 80 ? '良好' : v >= 70 ? '合格' : '不合格';
}

/* 当前项目模式 + 基线 + 步骤 + 评分（工作台/复盘页共用） */
router.get('/workbench/:projectId', auth.requireAuth, function (req, res) {
  const p = projectOf(req.params.projectId);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  res.json({
    projectId: p.id, mode: p.mode || '基础版', scenarioId: p.scenario_id,
    nodes: nodesOf(p.scenario_id),
    steps: db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(p.id),
    score: score(p.id)
  });
});

/* 切换演练模式（指挥部/平台管理员/信息科负责人） */
router.patch('/mode/:projectId', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (['commander', 'platformadmin', 'itlead'].indexOf(rid) < 0) return res.status(403).json({ error: '无演练模式调整权限' });
  const mode = String((req.body && req.body.mode) || '');
  if (MODES.indexOf(mode) < 0) return res.status(400).json({ error: '演练模式仅支持：基础版 / 实战版' });
  const p = projectOf(req.params.projectId);
  if (!p) return res.status(404).json({ error: '演练项目不存在' });
  if (p.status === '已结束') return res.status(400).json({ error: '已结束的演练项目不可调整模式' });
  db.prepare('UPDATE projects SET mode=? WHERE id=?').run(mode, p.id);
  auth.audit(req, '调整演练模式 ' + p.id + ' → ' + mode);
  res.json({ ok: true, mode: mode });
});

/* 实战版：参演人员自行添加处置步骤 */
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
  const limit = m.node && mins(m.node.time) !== null ? mins(m.node.time) + (m.node.timeout || 0) : null;
  const ontime = !!(m.node && limit !== null && mins(stepTime) !== null && mins(stepTime) <= limit);
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
