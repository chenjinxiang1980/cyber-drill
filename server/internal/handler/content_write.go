package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"drill-system/internal/auth"
	"drill-system/internal/db"
	"drill-system/internal/util"

	"github.com/gin-gonic/gin"
)

// ---------- 剧本权限中间件 ----------
// 可维护剧本的角色：platformadmin / commander / itlead
var scenarioEditRoles = map[string]bool{
	"platformadmin": true,
	"commander":     true,
	"itlead":        true,
}

func requireScenarioEditor(c *gin.Context) bool {
	u := auth.CurrentUser(c)
	if u == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return false
	}
	if !scenarioEditRoles[u.Role.ID] {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/指挥部/信息科负责人可维护剧本"})
		return false
	}
	return true
}

// ---------- POST /api/content/scenarios — 新建剧本 ----------
func PostScenario(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireScenarioEditor(c) {
			return
		}
		var body struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Level string `json:"level"`
			Desc  string `json:"desc"`
			Alert string `json:"alert"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "剧本名称不能为空"})
			return
		}

		// 查当前最大序号
		var maxID sql.NullString
		_ = database.QueryRow(`SELECT MAX(CAST(SUBSTR(id,2) AS UNSIGNED)) FROM scenarios`).Scan(&maxID)
		nextNum := 1
		if maxID.Valid {
			re := regexp.MustCompile(`^S(\d+)$`)
			m := re.FindStringSubmatch(maxID.String)
			if len(m) == 2 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					nextNum = n + 1
				}
			}
		}
		id := "S" + strconv.Itoa(nextNum)
		if len(id) < 3 {
			id = "S0" + strconv.Itoa(nextNum)
		}
		if len(id) < 4 {
			id = "S0" + strconv.Itoa(nextNum)
		}

		level := strings.TrimSpace(body.Level)
		if level == "" {
			level = "一般"
		}
		typ := strings.TrimSpace(body.Type)

		now := util.DateStr()
		_, err := database.Exec(
			`INSERT INTO scenarios (id,name,type,level,status,version,updated,nodes,descr,alert)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, name, typ, level, "草稿", "v1.0", now, 0,
			strings.TrimSpace(body.Desc), strings.TrimSpace(body.Alert),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建剧本失败"})
			return
		}
		c.JSON(200, gin.H{"ok": true, "id": id})
	}
}

// ---------- PATCH /api/content/scenarios/:id — 更新剧本 ----------
func PatchScenario(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireScenarioEditor(c) {
			return
		}
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "剧本ID不能为空"})
			return
		}

		// 查存在性
		var exists bool
		_ = database.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE id=?`, id).Scan(&exists)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "剧本不存在"})
			return
		}

		// 动态字段更新（Node 版按 body 里有啥就更啥）
		type patchBody struct {
			Name    *string `json:"name"`
			Type    *string `json:"type"`
			Level   *string `json:"level"`
			Status  *string `json:"status"`
			Desc    *string `json:"desc"`
			Alert   *string `json:"alert"`
			Version *string `json:"version"`
			Nodes   *int64  `json:"nodes"`
		}
		var body patchBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
			return
		}

		fields := []struct {
			col  string
			val  any
			flag bool
		}{}
		if body.Name != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"name", *body.Name, true})
		}
		if body.Type != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"type", *body.Type, true})
		}
		if body.Level != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"level", *body.Level, true})
		}
		if body.Status != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"status", *body.Status, true})
		}
		if body.Desc != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"`descr`", *body.Desc, true})
		}
		if body.Alert != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"alert", *body.Alert, true})
		}
		if body.Version != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"version", *body.Version, true})
		}
		if body.Nodes != nil {
			fields = append(fields, struct {
				col  string
				val  any
				flag bool
			}{"nodes", *body.Nodes, true})
		}

		if len(fields) > 0 {
			sets := make([]string, len(fields))
			args := make([]any, 0, len(fields)+1)
			for i, f := range fields {
				sets[i] = f.col + "=?"
				args = append(args, f.val)
			}
			args = append(args, id)
			if _, err := database.Exec(
				`UPDATE scenarios SET `+strings.Join(sets, ",")+` WHERE id=?`,
				args...,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新剧本失败：" + err.Error()})
				return
			}
		}
		// 无论如何都更新 updated 日期
		if _, err := database.Exec(`UPDATE scenarios SET updated=? WHERE id=?`, util.DateStr(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新剧本时间失败：" + err.Error()})
			return
		}
		c.JSON(200, gin.H{"ok": true})
	}
}

// ---------- DELETE /api/content/knowledge/:id — 删除知识库文档 ----------
func DeleteKnowledge(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		// KB_EDIT_ROLES: platformadmin / commander / itlead
		kbEditorRoles := map[string]bool{
			"platformadmin": true, "commander": true, "itlead": true,
		}
		if !kbEditorRoles[u.Role.ID] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "仅平台管理员/院应急指挥部/信息科负责人可维护知识库文档"})
			return
		}

		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文档ID不能为空"})
			return
		}

		var exists bool
		_ = database.QueryRow(`SELECT COUNT(*) FROM knowledge WHERE id=?`, id).Scan(&exists)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
			return
		}

		if _, err := database.Exec(`DELETE FROM knowledge WHERE id=?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文档失败：" + err.Error()})
			return
		}
		c.JSON(200, gin.H{"ok": true, "id": id})
	}
}

