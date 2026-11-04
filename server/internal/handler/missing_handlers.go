// Package handler —— 补齐 Go 后端缺失的路由 handler
//
// 本文件承载从旧 Node 版迁移但 Go router.go 未注册的接口实现。
// 全部 handler 风格与既有 handler 保持一致：返回 gin.H 或数组，不用
// 包装层 {ok,data}（Node 版就是直接返回对象）。

package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// =========================================================================
// 从 bootstrap.go 导出但需要暴露为独立 HTTP handler 的 GET 接口
// =========================================================================

// GetContentRoles GET /api/content/roles —— 公开角色列表
func GetContentRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryRoles(database)) }
}

// GetContentUsers GET /api/content/users —— 用户列表
func GetContentUsers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryUsers(database, auth.CurrentUser(c))) }
}

// GetContentAudit GET /api/content/audit —— 审计日志
func GetContentAudit(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryAuditLogs(database, auth.CurrentUser(c))) }
}

// GetDashboard GET /api/content/dashboard —— 概览
func GetDashboard(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var totalProjects, activeProjects, totalTasks, doneTasks int
		_ = database.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&totalProjects)
		_ = database.QueryRow(`SELECT COUNT(*) FROM projects WHERE status IN ('演练中','暂停')`).Scan(&activeProjects)
		_ = database.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&totalTasks)
		_ = database.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='已完成'`).Scan(&doneTasks)
		c.JSON(http.StatusOK, gin.H{
			"totalProjects":  totalProjects,
			"activeProjects": activeProjects,
			"totalTasks":     totalTasks,
			"doneTasks":      doneTasks,
		})
	}
}

// =========================================================================
// content 组 —— 写操作补齐
// =========================================================================

// PostMessages POST /api/content/messages —— 发布站内消息
func PostMessages(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		rid := u.Role.ID
		if rid != "commander" && rid != "itlead" && rid != "platformadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅指挥部/信息科负责人/平台管理员可发布指令类消息"})
			return
		}
		var body struct {
			Level   string   `json:"level"`
			Source  string   `json:"source"`
			Text    string   `json:"text"`
			Targets []string `json:"targets"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Level == "" {
			body.Level = "P2"
		}
		if body.Source == "" {
			body.Source = u.Name + "（人工触发）"
		}
		if body.Targets == nil {
			body.Targets = []string{"*"}
		}
		seq := 0
		_ = database.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM messages`).Scan(&seq)
		id := fmt.Sprintf("M%d", time.Now().UnixMilli())
		targetsJSON := util.ToJSON(body.Targets)
		var activeID sql.NullString
		_ = database.QueryRow(`SELECT value FROM settings WHERE key='active_project'`).Scan(&activeID)
		pid := activeID.String
		_, _ = database.Exec(
			`INSERT INTO messages (id,time,level,source,text,targets,seq,project_id) VALUES (?,?,?,?,?,?,?,?)`,
			id, util.HHMM(), body.Level, body.Source, body.Text, targetsJSON, seq, pid,
		)
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
	}
}

// PostNotifsRead POST /api/content/notifs/read —— 标记通知已读
func PostNotifsRead(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = c.ShouldBindJSON(&body)
		if len(body.IDs) == 0 {
			rows, _ := database.Query(`SELECT id FROM notifs`)
			defer rows.Close()
			var allID string
			for rows.Next() {
				if rows.Scan(&allID) == nil {
					body.IDs = append(body.IDs, allID)
				}
			}
		}
		stmt, _ := database.Prepare(`INSERT IGNORE INTO notif_reads (user_id,notif_id) VALUES (?,?)`)
		defer stmt.Close()
		for _, id := range body.IDs {
			_, _ = stmt.Exec(u.ID, id)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostSnapshots POST /api/content/snapshots —— 创建快照
func PostSnapshots(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var body struct {
			Name string `json:"name"`
			Note string `json:"note"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Name == "" {
			body.Name = "手动快照"
		}
		res, _ := database.Exec(
			`INSERT INTO snapshots (name,time,by_user,note) VALUES (?,?,?,?)`,
			body.Name, util.NowSec(), u.Name, body.Note,
		)
		var id int64
		if res != nil {
			id, _ = res.LastInsertId()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
	}
}

