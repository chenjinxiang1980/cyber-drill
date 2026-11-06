package handler

import (
	"strings"
	"testing"
)

// 匹配层容错用例：归一化（全半角 / 空白 / 大小写 / 数字体系）+ 必含词同义词组 + 提交硬门槛。

func TestNormalizeMatchText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ｕ盘", "u盘"},
		{"U 盘", "u盘"},
		{"ＲＰＯ　值", "rpo值"},
		{"RPO 值", "rpo值"},
		{"Ⅱ级", "2级"},
		{"II级", "2级"},
		{"2 级", "2级"},
		{"三级", "3级"},
		{"Ⅲ 类", "3类"},
		{"较大", "较大"},
		{" 已 处 置 完 毕 ", "已处置完毕"},
	}
	for _, c := range cases {
		if got := normalizeMatchText(c.in); got != c.want {
			t.Fatalf("normalizeMatchText(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestRequiredKeywordGroupOr(t *testing.T) {
	rules := parseQualityRules(`{"requiredKeywords":["回切|回退|切回","事件等级","启动"]}`)
	if len(rules.RequiredKeywords) != 3 {
		t.Fatalf("同义词组被拆散：%+v", rules.RequiredKeywords)
	}
	groups := requiredKeywordGroups(rules.RequiredKeywords)
	ok := []string{
		"已完成回切至主链路，事件等级判定为较大，报指挥部启动响应",
		"已完成回退至主链路，事件等级判定为较大，报指挥部启动响应",
		"已完成切回主链路，事件等级判定为较大，报指挥部启动响应",
	}
	for _, text := range ok {
		if missing := missingRequiredGroups(groups, text); len(missing) != 0 {
			t.Fatalf("%q 不应缺项，实际缺失：%v", text, missing)
		}
	}
	missing := missingRequiredGroups(groups, "已完成隔离，事件等级为较大，启动响应")
	if len(missing) != 1 || !strings.Contains(missing[0], "回切") {
		t.Fatalf("应只缺同义词组项，实际：%v", missing)
	}
}

func TestRequiredKeywordBackwardCompatible(t *testing.T) {
	rules := parseQualityRules(`{"requiredKeywords":["隔离","断网"]}`)
	groups := requiredKeywordGroups(rules.RequiredKeywords)
	if len(groups) != 2 {
		t.Fatalf("纯字符串应保持「每项都要命中」的旧语义，实际 %d 组", len(groups))
	}
	if missing := missingRequiredGroups(groups, "仅完成断网"); len(missing) != 1 || missing[0] != "隔离" {
		t.Fatalf("应缺「隔离」，实际：%v", missing)
	}
}

func TestKeywordFullWidthAndSpacing(t *testing.T) {
	rules := parseQualityRules(`{"requiredKeywords":["U盘","RPO值"]}`)
	groups := requiredKeywordGroups(rules.RequiredKeywords)
	text := "已对Ｕ盘执行全盘查杀，RPO 值恢复到 15 分钟以内"
	if missing := missingRequiredGroups(groups, text); len(missing) != 0 {
		t.Fatalf("全半角/空格变体应命中，实际缺失：%v", missing)
	}
}

func TestKeywordRomanNumeralVariant(t *testing.T) {
	groups := requiredKeywordGroups(parseQualityRules(`{"requiredKeywords":["Ⅱ级响应"]}`).RequiredKeywords)
	for _, text := range []string{"已按 2 级响应启动处置", "已按II级响应启动处置", "已按Ⅱ级响应启动处置"} {
		if missing := missingRequiredGroups(groups, text); len(missing) != 0 {
			t.Fatalf("%q 应命中Ⅱ级响应，实际缺失：%v", text, missing)
		}
	}
	reverse := requiredKeywordGroups(parseQualityRules(`{"requiredKeywords":["2级响应"]}`).RequiredKeywords)
	if missing := missingRequiredGroups(reverse, "已按Ⅱ级响应启动处置"); len(missing) != 0 {
		t.Fatalf("规则写阿拉伯数字也应命中罗马数字提交，实际缺失：%v", missing)
	}
}

func TestHardGateOnlyBlocksNoWork(t *testing.T) {
	rules := parseQualityRules(`{"minMeasureLen":15,"minResultLen":10,"requiredElements":["对象","动作","结论"]}`)
	q := submissionQuality{Measure: "对文件服务器执行断网隔离并清除恶意样本", Result: "业务恢复正常"}
	if reasons := evaluateHardGate(q, rules, "correct"); len(reasons) != 0 {
		t.Fatalf("正常作答不应被硬门槛拦截：%v", reasons)
	}
	// 低于节点配置的 15 字，但属于正常作答 → 放行，转扣分
	if reasons := evaluateHardGate(submissionQuality{Measure: "隔离受影响终端主机", Result: "已完成处置"}, rules, "correct"); len(reasons) != 0 {
		t.Fatalf("细节字数不足应转扣分而非 400：%v", reasons)
	}
	blocked := []submissionQuality{
		{Measure: "", Result: ""},
		{Measure: "已处理", Result: "无"},
		{Measure: "已隔离", Result: "完成"},
	}
	for _, c := range blocked {
		if reasons := evaluateHardGate(c, rules, "correct"); len(reasons) == 0 {
			t.Fatalf("%+v 应被硬门槛拦截", c)
		}
	}

	// 混淆节点：长度不拦，只保留「未填措施 / 纯敷衍词」
	if reasons := evaluateHardGate(submissionQuality{Measure: "密码改了就结束"}, rules, "distractor"); len(reasons) != 0 {
		t.Fatalf("混淆节点 7 字选项不应被拦：%v", reasons)
	}
	for _, c := range []submissionQuality{{Measure: ""}, {Measure: "已处理"}} {
		if reasons := evaluateHardGate(c, rules, "distractor"); len(reasons) == 0 {
			t.Fatalf("混淆节点 %+v 应被拦截", c)
		}
	}
}
