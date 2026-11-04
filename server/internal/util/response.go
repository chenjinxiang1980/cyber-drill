// Package util 提供 HTTP 响应辅助与错误类型。
package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Err 统一错误响应体（与前端 api.js 期望的格式一致）
type Err struct {
	Error      string `json:"error"`
	NeedReason bool   `json:"needReason,omitempty"` // 混淆节点跳过时必填原因
}

// OK 200 通用成功响应（`{"ok":true, ...}`）
func OK(c *gin.Context, data gin.H) {
	if data == nil {
		data = gin.H{"ok": true}
	} else if _, exists := data["ok"]; !exists {
		data["ok"] = true
	}
	c.JSON(http.StatusOK, data)
}

// Fail 通用失败响应
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, Err{Error: msg})
}

// BadRequest 400
func BadRequest(c *gin.Context, msg string) {
	Fail(c, http.StatusBadRequest, msg)
}

// Unauthorized 401 —— 未认证 / 令牌过期
func Unauthorized(c *gin.Context, msg string) {
	Fail(c, http.StatusUnauthorized, msg)
}

// Forbidden 403 —— 已认证但权限不足
func Forbidden(c *gin.Context, msg string) {
	Fail(c, http.StatusForbidden, msg)
}

// NotFound 404
func NotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, msg)
}

// Conflict 409
func Conflict(c *gin.Context, msg string) {
	Fail(c, http.StatusConflict, msg)
}
