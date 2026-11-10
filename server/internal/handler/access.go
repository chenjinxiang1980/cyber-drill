package handler

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"drill-system/internal/auth"
	"drill-system/internal/db"

	"github.com/gin-gonic/gin"
)

// =========================================================================
// 对象级授权（P0-1）：任务附件 / 表单 / 取证下载 / 上传文件直链
//
// 规则与 bootstrap.queryTasks / queryEvidence 的既有可见性保持一致，
// 避免“列表看不到、按 id 却能直接读”的 BOLA/IDOR。
// =========================================================================

// externalRoleSet 读取 roles.external=1 的角色集合（含自定义外部角色）。
func externalRoleSet(database *db.DB) map[string]bool {
	out := map[string]bool{
		auth.RoleHealthBureau: true,
		auth.RoleCAC:          true,
		auth.RoleCyberPolice:  true,
	}
	rows, err := database.Query("SELECT id FROM roles WHERE COALESCE(`external`,0)=1")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id sql.NullString
		if rows.Scan(&id) == nil {
			if v := strings.TrimSpace(id.String); v != "" {
				out[v] = true
			}
		}
	}
	return out
}

// canReadTaskData 判断用户能否读取指定任务的从属数据（附件 / 表单）。
// 返回 (allowed, exists)：exists=false 表示任务不存在，调用方应返回 404。
func canReadTaskData(database *db.DB, u *auth.User, taskID string) (bool, bool) {
	if u == nil || strings.TrimSpace(taskID) == "" {
		return false, false
	}
	var roleID sql.NullString
	err := database.QueryRow(`SELECT COALESCE(role_id,'') FROM tasks WHERE id=?`, taskID).Scan(&roleID)
	if err != nil {
		return false, false
	}
	taskRole := strings.TrimSpace(roleID.String)

	// 外部单位：只允许读取归属自己角色的任务数据；
	// 内部任务（duty/netsec/...）一律拒绝，堵住“从大屏拿任务号后直取附件/表单”。
	if u.Role.External {
		return taskRole != "" && taskRole == u.Role.ID, true
	}

	switch u.Role.ID {
	case auth.RolePlatformAdmin, auth.RoleCommander:
		return true, true
	case auth.RoleITLead:
		// 信息科负责人：本院可管理角色 + 外部单位任务（代填场景）
		if taskRole == "" {
			return true, true
		}
		return managedRoles[taskRole] || externalRoleSet(database)[taskRole], true
	}
	if taskRole == "" {
		return true, true
	}
	return taskRole == u.Role.ID, true
}

// canReadEvidenceRow 判断用户能否读取一条取证记录（与 queryEvidence 外部口径一致）。
func canReadEvidenceRow(u *auth.User, typ, roleID string) bool {
	if u == nil {
		return false
	}
	if !u.Role.External {
		return true
	}
	for _, t := range externalEvidenceTypes {
		if t == typ {
			return true
		}
	}
	return externalEvidenceRoles[roleID]
}

// CanReadUploadPath 判断用户能否通过 /uploads 或 /media 读取某个相对路径。
// 供 router 在静态文件出站前调用，避免仅靠“登录即可读任意文件路径”。
func CanReadUploadPath(database *db.DB, u *auth.User, rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "./")
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	// 品牌 Logo 是公开资源：登录页在未登录时也要能显示。
	if strings.HasPrefix(rel, "brand/") {
		return true
	}
	if u == nil {
		return false
	}

	// 取证材料：沿用列表页的外部口径（不叠加任务归属，否则内部其他角色会看不到本可浏览的取证）。
	var typ, roleID sql.NullString
	if err := database.QueryRow(
		`SELECT COALESCE(type,''),COALESCE(role_id,'') FROM evidence WHERE file_path=? LIMIT 1`, rel,
	).Scan(&typ, &roleID); err == nil {
		return canReadEvidenceRow(u, typ.String, roleID.String)
	}

	// 任务附件：与任务列表的角色可见性一致。
	var attTask sql.NullString
	if err := database.QueryRow(
		`SELECT COALESCE(task_id,'') FROM task_attachments WHERE file_path=? LIMIT 1`, rel,
	).Scan(&attTask); err == nil {
		ok, _ := canReadTaskData(database, u, strings.TrimSpace(attTask.String))
		return ok
	}

	// 知识库原件：仅内部角色。
	var kid sql.NullString
	if err := database.QueryRow(
		`SELECT id FROM knowledge WHERE file_path=? LIMIT 1`, rel,
	).Scan(&kid); err == nil {
		return !u.Role.External
	}

	// 未登记的静态资源（品牌 logo 等）：仅内部。
	return !u.Role.External
}

// =========================================================================
// 上传扩展名白名单（P0-2）
//
// 只允许业务确实需要的文档 / 图片 / 压缩包 / 抓包与配置文本；
// 明确排除 html/htm/xhtml/svg/xml/mhtml 等可在同源执行脚本的类型。
// =========================================================================

var allowedUploadExts = map[string]bool{
	// 图片
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
	// PDF / Office
	".pdf": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	// 文本类
	".txt": true, ".log": true, ".csv": true, ".md": true,
	// 压缩包
	".zip": true, ".7z": true, ".rar": true,
	// 抓包 / 配置
	".pcap": true, ".cap": true, ".pcapng": true,
	".cfg": true, ".conf": true, ".ini": true, ".yaml": true, ".yml": true, ".json": true,
}

// inlineImageExts 可在 /media 内联预览的图片类型 → Content-Type。
var inlineImageExts = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp",
}

// blockedUploadSuffix 即使出现在双扩展名尾部也直接拒绝。
var blockedUploadSuffix = []string{".html", ".htm", ".xhtml", ".svg", ".xml", ".mhtml", ".mht"}

// ValidateUploadExt 校验上传文件名是否在白名单内，返回小写扩展名。
func ValidateUploadExt(name string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	ext := filepath.Ext(lower)
	if ext == "" || ext == "." {
		return "", false
	}
	for _, bad := range blockedUploadSuffix {
		if strings.HasSuffix(lower, bad) {
			return ext, false
		}
	}
	if !allowedUploadExts[ext] {
		return ext, false
	}
	return ext, true
}

// InlineImageType 判断扩展名是否为允许内联的图片，返回对应 Content-Type。
func InlineImageType(ext string) (string, bool) {
	mt, ok := inlineImageExts[strings.ToLower(ext)]
	return mt, ok
}

// validChatFileRef 校验聊天消息里客户端提交的 filePath 只能是 uploads 下已存在的白名单文件。
func validChatFileRef(uploadDir, p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	p = strings.TrimPrefix(p, "/uploads/")
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	if _, ok := ValidateUploadExt(clean); !ok {
		return false
	}
	full := filepath.Join(uploadDir, clean)
	rel, err := filepath.Rel(uploadDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	st, err := os.Stat(full)
	return err == nil && !st.IsDir()
}

// =========================================================================
// 知识库 / 台账：外部单位不可见
// =========================================================================

func knowledgeForUser(database *db.DB, u *auth.User) []gin.H {
	if u == nil || u.Role.External {
		return []gin.H{}
	}
	return queryKnowledge(database)
}

func ledgerForUser(database *db.DB, u *auth.User) []gin.H {
	if u == nil || u.Role.External {
		return []gin.H{}
	}
	return queryLedger(database)
}
