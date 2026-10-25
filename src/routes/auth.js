/* 认证接口 */
'use strict';
const express = require('express');
const { db } = require('../db');
const auth = require('../auth');
const { nowSec } = require('../util');

const router = express.Router();

/* 登录页可选账号（不含密码，仅用于选择角色后下拉选择账号） */
router.get('/accounts', function (req, res) {
  const rows = db.prepare("SELECT id, username, name, org FROM users WHERE status='启用' ORDER BY id").all();
  const roleRows = db.prepare(
    'SELECT ur.user_id, ur.role_id, r.name, r.short, r.color, r.external ' +
    'FROM user_roles ur JOIN roles r ON r.id=ur.role_id JOIN users u ON u.id=ur.user_id ' +
    "WHERE u.status='启用' AND r.status='启用' ORDER BY r.seq"
  ).all();
  const byUser = {};
  roleRows.forEach(function (x) {
    (byUser[x.user_id] = byUser[x.user_id] || []).push({
      id: x.role_id, name: x.name, short: x.short, color: x.color, external: !!x.external
    });
  });
  const out = [];
  rows.forEach(function (r) {
    const roles = byUser[r.id] || [];
    if (!roles.length) return; /* 全部角色均已停用时不显示该账号 */
    out.push({
      username: r.username, name: r.name, org: r.org,
      role: roles[0].id, roleName: roles[0].name, short: roles[0].short,
      color: roles[0].color, external: roles[0].external,
      roles: roles
    });
  });
  res.json(out);
});

/* 公开角色列表（登录页展示；停用角色不显示） */
router.get('/roles', function (req, res) {
  const rows = db.prepare("SELECT * FROM roles WHERE status='启用' ORDER BY seq").all();
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
  /* 登录所选角色（多角色账号）；未指定时使用账号主角色 */
  const picked = String((req.body && req.body.role) || '').trim();
  const roleId = picked || u.role_id;
  if (picked && !db.prepare('SELECT 1 AS ok FROM user_roles WHERE user_id=? AND role_id=?').get(u.id, picked)) {
    return res.status(400).json({ error: '该账号不具备所选角色，请重新选择角色或联系平台管理员' });
  }
  const role = db.prepare('SELECT * FROM roles WHERE id=?').get(roleId);
  if (!role) return res.status(400).json({ error: '角色不存在，请联系平台管理员' });
  if ((role.status || '启用') !== '启用') return res.status(403).json({ error: '该角色已停用，请选择其他角色登录' });
  if (!auth.verifyPassword(password, u.pwd_salt, u.pwd_hash)) {
    auth.audit(req, '登录失败：账号 ' + username, { name: username, role_id: '-' });
    return res.status(401).json({ error: '账号或密码错误' });
  }

  auth.cleanExpired();
  const token = auth.createSession(u.id, roleId);
  auth.audit(req, '登录系统', { id: u.id, name: u.name, role_id: roleId });

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
    username: u.username, name: u.name, org: u.org, mustChange: !!u.must_change,
    role: u.role, roles: u.roles || []
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
