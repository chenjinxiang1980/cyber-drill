/* 演练模式支撑：实战版处置决策选项、基础版任务生成 */
'use strict';


/* 剧本责任角色 → 系统角色 ID */
const ROLE_BY_TARGET = {
  '院应急指挥部': 'commander',
  '信息科负责人': 'itlead',
  '信息科值班员': 'duty',
  '网络安全管理员': 'netsec',
  '系统管理员': 'sysadmin',
  '医务科主任': 'medlead',
  '临床科室': 'clinical',
  '市委网信办': 'cac'
};

/* 基础版按动作类型（及责任角色）自动匹配的表单 */
const FORM_BY_ACTION = {
  '告警核实': 'F1',
  '隔离处置': 'F2',
  '上报审批': 'F8',
  '业务兜底': 'F3',
  '取证留痕': 'F5',
  '处置恢复': 'F7'
};

/* 场景等级 → 任务等级 */
const LEVEL_BY_SCENARIO = { '重大': 'P0', '较大': 'P1', '一般': 'P2' };

/* 每个动作类型的决策问题与干扰项（干扰项均为不符合预案的处置） */
const ACTION_META = {
  '告警核实': {
    alert: '监控平台推送异常告警，值班人员首先应采取哪项处置？',
    q: '为确认事件性质与影响范围，当前应采取哪项处置？',
    wrong: [
      '先等待10分钟，观察告警是否自动恢复',
      '直接在防火墙上封禁全部外网访问',
      '告知来电人员稍后再联系，暂不记录',
      '仅记录告警截图，不通知任何人员',
      '立即重启核心业务服务器尝试恢复'
    ]
  },
  '隔离处置': {
    q: '为防止事态扩大，当前应采取哪项处置？',
    wrong: [
      '继续观察，待业务科室反馈后再处置',
      '直接关闭全院网络交换机电源',
      '仅在本机删除可疑文件，不做网络隔离',
      '等待厂商到场后再决定是否隔离',
      '先恢复业务系统，再排查异常主机'
    ]
  },
  '取证留痕': {
    q: '在处置的同时，取证与留痕方面应如何做？',
    wrong: [
      '格式化受影响主机后再排查',
      '仅口头记录，事后补写处置经过',
      '先删除可疑文件，避免病毒扩散',
      '关闭审计日志服务以释放磁盘空间',
      '将可疑样本直接转发到互联网网盘'
    ]
  },
  '上报审批': {
    report: '当前事态下，报告与决策方面应如何处理？',
    q: '该环节在上报与协同方面应如何处置？',
    wrong: [
      '先自行处置完毕，事后视情况再上报',
      '仅在科室内部口头告知，不形成书面记录',
      '越级直接向市委网信办报告，不经过院应急指挥部',
      '等待业务完全中断后再上报',
      '只上报业务科室，不通知信息科负责人'
    ]
  },
  '协同沟通': {
    q: '跨部门协同方面，当前应如何处置？',
    wrong: [
      '让业务科室自行处理，信息科不介入',
      '在未确认范围前通知全院停诊',
      '仅通过私人微信通知个别人员',
      '等待上级电话询问后再组织协同',
      '暂不沟通，避免引起恐慌'
    ]
  },
  '业务兜底': {
    q: '业务连续性受影响时，当前应采取哪项处置？',
    wrong: [
      '继续依赖故障系统，等待其自行恢复',
      '直接宣布全院停诊，不做任何兜底',
      '让临床科室手工记录后不再补录',
      '仅恢复挂号系统，其他业务不处理',
      '关闭所有业务系统以统一恢复'
    ]
  },
  '处置恢复': {
    q: '事件受控后，恢复与验证方面应如何处置？',
    wrong: [
      '未做验证即全面恢复对外服务',
      '直接恢复上线，不再检查日志',
      '仅恢复一台终端即认为业务恢复',
      '恢复后立即删除全部取证材料',
      '跳过审批直接恢复核心数据库'
    ]
  },
  '其他': {
    q: '针对当前异常，应采取哪项处置？',
    wrong: [
      '暂不处置，继续观察',
      '仅口头报告，不形成记录',
      '未经研判直接重启全部系统',
      '等待他人处理，自己不介入',
      '删除异常日志后再上报'
    ]
  }
};

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
function buildDecision(node) {
  const meta = ACTION_META[node.action] || ACTION_META['其他'];
  const question = (node.type === 'alert' ? (meta.alert || meta.q) : (meta.report || meta.q)) || meta.q;
  const pool = meta.wrong || ACTION_META['其他'].wrong;
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

/* 判断当前角色是否可处置该剧本节点 */
function canHandle(role, target) {
  if (!role) return false;
  if (role.id === 'platformadmin' || role.id === 'commander') return true;
  const mapped = ROLE_BY_TARGET[target] || null;
  if (role.id === 'itlead') return ['itlead', 'duty', 'netsec', 'sysadmin'].indexOf(mapped) >= 0;
  return target === role.name || target === role.short || mapped === role.id;
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
  const level = LEVEL_BY_SCENARIO[(sc && sc.level) || ''] || 'P2';
  const prefix = 'T' + String(project.id).replace(/[^0-9]/g, '') + '-';
  const insert = db.prepare('INSERT INTO tasks (id,project_id,role_id,title,descr,deadline,status,level,forms,node_id,seq) VALUES (?,?,?,?,?,?,?,?,?,?,?)');
  let count = 0;
  nodes.forEach(function (n, i) {
    const roleId = ROLE_BY_TARGET[n.target] || 'duty';
    /* 上报审批：指挥部决策环节使用《应急启动审批单》，信息科负责人环节使用《网络和数据安全事件反馈单》 */
    let formId = FORM_BY_ACTION[n.action] || '';
    if (n.action === '上报审批' && n.target === '院应急指挥部') formId = 'F4';
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
      const roleId = ROLE_BY_TARGET[n.target] || 'duty';
      if (!byRole[roleId]) byRole[roleId] = [];
      byRole[roleId].push(n);
    });
    let seq = maxSeq;
    Object.keys(byRole).forEach(function (roleId) {
      const group = byRole[roleId];
      const first = group[0];
      const meta = ACTION_META[first.action] || ACTION_META['其他'];
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

module.exports = {
  ROLE_BY_TARGET, FORM_BY_ACTION, buildDecision, canHandle, ensureBasicTasks, backfillBasicTasks,
  defaultScores, ensureDistractors, backfillFlowScores, backfillReceivers, backfillFeedbackForm,
  parseDeps, basicNodeAvailability
};