// PostSeqFlow POST /api/content/seqflow —— 保存时序流程
func PostSeqFlow(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		var body struct {
			Items []struct {
				ProjectID string `json:"projectId"`
				Name      string `json:"name"`
				Time      string `json:"time"`
				Role      string `json:"role"`
				State     string `json:"state"`
			} `json:"items"`
		}
		_ = c.ShouldBindJSON(&body)
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "事务失败"})
			return
		}
		defer tx.Rollback()
		for _, it := range body.Items {
			seq := 0
			_ = tx.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM seq_flow WHERE project_id=?`, it.ProjectID).Scan(&seq)
			_, _ = tx.Exec(
				`INSERT INTO seq_flow (project_id,name,time,role,state,seq) VALUES (?,?,?,?,?,?)`,
				it.ProjectID, it.Name, it.Time, it.Role, it.State, seq,
			)
		}
		_ = tx.Commit()
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PatchImpact PATCH /api/content/impact/:sys —— 更新受影响系统状态
func PatchImpact(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		sys := c.Param("sys")
		var body struct {
			State string  `json:"state"`
			Pct   float64 `json:"pct"`
		}
		_ = c.ShouldBindJSON(&body)
		activeID := activeProjectID(database)
		if body.State != "" {
			_, _ = database.Exec(`UPDATE impact SET state=? WHERE sys=? AND project_id=?`, body.State, sys, activeID)
		}
		if body.Pct != 0 {
			_, _ = database.Exec(`UPDATE impact SET pct=? WHERE sys=? AND project_id=?`, body.Pct, sys, activeID)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostAudit POST /api/content/audit —— 写入审计日志
func PostAudit(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Action string `json:"action"`
		}
		_ = c.ShouldBindJSON(&body)
		if u != nil {
			_, _ = database.Exec(
				`INSERT INTO audit_logs (time,user,role_id,action,ip) VALUES (?,?,?,?,?)`,
				util.Now(), u.Name, u.Role.ID, body.Action, c.ClientIP(),
			)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostOpinion POST /api/content/opinion —— 新增舆情
func PostOpinion(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Channel string `json:"channel"`
			Title   string `json:"title"`
			Heat    int    `json:"heat"`
			Risk    string `json:"risk"`
			Source  string `json:"source"`
		}
		_ = c.ShouldBindJSON(&body)
		res, _ := database.Exec(
			`INSERT INTO opinion (time,channel,title,heat,risk,status,source) VALUES (?,?,?,?,?,?,?)`,
			util.NowSec(), body.Channel, body.Title, body.Heat, body.Risk, "新发布", body.Source,
		)
		var id int64
		if res != nil {
			id, _ = res.LastInsertId()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
	}
}

// PatchOpinion PATCH /api/content/opinion/:id —— 更新舆情状态
func PatchOpinion(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Heat   *int   `json:"heat"`
			Risk   string `json:"risk"`
			Status string `json:"status"`
		}
		_ = c.ShouldBindJSON(&body)
		fields := []string{}
		args := []any{}
		if body.Heat != nil {
			fields = append(fields, "heat=?")
			args = append(args, *body.Heat)
		}
		if body.Risk != "" {
			fields = append(fields, "risk=?")
			args = append(args, body.Risk)
		}
		if body.Status != "" {
			fields = append(fields, "status=?")
			args = append(args, body.Status)
		}
		if len(fields) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无更新字段"})
			return
		}
		args = append(args, id)
		_, _ = database.Exec(fmt.Sprintf("UPDATE opinion SET %s WHERE id=?", strings.Join(fields, ",")))
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DeleteProject DELETE /api/content/projects/:id —— 删除项目
// 行为对齐 Node src/routes/content.js L1278-1327：仅平台管理员可删（403）、
// 项目不存在 404、同事务清理全部关联数据（含 notifs / notif_reads），
// impact 为全局影响面快照不删除；事务提交后按 Node 规则 unlink uploads/ 下的物理附件。
func DeleteProject(database *db.DB, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.ID != "platformadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅平台管理员可删除演练项目"})
			return
		}
		var exists int
		if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE id=?`, id).Scan(&exists); err != nil || exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}

		var taskCount int
		_ = database.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id=?`, id).Scan(&taskCount)

		// 汇总待物理删除的上传文件（Node content.js:1284-1289：任务附件 + 取证材料）
		fileSet := map[string]bool{}
		filePaths := make([]string, 0)
		addFiles := func(rows *sql.Rows, err error) {
			if err != nil || rows == nil {
				return
			}
			defer rows.Close()
			for rows.Next() {
				var fp sql.NullString
				if rows.Scan(&fp) != nil {
					continue
				}
				p := strings.TrimSpace(util.ScanNullableString(fp))
				if p != "" && !fileSet[p] {
					fileSet[p] = true
					filePaths = append(filePaths, p)
				}
			}
		}
		addFiles(database.Query(`SELECT file_path FROM task_attachments WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)`, id))
		addFiles(database.Query(`SELECT file_path FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?)`, id, id))

		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败"})
			return
		}
		defer tx.Rollback()

		// 附件与表单提交没有 project_id 列，必须经 tasks 关联（Node 同口径）
		_, _ = tx.Exec(`DELETE FROM task_attachments WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)`, id)
		_, _ = tx.Exec(`DELETE FROM form_submissions WHERE task_id IN (SELECT id FROM tasks WHERE project_id=?)`, id)

		// 取证材料：按 project_id 或关联任务两种归属方式清理（Node 同口径）
		_, _ = tx.Exec(`DELETE FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?)`, id, id)

		// impact 为全局影响面快照，Node 显式保留（大屏在用），不在此列表
		tables := []string{
			"tasks", "timeline", "snapshots", "drill_steps", "drill_node_state",
			"scores", "seq_flow", "ledger", "opinion", "messages", "chat_messages",
		}
		for _, t := range tables {
			_, _ = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE project_id=?", t), id)
		}

		// 站内消息：先按 notif_id 清理已读状态，再按 project_id 删除消息本身
		_, _ = tx.Exec(`DELETE FROM notif_reads WHERE notif_id IN (SELECT id FROM notifs WHERE project_id=?)`, id)
		res, notifErr := tx.Exec(`DELETE FROM notifs WHERE project_id=?`, id)
		if notifErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "站内消息清理失败"})
			return
		}
		deletedNotifs, _ := res.RowsAffected()

		_, _ = tx.Exec("DELETE FROM projects WHERE id=?", id)
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
			return
		}

		// 物理文件：仅删除 uploads/ 目录内的路径（拒绝绝对路径与目录逃逸），不做通配
		uploadAbs, absErr := filepath.Abs(uploadDir)
		filesRemoved, filesMissing := 0, 0
		skippedFiles := make([]string, 0)
		for _, rel := range filePaths {
			rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
			if rel == "" || filepath.IsAbs(rel) {
				skippedFiles = append(skippedFiles, rel)
				continue
			}
			full := filepath.Clean(filepath.Join(uploadAbs, filepath.FromSlash(rel)))
			if absErr != nil {
				skippedFiles = append(skippedFiles, rel)
				continue
			}
			back, rerr := filepath.Rel(uploadAbs, full)
			if rerr != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
				skippedFiles = append(skippedFiles, rel)
				continue
			}
			if err := os.Remove(full); err != nil {
				if os.IsNotExist(err) {
					filesMissing++
				} else {
					skippedFiles = append(skippedFiles, rel)
				}
				continue
			}
			filesRemoved++
		}

		c.JSON(http.StatusOK, gin.H{
			"ok": true, "id": id, "tasks": taskCount,
			"files":          len(filePaths),
			"files_removed":  filesRemoved,
			"files_missing":  filesMissing,
			"files_skipped":  skippedFiles,
			"deleted_notifs": deletedNotifs,
		})
	}
}

// PatchProjectMembers PATCH /api/content/projects/:id/members —— 更新项目成员
func PatchProjectMembers(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Members []any `json:"members"`
		}
		_ = c.ShouldBindJSON(&body)
		membersJSON := util.ToJSON(body.Members)
		_, _ = database.Exec("UPDATE projects SET members=? WHERE id=?", membersJSON, id)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// GetProjectScriptExport GET /api/content/projects/:id/script/export —— 脚本导出
func GetProjectScriptExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		fmtQ := c.DefaultQuery("fmt", "word")
		var name string
		_ = database.QueryRow(`SELECT name FROM projects WHERE id=?`, id).Scan(&name)
		body := fmt.Sprintf(`<html><head><meta charset='utf-8'><title>%s 演练脚本</title></head><body><h1>%s</h1><p>项目ID：%s</p></body></html>`, name, name, id)
		ctype := "application/msword"
		if fmtQ == "html" {
			ctype = "text/html; charset=utf-8"
		}
		c.Header("Content-Type", ctype)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="script_%s.doc"`, id))
		c.String(http.StatusOK, body)
	}
}

