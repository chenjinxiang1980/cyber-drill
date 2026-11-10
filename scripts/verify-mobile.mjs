// Playwright 三视口验证脚本（纯前端渲染断言，无需真实登录态）
// 跑法：cd e:\Trae\Emergency; node scripts\verify-mobile.mjs
import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:8090';
const VIEWPORTS = [
  { name: 'phone-375x812',   width: 375,  height: 812,  isTouch: true  },
  { name: 'tablet-768x1024', width: 768,  height: 1024, isTouch: false },
  { name: 'desktop-1920x1080', width: 1920, height: 1080, isTouch: false },
];

const AUTH_PAGES = [
  { path: 'dashboard.html',   key: 'dashboard' },
  { path: 'projects.html',     key: 'projects' },
  { path: 'scenarios.html',    key: 'scenarios' },
  { path: 'workbench.html',    key: 'workbench' },
  { path: 'admin.html',        key: 'admin' },
  { path: 'password.html',     key: 'password' },
];

const PUB_PAGES = [
  { path: 'index.html',    key: 'index' },
  { path: 'screen.html',   key: 'screen' },
];

const results = [];
function push(page, vp, pass, detail) {
  results.push({ page, viewport: vp.name, pass, detail });
  const icon = pass ? '✅' : '❌';
  console.log(`${icon} [${vp.name}] ${page} — ${detail}`);
}

