-- =====================================================================
-- 0127 剧本责任科室转正式角色（纯 DML）
-- ---------------------------------------------------------------------
-- 背景：S09/S10/S13/S14/S16 的必做节点责任对象写作「检验科主任 / 医保办 /
--       运维负责人 / 算法工程师 / 门诊部」，roles 表中没有对应行，
--       只能靠 Go 侧 targetRoleAliases 映射到 medlead / sysadmin / netsec。
--       映射结果导致：任务归属角色与剧本责任科室不一致、工作台按真实角色
--       看不到任务、角色权限矩阵无法覆盖这些科室。
-- 处理：补齐 5 个正式角色行（external=0，与原先映射目标同为院内角色，
--       外部单位代填校验范围不变）；Go 侧别名映射保留为向后兼容，
--       解析顺序改为「精确 name/short → 别名 → 模糊匹配」。
--       既有 15 个角色的属性一律不改（下方指纹守卫）。
-- 写法：前置断言 + 单事务 + 范围外行指纹 + 后置断言；守卫失败触发 1242 回滚。
-- =====================================================================
SET NAMES utf8mb4;

-- ===== 前置断言 =====
SET @p_roles := (SELECT COUNT(*) FROM roles);
SET @p_ext   := (SELECT COUNT(*) FROM roles WHERE COALESCE(`external`,0)=1);
SET @p_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
SET @p_collide := (SELECT COUNT(*) FROM roles
  WHERE id IN ('labdirector','insurance','opslead','algoengineer','outpatient')
     OR name IN ('检验科主任','医保办','运维负责人','算法工程师','门诊部')
     OR short IN ('检验主任','医保办','运维负责人','算法工程师','门诊部'));
SET @p_t1 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND target='检验科主任');
SET @p_t2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND target='医保办');
SET @p_t3 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND target='运维负责人');
SET @p_t4 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND target='算法工程师');
SET @p_t5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND target='门诊部');
-- 必做节点（不含干扰项）数量，用于确认责任科室确实落在主流流程上
SET @p_r1 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND required=1 AND target='检验科主任');
SET @p_r2 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND required=1 AND target='医保办');
SET @p_r3 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND required=1 AND target='运维负责人');
SET @p_r4 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND required=1 AND target='算法工程师');
SET @p_r5 := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND required=1 AND target='门诊部');

SET @guard_pre := IF(@p_roles=15 AND @p_ext=4 AND @p_nodes=1173 AND @p_collide=0
  AND @p_t1=3 AND @p_t2=10 AND @p_t3=3 AND @p_t4=14 AND @p_t5=3
  AND @p_r1=1 AND @p_r2=3 AND @p_r3=1 AND @p_r4=5 AND @p_r5=1, 1,
  (SELECT table_name FROM information_schema.tables));

-- ===== 范围外行指纹（写前：既有 15 个角色必须一字不动） =====
SET @fp_roles_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'))
  FROM roles WHERE id NOT IN ('labdirector','insurance','opslead','algoengineer','outpatient'));
SET @fp_nodes_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL);
SET @fp_scen_before := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);

-- ===== 1. 补 5 个正式角色（院内，external=0；seq 接排 16–20） =====
INSERT INTO roles (id,name,short,icon,color,descr,nav,`external`,status,seq) VALUES
('labdirector','检验科主任','检验主任','hospital','#f778ba',
 '检验结果真实性核定：原始记录与临床纸质报告、标本与仪器台账比对，检验数据复核；不承担系统处置。',
 '["workbench","collab","knowledge"]',0,'启用',16),
('insurance','医保办','医保办','file','#d29922',
 '医保结算核查与止损：冒名与虚假结算规模核定、阻断虚假入账、向医保经办机构书面通知并取回执。',
 '["workbench","collab","knowledge"]',0,'启用',17),
('opslead','运维负责人','运维负责人','db','#3fb950',
 '运维保障与外部链路协调：服务器/存储/网络运维处置，向运营商书面通知并取回执。',
 '["workbench","collab","evidence","knowledge"]',0,'启用',18),
('algoengineer','算法工程师','算法工程师','net','#4f7cff',
 '模型数据治理：污染样本台账与标注来源核对、涉事模型下线切换、回滚清洗重训与灰度验证。',
 '["workbench","collab","evidence","knowledge"]',0,'启用',19),
('outpatient','门诊部','门诊部','building','#56d364',
 '门诊业务恢复：补录数据核对、门诊与信息科业务双签确认。',
 '["workbench","collab","knowledge"]',0,'启用',20);

-- ===== 后置断言 =====
SET @q_roles := (SELECT COUNT(*) FROM roles);
SET @q_ext   := (SELECT COUNT(*) FROM roles WHERE COALESCE(`external`,0)=1);
SET @q_new   := (SELECT COUNT(*) FROM roles
  WHERE id IN ('labdirector','insurance','opslead','algoengineer','outpatient')
    AND COALESCE(`external`,0)=0 AND status='启用' AND seq BETWEEN 16 AND 20);
SET @q_nodes := (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL);
-- 剧本中的 5 个责任科室名必须都能精确命中 roles.name
SET @q_name_hit := (SELECT COUNT(*) FROM (
  SELECT '检验科主任' AS n UNION ALL SELECT '医保办' UNION ALL
  SELECT '运维负责人' UNION ALL SELECT '算法工程师' UNION ALL SELECT '门诊部'
) t WHERE EXISTS (SELECT 1 FROM roles r WHERE r.name=t.n));
SET @fp_roles_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,name,short,icon,color,descr,nav,COALESCE(`external`,0),status,seq)
  ORDER BY id SEPARATOR '\n'))
  FROM roles WHERE id NOT IN ('labdirector','insurance','opslead','algoengineer','outpatient'));
SET @fp_nodes_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',
    id,scenario_id,seq,type,flow_type,required,score,timeout,target,deps)
  ORDER BY id SEPARATOR '\n'))
  FROM script_nodes WHERE deleted_at IS NULL);
SET @fp_scen_after := (SELECT MD5(GROUP_CONCAT(CONCAT_WS('|',id,COALESCE(nodes,0)) ORDER BY id SEPARATOR '\n')) FROM scenarios);

SET @guard_post := IF(@q_roles=20 AND @q_ext=4 AND @q_new=5 AND @q_nodes=1173 AND @q_name_hit=5
  AND @fp_roles_before=@fp_roles_after
  AND @fp_nodes_before=@fp_nodes_after
  AND @fp_scen_before=@fp_scen_after, 1,
  (SELECT table_name FROM information_schema.tables));
