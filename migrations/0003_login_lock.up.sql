-- =====================================================================
-- Migration 0003 · 登录失败锁定
-- 语义：同一 username 连续失败 LOGIN_MAX_FAILURES 次 → 锁 LOGIN_LOCK_MINUTES 分钟
--      成功登录即清零；锁定期内返回 423 + 剩余分钟
-- 兼容：对已存在用户 DEFAULT 0 / NULL，不触发任何行为
-- =====================================================================

ALTER TABLE users
    ADD COLUMN login_attempts INT NOT NULL DEFAULT 0
        COMMENT '连续登录失败次数（成功或过期自动清零）',
    ADD COLUMN locked_until DATETIME NULL
        COMMENT '锁定截止时间（锁定期内登录将被拒绝）';
