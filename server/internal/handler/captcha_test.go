package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newCaptchaTestCtx 构造与登录请求同源 IP 的 gin 上下文，
// 验证码绑定客户端 IP，测试上下文必须稳定。
func newCaptchaTestCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/auth/login", nil)
	return c
}

// TestVerifyCaptchaCorrectPasses 覆盖「验证码正确则放行」：
// 生成验证码后按接口返回的答案提交，必须校验通过。
func TestVerifyCaptchaCorrectPasses(t *testing.T) {
	c := newCaptchaTestCtx()
	id, code, img := newCaptcha(120, c.ClientIP())
	if id == "" || code == "" || img == "" {
		t.Fatalf("验证码生成不完整：id=%q code=%q imgLen=%d", id, code, len(img))
	}
	if !verifyCaptcha(c, id, code) {
		t.Fatal("验证码答案正确但校验未通过（应放行）")
	}
}

// TestVerifyCaptchaWrongAndReuseRejected 校验错误答案与一次性使用语义。
func TestVerifyCaptchaWrongAndReuseRejected(t *testing.T) {
	c := newCaptchaTestCtx()
	id, code, _ := newCaptcha(120, c.ClientIP())
	if verifyCaptcha(c, id, code+"0") {
		t.Fatal("错误验证码不应通过")
	}
	// 上面的失败已消耗该验证码，正确答案也不能再次使用
	if verifyCaptcha(c, id, code) {
		t.Fatal("验证码应一次性使用，重复提交不应通过")
	}
}
