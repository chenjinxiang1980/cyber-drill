package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

// ---------- 越权 helper: 与 Node 版 bootstrap.js MANAGED_ROLES 对齐 ----------
// Node MANAGED_ROLES = ['itlead', 'duty', 'netsec', 'sysadmin']
// platformadmin 全权限；commander 进不来 admin 组（RequireAdmin 拦）
// 外部单位角色 commander/healthbureau/cac/cyberpolice/medlead/clinical 一律不可动
var itleadManageable = map[string]bool{
	auth.RoleITLead:   true,
	auth.RoleDuty:     true,
	auth.RoleNetSec:   true,
	auth.RoleSysAdmin: true,
}

func canModifyRole(u *auth.User, targetRoleID string) bool {
	if u == nil {
		return false
	}
	if u.Role.ID == auth.RolePlatformAdmin {
		return true
	}
	if u.Role.ID == auth.RoleITLead {
		return itleadManageable[targetRoleID]
	}
	return false
}

// canModifyUserRole: 校验发起者 u 对目标用户 targetUserID 的所有角色（旧 + 新）是否有权
// newRoles 为空 []string{} 时仅校验 target 当前绑定的角色（适用于 DELETE 操作）
func canModifyUserRole(database *db.DB, u *auth.User, targetUserID int64, newRoles []string) (bool, string) {
	rows, _ := database.Query(`SELECT role_id FROM user_roles WHERE user_id=?`, targetUserID)
	defer rows.Close()
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			continue
		}
		if !canModifyRole(u, rid) {
			return false, rid
		}
	}
	for _, rid := range newRoles {
		if !canModifyRole(u, rid) {
			return false, rid
		}
	}
	return true, ""
}

// --- admin 系统管理（路由层已挂 RequireAdmin）---

// AdminGetUsers GET /api/admin/users —— 返回完整 users + createdAt（admin 专用，与 bootstrap 的 users 不同）
func AdminGetUsers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 先查 roleMap（用户绑定多个角色）
		type roleEntry struct {
			userID  int64
			roleID  string
			roleNum int
			name    string
			color   string
			short   string
			ext     bool
		}
		rows, err := database.Query(
			`SELECT ur.user_id, r.id, r.seq, r.name, r.color, r.short, r.` + "`external`" +
				` FROM user_roles ur JOIN roles r ON r.id=ur.role_id ORDER BY r.seq`,
		)
		if err != nil {
			slog.Warn("AdminGetUsers roleMap 失败", "err", err)
		}
		defer rows.Close()
		roleMap := map[int64][]gin.H{}
		for rows.Next() {
			var e roleEntry
			var extNum int
			if err := rows.Scan(&e.userID, &e.roleID, &e.roleNum, &e.name, &e.color, &e.short, &extNum); err != nil {
				continue
			}
			e.ext = extNum == 1
			roleMap[e.userID] = append(roleMap[e.userID], gin.H{
				"id":       e.roleID,
				"name":     e.name,
				"short":    e.short,
				"color":    e.color,
				"external": e.ext,
			})
		}

		rows2, err := database.Query(
			`SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,u.created_at,r.name AS role_name
			 FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id`,
		)
		if err != nil {
			slog.Warn("AdminGetUsers 失败", "err", err)
			c.JSON(200, []gin.H{})
			return
		}
		defer rows2.Close()
		out := make([]gin.H, 0)
		for rows2.Next() {
			var id int64
			var username, name, roleID, org, status, roleName, createdAt sql.NullString
			if err := rows2.Scan(&id, &username, &name, &roleID, &org, &status, &createdAt, &roleName); err != nil {
				continue
			}
			rid := util.ScanNullableString(roleID)
			rn := util.ScanNullableString(roleName)
			roleList := roleMap[id]
			if len(roleList) == 0 {
				roleList = []gin.H{{"id": rid, "name": rn, "short": "", "color": "", "external": false}}
			}
			rolesIDs := make([]string, 0, len(roleList))
			for _, r := range roleList {
				if x, ok := r["id"].(string); ok {
					rolesIDs = append(rolesIDs, x)
				}
			}
			out = append(out, gin.H{
				"id":        id,
				"username":  util.ScanNullableString(username),
				"name":      util.ScanNullableString(name),
				"role":      rid,
				"roleName":  rn,
				"roles":     rolesIDs,
				"roleList":  roleList,
				"org":       util.ScanNullableString(org),
				"status":    util.ScanNullableString(status),
				"createdAt": scanNS(createdAt),
			})
		}
		c.JSON(200, out)
	}
}

