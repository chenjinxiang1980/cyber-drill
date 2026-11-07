// Package handler —— 补齐 Go 后端缺失的路由 handler
//
// 本文件承载从旧 Node 版迁移但 Go router.go 未注册的接口实现。
// 全部 handler 风格与既有 handler 保持一致：返回 gin.H 或数组，不用
// 包装层 {ok,data}（Node 版就是直接返回对象）。

package handler

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
		if !ensureBoardWrite(c, u) {
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
		if !ensureBoardWrite(c, auth.CurrentUser(c)) {
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
		if !ensureBoardWrite(c, u) {
			return
		}
		sys := c.Param("sys")
		var body struct {
			State string   `json:"state"`
			Pct   *float64 `json:"pct"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
			return
		}
		if body.State == "" && body.Pct == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无更新字段"})
			return
		}
		if body.Pct != nil && (*body.Pct < 0 || *body.Pct > 100) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "影响比例需在 0-100 之间"})
			return
		}
		activeID := activeProjectID(database)
		if activeID == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "当前没有进行中的演练项目"})
			return
		}
		var status string
		if err := database.QueryRow(`SELECT COALESCE(status,'') FROM projects WHERE id=?`, activeID).Scan(&status); err != nil || status == "已归档" {
			c.JSON(http.StatusConflict, gin.H{"error": "当前演练项目已归档，不可修改影响面"})
			return
		}
		var exists int
		if err := database.QueryRow(`SELECT COUNT(*) FROM impact WHERE sys=? AND project_id=?`, sys, activeID).Scan(&exists); err != nil || exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "系统不存在或未被当前演练项目引用"})
			return
		}
		if body.State != "" {
			if _, err := database.Exec(`UPDATE impact SET state=? WHERE sys=? AND project_id=?`, body.State, sys, activeID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新影响面失败"})
				return
			}
		}
		if body.Pct != nil {
			if _, err := database.Exec(`UPDATE impact SET pct=? WHERE sys=? AND project_id=?`, *body.Pct, sys, activeID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新影响面失败"})
				return
			}
		}
		writeAudit(database, u, c, "更新影响面："+sys)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostAudit POST /api/content/audit —— 兼容旧前端：接口保留但不再写入任何客户端字段。
// 真正的审计由服务端 AuditWrites 中间件与各业务 handler 生成；客户端提交的
// action 自由文本一律丢弃，避免伪造/污染审计结构化字段。
func PostAudit(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": true})
	}
}

// PostOpinion POST /api/content/opinion —— 新增舆情
func PostOpinion(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if !ensureBoardWrite(c, u) {
			return
		}
		var body struct {
			Channel string `json:"channel"`
			Title   string `json:"title"`
			Heat    int    `json:"heat"`
			Risk    string `json:"risk"`
			Source  string `json:"source"`
		}
		_ = c.ShouldBindJSON(&body)
		if strings.TrimSpace(body.Title) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "舆情标题不能为空"})
			return
		}
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
		u := auth.CurrentUser(c)
		if !ensureBoardWrite(c, u) {
			return
		}
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
		res, err := database.Exec(fmt.Sprintf("UPDATE opinion SET %s WHERE id=?", strings.Join(fields, ",")), args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新舆情失败"})
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// MySQL 默认返回“实际变更行数”，相同值更新也会是 0；这里用查询确认存在性
			var exists int
			if database.QueryRow(`SELECT COUNT(*) FROM opinion WHERE id=?`, id).Scan(&exists); exists == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "舆情记录不存在"})
				return
			}
		}
		writeAudit(database, u, c, "更新舆情："+id)
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
		u := auth.CurrentUser(c)
		if !ensureBoardWrite(c, u) {
			return
		}
		id := c.Param("id")
		var status string
		if err := database.QueryRow(`SELECT COALESCE(status,'') FROM projects WHERE id=?`, id).Scan(&status); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status == "已归档" {
			c.JSON(http.StatusConflict, gin.H{"error": "已归档项目不可修改成员"})
			return
		}
		var body struct {
			Members []any `json:"members"`
		}
		_ = c.ShouldBindJSON(&body)
		membersJSON := util.ToJSON(body.Members)
		_, _ = database.Exec("UPDATE projects SET members=? WHERE id=?", membersJSON, id)
		writeAudit(database, u, c, "更新演练项目成员："+id)
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
// 导调中心「节点操作情况」：按当前激活项目关联 scenario 剧本节点、处置记录与节点打开状态，
// 返回处置中 / 已完成 / 决策偏离的节点。口径与大屏 computeMonitorNodes 保持一致：
// 实战版以 drill_steps 为准，基础版回退到 tasks 状态；end 与 distractor 节点不进入盘面。
func GetDrillProgress(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.DefaultQuery("channel", "inner")
		activeID := activeProjectID(database)
		if activeID == "" {
			c.JSON(http.StatusOK, gin.H{"project": nil, "mode": "", "nodes": []gin.H{}, "channel": channel})
			return
		}
		var pName, pMode, pStatus, pDate, scenarioID string
		if err := database.QueryRow(
			`SELECT COALESCE(name,''),COALESCE(mode,'基础版'),COALESCE(status,''),COALESCE(date,''),COALESCE(scenario_id,'') FROM projects WHERE id=?`, activeID,
		).Scan(&pName, &pMode, &pStatus, &pDate, &scenarioID); err != nil {
			c.JSON(http.StatusOK, gin.H{"project": nil, "mode": "", "nodes": []gin.H{}, "channel": channel})
			return
		}

		type roleLite struct {
			id, name, short string
			external        bool
		}
		roles := map[string]roleLite{}
		if rows, err := database.Query(`SELECT id,COALESCE(name,''),COALESCE(short,''),COALESCE(` + "`external`" + `,0) FROM roles`); err == nil {
			for rows.Next() {
				var r roleLite
				var ext int
				if rows.Scan(&r.id, &r.name, &r.short, &ext) == nil {
					r.external = ext == 1
					roles[r.id] = r
				}
			}
			rows.Close()
		}
		roleByTarget := map[string]roleLite{}
		for _, r := range roles {
			if strings.TrimSpace(r.name) != "" {
				roleByTarget[strings.TrimSpace(r.name)] = r
			}
			if strings.TrimSpace(r.short) != "" {
				roleByTarget[strings.TrimSpace(r.short)] = r
			}
		}

		type stepLite struct {
			stepTime, user string
			correct, skip  int
		}
		steps := map[string]stepLite{}
		if rows, err := database.Query(
			`SELECT COALESCE(matched_node,''),COALESCE(correct,1),COALESCE(skipped,0),COALESCE(step_time,''),COALESCE(user,'')
			 FROM drill_steps WHERE project_id=? ORDER BY step_time,id`, activeID,
		); err == nil {
			for rows.Next() {
				var nid string
				var s stepLite
				if rows.Scan(&nid, &s.correct, &s.skip, &s.stepTime, &s.user) == nil && nid != "" {
					if _, ok := steps[nid]; !ok {
						steps[nid] = s
					}
				}
			}
			rows.Close()
		}
		type stateLite struct{ user, startedAt string }
		states := map[string]stateLite{}
		if rows, err := database.Query(`SELECT COALESCE(node_id,''),COALESCE(user,''),COALESCE(started_at,'') FROM drill_node_state WHERE project_id=?`, activeID); err == nil {
			for rows.Next() {
				var nid string
				var s stateLite
				if rows.Scan(&nid, &s.user, &s.startedAt) == nil && nid != "" {
					states[nid] = s
				}
			}
			rows.Close()
		}
		type taskLite struct{ status, acceptedAt, completedAt, roleID string }
		tasks := map[string]taskLite{}
		if rows, err := database.Query(
			`SELECT COALESCE(node_id,''),COALESCE(status,''),COALESCE(accepted_at,''),COALESCE(completed_at,''),COALESCE(role_id,'')
			 FROM tasks WHERE project_id=?`, activeID,
		); err == nil {
			for rows.Next() {
				var nid string
				var t taskLite
				if rows.Scan(&nid, &t.status, &t.acceptedAt, &t.completedAt, &t.roleID) == nil && nid != "" {
					tasks[nid] = t
				}
			}
			rows.Close()
		}

		type scriptNode struct {
			id, title, shortTitle, target, flowType string
			seq, timeout                            int
		}
		scriptNodes := []scriptNode{}
		if rows, err := database.Query(
			`SELECT id,COALESCE(title,''),COALESCE(short_title,''),COALESCE(target,''),COALESCE(flow_type,'correct'),COALESCE(seq,0),COALESCE(timeout,0)
			 FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND COALESCE(type,'task')!='end' ORDER BY seq`, scenarioID,
		); err == nil {
			for rows.Next() {
				var n scriptNode
				if rows.Scan(&n.id, &n.title, &n.shortTitle, &n.target, &n.flowType, &n.seq, &n.timeout) == nil {
					scriptNodes = append(scriptNodes, n)
				}
			}
			rows.Close()
		}

		nodes := []gin.H{}
		for _, n := range scriptNodes {
			if strings.EqualFold(strings.TrimSpace(n.flowType), "distractor") {
				continue
			}
			targetRole := roleByTarget[strings.TrimSpace(n.target)]
			if channel == "external" && !targetRole.external {
				continue
			}
			status, opTime, actor := "", "", ""
			if s, ok := steps[n.id]; ok {
				status = "done"
				if s.correct == 0 {
					status = "wrong"
				}
				opTime, actor = s.stepTime, s.user
			} else if s, ok := states[n.id]; ok {
				status, opTime, actor = "doing", s.startedAt, s.user
			} else if t, ok := tasks[n.id]; ok {
				switch strings.TrimSpace(t.status) {
				case "已完成", "超时":
					status, opTime = "done", t.completedAt
				case "处理中":
					status, opTime = "doing", t.acceptedAt
				}
				if actor == "" {
					if r, ok := roles[t.roleID]; ok {
						actor = r.name
					}
				}
			}
			if status == "" {
				continue // 未处置节点不进入操作盘面
			}
			title := strings.TrimSpace(n.title)
			if title == "" {
				title = strings.TrimSpace(n.shortTitle)
			}
			if title == "" {
				title = strings.TrimSpace(n.target)
			}
			scope := "inner"
			if targetRole.external {
				scope = "external"
			}
			nodes = append(nodes, gin.H{
				"nodeId": n.id, "id": n.id, "seq": n.seq, "title": title,
				"target": n.target, "targetRole": targetRole.name,
				"status": status, "actor": actor, "opTime": opTime,
				"duration": n.timeout, "scope": scope,
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"project":   gin.H{"id": activeID, "name": pName, "mode": pMode, "status": pStatus, "date": pDate, "scenarioId": scenarioID},
			"mode":      pMode,
			"nodes":     nodes,
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
		u := auth.CurrentUser(c)
		if !ensureBoardWrite(c, u) {
			return
		}
		id := c.Param("id")
		var name, status string
		err := database.QueryRow(`SELECT COALESCE(name,''),COALESCE(status,'') FROM projects WHERE id=?`, id).Scan(&name, &status)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status == "已归档" {
			c.JSON(http.StatusConflict, gin.H{"error": "已归档项目不可设为当前演练项目"})
			return
		}
		_, _ = database.Exec(
			`INSERT INTO settings (key,value) VALUES ('active_project',?) ON DUPLICATE KEY UPDATE value=VALUES(value)`,
			id,
		)
		writeAudit(database, u, c, "设为当前演练项目："+name)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PatchProjectStatus PATCH /api/projects/:id/status
func PatchProjectStatus(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if !ensureBoardWrite(c, u) {
			return
		}
		id := c.Param("id")
		var body struct {
			Status    string         `json:"status"`
			Exception map[string]any `json:"exception"`
			Force     bool           `json:"force"`
			Reason    string         `json:"reason"`
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
		if !canProjectStatusTransit(prev, body.Status) {
			c.JSON(http.StatusConflict, gin.H{"error": "不允许从「" + prev + "」变更为「" + body.Status + "」"})
			return
		}
		/* 结束演练必须走封存流程：复用 /api/drill/finish 的核对清单、缺项说明与快照写入，
		 * 避免状态接口绕过封存（历史 P2026-001 即由本接口收口，无快照、timeline 缺结束记录）。
		 *   - 证据齐全：直接结束并封存；
		 *   - 核心证据缺项：返回 400/needsException，携带 exception（原因/责任人/时限/批准人）重试；
		 *   - 运维例外：force=true 且 reason 必填，快照标记「异常封存 / 异常结束」并落审计。 */
		if body.Status == "已结束" && prev != "已结束" {
			p := drillV1ProjectOf(database, id)
			if p == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
				return
			}
			finishBody := map[string]any{
				"projectId": id,
				"force":     body.Force,
				"reason":    strings.TrimSpace(body.Reason),
			}
			if body.Exception != nil {
				finishBody["exception"] = body.Exception
			}
			code, payload := executeDrillFinish(database, u, p, finishBody)
			if code != http.StatusOK {
				payload["finishUrl"] = "/api/drill/finish"
				payload["hint"] = "结束演练需走结束流程：可携带 exception 说明缺项，或由管理员 force + reason 强制结束"
				c.JSON(code, payload)
				return
			}
			payload["project"] = projectJSONByID(database, id)
			c.JSON(http.StatusOK, payload)
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
		writeAudit(database, u, c, fmt.Sprintf("项目状态变更：%s %s→%s", name, prev, body.Status))
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

// ensureBoardWrite 写接口的 handler 层二次校验：
// 外部单位一律 403；非“平台管理员/指挥部/信息科负责人”403。
// 路由层已挂对应中间件，这里是防漏挂的第二道闸。
func ensureBoardWrite(c *gin.Context, u *auth.User) bool {
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return false
	}
	if u.Role.External {
		c.JSON(http.StatusForbidden, gin.H{"error": "外部单位仅可查看医院上报材料（分级权限隔离）"})
		return false
	}
	if !auth.IsPlatformBoard(u.Role.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可操作"})
		return false
	}
	return true
}

type taskWriteTarget struct {
	User          *auth.User
	Title         string
	ProjectID     string
	ProjectStatus string
	RoleID        string
	Status        string
}

// guardTaskWrite 任务写操作统一守门：登录、非外部、任务存在、项目未归档、
// 当前角色可处置该任务。
func guardTaskWrite(database *db.DB, c *gin.Context) (*taskWriteTarget, bool) {
	u := auth.CurrentUser(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return nil, false
	}
	if u.Role.External {
		c.JSON(http.StatusForbidden, gin.H{"error": "外部单位仅可查看医院上报材料（分级权限隔离）"})
		return nil, false
	}
	taskID := c.Param("id")
	t := &taskWriteTarget{User: u}
	if err := database.QueryRow(
		`SELECT COALESCE(t.title,''),COALESCE(t.project_id,''),COALESCE(p.status,''),COALESCE(t.role_id,''),COALESCE(t.status,'')
		 FROM tasks t LEFT JOIN projects p ON p.id=t.project_id WHERE t.id=?`, taskID,
	).Scan(&t.Title, &t.ProjectID, &t.ProjectStatus, &t.RoleID, &t.Status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return nil, false
	}
	if t.ProjectStatus == "已归档" {
		c.JSON(http.StatusConflict, gin.H{"error": "已归档项目的任务不可再修改"})
		return nil, false
	}
	if !canManageTask(database, u, t.RoleID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限操作该任务"})
		return nil, false
	}
	return t, true
}

// canProjectStatusTransit 项目状态白名单：未开始→演练中→暂停/已结束→已归档。
// 同状态视为幂等；禁止已归档回退、已结束重新开始等任意跳转。
func canProjectStatusTransit(from, to string) bool {
	if from == "" || from == to {
		return true
	}
	switch from {
	case "未开始":
		return to == "演练中"
	case "演练中":
		return to == "暂停" || to == "已结束"
	case "暂停":
		return to == "演练中" || to == "已结束"
	case "已结束":
		return to == "已归档"
	}
	return false
}

// GetTasksLegacy GET /api/tasks
func GetTasksLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryTasks(database, auth.CurrentUser(c))) }
}

// PostTaskAccept POST /api/tasks/:id/accept
func PostTaskAccept(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := guardTaskWrite(database, c)
		if !ok {
			return
		}
		if t.Status == "已完成" {
			c.JSON(http.StatusConflict, gin.H{"error": "已完成任务不可重新接单"})
			return
		}
		id := c.Param("id")
		_, _ = database.Exec(`UPDATE tasks SET status='处理中', accepted_at=COALESCE(accepted_at,?) WHERE id=?`, util.HHMM(), id)
		c.JSON(http.StatusOK, gin.H{"ok": true, "status": "处理中"})
	}
}

// PostTaskSubmit POST /api/tasks/:id/submit
func PostTaskSubmit(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := guardTaskWrite(database, c)
		if !ok {
			return
		}
		if t.Status == "已完成" {
			c.JSON(http.StatusConflict, gin.H{"error": "已完成任务不可重复提交处置记录"})
			return
		}
		id := c.Param("id")
		u := t.User
		eid := fmt.Sprintf("E%d", time.Now().UnixMilli())
		_, _ = database.Exec(
			`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			eid, "处置记录", fmt.Sprintf("任务处置记录-%s(模拟)", id), "9 KB", util.HHMM(),
			u.Role.ID, u.Name, u.ID, 0, id, t.ProjectID, util.Now(),
		)
		c.JSON(http.StatusOK, gin.H{"ok": true, "evidenceId": eid})
	}
}

