-- =====================================================================
-- 0099 业务数据第二批单点修正（纯 DML）
-- ---------------------------------------------------------------------
-- 范围：仅 script_nodes / form_templates / settings 的文案与结构；
--       不改 Go 代码、前端、roles 新增角色，不触碰回执关键词相关节点。
-- 写法：目标行精确匹配 + 单事务 + 前后指纹 + 后置断言；
--       任一守卫失败触发 1242，事务整体回滚。
-- =====================================================================
SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言：目标行必须是预检时的版本 =====
SET @p1 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-8' AND descr LIKE '%S01-8B%' AND evidence_req LIKE '%两个接收单位各一份%');
SET @p2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S05-12' AND descr LIKE '%D-01%');
SET @p3 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-13' AND guide LIKE '%结束后按程序向市网信办报告%');
SET @p4 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S02-14' AND descr LIKE '%勒索病毒%');
SET @p5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-17' AND evidence_req LIKE '%四项结束条件%');
SET @p6 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-13' AND evidence_req LIKE '%五项%');
SET @p7 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S06-11','S06-12') AND deps='["S06-6","S06-7"]');
SET @p8 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S03-9','S05-8') AND target='信息科负责人');
SET @p9 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-2' AND descr NOT LIKE '%万条%');
SET @p10 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-9' AND JSON_TYPE(evidence_req)='ARRAY' AND JSON_TYPE(JSON_EXTRACT(evidence_req,'$[0]'))='STRING' AND JSON_LENGTH(evidence_req)=4);
SET @p11 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S04-13' AND NOT JSON_CONTAINS(deps,'"S04-12"'));
SET @p12 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S08-14' AND NOT JSON_CONTAINS(deps,'"S08-12"'));
SET @p13 := (SELECT COUNT(*) FROM form_templates WHERE id IN ('F2','F3','F5','F8','F18') AND JSON_VALID(fields)=1);
SET @p14 := (SELECT COUNT(*) FROM form_templates WHERE id='F5' AND JSON_LENGTH(fields)=14);
SET @p15 := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v0.8.1');
SET @p16 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @p17 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @guard_pre := IF(@p1=1 AND @p2=1 AND @p3=1 AND @p4=1 AND @p5=1 AND @p6=1 AND @p7=2 AND @p8=2 AND @p9=1 AND @p10=1 AND @p11=1 AND @p12=1 AND @p13=5 AND @p14=1 AND @p15=1 AND @p16=353 AND @p17=220, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写前） =====
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,flow_type,title,COALESCE(target,''),COALESCE(descr,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(evidence_req,''),COALESCE(quality_rules,''),COALESCE(deps,''),COALESCE(score,0),COALESCE(required,1),COALESCE(timeout,0),COALESCE(action,''),COALESCE(`wrong`,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND id NOT IN ('S01-8','S01-9','S01-17','S02-14','S03-9','S03-13','S04-13','S05-8','S05-12','S06-2','S06-11','S06-12','S06-13','S08-14'));
SET @fp_forms_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,code,fields) ORDER BY id SEPARATOR '\n'))
  FROM form_templates WHERE id NOT IN ('F2','F3','F5','F8','F18'));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_roles_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(external,0)) ORDER BY id SEPARATOR '\n')) FROM roles);
SET @fp_settings_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,Coalesce(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');
SET @ms_before := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);

-- ===== 1. S01-8：引用 S01-19；证据改为单接收单位 =====
UPDATE script_nodes
SET descr='按「先院内上报、后对外报案、双报可并行」口径，向市卫生健康局报送事件初报。与 S01-19（市网信办）并行，互不阻塞；报送回执不作为后续节点或演练结束的前置条件。',
    evidence_req='[{"type":"上报材料","name":"上报回执","note":"仅市卫生健康局一份，含时间与接收人"},{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象与时限依据"}]'
WHERE id='S01-8' AND scenario_id='S01' AND deleted_at IS NULL;

