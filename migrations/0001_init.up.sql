-- =====================================================================
-- 演练系统 · MySQL 8 最终 Schema
-- InnoDB + utf8mb4 + utf8mb4_unicode_ci
-- 本文件为一次性建表，包含所有 Phase 0 盘点的最终字段（直接
-- 对齐 impact 联合主键、script_nodes.anchor_id/release_timing、
-- drill_steps.skip_reason、所有关联表 project_id 等，不再走
-- SQLite migrate() 补列的脏活）
-- =====================================================================
SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ---- 版本跟踪 ----
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     VARCHAR(32) PRIMARY KEY,
    applied_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 角色 ----
CREATE TABLE IF NOT EXISTS roles (
    id          VARCHAR(32)  PRIMARY KEY,
    name        VARCHAR(64)  NOT NULL,
    short       VARCHAR(16)  DEFAULT NULL,
    icon        VARCHAR(64)  DEFAULT NULL,
    color       VARCHAR(16)  DEFAULT NULL,
    descr       VARCHAR(256) DEFAULT NULL,
    nav         TEXT         DEFAULT NULL,   -- JSON
    `external`    TINYINT(1)   DEFAULT 0,
    status      VARCHAR(8)   DEFAULT '启用',
    seq         INT          DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 用户 ----
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT       PRIMARY KEY AUTO_INCREMENT,
    username    VARCHAR(64)  NOT NULL UNIQUE,
    name        VARCHAR(64)  NOT NULL,
    role_id     VARCHAR(32)  NOT NULL,
    org         VARCHAR(128) DEFAULT NULL,
    pwd_salt    VARCHAR(64)  NOT NULL,
    pwd_hash    VARCHAR(128) NOT NULL,
    status      VARCHAR(8)   DEFAULT '启用',
    must_change TINYINT(1)   DEFAULT 1,
    created_at  VARCHAR(19)  DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 会话（令牌）----
CREATE TABLE IF NOT EXISTS sessions (
    token      VARCHAR(512) PRIMARY KEY,
    user_id    BIGINT       NOT NULL,
    role_id    VARCHAR(32)  DEFAULT NULL,
    created_at VARCHAR(19)  DEFAULT NULL,
    expires_at VARCHAR(19)  DEFAULT NULL,
    INDEX idx_sessions_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 用户-角色绑定 ----
CREATE TABLE IF NOT EXISTS user_roles (
    user_id BIGINT      NOT NULL,
    role_id VARCHAR(32) NOT NULL,
    PRIMARY KEY (user_id, role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 演练项目 ----
CREATE TABLE IF NOT EXISTS projects (
    id          VARCHAR(32)  PRIMARY KEY,
    name        VARCHAR(128) DEFAULT NULL,
    type        VARCHAR(32)  DEFAULT NULL,
    date        VARCHAR(10)  DEFAULT NULL,
    scenario_id VARCHAR(32)  DEFAULT NULL,
    level       VARCHAR(8)   DEFAULT NULL,
    status      VARCHAR(16)  DEFAULT NULL,
    target      VARCHAR(128) DEFAULT NULL,
    units       TEXT         DEFAULT NULL,   -- JSON
    members     TEXT         DEFAULT NULL,   -- JSON
    observers   TEXT         DEFAULT NULL,   -- JSON
    mode        VARCHAR(8)   DEFAULT '基础版',
    started_at  VARCHAR(19)  DEFAULT NULL,
    paused_at   VARCHAR(19)  DEFAULT NULL,
    paused_sec  INT          DEFAULT 0,
    ended_at    VARCHAR(19)  DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 演练场景剧本 ----
CREATE TABLE IF NOT EXISTS scenarios (
    id         VARCHAR(32)  PRIMARY KEY,
    name       VARCHAR(128) DEFAULT NULL,
    type       VARCHAR(32)  DEFAULT NULL,
    level      VARCHAR(8)   DEFAULT NULL,
    difficulty VARCHAR(8)   DEFAULT NULL,
    status     VARCHAR(8)   DEFAULT NULL,
    version    VARCHAR(16)  DEFAULT NULL,
    updated    VARCHAR(10)  DEFAULT NULL,
    nodes      INT          DEFAULT NULL,
    descr      VARCHAR(512) DEFAULT NULL,
    alert      TEXT         DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 剧本节点（完整最终字段）----
CREATE TABLE IF NOT EXISTS script_nodes (
    id             VARCHAR(48)  PRIMARY KEY,
    scenario_id    VARCHAR(32)  DEFAULT NULL,
    type           VARCHAR(16)  DEFAULT NULL,
    title          VARCHAR(256) DEFAULT NULL,
    short_title    VARCHAR(32)  DEFAULT NULL,   -- ≤6 汉字
    target         VARCHAR(32)  DEFAULT NULL,
    time           VARCHAR(16)  DEFAULT NULL,
    timeout        INT          DEFAULT NULL,
    form           VARCHAR(16)  DEFAULT NULL,
    att            VARCHAR(256) DEFAULT NULL,
    descr          TEXT         DEFAULT NULL,
    branches       TEXT         DEFAULT NULL,   -- JSON
    seq            INT          DEFAULT NULL,
    guide          TEXT         DEFAULT NULL,   -- 基础版引导
    keywords       VARCHAR(512) DEFAULT NULL,   -- 逗号分隔
    action         VARCHAR(64)  DEFAULT NULL,
    wrong          TEXT         DEFAULT NULL,   -- JSON 干扰项
    evidence_req   TEXT         DEFAULT NULL,   -- JSON 取证要求
    required       TINYINT(1)   DEFAULT 1,
    flow_type      VARCHAR(16)  DEFAULT 'correct',
    score          INT          DEFAULT 0,
    receiver       VARCHAR(256) DEFAULT NULL,   -- 多选逗号分隔
    deps           TEXT         DEFAULT NULL,   -- JSON 前置节点
    pos_x          INT          DEFAULT NULL,
    pos_y          INT          DEFAULT NULL,
    anchor_id      VARCHAR(48)  DEFAULT NULL,   -- 混淆锚定
    release_timing VARCHAR(16)  DEFAULT 'before', -- before/after
    parallel_desc  VARCHAR(256) DEFAULT NULL,
    INDEX idx_sn_scenario (scenario_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 实战版处置记录 ----
CREATE TABLE IF NOT EXISTS drill_steps (
    id            BIGINT       PRIMARY KEY AUTO_INCREMENT,
    project_id    VARCHAR(32)  DEFAULT NULL,
    scenario_id   VARCHAR(32)  DEFAULT NULL,
    role_id       VARCHAR(32)  DEFAULT NULL,
    user          VARCHAR(64)  DEFAULT NULL,
    action        VARCHAR(64)  DEFAULT NULL,
    title         VARCHAR(256) DEFAULT NULL,
    descr         TEXT         DEFAULT NULL,
    step_time     VARCHAR(16)  DEFAULT NULL,   -- HH:mm
    matched_node  VARCHAR(48)  DEFAULT NULL,
    matched       TINYINT(1)   DEFAULT 0,
    ontime        TINYINT(1)   DEFAULT 0,
    choice        VARCHAR(256) DEFAULT NULL,
    correct       TINYINT(1)   DEFAULT 1,
    found_at      VARCHAR(19)  DEFAULT NULL,
    found         TEXT         DEFAULT NULL,
    measure       TEXT         DEFAULT NULL,
    result        TEXT         DEFAULT NULL,
    evidence      TEXT         DEFAULT NULL,
    forms         TEXT         DEFAULT NULL,   -- JSON
    flow_type     VARCHAR(16)  DEFAULT 'correct',
    score         DOUBLE       DEFAULT 0,
    receiver      VARCHAR(256) DEFAULT NULL,
    receiver_ok   TINYINT(1)   DEFAULT 1,
    skipped       TINYINT(1)   DEFAULT 0,
    skip_reason   VARCHAR(500) DEFAULT NULL,
    created_at    VARCHAR(19)  DEFAULT NULL,
    INDEX idx_ds_proj (project_id),
    INDEX idx_ds_proj_node (project_id, matched_node)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 节点处置中状态 ----
CREATE TABLE IF NOT EXISTS drill_node_state (
    project_id VARCHAR(32)  NOT NULL,
    node_id    VARCHAR(48)  NOT NULL,
    role_id    VARCHAR(32)  DEFAULT NULL,
    user       VARCHAR(64)  DEFAULT NULL,
    started_at VARCHAR(19)  DEFAULT NULL,
    PRIMARY KEY (project_id, node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 基础版任务 ----
CREATE TABLE IF NOT EXISTS tasks (
    id           VARCHAR(32)  PRIMARY KEY,
    project_id   VARCHAR(32)  DEFAULT NULL,
    role_id      VARCHAR(32)  DEFAULT NULL,
    title        VARCHAR(256) DEFAULT NULL,
    descr        TEXT         DEFAULT NULL,
    deadline     VARCHAR(16)  DEFAULT NULL,
    status       VARCHAR(16)  DEFAULT NULL,
    level        VARCHAR(8)   DEFAULT NULL,
    forms        TEXT         DEFAULT NULL,   -- JSON
    node_id      VARCHAR(48)  DEFAULT NULL,
    seq          INT          DEFAULT NULL,
    completed_at VARCHAR(19)  DEFAULT NULL,
    accepted_at  VARCHAR(19)  DEFAULT NULL,
    INDEX idx_tasks_proj (project_id),
    INDEX idx_tasks_role (role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 任务附件 ----
CREATE TABLE IF NOT EXISTS task_attachments (
    id           BIGINT       PRIMARY KEY AUTO_INCREMENT,
    task_id      VARCHAR(32)  NOT NULL,
    name         VARCHAR(256) NOT NULL,
    type         VARCHAR(32)  DEFAULT NULL,
    size         VARCHAR(16)  DEFAULT NULL,
    url          VARCHAR(512) DEFAULT NULL,
    file_path    VARCHAR(512) DEFAULT NULL,
    uploader     VARCHAR(64)  DEFAULT NULL,
    uploader_id  BIGINT       DEFAULT NULL,
    time         VARCHAR(19)  DEFAULT NULL,
    auto         TINYINT(1)   DEFAULT 0,
    INDEX idx_att_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 表单模板 ----
CREATE TABLE IF NOT EXISTS form_templates (
    id     VARCHAR(16) PRIMARY KEY,
    name   VARCHAR(128) DEFAULT NULL,
    fields TEXT         DEFAULT NULL   -- JSON
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 表单提交 ----
CREATE TABLE IF NOT EXISTS form_submissions (
    id        BIGINT       PRIMARY KEY AUTO_INCREMENT,
    task_id   VARCHAR(32)  DEFAULT NULL,
    form_id   VARCHAR(16)  DEFAULT NULL,
    form_name VARCHAR(128) DEFAULT NULL,
    data      TEXT         DEFAULT NULL,   -- JSON
    submitter VARCHAR(64)  DEFAULT NULL,
    user_id   BIGINT       DEFAULT NULL,
    role_id   VARCHAR(32)  DEFAULT NULL,
    time      VARCHAR(19)  DEFAULT NULL,
    INDEX idx_fs_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 指令类消息 ----
CREATE TABLE IF NOT EXISTS messages (
    id         VARCHAR(32)  PRIMARY KEY,
    time       VARCHAR(19)  DEFAULT NULL,
    level      VARCHAR(8)   DEFAULT NULL,
    source     VARCHAR(64)  DEFAULT NULL,
    text       TEXT         DEFAULT NULL,
    targets    TEXT         DEFAULT NULL,   -- JSON
    seq        INT          DEFAULT NULL,
    project_id VARCHAR(32)  DEFAULT NULL,
    INDEX idx_msgs_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 站内通知 ----
CREATE TABLE IF NOT EXISTS notifs (
    id         VARCHAR(32)  PRIMARY KEY,
    time       VARCHAR(19)  DEFAULT NULL,
    level      VARCHAR(8)   DEFAULT NULL,
    source     VARCHAR(64)  DEFAULT NULL,
    title      VARCHAR(256) DEFAULT NULL,
    descr      TEXT         DEFAULT NULL,
    seq        INT          DEFAULT NULL,
    target     VARCHAR(16)  DEFAULT 'all',
    project_id VARCHAR(32)  DEFAULT NULL,
    INDEX idx_notifs_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 通知未读状态 ----
CREATE TABLE IF NOT EXISTS notif_reads (
    user_id    BIGINT       NOT NULL,
    notif_id   VARCHAR(32)  NOT NULL,
    project_id VARCHAR(32)  DEFAULT NULL,
    PRIMARY KEY (user_id, notif_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 会商消息 ----
CREATE TABLE IF NOT EXISTS chat_messages (
    id         BIGINT       PRIMARY KEY AUTO_INCREMENT,
    grp        VARCHAR(16)  NOT NULL,
    sender     VARCHAR(64)  DEFAULT NULL,
    role_id    VARCHAR(32)  DEFAULT NULL,
    user_id    BIGINT       DEFAULT NULL,
    time       VARCHAR(16)  DEFAULT NULL,
    text       TEXT         DEFAULT NULL,
    att        VARCHAR(128) DEFAULT NULL,
    file_path  VARCHAR(512) DEFAULT NULL,
    project_id VARCHAR(32)  DEFAULT NULL,
    INDEX idx_chat_grp (grp),
    INDEX idx_chat_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 取证材料 ----
CREATE TABLE IF NOT EXISTS evidence (
    id         VARCHAR(32)  PRIMARY KEY,
    type       VARCHAR(32)  DEFAULT NULL,
    name       VARCHAR(256) DEFAULT NULL,
    size       VARCHAR(16)  DEFAULT NULL,
    time       VARCHAR(16)  DEFAULT NULL,
    role_id    VARCHAR(32)  DEFAULT NULL,
    uploader   VARCHAR(64)  DEFAULT NULL,
    user_id    BIGINT       DEFAULT NULL,
    auto       TINYINT(1)   DEFAULT 0,
    task_id    VARCHAR(32)  DEFAULT NULL,
    project_id VARCHAR(32)  DEFAULT NULL,
    url        VARCHAR(512) DEFAULT NULL,
    file_path  VARCHAR(512) DEFAULT NULL,
    created_at VARCHAR(19)  DEFAULT NULL,
    note       VARCHAR(500) DEFAULT NULL,
    node_id    VARCHAR(48)  DEFAULT NULL,
    req_key    VARCHAR(128) DEFAULT NULL,
    INDEX idx_ev_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 状态快照 ----
CREATE TABLE IF NOT EXISTS snapshots (
    id         VARCHAR(32)  PRIMARY KEY,
    project_id VARCHAR(32)  DEFAULT NULL,
    name       VARCHAR(128) DEFAULT NULL,
    time       VARCHAR(19)  DEFAULT NULL,
    by_user    VARCHAR(64)  DEFAULT NULL,
    note       VARCHAR(500) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 时序台账 ----
CREATE TABLE IF NOT EXISTS timeline (
    id         BIGINT       PRIMARY KEY AUTO_INCREMENT,
    project_id VARCHAR(32)  DEFAULT NULL,
    t          VARCHAR(16)  DEFAULT NULL,
    actor      VARCHAR(64)  DEFAULT NULL,
    role_id    VARCHAR(32)  DEFAULT NULL,
    action     VARCHAR(256) DEFAULT NULL,
    detail     TEXT         DEFAULT NULL,
    level      VARCHAR(8)   DEFAULT NULL,
    seq        INT          DEFAULT NULL,
    INDEX idx_tl_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 处置时序图节点（当前已被 monitor/dashboard 动态计算替代）----
CREATE TABLE IF NOT EXISTS seq_flow (
    id         BIGINT       PRIMARY KEY AUTO_INCREMENT,
    project_id VARCHAR(32)  DEFAULT NULL,
    name       VARCHAR(256) DEFAULT NULL,
    time       VARCHAR(16)  DEFAULT NULL,
    role       VARCHAR(64)  DEFAULT NULL,
    state      VARCHAR(16)  DEFAULT NULL,
    seq        INT          DEFAULT NULL,
    INDEX idx_sf_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 评分指标模板 ----
CREATE TABLE IF NOT EXISTS score_items (
    id     VARCHAR(32) PRIMARY KEY,
    name   VARCHAR(128) DEFAULT NULL,
    weight INT          DEFAULT NULL,
    rule   VARCHAR(256) DEFAULT NULL,
    seq    INT          DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 实际评分结果 ----
CREATE TABLE IF NOT EXISTS scores (
    item_id    VARCHAR(32)  NOT NULL,
    project_id VARCHAR(32)  NOT NULL,
    score      DOUBLE       DEFAULT NULL,
    max        INT          DEFAULT NULL,
    note       VARCHAR(500) DEFAULT NULL,
    auto       TINYINT(1)   DEFAULT 1,
    updated_at VARCHAR(19)  DEFAULT NULL,
    PRIMARY KEY (item_id, project_id),
    INDEX idx_scores_proj (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 演练台账 ----
CREATE TABLE IF NOT EXISTS ledger (
    id         VARCHAR(32)  PRIMARY KEY,
    project_id VARCHAR(32)  DEFAULT NULL,
    name       VARCHAR(128) DEFAULT NULL,
    date       VARCHAR(10)  DEFAULT NULL,
    type       VARCHAR(32)  DEFAULT NULL,
    event      VARCHAR(256) DEFAULT NULL,
    level      VARCHAR(8)   DEFAULT NULL,
    result     VARCHAR(256) DEFAULT NULL,
    score      VARCHAR(16)  DEFAULT NULL,
    org        VARCHAR(128) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 舆情记录 ----
CREATE TABLE IF NOT EXISTS opinion (
    id         VARCHAR(32)  PRIMARY KEY,
    project_id VARCHAR(32)  DEFAULT NULL,
    time       VARCHAR(19)  DEFAULT NULL,
    channel    VARCHAR(64)  DEFAULT NULL,
    title      VARCHAR(256) DEFAULT NULL,
    heat       VARCHAR(8)   DEFAULT NULL,
    risk       VARCHAR(8)   DEFAULT NULL,
    status     VARCHAR(16)  DEFAULT NULL,
    source     VARCHAR(64)  DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 预案知识库 ----
CREATE TABLE IF NOT EXISTS knowledge (
    id        VARCHAR(16)  PRIMARY KEY,
    name      VARCHAR(128) DEFAULT NULL,
    cat       VARCHAR(64)  DEFAULT NULL,
    updated   VARCHAR(10)  DEFAULT NULL,
    size      VARCHAR(16)  DEFAULT NULL,
    pages     INT          DEFAULT NULL,
    content   TEXT         DEFAULT NULL,
    file_path VARCHAR(512) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 审计日志（只追加）----
CREATE TABLE IF NOT EXISTS audit_logs (
    id      BIGINT       PRIMARY KEY AUTO_INCREMENT,
    time    VARCHAR(19)  DEFAULT NULL,
    user    VARCHAR(64)  DEFAULT NULL,
    role_id VARCHAR(32)  DEFAULT NULL,
    user_id BIGINT       DEFAULT NULL,
    action  VARCHAR(512) DEFAULT NULL,
    ip      VARCHAR(64)  DEFAULT NULL,
    INDEX idx_al_time (time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 业务影响（按项目隔离，最终联合主键）----
CREATE TABLE IF NOT EXISTS impact (
    sys        VARCHAR(128) NOT NULL,
    state      VARCHAR(16)  DEFAULT NULL,
    pct        INT          DEFAULT NULL,
    seq        INT          DEFAULT NULL,
    project_id VARCHAR(32)  NOT NULL DEFAULT '',
    PRIMARY KEY (sys, project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 流程字典 / 站内消息模板 ----
CREATE TABLE IF NOT EXISTS dict (
    `key`   VARCHAR(64) PRIMARY KEY,
    value   TEXT        DEFAULT NULL   -- JSON
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 场景模拟数据 ----
CREATE TABLE IF NOT EXISTS sim_data (
    `type` VARCHAR(32) PRIMARY KEY,
    data   TEXT        DEFAULT NULL   -- JSON
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 系统设置 ----
CREATE TABLE IF NOT EXISTS settings (
    `key`   VARCHAR(64) PRIMARY KEY,
    value   TEXT        DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- 种子基线版本 ----
CREATE TABLE IF NOT EXISTS seed_baseline (
    id          BIGINT       PRIMARY KEY AUTO_INCREMENT,
    version     VARCHAR(32)  NOT NULL,
    component   VARCHAR(32)  NOT NULL,
    data        TEXT         NOT NULL,
    note        VARCHAR(256) DEFAULT NULL,
    created_at  VARCHAR(19)  NOT NULL,
    UNIQUE KEY uk_version_component (version, component)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;
