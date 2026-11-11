-- =====================================================================
-- 0134 S21 改编号为 S20（数据权限剧本，纯 DML）
-- ---------------------------------------------------------------------
-- 【背景】
--   0133 已物理删除退役的旧 S20「数据加密勒索」，编号 S20 空出。
--   S21「数据权限（权限配置错误致越权访问）」是最后一个在用剧本，
--   为使编号连续（S01–S20、无空洞），把 S21 整体改为 S20。
--
-- 【改名范围】
--   scenarios.id        S21 → S20（name/descr/alert 内无“S21”字样，不改正文）
--   script_nodes.scenario_id  S21 → S20
--   script_nodes.id     21 个 S21-n → S20-n；40 个 DSS21-* → DSS20-*
--   script_nodes.deps   20 行内互引 S21-n → S20-n
--   script_nodes.anchor_id 40 行 S21-n → S20-n
--
-- 【前置】
--   台账 0133 已应用、本迁移未应用；剧本 20 行 / 在用 20 个；
--   S20 不存在、S21 在用且 nodes=61；S21 节点 61（21 正确合计 100 分、
--   40 混淆、1 个 end、0 软删）；deps 含 S21 共 20 行、anchor_id 含 S21 共 40 行；
--   S20-/DSS20- 节点 id 无占用；S21 场景与节点 id 在外围表引用为 0；
--   六大必需角色覆盖 20、各剧本 100 分共 20。
-- 【后置】
--   剧本 20 行 / 在用 20 个，编号 S01–S20 连续无空洞，S21 零残留；
--   S20 为原 S21（61 节点、21 正确合计 100 分、40 混淆、1 个 end）；
--   依赖与锚点全部可解析、每个非 end 正确节点 ≥2 条混淆；
--   六大必需角色覆盖 20、各剧本 100 分共 20；范围外指纹不变。
-- 【回滚】把 S20 改回 S21、id/deps/anchor_id 反向替换（需先确认无新引用）。
-- =====================================================================
SET SESSION group_concat_max_len = 10485760;

-- ===== 0. 前置断言 =====
SET @p_ledger := (SELECT COUNT(*) FROM schema_migrations WHERE version='0133_delete_s20');
SET @p_self := (SELECT COUNT(*) FROM schema_migrations WHERE version='0134_rename_s21_to_s20');
SET @p_scen_all := (SELECT COUNT(*) FROM scenarios);
SET @p_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');
SET @p_s20_scen := (SELECT COUNT(*) FROM scenarios WHERE id='S20');
SET @p_s21_scen := (SELECT COUNT(*) FROM scenarios WHERE id='S21' AND COALESCE(status,'启用')='启用' AND nodes=61);
SET @p_s21_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21');
SET @p_s21_del := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND deleted_at IS NOT NULL);
SET @p_s21_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND flow_type='correct');
SET @p_s21_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S21' AND flow_type='correct');
SET @p_s21_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND flow_type='distractor');
SET @p_s21_end := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND type='end');
SET @p_s21_deps := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND deps LIKE '%S21-%');
SET @p_s21_anchor := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S21' AND anchor_id LIKE '%S21-%');
SET @p_s21_baddep := (SELECT COUNT(*) FROM script_nodes n
  JOIN (SELECT 0 i UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3
        UNION ALL SELECT 4 UNION ALL SELECT 5) idx ON idx.i < JSON_LENGTH(n.deps)
  WHERE n.scenario_id='S21' AND n.deleted_at IS NULL
    AND n.deps IS NOT NULL AND n.deps<>'' AND n.deps<>'[]'
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S21' AND c.deleted_at IS NULL
                      AND c.id = JSON_UNQUOTE(JSON_EXTRACT(n.deps, CONCAT('$[',idx.i,']')))));
