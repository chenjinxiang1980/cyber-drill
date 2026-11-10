-- 0104_brand_version_086.up.sql
-- 0.8.6 品牌版本号统一：settings.brand_version 从 v0.8.5 更新为 v0.8.6。
-- 仅动 brand_version 一行；前置断言：库已应用到 0103 且当前值为 v0.8.5；
-- 写后校验 brand_version 已更新且其余 settings 行指纹不变。

SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
SET @c_mig  := (SELECT COUNT(*) FROM schema_migrations);
SET @c_last := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
SET @c_bv   := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v0.8.5');
SET @fp_other_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,COALESCE(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');

SET @guard_pre := IF(@c_mig=59 AND @c_last='0103_brand_version_085' AND @c_bv=1, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 写入 =====
UPDATE settings SET value='v0.8.6' WHERE `key`='brand_version' AND value='v0.8.5';

-- ===== 后置断言 =====
SET @fp_other_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,COALESCE(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');
SET @q1 := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v0.8.6');

SET @guard_post := IF(@q1=1 AND @fp_other_after<=>@fp_other_before, 1, (SELECT table_name FROM information_schema.tables));
