// migrate —— 读取 migrations/*.up.sql 执行建表 SQL。
//
// 用法：
//   go run ./cmd/migrate -root=/path/to/project up
//   go run ./cmd/migrate status
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"drill-system/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	rootFlag := flag.String("root", "..", "项目根目录（migrations/ 与 .env 所在）")
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
		fmt.Println("Usage: migrate [-root=PATH] <up|status|reset>")
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
		runReset(db, filepath.Join(projectRoot, "migrations"))
	default:
		fmt.Println("未知 action:", action)
		os.Exit(1)
	}
}

func runUp(db *sql.DB, migrationsDir string) {
	versions := loadMigrations(migrationsDir)
	applied := appliedVersions(db)
	var appliedCount int
	for _, v := range versions {
		if _, ok := applied[v.version]; ok {
			fmt.Printf("  SKIP  %s (已应用)\n", v.version)
			continue
		}
		fmt.Printf("  APPLY %s ... ", v.version)
		if err := applyFile(db, v.path); err != nil {
			fmt.Printf("FAIL: %v\n", err)
			os.Exit(1)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v.version); err != nil {
			fmt.Printf("OK (版本表写入失败: %v)\n", err)
		} else {
			fmt.Println("OK")
		}
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
	fmt.Println("默认确认（脚本化执行）")

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

func applyFile(db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(data))
	return err
}