SET @p_s21_badanchor := (SELECT COUNT(*) FROM script_nodes n
  WHERE n.scenario_id='S21' AND n.deleted_at IS NULL AND COALESCE(n.anchor_id,'')<>''
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S21' AND c.deleted_at IS NULL AND c.id=n.anchor_id));
SET @p_s21_baddist := (SELECT COUNT(*) FROM script_nodes n
  WHERE n.scenario_id='S21' AND n.deleted_at IS NULL AND n.flow_type='correct' AND n.type<>'end'
    AND (SELECT COUNT(*) FROM script_nodes d WHERE d.scenario_id='S21' AND d.deleted_at IS NULL
           AND d.flow_type='distractor' AND d.anchor_id=n.id) < 2);
SET @p_id_free := (SELECT COUNT(*) FROM script_nodes WHERE id LIKE 'S20-%' OR id LIKE 'DSS20-%');
SET @p_ref := (SELECT (SELECT COUNT(*) FROM projects WHERE scenario_id='S21')
                    + (SELECT COUNT(*) FROM script_revisions WHERE scenario_id='S21')
                    + (SELECT COUNT(*) FROM drill_steps WHERE scenario_id='S21')
                    + (SELECT COUNT(*) FROM project_core_evidence WHERE scenario_id='S21'));
SET @p_idref := (SELECT (SELECT COUNT(*) FROM tasks WHERE node_id LIKE 'S21-%' OR node_id LIKE 'DSS21-%')
                      + (SELECT COUNT(*) FROM drill_node_state WHERE node_id LIKE 'S21-%' OR node_id LIKE 'DSS21-%')
                      + (SELECT COUNT(*) FROM drill_steps WHERE matched_node LIKE 'S21-%' OR matched_node LIKE 'DSS21-%')
                      + (SELECT COUNT(*) FROM evidence WHERE node_id LIKE 'S21-%' OR node_id LIKE 'DSS21-%')
                      + (SELECT COUNT(*) FROM impact_events WHERE source_node_id LIKE 'S21-%' OR source_node_id LIKE 'DSS21-%')
                      + (SELECT COUNT(*) FROM project_core_evidence WHERE node_id LIKE 'S21-%' OR node_id LIKE 'DSS21-%'));
SET @p_payload := (SELECT COUNT(*) FROM script_revisions WHERE nodes_payload LIKE '%S21%');
SET @p_text := (SELECT (SELECT COUNT(*) FROM script_nodes
                          WHERE scenario_id='S21' AND CONCAT_WS('|',COALESCE(title,''),COALESCE(short_title,''),COALESCE(descr,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(evidence_req,''),COALESCE(wrong,''),COALESCE(parallel_desc,''),COALESCE(action,''),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(form,'')) LIKE '%S21%')
                     + (SELECT COUNT(*) FROM scenarios
                          WHERE id='S21' AND CONCAT_WS('|',COALESCE(name,''),COALESCE(descr,''),COALESCE(alert,'')) LIKE '%S21%'));
SET @p_cov := (SELECT COUNT(*) FROM (
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
SET @p_score100 := (SELECT COUNT(*) FROM (
    SELECT scenario_id, SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS s
    FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t WHERE t.s=100);

SET @fp_scen_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(category,''),COALESCE(name,''),COALESCE(type,''),COALESCE(level,''),COALESCE(status,''),COALESCE(version,''),COALESCE(updated,''),COALESCE(nodes,-1),COALESCE(descr,''),COALESCE(alert,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id IN ('S01','S02','S03','S04','S05','S06','S07','S08','S09','S10','S11','S12','S13','S14','S15','S16','S17','S18','S19'));
SET @fp_nodes_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(type,''),COALESCE(title,''),COALESCE(target,''),COALESCE(timeout,-1),COALESCE(form,''),COALESCE(descr,''),COALESCE(seq,-1),COALESCE(required,-1),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id IN ('S01','S02','S03','S04','S05','S06','S07','S08','S09','S10','S11','S12','S13','S14','S15','S16','S17','S18','S19'));

SET @p_ok := IF(@p_ledger=1 AND @p_self=0
  AND @p_scen_all=20 AND @p_active=20
  AND @p_s20_scen=0 AND @p_s21_scen=1 AND @p_s21_nodes=61 AND @p_s21_del=0
  AND @p_s21_correct=21 AND @p_s21_score=100 AND @p_s21_dist=40 AND @p_s21_end=1
  AND @p_s21_deps=20 AND @p_s21_anchor=40
  AND @p_s21_baddep=0 AND @p_s21_badanchor=0 AND @p_s21_baddist=0
  AND @p_id_free=0 AND @p_ref=0 AND @p_idref=0 AND @p_payload=0 AND @p_text=0
  AND @p_cov=20 AND @p_score100=20, 1, NULL);
SELECT IF(@p_ok=1, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 1. 改编号：节点 id → 场景外键 → 依赖 → 锚点 → 场景行 =====
UPDATE script_nodes SET id=REPLACE(id,'S21-','S20-')
 WHERE scenario_id='S21' AND (id LIKE 'S21-%' OR id LIKE 'DSS21-%');

UPDATE script_nodes SET scenario_id='S20'
 WHERE scenario_id='S21';

UPDATE script_nodes SET deps=REPLACE(deps,'S21-','S20-')
 WHERE scenario_id='S20' AND deps LIKE '%S21-%';

UPDATE script_nodes SET anchor_id=REPLACE(anchor_id,'S21-','S20-')
 WHERE scenario_id='S20' AND COALESCE(anchor_id,'') LIKE '%S21-%';

UPDATE scenarios SET id='S20', updated='2026-11-11'
 WHERE id='S21';

-- ===== 2. 后置断言 =====
SET @q_scen_all := (SELECT COUNT(*) FROM scenarios);
SET @q_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');
SET @q_s21_scen := (SELECT COUNT(*) FROM scenarios WHERE id='S21');
SET @q_s20_scen := (SELECT COUNT(*) FROM scenarios WHERE id='S20' AND COALESCE(status,'启用')='启用' AND nodes=61
  AND name='数据权限（权限配置错误致越权访问）');
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20');
SET @q_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND flow_type='correct');
SET @q_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S20' AND flow_type='correct');
SET @q_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND flow_type='distractor');
SET @q_end := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND type='end');
SET @q_id1 := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND id LIKE 'S20-%');
SET @q_id2 := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND id LIKE 'DSS20-%');
SET @q_s21_left := (SELECT COUNT(*) FROM script_nodes
  WHERE id LIKE 'S21-%' OR id LIKE 'DSS21-%' OR scenario_id='S21'
     OR deps LIKE '%S21-%' OR COALESCE(anchor_id,'') LIKE '%S21-%');
