-- =====================================================================
-- 0130 补齐剩余剧本的必需角色（第二批，纯 DML）
-- ---------------------------------------------------------------------
-- 背景：0129 之后剩余缺口（含混淆节点）：
--       S05 缺 网络安全管理员、数据安全管理员
--       S06 缺 数据安全管理员、系统管理员
--       S10 缺 系统管理员
--       S11 缺 系统管理员
--       S15 缺 信息科值班员、系统管理员
-- 处理：只改责任角色，不动标题、时限、依赖、分值、流程类型与锚点。
--       每个承接点连同其锚定混淆节点一起改角色，保证该角色在流程与
--       干扰项上都可见。
-- 逐项说明（原角色 → 新角色｜依据）：
--   S05-2  核实存储与业务影响范围 系统→网安｜节点含"异常扩展名/勒索说明文件/批量加密写入"判定，属攻击破坏排查
--     DSS05-sysadmin-2-1 只看光纤链路灯… 系统→网安
--     DSS05-sysadmin-2-2 等厂商工程师到场再核实 系统→网安
--   S05-11 数据完整性校验 系统→数安｜总量比对、差异清单、补录一致性属数据完整性核验
--     DSS05-sysadmin-11-1 让业务科室自己核对数据对不对 系统→数安
--     DSS05-sysadmin-11-2 直接上线跑，有问题再回滚 系统→数安
--   S06-2  核实泄露样本与数据量级 值班员→数安｜字段、条数、敏感属性认定属数据侧核实
--     DSS06-duty-2-1  只对比时间点，不核对导出日志 值班员→数安
--     DSS06-netsec-2-2 只凭字段就认定源头，不查导出日志 网安→数安
--   S06-8  关闭外泄接口并加固数据权限 网安→系统｜接口停用、账号权限回收与最小权限复核属系统变更
--     DSS06-netsec-8-1 只锁涉及泄露的那一个接口 网安→系统
--     DSS06-netsec-8-2 等数据泄露源头抓到了再改权限 网安→系统
--   S10-16 冻结冒名档案与暂停相关账户 负责人→系统｜档案冻结、账户与医保电子凭证停用属账号/系统操作
--     DSS10-x16-a 只冻结账号不冻结档案 负责人→系统
--     DSS10-x16-b 按姓名批量冻结误伤患者 负责人→系统
--   S11-16 止损：仓库下架与备份包删除 网安→系统｜代码仓库/制品库治理与副本清理属系统与平台管理
--     DSS11-dev-1 先让外包把仓库删了… 网安→系统
--     DSS11-x16-a 只删主仓库不删复刻副本 网安→系统
--   S15-3  事件记录与内部移交签收 数安→值班员｜建档、移交与签收为值班员首接职责（与 S11-3 一致）
--     DSS15-x03-a 建档不写协议编号 数安→值班员
--     DSS15-x03-b 移交只给科研处不给法务 数安→值班员
--   S15-19 恢复：共享审批与接口白名单恢复验证 网安→系统｜接口白名单、字段脱敏与告警配置属系统变更
--     DSS15-x19-a 白名单一开就宣布恢复 网安→系统
--     DSS15-x19-b 恢复后不复核共享字段 网安→系统
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

-- 24 个承接行（8 个流程节点 + 16 个混淆节点）必须仍处于迁移前原角色
SET @p_hit := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND (
       (id='S05-2'  AND target='系统管理员')
    OR (id='S05-11' AND target='系统管理员')
    OR (id='S06-2'  AND target='信息科值班员')
    OR (id='S06-8'  AND target='网络安全管理员')
    OR (id='S10-16' AND target='信息科负责人')
    OR (id='S11-16' AND target='网络安全管理员')
    OR (id='S15-3'  AND target='数据安全管理员')
    OR (id='S15-19' AND target='网络安全管理员')
    OR (id IN ('DSS05-sysadmin-2-1','DSS05-sysadmin-2-2')     AND target='系统管理员')
    OR (id IN ('DSS05-sysadmin-11-1','DSS05-sysadmin-11-2')   AND target='系统管理员')
    OR (id='DSS06-duty-2-1'                                   AND target='信息科值班员')
    OR (id='DSS06-netsec-2-2'                                 AND target='网络安全管理员')
    OR (id IN ('DSS06-netsec-8-1','DSS06-netsec-8-2')         AND target='网络安全管理员')
    OR (id IN ('DSS10-x16-a','DSS10-x16-b')                   AND target='信息科负责人')
    OR (id IN ('DSS11-dev-1','DSS11-x16-a')                   AND target='网络安全管理员')
    OR (id IN ('DSS15-x03-a','DSS15-x03-b')                   AND target='数据安全管理员')
    OR (id IN ('DSS15-x19-a','DSS15-x19-b')                   AND target='网络安全管理员')));

SET @p_score := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',scenario_id,correct_sum) ORDER BY scenario_id SEPARATOR '\n'), 256)
  FROM (SELECT scenario_id,
          SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS correct_sum
        FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t);

SET @guard_pre := IF(@p_roles=18 AND @p_required=6 AND @p_nodes=1173 AND @p_hit=24, 1,
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
    'S05-2','S05-11','S06-2','S06-8','S10-16','S11-16','S15-3','S15-19',
    'DSS05-sysadmin-2-1','DSS05-sysadmin-2-2','DSS05-sysadmin-11-1','DSS05-sysadmin-11-2',
    'DSS06-duty-2-1','DSS06-netsec-2-2','DSS06-netsec-8-1','DSS06-netsec-8-2',
    'DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a',
    'DSS15-x03-a','DSS15-x03-b','DSS15-x19-a','DSS15-x19-b'));
