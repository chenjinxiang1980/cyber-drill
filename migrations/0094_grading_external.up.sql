-- =====================================================================
-- 0094 定级量化阈值/先证据后定级 + 外部单位代填口径
-- ---------------------------------------------------------------------
-- A. S01-5 / S02-3 / S05-3 / S06-3：
--    * 补《网络安全事件分级指南》的量化阈值（业务中断时长、数据量、损失金额）；
--    * 把“先证据后定级”写成可执行步骤（引用已固定证据编号，不等待全量取证）；
--    * S02-3 去掉“数据外泄”这一必须命中的事实性关键词，改为“外泄风险”。
-- B. 外部单位节点（公安/卫健/网信，共 10 个）：
--    * 保留 target 为外部单位，由信息科负责人通过系统“代填”能力填写；
--    * guide 统一改为“代填项（信息科负责人）”，写清代填表单、附件说明、
--      报告/复盘留痕要求；
--    * 确认 roles.external=1，代填校验可通过；
--    * S04-14/S08-15 已在 0093 改为 0 分 0 必做，本迁移不再覆盖。
--
-- 纯 DML，可重复执行；守卫失败触发 1242 并回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- 守卫 1：四个定级节点必须存在
SET @g_nodes := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S01-5','S02-3','S05-3','S06-3'));
SET @guard_1 := IF(@g_nodes=4, 1, (SELECT table_name FROM information_schema.tables));

-- ===== 定级节点：量化阈值 + 先证据后定级 =====
UPDATE script_nodes
SET descr='汇总 S01-2/3/4 的核实、隔离与备份结果，先固定证据（F13 清单、告警截图、备份校验单），再对照《网络安全事件分级指南》量化研判：核心数据库被加密导致 HIS/EMR 主要功能预计中断≥3 小时，或核心业务整体中断≥1 小时，或直接经济损失≥2000 万元，或个人信息≥1000 万人，构成重大（Ⅱ级）；低于上述阈值按较大/一般研判。定级意见须逐条引用证据编号与时间，不以当前已中断时长单独定级。',
    guide='先证据后定级：先完成 S01-2/3/4 并形成 F13 证据清单，再对照阈值；预计恢复时长要有备份校验或厂商评估依据。',
    keywords='定级依据,量化阈值,患者安全,扩散趋势,Ⅱ级'
WHERE id='S01-5' AND scenario_id='S01' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='汇总失陷账号、登录来源、横向痕迹与数据访问范围，先固定登录日志、账号清单与访问记录（S02-11 并行取证），再对照《分级指南》研判：CII 主要功能中断≥30 分钟或整体中断≥10 分钟，或个人信息≥100 万人，或直接经济损失≥500 万元，构成较大（Ⅲ级）；低于上述按一般（Ⅳ级）。数据外泄尚不能证实的，按“外泄风险”与已确认影响定级，不因未证实外泄而拒绝定级或降为Ⅳ级。',
    guide='先证据后定级：引用 S02-2 已固定的账号/日志证据编号；全量取证由 S02-11 并行推进，定级不等待全量取证，但结论必须写明证据来源与未证实事项。',
    keywords='定级依据,量化阈值,Ⅲ级,外泄风险'
WHERE id='S02-3' AND scenario_id='S02' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='依据 HIS 是否全不可用、受影响科室与窗口、是否需要全面纸质兜底，并对照量化阈值：核心业务系统整体中断≥1 小时或主要功能中断≥3 小时，或直接经济损失≥2000 万元，或个人信息≥1000 万人，构成重大（Ⅱ级）；同时核实是否存在勒索加密/人为破坏迹象，不能仅凭硬件故障表象定级。定级前先固定存储日志、告警截图与备份校验结果。',
    guide='先证据后定级：先完成 S05-2 并固定存储日志/告警/备份校验，再对照阈值；“HIS 完全不可用+预计恢复≥3 小时”才按重大，避免先入为主认定纯硬件故障。',
    keywords='研判,分级,Ⅱ级,量化阈值,证据编号'
WHERE id='S05-3' AND scenario_id='S05' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='依据已核实的泄露数据量级与敏感字段（身份证号、病历号、诊断）、扩散速度与舆情热度，对照《分级指南》：个人信息≥1000 万人，或涉及重要数据/核心数据被窃取、篡改、破坏，或造成特别严重社会影响，构成重大（Ⅱ级）；100 万人以上为较大（Ⅲ级）；低于为一般。定级意见须写明数据量统计口径、样本来源与证据编号，不等抓到泄露源头。',
    guide='先证据后定级：先完成 S06-2 的样本与数据量核实，再对照阈值；数据量无法精确统计时按已确认下限+抽样外推区间写明，不得空泛写“重大”。',
    keywords='研判,分级,Ⅱ级,数据量级,证据编号'
WHERE id='S06-3' AND scenario_id='S06' AND deleted_at IS NULL;

-- ===== 外部单位代填：roles.external 收口（幂等） =====
UPDATE roles SET external=1
WHERE id IN ('healthbureau','cac','cyberpolice') AND COALESCE(external,0)<>1;

