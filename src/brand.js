/* 系统品牌设置（系统名称 / 单位名称 / 单位Logo / 版本信息 / 项目地址）
 * 统一存储于 settings 表，供登录页、大屏页与登录后各页面同步使用。
 */
'use strict';

const KEYS = {
  systemName: 'brand_system_name',
  unitName: 'brand_unit_name',
  logo: 'brand_logo',
  version: 'brand_version',
  github: 'brand_github',
  gitee: 'brand_gitee'
};

const FIELDS = Object.keys(KEYS);

const DEFAULTS = {
  systemName: '网络安全应急演练系统',
  unitName: '盘州市人民医院',
  logo: 'img/logo.png',
  version: 'v' + require('../package.json').version,
  github: 'https://github.com/chenjinxiang1980/cyber-drill',
  gitee: 'https://gitee.com/habeas/cyber-drill'
};

/* 幂等补齐默认值，不覆盖已修改的设置 */
function ensureBrand(db) {
  const ins = db.prepare('INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO NOTHING');
  FIELDS.forEach(function (f) { ins.run(KEYS[f], DEFAULTS[f]); });
}

/* 读取品牌设置（缺失或为空时回落到默认值） */
function readBrand(db) {
  const rows = db.prepare('SELECT key,value FROM settings').all();
  const map = {};
  rows.forEach(function (r) { map[r.key] = r.value; });
  const out = {};
  FIELDS.forEach(function (f) {
    const v = map[KEYS[f]];
    out[f] = (v === null || v === undefined || v === '') ? DEFAULTS[f] : String(v);
  });
  return out;
}

/* 写入品牌设置：仅接受受支持字段，逐项校验并落库 */
function writeBrand(db, patch) {
  patch = patch || {};
  const next = {};
  FIELDS.forEach(function (f) {
    if (patch[f] === undefined || patch[f] === null) return;
    next[f] = String(patch[f]).trim();
  });
  if (!next.systemName) next.systemName = DEFAULTS.systemName;
  if (!next.unitName) next.unitName = DEFAULTS.unitName;
  if (!next.version) next.version = DEFAULTS.version;
  if (next.github !== undefined && next.github && !/^https?:\/\//i.test(next.github)) {
    const err = new Error('GitHub 地址必须以 http:// 或 https:// 开头');
    err.status = 400;
    throw err;
  }
  if (next.gitee !== undefined && next.gitee && !/^https?:\/\//i.test(next.gitee)) {
    const err = new Error('Gitee 地址必须以 http:// 或 https:// 开头');
    err.status = 400;
    throw err;
  }
  if (next.logo !== undefined && next.logo && !/^\/?(img|uploads)\//.test(next.logo)) {
    const err = new Error('Logo 地址不合法');
    err.status = 400;
    throw err;
  }

  const up = db.prepare(
    'INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value'
  );
  Object.keys(next).forEach(function (f) { up.run(KEYS[f], next[f]); });
  return readBrand(db);
}

module.exports = { KEYS, FIELDS, DEFAULTS, ensureBrand, readBrand, writeBrand };
