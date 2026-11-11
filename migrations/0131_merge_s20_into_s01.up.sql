-- =====================================================================
-- 0131 合并 S20 到 S01（勒索病毒 / 数据加密勒索，纯 DML）
-- ---------------------------------------------------------------------
-- 【背景】
--   S01「勒索病毒攻击演练」（网络安全演练，Ⅱ级）与 S20「数据加密勒索」
--   （数据安全演练，Ⅰ级）为同一题材：勒索软件加密核心业务数据、业务中断、
--   备份校验、监管上报、报案、恢复、复盘。经逐节点比对后合并为 S01，
--   S20 退役（status=停用），不物理删除，保留行与历史可追溯。
--
-- 【并入（S20 独有且 S01 缺失，4 个正确节点 + 8 个混淆节点）】
--   S01-22 溯源排查：横向移动路径与初始入侵点      ← S20-5   (3 分)
--   S01-23 清除持久化并关闭暴露的运维端口          ← S20-6   (3 分)
--   S01-24 第三方通知：云备份服务商配合恢复与留证  ← S20-14  (3 分)
--   S01-25 患者个人信息告知（模拟送达、真实留痕）  ← S20-15  (3 分)
--   混淆：DSS01-x22-a/b、x23-a/b、x24-a/b、x25-a/b（各 2 条，锚定新节点）
--
-- 【内容吸收（不新增行，只改现有节点正文）】
--   S01-1  云副本删除告警 + 首次发现时间为法定报告时钟起点
--   S01-2  四口径核算（系统/文件数/患者数/中断时长）
--   S01-4  不可变 WORM 副本、离线副本、云副本删除审计、可恢复时点
--   S01-7  四个工作组与对外报告/患者告知/第三方对接授权人
--   S01-11 按临床优先级恢复 + 完整性与一致性校验 + 严禁赎金
--   S01-14 缺口台账（无法补录的原因、影响、补救、科室签字）
--   S01-17 四项结束条件（含患者告知与补录闭环）+ 观察期与续报
--   S01-18 根因（端口暴露/弱口令/无 MFA/备份可同账号删除）+ 硬措施整改
--   S01-20 F22 五维量化口径与「中断>4 小时且涉急诊/手术→Ⅰ级」门槛
--   S01-21 赎金不可作为选项 + 4 小时内回落Ⅱ级旋钮
--
-- 【去重丢弃（判定为与 S01 完全重复，不双份保留）】
--   S20-1/2/3/4/8/9/10/11/12/13/16/17/18/19/20/21/22 及 34 条重复混淆
--   （映射关系见 0131 交付报告）
--
-- 【分值重新平衡（正确节点合计仍为 100）】
--   新增 4 节点各 3 分 = +12；
--   S01-2/3/4/5 由 6 分下调为 3 分 = -12（原四节点为早期核实类，
--   与新增支撑类节点统一为 3 分档）；其余节点分值不变。
--
-- 【补齐混淆下限】
--   S01-18、S01-19、S01-20 原为 0 条混淆，按 S20 内容各补 2 条
--   （DSS01-x18-a/b、x19-a/b、x20-a/b），使全部非 end 正确节点 ≥2 条。
--
-- 【S20 退役方式】
--   UPDATE scenarios SET status='停用'（不删行、不删节点、不软删节点），
--   projects / script_revisions / drill_steps 引用均为 0，退役不断链。
--
-- 【守卫】
--   前置：台账 0130 已应用、S01/S20 均启用、S01 correct=21/dist=34/分值 100、
--         新增 18 个 ID 未被占用、S20 三类引用全 0、S20 节点 64、在用剧本 21。
--   后置：S01 correct=25/dist=48/合计 73/分值 100、全部非 end 正确节点 ≥2 混淆、
--         锚点与依赖全部可解析、时限 5–60 分钟无 300 分钟、六大角色全覆盖、
--         在用剧本 20、S20 停用且节点行数不变。
--   指纹：范围外节点、范围外剧本、S01 既有 55 个节点的结构指纹前后一致。
--   纯 DML 单事务；失败整体回滚。
--
-- 回滚（人工，需先确认无 S20 引用）：
--   DELETE FROM script_nodes WHERE id IN (新增 18 个 ID);
--   UPDATE script_nodes SET score=6 WHERE id IN ('S01-2','S01-3','S01-4','S01-5');
--   UPDATE scenarios SET status='启用', nodes=64, descr=<原值> WHERE id='S20';
--   UPDATE scenarios SET nodes=55 WHERE id='S01';
--   （正文 descr 回滚需按备份恢复）
-- =====================================================================

