-- 0030 S01 双报拆分：市卫健委 / 市网信办各成节点（P1-03）
-- 保留 S01-8 作为报卫健节点，新增 S01-8B 报网信办，避免历史 drill_steps 关联失效。
-- 纯 DML，固定终值，可重复执行。

UPDATE script_nodes
   SET title       = '向市卫生健康局报送事件（1 小时内）',
       short_title = '报市卫健委',
       receiver    = '市卫生健康局',
       descr       = '按「先院内上报、后对外报案、双报可并行」口径，向市卫生健康局报送事件初报。与 S01-8B（市网信办）并行，互不阻塞；报送回执不作为后续节点或演练结束的前置条件。',
       guide       = '进入上报页选择「市卫生健康局」，填写事件类型、等级、影响范围、已采取措施和时限依据，提交后进入公安报案与处置并行。',
       keywords    = '市卫生健康局,市卫健委,上报,初报',
       deps        = '["S01-7"]',
       seq         = 7
 WHERE id = 'S01-8';

INSERT INTO script_nodes
  (id, scenario_id, type, title, short_title, target, time, timeout, form, att, descr, branches, seq,
   guide, keywords, action, wrong, evidence_req, required, flow_type, score, receiver, deps,
   pos_x, pos_y, anchor_id, release_timing, parallel_desc)
SELECT
  'S01-8B', scenario_id, type, '向市网信办报送事件（1 小时内）', '报市网信办', target, time, timeout, form, att,
  '按「先院内上报、后对外报案、双报可并行」口径，向市网信办报送事件初报（网信办负责指导，不承担审核职责）。与 S01-8（市卫生健康局）并行，互不阻塞。',
  branches, 8,
  '进入上报页选择「市网信办」，报送事件基本情况、影响范围与处置进展；网信办的指导意见用于处置参考，不构成审批。',
  '市网信办,网信办,上报,初报', action, wrong, evidence_req, required, flow_type, score, '市网信办', '["S01-7"]',
  pos_x + 40, pos_y, NULL, release_timing, parallel_desc
  FROM script_nodes WHERE id = 'S01-8'
ON DUPLICATE KEY UPDATE
  title = VALUES(title), short_title = VALUES(short_title), receiver = VALUES(receiver),
  descr = VALUES(descr), guide = VALUES(guide), keywords = VALUES(keywords),
  deps = VALUES(deps), seq = VALUES(seq);

-- 后续节点顺序整体后移一位；按 ID 固定赋值，重复执行不漂移
UPDATE script_nodes SET seq = 9  WHERE id = 'S01-9';
UPDATE script_nodes SET seq = 10 WHERE id = 'S01-10';
UPDATE script_nodes SET seq = 11 WHERE id = 'S01-11';
UPDATE script_nodes SET seq = 12 WHERE id = 'S01-12';
UPDATE script_nodes SET seq = 13 WHERE id = 'S01-13';
UPDATE script_nodes SET seq = 14 WHERE id = 'S01-14';
UPDATE script_nodes SET seq = 15 WHERE id = 'S01-15';
UPDATE script_nodes SET seq = 16 WHERE id = 'S01-16';
UPDATE script_nodes SET seq = 17 WHERE id = 'S01-17';
UPDATE script_nodes SET seq = 18 WHERE id = 'S01-18';

-- 主管部门指导节点分别挂到对应上报节点
UPDATE script_nodes SET deps = '["S01-8"]'  WHERE id = 'S01-12';
UPDATE script_nodes SET deps = '["S01-8B"]' WHERE id = 'S01-13';

-- 场景节点数改为全部节点数（含干扰项/终止项），与编辑器节点总数一致（P1-3）
UPDATE scenarios s
   SET nodes = (SELECT COUNT(*) FROM script_nodes n WHERE n.scenario_id = s.id);
