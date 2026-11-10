// seed —— 容器首次启动的幂等配置补齐。
//
// 职责（只增不删）：
//  1. 确保 12 个系统角色存在；platformadmin 缺失时补建（迁移 0004 是主路径，这里是兜底）
//  2. 确保 admin / platformadmin 两个平台管理员账号存在，并绑定 platformadmin 角色
//  3. 全程单事务；任何失败回滚，旧数据原样保留
//
// 明确禁止：DELETE / TRUNCATE 角色、剧本（scenarios/script_nodes）、流程字典（dict）。
// 账号清理只走 POST /api/admin/users/cleanup-test-accounts（仅测试账号）。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"

	_ "github.com/go-sql-driver/mysql"
)

type account struct {
	Username string
	Name     string
	Org      string
	// EnvKey 一键安装器写入 .env 的初始口令变量名。
	// 有值且合法时优先使用（安装输出已打印一次，不再写启动日志）；
	// 缺失、仍是占位符或强度不足时回退为随机生成。
	EnvKey string
}

func main() {
	rootFlag := flag.String("root", "/opt/drill", "项目根目录（.env 所在）")
	flag.Parse()

	root, _ := filepath.Abs(*rootFlag)
	cfg := config.Load(root)

	database, err := db.Open(cfg)
	if err != nil {
		slog.Error("seed: 连接 MySQL 失败", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	result, err := ensure(database)
	if err != nil {
		slog.Error("seed: 初始化失败（已回滚，旧数据保留）", "err", err)
		os.Exit(1)
	}

	slog.Info("seed: 初始化完成（只增不删）",
		"roles", result.roles, "users", result.users,
		"accountsCreated", result.created, "accountsKept", result.kept)
	if result.created > 0 {
		var fromEnv []string
		for i, name := range result.createdNames {
			if result.createdFromEnv[i] {
				// 安装器已把口令写入 .env 并在安装输出中打印一次，日志不重复泄露。
				fromEnv = append(fromEnv, name)
				continue
			}
			// 兜底路径（手工 docker compose 部署且未配置初始口令）：
			// 随机口令仅在本启动日志中出现一次；账号 must_change=1，首次登录必须改密。
			slog.Warn("seed: 已创建平台管理员账号（一次性初始口令，首次登录强制改密，请立即转交本人）",
				"username", name, "initialPassword", result.createdPasswords[i])
		}
		if len(fromEnv) > 0 {
			slog.Info("seed: 管理员初始口令来自部署环境文件（安装输出已打印一次，本日志不重复输出）",
				"accounts", strings.Join(fromEnv, ","))
		}
	}
}

type seedResult struct {
	roles            int
	users            int
	created          int
	kept             int
	createdNames     []string
	createdPasswords []string
	createdFromEnv   []bool
}

var platformAccounts = []account{
	{Username: "admin", Name: "平台管理员", Org: "信息科平台运维", EnvKey: "ADMIN_INIT_PASSWORD"},
	{Username: "platformadmin", Name: "平台管理员", Org: "平台运维", EnvKey: "PLATFORMADMIN_INIT_PASSWORD"},
}

// ensure 在一个事务里补齐角色与管理员账号；commit 失败或中途出错全部回滚。
func ensure(database *db.DB) (seedResult, error) {
	var res seedResult

	tx, err := database.Begin()
	if err != nil {
		return res, fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 1) 平台管理员角色兜底（迁移 0004 已用 upsert 建好 12 个角色）
	if err = ensurePlatformRole(tx); err != nil {
		return res, err
	}

	// 2) 管理员账号：已存在则跳过（绝不覆盖用户已修改的密码）；
	//    新建账号的初始口令来自安装器写入 .env 的变量，缺失时随机生成。
	for _, a := range platformAccounts {
		pwd, fromEnv, genErr := initialPasswordFor(a)
		if genErr != nil {
			return res, fmt.Errorf("生成初始口令失败: %w", genErr)
		}
		created, err := ensureAccount(tx, a, pwd)
		if err != nil {
			return res, err
		}
		if created {
			res.created++
			res.createdNames = append(res.createdNames, a.Username)
			res.createdPasswords = append(res.createdPasswords, pwd)
			res.createdFromEnv = append(res.createdFromEnv, fromEnv)
		} else {
			res.kept++
		}
	}

	// 3) 统计（提交前读取，同一事务内一致）
	if err = tx.QueryRow(`SELECT COUNT(*) FROM roles`).Scan(&res.roles); err != nil {
		return res, fmt.Errorf("统计 roles 失败: %w", err)
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&res.users); err != nil {
		return res, fmt.Errorf("统计 users 失败: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return res, fmt.Errorf("提交事务失败: %w", err)
	}
	return res, nil
}

// initialPasswordFor 返回账号的一次性初始口令。
// 优先级：安装器写入 .env 的合法口令（fromEnv=true）> 随机生成（fromEnv=false）。
// 占位符（__REPLACE_WITH...）按未配置处理，避免把模板值当成真实口令。
func initialPasswordFor(a account) (pwd string, fromEnv bool, err error) {
	if a.EnvKey != "" {
		v := strings.TrimSpace(os.Getenv(a.EnvKey))
		if v != "" && !strings.HasPrefix(v, "__REPLACE_WITH") {
			if validInitialPassword(v) {
				return v, true, nil
			}
			slog.Warn("seed: 环境文件中的初始口令不符合要求（至少 8 位且含字母与数字），改为随机生成",
				"env", a.EnvKey)
		}
	}
	p, genErr := auth.GenerateTempPassword()
	return p, false, genErr
}

// validInitialPassword 与改密接口的强度要求保持一致：至少 8 位，含字母与数字。
func validInitialPassword(p string) bool {
	if len(p) < 8 {
		return false
	}
	var letter, digit bool
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			letter = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	return letter && digit
}

func ensurePlatformRole(tx *sql.Tx) error {
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM roles WHERE id='platformadmin'`).Scan(&exists); err != nil {
		return fmt.Errorf("查询 platformadmin 角色失败: %w", err)
	}
	if exists > 0 {
		return nil
	}
	_, err := tx.Exec(
		"INSERT INTO roles (id,name,short,icon,color,descr,nav,`external`,status,seq) VALUES " +
			"('platformadmin','平台系统管理员','平台管理员','user','#e74c3c','平台全局管理员，拥有全部权限'," +
			"'[\"dashboard\",\"workbench\",\"collab\",\"evidence\",\"replay\",\"knowledge\",\"screen\",\"report\"]',0,'启用',12)")
	if err != nil {
		return fmt.Errorf("补建 platformadmin 角色失败: %w", err)
	}
	slog.Warn("seed: 检测到 platformadmin 角色缺失，已补建")
	return nil
}

func ensureAccount(tx *sql.Tx, a account, password string) (bool, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM users WHERE username=?`, a.Username).Scan(&id)
	if err == nil {
		// 已有账号：只补 user_roles 绑定（INSERT IGNORE），不改密码/角色
		if _, err = tx.Exec(`INSERT IGNORE INTO user_roles (user_id,role_id) VALUES (?,?)`, id, "platformadmin"); err != nil {
			return false, fmt.Errorf("补 user_roles 失败(%s): %w", a.Username, err)
		}
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, fmt.Errorf("查询账号失败(%s): %w", a.Username, err)
	}

	salt, hash, err := auth.HashPassword(password)
	if err != nil {
		return false, fmt.Errorf("生成密码散列失败(%s): %w", a.Username, err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := tx.Exec(
		`INSERT INTO users (username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES (?,?,?,?,?,?, '启用',1,?)`,
		a.Username, a.Name, "platformadmin", a.Org, salt, hash, now)
	if err != nil {
		return false, fmt.Errorf("创建账号失败(%s): %w", a.Username, err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return false, fmt.Errorf("读取账号 id 失败(%s): %w", a.Username, err)
	}
	if _, err = tx.Exec(`INSERT IGNORE INTO user_roles (user_id,role_id) VALUES (?,?)`, id, "platformadmin"); err != nil {
		return false, fmt.Errorf("写入 user_roles 失败(%s): %w", a.Username, err)
	}
	return true, nil
}
