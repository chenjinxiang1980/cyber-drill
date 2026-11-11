package handler

// 演练收尾与评分封存：核心证据分级、结束核对清单、有缺项结束、评分快照。
//
// 口径来源：docs/system-review-business-2026-11.md §6.4。
// 设计约束：
//   - 项目状态仍写 "已结束"（状态字符串在 missing_handlers.go / 归档迁移中被白名单使用，
//     本轮不改该文件），"完整 / 有缺项" 由 project_end_seal.end_mode 区分；
//   - 结束时刻只调用一次 scoreInternal(forceEnded=true)，快照与结束通知使用同一份结果；
//   - 快照写入 score_snapshots，重算生成 version+1，不覆盖封存版本。

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ========================================================================
// 策略配置（settings 表，缺省值兜底）
// ========================================================================

func settingIntValue(database *db.DB, key string, def int) int {
	var raw sql.NullString
	if err := database.QueryRow(`SELECT value FROM settings WHERE `+"`key`"+`=?`, key).Scan(&raw); err != nil {
		return def
	}
	var v int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw.String), "%d", &v); err != nil {
		return def
	}
	return v
}

func settingTextValue(database *db.DB, key, def string) string {
	var raw sql.NullString
	if err := database.QueryRow(`SELECT value FROM settings WHERE `+"`key`"+`=?`, key).Scan(&raw); err != nil {
		return def
	}
	if strings.TrimSpace(raw.String) == "" {
		return def
	}
	return strings.TrimSpace(raw.String)
}

// ========================================================================
// 核心证据清单
// ========================================================================

type coreEvidenceItem struct {
	Key       string `json:"key"`
	NodeID    string `json:"nodeId"`
	NodeTitle string `json:"nodeTitle"`
	Target    string `json:"target"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Seq       int    `json:"seq"`
}

type rowQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// coreEvidencePriority 证据名称优先级：越接近告警/定级/启动/隔离/备份/上报/回执/取证/恢复/结束，
// 越优先进入 core 清单；其余排在最后，仅在数量不足时补足。
func coreEvidencePriority(name string) int {
	switch {
	case strings.Contains(name, "告警"):
		return 1
	case strings.Contains(name, "定级") || strings.Contains(name, "研判"):
		return 2
	case strings.Contains(name, "启动"):
		return 3
	case strings.Contains(name, "隔离") || strings.Contains(name, "封堵"):
		return 4
	case strings.Contains(name, "备份") || strings.Contains(name, "恢复"):
		return 5
	case strings.Contains(name, "双报") || strings.Contains(name, "上报") || strings.Contains(name, "报告"):
		return 6
	case strings.Contains(name, "回执"):
		return 7
	case strings.Contains(name, "取证") || strings.Contains(name, "保全") || strings.Contains(name, "镜像"):
		return 8
	case strings.Contains(name, "结束") || strings.Contains(name, "复盘"):
		return 9
	default:
		return 50
	}
}

// defaultCoreEvidenceFrom 按剧本基线推导平台内置默认 core 清单（每剧本 10–12 项）。
// 只取正确流程（flow_type=correct）节点的取证要求；evidence_req 中显式标记 "core":true
// 的取证项（法定上报记录、结束记录单、受理回执等）优先纳入且不受 12 项上限挤压，
// 其余按证据名称优先级、再按节点顺序稳定排序补足，保证多次计算结果一致。
func defaultCoreEvidenceFrom(q rowQuerier, database *db.DB, scenarioID string) []coreEvidenceItem {
	maxN := settingIntValue(database, "drill_core_evidence_max", 12)
	if maxN < 3 {
		maxN = 12
	}
	if maxN > 20 {
		maxN = 20
	}

	rows, err := q.Query(`SELECT id,title,COALESCE(target,''),COALESCE(evidence_req,'[]'),COALESCE(flow_type,'correct'),seq
		FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND type!='end' ORDER BY seq`, scenarioID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type candidate struct {
		item     coreEvidenceItem
		priority int
		order    int
	}
	var cands []candidate
	order := 0
	explicitCore := 0
	for rows.Next() {
		var nid, title, target, evReq, flowType string
		var seq int
		if err := rows.Scan(&nid, &title, &target, &evReq, &flowType, &seq); err != nil {
			continue
		}
		if strings.EqualFold(flowType, "distractor") {
			continue
		}
		var evList []map[string]any
		if json.Unmarshal([]byte(evReq), &evList) != nil {
			continue
		}
		for i, it := range evList {
			name, _ := it["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			typ, _ := it["type"].(string)
			priority := coreEvidencePriority(name)
			if isCore, _ := it["core"].(bool); isCore {
				// 剧本显式声明的必备取证项：无条件纳入，不再按名称猜测。
				priority = 0
				explicitCore++
			}
			order++
			cands = append(cands, candidate{
				item: coreEvidenceItem{
					Key:       fmt.Sprintf("%s#%d", nid, i),
					NodeID:    nid,
					NodeTitle: title,
					Target:    target,
					Type:      typ,
					Name:      name,
					Seq:       seq,
				},
				priority: priority,
				order:    order,
			})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].priority != cands[j].priority {
			return cands[i].priority < cands[j].priority
		}
		return cands[i].order < cands[j].order
	})
	keep := maxN
	if explicitCore > keep {
		keep = explicitCore
	}
	if len(cands) > keep {
		cands = cands[:keep]
	}
	out := make([]coreEvidenceItem, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.item)
	}
	return out
}

// loadCoreEvidence 优先读取项目级清单；老项目没有项目级记录时回落到平台内置默认。
func loadCoreEvidence(database *db.DB, projectID, scenarioID string) []coreEvidenceItem {
	rows, err := database.Query(`SELECT ev_key,COALESCE(node_id,''),COALESCE(node_title,''),COALESCE(target,''),
		COALESCE(ev_name,''),COALESCE(seq,0)
		FROM project_core_evidence WHERE project_id=? ORDER BY seq,node_id,ev_key`, projectID)
	if err == nil {
		var out []coreEvidenceItem
		for rows.Next() {
			var it coreEvidenceItem
			if rows.Scan(&it.Key, &it.NodeID, &it.NodeTitle, &it.Target, &it.Name, &it.Seq) == nil {
				out = append(out, it)
			}
		}
		rows.Close()
		if len(out) > 0 {
			return out
		}
	}
	return defaultCoreEvidenceFrom(database, database, scenarioID)
}

// seedProjectCoreEvidenceTx 项目创建时把平台默认 core 清单复制到项目级，后续可在项目内调整。
func seedProjectCoreEvidenceTx(tx *sql.Tx, database *db.DB, projectID, scenarioID string) error {
	if strings.TrimSpace(scenarioID) == "" {
		return nil
	}
	items := defaultCoreEvidenceFrom(tx, database, scenarioID)
	for _, it := range items {
		idx := 0
		if _, err := fmt.Sscanf(it.Key[strings.LastIndex(it.Key, "#")+1:], "%d", &idx); err != nil {
			idx = 0
		}
		if _, err := tx.Exec(`INSERT IGNORE INTO project_core_evidence
			(project_id,ev_key,scenario_id,node_id,ev_index,ev_name,node_title,target,seq,source)
			VALUES (?,?,?,?,?,?,?,?,?, 'default')`,
			projectID, it.Key, scenarioID, it.NodeID, idx, it.Name, it.NodeTitle, it.Target, it.Seq); err != nil {
			return err
		}
	}
	return nil
}

// ========================================================================
// 结束核对清单
// ========================================================================

type endChecklistEvidence struct {
	Key               string `json:"key"`
	NodeID            string `json:"nodeId"`
	NodeTitle         string `json:"nodeTitle"`
	Target            string `json:"target"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	Core              bool   `json:"core"`
	SuggestedDeadline string `json:"suggestedDeadline"`
}

