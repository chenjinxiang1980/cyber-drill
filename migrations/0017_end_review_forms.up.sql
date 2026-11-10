-- =====================================================================
-- 0017_end_review_forms.up.sql
-- 批次 C：结束节点 F8→F18（7 个）；复盘节点 空/F9→F8（7 个）
-- ---------------------------------------------------------------------
-- 结束（7）：S02-13、S03-13、S04-13、S05-14、S06-13、S07-13、S08-14
--   绑定 F18《应急响应结束记录单》；evidence 统一为
--   “应急响应结束记录单 + 结束条件核对记录”；
--   同步修正术语：市委网信办→市网信办、市卫健局→市卫生健康局、
--   省指挥部报备→市卫生健康局/市网信办报告。
-- 复盘（7）：S02-16、S03-16、S04-15、S05-17、S06-15、S07-15、S08-16
--   绑定 F8《网络和数据安全事件反馈单》；
--   S04-15/S05-17/S08-16 标题去掉“应急结束”前缀；
--   S07-15/S08-16 补齐 evidence（复盘报告/整改清单）。
--
-- 不改分值、不改 target/deps/seq。
-- 幂等：固定终值。
-- =====================================================================
SET NAMES utf8mb4;
-- 不再硬编码 USE：迁移必须跟随连接库（DB_NAME）执行。

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

-- ---------------------------------------------------------------------
-- 结束节点：绑定 F18 + evidence 同步
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET form = 'F18',
       evidence_req = '[{"type":"表单文书","name":"应急响应结束记录单","note":"含结束条件核对结果、批准人与结束时间"},{"type":"处置记录","name":"结束条件核对记录","note":"含结束条件逐条结论"}]'
 WHERE id IN ('S02-13', 'S03-13', 'S04-13', 'S05-14', 'S07-13', 'S08-14');

UPDATE script_nodes
   SET form = 'F18',
       evidence_req = '[{"type":"表单文书","name":"应急响应结束记录单","note":"含五项结束条件核对结果、批准人与结束时间"},{"type":"处置记录","name":"结束条件核对记录","note":"含五项条件逐条结论"}]'
 WHERE id = 'S06-13';

-- 结束节点文案中的术语与结束口径
UPDATE script_nodes
   SET guide = '结束后按程序向市网信办报告。'
 WHERE id IN ('S03-13', 'S07-13');

UPDATE script_nodes
   SET guide = 'Ⅱ级响应结束由院应急指挥部宣布，报市卫生健康局、市网信办；结束前完成校验、补录与上报闭环。'
 WHERE id = 'S05-14';

UPDATE script_nodes
   SET descr = '满足结束条件后宣布Ⅱ级响应结束：外泄接口关闭、证据固定、患者告知完成、舆情可控、上报与报案闭环；填写《应急响应结束记录单》。',
       guide = 'Ⅱ级响应结束由院应急指挥部按预案终止条件评估后宣布，并报市卫生健康局、市网信办；不得以删帖作为结束条件。'
 WHERE id = 'S06-13';

UPDATE script_nodes
   SET guide = '由院应急指挥部宣布结束，报市卫生健康局、市网信办；结束记录归档。'
 WHERE id = 'S08-14';

-- ---------------------------------------------------------------------
-- 复盘节点：绑定 F8
-- ---------------------------------------------------------------------
UPDATE script_nodes SET form = 'F8'
 WHERE id IN ('S02-16', 'S03-16', 'S04-15', 'S05-17', 'S06-15', 'S07-15', 'S08-16');

-- 标题去除“应急结束”前缀，避免与结束节点混淆
UPDATE script_nodes SET title = '复盘评估：形成总结报告与整改清单' WHERE id = 'S04-15';
UPDATE script_nodes SET title = '复盘总结：故障根因、处置得失与整改' WHERE id = 'S05-17';
UPDATE script_nodes SET title = '复盘归档：响应终止与整改闭环' WHERE id = 'S08-16';

-- 复盘证据补齐（原为空）
UPDATE script_nodes
   SET evidence_req = '[{"type":"表单文书","name":"网络和数据安全事件反馈单","note":"含处置过程、根因分析、经验教训与整改计划"},{"type":"处置记录","name":"整改清单","note":"含责任部门与完成时限"}]'
 WHERE id IN ('S07-15', 'S08-16');

-- ---------------------------------------------------------------------
-- 断言
-- ---------------------------------------------------------------------
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-13','S03-13','S04-13','S05-14','S06-13','S07-13','S08-14')
      AND form = 'F18'
      AND flow_type = 'correct'
  ) = 7, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-16','S03-16','S04-15','S05-17','S06-15','S07-15','S08-16')
      AND form = 'F8'
      AND flow_type = 'correct'
  ) = 7, 1, 0);

-- 结束 evidence 全部指向《应急响应结束记录单》
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-13','S03-13','S04-13','S05-14','S06-13','S07-13','S08-14')
      AND evidence_req LIKE '%应急响应结束记录单%'
  ) = 7, 1, 0);

-- 复盘 evidence 至少包含复盘报告或反馈单
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-16','S03-16','S04-15','S05-17','S06-15','S07-15','S08-16')
      AND (evidence_req LIKE '%复盘%' OR evidence_req LIKE '%反馈单%')
  ) = 7, 1, 0);

-- 结束/复盘节点分值合计保持 46 / 42
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT SUM(score) FROM script_nodes
    WHERE id IN ('S02-13','S03-13','S04-13','S05-14','S06-13','S07-13','S08-14')) = 46
  AND
  (SELECT SUM(score) FROM script_nodes
    WHERE id IN ('S02-16','S03-16','S04-15','S05-17','S06-15','S07-15','S08-16')) = 42,
  1, 0);

-- 复盘标题已去除“应急结束”
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S04-15','S05-17','S08-16')
      AND title NOT LIKE '应急结束%'
  ) = 3, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
