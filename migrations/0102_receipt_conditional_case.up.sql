-- 0102_receipt_conditional_case.up.sql
-- 条件报案节点口径放开：S04-12 / S08-12 的必含组「报案对象」「报案材料」增加"不报案"路径替代项，
-- 参演人写清"经评估决定不报案"时不再因为没写"市网安大队/报案材料"被扣分。
-- 不动 score / required / deps / flow_type / seq / title / target / receiver / form / timeout / keywords。
-- 不改 0101 已落库内容；S04-14 / S08-15 为 0 分不计分节点，本迁移不修改（仅断言其分值/必填/规则不变）。
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

-- 2 个目标行质量规则原文精确匹配（0101 之后的值）
SET @q_old_qr := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND (
   (id='S04-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案材料","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}')
OR (id='S08-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}')
));

-- 2 个 0 分条件节点："仅在决定报案时执行"，必须保持 score=0 / required=0，不参与扣分
SET @q_zero2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S04-14','S08-15')
   AND COALESCE(score,0)=0 AND COALESCE(`required`,0)=0 AND JSON_VALID(quality_rules)=1);

-- 目标行不可变列指纹（排除 quality_rules）
SET @fp_imm_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id IN ('S04-12','S08-12'));

-- 范围外行全列指纹（含 quality_rules）
SET @fp_out_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S04-12','S08-12'));
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);

SET @guard_pre := IF(@c_nodes=357 AND @c_corr=137 AND @c_dist=220 AND @c_forms=19 AND @c_scen=8 AND @c_roles=15 AND @c_mig=57 AND @c_last='0101_receipt_wording'
  AND @ms_before='S01:112,S02:106,S03:110,S04:99,S05:107,S06:106,S07:105,S08:104'
  AND @q_old_qr=2 AND @q_zero2=2,
  1, (SELECT table_name FROM information_schema.tables));

-- ==================== 二、2 个条件报案节点的必含组放开 ====================
SET @rows := 0;

-- S04-12：组1 增加「不报案|无需报案」；组2 改为「报案材料|不形成报案材料|不报案|无需报案」
-- 组3 保留「…|不报案理由」，不报案路径仍要求写明理由（F19 字段名即"不报案理由"）
UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安|不报案|无需报案","报案材料|不形成报案材料|不报案|无需报案","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}'
 WHERE id='S04-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案材料","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["流程证据","事件经过","代执行"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}';
SET @rows := @rows + ROW_COUNT();

-- S08-12：组1 同上；组2 保留原有「报案」并补齐不报案路径写法（避免报案路径出现新扣分）
UPDATE script_nodes SET quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安|不报案|无需报案","报案|报案材料|不形成报案材料|不报案|无需报案","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}'
 WHERE id='S08-12' AND quality_rules='{"requiredKeywords":["市网安大队|网安大队|公安网安|市公安网安","报案","受理编号|受案编号|案件编号|回执编号|接报人|警号|不报案理由"],"bonusKeywords":["报案条件","证据材料","不等外部意见","后续补报"],"minMeasureLen":30,"minResultLen":18,"requiredElements":["对象","时间"],"fillerWords":["无","已知悉","已阅","收到","按要求处理","处理完毕","同上","ok","OK"],"fillerMaxLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}';
SET @rows := @rows + ROW_COUNT();

-- ==================== 三、后置断言 ====================
SET @c_nodes2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @c_corr2  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct');
SET @c_dist2  := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor');
SET @c_forms2 := (SELECT COUNT(*) FROM form_templates);
SET @c_scen2  := (SELECT COUNT(*) FROM scenarios);
SET @c_roles2 := (SELECT COUNT(*) FROM roles);
SET @ms_after := (SELECT GROUP_CONCAT(CONCAT(scenario_id,':',s) ORDER BY scenario_id SEPARATOR ',') FROM (SELECT scenario_id,SUM(score) AS s FROM script_nodes WHERE deleted_at IS NULL AND flow_type='correct' GROUP BY scenario_id) t);

-- 两行仍是 3 组必含，且组1/组2 都含「不报案」替代项
SET @q_new2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND (
   (id='S04-12' AND JSON_VALID(quality_rules)=1
      AND JSON_LENGTH(JSON_EXTRACT(quality_rules,'$.requiredKeywords'))=3
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[0]') LIKE '%不报案%'
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[1]') LIKE '%不报案%'
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[2]') LIKE '%不报案理由%'
      AND JSON_EXTRACT(quality_rules,'$.keywordPenaltyRatio')=0.3)
OR (id='S08-12' AND JSON_VALID(quality_rules)=1
      AND JSON_LENGTH(JSON_EXTRACT(quality_rules,'$.requiredKeywords'))=3
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[0]') LIKE '%不报案%'
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[1]') LIKE '%不报案%'
      AND JSON_EXTRACT(quality_rules,'$.requiredKeywords[2]') LIKE '%不报案理由%'
      AND JSON_EXTRACT(quality_rules,'$.keywordPenaltyRatio')=0.25)
));

-- 0 分条件节点仍未被动过：score=0 / required=0
SET @q_zero2_after := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND id IN ('S04-14','S08-15')
   AND COALESCE(score,0)=0 AND COALESCE(`required`,0)=0 AND JSON_VALID(quality_rules)=1);

SET @fp_imm_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id IN ('S04-12','S08-12'));
SET @fp_out_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,''),COALESCE(pos_x,-1),COALESCE(pos_y,-1),COALESCE(anchor_id,''),COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,''),COALESCE(receiver_cc,'')) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE id NOT IN ('S04-12','S08-12'));
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(level,''),COALESCE(descr,'')) ORDER BY id SEPARATOR '\n')) FROM scenarios);

SET @guard_counts := IF(@c_nodes2=357 AND @c_corr2=137 AND @c_dist2=220 AND @c_forms2=19 AND @c_scen2=8 AND @c_roles2=15, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_ms := IF(@ms_after<=>@ms_before, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_rows := IF(@rows=2 AND @q_new2=2 AND @q_zero2_after=2, 1, (SELECT table_name FROM information_schema.tables));
SET @guard_scope := IF(@fp_imm_before<=>@fp_imm_after AND @fp_out_before<=>@fp_out_after AND @fp_scen_before<=>@fp_scen_after, 1, (SELECT table_name FROM information_schema.tables));
