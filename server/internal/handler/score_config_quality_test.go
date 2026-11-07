package handler

import (
	"fmt"
	"strings"
	"testing"
)

// 本文件是纯函数对照用例：不连数据库、不写库，只对比「改造前口径」与
// 「当前 correctNodeDeduction」在同一份真实节点配置 + 同一份提交文本上的得分。
// 节点配置取自现网库 script_nodes：
//   S01-2  score=7  keywords=终端清单,网段,仍在加密,受影响主机
//          evidence_req=[{受影响终端清单},{共享目录访问告警截图}]
//   S07-15 score=6  keywords=复盘,总结,整改   evidence_req=[]
//   S08-16 score=6  keywords=结束,复盘归档,响应终止 evidence_req=[]

type qualityCase struct {
	name        string
	nodeScore   int
	keywords    []string
	evidenceReq string
	rulesRaw    string
	measure     string
	result      string
	found       string
	evidence    string
	formValues  string
}

// legacyDeduct 复刻改造前的扣分口径（commit acf3a69 版本，含两个已知缺陷）：
// 关键词在「处置文本 + 表单值」里命中 ≥2 即通过；evidence_req 非空串就校验，
// 包含 "[]"；没有文字质量判定。
func (c qualityCase) legacyDeduct() float64 {
	var parts []scoreDeductPart
	text := strings.Join([]string{c.measure, c.result, c.found, c.evidence, c.formValues}, "\n")
	kwTotal := len(splitReqTokens(strings.Join(c.keywords, "、")))
	if kwTotal > 0 && len(text) > 0 {
		threshold := KeywordMinHits
		if kwTotal < threshold {
			threshold = kwTotal
		}
		hits := 0
		for _, kw := range splitReqTokens(strings.Join(c.keywords, "、")) {
			if len([]rune(kw)) < 2 {
				continue
			}
			if strings.Contains(text, kw) {
				hits++
			}
		}
		if hits < threshold {
			parts = append(parts, scoreDeductPart{Type: "old-keyword", Point: ratioDeduct(c.nodeScore, KeywordHitPenaltyRatio)})
		}
	}
	if strings.TrimSpace(c.evidenceReq) != "" {
		missing := strings.TrimSpace(c.evidence) == ""
		if !missing && !evidenceRequirementHit(c.evidenceReq, text) {
			missing = true
		}
		if missing {
			parts = append(parts, scoreDeductPart{Type: "old-evidence", Point: ratioDeduct(c.nodeScore, EvidencePenaltyRatio)})
		}
	}
	total := 0.0
	for _, p := range capNodeParts(parts, c.nodeScore) {
		total += p.Point
	}
	return mathRound1(total)
}

// newDeduct 走改造后的 correctNodeDeduction：关键词只匹配处置文本（不含表单值）
func (c qualityCase) newDeduct() (float64, []scoreDeductPart) {
	q := submissionQuality{
		HasSubmission: true,
		Measure:       c.measure,
		Result:        c.result,
		Found:         c.found,
		EvidenceText:  c.evidence,
		Keywords:      c.keywords,
		EvidenceReq:   c.evidenceReq,
		Rules:         parseQualityRules(c.rulesRaw),
		Text:          strings.Join([]string{c.measure, c.result, c.found, c.evidence}, "\n"),
	}
	return correctNodeDeduction(c.nodeScore, q)
}

