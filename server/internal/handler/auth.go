package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// dummyPwdSalt / dummyPwdHash 供“账号不存在”分支做一次等价耗时的 scrypt 计算，
// 让“用户不存在”与“密码错误”两条路径的响应时间接近，避免时间侧信道枚举账号。
var dummyPwdSalt, dummyPwdHash = func() (string, string) {
	s, h, err := auth.HashPassword("dummy-password-for-timing")
	if err != nil {
		return "00000000000000000000000000000000", strings.Repeat("0", 64)
	}
	return s, h
}()

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

		// ---- 登录失败锁定（账号 + IP 双维度，防止恶意锁死他人账号）----
		// 锁定检查放在最前：已锁定的请求不再校验验证码，避免变成免费答题机。
		if locked, mins := loginGuardLocked(guardKey); locked {
			writeAuditRaw(database, body.Username, "", 0, "登录失败：账号在该 IP 下已临时锁定", ip)
			c.AbortWithStatusJSON(http.StatusLocked, gin.H{
				"error":   "该账号在当前网络下失败次数过多，请 " + itoa(mins) + " 分钟后再试",
				"minutes": mins,
			})
			return
		}

		// ---- 验证码校验（错误同样计入锁定，阈值放宽为密码阈值的 2 倍）----
		if cfg.CaptchaEnabled && !verifyCaptcha(c, body.CaptchaID, body.CaptchaCode) {
			locked, mins := loginGuardCaptchaFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			writeAuditRaw(database, body.Username, "", 0, "登录失败：验证码错误", ip)
			if locked {
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "验证码错误次数过多，该账号在当前网络下已被锁定 " + itoa(mins) + " 分钟",
					"minutes": mins,
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "验证码错误，请重新获取"})
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
			// 账号不存在时也跑一次 scrypt，避免用响应时间区分“用户是否存在”
			auth.VerifyPassword(body.Password, dummyPwdSalt, dummyPwdHash)
			locked, mins := loginGuardFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			writeAuditRaw(database, body.Username, "", 0, "登录失败：账号或密码错误", ip)
			if locked {
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "密码错误次数过多，该账号在当前网络下已被锁定 " + itoa(mins) + " 分钟",
					"minutes": mins,
				})
				return
			}
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 先验密码：密码未通过的请求一律得到同一句话，
		// 不允许凭响应区分“账号已停用 / 角色不符 / 密码错误”。
		if !auth.VerifyPassword(body.Password, pwdSalt, pwdHash) {
			locked, mins := loginGuardFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			slog.Info("登录失败", "username", body.Username, "ip", ip)
			if locked {
				writeAuditRaw(database, body.Username, "", uid, "登录失败：密码错误次数过多，已临时锁定", ip)
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "密码错误次数过多，该账号在当前网络下已被锁定 " + itoa(mins) + " 分钟",
					"minutes": mins,
				})
				return
			}
			writeAuditRaw(database, body.Username, "", uid, "登录失败：账号或密码错误", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 密码正确 → 清除失败计数与历史锁定字段
		loginGuardSuccess(guardKey)
		database.Exec(`UPDATE users SET login_attempts=0, locked_until=NULL WHERE id=?`, uid)

		// 密码通过后再判状态与角色；任何不符合都返回与“密码错误”完全相同的文案与状态码。
		if status != "启用" {
			writeAuditRaw(database, body.Username, "", uid, "登录失败：账号已停用", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 取该账号当前可用的已绑定角色（只含启用角色）。
		// 只有验证码、密码与账号状态全部通过后才会执行到这里，
		// 未通过凭据校验的请求不会看到任何角色信息。
		roles, err := queryLoginRoles(database, uid)
		if err != nil {
			slog.Error("查询登录角色失败", "err", err, "uid", uid)
			util.Fail(c, http.StatusInternalServerError, "登录失败，请稍后重试")
			return
		}
		if len(roles) == 0 {
			writeAuditRaw(database, body.Username, "", uid, "登录失败：未绑定可用角色", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 单角色：行为与改造前一致，直接签发会话。
		if len(roles) == 1 {
			finishLogin(c, svc, database, cfg, uid, body.Username, roles[0].ID, ip)
			return
		}

		// 多角色：旧客户端显式传 role 时仍可使用，但必须命中本人的启用角色快照。
		if explicit := strings.TrimSpace(body.Role); explicit != "" {
			if findLoginRole(roles, explicit) == nil {
				writeAuditRaw(database, body.Username, explicit, uid, "登录失败：不具备所选角色", ip)
				util.Unauthorized(c, "账号或密码错误")
				return
			}
			finishLogin(c, svc, database, cfg, uid, body.Username, explicit, ip)
			return
		}

		// 多角色且未指定：返回待选角色与一次性凭据，本响应不下发 token。
		ticket, expiresIn := issueRoleTicket(uid, body.Username, roles)
		if ticket == "" {
			util.Fail(c, http.StatusInternalServerError, "登录失败，请稍后重试")
			return
		}
		defaultRole := roles[0].ID
		var mainRole string
		if err := database.QueryRow(`SELECT role_id FROM users WHERE id=?`, uid).Scan(&mainRole); err == nil {
			if findLoginRole(roles, mainRole) != nil {
				defaultRole = mainRole
			}
		}
		writeAuditRaw(database, body.Username, "", uid, "登录校验通过：待选择登录角色", ip)
		c.JSON(http.StatusOK, gin.H{
			"needRole":    true,
			"ticket":      ticket,
			"roles":       roles,
			"defaultRole": defaultRole,
			"expiresIn":   expiresIn,
		})
	}
}

// ---- POST /api/auth/login/role ----
// 多角色账号第二步：凭一次性票据 + 所选角色换 token。
// 票据无论 redeem 成败都立即作废；角色必须命中第一步生成的快照，
// 且账号、绑定、角色状态需再次复核，避免快照生成后被停用仍可登录。
func LoginSelectRole(svc *auth.Service, database *db.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Ticket string `json:"ticket" binding:"required"`
			Role   string `json:"role" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "请选择本次登录身份")
			return
		}
		ticketID := strings.TrimSpace(body.Ticket)
		roleID := strings.TrimSpace(body.Role)
		ip := c.ClientIP()

		t, ok := redeemRoleTicket(ticketID)
		if !ok || t == nil {
			writeAuditRaw(database, "", roleID, 0, "登录失败：身份选择凭据无效或已过期", ip)
			util.Unauthorized(c, "登录状态已过期，请重新登录")
			return
		}

		guardKey := loginGuardKey(t.Username, ip)
		if locked, mins := loginGuardLocked(guardKey); locked {
			writeAuditRaw(database, t.Username, roleID, t.UserID, "登录失败：账号在该 IP 下已临时锁定", ip)
			c.AbortWithStatusJSON(http.StatusLocked, gin.H{
				"error":   "该账号在当前网络下失败次数过多，请 " + itoa(mins) + " 分钟后再试",
				"minutes": mins,
			})
			return
		}

		// 角色必须来自本人快照，否则按一次登录失败处理。
		if findLoginRole(t.Roles, roleID) == nil {
			locked, mins := loginGuardFailure(guardKey, cfg.LoginMaxFailures, cfg.LoginLockMinutes)
			writeAuditRaw(database, t.Username, roleID, t.UserID, "登录失败：所选身份无效", ip)
			if locked {
				c.AbortWithStatusJSON(http.StatusLocked, gin.H{
					"error":   "密码错误次数过多，该账号在当前网络下已被锁定 " + itoa(mins) + " 分钟",
					"minutes": mins,
				})
				return
			}
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		// 二次复核：账号仍启用、绑定仍存在、角色仍启用。
		var status string
		if err := database.QueryRow(`SELECT status FROM users WHERE id=?`, t.UserID).Scan(&status); err != nil || status != "启用" {
			writeAuditRaw(database, t.Username, roleID, t.UserID, "登录失败：账号已停用", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}
		var held int
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id
			 WHERE ur.user_id=? AND ur.role_id=? AND r.status='启用'`,
			t.UserID, roleID,
		).Scan(&held); err != nil || held == 0 {
			writeAuditRaw(database, t.Username, roleID, t.UserID, "登录失败：不具备所选角色或角色已停用", ip)
			util.Unauthorized(c, "账号或密码错误")
			return
		}

		loginGuardSuccess(guardKey)
		finishLogin(c, svc, database, cfg, t.UserID, t.Username, roleID, ip)
	}
}

// queryLoginRoles 返回该用户当前可用的已绑定角色（只含启用角色，按角色排序）。
func queryLoginRoles(database *db.DB, uid int64) ([]auth.Role, error) {
	rows, err := database.Query(
		`SELECT r.id,r.name,r.short,r.icon,r.color,r.descr,r.nav,`+"`external`"+`
		 FROM user_roles ur JOIN roles r ON r.id=ur.role_id
		 WHERE ur.user_id=? AND r.status='启用' ORDER BY r.seq`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]auth.Role, 0, 4)
	for rows.Next() {
		var id, name, short, icon, color, descr, nav sql.NullString
		var ext sql.NullInt64
		if err := rows.Scan(&id, &name, &short, &icon, &color, &descr, &nav, &ext); err != nil {
			continue
		}
		out = append(out, auth.Role{
			ID:       nullStr(id),
			Name:     nullStr(name),
			Short:    nullStr(short),
			Icon:     nullStr(icon),
			Color:    nullStr(color),
			Desc:     nullStr(descr),
			Nav:      util.ParseJSONArr(nullStr(nav)),
			External: ext.Valid && ext.Int64 == 1,
		})
	}
	return out, rows.Err()
}

