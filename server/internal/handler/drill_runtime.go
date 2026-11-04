package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ========== 演练计时工具 ==========

func hhmm() string {
	return time.Now().Format("15:04")
}

func nowSec() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func formatElapsed(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// elapsedSec 复刻 src/drill_clock.js:11
// 未启动返回 nil；暂停取暂停时刻；结束取结束时刻；扣除累计暂停
func elapsedSecLite(startedAt, pausedAt string, pausedSec int, endedAt string, status string) *int64 {
	if startedAt == "" {
		return nil
	}
	start := parseLocalTime(startedAt)
	if start == 0 {
		return nil
	}
	var end int64
	if endedAt != "" {
		end = parseLocalTime(endedAt)
	} else if pausedAt != "" && status == "暂停" {
		end = parseLocalTime(pausedAt)
	} else {
		end = time.Now().UnixMilli()
	}
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	sec := int64((end-start)/1000) - int64(pausedSec)
	if sec < 0 {
		sec = 0
	}
	return &sec
}

// ========== POST /api/drill/projects/:id/start ==========
func StartProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var status, name, level, mode string
		err := database.QueryRow(`SELECT COALESCE(status,''),COALESCE(name,''),COALESCE(level,''),COALESCE(mode,'基础版') FROM projects WHERE id=?`, id).Scan(&status, &name, &level, &mode)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询项目失败"})
			return
		}
		if status != "未开始" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + status + "，不能重复开始"})
			return
		}

		now := nowSec()
		_, err = database.Exec(`UPDATE projects SET status='演练中', started_at=? WHERE id=?`, now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开始演练失败：" + err.Error()})
			return
		}

		taskStatusUpdate(database, id, "待处理", "")

		// 站内消息：演练开始（Node bootstrap.js:333-345）
		pLevel := level
		if pLevel == "" {
			pLevel = "较大"
		}
		nLevel := "P1"
		if level == "重大" {
			nLevel = "P0"
		}
		pushNotif(database, "drill_start", map[string]any{
			"action": "演练开始", "project": name, "resumed": "已启动", "mode": mode, "level": pLevel,
		}, id, nLevel, "", "internal")

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, hhmm(), userNameFromContext(c), roleIDFromContext(c), "开始演练", "演练项目已启动", "", nil)

		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "startedAt": now})
	}
}

func taskStatusUpdate(database *db.DB, projectID, from, to string) {
	_, _ = database.Exec(`UPDATE tasks SET status=? WHERE project_id=? AND status=?`, to, projectID, from)
}

func roleIDFromContext(c *gin.Context) string {
	if v, ok := c.Get("role_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	u := auth.CurrentUser(c)
	if u != nil {
		return u.Role.ID
	}
	return ""
}

// ========== PATCH /api/drill/projects/:id/pause ==========
func PauseProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var status, startedAt string
		var pausedSec int
		err := database.QueryRow(`SELECT status,COALESCE(started_at,''),COALESCE(paused_sec,0) FROM projects WHERE id=?`, id).Scan(&status, &startedAt, &pausedSec)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status != "演练中" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + status + "，不能暂停"})
			return
		}

		now := nowSec()
		_, err = database.Exec(`UPDATE projects SET status='暂停', paused_at=? WHERE id=?`, now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "暂停失败"})
			return
		}

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, hhmm(), userNameFromContext(c), roleIDFromContext(c), "暂停演练", "演练已暂停", "", nil)

		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "pausedAt": now})
	}
}

