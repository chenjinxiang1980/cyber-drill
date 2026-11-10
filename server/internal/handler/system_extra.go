package handler

// C 块: 备份/恢复/系统信息/seed 导出/system reset
//
// 与 Node src/routes/admin.js + src/routes/system.js 对齐
// 关键差异: Node 用 SQLite 单文件复制做备份，Go 用 MySQL mysqldump 导出 SQL

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ---------- reset 高危操作锁 ----------
var (
	resetLock       sync.Mutex
	resetInProgress bool
)

// ---------- 恢复维护窗口 ----------
// 恢复期间阻断业务写请求，避免导入过程中继续写库造成数据交叉。
// 进程内标志：仅影响当前实例，恢复结束立即清除。
var maintenanceFlag atomic.Bool

// InMaintenance 供全局写请求中间件判断是否处于恢复维护窗口。
func InMaintenance() bool { return maintenanceFlag.Load() }

func beginMaintenance() { maintenanceFlag.Store(true) }
func endMaintenance()   { maintenanceFlag.Store(false) }

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
// 注意：impact 不在此列表——它需要特殊处理（只清 project_id <> ” 的实例行，保留 project_id=” 的模板行）
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

// =========================================================================
// 备份/恢复公共实现（P0-2）
//
// 备份格式统一为 .sql（mysqldump 文本）：
//   - 系统自产备份、上传恢复都只接受 .sql；
//   - 旧 Node 版遗留的 SQLite .db 文件不再列出、不再允许恢复（格式不兼容）；
//   - 恢复前强制自动备份、恢复失败自动回滚、全过程写 restore-*.log 与 audit_logs。
// =========================================================================

type dbConnConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

func dbConnFromConfig(cfg *config.Config) dbConnConfig {
	return dbConnConfig{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Name: cfg.DBName}
}

// dbConnFromEnv 供没有 *config.Config 参数的 handler 使用（配置加载时已写入环境变量）。
func dbConnFromEnv() dbConnConfig {
	port := 3306
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("DB_PORT"))); err == nil && n > 0 {
		port = n
	}
	return dbConnConfig{
		Host:     envText("DB_HOST", "127.0.0.1"),
		Port:     port,
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		Name:     envText("DB_NAME", "drill_system"),
	}
}

