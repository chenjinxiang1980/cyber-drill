// verify_tls — 自动化 TLS 行为断言（全进程实测）
// 1. TLS_ENABLED=false  → 真启动 HTTP server，GET /api/health → 200
// 2. TLS_ENABLED=true + 证书缺失 → 真启动，断言 exit code=2，stderr 含证书路径
// 3. TLS_ENABLED=true + 真证书   → 真启动 HTTPS，GET /api/health → 200
// 4. TLS_ENABLED=true + 真证书 + HTTP_REDIRECT_TO_HTTPS=true → HTTP GET → 301 + https Location
//
// 运行: PROBE_DB_NAME=drill_system go run ./cmd/verify_tls
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func hexPad(s string, n int) string {
	for len(s) < n {
		s += "0"
	}
	return s
}

// pickFreePort 找一个空闲端口
func pickFreePort() int {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// waitHTTP 轮询直到 http://host:port/path 返回状态码
func waitHTTP(client *http.Client, url string, wantStatus int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return resp.StatusCode, nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, fmt.Errorf("超时：期待 %d 但没成功", wantStatus)
}

// waitHTTPS 同 waitHTTP 但忽略证书校验
func waitHTTPS(hostPort, path string, wantStatus int, timeout time.Duration) (int, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	return waitHTTP(client, "https://"+hostPort+path, wantStatus, timeout)
}

// runServer 启动 drill-server.exe，设置环境变量，返回 cmd、stdout+stderr 合并的 buffer
// exePath 相对于当前工作目录：server/deploy/drill-server.exe
func runServer(projectRoot string, env map[string]string, args ...string) (*exec.Cmd, *strings.Builder) {
	exePath, _ := filepath.Abs("server/deploy/drill-server.exe")
	cmd := exec.Command(exePath, args...)
	// 继承当前进程环境
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	return cmd, &buf
}

// genSelfSigned 生成临时自签证书，返回 certPEM、keyPEM 字节
func genSelfSigned() ([]byte, []byte) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:              []string{"localhost"},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func main() {
	cwd, _ := os.Getwd()
	projectRoot := cwd // 从项目根目录运行时就是 E:\Trae\Emergency

	// 共用的必填环境变量（覆盖 .env 里可能有的冲突值）
	// ⚠️ DB_USER/DB_PASSWORD 不在此设置，让子进程从 .env 或父进程环境继承 —— 仓库内零凭证字面量
	baseEnv := func(httpAddr string) map[string]string {
		return map[string]string{
			"DB_NAME":        "drill_system",
			"DB_HOST":        "127.0.0.1",
			"DB_PORT":        "3306",
			"JWT_SECRET":     hexPad("deadbeef", 64),
			"SESSION_SECRET": hexPad("cafebabe", 64),
			"APP_ENV":        "test",
			"TLS_ENABLED":    "false", // 每个断言里会覆盖
			"HTTP_ADDR":      httpAddr,
		}
	}

	allPass := true

	// ============ 断言 1: TLS_ENABLED=false → HTTP 正常 ============
	fmt.Println("=== 断言1: TLS_ENABLED=false → HTTP 正常 /api/health ===")
	{
		httpPort := pickFreePort()
		httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
		env := baseEnv(httpAddr)
		env["TLS_ENABLED"] = "false"

		cmd, buf := runServer(projectRoot, env, "-root", projectRoot)
		if err := cmd.Start(); err != nil {
			fmt.Printf("  ❌ 启动失败: %v\n", err)
			allPass = false
		} else {
			fmt.Printf("  进程已启动 PID=%d，HTTP_ADDR=%s\n", cmd.Process.Pid, httpAddr)
			status, err := waitHTTP(&http.Client{Timeout: 2 * time.Second},
				"http://"+httpAddr+"/api/health", 200, 10*time.Second)
			if err != nil {
				fmt.Printf("  ❌ /api/health 失败: %v\n", err)
				fmt.Printf("  --- 进程输出 ---\n%s\n----------------\n", buf.String())
				allPass = false
			} else {
				fmt.Printf("  ✅ HTTP /api/health → %d OK\n", status)
			}
			// 优雅杀进程
			cmd.Process.Kill()
			cmd.Wait()
		}
	}

	// ============ 断言 2: TLS_ENABLED=true + 证书缺失 → exit 2 ============
	fmt.Println("\n=== 断言2: TLS_ENABLED=true + 证书缺失 → exit 2 ===")
	{
		env := baseEnv("127.0.0.1:18092")
		env["TLS_ENABLED"] = "true"
		env["HTTPS_CERT_FILE"] = "__nonexistent_cert_path_xyz__.crt"
		env["HTTPS_KEY_FILE"] = "__nonexistent_key_path_xyz__.key"

		cmd, buf := runServer(projectRoot, env, "-root", projectRoot)
		// 用 Start + Wait 同步，因为 config.Load exit 2 很快
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode := exitErr.ExitCode()
				fmt.Printf("  进程退出码: %d\n", exitCode)
				output := buf.String()
				fmt.Printf("  --- 进程输出 ---\n%s\n----------------\n", output)
				if exitCode == 2 {
					if strings.Contains(output, "证书") || strings.Contains(output, "nonexistent") {
						fmt.Println("  ✅ exit code=2 且输出包含证书缺失提示")
					} else {
						fmt.Println("  ⚠️ exit code=2 但输出没提到证书路径")
						allPass = false
					}
				} else {
					fmt.Printf("  ❌ 期望 exit code=2 但 got %d\n", exitCode)
					allPass = false
				}
			} else {
				fmt.Printf("  ❌ 执行错误: %v\n", err)
				allPass = false
			}
		} else {
			fmt.Println("  ❌ 进程意外 exit 0（证书缺失应该拒绝启动）")
			allPass = false
		}
	}

	// ============ 断言 3: TLS_ENABLED=true + 真证书 → HTTPS 正常 ============
	fmt.Println("\n=== 断言3: TLS_ENABLED=true + 自签证书 → HTTPS /api/health ===")
	{
		// 生成临时证书
		certPEM, keyPEM := genSelfSigned()
		tmpDir, _ := os.MkdirTemp("", "verify_tls_")
		defer os.RemoveAll(tmpDir)
		certPath := filepath.Join(tmpDir, "server.crt")
		keyPath := filepath.Join(tmpDir, "server.key")
		os.WriteFile(certPath, certPEM, 0600)
		os.WriteFile(keyPath, keyPEM, 0600)

		tlsPort := pickFreePort()
		tlsAddr := fmt.Sprintf("127.0.0.1:%d", tlsPort)
		env := baseEnv("127.0.0.1:18093")
		env["TLS_ENABLED"] = "true"
		env["TLS_ADDR"] = tlsAddr
		env["HTTPS_CERT_FILE"] = certPath
		env["HTTPS_KEY_FILE"] = keyPath

		cmd, buf := runServer(projectRoot, env, "-root", projectRoot)
		if err := cmd.Start(); err != nil {
			fmt.Printf("  ❌ 启动失败: %v\n", err)
			allPass = false
		} else {
			fmt.Printf("  进程已启动 PID=%d，TLS_ADDR=%s\n", cmd.Process.Pid, tlsAddr)
			status, err := waitHTTPS(tlsAddr, "/api/health", 200, 10*time.Second)
			if err != nil {
				fmt.Printf("  ❌ HTTPS /api/health 失败: %v\n", err)
				fmt.Printf("  --- 进程输出 ---\n%s\n----------------\n", buf.String())
				allPass = false
			} else {
				fmt.Printf("  ✅ HTTPS %s/api/health → %d OK\n", tlsAddr, status)
			}
			cmd.Process.Kill()
			cmd.Wait()
		}
	}

	// ============ 断言 4: TLS_ENABLED=true + HTTP_REDIRECT_TO_HTTPS → 301 ============
	fmt.Println("\n=== 断言4: TLS_ENABLED=true + HTTP_REDIRECT_TO_HTTPS → HTTP 301 ===")
	{
		certPEM, keyPEM := genSelfSigned()
		tmpDir, _ := os.MkdirTemp("", "verify_tls_r_")
		defer os.RemoveAll(tmpDir)
		certPath := filepath.Join(tmpDir, "server.crt")
		keyPath := filepath.Join(tmpDir, "server.key")
		os.WriteFile(certPath, certPEM, 0600)
		os.WriteFile(keyPath, keyPEM, 0600)

		httpPort := pickFreePort()
		httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
		tlsPort := pickFreePort()
		tlsAddr := fmt.Sprintf("127.0.0.1:%d", tlsPort)

		env := baseEnv(httpAddr)
		env["TLS_ENABLED"] = "true"
		env["TLS_ADDR"] = tlsAddr
		env["HTTPS_CERT_FILE"] = certPath
		env["HTTPS_KEY_FILE"] = keyPath
		env["HTTP_REDIRECT_TO_HTTPS"] = "true"

		cmd, buf := runServer(projectRoot, env, "-root", projectRoot)
		if err := cmd.Start(); err != nil {
			fmt.Printf("  ❌ 启动失败: %v\n", err)
			allPass = false
		} else {
			fmt.Printf("  进程已启动 PID=%d，HTTP=%s TLS=%s\n", cmd.Process.Pid, httpAddr, tlsAddr)

			// 先等 HTTPS 主服务起来（它先于 HTTP redirect 监听）
			_, err := waitHTTPS(tlsAddr, "/api/health", 200, 10*time.Second)
			if err != nil {
				fmt.Printf("  ⚠️ HTTPS 主服务未就绪: %v（继续测 HTTP 301）\n", err)
			}

			// 用不跟随跳转的 client 测 HTTP → 301
			httpNoFollow := &http.Client{
				Timeout: 2 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			deadline := time.Now().Add(10 * time.Second)
			var gotStatus int
			var gotLoc string
			var lastErr error
			for time.Now().Before(deadline) {
				resp, err2 := httpNoFollow.Get("http://" + httpAddr + "/api/health")
				if err2 == nil {
					resp.Body.Close()
					gotStatus = resp.StatusCode
					gotLoc = resp.Header.Get("Location")
					break
				}
				lastErr = err2
				time.Sleep(150 * time.Millisecond)
			}
			if gotStatus == 0 {
				fmt.Printf("  ❌ HTTP 请求失败: %v\n", lastErr)
				fmt.Printf("  --- 进程输出 ---\n%s\n----------------\n", buf.String())
				allPass = false
			} else if gotStatus == 301 && strings.HasPrefix(gotLoc, "https://") {
				fmt.Printf("  ✅ HTTP → HTTPS 301 跳转，Location=%s\n", gotLoc)
			} else {
				fmt.Printf("  ❌ 期望 301 + https Location 但 got status=%d loc=%s\n", gotStatus, gotLoc)
				allPass = false
			}
			cmd.Process.Kill()
			cmd.Wait()
		}
	}

	// ============ 汇总 ============
	fmt.Println("\n==========================================")
	if allPass {
		fmt.Println("  🟢 verify_tls: ALL 4 ASSERTIONS PASS（全进程实测）")
	} else {
		fmt.Println("  🔴 verify_tls: SOME FAILED")
		os.Exit(1)
	}
	fmt.Println("==========================================")
}
