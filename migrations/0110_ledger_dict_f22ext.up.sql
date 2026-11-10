-- =====================================================================
-- 0110 台账事件类型字典扩容（8→20）+ F22 扩展（25→29）+ 顺序与引用守卫
-- ---------------------------------------------------------------------
-- 【为什么从 0110 开始】0109_brand_version_087 已被 0.8.7 占用并上线，不得覆盖。
--
-- 【0107/0108 的阻断原因（本迁移头部结论）】
--   0107 的前置断言把 @last_mig 钉死为 '0106_scenario_category_backfill'，
--   0108 又钉死为 '0107_data_security_forms'。0109_brand_version_087 已上线并成为
--   最新版本，且其自身断言要求前序为 0108。由此产生两类阻断：
--     1) 正常升级链：migrate 器按 schema_migrations SKIP，0107/0108 不会重跑；
--     2) 任何人工重跑 0107/0108：@last_mig 已不是各自前序，断言立即失败并回滚。
--   结论：0107/0108 视为“已应用且不可重跑”，B 方案只能以 0110+ 新迁移落地，
--   不得改动 0107/0108 文件本体。
--
-- 【S09–S14 引用检查结论（迁移头部留痕）】
--   projects.scenario_id IN ('S09'…'S14') 必须为 0 才允许后续替换；
--   本迁移以 @guard_pre 强制执行，>0 立即中止，由人工先归档/改挂项目后再重跑。
--   注意：projects 为空表时本守卫恒真（空转放行），**不能**据此认定已验证无引用；
--   现网复验即为空表，真实引用检查须在有数据的库或脱敏快照上另做。
--   0111–0113 据此采用“物理删除旧节点与旧剧本行 + 写新 13 部”的替换方式，并保留回滚语句。
--
-- 【本迁移内容】
--   A. dict.key='eventTypes'：8 类 → 20 类（旧 8 任意项与顺序不变，新 12 追加末尾）；
--   B. F22 数据定级与分类分级表：25 字段 → 29 字段（追加 4 个非必填字段）；
--   C. 顺序断言：@last_mig 必须为 0109_brand_version_087，且 F22 恰为 25 字段、
--      字典恰为 8 类、S09–S14 无项目引用，确保只在正确基线执行一次。
--      —— 0107 的 @last_mig 断言在本迁移应用后必然失败，故 0107 无法重跑覆盖 F22。
--
-- 【安全约束】
--   * 只改 dict 一行与 F22 一行，不触碰 script_nodes、scenarios、projects、snapshots；
--   * 写前后对 S01–S08 全部节点做全字段指纹断言；
--   * 纯 DML 单事务；UPDATE 带状态条件，重复执行不产生二次变更。
-- 回滚（人工）：
--   UPDATE dict SET value=JSON_ARRAY('勒索病毒','内网渗透','DDoS攻击','钓鱼邮件','系统宕机',
--            '数据泄露','边界突破','恶意代码') WHERE `key`='eventTypes';
--   UPDATE form_templates SET fields=JSON_REMOVE(fields,'$[28]','$[27]','$[26]','$[25]')
--     WHERE id='F22' AND JSON_LENGTH(fields)=29;
-- =====================================================================
-- 排序规则：不执行 SET NAMES，避免会话排序规则被改成 utf8mb4_0900_ai_ci，
--           与 script_nodes.id 的 utf8mb4_unicode_ci 冲突（ERROR 1267）。
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
SET @last_mig := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
-- F22 必须仍是 0107 写入的 25 字段
SET @p_f22 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F22' AND code='F022' AND JSON_VALID(fields)=1 AND JSON_LENGTH(fields)=25
);
-- 字典必须仍是旧 8 类，且前两项顺序为「勒索病毒、内网渗透」
SET @p_dict := (
  SELECT COUNT(*) FROM dict
   WHERE `key`='eventTypes' AND JSON_VALID(value)=1 AND JSON_LENGTH(value)=8
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[0]'))='勒索病毒'
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[1]'))='内网渗透'
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[7]'))='恶意代码'
);
-- 引用检查：0108 遗留的 S09–S14 不得被任何项目引用（硬性为 0）
SET @p_ref := (
  SELECT COUNT(*) FROM projects WHERE scenario_id IN ('S09','S10','S11','S12','S13','S14')
);
-- 0108 遗留剧本与节点：6 部、132 个未删除节点（6×(18 正确 + 4 干扰)）；
-- 如果 0108 从未进入本库，则为 0 部 / 0 节点（克隆库验证后未上线的场景）。
-- 两种状态都放行；1–5 部视为半成品残留，直接中止。
SET @p_oldscen := (SELECT COUNT(*) FROM scenarios WHERE id IN ('S09','S10','S11','S12','S13','S14'));
SET @p_oldnodes := (
  SELECT COUNT(*) FROM script_nodes
   WHERE deleted_at IS NULL AND scenario_id IN ('S09','S10','S11','S12','S13','S14')
);
SET @guard_pre := IF(
  @last_mig='0109_brand_version_087'
  AND @p_f22=1 AND @p_dict=1 AND @p_ref=0
  AND ((@p_oldscen=0 AND @p_oldnodes=0) OR (@p_oldscen=6 AND @p_oldnodes=132)),
  1, (SELECT table_name FROM information_schema.tables));

