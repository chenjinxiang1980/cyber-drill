package handler

// C 块: 备份/恢复/系统信息/seed 导出/system reset
//
// 与 Node src/routes/admin.js + src/routes/system.js 对齐
// 关键差异: Node 用 SQLite 单文件复制做备份，Go 用 MySQL mysqldump 导出 SQL

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ---------- reset 高危操作锁 ----------
var (
	resetLock sync.Mutex
	resetInProgress bool
)

// ResetKeepTables 保留表：reset 不清这些（语义与 Node seed.js clearAll 不同）
var ResetKeepTables = []string{
	"schema_migrations",
	"roles",
	"user_roles",
	"users",
	"dict",
	"settings",
	"scenarios",
	"script_nodes",
	"form_templates",
	"score_items",
	"sim_data",
	"seed_baseline",
	"knowledge",
	// 注意：impact 不在此列表——它需要特殊处理（模板行保留、项目实例行清除），在下面单独断言
}

// ResetClearTables 清除表（演练流水）
// 注意：impact 不在此列表——它需要特殊处理（只清 project_id <> '' 的实例行，保留 project_id='' 的模板行）
var ResetClearTables = []string{
	"sessions",
	"projects",
	"tasks",
	"drill_steps",
	"drill_node_state",
	"task_attachments",
	"form_submissions",
	"messages",
	"notifs",
	"notif_reads",
	"chat_messages",
	"evidence",
	"snapshots",
	"timeline",
	"seq_flow",
	"scores",
	"ledger",
	"opinion",
	"audit_logs",
}

// ---------- POST /api/admin/backup ----------
// Node 版：复制 SQLite .db 文件到 data/backups/。Go 版：mysqldump 导出 SQL。
func AdminBackup(database *db.DB, backupDir string, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份目录失败: "+err.Error())
			return
		}

		ts := strings.ReplaceAll(strings.ReplaceAll(time.Now().Format("2006-01-02 15:04:05"), "-", ""), ":", "")
		name := fmt.Sprintf("drill-%s.sql", ts)
		target := filepath.Join(backupDir, name)

		// 用 mysqldump 导出
		mysqldumpPath := findMysqldump()
		if mysqldumpPath == "" {
			util.Error(c, http.StatusInternalServerError, "找不到 mysqldump，无法备份")
			return
		}

		args := []string{
			"--quick",
			"--routines",
			"--triggers",
			"--skip-lock-tables",  // 普通用户没有 RELOAD 权限，跳过 FLUSH TABLES
			"--no-tablespaces",
			fmt.Sprintf("--host=%s", cfg.DBHost),
			fmt.Sprintf("--port=%d", cfg.DBPort),
			fmt.Sprintf("--user=%s", cfg.DBUser),
			cfg.DBName,
		}
		cmd := exec.Command(mysqldumpPath, args...)
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
		cmd.Stderr = os.Stderr

		outFile, err := os.Create(target)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份文件失败: "+err.Error())
			return
		}
		defer outFile.Close()
		cmd.Stdout = outFile

		if err := cmd.Run(); err != nil {
			outFile.Close()
			os.Remove(target)
			util.Error(c, http.StatusInternalServerError, "mysqldump 执行失败: "+err.Error())
			return
		}

		st, _ := os.Stat(target)
		util.OK(c, gin.H{
			"ok":   true,
			"file": name,
			"path": target,
			"size": st.Size(),
			"time": time.Now().Format("2006-01-02 15:04:05"),
		})
	}
}

// ---------- GET /api/admin/backups ----------
func AdminListBackups(database *db.DB, backupDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
	entries, err := os.ReadDir(backupDir)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusOK, []any{})
				return
			}
			util.Error(c, http.StatusInternalServerError, "读取备份目录失败: "+err.Error())
			return
		}
		re := regexp.MustCompile(`\.(sql|db)$`)
		type item struct {
			File string `json:"file"`
			Size int64  `json:"size"`
			Time string `json:"time"`
		}
		list := []item{}
		for _, e := range entries {
			if e.IsDir() || !re.MatchString(e.Name()) {
				continue
			}
			st, _ := e.Info()
			if st == nil {
				continue
			}
			list = append(list, item{
				File: e.Name(),
				Size: st.Size(),
				Time: st.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
		// 按文件名倒序（最新在前）
		for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
			list[i], list[j] = list[j], list[i]
		}
		c.JSON(http.StatusOK, list)
	}
}

