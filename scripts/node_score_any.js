#!/usr/bin/env node
/**
 * 评分等价性对照脚本
 *
 * 价值：独立实现 score()（原码来自 src/routes/drill.js L162-400），
 * 作为"另一把尺子"去量 Go 端 Score()，避免自己量自己。
 *
 * 适配点（3 处）：
 *   1) DB 访问：原 require('../db') Proxy 包装 → 直接 node:sqlite DatabaseSync
 *      （原 Proxy 仅用于 reload() 无缝替换底层连接，本脚本只读不需要）
 *   2) 时钟解析：原 require('../drill_clock').parseLocal → 内联正则（drill_clock.js L4-8 原码）
 *   3) 时间函数：原 require('../util').now → 内联简单 pad 函数（util.js L4-10 原码）
 *
 * DSN 环境变量：
 *   DRILL_DB — SQLite 文件路径（默认 data/drill.db）
 *
 * 用法：
 *   DRILL_DB=scripts/_score_copy.db node scripts/node_score_any.js <projectId>
 *   不传参 → 自动跑 S01/S02/S05 三个场景
 */
'use strict';

const path = require('node:path');
const fs = require('node:fs');
const { DatabaseSync } = require('node:sqlite');

// ========== 内联工具（原码来自 src/drill_clock.js L4-8） ==========
function parseLocal(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?/.exec(String(s || ''));
  if (!m) return NaN;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] || 0)).getTime();
}

