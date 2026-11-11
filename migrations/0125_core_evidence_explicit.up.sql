-- =====================================================================
-- 0125 核心取证项显式标记（纯 DML）
-- ---------------------------------------------------------------------
-- 背景：结束核对默认按「证据名称关键词取前 12 条」推导核心取证清单，
--       会将「应急响应结束记录单」「网络安全事件上报表 / 上报回执」等
--       法定必备凭证挤出 core 范围，导致核对口径与法规不符。
-- 处理：在 script_nodes.evidence_req 的对应取证项上显式写 "core": true；
--       Go 侧 defaultCoreEvidenceFrom 优先纳入显式 core 项（不受 12 项上限挤压），
--       其余名额再按名称优先级补足，不再靠名称猜测必备项。
-- 写法：前置断言 + 单事务 + 范围外行指纹 + 后置断言；守卫失败触发 1242 回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- ===== 前置断言：目标证据项数量必须是预检时的版本 =====
SET @p1 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%结束记录%');
SET @p2 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%网络安全事件上报表%');
SET @p3 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%回执%');
SET @p4 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(c VARCHAR(8) PATH '$.core')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.c IS NOT NULL);
SET @p5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @guard_pre := IF(@p1=8 AND @p2=8 AND @p3=34 AND @p4=0 AND @p5=1173, 1,
  (SELECT table_name FROM information_schema.tables));

-- ===== 待修改节点集合 + 范围外行指纹（写前） =====
SET @target_ids := (SELECT GROUP_CONCAT(DISTINCT n.id) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name')) e ON 1=1
  WHERE n.deleted_at IS NULL AND (e.nm LIKE '%结束记录%' OR e.nm LIKE '%网络安全事件上报表%' OR e.nm LIKE '%回执%'));
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,evidence_req) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND NOT FIND_IN_SET(id, @target_ids));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_forms_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,fields) ORDER BY id SEPARATOR '\n')) FROM form_templates);

-- ===== 1. 收集需要标记 core 的取证项（node_id + 数组下标） =====
CREATE TEMPORARY TABLE tmp_core_ev (
  nid VARCHAR(64) NOT NULL,
  idx INT NOT NULL,
  PRIMARY KEY (nid, idx)
);
INSERT INTO tmp_core_ev (nid, idx)
SELECT n.id, e.ord
FROM script_nodes n
JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(ord FOR ORDINALITY, nm VARCHAR(255) PATH '$.name')) e ON 1=1
WHERE n.deleted_at IS NULL
  AND (e.nm LIKE '%结束记录%' OR e.nm LIKE '%网络安全事件上报表%' OR e.nm LIKE '%回执%');

-- ===== 2. 写入显式 core 标记 =====
UPDATE script_nodes n
JOIN tmp_core_ev t ON t.nid = n.id
SET n.evidence_req = JSON_SET(n.evidence_req, CONCAT('$[', t.idx - 1, '].core'), TRUE);

SET @flagged := (SELECT COUNT(*) FROM tmp_core_ev);
DROP TEMPORARY TABLE tmp_core_ev;

-- ===== 后置断言 =====
SET @q1 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(c VARCHAR(8) PATH '$.core')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.c = 'true');
SET @q2 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name', c VARCHAR(8) PATH '$.core')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%结束记录%' AND e.c = 'true');
SET @q3 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name', c VARCHAR(8) PATH '$.core')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%网络安全事件上报表%' AND e.c = 'true');
SET @q4 := (SELECT COUNT(*) FROM script_nodes n
  JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(nm VARCHAR(255) PATH '$.name', c VARCHAR(8) PATH '$.core')) e ON 1=1
  WHERE n.deleted_at IS NULL AND e.nm LIKE '%回执%' AND e.c = 'true');
SET @q5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,evidence_req) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND NOT FIND_IN_SET(id, @target_ids));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_forms_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,fields) ORDER BY id SEPARATOR '\n')) FROM form_templates);

SET @guard_post := IF(@q1 = @flagged AND @q1 = 50 AND @q2 = 8 AND @q3 = 8 AND @q4 = 34
  AND @q5 = 1173
  AND @fp_nodes_before = @fp_nodes_after
  AND @fp_scen_before = @fp_scen_after
  AND @fp_forms_before = @fp_forms_after, 1,
  (SELECT table_name FROM information_schema.tables));
