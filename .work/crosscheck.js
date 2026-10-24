// 全局静态自查：路由/图标/API/App/MOCK 引用一致性
const fs = require('fs');
const path = require('path');
const dir = 'e:/Trae/Emergency/cyber-drill-prototype';
const files = fs.readdirSync(dir).filter(f => f.endsWith('.html'));

const appJs = fs.readFileSync(path.join(dir, 'app.js'), 'utf8');
const apiJs = fs.readFileSync(path.join(dir, 'api.js'), 'utf8');
const mockJs = fs.readFileSync(path.join(dir, 'mock.js'), 'utf8');

// ICONS 键
const iconBlock = appJs.match(/var ICONS = \{([\s\S]*?)\n\};/)[1];
const icons = new Set([...iconBlock.matchAll(/^\s{2}([a-zA-Z0-9_-]+):/gm)].map(m => m[1]));

// App 导出
const appExpBlock = appJs.match(/window\.App = \{([\s\S]*?)\};/);
const appExports = new Set(appExpBlock ? [...appExpBlock[1].matchAll(/([a-zA-Z0-9_]+)\s*:/g)].map(m => m[1]) : []);

// API 导出（api.js 用 return { name:fn, ... } 形式）
const apiExpBlock = apiJs.match(/return \{([\s\S]*?)\};\s*\}\)\(\);?\s*$/m) || apiJs.match(/return \{ login:login[\s\S]*?\};/);
const apiExports = new Set(apiExpBlock ? [...apiExpBlock[1].matchAll(/([a-zA-Z0-9_]+)\s*:/g)].map(m => m[1]) : []);

// MOCK 顶层键（mock.js 用 window.MOCK = (function(){ ... return {KEY:...,} })()）
const mockReturn = mockJs.match(/return \{([\s\S]*?)\};?\s*\}\)\(\);?/) || mockJs.match(/return \{([\s\S]*?)\};/);
const mockKeys = new Set(mockReturn ? [...mockReturn[1].matchAll(/([A-Z_0-9]+)\s*:/g)].map(m => m[1]) : []);

// NAV_GROUPS keys
const navKeys = new Set([...appJs.matchAll(/\{key:'([a-zA-Z0-9_-]+)'/g)].map(m => m[1]));

let problems = [];
for (const f of files) {
  const src = fs.readFileSync(path.join(dir, f), 'utf8');
  const dp = (src.match(/data-page="([^"]+)"/) || [])[1];
  const boot = (src.match(/App\.boot\('([^']+)'\)/) || [])[1];
  if (f !== 'index.html') {
    if (!dp) problems.push(`${f}: 缺 data-page`);
    if (!boot) problems.push(`${f}: 缺 App.boot`);
    if (dp && boot && dp !== boot) problems.push(`${f}: data-page(${dp}) != boot(${boot})`);
    if (boot && !navKeys.has(boot)) problems.push(`${f}: boot key "${boot}" 不在 NAV_GROUPS`);
  }
  // href 引用
  for (const m of src.matchAll(/href='([a-zA-Z0-9_.-]+\.html)/g)) if (!fs.existsSync(path.join(dir, m[1]))) problems.push(`${f}: 断链 ${m[1]}`);
  for (const m of src.matchAll(/href="([a-zA-Z0-9_.-]+\.html)/g)) if (!fs.existsSync(path.join(dir, m[1]))) problems.push(`${f}: 断链 ${m[1]}`);
  for (const m of src.matchAll(/location\.href\s*=\s*'([a-zA-Z0-9_.-]+\.html)/g)) if (!fs.existsSync(path.join(dir, m[1]))) problems.push(`${f}: 断链 ${m[1]}`);
  // 图标
  for (const m of src.matchAll(/App\.icon\('([a-zA-Z0-9_-]+)'/g)) if (!icons.has(m[1])) problems.push(`${f}: 未知图标 ${m[1]}`);
  for (const m of src.matchAll(/icon\('([a-zA-Z0-9_-]+)'/g)) if (!icons.has(m[1])) problems.push(`${f}: 未知图标 ${m[1]}`);
  // API 方法
  for (const m of src.matchAll(/API\.([a-zA-Z0-9_]+)\s*\(/g)) if (!apiExports.has(m[1])) problems.push(`${f}: API.${m[1]} 未导出`);
  // App 方法
  for (const m of src.matchAll(/App\.([a-zA-Z0-9_]+)\s*\(/g)) if (!appExports.has(m[1])) problems.push(`${f}: App.${m[1]} 未导出`);
  // MOCK 顶层
  for (const m of src.matchAll(/MOCK\.([A-Z_0-9]+)/g)) if (!mockKeys.has(m[1])) problems.push(`${f}: MOCK.${m[1]} 不存在`);
  // 重复 id
  const ids = [...src.matchAll(/\bid="([a-zA-Z0-9_-]+)"/g)].map(m => m[1]);
  const seen = {}; ids.forEach(i => seen[i] = (seen[i] || 0) + 1);
  for (const i in seen) if (seen[i] > 1) problems.push(`${f}: 重复静态 id "${i}" x${seen[i]}`);
}

// 角色 nav 覆盖
const rolesBlock = mockJs.match(/ROLES:\s*\[[\s\S]*?\n\s*\]/) || mockJs.match(/var ROLES[\s\S]*?\];/);
if (rolesBlock) {
  const navs = new Set();
  for (const m of rolesBlock[0].matchAll(/nav:\s*\[([^\]]*)\]/g)) {
    [...m[1].matchAll(/'([^']+)'/g)].forEach(x => navs.add(x[1]));
  }
  for (const n of navs) {
    if (n === 'index') continue;
    if (!fs.existsSync(path.join(dir, n + '.html'))) problems.push(`ROLES.nav 引用无页面: ${n}.html`);
  }
  console.log('角色nav覆盖键:', [...navs].join(', '));
}

console.log('页面文件:', files.join(', '));
console.log('ICONS数:', icons.size, '| App导出:', appExports.size, '| API导出:', apiExports.size, '| MOCK键:', mockKeys.size);
if (problems.length) { console.log('\n=== 发现问题 ==='); problems.forEach(p => console.log('-', p)); }
else console.log('\n=== 全部通过，无问题 ===');
