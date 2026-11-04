// prodserver —— 生产入口（独立 cmd，绕过 SmartScreen 对 go run . 的拦截）。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"drill-system/internal/auth"
	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/router"

	"github.com/gin-gonic/gin"
)

func main() {
	rootFlag := flag.String("root", "..", "项目根目录")
	flag.Parse()
	var projectRoot string
	if filepath.IsAbs(*rootFlag) {
		projectRoot = *rootFlag
	} else {
		cwd, _ := os.Getwd()
		projectRoot, _ = filepath.Abs(filepath.Join(cwd, *rootFlag))
	}

	cfg := config.Load(projectRoot)
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}
	defer database.Close()

	authSvc := auth.NewService(database, cfg)

	gin.SetMode(gin.ReleaseMode)
	r := router.New(database, cfg, authSvc, projectRoot)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: r,
	}

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\n🛑 收到信号，关闭服务...")
		srv.Close()
	}()

	fmt.Printf("🚀 Go 后端启动 http://%s (root=%s)\n", cfg.HTTPAddr, projectRoot)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("ListenAndServe:", err)
	}
	fmt.Println("✅ 服务已关闭")
}