SET @fp_scen_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
SET @fp_struct_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,flow_type,required,score,COALESCE(anchor_id,'-'),COALESCE(deps,'-'),COALESCE(timeout,0),COALESCE(seq,0))
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes WHERE deleted_at IS NULL AND id IN (
    'S05-2','S05-11','S06-2','S06-8','S10-16','S11-16','S15-3','S15-19',
    'DSS05-sysadmin-2-1','DSS05-sysadmin-2-2','DSS05-sysadmin-11-1','DSS05-sysadmin-11-2',
    'DSS06-duty-2-1','DSS06-netsec-2-2','DSS06-netsec-8-1','DSS06-netsec-8-2',
    'DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a',
    'DSS15-x03-a','DSS15-x03-b','DSS15-x19-a','DSS15-x19-b'));

-- ===== 责任角色调整 =====
UPDATE script_nodes SET target='网络安全管理员'
  WHERE deleted_at IS NULL AND id IN (
    'S05-2','DSS05-sysadmin-2-1','DSS05-sysadmin-2-2');
UPDATE script_nodes SET target='数据安全管理员'
  WHERE deleted_at IS NULL AND id IN (
    'S05-11','S06-2','DSS05-sysadmin-11-1','DSS05-sysadmin-11-2',
    'DSS06-duty-2-1','DSS06-netsec-2-2');
UPDATE script_nodes SET target='系统管理员'
  WHERE deleted_at IS NULL AND id IN (
    'S06-8','S10-16','S11-16','S15-19',
    'DSS06-netsec-8-1','DSS06-netsec-8-2',
    'DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a',
    'DSS15-x19-a','DSS15-x19-b');
UPDATE script_nodes SET target='信息科值班员'
  WHERE deleted_at IS NULL AND id IN ('S15-3','DSS15-x03-a','DSS15-x03-b');

-- ===== 后置断言 =====
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @q_hit := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND (
       (id IN ('S05-2') AND target='网络安全管理员')
    OR (id IN ('S05-11','S06-2') AND target='数据安全管理员')
    OR (id IN ('S06-8','S10-16','S11-16','S15-19') AND target='系统管理员')
    OR (id IN ('S15-3') AND target='信息科值班员')
    OR (id IN ('DSS05-sysadmin-2-1','DSS05-sysadmin-2-2')   AND target='网络安全管理员')
    OR (id IN ('DSS05-sysadmin-11-1','DSS05-sysadmin-11-2','DSS06-duty-2-1','DSS06-netsec-2-2') AND target='数据安全管理员')
    OR (id IN ('DSS06-netsec-8-1','DSS06-netsec-8-2','DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a','DSS15-x19-a','DSS15-x19-b') AND target='系统管理员')
    OR (id IN ('DSS15-x03-a','DSS15-x03-b')                 AND target='信息科值班员')));

-- 21 个剧本（含混淆节点）全部覆盖六大必需角色
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
    GROUP BY sc.id) c
  WHERE c.duty=1 AND c.netsec=1 AND c.datasec=1 AND c.sysadmin=1 AND c.itlead=1 AND c.commander=1);
SET @q_scen := (SELECT COUNT(*) FROM scenarios);

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
    'S05-2','S05-11','S06-2','S06-8','S10-16','S11-16','S15-3','S15-19',
    'DSS05-sysadmin-2-1','DSS05-sysadmin-2-2','DSS05-sysadmin-11-1','DSS05-sysadmin-11-2',
    'DSS06-duty-2-1','DSS06-netsec-2-2','DSS06-netsec-8-1','DSS06-netsec-8-2',
    'DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a',
    'DSS15-x03-a','DSS15-x03-b','DSS15-x19-a','DSS15-x19-b'));
SET @fp_scen_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
SET @fp_struct_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,flow_type,required,score,COALESCE(anchor_id,'-'),COALESCE(deps,'-'),COALESCE(timeout,0),COALESCE(seq,0))
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes WHERE deleted_at IS NULL AND id IN (
    'S05-2','S05-11','S06-2','S06-8','S10-16','S11-16','S15-3','S15-19',
    'DSS05-sysadmin-2-1','DSS05-sysadmin-2-2','DSS05-sysadmin-11-1','DSS05-sysadmin-11-2',
    'DSS06-duty-2-1','DSS06-netsec-2-2','DSS06-netsec-8-1','DSS06-netsec-8-2',
    'DSS10-x16-a','DSS10-x16-b','DSS11-dev-1','DSS11-x16-a',
    'DSS15-x03-a','DSS15-x03-b','DSS15-x19-a','DSS15-x19-b'));

SET @guard_post := IF(@q_nodes=1173 AND @q_hit=24
  AND @q_cov=21 AND @q_scen=21
  AND @q_score100=21 AND @q_score=@p_score
  AND @fp_roles_before=@fp_roles_after
  AND @fp_nodes_before=@fp_nodes_after
  AND @fp_scen_before=@fp_scen_after
  AND @fp_struct_before=@fp_struct_after, 1,
  (SELECT table_name FROM information_schema.tables));