func mathRound1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func TestQualityRulesContrast(t *testing.T) {
	longIrrelevant := strings.Repeat("本次演练为内部模拟场景，相关科室按流程完成值班交接与日常巡检记录，未发现异常情况。", 3)

	cases := []qualityCase{
		{
			name: "A 敷衍短文本", nodeScore: 7,
			keywords:    []string{"终端清单", "网段", "仍在加密", "受影响主机"},
			evidenceReq: `[{"type":"处置记录","name":"受影响终端清单"},{"type":"附件截图","name":"共享目录访问告警截图"}]`,
			measure:     "已处理，无异常",
			result:      "无",
			evidence:    "已上传受影响终端清单截图",
		},
		{
			name: "B 超长无关文本", nodeScore: 7,
			keywords:    []string{"终端清单", "网段", "仍在加密", "受影响主机"},
			evidenceReq: `[{"type":"处置记录","name":"受影响终端清单"},{"type":"附件截图","name":"共享目录访问告警截图"}]`,
			measure:     longIrrelevant,
			result:      "按要求完成日常巡检记录",
			evidence:    "见受影响终端清单附件",
		},
		{
			name: "C 详实命中文本", nodeScore: 7,
			keywords:    []string{"终端清单", "网段", "仍在加密", "受影响主机"},
			evidenceReq: `[{"type":"处置记录","name":"受影响终端清单"},{"type":"附件截图","name":"共享目录访问告警截图"}]`,
			measure:     "对受影响主机所在网段执行隔离并核实加密告警，确认受影响终端清单无新增",
			result:      "受影响主机已隔离，业务恢复正常",
			evidence:    "已归档受影响终端清单与共享目录访问告警截图",
		},
		{
			name: "D 关键词塞进表单", nodeScore: 7,
			keywords:    []string{"终端清单", "网段", "仍在加密", "受影响主机"},
			evidenceReq: `[{"type":"处置记录","name":"受影响终端清单"},{"type":"附件截图","name":"共享目录访问告警截图"}]`,
			measure:     "对受影响服务器执行隔离并核实告警，确认业务已恢复",
			result:      "业务已恢复正常运行，监控无异常",
			evidence:    "已上传共享目录访问告警截图",
			formValues:  "终端清单 网段 仍在加密 受影响主机",
		},
		{
			name: "E evidence_req=[] 无取证要求", nodeScore: 6,
			keywords:    []string{"复盘", "总结", "整改"},
			evidenceReq: `[]`,
			measure:     "完成本次终端安全事件应急处置复盘并总结问题，输出整改措施清单",
			result:      "已形成复盘报告并提交终端整改计划",
			evidence:    "",
		},
		{
			name: "F 必含关键词缺 2/4（比例扣分）", nodeScore: 7,
			keywords:    []string{"终端清单", "网段", "仍在加密", "受影响主机"},
			evidenceReq: `[{"type":"处置记录","name":"受影响终端清单"},{"type":"附件截图","name":"共享目录访问告警截图"}]`,
			rulesRaw:    `{"requiredKeywords":["终端清单","网段","仍在加密","受影响主机"],"bonusKeywords":[]}`,
			measure:     "对受影响主机所在网段执行隔离并核实告警，确认业务恢复正常",
			result:      "受影响主机已隔离，业务恢复正常",
			evidence:    "已上传共享目录访问告警截图",
		},
	}

	fmt.Println("节点真实配置 + 同一提交文本，改造前后扣分对照（扣分越少越好）")
	fmt.Println(strings.Repeat("-", 96))
	for _, c := range cases {
		oldDeduct := c.legacyDeduct()
		newDeduct, parts := c.newDeduct()
		newDeduct = mathRound1(newDeduct)
		oldScore := float64(c.nodeScore) - oldDeduct
		newScore := float64(c.nodeScore) - newDeduct
		fmt.Printf("%-28s 节点分 %d | 改造前 -%.1f（得 %.1f） | 改造后 -%.1f（得 %.1f）\n",
			c.name, c.nodeScore, oldDeduct, oldScore, newDeduct, newScore)
		for _, p := range parts {
			fmt.Printf("    · -%.1f %s\n", p.Point, p.Type)
		}
	}

	// 断言关键结论，防止后续重构回退
	assertDeduct := func(c qualityCase, wantNew float64) {
		t.Helper()
		got, _ := c.newDeduct()
		if mathRound1(got) != wantNew {
			t.Fatalf("%s: 改造后扣分 = %.1f，期望 %.1f", c.name, mathRound1(got), wantNew)
		}
	}
	assertDeduct(cases[0], 3.5) // 敷衍：关键词 2.1 + 文字质量 1.4
	assertDeduct(cases[1], 3.5) // 超长无关：关键词 2.1 + 文字质量 1.4
	assertDeduct(cases[2], 0)   // 详实命中：不扣
	assertDeduct(cases[3], 2.1) // 塞表单：关键词不再命中（7 × 30%）
	assertDeduct(cases[4], 0)   // evidence_req=[]：不再扣 20%
	assertDeduct(cases[5], 1.1) // 必含缺 2/4：30% × 2/4 = 15%

	// 缺词必须落到扣分理由里，而不是只报个数
	_, parts := cases[5].newDeduct()
	if len(parts) == 0 || !strings.Contains(parts[0].Type, "终端清单") || !strings.Contains(parts[0].Type, "仍在加密") {
		t.Fatalf("必含关键词扣分理由未列出缺失词：%+v", parts)
	}
}

