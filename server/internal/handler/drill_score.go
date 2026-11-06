package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"drill-system/internal/db"
)

// ScoreResult 纯函数 Score() 返回值（结构与 Node drill.js:162 对齐）
type ScoreResult struct {
	ProjectID      string         `json:"projectId"`
	Mode           string         `json:"mode"`
	ScenarioID     string         `json:"scenarioId"`
	Ended          bool           `json:"ended"`
	Total          int            `json:"total"`
	Hit            int            `json:"hit"`
	Ontime         int            `json:"ontime"`
	Missed         int            `json:"missed"`
	Pending        int            `json:"pending"`
	Wrong          int            `json:"wrong"`
	Late           int            `json:"late"`
	MaxScore       int            `json:"maxScore"`
	Distractors    int            `json:"distractors"`
	Skipped        int            `json:"skipped"`
	SkippedCorrect int            `json:"skippedCorrect"`
	SkippedDistract int           `json:"skippedDistract"`
	Completeness   float64        `json:"completeness"`
	TimeRate       float64        `json:"timeRate"`
	Score          float64        `json:"score"`
	Grade          string         `json:"grade"`
	Deductions     []Deduction    `json:"deductions"`
	Bonuses        []any          `json:"bonuses"`
	Formula        string         `json:"formula"`
	Nodes          []NodeResult   `json:"nodes"`
	Steps          []any          `json:"steps"`
	CorrectTotal   int            `json:"correctTotal"`
}

type Deduction struct {
	NodeID string  `json:"nodeId"`
	Title  string  `json:"title"`
	Type   string  `json:"type"`
	Point  float64 `json:"point"`
}

type NodeResult struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Role        string  `json:"role"`
	Plan        string  `json:"plan"`
	Actual      string  `json:"actual"`
	Required    bool    `json:"required"`
	Submitted   bool    `json:"submitted"`
	Hit         bool    `json:"hit"`
	Ontime      bool    `json:"ontime"`
	Wrong       bool    `json:"wrong"`
	Skipped     bool    `json:"skipped"`
	Score       int     `json:"score"`
	Earned      float64 `json:"earned"`
	Late        bool    `json:"late"`
	Receiver    string  `json:"receiver,omitempty"`
	ReceiverOk  bool    `json:"receiverOk"`
	State       string  `json:"state"`
	Distract    bool    `json:"distract,omitempty"`
}

// ScriptNodeForScore score() 内部使用的剧本节点轻量结构
type ScriptNodeForScore struct {
	ID           string
	Type         string
	Title        string
	Target       string
	Timeout      int
	Form         string
	FlowType     string // "correct" 或 "distractor"
	Score        int
	Receiver     string
	Deps         []string
	AnchorID     string
	ReleaseTiming string
	Keywords     []string
	EvidenceReq  string
	QualityRules string
	Action       string
	Desc         string
	Guide        string
	Wrong        []string
	Required     bool
}

// DrillStepForScore score() 内部使用的处置记录轻量结构
type DrillStepForScore struct {
	ID           int64
	MatchedNode  string
	Correct      int
	Ontime       int
	Skipped      int
	Receiver     string
	ReceiverOK   int
	StepTime     string
	CreatedAt    string
	FlowType     string
	Score        float64
	Target       string
	RoleID       string
	User         string
	Forms        string
	Measure      string
	Result       string
	Found        string
	Evidence     string
}

// ProjectForScore score() 内部使用的项目轻量结构
type ProjectForScore struct {
	ID         string
	ScenarioID string
	Mode       string
	Status     string
	StartedAt  string
	Date       string
}

// ========== 时间工具（复刻 src/drill_clock.js） ==========

func parseLocalTime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 尝试 YYYY-MM-DD HH:mm:ss
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.UnixMilli()
	}
	// 尝试 YYYY-MM-DD HH:mm
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t.UnixMilli()
	}
	// 尝试 YYYY-MM-DDTHH:mm:ss
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// ========== ScriptNode 查询 ==========