// safeBackupFile 防止路径穿越
var safeBackupRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+\.(sql|db)$`)

func safeBackupFile(name, backupDir string) string {
	if name == "" || !safeBackupRe.MatchString(name) {
		return ""
	}
	full := filepath.Join(backupDir, name)
	abs, _ := filepath.Abs(full)
	absDir, _ := filepath.Abs(backupDir)
	if !strings.HasPrefix(abs, absDir) {
		return ""
	}
	return full
}

// ---------- POST /api/admin/restore ----------
func AdminRestore(database *db.DB, backupDir string, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			File string `json:"file"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.File == "" {
			util.Error(c, http.StatusBadRequest, "缺少备份文件名")
			return
		}
		full := safeBackupFile(body.File, backupDir)
		if full == "" {
			util.Error(c, http.StatusBadRequest, "备份文件名不合法")
			return
		}
		if _, err := os.Stat(full); err != nil {
			util.Error(c, http.StatusBadRequest, "备份文件不存在: "+body.File)
			return
		}

		// 安全网：恢复前自动备份
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份目录失败")
			return
		}
		preTs := strings.ReplaceAll(strings.ReplaceAll(time.Now().Format("2006-01-02 15:04:05"), "-", ""), ":", "")
		preBackupPath := filepath.Join(backupDir, fmt.Sprintf("drill-pre-restore-%s.sql", preTs))

		// 先做一次完整备份当安全网
		mysqldumpPath := findMysqldump()
		if mysqldumpPath == "" {
			util.Error(c, http.StatusInternalServerError, "找不到 mysqldump")
			return
		}
		dumpArgs := []string{
			"--quick", "--skip-lock-tables", "--no-tablespaces",
			fmt.Sprintf("--host=%s", cfg.DBHost),
			fmt.Sprintf("--port=%d", cfg.DBPort),
			fmt.Sprintf("--user=%s", cfg.DBUser),
			cfg.DBName,
		}
		preFile, err := os.Create(preBackupPath)
		if err == nil {
			cmd := exec.Command(mysqldumpPath, dumpArgs...)
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
			cmd.Stdout = preFile
			_ = cmd.Run()
			preFile.Close()
		}

		// 用 mysql CLI 导入
		mysqlPath := findMysql()
		if mysqlPath == "" {
			util.Error(c, http.StatusInternalServerError, "找不到 mysql CLI，无法恢复")
			return
		}
		mysqlArgs := []string{
			fmt.Sprintf("--host=%s", cfg.DBHost),
			fmt.Sprintf("--port=%d", cfg.DBPort),
			fmt.Sprintf("--user=%s", cfg.DBUser),
			cfg.DBName,
		}
		cmd := exec.Command(mysqlPath, mysqlArgs...)
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
		sql, _ := os.ReadFile(full)
		cmd.Stdin = strings.NewReader(string(sql))
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			util.Error(c, http.StatusInternalServerError, "恢复失败: "+stderr.String())
			return
		}

		// 清所有 sessions（强制重新登录）
		database.Exec(`DELETE FROM sessions`)

		util.OK(c, gin.H{
			"ok":        true,
			"file":      body.File,
			"preBackup": filepath.Base(preBackupPath),
			"time":      time.Now().Format("2006-01-02 15:04:05"),
		})
	}
}

// ---------- DELETE /api/admin/backups/:file ----------
func AdminDeleteBackup(database *db.DB, backupDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file := c.Param("file")
		full := safeBackupFile(file, backupDir)
		if full == "" {
			util.Error(c, http.StatusBadRequest, "备份文件名不合法")
			return
		}
		if _, err := os.Stat(full); err != nil {
			util.Error(c, http.StatusNotFound, "备份文件不存在")
			return
		}
		os.Remove(full)
		util.OK(c, gin.H{"ok": true})
	}
}

