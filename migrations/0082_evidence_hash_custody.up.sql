-- 0082 证据哈希与保管链（P1-07 结构侧）
-- 依据：评审 P1-07 —— evidence 表没有文件摘要、保管人、封存时间与保管链，
-- F13 填报的哈希无法与归档文件实际哈希比对，报告也就无法展示校验结果。
-- 本迁移只加列与回填，不改任何既有列与既有数据语义；含 DDL，按可重复执行编写。

SET NAMES utf8mb4;

-- ---- sha256 ----
SET @has_sha := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'evidence' AND column_name = 'sha256'
);
SET @ddl := IF(@has_sha = 0,
  'ALTER TABLE evidence ADD COLUMN sha256 CHAR(64) DEFAULT NULL',
  'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ---- custodian ----
SET @has_cust := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'evidence' AND column_name = 'custodian'
);
SET @ddl2 := IF(@has_cust = 0,
  'ALTER TABLE evidence ADD COLUMN custodian VARCHAR(64) DEFAULT NULL',
  'SELECT 1');
PREPARE stmt2 FROM @ddl2; EXECUTE stmt2; DEALLOCATE PREPARE stmt2;

-- ---- sealed_at ----
SET @has_sealed := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'evidence' AND column_name = 'sealed_at'
);
SET @ddl3 := IF(@has_sealed = 0,
  'ALTER TABLE evidence ADD COLUMN sealed_at VARCHAR(19) DEFAULT NULL',
  'SELECT 1');
PREPARE stmt3 FROM @ddl3; EXECUTE stmt3; DEALLOCATE PREPARE stmt3;

-- ---- chain_of_custody ----
SET @has_chain := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'evidence' AND column_name = 'chain_of_custody'
);
SET @ddl4 := IF(@has_chain = 0,
  'ALTER TABLE evidence ADD COLUMN chain_of_custody TEXT DEFAULT NULL',
  'SELECT 1');
PREPARE stmt4 FROM @ddl4; EXECUTE stmt4; DEALLOCATE PREPARE stmt4;

-- ---- 摘要检索索引 ----
SET @has_idx := (
  SELECT COUNT(*) FROM information_schema.statistics
   WHERE table_schema = DATABASE() AND table_name = 'evidence' AND index_name = 'idx_ev_sha'
);
SET @ddl5 := IF(@has_idx = 0,
  'ALTER TABLE evidence ADD KEY idx_ev_sha (sha256)',
  'SELECT 1');
PREPARE stmt5 FROM @ddl5; EXECUTE stmt5; DEALLOCATE PREPARE stmt5;

-- ---- 历史数据回填（幂等：只补空值） ----
UPDATE evidence
   SET custodian = uploader
 WHERE (custodian IS NULL OR custodian = '') AND uploader IS NOT NULL AND uploader <> '';

UPDATE evidence
   SET sealed_at = COALESCE(NULLIF(created_at, ''), time)
 WHERE sealed_at IS NULL OR sealed_at = '';

UPDATE evidence
   SET chain_of_custody = JSON_ARRAY(JSON_OBJECT(
       'at', COALESCE(NULLIF(created_at, ''), time),
       'actor', COALESCE(NULLIF(uploader, ''), 'system'),
       'action', '历史归档',
       'note', '迁移回填'))
 WHERE chain_of_custody IS NULL OR chain_of_custody = '';
