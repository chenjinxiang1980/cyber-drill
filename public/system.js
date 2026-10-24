/* 正式版数据层：令牌管理、REST 调用、登录后引导数据填充 MOCK
 * 页面仍按原方式同步读取 window.MOCK / window.API，网络请求走 REST 接口。
 */
window.MOCK = window.MOCK || {};
window.DRILL = (function () {
  'use strict';
  var TOKEN_KEY = 'pzy_token';

  function token() { return sessionStorage.getItem(TOKEN_KEY) || ''; }
  function setToken(t) {
    if (t) sessionStorage.setItem(TOKEN_KEY, t);
    else sessionStorage.removeItem(TOKEN_KEY);
  }

  /* 统一 REST 调用 */
  function api(path, opts) {
    opts = opts || {};
    var isLogin = path.indexOf('/auth/login') === 0; /* 登录接口的 401 是账号密码错误，需回传给页面提示 */
    var headers = {};
    var body = opts.body;
    if (!(body instanceof FormData) && body !== undefined && body !== null) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(body);
    }
    var t = token();
    if (t) headers['Authorization'] = 'Bearer ' + t;

    return fetch('/api' + path, {
      method: opts.method || 'GET',
      headers: headers,
      body: body,
      credentials: 'same-origin'
    }).then(function (r) {
      var ct = r.headers.get('content-type') || '';
      var parsed = ct.indexOf('json') >= 0 ? r.json() : r.text();
      return parsed.then(function (data) {
        if (r.status === 401 && !isLogin) {
          setToken('');
          sessionStorage.removeItem('pzy_role');
          sessionStorage.removeItem('pzy_user');
          if (location.pathname.indexOf('index.html') < 0 && /\/$|\.html$/.test(location.pathname)) {
            location.href = 'index.html';
          }
          return new Promise(function () {}); /* 挂起，避免页面继续执行 */
        }
        if (!r.ok) {
          var msg = (data && data.error) || ('请求失败（HTTP ' + r.status + '）');
          var err = new Error(msg);
          err.status = r.status;
          throw err;
        }
        return data;
      });
    });
  }

  /* 引导数据 → 填充 MOCK（字段名与原型保持一致，页面无需改动） */
  function fill(d) {
    MOCK.ROLES = d.roles || [];
    MOCK.USERS = (d.users || []).map(function (u) {
      return { id: u.id, username: u.username, name: u.name, role: u.role, roleName: u.roleName, org: u.org, status: u.status };
    });
    MOCK.PROJECTS = d.projects || [];
    MOCK.SCENARIOS = d.scenarios || [];
    MOCK.SCRIPT_NODES = d.scriptNodes || [];
    MOCK.TASKS = d.tasks || [];
    MOCK.FORM_TEMPLATES = d.formTemplates || [];
    MOCK.MESSAGES = d.messages || [];
    MOCK.NOTIFS = d.notifs || [];
    MOCK.CHAT = d.chat || { inner: [], external: [] };
    if (!MOCK.CHAT.inner) MOCK.CHAT.inner = [];
    if (!MOCK.CHAT.external) MOCK.CHAT.external = [];
    MOCK.EVIDENCE = d.evidence || [];
    MOCK.SNAPSHOTS = d.snapshots || [];
    MOCK.TIMELINE = d.timeline || [];
    MOCK.SEQ_FLOW = d.seqFlow || [];
    MOCK.SCORE_ITEMS = d.scoreItems || [];
    MOCK.SCORES = d.scores || [];
    MOCK.LEDGER = d.ledger || [];
    MOCK.OPINION = d.opinion || [];
    MOCK.KNOWLEDGE = d.knowledge || [];
    MOCK.AUDIT_LOGS = d.auditLogs || [];
    MOCK.IMPACT = d.impact || [];
    MOCK.DICT = d.dict || {};
    MOCK.SIM_DATA = d.simData || {};
    MOCK.META = { serverTime: d.serverTime, currentUser: d.currentUser, roleId: d.currentUser && d.currentUser.roleId };
    if (d.activeProjectId) sessionStorage.setItem('pzy_proj', d.activeProjectId);
    return d;
  }

  /* 拉取引导数据并填充（幂等，可重复调用刷新） */
  function bootstrap() {
    return api('/bootstrap').then(function (d) {
      fill(d);
      return d;
    });
  }

  /* 统一失败提示（页面多数只在 .then 中处理成功分支） */
  window.addEventListener('unhandledrejection', function (e) {
    var reason = e && e.reason;
    if (reason && (reason.status || reason.message)) {
      e.preventDefault();
      if (window.App && App.toast) App.toast(reason.message || '操作失败', 'err');
      else console.error('[api]', reason.message || reason);
    }
  });

  return {
    api: api, token: token, setToken: setToken,
    bootstrap: bootstrap, fill: fill,
    get: function (p) { return api(p); },
    post: function (p, b) { return api(p, { method: 'POST', body: b === undefined ? {} : b }); },
    patch: function (p, b) { return api(p, { method: 'PATCH', body: b || {} }); },
    del: function (p) { return api(p, { method: 'DELETE' }); },
    upload: function (p, formData) { return api(p, { method: 'POST', body: formData }); },
    /* 本地快捷 toast（避免页面依赖 App 初始化顺序） */
    fail: function (e) {
      var m = (e && e.message) || '操作失败';
      if (window.App && App.toast) App.toast(m, 'err');
      else console.error(m);
    }
  };
})();
