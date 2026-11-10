// Package config 负责从环境变量与 .env 文件加载配置。
// 所有配置项均有默认值或必填校验；启动时校验缺失即 Fatal。
package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func getEnvBool(k string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// Config 全部运行配置。字段全部与 .env 变量一一对应。
type Config struct {
	// 运行环境
	AppEnv string // development | production

	// HTTP
	HTTPAddr   string
	PublicBase string

	// MySQL
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// JWT
	JWTSecret string
	JWTTTL    time.Duration

	// Session
	SessionSecret string

	// HTTPS/TLS
	TLSEnabled          bool
	TLSAddr             string // ":8443" — 显式开关时使用，覆盖 HTTPAddr
	HTTPSCertFile       string
	HTTPSKeyFile        string
	HTTPRedirectToHTTPS bool // true = 同时起 HTTP 做 301 跳转

	// 上传
	UploadRoot  string
	UploadMaxMB int

	// 数据目录（备份/恢复等文件存储位置，相对于 projectRoot）
	DataDir string

	// 备份与恢复
	BackupPassphrase   string // BACKUP_PASSPHRASE：备份加密口令（AES-256），绝不写入日志与备份
	RestorePassword    string // RESTORE_PASSWORD：独立恢复口令，与登录口令分离
	AuditRetentionDays int    // AUDIT_RETENTION_DAYS：审计保留天数，0 = 不清理

	// 限流
	RateLimitGlobal  int // req/min（仅 /api/*）
	RateLimitAuth    int
	RateLimitScreen  int // req/min（大屏匿名只读接口，单独计数）
	RateLimitCaptcha int // req/min（验证码接口，单独限流）

	// 大屏免登录观摩：true = 未登录可只读访问 /api/screen/*，false = 恢复必须登录
	ScreenPublicView bool

	// 登录失败锁定
	LoginMaxFailures int // 连续失败次数阈值
	LoginLockMinutes int // 锁定时长（分钟）

	// 验证码
	CaptchaEnabled    bool
	CaptchaTTLSeconds int

	// CORS
	AllowOrigins []string

	// 可信代理（Gin 用）
	TrustedProxies []string
}

// Load 从 .env（存在则读）+ 环境变量（覆盖）加载，然后做必填校验。
// 任一必填项缺失时 log.Fatalf 退出。
func Load(projectRoot string) *Config {
	// 1. 尝试 .env（不存在静默跳过）
	envFile := filepath.Join(projectRoot, ".env")
	if f, err := os.Open(envFile); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// 去掉行内 # 注释（引号内的 # 不处理，.env.example 无这种复杂值）
			if idx := strings.Index(line, "#"); idx > 0 {
				line = strings.TrimSpace(line[:idx])
			}
			eq := strings.Index(line, "=")
			if eq <= 0 {
				continue
			}
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			// 去掉首尾引号
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" { // 环境变量优先级更高
				os.Setenv(k, v)
			}
		}
	}

	c := &Config{
		AppEnv:              getEnv("APP_ENV", "development"),
		HTTPAddr:            getEnv("HTTP_ADDR", "0.0.0.0:8090"),
		PublicBase:          strings.TrimRight(getEnv("PUBLIC_BASE_URL", ""), "/"),
		DBHost:              getEnv("DB_HOST", "127.0.0.1"),
		DBPort:              getEnvInt("DB_PORT", 3306),
		DBName:              getEnv("DB_NAME", "drill_system"),
		DBUser:              getEnv("DB_USER", ""),
		DBPassword:          getEnv("DB_PASSWORD", ""),
		DBSSLMode:           getEnv("DB_SSLMODE", "disable"),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		JWTTTL:              time.Duration(getEnvInt("JWT_TTL", 480)) * time.Minute,
		SessionSecret:       getEnv("SESSION_SECRET", ""),
		TLSEnabled:          getEnvBool("TLS_ENABLED", false),
		TLSAddr:             getEnv("TLS_ADDR", ":8443"),
		HTTPSCertFile:       getEnv("HTTPS_CERT_FILE", ""),
		HTTPSKeyFile:        getEnv("HTTPS_KEY_FILE", ""),
		HTTPRedirectToHTTPS: getEnvBool("HTTP_REDIRECT_TO_HTTPS", false),
		UploadRoot:          getEnv("UPLOAD_ROOT", "uploads"),
		DataDir:             getEnv("DATA_DIR", "data"),
		BackupPassphrase:    getEnv("BACKUP_PASSPHRASE", ""),
		RestorePassword:     getEnv("RESTORE_PASSWORD", ""),
		AuditRetentionDays:  getEnvInt("AUDIT_RETENTION_DAYS", 365),
		UploadMaxMB:         getEnvInt("UPLOAD_MAX_MB", 20),
		RateLimitGlobal:     getEnvInt("RATE_LIMIT_GLOBAL", 300),
		RateLimitAuth:       getEnvInt("RATE_LIMIT_AUTH", 30),
		RateLimitScreen:     getEnvInt("RATE_LIMIT_SCREEN", 60),
		RateLimitCaptcha:    getEnvInt("RATE_LIMIT_CAPTCHA", 20),
		ScreenPublicView:    getEnvBool("SCREEN_PUBLIC_VIEW", true),
		LoginMaxFailures:    getEnvInt("LOGIN_MAX_FAILURES", 5),
		LoginLockMinutes:    getEnvInt("LOGIN_LOCK_MINUTES", 15),
		CaptchaEnabled:      getEnvBool("CAPTCHA_ENABLED", true),
		CaptchaTTLSeconds:   getEnvInt("CAPTCHA_TTL_SECONDS", 120),
		// 默认不允许跨域：前后端同源部署；确需跨域时显式配置 ALLOW_ORIGINS。
		AllowOrigins:   splitCSV(getEnv("ALLOW_ORIGINS", "")),
		TrustedProxies: splitCSV(getEnv("TRUSTED_PROXIES", "")),
	}

	if c.AuditRetentionDays < 0 {
		c.AuditRetentionDays = 0
	}

	// 2. 校验必填项
	var missing []string
	if c.DBUser == "" {
		missing = append(missing, "DB_USER")
	}
	if c.JWTSecret == "" || strings.Contains(c.JWTSecret, "REPLACE_WITH") {
		missing = append(missing, "JWT_SECRET (openssl rand -hex 32 生成)")
	}
	if c.SessionSecret == "" || strings.Contains(c.SessionSecret, "REPLACE_WITH") {
		missing = append(missing, "SESSION_SECRET (openssl rand -hex 32 生成)")
	}
	if len(missing) > 0 {
		slog.Error("配置缺失或未替换占位符", "missing", missing)
		os.Exit(2)
	}

	// 3. TLS 强制校验：开启时证书必须存在，拒绝静默降级
	if c.TLSEnabled {
		if c.HTTPSCertFile == "" || c.HTTPSKeyFile == "" {
			slog.Error("TLS_ENABLED=true 但 HTTPS_CERT_FILE / HTTPS_KEY_FILE 未配置",
				"hint", "设置证书路径，或关闭 TLS_ENABLED=false 使用 HTTP")
			os.Exit(2)
		}
		if _, err := os.Stat(c.HTTPSCertFile); err != nil {
			slog.Error("TLS 证书文件不存在", "file", c.HTTPSCertFile, "err", err)
			os.Exit(2)
		}
		if _, err := os.Stat(c.HTTPSKeyFile); err != nil {
			slog.Error("TLS 私钥文件不存在", "file", c.HTTPSKeyFile, "err", err)
			os.Exit(2)
		}
	}

	if c.PublicBase == "" {
		c.PublicBase = fmt.Sprintf("http://localhost:%s", portOf(c.HTTPAddr))
	}

	slog.Info("配置加载完成",
		"app_env", c.AppEnv,
		"http", c.HTTPAddr,
		"db", fmt.Sprintf("%s@%s:%d/%s", c.DBUser, c.DBHost, c.DBPort, c.DBName),
		"jwt_ttl", c.JWTTTL,
		"upload_max_mb", c.UploadMaxMB,
		"cors_origins", len(c.AllowOrigins),
		"screen_public_view", c.ScreenPublicView,
		"rate_limit_screen", c.RateLimitScreen,
	)
	return c
}

// MySQL DSN（go-sql-driver/mysql 格式）
func (c *Config) DSN() string {
	// parseTime=true 至关重要：让 DATETIME/TIMESTAMP 自动转 time.Time，配合 loc=Local 保持本地时区语义
	// interpolateParams=true 由 driver 自动转义，减少预编译语句数量
	// charset=utf8mb4 确保 emoji 等 4 字节字符正常存储
	params := url.Values{}
	params.Set("parseTime", "true")
	params.Set("loc", "Local")
	params.Set("charset", "utf8mb4")
	params.Set("multiStatements", "true")

	// go-sql-driver/mysql tls 参数：false 或 true 或 自定义配置名
	// 不是 MySQL 的 disable/verify-ca 等 openssl 风格
	if strings.EqualFold(c.DBSSLMode, "disable") || c.DBSSLMode == "" {
		params.Set("tls", "false")
	} else {
		// require / verify-ca / verify-full → MySQL driver 自定义 TLS 配置
		// 先当作 "custom" 占位，生产需注册 tls.Config
		params.Set("tls", c.DBSSLMode)
	}

	// mysql://user:pwd@host:port/db?params
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, params.Encode())
	return dsn
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func portOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return "8090"
}
