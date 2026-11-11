-- =====================================================================
-- 0128 下线「运维负责人 / 算法工程师」两个临时角色（纯 DML）
-- ---------------------------------------------------------------------
-- 背景：0127 为承接 S13/S14 的责任科室补了 opslead(运维负责人)、
--       algoengineer(算法工程师) 两个正式角色。现网已确认：
--       S13 的运维负责人改为「信息科负责人」，S14 的算法工程师改为
--       「系统管理员」，两个临时角色不再需要。
-- 处理：先把 script_nodes 中所有旧 target 改成新角色名（含已软删行），
--       再处理 users.role_id / user_roles / tasks / sessions 的绑定，
--       最后删除 roles 两行；itlead/sysadmin 角色行本身不动。
-- 兼容：现网可能已被人工改过一部分 target，本迁移按 target 名称匹配，
--       改过 0 行也能正常通过；全新安装（0127 之后）同样会走一遍，
--       最终状态一致。seq 18/19 删除后留空，不重排门诊部 seq=20，
--       避免触碰范围外角色行。
-- 写法：前置断言 + 单事务（由 migrate 执行器保证）+ 范围外行指纹 +
--       后置断言；守卫失败触发 1242 回滚。
-- =====================================================================
SET NAMES utf8mb4;
SET SESSION group_concat_max_len = 10485760;

-- ===== 前置断言 =====
SET @p_roles := (SELECT COUNT(*) FROM roles);
SET @p_ops := (SELECT COUNT(*) FROM roles
  WHERE id='opslead' AND name='运维负责人');
SET @p_algo := (SELECT COUNT(*) FROM roles
  WHERE id='algoengineer' AND name='算法工程师');
SET @p_itlead := (SELECT COUNT(*) FROM roles
  WHERE id='itlead' AND name='信息科负责人');
SET @p_sysadmin := (SELECT COUNT(*) FROM roles
  WHERE id='sysadmin' AND name='系统管理员');
SET @p_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);

-- 旧 target 只允许出现在对应剧本，避免误改其他剧本
SET @p_ops_bad := (SELECT COUNT(*) FROM script_nodes
  WHERE target='运维负责人' AND scenario_id<>'S13');
SET @p_algo_bad := (SELECT COUNT(*) FROM script_nodes
  WHERE target='算法工程师' AND scenario_id<>'S14');

-- 记录 S13/S14 迁移前后应保持守恒的数量
SET @p_s13_old := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S13' AND target='运维负责人');
SET @p_s13_new := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S13' AND target='信息科负责人');
SET @p_s14_old := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S14' AND target='算法工程师');
SET @p_s14_new := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S14' AND target='系统管理员');

SET @guard_pre := IF(@p_roles=20 AND @p_ops=1 AND @p_algo=1
  AND @p_itlead=1 AND @p_sysadmin=1 AND @p_nodes=1173
  AND @p_ops_bad=0 AND @p_algo_bad=0, 1,
  (SELECT table_name FROM information_schema.tables));

-- 本迁移会改写的节点 id 清单；前后指纹都排除这批行，保证指纹行集合一致
SET @scope_ids := (SELECT GROUP_CONCAT(id ORDER BY id)
  FROM script_nodes WHERE target IN ('运维负责人','算法工程师'));

-- ===== 范围外行指纹（写前） =====
SET @fp_roles_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM roles WHERE id NOT IN ('opslead','algoengineer'));
SET @fp_nodes_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes
  WHERE deleted_at IS NULL
    AND (@scope_ids IS NULL OR FIND_IN_SET(id,@scope_ids)=0));
SET @fp_scen_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
SET @fp_score_before := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',scenario_id,correct_sum) ORDER BY scenario_id SEPARATOR '\n'), 256)
  FROM (SELECT scenario_id,
          SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS correct_sum
        FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t);

-- ===== 1. 剧本节点责任角色改名（含软删行，确保旧名称彻底消失） =====
UPDATE script_nodes SET target='信息科负责人' WHERE target='运维负责人';
UPDATE script_nodes SET target='系统管理员'   WHERE target='算法工程师';

