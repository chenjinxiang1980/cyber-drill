'use strict';
const { db } = require('./db');
const { now } = require('./util');

/* 写入一条站内消息（默认仅院内可见） */
function push(opts) {
  const o = opts || {};
  const title = String(o.title || '').trim().slice(0, 120);
  if (!title) return null;
  const seq = (db.prepare('SELECT COALESCE(MAX(seq),0)+1 AS n FROM notifs').get().n) || 1;
  const id = 'N' + Date.now() + '_' + Math.floor(Math.random() * 900 + 100);
  db.prepare('INSERT INTO notifs (id,time,level,source,title,descr,seq,target) VALUES (?,?,?,?,?,?,?,?)')
    .run(id, now(), ['P0', 'P1', 'P2'].indexOf(o.level) >= 0 ? o.level : 'P2',
      String(o.source || '演练调度').slice(0, 40), title, String(o.desc || '').slice(0, 500), seq,
      o.target === 'all' ? 'all' : 'internal');
  return id;
}

module.exports = { push };
