// Package router 组装全部 Gin 中间件和业务 handler。
package router

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/handler"
	"drill-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// New 组装完整 Gin engine
func New(database *db.DB, cfg *config.Config, authSvc *auth.Service, projectRoot string) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery()) // panic recover，返回 500

	// 可信代理（让 gin 正确识别 ClientIP）
	if len(cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			slog.Warn("SetTrustedProxies 失败", "err", err)
		}
	}

	// ---- 全局中间件（先安全头 + CORS + BodyLimit）----
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg))
	r.Use(middleware.BodyLimit(int64(cfg.UploadMaxMB)))
	// 服务端审计：非只读 /api 请求在响应后落库，字段全部来自服务端上下文。
	r.Use(handler.AuditWrites(database))

	// 全局限流只覆盖 /api/*，不覆盖 /public/*、/uploads/*、SPA 兜底页。
	// 先创建一个共享的 limiter（store 跨请求持久化），再用闭包按路径分发。
	apiRateLimiter := middleware.RateLimit(cfg.RateLimitGlobal)
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			apiRateLimiter(c)
		} else {
			c.Next()
		}
	})

	// ---- 静态：public/ 前端 ----
	publicDir := filepath.Join(projectRoot, "public")
	if st, err := os.Stat(publicDir); err == nil && st.IsDir() {
		slog.Info("挂载静态 public/ 目录", "path", publicDir)
		// 显式挂载 /public 到 publicDir
		r.Static("/public", publicDir)
		// NoRoute：所有已注册路由都不匹配时进来。
		// 在此处理根级裸链接 /<name>.html：存在则 serve，否则兜底返回登录页。
		// 这样前端 HTML 页面间裸相对链接（如 href="projects.html"）能正常命中真实页面。
		// 不用 GET /*name 通配符（与 /public/*filepath 冲突，会 panic）。
		r.NoRoute(func(c *gin.Context) {
			orig := c.Request.URL.Path

			// 流程字典页已并入系统设置：旧链接 302 到 admin.html#dict，避免历史书签 404
			if strings.TrimPrefix(orig, "/") == "dictionary.html" {
				c.Redirect(http.StatusFound, "/admin.html#dict")
				return
			}

			// ---- 安全静态文件服务 ----
			// 1. URL 解码 + 去除查询串
			pathRaw := c.Request.URL.Path
			if u, err := url.PathUnescape(pathRaw); err == nil {
				pathRaw = u
			}

			// 2. 规范化为纯相对路径（跨平台 Clean）
			cleaned := filepath.Clean(pathRaw)
			// filepath.Clean 会保留前导分隔符，去掉它
			cleaned = strings.TrimPrefix(cleaned, string(filepath.Separator))
			cleaned = strings.TrimPrefix(cleaned, "/")

			// 3. 逐段拒绝 ..（filepath.Clean 已合并合法的 ..，剩下的 ".." 都是越界段）
			//    用 separator 拆分后逐段检查，避免误杀文件名里含 ".." 的正常文件
			for _, seg := range strings.FieldsFunc(cleaned, func(r rune) bool { return r == '/' || r == '\\' }) {
				if seg == ".." {
					c.Status(http.StatusNotFound)
					return
				}
				// 4. 拒绝隐藏文件（段以 "." 开头，如 ".env"、".git"）
				if strings.HasPrefix(seg, ".") {
					c.Status(http.StatusNotFound)
					return
				}
			}

			// 5. 拼绝对路径并做边界校验
			absDisk := filepath.Join(publicDir, cleaned)
			// 防止 Windows 反斜杠穿越变体已由 filepath.Clean 覆盖
			// 用 Rel 检查 absDisk 是否仍在 publicDir 内
			rel, err := filepath.Rel(publicDir, absDisk)
			if err != nil || strings.HasPrefix(rel, "..") {
				c.Status(http.StatusNotFound)
				return
			}

			// 6. 只允许文件，拒绝目录
			if fi, err := os.Stat(absDisk); err == nil {
				if !fi.IsDir() {
					// HTML 或静态资源都允许（通过安全校验）
					c.File(absDisk)
					return
				}
				// 目录：走 SPA fallback 到 index.html
				if strings.HasSuffix(orig, ".html") || orig == "/" {
					c.File(filepath.Join(publicDir, "index.html"))
					return
				}
				c.Status(http.StatusNotFound)
				return
			}

			// 7. 文件不存在
			if strings.HasSuffix(orig, ".html") || orig == "/" {
				c.File(filepath.Join(publicDir, "index.html"))
				return
			}
			c.Status(http.StatusNotFound)
		})
	} else {
		slog.Warn("public/ 目录不存在，未挂载前端", "path", publicDir)
	}

	// ---- 附件目录：不再匿名静态暴露 ----
	// 浏览器 <img>/<a> 无法带自定义 Header，因此同时接受 Authorization /
	// X-Drill-Token / ?t= 与登录时下发的 HttpOnly 媒体 Cookie。
	uploadDir := filepath.Join(projectRoot, cfg.UploadRoot)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		slog.Error("创建 uploads 目录失败", "path", uploadDir, "err", err)
	} else {
		slog.Info("挂载鉴权 uploads/ 目录", "path", uploadDir)
		mediaAuth := uploadAuth(authSvc)
		r.GET("/uploads/*filepath", mediaAuth, serveUpload(uploadDir))
		r.HEAD("/uploads/*filepath", mediaAuth, serveUpload(uploadDir))
	}

	// ---- /api ----
	api := r.Group("/api")
	{
		// 无需认证
		api.GET("/health", handler.Health(database))

		// ---- api/screen — 大屏免登录只读观摩（SCREEN_PUBLIC_VIEW 控制）----
		// 打开时：匿名可 GET，独立匿名限流；任何写方法 403，绝不落库。
		// 关闭时：恢复 RequireAuth，未登录 401。
		scr := api.Group("/screen")
		if cfg.ScreenPublicView {
			scr.Use(middleware.AnonRateLimit(cfg.RateLimitScreen), authSvc.OptionalAuth())
		} else {
			scr.Use(authSvc.RequireAuth())
		}
		{
			scr.GET("/monitor", handler.ScreenMonitorHandler(database))
			scr.GET("/notifs", handler.ScreenNotifsHandler(database))
			scr.GET("/summary", handler.ScreenSummaryHandler(database))
			for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
				scr.Handle(m, "/monitor", screenDenyWrite)
				scr.Handle(m, "/notifs", screenDenyWrite)
				scr.Handle(m, "/summary", screenDenyWrite)
			}
		}

		// ---- api/auth 组 ----
		authg := api.Group("/auth")
		{
			// 免登录（账号列表已关闭匿名访问，改为仅平台管理员维护时使用）
			authg.GET("/roles", handler.GetRoles(database))
			authg.GET("/captcha", middleware.RateLimit(cfg.RateLimitCaptcha), handler.Captcha(cfg.CaptchaTTLSeconds))
			authg.POST("/login",
				middleware.RateLimitByGroup("/api/auth", cfg.RateLimitAuth),
				handler.Login(authSvc, database, cfg))
			// logout 可不带 auth（delete token 尽力而为），同时清理媒体 Cookie
			authg.POST("/logout", handler.Logout(authSvc, cfg))
		}
		// 需认证
		api.GET("/auth/accounts", authSvc.RequireAuth(), authSvc.RequireAdmin(), handler.GetAccounts(database))
		api.GET("/auth/me", authSvc.RequireAuth(), handler.Me())
		api.POST("/auth/change-password", authSvc.RequireAuth(), handler.ChangePassword(database, authSvc, cfg))

		// ---- api/bootstrap 聚合 DTO ----
		api.GET("/bootstrap", authSvc.RequireAuth(), handler.BootstrapHandler(database))
		api.POST("/projects/:id/activate", authSvc.RequireAuth(), authSvc.RequirePlatformBoard(), handler.PostProjectActivate(database))
		api.PATCH("/projects/:id/status", authSvc.RequireAuth(), authSvc.RequirePlatformBoard(), handler.PatchProjectStatus(database))

		// ---- 备份目录（admin + system reset 共用）----
		backupDir := filepath.Join(projectRoot, cfg.DataDir, "backups")

		// ---- api/system — 部分免登录 ----
		sys := api.Group("/system")
		{
			sys.GET("/brand", handler.GetBrand(database)) // 免登录
			sys.PUT("/brand", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.PutBrand(database))
			sys.GET("/status", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.GetStatus(database))
			sys.GET("/dict/:key", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.GetDrillDict(database))
			sys.PUT("/dict/:key", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.PutDrillDict(database))

			// ---- C 块: seed/export/baselines + system reset ----
			sys.GET("/seed/baselines", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.SystemSeedBaselines(database))
			sys.POST("/seed/export", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.SystemSeedExport(database))
			sys.POST("/reset", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.SystemReset(database, cfg, backupDir))
			sys.POST("/logo", authSvc.RequireAuth(), authSvc.RequireBrandAdmin(), handler.PostSystemLogo(database))
		}

		// ---- api/admin — 全部 RequireAdmin ----
		adm := api.Group("/admin", authSvc.RequireAuth(), authSvc.RequireAdmin())
		{
			adm.GET("/users", handler.AdminGetUsers(database))
			adm.POST("/users", handler.AdminPostUsers(database))
			adm.PATCH("/users/:id", handler.AdminPatchUsers(database))
			adm.POST("/users/:id/reset-password", handler.AdminResetPassword(database))
			adm.POST("/users/cleanup-test-accounts", handler.AdminCleanupTestAccounts(database))
			adm.GET("/roles", handler.AdminGetRoles(database))
			adm.POST("/roles", handler.AdminPostRoles(database))
			adm.PUT("/roles/:id", handler.AdminPutRoles(database))
			adm.PATCH("/roles/:id/status", handler.AdminPatchRolesStatus(database))
			adm.DELETE("/roles/:id", handler.AdminDeleteRoles(database))
			adm.PATCH("/roles/order", handler.AdminPatchRolesOrder(database))
			adm.GET("/forms", handler.AdminGetForms(database))
			adm.POST("/forms", handler.AdminPostForms(database))
			adm.PUT("/forms/:id", handler.AdminPutForms(database))
			adm.DELETE("/forms/:id", handler.AdminDeleteForms(database))
			adm.GET("/audit/export", handler.AdminGetAuditExport(database))

			// ---- C 块: 备份/恢复/系统信息 ----
			adm.POST("/backup", handler.AdminBackup(database, backupDir, cfg))
			adm.GET("/backups", handler.AdminListBackups(database, backupDir))
			adm.GET("/backups/:file", handler.AdminDownloadBackup(database, backupDir))
			adm.DELETE("/backups/:file", handler.AdminDeleteBackup(database, backupDir))
			adm.POST("/restore", handler.AdminRestore(database, backupDir, cfg))
			adm.POST("/restore-upload", handler.AdminRestoreUpload(database))
			adm.GET("/sysinfo", handler.AdminSysInfo(database))
		}

		// ---- 演练报告：平台管理员 / 指挥部 / 信息科负责人（与复盘评估同权）----
		rpt := api.Group("/admin", authSvc.RequireAuth(), authSvc.RequirePlatformBoard())
		{
			rpt.GET("/report/:projectId", handler.AdminReport(database))
			rpt.POST("/report/:projectId/export", handler.AdminReportExport(database))
		}

		// ---- api/content — 全部 RequireAuth；外部单位任何写操作统一 403 ----
		cnt := api.Group("/content", authSvc.RequireAuth(), authSvc.RequireNotExternalWrite())
		{
			cnt.GET("/monitor", handler.MonitorContentHandler(database))                                           // 大屏监视聚合（screen.html 依赖）
			cnt.GET("/evaluation", handler.GetEvaluation(database))                                                // 评分项模板 + 当前项目 scores 明细
			cnt.POST("/evaluation/scores", authSvc.RequirePlatformBoard(), handler.PostEvaluationScores(database)) // 手动/自动考评 K1-K9 分项写入
			cnt.GET("/scenarios", handler.GetScenarios(database))
			cnt.GET("/script-nodes", handler.GetScriptNodes(database))
			cnt.GET("/projects", handler.GetProjects(database))
			cnt.GET("/messages", handler.GetMessages(database))
			cnt.GET("/notifs", handler.GetNotifs(database))
			cnt.GET("/dict", handler.GetDict(database))
			cnt.GET("/sim-data", handler.GetSimData(database))
			cnt.GET("/knowledge", handler.GetKnowledge(database))
			cnt.GET("/timeline", handler.GetTimeline(database))
			cnt.GET("/ledger", handler.GetLedger(database))
			cnt.GET("/opinion", handler.GetOpinion(database))
			cnt.GET("/impact", handler.GetImpact(database))
			cnt.GET("/snapshots", handler.GetSnapshots(database))
			cnt.GET("/seqflow", handler.GetSeqFlow(database))

			// ---- P2-3 写操作 ----
			cnt.POST("/scenarios", authSvc.RequireScenariosWrite(), handler.PostScenario(database))
			cnt.PATCH("/scenarios/:id", authSvc.RequireScenariosWrite(), handler.PatchScenario(database))
			cnt.DELETE("/knowledge/:id", authSvc.RequirePlatformBoard(), handler.DeleteKnowledge(database))
			cnt.POST("/knowledge", authSvc.RequirePlatformBoard(), handler.PostKnowledge(database, uploadDir, cfg.UploadMaxMB))
			cnt.POST("/script-nodes", authSvc.RequireScenariosAdmin(), handler.PostScriptNodes(database))
			// 剧本节点软删：回收站与恢复
			cnt.GET("/scenarios/:id/nodes/deleted", handler.GetDeletedScriptNodes(database))
			cnt.POST("/scenarios/:id/nodes/:nodeId/restore", authSvc.RequireScenariosAdmin(), handler.PostScriptNodeRestore(database))
			// P1-3 剧本版本历史：查看与回滚
			cnt.GET("/scenarios/:id/revisions", handler.GetScriptRevisions(database))
			cnt.GET("/scenarios/:id/revisions/:version", handler.GetScriptRevision(database))
			cnt.POST("/scenarios/:id/revisions/:version/restore", authSvc.RequireScenariosAdmin(), handler.PostScriptRevisionRestore(database))

			// ---- P2-4 Drill Core ----
			cnt.POST("/projects", authSvc.RequirePlatformBoard(), handler.PostProject(database))
			cnt.GET("/projects/:id/script", handler.GetProjectScript(database))
			cnt.POST("/projects/:id/script/force", authSvc.RequirePlatformBoard(), handler.PostProjectScriptForceView(database))
			cnt.GET("/projects/:id/script/export", handler.GetProjectScriptExport(database))
			cnt.POST("/projects/:id/notify", authSvc.RequirePlatformBoard(), handler.PostProjectNotify(database))
			cnt.GET("/projects/:id/notify/export", handler.GetProjectNotifyExport(database))
			cnt.PATCH("/projects/:id/members", authSvc.RequirePlatformBoard(), handler.PatchProjectMembers(database))
			cnt.DELETE("/projects/:id", authSvc.RequirePlatformBoard(), handler.DeleteProject(database, uploadDir))

			// ---- content 写操作补齐 ----
			cnt.POST("/messages", authSvc.RequireMessagePublisher(), handler.PostMessages(database))
			cnt.POST("/notifs/read", handler.PostNotifsRead(database))
			cnt.POST("/snapshots", authSvc.RequirePlatformBoard(), handler.PostSnapshots(database))
			cnt.POST("/seqflow", authSvc.RequirePlatformBoard(), handler.PostSeqFlow(database))
			cnt.POST("/audit", handler.PostAudit(database))
			cnt.GET("/audit", handler.GetContentAudit(database))
			cnt.POST("/opinion", authSvc.RequirePlatformBoard(), handler.PostOpinion(database))
			cnt.PATCH("/opinion/:id", authSvc.RequirePlatformBoard(), handler.PatchOpinion(database))
			cnt.PATCH("/impact/:sys", authSvc.RequirePlatformBoard(), handler.PatchImpact(database))
			cnt.GET("/users", handler.GetContentUsers(database))
			cnt.GET("/roles", handler.GetContentRoles(database))
			cnt.GET("/dashboard", handler.GetDashboard(database))
			cnt.GET("/drill-progress", handler.GetDrillProgress(database))

			// ---- chat ----
			cnt.GET("/chat/channels", handler.GetChatChannels(database))
			cnt.GET("/chat/:group", handler.GetChat(database))
			cnt.POST("/chat/:group", handler.PostChat(database))
			cnt.POST("/chat/:group/upload", handler.PostChatUpload(database))

			// ---- /api/drill v1 旧路径（前端仍在调用，v2 新路径无调用方已删除） ----
			drill := api.Group("/drill", authSvc.RequireAuth(), authSvc.RequireNotExternalWrite())
			{
				drill.GET("/workbench/:projectId", handler.GetDrillWorkbench(database))
				drill.POST("/nodes/:nodeId/start", handler.PostDrillNodesStart(database))
				drill.GET("/flows/:projectId", handler.GetDrillFlows(database))
				drill.GET("/faults/:projectId", handler.GetDrillFaults(database))
				drill.POST("/skip", handler.PostDrillSkip(database))
				drill.POST("/decide", handler.PostDrillDecide(database))
				drill.POST("/finish", handler.PostDrillFinish(database))
				drill.POST("/steps", handler.PostDrillSteps(database))
				drill.DELETE("/steps/:id", handler.DeleteDrillStep(database))
				drill.POST("/score/:projectId/recalc", handler.PostDrillScoreRecalcV1(database))
				drill.GET("/predecessor", handler.GetDrillPredecessor(database))
			}
		}

		// ---- api/tasks 旧版路径（前端仍在调用）；外部单位任何写操作 403 ----
		tasks := api.Group("/tasks", authSvc.RequireAuth(), authSvc.RequireNotExternalWrite())
		{
			tasks.GET("", handler.GetTasksLegacy(database))
			tasks.GET("/", handler.GetTasksLegacy(database))
			tasks.POST("/reset-all", authSvc.RequireCommanderOrAdmin(), handler.PostTaskResetAll(database))
			tasks.POST("/:id/accept", handler.PostTaskAccept(database))
			tasks.POST("/:id/submit", handler.PostTaskSubmit(database))
			tasks.POST("/:id/complete", handler.PostTaskComplete(database))
			tasks.DELETE("/:id", handler.DeleteTaskLegacy(database))
			tasks.GET("/:id/attachments", handler.GetTaskAttachments(database))
			tasks.POST("/:id/attachments", handler.PostTaskAttachments(database, uploadDir, cfg.UploadMaxMB))
			tasks.DELETE("/:id/attachments/:attId", handler.DeleteTaskAttachment(database, uploadDir))
			tasks.POST("/:id/forms", handler.PostTaskForms(database))
			tasks.GET("/:id/forms", handler.GetTaskForms(database))
		}

		// ---- api/evidence 旧版路径（前端仍在调用）；外部单位只读 ----
		evd := api.Group("/evidence", authSvc.RequireAuth(), authSvc.RequireNotExternalWrite())
		{
			evd.GET("", handler.GetEvidenceLegacy(database))
			evd.GET("/", handler.GetEvidenceLegacy(database))
			evd.POST("", handler.PostEvidenceLegacy(database))
			evd.POST("/", handler.PostEvidenceLegacy(database))
			evd.POST("/prefill", handler.PostEvidencePrefill(database))
			evd.POST("/mock", handler.PostEvidenceMock(database))
			evd.POST("/upload", handler.PostEvidenceUpload(database))
			evd.GET("/requirements", handler.GetEvidenceRequirements(database))
			evd.GET("/:id/download", handler.GetEvidenceDownload(database))
			evd.DELETE("/:id", handler.DeleteEvidence(database, uploadDir))
		}
	}

	return r
}

