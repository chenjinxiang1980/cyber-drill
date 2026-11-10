package handler

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// anonNameMasker 匿名大屏姓名脱敏：把已知用户姓名替换为其角色名。
// 只处理姓名（>=2 字），不改变时间、动作、状态等时间线信息。
func anonNameMasker(database *db.DB) func(string) string {
	type pair struct{ name, role string }
	var pairs []pair
	rows, err := database.Query(
		`SELECT COALESCE(u.name,''),COALESCE(r.name,'参演人员')
		 FROM users u LEFT JOIN roles r ON r.id=u.role_id
		 WHERE u.name IS NOT NULL AND u.name<>''`)
	if err != nil {
		return func(s string) string { return s }
	}
	for rows.Next() {
		var name, role string
		if rows.Scan(&name, &role) == nil {
			pairs = append(pairs, pair{name: name, role: role})
		}
	}
	rows.Close()
	// 长姓名优先，避免“张三丰”被“张三”部分替换
	sort.Slice(pairs, func(i, j int) bool {
		return len([]rune(pairs[i].name)) > len([]rune(pairs[j].name))
	})
	return func(s string) string {
		if s == "" {
			return s
		}
		for _, p := range pairs {
			if len([]rune(p.name)) < 2 {
				continue
			}
			s = strings.ReplaceAll(s, p.name, p.role)
		}
		return s
	}
}

// ScreenNotifsHandler GET /api/screen/notifs?projectId=
// 大屏语音播报/操作流专用：只返回当前项目的站内消息，不返回全局公告（project_id 为空）。
// 口径与登录用户的通知中心一致：包含 target='internal' 的内部可见消息（服务端生成的站内消息均为此值），
// 否则真实演练数据会被全部过滤掉，大屏既无法展示也无法播报。
// 已登录时按该用户已读状态；未登录（观摩）按“任一用户已读”判定，避免大屏重复播报已被处理的告警。
// 仅暴露播报展示所需字段，不包含账号、配置、审计、令牌等敏感数据。
func ScreenNotifsHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := strings.TrimSpace(c.Query("projectId"))
		if pid == "" {
			pid = currentProjectID(database)
		}
		if pid == "" {
			c.JSON(http.StatusOK, gin.H{"serverTime": nowSec(), "projectId": "", "notifs": []gin.H{}})
			return
		}

		// 已读集合：登录态按当前用户；匿名按任一用户已读（大屏为共享只读视图）
		readSet := map[string]bool{}
		readSQL := `SELECT DISTINCT notif_id FROM notif_reads`
		var args []any
		current := auth.CurrentUser(c)
		if current != nil {
			readSQL = `SELECT notif_id FROM notif_reads WHERE user_id=?`
			args = append(args, current.ID)
		}
		var anonMask func(string) string
		if current == nil {
			anonMask = anonNameMasker(database)
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
			 WHERE project_id=? ORDER BY seq DESC LIMIT 50`, pid)
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
			src := util.ScanNullableString(source)
			titleText := util.ScanNullableString(title)
			descrText := util.ScanNullableString(descr)
			if anonMask != nil {
				src = anonMask(src)
				titleText = anonMask(titleText)
				descrText = anonMask(descrText)
			}
			out = append(out, gin.H{
				"id":        nid,
				"time":      util.ScanNullableString(tm),
				"level":     util.ScanNullableString(level),
				"source":    src,
				"title":     titleText,
				"desc":      descrText,
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
		_ = database.QueryRow(`SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND COALESCE(flow_type,'correct')!='distractor'`).Scan(&nodes)
		_ = database.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projs)
		c.JSON(http.StatusOK, gin.H{"scenarios": scens, "scriptNodes": nodes, "projects": projs})
	}
}
