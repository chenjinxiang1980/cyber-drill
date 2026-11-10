import { chromium } from 'playwright';

/*
 * 真机只读验证：直接访问 VM 上的线上实例（前端 app.js 已与本地逐字节一致）。
 * 只读：仅登录 + GET；不调用 /api/content/notifs/read，不修改任何业务数据。
 * 口令只从 VERIFY_PASS 环境变量读取，脚本内不保存。
 */
const BASE = process.env.VERIFY_BASE || 'http://127.0.0.1:8090';
const USER = process.env.VERIFY_USER || 'admin';
const PASS = process.env.VERIFY_PASS || '';

const results = [];
function check(name, ok, detail) {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' :: ' + detail : ''}`);
}

/* 验证码是算术题 SVG：解析题目并计算，不绕过校验逻辑 */
async function login() {
  const c = await (await fetch(BASE + '/api/auth/captcha')).json();
  const svg = Buffer.from(String(c.image).replace('data:image/svg+xml;base64,', ''), 'base64').toString('utf8');
  const m = svg.match(/>\s*(\d+)\s*([+\-])\s*(\d+)\s*=/);
  if (!m) throw new Error('验证码解析失败');
  const ans = m[2] === '+' ? Number(m[1]) + Number(m[3]) : Number(m[1]) - Number(m[3]);
  const r = await fetch(BASE + '/api/auth/login', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS, captchaId: c.id, captchaCode: String(ans) }),
  });
  if (!r.ok) throw new Error('login failed ' + r.status + ' ' + (await r.text()));
  return r.json();
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
  }, auth);
}

async function waitReady(page) {
  await page.waitForFunction(() => window.MOCK && Array.isArray(window.MOCK.PROJECTS) && window.MOCK.PROJECTS.length > 0, null, { timeout: 30000 });
  await page.waitForTimeout(4000);
}

const spoken = (page) => page.evaluate(() => (window.__spokenCalls || []).map((x) => x.text));

async function run() {
  if (!PASS) throw new Error('缺少 VERIFY_PASS 环境变量');
  const auth0 = await login();
  const boot = await (await fetch(BASE + '/api/bootstrap', { headers: { authorization: 'Bearer ' + auth0.token } })).json();
  const pid = boot.activeProjectId;
  const role = (boot.roles || []).filter((r) => r.id === (boot.currentUser || {}).roleId)[0];
  if (!pid || !role) throw new Error('bootstrap 缺少 activeProjectId 或当前角色');
  const auth = {
    token: auth0.token,
    role,
    user: { name: boot.currentUser.name, org: boot.currentUser.org || '' },
  };

  const notifs = Array.isArray(boot.notifs) ? boot.notifs : [];
  const expected = notifs.filter((n) => !n.read && n.projectId === pid && n.desc).map((n) => n.desc);
  const forbiddenDescs = notifs.filter((n) => n.read || n.projectId !== pid || !n.desc).map((n) => n.desc).filter(Boolean);
  const titles = notifs.map((n) => n.title).filter(Boolean);
  console.log('当前项目 ' + pid + '；未读正文条数=' + expected.length + '；消息总数=' + notifs.length);

  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });

  const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  await installStubs(ctx, auth);
  const page = await ctx.newPage();
  const errors = [];
  page.on('console', (m) => { if (m.type() === 'error') errors.push('console: ' + m.text()); });
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
  await page.goto(BASE + '/workbench.html', { waitUntil: 'load' });
  await waitReady(page);

  const t1 = await spoken(page);
  check('线上语音桩已注入', await page.evaluate(() => window.__voiceStubInstalled === true), '');
  check('① 播报条数 = 当前项目未读且正文非空的消息数', t1.length === expected.length, `actual=${t1.length} expected=${expected.length}`);
  check('② 播报文本逐字等于 descr', t1.length === expected.length && expected.every((e) => t1.indexOf(e) >= 0), JSON.stringify(t1));
  check('③ 播报文本不含任何 title 子串', t1.every((t) => titles.every((ti) => t.indexOf(ti) < 0)), JSON.stringify(titles));
  check('④ 已读/跨项目/全局消息正文均未播报', t1.every((t) => forbiddenDescs.indexOf(t) < 0), '');
  t1.forEach((t) => console.log('    播报文本 [' + t.length + '] = ' + JSON.stringify(t)));

  await page.waitForTimeout(31000);
  const t2 = await spoken(page);
  check('⑤ 2 个轮询周期不重复播报', t2.length === t1.length, `before=${t1.length} after=${t2.length}`);

  await page.reload({ waitUntil: 'load' });
  await waitReady(page);
  const t3 = await spoken(page);
  check('⑥ 刷新后重新播报同一批未读正文', t3.length === expected.length && expected.every((e) => t3.indexOf(e) >= 0), JSON.stringify(t3));
  check('⑦ 刷新后仍不含标题', t3.every((t) => titles.every((ti) => t.indexOf(ti) < 0)), '');

  const ov = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }));
  check('1920x1080 无横向溢出', ov.sw <= ov.cw + 1, JSON.stringify(ov));
  check('1920x1080 控制台零报错', errors.length === 0, errors.join(' | '));
  await ctx.close();

  /* 跨标签页：同一消息只发声一次 */
  const c2 = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  await installStubs(c2, auth);
  const p1 = await c2.newPage();
  const p2 = await c2.newPage();
  await Promise.all([p1.goto(BASE + '/workbench.html', { waitUntil: 'load' }), p2.goto(BASE + '/workbench.html', { waitUntil: 'load' })]);
  await Promise.all([waitReady(p1), waitReady(p2)]);
  await p1.waitForTimeout(4000);
  const a = await spoken(p1);
  const b = await spoken(p2);
  const all = a.concat(b);
  check('⑧ 跨标签页合计不重复', all.length === expected.length && expected.every((e) => all.indexOf(e) >= 0), `tab1=${JSON.stringify(a)} tab2=${JSON.stringify(b)}`);
  await c2.close();

  /* 其余两个视口 */
  for (const vp of [{ width: 393, height: 873, name: '393x873' }, { width: 412, height: 915, name: '412x915' }]) {
    const c3 = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    await installStubs(c3, auth);
    const p3 = await c3.newPage();
    const errs = [];
    p3.on('console', (m) => { if (m.type() === 'error') errs.push('console: ' + m.text()); });
    p3.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
    await p3.goto(BASE + '/workbench.html', { waitUntil: 'load' });
    await waitReady(p3);
    const o = await p3.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }));
    check(vp.name + ' 无横向溢出', o.sw <= o.cw + 1, JSON.stringify(o));
    check(vp.name + ' 控制台零报错', errs.length === 0, errs.join(' | '));
    await c3.close();
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
