package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ========== P2-4 Drill Core ==========

var drillModes = []string{"基础版", "实战版"}

func modeAllowed(m string) bool {
	for _, v := range drillModes {
		if v == m {
			return true
		}
	}
	return false
}

// -------------------- POST /api/content/projects --------------------

type projectCreateIn struct {
	Name       string           `json:"name"`
	Date       string           `json:"date"`
	ScenarioID string           `json:"scenarioId"`
	Type       string           `json:"type"`
	Level      string           `json:"level"`
	Target     string           `json:"target"`
	Mode       string           `json:"mode"`
	Units      []string         `json:"units"`
	Members    []map[string]any `json:"members"`
	Observers  []map[string]any `json:"observers"`
}

// PostProject 新建演练项目（与 Node content.js:800 语义一致）
// 所有写操作包裹在单一事务中：INSERT projects + ensureBasicTasks + copyImpact + timeline。
// 任何一步失败 → 全量回滚，无半截数据。
func PostProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		roleID := ""
		if u != nil {
			roleID = u.Role.ID
		}
		if roleID != "platformadmin" && roleID != "commander" && roleID != "itlead" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可新建演练项目"})
			return
		}

		var b projectCreateIn
		if err := c.ShouldBindJSON(&b); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目数据格式错误"})
			return
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目名称不能为空"})
			return
		}
		if len(name) > 80 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目名称不能超过 80 个字符"})
			return
		}
		date := strings.TrimSpace(b.Date)
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(date) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "计划日期格式应为 YYYY-MM-DD"})
			return
		}
		scenarioID := strings.TrimSpace(b.ScenarioID)
		if scenarioID != "" {
			var exists int
			if err := database.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE id=?`, scenarioID).Scan(&exists); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败"})
				return
			}
			if exists == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "关联剧本不存在：" + scenarioID})
				return
			}
		}
		mode := strings.TrimSpace(b.Mode)
		if mode == "" {
			mode = "基础版"
		}
		if !modeAllowed(mode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练模式应为 基础版 或 实战版"})
			return
		}

		// 生成项目编号
		year := time.Now().Format("2006")
		var ids []string
		rows, qErr := database.Query(`SELECT id FROM projects`)
		if qErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "项目列表查询失败"})
			return
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err == nil {
				ids = append(ids, s)
			}
		}
		rows.Close()
		maxSeq := 0
		re := regexp.MustCompile(`^P` + year + `-(\d+)$`)
		for _, s := range ids {
			if m := re.FindStringSubmatch(s); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n > maxSeq {
					maxSeq = n
				}
			}
		}
		id := "P" + year + "-" + pad3(maxSeq+1)

		// 默认值
		var level string
		if scenarioID != "" {
			_ = database.QueryRow(`SELECT COALESCE(level,'') FROM scenarios WHERE id=?`, scenarioID).Scan(&level)
		}
		if level == "" {
			level = strings.TrimSpace(b.Level)
		}
		if level == "" {
			level = "一般"
		}

		var defaultUnit string
		_ = database.QueryRow("SELECT COALESCE(value,'') FROM settings WHERE `key`='brand_unit_name'").Scan(&defaultUnit)
		if defaultUnit == "" {
			defaultUnit = "示例医院"
		}
		units := b.Units
		if len(units) == 0 {
			units = []string{defaultUnit}
		}
		unitsJSON, _ := json.Marshal(units)
		membersJSON, _ := json.Marshal(b.Members)
		observersJSON, _ := json.Marshal(b.Observers)

		// ========== 单一事务包裹所有写操作 ==========
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()

		// 1. INSERT projects
		if _, err := tx.Exec(`INSERT INTO projects (id,name,type,date,scenario_id,level,status,target,units,members,observers,mode) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, name, emptyOr(b.Type, "实战协同演练"), date, scenarioID, level, "未开始",
			strings.TrimSpace(b.Target),
			string(unitsJSON), string(membersJSON), string(observersJSON), mode); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建项目失败：" + err.Error()})
			return
		}

		// 2. ensureBasicTasks（基础版 + 有剧本才生成）
		var taskCount int
		if mode == "基础版" && scenarioID != "" {
			taskCount, err = ensureBasicTasksTx(tx, id, scenarioID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "自动生成任务失败：" + err.Error()})
				return
			}
		}

		// 3. copyImpact 模板
		if err := copyImpactTx(tx, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化业务系统状态失败：" + err.Error()})
			return
		}

		// 3.5 复制项目核心证据清单（平台默认，项目内可调整）
		if err := seedProjectCoreEvidenceTx(tx, database, id, scenarioID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化核心证据清单失败：" + err.Error()})
			return
		}

		// 4. timeline 审计
		now := time.Now().Format("15:04")
		if _, err := tx.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, now, userNameFromContext(c), roleID, "创建演练项目",
			"项目「"+name+"」（"+mode+"，关联剧本 "+scenarioName(database, scenarioID)+"，计划日期 "+date+"）已创建",
			"", nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入审计日志失败：" + err.Error()})
			return
		}

		// 5. Commit
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存项目失败：" + err.Error()})
			return
		}

		// 6. 站内消息：演练项目已创建（Node content.js:856-868）
		scName := "未关联剧本"
		if scenarioID != "" {
			scName = scenarioID
			var sname string
			if database.QueryRow(`SELECT COALESCE(name,'') FROM scenarios WHERE id=?`, scenarioID).Scan(&sname) == nil && sname != "" {
				scName = sname
			}
		}
		targetText := strings.TrimSpace(b.Target)
		if targetText == "" {
			targetText = "按剧本预案完成全流程处置"
		}
		nLevel := "P1"
		if level == "重大" {
			nLevel = "P0"
		}
		pushNotif(database, "project_created", map[string]any{
			"project": name, "projectId": id, "mode": mode, "scenario": scName,
			"level": level, "date": date, "target": truncRunes(targetText, 40),
		}, id, nLevel, "", "internal")

		// 读回项目返回（Commit 之后）
		var pname, pscenario, plevel, pdate, ptarget, pmode, pstatus string
		var punits, pmembers, pobservers string
		_ = database.QueryRow(`SELECT COALESCE(name,''),COALESCE(scenario_id,''),COALESCE(level,'一般'),COALESCE(date,''),COALESCE(target,''),COALESCE(mode,'基础版'),COALESCE(status,''),COALESCE(units,'[]'),COALESCE(members,'[]'),COALESCE(observers,'[]') FROM projects WHERE id=?`, id).
			Scan(&pname, &pscenario, &plevel, &pdate, &ptarget, &pmode, &pstatus, &punits, &pmembers, &pobservers)

		c.JSON(http.StatusOK, gin.H{
			"ok":        true,
			"id":        id,
			"taskCount": taskCount,
			"project": gin.H{
				"id":         id,
				"name":       pname,
				"scenarioId": pscenario,
				"level":      plevel,
				"date":       pdate,
				"target":     ptarget,
				"mode":       pmode,
				"status":     pstatus,
				"units":      parseJSONStr(punits),
				"members":    parseJSONStr(pmembers),
				"observers":  parseJSONStr(pobservers),
			},
		})
	}
}

