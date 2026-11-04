import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/*
 * 站内消息语音播报验证（只播正文 descr 版本）
 *
 * 两种模式：
 *  - VERIFY_MODE=vm   ：连真实服务（默认 http://127.0.0.1:8090），需 VERIFY_PASS
 *  - VERIFY_MODE=mock ：默认。用 Playwright 路由拦截：
 *       · 静态资源直接读本地 public/（与线上逐字节一致的 app.js）
 *       · /api/* 全部由桩数据返回（不触达任何现役库，不产生任何 exe）
 */
const MODE = process.env.VERIFY_MODE === 'vm' ? 'vm' : 'mock';
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const PUBLIC_DIR = path.join(ROOT, 'public');
const BASE = MODE === 'vm'
  ? (process.env.VERIFY_BASE || 'http://127.0.0.1:8090')
  : 'http://verify.local';
const USER = process.env.VERIFY_USER || 'admin';
const PASS = process.env.VERIFY_PASS || '';

/* 测试消息：播报文本必须与 descr 逐字相等，且不得包含任何 title 子串 */
const DESC = 'CUR-UNREAD-DESC';
const MIXED_DESC = 'Punctuation, numbers 2026-11-05 and English mixed.';
const EXPECTED = [DESC, MIXED_DESC];
const TITLES = [
  'CUR-UNREAD-TITLE',
  'MIXED-TITLE-MUST-NOT-SPEAK',
  'EMPTY-TITLE-MUST-NOT-SPEAK',
  'CUR-READ-TITLE',
  'OTHER-TITLE',
  'GLOBAL-TITLE',
];
const CUR_PROJ = 'P2026-001';

/* ---------------- 桩数据（mock 模式） ---------------- */
const NOTIF_DEFS = [
  { id: 'NV_CUR_UNREAD', title: 'CUR-UNREAD-TITLE', desc: 'CUR-UNREAD-DESC', projectId: CUR_PROJ },
  { id: 'NV_CUR_MIXED', title: 'MIXED-TITLE-MUST-NOT-SPEAK', desc: 'Punctuation, numbers 2026-11-05 and English mixed.', projectId: CUR_PROJ },
  { id: 'NV_CUR_EMPTY', title: 'EMPTY-TITLE-MUST-NOT-SPEAK', desc: '', projectId: CUR_PROJ },
  { id: 'NV_CUR_READ', title: 'CUR-READ-TITLE', desc: 'CUR-READ-DESC', projectId: CUR_PROJ },
  { id: 'NV_OTHER_UNREAD', title: 'OTHER-TITLE', desc: 'OTHER-DESC', projectId: 'P_VERIFY_OTHER' },
  { id: 'NV_GLOBAL', title: 'GLOBAL-TITLE', desc: 'GLOBAL-DESC', projectId: '' },
];
const readIds = new Set(['NV_CUR_READ']);

function notifList() {
  return NOTIF_DEFS.map((d) => ({
    id: d.id,
    time: '2026-11-05 01:00:00',
    level: '一般',
    source: 'SYSTEM',
    title: d.title,
    desc: d.desc,
    read: readIds.has(d.id),
    projectId: d.projectId,
  }));
}

function bootstrapPayload() {
  return {
    serverTime: '2026-11-05 02:00:00',
    currentUser: { id: 56, username: 'admin', name: '平台管理员', roleId: 'platformadmin', org: '信息科' },
    roles: [{ id: 'platformadmin', name: '平台管理员', external: false }],
    projects: [{
      id: CUR_PROJ, name: '播报验证项目', type: 'BASIC', date: '2026-11-05', scenarioId: 'S2026-001',
      level: 'GENERAL', status: '未开始', target: 'ALL', units: '', members: '', observers: '',
      mode: '基础版', paused_sec: 0,
    }],
    activeProjectId: CUR_PROJ,
    scenarios: [{ id: 'S2026-001', name: '验证场景', desc: '', alert: '' }],
    scriptNodes: [], tasks: [], formTemplates: [], messages: [],
    notifs: notifList(),
    chat: { inner: [], external: [] },
    evidence: [], snapshots: [], timeline: [], seqFlow: [], scoreItems: [], scores: [],
    ledger: [], knowledge: [], auditLogs: [], impact: [],
    dict: { role_feed_mapping: { platformadmin: 'messages' } },
    simData: {},
  };
}

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.json': 'application/json; charset=utf-8',
};

