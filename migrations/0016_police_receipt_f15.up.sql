-- =====================================================================
-- 0016_police_receipt_f15.up.sql
-- 批次 B：公安受理回执节点改绑 F15，标题/口径/证据同步
-- ---------------------------------------------------------------------
-- 范围（7）：S02-14、S03-15、S04-14、S05-16、S06-14、S07-14、S08-15
-- 问题：5 处 form 为空、2 处绑 F9（受理回执），节点无法记录受理编号；
--       4 处 guide 写“接收人填本单位接收人”，口吻像院内节点。
-- 口径：title/descr 写公安职责；guide = 外单位只读 + 指挥部代填 F15；
--       evidence 去掉“归档/综合处置记录单”等院内动作。
-- 依赖：需先执行 0014（F15 已含“受理编号”）。
-- 幂等：固定终值；断言按终值校验。
-- =====================================================================
SET NAMES utf8mb4;
USE drill_system;

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

UPDATE script_nodes
   SET title = '市网安大队受理报案并出具受理回执与取证要求',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理勒索病毒/账号被盗报案，出具受理回执：受理编号、是否立案、承办警官、取证边界与补证清单；院方按清单配合补证，非涉案业务可恢复。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官、取证边界与补证时限；接收人/接收时间填本院对接人与实际接收时间。'
 WHERE id = 'S02-14';

UPDATE script_nodes
   SET title = '市网安大队受理DDoS报案并出具回执与补证要求',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理DDoS攻击报案，出具受理回执：受理编号、是否立案、承办警官、补证清单与时限；院方按清单补齐流量证据与日志，形成报案闭环。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官、补证清单与时限；接收人/接收时间填本院对接人与实际接收时间。'
 WHERE id = 'S03-15';

UPDATE script_nodes
   SET title = '市网安大队受理钓鱼邮件报案并出具回执',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理钓鱼邮件报案，出具受理回执：受理编号、受理人、受理时间与是否立案；院方登记回执并按要求补证。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、受理人、受理时间与是否立案；回执由本院登记归档，公安不代本院归档。',
       keywords = '受理回执,受理编号,受理人,报案闭环',
       evidence_req = '[{"type":"上报材料","name":"受理回执","note":"含受理编号、受理人、受理时间"},{"type":"处置记录","name":"补充材料清单","note":"含补证项、责任人与时限"}]'
 WHERE id = 'S04-14';

UPDATE script_nodes
   SET title = '市网安大队受理存储故障报案并出具回执与取证要求',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理存储故障报案，出具受理回执：受理编号、是否立案、取证边界与配合时限；院方按清单配合取证，业务恢复与取证并行。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官、取证边界与配合时限；接收人/接收时间填本院对接人与实际接收时间。'
 WHERE id = 'S05-16';

UPDATE script_nodes
   SET title = '市网安大队受理数据泄露报案并出具回执与取证要求',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理数据泄露报案，出具受理回执：受理编号、是否立案、承办警官、取证边界与补报时限；院方按清单补报，患者信息保护不受影响。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官、取证边界与补报时限；接收人/接收时间填本院对接人与实际接收时间。'
 WHERE id = 'S06-14';

UPDATE script_nodes
   SET title = '市网安大队受理报案并出具受理回执与补证要求',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理边界防火墙被突破报案，出具受理回执：受理编号、是否立案、承办警官、补证清单与时限；院方按清单补证。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官、补证时限与对接人；接收人/接收时间填本院对接人与实际接收时间。'
 WHERE id = 'S07-14';

UPDATE script_nodes
   SET title = '市网安大队受理恶意代码报案并出具受理回执',
       short_title = '网安受理回执',
       form = 'F15',
       descr = '市网安大队受理恶意代码/共享盘感染报案，出具受理回执：受理编号、受理人、受理时间与是否立案；院方登记回执并按补充材料要求补报。',
       guide = '外单位只读；由院应急指挥部代填 F15：记录受理编号、是否立案、承办警官与补充材料要求；接收人/接收时间填本院对接人与实际接收时间。',
       evidence_req = '[{"type":"上报材料","name":"报案受理回执","note":"含受理编号、受理人、受理时间"},{"type":"上报材料","name":"补充材料要求","note":"含补证项与时限"}]'
 WHERE id = 'S08-15';

-- ---------------------------------------------------------------------
-- 断言
-- ---------------------------------------------------------------------
-- 7 个节点全部绑定 F15，且仍为 correct / F2 之外的表单节点
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-14','S03-15','S04-14','S05-16','S06-14','S07-14','S08-15')
      AND form = 'F15'
      AND flow_type = 'correct'
  ) = 7, 1, 0);

-- guide 统一为“外单位只读 + 指挥部代填”
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S02-14','S03-15','S04-14','S05-16','S06-14','S07-14','S08-15')
      AND guide LIKE '外单位只读；由院应急指挥部代填 F15%'
  ) = 7, 1, 0);

-- 标题终值逐条校验
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE (id = 'S02-14' AND title = '市网安大队受理报案并出具受理回执与取证要求')
       OR (id = 'S03-15' AND title = '市网安大队受理DDoS报案并出具回执与补证要求')
       OR (id = 'S04-14' AND title = '市网安大队受理钓鱼邮件报案并出具回执')
       OR (id = 'S05-16' AND title = '市网安大队受理存储故障报案并出具回执与取证要求')
       OR (id = 'S06-14' AND title = '市网安大队受理数据泄露报案并出具回执与取证要求')
       OR (id = 'S07-14' AND title = '市网安大队受理报案并出具受理回执与补证要求')
       OR (id = 'S08-15' AND title = '市网安大队受理恶意代码报案并出具受理回执')
  ) = 7, 1, 0);

-- 证据不再出现院内“归档/综合处置记录单”
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S04-14','S08-15')
      AND evidence_req NOT LIKE '%归档%'
      AND evidence_req NOT LIKE '%综合处置记录单%'
  ) = 2, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