func pad3(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func emptyOr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func userNameFromContext(c *gin.Context) string {
	if v, ok := c.Get("username"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "system"
}

func scenarioName(database *db.DB, scenarioID string) string {
	if scenarioID == "" {
		return "无"
	}
	var name string
	database.QueryRow(`SELECT COALESCE(name,'') FROM scenarios WHERE id=?`, scenarioID).Scan(&name)
	if name == "" {
		return scenarioID
	}
	return name
}

func parseJSONStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return []any{}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return []any{}
	}
	return v
}

// ensureBasicTasksTx 基础版演练自动为每个主流节点生成一条任务（事务内版本）。
// 任何一条 INSERT 失败 → 返回 error → 上层事务回滚。
// 注意：事务内禁止在 rows.Next() 循环中再开 tx.Query —— 会触发
// MySQL "commands out of sync"。所有 role 查找预先加载到 map。
func ensureBasicTasksTx(tx *sql.Tx, projectID, scenarioID string) (int, error) {
	// 已有任务则跳过
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id=?`, projectID).Scan(&exists); err != nil {
		return 0, fmt.Errorf("查询已有任务失败: %w", err)
	}
	if exists > 0 {
		return 0, nil
	}

	// 预加载 roles 映射 —— 避免 rows.Next() 内嵌套 Query
	roleByKey := make(map[string]string)
	roleRows, _ := tx.Query(`SELECT id,COALESCE(name,''),COALESCE(short,'') FROM roles`)
	for roleRows.Next() {
		var id, name, short string
		roleRows.Scan(&id, &name, &short)
		roleByKey[strings.TrimSpace(name)] = id
		roleByKey[strings.TrimSpace(short)] = id
	}
	roleRows.Close()

	// 一次性读所有 script_nodes 到内存
	type node struct {
		id, typ, title, descr, form, target string
	}
	var nodes []node
	rows, err := tx.Query(`SELECT id,type,title,COALESCE(`+"`descr`"+`,''),COALESCE(form,''),COALESCE(target,'') FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND type!='end' AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq`, scenarioID)
	if err != nil {
		return 0, fmt.Errorf("查询剧本节点失败: %w", err)
	}
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.id, &n.typ, &n.title, &n.descr, &n.form, &n.target); err != nil {
			rows.Close()
			return 0, fmt.Errorf("读取节点行失败: %w", err)
		}
		nodes = append(nodes, n)
	}
	rows.Close()

	prefix := "T" + strings.ReplaceAll(projectID, "P", "") + "-"
	count := 0

	level := "P2"
	var scLevel string
	_ = tx.QueryRow(`SELECT COALESCE(level,'一般') FROM scenarios WHERE id=?`, scenarioID).Scan(&scLevel)
	if scLevel == "重大" {
		level = "P0"
	} else if scLevel == "较大" {
		level = "P1"
	}

	for _, n := range nodes {
		roleID := resolveRoleFromMap(roleByKey, n.target)
		forms := []string{}
		if strings.TrimSpace(n.form) != "" {
			forms = append(forms, strings.TrimSpace(n.form))
		}
		formsJSON, _ := json.Marshal(forms)
		taskID := prefix + pad2(count+1)
		if _, err := tx.Exec(`INSERT INTO tasks (id,project_id,role_id,title,descr,deadline,status,level,forms,node_id,seq) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			taskID, projectID, roleID, n.title, strings.TrimSpace(n.descr),
			"", "待处理", level, string(formsJSON), n.id, count+1); err != nil {
			return count, fmt.Errorf("插入任务 %s 失败: %w", taskID, err)
		}
		count++
	}
	return count, nil
}

