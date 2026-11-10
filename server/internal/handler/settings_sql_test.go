package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 回归用例：settings.key 是 MySQL 保留字，任何 SQL 都必须写成 `key`。
//
// 历史问题：`INSERT INTO settings (key,value) ...` / `SELECT ... WHERE key='active_project'`
// 在 MySQL 8 直接报 1064，而调用处吞掉了错误，导致「设为当前演练项目」返回 200
// 但 active_project 永远写不进去（协同页永久空态、大屏默认项目失效）。
// 这里对包内全部非测试源码做静态扫描，防止同类写法再次混入。
func TestSettingsKeyMustBeBackticked(t *testing.T) {
	badPatterns := []*regexp.Regexp{
		regexp.MustCompile("(?i)settings\\s*\\(\\s*key\\b"),
		regexp.MustCompile("(?i)settings\\s+where\\s+key\\b"),
		regexp.MustCompile("(?i)settings\\s+set\\s+key\\b"),
		regexp.MustCompile("(?i)update\\s+settings\\s+set\\s+key\\b"),
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("未找到 handler 包源码，测试工作目录异常")
	}

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", f, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			for _, p := range badPatterns {
				if p.MatchString(line) {
					t.Errorf("%s:%d 使用了未转义的保留字 key：%s\n请改为 `key`", f, i+1, trimmed)
				}
			}
		}
	}
}
