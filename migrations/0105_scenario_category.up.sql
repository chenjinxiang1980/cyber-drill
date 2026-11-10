-- 0105_scenario_category.up.sql
-- 剧本（场景）新增「演练大类」维度：网络安全演练 / 数据安全演练。
-- 只新增一列，不改任何既有列、不改任何既有数据；
-- 历史行由列默认值安全归入「网络安全演练」，0106 再做显式归类与指纹断言。
-- 可重复执行：DDL 先查 information_schema。

SET NAMES utf8mb4;

SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'scenarios' AND column_name = 'category'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE scenarios ADD COLUMN category VARCHAR(32) NOT NULL DEFAULT ''网络安全演练'' AFTER id',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 后置断言：列存在且为 NOT NULL（不允许出现 NULL 分类）
SET @col_ok := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'scenarios'
     AND column_name = 'category' AND IS_NULLABLE = 'NO'
);
SET @guard_post := IF(@col_ok = 1, 1, (SELECT table_name FROM information_schema.tables));
