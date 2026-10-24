/* 任务协同接口：接单、提交处置、完成、附件上传、表单提交 */
'use strict';
const express = require('express');
const path = require('node:path');
const fs = require('node:fs');
const { db, parseJson } = require('../db');
const auth = require('../auth');
const { makeUploader, handleUploadError, MAX_MB } = require('../upload');
const { now, hhmm, fmtSize } = require('../util');

const router = express.Router();
const upload = makeUploader('tasks');

const MANAGED_ROLES = ['itlead', 'duty', 'netsec', 'sysadmin'];

/* 校验任务可见性 */
function taskAccess(user, task) {
  const rid = user.role.id;
  if (rid === 'commander') return true;
  /* 仅信息科负责人可统筹处置本科室条线任务 */
  if (rid === 'itlead') return MANAGED_ROLES.indexOf(task.role_id) >= 0;
  return task.role_id === rid;
}

function loadTask(req, res, next) {
  const t = db.prepare('SELECT * FROM tasks WHERE id=?').get(req.params.id);
  if (!t) return res.status(404).json({ error: '任务不存在' });
  if (!taskAccess(req.user, t)) {
    return res.status(403).json({ error: '当前角色无权操作该任务（分级权限隔离）' });
  }
  req.task = t;
  next();
}

function activeProjectId() {
  const r = db.prepare("SELECT value FROM settings WHERE key='active_project'").get();
  return r ? r.value : null;
}

/* 列表 */
router.get('/', auth.requireAuth, function (req, res) {
  const rows = db.prepare('SELECT * FROM tasks ORDER BY seq').all();
  const atts = db.prepare('SELECT * FROM task_attachments ORDER BY id').all();
  const byTask = {};
  atts.forEach(function (a) { (byTask[a.task_id] = byTask[a.task_id] || []).push(a); });
  res.json(rows.filter(function (t) { return taskAccess(req.user, t); }).map(function (t) {
    return {
      id: t.id, projectId: t.project_id, role: t.role_id, title: t.title, desc: t.descr,
      deadline: t.deadline, status: t.status, level: t.level,
      forms: parseJson(t.forms, []),
      atts: (byTask[t.id] || []).map(function (a) {
        return { id: a.id, name: a.name, type: a.type, size: a.size, url: a.url, by: a.uploader, time: a.time, auto: !!a.auto };
      })
    };
  }));
});

/* 接单：待处理 → 处理中 */
router.post('/:id/accept', auth.requireAuth, loadTask, function (req, res) {
  const t = req.task;
  if (t.status === '已完成') return res.status(400).json({ error: '任务已完成，无需接单' });
  db.prepare("UPDATE tasks SET status='处理中', accepted_at=COALESCE(accepted_at,?) WHERE id=?").run(hhmm(), t.id);
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.project_id, hhmm(), req.user.name, req.user.role.id, '接单处理',
      '任务 ' + t.id + '「' + t.title + '」接单', '', nextSeq(t.project_id));
  auth.audit(req, '任务 ' + t.id + ' 接单');
  res.json({ ok: true, status: '处理中' });
});

/* 提交处置记录（模拟），同时归档取证 */
router.post('/:id/submit', auth.requireAuth, loadTask, function (req, res) {
  const t = req.task;
  const note = String((req.body && req.body.note) || '').slice(0, 500);
  const eid = 'E' + Date.now();
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run(eid, '处置记录', '任务处置记录-' + t.id + '(模拟)', '9 KB', hhmm(), req.user.role.id,
      req.user.name, req.user.id, 0, t.id, t.project_id, null, now());
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.project_id, hhmm(), req.user.name, req.user.role.id, '提交处置记录',
      '任务 ' + t.id + (note ? ('：' + note) : ''), '', nextSeq(t.project_id));
  auth.audit(req, '提交任务 ' + t.id + ' 处置记录');
  res.json({ ok: true, evidenceId: eid });
});

/* 完成任务 */
router.post('/:id/complete', auth.requireAuth, loadTask, function (req, res) {
  const t = req.task;
  const doneAt = hhmm();
  db.prepare("UPDATE tasks SET status='已完成', completed_at=?, accepted_at=COALESCE(accepted_at,?) WHERE id=?").run(doneAt, doneAt, t.id);
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run('E' + Date.now(), '处置记录', '任务处置记录-' + t.id + '(模拟)', '9 KB', hhmm(),
      req.user.role.id, req.user.name, req.user.id, 0, t.id, t.project_id, null, now());
  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.project_id, hhmm(), req.user.name, req.user.role.id, '任务完成',
      '任务 ' + t.id + ' 已完成，处置留痕已归档', '', nextSeq(t.project_id));
  auth.audit(req, '完成任务 ' + t.id);
  res.json({ ok: true, status: '已完成' });
});

