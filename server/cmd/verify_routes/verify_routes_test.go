// verify_routes_test.go —— 路由覆盖率 + 真实 HTTP 探针
//
// 用 go test 而非独立 main，规避 360 对 .exe 的拦截。
// go test 不会生成可执行 .exe（在 Windows 上会用临时路径，但 360 对 go test 较宽容）。

package verify_routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/router"

	"github.com/gin-gonic/gin"
)

// Node 版 117 条路由清单
var nodeRoutes = []struct {
	Method string
	Path   string
}{
	{"GET", "/api/auth/accounts"},
	{"GET", "/api/auth/roles"},
	{"POST", "/api/auth/login"},
	{"POST", "/api/auth/logout"},
	{"GET", "/api/auth/me"},
	{"POST", "/api/auth/change-password"},
	{"GET", "/api/bootstrap"},
	{"POST", "/api/projects/:id/activate"},
	{"PATCH", "/api/projects/:id/status"},
	{"GET", "/api/content/messages"},
	{"POST", "/api/content/messages"},
	{"GET", "/api/content/notifs"},
	{"POST", "/api/content/notifs/read"},
	{"GET", "/api/content/chat/channels"},
	{"GET", "/api/content/chat/:group"},
	{"POST", "/api/content/chat/:group"},
	{"POST", "/api/content/chat/:group/upload"},
	{"GET", "/api/content/drill-progress"},
	{"GET", "/api/content/roles"},
	{"GET", "/api/content/users"},
	{"GET", "/api/content/dashboard"},
	{"GET", "/api/content/audit"},
	{"POST", "/api/content/audit"},
	{"GET", "/api/content/scenarios"},
	{"POST", "/api/content/scenarios"},
	{"PATCH", "/api/content/scenarios/:id"},
	{"GET", "/api/content/script-nodes"},
	{"POST", "/api/content/script-nodes"},
	{"GET", "/api/content/projects"},
	{"POST", "/api/content/projects"},
	{"GET", "/api/content/projects/:id/script"},
	{"GET", "/api/content/projects/:id/script/export"},
	{"POST", "/api/content/projects/:id/notify"},
	{"GET", "/api/content/projects/:id/notify/export"},
	{"PATCH", "/api/content/projects/:id/members"},
	{"DELETE", "/api/content/projects/:id"},
	{"GET", "/api/content/snapshots"},
	{"POST", "/api/content/snapshots"},
	{"GET", "/api/content/seqflow"},
	{"POST", "/api/content/seqflow"},
	{"GET", "/api/content/impact"},
	{"PATCH", "/api/content/impact/:sys"},
	{"GET", "/api/content/opinion"},
	{"POST", "/api/content/opinion"},
	{"PATCH", "/api/content/opinion/:id"},
	{"GET", "/api/content/knowledge"},
	{"POST", "/api/content/knowledge"},
	{"DELETE", "/api/content/knowledge/:id"},
	{"GET", "/api/content/monitor"},
	{"GET", "/api/content/timeline"},
	{"GET", "/api/content/ledger"},
	{"GET", "/api/content/dict"},
	{"GET", "/api/content/sim-data"},
	{"GET", "/api/content/evaluation"},
	{"POST", "/api/content/evaluation/scores"},
	{"GET", "/api/tasks"},
	{"GET", "/api/tasks/"},
	{"POST", "/api/tasks/:id/accept"},
	{"POST", "/api/tasks/:id/submit"},
	{"POST", "/api/tasks/:id/complete"},
	{"DELETE", "/api/tasks/:id"},
	{"POST", "/api/tasks/reset-all"},
	{"POST", "/api/tasks/:id/attachments"},
	{"GET", "/api/tasks/:id/attachments"},
	{"DELETE", "/api/tasks/:id/attachments/:attId"},
	{"POST", "/api/tasks/:id/forms"},
	{"GET", "/api/tasks/:id/forms"},
	{"GET", "/api/evidence"},
	{"GET", "/api/evidence/"},
	{"POST", "/api/evidence"},
	{"POST", "/api/evidence/"},
	{"POST", "/api/evidence/prefill"},
	{"POST", "/api/evidence/mock"},
	{"POST", "/api/evidence/upload"},
	{"GET", "/api/evidence/requirements"},
	{"GET", "/api/evidence/:id/download"},
	{"GET", "/api/admin/users"},
	{"POST", "/api/admin/users"},
	{"PATCH", "/api/admin/users/:id"},
	{"POST", "/api/admin/users/:id/reset-password"},
	{"GET", "/api/admin/roles"},
	{"POST", "/api/admin/roles"},
	{"PUT", "/api/admin/roles/:id"},
	{"PATCH", "/api/admin/roles/:id/status"},
	{"DELETE", "/api/admin/roles/:id"},
	{"PATCH", "/api/admin/roles/order"},
	{"GET", "/api/admin/forms"},
	{"POST", "/api/admin/forms"},
	{"PUT", "/api/admin/forms/:id"},
	{"POST", "/api/admin/backup"},
	{"GET", "/api/admin/backups"},
	{"POST", "/api/admin/restore"},
	{"POST", "/api/admin/restore-upload"},
	{"GET", "/api/admin/backups/:file"},
	{"DELETE", "/api/admin/backups/:file"},
	{"GET", "/api/admin/report/:projectId"},
	{"POST", "/api/admin/report/:projectId/export"},
	{"GET", "/api/admin/audit/export"},
	{"GET", "/api/admin/sysinfo"},
	{"GET", "/api/drill/workbench/:projectId"},
	{"POST", "/api/drill/nodes/:nodeId/start"},
	{"GET", "/api/drill/flows/:projectId"},
	{"GET", "/api/drill/faults/:projectId"},
	{"POST", "/api/drill/skip"},
	{"POST", "/api/drill/decide"},
	{"POST", "/api/drill/finish"},
	{"POST", "/api/drill/steps"},
	{"DELETE", "/api/drill/steps/:id"},
	{"POST", "/api/drill/score/:projectId/recalc"},
	{"GET", "/api/drill/predecessor"},
	{"GET", "/api/system/brand"},
	{"PUT", "/api/system/brand"},
	{"POST", "/api/system/logo"},
	{"GET", "/api/system/status"},
	{"POST", "/api/system/reset"},
	{"GET", "/api/system/dict/:key"},
	{"PUT", "/api/system/dict/:key"},
	{"GET", "/api/system/seed/baselines"},
	{"POST", "/api/system/seed/export"},
	{"GET", "/api/health"},
}