func envText(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// resolveProjectRootDir 定位项目根目录：
//   - 本地从 server/ 启动（-root=../）时命中父目录；
//   - 容器内 WORKDIR=/opt/drill（-root=/opt/drill）时命中自身。
//
// 路由签名不在本轮改动范围，取证上传/下载与 restore-upload 无法拿到 projectRoot，
// 因此通过向上查找 public/ 目录还原与 router 一致的路径。
func resolveProjectRootDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for i := 0; i < 6; i++ {
		if st, e := os.Stat(filepath.Join(dir, "public")); e == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func resolveUploadRootDir() string {
	v := envText("UPLOAD_ROOT", "uploads")
	if filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(resolveProjectRootDir(), v)
}

func resolveBackupDir() string {
	dataDir := envText("DATA_DIR", "data")
	if filepath.IsAbs(dataDir) {
		return filepath.Join(dataDir, "backups")
	}
	return filepath.Join(resolveProjectRootDir(), dataDir, "backups")
}

func mysqldumpArgs(dbc dbConnConfig) []string {
	return []string{
		"--quick",
		"--single-transaction", // 一致性快照，避免锁表
		"--routines",
		"--triggers",
		"--skip-lock-tables", // 普通账号没有 RELOAD 权限
		"--no-tablespaces",
		"--default-character-set=utf8mb4",
		fmt.Sprintf("--host=%s", dbc.Host),
		fmt.Sprintf("--port=%d", dbc.Port),
		fmt.Sprintf("--user=%s", dbc.User),
		dbc.Name,
	}
}

// runMysqldumpToFile 执行全量导出并校验产物非空、可识别。
func runMysqldumpToFile(dbc dbConnConfig, target string) error {
	dumpPath := findMysqldump()
	if dumpPath == "" {
		return fmt.Errorf("容器内缺少 mysqldump（镜像需安装 mariadb-client）")
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(dumpPath, mysqldumpArgs(dbc)...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbc.Password)
	cmd.Stdout = out
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	closeErr := out.Close()
	if runErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("mysqldump 执行失败: %v: %s", runErr, truncateText(strings.TrimSpace(stderr.String()), 800))
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("备份文件写入失败: %w", closeErr)
	}
	st, err := os.Stat(target)
	if err != nil || st.Size() == 0 {
		_ = os.Remove(target)
		return fmt.Errorf("mysqldump 输出为空")
	}
	if err := validateSQLDump(target); err != nil {
		_ = os.Remove(target)
		return err
	}
	return nil
}

// validateSQLDump 只接受 mysqldump 形态的 .sql，避免把任意文件当成数据库快照。
func validateSQLDump(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("读取备份文件失败: %w", err)
	}
	if st.IsDir() || st.Size() == 0 {
		return fmt.Errorf("备份文件为空或不是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("读取备份文件失败: %w", err)
	}
	defer f.Close()
	head := make([]byte, 64*1024)
	n, _ := f.Read(head)
	text := string(head[:n])
	lower := strings.ToLower(text)
	if strings.Contains(lower, "mysql dump") || strings.Contains(lower, "mariadb dump") ||
		strings.Contains(lower, "create table") || strings.Contains(lower, "insert into") {
		return nil
	}
	return fmt.Errorf("不是有效的 MySQL 备份（缺少 mysqldump 头部或 SQL 语句）")
}

func importSQLFile(dbc dbConnConfig, path string) error {
	client := findMysql()
	if client == "" {
		return fmt.Errorf("容器内缺少 mysql/mariadb 客户端（镜像需安装 mariadb-client）")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer f.Close()
	cmd := exec.Command(client,
		"--default-character-set=utf8mb4",
		fmt.Sprintf("--host=%s", dbc.Host),
		fmt.Sprintf("--port=%d", dbc.Port),
		fmt.Sprintf("--user=%s", dbc.User),
		dbc.Name,
	)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbc.Password)
	cmd.Stdin = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, truncateText(strings.TrimSpace(stderr.String()), 800))
	}
	return nil
}

var restoreVerifyTables = []string{"schema_migrations", "roles", "users", "projects", "tasks", "evidence", "script_nodes"}

// tableRowCounts 恢复后校验关键表可读，并返回行数快照。旧备份缺表时跳过。
func tableRowCounts(database *db.DB) (map[string]int64, error) {
	out := map[string]int64{}
	for _, table := range restoreVerifyTables {
		var n int64
		if err := database.QueryRow("SELECT COUNT(*) FROM `" + table + "`").Scan(&n); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "no such table") {
				continue
			}
			return out, fmt.Errorf("校验表 %s 失败: %v", table, err)
		}
		out[table] = n
	}
	if len(out) == 0 {
		return out, fmt.Errorf("恢复后数据库不可读（关键表全部缺失）")
	}
	return out, nil
}

