package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// BootstrapHandler GET /api/bootstrap —— 聚合 DTO
func BootstrapHandler(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.JSON(200, gin.H{"serverTime": util.NowSec()})
			return
		}
		activeID := activeProjectID(database)
		projects := queryProjects(database, activeID)
		// fallback：如果 settings 没设 active_project，用第一个项目的 id
		if activeID == "" && len(projects) > 0 {
			if pid, ok := projects[0]["id"].(string); ok {
				activeID = pid
			}
		}
		c.JSON(200, gin.H{
			"serverTime":      util.NowSec(),
			"activeProjectId": activeID,
			"currentUser": gin.H{
				"id":       u.ID,
				"username": u.Username,
				"name":     u.Name,
				"org":      u.Org,
				"roleId":   u.Role.ID,
			},
			"roles":         queryRoles(database),
			"users":         queryUsers(database, u),
			"projects":      projects,
			"scenarios":     queryScenarios(database),
			"scriptNodes":   queryScriptNodes(database),
			"tasks":         queryTasks(database, u),
			"formTemplates": queryFormTemplates(database),
			"messages":      queryMessages(database),
			"notifs":        queryNotifs(database, u),
			"chat":          queryChat(database, u),
			"evidence":      queryEvidence(database, u),
			"snapshots":     querySnapshots(database),
			"timeline":      queryTimelineForBootstrap(database, u),
			"seqFlow":       querySeqFlowForBootstrap(database),
			"scoreItems":    queryScoreItems(database),
			"scores":        queryScores(database, activeID),
			"ledger":        queryLedger(database),
			"opinion":       queryOpinion(database),
			"knowledge":     queryKnowledge(database),
			"auditLogs":     queryAuditLogs(database, u),
			"impact":        queryImpact(database, activeID),
			"dict":          queryDictMap(database),
			"simData":       querySimDataMap(database),
		})
	}
}

// --- 辅助 ---

// nullInt 从 sql.NullInt64 取 int（值接收，0 表示无效）
func nullInt(n sql.NullInt64) int {
	if n.Valid {
		return int(n.Int64)
	}
	return 0
}

// nullInt64 返回 int64 （admin.go 也在用）
func nullInt64(n sql.NullInt64) int64 {
	if n.Valid {
		return n.Int64
	}
	return 0
}

// nullInt64Ptr 无效时返回 nil，否则返回 int
func nullInt64Ptr(n sql.NullInt64) any {
	if n.Valid {
		return int(n.Int64)
	}
	return nil
}

// scanNS 同 util.ScanNullableString（admin.go 也在用）
func scanNS(s sql.NullString) string { return util.ScanNullableString(s) }

// nullStr 同 scanNS（admin.go 也在用）
func nullStr(s sql.NullString) string { return util.ScanNullableString(s) }

// --- active project ---

func activeProjectID(database *db.DB) string {
	var v sql.NullString
	if err := database.QueryRow(
		`SELECT value FROM settings WHERE ` + "`key`" + `='active_project'`,
	).Scan(&v); err != nil {
		return ""
	}
	return util.ScanNullableString(v)
}

// resolveActiveProjectID 与 bootstrap 的 activeProjectId 解析保持一致：
// 优先 settings.active_project，为空时回退到首个项目。
func resolveActiveProjectID(database *db.DB) string {
	if id := activeProjectID(database); id != "" {
		return id
	}
	var id sql.NullString
	_ = database.QueryRow(`SELECT id FROM projects LIMIT 1`).Scan(&id)
	return util.ScanNullableString(id)
}

// --- roles ---

func queryRoles(database *db.DB) []gin.H {
	rows, err := database.Query(
		`SELECT id,name,short,icon,color,descr,nav,` + "`external`" + `,status
		 FROM roles ORDER BY seq`,
	)
	if err != nil {
		slog.Warn("queryRoles 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, short, icon, color, descr, status sql.NullString
		var navStr sql.NullString
		var ext sql.NullInt64
		if err := rows.Scan(&id, &name, &short, &icon, &color, &descr, &navStr, &ext, &status); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":       util.ScanNullableString(id),
			"name":     util.ScanNullableString(name),
			"short":    util.ScanNullableString(short),
			"icon":     util.ScanNullableString(icon),
			"color":    util.ScanNullableString(color),
			"desc":     util.ScanNullableString(descr),
			"nav":      util.ParseJSONArr(util.ScanNullableString(navStr)),
			"external": ext.Valid && ext.Int64 == 1,
			"status":   util.ScanNullableString(status),
		})
	}
	return out
}

// --- users ---