type probe struct {
	Method      string
	Path        string
	Body        string
	RequireAuth bool
	AcceptCodes []int
	WantFields  []string
	Label       string
}

func TestAllRoutesAndHTTP(t *testing.T) {
	// 0. 确保测试库
	setupTestDBIfNeeded(t)

	absRoot, _ := filepath.Abs("../../../")
	cfg := config.Load(absRoot)

	// 白名单
	dbName := strings.ToLower(cfg.DBName)
	if dbName != "s99_ag_end_verify_routes" {
		t.Fatalf("DB_NAME=%s 不在白名单（只允许 S99_AG_END_verify_routes），abort.", cfg.DBName)
	}

	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("MySQL 连接失败: %v", err)
	}
	defer database.Close()

	authSvc := auth.NewService(database, cfg)
	gin.SetMode(gin.TestMode)
	r := router.New(database, cfg, authSvc, absRoot)

	// --- 1. 路由覆盖率 ---
	t.Run("coverage", func(t *testing.T) {
		ginRoutes := r.Routes()
		goKeys := make(map[string]bool)
		for _, rt := range ginRoutes {
			goKeys[rt.Method+" "+rt.Path] = true
		}
		var missing []string
		for _, nr := range nodeRoutes {
			key := nr.Method + " " + nr.Path
			if !goKeys[key] {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("Go 未注册 %d 条 Node 路由：", len(missing))
			for _, m := range missing {
				t.Log("  " + m)
			}
		} else {
			t.Logf("Node %d / Go %d → 覆盖率 100%%", len(nodeRoutes), len(ginRoutes))
		}
	})

	// --- 2. 真实 HTTP 探针 ---
	t.Run("http-probes", func(t *testing.T) {
		// Issue admin token
		var adminID int64
		var adminRoleID string
		err := database.QueryRow(`SELECT id, role_id FROM users WHERE username='admin' LIMIT 1`).Scan(&adminID, &adminRoleID)
		if err != nil {
			t.Fatalf("查 admin 用户失败: %v（测试库必须有 admin 账号）", err)
		}
		token, err := authSvc.Issue(adminID, adminRoleID)
		if err != nil {
			t.Fatalf("Issue token 失败: %v", err)
		}
		now := time.Now()
		_, _ = database.Exec(`DELETE FROM sessions WHERE token=?`, token)
		_, _ = database.Exec(
			`INSERT INTO sessions (token,user_id,role_id,created_at,expires_at) VALUES (?,?,?,?,?)`,
			token, adminID, adminRoleID,
			now.Format("2006-01-02 15:04:05"),
			now.Add(cfg.JWTTTL).Format("2006-01-02 15:04:05"),
		)
		t.Logf("admin user_id=%d role_id=%s token=%s...OK", adminID, adminRoleID, token[:24])

		ts := httptest.NewServer(r)
		defer ts.Close()
		client := &http.Client{Timeout: 5 * time.Second}

		projectID := firstProjectID(database)
		taskID := firstTaskID(database)
		sysName := firstImpactSys(database)
		t.Logf("test refs project_id=%q task_id=%q sys=%q", projectID, taskID, sysName)

		probes := buildProbes(projectID, taskID, sysName)
		t.Logf("probe 总数: %d", len(probes))

		var pass, fail int
		for i, p := range probes {
			status, bodyStr, err := doProbe(client, ts, token, p)
			if err != nil {
				fail++
				t.Errorf("[%02d][FAIL] %s %-55s → conn err: %v", i+1, p.Method, p.Path, err)
				continue
			}
			if status == 404 && !acceptsCode(p.AcceptCodes, 404) {
				fail++
				t.Errorf("[%02d][FAIL] %s %-55s → **404**  %s", i+1, p.Method, p.Path, trunc(bodyStr, 120))
				continue
			}
			if status >= 500 {
				fail++
				t.Errorf("[%02d][FAIL] %s %-55s → **%d**  %s", i+1, p.Method, p.Path, status, trunc(bodyStr, 120))
				continue
			}
			if len(p.AcceptCodes) > 0 {
				ok := false
				for _, c := range p.AcceptCodes {
					if status == c {
						ok = true
						break
					}
				}
				if !ok {
					fail++
					t.Errorf("[%02d][FAIL] %s %-55s → **%d** 不在 %v  %s", i+1, p.Method, p.Path, status, p.AcceptCodes, trunc(bodyStr, 120))
					continue
				}
			}
			if len(p.WantFields) > 0 {
				var parsed map[string]any
				if jerr := json.Unmarshal([]byte(bodyStr), &parsed); jerr != nil {
					fail++
					t.Errorf("[%02d][FAIL] %s %-55s → %d 非JSON  %s", i+1, p.Method, p.Path, status, trunc(bodyStr, 120))
					continue
				}
				mf := []string{}
				for _, f := range p.WantFields {
					if _, ex := parsed[f]; !ex {
						mf = append(mf, f)
					}
				}
				if len(mf) > 0 {
					fail++
					t.Errorf("[%02d][FAIL] %s %-55s → %d 缺字段 %v  %s", i+1, p.Method, p.Path, status, mf, trunc(bodyStr, 120))
					continue
				}
			}
			pass++
			t.Logf("[%02d][OK]   %s %-55s → %d  %s", i+1, p.Method, p.Path, status, trunc(bodyStr, 100))
		}

		t.Logf("=== 结果: pass=%d  fail=%d  total=%d ===", pass, fail, len(probes))
		if fail > 0 {
			t.Fatalf("%d 条探针失败", fail)
		}
	})
}

