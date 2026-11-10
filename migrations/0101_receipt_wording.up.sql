-- 0101_receipt_wording.up.sql
-- 回执口径收敛（业务 P1-5）：把"必须有回执"改为"回执 / 接收记录 / 无回执说明"三选一可满足。
-- 不动 score / required / deps / flow_type / seq / title / target / receiver / form / timeout。
SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 100000000;

-- ==================== 一、前置基线 ====================
SET @c_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @c_corr  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct');
SET @c_dist  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @c_forms := (SELECT COUNT(*) FROM form_templates);
SET @c_scen  := (SELECT COUNT(*) FROM scenarios);
SET @c_roles := (SELECT COUNT(*) FROM roles);
SET @c_mig   := (SELECT COUNT(*) FROM schema_migrations);
SET @c_last  := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
SET @ms_before := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);

-- 37 个目标节点（24 个质量规则 + 6 个仅证据 + 7 个升级研判）不可变列指纹
SET @fp_imm_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id IN ('S01-5A','S01-8','S01-12','S01-13','S01-15','S01-16','S01-19','S02-3A','S02-8','S02-9','S02-12','S02-14','S03-3A','S03-8A','S03-11','S03-15','S04-7','S04-8','S04-12','S04-13','S04-14','S05-3A','S05-16','S06-3A','S06-6','S06-7','S06-10','S06-12','S06-14','S07-3A','S07-13','S07-14','S08-3A','S08-9','S08-12','S08-14','S08-15'));

-- 范围外行指纹
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S01-5A','S01-8','S01-12','S01-13','S01-15','S01-16','S01-19','S02-3A','S02-8','S02-9','S02-12','S02-14','S03-3A','S03-8A','S03-11','S03-15','S04-7','S04-8','S04-12','S04-13','S04-14','S05-3A','S05-16','S06-3A','S06-6','S06-7','S06-10','S06-12','S06-14','S07-3A','S07-13','S07-14','S08-3A','S08-9','S08-12','S08-14','S08-15'));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);

