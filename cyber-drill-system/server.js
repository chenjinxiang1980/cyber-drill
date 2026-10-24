/* 盘州市人民医院网络安全应急演练系统 · 服务端入口 */
'use strict';
const path = require('node:path');
const fs = require('node:fs');
const os = require('node:os');
const express = require('express');

const { migrate, isSeeded, DB_FILE } = require('./src/db');

const app = express();
const PORT = Number(process.env.DRILL_PORT || 8080);
const HOST = process.env.DRILL_HOST || '0.0.0.0';
const PUBLIC_DIR = path.join(__dirname, 'public');
const UPLOAD_DIR = path.join(__dirname, 'uploads');

if (!fs.existsSync(UPLOAD_DIR)) fs.mkdirSync(UPLOAD_DIR, { recursive: true });

/* 首次启动自动初始化数据库 */
migrate();
if (!isSeeded()) {
  console.log('[init] 检测到数据库未初始化，正在导入初始数据…');
  require('./src/seed');
}

app.disable('x-powered-by');
app.set('trust proxy', true);
app.use(express.json({ limit: '32mb' }));
app.use(express.urlencoded({ extended: true, limit: '2mb' }));

/* 安全响应头（零外部依赖） */
app.use(function (req, res, next) {
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('X-Frame-Options', 'SAMEORIGIN');
  res.setHeader('Referrer-Policy', 'same-origin');
  next();
});

/* 静态资源：禁止上传目录被直接浏览，仅允许必要类型 */
app.use('/uploads', express.static(UPLOAD_DIR, {
  index: false,
  setHeaders: function (res) {
    res.setHeader('Content-Disposition', 'inline');
    res.setHeader('X-Content-Type-Options', 'nosniff');
  }
}));
app.use(express.static(PUBLIC_DIR, { index: 'index.html', extensions: ['html'] }));

/* 业务路由 */
app.use('/api/auth', require('./src/routes/auth'));
app.use('/api', require('./src/routes/bootstrap'));
app.use('/api/tasks', require('./src/routes/tasks'));
app.use('/api/evidence', require('./src/routes/evidence'));
app.use('/api/content', require('./src/routes/content'));
app.use('/api/admin', require('./src/routes/admin'));
app.use('/api/drill', require('./src/routes/drill'));

/* 健康检查 */
app.get('/api/health', function (req, res) {
  res.json({ ok: true, service: 'cyber-drill-system', time: new Date().toISOString() });
});

/* 未匹配的 API 返回 JSON 404 */
app.use('/api', function (req, res) {
  res.status(404).json({ error: '接口不存在：' + req.method + ' ' + req.originalUrl });
});

/* 统一错误处理 */
app.use(function (err, req, res, next) {
  console.error('[error]', err && err.stack ? err.stack : err);
  if (res.headersSent) return next(err);
  const status = err.status || 500;
  res.status(status).json({ error: err.message || '服务器内部错误' });
});

function lanAddresses() {
  const out = [];
  const ifaces = os.networkInterfaces();
  Object.keys(ifaces).forEach(function (name) {
    (ifaces[name] || []).forEach(function (i) {
      if (i.family === 'IPv4' && !i.internal) out.push({ name: name, address: i.address });
    });
  });
  return out;
}

app.listen(PORT, HOST, function () {
  console.log('');
  console.log('============================================================');
  console.log('  盘州市人民医院网络安全应急演练系统　服务已启动');
  console.log('============================================================');
  console.log('  本机访问：   http://127.0.0.1:' + PORT);
  const lan = lanAddresses();
  if (lan.length) {
    lan.forEach(function (i) {
      console.log('  局域网访问： http://' + i.address + ':' + PORT + '    (' + i.name + ')');
    });
  } else {
    console.log('  局域网访问： 未检测到可用网卡地址');
  }
  console.log('  数据库文件： ' + DB_FILE);
  console.log('  上传目录：   ' + UPLOAD_DIR);
  console.log('  停止服务：   Ctrl + C');
  console.log('============================================================');
  console.log('');
});
