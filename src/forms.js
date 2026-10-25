/* 表单模板：内置上报文书字段定义、字段规范化与首次迁移
 * 字段结构：{ label, hint, required, type, options }，type ∈ auto|text|textarea|select|datetime
 * 兼容历史数据：fields 为字符串数组时自动转换为对象结构
 */
'use strict';

const TYPES = ['auto', 'text', 'textarea', 'select', 'datetime'];
const MIGRATE_KEY = 'forms_builtin_v2';
const CONTROL_KEY = 'forms_controls_v1';

/* 标准化字段 → 下拉选项（用于事件等级等规范取值） */
const SELECT_OPTIONS = {
  '发现方式': ['安全告警系统', '值班巡检', '临床科室报障', '外部单位通报', '第三方监测', '其他'],
  '事件类型': ['勒索病毒', 'DDoS 攻击', '数据泄露', '系统宕机', '钓鱼邮件', '内网渗透', '其他'],
  '事件等级': ['特别重大', '重大', '较大', '一般'],
  '建议响应等级': ['Ⅰ级', 'Ⅱ级', 'Ⅲ级', 'Ⅳ级'],
  '风险等级': ['高', '中', '低']
};

function parseOptions(v) {
  if (Array.isArray(v)) return v.map(function (x) { return clip(x, 20); }).filter(Boolean).slice(0, 30);
  if (typeof v === 'string') return v.split(/[、,，|｜]/).map(function (x) { return clip(x, 20); }).filter(Boolean).slice(0, 30);
  return [];
}

function clip(s, n) {
  return String(s === null || s === undefined ? '' : s).trim().slice(0, n);
}

function parseJsonSafe(s, fallback) {
  if (s === null || s === undefined || s === '') return fallback;
  try { return JSON.parse(s); } catch (e) { return fallback; }
}

/* 字段规范化：字符串 → 对象；对象 → 清洗后的对象 */
function normalizeFields(raw) {
  if (!Array.isArray(raw)) return [];
  const out = [];
  raw.forEach(function (fl) {
    if (fl === null || fl === undefined) return;
    if (typeof fl === 'string') {
      const label = clip(fl, 40);
      if (label) out.push({ label: label, hint: '', required: true, type: 'auto', options: [] });
      return;
    }
    if (typeof fl !== 'object') return;
    const label = clip(fl.label || fl.name, 40);
    if (!label) return;
    let type = TYPES.indexOf(fl.type) >= 0 ? fl.type : 'auto';
    const options = parseOptions(fl.options);
    if (type === 'select' && !options.length) type = 'text'; /* 无选项的下拉退化为文本 */
    out.push({
      label: label,
      hint: clip(fl.hint || fl.desc, 200),
      required: fl.required !== false,
      type: type,
      options: type === 'select' ? options : []
    });
  });
  return out;
}

/* 模板行 → 接口返回结构 */
function normalizeTemplate(row) {
  return { id: row.id, name: row.name, fields: normalizeFields(parseJsonSafe(row.fields, [])) };
}

