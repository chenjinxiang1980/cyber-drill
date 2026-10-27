/* 演练处置流程字典访问层
 * 运行时从 dict 表读取 drill_* key，带 30s 内存缓存 + seed_data.js 兜底
 * 外部调用：drillDict.get(db, key, fallback)
 * drill_plan.js 不再硬编码 ROLE_BY_TARGET / FORM_BY_ACTION / LEVEL_BY_SCENARIO / ACTION_META
 */
'use strict';

const CACHE_TTL_MS = 30 * 1000;
const cache = {};

function get(db, key, fallback) {
  const now = Date.now();
  const hit = cache[key];
  if (hit && (now - hit.t) < CACHE_TTL_MS) return hit.v;
  let v = null;
  try {
    const row = db.prepare('SELECT value FROM dict WHERE key=?').get(key);
    if (row && row.value) v = JSON.parse(row.value);
  } catch (e) {
    // 查询失败或 JSON 非法，回落
  }
  if (!v || (typeof v === 'object' && Object.keys(v).length === 0)) v = fallback;
  cache[key] = { v: v, t: now };
  return v;
}

function invalidate() {
  for (const k of Object.keys(cache)) delete cache[k];
}

module.exports = { get, invalidate };
