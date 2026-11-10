package middleware

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestEffectiveTrustedProxies 回归 P2-6：未配置时失败关闭，只信任本机回环。
func TestEffectiveTrustedProxies(t *testing.T) {
	got, warnAll := EffectiveTrustedProxies(nil)
	if !reflect.DeepEqual(got, []string{"127.0.0.1", "::1"}) {
		t.Fatalf("未配置时应回退到回环地址，得到 %v", got)
	}
	if warnAll {
		t.Fatal("未配置时不应触发通配符告警")
	}

	got, warnAll = EffectiveTrustedProxies([]string{"", "  ", "*"})
	if !reflect.DeepEqual(got, []string{"127.0.0.1", "::1"}) || !warnAll {
		t.Fatalf("仅配置空白/通配符时应回退回环并告警，得到 %v warnAll=%v", got, warnAll)
	}

	got, warnAll = EffectiveTrustedProxies([]string{" 10.0.0.0/8 ", "", "*"})
	if !reflect.DeepEqual(got, []string{"10.0.0.0/8"}) {
		t.Fatalf("应保留明确配置并忽略通配符，得到 %v", got)
	}
	if !warnAll {
		t.Fatal("通配符应触发告警")
	}
}

func clientIPFor(t *testing.T, proxies []string, remoteAddr, xff string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(proxies); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	r.GET("/ip", func(c *gin.Context) { c.String(200, c.ClientIP()) })

	req := httptest.NewRequest("GET", "/ip", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String()
}

// TestSpoofedXFFIgnored 回归 P2-6：非可信对端伪造 X-Forwarded-For 不被采信。
func TestSpoofedXFFIgnored(t *testing.T) {
	proxies, _ := EffectiveTrustedProxies(nil)

	// 直连用户（非回环）伪造 XFF：必须取真实 TCP 对端地址。
	if got := clientIPFor(t, proxies, "192.0.2.10:1234", "1.2.3.4"); got != "192.0.2.10" {
		t.Fatalf("伪造 XFF 不应被采信，期望 192.0.2.10，得到 %q", got)
	}

	// 回环代理转发的 XFF：可信，正常采信。
	if got := clientIPFor(t, proxies, "127.0.0.1:1234", "1.2.3.4"); got != "1.2.3.4" {
		t.Fatalf("回环代理的 XFF 应被采信，期望 1.2.3.4，得到 %q", got)
	}

	// 显式配置的代理：可信，正常采信。
	if got := clientIPFor(t, []string{"192.0.2.10"}, "192.0.2.10:1234", "1.2.3.4"); got != "1.2.3.4" {
		t.Fatalf("显式配置代理的 XFF 应被采信，期望 1.2.3.4，得到 %q", got)
	}
}

// TestHasTokenIgnoresQuery 与 auth.TokenOf 对齐：?t= 不再算作已登录。
func TestHasTokenIgnoresQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/x?t=leaked", nil)
	if hasToken(c) {
		t.Fatal("hasToken 不应把 ?t= 当作令牌")
	}

	c.Request = httptest.NewRequest("GET", "/api/x", nil)
	c.Request.Header.Set("Authorization", "Bearer abc")
	if !hasToken(c) {
		t.Fatal("hasToken 应识别 Authorization Bearer")
	}
}