// PostTaskComplete POST /api/tasks/:id/complete
func PostTaskComplete(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := guardTaskWrite(database, c); !ok {
			return
		}
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
		t, ok := guardTaskWrite(database, c)
		if !ok {
			return
		}
		id := c.Param("id")
		_, _ = database.Exec(`DELETE FROM task_attachments WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM form_submissions WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM evidence WHERE task_id=?`, id)
		_, _ = database.Exec(`DELETE FROM tasks WHERE id=?`, id)
		writeAudit(database, t.User, c, "删除任务："+t.Title)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// PostTaskResetAll POST /api/tasks/reset-all
// 仅 platformadmin / commander；已归档项目的任务不参与重置。
func PostTaskResetAll(database *db.DB) gin.HandlerFunc {
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
		switch u.Role.ID {
		case auth.RolePlatformAdmin, auth.RoleCommander:
		default:
			c.JSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部可重置任务状态"})
			return
		}
		res, err := database.Exec(
			`UPDATE tasks t JOIN projects p ON p.id=t.project_id
			 SET t.status='待处理'
			 WHERE t.status='已完成' AND COALESCE(p.status,'')<>'已归档'`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "重置任务失败"})
			return
		}
		n, _ := res.RowsAffected()
		writeAudit(database, u, c, fmt.Sprintf("重置全部任务状态（影响 %d 条）", n))
		c.JSON(http.StatusOK, gin.H{"ok": true, "affected": n})
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

// =========================================================================
// 任务附件：真实落盘 + 落库 + 权限校验
// =========================================================================

// dangerousAttExts 可执行/脚本类扩展名黑名单，上传前整体拦截。
var dangerousAttExts = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true,
	".pif": true, ".msi": true, ".ps1": true, ".vbs": true, ".vbe": true,
	".js": true, ".jse": true, ".wsf": true, ".wsh": true, ".hta": true,
	".dll": true, ".cpl": true, ".jar": true, ".sh": true, ".bash": true,
	".py": true, ".php": true, ".asp": true, ".aspx": true, ".jsp": true,
	".reg": true, ".lnk": true, ".apk": true, ".bin": true, ".so": true,
	".dylib": true, ".elf": true,
}

// guessAttType 对齐前端 guessType / Node evidence.js 的材料分类。
func guessAttType(name string) string {
	n := strings.ToLower(name)
	switch strings.ToLower(filepath.Ext(n)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return "截图"
	case ".pcap", ".cap", ".pcapng":
		return "流量包摘要"
	case ".log", ".txt", ".csv":
		return "日志片段"
	case ".cfg", ".conf", ".ini", ".yaml", ".yml", ".json":
		return "配置备份"
	}
	return "文档"
}

// sanitizeFileName 保留原始展示名，去掉路径与非法字符，按 rune 截断。
func sanitizeFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	base = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, base)
	rs := []rune(base)
	if len(rs) > 120 {
		base = string(rs[:120])
	}
	if base == "" || base == "." {
		base = "未命名附件"
	}
	return base
}

// saveUploadedFile 写入 uploads/<sub>/<YYYYMM>/，返回相对路径与绝对路径。
func saveUploadedFile(uploadDir, sub, orig string, fh *multipart.FileHeader) (string, string, error) {
	subDir := filepath.Join(sub, time.Now().Format("200601"))
	targetDir := filepath.Join(uploadDir, subDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}
	storedName := fmt.Sprintf("%d-%s-%s", time.Now().UnixMilli(), randHex(4), sanitizeFileName(orig))
	full := filepath.Join(targetDir, storedName)
	src, err := fh.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()
	dst, err := os.Create(full)
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(full)
		return "", "", err
	}
	if err := dst.Close(); err != nil {
		os.Remove(full)
		return "", "", err
	}
	return filepath.ToSlash(filepath.Join(subDir, storedName)), full, nil
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(buf)
}

// removeUploadFile 删除 uploads 下的相对路径文件，拒绝目录逃逸。
func removeUploadFile(uploadDir, rel string) {
	if strings.TrimSpace(rel) == "" {
		return
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return
	}
	full := filepath.Join(uploadDir, clean)
	if check, err := filepath.Rel(uploadDir, full); err != nil || strings.HasPrefix(check, "..") {
		return
	}
	_ = os.Remove(full)
}

// canManageTask 非管理角色只能操作本角色负责的任务。
func canManageTask(database *db.DB, u *auth.User, roleID string) bool {
	switch u.Role.ID {
	case "platformadmin", "commander", "itlead":
		return true
	}
	return roleID == "" || roleID == u.Role.ID
}

