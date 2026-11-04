// Package auth —— JWT 签发校验 + scrypt 密码散列 + 三重令牌来源。
//
// 设计说明：
//  1. 令牌是 JWT（满足用户要求），payload 含 userId/roleId/exp。
//  2. 签发后在 sessions 表写一条（sessions.token = JWT 字符串），用于吊销、
//     过期清理、userByToken 查用户信息——与 Node 版 sessions 表保持语义一致。
//  3. 三重来源按优先级：Authorization Bearer > X-Drill-Token header > ?t= query。
//  4. 密码用 scrypt，参数 N=16384, r=8, p=1, length=32 —— 与 Node crypto.scryptSync 默认参数完全一致，
//     直接兼容现有 drill.db 里的 pwd_salt + pwd_hash。
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"drill-system/internal/config"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/scrypt"
)

// User 当前会话绑定的用户信息（从 sessions JOIN users JOIN roles 读取）
type User struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Org        string `json:"org"`
	Status     string `json:"status"`
	MustChange bool   `json:"mustChange"`
	Roles      []string `json:"roles"` // 全部角色 id 列表
	Role       Role   `json:"role"` // 当前会话角色
}

// Role 角色视图（与 Node 版返回字段对齐）
type Role struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Short    string   `json:"short"`
	Icon     string   `json:"icon"`
	Color    string   `json:"color"`
	Desc     string   `json:"desc"`
	Nav      []string `json:"nav"`
	External bool     `json:"external"`
	Status   string   `json:"-"` // 内部使用（角色已停用校验），不暴露给前端
}

// ContextKey 中间件向 gin.Context 写入的用户 key
const ContextKey = "drillUser"

// Service 认证服务（绑定 DB + 配置）
type Service struct {
	db  *db.DB
	cfg *config.Config
}

func NewService(database *db.DB, cfg *config.Config) *Service {
	return &Service{db: database, cfg: cfg}
}

// ---- 密码 ----
// ╔══════════════════════════════════════════════════════════════════════════════╗
// ║  SALT 格式铁律（禁止破坏）                                                    ║
// ║                                                                              ║
// ║  drill.db / MySQL 的 pwd_salt 列存的是 32 个十六进制字符（VARCHAR）。          ║
// ║  Node crypto.scryptSync(pwd, saltStr, 32) 直接拿这个字符串当 UTF-8 喂 scrypt    ║
// ║  → Go 必须用 []byte(saltHex) 直传（= UTF-8 32 字节），                          ║
// ║    绝对禁止 hex.DecodeString(saltHex)（那会变成 16 字节，存量 hash 验不过）。     ║
// ║                                                                              ║
// ║  证据链：drill.db admin(初始密码占位, salt=... 参考注释)                           ║
// ║    Go VerifyPassword → PASS  ✅                                               ║
// ║    Node verifyPassword → PASS ✅                                               ║
// ║  ⚠️  Phase 1 报告里"salt 长度 16 bytes"是错误结论，已纠正。                     ║
// ║                                                                              ║
// ║  新用户注册一律按这里生成：16 字节随机数 → hex 编码(32 字符) → []byte 直传      ║
// ╚══════════════════════════════════════════════════════════════════════════════╝

// HashPassword scrypt 散列，返回 salt(32 字符 hex 字符串) + hash(64 字符 hex)
// 参数 N=16384, r=8, p=1, dkLen=32 —— Node 默认值。
func HashPassword(pwd string) (salt, hash string, err error) {
	saltB := make([]byte, 16)
	if _, err = rand.Read(saltB); err != nil {
		return "", "", err
	}
	salt = hex.EncodeToString(saltB) // 32 字符 hex 字符串
	// 关键：salt 直接用 hex 字符串的 UTF-8 字节（32 字节），
	// 不做 hex 解码——与 Node scryptSync(String(pwd), salt, 32) 对齐
	hashB, err := scrypt.Key([]byte(pwd), []byte(salt), 16384, 8, 1, 32)
	if err != nil {
		return "", "", err
	}
	return salt, hex.EncodeToString(hashB), nil
}