type endChecklistNode struct {
	NodeID string `json:"nodeId"`
	Title  string `json:"title"`
	Target string `json:"target"`
}

type endChecklist struct {
	ProjectID         string                 `json:"projectId"`
	Mode              string                 `json:"mode"`
	GeneratedAt       string                 `json:"generatedAt"`
	RequiredTotal     int                    `json:"requiredTotal"`
	RequiredDone      int                    `json:"requiredDone"`
	RequiredMissing   []endChecklistNode     `json:"requiredMissing"`
	EvidenceTotal     int                    `json:"evidenceTotal"`
	EvidenceDone      int                    `json:"evidenceDone"`
	EvidenceRate      float64                `json:"evidenceRate"`
	CoreTotal         int                    `json:"coreTotal"`
	CoreDone          int                    `json:"coreDone"`
	CoreRate          float64                `json:"coreRate"`
	CorePending       int                    `json:"corePending"`
	CoreMissing       []endChecklistEvidence `json:"coreMissing"`
	SupportTotal      int                    `json:"supportTotal"`
	SupportDone       int                    `json:"supportDone"`
	SupportPending    int                    `json:"supportPending"`
	PendingItems      []endChecklistEvidence `json:"pendingItems"`
	ZeroEvidence      bool                   `json:"zeroEvidence"`
	CanEndComplete    bool                   `json:"canEndComplete"`
	NeedsException    bool                   `json:"needsException"`
	ExceptionFields   []string               `json:"exceptionFields"`
	SuggestedDeadline string                 `json:"suggestedDeadline"`
	Hint              string                 `json:"hint"`
}

func pctRate(done, total int) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(done)/float64(total)*1000) / 10
}

// loadDoneForEnd 结束核对用的完成集合：实战版读 drill_steps，基础版读 tasks（两者取并集）。
func loadDoneForEnd(database *db.DB, p *drillV1Project) map[string]bool {
	done := loadDoneSet(database, p.ID)
	rows, err := database.Query(`SELECT node_id FROM tasks WHERE project_id=? AND status='已完成' AND node_id IS NOT NULL AND node_id!=''`, p.ID)
	if err == nil {
		for rows.Next() {
			var nid string
			if rows.Scan(&nid) == nil && nid != "" {
				done[nid] = true
			}
		}
		rows.Close()
	}
	return done
}

func loadArchivedEvidenceKeys(database *db.DB, projectID string) map[string]bool {
	out := make(map[string]bool)
	rows, err := database.Query(`SELECT COALESCE(req_key,'') FROM evidence WHERE project_id=? AND req_key IS NOT NULL AND req_key!=''`, projectID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil && strings.TrimSpace(k) != "" {
			out[k] = true
		}
	}
	return out
}

// buildEndChecklist 生成一次性核对清单：必做节点、证据总数、core/support 分组与缺项明细。
func buildEndChecklist(database *db.DB, p *drillV1Project) endChecklist {
	ck := endChecklist{
		ProjectID:       p.ID,
		Mode:            p.Mode,
		GeneratedAt:     db.Now(),
		RequiredMissing: []endChecklistNode{},
		CoreMissing:     []endChecklistEvidence{},
		PendingItems:    []endChecklistEvidence{},
		ExceptionFields: []string{"reason", "owner", "deadline", "approver"},
	}
	hours := settingIntValue(database, "drill_supplement_hours", 72)
	if hours <= 0 {
		hours = 72
	}
	ck.SuggestedDeadline = time.Now().Add(time.Duration(hours) * time.Hour).Format("2006-01-02 15:04")

	done := loadDoneForEnd(database, p)
	archived := loadArchivedEvidenceKeys(database, p.ID)
	coreItems := loadCoreEvidence(database, p.ID, p.ScenarioID)
	coreSet := make(map[string]coreEvidenceItem, len(coreItems))
	for _, it := range coreItems {
		coreSet[it.Key] = it
	}

	nodes := drillV1NodesOf(database, p.ScenarioID)
	for _, n := range nodes {
		if n.Type == "end" || strings.EqualFold(n.FlowType, "distractor") {
			continue
		}
		if n.Required {
			ck.RequiredTotal++
			if done[n.ID] {
				ck.RequiredDone++
			} else {
				ck.RequiredMissing = append(ck.RequiredMissing, endChecklistNode{NodeID: n.ID, Title: n.Title, Target: n.Target})
			}
		}
		var evList []map[string]any
		if json.Unmarshal([]byte(n.EvidenceReq), &evList) != nil {
			continue
		}
		for i, it := range evList {
			name, _ := it["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			typ, _ := it["type"].(string)
			key := fmt.Sprintf("%s#%d", n.ID, i)
			ck.EvidenceTotal++
			isDone := archived[key]
			if isDone {
				ck.EvidenceDone++
			}
			_, isCore := coreSet[key]
			if isCore {
				ck.CoreTotal++
				if isDone {
					ck.CoreDone++
				} else {
					item := endChecklistEvidence{
						Key: key, NodeID: n.ID, NodeTitle: n.Title, Target: n.Target,
						Type: typ, Name: name, Core: true, SuggestedDeadline: ck.SuggestedDeadline,
					}
					ck.CoreMissing = append(ck.CoreMissing, item)
					ck.PendingItems = append(ck.PendingItems, item)
				}
			} else {
				ck.SupportTotal++
				if isDone {
					ck.SupportDone++
				} else {
					ck.PendingItems = append(ck.PendingItems, endChecklistEvidence{
						Key: key, NodeID: n.ID, NodeTitle: n.Title, Target: n.Target,
						Type: typ, Name: name, Core: false, SuggestedDeadline: ck.SuggestedDeadline,
					})
				}
			}
		}
	}

	ck.CorePending = ck.CoreTotal - ck.CoreDone
	ck.SupportPending = ck.SupportTotal - ck.SupportDone
	ck.EvidenceRate = pctRate(ck.EvidenceDone, ck.EvidenceTotal)
	ck.CoreRate = pctRate(ck.CoreDone, ck.CoreTotal)
	ck.ZeroEvidence = ck.EvidenceTotal > 0 && ck.EvidenceDone == 0
	requiredPending := ck.RequiredTotal - ck.RequiredDone
	// 必做节点未完成同样属于缺项：不得直接标记「完整结束」。
	ck.NeedsException = requiredPending > 0 || ck.CorePending > 0 || ck.ZeroEvidence
	ck.CanEndComplete = !ck.NeedsException

	switch {
	case requiredPending > 0:
		ck.Hint = fmt.Sprintf("必做节点 %d/%d 未完成（缺 %d 项），不得直接结束；请补齐后结束，或填写缺项说明并降级封存",
			ck.RequiredDone, ck.RequiredTotal, requiredPending)
	case ck.ZeroEvidence:
		ck.Hint = fmt.Sprintf("本项目尚未归档任何取证材料（0/%d），不得直接结束；请填写缺项说明并降级评级", ck.EvidenceTotal)
	case ck.CorePending > 0:
		ck.Hint = fmt.Sprintf("核心证据 %d/%d 未齐（缺 %d 项），可补齐后结束，或填写缺项说明申请结束",
			ck.CoreDone, ck.CoreTotal, ck.CorePending)
	case ck.SupportPending > 0:
		ck.Hint = fmt.Sprintf("核心证据已齐（%d/%d），仍有 %d 项非核心取证未归档：仅扣分、不阻断结束",
			ck.CoreDone, ck.CoreTotal, ck.SupportPending)
	default:
		ck.Hint = "必做节点与取证材料已齐，可直接结束演练"
	}
	return ck
}

// ========================================================================
// 有缺项结束说明
// ========================================================================

type drillEndException struct {
	Reason      string   `json:"reason"`
	MissingKeys []string `json:"missingKeys"`
	Owner       string   `json:"owner"`
	Deadline    string   `json:"deadline"`
	Approver    string   `json:"approver"`
}

func parseEndException(raw any) *drillEndException {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	ex := &drillEndException{
		Reason:   strings.TrimSpace(strOr(m["reason"], "")),
		Owner:    strings.TrimSpace(strOr(m["owner"], "")),
		Deadline: strings.TrimSpace(strOr(m["deadline"], "")),
		Approver: strings.TrimSpace(strOr(m["approver"], "")),
	}
	switch v := m["missingKeys"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				ex.MissingKeys = append(ex.MissingKeys, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				ex.MissingKeys = append(ex.MissingKeys, strings.TrimSpace(s))
			}
		}
	}
	return ex
}