// PostProjectNotify POST /api/content/projects/:id/notify —— 手动生成演练通知
func PostProjectNotify(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		rid := u.Role.ID
		if rid != "platformadmin" && rid != "commander" && rid != "itlead" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可生成演练通知"})
			return
		}
		var name, scenarioID, mode, level, date, tgt string
		if err := database.QueryRow(
			`SELECT COALESCE(name,''),COALESCE(scenario_id,''),COALESCE(mode,'基础版'),COALESCE(level,'一般'),COALESCE(date,''),COALESCE(target,'') FROM projects WHERE id=?`, id,
		).Scan(&name, &scenarioID, &mode, &level, &date, &tgt); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		scenarioName := "未关联剧本"
		if scenarioID != "" {
			var sname string
			if database.QueryRow(`SELECT COALESCE(name,'') FROM scenarios WHERE id=?`, scenarioID).Scan(&sname) == nil && sname != "" {
				scenarioName = sname
			}
		}
		if tgt == "" {
			tgt = "按剧本预案完成全流程处置"
		}
		nLevel := "P1"
		if level == "重大" {
			nLevel = "P0"
		}
		// 站内消息：演练通知（Node content.js:988-996；手动点击每次都发，跳过判重）
		pushNotifForce(database, "project_created", map[string]any{
			"project": name, "projectId": id, "mode": mode, "scenario": scenarioName,
			"level": level, "date": date, "target": tgt,
		}, id, nLevel, "", "internal")
		_, _ = database.Exec(
			`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, util.HHMM(), u.Name, u.Role.ID, "生成演练通知", "演练通知已推送至所有参演角色的站内消息", "", nil,
		)
		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "msg": "演练通知已推送至站内消息"})
	}
}

// GetProjectNotifyExport GET /api/content/projects/:id/notify/export
func GetProjectNotifyExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		fmtQ := c.DefaultQuery("fmt", "word")
		body := fmt.Sprintf(`<html><head><meta charset='utf-8'></head><body><h1>演练通知</h1><p>项目ID：%s</p></body></html>`, id)
		ctype := "application/msword"
		if fmtQ == "html" {
			ctype = "text/html; charset=utf-8"
		}
		c.Header("Content-Type", ctype)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="notify_%s.doc"`, id))
		c.String(http.StatusOK, body)
	}
}

// GetDrillProgress GET /api/content/drill-progress
func GetDrillProgress(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.DefaultQuery("channel", "inner")
		var activeNull sql.NullString
		_ = database.QueryRow(`SELECT value FROM settings WHERE key='active_project'`).Scan(&activeNull)
		activeID := activeNull.String
		if activeID == "" {
			c.JSON(http.StatusOK, gin.H{"project": nil, "mode": "", "nodes": []gin.H{}, "channel": channel})
			return
		}
		var pName, pMode, pStatus, pDate, scenarioID string
		_ = database.QueryRow(`SELECT name,COALESCE(mode,'基础版'),status,COALESCE(date,''),COALESCE(scenario_id,'') FROM projects WHERE id=?`, activeID).Scan(&pName, &pMode, &pStatus, &pDate, &scenarioID)
		c.JSON(http.StatusOK, gin.H{
			"project":   gin.H{"id": activeID, "name": pName, "mode": pMode, "status": pStatus, "date": pDate, "scenarioId": scenarioID},
			"mode":      pMode,
			"nodes":     []gin.H{},
			"updatedAt": util.Now(),
			"channel":   channel,
		})
	}
}

// =========================================================================
// Chat 全套（简化实现）
// =========================================================================

