-- =====================================================================
-- 0100 角色补齐 + 联动修正（纯 DML）
-- ---------------------------------------------------------------------
-- 范围：roles 新增 3 个角色；script_nodes 改 3 个节点的 target/receiver/文案；
--       新增 4 个“等级升级研判”节点（克隆 S02-3A 结构，各 4 分）。
-- 约束：不改 Go 代码、前端、回执关键词节点；单事务；前后指纹；
--       范围外行指纹排除 seq 列（seq 位移仅动 seq，其它列必须不变）。
-- =====================================================================
SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
SET @p_roles := (SELECT COUNT(*) FROM roles);
SET @p_newids := (SELECT COUNT(*) FROM roles WHERE id IN ('office','legal','isp'));
SET @p_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @p_corr := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct');
SET @p_dist := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @p_38a := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-8A' AND target='信息科负责人' AND receiver='运营商' AND deps='["S03-8"]');
SET @p_311 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-11' AND target='院应急指挥部' AND receiver='院应急指挥部');
SET @p_312 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-12' AND target='医务处' AND receiver='院应急指挥部');
SET @p_313 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-13' AND JSON_CONTAINS(deps,'"S03-8A"'));
SET @p_tpl := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S02-3A' AND score=4 AND `required`=1 AND form='F3' AND deps='["S02-3"]');
SET @p_s01 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND scenario_id='S01' AND seq>=5);
SET @p_s03 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND scenario_id='S03' AND seq>=3);
SET @p_s05 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND scenario_id='S05' AND seq>=3);
SET @p_s07 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND scenario_id='S07' AND seq>=3);
SET @p_ms := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);
SET @p_body := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S01-5','S03-3','S05-3','S07-3'));
SET @guard_pre := IF(@p_roles=12 AND @p_newids=0 AND @p_nodes=353 AND @p_corr=133 AND @p_dist=220
  AND @p_38a=1 AND @p_311=1 AND @p_312=1 AND @p_313=1 AND @p_tpl=1 AND @p_body=4
  AND @p_s01=48 AND @p_s03=40 AND @p_s05=42 AND @p_s07=38
  AND @p_ms='S01:108,S02:106,S03:106,S04:99,S05:103,S06:106,S07:101,S08:104',
  1, (SELECT table_name FROM information_schema.tables));

-- ===== 写前指纹 =====
SET @fp_roles_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,name,COALESCE(short,''),COALESCE(icon,''),COALESCE(color,''),COALESCE(descr,''),COALESCE(nav,''),COALESCE(`external`,0),COALESCE(status,''),COALESCE(seq,0)) ORDER BY id SEPARATOR '\n')) FROM roles);
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S06-11','S06-12','S03-8A','S01-5A','S03-3A','S05-3A','S07-3A'));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);

-- ===== 1. 新增角色 =====
INSERT INTO roles (id,name,short,icon,color,descr,nav,`external`,status,seq) VALUES
('office','院办','院办','building','#f0883e','综合协调：统一对外口径与信息发布、组织全院通知分发、协调新闻发言人与舆情回应；重大口径经法务审核后报指挥部批准，不直接操作技术系统。','["workbench","collab","knowledge","opinion"]',0,'启用',13),
('legal','法务','法务','file','#b392f0','法律审核：对上报文书、对外口径、患者告知内容与个人信息保护影响评估出具法律意见并会签；不代替业务部门执行处置。','["workbench","collab","knowledge"]',0,'启用',14),
('isp','运营商','运营商','net','#4bb8a9','链路保障：接收清洗/封禁工单、确认出口链路与上游清洗能力、反馈处置时段与回执（外部单位，仅报送材料，由信息科负责人代填）。','["workbench","collab","knowledge"]',1,'启用',15);