// ========== PATCH /api/drill/projects/:id/resume ==========
func ResumeProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var status, startedAt, pausedAt string
		var pausedSec int
		var name, level, mode string
		err := database.QueryRow(`SELECT status,COALESCE(started_at,''),COALESCE(paused_at,''),COALESCE(paused_sec,0),COALESCE(name,''),COALESCE(level,''),COALESCE(mode,'基础版') FROM projects WHERE id=?`, id).Scan(&status, &startedAt, &pausedAt, &pausedSec, &name, &level, &mode)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status != "暂停" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + status + "，不能继续"})
			return
		}

		// 计算本次暂停时长（秒）
		var pauseSec int
		if pausedAt != "" {
			nowMs := time.Now().UnixMilli()
			pauseMs := parseLocalTime(pausedAt)
			if pauseMs > 0 && nowMs > pauseMs {
				pauseSec = int((nowMs - pauseMs) / 1000)
			}
		}
		newPausedSec := pausedSec + pauseSec

		_, err = database.Exec(`UPDATE projects SET status='演练中', paused_at='', paused_sec=? WHERE id=?`, newPausedSec, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "继续失败"})
			return
		}

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, hhmm(), userNameFromContext(c), roleIDFromContext(c), "继续演练",
			fmt.Sprintf("继续演练（暂停 %d 秒）", pauseSec), "", nil)

		// 站内消息：演练继续（Node bootstrap.js:333-345）
		pLevel := level
		if pLevel == "" {
			pLevel = "较大"
		}
		nLevel := "P1"
		if level == "重大" {
			nLevel = "P0"
		}
		pushNotif(database, "drill_start", map[string]any{
			"action": "演练继续", "project": name, "resumed": "已恢复", "mode": mode, "level": pLevel,
		}, id, nLevel, "", "internal")

		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "pausedSec": newPausedSec})
	}
}

// ========== POST /api/drill/projects/:id/end ==========
func EndProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var status, mode, name string
		err := database.QueryRow(`SELECT status,COALESCE(mode,'基础版'),COALESCE(name,'') FROM projects WHERE id=?`, id).Scan(&status, &mode, &name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		if status == "已结束" || status == "已归档" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练已结束"})
			return
		}

		now := nowSec()
		_, err = database.Exec(`UPDATE projects SET status='已结束', ended_at=? WHERE id=?`, now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "结束演练失败"})
			return
		}

		// 站内消息：演练结束（Node bootstrap.js:346-350；finish 路径推 drill_end_score，此处推 drill_end）
		pushNotif(database, "drill_end", map[string]any{
			"project": name, "mode": mode,
		}, id, "", "", "internal")

		// 自动评分
		sc := Score(database, id)

		// 写入主评分项
		if sc != nil {
			_, _ = database.Exec(`INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE score=VALUES(score),max=VALUES(max),note=VALUES(note),updated_at=VALUES(updated_at)`,
				"MODE", id, sc.Score, 100, sc.Formula, 1, nowSec())
		}

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, hhmm(), userNameFromContext(c), roleIDFromContext(c), "演练结束",
			fmt.Sprintf("演练结束，最终得分 %.1f 分", func() float64 { if sc != nil { return sc.Score }; return 0 }()),
			"P1", nil)

		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "endedAt": now, "score": sc})
	}
}

// ========== GET /api/drill/projects/:id/tasks — 任务中心 ==========
func GetProjectTasks(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var scenarioID, mode string
		database.QueryRow(`SELECT COALESCE(scenario_id,''),COALESCE(mode,'基础版') FROM projects WHERE id=?`, id).Scan(&scenarioID, &mode)

		rows, err := database.Query(`SELECT id,COALESCE(role_id,''),COALESCE(title,''),COALESCE(`+"`descr`"+`,''),COALESCE(deadline,''),COALESCE(status,''),COALESCE(level,''),COALESCE(forms,'[]'),COALESCE(node_id,''),seq,
			COALESCE(completed_at,''),COALESCE(accepted_at,'')
			FROM tasks WHERE project_id=? ORDER BY seq`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
			return
		}
		defer rows.Close()

		var tasks []gin.H
		for rows.Next() {
			var tid, roleID, title, descr, deadline, status, level, forms, nodeID, completedAt, acceptedAt string
			var seq int
			rows.Scan(&tid, &roleID, &title, &descr, &deadline, &status, &level, &forms, &nodeID, &seq, &completedAt, &acceptedAt)
			var formsArr []string
			json.Unmarshal([]byte(forms), &formsArr)
			tasks = append(tasks, gin.H{
				"id":          tid,
				"roleId":      roleID,
				"title":       title,
				"descr":       descr,
				"deadline":    deadline,
				"status":      status,
				"level":       level,
				"forms":       formsArr,
				"nodeId":      nodeID,
				"seq":         seq,
				"completedAt": completedAt,
				"acceptedAt":  acceptedAt,
			})
		}

		// 可用性：检查 deps 是否全部完成
		type nodeInfo struct {
			deps []string
			flowType string
		}
		nodeMap := make(map[string]nodeInfo)
		nodeRows, _ := database.Query(`SELECT id,COALESCE(deps,'[]'),COALESCE(flow_type,'correct') FROM script_nodes WHERE scenario_id=?`, scenarioID)
		defer nodeRows.Close()
		for nodeRows.Next() {
			var nid, depsStr, ft string
			nodeRows.Scan(&nid, &depsStr, &ft)
			var deps []string
			json.Unmarshal([]byte(depsStr), &deps)
			nodeMap[nid] = nodeInfo{deps: deps, flowType: ft}
		}
		doneSet := make(map[string]bool)
		for _, t := range tasks {
			if t["status"] == "已完成" {
				nid, _ := t["nodeId"].(string)
				if nid != "" {
					doneSet[nid] = true
				}
			}
		}

		tasksWithAvail := []gin.H{}
		for _, t := range tasks {
			nid, _ := t["nodeId"].(string)
			taskStatus, _ := t["status"].(string)
			available := true
			var blockedBy []gin.H
			// 已锁定任务不出现在待办可执行列表
			if taskStatus == "已锁定" {
				available = false
				blockedBy = append(blockedBy, gin.H{"id": "LOCKED", "title": "任务已锁定"})
			}
			if available && nid != "" {
				if ni, ok := nodeMap[nid]; ok && ni.flowType == "correct" {
					for _, depID := range ni.deps {
						if !doneSet[depID] {
							available = false
							var depTitle string
							database.QueryRow(`SELECT title FROM script_nodes WHERE id=?`, depID).Scan(&depTitle)
							blockedBy = append(blockedBy, gin.H{"id": depID, "title": depTitle})
						}
					}
				}
			}
			t["available"] = available
			t["blockedBy"] = blockedBy
			tasksWithAvail = append(tasksWithAvail, t)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "mode": mode, "tasks": tasksWithAvail})
	}
}

