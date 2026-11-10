package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// clampManualScore 手工评分收敛：下限 0，上限为该评分项满分（max）。
// max <= 0 时由调用方先行兜底，这里按 0 处理，避免写入无限值。
func clampManualScore(score, max float64) float64 {
	if max <= 0 {
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > max {
		return max
	}
	return score
}

// GetEvaluation GET /api/content/evaluation
// 返回 score_items 模板 + 当前项目的 scores 明细（含 MODE 和 K1-K9）
func GetEvaluation(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		qProjID := c.Query("projectId")
		if qProjID == "" {
			qProjID = currentProjectID(database)
		}

		// score_items 模板
		var items []gin.H
		itemRows, _ := database.Query(`SELECT id,COALESCE(name,''),COALESCE(weight,0),COALESCE(rule,'') FROM score_items ORDER BY seq`)
		if itemRows != nil {
			for itemRows.Next() {
				var id, name, rule string
				var weight int
				if err := itemRows.Scan(&id, &name, &weight, &rule); err != nil {
					continue
				}
				items = append(items, gin.H{"id": id, "name": name, "weight": weight, "rule": rule})
			}
			itemRows.Close()
		}

		// 当前项目 scores
		var scores []gin.H
		if qProjID != "" {
			scoreRows, _ := database.Query(`SELECT item_id,COALESCE(score,0),COALESCE(max,0),COALESCE(note,''),COALESCE(auto,0) FROM scores WHERE project_id=?`, qProjID)
			if scoreRows != nil {
				for scoreRows.Next() {
					var itemID, note string
					var score, max float64
					var auto int
					if err := scoreRows.Scan(&itemID, &score, &max, &note, &auto); err != nil {
						continue
					}
					scores = append(scores, gin.H{
						"itemId": itemID, "score": score, "max": max, "note": note, "auto": auto == 1,
					})
				}
				scoreRows.Close()
			}
		}

		if items == nil {
			items = []gin.H{}
		}
		if scores == nil {
			scores = []gin.H{}
		}

		resp := gin.H{
			"items":  items,
			"scores": scores,
		}
		if qProjID != "" {
			// 评分封存读取路径：默认展示封存版本；?snapshotVersion=N 可切换历史/重算版本。
			seal := loadSeal(database, qProjID)
			ver, _ := strconv.Atoi(c.Query("snapshotVersion"))
			if ver <= 0 && seal != nil {
				ver = seal.SnapshotVersion
			}
			resp["seal"] = seal
			resp["snapshots"] = listSnapshots(database, qProjID)
			resp["snapshot"] = loadSnapshot(database, qProjID, ver)
			resp["endException"] = loadEndException(database, qProjID)
		}

		c.JSON(http.StatusOK, resp)
	}
}

// PostEvaluationScores POST /api/content/evaluation/scores
// 手动考评 K1-K9 分项写入。权限: commander / itlead / platformadmin
// Request body: { projectId?: string, scores: [{ itemId: 'K1', score: 0, max: 15, note?: ” }, ...] }
// MySQL 复合主键 ON DUPLICATE KEY UPDATE 以 (item_id, project_id) 为冲突键
func PostEvaluationScores(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 权限检查：*auth.User.Role.ID
		roleID := ""
		if v, ok := c.Get(auth.ContextKey); ok {
			if u, ok := v.(*auth.User); ok {
				roleID = u.Role.ID
			}
		}
		if roleID != "commander" && roleID != "itlead" && roleID != "platformadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅指挥部/信息科负责人/平台管理员可执行考评"})
			return
		}

		var body struct {
			ProjectID string `json:"projectId"`
			Scores    []struct {
				ItemID string  `json:"itemId"`
				Score  float64 `json:"score"`
				Max    float64 `json:"max"`
				Note   string  `json:"note"`
			} `json:"scores"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "评分数据格式错误"})
			return
		}
		if len(body.Scores) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "评分数据格式错误"})
			return
		}

		qProjID := body.ProjectID
		if qProjID == "" {
			qProjID = c.Query("projectId")
		}
		if qProjID == "" {
			// 列出 status ∈ {未开始, 演练中, 已结束} 的候选项目
			type projCandidate struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
			}
			var candidates []projCandidate
			rows, _ := database.Query(
				`SELECT id,COALESCE(name,''),COALESCE(status,'未开始') FROM projects WHERE status IN ('未开始','演练中','已结束') ORDER BY date DESC, id DESC`)
			defer rows.Close()
			for rows.Next() {
				var c projCandidate
				if rows.Scan(&c.ID, &c.Name, &c.Status) == nil {
					candidates = append(candidates, c)
				}
			}
			if len(candidates) == 1 {
				qProjID = candidates[0].ID
			} else if len(candidates) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "未找到任何候选演练项目，请先创建演练项目", "candidates": []any{}})
				return
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":      "未指定项目，当前存在多个候选演练项目，请在 body 或 query 里显式传入 projectId",
					"candidates": candidates,
				})
				return
			}
		}
		if qProjID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未指定项目"})
			return
		}

		ts := nowSec()
		tx, err := database.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库事务失败"})
			return
		}
		defer tx.Rollback()

		stmt, err := tx.Prepare(`INSERT INTO scores (item_id,project_id,score,max,note,auto,updated_at)
			VALUES (?,?,?,?,?,0,?)
			ON DUPLICATE KEY UPDATE score=VALUES(score),max=VALUES(max),note=VALUES(note),auto=0,updated_at=VALUES(updated_at)`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "准备语句失败"})
			return
		}
		defer stmt.Close()

		var sum, maxTotal float64
		for _, s := range body.Scores {
			if s.ItemID == "" {
				continue
			}
			maxVal := s.Max
			if maxVal <= 0 {
				maxVal = 10
			}
			// 手工评分收敛：下限 0，上限为该项满分，避免超满分或负分写入。
			scoreVal := clampManualScore(s.Score, maxVal)
			noteVal := s.Note
			if noteVal == "" {
				noteVal = ""
			}
			if _, err := stmt.Exec(s.ItemID, qProjID, scoreVal, maxVal, noteVal, ts); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "写入失败: " + err.Error()})
				return
			}
			sum += scoreVal
			maxTotal += maxVal
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "提交失败"})
			return
		}

		// 站内消息：自动考评完成（Node content.js:1400-1406）
		maxText := ""
		if maxTotal != 0 {
			maxText = "/" + fmt.Sprint(maxTotal)
		}
		pushNotif(database, "auto_eval", map[string]any{
			"count": len(body.Scores), "sum": sum, "max": maxText,
		}, qProjID, "", "", "internal")

		c.JSON(http.StatusOK, gin.H{
			"ok":        true,
			"projectId": qProjID,
			"items":     len(body.Scores),
		})
	}
}
