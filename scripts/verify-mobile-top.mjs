// verify-mobile-top.mjs —— 手机端"顶部超界"修复验证
// node scripts/verify-mobile-top.mjs <before|after>
import { chromium } from 'playwright';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dir = path.dirname(fileURLToPath(import.meta.url));
const SHOTS = path.join(__dir, '..', '.trae', 'tmp', 'screenshots', 'mobile-shots');
fs.mkdirSync(SHOTS, { recursive: true });
const MODE = process.argv[2] || 'after';
// 独立静态副本（scripts/serve-public.mjs），避免使用 8090 的旧构建副本
const BASE = process.env.BASE || 'http://127.0.0.1:8099';

const CASES = [
  { name: '360x740', w: 360, h: 740 },
  { name: '393x873', w: 393, h: 873 },
  { name: '412x915', w: 412, h: 915 },
  { name: '393x700-toolbar', w: 393, h: 700, toolbar: true }, // 可视高度被工具栏占用
  { name: '1920x1080-desktop', w: 1920, h: 1080, dsf: 1, desktop: true }, // 桌面回归
];
const PAGES = ['index', 'dashboard', 'projects', 'workbench', 'evaluation'];

const NAV = ['dashboard', 'projects', 'scenarios', 'workbench', 'collab', 'evidence', 'knowledge',
  'replay', 'evaluation', 'report', 'ledger', 'admin', 'dictionary', 'password'];

const MOCK_BOOTSTRAP = {
  serverTime: Date.now(),
  currentUser: { id: 'u1', name: '测试用户', roleId: 'commander', org: '示例医院' },
  roles: [{ id: 'commander', name: '总指挥', perms: [], nav: NAV }],
  users: [], projects: [], scenarios: [], scriptNodes: [], tasks: [], formTemplates: [],
  messages: [], notifs: [], chat: { inner: [], external: [] }, evidence: [], snapshots: [],
  timeline: [], seqFlow: [], scoreItems: [], scores: [], ledger: [], knowledge: [],
  auditLogs: [], impact: [], dict: {}, simData: {},
};

const measure = () => {
  const r = el => { if (!el) return null; const b = el.getBoundingClientRect();
    return { top: Math.round(b.top), bottom: Math.round(b.bottom), h: Math.round(b.h || b.height),
      text: (el.textContent || '').trim().slice(0, 14) }; };
  const el = s => document.querySelector(s);
  return {
    metaViewport: el('meta[name=viewport]')?.content || '(none)',
    innerW: window.innerWidth,
    innerH: window.innerHeight,
    visualH: window.visualViewport ? Math.round(window.visualViewport.height) : null,
    scrollW: document.documentElement.scrollWidth,
    scrollH: document.documentElement.scrollHeight,
    scrollY: Math.round(window.scrollY),
    loginBg: (() => {
      const b = el('.login-bg'); if (!b) return null; const cs = getComputedStyle(b);
      return { position: cs.position, display: cs.display, alignItems: cs.alignItems,
        minHeight: cs.minHeight, paddingTop: cs.paddingTop, overflow: cs.overflow };
    })(),
    screenEntry: r(el('#screenEntry')),
    loginCard: r(el('.login-card')),
    loginH2: r(el('.login-right h2')),
    firstRole: r(el('.role-cell')),
    pageHead: r(el('.page-head h1')),
    pageFirst: r(el('#page > *')),
    topbar: r(el('.topbar')),
  };
};

async function newCtx(browser, c) {
  const ctx = await browser.newContext({
    viewport: { width: c.w, height: c.h },
    deviceScaleFactor: c.dsf || 3, isMobile: !c.desktop, hasTouch: !c.desktop,
  });
  await ctx.addInitScript(() => {
    sessionStorage.setItem('pzy_token', 'mock-token-for-layout-test');
    sessionStorage.setItem('pzy_role', JSON.stringify({ id: 'commander', name: '总指挥', perms: [],
      nav: ['dashboard', 'projects', 'scenarios', 'workbench', 'collab', 'evidence', 'knowledge',
        'replay', 'evaluation', 'report', 'ledger', 'admin', 'dictionary', 'password'] }));
    sessionStorage.setItem('pzy_user', '测试用户');
  });
  return ctx;
}

