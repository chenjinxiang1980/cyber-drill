-- =====================================================================
-- 0015_external_units_dual_report.up.sql
-- 批次 A：S02–S06 双报节点“下一节点接收人”改为对应外单位
-- ---------------------------------------------------------------------
-- 问题：双报节点的 receiver 指向信息科负责人自己，外单位收不到任务，
--       接收人判定按 30% 扣分（10 节点 / 66 分，约 19.8 分风险）。
-- 口径：与 S01-8、S07-7/8、S08-7/8 的正确样板对齐；
--       S06 保留医务处作为院内抄送。
--
-- 只改 receiver，不动 title/descr/guide/form/score/deps/seq。
-- 幂等：固定终值，重复执行结果一致。
-- 断言：10 个节点 receiver 全部为终值，且 F2 绑定与分值未变。
-- =====================================================================
SET NAMES utf8mb4;
-- 不再硬编码 USE：迁移必须跟随连接库（DB_NAME）执行。

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

UPDATE script_nodes SET receiver = '市卫生健康局'
 WHERE id IN ('S02-8', 'S03-6', 'S04-7', 'S05-6');

UPDATE script_nodes SET receiver = '市网信办'
 WHERE id IN ('S02-9', 'S03-7', 'S04-8', 'S05-7');

UPDATE script_nodes SET receiver = '市卫生健康局、医务处'
 WHERE id = 'S06-6';

UPDATE script_nodes SET receiver = '市网信办、医务处'
 WHERE id = 'S06-7';

-- ---------------------------------------------------------------------
-- 断言：10 个节点全部命中终值，且仍绑定 F2、分值不变
-- ---------------------------------------------------------------------
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE (id = 'S02-8' AND receiver = '市卫生健康局')
       OR (id = 'S03-6' AND receiver = '市卫生健康局')
       OR (id = 'S04-7' AND receiver = '市卫生健康局')
       OR (id = 'S05-6' AND receiver = '市卫生健康局')
       OR (id = 'S02-9' AND receiver = '市网信办')
       OR (id = 'S03-7' AND receiver = '市网信办')
       OR (id = 'S04-8' AND receiver = '市网信办')
       OR (id = 'S05-7' AND receiver = '市网信办')
       OR (id = 'S06-6' AND receiver = '市卫生健康局、医务处')
       OR (id = 'S06-7' AND receiver = '市网信办、医务处')
  ) = 10, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-8','S02-9','S03-6','S03-7','S04-7','S04-8','S05-6','S05-7','S06-6','S06-7')
      AND form = 'F2'
      AND flow_type = 'correct'
  ) = 10, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT SUM(score) FROM script_nodes
    WHERE id IN ('S02-8','S02-9','S03-6','S03-7','S04-7','S04-8','S05-6','S05-7','S06-6','S06-7')
  ) = 66, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
