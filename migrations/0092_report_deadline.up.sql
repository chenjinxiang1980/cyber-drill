-- =====================================================================
-- 0092 报告时限改为《国家网络安全事件报告管理办法》第四条口径
-- ---------------------------------------------------------------------
-- 依据：2025-11-01 施行的《国家网络安全事件报告管理办法》第四条：
--   * 关键信息基础设施（CII）运营者：发现事件后 1 小时内向保护工作部门、
--     公安机关报告；
--   * 其他网络运营者：发现事件后 4 小时内向属地省级网信部门报告。
-- 本剧本集把市/县级医院核心业务系统设定为已纳入行业保护工作部门清单的
-- CII 运营者（S01/S02/S03/S05/S06/S07）；S04/S08 为Ⅳ级一般事件且未达到
-- “较大以上”，不触发 1 小时/4 小时强制时限，按院内制度 4 小时内报告。
--
-- 同步修改：报告节点 timeout、title/descr/guide/keywords、quality_rules
-- 的 bonusKeywords、干扰项 descr/keywords/evidence_req、场景 descr 的
-- 等级与 CII 判定依据。
-- 纯 DML，可重复执行；守卫失败触发 1242 并回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- 守卫 1：六个 CII 报告节点当前应为 120 分钟，确认改的是预期版本
SET @g_rep := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL
    AND id IN ('S02-8','S02-9','S03-6','S03-7','S07-7','S07-8')
    AND timeout=120);
SET @guard_1 := IF(@g_rep=6, 1, (SELECT table_name FROM information_schema.tables));

-- ===== S02：Ⅲ级（较大），CII 运营者，1 小时 =====
UPDATE script_nodes
SET timeout=60,
    title='向市卫生健康局报送事件（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限，向市卫生健康局报送事件初报；同步准备向市网信办报送，双报可并行、互不阻塞。报送回执不作为后续节点或演练结束的前置条件。',
    guide='进入上报页选择「市卫生健康局」，填写事件类型、等级（Ⅲ级）、影响范围、已采取措施和时限依据（《国家网络安全事件报告管理办法》第四条：CII 1 小时）；提交后进入公安报案与处置并行。',
    keywords='市卫生健康局,市卫健委,上报,初报,1小时'
WHERE id='S02-8' AND scenario_id='S02' AND deleted_at IS NULL;

UPDATE script_nodes
SET timeout=60,
    title='同步报市网信办（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限同步向市网信办报送初报；与市卫生健康局报送并行，不因等待一方回执而延迟另一方。',
    guide='填写事件等级（Ⅲ级）、影响范围、已采取措施与时限依据；网信办接收人按属地口径选择。回执不作为结束前置。',
    keywords='市网信办,1小时,初报,双报'
WHERE id='S02-9' AND scenario_id='S02' AND deleted_at IS NULL;

-- ===== S03：Ⅲ级（较大），收费/HIS 属 CII，1 小时 =====
UPDATE script_nodes
SET timeout=60,
    title='同时报市卫生健康局（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限向市卫生健康局报送初报：事件性质（DDoS）、中断时长、受影响门诊量、已启用兜底措施、下一步处置计划与联系人。',
    guide='使用平台内置《网络安全事件上报表》；时限依据写《国家网络安全事件报告管理办法》第四条 CII 1 小时。',
    keywords='市卫生健康局,1小时,初报,Ⅲ级'
WHERE id='S03-6' AND scenario_id='S03' AND deleted_at IS NULL;

UPDATE script_nodes
SET timeout=60,
    title='同时报市网信办（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限同步向市网信办报送初报：攻击类型（SYN Flood/CC）、峰值流量、源 IP 段特征、已采取的清洗与封禁动作、续报节奏。',
    guide='报送材料使用平台内置《网络安全事件上报表》；不得等公安或运营商结论再报。',
    keywords='市网信办,1小时,初报,双报'
WHERE id='S03-7' AND scenario_id='S03' AND deleted_at IS NULL;