// AdminGetRoles GET /api/admin/roles —— 返回完整角色列表
// 直接返回数组，与 Node 版 admin.js 对齐。
func AdminGetRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryRoles(database))
	}
}

// AdminGetForms GET /api/admin/forms —— form_templates 完整列表
// 直接返回数组，与 Node 版 admin.js 对齐。
func AdminGetForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// code 是展示编号（F001…），排序按它；引用关系仍然用 id。
		rows, err := database.Query(
			`SELECT id,COALESCE(code,''),name,fields FROM form_templates ORDER BY code`,
		)
		if err != nil {
			// 老库未执行 0012（没有 code 列）时回退：按 id 排序，编号由 id 推导
			rows, err = database.Query(
				`SELECT id,'' AS code,name,fields FROM form_templates ORDER BY id`,
			)
		}
		if err != nil {
			slog.Warn("AdminGetForms 失败", "err", err)
			util.Fail(c, 500, "读取表单模板失败")
			return
		}
		defer rows.Close()
		out := make([]gin.H, 0)
		for rows.Next() {
			var id, code, name sql.NullString
			var fieldsStr sql.NullString
			if err := rows.Scan(&id, &code, &name, &fieldsStr); err != nil {
				continue
			}
			out = append(out, gin.H{
				"id":   nullStr(id),
				"code": formCodeOf(nullStr(id), nullStr(code)),
				"name": nullStr(name),
				// fields 存的是对象数组（{label,hint,required,type,options}），
				// 必须用 ParseJSONArrAny；用 []string 版本会把每个元素解析成空串。
				"fields": util.ParseJSONArrAny(nullStr(fieldsStr)),
			})
		}
		c.JSON(200, out)
	}
}

// csvSafeCell 防 CSV 公式注入：Excel/WPS 会把 = + - @ 以及 \t \r 开头的单元格
// 当作公式解析（如 =cmd|' /c calc'!A0），打开导出文件即可能触发命令。
// 统一前置单引号，使其保持文本；只处理首字符，不改变正常内容。
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// AdminGetAuditExport GET /api/admin/audit/export —— CSV 导出审计日志
// 头部：时间,操作人,角色,操作内容,来源IP
func AdminGetAuditExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := database.Query(
			`SELECT time,user,role_id,action,ip FROM audit_logs ORDER BY id`,
		)
		if err != nil {
			slog.Warn("AdminGetAuditExport 查询失败", "err", err)
			util.Fail(c, 500, "导出失败")
			return
		}
		defer rows.Close()

		writeAudit(database, auth.CurrentUser(c), c, "导出审计日志（CSV）")
		filename := "audit_export_" + time.Now().Format("20060102_150405") + ".csv"
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s"`, filename))

		c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
		w := csv.NewWriter(c.Writer)
		defer w.Flush()

		// 表头
		if err := w.Write([]string{"时间", "操作人", "角色", "操作内容", "来源IP"}); err != nil {
			return
		}

		for rows.Next() {
			var tm, usr, roleID, action, ip sql.NullString
			if err := rows.Scan(&tm, &usr, &roleID, &action, &ip); err != nil {
				continue
			}
			_ = w.Write([]string{
				csvSafeCell(nullStr(tm)),
				csvSafeCell(nullStr(usr)),
				csvSafeCell(nullStr(roleID)),
				csvSafeCell(nullStr(action)),
				csvSafeCell(nullStr(ip)),
			})
		}
		// 让 csv 结束后不输出 JSON 错误
		c.Status(http.StatusOK)
	}
}