// ========== 加分词抵扣漏洞回归（证据不参与加分 + 抵扣不得归零） ==========

// bonusRules 复刻 S01-8B 类配置：3 组必含 + 4 个加分词，关键词扣分比例 25%。
const bonusRules = `{"requiredKeywords":["向网信办报送|向市网信办报送|报送市网信办","事件基本情况|基本情况|影响范围","已采取措施|处置措施|应对措施"],"bonusKeywords":["1小时","一小时内","回执","研判"],"minMeasureLen":15,"minResultLen":10,"textQualityRatio":0.2,"keywordPenaltyRatio":0.25}`

// bonusCase 一个提交样例：old 按改造前口径（证据参与加分、抵扣无上限）只算关键词扣分。
type bonusCase struct {
	name     string
	measure  string
	result   string
	found    string
	evidence string
	flowType string
}

func (c bonusCase) quality() submissionQuality {
	return submissionQuality{
		HasSubmission: true,
		Measure:       c.measure,
		Result:        c.result,
		Found:         c.found,
		EvidenceText:  c.evidence,
		Rules:         parseQualityRules(bonusRules),
		Text:          strings.Join([]string{c.measure, c.result, c.found, c.evidence}, "\n"),
	}
}

// legacyKeywordDeduct 改造前关键词扣分：加分词在含证据的整段文本上命中，无抵扣上限。
func legacyKeywordDeduct(nodeScore int, q submissionQuality, rules QualityRules) float64 {
	groups := requiredKeywordGroups(rules.RequiredKeywords)
	missing := missingRequiredGroups(groups, q.Text)
	if len(missing) == 0 {
		return 0
	}
	effective := len(missing) - bonusHitCount(rules.BonusKeywords, q.Text)
	if effective <= 0 {
		return 0
	}
	return ratioDeduct(nodeScore, rules.KeywordPenaltyRatio*float64(effective)/float64(len(groups)))
}

