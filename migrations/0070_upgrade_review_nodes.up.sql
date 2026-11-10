-- 0070 新增「等级升级研判」独立节点（S02/S06/S08）
-- 依据：用户对 P1-13 的裁决——新增独立节点，不只在研判节点写文字；
-- 覆盖 0039 曾写「按升级流程重新研判」的 S02-3、S06-3、S08-3 所在场景。
-- 设计：
--   * 类型 task、责任角色 信息科负责人、接收人 院应急指挥部、表单 F3（预警研判单）；
--   * 分 4 分：低于主研判 8 分，高于普通干扰项，取 3–5 分区间中位；
--   * 依赖：新节点依赖原研判节点，原「向指挥部报告」节点改为依赖新节点，形成
--     「研判 → 升级研判 → 报告」单链，不产生环；
--   * 原研判节点 descr 移除升级续报句，避免与新节点重复表述；
--   * 新节点写满 quality_rules（必含/加分、长度、扣分比例），档位与同类研判节点一致。
-- 可重复执行：新节点仅当不存在时插入；seq 位移仅在新节点插入前执行；
-- scenarios.nodes 按实际节点数回写。

-- ============ S02 ============
SET @ins := (SELECT COUNT(*) FROM script_nodes WHERE id = 'S02-3A');
UPDATE script_nodes SET seq = seq + 1 WHERE scenario_id = 'S02' AND seq >= 3 AND @ins = 0;
INSERT IGNORE INTO script_nodes
  (id, scenario_id, type, title, short_title, target, time, timeout, form, att, descr, branches,
   seq, guide, keywords, quality_rules, action, wrong, evidence_req, required, flow_type, score,
   receiver, deps, pos_x, pos_y, anchor_id, release_timing, parallel_desc)
VALUES
  ('S02-3A','S02','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人','',15,'F3','',
   '复核 S02-3 研判结论：出现核心业务受损、电子病历数据外泄、横向传播不可控或舆情扩散等情形，按院预案上一级标准重新定级，报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅲ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。',
   '[]',3,'先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。','升级研判,维持,升级,新等级',
   '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["影响扩大","新等级","续报","指挥部批准","观察时限"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
   '研判分级',NULL,
   '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、触发条件、依据与报批时间"},{"type":"上报材料","name":"升级续报记录","note":"升级时含新等级、补报对象与时间；维持时注明观察时限"}]',
   1,'correct',4,'院应急指挥部','["S02-3"]',654,912,NULL,'before',NULL);
UPDATE script_nodes
   SET descr = '汇总失陷账号、登录来源、横向痕迹与数据外泄风险，对照院预案分级标准研判为较大（Ⅲ级），明确是否已造成电子病历数据外泄。'
 WHERE id = 'S02-3';
UPDATE script_nodes SET deps = '["S02-3A"]' WHERE id = 'S02-4';

-- ============ S06 ============
SET @ins := (SELECT COUNT(*) FROM script_nodes WHERE id = 'S06-3A');
UPDATE script_nodes SET seq = seq + 1 WHERE scenario_id = 'S06' AND seq >= 3 AND @ins = 0;
INSERT IGNORE INTO script_nodes
  (id, scenario_id, type, title, short_title, target, time, timeout, form, att, descr, branches,
   seq, guide, keywords, quality_rules, action, wrong, evidence_req, required, flow_type, score,
   receiver, deps, pos_x, pos_y, anchor_id, release_timing, parallel_desc)
VALUES
  ('S06-3A','S06','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人','',15,'F3','',
   '复核 S06-3 定级：泄露数据扩展到全院或大规模敏感个人信息、出现数据交易/敲诈、舆情全国扩散、核心业务停摆等情形，按院预案上一级标准重新定级为Ⅰ级，报院应急指挥部批准后补报并按新等级续报；未达升级条件时明确维持Ⅱ级及观察时限。判定须引用数据量级、敏感字段、扩散速度与舆情热度。',
   '[]',3,'先给维持/升级结论，再列触发条件与依据；升级须报批并同步监管部门续报。','升级研判,维持,升级,新等级',
   '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["数据量级","敏感字段","舆情热度","新等级","续报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
   '研判分级',NULL,
   '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、触发条件、依据与报批时间"},{"type":"上报材料","name":"升级续报记录","note":"升级时含新等级、补报对象与时间；维持时注明观察时限"}]',
   1,'correct',4,'院应急指挥部','["S06-3"]',NULL,NULL,NULL,'before',NULL);
UPDATE script_nodes
   SET descr = '依据数据量级、敏感字段（身份证号、病历号、诊断）、扩散速度与舆情热度，确认重大（Ⅱ级）事件并初判泄露来源；结论写入定级依据。',
       guide = '定级不得等待抓到泄露源头；先按已知量级与舆情热度定级，影响扩大时进入升级研判。'
 WHERE id = 'S06-3';
UPDATE script_nodes SET deps = '["S06-3A"]' WHERE id = 'S06-4';

-- ============ S08 ============
SET @ins := (SELECT COUNT(*) FROM script_nodes WHERE id = 'S08-3A');
UPDATE script_nodes SET seq = seq + 1 WHERE scenario_id = 'S08' AND seq >= 3 AND @ins = 0;
INSERT IGNORE INTO script_nodes
  (id, scenario_id, type, title, short_title, target, time, timeout, form, att, descr, branches,
   seq, guide, keywords, quality_rules, action, wrong, evidence_req, required, flow_type, score,
   receiver, deps, pos_x, pos_y, anchor_id, release_timing, parallel_desc)
VALUES
  ('S08-3A','S08','task','等级升级研判：影响扩大时按新等级续报','升级研判','信息科负责人','',15,'F3','',
   '复核 S08-3 定级：恶意代码波及业务网/HIS、感染终端持续增加、出现勒索或横向移动、医疗设备受影响等情形，按院预案上一级标准重新定级，报院应急指挥部批准后补报并按新等级续报；未达升级条件时明确维持Ⅳ级及观察时限。判定须引用感染终端数、传播方式与业务网影响。',
   '[]',3,'先给维持/升级结论，再列触发条件与依据；升级须报批，不在原节点内静默改级。','升级研判,维持,升级,新等级',
   '{"requiredKeywords":["升级研判|等级升级|升级结论","维持|升级","依据"],"bonusKeywords":["感染终端数","波及业务网","扩散趋势","新等级","续报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
   '研判分级',NULL,
   '[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、触发条件、依据与报批时间"},{"type":"上报材料","name":"升级续报记录","note":"升级时含新等级、补报对象与时间；维持时注明观察时限"}]',
   1,'correct',4,'院应急指挥部','["S08-3"]',NULL,NULL,NULL,'before',NULL);
UPDATE script_nodes
   SET descr = '汇总感染终端数、传播方式与业务网影响，对照院预案分级标准研判为一般（Ⅳ级）；定级依据须明确是否波及业务网、感染终端数与是否需全院预警。'
 WHERE id = 'S08-3';
UPDATE script_nodes SET deps = '["S08-3A"]' WHERE id = 'S08-4';

-- ============ 场景节点数回写 ============
UPDATE scenarios s
   SET s.nodes = (SELECT COUNT(*) FROM script_nodes n WHERE n.scenario_id = s.id),
       s.updated = '2026-11-07'
 WHERE s.id IN ('S02','S06','S08');