func findLoginRole(roles []auth.Role, roleID string) *auth.Role {
	for i := range roles {
		if roles[i].ID == roleID {
			return &roles[i]
		}
	}
	return nil
}

// finishLogin 完成会话签发与响应：第一步（单角色/显式角色）与第二步身份选择共用，
// 保证两条路径的令牌、媒体 Cookie、审计与响应结构完全一致。
func finishLogin(c *gin.Context, svc *auth.Service, database *db.DB, cfg *config.Config, uid int64, username, roleID, ip string) {
	var mustChange int
	if err := database.QueryRow(`SELECT must_change FROM users WHERE id=?`, uid).Scan(&mustChange); err != nil {
		util.Unauthorized(c, "账号或密码错误")
		return
	}
	// 角色状态兜底复核：快照生成后角色可能被停用。
	var roleStatus string
	if err := database.QueryRow(`SELECT COALESCE(status,'启用') FROM roles WHERE id=?`, roleID).Scan(&roleStatus); err != nil || roleStatus != "启用" {
		writeAuditRaw(database, username, roleID, uid, "登录失败：角色不可用", ip)
		util.Unauthorized(c, "账号或密码错误")
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

	slog.Info("登录成功", "username", username, "role", roleID, "ip", ip)
	writeAuditRaw(database, username, roleID, uid, "登录成功", ip)
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
// Path 必须覆盖 /uploads 与 /media 两个前缀：图片内联预览走 /media，
// 若仍只设 /uploads，浏览器不会在 <img src="/media/..."> 请求上带 Cookie，预览会 401。
func setMediaCookie(c *gin.Context, token string, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.MediaCookieName, token, int(cfg.JWTTTL.Seconds()), "/", "", cfg.TLSEnabled, true)
}

func clearMediaCookie(c *gin.Context, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.MediaCookieName, "", -1, "/", "", cfg.TLSEnabled, true)
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
