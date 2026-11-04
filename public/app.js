/* 示例医院网络安全应急演练系统 · App Shell / 图标 / 通用组件（共享骨架，禁止各页自写导航） */
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
  knowledge:'<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V4H6.5A2.5 2.5 0 0 0 4 6.5v13z" '+P+'/><path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5" '+P+'/>',
  admin:'<circle cx="12" cy="12" r="3" '+P+'/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h0a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5h0a1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v0a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" '+P+'/>',
  bell:'<path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" '+P+'/><path d="M13.7 21a2 2 0 0 1-3.4 0" '+P+'/>',
  volume:'<path d="M11 5L6 9H3v6h3l5 4z" '+P+'/><path d="M15.5 8.5a5 5 0 0 1 0 7M18.5 5.5a9 9 0 0 1 0 13" '+P+'/>',
  'volume-off':'<path d="M11 5L6 9H3v6h3l5 4z" '+P+'/><path d="M16 9l5 6M21 9l-5 6" '+P+'/>',
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
  gitee:'<circle cx="12" cy="12" r="9.2" fill="currentColor" stroke="none"/><path d="M16.2 9.1h-5.6a2.5 2.5 0 0 0 0 5h2.7v2.3H9.2" fill="none" stroke="#0b1220" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/>',
  'arrow-right':'<path d="M5 12h14M13 5l7 7-7 7" '+P+'/>',
  'arrow-up':'<path d="M12 19V5M5 12l7-7 7 7" '+P+'/>',
  'arrow-down':'<path d="M12 5v14M5 12l7 7 7-7" '+P+'/>',
  'arrow-left':'<path d="M19 12H5M12 5l-7 7 7 7" '+P+'/>',
  'center-h':'<path d="M3 12h18M8 7v10M16 7v10" '+P+'/>',
  'center-v':'<path d="M12 3v18M7 8h10M7 16h10" '+P+'/>',
  'dist-v':'<path d="M12 3v4M12 10v4M12 17v4M5 5h14M5 12h14M5 19h14" '+P+'/>',
  'dist-h':'<path d="M3 12h4M10 12h4M17 12h4M5 5v14M12 5v14M19 5v14" '+P+'/>',
  'grid':'<rect x="3" y="3" width="18" height="18" rx="2" '+P+'/><path d="M3 9h18M3 15h15M9 3v18M15 3v18" '+P+'/>',
  'flow':'<path d="M3 7h13a3 3 0 0 1 3 3v1" '+P+'/><path d="M21 13a3 3 0 0 1-3 3H3" '+P+'/><circle cx="16" cy="10" r="1.5" '+P+' fill="currentColor"/><circle cx="8" cy="16" r="1.5" '+P+' fill="currentColor"/>',
  'score':'<path d="M12 2l3 7h7l-5.5 4.5L18 21l-6-4-6 4 1.5-7.5L2 9h7z" '+P+'/>',
  'scenarios':'<rect x="3" y="5" width="18" height="14" rx="2" '+P+'/><path d="M3 9h18M9 5v14M15 5v14" '+P+'/>',
  'workbench':'<path d="M3 10h18M5 10v8M19 10v8M7 14h10" '+P+'/><rect x="8" y="4" width="8" height="6" rx="1" '+P+'/>',
  'collab':'<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" '+P+'/>',
  'dashboard':'<rect x="3" y="3" width="7" height="9" rx="1" '+P+'/><rect x="14" y="3" width="7" height="5" rx="1" '+P+'/><rect x="14" y="12" width="7" height="9" rx="1" '+P+'/><rect x="3" y="16" width="7" height="5" rx="1" '+P+'/>',
  'screen':'<rect x="3" y="4" width="18" height="13" rx="2" '+P+'/><path d="M8 21h8M12 17v4" '+P+'/>',
  'evidence':'<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" '+P+'/><path d="M14 2v6h6M9 13h6M9 17h4" '+P+'/>',
  'replay':'<circle cx="12" cy="12" r="9" '+P+'/><path d="M12 7v5l3 3" '+P+'/>',
  'evaluation':'<path d="M12 2l3 7h7l-5.5 4.5L18 21l-6-4-6 4 1.5-7.5L2 9h7z" '+P+'/>',
  'projects':'<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" '+P+'/>',
  'chat':'<path d="M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8v.5z" '+P+'/>'
};
function icon(name, cls){ return '<svg viewBox="0 0 24 24" class="'+(cls||'')+'">'+(ICONS[name]||ICONS.file)+'</svg>'; }

