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
