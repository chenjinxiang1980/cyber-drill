package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// --- content 基础只读接口（走 bootstrap 里的 queryXxx 函数）---
//
// 这些 handler 直接调用 bootstrap.go 里已实现的 query 函数返回。
// 路由层已经挂 auth.RequireAuth()，handler 内可用 auth.CurrentUser(c)。
// Node 版这些 GET 都直接返回数组/对象，不包 {ok,data}。

// GetScenarios GET /api/content/scenarios
func GetScenarios(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list := queryScenarios(database)
		if isExternalUser(auth.CurrentUser(c)) {
			list = externalScenarios(list)
		}
		c.JSON(200, list)
	}
}

// GetScriptNodes GET /api/content/script-nodes
func GetScriptNodes(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryScriptNodes(database))
	}
}

// GetProjects GET /api/content/projects
func GetProjects(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		list := queryProjects(database, "")
		c.JSON(200, filterProjectsForUser(list, u, activeProjectID(database)))
	}
}

// GetMessages GET /api/content/messages
func GetMessages(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExternalUser(auth.CurrentUser(c)) {
			c.JSON(200, []gin.H{})
			return
		}
		c.JSON(200, queryMessages(database))
	}
}

// GetNotifs GET /api/content/notifs
func GetNotifs(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryNotifs(database, auth.CurrentUser(c)))
	}
}

// GetDict GET /api/content/dict
func GetDict(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryDictMap(database))
	}
}

// GetSimData GET /api/content/sim-data
func GetSimData(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, querySimDataMap(database))
	}
}

// GetKnowledge GET /api/content/knowledge
func GetKnowledge(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, knowledgeForUser(database, auth.CurrentUser(c)))
	}
}

// GetTimeline GET /api/content/timeline
func GetTimeline(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryTimeline(database, auth.CurrentUser(c)))
	}
}

// GetLedger GET /api/content/ledger
func GetLedger(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, ledgerForUser(database, auth.CurrentUser(c)))
	}
}

// GetOpinion GET /api/content/opinion
func GetOpinion(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExternalUser(auth.CurrentUser(c)) {
			c.JSON(200, []gin.H{})
			return
		}
		c.JSON(200, queryOpinion(database))
	}
}

// GetImpact GET /api/content/impact
func GetImpact(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExternalUser(auth.CurrentUser(c)) {
			c.JSON(200, []gin.H{})
			return
		}
		c.JSON(200, queryImpact(database, resolveActiveProjectID(database)))
	}
}

// GetSnapshots GET /api/content/snapshots
func GetSnapshots(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExternalUser(auth.CurrentUser(c)) {
			c.JSON(200, []gin.H{})
			return
		}
		c.JSON(200, querySnapshots(database))
	}
}

// GetSeqFlow GET /api/content/seqflow
func GetSeqFlow(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isExternalUser(auth.CurrentUser(c)) {
			c.JSON(200, []gin.H{})
			return
		}
		c.JSON(200, querySeqFlow(database))
	}
}

// GetScenarioRoles GET /api/content/scenarios/:id/roles
// 返回剧本涉及的角色（来自 script_nodes.target → roles 解析）及各角色的账号数量。
// 仅新增只读接口，不改动项目创建/角色选择既有契约，也不触碰剧本数据与评分。
func GetScenarioRoles(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		scenarioID := strings.TrimSpace(c.Param("id"))
		if scenarioID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "剧本编号不能为空"})
			return
		}

		var exists int
		if err := database.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE id=?`, scenarioID).Scan(&exists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "剧本查询失败"})
			return
		}
		if exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "剧本不存在：" + scenarioID})
			return
		}

		type roleInfo struct {
			id       string
			name     string
			short    string
			external bool
		}
		roles := make([]roleInfo, 0)
		roleByKey := make(map[string]string)
		roleByID := make(map[string]roleInfo)
		rows, err := database.Query("SELECT id,COALESCE(name,''),COALESCE(short,''),COALESCE(`external`,0) FROM roles ORDER BY seq")
		if err != nil {
			slog.Warn("GetScenarioRoles 查询角色失败", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "角色查询失败"})
			return
		}
		for rows.Next() {
			var r roleInfo
			var ext int
			if err := rows.Scan(&r.id, &r.name, &r.short, &ext); err != nil {
				continue
			}
			r.external = ext == 1
			roles = append(roles, r)
			roleByID[r.id] = r
			if name := strings.TrimSpace(r.name); name != "" {
				roleByKey[name] = r.id
			}
			if short := strings.TrimSpace(r.short); short != "" {
				roleByKey[short] = r.id
			}
		}
		rows.Close()

		// 目标角色：script_nodes.target（所有未删除节点，包含混淆/结束节点，符合“剧本涉及”口径）
		targetRows, err := database.Query(`SELECT DISTINCT target FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND TRIM(COALESCE(target,''))<>''`, scenarioID)
		if err != nil {
			slog.Warn("GetScenarioRoles 查询剧本目标角色失败", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "剧本角色查询失败"})
			return
		}
		involved := make(map[string]bool)
		for targetRows.Next() {
			var target string
			if err := targetRows.Scan(&target); err != nil {
				continue
			}
			roleID := resolveRoleFromMap(roleByKey, target)
			if _, ok := roleByID[roleID]; ok {
				involved[roleID] = true
			}
		}
		targetRows.Close()

		// 账号数量：仅统计启用状态账号；users.role_id 主角色 + user_roles 兼岗，按 (user_id, role_id) 去重
		accountCount := make(map[string]int)
		accRows, err := database.Query(`SELECT role_id,COUNT(DISTINCT user_id) FROM (
			SELECT u.id AS user_id,u.role_id AS role_id FROM users u WHERE u.status='启用'
			UNION
			SELECT ur.user_id,ur.role_id FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE u.status='启用'
		) t GROUP BY role_id`)
		if err != nil {
			slog.Warn("GetScenarioRoles 查询角色账号失败", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "角色账号查询失败"})
			return
		}
		for accRows.Next() {
			var roleID string
			var count int
			if err := accRows.Scan(&roleID, &count); err == nil {
				accountCount[roleID] = count
			}
		}
		accRows.Close()

		roleList := make([]gin.H, 0, len(roles))
		involvedRoleIDs := make([]string, 0)
		missingRoles := make([]gin.H, 0)
		missingNames := make([]string, 0)
		for _, r := range roles {
			count := accountCount[r.id]
			isInvolved := involved[r.id]
			roleList = append(roleList, gin.H{
				"id":           r.id,
				"name":         r.name,
				"short":        r.short,
				"external":     r.external,
				"accountCount": count,
				"involved":     isInvolved,
			})
			if !isInvolved {
				continue
			}
			involvedRoleIDs = append(involvedRoleIDs, r.id)
			if count == 0 {
				missingRoles = append(missingRoles, gin.H{"id": r.id, "name": r.name})
				missingNames = append(missingNames, "「"+r.name+"」")
			}
		}

		missingMessage := ""
		if len(missingNames) == 1 {
			missingMessage = "角色" + missingNames[0] + "暂无账号，请先添加账号。"
		} else if len(missingNames) > 1 {
			missingMessage = "以下角色暂无账号，请先添加账号：" + strings.Join(missingNames, "、") + "。"
		}

		c.JSON(http.StatusOK, gin.H{
			"scenarioId":      scenarioID,
			"roles":           roleList,
			"involvedRoleIds": involvedRoleIDs,
			"missingRoles":    missingRoles,
			"missingMessage":  missingMessage,
		})
	}
}
