'use strict';
const { db } = require('./db');
const { now } = require('./util');

/* ---------- 站内消息模板引擎 ----------
 * 模板存储于 dict 表 key='notif_templates'，结构：
 * {
 *   "drill_start": { "title": "演练开始 · {project}", "desc": "...", "level": "P0|p1|P2", "source": "..." },
 *   ...
 * }
 * 模板变量用 {varname} 占位，vars 对象传入时替换；模板中不存在的 event 使用 fallback 默认值
 */

/* 内置 fallback 模板（dict 表读不到时使用，保证系统永不出错） */
const FALLBACK_TEMPLATES = {
  drill_start: {
    title: '{action} · {project}',
    desc: '演练项目「{project}」{resumed}（{mode}，{level}级事件）。请各参演角色进入角色工作台按预案处置。',
    source: '演练调度', level: 'P0'
  },
  drill_end: {
    title: '演练结束 · {project}',
    desc: '演练项目「{project}」（{mode}）已结束，请前往复盘评估查看综合得分与考评结果。',
    source: '演练调度', level: 'P1'
  },
  drill_end_score: {
    title: '演练结束 · 综合得分 {score} 分',
    desc: '演练项目「{project}」（{mode}）已结束；正确流程完成 {hit}/{total} 项，评级 {grade}。',
    source: '演练调度', level: 'P1'
  },
  node_done: {
    title: '节点完成 · {title}',
    desc: '演练项目「{project}」任务 {taskId} 已由 {user}（{role}）完成，已完成 {done}/{total} 个节点。',
    source: '角色工作台', level: 'P2'
  },
  node_submit: {
    title: '节点处置 · {title}',
    desc: '演练项目「{project}」由 {user} 提交处置记录；{correct}。',
    source: '演练导调中心', level: 'P2'
  },
  node_skip: {
    title: '节点处置 · {title}',
    desc: '演练项目「{project}」由 {user} 判定该流程不执行（跳过）；{reason}。',
    source: '演练导调中心', level: 'P2'
  },
  auto_eval: {
    title: '系统自动考评完成',
    desc: '已更新 {count} 项考评指标，合计得分 {sum}{max}。',
    source: '复盘评估', level: 'P1'
  },
  project_created: {
    title: '演练项目已创建 · {project}',
    desc: '演练项目「{project}」（{mode}）已创建，关联剧本「{scenario}」（{level}级），计划日期 {date}。请参演角色进入系统查看演练脚本。',
    source: '演练调度', level: 'P1'
  }
};

function loadTemplates() {
  try {
    const row = db.prepare("SELECT value FROM dict WHERE key='notif_templates'").get();
    if (row && row.value) {
      const v = JSON.parse(row.value);
      if (v && typeof v === 'object') return v;
    }
  } catch (e) { /* malformed → fallback */ }
  return FALLBACK_TEMPLATES;
}

/* 首次启动/缺失时，把内置 fallback 模板写入 dict 表，便于前端显示编辑 */
function ensureTemplates() {
  try {
    const row = db.prepare("SELECT value FROM dict WHERE key='notif_templates'").get();
    if (row && row.value) {
      /* 已存在：补齐缺失的 event 键，保留用户已有配置 */
      let cur = {};
      try { cur = JSON.parse(row.value) || {}; } catch (e) { cur = {}; }
      let changed = false;
      Object.keys(FALLBACK_TEMPLATES).forEach(function (ev) {
        if (!cur[ev]) { cur[ev] = FALLBACK_TEMPLATES[ev]; changed = true; }
      });
      if (changed) {
        db.prepare("UPDATE dict SET value=? WHERE key='notif_templates'").run(JSON.stringify(cur));
        console.log('[migrate] 站内消息模板：补齐缺失 event');
      }
      return;
    }
    /* 不存在：整包插入 */
    db.prepare("INSERT INTO dict (key,value) VALUES ('notif_templates', ?)").run(JSON.stringify(FALLBACK_TEMPLATES));
    console.log('[migrate] 站内消息模板：首次写入 7 条默认模板');
  } catch (e) {
    console.warn('[migrate] 站内消息模板写入跳过：', e.message);
  }
}

function render(tpl, vars) {
  if (!tpl) return '';
  let s = String(tpl);
  const v = vars || {};
  s = s.replace(/\{(\w+)\}/g, function (m, k) {
    const val = v[k];
    return val == null ? '' : String(val);
  });
  return s;
}

/* 写入一条站内消息
 * 两种调用方式均可：
 *   旧：notify.push({ title:'...', desc:'...', level, source, project_id })
 *   新：notify.push({ event:'drill_start', vars:{ project, mode, level, ... }, project_id })
 * 新方式优先；event 不存在或模板缺失时自动回退为旧方式 */
function push(opts) {
  const o = opts || {};
  let title = '', desc = '', level = 'P2', source = '演练调度';

  if (o.event) {
    const tpls = loadTemplates();
    const tpl = tpls[o.event] || FALLBACK_TEMPLATES[o.event];
    if (tpl) {
      title = render(tpl.title, o.vars);
      desc = render(tpl.desc, o.vars);
      level = tpl.level || level;
      source = tpl.source || source;
    }
  }
  /* 旧参数覆盖（兼容直接传 title/desc 的场景，如测试/临时通知） */
  if (o.title) title = String(o.title).trim();
  if (o.desc != null) desc = String(o.desc);
  if (o.level && ['P0', 'P1', 'P2'].indexOf(o.level) >= 0) level = o.level;
  if (o.source) source = String(o.source);

  if (!title) return null;

  const seq = (db.prepare('SELECT COALESCE(MAX(seq),0)+1 AS n FROM notifs').get().n) || 1;
  const id = 'N' + Date.now() + '_' + Math.floor(Math.random() * 900 + 100);
  let pid = o.project_id || null;
  if (!pid) {
    try { pid = (db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value || null; } catch (e) {}
  }
  db.prepare('INSERT INTO notifs (id,time,level,source,title,descr,seq,target,project_id) VALUES (?,?,?,?,?,?,?,?,?)')
    .run(id, now(), level, source.slice(0, 40), title.slice(0, 120), desc.slice(0, 500), seq,
      o.target === 'all' ? 'all' : 'internal', pid);
  return id;
}

module.exports = { push, FALLBACK_TEMPLATES, ensureTemplates };
