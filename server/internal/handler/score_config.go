package handler

import (
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"drill-system/internal/db"
)

// ========== 评分权重集中配置（一处修改，Score() / 提交记录 delta / 文案全链生效） ==========
//
// 扣分制总分仍为 100，maxScore 仍为正确节点分值合计；以下比例都从「节点分值」内部拆分，
// 不额外加减总分。
const (
	// ReceiverErrPenaltyRatio 接收人选择错误：扣本节点分值的 30%
	ReceiverErrPenaltyRatio = 0.30
	// TimeoutPenaltyRatio 超时完成：扣本节点分值的 20%
	TimeoutPenaltyRatio = 0.20
	// KeywordHitPenaltyRatio 处置关键词命中少于 KeywordMinHits 个：扣本节点分值的 30%
	KeywordHitPenaltyRatio = 0.30
	// KeywordMinHits 关键词命中下限（评审口径：execute ≥2 keywords）
	KeywordMinHits = 2
	// EvidencePenaltyRatio 节点 evidence_req 未提交或未命中：扣本节点分值的 20%
	EvidencePenaltyRatio = 0.20
	// NodeMaxPenaltyRatio 单节点合计扣减上限（占节点分值），避免一次错误把节点分扣成负数
	NodeMaxPenaltyRatio = 0.80
)

// scoreDeductPart 单节点内的一项扣减（用于统一生成明细与合计）
type scoreDeductPart struct {
	Type  string
	Point float64
}

// ratioDeduct 按比例计算扣分，保留 1 位小数（与前端展示口径一致）
func ratioDeduct(nodeScore int, ratio float64) float64 {
	if nodeScore <= 0 || ratio <= 0 {
		return 0
	}
	return math.Round(float64(nodeScore)*ratio*10) / 10
}

// capNodeParts 施加单节点 80% 上限；未超限原样返回
func capNodeParts(parts []scoreDeductPart, nodeScore int) []scoreDeductPart {
	if len(parts) == 0 || nodeScore <= 0 {
		return parts
	}
	capDeduct := ratioDeduct(nodeScore, NodeMaxPenaltyRatio)
	total := 0.0
	for _, p := range parts {
		total += p.Point
	}
	if total <= capDeduct {
		return parts
	}
	scaled := make([]scoreDeductPart, 0, len(parts))
	acc := 0.0
	for i, p := range parts {
		d := math.Round(p.Point/total*capDeduct*10) / 10
		if i == len(parts)-1 {
			d = math.Round((capDeduct-acc)*10) / 10
		}
		if d <= 0 {
			continue
		}
		acc += d
		scaled = append(scaled, scoreDeductPart{Type: p.Type, Point: d})
	}
	return scaled
}

// ========== 表单模板必填字段（服务端强校验 + 处置质量分共用） ==========

type formRequiredField struct {
	Label    string
	Key      string
	Required bool
}

// Display 返回「中文名(key)」；模板无 key 时只返回中文名
func (f formRequiredField) Display() string {
	if f.Key != "" && f.Key != f.Label {
		return f.Label + "(" + f.Key + ")"
	}
	return f.Label
}

// loadFormRequiredFields 读取表单模板的字段定义（面板中 string 形式默认必填）
func loadFormRequiredFields(database *db.DB, formID string) []formRequiredField {
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return nil
	}
	var fieldsStr sql.NullString
	if err := database.QueryRow(`SELECT COALESCE(fields,'[]') FROM form_templates WHERE id=?`, formID).Scan(&fieldsStr); err != nil {
		return nil
	}
	return parseFormRequiredFields(fieldsStr.String)
}

func parseFormRequiredFields(fieldsJSON string) []formRequiredField {
	var arr []any
	if json.Unmarshal([]byte(fieldsJSON), &arr) != nil {
		return nil
	}
	out := make([]formRequiredField, 0, len(arr))
	for _, x := range arr {
		switch v := x.(type) {
		case string:
			label := clipStr(strings.TrimSpace(v), 40)
			if label == "" {
				continue
			}
			out = append(out, formRequiredField{Label: label, Required: true})
		case map[string]any:
			labelRaw, _ := v["label"].(string)
			if strings.TrimSpace(labelRaw) == "" {
				labelRaw, _ = v["name"].(string)
			}
			label := clipStr(strings.TrimSpace(labelRaw), 40)
			if label == "" {
				continue
			}
			key, _ := v["key"].(string)
			key = strings.TrimSpace(key)
			required := true
			if r, ok := v["required"].(bool); ok && !r {
				required = false
			}
			out = append(out, formRequiredField{Label: label, Key: key, Required: required})
		}
	}
	return out
}

func nonEmptyFormValue(v any) bool {
	_, ok := v.(string)
	if ok {
		return strings.TrimSpace(v.(string)) != ""
	}
	return formValueString(v) != ""
}

// missingRequiredFields 返回缺失/空串的必填字段（按模板定义顺序）
func missingRequiredFields(formsJSON string, req []formRequiredField) []formRequiredField {
	var m map[string]any
	if strings.TrimSpace(formsJSON) != "" {
		_ = json.Unmarshal([]byte(formsJSON), &m)
	}
	var missing []formRequiredField
	for _, f := range req {
		if !f.Required {
			continue
		}
		v, ok := m[f.Label]
		if !ok && f.Key != "" {
			v, ok = m[f.Key]
		}
		if !ok || !nonEmptyFormValue(v) {
			missing = append(missing, f)
		}
	}
	return missing
}