-- ===== S07：Ⅲ级（较大），边界设备/核心网络属 CII，1 小时 =====
UPDATE script_nodes
SET timeout=60,
    title='向市卫生健康局报告（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限向市卫生健康局报告初报：事件等级（Ⅲ级）、边界设备受影响范围、已采取封堵措施与处置进展；与市网信办同一时点双报，不等外部回复。',
    guide='使用 F2 上报表；时限依据写《国家网络安全事件报告管理办法》第四条 CII 1 小时；回执不作为结束前置。',
    keywords='市卫生健康局,1小时,初报,Ⅲ级'
WHERE id='S07-7' AND scenario_id='S07' AND deleted_at IS NULL;

UPDATE script_nodes
SET timeout=60,
    title='同步报市网信办（CII，1 小时内）',
    descr='按 CII 运营者 1 小时时限同步向市网信办报告初报：事件等级（Ⅲ级）、边界设备受影响范围与已采取措施，不等外部回复。',
    guide='使用 F2 上报表；与市卫生健康局报送并行，不得等一方回执再报。',
    keywords='市网信办,1小时,初报,双报'
WHERE id='S07-8' AND scenario_id='S07' AND deleted_at IS NULL;

-- ===== S04：Ⅳ级（一般），不触发 1/4 小时强制时限，院内 4 小时 =====
UPDATE script_nodes
SET descr='发现后立即向市卫生健康局报送初报，最迟不超过 4 小时（院内制度；一般事件不适用《报告管理办法》第四条 1 小时/4 小时强制时限），事件类型填钓鱼邮件、等级一般（Ⅳ级），留存报送回执。',
    keywords='市卫生健康局,4小时,初报,钓鱼邮件,Ⅳ级'
WHERE id='S04-7' AND scenario_id='S04' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='发现后立即同步向市网信办报送初报，最迟不超过 4 小时（院内制度）；与院统一对外口径保持一致，涉及公共传播的口径征询市网信办指导意见，记录报送时间、接收人与回执。',
    keywords='市网信办,4小时,初报,口径一致,回执'
WHERE id='S04-8' AND scenario_id='S04' AND deleted_at IS NULL;

-- ===== S08：Ⅳ级（一般），不触发 1/4 小时强制时限，院内 4 小时 =====
UPDATE script_nodes
SET descr='发现后立即向市卫生健康局报送初报，最迟不超过 4 小时（院内制度；Ⅳ级一般事件不适用《报告管理办法》第四条 1 小时/4 小时强制时限），写明办公网影响范围与业务网未受影响。',
    keywords='市卫生健康局,4小时,初报,Ⅳ级'
WHERE id='S08-7' AND scenario_id='S08' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='发现后立即同步向市网信办报送初报，最迟不超过 4 小时（院内制度）；与院统一对外口径保持一致，记录报送时间、接收人与回执。',
    keywords='市网信办,4小时,初报,口径一致'
WHERE id='S08-8' AND scenario_id='S08' AND deleted_at IS NULL;

-- ===== 干扰项：删除“2 小时法定/续报”错误口径 =====
UPDATE script_nodes
SET descr='整盘镜像可完整保留现场，单机约 30–60 分钟；期间主机保持联网以便复制，加密可能继续扩散。',
    quality_rules=REPLACE(quality_rules,'"1-2小时"','"30-60分钟"')
WHERE id='DSS01-sysadmin-2-2' AND scenario_id='S01' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='仅以“收费中断超 30 分钟”单一条件定为Ⅲ级，未评估影响窗口、门诊量与纸质兜底需求。',
    quality_rules=REPLACE(quality_rules,'"2小时"','"30分钟"'),
    evidence_req=REPLACE(evidence_req,'收费>2 小时中断','收费>30 分钟中断')
WHERE id='DSS03-itlead-3-1' AND scenario_id='S03' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='以网信办意见为前置条件，延误法定 1 小时上报。'
WHERE id='DSS03-itlead-6-1' AND scenario_id='S03' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='只完成首次上报，未按 1 小时节点续报处置进展与恢复情况。'
WHERE id='DSS03-itlead-7-1' AND scenario_id='S03' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='等市网信办意见后再向市卫生健康局上报，延误院内 4 小时上报时限。'
WHERE id='DSS04-itlead-7-1' AND scenario_id='S04' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET guide='初报后须按 1 小时时限续报等级与影响范围。'
WHERE id='DSS07-itlead-7-2' AND scenario_id='S07' AND flow_type='distractor' AND deleted_at IS NULL;

