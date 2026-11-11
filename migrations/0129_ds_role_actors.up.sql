-- =====================================================================
-- 0129 补齐「数据安全管理员」参与度（第一批，纯 DML）
-- ---------------------------------------------------------------------
-- 背景：21 个剧本要求覆盖六大必需角色。S02/S03/S04/S07/S08 缺
--       「数据安全管理员」（role_mvnaewi63am）。
-- 处理：只改责任角色，不动标题、时限、依赖、分值、流程类型与锚点。
--       每个承接点连同其锚定混淆节点一起改角色 —— 混淆节点是该节点的
--       错误选项，归属角色必须与节点一致，否则该角色在演练里既没有
--       正确选项也没有对应干扰项。
-- 逐项说明（原角色 → 新角色｜依据）：
--   S02-11 取证与日志审计 网安→数安｜固定登录/堡垒机/数据访问日志，属数据侧审计与证据登记
--     DSS02-netsec-11-1 只保存登录日志… 网安→数安
--     DSS02-netsec-11-2 先恢复业务，取证留描述… 网安→数安
--   S03-12 业务恢复验证与数据核对 系统→数安｜交易流水逐笔核对、漏费错费清单，属数据一致性核验
--     DSS03-sysadmin-12-1 让窗口试刷做冒烟验证… 系统→数安
--     DSS03-sysadmin-12-2 直接上线跑… 系统→数安
--   S03-8  启用院内清洗策略与限速 网安→系统管理员｜策略下发、限速配置与业务可用性保障为系统侧执行；攻击研判、证据保全、溯源仍归网安（S03-2/S03-14/S03-10）
--     DSS03-netsec-8-1 直接封禁带宽最高 IP… 网安→系统
--     DSS03-netsec-8-2 让业务走备用线路… 网安→系统
--   S04-11 取证邮件样本与网关日志 网安→数安｜邮件网关投递日志、点击名单与凭据泄露范围属数据侧证据
--     DSS04-netsec-11-1 只保存钓鱼邮件样本… 网安→数安
--     DSS04-netsec-11-2 先处置业务，取证留描述… 网安→数安
--   S07-9  排查内网是否被横向利用 网安→数安｜内容为数据库与文件服务器外传痕迹、数据是否外传
--     DSS07-netsec-9-1 只查登录日志，不查外联与数据库外传 网安→数安
--     DSS07-netsec-9-2 等防火墙改回后再查内网 网安→数安
--   S08-11 取证样本与传播链 网安→数安｜共享盘访问日志与数据传播链固定属数据侧证据
--     DSS08-netsec-11-1 先处置完业务再说取证 网安→数安
--     DSS08-netsec-11-2 让厂商帮忙取证 网安→数安
-- 写法：前置断言 + 单事务（由 migrate 执行器保证）+ 范围外行指纹 +
--       后置断言；守卫失败触发 1242 回滚。
-- =====================================================================
SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 10485760;

-- ===== 前置断言 =====
SET @p_roles := (SELECT COUNT(*) FROM roles);
SET @p_required := (SELECT COUNT(*) FROM roles
  WHERE id IN ('duty','netsec','role_mvnaewi63am','sysadmin','itlead','commander'));
SET @p_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);

-- 18 个承接行（6 个流程节点 + 12 个混淆节点）必须仍处于迁移前原角色
SET @p_hit := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND (
       (id='S02-11' AND target='网络安全管理员')
    OR (id='S03-12' AND target='系统管理员')
    OR (id='S03-8'  AND target='网络安全管理员')
    OR (id='S04-11' AND target='网络安全管理员')
    OR (id='S07-9'  AND target='网络安全管理员')
    OR (id='S08-11' AND target='网络安全管理员')
    OR (id IN ('DSS02-netsec-11-1','DSS02-netsec-11-2')           AND target='网络安全管理员')
    OR (id IN ('DSS03-sysadmin-12-1','DSS03-sysadmin-12-2')       AND target='系统管理员')
    OR (id IN ('DSS03-netsec-8-1','DSS03-netsec-8-2')             AND target='网络安全管理员')
    OR (id IN ('DSS04-netsec-11-1','DSS04-netsec-11-2')           AND target='网络安全管理员')
    OR (id IN ('DSS07-netsec-9-1','DSS07-netsec-9-2')             AND target='网络安全管理员')
    OR (id IN ('DSS08-netsec-11-1','DSS08-netsec-11-2')           AND target='网络安全管理员')));