-- ========== 0. 前置断言 ==========
SET SESSION group_concat_max_len = 10485760;

SET @p_ledger := (SELECT COUNT(*) FROM schema_migrations WHERE version='0130_role_coverage_fill');
SET @p_self := (SELECT COUNT(*) FROM schema_migrations WHERE version='0131_merge_s20_into_s01');
SET @p_s01 := (SELECT COUNT(*) FROM scenarios WHERE id='S01' AND COALESCE(status,'启用')='启用');
SET @p_s20 := (SELECT COUNT(*) FROM scenarios WHERE id='S20' AND COALESCE(status,'启用')='启用');
SET @p_s01_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @p_s01_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='distractor');
SET @p_s01_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @p_id_free := (SELECT COUNT(*) FROM script_nodes WHERE id IN (
  'S01-22','S01-23','S01-24','S01-25',
  'DSS01-x18-a','DSS01-x18-b','DSS01-x19-a','DSS01-x19-b','DSS01-x20-a','DSS01-x20-b',
  'DSS01-x22-a','DSS01-x22-b','DSS01-x23-a','DSS01-x23-b',
  'DSS01-x24-a','DSS01-x24-b','DSS01-x25-a','DSS01-x25-b'));
SET @p_s20_ref := (SELECT (SELECT COUNT(*) FROM projects WHERE scenario_id='S20')
                       + (SELECT COUNT(*) FROM script_revisions WHERE scenario_id='S20')
                       + (SELECT COUNT(*) FROM drill_steps WHERE scenario_id='S20'));
SET @p_s20_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND deleted_at IS NULL);
SET @p_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');

SET @fp_nodes_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(target,''),COALESCE(anchor_id,''),COALESCE(deps,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE (scenario_id IS NULL OR scenario_id NOT IN ('S01','S20')));
SET @fp_scen_out := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(type,''),COALESCE(status,''),COALESCE(nodes,-1),COALESCE(version,''),COALESCE(updated,''),COALESCE(category,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id NOT IN ('S01','S20'));
SET @fp_s01_shape := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(flow_type,''),COALESCE(target,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL);

SET @p_ok := IF(@p_ledger=1 AND @p_self=0 AND @p_s01=1 AND @p_s20=1
  AND @p_s01_correct=21 AND @p_s01_dist=34 AND @p_s01_score=100
  AND @p_id_free=0 AND @p_s20_ref=0 AND @p_s20_nodes=64 AND @p_active=21, 1, NULL);
SELECT IF(@p_ok=1, 1, (SELECT table_name FROM information_schema.tables));

