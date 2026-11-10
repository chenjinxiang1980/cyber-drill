-- 演练收尾策略默认值（INSERT IGNORE 幂等，不覆盖现网已调整的配置）
INSERT IGNORE INTO settings (`key`, value) VALUES
    ('drill_core_evidence_min', '10'),
    ('drill_core_evidence_max', '12'),
    ('drill_supplement_hours', '72'),
    ('drill_rule_version_salt', '2026-11');