-- 24 个节点质量规则原文精确匹配
SET @q_old_qr := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND (
   (id='S01-8'  AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","市网信办|网信办|市委网信办","回执"],"bonusKeywords":["1小时","时限依据","不等回复"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
OR (id='S01-12' AND quality_rules='{"requiredKeywords":["响应级别|响应等级|响应分级","证据保全","回执"],"bonusKeywords":["上报时限","联系人","升级"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='行业指导,响应级别,证据保全,回执')
OR (id='S01-19' AND quality_rules='{"requiredKeywords":["向网信办报送|向市网信办报送|向市委网信办报送|报送市网信办|报送市委网信办|报送至市网信办|报送至市委网信办|报市网信办|报市委网信办|报至市网信办|市委网信办报送|网信办报送","事件基本情况|基本情况|影响范围|影响面|波及范围","已采取措施|采取的措施|处置措施|应对措施|已处置|已完成处置"],"bonusKeywords":["1小时","一小时内","回执","研判"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
OR (id='S02-8'  AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","影响范围","回执"],"bonusKeywords":["1小时","已采取措施","先电话后书面"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
OR (id='S02-9'  AND quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","舆情风险","回执"],"bonusKeywords":["同一时点","不等回复","攻击来源","事件编号"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
OR (id='S02-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["不等待","事件经过","联系人","书面材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S02-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","材料清单"],"bonusKeywords":["取证要求","提交时限","对接人","转指挥部"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='受理回执,补充材料,提交时限,对接人')
OR (id='S03-8A' AND quality_rules='{"requiredKeywords":["运营商|上游","工单|清洗","回执|确认"],"bonusKeywords":["出口链路","封禁范围","处置时段","先内后外"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已处理","已完成","已隔离","正常","收到","处理完毕","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}' AND keywords='运营商,工单,清洗,回执')
OR (id='S03-11' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["先行","抓包","哈希","补充材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S03-15' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","取证要求"],"bonusKeywords":["受理人","补充材料","报案闭环","签收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='受理回执,受理编号,补充材料')
OR (id='S04-7'  AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","初报","回执"],"bonusKeywords":["钓鱼邮件","一般","4小时","报送时间","同时报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S04-8'  AND quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","口径","回执"],"bonusKeywords":["初报","报送时间","接收人","市卫生健康局","同步报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='市网信办,4小时,初报,口径一致,回执')
OR (id='S04-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案材料","受理回执|受案回执|回执单|回执"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}')
OR (id='S04-13' AND quality_rules='{"requiredKeywords":["无新增点击|无新增中招|无新增感染","终端验证","闭环|闭环管理|办结|归档"],"bonusKeywords":["24小时","邮件召回","报案回执","Ⅳ级"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S04-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","归档"],"bonusKeywords":["受理人","受理时间","报案材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='受理回执,受理编号,受理人,报案闭环')
OR (id='S05-16' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","取证要求","配合时限"],"bonusKeywords":["受理编号","取证边界","只读","代提交"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}' AND keywords='受理,回执,取证,网安')
OR (id='S06-6'  AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","事件等级|事件级别|事件分级","回执"],"bonusKeywords":["1小时","泄露范围","舆情","同步上报","续报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}' AND keywords='上报,卫健,1小时,舆情,回执')
OR (id='S06-14' AND quality_rules='{"requiredKeywords":["受理编号|受案编号|案件编号|回执编号","取证要求","回执"],"bonusKeywords":["受理人","取证清单","代收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='回执,受理,立案,取证要求')
OR (id='S07-13' AND quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","受理回执|受案回执|回执单|回执","闭环|闭环管理|办结|归档"],"bonusKeywords":["策略基线","证据固定","横向排查","报备"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}')
OR (id='S07-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","补证"],"bonusKeywords":["补证时限","对接人","缺件","归档"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='报案,回执,受理,补证')
OR (id='S08-9'  AND quality_rules='{"requiredKeywords":["全院预警","禁用|禁止|封禁|不插入|不使用|不得使用|不要插入","回执"],"bonusKeywords":["OA","短信","病毒库","可疑文件"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","结论"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S08-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
OR (id='S08-14' AND quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","复扫|复查|再次扫描|二次扫描","受理回执|受案回执|回执单|回执"],"bonusKeywords":["共享盘","离线终端","宣布结束","留存记录"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}')
OR (id='S08-15' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","补充材料"],"bonusKeywords":["补报时限","代录入","核对"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='报案,受理,回执,网安大队')
));

-- 28 条证据名称含"回执"（另有 S04-14、S08-15 仅 note 含，名称本身已合规）
SET @q_old_ev := (SELECT COUNT(*) FROM script_nodes n JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(ev_name VARCHAR(128) PATH '$.name')) j
  WHERE n.deleted_at IS NULL AND j.ev_name LIKE '%回执%');

SET @guard_pre := IF(@c_nodes=357 AND @c_corr=137 AND @c_dist=220 AND @c_forms=19 AND @c_scen=8 AND @c_roles=15 AND @c_mig=56 AND @c_last='0100_roles_upgrade'
  AND @ms_before='S01:112,S02:106,S03:110,S04:99,S05:107,S06:106,S07:105,S08:104'
  AND @q_old_qr=24 AND @q_old_ev=28,
  1, (SELECT table_name FROM information_schema.tables));

-- ==================== 二、24 个节点质量规则 ====================
SET @rows := 0;

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","市网信办|网信办|市委网信办","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["1小时","时限依据","不等回复"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S01-8' AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","市网信办|网信办|市委网信办","回执"],"bonusKeywords":["1小时","时限依据","不等回复"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["响应级别|响应等级|响应分级","证据保全","回执|接收记录|沟通记录|接收人确认"],"bonusKeywords":["上报时限","联系人","升级"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}', keywords='行业指导,响应级别,证据保全,接收确认'
 WHERE id='S01-12' AND quality_rules='{"requiredKeywords":["响应级别|响应等级|响应分级","证据保全","回执"],"bonusKeywords":["上报时限","联系人","升级"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='行业指导,响应级别,证据保全,回执';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["向网信办报送|向市网信办报送|向市委网信办报送|报送市网信办|报送市委网信办|报送至市网信办|报送至市委网信办|报市网信办|报市委网信办|报至市网信办|市委网信办报送|网信办报送","事件基本情况|基本情况|影响范围|影响面|波及范围","已采取措施|采取的措施|处置措施|应对措施|已处置|已完成处置"],"bonusKeywords":["1小时","一小时内","接收记录","研判"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S01-19' AND quality_rules='{"requiredKeywords":["向网信办报送|向市网信办报送|向市委网信办报送|报送市网信办|报送市委网信办|报送至市网信办|报送至市委网信办|报市网信办|报市委网信办|报至市网信办|市委网信办报送|网信办报送","事件基本情况|基本情况|影响范围|影响面|波及范围","已采取措施|采取的措施|处置措施|应对措施|已处置|已完成处置"],"bonusKeywords":["1小时","一小时内","回执","研判"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","影响范围","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["1小时","已采取措施","先电话后书面"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S02-8' AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","影响范围","回执"],"bonusKeywords":["1小时","已采取措施","先电话后书面"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","舆情风险","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["同一时点","不等回复","攻击来源","事件编号"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S02-9' AND quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","舆情风险","回执"],"bonusKeywords":["同一时点","不等回复","攻击来源","事件编号"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号|接报人|警号"],"bonusKeywords":["不等待","事件经过","联系人","书面材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S02-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["不等待","事件经过","联系人","书面材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","受理编号|受案编号|案件编号|回执编号|接报人|警号","材料清单"],"bonusKeywords":["取证要求","提交时限","对接人","转指挥部"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}', keywords='受理记录,补充材料,提交时限,对接人'
 WHERE id='S02-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","材料清单"],"bonusKeywords":["取证要求","提交时限","对接人","转指挥部"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='受理回执,补充材料,提交时限,对接人';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["运营商|上游","工单|清洗","回执|工单确认|接收确认|处置反馈|通话记录"],"bonusKeywords":["出口链路","封禁范围","处置时段","先内后外"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已处理","已完成","已隔离","正常","收到","处理完毕","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}', keywords='运营商,工单,清洗,处置反馈'
 WHERE id='S03-8A' AND quality_rules='{"requiredKeywords":["运营商|上游","工单|清洗","回执|确认"],"bonusKeywords":["出口链路","封禁范围","处置时段","先内后外"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已处理","已完成","已隔离","正常","收到","处理完毕","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}' AND keywords='运营商,工单,清洗,回执';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号|接报人|警号"],"bonusKeywords":["先行","抓包","哈希","补充材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S03-11' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["先行","抓包","哈希","补充材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","受理编号|受案编号|案件编号|回执编号|接报人|警号","取证要求"],"bonusKeywords":["受理人","补充材料","报案闭环","签收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}', keywords='受理记录,受理编号,补充材料'
 WHERE id='S03-15' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","取证要求"],"bonusKeywords":["受理人","补充材料","报案闭环","签收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='受理回执,受理编号,补充材料';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","初报","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["钓鱼邮件","一般","4小时","报送时间","同时报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S04-7' AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","初报","回执"],"bonusKeywords":["钓鱼邮件","一般","4小时","报送时间","同时报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","口径","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["初报","报送时间","接收人","市卫生健康局","同步报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}', keywords='市网信办,4小时,初报,口径一致,接收确认'
 WHERE id='S04-8' AND quality_rules='{"requiredKeywords":["市网信办|网信办|市委网信办","口径","回执"],"bonusKeywords":["初报","报送时间","接收人","市卫生健康局","同步报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='市网信办,4小时,初报,口径一致,回执';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案材料","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}'
 WHERE id='S04-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案材料","受理回执|受案回执|回执单|回执"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["无新增点击|无新增中招|无新增感染","终端验证","闭环|闭环管理|办结|归档"],"bonusKeywords":["24小时","邮件召回","接报确认","Ⅳ级"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S04-13' AND quality_rules='{"requiredKeywords":["无新增点击|无新增中招|无新增感染","终端验证","闭环|闭环管理|办结|归档"],"bonusKeywords":["24小时","邮件召回","报案回执","Ⅳ级"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","受理编号|受案编号|案件编号|回执编号|接报人|警号","归档"],"bonusKeywords":["受理人","受理时间","报案材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}', keywords='受理记录,受理编号,受理人,报案闭环'
 WHERE id='S04-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","归档"],"bonusKeywords":["受理人","受理时间","报案材料"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='受理回执,受理编号,受理人,报案闭环';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","取证要求","配合时限"],"bonusKeywords":["受理编号","取证边界","只读","代提交"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}', keywords='受理,沟通记录,取证,网安'
 WHERE id='S05-16' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","取证要求","配合时限"],"bonusKeywords":["受理编号","取证边界","只读","代提交"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.15,"keywordPenaltyRatio":0.2}' AND keywords='受理,回执,取证,网安';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","事件等级|事件级别|事件分级","回执|接收记录|报送截图|通话记录|接收人确认"],"bonusKeywords":["1小时","泄露范围","舆情","同步上报","续报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}', keywords='上报,卫健,1小时,舆情,接收确认'
 WHERE id='S06-6' AND quality_rules='{"requiredKeywords":["市卫生健康局|市卫健局|卫健委|卫生健康局","事件等级|事件级别|事件分级","回执"],"bonusKeywords":["1小时","泄露范围","舆情","同步上报","续报"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","动作","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}' AND keywords='上报,卫健,1小时,舆情,回执';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理编号|受案编号|案件编号|回执编号|接报人|警号","取证要求","回执|接报确认|沟通记录|接收人确认"],"bonusKeywords":["受理人","取证清单","代收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}', keywords='受理记录,受理,立案,取证要求'
 WHERE id='S06-14' AND quality_rules='{"requiredKeywords":["受理编号|受案编号|案件编号|回执编号","取证要求","回执"],"bonusKeywords":["受理人","取证清单","代收"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='回执,受理,立案,取证要求';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","受理回执|受案回执|受理编号|接报确认|沟通记录|闭环说明|不报案登记","闭环|闭环管理|办结|归档"],"bonusKeywords":["策略基线","证据固定","横向排查","报备"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}'
 WHERE id='S07-13' AND quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","受理回执|受案回执|回执单|回执","闭环|闭环管理|办结|归档"],"bonusKeywords":["策略基线","证据固定","横向排查","报备"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","受理编号|受案编号|案件编号|回执编号|接报人|警号","补证"],"bonusKeywords":["补证时限","对接人","缺件","归档"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}', keywords='报案,受理记录,受理,补证'
 WHERE id='S07-14' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","补证"],"bonusKeywords":["补证时限","对接人","缺件","归档"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}' AND keywords='报案,回执,受理,补证';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["全院预警","禁用|禁止|封禁|不插入|不使用|不得使用|不要插入","回执|接收确认|签收记录|群发截图|已通知"],"bonusKeywords":["OA","短信","病毒库","可疑文件"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","结论"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S08-9' AND quality_rules='{"requiredKeywords":["全院预警","禁用|禁止|封禁|不插入|不使用|不得使用|不要插入","回执"],"bonusKeywords":["OA","短信","病毒库","可疑文件"],"minMeasureLen":40,"minResultLen":25,"requiredElements":["对象","结论"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S08-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","复扫|复查|再次扫描|二次扫描","受理回执|受案回执|受理编号|接报确认|沟通记录|闭环说明|不报案登记"],"bonusKeywords":["共享盘","离线终端","宣布结束","留存记录"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}'
 WHERE id='S08-14' AND quality_rules='{"requiredKeywords":["结束条件|终止条件|结束标准|终止标准","复扫|复查|再次扫描|二次扫描","受理回执|受案回执|回执单|回执"],"bonusKeywords":["共享盘","离线终端","宣布结束","留存记录"],"minMeasureLen":35,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已审批","同意","已阅","收到","按要求处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET quality_rules='{"requiredKeywords":["受理回执|受案回执|受理编号|接报确认|沟通记录","受理编号|受案编号|案件编号|回执编号|接报人|警号","补充材料"],"bonusKeywords":["补报时限","代录入","核对"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}', keywords='报案,受理,受理记录,网安大队'
 WHERE id='S08-15' AND quality_rules='{"requiredKeywords":["受理回执|受案回执|回执单|回执","受理编号|受案编号|案件编号|回执编号","补充材料"],"bonusKeywords":["补报时限","代录入","核对"],"minMeasureLen":30,"minResultLen":18,"requiredElements":[],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}' AND keywords='报案,受理,回执,网安大队';
SET @rows := @rows + ROW_COUNT();

-- ==================== 三、30 个节点证据改名（名称不再以"回执"为前缀/主词） ====================
UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S01-5A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"上报与接收记录","note":"仅市卫生健康局一份，含时间与接收人；回执/报送截图/通话记录/接收人确认任一即可"},{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象与时限依据"}]'
 WHERE id='S01-8' AND evidence_req='[{"type":"上报材料","name":"上报回执","note":"仅市卫生健康局一份，含时间与接收人"},{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象与时限依据"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"处置记录","name":"市卫生健康局指导意见记录","note":"含意见要点、联系人与时间；回执/通话记录/接收人确认任一即可"}]'
 WHERE id='S01-12' AND evidence_req='[{"type":"处置记录","name":"市卫生健康局指导意见回执","note":"含意见要点、联系人与时间"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"处置记录","name":"口径指导与接收记录","note":"含统一表述、通报要求、复查意见与接收人；回执/通话记录/接收人确认任一即可"},{"type":"表单文书","name":"外部单位指导意见记录单","note":"含指导要求、通报口径与反馈时限"}]'
 WHERE id='S01-13' AND evidence_req='[{"type":"处置记录","name":"口径回执","note":"含统一表述、通报要求与复查意见"},{"type":"表单文书","name":"外部单位指导意见记录单","note":"含指导要求、通报口径与反馈时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含事件经过、损失与证据清单"},{"type":"上报材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与时间；回执/接报确认/通话记录任一即可"}]'
 WHERE id='S01-15' AND evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含事件经过、损失与证据清单"},{"type":"上报材料","name":"受理回执","note":"含受理编号与受理人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"处置记录","name":"取证要求清单","note":"含保全范围、联系人与时限"}]'
 WHERE id='S01-16' AND evidence_req='[{"type":"上报材料","name":"受理回执","note":"含受理编号与受理人"},{"type":"处置记录","name":"取证要求清单","note":"含保全范围、联系人与时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"上报与接收记录","note":"市网信办一份，含时间与接收人；回执/报送截图/通话记录/接收人确认任一即可"},{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象与时限依据"}]'
 WHERE id='S01-19' AND evidence_req='[{"type":"上报材料","name":"上报回执","note":"两个接收单位各一份，含时间与接收人"},{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象与时限依据"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S02-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"上报与接收记录","note":"含接收人、时间与编号（如有）；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"}]'
 WHERE id='S02-8' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"上报回执","note":"含接收人、回执编号与时间；未取得回执时附报送截图、通话记录与接收人确认"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"上报与接收记录","note":"含接收人、时间与编号（如有）；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"}]'
 WHERE id='S02-9' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"上报回执","note":"含接收人、回执编号与时间；未取得回执时附报送截图、通话记录与接收人确认"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含报案时间、报案方式与证据清单"},{"type":"报案材料","name":"受案受理与对接记录","note":"含受理编号（如有）、接待人/警号与时间；回执/接报确认/通话记录任一即可"}]'
 WHERE id='S02-12' AND evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含报案时间、报案方式与证据清单"},{"type":"报案材料","name":"受案回执或对接记录","note":"含受理编号或接待人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理单位与受理时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"处置记录","name":"补充材料清单","note":"含材料项、责任人、提交时限"}]'
 WHERE id='S02-14' AND evidence_req='[{"type":"报案材料","name":"受理回执","note":"含受理编号、受理单位与受理时间"},{"type":"处置记录","name":"补充材料清单","note":"含材料项、责任人、提交时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S03-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"附件","name":"运营商工单与处置反馈","note":"工单编号、处置时段与处置结论；回执/工单确认/通话记录任一即可"},{"type":"处置记录","name":"出口链路确认记录","note":"含运营商联系人、链路与清洗能力确认结果"}]'
 WHERE id='S03-8A' AND evidence_req='[{"type":"附件","name":"运营商工单/流量清洗回执","note":"工单编号、处置时段与回执截图"},{"type":"处置记录","name":"出口链路确认记录","note":"含运营商联系人、链路与清洗能力确认结果"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与受理时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"上报材料","name":"补充材料清单","note":"含提交项、签收人与时间"}]'
 WHERE id='S03-15' AND evidence_req='[{"type": "报案材料", "name": "报案受理回执", "note": "含受理编号、受理人与受理时间"}, {"type": "上报材料", "name": "补充材料清单", "note": "含提交项、签收人与时间"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表（F2）","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"报送与接收记录","note":"含报送时间、接收人与编号（如有）；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"}]'
 WHERE id='S04-7' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表（F2）","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"报送回执或编号","note":"未取得回执时记录报送时间与接收人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表（F2）","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"报送与接收记录","note":"含报送时间、接收人与编号（如有）；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"}]'
 WHERE id='S04-8' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表（F2）","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"报送回执或编号","note":"未取得回执时记录报送时间与接收人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、接报确认时间、对方姓名/警号；回执/沟通记录/接收人确认任一即可，均无时附代填说明"},{"type":"处置记录","name":"补充材料清单","note":"含补证项、责任人与时限"}]'
 WHERE id='S04-14' AND evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、口头回执时间、对方姓名/警号；未取得书面回执时附沟通记录与代填说明"},{"type":"处置记录","name":"补充材料清单","note":"含补证项、责任人与时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S05-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"上报材料","name":"取证要求清单","note":"含取证边界、配合时限与联系人"}]'
 WHERE id='S05-16' AND evidence_req='[{"type":"上报材料","name":"受理回执","note":"含受理编号与受理人"},{"type":"上报材料","name":"取证要求清单","note":"含取证边界、配合时限与联系人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S06-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与报送时间"},{"type":"上报材料","name":"市卫生健康局上报与接收记录","note":"含接收人与时间；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"}]'
 WHERE id='S06-6' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与报送时间"},{"type":"上报材料","name":"市卫生健康局上报回执","note":"含接收人与回执编号"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与报送时间"},{"type":"上报材料","name":"市网信办上报与接收记录","note":"含接收人与时间；回执/报送截图/通话记录/接收人确认任一即可，均无时写明无回执说明"},{"type":"样本摘要","name":"样本哈希清单","note":"外发前已固定哈希"}]'
 WHERE id='S06-7' AND evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与报送时间"},{"type":"上报材料","name":"市网信办上报回执","note":"含接收人与回执编号"},{"type":"样本摘要","name":"样本哈希清单","note":"外发前已固定哈希"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含事件经过、字段清单与证据目录"},{"type":"报案材料","name":"报案材料与证据目录","note":"仅字段名与结构，不含患者个人信息"},{"type":"上报材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与时间；回执/接报确认/通话记录任一即可"}]'
 WHERE id='S06-10' AND evidence_req='[{"type":"表单文书","name":"报案登记表","note":"含事件经过、字段清单与证据目录"},{"type":"报案材料","name":"报案材料与证据目录","note":"仅字段名与结构，不含患者个人信息"},{"type":"上报材料","name":"受理回执","note":"含受理编号与受理人"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"处置记录","name":"患者告知记录","note":"含告知范围、方式、时间与执行人"},{"type":"表单文书","name":"法务审核意见","note":"含告知内容与合规结论"},{"type":"表单文书","name":"个人信息保护影响评估记录","note":"含泄露数据类别、敏感个人信息范围、告知方式与监管报告义务"},{"type":"处置记录","name":"患者告知模板与发送记录","note":"含模板版本、发送渠道、覆盖人数与时间"},{"type":"表单文书","name":"监管报告与受理记录","note":"向监管部门报告的受理编号或回执；未要求报告时注明依据"}]'
 WHERE id='S06-12' AND evidence_req='[{"type":"处置记录","name":"患者告知记录","note":"含告知范围、方式、时间与执行人"},{"type":"表单文书","name":"法务审核意见","note":"含告知内容与合规结论"},{"type":"表单文书","name":"个人信息保护影响评估记录","note":"含泄露数据类别、敏感个人信息范围、告知方式与监管报告义务"},{"type":"处置记录","name":"患者告知模板与发送记录","note":"含模板版本、发送渠道、覆盖人数与时间"},{"type":"表单文书","name":"监管报告回执","note":"向监管部门报告的受理编号或回执；未要求报告时注明依据"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"市网安大队受理与沟通记录","note":"含受理编号（如有）、受理人与受理时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"处置记录","name":"取证要求清单","note":"含补充材料、联系人与时限"}]'
 WHERE id='S06-14' AND evidence_req='[{"type":"上报材料","name":"市网安大队受理回执","note":"含受理编号、受理人与受理时间"},{"type":"处置记录","name":"取证要求清单","note":"含补充材料、联系人与时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S07-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"上报材料","name":"报案受理与沟通记录","note":"含受理编号（如有）、受理人与时间；回执/接报确认/通话记录任一即可，均无时写无回执说明（信息科负责人代填）"},{"type":"处置记录","name":"补充材料要求清单","note":"含补证项与时限"}]'
 WHERE id='S07-14' AND evidence_req='[{"type":"上报材料","name":"受理回执","note":"含受理编号与受理人"},{"type":"处置记录","name":"补充材料要求清单","note":"含补证项与时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与接收记录","note":"升级时含新等级、补报对象、时间与接收人；回执/接收记录/通话记录任一即可；维持时注明观察时限"}]'
 WHERE id='S08-3A' AND evidence_req='[{"type":"表单文书","name":"升级研判记录","note":"含维持/升级结论、事实依据（引用原研判节点已采集的系统/数据/患者安全/扩散事实）、触发条件与报批时间"},{"type":"上报材料","name":"升级续报与回执","note":"升级时含新等级、补报对象、时间与接收回执；维持时注明观察时限"}]';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、接报确认时间、对方姓名/警号；回执/沟通记录/接收人确认任一即可，均无时附代填说明"},{"type":"处置记录","name":"补充材料要求","note":"含补证项与时限"}]'
 WHERE id='S08-15' AND evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、口头回执时间、对方姓名/警号；未取得书面回执时附沟通记录与代填说明"},{"type":"处置记录","name":"补充材料要求","note":"含补证项与时限"}]';
SET @rows := @rows + ROW_COUNT();

-- ==================== 四、9 个代填/外部协同节点 guide 口径统一 ====================
UPDATE script_nodes SET guide=REPLACE(guide,'附本院上报材料与沟通记录。','附本院上报材料与沟通记录；由信息科负责人代填，附报送截图/通话记录/接收人确认。')
 WHERE id='S01-12' AND guide LIKE '%附本院上报材料与沟通记录。%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'附舆情监测截图与沟通记录。','附舆情监测截图与沟通记录；由信息科负责人代填，附报送截图/通话记录/接收人确认。')
 WHERE id='S01-13' AND guide LIKE '%附舆情监测截图与沟通记录。%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S01-16' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S02-14' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'工单号、时段与回执必须落档；外部单位不登录系统，由信息科负责人代填并记录代填人。','工单号、处置时段与处置反馈必须落档（回执/工单确认/通话记录任一即可）；外部单位不登录系统，由信息科负责人代填，附报送截图/通话记录/接收人确认，并记录代填人。')
 WHERE id='S03-8A' AND guide LIKE '%工单号、时段与回执必须落档；外部单位不登录系统，由信息科负责人代填并记录代填人。%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S03-15' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S05-16' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S06-14' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

UPDATE script_nodes SET guide=REPLACE(guide,'未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；','未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；由信息科负责人代填，附报送截图/通话记录/接收人确认，均无任何报送或沟通证明才视为未完成；')
 WHERE id='S07-14' AND guide LIKE '%未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；%';
SET @rows := @rows + ROW_COUNT();

-- ==================== 四之二、3 个仅证据节点 keywords 列去裸"回执" ====================
UPDATE script_nodes SET keywords='舆情口径,监管要求,复查,接收确认' WHERE id='S01-13' AND keywords='舆情口径,监管要求,复查,回执';
SET @rows := @rows + ROW_COUNT();
UPDATE script_nodes SET keywords='受理记录,取证要求,保护现场' WHERE id='S01-16' AND keywords='受理回执,取证要求,保护现场';
SET @rows := @rows + ROW_COUNT();
UPDATE script_nodes SET keywords='上报,网信,1小时,舆情,接收确认' WHERE id='S06-7' AND keywords='上报,网信,1小时,舆情,回执';
SET @rows := @rows + ROW_COUNT();

-- ==================== 五、后置断言 ====================
SET @c_nodes2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @c_corr2  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct');
SET @c_dist2  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @c_forms2 := (SELECT COUNT(*) FROM form_templates);
SET @c_scen2  := (SELECT COUNT(*) FROM scenarios);
SET @c_roles2 := (SELECT COUNT(*) FROM roles);
SET @ms_after := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);
SET @q_evnames := (SELECT COUNT(*) FROM script_nodes n JOIN JSON_TABLE(n.evidence_req,'$[*]' COLUMNS(ev_name VARCHAR(128) PATH '$.name')) j WHERE n.deleted_at IS NULL AND j.ev_name LIKE '%回执%');
SET @q_guide9 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S01-12','S01-13','S01-16','S02-14','S03-8A','S03-15','S05-16','S06-14','S07-14') AND guide LIKE '%由信息科负责人代填，附报送截图/通话记录/接收人确认%');
SET @q_qr24 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S01-8','S01-12','S01-19','S02-8','S02-9','S02-12','S02-14','S03-8A','S03-11','S03-15','S04-7','S04-8','S04-12','S04-13','S04-14','S05-16','S06-6','S06-14','S07-13','S07-14','S08-9','S08-12','S08-14','S08-15') AND JSON_VALID(quality_rules)=1);

SET @guard_counts := IF(@c_nodes2=357 AND @c_corr2=137 AND @c_dist2=220 AND @c_forms2=19 AND @c_scen2=8 AND @c_roles2=15, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_ms := IF(@ms_after<=>@ms_before, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_rows := IF(@rows=66 AND @q_evnames=0 AND @q_guide9=9 AND @q_qr24=24, 1, (SELECT table_name FROM information_schema.tables));

SET @fp_imm_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id IN ('S01-5A','S01-8','S01-12','S01-13','S01-15','S01-16','S01-19','S02-3A','S02-8','S02-9','S02-12','S02-14','S03-3A','S03-8A','S03-11','S03-15','S04-7','S04-8','S04-12','S04-13','S04-14','S05-3A','S05-16','S06-3A','S06-6','S06-7','S06-10','S06-12','S06-14','S07-3A','S07-13','S07-14','S08-3A','S08-9','S08-12','S08-14','S08-15'));
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S01-5A','S01-8','S01-12','S01-13','S01-15','S01-16','S01-19','S02-3A','S02-8','S02-9','S02-12','S02-14','S03-3A','S03-8A','S03-11','S03-15','S04-7','S04-8','S04-12','S04-13','S04-14','S05-3A','S05-16','S06-3A','S06-6','S06-7','S06-10','S06-12','S06-14','S07-3A','S07-13','S07-14','S08-3A','S08-9','S08-12','S08-14','S08-15'));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);

SET @guard_scope := IF(@fp_imm_before<=>@fp_imm_after AND @fp_nodes_before<=>@fp_nodes_after AND @fp_scen_before<=>@fp_scen_after, 1, (SELECT table_name FROM information_schema.tables));