UPDATE script_nodes
SET descr='等市网信办同步后再报卫健，延误院内 4 小时上报时限。'
WHERE id='DSS08-itlead-7-1' AND scenario_id='S08' AND flow_type='distractor' AND deleted_at IS NULL;

-- ===== 兜底：把 quality_rules / keywords 中残留的旧时限词全部替换 =====
-- CII 场景（S02/S03/S07）：旧“2小时/两小时”统一改为 1 小时
UPDATE script_nodes
SET keywords=REPLACE(keywords,'2小时','1小时'),
    quality_rules=REPLACE(REPLACE(quality_rules,'"2小时"','"1小时"'),',"两小时"','')
WHERE scenario_id IN ('S02','S03','S07') AND deleted_at IS NULL
  AND (keywords LIKE '%2小时%' OR quality_rules LIKE '%2小时%' OR quality_rules LIKE '%两小时%');

-- 非 CII 场景（S04/S08）：旧“2小时”统一改为 4 小时
UPDATE script_nodes
SET keywords=REPLACE(keywords,'2小时','4小时'),
    quality_rules=REPLACE(quality_rules,'"2小时"','"4小时"')
WHERE scenario_id IN ('S04','S08') AND deleted_at IS NULL
  AND (keywords LIKE '%2小时%' OR quality_rules LIKE '%2小时%');

-- 场景 descr：写明等级与 CII 判定依据（只追加一次）
UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅱ级（重大）。CII 判定：核心数据库与 HIS/EMR 属核心业务系统，已纳入行业保护工作部门清单，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S01' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅲ级（较大）。CII 判定：核心业务系统与患者数据属 CII 范围，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S02' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅲ级（较大）。CII 判定：门诊收费与 HIS 属核心业务系统，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S03' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅳ级（一般）。CII 判定：钓鱼邮件未扩散、业务未中断，未达到较大以上；不适用《办法》第四条 1 小时/4 小时强制时限，按院内制度 4 小时内报告。')
WHERE id='S04' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅱ级（重大）。CII 判定：HIS/EMR/核心存储属核心业务系统，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S05' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅱ级（重大）。CII 判定：患者个人信息与重要数据属 CII 保护范围，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S06' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅲ级（较大）。CII 判定：边界防火墙与核心网络属 CII 范围，按 CII 运营者执行 1 小时报告时限。')
WHERE id='S07' AND descr NOT LIKE '%CII 判定%';

UPDATE scenarios SET descr=CONCAT(COALESCE(descr,''),' 定级：Ⅳ级（一般）。CII 判定：办公网事件未波及业务网，未达到较大以上；不适用《办法》第四条 1 小时/4 小时强制时限，按院内制度 4 小时内报告。')
WHERE id='S08' AND descr NOT LIKE '%CII 判定%';

-- 守卫 2：六个 CII 报告节点必须已改为 60 分钟
SET @g_rep2 := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL
    AND id IN ('S02-8','S02-9','S03-6','S03-7','S07-7','S07-8')
    AND timeout=60);
SET @guard_2 := IF(@g_rep2=6, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 3：剧本节点中不得再出现“2 小时/2小时/两小时”
SET @g_2h := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL
    AND (CONCAT_WS('|',title,descr,guide,action,quality_rules,evidence_req,keywords,`wrong`) LIKE '%2 小时%'
      OR CONCAT_WS('|',title,descr,guide,action,quality_rules,evidence_req,keywords,`wrong`) LIKE '%2小时%'
      OR CONCAT_WS('|',title,descr,guide,action,quality_rules,evidence_req,keywords,`wrong`) LIKE '%两小时%'));
SET @guard_3 := IF(@g_2h=0, 1, (SELECT table_name FROM information_schema.tables));

-- 守卫 4：八个场景都必须带上 CII 判定说明
SET @g_cii := (SELECT COUNT(*) FROM scenarios WHERE id IN ('S01','S02','S03','S04','S05','S06','S07','S08') AND descr LIKE '%CII 判定%');
SET @guard_4 := IF(@g_cii=8, 1, (SELECT table_name FROM information_schema.tables));
