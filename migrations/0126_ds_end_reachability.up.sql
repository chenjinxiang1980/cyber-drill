-- =====================================================================
-- 0126 数据安全剧本（S09–S21）必做节点依赖补全（纯 DML）
-- ---------------------------------------------------------------------
-- 背景：S09–S21 的必做节点里存在「断头分支」——建档签收、监管续报、
--       刑事报案/受案、第三方通知、部分止损子步骤等节点没有任何后继依赖，
--       既不进入结束节点，也不进入恢复/复盘节点。表现为：
--         1) 结束节点（type=end）在仍有必做分支未完成时就已解锁；
--         2) 恢复类节点可在止损未完成时提前开始（例：S16-16 未依赖 S16-14/15）；
--         3) 流程图出现悬空必做节点，与实际处置顺序不符。
-- 处理：按剧本语义为断头终节点补一条后继依赖（见 tmp_edge），
--       使结束节点成为全部必做节点的汇聚点，恢复/复盘节点等待其前置分支完成。
--       S01–S08 已在 0124 中处理，本迁移不触碰。
-- 写法：前置断言 + 单事务 + 范围外行指纹 + 后置断言；守卫失败触发 1242 回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- ===== 1. 待补依赖边集合（target 依赖 dep） =====
CREATE TEMPORARY TABLE tmp_edge (
  target_id VARCHAR(32) NOT NULL,
  dep_id    VARCHAR(32) NOT NULL,
  PRIMARY KEY (target_id, dep_id)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO tmp_edge (target_id, dep_id) VALUES
  -- S09 数据篡改
  ('S09-7','S09-4'), ('S09-21','S09-13'), ('S09-21','S09-14'), ('S09-21','S09-16'),
  -- S10 数据假冒
  ('S10-7','S10-4'), ('S10-20','S10-12'), ('S10-20','S10-14'), ('S10-20','S10-19'),
  -- S11 数据泄露（代码仓库）
  ('S11-7','S11-4'), ('S11-21','S11-13'), ('S11-21','S11-15'), ('S11-21','S11-20'),
  -- S12 数据窃取（接口）
  ('S12-7','S12-4'), ('S12-21','S12-13'), ('S12-21','S12-15'), ('S12-21','S12-20'),
  -- S13 数据拦截（镜像流量）
  ('S13-7','S13-4'), ('S13-21','S13-13'), ('S13-21','S13-15'), ('S13-21','S13-20'),
  -- S14 数据投毒（模型）
  ('S14-7','S14-4'), ('S14-20','S14-13'),
  -- S15 数据滥用与出境
  ('S15-7','S15-4'), ('S15-21','S15-15'),
  -- S16 数据误删
  ('S16-7','S16-4'), ('S16-16','S16-14'), ('S16-16','S16-15'), ('S16-19','S16-12'),
  -- S17 社工攻击
  ('S17-6','S17-3'), ('S17-20','S17-14'),
  -- S18 违规收集（小程序）
  ('S18-6','S18-5'), ('S18-17','S18-14'), ('S18-18','S18-11'),
  -- S19 行踪轨迹
  ('S19-6','S19-5'), ('S19-19','S19-14'), ('S19-19','S19-15'), ('S19-20','S19-11'),
  -- S20 勒索加密
  ('S20-17','S20-6'), ('S20-17','S20-14'), ('S20-20','S20-12'), ('S20-20','S20-15'),
  -- S21 越权访问
  ('S21-6','S21-4'), ('S21-18','S21-15'), ('S21-19','S21-11'), ('S21-19','S21-16');

-- ===== 前置断言 =====
SET @p_edges := (SELECT COUNT(*) FROM tmp_edge);
SET @p_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @p_end   := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND type='end');
SET @p_req   := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND required=1 AND type<>'end' AND flow_type='correct');
-- 每条边的两端节点都必须存在
SET @p_missing := (SELECT COUNT(*) FROM tmp_edge t
  WHERE NOT EXISTS (SELECT 1 FROM script_nodes n WHERE n.deleted_at IS NULL AND n.id=t.target_id)
     OR NOT EXISTS (SELECT 1 FROM script_nodes n WHERE n.deleted_at IS NULL AND n.id=t.dep_id));
-- 待补边不得已存在于 deps 中
SET @p_dup := (SELECT COUNT(*) FROM tmp_edge t
  JOIN script_nodes n ON n.id=t.target_id AND n.deleted_at IS NULL
  WHERE JSON_CONTAINS(n.deps, JSON_QUOTE(t.dep_id)));
SET @guard_pre := IF(@p_edges=45 AND @p_nodes=1173 AND @p_end=21 AND @p_req=402
  AND @p_missing=0 AND @p_dup=0, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写前） =====
SET @target_ids := (SELECT GROUP_CONCAT(target_id) FROM tmp_edge);
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,seq,type,flow_type,required,score,timeout)
  ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND NOT FIND_IN_SET(id, @target_ids));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_forms_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,fields) ORDER BY id SEPARATOR '\n')) FROM form_templates);
SET @dep_len_before := (SELECT SUM(JSON_LENGTH(deps)) FROM script_nodes WHERE deleted_at IS NULL);

-- ===== 2. 合并依赖（每个 target 一次写入，避免多次读改写丢失） =====
UPDATE script_nodes n
JOIN (
  SELECT target_id, JSON_ARRAYAGG(dep_id) AS addarr FROM tmp_edge GROUP BY target_id
) x ON x.target_id = n.id
SET n.deps = JSON_MERGE_PRESERVE(n.deps, x.addarr);

DROP TEMPORARY TABLE tmp_edge;

-- ===== 后置断言 =====
SET @q_edges := (SELECT SUM(JSON_LENGTH(deps)) - @dep_len_before FROM script_nodes WHERE deleted_at IS NULL LIMIT 1);
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @q_end   := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND type='end');
-- 结束节点可达的必做节点数必须等于全部必做节点数（不再有断头）
SET @q_reach := (
  WITH RECURSIVE edges AS (
    SELECT n.id AS child, jt.dep COLLATE utf8mb4_unicode_ci AS parent
    FROM script_nodes n
    JOIN JSON_TABLE(n.deps,'$[*]' COLUMNS(dep VARCHAR(32) PATH '$')) jt ON 1=1
    WHERE n.deleted_at IS NULL
  ), reach AS (
    SELECT id AS node FROM script_nodes WHERE type='end' AND deleted_at IS NULL
    UNION
    SELECT e.parent FROM reach r JOIN edges e ON e.child=r.node
  )
  SELECT COUNT(*) FROM script_nodes s
  WHERE s.deleted_at IS NULL AND s.required=1 AND s.type<>'end' AND s.flow_type='correct'
    AND EXISTS (SELECT 1 FROM reach r WHERE r.node=s.id)
);
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,seq,type,flow_type,required,score,timeout)
  ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND NOT FIND_IN_SET(id, @target_ids));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_forms_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,fields) ORDER BY id SEPARATOR '\n')) FROM form_templates);

SET @guard_post := IF(@q_edges=45 AND @q_nodes=1173 AND @q_end=21 AND @q_reach=402
  AND @fp_nodes_before=@fp_nodes_after
  AND @fp_scen_before=@fp_scen_after
  AND @fp_forms_before=@fp_forms_after, 1,
  (SELECT table_name FROM information_schema.tables));