// ========== A 块写操作: users / roles / forms ==========

var usernameRegex = regexp.MustCompile(`^(?i)[a-z0-9_]{3,16}$`)

// ---- POST /api/admin/users —— 新增用户 ----
func AdminPostUsers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Username string   `json:"username"`
			Name     string   `json:"name"`
			Role     string   `json:"role"`
			Org      string   `json:"org"`
			Password string   `json:"password"`
			Roles    []string `json:"roles"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "参数格式错误")
			return
		}
		body.Username = strings.TrimSpace(body.Username)
		body.Name = strings.TrimSpace(body.Name)
		if body.Username == "" || body.Name == "" {
			util.BadRequest(c, "用户名和姓名必填")
			return
		}
		if !usernameRegex.MatchString(body.Username) {
			util.BadRequest(c, "用户名 3-16 位，仅支持字母数字下划线")
			return
		}
		// 角色：优先 body.Roles[0]，其次 body.Role
		primaryRole := body.Role
		if len(body.Roles) > 0 {
			primaryRole = body.Roles[0]
		}
		if primaryRole == "" {
			util.BadRequest(c, "角色必填")
			return
		}
		// 角色越权校验
		if !canModifyRole(u, primaryRole) {
			util.Forbidden(c, "无权限创建角色 "+primaryRole+" 的用户")
			return
		}
		// 角色存在性
		var cnt int
		if err := database.QueryRow(`SELECT COUNT(*) FROM roles WHERE id=?`, primaryRole).Scan(&cnt); err != nil || cnt == 0 {
			util.BadRequest(c, "角色不存在: "+primaryRole)
			return
		}
		// username 唯一
		if err := database.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, body.Username).Scan(&cnt); err == nil && cnt > 0 {
			util.BadRequest(c, "用户名已存在")
			return
		}
		// 多角色越权校验
		for _, rid := range body.Roles {
			if rid != primaryRole && !canModifyRole(u, rid) {
				util.Forbidden(c, "无权限绑定角色 "+rid)
				return
			}
		}
		// 初始口令：一律随机生成，明文只在本次响应中返回一次；首次登录强制改密。
		pwd, err := auth.GenerateTempPassword()
		if err != nil {
			util.Fail(c, 500, "生成初始口令失败")
			return
		}
		salt, hash, err := auth.HashPassword(pwd)
		if err != nil {
			util.Fail(c, 500, "密码散列失败")
			return
		}
		res, err := database.Exec(`INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,must_change,status,created_at) VALUES (?,?,?,?,?,?,1,'启用',NOW())`,
			body.Username, body.Name, primaryRole, body.Org, salt, hash)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				util.BadRequest(c, "用户名已存在")
				return
			}
			util.Fail(c, 500, "创建用户失败: "+err.Error())
			return
		}
		newID, _ := res.LastInsertId()
		// 绑定主角色 + 追加的多角色
		database.Exec(`DELETE FROM user_roles WHERE user_id=?`, newID)
		database.Exec(`INSERT INTO user_roles (user_id,role_id) VALUES (?,?)`, newID, primaryRole)
		for _, rid := range body.Roles {
			if rid != primaryRole {
				database.Exec(`INSERT INTO user_roles (user_id,role_id) VALUES (?,?)`, newID, rid)
			}
		}
		writeAudit(database, u, c, "新建用户："+body.Username+"（角色 "+primaryRole+"）")
		util.OK(c, gin.H{"ok": true, "id": newID, "username": body.Username, "initialPassword": pwd, "mustChange": true})
	}
}

// ---- PATCH /api/admin/users/:id —— 改用户 ----
func AdminPatchUsers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		idStr := c.Param("id")
		var body struct {
			Name   string   `json:"name"`
			Role   string   `json:"role"`
			Org    string   `json:"org"`
			Status string   `json:"status"`
			Roles  []string `json:"roles"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "参数格式错误")
			return
		}
		var id int64
		fmt.Sscanf(idStr, "%d", &id)
		if id == 0 {
			util.BadRequest(c, "无效用户 ID")
			return
		}
		// 用户存在 + 当前 role
		var curRole, curUsername string
		var curStatus string
		err := database.QueryRow(`SELECT role_id,username,status FROM users WHERE id=?`, id).Scan(&curRole, &curUsername, &curStatus)
		if err != nil {
			util.Fail(c, 404, "用户不存在")
			return
		}
		// 不能停用自己（admin 版）
		if id == u.ID && body.Status == "停用" {
			util.Forbidden(c, "不能停用自己")
			return
		}
		// 越权：能改这个用户当前绑定的所有角色 + 要改成的新角色吗？
		newRoles := []string{}
		if body.Role != "" {
			newRoles = append(newRoles, body.Role)
		}
		for _, rid := range body.Roles {
			if rid != "" {
				newRoles = append(newRoles, rid)
			}
		}
		if ok, badRole := canModifyUserRole(database, u, id, newRoles); !ok {
			util.Forbidden(c, "无权限修改角色 "+badRole+" 的用户")
			return
		}
		// UPDATE
		updates := []string{}
		args := []any{}
		if body.Name != "" {
			updates = append(updates, "name=?")
			args = append(args, body.Name)
		}
		if body.Role != "" {
			updates = append(updates, "role_id=?")
			args = append(args, body.Role)
		}
		if body.Org != "" {
			updates = append(updates, "org=?")
			args = append(args, body.Org)
		}
		if body.Status != "" {
			updates = append(updates, "status=?")
			args = append(args, body.Status)
		}
		if len(updates) > 0 {
			args = append(args, id)
			database.Exec(fmt.Sprintf("UPDATE users SET %s WHERE id=?", strings.Join(updates, ",")), args...)
		}
		// 多角色 user_roles 重写
		if body.Role != "" || len(body.Roles) > 0 {
			database.Exec(`DELETE FROM user_roles WHERE user_id=?`, id)
			primary := body.Role
			if primary == "" && len(body.Roles) > 0 {
				primary = body.Roles[0]
			}
			if primary != "" {
				database.Exec(`INSERT INTO user_roles (user_id,role_id) VALUES (?,?)`, id, primary)
			}
			for _, rid := range body.Roles {
				if rid != primary {
					database.Exec(`INSERT INTO user_roles (user_id,role_id) VALUES (?,?)`, id, rid)
				}
			}
		}
		// status=停用 → 该用户所有 sessions 立即失效
		if body.Status == "停用" {
			database.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
		}
		// status=启用 → 同时清除登录失败锁定，恢复后可立即登录
		if body.Status == "启用" {
			loginGuardClearUsername(curUsername)
		}
		writeAudit(database, u, c, "修改用户："+curUsername)
		util.OK(c, gin.H{"ok": true, "id": id})
	}
}

