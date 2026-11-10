// drill-upgrade —— 源码编译用户的「只升级数据库」工具。
//
// 与 deploy/upgrade/upgrade.sh 共用同一份版本清单（deploy/upgrade/manifest.json），
// 但只做两件事：
//
//	verify  只读检查：当前版本、链是否完整、需要跑哪几条迁移
//	db      备份 → 调用 drill-migrate 应用迁移 → 校验迁移记录
//
// 它不碰容器、不改 compose；容器部署请用 upgrade.sh。
//
// 用法：
//
//	drill-upgrade verify --to 0.9.0 --env-file /opt/drill/etc/drill.env
//	drill-upgrade db --to 0.9.0 --env-file /opt/drill/etc/drill.env --backup-dir /opt/drill/backups
//
// 清单内嵌在二进制里，可用 --manifest 指向外部文件（排障用）。
package main

import (
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"drill-system/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

//go:generate cp ../../../deploy/upgrade/manifest.json manifest.json

//go:embed manifest.json
var embeddedManifest embed.FS

// ---------- 清单结构 ----------

type manifest struct {
	SchemaVersion int       `json:"schema_version"`
	UpdatedAt     string    `json:"updated_at"`
	Releases      []release `json:"releases"`
}

type release struct {
	Version       string `json:"version"`
	Previous      string `json:"previous"`
	Channel       string `json:"channel"`
	BestEffort    bool   `json:"best_effort"`
	PublishedAt   string `json:"published_at"`
	Image         imageInfo
	Migrations    migrationsInfo `json:"migrations"`
	SchemaTotal   int            `json:"schema_migrations_total"`
	MinFreeBytes  int64          `json:"min_free_bytes"`
	NotesURL      string         `json:"notes_url"`
}

type imageInfo struct {
	Tags    []string          `json:"tags"`
	Digest  map[string]string `json:"digest"`
	Size    int64             `json:"size_bytes"`
	Layers  int               `json:"layers"`
	Unknown bool              `json:"digest_unknown"`
}

type migrationsInfo struct {
	FromExclusive string `json:"from_exclusive"`
	Through       string `json:"through"`
	Count         int    `json:"count"`
	ContainsDDL   bool   `json:"contains_ddl"`
	Compatible    bool   `json:"backward_compatible"`
}

func loadManifest(path string) (*manifest, []byte, error) {
	var raw []byte
	var err error
	if path != "" {
		raw, err = os.ReadFile(path)
	} else {
		raw, err = embeddedManifest.ReadFile("manifest.json")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取版本清单失败: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, fmt.Errorf("解析版本清单失败: %w", err)
	}
	if m.SchemaVersion != 1 {
		return nil, nil, fmt.Errorf("清单 schema_version=%d，本工具只支持 1", m.SchemaVersion)
	}
	return &m, raw, nil
}

func (m *manifest) release(v string) *release {
	for i := range m.Releases {
		if m.Releases[i].Version == v {
			return &m.Releases[i]
		}
	}
	return nil
}

func (m *manifest) versionByTotal(n int) string {
	for i := range m.Releases {
		if m.Releases[i].SchemaTotal == n {
			return m.Releases[i].Version
		}
	}
	return ""
}

func (m *manifest) versions() []string {
	out := make([]string, 0, len(m.Releases))
	for _, r := range m.Releases {
		out = append(out, r.Version)
	}
	sort.Strings(out)
	return out
}

// chain 返回 from（不含）→ to（含）的版本序列；同时校验连续性。
func (m *manifest) chain(from, to string) ([]*release, error) {
	fr, tr := m.release(from), m.release(to)
	if fr == nil {
		return nil, fmt.Errorf("当前版本 %s 不在清单内（已知：%s）", from, strings.Join(m.versions(), "、"))
	}
	if tr == nil {
		return nil, fmt.Errorf("目标版本 %s 不在清单内（已知：%s）", to, strings.Join(m.versions(), "、"))
	}

	// 降级检测：从 from 往上追溯，若遇到 to 说明 to 是 from 的前置
	for cur, n := from, 0; n < 50; n++ {
		if cur == to {
			return nil, fmt.Errorf("不支持降级：%s 是 %s 的前置版本（降级请用整库备份恢复）", to, from)
		}
		r := m.release(cur)
		if r == nil || r.Previous == "" {
			break
		}
		cur = r.Previous
	}

	var rev []*release
	cur := to
	for i := 0; cur != from; i++ {
		if i > 50 {
			return nil, fmt.Errorf("版本链出现环（超过 50 跳）")
		}
		r := m.release(cur)
		if r == nil {
			return nil, fmt.Errorf("版本链缺环：清单里没有 %s", cur)
		}
		if r.Previous == "" {
			return nil, fmt.Errorf("版本链缺环：%s 的 previous 为空，无法回溯到 %s", cur, from)
		}
		rev = append(rev, r)
		cur = r.Previous
	}
	// 反转成从旧到新
	chain := make([]*release, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		chain = append(chain, rev[i])
	}
	if len(chain) > 0 {
		if got, want := chain[0].Migrations.FromExclusive, fr.Migrations.Through; got != want {
			return nil, fmt.Errorf("版本链不连续：%s 的区间起点是 %s，但 %s 的终点是 %s",
				chain[0].Version, got, from, want)
		}
		for i := 1; i < len(chain); i++ {
			if chain[i-1].Migrations.Through != chain[i].Migrations.FromExclusive {
				return nil, fmt.Errorf("版本链缺环：%s 终点 %s ≠ %s 起点 %s",
					chain[i-1].Version, chain[i-1].Migrations.Through,
					chain[i].Version, chain[i].Migrations.FromExclusive)
			}
		}
	}
	return chain, nil
}

// ---------- 数据库 ----------

type dbConfig struct {
	cfg  *config.Config
	name string
}

func parseEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		v = strings.Trim(v, `"'`)
		out[k] = v
	}
	return out, nil
}

