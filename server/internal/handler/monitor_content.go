package handler

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// MonitorContentHandler GET /api/content/monitor（需登录）
// 大屏监视聚合 DTO：对齐 src/routes/content.js:1611 router.get('/monitor')
// 所有 rows 立即 Close，禁止 defer 在同一函数内延迟关闭导致 MySQL 多结果集 race
func MonitorContentHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, monitorPayload(database, c.Query("projectId"), false))
	}
}

// ScreenMonitorHandler GET /api/screen/monitor（大屏免登录只读）
// 与登录态大屏使用同一份展示 DTO；匿名视图把操作人姓名脱敏为角色名，时间线保留。
func ScreenMonitorHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		anonymous := auth.CurrentUser(c) == nil
		c.JSON(http.StatusOK, monitorPayload(database, c.Query("projectId"), anonymous))
	}
}

func monitorPayload(database *db.DB, qProjID string, anonymous bool) gin.H {
	if qProjID == "" {
		var activeID sql.NullString
		_ = database.QueryRow(`SELECT id FROM projects WHERE status != '未开始' LIMIT 1`).Scan(&activeID)
		if activeID.Valid {
			qProjID = activeID.String
		}
	}

	// ---- projects 列表 ----
	var projects []gin.H
	projRows, _ := database.Query(`SELECT id,name,COALESCE(date,''),COALESCE(status,'未开始'),COALESCE(mode,'基础版'),
			COALESCE(level,''),COALESCE(scenario_id,''),COALESCE(started_at,''),COALESCE(paused_at,''),
			COALESCE(paused_sec,0),COALESCE(ended_at,'') FROM projects ORDER BY COALESCE(date,'') DESC, id DESC`)
	if projRows != nil {
		for projRows.Next() {
			var id, name, date, status, mode, level, scenarioID, startedAt, pausedAt, endedAt string
			var pausedSec int
			projRows.Scan(&id, &name, &date, &status, &mode, &level, &scenarioID, &startedAt, &pausedAt, &pausedSec, &endedAt)
			es := int64(0)
			if sec := elapsedSecLite(startedAt, pausedAt, pausedSec, endedAt, status); sec != nil {
				es = *sec
			}
			projects = append(projects, gin.H{
				"id": id, "name": name, "date": date, "status": status, "mode": mode,
				"level": level, "scenarioId": scenarioID, "startedAt": nullIfEmpty(startedAt),
				"pausedAt": nullIfEmpty(pausedAt), "pausedSec": pausedSec, "endedAt": nullIfEmpty(endedAt),
				"elapsedSec": es,
			})
		}
		projRows.Close()
	}

	// ---- 当前选中项目 ----
	var curProj gin.H
	var pMode, pScenarioID, pStartedAt string
	if qProjID != "" {
		var id, name, date, status, mode, level, scenarioID, startedAt, pausedAt, endedAt string
		var pausedSec int
		err := database.QueryRow(`SELECT id,name,COALESCE(date,''),COALESCE(status,'未开始'),COALESCE(mode,'基础版'),
				COALESCE(level,''),COALESCE(scenario_id,''),COALESCE(started_at,''),COALESCE(paused_at,''),
				COALESCE(paused_sec,0),COALESCE(ended_at,'') FROM projects WHERE id=?`, qProjID).
			Scan(&id, &name, &date, &status, &mode, &level, &scenarioID, &startedAt, &pausedAt, &pausedSec, &endedAt)
		if err == nil {
			es := int64(0)
			if sec := elapsedSecLite(startedAt, pausedAt, pausedSec, endedAt, status); sec != nil {
				es = *sec
			}
			curProj = gin.H{
				"id": id, "name": name, "date": date, "status": status, "mode": mode,
				"level": level, "scenarioId": scenarioID, "startedAt": nullIfEmpty(startedAt),
				"pausedAt": nullIfEmpty(pausedAt), "pausedSec": pausedSec, "endedAt": nullIfEmpty(endedAt),
				"elapsedSec": es,
			}
			pMode = mode
			pScenarioID = scenarioID
			pStartedAt = startedAt
		}
	}
	if curProj == nil {
		if len(projects) > 0 {
			curProj = projects[0]
			if qProjID == "" {
				qProjID = curProj["id"].(string)
			}
			// 补 pMode / pScenarioID
			_ = database.QueryRow(`SELECT COALESCE(mode,'基础版'),COALESCE(scenario_id,''),COALESCE(started_at,'') FROM projects WHERE id=?`, qProjID).
				Scan(&pMode, &pScenarioID, &pStartedAt)
		} else {
			return gin.H{
				"serverTime": nowSec(),
				"project":    nil, "projects": []gin.H{},
				"stats": gin.H{"total": 0, "done": 0, "doing": 0, "pending": 0, "overdue": 0},
				"tasks": []gin.H{}, "timeline": []gin.H{}, "seqFlow": []gin.H{},
				"chainNodes": []gin.H{}, "score": gin.H{}, "impact": []gin.H{},
				"messages": []gin.H{}, "roles": []gin.H{}, "roleAgg": gin.H{},
			}
		}
	}

	// ---- tasks ----
	var tasks []gin.H
	taskRows, _ := database.Query(`SELECT id,COALESCE(role_id,''),COALESCE(title,''),COALESCE(status,''),
			COALESCE(level,''),COALESCE(deadline,''),COALESCE(accepted_at,''),COALESCE(completed_at,''),COALESCE(node_id,'')
			FROM tasks WHERE project_id=? ORDER BY seq`, qProjID)
	taskStatusByNode := map[string]gin.H{}
	if taskRows != nil {
		for taskRows.Next() {
			var tid, roleID, title, tstatus, level, deadline, acceptedAt, completedAt, nodeID string
			taskRows.Scan(&tid, &roleID, &title, &tstatus, &level, &deadline, &acceptedAt, &completedAt, &nodeID)
			t := gin.H{
				"id": tid, "role": roleID, "title": title, "status": tstatus, "level": level,
				"deadline": nullIfEmpty(deadline), "acceptedAt": nullIfEmpty(acceptedAt), "completedAt": nullIfEmpty(completedAt),
			}
			tasks = append(tasks, t)
			if nodeID != "" {
				taskStatusByNode[nodeID] = gin.H{
					"status": tstatus, "completed_at": completedAt, "accepted_at": acceptedAt,
				}
			}
		}
		taskRows.Close()
	}

	// ---- timeline ----
	var timeline []gin.H
	tlRows, _ := database.Query(`SELECT COALESCE(t,''),COALESCE(actor,''),COALESCE(role_id,''),
			COALESCE(action,''),COALESCE(detail,''),COALESCE(level,'') FROM timeline
			WHERE project_id=? ORDER BY seq DESC LIMIT 200`, qProjID)
	if tlRows != nil {
		for tlRows.Next() {
			var t, actor, roleID, action, detail, level string
			tlRows.Scan(&t, &actor, &roleID, &action, &detail, &level)
			timeline = append(timeline, gin.H{"t": t, "actor": actor, "role": roleID, "action": action, "detail": detail, "level": level})
		}
		tlRows.Close()
	}
	sort.Slice(timeline, func(i, j int) bool { return timeline[i]["t"].(string) < timeline[j]["t"].(string) })

	// ---- score (完整 ScoreResult) ----
	sc := displayScore(database, qProjID)

	// ---- impact ----
	// 与 /api/content/impact 共用 impactSnapshot：项目实例行 → 全局模板补齐 → 基线兜底，
	// 保证大屏与看板不会各算一套。
	impact := impactSnapshot(database, qProjID)

	// ---- roles ----
	var roles []gin.H
	roleRows, rerr := database.Query("SELECT id,COALESCE(name,''),COALESCE(color,''),COALESCE(`external`,0) FROM roles ORDER BY seq")
	if roleRows != nil {
		for roleRows.Next() {
			var rid, rname, rcolor string
			var rext int
			if err := roleRows.Scan(&rid, &rname, &rcolor, &rext); err != nil {
				continue
			}
			roles = append(roles, gin.H{"id": rid, "name": rname, "color": rcolor, "external": rext == 1})
		}
		roleRows.Close()
	}
	if rerr != nil {
		roles = []gin.H{}
	}
	roleByName := map[string]string{}
	for _, r := range roles {
		roleByName[strOr(r["name"], "")] = strOr(r["id"], "")
	}
	roleNameByID := map[string]string{}
	for _, r := range roles {
		roleNameByID[strOr(r["id"], "")] = strOr(r["name"], "")
	}

	// 匿名大屏：操作人姓名 → 角色名；时间线、动作、详情保留，只做姓名脱敏
	var anonMask func(string) string
	if anonymous {
		anonMask = anonNameMasker(database)
		for _, it := range timeline {
			rid := strOr(it["role"], "")
			actor := strOr(it["actor"], "")
			if rid != "" {
				if rn := roleNameByID[rid]; rn != "" {
					actor = rn
				} else if actor != "" {
					actor = anonMask(actor)
				}
			} else if actor != "" {
				actor = anonMask(actor)
			}
			it["actor"] = actor
			it["action"] = anonMask(strOr(it["action"], ""))
			it["detail"] = anonMask(strOr(it["detail"], ""))
		}
	}

	// ---- drill_steps ----
	stepByNode := map[string]gin.H{}
	type stepLite struct {
		nodeID, stepTime, roleID string
		ontime, correct, skipped int
	}
	var steps []stepLite
	stepRows, _ := database.Query(`SELECT COALESCE(matched_node,''),COALESCE(ontime,1),COALESCE(correct,1),
			COALESCE(skipped,0),COALESCE(step_time,''),COALESCE(role_id,'') FROM drill_steps WHERE project_id=?`, qProjID)
	if stepRows != nil {
		for stepRows.Next() {
			var nodeID, stepTime, roleID string
			var ontime, correct, skipped int
			stepRows.Scan(&nodeID, &ontime, &correct, &skipped, &stepTime, &roleID)
			stepByNode[nodeID] = gin.H{"ontime": ontime, "correct": correct, "skipped": skipped, "step_time": stepTime, "role_id": roleID}
			steps = append(steps, stepLite{nodeID, stepTime, roleID, ontime, correct, skipped})
		}
		stepRows.Close()
	}

	// ---- drill_node_state ----
	doingNode := map[string]gin.H{}
	dnsRows, _ := database.Query(`SELECT COALESCE(node_id,''),COALESCE(user,''),COALESCE(started_at,'') FROM drill_node_state WHERE project_id=?`, qProjID)
	if dnsRows != nil {
		for dnsRows.Next() {
			var nid, u, sa string
			dnsRows.Scan(&nid, &u, &sa)
			doingNode[nid] = gin.H{"node_id": nid, "user": u, "started_at": sa}
		}
		dnsRows.Close()
	}

	// ---- script_nodes ----
	type nodeLite struct {
		id, target, depsStr, parallelDesc, flowType, ntype, title, shortTitle string
		seq, timeout                                                          int
	}
	var scriptNodes []nodeLite
	nodesRows, _ := database.Query(`SELECT id,COALESCE(target,''),COALESCE(deps,''),
			COALESCE(parallel_desc,''),COALESCE(seq,0),COALESCE(timeout,0),
			COALESCE(flow_type,'correct'),COALESCE(type,'task'),
			COALESCE(title,''),COALESCE(short_title,'')
			FROM script_nodes WHERE scenario_id=? AND deleted_at IS NULL AND type!='end'
			AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq`, pScenarioID)
	if nodesRows != nil {
		for nodesRows.Next() {
			var nid, target, depsStr, parallelDesc, flowType, ntype, title, shortTitle string
			var seq, timeout int
			nodesRows.Scan(&nid, &target, &depsStr, &parallelDesc, &seq, &timeout, &flowType, &ntype, &title, &shortTitle)
			scriptNodes = append(scriptNodes, nodeLite{nid, target, depsStr, parallelDesc, flowType, ntype, title, shortTitle, seq, timeout})
		}
		nodesRows.Close()
	}
	nodeById := map[string]nodeLite{}
	for _, n := range scriptNodes {
		nodeById[n.id] = n
	}

	// ---- chainNodes + seqFlow (computeMonitorNodes) ----
	var chainNodes, seqFlow []gin.H
	total, doneCnt, doingCnt, pendingCnt, overdueCnt := 0, 0, 0, 0, 0

	// 评分引擎的逐节点结论：基础版任务状态只有「已完成」，是否超时由 Score 依据
	// 节点时限与完成耗时判定（late），必须用它校正，否则「已完成但超时」会被漏掉。
	scoreNodeByID := map[string]NodeResult{}
	if sc != nil {
		for _, sn := range sc.Nodes {
			scoreNodeByID[sn.ID] = sn
		}
	}

	visible := map[string]bool{}
	if pMode == "实战版" {
		for _, n := range scriptNodes {
			_, recorded := stepByNode[n.id]
			_, doing := doingNode[n.id]
			deps := parseDepsGo(n.depsStr)
			allDeps := true
			for _, d := range deps {
				if _, ok := stepByNode[d]; !ok {
					allDeps = false
					break
				}
			}
			visible[n.id] = recorded || doing || allDeps
		}
	}

	for _, n := range scriptNodes {
		total++
		hidden := (pMode == "实战版" && !visible[n.id])

		_, hasStep := stepByNode[n.id]
		_, hasDoing := doingNode[n.id]
		var state, timeStr string
		var isOverdue bool

		if hasStep {
			s := stepByNode[n.id]
			state = "done"
			timeStr = strOr(s["step_time"], "")
			if cInt(s["ontime"]) == 0 && cInt(s["correct"]) == 1 {
				isOverdue = true
			}
		} else {
			ts := taskStatusByNode[n.id]
			tStatus := strOr(ts["status"], "")
			tCompletedAt := strOr(ts["completed_at"], "")
			tAcceptedAt := strOr(ts["accepted_at"], "")
			if tStatus == "已完成" {
				state = "done"
				timeStr = tCompletedAt
			} else if hasDoing {
				state = "doing"
				timeStr = strOr(doingNode[n.id]["started_at"], "")
			} else if tStatus == "处理中" {
				state = "doing"
				timeStr = tAcceptedAt
			} else if tStatus == "超时" {
				state = "done"
				isOverdue = true
				timeStr = tCompletedAt
			} else {
				state = "pending"
			}
		}
		// 以评分引擎结论校正「已完成但超时」：late 节点一律计为超时完成；
		// 若本地状态还没落到 done（提交记录缺失），也按已完成收敛，避免统计漏项。
		if sn, ok := scoreNodeByID[n.id]; ok && sn.Late {
			isOverdue = true
			if state != "done" {
				state = "done"
			}
		}
		if isOverdue {
			overdueCnt++
		}
		switch state {
		case "done":
			doneCnt++
		case "doing":
			doingCnt++
		default:
			pendingCnt++
		}

		if timeStr != "" && len(timeStr) >= 16 && strings.HasPrefix(timeStr, "20") {
			timeStr = timeStr[5:16]
		}

		displayName := n.title
		if strings.TrimSpace(displayName) == "" {
			displayName = n.target
		}
		displayShort := strings.TrimSpace(n.shortTitle)
		if displayShort == "" {
			displayShort = displayName
		}
		item := gin.H{
			"id": n.id, "seq": n.seq, "name": displayName, "shortTitle": displayShort,
			"role": n.target, "time": timeStr, "state": state, "isOverdue": isOverdue,
			"timeout": n.timeout, "deps": parseDepsGo(n.depsStr), "type": n.ntype,
			"parallelDesc": n.parallelDesc,
		}
		chainNodes = append(chainNodes, item)
		if !hidden && state != "pending" {
			seqFlow = append(seqFlow, item)
		}
	}

	// 注：overdueCnt 已在上面的循环里用 Score 的 late 结论校正；遗漏节点仍按未完成计入 pending

	// ---- roleAgg ----
	roleAgg := gin.H{}
	for _, t := range tasks {
		rid := strOr(t["role"], "")
		if rid == "" {
			continue
		}
		ag, ok := roleAgg[rid].(gin.H)
		if !ok {
			ag = gin.H{"roleId": rid, "total": 0, "done": 0, "overtime": 0}
		}
		ag["total"] = cInt(ag["total"]) + 1
		if strOr(t["status"], "") == "已完成" {
			ag["done"] = cInt(ag["done"]) + 1
		}
		if strOr(t["status"], "") == "超时" {
			ag["overtime"] = cInt(ag["overtime"]) + 1
		}
		roleAgg[rid] = ag
	}
	for _, s := range steps {
		rid := s.roleID
		if rid == "" {
			if n, ok := nodeById[s.nodeID]; ok {
				rid = roleByName[n.target]
			}
		}
		if rid == "" {
			continue
		}
		ag, ok := roleAgg[rid].(gin.H)
		if !ok {
			ag = gin.H{"roleId": rid, "total": 0, "done": 0, "overtime": 0}
		}
		ag["total"] = cInt(ag["total"]) + 1
		if s.correct == 1 {
			ag["done"] = cInt(ag["done"]) + 1
		}
		if s.correct == 1 && s.ontime == 0 {
			ag["overtime"] = cInt(ag["overtime"]) + 1
		}
		roleAgg[rid] = ag
	}

	// 统一「超时」口径：以 chainNodes 的 isOverdue 为准（基础版任务状态只有「已完成」，
	// 超时由 Score 的 late 判定），避免角色面板与顶部统计各算一套。
	for _, v := range roleAgg {
		if ag, ok := v.(gin.H); ok {
			ag["overtime"] = 0
		}
	}
	for _, it := range chainNodes {
		ov, _ := it["isOverdue"].(bool)
		if !ov {
			continue
		}
		rid := roleByName[strOr(it["role"], "")]
		if rid == "" {
			continue
		}
		if v, ok := roleAgg[rid]; ok {
			if ag, ok2 := v.(gin.H); ok2 {
				ag["overtime"] = cInt(ag["overtime"]) + 1
			}
		}
	}

	// ---- messages ----
	var msgs []gin.H
	msgRows, _ := database.Query(`SELECT COALESCE(time,''),COALESCE(level,'P2'),COALESCE(source,''),COALESCE(text,'')
			FROM messages WHERE project_id=? OR project_id IS NULL ORDER BY seq DESC LIMIT 16`, qProjID)
	if msgRows != nil {
		for msgRows.Next() {
			var tm, lv, src, txt string
			msgRows.Scan(&tm, &lv, &src, &txt)
			if anonymous {
				src = anonMask(src)
				txt = anonMask(txt)
			}
			msgs = append(msgs, gin.H{"time": tm, "level": lv, "source": src, "text": txt})
		}
		msgRows.Close()
	}
	var chatMsgs []gin.H
	chatRows, _ := database.Query(`SELECT COALESCE(time,''),COALESCE(sender,''),COALESCE(role_id,''),COALESCE(text,'')
			FROM chat_messages WHERE grp='inner' AND (project_id=? OR project_id IS NULL) ORDER BY id DESC LIMIT 16`, qProjID)
	if chatRows != nil {
		for chatRows.Next() {
			var tm, sender, rid, txt string
			chatRows.Scan(&tm, &sender, &rid, &txt)
			src := "院内会商·" + sender
			if anonymous {
				name := roleNameByID[rid]
				if name == "" {
					name = anonMask(sender)
				}
				if name == "" {
					name = "参演人员"
				}
				src = "院内会商·" + name
				txt = anonMask(txt)
			}
			chatMsgs = append(chatMsgs, gin.H{"time": tm, "level": "P2", "source": src, "text": txt})
		}
		chatRows.Close()
	}
	mergedMsgs := append(msgs, chatMsgs...)
	sort.Slice(mergedMsgs, func(i, j int) bool {
		return strOr(mergedMsgs[i]["time"], "") > strOr(mergedMsgs[j]["time"], "")
	})
	if len(mergedMsgs) > 24 {
		mergedMsgs = mergedMsgs[:24]
	}

	return gin.H{
		"serverTime": nowSec(),
		"project":    curProj,
		"projects":   projects,
		"stats": gin.H{
			"total": total, "done": doneCnt, "doing": doingCnt,
			"pending": pendingCnt, "overdue": overdueCnt,
		},
		"tasks":      tasks,
		"timeline":   timeline,
		"seqFlow":    seqFlow,
		"chainNodes": chainNodes,
		"score":      sc,
		"impact":     impact,
		"messages":   mergedMsgs,
		"roles":      roles,
		"roleAgg":    roleAgg,
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func strOr(v interface{}, def string) string {
	if v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return def
	}
	return s
}

func cInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