// GetChatChannels GET /api/content/chat/channels —— 对齐 Node content.js:134
// 读 dict.chat_channels，按当前角色过滤（roles 含本角色才可见），带 count/last
func GetChatChannels(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		metas, err := chatChannelMetas(database)
		if err != nil || len(metas) == 0 {
			c.JSON(http.StatusOK, []gin.H{})
			return
		}
		out := []gin.H{}
		for _, meta := range metas {
			key, label := meta.Key, meta.Label
			allowed := false
			for _, r := range meta.Roles {
				if r == u.Role.ID {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
			var cnt int
			_ = database.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE grp=?`, key).Scan(&cnt)
			var last any
			var text, t, sender string
			if err := database.QueryRow(
				`SELECT COALESCE(text,''),COALESCE(time,''),COALESCE(sender,'') FROM chat_messages WHERE grp=? ORDER BY id DESC LIMIT 1`,
				key,
			).Scan(&text, &t, &sender); err == nil {
				last = gin.H{"text": text, "time": fullTime16(t), "sender": sender}
			}
			out = append(out, gin.H{"key": key, "label": label, "count": cnt, "last": last})
		}
		c.JSON(http.StatusOK, out)
	}
}

// fullTime16 对齐 Node content.js fullTime：已是 yyyy-mm-dd 开头取前 16 位，否则补当天日期
func fullTime16(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if len(t) >= 10 && t[4] == '-' && t[7] == '-' {
		if len(t) > 16 {
			return t[:16]
		}
		return t
	}
	if len(t) > 5 {
		t = t[:5]
	}
	return time.Now().Format("2006-01-02") + " " + t
}

// chatChannelMeta —— dict.chat_channels 中的单个频道定义（对齐 Node content.js:102-118）
type chatChannelMeta struct {
	Key   string
	Label string
	Roles []string
}

// chatChannelMetas 读取并解析 dict.chat_channels，保持字典中的键顺序（Node 用 Object.keys 顺序）
func chatChannelMetas(database *db.DB) ([]chatChannelMeta, error) {
	var raw sql.NullString
	if err := database.QueryRow(`SELECT value FROM dict WHERE ` + "`key`" + `='chat_channels'`).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	s := strings.TrimSpace(util.ScanNullableString(raw))
	if s == "" {
		return nil, nil
	}
	// 兼容历史数据的外层 {"value":{...}} 包裹
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(s), &top) == nil {
		if inner, ok := top["value"]; ok && len(top) == 1 {
			s = string(inner)
		}
	}
	dec := json.NewDecoder(strings.NewReader(s))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	out := []chatChannelMeta{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := kt.(string)
		var meta struct {
			Label string   `json:"label"`
			Roles []string `json:"roles"`
		}
		if err := dec.Decode(&meta); err != nil {
			break
		}
		label := meta.Label
		if label == "" {
			label = key
		}
		out = append(out, chatChannelMeta{Key: key, Label: label, Roles: meta.Roles})
	}
	return out, nil
}

// chatChannelOf 对齐 Node channelOf（content.js:102-118）：
// 频道不存在 → 400「会商频道不存在」；当前角色不在频道 roles → 403「无权访问」；通过时返回频道元数据。
func chatChannelOf(database *db.DB, u *auth.User, group string, c *gin.Context) (chatChannelMeta, bool) {
	metas, err := chatChannelMetas(database)
	if err != nil || len(metas) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "会商频道不存在"})
		return chatChannelMeta{}, false
	}
	for _, m := range metas {
		if m.Key != group {
			continue
		}
		for _, r := range m.Roles {
			if r == u.Role.ID {
				return m, true
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问「" + m.Label + "」频道"})
		return m, false
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "会商频道不存在"})
	return chatChannelMeta{}, false
}

// GetChat GET /api/content/chat/:group
func GetChat(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		group := c.Param("group")
		ch, ok := chatChannelOf(database, u, group, c)
		if !ok {
			return
		}
		rows, err := database.Query(`SELECT id,sender,role_id,COALESCE(user_id,0),time,COALESCE(text,''),COALESCE(att,''),COALESCE(file_path,'') FROM chat_messages WHERE grp=? ORDER BY id`, ch.Key)
		if err != nil {
			c.JSON(http.StatusOK, []gin.H{})
			return
		}
		defer rows.Close()
		out := []gin.H{}
		for rows.Next() {
			var id int64
			var sender, role, time2, text, att, filePath string
			var userID int64
			if rows.Scan(&id, &sender, &role, &userID, &time2, &text, &att, &filePath) != nil {
				continue
			}
			url := ""
			if filePath != "" {
				url = "/uploads/" + filePath
			}
			out = append(out, gin.H{
				"id": id, "sender": sender, "role": role, "userId": userID,
				"time": time2, "text": text, "att": att, "url": url,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

// PostChat POST /api/content/chat/:group
func PostChat(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		group := c.Param("group")
		ch, ok := chatChannelOf(database, u, group, c)
		if !ok {
			return
		}
		var body struct {
			Text     string `json:"text"`
			Att      string `json:"att"`
			FilePath string `json:"filePath"`
		}
		_ = c.ShouldBindJSON(&body)
		res, err := database.Exec(
			`INSERT INTO chat_messages (grp,sender,role_id,user_id,time,text,att,file_path) VALUES (?,?,?,?,?,?,?,?)`,
			ch.Key, u.Name, u.Role.ID, u.ID, util.HHMM(), body.Text, body.Att, body.FilePath,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var id int64
		if res != nil {
			id, _ = res.LastInsertId()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
	}
}

// PostChatUpload POST /api/content/chat/:group/upload
func PostChatUpload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if _, ok := chatChannelOf(database, u, c.Param("group"), c); !ok {
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		ext := ""
		if idx := strings.LastIndex(file.Filename, "."); idx >= 0 {
			ext = file.Filename[idx:]
		}
		saved := fmt.Sprintf("chat/%d%s", time.Now().UnixMilli(), ext)
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "name": file.Filename, "size": file.Size,
			"url": "/uploads/" + saved, "path": saved,
		})
	}
}

// =========================================================================
// /api/projects/*  —— bootstrap 下的项目激活 & 状态切换
// =========================================================================

// PostProjectActivate POST /api/projects/:id/activate
func PostProjectActivate(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var exists int
		_ = database.QueryRow(`SELECT COUNT(*) FROM projects WHERE id=?`, id).Scan(&exists)
		if exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		_, _ = database.Exec(
			`INSERT INTO settings (key,value) VALUES ('active_project',?) ON DUPLICATE KEY UPDATE value=VALUES(value)`,
			id,
		)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PatchProjectStatus PATCH /api/projects/:id/status
func PatchProjectStatus(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Status string `json:"status"`
		}
		_ = c.ShouldBindJSON(&body)
		allow := map[string]bool{"未开始": true, "演练中": true, "暂停": true, "已结束": true, "已归档": true}
		if !allow[body.Status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的项目状态"})
			return
		}
		var prev, name, level, mode string
		var startedAt, pausedAt, endedAt sql.NullString
		var pausedSec sql.NullInt64
		if err := database.QueryRow(
			`SELECT COALESCE(status,''),COALESCE(name,''),COALESCE(level,''),COALESCE(mode,'基础版'),
			 started_at,paused_at,paused_sec,ended_at FROM projects WHERE id=?`, id,
		).Scan(&prev, &name, &level, &mode, &startedAt, &pausedAt, &pausedSec, &endedAt); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		/* 真实计时（逐行对齐 Node src/routes/bootstrap.js:306-329）：
		 *   演练中：首次启动写 started_at；从暂停恢复时把本次暂停时长累加进 paused_sec 并清空 paused_at
		 *   暂停：写 paused_at（累计值不动）
		 *   已结束/已归档：把未结算的暂停时长累加进 paused_sec，写 ended_at
		 *   未开始：四项全部清空
		 * 时间格式 YYYY-MM-DD HH:mm:ss（服务器本地时区，VARCHAR(19)），与 Node nowSec() 一致。 */
		ts := util.Now()
		started := util.ScanNullableString(startedAt)
		paused := util.ScanNullableString(pausedAt)
		ended := util.ScanNullableString(endedAt)
		ps := int64(0)
		if pausedSec.Valid {
			ps = pausedSec.Int64
		}
		switch body.Status {
		case "未开始":
			started, paused, ps, ended = "", "", 0, ""
		case "演练中":
			if started == "" {
				started = ts
			}
			if prev == "暂停" && paused != "" {
				ps += secBetweenLocal(paused, ts)
				paused = ""
			}
			ended = ""
		case "暂停":
			if prev != "暂停" {
				paused = ts
			}
		case "已结束", "已归档":
			if prev != body.Status {
				if paused != "" {
					ps += secBetweenLocal(paused, ts)
					paused = ""
				}
				ended = ts
			}
		}
		_, _ = database.Exec(`UPDATE projects SET status=?, started_at=?, paused_at=?, paused_sec=?, ended_at=? WHERE id=?`,
			body.Status, nullIfEmpty(started), nullIfEmpty(paused), ps, nullIfEmpty(ended), id)
		// 站内消息：演练启动/继续、演练结束（Node bootstrap.js:331-351）
		if body.Status == "演练中" && prev != "演练中" {
			resume := prev == "暂停"
			action, resumed := "演练开始", "已启动"
			if resume {
				action, resumed = "演练继续", "已恢复"
			}
			pLevel := level
			if pLevel == "" {
				pLevel = "较大"
			}
			nLevel := "P1"
			if level == "重大" {
				nLevel = "P0"
			}
			pushNotif(database, "drill_start", map[string]any{
				"action": action, "project": name, "resumed": resumed, "mode": mode, "level": pLevel,
			}, id, nLevel, "", "internal")
		} else if body.Status == "已结束" && prev != "已结束" {
			pushNotif(database, "drill_end", map[string]any{
				"project": name, "mode": mode,
			}, id, "", "", "internal")
		}
		/* 与 Node bootstrap.js:352 一致：回传最新项目对象（含 startedAt/pausedAt/pausedSec/endedAt），
		 * 前端 API.updateProjectStatus 用它刷新本地缓存，避免"开始演练"后计时仍停留在旧值 */
		c.JSON(http.StatusOK, gin.H{"ok": true, "project": projectJSONByID(database, id)})
	}
}

// secBetweenLocal 两个本地时间字符串之间的秒数（Node drill_clock.secBetween 对齐，负数按 0）
func secBetweenLocal(a, b string) int64 {
	x, y := util.ParseLocal(a), util.ParseLocal(b)
	if x > 0 && y > 0 {
		if d := int64((y - x) / 1000); d > 0 {
			return d
		}
	}
	return 0
}

// projectJSONByID 单个项目 → 前端对象（字段与 Node bootstrap.mapProject 对齐）
func projectJSONByID(database *db.DB, id string) gin.H {
	var pid, name, typ, date, scenarioID, level, status, target sql.NullString
	var unitsStr, membersStr, observersStr, mode, startedAt, pausedAt, endedAt sql.NullString
	var pausedSec sql.NullInt64
	if err := database.QueryRow(`SELECT id,name,type,date,scenario_id,level,status,target,
		units,members,observers,mode,started_at,paused_at,paused_sec,ended_at FROM projects WHERE id=?`, id).
		Scan(&pid, &name, &typ, &date, &scenarioID, &level, &status, &target,
			&unitsStr, &membersStr, &observersStr, &mode, &startedAt, &pausedAt, &pausedSec, &endedAt); err != nil {
		return nil
	}
	modeV := util.ScanNullableString(mode)
	if modeV == "" {
		modeV = "基础版"
	}
	ps := int64(0)
	if pausedSec.Valid {
		ps = pausedSec.Int64
	}
	statusV := util.ScanNullableString(status)
	idStr := util.ScanNullableString(pid)
	var startedVal, pausedVal, endedVal any
	if s := util.ScanNullableString(startedAt); s != "" {
		startedVal = s
	}
	if s := util.ScanNullableString(pausedAt); s != "" {
		pausedVal = s
	}
	if s := util.ScanNullableString(endedAt); s != "" {
		endedVal = s
	}
	return gin.H{
		"id":         idStr,
		"name":       util.ScanNullableString(name),
		"type":       util.ScanNullableString(typ),
		"date":       util.ScanNullableString(date),
		"scenarioId": util.ScanNullableString(scenarioID),
		"level":      util.ScanNullableString(level),
		"status":     statusV,
		"target":     util.ScanNullableString(target),
		"units":      util.ParseJSONArrAny(util.ScanNullableString(unitsStr)),
		"members":    util.ParseJSONArrAny(util.ScanNullableString(membersStr)),
		"observers":  util.ParseJSONArrAny(util.ScanNullableString(observersStr)),
		"mode":       modeV,
		"startedAt":  startedVal,
		"pausedAt":   pausedVal,
		"pausedSec":  ps,
		"endedAt":    endedVal,
		"elapsedSec": util.ElapsedSec(util.ScanNullableString(startedAt), util.ScanNullableString(pausedAt), util.ScanNullableString(endedAt), ps, statusV),
		"progress":   projectProgress(database, idStr, util.ScanNullableString(scenarioID), modeV, statusV),
	}
}

// =========================================================================
// /api/tasks/*  —— 旧版任务路径
// =========================================================================

// GetTasksLegacy GET /api/tasks
func GetTasksLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryTasks(database, auth.CurrentUser(c))) }
}

// PostTaskAccept POST /api/tasks/:id/accept
func PostTaskAccept(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		_, _ = database.Exec(`UPDATE tasks SET status='处理中', accepted_at=COALESCE(accepted_at,?) WHERE id=?`, util.HHMM(), id)
		c.JSON(http.StatusOK, gin.H{"ok": true, "status": "处理中"})
	}
}

// PostTaskSubmit POST /api/tasks/:id/submit
func PostTaskSubmit(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		eid := fmt.Sprintf("E%d", time.Now().UnixMilli())
		var pid int64
		_ = database.QueryRow(`SELECT project_id FROM tasks WHERE id=?`, id).Scan(&pid)
		_, _ = database.Exec(
			`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			eid, "处置记录", fmt.Sprintf("任务处置记录-%s(模拟)", id), "9 KB", util.HHMM(),
			u.Role.ID, u.Name, u.ID, 0, id, pid, util.Now(),
		)
		c.JSON(http.StatusOK, gin.H{"ok": true, "evidenceId": eid})
	}
}

// PostTaskComplete POST /api/tasks/:id/complete
func PostTaskComplete(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		var title, projectID, taskStatus string
		if err := database.QueryRow(
			`SELECT COALESCE(title,''),COALESCE(project_id,''),COALESCE(status,'') FROM tasks WHERE id=?`, id,
		).Scan(&title, &projectID, &taskStatus); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		wasDone := taskStatus == "已完成"
		_, _ = database.Exec(`UPDATE tasks SET status='已完成', completed_at=? WHERE id=?`, util.Now(), id)
		// 站内消息：节点完成（Node tasks.js:134-148；已完成的任务不再重复推送）
		if !wasDone {
			var projectName string
			_ = database.QueryRow(`SELECT COALESCE(name,'') FROM projects WHERE id=?`, projectID).Scan(&projectName)
			var total, done int
			_ = database.QueryRow(
				`SELECT COUNT(*),COALESCE(SUM(CASE WHEN status='已完成' THEN 1 ELSE 0 END),0) FROM tasks WHERE project_id=?`, projectID,
			).Scan(&total, &done)
			userName, roleName := "系统", ""
			if u != nil {
				userName, roleName = u.Name, u.Role.Name
			}
			pushNotif(database, "node_done", map[string]any{
				"title": title, "project": projectName, "taskId": id,
				"user": userName, "role": roleName, "done": done, "total": total,
			}, projectID, "", "", "internal")
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "status": "已完成"})
	}
}