// ========== PATCH /api/drill/tasks/:id/skip — 跳过任务 ==========
// 规则：distractor 节点强制 reason 非空（skip_reason 在 drill_steps），correct 节点允许空
// 写库：UPDATE tasks.status='已跳过' + INSERT drill_steps(skipped=1, skip_reason) + INSERT timeline
func SkipTask(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if len([]rune(reason)) > 500 {
			reason = string([]rune(reason)[:500])
		}

		var projectID, taskStatus, nodeID string
		err := database.QueryRow(`SELECT COALESCE(project_id,''),COALESCE(status,''),COALESCE(node_id,'') FROM tasks WHERE id=?`, id).Scan(&projectID, &taskStatus, &nodeID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		if projectID == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "任务数据异常"})
			return
		}
		if taskStatus == "已完成" || taskStatus == "已跳过" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "任务已完成或已跳过"})
			return
		}

		// 查 script_nodes.flow_type — 只有 distractor 节点强制 reason
		isDistractor := false
		if nodeID != "" {
			var ft string
			_ = database.QueryRow(`SELECT COALESCE(flow_type,'correct') FROM script_nodes WHERE id=?`, nodeID).Scan(&ft)
			isDistractor = (ft == "distractor")
		}
		if isDistractor && reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "混淆流程跳过必须填写原因", "needReason": true})
			return
		}

		_, err = database.Exec(`UPDATE tasks SET status='已跳过' WHERE id=?`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "跳过失败"})
			return
		}

		// 同步写 drill_steps（让 Score() 的 loadStepsForScore 能识别 skipped=1）
		var scenarioID string
		_ = database.QueryRow(`SELECT COALESCE(scenario_id,'') FROM projects WHERE id=?`, projectID).Scan(&scenarioID)
		now := nowSec()
		_, _ = database.Exec(`INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,correct,found_at,flow_type,score,skipped,skip_reason,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			projectID, scenarioID, roleIDFromContext(c), userNameFromContext(c),
			"跳过任务", "", "", hhmm(), nodeID, 0, 0, 0, now, "correct", 0, 1, reason, now)

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			projectID, hhmm(), userNameFromContext(c), roleIDFromContext(c), "跳过任务",
			"任务 "+id+" 跳过"+func() string { if reason != "" { return "，原因：" + reason }; return "" }(), "P1", nil)

		c.JSON(http.StatusOK, gin.H{"ok": true, "taskId": id, "status": "已跳过", "skipReason": reason})
	}
}

// ========== PATCH /api/drill/tasks/:id/lock ==========
func LockTask(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var projectID string
		err := database.QueryRow(`SELECT COALESCE(project_id,'') FROM tasks WHERE id=?`, id).Scan(&projectID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		_, err = database.Exec(`UPDATE tasks SET status='已锁定' WHERE id=?`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "锁定失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "taskId": id, "status": "已锁定"})
	}
}

// ========== PATCH /api/drill/tasks/:id/unlock ==========
func UnlockTask(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var status string
		err := database.QueryRow(`SELECT status FROM tasks WHERE id=?`, id).Scan(&status)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		newStatus := "待处理"
		if status == "已完成" || status == "已跳过" || status == "已锁定" {
			newStatus = "待处理"
		}
		_, err = database.Exec(`UPDATE tasks SET status=? WHERE id=?`, newStatus, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "解锁失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "taskId": id, "status": newStatus})
	}
}

// ========== POST /api/drill/projects/:id/tasks/:taskId/complete — 基础版任务完成 ==========
func CompleteTask(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID := c.Param("id")
		taskID := c.Param("taskId")
		var projectStatus string
		database.QueryRow(`SELECT status FROM projects WHERE id=?`, projectID).Scan(&projectStatus)
		if projectStatus != "演练中" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演练" + projectStatus + "，不能提交任务"})
			return
		}

		var taskTitle string
		var taskStatus sql.NullString
		err := database.QueryRow(`SELECT title,status FROM tasks WHERE id=? AND project_id=?`, taskID, projectID).Scan(&taskTitle, &taskStatus)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		if taskStatus.String == "已完成" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "任务已完成"})
			return
		}

		now := nowSec()
		_, err = database.Exec(`UPDATE tasks SET status='已完成', completed_at=? WHERE id=?`, now, taskID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "完成失败"})
			return
		}

		// 自动归档处置记录到 drill_steps（基础版）
		var nodeID sql.NullString
		database.QueryRow(`SELECT node_id FROM tasks WHERE id=?`, taskID).Scan(&nodeID)
		if nodeID.Valid && nodeID.String != "" {
			var scenarioID string
			database.QueryRow(`SELECT scenario_id FROM projects WHERE id=?`, projectID).Scan(&scenarioID)
			_, _ = database.Exec(`INSERT INTO drill_steps (project_id,scenario_id,role_id,user,action,title,descr,step_time,matched_node,matched,ontime,correct,found_at,flow_type,score,skipped,created_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				projectID, scenarioID, roleIDFromContext(c), userNameFromContext(c),
				"任务完成", taskTitle, "", hhmm(), nodeID.String, 1, 1, 1, now, "correct", 0, 0, nowSec())
		}

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			projectID, hhmm(), userNameFromContext(c), roleIDFromContext(c), "完成任务",
			taskTitle, "", nil)

		// 自动 score() 用于前端实时刷新
		sc := Score(database, projectID)
		c.JSON(http.StatusOK, gin.H{"ok": true, "taskId": taskID, "completedAt": now, "score": sc})
	}
}