func openDB(envFile, host, port string) (*dbConfig, error) {
	env, err := parseEnvFile(envFile)
	if err != nil {
		return nil, fmt.Errorf("读取环境文件 %s 失败: %w", envFile, err)
	}
	get := func(k, def string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		if v := env[k]; v != "" {
			return v
		}
		return def
	}
	c := &config.Config{
		DBHost:     get("DB_HOST", "127.0.0.1"),
		DBName:     get("DB_NAME", "drill_system"),
		DBUser:     get("DB_USER", ""),
		DBPassword: get("DB_PASSWORD", ""),
		DBSSLMode:  get("DB_SSLMODE", "disable"),
	}
	if host != "" {
		c.DBHost = host
	}
	p := get("DB_PORT", "3306")
	if port != "" {
		p = port
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return nil, fmt.Errorf("DB_PORT 非法: %s", p)
	}
	c.DBPort = n
	if c.DBUser == "" || c.DBPassword == "" {
		return nil, fmt.Errorf("环境文件里缺少 DB_USER / DB_PASSWORD（%s）", envFile)
	}
	return &dbConfig{cfg: c, name: c.DBName}, nil
}

func (d *dbConfig) connect() (*sql.DB, error) {
	db, err := sql.Open("mysql", d.cfg.DSN())
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetMaxOpenConns(2)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库 %s:%d/%s 失败: %w", d.cfg.DBHost, d.cfg.DBPort, d.name, err)
	}
	return db, nil
}

func schemaState(db *sql.DB) (int, string, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'schema_migrations'`).Scan(&n); err != nil {
		return 0, "", err
	}
	if n == 0 {
		return 0, "", nil
	}
	var count int
	var last string
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return 0, "", err
	}
	if err := db.QueryRow(`SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&last); err != nil {
		last = ""
	}
	return count, last, nil
}

// ---------- 子命令 ----------

