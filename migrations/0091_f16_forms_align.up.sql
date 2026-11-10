-- =====================================================================
-- 0091 F16 字段补全 + 表单口径收口（业务侧 P0-5 复核修正）
-- ---------------------------------------------------------------------
-- 复核结论：F16 的 fields 在 MySQL 中是合法 JSON（JSON_VALID=1、8 个字段），
-- 之前“字面量 \n 导致 json_valid=0”是 mysql CLI 导出时把真实换行转义成
-- “\n”造成的误判，不是数据损坏。
-- 本次仍做两件事：
--   1) F16 追加“兜底期间业务量与积压情况”字段，覆盖 S01/S03/S05 的兜底量统计；
--   2) 把 S01-10/S03-9/S05-8 三个节点的证据要求改成真正可从 F16 取证的口径；
--   3) 同步 F15/F19 的提示文案（去掉“2 小时”，补代填/不报案口径）。
--
-- 纯 DML，无 DDL；可重复执行。守卫失败会触发 1242 并回滚整个迁移事务。
-- =====================================================================
SET NAMES utf8mb4;

-- 守卫 1：F16 必须是合法 JSON，否则不改，避免把坏数据覆盖成另一份坏数据
SET @g_f16_valid := (SELECT COUNT(*) FROM form_templates WHERE id='F16' AND JSON_VALID(fields)=1);
SET @guard_1 := IF(@g_f16_valid=1, 1, (SELECT table_name FROM information_schema.tables));

UPDATE form_templates
SET fields = JSON_ARRAY_APPEND(fields, '$',
    JSON_OBJECT(
      'label','兜底期间业务量与积压情况',
      'type','textarea',
      'required',FALSE,
      'hint','记录各环节手工处理量、积压量与补录安排。'))
WHERE id='F16'
  AND JSON_VALID(fields)=1
  AND JSON_SEARCH(fields,'one','兜底期间业务量与积压情况',NULL,'$[*].label') IS NULL;

-- F15：要求时限提示改为 1 小时；回执形式补“无回执可代填”
UPDATE form_templates
SET fields = JSON_SET(fields,
  '$[4].hint','如「1 小时内反馈」「2026-11-06 18:00 前」，无明确时限填「未明确」。')
WHERE id='F15' AND JSON_VALID(fields)=1;

UPDATE form_templates
SET fields = JSON_SET(fields,
  '$[2].hint','收到书面回执选「书面」；仅口头/电话回执选对应项并注明时间与对方姓名；未取得任何回执选「无回执」，由信息科负责人代填并附沟通记录。')
WHERE id='F15' AND JSON_VALID(fields)=1;

-- F19：不报案不扣分；外部意见代填说明
UPDATE form_templates
SET fields = JSON_SET(fields,
  '$[7].hint','选择暂不报案不扣分，但必须写明理由、补充措施与复核时间；该评估不等同于免于报告。')
WHERE id='F19' AND JSON_VALID(fields)=1;

UPDATE form_templates
SET fields = JSON_SET(fields,
  '$[5].hint','外部单位未登录系统时，由信息科负责人代填并注明来源、代填人与时间。')
WHERE id='F19' AND JSON_VALID(fields)=1;

-- S01-10：兜底口径由“四项”改为“六大环节”，与 F16 的启用环节一致
UPDATE script_nodes
SET evidence_req='[{"type":"处置记录","name":"兜底启动记录","note":"含六大环节、责任人、启动时间与纸质号段"},{"type":"附件","name":"手工单据样例","note":"含号段与填写示例"}]',
    keywords='手工兜底,单据号段,回退,六大环节'
WHERE id='S01-10' AND scenario_id='S01' AND deleted_at IS NULL;

-- S03-9：证据落到 F16，不再用泛化的“兜底操作记录”
UPDATE script_nodes
SET evidence_req='[{"type":"表单文书","name":"业务兜底处置记录单（F16）","note":"含收费环节、启动时间、纸质号段、责任人与补录安排"},{"type":"附件","name":"手工单据样例","note":"含号段与填写示例"}]'
WHERE id='S03-9' AND scenario_id='S03' AND deleted_at IS NULL;

-- S05-8：证据落到 F16，覆盖六大环节与积压量
UPDATE script_nodes
SET evidence_req='[{"type":"表单文书","name":"业务兜底处置记录单（F16）","note":"含六大环节、启动时间、纸质号段、责任人与兜底期间业务量"},{"type":"附件","name":"手工单据样例","note":"含号段与填写示例"}]'
WHERE id='S05-8' AND scenario_id='S05' AND deleted_at IS NULL;

-- 守卫 2：F16 追加后应为 9 个字段
SET @g_f16_len := (SELECT JSON_LENGTH(fields) FROM form_templates WHERE id='F16');
SET @guard_2 := IF(@g_f16_len=9, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 3：全库表单 JSON 必须全部合法
SET @g_bad_forms := (SELECT COUNT(*) FROM form_templates WHERE JSON_VALID(fields)=0);
SET @guard_3 := IF(@g_bad_forms=0, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 4：表单模板中不得再出现“2 小时”时限口径
SET @g_forms_2h := (SELECT COUNT(*) FROM form_templates WHERE fields LIKE '%2 小时%' OR fields LIKE '%2小时%');
SET @guard_4 := IF(@g_forms_2h=0, 1, (SELECT table_name FROM information_schema.tables));