// PostTaskAttachments POST /api/tasks/:id/attachments（multipart，字段 files，兼容 file）
func PostTaskAttachments(database *db.DB, uploadDir string, maxMB int) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可上传任务附件"})
			return
		}
		taskID := c.Param("id")
		var projectID, roleID, projectStatus, nodeID string
		if err := database.QueryRow(
			`SELECT COALESCE(t.project_id,''),COALESCE(t.role_id,''),COALESCE(p.status,''),COALESCE(t.node_id,'')
			 FROM tasks t LEFT JOIN projects p ON p.id=t.project_id WHERE t.id=?`, taskID,
		).Scan(&projectID, &roleID, &projectStatus, &nodeID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		if projectStatus == "已归档" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "已归档项目的任务不可再上传附件"})
			return
		}
		if !canManageTask(database, u, roleID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "该任务由「" + roleID + "」负责，当前角色不可代为上传附件"})
			return
		}

		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		files := form.File["files"]
		if len(files) == 0 {
			files = form.File["file"]
		}
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		if len(files) > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "单次最多上传 10 个文件"})
			return
		}
		maxBytes := int64(maxMB) * 1024 * 1024
		for _, fh := range files {
			if fh.Size <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "文件内容为空：" + fh.Filename})
				return
			}
			if maxBytes > 0 && fh.Size > maxBytes {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("文件「%s」超过 %dMB 限制", sanitizeFileName(fh.Filename), maxMB)})
				return
			}
			if dangerousAttExts[strings.ToLower(filepath.Ext(fh.Filename))] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "不允许上传可执行程序或脚本：" + sanitizeFileName(fh.Filename)})
				return
			}
		}

		type savedFile struct {
			rel, full, name, typ, size string
			id                         int64
		}
		saved := make([]savedFile, 0, len(files))
		cleanup := func() {
			for _, s := range saved {
				_ = os.Remove(s.full)
			}
		}
		for _, fh := range files {
			rel, full, err := saveUploadedFile(uploadDir, "tasks", fh.Filename, fh)
			if err != nil {
				cleanup()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败：" + err.Error()})
				return
			}
			saved = append(saved, savedFile{
				rel: rel, full: full,
				name: sanitizeFileName(fh.Filename), typ: guessAttType(fh.Filename), size: fmtSizeGo(fh.Size),
			})
		}

		tx, err := database.Begin()
		if err != nil {
			cleanup()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务开启失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()
		out := make([]gin.H, 0, len(saved))
		nowTime := util.HHMM()
		for i := range saved {
			res, err := tx.Exec(
				`INSERT INTO task_attachments (task_id,name,type,size,url,file_path,uploader,uploader_id,time,auto)
				 VALUES (?,?,?,?,?,?,?,?,?,0)`,
				taskID, saved[i].name, saved[i].typ, saved[i].size, "/uploads/"+saved[i].rel, saved[i].rel,
				u.Name, u.ID, nowTime,
			)
			if err != nil {
				cleanup()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "附件记录保存失败"})
				return
			}
			attID, _ := res.LastInsertId()
			saved[i].id = attID
			// 镜像一条取证材料，保证取证模块与前置详情都能看到同一份文件
			evID := fmt.Sprintf("E%d_%d", time.Now().UnixMilli(), attID)
			_, _ = tx.Exec(
				`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,node_id,created_at)
				 VALUES (?,?,?,?,?,?,?,?,0,?,?,?,?,?,?)`,
				evID, saved[i].typ, saved[i].name+"（任务"+taskID+"）", saved[i].size, nowTime,
				u.Role.ID, u.Name, u.ID, taskID, projectID, "/uploads/"+saved[i].rel, saved[i].rel,
				nullableText(nodeID), util.Now(),
			)
			out = append(out, gin.H{
				"id": attID, "name": saved[i].name, "type": saved[i].typ, "size": saved[i].size,
				"url": "/uploads/" + saved[i].rel, "by": u.Name, "time": nowTime, "auto": false,
			})
		}
		if err := tx.Commit(); err != nil {
			cleanup()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "附件提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "attachments": out})
	}
}

// DeleteTaskAttachment DELETE /api/tasks/:id/attachments/:attId
func DeleteTaskAttachment(database *db.DB, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		taskID, attID := c.Param("id"), c.Param("attId")
		var filePath, name string
		var uploaderID int64
		if err := database.QueryRow(
			`SELECT COALESCE(file_path,''),COALESCE(uploader_id,0),COALESCE(name,'') FROM task_attachments WHERE id=? AND task_id=?`,
			attID, taskID,
		).Scan(&filePath, &uploaderID, &name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "附件不存在"})
			return
		}
		allowed := false
		switch u.Role.ID {
		case "platformadmin", "commander", "itlead":
			allowed = true
		default:
			allowed = uploaderID == u.ID
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅上传者本人或指挥部/信息科负责人/平台管理员可删除附件"})
			return
		}
		removeUploadFile(uploadDir, filePath)
		if _, err := database.Exec(`DELETE FROM task_attachments WHERE id=? AND task_id=?`, attID, taskID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "附件删除失败"})
			return
		}
		if filePath != "" {
			_, _ = database.Exec(`DELETE FROM evidence WHERE task_id=? AND file_path=?`, taskID, filePath)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": attID, "name": name})
	}
}

// validateReportAssessment 校验 F19《报案必要性评估单》的跨字段一致性：
//   - 评估结论为“达到”报案标准时，处置决定必须是“立即报案”（否则属于该报不报）；
//   - 决定“暂不报案/继续观察”时，必须写明理由、后续措施与复核时间。
//
// 返回空串表示通过。该检查在基础版与实战版都硬拦截，避免“不评估直接不报案”
// 或“该报不报”仍被记为正确完成；被拦截的节点按未完成处理，结束时扣该节点分值。
func validateReportAssessment(formID string, data map[string]any) string {
	if strings.TrimSpace(formID) != "F19" {
		return ""
	}
	standard := formValueString(data["是否达到报案标准"])
	decision := formValueString(data["处置决定"])
	if standard == "达到" && decision != "立即报案" {
		return "评估认定已达到报案标准时，处置决定必须选择「立即报案」"
	}
	if decision == "暂不报案" || decision == "继续观察" {
		if formValueString(data["暂不报案理由与后续措施"]) == "" {
			return "选择「暂不报案」或「继续观察」时，必须填写暂不报案理由与后续措施"
		}
		if formValueString(data["后续复核时间"]) == "" {
			return "选择「暂不报案」或「继续观察」时，必须填写后续复核时间"
		}
	}
	return ""
}

// PostTaskForms POST /api/tasks/:id/forms
func PostTaskForms(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := guardTaskWrite(database, c)
		if !ok {
			return
		}
		id := c.Param("id")
		u := t.User
		// 先取原始字节做 UTF-8 合法性校验：非法字节（如 GBK 页面/异常客户端直传）
		// 写入库会变成 U+FFFD，无法还原，这里直接 400 拒绝。
		raw, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
			return
		}
		if !utf8.Valid(raw) {
			userName := ""
			if u != nil {
				userName = u.Name
			}
			slog.Warn("表单提交请求体非法 UTF-8，已拒绝", "task", id, "user", userName, "bytes", len(raw))
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是合法的 UTF-8 编码，请检查提交内容后重试"})
			return
		}
		var body struct {
			FormID string         `json:"formId"`
			Data   map[string]any `json:"data"`
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "请求体 JSON 解析失败"})
				return
			}
		}
		var formName, fieldsJSON string
		if database.QueryRow(`SELECT name,COALESCE(fields,'') FROM form_templates WHERE id=?`, body.FormID).Scan(&formName, &fieldsJSON) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "表单模板不存在"})
			return
		}
		// 下拉类字段必须落在模板 options 内（空值、未配置选项的历史模板、多选数组均兼容）
		if err := validateSelectOptions(fieldsJSON, body.Data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// F19 报案必要性评估单：跨字段一致性硬校验（基础版同样拦截）
		if msg := validateReportAssessment(body.FormID, body.Data); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		pid := id
		nodeID := ""
		_ = database.QueryRow(`SELECT COALESCE(project_id,''),COALESCE(node_id,'') FROM tasks WHERE id=?`, id).Scan(&pid, &nodeID)

		// 表单校验：必填项 + 节点文字质量规则。
		// 基础版（诱导演练）只提示不阻断：照常落库，问题通过 warnings 返回；
		// 正式版（实战版）保持 400 + reasons 硬拦截。
		mode := ""
		_ = database.QueryRow(`SELECT COALESCE(mode,'') FROM projects WHERE id=?`, pid).Scan(&mode)
		isBase := strings.TrimSpace(mode) == "基础版"
		warnings, reasons := validateFormSubmission(database, nodeID, fieldsJSON, body.Data)
		if len(reasons) > 0 && !isBase {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "表单校验未通过：" + strings.Join(reasons, "；"),
				"reasons": reasons,
			})
			return
		}

		// 表单落库与业务影响联动放在同一事务：任一失败整体回滚，
		// 避免出现「表单已提交但大屏/看板状态没变」的假成功。
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()

		dataJSON := util.ToJSON(body.Data)
		now := util.Now()
		res, err := tx.Exec(
			`INSERT INTO form_submissions (task_id,form_id,form_name,data,submitter,user_id,role_id,time) VALUES (?,?,?,?,?,?,?,?)`,
			id, body.FormID, formName, dataJSON, u.Name, u.ID, u.Role.ID, now,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "表单保存失败"})
			return
		}
		var sid int64
		if res != nil {
			sid, _ = res.LastInsertId()
		}
		changed := []string{}
		if err := applyFormImpactTx(tx, ImpactSource{
			ProjectID: pid, TaskID: id, NodeID: nodeID,
			FormID: body.FormID, FormName: formName,
			SubmissionID: sid, Operator: u.Name, RoleID: u.Role.ID, At: now,
		}, body.FormID, body.Data); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "业务影响状态更新失败"})
			return
		} else {
			changed = collectImpactSystems(body.FormID, body.Data)
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "表单提交失败"})
			return
		}
		if warnings == nil {
			warnings = []formWarning{}
		}
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "submissionId": sid,
			"impactUpdated": len(changed) > 0, "systems": changed,
			// 基础版只提示不阻断：这些问题不影响本次提交，前端以提示条展示
			"warnings": warnings,
		})
	}
}

