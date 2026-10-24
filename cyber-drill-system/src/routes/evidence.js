/* 取证与材料归档接口 */
'use strict';
const express = require('express');
const path = require('node:path');
const fs = require('node:fs');
const { db } = require('../db');
const auth = require('../auth');
const { makeUploader, handleUploadError, UPLOAD_DIR } = require('../upload');
const { now, hhmm, fmtSize, safeName } = require('../util');

const router = express.Router();
const upload = makeUploader('evidence');

const EXTERNAL_EVIDENCE_TYPES = ['表单文书', '上报材料', '报案材料'];

function visible(user, rows) {
  if (!user.role.external) return rows;
  return rows.filter(function (e) {
    return EXTERNAL_EVIDENCE_TYPES.indexOf(e.type) >= 0 || ['itlead', 'commander'].indexOf(e.role_id) >= 0;
  });
}

router.get('/', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT * FROM evidence ORDER BY created_at, id').all();
  res.json(visible(req.user, rows).map(function (e) {
    return {
      id: e.id, type: e.type, name: e.name, size: e.size, time: e.time,
      role: e.role_id, uploader: e.uploader, auto: !!e.auto,
      taskId: e.task_id, url: e.file_path ? ('/uploads/' + e.file_path) : (e.url || null)
    };
  }));
});

/* 保存 dataURL 图片为真实文件，返回相对路径 */
function saveDataUrl(dataUrl) {
  const m = /^data:image\/(png|jpe?g|gif|webp);base64,(.+)$/i.exec(String(dataUrl || ''));
  if (!m) return null;
  const ext = m[1].toLowerCase() === 'jpeg' ? 'jpg' : m[1].toLowerCase();
  const buf = Buffer.from(m[2], 'base64');
  if (buf.length > 8 * 1024 * 1024) return null;
  const d = new Date();
  const sub = d.getFullYear() + ('0' + (d.getMonth() + 1)).slice(-2);
  const dir = path.join(UPLOAD_DIR, 'auto', sub);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  const fname = Date.now() + '-' + Math.random().toString(36).slice(2, 8) + '.' + ext;
  fs.writeFileSync(path.join(dir, fname), buf);
  return { rel: 'auto/' + sub + '/' + fname, size: fmtSize(buf.length) };
}

/* 登记取证材料（含自动取证截图 dataURL）
 * asAttachment=true 时同时登记为任务附件（用于系统自动取证截图），并按 任务+名称 幂等去重
 */
router.post('/', auth.requireAuth, function (req, res) {
  const b = req.body || {};
  const type = String(b.type || '文档').slice(0, 20);
  const name = String(b.name || '未命名材料').slice(0, 200);
  if (!name) return res.status(400).json({ error: '材料名称不能为空' });

  /* 自动取证：同一任务下同名材料只登记一次 */
  if (b.auto && b.taskId) {
    const exist = db.prepare('SELECT * FROM evidence WHERE task_id=? AND name=? AND auto=1').get(String(b.taskId), name);
    if (exist) {
      return res.json({ ok: true, id: exist.id, url: exist.file_path ? ('/uploads/' + exist.file_path) : null, size: exist.size, deduped: true });
    }
  }

  let rel = null, size = String(b.size || '-');
  if (b.dataUrl) {
    const saved = saveDataUrl(b.dataUrl);
    if (saved) { rel = saved.rel; size = saved.size; }
  }

  const id = 'E' + Date.now() + Math.floor(Math.random() * 100);
  const activeProj = (db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value || null;
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(id, type, name, size, hhmm(), req.user.role.id, req.user.name, req.user.id,
      b.auto ? 1 : 0, b.taskId || null, b.projectId || activeProj, null, rel, now());

  let attId = null;
  if (b.asAttachment && b.taskId) {
    const r = db.prepare('INSERT INTO task_attachments (task_id,name,type,size,url,file_path,uploader,uploader_id,time,auto) VALUES (?,?,?,?,?,?,?,?,?,?)')
      .run(String(b.taskId), name, type, size, rel ? ('/uploads/' + rel) : null, rel,
        b.auto ? '系统自动' : req.user.name, req.user.id, hhmm(), b.auto ? 1 : 0);
    attId = Number(r.lastInsertRowid);
  }

  auth.audit(req, '归档取证材料：' + name);
  res.json({ ok: true, id: id, attId: attId, url: rel ? '/uploads/' + rel : null, size: size });
});

/* 上传真实文件作为取证材料 */
router.post('/upload', auth.requireAuth, upload.array('files', 10), handleUploadError, function (req, res) {
  const files = req.files || [];
  if (!files.length) return res.status(400).json({ error: '未接收到文件' });
  const rel0 = req._uploadRel || 'evidence';
  const activeProj = (db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value || null;
  const out = files.map(function (f) {
    const relPath = rel0 + '/' + f.filename;
    const display = f.filename.replace(/^\d+-[a-z0-9]+-/, '');
    const id = 'E' + Date.now() + Math.floor(Math.random() * 100);
    db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
      .run(id, String((req.body && req.body.type) || '附件'), display, fmtSize(f.size), hhmm(),
        req.user.role.id, req.user.name, req.user.id, 0, (req.body && req.body.taskId) || null,
        activeProj, null, relPath, now());
    return { id: id, name: display, size: fmtSize(f.size), url: '/uploads/' + relPath };
  });
  auth.audit(req, '上传取证材料 ' + out.length + ' 个');
  res.json({ ok: true, files: out });
});

/* 下载（外部单位仅可下载报送类材料） */
router.get('/:id/download', auth.requireAuth, function (req, res) {
  const e = db.prepare('SELECT * FROM evidence WHERE id=?').get(req.params.id);
  if (!e) return res.status(404).json({ error: '材料不存在' });
  if (visible(req.user, [e]).length === 0) {
    auth.audit(req, '尝试下载无权访问的取证材料：' + e.name);
    return res.status(403).json({ error: '当前角色无权访问该材料（外部单位仅可查看医院主动报送的材料）' });
  }
  if (!e.file_path) return res.status(404).json({ error: '该材料为记录类条目，无实体文件' });
  const abs = path.join(UPLOAD_DIR, e.file_path);
  if (!fs.existsSync(abs)) return res.status(404).json({ error: '文件已丢失，请重新上传' });
  auth.audit(req, '下载取证材料：' + e.name);
  res.download(abs, safeName(e.name));
});

module.exports = router;
