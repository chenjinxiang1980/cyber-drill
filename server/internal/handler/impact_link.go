package handler

import (
	"database/sql"
	"sort"
	"strings"

	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// =========================================================================
// 业务系统影响：统一状态词表、写入联动、读取投影
// -------------------------------------------------------------------------
// 设计要点（对应 S01 问题 A 的裁决）：
//  1. impact 表只做「当前态投影」，大屏/看板读它；impact_events 做 append-only 溯源。
//  2. 同一系统多次标记以最后一条为准（主键 (sys, project_id) 覆盖 + 事件表追加）。
//  3. 无任何数据时默认「正常」（source=baseline），不写库，避免开局假故障。
//  4. 大屏（/api/content/monitor）与看板（/api/content/impact）共用 impactSnapshot。
// =========================================================================

// 五档状态词（统一词表）
const (
	ImpactNormal   = "正常"    // pct 100
	ImpactFault    = "故障"    // pct 30
	ImpactIsolated = "隔离中"   // pct 20
	ImpactDRSwitch = "灾备切换中" // pct 60
	ImpactVerify   = "恢复验证中" // pct 85
)

// impactDefaultSystems 与全局模板 6 行保持一致（顺序即展示顺序）
var impactDefaultSystems = []string{
	"HIS收费系统", "医护工作站", "电子病历系统",
	"临床检验系统", "医学影像系统", "办公OA系统",
}

// impactPctOf 状态词 → 展示百分比（大屏/看板着色阈值：>=100 绿、>=60 黄、其余红）
func impactPctOf(state string) int {
	switch strings.TrimSpace(state) {
	case ImpactNormal:
		return 100
	case ImpactFault:
		return 30
	case ImpactIsolated:
		return 20
	case ImpactDRSwitch:
		return 60
	case ImpactVerify:
		return 85
	}
	return 100
}

// normalizeImpactState 历史值/表单值 → 五档词。返回空串表示无法识别，调用方跳过。
// 映射表（旧值 → 新值）：
//
//	正常 / 正常运行 / 已恢复 / 恢复正常 → 正常（100）
//	故障 / 受影响 / 中断 / 不可用        → 故障（30）
//	隔离中 / 隔离                        → 隔离中（20）
//	灾备切换中 / 切换中                  → 灾备切换中（60）
//	恢复验证中 / 验证中                  → 恢复验证中（85）
func normalizeImpactState(s string) string {
	switch strings.TrimSpace(s) {
	case "正常", "正常运行", "已恢复", "恢复正常":
		return ImpactNormal
	case "故障", "受影响", "中断", "不可用":
		return ImpactFault
	case "隔离中", "隔离":
		return ImpactIsolated
	case "灾备切换中", "切换中":
		return ImpactDRSwitch
	case "恢复验证中", "验证中":
		return ImpactVerify
	}
	return ""
}

// ImpactSource 一次状态变更的来源信息（项目/节点/表单/时间/操作人）
type ImpactSource struct {
	ProjectID    string
	TaskID       string
	NodeID       string
	FormID       string
	FormName     string
	SubmissionID int64
	Operator     string
	RoleID       string
	At           string
}

// impactQuerier 统一 *db.DB 与 *sql.Tx 的查询能力
type impactQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// impactDefaultSeq 取全局模板行（project_id=''）的展示序号；查不到用 99。
// 必须先在 Go 侧单独查询再以字面量插入：MySQL 不允许在
// INSERT ... ON DUPLICATE KEY UPDATE 中对目标表 impact 自身做子查询（ERROR 1093）。
func impactDefaultSeq(q impactQuerier, sys string) int {
	rs, err := q.Query(`SELECT seq FROM impact WHERE sys=? AND project_id='' LIMIT 1`, sys)
	if err != nil {
		return 99
	}
	defer rs.Close()
	if rs.Next() {
		var seq sql.NullInt64
		if rs.Scan(&seq) == nil && seq.Valid {
			return int(seq.Int64)
		}
	}
	return 99
}

// applyFormImpact 非事务版本（实战版 PostDrillDecide 复用，写 drill_steps 后调用）
func applyFormImpact(database *db.DB, src ImpactSource, formID string, data map[string]any) []string {
	changes := collectImpactChanges(formID, data)
	for _, ch := range changes {
		seq := impactDefaultSeq(database, ch.sys)
		_, _ = database.Exec(
			`INSERT INTO impact (sys,state,pct,seq,project_id) VALUES (?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE state=VALUES(state),pct=VALUES(pct)`,
			ch.sys, ch.state, ch.pct, seq, src.ProjectID)
		insertImpactEvent(database, src, ch.sys, ch.state, ch.pct)
	}
	return changes.systems()
}

// applyFormImpactTx 事务版本（基础版 PostTaskForms 使用，与 form_submissions 同事务）
func applyFormImpactTx(tx *sql.Tx, src ImpactSource, formID string, data map[string]any) error {
	changes := collectImpactChanges(formID, data)
	for _, ch := range changes {
		seq := impactDefaultSeq(tx, ch.sys)
		if _, err := tx.Exec(
			`INSERT INTO impact (sys,state,pct,seq,project_id) VALUES (?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE state=VALUES(state),pct=VALUES(pct)`,
			ch.sys, ch.state, ch.pct, seq, src.ProjectID); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO impact_events (project_id,sys,state,pct,source_form_id,source_form_name,
				source_task_id,source_node_id,submission_id,operator,role_id,note,created_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			src.ProjectID, ch.sys, ch.state, ch.pct, src.FormID, src.FormName,
			src.TaskID, src.NodeID, src.SubmissionID, src.Operator, src.RoleID, ch.note, src.At); err != nil {
			return err
		}
	}
	return nil
}

type impactChange struct {
	sys   string
	state string
	pct   int
	note  string
}

// collectImpactSystems 仅返回本次提交会改动的系统名（用于响应体回显）
func collectImpactSystems(formID string, data map[string]any) []string {
	return collectImpactChanges(formID, data).systems()
}

type impactChanges []impactChange

func (cs impactChanges) systems() []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.sys)
	}
	return out
}

