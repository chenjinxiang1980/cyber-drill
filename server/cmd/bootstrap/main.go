// bootstrap —— 一次性建 MySQL 账号 + 数据库 + migrations + seed。
// 用 root 连接 MySQL server 建库建账号，然后切到业务账号跑 migrations。
//
// 关键配置拆分（.env 里）：
//   DB_ROOT_USER / DB_ROOT_PASSWORD —— MySQL root（或有 CREATE USER/GRANT 权限的高权限账号）
//       默认 root / 空（本机无密码 root 连接场景）。仅 bootstrap 需要，服务启动不需要。
//   DB_USER / DB_PASSWORD —— 业务账号（drill_app 这类，仅 grant drill_system.*）
//       DB_PASSWORD 必须从 "CHANGE_ME" 哨兵替换为真实值，否则 bootstrap 拒绝执行。
//
// 用法：go run ./cmd/bootstrap（从 server/ 目录）
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	rootFlag := flag.String("root", "..", "项目根目录")
	flag.Parse()
	var projectRoot string
	if filepath.IsAbs(*rootFlag) {
		projectRoot = *rootFlag
	} else {
		cwd, _ := os.Getwd()
		projectRoot, _ = filepath.Abs(filepath.Join(cwd, *rootFlag))
	}

	// 读 .env
	envMap := readEnv(filepath.Join(projectRoot, ".env"))
	get := func(k, def string) string {
		if v := envMap[k]; v != "" {
			return v
		}
		return def
	}

	// === root 连接凭据（高权限账号，用于 CREATE DATABASE / CREATE USER / GRANT） ===
	// 优先读 DB_ROOT_USER / DB_ROOT_PASSWORD；若 key 不存在（不是空值）才回退到 DB_USER / DB_PASSWORD；
	// 再不存在就默认 root / 空（本机无密码 root 连接场景）。
	// 注意：DB_ROOT_PASSWORD 显式留空表示"root 无密码"，此时不会回退到 DB_PASSWORD。
	rootUser, rootPwd := "root", ""
	if v, ok := envMap["DB_ROOT_USER"]; ok {
		rootUser = v
	} else if v, ok := envMap["DB_USER"]; ok {
		rootUser = v
	}
	if _, ok := envMap["DB_ROOT_PASSWORD"]; ok {
		rootPwd = envMap["DB_ROOT_PASSWORD"] // key 存在 → 使用显式值（包括空字符串）
	} else if v, ok := envMap["DB_PASSWORD"]; ok {
		rootPwd = v
	}

	dbName := get("DB_NAME", "drill_system")
	host := get("DB_HOST", "127.0.0.1")
	port := get("DB_PORT", "3306")

	// === 业务账号（只给 dbName.* 权限） ===
	targetUser := get("DB_USER", "drill_app")
	targetPwd := get("DB_PASSWORD", "")
	if targetPwd == "" {
		fmt.Println("❌ DB_PASSWORD 未设置。请在 .env 填写真实业务账号密码（不能留空）。")
		os.Exit(1)
	}
	if targetPwd == "CHANGE_ME" {
		fmt.Println("❌ DB_PASSWORD=CHANGE_ME 哨兵未替换。请改成真实业务账号密码后重试。")
		os.Exit(1)
	}
	// 安全检查：root 和 target 不能是同一个账号且密码相同（防止误操作）
	if rootUser == targetUser && rootPwd == targetPwd && rootPwd != "" {
		fmt.Printf("⚠️  警告：DB_ROOT_USER(%s) 与 DB_USER(%s) 相同且密码一致。\n", rootUser, targetUser)
		fmt.Println("    建议 root 用独立高权限账号，业务账号只授予单库权限。继续执行中...")
	}

	// 1. 用 root 连 MySQL server（不指定 db）
	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&loc=Local&charset=utf8mb4&tls=false&multiStatements=true",
		rootUser, rootPwd, host, port)
	srv, err := sql.Open("mysql", rootDSN)
	if err != nil {
		log.Fatalf("打开 MySQL server 失败: %v", err)
	}
	defer srv.Close()
	if err := srv.Ping(); err != nil {
		log.Fatalf("Ping MySQL server (%s@%s:%s) 失败: %v", rootUser, host, port, err)
	}
	fmt.Printf("✅ MySQL server 连接成功 (%s@%s:%s)\n", rootUser, host, port)

	// 2. 建库
	if _, err := srv.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName,
	)); err != nil {
		log.Fatalf("CREATE DB: %v", err)
	}
	fmt.Printf("✅ 数据库 %s 已就绪\n", dbName)

	// 3. 建业务账号（MySQL 8 语法，先 drop 再 create 避免冲突）
	srv.Exec(fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", targetUser))
	srv.Exec(fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", targetUser))
	if _, err := srv.Exec(fmt.Sprintf(
		"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'", targetUser, targetPwd,
	)); err != nil {
		log.Fatalf("CREATE USER: %v", err)
	}
	if _, err := srv.Exec(fmt.Sprintf(
		"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'", dbName, targetUser,
	)); err != nil {
		log.Fatalf("GRANT: %v", err)
	}
	if _, err := srv.Exec("FLUSH PRIVILEGES"); err != nil {
		log.Fatalf("FLUSH: %v", err)
	}
	fmt.Printf("✅ 业务账号 '%s' 已创建 (密码 %s)\n", targetUser, targetPwd)
	fmt.Printf("   已授权 %s.* 的全部权限\n", dbName)

	// 4. 用 drill_app 连 drill_system，跑 migrations
	drillDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local&charset=utf8mb4&tls=false&multiStatements=true",
		targetUser, targetPwd, host, port, dbName)
	db, err := sql.Open("mysql", drillDSN)
	if err != nil {
		log.Fatalf("打开 drill_system 失败: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("Ping drill_system as %s 失败: %v", targetUser, err)
	}
	fmt.Printf("✅ drill_app → drill_system 连接验证通过\n")

	migFile := filepath.Join(projectRoot, "migrations", "0001_init.up.sql")
	data, err := os.ReadFile(migFile)
	if err != nil {
		log.Fatalf("读 migrations 失败: %v", err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		log.Fatalf("执行 migrations: %v", err)
	}
	db.Exec(`INSERT IGNORE INTO schema_migrations (version) VALUES ('0001_init')`)
	fmt.Println("✅ 33 张表已建好")

	// 5. 写测试 seed（与 setup.go 相同逻辑）
	writeTestSeed(db)

	fmt.Println("\n🎉 Bootstrap 完成！")
	fmt.Println("   请更新 .env 中 DB_USER + DB_PASSWORD（与上方 CREATE USER 时填写一致）")
	fmt.Println("   然后就可以 go run . -root=... 启动后端了")
}

func writeTestSeed(db *sql.DB) {
	db.Exec(`INSERT IGNORE INTO roles (id,name,short,color,descr,` + "`external`" + `,status,seq) VALUES 
		('platformadmin','平台管理员','admin','#e74c3c','全局管理员',0,'启用',1),
		('incidentcmd','事件指挥长','cmd','#3498db','事件处置总指挥',0,'启用',2),
		('defender','防守方','def','#27ae60','防守方队员',0,'启用',3),
		('attacker','攻击方','atk','#c0392b','攻击方队员',0,'启用',4),
		('monitor','导调员','mon','#8e44ad','红蓝对抗导调员',0,'启用',5),
		('observer','观察员','obs','#7f8c8d','外部观察员',0,'启用',6)`)

	// admin 初始密码散列值 → 读取 .env 的 ADMIN_INIT_PASSWORD
	// 注意：salt+hash 是固定的，密码变了必须改 .env 并手动算 hash（或重跑 bootstrap）
	initPwd := os.Getenv("ADMIN_INIT_PASSWORD")
	if initPwd == "" { initPwd = "REPLACE_WITH_STRONG_PASSWORD" }
	if initPwd == "REPLACE_WITH_STRONG_PASSWORD" {
		fmt.Println("❌ .env 里 ADMIN_INIT_PASSWORD 未设置，请填写一个强密码后重试")
		os.Exit(1)
	}
	// 以下 salt+hash 是 initPwd 对应的值，由 Node crypto.scryptSync 生成后硬编码。
	// 如果改了 ADMIN_INIT_PASSWORD，必须重新生成这两个值：
	//   node -e "const c=require('crypto'); const salt=c.randomBytes(8).toString('hex'); const h=c.scryptSync(process.env.ADMIN_INIT_PASSWORD, salt, 32); console.log('SALT='+salt); console.log('HASH='+h.toString('hex'))"
	saltHex := "REPLACE_WITH_SALT_HEX"
	hashHex := "REPLACE_WITH_HASH_HEX"

	db.Exec(`INSERT IGNORE INTO users (id,username,name,role_id,org,pwd_salt,pwd_hash,status,must_change,created_at) VALUES 
		(1,'testadmin','测试管理员','platformadmin','示例医院',?,?,?,?,0,'2026-11-02 22:54:00')`,
		saltHex, hashHex, "启用",
	)
	db.Exec(`INSERT IGNORE INTO user_roles (user_id,role_id) VALUES (1,'platformadmin')`)
	db.Exec(`DELETE FROM sessions WHERE user_id=1`)

	fmt.Println("✅ 测试 seed 已写入（testadmin / $ADMIN_INIT_PASSWORD）")
}

func readEnv(path string) map[string]string {
	m := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()
	var buf [4096]byte
	n, _ := f.Read(buf[:])
	for _, line := range splitLines(string(buf[:n])) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if idx := index(line, '='); idx > 0 {
			k := trimSpace(line[:idx])
			v := trimSpace(line[idx+1:])
			v = trim(v, `"'`)
			m[k] = v
		}
	}
	return m
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		} else if s[i] == '\r' {
			if i+1 < len(s) && s[i+1] == '\n' {
				out = append(out, s[start:i])
				start = i + 2
				i++
			} else {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trimSpace(s string) string { return trim(s, " \t\r\n") }
func trim(s, cutset string) string {
	start, end := 0, len(s)
	for start < end && contains(cutset, s[start]) {
		start++
	}
	for end > start && contains(cutset, s[end-1]) {
		end--
	}
	return s[start:end]
}
func contains(cutset string, b byte) bool {
	for i := 0; i < len(cutset); i++ {
		if cutset[i] == b {
			return true
		}
	}
	return false
}
func index(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