func TestBonusCannotOffsetMissingRequired(t *testing.T) {
	const nodeScore = 6 // 6 × 25% / 3 组 = 0.5 分/组
	cases := []bonusCase{
		{
			name:     "完整合规（证据含回执）",
			measure:  "于11月7日10时向市网信办报送事件基本情况与影响范围，并已采取措施隔离受影响主机",
			result:   "已确认业务恢复正常运行",
			evidence: "回执与上报表已归档",
		},
		{
			name:     "缺报送对象但证据写回执",
			measure:  "于11月7日10时完成事件基本情况与影响范围的核实，并已采取措施隔离受影响主机",
			result:   "已确认业务恢复正常运行",
			evidence: "已取得市网信办回执并归档",
		},
		{
			name:     "缺报送对象且处置文本命中 1 个加分词",
			measure:  "于11月7日10时完成事件基本情况与影响范围的核实，并已采取措施隔离受影响主机，1小时内向指挥中心报告",
			result:   "已确认业务恢复正常运行",
			evidence: "无",
		},
		{
			name:     "缺报送对象且处置文本命中 2 个加分词",
			measure:  "于11月7日10时完成事件基本情况与影响范围的核实，并已采取措施隔离受影响主机，1小时内向指挥中心报告并完成研判",
			result:   "已确认业务恢复正常运行",
			evidence: "无",
		},
		{
			name:     "缺 2 组且命中 2 个加分词",
			measure:  "于11月7日10时完成事件基本情况与影响范围的核实，1小时内向指挥中心报告并完成研判",
			result:   "已确认业务恢复正常运行",
			evidence: "无",
		},
		{
			name:     "纯套话",
			measure:  "已处理",
			result:   "无",
			evidence: "回执已归档",
		},
		{
			name:     "distractor 短文本",
			measure:  "隔离主机",
			result:   "已恢复",
			evidence: "已归档",
			flowType: "distractor",
		},
	}

	fmt.Println("加分抵扣漏洞：改造前 / 改造后扣分对照（节点 6 分，3 组必含，关键词 25%）")
	fmt.Println(strings.Repeat("-", 96))
	got := make([]float64, len(cases))
	for i, c := range cases {
		q := c.quality()
		rules := effectiveQualityRules(q)
		oldPart := legacyKeywordDeduct(nodeScore, q, rules)
		newTotal, parts := correctNodeDeduction(nodeScore, q)
		got[i] = newTotal
		fmt.Printf("%-32s 改造前关键词 -%.1f | 改造后合计 -%.1f\n", c.name, oldPart, newTotal)
		for _, p := range parts {
			fmt.Printf("    · -%.1f %s\n", p.Point, p.Type)
		}
	}

	// 完整合规：必含项齐全，证据里的「回执」不产生加分，也不应扣分
	if got[0] != 0 {
		t.Fatalf("完整合规应 0 扣分，实际 %.1f", got[0])
	}
	// 缺 1 组：证据里的加分词不再抵扣，必须扣 1 组（0.5 分）
	if got[1] != 0.5 {
		t.Fatalf("缺报送对象且证据写回执：应扣 0.5，实际 %.1f", got[1])
	}
	// 缺 1 组 + 处置文本命中 1/2 个加分词：抵扣最多归零到保留 1 项，仍扣 0.5
	if got[2] != 0.5 || got[3] != 0.5 {
		t.Fatalf("缺 1 组被处置文本加分抹平：命中 1 个扣 %.1f，命中 2 个扣 %.1f，期望均 0.5", got[2], got[3])
	}
	// 缺 2 组：最多抵扣 1 组，仍保留 1 组扣分（0.5），不得归零
	if got[4] != 0.5 {
		t.Fatalf("缺 2 组且命中 2 个加分词：应扣 0.5（保留 1 组），实际 %.1f", got[4])
	}
	// 纯套话：硬门槛拦截，扣分口径不变（关键词 3 组全缺 1.5 + 文字质量 1.2）
	if reasons := evaluateHardGate(cases[5].quality(), effectiveQualityRules(cases[5].quality()), cases[5].flowType); len(reasons) == 0 {
		t.Fatal("纯套话应被硬门槛拦截")
	}
	if got[5] != 2.7 {
		t.Fatalf("纯套话扣分应为 2.7，实际 %.1f", got[5])
	}
	// distractor 短文本：硬门槛按原口径放行，扣分口径与改造前一致
	q6 := cases[6].quality()
	if reasons := evaluateHardGate(q6, effectiveQualityRules(q6), cases[6].flowType); len(reasons) != 0 {
		t.Fatalf("distractor 短文本不应被硬门槛拦截：%v", reasons)
	}
	if got[6] != 2.7 {
		t.Fatalf("distractor 短文本扣分应为 2.7，实际 %.1f", got[6])
	}
}
