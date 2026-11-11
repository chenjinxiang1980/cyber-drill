// Package handler —— /api/drill v1 旧路径全套实战版 handler
//
// 语义 1:1 对齐 src/routes/drill.js：skip / decide / finish / steps /
// deleteStep / score-recalc / predecessor。前端 api.js 仍在调用这些旧路径
// （api.js 行 159 / 491-499），Go 后端必须提供完整 DB 写入和校验链。

package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ========================================================================
// 辅助函数（复刻 drill.js 全局 helper）
// ========================================================================

type drillV1Node struct {
	ID            string
	Type          string
	Title         string
	Target        string
	Timeout       int
	Form          string
	FlowType      string // "correct" / "distractor"
	Score         int
	Receiver      string
	ReceiverCC    string
	Deps          []string
	AnchorID      string
	ReleaseTiming string
	Action        string
	Desc          string
	Keywords      []string
	EvidenceReq   string
	QualityRules  string
	Required      bool
}

type drillV1Project struct {
	ID         string
	ScenarioID string
	Mode       string
	Status     string
	StartedAt  string
	Date       string
	Name       string
	PausedSec  int
	PausedAt   string
}

// drillV1ProjectOf 等价 projectOf(pid)
func drillV1ProjectOf(database *db.DB, pid string) *drillV1Project {
	var p drillV1Project
	err := database.QueryRow(
		`SELECT id,COALESCE(scenario_id,''),COALESCE(mode,'基础版'),COALESCE(status,''),COALESCE(started_at,''),COALESCE(date,''),COALESCE(name,''),
			COALESCE(paused_sec,0),COALESCE(paused_at,'') FROM projects WHERE id=?`, pid,
	).Scan(&p.ID, &p.ScenarioID, &p.Mode, &p.Status, &p.StartedAt, &p.Date, &p.Name, &p.PausedSec, &p.PausedAt)
	if err != nil {
		return nil
	}
	return &p
}

// drillV1NodesOf 等价 nodesOf(scenarioId)，过滤 type=end
func drillV1NodesOf(database *db.DB, scenarioID string) []drillV1Node {
	rows, err := database.Query(
		`SELECT id,type,title,COALESCE(target,''),COALESCE(timeout,0),COALESCE(form,''),
			COALESCE(flow_type,'correct'),COALESCE(score,0),COALESCE(receiver,''),COALESCE(receiver_cc,''),COALESCE(deps,'[]'),
			COALESCE(anchor_id,''),COALESCE(release_timing,'before'),COALESCE(action,''),COALESCE(`+"`descr`"+`,''),
			COALESCE(keywords,''),COALESCE(evidence_req,''),COALESCE(quality_rules,''),COALESCE(required,1)
		 FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL ORDER BY seq`, scenarioID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []drillV1Node
	for rows.Next() {
		var n drillV1Node
		var depsStr, keywords string
		var required int
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Target, &n.Timeout, &n.Form,
			&n.FlowType, &n.Score, &n.Receiver, &n.ReceiverCC, &depsStr, &n.AnchorID, &n.ReleaseTiming,
			&n.Action, &n.Desc, &keywords, &n.EvidenceReq, &n.QualityRules, &required); err != nil {
			continue
		}
		for _, kw := range strings.Split(keywords, ",") {
			if kw = strings.TrimSpace(kw); kw != "" {
				n.Keywords = append(n.Keywords, kw)
			}
		}
		if strings.EqualFold(n.FlowType, "distractor") {
			n.FlowType = "distractor"
		} else {
			n.FlowType = "correct"
		}
		if strings.TrimSpace(depsStr) != "" {
			_ = json.Unmarshal([]byte(depsStr), &n.Deps)
		}
		n.Required = required == 1
		out = append(out, n)
	}
	return out
}

// projectForTiming / scriptNodeForTiming 把 v1 结构适配到评分引擎的计时结构（P0-10）。
func projectForTiming(p *drillV1Project) *ProjectForScore {
	return &ProjectForScore{
		ID: p.ID, ScenarioID: p.ScenarioID, Mode: p.Mode, Status: p.Status,
		StartedAt: p.StartedAt, Date: p.Date, PausedSec: p.PausedSec, PausedAt: p.PausedAt,
	}
}

func scriptNodeForTiming(n *drillV1Node) ScriptNodeForScore {
	return ScriptNodeForScore{ID: n.ID, Timeout: n.Timeout, Deps: n.Deps}
}

// drillV1NodeLockResult 等价 nodeLock(node, byId, doneSet)
type drillV1NodeLockResult struct {
	Locked    bool    `json:"locked"`
	BlockedBy []gin.H `json:"blockedBy"`
}

func drillV1NodeLock(node drillV1Node, byId map[string]*drillV1Node, done map[string]bool) drillV1NodeLockResult {
	res := drillV1NodeLockResult{BlockedBy: []gin.H{}}
	if node.FlowType == "distractor" {
		if strings.EqualFold(node.ReleaseTiming, "after") {
			anchorID := strings.TrimSpace(node.AnchorID)
			anchor := byId[anchorID]
			if anchor != nil && !done[anchorID] {
				res.Locked = true
				res.BlockedBy = append(res.BlockedBy, gin.H{"id": anchor.ID, "title": anchor.Title, "reason": "锚点主流节点尚未完成（after 下发时机）"})
			}
		}
		return res
	}
	for _, depID := range node.Deps {
		if dep, ok := byId[depID]; ok && !done[depID] {
			res.Locked = true
			res.BlockedBy = append(res.BlockedBy, gin.H{"id": depID, "title": dep.Title})
		}
	}
	return res
}