func loadNodesForScore(database *db.DB, scenarioID string) []ScriptNodeForScore {
	rows, err := database.Query(`SELECT id,type,title,COALESCE(target,''),COALESCE(timeout,0),COALESCE(form,''),
		COALESCE(flow_type,'correct'),COALESCE(score,0),COALESCE(receiver,''),COALESCE(deps,'[]'),
		COALESCE(anchor_id,''),COALESCE(release_timing,'before'),
		COALESCE(keywords,''),COALESCE(evidence_req,''),COALESCE(quality_rules,''),COALESCE(action,''),COALESCE(`+"`descr`"+`,''),COALESCE(guide,''),
		COALESCE(wrong,'[]'),COALESCE(required,1)
		FROM script_nodes WHERE scenario_id=? ORDER BY seq`, scenarioID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []ScriptNodeForScore
	for rows.Next() {
		var n ScriptNodeForScore
		var depsStr, wrongStr, keywords string
		var required int
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Target, &n.Timeout, &n.Form,
			&n.FlowType, &n.Score, &n.Receiver, &depsStr, &n.AnchorID, &n.ReleaseTiming,
			&keywords, &n.EvidenceReq, &n.QualityRules, &n.Action, &n.Desc, &n.Guide, &wrongStr, &required); err != nil {
			continue
		}
		if strings.EqualFold(n.FlowType, "distractor") {
			n.FlowType = "distractor"
		} else {
			n.FlowType = "correct"
		}
		if strings.TrimSpace(depsStr) != "" {
			_ = json.Unmarshal([]byte(depsStr), &n.Deps)
		}
		if strings.TrimSpace(wrongStr) != "" {
			_ = json.Unmarshal([]byte(wrongStr), &n.Wrong)
		}
		if strings.TrimSpace(keywords) != "" {
			for _, kw := range strings.Split(keywords, ",") {
				if kw = strings.TrimSpace(kw); kw != "" {
					n.Keywords = append(n.Keywords, kw)
				}
			}
		}
		n.Required = required == 1
		out = append(out, n)
	}
	return out
}

func loadStepsForScore(database *db.DB, projectID string) []DrillStepForScore {
	rows, err := database.Query(`SELECT id,matched_node,correct,ontime,skipped,
		COALESCE(receiver,''),COALESCE(receiver_ok,1),COALESCE(step_time,''),COALESCE(created_at,''),
		COALESCE(flow_type,'correct'),COALESCE(score,0),COALESCE(role_id,''),COALESCE(user,''),COALESCE(forms,''),
		COALESCE(measure,''),COALESCE(result,''),COALESCE(found,''),COALESCE(evidence,'')
		FROM drill_steps WHERE project_id=? ORDER BY step_time,id`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []DrillStepForScore
	for rows.Next() {
		var s DrillStepForScore
		if err := rows.Scan(&s.ID, &s.MatchedNode, &s.Correct, &s.Ontime, &s.Skipped,
			&s.Receiver, &s.ReceiverOK, &s.StepTime, &s.CreatedAt,
			&s.FlowType, &s.Score, &s.RoleID, &s.User, &s.Forms,
			&s.Measure, &s.Result, &s.Found, &s.Evidence); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out
}

func loadTasksForScore(database *db.DB, projectID string) []map[string]any {
	rows, err := database.Query(`SELECT id,node_id,status,COALESCE(completed_at,''),COALESCE(accepted_at,''),
		COALESCE(title,''),COALESCE(descr,''),COALESCE(role_id,'') FROM tasks WHERE project_id=?`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, nodeID, status, completedAt, acceptedAt, title, descr, roleID string
		if err := rows.Scan(&id, &nodeID, &status, &completedAt, &acceptedAt, &title, &descr, &roleID); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id":          id,
			"node_id":     nodeID,
			"status":      status,
			"completed_at": completedAt,
			"accepted_at": acceptedAt,
			"title":       title,
			"descr":       descr,
			"role_id":     roleID,
		})
	}
	return out
}