// DeleteTaskLegacy DELETE /api/tasks/:id
func DeleteTaskLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		_, _ = database.Exec(`DELETE FROM task_attachments WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM form_submissions WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM evidence WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM tasks WHERE id=?`, id)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostTaskResetAll POST /api/tasks/reset-all
func PostTaskResetAll(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, _ = database.Exec(`UPDATE tasks SET status='待处理' WHERE status='已完成'`)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// GetTaskAttachments GET /api/tasks/:id/attachments
func GetTaskAttachments(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		rows, _ := database.Query(`SELECT id,name,type,size,COALESCE(url,''),uploader,time,COALESCE(auto,0) FROM task_attachments WHERE task_id=? ORDER BY id`, id)
		defer rows.Close()
		out := []gin.H{}
		for rows.Next() {
			var id2 int64
			var name, typ, size, url, uploader, timeStr string
			var auto int
			if rows.Scan(&id2, &name, &typ, &size, &url, &uploader, &timeStr, &auto) != nil {
				continue
			}
			out = append(out, gin.H{"id": id2, "name": name, "type": typ, "size": size, "url": url, "by": uploader, "time": timeStr, "auto": auto == 1})
		}
		c.JSON(http.StatusOK, out)
	}
}

// PostTaskAttachments POST /api/tasks/:id/attachments
func PostTaskAttachments(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DeleteTaskAttachment DELETE /api/tasks/:id/attachments/:attId
func DeleteTaskAttachment(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		attID := c.Param("attId")
		_, _ = database.Exec(`DELETE FROM task_attachments WHERE id=?`, attID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostTaskForms POST /api/tasks/:id/forms
func PostTaskForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		var body struct {
			FormID string         `json:"formId"`
			Data   map[string]any `json:"data"`
		}
		_ = c.ShouldBindJSON(&body)
		var formName string
		if database.QueryRow(`SELECT name FROM form_templates WHERE id=?`, body.FormID).Scan(&formName) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "表单模板不存在"})
			return
		}
		pid := id
		_ = database.QueryRow(`SELECT project_id FROM tasks WHERE id=?`, id).Scan(&pid)
		dataJSON := util.ToJSON(body.Data)
		res, _ := database.Exec(
			`INSERT INTO form_submissions (task_id,form_id,form_name,data,submitter,user_id,role_id,time) VALUES (?,?,?,?,?,?,?,?)`,
			id, body.FormID, formName, dataJSON, u.Name, u.ID, u.Role.ID, util.Now(),
		)
		var sid int64
		if res != nil {
			sid, _ = res.LastInsertId()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "submissionId": sid})
	}
}

