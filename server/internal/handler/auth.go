package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ---- POST /api/auth/login ----
// 入参 {username, password, role?, captchaId?, captchaCode?}
// 出参 {token, user:{...}, role:{...}}
func Login(svc *auth.Service, database *db.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Username   string `json:"username" binding:"required"`
			Password   string `json:"password" binding:"required"`
			Role       string `json:"role"`
			CaptchaID  string `json:"captchaId"`
			CaptchaCode string `json:"captchaCode"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "请输入账号和密码")
			return
		}
		body.Username = strings.TrimSpace(body.Username)

		// ---- 验证码校验 ----
		if cfg.CaptchaEnabled {
			if !verifyCaptcha(c, body.CaptchaID, body.CaptchaCode) {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "验证码错误，请重新获取"})
				return
			}
		}

		// 查用户（同时取锁定状态）
		var uid int64
		var uName, org, pwdSalt, pwdHash, status string
		var mustChange int
		var loginAttempts int
		var lockedUntil sql.NullTime
		err := database.QueryRow(
			`SELECT id,name,org,pwd_salt,pwd_hash,status,must_change,login_attempts,locked_until FROM users WHERE username=?`,
			body.Username,
		).Scan(&uid, &uName, &org, &pwdSalt, &pwdHash, &status, &mustChange, &loginAttempts, &lockedUntil)
		if err != nil {
			// 不区分用户不存在和密码错误（防枚举）
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// ---- 锁定检查 ----
		if lockedUntil.Valid && lockedUntil.Time.After(time.Now()) {
			// 计算剩余分钟
			remaining := int(lockedUntil.Time.Sub(time.Now()).Minutes())
			if remaining < 1 {
				remaining = 1
			}
			c.AbortWithStatusJSON(http.StatusLocked, gin.H{
				"error":   "账号已被临时锁定，请 " + itoa(remaining) + " 分钟后再试",
				"minutes": remaining,
			})
			return
		}
		// 锁定期已过 → 自动清零
		if lockedUntil.Valid {
			database.Exec(`UPDATE users SET login_attempts=0, locked_until=NULL WHERE id=?`, uid)
			loginAttempts = 0
		}

		if status != "启用" {
			util.Forbidden(c, "账号已停用，请联系平台管理员")
			return
		}

		// 取登录角色
		roleID := strings.TrimSpace(body.Role)
		if roleID == "" {
			if err := database.QueryRow(`SELECT role_id FROM users WHERE id=?`, uid).Scan(&roleID); err != nil {
				util.BadRequest(c, "用户未绑定角色")
				return
			}
		} else {
			var held int
			_ = database.QueryRow(
				`SELECT COUNT(*) FROM user_roles WHERE user_id=? AND role_id=?`, uid, roleID,
			).Scan(&held)
			if held == 0 {
				util.BadRequest(c, "该账号不具备所选角色")
				return
			}
		}

		// 校验密码
		if !auth.VerifyPassword(body.Password, pwdSalt, pwdHash) {
			slog.Info("登录失败", "username", body.Username, "ip", c.ClientIP(), "attempts", loginAttempts+1)
			newAttempts := loginAttempts + 1
			if newAttempts >= cfg.LoginMaxFailures {
				lockUntil := time.Now().Add(time.Duration(cfg.LoginLockMinutes) * time.Minute)
				database.Exec(`UPDATE users SET login_attempts=?, locked_until=? WHERE id=?`,
					newAttempts, lockUntil.Format("2006-01-02 15:04:05"), uid)
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "密码错误次数过多，账号已被锁定 " + itoa(cfg.LoginLockMinutes) + " 分钟",
					"minutes": cfg.LoginLockMinutes,
				})
				return
			}
			database.Exec(`UPDATE users SET login_attempts=? WHERE id=?`, newAttempts, uid)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 密码正确 → 清零锁定状态
		database.Exec(`UPDATE users SET login_attempts=0, locked_until=NULL WHERE id=?`, uid)

		// 角色状态检查
		var roleStatus string
		if err := database.QueryRow(`SELECT COALESCE(status,'启用') FROM roles WHERE id=?`, roleID).Scan(&roleStatus); err != nil {
			util.BadRequest(c, "角色不存在")
			return
		}
		if roleStatus != "启用" {
			util.Forbidden(c, "该角色已停用，请选择其他角色")
			return
		}

		// 清理过期会话
		svc.CleanExpired()

		// 签发 JWT
		token, err := svc.Issue(uid, roleID)
		if err != nil {
			slog.Error("签发会话失败", "err", err)
			util.Fail(c, http.StatusInternalServerError, "会话创建失败")
			return
		}

		u, err := svc.UserByToken(token)
		if err != nil || u == nil {
			svc.Destroy(token)
			util.Fail(c, http.StatusInternalServerError, "会话验证失败")
			return
		}

		slog.Info("登录成功", "username", body.Username, "role", roleID, "ip", c.ClientIP())
		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"user": gin.H{
				"username":   u.Username,
				"name":       u.Name,
				"org":        u.Org,
				"mustChange": u.MustChange,
			},
			"role": gin.H{
				"id":       u.Role.ID,
				"name":     u.Role.Name,
				"short":    u.Role.Short,
				"icon":     u.Role.Icon,
				"color":    u.Role.Color,
				"desc":     u.Role.Desc,
				"nav":      u.Role.Nav,
				"external": u.Role.External,
			},
		})
	}
}

// itoa 简单 int → string
func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// ---- POST /api/auth/logout ----
func Logout(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := auth.TokenOf(c)
		if token != "" {
			svc.Destroy(token)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ---- GET /api/auth/me ----
func Me() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			util.Unauthorized(c, "未登录")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"username":   u.Username,
			"name":       u.Name,
			"org":        u.Org,
			"mustChange": u.MustChange,
			"role":       u.Role,
			"roles":      u.Roles,
		})
	}
}