const ROLES = [
  { id: 'duty', name: '信息科值班员' }, { id: 'netsec', name: '网络安全管理员' },
  { id: 'dpo', name: '数据安全管理员' }, { id: 'sysadmin', name: '系统管理员' },
  { id: 'itlead', name: '信息科负责人' }, { id: 'commander', name: '院应急指挥部' },
  { id: 'medlead', name: '医务处' }, { id: 'cashier', name: '收费窗口' },
  { id: 'health', name: '市卫生健康局' }, { id: 'cac', name: '市网信办' },
  { id: 'police', name: '市网安大队' }, { id: 'platformadmin', name: '平台系统管理员' },
];
const ACCOUNTS = [
  { username: 'duty01', name: '值班一号', roles: [{ id: 'duty' }] },
  { username: 'netsec01', name: '网安一号', roles: [{ id: 'netsec' }] },
];
const CAPTCHA_SVG = 'data:image/svg+xml;base64,' + Buffer.from(
  '<svg width="120" height="40" xmlns="http://www.w3.org/2000/svg">' +
  '<rect width="100%" height="100%" fill="rgb(230,235,245)"/>' +
  '<text x="10" y="28" fill="rgb(30,40,60)" font-size="20">43 - 30 = ?</text></svg>').toString('base64');

async function wire(page) {
  // 兜底必须先注册（Playwright 后注册优先匹配，具体接口需后注册才能覆盖兜底）
  await page.route(/\/api\//, route =>
    route.fulfill({ status: 200, contentType: 'application/json',
      body: JSON.stringify({ value: [], items: [], total: 0, ok: true }) }));
  await page.route('**/api/bootstrap', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_BOOTSTRAP) }));
  // 内页把 /content/projects 当数组用
  await page.route('**/api/content/projects', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }));
  await page.route('**/api/auth/roles', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ROLES) }));
  await page.route('**/api/auth/accounts', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ACCOUNTS) }));
  await page.route('**/api/auth/captcha', route =>
    route.fulfill({ status: 200, contentType: 'application/json',
      body: JSON.stringify({ id: 'cap-test', image: CAPTCHA_SVG, ttl: 120 }) }));
  await page.route('**/api/system/brand', route =>
    route.fulfill({ status: 200, contentType: 'application/json',
      body: JSON.stringify({ unitName: '示例医院', systemName: '网络安全应急演练推演平台' }) }));
}