// VerifyPassword 与 Node crypto.timingSafeEqual 对齐
// saltHex 是 hex 字符串（32 字符），直接用 UTF-8 编码喂 scrypt.Key
func VerifyPassword(pwd, saltHex, hashHex string) bool {
	// 关键：salt 不 hex 解码，直接 []byte(saltHex) → 32 字节 UTF-8
	want, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}
	got, err := scrypt.Key([]byte(pwd), []byte(saltHex), 16384, 8, 1, 32)
	if err != nil || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---- JWT ----

// tokenClaims JWT payload
type tokenClaims struct {
	jwt.RegisteredClaims
	UserID int64  `json:"uid"`
	RoleID string `json:"rid"`
}

// Issue 签发 JWT + 写 sessions 表。返回 (JWT token, error)
func (s *Service) Issue(userID int64, roleID string) (string, error) {
	now := time.Now()
	exp := now.Add(s.cfg.JWTTTL)
	claims := tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Subject:   fmt.Sprintf("%d:%s", userID, roleID),
		},
		UserID: userID,
		RoleID: roleID,
	}
	tokenStr := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tokenStr.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("JWT 签发失败: %w", err)
	}

	// sessions 表（用于吊销、过期清理、userByToken 查用户）
	if _, err := s.db.Exec(
		`INSERT INTO sessions (token,user_id,role_id,created_at,expires_at) VALUES (?,?,?,?,?)`,
		signed, userID, roleID, db.Now(), exp.Format("2006-01-02 15:04:05"),
	); err != nil {
		return "", fmt.Errorf("写入 sessions 表失败: %w", err)
	}
	return signed, nil
}

// Parse 校验 JWT 签名与 exp
func (s *Service) Parse(tokenStr string) (*tokenClaims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &tokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非 HMAC 签名: %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := tok.Claims.(*tokenClaims)
	if !ok || !tok.Valid {
		return nil, errors.New("JWT 无效")
	}
	return c, nil
}

// ---- 令牌来源解析 ----

// TokenOf 按 Node 版 auth.tokenOf 优先级从 gin.Context 取令牌
//   1. Authorization: Bearer X
//   2. X-Drill-Token: X
//   3. Query ?t=X  （文件下载场景用这个，因为 <a download> 无 header）
func TokenOf(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if h := c.GetHeader("X-Drill-Token"); h != "" {
		return strings.TrimSpace(h)
	}
	if q := c.Query("t"); q != "" {
		return strings.TrimSpace(q)
	}
	return ""
}

// ---- 用户 ----

