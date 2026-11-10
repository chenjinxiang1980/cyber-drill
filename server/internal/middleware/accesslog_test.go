package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// captureLogs 把默认 slog 输出重定向到缓冲区，返回读取函数。
func captureLogs(t *testing.T, level slog.Level) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	return &buf, func() { slog.SetDefault(old) }
}

func newLogTestRouter(h gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AccessLog())
	r.GET("/api/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	r.GET("/api/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/api/screen/monitor", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.POST("/api/auth/login", func(c *gin.Context) {
		var body map[string]any
		_ = c.ShouldBindJSON(&body)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/slow", func(c *gin.Context) {
		time.Sleep(1200 * time.Millisecond)
		c.String(http.StatusOK, "slow")
	})
	r.GET("/public/index.html", func(c *gin.Context) { c.String(http.StatusOK, "html") })
	if h != nil {
		r.GET("/api/custom", h)
	}
	return r
}

func doRequest(r *gin.Engine, method, path, body string, headers map[string]string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
}

func TestAccessLogNormalRequest(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelInfo)
	defer restore()
	r := newLogTestRouter(nil)

	doRequest(r, http.MethodGet, "/api/ping", "", nil)
	out := buf.String()
	for _, want := range []string{
		"http_request", "method=GET", "path=/api/ping", "status=200",
		"latency_ms=", "client_ip=", "bytes=4", "user_id=0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("访问日志缺少 %q，实际输出：%s", want, out)
		}
	}
}

func TestAccessLogSkipsNonAPI(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()
	r := newLogTestRouter(nil)

	doRequest(r, http.MethodGet, "/public/index.html", "", nil)
	if strings.Contains(buf.String(), "http_request") {
		t.Fatalf("非 /api 路径不应记录访问日志：%s", buf.String())
	}
}

func TestAccessLogNoiseOnlyDebug(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelInfo)
	defer restore()
	r := newLogTestRouter(nil)

	for i := 0; i < 50; i++ {
		doRequest(r, http.MethodGet, "/api/health", "", nil)
		doRequest(r, http.MethodGet, "/api/screen/monitor", "", nil)
	}
	out := buf.String()
	if strings.Contains(out, "http_request") {
		t.Fatalf("噪声接口在 Info 级别不应逐条输出：%s", out)
	}
	if strings.Contains(out, "slow_request") {
		t.Fatalf("正常噪声请求不应触发慢请求告警：%s", out)
	}
}

func TestAccessLogNoiseSummary(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelInfo)
	defer restore()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AccessLog(AccessLogOptions{SummaryInterval: time.Millisecond}))
	r.GET("/api/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	doRequest(r, http.MethodGet, "/api/health", "", nil)
	time.Sleep(2 * time.Millisecond)
	doRequest(r, http.MethodGet, "/api/health", "", nil)

	out := buf.String()
	if !strings.Contains(out, "http_noise_summary") {
		t.Fatalf("到达汇总窗口后应输出一条 Info 汇总：%s", out)
	}
	if !strings.Contains(out, "requests=2") {
		t.Fatalf("汇总应包含本轮请求次数：%s", out)
	}
	if strings.Contains(out, "http_request") {
		t.Fatalf("噪声接口不应逐条打 Info：%s", out)
	}
}

func TestAccessLogSlowWarn(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelInfo)
	defer restore()
	r := newLogTestRouter(nil)

	doRequest(r, http.MethodGet, "/api/slow", "", nil)
	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "slow_request") {
		t.Fatalf("超过阈值的请求应打 Warn：%s", out)
	}
	if !strings.Contains(out, "threshold_ms=1000") {
		t.Fatalf("慢请求告警应带阈值：%s", out)
	}
	t.Logf("slow request log: %s", strings.TrimSpace(out))
}

func TestAccessLogNeverLogsSecrets(t *testing.T) {
	buf, restore := captureLogs(t, slog.LevelDebug)
	defer restore()
	r := newLogTestRouter(nil)

	doRequest(r, http.MethodPost, "/api/auth/login?token=query-secret",
		`{"username":"alice","password":"P@ssw0rd-secret"}`,
		map[string]string{
			"Authorization": "Bearer header-secret-token",
			"Cookie":        "drill_session=cookie-secret",
			"Content-Type":  "application/json",
		})
	out := buf.String()
	for _, secret := range []string{"P@ssw0rd-secret", "header-secret-token", "cookie-secret", "query-secret", "alice"} {
		if strings.Contains(out, secret) {
			t.Fatalf("访问日志泄露敏感信息 %q：%s", secret, out)
		}
	}
}
