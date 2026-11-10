package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// 误伤用例实测：夹具由 VM 临时库 drill_verify_tmp 导出（真实节点 + 真实规则 + 正常作答）。
// 只有显式设置 DRILL_QUALITY_FIXTURE 时才执行，普通 go test 不受影响。
//
//	DRILL_QUALITY_FIXTURE=<path> go test ./internal/handler -run TestQualityFixture -v
type fixtureCase struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	FlowType     string   `json:"flowType"`
	Score        int      `json:"score"`
	Keywords     []string `json:"keywords"`
	EvidenceReq  string   `json:"evidenceReq"`
	Rules        string   `json:"rules"`
	PatchedRules string   `json:"patchedRules"`
	Measure      string   `json:"measure"`
	Result       string   `json:"result"`
	Found        string   `json:"found"`
	Evidence     string   `json:"evidence"`
	Note         string   `json:"note"`
}

func TestQualityFixture(t *testing.T) {
	path := os.Getenv("DRILL_QUALITY_FIXTURE")
	if path == "" {
		t.Skip("未设置 DRILL_QUALITY_FIXTURE，跳过 VM 夹具实测")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取夹具失败: %v", err)
	}
	var cases []fixtureCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("解析夹具失败: %v", err)
	}

	fmt.Println("节点 | 改前门槛 | 改后门槛 | 改后扣分 | 得分")
	fmt.Println(strings.Repeat("-", 110))
	for _, c := range cases {
		rules := parseQualityRules(c.Rules)
		q := submissionQuality{
			HasSubmission: true,
			Measure:       c.Measure,
			Result:        c.Result,
			Found:         c.Found,
			EvidenceText:  c.Evidence,
			Keywords:      c.Keywords,
			EvidenceReq:   c.EvidenceReq,
			Rules:         rules,
			Text:          strings.Join([]string{c.Measure, c.Result, c.Found, c.Evidence}, "\n"),
		}
		old := evaluateTextQuality(q, rules)
		hard := evaluateHardGate(q, rules, c.FlowType)
		oldState, newState := "通过", "通过"
		if old.Invalid {
			oldState = "400（" + strings.Join(old.Reasons, "；") + "）"
		}
		if len(hard) > 0 {
			newState = "400（" + strings.Join(hard, "；") + "）"
		}
		deduct, parts := correctNodeDeduction(c.Score, q)
		missing := missingRequiredGroups(requiredKeywordGroups(rules.RequiredKeywords), q.Text)
		_, bonusHit := keywordHits(rules.BonusKeywords, bonusMatchText(q))
		fmt.Printf("%s [%s/%d] %s\n  改前: %s\n  改后: %s\n  必含缺项: %v | 加分命中: %v\n  扣分: -%.1f",
			c.ID, c.FlowType, c.Score, c.Note, oldState, newState, missing, bonusHit, deduct)
		for _, p := range parts {
			fmt.Printf("\n        · -%.1f %s", p.Point, p.Type)
		}
		fmt.Printf("\n  得分: %.1f / %d\n\n", float64(c.Score)-deduct, c.Score)

		// 门槛断言：正常作答改前被拦、改后放行
		if c.ID != "S01-4" {
			if !old.Invalid {
				t.Errorf("%s: 预期改前被 400，实际通过（用例需调整）", c.ID)
			}
			if len(hard) > 0 {
				t.Errorf("%s: 改后仍被拦：%v", c.ID, hard)
			}
		}
	}

	// 同义替换修正：S03-9 规则改为「回切|回退|切回」后不再扣关键词分
	for _, c := range cases {
		if c.ID != "S03-9" || c.PatchedRules == "" {
			continue
		}
		patched := parseQualityRules(c.PatchedRules)
		q := submissionQuality{
			HasSubmission: true, Measure: c.Measure, Result: c.Result, Found: c.Found,
			EvidenceText: c.Evidence, Keywords: c.Keywords, EvidenceReq: c.EvidenceReq,
			Rules: patched, Text: strings.Join([]string{c.Measure, c.Result, c.Found, c.Evidence}, "\n"),
		}
		_, parts := correctNodeDeduction(c.Score, q)
		fmt.Printf("S03-9 修正规则后扣分项: %+v\n", parts)
		for _, p := range parts {
			if strings.Contains(p.Type, "处置关键词必含项缺失") {
				t.Errorf("S03-9 修正规则后仍扣关键词分：%s", p.Type)
			}
		}
	}
}
