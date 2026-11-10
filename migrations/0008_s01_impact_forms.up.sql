-- =====================================================================
-- 0008_s01_impact_forms.up.sql
-- S01 业务影响联动 · 审议表单 · 勒索时序修正
-- ---------------------------------------------------------------------
-- 适用前提：
--   1. 已按顺序应用 0001 ~ 0007（script_nodes 348 条、form_templates 13 张）。
--   2. 与 0007 的关系：**只新增，不修改 0007**。0007 已作为基线版本发布，
--      本文件在其之上做增量；空库重放顺序为 0001→0008。
--   3. 全部语句幂等，可重复执行：
--        - 建表用 CREATE TABLE IF NOT EXISTS；
--        - MySQL 8 不支持 ADD COLUMN IF NOT EXISTS，改用 information_schema
--          判断 + PREPARE 动态 DDL；
--        - 数据写入用 ON DUPLICATE KEY UPDATE / 按主键 UPDATE。
--   4. 只改全局 impact 模板行（project_id=''），不动任何项目实例行。
--   5. F9《综合处置记录单》保留原样（S02–S08 共 35 处复用）；
--      F14《应急启动申请单》为新增模板，仅绑定 S01 的审议节点。
--      F4 与 F14 同名不同 id：F4 为 5 字段的申请单（fill），
--      F14 为 8 字段的审议单（review）。系统内一律按 id 引用，禁止按 name 查找。
-- ---------------------------------------------------------------------
-- 回滚方式（按需，人工执行）：
--   DROP TABLE IF EXISTS impact_events;
--   ALTER TABLE form_templates DROP COLUMN mode;
--   DELETE FROM form_templates WHERE id='F14';
--   UPDATE script_nodes SET form='F9'  WHERE id='S01-7';
--   UPDATE script_nodes SET form=''    WHERE id='S01-9';
--   UPDATE script_nodes SET form='F9'  WHERE id='S01-17';
--   （S01 标题/descr/guide/evidence_req、scenarios.alert、sim_data 的原文
--     见备份或 0007 中对应段落，回滚时按其恢复。）
-- =====================================================================
SET NAMES utf8mb4;