// ========== POST /api/drill/projects/:id/evidence — 证据上传归档 ==========
func UploadEvidence(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var body struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			Note     string `json:"note"`
			NodeID   string `json:"nodeId"`
			FileName string `json:"fileName"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "证据名称不能为空"})
			return
		}

		now := nowSec()
		evID := "E" + fmt.Sprintf("%d", time.Now().Unix())
		// evidence 表列顺序: id,type,name,size(time→size改varchar),time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,created_at,note,node_id,req_key
		// 按真实表结构精确对齐
		reqKey := strings.TrimSpace(body.FileName)
		if reqKey == "" {
			reqKey = strings.TrimSpace(body.NodeID)
		}
		if _, err := database.Exec(`INSERT INTO evidence (id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,file_path,node_id,req_key,created_at,note)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			evID, strings.TrimSpace(body.Type), strings.TrimSpace(body.Name), "", hhmm(),
			roleIDFromContext(c), userNameFromContext(c), nil, 1, nil, id,
			strings.TrimSpace(body.FileName), strings.TrimSpace(body.NodeID), reqKey, now, strings.TrimSpace(body.Note)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "上传证据失败：" + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "evidenceId": evID})
	}
}

// ========== POST /api/drill/projects/:id/forms/:formId — 表单提交（触发 F1/F7 impact 联动） ==========
// F1 核实单 → 对应业务系统变 "故障"（pct 70）
// F7 恢复确认单 → 按表单值更新业务系统状态，全部已恢复时标记"正常运行"
func SubmitForm(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID := c.Param("id")
		formID := c.Param("formId")

		var body struct {
			Data       map[string]any `json:"data"`
			System     string         `json:"system"`
			Status     string         `json:"status"`
			ImpactPct  int            `json:"impactPct"`
			Recovered  bool           `json:"recovered"`
		}
		// 兼容两种格式
		if err := c.ShouldBindJSON(&body); err != nil {
			// 尝试 data-only 格式
			var body2 struct {
				FormData map[string]any `json:"formData"`
				System   string         `json:"system"`
			}
			if err2 := c.ShouldBindJSON(&body2); err2 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "表单数据格式错误"})
				return
			}
			body.Data = body2.FormData
			body.System = body2.System
		}

		formID = strings.ToUpper(strings.TrimSpace(formID))

		// ========== F1 联动 ==========
		if formID == "F1" {
			sysName := strings.TrimSpace(body.System)
			if sysName == "" && body.Data != nil {
				if v, ok := body.Data["system"]; ok {
					sysName = strings.TrimSpace(fmt.Sprint(v))
				}
			}
			if sysName == "" {
				// 兜底：将所有 impact 项置为故障
				_, _ = database.Exec(`UPDATE impact SET state='故障', pct=70 WHERE project_id=?`, projectID)
				logImpactTimeline(database, projectID, "F1核实单提交（批量）", "所有业务系统标记为故障")
				c.JSON(http.StatusOK, gin.H{"ok": true, "formId": formID, "impactUpdated": true, "detail": "所有业务系统标记为故障"})
				return
			}
			_, err := database.Exec(`UPDATE impact SET state='故障', pct=70 WHERE sys=? AND project_id=?`, sysName, projectID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新业务系统状态失败"})
				return
			}
			logImpactTimeline(database, projectID, "F1核实单提交", sysName+" → 故障（70%）")
			c.JSON(http.StatusOK, gin.H{"ok": true, "formId": formID, "impactUpdated": true, "detail": sysName + " 标记为故障"})
			return
		}

		// ========== F7 联动 ==========
		if formID == "F7" {
			sysName := strings.TrimSpace(body.System)
			newState := strings.TrimSpace(body.Status)
			pct := body.ImpactPct
			recovered := body.Recovered

			if newState == "" && body.Data != nil {
				if v, ok := body.Data["status"]; ok {
					newState = strings.TrimSpace(fmt.Sprint(v))
				}
			}
			if newState == "" {
				newState = "正常运行"
			}
			if pct == 0 {
				if newState == "正常运行" {
					pct = 100
				} else if newState == "故障" {
					pct = 0
				}
			}

			if sysName != "" {
				_, err := database.Exec(`UPDATE impact SET state=?, pct=? WHERE sys=? AND project_id=?`, newState, pct, sysName, projectID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "更新业务系统状态失败"})
					return
				}
				logImpactTimeline(database, projectID, "F7恢复确认单提交", sysName+" → "+newState)
			}

			// 检查所有系统是否都已恢复 → 整体业务恢复
			if recovered || newState == "正常运行" {
				var nonNormal int
				database.QueryRow(`SELECT COUNT(*) FROM impact WHERE project_id=? AND state!='正常运行'`, projectID).Scan(&nonNormal)
				if nonNormal == 0 {
					logImpactTimeline(database, projectID, "全部业务恢复", "所有业务系统状态已恢复正常")
				}
			}

			c.JSON(http.StatusOK, gin.H{"ok": true, "formId": formID, "impactUpdated": true, "detail": sysName + " → " + newState})
			return
		}

		// 其他表单：只记录处置记录，不触发 impact 联动
		logImpactTimeline(database, projectID, formID+" 表单提交", "表单已提交（无 impact 联动）")
		c.JSON(http.StatusOK, gin.H{"ok": true, "formId": formID})
	}
}

