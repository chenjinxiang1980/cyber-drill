-- 0095 节点处置中状态补充 user_id
-- 背景：POST /api/drill/nodes/:nodeId/start 一直写入 user_id 列，但 0001 建表时没有该列，
--       MySQL 报 1054 且错误被吞，导致 drill_node_state 永远 0 行、协同页「处置中」永不出现。
-- 本迁移补列并加索引；可重复执行（DDL 先查 information_schema）。

SET NAMES utf8mb4;

SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'drill_node_state' AND column_name = 'user_id'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE drill_node_state ADD COLUMN user_id BIGINT DEFAULT NULL AFTER user',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_idx := (
  SELECT COUNT(*) FROM information_schema.statistics
   WHERE table_schema = DATABASE() AND table_name = 'drill_node_state' AND index_name = 'idx_dns_user'
);
SET @ddl2 := IF(@has_idx = 0,
  'ALTER TABLE drill_node_state ADD KEY idx_dns_user (project_id, user_id)',
  'SELECT 1');
PREPARE stmt2 FROM @ddl2;
EXECUTE stmt2;
DEALLOCATE PREPARE stmt2;