// collectImpactChanges 按表单类型解析要写入的业务系统：
//   F1 事件初步核实单  → 「受影响系统」多选 → 故障
//   F3 业务影响评估表  → 「受影响业务系统」多选 → 故障（权威来源）
//   F7 业务恢复确认单  → 「××系统状态」select → 对应档位（已恢复=正常）
func collectImpactChanges(formID string, data map[string]any) impactChanges {
	if data == nil {
		return nil
	}
	var out impactChanges
	switch strings.ToUpper(strings.TrimSpace(formID)) {
	case "F1", "F3":
		for k, v := range data {
			if strings.Contains(k, "受影响系统") || strings.Contains(k, "受影响业务系统") {
				for _, sys := range parseImpactSystems(v) {
					out = append(out, impactChange{sys: sys, state: ImpactFault, pct: impactPctOf(ImpactFault), note: "表单标记受影响"})
				}
			}
		}
	case "F7":
		for k, v := range data {
			if !strings.HasSuffix(k, "系统状态") {
				continue
			}
			sys := strings.TrimSuffix(k, "状态")
			state := normalizeImpactState(toPlainString(v))
			if sys == "" || state == "" {
				continue
			}
			out = append(out, impactChange{sys: sys, state: state, pct: impactPctOf(state), note: "恢复确认单更新"})
		}
	}
	return out
}

