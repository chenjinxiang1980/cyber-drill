package handler

import (
	"database/sql"
	"log/slog"
	"regexp"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// --- auth 补充接口 ---

// GetAccounts GET /api/auth/accounts —— 账号列表（仅平台管理员）
// 登录页不再匿名获取账号姓名/单位/角色；该接口仅供 admin.html 维护用户时使用。
// 返回 [{username,name,org,role,roles:[{id,name,short,color,external}]}]
func GetAccounts(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 先查所有启用的用户
		uRows, err := database.Query(
			`SELECT id,username,name,org,role_id FROM users WHERE status='启用' ORDER BY id`,
		)
		if err != nil {
			slog.Warn("GetAccounts 查询 users 失败", "err", err)
			util.Fail(c, 500, "读取账号列表失败")
			return
		}
		defer uRows.Close()

		type uRow struct {
			id       int64
			username string
			name     string
			org      string
			roleID   string
		}
		var users []uRow
		for uRows.Next() {
			var id int64
			var username, name, org, roleID sql.NullString
			if err := uRows.Scan(&id, &username, &name, &org, &roleID); err != nil {
				continue
			}
			users = append(users, uRow{
				id:       id,
				username: nullStr(username),
				name:     nullStr(name),
				org:      nullStr(org),
				roleID:   nullStr(roleID),
			})
		}

		// 查 user_roles JOIN roles（只取启用的角色）
		urRows, err := database.Query(
			`SELECT ur.user_id, ur.role_id, r.name, r.short, r.color, r.` + "`external`" + `
			 FROM user_roles ur
			 JOIN roles r ON ur.role_id=r.id
			 WHERE r.status='启用'
			 ORDER BY ur.user_id, r.seq`,
		)
		if err != nil {
			slog.Warn("GetAccounts 查询 user_roles 失败", "err", err)
		}
		defer urRows.Close()

		roleMap := map[int64][]gin.H{}
		for urRows != nil && urRows.Next() {
			var uid int64
			var rid, rname, rshort, rcolor sql.NullString
			var rext sql.NullInt64
			if err := urRows.Scan(&uid, &rid, &rname, &rshort, &rcolor, &rext); err != nil {
				continue
			}
			roleMap[uid] = append(roleMap[uid], gin.H{
				"id":       nullStr(rid),
				"name":     nullStr(rname),
				"short":    nullStr(rshort),
				"color":    nullStr(rcolor),
				"external": rext.Valid && rext.Int64 == 1,
			})
		}

		out := make([]gin.H, 0, len(users))
		for _, u := range users {
			roles := roleMap[u.id]
			if len(roles) == 0 {
				continue // 全部角色停用则不显示
			}
			primary := roles[0]
			out = append(out, gin.H{
				"username": u.username,
				"name":     u.name,
				"org":      u.org,
				"role":     primary["id"],
				"roleName": primary["name"],
				"short":    primary["short"],
				"color":    primary["color"],
				"external": primary["external"],
				"roles":    roles,
			})
		}
		c.JSON(200, out)
	}
}

// GetRoles GET /api/auth/roles —— 免登录，只读启用角色（含 nav JSON parse）
func GetRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := database.Query(
			`SELECT id,name,short,icon,color,descr,nav,` + "`external`" + `
			 FROM roles WHERE status='启用' ORDER BY seq`,
		)
		if err != nil {
			slog.Warn("GetRoles 失败", "err", err)
			util.Fail(c, 500, "读取角色列表失败")
			return
		}
		defer rows.Close()
		out := make([]gin.H, 0)
		for rows.Next() {
			var id, name, short, icon, color, descr sql.NullString
			var navStr sql.NullString
			var ext sql.NullInt64
			if err := rows.Scan(&id, &name, &short, &icon, &color, &descr, &navStr, &ext); err != nil {
				continue
			}
			out = append(out, gin.H{
				"id":       nullStr(id),
				"name":     nullStr(name),
				"short":    nullStr(short),
				"icon":     nullStr(icon),
				"color":    nullStr(color),
				"desc":     nullStr(descr),
				"nav":      util.ParseJSONArr(nullStr(navStr)),
				"external": ext.Valid && ext.Int64 == 1,
			})
		}
		c.JSON(200, out)
	}
}

