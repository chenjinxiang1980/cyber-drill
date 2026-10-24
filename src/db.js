/* 数据库连接与初始化（使用 Node 内置 node:sqlite，无需编译原生模块） */
'use strict';
const path = require('node:path');
const fs = require('node:fs');
const { DatabaseSync } = require('node:sqlite');

const DATA_DIR = path.join(__dirname, '..', 'data');
const DB_FILE = process.env.DRILL_DB || path.join(DATA_DIR, 'drill.db');

if (!fs.existsSync(DATA_DIR)) fs.mkdirSync(DATA_DIR, { recursive: true });

const db = new DatabaseSync(DB_FILE);

/* 建表（幂等） */
function migrate() {
  const sql = fs.readFileSync(path.join(__dirname, 'schema.sql'), 'utf8');
  db.exec(sql);
  addColumn('projects', 'mode', "TEXT DEFAULT '基础版'");
  addColumn('script_nodes', 'guide', 'TEXT');
  addColumn('script_nodes', 'keywords', 'TEXT');
  addColumn('script_nodes', 'action', 'TEXT');
  addColumn('script_nodes', 'required', 'INTEGER DEFAULT 1');
  addColumn('tasks', 'completed_at', 'TEXT');
  addColumn('tasks', 'accepted_at', 'TEXT');
  addColumn('knowledge', 'file_path', 'TEXT');
  /* 旧库升级：平台管理员需具备「演练报告导出」与「时序回放（整体复盘分析）」权限 */
  grantNav('platformadmin', 'report');
  grantNav('platformadmin', 'replay');
  /* 「演练监视大屏」：指挥部、平台管理员、信息科负责人 */
  grantNav('commander', 'screen');
  grantNav('platformadmin', 'screen');
  grantNav('itlead', 'screen');
}

/* 幂等为指定角色补充菜单权限（角色尚未导入时跳过） */
function grantNav(roleId, key) {
  const row = db.prepare('SELECT nav FROM roles WHERE id=?').get(roleId);
  if (!row) return;
  let nav = parseJson(row.nav, []);
  if (!Array.isArray(nav)) nav = [];
  if (nav.indexOf(key) >= 0) return;
  nav.push(key);
  db.prepare('UPDATE roles SET nav=? WHERE id=?').run(JSON.stringify(nav), roleId);
}

/* 兼容旧库：补齐新增列 */
function addColumn(table, col, def) {
  const cols = db.prepare('PRAGMA table_info(' + table + ')').all();
  if (!cols.some(function (c) { return c.name === col; })) {
    db.exec('ALTER TABLE ' + table + ' ADD COLUMN ' + col + ' ' + def);
  }
}

/* 是否已初始化（有角色数据即视为已初始化） */
function isSeeded() {
  try {
    const row = db.prepare('SELECT COUNT(*) AS c FROM roles').get();
    return row.c > 0;
  } catch (e) {
    return false;
  }
}

/* 事务包装 */
function tx(fn) {
  db.exec('BEGIN');
  try {
    const r = fn();
    db.exec('COMMIT');
    return r;
  } catch (e) {
    try { db.exec('ROLLBACK'); } catch (_) {}
    throw e;
  }
}

/* JSON 字段安全解析 */
function parseJson(s, fallback) {
  if (s === null || s === undefined || s === '') return fallback;
  try { return JSON.parse(s); } catch (e) { return fallback; }
}

module.exports = { db, migrate, isSeeded, tx, parseJson, DB_FILE, DATA_DIR };
