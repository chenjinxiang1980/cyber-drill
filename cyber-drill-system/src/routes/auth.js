/* 认证接口 */
'use strict';
const express = require('express');
const { db } = require('../db');
const auth = require('../auth');
const { nowSec } = require('../util');

const router = express.Router();

/* 登录页可选账号（不含密码，仅用于选择角色后自动填充账号） */
router.get('/accounts', function (req, res) {
  const rows = db.prepare(
    'SELECT u.username, u.name, u.role_id, r.name AS role_name, r.short, r.color, r.external ' +
    'FROM users u JOIN roles r ON r.id=u.role_id WHERE u.status=\'启用\' ORDER BY r.seq, u.id'
  ).all();
  res.json(rows.map(function (r) {
    return { username: r.username, name: r.name, role: r.role_id, roleName: r.role_name, short: r.short, color: r.color, external: !!r.external };
  }));
});

/* 公开角色列表（登录页展示） */
router.get('/roles', function (req, res) {
  const rows = db.prepare('SELECT * FROM roles ORDER BY seq').all();
  res.json(rows.map(function (r) {
    return {
      id: r.id, name: r.name, short: r.short, icon: r.icon, color: r.color,
      desc: r.descr, nav: JSON.parse(r.nav || '[]'), external: !!r.external
    };
  }));
});

/* 登录 */
router.post('/login', function (req, res) {
  const username = String((req.body && req.body.username) || '').trim();
  const password = String((req.body && req.body.password) || '');
  if (!username || !password) return res.status(400).json({ error: '请输入账号和密码' });

  const u = db.prepare('SELECT * FROM users WHERE username=?').get(username);
  if (!u) return res.status(401).json({ error: '账号或密码错误' });
  if (u.status !== '启用') return res.status(403).json({ error: '账号已停用，请联系平台管理员' });
  if (!auth.verifyPassword(password, u.pwd_salt, u.pwd_hash)) {
    auth.audit(req, '登录失败：账号 ' + username, { name: username, role_id: '-' });
    return res.status(401).json({ error: '账号或密码错误' });
  }

  auth.cleanExpired();
  const token = auth.createSession(u.id);
  const role = db.prepare('SELECT * FROM roles WHERE id=?').get(u.role_id);
  auth.audit(req, '登录系统', { id: u.id, name: u.name, role_id: u.role_id });

  res.json({
    token: token,
    user: { username: u.username, name: u.name, org: u.org, mustChange: !!u.must_change },
    role: {
      id: role.id, name: role.name, short: role.short, icon: role.icon, color: role.color,
      desc: role.descr, nav: JSON.parse(role.nav || '[]'), external: !!role.external
    }
  });
});

/* 登出 */
router.post('/logout', function (req, res) {
  const token = auth.tokenOf(req);
  const u = auth.userByToken(token);
  if (u) auth.audit(req, '退出登录');
  auth.destroySession(token);
  res.json({ ok: true });
});

/* 当前用户 */
router.get('/me', auth.requireAuth, function (req, res) {
  const u = req.user;
  res.json({
    username: u.username, name: u.name, org: u.org, mustChange: u.mustChange,
    role: u.role
  });
});

/* 修改密码 */
router.post('/change-password', auth.requireAuth, function (req, res) {
  const oldPwd = String((req.body && req.body.oldPassword) || '');
  const newPwd = String((req.body && req.body.newPassword) || '');
  if (newPwd.length < 8 || !/[A-Za-z]/.test(newPwd) || !/[0-9]/.test(newPwd)) {
    return res.status(400).json({ error: '新密码需至少 8 位，且同时包含字母与数字' });
  }
  const u = db.prepare('SELECT * FROM users WHERE id=?').get(req.user.id);
  if (!auth.verifyPassword(oldPwd, u.pwd_salt, u.pwd_hash)) {
    return res.status(400).json({ error: '原密码不正确' });
  }
  const p = auth.hashPassword(newPwd);
  db.prepare('UPDATE users SET pwd_salt=?, pwd_hash=?, must_change=0 WHERE id=?').run(p.salt, p.hash, u.id);
  auth.audit(req, '修改登录密码');
  res.json({ ok: true, time: nowSec() });
});

module.exports = router;