-- ===== 2. S06-11：target 落到院办 =====
UPDATE script_nodes
SET target='院办',
    descr='由院办统筹统一对外口径与舆情回应，新闻发言人对外发布；信息科提供已核实技术事实，重大口径经法务审核并报院应急指挥部批准；结合市网信办指导意见，不将其作为审核前置。未经审批不得披露事件细节或患者信息。',
    guide='口径由法务审核、报指挥部批准后发布；重大公共传播口径征询市网信办指导；只发布已核实事实，不披露患者个人信息。'
WHERE id='S06-11' AND scenario_id='S06' AND deleted_at IS NULL AND target='院应急指挥部';

-- ===== 3. S06-12：receiver 补院办、法务（会签方） =====
UPDATE script_nodes
SET receiver='院办、法务',
    descr='医务处牵头患者告知与个人信息保护影响评估：按影响分级确定告知范围，采用短信、电话或公告告知泄露字段、可能影响与求助渠道；同步评估泄露数据类别、敏感个人信息范围、告知方式与监管报告义务，评估记录与告知记录一并留痕；法律口径由院办、法务会签（范围、方式、会签意见、时间），会签意见随告知记录归档。'
WHERE id='S06-12' AND scenario_id='S06' AND deleted_at IS NULL AND target='医务处' AND receiver='院应急指挥部';

-- ===== 4. S03-8A：target 落到运营商（外部，代填） =====
UPDATE script_nodes
SET target='运营商',
    descr='院内清洗策略生效后，由网络安全管理员提供出口链路与清洗策略信息；本节点由运营商承接（外部单位，信息科负责人代填）：确认出口链路、上游清洗能力与封禁范围，提交清洗/封禁工单，留存工单编号、处置时段与回执。先内后外：不得以运营商处置替代院内兜底与取证。',
    guide='先确认院内策略已生效再联系运营商；工单号、时段与回执必须落档；外部单位不登录系统，由信息科负责人代填并记录代填人。'
WHERE id='S03-8A' AND scenario_id='S03' AND deleted_at IS NULL AND target='信息科负责人';

-- ===== 5. seq 位移（仅改 seq 一列） =====
UPDATE script_nodes SET seq=seq+1 WHERE deleted_at IS NULL AND scenario_id='S01' AND seq>=5;
UPDATE script_nodes SET seq=seq+1 WHERE deleted_at IS NULL AND scenario_id='S03' AND seq>=3;
UPDATE script_nodes SET seq=seq+1 WHERE deleted_at IS NULL AND scenario_id='S05' AND seq>=3;
UPDATE script_nodes SET seq=seq+1 WHERE deleted_at IS NULL AND scenario_id='S07' AND seq>=3;

-- ===== 6. 新增 4 个等级升级研判节点（克隆 S02-3A） =====
INSERT INTO script_nodes
 (id,scenario_id,type,title,short_title,target,timeout,form,descr,seq,guide,keywords,quality_rules,action,evidence_req,`required`,flow_type,score,receiver,deps,pos_x,pos_y,release_timing)
VALUES
('S01-5A','S01','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人',15,'F3',
 '复核 S01-5 研判结论：出现勒索加密扩散至核心业务库、备份文件被加密、恢复点不可用、患者数据外泄或舆情扩散等情形，按院预案上一级标准重新定级为Ⅰ级，报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅱ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。',
 5,
 '先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。',
 '升级研判,维持,升级,新等级',
 '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["影响扩大","新等级","续报","指挥部批准","观察时限"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
 '研判分级',
 '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]',
 1,'correct',4,'院应急指挥部','["S01-5"]',641,408,'before'),
('S03-3A','S03','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人',15,'F3',
 '复核 S03-3 研判结论：出现门诊收费与挂号全面中断、核心业务受损、攻击扩散至内网或舆情扩散等情形，按院预案上一级标准重新定级为Ⅱ级，报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅲ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。',
 3,
 '先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。',
 '升级研判,维持,升级,新等级',
 '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["影响扩大","新等级","续报","指挥部批准","观察时限"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
 '研判分级',
 '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]',
 1,'correct',4,'院应急指挥部','["S03-3"]',NULL,NULL,'before'),
