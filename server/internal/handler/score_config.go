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
	// QualityRatio 绑定表单模板的节点：从节点分中划出 20% 作为处置质量分
	QualityRatio = 0.20
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

// qualityParts 计算处置质量扣分（节点分内部 20% 按必填齐全度比例扣减）
func qualityParts(nodeScore int, formsJSON string, req []formRequiredField) ([]scoreDeductPart, int, int) {
	total := 0
	for _, f := range req {
		if f.Required {
			total++
		}
	}
	if total == 0 || nodeScore <= 0 {
		return nil, 0, 0
	}
	missing := missingRequiredFields(formsJSON, req)
	if len(missing) == 0 {
		return nil, 0, total
	}
	ratio := QualityRatio * float64(len(missing)) / float64(total)
	d := math.Round(float64(nodeScore)*ratio*10) / 10
	if d <= 0 {
		return nil, len(missing), total
	}
	return []scoreDeductPart{{
		Type: "处置质量不足（必填 " + strconv.Itoa(total-len(missing)) + "/" + strconv.Itoa(total) +
			" 齐全，缺 " + strconv.Itoa(len(missing)) + " 项）（扣本节点 20% 内按比例）",
		Point: d,
	}}, len(missing), total
}

// correctNodeDeduction 正确节点（已提交）的统一扣分口径，Score() 与提交写库共用
func correctNodeDeduction(nodeScore int, late bool, recvBad bool, formsJSON string, req []formRequiredField) (float64, []scoreDeductPart) {
	var parts []scoreDeductPart
	if late {
		parts = append(parts, scoreDeductPart{Type: "超时完成（扣20%）", Point: ratioDeduct(nodeScore, TimeoutPenaltyRatio)})
	}
	if recvBad {
		parts = append(parts, scoreDeductPart{Type: "接收人选择错误（扣30%）", Point: ratioDeduct(nodeScore, ReceiverErrPenaltyRatio)})
	}
	if qp, _, _ := qualityParts(nodeScore, formsJSON, req); len(qp) > 0 {
		parts = append(parts, qp...)
	}
	parts = capNodeParts(parts, nodeScore)
	total := 0.0
	for _, p := range parts {
		total += p.Point
	}
	return total, parts
}