async function run() {
  const browser = await chromium.launch({ headless: true, channel: 'chrome', args: ['--no-sandbox'] });

  for (const vp of VIEWPORTS) {
    console.log(`\n========== VIEWPORT ${vp.name} (${vp.width}×${vp.height}) ==========`);
    const ctx = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      isMobile: vp.isTouch, hasTouch: vp.isTouch,
    });

    // ===== 公共页面 =====
    for (const p of PUB_PAGES) {
      const page = await ctx.newPage();

      // screen.html 的 API 也需要拦截 401，不然 system.js 会跳回 index.html
      await page.route(/\/api\//, async route => {
        const resp = await route.fetch().catch(() => null);
        if (!resp) { route.fulfill({ status: 200, body: '{"value":[]}' }); return; }
        if (resp.status() === 401 || resp.status() === 403) {
          route.fulfill({ status: 200, body: '{"value":[]}' });
        } else { route.fulfill({ response: resp }); }
      });

      try {
        await page.goto(`${BASE}/public/${p.path}`, { waitUntil: 'load', timeout: 15000 });
        await page.waitForTimeout(2000);

        if (p.key === 'screen') {
          const m = await page.evaluate(() => ({
            href: location.href,
            tipExists: !!document.getElementById('mobileTip'),
            tipDisplay: document.getElementById('mobileTip') ? getComputedStyle(document.getElementById('mobileTip')).display : null,
            bodyTransform: getComputedStyle(document.body).transform,
            bodyStyleTransform: document.body.style.transform,
            bodyPos: getComputedStyle(document.body).position,
            bodyWidth: getComputedStyle(document.body).width,
          }));

          const isSmallOrTouch = vp.width < 1024 || vp.isTouch;
          if (isSmallOrTouch) {
            push(p.key + ':mobileTip', vp, m.tipExists,
              `mobileTip=${m.tipExists}, display=${m.tipDisplay}, href=${m.href}`);
          } else {
            // 桌面端：无 mobileTip，body.style.transform 有值（scale() 被调用）
            // 1920×1080 下 scale=1，所以 computed transform 是 identity
            const isIdentity = m.bodyTransform === 'matrix(1, 0, 0, 1, 0, 0)';
            push(p.key + ':desktopUnchanged', vp, !m.tipExists && isIdentity && m.bodyPos === 'fixed',
              `mobileTip=${m.tipExists}, body.transform="${m.bodyTransform}", body.pos=${m.bodyPos}, body.style.transform="${m.bodyStyleTransform}"`);
          }
        } else if (p.key === 'index') {
          const m = await page.evaluate(() => {
            const root = document.documentElement;
            const btn = document.getElementById('btnLogin');
            const input = document.getElementById('inUser');
            const card = document.querySelector('.login-card');
            const left = document.querySelector('.login-left');
            return {
              scrollW: root.scrollWidth, innerW: window.innerWidth,
              btnVisible: !!btn && btn.offsetParent !== null,
              btnRect: btn ? btn.getBoundingClientRect() : null,
              bodyFontSize: parseFloat(getComputedStyle(document.body).fontSize),
              viewportMeta: document.querySelector('meta[name="viewport"]')?.content,
              cardCols: card ? getComputedStyle(card).gridTemplateColumns : null,
              leftDisplay: left ? getComputedStyle(left).display : null,
            };
          });
          push(p.key + ':overflow',   vp, m.scrollW <= m.innerW + 1, `scrollW=${m.scrollW}, innerW=${m.innerW}`);
          push(p.key + ':btnVisible', vp, m.btnVisible, `btnRect=${JSON.stringify(m.btnRect)}`);
          push(p.key + ':btnHit',     vp, m.btnRect && m.btnRect.width >= 36 && m.btnRect.height >= 36,
            `hit area w=${m.btnRect?.width} h=${m.btnRect?.height}`);
          push(p.key + ':body14px',   vp, m.bodyFontSize >= 13.5, `body font-size=${m.bodyFontSize}`);
          push(p.key + ':viewport',   vp, !!(m.viewportMeta && m.viewportMeta.includes('width=device-width')),
            `meta viewport="${m.viewportMeta}"`);

          if (vp.width <= 767) {
            push(p.key + ':mobileLayout', vp, m.leftDisplay === 'none',
              `.login-left.display=${m.leftDisplay}, card.cols=${m.cardCols}`);
          }
        }
      } catch (e) {
        push(p.key, vp, false, 'ERROR: ' + e.message);
      } finally { await page.close().catch(() => {}); }
    }

    // ===== 需登录的页面 =====
    for (const p of AUTH_PAGES) {
      const page = await ctx.newPage();
      await page.addInitScript((roleJson) => {
        try {
          sessionStorage.setItem('pzy_role', roleJson);
          sessionStorage.setItem('pzy_user', '平台管理员');
          sessionStorage.setItem('pzy_token', 'playwright-dummy-token');
        } catch(e){}
      }, JSON.stringify({
        id: 'platformadmin', name: '平台管理员',
        nav: ['dashboard','workbench','collab','projects','scenarios','evidence','replay','evaluation','ledger','knowledge','admin','dictionary','report','screen','password'],
      }));

      await page.route(/\/api\//, async route => {
        const resp = await route.fetch().catch(() => null);
        if (!resp) { route.fulfill({ status: 200, body: '{"value":[]}' }); return; }
        if (resp.status() === 401 || resp.status() === 403) {
          route.fulfill({ status: 200, body: '{"value":[]}' });
        } else { route.fulfill({ response: resp }); }
      });

      try {
        await page.goto(`${BASE}/public/${p.path}`, { waitUntil: 'domcontentloaded', timeout: 15000 });
        await page.waitForTimeout(2500);

        const m = await page.evaluate(() => {
          const root = document.documentElement;
          const sidebar = document.querySelector('.sidebar');
          const mainWrap = document.querySelector('.main-wrap');
          const navToggle = document.getElementById('navToggle');
          const body = document.body;
          return {
            scrollW: root.scrollWidth, innerW: window.innerWidth,
            bodyPos: getComputedStyle(body).position,
            bodyWidth: getComputedStyle(body).width,
            bodyFontSize: parseFloat(getComputedStyle(body).fontSize),
            sidebarRect: sidebar ? sidebar.getBoundingClientRect() : null,
            sidebarTransform: sidebar ? getComputedStyle(sidebar).transform : null,
            mainMarginLeft: mainWrap ? getComputedStyle(mainWrap).marginLeft : null,
            navToggleVisible: navToggle ? navToggle.offsetParent !== null : false,
          };
        });

        push(p.key + ':overflow',    vp, m.scrollW <= m.innerW + 1, `scrollW=${m.scrollW}, innerW=${m.innerW}`);
        push(p.key + ':noFixedBody', vp, m.bodyPos !== 'fixed' && parseFloat(m.bodyWidth || '0') < 1921,
          `body.position=${m.bodyPos}, width=${m.bodyWidth}`);
        push(p.key + ':body14px',    vp, m.bodyFontSize >= 13.5, `body font-size=${m.bodyFontSize}`);

        if (vp.width <= 767) {
          push(p.key + ':sidebarHidden', vp, parseFloat(m.mainMarginLeft || '999') <= 4,
            `main-wrap.marginLeft=${m.mainMarginLeft}, sidebar.transform="${m.sidebarTransform}"`);
          push(p.key + ':burgerBtn', vp, m.navToggleVisible === true,
            `navToggle.visible=${m.navToggleVisible}`);
        } else {
          push(p.key + ':sidebarVisible', vp, m.sidebarRect && m.sidebarRect.width > 100,
            `sidebar.rect.width=${m.sidebarRect?.width}`);
        }
      } catch (e) {
        push(p.key, vp, false, 'ERROR: ' + e.message);
      } finally { await page.close().catch(() => {}); }
    }

    await ctx.close();
  }

  await browser.close();

  console.log('\n\n================ SUMMARY ================');
  const pass = results.filter(r => r.pass).length;
  const fail = results.filter(r => !r.pass).length;
  console.log(`总共 ${results.length} 条断言：✅ ${pass} 通过，❌ ${fail} 失败`);
  if (fail > 0) {
    console.log('\n失败详情：');
    results.filter(r => !r.pass).forEach(r => console.log(`  [${r.viewport}] ${r.page} — ${r.detail}`));
  }
}

run().catch(e => { console.error(e); process.exit(1); });
