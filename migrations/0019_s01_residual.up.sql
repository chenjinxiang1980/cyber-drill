-- =====================================================================
-- 0019_s01_residual.up.sql
-- 批次 D：S01 外单位残留项（当前暂缓执行，等 S01 演练/核验会话结束）
-- ---------------------------------------------------------------------
-- 覆盖：
--   S01-12/13/16  guide 代填口径（外单位只读 + 指挥部代填）
--   S01-13        evidence 删除“舆情风险研判表”，改“外部单位指导意见记录单”
--   S01-17        evidence “应急结束审批与反馈单”→“应急响应结束记录单”
--   DSS01-healthbureau-11-2（必改）、DSS01-healthbureau-11-1、DSS01-cac-12-2
--
-- 依赖：0014（F15 已含受理编号）。
-- 幂等：固定终值。
-- 注意：执行前必须确认 S01 演练项目 P2026-001 已结束、无会话在读写，
--       否则会改变参演人/核验方看到的 S01 剧本内容。
-- =====================================================================
SET NAMES utf8mb4;
-- 不再硬编码 USE：迁移必须跟随连接库（DB_NAME）执行。

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

-- 前置守卫：存在“演练中”项目时禁止执行（避免改动正在参演/核验的 S01 剧本）
INSERT INTO _assert_guard (v)
SELECT IF((SELECT COUNT(*) FROM projects WHERE status = '演练中') = 0, 1, 0);

-- 前置守卫：0014 已为 F15 增加“受理编号”
INSERT INTO _assert_guard (v)
SELECT IF((SELECT COUNT(*) FROM form_templates
            WHERE id = 'F15'
              AND JSON_SEARCH(fields, 'one', '受理编号', NULL, '$[*].label') IS NOT NULL) = 1, 1, 0);

-- ---------------------------------------------------------------------
-- S01 外单位节点：guide 统一为“只读 + 指挥部代填”
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET guide = '外单位只读；由院应急指挥部代填：记录指导意见要点、续报时限与联系人；接收人/接收时间填本院对接人与实际接收时间；落实情况由本院后续补填。行业指导不等同于响应等级审批，定级仍由院应急指挥部决定。'
 WHERE id = 'S01-12';

UPDATE script_nodes
   SET guide = '外单位只读；由院应急指挥部代填：记录指导要求、通报口径、反馈时限与联系人；接收人/接收时间填本院对接人与实际接收时间。舆情监测属本院宣传/舆情岗位职责，不由网信办承担。'
 WHERE id = 'S01-13';

UPDATE script_nodes
   SET guide = '外单位只读；由院应急指挥部代填：确认受理编号、是否立案、承办警官；按取证清单记录保全范围与时限。保护现场不等于全院停机。'
 WHERE id = 'S01-16';

-- ---------------------------------------------------------------------
-- S01 证据同步
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET evidence_req = '[{"type":"处置记录","name":"口径回执","note":"含统一表述、通报要求与复查意见"},{"type":"表单文书","name":"外部单位指导意见记录单","note":"含指导要求、通报口径与反馈时限"}]'
 WHERE id = 'S01-13';

UPDATE script_nodes
   SET evidence_req = '[{"type":"表单文书","name":"应急响应结束记录单","note":"含四项结束条件核对结果与批准人"},{"type":"处置记录","name":"整改初稿","note":"含整改项、责任人与时限"}]'
 WHERE id = 'S01-17';

-- ---------------------------------------------------------------------
-- S01 干扰项职责主体修正（错误点保留在行为/时序）
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET title = '建议院方以业务为重，先保障业务连续性',
       descr = '业务中断直接影响患者安全，建议先保业务；取证由医院按公安要求后置安排，恢复过程可能覆盖部分现场。',
       guide = '先保业务连续性，取证后置。',
       keywords = '业务优先,后取证,现场',
       quality_rules = '{"requiredKeywords":["业务","取证","现场"],"bonusKeywords":["患者安全","恢复后","覆盖"],"minMeasureLen":20,"minResultLen":12,"requiredElements":["对象"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.15,"keywordPenaltyRatio":0.25}'
 WHERE id = 'DSS01-healthbureau-11-1';

UPDATE script_nodes
   SET title = '要求医院必须经卫健局同意后才能启动响应',
       descr = '把响应启动审批权收到卫健局，医院须先取得同意才能启动；延误启动时机，且卫健局不代行院内响应等级审批。',
       guide = '先报卫健局等批准再启动。',
       keywords = '越权审批,启动响应,延误',
       quality_rules = '{"requiredKeywords":["卫健局","同意","启动"],"bonusKeywords":["越权","审批","延误"],"minMeasureLen":15,"minResultLen":10,"requiredElements":["对象"],"fillerWords":["无","已研判","已定级","按预案","正常","已知悉","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}'
 WHERE id = 'DSS01-healthbureau-11-2';

UPDATE script_nodes
   SET title = '要求医院先删帖控评，再处置',
       descr = '先压住舆情再处置可减少扩散；删除帖子会灭失传播证据，并打乱通报顺序。',
       guide = '先删帖控评。',
       keywords = '删帖,控评,证据灭失',
       quality_rules = '{"requiredKeywords":["删帖","控评","处置"],"bonusKeywords":["证据灭失","通报顺序","舆情"],"minMeasureLen":20,"minResultLen":12,"requiredElements":["对象"],"fillerWords":["无","已处理","已完成","已隔离","正常","收到","处理完毕","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.15,"keywordPenaltyRatio":0.25}'
 WHERE id = 'DSS01-cac-12-2';

-- ---------------------------------------------------------------------
-- 断言
-- ---------------------------------------------------------------------
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id IN ('S01-12','S01-13','S01-16')
      AND guide LIKE '外单位只读；由院应急指挥部代填%'
  ) = 3, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S01-13'
      AND evidence_req LIKE '%外部单位指导意见记录单%'
      AND evidence_req NOT LIKE '%舆情风险研判表%'
  ) = 1
  AND
  (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S01-17'
      AND evidence_req LIKE '%应急响应结束记录单%'
      AND evidence_req NOT LIKE '%应急结束审批与反馈单%'
  ) = 1, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE (id = 'DSS01-healthbureau-11-1' AND title = '建议院方以业务为重，先保障业务连续性')
       OR (id = 'DSS01-healthbureau-11-2' AND title = '要求医院必须经卫健局同意后才能启动响应')
       OR (id = 'DSS01-cac-12-2' AND title = '要求医院先删帖控评，再处置')
  ) = 3, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
