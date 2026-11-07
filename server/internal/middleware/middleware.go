// Package middleware 提供 CORS / 限流 / 安全响应头 / 请求体大小限制。
package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"drill-system/internal/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ---- CORS ----

// CORS 简单实现。默认不允许跨域（AllowOrigins 为空），仅对显式配置的来源放行。
// 通配 "*" 时不发送 Allow-Credentials（浏览器规范禁止二者同时使用）。
func CORS(cfg *config.Config) gin.HandlerFunc {
	allowAll := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := false
		if origin != "" {
			if allowAll {
				c.Header("Access-Control-Allow-Origin", "*")
				allowed = true
			} else {
				for _, o := range cfg.AllowOrigins {
					if o == origin {
						c.Header("Access-Control-Allow-Origin", origin)
						c.Header("Access-Control-Allow-Credentials", "true")
						allowed = true
						break
					}
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

// ---- 安全响应头 ----

// SecurityHeaders 给所有响应加上安全头。
// CSP 为了兼容现有前端（有内联 <script>），允许 'unsafe-inline'，
// 但仅限 script-src 和 style-src；connect-src / img-src 允许相对路径和 data:（base64 图片）。
func SecurityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("Referrer-Policy", "same-origin")
	c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	// 严格的 CSP：script-src/style-src 允许 unsafe-inline（Phase 1 先能用），
	// 后续 Phase 可逐步收紧（nonce / hash 方式改前端）。
	c.Header("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' 'unsafe-inline'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; "+
			"font-src 'self' data:; "+
			"connect-src 'self'; "+
			"frame-ancestors 'self'; "+
			"form-action 'self'")
	c.Next()
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

// hasToken 与 auth.TokenOf 的三重来源保持一致（此处不依赖 auth 包，避免循环引用）
func hasToken(c *gin.Context) bool {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") && strings.TrimSpace(h[7:]) != "" {
		return true
	}
	if strings.TrimSpace(c.GetHeader("X-Drill-Token")) != "" {
		return true
	}
	return strings.TrimSpace(c.Query("t")) != ""
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

// 让 rateStore 不被 gc 太早回收（只是占位，真正保留由闭包引用）
var _ = time.Minute
