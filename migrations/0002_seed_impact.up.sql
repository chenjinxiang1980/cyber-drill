-- =====================================================================
-- 演练系统 · impact 全局模板种子
-- 在 schema_migrations 里记一条 0002，INSERT IGNORE 保证幂等
-- 对齐 Node src/seed_data.js IMPACT 原始模板：
--   HIS收费系统  故障  30%（示例市医院勒索演练固有告警）
--   医护工作站   正常 100%
--   电子病历系统 正常 100%
--   临床检验系统 正常 100%
--   医学影像系统 正常 100%
--   办公OA系统   正常 100%
-- =====================================================================
SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- 记录已应用
INSERT IGNORE INTO schema_migrations (version) VALUES ('0002_seed_impact');

-- 6 行全局模板（project_id = ''）
INSERT IGNORE INTO impact (sys, state, pct, seq, project_id) VALUES
    ('HIS收费系统',   '故障', 30, 0, ''),
    ('医护工作站',    '正常', 100, 1, ''),
    ('电子病历系统',  '正常', 100, 2, ''),
    ('临床检验系统',  '正常', 100, 3, ''),
    ('医学影像系统',  '正常', 100, 4, ''),
    ('办公OA系统',    '正常', 100, 5, '');

SET FOREIGN_KEY_CHECKS = 1;