/* 静态资源读本地 public/；/api/* 全部走桩，不触达任何数据库 */
async function installMockNetwork(context) {
  await context.route('**/*', async (route) => {
    const req = route.request();
    const u = new URL(req.url());
    const json = (data) => route.fulfill({ status: 200, contentType: 'application/json; charset=utf-8', body: JSON.stringify(data) });

    if (u.pathname.startsWith('/api/')) {
      if (u.pathname === '/api/bootstrap') return json(bootstrapPayload());
      if (u.pathname === '/api/content/notifs' && req.method() === 'GET') return json(notifList());
      if (u.pathname === '/api/content/notifs/read' && req.method() === 'POST') {
        let ids = [];
        try { ids = JSON.parse(req.postData() || '{}').ids || []; } catch (e) {}
        ids.forEach((id) => readIds.add(String(id)));
        return json({ ok: true });
      }
      if (u.pathname === '/api/system/brand') return json({});
      return json([]);
    }

    const rel = u.pathname === '/' ? 'workbench.html' : decodeURIComponent(u.pathname).replace(/^\/+/, '');
    const file = path.join(PUBLIC_DIR, rel);
    if (!file.startsWith(PUBLIC_DIR) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) {
      return route.fulfill({ status: 404, contentType: 'text/plain', body: 'not found' });
    }
    return route.fulfill({
      status: 200,
      contentType: MIME[path.extname(file).toLowerCase()] || 'application/octet-stream',
      body: fs.readFileSync(file),
    });
  });
}

/* ---------------- 语音桩 ---------------- */
function initPayload(auth) {
  return { token: auth.token, role: auth.role, user: auth.user.name, org: auth.user.org || '' };
}

async function installStubs(context, auth) {
  await context.addInitScript((p) => {
    try {
      sessionStorage.setItem('pzy_token', p.token);
      sessionStorage.setItem('pzy_role', JSON.stringify(p.role));
      sessionStorage.setItem('pzy_user', p.user);
      sessionStorage.setItem('pzy_org', p.org);
    } catch (e) {}
    window.__spokenCalls = [];
    window.__voiceStubInstalled = false;
    var s = window.speechSynthesis;
    if (!s) return;
    s.speak = function (u) {
      window.__spokenCalls.push({ text: String(u.text), at: Date.now() });
      setTimeout(function () { try { if (u.onend) u.onend(); } catch (e) {} }, 25);
    };
    s.cancel = function () {};
    s.getVoices = function () { return []; };
    window.__voiceStubInstalled = true;
  }, initPayload(auth));
}

async function login() {
  const r = await fetch(BASE + '/api/auth/login', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  });
  if (!r.ok) throw new Error('login failed ' + r.status + ' ' + await r.text());
  return r.json();
}

