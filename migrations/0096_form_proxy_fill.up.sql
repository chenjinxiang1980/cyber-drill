-- 0096 外部单位表单代填留痕
-- 背景：公安/运营商/市卫生健康局/市网信办等外部单位不实际登录系统，
--       由信息科负责人代其填写表单。代填必须可追溯：记录填写人、被代填对象、代填时间。
-- 说明：submitter/user_id/role_id 仍记录真实操作人（信息科负责人），
--       proxy_* 记录被代填对象；is_proxy=1 表示本条为代填记录。
-- 可重复执行（DDL 先查 information_schema）。

SET NAMES utf8mb4;

SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'form_submissions' AND column_name = 'is_proxy'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE form_submissions
     ADD COLUMN is_proxy TINYINT NOT NULL DEFAULT 0 AFTER role_id,
     ADD COLUMN proxy_for_role_id VARCHAR(32) DEFAULT NULL AFTER is_proxy,
     ADD COLUMN proxy_for_role_name VARCHAR(64) DEFAULT NULL AFTER proxy_for_role_id,
     ADD COLUMN proxy_by_user_id BIGINT DEFAULT NULL AFTER proxy_for_role_name,
     ADD COLUMN proxy_by_name VARCHAR(64) DEFAULT NULL AFTER proxy_by_user_id',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_idx := (
  SELECT COUNT(*) FROM information_schema.statistics
   WHERE table_schema = DATABASE() AND table_name = 'form_submissions' AND index_name = 'idx_fs_proxy'
);
SET @ddl2 := IF(@has_idx = 0,
  'ALTER TABLE form_submissions ADD KEY idx_fs_proxy (proxy_for_role_id, is_proxy)',
  'SELECT 1');
PREPARE stmt2 FROM @ddl2;
EXECUTE stmt2;
DEALLOCATE PREPARE stmt2;
