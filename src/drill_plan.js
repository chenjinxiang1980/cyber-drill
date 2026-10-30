/* 演练模式支撑：实战版处置决策选项、基础版任务生成
 * 字典数据（角色映射/表单映射/等级映射/决策干扰项）已迁移至 dict 表：
 *   key=drill_role_by_target / drill_form_by_action / drill_level_by_scenario / drill_action_meta
 *  运行时从 src/drill_dict.js 读取（30s 缓存 + seed_data.js 兜底）
 * admin 系统设置 → 处置流程字典 区块可编辑，修改后即时生效（drillDict.invalidate 清缓存）
 */
'use strict';

const drillDict = require('./drill_dict');
const S = require('./seed_data');

/* 从 dict 表或 seed_data 兜底获取字典（带 30s 缓存） */
function dict(db, key, fallback) {
  return drillDict.get(db || require('./db').db, key, fallback);
}

function hash(str) {
  /* FNV-1a + 末位混合：避免连续节点 ID 得到相同的取模结果，保证选项位置真正打乱 */
  let h = 2166136261 >>> 0;
  const s = String(str || '');
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619) >>> 0;
  }
  h ^= h >>> 16; h = Math.imul(h, 2246822507) >>> 0;
  h ^= h >>> 13; h = Math.imul(h, 3266489909) >>> 0;
  h ^= h >>> 16;
  return h >>> 0;
}

/* 生成固定的三项选项（1项正确 + 2项干扰），选项顺序按节点 ID 稳定打乱
 * 干扰项优先使用剧本编辑中按节点配置的 wrong，未配置时按动作类型自动生成 */
function buildDecision(node, _db) {
  const db = _db || require('./db').db;
  const actionMeta = dict(db, 'drill_action_meta', S.DICT.drill_action_meta);
  const meta = actionMeta[node.action] || actionMeta['其他'];
  const question = (node.type === 'alert' ? (meta.alert || meta.q) : (meta.report || meta.q)) || meta.q;
  const pool = meta.wrong || (actionMeta['其他'] && actionMeta['其他'].wrong) || [];
  const h = hash(node.id);
  const title = String(node.title || '按预案处置');
  const distractors = [];
  const custom = Array.isArray(node.wrong) ? node.wrong : [];
  custom.forEach(function (x) {
    const text = String(x || '').trim();
    if (text && text !== title && distractors.indexOf(text) < 0) distractors.push(text);
  });
  /* 自定义干扰项不足 2 项时，从动作类型通用干扰项补齐 */
  for (let k = 0; distractors.length < 2 && k < pool.length; k++) {
    const text = pool[(h + k) % pool.length];
    if (text !== title && distractors.indexOf(text) < 0) distractors.push(text);
  }
  const raw = [
    { text: title, correct: true },
    { text: distractors[0] || '暂不处置，继续观察', correct: false },
    { text: distractors[1] || '仅口头报告，不形成记录', correct: false }
  ];
  const shift = (h >>> 5) % 3;
  const options = [];
  for (let i = 0; i < 3; i++) {
    const item = raw[(i + shift) % 3];
    options.push({ id: i + 1, text: item.text, correct: item.correct });
  }
  return { question: question, options: options };
}

/* target 中文名 → roles 表 role_id
 * 三级匹配：精确 name/short → 反向包含（target 包含 role.name 或 role.short，反之亦然） → 兜底 duty */
function targetToRoleId(db, target) {
  const t = String(target || '').trim();
  if (!t) return 'duty';
  // 精确匹配
  let row = db.prepare("SELECT id FROM roles WHERE name=? OR short=?").get(t, t);
  if (row) return row.id;
  // 包含匹配（处理 "医务部/门诊部负责人" 这种组合名称 → roles.short="医务部"）
  const contains = db.prepare("SELECT id, name, short FROM roles WHERE ? LIKE '%' || name || '%' OR ? LIKE '%' || short || '%' OR name LIKE '%' || ? || '%' OR short LIKE '%' || ? || '%'").all(t, t, t, t);
  if (contains.length === 1) return contains[0].id;
  return 'duty';
}

/* 判断当前角色是否可处置该剧本节点（target=中文名 → 查表） */
function canHandle(role, target, _db) {
  if (!role) return false;
  if (role.id === 'platformadmin' || role.id === 'commander') return true;
  const db = _db || require('./db').db;
  const mapped = targetToRoleId(db, target);
  if (role.id === 'itlead') return ['itlead', 'duty', 'netsec', 'sysadmin'].indexOf(mapped) >= 0;
  return mapped === role.id;
}

