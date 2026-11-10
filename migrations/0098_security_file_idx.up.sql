-- 0098: 为 /uploads、/media 的对象级授权补充 file_path 反查索引
-- 背景：P0-1 修复后，静态文件出站前要按 file_path 反查 evidence / task_attachments / knowledge
--       判断调用者角色与归属，避免仅凭路径直接读取他人材料。
-- 说明：MySQL 8 不支持 CREATE INDEX IF NOT EXISTS，这里用 information_schema 判重 + 动态 DDL。

-- evidence.file_path
SET @has_ev_fp := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'evidence' AND index_name = 'idx_ev_file_path'
);
SET @ddl_ev_fp := IF(@has_ev_fp = 0,
  'CREATE INDEX idx_ev_file_path ON evidence(file_path)',
  'SELECT 1');
PREPARE stmt_ev_fp FROM @ddl_ev_fp;
EXECUTE stmt_ev_fp;
DEALLOCATE PREPARE stmt_ev_fp;

-- task_attachments.file_path
SET @has_att_fp := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'task_attachments' AND index_name = 'idx_att_file_path'
);
SET @ddl_att_fp := IF(@has_att_fp = 0,
  'CREATE INDEX idx_att_file_path ON task_attachments(file_path)',
  'SELECT 1');
PREPARE stmt_att_fp FROM @ddl_att_fp;
EXECUTE stmt_att_fp;
DEALLOCATE PREPARE stmt_att_fp;

-- knowledge.file_path
SET @has_kb_fp := (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'knowledge' AND index_name = 'idx_kb_file_path'
);
SET @ddl_kb_fp := IF(@has_kb_fp = 0,
  'CREATE INDEX idx_kb_file_path ON knowledge(file_path)',
  'SELECT 1');
PREPARE stmt_kb_fp FROM @ddl_kb_fp;
EXECUTE stmt_kb_fp;
DEALLOCATE PREPARE stmt_kb_fp;
