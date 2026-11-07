-- =====================================================================
-- 0014_external_unit_form_fields.up.sql
-- 外单位相关表单字段调整（模板级）
-- ---------------------------------------------------------------------
-- 变更：
--   A. F15《外部单位指导意见记录单》增加“受理编号”（text，选填）
--      插入到“来源单位”之后，覆盖公安受理回执场景
--   B. F14《应急启动申请单》“审批事项”删除“结束Ⅱ级应急响应”选项，
--      结束动作统一走 F18《应急响应结束记录单》
--
-- 不改：F6（本轮只给停用/拆分结论，不删除、不改字段）；
--       form_templates 其它模板；script_nodes 不涉及。
--
-- 幂等：
--   * F15 仅在缺少“受理编号”时插入（JSON_SEARCH 守卫）
--   * F14 仅在仍含“结束Ⅱ级应急响应”时替换（JSON_SEARCH 守卫）
--   * 重复执行不产生二次变更
--
-- 事务与断言：
--   * 由执行脚本包在单事务中；本文件的断言守卫用临时表 CHECK 约束，
--     任一断言不成立即语句报错，连接断开后事务自动回滚
--   * 临时表为会话级，不落库、不影响 schema
-- =====================================================================
SET NAMES utf8mb4;
USE drill_system;

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

-- ---------------------------------------------------------------------
-- A. F15 增加“受理编号”（仅当不存在时插入到下标 1）
-- ---------------------------------------------------------------------
UPDATE form_templates
   SET fields = JSON_ARRAY_INSERT(
         fields,
         '$[1]',
         JSON_OBJECT(
           'label', '受理编号',
           'type', 'text',
           'required', FALSE,
           'hint', '网安大队受理报案时填写受理编号；其他单位可留空。'
         )
       )
 WHERE id = 'F15'
   AND JSON_SEARCH(fields, 'one', '受理编号', NULL, '$[*].label') IS NULL;

-- ---------------------------------------------------------------------
-- B. F14 删除“结束Ⅱ级应急响应”选项，并修正相关提示
-- ---------------------------------------------------------------------
UPDATE form_templates
   SET fields = JSON_REPLACE(
         fields,
         '$[0].options', JSON_ARRAY('启动Ⅱ级应急响应'),
         '$[5].hint', '批准启动的响应级别。'
       )
 WHERE id = 'F14'
   AND JSON_SEARCH(fields, 'one', '结束Ⅱ级应急响应', NULL, '$[*].options[*]') IS NOT NULL;

-- ---------------------------------------------------------------------
-- 断言
-- ---------------------------------------------------------------------
-- F15 字段数为 9，受理编号位于“来源单位”之后
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM form_templates
    WHERE id = 'F15'
      AND JSON_LENGTH(fields) = 9
      AND JSON_UNQUOTE(JSON_EXTRACT(fields, '$[0].label')) = '来源单位'
      AND JSON_UNQUOTE(JSON_EXTRACT(fields, '$[1].label')) = '受理编号'
      AND JSON_UNQUOTE(JSON_EXTRACT(fields, '$[2].label')) = '指导意见摘要'
      AND JSON_UNQUOTE(JSON_EXTRACT(fields, '$[1].required')) = 'false'
  ) = 1, 1, 0);

-- F14 仅保留启动选项，且全表不再出现“结束Ⅱ级应急响应”
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM form_templates
    WHERE id = 'F14'
      AND JSON_EXTRACT(fields, '$[0].options') = JSON_ARRAY('启动Ⅱ级应急响应')
      AND JSON_UNQUOTE(JSON_EXTRACT(fields, '$[5].hint')) = '批准启动的响应级别。'
      AND JSON_SEARCH(fields, 'one', '结束Ⅱ级应急响应', NULL, '$[*].options[*]') IS NULL
  ) = 1, 1, 0);

-- 模板总数不变
INSERT INTO _assert_guard (v)
SELECT IF((SELECT COUNT(*) FROM form_templates) = 18, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