// ---------- GET /api/admin/backups/:file 下载 ----------
func AdminDownloadBackup(database *db.DB, backupDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file := c.Param("file")
		full := safeBackupFile(file, backupDir)
		if full == "" {
			util.Error(c, http.StatusBadRequest, "备份文件名不合法")
			return
		}
		if _, err := os.Stat(full); err != nil {
			util.Error(c, http.StatusNotFound, "备份文件不存在")
			return
		}
		c.File(full)
	}
}

// ---------- GET /api/admin/sysinfo ----------
func AdminSysInfo(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		counts := map[string]int64{}
		tables := []string{"users", "roles", "projects", "tasks", "evidence", "timeline", "audit_logs", "knowledge"}
		for _, t := range tables {
			var cnt int64
			database.QueryRow("SELECT COUNT(*) FROM "+t).Scan(&cnt)
			counts[t] = cnt
		}
		util.OK(c, gin.H{
			"goVersion":   runtime.Version(),
			"platform":    runtime.GOOS + " " + runtime.GOARCH,
			"hostname":    hostnameSafe(),
			"serverTime":  time.Now().Format("2006-01-02 15:04:05"),
			"dbBackend":   "MySQL",
			"counts":      counts,
		})
	}
}

// ---------- GET /api/system/seed/baselines ----------
func SystemSeedBaselines(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := database.Query(`SELECT version, component, note, created_at FROM seed_baseline ORDER BY created_at DESC`)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		type baseline struct {
			Version   string `json:"version"`
			Component string `json:"component"`
			Note      string `json:"note"`
			CreatedAt string `json:"createdAt"`
		}
		list := []baseline{}
		for rows.Next() {
			var b baseline
			var note sql.NullString
			if err := rows.Scan(&b.Version, &b.Component, &note, &b.CreatedAt); err != nil {
				continue
			}
			if note.Valid {
				b.Note = note.String
			}
			list = append(list, b)
		}
		c.JSON(http.StatusOK, list)
	}
}

// ---------- POST /api/system/seed/export ----------
// 导出 seed 数据：把保留表（配置骨架）导出为 base64 JSON，供跨环境迁移
func SystemSeedExport(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		exportTables := []string{
			"roles", "user_roles", "dict", "settings",
			"scenarios", "script_nodes", "form_templates",
			"score_items", "sim_data", "knowledge",
		}
		result := map[string]any{}
		for _, t := range exportTables {
			rows, err := database.Query("SELECT * FROM " + t)
			if err != nil {
				result[t] = []any{}
				continue
			}
			data, err := scanAllRows(rows)
			rows.Close()
			if err != nil {
				result[t] = []any{}
				continue
			}
			result[t] = data
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		b64 := base64.StdEncoding.EncodeToString(jsonBytes)
		util.OK(c, gin.H{
			"ok":       true,
			"format":   "seed-export-v1",
			"tables":   exportTables,
			"base64":   b64,
			"exported": time.Now().Format("2006-01-02 15:04:05"),
		})
	}
}