func queryUsers(database *db.DB, u *auth.User) []gin.H {
	if u == nil {
		return []gin.H{}
	}
	if u.Role.ID != auth.RolePlatformAdmin && u.Role.ID != auth.RoleCommander {
		return []gin.H{}
	}

	// 先查 user_roles + roles 构建 roleMap: userId -> [{id, name}]
	roleMap := make(map[int64][]gin.H)
	rrows, err := database.Query(
		`SELECT ur.user_id, r.id, r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id ORDER BY r.seq`,
	)
	if err == nil {
		for rrows.Next() {
			var uid int64
			var rid, rname sql.NullString
			if err := rrows.Scan(&uid, &rid, &rname); err != nil {
				continue
			}
			roleMap[uid] = append(roleMap[uid], gin.H{
				"id":   util.ScanNullableString(rid),
				"name": util.ScanNullableString(rname),
			})
		}
		rrows.Close()
	} else {
		slog.Warn("queryUsers roleMap 失败", "err", err)
	}

	rows, err := database.Query(
		`SELECT u.id,u.username,u.name,u.role_id,u.org,u.status,r.name AS role_name
		 FROM users u JOIN roles r ON r.id=u.role_id ORDER BY u.id`,
	)
	if err != nil {
		slog.Warn("queryUsers 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var username, name, roleID, org, status, roleName sql.NullString
		if err := rows.Scan(&id, &username, &name, &roleID, &org, &status, &roleName); err != nil {
			continue
		}
		rid := util.ScanNullableString(roleID)
		rn := util.ScanNullableString(roleName)
		roleList := roleMap[id]
		if len(roleList) == 0 {
			roleList = []gin.H{{"id": rid, "name": rn}}
		}
		rolesIDs := make([]string, 0, len(roleList))
		for _, r := range roleList {
			if x, ok := r["id"].(string); ok {
				rolesIDs = append(rolesIDs, x)
			}
		}
		out = append(out, gin.H{
			"id":       id,
			"username": util.ScanNullableString(username),
			"name":     util.ScanNullableString(name),
			"role":     rid,
			"roleName": rn,
			"roles":    rolesIDs,
			"roleList": roleList,
			"org":      util.ScanNullableString(org),
			"status":   util.ScanNullableString(status),
		})
	}
	return out
}

// --- projects ---

func queryProjects(database *db.DB, activeID string) []gin.H {
	rows, err := database.Query(`SELECT id,name,type,date,scenario_id,level,status,target,
		units,members,observers,mode,started_at,paused_at,paused_sec,ended_at FROM projects`)
	if err != nil {
		slog.Warn("queryProjects 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()

	// 先收集实战版项目 id 集合
	practicalIDs := map[string]bool{}
	pmRows, _ := database.Query(`SELECT id FROM projects WHERE mode='实战版'`)
	if pmRows != nil {
		for pmRows.Next() {
			var pid sql.NullString
			if pmRows.Scan(&pid) == nil {
				practicalIDs[util.ScanNullableString(pid)] = true
			}
		}
		pmRows.Close()
	}

	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, typ, date, scenarioID, level, status, target sql.NullString
		var unitsStr, membersStr, observersStr sql.NullString
		var mode, startedAt, pausedAt, endedAt sql.NullString
		var pausedSec sql.NullInt64
		if err := rows.Scan(&id, &name, &typ, &date, &scenarioID, &level, &status, &target,
			&unitsStr, &membersStr, &observersStr, &mode, &startedAt, &pausedAt, &pausedSec, &endedAt); err != nil {
			continue
		}
		statusV := util.ScanNullableString(status)
		modeV := util.ScanNullableString(mode)
		if modeV == "" {
			modeV = "基础版"
		}
		psec := int64(0)
		if pausedSec.Valid {
			psec = pausedSec.Int64
		}
		idStr := util.ScanNullableString(id)

		// 时间字段 NULL 或空字符串时返回 JSON null（Node 版 r.started_at || null）
		var startedAtVal any
		if startedAt.Valid && startedAt.String != "" {
			startedAtVal = startedAt.String
		}
		var pausedAtVal any
		if pausedAt.Valid && pausedAt.String != "" {
			pausedAtVal = pausedAt.String
		}
		var endedAtVal any
		if endedAt.Valid && endedAt.String != "" {
			endedAtVal = endedAt.String
		}

		out = append(out, gin.H{
			"id":         idStr,
			"name":       util.ScanNullableString(name),
			"type":       util.ScanNullableString(typ),
			"date":       util.ScanNullableString(date),
			"scenarioId": util.ScanNullableString(scenarioID),
			"level":      util.ScanNullableString(level),
			"status":     statusV,
			"target":     util.ScanNullableString(target),
			"units":      util.ParseJSONArrAny(util.ScanNullableString(unitsStr)),
			"members":    util.ParseJSONArrAny(util.ScanNullableString(membersStr)),
			"observers":  util.ParseJSONArrAny(util.ScanNullableString(observersStr)),
			"mode":       modeV,
			"startedAt":  startedAtVal,
			"pausedAt":   pausedAtVal,
			"pausedSec":  psec,
			"endedAt":    endedAtVal,
			"elapsedSec": util.ElapsedSec(util.ScanNullableString(startedAt), util.ScanNullableString(pausedAt), util.ScanNullableString(endedAt), psec, statusV),
			"progress":   projectProgress(database, idStr, util.ScanNullableString(scenarioID), modeV, statusV),
		})
	}
	return out
}

// projectProgress 与 Node src/routes/bootstrap.js projectProgress 完全对齐
func projectProgress(database *db.DB, projectID, scenarioID, mode, status string) int {
	if status == "已结束" || status == "已归档" {
		return 100
	}
	if status == "未开始" {
		return 0
	}
	if mode == "基础版" {
		var total, done sql.NullInt64
		_ = database.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='已完成' THEN 1 ELSE 0 END),0)
			 FROM tasks WHERE project_id=?`, projectID,
		).Scan(&total, &done)
		if !total.Valid || total.Int64 == 0 {
			return 0
		}
		return int(float64(done.Int64) / float64(total.Int64) * 100)
	}
	// 实战版
	if scenarioID == "" {
		return 0
	}
	var correct sql.NullInt64
	_ = database.QueryRow(
		`SELECT COUNT(*) FROM script_nodes WHERE scenario_id=? AND type!='end' AND COALESCE(flow_type,'correct')!='distractor'`,
		scenarioID,
	).Scan(&correct)
	if !correct.Valid || correct.Int64 == 0 {
		return 0
	}
	var done sql.NullInt64
	_ = database.QueryRow(
		`SELECT COUNT(DISTINCT ds.matched_node) FROM drill_steps ds
		 JOIN script_nodes n ON n.id=ds.matched_node AND n.scenario_id=?
		   AND COALESCE(n.flow_type,'correct')!='distractor' AND n.type!='end'
		 WHERE ds.project_id=?`,
		scenarioID, projectID,
	).Scan(&done)
	return int(float64(done.Int64) / float64(correct.Int64) * 100)
}

// --- scenarios ---

func queryScenarios(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,type,level,difficulty,status,version,updated,nodes,descr,alert FROM scenarios ORDER BY id DESC`)
	if err != nil {
		slog.Warn("queryScenarios 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, typ, level, difficulty, status, version, updated sql.NullString
		var nodes sql.NullInt64
		var descr, alert sql.NullString
		if err := rows.Scan(&id, &name, &typ, &level, &difficulty, &status, &version, &updated, &nodes, &descr, &alert); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":      util.ScanNullableString(id),
			"name":    util.ScanNullableString(name),
			"type":    util.ScanNullableString(typ),
			"level":   util.ScanNullableString(level),
			"status":  util.ScanNullableString(status),
			"version": util.ScanNullableString(version),
			"updated": util.ScanNullableString(updated),
			"nodes":   nullInt(nodes),
			"desc":    util.ScanNullableString(descr),
			"alert":   util.ScanNullableString(alert),
		})
	}
	return out
}