/* 删除任务（仅指挥部/信息科负责人，且仅限其可见范围的未归档项目） */
router.delete('/:id', auth.requireAuth, loadTask, function (req, res) {
  const t = req.task;
  const rid = req.user.role.id;
  if (rid !== 'commander' && rid !== 'itlead') {
    return res.status(403).json({ error: '仅院应急指挥部或信息科负责人可删除演练任务' });
  }
  const p = db.prepare('SELECT status FROM projects WHERE id=?').get(t.project_id);
  if (p && p.status === '已归档') {
    return res.status(400).json({ error: '已归档项目的任务不可删除（历史记录已封存）' });
  }

  /* 一并清理任务附件、表单提交与关联取证（含上传的物理文件） */
  const atts = db.prepare('SELECT * FROM task_attachments WHERE task_id=?').all(t.id);
  const evs = db.prepare('SELECT * FROM evidence WHERE task_id=?').all(t.id);
  atts.concat(evs).forEach(function (x) {
    if (x.file_path) {
      try { fs.unlinkSync(path.join(__dirname, '..', '..', 'uploads', x.file_path)); } catch (e) {}
    }
  });
  db.prepare('DELETE FROM task_attachments WHERE task_id=?').run(t.id);
  db.prepare('DELETE FROM form_submissions WHERE task_id=?').run(t.id);
  db.prepare('DELETE FROM evidence WHERE task_id=?').run(t.id);
  db.prepare('DELETE FROM tasks WHERE id=?').run(t.id);

  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.project_id, hhmm(), req.user.name, rid, '删除任务',
      '任务 ' + t.id + '「' + t.title + '」已删除，附件与关联取证一并清理', '', nextSeq(t.project_id));
  auth.audit(req, '删除任务 ' + t.id + '（附件 ' + atts.length + ' 个，取证 ' + evs.length + ' 条）');
  res.json({ ok: true, id: t.id, attachments: atts.length, evidence: evs.length });
});

/* 重置任务（仅平台管理员/指挥部） */
router.post('/reset-all', auth.requireAuth, function (req, res) {
  const rid = req.user.role.id;
  if (rid !== 'platformadmin' && rid !== 'commander') {
    return res.status(403).json({ error: '仅平台管理员或院应急指挥部可重置任务状态' });
  }
  db.prepare("UPDATE tasks SET status='待处理' WHERE status='已完成'").run();
  auth.audit(req, '重置已完成任务状态');
  res.json({ ok: true });
});

/* 附件上传（multipart，最多 10 个） */
router.post('/:id/attachments', auth.requireAuth, loadTask, upload.array('files', 10), handleUploadError,
  function (req, res) {
    const t = req.task;
    const files = req.files || [];
    if (!files.length) return res.status(400).json({ error: '未接收到文件' });
    const rel = req._uploadRel || 'tasks';
    const saved = [];
    files.forEach(function (f) {
      const relPath = rel + '/' + f.filename;
      const url = '/uploads/' + relPath;
      const size = fmtSize(f.size);
      const r = db.prepare('INSERT INTO task_attachments (task_id,name,type,size,url,file_path,uploader,uploader_id,time,auto) VALUES (?,?,?,?,?,?,?,?,?,?)')
        .run(t.id, f.filename.replace(/^\d+-[a-z0-9]+-/, ''), guessType(f.filename), size, url, relPath,
          req.user.name, req.user.id, hhmm(), 0);
      const eid = 'E' + Date.now() + '_' + r.lastInsertRowid;
      db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)')
        .run(eid, guessType(f.filename), f.filename.replace(/^\d+-[a-z0-9]+-/, '') + '（任务' + t.id + '）',
          size, hhmm(), req.user.role.id, req.user.name, req.user.id, 0, t.id, t.project_id, relPath, now());
      saved.push({
        id: Number(r.lastInsertRowid), name: f.filename.replace(/^\d+-[a-z0-9]+-/, ''),
        type: guessType(f.filename), size: size, url: url, by: req.user.name, time: hhmm(), auto: false
      });
    });
    auth.audit(req, '上传证据附件 ' + saved.length + ' 个至任务 ' + t.id);
    res.json({ ok: true, attachments: saved });
  });