func conciseCounts(before, after map[string]int64) string {
	parts := make([]string, 0, len(restoreVerifyTables))
	for _, table := range restoreVerifyTables {
		b, bok := before[table]
		a, aok := after[table]
		if !bok || !aok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d→%d", table, b, a))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func truncateText(s string, max int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max]) + "…"
}

func writeRestoreLog(path, format string, args ...any) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Warn("写入恢复日志失败", "path", path, "err", err)
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func writeRestoreAudit(database *db.DB, actor, roleID string, userID int64, ip, action, result string) {
	if database == nil {
		return
	}
	action = truncateText(action, 500)
	if _, err := database.Exec(
		"INSERT INTO audit_logs (time,`user`,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)",
		util.Now(), actor, roleID, userID, action, ip,
	); err != nil {
		slog.Warn("写入恢复审计失败", "result", result, "err", err)
	}
}

type restoreFailure struct {
	Code int
	Msg  string
}

func (e *restoreFailure) Error() string { return e.Msg }

var restoreLock sync.Mutex

// performRestore 是所有恢复入口（列表恢复 / 上传恢复）的唯一实现。
// 加密备份（.sql.enc 与 *.tar.gz.enc）先用口令解密到 0600 临时文件，恢复结束即删。
// 失败语义：任何一步失败都返回非 2xx，绝不返回 ok。
func performRestore(database *db.DB, c *gin.Context, dbc dbConnConfig, backupDir, full, displayName, passphrase string) (gin.H, *restoreFailure) {
	restoreLock.Lock()
	defer restoreLock.Unlock()

	beginMaintenance()
	defer endMaintenance()

	started := time.Now()
	if err := ensurePrivateDir(backupDir); err != nil {
		return nil, &restoreFailure{Code: http.StatusInternalServerError, Msg: "创建备份目录失败: " + err.Error()}
	}
	logName := fmt.Sprintf("restore-%s-%s.log", started.Format("20060102-150405"), randHex(3))
	logPath := filepath.Join(backupDir, logName)

	u := auth.CurrentUser(c)
	actor, roleID, userID, ip := "unknown", "", int64(0), c.ClientIP()
	if u != nil {
		actor, roleID, userID = u.Name, u.Role.ID, u.ID
	}
	writeRestoreLog(logPath, "开始恢复：来源=%s 操作人=%s(%d) 角色=%s IP=%s", displayName, actor, userID, roleID, ip)

	fail := func(code int, msg string) (gin.H, *restoreFailure) {
		writeRestoreLog(logPath, "失败：%s", msg)
		writeRestoreAudit(database, actor, roleID, userID, ip, "数据恢复失败："+displayName+"；"+truncateText(msg, 300), "failed")
		slog.Warn("备份恢复失败", "source", displayName, "actor", actor, "err", msg)
		return nil, &restoreFailure{Code: code, Msg: msg}
	}

	// 加密备份先解密到临时文件（0600），明文只在本函数生命周期内存在
	sqlPath := full
	encrypted := isEncryptedBackup(full)
	if encrypted {
		tmp, err := os.CreateTemp(backupDir, ".decrypt-*.sql")
		if err != nil {
			return fail(http.StatusInternalServerError, "创建解密临时文件失败："+err.Error())
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		if derr := decryptBackupFile(full, tmpPath, passphrase); derr != nil {
			_ = os.Remove(tmpPath)
			return fail(http.StatusBadRequest, "备份解密失败："+derr.Error())
		}
		defer os.Remove(tmpPath)
		sqlPath = tmpPath
		writeRestoreLog(logPath, "已解密到临时文件：%s", filepath.Base(tmpPath))
	}

	if err := validateSQLDump(sqlPath); err != nil {
		return fail(http.StatusBadRequest, "备份文件校验失败："+err.Error())
	}

	preBase := fmt.Sprintf("drill-pre-restore-%s-%s", started.Format("20060102-150405"), randHex(3))
	prePlain := filepath.Join(backupDir, preBase+".sql")
	if err := runMysqldumpToFile(dbc, prePlain); err != nil {
		return fail(http.StatusServiceUnavailable, "恢复前自动备份失败，已中止恢复（当前数据库未被改动）："+err.Error())
	}
	preName := encryptedBackupName(preBase + ".sql")
	prePath := filepath.Join(backupDir, preName)
	if err := encryptBackupFile(prePlain, prePath, passphrase); err != nil {
		_ = os.Remove(prePlain)
		return fail(http.StatusServiceUnavailable, "恢复前自动备份加密失败，已中止恢复（当前数据库未被改动）："+err.Error())
	}
	_ = os.Remove(prePlain)
	writeRestoreLog(logPath, "恢复前自动备份完成：%s", preName)
	before, _ := tableRowCounts(database)

	// rollback 用恢复前快照覆盖回去（快照是加密的，先解密再导入）
	rollback := func() error {
		path := prePath
		if isEncryptedBackup(prePath) {
			tmp, err := os.CreateTemp(backupDir, ".rollback-*.sql")
			if err != nil {
				return err
			}
			tmpPath := tmp.Name()
			_ = tmp.Close()
			defer os.Remove(tmpPath)
			if err := decryptBackupFile(prePath, tmpPath, passphrase); err != nil {
				return err
			}
			path = tmpPath
		}
		return importSQLFile(dbc, path)
	}

	if err := importSQLFile(dbc, sqlPath); err != nil {
		writeRestoreLog(logPath, "导入失败：%v；开始回滚到 %s", err, preName)
		if rollbackErr := rollback(); rollbackErr != nil {
			msg := fmt.Sprintf("恢复失败：%v；自动回滚也失败：%v；请立即使用 %s 人工恢复", err, rollbackErr, preName)
			return fail(http.StatusInternalServerError, msg)
		}
		writeRestoreLog(logPath, "已回滚到恢复前快照 %s", preName)
		return fail(http.StatusInternalServerError, fmt.Sprintf("恢复失败：%v；已自动回滚到恢复前备份 %s", err, preName))
	}

	after, verifyErr := tableRowCounts(database)
	if verifyErr != nil {
		writeRestoreLog(logPath, "恢复后校验失败：%v；开始回滚", verifyErr)
		if rollbackErr := rollback(); rollbackErr != nil {
			msg := fmt.Sprintf("恢复后校验失败：%v；自动回滚失败：%v；请使用 %s 人工恢复", verifyErr, rollbackErr, preName)
			return fail(http.StatusInternalServerError, msg)
		}
		return fail(http.StatusInternalServerError, fmt.Sprintf("恢复后校验失败：%v；已自动回滚到恢复前备份 %s", verifyErr, preName))
	}

	// 附件归档：与本备份同名的 *.uploads.tar.gz(.enc) 存在时同步恢复；恢复前先备份当前附件。
	uploadsResult := "附件：无归档，未变更"
	sqlBase := strings.TrimSuffix(strings.TrimSuffix(full, ".enc"), ".sql")
	uploadsArchive := sqlBase + ".uploads.tar.gz"
	cleanupUploads := func() {}
	if encArchive := uploadsArchive + ".enc"; fileExists(encArchive) {
		tmp, err := os.CreateTemp(backupDir, ".uploads-*.tar.gz")
		if err != nil {
			uploadsResult = "附件：解密临时文件创建失败（数据库已恢复）"
		} else {
			tmpPath := tmp.Name()
			_ = tmp.Close()
			if derr := decryptBackupFile(encArchive, tmpPath, passphrase); derr != nil {
				_ = os.Remove(tmpPath)
				uploadsResult = "附件恢复失败（数据库已恢复）：附件归档解密失败"
			} else {
				uploadsArchive = tmpPath
				cleanupUploads = func() { _ = os.Remove(tmpPath) }
			}
		}
	}
	if st, statErr := os.Stat(uploadsArchive); statErr == nil && !st.IsDir() {
		preUploadsBase := fmt.Sprintf("drill-pre-restore-%s-%s", started.Format("20060102-150405"), randHex(3))
		preUploads, preFiles := archiveUploadsDir(backupDir, preUploadsBase)
		if preUploads != "" {
			preUploadsPlain := filepath.Join(backupDir, preUploads)
			if err := encryptBackupFile(preUploadsPlain, preUploadsPlain+".enc", passphrase); err == nil {
				_ = os.Remove(preUploadsPlain)
				preUploads += ".enc"
			}
		}
		if err := extractUploadsArchive(uploadsArchive); err != nil {
			uploadsResult = "附件恢复失败（数据库已恢复）：" + truncateText(err.Error(), 200)
			writeRestoreLog(logPath, "%s", uploadsResult)
			slog.Warn("附件恢复失败", "archive", filepath.Base(uploadsArchive), "err", err)
		} else {
			uploadsResult = fmt.Sprintf("附件已恢复：%s；恢复前附件备份=%s(%d 个文件)", filepath.Base(uploadsArchive), preUploads, preFiles)
			writeRestoreLog(logPath, "%s", uploadsResult)
		}
		cleanupUploads()
	}

	// 恢复后强制重新登录
	_, _ = database.Exec(`DELETE FROM sessions`)

	duration := time.Since(started).Truncate(time.Millisecond).String()
	summary := fmt.Sprintf("数据恢复成功：%s；安全网=%s；耗时=%s；行数=%s；%s", displayName, preName, duration, conciseCounts(before, after), uploadsResult)
	writeRestoreLog(logPath, "%s", summary)
	writeRestoreAudit(database, actor, roleID, userID, ip, summary, "success")
	slog.Info("备份恢复成功", "source", displayName, "actor", actor, "preBackup", preName, "log", logName, "duration", duration)

	return gin.H{
		"ok":        true,
		"file":      displayName,
		"preBackup": preName,
		"before":    before,
		"after":     after,
		"verified":  true,
		"encrypted": encrypted,
		"uploads":   uploadsResult,
		"log":       logName,
		"duration":  duration,
		"note":      "已覆盖整库并清空全部登录会话，所有用户需重新登录",
		"time":      time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

// ---------- 备份附件与清单（P1-8） ----------

// findTar 定位 tar 命令（Linux/容器为 GNU tar，Windows 10+ 自带 bsdtar）。
func findTar() string {
	if p, err := exec.LookPath("tar"); err == nil {
		return p
	}
	for _, p := range []string{"/bin/tar", "/usr/bin/tar"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// archiveUploadsDir 把 uploads 目录打包为 <base>.uploads.tar.gz，返回归档名与文件数。
// 打包失败不阻断数据库备份（SQL 仍是主产物），只记日志。
func archiveUploadsDir(backupDir, base string) (string, int) {
	root := resolveUploadRootDir()
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return "", 0
	}
	files := 0
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && info != nil && !info.IsDir() {
			files++
		}
		return nil
	})
	if files == 0 {
		return "", 0
	}
	tarBin := findTar()
	if tarBin == "" {
		slog.Warn("备份附件跳过：未找到 tar 命令", "root", root)
		return "", files
	}
	name := base + ".uploads.tar.gz"
	target := filepath.Join(backupDir, name)
	cmd := exec.Command(tarBin, "-czf", target, "-C", filepath.Dir(root), filepath.Base(root))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Warn("备份附件失败", "root", root, "err", err, "stderr", truncateText(stderr.String(), 400))
		_ = os.Remove(target)
		return "", files
	}
	return name, files
}

// validateUploadsArchive 校验归档内路径均位于 uploads 根下，拒绝绝对路径与 .. 穿越。
func validateUploadsArchive(archive, rootBase string) error {
	tarBin := findTar()
	if tarBin == "" {
		return fmt.Errorf("未找到 tar 命令")
	}
	out, err := exec.Command(tarBin, "-tzf", archive).Output()
	if err != nil {
		return fmt.Errorf("读取附件归档失败: %w", err)
	}
	prefix := strings.TrimSuffix(rootBase, "/") + "/"
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "/") || strings.Contains(p, "..") || !strings.HasPrefix(p, prefix) {
			return fmt.Errorf("附件归档包含非法路径: %s", truncateText(p, 120))
		}
	}
	return nil
}

