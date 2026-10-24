/* 剧本取证材料要求：按节点动作与责任角色自动生成需取证材料清单 */
'use strict';

const EV_TYPES = ['附件截图', '操作日志', '处置记录', '样本摘要', '表单文书', '上报材料', '报案材料', '附件'];

function parseJsonSafe(s, fb) {
  if (s === null || s === undefined || s === '') return fb;
  try { return JSON.parse(s); } catch (e) { return fb; }
}

/* 单条材料规范化，避免脏数据进入清单 */
function normalizeItem(it) {
  if (!it) return null;
  const name = String(it.name || '').trim().slice(0, 160);
  if (!name) return null;
  const type = EV_TYPES.indexOf(it.type) >= 0 ? it.type : '处置记录';
  return { type: type, name: name, note: String(it.note || '').trim().slice(0, 400) };
}

function normalizeReq(arr) {
  if (!Array.isArray(arr)) return [];
  const out = [];
  arr.forEach(function (x) { const n = normalizeItem(x); if (n) out.push(n); });
  return out.slice(0, 6);
}

/* 依据节点信息自动生成「需取证材料」建议（剧本编辑可一键生成后逐项修改） */
function autoEvidenceReq(node) {
  const title = String((node && node.title) || '处置环节');
  const target = String((node && node.target) || '—');
  const action = String((node && node.action) || '');
  const guide = String((node && (node.guide || node.desc || node.descr)) || '按预案执行处置');
  const out = [];

  out.push({
    type: '处置记录',
    name: '处置记录-' + title + '.md',
    note: '责任角色：' + target + '；处置要点：' + guide.slice(0, 200)
  });

  /* 监控/核查/验证类节点：生成与剧本相符的界面截图材料 */
  if (/监控|监测|告警|发现|核查|检查|查看|确认|验证|界面|登录|日志/.test(action + title + guide)) {
    out.push({
      type: '附件截图',
      name: '界面截图-' + title + '.svg',
      note: '责任角色：' + target + '；截取节点「' + title + '」处置时的系统/告警界面（含时间、主机、告警内容），用于证明现场处置状态。'
    });
  }

  if (node && node.form) {
    out.push({
      type: '表单文书',
      name: '表单文书-' + title + '.md',
      note: '节点关联表单 ' + node.form + ' 提交后的归档文书（演练虚拟版本）。'
    });
  }

  if (/取证|样本|日志|排查|分析/.test(action + title + guide)) {
    out.push({
      type: '样本摘要',
      name: '样本摘要-' + title + '.txt',
      note: '虚拟样本摘要（仅哈希与特征描述，不含真实恶意程序）。'
    });
  }

  if (/隔离|断网|封堵|切换|恢复|停机/.test(action + title)) {
    out.push({
      type: '操作日志',
      name: '操作日志-' + title + '.md',
      note: '责任角色：' + target + '；记录节点「' + title + '」的设备/系统操作命令与回显（演练虚拟日志）。'
    });
  }

  return normalizeReq(out);
}

module.exports = { EV_TYPES, normalizeReq, normalizeItem, autoEvidenceReq, parseJsonSafe };
