-- 0106_scenario_category_backfill.up.sql
-- 纯数据迁移：把现有内置网络安全剧本（S01–S08）显式归入「网络安全演练」，
-- 并把任何未指定 / 空分类的历史行安全兜底为「网络安全演练」。
-- 不触碰 script_nodes（节点、内容、分值、依赖、流程）与任何封存快照；
-- 写入前后分别对 scenarios（排除 category 列）与 script_nodes 计算指纹并断言不变。

SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
-- 1) 0105 已加列；2) 上一个迁移版本正确（迁移必须按序执行）
SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'scenarios' AND column_name = 'category'
);
SET @last_mig := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
SET @guard_pre := IF(@has_col = 1 AND @last_mig = '0105_scenario_category', 1,
  (SELECT table_name FROM information_schema.tables));

-- 3) 现有内置剧本必须齐备（S01–S08 共 8 个）
SET @c_builtin := (
  SELECT COUNT(*) FROM scenarios
   WHERE id IN ('S01','S02','S03','S04','S05','S06','S07','S08')
);
SET @guard_builtin := IF(@c_builtin = 8, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 写入前指纹（排除 category 列，用于证明除分类外零变更）=====
SET @fp_scen_before := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id, COALESCE(name,''), COALESCE(type,''), COALESCE(level,''), COALESCE(difficulty,''),
    COALESCE(status,''), COALESCE(version,''), COALESCE(updated,''), COALESCE(nodes,0),
    COALESCE(`descr`,''), COALESCE(alert,'')
  ) ORDER BY id SEPARATOR '\n')) FROM scenarios
);
SET @fp_nodes_before := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id, COALESCE(scenario_id,''), COALESCE(type,''), COALESCE(title,''), COALESCE(timeout,0),
    COALESCE(form,''), COALESCE(required,1), COALESCE(flow_type,'correct'), COALESCE(score,0),
    COALESCE(receiver,''), COALESCE(deps,''), COALESCE(anchor_id,''), COALESCE(release_timing,'')
  ) ORDER BY id SEPARATOR '\n')) FROM script_nodes WHERE deleted_at IS NULL
);

-- ===== 写入：内置 8 个剧本 + 所有未指定分类的历史行 =====
UPDATE scenarios
   SET category = '网络安全演练'
 WHERE category IS NULL OR category = ''
    OR id IN ('S01','S02','S03','S04','S05','S06','S07','S08');

-- ===== 后置断言 =====
SET @fp_scen_after := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id, COALESCE(name,''), COALESCE(type,''), COALESCE(level,''), COALESCE(difficulty,''),
    COALESCE(status,''), COALESCE(version,''), COALESCE(updated,''), COALESCE(nodes,0),
    COALESCE(`descr`,''), COALESCE(alert,'')
  ) ORDER BY id SEPARATOR '\n')) FROM scenarios
);
SET @fp_nodes_after := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id, COALESCE(scenario_id,''), COALESCE(type,''), COALESCE(title,''), COALESCE(timeout,0),
    COALESCE(form,''), COALESCE(required,1), COALESCE(flow_type,'correct'), COALESCE(score,0),
    COALESCE(receiver,''), COALESCE(deps,''), COALESCE(anchor_id,''), COALESCE(release_timing,'')
  ) ORDER BY id SEPARATOR '\n')) FROM script_nodes WHERE deleted_at IS NULL
);

SET @c_empty_cat   := (SELECT COUNT(*) FROM scenarios WHERE category IS NULL OR category = '');
SET @c_builtin_cat := (
  SELECT COUNT(*) FROM scenarios
   WHERE id IN ('S01','S02','S03','S04','S05','S06','S07','S08')
     AND category = '网络安全演练'
);
SET @c_unknown_cat := (
  SELECT COUNT(*) FROM scenarios
   WHERE category NOT IN ('网络安全演练','数据安全演练')
);

SET @guard_post := IF(
  @fp_scen_after <=> @fp_scen_before
  AND @fp_nodes_after <=> @fp_nodes_before
  AND @c_empty_cat = 0
  AND @c_builtin_cat = 8
  AND @c_unknown_cat = 0,
  1, (SELECT table_name FROM information_schema.tables)
);