-- ========== 1. 新增 4 个正确节点 ==========
INSERT INTO script_nodes
(id,scenario_id,type,title,short_title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,quality_rules,action,wrong,evidence_req,required,flow_type,score,receiver,receiver_cc,deps,pos_x,pos_y,parallel_desc,anchor_id,release_timing) VALUES
('S01-22','S01','task','溯源排查：横向移动路径与初始入侵点','溯源排查','网络安全管理员','',25,'','',
 '在隔离生效、现场保全的前提下排查入侵链：以远程运维端口登录日志为主线，结合终端进程、计划任务、共享访问与账号登录记录，逐台确认感染主机与被使用账号，输出感染面清单与横向移动路径图，明确初始入侵点（暴露在互联网的远程运维端口，弱口令且未启用双因素）。未完成排查的网段不得放开；排查结论作为清除与整改的依据。',
 '[]',45,'先查登录日志再查进程；跳板机与堡垒机必须纳入排查范围。','溯源,横向移动,初始入侵点,登录日志,跳板机',
 '{"requiredKeywords":["溯源|排查","横向|入侵点","日志"],"bonusKeywords":["跳板","弱口令","双因素","感染面"],"minMeasureLen":40,"minResultLen":20,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已排查","正常","已处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}',
 '取证留痕','[]',
 '[{"type":"分析报告","name":"感染面清单与横向路径图","note":"含主机、账号、路径与跳板"},{"type":"附件","name":"远程运维端口登录日志导出","note":"含时间、源 IP、账号与结果"}]',
 1,'correct',3,'院应急指挥部','','["S01-3"]',NULL,NULL,'','','before'),
('S01-23','S01','task','清除持久化并关闭暴露的运维端口','清除持久化','网络安全管理员','',20,'','',
 '在隔离与取证完成后清除勒索载荷与持久化机制：逐台清除计划任务、服务、启动项与影子账号，关闭暴露在互联网的远程运维端口，改为堡垒机 + 双因素 + 来源 IP 白名单；清除前后各做一次加密行为监测，确认无复发后再进入恢复阶段。未完成取证的机器不得先行清除，清除动作逐台留痕（时间、主机、操作人、清理项）。',
 '[]',55,'先取证后清除；只关端口不清持久化等于没断根。','持久化,计划任务,影子账号,端口收敛,堡垒机,双因素',
 '{"requiredKeywords":["清除|清理","持久化|计划任务|服务","端口"],"bonusKeywords":["影子账号","堡垒机","双因素","白名单"],"minMeasureLen":40,"minResultLen":20,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已清除","正常","已处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}',
 '处置恢复','[]',
 '[{"type":"处置记录","name":"持久化清除记录","note":"含主机、清理项、时间与操作人"},{"type":"配置截图","name":"运维端口收敛与双因素配置","note":"含堡垒机白名单与双因素启用"}]',
 1,'correct',3,'院应急指挥部','','["S01-22"]',NULL,NULL,'','','before'),
('S01-24','S01','task','第三方通知：云备份服务商配合恢复与留证','第三方通知','信息科负责人','',20,'F24','',
 '向云备份服务商发出书面通知：立即冻结涉事账号、保留删除日志与操作审计、协助恢复可用版本、提供副本完整性与留存期限说明，明确 2 小时内响应、24 小时内书面反馈；核对合同中的备份保留期、RTO/RPO 与数据安全条款，留存不少于 3 年。不得只口头沟通，通知与回执归档备查。',
 '[]',125,'第三方是恢复链的一环，必须先冻结再取数；口头沟通不留痕等于没通知。','第三方,云服务商,冻结,留痕,回执',
 '{"requiredKeywords":["通知|函","云|服务商","日志|审计"],"bonusKeywords":["冻结","保留","响应时限","回执"],"minMeasureLen":35,"minResultLen":18,"requiredElements":["对象","动作","结论"],"fillerWords":["无","已通知","正常","已处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.2,"keywordPenaltyRatio":0.3}',
 '对外协调','[]',
 '[{"type":"表单文书","name":"F24 第三方通知与回执","note":"含通知方式、时间、响应时限与回执摘要"},{"type":"函件","name":"书面通知函","note":"加盖公章，注明签收人与送达时间"}]',
 1,'correct',3,'院应急指挥部','','["S01-7"]',NULL,NULL,'','','before'),
