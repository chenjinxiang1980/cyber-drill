package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"drill-system/internal/db"
)

// ParseLocalTime 导出版 parseLocalTime（供 verify 工具调用）
func ParseLocalTime(s string) int64 {
	return parseLocalTime(s)
}

// Exported wrappers for verify tools — just delegate to internal funcs

// ElapsedSecExport 直接返回秒数（而非 *int64）；未开始返回 -1
func ElapsedSecExport(startedAt, pausedAt string, pausedSec int, endedAt, status string) int64 {
	if strings.TrimSpace(startedAt) == "" {
		return -1
	}
	v := elapsedSecLite(startedAt, pausedAt, pausedSec, endedAt, status)
	if v == nil {
		return -1
	}
	return *v
}

// eventAlias — 事件名 → dict.notif_templates 模板键别名映射
// dict 里的模板键是 {drill_start, drill_end, drill_end_score, node_done, node_submit, node_skip, auto_eval, project_created}
var eventAlias = map[string]string{
	"project_created": "project_created", // 直接匹配
	"project_started": "drill_start",     // 演练开始 → drill_start
	"project_paused":  "",                // dict 暂无 paused 模板 → 兜底
	"project_resumed": "",                // dict 暂无 resumed 模板 → 兜底
	"project_ended":   "drill_end",       // 演练结束 → drill_end
	"node_submit":     "node_submit",     // 直接匹配
	"node_skip":       "node_skip",       // 直接匹配
	"node_done":       "node_done",       // 直接匹配
	"auto_eval":       "auto_eval",       // 直接匹配
	"drill_start":     "drill_start",     // 直接匹配（演练开始/继续）
	"drill_end":       "drill_end",       // 直接匹配（状态改为已结束）
}

// RenderMessageDB 带 DB 的版本：优先读 dict.notif_templates，命中则按模板渲染 title + desc；
// 读不到（无 DB / 模板键缺失 / JSON 解析失败）则走内置兜底。第二个返回值 fromDB 表示是否命中 DB 模板。
func RenderMessageDB(database *db.DB, event string, vars map[string]any) (string, bool) {
	// 1. 事件名 → dict 模板键
	tmplKey, ok := eventAlias[event]
	if !ok || tmplKey == "" {
		return renderFallback(event, vars), false
	}

	// 2. 读 dict 表（nil DB → 直接兜底）
	if database == nil {
		return renderFallback(event, vars), false
	}
	var val sql.NullString
	err := database.QueryRow(`SELECT value FROM dict WHERE ` + "`key`" + `='notif_templates'`).Scan(&val)
	if err != nil || !val.Valid || strings.TrimSpace(val.String) == "" {
		return renderFallback(event, vars), false
	}

	// 3. 解析 JSON
	var tmpls map[string]json.RawMessage
	if err := json.Unmarshal([]byte(val.String), &tmpls); err != nil {
		return renderFallback(event, vars), false
	}
	raw, hit := tmpls[tmplKey]
	if !hit {
		return renderFallback(event, vars), false
	}
	var tmpl struct {
		Title  string `json:"title"`
		Desc   string `json:"desc"`
		Source string `json:"source"`
		Level  string `json:"level"`
	}
	if err := json.Unmarshal(raw, &tmpl); err != nil {
		return renderFallback(event, vars), false
	}

	// 4. 变量填充：同时支持 vars 键（驼峰）和模板占位符
	vars2 := expandVars(vars)
	title := fillVars(tmpl.Title, vars2)
	desc := fillVars(tmpl.Desc, vars2)
	if desc != "" {
		return fmt.Sprintf("%s · %s", title, desc), true
	}
	return title, true
}