// ---- DELETE /api/admin/users/:id ----
func AdminDeleteUsers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		idStr := c.Param("id")
		var id int64
		fmt.Sscanf(idStr, "%d", &id)
		if id == 0 {
			util.BadRequest(c, "无效用户 ID")
			return
		}
		if id == u.ID {
			util.Forbidden(c, "不能删除自己")
			return
		}
		// 越权：能删这个角色的用户吗？
		if ok, badRole := canModifyUserRole(database, u, id, []string{}); !ok {
			util.Forbidden(c, "无权限删除角色 "+badRole+" 的用户")
			return
		}
		// 不能删除最后一个 platformadmin
		var cnt int
		database.QueryRow(`SELECT COUNT(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE ur.role_id='platformadmin'`).Scan(&cnt)
		if cnt <= 1 {
			util.Forbidden(c, "至少保留 1 个平台管理员")
			return
		}
		var uname string
		_ = database.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&uname)
		database.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
		database.Exec(`DELETE FROM user_roles WHERE user_id=?`, id)
		database.Exec(`DELETE FROM users WHERE id=?`, id)
		writeAudit(database, u, c, "删除用户："+uname)
		util.OK(c, gin.H{"ok": true})
	}
}

// ---- POST /api/admin/users/:id/reset-password ----
func AdminResetPassword(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		var id int64
		fmt.Sscanf(idStr, "%d", &id)
		if id == 0 {
			util.BadRequest(c, "无效用户 ID")
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		c.ShouldBindJSON(&body)
		// 重置口令：忽略客户端提交值，一律随机生成并只返回一次
		pwd, err := auth.GenerateTempPassword()
		if err != nil {
			util.Fail(c, 500, "生成初始口令失败")
			return
		}
		salt, hash, err := auth.HashPassword(pwd)
		if err != nil {
			util.Fail(c, 500, "密码散列失败")
			return
		}
		res, err := database.Exec(`UPDATE users SET pwd_salt=?, pwd_hash=?, must_change=1 WHERE id=?`, salt, hash, id)
		if err != nil {
			util.Fail(c, 500, "重置密码失败")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			util.Fail(c, 404, "用户不存在")
			return
		}
		// 重置后 sessions 失效
		database.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
		var uname string
		_ = database.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&uname)
		// 重置口令同时解除该账号在所有 IP 上的连续失败锁定，避免管理员重置后仍被拦在门外
		loginGuardClearUsername(uname)
		writeAudit(database, auth.CurrentUser(c), c, "重置用户口令："+uname)
		util.OK(c, gin.H{"ok": true, "mustChange": true, "initialPassword": pwd})
	}
}