// canHandleRole 角色 roleID 是否能处理 target 角色名的节点
// 复刻 Node drill_plan.js canHandle：指挥部/平台管理员全量，
// 信息科负责人仅统筹 itlead/duty/netsec/sysadmin，其余角色仅本角色。
func canHandleRole(database *db.DB, roleID, target string) bool {
	if roleID == "commander" || roleID == "platformadmin" {
		return true
	}
	mapped := targetToRoleID(database, target)
	if roleID == "itlead" {
		switch mapped {
		case "itlead", "duty", "netsec", "sysadmin":
			return true
		}
		return false
	}
	return mapped == roleID
}

// evSummaryResult 等价 evidenceSummary(project) 的返回值（含 core / support 分级）
type evSummaryResult struct {
	Total          int             `json:"total"`
	Done           int             `json:"done"`
	Pending        int             `json:"pending"`
	Core           int             `json:"core"`
	CoreDone       int             `json:"coreDone"`
	CorePending    int             `json:"corePending"`
	CoreRate       float64         `json:"coreRate"`
	Support        int             `json:"support"`
	SupportDone    int             `json:"supportDone"`
	SupportPending int             `json:"supportPending"`
	PendingItems   []evSummaryItem `json:"pendingItems"`
}

type evSummaryItem struct {
	Key               string `json:"key"`
	NodeID            string `json:"nodeId"`
	NodeTitle         string `json:"nodeTitle"`
	Target            string `json:"target"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	Core              bool   `json:"core"`
	SuggestedDeadline string `json:"suggestedDeadline"`
}

// loadEvidenceSummary 复刻 drill.js evidenceSummary(project)，并按项目 core 清单标注分级。
func loadEvidenceSummary(database *db.DB, p *drillV1Project) evSummaryResult {
	archived := make(map[string]bool)
	rows, err := database.Query(`SELECT COALESCE(req_key,'') FROM evidence WHERE project_id=? AND req_key IS NOT NULL AND req_key!=''`, p.ID)
	if err == nil && rows != nil {
		for rows.Next() {
			var k string
			rows.Scan(&k)
			if strings.TrimSpace(k) != "" {
				archived[k] = true
			}
		}
		rows.Close()
	}

	coreSet := make(map[string]bool)
	for _, it := range loadCoreEvidence(database, p.ID, p.ScenarioID) {
		coreSet[it.Key] = true
	}
	hours := settingIntValue(database, "drill_supplement_hours", 72)
	if hours <= 0 {
		hours = 72
	}
	suggestedDeadline := time.Now().Add(time.Duration(hours) * time.Hour).Format("2006-01-02 15:04")

	var summary evSummaryResult
	nodeRows, err := database.Query(
		`SELECT id,title,COALESCE(target,''),COALESCE(evidence_req,'[]'),COALESCE(flow_type,'correct')
		 FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND type!='end' ORDER BY seq`, p.ScenarioID)
	if err != nil || nodeRows == nil {
		return summary
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		var nid, ntitle, ntarget, nft, evReqStr string
		nodeRows.Scan(&nid, &ntitle, &ntarget, &evReqStr, &nft)
		if strings.EqualFold(nft, "distractor") {
			continue
		}
		var evList []map[string]any
		_ = json.Unmarshal([]byte(evReqStr), &evList)
		for i, it := range evList {
			summary.Total++
			key := fmt.Sprintf("%s#%d", nid, i)
			isCore := coreSet[key]
			if isCore {
				summary.Core++
			} else {
				summary.Support++
			}
			if archived[key] {
				if isCore {
					summary.CoreDone++
				} else {
					summary.SupportDone++
				}
				continue
			}
			summary.Pending++
			var itype, iname string
			if v, ok := it["type"].(string); ok {
				itype = v
			}
			if v, ok := it["name"].(string); ok {
				iname = v
			}
			summary.PendingItems = append(summary.PendingItems, evSummaryItem{
				Key: key, NodeID: nid, NodeTitle: ntitle, Target: ntarget, Type: itype, Name: iname,
				Core: isCore, SuggestedDeadline: suggestedDeadline,
			})
		}
	}
	summary.Done = summary.Total - summary.Pending
	summary.CorePending = summary.Core - summary.CoreDone
	summary.SupportPending = summary.Support - summary.SupportDone
	summary.CoreRate = pctRate(summary.CoreDone, summary.Core)
	return summary
}

// ========================================================================
// POST /api/drill/skip — 实战版跳过处置流程（Node drill.js:573）
// ========================================================================

// PostDrillSkip 10 级校验链 + INSERT drill_steps + INSERT timeline
func PostDrillSkip(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不参与处置记录"})
			return
		}
		body := readJSON(c)
		pid := strOr(body["projectId"], "")
		nodeID := strOr(body["nodeId"], "")
		reason := strings.TrimSpace(strOr(body["reason"], ""))
		if len([]rune(reason)) > 500 {
			reason = string([]rune(reason)[:500])
		}

		// 项目存在
		p := drillV1ProjectOf(database, pid)
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		// 实战版
		if strings.TrimSpace(p.Mode) != "实战版" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前项目为基础版，请在角色工作台按正确流程逐步执行"})
			return
		}
		if p.Status != "演练中" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + p.Status + "，暂不能跳过流程"})
			return
		}

		// 节点存在
		nodes := drillV1NodesOf(database, p.ScenarioID)
		var node *drillV1Node
		byId := make(map[string]*drillV1Node)
		for i := range nodes {
			if nodes[i].Type != "end" {
				byId[nodes[i].ID] = &nodes[i]
			}
		}
		node = byId[nodeID]
		if node == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "处置流程不存在"})
			return
		}

		// canHandle（未参演角色允许代填人代填跳过）
		proxyForRoleID := ""
		if canProxyForRole(u.Role.ID) {
			if members, ok := projectMemberRoles(database, p.ID); ok {
				tr := targetToRoleID(database, node.Target)
				if tr != proxyHandlerRoleID && !members[tr] {
					proxyForRoleID = tr
				}
			}
		}
		if !canHandleRole(database, u.Role.ID, node.Target) && proxyForRoleID == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "该流程责任角色为「" + node.Target + "」，当前角色无权处置"})
			return
		}

		// 前置依赖锁
		done := loadDoneSet(database, p.ID)
		lock := drillV1NodeLock(*node, byId, done)
		if lock.Locked {
			titles := make([]string, 0)
			for _, b := range lock.BlockedBy {
				titles = append(titles, strOr(b["title"], ""))
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "前置流程未完成：" + strings.Join(titles, "、"), "blockedBy": lock.BlockedBy})
			return
		}

		// 重复跳过
		var dupUser string
		var dupID int64
		err := database.QueryRow(`SELECT id,user FROM drill_steps WHERE project_id=? AND matched_node=? AND skipped=1`, p.ID, node.ID).Scan(&dupID, &dupUser)
		if err == nil && dupID > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "该流程已由 " + dupUser + " 跳过，不可重复跳过"})
			return
		}
		// 也检查是否已有非跳过记录（已被执行）
		err = database.QueryRow(`SELECT id FROM drill_steps WHERE project_id=? AND matched_node=?`, p.ID, node.ID).Scan(&dupID)
		if err == nil && dupID > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "该流程已有处置记录，不可重复跳过"})
			return
		}

		// 混淆节点强制 reason
		isCorrect := node.FlowType != "distractor"
		if !isCorrect && reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "跳过混淆节点必须填写原因（请说明为何判定为干扰项）", "needReason": true})
			return
		}

		// 写库
		stepTime := hhmm()
		now := nowSec()
		nowS := timeNowSec()
		delta := 0
		var res sql.Result
		res, err = database.Exec(
			`INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,choice,correct,found_at,measure,result,forms,flow_type,score,receiver,receiver_ok,skipped,skip_reason,created_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE id=id`,
			p.ID, p.ScenarioID, u.Role.ID, u.Name, node.Action, node.Title,
			func() string {
				if proxyForRoleID != "" {
					return "信息科负责人代填未参演角色「" + node.Target + "」，判定该流程不执行（跳过）"
				}
				return "参演人员判定该流程不执行（跳过）"
			}(), stepTime, node.ID, 1, 0, node.Title,
			func() int {
				if isCorrect {
					return 1
				}
				return 0
			}(),
			now, "参演人员判定不执行，跳过该流程", "已跳过（未执行）", "",
			node.FlowType, delta, "", 1, 1, reason, nowS,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入处置记录失败：" + err.Error()})
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "该流程已有处置记录，不可重复跳过", "nodeId": node.ID})
			return
		}

		skipAction, skipDetail := "跳过处置流程", "流程「"+node.Title+"」由参演人员判定不执行（跳过）"
		if proxyForRoleID != "" {
			skipAction = "代填处置：跳过流程"
			skipDetail = "流程「" + node.Title + "」由" + u.Name + "代填未参演角色「" + node.Target + "」判定不执行（跳过）"
		}
		if _, err := database.Exec(
			`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			p.ID, stepTime, u.Name, u.Role.ID, skipAction,
			skipDetail, "", nil,
		); err != nil {
			logSideEffectErr("timeline", p.ID, "跳过处置流程", err)
		}

		sc := Score(database, p.ID)
		// 站内消息：节点跳过（Node drill.js:618-628，level P1/P2）
		var scScore float64
		if sc != nil {
			scScore = sc.Score
		}
		reasonText, lvl := "混淆流程跳过，无扣分无奖励", "P2"
		if isCorrect {
			reasonText, lvl = "正确流程跳过，结束演练时扣全部分值", "P1"
		}
		pushNotif(database, "node_skip", map[string]any{
			"title": node.Title, "project": p.Name, "user": u.Name,
			"reason": reasonText, "score": scScore,
		}, p.ID, lvl, "", "internal")
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "nodeId": node.ID, "skipped": true,
			"stepTime": stepTime, "delta": delta, "score": sc,
		})
	}
}