// joinFieldDisplays 生成「中文名(key)、中文名(key)」文案
func joinFieldDisplays(fields []formRequiredField) string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Display())
	}
	return strings.Join(names, "、")
}

// ========== 处置质量：关键词命中 + 证据要求（评审原文口径） ==========

// submissionQuality 一次处置提交的文本证据（实战版结算用；基础版无提交文本，HasSubmission=false）
type submissionQuality struct {
	Late         bool
	RecvBad      bool
	HasSubmission bool
	// Text 全部可检索文本：处置措施、处置结果、发现情况、证据说明、表单值
	Text         string
	// EvidenceText 单独的「证据材料说明」字段（判断是否提交证据）
	EvidenceText string
	Keywords     []string
	EvidenceReq  string
}

func splitReqTokens(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune("、,，;；|｜/", r)
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// keywordHitCount 统计处置文本命中的关键词个数
func keywordHitCount(keywords []string, text string) int {
	hits := 0
	for _, kw := range splitReqTokens(strings.Join(keywords, "、")) {
		if len([]rune(kw)) < 2 {
			continue
		}
		if strings.Contains(text, kw) {
			hits++
		}
	}
	return hits
}

// formValuesText 仅拼接表单「值」（排除字段名，避免字段标签本身命中关键词/证据要求）
func formValuesText(formsJSON string) string {
	var m map[string]any
	if strings.TrimSpace(formsJSON) == "" || json.Unmarshal([]byte(formsJSON), &m) != nil {
		return ""
	}
	vals := make([]string, 0, len(m))
	for _, v := range m {
		if s := formValueString(v); s != "" {
			vals = append(vals, s)
		}
	}
	return strings.Join(vals, "\n")
}

// evidenceReqTokens 解析 evidence_req：
// 现网为 JSON 数组（元素含 name/note），也兼容逗号分隔纯文本；取每个材料的名称作为检索项。
func evidenceReqTokens(req string) []string {
	req = strings.TrimSpace(req)
	if req == "" {
		return nil
	}
	var arr []any
	if json.Unmarshal([]byte(req), &arr) == nil {
		tokens := make([]string, 0, len(arr))
		for _, x := range arr {
			switch v := x.(type) {
			case string:
				tokens = append(tokens, splitReqTokens(v)...)
			case map[string]any:
				if name, _ := v["name"].(string); strings.TrimSpace(name) != "" {
					tokens = append(tokens, strings.TrimSpace(name))
				}
			}
		}
		return tokens
	}
	return splitReqTokens(req)
}

// evidenceRequirementHit evidence_req 的任一材料名称在处置文本中出现即视为命中
func evidenceRequirementHit(req string, text string) bool {
	for _, token := range evidenceReqTokens(req) {
		if len([]rune(token)) < 2 {
			continue
		}
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// correctNodeDeduction 正确节点（已提交）的统一扣分口径，Score() 与提交写库共用。
// 超时 20% / 接收人 30% / 关键词命中不足 30% / 证据未提交或未命中 20%，单节点合计上限 80%。
func correctNodeDeduction(nodeScore int, q submissionQuality) (float64, []scoreDeductPart) {
	var parts []scoreDeductPart
	if q.Late {
		parts = append(parts, scoreDeductPart{Type: "超时完成（扣本节点20%）", Point: ratioDeduct(nodeScore, TimeoutPenaltyRatio)})
	}
	if q.RecvBad {
		parts = append(parts, scoreDeductPart{Type: "接收人选择错误（扣本节点30%）", Point: ratioDeduct(nodeScore, ReceiverErrPenaltyRatio)})
	}
	if q.HasSubmission {
		kwTotal := len(splitReqTokens(strings.Join(q.Keywords, "、")))
		if kwTotal > 0 && len(q.Text) > 0 {
			threshold := KeywordMinHits
			if kwTotal < threshold {
				threshold = kwTotal
			}
			hits := keywordHitCount(q.Keywords, q.Text)
			if hits < threshold {
				parts = append(parts, scoreDeductPart{
					Type: "处置关键词命中不足（命中 " + strconv.Itoa(hits) + "/" + strconv.Itoa(threshold) + " 个，扣本节点30%）",
					Point: ratioDeduct(nodeScore, KeywordHitPenaltyRatio),
				})
			}
		}
		if req := strings.TrimSpace(q.EvidenceReq); req != "" {
			missingEvidence := strings.TrimSpace(q.EvidenceText) == ""
			if !missingEvidence && !evidenceRequirementHit(req, q.Text) {
				missingEvidence = true
			}
			if missingEvidence {
				parts = append(parts, scoreDeductPart{
					Type:  "证据材料未提交或未命中要求（扣本节点20%）",
					Point: ratioDeduct(nodeScore, EvidencePenaltyRatio),
				})
			}
		}
	}
	parts = capNodeParts(parts, nodeScore)
	kept := parts[:0]
	total := 0.0
	for _, p := range parts {
		if p.Point <= 0 {
			continue
		}
		kept = append(kept, p)
		total += p.Point
	}
	return total, kept
}
