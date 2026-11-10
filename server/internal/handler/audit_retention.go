package handler

import (
	"fmt"
	"log/slog"
	"time"

	"drill-system/internal/db"
)

// 审计保留：默认保留 365 天，AUDIT_RETENTION_DAYS=0 表示不清理。
// 不新增表结构，按时间字段分批删除，避免长事务锁表；每轮清理写一条自身审计。
const (
	auditRetentionInterval = 6 * time.Hour
	auditRetentionBatch    = 5000
	auditRetentionMaxRound = 200 // 单轮最多删 100 万条，防止异常情况下长时间占用
)

// StartAuditRetention 启动后台清理协程（启动即跑一次，之后每 6 小时一次）。
func StartAuditRetention(database *db.DB, days int) {
	if days <= 0 {
		slog.Info("审计保留策略：已关闭自动清理（AUDIT_RETENTION_DAYS=0）")
		return
	}
	slog.Info("审计保留策略已启用", "retentionDays", days, "interval", auditRetentionInterval.String())
	go func() {
		runAuditRetentionOnce(database, days)
		ticker := time.NewTicker(auditRetentionInterval)
		defer ticker.Stop()
		for range ticker.C {
			runAuditRetentionOnce(database, days)
		}
	}()
}

func runAuditRetentionOnce(database *db.DB, days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	var total int64
	for round := 0; round < auditRetentionMaxRound; round++ {
		res, err := database.Exec(`DELETE FROM audit_logs WHERE time < ? LIMIT ?`, cutoff, auditRetentionBatch)
		if err != nil {
			slog.Warn("审计清理失败", "err", err, "deletedSoFar", total)
			break
		}
		n, err := res.RowsAffected()
		if err != nil {
			break
		}
		total += n
		if n < auditRetentionBatch {
			break
		}
	}
	writeAuditRaw(database, "system", "", 0,
		fmt.Sprintf("审计清理：删除 %d 条 %s 之前的记录（保留 %d 天）", total, cutoff, days), "127.0.0.1")
	slog.Info("审计清理完成", "deleted", total, "cutoff", cutoff, "retentionDays", days)
}
