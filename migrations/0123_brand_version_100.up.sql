-- 0123_brand_version_100.up.sql
-- 1.0.0 品牌版本号统一：settings.brand_version 从 v0.9.0 更新为 v1.0.0。
-- 仅动 brand_version 一行；前置断言：库已应用到 0122 且当前值为 v0.9.0；
-- 后置断言：迁移记录 76 条、brand_version 已更新、其余 settings 行指纹不变。
-- 升级路径：0.9.0 现网（75 条 / 末条 0122）→ 0123。
--
-- 说明：drill-migrate 在同一个事务里执行本文件后才会补写 schema_migrations；
-- 为满足“后置断言 76 条”，本文件先自行插入 0123 记录，迁移器随后的
-- INSERT ... ON DUPLICATE KEY UPDATE 是幂等的，不会重复计数。

SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
SET @c_mig  := (SELECT COUNT(*) FROM schema_migrations);
SET @c_last := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
SET @c_bv   := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v0.9.0');
SET @fp_other_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,COALESCE(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');

SET @guard_pre := IF(@c_mig=75 AND @c_last='0122_brand_version_090' AND @c_bv=1, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 写入 =====
UPDATE settings SET value='v1.0.0' WHERE `key`='brand_version' AND value='v0.9.0';
INSERT INTO schema_migrations (version) VALUES ('0123_brand_version_100')
  ON DUPLICATE KEY UPDATE version=version;

-- ===== 后置断言 =====
SET @c_mig_after := (SELECT COUNT(*) FROM schema_migrations);
SET @last_after  := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
SET @fp_other_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,COALESCE(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');
SET @q1 := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v1.0.0');

SET @guard_post := IF(@c_mig_after=76 AND @last_after='0123_brand_version_100'
                      AND @q1=1 AND @fp_other_after<=>@fp_other_before,
                      1, (SELECT table_name FROM information_schema.tables));