// GetTaskForms GET /api/tasks/:id/forms
func GetTaskForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		rows, _ := database.Query(`SELECT id,form_id,form_name,COALESCE(data,''),submitter,time FROM form_submissions WHERE task_id=? ORDER BY id`, id)
		defer rows.Close()
		out := []gin.H{}
		for rows.Next() {
			var id2 int64
			var formID, formName, dataJSON, submitter, timeStr string
			if rows.Scan(&id2, &formID, &formName, &dataJSON, &submitter, &timeStr) != nil {
				continue
			}
			var data any
			_ = util.ParseJSON(dataJSON, &data, map[string]any{})
			out = append(out, gin.H{"id": id2, "formId": formID, "formName": formName, "data": data, "submitter": submitter, "time": timeStr})
		}
		c.JSON(http.StatusOK, out)
	}
}

// =========================================================================
// /api/evidence/*  —— 取证模块全套
// =========================================================================

// GetEvidenceLegacy GET /api/evidence
func GetEvidenceLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryEvidence(database, auth.CurrentUser(c))) }
}

// PostEvidenceLegacy POST /api/evidence
func PostEvidenceLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Size string `json:"size"`
			Auto bool   `json:"auto"`
			Task string `json:"taskId"`
			Proj string `json:"projectId"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Type == "" {
			body.Type = "文档"
		}
		if body.Name == "" {
			body.Name = "未命名材料"
		}
		id := fmt.Sprintf("E%d", time.Now().UnixMilli())
		_, _ = database.Exec(
			`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, body.Type, body.Name, body.Size, util.HHMM(), u.Role.ID, u.Name, u.ID, body.Auto, body.Task, body.Proj, util.Now(),
		)
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
	}
}

// PostEvidencePrefill POST /api/evidence/prefill
func PostEvidencePrefill(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true, "count": 0}) }
}

// PostEvidenceMock POST /api/evidence/mock
func PostEvidenceMock(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
}

// GetEvidenceRequirements GET /api/evidence/requirements
func GetEvidenceRequirements(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, []gin.H{}) }
}

// PostEvidenceUpload POST /api/evidence/upload
func PostEvidenceUpload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		ext := ""
		if idx := strings.LastIndex(file.Filename, "."); idx >= 0 {
			ext = file.Filename[idx:]
		}
		saved := fmt.Sprintf("evidence/%d%s", time.Now().UnixMilli(), ext)
		id := fmt.Sprintf("E%d", time.Now().UnixMilli())
		c.JSON(http.StatusOK, gin.H{"files": []gin.H{gin.H{
			"id": id, "name": file.Filename, "size": file.Size,
			"url": "/uploads/" + saved,
		}}})
	}
}