// --- script nodes ---

func queryScriptNodes(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,scenario_id,type,title,short_title,target,time,timeout,form,att,descr,
		branches,seq,guide,keywords,action,wrong,evidence_req,required,flow_type,score,receiver,deps,
		pos_x,pos_y,anchor_id,release_timing,parallel_desc FROM script_nodes ORDER BY scenario_id,seq`)
	if err != nil {
		slog.Warn("queryScriptNodes 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, scenarioID, typ, title, shortTitle, target, tm sql.NullString
		var timeout sql.NullInt64
		var form, att, descr sql.NullString
		var branchesStr, guide, keywords, action sql.NullString
		var wrongStr, evidenceReqStr sql.NullString
		var required sql.NullInt64
		var flowType sql.NullString
		var seq sql.NullInt64
		var score sql.NullInt64
		var receiver sql.NullString
		var depsStr sql.NullString
		var posX, posY sql.NullInt64
		var anchorID, releaseTiming, parallelDesc sql.NullString
		if err := rows.Scan(&id, &scenarioID, &typ, &title, &shortTitle, &target, &tm, &timeout, &form, &att, &descr,
			&branchesStr, &seq, &guide, &keywords, &action, &wrongStr, &evidenceReqStr, &required, &flowType, &score, &receiver, &depsStr,
			&posX, &posY, &anchorID, &releaseTiming, &parallelDesc); err != nil {
			continue
		}
		// flowType
		ft := util.ScanNullableString(flowType)
		flowTypeOut := "correct"
		if ft == "distractor" {
			flowTypeOut = "distractor"
		}
		// keywords split + filter empty
		kwIn := util.ScanNullableString(keywords)
		kwOut := []string{}
		if kwIn != "" {
			for _, k := range strings.Split(kwIn, ",") {
				k = strings.TrimSpace(k)
				if k != "" {
					kwOut = append(kwOut, k)
				}
			}
		}
		// required 默认 true（与 Node 版对齐：undefined → true）
		req := true
		if required.Valid {
			req = required.Int64 == 1
		}
		// Node 版 form/att/parallelDesc：r.form || undefined → JSON 里 undefined 被忽略
		// Go 版改用条件赋值：空字符串时不写入 key
		node := gin.H{
			"id":            util.ScanNullableString(id),
			"scenarioId":    util.ScanNullableString(scenarioID),
			"type":          util.ScanNullableString(typ),
			"title":         util.ScanNullableString(title),
			"target":        util.ScanNullableString(target),
			"timeout":       nullInt(timeout),
			"desc":          util.ScanNullableString(descr),
			"branches":      util.ParseJSONArrAny(util.ScanNullableString(branchesStr)),
			"guide":         util.ScanNullableString(guide),
			"keywords":      kwOut,
			"action":        util.ScanNullableString(action),
			"required":      req,
			"wrong":         util.ParseJSONArrAny(util.ScanNullableString(wrongStr)),
			"evidenceReq":   util.ParseJSONArrAny(util.ScanNullableString(evidenceReqStr)),
			"flowType":      flowTypeOut,
			"score":         nullInt(score),
			"receiver":      util.ScanNullableString(receiver),
			"deps":          util.ParseJSONArrAny(util.ScanNullableString(depsStr)),
			"posX":          nullInt64Ptr(posX),
			"posY":          nullInt64Ptr(posY),
			"anchorId":      util.ScanNullableString(anchorID),
			"releaseTiming": util.ScanNullableString(releaseTiming),
			"shortTitle":    util.ScanNullableString(shortTitle),
		}
		if f := util.ScanNullableString(form); f != "" {
			node["form"] = f
		}
		if a := util.ScanNullableString(att); a != "" {
			node["att"] = a
		}
		if pd := util.ScanNullableString(parallelDesc); pd != "" {
			node["parallelDesc"] = pd
		}
		out = append(out, node)
	}
	return out
}

// --- tasks ---

// MANAGED_ROLES 信息科负责人可统筹的角色
var managedRoles = map[string]bool{
	"itlead":   true,
	"duty":     true,
	"netsec":   true,
	"sysadmin": true,
}

// nodeAvailability 单节点前置可用性（Node: {available, blockedBy:[{id,title}]}）
type nodeAvailability struct {
	Available bool
	BlockedBy []gin.H
}

// basicNodeAvailability 复刻 Node src/drill_plan.js basicNodeAvailability：
// 基础版按剧本依赖解锁，前置全部完成才 available；end/混淆节点不在映射内。
func basicNodeAvailability(database *db.DB, projectID string) map[string]nodeAvailability {
	out := map[string]nodeAvailability{}
	var scenarioID sql.NullString
	if err := database.QueryRow(`SELECT scenario_id FROM projects WHERE id=?`, projectID).Scan(&scenarioID); err != nil {
		return out
	}
	sid := util.ScanNullableString(scenarioID)
	if sid == "" {
		return out
	}
	type availNode struct {
		id, title, typ, flow string
		deps                 []string
	}
	byID := map[string]*availNode{}
	order := make([]*availNode, 0)
	rows, err := database.Query(
		`SELECT id,COALESCE(title,''),COALESCE(deps,'[]'),COALESCE(flow_type,'correct'),COALESCE(type,'') FROM script_nodes WHERE scenario_id=?`, sid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, title, depsStr, flow, typ string
			if rows.Scan(&id, &title, &depsStr, &flow, &typ) != nil {
				continue
			}
			if strings.EqualFold(flow, "distractor") {
				flow = "distractor"
			} else {
				flow = "correct"
			}
			n := &availNode{id: id, title: title, typ: typ, flow: flow, deps: util.ParseJSONArr(depsStr)}
			byID[id] = n
			order = append(order, n)
		}
	} else {
		return out
	}

	done := map[string]bool{}
	drows, err := database.Query(
		`SELECT node_id FROM tasks WHERE project_id=? AND status='已完成' AND node_id IS NOT NULL AND node_id!=''`, projectID)
	if err == nil {
		defer drows.Close()
		for drows.Next() {
			var nid sql.NullString
			if drows.Scan(&nid) == nil {
				done[util.ScanNullableString(nid)] = true
			}
		}
	}

	for _, n := range order {
		if n.typ == "end" || n.flow == "distractor" {
			continue
		}
		blockedBy := make([]gin.H, 0)
		for _, depID := range n.deps {
			if dep, ok := byID[depID]; ok && !done[depID] {
				blockedBy = append(blockedBy, gin.H{"id": depID, "title": dep.title})
			}
		}
		out[n.id] = nodeAvailability{Available: len(blockedBy) == 0, BlockedBy: blockedBy}
	}
	return out
}

func queryTasks(database *db.DB, u *auth.User) []gin.H {
	if u == nil {
		return []gin.H{}
	}

	// 先查实战版项目 id 集合
	practicalIDs := map[string]bool{}
	pmRows, _ := database.Query(`SELECT id FROM projects WHERE mode='实战版'`)
	if pmRows != nil {
		for pmRows.Next() {
			var pid sql.NullString
			if pmRows.Scan(&pid) == nil {
				practicalIDs[util.ScanNullableString(pid)] = true
			}
		}
		pmRows.Close()
	}

	// 查任务附件 task_attachments
	type attRow struct {
		taskID, name, tp, size, url, by, filePath, time string
		auto                                            bool
	}
	attsByTask := map[string][]gin.H{}
	arows, _ := database.Query(
		`SELECT task_id,id,name,type,size,url,uploader,time,auto FROM task_attachments ORDER BY id`,
	)
	if arows != nil {
		for arows.Next() {
			var taskID, name, tp, size, url, uploader, tm sql.NullString
			var id sql.NullInt64
			var auto sql.NullInt64
			if arows.Scan(&taskID, &id, &name, &tp, &size, &url, &uploader, &tm, &auto) == nil {
				tid := util.ScanNullableString(taskID)
				attsByTask[tid] = append(attsByTask[tid], gin.H{
					"id":   nullInt(id),
					"name": util.ScanNullableString(name),
					"type": util.ScanNullableString(tp),
					"size": util.ScanNullableString(size),
					"url":  util.ScanNullableString(url),
					"by":   util.ScanNullableString(uploader),
					"time": util.ScanNullableString(tm),
					"auto": auto.Valid && auto.Int64 == 1,
				})
			}
		}
		arows.Close()
	}

	// 项目状态表（读取侧门控：仅「演练中」项目下发任务；写库仍按创建即生成）
	projectStatus := map[string]string{}
	if srows, serr := database.Query(`SELECT id,status FROM projects`); serr == nil {
		for srows.Next() {
			var pid, st sql.NullString
			if srows.Scan(&pid, &st) == nil {
				projectStatus[util.ScanNullableString(pid)] = util.ScanNullableString(st)
			}
		}
		srows.Close()
	}

	rows, err := database.Query(
		`SELECT id,project_id,role_id,title,descr,deadline,status,level,forms,node_id,seq,completed_at,accepted_at
		 FROM tasks ORDER BY seq`,
	)
	if err != nil {
		slog.Warn("queryTasks 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()

	rid := u.Role.ID
	isCmdr := auth.IsCommander(rid)
	isAdmin := u.Role.ID == auth.RolePlatformAdmin

	allTasks := make([]gin.H, 0)
	availByProject := map[string]map[string]nodeAvailability{}
	for rows.Next() {
		var id, projectID, roleID, title, descr, deadline, status, level sql.NullString
		var formsStr, nodeID sql.NullString
		var seq sql.NullInt64
		var completedAt, acceptedAt sql.NullString
		if err := rows.Scan(&id, &projectID, &roleID, &title, &descr, &deadline, &status, &level, &formsStr, &nodeID, &seq, &completedAt, &acceptedAt); err != nil {
			continue
		}
		pid := util.ScanNullableString(projectID)
		// 实战版项目的任务不发布
		if practicalIDs[pid] {
			continue
		}
		/* 状态门控（读取侧）：仅「演练中」项目下发任务；未开始/暂停/已结束/已归档一律返回空，
		 * 已提交记录仍可通过任务详情接口按 id 读取（写库行为不变） */
		pst := projectStatus[pid]
		if pst != "演练中" {
			continue
		}
		taskRole := util.ScanNullableString(roleID)
		nid := util.ScanNullableString(nodeID)
		av := nodeAvailability{Available: true, BlockedBy: []gin.H{}}
		if nid != "" {
			if _, ok := availByProject[pid]; !ok {
				availByProject[pid] = basicNodeAvailability(database, pid)
			}
			if a, ok := availByProject[pid][nid]; ok {
				av = a
			}
		}
		allTasks = append(allTasks, gin.H{
			"id":            util.ScanNullableString(id),
			"projectId":     pid,
			"role":          taskRole,
			"title":         util.ScanNullableString(title),
			"desc":          util.ScanNullableString(descr),
			"deadline":      util.ScanNullableString(deadline),
			"status":        util.ScanNullableString(status),
			"level":         util.ScanNullableString(level),
			"nodeId":        nid,
			"available":     av.Available,
			"blockedBy":     av.BlockedBy,
			"completedAt":   util.ScanNullableString(completedAt),
			"projectStatus": pst,
			"forms":         util.ParseJSONArr(util.ScanNullableString(formsStr)),
			"atts":          attsByTask[util.ScanNullableString(id)],
			"_taskRole":     taskRole, // 内部字段，用于过滤后删除
		})
	}

	// commander / admin 看全部
	if isCmdr || isAdmin {
		for i := range allTasks {
			delete(allTasks[i], "_taskRole")
		}
		return allTasks
	}
	// 其他角色过滤
	out := make([]gin.H, 0, len(allTasks))
	for _, t := range allTasks {
		tr, _ := t["_taskRole"].(string)
		var keep bool
		if rid == "itlead" {
			keep = managedRoles[tr]
		} else {
			keep = tr == rid
		}
		if keep {
			delete(t, "_taskRole")
			out = append(out, t)
		}
	}
	return out
}

// --- form templates ---

func queryFormTemplates(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,fields FROM form_templates ORDER BY id`)
	if err != nil {
		slog.Warn("queryFormTemplates 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	// mode：fill（填写）/ review（审议）。列不存在时（旧库未迁移）静默降级为 fill。
	modes := map[string]string{}
	if mrows, merr := database.Query(`SELECT id,COALESCE(mode,'') FROM form_templates`); merr == nil {
		for mrows.Next() {
			var mid, mmode sql.NullString
			if mrows.Scan(&mid, &mmode) == nil {
				modes[util.ScanNullableString(mid)] = util.ScanNullableString(mmode)
			}
		}
		mrows.Close()
	}
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name sql.NullString
		var fieldsStr sql.NullString
		if err := rows.Scan(&id, &name, &fieldsStr); err != nil {
			continue
		}
		fid := util.ScanNullableString(id)
		fields := normalizeFields(util.ParseJSONArrAny(util.ScanNullableString(fieldsStr)))
		mode := modes[fid]
		if mode == "" {
			mode = "fill"
		}
		out = append(out, gin.H{
			"id":     fid,
			"name":   util.ScanNullableString(name),
			"fields": fields,
			"mode":   mode,
		})
	}
	return out
}