// validateSelectOptions 校验下拉类字段的提交值必须命中模板 options。
// 兼容：空值放行；模板里不存在该字段（历史模板）放行；非 select/multiselect 放行；
// 未配置 options 的历史模板放行；multiselect 允许「、，,;；」分隔的字符串或 JSON 数组。
// 只做提交校验，不参与历史数据的只读回显（GetTaskForms 不受影响）。
func validateSelectOptions(fieldsJSON string, data map[string]any) error {
	if strings.TrimSpace(fieldsJSON) == "" || len(data) == 0 {
		return nil
	}
	var raw []any
	if err := json.Unmarshal([]byte(fieldsJSON), &raw); err != nil {
		// 模板结构异常时不阻断业务提交，交由后台模板编辑器修正
		return nil
	}
	order := []string{}
	types := map[string]string{}
	opts := map[string]map[string]bool{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label, _ := m["label"].(string)
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		tp, _ := m["type"].(string)
		tp = strings.ToLower(strings.TrimSpace(tp))
		if tp != "select" && tp != "multiselect" {
			continue
		}
		set := map[string]bool{}
		if arr, ok := m["options"].([]any); ok {
			for _, o := range arr {
				s, ok := o.(string)
				if !ok {
					continue
				}
				if s = strings.TrimSpace(s); s != "" {
					set[s] = true
				}
			}
		}
		if len(set) == 0 {
			continue
		}
		order = append(order, label)
		types[label] = tp
		opts[label] = set
	}
	for _, label := range order {
		v, ok := data[label]
		if !ok || v == nil {
			continue
		}
		var vals []string
		switch vv := v.(type) {
		case string:
			s := strings.TrimSpace(vv)
			if s == "" {
				continue // 空值走「未填写」语义，保持与旧行为一致
			}
			if types[label] == "multiselect" {
				for _, p := range strings.FieldsFunc(s, func(r rune) bool {
					return r == '、' || r == ',' || r == '，' || r == ';' || r == '；'
				}) {
					if p = strings.TrimSpace(p); p != "" {
						vals = append(vals, p)
					}
				}
			} else {
				vals = []string{s}
			}
		case []any:
			for _, e := range vv {
				s, ok := e.(string)
				if !ok {
					continue
				}
				if s = strings.TrimSpace(s); s != "" {
					vals = append(vals, s)
				}
			}
		default:
			continue // 非字符串值（数字/布尔）不做下拉校验
		}
		for _, got := range vals {
			if !opts[label][got] {
				return fmt.Errorf("字段「%s」的值「%s」不在下拉选项内，请从候选项中选择", label, got)
			}
		}
	}
	return nil
}

// formWarning 表单校验问题（逐字段）。基础版仅提示，正式版转成 reasons 拦截。
type formWarning struct {
	Field   string `json:"field"`
	Label   string `json:"label"`
	Message string `json:"message"`
}

// validateFormSubmission 表单提交校验（与实战版 PostDrillDecide 的提交硬门槛对齐）：
//   - 表单模板中 required 字段必须非空；
//   - 节点配置了文字质量规则时，对表单内自由文本套用同一硬门槛（未实质填写 / 纯敷衍 / 有效字数过低）；
//   - 全下拉、全数字表单不参与文字质量判定；混淆节点不拦截（与实战版一致）。
//
// 只做提交校验，不影响历史提交的只读回显。
// 返回值：warnings 为逐字段提示（基础版展示用），reasons 为正式版 400 拦截文案（保持原有汇总口径）。
func validateFormSubmission(database *db.DB, nodeID, fieldsJSON string, data map[string]any) ([]formWarning, []string) {
	warnings := []formWarning{}
	reasons := []string{}
	if missing := missingRequiredFields(util.ToJSON(data), parseFormRequiredFields(fieldsJSON)); len(missing) > 0 {
		for _, f := range missing {
			warnings = append(warnings, formWarning{
				Field:   f.Key,
				Label:   f.Label,
				Message: "必填项未填写：" + f.Display(),
			})
		}
		reasons = append(reasons, "表单必填项缺失："+joinFieldDisplays(missing))
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || len(data) == 0 {
		return warnings, reasons
	}
	var qualityRaw, flowType, keywordsRaw string
	if err := database.QueryRow(
		`SELECT COALESCE(quality_rules,''),COALESCE(flow_type,'correct'),COALESCE(keywords,'') FROM script_nodes WHERE id=? AND deleted_at IS NULL`,
		nodeID,
	).Scan(&qualityRaw, &flowType, &keywordsRaw); err != nil {
		return warnings, reasons
	}
	// 仅对显式配置了质量规则的节点做文字门槛，避免把默认规则套到普通备注字段。
	if strings.TrimSpace(qualityRaw) == "" {
		return warnings, reasons
	}
	texts := make([]string, 0, len(data))
	for _, v := range data {
		if s, ok := v.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				texts = append(texts, s)
			}
		}
	}
	if len(texts) == 0 {
		return warnings, reasons
	}
	combined := strings.Join(texts, "\n")
	keywords := make([]string, 0, 4)
	for _, kw := range strings.Split(keywordsRaw, ",") {
		if kw = strings.TrimSpace(kw); kw != "" {
			keywords = append(keywords, kw)
		}
	}
	q := submissionQuality{
		HasSubmission: true,
		Text:          combined,
		Measure:       combined,
		Keywords:      keywords,
	}
	if gate := evaluateHardGate(q, parseQualityRules(qualityRaw), flowType); len(gate) > 0 {
		for _, g := range gate {
			warnings = append(warnings, formWarning{Label: "文字质量", Message: g})
		}
		reasons = append(reasons, gate...)
	}
	return warnings, reasons
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
//
// 数据流：multipart/截图 base64 → uploads/evidence/YYYYMM/ 落盘 → evidence 表
// （project_id / task_id / node_id / req_key / file_path 全量绑定）→
// 工作台与取证页共用 queryEvidence 返回的同一份记录与 /uploads 地址。
// =========================================================================

type evidenceRequirement struct {
	Type string
	Name string
	Note string
}

// parseEvidenceReqs 兼容对象数组与旧版字符串数组两种 evidence_req 存储格式。
func parseEvidenceReqs(raw string) []evidenceRequirement {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var objs []map[string]any
	if err := json.Unmarshal([]byte(raw), &objs); err == nil {
		out := make([]evidenceRequirement, 0, len(objs))
		for _, m := range objs {
			it := evidenceRequirement{}
			if v, ok := m["type"].(string); ok {
				it.Type = strings.TrimSpace(v)
			}
			if v, ok := m["name"].(string); ok {
				it.Name = strings.TrimSpace(v)
			}
			if v, ok := m["note"].(string); ok {
				it.Note = strings.TrimSpace(v)
			}
			out = append(out, it)
		}
		return out
	}
	var strs []string
	if err := json.Unmarshal([]byte(raw), &strs); err == nil {
		out := make([]evidenceRequirement, 0, len(strs))
		for _, s := range strs {
			out = append(out, evidenceRequirement{Name: strings.TrimSpace(s)})
		}
		return out
	}
	return nil
}

// evidenceBinding 是取证记录的三重绑定：任务、项目、剧本节点。
type evidenceBinding struct {
	ProjectID string
	TaskID    string
	NodeID    string
	RoleID    string
}

// resolveEvidenceBinding 复用任务附件的权限口径：
//   - 任务必须存在、项目未归档、非管理角色只能操作本角色任务；
//   - 节点必须存在且与项目剧本一致；未显式给项目时按节点所属剧本回填。
func resolveEvidenceBinding(database *db.DB, u *auth.User, taskID, projectID, nodeID string) (evidenceBinding, string) {
	b := evidenceBinding{
		ProjectID: strings.TrimSpace(projectID),
		TaskID:    strings.TrimSpace(taskID),
		NodeID:    strings.TrimSpace(nodeID),
	}
	if b.TaskID != "" {
		var taskProject, taskRole, taskNode, projectStatus string
		err := database.QueryRow(
			`SELECT COALESCE(t.project_id,''),COALESCE(t.role_id,''),COALESCE(t.node_id,''),COALESCE(p.status,'')
			 FROM tasks t LEFT JOIN projects p ON p.id=t.project_id WHERE t.id=?`, b.TaskID,
		).Scan(&taskProject, &taskRole, &taskNode, &projectStatus)
		if err != nil {
			return b, "任务不存在"
		}
		if projectStatus == "已归档" {
			return b, "已归档项目的任务不可再上传取证材料"
		}
		if !canManageTask(database, u, taskRole) {
			return b, "该任务由「" + taskRole + "」负责，当前角色不可代为上传取证材料"
		}
		if b.ProjectID == "" {
			b.ProjectID = taskProject
		}
		if b.NodeID == "" {
			b.NodeID = taskNode
		}
		b.RoleID = taskRole
	}
	if b.ProjectID != "" {
		var status string
		if err := database.QueryRow(`SELECT COALESCE(status,'') FROM projects WHERE id=?`, b.ProjectID).Scan(&status); err != nil {
			return b, "演练项目不存在"
		}
		if status == "已归档" {
			return b, "已归档项目不可再上传取证材料"
		}
	}
	if b.NodeID != "" {
		var nodeScenario, nodeTarget string
		if err := database.QueryRow(`SELECT COALESCE(scenario_id,''),COALESCE(target,'') FROM script_nodes WHERE id=? AND deleted_at IS NULL`, b.NodeID).Scan(&nodeScenario, &nodeTarget); err != nil {
			return b, "剧本节点不存在"
		}
		// 仅绑定节点时按节点归属角色校验；任务绑定已在上面的 canManageTask 覆盖
		if b.TaskID == "" && !canHandleRole(database, u.Role.ID, nodeTarget) {
			return b, "该剧本节点由其他角色负责，当前角色不可上传"
		}
		if b.ProjectID != "" {
			var projectScenario string
			if err := database.QueryRow(`SELECT COALESCE(scenario_id,'') FROM projects WHERE id=?`, b.ProjectID).Scan(&projectScenario); err != nil {
				return b, "演练项目不存在"
			}
			if nodeScenario != "" && projectScenario != "" && nodeScenario != projectScenario {
				return b, "剧本节点与演练项目不匹配"
			}
		} else if nodeScenario != "" {
			// 只给节点时按该节点所属剧本回填一个项目，保证列表按项目可检索
			var pid string
			if err := database.QueryRow(`SELECT id FROM projects WHERE scenario_id=? ORDER BY COALESCE(date,'') DESC,id DESC LIMIT 1`, nodeScenario).Scan(&pid); err == nil {
				b.ProjectID = pid
			}
		}
	}
	if b.RoleID == "" {
		b.RoleID = u.Role.ID
	}
	return b, ""
}