('S01-25','S01','task','患者个人信息告知（模拟送达、真实留痕）','个人信息告知','医务处','',30,'F23','',
 '对受影响患者分批告知（模拟送达、真实留痕）：以首次发现时间起算，24 小时内完成首批短信告知，72 小时内完成高风险（急诊、手术、住院）患者电话补告；告知内容包括可能受影响的数据范围、已采取的措施、就诊资料补录安排、个人自护建议与咨询渠道。无法送达的转医院官网公告不少于 30 日；豁免须书面理由并经法务、院办会签。',
 '[]',135,'告知时钟与上报时钟同源；高风险管理患者优先电话补告。','告知,患者,短信,电话,公告,失败处理',
 '{"requiredKeywords":["告知|通知","患者|个人","渠道|短信|电话|公告"],"bonusKeywords":["失败","豁免","72","统计"],"minMeasureLen":45,"minResultLen":20,"requiredElements":["对象","结论"],"fillerWords":["无","已告知","正常","已处理","ok","OK"],"fillerMaxLen":8,"textQualityRatio":0.25,"keywordPenaltyRatio":0.3}',
 '患者沟通','[]',
 '[{"type":"表单文书","name":"F23 个人信息告知记录表","note":"含对象范围、渠道、模板版本、发送成功/失败数与失败处理"},{"type":"发送记录","name":"短信/电话发送统计与公告截图","note":"模拟送达须注明并保留统计底稿"}]',
 1,'correct',3,'院应急指挥部','','["S01-7"]',NULL,NULL,'','','before');

-- ========== 2. 新增 8 条混淆（锚定 S01-22 ~ S01-25，各 2 条） ==========
INSERT INTO script_nodes
(id,scenario_id,type,title,short_title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,quality_rules,action,wrong,evidence_req,required,flow_type,score,receiver,receiver_cc,deps,pos_x,pos_y,parallel_desc,anchor_id,release_timing) VALUES
('DSS01-x22-a','S01','task','只看防火墙告警不查登录日志','只看边界告警','网络安全管理员','',25,'','',
 '排查只翻看防火墙近 24 小时告警，未核查远程运维端口的登录日志，初判入侵点为钓鱼邮件，溯源方向错误，横向移动链条无法还原。',
 '[]',46,'边界告警不覆盖账号与登录行为，必须结合主机与登录日志。','防火墙,登录日志,溯源方向','','取证留痕','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-22','before'),
('DSS01-x22-b','S01','task','只查被加密主机不查跳板','漏查跳板机','网络安全管理员','',25,'','',
 '只在被加密主机上查进程，未排查作为跳板的运维堡垒机，横向移动链条缺关键节点，感染面清单不完整，责任边界无法界定。',
 '[]',47,'跳板与堡垒机往往是同一条入侵链的必经节点。','跳板,堡垒机,感染面','','取证留痕','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-22','before'),
('DSS01-x23-a','S01','task','只关端口不清持久化任务','只关端口','网络安全管理员','',20,'','',
 '关闭暴露的远程运维端口后即认为入侵通道已断，未清理主机上的计划任务、服务、启动项与影子账号，48 小时后加密进程再次触发。',
 '[]',56,'端口只是入口，持久化机制不清理等于没断根。','端口,计划任务,持久化','','处置恢复','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-23','before'),
('DSS01-x23-b','S01','task','只改口令不限制运维来源 IP','只改口令','网络安全管理员','',20,'','',
 '把运维账号口令全部重置，但未限制访问来源 IP 与启用双因素，攻击者用窃取的凭证从境外 IP 重新登录成功。',
 '[]',57,'改口令不等于控入口，双因素与来源白名单必须同时上。','口令,双因素,来源IP','','处置恢复','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-23','before'),
('DSS01-x24-a','S01','task','只口头让云商配合不签授权','只口头沟通','信息科负责人','',20,'','',
 '只电话要求云备份服务商开放快照读取，未签取证配合授权与数据保密协议，云商拒绝提供多个时间点快照，恢复推迟 5 小时。',
 '[]',126,'第三方配合需要书面授权与责任边界，口头沟通无法追责。','口头,授权,云服务商','','对外协调','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-24','before'),
