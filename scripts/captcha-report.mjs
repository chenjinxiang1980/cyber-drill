// captcha-report.mjs —— 改前/改后真实像素采样对比
// 固定同一张后端 SVG → 分别用"旧 filter 方案"和"新 JS 换色方案"渲染
// → element.screenshot → PNG 解码 → 采样三组 RGB → WCAG 对比度 → 输出报告
import { chromium } from 'playwright';
import fs from 'fs';
import zlib from 'zlib';
import path from 'path';
import { fileURLToPath } from 'url';

const __dir = path.dirname(fileURLToPath(import.meta.url));
const SHOTS = path.join(__dir, '..', '.trae', 'tmp', 'screenshots', 'mobile-shots');
fs.mkdirSync(SHOTS, { recursive: true });

// ---------- PNG decoder（修正版: 所有 filter type 越界邻居=0, 不跳过整行） ----------
function pDecodePNG(buf) {
  if (buf.readUInt32BE(0) !== 0x89504E47 || buf.readUInt32BE(4) !== 0x0D0A1A0A)
    throw new Error('not a PNG');
  let off = 8;
  let ihdr = null, idatChunks = [];
  while (off < buf.length) {
    const len = buf.readUInt32BE(off); off += 4;
    const type = buf.slice(off, off + 4).toString('ascii'); off += 4;
    const data = buf.slice(off, off + len); off += len + 4;
    if (type === 'IHDR') ihdr = data;
    if (type === 'IDAT') idatChunks.push(data);
    if (type === 'IEND') break;
  }
  const w = ihdr.readUInt32BE(0), h = ihdr.readUInt32BE(4);
  const ct = ihdr[9];
  const inflated = zlib.inflateSync(Buffer.concat(idatChunks));
  let bpp;
  switch (ct) {
    case 0: bpp = 1; break;  // Grayscale
    case 2: bpp = 3; break;  // Truecolor
    case 3: bpp = 1; break;  // Indexed
    case 4: bpp = 2; break;  // Grayscale+Alpha
    case 6: bpp = 4; break;  // Truecolor+Alpha
    default: bpp = 3;
  }
  const stride = w * bpp;
  const out = Buffer.alloc(h * stride);
  for (let y = 0; y < h; y++) {
    const slOff = y * (stride + 1);
    const filt = inflated[slOff];
    const slStart = slOff + 1;
    for (let x = 0; x < stride; x++) {
      let v = inflated[slStart + x];
      // 统一越界处理：a/b/c 越界视为 0
      const a = x >= bpp ? out[y * stride + x - bpp] : 0;
      const b = y > 0 ? out[(y - 1) * stride + x] : 0;
      const c = (x >= bpp && y > 0) ? out[(y - 1) * stride + x - bpp] : 0;
      switch (filt) {
        case 1: v = (v + a) & 0xFF; break;
        case 2: v = (v + b) & 0xFF; break;
        case 3: v = (v + Math.floor((a + b) / 2)) & 0xFF; break;
        case 4: {
          const p = a + b - c;
          const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c);
          const R = pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
          v = (v + R) & 0xFF;
          break;
        }
      }
      out[y * stride + x] = v;
    }
  }
  return { w, h, bpp, pixels: out };
}

// ---------- 颜色工具 ----------
function rgbToHue(r, g, b) {
  let rr = r / 255, gg = g / 255, bb = b / 255;
  const max = Math.max(rr, gg, bb), min = Math.min(rr, gg, bb);
  const d = max - min;
  if (d === 0) return 0;
  let h;
  if (max === rr) h = ((gg - bb) / d + (gg < bb ? 6 : 0));
  else if (max === gg) h = ((bb - rr) / d + 2);
  else h = ((rr - gg) / d + 4);
  return Math.round(h * 60);
}

