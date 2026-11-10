-- =====================================================================
-- 0012_form_template_code.up.sql
-- 表单模板展示编号（三位补零）
-- ---------------------------------------------------------------------
-- 背景：
--   form_templates.id 是 F1…F17 这种不补零编号，按字符串排序时 F10 会排在
--   F1 之后、F2 之前，系统设置的模板列表与剧本编排的「关联表单」下拉顺序错乱。
--
-- 方案（方案 B）：
--   id 一律不动（历史数据零改动），新增展示列 code，仅用于展示与排序。
--   script_nodes.form、tasks.forms、form_submissions.form_id、impact_events
--   等所有引用关系继续走 id，不受影响。
--
-- 变更：
--   1) 新增 code VARCHAR(16) NULL
--   2) 按 id 的数字大小回填 F001…F017
--   3) 收紧为 NOT NULL，并加唯一索引
--
-- 幂等：
--   由 schema_migrations 版本表保证只执行一次；第 2 步 UPDATE 为固定终值，
--   重复执行结果一致。
--
-- 回滚方式（人工执行）：
--   ALTER TABLE form_templates DROP INDEX uk_form_templates_code;
--   ALTER TABLE form_templates DROP COLUMN code;
--
-- 注意：
--   MySQL 的 DDL 会隐式提交，本文件不能整体回滚；第 2 步 UPDATE 本身是原子的。
--   若存在不符合 ^F[0-9]+$ 的 id，第 3 步会因 NULL 报错终止，属于预期保护。
-- =====================================================================
SET NAMES utf8mb4;

ALTER TABLE form_templates
    ADD COLUMN code VARCHAR(16) NULL
    COMMENT '展示编号（F001…），仅用于展示与排序，引用仍用 id'
    AFTER id;

UPDATE form_templates
   SET code = CONCAT('F', LPAD(CAST(SUBSTRING(id, 2) AS UNSIGNED), 3, '0'))
 WHERE id REGEXP '^F[0-9]+$';

ALTER TABLE form_templates
    MODIFY COLUMN code VARCHAR(16) NOT NULL
    COMMENT '展示编号（F001…），仅用于展示与排序，引用仍用 id';

ALTER TABLE form_templates
    ADD UNIQUE KEY uk_form_templates_code (code);
