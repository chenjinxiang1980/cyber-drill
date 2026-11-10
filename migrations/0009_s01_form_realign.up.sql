-- =====================================================================
-- 0009_s01_form_realign.up.sql
-- S01 表单错配修正（第二批）· seq 倒挂修正
-- ---------------------------------------------------------------------
-- 适用前提：
--   1. 已按顺序应用 0001 ~ 0008（script_nodes 348 条、form_templates 14 张，
--      含 F14《应急启动申请单》review）。
--   2. 与历史基线的关系：不修改 0007，也不修改 0008；本文件只做增量。
--   3. 全部为 DML（无 DDL、无 DELETE、无 TRUNCATE），可放入单个事务。
--      迁移器（drill-migrate）本身用事务包裹本文件；手工执行时请自行
--      `START TRANSACTION; ... COMMIT;` 包裹。
--   4. 幂等：模板用 INSERT ... ON DUPLICATE KEY UPDATE；字段追加用
--      JSON_SEARCH 判重（已存在则跳过）；绑定与 seq 用按主键 UPDATE。
--   5. 不改动 F9《综合处置记录单》的任何字段——它被 8 本剧本 34 个节点复用。
-- ---------------------------------------------------------------------
-- 改动清单：
--   a. 新增 F15《外部单位指导意见记录单》(fill)，S01-12 / S01-16 由 F9 改绑 F15
--   b. 新增 F16《业务兜底处置记录单》(fill)，S01-10 由 F9 改绑 F16
--   c. 新增 F17《数据补录稽核单》(fill)，S01-14 由 F9 改绑 F17
--   d. F3 追加「最终定级」「研判结论」
--   e. F6 追加「来源单位」「接收人」「接收时间」
--   f. F8 追加「根因分析」「经验教训」「改进建议」
--   g. S01-9 seq 9→8、S01-10 seq 8→9，修正依赖顺序倒挂
-- ---------------------------------------------------------------------
-- 回滚方式（人工执行）：
--   UPDATE script_nodes SET form='F9'  WHERE id IN ('S01-12','S01-16','S01-10','S01-14');
--   DELETE FROM form_templates WHERE id IN ('F15','F16','F17');
--   UPDATE script_nodes SET seq=9 WHERE id='S01-9';
--   UPDATE script_nodes SET seq=8 WHERE id='S01-10';
--   （F3/F6/F8 的追加字段需按 label 用 JSON_REMOVE 单独移除，或从备份恢复。）
-- =====================================================================
SET NAMES utf8mb4;

-- ---------------------------------------------------------------------
-- a. F15 外部单位指导意见记录单
--    接收市卫健局 / 市网信办 / 市网安大队等外部指导的登记表。
--    「是否立案」「承办警官」为可选字段，供网安节点使用，不另建表。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, name, fields, mode) VALUES
('F15','外部单位指导意见记录单',
 '[{"label":"来源单位","type":"select","options":["市卫生健康局","市网信办","市网安大队","市应急管理局","其他"],"required":true,"hint":"选择下达指导意见的外部单位。"},
   {"label":"指导意见摘要","type":"textarea","required":true,"hint":"如实记录对方提出的研判结论、处置要求与口径，不要改写成本院处置动作。"},
   {"label":"要求时限","type":"text","required":true,"hint":"如「2 小时内反馈」「2026-11-06 18:00 前」，无明确时限填「未明确」。"},
   {"label":"接收人","type":"text","required":true,"hint":"本院接收该指导意见的人员姓名与岗位。"},
   {"label":"接收时间","type":"datetime","required":true,"hint":"实际接收时间，精确到分钟。"},
   {"label":"落实情况","type":"textarea","required":false,"hint":"按指导意见已落实的事项与进度，可与后续处置节点相互印证。"},
   {"label":"是否立案","type":"select","options":["已立案","未立案","不适用"],"required":false,"hint":"仅网安大队指导时填写；其他单位可留空。"},
   {"label":"承办警官","type":"text","required":false,"hint":"仅网安大队指导时填写承办警官姓名或警号。"}]',
 'fill')
ON DUPLICATE KEY UPDATE name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- b. F16 业务兜底处置记录单
--    只装本节点实际要填的内容：手工流程启用的环节、时间、科室与验证。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, name, fields, mode) VALUES
('F16','业务兜底处置记录单',
 '[{"label":"兜底启动时间","type":"datetime","required":true,"hint":"首个环节切换为手工流程的时间，精确到分钟。"},
   {"label":"启用环节","type":"multiselect","options":["挂号","收费","门诊医嘱","发药","检验申请","检查预约"],"required":true,"hint":"勾选本次已切换为手工流程的环节（可多选）；如本院口径不同可在后台调整选项。"},
   {"label":"各环节执行科室","type":"textarea","required":true,"hint":"按「环节 / 执行科室 / 责任人」逐条列明。"},
   {"label":"纸质单据号段","type":"text","required":true,"hint":"挂号、收费、发药等纸质单据的号段，便于事后补录与稽核。"},
   {"label":"切换验证","type":"select","options":["通过","部分通过","未通过"],"required":true,"hint":"手工流程实际跑通情况的验证结论。"},
   {"label":"回退条件","type":"textarea","required":false,"hint":"明确何时、由谁决定由手工切回系统，避免双轨长期并行。"},
   {"label":"责任人","type":"text","required":true,"hint":"本次业务兜底的统筹责任人姓名与岗位。"},
   {"label":"记录时间","type":"datetime","required":true,"hint":"完成本记录的时间。"}]',
 'fill')
