'use strict';

/* 解析本地时间字符串（YYYY-MM-DD HH:mm[:ss]），返回毫秒；非法返回 NaN */
function parseLocal(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?/.exec(String(s || ''));
  if (!m) return NaN;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] || 0)).getTime();
}

/* 已进行秒数：未启动返回 null；暂停取暂停时刻；结束取结束时刻；扣除累计暂停 */
function elapsedSec(p, at) {
  if (!p || !p.started_at) return null;
  const start = parseLocal(p.started_at);
  if (!isFinite(start)) return null;
  let end;
  if (p.ended_at) end = parseLocal(p.ended_at);
  else if (p.paused_at && p.status === '暂停') end = parseLocal(p.paused_at);
  else end = at ? (typeof at === 'number' ? at : parseLocal(at)) : Date.now();
  if (!isFinite(end)) end = Date.now();
  const sec = (end - start) / 1000 - (Number(p.paused_sec) || 0);
  return sec > 0 ? Math.floor(sec) : 0;
}

function elapsedMinutes(p, at) {
  const s = elapsedSec(p, at);
  return s === null ? null : Math.floor(s / 60);
}

function formatElapsed(sec) {
  const s = Math.max(0, Math.floor(Number(sec) || 0));
  const p2 = function (n) { return n < 10 ? '0' + n : '' + n; };
  return p2(Math.floor(s / 3600)) + ':' + p2(Math.floor(s % 3600 / 60)) + ':' + p2(s % 60);
}

/* 两个本地时间字符串之间的秒数（用于继续演练时累加暂停时长） */
function secBetween(a, b) {
  const x = parseLocal(a), y = parseLocal(b);
  return (isFinite(x) && isFinite(y)) ? Math.max(0, Math.floor((y - x) / 1000)) : 0;
}

module.exports = { parseLocal, elapsedSec, elapsedMinutes, formatElapsed, secBetween };
