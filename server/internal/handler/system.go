package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// --- system 系统设置 ---

// brandDefaults 与 Node 版 brand.js 的 DEFAULTS 对齐
var brandDefaults = gin.H{
	"systemName": "网络安全应急演练系统",
	"unitName":   "示例医院",
	"logo":       "img/logo.png",
	"version":    "v0.5.0",
	"github":     "https://github.com/chenjinxiang1980/cyber-drill",
	"gitee":      "https://gitee.com/habeas/cyber-drill",
}

// brandKeyMap 与 Node 版 brand.js 的 KEYS 对齐：输出字段 → settings 表 key
var brandKeyMap = map[string]string{
	"systemName": "brand_system_name",
	"unitName":   "brand_unit_name",
	"logo":       "brand_logo",
	"version":    "brand_version",
	"github":     "brand_github",
	"gitee":      "brand_gitee",
}

var brandWriteKeys = []string{"systemName", "unitName", "logo", "version", "github", "gitee"}

// GetBrand GET /api/system/brand —— 免登录读取品牌设置
// Node 版 brand.readBrand(db) 返回 {systemName, unitName, logo, version, github, gitee}
func GetBrand(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		brand := gin.H{}
		for _, field := range brandWriteKeys {
			key := brandKeyMap[field]
			var v sql.NullString
			err := database.QueryRow(`SELECT value FROM settings WHERE `+"`key`"+`=?`, key).Scan(&v)
			if err != nil {
				// 键不存在 → fallback 默认值
				brand[field] = brandDefaults[field]
				continue
			}
			val := util.ScanNullableString(v)
			if val == "" {
				brand[field] = brandDefaults[field]
			} else {
				brand[field] = val
			}
		}
		c.JSON(200, brand)
	}
}

// PutBrand PUT /api/system/brand —— 品牌设置写（RequireBrandAdmin 由路由层挂）
// body 接受 brandWriteKeys 里的字段，映射到 settings 表正确的 key
func PutBrand(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body gin.H
		if err := c.ShouldBindJSON(&body); err != nil {
			util.BadRequest(c, "无效的请求体")
			return
		}
		tx, err := database.Begin()
		if err != nil {
			slog.Warn("PutBrand 开启事务失败", "err", err)
			util.Fail(c, 500, "保存失败")
			return
		}
		defer func() { _ = tx.Rollback() }()

		allowed := map[string]bool{}
		for _, k := range brandWriteKeys {
			allowed[k] = true
		}
		updated := 0
		for k, raw := range body {
			if !allowed[k] {
				continue
			}
			val := toString(raw)
			settingsKey := brandKeyMap[k]
			if _, err := tx.Exec(
				`INSERT INTO settings (`+"`key`"+`, value) VALUES (?,?)
				 ON DUPLICATE KEY UPDATE value=VALUES(value)`,
				settingsKey, val,
			); err != nil {
				slog.Warn("PutBrand 写入失败", "key", settingsKey, "err", err)
				util.Fail(c, 500, "保存 "+k+" 失败")
				return
			}
			updated++
		}
		if err := tx.Commit(); err != nil {
			slog.Warn("PutBrand 提交失败", "err", err)
			util.Fail(c, 500, "保存失败")
			return
		}
		c.JSON(200, gin.H{"ok": true, "updated": updated})
	}
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// isEmptyDictPayload 空保存防护：null / 空串 / {} / [] 一律视为空，拒绝写库
func isEmptyDictPayload(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		s := strings.TrimSpace(x)
		return s == "" || s == "{}" || s == "[]" || s == "null"
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	default:
		return false
	}
}