-- ===== 2. S05-12：删除内部决策编号 D-01 =====
UPDATE script_nodes
SET descr='勒索加密或人为破坏迹象通常已达立案标准；确认后即向市网安大队报案，提交事件经过、影响范围与已保全证据，不等待内部恢复完成，回执仅作归档。'
WHERE id='S05-12' AND scenario_id='S05' AND deleted_at IS NULL;

-- ===== 3. S03-13：删除与 S03-7 重复的“结束后报告”指引 =====
UPDATE script_nodes
SET guide='确认双报已完成、证据已归档后再宣布结束；不得因等待外部回执延迟结束。'
WHERE id='S03-13' AND scenario_id='S03' AND deleted_at IS NULL;

-- ===== 4. S02-14：按 S02 实际场景收敛（账号被盗/病历数据非法访问） =====
UPDATE script_nodes
SET descr='市网安大队受理账号被盗与病历数据非法访问报案，出具受理回执：受理编号、是否立案、承办警官、取证边界与补证清单；院方按清单配合补证，非涉案业务可恢复。'
WHERE id='S02-14' AND scenario_id='S02' AND deleted_at IS NULL;

-- ===== 5. S01-17/S06-13：结束条件数量统一为三项 =====
UPDATE script_nodes
SET evidence_req='[{"type":"表单文书","name":"应急响应结束记录单","note":"含三项结束条件核对结果与批准人"},{"type":"处置记录","name":"整改初稿","note":"含整改项、责任人与时限"}]'
WHERE id='S01-17' AND scenario_id='S01' AND deleted_at IS NULL;

UPDATE script_nodes
SET evidence_req='[{"type":"表单文书","name":"应急响应结束记录单","note":"含三项结束条件核对结果、批准人与结束时间"},{"type":"处置记录","name":"结束条件核对记录","note":"含三项条件逐条结论"}]'
WHERE id='S06-13' AND scenario_id='S06' AND deleted_at IS NULL;

-- ===== 6. S06-11/S06-12：去掉外部报告前置，改为敏感数据处置前置 =====
UPDATE script_nodes
SET deps='["S06-8","S06-9"]'
WHERE id IN ('S06-11','S06-12') AND scenario_id='S06' AND deleted_at IS NULL;

-- ===== 7. S03-9/S05-8：target 落到实际承接方 =====
UPDATE script_nodes
SET target='收费窗口',
    descr='由收费处牵头启用手工收费与纸质票据，财务处负责票据与对账口径，信息科负责系统恢复与补录稽核，收费窗口负责执行与患者解释，避免患者聚集。'
WHERE id='S03-9' AND scenario_id='S03' AND deleted_at IS NULL;

UPDATE script_nodes
SET target='医务处',
    descr='信息科负责灾备切换，医务处牵头业务兜底并组织挂号、收费、发药、检验、住院六大环节手工流程；财务处、收费处配合完成手工收费与补录，核心业务恢复前不得解除兜底。'
WHERE id='S05-8' AND scenario_id='S05' AND deleted_at IS NULL;

-- ===== 8. S06-2：写明具体泄露条数，支撑“重大”结论 =====
UPDATE script_nodes
SET descr='背景：外部论坛已出现售卖帖，初判泄露约 128 万条就诊记录（含身份证号 46 万条、病历号 21 万条），涉及 2025-09 至 2026-10 的门诊与住院数据，属重要数据/敏感个人信息并已形成公开售卖舆情；定级按「重要数据被窃取＋公开传播范围」认定重大（Ⅱ级），不单以条数计。本节点仅比对脱敏字段（字段名、类型、编码规则），核定泄露条数、涉及字段、时间范围与是否含身份证号、病历号等敏感标识；不得复制、转存或外发原始样本。'
WHERE id='S06-2' AND scenario_id='S06' AND deleted_at IS NULL;

