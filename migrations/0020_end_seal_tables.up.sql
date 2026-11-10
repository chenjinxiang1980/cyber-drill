-- 演练收尾与评分封存：核心证据清单、结束封存、缺项说明、评分快照
-- 幂等：全部使用 CREATE TABLE IF NOT EXISTS。

-- 项目级核心证据清单（创建项目时从平台默认复制，项目内可调整）
CREATE TABLE IF NOT EXISTS project_core_evidence (
    project_id   VARCHAR(32)  NOT NULL,
    ev_key       VARCHAR(128) NOT NULL,
    scenario_id  VARCHAR(32)  DEFAULT NULL,
    node_id      VARCHAR(48)  DEFAULT NULL,
    ev_index     INT          DEFAULT 0,
    ev_name      VARCHAR(256) DEFAULT NULL,
    node_title   VARCHAR(256) DEFAULT NULL,
    target       VARCHAR(64)  DEFAULT NULL,
    seq          INT          DEFAULT 0,
    source       VARCHAR(16)  DEFAULT 'default',
    adjusted_by  VARCHAR(64)  DEFAULT NULL,
    adjusted_at  VARCHAR(19)  DEFAULT NULL,
    PRIMARY KEY (project_id, ev_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 项目结束封存（1:1）：end_mode 区分「完整 / 有缺项」
CREATE TABLE IF NOT EXISTS project_end_seal (
    project_id       VARCHAR(32) NOT NULL,
    end_mode         VARCHAR(16) NOT NULL DEFAULT '完整',
    snapshot_id      VARCHAR(48) DEFAULT NULL,
    snapshot_version INT         DEFAULT NULL,
    sealed_at        VARCHAR(19) DEFAULT NULL,
    sealed_by        VARCHAR(64) DEFAULT NULL,
    exception_id     BIGINT      DEFAULT NULL,
    PRIMARY KEY (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 有缺项结束说明：原因 / 缺项 key / 责任人 / 补录时限 / 批准人
CREATE TABLE IF NOT EXISTS project_end_exception (
    id           BIGINT       PRIMARY KEY AUTO_INCREMENT,
    project_id   VARCHAR(32)  NOT NULL,
    snapshot_id  VARCHAR(48)  DEFAULT NULL,
    reason       VARCHAR(500) DEFAULT NULL,
    missing_keys TEXT         DEFAULT NULL,
    missing_count INT         DEFAULT 0,
    owner        VARCHAR(64)  DEFAULT NULL,
    deadline     VARCHAR(19)  DEFAULT NULL,
    approver     VARCHAR(64)  DEFAULT NULL,
    created_by   VARCHAR(64)  DEFAULT NULL,
    created_at   VARCHAR(19)  DEFAULT NULL,
    INDEX idx_pee_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 评分快照：封存时刻的逐节点扣分明细 / 完整率 / 规则版本 / 剧本版本
CREATE TABLE IF NOT EXISTS score_snapshots (
    id                     VARCHAR(48) NOT NULL,
    project_id             VARCHAR(32) NOT NULL,
    version                INT         NOT NULL DEFAULT 1,
    snapshot_status        VARCHAR(16) NOT NULL DEFAULT '已封存',
    end_mode               VARCHAR(16) DEFAULT NULL,
    score                  DECIMAL(6,1) NOT NULL DEFAULT 0,
    grade                  VARCHAR(24) DEFAULT NULL,
    max_score              INT         DEFAULT 0,
    hit                    INT         DEFAULT 0,
    total                  INT         DEFAULT 0,
    completeness           DECIMAL(6,2) DEFAULT 0,
    time_rate              DECIMAL(6,2) DEFAULT 0,
    evidence_complete_rate DECIMAL(6,2) DEFAULT 0,
    core_complete_rate     DECIMAL(6,2) DEFAULT 0,
    core_total             INT         DEFAULT 0,
    core_done              INT         DEFAULT 0,
    rule_version           VARCHAR(64) DEFAULT NULL,
    scenario_version       VARCHAR(32) DEFAULT NULL,
    payload                LONGTEXT    DEFAULT NULL,
    recalc_from            INT         DEFAULT NULL,
    recalc_reason          VARCHAR(255) DEFAULT NULL,
    sealed_at              VARCHAR(19) DEFAULT NULL,
    sealed_by              VARCHAR(64) DEFAULT NULL,
    created_at             VARCHAR(19) DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_snap_project_version (project_id, version),
    INDEX idx_snap_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