// normalizeEndException 校验并补齐缺项 key；返回错误文案为空表示通过。
func normalizeEndException(ex *drillEndException, ck endChecklist) string {
	if ex == nil {
		return "缺少缺项结束说明"
	}
	if n := len([]rune(ex.Reason)); n < 5 {
		return "缺项原因至少 5 个字"
	} else if n > 500 {
		ex.Reason = string([]rune(ex.Reason)[:500])
	}
	if ex.Owner == "" {
		return "请填写补录责任人"
	}
	ex.Owner = string([]rune(ex.Owner)[:minInt(len([]rune(ex.Owner)), 64)])
	if ex.Approver == "" {
		return "请填写批准人"
	}
	ex.Approver = string([]rune(ex.Approver)[:minInt(len([]rune(ex.Approver)), 64)])
	if ex.Deadline == "" {
		return "请填写补录时限"
	}
	normalized, ok := normalizeDeadline(ex.Deadline)
	if !ok {
		return "补录时限格式应为 YYYY-MM-DD HH:mm"
	}
	if parseLocalTime(normalized) <= time.Now().UnixMilli() {
		return "补录时限必须晚于当前时间"
	}
	ex.Deadline = normalized

	// 缺项 key 覆盖「未完成的必做节点 + 未归档的核心证据」；未填写时按核对清单自动补齐。
	provided := make(map[string]bool)
	for _, k := range ex.MissingKeys {
		provided[k] = true
	}
	merged := make([]string, 0, len(ck.RequiredMissing)+len(ck.CoreMissing))
	for _, it := range ck.RequiredMissing {
		if !provided[it.NodeID] {
			provided[it.NodeID] = true
			merged = append(merged, it.NodeID)
		}
	}
	for _, it := range ck.CoreMissing {
		if !provided[it.Key] {
			provided[it.Key] = true
			merged = append(merged, it.Key)
		}
	}
	ex.MissingKeys = append(ex.MissingKeys, merged...)
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeDeadline(s string) (string, bool) {
	s = strings.TrimSpace(s)
	layouts := []struct {
		in  string
		out string
	}{
		{"2006-01-02 15:04:05", "2006-01-02 15:04"},
		{"2006-01-02 15:04", "2006-01-02 15:04"},
		{"2006/01/02 15:04", "2006-01-02 15:04"},
		{"2006-01-02T15:04:05", "2006-01-02 15:04"},
		{"2006-01-02T15:04", "2006-01-02 15:04"},
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l.in, s, time.Local); err == nil {
			return t.Format(l.out), true
		}
	}
	return "", false
}

// ========================================================================
// 评分快照
// ========================================================================

type scoreSnapshot struct {
	ID                   string  `json:"id"`
	ProjectID            string  `json:"projectId"`
	Version              int     `json:"version"`
	SnapshotStatus       string  `json:"snapshotStatus"`
	EndMode              string  `json:"endMode"`
	Score                float64 `json:"score"`
	Grade                string  `json:"grade"`
	MaxScore             int     `json:"maxScore"`
	Hit                  int     `json:"hit"`
	Total                int     `json:"total"`
	Completeness         float64 `json:"completeness"`
	TimeRate             float64 `json:"timeRate"`
	EvidenceCompleteRate float64 `json:"evidenceCompleteRate"`
	CoreCompleteRate     float64 `json:"coreCompleteRate"`
	CoreTotal            int     `json:"coreTotal"`
	CoreDone             int     `json:"coreDone"`
	RuleVersion          string  `json:"ruleVersion"`
	ScenarioVersion      string  `json:"scenarioVersion"`
	RecalcFrom           int     `json:"recalcFrom,omitempty"`
	RecalcReason         string  `json:"recalcReason,omitempty"`
	SealedAt             string  `json:"sealedAt"`
	SealedBy             string  `json:"sealedBy"`
	Snapshot             any     `json:"snapshot,omitempty"`
}

type snapshotPayload struct {
	SnapshotID      string             `json:"snapshotId"`
	Version         int                `json:"version"`
	SnapshotStatus  string             `json:"snapshotStatus"`
	EndMode         string             `json:"endMode"`
	Score           *ScoreResult       `json:"score"`
	Checklist       endChecklist       `json:"checklist"`
	Exception       *drillEndException `json:"exception,omitempty"`
	RuleVersion     string             `json:"ruleVersion"`
	ScenarioVersion string             `json:"scenarioVersion"`
	SealedAt        string             `json:"sealedAt"`
	SealedBy        string             `json:"sealedBy"`
	RecalcFrom      int                `json:"recalcFrom,omitempty"`
	RecalcReason    string             `json:"recalcReason,omitempty"`
}