SET @q_anchor := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND COALESCE(anchor_id,'')<>'');
SET @q_baddep := (SELECT COUNT(*) FROM script_nodes n
  JOIN (SELECT 0 i UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3
        UNION ALL SELECT 4 UNION ALL SELECT 5) idx ON idx.i < JSON_LENGTH(n.deps)
  WHERE n.scenario_id='S20' AND n.deleted_at IS NULL
    AND n.deps IS NOT NULL AND n.deps<>'' AND n.deps<>'[]'
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S20' AND c.deleted_at IS NULL
                      AND c.id = JSON_UNQUOTE(JSON_EXTRACT(n.deps, CONCAT('$[',idx.i,']')))));
SET @q_badanchor := (SELECT COUNT(*) FROM script_nodes n
  WHERE n.scenario_id='S20' AND n.deleted_at IS NULL AND COALESCE(n.anchor_id,'')<>''
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S20' AND c.deleted_at IS NULL AND c.id=n.anchor_id));
SET @q_baddist := (SELECT COUNT(*) FROM script_nodes n
  WHERE n.scenario_id='S20' AND n.deleted_at IS NULL AND n.flow_type='correct' AND n.type<>'end'
    AND (SELECT COUNT(*) FROM script_nodes d WHERE d.scenario_id='S20' AND d.deleted_at IS NULL
           AND d.flow_type='distractor' AND d.anchor_id=n.id) < 2);
SET @q_timeout := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND deleted_at IS NULL AND (timeout<5 OR timeout>120));
SET @q_gap := (SELECT COUNT(*) FROM (
    SELECT 1 n UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5
    UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9 UNION ALL SELECT 10
    UNION ALL SELECT 11 UNION ALL SELECT 12 UNION ALL SELECT 13 UNION ALL SELECT 14 UNION ALL SELECT 15
    UNION ALL SELECT 16 UNION ALL SELECT 17 UNION ALL SELECT 18 UNION ALL SELECT 19 UNION ALL SELECT 20) seq
  WHERE NOT EXISTS (SELECT 1 FROM scenarios WHERE id=CONCAT('S',LPAD(seq.n,2,'0'))));