('DSS01-x24-b','S01','task','让云商先重置实例再取快照','先重置再取证','信息科负责人','',20,'','',
 '要求云服务商先重置受影响实例再提供快照，重置后系统盘被覆盖，原始加密现场与引导记录丢失，取证价值大幅下降。',
 '[]',127,'重置会覆盖现场，必须先冻结、再取快照、后恢复。','重置,快照,现场','','对外协调','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-24','before'),
('DSS01-x25-a','S01','task','患者告知等系统全部恢复后统一发送','延后告知','医务处','',30,'','',
 '认为当前最紧急的是恢复系统，患者告知可以等全部系统恢复后再统一发送，避免多批次打扰患者；告知时限以发现时间起算，延后即违规。',
 '[]',136,'告知时钟不因恢复进度暂停；分批告知是被允许的，延后不是。','延后,统一发送,告知时限','','患者沟通','[]','[]',0,'distractor',3,'','','[]',NULL,NULL,'','S01-25','before'),
('DSS01-x25-b','S01','task','只在院内网发通知不触达患者','只在院内网公告','医务处','',30,'','',
 '把告知内容挂在内网公告栏，未通过短信、公众号与电话触达受影响患者，3 天后仅少数人看到，告知义务未履行。',
 '[]',137,'内网公告不构成对患者的有效告知，必须走可触达渠道并留痕。','内网,公告,触达','','患者沟通','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-25','before');

-- ========== 3. 补齐 S01-18/19/20 的混淆下限（原为 0 条） ==========
INSERT INTO script_nodes
(id,scenario_id,type,title,short_title,target,time,timeout,form,att,descr,branches,seq,guide,keywords,quality_rules,action,wrong,evidence_req,required,flow_type,score,receiver,receiver_cc,deps,pos_x,pos_y,parallel_desc,anchor_id,release_timing) VALUES
('DSS01-x18-a','S01','task','复盘只写加强防护意识与购买杀毒软件','复盘空转','院应急指挥部','',60,'','',
 '复盘结论停留在加强防护意识与购买杀毒软件，未落地远程端口收敛、双因素与不可变备份三项硬措施，6 个月后同网段再现加密。',
 '[]',181,'复盘要落到能否被加密、能否快速恢复两条主线。','复盘,空泛,整改','','研判分级','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-18','before'),
('DSS01-x18-b','S01','task','只做攻防演练不改恢复目标','不改恢复目标','院应急指挥部','',60,'','',
 '组织 1 次攻防演练但未修订 RTO/RPO、未按核心系统分别设定恢复优先级，下次中断仍按临时顺序恢复。',
 '[]',182,'演练不改指标等于没改能力，RTO/RPO 要逐系统落。','RTO,RPO,恢复优先级','','研判分级','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-18','before'),
('DSS01-x19-a','S01','report','只报卫健局不报网信办','漏报网信办','信息科负责人','',60,'','',
 '认为勒索属卫生行业事件，只向市卫生健康局报告，未向市网信办报送，网络安全主管部门在舆情发酵后才介入。',
 '[]',161,'双报是并行义务，不能以行业主管替代网络安全主管。','漏报,网信办,双报','','对外上报','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-19','before'),
('DSS01-x19-b','S01','report','上报等恢复完成再补报细节','首报不完整','信息科负责人','',60,'','',
 '首次上报只写系统中断，未写被加密文件数、受影响患者数与勒索事实，承诺恢复后再补报，监管认定首报不完整。',
 '[]',162,'首报必须写清性质、范围与量级，细节后补不等于可以空报。','首报,要素,补报','','对外上报','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-19','before'),
('DSS01-x20-a','S01','task','按加密文件数套阈值定Ⅲ级','定级偏低','信息科负责人','',15,'','',
 '以被加密文件数、未直接泄露个人信息为由定Ⅲ级，未计入核心系统中断时长与急诊、手术受影响的后果，定级偏低。',
 '[]',201,'定级按后果与患者安全就高，不能只看数据条数。','定级,阈值,后果','','研判分级','[]','[]',0,'distractor',8,'','','[]',NULL,NULL,'','S01-20','before'),