// sealedGrade 评级联动：core 完整率 <100% 不得优秀，<80% 不得良好；按结束方式追加标识。
// coreTotal=0（剧本未配置取证要求）时不做封顶。
func sealedGrade(base string, coreRate float64, coreTotal int, endMode string) string {
	g := base
	if coreTotal > 0 {
		if coreRate < 80 {
			if g == "优秀" || g == "良好" {
				g = "合格"
			}
		} else if coreRate < 100 {
			if g == "优秀" {
				g = "良好"
			}
		}
	}
	switch endMode {
	case "有缺项":
		g += "（有缺项）"
	case "异常结束":
		g += "（异常结束）"
	}
	return g
}

// computeRuleVersion 规则版本 = SHA-256(剧本节点全文 + 评分比例常量 + core key 清单 + 盐)。
func computeRuleVersion(database *db.DB, scenarioID string, coreKeys []string) string {
	var b strings.Builder
	b.WriteString("score-rule-v1|")
	b.WriteString(fmt.Sprintf("ratios:%.2f,%.2f,%.2f,%.2f,%.2f,%.2f|",
		TimeoutPenaltyRatio, ReceiverErrPenaltyRatio, KeywordHitPenaltyRatio,
		EvidencePenaltyRatio, TextQualityPenaltyRatio, NodeMaxPenaltyRatio))
	b.WriteString("salt:" + settingTextValue(database, "drill_rule_version_salt", "2026-11") + "|")
	sort.Strings(coreKeys)
	b.WriteString("core:" + strings.Join(coreKeys, ",") + "|")
	rows, err := database.Query(`SELECT id,COALESCE(title,''),COALESCE(type,''),COALESCE(flow_type,''),
		COALESCE(score,0),COALESCE(timeout,0),COALESCE(receiver,''),COALESCE(deps,'[]'),
		COALESCE(evidence_req,''),COALESCE(quality_rules,''),COALESCE(required,1)
		FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL ORDER BY seq`, scenarioID)
	if err == nil {
		for rows.Next() {
			var id, title, typ, flow, receiver, deps, evReq, quality string
			var score, timeout, required int
			if rows.Scan(&id, &title, &typ, &flow, &score, &timeout, &receiver, &deps, &evReq, &quality, &required) == nil {
				b.WriteString(fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s|%s|%s|%s|%d;",
					id, title, typ, flow, score, timeout, receiver, deps, evReq, quality, required))
			}
		}
		rows.Close()
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func scenarioVersionOf(database *db.DB, scenarioID string) string {
	var v string
	if err := database.QueryRow(`SELECT COALESCE(version,'') FROM scenarios WHERE id=?`, scenarioID).Scan(&v); err != nil {
		return ""
	}
	return v
}

func nextSnapshotVersion(q rowQuerier, projectID string) int {
	var v int
	if err := q.QueryRow(`SELECT COALESCE(MAX(version),0) FROM score_snapshots WHERE project_id=?`, projectID).Scan(&v); err != nil {
		return 1
	}
	return v + 1
}

func newSnapshotID() string {
	return "SS" + time.Now().Format("20060102150405") + fmt.Sprintf("%03d", randInt(1000))
}

// insertScoreSnapshot 写入一条评分快照；调用方负责事务。
func insertScoreSnapshot(tx *sql.Tx, database *db.DB, p *drillV1Project, sc *ScoreResult,
	ck endChecklist, ex *drillEndException, actor, endMode, snapshotStatus string,
	recalcFrom int, recalcReason string) (*scoreSnapshot, error) {

	version := nextSnapshotVersion(tx, p.ID)
	coreKeys := make([]string, 0, ck.CoreTotal)
	for _, it := range loadCoreEvidence(database, p.ID, p.ScenarioID) {
		coreKeys = append(coreKeys, it.Key)
	}
	ruleVersion := computeRuleVersion(database, p.ScenarioID, coreKeys)
	scenarioVersion := scenarioVersionOf(database, p.ScenarioID)

	grade := sealedGrade(sc.Grade, ck.CoreRate, ck.CoreTotal, endMode)
	sealedAt := db.Now()

	scoreForSnapshot := *sc
	scoreForSnapshot.Steps = nil
	// P0-04：payload 内等级与 score_snapshots.grade 使用同一个封顶值，
	// 避免大屏读 payload、报告读列造成“良好 / 合格（有缺项）”不一致。
	scoreForSnapshot.Grade = grade

	payload := snapshotPayload{
		SnapshotID:      newSnapshotID(),
		Version:         version,
		SnapshotStatus:  snapshotStatus,
		EndMode:         endMode,
		Score:           &scoreForSnapshot,
		Checklist:       ck,
		Exception:       ex,
		RuleVersion:     ruleVersion,
		ScenarioVersion: scenarioVersion,
		SealedAt:        sealedAt,
		SealedBy:        actor,
		RecalcFrom:      recalcFrom,
		RecalcReason:    recalcReason,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`INSERT INTO score_snapshots
		(id,project_id,version,snapshot_status,end_mode,score,grade,max_score,hit,total,
		 completeness,time_rate,evidence_complete_rate,core_complete_rate,core_total,core_done,
		 rule_version,scenario_version,payload,recalc_from,recalc_reason,sealed_at,sealed_by,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		payload.SnapshotID, p.ID, version, snapshotStatus, endMode, sc.Score, grade, sc.MaxScore,
		sc.Hit, sc.Total, sc.Completeness, sc.TimeRate, ck.EvidenceRate, ck.CoreRate,
		ck.CoreTotal, ck.CoreDone, ruleVersion, scenarioVersion, string(payloadJSON),
		sqlNullInt(recalcFrom), sqlNullString(recalcReason), sealedAt, actor, sealedAt)
	if err != nil {
		return nil, err
	}

	return &scoreSnapshot{
		ID: payload.SnapshotID, ProjectID: p.ID, Version: version, SnapshotStatus: snapshotStatus,
		EndMode: endMode, Score: sc.Score, Grade: grade, MaxScore: sc.MaxScore,
		Hit: sc.Hit, Total: sc.Total, Completeness: sc.Completeness, TimeRate: sc.TimeRate,
		EvidenceCompleteRate: ck.EvidenceRate, CoreCompleteRate: ck.CoreRate,
		CoreTotal: ck.CoreTotal, CoreDone: ck.CoreDone, RuleVersion: ruleVersion,
		ScenarioVersion: scenarioVersion, RecalcFrom: recalcFrom, RecalcReason: recalcReason,
		SealedAt: sealedAt, SealedBy: actor,
	}, nil
}

func sqlNullString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func sqlNullInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	return int(time.Now().UnixNano()/1e3) % n
}

// ========================================================================
// 读取封存快照（报告 / 复盘 / 工作台）
// ========================================================================

type sealSummary struct {
	ProjectID       string `json:"projectId"`
	EndMode         string `json:"endMode"`
	SnapshotID      string `json:"snapshotId"`
	SnapshotVersion int    `json:"snapshotVersion"`
	SealedAt        string `json:"sealedAt"`
	SealedBy        string `json:"sealedBy"`
	ExceptionID     int64  `json:"exceptionId,omitempty"`
}

func loadSeal(database *db.DB, projectID string) *sealSummary {
	var s sealSummary
	var exID sql.NullInt64
	err := database.QueryRow(`SELECT project_id,COALESCE(end_mode,'完整'),COALESCE(snapshot_id,''),
		COALESCE(snapshot_version,0),COALESCE(sealed_at,''),COALESCE(sealed_by,''),exception_id
		FROM project_end_seal WHERE project_id=?`, projectID).
		Scan(&s.ProjectID, &s.EndMode, &s.SnapshotID, &s.SnapshotVersion, &s.SealedAt, &s.SealedBy, &exID)
	if err != nil {
		return nil
	}
	if exID.Valid {
		s.ExceptionID = exID.Int64
	}
	return &s
}

// loadSnapshot 按版本读取快照；version<=0 时读项目最新一条。
func loadSnapshot(database *db.DB, projectID string, version int) map[string]any {
	var id, status, endMode, grade, ruleVersion, scenarioVersion, sealedAt, sealedBy, payload string
	var v int
	var score, evidenceRate, coreRate float64
	var maxScore, hit, total, coreTotal, coreDone int
	query := `SELECT id,version,snapshot_status,COALESCE(end_mode,''),score,COALESCE(grade,''),max_score,hit,total,
		evidence_complete_rate,core_complete_rate,core_total,core_done,COALESCE(rule_version,''),
		COALESCE(scenario_version,''),COALESCE(payload,''),COALESCE(sealed_at,''),COALESCE(sealed_by,'')
		FROM score_snapshots WHERE project_id=?`
	args := []any{projectID}
	if version > 0 {
		query += ` AND version=?`
		args = append(args, version)
	} else {
		query += ` ORDER BY version DESC LIMIT 1`
	}
	err := database.QueryRow(query, args...).Scan(&id, &v, &status, &endMode, &score, &grade, &maxScore,
		&hit, &total, &evidenceRate, &coreRate, &coreTotal, &coreDone, &ruleVersion, &scenarioVersion,
		&payload, &sealedAt, &sealedBy)
	if err != nil {
		return nil
	}
	var detail any
	if json.Unmarshal([]byte(payload), &detail) != nil {
		detail = nil
	}
	return map[string]any{
		"id": id, "version": v, "snapshotStatus": status, "endMode": endMode,
		"score": score, "grade": grade, "maxScore": maxScore, "hit": hit, "total": total,
		"evidenceCompleteRate": evidenceRate, "coreCompleteRate": coreRate,
		"coreTotal": coreTotal, "coreDone": coreDone, "ruleVersion": ruleVersion,
		"scenarioVersion": scenarioVersion, "sealedAt": sealedAt, "sealedBy": sealedBy,
		"snapshot": detail,
	}
}

// sealedScoreResult 读取封存版本对应的 ScoreResult；未封存或快照缺失返回 nil。
// 只读历史快照，不回写、不重算，保证剧本后续调整不影响已封存项目的展示口径。
func sealedScoreResult(database *db.DB, projectID string) *ScoreResult {
	seal := loadSeal(database, projectID)
	if seal == nil || seal.SnapshotVersion <= 0 {
		return nil
	}
	var payload, sealedGradeVal string
	if err := database.QueryRow(`SELECT COALESCE(payload,''),COALESCE(grade,'') FROM score_snapshots WHERE project_id=? AND version=?`,
		projectID, seal.SnapshotVersion).Scan(&payload, &sealedGradeVal); err != nil {
		return nil
	}
	var p snapshotPayload
	if json.Unmarshal([]byte(payload), &p) != nil || p.Score == nil {
		return nil
	}
	sc := *p.Score
	sc.ProjectID = projectID
	sc.Ended = true
	sc.Steps = nil
	// P0-04：等级以封存列的封顶值为准，只覆盖展示层，不改写快照 payload。
	// 历史快照（如 P2026-001）payload 内仍是旧等级，这里统一后大屏与报告一致。
	if strings.TrimSpace(sealedGradeVal) != "" {
		sc.Grade = sealedGradeVal
	}
	return &sc
}

// displayScore 展示口径：已封存项目读封存快照，未封存项目读实时 Score()。
// 仅用于只读展示（工作台 / 大屏 / 报告 / 项目评分查询）；写路径与重算仍调用 Score()。
func displayScore(database *db.DB, projectID string) *ScoreResult {
	if sc := sealedScoreResult(database, projectID); sc != nil {
		return sc
	}
	return Score(database, projectID)
}

func listSnapshots(database *db.DB, projectID string) []gin.H {
	var out []gin.H
	rows, err := database.Query(`SELECT version,snapshot_status,COALESCE(end_mode,''),score,COALESCE(grade,''),
		COALESCE(evidence_complete_rate,0),COALESCE(core_complete_rate,0),COALESCE(rule_version,''),
		COALESCE(sealed_at,''),COALESCE(sealed_by,''),COALESCE(recalc_from,0),COALESCE(recalc_reason,'')
		FROM score_snapshots WHERE project_id=? ORDER BY version DESC`, projectID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var version, recalcFrom int
		var status, endMode, grade, ruleVersion, sealedAt, sealedBy, recalcReason string
		var score, evidenceRate, coreRate float64
		if rows.Scan(&version, &status, &endMode, &score, &grade, &evidenceRate, &coreRate,
			&ruleVersion, &sealedAt, &sealedBy, &recalcFrom, &recalcReason) != nil {
			continue
		}
		out = append(out, gin.H{
			"version": version, "snapshotStatus": status, "endMode": endMode, "score": score,
			"grade": grade, "evidenceCompleteRate": evidenceRate, "coreCompleteRate": coreRate,
			"ruleVersion": ruleVersion, "sealedAt": sealedAt, "sealedBy": sealedBy,
			"recalcFrom": recalcFrom, "recalcReason": recalcReason,
		})
	}
	return out
}

// loadEndException 读取项目缺项结束说明（报告用）。
func loadEndException(database *db.DB, projectID string) map[string]any {
	var reason, missingKeys, owner, deadline, approver, createdBy, createdAt string
	var missingCount int
	err := database.QueryRow(`SELECT COALESCE(reason,''),COALESCE(missing_keys,''),COALESCE(missing_count,0),
		COALESCE(owner,''),COALESCE(deadline,''),COALESCE(approver,''),COALESCE(created_by,''),COALESCE(created_at,'')
		FROM project_end_exception WHERE project_id=? ORDER BY id DESC LIMIT 1`, projectID).
		Scan(&reason, &missingKeys, &missingCount, &owner, &deadline, &approver, &createdBy, &createdAt)
	if err != nil {
		return nil
	}
	var keys []string
	_ = json.Unmarshal([]byte(missingKeys), &keys)
	return map[string]any{
		"reason": reason, "missingKeys": keys, "missingCount": missingCount,
		"owner": owner, "deadline": deadline, "approver": approver,
		"createdBy": createdBy, "createdAt": createdAt,
	}
}

// ========================================================================
// 结束演练主流程
// ========================================================================

// executeDrillFinish 结束演练：核对清单 → 缺项说明 → 状态 → 快照 → timeline → 审计。
// 返回 (HTTP 状态码, 响应体)；dryRun 时只返回核对清单，不写库。
func executeDrillFinish(database *db.DB, u *auth.User, p *drillV1Project, body map[string]any) (int, gin.H) {
	ck := buildEndChecklist(database, p)

	dryRun := false
	if v, ok := body["dryRun"].(bool); ok && v {
		dryRun = true
	}
	if strOr(body["mode"], "") == "check" {
		dryRun = true
	}
	if dryRun {
		return 200, gin.H{"ok": true, "dryRun": true, "projectId": p.ID, "checklist": ck}
	}

	// 运维强制结束：必须填写原因；有缺项时仍写入缺项说明，便于事后追溯。
	force, _ := body["force"].(bool)
	forceReason := strings.TrimSpace(strOr(body["reason"], ""))

	actor := u.Name
	if strings.TrimSpace(actor) == "" {
		actor = u.Username
	}

	ex := parseEndException(body["exception"])
	if force {
		if len([]rune(forceReason)) < 5 {
			return 400, gin.H{"error": "强制结束必须填写不少于 5 个字的原因", "forceReasonRequired": true, "checklist": ck}
		}
		if ex == nil {
			ex = &drillEndException{Approver: actor}
		}
		if strings.TrimSpace(ex.Reason) == "" {
			ex.Reason = forceReason
		} else {
			ex.Reason = "【强制结束】" + ex.Reason
		}
		if len(ex.MissingKeys) == 0 {
			for _, it := range ck.RequiredMissing {
				ex.MissingKeys = append(ex.MissingKeys, it.NodeID)
			}
			for _, it := range ck.CoreMissing {
				ex.MissingKeys = append(ex.MissingKeys, it.Key)
			}
		}
	} else {
		if ex != nil && !ck.NeedsException {
			// 证据已齐时忽略缺项说明，避免在报告中产生无意义的缺项记录
			ex = nil
		}
		if ex == nil && ck.NeedsException {
			return 400, gin.H{
				"error":          ck.Hint,
				"needsException": true,
				"checklist":      ck,
			}
		}
		if ex != nil {
			if msg := normalizeEndException(ex, ck); msg != "" {
				return 400, gin.H{"error": msg, "needsException": true, "checklist": ck}
			}
		}
	}

	endMode := "完整"
	snapshotStatus := "已封存"
	if force {
		endMode = "异常结束"
		snapshotStatus = "异常封存"
	} else if ck.NeedsException {
		endMode = "有缺项"
		// 未完成必做节点/核心证据即降级封存，避免与完整结束混淆。
		snapshotStatus = "降级封存"
	}

	// 只计算一次评分：强制按「已结束」口径补扣未完成节点，快照/通知共用。
	final := scoreInternal(database, p.ID, true)
	if final == nil {
		return 404, gin.H{"error": "演练项目不存在"}
	}

	tx, err := database.Begin()
	if err != nil {
		return 500, gin.H{"error": "数据库事务失败"}
	}
	defer tx.Rollback()

	sealed, err := insertScoreSnapshot(tx, database, p, final, ck, ex, actor, endMode, snapshotStatus, 0, "")
	if err != nil {
		return 500, gin.H{"error": "写入评分快照失败：" + err.Error()}
	}

	var exceptionID int64
	if ex != nil {
		missingJSON, _ := json.Marshal(ex.MissingKeys)
		res, err := tx.Exec(`INSERT INTO project_end_exception
			(project_id,snapshot_id,reason,missing_keys,missing_count,owner,deadline,approver,created_by,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			p.ID, sealed.ID, ex.Reason, string(missingJSON), len(ex.MissingKeys), ex.Owner, ex.Deadline,
			ex.Approver, actor, sealed.SealedAt)
		if err != nil {
			return 500, gin.H{"error": "写入缺项结束说明失败：" + err.Error()}
		}
		exceptionID, _ = res.LastInsertId()
	}

	// 结束前结算未结束的暂停段：与原状态接口/Node 行为一致，避免 ended 后 paused_at 残留。
	var pausedAt sql.NullString
	var pausedSec sql.NullInt64
	_ = tx.QueryRow(`SELECT paused_at, paused_sec FROM projects WHERE id=? FOR UPDATE`, p.ID).Scan(&pausedAt, &pausedSec)
	ps := int64(0)
	if pausedSec.Valid {
		ps = pausedSec.Int64
	}
	if pausedAt.Valid && strings.TrimSpace(pausedAt.String) != "" {
		ps += secBetweenLocal(pausedAt.String, sealed.SealedAt)
	}

	res, err := tx.Exec(`UPDATE projects SET status='已结束', ended_at=?, paused_at=NULL, paused_sec=? WHERE id=? AND status IN ('演练中','暂停')`,
		sealed.SealedAt, ps, p.ID)
	if err != nil {
		return 500, gin.H{"error": "结束演练失败：" + err.Error()}
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 409, gin.H{"error": "演练状态已变化，请刷新后重试", "checklist": ck}
	}

	_, err = tx.Exec(`INSERT INTO project_end_seal
		(project_id,end_mode,snapshot_id,snapshot_version,sealed_at,sealed_by,exception_id)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE end_mode=VALUES(end_mode),snapshot_id=VALUES(snapshot_id),
			snapshot_version=VALUES(snapshot_version),sealed_at=VALUES(sealed_at),
			sealed_by=VALUES(sealed_by),exception_id=VALUES(exception_id)`,
		p.ID, endMode, sealed.ID, sealed.Version, sealed.SealedAt, actor, sqlNullInt(int(exceptionID)))
	if err != nil {
		return 500, gin.H{"error": "写入封存记录失败：" + err.Error()}
	}

	detail := fmt.Sprintf("结束演练（%s）：正确流程 %d/%d，得分 %.1f，评级 %s；核心证据 %d/%d（%.1f%%），证据归档 %d/%d（%.1f%%）；缺项 %d 项；评分快照 v%d（%s）",
		endMode, final.Hit, final.Total, final.Score, sealed.Grade,
		ck.CoreDone, ck.CoreTotal, ck.CoreRate, ck.EvidenceDone, ck.EvidenceTotal, ck.EvidenceRate,
		ck.CorePending+ck.SupportPending, sealed.Version, sealed.RuleVersion)
	if ex != nil {
		if force {
			detail += "；异常结束原因：" + forceReason
		} else {
			detail += fmt.Sprintf("；批准人 %s；责任人 %s；补录时限 %s；原因：%s",
				ex.Approver, ex.Owner, ex.Deadline, ex.Reason)
		}
	}
	if _, err := tx.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq)
		VALUES (?,?,?,?,?,?,?,?)`,
		p.ID, hhmm(), actor, u.Role.ID, "演练结束", detail, "P1", nil); err != nil {
		return 500, gin.H{"error": "写入结束时间线失败：" + err.Error()}
	}

	if _, err := tx.Exec(`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
		sealed.SealedAt, actor, u.Role.ID, u.ID, truncRunes("结束演练："+detail, 500), ""); err != nil {
		slog.Error("结束演练写入审计失败", "project_id", p.ID, "err", err)
		return 500, gin.H{"error": "写入结束演练审计失败：" + err.Error()}
	}

	if err := tx.Commit(); err != nil {
		return 500, gin.H{"error": "提交结束演练失败：" + err.Error()}
	}

	pushDrillEndSealNotif(database, p, final, sealed)

	return 200, gin.H{
		"ok": true, "projectId": p.ID, "endMode": endMode, "forced": force,
		"sealed": gin.H{
			"snapshotId": sealed.ID, "version": sealed.Version, "grade": sealed.Grade,
			"score": sealed.Score, "sealedAt": sealed.SealedAt, "sealedBy": sealed.SealedBy,
			"ruleVersion": sealed.RuleVersion, "scenarioVersion": sealed.ScenarioVersion,
			"evidenceCompleteRate": sealed.EvidenceCompleteRate, "coreCompleteRate": sealed.CoreCompleteRate,
		},
		"score": final, "checklist": ck, "timelineAction": "演练结束",
	}
}

