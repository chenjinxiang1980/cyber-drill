-- P1-7 同一节点并发提交可重复计分
-- 1) 存量去重：同一 project + 同一 matched_node 只保留一条；
--    优先保留真实处置（skipped=0），同类记录保留写入最早的一条（id 最小）。
-- 2) impact_events.submission_id 先改指向保留记录，避免去重后溯源悬空。
-- 3) matched_node='' 归一为 NULL：唯一索引允许多个 NULL，
--    避免「通用动作记录」（无匹配节点）被误判为重复。
-- 4) 唯一索引幂等创建。
-- 可重复执行：去重/归一/改指向为幂等 DML，DDL 先查 information_schema。

-- 跳过记录被真实处置取代时，业务影响溯源改指真实处置记录
UPDATE impact_events i
JOIN drill_steps d ON i.submission_id = d.id
JOIN drill_steps k
  ON k.project_id = d.project_id
 AND k.matched_node = d.matched_node
SET i.submission_id = k.id
WHERE d.matched_node IS NOT NULL
  AND d.matched_node <> ''
  AND COALESCE(k.skipped, 0) = 0
  AND COALESCE(d.skipped, 0) = 1;

-- 同类重复记录统一改指该组 id 最小的一条（即去重后保留的那条）
UPDATE impact_events i
JOIN drill_steps d ON i.submission_id = d.id
SET i.submission_id = (
      SELECT MIN(k.id) FROM drill_steps k
       WHERE k.project_id = d.project_id
         AND k.matched_node = d.matched_node
         AND COALESCE(k.skipped, 0) = COALESCE(d.skipped, 0)
    )
WHERE d.matched_node IS NOT NULL
  AND d.matched_node <> '';

DELETE d FROM drill_steps d
JOIN drill_steps k
  ON d.project_id = k.project_id
 AND d.matched_node = k.matched_node
 AND d.matched_node IS NOT NULL
 AND d.matched_node <> ''
 AND (
      (COALESCE(d.skipped,0) = 1 AND COALESCE(k.skipped,0) = 0)
   OR (COALESCE(d.skipped,0) = COALESCE(k.skipped,0) AND d.id > k.id)
 );

UPDATE drill_steps SET matched_node = NULL WHERE matched_node = '';

SET @has_uk := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'drill_steps'
    AND index_name = 'uk_ds_proj_node'
);
SET @ddl := IF(@has_uk = 0,
  'ALTER TABLE drill_steps ADD UNIQUE KEY uk_ds_proj_node (project_id, matched_node)',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