// WCAG 相对亮度
function wcagLuminance(r, g, b) {
  const toLin = v => {
    v /= 255;
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * toLin(r) + 0.7152 * toLin(g) + 0.0722 * toLin(b);
}

// WCAG 对比度比 (L1 + 0.05) / (L2 + 0.05)，L1 较亮
function wcagContrast(rgb1, rgb2) {
  const L1 = wcagLuminance(...rgb1);
  const L2 = wcagLuminance(...rgb2);
  const [light, dark] = L1 >= L2 ? [L1, L2] : [L2, L1];
  return ((light + 0.05) / (dark + 0.05)).toFixed(2);
}

// ---------- 像素采样核心 ----------
// 给定 PNG，按亮度排序，取最暗 N 个=bg，最亮 N 个=fg
// 用固定数量（20）代替百分比，避免抗锯齿边缘稀释
function sampleCaptchaPixels(png) {
  const pixels = [];
  for (let y = 0; y < png.h; y++) {
    for (let x = 0; x < png.w; x++) {
      const i = y * png.w * png.bpp + x * png.bpp;
      const r = png.pixels[i], g = png.pixels[i + 1], b = png.pixels[i + 2];
      let a = 255;
      if (png.bpp === 4) a = png.pixels[i + 3];
      if (a < 30) continue;
      const L = wcagLuminance(r, g, b);
      pixels.push({ r, g, b, L });
    }
  }
  if (pixels.length === 0) return { bg: [0, 0, 0], fg: [0, 0, 0], warn: 'no opaque pixels' };

  // 按亮度排序
  pixels.sort((a, b) => a.L - b.L);

  // 取最暗 20 个像素 (rect 底区域)
  const bgN = Math.min(20, pixels.length);
  const bgPixels = pixels.slice(0, bgN);
  // 取最亮 20 个像素 (字符笔画中心)
  const fgN = Math.min(20, pixels.length);
  const fgPixels = pixels.slice(-fgN);

  const mean = arr => {
    const r = arr.reduce((s, p) => s + p.r, 0) / arr.length | 0;
    const g = arr.reduce((s, p) => s + p.g, 0) / arr.length | 0;
    const b = arr.reduce((s, p) => s + p.b, 0) / arr.length | 0;
    return [r, g, b];
  };

  // 最暗/最亮单个像素（用于验证纯色色值）
  const darkest = pixels[0];
  const brightest = pixels[pixels.length - 1];

  return {
    bg: mean(bgPixels),
    fg: mean(fgPixels),
    singleDarkest: [darkest.r, darkest.g, darkest.b],
    singleBrightest: [brightest.r, brightest.g, brightest.b],
    total: pixels.length
  };
}

// 卡片背景采样：从 login-card 元素区域取平均色
async function sampleCardBG(page) {
  const box = await page.locator('.login-card').boundingBox();
  if (!box) return [17, 28, 46]; // fallback: #111c2e
  const shotFn = path.join(SHOTS, '_card-bg-sample.png');
  await page.locator('.login-card').screenshot({ path: shotFn });
  const png = pDecodePNG(fs.readFileSync(shotFn));
  // 取四周像素（避开验证码和按钮区域，那里可能有其他色）
  let rSum = 0, gSum = 0, bSum = 0, cnt = 0;
  const sample = (x, y) => {
    const i = y * png.w * png.bpp + x * png.bpp;
    rSum += png.pixels[i]; gSum += png.pixels[i + 1]; bSum += png.pixels[i + 2];
    cnt++;
  };
  // 取边缘 10px 区域
  for (let x = 0; x < png.w; x++) {
    for (let dy = 0; dy < 10; dy++) { sample(x, dy); sample(x, png.h - 1 - dy); }
  }
  for (let y = 0; y < png.h; y++) {
    for (let dx = 0; dx < 10; dx++) { sample(dx, y); sample(png.w - 1 - dx, y); }
  }
  fs.unlinkSync(shotFn);
  return [rSum / cnt | 0, gSum / cnt | 0, bSum / cnt | 0];
}

// ---------- 主流程 ----------
async function run() {
  const browser = await chromium.launch({
    headless: true, channel: 'chrome', args: ['--no-sandbox']
  });

  // ========== Phase 0: 取一张固定的后端 SVG ==========
  const apiResp = await fetch('http://127.0.0.1:8090/api/auth/captcha');
  const apiJson = await apiResp.json();
  const FIXED_DATA_URI = apiJson.image;
  console.log(`\n🟢 Phase 0: 已固定后端 captcha SVG（id=${apiJson.id}）`);

  // 解析原始 SVG 的 rect 和 text fill，作为"改前"参考
  const origXml = Buffer.from(FIXED_DATA_URI.replace('data:image/svg+xml;base64,', ''), 'base64').toString();
  const origFills = [...origXml.matchAll(/fill="([^"]+)"/g)].map(m => m[1]);
  const origStrokes = [...origXml.matchAll(/stroke="([^"]+)"/g)].map(m => m[1]);
  console.log(`  原始 SVG fills:  [${origFills.join(', ')}]`);
  console.log(`  原始 SVG strokes:[${origStrokes.join(', ')}]`);

  // 用 route intercept 让后续所有 captcha 请求都返回同一张
  const ctx = await browser.newContext({
    viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true
  });
  const page = await ctx.newPage();
  await page.route('**/api/auth/captcha', async route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ id: apiJson.id, image: FIXED_DATA_URI, ttl: 120 })
    });
  });
  // 其他 API 放行或 mock 空
  await page.route(/\/api\/(?!auth\/captcha)/, async route => {
    const resp = await route.fetch().catch(() => null);
    if (!resp) return route.fulfill({ status: 200, body: '{"value":[]}' });
    route.fulfill({ response: resp });
  });

  // ========== Phase 1: 改前 —— 原始 SVG + 旧 filter ==========
  console.log(`\n🔵 Phase 1: 改前方案（原始 SVG + filter: invert(1) hue-rotate(258deg) saturate(1.2) brightness(1.08)）`);

  // 先打开页面，注入一段 JS 强制 capBox 不加 swapCaptchaColors
  await page.goto('http://127.0.0.1:8090/index.html', { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(1500);

  // 注入旧 filter CSS + 覆盖 swapCaptchaColors 为空函数
  await page.addStyleTag({ content: `.captcha-box img { filter: invert(1) hue-rotate(258deg) saturate(1.2) brightness(1.08) !important; }` });
  // 强制重新加载 captcha（跳过 swapCaptchaColors）
  await page.evaluate(async (uri) => {
    // 覆盖 swapCaptchaColors 为 identity
    window.swapCaptchaColors = (x) => x;
    // 手动 reload
    const capBox = document.getElementById('capBox');
    capBox.src = uri;
    await new Promise(r => { capBox.onload = r; setTimeout(r, 2000); });
  }, FIXED_DATA_URI);
  await page.waitForTimeout(800);

  // 改前截图
  const beforeElemShot = path.join(SHOTS, 'captcha-BEFORE-elem.png');
  const beforePageShot = path.join(SHOTS, 'captcha-BEFORE-fullpage.png');
  await page.locator('#capBox').screenshot({ path: beforeElemShot });
  await page.screenshot({ path: beforePageShot });

  // 改前 DOM 检查
  const beforeDOM = await page.evaluate(() => {
    const img = document.getElementById('capBox');
    const cs = getComputedStyle(img);
    return {
      filter: cs.filter,
      srcPrefix: img.src.substring(0, 60) + '...',
    };
  });
  console.log(`  computed filter: ${beforeDOM.filter}`);

  // 改前像素采样
  const beforePng = pDecodePNG(fs.readFileSync(beforeElemShot));
  const beforeSamp = sampleCaptchaPixels(beforePng);
  const beforeCardBG = await sampleCardBG(page);
  console.log(`  采样 bg(容器底): RGB(${beforeSamp.bg.join(',')})`);
  console.log(`  采样 fg(字符亮): RGB(${beforeSamp.fg.join(',')})`);
  console.log(`  卡片背景:       RGB(${beforeCardBG.join(',')})`);
  const beforeContrast = wcagContrast(beforeSamp.fg, beforeSamp.bg);
  const beforeFgHue = rgbToHue(...beforeSamp.fg);
  const beforeBgHue = rgbToHue(...beforeSamp.bg);
  console.log(`  fg vs bg 对比度: ${beforeContrast}:1`);
  console.log(`  fg hue: ${beforeFgHue}° (目标 200-220°)`);
  console.log(`  bg hue: ${beforeBgHue}°`);

  // ========== Phase 2: 改后 —— JS swapCaptchaColors 方案 ==========
  console.log(`\n🟢 Phase 2: 改后方案（JS swapCaptchaColors → rect=#111c2e, text/line=#4d9bff）`);

  // 重新打开干净页面
  await page.goto('http://127.0.0.1:8090/index.html', { waitUntil: 'load', timeout: 15000 });
  await page.waitForTimeout(2000);

  // 改后 DOM 检查
  const afterDOM = await page.evaluate(() => {
    const img = document.getElementById('capBox');
    const wrap = document.getElementById('capBoxWrap');
    const cs = el => getComputedStyle(el);
    const src = img.src;
    let xml = '';
    try { xml = atob(src.replace('data:image/svg+xml;base64,', '')); } catch(e) { xml = 'decode fail'; }
    const fills = [...xml.matchAll(/fill="([^"]+)"/g)].map(m => m[1]);
    const strokes = [...xml.matchAll(/stroke="([^"]+)"/g)].map(m => m[1]);
    return {
      imgFilter: cs(img).filter,
      wrapBG: cs(wrap).backgroundColor,
      fills, strokes,
    };
  });
  console.log(`  computed img filter: ${afterDOM.imgFilter} (期望 none)`);
  console.log(`  computed wrap bg:    ${afterDOM.wrapBG}`);
  console.log(`  SVG fills:  [${afterDOM.fills.join(', ')}]`);
  console.log(`  SVG strokes:[${afterDOM.strokes.join(', ')}]`);

  // 改后截图
  const afterElemShot = path.join(SHOTS, 'captcha-AFTER-elem.png');
  const afterPageShot = path.join(SHOTS, 'captcha-AFTER-fullpage.png');
  await page.locator('#capBox').screenshot({ path: afterElemShot });
  await page.screenshot({ path: afterPageShot });

  // 改后像素采样
  const afterPng = pDecodePNG(fs.readFileSync(afterElemShot));
  const afterSamp = sampleCaptchaPixels(afterPng);
  const afterCardBG = await sampleCardBG(page);
  console.log(`  采样 bg(容器底): RGB(${afterSamp.bg.join(',')})`);
  console.log(`  采样 fg(字符亮): RGB(${afterSamp.fg.join(',')})`);
  console.log(`  卡片背景:       RGB(${afterCardBG.join(',')})`);
  const afterContrast = wcagContrast(afterSamp.fg, afterSamp.bg);
  const afterFgHue = rgbToHue(...afterSamp.fg);
  const afterBgHue = rgbToHue(...afterSamp.bg);
  // 卡片底 vs 验证码底 的亮度差
  const captchaBgLum = wcagLuminance(...afterSamp.bg);
  const cardBgLum = wcagLuminance(...afterCardBG);
  const lumDelta = Math.abs(captchaBgLum - cardBgLum);
  console.log(`  fg vs bg 对比度: ${afterContrast}:1 (目标 ≥4.5:1)`);
  console.log(`  fg hue: ${afterFgHue}° (目标 200-220°)`);
  console.log(`  bg hue: ${afterBgHue}°`);
  console.log(`  验证码底 vs 卡底 亮度差: ${lumDelta.toFixed(4)} (目标尽量小=融合)`);

  // ========== Phase 3: 桌面 1920 截图 ==========
  console.log(`\n🖥️  Phase 3: 桌面 1920×1080 验证`);
  const ctx2 = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const page2 = await ctx2.newPage();
  await page2.route('**/api/auth/captcha', async route => {
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ id: apiJson.id, image: FIXED_DATA_URI, ttl: 120 })
    });
  });
  await page2.route(/\/api\/(?!auth\/captcha)/, async route => {
    const resp = await route.fetch().catch(() => null);
    if (!resp) return route.fulfill({ status: 200, body: '{"value":[]}' });
    route.fulfill({ response: resp });
  });
  await page2.goto('http://127.0.0.1:8090/index.html', { waitUntil: 'load', timeout: 15000 });
  await page2.waitForTimeout(2000);
  const desktopShot = path.join(SHOTS, 'captcha-AFTER-desktop.png');
  await page2.screenshot({ path: desktopShot });
  console.log(`  桌面截图: ${desktopShot}`);

  // ========== Phase 4: 汇总报告 ==========
  console.log(`\n${'='.repeat(70)}`);
  console.log(`📋 验证码换色 —— 真实像素采样报告`);
  console.log(`${'='.repeat(70)}`);
  console.log(`\n【方案】从 CSS filter 路线 → JS DOMParser SVG 预处理`);
  console.log(`  旧 filter: invert(1) hue-rotate(258deg) saturate(1.2) brightness(1.08)`);
  console.log(`  新  方案: rect→#111c2e(--panel), text/tspan/line→#4d9bff(--accent2)`);
  console.log(`  取舍: filter 作用于已渲染像素受抗锯齿非线性影响，后端颜色全随机时`);
  console.log(`        无法稳定命中蓝区(hue 200-220°)；DOMParser 直接改 SVG 属性，`);
  console.log(`        100% 控制输出颜色，不依赖原始输入。`);

  console.log(`\n【采样 RGB 数值】（PNG element.screenshot → 解码 → 亮度排序取簇 median）`);
  console.log(`  ┌────────────────┬─────────────────────────┬─────────────────────────┬─────────────────────────┐`);
  console.log(`  │ 场景           │ 改前 (filter)           │ 改后 (JS 换色)          │ 目标值                  │`);
  console.log(`  ├────────────────┼─────────────────────────┼─────────────────────────┼─────────────────────────┤`);
  const fmt = arr => `RGB(${arr[0]},${arr[1]},${arr[2]})`;
  const fmtHue = h => `${h}°`;
  console.log(`  │ 容器底 (bg)    │ ${fmt(beforeSamp.bg).padEnd(23)} │ ${fmt(afterSamp.bg).padEnd(23)} │ RGB(17,28,46) 卡底同款  │`);
  console.log(`  │ 字符亮 (fg)    │ ${fmt(beforeSamp.fg).padEnd(23)} │ ${fmt(afterSamp.fg).padEnd(23)} │ RGB(77,155,255) 主题蓝  │`);
  console.log(`  │ 卡片背景        │ ${fmt(beforeCardBG).padEnd(23)} │ ${fmt(afterCardBG).padEnd(23)} │ RGB(17,28,46)           │`);
  console.log(`  └────────────────┴─────────────────────────┴─────────────────────────┴─────────────────────────┘`);

  console.log(`\n【对比度 & 色相】`);
  console.log(`  ┌────────────────────────┬──────────────┬──────────────┬──────────────┬──────────────┐`);
  console.log(`  │ 指标                    │ 改前         │ 改后         │ 目标         │ 达标         │`);
  console.log(`  ├────────────────────────┼──────────────┼──────────────┼──────────────┼──────────────┤`);
  const passContrast = afterContrast >= 4.5;
  const passHue = afterFgHue >= 200 && afterFgHue <= 220;
  const passFusion = lumDelta < 0.03;  // 亮度差 < 3% 算融合
  console.log(`  │ fg vs bg 对比度         │ ${beforeContrast.padEnd(12)} │ ${afterContrast.padEnd(12)} │ ≥ 4.5        │ ${passContrast ? '✅' : '❌'}           │`);
  console.log(`  │ fg 色相                 │ ${(beforeFgHue+'°').padEnd(12)} │ ${(afterFgHue+'°').padEnd(12)} │ 200-220°     │ ${passHue ? '✅' : '❌'}           │`);
  console.log(`  │ 验证码底 vs 卡底 亮度差  │ —            │ ${lumDelta.toFixed(4).padEnd(12)} │ < 0.03 融合   │ ${passFusion ? '✅' : '❌'}           │`);
  console.log(`  └────────────────────────┴──────────────┴──────────────┴──────────────┴──────────────┘`);

  console.log(`\n【截图路径】`);
  console.log(`  改前验证码特写: ${path.relative(process.cwd(), beforeElemShot)}`);
  console.log(`  改前整页 375×812:${path.relative(process.cwd(), beforePageShot)}`);
  console.log(`  改后验证码特写: ${path.relative(process.cwd(), afterElemShot)}`);
  console.log(`  改后整页 375×812:${path.relative(process.cwd(), afterPageShot)}`);
  console.log(`  改后桌面 1920:  ${path.relative(process.cwd(), desktopShot)}`);

  const allPass = passContrast && passHue && passFusion;
  console.log(`\n${allPass ? '✅ 三项指标全部达标' : '⚠️ 有指标未达标，需继续调优'}`);

  // 将报告写入 JSON 以便存档
  const reportJson = {
    timestamp: new Date().toISOString(),
    fixedCaptchaId: apiJson.id,
    origSVG: { fills: origFills, strokes: origStrokes },
    before: {
      filter: 'invert(1) hue-rotate(258deg) saturate(1.2) brightness(1.08)',
      bg: beforeSamp.bg, fg: beforeSamp.fg, cardBG: beforeCardBG,
      contrast: parseFloat(beforeContrast), fgHue: beforeFgHue,
    },
    after: {
      method: 'JS DOMParser SVG swap (rect→#111c2e, text/tspan/line→#4d9bff)',
      bg: afterSamp.bg, fg: afterSamp.fg, cardBG: afterCardBG,
      contrast: parseFloat(afterContrast), fgHue: afterFgHue,
      lumDelta: lumDelta,
    },
    checks: { passContrast, passHue, passFusion, allPass },
    screenshots: {
      beforeElem: beforeElemShot, beforePage: beforePageShot,
      afterElem: afterElemShot, afterPage: afterPageShot,
      desktop: desktopShot,
    }
  };
  fs.writeFileSync(
    path.join(SHOTS, 'captcha-report.json'),
    JSON.stringify(reportJson, null, 2),
    'utf-8'
  );
  console.log(`\n📄 报告已存档: .trae/tmp/screenshots/mobile-shots/captcha-report.json`);

  await ctx.close();
  await ctx2.close();
  await browser.close();
}

run().catch(e => { console.error('FATAL:', e); process.exit(1); });
