/* 盘州市人民医院网络安全应急演练系统 · App Shell / 图标 / 通用组件（共享骨架，禁止各页自写导航） */
(function(){
"use strict";

/* ---------- 内联 SVG 图标（无外部图标库） ---------- */
var P = 'stroke="currentColor" fill="none" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"';
var ICONS = {
  shield:'<path d="M12 3l7 3v5c0 4.5-3 8-7 10-4-2-7-5.5-7-10V6l7-3z" '+P+'/><path d="M9.5 12l2 2 4-4" '+P+'/>',
  dashboard:'<rect x="3" y="3" width="8" height="10" rx="1.5" '+P+'/><rect x="13" y="3" width="8" height="6" rx="1.5" '+P+'/><rect x="13" y="11" width="8" height="10" rx="1.5" '+P+'/><rect x="3" y="15" width="8" height="6" rx="1.5" '+P+'/>',
  screen:'<rect x="2.5" y="4" width="19" height="12.5" rx="2" '+P+'/><path d="M9.5 20.5h5M12 16.5v4" '+P+'/><path d="M6 11.5h2.3l1.5-3.2 2 6 1.5-2.8H18" '+P+'/>',
  tasks:'<path d="M9 6h11M9 12h11M9 18h11" '+P+'/><path d="M4 6l1.5 1.5L8 5" '+P+'/><circle cx="5.5" cy="12" r="1.5" '+P+'/><circle cx="5.5" cy="18" r="1.5" '+P+'/>',
  workbench:'<rect x="3" y="4" width="18" height="12" rx="2" '+P+'/><path d="M8 20h8M12 16v4" '+P+'/>',
  chat:'<path d="M21 12a8 8 0 0 1-8 8H4l2-3a8 8 0 1 1 15-5z" '+P+'/>',
  scenarios:'<path d="M4 5a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" '+P+'/><path d="M8 8h8M8 12h8M8 16h5" '+P+'/>',
  flow:'<circle cx="6" cy="6" r="2.5" '+P+'/><circle cx="18" cy="6" r="2.5" '+P+'/><circle cx="12" cy="18" r="2.5" '+P+'/><path d="M8.5 6H15M7 8l4 8M17 8l-4 8" '+P+'/>',
  projects:'<rect x="3" y="7" width="18" height="13" rx="2" '+P+'/><path d="M8 7V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" '+P+'/>',
  evidence:'<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" '+P+'/><path d="M14 3v5h5" '+P+'/><circle cx="11.5" cy="13.5" r="2.5" '+P+'/><path d="M13.3 15.3l1.7 1.7" '+P+'/>',
  replay:'<circle cx="12" cy="12" r="9" '+P+'/><path d="M12 7v5l3.5 2" '+P+'/>',
  score:'<path d="M12 3l2.6 5.6 6 .8-4.4 4.2 1.1 6-5.3-2.9L6.7 19.6l1.1-6L3.4 9.4l6-.8z" '+P+'/>',
  ledger:'<path d="M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z" '+P+'/><path d="M7 8h10M7 12h10M7 16h6" '+P+'/>',
  opinion:'<path d="M3 11l18-5v12L3 13v-2z" '+P+'/><path d="M11.6 16.8a3 3 0 1 1-5.8-1.5" '+P+'/>',
  knowledge:'<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V4H6.5A2.5 2.5 0 0 0 4 6.5v13z" '+P+'/><path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5" '+P+'/>',
  admin:'<circle cx="12" cy="12" r="3" '+P+'/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h0a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5h0a1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v0a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" '+P+'/>',
  bell:'<path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" '+P+'/><path d="M13.7 21a2 2 0 0 1-3.4 0" '+P+'/>',
  logout:'<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" '+P+'/><path d="M16 17l5-5-5-5M21 12H9" '+P+'/>',
  search:'<circle cx="11" cy="11" r="7" '+P+'/><path d="M21 21l-4.3-4.3" '+P+'/>',
  plus:'<path d="M12 5v14M5 12h14" '+P+'/>',
  file:'<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" '+P+'/><path d="M14 3v5h5" '+P+'/>',
  upload:'<path d="M4 14v5a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5" '+P+'/><path d="M12 3v12M7 8l5-5 5 5" '+P+'/>',
  download:'<path d="M4 14v5a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5" '+P+'/><path d="M12 3v12M7 10l5 5 5-5" '+P+'/>',
  play:'<path d="M6 4l14 8-14 8z" '+P+'/>',
  pause:'<path d="M7 4v16M17 4v16" '+P+' stroke-width="3"/>',
  check:'<path d="M5 13l4 4L19 7" '+P+' stroke-width="2.4"/>',
  x:'<path d="M6 6l12 12M18 6L6 18" '+P+'/>',
  clock:'<circle cx="12" cy="12" r="9" '+P+'/><path d="M12 7v5l3 2" '+P+'/>',
  alert:'<path d="M12 3l10 18H2z" '+P+'/><path d="M12 10v5M12 18.5v.01" '+P+'/>',
  lock:'<rect x="5" y="11" width="14" height="9" rx="2" '+P+'/><path d="M8 11V7a4 4 0 1 1 8 0v4" '+P+'/>',
  user:'<circle cx="12" cy="8" r="4" '+P+'/><path d="M4 21c0-4 3.6-6.5 8-6.5s8 2.5 8 6.5" '+P+'/>',
  eye:'<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" '+P+'/><circle cx="12" cy="12" r="3" '+P+'/>',
  edit:'<path d="M12 20h9" '+P+'/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z" '+P+'/>',
  copy:'<rect x="9" y="9" width="12" height="12" rx="2" '+P+'/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" '+P+'/>',
  trash:'<path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" '+P+'/>',
  camera:'<path d="M3 8a2 2 0 0 1 2-2h2l2-2h6l2 2h2a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" '+P+'/><circle cx="12" cy="13" r="3.5" '+P+'/>',
  power:'<path d="M12 3v9" '+P+' stroke-width="2.4"/><path d="M6.3 6.3a8 8 0 1 0 11.4 0" '+P+'/>',
  net:'<circle cx="12" cy="12" r="9" '+P+'/><path d="M3 12h18M12 3c2.5 2.6 3.8 5.7 3.8 9S14.5 18.4 12 21c-2.5-2.6-3.8-5.7-3.8-9S9.5 5.6 12 3z" '+P+'/>',
  db:'<ellipse cx="12" cy="5.5" rx="8" ry="3" '+P+'/><path d="M4 5.5v13c0 1.7 3.6 3 8 3s8-1.3 8-3v-13" '+P+'/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" '+P+'/>',
  hospital:'<rect x="4" y="7" width="16" height="14" rx="1.5" '+P+'/><path d="M12 7V4h0M9 12h6M12 9v6" '+P+'/>',
  send:'<path d="M22 2L11 13M22 2l-7 20-4-9-9-4z" '+P+'/>',
  archive:'<rect x="3" y="4" width="18" height="5" rx="1" '+P+'/><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4" '+P+'/>',
  refresh:'<path d="M21 12a9 9 0 1 1-2.6-6.3" '+P+'/><path d="M21 3v6h-6" '+P+'/>',
  filter:'<path d="M3 5h18l-7 8v5l-4 2v-7z" '+P+'/>',
  building:'<rect x="5" y="3" width="14" height="18" rx="1" '+P+'/><path d="M9 7h2M13 7h2M9 11h2M13 11h2M9 15h2M13 15h2M10 21v-3h4v3" '+P+'/>',
  github:'<path d="M12 2C6.48 2 2 6.48 2 12c0 4.42 2.87 8.17 6.84 9.5.5.09.68-.22.68-.48v-1.7c-2.78.6-3.37-1.34-3.37-1.34-.45-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.89 1.53 2.34 1.09 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.56-1.11-4.56-4.93 0-1.09.39-1.98 1.03-2.68-.1-.25-.45-1.27.1-2.65 0 0 .84-.27 2.75 1.02a9.5 9.5 0 0 1 2.5-.34c.85 0 1.7.12 2.5.34 1.91-1.29 2.75-1.02 2.75-1.02.55 1.38.2 2.4.1 2.65.64.7 1.03 1.59 1.03 2.68 0 3.83-2.34 4.68-4.57 4.92.36.31.68.92.68 1.85v2.74c0 .27.18.58.69.48A10 10 0 0 0 22 12c0-5.52-4.48-10-10-10z" fill="currentColor" stroke="none"/>',
  gitee:'<circle cx="12" cy="12" r="9.2" fill="currentColor" stroke="none"/><path d="M16.2 9.1h-5.6a2.5 2.5 0 0 0 0 5h2.7v2.3H9.2" fill="none" stroke="#0b1220" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/>'
};
function icon(name, cls){ return '<svg viewBox="0 0 24 24" class="'+(cls||'')+'">'+(ICONS[name]||ICONS.file)+'</svg>'; }

/* ---------- 导航配置（唯一来源，按角色 nav 过滤） ---------- */
var NAV_GROUPS = [
  { title:'演练执行', items:[
    {key:'dashboard', icon:'dashboard', label:'态势总看板', href:'dashboard.html'},
    {key:'screen', icon:'screen', label:'演练监视大屏', href:'screen.html'},
    {key:'workbench', icon:'workbench', label:'角色工作台', href:'workbench.html'},
    {key:'collab', icon:'chat', label:'演练导调中心', href:'collab.html'}
  ]},
  { title:'编排与资源', items:[
    {key:'scenarios', icon:'scenarios', label:'场景剧本库', href:'scenarios.html'},
    {key:'script-editor', icon:'flow', label:'剧本可视化编排', href:'script-editor.html'},
    {key:'projects', icon:'projects', label:'演练项目管理', href:'projects.html'},
    {key:'evidence', icon:'evidence', label:'取证与材料归档', href:'evidence.html'},
    {key:'knowledge', icon:'knowledge', label:'预案知识库', href:'knowledge.html'},
    {key:'opinion', icon:'opinion', label:'公告与舆情模拟', href:'opinion.html'}
  ]},
  { title:'复盘与报送', items:[
    {key:'replay', icon:'replay', label:'演练时序回放', href:'replay.html'},
    {key:'evaluation', icon:'score', label:'复盘评估', href:'evaluation.html'},
    {key:'report', icon:'file', label:'演练报告导出', href:'report.html'},
    {key:'ledger', icon:'ledger', label:'台账与统计', href:'ledger.html'}
  ]},
  { title:'系统管理', items:[
    {key:'admin', icon:'admin', label:'系统基础管理', href:'admin.html'}
  ]}
];

/* ---------- 会话与守卫 ---------- */
function curRole(){ try{ return JSON.parse(sessionStorage.getItem('pzy_role')||'null'); }catch(e){ return null; } }
function curUser(){ return sessionStorage.getItem('pzy_user')||''; }
function isLogin(){ return !!curRole(); }
function guard(pageKey){
  var r = curRole();
  if(!r){ location.href='index.html'; return false; }
  var nav = r.nav||[];
  if(pageKey!=='index' && pageKey!=='password' && nav.indexOf(pageKey)<0){ return 'forbidden'; }
  return true;
}

/* ---------- Shell 渲染 ---------- */
function mountShell(pageKey){
  var r = curRole()||{name:'未登录'};
  var nav = r.nav||[];
  var B = DRILL.brand()||{};
  var brandLogo = B.logo || 'img/logo.png';
  var brandSystem = B.systemName || '网络安全应急演练';
  var brandUnit = B.unitName || '盘州市人民医院';
  var brandVer = B.version || 'v0.1.6';
  var brandGh = B.github || 'https://github.com/chenjinxiang1980/cyber-drill';
  var brandGe = B.gitee || 'https://gitee.com/habeas/cyber-drill';
  var html = '<aside class="sidebar"><div class="brand"><span class="logo brand-logo"><img src="'+brandLogo+'" alt="'+App.esc(brandUnit)+'院徽"></span><span class="bt"><div class="t1">'+App.esc(brandSystem)+'</div><div class="t2">'+App.esc(brandUnit)+' · DRILL</div></span></div><nav class="nav">';
  NAV_GROUPS.forEach(function(g){
    var items = g.items.filter(function(it){ return nav.indexOf(it.key)>=0; });
    if(!items.length) return;
    html += '<div class="nav-group"><div class="g-title">'+g.title+'</div>';
    items.forEach(function(it){
      var active = it.key===pageKey ? ' active':'';
      var badge = '';
      html += '<a class="nav-item'+active+'" href="'+it.href+'">'+icon(it.icon)+'<span>'+it.label+'</span>'+badge+'</a>';
    });
    html += '</div>';
  });
  html += '</nav><div class="side-foot"><div class="u-role">'+r.name+'</div><div class="u-name">'+curUser()+'</div><div class="u-out" onclick="App.logout()">'+icon('logout')+'<span>退出登录</span></div>'
    + '<div class="u-meta"><span class="ver" data-brand="version">'+App.esc(brandVer)+'</span>'
    + '<a class="lnk" data-brand="github" href="'+App.esc(brandGh)+'" target="_blank" rel="noopener" title="GitHub 项目地址">'+icon('github')+'</a>'
    + '<a class="lnk" data-brand="gitee" href="'+App.esc(brandGe)+'" target="_blank" rel="noopener" title="Gitee 项目地址">'+icon('gitee')+'</a>'
    + '</div></div></aside>';

  html += '<div class="main-wrap"><header class="topbar">'
    + '<span class="proj-sel" onclick="App.switchProject()">'+icon('projects')+'<b id="topProj"></b><span class="dim">切换 ▾</span></span>'
    + '<span id="topLive"></span>'
    + '<span class="right tb-clock" id="topClock"></span>'
    + '<span class="bell" onclick="App.showNotifs()">'+icon('bell')+'<span class="n" id="bellN">0</span></span>'
    + '<span class="role-chip"><span class="av">'+(r.name||'?').slice(0,1)+'</span>'+r.name+'</span>'
    + '<span class="tb-out" onclick="location.href=\'password.html\'">'+icon('lock')+'修改密码</span>'
    + '<span class="tb-out" onclick="App.logout()">'+icon('logout')+'退出</span>'
    + '</header><main id="page"></main></div>';
  document.body.insertAdjacentHTML('afterbegin', html);

  // 时钟（立即启动，不依赖数据）
  function tick(){ var d=new Date(); var el=document.getElementById('topClock'); if(el) el.textContent = d.toLocaleDateString('zh-CN')+' '+d.toTimeString().slice(0,8); }
  tick(); setInterval(tick,1000);
}

/* 数据加载完成后刷新顶栏与徽标（bootstrap 之后调用） */
function refreshShell(){
  var r = curRole()||{};
  var proj = API.activeProject();

  // 角色名称（管理员修改角色组名称后同步，含账号角色标签与侧栏）
  if(r.name){
    var chip = document.querySelector('.role-chip');
    if(chip) chip.innerHTML = '<span class="av">'+App.esc(r.name.slice(0,1))+'</span>'+App.esc(r.name);
    var urole = document.querySelector('.side-foot .u-role');
    if(urole) urole.textContent = r.name;
  }

  var tp = document.getElementById('topProj');
  if(tp) tp.textContent = proj ? proj.name : '未选择演练项目';

  var live = document.getElementById('topLive');
  if(live){
    if(proj && proj.status==='演练中'){
      live.innerHTML = '<span class="live-chip"><span class="dot"></span>演练进行中 · '+proj.level+'级事件 · T+'+API.drillElapsed()+'</span>';
    } else {
      live.innerHTML = '<span class="live-chip none"><span class="dot"></span>'+(proj?proj.status:'暂无演练')+'</span>';
    }
  }

  // 未读通知
  API.getNotifs().then(function(ns){
    var n = document.getElementById('bellN');
    if(n) n.textContent = ns.filter(function(x){ return !x.read; }).length;
  });

  // 工作台待办数量在页面内展示，顶栏不再显示任务徽标
}

/* ---------- Toast / Modal / Drawer ---------- */
function toast(msg, type){
  var box = document.querySelector('.toasts') || (function(){ var d=document.createElement('div'); d.className='toasts'; document.body.appendChild(d); return d; })();
  var t = document.createElement('div'); t.className='toast '+(type||'');
  t.innerHTML = icon(type==='err'?'alert':type==='warn'?'alert':type==='ok'?'check':'bell') + '<span>'+msg+'</span>';
  box.appendChild(t);
  setTimeout(function(){ t.style.opacity='0'; t.style.transition='.3s'; setTimeout(function(){t.remove();},300); }, 3200);
}
function modal(opts){
  var ov = document.createElement('div'); ov.className='overlay';
  ov.innerHTML = '<div class="modal'+(opts.wide?' wide':'')+'"><div class="m-head">'+opts.title+'<span class="x">×</span></div><div class="m-body">'+opts.body+'</div>'+(opts.foot?'<div class="m-foot">'+opts.foot+'</div>':'')+'</div>';
  document.body.appendChild(ov);
  function close(){ ov.remove(); }
  ov.querySelector('.x').onclick=close;
  ov.addEventListener('click',function(e){ if(e.target===ov) close(); });
  ov.close = close;
  if(opts.onOpen) opts.onOpen(ov);
  return ov;
}
function drawer(opts){
  var ov = document.createElement('div'); ov.className='overlay'; ov.style.justifyContent='flex-end';
  ov.innerHTML = '<div class="drawer"><div class="d-head">'+opts.title+'<span class="x">×</span></div><div class="d-body">'+opts.body+'</div></div>';
  document.body.appendChild(ov);
  function close(){ ov.remove(); }
  ov.querySelector('.x').onclick=close;
  ov.addEventListener('click',function(e){ if(e.target===ov) close(); });
  ov.close = close;
  if(opts.onOpen) opts.onOpen(ov);
  return ov;
}
function confirmSim(msg, onOk){
  modal({ title:'演练模拟操作确认', body:'<div class="sim-banner">'+icon('alert')+'<span>本操作为演练推演模拟记录，不会下发任何真实网络配置命令。</span></div><p style="font-size:13.5px">'+msg+'</p>',
    foot:'<button class="btn" id="cfNo">取消</button><button class="btn primary" id="cfYes">确认执行（模拟）</button>',
    onOpen:function(ov){ ov.querySelector('#cfNo').onclick=ov.close; ov.querySelector('#cfYes').onclick=function(){ ov.close(); onOk&&onOk(); }; } });
}
function forbiddenPanel(pageKey){
  var r = curRole()||{};
  document.body.innerHTML = '<div class="login-bg"><div class="forbidden">'+icon('lock')+'<h2>403 · 越权访问拦截</h2><p>当前角色「'+(r.name||'')+'」无「'+pageKey+'」模块权限。<br>系统严格执行分级权限隔离，外部单位仅可查看医院主动上报的材料。</p><button class="btn primary" onclick="location.href=\'index.html\'">返回登录切换角色</button></div></div>';
}

/* ---------- 图表辅助（手写SVG/CSS） ---------- */
function barChart(el, data, opts){ // data:[{label,value,color?}]
  var max = Math.max.apply(null, data.map(function(d){return d.value;}))||1;
  el.innerHTML = '<div class="bar-chart">'+data.map(function(d){
    return '<div class="bar-col"><span class="b-val">'+d.value+'</span><div class="b-bar '+(d.color||'')+'" style="height:'+Math.round(d.value/max*100)+'%"></div><span class="b-label">'+d.label+'</span></div>';
  }).join('')+'</div>';
}
function hbar(el, data){ // [{label,value,max}]
  el.innerHTML = data.map(function(d){ var pct=Math.round(d.value/(d.max||100)*100);
    return '<div class="hbar-row"><span class="muted">'+d.label+'</span><div class="hb-track"><div class="hb-fill" style="width:'+pct+'%;background:'+(d.color||'var(--accent)')+'"></div></div><span class="hb-val">'+d.value+'</span></div>'; }).join('');
}
function donut(el, data, size){ // [{label,value,color}]
  size = size||140; var total = data.reduce(function(s,d){return s+d.value;},0)||1;
  var r = size/2-10, c = 2*Math.PI*r, off = 0, segs='';
  data.forEach(function(d){ var len = d.value/total*c;
    segs += '<circle cx="'+size/2+'" cy="'+size/2+'" r="'+r+'" fill="none" stroke="'+d.color+'" stroke-width="14" stroke-dasharray="'+len+' '+(c-len)+'" stroke-dashoffset="'+(-off)+'" transform="rotate(-90 '+size/2+' '+size/2+')"/>'; off += len; });
  el.innerHTML = '<div class="donut-wrap"><svg width="'+size+'" height="'+size+'">'+segs+'<text x="'+size/2+'" y="'+size/2+5+'" text-anchor="middle" fill="#e6edf7" font-size="16" font-weight="700" font-family="Consolas">'+total+'</text></svg><div class="legend">'+data.map(function(d){return '<span class="lg-item"><span class="lg-dot" style="background:'+d.color+'"></span>'+d.label+' · '+d.value+'</span>';}).join('')+'</div></div>';
}

/* ---------- 项目切换 / 通知 ---------- */
function switchProject(){
  API.getProjects().then(function(ps){
    modal({ title:'切换演练项目', body: ps.map(function(p){ return '<div class="task-item" data-id="'+p.id+'" style="margin-bottom:8px"><span class="tk-ic">'+icon('projects')+'</span><div><div class="tk-title">'+p.name+'</div><div class="tk-meta"><span>'+p.type+'</span><span>'+p.date+'</span></div></div><span class="tk-right">'+statusTag(p.status)+'</span></div>'; }).join(''),
      onOpen:function(ov){ ov.querySelectorAll('.task-item').forEach(function(el){ el.onclick=function(){ API.setActiveProject(el.dataset.id).then(function(){ toast('已切换演练项目','ok'); location.reload(); }); }; }); } });
  });
}
function showNotifs(){
  API.getNotifs().then(function(ns){
    modal({ title:'消息通知', wide:true, body:'<div class="timeline">'+ns.map(function(n){ return '<div class="tl-item '+(n.level==='P0'?'err':n.level==='P1'?'warn':'gray')+'"><span class="tl-dot"></span><div class="tl-time">'+n.time+' · 来源：'+n.source+'</div><div class="tl-title">'+n.title+'</div><div class="tl-desc">'+n.desc+'</div></div>'; }).join('')+'</div>',
      onOpen:function(ov){ API.readNotifs(); var b=document.getElementById('bellN'); if(b)b.textContent='0'; } });
  });
}
function statusTag(s){
  var m = {'未开始':'gray','演练中':'yellow','暂停':'blue','已结束':'green','已归档':'purple','待处理':'blue','处理中':'yellow','已完成':'green','超时':'red','启用':'green','停用':'gray','草稿':'gray','已发布':'green','一般':'blue','较大':'yellow','重大':'red'};
  return '<span class="tag '+(m[s]||'gray')+'"><span class="dot"></span>'+s+'</span>';
}
function levelTag(l){ var m={'P0':'red','P1':'yellow','P2':'blue','一般':'blue','较大':'yellow','重大':'red'}; return '<span class="tag '+(m[l]||'gray')+'">'+l+'</span>'; }

function logout(){
  function done(){
    DRILL.setToken('');
    sessionStorage.removeItem('pzy_role'); sessionStorage.removeItem('pzy_user');
    sessionStorage.removeItem('pzy_org'); sessionStorage.removeItem('pzy_proj');
    localStorage.removeItem('pzy_taskdone'); localStorage.removeItem('pzy_proj');
    location.href='index.html';
  }
  try{ API.logout().then(done, done); }catch(e){ done(); }
}

/* ---------- 启动 ---------- */
function boot(pageKey){
  var g = guard(pageKey);
  if(g==='forbidden'){ forbiddenPanel(pageKey); return; }
  if(!g) return;
  mountShell(pageKey);

  var page = document.getElementById('page');
  if(page && !page.innerHTML.trim()){
    page.innerHTML = '<div class="loading-block"><div class="spinner"></div><span>正在加载演练数据…</span></div>';
  }

  /* 品牌设置：与业务数据并行加载，加载完成后同步标题、品牌区与登录信息区 */
  DRILL.loadBrand().then(function(b){ if(b) DRILL.applyBrand(b); });

  /* 先加载真实数据，再通知各页面渲染（页面内同步读取 MOCK） */
  DRILL.bootstrap().then(function(){
    refreshShell();
    document.dispatchEvent(new CustomEvent('app:ready',{detail:{role:curRole(), data:MOCK.META}}));
  }).catch(function(e){
    var p = document.getElementById('page');
    if(p){
      p.innerHTML = '<div class="empty" style="padding:48px 20px"><div class="e-ic">'+icon('alert')+'</div>'
        + '<div class="e-title">数据加载失败</div>'
        + '<div class="e-desc">'+String((e && e.message) || '无法连接服务端，请确认服务已启动')+'</div>'
        + '<button class="btn primary" onclick="location.reload()">重新加载</button></div>';
    }
  });
}

window.App = { boot:boot, icon:icon, ICONS:ICONS, toast:toast, modal:modal, drawer:drawer, confirmSim:confirmSim,
  logout:logout, switchProject:switchProject, showNotifs:showNotifs, statusTag:statusTag, levelTag:levelTag,
  refreshShell:refreshShell,
  barChart:barChart, hbar:hbar, donut:donut, curRole:curRole, curUser:curUser, esc:function(s){return String(s==null?'':s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');} };
})();
