package config

import (
	"testing"
)

// loadForTest 在隔离的临时目录（无 .env）加载配置，避免读取仓库真实配置。
func loadForTest(t *testing.T) *Config {
	t.Helper()
	t.Setenv("DB_USER", "drill_app")
	t.Setenv("JWT_SECRET", "test-jwt-secret")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	return Load(t.TempDir())
}

// TestCaptchaTestModeOnlyInTestEnv 验证码联调开关只允许 APP_ENV=test 生效。
func TestCaptchaTestModeOnlyInTestEnv(t *testing.T) {
	t.Run("test 环境生效", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		t.Setenv("CAPTCHA_TEST_MODE", "true")
		cfg := loadForTest(t)
		if !cfg.CaptchaTestMode {
			t.Fatal("APP_ENV=test 且 CAPTCHA_TEST_MODE=true 时应开启联调模式")
		}
	})

	t.Run("生产环境忽略开关", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("CAPTCHA_TEST_MODE", "true")
		cfg := loadForTest(t)
		if cfg.CaptchaTestMode {
			t.Fatal("生产环境必须忽略 CAPTCHA_TEST_MODE，不能返回 testCode")
		}
		if !cfg.CaptchaEnabled {
			t.Fatal("生产环境验证码校验默认必须开启")
		}
	})

	t.Run("默认关闭", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("CAPTCHA_TEST_MODE", "")
		cfg := loadForTest(t)
		if cfg.CaptchaTestMode {
			t.Fatal("未配置 CAPTCHA_TEST_MODE 时联调模式必须关闭")
		}
	})
}