-- ===== 2. 账号/任务/会话绑定处理 =====
-- 主角色跟随迁移，避免删角色后账号失去归属
UPDATE users SET role_id='itlead'   WHERE role_id='opslead';
UPDATE users SET role_id='sysadmin' WHERE role_id='algoengineer';
-- 附角色按需求直接清理（不做自动叠加，避免改变用户权限集合）
DELETE FROM user_roles WHERE role_id IN ('opslead','algoengineer');
-- 待处理任务的归属角色跟随改名，避免工作台出现无主任务
UPDATE tasks SET role_id='itlead'   WHERE role_id='opslead';
UPDATE tasks SET role_id='sysadmin' WHERE role_id='algoengineer';
-- 旧角色会话强制失效，重新登录后按新角色授权
DELETE FROM sessions WHERE role_id IN ('opslead','algoengineer');

-- ===== 3. 删除两个临时角色 =====
DELETE FROM roles WHERE id IN ('opslead','algoengineer');

-- ===== 后置断言 =====
SET @q_roles := (SELECT COUNT(*) FROM roles);
SET @q_old_role := (SELECT COUNT(*) FROM roles
  WHERE id IN ('opslead','algoengineer')
     OR name IN ('运维负责人','算法工程师')
     OR short IN ('运维负责人','算法工程师'));
SET @q_old_target := (SELECT COUNT(*) FROM script_nodes
  WHERE target IN ('运维负责人','算法工程师'));
SET @q_s13_new := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S13' AND target='信息科负责人');
SET @q_s14_new := (SELECT COUNT(*) FROM script_nodes
  WHERE deleted_at IS NULL AND scenario_id='S14' AND target='系统管理员');
SET @q_users := (SELECT COUNT(*) FROM users WHERE role_id IN ('opslead','algoengineer'));
SET @q_ur := (SELECT COUNT(*) FROM user_roles WHERE role_id IN ('opslead','algoengineer'));
SET @q_tasks := (SELECT COUNT(*) FROM tasks WHERE role_id IN ('opslead','algoengineer'));
SET @q_sessions := (SELECT COUNT(*) FROM sessions WHERE role_id IN ('opslead','algoengineer'));
SET @q_required := (SELECT COUNT(*) FROM roles
  WHERE name IN ('信息科值班员','网络安全管理员','数据安全管理员','系统管理员','信息科负责人','院应急指挥部'));

SET @fp_roles_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM roles WHERE id NOT IN ('opslead','algoengineer'));
SET @fp_nodes_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'), 256)
  FROM script_nodes
  WHERE deleted_at IS NULL
    AND (@scope_ids IS NULL OR FIND_IN_SET(id,@scope_ids)=0));
SET @fp_scen_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n'), 256) FROM scenarios);
SET @fp_score_after := (SELECT SHA2(GROUP_CONCAT(CONCAT_WS('|',scenario_id,correct_sum) ORDER BY scenario_id SEPARATOR '\n'), 256)
  FROM (SELECT scenario_id,
          SUM(CASE WHEN flow_type='correct' THEN score ELSE 0 END) AS correct_sum
        FROM script_nodes WHERE deleted_at IS NULL GROUP BY scenario_id) t);

SET @guard_post := IF(@q_roles=18 AND @q_old_role=0 AND @q_old_target=0
  AND @q_s13_new=@p_s13_new+@p_s13_old
  AND @q_s14_new=@p_s14_new+@p_s14_old
  AND @q_users=0 AND @q_ur=0 AND @q_tasks=0 AND @q_sessions=0
  AND @q_required=6
  AND @fp_roles_before=@fp_roles_after
  AND @fp_nodes_before=@fp_nodes_after
  AND @fp_scen_before=@fp_scen_after
  AND @fp_score_before=@fp_score_after, 1,
  (SELECT table_name FROM information_schema.tables));