/* 基础版：按剧本节点自动下发任务，并自动匹配需提交的表单 */
function ensureBasicTasks(db, project) {
  if (!project || !project.scenario_id) return 0;
  if ((project.mode || '基础版') !== '基础版') return 0;
  const exists = db.prepare('SELECT COUNT(*) AS c FROM tasks WHERE project_id=?').get(project.id);
  if (exists && exists.c > 0) return 0;
  const nodes = db.prepare('SELECT * FROM script_nodes WHERE scenario_id=? ORDER BY seq').all(project.scenario_id)
    .filter(function (n) { return n.type !== 'end' && n.flow_type !== 'distractor'; });
  if (!nodes.length) return 0;
  const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(project.scenario_id);
  const levelByScenario = dict(db, 'drill_level_by_scenario', S.DICT.drill_level_by_scenario);
  const level = levelByScenario[(sc && sc.level) || ''] || 'P2';
  const prefix = 'T' + String(project.id).replace(/[^0-9]/g, '') + '-';
  const insert = db.prepare('INSERT INTO tasks (id,project_id,role_id,title,descr,deadline,status,level,forms,node_id,seq) VALUES (?,?,?,?,?,?,?,?,?,?,?)');
  let count = 0;
  nodes.forEach(function (n, i) {
    const roleId = targetToRoleId(db, n.target);
    let formId = String(n.form || '').trim();
    if (!formId && n.action === '上报审批' && targetToRoleId(db, n.target) === 'commander') formId = 'F4';
    const forms = formId ? [formId] : [];
    insert.run(prefix + String(i + 1).padStart(2, '0'), project.id, roleId, n.title, n.descr || '',
      '', '待处理', level, JSON.stringify(forms), n.id, i + 1);
    count++;
  });
  return count;
}

