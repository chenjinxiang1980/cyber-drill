-- =====================================================================
-- 0107 数据安全演练表单（F22 / F23 / F24）
-- ---------------------------------------------------------------------
-- 编号避让：现网既有 19 张表单（F1–F19），其中 F19 已被 0081 占用为
--   「报案必要性评估单」（10 字段，S04-12 / S08-12 引用）。为确保不与
--   现网及后续人工新增的表单撞号，本迁移把三张新表单整体后移到 F22–F24。
--
--   F22 数据定级与分类分级表 —— 定级节点（性质/时钟/量级/后果/结论）
--   F23 个人信息告知记录表   —— 个人信息泄露告知节点（对象/渠道/时限/失败处理）
--   F24 第三方通知与回执     —— 第三方/供应链数据事件的通知、暂停与整改回执
--
-- 法定时限（裁决 D3）：1 小时/4 小时不做节点自动计时，改为在 F22 内承载
--   「事件性质判定」「法定时钟起算时间」「适用法定报告时限」
--   「是否在法定时限内完成报送（人工判定）」「超时原因与补救措施」五项。
--   F2 是 S01–S08 共用上报表，本次零改动，字段数保持 25。
--
-- 范围与安全：
--   * 只新增 F22–F24 三行，不改任何既有表单（含 F2、F19）；
--   * 写前后对除 F22/F23/F24 外的全部表单做指纹断言；
--   * 不改 script_nodes、scenarios、演练项目与已封存快照；
--   * 纯 DML；INSERT ... ON DUPLICATE KEY UPDATE 内容恒定，可重复执行。
-- 回滚（人工）：
--   DELETE FROM form_templates WHERE id IN ('F22','F23','F24');
-- =====================================================================
SET SESSION group_concat_max_len = 100000000;

-- ===== 前置断言 =====
SET @has_cat := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'scenarios' AND column_name = 'category'
);
SET @last_mig := (SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1);
-- F19 必须仍是被 0081 占用的「报案必要性评估单」，且未被本迁移改动
SET @p_f19 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F19' AND code='F019' AND JSON_VALID(fields)=1 AND JSON_LENGTH(fields)=10
);
-- F2 必须保持 25 字段，且不含本次迁入 F22 的「法定时钟起算时间」
SET @p_f2 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F2' AND JSON_VALID(fields)=1 AND JSON_LENGTH(fields)=25
     AND JSON_SEARCH(fields,'one','法定时钟起算时间',NULL,'$[*].label') IS NULL
);
SET @p_forms := (SELECT COUNT(*) FROM form_templates WHERE id REGEXP '^F[0-9]+$');
SET @p_new := (SELECT COUNT(*) FROM form_templates WHERE id IN ('F22','F23','F24'));
-- 基线：19 张 + 新增 0 张；幂等重跑：22 张 + 新增 3 张。其余状态一律中止。
SET @guard_pre := IF(@has_cat=1 AND @last_mig='0106_scenario_category_backfill'
  AND @p_f19=1 AND @p_f2=1
  AND ((@p_forms=19 AND @p_new=0) OR (@p_forms=22 AND @p_new=3)),
  1, (SELECT table_name FROM information_schema.tables));

-- ===== 写入前指纹（排除本次新增的三张；F2、F19 在指纹内，用于证明未被改动）=====
SET @fp_forms_before := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,code,COALESCE(name,''),COALESCE(fields,''),COALESCE(mode,'')) ORDER BY id SEPARATOR '\n'))
    FROM form_templates WHERE id NOT IN ('F22','F23','F24')
);