/* 内置模板：按真实网络安全事件处置流程编制（字段说明用于分步填表引导） */
const DEFAULT_TEMPLATES = [
  {
    id: 'F1', name: '事件初步核实单',
    fields: [
      { label: '发现时间', hint: '以监控平台或值班记录中首次发现的时间为准，精确到分钟。', required: true, type: 'datetime' },
      { label: '发现方式', hint: '如安全告警系统、值班巡检、临床科室报障、外部单位通报。', required: true, type: 'select', options: SELECT_OPTIONS['发现方式'] },
      { label: '报告人', hint: '首先发现并报告的人员姓名及所属科室。', required: true, type: 'text' },
      { label: '受影响系统', hint: '出现异常的业务系统名称，如 HIS、EMR、LIS、PACS。', required: true, type: 'text' },
      { label: 'IP/域名', hint: '涉及的主机 IP、域名或访问地址，多个用顿号分隔。', required: true, type: 'text' },
      { label: '异常现象', hint: '客观描述观察到的现象，如文件被加密、异常外联、登录失败激增。', required: true, type: 'textarea' },
      { label: '初步判断', hint: '结合告警与现象初判事件类型和等级，注明为模拟核实结论。', required: true, type: 'textarea' },
      { label: '初步影响范围', hint: '已确认受影响的科室、业务和主机数量，未确认的标注「待评估」。', required: true, type: 'textarea' },
      { label: '已采取临时措施', hint: '如暂时断开网线、暂停共享目录写入，注明执行人与时间。', required: true, type: 'textarea' },
      { label: '核实人', hint: '完成初步核实的信息科值班人员姓名。', required: true, type: 'text' },
      { label: '核实时间', hint: '完成初步核实的时间，精确到分钟。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F2', name: '网络安全事件上报表',
    fields: [
      { label: '上报单位', hint: '填写医院全称，与系统单位名称一致。', required: true, type: 'text' },
      { label: '事件类型', hint: '如勒索病毒、DDoS 攻击、数据泄露、系统宕机等。', required: true, type: 'select', options: SELECT_OPTIONS['事件类型'] },
      { label: '事件等级', hint: '按预案分为特别重大/重大/较大/一般，与研判结论一致。', required: true, type: 'select', options: SELECT_OPTIONS['事件等级'] },
      { label: '发生时间', hint: '事件实际发生时间；无法确认时填写首次发现时间并注明。', required: true, type: 'datetime' },
      { label: '发现时间', hint: '首次发现事件的时间，精确到分钟。', required: true, type: 'datetime' },
      { label: '受攻击系统', hint: '被攻击或受影响的业务系统名称。', required: true, type: 'text' },
      { label: 'IP/域名', hint: '受攻击资产 IP、域名或端口，多个用顿号分隔。', required: true, type: 'text' },
      { label: '事件基本情况', hint: '按「何时、何地、何事、如何发现」叙述事件经过。', required: true, type: 'textarea' },
      { label: '影响范围', hint: '受影响业务系统、科室、患者服务与数据范围（演练虚拟数据）。', required: true, type: 'textarea' },
      { label: '已采取措施', hint: '已执行的隔离、封堵、停机、备份等处置动作及时间。', required: true, type: 'textarea' },
      { label: '处置进展', hint: '当前处置阶段与预计完成时间。', required: true, type: 'textarea' },
      { label: '下一步计划', hint: '拟开展的溯源、恢复、整改等计划安排。', required: true, type: 'textarea' },
      { label: '报送人/联系电话', hint: '报送人姓名 + 虚拟联系电话。', required: true, type: 'text' },
      { label: '报送时间', hint: '实际报送时间，精确到分钟。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F3', name: '业务影响评估表',
    fields: [
      { label: '受影响业务系统', hint: '受影响的业务系统清单，多个用顿号分隔。', required: true, type: 'text' },
      { label: '业务中断起止时间', hint: '格式如 2026-10-24 09:00 至 10:30；仍在中断填「持续中」。', required: true, type: 'text' },
      { label: '受影响科室', hint: '受影响的临床、医技与窗口科室。', required: true, type: 'textarea' },
      { label: '受影响患者数（虚拟）', hint: '仅填写演练虚拟数量，不得录入真实患者信息。', required: true, type: 'text' },
      { label: '关键业务兜底方案', hint: '如纸质处方、人工挂号收费、手工登记，注明启动条件。', required: true, type: 'textarea' },
      { label: '预计恢复时间', hint: '预计业务恢复时间。', required: true, type: 'datetime' },
      { label: '评估人', hint: '完成评估的医务/门诊负责人。', required: true, type: 'text' },
      { label: '评估时间', hint: '评估完成时间。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F4', name: '应急启动审批单',
    fields: [
      { label: '事件摘要', hint: '简述事件事实、等级与当前影响。', required: true, type: 'textarea' },
      { label: '建议响应等级', hint: '按预案填写 Ⅰ级/Ⅱ级/Ⅲ级/Ⅳ级。', required: true, type: 'select', options: SELECT_OPTIONS['建议响应等级'] },
      { label: '启动依据', hint: '引用预案条款与研判结论。', required: true, type: 'textarea' },
      { label: '拟采取措施', hint: '拟启动的响应措施与责任分工。', required: true, type: 'textarea' },
      { label: '申请部门/申请人', hint: '提出启动申请的单位与人员。', required: true, type: 'text' },
      { label: '审批人', hint: '具备启动审批权限的院领导。', required: true, type: 'text' },
      { label: '审批意见', hint: '同意/不同意及补充要求。', required: true, type: 'textarea' },
      { label: '审批时间', hint: '审批完成时间。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F5', name: '报案登记表',
    fields: [
      { label: '报案单位', hint: '报案单位全称。', required: true, type: 'text' },
      { label: '报案人/联系电话', hint: '报案人姓名 + 虚拟联系电话。', required: true, type: 'text' },
      { label: '事件发现时间', hint: '首次发现事件的时间。', required: true, type: 'datetime' },
      { label: '事件经过', hint: '按时间顺序叙述发现与处置经过。', required: true, type: 'textarea' },
      { label: '受攻击系统', hint: '受攻击系统名称。', required: true, type: 'text' },
      { label: 'IP/域名', hint: '涉案 IP、域名与端口。', required: true, type: 'text' },
      { label: '损失情况', hint: '仅描述演练虚拟损失，如中断时长、受影响终端数。', required: true, type: 'textarea' },
      { label: '证据清单', hint: '已固定的日志、样本摘要、截图等材料名称。', required: true, type: 'textarea' },
      { label: '报案时间', hint: '提交报案时间。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F6', name: '舆情风险研判表',
    fields: [
      { label: '监测渠道', hint: '如社交媒体、论坛、短视频平台、上级通报。', required: true, type: 'text' },
      { label: '舆情摘要', hint: '舆情主要内容与关键截图描述。', required: true, type: 'textarea' },
      { label: '传播趋势', hint: '扩散速度、主要传播节点与当前热度。', required: true, type: 'textarea' },
      { label: '风险等级', hint: '按研判结论填写 高/中/低。', required: true, type: 'select', options: SELECT_OPTIONS['风险等级'] },
      { label: '口径建议', hint: '统一对外答复口径，不得透露真实患者信息。', required: true, type: 'textarea' },
      { label: '研判人', hint: '参与研判的人员姓名。', required: true, type: 'text' },
      { label: '研判时间', hint: '研判完成时间。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F7', name: '业务恢复确认单',
    fields: [
      { label: '恢复系统清单', hint: '已完成恢复的系统清单。', required: true, type: 'textarea' },
      { label: '数据完整性校验', hint: '备份校验方式、校验结果与 RPO 达成情况。', required: true, type: 'textarea' },
      { label: '业务功能验证结果', hint: '逐项记录 HIS/EMR/LIS/PACS 验证结果。', required: true, type: 'textarea' },
      { label: '恢复时间', hint: '业务恢复完成时间。', required: true, type: 'datetime' },
      { label: '确认人', hint: '业务与信息科双确认人员。', required: true, type: 'text' },
      { label: '确认时间', hint: '确认完成时间。', required: true, type: 'datetime' }
    ]
  },
  {
    id: 'F8', name: '网络和数据安全事件反馈单',
    fields: [
      { label: '受攻击系统', hint: '被攻击或受影响的系统名称及版本，多个用顿号分隔。', required: true, type: 'textarea' },
      { label: 'IP/域名', hint: '受攻击资产 IP、域名、端口，多个用顿号分隔。', required: true, type: 'textarea' },
      { label: '事件基本情况', hint: '简述发生时间、发现方式、攻击手法与事件等级。', required: true, type: 'textarea' },
      { label: '事件影响范围', hint: '受影响业务、科室、数据与患者服务范围（演练虚拟数据）。', required: true, type: 'textarea' },
      { label: '现场处置过程', hint: '按时间顺序记录隔离、封堵、取证、恢复等动作及执行人。', required: true, type: 'textarea' },
      { label: '事件处置结果', hint: '处置成效、业务恢复情况与残留风险。', required: true, type: 'textarea' },
      { label: '下一步整改及工作计划', hint: '整改措施、责任部门、完成时限与复查安排。', required: true, type: 'textarea' }
    ]
  }
];

/* 一次性迁移：升级内置模板字段（含说明）并补齐反馈单，之后以管理员修改为准 */
function ensureForms(db) {
  try {
    const flag = db.prepare('SELECT value FROM settings WHERE key=?').get(MIGRATE_KEY);
    if (flag) return 0;
    const up = db.prepare(
      'INSERT INTO form_templates (id,name,fields) VALUES (?,?,?) ' +
      'ON CONFLICT(id) DO UPDATE SET name=excluded.name, fields=excluded.fields'
    );
    DEFAULT_TEMPLATES.forEach(function (t) { up.run(t.id, t.name, JSON.stringify(t.fields)); });
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value')
      .run(MIGRATE_KEY, '1');
    return DEFAULT_TEMPLATES.length;
  } catch (e) {
    console.warn('[forms] 内置表单升级跳过：', e.message);
    return 0;
  }
}

/* 一次性迁移：把标准化字段升级为下拉控件，并把时间字段显式标记为日期时间控件
 * 已手工配置的类型（select/textarea）保持不变，仅处理 auto/text */
function ensureFormControls(db) {
  try {
    if (db.prepare('SELECT value FROM settings WHERE key=?').get(CONTROL_KEY)) return 0;
    let changed = 0;
    db.prepare('SELECT id, fields FROM form_templates').all().forEach(function (row) {
      const raw = parseJsonSafe(row.fields, []);
      if (!Array.isArray(raw)) return;
      let dirty = false;
      const fields = raw.map(function (fl) {
        if (!fl || typeof fl !== 'object') return fl;
        const label = clip(fl.label, 40);
        const type = TYPES.indexOf(fl.type) >= 0 ? fl.type : 'auto';
        if (SELECT_OPTIONS[label] && (type === 'auto' || type === 'text')) {
          fl.type = 'select'; fl.options = SELECT_OPTIONS[label]; dirty = true;
        } else if ((type === 'auto' || type === 'text') && /时间$|日期$/.test(label) && !/起止|区间|时段/.test(label)) {
          fl.type = 'datetime'; dirty = true;
        }
        return fl;
      });
      if (dirty) {
        db.prepare('UPDATE form_templates SET fields=? WHERE id=?').run(JSON.stringify(fields), row.id);
        changed++;
      }
    });
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value')
      .run(CONTROL_KEY, '1');
    return changed;
  } catch (e) {
    console.warn('[forms] 表单控件升级跳过：', e.message);
    return 0;
  }
}

module.exports = {
  TYPES: TYPES, DEFAULT_TEMPLATES: DEFAULT_TEMPLATES, SELECT_OPTIONS: SELECT_OPTIONS,
  normalizeFields: normalizeFields, normalizeTemplate: normalizeTemplate,
  ensureForms: ensureForms, ensureFormControls: ensureFormControls
};