func logImpactTimeline(database *db.DB, projectID, action, detail string) {
	_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
		projectID, hhmm(), "system", "", action, detail, "", nil)
}

// ========== GET /api/drill/projects/:id/impact — 项目隔离的 impact ==========
func GetProjectImpact(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var rows []gin.H
		dbRows, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id=? ORDER BY seq`, id)
		defer dbRows.Close()
		for dbRows.Next() {
			var sys, state string
			var pct int
			dbRows.Scan(&sys, &state, &pct)
			rows = append(rows, gin.H{"sys": sys, "state": state, "pct": pct})
		}
		if len(rows) == 0 {
			dbRows, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id='' ORDER BY seq`)
			defer dbRows.Close()
			for dbRows.Next() {
				var sys, state string
				var pct int
				dbRows.Scan(&sys, &state, &pct)
				rows = append(rows, gin.H{"sys": sys, "state": state, "pct": pct})
			}
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": id, "impact": rows})
	}
}

// ========== GET /api/drill/projects/:id/monitor — 大屏 ==========
func MonitorProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var pname, status, mode, scenarioID, startedAt, pausedAt, endedAt string
		var pausedSec int
		err := database.QueryRow(`SELECT name,status,COALESCE(mode,'基础版'),COALESCE(scenario_id,''),COALESCE(started_at,''),COALESCE(paused_at,''),COALESCE(ended_at,''),COALESCE(paused_sec,0) FROM projects WHERE id=?`, id).
			Scan(&pname, &status, &mode, &scenarioID, &startedAt, &pausedAt, &endedAt, &pausedSec)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}

		// 计时
		elapsed := elapsedSecLite(startedAt, pausedAt, pausedSec, endedAt, status)
		elapsedStr := ""
		if elapsed != nil {
			elapsedStr = formatElapsed(*elapsed)
		}

		// 任务列表
		var tasks []gin.H
		taskRows, _ := database.Query(`SELECT id,COALESCE(role_id,''),COALESCE(title,''),COALESCE(status,''),COALESCE(level,'') FROM tasks WHERE project_id=? ORDER BY seq`, id)
		defer taskRows.Close()
		for taskRows.Next() {
			var tid, roleID, title, tstatus, level string
			taskRows.Scan(&tid, &roleID, &title, &tstatus, &level)
			tasks = append(tasks, gin.H{"id": tid, "roleId": roleID, "title": title, "status": tstatus, "level": level})
		}

		// timeline 最近 200 条
		var timeline []gin.H
		tlRows, _ := database.Query(`SELECT COALESCE(t,''),COALESCE(actor,''),COALESCE(role_id,''),COALESCE(action,''),COALESCE(detail,''),COALESCE(level,'') FROM timeline WHERE project_id=? ORDER BY seq DESC LIMIT 200`, id)
		defer tlRows.Close()
		for tlRows.Next() {
			var t, actor, roleID, action, detail, level string
			tlRows.Scan(&t, &actor, &roleID, &action, &detail, &level)
			timeline = append(timeline, gin.H{"t": t, "actor": actor, "role": roleID, "action": action, "detail": detail, "level": level})
		}
		sort.Slice(timeline, func(i, j int) bool { return timeline[i]["t"].(string) < timeline[j]["t"].(string) })

		// score
		sc := Score(database, id)

		// impact
		impactRows, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id=? ORDER BY seq`, id)
		defer impactRows.Close()
		var impact []gin.H
		for impactRows.Next() {
			var sys, state string
			var pct int
			impactRows.Scan(&sys, &state, &pct)
			impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
		}
		if len(impact) == 0 {
			impactRows2, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id='' ORDER BY seq`)
			defer impactRows2.Close()
			for impactRows2.Next() {
				var sys, state string
				var pct int
				impactRows2.Scan(&sys, &state, &pct)
				impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
			}
		}

		// stats
		stats := gin.H{"total": 0, "done": 0, "doing": 0, "pending": 0, "overdue": 0}
		if sc != nil {
			stats = gin.H{
				"total":   sc.Total,
				"done":    sc.Hit,
				"doing":   0,
				"pending": sc.Pending,
				"overdue": sc.Late + sc.Missed,
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"serverTime": nowSec(),
			"project": gin.H{
				"id":        id,
				"name":      pname,
				"status":    status,
				"mode":      mode,
				"scenarioId": scenarioID,
				"startedAt": startedAt,
				"pausedAt":  pausedAt,
				"endedAt":   endedAt,
				"elapsed":   elapsedStr,
			},
			"stats":   stats,
			"tasks":   tasks,
			"timeline": timeline,
			"score":   sc,
			"impact":  impact,
		})
	}
}

