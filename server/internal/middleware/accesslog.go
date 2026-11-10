package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"drill-system/internal/auth"

	"github.com/gin-gonic/gin"
)

// 访问日志默认值：慢请求阈值 1s，噪声接口每分钟汇总一条。
const (
	defaultSlowThreshold = time.Second
	defaultSummaryWindow = time.Minute
)

// AccessLogOptions 访问日志可调参数（仅单测需要显式缩短汇总窗口）。
type AccessLogOptions struct {
	SlowThreshold   time.Duration
	SummaryInterval time.Duration
}

// AccessLog 记录 /api/* 访问日志。
//
// 设计要点：
//   - 只记录 /api/*，静态页与 /media、/uploads 由下载审计覆盖，避免刷屏；
//   - 只记录 method/path/status/latency/client_ip/bytes/user_id，绝不读取或输出
//     Authorization、Cookie、请求体、query（登录口令不可能进日志）；
//   - /api/health 与 /api/screen/* 属高频轮询：逐条只打 Debug，每分钟额外输出一条
//     Info 汇总（次数 + 平均耗时）；
//   - 任何超过阈值的请求都单独打 Warn，不受噪声采样影响。
//
// 必须注册在 gin.Recovery() 之前：panic 时 Recovery 在 c.Next() 内写回 500 并返回，
// 本中间件才能拿到最终 status 与响应字节数。
func AccessLog(opts ...AccessLogOptions) gin.HandlerFunc {
	slow := defaultSlowThreshold
	summaryWindow := defaultSummaryWindow
	if len(opts) > 0 {
		if opts[0].SlowThreshold > 0 {
			slow = opts[0].SlowThreshold
		}
		if opts[0].SummaryInterval > 0 {
			summaryWindow = opts[0].SummaryInterval
		}
	}
	noise := &noiseStats{windowStart: time.Now()}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// 非 /api 请求与 CORS 预检不记录
		if !strings.HasPrefix(path, "/api/") || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		size := c.Writer.Size()
		if size < 0 {
			size = 0
		}
		var uid int64
		if u := auth.CurrentUser(c); u != nil {
			uid = u.ID
		}
		fields := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
			"bytes", size,
			"user_id", uid,
		}

		if isNoiseAPIPath(path) {
			slog.Debug("http_request", fields...)
			if n, avgMs, ok := noise.observe(latency, summaryWindow); ok {
				slog.Info("http_noise_summary",
					"window", summaryWindow.String(), "requests", n, "avg_latency_ms", avgMs)
			}
		} else {
			slog.Info("http_request", fields...)
		}

		if latency > slow {
			slog.Warn("slow_request", append(fields, "threshold_ms", slow.Milliseconds())...)
		}
	}
}

// isNoiseAPIPath 高频只读轮询接口：健康检查与大屏观测（monitor/notifs/summary）。
func isNoiseAPIPath(path string) bool {
	return path == "/api/health" || strings.HasPrefix(path, "/api/screen/")
}

// noiseStats 噪声接口的分钟级汇总器。
type noiseStats struct {
	mu          sync.Mutex
	count       uint64
	total       time.Duration
	windowStart time.Time
}

// observe 累加一次噪声请求；到达汇总窗口时返回本轮次数与平均耗时。
func (n *noiseStats) observe(latency time.Duration, window time.Duration) (count uint64, avgMs int64, flush bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.count++
	n.total += latency
	if time.Since(n.windowStart) < window {
		return 0, 0, false
	}
	count = n.count
	if count > 0 {
		avgMs = n.total.Milliseconds() / int64(count)
	}
	n.count, n.total, n.windowStart = 0, 0, time.Now()
	return count, avgMs, true
}