// ---------- roles 写操作 ----------

// POST /api/admin/roles —— 新增自定义角色（platformadmin only）
func AdminPostRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u.Role.ID != auth.RolePlatformAdmin {
			util.Forbidden(c, "仅平台管理员可新增角色")
			return
		}
		var body struct {
			Name  string   `json:"name"`
			Nav   []string `json:"nav"`
			Color string   `json:"color"`
			Short string   `json:"short"`
			Descr string   `json:"descr"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
			util.BadRequest(c, "参数错误，name 必填")
			return
		}
		// nav 校验
		navOK := validNav(body.Nav)
		if !navOK {
			util.BadRequest(c, "nav 非法，应为合法模块 ID 数组")
			return
		}
		// 生成 role_ + 12 hex 随机 ID
		buf := make([]byte, 6)
		rand.Read(buf)
		newID := "role_" + hex.EncodeToString(buf)
		// seq = MAX(seq) + 1
		var maxSeq int
		database.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM roles`).Scan(&maxSeq)
		newSeq := maxSeq + 1
		navJSON, _ := json.Marshal(body.Nav)
		// roles 表完整 10 列: id,name,short,icon,color,descr,nav,external,status,seq
		// 注意: external 是 MySQL 保留字，必须反引号
		_, err := database.Exec(
			"INSERT INTO roles (id,name,short,icon,color,descr,nav,`external`,status,seq) VALUES (?,?,?,?,?,?,?,?,?,?)",
			newID, body.Name, body.Short, "", body.Color, body.Descr, string(navJSON), 0, "启用", newSeq)
		if err != nil {
			util.Fail(c, 500, "新增角色失败: "+err.Error())
			return
		}
		writeAudit(database, u, c, "新建角色："+body.Name)
		util.OK(c, gin.H{"ok": true, "id": newID})
	}
}