// UserByToken 按令牌查当前用户（JWT 校验 + sessions 表 + user_roles 绑定校验）
func (s *Service) UserByToken(tokenStr string) (*User, error) {
	if tokenStr == "" {
		return nil, errors.New("empty token")
	}
	// JWT 签名与 exp 校验（claims 结果已足够，不往下用）
	_, err := s.Parse(tokenStr)
	if err != nil {
		return nil, err
	}

	// sessions 表必须有这条记录（用于吊销机制：logout 直接删 sessions 行，
	// 下次带同样 JWT 来虽然签名对但 sessions 找不到 → 拒绝）
	var sessToken, sessRoleID string
	var sessUID int64
	var sessExpires string
	err = s.db.QueryRow(
		`SELECT token,user_id,role_id,expires_at FROM sessions WHERE token=?`, tokenStr,
	).Scan(&sessToken, &sessUID, &sessRoleID, &sessExpires)
	if err == sql.ErrNoRows {
		return nil, errors.New("会话不存在或已注销")
	}
	if err != nil {
		return nil, err
	}

	// 会话过期兜底（JWT exp 已经校验，但 sessions 表可能更严格）
	if sessExpires != "" {
		if expT, e := time.ParseInLocation("2006-01-02 15:04:05", sessExpires, time.Local); e == nil && expT.Before(time.Now()) {
			s.db.Exec(`DELETE FROM sessions WHERE token=?`, tokenStr)
			return nil, errors.New("会话已过期")
		}
	}

	// 取用户
	var u User
	err = s.db.QueryRow(
		`SELECT u.id,u.username,u.name,u.org,u.status,u.must_change FROM users u WHERE u.id=?`,
		sessUID,
	).Scan(&u.ID, &u.Username, &u.Name, &u.Org, &u.Status, &u.MustChange)
	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, err
	}
	if u.Status != "启用" {
		return nil, errors.New("账号已停用")
	}

	// 会话角色优先，否则用户主角色
	activeRoleID := sessRoleID
	if activeRoleID == "" {
		var mainRID string
		if s.db.QueryRow(`SELECT role_id FROM users WHERE id=?`, u.ID).Scan(&mainRID) == nil {
			activeRoleID = mainRID
		}
	}

	// 角色已绑定校验（user_roles）
	var held int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM user_roles WHERE user_id=? AND role_id=?`, u.ID, activeRoleID,
	).Scan(&held); err != nil || held == 0 {
		s.db.Exec(`DELETE FROM sessions WHERE token=?`, tokenStr)
		return nil, errors.New("账号已不具备该角色")
	}

	// 当前会话角色信息
	r := queryRole(s.db, activeRoleID)
	if r == nil {
		return nil, errors.New("角色不存在")
	}
	u.Role = *r
	if (r.Status != "" && r.Status != "启用") {
		s.db.Exec(`DELETE FROM sessions WHERE token=?`, tokenStr)
		return nil, errors.New("角色已停用")
	}

	// 全部角色列表
	rows, _ := s.db.Query(`SELECT role_id FROM user_roles WHERE user_id=?`, u.ID)
	defer rows.Close()
	for rows.Next() {
		var rid string
		if rows.Scan(&rid) == nil {
			u.Roles = append(u.Roles, rid)
		}
	}

	return &u, nil
}

func queryRole(d *db.DB, roleID string) *Role {
	r := &Role{}
	var navStr sql.NullString
	var external sql.NullInt64
	var status sql.NullString
	var icon, color, descr, short, name sql.NullString
	err := d.QueryRow(
		"SELECT name,short,icon,color,descr,nav,`external`,status FROM roles WHERE id=?", roleID,
	).Scan(&name, &short, &icon, &color, &descr, &navStr, &external, &status)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		slog.Warn("queryRole 失败", "err", err)
		return nil
	}
	r.ID = roleID
	r.Name = name.String
	r.Short = short.String
	r.Icon = icon.String
	r.Color = color.String
	r.Desc = descr.String
	r.External = external.Int64 == 1
	if navStr.Valid && navStr.String != "" {
		// 简单解析 JSON 数组
		r.Nav = parseStringArray(navStr.String)
	}
	if status.Valid {
		r.Status = status.String
	}
	return r
}

func parseStringArray(s string) []string {
	// 简单 JSON 数组解析（不引入额外依赖）
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return nil
	}
	// 去掉头尾的 []
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- 会话管理 ----

// Destroy 删除 sessions 表中的令牌行（退出登录）
func (s *Service) Destroy(tokenStr string) {
	if tokenStr == "" {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token=?`, tokenStr); err != nil {
		slog.Warn("Destroy session 失败", "err", err)
	}
}

// CleanExpired 删除已过期 sessions 行
func (s *Service) CleanExpired() {
	cutoff := time.Now().Format("2006-01-02 15:04:05")
	if n, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, cutoff); err == nil {
		if rn, _ := n.RowsAffected(); rn > 0 {
			slog.Info("清理过期 sessions", "count", rn)
		}
	}
}

// ---- Gin 中间件 ----

// RequireAuth 强制认证。失败返回 401。
func (s *Service) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := TokenOf(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录，请先登录"})
			return
		}
		u, err := s.UserByToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期，请重新登录"})
			return
		}
		c.Set(ContextKey, u)
		c.Next()
	}
}

// CurrentUser 从 gin.Context 取当前用户（中间件之后调用）
func CurrentUser(c *gin.Context) *User {
	v, ok := c.Get(ContextKey)
	if !ok {
		return nil
	}
	u, _ := v.(*User)
	return u
}

// RequireRole 角色白名单。未在列表内返回 403。
// 空列表 = 只要求已认证（与 RequireAuth 等价）。
func (s *Service) RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if len(allowed) == 0 {
			c.Next()
			return
		}
		for _, r := range allowed {
			if u.Role.ID == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "当前角色无该权限"})
	}
}
