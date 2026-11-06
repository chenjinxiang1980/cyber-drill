// Package handler 放各业务模块的 HTTP handler。
package handler

import (
	"drill-system/internal/db"
	"drill-system/internal/util"
	"drill-system/internal/version"

	"github.com/gin-gonic/gin"
)

// Health 健康检查。在 Node 版 /api/health 字段基础上增加 version：
// {"ok":true,"service":"cyber-drill-system","version":"0.6.0","time":"YYYY-MM-DD HH:mm"}
// 此接口无需鉴权。
func Health(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"ok":      true,
			"service": "cyber-drill-system",
			"version": version.Version,
			"time":    util.NowSec(),
		})
	}
}