// PUT /api/admin/roles/:id —— 编辑角色
func AdminPutRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u.Role.ID != auth.RolePlatformAdmin {
			util.Forbidden(c, "仅平台管理员可编辑角色")
			return
		}
		// 不可编辑系统角色的 external / status
		if isSystemRole(id) {
			// 但允许改 name/descr/nav/color/short —— 和 Node admin.js 一致
		}
		var body struct {
			Name   string   `json:"name"`
			Nav    []string `json:"nav"`
			Color  string   `json:"color"`
			Short  string   `json:"short"`
			Descr  string   `json:"descr"`
			Status string   `json:"status"`
		}
		c.ShouldBindJSON(&body)
		updates := []string{}
		args := []any{}
		if body.Name != "" {
			updates = append(updates, "name=?")
			args = append(args, body.Name)
		}
		if body.Short != "" {
			updates = append(updates, "short=?")
			args = append(args, body.Short)
		}
		if body.Color != "" {
			updates = append(updates, "color=?")
			args = append(args, body.Color)
		}
		if body.Descr != "" {
			updates = append(updates, "descr=?")
			args = append(args, body.Descr)
		}
		if body.Nav != nil && validNav(body.Nav) {
			navJSON, _ := json.Marshal(body.Nav)
			updates = append(updates, "nav=?")
			args = append(args, string(navJSON))
		}
		args = append(args, id)
		if len(updates) > 0 {
			sql := "UPDATE roles SET " + strings.Join(updates, ",") + " WHERE id=?"
			_, err := database.Exec(sql, args...)
			if err != nil {
				util.Fail(c, 500, "更新角色失败")
				return
			}
		}
		writeAudit(database, u, c, "编辑角色："+id)
		util.OK(c, gin.H{"ok": true})
	}
}

// PATCH /api/admin/roles/:id/status —— 停用/启用角色
// 停用立即 DELETE sessions WHERE role_id=?
func AdminPatchRolesStatus(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u.Role.ID != auth.RolePlatformAdmin {
			util.Forbidden(c, "仅平台管理员可停用角色")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		c.ShouldBindJSON(&body)
		if body.Status != "启用" && body.Status != "停用" {
			util.BadRequest(c, "status 应为 启用/停用")
			return
		}
		// 不能停用 platformadmin
		if id == auth.RolePlatformAdmin {
			util.Forbidden(c, "不能停用平台管理员角色")
			return
		}
		database.Exec(`UPDATE roles SET status=? WHERE id=?`, body.Status, id)
		if body.Status == "停用" {
			// 该 role 绑定的所有会话立即失效（真实 sessions 表）
			database.Exec(`DELETE FROM sessions WHERE role_id=?`, id)
		}
		writeAudit(database, u, c, "角色状态变更："+id+"→"+body.Status)
		util.OK(c, gin.H{"ok": true})
	}
}

// DELETE /api/admin/roles/:id —— 删除角色
func AdminDeleteRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u.Role.ID != auth.RolePlatformAdmin {
			util.Forbidden(c, "仅平台管理员可删除角色")
			return
		}
		// 系统角色不可删
		if isSystemRole(id) {
			util.Forbidden(c, "系统内置角色不可删除")
			return
		}
		// 有用户绑定的角色不可删
		var cnt int
		database.QueryRow(`SELECT COUNT(*) FROM user_roles WHERE role_id=?`, id).Scan(&cnt)
		if cnt > 0 {
			util.Forbidden(c, fmt.Sprintf("该角色已绑定 %d 个用户，先移除绑定", cnt))
			return
		}
		database.Exec(`DELETE FROM roles WHERE id=?`, id)
		writeAudit(database, u, c, "删除角色："+id)
		util.OK(c, gin.H{"ok": true})
	}
}

// PATCH /api/admin/roles/order —— 调整 seq 顺序
// body: { "order": ["role_a","role_b","role_c"...] }
func AdminPatchRolesOrder(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u.Role.ID != auth.RolePlatformAdmin {
			util.Forbidden(c, "仅平台管理员可调整顺序")
			return
		}
		var body struct {
			Order []string `json:"order"`
		}
		c.ShouldBindJSON(&body)
		for i, rid := range body.Order {
			database.Exec(`UPDATE roles SET seq=? WHERE id=?`, i, rid)
		}
		util.OK(c, gin.H{"ok": true})
	}
}

