package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTokenCtx(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", target, nil)
	return c
}

// TestTokenOfRejectsQuery 回归 P2-3：?t= 查询参数不再被接受。
func TestTokenOfRejectsQuery(t *testing.T) {
	c := newTokenCtx("/api/content/projects/p1/script/export?fmt=word&t=leaked-token")
	if got := TokenOf(c); got != "" {
		t.Fatalf("TokenOf 不应接受 ?t= 查询参数，得到 %q", got)
	}
}

// TestTokenOfAcceptsHeaders 确认移除 Query 后两种请求头仍然可用。
func TestTokenOfAcceptsHeaders(t *testing.T) {
	bearer := newTokenCtx("/api/x")
	bearer.Request.Header.Set("Authorization", "Bearer abc.def")
	if got := TokenOf(bearer); got != "abc.def" {
		t.Fatalf("Authorization Bearer 解析错误，得到 %q", got)
	}

	custom := newTokenCtx("/api/x")
	custom.Request.Header.Set("X-Drill-Token", "xyz")
	if got := TokenOf(custom); got != "xyz" {
		t.Fatalf("X-Drill-Token 解析错误，得到 %q", got)
	}
}
