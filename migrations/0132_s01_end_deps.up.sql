-- =====================================================================
-- 0132 补齐 S01 结束节点的依赖（承接 0131 合并，纯 DML）
-- ---------------------------------------------------------------------
-- 0131 并入的 4 个节点（S01-22 溯源排查、S01-23 清除持久化、
-- S01-24 第三方通知、S01-25 患者告知）挂在 S01-3 / S01-7 之后，
-- 但未纳入结束判定，形成“可跳过”的末端分支。本迁移把依赖补上：
--   S01-17（应急结束） += S01-23（载荷清除）、S01-25（患者告知闭环）
--   S01-21（演练总结） += S01-22、S01-23、S01-24、S01-25
-- 不改节点分值、时限、角色、标题与正文；其余数据不动。
--
-- 前置：台账 0131 已应用；S01-17 deps 为 5 项、S01-21 deps 为 6 项；
--       S01 correct=25、distractor=48、合计 73、分值 100。
-- 后置：S01-17 deps=7、S01-21 deps=10，全部可解析；上述计数不变；
--       范围外节点/剧本指纹不变。
-- 回滚：把两条 deps 改回原值（见前置断言中的字符串）。
-- =====================================================================
SET SESSION group_concat_max_len = 10485760;

SET @p_ledger := (SELECT COUNT(*) FROM schema_migrations WHERE version='0131_merge_s20_into_s01');
SET @p_self := (SELECT COUNT(*) FROM schema_migrations WHERE version='0132_s01_end_deps');
SET @p_17 := (SELECT COUNT(*) FROM script_nodes WHERE id='S01-17' AND deleted_at IS NULL AND REPLACE(deps,' ','')='["S01-8","S01-19","S01-9","S01-11","S01-14"]');
SET @p_21 := (SELECT COUNT(*) FROM script_nodes WHERE id='S01-21' AND deleted_at IS NULL AND REPLACE(deps,' ','')='["S01-10","S01-12","S01-16","S01-18","S01-13","S01-20"]');
SET @p_new := (SELECT COUNT(*) FROM script_nodes WHERE id IN ('S01-22','S01-23','S01-24','S01-25') AND deleted_at IS NULL);
SET @p_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @p_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='distractor');
SET @p_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');

SET @fp_nodes_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(target,''),COALESCE(anchor_id,''),COALESCE(deps,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE (scenario_id IS NULL OR scenario_id NOT IN ('S01','S20')));
SET @fp_scen_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(type,''),COALESCE(status,''),COALESCE(nodes,-1),COALESCE(version,''),COALESCE(updated,''),COALESCE(category,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id NOT IN ('S01','S20'));
SET @fp_s01_other := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(deps,''),COALESCE(score,-1),COALESCE(timeout,-1),COALESCE(target,''),COALESCE(anchor_id,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND id NOT IN ('S01-17','S01-21'));

SET @p_ok := IF(@p_ledger=1 AND @p_self=0 AND @p_17=1 AND @p_21=1 AND @p_new=4
  AND @p_correct=25 AND @p_dist=48 AND @p_score=100, 1, NULL);
SELECT IF(@p_ok=1, 1, (SELECT table_name FROM information_schema.tables));

UPDATE script_nodes SET deps='["S01-8","S01-19","S01-9","S01-11","S01-14","S01-23","S01-25"]'
 WHERE id='S01-17' AND scenario_id='S01' AND deleted_at IS NULL;

UPDATE script_nodes SET deps='["S01-10","S01-12","S01-16","S01-18","S01-13","S01-20","S01-22","S01-23","S01-24","S01-25"]'
 WHERE id='S01-21' AND scenario_id='S01' AND deleted_at IS NULL;

SET @q_17 := (SELECT COUNT(*) FROM script_nodes WHERE id='S01-17' AND deleted_at IS NULL AND REPLACE(deps,' ','')='["S01-8","S01-19","S01-9","S01-11","S01-14","S01-23","S01-25"]');
SET @q_21 := (SELECT COUNT(*) FROM script_nodes WHERE id='S01-21' AND deleted_at IS NULL AND REPLACE(deps,' ','')='["S01-10","S01-12","S01-16","S01-18","S01-13","S01-20","S01-22","S01-23","S01-24","S01-25"]');
SET @q_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @q_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='distractor');
SET @q_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @q_bad_dep := (SELECT COUNT(*) FROM script_nodes n
  JOIN (SELECT 0 i UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3
        UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7
        UNION ALL SELECT 8 UNION ALL SELECT 9) idx
    ON idx.i < JSON_LENGTH(n.deps)
  WHERE n.scenario_id='S01' AND n.deleted_at IS NULL
    AND n.deps IS NOT NULL AND n.deps<>'' AND n.deps<>'[]'
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S01' AND c.deleted_at IS NULL
                      AND c.id = JSON_UNQUOTE(JSON_EXTRACT(n.deps, CONCAT('$[',idx.i,']')))));

SET @fp_nodes_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(target,''),COALESCE(anchor_id,''),COALESCE(deps,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE (scenario_id IS NULL OR scenario_id NOT IN ('S01','S20')));
SET @fp_scen_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(type,''),COALESCE(status,''),COALESCE(nodes,-1),COALESCE(version,''),COALESCE(updated,''),COALESCE(category,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id NOT IN ('S01','S20'));
SET @fp_s01_other2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(deps,''),COALESCE(score,-1),COALESCE(timeout,-1),COALESCE(target,''),COALESCE(anchor_id,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND id NOT IN ('S01-17','S01-21'));

SET @q_ok := IF(@q_17=1 AND @q_21=1 AND @q_correct=25 AND @q_dist=48 AND @q_score=100
  AND @q_bad_dep=0
  AND @fp_nodes_out2=@fp_nodes_out AND @fp_scen_out2=@fp_scen_out AND @fp_s01_other2=@fp_s01_other, 1, NULL);
SELECT IF(@q_ok=1, 1, (SELECT table_name FROM information_schema.tables));
