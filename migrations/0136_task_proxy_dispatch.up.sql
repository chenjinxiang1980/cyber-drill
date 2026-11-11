-- 0136_task_proxy_dispatch.up.sql
-- 未参演角色任务代填：任务下发/开始演练时，若节点目标角色不在本项目参演角色中，
-- 任务自动转交信息科负责人（roles.id=itlead）代填，并保留被代填角色以便追溯。
--
-- 说明：
--   1) DDL 用 information_schema 守卫，重复执行安全；
--   2) 存量数据回填使用显式事务 + 前后断言（DDL 已提交，无法与 DDL 共用事务）；
--   3) 只处理未完成（非已完成/已跳过）任务，不改写历史完成记录。

SET NAMES utf8mb4;

SET @db := DATABASE();

-- 1) tasks.is_proxy
SET @has := (SELECT COUNT(*) FROM information_schema.COLUMNS
             WHERE TABLE_SCHEMA=@db AND TABLE_NAME='tasks' AND COLUMN_NAME='is_proxy');
SET @sql := IF(@has=0,
  'ALTER TABLE tasks ADD COLUMN is_proxy TINYINT NOT NULL DEFAULT 0 COMMENT ''未参演角色代填任务标记''',
  'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2) tasks.proxy_for_role_id（被代填角色，任务实际归属已改写为 itlead）
SET @has := (SELECT COUNT(*) FROM information_schema.COLUMNS
             WHERE TABLE_SCHEMA=@db AND TABLE_NAME='tasks' AND COLUMN_NAME='proxy_for_role_id');
SET @sql := IF(@has=0,
  'ALTER TABLE tasks ADD COLUMN proxy_for_role_id VARCHAR(32) DEFAULT NULL COMMENT ''被代填角色ID''',
  'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 3) tasks.proxy_for_role_name（被代填角色名称快照，便于页面/报告展示）
SET @has := (SELECT COUNT(*) FROM information_schema.COLUMNS
             WHERE TABLE_SCHEMA=@db AND TABLE_NAME='tasks' AND COLUMN_NAME='proxy_for_role_name');
SET @sql := IF(@has=0,
  'ALTER TABLE tasks ADD COLUMN proxy_for_role_name VARCHAR(64) DEFAULT NULL COMMENT ''被代填角色名称''',
  'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 4) tasks.proxy_reason（代填原因，当前固定 unparticipating）
SET @has := (SELECT COUNT(*) FROM information_schema.COLUMNS
             WHERE TABLE_SCHEMA=@db AND TABLE_NAME='tasks' AND COLUMN_NAME='proxy_reason');
SET @sql := IF(@has=0,
  'ALTER TABLE tasks ADD COLUMN proxy_reason VARCHAR(32) DEFAULT NULL COMMENT ''代填原因''',
  'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 5) 索引：按项目筛选代填任务
SET @has := (SELECT COUNT(*) FROM information_schema.STATISTICS
             WHERE TABLE_SCHEMA=@db AND TABLE_NAME='tasks' AND INDEX_NAME='idx_tasks_proxy');
SET @sql := IF(@has=0,
  'ALTER TABLE tasks ADD INDEX idx_tasks_proxy (project_id,is_proxy)',
  'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 6) 存量回填：未开始/演练中/暂停项目里，目标角色未参演的未完成任务转交 itlead 代填
START TRANSACTION;

SET @before := (SELECT COUNT(*)
  FROM tasks t JOIN projects p ON p.id=t.project_id
  WHERE t.is_proxy=0
    AND COALESCE(t.status,'') NOT IN ('已完成','已跳过')
    AND p.status IN ('未开始','演练中','暂停')
    AND t.role_id IS NOT NULL AND t.role_id<>'' AND t.role_id<>'itlead'
    AND JSON_VALID(p.members) AND JSON_LENGTH(p.members)>0
    AND COALESCE(JSON_CONTAINS(p.members->'$[*].role', JSON_QUOTE(t.role_id)),0)=0
    AND COALESCE(JSON_CONTAINS(p.members->'$[*].roleId', JSON_QUOTE(t.role_id)),0)=0);

UPDATE tasks t
LEFT JOIN roles r ON r.id=t.role_id
JOIN projects p ON p.id=t.project_id
SET t.is_proxy=1,
    t.proxy_for_role_id=t.role_id,
    t.proxy_for_role_name=COALESCE(NULLIF(r.name,''),t.role_id),
    t.proxy_reason='unparticipating',
    t.role_id='itlead'
WHERE t.is_proxy=0
  AND COALESCE(t.status,'') NOT IN ('已完成','已跳过')
  AND p.status IN ('未开始','演练中','暂停')
  AND t.role_id IS NOT NULL AND t.role_id<>'' AND t.role_id<>'itlead'
  AND JSON_VALID(p.members) AND JSON_LENGTH(p.members)>0
  AND COALESCE(JSON_CONTAINS(p.members->'$[*].role', JSON_QUOTE(t.role_id)),0)=0
  AND COALESCE(JSON_CONTAINS(p.members->'$[*].roleId', JSON_QUOTE(t.role_id)),0)=0;

SET @migrated := ROW_COUNT();

-- 前后断言：应无遗留（仍指向未参演角色的未完成任务），且代填任务结构完整
SET @after := (SELECT COUNT(*)
  FROM tasks t JOIN projects p ON p.id=t.project_id
  WHERE t.is_proxy=0
    AND COALESCE(t.status,'') NOT IN ('已完成','已跳过')
    AND p.status IN ('未开始','演练中','暂停')
    AND t.role_id IS NOT NULL AND t.role_id<>'' AND t.role_id<>'itlead'
    AND JSON_VALID(p.members) AND JSON_LENGTH(p.members)>0
    AND COALESCE(JSON_CONTAINS(p.members->'$[*].role', JSON_QUOTE(t.role_id)),0)=0
    AND COALESCE(JSON_CONTAINS(p.members->'$[*].roleId', JSON_QUOTE(t.role_id)),0)=0);

SET @bad := (SELECT COUNT(*) FROM tasks
             WHERE is_proxy=1
               AND (role_id<>'itlead' OR proxy_for_role_id IS NULL OR proxy_for_role_id=''
                    OR proxy_for_role_name IS NULL OR proxy_for_role_name=''));

SELECT IF(@after=0, 1, (SELECT table_name FROM information_schema.tables)) AS assert_no_pending_unparticipating;
SELECT IF(@bad=0, 1, (SELECT table_name FROM information_schema.tables)) AS assert_proxy_shape;

COMMIT;
