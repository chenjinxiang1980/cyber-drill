-- =====================================================================
-- 0018_duties_terms.up.sql
-- 批次 E（S02–S08 部分）：术语统一、职责边界纠正、干扰项职责主体修正
-- ---------------------------------------------------------------------
-- 只改文案/关键词/评分关键词，不动表单绑定、不动 target/deps/seq/score。
--
-- 覆盖：
--   S03-8  补充运营商出口链路与上游清洗（descr/guide 追加）
--   S04-8  卫健委口径 → 院统一口径 + 网信办指导
--   S04-12 报案主体明确为信息科负责人，回执由指挥部代收登记
--   S06-11 网信办“审核” → “指导”
--   S07-11 guide 改为报案指引
--   干扰项：DSS02-itlead-8-2、DSS03-itlead-6-1、DSS04-itlead-9-2、
--           DSS07-netsec-10-2、DSS08-netsec-11-2、DSS08-itlead-8-1
--
-- 注：S01 的三个干扰项（DSS01-healthbureau-11-1/2、DSS01-cac-12-2）
--     归入 0019 S01 残留批次，避免与正在进行的 S01 演练/核验并发。
--
-- 幂等：S03-8 用 NOT LIKE 守卫追加；其余为固定终值。
-- =====================================================================
SET NAMES utf8mb4;
USE drill_system;

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
CREATE TEMPORARY TABLE _assert_guard (v TINYINT NOT NULL CHECK (v = 1));

-- ---------------------------------------------------------------------
-- 文案纠正
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET descr = CONCAT(descr, '同步联系运营商确认出口链路，必要时启用上游清洗/黑洞路由；院内设备与运营商处置分别留痕。'),
       guide = CONCAT(guide, '同步联系运营商确认出口链路与上游清洗能力。')
 WHERE id = 'S03-8'
   AND descr NOT LIKE '%运营商%';

UPDATE script_nodes
   SET descr = '同步向市网信办报送初报，与院统一对外口径保持一致；涉及公共传播的口径，征询市网信办指导意见，记录报送时间、接收人与回执。'
 WHERE id = 'S04-8';

UPDATE script_nodes
   SET descr = '信息科负责人向市网安大队报案并提交报案材料、事件经过与流程证据；外部单位只读；受理回执由院应急指挥部代收登记。'
 WHERE id = 'S04-12';

UPDATE script_nodes
   SET descr = '在征询市网信办指导意见、经院内法务审核后，由院新闻发言人按授权统一对外发布事件口径与进展；未经审批不得披露事件细节或患者信息。',
       guide = '口径由院内法务审核，重大公共传播口径征询市网信办指导；只发布已核实事实，不披露患者个人信息。'
 WHERE id = 'S06-11';

UPDATE script_nodes
   SET guide = '先电话报案占受理号，后补书面材料；核对证据清单与报案材料，保留受理编号与补证时限。'
 WHERE id = 'S07-11';

-- ---------------------------------------------------------------------
-- 干扰项：错误点保留在行为/时序，不放在外单位职责主体
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET title = '直接让网络安全管理员与市卫生健康局联系人对接报送',
       descr = '绕过指挥链，让网络安全管理员直接与市卫生健康局联系人对接报送。',
       guide = '技术人直接对接效率最高。',
       keywords = '绕过指挥链,越级直联,报送',
       quality_rules = '{"requiredKeywords":["网络安全管理员","联系人","对接"],"bonusKeywords":["指挥链","绕过","越级"],"minMeasureLen":15,"minResultLen":10,"requiredElements":["对象"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}'
 WHERE id = 'DSS02-itlead-8-2';

UPDATE script_nodes
   SET guide = '卫健局需要完整情况，等网信办指导意见出来一起报。'
 WHERE id = 'DSS03-itlead-6-1';

UPDATE script_nodes
   SET title = '等安全厂商的钓鱼样本分析再封堵',
       descr = '等安全厂商的样本分析结果再封堵，封堵空窗期内邮件继续投递与点击。'
 WHERE id = 'DSS04-itlead-9-2';

UPDATE script_nodes
   SET title = '让厂商代为取证',
       descr = '厂商可协助样本分析，但电子数据取证由公安负责；把取证外包给厂商会延误时机并破坏证据链。',
       guide = '取证权属公安，厂商仅提供技术支持。',
       keywords = '厂商,取证,公安',
       quality_rules = '{"requiredKeywords":["厂商","取证","公安"],"bonusKeywords":["样本分析","延误","证据链"],"minMeasureLen":15,"minResultLen":10,"requiredElements":["对象"],"fillerWords":["无","已观察","等待中","已在处理","正常","ok","OK"],"fillerMaxLen":6,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}'
 WHERE id = 'DSS07-netsec-10-2';

UPDATE script_nodes
   SET descr = '让厂商代为取证：厂商可协助样本分析，但电子数据取证由公安负责，本院应先自行保全；依赖外部会延误取证时机。',
       guide = '取证由公安负责，厂商仅提供技术支持。'
 WHERE id = 'DSS08-netsec-11-2';

UPDATE script_nodes
   SET guide = '让安全厂商分析。'
 WHERE id = 'DSS08-itlead-8-1';

-- ---------------------------------------------------------------------
-- 断言
-- ---------------------------------------------------------------------
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S03-8'
      AND descr LIKE '%运营商%'
      AND guide LIKE '%运营商%'
  ) = 1, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S04-8' AND descr LIKE '%院统一对外口径%' AND descr LIKE '%指导意见%')
  + (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S04-12' AND descr LIKE '%受理回执由院应急指挥部代收登记%')
  + (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S06-11' AND descr LIKE '%指导意见%' AND descr NOT LIKE '%网信办审核口径后%')
  + (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'S07-11' AND guide LIKE '%报案%')
  = 4, 1, 0);

INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE (id = 'DSS02-itlead-8-2' AND title = '直接让网络安全管理员与市卫生健康局联系人对接报送' AND guide = '技术人直接对接效率最高。')
       OR (id = 'DSS03-itlead-6-1' AND guide = '卫健局需要完整情况，等网信办指导意见出来一起报。')
       OR (id = 'DSS04-itlead-9-2' AND title = '等安全厂商的钓鱼样本分析再封堵')
       OR (id = 'DSS07-netsec-10-2' AND title = '让厂商代为取证' AND guide = '取证权属公安，厂商仅提供技术支持。')
       OR (id = 'DSS08-netsec-11-2' AND descr LIKE '%本院应先自行保全%' AND guide = '取证由公安负责，厂商仅提供技术支持。')
       OR (id = 'DSS08-itlead-8-1' AND guide = '让安全厂商分析。')
  ) = 6, 1, 0);

-- 干扰项评分关键词与最终文案自洽（requiredKeywords 均能在 title/descr/guide 中命中）
INSERT INTO _assert_guard (v)
SELECT IF(
  (SELECT COUNT(*) FROM script_nodes
    WHERE id = 'DSS07-netsec-10-2'
      AND quality_rules LIKE '%"厂商"%'
      AND quality_rules LIKE '%"公安"%'
      AND descr LIKE '%厂商%'
  ) = 1, 1, 0);

DROP TEMPORARY TABLE IF EXISTS _assert_guard;