/* ---------------- 断言工具 ---------------- */
const results = [];
function check(name, ok, detail) {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' :: ' + detail : ''}`);
}

async function waitReady(page) {
  await page.waitForFunction(() => window.MOCK && Array.isArray(window.MOCK.PROJECTS) && window.MOCK.PROJECTS.length > 0, null, { timeout: 30000 });
  await page.waitForTimeout(3000);
}

async function spokenTexts(page) {
  return page.evaluate(() => (window.__spokenCalls || []).map((x) => x.text));
}

function countContaining(texts, needle) {
  return texts.filter((t) => t.indexOf(needle) >= 0).length;
}

function sameTexts(texts) {
  return texts.length === EXPECTED.length && EXPECTED.every((e) => texts.indexOf(e) >= 0);
}

function noTitle(texts) {
  return texts.every((t) => TITLES.every((title) => t.indexOf(title) < 0));
}

async function openPage(context) {
  const page = await context.newPage();
  await page.goto(BASE + '/workbench.html', { waitUntil: 'load' });
  return page;
}

/* 跨标签页：同一消息只在一个标签发声 */
async function multiTabCheck(browser, auth) {
  for (let i = 1; i <= 2; i++) {
    const c = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
    if (MODE === 'mock') await installMockNetwork(c);
    await installStubs(c, auth);
    const p1 = await c.newPage();
    const p2 = await c.newPage();
    await Promise.all([
      p1.goto(BASE + '/workbench.html', { waitUntil: 'load' }),
      p2.goto(BASE + '/workbench.html', { waitUntil: 'load' }),
    ]);
    await Promise.all([waitReady(p1), waitReady(p2)]);
    await p1.waitForTimeout(4000);
    const a = await spokenTexts(p1);
    const b = await spokenTexts(p2);
    const all = a.concat(b);
    /* 口径：同一消息跨标签页只发声一次；允许两条消息分别落在不同标签（合计不超、不重复） */
    check(`跨标签页第${i}轮合计播报 ${EXPECTED.length} 次、无重复发声`,
      all.length === EXPECTED.length && sameTexts(all),
      `tab1=${JSON.stringify(a)} tab2=${JSON.stringify(b)}`);
    check(`跨标签页第${i}轮文本逐字正确且不含标题`, sameTexts(all) && noTitle(all), JSON.stringify(all));
    await c.close();
  }
}

async function run() {
  let auth;
  if (MODE === 'vm') {
    if (!PASS) throw new Error('VERIFY_MODE=vm 需要 VERIFY_PASS 环境变量');
    auth = await login();
    console.log('login ok, role=' + auth.role.id);
  } else {
    auth = {
      token: 'mock-token',
      role: {
        id: 'platformadmin', name: '平台管理员', external: false,
        nav: ['dashboard', 'workbench', 'collab', 'scenarios', 'projects', 'evidence', 'knowledge',
          'replay', 'evaluation', 'report', 'ledger', 'admin'],
      },
      user: { name: 'admin', org: '信息科' },
    };
    console.log('mock mode: 本地 public/ + /api 桩（不触达任何数据库）');
  }

  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });

  /* ---------- 主功能验证：1920x1080 ---------- */
  const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  if (MODE === 'mock') await installMockNetwork(ctx);
  await installStubs(ctx, auth);
  const page = await ctx.newPage();
  const errors = [];
  page.on('console', (m) => { if (m.type() === 'error') errors.push('console: ' + m.text()); });
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));

  await page.goto(BASE + '/workbench.html', { waitUntil: 'load' });
  await waitReady(page);
  const stubOk = await page.evaluate(() => window.__voiceStubInstalled === true);
  check('语音桩已注入', stubOk, String(stubOk));

  const texts = await spokenTexts(page);
  check('① 只播当前项目未读正文：恰好 2 次（空正文消息 0 次）', sameTexts(texts), JSON.stringify(texts));
  check('② 当前项目已读消息播报 0 次', countContaining(texts, 'CUR-READ') === 0, 'count=' + countContaining(texts, 'CUR-READ'));
  check('③ 其他项目消息播报 0 次', countContaining(texts, 'OTHER') === 0, 'count=' + countContaining(texts, 'OTHER'));
  check('④ 全局公告播报 0 次', countContaining(texts, 'GLOBAL') === 0, 'count=' + countContaining(texts, 'GLOBAL'));
  check('⑤ 播报文本与 descr 逐字相等，且不含任何 title 子串',
    sameTexts(texts) && noTitle(texts),
    'expected=' + JSON.stringify(EXPECTED) + ' actual=' + JSON.stringify(texts));
  check('⑥ 空正文消息（EMPTY-TITLE-MUST-NOT-SPEAK）播报 0 次',
    countContaining(texts, 'EMPTY-TITLE') === 0 && !texts.some((t) => t.length === 0),
    JSON.stringify(texts));
  check('⑦ 标点/数字/英文混排正文逐字相等、未被压缩',
    texts.indexOf(MIXED_DESC) >= 0, 'expected=' + JSON.stringify(MIXED_DESC));
  for (const t of texts) console.log('    播报文本 [' + t.length + '] = ' + JSON.stringify(t));

  console.log('    等待 2 个轮询周期（15s x 2）…');
  await page.waitForTimeout(31000);
  const texts2 = await spokenTexts(page);
  check('⑧ 2 个轮询周期后累计次数不增加', texts2.length === texts.length, `before=${texts.length} after=${texts2.length}`);

  /* 刷新：已读仍 0，未读再播 2 次（仍只播正文） */
  await page.reload({ waitUntil: 'load' });
  await waitReady(page);
  const texts3 = await spokenTexts(page);
  check('⑨a 刷新后已读消息仍 0 次', countContaining(texts3, 'CUR-READ') === 0, JSON.stringify(texts3));
  check('⑨b 刷新后未读消息再播 2 次且只播正文', sameTexts(texts3) && noTitle(texts3), JSON.stringify(texts3));

  /* 跨标签页：同一消息只在一个标签页发声 */
  await multiTabCheck(browser, auth);

  /* 标记已读后再等一个周期：不再播 */
  let readStatus;
  if (MODE === 'mock') {
    readStatus = await page.evaluate(() => fetch('/api/content/notifs/read', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ ids: ['NV_CUR_UNREAD', 'NV_CUR_MIXED', 'NV_CUR_EMPTY'] }),
    }).then((r) => r.status));
  } else {
    readStatus = await fetch(BASE + '/api/content/notifs/read', {
      method: 'POST',
      headers: { 'content-type': 'application/json', authorization: 'Bearer ' + auth.token },
      body: JSON.stringify({ ids: ['NV_CUR_UNREAD', 'NV_CUR_MIXED', 'NV_CUR_EMPTY'] }),
    }).then((r) => r.status);
  }
  check('标记已读接口返回 200', readStatus === 200, 'status=' + readStatus);
  await page.waitForTimeout(17000);
  const texts4 = await spokenTexts(page);
  check('⑩ 标记已读后一个周期不再播报', texts4.length === texts3.length, `before=${texts3.length} after=${texts4.length}`);

  const overflow = await page.evaluate(() => ({
    sw: document.documentElement.scrollWidth,
    cw: document.documentElement.clientWidth,
  }));
  check('1920x1080 无横向溢出', overflow.sw <= overflow.cw + 1, JSON.stringify(overflow));
  check('1920x1080 控制台零报错', errors.length === 0, errors.join(' | '));
  await ctx.close();

  /* ---------- 另两个视口 ---------- */
  const viewports = [
    { width: 393, height: 873, name: '393x873' },
    { width: 412, height: 915, name: '412x915' },
  ];
  for (const vp of viewports) {
    const c2 = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    if (MODE === 'mock') await installMockNetwork(c2);
    await installStubs(c2, auth);
    const p2 = await c2.newPage();
    const errs = [];
    p2.on('console', (m) => { if (m.type() === 'error') errs.push('console: ' + m.text()); });
    p2.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
    await p2.goto(BASE + '/workbench.html', { waitUntil: 'load' });
    await waitReady(p2);
    const ov = await p2.evaluate(() => ({
      sw: document.documentElement.scrollWidth,
      cw: document.documentElement.clientWidth,
    }));
    check(vp.name + ' 无横向溢出', ov.sw <= ov.cw + 1, JSON.stringify(ov));
    check(vp.name + ' 控制台零报错', errs.length === 0, errs.join(' | '));
    await c2.close();
  }

  await browser.close();
  const failed = results.filter((x) => !x.ok);
  console.log('\nSUMMARY: ' + (results.length - failed.length) + '/' + results.length + ' passed');
  if (failed.length) {
    console.log('FAILED: ' + failed.map((f) => f.name).join('; '));
    process.exitCode = 1;
  }
}

run().catch((e) => { console.error('FATAL', e); process.exitCode = 1; });