// resolveRoleFromMap — 在预加载的 role map 中查 role_id。
// 顺序：精确 name/short → 别名（向后兼容）→ 模糊匹配。
// 正式角色行存在时精确匹配优先，别名只在旧库/旧数据缺角色行时兜底。
func resolveRoleFromMap(roleByKey map[string]string, target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return "duty"
	}
	if id, ok := roleByKey[t]; ok {
		return id
	}
	if id, ok := targetRoleAliases[t]; ok {
		return id
	}
	// 模糊匹配
	for name, id := range roleByKey {
		if strings.Contains(name, t) || strings.Contains(t, name) {
			return id
		}
	}
	return t
}

// targetToRoleIDTx — 事务内版本，直接查 roles 表（等价于原 targetToRoleID 的逻辑）。
// 顺序：精确 name/short → 别名（向后兼容）→ 模糊匹配。
func targetToRoleIDTx(tx *sql.Tx, target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return "duty"
	}
	var id string
	_ = tx.QueryRow(`SELECT id FROM roles WHERE name=? OR short=? LIMIT 1`, t, t).Scan(&id)
	if id != "" {
		return id
	}
	if id, ok := targetRoleAliases[t]; ok {
		return id
	}
	// 模糊匹配
	rows, err := tx.Query(`SELECT id,name FROM roles WHERE name LIKE ? OR short LIKE ?`, "%"+t+"%", "%"+t+"%")
	if err != nil {
		return t
	}
	defer rows.Close()
	for rows.Next() {
		var rid, rname string
		if err := rows.Scan(&rid, &rname); err == nil {
			return rid
		}
	}
	return t
}

