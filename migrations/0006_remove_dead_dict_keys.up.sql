-- Migration 0006 · 删除「流程字典」中已被替代/从未被读取的键
-- 依据：docs/dict-merge-plan.md 盘点结论；server/（Go）与 public/ 对下列键 0 读取
--       （node/ 为已停用的旧实现，其中部分键仍有读取，不影响现网 Go 服务）。
-- 删除键（7 个）：
--   drill_action_meta       决策与干扰项（已被 script_nodes.wrong / flow_type=distractor 混淆流程取代）
--   drill_form_by_action    动作→表单模板（Go 以 script_nodes.form 为准，仅旧 Node 读取）
--   drill_level_by_scenario 剧本等级映射（Go 硬编码，仅旧 Node 读取）
--   external_node_patterns  院外节点识别特征（Go 与 public/ 0 读取）
--   difficulties            困难度枚举（Go 与 public/ 0 读取）
--   drill_role_by_target    目标单位→角色映射（Go 与 public/ 0 读取）
--   target_role_aliases     目标角色别名（Go 与 public/ 0 读取）
-- 保留：role_feed_mapping —— 复核发现 public/workbench.html 仍按角色读取该键作为默认工作台模块，
--       故不删除（与上一轮盘点结论不同，以实际 grep 结果为准）。
-- 安全约定：仅精确 DELETE 指定 key，禁止 TRUNCATE / 整表操作；不影响其它字典键与业务表。
-- 回滚方式（任选其一）：
--   1) 用发布前备份：mysqldump 备份的 dict 表 → 按 key 恢复上述 7 行；
--   2) 重放种子：重新执行 0004_seed_baseline.up.sql / 0005_seed_notif_dict.up.sql 中的
--      `INSERT IGNORE INTO dict ...` 对应语句（这两个文件均使用 INSERT IGNORE，不会覆盖已有键值）。
SET NAMES utf8mb4;

DELETE FROM dict WHERE `key` IN (
  'drill_action_meta',
  'drill_form_by_action',
  'drill_level_by_scenario',
  'external_node_patterns',
  'difficulties',
  'drill_role_by_target',
  'target_role_aliases'
);

INSERT INTO schema_migrations (version) VALUES ('0006_remove_dead_dict_keys');
