// migrate —— 读取 migrations/*.up.sql 执行建表 SQL。
//
// 用法：
//
//	go run ./cmd/migrate -root=/path/to/project up
//	go run ./cmd/migrate status
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"drill-system/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// migrationLockName 迁移互斥锁名，多个进程（滚动重启/并发启动）共享同一把 MySQL advisory lock。
const migrationLockName = "drill_schema_migrations"

func main() {
	rootFlag := flag.String("root", "..", "项目根目录（migrations/ 与 .env 所在）")
	yesFlag := flag.Bool("yes", false, "确认执行高危操作（reset 必须显式传入）")
	flag.Parse()
	action := flag.Arg(0)

	var projectRoot string
	if filepath.IsAbs(*rootFlag) {
		projectRoot = *rootFlag
	} else {
		cwd, _ := os.Getwd()
		projectRoot, _ = filepath.Abs(filepath.Join(cwd, *rootFlag))
	}

	if action == "" {
		fmt.Println("Usage: migrate [-root=PATH] [-yes] <up|status|reset>")
		fmt.Println("  projectRoot =", projectRoot)
		os.Exit(1)
	}

	cfg := config.Load(projectRoot)

	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		log.Fatal("打开 MySQL 失败:", err)
	}
	defer db.Close()

	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(32) PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	switch action {
	case "up":
		runUp(db, filepath.Join(projectRoot, "migrations"))
	case "status":
		runStatus(db, filepath.Join(projectRoot, "migrations"))
	case "reset":
		// reset 会 DROP 全部表，必须同时满足：
		//   1) 命令行显式 -yes；
		//   2) 环境变量 DRILL_ALLOW_RESET=1。
		// 避免历史运维脚本、误触 Tab 补全或容器启动脚本静默清库。
		if !*yesFlag || strings.TrimSpace(os.Getenv("DRILL_ALLOW_RESET")) != "1" {
			fmt.Println("拒绝执行 reset：该操作会清空全部表。")
			fmt.Println("如确需执行，请设置 DRILL_ALLOW_RESET=1 并追加 -yes，例如：")
			fmt.Println("  DRILL_ALLOW_RESET=1 go run ./cmd/migrate -root=.. -yes reset")
			os.Exit(1)
		}
		runReset(db, filepath.Join(projectRoot, "migrations"))
	default:
		fmt.Println("未知 action:", action)
		os.Exit(1)
	}
}