// targetRoleAliases 剧本里按科室/岗位书写的责任对象 → 角色的向后兼容别名。
// 这些科室/岗位已在迁移 0127 中落为正式角色行（检验科主任/医保办/门诊部），
// 正常解析走「精确 name/short 匹配」优先；别名仅用于旧库尚未执行 0127、
// 或历史导入数据缺角色行时兜底，避免必做节点没有对口角色可接单。
// 0128 起「运维负责人 / 算法工程师」两个角色行已删除，旧 target 仅作为
// 历史修订快照（script_revisions）的兼容映射指向新角色，不再对应独立角色。
var targetRoleAliases = map[string]string{
	"检验科主任": "medlead",
	"医保办":   "medlead",
	"门诊部":   "medlead",
	"运维负责人": "itlead",
	"算法工程师": "sysadmin",
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

// targetToRoleID 复刻 Node drill_plan.js targetToRoleId：
// 精确 name/short → 别名（向后兼容）→ 双向包含匹配（唯一命中）→ 兜底 duty
func targetToRoleID(database *db.DB, target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return "duty"
	}
	var id string
	if err := database.QueryRow(`SELECT id FROM roles WHERE name=? OR short=? LIMIT 1`, t, t).Scan(&id); err == nil && id != "" {
		return id
	}
	if id, ok := targetRoleAliases[t]; ok {
		return id
	}
	rows, err := database.Query(
		`SELECT id FROM roles WHERE ? LIKE CONCAT('%',name,'%') OR ? LIKE CONCAT('%',short,'%')
		   OR name LIKE CONCAT('%',?,'%') OR short LIKE CONCAT('%',?,'%')`,
		t, t, t, t,
	)
	if err == nil {
		defer rows.Close()
		ids := make([]string, 0, 4)
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				ids = append(ids, v)
			}
		}
		if len(ids) == 1 {
			return ids[0]
		}
	}
	return "duty"
}

