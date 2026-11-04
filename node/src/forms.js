/* 表单模板工具：字段规范化、模板迁移、控件自动升级
 * 内置数据（SELECT_OPTIONS / DEFAULT_TEMPLATES）已迁移至 src/seed_data.js
 * 本文件仅保留运行时工具函数与向后兼容导出
 * 字段结构：{ label, hint, required, type, options }
 *   type ∈ auto|text|textarea|select|multiselect|datetime
 *   select=下拉单选，multiselect=下拉多选（options 用 JSON 数组存储，提交值为顿号分隔字符串）
 */
'use strict';

const { SELECT_OPTIONS: _SO, FORM_TEMPLATES: _FT } = require('./seed_data');
const TYPES = ['auto', 'text', 'textarea', 'select', 'multiselect', 'datetime'];
const MIGRATE_KEY = 'forms_builtin_v2';
const CONTROL_KEY = 'forms_controls_v1';

const SELECT_OPTIONS = _SO;
const DEFAULT_TEMPLATES = _FT;

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
    if (type === 'select' && !options.length) type = 'text';
    const hasOpts = type === 'select' || type === 'multiselect';
    out.push({
      label: label,
      hint: clip(fl.hint || fl.desc, 200),
      required: fl.required !== false,
      type: type,
      options: hasOpts ? options : []
    });
  });
  return out;
}

/* 模板行 → 接口返回结构 */
function normalizeTemplate(row) {
  return { id: row.id, name: row.name, fields: normalizeFields(parseJsonSafe(row.fields, [])) };
}

/* 一次性迁移：升级内置模板字段（含说明）并补齐反馈单，之后以管理员修改为准
 * 同时作为兜底：即使 flag 已存在但表内 fields 全空时也自动补写 */
function ensureForms(db) {
  try {
    const flag = db.prepare('SELECT value FROM settings WHERE key=?').get(MIGRATE_KEY);
    const up = db.prepare(
      'INSERT INTO form_templates (id,name,fields) VALUES (?,?,?) ' +
      'ON CONFLICT(id) DO UPDATE SET name=excluded.name, fields=excluded.fields'
    );
    /* seed_data.js 里字段叫 controls，这里兼容两种命名 */
    function fieldsOf(t) {
      if (t.fields !== undefined) return t.fields;
      if (t.controls !== undefined) return t.controls;
      return [];
    }
    /* 检查表内是否存在至少一条非空 fields；若全空则强制走一次全量写入 */
    let anyNonEmpty = false;
    try {
      const cnt = db.prepare(
        "SELECT COUNT(*) AS c FROM form_templates WHERE fields IS NOT NULL AND fields<>'' AND fields<>'null'"
      ).get().c;
      anyNonEmpty = cnt > 0;
    } catch (e) { anyNonEmpty = false; }

    if (flag && anyNonEmpty) return 0;   /* flag 有且表内正常 → 跳过 */

    let n = 0;
    DEFAULT_TEMPLATES.forEach(function (t) {
      up.run(t.id, t.name, JSON.stringify(fieldsOf(t)));
      n++;
    });
    db.prepare('INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value')
      .run(MIGRATE_KEY, '1');
    return n;
  } catch (e) {
    console.warn('[forms] 内置表单升级跳过：', e.message);
    return 0;
  }
}

/* 一次性迁移：把标准化字段升级为下拉控件，并把时间字段显式标记为日期时间控件 */
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
  TYPES, DEFAULT_TEMPLATES, SELECT_OPTIONS,
  normalizeFields, normalizeTemplate, parseOptions,
  ensureForms, ensureFormControls
};