-- ===== 9. S01-9：证据数组由字符串改为对象，字段不丢 =====
UPDATE script_nodes
SET evidence_req='[{"type":"处置记录","name":"数据库加密前后文件哈希清单","note":"含文件路径、算法与比对结果"},{"type":"日志样本","name":"数据库错误日志与勒索信样本","note":"含时间戳与原始样本哈希"},{"type":"表单文书","name":"核心库备份校验报告","note":"含备份时间点、校验方式与可恢复结论"},{"type":"附件","name":"主机隔离与现场取证照片","note":"含隔离时间、设备编号与拍摄人"}]'
WHERE id='S01-9' AND scenario_id='S01' AND deleted_at IS NULL;

-- ===== 10. S04-13/S08-14：报案评估进入结束前置 =====
UPDATE script_nodes
SET deps=JSON_ARRAY_APPEND(deps,'$','S04-12')
WHERE id='S04-13' AND scenario_id='S04' AND deleted_at IS NULL AND NOT JSON_CONTAINS(deps,'"S04-12"');

UPDATE script_nodes
SET deps=JSON_ARRAY_APPEND(deps,'$','S08-12')
WHERE id='S08-14' AND scenario_id='S08' AND deleted_at IS NULL AND NOT JSON_CONTAINS(deps,'"S08-12"');

-- ===== 11. 表单等级口径对照说明 =====
UPDATE form_templates
SET fields=JSON_SET(fields,'$[2].hint','按预案分为特别重大/重大/较大/一般，与研判结论一致。对照：特别重大=Ⅰ级、重大=Ⅱ级、较大=Ⅲ级、一般=Ⅳ级；与 F8「定级结论」、F18「响应级别」使用同一枚举。')
WHERE id='F2' AND JSON_VALID(fields)=1;

UPDATE form_templates
SET fields=JSON_SET(fields,
      '$[4].hint','按预案条款与影响评估给出建议事件等级，与上报表保持一致。对照：特别重大=Ⅰ级、重大=Ⅱ级、较大=Ⅲ级、一般=Ⅳ级。',
      '$[14].hint','本节点确认后的最终事件等级，须与上报表、启动申请保持一致。对照：特别重大=Ⅰ级、重大=Ⅱ级、较大=Ⅲ级、一般=Ⅳ级；与 F2/F8/F18 同一枚举。')
WHERE id='F3' AND JSON_VALID(fields)=1;

UPDATE form_templates
SET fields=JSON_SET(fields,'$[3].hint','按预案确认的事件等级。对照：Ⅰ级=特别重大、Ⅱ级=重大、Ⅲ级=较大、Ⅳ级=一般；与 F2/F3 的等级名称一一对应。')
WHERE id='F8' AND JSON_VALID(fields)=1;

UPDATE form_templates
SET fields=JSON_SET(fields,
      '$[0].hint','本次结束的应急响应级别，须与启动审批一致。对照：Ⅰ级=特别重大、Ⅱ级=重大、Ⅲ级=较大、Ⅳ级=一般；与 F2/F3/F8 同一枚举。',
      '$[1].hint','按本场景节点说明逐条核对结束条件（S01/S06 为三项，S07/S08 按网络/终端口径），并引用预案条款；逐项结论请在「结束条件核对」中勾选。')
WHERE id='F18' AND JSON_VALID(fields)=1;

-- ===== 12. F5：补“涉嫌罪名”“个人信息类型/数量” =====
UPDATE form_templates
SET fields=JSON_ARRAY_APPEND(fields,'$',
      JSON_OBJECT('label','涉嫌罪名','type','text','required',FALSE,'hint','如破坏计算机信息系统罪、非法获取计算机信息系统数据罪；不确定填「待定性」。'),
      '$',
      JSON_OBJECT('label','个人信息类型/数量','type','textarea','required',FALSE,'hint','写明姓名、身份证号、病历号等类型及各自条数（演练虚拟数据）。'))
WHERE id='F5' AND JSON_VALID(fields)=1 AND JSON_SEARCH(fields,'one','涉嫌罪名',NULL,'$[*].label') IS NULL;

-- ===== 13. 品牌版本号 =====
UPDATE settings SET value='v0.8.4' WHERE `key`='brand_version' AND value='v0.8.1';

