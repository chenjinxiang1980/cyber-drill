package main

// 补封存运维命令：对「已结束但无评分封存」的项目补写封存记录。
// 复用 handler 内的核对、评分、快照、缺项与审计逻辑，不更新 projects 行。
//
// 用法（在应用容器内执行，沿用容器的 DB 环境变量）：
//   supplemental-seal -project P2026-001 -reason "..." -owner "..." \
//     -deadline "2026-11-10 13:51:21" -approver "..." -actor "平台管理员"

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"drill-system/internal/config"
	"drill-system/internal/db"
	"drill-system/internal/handler"
)

func main() {
	root := flag.String("root", "/opt/drill", "应用根目录（读取 .env，环境变量优先）")
	project := flag.String("project", "", "项目 ID")
	reason := flag.String("reason", "", "缺项原因")
	owner := flag.String("owner", "", "补录责任人")
	deadline := flag.String("deadline", "", "补录时限（YYYY-MM-DD HH:mm[:ss]）")
	approver := flag.String("approver", "", "批准人")
	actor := flag.String("actor", "平台管理员", "操作人（写入 sealed_by / timeline / audit）")
	role := flag.String("role", "platformadmin", "操作人角色 ID")
	userID := flag.String("user-id", "56", "操作人 user_id")
	dryRun := flag.Bool("dry-run", false, "只打印核对清单与预演评分，不写库")
	flag.Parse()

	if *project == "" || *reason == "" || *owner == "" || *deadline == "" || *approver == "" {
		fmt.Fprintln(os.Stderr, "缺少必填参数：-project -reason -owner -deadline -approver")
		os.Exit(2)
	}

	cfg := config.Load(*root)
	database, err := db.Open(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "连接数据库失败："+err.Error())
		os.Exit(2)
	}
	defer database.Close()

	out, err := handler.SupplementalSeal(database, *project, handler.SupplementalSealInput{
		Reason: *reason, Owner: *owner, Deadline: *deadline, Approver: *approver,
		Actor: *actor, ActorRole: *role, ActorID: *userID, DryRun: *dryRun,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "补封存失败："+err.Error())
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	if !*dryRun && !out.Matched {
		fmt.Fprintln(os.Stderr, "快照分与实时 Score() 不一致")
		os.Exit(3)
	}
}