SET @p_score := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',scenario_id,correct_sum) ORDER BY scenario_id SEPARATOR '\n'), 256)
  FROM (SELECT scenario_id,
          SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS correct_sum
        FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t);

SET @guard_pre := IF(@p_roles=18 AND @p_required=6 AND @p_nodes=1173 AND @p_hit=18, 1,
  (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写前） =====
SET @fp_roles_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'), 256) FROM roles);
SET @fp_nodes_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes
  WHERE deleted_at IS NULL AND id NOT IN (
    'S02-11','S03-12','S03-8','S04-11','S07-9','S08-11',
    'DSS02-netsec-11-1','DSS02-netsec-11-2',
    'DSS03-sysadmin-12-1','DSS03-sysadmin-12-2',
    'DSS03-netsec-8-1','DSS03-netsec-8-2',
    'DSS04-netsec-11-1','DSS04-netsec-11-2',
    'DSS07-netsec-9-1','DSS07-netsec-9-2',
    'DSS08-netsec-11-1','DSS08-netsec-11-2'));
SET @fp_scen_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
-- 范围内行的结构字段（流程类型/必答/分值/锚点/依赖）必须原样不变
SET @fp_struct_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,flow_type,required,score,COALESCE(anchor_id,'-'),COALESCE(deps,'-'),COALESCE(timeout,0),COALESCE(seq,0))
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes WHERE deleted_at IS NULL AND id IN (
    'S02-11','S03-12','S03-8','S04-11','S07-9','S08-11',
    'DSS02-netsec-11-1','DSS02-netsec-11-2',
    'DSS03-sysadmin-12-1','DSS03-sysadmin-12-2',
    'DSS03-netsec-8-1','DSS03-netsec-8-2',
    'DSS04-netsec-11-1','DSS04-netsec-11-2',
    'DSS07-netsec-9-1','DSS07-netsec-9-2',
    'DSS08-netsec-11-1','DSS08-netsec-11-2'));

-- ===== 责任角色调整 =====
UPDATE script_nodes SET target='数据安全管理员'
  WHERE deleted_at IS NULL AND id IN (
    'S02-11','S03-12','S04-11','S07-9','S08-11',
    'DSS02-netsec-11-1','DSS02-netsec-11-2',
    'DSS03-sysadmin-12-1','DSS03-sysadmin-12-2',
    'DSS04-netsec-11-1','DSS04-netsec-11-2',
    'DSS07-netsec-9-1','DSS07-netsec-9-2',
    'DSS08-netsec-11-1','DSS08-netsec-11-2');
UPDATE script_nodes SET target='系统管理员'
  WHERE deleted_at IS NULL AND id IN ('S03-8','DSS03-netsec-8-1','DSS03-netsec-8-2');

-- ===== 后置断言 =====
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @q_hit := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND (
       (id IN ('S02-11','S04-11','S07-9','S08-11','S03-12') AND target='数据安全管理员')
    OR (id IN ('S03-8') AND target='系统管理员')
    OR (id IN ('DSS02-netsec-11-1','DSS02-netsec-11-2')     AND target='数据安全管理员')
    OR (id IN ('DSS03-sysadmin-12-1','DSS03-sysadmin-12-2') AND target='数据安全管理员')
    OR (id IN ('DSS03-netsec-8-1','DSS03-netsec-8-2')       AND target='系统管理员')
    OR (id IN ('DSS04-netsec-11-1','DSS04-netsec-11-2')     AND target='数据安全管理员')
    OR (id IN ('DSS07-netsec-9-1','DSS07-netsec-9-2')       AND target='数据安全管理员')
    OR (id IN ('DSS08-netsec-11-1','DSS08-netsec-11-2')     AND target='数据安全管理员')));

