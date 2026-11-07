package handler

import (
	"log/slog"
	"net/http"
	"strings"

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
			Username    string `json:"username" binding:"required"`
			Password    string `json:"password" binding:"required"`
			Role        string `json:"role"`
			CaptchaID   string `json:"captchaId"`
			CaptchaCode string `json:"captchaCode"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "请输入账号和密码")
			return
		}
		body.Username = strings.TrimSpace(body.Username)
		ip := c.ClientIP()
		guardKey := loginGuardKey(body.Username, ip)

		// ---- 验证码校验 ----
		if cfg.CaptchaEnabled {
			if !verifyCaptcha(c, body.CaptchaID, body.CaptchaCode) {
				writeAuditRaw(database, body.Username, "", 0, "登录失败：验证码错误", ip)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "验证码错误，请重新获取"})
				return
			}
		}

		// ---- 登录失败锁定（账号 + IP 双维度，防止恶意锁死他人账号）----
		if locked, mins := loginGuardLocked(guardKey); locked {
			writeAuditRaw(database, body.Username, "", 0, "登录失败：账号在该 IP 下已临时锁定", ip)
			c.AbortWithStatusJSON(http.StatusLocked, gin.H{
				"error":   "该账号在当前网络下失败次数过多，请 " + itoa(mins) + " 分钟后再试",
				"minutes": mins,
			})
			return
		}

		// 查用户（不区分用户不存在和密码错误，防枚举）
		var uid int64
		var uName, org, pwdSalt, pwdHash, status string
		var mustChange int
		err := database.QueryRow(
			`SELECT id,name,org,pwd_salt,pwd_hash,status,must_change FROM users WHERE username=?`,
			body.Username,
		).Scan(&uid, &uName, &org, &pwdSalt, &pwdHash, &status, &mustChange)
		if err != nil {
			loginGuardFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			writeAuditRaw(database, body.Username, "", 0, "登录失败：账号或密码错误", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		if status != "启用" {
			writeAuditRaw(database, body.Username, "", uid, "登录失败：账号已停用", ip)
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
				writeAuditRaw(database, body.Username, roleID, uid, "登录失败：不具备所选角色", ip)
				util.BadRequest(c, "该账号不具备所选角色")
				return
			}
		}

		// 校验密码
		if !auth.VerifyPassword(body.Password, pwdSalt, pwdHash) {
			locked, mins := loginGuardFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			slog.Info("登录失败", "username", body.Username, "ip", ip)
			if locked {
				writeAuditRaw(database, body.Username, roleID, uid, "登录失败：密码错误次数过多，已临时锁定", ip)
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "密码错误次数过多，该账号在当前网络下已被锁定 " + itoa(mins) + " 分钟",
					"minutes": mins,
				})
				return
			}
			writeAuditRaw(database, body.Username, roleID, uid, "登录失败：账号或密码错误", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 密码正确 → 清除失败计数与历史锁定字段
		loginGuardSuccess(guardKey)
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

		// 签发 JWT：必须改密时只签发受限作用域令牌
		scope := ""
		if mustChange == 1 {
			scope = auth.ScopePasswordChange
		}
		token, err := svc.IssueScoped(uid, roleID, scope)
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

		// 正式会话才发放 /uploads 媒体 Cookie；受限会话必须先把密码改完
		if !u.MustChange && u.Scope == "" {
			setMediaCookie(c, token, cfg)
		}

		slog.Info("登录成功", "username", body.Username, "role", roleID, "ip", ip)
		writeAuditRaw(database, body.Username, roleID, uid, "登录成功", ip)
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

// setMediaCookie 发放 /uploads 目录的 HttpOnly 会话 Cookie。
// 浏览器 <img src="/uploads/...">、<a download> 无法附加自定义 Header，
// 用同源 Cookie 完成鉴权，避免把附件目录暴露为匿名静态目录。
func setMediaCookie(c *gin.Context, token string, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.MediaCookieName, token, int(cfg.JWTTTL.Seconds()), "/uploads", "", cfg.TLSEnabled, true)
}

func clearMediaCookie(c *gin.Context, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.MediaCookieName, "", -1, "/uploads", "", cfg.TLSEnabled, true)
}

// ---- POST /api/auth/logout ----
func Logout(svc *auth.Service, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := auth.TokenOf(c)
		if token != "" {
			svc.Destroy(token)
		}
		clearMediaCookie(c, cfg)
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