// normalizeFields — 与 Node src/forms.js normalizeFields 严格对齐
// 规则：
//   - 字符串 → { label, hint:”, required:true, type:'auto', options:[] }
//   - 对象 → 清洗为 { label, hint, required, type, options }，字段齐全
//   - 白名单 type：auto|text|textarea|select|multiselect|datetime，其他 → auto
//   - select 无 options 自动降级成 text
//   - 所有输出字段都强制有 options: []
func normalizeFields(raw []any) []any {
	out := make([]any, 0, len(raw))
	for _, fl := range raw {
		if fl == nil {
			continue
		}
		switch v := fl.(type) {
		case string:
			label := clipStr(v, 40)
			if label == "" {
				continue
			}
			out = append(out, map[string]any{
				"label":    label,
				"hint":     "",
				"required": true,
				"type":     "auto",
				"options":  []any{},
			})
		case map[string]any:
			labelRaw := ""
			if x, ok := v["label"].(string); ok {
				labelRaw = x
			} else if x, ok := v["name"].(string); ok {
				labelRaw = x
			}
			label := clipStr(labelRaw, 40)
			if label == "" {
				continue
			}
			// type 白名单
			typeStr := "auto"
			if x, ok := v["type"].(string); ok {
				if _, ok := formTypeWhitelist[x]; ok {
					typeStr = x
				}
			}
			// options: parse JSON or string → []any
			opts := parseOptions(v["options"])
			// select 无 options 降级成 text
			if typeStr == "select" && len(opts) == 0 {
				typeStr = "text"
			}
			// required 默认 true
			required := true
			if x, ok := v["required"].(bool); ok {
				required = x
			}
			// hint: hint 或 desc
			hint := ""
			if x, ok := v["hint"].(string); ok {
				hint = clipStr(x, 200)
			} else if x, ok := v["desc"].(string); ok {
				hint = clipStr(x, 200)
			}
			// 只有 select/multiselect 才有非空 options（否则强制 []）
			hasOpts := typeStr == "select" || typeStr == "multiselect"
			var finalOpts []any
			if hasOpts {
				finalOpts = opts
			} else {
				finalOpts = []any{}
			}
			out = append(out, map[string]any{
				"label":    label,
				"hint":     hint,
				"required": required,
				"type":     typeStr,
				"options":  finalOpts,
			})
		default:
			continue
		}
	}
	return out
}