// extractUploadsArchive 将附件归档解包回 uploads 根目录。
func extractUploadsArchive(archive string) error {
	root := resolveUploadRootDir()
	if err := validateUploadsArchive(archive, filepath.Base(root)); err != nil {
		return err
	}
	tarBin := findTar()
	if tarBin == "" {
		return fmt.Errorf("未找到 tar 命令")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	cmd := exec.Command(tarBin, "-xzf", archive, "-C", filepath.Dir(root))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, truncateText(stderr.String(), 400))
	}
	return nil
}

type backupManifest struct {
	Time        string           `json:"time"`
	Database    string           `json:"database"`
	SQL         string           `json:"sql"`
	SQLSize     int64            `json:"sqlSize"`
	SQLSHA256   string           `json:"sqlSha256"`
	Encrypted   bool             `json:"encrypted"`
	Cipher      string           `json:"cipher,omitempty"`
	Uploads     string           `json:"uploads,omitempty"`
	UploadFiles int              `json:"uploadFiles"`
	Counts      map[string]int64 `json:"counts"`
}

// fileSHA256 返回文件摘要；失败返回空串（清单不因此失败）。
func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// writeBackupManifest 写 <base>.manifest.json，记录 SQL 摘要、附件归档与关键表行数。
func writeBackupManifest(backupDir, base string, database *db.DB, dbName, sqlPath, uploadsName string, uploadFiles int) string {
	st, _ := os.Stat(sqlPath)
	var size int64
	if st != nil {
		size = st.Size()
	}
	counts, _ := tableRowCounts(database)
	encrypted := strings.HasSuffix(filepath.Base(sqlPath), ".enc")
	m := backupManifest{
		Time:        time.Now().Format("2006-01-02 15:04:05"),
		Database:    dbName,
		SQL:         filepath.Base(sqlPath),
		SQLSize:     size,
		SQLSHA256:   fileSHA256(sqlPath),
		Encrypted:   encrypted,
		Uploads:     uploadsName,
		UploadFiles: uploadFiles,
		Counts:      counts,
	}
	if encrypted {
		m.Cipher = "AES-256-GCM/PBKDF2-SHA256"
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return ""
	}
	name := base + ".manifest.json"
	if err := os.WriteFile(filepath.Join(backupDir, name), data, 0o600); err != nil {
		slog.Warn("写入备份清单失败", "err", err)
		return ""
	}
	return name
}