// GetStatus GET /api/system/status —— 聚合计数
// Node 版 seed.status() 返回 {initialized, userCount, roleCount, scenarioCount,
//   nodeCount, projectCount, brand_version, initialized_at, seedVersion, hasBaseline}
func GetStatus(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		count := func(table string) int64 {
			var n sql.NullInt64
			_ = database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
			if n.Valid {
				return n.Int64
			}
			return 0
		}

		userCount := count("users")
		roleCount := count("roles")
		scenarioCount := count("scenarios")
		nodeCount := count("script_nodes")
		projectCount := count("projects")
		initialized := roleCount > 0

		// 从 settings 读 brand_version 和 initialized_at
		var brandVersion string
		var initializedAt any
		if v, err := settingsValue(database, "brand_version"); err == nil && v != "" {
			brandVersion = v
		} else {
			brandVersion = "unknown"
		}
		if v, err := settingsValue(database, "initialized_at"); err == nil && v != "" {
			initializedAt = v
		}

		// seed_baseline: hasBaseline + seedVersion
		var baselineCount sql.NullInt64
		_ = database.QueryRow(`SELECT COUNT(*) FROM seed_baseline`).Scan(&baselineCount)
		hasBaseline := baselineCount.Valid && baselineCount.Int64 > 0
		var seedVersion any
		{
			var sv sql.NullString
			_ = database.QueryRow(`SELECT version FROM seed_baseline ORDER BY id DESC LIMIT 1`).Scan(&sv)
			v := util.ScanNullableString(sv)
			if v != "" {
				seedVersion = v
			}
		}

		c.JSON(200, gin.H{
			"initialized":    initialized,
			"userCount":      userCount,
			"roleCount":      roleCount,
			"scenarioCount":  scenarioCount,
			"nodeCount":      nodeCount,
			"projectCount":   projectCount,
			"brand_version":  brandVersion,
			"initialized_at": initializedAt,
			"hasBaseline":    hasBaseline,
			"seedVersion":    seedVersion,
		})
	}
}

// settingsValue 从 settings 表读取单个值，返回错误或空字符串
func settingsValue(database *db.DB, key string) (string, error) {
	var v sql.NullString
	err := database.QueryRow(`SELECT value FROM settings WHERE `+"`key`"+`=?`, key).Scan(&v)
	if err != nil {
		return "", err
	}
	return util.ScanNullableString(v), nil
}

// GetDrillDict GET /api/system/dict/:key —— 读取 dict 条目（JSON parse）
// Node 版 system.js 返回 {key, value}，不包 ok
func GetDrillDict(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := c.Param("key")
		var v sql.NullString
		err := database.QueryRow(`SELECT value FROM dict WHERE `+"`key`"+`=?`, k).Scan(&v)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(200, gin.H{"key": k, "value": nil})
				return
			}
			slog.Warn("GetDrillDict 失败", "key", k, "err", err)
			util.Fail(c, 500, "读取失败")
			return
		}
		raw := util.ScanNullableString(v)
		if raw == "" {
			c.JSON(200, gin.H{"key": k, "value": nil})
			return
		}
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			c.JSON(200, gin.H{"key": k, "value": parsed})
			return
		}
		c.JSON(200, gin.H{"key": k, "value": raw})
	}
}

// PutDrillDict PUT /api/system/dict/:key —— 写 dict 条目
// body 可为任意 JSON（存为 value TEXT）
// Node 版返回 {ok: true}
func PutDrillDict(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := c.Param("key")
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil || body == nil {
			util.BadRequest(c, "无效的 JSON 请求体")
			return
		}
		// 前端 public/api.js saveDict 提交的是 {value: <字典内容>}。
		// Node 版 system.js 取 req.body.value 后 JSON.stringify(value) 落库；
		// Go 版此前直接 toString(body)，导致库里多包一层 {"value":...}，页面读不到内容。
		payload, wrapped := body["value"]
		if !wrapped {
			payload = any(body) // 兼容直接提交裸对象
		}
		if isEmptyDictPayload(payload) {
			util.BadRequest(c, "字典内容为空，拒绝保存（防止清空配置）")
			return
		}
		val := toString(payload)
		if _, err := database.Exec(
			`INSERT INTO dict (`+"`key`"+`, value) VALUES (?,?)
			 ON DUPLICATE KEY UPDATE value=VALUES(value)`,
			k, val,
		); err != nil {
			slog.Warn("PutDrillDict 失败", "key", k, "err", err)
			util.Fail(c, 500, "保存失败")
			return
		}
		c.JSON(200, gin.H{"ok": true})
	}
}