// copyImpactTx 从全局模板 (project_id=”) 复制业务系统状态到新项目（事务内版本）
// 修复根因：MySQL 不允许一个连接上 SELECT 结果集未关就发 INSERT —— 必须先 rows.Close() 再循环 Exec
// 修复 v2: 当全局模板 project_id=” 为空时，用硬编码默认值初始化模板再复制（对齐 Node seed_data.js IMPACT）
func copyImpactTx(tx *sql.Tx, projectID string) error {
	if _, err := tx.Exec(`DELETE FROM impact WHERE project_id=?`, projectID); err != nil {
		return fmt.Errorf("清理旧业务系统状态失败: %w", err)
	}

	rows, err := tx.Query(`SELECT sys,state,pct,seq FROM impact WHERE project_id='' ORDER BY seq`)
	if err != nil {
		return fmt.Errorf("查询模板业务系统状态失败: %w", err)
	}
	// Step 1: 全读进内存，立即关闭结果集，释放连接
	type impRow struct {
		sys, state string
		pct, seq   int
	}
	var templateRows []impRow
	for rows.Next() {
		var r impRow
		if err := rows.Scan(&r.sys, &r.state, &r.pct, &r.seq); err != nil {
			rows.Close()
			return fmt.Errorf("读取模板行失败: %w", err)
		}
		templateRows = append(templateRows, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("遍历模板行失败: %w", err)
	}
	rows.Close() // ⚠️ 必须在循环 INSERT 之前关闭！

	// Step 1.5: 模板为空时，从硬编码默认值初始化（对齐 Node seed_data.js IMPACT）
	// 这是克隆部署后建第一个项目就会遇到的路径
	if len(templateRows) == 0 {
		defaults := []impRow{
			{"HIS收费系统", "正常", 100, 0},
			{"医护工作站", "正常", 100, 1},
			{"电子病历系统", "正常", 100, 2},
			{"临床检验系统", "正常", 100, 3},
			{"医学影像系统", "正常", 100, 4},
			{"办公OA系统", "正常", 100, 5},
		}
		for _, r := range defaults {
			if _, err := tx.Exec(`INSERT INTO impact (sys,state,pct,seq,project_id) VALUES (?,?,?,?,?)`,
				r.sys, r.state, r.pct, r.seq, ""); err != nil {
				return fmt.Errorf("写入默认模板行 %s 失败: %w", r.sys, err)
			}
		}
		templateRows = defaults
	}

	// Step 2: 在已关闭的连接上逐条 INSERT
	for _, r := range templateRows {
		if _, err := tx.Exec(`INSERT INTO impact (sys,state,pct,seq,project_id) VALUES (?,?,?,?,?)`,
			r.sys, r.state, r.pct, r.seq, projectID); err != nil {
			return fmt.Errorf("插入业务系统状态 %s 失败: %w", r.sys, err)
		}
	}
	return nil
}

// ==================== GetProjects (增强版) ====================

// GetProjectsList 列出所有项目（P2-4 新增）
func GetProjectsList(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, _ := database.Query(`SELECT id,COALESCE(name,''),COALESCE(date,''),COALESCE(scenario_id,''),COALESCE(level,'一般'),COALESCE(status,''),COALESCE(mode,'基础版'),COALESCE(started_at,''),COALESCE(paused_sec,0),COALESCE(ended_at,'') FROM projects ORDER BY id DESC`)
		defer rows.Close()
		out := []gin.H{}
		for rows.Next() {
			var id, name, date, scenarioID, level, status, mode, startedAt, endedAt string
			var pausedSec int
			rows.Scan(&id, &name, &date, &scenarioID, &level, &status, &mode, &startedAt, &pausedSec, &endedAt)
			out = append(out, gin.H{
				"id":         id,
				"name":       name,
				"date":       date,
				"scenarioId": scenarioID,
				"level":      level,
				"status":     status,
				"mode":       mode,
				"startedAt":  startedAt,
				"endedAt":    endedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
	}
}

// GetProjectScript 构建项目演练脚本（Node buildProjectScript 等价）
func GetProjectScript(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if strings.TrimSpace(id) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目ID不能为空"})
			return
		}
		var name, scenarioID, mode, status, level, membersJSON string
		err := database.QueryRow(`SELECT name,COALESCE(scenario_id,''),COALESCE(mode,'基础版'),COALESCE(status,''),COALESCE(level,'一般'),COALESCE(members,'[]') FROM projects WHERE id=?`, id).
			Scan(&name, &scenarioID, &mode, &status, &level, &membersJSON)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "项目查询失败"})
			return
		}
		if scenarioID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该项目未关联剧本"})
			return
		}
		// 查看门控（A-8）：实战版演练开始后，参演角色不可直接查看脚本；
		// 平台管理员（演练项目管理页）不受门控影响，可随时查看全部流程。
		u := auth.CurrentUser(c)
		isAdmin := u != nil && u.Role.ID == "platformadmin"
		forceOK := false
		if v, ok := c.Get("script_force_ok"); ok {
			forceOK, _ = v.(bool)
		}
		if mode == "实战版" && !isAdmin && statusStarted(status) && !forceOK {
			c.JSON(http.StatusForbidden, gin.H{
				"code":  "script_locked",
				"error": "演练已开始，演练脚本已锁定；如确需查看，请提交强制查看申请（系统将记录并扣 10 分）",
			})
			return
		}
		// 基础版、以及实战版开始前：只返回正确流程（过滤 end + distractor），与基础版口径一致。
		// 实战版管理员视角：返回全部处置流程（仅过滤 end），混淆流程作为并列选项。
		includeDistractors := mode == "实战版" && isAdmin
		nodeWhere := `scenario_id=? AND type!='end'`
		if !includeDistractors {
			nodeWhere += ` AND COALESCE(flow_type,'correct')!='distractor'`
		}
		rows, err := database.Query(`SELECT seq,id,type,title,COALESCE(target,''),COALESCE(action,''),COALESCE(`+"`descr`"+`,''),COALESCE(guide,''),COALESCE(keywords,''),COALESCE(score,0),COALESCE(receiver,''),COALESCE(evidence_req,'[]'),COALESCE(deps,'[]'),COALESCE(timeout,0),COALESCE(form,''),COALESCE(required,1),COALESCE(flow_type,'correct'),COALESCE(anchor_id,'') FROM script_nodes WHERE `+nodeWhere+` AND deleted_at IS NULL ORDER BY seq`, scenarioID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "剧本节点查询失败"})
			return
		}
		// 先全读进 slice，立即 Close，再循环处理。循环内不能再 QueryRow。
		type scriptNode struct {
			seq                                                                                                  int64
			nodeID, typ, title, target, action, descr, guide, keywords, receiver, evReq, depsStr, form, flowType string
			anchorID                                                                                             string
			score, timeout                                                                                       int
			required                                                                                             int64
		}
		var nodes []scriptNode
		for rows.Next() {
			var n scriptNode
			if err := rows.Scan(&n.seq, &n.nodeID, &n.typ, &n.title, &n.target, &n.action, &n.descr, &n.guide,
				&n.keywords, &n.score, &n.receiver, &n.evReq, &n.depsStr, &n.timeout, &n.form, &n.required, &n.flowType, &n.anchorID); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "剧本节点读取失败"})
				return
			}
			nodes = append(nodes, n)
		}
		rows.Close()

		// 预加载 roles map（target→roleID + roleID→roleName）
		roleByKey := make(map[string]string)    // name/short → roleID
		roleNameByID := make(map[string]string) // roleID → name
		var roleRows *sql.Rows
		roleRows, _ = database.Query(`SELECT id,COALESCE(name,''),COALESCE(short,'') FROM roles`)
		for roleRows.Next() {
			var rid, rname, rshort string
			roleRows.Scan(&rid, &rname, &rshort)
			roleNameByID[rid] = rname
			roleByKey[strings.TrimSpace(rname)] = rid
			roleByKey[strings.TrimSpace(rshort)] = rid
		}
		roleRows.Close()

		list := []gin.H{}
		for _, n := range nodes {
			roleID := resolveRoleFromMap(roleByKey, n.target)
			roleName := roleNameByID[roleID]
			if roleName == "" {
				roleName = n.target
			}
			base := gin.H{
				"seq":      n.seq,
				"id":       n.nodeID,
				"type":     n.typ,
				"title":    n.title,
				"role":     roleName,
				"roleId":   roleID,
				"action":   n.action,
				"deps":     parseDepsGo(n.depsStr),
				"timeout":  n.timeout,
				"flowType": n.flowType,
				"anchorId": n.anchorID,
			}
			if !includeDistractors {
				base["desc"] = n.descr
				base["guide"] = n.guide
				if kw := strings.TrimSpace(n.keywords); kw != "" {
					base["keywords"] = strings.Split(kw, ",")
				} else {
					base["keywords"] = []string{}
				}
				base["score"] = n.score
				base["receiver"] = n.receiver
				base["evidenceReq"] = parseJSONItems(n.evReq)
				base["form"] = n.form
				base["required"] = n.required == 1
			} else {
				// 实战版：不返回分值/是否必经，避免提前暴露正确流程。
				// 主流节点 guide 为空，混淆节点带上其原有研判说明，供前端并列展示。
				base["desc"] = n.descr
				base["guide"] = n.guide
			}
			list = append(list, base)
		}

		// 参演角色列表（与 Node buildProjectScript 的 members 结构一致）
		members := []gin.H{}
		var mArr []map[string]any
		if json.Unmarshal([]byte(membersJSON), &mArr) == nil {
			for _, m := range mArr {
				rid, _ := m["role"].(string)
				if rid == "" {
					rid, _ = m["roleId"].(string)
				}
				rname := roleNameByID[rid]
				if rname == "" {
					rname = rid
				}
				if rname == "" {
					continue
				}
				members = append(members, gin.H{"roleId": rid, "name": rname})
			}
		}

		// 剧本基础信息
		var scName, scType, scDescr, scAlert string
		database.QueryRow(`SELECT COALESCE(name,''),COALESCE(type,''),COALESCE(`+"`descr`"+`,''),COALESCE(alert,'') FROM scenarios WHERE id=?`, scenarioID).
			Scan(&scName, &scType, &scDescr, &scAlert)

		viewMode := "实战版"
		if !includeDistractors {
			viewMode = "基础版"
		}
		// 基础版：全程可看，但留一条非扣分类查看审计（A-8 要求）
		if mode == "基础版" {
			writeScriptAudit(database, c, u, id, "普通", "", false)
		}

		resp := gin.H{
			"ok":           true,
			"projectId":    id,
			"name":         name,
			"projectName":  name,
			"mode":         mode,
			"viewMode":     viewMode,
			"status":       status,
			"level":        level,
			"scenario":     gin.H{"id": scenarioID, "name": scName, "type": scType, "desc": scDescr, "alert": scAlert},
			"members":      members,
			"script":       list,
			"instructions": instructionsForMode(viewMode),
		}
		if v, ok := c.Get("script_force_deducted"); ok {
			resp["forceView"] = gin.H{"recorded": true, "deducted": v}
		}
		c.JSON(http.StatusOK, resp)
	}
}