// ---------- POST /api/admin/backup ----------
// 备份产物统一加密：drill-<时间>.sql.enc（AES-256-GCM），附件同名加 .enc。
func AdminBackup(database *db.DB, backupDir string, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		passphrase, err := backupPassphrase()
		if err != nil {
			util.Error(c, http.StatusServiceUnavailable, "备份加密未配置："+err.Error())
			return
		}
		if err := ensurePrivateDir(backupDir); err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份目录失败: "+err.Error())
			return
		}

		base := fmt.Sprintf("drill-%s", time.Now().Format("20060102-150405"))
		plainPath := filepath.Join(backupDir, base+".sql")
		if err := runMysqldumpToFile(dbConnFromConfig(cfg), plainPath); err != nil {
			util.Error(c, http.StatusInternalServerError, "备份失败: "+err.Error())
			return
		}
		name := encryptedBackupName(base + ".sql")
		target := filepath.Join(backupDir, name)
		if err := encryptBackupFile(plainPath, target, passphrase); err != nil {
			_ = os.Remove(plainPath)
			util.Error(c, http.StatusInternalServerError, "备份加密失败: "+err.Error())
			return
		}
		// 明文只在加密成功后删除，避免加密失败时连备份也没有
		_ = os.Remove(plainPath)

		uploadsName, uploadFiles := archiveUploadsDir(backupDir, base)
		if uploadsName != "" {
			encUploads := encryptedBackupName(uploadsName)
			if err := encryptBackupFile(filepath.Join(backupDir, uploadsName), filepath.Join(backupDir, encUploads), passphrase); err != nil {
				slog.Warn("附件归档加密失败", "err", err)
				_ = os.Remove(filepath.Join(backupDir, uploadsName))
				uploadsName = ""
			} else {
				_ = os.Remove(filepath.Join(backupDir, uploadsName))
				uploadsName = encUploads
			}
		}
		manifestName := writeBackupManifest(backupDir, base, database, dbConnFromConfig(cfg).Name, target, uploadsName, uploadFiles)

		st, _ := os.Stat(target)
		util.OK(c, gin.H{
			"ok":          true,
			"file":        name,
			"path":        target,
			"size":        st.Size(),
			"encrypted":   true,
			"cipher":      "AES-256-GCM/PBKDF2-SHA256",
			"uploads":     uploadsName,
			"uploadFiles": uploadFiles,
			"manifest":    manifestName,
			"time":        time.Now().Format("2006-01-02 15:04:05"),
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
		// 统一为 .sql / .sql.enc；旧 .db（SQLite）不再展示，避免误恢复
		re := regexp.MustCompile(`(?i)\.sql(\.enc)?$`)
		type item struct {
			File string `json:"file"`
			Size int64  `json:"size"`
			Time string `json:"time"`
			mod  time.Time
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
				mod:  st.ModTime(),
			})
		}
		// 最新备份在前
		sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
		c.JSON(http.StatusOK, list)
	}
}

