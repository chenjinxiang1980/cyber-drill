/* 取证与材料归档接口 */
'use strict';
const express = require('express');
const path = require('node:path');
const fs = require('node:fs');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { makeUploader, handleUploadError, UPLOAD_DIR } = require('../upload');
const { now, hhmm, fmtSize, safeName } = require('../util');
const drillDict = require('../drill_dict');

const router = express.Router();
const upload = makeUploader('evidence');

function roleIdOfTarget(target) {
  const aliases = drillDict.get(db, 'target_role_aliases', {
    '院应急指挥部': 'commander', '信息科值班员': 'duty', '网络安全管理员': 'netsec',
    '系统管理员': 'sysadmin', '信息科负责人': 'itlead',
    '医务科主任': 'medlead', '医务部主任': 'medlead', '门诊部主任': 'medlead', '医务部/门诊部负责人': 'medlead',
    '临床科室': 'clinical', '收费窗口': 'clinical', '临床科室/收费窗口': 'clinical',
    '市卫生健康局': 'healthbureau', '卫健委': 'healthbureau',
    '市委网信办': 'cac', '市网信办': 'cac',
    '市网安大队': 'cyberpolice', '公安网安': 'cyberpolice'
  });
  const t = String(target || '').trim();
  if (aliases[t]) return aliases[t];
  const hit = Object.keys(aliases).filter(function (k) { return t && (t.indexOf(k) >= 0 || k.indexOf(t) >= 0); })[0];
  return hit ? aliases[hit] : '';
}

/* 当前用户是否可为本节点取证（不能由一个角色一次性填充全部材料） */
function canFillNode(user, node) {
  if (!node) return false;
  if (user.role.external) return false;
  const fillRoles = drillDict.get(db, 'fill_roles_by_role', {
    platformadmin: null, commander: null,
    itlead: ['itlead', 'duty', 'netsec', 'sysadmin'],
    duty: ['duty'], netsec: ['netsec'], sysadmin: ['sysadmin'],
    medlead: ['medlead'], clinical: ['clinical'],
    healthbureau: ['healthbureau'], cac: ['cac'], cyberpolice: ['cyberpolice']
  });
  const allow = fillRoles[user.role.id];
  if (!allow) return true;
  return allow.indexOf(roleIdOfTarget(node.target)) >= 0;
}

/* multer 以 latin1 解析文件名，中文需转回 utf8（只解码一次） */
function decodeName(raw) {
  const s = String(raw || 'file');
  const dec = Buffer.from(s, 'latin1').toString('utf8');
  return /\uFFFD/.test(dec) ? s : dec;
}

function visible(user, rows) {
  if (!user.role.external) return rows;
  const extTypes = drillDict.get(db, 'external_evidence_types', ['表单文书', '上报材料', '报案材料']);
  return rows.filter(function (e) {
    return extTypes.indexOf(e.type) >= 0 || ['itlead', 'commander'].indexOf(e.role_id) >= 0;
  });
}

router.get('/', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT e.*, p.name AS project_name FROM evidence e LEFT JOIN projects p ON p.id=e.project_id ORDER BY e.created_at, e.id').all();
  res.json(visible(req.user, rows).map(function (e) {
    return {
      id: e.id, type: e.type, name: e.name, size: e.size, time: e.time,
      role: e.role_id, uploader: e.uploader, auto: !!e.auto,
      taskId: e.task_id, projectId: e.project_id || null, projectName: e.project_name || '',
      note: e.note || '', hasFile: !!e.file_path,
      nodeId: e.node_id || null, reqKey: e.req_key || null,
      url: e.file_path ? ('/uploads/' + e.file_path) : (e.url || null)
    };
  }));
});

