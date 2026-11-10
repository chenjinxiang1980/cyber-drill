// 演练系统 Go 后端 · 入口
//
// 启动流程：
//  1. 解析 .env / 环境变量（config.Load），必填项缺失 → Fatal
//  2. 打开 MySQL 连接池并 Ping
//  3. 组装 Gin engine（CORS / 安全头 / 限流 / 静态 + /api）
//  4. Start 监听 HTTP(S)
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/handler"
	"drill-system/internal/router"
	"drill-system/internal/version"
)

func main() {
	projectRoot := flag.String("root", "../", "项目根目录（用于读 .env、挂 public/ 和 uploads/）")
	flag.Parse()
	absRoot, _ := filepath.Abs(*projectRoot)

	// 1. 配置加载
	slog.Info("启动", "version", version.Version, "buildTime", version.BuildTime, "gitCommit", version.GitCommit)
	slog.Info("加载配置", "root", absRoot)
	cfg := config.Load(absRoot)

	// 2. MySQL 连接
	database, err := db.Open(cfg)
	if err != nil {
		slog.Error("MySQL 连接失败", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	// 3. 定时清理过期 sessions（每分钟一次，轻量）
	authSvc := auth.NewService(database, cfg)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			authSvc.CleanExpired()
		}
	}()

	// 4. 组装 router
	r := router.New(database, cfg, authSvc, absRoot)

	// 4.1 审计保留：后台定期清理过期 audit_logs（0 = 关闭）
	handler.StartAuditRetention(database, cfg.AuditRetentionDays)

	// 5. 启动 HTTP(S)
	var mainServer *http.Server
	var redirectServer *http.Server

	if cfg.TLSEnabled {
		// === TLS 模式 ===
		tlsAddr := cfg.TLSAddr
		if tlsAddr == "" {
			tlsAddr = ":8443"
		}
		mainServer = &http.Server{
			Addr:              tlsAddr,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		slog.Info("TLS 模式", "addr", tlsAddr, "cert", cfg.HTTPSCertFile, "key", cfg.HTTPSKeyFile)

		if cfg.HTTPRedirectToHTTPS {
			// 同时起 HTTP 做 301 到 HTTPS
			httpAddr := cfg.HTTPAddr
			redirectServer = &http.Server{
				Addr: httpAddr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					// 跳转目标使用 TLS 监听端口，而不是 HTTP 监听端口
					host := req.Host
					if h, _, err := net.SplitHostPort(req.Host); err == nil {
						host = h
					}
					if _, port, err := net.SplitHostPort(tlsAddr); err == nil && port != "443" {
						host = net.JoinHostPort(host, port)
					}
					target := "https://" + host + req.RequestURI
					http.Redirect(w, req, target, http.StatusMovedPermanently)
				}),
				ReadHeaderTimeout: 10 * time.Second,
			}
			slog.Info("HTTP→HTTPS 301 跳转", "addr", httpAddr)
		}
	} else {
		// === HTTP 模式（默认）===
		mainServer = &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		slog.Info("HTTP 模式", "addr", cfg.HTTPAddr)
	}

	// 优雅关闭
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		var runErr error
		if cfg.TLSEnabled {
			runErr = mainServer.ListenAndServeTLS(cfg.HTTPSCertFile, cfg.HTTPSKeyFile)
		} else {
			runErr = mainServer.ListenAndServe()
		}
		if runErr != nil && runErr != http.ErrServerClosed {
			slog.Error("主服务意外退出", "err", runErr)
			os.Exit(1)
		}
	}()

	if redirectServer != nil {
		go func() {
			if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("跳转服务意外退出", "err", err)
			}
		}()
	}

	<-done
	slog.Info("收到退出信号，优雅关闭…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mainServer.Shutdown(ctx); err != nil {
		slog.Error("主服务优雅关闭超时", "err", err)
	} else {
		slog.Info("主服务已关闭")
	}
	if redirectServer != nil {
		_ = redirectServer.Shutdown(context.Background())
	}
}