ON DUPLICATE KEY UPDATE name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- c. F17 数据补录稽核单
--    只装本节点实际要填的内容：补录条数、稽核差异、封账与责任人。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, name, fields, mode) VALUES
('F17','数据补录稽核单',
 '[{"label":"补录环节","type":"multiselect","options":["挂号","收费","门诊医嘱","发药","检验申请","检查预约"],"required":true,"hint":"勾选本次完成补录的环节（可多选）。"},
   {"label":"补录条数","type":"text","required":true,"hint":"按环节分别填写补录单据条数，如「挂号 128、收费 96」。"},
   {"label":"补录起止时间","type":"text","required":true,"hint":"格式如 2026-11-06 10:00 至 16:00。"},
   {"label":"稽核差异条数","type":"text","required":true,"hint":"双人复核后发现的差异笔数，无差异填「0」。"},
   {"label":"差异明细与处理","type":"textarea","required":true,"hint":"逐笔写明差异单据、金额或药品流向、处理结论；无差异填「无」。"},
   {"label":"双人复核人","type":"text","required":true,"hint":"补录人与复核人双方姓名，不得为同一人。"},
   {"label":"封账时间","type":"datetime","required":true,"hint":"差异查清并完成补录封账的时间。"},
   {"label":"记录人","type":"text","required":true,"hint":"填写本记录的人员姓名与岗位。"}]',
 'fill')
ON DUPLICATE KEY UPDATE name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- d. F3 业务影响评估表：追加「最终定级」「研判结论」（追加到末尾）
-- ---------------------------------------------------------------------
UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','最终定级','type','select',
        'options', JSON_ARRAY('一般','较大','重大','特别重大'),
        'required', TRUE,
        'hint','本节点确认后的最终事件等级，须与上报表、启动申请保持一致。'))
 WHERE id='F3'
   AND JSON_SEARCH(fields,'one','最终定级',NULL,'$[*].label') IS NULL;

UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','研判结论','type','textarea',
        'required', TRUE,
        'hint','写明确认定级的依据、结论与后续启动建议，供审批节点引用。'))
 WHERE id='F3'
   AND JSON_SEARCH(fields,'one','研判结论',NULL,'$[*].label') IS NULL;

-- ---------------------------------------------------------------------
-- e. F6 舆情风险研判表：追加「来源单位」「接收人」「接收时间」
-- ---------------------------------------------------------------------
UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','来源单位','type','text',
        'required', TRUE,
        'hint','下达舆情或监管口径的外部单位，如市网信办。'))
 WHERE id='F6'
   AND JSON_SEARCH(fields,'one','来源单位',NULL,'$[*].label') IS NULL;

UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','接收人','type','text',
        'required', TRUE,
        'hint','本院接收该口径指导的人员姓名与岗位。'))
 WHERE id='F6'
   AND JSON_SEARCH(fields,'one','接收人',NULL,'$[*].label') IS NULL;

UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','接收时间','type','datetime',
        'required', TRUE,
        'hint','实际接收口径指导的时间，精确到分钟。'))
 WHERE id='F6'
   AND JSON_SEARCH(fields,'one','接收时间',NULL,'$[*].label') IS NULL;

-- ---------------------------------------------------------------------
-- f. F8 反馈单：追加「根因分析」「经验教训」「改进建议」
-- ---------------------------------------------------------------------
UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','根因分析','type','textarea',
        'required', TRUE,
        'hint','从技术、管理、流程三方面写明本次事件的直接原因与根本原因。'))
 WHERE id='F8'
   AND JSON_SEARCH(fields,'one','根因分析',NULL,'$[*].label') IS NULL;

UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','经验教训','type','textarea',
        'required', TRUE,
        'hint','总结本次处置中有效的做法与暴露的短板，注明对应节点。'))
 WHERE id='F8'
   AND JSON_SEARCH(fields,'one','经验教训',NULL,'$[*].label') IS NULL;

UPDATE form_templates
   SET fields = JSON_ARRAY_APPEND(fields, '$', JSON_OBJECT(
        'label','改进建议','type','textarea',
        'required', TRUE,
        'hint','提出可落地的整改建议与责任部门，与「下一步整改及工作计划」区分：本栏重建议，后者重排期。'))
 WHERE id='F8'
   AND JSON_SEARCH(fields,'one','改进建议',NULL,'$[*].label') IS NULL;

-- ---------------------------------------------------------------------
-- g. 绑定调整：外部指导 / 业务兜底 / 数据补录 各自独立承载
--    S01-12 市卫健局研判指导  F9 → F15
--    S01-16 市网安大队处置指导 F9 → F15
--    S01-10 业务兜底处置      F9 → F16
--    S01-14 数据补录稽核      F9 → F17
-- ---------------------------------------------------------------------
UPDATE script_nodes SET form='F15' WHERE id='S01-12';
UPDATE script_nodes SET form='F15' WHERE id='S01-16';
UPDATE script_nodes SET form='F16' WHERE id='S01-10';
UPDATE script_nodes SET form='F17' WHERE id='S01-14';

-- ---------------------------------------------------------------------
-- h. seq 倒挂修正：S01-9 固定证据应先于 S01-10 业务兜底
--    先置临时值再交换，兼容未来可能出现的 (scenario_id, seq) 唯一约束。
-- ---------------------------------------------------------------------
UPDATE script_nodes SET seq=1008 WHERE id='S01-9';
UPDATE script_nodes SET seq=8    WHERE id='S01-10';
UPDATE script_nodes SET seq=9    WHERE id='S01-9';