/* 当前项目ID（缺省时取活动项目） */
function projectOfBody(body) {
  const pid = (body && body.projectId) ? String(body.projectId) : null;
  if (pid) return db.prepare('SELECT id FROM projects WHERE id=?').get(pid) ? pid : null;
  const r = db.prepare("SELECT value FROM settings WHERE key='active_project'").get();
  return r ? r.value : null;
}

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
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不可归档取证材料（分级权限隔离）' });
  const b = req.body || {};
  const type = String(b.type || '文档').slice(0, 20);
  const name = String(b.name || '未命名材料').slice(0, 200);
  if (!name) return res.status(400).json({ error: '材料名称不能为空' });
  if (b.nodeId) {
    const node = db.prepare('SELECT * FROM script_nodes WHERE id=?').get(String(b.nodeId));
    if (node && !canFillNode(req.user, node)) {
      return res.status(403).json({ error: '该取证材料由「' + (node.target || '其他角色') + '」负责，当前角色不可代为归档' });
    }
  }

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
  const note = String(b.note || '').slice(0, 500);
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at,note,node_id,req_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(id, type, name, size, now().slice(0,16), req.user.role.id, req.user.name, req.user.id,
      b.auto ? 1 : 0, b.taskId || null, projectOfBody(b), null, rel, now(), note,
      b.nodeId || null, b.reqKey || null);

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

/* 按剧本预填材料：仅可预填本角色负责节点的取证材料，一次归档多条
 * 生成与剧本相符的真实模拟文件（截图类 SVG / 日志文本），参演人员可先修改名称与说明
 */
router.post('/prefill', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不可代医院归档取证材料（分级权限隔离）' });
  const b = req.body || {};
  const items = Array.isArray(b.items) ? b.items.slice(0, 30) : [];
  if (!items.length) return res.status(400).json({ error: '未提交预填材料' });
  const pid = projectOfBody(b);
  const out = [];
  const skipped = [];
  const ins = db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at,note,node_id,req_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)');
  items.forEach(function (it, i) {
    const node = it && it.nodeId ? db.prepare('SELECT * FROM script_nodes WHERE id=?').get(String(it.nodeId)) : null;
    if (!node) { skipped.push({ name: (it && it.name) || '', reason: '未关联剧本节点' }); return; }
    if (!canFillNode(req.user, node)) {
      skipped.push({ name: (it && it.name) || '', reason: '该材料由「' + (node.target || '其他角色') + '」负责，不能由当前角色填充' });
      return;
    }
    const idx = Number((it && it.index) || 0);
    const reqItem = parseJson(node.evidence_req, [])[idx];
    if (!reqItem) { skipped.push({ name: (it && it.name) || '', reason: '节点未配置该项取证材料' }); return; }
    const key = node.id + '#' + idx;
    if (db.prepare('SELECT id FROM evidence WHERE project_id=? AND req_key=?').get(pid, key)) {
      skipped.push({ name: reqItem.name, reason: '该材料已归档' });
      return;
    }
    const name = String((it && it.name) || reqItem.name).trim().slice(0, 200);
    if (!name) return;
    const rawNote = String((it && it.note) || reqItem.note || '').trim();
    const note = ('按剧本预填的模拟材料（请核对后按实际处置修改）' + (rawNote ? '；' + rawNote : '')).slice(0, 500);
    const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(node.scenario_id) || null;
    const saved = saveMockFile({ type: reqItem.type, name: name, note: note }, node, sc);
    const id = 'E' + Date.now() + i + Math.floor(Math.random() * 100);
    ins.run(id, reqItem.type, name, saved.size, now().slice(0,16), req.user.role.id, req.user.name, req.user.id,
      0, null, pid, null, saved.rel, now(), note, node.id, key);
    out.push({ id: id, name: name, type: reqItem.type, size: saved.size, url: '/uploads/' + saved.rel });
  });
  if (!out.length) {
    return res.status(400).json({ error: '没有可归档的本角色取证材料' + (skipped.length ? '：' + skipped[0].reason : '') });
  }
  auth.audit(req, '按剧本预填取证材料 ' + out.length + ' 份' + (skipped.length ? ('，跳过 ' + skipped.length + ' 份') : ''));
  res.json({ ok: true, count: out.length, items: out, skipped: skipped });
});