type result struct {
	OK      bool     `json:"ok"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	DBRows  int      `json:"db_rows"`
	DBLast  string   `json:"db_last"`
	Chain   []string `json:"chain"`
	Backup  string   `json:"backup,omitempty"`
	Message string   `json:"message,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "db":
		os.Exit(cmdDB(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令：%s\n\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Print(`drill-upgrade —— 只升级 Drill 数据库（不碰容器）

用法：
  drill-upgrade verify --to <版本> --env-file <环境文件> [--manifest <清单>]
  drill-upgrade db     --to <版本> --env-file <环境文件> [--backup-dir <目录>] [--yes]

选项：
  --to <ver>          目标版本（必填）
  --from <ver>        当前版本（默认按 schema_migrations 行数反查）
  --env-file <file>   部署环境文件（DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASSWORD）
  --db-host <host>    覆盖 DB_HOST（默认取环境文件；容器内服务名 mysql 在宿主机不可解析）
  --db-port <port>    覆盖 DB_PORT
  --manifest <file>   使用外部清单（默认用二进制内嵌的那份）
  --backup-dir <dir>  db 子命令的备份目录（默认 ./backups）
  --root <dir>        migrations/ 所在目录（默认 .，与 drill-migrate -root 一致）
  --migrate-bin <f>   drill-migrate 路径（默认同目录下 drill-migrate，其次 PATH）
  --mysqldump <f>     mysqldump 路径（默认 PATH 里的 mysqldump）
  --yes               非交互执行 db（不做二次确认）
  --json              以 JSON 输出结果

退出码：0 成功 / 1 执行失败 / 2 参数或版本链错误
`)
	os.Exit(2)
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	to := fs.String("to", "", "目标版本")
	from := fs.String("from", "", "当前版本")
	envFile := fs.String("env-file", "", "环境文件")
	host := fs.String("db-host", "", "覆盖 DB_HOST")
	port := fs.String("db-port", "", "覆盖 DB_PORT")
	mfPath := fs.String("manifest", "", "外部清单")
	asJSON := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *to == "" || *envFile == "" {
		fmt.Fprintln(os.Stderr, "verify 需要 --to 与 --env-file")
		return 2
	}
	m, raw, err := loadManifest(*mfPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	sum := sha256.Sum256(raw)
	fmt.Printf("清单：%s（updated_at %s，sha256 %s）\n", mfName(*mfPath), m.UpdatedAt, hex.EncodeToString(sum[:8]))

	d, err := openDB(*envFile, *host, *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	db, err := d.connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()

	rows, last, err := schemaState(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 schema_migrations 失败: %v\n", err)
		return 1
	}
	fmt.Printf("数据库：%s  迁移记录：%d 条（末条 %s）\n", d.name, rows, orDash(last))

	cur := *from
	if cur == "" {
		cur = m.versionByTotal(rows)
		if cur == "" {
			fmt.Fprintf(os.Stderr, "无法判定当前版本：%d 条迁移与清单任何版本都不匹配，请用 --from 指定\n", rows)
			return 2
		}
		fmt.Printf("推断当前版本：%s\n", cur)
	}
	if r := m.release(cur); r == nil || r.SchemaTotal != rows {
		fmt.Fprintf(os.Stderr, "数据库与版本 %s 不一致：期望 %d 条迁移，实际 %d 条\n", cur, totalOf(m, cur), rows)
		return 2
	}

	chain, err := m.chain(cur, *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "版本链校验失败：%v\n", err)
		return 2
	}
	fmt.Printf("升级路径：%s → %s（%d 跳，只跑目标版本镜像里的迁移）\n", cur, *to, len(chain))
	for _, r := range chain {
		note := ""
		if r.Channel == "internal" {
			note += " [internal]"
		}
		if r.BestEffort {
			note += " [best-effort]"
		}
		fmt.Printf("  → %s  +%d 条（累计 %d，末条 %s）DDL=%v%s\n",
			r.Version, r.Migrations.Count, r.SchemaTotal, r.Migrations.Through, r.Migrations.ContainsDDL, note)
	}
	if last != "" && last != chain[0].Migrations.FromExclusive {
		fmt.Fprintf(os.Stderr, "警告：数据库末条 %s 与清单起点 %s 不一致，迁移可能无法衔接\n", last, chain[0].Migrations.FromExclusive)
	}

	if *asJSON {
		printJSON(result{OK: true, From: cur, To: *to, DBRows: rows, DBLast: last, Chain: versionsOf(chain)})
	}
	fmt.Println("检查通过：版本链完整，可以执行 db 子命令")
	return 0
}

func cmdDB(args []string) int {
	fs := flag.NewFlagSet("db", flag.ContinueOnError)
	to := fs.String("to", "", "目标版本")
	from := fs.String("from", "", "当前版本")
	envFile := fs.String("env-file", "", "环境文件")
	host := fs.String("db-host", "", "覆盖 DB_HOST")
	port := fs.String("db-port", "", "覆盖 DB_PORT")
	mfPath := fs.String("manifest", "", "外部清单")
	backupDir := fs.String("backup-dir", "./backups", "备份目录")
	root := fs.String("root", ".", "migrations/ 所在目录")
	migrateBin := fs.String("migrate-bin", "", "drill-migrate 路径")
	mysqldumpBin := fs.String("mysqldump", "", "mysqldump 路径")
	yes := fs.Bool("yes", false, "跳过二次确认")
	asJSON := fs.Bool("json", false, "JSON 输出")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *to == "" || *envFile == "" {
		fmt.Fprintln(os.Stderr, "db 需要 --to 与 --env-file")
		return 2
	}

	m, _, err := loadManifest(*mfPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	d, err := openDB(*envFile, *host, *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	db, err := d.connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()

	rows, last, err := schemaState(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 schema_migrations 失败: %v\n", err)
		return 1
	}
	if last != "" {
		fmt.Printf("数据库：%s  迁移记录：%d 条（末条 %s）\n", d.name, rows, last)
	}
	cur := *from
	if cur == "" {
		cur = m.versionByTotal(rows)
	}
	if cur == "" {
		fmt.Fprintf(os.Stderr, "无法判定当前版本（%d 条迁移），请用 --from 指定\n", rows)
		return 2
	}
	if r := m.release(cur); r == nil || r.SchemaTotal != rows {
		fmt.Fprintf(os.Stderr, "数据库与版本 %s 不一致（期望 %d 条，实际 %d 条），先修好再升级\n", cur, totalOf(m, cur), rows)
		return 2
	}
	chain, err := m.chain(cur, *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "版本链校验失败：%v\n", err)
		return 2
	}
	target := chain[len(chain)-1]

	// 先确认迁移器可用，再动备份，避免「备份完才发现跑不了」
	bin, err := findMigrateBin(*migrateBin, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dumpBin, err := findBin(*mysqldumpBin, "mysqldump")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("升级数据库：%s → %s（%d 条迁移，末条 %s）\n", cur, *to, target.Migrations.Count, target.Migrations.Through)
	if !*yes {
		if !isTerminal() {
			fmt.Fprintln(os.Stderr, "非交互环境需要 --yes")
			return 2
		}
		fmt.Print("确认执行？输入 yes 继续：")
		var ans string
		fmt.Fscanln(os.Stdin, &ans)
		if ans != "yes" {
			fmt.Println("已取消")
			return 0
		}
	}

	if err := os.MkdirAll(*backupDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "创建备份目录失败: %v\n", err)
		return 1
	}
	stamp := time.Now().Format("20060102-150405")
	base := filepath.Join(*backupDir, fmt.Sprintf("db-%s-to-%s-%s.sql.gz", cur, *to, stamp))
	if err := dumpDatabase(d.cfg, dumpBin, base); err != nil {
		fmt.Fprintf(os.Stderr, "备份失败：%v\n", err)
		return 1
	}
	fmt.Printf("备份完成：%s\n", base)

	cmd := exec.Command(bin, "-root="+absOr(*root), "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n迁移失败：%v\n备份在 %s（可整库恢复）\n", err, base)
		return 1
	}

	after, lastAfter, err := schemaState(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "迁移后读取 schema_migrations 失败: %v\n", err)
		return 1
	}
	if after != target.SchemaTotal || (target.Migrations.Through != "" && lastAfter != target.Migrations.Through) {
		fmt.Fprintf(os.Stderr, "迁移后校验失败：%d 条 / 末条 %s，期望 %d 条 / %s\n备份在 %s\n",
			after, orDash(lastAfter), target.SchemaTotal, target.Migrations.Through, base)
		return 1
	}
	fmt.Printf("完成：%d 条迁移，末条 %s\n", after, lastAfter)
	if *asJSON {
		printJSON(result{OK: true, From: cur, To: *to, DBRows: after, DBLast: lastAfter,
			Chain: versionsOf(chain), Backup: base})
	}
	return 0
}

// ---------- 辅助 ----------

func totalOf(m *manifest, v string) int {
	if r := m.release(v); r != nil {
		return r.SchemaTotal
	}
	return -1
}

func versionsOf(chain []*release) []string {
	out := make([]string, 0, len(chain))
	for _, r := range chain {
		out = append(out, r.Version)
	}
	return out
}

func mfName(p string) string {
	if p == "" {
		return "（内嵌 deploy/upgrade/manifest.json）"
	}
	return p
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func absOr(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func findBin(explicit, name string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("找不到 %s: %w", explicit, err)
		}
		return explicit, nil
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("PATH 里找不到 %s，请用 --%s 指定", name, name)
	}
	return p, nil
}

// findMigrateBin 默认优先可执行文件同目录，其次 PATH，最后 <root>/../dist。
func findMigrateBin(explicit, root string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("找不到 %s: %w", explicit, err)
		}
		return explicit, nil
	}
	var cands []string
	if self, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(self), "drill-migrate"))
	}
	cands = append(cands,
		filepath.Join(absOr(root), "drill-migrate"),
		filepath.Join(absOr(root), "..", "dist", "drill-migrate"))
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath("drill-migrate"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到 drill-migrate（已尝试：%s），请用 --migrate-bin 指定", strings.Join(cands, "、"))
}

func dumpDatabase(c *config.Config, dumpBin, out string) error {
	cmd := exec.Command(dumpBin,
		"--single-transaction", "--quick", "--routines", "--triggers", "--events",
		"--no-tablespaces", // 避免非 root 用户因缺 PROCESS 权限而报错
		"--default-character-set=utf8mb4",
		"-h", c.DBHost, "-P", strconv.Itoa(c.DBPort), "-u", c.DBUser, c.DBName)
	// 口令走环境变量，不进命令行
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+c.DBPassword)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	gz := gzip.NewWriter(f)
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(gz, h), stdout); err != nil {
		gz.Close()
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		_ = os.Remove(out)
		return fmt.Errorf("mysqldump 退出异常: %w", err)
	}
	sumFile := out + ".sha256"
	sum := hex.EncodeToString(h.Sum(nil))
	if err := os.WriteFile(sumFile, []byte(sum+"  "+filepath.Base(out)+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("备份 sha256：%s\n", sum)
	return nil
}

func printJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	fmt.Println(string(b))
}