-- ===== 外部单位代填：guide 统一口径 =====
UPDATE script_nodes
SET guide='代填项（信息科负责人）：由信息科负责人代市卫生健康局填写 F15，记录指导意见要点、续报时限、联系人与接收时间；附本院上报材料与沟通记录。行业指导不等同于响应等级审批，定级仍由院应急指挥部决定；报告/复盘中写明“外部单位未登录系统，由信息科负责人代填”及代填人、时间；本节点不因外部单位未实际操作而扣医院分。'
WHERE id='S01-12' AND scenario_id='S01' AND deleted_at IS NULL;

UPDATE script_nodes
SET guide='代填项（信息科负责人）：由信息科负责人代市网信办填写外部指导意见记录单，记录统一表述、通报要求、反馈时限与联系人；附舆情监测截图与沟通记录。舆情监测属本院宣传/舆情岗位职责，不由网信办承担；报告/复盘中写明“外部单位未登录系统，由信息科负责人代填”及代填人、时间；本节点不因外部单位未实际操作而扣医院分。'
WHERE id='S01-13' AND scenario_id='S01' AND deleted_at IS NULL;

UPDATE script_nodes
SET guide='代填项（信息科负责人）：由信息科负责人代市网安大队填写 F15，不要求外单位登录系统。①按实际沟通填写受理编号、是否立案、承办警官、补充材料与时限；未取得书面回执时“回执形式”选“无回执”，记录口头回执时间与对方姓名/警号；②附本院报案材料、证据目录或通话记录截图；③在事件报告与复盘中写明“外部单位未登录系统，由信息科负责人代填”及代填人、时间；④回执不作为结束前置，本节点不因外部单位未实际操作而扣医院分。'
WHERE id IN ('S01-16','S02-14','S03-15','S05-16','S06-14','S07-14')
  AND deleted_at IS NULL;

-- ===== 条件报案节点：证据名不得占用 core 清单名额 =====
-- buildEndChecklist 会把所有正确节点的证据按名称优先级取前 12 项作为 core；
-- S04 只有 9 项优先级≤6 的证据，S04-14 的“受理回执”会被选入 core，
-- 选择“评估后不报案”时仍会因缺少回执而阻断干净结束。
-- 改为院内总能提供的“报案受理与沟通记录”，名称不再命中“回执”优先级。
UPDATE script_nodes
SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、口头回执时间、对方姓名/警号；未取得书面回执时附沟通记录与代填说明"},{"type":"处置记录","name":"补充材料清单","note":"含补证项、责任人与时限"}]'
WHERE id='S04-14' AND scenario_id='S04' AND deleted_at IS NULL;

UPDATE script_nodes
SET evidence_req='[{"type":"报案材料","name":"报案受理与沟通记录（如已报案）","note":"含受理编号（如有）、口头回执时间、对方姓名/警号；未取得书面回执时附沟通记录与代填说明"},{"type":"处置记录","name":"补充材料要求","note":"含补证项与时限"}]'
WHERE id='S08-15' AND scenario_id='S08' AND deleted_at IS NULL;

-- ===== 报告节点回执：外单位不回复时可用报送凭证归档 =====
UPDATE script_nodes
SET evidence_req='[{"type":"表单文书","name":"网络安全事件上报表","note":"含上报对象、时限依据与签发人"},{"type":"上报材料","name":"上报回执","note":"含接收人、回执编号与时间；未取得回执时附报送截图、通话记录与接收人确认"}]'
WHERE id IN ('S02-8','S02-9') AND scenario_id='S02' AND deleted_at IS NULL;

-- 守卫 2：外部角色必须是 external=1（代填校验前置条件）
SET @g_ext_roles := (SELECT COUNT(*) FROM roles WHERE id IN ('healthbureau','cac','cyberpolice') AND COALESCE(external,0)=1);
SET @guard_2 := IF(@g_ext_roles=3, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 3：十个外部节点都必须写明信息科负责人代填
SET @g_ext_nodes := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S01-12','S01-13','S01-16','S02-14','S03-15','S04-14','S05-16','S06-14','S07-14','S08-15')
    AND guide LIKE '%信息科负责人%');
SET @guard_3 := IF(@g_ext_nodes=10, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 4：四个定级节点必须带量化阈值
SET @g_quant := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S01-5','S02-3','S05-3','S06-3') AND descr LIKE '%≥%');
SET @guard_4 := IF(@g_quant=4, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 5：S02-3 关键词不得再强制“数据外泄”
SET @g_s02kw := (SELECT COUNT(*) FROM script_nodes WHERE id='S02-3' AND keywords LIKE '%数据外泄%');
SET @guard_5 := IF(@g_s02kw=0, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 6：条件报案节点的证据名不得再命中“回执”优先级
SET @g_cond_ev := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S04-14','S08-15')
    AND JSON_SEARCH(evidence_req,'one','%回执%',NULL,'$[*].name') IS NULL);
SET @guard_6 := IF(@g_cond_ev=2, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 7：S02 报告节点回执须允许“未取得回执”的归档口径
SET @g_rep_note := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND id IN ('S02-8','S02-9') AND evidence_req LIKE '%未取得回执%');
SET @guard_7 := IF(@g_rep_note=2, 1, (SELECT table_name FROM information_schema.tables));
