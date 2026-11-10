package handler

import "testing"

// 演练进行中不返回等级：只有封存（有评分快照）后才按分值给评级。
func TestEffectiveGradeOngoingReturnsEmpty(t *testing.T) {
	if got := effectiveGrade(false, 95); got != "" {
		t.Fatalf("演练进行中不应返回等级，实际 %q", got)
	}
	if got := effectiveGrade(false, 60); got != "" {
		t.Fatalf("演练进行中低分同样不应返回等级，实际 %q", got)
	}

	cases := []struct {
		score float64
		want  string
	}{
		{95, "优秀"},
		{90, "优秀"},
		{89.9, "良好"},
		{80, "良好"},
		{70, "合格"},
		{69.9, "不合格"},
	}
	for _, c := range cases {
		if got := effectiveGrade(true, c.score); got != c.want {
			t.Fatalf("封存后 %.1f 分应为 %q，实际 %q", c.score, c.want, got)
		}
	}
}

// 手工评分超过该评分项满分时收敛到满分，负分兜底为 0。
func TestClampManualScoreCapsAtMax(t *testing.T) {
	cases := []struct {
		score, max, want float64
	}{
		{120, 100, 100},
		{15, 10, 10},
		{10, 10, 10},
		{8, 10, 8},
		{-3, 10, 0},
		{0, 10, 0},
	}
	for _, c := range cases {
		if got := clampManualScore(c.score, c.max); got != c.want {
			t.Fatalf("clampManualScore(%.1f, %.1f) = %.1f，期望 %.1f", c.score, c.max, got, c.want)
		}
	}
}
