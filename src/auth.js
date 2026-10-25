/* 认证与会话（scrypt 密码散列 + 内存/数据库会话） */
'use strict';
const crypto = require('node:crypto');
const { db } = require('./db');
const { now, nowSec, ipOf } = require('./util');

const SESSION_TTL_MIN = Number(process.env.DRILL_SESSION_MIN || 480); // 默认 8 小时
const INITIAL_PASSWORD = process.env.DRILL_INIT_PWD || 'Pzy@2026';

function hashPassword(pwd, salt) {
  salt = salt || crypto.randomBytes(16).toString('hex');
  const hash = crypto.scryptSync(String(pwd), salt, 32).toString('hex');
  return { salt, hash };
}

function verifyPassword(pwd, salt, hash) {
  try {
    const h = crypto.scryptSync(String(pwd), salt, 32).toString('hex');
    return crypto.timingSafeEqual(Buffer.from(h, 'hex'), Buffer.from(hash, 'hex'));
  } catch (e) {
    return false;
  }
}

function createSession(userId, roleId) {
  const token = crypto.randomBytes(24).toString('hex');
  const exp = new Date(Date.now() + SESSION_TTL_MIN * 60000);
  db.prepare('INSERT INTO sessions (token,user_id,role_id,created_at,expires_at) VALUES (?,?,?,?,?)')
    .run(token, userId, roleId || null, nowSec(), nowSec(exp.getTime()));
  return token;
}

/* 用户全部角色 id */
function rolesOf(userId) {
  return db.prepare('SELECT role_id FROM user_roles WHERE user_id=?').all(userId)
    .map(function (r) { return r.role_id; });
}

function destroySession(token) {
  if (!token) return;
  db.prepare('DELETE FROM sessions WHERE token=?').run(token);
}

function cleanExpired() {
  db.prepare("DELETE FROM sessions WHERE expires_at < ?").run(nowSec());
}

/* 根据 token 取用户（含角色信息） */
function userByToken(token) {
  if (!token) return null;
  const row = db.prepare(
    'SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,u.must_change,s.role_id AS session_role,s.expires_at ' +
    'FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=?'
  ).get(token);
  if (!row) return null;
  if (row.expires_at < nowSec()) { destroySession(token); return null; }
  /* 会话角色优先；旧会话无角色时回落到账号主角色 */
  const roleId = row.session_role || row.role_id;
  const held = db.prepare('SELECT 1 AS ok FROM user_roles WHERE user_id=? AND role_id=?').get(row.id, roleId);
  if (!held) { destroySession(token); return null; } /* 角色已被移除，强制重新登录 */
  const role = db.prepare('SELECT * FROM roles WHERE id=?').get(roleId);
  if (!role) return null;
  if ((role.status || '启用') !== '启用') { destroySession(token); return null; } /* 角色已停用 */
  return {
    id: row.id,
    username: row.username,
    name: row.name,
    org: row.org,
    status: row.status,
    mustChange: !!row.must_change,
    roles: rolesOf(row.id),
    role: {
      id: role.id, name: role.name, short: role.short, icon: role.icon,
      color: role.color, desc: role.descr,
      nav: JSON.parse(role.nav || '[]'),
      external: !!role.external
    }
  };
}

function tokenOf(req) {
  const h = req.headers['authorization'] || '';
  if (h.indexOf('Bearer ') === 0) return h.slice(7).trim();
  return req.headers['x-drill-token'] || '';
}

/* 认证中间件 */
function requireAuth(req, res, next) {
  const u = userByToken(tokenOf(req));
  if (!u) return res.status(401).json({ error: '未登录或会话已过期，请重新登录' });
  if (u.status !== '启用') return res.status(403).json({ error: '账号已停用，请联系平台管理员' });
  req.user = u;
  next();
}

/* 页面权限中间件：页面 key 必须在角色 nav 中 */
function requirePage(key) {
  return function (req, res, next) {
    if (!req.user) return res.status(401).json({ error: '未登录' });
    if (req.user.role.nav.indexOf(key) < 0) {
      return res.status(403).json({ error: '当前角色无该模块权限（分级权限隔离）' });
    }
    next();
  };
}

/* 审计留痕：只追加，不提供删除接口 */
function audit(req, action, userOverride) {
  const u = userOverride || req.user || {};
  try {
    db.prepare('INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)')
      .run(now(), u.name || '匿名', u.role ? u.role.id : (u.role_id || '-'),
           u.id || null, String(action), ipOf(req));
  } catch (e) {
    console.error('[audit] 写入失败:', e.message);
  }
}

module.exports = {
  INITIAL_PASSWORD, hashPassword, verifyPassword,
  createSession, destroySession, cleanExpired, userByToken, tokenOf, rolesOf,
  requireAuth, requirePage, audit
};
