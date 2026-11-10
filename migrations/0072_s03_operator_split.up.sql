-- 0072 S03-8 拆分运营商协同节点 S03-8A
-- 依据：用户对 P1-15 的裁决——把运营商协同拆为独立节点、单独评分；
-- 0036 只在 S03-8 的 evidence_req 里补了运营商回执，未独立评分。
-- 设计：
--   * S03-8 保留院内侧处置（院内清洗策略与限速），target 改为网络安全管理员，
--     evidence_req 去掉运营商回执，避免与 S03-8A 重复；
--   * S03-8A 运营商协同：target 信息科负责人，依赖 S03-8（先内后外），
--     分 4 分（3–5 分区间中位），evidence_req 保留运营商工单/清洗回执；
--   * S03-13 结束节点依赖补入 S03-8A，确保对外协同闭环后才结束；
--   * 同步 seq、scenarios.nodes；S03 坐标原为 NULL（前端自动布局），新节点保持 NULL。
-- 可重复执行：新节点仅当不存在时插入；seq 位移仅在新节点插入前执行。

SET @ins := (SELECT COUNT(*) FROM script_nodes WHERE id = 'S03-8A');
UPDATE script_nodes SET seq = seq + 1 WHERE scenario_id = 'S03' AND seq >= 9 AND @ins = 0;

INSERT IGNORE INTO script_nodes
  (id, scenario_id, type, title, short_title, target, time, timeout, form, att, descr, branches,
   seq, guide, keywords, quality_rules, action, wrong, evidence_req, required, flow_type, score,
   receiver, deps, pos_x, pos_y, anchor_id, release_timing, parallel_desc)
VALUES
  ('S03-8A','S03','task','运营商协同：出口链路确认与清洗工单','运营商协同','信息科负责人','',15,'F9','',
   '院内清洗策略生效后，联系运营商确认出口链路、上游清洗能力与封禁范围；提交清洗/封禁工单，留存工单编号、处置时段与回执。先内后外：不得以运营商处置替代院内兜底与取证。',
   '[]',9,'先确认院内策略已生效再联系运营商；工单号、时段与回执必须落档。','运营商,工单,清洗,回执',
   '{"requiredKeywords":["运营商|上游","工单|清洗","回执|确认"],"bonusKeywords":["出口链路","封禁范围","处置时段","先内后外"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已处理","已完成","已隔离","正常","收到","处理完毕","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
   '隔离处置',NULL,
   '[{"type":"附件","name":"运营商工单/流量清洗回执","note":"工单编号、处置时段与回执截图"},{"type":"处置记录","name":"出口链路确认记录","note":"含运营商联系人、链路与清洗能力确认结果"}]',
   1,'correct',4,'信息科负责人','["S03-8"]',NULL,NULL,NULL,'before',NULL);

UPDATE script_nodes
   SET title = '启用院内流量清洗策略与限速',
       target = '网络安全管理员',
       receiver = '网络安全管理员',
       descr = '先抓包并留存清洗设备/防火墙策略日志与源 IP 清单，再下发院内清洗与限速策略；策略先观察后生效，优先保障收费、挂号，避免误封正常患者访问；院内设备处置留痕，运营商侧协同由 S03-8A 节点承接。',
       guide = '清洗策略先观察后生效，避免误封正常患者访问。',
       evidence_req = '[{"type":"操作日志","name":"网络设备/防火墙操作日志","note":""},{"type":"处置记录","name":"隔离处置记录","note":""}]'
 WHERE id = 'S03-8';

UPDATE script_nodes
   SET deps = '["S03-6","S03-7","S03-10","S03-11","S03-12","S03-14","S03-8A"]'
 WHERE id = 'S03-13';

UPDATE scenarios s
   SET s.nodes = (SELECT COUNT(*) FROM script_nodes n WHERE n.scenario_id = s.id),
       s.updated = '2026-11-07'
 WHERE s.id = 'S03';