/* ---------- 导航配置（唯一来源，按角色 nav 过滤） ---------- */
var NAV_GROUPS = [
  { title:'演练执行', items:[
    {key:'dashboard', icon:'dashboard', label:'态势总看板', href:'dashboard.html'},
    {key:'workbench', icon:'workbench', label:'角色工作台', href:'workbench.html'},
    {key:'collab', icon:'chat', label:'演练导调中心', href:'collab.html'}
  ]},
  { title:'编排与资源', items:[
    {key:'scenarios', icon:'scenarios', label:'场景剧本库与编排', href:'scenarios.html'},
    {key:'projects', icon:'projects', label:'演练项目管理', href:'projects.html'},
    {key:'evidence', icon:'evidence', label:'取证与材料归档', href:'evidence.html'},
    {key:'knowledge', icon:'knowledge', label:'预案知识库', href:'knowledge.html'}
  ]},
  { title:'复盘与报送', items:[
    {key:'replay', icon:'replay', label:'演练时序回放', href:'replay.html'},
    {key:'evaluation', icon:'score', label:'复盘评估', href:'evaluation.html'},
    {key:'report', icon:'file', label:'演练报告导出', href:'report.html'},
    {key:'ledger', icon:'ledger', label:'台账与统计', href:'ledger.html'}
  ]},
  { title:'系统管理', items:[
    {key:'admin', icon:'admin', label:'系统设置', href:'admin.html'},
    {key:'dictionary', icon:'db', label:'流程字典', href:'dictionary.html'}
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
  var brandUnit = B.unitName || '示例医院';
  var brandVer = B.version || 'v0.5.0';
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
    + '<div class="topbar-row topbar-row-1">'
    +   '<button class="nav-toggle" id="navToggle" type="button" aria-label="导航菜单">☰</button>'
    +   '<span class="proj-sel" onclick="App.switchProject()">'+icon('projects')+'<b id="topProj"></b><span class="dim">切换 ▾</span></span>'
    +   '<span id="topLive"></span>'
    + '</div>'
    + '<span class="tb-center" onclick="App.openScreen()">'+icon('screen')+'<span id="topStatus">演练监视大屏</span></span>'
    + '<div class="topbar-right-group">'
    +   '<span class="bell" onclick="App.showNotifs()">'+icon('bell')+'<span class="n" id="bellN" style="display:none">0</span></span>'
    +   '<span class="bell voice-btn" id="voiceBtn" onclick="App.toggleVoice()" title="语音播报"></span>'
    +   '<span class="role-chip"><span class="av">'+(r.name||'?').slice(0,1)+'</span>'+r.name+'</span>'
    +   '<span class="tb-out" onclick="location.href=\'password.html\'">'+icon('lock')+'修改密码</span>'
    +   '<span class="tb-out" onclick="App.logout()">'+icon('logout')+'退出</span>'
    +   '<span class="tb-clock" id="topClock"></span>'
    + '</div>'
    + '</header><main id="page"></main></div>';
  document.body.insertAdjacentHTML('afterbegin', html);

  // 时钟（立即启动，不依赖数据）
  function tick(){ var d=new Date(); var el=document.getElementById('topClock'); if(el) el.textContent = d.toLocaleDateString('zh-CN')+' '+d.toTimeString().slice(0,8); }
  tick(); shellClockTick = setInterval(tick,1000);

  // 移动端汉堡：切换 sidebar 抽屉 + body.drawer-open 遮罩
  var toggle = document.getElementById('navToggle');
  var side = document.querySelector('.sidebar');
  if(toggle && side){
    function openDrawer(){ side.classList.add('open'); document.body.classList.add('drawer-open'); }
    function closeDrawer(){ side.classList.remove('open'); document.body.classList.remove('drawer-open'); }
    toggle.addEventListener('click', function(e){
      e.stopPropagation();
      if(side.classList.contains('open')) closeDrawer(); else openDrawer();
    });
    document.addEventListener('click', function(e){
      if(!side.classList.contains('open')) return;
      if(side.contains(e.target)) return;
      if(toggle.contains(e.target)) return;
      closeDrawer();
    });
    // 抽屉打开时锁 body 滚动（额外在 styles.css body.drawer-open::before 做遮罩）
  }
}

/* 未读消息角标：数字 >0 时显示，打开消息弹窗后自动隐藏 */
function setBell(n){
  var b = document.getElementById('bellN');
  if(!b) return;
  b.textContent = String(n);
  b.style.display = n > 0 ? '' : 'none';
}
/* ---------- 语音引擎（浏览器内置 speechSynthesis，零依赖、本机记忆开关） ---------- */
var VOICE_KEY = 'pzy_voice_on';
var voiceLastText = '';
var voiceTimer = null;   /* 交互提示播报（表单提示/开关提示）的防抖定时器 */
var voiceSeq = 0;

/* 站内消息播报：以消息 id 去重，按页面会话生命周期维护（刷新后未读仍会再播一次） */
var notifSpoken = {};    /* 已入队/已播的消息 id */
var notifQueue = [];     /* [{id,text}] 串行播报队列 */
var notifSpeaking = false;
var notifWatchdog = null;
var notifClaimTimer = null;
var notifBusy = false;   /* 通知刷新请求进行中，避免并发响应重复入队 */
var notifChannel = null; /* 同浏览器多标签互斥（同一消息只在一个标签播报） */
function voiceSupported(){
  return typeof window !== 'undefined' && !!window.speechSynthesis && typeof window.SpeechSynthesisUtterance === 'function';
}
function voiceOn(){
  try{ return localStorage.getItem(VOICE_KEY) !== '0'; }catch(e){ return true; }
}
function buildUtterance(t){
  var u = new SpeechSynthesisUtterance(t);
  u.lang = 'zh-CN'; u.rate = 1; u.pitch = 1;
  var voices = (typeof window.speechSynthesis.getVoices === 'function') ? (window.speechSynthesis.getVoices() || []) : [];
  var zh = voices.filter(function(v){ return /^zh|chinese/i.test(v.lang || ''); })[0];
  if(zh) u.voice = zh;
  return u;
}
function voiceSpeak(text, opts){
  if(!voiceSupported()) return;
  var always = !!(opts && opts.always);
  /* 站内消息提醒不受语音开关限制（opts.always）；其余播报受顶栏开关控制 */
  if(!always && !voiceOn()) return;
  var t = String(text == null ? '' : text).replace(/\s+/g, ' ').trim();
  if(!t) return;
  if(t.length > 220) t = t.slice(0, 220) + '…';
  if(t === voiceLastText && !(opts && opts.force)) return;
  voiceLastText = t;
  var seq = ++voiceSeq;
  try{
    var synth = window.speechSynthesis;
    if(!(opts && opts.queue)){
      /* cancel 为异步操作，立即 speak 会吞掉首字（如「请填写」的「请」），延后播报 */
      if(synth.speaking || synth.pending || synth.paused) synth.cancel();
      if(voiceTimer){ clearTimeout(voiceTimer); voiceTimer = null; }
      voiceTimer = setTimeout(function(){
        voiceTimer = null;
        if(seq !== voiceSeq) return;
        if(!always && !voiceOn()) return;
        try{ synth.speak(buildUtterance(t)); }catch(e){}
      }, 200);
    } else {
      synth.speak(buildUtterance(t));
    }
  }catch(e){}
}
function voiceStop(){
  if(voiceTimer){ clearTimeout(voiceTimer); voiceTimer = null; }
  voiceSeq++;
  try{ if(voiceSupported()) window.speechSynthesis.cancel(); }catch(e){}
}
function syncVoiceBtn(){
  var b = document.getElementById('voiceBtn');
  if(!b) return;
  var on = voiceOn() && voiceSupported();
  b.innerHTML = icon(on ? 'volume' : 'volume-off');
  b.classList.toggle('off', !on);
  b.title = voiceSupported()
    ? (on ? '语音播报已开启（点击关闭）' : '语音播报已关闭（点击开启）')
    : '当前浏览器不支持语音合成';
}
function toggleVoice(){
  if(!voiceSupported()){ toast('当前浏览器不支持语音合成，请使用 Chrome / Edge 等现代浏览器','warn'); return; }
  var next = !voiceOn();
  try{ localStorage.setItem(VOICE_KEY, next ? '1' : '0'); }catch(e){}
  if(!next) voiceStop();
  voiceLastText = '';
  syncVoiceBtn();
  toast(next ? '语音播报已开启' : '语音播报已关闭', next ? 'ok' : '');
  if(next) voiceSpeak('语音播报已开启', { force:true });
}
/* ---------- 站内消息播报：串行队列 + 消息 id 去重 + 当前项目过滤 ---------- */
function initNotifChannel(){
  if(notifChannel || typeof BroadcastChannel !== 'function') return;
  try{
    notifChannel = new BroadcastChannel('pzy_notif_voice');
    notifChannel.onmessage = function(ev){
      var d = ev && ev.data;
      if(!d || d.type !== 'spoken' || !d.id) return;
      var id = String(d.id);
      notifSpoken[id] = 1;
      notifQueue = notifQueue.filter(function(q){ return q.id !== id; });
    };
  }catch(e){ notifChannel = null; }
}
/* 同标签页刷新沿用 sessionStorage 中的 tab id，其他标签页拿不到它 */
function notifTabId(){
  try{
    var id = sessionStorage.getItem('pzy_voice_tab');
    if(!id){
      id = Math.random().toString(36).slice(2) + Date.now().toString(36);
      sessionStorage.setItem('pzy_voice_tab', id);
    }
    return id;
  }catch(e){ return 'tab-' + Date.now(); }
}
/* 抢播占位：同一消息 5 秒内只允许一个标签页发声；同一标签页刷新不受限（仍会再播一次） */
function notifClaim(id){
  var mine = notifTabId();
  try{
    var rec = JSON.parse(localStorage.getItem('pzy_voice_claim_' + id) || 'null');
    if(rec && rec.tab !== mine && Date.now() - rec.t < 5000) return 'other';
    localStorage.setItem('pzy_voice_claim_' + id, JSON.stringify({ tab: mine, t: Date.now() }));
    return 'mine';
  }catch(e){ return 'mine'; }
}
function notifQueueDrain(){
  if(notifSpeaking) return;
  if(!voiceSupported()){ notifQueue.length = 0; return; }
  var item = notifQueue.shift();
  while(item){
    /* 播报前再核对一次服务端已读状态：已读消息不再播 */
    var cur = (MOCK.NOTIFS || []).filter(function(x){ return String(x.id) === item.id; })[0];
    if(cur && cur.read){ item = notifQueue.shift(); continue; }
    if(notifClaim(item.id) === 'other'){ item = notifQueue.shift(); continue; }
    break;
  }
  if(!item) return;
  notifSpeaking = true;
  var synth = window.speechSynthesis;
  var finished = false;
  function done(){
    if(finished) return;
    finished = true;
    if(notifWatchdog){ clearTimeout(notifWatchdog); notifWatchdog = null; }
    notifSpeaking = false;
    notifQueueDrain();
  }
  /* 留出跨标签页写占位的确认窗口：若同帧被另一标签抢占则让出 */
  notifClaimTimer = setTimeout(function(){
    notifClaimTimer = null;
    if(notifClaim(item.id) === 'other'){ notifSpeaking = false; notifQueueDrain(); return; }
    try{
      var u = buildUtterance(item.text);
      u.onend = done; u.onerror = done;
      /* 兜底：个别环境不触发 onend，按正文长度解除占用，避免队列卡死 */
      notifWatchdog = setTimeout(done, 1200 + item.text.length * 260);
      synth.speak(u);
    }catch(e){ done(); }
  }, 150);
}
function announceNotif(n){
  var id = String(n.id || '');
  if(!id || notifSpoken[id]) return;
  /* 播报文本 = 消息正文（descr）原样：不拼 title、不加序号、不截断、不做空白归一化 */
  var text = String(n.desc == null ? '' : n.desc);
  if(!text) return; /* 正文为空（空串/null）不播报，不退化为播标题 */
  notifSpoken[id] = 1;
  notifQueue.push({ id:id, text:text });
  try{ if(notifChannel && notifChannel.postMessage) notifChannel.postMessage({ type:'spoken', id:id }); }catch(e){}
}
function announceUnreadNotifs(ns){
  var proj = API.activeProject();
  var pid = proj && proj.id ? String(proj.id) : '';
  if(!pid) return; /* 无当前演练项目：不播报 */
  (ns || []).forEach(function(n){
    if(!n || n.read) return; /* 已读消息不播报，读状态以服务端返回为准 */
    var npid = n.projectId == null ? '' : String(n.projectId);
    if(!npid || npid !== pid) return; /* 跨项目消息 / project_id 为空的全局公告：一律不播 */
    announceNotif(n);
  });
  notifQueueDrain();
}
function refreshNotifBadge(){
  if(notifBusy) return; /* 并发刷新只允许一个请求落地，防止同一消息重复入队 */
  notifBusy = true;
  API.getNotifs().then(function(ns){
    notifBusy = false;
    MOCK.NOTIFS = ns;
    var unread = ns.filter(function(x){ return !x.read; });
    setBell(unread.length);
    announceUnreadNotifs(ns);
  }).catch(function(){ notifBusy = false; });
}
var notifPoll = null;
var liveTick = null;
var shellClockTick = null;
/* 顶栏演练计时：按真实起止时刻每秒本地推算 T+HH:MM:SS */
function renderLiveChip(){
  var live = document.getElementById('topLive');
  if(!live) return;
  var proj = API.activeProject();
  if(!proj){ live.innerHTML = '<span class="live-chip none"><span class="dot"></span>暂无演练</span>'; return; }
  if(proj.status==='演练中' || proj.status==='暂停'){
    var paused = proj.status==='暂停';
    /* 历史数据可能 started_at 为空（未写入开始时刻）：如实标注"计时未启动"，不显示误导性的 T+00:00:00 */
    var es = API.elapsedOf(proj);
    var timerTxt = (es === null) ? '计时未启动' : ('T+' + API.fmtElapsed(es));
    live.innerHTML = '<span class="live-chip'+(paused?' none':'')+'"><span class="dot"></span>'
      + (paused?'演练已暂停':'演练进行中') + ' · ' + App.esc(proj.level||'') + '级事件 · ' + timerTxt + '</span>';
  } else {
    live.innerHTML = '<span class="live-chip none"><span class="dot"></span>' + App.esc(proj.status) + '</span>';
  }
}
function startNotifPolling(){
  if(notifPoll) return;
  initNotifChannel();
  notifPoll = setInterval(refreshNotifBadge, 15000);
}
function stopNotifPolling(){
  if(notifPoll){ clearInterval(notifPoll); notifPoll = null; }
}
/* 页面卸载/进入后台缓存时释放全部定时器与跨标签通道，避免重复轮询与叠加播报 */
function clearShellTimers(){
  stopNotifPolling();
  if(liveTick){ clearInterval(liveTick); liveTick = null; }
  if(shellClockTick){ clearInterval(shellClockTick); shellClockTick = null; }
  voiceStop();
  notifQueue.length = 0;
  notifSpeaking = false;
  if(notifWatchdog){ clearTimeout(notifWatchdog); notifWatchdog = null; }
  if(notifClaimTimer){ clearTimeout(notifClaimTimer); notifClaimTimer = null; }
  if(notifChannel){ try{ notifChannel.close(); }catch(e){} notifChannel = null; }
}
window.addEventListener('pagehide', clearShellTimers);
window.addEventListener('pageshow', function(){
  initNotifChannel();
  startNotifPolling();
  if(!liveTick) liveTick = setInterval(renderLiveChip, 1000);
});

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
  if(live){ renderLiveChip(); }
  if(!liveTick) liveTick = setInterval(renderLiveChip, 1000);

  // 未读通知：立即刷新并定时轮询，演练中的节点/得分消息无需刷新页面即可更新角标
  refreshNotifBadge();
  startNotifPolling();
  syncVoiceBtn();

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
  ov.innerHTML = '<div class="modal'+(opts.wide?' wide':'')+(opts.full?' full':'')+'"><div class="m-head">'+opts.title+'<span class="x">×</span></div><div class="m-body">'+opts.body+'</div>'+(opts.foot?'<div class="m-foot">'+opts.foot+'</div>':'')+'</div>';
  document.body.appendChild(ov);
  function close(){ if(opts.onClose) opts.onClose(); ov.remove(); }
  ov.querySelector('.x').onclick=close;
  if(!opts.persist) ov.addEventListener('click',function(e){ if(e.target===ov) close(); });
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
  if(!opts.persist) ov.addEventListener('click',function(e){ if(e.target===ov) close(); });
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
    var list = ns.slice().reverse(); /* 最新消息在最上方 */
    var body = list.length ? '<div class="timeline">'+list.map(function(n){
      return '<div class="tl-item '+(n.level==='P0'?'err':n.level==='P1'?'warn':'gray')+'"><span class="tl-dot"></span>'
        + '<div class="tl-time">'+App.esc(n.time)+' · 来源：'+App.esc(n.source)+'</div>'
        + '<div class="tl-title">'+App.esc(n.title)+(n.read?'':' <span class="tag blue">未读</span>')+'</div>'
        + '<div class="tl-desc">'+App.esc(n.desc)+'</div></div>';
    }).join('')+'</div>' : '<div class="empty">'+icon('bell')+'<div>暂无站内消息</div></div>';
    modal({ title:'站内消息', wide:true, body:body,
      onOpen:function(ov){ API.readNotifs(); setBell(0); } });
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

/* 演练监视大屏：弹出独立窗口（无边框、全屏体验） */
function openScreen(){
  var w = 1680, h = 960;
  var left = Math.max(0, Math.floor((screen.availWidth - w) / 2));
  var top = Math.max(0, Math.floor((screen.availHeight - h) / 2));
  var feat = 'width='+w+',height='+h+',left='+left+',top='+top+',menubar=no,toolbar=no,location=no,status=no,scrollbars=no,resizable=yes';
  var win = window.open('screen.html', 'pzy_screen', feat);
  if(!win){
    /* 被浏览器拦截时 fallback：同标签页跳转 */
    location.href = 'screen.html';
    return;
  }
  try { win.focus(); } catch(e){}
}

/* ===== 演练脚本弹层（项目管理页与角色工作台共用，只读） ===== */
function scriptModeOf(data, p){
  var mode = (data && data.mode) || (p && p.mode) || '基础版';
  return mode === '实战版' ? '实战版' : '基础版';
}
function scriptNodeCard(n, idx, mode){
  var typeLabel = ({alert:'告警节点',parallel:'并行节点',task:'处置节点',report:'上报节点',decision:'决策节点'})[n.type] || '节点';
  var timeoutTag = n.timeout ? (n.timeout + ' 分钟') : '未设置';
  var extras = '';
  if(mode === '基础版'){
    if(n.guide) extras += '<div class="sn-guide"><span class="sn-guide-label">演练提示：</span>'+App.esc(n.guide)+'</div>';
    if(n.desc) extras += '<div class="sn-desc">'+App.esc(n.desc)+'</div>';
    if(n.score) extras += '<div class="sn-score">分值 '+n.score+' 分</div>';
  }else{
    /* 实战版：组内所有条目外观完全一致，不出现任何对错标记 */
    if(n.desc) extras += '<div class="sn-desc">'+App.esc(n.desc)+'</div>';
    if(n.action) extras += '<div class="sn-action">处置动作：'+App.esc(n.action)+'</div>';
  }
  var titleHtml = mode === '基础版' ? ((idx+1)+'. '+App.esc(n.title)) : (App.esc(n.title) || '未命名动作');
  return '<div class="script-node '+(mode === '基础版' ? 'basic' : 'option')+'">'
    + '<div class="sn-head"><span class="sn-title">'+titleHtml+'</span></div>'
    + '<div class="sn-meta">'
    +   '<span>'+typeLabel+'</span>'
    +   '<span>责任角色：'+(n.role ? App.esc(n.role) : '—')+'</span>'
    +   '<span>处置时长：'+timeoutTag+'</span>'
    +   (mode === '基础版' && n.action ? '<span>动作：'+App.esc(n.action)+'</span>' : '')
    + '</div>'
    + extras
    + '</div>';
}
/* 实战版分组：一个主流节点 = 一个步骤组，组内混合锚定在该节点上的全部动作 */
function shuffleList(arr){
  for(var i=arr.length-1;i>0;i--){
    var j = Math.floor(Math.random()*(i+1));
    var t = arr[i]; arr[i] = arr[j]; arr[j] = t;
  }
  return arr;
}
function buildScriptGroups(list){
  var groups = [], groupByMainId = {};
  list.forEach(function(n){
    if(String(n.flowType || '') === 'distractor') return;
    var g = { main:n, items:[n] };
    groups.push(g);
    groupByMainId[String(n.id)] = g;
  });
  var orphans = [];
  list.forEach(function(n){
    if(String(n.flowType || '') !== 'distractor') return;
    var g = groupByMainId[String(n.anchorId || '')];
    if(g){ g.items.push(n); } else { orphans.push(n); }
  });
  /* 锚点缺失或失配的条目并入序号最接近的前一组，避免静默丢失 */
  orphans.forEach(function(n){
    var target = groups[0];
    for(var i=0;i<groups.length;i++){
      if(Number(groups[i].main.seq) <= Number(n.seq)) target = groups[i];
    }
    if(target) target.items.push(n);
  });
  /* 组内随机混排；若本次打开中主流条目排在首位的组数超过半数则重新洗牌 */
  var tries = 0;
  do {
    groups.forEach(function(g){ shuffleList(g.items); });
    tries++;
    var multi = groups.filter(function(g){ return g.items.length > 1; });
    var firstCount = multi.filter(function(g){ return g.items[0] === g.main; }).length;
  } while(firstCount > Math.floor(multi.length/2) && tries < 30);
  return groups;
}
function renderScriptBody(container, data, p){
  if(!data || !data.ok){ container.innerHTML = '<div class="center dim">脚本数据异常</div>'; return; }
  var mode = scriptModeOf(data, p);
  var all = Array.isArray(data.script) ? data.script : [];
  /* 终止节点一律不展示；基础版再过滤混淆节点，实战版保留全部动作 */
  var list = all.filter(function(n){
    if(n.type === 'end') return false;
    if(mode === '基础版' && String(n.flowType || '') === 'distractor') return false;
    return true;
  });
  var instructionsHtml = (data.instructions || []).map(function(s){ return '<li>'+App.esc(s)+'</li>'; }).join('');
  var membersHtml = (data.members || []).map(function(m){ return '<span class="sm-chip">'+App.esc(m.name)+'</span>'; }).join('');
  var projectName = data.projectName || data.name || (p && p.name) || '—';

  var flowSection;
  if(mode === '实战版'){
    /* 一个主流节点 = 一个步骤组；组内主流动作与锚定动作随机混排，外观完全一致 */
    var groups = buildScriptGroups(list);
    var groupsHtml = groups.map(function(g, gi){
      return '<div class="script-section"><h3>步骤组 '+(gi+1)+'</h3>'
        + g.items.map(function(n){ return scriptNodeCard(n, 0, mode); }).join('')
        + '</div>';
    }).join('');
    flowSection = '<div class="script-section"><h3>演练步骤（'+groups.length+' 个步骤组 · '+list.length+' 个动作）</h3>'
      + '<div class="tiny dim" style="margin-bottom:10px">每个步骤组对应一个主流节点，组内动作顺序随机排列；请结合现场情况自主选择执行的处置动作。</div>'
      + groupsHtml
      + '</div>';
  }else{
    flowSection = '<div class="script-section"><h3>演练流程（'+list.length+' 个主流节点）</h3>'
      + list.map(function(n, idx){ return scriptNodeCard(n, idx, mode); }).join('')
      + '</div>';
  }

  container.innerHTML =
    '<div class="script-instructions"><h4>'+App.esc(mode)+'演练脚本说明</h4><ul>'+instructionsHtml+'</ul>'
    + '<div class="tiny dim mt2" style="margin-top:8px">'
    + (mode === '实战版'
        ? '每个步骤组内的动作顺序为随机排列，不提前标注执行结果'
        : '✅ 已自动过滤全部混淆节点与终止节点，仅展示主流处置流程')
    + '</div></div>'
    + '<div class="script-section"><h3>演练概览</h3>'
    +   '<div class="grp-kv" style="margin-top:6px">'
    +     '<span class="k">项目编号</span><span>'+App.esc(data.projectId)+'</span>'
    +     '<span class="k">项目名称</span><span>'+App.esc(projectName)+'</span>'
    +     '<span class="k">演练模式</span><span>'+(mode === '实战版' ? '<span class="tag red">实战版</span>' : '<span class="tag green">基础版</span>')+'</span>'
    +     '<span class="k">关联剧本</span><span>'+(data.scenario ? App.esc(data.scenario.name) : '—')+'</span>'
    +     '<span class="k">剧本类型</span><span>'+(data.scenario ? App.esc(data.scenario.type) : '—')+'</span>'
    +     '<span class="k">当前状态</span><span>'+statusTag(data.status)+'</span>'
    +     '<span class="k">事件等级</span><span>'+App.esc(data.level)+'</span>'
    +   '</div>'
    + '</div>'
    + '<div class="script-section"><h3>参演角色（'+((data.members||[]).length)+' 人）</h3>'
    +   (membersHtml ? '<div class="script-members">'+membersHtml+'</div>' : '<div class="tiny dim">尚未添加参演角色</div>')
    + '</div>'
    + flowSection;
}
function openScript(p, opts){
  opts = opts || {};
  /* 无进行中项目：给空态提示，不弹错误 */
  if(!p || !p.id){
    modal({
      title:'演练脚本',
      body:'<div class="standby-hero" style="margin:0"><div class="sb-icon">'+icon('file-text')+'</div>'
        + '<div class="sb-body"><span class="sb-tag"><span class="dot"></span>NO ACTIVE DRILL · 暂无演练</span>'
        + '<h2>暂无可查看的演练脚本</h2>'
        + '<div class="sb-sub">当前没有进行中的演练项目。演练项目启动后，可在此查看本项目全部处置流程与选项（只读，不提供执行入口）。</div>'
        + '</div></div>',
      foot:'<button class="btn" id="scEmptyClose">关闭</button>',
      onOpen:function(ov){ ov.querySelector('#scEmptyClose').onclick = ov.close; }
    });
    return;
  }
  var foot = '<button class="btn" id="scClose">关闭</button>';
  if(opts.canExport){
    foot += '<a class="btn ok" id="scWord" style="text-decoration:none" target="_blank">'+icon('download')+'导出 Word</a>';
  }
  modal({ title:'演练脚本 · '+App.esc(p.name)+'（'+(p.mode||'基础版')+'）', wide:true,
    body:'<div id="scriptBody" class="script-body"><div class="center dim">加载中…</div></div>',
    foot:foot,
    onOpen:function(ov){
      ov.querySelector('#scClose').onclick = ov.close;
      if(opts.canExport && ov.querySelector('#scWord')) ov.querySelector('#scWord').href = API.exportProjectScriptUrl(p.id);
      API.getProjectScript(p.id).then(function(data){
        try { renderScriptBody(ov.querySelector('#scriptBody'), data, p); }
        catch(err){
          console.error('[script-view] renderScriptBody error:', err);
          ov.querySelector('#scriptBody').innerHTML = '<div class="center dim" style="color:#e74c3c">渲染异常：'+App.esc(err.message)+'<br>请刷新页面重试</div>';
        }
      }).catch(function(err){
        console.error('[script-view] getProjectScript error:', err);
        ov.querySelector('#scriptBody').innerHTML = '<div class="center dim" style="color:#e74c3c">加载失败：'+App.esc((err&&err.message)||'未知错误')+'<br>请检查登录状态或刷新页面</div>';
      });
    } });
}

window.App = { boot:boot, icon:icon, ICONS:ICONS, toast:toast, modal:modal, drawer:drawer, confirmSim:confirmSim,
  logout:logout, switchProject:switchProject, showNotifs:showNotifs, openScreen:openScreen, statusTag:statusTag, levelTag:levelTag,
  refreshShell:refreshShell, renderLiveChip:renderLiveChip, toggleVoice:toggleVoice,
  voice:{ speak:voiceSpeak, stop:voiceStop, on:voiceOn, supported:voiceSupported, sync:syncVoiceBtn },
  barChart:barChart, hbar:hbar, donut:donut, curRole:curRole, curUser:curUser, openScript:openScript, esc:function(s){return String(s==null?'':s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');} };
})();
