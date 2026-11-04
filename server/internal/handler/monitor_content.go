package handler

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"

	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// MonitorContentHandler GET /api/content/monitor
// 大屏监视聚合 DTO：对齐 src/routes/content.js:1611 router.get('/monitor')
// 所有 rows 立即 Close，禁止 defer 在同一函数内延迟关闭导致 MySQL 多结果集 race
func MonitorContentHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		qProjID := c.Query("projectId")
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
				c.JSON(http.StatusOK, gin.H{
					"serverTime": nowSec(),
					"project": nil, "projects": []gin.H{},
					"stats": gin.H{"total": 0, "done": 0, "doing": 0, "pending": 0, "overdue": 0},
					"tasks": []gin.H{}, "timeline": []gin.H{}, "seqFlow": []gin.H{},
					"chainNodes": []gin.H{}, "score": gin.H{}, "impact": []gin.H{},
					"messages": []gin.H{}, "roles": []gin.H{}, "roleAgg": gin.H{},
				})
				return
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
		sc := Score(database, qProjID)

		// ---- impact ----
		var impact []gin.H
		impactRows, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id=? ORDER BY seq`, qProjID)
		if impactRows != nil {
			for impactRows.Next() {
				var sys, state string
				var pct int
				impactRows.Scan(&sys, &state, &pct)
				impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
			}
			impactRows.Close()
		}
		if len(impact) == 0 {
			impactRows2, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id='' ORDER BY seq`)
			if impactRows2 != nil {
				for impactRows2.Next() {
					var sys, state string
					var pct int
					impactRows2.Scan(&sys, &state, &pct)
					impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
				}
				impactRows2.Close()
			}
		}

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
			id, target, depsStr, parallelDesc, flowType, ntype string
			seq, timeout int
		}
		var scriptNodes []nodeLite
		nodesRows, _ := database.Query(`SELECT id,COALESCE(target,''),COALESCE(deps,''),
			COALESCE(parallel_desc,''),COALESCE(seq,0),COALESCE(timeout,0),
			COALESCE(flow_type,'correct'),COALESCE(type,'task')
			FROM script_nodes WHERE scenario_id=? AND type!='end'
			AND COALESCE(flow_type,'correct')!='distractor' ORDER BY seq`, pScenarioID)
		if nodesRows != nil {
			for nodesRows.Next() {
				var nid, target, depsStr, parallelDesc, flowType, ntype string
				var seq, timeout int
				nodesRows.Scan(&nid, &target, &depsStr, &parallelDesc, &seq, &timeout, &flowType, &ntype)
				scriptNodes = append(scriptNodes, nodeLite{nid, target, depsStr, parallelDesc, flowType, ntype, seq, timeout})
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

			item := gin.H{
				"id": n.id, "seq": n.seq, "name": n.target, "shortTitle": n.target,
				"role": n.target, "time": timeStr, "state": state, "isOverdue": isOverdue,
				"timeout": n.timeout, "deps": parseDepsGo(n.depsStr), "type": n.ntype,
				"parallelDesc": n.parallelDesc,
			}
			chainNodes = append(chainNodes, item)
			if !hidden && state != "pending" {
				seqFlow = append(seqFlow, item)
			}
		}

		// 注：overdueCnt 由 chainNodes 逐节点计算（isOverdue=true 的数量），不使用 Score 的 late+missed 覆盖

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
			if s.correct == 1 && s.ontime != 0 {
				ag["done"] = cInt(ag["done"]) + 1
			}
			if s.correct == 1 && s.ontime == 0 {
				ag["overtime"] = cInt(ag["overtime"]) + 1
			}
			roleAgg[rid] = ag
		}

		// ---- messages ----
		var msgs []gin.H
		msgRows, _ := database.Query(`SELECT COALESCE(time,''),COALESCE(level,'P2'),COALESCE(source,''),COALESCE(text,'')
			FROM messages WHERE project_id=? OR project_id IS NULL ORDER BY seq DESC LIMIT 16`, qProjID)
		if msgRows != nil {
			for msgRows.Next() {
				var tm, lv, src, txt string
				msgRows.Scan(&tm, &lv, &src, &txt)
				msgs = append(msgs, gin.H{"time": tm, "level": lv, "source": src, "text": txt})
			}
			msgRows.Close()
		}
		var chatMsgs []gin.H
		chatRows, _ := database.Query(`SELECT COALESCE(time,''),COALESCE(sender,''),COALESCE(text,'')
			FROM chat_messages WHERE grp='inner' AND (project_id=? OR project_id IS NULL) ORDER BY id DESC LIMIT 16`, qProjID)
		if chatRows != nil {
			for chatRows.Next() {
				var tm, sender, txt string
				chatRows.Scan(&tm, &sender, &txt)
				chatMsgs = append(chatMsgs, gin.H{"time": tm, "level": "P2", "source": "院内会商·" + sender, "text": txt})
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

		c.JSON(http.StatusOK, gin.H{
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
		})
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
