-- =====================================================================
-- 0093 Ⅳ级“评估后可不报案”四层放开 + S02 结束链去死锁
-- ---------------------------------------------------------------------
-- A. S04/S08 的报案必要性评估：
--    * 关键词只要求“评估依据/处置决定/复核/不报案理由”，不再要求“受理回执/受理编号”；
--    * 证据只要求 F19 评估单与评估记录，不要求 F5/回执；
--    * 报案回执任务 S04-14/S08-15 改为 0 分 0 必做，仅在选择报案时填写；
--    * 结束节点本来不依赖回执节点，本次加守卫确认。
-- B. S02：S02-15（恢复验证）去掉对 S02-14（公安回执）的依赖，
--    保留 S02-13 → S02-15，使“回执不到”不再阻塞正确结束路径。
--
-- 纯 DML，可重复执行；守卫失败触发 1242 并回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- 守卫 1：S02-15 当前必须含 S02-14
SET @g1 := (SELECT COUNT(*) FROM script_nodes
  WHERE id='S02-15' AND scenario_id='S02' AND deleted_at IS NULL
    AND JSON_CONTAINS(deps,'"S02-14"'));
SET @guard_1 := IF(@g1=1, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 2：S04-12/S08-12 当前仍要求回执类关键词
SET @g2 := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S04-12','S08-12')
    AND (keywords LIKE '%受理回执%' OR keywords LIKE '%受理编号%'));
SET @guard_2 := IF(@g2=2, 1, (SELECT table_name FROM information_schema.tables));

-- ===== S02：去死锁 =====
UPDATE script_nodes
SET deps='["S02-10","S02-11"]',
    descr='验证核心业务恢复与数据完整性：HIS/EMR 关键功能可用、备份数据可读、无二次加密或残留横向痕迹。恢复验证以业务与数据事实为准，不以公安回执为前置；回执由信息科负责人代填，不作为本节点或演练结束条件。'
WHERE id='S02-15' AND scenario_id='S02' AND deleted_at IS NULL;

UPDATE script_nodes
SET guide='结束条件：S02-8/S02-9 双报完成、S02-15 恢复验证完成即可；S02-14 为信息科负责人代填项，公安回执不作为结束前置。'
WHERE id='S02-13' AND scenario_id='S02' AND deleted_at IS NULL;

-- ===== S04：评估后可选择不报案 =====
UPDATE script_nodes
SET descr='对是否报案进行必要性评估：对照《国家网络安全事件报告管理办法》与院内标准，判断是否达到报案条件；无论决定报案或不报案，均须写明评估依据、参与人、处置决定与复核时间。选择不报案不扣分。',
    guide='使用 F19：不报案理由必须具体（如未达到立案标准、已由卫健部门协调、证据不足以支撑刑事立案），并安排复核；不得以“等外单位回复”为由跳过评估。',
    keywords='报案必要性,评估依据,处置决定,复核,不报案理由',
    evidence_req='[{"type":"表单文书","name":"报案必要性评估单（F19）","note":"含评估依据、处置决定、不报案理由与复核时间"},{"type":"处置记录","name":"评估记录","note":"含评估人、时间、参与人与结论"}]'
WHERE id='S04-12' AND scenario_id='S04' AND deleted_at IS NULL;

UPDATE script_nodes
SET score=0,
    required=0,
    descr='仅在 S04-12 评估决定报案时填写：记录报案受理编号、回执形式与补证要求；选择不报案时不执行本节点，不扣分。',
    guide='代填项（信息科负责人）：由信息科负责人代市网安大队填写 F15；未取得书面回执时选“无回执”，记录口头回执时间与对方姓名；附报案材料或沟通记录；本节点 0 分，不计入医院扣分。'
WHERE id='S04-14' AND scenario_id='S04' AND deleted_at IS NULL;

-- ===== S08：评估后可选择不报案 =====
UPDATE script_nodes
SET descr='对是否报案进行必要性评估：结合办公网受影响范围、是否波及业务网、证据完整性判断是否达到报案条件；无论决定报案或不报案，均须写明评估依据、参与人、处置决定与复核时间。选择不报案不扣分。',
    guide='使用 F19：不报案理由必须具体（如业务网未受影响、未达到立案标准、证据不足），并安排复核；不得因“业务网没事”直接跳过评估。',
    keywords='报案必要性,评估依据,处置决定,复核,不报案理由',
    evidence_req='[{"type":"表单文书","name":"报案必要性评估单（F19）","note":"含评估依据、处置决定、不报案理由与复核时间"},{"type":"处置记录","name":"评估记录","note":"含评估人、时间、参与人与结论"}]'
WHERE id='S08-12' AND scenario_id='S08' AND deleted_at IS NULL;

UPDATE script_nodes
SET score=0,
    required=0,
    descr='仅在 S08-12 评估决定报案时填写：记录报案受理编号、回执形式与补证要求；选择不报案时不执行本节点，不扣分。',
    guide='代填项（信息科负责人）：由信息科负责人代市网安大队填写 F15；未取得书面回执时选“无回执”，记录口头回执时间与对方姓名；附报案材料或沟通记录；本节点 0 分，不计入医院扣分。'
WHERE id='S08-15' AND scenario_id='S08' AND deleted_at IS NULL;

-- 守卫 3：S02-15 不再依赖 S02-14
SET @g3 := (SELECT COUNT(*) FROM script_nodes
  WHERE id='S02-15' AND scenario_id='S02' AND deleted_at IS NULL
    AND JSON_CONTAINS(deps,'"S02-14"'));
SET @guard_3 := IF(@g3=0, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 4：S04-12/S08-12 不再要求回执类关键词
SET @g4 := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S04-12','S08-12')
    AND (keywords LIKE '%受理回执%' OR keywords LIKE '%受理编号%'));
SET @guard_4 := IF(@g4=0, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 5：S04-14/S08-15 必须 0 分 0 必做
SET @g5 := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S04-14','S08-15') AND score=0 AND required=0);
SET @guard_5 := IF(@g5=2, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 6：结束节点不得依赖 S04-14/S08-15
SET @g6 := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S04-13','S08-14')
    AND (JSON_CONTAINS(deps,'"S04-14"') OR JSON_CONTAINS(deps,'"S08-15"')));
SET @guard_6 := IF(@g6=0, 1, (SELECT table_name FROM information_schema.tables));
