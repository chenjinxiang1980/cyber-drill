/* 系统品牌设置接口：免登录读取 + 平台管理员维护（含 Logo 上传）
 * + 系统初始化状态查询 / 重置（POST /reset, GET /status）
 * + 处置流程字典维护（GET /dict/:key, PUT /dict/:key）
 */
'use strict';
const fs = require('node:fs');
const express = require('express');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const brand = require('../brand');
const seed = require('../seed');
const drillDict = require('../drill_dict');
const { makeUploader, handleUploadError } = require('../upload');

const router = express.Router();

function requireBrandAdmin(req, res, next) {
  if (!req.user) return res.status(401).json({ error: '未登录' });
  if (req.user.role.id !== 'platformadmin') {
    return res.status(403).json({ error: '当前角色无系统设置权限（仅平台系统管理员可维护）' });
  }
  next();
}

/* 互斥锁：防止并发重置 */
const LOCK_KEY = 'reset_lock';
const LOCK_TIMEOUT_MS = 30 * 1000;

function tryLock() {
  try {
    const existing = db.prepare("SELECT value FROM settings WHERE key=?").get(LOCK_KEY);
    if (existing) {
      const t = Number(existing.value);
      if (t && Date.now() - t < LOCK_TIMEOUT_MS) return false;
      db.prepare("DELETE FROM settings WHERE key=?").run(LOCK_KEY);
    }
    db.prepare("INSERT INTO settings (key,value) VALUES (?,?)").run(LOCK_KEY, String(Date.now()));
    return true;
  } catch (e) { return false; }
}
function releaseLock() {
  try { db.prepare("DELETE FROM settings WHERE key=?").run(LOCK_KEY); } catch (e) {}
}

/* 公共品牌信息 */
router.get('/brand', function (req, res) {
  res.json(brand.readBrand(db));
});

/* 保存系统设置 */
router.put('/brand', auth.requireAuth, requireBrandAdmin, function (req, res) {
  try {
    const next = brand.writeBrand(db, req.body);
    const changed = brand.FIELDS.filter(function (f) { return req.body && req.body[f] !== undefined; });
    auth.audit(req, '更新系统设置：' + (changed.length ? changed.join('、') : '无变更'));
    res.json({ ok: true, brand: next });
  } catch (e) {
    res.status(e.status || 400).json({ error: e.message });
  }
});

/* 单位 Logo 上传 */
const LOGO_MIME = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'];
router.post('/logo', auth.requireAuth, requireBrandAdmin,
  makeUploader('system').single('file'), handleUploadError, function (req, res) {
    const f = req.file;
    if (!f) return res.status(400).json({ error: '请选择要上传的 Logo 文件' });
    if (!LOGO_MIME.includes(f.mimetype)) {
      try { fs.unlinkSync(f.path); } catch (e) {}
      return res.status(400).json({ error: '仅支持 PNG / JPG / WebP / GIF 图片' });
    }
    if (f.size > 2 * 1024 * 1024) {
      try { fs.unlinkSync(f.path); } catch (e) {}
      return res.status(400).json({ error: 'Logo 文件不能超过 2MB' });
    }
    auth.audit(req, '上传单位 Logo：' + (f.originalname || f.filename));
    res.json({ ok: true, url: '/uploads/' + req._uploadRel + '/' + f.filename, name: f.originalname });
  });

/* 系统初始化状态（platformadmin） */
router.get('/status', auth.requireAuth, requireBrandAdmin, function (req, res) {
  res.json(seed.status());
});

/* 系统重置：清空全部业务数据，按 src/seed_data.js 基线重新初始化 */
router.post('/reset', auth.requireAuth, requireBrandAdmin, function (req, res) {
  if (!tryLock()) {
    return res.status(409).json({ error: '另一个系统重置正在进行中，请稍后再试' });
  }
  try {
    const r = seed.resetAll();
    auth.audit(req, '重置系统为初始基线（账号、角色、剧本、项目、处置流程字典等全部恢复基线）');
    res.json({ ok: true, userCount: r.userCount, resetAt: r.resetAt });
  } catch (e) {
    console.error('[reset]', e && e.stack ? e.stack : e);
    res.status(500).json({ error: '系统重置失败：' + (e.message || e) });
  } finally {
    releaseLock();
  }
});

