/* 正式版 API 客户端：全部数据经 REST 接口读写（保留原方法名与签名，页面无需改动）
 * 同步方法（curRole / activeProject / drillElapsed）读取已由 DRILL.bootstrap 填充的本地缓存。
 */
window.API = (function () {
  'use strict';

  function curRole() {
    try { return JSON.parse(sessionStorage.getItem('pzy_role') || 'null'); } catch (e) { return null; }
  }
  /* 当前登录用户名（api.js 为独立闭包，不能直接引用 app.js 内的 curUser） */
  function curUser() {
    return sessionStorage.getItem('pzy_user') || '';
  }

  /* ---------- 演练项目 ---------- */
  function activeProject() {
    var id = sessionStorage.getItem('pzy_proj');
    var ps = (window.MOCK && MOCK.PROJECTS) || [];
    return ps.filter(function (p) { return p.id === id; })[0] ||
      ps.filter(function (p) { return p.status === '演练中'; })[0] || ps[0];
  }
  function setActiveProject(id) {
    sessionStorage.setItem('pzy_proj', id);
    return DRILL.post('/projects/' + encodeURIComponent(id) + '/activate');
  }
  function getProjects() { return DRILL.get('/content/projects'); }
  function createProject(payload) {
    return DRILL.post('/content/projects', payload).then(function (r) {
      if (r && r.project) {
        var ps = window.MOCK.PROJECTS || (window.MOCK.PROJECTS = []);
        if (!ps.some(function (p) { return p.id === r.project.id; })) ps.unshift(r.project);
      }
      return r;
    });
  }
  /* 删除演练项目（平台管理员）：服务端清除全部关联数据，本地同步移除 */
  function deleteProject(id) {
    return DRILL.del('/content/projects/' + encodeURIComponent(id)).then(function (r) {
      window.MOCK.PROJECTS = (window.MOCK.PROJECTS || []).filter(function (p) { return p.id !== id; });
      MOCK.TASKS = (MOCK.TASKS || []).filter(function (t) { return t.projectId !== id; });
      MOCK.EVIDENCE = (MOCK.EVIDENCE || []).filter(function (e) { return e.projectId !== id; });
      if (sessionStorage.getItem('pzy_proj') === id) {
        var nx = (window.MOCK.PROJECTS || [])[0];
        if (nx) sessionStorage.setItem('pzy_proj', nx.id);
        else sessionStorage.removeItem('pzy_proj');
      }
      return r;
    });
  }
  function updateProjectMembers(id, members) {
    return DRILL.patch('/content/projects/' + encodeURIComponent(id) + '/members', { members: members }).then(function (r) {
      var p = (MOCK.PROJECTS || []).filter(function (x) { return x.id === id; })[0];
      if (p && r && r.project) p.members = r.project.members;
      return r;
    });
  }
  function updateProjectStatus(id, status) {
    return DRILL.patch('/projects/' + encodeURIComponent(id) + '/status', { status: status }).then(function (r) {
      var p = (MOCK.PROJECTS || []).filter(function (x) { return x.id === id; })[0];
      if (p) {
        if (r && r.project) Object.keys(r.project).forEach(function (k) { p[k] = r.project[k]; });
        else p.status = status;
      }
      return r;
    });
  }
  /* 解析本地时间字符串（YYYY-MM-DD HH:mm[:ss]）为毫秒 */
  function parseLocal(s) {
    var m = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?/.exec(String(s || ''));
    if (!m) return NaN;
    return new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +(m[6] || 0)).getTime();
  }
  /* 真实已进行秒数：未启动返回 null；暂停/结束取冻结时刻；扣除累计暂停时长 */
  function elapsedOf(p) {
    if (!p || !p.startedAt) return null;
    var start = parseLocal(p.startedAt);
    if (!isFinite(start)) return null;
    var end = p.endedAt ? parseLocal(p.endedAt) : (p.pausedAt ? parseLocal(p.pausedAt) : Date.now());
    if (!isFinite(end)) end = Date.now();
    var sec = Math.floor((end - start) / 1000 - (Number(p.pausedSec) || 0));
    return sec > 0 ? sec : 0;
  }
  function fmtElapsed(sec) {
    if (sec === null || sec === undefined) return '00:00:00';
    var s = Math.max(0, Math.floor(Number(sec) || 0));
    var p2 = function (n) { return n < 10 ? '0' + n : '' + n; };
    return p2(Math.floor(s / 3600)) + ':' + p2(Math.floor(s % 3600 / 60)) + ':' + p2(s % 60);
  }
  function drillElapsed() {
    return fmtElapsed(elapsedOf(activeProject()));
  }

  /* ---------- 任务 ---------- */
  function localTask(id) {
    return ((window.MOCK && MOCK.TASKS) || []).filter(function (x) { return x.id === id; })[0] || null;
  }
  function setTaskStatus(id, status) {
    var t = localTask(id);
    if (t) t.status = status;
  }
  function getTasks() { return DRILL.get('/tasks'); }
  function deleteTask(id) {
    return DRILL.del('/tasks/' + encodeURIComponent(id)).then(function (r) {
      MOCK.TASKS = (MOCK.TASKS || []).filter(function (t) { return t.id !== id; });
      return r;
    });
  }
  function completeTask(id) {
    return DRILL.post('/tasks/' + encodeURIComponent(id) + '/complete').then(function (r) { setTaskStatus(id, '已完成'); return r; });
  }
  function resetTasks() {
    return DRILL.post('/tasks/reset-all').then(function (r) {
      (MOCK.TASKS || []).forEach(function (t) { if (t.status === '已完成') t.status = '待处理'; });
      return r;
    });
  }
  function acceptTask(id) {
    return DRILL.post('/tasks/' + encodeURIComponent(id) + '/accept').then(function (r) { setTaskStatus(id, '处理中'); return r; });
  }
  function submitTask(id, note) {
    return DRILL.post('/tasks/' + encodeURIComponent(id) + '/submit', { note: note || '' });
  }
  function getTaskAttachments(id) { return DRILL.get('/tasks/' + encodeURIComponent(id) + '/attachments'); }
  function uploadTaskAttachments(id, formData) {
    return DRILL.upload('/tasks/' + encodeURIComponent(id) + '/attachments', formData).then(function (r) {
      var t = localTask(id);
      if (t && r && r.attachments) t.atts = (t.atts || []).concat(r.attachments);
      return r;
    });
  }
  function deleteTaskAttachment(id, attId) {
    return DRILL.del('/tasks/' + encodeURIComponent(id) + '/attachments/' + encodeURIComponent(attId)).then(function (r) {
      var t = localTask(id);
      if (t) t.atts = (t.atts || []).filter(function (a) { return String(a.id) !== String(attId); });
      return r;
    });
  }
  function submitForm(id, formId, data) {
    return DRILL.post('/tasks/' + encodeURIComponent(id) + '/forms', { formId: formId, data: data });
  }

  /* ---------- 会话 ---------- */
  function login(username, password, role) {
    return DRILL.post('/auth/login', { username: username, password: password, role: role || '' }).then(function (r) {
      DRILL.setToken(r.token);
      sessionStorage.setItem('pzy_role', JSON.stringify(r.role));
      sessionStorage.setItem('pzy_user', r.user.name);
      sessionStorage.setItem('pzy_org', r.user.org || '盘州市人民医院');
      return r;
    });
  }
  function logout() {
    return DRILL.post('/auth/logout').then(function () {
      DRILL.setToken('');
      sessionStorage.removeItem('pzy_role');
      sessionStorage.removeItem('pzy_user');
      sessionStorage.removeItem('pzy_org');
      sessionStorage.removeItem('pzy_proj');
    });
  }
  function changePassword(oldPassword, newPassword) {
    return DRILL.post('/auth/change-password', { oldPassword: oldPassword, newPassword: newPassword });
  }
  function me() { return DRILL.get('/auth/me'); }

  /* ---------- 消息 / 通知 ---------- */
  function getMessages() { return DRILL.get('/content/messages'); }
  function getNotifs() { return DRILL.get('/content/notifs'); }
  function readNotifs() {
    return DRILL.post('/content/notifs/read', {}).then(function (r) {
      (MOCK.NOTIFS || []).forEach(function (n) { n.read = true; });
      return r;
    });
  }
  function pushMessage(msg) {
    return DRILL.post('/content/messages', msg).then(function (r) {
      (MOCK.MESSAGES = MOCK.MESSAGES || []).push({
        time: new Date().toTimeString().slice(0, 5), level: msg.level || 'P2',
        source: msg.source || '', text: msg.text || '', targets: msg.targets || ['*']
      });
      return r;
    });
  }

  /* ---------- 演练导调中心（院内演练会商 / 院外上报沟通） ---------- */
  function getChatChannels() { return DRILL.get('/content/chat/channels'); }
  function getChat(group) { return DRILL.get('/content/chat/' + encodeURIComponent(group)); }
  function uploadChatAttachment(group, formData) {
    return DRILL.upload('/content/chat/' + encodeURIComponent(group) + '/upload', formData);
  }
  function sendChat(group, text, att, filePath, attSize) {
    return DRILL.post('/content/chat/' + encodeURIComponent(group), {
      text: text || '', att: att || null, filePath: filePath || null, attSize: attSize || null
    });
  }
  /* 演练节点操作情况（按频道权限过滤：院内看全部，院外仅上报沟通阶段） */
  function getDrillProgress(channel) {
    return DRILL.get('/content/drill-progress?channel=' + encodeURIComponent(channel || 'inner'));
  }
  /* 打开处置表单即标记该节点为处置中 */
  function startNode(nodeId) {
    return DRILL.post('/drill/nodes/' + encodeURIComponent(nodeId) + '/start');
  }

  /* ---------- 表单 / 场景 / 剧本 ---------- */
  function getForms() { return Promise.resolve((window.MOCK && MOCK.FORM_TEMPLATES) || []); }
  function getScenarios() { return DRILL.get('/content/scenarios'); }
  function getScriptNodes() { return DRILL.get('/content/script-nodes'); }
  function saveScenario(id, patch) {
    return DRILL.patch('/content/scenarios/' + encodeURIComponent(id), patch).then(function (r) {
      var s = (MOCK.SCENARIOS || []).filter(function (x) { return x.id === id; })[0];
      if (s) Object.keys(patch || {}).forEach(function (k) { if (k !== 'desc' || patch.desc !== undefined) s[k] = patch[k]; });
      return r;
    });
  }
  function addScenario(s) {
    return DRILL.post('/content/scenarios', s).then(function (r) {
      (MOCK.SCENARIOS = MOCK.SCENARIOS || []).push({
        id: r.id, name: s.name, type: s.type || '', level: s.level || '一般',
        status: '草稿', version: 'v1.0',
        updated: new Date().toISOString().slice(0, 10), nodes: 0, desc: s.desc || '', alert: s.alert || ''
      });
      return r;
    });
  }
  function saveScriptNodes(scenarioId, nodes, alert, name, versionBump) {
    return DRILL.post('/content/script-nodes', { scenarioId: scenarioId, nodes: nodes, alert: alert, name: name, versionBump: versionBump || 'minor' }).then(function (r) {
      var s = (MOCK.SCENARIOS || []).filter(function (x) { return x.id === scenarioId; })[0];
      if (s) {
        s.version = r.version;
        s.nodes = r.count;
        s.updated = r.updated;
        if (r.alert !== undefined) s.alert = r.alert;
        if (r.name) s.name = r.name;
      }
      /* 本地节点缓存同步，保证回放/编排切换场景后读到最新编排 */
      MOCK.SCRIPT_NODES = (MOCK.SCRIPT_NODES || []).filter(function (n) { return n.scenarioId !== scenarioId; })
        .concat(nodes.map(function (n) {
          return Object.assign({}, n, { scenarioId: scenarioId });
        }));
      return r;
    });
  }

  /* ---------- 取证 / 快照 / 回放 ---------- */
  function getEvidence() { return DRILL.get('/evidence'); }
  function addEvidence(e) {
    return DRILL.post('/evidence', e).then(function (r) {
      if (r && r.ok && !r.deduped) {
        (MOCK.EVIDENCE = MOCK.EVIDENCE || []).push({
          id: r.id, type: e.type || '文档', name: e.name || '未命名材料', size: r.size || e.size || '-',
          time: new Date().toTimeString().slice(0, 5), role: (curRole() || {}).id, uploader: curUser(), auto: !!e.auto,
          taskId: e.taskId || null, url: r.url || null
        });
      }
      return r;
    });
  }
  function uploadEvidence(formData) {
    return DRILL.upload('/evidence/upload', formData).then(function (r) {
      if (r && r.files) {
        r.files.forEach(function (f) {
          (MOCK.EVIDENCE = MOCK.EVIDENCE || []).push({
            id: f.id, type: (formData.get && formData.get('type')) || '附件', name: f.name, size: f.size,
            time: new Date().toTimeString().slice(0, 5), role: (curRole() || {}).id, uploader: curUser(), auto: false, url: f.url
          });
        });
      }
      return r;
    });
  }
  /* 按剧本预填材料（批量归档） */
  function prefillEvidence(items, projectId) {
    var p = activeProject();
    return DRILL.post('/evidence/prefill', { items: items || [], projectId: projectId || (p && p.id) || null });
  }
  /* 剧本要求取证材料清单（逐项归档状态；mine=true 仅本角色负责的节点） */
  function getEvidenceRequirements(projectId, mine) {
    var p = activeProject();
    var pid = projectId || (p && p.id) || '';
    var qs = [];
    if (pid) qs.push('projectId=' + encodeURIComponent(pid));
    if (mine) qs.push('mine=1');
    return DRILL.get('/evidence/requirements' + (qs.length ? ('?' + qs.join('&')) : ''));
  }
  /* 一键导入与剧本相符的模拟取证材料 */
  function importMockEvidence(nodeId, index, projectId) {
    var p = activeProject();
    return DRILL.post('/evidence/mock', { nodeId: nodeId, index: index, projectId: projectId || (p && p.id) || null });
  }

  /* 下载归档材料（携带令牌的真实文件下载） */
  function downloadEvidence(id, name) {
    return fetch('/api/evidence/' + encodeURIComponent(id) + '/download', {
      headers: { Authorization: 'Bearer ' + DRILL.token() }
    }).then(function (r) {
      if (!r.ok) return r.json().then(function (d) { throw new Error((d && d.error) || '下载失败'); });
      return r.blob();
    }).then(function (blob) {
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name || 'evidence';
      document.body.appendChild(a); a.click(); a.remove();
      setTimeout(function () { URL.revokeObjectURL(a.href); }, 2000);
      return true;
    });
  }
  function getSnapshots() { return DRILL.get('/content/snapshots'); }
  function addSnapshot(name, note) {
    return DRILL.post('/content/snapshots', { name: name, note: note }).then(function (r) {
      (MOCK.SNAPSHOTS = MOCK.SNAPSHOTS || []).push({
        id: r.id, name: name || '手动快照', time: new Date().toTimeString().slice(0, 5), by: curUser(), note: note || ''
      });
      return r;
    });
  }
  function getTimeline() { return DRILL.get('/content/timeline'); }
  function getSeqFlow() { return DRILL.get('/content/seqflow'); }
  function saveSeqFlow(items) { return DRILL.post('/content/seqflow', { items: items }); }

  /* ---------- 评估（系统自动考评） ---------- */
  function getScoreItems() {
    return DRILL.get('/content/evaluation').then(function (d) { return d.items; });
  }
  function getScores() {
    return DRILL.get('/content/evaluation').then(function (d) { return d.scores; });
  }
  function saveScores(items) {
    return DRILL.post('/content/evaluation/scores', { scores: items }).then(function (r) {
      MOCK.SCORES = MOCK.SCORES || [];
      (items || []).forEach(function (it) {
        var s = MOCK.SCORES.filter(function (x) { return x.itemId === it.itemId; })[0];
        if (s) { s.score = it.score; s.max = it.max; s.note = it.note; s.auto = true; }
        else MOCK.SCORES.push({ itemId: it.itemId, score: it.score, max: it.max || 10, note: it.note || '', auto: true });
      });
      return r;
    });
  }
  function saveScore(itemId, score, note) {
    return saveScores([{ itemId: itemId, score: score, max: 10, note: note }]);
  }

  /* ---------- 台账 / 知识库 / 审计 ---------- */
  function getLedger() { return DRILL.get('/content/ledger'); }
  function getKnowledge() { return DRILL.get('/content/knowledge'); }
  function uploadKnowledge(formData) {
    return DRILL.upload('/content/knowledge', formData).then(function (r) {
      if (r && r.doc) (MOCK.KNOWLEDGE = MOCK.KNOWLEDGE || []).push(r.doc);
      return r;
    });
  }
  function deleteKnowledge(id) {
    return DRILL.del('/content/knowledge/' + encodeURIComponent(id)).then(function (r) {
      MOCK.KNOWLEDGE = (MOCK.KNOWLEDGE || []).filter(function (k) { return k.id !== id; });
      return r;
    });
  }
  function updateKnowledge(id, patch) {
    return DRILL.patch('/content/knowledge/' + encodeURIComponent(id), patch).then(function (r) {
      if (r && r.doc) {
        var list = MOCK.KNOWLEDGE || [];
        var k = list.filter(function (x) { return x.id === id; })[0];
        if (k) Object.keys(r.doc).forEach(function (key) { k[key] = r.doc[key]; });
        else list.push(r.doc);
      }
      return r;
    });
  }
  function getAuditLogs() { return DRILL.get('/content/audit'); }
  function addAudit(action) {
    return DRILL.post('/content/audit', { action: action }).then(function (r) {
      (MOCK.AUDIT_LOGS = MOCK.AUDIT_LOGS || []).unshift({
        time: new Date().toISOString().slice(0, 19).replace('T', ' '), user: curUser(),
        role: (curRole() || {}).id, action: action, ip: MOCK.META ? (MOCK.META.ip || '-') : '-'
      });
      return r;
    });
  }

  /* ---------- 业务影响 / 字典 ---------- */
  function getImpact() { return DRILL.get('/content/impact'); }
  function updateImpact(sys, state, pct) {
    return DRILL.patch('/content/impact/' + encodeURIComponent(sys), { state: state, pct: pct }).then(function (r) {
      var it = (MOCK.IMPACT || []).filter(function (x) { return x.sys === sys; })[0];
      if (it) { it.state = state; it.pct = pct; }
      return r;
    });
  }
  function getDict() { return DRILL.get('/content/dict'); }

  /* ---------- 模拟处置动作（统一入口，仅留痕） ---------- */
  function actSim(action, detail) {
    return DRILL.post('/content/audit', {
      action: '执行模拟处置：' + action + (detail ? ('（' + detail + '）') : '')
    });
  }

  /* ---------- 用户 / 角色 ---------- */
  function getUsers() { return DRILL.get('/content/users'); }
  function getRoles() { return DRILL.get('/content/roles'); }

  /* ---------- 系统管理（平台管理员） ---------- */
  function adminUsers() { return DRILL.get('/admin/users'); }
  function addUser(u) {
    return DRILL.post('/admin/users', u).then(function (r) {
      (MOCK.USERS = MOCK.USERS || []).push({
        id: r.id, username: u.username, name: u.name,
        role: (u.roles && u.roles[0]) || u.role, roleName: '',
        roles: (u.roles || [u.role]).slice(), org: u.org || '盘州市人民医院', status: '启用'
      });
      return r;
    });
  }
  function updateUser(id, patch) {
    return DRILL.patch('/admin/users/' + encodeURIComponent(id), patch).then(function (r) {
      var u = (MOCK.USERS || []).filter(function (x) { return x.id === id; })[0];
      if (u) {
        Object.keys(patch || {}).forEach(function (k) { u[k] = patch[k]; });
        if (patch.roles) { u.roles = patch.roles.slice(); u.role = patch.roles[0]; }
      }
      return r;
    });
  }
  function adminRoles() { return DRILL.get('/admin/roles'); }
  function addRole(r) { return DRILL.post('/admin/roles', r); }
  function setRoleStatus(id, status) { return DRILL.api('/admin/roles/' + encodeURIComponent(id) + '/status', { method: 'PATCH', body: { status: status } }); }
  function deleteRole(id) { return DRILL.api('/admin/roles/' + encodeURIComponent(id), { method: 'DELETE' }); }
  function reorderRoles(ids) { return DRILL.api('/admin/roles/order', { method: 'PATCH', body: { ids: ids } }); }
  function updateRole(id, patch) { return DRILL.api('/admin/roles/' + encodeURIComponent(id), { method: 'PUT', body: patch }); }
  function resetUserPassword(id, pwd) { return DRILL.post('/admin/users/' + encodeURIComponent(id) + '/reset-password', { password: pwd }); }
  function adminForms() { return DRILL.get('/admin/forms'); }
  function addFormTemplate(t) {
    return DRILL.post('/admin/forms', t).then(function (r) {
      (MOCK.FORM_TEMPLATES = MOCK.FORM_TEMPLATES || []).push({ id: r.id, name: r.name, fields: r.fields });
      return r;
    });
  }
  function updateFormTemplate(id, payload) {
    return DRILL.api('/admin/forms/' + encodeURIComponent(id), { method: 'PUT', body: payload }).then(function (r) {
      var list = MOCK.FORM_TEMPLATES || [];
      (list || []).forEach(function (x, i) {
        if (x.id === r.template.id) list[i] = r.template;
      });
      return r;
    });
  }
  function backupData() { return DRILL.post('/admin/backup', {}); }
  function listBackups() { return DRILL.get('/admin/backups'); }
  function sysinfo() { return DRILL.get('/admin/sysinfo'); }
  function getDrillReport(projectId) { return DRILL.get('/admin/report/' + encodeURIComponent(projectId)); }
  function markReportExported(projectId, format) {
    return DRILL.post('/admin/report/' + encodeURIComponent(projectId) + '/export', { format: format });
  }

  /* ---------- 系统设置（平台管理员） ---------- */
  function getBrand() { return DRILL.loadBrand(); }
  function saveBrand(payload) { return DRILL.api('/system/brand', { method: 'PUT', body: payload }); }
  function uploadBrandLogo(formData) { return DRILL.upload('/system/logo', formData); }
  function getSystemStatus() { return DRILL.api('/system/status'); }
  function resetSystem() { return DRILL.api('/system/reset', { method: 'POST', body: {} }); }
  function getDict(key) { return DRILL.api('/system/dict/' + encodeURIComponent(key)); }
  function saveDict(key, value) { return DRILL.api('/system/dict/' + encodeURIComponent(key), { method: 'PUT', body: { value: value } }); }

  /* ---------- 演练模式与自动考评（基础版 / 实战版） ---------- */
  function getDrillWorkbench(projectId) { return DRILL.get('/drill/workbench/' + encodeURIComponent(projectId)); }
  function getDrillFlows(projectId) { return DRILL.get('/drill/flows/' + encodeURIComponent(projectId)); }
  function getDrillFaults(projectId) { return DRILL.get('/drill/faults/' + encodeURIComponent(projectId)); }
  function submitDrillFlow(payload) { return DRILL.post('/drill/decide', payload); }
  function skipDrillFlow(projectId, nodeId) { return DRILL.post('/drill/skip', { projectId: projectId, nodeId: nodeId }); }
  function finishDrill(projectId) { return DRILL.post('/drill/finish', { projectId: projectId }); }
  function addDrillStep(step) { return DRILL.post('/drill/steps', step); }
  function deleteDrillStep(id) { return DRILL.del('/drill/steps/' + encodeURIComponent(id)); }
  function recalcDrillScore(projectId) { return DRILL.post('/drill/score/' + encodeURIComponent(projectId) + '/recalc', {}); }

  return {
    login: login, logout: logout, curRole: curRole, changePassword: changePassword, me: me,
    activeProject: activeProject, setActiveProject: setActiveProject,
    getProjects: getProjects, createProject: createProject, deleteProject: deleteProject, updateProjectMembers: updateProjectMembers, updateProjectStatus: updateProjectStatus,
    drillElapsed: drillElapsed, elapsedOf: elapsedOf, fmtElapsed: fmtElapsed,
    getTasks: getTasks, deleteTask: deleteTask, completeTask: completeTask, resetTasks: resetTasks,
    acceptTask: acceptTask, submitTask: submitTask,
    getTaskAttachments: getTaskAttachments, uploadTaskAttachments: uploadTaskAttachments,
    deleteTaskAttachment: deleteTaskAttachment, submitForm: submitForm,
    getMessages: getMessages, getNotifs: getNotifs, readNotifs: readNotifs, pushMessage: pushMessage,
    getChatChannels: getChatChannels, getChat: getChat, sendChat: sendChat, uploadChatAttachment: uploadChatAttachment, getDrillProgress: getDrillProgress, startNode: startNode, getForms: getForms,
    getScenarios: getScenarios, getScriptNodes: getScriptNodes,
    saveScenario: saveScenario, addScenario: addScenario, saveScriptNodes: saveScriptNodes,
    getEvidence: getEvidence, addEvidence: addEvidence, uploadEvidence: uploadEvidence, prefillEvidence: prefillEvidence, downloadEvidence: downloadEvidence, getEvidenceRequirements: getEvidenceRequirements, importMockEvidence: importMockEvidence,
    getSnapshots: getSnapshots, addSnapshot: addSnapshot,
    getTimeline: getTimeline, getSeqFlow: getSeqFlow, saveSeqFlow: saveSeqFlow,
    getScoreItems: getScoreItems, getScores: getScores, saveScore: saveScore, saveScores: saveScores,
    getLedger: getLedger,
    getKnowledge: getKnowledge, uploadKnowledge: uploadKnowledge, deleteKnowledge: deleteKnowledge, updateKnowledge: updateKnowledge, getAuditLogs: getAuditLogs, addAudit: addAudit,
    getImpact: getImpact, updateImpact: updateImpact, getDict: getDict, actSim: actSim,
    getUsers: getUsers, getRoles: getRoles,
    adminUsers: adminUsers, addUser: addUser, updateUser: updateUser, resetUserPassword: resetUserPassword,
    adminRoles: adminRoles, addRole: addRole, updateRole: updateRole,
    setRoleStatus: setRoleStatus, deleteRole: deleteRole, reorderRoles: reorderRoles,
    adminForms: adminForms, addFormTemplate: addFormTemplate, updateFormTemplate: updateFormTemplate,
    backupData: backupData, listBackups: listBackups, sysinfo: sysinfo,
    getBrand: getBrand, saveBrand: saveBrand, uploadBrandLogo: uploadBrandLogo,
    getSystemStatus: getSystemStatus, resetSystem: resetSystem,
    getDict: getDict, saveDict: saveDict,
    getDrillReport: getDrillReport, markReportExported: markReportExported,
    getDrillWorkbench: getDrillWorkbench,
    getDrillFlows: getDrillFlows, getDrillFaults: getDrillFaults,
    submitDrillFlow: submitDrillFlow, skipDrillFlow: skipDrillFlow, finishDrill: finishDrill,
    addDrillStep: addDrillStep, deleteDrillStep: deleteDrillStep, recalcDrillScore: recalcDrillScore
  };
})();