// ---------- B 块: knowledge multipart 上传 ----------
// 权限: KB_EDIT_ROLES（与 Node content.js 对齐）
var kbEditRoles = map[string]bool{
	"platformadmin": true, "commander": true, "itlead": true,
}

// BAD_EXT: 禁止上传可执行/脚本类文件（与 Node upload.js 对齐）
var kbBadExt = regexp.MustCompile(`(?i)\.(exe|bat|cmd|vbs|scr|msi|com|pif|jar|ps1|sh|dll|sys|lnk)$`)

// PostKnowledge POST /api/content/knowledge —— multipart 上传知识库文档
// 存储: uploadRoot/knowledge/{YYYYMM}/{ts}-{rand6}-{safeName}
func PostKnowledge(database *db.DB, uploadRoot string, maxMB int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 权限
		u := auth.CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if !kbEditRoles[u.Role.ID] {
			util.Forbidden(c, "仅平台管理员/院应急指挥部/信息科负责人可维护知识库文档")
			return
		}
		// 限制大小
		maxBytes := int64(maxMB) * 1024 * 1024
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)

		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			util.BadRequest(c, "表单解析失败: "+err.Error())
			return
		}
		name := strings.TrimSpace(c.PostForm("name"))
		cat := strings.TrimSpace(c.PostForm("cat"))
		if cat == "" {
			cat = "上传文档"
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			util.BadRequest(c, "请选择要上传的文档: "+err.Error())
			return
		}
		defer file.Close()
		orig := strings.TrimSpace(header.Filename)
		if orig == "" {
			orig = "文档"
		}
		size := header.Size
		if size > maxBytes {
			util.BadRequest(c, fmt.Sprintf("单个文件不能超过 %dMB", maxMB))
			return
		}
		// 扩展名黑名单
		ext := strings.ToLower(filepath.Ext(orig))
		if kbBadExt.MatchString(ext) {
			util.BadRequest(c, "禁止上传可执行程序/脚本类文件: "+orig)
			return
		}
		displayName := name
		if displayName == "" {
			displayName = strings.TrimSuffix(filepath.Base(orig), ext)
		}
		if len([]rune(displayName)) > 100 {
			util.BadRequest(c, "文档名称不能超过 100 个字符")
			return
		}

		// 自动编号 KB{N+1}
		var maxSeq int
		database.QueryRow(`SELECT COALESCE(MAX(CAST(SUBSTRING(id,3) AS UNSIGNED)),0) FROM knowledge WHERE id LIKE 'KB%'`).Scan(&maxSeq)
		newID := fmt.Sprintf("KB%d", maxSeq+1)

		// 目录: uploads/knowledge/{YYYYMM}/
		now := time.Now()
		subDir := now.Format("200601")
		targetDir := filepath.Join(uploadRoot, "knowledge", subDir)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			util.Fail(c, 500, "创建上传目录失败: "+err.Error())
			return
		}

		// 文件名: ts-rand6-safeName
		safeOrig := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`\/:*?"<>|`, r) {
				return '_'
			}
			return r
		}, filepath.Base(orig))
		if len(safeOrig) > 120 {
			safeOrig = safeOrig[:120]
		}
		buf := make([]byte, 3)
		rand.Read(buf)
		randHex := hex.EncodeToString(buf)
		storedName := fmt.Sprintf("%d-%s-%s", time.Now().UnixMilli(), randHex, safeOrig)
		fullPath := filepath.Join(targetDir, storedName)

		outFile, err := os.Create(fullPath)
		if err != nil {
			util.Fail(c, 500, "保存文件失败: "+err.Error())
			return
		}
		defer outFile.Close()
		if _, err := io.Copy(outFile, file); err != nil {
			os.Remove(fullPath)
			util.Fail(c, 500, "文件落盘失败")
			return
		}

		relPath := filepath.ToSlash(filepath.Join("knowledge", subDir, storedName))
		updated := now.Format("2006-01-02")
		sizeStr := fmtSizeGo(size)

		_, err = database.Exec(
			`INSERT INTO knowledge (id,name,cat,updated,size,pages,content,file_path) VALUES (?,?,?,?,?,?,?,?)`,
			newID, displayName, cat, updated, sizeStr, 0, "", relPath,
		)
		if err != nil {
			os.Remove(fullPath)
			util.Fail(c, 500, "写入知识库失败: "+err.Error())
			return
		}

		util.OK(c, gin.H{
			"ok":  true,
			"id":  newID,
			"url": "/uploads/" + relPath,
		})
	}
}

// fmtSizeGo: bytes → "KB/MB/GB" 字符串（与 Node util.fmtSize 对齐）
func fmtSizeGo(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