// resealProjectSnapshot 已封存项目重新考评：只追加新版本，不动 project_end_seal 指向的封存版本。
func resealProjectSnapshot(database *db.DB, p *drillV1Project, sc *ScoreResult, seal *sealSummary, actor, reason string) int {
	if p == nil || sc == nil || seal == nil {
		return 0
	}
	ck := buildEndChecklist(database, p)
	ex := loadExceptionRecord(database, p.ID)
	tx, err := database.Begin()
	if err != nil {
		return 0
	}
	defer tx.Rollback()
	snap, err := insertScoreSnapshot(tx, database, p, sc, ck, ex, actor, seal.EndMode, "重算", seal.SnapshotVersion, reason)
	if err != nil {
		return 0
	}
	if err := tx.Commit(); err != nil {
		return 0
	}
	return snap.Version
}

func loadExceptionRecord(database *db.DB, projectID string) *drillEndException {
	var reason, missingKeys, owner, deadline, approver string
	err := database.QueryRow(`SELECT COALESCE(reason,''),COALESCE(missing_keys,''),COALESCE(owner,''),
		COALESCE(deadline,''),COALESCE(approver,'')
		FROM project_end_exception WHERE project_id=? ORDER BY id DESC LIMIT 1`, projectID).
		Scan(&reason, &missingKeys, &owner, &deadline, &approver)
	if err != nil {
		return nil
	}
	ex := &drillEndException{Reason: reason, Owner: owner, Deadline: deadline, Approver: approver}
	_ = json.Unmarshal([]byte(missingKeys), &ex.MissingKeys)
	return ex
}