// RenderNotifTemplate 逐字段渲染站内通知（对齐 Node notify.js push 的 title/desc/source/level 分列）。
// RenderMessageDB 返回 "title · desc" 合并串，无法拆回 notifs 表的两列，故这里单独提供字段级渲染。
func RenderNotifTemplate(database *db.DB, event string, vars map[string]any) (title, descr, source, level string, fromDB bool) {
	tmplKey, ok := eventAlias[event]
	if !ok || tmplKey == "" || database == nil {
		return "", "", "", "", false
	}
	var val sql.NullString
	err := database.QueryRow(`SELECT value FROM dict WHERE ` + "`key`" + `='notif_templates'`).Scan(&val)
	if err != nil || !val.Valid || strings.TrimSpace(val.String) == "" {
		return "", "", "", "", false
	}
	var tmpls map[string]json.RawMessage
	if err := json.Unmarshal([]byte(val.String), &tmpls); err != nil {
		return "", "", "", "", false
	}
	// 兼容历史数据的外层 {"value":{...}} 包裹
	if inner, ok := tmpls["value"]; ok && len(tmpls) == 1 {
		var nested map[string]json.RawMessage
		if json.Unmarshal(inner, &nested) == nil {
			tmpls = nested
		}
	}
	raw, ok := tmpls[tmplKey]
	if !ok {
		return "", "", "", "", false
	}
	var tmpl struct {
		Title  string `json:"title"`
		Desc   string `json:"desc"`
		Source string `json:"source"`
		Level  string `json:"level"`
	}
	if err := json.Unmarshal(raw, &tmpl); err != nil {
		return "", "", "", "", false
	}
	v2 := expandVars(vars)
	return fillVars(tmpl.Title, v2), fillVars(tmpl.Desc, v2), tmpl.Source, tmpl.Level, true
}

// RenderMessage 无 DB 版本（兼容旧调用点）—— 始终走内置兜底
func RenderMessage(event string, vars map[string]any) (string, bool) {
	return renderFallback(event, vars), false
}

func renderFallback(event string, vars map[string]any) string {
	defaults := map[string]string{
		"project_created": fmt.Sprintf("演练项目「%s」已创建。", strVar(vars, "projectName")),
		"project_started": fmt.Sprintf("演练项目「%s」已开始，请相关人员按预案执行。", strVar(vars, "projectName")),
		"project_paused":  fmt.Sprintf("演练项目「%s」已暂停，原因：%s。", strVar(vars, "projectName"), strVar(vars, "detail")),
		"project_resumed": fmt.Sprintf("演练项目「%s」已继续。", strVar(vars, "projectName")),
		"project_ended":   fmt.Sprintf("演练项目「%s」已结束。详细处置情况请查看演练报告。", strVar(vars, "projectName")),
		"task_assigned":   fmt.Sprintf("您好 %s，任务「%s」已分配给您（角色：%s）。", strVar(vars, "userName"), strVar(vars, "taskTitle"), strVar(vars, "roleName")),
		"impact_changed":  fmt.Sprintf("业务系统状态变更：%s。详情：%s。", strVar(vars, "sys"), strVar(vars, "detail")),
	}
	if tmpl, ok := defaults[event]; ok {
		return tmpl
	}
	return fmt.Sprintf("事件通知 [%s]：%s", event, strVar(vars, "detail"))
}

// expandVars 把常见驼峰键映射到模板占位符命名（如 projectName → {project}）
func expandVars(vars map[string]any) map[string]any {
	out := make(map[string]any, len(vars)+8)
	for k, v := range vars {
		out[k] = v
	}
	aliases := map[string]string{
		"projectName": "project",
		"taskTitle":   "title",
		"taskId":      "taskId",
		"userName":    "user",
		"roleName":    "role",
		"eventType":   "action",
		"eventDetail": "detail",
		"impactSys":   "sys",
	}
	for src, dst := range aliases {
		if v, ok := out[src]; ok {
			out[dst] = v
		}
	}
	return out
}

// fillVars 把 {key} 占位符替换为 vars 值（找不到保留占位符）
func fillVars(tmpl string, vars map[string]any) string {
	out := tmpl
	for k, v := range vars {
		placeholder := "{" + k + "}"
		out = strings.ReplaceAll(out, placeholder, strVal(v))
	}
	return out
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func strVar(vars map[string]any, key string) string {
	if v, ok := vars[key]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}