('DSS01-x20-b','S01','task','压低中断时长以规避升级','压低时长','信息科负责人','',15,'','',
 '定级时把实际中断时长写短以规避更高级别条款，与业务日志、急诊登记记录矛盾，监管复核认定为瞒报。',
 '[]',202,'中断时长有日志与登记可复核，压低即瞒报。','中断时长,瞒报,复核','','研判分级','[]','[]',0,'distractor',5,'','','[]',NULL,NULL,'','S01-20','before');

-- ========== 4. 内容吸收：用 S20 更完整的表述升级现有节点正文 ==========
UPDATE script_nodes SET descr='巡检发现 HIS 数据库服务器 db-his-01 数据文件扩展名被改为 .locked，数据库服务无法启动；门诊、收费、检验、影像等前端随即批量报「无法连接数据库」，业务中断。随后部分医护工作站与共享盘出现勒索弹窗与勒索信（要求 48 小时内支付赎金），备份平台同时告警一份云副本被删除。值班员须记录首次发现时间——该时间是法定报告的时钟起点——并立即通知信息科负责人、网络安全管理员与院总值班。'
 WHERE id='S01-1' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='15 分钟内确认 HIS/LIS/PACS 数据库服务器加密范围与最近可用备份，再统计前端不可用清单；同步按四口径形成核算表：受影响系统（HIS/EMR/LIS/PACS）、被加密文件数、涉及患者数（按日均门急诊量 × 中断天数估算）、业务中断时长，写明估算方法与数据来源，供定级与告知范围使用。终端勒索弹窗作为次生现象一并登记。'
 WHERE id='S01-2' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='先关闭共享目录写入，再隔离已加密与高置信主机；按 VLAN/网段关闭核心业务区与办公区之间的横向通道，暂停远程桌面与共享访问；HIS/EMR 核心服务器只做 ACL 降权，不直接断网。仅保留取证与恢复所需的受控通道，隔离范围、时间与受影响业务逐项记录；不得为「先恢复业务」保留已被感染的通道。'
 WHERE id='S01-3' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='核对最近 3 个恢复点时间与大小，确认 RPO；抽 1 个非核心库验证可读，登记不可用备份。逐项核查本地备份、异地容灾与对象存储版本化副本，确认是否存在未被加密的不可变（WORM）副本与离线副本；对云副本被删除的情况核实删除时间、操作账号与能否按版本恢复。输出可用备份清单与「可恢复至哪一时点、最多丢失多少数据」，不得在未校验前覆盖生产库。'
 WHERE id='S01-4' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='批准启动Ⅱ级：明确响应级别、启动时间、授权范围（停机、断网、数据恢复、对外上报），形成启动令。同步成立院级应急指挥部，下设技术处置、医疗救治、患者沟通、对外报告四个组，明确对外报告授权人、患者告知牵头人与第三方（云服务商等）对接人；授权记录载明时间与范围。'
 WHERE id='S01-7' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='在备份校验通过、隔离完成、载荷清除后按临床优先级恢复：先 HIS 挂号收费与医嘱执行，再 LIS/PACS，最后 EMR 历史病历查询；每恢复一个系统先做只读验证，确认无再次加密再开放写。恢复后做完整性与一致性校验：收费流水与财务对账、医嘱与执行记录、检验结果与报告单、影像与报告关联，抽样比对记录数与时间戳，由医务处与财务处共同签字；未通过不得宣布恢复。记录 RPO、RTO、未恢复系统；严禁支付赎金换取解密工具。'
 WHERE id='S01-11' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='先补当日挂号收费与发药记录，双人复核；差异逐笔登记，限时封账，补录范围与稽核规则留痕。中断期间的急诊分诊、手工医嘱、检验应急报告、手术记录与收费凭证按科室清单逐项补录；无法补录的登记为数据缺口，注明原因、影响范围与补救措施，由科室负责人签字确认，形成缺口台账备查。'
 WHERE id='S01-14' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='结束条件逐项核对：业务恢复并完成验证、核心证据已固定、向市卫生健康局与市网信办双报完成、患者告知与数据补录已闭环（含失败与缺口处理）。由院应急指挥部宣布Ⅱ级响应结束，明确观察期、监管续报安排与后续加固责任，结束决定书面留存并经指挥部批准。公安受案/立案回执不作为演练结束前置条件。'
 WHERE id='S01-17' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='由院应急指挥部牵头，信息科、医务处、财务处等院内部门及市卫生健康局、市网信办、市网安大队等外单位参加（外单位只读观摩）；形成问题清单、整改责任人与时限，归档演练记录。复盘须写明根因（远程运维端口暴露、弱口令、未启用双因素、备份副本可被同一账号删除、缺乏不可变备份）、损失评估（中断时长、文件数、患者数）与硬措施整改（端口收敛、堡垒机 + 双因素、备份 3-2-1 与不可变副本、异地容灾演练、终端 EDR），整改纳入下一轮检查。'
 WHERE id='S01-18' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='复核 S01-5 研判结论：出现勒索加密扩散至核心业务库、备份文件被加密、恢复点不可用、患者数据外泄或舆情扩散等情形，按院预案上一级标准重新定级为Ⅰ级；量化口径按 F22 五维——核心业务中断 > 4 小时且涉及急诊/手术即构成特别重大（Ⅰ级），敏感个人信息按 R2 档位、后果按 R4 就高。报院应急指挥部批准后补报并按新等级续报；未达到升级条件时明确维持Ⅱ级及观察时限。判定须引用影响系统、数据、患者安全、扩散趋势四类依据。'
 WHERE id='S01-20' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