// ========== GET /api/drill/projects/:id/dashboard — 态势总看板 ==========
func DashboardProject(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var pname, status, mode, scenarioID string
		err := database.QueryRow(`SELECT name,status,COALESCE(mode,'基础版'),COALESCE(scenario_id,'') FROM projects WHERE id=?`, id).
			Scan(&pname, &status, &mode, &scenarioID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}

		sc := Score(database, id)
		scoreMap := gin.H{}
		if sc != nil {
			scoreMap = gin.H{
				"score": sc.Score, "grade": sc.Grade,
				"total": sc.Total, "hit": sc.Hit, "ontime": sc.Ontime,
				"late": sc.Late, "missed": sc.Missed, "wrong": sc.Wrong,
				"completeness": sc.Completeness, "timeRate": sc.TimeRate,
				"deductions": sc.Deductions,
				"formula": sc.Formula,
			}
		}

		// impact
		impactRows, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id=? ORDER BY seq`, id)
		defer impactRows.Close()
		var impact []gin.H
		for impactRows.Next() {
			var sys, state string
			var pct int
			impactRows.Scan(&sys, &state, &pct)
			impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
		}
		if len(impact) == 0 {
			impactRows2, _ := database.Query(`SELECT sys,state,pct FROM impact WHERE project_id='' ORDER BY seq`)
			defer impactRows2.Close()
			for impactRows2.Next() {
				var sys, state string
				var pct int
				impactRows2.Scan(&sys, &state, &pct)
				impact = append(impact, gin.H{"sys": sys, "state": state, "pct": pct})
			}
		}

		// timeline 最近 50
		var timeline []gin.H
		tlRows, _ := database.Query(`SELECT COALESCE(t,''),COALESCE(actor,''),COALESCE(role_id,''),COALESCE(action,''),COALESCE(detail,''),COALESCE(level,'') FROM timeline WHERE project_id=? ORDER BY seq DESC LIMIT 50`, id)
		defer tlRows.Close()
		for tlRows.Next() {
			var t, actor, roleID, action, detail, level string
			tlRows.Scan(&t, &actor, &roleID, &action, &detail, &level)
			timeline = append(timeline, gin.H{"t": t, "actor": actor, "role": roleID, "action": action, "detail": detail, "level": level})
		}

		c.JSON(http.StatusOK, gin.H{
			"serverTime": nowSec(),
			"project": gin.H{"id": id, "name": pname, "status": status, "mode": mode},
			"score":  scoreMap,
			"impact": impact,
			"timeline": timeline,
		})
	}
}

// ========== GET /api/drill/projects/:id/score — 直接取评分 ==========
func GetProjectScore(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		sc := Score(database, id)
		if sc == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		c.JSON(http.StatusOK, sc)
	}
}

// ========== POST /api/drill/projects/:id/score/recalc — 重新自动考评 ==========
func RecalcProjectScore(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		rid := ""
		if u != nil {
			rid = u.Role.ID
		}
		if rid != "commander" && rid != "platformadmin" && rid != "itlead" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅指挥部/平台管理员/信息科负责人可执行考评"})
			return
		}
		id := c.Param("id")
		sc := Score(database, id)
		if sc == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "演练项目不存在"})
			return
		}
		now := nowSec()
		_, _ = database.Exec(`INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at) VALUES (?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE score=VALUES(score),max=VALUES(max),note=VALUES(note),updated_at=VALUES(updated_at)`,
			"MODE", id, sc.Score, 100, sc.Formula, 1, now)

		_, _ = database.Exec(`INSERT INTO timeline (project_id,t,actor,role_id,action,detail,level,seq) VALUES (?,?,?,?,?,?,?,?)`,
			id, hhmm(), userNameFromContext(c), roleIDFromContext(c), "重新自动考评",
			fmt.Sprintf("综合得分 %.1f 分（%s）", sc.Score, sc.Grade), "P1", nil)

		c.JSON(http.StatusOK, sc)
	}
}

// ========== POST /api/drill/projects/:id/disable — 手动标记混淆节点为未下发 ==========
// 不涉及 handler，score() 内部 isDistractorReleased 已正确处理 before/after 时机