func loadDoneSet(database *db.DB, projectID string) map[string]bool {
	out := make(map[string]bool)
	rows, _ := database.Query(`SELECT matched_node FROM drill_steps WHERE project_id=? AND matched_node IS NOT NULL AND matched_node!=''`, projectID)
	defer rows.Close()
	for rows.Next() {
		var n string
		rows.Scan(&n)
		if n != "" {
			out[n] = true
		}
	}
	return out
}

// ========================================================================
// POST /api/drill/decide — 实战版提交处置（Node drill.js:633）
// ========================================================================

// PostDrillDecide 完整实现 — 有完整 DB 写入（24 列）
func PostDrillDecide(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不参与处置记录"})
			return
		}
		body := readJSON(c)
		pid := strOr(body["projectId"], "")
		nodeID := strOr(body["nodeId"], "")
		measure := strings.TrimSpace(strOr(body["measure"], ""))
		result := strings.TrimSpace(strOr(body["result"], ""))
		found := strings.TrimSpace(strOr(body["found"], ""))
		evidence := strings.TrimSpace(strOr(body["evidence"], ""))
		receiverRaw := body["receiver"]

		p := drillV1ProjectOf(database, pid)
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if strings.TrimSpace(p.Mode) != "实战版" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前项目为基础版，请在角色工作台按正确流程逐步执行"})
			return
		}
		if p.Status != "演练中" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + p.Status + "，暂不能提交处置记录"})
			return
		}

		nodes := drillV1NodesOf(database, p.ScenarioID)
		byId := make(map[string]*drillV1Node)
		for i := range nodes {
			if nodes[i].Type != "end" {
				byId[nodes[i].ID] = &nodes[i]
			}
		}
		node := byId[nodeID]
		if node == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "处置流程不存在"})
			return
		}

		// 代填：目标角色未参演时，信息科负责人/平台管理员可代填处置。
		proxyForRoleID := ""
		if canProxyForRole(u.Role.ID) {
			if members, ok := projectMemberRoles(database, p.ID); ok {
				tr := targetToRoleID(database, node.Target)
				if tr != proxyHandlerRoleID && !members[tr] {
					proxyForRoleID = tr
				}
			}
		}
		if !canHandleRole(database, u.Role.ID, node.Target) && proxyForRoleID == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "该流程责任角色为「" + node.Target + "」，当前角色无权处置"})
			return
		}

		done := loadDoneSet(database, p.ID)
		lock := drillV1NodeLock(*node, byId, done)
		if lock.Locked {
			titles := make([]string, 0)
			for _, b := range lock.BlockedBy {
				titles = append(titles, strOr(b["title"], ""))
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "前置流程未完成：" + strings.Join(titles, "、"), "blockedBy": lock.BlockedBy})
			return
		}

		// 混淆流程必须在其锚点释放后才可提交，避免绕过实战版的时机设计提前执行。
		if node.FlowType == "distractor" && !drillV1DistractorReleased(*node, byId, done) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该混淆流程尚未到释放时机（需先完成锚点节点）"})
			return
		}

		var dupID int64
		err := database.QueryRow(`SELECT id FROM drill_steps WHERE project_id=? AND matched_node=?`, p.ID, node.ID).Scan(&dupID)
		if err == nil && dupID > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "该流程已有处置记录，不可重复执行"})
			return
		}

		// 表单必填服务端强校验：按该节点绑定模板的必填项校验，缺失或空串一律 400
		formsJSON := "{}"
		if formsRaw, ok := body["forms"]; ok {
			b, _ := json.Marshal(formsRaw)
			formsJSON = string(b)
		}
		reqFields := loadFormRequiredFields(database, node.Form)
		if missing := missingRequiredFields(formsJSON, reqFields); len(missing) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "表单必填项缺失：" + joinFieldDisplays(missing)})
			return
		}
		// F19 报案必要性评估单：跨字段一致性硬校验
		if msg := validateReportAssessment(node.Form, util.ParseJSONObj(formsJSON)); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		// 提交硬门槛：只拦「不写就说明没做这件事」（未填措施 / 有效字数过低 / 敷衍短文本）。
		// 节点配置的最少字数与必备要素转为扣分项（correctNodeDeduction 内的 evaluateTextQuality），
		// 正常作答不再因细节词被 400 挡住。
		rules := parseQualityRules(node.QualityRules)
		if reasons := evaluateHardGate(submissionQuality{Measure: measure, Result: result, Found: found}, rules, node.FlowType); len(reasons) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "处置文本无效：" + strings.Join(reasons, "；")})
			return
		}

		isCorrect := node.FlowType != "distractor"
		// receiver
		wantRecv := parseReceivers(node.Receiver)
		var recv []string
		switch v := receiverRaw.(type) {
		case string:
			recv = parseReceivers(v)
		case []any:
			for _, item := range v {
				recv = append(recv, strOr(item, ""))
			}
			sort.Strings(recv)
		}
		if isCorrect && len(wantRecv) > 0 && len(recv) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择下一节点接收人（可多选）"})
			return
		}
		receiver := strings.Join(recv, "、")
		recvRatio := receiverDeductRatio(wantRecv, parseReceivers(node.ReceiverCC), recv, node.Target)
		receiverOk := 1
		if recvRatio > 0 {
			receiverOk = 0
		}
		recvNote := ""
		if recvRatio >= ReceiverErrPenaltyRatio {
			recvNote = "接收人选择错误（主送未命中）"
		} else if recvRatio > 0 {
			recvNote = "接收人抄送缺失"
		}

		stepTime := hhmm()
		now := nowSec()
		nowS := timeNowSec()
		// P0-10：按前驱完成/任务下发 + 超时，扣除暂停时长；缺时间戳回退旧口径
		ontime := !isLateV1(p, node)
		if late, ok := nodeLateAdvanced(projectForTiming(p), scriptNodeForTiming(node), nowS, nil, database); ok {
			ontime = !late
		}

		// delta（与 Score() 同一口径：超时 20% / 接收人 30%（抄送缺失 10%）/ 关键词分级扣分 / 文字质量 20% / 证据按归档率扣分）
		var delta float64
		var parts []scoreDeductPart
		if !isCorrect {
			delta = -float64(node.Score)
		} else {
			reqKeys := evidenceReqKeys(node.ID, node.EvidenceReq)
			archivedKeys := loadArchivedEvidenceKeys(database, p.ID)
			archivedCnt := 0
			for _, k := range reqKeys {
				if archivedKeys[k] {
					archivedCnt++
				}
			}
			q := submissionQuality{
				Late: !ontime, RecvBad: receiverOk == 0, RecvRatio: recvRatio, RecvNote: recvNote, HasSubmission: true,
				// 关键词/证据只匹配处置文本，表单填写值不参与（避免塞关键词刷分）
				Text:         strings.Join([]string{measure, result, found, evidence}, "\n"),
				Measure:      measure,
				Result:       result,
				Found:        found,
				EvidenceText: evidence, Keywords: node.Keywords, EvidenceReq: node.EvidenceReq,
				Rules:            rules,
				EvidenceReqCount: len(reqKeys), EvidenceArchived: archivedCnt,
			}
			deduct, ps := correctNodeDeduction(node.Score, q)
			parts = ps
			delta = -deduct
		}

		// 截断 descr
		descr := "措施：" + measure + "；结果：" + result
		if found != "" {
			descr += "；发现：" + found
		}
		if proxyForRoleID != "" {
			descr += "；代填未参演角色：" + node.Target
		}
		if len([]rune(descr)) > 2000 {
			descr = string([]rune(descr)[:2000])
		}

		res, err := database.Exec(
			`INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,choice,correct,found_at,found,measure,result,evidence,forms,flow_type,score,receiver,receiver_ok,created_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE id=id`,
			p.ID, p.ScenarioID, u.Role.ID, u.Name, node.Action, node.Title, descr, stepTime,
			node.ID, 1, ontime, node.Title, func() int {
				if isCorrect {
					return 1
				}
				return 0
			}(),
			now, found, measure, result, evidence, formsJSON, node.FlowType, delta, receiver, receiverOk, nowS,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入处置记录失败：" + err.Error()})
			return
		}
		// uk_ds_proj_node 唯一索引兜底：并发/重复点击时第二次插入影响 0 行。
		if n, _ := res.RowsAffected(); n == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "该节点已提交，请刷新后查看", "nodeId": node.ID})
			return
		}
		id, _ := res.LastInsertId()

		// 业务影响联动：与基础版共用同一套规则（F1/F3 → 故障，F7 → 恢复档位）。
		// 处置记录已落库，联动失败不阻断本次提交，但会写入 timeline 便于追溯。
		if formMap, ok := body["forms"].(map[string]any); ok && node.Form != "" {
			formName, _ := drillV1FormFields(database, node.Form)
			changed := applyFormImpact(database, ImpactSource{
				ProjectID: p.ID, NodeID: node.ID, FormID: node.Form, FormName: formName,
				SubmissionID: id, Operator: u.Name, RoleID: u.Role.ID, At: util.Now(),
			}, node.Form, formMap)
			if len(changed) > 0 {
				logImpactTimeline(database, p.ID, "处置表单联动业务影响",
					node.Form+" 更新："+strings.Join(changed, "、"))
			}
		}
		if evidence != "" {
			evID := "E" + fmt.Sprintf("%d", time.Now().UnixNano())
			evName := "实战处置记录-" + node.ID + "-" + u.Name + "（" + evidence + "）"
			if len([]rune(evName)) > 180 {
				evName = string([]rune(evName)[:180])
			}
			// 处置记录已落库；证据归档失败只降级记录，避免重试触发 drill_steps 重复冲突（409）。
			if _, err := database.Exec(
				`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				evID, "处置记录", evName, "表单", stepTime, u.Role.ID, u.Name, u.ID, 0, nil, p.ID, nil, nil, nowS,
			); err != nil {
				logSideEffectErr("evidence", p.ID, "归档实战处置记录", err)
			}
		}

		// timeline
		level := ""
		if !isCorrect || receiverOk == 0 || !ontime {
			level = "P1"
		}
		detail := "流程「" + node.Title + "」"
		if proxyForRoleID != "" {
			detail += "；由" + u.Name + "代填未参演角色「" + node.Target + "」"
		}
		if !isCorrect {
			detail += "；执行混淆流程，扣分 " + fmt.Sprintf("%.1f", math.Abs(delta)) + " 分"
		} else {
			if len(parts) == 0 {
				detail += "；无扣分"
			}
			for _, p := range parts {
				detail += "；" + p.Type + "，扣 " + fmt.Sprintf("%.1f", p.Point) + " 分"
			}
		}
		timelineAction := func() string {
			a := "执行正确处置流程"
			if !isCorrect {
				a = "执行混淆处置流程"
			}
			if proxyForRoleID != "" {
				a = "代填处置：" + a
			}
			return a
		}()
		if _, err := database.Exec(
			`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			p.ID, stepTime, u.Name, u.Role.ID,
			timelineAction,
			detail, level, nil,
		); err != nil {
			logSideEffectErr("timeline", p.ID, "处置流程时间线", err)
		}

		sc := Score(database, p.ID)
		// 站内消息：节点处置结果（Node drill.js:719-730，level P2/P1）
		var scScore float64
		if sc != nil {
			scScore = sc.Score
		}
		deductAbs := math.Abs(delta)
		deductText := "无扣分"
		if delta < 0 {
			deductText = "扣分 " + fmt.Sprint(deductAbs) + " 分"
		}
		correctText := "混淆流程，扣分 " + fmt.Sprint(deductAbs) + " 分"
		if isCorrect {
			correctText = "正确流程，" + deductText
		}
		if isCorrect {
			for _, p := range parts {
				correctText += "；" + p.Type
			}
		}
		correctText += "；当前累计得分 " + fmt.Sprint(scScore) + " 分"
		lvl := "P1"
		if isCorrect && receiverOk == 1 && delta == 0 {
			lvl = "P2"
		}
		pushNotif(database, "node_submit", map[string]any{
			"title": node.Title, "project": p.Name, "user": u.Name, "correct": correctText,
		}, p.ID, lvl, "", "internal")
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "id": id, "nodeId": node.ID, "stepTime": stepTime,
			"correct": func() int {
				if isCorrect {
					return 1
				}
				return 0
			}(),
			"ontime": ontime, "delta": delta,
			"receiver": receiver, "receiverOk": receiverOk,
			"score": sc, "isProxy": proxyForRoleID != "",
		})
	}
}