func nullableText(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// evidenceChainJSON 生成保管链初始事件（JSON 数组）。
func evidenceChainJSON(actor, action, at, note string) string {
	ev := []gin.H{{
		"at":     at,
		"actor":  actor,
		"action": action,
		"note":   truncateText(note, 200),
	}}
	b, _ := json.Marshal(ev)
	return string(b)
}

// insertEvidenceRecord 写入证据记录；sha 为空表示无实体文件（预填/模拟/元数据），
// 有 sha 则视为上传事件，同时写入保管人、封存时间与保管链（P1-07）。
func insertEvidenceRecord(tx *sql.Tx, u *auth.User, b evidenceBinding, id, typ, name, size, rel, note, reqKey string, auto int, sha string) error {
	url := ""
	if strings.TrimSpace(rel) != "" {
		url = "/uploads/" + rel
	}
	at := util.Now()
	action := "归档"
	if strings.TrimSpace(sha) != "" {
		action = "上传"
	}
	_, err := tx.Exec(
		`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,note,node_id,req_key,created_at,sha256,custodian,sealed_at,chain_of_custody)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, truncateText(typ, 32), truncateText(name, 120), truncateText(size, 16), util.HHMM(), b.RoleID, u.Name, u.ID, auto,
		nullableText(b.TaskID), nullableText(b.ProjectID), nullableText(url), nullableText(rel),
		nullableText(truncateText(note, 500)), nullableText(b.NodeID), nullableText(truncateText(reqKey, 128)), at,
		nullableText(strings.TrimSpace(sha)), u.Name, at, evidenceChainJSON(u.Name, action, at, note),
	)
	return err
}

func insertEvidenceAttachmentMirror(tx *sql.Tx, u *auth.User, taskID, name, typ, size, rel string) error {
	if taskID == "" || rel == "" {
		return nil
	}
	_, err := tx.Exec(
		`INSERT INTO task_attachments (task_id,name,type,size,url,file_path,uploader,uploader_id,time,auto)
		 VALUES (?,?,?,?,?,?,?,?,?,0)`,
		taskID, name, typ, size, "/uploads/"+rel, rel, u.Name, u.ID, util.HHMM(),
	)
	return err
}

func formField(form *multipart.Form, key string) string {
	if form == nil {
		return ""
	}
	if v := form.Value[key]; len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
}

func uploadMaxMB() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("UPLOAD_MAX_MB"))); err == nil && n > 0 {
		return n
	}
	return 20
}

func imageExtByMagic(b []byte) (string, bool) {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return ".png", true
	case len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff:
		return ".jpg", true
	case len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a"))):
		return ".gif", true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return ".webp", true
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return ".bmp", true
	}
	return "", false
}

// saveEvidenceDataURL 保存抓屏 base64 图片，按真实文件头决定扩展名。
func saveEvidenceDataURL(uploadDir, dataURL, origName string, maxMB int) (rel, displayName, size string, err error) {
	const prefix = "data:image/"
	if !strings.HasPrefix(strings.ToLower(dataURL), prefix) {
		return "", "", "", fmt.Errorf("截图数据格式不正确")
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return "", "", "", fmt.Errorf("截图数据缺少 base64 内容")
	}
	meta := dataURL[len(prefix):comma]
	parts := strings.Split(meta, ";")
	isBase64 := false
	for _, p := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(p), "base64") {
			isBase64 = true
		}
	}
	if !isBase64 {
		return "", "", "", fmt.Errorf("截图数据不是 base64 编码")
	}
	raw, decErr := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if decErr != nil {
		raw, decErr = base64.RawStdEncoding.DecodeString(dataURL[comma+1:])
	}
	if decErr != nil {
		return "", "", "", fmt.Errorf("截图 base64 解码失败")
	}
	if len(raw) == 0 {
		return "", "", "", fmt.Errorf("截图内容为空")
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	if maxBytes > 0 && int64(len(raw)) > maxBytes {
		return "", "", "", fmt.Errorf("截图超过 %dMB 限制", maxMB)
	}
	ext, ok := imageExtByMagic(raw)
	if !ok {
		return "", "", "", fmt.Errorf("仅支持 PNG/JPEG/GIF/WEBP/BMP 截图")
	}
	base := strings.TrimSuffix(sanitizeFileName(origName), filepath.Ext(origName))
	if strings.TrimSpace(base) == "" || base == "." {
		base = "屏幕截图"
	}
	displayName = base + ext

	subDir := filepath.Join("evidence", time.Now().Format("200601"))
	targetDir := filepath.Join(uploadDir, subDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", "", err
	}
	storedName := fmt.Sprintf("%d-%s-%s", time.Now().UnixMilli(), randHex(4), displayName)
	full := filepath.Join(targetDir, storedName)
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		return "", "", "", err
	}
	return filepath.ToSlash(filepath.Join(subDir, storedName)), displayName, fmtSizeGo(int64(len(raw))), nil
}

// GetEvidenceLegacy GET /api/evidence
func GetEvidenceLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, queryEvidence(database, auth.CurrentUser(c))) }
}

// PostEvidenceLegacy POST /api/evidence
// 兼容旧前端：dataUrl 抓屏入库；无 dataUrl 时保留纯记录语义。
func PostEvidenceLegacy(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可上传取证材料"})
			return
		}
		var body struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Size      string `json:"size"`
			Note      string `json:"note"`
			DataURL   string `json:"dataUrl"`
			ReqKey    string `json:"reqKey"`
			TaskID    string `json:"taskId"`
			ProjectID string `json:"projectId"`
			NodeID    string `json:"nodeId"`
			Auto      bool   `json:"auto"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
			return
		}
		b, msg := resolveEvidenceBinding(database, u, body.TaskID, body.ProjectID, body.NodeID)
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		typ := strings.TrimSpace(body.Type)
		if typ == "" {
			typ = "文档"
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			name = "未命名材料"
		}
		size := strings.TrimSpace(body.Size)
		rel := ""
		if strings.TrimSpace(body.DataURL) != "" {
			uploadDir := resolveUploadRootDir()
			savedRel, savedName, savedSize, err := saveEvidenceDataURL(uploadDir, body.DataURL, name, uploadMaxMB())
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			rel, name, size = savedRel, savedName, savedSize
			if strings.TrimSpace(body.Type) == "" {
				typ = "附件截图"
			}
		}

		id := fmt.Sprintf("E%d_%s", time.Now().UnixMilli(), randHex(3))
		tx, err := database.Begin()
		if err != nil {
			if rel != "" {
				removeUploadFile(resolveUploadRootDir(), rel)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务开启失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()
		auto := 0
		if body.Auto {
			auto = 1
		}
		if err := insertEvidenceRecord(tx, u, b, id, typ, name, size, rel, strings.TrimSpace(body.Note), body.ReqKey, auto, ""); err != nil {
			if rel != "" {
				removeUploadFile(resolveUploadRootDir(), rel)
			}
			slog.Warn("取证记录写入失败", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取证材料保存失败"})
			return
		}
		if err := insertEvidenceAttachmentMirror(tx, u, b.TaskID, name, typ, size, rel); err != nil {
			if rel != "" {
				removeUploadFile(resolveUploadRootDir(), rel)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "附件记录保存失败"})
			return
		}
		if err := tx.Commit(); err != nil {
			if rel != "" {
				removeUploadFile(resolveUploadRootDir(), rel)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取证材料提交失败"})
			return
		}

		url := ""
		if rel != "" {
			url = "/uploads/" + rel
		}
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "id": id, "name": name, "type": typ, "size": size,
			"url": url, "hasFile": rel != "", "projectId": b.ProjectID,
			"taskId": b.TaskID, "nodeId": b.NodeID, "reqKey": body.ReqKey,
		})
	}
}

// PostEvidencePrefill POST /api/evidence/prefill
// 按「节点#序号」把本角色负责的剧本取证要求登记为已归档材料。
func PostEvidencePrefill(database *db.DB) gin.HandlerFunc {
	type prefillItem struct {
		NodeID string `json:"nodeId"`
		Index  int    `json:"index"`
		Name   string `json:"name"`
		Note   string `json:"note"`
	}
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可预填取证材料"})
			return
		}
		var body struct {
			ProjectID string        `json:"projectId"`
			Items     []prefillItem `json:"items"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
			return
		}
		pid := strings.TrimSpace(body.ProjectID)
		if pid == "" {
			pid = resolveActiveProjectID(database)
		}
		if pid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未指定演练项目"})
			return
		}
		var scenario, status string
		if err := database.QueryRow(`SELECT COALESCE(scenario_id,''),COALESCE(status,'') FROM projects WHERE id=?`, pid).Scan(&scenario, &status); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status == "已归档" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "已归档项目不可再预填取证材料"})
			return
		}

		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务开启失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()

		created := 0
		skipped := []string{}
		seen := map[string]bool{}
		for _, it := range body.Items {
			nid := strings.TrimSpace(it.NodeID)
			if nid == "" {
				skipped = append(skipped, "缺少节点")
				continue
			}
			var target, nodeScenario, rawReq string
			if err := tx.QueryRow(
				`SELECT COALESCE(target,''),COALESCE(scenario_id,''),COALESCE(evidence_req,'[]')
				 FROM script_nodes WHERE id=? AND deleted_at IS NULL`, nid,
			).Scan(&target, &nodeScenario, &rawReq); err != nil {
				skipped = append(skipped, nid+"：节点不存在")
				continue
			}
			if scenario != "" && nodeScenario != "" && scenario != nodeScenario {
				skipped = append(skipped, nid+"：不属于当前项目剧本")
				continue
			}
			reqs := parseEvidenceReqs(rawReq)
			if it.Index < 0 || it.Index >= len(reqs) {
				skipped = append(skipped, fmt.Sprintf("%s#%d：取证要求不存在", nid, it.Index))
				continue
			}
			key := fmt.Sprintf("%s#%d", nid, it.Index)
			if seen[key] {
				continue
			}
			seen[key] = true
			if !canHandleRole(database, u.Role.ID, target) {
				skipped = append(skipped, key+"：非本角色负责")
				continue
			}
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM evidence WHERE project_id=? AND req_key=?`, pid, key).Scan(&exists); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "校验取证状态失败"})
				return
			}
			if exists > 0 {
				skipped = append(skipped, key+"：已归档")
				continue
			}
			req := reqs[it.Index]
			name := strings.TrimSpace(it.Name)
			if name == "" {
				name = req.Name
			}
			if name == "" {
				name = "取证材料"
			}
			typ := req.Type
			if typ == "" {
				typ = "文档"
			}
			id := fmt.Sprintf("E%d_%s", time.Now().UnixMilli(), randHex(3))
			b := evidenceBinding{ProjectID: pid, NodeID: nid, RoleID: u.Role.ID}
			if err := insertEvidenceRecord(tx, u, b, id, typ, name, "-", "", strings.TrimSpace(it.Note), key, 1, ""); err != nil {
				slog.Warn("预填取证材料写入失败", "err", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "预填材料保存失败"})
				return
			}
			created++
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "预填材料提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "count": created, "skipped": skipped, "projectId": pid})
	}
}