// --- 密码校验正则（与 Node 版对齐）---

var (
	pwdLenRe    = regexp.MustCompile(`^.{8,}$`)
	pwdLetterRe = regexp.MustCompile(`[A-Za-z]`)
	pwdDigitRe  = regexp.MustCompile(`[0-9]`)
)

// ChangePassword POST /api/auth/change-password —— 修改当前用户密码
// 路由层已挂 RequireAuth；待改密受限令牌也放行本接口。
// 成功后吊销该用户全部旧会话，签发正式 token 并写 /uploads 媒体 Cookie。
func ChangePassword(database *db.DB, authSvc *auth.Service, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			util.Unauthorized(c, "未登录")
			return
		}

		var body struct {
			OldPassword string `json:"oldPassword"`
			NewPassword string `json:"newPassword"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "请输入原密码和新密码")
			return
		}
		body.OldPassword = trimSpace(body.OldPassword)
		body.NewPassword = trimSpace(body.NewPassword)

		// 1. 新密码强度校验：至少 8 位，含字母 + 数字
		if !pwdLenRe.MatchString(body.NewPassword) ||
			!pwdLetterRe.MatchString(body.NewPassword) ||
			!pwdDigitRe.MatchString(body.NewPassword) {
			util.BadRequest(c, "新密码需至少 8 位，且同时包含字母与数字")
			return
		}

		// 2. 查当前用户，验证原密码
		var pwdSalt, pwdHash string
		err := database.QueryRow(
			`SELECT pwd_salt,pwd_hash FROM users WHERE id=?`, u.ID,
		).Scan(&pwdSalt, &pwdHash)
		if err != nil {
			slog.Warn("ChangePassword 查用户失败", "err", err, "uid", u.ID)
			util.Fail(c, 500, "修改失败")
			return
		}
		if !auth.VerifyPassword(body.OldPassword, pwdSalt, pwdHash) {
			util.BadRequest(c, "原密码不正确")
			return
		}

		// 3. 散列新密码
		newSalt, newHash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			slog.Warn("ChangePassword HashPassword 失败", "err", err)
			util.Fail(c, 500, "修改失败")
			return
		}

		// 4. 更新密码 + 清除 must_change
		if _, err := database.Exec(
			`UPDATE users SET pwd_salt=?, pwd_hash=?, must_change=0, login_attempts=0, locked_until=NULL WHERE id=?`,
			newSalt, newHash, u.ID,
		); err != nil {
			slog.Warn("ChangePassword UPDATE 失败", "err", err, "uid", u.ID)
			util.Fail(c, 500, "修改失败")
			return
		}

		// 5. 吊销全部旧会话（含当前受限会话），随后签发正式 token
		database.Exec(`DELETE FROM sessions WHERE user_id=?`, u.ID)
		// 安装期生成的一次性初始口令已使用，清空部署 .env 中的明文（尽力而为）。
		invalidateInitialPassword(u.Username)
		token, err := authSvc.Issue(u.ID, u.Role.ID)
		if err != nil {
			slog.Warn("ChangePassword 重新签发会话失败", "err", err, "uid", u.ID)
			c.JSON(200, gin.H{"ok": true, "time": util.NowSec()})
			return
		}
		setMediaCookie(c, token, cfg)
		writeAudit(database, u, c, "修改登录密码")
		c.JSON(200, gin.H{"ok": true, "time": util.NowSec(), "token": token})
	}
}

func trimSpace(s string) string {
	// 避免引入 strings 包（Go 1.21+ 有 strings.TrimSpace），
	// 但这里用简单实现；或者直接引入 strings
	return trim(s)
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
