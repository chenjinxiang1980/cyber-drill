package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
	// TextQualityPenaltyRatio 处置文本质量不达标（敷衍 / 过短 / 要素缺失）：扣本节点分值的 20%
	TextQualityPenaltyRatio = 0.20
	// DefaultMinMeasureLen 处置措施默认最少字数
	DefaultMinMeasureLen = 15
	// DefaultMinResultLen 处置结果默认最少字数
	DefaultMinResultLen = 10
	// DefaultFillerMaxLen 敷衍文本判定长度上限（去掉标点后不超过该字数且全部为敷衍词）
	DefaultFillerMaxLen = 8
	// HardGateMinTextRunes 提交硬门槛的绝对有效字数下限（措施+结果，去空白与标点）。
	// 仅拦「不写就等于没做这件事」的情况；节点配置的更严长度/要素一律转为扣分项。
	HardGateMinTextRunes = 8
)

// defaultFillerWords 敷衍词黑名单（可在节点 quality_rules.fillerWords 覆盖）
var defaultFillerWords = []string{
	"无", "已处理", "正常", "已知悉", "已阅", "收到", "已解决", "已确认",
	"按要求处理", "处理完毕", "同上", "无异常", "ok", "OK",
}

// ========== 节点级文字质量规则（script_nodes.quality_rules，缺省走默认口径） ==========

// QualityRules 节点级处置文字质量规则。
// 所有阈值、比例、黑名单、要素均在剧本编排「节点配置」中维护；库内没有该字段时
// 全部取默认值，且关键词判定退回旧口径（keywords + 命中≥2），避免老节点被误扣。
type QualityRules struct {
	// RequiredKeywords 必含关键词：缺一项按比例扣本节点分值
	RequiredKeywords []string `json:"requiredKeywords"`
	// BonusKeywords 加分关键词：每命中 1 个可抵扣 1 项必含缺失的比例
	BonusKeywords []string `json:"bonusKeywords"`
	// MinMeasureLen 处置措施最少字数（≤0 表示不校验）
	MinMeasureLen int `json:"minMeasureLen"`
	// MinResultLen 处置结果最少字数（≤0 表示不校验）
	MinResultLen int `json:"minResultLen"`
	// RequiredElements 必须出现的要素（对象 / 动作 / 时间 / 结论）
	RequiredElements []string `json:"requiredElements"`
	// FillerWords 敷衍词黑名单
	FillerWords []string `json:"fillerWords"`
	// FillerMaxLen 敷衍判定长度上限
	FillerMaxLen int `json:"fillerMaxLen"`
	// TextQualityRatio 文字质量不达标扣分比例（占节点分值）
	TextQualityRatio float64 `json:"textQualityRatio"`
	// KeywordPenaltyRatio 必含关键词缺失扣分比例（占节点分值，缺一项按比例折算）
	KeywordPenaltyRatio float64 `json:"keywordPenaltyRatio"`
}

// defaultQualityRules 未配置时的默认口径（与历史行为最接近 + 最小文字质量门槛）
func defaultQualityRules() QualityRules {
	return QualityRules{
		MinMeasureLen:       DefaultMinMeasureLen,
		MinResultLen:        DefaultMinResultLen,
		RequiredElements:    []string{"对象", "动作", "结论"},
		FillerWords:         append([]string{}, defaultFillerWords...),
		FillerMaxLen:        DefaultFillerMaxLen,
		TextQualityRatio:    TextQualityPenaltyRatio,
		KeywordPenaltyRatio: KeywordHitPenaltyRatio,
	}
}

// parseQualityRules 解析节点 quality_rules JSON；空串/非法 JSON/缺字段都回落到默认值。
func parseQualityRules(raw string) QualityRules {
	rules := defaultQualityRules()
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return rules
	}
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return defaultQualityRules()
	}
	return normalizeQualityRules(rules)
}

