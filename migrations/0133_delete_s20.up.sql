-- =====================================================================
-- 0133 物理删除已退役剧本 S20「数据加密勒索」（纯 DML）
-- ---------------------------------------------------------------------
-- 【背景】
--   0131 已把 S20 的内容并入 S01 并置 status='停用'，保留 64 个节点行
--   供追溯。按内容侧要求，本次把这条已退役剧本连同其全部节点行
--       （22 个正确节点 + 42 个混淆节点，含软删行）物理删除。
--   S20 停用后从未被使用，projects / script_revisions / drill_steps /
--   project_core_evidence 对 scenario_id='S20' 的引用均为 0；
--   tasks / drill_node_state / evidence / impact_events /
--   project_core_evidence 对 S20-* / DSS20-* 节点 id 的引用也均为 0。
--
-- 【执行顺序】
--   本迁移必须先于 0134 执行：0134 会把 S21 的 id 改成 S20，
--   若旧 S20 行还在会造成 scenarios.id 主键与 S20-* / DSS20-* 节点 id 冲突。
--
-- 【前置】
--   台账 0132 已应用、本迁移未应用；
--   剧本 21 行 / 在用 20 个；S20 存在且 status='停用'、nodes=64；
--   S20 节点 64 行（0 软删、22 正确合计 100 分、42 混淆、1 个 end）；
--   S20 场景引用 0、节点 id 引用 0；S21 在用且 61 个节点。
-- 【后置】
--   剧本 20 行 / 在用 20 个；S20 场景行 0、S20 节点行 0；
--   六大必需角色覆盖 20、各剧本正确节点分值 100 共 20；
--   S21 仍为 61 节点 / 100 分（供 0134 改名）。
-- 【指纹】S20 以外的剧本行与节点行前后一致。
-- 【回滚】按 pre-0133 备份整库恢复（删除不可逆，不能用 DML 回滚）。
-- =====================================================================
SET SESSION group_concat_max_len = 10485760;

-- ===== 0. 前置断言 =====
SET @p_ledger := (SELECT COUNT(*) FROM schema_migrations WHERE version='0132_s01_end_deps');
SET @p_self := (SELECT COUNT(*) FROM schema_migrations WHERE version='0133_delete_s20');
SET @p_scen_all := (SELECT COUNT(*) FROM scenarios);
SET @p_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');
SET @p_s20 := (SELECT COUNT(*) FROM scenarios WHERE id='S20' AND COALESCE(status,'启用')='停用' AND nodes=64);
SET @p_s20_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20');
SET @p_s20_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND flow_type='correct');
SET @p_s20_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S20' AND flow_type='correct');
SET @p_s20_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND flow_type='distractor');
SET @p_s20_end := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND type='end');
SET @p_s20_ref := (SELECT (SELECT COUNT(*) FROM projects WHERE scenario_id='S20')
                       + (SELECT COUNT(*) FROM script_revisions WHERE scenario_id='S20')
                       + (SELECT COUNT(*) FROM drill_steps WHERE scenario_id='S20')
                       + (SELECT COUNT(*) FROM project_core_evidence WHERE scenario_id='S20'));
SET @p_s20_idref := (SELECT (SELECT COUNT(*) FROM tasks WHERE node_id LIKE 'S20-%' OR node_id LIKE 'DSS20-%')
                         + (SELECT COUNT(*) FROM drill_node_state WHERE node_id LIKE 'S20-%' OR node_id LIKE 'DSS20-%')
                         + (SELECT COUNT(*) FROM drill_steps WHERE matched_node LIKE 'S20-%' OR matched_node LIKE 'DSS20-%')
                         + (SELECT COUNT(*) FROM evidence WHERE node_id LIKE 'S20-%' OR node_id LIKE 'DSS20-%')
                         + (SELECT COUNT(*) FROM impact_events WHERE source_node_id LIKE 'S20-%' OR source_node_id LIKE 'DSS20-%')
                         + (SELECT COUNT(*) FROM project_core_evidence WHERE node_id LIKE 'S20-%' OR node_id LIKE 'DSS20-%'));