-- @@DDL_BEGIN
-- ---------------------------------------------------------------------
-- 1. 业务影响事件表（append-only 溯源：项目/节点/表单/时间/操作人）
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS impact_events (
    id               BIGINT       PRIMARY KEY AUTO_INCREMENT,
    project_id       VARCHAR(32)  NOT NULL DEFAULT '',
    sys              VARCHAR(128) NOT NULL,
    state            VARCHAR(16)  NOT NULL,
    pct              INT          NOT NULL DEFAULT 0,
    source_form_id   VARCHAR(16)  DEFAULT '',
    source_form_name VARCHAR(128) DEFAULT '',
    source_task_id   VARCHAR(32)  DEFAULT '',
    source_node_id   VARCHAR(48)  DEFAULT '',
    submission_id    BIGINT       DEFAULT NULL,
    operator         VARCHAR(64)  DEFAULT '',
    role_id          VARCHAR(32)  DEFAULT '',
    note             VARCHAR(255) DEFAULT '',
    created_at       VARCHAR(19)  NOT NULL,
    INDEX idx_ie_proj_sys (project_id, sys, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------
-- 2. form_templates.mode：fill（填写）/ review（审议）
--    MySQL 8 无 ADD COLUMN IF NOT EXISTS，用 information_schema 判断。
-- ---------------------------------------------------------------------
SET @has_mode := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
     WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='form_templates' AND COLUMN_NAME='mode'
);
SET @ddl := IF(@has_mode=0,
    'ALTER TABLE form_templates ADD COLUMN mode VARCHAR(16) NOT NULL DEFAULT ''fill''',
    'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 既有模板一律默认 fill；只有 F14 为 review（见下）。
UPDATE form_templates SET mode='fill' WHERE mode IS NULL OR mode='';
-- @@DDL_END

-- @@DML_BEGIN
-- ---------------------------------------------------------------------
-- 3. 全局 impact 模板：清除「HIS 开局即故障」的假故障
--    仅动 project_id='' 的模板行；不改任何项目实例行。
-- ---------------------------------------------------------------------
UPDATE impact SET state='正常', pct=100
 WHERE project_id='' AND sys='HIS收费系统' AND state='故障';

-- ---------------------------------------------------------------------
-- 4. F3《业务影响评估表》：「受影响业务系统」text → multiselect
--    用 JSON_SEARCH 定位 label 下标，避免整体覆盖 fields 丢字段。
-- ---------------------------------------------------------------------
SET @f3_path := (
    SELECT JSON_UNQUOTE(JSON_SEARCH(fields, 'one', '受影响业务系统', NULL, '$[*].label'))
      FROM form_templates WHERE id='F3'
);
SET @f3_idx := REPLACE(REPLACE(@f3_path, '$[', ''), '].label', '');
UPDATE form_templates
   SET fields = JSON_SET(
        fields,
        CONCAT('$[', @f3_idx, '].type'), 'multiselect',
        CONCAT('$[', @f3_idx, '].required'), CAST(1 AS JSON),
        CONCAT('$[', @f3_idx, '].options'), JSON_ARRAY(
            'HIS收费系统','医护工作站','电子病历系统','临床检验系统','医学影像系统','办公OA系统'),
        CONCAT('$[', @f3_idx, '].hint'),
            '勾选本次事件实际受影响的业务系统（可多选），提交后驱动大屏/看板业务影响状态。'
   )
 WHERE id='F3' AND @f3_idx IS NOT NULL AND @f3_idx <> '';

-- ---------------------------------------------------------------------
-- 5. F14《应急启动申请单》（新增，mode=review）
--    审议类节点只读展示前置节点提交内容，本表只采集审批结论与意见。
-- ---------------------------------------------------------------------
INSERT INTO form_templates (id, name, fields, mode) VALUES
('F14','应急启动申请单',
 '[{"label":"审批事项","type":"select","options":["启动Ⅱ级应急响应","结束Ⅱ级应急响应"],"required":true,"hint":"选择本节点要审批的事项。"},
   {"label":"前置申请要点","type":"textarea","required":false,"hint":"自动带出前置节点提交的申请内容，可补充摘要；正式依据以上方只读区为准。"},
   {"label":"审批结论","type":"select","options":["同意","不同意","有条件同意"],"required":true,"hint":"根据前置申请与现场态势作出明确结论。"},
   {"label":"审批意见","type":"textarea","required":true,"hint":"写明同意/不同意的理由、附加条件与执行要求。"},
   {"label":"授权范围","type":"multiselect","options":["停机","断网隔离","数据恢复","对外上报","对外发布口径"],"required":false,"hint":"勾选本次批准授权的处置范围（可多选）。"},
   {"label":"批准响应级别","type":"select","options":["Ⅰ级","Ⅱ级","Ⅲ级","Ⅳ级"],"required":false,"hint":"批准启动或结束的响应级别。"},
   {"label":"审批人","type":"text","required":true,"hint":"填写审批人姓名与职务。"},
   {"label":"审批时间","type":"datetime","required":true,"hint":"填写审批完成时间。"}]',
 'review')
ON DUPLICATE KEY UPDATE name=VALUES(name), fields=VALUES(fields), mode=VALUES(mode);

-- ---------------------------------------------------------------------
-- 6. S01 节点重新绑定（只按 id 更新，不碰 seq / short_title）
--    S01-7  批准启动Ⅱ级响应        → F14（只读前置申请单 + 审批）
--    S01-9  固定样本与日志证据      → F13（证据保全清单）
--    S01-17 宣布Ⅱ级响应结束        → F14（结束审批）
-- ---------------------------------------------------------------------
UPDATE script_nodes SET form='F14' WHERE id='S01-7';
UPDATE script_nodes SET form='F13' WHERE id='S01-9';
UPDATE script_nodes SET form='F14' WHERE id='S01-17';

-- ---------------------------------------------------------------------
-- 7. 问题 C：时序改为「数据库服务器先、客户端后」
--    S01-1 / S01-2 标题与描述；S01-3 / S01-4 引导；S01-9 取证要求
-- ---------------------------------------------------------------------
UPDATE script_nodes
   SET title='HIS 数据库服务器数据文件被批量加密、数据库服务不可用',
       descr='巡检发现 HIS 数据库服务器 db-his-01 数据文件扩展名被改为 .locked，数据库服务无法启动；门诊、收费、检验、影像等前端随即批量报「无法连接数据库」，业务中断。随后部分医护工作站与共享盘出现勒索弹窗与勒索信。'
 WHERE id='S01-1';

UPDATE script_nodes
   SET title='10 分钟内确认数据库服务器与业务系统受影响范围',
       descr='优先确认 HIS/LIS/PACS 数据库服务器加密范围与最近可用备份，再统计前端不可用清单；终端勒索弹窗作为次生现象一并登记。'
 WHERE id='S01-2';

UPDATE script_nodes
   SET guide='先隔离已确认被加密的数据库服务器及同网段备份服务器、域控，阻断横向加密；再收敛出现勒索弹窗的终端与共享目录。仅前端报错、未见加密痕迹的应用服务器不直接断网，避免扩大业务中断面。'
 WHERE id='S01-3';

UPDATE script_nodes
   SET guide='优先校验 HIS/LIS 核心库的全量备份、增量备份与异地副本是否可读、可还原，再抽验非核心库；恢复点需满足「不早于最后一次合规业务变更」。'
 WHERE id='S01-4';

UPDATE script_nodes
   SET evidence_req='["数据库加密前后文件哈希清单","数据库错误日志与勒索信样本","核心库备份校验报告","主机隔离与现场取证照片"]'
 WHERE id='S01-9';

-- ---------------------------------------------------------------------
-- 8. 场景开篇通报：服务器先被加密 → 业务中断 → 客户端弹窗
-- ---------------------------------------------------------------------
UPDATE scenarios
   SET alert='【演练模拟】HIS 数据库服务器 db-his-01 与 LIS/PACS 数据库数据文件被批量改为 .locked，数据库服务无法启动；门诊、收费、检验、影像前端批量报「无法连接数据库」，HIS 业务中断。随后部分医护工作站与共享盘出现勒索弹窗和勒索信。请信息科立即核实并按预案处置。',
       descr='核心数据库服务器遭勒索加密导致 HIS 等业务系统中断，终端随后出现勒索信；需完成隔离、备份校验、恢复与上报全流程。'
 WHERE id='S01';

-- ---------------------------------------------------------------------
-- 9. sim_data：勒索病毒场景预填数据改为「数据库优先」
-- ---------------------------------------------------------------------
INSERT INTO sim_data (`type`, data) VALUES ('勒索病毒',
 '{"sys":"HIS收费系统、临床检验系统、医学影像系统",
   "fault":"HIS/LIS/PACS 数据库服务器数据文件被改为 .locked，数据库服务无法启动",
   "alert":"数据库监控告警：db-his-01 数据文件批量变更；前端连接数据库失败",
   "ip":"10.20.30.11（模拟数据库服务器可疑连接源）",
   "sample":"勒索信样本：README_RESTORE.txt（模拟）",
   "scope":"HIS/LIS/PACS 数据库服务器被加密，HIS 业务中断，门诊/收费/检验/影像前端不可用，部分工作站出现勒索弹窗",
   "action":"隔离数据库服务器与同网段备份/域控，封堵横向传播，保留取证",
   "progress":"数据库服务器已隔离，核心库备份校验中，终端弹窗仍在收敛",
   "backup":"HIS/LIS 核心库全量+增量+异地副本校验（模拟）",
   "impact":"HIS 业务中断，收费与检验、影像业务受影响",
   "measure":"启用纸质单据与手工流程兜底，按预案逐级恢复"}')
ON DUPLICATE KEY UPDATE data=VALUES(data);
-- @@DML_END