func runUp(db *sql.DB, migrationsDir string) {
	// 并发保护：迁移期间持有一条专用连接上的 MySQL advisory lock。
	// 后启动的进程会阻塞等待，拿到锁后再读一次已应用版本，避免同一文件被重复执行。
	// 锁绑定在连接上，连接关闭即自动释放；这里仍显式 RELEASE_LOCK 以便快速让出。
	ctx := context.Background()
	lockConn, err := db.Conn(ctx)
	if err != nil {
		log.Fatal("获取迁移锁连接失败:", err)
	}
	defer lockConn.Close()
	var got sql.NullInt64
	if err := lockConn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 60)`, migrationLockName).Scan(&got); err != nil {
		log.Fatal("获取迁移锁失败:", err)
	}
	if !got.Valid || got.Int64 != 1 {
		log.Fatal("获取迁移锁超时：另一个进程正在执行迁移，本次退出")
	}
	defer func() { _, _ = lockConn.ExecContext(ctx, `SELECT RELEASE_LOCK(?)`, migrationLockName) }()

	versions := loadMigrations(migrationsDir)
	applied := appliedVersions(db)
	var appliedCount int
	for _, v := range versions {
		if _, ok := applied[v.version]; ok {
			fmt.Printf("  SKIP  %s (已应用)\n", v.version)
			continue
		}
		fmt.Printf("  APPLY %s ... ", v.version)
		if err := applyFileTx(db, v.path, v.version); err != nil {
			fmt.Printf("FAIL: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OK")
		appliedCount++
	}
	fmt.Printf("\n完成，本次执行 %d 条迁移\n", appliedCount)
}

func runStatus(db *sql.DB, migrationsDir string) {
	versions := loadMigrations(migrationsDir)
	applied := appliedVersions(db)
	for _, v := range versions {
		if _, ok := applied[v.version]; ok {
			fmt.Printf("  [x] %s  %s\n", v.version, v.path)
		} else {
			fmt.Printf("  [ ] %s  %s\n", v.version, v.path)
		}
	}
	fmt.Printf("\n已应用 %d / %d\n", len(applied), len(versions))
}

func runReset(db *sql.DB, migrationsDir string) {
	fmt.Println("⚠️  即将清空全部表并重新跑 migrations")
	fmt.Println("已通过 -yes 与 DRILL_ALLOW_RESET=1 双重确认")

	rows, _ := db.Query("SHOW TABLES")
	var tables []string
	if rows != nil {
		for rows.Next() {
			var t string
			rows.Scan(&t)
			tables = append(tables, t)
		}
		rows.Close()
	}
	fmt.Printf("删除 %d 张表...\n", len(tables))
	db.Exec("SET FOREIGN_KEY_CHECKS=0")
	for _, t := range tables {
		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", t))
	}
	db.Exec("SET FOREIGN_KEY_CHECKS=1")

	fmt.Println("\n重跑 migrations:")
	runUp(db, migrationsDir)
}

type migrationFile struct {
	version string
	path    string
}

func loadMigrations(dir string) []migrationFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal("读取 migrations 目录失败:", dir, err)
	}
	var out []migrationFile
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			version := strings.TrimSuffix(e.Name(), ".up.sql")
			out = append(out, migrationFile{version, filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out
}

func appliedVersions(db *sql.DB) map[string]bool {
	out := make(map[string]bool)
	rows, _ := db.Query(`SELECT version FROM schema_migrations`)
	if rows == nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		rows.Scan(&v)
		out[v] = true
	}
	return out
}

// ddlPattern 识别迁移文件是否包含 DDL。MySQL 的 DDL 会隐式提交当前事务，
// 因此含 DDL 的迁移无法靠事务回滚，失败后可能留下「半套结构」。
var ddlPattern = regexp.MustCompile(`(?is)\b(create|alter|drop|truncate|rename)\s+(table|index|unique|database|view|procedure|function|trigger)\b`)

// containsDDL 判断迁移 SQL 是否包含 DDL（忽略 -- 行注释）。
func containsDDL(sqlText string) bool {
	var b strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return ddlPattern.MatchString(b.String())
}

// applyFileTx 执行迁移 SQL 并写入版本行。
//
// 纯 DML 迁移在单事务内执行：失败立即回滚，不会出现「SQL 已执行但版本没记上」。
// 含 DDL 的迁移由 MySQL 隐式提交，事务无法回滚（P1-9）：改为直接执行并明确告警，
// 失败时提示「已执行的 DDL 不会回滚」，迁移文件必须写成可重复执行的形式
// （CREATE TABLE IF NOT EXISTS / 先查 information_schema）。
// INSERT ... ON DUPLICATE KEY UPDATE 保证版本行已存在时也不报错（幂等）。
func applyFileTx(db *sql.DB, path, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if containsDDL(string(data)) {
		fmt.Print("(含 DDL，无法回滚) ")
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("DDL 迁移失败，已执行的 DDL 不会回滚；请修正迁移并保证可重复执行后重跑: %w", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?) ON DUPLICATE KEY UPDATE version=version`, version); err != nil {
			return fmt.Errorf("写 schema_migrations 失败: %w", err)
		}
		return nil
	}
	// 纯 DML 单事务执行：失败立即回滚，保留旧数据
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(string(data)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?) ON DUPLICATE KEY UPDATE version=version`, version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("写 schema_migrations 失败: %w", err)
	}
	if err = tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}