/* 上传真实文件作为取证材料 */
router.post('/upload', auth.requireAuth, function (req, res, next) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不可上传取证材料（分级权限隔离）' });
  upload.array('files', 10)(req, res, function (err) {
    if (err) return handleUploadError(err, req, res, next);
    const files = req.files || [];
    if (!files.length) return res.status(400).json({ error: '未接收到文件' });
    const rel0 = req._uploadRel || 'evidence';
    const nodeId = (req.body && req.body.nodeId) || null;
    if (nodeId) {
      const node = db.prepare('SELECT * FROM script_nodes WHERE id=?').get(String(nodeId));
      if (node && !canFillNode(req.user, node)) {
        files.forEach(function (f) { try { fs.unlinkSync(path.join(UPLOAD_DIR, rel0, f.filename)); } catch (e) {} });
        return res.status(403).json({ error: '该取证材料由「' + (node.target || '其他角色') + '」负责，当前角色不可代为上传' });
      }
    }
    const pid = projectOfBody(req.body || {});
    const note = String((req.body && req.body.note) || '').slice(0, 500);
    const out = files.map(function (f) {
      const relPath = rel0 + '/' + f.filename;
      /* multer 以 latin1 解析原始文件名，中文名需转回 utf8（避免二次解码） */
      const display = decodeName(f.originalname);
      const id = 'E' + Date.now() + Math.floor(Math.random() * 100);
      db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at,note,node_id,req_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
        .run(id, String((req.body && req.body.type) || '附件').slice(0, 20), display, fmtSize(f.size), now().slice(0, 16),
          req.user.role.id, req.user.name, req.user.id, 0, (req.body && req.body.taskId) || null,
          pid, null, relPath, now(), note, (req.body && req.body.nodeId) || null, (req.body && req.body.reqKey) || null);
      return { id: id, name: display, size: fmtSize(f.size), url: '/uploads/' + relPath };
    });
    auth.audit(req, '上传取证材料 ' + out.length + ' 个');
    res.json({ ok: true, files: out });
  });
});

