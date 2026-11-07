-- 0083 接收人主送/抄送分离（P1-02 结构侧）
-- 依据：评审 P1-02 —— 接收人按“集合完全相等”判定，多选合法协同单位也扣 30%；
-- 另需要有“主送必须命中、抄送缺失降权”的表达能力。
-- 本迁移只新增 script_nodes.receiver_cc（抄送，可选），不改任何既有 receiver 值：
--   1) receiver 继续表示主送对象，评分必须命中；
--   2) receiver_cc 为空时评分不得产生任何扣分（用户补充口径）；
--   3) S06-6/S06-7 的“医务处、院办”目前只写在 descr 的院内抄送说明里，
--      未进入 receiver，也无独立抄送选择入口；本轮不写入 receiver_cc，避免
--      “用户无法选择却被扣 10%”的新问题，待前端提供抄送选择后再配置。
-- 含 DDL，按可重复执行编写。

SET NAMES utf8mb4;

SET @has_col := (
  SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'script_nodes' AND column_name = 'receiver_cc'
);
SET @ddl := IF(@has_col = 0,
  'ALTER TABLE script_nodes ADD COLUMN receiver_cc VARCHAR(256) DEFAULT NULL',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 归一：历史空串统一为 NULL，避免“空抄送”被误判为非空
UPDATE script_nodes SET receiver_cc = NULL WHERE receiver_cc = '';