// ========================================================================
// 补封存（已结束但历史上未封存的项目）
// ========================================================================

type SupplementalSealInput struct {
	Reason    string
	Owner     string
	Deadline  string
	Approver  string
	Actor     string
	ActorRole string
	ActorID   string
	DryRun    bool
}

type SupplementalSealResult struct {
	ProjectID            string       `json:"projectId"`
	EndMode              string       `json:"endMode"`
	SnapshotID           string       `json:"snapshotId"`
	Version              int          `json:"version"`
	Score                float64      `json:"score"`
	Grade                string       `json:"grade"`
	EvidenceCompleteRate float64      `json:"evidenceCompleteRate"`
	CoreCompleteRate     float64      `json:"coreCompleteRate"`
	CoreTotal            int          `json:"coreTotal"`
	CoreDone             int          `json:"coreDone"`
	RuleVersion          string       `json:"ruleVersion"`
	ScenarioVersion      string       `json:"scenarioVersion"`
	SealedAt             string       `json:"sealedAt"`
	SealedBy             string       `json:"sealedBy"`
	ExceptionID          int64        `json:"exceptionId"`
	LiveScore            float64      `json:"liveScore"`
	LiveGrade            string       `json:"liveGrade"`
	Matched              bool         `json:"matched"`
	Checklist            endChecklist `json:"checklist"`
}