/* ---------- 剧本要求取证材料：清单、一键导入模拟材料 ---------- */
function xmlEsc(s) {
  return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function wrapText(s, n) {
  const out = [];
  let line = '';
  String(s || '').split('').forEach(function (ch) {
    if (ch === '\n' || line.length >= n) { out.push(line); line = ''; if (ch === '\n') return; }
    line += ch;
  });
  if (line) out.push(line);
  return out;
}

/* 按节点与材料项生成模拟材料文件（截图类生成 SVG 图片，其余生成文本材料） */
function buildMockFile(item, node, sc) {
  const stamp = now().slice(0, 16);
  if (item.type === '附件截图') {
    const alertText = (sc && sc.alert) || node.descr || '演练模拟取证截图';
    const lines = wrapText(alertText, 44).slice(0, 4);
    const body = lines.map(function (t, i) {
      return '<text x="44" y="' + (196 + i * 30) + '" fill="#ffd7d5" font-size="18">' + xmlEsc(t) + '</text>';
    }).join('');
    const rows = [
      ['处置节点', node.title + '（' + (node.target || '—') + '）'],
      ['计划时刻', node.time || '—'],
      ['处置状态', '已按剧本执行 · 演练模拟'],
      ['受影响范围', (sc && sc.target) || (sc && sc.type) || '院内业务系统']
    ].map(function (r, i) {
      return '<text x="44" y="' + (372 + i * 32) + '" fill="#8b949e" font-size="16">' + xmlEsc(r[0]) + '：</text>'
        + '<text x="160" y="' + (372 + i * 32) + '" fill="#d5dde8" font-size="16">' + xmlEsc(String(r[1]).slice(0, 60)) + '</text>';
    }).join('');
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="560">'
      + '<rect width="900" height="560" fill="#0d1420"/>'
      + '<rect x="0" y="0" width="900" height="64" fill="#16202f"/>'
      + '<circle cx="30" cy="32" r="7" fill="#f85149"/><circle cx="54" cy="32" r="7" fill="#d29922"/><circle cx="78" cy="32" r="7" fill="#3fb950"/>'
      + '<text x="110" y="40" fill="#8b949e" font-size="18">' + xmlEsc((sc && sc.name ? sc.name + ' · ' : '') + '应急处置取证控制台') + '</text>'
      + '<text x="700" y="40" fill="#6e7681" font-size="15">' + xmlEsc(stamp) + '</text>'
      + '<text x="44" y="108" fill="#f85149" font-size="23" font-weight="bold">[演练模拟] 取证截图 · ' + xmlEsc(item.name) + '</text>'
      + '<rect x="32" y="132" width="836" height="' + (58 + lines.length * 30) + '" rx="6" fill="#2a1416" stroke="#f85149" stroke-opacity="0.5"/>'
      + '<text x="44" y="164" fill="#f85149" font-size="17" font-weight="bold">安全告警 / 现场取证内容</text>'
      + body
      + rows
      + '<text x="44" y="500" fill="#8b949e" font-size="15">责任角色：' + xmlEsc(node.target || '—') + '　材料类型：附件截图　生成时间：' + xmlEsc(stamp) + '</text>'
      + '<text x="44" y="528" fill="#6e7681" font-size="13">本图为演练平台按剧本节点自动生成的模拟取证画面，不含真实业务数据与患者信息。</text>'
      + '</svg>';
    return { buf: Buffer.from(svg, 'utf8'), ext: 'svg', size: fmtSize(Buffer.byteLength(svg)) };
  }
  const text = [
    '# ' + item.name, '',
    '- 材料类型：' + item.type,
    '- 关联节点：' + node.title + '（责任角色：' + (node.target || '—') + '）',
    '- 节点计划时刻：' + (node.time || '—'),
    '- 所属场景：' + ((sc && sc.name) || '—'),
    '- 生成时间：' + stamp, '',
    '## 取证说明', item.note || '按剧本要求归档的模拟取证材料。', '',
    '## 处置要点 / 节点说明', node.guide || node.descr || '按预案执行处置。', ''
  ];
  if (item.type === '操作日志') {
    text.push('## 操作记录片段（演练虚拟）');
    text.push('[' + stamp + '] ' + (node.target || '—') + ' 登录管理终端，执行节点「' + node.title + '」相关处置');
    text.push('[' + stamp + '] 处置动作：' + (node.action || node.title));
    text.push('[' + stamp + '] 执行结果：成功（模拟返回码 0）；已留存操作回显与设备日志');
    text.push('');
  }
  text.push('> 本文件由演练平台按剧本自动生成，仅供演练取证演示，不含真实业务数据或患者信息。');
  const filled = text.join('\n');
  const m = /\.(md|txt|log|csv|json|xml)$/i.exec(item.name || '');
  return { buf: Buffer.from(filled, 'utf8'), ext: m ? m[1].toLowerCase() : (item.type === '样本摘要' ? 'txt' : 'md'), size: fmtSize(Buffer.byteLength(filled)) };
}

/* 生成并落盘剧本模拟材料文件，返回相对路径与大小 */
function saveMockFile(item, node, sc) {
  const mock = buildMockFile(item, node, sc);
  const d = new Date();
  const sub = d.getFullYear() + ('0' + (d.getMonth() + 1)).slice(-2);
  const dir = path.join(UPLOAD_DIR, 'mock', sub);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  const fname = Date.now() + '-' + Math.random().toString(36).slice(2, 8) + '.' + mock.ext;
  fs.writeFileSync(path.join(dir, fname), mock.buf);
  return { rel: 'mock/' + sub + '/' + fname, size: mock.size, ext: mock.ext };
}

/* 当前演练项目需要的取证材料清单（按剧本节点逐项给出归档状态）
 * ?mine=1 仅返回当前角色可负责取证的材料（按剧本责任角色划分）
 */
router.get('/requirements', auth.requireAuth, function (req, res) {
  const pid = req.query.projectId ? String(req.query.projectId)
    : ((db.prepare("SELECT value FROM settings WHERE key='active_project'").get() || {}).value || null);
  const p = pid ? db.prepare('SELECT * FROM projects WHERE id=?').get(pid) : null;
  if (!p) return res.json({ project: null, items: [], done: 0, total: 0, pending: 0, mineDone: 0, mineTotal: 0, minePending: 0 });
  const onlyMine = String(req.query.mine || '') === '1';
  const archived = {};
  db.prepare('SELECT req_key,id,time,url,file_path FROM evidence WHERE project_id=? AND req_key IS NOT NULL').all(p.id)
    .forEach(function (e) { archived[e.req_key] = e; });
  const items = [];
  db.prepare("SELECT * FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq").all(p.scenario_id)
    .forEach(function (n) {
      const reqs = parseJson(n.evidence_req, []);
      const mine = canFillNode(req.user, n);
      if (onlyMine && !mine) return;
      reqs.forEach(function (it, i) {
        const key = n.id + '#' + i;
        const hit = archived[key] || null;
        items.push({
          key: key, nodeId: n.id, nodeTitle: n.title, target: n.target, targetRoleId: roleIdOfTarget(n.target),
          index: i, type: it.type, name: it.name, note: it.note || '', mine: mine,
          done: !!hit, evidenceId: hit ? hit.id : null, doneAt: hit ? hit.time : null,
          hasFile: !!(hit && hit.file_path), url: hit && hit.file_path ? ('/uploads/' + hit.file_path) : null
        });
      });
    });
  const done = items.filter(function (x) { return x.done; }).length;
  const mineItems = items.filter(function (x) { return x.mine; });
  const mineDone = mineItems.filter(function (x) { return x.done; }).length;
  res.json({
    project: { id: p.id, name: p.name, scenarioId: p.scenario_id, status: p.status },
    items: items, done: done, total: items.length, pending: items.length - done,
    mineDone: mineDone, mineTotal: mineItems.length, minePending: mineItems.length - mineDone
  });
});

/* 一键导入剧本相符的模拟取证材料 */
router.post('/mock', auth.requireAuth, function (req, res) {
  if (req.user.role.external) return res.status(403).json({ error: '外部单位不可归档取证材料（分级权限隔离）' });
  const b = req.body || {};
  const nodeId = String(b.nodeId || '');
  const idx = Number(b.index || 0);
  const node = db.prepare('SELECT * FROM script_nodes WHERE id=?').get(nodeId);
  if (!node) return res.status(404).json({ error: '剧本节点不存在' });
  if (!canFillNode(req.user, node)) {
    return res.status(403).json({ error: '该取证材料由「' + (node.target || '其他角色') + '」负责，当前角色不可代为导入' });
  }
  const reqs = parseJson(node.evidence_req, []);
  const item = reqs[idx];
  if (!item) return res.status(404).json({ error: '该节点未配置此项取证材料' });
  const pid = projectOfBody(b);
  const key = node.id + '#' + idx;
  const exist = db.prepare('SELECT * FROM evidence WHERE project_id=? AND req_key=?').get(pid, key);
  if (exist) {
    return res.json({ ok: true, id: exist.id, name: exist.name, deduped: true, url: exist.file_path ? ('/uploads/' + exist.file_path) : null });
  }
  const sc = db.prepare('SELECT * FROM scenarios WHERE id=?').get(node.scenario_id) || null;
  const saved = saveMockFile(item, node, sc);
  const id = 'E' + Date.now() + Math.floor(Math.random() * 100);
  const note = '模拟取证材料（依据剧本节点「' + node.title + '」自动生成，导入后请核对并按实际处置修改）' + (item.note ? '；' + item.note : '');
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at,note,node_id,req_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(id, item.type, item.name, saved.size, now().slice(0, 16), req.user.role.id, req.user.name, req.user.id,
      0, null, pid, null, saved.rel, now(), note.slice(0, 500), node.id, key);
  auth.audit(req, '一键导入模拟取证材料：' + item.name + '（节点：' + node.title + '）');
  res.json({ ok: true, id: id, name: item.name, size: saved.size, url: '/uploads/' + saved.rel });
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

