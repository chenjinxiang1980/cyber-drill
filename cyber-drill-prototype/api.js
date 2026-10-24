/* 盘州市人民医院网络安全应急演练系统 · API 桩（Promise + 模拟延迟；写操作内存态 + 关键状态 localStorage） */
window.API = (function(){
"use strict";
var delay = function(ms){ return new Promise(function(r){ setTimeout(r, ms||160); }); };
function clone(x){ return JSON.parse(JSON.stringify(x)); }

/* 会话 */
function login(roleId, username, org){
  var role = MOCK.ROLES.filter(function(r){ return r.id===roleId; })[0];
  if(!role) return Promise.reject('角色不存在');
  sessionStorage.setItem('pzy_role', JSON.stringify(role));
  sessionStorage.setItem('pzy_user', username||role.short);
  sessionStorage.setItem('pzy_org', org||'盘州市人民医院');
  return delay(120).then(function(){ return role; });
}
function curRole(){ try{ return JSON.parse(sessionStorage.getItem('pzy_role')||'null'); }catch(e){ return null; } }

/* 演练项目 */
var _projects = MOCK.PROJECTS;
function activeProject(){
  var id = localStorage.getItem('pzy_proj');
  return _projects.filter(function(p){ return p.id===id; })[0] || _projects.filter(function(p){ return p.status==='演练中'; })[0] || _projects[0];
}
function setActiveProject(id){ localStorage.setItem('pzy_proj', id); }
function getProjects(){ return delay(200).then(function(){ return clone(_projects); }); }
function updateProjectStatus(id, status){
  var p = _projects.filter(function(x){ return x.id===id; })[0];
  if(p) p.status = status;
  return delay(150).then(function(){ return clone(p); });
}
function drillElapsed(){
  var p = activeProject();
  if(!p || p.status!=='演练中') return '00:00';
  var start = new Date(p.date.replace(/-/g,'/'));
  var mins = Math.max(0, Math.round((Date.now()-start.getTime())/60000));
  var h = Math.floor(mins/60), m = mins%60;
  return (h<10?'0'+h:h)+':'+(m<10?'0'+m:m);
}

/* 任务：按当前角色过滤；外部角色仅见报送类任务；完成状态持久化 */
function getTasks(){
  var r = curRole();
  var done = JSON.parse(localStorage.getItem('pzy_taskdone')||'[]');
  return delay(200).then(function(){
    var list = clone(MOCK.TASKS).filter(function(t){ return r && t.role===r.id; });
    list.forEach(function(t){ if(done.indexOf(t.id)>=0) t.status='已完成'; });
    return list;
  });
}
function completeTask(id){
  var done = JSON.parse(localStorage.getItem('pzy_taskdone')||'[]');
  if(done.indexOf(id)<0) done.push(id);
  localStorage.setItem('pzy_taskdone', JSON.stringify(done));
  return delay(150);
}
function resetTasks(){ localStorage.removeItem('pzy_taskdone'); return delay(50); }

/* 消息 / 通知 */
function getMessages(){ return delay(180).then(function(){ return clone(MOCK.MESSAGES); }); }
function getNotifs(){
  var read = JSON.parse(localStorage.getItem('pzy_notifread')||'[]');
  return delay(120).then(function(){
    return clone(MOCK.NOTIFS).map(function(n){ n.read = read.indexOf(n.id)>=0; return n; });
  });
}
function readNotifs(){
  var read = MOCK.NOTIFS.map(function(n){ return n.id; });
  localStorage.setItem('pzy_notifread', JSON.stringify(read));
  return delay(80);
}
function pushMessage(msg){
  MOCK.MESSAGES.push(Object.assign({ id:'M'+Date.now(), time:new Date().toTimeString().slice(0,5), level:'P2', source:'人工触发（演练模拟）', targets:['*'] }, msg));
  return delay(150);
}

/* 会商 */
function getChat(group){ return delay(160).then(function(){ return clone(MOCK.CHAT[group]||[]); }); }
function sendChat(group, text, att){
  var r = curRole();
  MOCK.CHAT[group].push({ sender: sessionStorage.getItem('pzy_user')||r.short, role:r.id, time:new Date().toTimeString().slice(0,5), text:text, att:att||null });
  return delay(120);
}

/* 表单模板 / 场景 / 剧本 */
function getForms(){ return delay(120).then(function(){ return clone(MOCK.FORM_TEMPLATES); }); }
function getScenarios(){ return delay(200).then(function(){ return clone(MOCK.SCENARIOS); }); }
function getScriptNodes(){ return delay(160).then(function(){ return clone(MOCK.SCRIPT_NODES); }); }
function saveScenario(id, patch){
  var s = MOCK.SCENARIOS.filter(function(x){ return x.id===id; })[0];
  if(s) Object.assign(s, patch, { updated:new Date().toISOString().slice(0,10) });
  return delay(180);
}
function addScenario(s){
  s.id = 'S'+String(MOCK.SCENARIOS.length+1).padStart(2,'0');
  MOCK.SCENARIOS.push(s);
  return delay(200).then(function(){ return clone(s); });
}

/* 取证 / 快照 / 回放 */
function getEvidence(){ return delay(180).then(function(){ return clone(MOCK.EVIDENCE); }); }
function addEvidence(e){
  var r = curRole();
  MOCK.EVIDENCE.push(Object.assign({ id:'E'+Date.now(), time:new Date().toTimeString().slice(0,5), role:r.id, uploader:sessionStorage.getItem('pzy_user')||r.short, auto:false }, e));
  return delay(150);
}
function getSnapshots(){ return delay(140).then(function(){ return clone(MOCK.SNAPSHOTS); }); }
function addSnapshot(name, note){
  var r = curRole();
  MOCK.SNAPSHOTS.push({ id:'SP'+Date.now(), name:name, time:new Date().toTimeString().slice(0,5), by:r.short, note:note });
  return delay(160);
}
function getTimeline(){ return delay(220).then(function(){ return clone(MOCK.TIMELINE); }); }
function getSeqFlow(){ return delay(150).then(function(){ return clone(MOCK.SEQ_FLOW); }); }

/* 评估 */
function getScoreItems(){ return delay(150).then(function(){ return clone(MOCK.SCORE_ITEMS); }); }
function getScores(){ return delay(160).then(function(){ return clone(MOCK.SCORES); }); }
function saveScore(itemId, score, note){
  var s = MOCK.SCORES.filter(function(x){ return x.itemId===itemId; })[0];
  if(s){ s.score=score; s.note=note; } else { MOCK.SCORES.push({ itemId:itemId, score:score, note:note, max:(MOCK.SCORE_ITEMS.filter(function(i){return i.id===itemId;})[0]||{}).weight||10 }); }
  return delay(140);
}

/* 台账 / 舆情 / 知识库 / 审计 */
function getLedger(){ return delay(200).then(function(){ return clone(MOCK.LEDGER); }); }
function getOpinions(){ return delay(160).then(function(){ return clone(MOCK.OPINION); }); }
function addOpinion(o){ MOCK.OPINION.push(Object.assign({ id:'O'+Date.now(), time:new Date().toTimeString().slice(0,5), status:'待研判', source:'人工发布（演练模拟）' }, o)); return delay(150); }
function updateOpinion(id, patch){ var o=MOCK.OPINION.filter(function(x){return x.id===id;})[0]; if(o) Object.assign(o,patch); return delay(120); }
function getKnowledge(){ return delay(160).then(function(){ return clone(MOCK.KNOWLEDGE); }); }
function getAuditLogs(){ return delay(200).then(function(){ return clone(MOCK.AUDIT_LOGS); }); }
function addAudit(action){
  var r = curRole()||{name:'匿名',id:'-'};
  MOCK.AUDIT_LOGS.push({ time:new Date().toISOString().slice(0,16).replace('T',' '), user:r.name, role:r.id, action:action, ip:'10.30.1.1' });
  return delay(80);
}

/* 业务影响 / 字典 */
function getImpact(){ return delay(140).then(function(){ return clone(MOCK.IMPACT); }); }
function updateImpact(sys, state, pct){ var i=MOCK.IMPACT.filter(function(x){return x.sys===sys;})[0]; if(i){i.state=state;i.pct=pct;} return delay(120); }
function getDict(){ return delay(60).then(function(){ return clone(MOCK.DICT); }); }

/* 模拟处置动作（统一入口，仅记录） */
function actSim(action, detail){
  addAudit('执行模拟处置：'+action+(detail?('（'+detail+'）'):''));
  return delay(260);
}

/* 用户/角色管理 */
function getUsers(){ return delay(150).then(function(){ return clone(MOCK.USERS); }); }
function getRoles(){ return delay(120).then(function(){ return clone(MOCK.ROLES); }); }

return { login:login, curRole:curRole, activeProject:activeProject, setActiveProject:setActiveProject,
  getProjects:getProjects, updateProjectStatus:updateProjectStatus, drillElapsed:drillElapsed,
  getTasks:getTasks, completeTask:completeTask, resetTasks:resetTasks,
  getMessages:getMessages, getNotifs:getNotifs, readNotifs:readNotifs, pushMessage:pushMessage,
  getChat:getChat, sendChat:sendChat, getForms:getForms,
  getScenarios:getScenarios, getScriptNodes:getScriptNodes, saveScenario:saveScenario, addScenario:addScenario,
  getEvidence:getEvidence, addEvidence:addEvidence, getSnapshots:getSnapshots, addSnapshot:addSnapshot,
  getTimeline:getTimeline, getSeqFlow:getSeqFlow,
  getScoreItems:getScoreItems, getScores:getScores, saveScore:saveScore,
  getLedger:getLedger, getOpinions:getOpinions, addOpinion:addOpinion, updateOpinion:updateOpinion,
  getKnowledge:getKnowledge, getAuditLogs:getAuditLogs, addAudit:addAudit,
  getImpact:getImpact, updateImpact:updateImpact, getDict:getDict, actSim:actSim,
  getUsers:getUsers, getRoles:getRoles };
})();