// SupplementalSeal 对「已结束但无封存」的项目补写评分封存。
// 与正常结束共用核对、评分、快照与缺项逻辑，但不更新 projects（status/started_at/ended_at/paused_sec 保持原值），
// 只追加 score_snapshots / project_end_exception / project_end_seal / timeline / audit_logs。
func SupplementalSeal(database *db.DB, projectID string, in SupplementalSealInput) (*SupplementalSealResult, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("projectId 不能为空")
	}
	if strings.TrimSpace(in.Actor) == "" {
		return nil, fmt.Errorf("操作人不能为空")
	}
	p := drillV1ProjectOf(database, projectID)
	if p == nil {
		return nil, fmt.Errorf("演练项目不存在：%s", projectID)
	}
	if p.Status != "已结束" {
		return nil, fmt.Errorf("仅「已结束」项目可补封存，当前状态：%s", p.Status)
	}
	var endedAt string
	if err := database.QueryRow(`SELECT COALESCE(ended_at,'') FROM projects WHERE id=?`, projectID).Scan(&endedAt); err != nil {
		return nil, fmt.Errorf("读取项目结束时间失败：%v", err)
	}
	if strings.TrimSpace(endedAt) == "" {
		return nil, fmt.Errorf("项目 ended_at 为空，不能补封存")
	}
	if seal := loadSeal(database, projectID); seal != nil {
		return nil, fmt.Errorf("项目已封存（快照 v%d），无需补封存", seal.SnapshotVersion)
	}
	var snapCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM score_snapshots WHERE project_id=?`, projectID).Scan(&snapCount); err != nil {
		return nil, fmt.Errorf("检查评分快照失败：%v", err)
	}
	if snapCount > 0 {
		return nil, fmt.Errorf("已存在 %d 条评分快照，请先核验，避免重复补封存", snapCount)
	}

	ck := buildEndChecklist(database, p)
	if !ck.NeedsException {
		return nil, fmt.Errorf("结束核对未发现缺项（核心证据 %d/%d，证据 %d/%d），不属于有缺项结束，请走正常结束流程",
			ck.CoreDone, ck.CoreTotal, ck.EvidenceDone, ck.EvidenceTotal)
	}

	ex := &drillEndException{
		Reason:   strings.TrimSpace(in.Reason),
		Owner:    strings.TrimSpace(in.Owner),
		Deadline: strings.TrimSpace(in.Deadline),
		Approver: strings.TrimSpace(in.Approver),
	}
	if msg := normalizeEndException(ex, ck); msg != "" {
		return nil, fmt.Errorf("缺项说明校验失败：%s", msg)
	}

	sc := scoreInternal(database, projectID, true)
	if sc == nil {
		return nil, fmt.Errorf("评分计算失败")
	}

	if in.DryRun {
		return &SupplementalSealResult{
			ProjectID:            p.ID,
			EndMode:              "有缺项",
			Score:                sc.Score,
			Grade:                sealedGrade(sc.Grade, ck.CoreRate, ck.CoreTotal, "有缺项"),
			EvidenceCompleteRate: ck.EvidenceRate,
			CoreCompleteRate:     ck.CoreRate,
			CoreTotal:            ck.CoreTotal,
			CoreDone:             ck.CoreDone,
			Checklist:            ck,
		}, nil
	}

	tx, err := database.Begin()
	if err != nil {
		return nil, fmt.Errorf("数据库事务失败：%v", err)
	}
	defer tx.Rollback()

	sealed, err := insertScoreSnapshot(tx, database, p, sc, ck, ex, in.Actor, "有缺项", "已封存", 0, "")
	if err != nil {
		return nil, fmt.Errorf("写入评分快照失败：%v", err)
	}

	missingJSON, _ := json.Marshal(ex.MissingKeys)
	res, err := tx.Exec(`INSERT INTO project_end_exception
		(project_id,snapshot_id,reason,missing_keys,missing_count,owner,deadline,approver,created_by,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, sealed.ID, ex.Reason, string(missingJSON), len(ck.CoreMissing), ex.Owner, ex.Deadline,
		ex.Approver, in.Actor, sealed.SealedAt)
	if err != nil {
		return nil, fmt.Errorf("写入缺项结束说明失败：%v", err)
	}
	exceptionID, _ := res.LastInsertId()

	if _, err := tx.Exec(`INSERT INTO project_end_seal
		(project_id,end_mode,snapshot_id,snapshot_version,sealed_at,sealed_by,exception_id)
		VALUES (?,?,?,?,?,?,?)`,
		p.ID, "有缺项", sealed.ID, sealed.Version, sealed.SealedAt, in.Actor, exceptionID); err != nil {
		return nil, fmt.Errorf("写入封存记录失败：%v", err)
	}

	detail := fmt.Sprintf("演练结束（补封存/有缺项）：原结束时间 %s，正确流程 %d/%d，得分 %.1f，评级 %s；核心证据 %d/%d（%.1f%%），证据归档 %d/%d（%.1f%%）；缺项 %d 项；评分快照 v%d（%s）；原因：%s；责任人 %s；补录时限 %s；批准人 %s",
		endedAt, sc.Hit, sc.Total, sc.Score, sealed.Grade,
		ck.CoreDone, ck.CoreTotal, ck.CoreRate, ck.EvidenceDone, ck.EvidenceTotal, ck.EvidenceRate,
		ck.CorePending+ck.SupportPending, sealed.Version, sealed.RuleVersion,
		ex.Reason, ex.Owner, ex.Deadline, ex.Approver)
	if _, err := tx.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq)
		VALUES (?,?,?,?,?,?,?,?)`,
		p.ID, hhmm(), in.Actor, in.ActorRole, "演练结束（补封存）", detail, "P1", nil); err != nil {
		return nil, fmt.Errorf("写入结束时间线失败：%v", err)
	}
	if _, err := tx.Exec(`INSERT INTO audit_logs (time,user,role_id,user_id,action,ip) VALUES (?,?,?,?,?,?)`,
		sealed.SealedAt, in.Actor, in.ActorRole, in.ActorID, truncRunes("补封存："+detail, 500), "127.0.0.1"); err != nil {
		return nil, fmt.Errorf("写入审计日志失败：%v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交补封存失败：%v", err)
	}

	live := Score(database, projectID)
	out := &SupplementalSealResult{
		ProjectID:            p.ID,
		EndMode:              "有缺项",
		SnapshotID:           sealed.ID,
		Version:              sealed.Version,
		Score:                sealed.Score,
		Grade:                sealed.Grade,
		EvidenceCompleteRate: sealed.EvidenceCompleteRate,
		CoreCompleteRate:     sealed.CoreCompleteRate,
		CoreTotal:            sealed.CoreTotal,
		CoreDone:             sealed.CoreDone,
		RuleVersion:          sealed.RuleVersion,
		ScenarioVersion:      sealed.ScenarioVersion,
		SealedAt:             sealed.SealedAt,
		SealedBy:             sealed.SealedBy,
		ExceptionID:          exceptionID,
		Checklist:            ck,
	}
	if live != nil {
		out.LiveScore, out.LiveGrade = live.Score, live.Grade
		out.Matched = math.Abs(live.Score-sealed.Score) < 0.001
	}
	return out, nil
}

// pushDrillEndSealNotif 结束通知（幂等：同项目只推一条 drill_end_score）。
func pushDrillEndSealNotif(database *db.DB, p *drillV1Project, final *ScoreResult, sealed *scoreSnapshot) {
	if final == nil {
		return
	}
	var dup int
	_ = database.QueryRow(`SELECT COUNT(*) FROM notifs WHERE project_id=? AND title LIKE '演练结束%'`, p.ID).Scan(&dup)
	if dup > 0 {
		return
	}
	vars := map[string]any{
		"project": p.Name, "mode": p.Mode, "score": final.Score,
		"hit": final.Hit, "total": final.Total, "grade": sealed.Grade,
	}
	title, descr, source, level, fromDB := RenderNotifTemplate(database, "drill_end_score", vars)
	if !fromDB {
		title = "演练结束 · 综合得分 " + strVal(final.Score) + " 分"
		descr = fmt.Sprintf("演练项目「%s」（%s）已结束；正确流程完成 %d/%d 项，评级 %s；核心证据完整率 %.1f%%。",
			p.Name, p.Mode, final.Hit, final.Total, sealed.Grade, sealed.CoreCompleteRate)
		source, level = "演练调度", "P1"
	}
	seq := 1
	if err := database.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM notifs`).Scan(&seq); err != nil {
		seq = 1
	}
	id := "N" + fmt.Sprint(time.Now().UnixMilli()) + "_" + itoa(100+randInt(900))
	if _, err := database.Exec(
		`INSERT INTO notifs (id,time,level,source,title,descr,seq,target,project_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, time.Now().Format("2006-01-02 15:04"), level, truncRunes(source, 40),
		truncRunes(title, 120), truncRunes(descr, 500), seq, "internal", p.ID,
	); err != nil {
		logSideEffectErr("notifs", p.ID, "演练结束通知", err)
	}
}