UPDATE script_nodes SET descr='总结关键判断：法定上报时限以发现时间起算；先隔离断网再固定证据；备份恢复前必须抽验可读，并确认存在不可变/离线副本；业务兜底覆盖六大环节，患者安全兜底优先于数据恢复；结束需同时满足四项条件并经指挥部批准；赎金绝不可作为选项。Ⅰ级判定由中断时长与患者安全触发，保留「中断压至 4 小时内回落Ⅱ级」的旋钮。'
 WHERE id='S01-21' AND scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct';

-- ========== 5. 分值重新平衡（正确节点合计保持 100） ==========
UPDATE script_nodes SET score=3 WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct' AND id IN ('S01-2','S01-3','S01-4','S01-5');

-- ========== 6. 计数与更新日期 ==========
UPDATE scenarios SET nodes=73, updated='2026-11-11' WHERE id='S01';

-- ========== 7. S20 退役（停用，保留行与节点，不物理删除） ==========
UPDATE scenarios SET status='停用', updated='2026-11-11',
  descr=CONCAT('【已退役：内容已并入 S01「勒索病毒攻击演练」，仅保留历史追溯，不再用于新建演练项目】', COALESCE(descr,''))
 WHERE id='S20';

-- ========== 8. 后置断言 ==========
SET @q_s01_correct := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @q_s01_dist := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='distractor');
SET @q_s01_score := (SELECT COALESCE(SUM(score),0) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND flow_type='correct');
SET @q_s01_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL);
SET @q_scen_nodes := (SELECT COALESCE(nodes,-1) FROM scenarios WHERE id='S01');
SET @q_active := (SELECT COUNT(*) FROM scenarios WHERE COALESCE(status,'启用')='启用');
SET @q_s20_retired := (SELECT COUNT(*) FROM scenarios WHERE id='S20' AND status='停用');
SET @q_s20_nodes := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S20' AND deleted_at IS NULL);
SET @q_bad_dist := (SELECT COUNT(*) FROM script_nodes c
  WHERE c.scenario_id='S01' AND c.deleted_at IS NULL AND c.flow_type='correct' AND c.type<>'end'
    AND (SELECT COUNT(*) FROM script_nodes d WHERE d.scenario_id='S01' AND d.deleted_at IS NULL AND d.flow_type='distractor' AND d.anchor_id=c.id) < 2);