/* 旧库升级：进行中的基础版项目若任务未关联剧本节点，按基线重建任务，保证任务中心展示具体处置步骤 */
function backfillBasicTasks(db) {
  const projs = db.prepare("SELECT * FROM projects WHERE scenario_id IS NOT NULL AND status NOT IN ('已归档','已结束')").all();
  let count = 0;
  projs.forEach(function (p) {
    if ((p.mode || '基础版') !== '基础版') return;
    const rows = db.prepare('SELECT id,node_id FROM tasks WHERE project_id=?').all(p.id);
    if (rows.length && rows.every(function (r) { return !!r.node_id; })) return;
    if (!db.prepare('SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=?').get(p.scenario_id).c) return;
    if (rows.length) {
      db.prepare('DELETE FROM task_attachments WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id);
      db.prepare('DELETE FROM form_submissions WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id);
      db.prepare('DELETE FROM evidence WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)').run(p.id);
      db.prepare('DELETE FROM tasks WHERE project_id=?').run(p.id);
    }
    count += ensureBasicTasks(db, p);
  });
  return count;
}

/* 正确流程默认分值：按节点数均分 100（余数分给前面节点），仅当全部节点分值为 0 时补齐 */
function defaultScores(db, scenarioId) {
  const nodes = db.prepare("SELECT * FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq").all(scenarioId);
  if (!nodes.length) return 0;
  if (nodes.some(function (n) { return Number(n.score || 0) > 0; })) return 0;
  const base = Math.floor(100 / nodes.length);
  const rest = 100 - base * nodes.length;
  const up = db.prepare('UPDATE script_nodes SET score=? WHERE id=?');
  nodes.forEach(function (n, i) { up.run(base + (i < rest ? 1 : 0), n.id); });
  return nodes.length;
}

/* 实战版混淆流程：按责任角色为每个正确流程角色额外生成 3 条与剧本动作相符的处置流程
 * 基础版不显示混淆流程（工作台按 flow_type 过滤），因此共用同一份剧本 */
function ensureDistractors(db) {
  const scenarios = db.prepare('SELECT id FROM scenarios').all();
  const actionMeta = dict(db, 'drill_action_meta', S.DICT.drill_action_meta);
  let count = 0;
  scenarios.forEach(function (sc) {
    const exists = db.prepare("SELECT COUNT(*) AS c FROM script_nodes WHERE scenario_id=? AND flow_type='distractor'").get(sc.id);
    if (exists && exists.c > 0) return;
    const nodes = db.prepare("SELECT * FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq").all(sc.id);
    if (!nodes.length) return;
    const maxSeq = db.prepare('SELECT MAX(seq) AS m FROM script_nodes WHERE scenario_id=?').get(sc.id).m || 0;
    const ins = db.prepare('INSERT INTO script_nodes (id,scenario_id,type,title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,action,wrong,evidence_req,required,flow_type,score,deps,pos_x,pos_y) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)');
    const byRole = {};
    nodes.forEach(function (n) {
      const roleId = targetToRoleId(db, n.target);
      if (!byRole[roleId]) byRole[roleId] = [];
      byRole[roleId].push(n);
    });
    let seq = maxSeq;
    Object.keys(byRole).forEach(function (roleId) {
      const group = byRole[roleId];
      const first = group[0];
      const meta = actionMeta[first.action] || actionMeta['其他'];
      const titles = group.map(function (n) { return n.title; });
      const picked = [];
      meta.wrong.forEach(function (w) {
        if (picked.length >= 3) return;
        if (titles.indexOf(w) >= 0 || picked.indexOf(w) >= 0) return;
        picked.push(w);
      });
      picked.forEach(function (title, k) {
        seq++;
        ins.run('D' + sc.id + '-' + roleId + '-' + (k + 1), sc.id, first.type || 'task', title, first.target,
          '', first.timeout || null, first.form || '', '', '处置流程：' + title, '[]', seq,
          '', '', first.action || '', '[]', '[]', 0, 'distractor', 10, '[]', null, null);
        count++;
      });
    });
  });
  return count;
}

/* 旧库升级：补齐正确流程分值并生成实战版混淆流程 */
function backfillFlowScores(db) {
  const scenarios = db.prepare('SELECT id FROM scenarios').all();
  let scored = 0;
  scenarios.forEach(function (sc) { scored += defaultScores(db, sc.id); });
  return { scored: scored, distractors: ensureDistractors(db) };
}

/* 安全解析节点前置依赖（JSON 数组） */
function parseDeps(v) {
  try {
    const arr = JSON.parse(v || '[]');
    return Array.isArray(arr) ? arr.filter(Boolean).map(String) : [];
  } catch (e) {
    return [];
  }
}

/* 下一节点接收人：按依赖关系取后继正确流程的责任角色并集，无后继时流转至院应急指挥部 */
function backfillReceivers(db) {
  const scenarios = db.prepare('SELECT id FROM scenarios').all();
  const up = db.prepare('UPDATE script_nodes SET receiver=? WHERE id=?');
  let count = 0;
  scenarios.forEach(function (sc) {
    const nodes = db.prepare("SELECT * FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq").all(sc.id);
    nodes.forEach(function (n) {
      if (String(n.receiver || '').trim()) return;
      const targets = [];
      nodes.forEach(function (x) {
        if (!parseDeps(x.deps).length || parseDeps(x.deps).indexOf(n.id) < 0) return;
        const t = String(x.target || '').trim();
        if (t && targets.indexOf(t) < 0) targets.push(t);
      });
      up.run(targets.length ? targets.join('、') : '院应急指挥部', n.id);
      count++;
    });
  });
  return count;
}

/* 基础版任务可用性：前置全部完成才可处置
 * 返回 nodeId -> { available, blockedBy:[{id,title}] } */
function basicNodeAvailability(db, projectId) {
  const p = db.prepare('SELECT scenario_id FROM projects WHERE id=?').get(projectId);
  if (!p) return {};
  const nodes = db.prepare('SELECT id,title,deps,flow_type,type FROM script_nodes WHERE scenario_id=?').all(p.scenario_id);
  const byId = {};
  nodes.forEach(function (n) { byId[n.id] = n; });
  const done = new Set(db.prepare("SELECT node_id FROM tasks WHERE project_id=? AND status='已完成'").all(projectId)
    .map(function (r) { return r.node_id; }));
  const map = {};
  nodes.forEach(function (n) {
    if (n.type === 'end' || n.flow_type === 'distractor') return;
    const pending = parseDeps(n.deps).filter(function (id) { return byId[id] && !done.has(id); });
    map[n.id] = {
      available: pending.length === 0,
      blockedBy: pending.map(function (id) { return { id: id, title: byId[id].title }; })
    };
  });
  return map;
}

/* 旧库升级：信息科负责人「上报审批」节点绑定《网络和数据安全事件反馈单》（F8）
 * 仅回填空表单节点，并把基础版该类任务的旧上报表替换为反馈单 */
function backfillFeedbackForm(db) {
  let nodes = 0, tasks = 0;
  try {
    /* 仅回填正确流程节点；混淆项（DS 前缀）不挂表单，避免误导参演人员 */
    db.prepare("SELECT id, form FROM script_nodes WHERE action='上报审批' AND target='信息科负责人' AND id NOT LIKE 'DS%'").all()
      .forEach(function (n) {
        if (!n.form) { db.prepare("UPDATE script_nodes SET form='F8' WHERE id=?").run(n.id); nodes++; }
      });
    db.prepare(
      "SELECT t.id, t.forms FROM tasks t JOIN script_nodes n ON n.id=t.node_id " +
      "JOIN projects p ON p.id=t.project_id " +
      "WHERE n.action='上报审批' AND n.target='信息科负责人' AND p.status NOT IN ('已归档')"
    ).all().forEach(function (t) {
      let arr = [];
      try { arr = JSON.parse(t.forms || '[]'); } catch (e) { arr = []; }
      if (!Array.isArray(arr)) arr = [];
      arr = arr.filter(function (x) { return x !== 'F2'; });
      if (arr.indexOf('F8') < 0) arr.push('F8');
      db.prepare('UPDATE tasks SET forms=? WHERE id=?').run(JSON.stringify(arr), t.id);
      tasks++;
    });
  } catch (e) {
    console.warn('[drill_plan] 反馈单绑定跳过：', e.message);
  }
  return { nodes: nodes, tasks: tasks };
}

/* 旧库升级：剧本节点表单与任务表单同步
 * 剧本节点 form 修改后，已存在项目的 tasks.forms 不会自动更新，
 * 本函数扫描所有活跃项目，确保 tasks.forms = script_nodes.form 的一对一映射
 * （仅修正已存在的不同步，不破坏 script_nodes.form 为空的兜底场景）
 */
function backfillTaskForms(db) {
  let fixed = 0, checked = 0;
  const projs = db.prepare("SELECT * FROM projects WHERE status NOT IN ('已归档','已结束') AND scenario_id IS NOT NULL").all();
  projs.forEach(function (p) {
    const nodes = db.prepare("SELECT id, form FROM script_nodes WHERE scenario_id=? AND COALESCE(flow_type,'correct')!='distractor'").all(p.scenario_id);
    const nodeFormMap = {};
    nodes.forEach(function (n) { nodeFormMap[n.id] = String(n.form || '').trim(); });
    const tasks = db.prepare('SELECT * FROM tasks WHERE project_id=? AND node_id IS NOT NULL').all(p.id);
    const upd = db.prepare('UPDATE tasks SET forms=? WHERE id=?');
    tasks.forEach(function (t) {
      checked++;
      const nodeForm = nodeFormMap[t.node_id] || '';
      let curForms = [];
      try { curForms = JSON.parse(t.forms || '[]'); } catch (e) { curForms = []; }
      if (!Array.isArray(curForms)) curForms = [];

      let targetForms;
      if (nodeForm) {
        // 剧本节点有明确 form → 一对一覆盖（修复 backfillFeedbackForm 等历史追加问题）
        targetForms = [nodeForm];
      } else {
        // 剧本节点没 form → 保留当前值
        targetForms = curForms.slice();
      }

      const oldStr = curForms.slice().sort().join(',');
      const newStr = targetForms.slice().sort().join(',');
      if (oldStr !== newStr) {
        upd.run(JSON.stringify(targetForms), t.id);
        fixed++;
      }
    });
  });
  console.log(`[migrate] 剧本/任务表单同步: 检查 ${checked} 条 tasks，修正 ${fixed} 条`);
  return { checked: checked, fixed: fixed };
}

/* 导出的字典快照（兼容层，只读）
 * 注意：这个是运行时快照，缓存 30s。如需写操作请走 dict 表 + drillDict.invalidate() */
function FORM_BY_ACTION(_db) { return dict(_db || require('./db').db, 'drill_form_by_action', S.DICT.drill_form_by_action); }
function LEVEL_BY_SCENARIO(_db) { return dict(_db || require('./db').db, 'drill_level_by_scenario', S.DICT.drill_level_by_scenario); }
function ACTION_META(_db) { return dict(_db || require('./db').db, 'drill_action_meta', S.DICT.drill_action_meta); }

module.exports = {
  FORM_BY_ACTION: FORM_BY_ACTION,
  LEVEL_BY_SCENARIO: LEVEL_BY_SCENARIO,
  ACTION_META: ACTION_META,
  buildDecision, canHandle, ensureBasicTasks, backfillBasicTasks,
  defaultScores, ensureDistractors, backfillFlowScores, backfillReceivers, backfillFeedbackForm, backfillTaskForms,
  parseDeps, basicNodeAvailability
};
