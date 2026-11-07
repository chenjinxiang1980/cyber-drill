package handler

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// initialPasswordEnvKeys 安装器写入部署 .env 的一次性初始口令变量。
// 只有这两个平台管理员账号由安装器生成口令；管理员在后台新建/重置的账号
// 走一次性响应返回，不落部署文件。
var initialPasswordEnvKeys = map[string]string{
	"admin":         "ADMIN_INIT_PASSWORD",
	"platformadmin": "PLATFORMADMIN_INIT_PASSWORD",
}

var initialPasswordMu sync.Mutex

// invalidateInitialPassword 在账号首次改密成功后，把部署 .env 中对应的初始口令
// 置空并标注失效时间，避免明文长期留驻。
//
// 仅当容器内挂载了 DRILL_ENV_FILE_PATH 且文件可写时生效；任何失败只记录日志，
// 绝不影响改密结果。未命中安装器账号（如后台新建账号）时直接返回。
func invalidateInitialPassword(username string) {
	key, ok := initialPasswordEnvKeys[username]
	if !ok {
		return
	}
	path := strings.TrimSpace(os.Getenv("DRILL_ENV_FILE_PATH"))
	if path == "" {
		return
	}

	initialPasswordMu.Lock()
	defer initialPasswordMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("initial password: 读取部署环境文件失败，跳过清理", "path", path, "err", err)
		return
	}

	lines := strings.Split(string(data), "\n")
	changed := false
	marked := false
	comment := fmt.Sprintf("# %s 已失效（账号 %s 于 %s 修改密码）",
		key, username, time.Now().Format("2006-01-02 15:04:05"))

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# "+key+" 已失效") {
			marked = true
			continue
		}
		if !strings.HasPrefix(line, key+"=") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, key+"=")) == "" {
			continue
		}
		suffix := ""
		if strings.HasSuffix(raw, "\r") {
			suffix = "\r"
		}
		lines[i] = key + "=" + suffix
		changed = true
	}
	if !changed {
		return
	}
	if !marked {
		lines = append(lines, comment)
	}

	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}

	mode := os.FileMode(0600)
	if st, statErr := os.Stat(path); statErr == nil {
		mode = st.Mode().Perm()
	}
	// 就地覆盖：该文件以单文件 bind mount 挂载，rename 覆盖挂载点会 EBUSY。
	if err := os.WriteFile(path, []byte(out), mode); err != nil {
		slog.Warn("initial password: 写入部署环境文件失败，跳过清理", "path", path, "err", err)
		return
	}
	_ = os.Chmod(path, mode)
	slog.Info("initial password: 已在部署环境文件中清空并标注失效", "account", username, "env", key)
}