async function run() {
  const browser = await chromium.launch({ headless: true, channel: 'chrome', args: ['--no-sandbox'] });
  const rows = [];
  const errors = [];

  for (const c of CASES) {
    const ctx = await newCtx(browser, c);
    const page = await ctx.newPage();
    page.on('pageerror', e => errors.push(`[${c.name}] pageerror: ${e.message}`));
    page.on('console', m => { if (m.type() === 'error') errors.push(`[${c.name}] console: ${m.text()}`); });
    await wire(page);

    for (const key of PAGES) {
      await page.goto(`${BASE}/${key}.html`, { waitUntil: 'load', timeout: 20000 });
      await page.waitForTimeout(1200);
      // 保证从页面顶部开始测量
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.waitForTimeout(150);

      let m = await page.evaluate(measure);

      // 工具栏动态压缩模拟：加载完成后把可视高度从 H 压到 0.9H，再测一次
      if (c.toolbar) {
        await page.setViewportSize({ width: c.w, height: Math.round(c.h * 0.9) });
        await page.waitForTimeout(400);
        const m2 = await page.evaluate(measure);
        m.shrunk = m2; m.shrunkH = m2.innerH;
        await page.setViewportSize({ width: c.w, height: c.h });
        await page.waitForTimeout(300);
      }

      rows.push({ case: c.name, page: key, desktop: !!c.desktop, ...m });
      const fn = path.join(SHOTS, `top-${MODE}-${key}-${c.name}.png`);
      await page.screenshot({ path: fn });
    }
    await ctx.close();
  }
  await browser.close();

  // ---------------- 输出 ----------------
  console.log(`\n===== ${MODE.toUpperCase()} 测量 =====`);
  for (const m of rows) {
    console.log(`\n[${m.case}] ${m.page}  innerH=${m.innerH} visualH=${m.visualH} scrollW=${m.scrollW}/${m.innerW} scrollH=${m.scrollH}`);
    if (m.loginBg) console.log(`  .login-bg: position=${m.loginBg.position} display=${m.loginBg.display} align=${m.loginBg.alignItems} minH=${m.loginBg.minHeight} padTop=${m.loginBg.paddingTop} overflow=${m.loginBg.overflow}`);
    const fmt = (n, v) => v ? `  ${n}: top=${v.top} bottom=${v.bottom} h=${v.h}${v.text ? ' "' + v.text + '"' : ''}` : '';
    ['screenEntry', 'loginCard', 'loginH2', 'firstRole', 'pageHead', 'topbar'].forEach(k => { if (m[k]) console.log(fmt(k, m[k])); });
    if (m.shrunk) {
      const s = m.shrunk;
      console.log(`  ↳ 压缩后 innerH=${s.innerH} loginCard.top=${s.loginCard ? s.loginCard.top : '-'} firstRole.top=${s.firstRole ? s.firstRole.top : '-'} pageHead.top=${s.pageHead ? s.pageHead.top : '-'}`);
    }
  }

  // ---------------- 断言 ----------------
  console.log(`\n===== 断言 =====`);
  let pass = 0, fail = 0;
  const bad = [];
  for (const m of rows) {
    const ck = [];
    const isLogin = m.page === 'index';
    if (isLogin) {
      ck.push(['.login-card top>=0', m.loginCard ? m.loginCard.top >= 0 : false]);
      ck.push(['标题 h2 完整可见', m.loginH2 ? (m.loginH2.top >= 0 && m.loginH2.bottom <= m.innerH) : false]);
      ck.push(['首排角色卡完整可见', m.firstRole ? (m.firstRole.top >= 0 && m.firstRole.bottom <= m.innerH) : false]);
      if (!m.desktop) {
        ck.push(['.login-bg 非 fixed 居中', m.loginBg ? !(m.loginBg.position === 'fixed' && m.loginBg.alignItems === 'center') : false]);
      }
    } else {
      ck.push(['topbar top>=0', m.topbar ? m.topbar.top >= 0 : false]);
      ck.push(['页面首个内容块 top>=0', m.pageFirst ? m.pageFirst.top >= 0 : false]);
      ck.push(['页面标题未超界', m.pageHead ? m.pageHead.top >= 0 : true]);
    }
    ck.push(['横向零溢出', m.scrollW === m.innerW]);
    ck.push(['滚动位置在顶部', m.scrollY === 0]);
    if (m.shrunk) {
      const s = m.shrunk;
      ck.push([`压缩后(${s.innerH}px) 卡片顶部仍 >=0`, isLogin && s.loginCard ? s.loginCard.top >= 0 : true]);
      ck.push([`压缩后(${s.innerH}px) 首排角色仍 >=0`, isLogin && s.firstRole ? s.firstRole.top >= 0 : true]);
    }
    for (const [name, ok] of ck) { if (ok) pass++; else { fail++; bad.push(`[${m.case}] ${m.page} — ${name}`); } }
  }
  bad.forEach(b => console.log('  ❌ ' + b));
  console.log(`  通过 ${pass} / 失败 ${fail}`);
  console.log(`控制台错误: ${errors.length ? '\n' + errors.join('\n') : '无'}`);
  console.log(`截图目录: .trae/tmp/screenshots/mobile-shots/top-${MODE}-*.png`);
}
run().catch(e => { console.error('FATAL', e); process.exit(1); });