/* === 处置流程字典维护 === */

/* 字典 key 白名单（共 15 个：4 个 drill_* + 9 个业务字典 + 2 个 HTML 硬编码字典） */
const DRILL_DICT_KEYS = [
  'drill_role_by_target',
  'drill_form_by_action',
  'drill_level_by_scenario',
  'drill_action_meta',
  'ev_types',
  'external_evidence_types',
  'target_role_aliases',
  'fill_roles_by_role',
  'managed_roles_by_role',
  'chat_channels',
  'kb_edit_roles',
  'external_node_patterns',
  'role_feed_mapping',
  'script_action_types'
];

/* key → schema 校验器（可选，校验失败返回错误字符串） */
function validateDrillDict(key, value) {
  const errors = [];
  const roles = db.prepare('SELECT id FROM roles').all().map(function (r) { return r.id; });
  const forms = db.prepare('SELECT id FROM form_templates').all().map(function (r) { return r.id; });
  if (!value || typeof value !== 'object') return ['字典值必须是对象'];

  if (key === 'drill_role_by_target' || key === 'target_role_aliases') {
    Object.keys(value).forEach(function (k) {
      if (!value[k] || roles.indexOf(value[k]) < 0) errors.push('「' + k + '」的角色 ID「' + value[k] + '」在 roles 表中不存在');
    });
  } else if (key === 'drill_form_by_action') {
    Object.keys(value).forEach(function (k) {
      if (!value[k] || forms.indexOf(value[k]) < 0) errors.push('「' + k + '」的表单模板 ID「' + value[k] + '」在 form_templates 表中不存在');
    });
  } else if (key === 'drill_level_by_scenario') {
    /* 结构校验：必须是简单 key→字符串映射 */
    Object.keys(value).forEach(function (k) {
      if (typeof value[k] !== 'string' || !value[k]) errors.push('「' + k + '」的任务等级必须是非空字符串');
    });
  } else if (key === 'drill_action_meta') {
    Object.keys(value).forEach(function (action) {
      const m = value[action];
      if (!m || typeof m !== 'object') { errors.push('「' + action + '」必须是对象'); return; }
      if (m.wrong && !Array.isArray(m.wrong)) errors.push('「' + action + '」的 wrong 必须是字符串数组');
      if (m.wrong && m.wrong.length === 0) errors.push('「' + action + '」的 wrong 干扰项不能全部为空');
      ['alert', 'report', 'q'].forEach(function (f) {
        if (m[f] !== undefined && m[f] !== null && typeof m[f] !== 'string') errors.push('「' + action + '」的 ' + f + ' 必须是字符串');
      });
    });
  } else if (key === 'ev_types') {
    if (!Array.isArray(value)) { errors.push('必须是数组'); }
    else value.forEach(function (t, i) { if (!t || typeof t !== 'string') errors.push('第 ' + (i+1) + ' 项必须是非空字符串'); });
  } else if (key === 'external_evidence_types') {
    if (!Array.isArray(value)) { errors.push('必须是数组'); }
    else {
      const evTypes = db.prepare("SELECT value FROM dict WHERE key='ev_types'").get();
      const evList = evTypes ? parseJson(evTypes.value, []) : [];
      value.forEach(function (t, i) {
        if (!t || typeof t !== 'string') errors.push('第 ' + (i+1) + ' 项必须是非空字符串');
        else if (evList.length && evList.indexOf(t) < 0) errors.push('第 ' + (i+1) + ' 项「' + t + '」不在 ev_types 中');
      });
    }
  } else if (key === 'fill_roles_by_role' || key === 'managed_roles_by_role') {
    Object.keys(value).forEach(function (rid) {
      const v = value[rid];
      if (roles.indexOf(rid) < 0) { errors.push('key 「' + rid + '」不是合法 role_id'); return; }
      if (v === null) return; /* null 表示全部 */
      if (!Array.isArray(v)) { errors.push('「' + rid + '」的值必须是数组或 null'); return; }
      v.forEach(function (r2) {
        if (roles.indexOf(r2) < 0) errors.push('「' + rid + '」数组中的「' + r2 + '」不是合法 role_id');
      });
    });
  } else if (key === 'chat_channels') {
    Object.keys(value).forEach(function (k) {
      const ch = value[k];
      if (!ch || typeof ch !== 'object') { errors.push('频道「' + k + '」必须是对象'); return; }
      if (!ch.label || typeof ch.label !== 'string') errors.push('频道「' + k + '」缺少非空 label');
      if (!Array.isArray(ch.roles)) { errors.push('频道「' + k + '」的 roles 必须是数组'); return; }
      ch.roles.forEach(function (r2) {
        if (roles.indexOf(r2) < 0) errors.push('频道「' + k + '」的 roles 中「' + r2 + '」不是合法 role_id');
      });
    });
  } else if (key === 'kb_edit_roles') {
    if (!Array.isArray(value)) { errors.push('必须是数组'); }
    else value.forEach(function (rid) {
      if (roles.indexOf(rid) < 0) errors.push('「' + rid + '」不是合法 role_id');
    });
  } else if (key === 'external_node_patterns') {
    if (!value.orgs || !Array.isArray(value.orgs)) errors.push('缺少 orgs 数组');
    if (!value.events || !Array.isArray(value.events)) errors.push('缺少 events 数组');
    if (value.orgs) value.orgs.forEach(function (t, i) { if (!t || typeof t !== 'string') errors.push('orgs 第 ' + (i+1) + ' 项必须是非空字符串'); });
    if (value.events) value.events.forEach(function (t, i) { if (!t || typeof t !== 'string') errors.push('events 第 ' + (i+1) + ' 项必须是非空字符串'); });
  } else if (key === 'role_feed_mapping') {
    if (!value || typeof value !== 'object') { errors.push('必须是对象'); }
    else Object.keys(value).forEach(function (rid) {
      if (roles.indexOf(rid) < 0) { errors.push('key 「' + rid + '」不是合法 role_id'); return; }
      const v = value[rid];
      if (!v || typeof v !== 'string') errors.push('「' + rid + '」的值必须是非空字符串（feed module 名）');
    });
  } else if (key === 'script_action_types') {
    if (!Array.isArray(value)) { errors.push('必须是数组'); }
    else value.forEach(function (t, i) { if (!t || typeof t !== 'string') errors.push('第 ' + (i+1) + ' 项必须是非空字符串'); });
  }
  return errors;
}

