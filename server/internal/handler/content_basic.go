package handler

import (
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
		c.JSON(200, queryScenarios(database))
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
		c.JSON(200, queryProjects(database, ""))
	}
}

// GetMessages GET /api/content/messages
func GetMessages(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
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
		c.JSON(200, queryOpinion(database))
	}
}

// GetImpact GET /api/content/impact
func GetImpact(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, queryImpact(database, resolveActiveProjectID(database)))
	}
}

// GetSnapshots GET /api/content/snapshots
func GetSnapshots(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, querySnapshots(database))
	}
}

// GetSeqFlow GET /api/content/seqflow
func GetSeqFlow(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, querySeqFlow(database))
	}
}
