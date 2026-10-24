/* 文件上传配置（multer）：限制类型与大小，禁止可执行/脚本类样本 */
'use strict';
const path = require('node:path');
const fs = require('node:fs');
const multer = require('multer');
const { safeName } = require('./util');

const UPLOAD_DIR = path.join(__dirname, '..', 'uploads');
const MAX_MB = Number(process.env.DRILL_MAX_UPLOAD_MB || 20);

/* 禁止上传的可执行/脚本扩展名（符合"禁止上传真实恶意样本"约束） */
const BAD_EXT = /\.(exe|bat|cmd|vbs|scr|msi|com|pif|jar|ps1|sh|dll|sys|lnk)$/i;

function destFor(kind) {
  const d = new Date();
  const sub = d.getFullYear() + ('0' + (d.getMonth() + 1)).slice(-2);
  const dir = path.join(UPLOAD_DIR, kind, sub);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  return { dir: dir, rel: path.join(kind, sub).replace(/\\/g, '/') };
}

function makeUploader(kind) {
  const storage = multer.diskStorage({
    destination: function (req, file, cb) {
      const t = destFor(kind);
      req._uploadRel = t.rel;
      cb(null, t.dir);
    },
    filename: function (req, file, cb) {
      /* multer 以 latin1 解析原始文件名，中文名需转回 utf8 */
      let orig = file.originalname || 'file';
      try { orig = Buffer.from(orig, 'latin1').toString('utf8'); } catch (e) {}
      const name = safeName(orig);
      cb(null, Date.now() + '-' + Math.random().toString(36).slice(2, 8) + '-' + name);
    }
  });

  return multer({
    storage: storage,
    limits: { fileSize: MAX_MB * 1024 * 1024, files: 10 },
    fileFilter: function (req, file, cb) {
      let orig = file.originalname || '';
      try { orig = Buffer.from(orig, 'latin1').toString('utf8'); } catch (e) {}
      if (BAD_EXT.test(orig)) {
        const err = new Error('禁止上传可执行程序/脚本类文件：' + orig);
        err.status = 400;
        return cb(err);
      }
      cb(null, true);
    }
  });
}

/* multer 错误转为友好提示 */
function handleUploadError(err, req, res, next) {
  if (!err) return next();
  if (err.code === 'LIMIT_FILE_SIZE') {
    return res.status(400).json({ error: '单个文件不能超过 ' + MAX_MB + 'MB' });
  }
  if (err.code === 'LIMIT_FILE_COUNT') {
    return res.status(400).json({ error: '单次最多上传 10 个文件' });
  }
  if (err.code === 'LIMIT_UNEXPECTED_FILE') {
    return res.status(400).json({ error: '不支持的文件字段：' + err.field });
  }
  return res.status(err.status || 500).json({ error: err.message || '上传失败' });
}

module.exports = { makeUploader, handleUploadError, UPLOAD_DIR, MAX_MB, BAD_EXT };
