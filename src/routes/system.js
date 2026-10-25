/* 系统品牌设置接口：免登录读取 + 平台管理员维护（含 Logo 上传） */
'use strict';
const fs = require('node:fs');
const express = require('express');
const { db } = require('../db');
const auth = require('../auth');
const brand = require('../brand');
const { makeUploader, handleUploadError } = require('../upload');

const router = express.Router();

/* 仅平台系统管理员可维护系统设置 */
function requireBrandAdmin(req, res, next) {
  if (!req.user) return res.status(401).json({ error: '未登录' });
  if (req.user.role.id !== 'platformadmin') {
    return res.status(403).json({ error: '当前角色无系统设置权限（仅平台系统管理员可维护）' });
  }
  next();
}

/* 公共品牌信息：登录页、大屏页免登录读取 */
router.get('/brand', function (req, res) {
  res.json(brand.readBrand(db));
});

/* 保存系统设置（系统名称 / 单位名称 / Logo / 版本 / GitHub / Gitee） */
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

/* 单位 Logo 上传（仅图片类型，≤2MB） */
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

module.exports = router;