// parseImpactSystems 多选值 → 系统名切片。兼容：
//   - multiselect 存值 "HIS收费系统、电子病历系统"
//   - [](any) / []string
//   - 换行/逗号/分号分隔
// 非模板内的系统名（如「院内业务系统（模拟地址 10.20.x.x）」）一律丢弃，避免污染 impact。
func parseImpactSystems(v any) []string {
	var raw []string
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		raw = t
	case []any:
		for _, item := range t {
			raw = append(raw, toPlainString(item))
		}
	default:
		raw = []string{toPlainString(v)}
	}
	known := make(map[string]bool, len(impactDefaultSystems))
	for _, s := range impactDefaultSystems {
		known[s] = true
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		for _, sys := range strings.FieldsFunc(part, func(r rune) bool {
			return r == '、' || r == ',' || r == '，' || r == ';' || r == '；' || r == '/' || r == '\n' || r == ' '
		}) {
			sys = strings.TrimSpace(sys)
			if sys == "" || seen[sys] || !known[sys] {
				continue
			}
			seen[sys] = true
			out = append(out, sys)
		}
	}
	return out
}

func toPlainString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return util.ToJSON(v)
}

func insertImpactEvent(database *db.DB, src ImpactSource, sys, state string, pct int) {
	_, _ = database.Exec(
		`INSERT INTO impact_events (project_id,sys,state,pct,source_form_id,source_form_name,
			source_task_id,source_node_id,submission_id,operator,role_id,note,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		src.ProjectID, sys, state, pct, src.FormID, src.FormName,
		src.TaskID, src.NodeID, src.SubmissionID, src.Operator, src.RoleID, "", src.At)
}

// logImpactTimeline 在项目时间线追加一条影响变更记录（大屏/审计可回溯）
func logImpactTimeline(database *db.DB, projectID, action, detail string) {
	_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
		projectID, hhmm(), "system", "", action, detail, "", nil)
}

// =========================================================================
// 读取投影：项目实例行优先 → 全局模板补齐 → 都没有则内存基线（正常）
// =========================================================================

func impactSnapshot(database *db.DB, projectID string) []gin.H {
	type row struct {
		sys, state, source string
		pct, seq           int
	}
	seen := make(map[string]bool)
	rows := make([]row, 0, 8)

	collect := func(q, source string, args ...any) {
		rs, err := database.Query(q, args...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var sys, state sql.NullString
			var pct, seq sql.NullInt64
			if rs.Scan(&sys, &state, &pct, &seq) != nil {
				continue
			}
			name := util.ScanNullableString(sys)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			st := normalizeImpactState(util.ScanNullableString(state))
			p := nullInt(pct)
			if st == "" {
				st = ImpactNormal
				p = 100
			}
			if p == 0 && st != ImpactIsolated {
				p = impactPctOf(st)
			}
			rows = append(rows, row{sys: name, state: st, source: source, pct: p, seq: nullInt(seq)})
		}
	}

	if projectID != "" {
		collect(`SELECT sys,state,pct,seq FROM impact WHERE project_id=? ORDER BY seq`, "project", projectID)
	}
	collect(`SELECT sys,state,pct,seq FROM impact WHERE project_id='' ORDER BY seq`, "template")

	// 基线兜底：未被任何表单声明过的系统 = 正常（不写库）
	for i, sys := range impactDefaultSystems {
		if seen[sys] {
			continue
		}
		seen[sys] = true
		rows = append(rows, row{sys: sys, state: ImpactNormal, source: "baseline", pct: 100, seq: i})
	}

	// 最近一次变更时间（溯源展示用；表不存在时静默跳过）
	lastAt := map[string]string{}
	if projectID != "" {
		if rs, err := database.Query(`SELECT sys,MAX(created_at) FROM impact_events WHERE project_id=? GROUP BY sys`, projectID); err == nil {
			for rs.Next() {
				var sys, at sql.NullString
				if rs.Scan(&sys, &at) == nil {
					lastAt[util.ScanNullableString(sys)] = util.ScanNullableString(at)
				}
			}
			rs.Close()
		}
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].seq < rows[j].seq })
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		item := gin.H{
			"sys": r.sys, "state": r.state, "pct": r.pct, "source": r.source,
			// declared=false 表示当前态来自模板/基线，尚未被本次演练的表单实际声明过
			"declared": r.source == "project",
		}
		if at := lastAt[r.sys]; at != "" {
			item["updatedAt"] = at
		}
		out = append(out, item)
	}
	return out
}