func loadProjectForScore(database *db.DB, projectID string) *ProjectForScore {
	var p ProjectForScore
	err := database.QueryRow(`SELECT id,scenario_id,COALESCE(mode,'基础版'),COALESCE(status,''),COALESCE(started_at,''),COALESCE(date,'') FROM projects WHERE id=?`, projectID).
		Scan(&p.ID, &p.ScenarioID, &p.Mode, &p.Status, &p.StartedAt, &p.Date)
	if err != nil {
		return nil
	}
	return &p
}

// ========== 混淆节点下发时机判定（复刻 drill.js:86 isDistractorReleased） ==========

func isDistractorReleased(p *ProjectForScore, node ScriptNodeForScore, byID map[string]*ScriptNodeForScore, done map[string]bool) bool {
	if node.FlowType != "distractor" {
		return true
	}
	anchorID := strings.TrimSpace(node.AnchorID)
	if anchorID == "" {
		return true
	}
	anchor, ok := byID[anchorID]
	if !ok {
		return true
	}
	timing := strings.TrimSpace(node.ReleaseTiming)
	if timing == "" {
		timing = "before"
	}
	if timing == "after" {
		return done[anchorID]
	}
	// before：锚点 deps 全部完成即可见
	for _, depID := range anchor.Deps {
		if dep, ok := byID[depID]; ok {
			if !done[dep.ID] {
				return false
			}
		}
	}
	return true
}

// ========== 是否超时（复刻 drill.js:40 isLate） ==========

func isLate(p *ProjectForScore, node ScriptNodeForScore, stepCreatedAt string, database *db.DB) bool {
	if node.Timeout <= 0 {
		return false
	}
	nodeStart := nodeStartMs(p, node, database)
	atMs := parseLocalTime(stepCreatedAt)
	if atMs <= 0 {
		atMs = time.Now().UnixMilli()
	}
	if nodeStart <= 0 {
		nodeStart = atMs
	}
	return float64(atMs-nodeStart)/60000 > float64(node.Timeout)
}

func nodeStartMs(p *ProjectForScore, node ScriptNodeForScore, database *db.DB) int64 {
	var best int64
	for _, depID := range node.Deps {
		var stepCreatedAt sql.NullString
		_ = database.QueryRow(`SELECT created_at FROM drill_steps WHERE project_id=? AND matched_node=? ORDER BY id LIMIT 1`, p.ID, depID).
			Scan(&stepCreatedAt)
		if stepCreatedAt.Valid {
			ms := parseLocalTime(stepCreatedAt.String)
			if ms > 0 && (best == 0 || ms > best) {
				best = ms
			}
		}
	}
	if best != 0 {
		return best
	}
	return parseLocalTime(p.StartedAt)
}

// ========== receiver 判定 ==========

func parseReceivers(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune("、,，;；", r)
	})
	var out []string
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len([]rune(p)) > 40 {
			p = string([]rune(p)[:40])
		}
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func sameReceivers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ========== grade ==========

func gradeOf(v float64) string {
	switch {
	case v >= 90:
		return "优秀"
	case v >= 80:
		return "良好"
	case v >= 70:
		return "合格"
	default:
		return "不合格"
	}
}

// ========== planText ==========

func planText(n ScriptNodeForScore) string {
	if n.Timeout > 0 {
		return fmt.Sprintf("处置时长 %d 分钟", n.Timeout)
	}
	return "未设置处置时长"
}

// ========== 表单取值格式化 ==========
func formValueString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, formValueString(x))
		}
		return strings.Join(parts, "、")
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", t), "0"), ".")
	case bool:
		if t {
			return "是"
		}
		return "否"
	default:
		return ""
	}
}

// ========== Score 纯函数（复刻 drill.js:162 score） ==========

