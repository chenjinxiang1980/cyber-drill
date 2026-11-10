-- 剧本节点软删除
-- deleted_at 为 NULL 表示有效；删除节点只打标记，保留 drill_steps 追溯与版本回滚。
-- 可重复执行：DDL 先查 information_schema。

SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'script_nodes' AND column_name = 'deleted_at'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE script_nodes ADD COLUMN deleted_at VARCHAR(19) DEFAULT NULL',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_idx := (
  SELECT COUNT(*) FROM information_schema.statistics
   WHERE table_schema = DATABASE() AND table_name = 'script_nodes' AND index_name = 'idx_sn_deleted'
);
SET @ddl2 := IF(@has_idx = 0,
  'ALTER TABLE script_nodes ADD KEY idx_sn_deleted (scenario_id, deleted_at)',
  'SELECT 1');
PREPARE stmt2 FROM @ddl2;
EXECUTE stmt2;
DEALLOCATE PREPARE stmt2;

UPDATE script_nodes SET deleted_at = NULL WHERE deleted_at = '';