// ========================================================================
// POST /api/drill/finish — 结束演练（Node drill.js:741）
// ========================================================================

// PostDrillFinish 结束演练：核对清单 → 缺项说明 → 状态 → 评分快照 → timeline/审计。
// 请求体：
//
//	{ projectId, dryRun?: true }                      仅返回结束核对清单
//	{ projectId, exception?: { reason, missingKeys[], owner, deadline, approver } } 有缺项结束
func PostDrillFinish(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不可结束演练"})
			return
		}
		rid := u.Role.ID
		if rid != "commander" && rid != "platformadmin" && rid != "itlead" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅院应急指挥部、信息科负责人、平台管理员可结束演练"})
			return
		}
		body := readJSON(c)
		pid := strOr(body["projectId"], "")
		p := drillV1ProjectOf(database, pid)
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if p.Status != "演练中" {
			if p.Status == "已结束" || p.Status == "已归档" {
				seal := loadSeal(database, p.ID)
				if seal != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":  "演练" + p.Status + "，无需重复结束",
						"sealed": gin.H{"snapshotId": seal.SnapshotID, "version": seal.SnapshotVersion, "endMode": seal.EndMode},
					})
					return
				}
				c.JSON(http.StatusBadRequest, gin.H{
					"error":                 "演练" + p.Status + "，无需重复结束",
					"needsSupplementalSeal": true,
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + p.Status + "，暂不能结束"})
			return
		}

		code, payload := executeDrillFinish(database, u, p, body)
		c.JSON(code, payload)
	}
}

