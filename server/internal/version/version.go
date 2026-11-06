// Package version 集中定义应用版本号。
// 发布构建时可由 -ldflags "-X drill-system/internal/version.Version=..." 覆盖。
package version

var (
	// Version 当前发布版本。
	Version = "0.6.0"
	// BuildTime 构建时间，发布构建时注入。
	BuildTime = "unknown"
	// GitCommit 构建对应提交，发布构建时注入。
	GitCommit = "unknown"
)