SET @q_bad_anchor := (SELECT COUNT(*) FROM script_nodes d
  WHERE d.scenario_id='S01' AND d.deleted_at IS NULL AND d.flow_type='distractor'
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S01' AND c.deleted_at IS NULL AND c.flow_type='correct' AND c.id=d.anchor_id));
SET @q_bad_dep := (SELECT COUNT(*) FROM script_nodes n
  JOIN (SELECT 0 i UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3
        UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7) idx
    ON idx.i < JSON_LENGTH(n.deps)
  WHERE n.scenario_id='S01' AND n.deleted_at IS NULL
    AND n.deps IS NOT NULL AND n.deps<>'' AND n.deps<>'[]'
    AND NOT EXISTS (SELECT 1 FROM script_nodes c WHERE c.scenario_id='S01' AND c.deleted_at IS NULL
                      AND c.id = JSON_UNQUOTE(JSON_EXTRACT(n.deps, CONCAT('$[',idx.i,']')))));
SET @q_bad_timeout := (SELECT COUNT(*) FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND (timeout IS NULL OR timeout<5 OR timeout>120));
SET @q_roles := (SELECT COUNT(DISTINCT target) FROM script_nodes
  WHERE scenario_id='S01' AND deleted_at IS NULL
    AND target IN ('信息科值班员','网络安全管理员','数据安全管理员','系统管理员','信息科负责人','院应急指挥部'));

SET @fp_nodes_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(scenario_id,''),COALESCE(flow_type,''),COALESCE(score,-1),COALESCE(target,''),COALESCE(anchor_id,''),COALESCE(deps,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(release_timing,''),COALESCE(deleted_at,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE (scenario_id IS NULL OR scenario_id NOT IN ('S01','S20')));
SET @fp_scen_out2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(name,''),COALESCE(type,''),COALESCE(status,''),COALESCE(nodes,-1),COALESCE(version,''),COALESCE(updated,''),COALESCE(category,'')) ORDER BY id SEPARATOR '#'),256)
  FROM scenarios WHERE id NOT IN ('S01','S20'));
SET @fp_s01_shape2 := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(flow_type,''),COALESCE(target,''),COALESCE(required,-1),COALESCE(timeout,-1),COALESCE(seq,-1),COALESCE(deps,''),COALESCE(anchor_id,''),COALESCE(release_timing,'')) ORDER BY id SEPARATOR '#'),256)
  FROM script_nodes WHERE scenario_id='S01' AND deleted_at IS NULL AND id NOT IN (
    'S01-22','S01-23','S01-24','S01-25',
    'DSS01-x18-a','DSS01-x18-b','DSS01-x19-a','DSS01-x19-b','DSS01-x20-a','DSS01-x20-b',
    'DSS01-x22-a','DSS01-x22-b','DSS01-x23-a','DSS01-x23-b',
    'DSS01-x24-a','DSS01-x24-b','DSS01-x25-a','DSS01-x25-b'));

SET @q_ok := IF(@q_s01_correct=25 AND @q_s01_dist=48 AND @q_s01_nodes=73 AND @q_s01_score=100
  AND @q_scen_nodes=73 AND @q_active=20 AND @q_s20_retired=1 AND @q_s20_nodes=64
  AND @q_bad_dist=0 AND @q_bad_anchor=0 AND @q_bad_dep=0 AND @q_bad_timeout=0 AND @q_roles=6
  AND @fp_nodes_out2=@fp_nodes_out AND @fp_scen_out2=@fp_scen_out AND @fp_s01_shape2=@fp_s01_shape, 1, NULL);
SELECT IF(@q_ok=1, 1, (SELECT table_name FROM information_schema.tables));