// ---------- forms 写操作 ----------

// POST /api/admin/forms —— 新增表单模板，id = F{MAX+1}
func AdminPostForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Name   string          `json:"name"`
			Fields json.RawMessage `json:"fields"` // 对象数组（也兼容字符串数组 / JSON 字符串）
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "参数错误")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			util.BadRequest(c, "模板名称不能为空")
			return
		}
		name = clipStr(name, 40)
		fields, err := parseFormFields(body.Fields)
		if err != nil {
			util.BadRequest(c, "字段格式不正确")
			return
		}
		if len(fields) == 0 {
			util.BadRequest(c, "字段不能为空")
			return
		}
		if dup := dupFieldLabel(fields); dup != "" {
			util.BadRequest(c, "字段名称重复："+dup)
			return
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			util.Fail(c, 500, "字段序列化失败")
			return
		}
		// id 与展示编号 code 都按当前最大数字 +1 生成（id=F18、code=F018）。
		// 并发新建时两个请求可能算出同一个编号，靠主键/唯一键冲突检测后重算重试。
		var newID, newCode string
		var insertErr error
		for attempt := 0; attempt < 5; attempt++ {
			var maxNum int
			if err := database.QueryRow(
				`SELECT COALESCE(MAX(CAST(SUBSTRING(id,2) AS UNSIGNED)),0) FROM form_templates`,
			).Scan(&maxNum); err != nil {
				slog.Warn("AdminPostForms 读取最大编号失败", "err", err)
				util.Fail(c, 500, "读取模板编号失败")
				return
			}
			newID = fmt.Sprintf("F%d", maxNum+1)
			newCode = fmt.Sprintf("F%03d", maxNum+1)
			// 不显式写 mode：老库可能还没有该列，交由列默认值 fill 兜底
			_, insertErr = database.Exec(
				`INSERT INTO form_templates (id,name,fields,code) VALUES (?,?,?,?)`,
				newID, name, string(raw), newCode)
			if insertErr == nil || !isDuplicateKeyErr(insertErr) {
				break
			}
		}
		if insertErr != nil {
			if isDuplicateKeyErr(insertErr) {
				slog.Warn("AdminPostForms 编号冲突", "id", newID, "code", newCode, "err", insertErr)
				util.Fail(c, 409, "模板编号生成冲突，请重试")
				return
			}
			slog.Warn("AdminPostForms 写入失败", "id", newID, "err", insertErr)
			util.Fail(c, 500, "新增表单模板失败")
			return
		}
		util.OK(c, gin.H{"ok": true, "id": newID, "code": newCode, "name": name, "fields": fields})
	}
}

