package handler

import (
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// adminWhitelist 服务端硬校验的基线账号名单。
// 开源仓库不内置任何真实账号：默认保留 admin / platformadmin（平台管理员账号），
// 其余由部署方通过 ADMIN_BASELINE_USERS（逗号分隔）在 .env 中补充，避免把账号清单写进源码。
var adminWhitelist = loadAdminWhitelist()

func loadAdminWhitelist() map[string]bool {
	m := map[string]bool{"admin": true, "platformadmin": true}
	for _, u := range strings.Split(os.Getenv("ADMIN_BASELINE_USERS"), ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			m[u] = true
		}
	}
	return m
}

// testAccountPattern 只有明确像测试账号的用户名才允许被清理。
// 业务账号（如 zhangsan、doctor01）即使不在白名单也不会被删除，避免误删真实账号。
var testAccountPattern = regexp.MustCompile(`^(test|demo|guest|tmp|try|ceshi)[0-9_-]*$`)

func isTestAccount(username string) bool {
	return testAccountPattern.MatchString(strings.ToLower(strings.TrimSpace(username)))
}

// AdminCleanupTestAccounts POST /api/admin/users/cleanup-test-accounts
// 一键清理所有不在基线白名单里的账号（dry-run 默认，执行需 confirm 口令）
func AdminCleanupTestAccounts(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			DryRun  bool   `json:"dryRun"`
			Confirm string `json:"confirm"`
		}
		// 缺省 true = dry-run 安全模式
		if err := c.ShouldBindJSON(&body); err != nil {
			body.DryRun = true
		}

		// 统计当前 users 总数（执行前）
		var usersCount, userRolesCount int
		database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&usersCount)
		database.QueryRow(`SELECT COUNT(*) FROM user_roles`).Scan(&userRolesCount)

		// 找出将被删除的账号（不在白名单里）
		type removableUser struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			RoleID   string `json:"roleId"`
		}
		rows, err := database.Query(`SELECT u.id, u.username, u.role_id FROM users u ORDER BY u.id`)
		if err != nil {
			slog.Error("Cleanup: 查 users 失败", "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败"})
			return
		}
		defer rows.Close()

		removable := make([]removableUser, 0)
		kept := 0
		protected := make([]map[string]any, 0)
		for rows.Next() {
			var r removableUser
			if err := rows.Scan(&r.ID, &r.Username, &r.RoleID); err != nil {
				continue
			}
			// 基线白名单 + 平台管理员账号永久保留（双重保险）
			if adminWhitelist[r.Username] || r.Username == "admin" || r.Username == "platformadmin" {
				kept++
				continue
			}
			// 只删"测试账号"：用户名不在测试前缀里的业务账号一律保护
			if !isTestAccount(r.Username) {
				protected = append(protected, map[string]any{
					"id": r.ID, "username": r.Username, "reason": "非测试账号，已保护",
				})
				continue
			}
			removable = append(removable, r)
		}

		response := gin.H{
			"dryRun":         body.DryRun,
			"removable":      removable,
			"protected":      protected,
			"keptCount":      kept,
			"usersCount":     usersCount,
			"userRolesCount": userRolesCount,
			"removedCount":   0,
			"policy":         "只删除测试账号（test*/demo*/guest*/tmp*/try*/ceshi*）；基线账号与业务账号永不删除",
		}

		// dry-run：不做任何删除
		if body.DryRun {
			action := "预览"
			if len(removable) == 0 {
				response["message"] = "预览：没有可删除的测试账号"
			} else {
				response["message"] = action + "：将删除 " + itoa(len(removable)) + " 个测试账号，未执行"
			}
			c.JSON(http.StatusOK, response)
			return
		}

		// 执行模式：必须带 confirm 口令
		if body.Confirm != "DELETE-TEST-ACCOUNTS" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "执行删除必须携带 confirm=DELETE-TEST-ACCOUNTS"})
			return
		}

		if len(removable) == 0 {
			response["message"] = "没有可删除的测试账号"
			c.JSON(http.StatusOK, response)
			return
		}

		// 清理前日志：删谁、为什么删、保护了谁
		names := make([]string, 0, len(removable))
		for _, u := range removable {
			names = append(names, u.Username)
		}
		slog.Info("Cleanup: 即将删除测试账号",
			"removable", names, "reason", "命中测试账号前缀且不在基线白名单",
			"keptBaseline", kept, "protectedNonTest", len(protected))

		// 开事务：先删 user_roles，再删 users
		tx, err := database.Begin()
		if err != nil {
			slog.Error("Cleanup: Begin 失败", "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "事务开启失败"})
			return
		}

		deletedIDs := make([]int64, 0, len(removable))
		usernames := make([]string, 0, len(removable))
		for _, u := range removable {
			// 先删 user_roles
			if _, err := tx.Exec(`DELETE FROM user_roles WHERE user_id=?`, u.ID); err != nil {
				tx.Rollback()
				slog.Error("Cleanup: DELETE user_roles 失败", "err", err, "user_id", u.ID)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "删除 user_roles 失败"})
				return
			}
			// 再删 users
			res, err := tx.Exec(`DELETE FROM users WHERE id=?`, u.ID)
			if err != nil {
				tx.Rollback()
				slog.Error("Cleanup: DELETE users 失败", "err", err, "user_id", u.ID)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "删除 users 失败"})
				return
			}
			n, _ := res.RowsAffected()
			if n > 0 {
				deletedIDs = append(deletedIDs, u.ID)
				usernames = append(usernames, u.Username)
			}
		}

		if err := tx.Commit(); err != nil {
			slog.Error("Cleanup: Commit 失败", "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
			return
		}

		// 执行后实际计数
		var newUsers, newRoles int
		database.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&newUsers)
		database.QueryRow(`SELECT COUNT(*) FROM user_roles`).Scan(&newRoles)

		response["removedCount"] = len(deletedIDs)
		response["usersCount"] = newUsers
		response["userRolesCount"] = newRoles
		response["message"] = "已删除 " + itoa(len(deletedIDs)) + " 个测试账号（事务内完成）"

		// 审计
		u := auth.CurrentUser(c)
		auditUser := ""
		auditUID := int64(0)
		auditRole := ""
		if u != nil {
			auditUser = u.Username
			auditUID = u.ID
			auditRole = u.Role.ID
		}
		ip := c.ClientIP()
		action := "cleanup-test-accounts: removed " + strings.Join(usernames, ",")
		database.Exec(
			`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
			time.Now().Format("2006-01-02 15:04:05"),
			auditUser, auditRole, auditUID, action, ip,
		)

		slog.Info("Cleanup: 测试账号清理完成", "removed", len(deletedIDs), "usernames", usernames, "operator", auditUser, "ip", ip)
		c.JSON(http.StatusOK, response)
	}
}