-- ===== 后置断言 =====
SET @q1 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-8' AND descr LIKE '%S01-19%' AND descr NOT LIKE '%S01-8B%' AND evidence_req LIKE '%仅市卫生健康局一份%' AND evidence_req NOT LIKE '%两个接收单位%');
SET @q2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S05-12' AND descr NOT LIKE '%D-01%');
SET @q3 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-13' AND guide NOT LIKE '%结束后按程序向市网信办报告%');
SET @q4 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S02-14' AND descr LIKE '%账号被盗%' AND descr NOT LIKE '%勒索病毒%');
SET @q5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-17' AND evidence_req LIKE '%三项结束条件%');
SET @q6 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-13' AND evidence_req LIKE '%三项%' AND evidence_req NOT LIKE '%五项%');
SET @q7 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S06-11','S06-12') AND deps='["S06-8","S06-9"]');
SET @q8 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-9' AND target='收费窗口');
SET @q9 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S05-8' AND target='医务处');
SET @q10 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-2' AND descr LIKE '%128 万条%');
SET @q11 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S01-9' AND JSON_TYPE(evidence_req)='ARRAY' AND JSON_TYPE(JSON_EXTRACT(evidence_req,'$[0]'))='OBJECT' AND JSON_LENGTH(evidence_req)=4);
SET @q12 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S04-13' AND JSON_CONTAINS(deps,'"S04-12"'));
SET @q13 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S08-14' AND JSON_CONTAINS(deps,'"S08-12"'));
SET @q14 := (SELECT COUNT(*) FROM form_templates WHERE id='F5' AND JSON_LENGTH(fields)=16 AND JSON_SEARCH(fields,'one','涉嫌罪名',NULL,'$[*].label') IS NOT NULL AND JSON_SEARCH(fields,'one','个人信息类型/数量',NULL,'$[*].label') IS NOT NULL);
SET @q15 := (SELECT COUNT(*) FROM form_templates WHERE id IN ('F2','F3','F8','F18') AND fields LIKE '%Ⅰ级%' AND fields LIKE '%Ⅳ级%');
SET @q16 := (SELECT COUNT(*) FROM settings WHERE `key`='brand_version' AND value='v0.8.4');

SET @guard_post := IF(@q1=1 AND @q2=1 AND @q3=1 AND @q4=1 AND @q5=1 AND @q6=1 AND @q7=2 AND @q8=1 AND @q9=1 AND @q10=1 AND @q11=1 AND @q12=1 AND @q13=1 AND @q14=1 AND @q15=4 AND @q16=1, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写后）与总量守卫 =====
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,flow_type,title,COALESCE(target,''),COALESCE(descr,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(evidence_req,''),COALESCE(quality_rules,''),COALESCE(deps,''),COALESCE(score,0),COALESCE(required,1),COALESCE(timeout,0),COALESCE(action,''),COALESCE(`wrong`,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL AND id NOT IN ('S01-8','S01-9','S01-17','S02-14','S03-9','S03-13','S04-13','S05-8','S05-12','S06-2','S06-11','S06-12','S06-13','S08-14'));
SET @fp_forms_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,code,fields) ORDER BY id SEPARATOR '\n'))
  FROM form_templates WHERE id NOT IN ('F2','F3','F5','F8','F18'));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @fp_roles_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(external,0)) ORDER BY id SEPARATOR '\n')) FROM roles);
SET @fp_settings_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',`key`,Coalesce(value,'')) ORDER BY `key` SEPARATOR '\n')) FROM settings WHERE `key`<>'brand_version');
SET @ms_after := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);

SET @guard_scope := IF(@fp_nodes_after<=>@fp_nodes_before AND @fp_forms_after<=>@fp_forms_before AND @fp_scen_after<=>@fp_scen_before AND @fp_roles_after<=>@fp_roles_before AND @fp_settings_after<=>@fp_settings_before, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_counts := IF((SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL)=353 AND (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor')=220 AND (SELECT COUNT(*) FROM form_templates)=19 AND (SELECT COUNT(*) FROM scenarios)=8, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_maxscore := IF(@ms_after<=>@ms_before, 1, (SELECT table_name FROM information_schema.tables));