// normalizeQualityRules 清洗规则：比例限制在 [0, NodeMaxPenaltyRatio]，字数/长度不为负，
// 要素只保留引擎认识的四种。
func normalizeQualityRules(r QualityRules) QualityRules {
	def := defaultQualityRules()
	// 必含关键词保留「|」同义词组（一个元素=一项，组内任一写法命中即算命中）
	r.RequiredKeywords = cleanRequiredKeywordList(r.RequiredKeywords)
	r.BonusKeywords = cleanKeywordList(r.BonusKeywords)
	if r.MinMeasureLen < 0 {
		r.MinMeasureLen = 0
	}
	if r.MinResultLen < 0 {
		r.MinResultLen = 0
	}
	if r.FillerMaxLen < 0 {
		r.FillerMaxLen = 0
	}
	if r.FillerWords == nil {
		r.FillerWords = def.FillerWords
	} else {
		r.FillerWords = cleanKeywordList(r.FillerWords)
	}
	known := map[string]bool{"对象": true, "动作": true, "时间": true, "结论": true}
	elements := make([]string, 0, len(r.RequiredElements))
	seen := map[string]bool{}
	for _, e := range r.RequiredElements {
		e = strings.TrimSpace(e)
		if !known[e] || seen[e] {
			continue
		}
		seen[e] = true
		elements = append(elements, e)
	}
	r.RequiredElements = elements
	r.TextQualityRatio = clampRatio(r.TextQualityRatio)
	r.KeywordPenaltyRatio = clampRatio(r.KeywordPenaltyRatio)
	return r
}

func clampRatio(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > NodeMaxPenaltyRatio {
		return NodeMaxPenaltyRatio
	}
	return v
}

// ========== 匹配归一化：全半角 / 空白 / 大小写 / 数字体系 ==========

// numeralVariantRe 数字体系变体：罗马数字（Ⅱ / II）、中文数字（二）、阿拉伯数字（2）
// 紧邻「级 / 类 / 档 / 期」时统一折算为阿拉伯数字写法。
var numeralVariantRe = regexp.MustCompile(`(^|[^0-9a-z])([ivx]+|[一二三四五六七八九十])(级|类|档|期)`)

// timeNumeralRe 时间量词前的中文数字（含「两」「半」）：两小时 / 三十分钟 / 两天 / 半小时。
// 只匹配「数字 + 时间单位」，不会把「今天 / 次日 / 及时」这类无数字词误折算。
var timeNumeralRe = regexp.MustCompile(`([零〇一二两三四五六七八九十百半]+)(?:个)?(小时|分钟|天|日)`)

// chineseTimeNumeral 中文数字折算：半 → 0.5，两 → 2，十五 → 15，二十四 → 24，一百 → 100。
func chineseTimeNumeral(s string) (float64, bool) {
	if s == "半" {
		return 0.5, true
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	total, cur, seen := 0, 0, false
	for _, r := range s {
		if d, ok := digits[r]; ok {
			cur = d
			seen = true
			continue
		}
		switch r {
		case '十':
			if cur == 0 {
				cur = 1
			}
			total += cur * 10
			cur = 0
			seen = true
		case '百':
			if cur == 0 {
				cur = 1
			}
			total += cur * 100
			cur = 0
			seen = true
		default:
			return 0, false
		}
	}
	if !seen {
		return 0, false
	}
	total += cur
	if total <= 0 {
		return 0, false
	}
	return float64(total), true
}

// normalizeTimeNumerals 把时间量词前的中文数字统一为阿拉伯数字（两小时 → 2小时，半小时 → 0.5小时）。
func normalizeTimeNumerals(s string) string {
	if !strings.ContainsAny(s, "零〇一二两三四五六七八九十百半") {
		return s
	}
	return timeNumeralRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := timeNumeralRe.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		n, ok := chineseTimeNumeral(sub[1])
		if !ok {
			return m
		}
		return strconv.FormatFloat(n, 'f', -1, 64) + sub[2]
	})
}

// normalizeMatchText 关键词/要素匹配前的统一归一化：
//  1. 全角字符（含全角空格）→ 半角；
//  2. 去除所有空白（含中文与数字/字母之间的空格）；
//  3. 英文统一小写；
//  4. 罗马数字 / 中文数字 / 阿拉伯数字变体统一（Ⅱ级 / II级 / 二级 / 2 级 → 2级）。
//
// 提交文本与规则词都走同一函数，只用于比对，不改变入库原文。
func normalizeMatchText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			continue
		case r >= 0xFF01 && r <= 0xFF5E: // 全角 ASCII 区
			r -= 0xFEE0
		case r == 0x3000: // 全角空格
			continue
		}
		if r >= 0x2160 && r <= 0x216B { // Ⅰ ~ Ⅻ
			b.WriteString(strconv.Itoa(int(r-0x2160) + 1))
			continue
		}
		if r >= 0x2170 && r <= 0x217B { // ⅰ ~ ⅻ
			b.WriteString(strconv.Itoa(int(r-0x2170) + 1))
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	out := b.String()
	// 时间量词前的中文数字先折算为阿拉伯数字，供「时间」要素与关键词统一识别。
	out = normalizeTimeNumerals(out)
	if strings.ContainsAny(out, "ivx级类档期一二三四五六七八九十") {
		out = numeralVariantRe.ReplaceAllStringFunc(out, func(m string) string {
			idx := numeralVariantRe.FindStringSubmatch(m)
			if len(idx) != 4 {
				return m
			}
			n, ok := numeralToInt(idx[2])
			if !ok {
				return m
			}
			return idx[1] + strconv.Itoa(n) + idx[3]
		})
	}
	return out
}

