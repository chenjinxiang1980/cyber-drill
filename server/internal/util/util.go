// Package util —— 通用工具：JSON 解析、时间格式化、错误响应辅助。
package util

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ParseJSON 把数据库 TEXT 列（存 JSON 字符串）安全解析到目标指针。
// Node 版 db.js 的 parseJson 默认 {} / []，这里保持同样语义。
//   - defaultValue 为 nil 时返回 nil 指针
//   - 否则返回 defaultValue 的深拷贝（如果解析失败）
func ParseJSON(raw string, target any, defaultValue any) error {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		if defaultValue == nil {
			return nil
		}
		return copyDefault(defaultValue, target)
	}
	if err := json.Unmarshal([]byte(s), target); err != nil {
		if defaultValue != nil {
			return copyDefault(defaultValue, target)
		}
		return err
	}
	return nil
}

// ParseJSONArr 快捷：解析 JSON 数组到 []string
func ParseJSONArr(raw string) []string {
	var out []string
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return nil
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// ParseJSONArrAny 快捷：解析 JSON 数组到 []any（兼容对象数组、字符串数组、数字数组等）
// 与 Node 版 db.js 的 parseJson(s, []) 对齐——返回原始解析后的数组。
// 空字符串/null 返回空数组 []（不是 nil，让 JSON 序列化输出 [] 而非 null）。
func ParseJSONArrAny(raw string) []any {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return []any{}
	}
	var out []any
	_ = json.Unmarshal([]byte(s), &out)
	// 如果解析失败也返回空数组
	if out == nil {
		return []any{}
	}
	return out
}

// ParseJSONObj 快捷：解析 JSON 对象到 map[string]any
func ParseJSONObj(raw string) map[string]any {
	var out map[string]any
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" {
		return nil
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// ToJSON 序列化为 JSON 字符串（写库用）
func ToJSON(v any) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func copyDefault(def any, target any) error {
	b, err := json.Marshal(def)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

// ---- 时间 ----

// Now 本地时间 "2006-01-02 15:04:05" （db.Now() 用的是同一个格式）
func Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// NowSec 同 Now，但名字对齐 Node util.nowSec（供 handler 复用同样的命名）
func NowSec() string { return Now() }

// HHMM 仅时分 "15:04" （Node 版 hhmm()）
func HHMM() string {
	return time.Now().Format("15:04")
}

// DateStr 仅日期 "2006-01-02"
func DateStr() string {
	return time.Now().Format("2006-01-02")
}

// ParseMySQLTime VARCHAR(19) "2006-01-02 15:04:05" → time.Time（本地时区）
func ParseMySQLTime(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
}

// ParseLocal 与 Node drill_clock.parseLocal 对齐：
// 支持 "2006-01-02 15:04:05"、"2006-01-02 15:04"、"15:04"、""
// 返回毫秒时间戳（Node Date.now() 单位），无效返回 NaN float64
func ParseLocal(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return float64(t.UnixMilli())
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return float64(t.UnixMilli())
	}
	if m, err := time.ParseInLocation("15:04", s, time.Local); err == nil {
		now := time.Now()
		t := time.Date(now.Year(), now.Month(), now.Day(), m.Hour(), m.Minute(), 0, 0, time.Local)
		return float64(t.UnixMilli())
	}
	return 0
}

// SecBetween 两时间字符串相差的秒数（abs），0 表示解析失败或相等
func SecBetween(a, b string) float64 {
	aa := ParseLocal(a)
	bb := ParseLocal(b)
	if aa == 0 || bb == 0 {
		return 0
	}
	d := aa - bb
	if d < 0 {
		d = -d
	}
	return d / 1000
}

// ElapsedSec 项目真实计时（started_at → paused_at/ended_at），与 Node drill_clock.elapsedSec 一致
// 规则：
//
//	status 未开始 或 started_at 为空/解析失败 → nil（JSON null）
//	结束时刻优先级：ended_at → paused_at（仅 status=暂停）→ 当前时间
//	结果 = 结束 − 开始 − paused_sec，负数按 0
func ElapsedSec(startedAt, pausedAt, endedAt string, pausedSec int64, status string) any {
	if status == "未开始" {
		return nil
	}
	if startedAt == "" {
		return nil
	}
	startMs := ParseLocal(startedAt)
	if startMs == 0 {
		return nil
	}
	// 对齐 Node drill_clock.js:16-19：结束时刻 > 暂停时刻（仅暂停态）> 当前时间
	var endMs float64
	if endedAt != "" {
		endMs = ParseLocal(endedAt)
	} else if pausedAt != "" && status == "暂停" {
		endMs = ParseLocal(pausedAt)
	} else {
		endMs = float64(time.Now().UnixMilli())
	}
	if endMs == 0 {
		endMs = float64(time.Now().UnixMilli())
	}
	ms := endMs - startMs - float64(pausedSec)*1000
	if ms < 0 {
		ms = 0
	}
	return int64(ms / 1000)
}

// ---- 安全 scan 辅助 ----

// ScanNullableString 从 sql.NullString 提取值（MySQL 中 NOT NULL 列也可以用）
func ScanNullableString(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}

// ---- 响应辅助 ----

// Error 统一错误响应（所有 handler 必须走这个，确保 {error: "..."} 格式）
func Error(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// Errorf 带 format 的错误响应
func Errorf(c *gin.Context, code int, format string, args ...any) {
	c.AbortWithStatusJSON(code, gin.H{"error": fmt.Sprintf(format, args...)})
}

// OK / OKJSON 已移至 response.go（response.OK(c, data)），
// 保留此段注释避免旧引用混淆。util.OK/OKJSON 签名与 response.OK 冲突，故移除。
