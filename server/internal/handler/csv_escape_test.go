package handler

import "testing"

// TestCsvSafeCell 回归 P2-4：以公式字符开头的单元格必须前置单引号。
func TestCsvSafeCell(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"=1+1", "'=1+1"},
		{"=cmd|' /c calc'!A0", "'=cmd|' /c calc'!A0"},
		{"+SUM(A1)", "'+SUM(A1)"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tpayload", "'\tpayload"},
		{"\rpayload", "'\rpayload"},
		{"正常内容", "正常内容"},
		{"127.0.0.1", "127.0.0.1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := csvSafeCell(c.in); got != c.want {
			t.Errorf("csvSafeCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