-- 这五个剧本（含混淆节点）改后六个必需角色都要在
SET @q_cov := (SELECT COUNT(*) FROM (
    SELECT sc.id,
      MAX(n.target=(SELECT name FROM roles WHERE id='duty')) AS duty,
      MAX(n.target=(SELECT name FROM roles WHERE id='netsec')) AS netsec,
      MAX(n.target=(SELECT name FROM roles WHERE id='role_mvnaewi63am')) AS datasec,
      MAX(n.target=(SELECT name FROM roles WHERE id='sysadmin')) AS sysadmin,
      MAX(n.target=(SELECT name FROM roles WHERE id='itlead')) AS itlead,
      MAX(n.target=(SELECT name FROM roles WHERE id='commander')) AS commander
    FROM scenarios sc LEFT JOIN script_nodes n
      ON n.scenario_id=sc.id AND n.deleted_at IS NULL
    WHERE sc.id IN ('S02','S03','S04','S07','S08')
    GROUP BY sc.id) c
  WHERE c.duty=1 AND c.netsec=1 AND c.datasec=1 AND c.sysadmin=1 AND c.itlead=1 AND c.commander=1);

SET @q_score := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',scenario_id,correct_sum) ORDER BY scenario_id SEPARATOR '\n'), 256)
  FROM (SELECT scenario_id,
          SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS correct_sum
        FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t);
SET @q_score100 := (SELECT COUNT(*) FROM (
    SELECT scenario_id, SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS c
    FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t WHERE t.c=100);

SET @fp_roles_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'), 256) FROM roles);
SET @fp_nodes_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes
  WHERE deleted_at IS NULL AND id NOT IN (
    'S02-11','S03-12','S03-8','S04-11','S07-9','S08-11',
    'DSS02-netsec-11-1','DSS02-netsec-11-2',
    'DSS03-sysadmin-12-1','DSS03-sysadmin-12-2',
    'DSS03-netsec-8-1','DSS03-netsec-8-2',
    'DSS04-netsec-11-1','DSS04-netsec-11-2',
    'DSS07-netsec-9-1','DSS07-netsec-9-2',
    'DSS08-netsec-11-1','DSS08-netsec-11-2'));
SET @fp_scen_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
SET @fp_struct_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,flow_type,required,score,COALESCE(anchor_id,'-'),COALESCE(deps,'-'),COALESCE(timeout,0),COALESCE(seq,0))
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes WHERE deleted_at IS NULL AND id IN (
    'S02-11','S03-12','S03-8','S04-11','S07-9','S08-11',
    'DSS02-netsec-11-1','DSS02-netsec-11-2',
    'DSS03-sysadmin-12-1','DSS03-sysadmin-12-2',
    'DSS03-netsec-8-1','DSS03-netsec-8-2',
    'DSS04-netsec-11-1','DSS04-netsec-11-2',
    'DSS07-netsec-9-1','DSS07-netsec-9-2',
    'DSS08-netsec-11-1','DSS08-netsec-11-2'));

SET @guard_post := IF(@q_nodes=1173 AND @q_hit=18
  AND @q_cov=5 AND @q_score100=21
  AND @q_score=@p_score
  AND @fp_roles_before=@fp_roles_after
  AND @fp_nodes_before=@fp_nodes_after
  AND @fp_scen_before=@fp_scen_after
  AND @fp_struct_before=@fp_struct_after, 1,
  (SELECT table_name FROM information_schema.tables));
