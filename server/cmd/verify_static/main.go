// verify_static —— 目录穿越防御 + 正常静态资源回归测试（自动化）
// 启动 drill-server.exe 后用 HTTP client 打 14 条载荷：
//
//	7 条穿越载荷 → 必须 404，响应体不含敏感内容
//	7 条正常资源 → 必须 200
//
// 用法：PROBE_DB_NAME=drill_system go run ./cmd/verify_static
// 依赖：同目录有 deploy/drill-server.exe + public/ + .env
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// 支持两种运行方式：
	//   1) go run ./cmd/verify_static  （cwd = server/ → 上级 Emergency/）
	//   2) 编译后在项目根目录运行       （cwd = Emergency/）
	cwd, _ := os.Getwd()
	var projectRoot string
	if strings.HasSuffix(cwd, "server") {
		projectRoot = filepath.Dir(cwd)
	} else {
		projectRoot = cwd
	}

	// 找空闲端口
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("❌ 无法监听测试端口:", err)
		os.Exit(1)
	}
	freePort := l.Addr().(*net.TCPAddr).Port
	l.Close()

	httpAddr := fmt.Sprintf("127.0.0.1:%d", freePort)

	// 启动 drill-server.exe
	exePath := filepath.Join(projectRoot, "server", "deploy", "drill-server.exe")
	if _, err := os.Stat(exePath); err != nil {
		fmt.Println("❌ 找不到 drill-server.exe:", exePath)
		os.Exit(1)
	}

	cmd := exec.Command(exePath, "-root", projectRoot)
	cmd.Env = os.Environ()
	// 覆盖 TLS 关，避免证书缺失
	cmd.Env = append(cmd.Env,
		"TLS_ENABLED=false",
		"HTTP_ADDR="+httpAddr,
	)
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		fmt.Println("❌ 启动 drill-server.exe 失败:", err)
		os.Exit(1)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// 等待服务就绪
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(10 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://%s/api/health", httpAddr))
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		fmt.Println("❌ 服务未能就绪")
		fmt.Println("--- server output ---")
		fmt.Println(buf.String())
		os.Exit(1)
	}

	fmt.Println("✅ 服务已就绪", httpAddr)
	fmt.Println()

	allPass := true

	// === 7 条目录穿越载荷 ===
	traversalPayloads := []struct {
		path  string
		label string
	}{
		{"/../.env", "纯 .. 穿越 .env"},
		{"/..%2f.env", "%2f 编码变体"},
		{"/%2e%2e/.env", "%2e 编码变体"},
		{"/../../.env", "双层 .."},
		{"/public/../../.env", "先挂 /public 再穿越"},
		{"/..%5c.env", "Windows 反斜杠编码"},
		{"/app.js/../../../.env", "正常资源 + 穿越"},
	}

	fmt.Println("=== 目录穿越载荷（全部必须 404 且 body 不含敏感内容）===")
	for _, p := range traversalPayloads {
		fullURL := fmt.Sprintf("http://%s%s", httpAddr, p.path)
		resp, err := client.Get(fullURL)
		if err != nil {
			fmt.Printf("  ❌ %-35s 请求错误: %v\n", p.label, err)
			allPass = false
			continue
		}
		body := make([]byte, 20)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		bodyStr := string(body[:n])

		if resp.StatusCode != 404 {
			fmt.Printf("  ❌ %-35s code=%d (期望 404) body=[%s]\n", p.label, resp.StatusCode, bodyStr)
			allPass = false
		} else if strings.Contains(bodyStr, "DB_") || strings.Contains(bodyStr, "PASSWORD") || strings.Contains(bodyStr, "JWT") {
			fmt.Printf("  ❌ %-35s 404 但响应体含敏感内容: [%s]\n", p.label, bodyStr)
			allPass = false
		} else {
			fmt.Printf("  ✅ %-35s 404 OK  body=[%s]\n", p.label, bodyStr)
		}
	}

	// === 7 条正常静态资源 ===
	normalResources := []struct {
		path  string
		label string
	}{
		{"/app.js", "app.js"},
		{"/styles.css", "styles.css"},
		{"/api.js", "api.js"},
		{"/system.js", "system.js"},
		{"/img/logo.png", "img/logo.png"},
		{"/", "根路径 → index.html (SPA)"},
		{"/screen.html", "screen.html"},
	}

	fmt.Println()
	fmt.Println("=== 正常静态资源（全部必须 200）===")
	for _, p := range normalResources {
		fullURL := fmt.Sprintf("http://%s%s", httpAddr, p.path)
		resp, err := client.Get(fullURL)
		if err != nil {
			fmt.Printf("  ❌ %-30s 请求错误: %v\n", p.label, err)
			allPass = false
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			fmt.Printf("  ❌ %-30s code=%d (期望 200)\n", p.label, resp.StatusCode)
			allPass = false
		} else {
			fmt.Printf("  ✅ %-30s 200 OK\n", p.label)
		}
	}

	// === 隐藏文件防御 ===
	fmt.Println()
	fmt.Println("=== 隐藏文件防御（.env / .git / .hidden 必须 404）===")
	hiddenPayloads := []string{"/.env", "/public/.env", "/.git/config", "/img/.hidden"}
	for _, p := range hiddenPayloads {
		fullURL := fmt.Sprintf("http://%s%s", httpAddr, p)
		resp, err := client.Get(fullURL)
		if err != nil {
			fmt.Printf("  ⚠️  %-25s 请求错误: %v\n", p, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			fmt.Printf("  ❌ %-25s code=%d (期望 404)\n", p, resp.StatusCode)
			allPass = false
		} else {
			fmt.Printf("  ✅ %-25s 404 OK\n", p)
		}
	}

	fmt.Println()
	fmt.Println("==========================================")
	if allPass {
		fmt.Println("  🟢 verify_static: ALL 断言 PASS")
	} else {
		fmt.Println("  🔴 verify_static: SOME FAILED")
		os.Exit(1)
	}
	fmt.Println("==========================================")
}