-- ---------------------------------------------------------------------
-- A. F22 数据定级与分类分级表（25 字段）
--    定级链条：性质/时钟 → 数据类别/级别 → 敏感个人信息 → 量级 → 系统重要性
--              → 影响后果 → 结论 → 上报与报案判断
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, code, name, fields, mode) VALUES
('F22', 'F022', '数据定级与分类分级表', JSON_ARRAY(
  JSON_OBJECT('label','事件名称','type','text','required',TRUE,
    'hint','与剧本名称、上报表事件名称保持一致。'),
  JSON_OBJECT('label','发现/获悉时间','type','datetime','required',TRUE,
    'hint','填写监测告警、临床科室报告或外部通报的首次发现/获悉时间；法定报告时钟以此为起算点，不得以内部签收或审批时间替代。'),
  JSON_OBJECT('label','首次对外报送时间','type','datetime','required',TRUE,
    'hint','实际向监管或公安首次报送的时间；尚未报送填「未报送」并在依据中说明原因。'),
  JSON_OBJECT('label','事件性质判定','type','select','options',JSON_ARRAY('网络安全事件','数据安全事件','二者兼具'),'required',TRUE,
    'hint','数据泄露、越权导出、违规共享、数据破坏定「数据安全事件」；同时存在入侵、勒索、DDoS 等技术攻击的定「二者兼具」，须分别满足两类报告的时限要求。'),
  JSON_OBJECT('label','法定时钟起算时间','type','datetime','required',TRUE,
    'hint','填写首次发现/获悉时间，不得填写内部签收、审批或领导签发时间；与「发现/获悉时间」不一致时，须在超时原因中说明。'),
  JSON_OBJECT('label','适用法定报告时限','type','select',
    'options',JSON_ARRAY('1 小时','4 小时','及时（按行业规定）','其他'),'required',TRUE,
    'hint','网络安全事件：关键信息基础设施运营者 1 小时内报保护工作部门与公安，其他网络运营者 4 小时内报属地省级网信部门；纯数据安全事件按数据安全法、网数条例及行业规定「及时」报告。'),
  JSON_OBJECT('label','是否在法定时限内完成报送（人工判定）','type','select',
    'options',JSON_ARRAY('是','否','待判定'),'required',TRUE,
    'hint','由评估组按「起算时间 + 适用时限 + 实际报送时间」人工判定；系统不对法定时限做节点自动倒计时。'),
  JSON_OBJECT('label','超时原因与补救措施','type','textarea','required',FALSE,
    'hint','判定为「否」时必填：说明超时原因、补报时间与责任人；未超时填「不适用」。'),
  JSON_OBJECT('label','数据类别','type','select',
    'options',JSON_ARRAY('核心数据','重要数据','一般数据（含个人信息）'),'required',TRUE,
    'hint','按本院数据分类分级清单判定；未完成清单的，先按敏感个人信息与诊疗核心数据从高认定。个人信息一律归入「一般数据（含个人信息）」，敏感个人信息在后续字段单独标注。'),
  JSON_OBJECT('label','数据级别判定依据','type','textarea','required',TRUE,
    'hint','写明所依据的分类分级条目、数据资产名称与判定人，不得只写「按预案」。'),
  JSON_OBJECT('label','是否敏感个人信息','type','select','options',JSON_ARRAY('是','否','待核实'),'required',TRUE,
    'hint','医疗健康、生物识别、身份证号、行踪轨迹、不满十四周岁未成年人信息属敏感个人信息。'),
  JSON_OBJECT('label','涉及个人信息类型','type','multiselect',
    'options',JSON_ARRAY('姓名','身份证号','手机号','病历号','就诊记录','检验检查结果','医学影像','生物识别','医保结算信息','住址'),'required',TRUE,
    'hint','可多选；仅勾选本次已确认涉及的字段类型，未核实的在依据中注明。'),
  JSON_OBJECT('label','涉及数据量级','type','select',
    'options',JSON_ARRAY('50 条以下','50 条及以上 1000 条以下','1000 条及以上 1 万条以下','1 万条及以上 10 万条以下','10 万条及以上'),'required',TRUE,
    'hint','按去重后的个人信息条数统计；量级与定级条款必须落在同一区间，不得出现「6000 条却引用 5 千条以下」的矛盾。'),
  JSON_OBJECT('label','涉及人数（估算）','type','text','required',TRUE,
    'hint','按患者人数去重估算，仅填写演练虚拟数量。'),
  JSON_OBJECT('label','系统重要性','type','select','options',JSON_ARRAY('核心业务系统','重要业务系统','一般业务系统'),'required',TRUE,
    'hint','HIS 收费、电子病历、检验、影像等诊疗核心系统按核心业务系统认定。'),
  JSON_OBJECT('label','影响后果','type','multiselect',
    'options',JSON_ARRAY('患者安全','诊疗中断','经济损失','舆情扩散','监管处罚','数据出境风险'),'required',TRUE,
    'hint','可多选；每项须在定级依据中给出对应事实，不得只勾选不说明。'),
  JSON_OBJECT('label','扩散状态','type','select','options',JSON_ARRAY('未出域','已出域未公开','已公开传播','无法确认'),'required',TRUE,
    'hint','决定是否升级与是否触发个人告知：已公开传播的，按就高原则定级。'),
  JSON_OBJECT('label','成因与主观状态','type','select','options',JSON_ARRAY('外部攻击','内部越权','过失误操作','第三方原因','无法确认'),'required',TRUE,
    'hint','内部越权、牟利出售等情形同时触发刑事报案必要性评估。'),
  JSON_OBJECT('label','定级结论','type','select','options',JSON_ARRAY('特别重大（Ⅰ级）','重大（Ⅱ级）','较大（Ⅲ级）','一般（Ⅳ级）'),'required',TRUE,
    'hint','与 F2「事件等级」、F3「最终定级」、F14 启动申请使用同一枚举；就高定级。'),
  JSON_OBJECT('label','定级依据与条款','type','textarea','required',TRUE,
    'hint','按「事实 → 条款 → 等级」逐条对应，引用的量级区间必须与「涉及数据量级」一致。'),
  JSON_OBJECT('label','是否需上报监管','type','select','options',JSON_ARRAY('是','否'),'required',TRUE,
    'hint','涉及个人信息或核心/重要数据的，一律选「是」；选「否」须在依据中说明理由。'),
  JSON_OBJECT('label','是否需刑事报案','type','select','options',JSON_ARRAY('是','否','待评估'),'required',TRUE,
    'hint','涉嫌非法获取计算机信息系统数据、侵犯公民个人信息、破坏计算机信息系统等犯罪的选「是」。'),
  JSON_OBJECT('label','评估人（信息科/数据安全）','type','text','required',TRUE,
    'hint','填写完成定级的人员姓名与岗位。'),
  JSON_OBJECT('label','会签人（医务处）','type','text','required',TRUE,
    'hint','涉及患者安全或诊疗影响的，由医务处会签。'),
  JSON_OBJECT('label','评估时间','type','datetime','required',TRUE,
    'hint','定级结论形成时间，精确到分钟。')
), 'fill')
ON DUPLICATE KEY UPDATE code=VALUES(code), name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- B. F23 个人信息告知记录表（19 字段）
--    依据个保法第 57 条：立即补救 + 通知监管与个人；无法逐一通知时公告。
--    演练允许「模拟送达」，但必须产生可归档的发送记录与失败处理记录。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, code, name, fields, mode) VALUES
('F23', 'F023', '个人信息告知记录表', JSON_ARRAY(
  JSON_OBJECT('label','告知事由','type','text','required',TRUE,
    'hint','如「第三方运维通道导出患者信息」「内部越权查询病历」，与定级结论一致。'),
  JSON_OBJECT('label','涉及数据字段','type','multiselect',
    'options',JSON_ARRAY('姓名','身份证号','手机号','病历号','就诊记录','检验检查结果','医学影像','生物识别','医保结算信息','住址'),'required',TRUE,
    'hint','按已核实字段勾选，不得扩大或缩小告知范围。'),
  JSON_OBJECT('label','涉及人数（估算）','type','text','required',TRUE,
    'hint','与 F22「涉及人数」一致；仅填写演练虚拟数量。'),
  JSON_OBJECT('label','告知对象范围','type','select',
    'options',JSON_ARRAY('全部受影响个人','高风险个人优先','未成年人监护人','无法逐一通知转公告'),'required',TRUE,
    'hint','高风险优先的须说明筛选标准；无法逐一通知的，必须填写公告渠道与时间。'),
  JSON_OBJECT('label','告知渠道','type','multiselect',
    'options',JSON_ARRAY('短信','电话','院内 APP/公众号','医院官网公告','媒体公告','书面函件','其他'),'required',TRUE,
    'hint','可多选；演练中为模拟发送，须在备注中注明「模拟送达、不触达真实个人」。'),
  JSON_OBJECT('label','告知模板名称/版本','type','text','required',TRUE,
    'hint','填写经法务审核的模板名称与版本号，避免临场自拟话术。'),
  JSON_OBJECT('label','告知内容摘要','type','textarea','required',TRUE,
    'hint','须包含：泄露字段、可能影响、已采取的措施、个人可采取的自护建议、咨询与投诉渠道。'),
  JSON_OBJECT('label','首次发送时间','type','datetime','required',TRUE,
    'hint','以发现/获悉时间为起点计算，不得晚于本院预案规定的告知时限。'),
  JSON_OBJECT('label','计划完成时间','type','datetime','required',TRUE,
    'hint','分批告知时填写最后一批的计划完成时间。'),
  JSON_OBJECT('label','发送成功数','type','text','required',TRUE,
    'hint','填写演练虚拟统计数，与涉及人数口径一致。'),
  JSON_OBJECT('label','发送失败数','type','text','required',TRUE,
    'hint','无失败填「0」；失败数大于 0 时必须填写失败处理方式。'),
  JSON_OBJECT('label','失败处理方式','type','select','options',JSON_ARRAY('重试','电话补告','公告替代','待处理'),'required',TRUE,
    'hint','选择「公告替代」的，须同步填写公告渠道与时间。'),
  JSON_OBJECT('label','公告渠道与时间','type','text','required',FALSE,
    'hint','如「医院官网 2026-11-09 18:00」「市级媒体 2026-11-10 09:00」；未采用公告填「不适用」。'),
  JSON_OBJECT('label','是否评估豁免告知','type','select','options',JSON_ARRAY('是','否'),'required',TRUE,
    'hint','仅在「采取有效措施能够避免信息泄露危害」时评估豁免；豁免须有书面理由，不得以「怕舆情」为由豁免。'),
  JSON_OBJECT('label','豁免理由','type','textarea','required',FALSE,
    'hint','写明豁免依据、评估人与批准人；不豁免填「不适用」。'),
  JSON_OBJECT('label','是否同步告知监管部门','type','select','options',JSON_ARRAY('是','否'),'required',TRUE,
    'hint','与 F2 监管上报节点相互印证；告知个人与报告监管是两项独立义务。'),
  JSON_OBJECT('label','告知记录归档编号','type','text','required',TRUE,
    'hint','与证据归档编号一致，便于复盘时核对发送记录。'),
  JSON_OBJECT('label','责任人','type','text','required',TRUE,
    'hint','患者告知的牵头人与执行人姓名、岗位。'),
  JSON_OBJECT('label','记录时间','type','datetime','required',TRUE,
    'hint','完成本记录的时间，精确到分钟。')
), 'fill')
ON DUPLICATE KEY UPDATE code=VALUES(code), name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- C. F24 第三方通知与回执（17 字段）
--    依据网数条例第 12 条：委托处理须约定数据安全义务、监督受托方、留存处理记录不少于 3 年。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, code, name, fields, mode) VALUES
('F24', 'F024', '第三方通知与回执', JSON_ARRAY(
  JSON_OBJECT('label','第三方/供应商名称','type','text','required',TRUE,
    'hint','填写全称，与合同主体一致；不接受简称或仅写「厂商」。'),
  JSON_OBJECT('label','合作类型','type','select',
    'options',JSON_ARRAY('运维服务','系统集成','数据处理受托方','云服务','科研合作','其他'),'required',TRUE,
    'hint','决定适用条款：受托处理适用委托处理条款，科研合作适用对外提供条款。'),
  JSON_OBJECT('label','合同/协议编号','type','text','required',TRUE,
    'hint','无书面合同的填「无合同」并在整改要求中列为问题。'),
  JSON_OBJECT('label','数据安全条款核对','type','select',
    'options',JSON_ARRAY('已核对且完备','有条款但缺失','无相关条款'),'required',TRUE,
    'hint','核对处理目的、期限、方式、种类、保护措施及双方责任；缺失项写入整改要求。'),
  JSON_OBJECT('label','通知事由与要求','type','textarea','required',TRUE,
    'hint','写明事件事实、涉及数据范围、要求对方立即停止处理、封存日志、配合取证与限期反馈。'),
  JSON_OBJECT('label','通知方式','type','multiselect','options',JSON_ARRAY('书面函件','电子邮件','工单','会议','电话+书面'),'required',TRUE,
    'hint','口头通知后必须补书面；只有口头通知的视为未完成。'),
  JSON_OBJECT('label','通知发出时间','type','datetime','required',TRUE,
    'hint','首次通知时间，精确到分钟。'),
  JSON_OBJECT('label','要求回复时限','type','text','required',TRUE,
    'hint','如「24 小时内书面反馈」「2026-11-10 18:00 前」。'),
  JSON_OBJECT('label','第三方回执/回复摘要','type','textarea','required',TRUE,
    'hint','如实记录对方回复的处置动作、影响范围与时间；未回复的填「未回复」并说明催办情况。'),
  JSON_OBJECT('label','回执时间','type','datetime','required',FALSE,
    'hint','取得书面回执的时间；未取得填「未取得」。'),
  JSON_OBJECT('label','是否暂停数据交互','type','select','options',JSON_ARRAY('是','否'),'required',TRUE,
    'hint','涉及数据泄露或接口滥用的，应先暂停再调查。'),
  JSON_OBJECT('label','暂停/恢复时间','type','text','required',FALSE,
    'hint','如「2026-11-09 11:20 暂停，2026-11-11 09:00 恢复」。'),
  JSON_OBJECT('label','整改要求与期限','type','textarea','required',TRUE,
    'hint','逐条写明整改事项、责任人与完成期限，作为后续供应商考核依据。'),
  JSON_OBJECT('label','是否需向监管报告该第三方事件','type','select','options',JSON_ARRAY('是','否'),'required',TRUE,
    'hint','受托方原因导致个人信息泄露的，数据处理者仍须履行报告与告知义务。'),
  JSON_OBJECT('label','处理记录留存期限','type','text','required',TRUE,
    'hint','不少于 3 年；填写起止时间或「长期」。'),
  JSON_OBJECT('label','责任人','type','text','required',TRUE,
    'hint','本院对接责任人与第三方对接人姓名、岗位。'),
  JSON_OBJECT('label','记录时间','type','datetime','required',TRUE,
    'hint','完成本记录的时间，精确到分钟。')
), 'fill')
ON DUPLICATE KEY UPDATE code=VALUES(code), name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ===== 后置断言 =====
SET @q_f2 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F2' AND JSON_VALID(fields)=1 AND JSON_LENGTH(fields)=25
     AND JSON_SEARCH(fields,'one','法定时钟起算时间',NULL,'$[*].label') IS NULL
);
SET @q_f19 := (
  SELECT COUNT(*) FROM form_templates
   WHERE id='F19' AND code='F019' AND JSON_LENGTH(fields)=10
);
SET @q_new := (
  SELECT COUNT(*) FROM form_templates
   WHERE (id='F22' AND code='F022' AND JSON_LENGTH(fields)=25 AND mode='fill')
      OR (id='F23' AND code='F023' AND JSON_LENGTH(fields)=19 AND mode='fill')
      OR (id='F24' AND code='F024' AND JSON_LENGTH(fields)=17 AND mode='fill')
);
SET @q_valid := (SELECT COUNT(*) FROM form_templates WHERE id IN ('F22','F23','F24') AND JSON_VALID(fields)=1);
SET @q_total := (SELECT COUNT(*) FROM form_templates WHERE id REGEXP '^F[0-9]+$');

SET @fp_forms_after := (
  SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,code,COALESCE(name,''),COALESCE(fields,''),COALESCE(mode,'')) ORDER BY id SEPARATOR '\n'))
    FROM form_templates WHERE id NOT IN ('F22','F23','F24')
);

SET @guard_post := IF(
  @q_f2=1 AND @q_f19=1 AND @q_new=3 AND @q_valid=3 AND @q_total=22
  AND @fp_forms_after <=> @fp_forms_before,
  1, (SELECT table_name FROM information_schema.tables)
);
