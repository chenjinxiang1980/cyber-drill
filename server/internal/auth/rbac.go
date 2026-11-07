// Package auth RBAC 权限矩阵 + 便捷中间件。
//
// 从 Node 版 src/auth.js + 8 个 routes/*.js 提取的权限模式，
// 每个中间件的错误文案必须与 Node 版完全一致（前端 api.js 靠中文文案做判断）。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---- 12 个真实角色常量（以 roles 表为准，硬编码在 Go 里，避免运行时查库）----
// 与数据库实际 12 条记录一致：
//
//	院内：platformadmin, commander, itlead, duty, netsec, sysadmin, medlead, clinical
//	院外：healthbureau, cac, cyberpolice
//	自定义：role_mvnaewi63am  (用户创建的自定义角色，external=false)
const (
	RolePlatformAdmin = "platformadmin" // 平台管理员
	RoleCommander     = "commander"     // 院应急指挥部
	RoleITLead        = "itlead"        // 信息科负责人
	RoleDuty          = "duty"          // 信息科值班
	RoleNetSec        = "netsec"        // 网络安全
	RoleSysAdmin      = "sysadmin"      // 系统管理员
	RoleMedLead       = "medlead"       // 医务科负责人
	RoleClinical      = "clinical"      // 临床科室
	RoleHealthBureau  = "healthbureau"  // 卫健局（外部）
	RoleCAC           = "cac"           // CAC/网信办（外部）
	RoleCyberPolice   = "cyberpolice"   // 公安网安（外部）
)

// IsExternal 外部单位角色判定（3 个院外单位）
func IsExternal(roleID string) bool {
	switch roleID {
	case RoleHealthBureau, RoleCAC, RoleCyberPolice:
		return true
	}
	return false
}

// IsAdmin 平台系统管理权限
func IsAdmin(roleID string) bool {
	return roleID == RolePlatformAdmin
}

// IsCommander 指挥部角色
func IsCommander(roleID string) bool {
	return roleID == RoleCommander
}

// IsPlatformBoard "院级统筹角色"：platformadmin / commander / itlead 三者，
// 剧本维护、项目新建、演练结束、考评等都要这个权限
func IsPlatformBoard(roleID string) bool {
	switch roleID {
	case RolePlatformAdmin, RoleCommander, RoleITLead:
		return true
	}
	return false
}

// ---- 便捷中间件 ----

// RequireAdmin 仅 platformadmin 可访问系统管理（与 Node src/routes/admin.js adminOnly 完全对齐）。
//
// 说明：之前尝试过放开 platformadmin+commander+itlead 进 admin 组并让 itlead 管理 itlead/duty/netsec/sysadmin，
// 但这是 Go 版独有的权限放大——旧 Node adminOnly 只放行 platformadmin，MANAGED_ROLES 仅用于工作台任务可见范围和
// drill_plan.canHandle，不参与账号管理路由。为避免与现网权限模型不一致，恢复为仅 platformadmin。
//
// admin.go 里 canModifyRole / canModifyUserRole / itleadManageable 等细粒度代码**保留**，作为将来若显式配置打开
// itlead 账号管理权限时的守门器；默认不启用（因 RequireAdmin 已进门就拦）。
func (s *Service) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if !IsAdmin(u.Role.ID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员可访问系统管理功能"})
			return
		}
		c.Next()
	}
}

// RequireBrandAdmin 系统品牌设置权限（即 platformadmin）。
// 错误文案对齐 Node src/routes/system.js requireBrandAdmin。
func (s *Service) RequireBrandAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.ID != RolePlatformAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "当前角色无系统设置权限（仅平台系统管理员可维护）"})
			return
		}
		c.Next()
	}
}

// RequirePlatformBoard 剧本维护 / 项目新建 / 演练结束 — platformadmin + commander + itlead。
// 错误文案对齐 Node src/routes/content.js / src/routes/drill.js。
func (s *Service) RequirePlatformBoard() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if !IsPlatformBoard(u.Role.ID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可操作"})
			return
		}
		c.Next()
	}
}

// RequireScenariosAdmin 剧本编排保存权限。
// 错误文案对齐 Node src/routes/content.js "无剧本编排权限"。
func (s *Service) RequireScenariosAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if !IsPlatformBoard(u.Role.ID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无剧本编排权限"})
			return
		}
		c.Next()
	}
}

// RequireNotExternal 外部单位不可访问（处置、取证、研判等内部操作）。
// 错误文案对齐 Node src/routes/drill.js "分级权限隔离" 系列。
func (s *Service) RequireNotExternal() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "外部单位仅可查看医院上报材料（分级权限隔离）"})
			return
		}
		c.Next()
	}
}

// RequireNotExternalWrite 外部单位只读：GET/HEAD/OPTIONS 放行，其余方法一律 403。
// 用于 content / tasks / evidence / drill 等业务分组，避免逐个写路由漏挂导致
// 卫健/网信/网安账号可以改分、启停任务、删任务或变更项目状态。
func (s *Service) RequireNotExternalWrite() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "外部单位仅可查看医院上报材料（分级权限隔离）"})
			return
		}
		c.Next()
	}
}

// RequireCommanderOrAdmin 仅 platformadmin / commander（任务全量重置等高风险操作）。
func (s *Service) RequireCommanderOrAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.ID != RolePlatformAdmin && u.Role.ID != RoleCommander {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部可重置任务状态"})
			return
		}
		c.Next()
	}
}

// RequireNotExternalDrill 演练处置操作专用（比上面更严格）
func (s *Service) RequireNotExternalDrill() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.External {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "外部单位不参与处置记录（分级权限隔离）"})
			return
		}
		c.Next()
	}
}

// RequireMessagePublisher 指令类消息发布权限：commander / itlead / platformadmin。
// 错误文案对齐 Node src/routes/content.js messages POST。
func (s *Service) RequireMessagePublisher() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		switch u.Role.ID {
		case RoleCommander, RoleITLead, RolePlatformAdmin:
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅指挥部/信息科负责人可发布指令类消息"})
		}
	}
}

// RequireScenariosWrite 剧本/场景写操作权限（比剧本保存稍松）。
// 对齐 Node src/routes/content.js scenarios PATCH / POST。
func (s *Service) RequireScenariosWrite() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if !IsPlatformBoard(u.Role.ID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可维护剧本"})
			return
		}
		c.Next()
	}
}