var formTypeWhitelist = map[string]bool{
	"auto":        true,
	"text":        true,
	"textarea":    true,
	"select":      true,
	"multiselect": true,
	"datetime":    true,
}

func clipStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// parseOptions 与 Node parseOptions 对齐：
//   - []any → trim 每个字符串，最多 30 个
//   - string → 按 [、,，|｜] 分隔
//   - nil / 其他 → []
func parseOptions(v any) []any {
	switch x := v.(type) {
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			s := ""
			switch t := item.(type) {
			case string:
				s = t
			default:
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if len(s) > 20 {
				s = s[:20]
			}
			out = append(out, s)
			if len(out) >= 30 {
				break
			}
		}
		return out
	case string:
		parts := strings.FieldsFunc(x, func(r rune) bool {
			return r == '、' || r == ',' || r == '，' || r == '|' || r == '｜'
		})
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if len(p) > 20 {
				p = p[:20]
			}
			out = append(out, p)
			if len(out) >= 30 {
				break
			}
		}
		return out
	default:
		return []any{}
	}
}

// --- messages ---

func queryMessages(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,time,level,source,text,targets,seq,project_id FROM messages ORDER BY seq`)
	if err != nil {
		slog.Warn("queryMessages 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, tm, level, source, text sql.NullString
		var targetsStr sql.NullString
		var seq sql.NullInt64
		var projectID sql.NullString
		if err := rows.Scan(&id, &tm, &level, &source, &text, &targetsStr, &seq, &projectID); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":        util.ScanNullableString(id),
			"time":      util.ScanNullableString(tm),
			"level":     util.ScanNullableString(level),
			"source":    util.ScanNullableString(source),
			"text":      util.ScanNullableString(text),
			"targets":   util.ParseJSONArr(util.ScanNullableString(targetsStr)),
			"seq":       nullInt(seq),
			"projectId": util.ScanNullableString(projectID),
		})
	}
	return out
}

// --- notifs ---

func queryNotifs(database *db.DB, u *auth.User) []gin.H {
	// 先查当前用户已读通知集合
	readSet := map[string]bool{}
	if u != nil {
		rr, err := database.Query(`SELECT notif_id FROM notif_reads WHERE user_id=?`, u.ID)
		if err == nil {
			for rr.Next() {
				var nid sql.NullString
				if rr.Scan(&nid) == nil {
					readSet[util.ScanNullableString(nid)] = true
				}
			}
			rr.Close()
		}
	}

	isExt := u != nil && auth.IsExternal(u.Role.ID)

	var rows *sql.Rows
	var err error
	if isExt {
		rows, err = database.Query(`SELECT id,time,level,source,title,descr,seq,project_id FROM notifs WHERE target!='internal' ORDER BY seq`)
	} else {
		rows, err = database.Query(`SELECT id,time,level,source,title,descr,seq,project_id FROM notifs ORDER BY seq`)
	}
	if err != nil {
		slog.Warn("queryNotifs 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, tm, level, source, title, descr, projectID sql.NullString
		var seq sql.NullInt64
		if err := rows.Scan(&id, &tm, &level, &source, &title, &descr, &seq, &projectID); err != nil {
			continue
		}
		nid := util.ScanNullableString(id)
		out = append(out, gin.H{
			"id":        nid,
			"time":      util.ScanNullableString(tm),
			"level":     util.ScanNullableString(level),
			"source":    util.ScanNullableString(source),
			"title":     util.ScanNullableString(title),
			"desc":      util.ScanNullableString(descr),
			"read":      readSet[nid],
			"projectId": util.ScanNullableString(projectID),
		})
	}
	return out
}

// --- chat ---

// chatRoleAccess 导调中心频道权限（对齐 Node 版）
var chatRoleAccess = map[string][]string{
	"duty":         {"inner"},
	"netsec":       {"inner"},
	"sysadmin":     {"inner"},
	"medlead":      {"inner"},
	"clinical":     {"inner"},
	"healthbureau": {"external"},
	"cac":          {"external"},
	"cyberpolice":  {"external"},
	"itlead":       {"inner", "external"},
	"commander":    {"inner", "external"},
}

func queryChat(database *db.DB, u *auth.User) gin.H {
	result := gin.H{}
	if u == nil {
		return result
	}
	allowed := chatRoleAccess[u.Role.ID]
	if len(allowed) == 0 {
		return result
	}
	allowMap := map[string]bool{}
	for _, a := range allowed {
		allowMap[a] = true
	}

	rows, err := database.Query(`SELECT id,grp,sender,role_id,user_id,time,text,att,file_path,project_id FROM chat_messages ORDER BY id`)
	if err != nil {
		slog.Warn("queryChat 失败", "err", err)
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var id, userID sql.NullInt64
		var grp, sender, roleID, tm, text, att, filePath, projectID sql.NullString
		if err := rows.Scan(&id, &grp, &sender, &roleID, &userID, &tm, &text, &att, &filePath, &projectID); err != nil {
			continue
		}
		g := util.ScanNullableString(grp)
		if !allowMap[g] {
			continue
		}
		fp := util.ScanNullableString(filePath)
		var url any
		if fp != "" {
			url = "/uploads/" + fp
		}
		item := gin.H{
			"id":     nullInt(id),
			"sender": util.ScanNullableString(sender),
			"role":   util.ScanNullableString(roleID),
			"time":   util.ScanNullableString(tm),
			"text":   util.ScanNullableString(text),
			"att":    util.ScanNullableString(att),
			"url":    url,
		}
		if g == "inner" {
			inner, _ := result["inner"].([]gin.H)
			result["inner"] = append(inner, item)
		} else if g == "external" {
			external, _ := result["external"].([]gin.H)
			result["external"] = append(external, item)
		}
	}
	return result
}

// --- evidence ---

var externalEvidenceTypes = []string{"表单文书", "上报材料", "报案材料"}
var externalEvidenceRoles = map[string]bool{"itlead": true, "commander": true}

func queryEvidence(database *db.DB, u *auth.User) []gin.H {
	rows, err := database.Query(`SELECT id,type,name,size,time,role_id,uploader,auto,task_id,project_id,note,file_path,created_at FROM evidence ORDER BY created_at,id`)
	if err != nil {
		slog.Warn("queryEvidence 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()

	isExt := u != nil && auth.IsExternal(u.Role.ID)
	allowedTypes := map[string]bool{}
	for _, t := range externalEvidenceTypes {
		allowedTypes[t] = true
	}

	out := make([]gin.H, 0)
	for rows.Next() {
		var id, tp, name, size, tm, roleID, uploader sql.NullString
		var auto sql.NullInt64
		var taskID, projectID, note, filePath, createdAt sql.NullString
		if err := rows.Scan(&id, &tp, &name, &size, &tm, &roleID, &uploader, &auto, &taskID, &projectID, &note, &filePath, &createdAt); err != nil {
			continue
		}
		typeV := util.ScanNullableString(tp)
		roleV := util.ScanNullableString(roleID)
		// 外部单位过滤
		if isExt {
			if !allowedTypes[typeV] && !externalEvidenceRoles[roleV] {
				continue
			}
		}
		fp := util.ScanNullableString(filePath)
		var url any
		if fp != "" {
			url = "/uploads/" + fp
		}
		out = append(out, gin.H{
			"id":        util.ScanNullableString(id),
			"type":      typeV,
			"name":      util.ScanNullableString(name),
			"size":      util.ScanNullableString(size),
			"time":      util.ScanNullableString(tm),
			"role":      roleV,
			"uploader":  util.ScanNullableString(uploader),
			"auto":      auto.Valid && auto.Int64 == 1,
			"taskId":    util.ScanNullableString(taskID),
			"projectId": util.ScanNullableString(projectID),
			"note":      util.ScanNullableString(note),
			"hasFile":   fp != "",
			"url":       url,
		})
	}
	return out
}

// --- snapshots ---

func querySnapshots(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,time,by_user,note FROM snapshots ORDER BY id`)
	if err != nil {
		slog.Warn("querySnapshots 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, tm, byUser, note sql.NullString
		if err := rows.Scan(&id, &name, &tm, &byUser, &note); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":   util.ScanNullableString(id),
			"name": util.ScanNullableString(name),
			"time": util.ScanNullableString(tm),
			"by":   util.ScanNullableString(byUser),
			"note": util.ScanNullableString(note),
		})
	}
	return out
}