// uploadAuth /uploads 目录鉴权：
//  1. 优先使用 Authorization / X-Drill-Token / ?t=；
//  2. 浏览器直接请求（<img>、<a>）无法带 Header，回退到登录时下发的
//     HttpOnly 媒体 Cookie；
//  3. 待改密受限会话不允许读取附件。
func uploadAuth(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := auth.TokenOf(c)
		if token == "" {
			if ck, err := c.Cookie(auth.MediaCookieName); err == nil {
				token = ck
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期，请重新登录"})
			return
		}
		u, err := svc.UserByToken(token)
		if err != nil || u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期，请重新登录"})
			return
		}
		if u.MustChange || u.Scope == auth.ScopePasswordChange {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "首次登录需先修改初始密码"})
			return
		}
		// 写入上下文，供全局审计中间件记录附件读取
		c.Set(auth.ContextKey, u)
		c.Next()
	}
}

// serveUpload 安全地提供 uploads 目录下的文件：
// 拒绝目录、隐藏文件与路径穿越，响应禁止缓存（避免鉴权内容被代理缓存）。
func serveUpload(root string) gin.HandlerFunc {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	return func(c *gin.Context) {
		raw := strings.TrimPrefix(c.Param("filepath"), "/")
		if raw == "" {
			c.Status(http.StatusNotFound)
			return
		}
		cleaned := filepath.Clean(raw)
		if cleaned == "." || strings.HasPrefix(cleaned, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		for _, seg := range strings.FieldsFunc(cleaned, func(r rune) bool { return r == '/' || r == '\\' }) {
			if seg == "" || seg == ".." || strings.HasPrefix(seg, ".") {
				c.Status(http.StatusNotFound)
				return
			}
		}
		abs := filepath.Join(absRoot, cleaned)
		rel, err := filepath.Rel(absRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		fi, err := os.Stat(abs)
		if err != nil || fi.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		http.ServeFile(c.Writer, c.Request, abs)
	}
}

// screenDenyWrite 大屏只读接口的写方法兜底：直接 403，不进入任何业务逻辑。
func screenDenyWrite(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "大屏观摩接口为只读，禁止写操作"})
}
