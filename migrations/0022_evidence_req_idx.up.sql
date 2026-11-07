-- 取证完整率查询索引：结束核对按 (project_id, req_key) 判断已归档
-- MySQL 8 无 CREATE INDEX IF NOT EXISTS，使用 information_schema 判断保证幂等。
SET @idx_exists := (
    SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'evidence'
      AND index_name = 'idx_ev_proj_req'
);
SET @idx_sql := IF(@idx_exists = 0,
    'ALTER TABLE evidence ADD INDEX idx_ev_proj_req (project_id, req_key)',
    'SELECT 1');
PREPARE idx_stmt FROM @idx_sql;
EXECUTE idx_stmt;
DEALLOCATE PREPARE idx_stmt;
