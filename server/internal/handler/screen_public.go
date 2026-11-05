package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ScreenNotifsHandler GET /api/screen/notifs?projectId=
// 大屏语音播报专用：只返回当前项目的站内消息，不返回全局公告（project_id 为空）。
// 已登录时按该用户已读状态；未登录（观摩）按“任一用户已读”判定，避免大屏重复播报已被处理的告警。
// 仅暴露播报展示所需字段，不包含账号、配置、审计、令牌等敏感数据。
func ScreenNotifsHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := strings.TrimSpace(c.Query("projectId"))
		if pid == "" {
			var activeID sql.NullString
			_ = database.QueryRow(`SELECT id FROM projects WHERE status != '未开始' LIMIT 1`).Scan(&activeID)
			if activeID.Valid {
				pid = activeID.String
			}
		}
		if pid == "" {
			c.JSON(http.StatusOK, gin.H{"serverTime": nowSec(), "projectId": "", "notifs": []gin.H{}})
			return
		}

		// 已读集合：登录态按当前用户；匿名按任一用户已读（大屏为共享只读视图）
		readSet := map[string]bool{}
		readSQL := `SELECT DISTINCT notif_id FROM notif_reads`
		var args []any
		if u := auth.CurrentUser(c); u != nil {
			readSQL = `SELECT notif_id FROM notif_reads WHERE user_id=?`
			args = append(args, u.ID)
		}
		if rr, err := database.Query(readSQL, args...); err == nil {
			for rr.Next() {
				var nid sql.NullString
				if rr.Scan(&nid) == nil {
					readSet[util.ScanNullableString(nid)] = true
				}
			}
			rr.Close()
		}

		rows, err := database.Query(
			`SELECT id,time,level,source,title,descr,project_id FROM notifs
			 WHERE target!='internal' AND project_id=? ORDER BY seq DESC LIMIT 50`, pid)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"serverTime": nowSec(), "projectId": pid, "notifs": []gin.H{}})
			return
		}
		out := make([]gin.H, 0)
		for rows.Next() {
			var id, tm, level, source, title, descr, projectID sql.NullString
			if err := rows.Scan(&id, &tm, &level, &source, &title, &descr, &projectID); err != nil {
				continue
			}
			nid := util.ScanNullableString(id)
			out = append(out, gin.H{
				"id":        nid,
				"time":      util.ScanNullableString(tm),
				"level":     util.ScanNullableString(level),
				"source":    util.ScanNullableString(source),
				"title":     util.ScanNullableString(title),
				"desc":      util.ScanNullableString(descr),
				"projectId": util.ScanNullableString(projectID),
				"read":      readSet[nid],
			})
		}
		rows.Close()
		c.JSON(http.StatusOK, gin.H{"serverTime": nowSec(), "projectId": pid, "notifs": out})
	}
}

// ScreenSummaryHandler GET /api/screen/summary
// 待机页只需要的三个计数，不返回剧本内容/项目明细。
func ScreenSummaryHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var scens, nodes, projs int
		_ = database.QueryRow(`SELECT COUNT(*) FROM scenarios`).Scan(&scens)
		_ = database.QueryRow(`SELECT COUNT(*) FROM script_nodes`).Scan(&nodes)
		_ = database.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projs)
		c.JSON(http.StatusOK, gin.H{"scenarios": scens, "scriptNodes": nodes, "projects": projs})
	}
}