// PUT /api/admin/forms/:id —— 编辑表单模板
// 语义：name / fields 都是可选字段，只更新请求里出现的部分；
// 返回体与 Node 版一致（{ok,template:{id,name,fields}}），前端依赖它回填列表。
func AdminPutForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Name   *string         `json:"name"`
			Fields json.RawMessage `json:"fields"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "参数错误")
			return
		}

		var oldName, oldFields, oldCode sql.NullString
		if err := database.QueryRow(
			`SELECT name,fields,COALESCE(code,'') FROM form_templates WHERE id=?`, id,
		).Scan(&oldName, &oldFields, &oldCode); err != nil {
			if err == sql.ErrNoRows {
				util.Fail(c, 404, "表单模板不存在")
				return
			}
			slog.Warn("AdminPutForms 查询失败", "id", id, "err", err)
			util.Fail(c, 500, "读取表单模板失败")
			return
		}

		name := nullStr(oldName)
		if body.Name != nil {
			name = strings.TrimSpace(*body.Name)
			if name == "" {
				util.BadRequest(c, "模板名称不能为空")
				return
			}
			name = clipStr(name, 40)
		}

		fields := util.ParseJSONArrAny(nullStr(oldFields))
		if len(body.Fields) > 0 && strings.TrimSpace(string(body.Fields)) != "null" {
			nf, err := parseFormFields(body.Fields)
			if err != nil {
				util.BadRequest(c, "字段格式不正确")
				return
			}
			if len(nf) == 0 {
				util.BadRequest(c, "字段不能为空")
				return
			}
			if dup := dupFieldLabel(nf); dup != "" {
				util.BadRequest(c, "字段名称重复："+dup)
				return
			}
			fields = nf
		}

		raw, err := json.Marshal(fields)
		if err != nil {
			util.Fail(c, 500, "字段序列化失败")
			return
		}
		if _, err := database.Exec(
			`UPDATE form_templates SET name=?, fields=? WHERE id=?`, name, string(raw), id,
		); err != nil {
			slog.Warn("AdminPutForms 更新失败", "id", id, "err", err)
			util.Fail(c, 500, "保存表单模板失败")
			return
		}
		util.OK(c, gin.H{"ok": true, "template": gin.H{
			"id": id, "code": formCodeOf(id, nullStr(oldCode)), "name": name, "fields": fields,
		}})
	}
}

// parseFormFields 把请求里的 fields 解析成规范化对象数组。
// 兼容三种历史载荷：对象数组（当前前端）、字符串数组（旧简化格式）、JSON 字符串（老接口嵌套）。
func parseFormFields(raw json.RawMessage) ([]any, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return []any{}, nil
	}
	// 兼容「字符串里再套一层 JSON」
	if strings.HasPrefix(s, "\"") {
		var inner string
		if err := json.Unmarshal(raw, &inner); err != nil {
			return nil, err
		}
		s = strings.TrimSpace(inner)
		if s == "" || s == "null" {
			return []any{}, nil
		}
	}
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, err
	}
	return normalizeFields(arr), nil
}

// isDuplicateKeyErr 判断是否为 MySQL 主键/唯一键冲突（1062）。
// 新建模板时 id 与 code 都由「当前最大编号+1」推导，并发下必然出现冲突，需靠它重试。
func isDuplicateKeyErr(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// dupFieldLabel 返回第一个重复的字段名，无重复返回空串
func dupFieldLabel(fields []any) string {
	seen := map[string]bool{}
	for _, f := range fields {
		m, ok := f.(map[string]any)
		if !ok {
			continue
		}
		label, _ := m["label"].(string)
		if label == "" {
			continue
		}
		if seen[label] {
			return label
		}
		seen[label] = true
	}
	return ""
}

// DELETE /api/admin/forms/:id —— 删除表单模板
func AdminDeleteForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		res, err := database.Exec(`DELETE FROM form_templates WHERE id=?`, id)
		if err != nil {
			util.Fail(c, 500, "删除失败")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			util.Fail(c, 404, "表单模板不存在")
			return
		}
		util.OK(c, gin.H{"ok": true})
	}
}

// ---------- helper ----------

// validNav: nav 必须是预定义模块列表的子集
var moduleKeys = map[string]bool{
	"dashboard": true, "workbench": true, "collab": true, "scenarios": true,
	"projects": true, "screen": true, "evidence": true, "replay": true,
	"evaluation": true, "report": true, "ledger": true, "opinion": true,
	"knowledge": true, "admin": true,
}

func validNav(nav []string) bool {
	if len(nav) == 0 {
		return false
	}
	for _, k := range nav {
		if !moduleKeys[k] {
			return false
		}
	}
	return true
}

// isSystemRole: 6 个内置角色 + 3 外部 + 4 临床 = 13 个不可删不可改 external/status
func isSystemRole(id string) bool {
	switch id {
	case auth.RolePlatformAdmin, auth.RoleCommander, auth.RoleITLead,
		auth.RoleDuty, auth.RoleNetSec, auth.RoleSysAdmin,
		auth.RoleMedLead, auth.RoleClinical,
		auth.RoleHealthBureau, auth.RoleCAC, auth.RoleCyberPolice:
		return true
	}
	return false
}