// numeralToInt 罗马数字/中文数字单值转阿拉伯数字
func numeralToInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	switch s {
	case "一":
		return 1, true
	case "二":
		return 2, true
	case "三":
		return 3, true
	case "四":
		return 4, true
	case "五":
		return 5, true
	case "六":
		return 6, true
	case "七":
		return 7, true
	case "八":
		return 8, true
	case "九":
		return 9, true
	case "十":
		return 10, true
	}
	vals := map[byte]int{'i': 1, 'v': 5, 'x': 10}
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := vals[s[i]]
		if !ok {
			return 0, false
		}
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	return total, total > 0
}

// splitKeywordAlternatives 把一个必含项按同义词分隔符「| ｜」拆成可选写法。
// 组内任一写法命中即视为该必含项命中；纯字符串（无分隔符）即单写法，保持旧语义。
func splitKeywordAlternatives(item string) []string {
	parts := strings.FieldsFunc(item, func(r rune) bool { return r == '|' || r == '｜' })
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := normalizeMatchText(p)
		if len([]rune(key)) < 2 || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// requiredKeywordGroups 把必含关键词解析为同义词组列表（保持配置顺序）
func requiredKeywordGroups(items []string) [][]string {
	var groups [][]string
	seen := map[string]bool{}
	for _, item := range items {
		alts := splitKeywordAlternatives(item)
		if len(alts) == 0 {
			continue
		}
		key := normalizeMatchText(strings.Join(alts, "|"))
		if seen[key] {
			continue
		}
		seen[key] = true
		groups = append(groups, alts)
	}
	return groups
}

// missingRequiredGroups 返回未命中的必含词组（组内任一写法命中即算命中）
func missingRequiredGroups(groups [][]string, text string) []string {
	norm := normalizeMatchText(text)
	var missing []string
	for _, alts := range groups {
		hit := false
		for _, alt := range alts {
			if strings.Contains(norm, normalizeMatchText(alt)) {
				hit = true
				break
			}
		}
		if !hit {
			missing = append(missing, strings.Join(alts, "|"))
		}
	}
	return missing
}

// bonusHitCount 加分词命中数（加分词不分组，逐个计数）
func bonusHitCount(words []string, text string) int {
	norm := normalizeMatchText(text)
	seen := map[string]bool{}
	hits := 0
	for _, w := range words {
		for _, kw := range splitReqTokens(w) {
			key := normalizeMatchText(kw)
			if len([]rune(key)) < 2 || seen[key] {
				continue
			}
			seen[key] = true
			if strings.Contains(norm, key) {
				hits++
			}
		}
	}
	return hits
}

// cleanRequiredKeywordList 清洗必含关键词：同义词组用「|」拼回一个元素，保持可读与可回写
func cleanRequiredKeywordList(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		alts := splitKeywordAlternatives(item)
		if len(alts) == 0 {
			continue
		}
		v := strings.Join(alts, "|")
		key := normalizeMatchText(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

// cleanKeywordList 按「、,，;；|｜/」与空白拆分并去重
func cleanKeywordList(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		for _, kw := range splitReqTokens(item) {
			for _, part := range strings.Fields(kw) {
				if seen[part] {
					continue
				}
				seen[part] = true
				out = append(out, part)
			}
		}
	}
	return out
}

// qualityRulesJSON 把规则序列化为库内 JSON 文本（用于剧本编排保存）
func qualityRulesJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	rules := parseQualityRules(string(b))
	out, err := json.Marshal(rules)
	if err != nil {
		return ""
	}
	return string(out)
}

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

// ========== 处置质量：文字质量门槛 + 关键词分级 + 证据要求 ==========

// submissionQuality 一次处置提交的文本证据（实战版结算用；基础版无提交文本，HasSubmission=false）
type submissionQuality struct {
	Late    bool
	RecvBad bool
	// RecvRatio 接收人扣分比例：主送缺失 0.30 / 仅抄送缺失 0.10 / 不扣 0。
	// >0 时优先使用，RecvBad 仅作为兜底与状态标签。
	RecvRatio float64
	// RecvNote 接收人扣分文案（主送缺失 / 抄送缺失），为空时回退默认文案。
	RecvNote      string
	HasSubmission bool
	// EvidenceArchived / EvidenceReqCount 证据表已归档数与本节点取证要求数（P0-04 主口径）
	EvidenceArchived int
	EvidenceReqCount int
	// Text 仅处置文本：处置措施、处置结果、发现情况、证据说明。
	// 表单填写值不参与关键词/证据匹配，避免「把关键词塞进表单」白拿分。
	Text string
	// Measure / Result 单独保留，用于最小长度、要素与敷衍判定
	Measure string
	Result  string
	Found   string
	// EvidenceText 单独的「证据材料说明」字段（判断是否提交证据）
	EvidenceText string
	// Keywords 节点旧版关键词（quality_rules 未配置必含项时的兼容口径）
	Keywords    []string
	EvidenceReq string
	// Rules 节点级文字质量规则（缺省为默认口径）
	Rules QualityRules
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

// keywordHits 统计命中的关键词（单字关键词忽略，与旧口径一致；比对前统一归一化）
func keywordHits(keywords []string, text string) (int, []string) {
	hits := 0
	var hitWords []string
	seen := map[string]bool{}
	norm := normalizeMatchText(text)
	for _, kw := range splitReqTokens(strings.Join(keywords, "、")) {
		key := normalizeMatchText(kw)
		if len([]rune(key)) < 2 || seen[key] {
			continue
		}
		seen[key] = true
		if strings.Contains(norm, key) {
			hits++
			hitWords = append(hitWords, kw)
		}
	}
	return hits, hitWords
}

// missingKeywords 返回未命中的关键词（保持配置顺序；比对前统一归一化）
func missingKeywords(keywords []string, text string) []string {
	var missing []string
	seen := map[string]bool{}
	norm := normalizeMatchText(text)
	for _, kw := range splitReqTokens(strings.Join(keywords, "、")) {
		key := normalizeMatchText(kw)
		if len([]rune(key)) < 2 || seen[key] {
			continue
		}
		seen[key] = true
		if !strings.Contains(norm, key) {
			missing = append(missing, kw)
		}
	}
	return missing
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
// 空数组（"[]"）返回空切片，表示本节点无取证要求。
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

// evidenceRequirementHit evidence_req 的任一材料名称在处置文本中出现即视为命中（归一化后比对）
func evidenceRequirementHit(req string, text string) bool {
	norm := normalizeMatchText(text)
	for _, token := range evidenceReqTokens(req) {
		key := normalizeMatchText(token)
		if len([]rune(key)) < 2 {
			continue
		}
		if strings.Contains(norm, key) {
			return true
		}
	}
	return false
}

// evidenceTextHitCount 返回处置文本中命中的材料名称数量（归一化后比对，单字忽略）。
func evidenceTextHitCount(req string, text string) int {
	norm := normalizeMatchText(text)
	hits := 0
	seen := map[string]bool{}
	for _, token := range evidenceReqTokens(req) {
		key := normalizeMatchText(token)
		if len([]rune(key)) < 2 || seen[key] {
			continue
		}
		seen[key] = true
		if strings.Contains(norm, key) {
			hits++
		}
	}
	return hits
}

// ========== 文字质量判定 ==========

// textElementRules 「对象 + 动作 + 时间 + 结论」四类要素的判定规则
var textElementRules = map[string]*regexp.Regexp{
	"对象": regexp.MustCompile(`[0-9A-Za-z]|终端|主机|服务器|账号|账户|文件|目录|端口|IP|网段|系统|数据库|进程|防火墙|交换机|设备|邮箱|用户|员工|科室|打印机|摄像头|业务|回执|舆情|口径|患者|指挥部|厂商|日志|策略|证据|样本|邮件|域名|工单|堡垒机|域控|跳板机|接口|权限|基线|证书|流量`),
	"动作": regexp.MustCompile(`隔离|断网|封堵|封禁|删除|清除|重置|恢复|备份|上报|通知|核实|排查|检查|关闭|开启|修改|加固|阻断|拦截|卸载|扫描|切换|迁移|停用|处置|锁定|冻结|升级|补丁|查杀|修复|替换|重启|报告|汇报|报送|批准|审批|签发|宣布|研判|启动|接收|告知|发布|总结|复盘|归档|会商|指导|通报|回应|评估|报请|报批|已报`),
	"时间": regexp.MustCompile(`\d{4}[-/年]\d{1,2}[-/月]\d{1,2}日?|\d{1,2}[:：]\d{2}|\d{1,2}[点时]|\d{1,2}月\d{1,2}日|\d+(?:\.\d+)?(?:个)?(?:小时|分钟|天|日)(?:内|以内|之内)?|当日|当天|次日|翌日|立即|即时|第一时间|截至|随后|已于|至今`),
	"结论": regexp.MustCompile(`已完成|完成|恢复|确认|验证|无异常|已消除|已阻断|成功|结束|闭环|正常|已解决|收到|登记|对齐|处置完毕|到位|办结|解除|通过|告知`),
}

// textQualityEvaluation 文字质量判定结果
type textQualityEvaluation struct {
	Invalid bool
	Filler  bool
	Reasons []string
}

// clipRunes 按字符截断，避免中文被截成半个字（clipStr 按字节）
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// stripFillerNoise 去掉空白与标点，用于敷衍短文本判定
func stripFillerNoise(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isFillerText 去掉标点后不超过长度上限，且完全由敷衍词拼成 → 视为未实质填写
func isFillerText(text string, rules QualityRules) bool {
	norm := stripFillerNoise(normalizeMatchText(text))
	if norm == "" {
		return true
	}
	if runeLen := len([]rune(norm)); runeLen > rules.FillerMaxLen {
		return false
	}
	words := make([]string, 0, len(rules.FillerWords))
	for _, w := range rules.FillerWords {
		if n := stripFillerNoise(normalizeMatchText(w)); n != "" {
			words = append(words, n)
		}
	}
	// 长词优先消除，避免「无异常」被拆成「无」+「异常」
	sort.Slice(words, func(i, j int) bool { return len(words[i]) > len(words[j]) })
	rest := strings.ToLower(norm)
	for _, w := range words {
		rest = strings.ReplaceAll(rest, w, "")
	}
	return strings.TrimSpace(rest) == ""
}

// evaluateTextQuality 按节点规则判定处置文本是否有效（敷衍 / 过短 / 要素缺失）
func evaluateTextQuality(q submissionQuality, rules QualityRules) textQualityEvaluation {
	var out textQualityEvaluation
	measure := strings.TrimSpace(q.Measure)
	result := strings.TrimSpace(q.Result)
	combined := measure + "\n" + result
	if isFillerText(combined, rules) {
		out.Invalid = true
		out.Filler = true
		out.Reasons = append(out.Reasons, "仅填写「"+clipRunes(strings.TrimSpace(measure+" "+result), 20)+"」等敷衍内容，视为未实质填写")
		return out
	}
	if rules.MinMeasureLen > 0 {
		if n := len([]rune(measure)); n < rules.MinMeasureLen {
			out.Reasons = append(out.Reasons, fmt.Sprintf("处置措施仅 %d 字（下限 %d 字）", n, rules.MinMeasureLen))
		}
	}
	if rules.MinResultLen > 0 {
		if n := len([]rune(result)); n < rules.MinResultLen {
			out.Reasons = append(out.Reasons, fmt.Sprintf("处置结果仅 %d 字（下限 %d 字）", n, rules.MinResultLen))
		}
	}
	if len(rules.RequiredElements) > 0 {
		elementsText := normalizeMatchText(combined + "\n" + strings.TrimSpace(q.Found))
		var missing []string
		for _, name := range rules.RequiredElements {
			re := textElementRules[name]
			if re == nil || !re.MatchString(elementsText) {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			out.Reasons = append(out.Reasons, "缺少要素："+strings.Join(missing, "、"))
		}
	}
	out.Invalid = len(out.Reasons) > 0
	return out
}

// evaluateHardGate 提交硬门槛（只决定是否 400）：只保留「不写就说明没做这件事」的判定。
//   - 未填处置措施；
//   - 措施+结果有效字数低于 HardGateMinTextRunes（仅正确节点）；
//   - 敷衍短文本（整段只由敷衍词拼成）。
//
// 混淆节点（flow_type=distractor）本就只扣分、不参与质量评分，其选项原文常常只有
// 7–9 个有效字；长度门槛对它们只保留「未填措施 / 纯敷衍词」两条，避免选错却提交不上。
// 节点配置的最少字数、必备要素等细节一律不在这里拦，转为 evaluateTextQuality 的扣分项。
func evaluateHardGate(q submissionQuality, rules QualityRules, flowType string) []string {
	measure := strings.TrimSpace(q.Measure)
	result := strings.TrimSpace(q.Result)
	combined := measure + "\n" + result
	if isFillerText(combined, rules) {
		return []string{"仅填写「" + clipRunes(strings.TrimSpace(measure+" "+result), 20) + "」等敷衍内容，视为未实质填写"}
	}
	if measure == "" {
		return []string{"未填写处置措施"}
	}
	if strings.EqualFold(strings.TrimSpace(flowType), "distractor") {
		return nil
	}
	if n := len([]rune(stripFillerNoise(normalizeMatchText(combined)))); n < HardGateMinTextRunes {
		return []string{fmt.Sprintf("处置文本有效字数仅 %d 字（最少 %d 字），视为未实质填写", n, HardGateMinTextRunes)}
	}
	return nil
}

// effectiveQualityRules 保证调用方未显式传规则时也拿到默认口径
func effectiveQualityRules(q submissionQuality) QualityRules {
	if q.Rules.KeywordPenaltyRatio == 0 && q.Rules.TextQualityRatio == 0 && q.Rules.MinMeasureLen == 0 &&
		q.Rules.MinResultLen == 0 && q.Rules.FillerMaxLen == 0 && q.Rules.RequiredElements == nil &&
		q.Rules.FillerWords == nil {
		return defaultQualityRules()
	}
	return normalizeQualityRules(q.Rules)
}

func ratioText(p float64) string {
	return fmt.Sprintf("%.0f%%", p*100)
}

// bonusMatchText 加分词的匹配文本：只取处置叙述（措施 / 结果 / 发现），不含证据说明。
// 证据说明只用于 evidence_req 命中判定，避免「证据里写回执」把缺失的必含项抵扣掉。
func bonusMatchText(q submissionQuality) string {
	if s := strings.TrimSpace(q.Measure + q.Result + q.Found); s != "" {
		return strings.Join([]string{q.Measure, q.Result, q.Found}, "\n")
	}
	text := q.Text
	if ev := strings.TrimSpace(q.EvidenceText); ev != "" {
		text = strings.Replace(text, ev, "", 1)
	}
	return text
}

// keywordDeductionPart 关键词扣分项：
//   - 配置了必含关键词：缺一项按比例扣，加分词每命中 1 个抵扣 1 项，但最多抵扣到
//     仍保留 1 项缺失（缺失扣分不得被加分归零）；
//   - 未配置（老节点）：保持旧口径 keywords + 命中≥2，未达标扣满分值比例。
func keywordDeductionPart(nodeScore int, q submissionQuality, rules QualityRules, text, bonusText string) *scoreDeductPart {
	required := rules.RequiredKeywords
	if len(required) == 0 {
		legacy := splitReqTokens(strings.Join(q.Keywords, "、"))
		if len(legacy) == 0 {
			return nil
		}
		threshold := KeywordMinHits
		if len(legacy) < threshold {
			threshold = len(legacy)
		}
		hits, _ := keywordHits(legacy, text)
		if hits >= threshold {
			return nil
		}
		point := ratioDeduct(nodeScore, rules.KeywordPenaltyRatio)
		if point <= 0 {
			return nil
		}
		missing := missingKeywords(legacy, text)
		return &scoreDeductPart{
			Type: "处置关键词命中不足（命中 " + strconv.Itoa(hits) + "/" + strconv.Itoa(threshold) +
				" 个；缺失：" + strings.Join(missing, "、") + "；扣本节点" + ratioText(rules.KeywordPenaltyRatio) + "）",
			Point: point,
		}
	}
	groups := requiredKeywordGroups(required)
	if len(groups) == 0 {
		return nil
	}
	missing := missingRequiredGroups(groups, text)
	if len(missing) == 0 {
		return nil
	}
	bonusHits := bonusHitCount(rules.BonusKeywords, bonusText)
	// 加分抵扣上限：最多抵扣到仍保留 1 项缺失，缺失必含项的扣分不得被加分归零。
	if bonusHits > len(missing)-1 {
		bonusHits = len(missing) - 1
	}
	if bonusHits < 0 {
		bonusHits = 0
	}
	effective := len(missing) - bonusHits
	if effective <= 0 {
		return nil
	}
	if effective > len(groups) {
		effective = len(groups)
	}
	ratio := rules.KeywordPenaltyRatio * float64(effective) / float64(len(groups))
	point := ratioDeduct(nodeScore, ratio)
	if point <= 0 {
		return nil
	}
	reason := "处置关键词必含项缺失：" + strings.Join(missing, "、")
	if bonusHits > 0 {
		reason += "（加分词命中 " + strconv.Itoa(bonusHits) + " 个已抵扣）"
	}
	return &scoreDeductPart{
		Type:  reason + "（缺 " + strconv.Itoa(effective) + "/" + strconv.Itoa(len(groups)) + " 项，扣本节点" + ratioText(ratio) + "）",
		Point: point,
	}
}

// correctNodeDeduction 正确节点（已提交）的统一扣分口径，Score() 与提交写库共用。
// 超时 20% / 接收人 30% / 关键词分级 30% / 文字质量 20% / 证据未提交或未命中 20%，
// 单节点合计上限 80%（NodeMaxPenaltyRatio）。
func correctNodeDeduction(nodeScore int, q submissionQuality) (float64, []scoreDeductPart) {
	var parts []scoreDeductPart
	rules := effectiveQualityRules(q)
	if q.Late {
		parts = append(parts, scoreDeductPart{Type: "超时完成（扣本节点20%）", Point: ratioDeduct(nodeScore, TimeoutPenaltyRatio)})
	}
	if q.RecvBad || q.RecvRatio > 0 {
		ratio := q.RecvRatio
		if ratio <= 0 {
			ratio = ReceiverErrPenaltyRatio
		}
		reason := q.RecvNote
		if strings.TrimSpace(reason) == "" {
			reason = "接收人选择错误"
		}
		parts = append(parts, scoreDeductPart{
			Type:  reason + "（扣本节点" + ratioText(ratio) + "）",
			Point: ratioDeduct(nodeScore, ratio),
		})
	}
	if q.HasSubmission {
		quality := evaluateTextQuality(q, rules)
		keywordText := q.Text
		bonusText := bonusMatchText(q)
		if quality.Filler {
			// 敷衍短文本按未填写处理：关键词与加分词均视为 0 命中
			keywordText = ""
			bonusText = ""
		}
		if p := keywordDeductionPart(nodeScore, q, rules, keywordText, bonusText); p != nil {
			parts = append(parts, *p)
		}
		if quality.Invalid && rules.TextQualityRatio > 0 {
			parts = append(parts, scoreDeductPart{
				Type:  "处置文本质量不达标（" + strings.Join(quality.Reasons, "；") + "，扣本节点" + ratioText(rules.TextQualityRatio) + "）",
				Point: ratioDeduct(nodeScore, rules.TextQualityRatio),
			})
		}
	}
	// 证据材料：以 evidence 表已归档为主口径，文本命中降为辅助（最多抵 50%）。
	// 不依赖 HasSubmission —— 基础版没有处置文本，也要按归档情况评分。
	if q.EvidenceReqCount > 0 {
		archivedRate := float64(q.EvidenceArchived) / float64(q.EvidenceReqCount)
		if archivedRate > 1 {
			archivedRate = 1
		}
		textCredit := 0.0
		if q.HasSubmission && strings.TrimSpace(q.EvidenceText) != "" {
			if hits := evidenceTextHitCount(q.EvidenceReq, q.Text); hits > 0 {
				textCredit = 0.5 * float64(hits) / float64(q.EvidenceReqCount)
				if textCredit > 0.5 {
					textCredit = 0.5
				}
			}
		}
		credit := math.Max(archivedRate, textCredit)
		if credit < 1 {
			parts = append(parts, scoreDeductPart{
				Type: "证据材料未提交或未归档（已归档 " + strconv.Itoa(q.EvidenceArchived) + "/" +
					strconv.Itoa(q.EvidenceReqCount) + "，扣本节点" + ratioText(EvidencePenaltyRatio*(1-credit)) + "）",
				Point: ratioDeduct(nodeScore, EvidencePenaltyRatio*(1-credit)),
			})
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