// nullableMatchedNode 空节点转 NULL：uk_ds_proj_node 允许多条 NULL，
// 避免「通用动作记录」因 matched_node 为空被误判为同一节点重复提交。
func nullableMatchedNode(id string) any {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return id
}

// ========================================================================
// POST /api/drill/steps — 旧版自主添加步骤（Node drill.js:781）
// ========================================================================

// PostDrillSteps 历史兼容：自主添加处置步骤
func PostDrillSteps(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || u.Role.External {
			c.JSON(http.StatusForbidden, gin.H{"error": "外部单位不参与处置步骤录入"})
			return
		}
		body := readJSON(c)
		pid := strOr(body["projectId"], "")
		p := drillV1ProjectOf(database, pid)
		if p == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if strings.TrimSpace(p.Mode) != "实战版" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前项目为基础版（诱导演练），不支持自主添加步骤"})
			return
		}
		title := strings.TrimSpace(strOr(body["title"], ""))
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写处置步骤标题"})
			return
		}
		action := strOr(body["action"], "其他")
		descr := strings.TrimSpace(strOr(body["descr"], strOr(body["desc"], "")))
		if len([]rune(descr)) > 2000 {
			descr = string([]rune(descr)[:2000])
		}
		stepTimeStr := strOr(body["stepTime"], "")
		var stepTime string
		matched := stepTimeRegex.MatchString(stepTimeStr)
		if matched {
			stepTime = stepTimeStr
		} else {
			stepTime = hhmm()
		}

		// matchNode（简化版：只匹配 required 主流节点）
		nodes := drillV1NodesOf(database, p.ScenarioID)
		var bestScore int
		var matchedNode *drillV1Node
		for i := range nodes {
			n := &nodes[i]
			if !n.Required || n.Type == "end" || n.FlowType == "distractor" {
				continue
			}
			s := 0
			if n.Action != "" && n.Action == action {
				s += 3
			}
			targetRole := n.Target
			if canHandleRole(database, u.Role.ID, targetRole) {
				s++
			}
			if s > bestScore {
				bestScore = s
				matchedNode = n
			}
		}

		matchedNodeID := ""
		ontime := false
		if matchedNode != nil {
			matchedNodeID = matchedNode.ID
			ontime = !isLateV1(p, matchedNode)
			// P0-10：同一套实战版计时口径
			if late, ok := nodeLateAdvanced(projectForTiming(p), scriptNodeForTiming(matchedNode), timeNowSec(), nil, database); ok {
				ontime = !late
			}
		}
		matchOK := bestScore >= 2

		res, err := database.Exec(
			`INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,created_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.ScenarioID, u.Role.ID, u.Name, action, title, descr, stepTime,
			nullableMatchedNode(matchedNodeID), func() int {
				if matchOK {
					return 1
				}
				return 0
			}(),
			func() int {
				if ontime {
					return 1
				}
				return 0
			}(),
			nowSec(),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入处置步骤失败"})
			return
		}
		id, _ := res.LastInsertId()

		c.JSON(http.StatusOK, gin.H{
			"ok":          true,
			"id":          id,
			"matchedNode": matchedNodeID,
			"matchedTitle": func() string {
				if matchedNode != nil {
					return matchedNode.Title
				}
				return ""
			}(),
			"ontime":   ontime,
			"stepTime": stepTime,
		})
	}
}

// ========================================================================
// DELETE /api/drill/steps/:id — 删除处置步骤（Node drill.js:805）
// ========================================================================

// DeleteDrillStep 加权限校验：仅本人或指挥部/平台管理员可删除
func DeleteDrillStep(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		id := c.Param("id")

		var stepRoleID string
		err := database.QueryRow(`SELECT COALESCE(role_id,'') FROM drill_steps WHERE id=?`, id).Scan(&stepRoleID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "步骤不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询步骤失败"})
			return
		}
		if stepRoleID != u.Role.ID && u.Role.ID != "platformadmin" && u.Role.ID != "commander" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅本人或指挥部/平台管理员可删除该步骤"})
			return
		}

		if _, err := database.Exec(`DELETE FROM drill_steps WHERE id=?`, id); err != nil {
			slog.Error("删除处置步骤失败", "step_id", id, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除处置步骤失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ========================================================================
// POST /api/drill/score/:projectId/recalc — 旧前端重算评分
// ========================================================================

// PostDrillScoreRecalcV1 调用 RecalcProjectScore 真实逻辑（角色校验 + Score() + timeline）
func PostDrillScoreRecalcV1(database *db.DB) gin.HandlerFunc {
	return RecalcProjectScore(database)
}

// ========================================================================
// GET /api/drill/predecessor — 查前置节点信息（Node 无真实实现，前端不依赖返回值）
// ========================================================================

// GetDrillPredecessor 返回节点的 deps 列表 + 已完成状态
func GetDrillPredecessor(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Query("nodeId")
		projectID := c.Query("projectId")

		var depsStr, ft string
		database.QueryRow(`SELECT COALESCE(deps,'[]'),COALESCE(flow_type,'correct') FROM script_nodes WHERE id=? AND deleted_at IS NULL`, nodeID).Scan(&depsStr, &ft)
		var deps []string
		_ = json.Unmarshal([]byte(depsStr), &deps)

		done := loadDoneSet(database, projectID)
		blockedBy := []gin.H{}
		for _, d := range deps {
			if !done[d] {
				var title string
				database.QueryRow(`SELECT COALESCE(title,'') FROM script_nodes WHERE id=? AND deleted_at IS NULL`, d).Scan(&title)
				blockedBy = append(blockedBy, gin.H{"id": d, "title": title})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"nodeId":    nodeID,
			"projectId": projectID,
			"deps":      deps,
			"blocked":   len(blockedBy) > 0,
			"blockedBy": blockedBy,
			"flowType":  ft,
			// 前置节点已提交内容（只读）：审议类节点据此展示「申请单 + 同意/不同意 + 意见」
			"predecessors": loadPredecessorDetails(database, projectID, deps),
		})
	}
}

// loadPredecessorDetails 读取直接前置节点的已提交内容。
// 基础版取 tasks + form_submissions，实战版取 drill_steps.forms；两者都按节点 id 精确定位。
func loadPredecessorDetails(database *db.DB, projectID string, deps []string) []gin.H {
	out := make([]gin.H, 0, len(deps))
	for _, dep := range deps {
		if strings.TrimSpace(dep) == "" {
			continue
		}
		out = append(out, loadPredecessorDetail(database, projectID, dep))
	}
	return out
}

func loadPredecessorDetail(database *db.DB, projectID, nodeID string) gin.H {
	var title, target, formID, nodeType string
	_ = database.QueryRow(
		`SELECT COALESCE(title,''),COALESCE(target,''),COALESCE(form,''),COALESCE(type,'') FROM script_nodes WHERE id=? AND deleted_at IS NULL`, nodeID,
	).Scan(&title, &target, &formID, &nodeType)
	out := gin.H{
		"nodeId": nodeID, "title": title, "target": target, "type": nodeType,
		"completed": false, "forms": []gin.H{},
	}

	// 基础版：任务 + 表单提交记录
	var taskID, taskStatus, roleID, completedAt string
	if err := database.QueryRow(
		`SELECT id,COALESCE(status,''),COALESCE(role_id,''),COALESCE(completed_at,'')
		 FROM tasks WHERE project_id=? AND node_id=? ORDER BY seq LIMIT 1`, projectID, nodeID,
	).Scan(&taskID, &taskStatus, &roleID, &completedAt); err == nil {
		out["taskId"] = taskID
		out["status"] = taskStatus
		out["role"] = roleID
		out["completedAt"] = completedAt
		out["completed"] = taskStatus == "已完成"
		forms := []gin.H{}
		if rs, err := database.Query(
			`SELECT form_id,COALESCE(form_name,''),COALESCE(data,'{}'),COALESCE(submitter,''),COALESCE(time,'')
			 FROM form_submissions WHERE task_id=? ORDER BY id`, taskID,
		); err == nil {
			for rs.Next() {
				var fid, fname, dataStr, submitter, at string
				if rs.Scan(&fid, &fname, &dataStr, &submitter, &at) != nil {
					continue
				}
				forms = append(forms, gin.H{
					"formId": fid, "formName": fname, "label": fname,
					"mode":      formModeOf(database, fid),
					"fields":    formFieldPairs(database, fid, util.ParseJSONObj(dataStr)),
					"submitter": submitter, "time": at,
				})
			}
			rs.Close()
		}
		out["forms"] = forms
	}

	// 实战版：处置步骤（同一节点只取最早一条）
	var stepUser, stepTime, measure, result, formsStr string
	if err := database.QueryRow(
		`SELECT COALESCE(user,''),COALESCE(step_time,''),COALESCE(measure,''),COALESCE(result,''),COALESCE(forms,'{}')
		 FROM drill_steps WHERE project_id=? AND matched_node=? ORDER BY id LIMIT 1`, projectID, nodeID,
	).Scan(&stepUser, &stepTime, &measure, &result, &formsStr); err == nil {
		fname, _ := drillV1FormFields(database, formID)
		out["completed"] = true
		out["user"] = stepUser
		out["stepTime"] = stepTime
		out["measure"] = measure
		out["result"] = result
		out["forms"] = []gin.H{{
			"formId": formID, "formName": fname, "label": fname,
			"mode":      formModeOf(database, formID),
			"fields":    formFieldPairs(database, formID, util.ParseJSONObj(formsStr)),
			"submitter": stepUser, "time": stepTime,
		}}
	}
	return out
}

// formFieldLegacyAliases 兼容历史提交：模板字段改名后，旧提交仍以旧 label 为 key。
var formFieldLegacyAliases = map[string][]string{
	"发现方式": {"发现渠道"},
	"影响数量": {"受影响主机数量"},
}

// formFieldPairs 把表单提交的 data 按模板字段顺序摊平为 [{label,value}]，
// 前端前置区直接按顺序只读渲染，避免 JSON 对象乱序或漏字段。
// 新字段在历史提交里没有 key 时返回空串；改名字段按别名回退取旧值。
func formFieldPairs(database *db.DB, formID string, data map[string]any) []gin.H {
	_, fields := drillV1FormFields(database, formID)
	out := make([]gin.H, 0, len(fields))
	for _, raw := range fields {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		label, _ := f["label"].(string)
		if strings.TrimSpace(label) == "" {
			continue
		}
		v := ""
		if raw, ok := data[label]; ok && raw != nil {
			v = toPlainString(raw)
		} else {
			for _, alias := range formFieldLegacyAliases[label] {
				if raw, ok := data[alias]; ok && raw != nil {
					v = toPlainString(raw)
					break
				}
			}
		}
		out = append(out, gin.H{"label": label, "value": v})
	}
	return out
}

// ========================================================================
// 通用工具
// ========================================================================

var stepTimeRegex = regexp.MustCompile(`^\d{2}:\d{2}$`)

func readJSON(c *gin.Context) map[string]any {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil || body == nil {
		body = map[string]any{}
	}
	return body
}

// truncRunes 按字符数截断（对齐 Node notify.js 的 slice(0,40/120/500)）
func truncRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// pushNotif 自动路径：渲染并落库，同一项目同一渲染结果只插一次。
func pushNotif(database *db.DB, event string, vars map[string]any, projectID, level, source, target string) bool {
	return pushNotifWith(database, event, vars, projectID, level, source, target, false)
}

// pushNotifForce 手动路径：跳过判重，每次点击都产生一条站内消息（与 Node 原版一致）。
func pushNotifForce(database *db.DB, event string, vars map[string]any, projectID, level, source, target string) bool {
	return pushNotifWith(database, event, vars, projectID, level, source, target, true)
}

// pushNotifWith 渲染并落库站内通知。列名列序与 Node notify.js:133-141 一致；
// level/source 传空串时取模板值，target 传空串时按 Node 默认 internal。
// force=false 时幂等键为 project_id + 渲染后 title + descr；force=true 不判重。
func pushNotifWith(database *db.DB, event string, vars map[string]any, projectID, level, source, target string, force bool) bool {
	title, descr, tSource, tLevel, ok := RenderNotifTemplate(database, event, vars)
	if !ok {
		return false
	}
	if level == "" {
		level = tLevel
	}
	if source == "" {
		source = tSource
	}
	if target == "" {
		target = "internal"
	}
	if !force {
		var dup int
		_ = database.QueryRow(`SELECT COUNT(*) FROM notifs WHERE project_id=? AND title=? AND descr=?`, projectID, title, descr).Scan(&dup)
		if dup > 0 {
			return false
		}
	}
	seq := 1
	if err := database.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM notifs`).Scan(&seq); err != nil {
		seq = 1
	}
	id := "N" + fmt.Sprint(time.Now().UnixMilli()) + "_" + itoa(100+rand.Intn(900))
	_, err := database.Exec(
		`INSERT INTO notifs (id,time,level,source,title,descr,seq,target,project_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, time.Now().Format("2006-01-02 15:04"), level, truncRunes(source, 40),
		truncRunes(title, 120), truncRunes(descr, 500), seq, target, projectID,
	)
	return err == nil
}

// timeNowSec 返回 Unix 秒（int64 但 drill_steps.created_at 用 varchar，转 string）
func timeNowSec() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// isLateV1 drill_v1 专用：复用 Score 的 parseLocalTime 但简化
func isLateV1(p *drillV1Project, node *drillV1Node) bool {
	if node == nil || node.Timeout <= 0 {
		return false
	}
	var stepCreatedAt sql.NullString
	err := sql.ErrNoRows
	// 简化：没有 step 记录视为未超时（由 Score 精确计算）
	// 实战版 skip/decide 的 ontime 由 Score 的 hitMap.Ontime 统一判定
	// 这里只做粗判：当前时间 - 项目 started_at > timeout * 60s
	_ = stepCreatedAt
	_ = err
	startMs := parseLocalTime(p.StartedAt)
	if startMs == 0 {
		startMs = parseLocalTime(p.Date)
	}
	if startMs == 0 {
		return false
	}
	elapsedMin := float64(time.Now().UnixMilli()-startMs) / 60000
	return elapsedMin > float64(node.Timeout)
}