// PostEvidenceMock POST /api/evidence/mock
// 一键导入与剧本要求相符的模拟材料（仅登记记录，不伪造物理文件）。
func PostEvidenceMock(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可导入模拟材料"})
			return
		}
		var body struct {
			NodeID    string `json:"nodeId"`
			Index     int    `json:"index"`
			ProjectID string `json:"projectId"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
			return
		}
		nid := strings.TrimSpace(body.NodeID)
		if nid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少节点 ID"})
			return
		}
		pid := strings.TrimSpace(body.ProjectID)
		if pid == "" {
			pid = resolveActiveProjectID(database)
		}
		if pid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未指定演练项目"})
			return
		}
		var target, nodeScenario, rawReq, status string
		if err := database.QueryRow(
			`SELECT COALESCE(target,''),COALESCE(scenario_id,''),COALESCE(evidence_req,'[]') FROM script_nodes WHERE id=? AND deleted_at IS NULL`, nid,
		).Scan(&target, &nodeScenario, &rawReq); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "剧本节点不存在"})
			return
		}
		if err := database.QueryRow(`SELECT COALESCE(status,'') FROM projects WHERE id=?`, pid).Scan(&status); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status == "已归档" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "已归档项目不可再导入模拟材料"})
			return
		}
		reqs := parseEvidenceReqs(rawReq)
		if body.Index < 0 || body.Index >= len(reqs) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该节点没有对应的取证要求"})
			return
		}
		if !canHandleRole(database, u.Role.ID, target) {
			c.JSON(http.StatusForbidden, gin.H{"error": "该取证要求不属于当前角色"})
			return
		}
		key := fmt.Sprintf("%s#%d", nid, body.Index)
		req := reqs[body.Index]
		name := req.Name
		if name == "" {
			name = "模拟材料"
		}
		typ := req.Type
		if typ == "" {
			typ = "文档"
		}
		var exists int
		if err := database.QueryRow(`SELECT COUNT(*) FROM evidence WHERE project_id=? AND req_key=?`, pid, key).Scan(&exists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "校验取证状态失败"})
			return
		}
		if exists > 0 {
			c.JSON(http.StatusOK, gin.H{"ok": true, "deduped": true, "name": name, "key": key, "projectId": pid})
			return
		}
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务开启失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()
		id := fmt.Sprintf("E%d_%s", time.Now().UnixMilli(), randHex(3))
		b := evidenceBinding{ProjectID: pid, NodeID: nid, RoleID: u.Role.ID}
		note := "演练模拟材料（按剧本预填，请按实际处置核对修改）"
		if err := insertEvidenceRecord(tx, u, b, id, typ, name, "-", "", note, key, 1, ""); err != nil {
			slog.Warn("模拟取证材料写入失败", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "模拟材料保存失败"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "模拟材料提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id, "name": name, "key": key, "projectId": pid})
	}
}

// GetEvidenceRequirements GET /api/evidence/requirements
// 按项目剧本节点 + evidence_req 返回逐项归档状态；mine 表示当前角色是否负责该节点。
func GetEvidenceRequirements(database *db.DB) gin.HandlerFunc {
	empty := func() gin.H {
		return gin.H{
			"project": nil, "items": []gin.H{}, "total": 0, "done": 0, "pending": 0,
			"mineTotal": 0, "mineDone": 0, "minePending": 0,
		}
	}
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		pid := strings.TrimSpace(c.Query("projectId"))
		if pid == "" {
			pid = resolveActiveProjectID(database)
		}
		if pid == "" {
			c.JSON(http.StatusOK, empty())
			return
		}
		var pName, scenario, status string
		if err := database.QueryRow(
			`SELECT COALESCE(name,''),COALESCE(scenario_id,''),COALESCE(status,'') FROM projects WHERE id=?`, pid,
		).Scan(&pName, &scenario, &status); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}

		type doneInfo struct {
			ID, Time string
			HasFile  bool
		}
		doneMap := map[string]doneInfo{}
		if rows, err := database.Query(
			`SELECT COALESCE(req_key,''),id,COALESCE(time,''),CASE WHEN COALESCE(file_path,'')<>'' THEN 1 ELSE 0 END
			 FROM evidence WHERE project_id=? AND req_key IS NOT NULL AND req_key<>''
			 ORDER BY created_at DESC,id DESC`, pid,
		); err == nil {
			for rows.Next() {
				var key, id, tm string
				var hasFile int
				if rows.Scan(&key, &id, &tm, &hasFile) != nil {
					continue
				}
				if _, ok := doneMap[key]; !ok {
					doneMap[key] = doneInfo{ID: id, Time: tm, HasFile: hasFile == 1}
				}
			}
			rows.Close()
		}

		items := []gin.H{}
		total, done, mineTotal, mineDone, minePending := 0, 0, 0, 0, 0
		rows, err := database.Query(
			`SELECT id,COALESCE(title,''),COALESCE(target,''),COALESCE(evidence_req,'[]'),COALESCE(flow_type,'correct')
			 FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND type!='end' ORDER BY seq,id`, scenario,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取剧本节点失败"})
			return
		}
		defer rows.Close()
		for rows.Next() {
			var nid, title, target, rawReq, flowType string
			if rows.Scan(&nid, &title, &target, &rawReq, &flowType) != nil {
				continue
			}
			if strings.EqualFold(flowType, "distractor") {
				continue
			}
			reqs := parseEvidenceReqs(rawReq)
			for i, req := range reqs {
				key := fmt.Sprintf("%s#%d", nid, i)
				typ := req.Type
				if typ == "" {
					typ = "文档"
				}
				mine := false
				if u != nil && !u.Role.External {
					mine = canHandleRole(database, u.Role.ID, target)
				}
				hit, isDone := doneMap[key]
				total++
				if isDone {
					done++
				}
				if mine {
					mineTotal++
					if isDone {
						mineDone++
					} else {
						minePending++
					}
				}
				items = append(items, gin.H{
					"key": key, "nodeId": nid, "nodeTitle": title, "target": target,
					"index": i, "type": typ, "name": req.Name, "note": req.Note,
					"mine": mine, "done": isDone,
					"evidenceId": hit.ID, "doneAt": hit.Time, "hasFile": hit.HasFile,
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"project": gin.H{"id": pid, "name": pName, "status": status},
			"items":   items, "total": total, "done": done, "pending": total - done,
			"mineTotal": mineTotal, "mineDone": mineDone, "minePending": minePending,
		})
	}
}

// PostEvidenceUpload POST /api/evidence/upload
func PostEvidenceUpload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可上传取证材料"})
			return
		}
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		files := form.File["files"]
		if len(files) == 0 {
			files = form.File["file"]
		}
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到文件"})
			return
		}
		if len(files) > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "单次最多上传 10 个文件"})
			return
		}

		taskID := formField(form, "taskId")
		projectID := formField(form, "projectId")
		nodeID := formField(form, "nodeId")
		reqKey := formField(form, "reqKey")
		note := formField(form, "note")
		typ := formField(form, "type")
		b, msg := resolveEvidenceBinding(database, u, taskID, projectID, nodeID)
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		maxMB := uploadMaxMB()
		maxBytes := int64(maxMB) * 1024 * 1024
		for _, fh := range files {
			if fh.Size <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "文件内容为空：" + sanitizeFileName(fh.Filename)})
				return
			}
			if maxBytes > 0 && fh.Size > maxBytes {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("文件「%s」超过 %dMB 限制", sanitizeFileName(fh.Filename), maxMB)})
				return
			}
			if dangerousAttExts[strings.ToLower(filepath.Ext(fh.Filename))] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "不允许上传可执行程序或脚本：" + sanitizeFileName(fh.Filename)})
				return
			}
		}

		type savedFile struct {
			rel, full, name, typ, size, id, sha string
		}
		uploadDir := resolveUploadRootDir()
		saved := make([]savedFile, 0, len(files))
		cleanup := func() {
			for _, s := range saved {
				_ = os.Remove(s.full)
			}
		}
		for _, fh := range files {
			rel, full, err := saveUploadedFile(uploadDir, "evidence", fh.Filename, fh)
			if err != nil {
				cleanup()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败：" + err.Error()})
				return
			}
			fileType := typ
			if fileType == "" {
				fileType = guessAttType(fh.Filename)
			}
			saved = append(saved, savedFile{
				rel: rel, full: full, name: sanitizeFileName(fh.Filename),
				typ: fileType, size: fmtSizeGo(fh.Size), sha: fileSHA256(full),
			})
		}

		tx, err := database.Begin()
		if err != nil {
			cleanup()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务开启失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()
		out := make([]gin.H, 0, len(saved))
		for i := range saved {
			id := fmt.Sprintf("E%d_%s", time.Now().UnixMilli(), randHex(3))
			saved[i].id = id
			if err := insertEvidenceRecord(tx, u, b, id, saved[i].typ, saved[i].name, saved[i].size, saved[i].rel, note, reqKey, 0, saved[i].sha); err != nil {
				cleanup()
				slog.Warn("取证材料写入失败", "err", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "取证材料保存失败"})
				return
			}
			if err := insertEvidenceAttachmentMirror(tx, u, b.TaskID, saved[i].name, saved[i].typ, saved[i].size, saved[i].rel); err != nil {
				cleanup()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "附件记录保存失败"})
				return
			}
			out = append(out, gin.H{
				"id": id, "name": saved[i].name, "type": saved[i].typ, "size": saved[i].size,
				"url": "/uploads/" + saved[i].rel, "projectId": b.ProjectID,
				"taskId": b.TaskID, "nodeId": b.NodeID, "reqKey": reqKey,
			})
		}
		if err := tx.Commit(); err != nil {
			cleanup()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取证材料提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "files": out, "count": len(out),
			"projectId": b.ProjectID, "taskId": b.TaskID, "nodeId": b.NodeID, "reqKey": reqKey,
		})
	}
}

// DeleteEvidence DELETE /api/evidence/:id —— 删除取证材料（含物理文件）
// 权限口径与任务附件一致：上传者本人或指挥部/信息科负责人/平台管理员。
func DeleteEvidence(database *db.DB, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可删除取证材料"})
			return
		}
		id := c.Param("id")
		var taskID, projectID, filePath string
		var userID int64
		if err := database.QueryRow(
			`SELECT COALESCE(task_id,''),COALESCE(project_id,''),COALESCE(file_path,''),COALESCE(user_id,0)
			 FROM evidence WHERE id=?`, id,
		).Scan(&taskID, &projectID, &filePath, &userID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "取证材料不存在"})
			return
		}
		allowed := false
		switch u.Role.ID {
		case "platformadmin", "commander", "itlead":
			allowed = true
		default:
			allowed = userID == u.ID
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅上传者本人或指挥部/信息科负责人/平台管理员可删除取证材料"})
			return
		}
		// 与任务附件共用同一物理文件时，先移除附件记录，避免留下悬空附件
		if filePath != "" && taskID != "" {
			_, _ = database.Exec(`DELETE FROM task_attachments WHERE task_id=? AND file_path=?`, taskID, filePath)
		}
		if _, err := database.Exec(`DELETE FROM evidence WHERE id=?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "取证材料删除失败"})
			return
		}
		// 仅当没有其他证据/附件引用同一物理文件时才删盘
		var remains int
		_ = database.QueryRow(`SELECT COUNT(*) FROM evidence WHERE file_path=?`, filePath).Scan(&remains)
		var attRemains int
		_ = database.QueryRow(`SELECT COUNT(*) FROM task_attachments WHERE file_path=?`, filePath).Scan(&attRemains)
		if filePath != "" && remains == 0 && attRemains == 0 {
			removeUploadFile(uploadDir, filePath)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "id": id, "projectId": projectID})
	}
}