// PostProjectScriptForceView POST /api/content/projects/:id/script/force
// 实战版演练开始后，参演角色强制查看脚本：校验原因、写审计、首次扣分（幂等），随后复用脚本渲染逻辑返回内容。
func PostProjectScriptForceView(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.ID == "platformadmin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "平台管理员可直接查看实战版脚本，无需强制查看"})
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)

		var mode, status string
		err := database.QueryRow(`SELECT COALESCE(mode,'基础版'),COALESCE(status,'') FROM projects WHERE id=?`, id).Scan(&mode, &status)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if mode != "实战版" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "基础版脚本可直接查看，无需强制查看"})
			return
		}
		if !statusForceAllowed(status) {
			c.JSON(http.StatusForbidden, gin.H{"error": "当前状态不允许强制查看（仅演练中/暂停时可申请）"})
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if len([]rune(reason)) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写不少于 8 个字的强制查看原因"})
			return
		}
		var already int
		_ = database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE ?`,
			"查看演练脚本|项目="+id+"|类型=强制|扣分=是|%").Scan(&already)
		deducted := already == 0

		writeScriptAudit(database, c, u, id, "强制", reason, deducted)

		c.Set("script_force_ok", true)
		c.Set("script_force_deducted", deducted)
		GetProjectScript(database)(c)
	}
}

// statusStarted 判断项目是否已开始（演练中/暂停/已结束/已归档均视为已开始）
func statusStarted(status string) bool {
	switch status {
	case "演练中", "暂停", "已结束", "已归档":
		return true
	}
	return false
}

// statusForceAllowed 强制查看仅在演练进行中（含暂停）允许
func statusForceAllowed(status string) bool {
	return status == "演练中" || status == "暂停"
}

// writeScriptAudit 写入脚本查看审计（audit_logs 既有表，action 内编码项目/类型/扣分/原因）
func writeScriptAudit(database *db.DB, c *gin.Context, u *auth.User, projectID, kind, reason string, deducted bool) {
	if u == nil {
		return
	}
	flag := "否"
	if deducted {
		flag = "是"
	}
	action := fmt.Sprintf("查看演练脚本|项目=%s|类型=%s|扣分=%s|原因=%s",
		projectID, kind, flag, truncateRunes(strings.TrimSpace(reason), 200))
	var uid any
	if u.ID > 0 {
		uid = u.ID
	}
	if _, err := database.Exec(`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
		time.Now().Format("2006-01-02 15:04:05"), u.Name, u.Role.ID, uid, action, c.ClientIP()); err != nil {
		// 审计失败不阻断脚本查看，但必须留痕
		logSideEffectErr("audit_logs", projectID, "查看演练脚本", err)
	}
}

