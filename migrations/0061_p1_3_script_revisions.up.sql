-- P1-3 剧本版本历史
-- 每次保存剧本前，把「保存前」的完整节点快照写入 script_revisions，
-- 供查看/回滚上一版；被删除的节点可从快照恢复（替代节点表软删）。
-- 可重复执行：CREATE TABLE IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS script_revisions (
    id            BIGINT       PRIMARY KEY AUTO_INCREMENT,
    scenario_id   VARCHAR(32)  NOT NULL,
    version       VARCHAR(16)  NOT NULL,
    name          VARCHAR(128) DEFAULT NULL,
    node_count    INT          DEFAULT 0,
    nodes_payload LONGTEXT     DEFAULT NULL,
    source        VARCHAR(16)  DEFAULT 'save',
    created_by    VARCHAR(64)  DEFAULT NULL,
    created_at    VARCHAR(19)  DEFAULT NULL,
    UNIQUE KEY uk_sr_scen_ver (scenario_id, version),
    INDEX idx_sr_scen (scenario_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