// GetEvidenceDownload GET /api/evidence/:id/download —— 返回真实文件字节流
func GetEvidenceDownload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var name, filePath string
		err := database.QueryRow(`SELECT COALESCE(name,''),COALESCE(file_path,'') FROM evidence WHERE id=?`, id).Scan(&name, &filePath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "取证材料不存在"})
			return
		}
		if strings.TrimSpace(filePath) == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "该取证材料没有实体文件"})
			return
		}
		uploadDir := resolveUploadRootDir()
		clean := filepath.Clean(filepath.FromSlash(filePath))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件路径不合法"})
			return
		}
		full := filepath.Join(uploadDir, clean)
		if rel, err := filepath.Rel(uploadDir, full); err != nil || strings.HasPrefix(rel, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件路径不合法"})
			return
		}
		st, err := os.Stat(full)
		if err != nil || st.IsDir() {
			c.JSON(http.StatusNotFound, gin.H{"error": "取证文件不存在或已被清理"})
			return
		}
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(full)
		}
		c.FileAttachment(full, sanitizeFileName(name))
	}
}

// =========================================================================
// /api/admin —— 缺失的 report、restore-upload
// =========================================================================

// AdminRestoreUpload POST /api/admin/restore-upload
// 上传 .sql 备份并真正执行恢复：校验 → 恢复前自动备份 → 导入 → 校验 → 审计。
func AdminRestoreUpload(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil || file == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未接收到备份文件"})
			return
		}
		if !strings.EqualFold(filepath.Ext(file.Filename), ".sql") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 .sql 备份文件（系统导出的 mysqldump 文本）；旧 .db 文件不再支持"})
			return
		}
		const maxRestoreBytes = int64(2) * 1024 * 1024 * 1024
		if file.Size <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "备份文件为空"})
			return
		}
		if file.Size > maxRestoreBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "备份文件超过 2GB 限制"})
			return
		}

		backupDir := resolveBackupDir()
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建备份目录失败：" + err.Error()})
			return
		}
		name := fmt.Sprintf("drill-upload-%s-%s.sql", time.Now().Format("20060102-150405"), randHex(3))
		target := filepath.Join(backupDir, name)

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败：" + err.Error()})
			return
		}
		defer src.Close()
		dst, err := os.Create(target)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存上传备份失败：" + err.Error()})
			return
		}
		written, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(target)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存上传备份失败"})
			return
		}
		if written <= 0 {
			_ = os.Remove(target)
			c.JSON(http.StatusBadRequest, gin.H{"error": "备份文件为空"})
			return
		}
		if err := validateSQLDump(target); err != nil {
			_ = os.Remove(target)
			c.JSON(http.StatusBadRequest, gin.H{"error": "备份文件校验失败：" + err.Error()})
			return
		}

		data, failure := performRestore(database, c, dbConnFromEnv(), backupDir, target, name)
		if failure != nil {
			c.JSON(failure.Code, gin.H{"error": failure.Msg})
			return
		}
		c.JSON(http.StatusOK, data)
	}
}

// sha256HexRe 匹配 64 位十六进制摘要（F13 申报值与归档文件摘要都按此提取）。
var sha256HexRe = regexp.MustCompile(`(?i)[a-f0-9]{64}`)