func doProbe(c *http.Client, ts *httptest.Server, token string, p probe) (int, string, error) {
	var rdr io.Reader
	if p.Body != "" {
		rdr = strings.NewReader(p.Body)
	}
	req, err := http.NewRequest(p.Method, ts.URL+p.Path, rdr)
	if err != nil {
		return 0, "", err
	}
	if p.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.RequireAuth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func acceptsCode(codes []int, c int) bool {
	for _, x := range codes {
		if x == c {
			return true
		}
	}
	return false
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- 引用 ID ---
func firstProjectID(db *db.DB) string {
	var id string
	_ = db.QueryRow(`SELECT id FROM projects LIMIT 1`).Scan(&id)
	if id == "" {
		now := time.Now().Format("2006-01-02 15:04:05")
		id = "verify_proj_001"
		_, _ = db.Exec(
			`INSERT INTO projects (id,name,type,date,status,mode,created_at) VALUES (?,?,?,?,?,?,?)`,
			id, "verify-routes 自动测试项目", "攻击链", now[:10], "未开始", "基础版", now,
		)
	}
	return id
}

func firstTaskID(db *db.DB) string {
	var id string
	if db.QueryRow(`SELECT id FROM tasks LIMIT 1`).Scan(&id) != nil {
		now := time.Now().Format("2006-01-02 15:04:05")
		id = "verify_task_001"
		pid := firstProjectID(db)
		_, _ = db.Exec(
			`INSERT INTO tasks (id,project_id,name,role_id,level,status,created_at) VALUES (?,?,?,?,?,?,?)`,
			id, pid, "verify-routes 自动测试任务", "platformadmin", "P2", "待处理", now,
		)
	}
	return id
}

func firstImpactSys(db *db.DB) string {
	var s string
	_ = db.QueryRow(`SELECT sys FROM impact LIMIT 1`).Scan(&s)
	if s == "" {
		s = "HIS收费系统"
	}
	return s
}

func setupTestDBIfNeeded(t *testing.T) {
	t.Helper()
	const dbName = "S99_AG_END_verify_routes"
	check := exec.Command("mysql", "--host=127.0.0.1", "--port=3306", "--user=root",
		"--database="+dbName, "-e", "SELECT COUNT(*) FROM roles;")
	if _, err := check.CombinedOutput(); err == nil {
		t.Logf("测试库 %s 已存在可用，跳过建库", dbName)
		return
	}
	t.Logf("测试库 %s 不存在，DROPCREATE + migrations + seed…", dbName)
	drop := exec.Command("mysql", "--host=127.0.0.1", "--port=3306", "--user=root",
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", dbName, dbName))
	if b, err := drop.CombinedOutput(); err != nil {
		t.Fatalf("DROP/CREATE 失败: %v  %s", err, string(b))
	}
	absRoot, _ := filepath.Abs("../../../")
	runSQL := func(file string) {
		src := filepath.Join(absRoot, "migrations", file)
		c := exec.Command("mysql", "--host=127.0.0.1", "--port=3306", "--user=root", "--database="+dbName)
		in, _ := os.Open(src)
		c.Stdin = in
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("migration %s 失败: %v  %s", file, err, string(b))
		}
	}
	runSQL("0001_init.up.sql")
	runSQL("0002_seed_impact.sql")
	runSQL("0003_login_lock.up.sql")
	seedFile := filepath.Join(absRoot, "_seed_dump2.sql")
	if _, err := os.Stat(seedFile); err == nil {
		c := exec.Command("mysql", "--host=127.0.0.1", "--port=3306", "--user=root", "--database="+dbName)
		in, _ := os.Open(seedFile)
		c.Stdin = in
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("seed restore 失败: %v  %s", err, string(b))
		}
	}
	t.Log("测试库就绪")
}

func buildProbes(projectID, taskID, sysName string) []probe {
	return []probe{
		// 免登录
		{Method: "GET", Path: "/api/health", RequireAuth: false, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/auth/roles", RequireAuth: false, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/auth/accounts", RequireAuth: false, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/system/brand", RequireAuth: false, AcceptCodes: []int{200}},

		// bootstrap
		{Method: "GET", Path: "/api/bootstrap", RequireAuth: true, AcceptCodes: []int{200}},

		// === 用户明确报错的两个场景 ===
		{Method: "GET", Path: "/api/content/messages", RequireAuth: true, AcceptCodes: []int{200}, Label: "站内消息列表（原报错点）"},
		{Method: "GET", Path: "/api/content/notifs", RequireAuth: true, AcceptCodes: []int{200}, Label: "通知列表（原报错点）"},
		{Method: "POST", Path: "/api/content/notifs/read", Body: `{"ids":[]}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/content/messages", Body: `{"text":"verify-routes 自动测试"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "GET", Path: "/api/content/evaluation", RequireAuth: true, AcceptCodes: []int{200}, Label: "复盘评估（原报错点）"},
		{Method: "POST", Path: "/api/content/evaluation/scores", Body: `{"projectId":"P2026-001","scores":[{"itemId":"K1","score":8,"max":10,"note":"verify-routes 自动评分"}]}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// 成员/角色/项目
		{Method: "GET", Path: "/api/content/users", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/roles", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/projects", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/projects/" + projectID + "/activate", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "PATCH", Path: "/api/projects/" + projectID + "/status", Body: `{"status":"演练中"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// 内容区
		{Method: "GET", Path: "/api/content/scenarios", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/script-nodes", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/drill-progress", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"project"}},
		{Method: "GET", Path: "/api/content/impact", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/dashboard", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/monitor", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/dict", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/sim-data", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/timeline", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/ledger", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/opinion", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/snapshots", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/seqflow", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/knowledge", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/audit", RequireAuth: true, AcceptCodes: []int{200}},

		// 写操作
		{Method: "POST", Path: "/api/content/audit", Body: `{"action":"verify-routes 自动"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/content/snapshots", Body: `{"name":"verify-routes 自动"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/content/opinion", Body: `{"channel":"测试","title":"verify-routes 自动"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "PATCH", Path: "/api/content/impact/" + sysName, Body: `{"state":"正常","pct":100}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// 项目操作
		{Method: "GET", Path: "/api/content/projects/" + projectID + "/script", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/projects/" + projectID + "/script/export", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/projects/" + projectID + "/notify/export", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/content/projects/" + projectID + "/notify", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "PATCH", Path: "/api/content/projects/" + projectID + "/members", Body: `{"members":[]}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// Chat
		{Method: "GET", Path: "/api/content/chat/channels", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/content/chat/room1", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/content/chat/room1", Body: `{"text":"verify-routes"}`, RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// tasks 旧路径
		{Method: "GET", Path: "/api/tasks", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/tasks/reset-all", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/tasks/" + taskID + "/accept", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/tasks/" + taskID + "/complete", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "GET", Path: "/api/tasks/" + taskID + "/attachments", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/tasks/" + taskID + "/forms", RequireAuth: true, AcceptCodes: []int{200}},

		// evidence 旧路径
		{Method: "GET", Path: "/api/evidence", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/evidence/requirements", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/evidence/prefill", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "POST", Path: "/api/evidence/mock", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},

		// drill v1 旧路径
		{Method: "GET", Path: "/api/drill/workbench/" + projectID, RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/drill/nodes/N01/start", RequireAuth: true, AcceptCodes: []int{200}, WantFields: []string{"ok"}},
		{Method: "GET", Path: "/api/drill/flows/" + projectID, RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/drill/faults/" + projectID, RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "POST", Path: "/api/drill/skip", Body: `{"projectId":"` + projectID + `","nodeId":"N01","reason":"跳过测试"}`, RequireAuth: true, AcceptCodes: []int{200, 400, 404, 403, 409}},
		{Method: "POST", Path: "/api/drill/decide", Body: `{"projectId":"` + projectID + `","nodeId":"N01","measure":"测试措施不少于六个字","result":"测试结果"}`, RequireAuth: true, AcceptCodes: []int{200, 400, 404, 403, 409}},
		{Method: "POST", Path: "/api/drill/finish", Body: `{"projectId":"` + projectID + `"}`, RequireAuth: true, AcceptCodes: []int{200, 400, 404, 403}},
		{Method: "POST", Path: "/api/drill/steps", Body: `{"projectId":"` + projectID + `","title":"测试自主步骤"}`, RequireAuth: true, AcceptCodes: []int{200, 400, 404}},
		{Method: "GET", Path: "/api/drill/predecessor?nodeId=N01&projectId=" + projectID, RequireAuth: true, AcceptCodes: []int{200}},

		// admin
		{Method: "GET", Path: "/api/admin/users", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/roles", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/forms", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/sysinfo", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/backups", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/report/" + projectID, RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/admin/audit/export", RequireAuth: true, AcceptCodes: []int{200}},

		// system
		{Method: "GET", Path: "/api/system/status", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/system/seed/baselines", RequireAuth: true, AcceptCodes: []int{200}},
		{Method: "GET", Path: "/api/system/dict/system_mode", RequireAuth: true, AcceptCodes: []int{200}},
	}
}