-- ===== 既有剧本写前指纹（S01–S08 全字段，证明零变更）=====
SET @fp_old_before := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),
    COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),
    COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),
    COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),
    COALESCE(score,0),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),
    COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,'')
  ) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE scenario_id IN ('S01','S02','S03','S04','S05','S06','S07','S08')
);

-- =====================================================================
-- A. 台账事件类型字典：8 → 20
--    旧 8 项一字不改、顺序不变；新增 12 项追加在末尾。
--    §5（数据泄露）复用既有「数据泄露」，故只新增 12 项。
-- =====================================================================
UPDATE dict
   SET value = JSON_ARRAY(
     '勒索病毒','内网渗透','DDoS攻击','钓鱼邮件','系统宕机','数据泄露','边界突破','恶意代码',
     '数据篡改','数据假冒','数据窃取','数据拦截','数据投毒','数据滥用','数据损失',
     '社会工程','数据非法收集','位置数据','数据加密勒索','数据权限')
 WHERE `key`='eventTypes' AND JSON_LENGTH(value)=8;

-- =====================================================================
-- B. F22 扩展：追加 4 个非必填字段（25 → 29）
--    非个人信息事件（数据包/样本/文件/中断时长）的计量留痕。
--    required=FALSE：F22 已被 0108 的节点引用且可能已有测试提交，设为必填会
--    让旧节点“突然填不满”而阻断提交；旧提交缺这 4 个 key 时按 label 取值即为空。
--    IF(JSON_LENGTH=25,…)：重复执行不产生二次追加，保持幂等。
-- =====================================================================
UPDATE form_templates
   SET fields = IF(JSON_LENGTH(fields)=25,
     JSON_ARRAY_APPEND(fields, '$',
       JSON_OBJECT('label','非个人信息计量对象','type','select','required',FALSE,
         'options',JSON_ARRAY('业务记录/交易笔数','数据包/会话','样本/模型','文件/备份','账号/权限条目','不适用（个人信息事件）'),
         'hint','仅当计量对象不是自然人条数时填写（如数据包、样本、文件）；个人信息事件选「不适用」。'),
       '$',
       JSON_OBJECT('label','非个人信息计量数量','type','text','required',FALSE,
         'hint','如「数据包 3000 个 / 会话 210 个」「文件 8000 份」；与「涉及数据量级」二选一或并列填写，不得留空又不选「不适用」。'),
       '$',
       JSON_OBJECT('label','业务中断时长','type','select','required',FALSE,
         'options',JSON_ARRAY('无中断','30 分钟以内','30 分钟–1 小时','1–4 小时','4–24 小时','24 小时以上'),
         'hint','以核心业务不可用起算，不含演练组织时间；非个人信息事件按此与「影响后果」共同定级。'),
       '$',
       JSON_OBJECT('label','受影响系统范围','type','text','required',FALSE,
         'hint','填写系统名与关键性，如「HIS/EMR/LIS/PACS 共 4 个核心系统」，与「系统重要性」配合使用。')
     ),
     fields)
 WHERE id='F22' AND JSON_VALID(fields)=1;

-- ===== 后置断言 =====
-- F22 必须恰为 29 字段，且第 26–29 字段分别为新增的 4 项
SET @q_f22 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F22' AND JSON_VALID(fields)=1 AND JSON_LENGTH(fields)=29
     AND JSON_UNQUOTE(JSON_EXTRACT(fields,'$[25].label'))='非个人信息计量对象'
     AND JSON_UNQUOTE(JSON_EXTRACT(fields,'$[26].label'))='非个人信息计量数量'
     AND JSON_UNQUOTE(JSON_EXTRACT(fields,'$[27].label'))='业务中断时长'
     AND JSON_UNQUOTE(JSON_EXTRACT(fields,'$[28].label'))='受影响系统范围'
);
-- 字典必须恰为 20 类，旧 8 项前缀与顺序不变
SET @q_dict := (
  SELECT COUNT(*) FROM dict
   WHERE `key`='eventTypes' AND JSON_VALID(value)=1 AND JSON_LENGTH(value)=20
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[0]'))='勒索病毒'
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[7]'))='恶意代码'
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[8]'))='数据篡改'
     AND JSON_UNQUOTE(JSON_EXTRACT(value,'$[19]'))='数据权限'
);
SET @q_ref := (SELECT COUNT(*) FROM projects WHERE scenario_id IN ('S09','S10','S11','S12','S13','S14'));
-- 既有剧本写后指纹
SET @fp_old_after := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,COALESCE(type,''),COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),
    COALESCE(time,''),COALESCE(timeout,0),COALESCE(form,''),COALESCE(att,''),COALESCE(descr,''),
    COALESCE(branches,''),COALESCE(seq,0),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(quality_rules,''),
    COALESCE(action,''),COALESCE(`wrong`,''),COALESCE(evidence_req,''),COALESCE(`required`,1),COALESCE(flow_type,''),
    COALESCE(score,0),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,''),COALESCE(anchor_id,''),
    COALESCE(release_timing,''),COALESCE(parallel_desc,''),COALESCE(deleted_at,'')
  ) ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE scenario_id IN ('S01','S02','S03','S04','S05','S06','S07','S08')
);

SET @guard_post := IF(
  @q_f22=1 AND @q_dict=1 AND @q_ref=0 AND @fp_old_after <=> @fp_old_before,
  1, (SELECT table_name FROM information_schema.tables));