SET @q_numeric := (SELECT COUNT(*) FROM scenarios WHERE id REGEXP '^S[0-9]+$');

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
SET @q_s20_cov := (SELECT COUNT(*) FROM (
    SELECT MAX(n.target=(SELECT name FROM roles WHERE id='duty')) AS duty,
           MAX(n.target=(SELECT name FROM roles WHERE id='netsec')) AS netsec,
           MAX(n.target=(SELECT name FROM roles WHERE id='role_mvnaewi63am')) AS datasec,
           MAX(n.target=(SELECT name FROM roles WHERE id='sysadmin')) AS sysadmin,
           MAX(n.target=(SELECT name FROM roles WHERE id='itlead')) AS itlead,
           MAX(n.target=(SELECT name FROM roles WHERE id='commander')) AS commander
      FROM script_nodes n WHERE n.scenario_id='S20' AND n.deleted_at IS NULL) c
  WHERE c.duty=1 AND c.netsec=1 AND c.datasec=1 AND c.sysadmin=1 AND c.itlead=1 AND c.commander=1);
SET @q_score100 := (SELECT COUNT(*) FROM (
    SELECT scenario_id, SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS s
    FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t WHERE t.s=100);
SET @q_text := (SELECT COUNT(*) FROM script_nodes
  WHERE scenario_id='S20' AND CONCAT_WS('|',COALESCE(title,''),COALESCE(short_title,''),COALESCE(descr,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(evidence_req,''),COALESCE(wrong,''),COALESCE(parallel_desc,''),COALESCE(action,''),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(form,'')) LIKE '%S21%');

SET @fp_scen_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(category,''),COALESCE(name,''),COALESCE(type,''),COALESCE(level,''),COALESCE(status,''),COALESCE(version,''),COALESCE(updated,''),COALESCE(nodes,-1),COALESCE(descr,''),COALESCE(alert,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id IN ('S01','S02','S03','S04','S05','S06','S07','S08','S09','S10','S11','S12','S13','S14','S15','S16','S17','S18','S19'));
SET @fp_nodes_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(type,''),COALESCE(title,''),COALESCE(target,''),COALESCE(timeout,-1),COALESCE(form,''),COALESCE(descr,''),COALESCE(seq,-1),COALESCE(required,-1),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id IN ('S01','S02','S03','S04','S05','S06','S07','S08','S09','S10','S11','S12','S13','S14','S15','S16','S17','S18','S19'));

SET @q_ok := IF(@q_scen_all=20 AND @q_active=20
  AND @q_s21_scen=0 AND @q_s20_scen=1
  AND @q_nodes=61 AND @q_correct=21 AND @q_score=100 AND @q_dist=40 AND @q_end=1
  AND @q_id1=21 AND @q_id2=40 AND @q_s21_left=0
  AND @q_anchor=40 AND @q_baddep=0 AND @q_badanchor=0 AND @q_baddist=0 AND @q_timeout=0
  AND @q_gap=0 AND @q_numeric=20
  AND @q_cov=20 AND @q_s20_cov=1 AND @q_score100=20 AND @q_text=0
  AND @fp_scen_out2=@fp_scen_out AND @fp_nodes_out2=@fp_nodes_out, 1, NULL);
SELECT IF(@q_ok=1, 1, (SELECT table_name FROM information_schema.tables));