// --- timeline ---

func queryTimeline(database *db.DB, u *auth.User) []gin.H {
	rows, err := database.Query(`SELECT t,actor,role_id,action,detail,level FROM timeline ORDER BY seq`)
	if err != nil {
		slog.Warn("queryTimeline 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	isExt := u != nil && auth.IsExternal(u.Role.ID)
	out := make([]gin.H, 0)
	for rows.Next() {
		var tm, actor, roleID, action, detail, level sql.NullString
		if err := rows.Scan(&tm, &actor, &roleID, &action, &detail, &level); err != nil {
			continue
		}
		actionV := util.ScanNullableString(action)
		if isExt {
			if !containsTimelineKeyword(actionV) {
				continue
			}
		}
		out = append(out, gin.H{
			"t":      util.ScanNullableString(tm),
			"actor":  util.ScanNullableString(actor),
			"role":   util.ScanNullableString(roleID),
			"action": actionV,
			"detail": util.ScanNullableString(detail),
			"level":  util.ScanNullableString(level),
		})
	}
	return out
}

// queryTimelineForBootstrap — bootstrap 用，带 projectId 字段
func queryTimelineForBootstrap(database *db.DB, u *auth.User) []gin.H {
	rows, err := database.Query(`SELECT project_id,t,actor,role_id,action,detail,level FROM timeline ORDER BY seq`)
	if err != nil {
		slog.Warn("queryTimelineForBootstrap 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	isExt := u != nil && auth.IsExternal(u.Role.ID)
	out := make([]gin.H, 0)
	for rows.Next() {
		var projectID, tm, actor, roleID, action, detail, level sql.NullString
		if err := rows.Scan(&projectID, &tm, &actor, &roleID, &action, &detail, &level); err != nil {
			continue
		}
		actionV := util.ScanNullableString(action)
		if isExt {
			if !containsTimelineKeyword(actionV) {
				continue
			}
		}
		out = append(out, gin.H{
			"projectId": util.ScanNullableString(projectID),
			"t":         util.ScanNullableString(tm),
			"actor":     util.ScanNullableString(actor),
			"role":      util.ScanNullableString(roleID),
			"action":    actionV,
			"detail":    util.ScanNullableString(detail),
			"level":     util.ScanNullableString(level),
		})
	}
	return out
}

func containsTimelineKeyword(s string) bool {
	if s == "" {
		return false
	}
	kw := []string{"上报", "报送", "接收", "督办", "报案", "审批"}
	for _, k := range kw {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// --- seq flow ---

func querySeqFlow(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT name,time,role,state FROM seq_flow ORDER BY seq`)
	if err != nil {
		slog.Warn("querySeqFlow 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var name, tm, role, state sql.NullString
		if err := rows.Scan(&name, &tm, &role, &state); err != nil {
			continue
		}
		out = append(out, gin.H{
			"name":  util.ScanNullableString(name),
			"time":  util.ScanNullableString(tm),
			"role":  util.ScanNullableString(role),
			"state": util.ScanNullableString(state),
		})
	}
	return out
}

// querySeqFlowForBootstrap — bootstrap 用，带 projectId 字段
func querySeqFlowForBootstrap(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT project_id,name,time,role,state FROM seq_flow ORDER BY seq`)
	if err != nil {
		slog.Warn("querySeqFlowForBootstrap 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var projectID, name, tm, role, state sql.NullString
		if err := rows.Scan(&projectID, &name, &tm, &role, &state); err != nil {
			continue
		}
		out = append(out, gin.H{
			"projectId": util.ScanNullableString(projectID),
			"name":      util.ScanNullableString(name),
			"time":      util.ScanNullableString(tm),
			"role":      util.ScanNullableString(role),
			"state":     util.ScanNullableString(state),
		})
	}
	return out
}

// --- score items ---

func queryScoreItems(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,weight,rule FROM score_items ORDER BY seq`)
	if err != nil {
		slog.Warn("queryScoreItems 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, rule sql.NullString
		var weight sql.NullInt64
		if err := rows.Scan(&id, &name, &weight, &rule); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":     util.ScanNullableString(id),
			"name":   util.ScanNullableString(name),
			"weight": nullInt(weight),
			"rule":   util.ScanNullableString(rule),
		})
	}
	return out
}

// --- scores ---

func queryScores(database *db.DB, activeID string) []gin.H {
	if activeID == "" {
		return []gin.H{}
	}
	rows, err := database.Query(`SELECT item_id,score,max,note,auto FROM scores WHERE project_id=? ORDER BY item_id`, activeID)
	if err != nil {
		slog.Warn("queryScores 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var itemID, note sql.NullString
		var score sql.NullFloat64
		var maxN sql.NullInt64
		var auto sql.NullInt64
		if err := rows.Scan(&itemID, &score, &maxN, &note, &auto); err != nil {
			continue
		}
		var scoreV float64
		if score.Valid {
			scoreV = score.Float64
		}
		out = append(out, gin.H{
			"itemId": util.ScanNullableString(itemID),
			"score":  scoreV,
			"max":    nullInt(maxN),
			"note":   util.ScanNullableString(note),
			"auto":   auto.Valid && auto.Int64 == 1,
		})
	}
	return out
}

// --- ledger ---

func queryLedger(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,date,type,event,level,result,score,org FROM ledger ORDER BY id`)
	if err != nil {
		slog.Warn("queryLedger 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, date, typ, event, level, result, score, org sql.NullString
		if err := rows.Scan(&id, &name, &date, &typ, &event, &level, &result, &score, &org); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":     util.ScanNullableString(id),
			"name":   util.ScanNullableString(name),
			"date":   util.ScanNullableString(date),
			"type":   util.ScanNullableString(typ),
			"event":  util.ScanNullableString(event),
			"level":  util.ScanNullableString(level),
			"result": util.ScanNullableString(result),
			"score":  util.ScanNullableString(score),
			"org":    util.ScanNullableString(org),
		})
	}
	return out
}

// --- opinion ---

func queryOpinion(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,time,channel,title,heat,risk,status,source FROM opinion ORDER BY id`)
	if err != nil {
		slog.Warn("queryOpinion 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, tm, channel, title, heat, risk, status, source sql.NullString
		if err := rows.Scan(&id, &tm, &channel, &title, &heat, &risk, &status, &source); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":      util.ScanNullableString(id),
			"time":    util.ScanNullableString(tm),
			"channel": util.ScanNullableString(channel),
			"title":   util.ScanNullableString(title),
			"heat":    util.ScanNullableString(heat),
			"risk":    util.ScanNullableString(risk),
			"status":  util.ScanNullableString(status),
			"source":  util.ScanNullableString(source),
		})
	}
	return out
}

// --- knowledge ---

func queryKnowledge(database *db.DB) []gin.H {
	rows, err := database.Query(`SELECT id,name,cat,updated,size,pages,content,file_path FROM knowledge ORDER BY id`)
	if err != nil {
		slog.Warn("queryKnowledge 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, name, cat, updated, size, content, filePath sql.NullString
		var pages sql.NullInt64
		if err := rows.Scan(&id, &name, &cat, &updated, &size, &pages, &content, &filePath); err != nil {
			continue
		}
		fp := util.ScanNullableString(filePath)
		var url any
		if fp != "" {
			url = "/uploads/" + fp
		}
		var filePathVal any
		if fp != "" {
			filePathVal = fp
		}
		out = append(out, gin.H{
			"id":       util.ScanNullableString(id),
			"name":     util.ScanNullableString(name),
			"cat":      util.ScanNullableString(cat),
			"updated":  util.ScanNullableString(updated),
			"size":     util.ScanNullableString(size),
			"pages":    nullInt(pages),
			"content":  util.ScanNullableString(content),
			"filePath": filePathVal,
			"url":      url,
		})
	}
	return out
}

// --- audit logs ---

func queryAuditLogs(database *db.DB, u *auth.User) []gin.H {
	if u == nil || u.Role.ID != auth.RolePlatformAdmin {
		return []gin.H{}
	}
	rows, err := database.Query(`SELECT time,user,role_id,action,ip FROM audit_logs ORDER BY id DESC LIMIT 500`)
	if err != nil {
		slog.Warn("queryAuditLogs 失败", "err", err)
		return []gin.H{}
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var tm, user, roleID, action, ip sql.NullString
		if err := rows.Scan(&tm, &user, &roleID, &action, &ip); err != nil {
			continue
		}
		out = append(out, gin.H{
			"time":   util.ScanNullableString(tm),
			"user":   util.ScanNullableString(user),
			"role":   util.ScanNullableString(roleID),
			"action": util.ScanNullableString(action),
			"ip":     util.ScanNullableString(ip),
		})
	}
	return out
}

// --- impact ---

// queryImpact 业务系统影响态势（对齐 Node /api/content/impact 口径）：
// 项目实例行优先，项目缺失的系统再回退全局模板行，最后用基线补齐。
// 实现统一收敛到 impactSnapshot，避免与大屏 monitor 的算法分叉。
func queryImpact(database *db.DB, projectID string) []gin.H {
	return impactSnapshot(database, projectID)
}

// --- dict map ---

func queryDictMap(database *db.DB) map[string]any {
	rows, err := database.Query(`SELECT ` + "`key`" + `, value FROM dict`)
	if err != nil {
		slog.Warn("queryDictMap 失败", "err", err)
		return map[string]any{}
	}
	defer rows.Close()
	out := make(map[string]any)
	for rows.Next() {
		var k, v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		key := util.ScanNullableString(k)
		val := util.ScanNullableString(v)
		// 默认数组
		var arr []any
		if val == "" {
			out[key] = arr
			continue
		}
		if jerr := json.Unmarshal([]byte(val), &arr); jerr == nil {
			out[key] = arr
			continue
		}
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(val), &obj); jerr == nil {
			out[key] = obj
			continue
		}
		out[key] = val
	}
	return out
}

// --- sim data map ---

func querySimDataMap(database *db.DB) map[string]any {
	rows, err := database.Query(`SELECT ` + "`type`" + `, data FROM sim_data`)
	if err != nil {
		slog.Warn("querySimDataMap 失败", "err", err)
		return map[string]any{}
	}
	defer rows.Close()
	out := make(map[string]any)
	for rows.Next() {
		var k, v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		key := util.ScanNullableString(k)
		val := util.ScanNullableString(v)
		if val == "" {
			out[key] = map[string]any{}
			continue
		}
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(val), &obj); jerr == nil {
			out[key] = obj
			continue
		}
		var arr []any
		if jerr := json.Unmarshal([]byte(val), &arr); jerr == nil {
			out[key] = arr
			continue
		}
		out[key] = val
	}
	return out
}
