/* 剧本取证材料要求：按节点动作与责任角色自动生成需取证材料清单 */
'use strict';
const { db } = require('./db');
const drillDict = require('./drill_dict');

function parseJsonSafe(s, fb) {
  if (s === null || s === undefined || s === '') return fb;
  try { return JSON.parse(s); } catch (e) { return fb; }
}

/* 单条材料规范化，避免脏数据进入清单；证据类型白名单从 dict 表 ev_types 读取 */
function normalizeItem(it) {
  if (!it) return null;
  const name = String(it.name || '').trim().slice(0, 160);
  if (!name) return null;
  const evTypes = drillDict.get(db, 'ev_types', ['附件截图','操作日志','处置记录','样本摘要','表单文书','上报材料','报案材料','附件']);
  const type = evTypes.indexOf(it.type) >= 0 ? it.type : '处置记录';
  return { type: type, name: name, note: String(it.note || '').trim().slice(0, 400) };
}

function normalizeReq(arr) {
  if (!Array.isArray(arr)) return [];
  const out = [];
  arr.forEach(function (x) { const n = normalizeItem(x); if (n) out.push(n); });
  return out.slice(0, 6);
}

/* 按剧本节点自动生成取证材料建议清单：结合动作类型、责任角色、表单、处置指南 */
function autoEvidenceReq(node) {
  const n = node || {};
  const title = String(n.title || '');
  const target = String(n.target || '');
  const action = String(n.action || '');
  const guide = String(n.guide || '');
  const desc = String(n.desc || '');
  const form = String(n.form || '');
  const doc = [];
  /* 表单类节点：表单本身即取证材料 */
  if (form) doc.push({ type: '表单文书', name: String(n.form).trim() + '（' + title.slice(0, 20) + '）', note: '需经签字/审批的正式文书' });
  /* 按动作关键词匹配取证类型 */
  const kv = {
    '告警|核实|监控|排查': [['附件截图', '告警/监控界面截图'], ['处置记录', '核实处置记录']],
    '隔离|断网|封堵|封禁': [['操作日志', '网络设备/防火墙操作日志'], ['处置记录', '隔离处置记录']],
    '取证|样本|日志|证据': [['样本摘要', '样本/日志摘要归档'], ['附件', '取证材料附件']],
    '备份|恢复|验证': [['处置记录', '备份/恢复操作记录'], ['附件截图', '业务恢复验证截图']],
    '报告|上报|审批|启动': [['上报材料', '上报材料/审批记录'], ['表单文书', '报告/审批表单']],
    '通知|预警|公告|兜底': [['处置记录', '通知发布记录'], ['附件截图', '预警/兜底界面截图']],
    '报案|舆情|对外|口径': [['报案材料', '报案材料/对接记录'], ['上报材料', '对外口径/舆情回应']],
    '复盘|终止|评估|整改': [['处置记录', '复盘评估记录']]
  };
  let matched = false;
  Object.keys(kv).forEach(function (pat) {
    const re = new RegExp(pat);
    if (re.test(action) || re.test(title) || re.test(desc) || re.test(guide)) {
      kv[pat].forEach(function (item) { doc.push({ type: item[0], name: item[1], note: '' }); });
      matched = true;
    }
  });
  /* 未匹配动作类型：按节点类型补充基础取证 */
  if (!matched) {
    doc.push({ type: '处置记录', name: title.slice(0, 24) + '处置记录', note: '' });
    doc.push({ type: '附件截图', name: '界面/流程截图', note: '' });
  }
  return normalizeReq(doc);
}

module.exports = { parseJsonSafe, normalizeItem, normalizeReq, autoEvidenceReq };