('S05-3A','S05','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人',15,'F3',
 '复核 S05-3 研判结论：出现核心存储不可用致临床业务全面中断、备份恢复失败、数据丢失或舆情扩散等情形，按院预案上一级标准重新定级为Ⅰ级，报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅱ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。',
 3,
 '先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。',
 '升级研判,维持,升级,新等级',
 '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["影响扩大","新等级","续报","指挥部批准","观察时限"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
 '研判分级',
 '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]',
 1,'correct',4,'院应急指挥部','["S05-3"]',NULL,NULL,'before'),
('S07-3A','S07','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人',15,'F3',
 '复核 S07-3 研判结论：出现边界设备失陷扩散至内网、横向移动、核心业务受影响或舆情扩散等情形，按院预案上一级标准重新定级为Ⅱ级，报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅲ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。',
 3,
 '先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。',
 '升级研判,维持,升级,新等级',
 '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["影响扩大","新等级","续报","指挥部批准","观察时限"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
 '研判分级',
 '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]',
 1,'correct',4,'院应急指挥部','["S07-3"]',NULL,NULL,'before');

-- ===== 后置断言 =====
SET @q_roles := (SELECT COUNT(*) FROM roles);
SET @q_newroles := (SELECT COUNT(*) FROM roles WHERE id IN ('office','legal','isp') AND status='启用' AND `external` IN (0,1) AND JSON_VALID(nav)=1);
SET @q_isp := (SELECT COUNT(*) FROM roles WHERE id='isp' AND `external`=1);
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @q_corr := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct');
SET @q_dist := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @q_new4 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S01-5A','S03-3A','S05-3A','S07-3A') AND score=4 AND `required`=1 AND form='F3' AND target='信息科负责人' AND receiver='院应急指挥部' AND flow_type='correct');
SET @q_deps := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND ((id='S01-5A' AND deps='["S01-5"]') OR (id='S03-3A' AND deps='["S03-3"]') OR (id='S05-3A' AND deps='["S05-3"]') OR (id='S07-3A' AND deps='["S07-3"]')));
SET @q_seq := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND ((id='S01-5A' AND seq=5) OR (id='S03-3A' AND seq=3) OR (id='S05-3A' AND seq=3) OR (id='S07-3A' AND seq=3)));
SET @q_311 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-11' AND target='院办');
SET @q_312 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S06-12' AND target='医务处' AND receiver='院办、法务');
SET @q_38a := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id='S03-8A' AND target='运营商' AND receiver='运营商' AND descr LIKE '%信息科负责人代填%');
SET @q_ms := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);
SET @guard_post := IF(@q_roles=15 AND @q_newroles=3 AND @q_isp=1 AND @q_nodes=357 AND @q_corr=137 AND @q_dist=220
  AND @q_new4=4 AND @q_deps=4 AND @q_seq=4 AND @q_311=1 AND @q_312=1 AND @q_38a=1
  AND @q_ms='S01:112,S02:106,S03:110,S04:99,S05:107,S06:106,S07:105,S08:104',
  1, (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写后） =====
SET @fp_roles_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,name,COALESCE(short,''),COALESCE(icon,''),COALESCE(color,''),COALESCE(descr,''),COALESCE(nav,''),COALESCE(`external`,0),COALESCE(status,''),COALESCE(seq,0)) ORDER BY id SEPARATOR '\n')) FROM roles WHERE id NOT IN ('office','legal','isp'));
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S06-11','S06-12','S03-8A','S01-5A','S03-3A','S05-3A','S07-3A'));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);
SET @guard_scope := IF(@fp_roles_before<=>@fp_roles_after AND @fp_nodes_before<=>@fp_nodes_after AND @fp_scen_before<=>@fp_scen_after,
  1, (SELECT table_name FROM information_schema.tables));