SET @p_s21 := (SELECT COUNT(*) FROM scenarios WHERE id='S21' AND COALESCE(status,'启用')='启用' AND nodes=61);
SET @p_s21_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND deleted_at IS NULL);

SET @fp_scen_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(category,''),COALESCE(name,''),COALESCE(type,''),COALESCE(level,''),COALESCE(status,''),COALESCE(version,''),COALESCE(updated,''),COALESCE(nodes,-1),COALESCE(descr,''),COALESCE(alert,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id<>'S20');
SET @fp_nodes_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(type,''),COALESCE(title,''),COALESCE(target,''),COALESCE(timeout,-1),COALESCE(form,''),COALESCE(descr,''),COALESCE(seq,-1),COALESCE(required,-1),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id<>'S20');

SET @p_ok := IF(@p_ledger=1 AND @p_self=0
  AND @p_scen_all=21 AND @p_active=20
  AND @p_s20=1 AND @p_s20_nodes=64 AND @p_s20_correct=22 AND @p_s20_score=100
  AND @p_s20_dist=42 AND @p_s20_end=1
  AND @p_s20_ref=0 AND @p_s20_idref=0
  AND @p_s21=1 AND @p_s21_nodes=61, 1, NULL);
SELECT IF(@p_ok=1, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 1. 删除 S20 节点与场景行 =====
DELETE FROM script_nodes WHERE scenario_id='S20';
DELETE FROM scenarios   WHERE id='S20';

-- ===== 2. 后置断言 =====
SET @q_scen_all := (SELECT COUNT(*) FROM scenarios);
SET @q_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');
SET @q_s20_scen := (SELECT COUNT(*) FROM scenarios WHERE id='S20');
SET @q_s20_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' OR id LIKE 'S20-%' OR id LIKE 'DSS20-%');
SET @q_s21_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND deleted_at IS NULL);
SET @q_s21_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S21' AND deleted_at IS NULL AND flow_type='correct');
SET @q_cov := (SELECT COUNT(*) FROM (
    SELECT sc.id,
      MAX(n.target=(SELECT name FROM roles WHERE id='duty')) AS duty,
      MAX(n.target=(SELECT name FROM roles WHERE id='netsec')) AS netsec,
      MAX(n.target=(SELECT name FROM roles WHERE id='role_mvnaewi63am')) AS datasec,
      MAX(n.target=(SELECT name FROM roles WHERE id='sysadmin')) AS sysadmin,
      MAX(n.target=(SELECT name FROM roles WHERE id='itlead')) AS itlead,
      MAX(n.target=(SELECT name FROM roles WHERE id='commander')) AS commander
    FROM scenarios sc LEFT JOIN script_nodes n
      ON n.scenario_id=sc.id AND n.deleted_at IS NULL
    GROUP BY sc.id) c
  WHERE c.duty=1 AND c.netsec=1 AND c.datasec=1 AND c.sysadmin=1 AND c.itlead=1 AND c.commander=1);
SET @q_score100 := (SELECT COUNT(*) FROM (
    SELECT scenario_id, SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS s
    FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t WHERE t.s=100);

SET @fp_scen_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(category,''),COALESCE(name,''),COALESCE(type,''),COALESCE(level,''),COALESCE(status,''),COALESCE(version,''),COALESCE(updated,''),COALESCE(nodes,-1),COALESCE(descr,''),COALESCE(alert,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id<>'S20');
SET @fp_nodes_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(type,''),COALESCE(title,''),COALESCE(target,''),COALESCE(timeout,-1),COALESCE(form,''),COALESCE(descr,''),COALESCE(seq,-1),COALESCE(required,-1),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id<>'S20');

SET @q_ok := IF(@q_scen_all=20 AND @q_active=20
  AND @q_s20_scen=0 AND @q_s20_nodes=0
  AND @q_s21_nodes=61 AND @q_s21_score=100
  AND @q_cov=20 AND @q_score100=20
  AND @fp_scen_out2=@fp_scen_out AND @fp_nodes_out2=@fp_nodes_out, 1, NULL);
SELECT IF(@q_ok=1, 1, (SELECT table_name FROM information_schema.tables));