/* 读取单个处置流程字典 */
router.get('/dict/:key', auth.requireAuth, requireBrandAdmin, function (req, res) {
  const key = req.params.key;
  if (DRILL_DICT_KEYS.indexOf(key) < 0) {
    return res.status(404).json({ error: '字典 key 不存在（仅支持：' + DRILL_DICT_KEYS.join(', ') + '）' });
  }
  const row = db.prepare('SELECT value FROM dict WHERE key=?').get(key);
  const value = row ? parseJson(row.value, {}) : {};
  res.json({ key: key, value: value });
});

/* 更新单个处置流程字典（含关联完整性校验） */
router.put('/dict/:key', auth.requireAuth, requireBrandAdmin, function (req, res) {
  const key = req.params.key;
  if (DRILL_DICT_KEYS.indexOf(key) < 0) {
    return res.status(404).json({ error: '字典 key 不存在（仅支持：' + DRILL_DICT_KEYS.join(', ') + '）' });
  }
  const value = req.body && req.body.value;
  const errors = validateDrillDict(key, value);
  if (errors.length) {
    return res.status(400).json({ error: '校验未通过', errors: errors });
  }
  try {
    db.prepare('INSERT INTO dict (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value')
      .run(key, JSON.stringify(value));
    drillDict.invalidate();
    auth.audit(req, '更新处置流程字典：' + key + '（涉及 ' + Object.keys(value).length + ' 条配置）');
    res.json({ ok: true });
  } catch (e) {
    console.error('[dict]', e && e.stack ? e.stack : e);
    res.status(500).json({ error: '保存失败：' + (e.message || e) });
  }
});

module.exports = router;