// GetEvidenceDownload GET /api/evidence/:id/download
func GetEvidenceDownload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var name string
		err := database.QueryRow(`SELECT COALESCE(name,'') FROM evidence WHERE id=?`, id).Scan(&name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "取证材料不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id, "name": name})
	}
}

// =========================================================================
// /api/admin —— 缺失的 report、restore-upload
// =========================================================================

// AdminRestoreUpload POST /api/admin/restore-upload
func AdminRestoreUpload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _ := c.FormFile("file")
		if file == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到备份文件"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "file": file.Filename})
	}
}

// AdminReport GET /api/admin/report/:projectId
func AdminReport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := c.Param("projectId")
		c.JSON(http.StatusOK, gin.H{"projectId": pid, "sections": []gin.H{}})
	}
}

// AdminReportExport POST /api/admin/report/:projectId/export
func AdminReportExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := c.Param("projectId")
		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": pid})
	}
}

// =========================================================================
// /api/drill —— 缺失的 workbench、flows、faults、decide、skip、finish、steps、predecessor、nodes/:id/start
// =========================================================================

// GetDrillWorkbench GET /api/drill/workbench/:projectId —— 工作台/复盘页共用
// 结构对齐 Node drill.js:486-496：projectId/mode/scenarioId/nodes/steps/evidence/score。
func GetDrillWorkbench(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := drillV1ProjectOf(database, c.Param("projectId"))
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		nodes := make([]gin.H, 0)
		rows, err := database.Query(
			`SELECT id,type,title,COALESCE(target,''),COALESCE(timeout,0),COALESCE(`+"`descr`"+`,''),
			        COALESCE(guide,''),COALESCE(keywords,''),COALESCE(action,''),COALESCE(required,1),
			        COALESCE(wrong,'[]'),COALESCE(form,''),COALESCE(flow_type,'correct'),COALESCE(score,0),
			        COALESCE(receiver,''),COALESCE(deps,'[]'),COALESCE(anchor_id,''),COALESCE(release_timing,'before')
			 FROM script_nodes WHERE scenario_id=? ORDER BY seq`, p.ScenarioID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, typ, title, target, descr, guide, keywords, action, wrongStr, form, flowType, receiver, depsStr, anchorID, releaseTiming string
				var timeout, required, score int
				if rows.Scan(&id, &typ, &title, &target, &timeout, &descr, &guide, &keywords, &action, &required,
					&wrongStr, &form, &flowType, &score, &receiver, &depsStr, &anchorID, &releaseTiming) != nil {
					continue
				}
				kw := make([]string, 0)
				for _, x := range strings.Split(keywords, ",") {
					if strings.TrimSpace(x) != "" {
						kw = append(kw, strings.TrimSpace(x))
					}
				}
				var wrong, deps any = []any{}, []any{}
				_ = json.Unmarshal([]byte(wrongStr), &wrong)
				_ = json.Unmarshal([]byte(depsStr), &deps)
				if flowType != "distractor" {
					flowType = "correct"
				}
				if flowType == "distractor" {
					continue // 对齐 Node：工作台/复盘页 nodes 不返回混淆节点
				}
				nodes = append(nodes, gin.H{
					"id": id, "type": typ, "title": title, "target": target, "timeout": timeout,
					"desc": descr, "guide": guide, "keywords": kw, "action": action,
					"required": required == 1, "wrong": wrong, "form": form, "flowType": flowType,
					"score": score, "receiver": receiver, "deps": deps,
					"anchorId": anchorID, "releaseTiming": releaseTiming,
				})
			}
		}

		steps := make([]gin.H, 0)
		srows, err := database.Query(`SELECT * FROM drill_steps WHERE project_id=? ORDER BY step_time,id`, p.ID)
		if err == nil {
			defer srows.Close()
			cols, _ := srows.Columns()
			for srows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if srows.Scan(ptrs...) != nil {
					continue
				}
				m := gin.H{}
				for i, col := range cols {
					v := vals[i]
					if b, ok := v.([]byte); ok {
						v = string(b)
					}
					m[col] = v
				}
				steps = append(steps, m)
			}
		}

		mode := p.Mode
		if mode == "" {
			mode = "基础版"
		}
		c.JSON(http.StatusOK, gin.H{
			"projectId":  p.ID,
			"mode":       mode,
			"scenarioId": p.ScenarioID,
			"nodes":      nodes,
			"steps":      steps,
			"evidence":   loadEvidenceSummary(database, p),
			"score":      Score(database, p.ID),
		})
	}
}

// PostDrillNodesStart POST /api/drill/nodes/:nodeId/start
func PostDrillNodesStart(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("nodeId")
		u := auth.CurrentUser(c)
		activeID := activeProjectID(database)
		_, _ = database.Exec(
			`INSERT INTO drill_node_state (project_id,node_id,user,user_id,started_at) VALUES (?,?,?,?,?)`,
			activeID, nodeID, u.Name, u.ID, util.Now(),
		)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// drillV1DistractorReleased 复刻 Node src/routes/drill.js L83-99 isDistractorReleased：
// distractor 无锚点/锚点缺失时兜底可见；after 需锚点完成；before 需锚点 deps 全部完成。
func drillV1DistractorReleased(node drillV1Node, byID map[string]*drillV1Node, done map[string]bool) bool {
	if node.FlowType != "distractor" {
		return true
	}
	anchorID := strings.TrimSpace(node.AnchorID)
	if anchorID == "" {
		return true
	}
	anchor, ok := byID[anchorID]
	if !ok {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(node.ReleaseTiming), "after") {
		return done[anchorID]
	}
	for _, depID := range anchor.Deps {
		if _, ok := byID[depID]; ok && !done[depID] {
			return false
		}
	}
	return true
}

// drillV1FormFields 复刻 Node drill.js formFields(formId) → (name, fields)
func drillV1FormFields(database *db.DB, formID string) (string, []any) {
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return "", []any{}
	}
	var name, fieldsStr sql.NullString
	if err := database.QueryRow(`SELECT name,fields FROM form_templates WHERE id=?`, formID).Scan(&name, &fieldsStr); err != nil {
		return "", []any{}
	}
	nm := util.ScanNullableString(name)
	if nm == "" {
		nm = formID
	}
	return nm, normalizeFields(util.ParseJSONArrAny(util.ScanNullableString(fieldsStr)))
}