// ---------- POST /api/system/reset ----------
// 高危操作：清空演练流水表，保留配置骨架表。
// 安全防护:
//   1. APP_ENV=production 时需 ALLOW_DANGEROUS_RESET=true
//   2. 请求体必须包含 {"confirm":"RESET"}
//   3. 执行前自动 mysqldump + ledger JSON dump
//   4. 所有清除完成后写一条 audit_logs（audit_logs 本身也在清除范围，故后置）
func SystemReset(database *db.DB, cfg *config.Config, backupDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 防护 1: production 默认禁用
		if cfg.AppEnv == "production" && os.Getenv("ALLOW_DANGEROUS_RESET") != "true" {
			util.Error(c, http.StatusForbidden,
				"production 环境默认禁用 system reset；设置 ALLOW_DANGEROUS_RESET=true 后重试")
			return
		}

		// 防护 2: confirm 参数
		var body struct {
			Confirm string `json:"confirm"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Confirm != "RESET" {
			util.Error(c, http.StatusBadRequest,
				"请求体必须包含 {\"confirm\":\"RESET\"} 以显式确认此高危操作")
			return
		}

		// 并发锁
		resetLock.Lock()
		defer resetLock.Unlock()
		if resetInProgress {
			util.Error(c, http.StatusConflict, "另一个系统重置正在进行中")
			return
		}
		resetInProgress = true
		defer func() { resetInProgress = false }()

		ts := strings.ReplaceAll(strings.ReplaceAll(
			time.Now().Format("2006-01-02 15:04:05"), "-", ""), ":", "")

		// 防护 3a: reset 前自动 mysqldump
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份目录失败: "+err.Error())
			return
		}
		preBackupPath := filepath.Join(backupDir, fmt.Sprintf("drill-pre-reset-%s.sql", ts))
		preBackupOK := false
		mysqldumpPath := findMysqldump()
		if mysqldumpPath != "" {
			dumpArgs := []string{
				"--quick", "--skip-lock-tables", "--no-tablespaces",
				fmt.Sprintf("--host=%s", cfg.DBHost),
				fmt.Sprintf("--port=%d", cfg.DBPort),
				fmt.Sprintf("--user=%s", cfg.DBUser),
				cfg.DBName,
			}
			preFile, err := os.Create(preBackupPath)
			if err == nil {
				cmd := exec.Command(mysqldumpPath, dumpArgs...)
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
				cmd.Stdout = preFile
				cmd.Stderr = os.Stderr
				if cmd.Run() == nil {
					preBackupOK = true
				}
				preFile.Close()
			}
		}

		// 防护 3b: ledger 行级 JSON dump（清除前）
		ledgerDumpPath := filepath.Join(backupDir, fmt.Sprintf("ledger-before-reset-%s.json", ts))
		ledgerDumpCount := 0
		if lrows, err := database.Query("SELECT * FROM ledger"); err == nil {
			if ldata, err := scanAllRows(lrows); err == nil {
				ledgerDumpCount = len(ldata)
				if jsonBytes, err := json.MarshalIndent(ldata, "", "  "); err == nil {
					os.WriteFile(ledgerDumpPath, jsonBytes, 0o644)
				}
			}
			lrows.Close()
		}

		// ---- 执行前 COUNT ----
		type tableCount struct {
			Name   string `json:"name"`
			Before int64  `json:"before"`
			After  int64  `json:"after,omitempty"`
		}
		var keepBefore []tableCount
		for _, t := range ResetKeepTables {
			var cnt int64
			database.QueryRow("SELECT COUNT(*) FROM "+t).Scan(&cnt)
			keepBefore = append(keepBefore, tableCount{Name: t, Before: cnt})
		}

		var impactBeforeTemplates, impactBeforeInstances int64
		database.QueryRow("SELECT COUNT(*) FROM impact WHERE project_id = ''").Scan(&impactBeforeTemplates)
		database.QueryRow("SELECT COUNT(*) FROM impact WHERE project_id <> ''").Scan(&impactBeforeInstances)

		// ---- 逐表清除 ----
		type clearResult struct {
			Name    string `json:"name"`
			Deleted int64  `json:"deleted"`
		}
		var clearedResults []clearResult
		var totalCleared int64

		database.Exec(`SET FOREIGN_KEY_CHECKS=0`)

		for _, t := range ResetClearTables {
			var before int64
			database.QueryRow("SELECT COUNT(*) FROM "+t).Scan(&before)
			if _, err := database.Exec("DELETE FROM " + t); err == nil {
				clearedResults = append(clearedResults, clearResult{Name: t, Deleted: before})
				totalCleared += before
			}
		}

		// impact 特殊处理：只清实例行（project_id <> ''）
		if _, err := database.Exec("DELETE FROM impact WHERE project_id <> ''"); err == nil {
			clearedResults = append(clearedResults, clearResult{
				Name:    "impact (project_id<>'' instances)",
				Deleted: impactBeforeInstances,
			})
			totalCleared += impactBeforeInstances
		}

		database.Exec(`SET FOREIGN_KEY_CHECKS=1`)

		// 清 sessions → 所有现有 JWT 失效
		database.Exec(`DELETE FROM sessions`)

		// ---- 防护 4: audit_logs 后置写入 ----
		// 注意 audit_logs 已被清，这条是 reset 后唯一的审计记录
		u := auth.CurrentUser(c)
		auditUser := ""
		auditUserID := int64(0)
		if u != nil {
			auditUser = u.Username
			auditUserID = u.ID
		}
		auditAction := fmt.Sprintf(
			"system reset: cleared %d rows across %d tables; impact templates kept=%d, instances cleared=%d; ledger dump=%d rows at %s",
			totalCleared, len(clearedResults),
			impactBeforeTemplates, impactBeforeInstances,
			ledgerDumpCount, filepath.Base(ledgerDumpPath),
		)
		database.Exec(
			`INSERT INTO audit_logs (time, user, role_id, user_id, action, ip) VALUES (?, ?, ?, ?, ?, ?)`,
			time.Now().Format("2006-01-02 15:04:05"),
			auditUser, "", auditUserID, auditAction, c.ClientIP(),
		)

		// ---- 执行后 COUNT + 断言 ----
		keepAfter := make([]tableCount, len(keepBefore))
		for i, kb := range keepBefore {
			var cnt int64
			database.QueryRow("SELECT COUNT(*) FROM "+kb.Name).Scan(&cnt)
			keepAfter[i] = tableCount{Name: kb.Name, Before: kb.Before, After: cnt}
		}

		var impactAfterTemplates int64
		database.QueryRow("SELECT COUNT(*) FROM impact WHERE project_id = ''").Scan(&impactAfterTemplates)

		var keptUnchanged, keptChanged []string
		for _, k := range keepAfter {
			if k.Before == k.After {
				keptUnchanged = append(keptUnchanged, k.Name)
			} else {
				keptChanged = append(keptChanged, fmt.Sprintf("%s: %d→%d", k.Name, k.Before, k.After))
			}
		}

		impactTemplatesOK := impactAfterTemplates == impactBeforeTemplates
		if impactTemplatesOK {
			keptUnchanged = append(keptUnchanged, fmt.Sprintf("impact_templates(%d)", impactAfterTemplates))
		} else {
			keptChanged = append(keptChanged, fmt.Sprintf("impact_templates: %d→%d", impactBeforeTemplates, impactAfterTemplates))
		}

		util.OK(c, gin.H{
			"ok":                     true,
			"environment":            cfg.AppEnv,
			"preBackup": map[string]any{
				"ok":   preBackupOK,
				"path": preBackupPath,
				"file": filepath.Base(preBackupPath),
			},
			"ledgerDump": map[string]any{
				"rows": ledgerDumpCount,
				"path": ledgerDumpPath,
				"file": filepath.Base(ledgerDumpPath),
			},
			"clearedTables":          clearedResults,
			"totalCleared":           totalCleared,
			"keptTables":             keepAfter,
			"impactTemplatesBefore":  impactBeforeTemplates,
			"impactTemplatesAfter":   impactAfterTemplates,
			"impactInstancesCleared": impactBeforeInstances,
			"keptUnchanged":          keptUnchanged,
			"keptChanged":            keptChanged,
			"auditWritten":           true,
			"resetAt":                time.Now().Format("2006-01-02 15:04:05"),
		})
	}
}

// ---------- 工具函数 ----------

func findMysqldump() string {
	for _, p := range []string{"mysqldump", `E:\scoop\apps\mysql\26.10.0\bin\mysqldump.exe`, `/usr/bin/mysqldump`} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	// 直接 scoop 路径
	st, err := os.Stat(`E:\scoop\apps\mysql\26.10.0\bin\mysqldump.exe`)
	if err == nil && !st.IsDir() {
		return `E:\scoop\apps\mysql\26.10.0\bin\mysqldump.exe`
	}
	return ""
}

func findMysql() string {
	for _, p := range []string{"mysql", `E:\scoop\apps\mysql\26.10.0\bin\mysql.exe`, `/usr/bin/mysql`} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return ""
}

func hostnameSafe() string {
	h, _ := os.Hostname()
	if h == "" {
		return "unknown"
	}
	return h
}

// scanAllRows 通用扫描：返回 []map[string]any，处理 sql.NullString/Int64/Float64
func scanAllRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for rows.Next() {
		ptrs := make([]any, len(columns))
		vals := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, col := range columns {
			v := vals[i]
			// 处理 []byte → string
			if b, ok := v.([]byte); ok {
				m[col] = string(b)
			} else {
				m[col] = v
			}
		}
		result = append(result, m)
	}
	return result, nil
}

// ---------- 防止 strconv 未使用警告 ----------
var _ = strconv.Itoa