// ========== 内联工具（原码来自 src/util.js L4-10） ==========
function pad(n) { return n < 10 ? '0' + n : '' + n; }
function now() {
  const d = new Date();
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

// ========== 内联工具（原码来自 src/db.js L368-371） ==========
function parseJson(s, fallback) {
  if (s === null || s === undefined || s === '') return fallback;
  try { return JSON.parse(s); } catch (e) { return fallback; }
}

// ========== DB 连接 ==========
const DB_PATH = process.env.DRILL_DB || path.join(__dirname, '..', 'data', 'drill.db');
if (!fs.existsSync(DB_PATH)) {
  console.error('[FATAL] SQLite 文件不存在:', DB_PATH);
  process.exit(1);
}
const db = new DatabaseSync(DB_PATH);

// ========== 以下函数全部从 src/routes/drill.js 原码逐行搬 ==========

function projectOf(projectId) {
  return db.prepare('SELECT * FROM projects WHERE id=?').get(projectId);
}

function nodesOf(scenarioId) {
  return db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(scenarioId).map(function (n) {
    return {
      id: n.id, type: n.type, title: n.title, target: n.target, timeout: n.timeout || 0,
      desc: n.descr || '', guide: n.guide || '', keywords: (n.keywords || '').split(',').filter(Boolean),
      action: n.action || '', required: !!n.required, wrong: parseJson(n.wrong, []),
      form: n.form || '', flowType: n.flow_type === 'distractor' ? 'distractor' : 'correct',
      score: Number(n.score || 0) || 0, receiver: n.receiver || '', deps: parseJson(n.deps, []),
      anchorId: n.anchor_id || '',
      releaseTiming: n.release_timing || 'before'
    };
  });
}

function planText(n) {
  return n && Number(n.timeout) > 0 ? ('处置时长 ' + n.timeout + ' 分钟') : '未设置处置时长';
}

function nodeStartMs(p, node) {
  let best = null;
  (node.deps || []).forEach(function (depId) {
    const s = db.prepare('SELECT created_at FROM drill_steps WHERE project_id=? AND matched_node=? ORDER BY id LIMIT 1')
      .get(p.id, depId);
    if (!s) return;
    const ms = parseLocal(s.created_at);
    if (isFinite(ms) && (best === null || ms > best)) best = ms;
  });
  if (best !== null) return best;
  const st = parseLocal(p.started_at);
  return isFinite(st) ? st : Date.now();
}

function isLate(p, node, atMs) {
  if (!node || !(Number(node.timeout) > 0)) return false;
  const start = nodeStartMs(p, node);
  const end = (typeof atMs === 'number') ? atMs : Date.now();
  return (end - start) / 60000 > Number(node.timeout);
}

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

function sameReceivers(a, b) {
  return a.length === b.length && a.every(function (x, i) { return x === b[i]; });
}

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

function isDistractorReleased(p, node, byId, done) {
  if (!node || node.flowType !== 'distractor') return true;
  const anchorId = String(node.anchorId || '').trim();
  if (!anchorId) return true;
  const anchor = byId[anchorId];
  if (!anchor) return true;
  const timing = String(node.releaseTiming || 'before');
  if (timing === 'after') {
    return !!done[anchorId];
  }
  const anchorDeps = Array.isArray(anchor.deps) ? anchor.deps : [];
  return anchorDeps.every(function (depId) { return !byId[depId] || !!done[depId]; });
}

function gradeOf(v) {
  return v >= 90 ? '优秀' : v >= 80 ? '良好' : v >= 70 ? '合格' : '不合格';
}

/* 评分（扣分制：总分 100 起，仅对违规行为扣分） */
function score(projectId) {
  const p = projectOf(projectId);
  if (!p) return null;
  const mode = p.mode || '基础版';
  const ended = p.status === '已结束' || p.status === '已归档';
  const all = nodesOf(p.scenario_id).filter(function (n) { return n.type !== 'end'; });
  const correct = all.filter(function (n) { return n.flowType !== 'distractor'; });
  const distractors = all.filter(function (n) { return n.flowType === 'distractor'; });
  const byId = {};
  all.forEach(function (n) { byId[n.id] = n; });
  const doneSet = {};
  db.prepare('SELECT matched_node FROM drill_steps WHERE project_id=?').all(p.id).forEach(function (r) {
    if (r.matched_node) doneSet[r.matched_node] = true;
  });
  const releasedDistractors = distractors.filter(function (n) { return isDistractorReleased(p, n, byId, doneSet); });
  const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=?').all(projectId);
  const steps = db.prepare('SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id').all(projectId);

  const taskByNode = {};
  tasks.forEach(function (t) {
    let node = t.node_id ? correct.filter(function (n) { return n.id === t.node_id; })[0] : null;
    if (!node) node = matchNode({ title: t.title, descr: t.descr, roleId: t.role_id }, correct).node;
    if (node && !taskByNode[node.id]) taskByNode[node.id] = t;
  });
  const startMs = parseLocal(p.started_at);
  const baseStart = isFinite(startMs) ? startMs : parseLocal(String(p.date || now()));
  const doneCache = {};
  function resolveStamp(raw, afterMs) {
    const s = String(raw || '').trim();
    if (!s) return NaN;
    if (/^\d{4}-\d{1,2}-\d{1,2}/.test(s)) return parseLocal(s);
    const m = /^(\d{1,2}):(\d{2})/.exec(s);
    if (!m) return NaN;
    const anchor = isFinite(afterMs) ? afterMs : baseStart;
    if (!isFinite(anchor)) return NaN;
    const d = new Date(anchor);
    const at = new Date(d.getFullYear(), d.getMonth(), d.getDate(), Number(m[1]), Number(m[2])).getTime();
    return at < anchor ? at + 86400000 : at;
  }
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
  const wrongPicked = {};
  if (mode !== '基础版') {
    steps.forEach(function (s) {
      if (!s.matched_node || s.correct !== 0 || s.skipped === 1) return;
      if (releasedDistractors.some(function (n) { return n.id === s.matched_node; })) wrongPicked[s.matched_node] = s;
    });
  }
  const skipMap = {};
  if (mode !== '基础版') {
    steps.forEach(function (s) {
      if (!s.matched_node || s.skipped !== 1) return;
      skipMap[s.matched_node] = s;
    });
  }

  /* ---------- 扣分制计算起点：100 分，逐项扣除 ---------- */
  const DEDUCT_BASE = 100;
  const nodeResults = [];
  const deductions = [];
  let earned = DEDUCT_BASE;
  let skippedCorrect = 0, skippedDistract = 0;

  correct.forEach(function (n) {
    const sc = n.score;
    const hit = hitMap[n.id];
    const skip = skipMap[n.id];
    if (!hit && skip) {
      skippedCorrect++;
      const missed = ended;
      let deduct = 0;
      if (missed) {
        deduct = sc;
        earned -= sc;
        deductions.push({ nodeId: n.id, title: n.title, type: '正确流程跳过（演练结束扣全分）', point: -sc, skipped: true });
      }
      nodeResults.push({
        id: n.id, title: n.title, role: n.target,
        plan: planText(n),
        actual: missed ? '已跳过（演练结束，按未完成计）' : '已跳过（未执行）',
        required: true, submitted: false, hit: false, ontime: false, wrong: false, skipped: true,
        score: sc, earned: -deduct, late: false, state: missed ? '遗漏' : '已跳过'
      });
      return;
    }
    if (hit) {
      const recvBad = !!(hit.step && hit.step.receiver_ok === 0);
      let deduct = 0;
      if (hit.late) {
        const d = Math.round(sc * 0.1 * 10) / 10;
        deduct += d;
        deductions.push({ nodeId: n.id, title: n.title, type: '超时完成（扣10%）', point: -d });
      }
      if (recvBad) {
        const d = Math.round(sc * 0.1 * 10) / 10;
        deduct += d;
        deductions.push({ nodeId: n.id, title: n.title, type: '接收人选择错误（扣10%）', point: -d });
      }
      earned -= deduct;
      nodeResults.push({
        id: n.id, title: n.title, role: n.target,
        plan: planText(n),
        actual: hit.stepTime
          ? ('完成于 ' + hit.stepTime + (hit.duration !== null && hit.duration !== undefined ? '（用时 ' + hit.duration + ' 分钟）' : ''))
          : '已完成',
        required: true, submitted: true, hit: true, ontime: !hit.late, wrong: false,
        score: sc, earned: -deduct, late: !!hit.late, receiver: (hit.step && hit.step.receiver) || '', receiverOk: !recvBad,
        state: recvBad ? (hit.late ? '超时且接收人错误' : '接收人错误') : (hit.late ? '超时完成' : '已完成')
      });
      return;
    }
    const missed = ended;
    let deduct = 0;
    if (missed) {
      deduct = sc;
      earned -= sc;
      deductions.push({ nodeId: n.id, title: n.title, type: '正确流程未完成（扣全分）', point: -sc });
    }
    nodeResults.push({
      id: n.id, title: n.title, role: n.target,
      plan: planText(n),
      actual: missed ? '未完成（演练结束）' : '待处置',
      required: true, submitted: false, hit: false, ontime: false, wrong: false,
      score: sc, earned: -deduct, late: false, state: missed ? '遗漏' : '待处置'
    });
  });

  Object.keys(wrongPicked).forEach(function (nid) {
    const n = distractors.filter(function (x) { return x.id === nid; })[0];
    const s = wrongPicked[nid];
    const deduct = n.score;
    earned -= deduct;
    deductions.push({ nodeId: n.id, title: n.title, type: '执行混淆流程（扣全分）', point: -deduct, distract: true });
    nodeResults.push({
      id: n.id, title: n.title, role: n.target, plan: planText(n),
      actual: '已执行（' + (s.step_time || '—') + '）', required: false, submitted: true,
      hit: false, ontime: false, wrong: true, distract: true, score: deduct, earned: -deduct,
      late: false, state: '决策偏离'
    });
  });

  Object.keys(skipMap).forEach(function (nid) {
    const n = distractors.filter(function (x) { return x.id === nid; })[0];
    if (!n) return;
    if (!releasedDistractors.some(function (x) { return x.id === nid; })) return;
    skippedDistract++;
    nodeResults.push({
      id: n.id, title: n.title, role: n.target, plan: planText(n),
      actual: '已跳过（未执行）', required: false, submitted: false,
      hit: false, ontime: false, wrong: false, skipped: true, score: 0, earned: 0,
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
  const totalDeduct = DEDUCT_BASE - earned;
  const sc = Math.max(0, Math.round(earned * 10) / 10);
  const completeness = total ? Math.round(hitCnt / total * 1000) / 10 : 0;
  const timeRate = total ? Math.round(ontimeCnt / total * 1000) / 10 : 0;
  return {
    projectId: projectId, mode: mode, scenarioId: p.scenario_id, ended: ended,
    total: total, hit: hitCnt, ontime: ontimeCnt, missed: missedCnt, pending: pendingCnt,
    wrong: wrongCnt, late: lateCnt, maxScore: maxScore, distractors: releasedDistractors.length,
    skipped: skippedCorrect + skippedDistract, skippedCorrect: skippedCorrect, skippedDistract: skippedDistract,
    completeness: completeness, timeRate: timeRate,
    score: sc, grade: gradeOf(sc), deductions: deductions, bonuses: [],
    formula: '扣分制：起始 100 分'
      + (hitCnt ? '，已完成正确流程 ' + hitCnt + '/' + total + ' 项' : '')
      + (lateCnt ? '，超时 ' + lateCnt + ' 项（每项扣本节点 10%）' : '')
      + (wrongCnt ? '，执行混淆流程 ' + wrongCnt + ' 项（各扣 ' + deductions.filter(function (d) { return d.distract; }).reduce(function (a, d) { return a - d.point; }, 0) + ' 分）' : '')
      + (missedCnt ? '，未完成正确流程 ' + missedCnt + ' 项（扣 ' + deductions.filter(function (d) { return d.type.indexOf('未完成') >= 0 || d.skipped; }).reduce(function (a, d) { return a - d.point; }, 0) + ' 分）' : '')
      + (totalDeduct ? '，合计扣 ' + totalDeduct + ' 分' : '，无扣分')
      + ' = ' + sc + ' 分',
    nodes: nodeResults, steps: steps, correctTotal: maxScore
  };
}

// ========== 精简输出（对齐 scripts/node_score_S01/S02/S05.json 基准字段） ==========
function compact(sc) {
  return {
    projectId: sc.projectId,
    scenarioId: sc.scenarioId,
    total: sc.total, hit: sc.hit, ontime: sc.ontime,
    late: sc.late, wrong: sc.wrong, missed: sc.missed,
    skipped: sc.skipped, distractors: sc.distractors,
    score: sc.score, grade: sc.grade,
    completeness: sc.completeness, timeRate: sc.timeRate,
    deductions: sc.deductions.map(function (d) {
      const out = { nodeId: d.nodeId, type: d.type, point: d.point };
      if (d.missed) out.missed = true;
      if (d.late) out.late = true;
      if (d.distract) out.distract = true;
      if (d.skipped) out.skipped = true;
      return out;
    }),
    formula: sc.formula
  };
}

// ========== CLI ==========
const args = process.argv.slice(2);

// 如果指定了 projectId 参数
if (args.length > 0) {
  const projectId = args[0];
  const result = score(projectId);
  if (!result) {
    console.error('[FATAL] 项目不存在:', projectId);
    process.exit(2);
  }
  console.log(JSON.stringify(compact(result), null, 2));
  process.exit(0);
}

// 否则自动跑 S01/S02/S05 — 先从 DB 查项目名里包含这些场景 ID 的
console.log('[INFO] DB:', DB_PATH);
console.log('[INFO] 自动查询 S01/S02/S05 场景项目...\n');

const want = ['S01', 'S02', 'S05'];
const projs = db.prepare('SELECT id, scenario_id, mode, status FROM projects WHERE scenario_id IS NOT NULL').all();
const results = [];

want.forEach(function (scTag) {
  // 匹配 scenario_id 含 S01/S02/S05 且 projectId 含 AG_END 的
  const match = projs.filter(function (p) {
    return String(p.scenario_id || '').indexOf(scTag) >= 0 && String(p.id || '').indexOf('AG_END') >= 0;
  })[0];
  if (!match) {
    console.log('[WARN] 未找到 ' + scTag + ' 场景项目');
    return;
  }
  const s = score(match.id);
  if (!s) {
    console.log('[WARN] score(' + match.id + ') 返回 null');
    return;
  }
  results.push(compact(s));
  console.log('--- ' + scTag + ' ---');
  console.log('  projectId:', match.id, '| scenarioId:', match.scenario_id, '| mode:', match.mode, '| status:', match.status);
  console.log('  score:', s.score, 'grade:', s.grade, 'total:', s.total, 'hit:', s.hit, 'wrong:', s.wrong, 'missed:', s.missed);
  console.log('  formula:', s.formula);
  console.log('');
});

// 完整 JSON 输出
console.log('=== FULL JSON ===');
console.log(JSON.stringify(results, null, 2));

process.exit(0);