// Score 是单一纯函数：从 MySQL 读取项目 + 剧本 + tasks + drill_steps，计算扣分制评分结果。
// 被 monitor/dashboard/finish/decide/recalc 共用，唯一数据源。
func Score(database *db.DB, projectID string) *ScoreResult {
	p := loadProjectForScore(database, projectID)
	if p == nil {
		return nil
	}
	mode := p.Mode
	ended := p.Status == "已结束" || p.Status == "已归档"

	all := loadNodesForScore(database, p.ScenarioID)
	// 过滤掉 end 节点
	var nonEnd []ScriptNodeForScore
	for _, n := range all {
		if n.Type != "end" {
			nonEnd = append(nonEnd, n)
		}
	}
	var correct []ScriptNodeForScore
	var distractors []ScriptNodeForScore
	byID := make(map[string]*ScriptNodeForScore)
	for i := range nonEnd {
		n := nonEnd[i]
		byID[n.ID] = &n
		if n.FlowType == "distractor" {
			distractors = append(distractors, n)
		} else {
			correct = append(correct, n)
		}
	}

	// doneSet：drill_steps 中已出现的 matched_node
	doneSet := make(map[string]bool)
	steps := loadStepsForScore(database, projectID)
	for _, s := range steps {
		if s.MatchedNode != "" {
			doneSet[s.MatchedNode] = true
		}
	}

	// 混淆节点按时机过滤
	var releasedDistractors []ScriptNodeForScore
	for _, n := range distractors {
		if isDistractorReleased(p, n, byID, doneSet) {
			releasedDistractors = append(releasedDistractors, n)
		}
	}

	tasks := loadTasksForScore(database, projectID)

	// correct ID 集合（用于 matchNode）
	correctIDs := make(map[string]bool)
	for _, n := range correct {
		correctIDs[n.ID] = true
	}

	// taskByNode
	taskByNode := make(map[string]map[string]any)
	for _, t := range tasks {
		nodeID, _ := t["node_id"].(string)
		if nodeID == "" {
			continue
		}
		if correctIDs[nodeID] {
			taskByNode[nodeID] = t
		}
	}

	// ========== hitMap ==========
	hitMap := make(map[string]map[string]any)

	if mode == "基础版" {
		startMs := parseLocalTime(p.StartedAt)
		baseStart := startMs
		if baseStart == 0 {
			baseStart = parseLocalTime(p.Date)
		}
		for _, t := range tasks {
			status, _ := t["status"].(string)
			if status != "已完成" {
				continue
			}
			nodeID, _ := t["node_id"].(string)
			if nodeID == "" || !correctIDs[nodeID] {
				continue
			}
			if _, dup := hitMap[nodeID]; dup {
				continue
			}
			completedAt, _ := t["completed_at"].(string)
			doneMs := parseLocalTime(completedAt)
			nodeStart := nodeStartMsBasic(p, byID[nodeID], taskByNode, baseStart, database)
			var duration *float64
			if doneMs > 0 && nodeStart > 0 && doneMs >= nodeStart {
				d := math.Round(float64(doneMs-nodeStart) / 60000)
				duration = &d
			}
			late := false
			if duration != nil && byID[nodeID].Timeout > 0 {
				late = *duration > float64(byID[nodeID].Timeout)
			}
			hitMap[nodeID] = map[string]any{
				"stepTime": completedAt,
				"late":     late,
				"duration": duration,
			}
		}
	} else {
		// 实战版
		for _, s := range steps {
			if s.MatchedNode == "" || s.Correct == 0 || s.Skipped == 1 {
				continue
			}
			if !correctIDs[s.MatchedNode] {
				continue
			}
			if _, dup := hitMap[s.MatchedNode]; dup {
				continue
			}
			hitMap[s.MatchedNode] = map[string]any{
				"stepTime": s.StepTime,
				"late":     s.Ontime == 0,
				"step":     s,
			}
		}
	}

	// ========== wrongPicked ==========
	wrongPicked := make(map[string]DrillStepForScore)
	if mode != "基础版" {
		for _, s := range steps {
			if s.MatchedNode == "" || s.Correct != 0 || s.Skipped == 1 {
				continue
			}
			for _, rd := range releasedDistractors {
				if rd.ID == s.MatchedNode {
					wrongPicked[s.MatchedNode] = s
					break
				}
			}
		}
	}

	// ========== skipMap ==========
	skipMap := make(map[string]DrillStepForScore)
	if mode != "基础版" {
		for _, s := range steps {
			if s.MatchedNode == "" || s.Skipped != 1 {
				continue
			}
			skipMap[s.MatchedNode] = s
		}
	}

	// ========== 扣分计算 ==========
	const deductBase = 100.0
	earned := deductBase
	nodeResults := []NodeResult{}
	deductions := []Deduction{}
	skippedCorrect := 0
	skippedDistract := 0
	keywordMissCnt := 0
	evidenceMissCnt := 0
	textQualityCnt := 0

	// 正确流程节点
	for _, n := range correct {
		sc := n.Score
		hit, hasHit := hitMap[n.ID]
		_, hasSkip := skipMap[n.ID]

		if !hasHit && hasSkip {
			skippedCorrect++
			missed := ended
			var deduct float64
			if missed {
				deduct = float64(sc)
				earned -= deduct
				deductions = append(deductions, Deduction{
					NodeID: n.ID, Title: n.Title,
					Type: "正确流程跳过（演练结束扣全分）", Point: -deduct,
				})
			}
			actual := "已跳过（未执行）"
			if missed {
				actual = "已跳过（演练结束，按未完成计）"
			}
			state := "已跳过"
			if missed {
				state = "遗漏"
			}
			nodeResults = append(nodeResults, NodeResult{
				ID: n.ID, Title: n.Title, Role: n.Target,
				Plan: planText(n), Actual: actual,
				Required: true, Submitted: false, Hit: false, Ontime: false,
				Wrong: false, Skipped: true, Score: sc, Earned: -deduct,
				Late: false, State: state,
			})
			continue
		}

		if hasHit {
			var recvBad bool
			lateHit, _ := hit["late"].(bool)
			q := submissionQuality{Late: lateHit, Keywords: n.Keywords, EvidenceReq: n.EvidenceReq, Rules: parseQualityRules(n.QualityRules)}
			if step, ok := hit["step"].(DrillStepForScore); ok {
				recvBad = step.ReceiverOK == 0
				q.HasSubmission = true
				q.RecvBad = recvBad
				q.EvidenceText = step.Evidence
				q.Measure = step.Measure
				q.Result = step.Result
				q.Found = step.Found
				// 关键词/证据只匹配处置文本，不再拼入表单填写值
				q.Text = strings.Join([]string{step.Measure, step.Result, step.Found, step.Evidence}, "\n")
			}
			// 超时 20% / 接收人错误 30% / 关键词命中不足 30% / 证据未提交或未命中 20%，单节点合计上限 80%
			deduct, parts := correctNodeDeduction(sc, q)
			for _, p := range parts {
				deductions = append(deductions, Deduction{
					NodeID: n.ID, Title: n.Title,
					Type: p.Type, Point: -p.Point,
				})
				if strings.HasPrefix(p.Type, "处置关键词") {
					keywordMissCnt++
				}
				if strings.HasPrefix(p.Type, "处置文本质量") {
					textQualityCnt++
				}
				if strings.HasPrefix(p.Type, "证据材料未提交") {
					evidenceMissCnt++
				}
			}
			earned -= deduct
			stepTime, _ := hit["stepTime"].(string)
			var receiverStr string
			var receiverOk bool
			if step, ok := hit["step"].(DrillStepForScore); ok {
				receiverStr = step.Receiver
				receiverOk = step.ReceiverOK != 0
			}
			state := "已完成"
			if recvBad {
				if lateHit {
					state = "超时且接收人错误"
				} else {
					state = "接收人错误"
				}
			} else if lateHit {
				state = "超时完成"
			}
			nodeResults = append(nodeResults, NodeResult{
				ID: n.ID, Title: n.Title, Role: n.Target,
				Plan: planText(n), Actual: stepTime,
				Required: true, Submitted: true, Hit: true, Ontime: !lateHit,
				Wrong: false, Score: sc, Earned: -deduct, Late: lateHit,
				Receiver: receiverStr, ReceiverOk: receiverOk, State: state,
			})
			continue
		}

		// 未 hit 未 skip
		missed := ended
		var deduct float64
		if missed {
			deduct = float64(sc)
			earned -= deduct
			deductions = append(deductions, Deduction{
				NodeID: n.ID, Title: n.Title,
				Type: "正确流程未完成（扣全分）", Point: -deduct,
			})
		}
		actual := "待处置"
		if missed {
			actual = "未完成（演练结束）"
		}
		state := "待处置"
		if missed {
			state = "遗漏"
		}
		nodeResults = append(nodeResults, NodeResult{
			ID: n.ID, Title: n.Title, Role: n.Target,
			Plan: planText(n), Actual: actual,
			Required: true, Submitted: false, Hit: false, Ontime: false,
			Wrong: false, Score: sc, Earned: -deduct, Late: false, State: state,
		})
	}

	// 混淆流程被执行
	for nid, s := range wrongPicked {
		var n *ScriptNodeForScore
		for i := range distractors {
			if distractors[i].ID == nid {
				n = &distractors[i]
				break
			}
		}
		if n == nil {
			continue
		}
		deduct := float64(n.Score)
		earned -= deduct
		deductions = append(deductions, Deduction{
			NodeID: n.ID, Title: n.Title,
			Type: "执行混淆流程（扣全分）", Point: -deduct,
		})
		nodeResults = append(nodeResults, NodeResult{
			ID: n.ID, Title: n.Title, Role: n.Target,
			Plan: planText(*n), Actual: "已执行（" + s.StepTime + "）",
			Required: false, Submitted: true, Hit: false, Ontime: false,
			Wrong: true, Distract: true, Score: int(deduct), Earned: -deduct,
			Late: false, State: "决策偏离",
		})
	}

	// 混淆流程被跳过
	for nid := range skipMap {
		var n *ScriptNodeForScore
		for i := range distractors {
			if distractors[i].ID == nid {
				n = &distractors[i]
				break
			}
		}
		if n == nil {
			continue
		}
		released := false
		for _, rd := range releasedDistractors {
			if rd.ID == nid {
				released = true
				break
			}
		}
		if !released {
			continue
		}
		skippedDistract++
		nodeResults = append(nodeResults, NodeResult{
			ID: n.ID, Title: n.Title, Role: n.Target,
			Plan: planText(*n), Actual: "已跳过（未执行）",
			Required: false, Submitted: false, Hit: false, Ontime: false,
			Wrong: false, Skipped: true, Score: 0, Earned: 0,
			Late: false, State: "已跳过",
		})
	}

	// 实战版演练中强制查看脚本：首次记录即扣 10 分（幂等，靠审计扣分标记判断）
	forceView := false
	if mode == "实战版" {
		var forceCnt int
		_ = database.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE ?`,
			"查看演练脚本|项目="+projectID+"|类型=强制|扣分=是|%").Scan(&forceCnt)
		if forceCnt > 0 {
			earned -= 10
			forceView = true
			deductions = append(deductions, Deduction{
				NodeID: "__script__", Title: "实战版演练中强制查看演练脚本",
				Type: "演练中强制查看脚本（扣 10 分）", Point: -10,
			})
		}
	}

	// ========== 汇总 ==========
	total := len(correct)
	hitCnt := 0
	ontimeCnt := 0
	wrongCnt := 0
	lateCnt := 0
	recvBadCnt := 0
	missedCnt := 0
	pendingCnt := 0
	for _, nr := range nodeResults {
		if nr.Required && nr.Hit {
			hitCnt++
		}
		if nr.Required && nr.Hit && nr.Ontime {
			ontimeCnt++
		}
		if nr.Distract {
			wrongCnt++
		}
		if nr.Required && nr.Hit && !nr.Ontime {
			lateCnt++
		}
		if nr.Required && !nr.Submitted && ended {
			missedCnt++
		}
		if nr.Required && !nr.Submitted && !nr.Skipped && !ended {
			pendingCnt++
		}
	}

	maxScore := 0
	for _, n := range correct {
		maxScore += n.Score
	}

	sc := math.Max(0, math.Round(earned*10)/10)
	completeness := 0.0
	if total > 0 {
		completeness = math.Round(float64(hitCnt)/float64(total)*1000) / 10
	}
	timeRate := 0.0
	if total > 0 {
		timeRate = math.Round(float64(ontimeCnt)/float64(total)*1000) / 10
	}

	// 生成 formula
	for _, d := range deductions {
		if strings.HasPrefix(d.Type, "接收人选择错误") {
			recvBadCnt++
		}
	}
	formula := fmt.Sprintf("扣分制：起始 %d 分", int(deductBase))
	if hitCnt > 0 {
		formula += fmt.Sprintf("，已完成正确流程 %d/%d 项", hitCnt, total)
	}
	if lateCnt > 0 {
		formula += fmt.Sprintf("，超时 %d 项（每项扣本节点 20%%）", lateCnt)
	}
	if recvBadCnt > 0 {
		formula += fmt.Sprintf("，接收人错误 %d 项（每项扣本节点 30%%）", recvBadCnt)
	}
	if wrongCnt > 0 {
		formula += fmt.Sprintf("，执行混淆流程 %d 项", wrongCnt)
	}
	if keywordMissCnt > 0 {
		formula += fmt.Sprintf("，处置关键词必含项缺失 %d 项（按缺失比例扣本节点分值）", keywordMissCnt)
	}
	if textQualityCnt > 0 {
		formula += fmt.Sprintf("，处置文本质量不达标 %d 项（每项扣本节点 20%%）", textQualityCnt)
	}
	if evidenceMissCnt > 0 {
		formula += fmt.Sprintf("，证据材料未提交或未命中 %d 项（每项扣本节点 20%%）", evidenceMissCnt)
	}
	if forceView {
		formula += "，演练中强制查看脚本 1 次（扣 10 分）"
	}
	if missedCnt > 0 {
		formula += fmt.Sprintf("，未完成正确流程 %d 项", missedCnt)
	}
	totalDeduct := deductBase - earned
	if totalDeduct > 0 {
		formula += fmt.Sprintf("，合计扣 %.1f 分", totalDeduct)
	} else {
		formula += "，无扣分"
	}
	formula += fmt.Sprintf(" = %.1f 分", sc)

	stepsAny := []any{}
	for _, s := range steps {
		stepsAny = append(stepsAny, s)
	}

	return &ScoreResult{
		ProjectID:      projectID,
		Mode:           mode,
		ScenarioID:     p.ScenarioID,
		Ended:          ended,
		Total:          total,
		Hit:            hitCnt,
		Ontime:         ontimeCnt,
		Missed:         missedCnt,
		Pending:        pendingCnt,
		Wrong:          wrongCnt,
		Late:           lateCnt,
		MaxScore:       maxScore,
		Distractors:    len(releasedDistractors),
		Skipped:        skippedCorrect + skippedDistract,
		SkippedCorrect: skippedCorrect,
		SkippedDistract: skippedDistract,
		Completeness:   completeness,
		TimeRate:       timeRate,
		Score:          sc,
		Grade:          gradeOf(sc),
		Deductions:     deductions,
		Bonuses:        []any{},
		Formula:        formula,
		Nodes:          nodeResults,
		Steps:          stepsAny,
		CorrectTotal:   maxScore,
	}
}

// nodeStartMsBasic 基础版：节点起算时刻（复刻 drill.js basicStartMs）
func nodeStartMsBasic(p *ProjectForScore, node *ScriptNodeForScore, taskByNode map[string]map[string]any, baseStart int64, database *db.DB) int64 {
	if node == nil {
		return 0
	}
	var best int64
	for _, depID := range node.Deps {
		t, ok := taskByNode[depID]
		if !ok {
			continue
		}
		status, _ := t["status"].(string)
		completedAt, _ := t["completed_at"].(string)
		if status != "已完成" || completedAt == "" {
			continue
		}
		ms := parseLocalTime(completedAt)
		if ms > 0 && (best == 0 || ms > best) {
			best = ms
		}
	}
	if best != 0 {
		return best
	}
	return baseStart
}
