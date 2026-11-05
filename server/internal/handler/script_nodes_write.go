package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ---------- script-nodes POST 辅助 ----------

// scriptNodeIn 前端提交的节点原始结构。
// FlowType / AnchorID / ReleaseTiming 为单一权威字段（json:"-"），
// 由自定义 UnmarshalJSON 同时接受 camelCase 和 snake_case 入参。
type scriptNodeIn struct {
	ID            string `json:"id"`
	ScenarioID    string `json:"scenarioId"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	ShortTitle    string `json:"shortTitle"`
	Target        string `json:"target"`
	Time          string `json:"time"`
	Timeout       *int64 `json:"timeout"`
	Form          string `json:"form"`
	Att           string `json:"att"`
	Desc          string `json:"desc"`
	Branches      any    `json:"branches"`
	Guide         string `json:"guide"`
	Keywords      string `json:"keywords"`
	Action        string `json:"action"`
	Wrong         any    `json:"wrong"`
	EvidenceReq   any    `json:"evidenceReq"`
	Required      any    `json:"required"`
	FlowType      string `json:"-"`
	Score         any    `json:"score"`
	Receiver      string `json:"receiver"`
	Deps          any    `json:"deps"`
	PosX          any    `json:"posX"`
	PosY          any    `json:"posY"`
	AnchorID      string `json:"-"`
	ParallelDesc  string `json:"parallel_desc"`
	ReleaseTiming string `json:"-"`
}

// UnmarshalJSON 归一并行两套前端字段名（camelCase / snake_case）到单一权威字段。
// 所有字段逐项容错解析：字符串字段遇到数组（如 keywords）、数字、布尔、null 时
// 降级转换而不是让整批节点解析失败（前端会因此收到「节点数据格式错误」）。
func (n *scriptNodeIn) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	n.ID = textOf(raw, "id")
	n.ScenarioID = textOf(raw, "scenarioId", "scenario_id")
	n.Type = textOf(raw, "type")
	n.Title = textOf(raw, "title")
	n.ShortTitle = textOf(raw, "shortTitle", "short_title")
	n.Target = textOf(raw, "target")
	n.Time = textOf(raw, "time")
	n.Form = textOf(raw, "form")
	n.Att = textOf(raw, "att")
	n.Desc = textOf(raw, "desc", "descr")
	n.Guide = textOf(raw, "guide")
	n.Keywords = textOf(raw, "keywords") // 前端提交数组 → 以逗号拼接
	n.Action = textOf(raw, "action")
	n.Receiver = textOf(raw, "receiver")
	n.ParallelDesc = textOf(raw, "parallel_desc", "parallelDesc")
	n.FlowType = textOf(raw, "flowType", "flow_type")
	n.AnchorID = textOf(raw, "anchorId", "anchor_id")
	n.ReleaseTiming = textOf(raw, "releaseTiming", "release_timing")
	n.Timeout = intPtrOf(raw, "timeout")
	n.Branches = anyOf(raw, "branches")
	n.Wrong = anyOf(raw, "wrong")
	n.EvidenceReq = anyOf(raw, "evidenceReq", "evidence_req")
	n.Required = requiredOf(raw, "required")
	n.Score = anyOf(raw, "score")
	n.Deps = anyOf(raw, "deps")
	n.PosX = anyOf(raw, "posX", "pos_x")
	n.PosY = anyOf(raw, "posY", "pos_y")
	return nil
}

// firstRaw 按候选键顺序取第一个非 null 的原始值。
func firstRaw(m map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && strings.TrimSpace(string(v)) != "null" {
			return v, true
		}
	}
	return nil, false
}

// textOf 把任意 JSON 标量/数组降级为字符串：数组按英文逗号拼接，对象忽略。
func textOf(m map[string]json.RawMessage, keys ...string) string {
	v, ok := firstRaw(m, keys...)
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(v, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			sub := map[string]json.RawMessage{"_": item}
			if t := textOf(sub, "_"); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ",")
	}
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		return strconv.FormatBool(b)
	}
	return ""
}

// intPtrOf 数值字段容错：数字/数字字符串均可，非法值降级为 nil（不阻断整批）。
func intPtrOf(m map[string]json.RawMessage, keys ...string) *int64 {
	v, ok := firstRaw(m, keys...)
	if !ok {
		return nil
	}
	var i int64
	if err := json.Unmarshal(v, &i); err == nil {
		return &i
	}
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		iv := int64(f)
		return &iv
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			iv := int64(f)
			return &iv
		}
	}
	return nil
}

// anyOf 取任意 JSON 值；null/缺失均返回 nil。
func anyOf(m map[string]json.RawMessage, keys ...string) any {
	v, ok := firstRaw(m, keys...)
	if !ok {
		return nil
	}
	var out any
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}
	return out
}

// requiredOf 布尔字段容错：bool / 0-1 / "true|false" 均可识别；缺失返回 nil（默认必经）。
func requiredOf(m map[string]json.RawMessage, keys ...string) any {
	v, ok := firstRaw(m, keys...)
	if !ok {
		return nil
	}
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		return b
	}
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		return f != 0
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "false", "0", "no", "":
			return false
		default:
			return true
		}
	}
	return nil
}

func (n *scriptNodeIn) GetFlowType() string      { return n.FlowType }
func (n *scriptNodeIn) GetAnchorID() string      { return strings.TrimSpace(n.AnchorID) }
func (n *scriptNodeIn) GetReleaseTiming() string { return n.ReleaseTiming }

// receiverStr 复刻 src/routes/content.js:21
func receiverStr(v string) string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return strings.ContainsRune("、,，;；", r)
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if len(s) > 40 {
			s = s[:40]
		}
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, "、")
}

// resolveVersion 复刻 src/routes/content.js:480
func resolveVersion(incoming, bump, currentV string) string {
	incoming = strings.TrimSpace(incoming)
	if incoming != "" {
		re := regexp.MustCompile(`^v?\d+\.\d+$`)
		if re.MatchString(incoming) {
			if strings.HasPrefix(incoming, "v") {
				return incoming
			}
			return "v" + incoming
		}
	}
	bump = strings.ToLower(strings.TrimSpace(bump))
	if bump == "" {
		bump = "minor"
	}
	re := regexp.MustCompile(`^v(\d+)\.(\d+)$`)
	m := re.FindStringSubmatch(strings.TrimSpace(currentV))
	if m == nil {
		return "v1.0"
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if bump == "none" || bump == "keep" {
		return "v" + strconv.Itoa(major) + "." + strconv.Itoa(minor)
	}
	if bump == "major" {
		return "v" + strconv.Itoa(major+1) + ".0"
	}
	return "v" + strconv.Itoa(major) + "." + strconv.Itoa(minor+1)
}

// topoSortNodes 复刻 src/routes/content.js:498
func topoSortNodes(ids []string, depMap map[string][]string) []string {
	present := make(map[string]bool)
	indeg := make(map[string]int)
	children := make(map[string][]string)
	for _, id := range ids {
		present[id] = true
		indeg[id] = 0
		children[id] = []string{}
	}
	for _, id := range ids {
		for _, d := range depMap[id] {
			if !present[d] {
				continue
			}
			indeg[id]++
			children[d] = append(children[d], id)
		}
	}
	queue := []string{}
	for _, id := range ids {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	out := []string{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, c := range children[id] {
			indeg[c]--
			if indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if len(out) != len(ids) {
		return nil
	}
	return out
}

var cleanRoleRe = regexp.MustCompile(`[^a-zA-Z0-9\p{Han}]`)

func cleanRoleName(s string) string {
	s = cleanRoleRe.ReplaceAllString(s, "")
	if len([]rune(s)) > 6 {
		runes := []rune(s)
		s = string(runes[:6])
	}
	if s == "" {
		return "other"
	}
	return s
}

// ---------- script-nodes POST — 完整复刻 src/routes/content.js:523-738 ----------
func PostScriptNodes(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if u.Role.ID != "platformadmin" && u.Role.ID != "commander" && u.Role.ID != "itlead" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无剧本编排权限"})
			return
		}

		var body struct {
			ScenarioID  string         `json:"scenarioId"`
			Name        string         `json:"name"`
			Alert       string         `json:"alert"`
			Version     string         `json:"version"`
			VersionBump string         `json:"versionBump"`
			Nodes       []scriptNodeIn `json:"nodes"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Nodes == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "节点数据格式错误"})
			return
		}
		scenarioID := strings.TrimSpace(body.ScenarioID)
		if scenarioID == "" {
			scenarioID = "S01"
		}

		sc, err := queryScenario(database, scenarioID)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "场景剧本不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败"})
			return
		}

		// 过滤 sceneNodes
		sceneNodes := []scriptNodeIn{}
		for _, n := range body.Nodes {
			if strings.TrimSpace(n.ScenarioID) == "" || n.ScenarioID == scenarioID {
				sceneNodes = append(sceneNodes, n)
			}
		}

		// ID 唯一性 + 跨剧本冲突校验
		byID := make(map[string]*scriptNodeIn)
		seen := make(map[string]bool)
		for i := range sceneNodes {
			id := strings.TrimSpace(sceneNodes[i].ID)
			if id == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "节点 ID 不能为空"})
				return
			}
			if seen[id] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "节点 ID 重复：" + id})
				return
			}
			var ownerScene sql.NullString
			_ = database.QueryRow(`SELECT scenario_id FROM script_nodes WHERE id=?`, id).Scan(&ownerScene)
			if ownerScene.Valid && ownerScene.String != "" && ownerScene.String != scenarioID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "节点 ID 与其他剧本冲突：" + id})
				return
			}
			seen[id] = true
			byID[id] = &sceneNodes[i]
		}

		// ========== 重编号 ==========
		mainIDRe := regexp.MustCompile(`^` + regexp.QuoteMeta(scenarioID) + `-(\d+)$`)
		distIDRe := regexp.MustCompile(`^DS` + regexp.QuoteMeta(scenarioID) + `-(.+?)-(\d+)$`)

		// Step 1: 主流节点收集已有编号，算空号和 max
		existingMainNums := []int{}
		for _, n := range sceneNodes {
			if n.GetFlowType() == "distractor" {
				continue
			}
			m := mainIDRe.FindStringSubmatch(strings.TrimSpace(n.ID))
			if m != nil {
				if num, err := strconv.Atoi(m[1]); err == nil {
					existingMainNums = append(existingMainNums, num)
				}
			}
		}
		sort.Ints(existingMainNums)
		mainMax := 0
		if len(existingMainNums) > 0 {
			mainMax = existingMainNums[len(existingMainNums)-1]
		}
		mainGaps := []int{}
		if len(existingMainNums) > 0 {
			idx := 0
			for i := 1; i <= mainMax; i++ {
				if idx >= len(existingMainNums) || existingMainNums[idx] > i {
					mainGaps = append(mainGaps, i)
				} else if existingMainNums[idx] == i {
					idx++
				}
			}
		}

		// Step 2: 主流节点编号分配
		gapIdx := 0
		nextNew := mainMax + 1
		idMap := make(map[string]string)
		for _, n := range sceneNodes {
			if n.GetFlowType() == "distractor" {
				continue
			}
			oldID := strings.TrimSpace(n.ID)
			m := mainIDRe.FindStringSubmatch(oldID)
			if m != nil {
				idMap[oldID] = oldID
			} else {
				var num int
				if gapIdx < len(mainGaps) {
					num = mainGaps[gapIdx]
					gapIdx++
				} else {
					num = nextNew
					nextNew++
				}
				idMap[oldID] = scenarioID + "-" + strconv.Itoa(num)
			}
		}

		// Step 3: 混淆节点编号分配
		distRoleUsed := make(map[string][]int)
		for _, n := range sceneNodes {
			if n.GetFlowType() != "distractor" {
				continue
			}
			m := distIDRe.FindStringSubmatch(strings.TrimSpace(n.ID))
			if m != nil {
				role := m[1]
				num, _ := strconv.Atoi(m[2])
				distRoleUsed[role] = append(distRoleUsed[role], num)
			}
		}
		for _, n := range sceneNodes {
			if n.GetFlowType() != "distractor" {
				continue
			}
			oldID := strings.TrimSpace(n.ID)
			m := distIDRe.FindStringSubmatch(oldID)
			if m != nil {
				idMap[oldID] = oldID
			} else {
				roleName := cleanRoleName(strings.TrimSpace(n.Target))
				nums := distRoleUsed[roleName]
				num := 1
				for {
					found := false
					for _, v := range nums {
						if v == num {
							found = true
							break
						}
					}
					if !found {
						break
					}
					num++
				}
				distRoleUsed[roleName] = append(nums, num)
				idMap[oldID] = "DS" + scenarioID + "-" + roleName + "-" + strconv.Itoa(num)
			}
		}

		// Step 4: 应用 idMap
		for i := range sceneNodes {
			oldID := strings.TrimSpace(sceneNodes[i].ID)
			if newID, ok := idMap[oldID]; ok && newID != oldID {
				sceneNodes[i].ID = newID
			}
			// deps
			if sceneNodes[i].Deps != nil {
				arr, ok := sceneNodes[i].Deps.([]any)
				if ok {
					newDeps := []string{}
					for _, d := range arr {
						s := strings.TrimSpace(anyToString(d))
						if newID, ok := idMap[s]; ok && newID != s {
							newDeps = append(newDeps, newID)
						} else if s != "" {
							newDeps = append(newDeps, s)
						}
					}
					sceneNodes[i].Deps = newDeps
				}
			}
			// anchor_id
			if sceneNodes[i].GetFlowType() == "distractor" {
				oldAnchor := sceneNodes[i].GetAnchorID()
				if newAnchor, ok := idMap[oldAnchor]; ok && newAnchor != oldAnchor {
					sceneNodes[i].AnchorID = newAnchor
				}
			}
		}

		// 重建 byID
		byID = make(map[string]*scriptNodeIn)
		for i := range sceneNodes {
			byID[strings.TrimSpace(sceneNodes[i].ID)] = &sceneNodes[i]
		}

		// 混淆节点锚点校验
		for _, n := range sceneNodes {
			if n.GetFlowType() != "distractor" {
				continue
			}
			title := strings.TrimSpace(n.Title)
			if title == "" {
				title = strings.TrimSpace(n.ID)
			}
			anchorID := n.GetAnchorID()
			if anchorID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "混淆节点「" + title + "」未配置「对应主流节点」，请先在节点侧边栏选择锚点主流节点"})
				return
			}
			anchor, ok := byID[anchorID]
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "混淆节点「" + title + "」的锚点节点「" + anchorID + "」不存在，请重新选择"})
				return
			}
			if anchor.GetFlowType() == "distractor" {
				aTitle := strings.TrimSpace(anchor.Title)
				if aTitle == "" {
					aTitle = strings.TrimSpace(anchor.ID)
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": "混淆节点「" + title + "」的锚点「" + aTitle + "」也是混淆节点，锚点必须指向正确流程节点"})
				return
			}
		}

		// 依赖清洗 + 拓扑排序（仅主流节点）
		depMap := make(map[string][]string)
		correctNodes := []*scriptNodeIn{}
		for i := range sceneNodes {
			if sceneNodes[i].GetFlowType() != "distractor" {
				correctNodes = append(correctNodes, &sceneNodes[i])
			}
		}
		for _, n := range correctNodes {
			deps := []string{}
			var rawDeps []any
			if arr, ok := n.Deps.([]any); ok {
				rawDeps = arr
			} else if arr, ok := n.Deps.([]string); ok {
				for _, s := range arr {
					rawDeps = append(rawDeps, s)
				}
			}
			for _, d := range rawDeps {
				id := strings.TrimSpace(anyToString(d))
				if id == "" || id == strings.TrimSpace(n.ID) {
					continue
				}
				found := false
				for _, existing := range deps {
					if existing == id {
						found = true
						break
					}
				}
				if found {
					continue
				}
				pre, ok := byID[id]
				if !ok {
					continue
				}
				if pre.GetFlowType() == "distractor" {
					continue
				}
				if strings.TrimSpace(pre.Type) == "end" {
					continue
				}
				deps = append(deps, id)
			}
			depMap[strings.TrimSpace(n.ID)] = deps
		}

		correctIDs := []string{}
		for _, n := range correctNodes {
			correctIDs = append(correctIDs, strings.TrimSpace(n.ID))
		}
		order := topoSortNodes(correctIDs, depMap)
		if order == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "剧本存在循环依赖，请检查节点前置关系"})
			return
		}

		// receiver 推导
		succTargets := make(map[string][]string)
		for _, n := range correctNodes {
			t := strings.TrimSpace(n.Target)
			if t == "" {
				continue
			}
			for _, d := range depMap[strings.TrimSpace(n.ID)] {
				succTargets[d] = append(succTargets[d], t)
			}
		}

		// ordered = topo 主流 + 混淆节点
		distractors := []scriptNodeIn{}
		for i := range sceneNodes {
			if sceneNodes[i].GetFlowType() == "distractor" {
				distractors = append(distractors, sceneNodes[i])
			}
		}
		ordered := []*scriptNodeIn{}
		for _, id := range order {
			ordered = append(ordered, byID[id])
		}
		for i := range distractors {
			ordered = append(ordered, &distractors[i])
		}

		// ========== 事务：DELETE + INSERT + UPDATE scenarios ==========
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败"})
			return
		}
		defer func() { _ = tx.Rollback() }()

		// 保存前记录既有节点的 seq：结构未变化的节点沿用原 seq，
		// 避免「打开即保存」时因拓扑重排导致序号漂移（多次保存结果不一致）。
		existingSeq := make(map[string]int64)
		var maxSeq int64
		seqRows, err := tx.Query(`SELECT id,seq FROM script_nodes WHERE scenario_id=?`, scenarioID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取旧节点序号失败：" + err.Error()})
			return
		}
		for seqRows.Next() {
			var nid string
			var sq sql.NullInt64
			if err := seqRows.Scan(&nid, &sq); err == nil {
				v := int64(0)
				if sq.Valid {
					v = sq.Int64
				}
				existingSeq[nid] = v
				if v > maxSeq {
					maxSeq = v
				}
			}
		}
		seqRows.Close()

		if _, err := tx.Exec(`DELETE FROM script_nodes WHERE scenario_id=?`, scenarioID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清理旧节点失败：" + err.Error()})
			return
		}

		for _, n := range ordered {
			flowType := "correct"
			if n.GetFlowType() == "distractor" {
				flowType = "distractor"
			}
			wrongList := []string{}
			if arr, ok := n.Wrong.([]any); ok {
				for _, x := range arr {
					s := strings.TrimSpace(anyToString(x))
					if s != "" {
						wrongList = append(wrongList, s)
						if len(wrongList) >= 5 {
							break
						}
					}
				}
			}
			receiver := ""
			if flowType != "distractor" {
				receiver = receiverStr(strings.TrimSpace(n.Receiver))
				if receiver == "" {
					targets := succTargets[strings.TrimSpace(n.ID)]
					if len(targets) > 0 {
						receiver = strings.Join(targets, "、")
					} else {
						receiver = "院应急指挥部"
					}
				}
			}
			var evReq []evItem
			if flowType != "distractor" {
				evReq = normalizeEvItemsFromAny(n.EvidenceReq)
				if len(evReq) == 0 && strings.TrimSpace(n.Type) != "end" {
					evReq = autoEvidenceReq(
						strings.TrimSpace(n.Title),
						strings.TrimSpace(n.Target),
						strings.TrimSpace(n.Action),
						strings.TrimSpace(n.Guide),
						strings.TrimSpace(n.Desc),
						strings.TrimSpace(n.Form),
					)
				}
			}
			wrongJSON, _ := json.Marshal(wrongList)
			evReqJSON, _ := json.Marshal(evReq)
			depsForNode := []string{}
			if flowType != "distractor" {
				depsForNode = depMap[strings.TrimSpace(n.ID)]
			}
			depsJSON, _ := json.Marshal(depsForNode)
			branchesJSON, _ := json.Marshal(nilSafeJSON(n.Branches))

			scoreVal := 0
			if s, err := strconv.Atoi(anyToString(n.Score)); err == nil {
				scoreVal = s
			}
			if flowType == "distractor" {
				// 混淆节点分值即「严重程度」，只允许高 8 / 中 5 / 低 3 三档。
				if n.Score == nil {
					scoreVal = 5
				} else if scoreVal != 3 && scoreVal != 5 && scoreVal != 8 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "混淆流程节点「" + strings.TrimSpace(n.Title) + "」的严重程度分值不合法：仅允许 8（高）/ 5（中）/ 3（低）三档"})
					return
				}
			} else {
				if scoreVal < 0 {
					scoreVal = 0
				}
				if scoreVal > 100 {
					scoreVal = 100
				}
			}

			var posX, posY sql.NullInt64
			if n.PosX != nil {
				if v, err := strconv.Atoi(anyToString(n.PosX)); err == nil {
					posX = sql.NullInt64{Int64: int64(v), Valid: true}
				}
			}
			if n.PosY != nil {
				if v, err := strconv.Atoi(anyToString(n.PosY)); err == nil {
					posY = sql.NullInt64{Int64: int64(v), Valid: true}
				}
			}

			var anchorVal sql.NullString
			var timingVal string
			if flowType == "distractor" {
				a := n.GetAnchorID()
				if a != "" {
					anchorVal = sql.NullString{String: a, Valid: true}
				}
				t := n.GetReleaseTiming()
				if t != "" {
					timingVal = t
				} else {
					timingVal = "before"
				}
			} else {
				timingVal = "before"
			}

			timeoutVal := sql.NullInt64{}
			if n.Timeout != nil && *n.Timeout > 0 {
				timeoutVal = sql.NullInt64{Int64: *n.Timeout, Valid: true}
			}
			requiredVal := int64(1)
			if v, ok := n.Required.(bool); ok && !v {
				requiredVal = 0
			}

			seqVal := maxSeq + 1
			if v, ok := existingSeq[strings.TrimSpace(n.ID)]; ok {
				seqVal = v
			} else {
				maxSeq++
			}

			if _, err := tx.Exec(`INSERT INTO script_nodes
				(id,scenario_id,type,title,short_title,target,time,timeout,form,att,
				` + "`descr`" + `,branches,seq,guide,keywords,action,wrong,evidence_req,required,flow_type,
				score,receiver,deps,pos_x,pos_y,parallel_desc,anchor_id,release_timing)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				strings.TrimSpace(n.ID), scenarioID,
				strings.TrimSpace(n.Type), strings.TrimSpace(n.Title), strings.TrimSpace(n.ShortTitle),
				strings.TrimSpace(n.Target), "",
				timeoutVal,
				strings.TrimSpace(n.Form), strings.TrimSpace(n.Att), strings.TrimSpace(n.Desc),
				string(branchesJSON), seqVal,
				strings.TrimSpace(n.Guide), strings.TrimSpace(n.Keywords), strings.TrimSpace(n.Action),
				string(wrongJSON), string(evReqJSON), requiredVal,
				flowType, scoreVal, receiver,
				string(depsJSON), posX, posY,
				strings.TrimSpace(n.ParallelDesc), anchorVal, timingVal,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "写入节点 " + strings.TrimSpace(n.ID) + " 失败：" + err.Error()})
				return
			}
		}

		finalName := strings.TrimSpace(body.Name)
		if finalName == "" {
			finalName = sc.Name
		}
		if len([]rune(finalName)) > 60 {
			finalName = string([]rune(finalName)[:60])
		}
		alertText := strings.TrimSpace(body.Alert)
		if alertText == "" {
			alertText = sc.Alert
		}
		if len([]rune(alertText)) > 1000 {
			alertText = string([]rune(alertText)[:1000])
		}

		newVersion := resolveVersion(body.Version, body.VersionBump, sc.Version)
		updated := util.DateStr()

		if _, err := tx.Exec(`UPDATE scenarios SET name=?, nodes=?, version=?, updated=?, alert=? WHERE id=?`,
			finalName, len(correctNodes), newVersion, updated, alertText, scenarioID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新场景信息失败：" + err.Error()})
			return
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存剧本编排失败"})
			return
		}

		nowSc, _ := queryScenario(database, scenarioID)
		c.JSON(200, gin.H{
			"ok":          true,
			"count":       len(correctNodes),
			"scenarioId":  scenarioID,
			"version":     nowSc.Version,
			"updated":     nowSc.Updated,
			"alert":       nowSc.Alert,
			"name":        nowSc.Name,
		})
	}
}

// --- 辅助 ---
func queryScenario(database *db.DB, id string) (Scenario, error) {
	row := database.QueryRow(
		`SELECT id,name,type,level,status,version,updated,nodes,`+"`descr`,alert FROM scenarios WHERE id=?", id)
	var s Scenario
	var descr sql.NullString
	var alert sql.NullString
	err := row.Scan(&s.ID, &s.Name, &s.Type, &s.Level, &s.Status, &s.Version,
		&s.Updated, &s.Nodes, &descr, &alert)
	if descr.Valid {
		s.Desc = descr.String
	}
	if alert.Valid {
		s.Alert = alert.String
	}
	return s, err
}

type Scenario struct {
	ID      string
	Name    string
	Type    string
	Level   string
	Status  string
	Version string
	Updated string
	Nodes   int64
	Desc    string
	Alert   string
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func nvl(s string) string {
	return s
}

type evItem struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Note string `json:"note"`
}

func normalizeEvItemsFromAny(v any) []evItem {
	if v == nil {
		return []evItem{}
	}
	arr, ok := v.([]any)
	if !ok {
		return []evItem{}
	}
	raw := []evItem{}
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		item := evItem{}
		if t, ok := m["type"].(string); ok {
			item.Type = strings.TrimSpace(t)
		}
		if nm, ok := m["name"].(string); ok {
			item.Name = strings.TrimSpace(nm)
		}
		if nt, ok := m["note"].(string); ok {
			item.Note = strings.TrimSpace(nt)
		}
		if item.Name != "" {
			raw = append(raw, item)
		}
	}
	return normalizeEvItems(raw)
}

func normalizeEvItems(items []evItem) []evItem {
	out := []evItem{}
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		if len([]rune(name)) > 160 {
			name = string([]rune(name)[:160])
		}
		typ := it.Type
		evTypes := []string{"附件截图", "操作日志", "处置记录", "样本摘要", "表单文书", "上报材料", "报案材料", "附件"}
		found := false
		for _, t := range evTypes {
			if t == typ {
				found = true
				break
			}
		}
		if !found {
			typ = "处置记录"
		}
		note := strings.TrimSpace(it.Note)
		if len([]rune(note)) > 400 {
			note = string([]rune(note)[:400])
		}
		out = append(out, evItem{Type: typ, Name: name, Note: note})
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func autoEvidenceReq(title, target, action, guide, desc, form string) []evItem {
	doc := []evItem{}
	if form != "" {
		shortTitle := title
		if len([]rune(shortTitle)) > 20 {
			shortTitle = string([]rune(shortTitle)[:20])
		}
		doc = append(doc, evItem{Type: "表单文书", Name: strings.TrimSpace(form) + "（" + shortTitle + "）", Note: "需经签字/审批的正式文书"})
	}
	type kvPattern struct {
		re    *regexp.Regexp
		items [][2]string
	}
	patterns := []kvPattern{
		{regexp.MustCompile(`告警|核实|监控|排查`), [][2]string{{"附件截图", "告警/监控界面截图"}, {"处置记录", "核实处置记录"}}},
		{regexp.MustCompile(`隔离|断网|封堵|封禁`), [][2]string{{"操作日志", "网络设备/防火墙操作日志"}, {"处置记录", "隔离处置记录"}}},
		{regexp.MustCompile(`取证|样本|日志|证据`), [][2]string{{"样本摘要", "样本/日志摘要归档"}, {"附件", "取证材料附件"}}},
		{regexp.MustCompile(`备份|恢复|验证`), [][2]string{{"处置记录", "备份/恢复操作记录"}, {"附件截图", "业务恢复验证截图"}}},
		{regexp.MustCompile(`报告|上报|审批|启动`), [][2]string{{"上报材料", "上报材料/审批记录"}, {"表单文书", "报告/审批表单"}}},
		{regexp.MustCompile(`通知|预警|公告|兜底`), [][2]string{{"处置记录", "通知发布记录"}, {"附件截图", "预警/兜底界面截图"}}},
		{regexp.MustCompile(`报案|舆情|对外|口径`), [][2]string{{"报案材料", "报案材料/对接记录"}, {"上报材料", "对外口径/舆情回应"}}},
		{regexp.MustCompile(`复盘|终止|评估|整改`), [][2]string{{"处置记录", "复盘评估记录"}}},
	}
	text := action + title + desc + guide
	matched := false
	for _, pat := range patterns {
		if pat.re.MatchString(text) {
			for _, item := range pat.items {
				doc = append(doc, evItem{Type: item[0], Name: item[1]})
			}
			matched = true
		}
	}
	if !matched {
		shortTitle := title
		if len([]rune(shortTitle)) > 24 {
			shortTitle = string([]rune(shortTitle)[:24])
		}
		doc = append(doc, evItem{Type: "处置记录", Name: shortTitle + "处置记录"})
		doc = append(doc, evItem{Type: "附件截图", Name: "界面/流程截图"})
	}
	return normalizeEvItems(doc)
}

func nilSafeJSON(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}