// GetDrillFlows GET /api/drill/flows/:projectId
// 结构对齐 Node drill.js:530-545：projectId/mode/projectStatus/flowCount/flows/receivers/evidence/score。
// flows 的键与 Node flowList 一致（nodeId/timeout/type/action/target/title/desc/formId/formName/
// fields/locked/blockedBy/submitted），混淆节点按 isDistractorReleased 门控。
func GetDrillFlows(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位仅可查看医院上报材料（分级权限隔离）"})
			return
		}
		p := drillV1ProjectOf(database, c.Param("projectId"))
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		mode := strings.TrimSpace(p.Mode)
		if mode == "" {
			mode = "基础版"
		}
		if mode != "实战版" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前项目不是实战版"})
			return
		}
		// 状态门控：仅「演练中」下发候选处置流程；未开始/暂停/已结束/已归档一律返回空列表，
		// projectStatus 如实回显。步骤/评分/取证等其他接口不受影响（见 loadEvidenceSummary/Score）。
		if strings.TrimSpace(p.Status) != "演练中" {
			c.JSON(http.StatusOK, gin.H{
				"projectId":     p.ID,
				"mode":          "实战版",
				"projectStatus": p.Status,
				"flowCount":     0,
				"flows":         []gin.H{},
				"receivers":     []string{},
				"evidence":      loadEvidenceSummary(database, p),
				"score":         Score(database, p.ID),
			})
			return
		}

		nodes := drillV1NodesOf(database, p.ScenarioID)
		byID := make(map[string]*drillV1Node, len(nodes))
		flowNodes := make([]drillV1Node, 0, len(nodes))
		for i := range nodes {
			if nodes[i].Type == "end" {
				continue
			}
			byID[nodes[i].ID] = &nodes[i]
			flowNodes = append(flowNodes, nodes[i])
		}
		done := loadDoneSet(database, p.ID)

		// 每条流程取最早一条处置记录（Node: steps.filter(matched_node===id)[0]，steps 按 step_time,id 排序）
		stepByNode := map[string]gin.H{}
		srows, err := database.Query(
			`SELECT matched_node,COALESCE(step_time,''),COALESCE(user,''),COALESCE(role_id,''),COALESCE(correct,1),
			        COALESCE(ontime,0),COALESCE(found_at,''),COALESCE(found,''),COALESCE(measure,''),COALESCE(result,''),
			        COALESCE(evidence,''),COALESCE(forms,'{}'),COALESCE(score,0),COALESCE(receiver,''),COALESCE(receiver_ok,1),COALESCE(skipped,0)
			 FROM drill_steps WHERE project_id=? ORDER BY step_time,id`, p.ID)
		if err == nil {
			defer srows.Close()
			for srows.Next() {
				var nid, stepTime, user, roleID, foundAt, found, measure, result, evidence, formsStr, receiver string
				var correct, ontime, score, receiverOK, skipped int
				if srows.Scan(&nid, &stepTime, &user, &roleID, &correct, &ontime, &foundAt, &found, &measure,
					&result, &evidence, &formsStr, &score, &receiver, &receiverOK, &skipped) != nil {
					continue
				}
				if nid == "" {
					continue
				}
				if _, exists := stepByNode[nid]; exists {
					continue
				}
				if correct == 0 {
					correct = 0
				} else {
					correct = 1
				}
				if receiverOK == 0 {
					receiverOK = 0
				} else {
					receiverOK = 1
				}
				if skipped == 1 {
					skipped = 1
				} else {
					skipped = 0
				}
				stepByNode[nid] = gin.H{
					"stepTime": stepTime, "user": user, "role": roleID,
					"correct": correct, "ontime": ontime == 1, "foundAt": foundAt,
					"found": found, "measure": measure, "result": result, "evidence": evidence,
					"forms": util.ParseJSONObj(formsStr), "delta": score,
					"receiver": receiver, "receiverOk": receiverOK, "skipped": skipped,
				}
			}
		}

		flows := make([]gin.H, 0)
		for _, n := range flowNodes {
			if !canHandleRole(database, u.Role.ID, n.Target) {
				continue
			}
			if !drillV1DistractorReleased(n, byID, done) {
				continue
			}
			formName, fields := drillV1FormFields(database, n.Form)
			lock := drillV1NodeLock(n, byID, done)
			var submitted any
			if st, ok := stepByNode[n.ID]; ok {
				submitted = st
			}
			flows = append(flows, gin.H{
				"nodeId": n.ID, "timeout": n.Timeout, "type": n.Type, "action": n.Action,
				"target": n.Target, "title": n.Title, "desc": n.Desc, "formId": n.Form,
				"formName": formName, "fields": fields,
				"locked": lock.Locked, "blockedBy": lock.BlockedBy, "submitted": submitted,
			})
		}
		// 与 Node 一致的稳定哈希排序，避免混淆流程集中在一处
		sort.SliceStable(flows, func(i, j int) bool {
			return flowHash(flows[i]["nodeId"].(string)) < flowHash(flows[j]["nodeId"].(string))
		})

		receivers := make([]string, 0)
		rrows, err := database.Query(`SELECT name FROM roles ORDER BY seq`)
		if err == nil {
			defer rrows.Close()
			for rrows.Next() {
				var name sql.NullString
				if rrows.Scan(&name) == nil {
					receivers = append(receivers, util.ScanNullableString(name))
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"projectId":     p.ID,
			"mode":          "实战版",
			"projectStatus": p.Status,
			"flowCount":     len(flows),
			"flows":         flows,
			"receivers":     receivers,
			"evidence":      loadEvidenceSummary(database, p),
			"score":         Score(database, p.ID),
		})
	}
}

// flowHash 复刻 Node flowList 的排序哈希（h*31 + charCode，uint32）
func flowHash(s string) uint32 {
	var h uint32
	for _, ch := range []byte(s) {
		h = h*31 + uint32(ch)
	}
	return h
}

// GetDrillFaults GET /api/drill/faults/:projectId
func GetDrillFaults(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := c.Param("projectId")
		c.JSON(http.StatusOK, gin.H{"projectId": pid, "faults": []gin.H{}})
	}
}

// =========================================================================
// /api/system/logo
// =========================================================================

// PostSystemLogo POST /api/system/logo
func PostSystemLogo(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _ := c.FormFile("file")
		if file == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "logo": "/uploads/brand/logo.png"})
	}
}
