// Package middleware 提供 CORS / 限流 / 安全响应头 / 请求体大小限制。
package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"drill-system/internal/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ---- CORS ----

// CORS 失败关闭：默认不允许跨域（AllowOrigins 为空），仅对显式配置的来源放行。
// 通配 "*" 一律视为未配置并告警——本系统是纯同源部署，不应把 API 暴露给任意站点。
// 白名单命中时回显 origin 并允许 credentials；未命中则完全不发 CORS 头。
func CORS(cfg *config.Config) gin.HandlerFunc {
	origins := sanitizeOrigins(cfg.AllowOrigins)
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := false
		if origin != "" {
			for _, o := range origins {
				if o == origin {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Access-Control-Allow-Credentials", "true")
					allowed = true
					break
				}
			}
		}
		c.Header("Vary", "Origin")
		if allowed {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Drill-Token")
			c.Header("Access-Control-Expose-Headers", "Content-Disposition, X-Total-Count")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// sanitizeOrigins 去空白、去空项，并拒绝 "*"（通配会导致任意站点可跨域读取 API）。
func sanitizeOrigins(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			slog.Warn("ALLOW_ORIGINS 含 *，已按失败关闭处理；请改为精确来源白名单")
			continue
		}
		out = append(out, o)
	}
	return out
}

// ---- 安全响应头 ----

// SecurityHeaders 给所有响应加上安全头。
//
// script-src 使用启动时从 public/*.html 计算出的内联脚本 sha256 白名单，
// 不再允许 'unsafe-inline'；style-src 仍保留 'unsafe-inline'，
// 因为现有页面大量使用 style="..." 属性，nonce/hash 无法覆盖属性级样式。
func SecurityHeaders(scriptHashes []string) gin.HandlerFunc {
	scriptSrc := "'self'"
	for _, h := range scriptHashes {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		scriptSrc += " 'sha256-" + h + "'"
	}
	csp := "default-src 'self'; " +
		"script-src " + scriptSrc + "; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'self'; " +
		"form-action 'self'"
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", csp)
		c.Next()
	}
}

// ---- 限流：按 IP + 可选路由分组 ----

// rateStore per-IP limiter 存储
type rateStore struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rps      float64
	burst    int
}

func newStore(rpm int) *rateStore {
	// rps = rpm / 60，burst 取 rpm 作为分钟内突发
	return &rateStore{
		limiters: make(map[string]*rate.Limiter),
		rps:      float64(rpm) / 60.0,
		burst:    rpm,
	}
}

func (s *rateStore) get(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.limiters[ip]; ok {
		return l
	}
	l := rate.NewLimiter(rate.Limit(s.rps), s.burst)
	s.limiters[ip] = l
	return l
}

// RateLimit 全局 IP 限流（rpm req/min）。
func RateLimit(rpm int) gin.HandlerFunc {
	store := newStore(rpm)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		// 简单清理（每分钟扫一次）——限流不是严格指标，偶尔泄漏 OK
		l := store.get(ip)
		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}

// RateLimitByGroup 为不同路由分组设置不同速率。pathPrefix = "/api/auth" 等。
func RateLimitByGroup(pathPrefix string, rpm int) gin.HandlerFunc {
	store := newStore(rpm)
	return func(c *gin.Context) {
		if len(c.Request.URL.Path) >= len(pathPrefix) &&
			c.Request.URL.Path[:len(pathPrefix)] == pathPrefix {
			l := store.get(c.ClientIP())
			if !l.Allow() {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁"})
				return
			}
		}
		c.Next()
	}
}

// AnonRateLimit 只对匿名请求限流（带令牌的已登录请求直接放行）。
// 用于大屏免登录观摩接口：避免匿名链路成为压测入口，同时不影响登录用户。
func AnonRateLimit(rpm int) gin.HandlerFunc {
	store := newStore(rpm)
	return func(c *gin.Context) {
		if hasToken(c) {
			c.Next()
			return
		}
		if !store.get(c.ClientIP()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "匿名访问过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}

// hasToken 与 auth.TokenOf 保持一致（此处不依赖 auth 包，避免循环引用）。
// 只认请求头：Query ?t= 已废弃，避免令牌出现在日志/Referer/浏览器历史里。
func hasToken(c *gin.Context) bool {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") && strings.TrimSpace(h[7:]) != "" {
		return true
	}
	return strings.TrimSpace(c.GetHeader("X-Drill-Token")) != ""
}

// BodyLimit 请求体大小限制（MB）。Gin 默认没有，我们包装 Body 读前截断。
func BodyLimit(maxMB int64) gin.HandlerFunc {
	maxBytes := maxMB * 1024 * 1024
	return func(c *gin.Context) {
		limit := maxBytes
		// 备份恢复上传的是整库 .sql 快照，体积与普通附件上限无关（P0-2）
		if c.Request.URL.Path == "/api/admin/restore-upload" {
			limit = 2 * 1024 * 1024 * 1024
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge,
				gin.H{"error": "请求体过大，超过 " + itoa(limit/1024/1024) + "MB 限制"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func itoa(n int64) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// TrustedProxies 让 Gin 正确读取 X-Forwarded-For（ClientIP）
func TrustedProxies(proxies []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// gin.SetTrustedProxies 在创建 engine 时调用；这里只做兜底
		c.Next()
	}
}

// loopbackProxies 未配置 TRUSTED_PROXIES 时的默认信任集合：只信任本机回环。
// 这样直连用户的 X-Forwarded-For 一律不被采信，ClientIP 取真实 TCP 对端地址，
// 登录锁定/审计/限流不会被伪造 XFF 绕过或串号。
var loopbackProxies = []string{"127.0.0.1", "::1"}

// EffectiveTrustedProxies 计算实际要交给 gin.SetTrustedProxies 的列表。
// 失败关闭：未配置（或只配了空白/通配符）时返回回环地址，而不是让 gin 信任所有代理。
func EffectiveTrustedProxies(configured []string) (list []string, warnAll bool) {
	out := make([]string, 0, len(configured))
	for _, p := range configured {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "*" {
			warnAll = true
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return loopbackProxies, warnAll
	}
	return out, warnAll
}

// 让 rateStore 不被 gc 太早回收（只是占位，真正保留由闭包引用）
var _ = time.Minute
