/* 通用工具 */
'use strict';

function pad(n) { return n < 10 ? '0' + n : '' + n; }

/* 本地时间字符串 YYYY-MM-DD HH:mm */
function now(ts) {
  const d = ts ? new Date(ts) : new Date();
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

/* 仅时分 HH:mm */
function hhmm(ts) {
  const d = ts ? new Date(ts) : new Date();
  return pad(d.getHours()) + ':' + pad(d.getMinutes());
}

/* 完整时间 YYYY-MM-DD HH:mm:ss */
function nowSec(ts) {
  const d = ts ? new Date(ts) : new Date();
  return now(ts) + ':' + pad(d.getSeconds());
}

/* 客户端 IP（兼容反向代理） */
function ipOf(req) {
  let ip = (req.headers['x-forwarded-for'] || '').split(',')[0].trim() || req.socket.remoteAddress || '';
  if (ip.indexOf('::ffff:') === 0) ip = ip.slice(7);
  if (ip === '::1') ip = '127.0.0.1';
  return ip;
}

/* 字节数格式化 */
function fmtSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1024 / 1024).toFixed(1) + ' MB';
}

/* 安全文件名（去除路径与危险字符） */
function safeName(name) {
  return String(name || 'file').replace(/[\\/:*?"<>|]/g, '_').replace(/^\.+/, '_').slice(0, 120);
}

module.exports = { now, hhmm, nowSec, ipOf, fmtSize, safeName };