/* 附件列表 */
router.get('/:id/attachments', auth.requireAuth, loadTask, function (req, res) {
  const rows = db.prepare('SELECT * FROM task_attachments WHERE task_id=? ORDER BY id').all(req.task.id);
  res.json(rows.map(function (a) {
    return { id: a.id, name: a.name, type: a.type, size: a.size, url: a.url, by: a.uploader, time: a.time, auto: !!a.auto };
  }));
});

/* 删除附件（上传者本人或平台管理员） */
router.delete('/:id/attachments/:attId', auth.requireAuth, loadTask, function (req, res) {
  const a = db.prepare('SELECT * FROM task_attachments WHERE id=? AND task_id=?').get(req.params.attId, req.task.id);
  if (!a) return res.status(404).json({ error: '附件不存在' });
  if (req.user.role.id !== 'platformadmin' && a.uploader_id !== req.user.id) {
    return res.status(403).json({ error: '仅上传者本人或平台管理员可删除附件' });
  }
  if (a.file_path) {
    const abs = path.join(__dirname, '..', '..', 'uploads', a.file_path);
    try { fs.unlinkSync(abs); } catch (e) {}
  }
  db.prepare('DELETE FROM task_attachments WHERE id=?').run(a.id);
  auth.audit(req, '删除任务 ' + req.task.id + ' 附件：' + a.name);
  res.json({ ok: true });
});

/* 表单提交 */
router.post('/:id/forms', auth.requireAuth, loadTask, function (req, res) {
  const t = req.task;
  const formId = String((req.body && req.body.formId) || '');
  const data = (req.body && req.body.data) || {};
  const tpl = db.prepare('SELECT * FROM form_templates WHERE id=?').get(formId);
  if (!tpl) return res.status(400).json({ error: '表单模板不存在：' + formId });
  if (!data || !Object.keys(data).length) return res.status(400).json({ error: '表单内容为空' });

  const r = db.prepare('INSERT INTO form_submissions (task_id,form_id,form_name,data,submitter,user_id,role_id,time) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.id, formId, tpl.name, JSON.stringify(data), req.user.name, req.user.id, req.user.role.id, now());

  const firstTwo = Object.keys(data).slice(0, 2).map(function (k) { return String(data[k]).slice(0, 18); }).join(' / ');
  db.prepare('INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,file_path,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)')
    .run('E' + Date.now(), '表单文书', '《' + tpl.name + '》-' + t.id + '（' + firstTwo + '）',
      '12 KB', hhmm(), req.user.role.id, req.user.name, req.user.id, 0, t.id, t.project_id, null, now());

  db.prepare('INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)')
    .run(t.project_id, hhmm(), req.user.name, req.user.role.id, '提交表单',
      '《' + tpl.name + '》' + (firstTwo ? ('：' + firstTwo) : ''), '', nextSeq(t.project_id));
  auth.audit(req, '提交表单《' + tpl.name + '》（任务 ' + t.id + '）');
  res.json({ ok: true, submissionId: Number(r.lastInsertRowid) });
});

/* 表单提交记录 */
router.get('/:id/forms', auth.requireAuth, loadTask, function (req, res) {
  const rows = db.prepare('SELECT * FROM form_submissions WHERE task_id=? ORDER BY id').all(req.task.id);
  res.json(rows.map(function (r) {
    return { id: r.id, formId: r.form_id, formName: r.form_name, data: parseJson(r.data, {}), submitter: r.submitter, time: r.time };
  }));
});

function nextSeq(projectId) {
  const r = db.prepare('SELECT COALESCE(MAX(seq),0)+1 AS n FROM timeline WHERE project_id=?').get(projectId);
  return r.n;
}

function guessType(n) {
  n = String(n || '').toLowerCase();
  if (/\.(png|jpe?g|gif|bmp|webp)$/.test(n)) return '截图';
  if (/\.(log|txt)$/.test(n)) return '日志片段';
  if (/\.(pcap|cap)$/.test(n)) return '流量包摘要';
  if (/\.(cfg|conf|ini|ya?ml|json)$/.test(n)) return '配置备份';
  return '文档';
}

module.exports = router;
module.exports.MAX_MB = MAX_MB;
