package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// AuditWrites 服务端审计：对所有非只读 /api 请求在响应后落一条审计。
// 结构化字段（操作人、角色、IP、动作）全部来自服务端上下文，不接受客户端提交。
// 业务 handler 可通过 writeAudit 写入更语义化的动作，并设置 auditWritten 避免重复。
func AuditWrites(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 备份恢复窗口：阻断业务写请求，避免导入过程中继续写库（登录与恢复入口除外）。
		if InMaintenance() && maintenanceBlocked(c) {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "系统维护中（数据恢复），请稍后重试"})
			return
		}
		if !auditWritesEnabled(c) {
			c.Next()
			return
		}
		c.Next()
		if c.GetBool("auditWritten") {
			return
		}
		u := auth.CurrentUser(c)
		if u == nil {
			return
		}
		action := c.Request.Method + " " + c.Request.URL.Path + " [" + itoa(c.Writer.Status()) + "]"
		writeAuditRaw(database, u.Username, u.Role.ID, u.ID, action, c.ClientIP())
	}
}

func auditWritesEnabled(c *gin.Context) bool {
	path := c.Request.URL.Path
	switch c.Request.Method {
	case http.MethodHead, http.MethodOptions:
		return false
	case http.MethodGet:
		// 附件读取由服务端记录；客户端自由文本审计接口不落库。
		return strings.HasPrefix(path, "/uploads/") || strings.HasPrefix(path, "/media/")
	}
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	switch path {
	case "/api/auth/login", "/api/auth/login/role", "/api/auth/captcha", "/api/content/audit":
		return false
	}
	return true
}

// maintenanceBlocked 判断恢复维护窗口内应被拒绝的请求：
// 只拦截业务写请求，放行只读接口、登录/验证码与恢复入口本身。
func maintenanceBlocked(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	path := c.Request.URL.Path
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	switch path {
	case "/api/auth/login", "/api/auth/login/role", "/api/auth/captcha", "/api/admin/restore", "/api/admin/restore-upload":
		return false
	}
	return true
}

// writeAudit 由业务 handler 调用，记录语义化动作。
func writeAudit(database *db.DB, u *auth.User, c *gin.Context, action string) {
	if u == nil {
		return
	}
	writeAuditRaw(database, u.Username, u.Role.ID, u.ID, action, c.ClientIP())
	c.Set("auditWritten", true)
}

// writeAuditRaw 底层写入；任何失败只记录日志，不影响业务响应。
func writeAuditRaw(database *db.DB, username, roleID string, userID int64, action, ip string) {
	action = truncateAudit(action, 480)
	if action == "" {
		return
	}
	_, err := database.Exec(
		`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
		time.Now().Format("2006-01-02 15:04:05"), truncateAudit(username, 60), truncateAudit(roleID, 30),
		userID, action, truncateAudit(ip, 60),
	)
	if err != nil {
		slog.Warn("写入审计日志失败", "err", err)
	}
}

func truncateAudit(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}