// truncateRunes 按字符截断，避免 512 列宽溢出
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func instructionsForMode(mode string) []string {
	if mode == "实战版" {
		return []string{
			"本脚本按步骤组列出本次演练的全部处置动作，每个步骤组对应一个主流节点。",
			"组内动作顺序随机排列，系统不预先标注各动作的执行结果。",
			"参演角色进入角色工作台后，需结合现场情况在每个步骤组内自主研判并选择执行动作；超时完成扣本节点 10% 分值。",
		}
	}
	return []string{
		"本脚本为基础版演练完整脚本，包含全部主流节点的详细处置步骤与提示信息。",
		"参演角色按任务中心提示，依次完成各自负责的节点处置。",
		"系统按剧本时限自动评分，超时完成扣本节点 10% 分值。",
	}
}

func parseDepsGo(j string) []string {
	j = strings.TrimSpace(j)
	if j == "" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(j), &arr); err != nil {
		return []string{}
	}
	return arr
}

func parseJSONItems(j string) []any {
	j = strings.TrimSpace(j)
	if j == "" {
		return []any{}
	}
	var arr []any
	if err := json.Unmarshal([]byte(j), &arr); err != nil {
		return []any{}
	}
	return arr
}

// sortStrings 辅助
func sortStrings(arr []string) []string {
	sort.Strings(arr)
	return arr
}