// safeBackupFile 防止路径穿越；只接受 .sql 与 .sql.enc（兼容历史上带空格的文件名）。
var safeBackupRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._ -]*\.sql(\.enc)?$`)

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
// 恢复会覆盖整库并清空会话；必须提交确认词 RESTORE 与独立恢复口令。
func AdminRestore(database *db.DB, backupDir string, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			File            string `json:"file"`
			Confirm         string `json:"confirm"`
			RestorePassword string `json:"restorePassword"`
			Passphrase      string `json:"passphrase"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.File) == "" {
			util.Error(c, http.StatusBadRequest, "缺少备份文件名")
			return
		}
		if code, msg := checkRestoreGate(body.Confirm, body.RestorePassword); code != 0 {
			util.Error(c, code, msg)
			return
		}
		full := safeBackupFile(body.File, backupDir)
		if full == "" {
			util.Error(c, http.StatusBadRequest, "备份文件名不合法（仅支持 .sql / .sql.enc）")
			return
		}
		if _, err := os.Stat(full); err != nil {
			util.Error(c, http.StatusBadRequest, "备份文件不存在: "+body.File)
			return
		}

		passphrase := strings.TrimSpace(body.Passphrase)
		if passphrase == "" {
			passphrase, _ = backupPassphrase()
		}
		data, failure := performRestore(database, c, dbConnFromConfig(cfg), backupDir, full, body.File, passphrase)
		if failure != nil {
			util.Error(c, failure.Code, failure.Msg)
			return
		}
		util.OK(c, data)
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
		base := strings.TrimSuffix(strings.TrimSuffix(full, ".enc"), ".sql")
		_ = os.Remove(full)
		_ = os.Remove(base + ".sql")
		_ = os.Remove(base + ".sql.enc")
		_ = os.Remove(base + ".uploads.tar.gz")
		_ = os.Remove(base + ".uploads.tar.gz.enc")
		_ = os.Remove(base + ".manifest.json")
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
			database.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&cnt)
			counts[t] = cnt
		}
		util.OK(c, gin.H{
			"goVersion":  runtime.Version(),
			"platform":   runtime.GOOS + " " + runtime.GOARCH,
			"hostname":   hostnameSafe(),
			"serverTime": time.Now().Format("2006-01-02 15:04:05"),
			"dbBackend":  "MySQL",
			"counts":     counts,
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
//  1. APP_ENV=production 时需 ALLOW_DANGEROUS_RESET=true
//  2. 请求体必须包含 {"confirm":"RESET"}
//  3. 执行前自动 mysqldump + ledger JSON dump
//  4. 所有清除完成后写一条 audit_logs（audit_logs 本身也在清除范围，故后置）
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

		// 防护 3a: reset 前自动 mysqldump（默认加密，失败时保留明文并告警）
		if err := ensurePrivateDir(backupDir); err != nil {
			util.Error(c, http.StatusInternalServerError, "创建备份目录失败: "+err.Error())
			return
		}
		preBackupPath := filepath.Join(backupDir, fmt.Sprintf("drill-pre-reset-%s.sql", ts))
		preBackupPlain := preBackupPath
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
			preFile, err := os.OpenFile(preBackupPlain, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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
			if preBackupOK {
				if pass, perr := backupPassphrase(); perr == nil {
					encPath := preBackupPlain + ".enc"
					if eerr := encryptBackupFile(preBackupPlain, encPath, pass); eerr == nil {
						_ = os.Remove(preBackupPlain)
						preBackupPath = encPath
					} else {
						slog.Warn("重置前备份加密失败，保留明文备份", "err", eerr)
					}
				} else {
					slog.Warn("未配置备份加密口令，重置前备份保持明文", "err", perr)
				}
			} else {
				_ = os.Remove(preBackupPlain)
			}
		}

		// 防护 3b: ledger 行级 JSON dump（清除前）
		ledgerDumpPath := filepath.Join(backupDir, fmt.Sprintf("ledger-before-reset-%s.json", ts))
		ledgerDumpCount := 0
		if lrows, err := database.Query("SELECT * FROM ledger"); err == nil {
			if ldata, err := scanAllRows(lrows); err == nil {
				ledgerDumpCount = len(ldata)
				if jsonBytes, err := json.MarshalIndent(ldata, "", "  "); err == nil {
					os.WriteFile(ledgerDumpPath, jsonBytes, 0o600)
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
			database.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&cnt)
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
			database.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&before)
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
			database.QueryRow("SELECT COUNT(*) FROM " + kb.Name).Scan(&cnt)
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
			"ok":          true,
			"environment": cfg.AppEnv,
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
	for _, p := range []string{
		"mysqldump", "mariadb-dump",
		`E:\scoop\apps\mysql\26.10.0\bin\mysqldump.exe`,
		"/usr/bin/mysqldump", "/usr/bin/mariadb-dump",
	} {
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
	for _, p := range []string{
		"mysql", "mariadb",
		`E:\scoop\apps\mysql\26.10.0\bin\mysql.exe`,
		"/usr/bin/mysql", "/usr/bin/mariadb",
	} {
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