// extractSHA256Values 从任意文本/JSON 中提取去重后的 SHA-256 摘要（小写）。
func extractSHA256Values(raw string) []string {
	out := make([]string, 0, 2)
	seen := map[string]bool{}
	for _, m := range sha256HexRe.FindAllString(raw, -1) {
		h := strings.ToLower(m)
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// evidenceHashCheck 比对 F13 申报哈希与 evidence 归档文件实际 SHA-256（P1-07）。
// 结果只用于报告展示，不参与扣分；未申报/无归档时给出明确状态而不是报错。
func evidenceHashCheck(database *db.DB, projectID string) gin.H {
	declared := make([]string, 0, 4)
	declaredSet := map[string]bool{}
	collect := func(raw string) {
		for _, h := range extractSHA256Values(raw) {
			if !declaredSet[h] {
				declaredSet[h] = true
				declared = append(declared, h)
			}
		}
	}
	// 基础版：任务表单提交（F13）
	if rows, err := database.Query(`SELECT COALESCE(fs.data,'') FROM form_submissions fs
		JOIN tasks t ON t.id=fs.task_id WHERE t.project_id=? AND fs.form_id='F13'`, projectID); err == nil {
		for rows.Next() {
			var d string
			if rows.Scan(&d) == nil {
				collect(d)
			}
		}
		rows.Close()
	}
	// 实战版：处置记录里的表单 JSON
	if rows, err := database.Query(`SELECT COALESCE(forms,'') FROM drill_steps
		WHERE project_id=? AND COALESCE(forms,'')<>''`, projectID); err == nil {
		for rows.Next() {
			var d string
			if rows.Scan(&d) == nil {
				collect(d)
			}
		}
		rows.Close()
	}

	type evHash struct{ id, name, sha string }
	evs := make([]evHash, 0, 4)
	if rows, err := database.Query(`SELECT id,COALESCE(name,''),LOWER(COALESCE(sha256,'')) FROM evidence
		WHERE project_id=? AND COALESCE(sha256,'')<>'' ORDER BY created_at,id`, projectID); err == nil {
		for rows.Next() {
			var e evHash
			if rows.Scan(&e.id, &e.name, &e.sha) == nil {
				evs = append(evs, e)
			}
		}
		rows.Close()
	}
	actualSet := map[string]bool{}
	for _, e := range evs {
		actualSet[e.sha] = true
	}
	matched := 0
	for _, h := range declared {
		if actualSet[h] {
			matched++
		}
	}
	status := "一致"
	switch {
	case len(declared) == 0:
		status = "未申报"
	case matched == 0:
		status = "不一致"
	case matched < len(declared):
		status = "部分一致"
	}
	items := make([]gin.H, 0, len(evs))
	for _, e := range evs {
		items = append(items, gin.H{"evidenceId": e.id, "name": e.name, "sha256": e.sha, "matched": declaredSet[e.sha]})
	}
	return gin.H{
		"status": status, "declared": len(declared), "archived": len(evs),
		"matched": matched, "mismatched": len(declared) - matched, "items": items,
	}
}

// AdminReport GET /api/admin/report/:projectId
// 演练报告数据聚合：按项目汇总概况、剧本、任务、时序、实战决策、取证、评分与台账。
// 结构与旧 Node 版 src/routes/admin.js 对齐，前端 report.js 直接消费。
func AdminReport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := strings.TrimSpace(c.Param("projectId"))
		var (
			pID, pName, pType, pDate, pLevel, pStatus, pTarget, pScenario, pMode string
			pUnits, pMembers, pObservers                                         string
		)
		err := database.QueryRow(`SELECT id,COALESCE(name,''),COALESCE(type,''),COALESCE(date,''),COALESCE(level,''),
			COALESCE(status,''),COALESCE(target,''),COALESCE(scenario_id,''),COALESCE(mode,'基础版'),
			COALESCE(units,'[]'),COALESCE(members,'[]'),COALESCE(observers,'[]')
			FROM projects WHERE id=?`, pid).Scan(&pID, &pName, &pType, &pDate, &pLevel, &pStatus, &pTarget,
			&pScenario, &pMode, &pUnits, &pMembers, &pObservers)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取演练项目失败"})
			return
		}

		// 角色名称映射
		roleNames := map[string]string{}
		if rows, e := database.Query(`SELECT id,COALESCE(name,'') FROM roles`); e == nil {
			for rows.Next() {
				var rid, rname string
				if rows.Scan(&rid, &rname) == nil {
					roleNames[rid] = rname
				}
			}
			rows.Close()
		}
		roleName := func(rid string) string {
			if n, ok := roleNames[rid]; ok && n != "" {
				return n
			}
			return rid
		}

		// 剧本与节点元数据（时限、标题、责任角色）
		var scenario gin.H
		nodeTotal := 0
		nodeMeta := map[string]struct{ Title, Target string }{}
		durationMap := map[string]int{}
		if pScenario != "" {
			var sID, sName, sType, sLevel, sVersion, sDescr string
			if e := database.QueryRow(`SELECT id,COALESCE(name,''),COALESCE(type,''),COALESCE(level,''),
				COALESCE(version,''),COALESCE(descr,'') FROM scenarios WHERE id=?`, pScenario).
				Scan(&sID, &sName, &sType, &sLevel, &sVersion, &sDescr); e == nil {
				scenario = gin.H{"id": sID, "name": sName, "type": sType, "level": sLevel,
					"version": sVersion, "descr": sDescr}
			}
			if rows, e := database.Query(`SELECT id,COALESCE(title,''),COALESCE(target,''),COALESCE(timeout,0),COALESCE(flow_type,'correct')
				FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL`, pScenario); e == nil {
				for rows.Next() {
					var nid, title, target, flowType string
					var timeout int
					if rows.Scan(&nid, &title, &target, &timeout, &flowType) == nil {
						nodeMeta[nid] = struct{ Title, Target string }{title, target}
						if timeout > 0 {
							durationMap[nid] = timeout
						}
						// 报告展示口径：混淆节点不计入剧本节点数（仍保留元数据供错选明细使用）
						if !strings.EqualFold(strings.TrimSpace(flowType), "distractor") {
							nodeTotal++
						}
					}
				}
				rows.Close()
			}
			if scenario != nil {
				scenario["nodes"] = nodeTotal
			}
		}

		// 任务 + 分角色统计
		tasks := []gin.H{}
		type roleAgg struct {
			Role                                 string
			Total, Done, Doing, Pending, Overdue int
		}
		byRoleMap := map[string]*roleAgg{}
		byRoleOrder := []string{}
		total, done, doing, pending, overdue := 0, 0, 0, 0, 0
		if rows, e := database.Query(`SELECT id,COALESCE(title,''),COALESCE(descr,''),COALESCE(role_id,''),
			COALESCE(level,''),COALESCE(deadline,''),COALESCE(node_id,''),COALESCE(status,'')
			FROM tasks WHERE project_id=? ORDER BY seq,id`, pid); e == nil {
			for rows.Next() {
				var tid, title, descr, roleID, level, deadline, nodeID, status string
				if rows.Scan(&tid, &title, &descr, &roleID, &level, &deadline, &nodeID, &status) != nil {
					continue
				}
				rname := roleName(roleID)
				var duration any
				if d, ok := durationMap[nodeID]; ok {
					duration = d
				}
				tasks = append(tasks, gin.H{"id": tid, "title": title, "descr": descr, "role": roleID,
					"roleName": rname, "level": level, "deadline": deadline, "duration": duration, "status": status})
				total++
				agg := byRoleMap[rname]
				if agg == nil {
					agg = &roleAgg{Role: rname}
					byRoleMap[rname] = agg
					byRoleOrder = append(byRoleOrder, rname)
				}
				agg.Total++
				switch status {
				case "已完成":
					done++
					agg.Done++
				case "处理中":
					doing++
					agg.Doing++
				case "超时":
					overdue++
					agg.Overdue++
				default:
					pending++
					agg.Pending++
				}
			}
			rows.Close()
		}
		byRole := []gin.H{}
		for _, k := range byRoleOrder {
			a := byRoleMap[k]
			byRole = append(byRole, gin.H{"role": a.Role, "total": a.Total, "done": a.Done,
				"doing": a.Doing, "pending": a.Pending, "overdue": a.Overdue})
		}
		rate := 0
		if total > 0 {
			rate = int(math.Round(float64(done) / float64(total) * 100))
		}

		// 时序台账
		timeline := []gin.H{}
		if rows, e := database.Query(`SELECT COALESCE(t,''),COALESCE(actor,''),COALESCE(role_id,''),
			COALESCE(action,''),COALESCE(detail,''),COALESCE(level,'') FROM timeline WHERE project_id=? ORDER BY seq,id`, pid); e == nil {
			for rows.Next() {
				var t, actor, roleID, action, detail, level string
				if rows.Scan(&t, &actor, &roleID, &action, &detail, &level) == nil {
					timeline = append(timeline, gin.H{"t": t, "actor": actor, "role": roleName(roleID),
						"action": action, "detail": detail, "level": level})
				}
			}
			rows.Close()
		}

		// 处置时序图
		seqFlow := []gin.H{}
		if rows, e := database.Query(`SELECT COALESCE(name,''),COALESCE(time,''),COALESCE(role,''),
			COALESCE(state,'') FROM seq_flow WHERE project_id=? ORDER BY seq,id`, pid); e == nil {
			for rows.Next() {
				var name, tm, role, state string
				if rows.Scan(&name, &tm, &role, &state) == nil {
					seqFlow = append(seqFlow, gin.H{"name": name, "time": tm, "role": role, "state": state})
				}
			}
			rows.Close()
		}

		// 取证材料
		evidence := []gin.H{}
		evByType := map[string]int{}
		autoEv := 0
		if rows, e := database.Query(`SELECT id,COALESCE(type,''),COALESCE(name,''),COALESCE(size,''),
			COALESCE(time,''),COALESCE(role_id,''),COALESCE(task_id,''),COALESCE(auto,0),
			COALESCE(node_id,''),COALESCE(req_key,''),COALESCE(sha256,''),COALESCE(custodian,''),COALESCE(sealed_at,'')
			FROM evidence WHERE project_id=? OR task_id IN (SELECT id FROM tasks WHERE project_id=?)
			ORDER BY created_at,id`, pid, pid); e == nil {
			for rows.Next() {
				var eid, etype, name, size, tm, roleID, taskID, nodeID, reqKey, sha, custodian, sealedAt string
				var auto int
				if rows.Scan(&eid, &etype, &name, &size, &tm, &roleID, &taskID, &auto,
					&nodeID, &reqKey, &sha, &custodian, &sealedAt) != nil {
					continue
				}
				isAuto := auto == 1
				evidence = append(evidence, gin.H{"id": eid, "type": etype, "name": name, "size": size,
					"time": tm, "role": roleID, "taskId": taskID, "auto": isAuto,
					"nodeId": nodeID, "reqKey": reqKey, "sha256": sha,
					"custodian": custodian, "sealedAt": sealedAt})
				evByType[etype]++
				if isAuto {
					autoEv++
				}
			}
			rows.Close()
		}

		// 实战版处置决策记录
		decisions := []gin.H{}
		dTotal, dCorrect, dWrong, dOntime := 0, 0, 0, 0
		if rows, e := database.Query(`SELECT COALESCE(matched_node,''),COALESCE(title,''),COALESCE(choice,''),
			COALESCE(correct,1),COALESCE(ontime,0),COALESCE(step_time,''),COALESCE(found_at,''),
			COALESCE(found,''),COALESCE(measure,''),COALESCE(result,''),COALESCE(user,'')
			FROM drill_steps WHERE project_id=? ORDER BY step_time,id`, pid); e == nil {
			for rows.Next() {
				var nodeID, title, choice, stepTime, foundAt, found, measure, result, user string
				var correct, ontime int
				if rows.Scan(&nodeID, &title, &choice, &correct, &ontime, &stepTime, &foundAt,
					&found, &measure, &result, &user) != nil {
					continue
				}
				meta := nodeMeta[nodeID]
				dTitle := title
				if meta.Title != "" {
					dTitle = meta.Title
				}
				dCorrectFlag := correct != 0
				decisions = append(decisions, gin.H{"nodeId": nodeID, "title": dTitle, "target": meta.Target,
					"choice": choice, "correct": dCorrectFlag, "ontime": ontime == 1, "stepTime": stepTime,
					"foundAt": foundAt, "found": found, "measure": measure, "result": result, "user": user})
				dTotal++
				if dCorrectFlag {
					dCorrect++
				} else {
					dWrong++
				}
				if ontime == 1 {
					dOntime++
				}
			}
			rows.Close()
		}
		baseline := 0
		if pScenario != "" {
			_ = database.QueryRow(`SELECT COUNT(*) FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND required=1 AND type<>'end' AND COALESCE(flow_type,'correct')!='distractor'`,
				pScenario).Scan(&baseline)
		}

		// 考评明细（K1-K9）+ MODE 模式化自动考评
		type scoreRow struct {
			Score sql.NullFloat64
			Max   sql.NullInt64
			Note  string
		}
		scoreMap := map[string]scoreRow{}
		if rows, e := database.Query(`SELECT item_id,score,max,COALESCE(note,'') FROM scores WHERE project_id=?`, pid); e == nil {
			for rows.Next() {
				var itemID string
				var sr scoreRow
				if rows.Scan(&itemID, &sr.Score, &sr.Max, &sr.Note) == nil {
					scoreMap[itemID] = sr
				}
			}
			rows.Close()
		}
		scores := []gin.H{}
		scoreSum, scoreMax := 0.0, 0
		graded := 0
		if rows, e := database.Query(`SELECT id,COALESCE(name,''),COALESCE(weight,0),COALESCE(rule,'')
			FROM score_items WHERE id<>'MODE' ORDER BY seq,id`); e == nil {
			for rows.Next() {
				var itemID, name, rule string
				var weight int
				if rows.Scan(&itemID, &name, &weight, &rule) != nil {
					continue
				}
				max := weight
				var sc any = nil
				note := ""
				if sr, ok := scoreMap[itemID]; ok {
					if sr.Max.Valid {
						max = int(sr.Max.Int64)
					}
					if sr.Score.Valid {
						sc = sr.Score.Float64
						scoreSum += sr.Score.Float64
						graded++
					}
					note = sr.Note
				}
				scoreMax += max
				scores = append(scores, gin.H{"itemId": itemID, "name": name, "weight": weight,
					"rule": rule, "max": max, "score": sc, "note": note})
			}
			rows.Close()
		}
		var scorePct any
		if graded > 0 && scoreMax > 0 {
			scorePct = int(math.Round(scoreSum / float64(scoreMax) * 100))
		}
		var modeScore any
		if sr, ok := scoreMap["MODE"]; ok {
			modeScore = gin.H{"score": sr.Score.Float64, "max": sr.Max.Int64, "note": sr.Note}
		}

		// 台账
		var ledger any
		var lID, lName, lDate, lType, lEvent, lLevel, lResult, lScore, lOrg string
		if e := database.QueryRow(`SELECT id,COALESCE(name,''),COALESCE(date,''),COALESCE(type,''),
			COALESCE(event,''),COALESCE(level,''),COALESCE(result,''),COALESCE(score,''),COALESCE(org,'')
			FROM ledger WHERE project_id=? LIMIT 1`, pid).
			Scan(&lID, &lName, &lDate, &lType, &lEvent, &lLevel, &lResult, &lScore, &lOrg); e == nil {
			ledger = gin.H{"id": lID, "name": lName, "date": lDate, "type": lType, "event": lEvent,
				"level": lLevel, "result": lResult, "score": lScore, "org": lOrg}
		}

		generatedBy := "平台管理员"
		if u := auth.CurrentUser(c); u != nil && u.Name != "" {
			generatedBy = u.Name
		}

		c.JSON(http.StatusOK, gin.H{
			"project": gin.H{
				"id": pID, "name": pName, "type": pType, "date": pDate, "level": pLevel,
				"status": pStatus, "target": pTarget, "scenarioId": pScenario, "mode": pMode,
				"units": rptJSONArray(pUnits), "members": rptJSONArray(pMembers), "observers": rptJSONArray(pObservers),
			},
			"scenario": scenario,
			"stats": gin.H{"total": total, "done": done, "doing": doing, "pending": pending,
				"overdue": overdue, "rate": rate},
			"byRole": byRole,
			"tasks":  tasks, "timeline": timeline, "seqFlow": seqFlow,
			"decisions": decisions,
			"decisionStats": gin.H{"baseline": baseline, "total": dTotal, "correct": dCorrect,
				"wrong": dWrong, "ontime": dOntime},
			"evidence":          evidence,
			"evidenceStats":     gin.H{"total": len(evidence), "auto": autoEv, "byType": evByType},
			"evidenceHashCheck": evidenceHashCheck(database, pid),
			"scores":            scores,
			"scoreStats": gin.H{"sum": scoreSum, "max": scoreMax, "pct": scorePct,
				"graded": graded, "items": len(scores)},
			"modeScore":   modeScore,
			"ledger":      ledger,
			"generatedAt": time.Now().Format("2006-01-02 15:04:05"),
			"generatedBy": generatedBy,
		})
	}
}

// AdminReportExport POST /api/admin/report/:projectId/export
// 校验项目存在后写入审计日志（仅追加），返回导出时间。
func AdminReportExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := strings.TrimSpace(c.Param("projectId"))
		var name string
		err := database.QueryRow(`SELECT COALESCE(name,'') FROM projects WHERE id=?`, pid).Scan(&name)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取演练项目失败"})
			return
		}
		var body struct {
			Format string `json:"format"`
		}
		_ = c.ShouldBindJSON(&body)
		fmtName := strings.TrimSpace(body.Format)
		if fmtName == "" {
			fmtName = "HTML"
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		u := auth.CurrentUser(c)
		if u != nil {
			var uid any
			if u.ID > 0 {
				uid = u.ID
			}
			_, _ = database.Exec(`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
				now, u.Name, u.Role.ID, uid, "导出演练报告《"+name+"》（"+fmtName+"）", c.ClientIP())
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "time": now})
	}
}

// rptJSONArray 将库内 JSON 文本解析为数组；空值/非法值返回空数组，避免前端拿到 null。
func rptJSONArray(s string) []any {
	out := []any{}
	if strings.TrimSpace(s) != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	if out == nil {
		out = []any{}
	}
	return out
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
			 FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL ORDER BY seq`, p.ScenarioID)
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
			"score":      displayScore(database, p.ID),
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

// formModeOf 返回表单的交互模式：fill（填写）/ review（审议）。
// 旧库无 mode 列时返回空串，前端按 fill 处理。
func formModeOf(database *db.DB, formID string) string {
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return ""
	}
	var mode sql.NullString
	if err := database.QueryRow(`SELECT COALESCE(mode,'') FROM form_templates WHERE id=?`, formID).Scan(&mode); err != nil {
		return ""
	}
	return util.ScanNullableString(mode)
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
		// 外部单位改为只读详情入口：可查看被指派节点的内容与表单，但不可提交
		// （提交/跳过/结束等写入入口仍对 u.Role.External 返回 403，由院应急指挥部代执行）。
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
				"score":         displayScore(database, p.ID),
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
				"formName": formName, "fields": fields, "formMode": formModeOf(database, n.Form),
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
			"score":         displayScore(database, p.ID),
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
