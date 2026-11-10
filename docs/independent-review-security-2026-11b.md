# 网络安全应急演练系统 · 独立安全评审报告（渗透/攻防视角）

| 项目 | 内容 |
|---|---|
| 评审对象 | 网络安全应急演练系统 v0.8.3（Go + Gin + MySQL 8 + 纯静态前端） |
| 评审环境 | 现网 `http://192.168.97.131:8090`（只读探测）；代码 `e:\Trae\Emergency` |
| 评审日期 | 2026-11-08 |
| 评审方式 | 黑盒只读探测 + 全量源码白盒审查（独立完成，未参考历史评审结论后再回来做差集） |
| 硬约束执行情况 | 未改现网任何文件、未写生产库、未重启/重建容器、未推仓库；破坏性用例（改密/删数据/越权写）全部未执行 |

> 说明：本报告在完成独立结论后才回看历史评审文档，第 8 节给出差集。
> 凡"代码证实"项，均给出可复核的 `文件:行`；凡未能在真实环境复现的，明确标注"未验证/未用真实账号复现"。

---

## 0. 结论速览

| 编号 | 等级 | 问题 | 状态 |
|---|---|---|---|
| P0-1 | P0 | 外部单位（卫健/网信/网安）可通过无鉴权过滤的附件/取证/表单接口 + 可枚举任务号，批量拉取全院演练原始材料 | 代码证实 + 任务号现网复现 |
| P0-2 | P0 | 任意院内账号可上传 `.html/.svg` 附件，`/uploads` 同源内联渲染 → 存储型 XSS → 管理员会话/口令失守 | 代码证实（动态利用未执行） |
| P1-1 | P1 | 匿名大屏接口泄露完整演练项目、任务号、岗位、处置时间线、评分与短板 | 现网复现 |
| P1-2 | P1 | 图形验证码答案以明文 SVG（base64）下发，自动化防护失效 | 现网复现 |
| P1-3 | P1 | 知识库/台账等内部资料对所有登录账号（含外部单位）无差别可读、可下载 | 代码证实（现网匿名 401） |
| P1-4 | P1 | 生产 MySQL 3306 对内网开放（MySQL 8.0.46 握手可达） | 现网复现 |
| P2-1 | P2 | 现网 CORS 为 `Access-Control-Allow-Origin: *` | 现网复现 |
| P2-2 | P2 | 登录接口在验密前泄露账号状态与角色持有情况，且不计入失败锁定 | 代码证实 |
| P2-3 | P2 | JWT 可经 URL `?t=` 传递，导出/审计链接把完整令牌写进地址栏与日志 | 代码证实 |
| P2-4 | P2 | 审计日志 CSV 导出存在公式注入（用户名可控） | 代码证实 |
| P2-5 | P2 | `sessions.token` 明文入库 + 备份含全库 → 备份=活会话+口令散列；admin 恢复功能可执行任意 SQL | 代码证实（未下载现网备份） |
| P2-6 | P2 | `TRUSTED_PROXIES` 为空时 Gin 默认信任任意 `X-Forwarded-For`（现网实测未生效） | 代码证实 + 现网反证 |
| P2-7 | P2 | 未认证的失败登录会无限写入审计表（配合验证码可解可放大/污染日志） | 代码证实 |

**一句话结论**：认证与角色模型本身做得不差（JWT+会话表双校验、改密/停用清会话、SQL 全参数化、上传路径做了穿越防护），但**"外部单位只读隔离"只做在部分 GET 接口上**，而**文件层（`/uploads`、附件、取证下载）完全没有对象级授权**；同时**上传白名单缺失（黑名单）把一个"文件存储"问题升级成了同源 XSS**。这两条叠加，足以让一个最低权限的窗口收费账号或一个外部单位账号打穿整个演练数据面，并进一步威胁平台管理员。

---

## 1. 评审范围与方法

覆盖：JWT 鉴权、现网 12 个启用角色（含 3 个外部单位角色 `healthbureau/cac/cyberpolice`）与权限模型、匿名大屏、上传/下载（`/uploads`）、报告导出、备份恢复、初始口令与改密、审计日志、迁移器、SQL 注入面、XSS/CSRF、路径穿越、横向/纵向越权、敏感信息泄露。

方法：
1. **只读黑盒**：对现网发 GET/HEAD/OPTIONS 与验证码接口（不产生业务写入），验证匿名暴露面、CORS、限速、路径穿越、版本信息。
2. **白盒**：逐条核对路由 → 中间件 → handler → SQL，构造"低权限/外部单位"攻击者的可达路径。
3. **不做的部分**：未用真实账号登录（无凭据，且避免误锁现网账号）；未做改密、删除、越权写等破坏性验证（按约束应在隔离环境执行，本轮无可用隔离环境，故全部以代码证据定级）。

---

## 2. P0 级问题

### P0-1 外部单位读隔离形同虚设：附件/取证/表单/资料可被全量拉取

**漏洞点**

1. `GET /api/tasks/:id/attachments` 只挂了"登录 + 外部单位禁止写（GET 放行）"，handler 里没有任何角色、项目、任务归属校验：

```go
// server/internal/router/router.go:338-352
tasks := api.Group("/tasks", authSvc.RequireAuth(), authSvc.RequireNotExternalWrite())
...
tasks.GET("/:id/attachments", handler.GetTaskAttachments(database))
tasks.GET("/:id/forms", handler.GetTaskForms(database))
```

```go
// server/internal/handler/missing_handlers.go:1468-1483
id := c.Param("id")
rows, _ := database.Query(`SELECT id,name,type,size,COALESCE(url,''),uploader,time,COALESCE(auto,0)
                           FROM task_attachments WHERE task_id=? ORDER BY id`, id)
```

```go
// server/internal/handler/missing_handlers.go:2089-2105  （表单提交同样无鉴权）
rows, _ := database.Query(`SELECT id,form_id,form_name,COALESCE(data,''),submitter,time
                           FROM form_submissions WHERE task_id=? ORDER BY id`, id)
```

2. `GET /api/evidence/:id/download` 没有任何归属判断，登录即可按 ID 下载任意取证文件：

```go
// server/internal/handler/missing_handlers.go:3010-3042
err := database.QueryRow(`SELECT COALESCE(name,''),COALESCE(file_path,'') FROM evidence WHERE id=?`, id).Scan(&name, &filePath)
...
c.FileAttachment(full, sanitizeFileName(name))
```

3. 对比同文件的列表接口，**外部单位过滤是明确实现过的**，说明这是遗漏而不是设计：

```go
// server/internal/handler/bootstrap.go:1157,1173-1178
isExt := u != nil && auth.IsExternal(u.Role.ID)
...
if isExt {
    if !allowedTypes[typeV] && !externalEvidenceRoles[roleV] {
        continue
    }
}
```

4. `/uploads/*` 只校验"令牌有效"，不校验文件归属；外部单位账号同样可访问：

```go
// server/internal/router/router.go:373-403 uploadAuth：只做 UserByToken + MustChange 判断
// server/internal/router/router.go:441
http.ServeFile(c.Writer, c.Request, full)
```

5. 任务号完全可预测：`T2026-001-01` 这种格式，且**匿名大屏已经把项目号和任务号直接吐出来**（见 P1-1 现网证据）。也就是说外部账号不需要列举，匿名接口就能拿到枚举起点：

```go
// server/internal/handler/drill_handler.go:352
prefix := "T" + strings.ReplaceAll(projectID, "P", "") + "-"
```

**证据（现网 + 代码）**

```http
# 匿名访问受保护接口均 401（说明接口本身要求登录）
GET /api/tasks/T2026-001-01/attachments        -> 401
GET /api/evidence/E1/download                  -> 401
GET /api/content/knowledge                     -> 401

# 但匿名大屏直接给出项目号/任务号（复现于 2026-11-08 09:0x）
GET /api/screen/monitor                        -> 200
{"projects":[{"id":"P2026-001", ...}],
 "tasks":[{"id":"T2026-001-01","role":"duty",
           "title":"HIS 数据库服务器数据文件被批量加密、数据库服务不可用","status":"已完成"},
          {"id":"T2026-001-02", ...}]}
```

外部单位（`healthbureau` / `cac` / `cyberpolice`）拿到任意一个有效令牌后，只需：

```http
GET /api/tasks/T2026-001-01/attachments
-> [{"name":"...","url":"/uploads/tasks/202611/xxx.pdf", ...}, ...]
GET /uploads/tasks/202611/xxx.pdf              # 带上 /uploads 的媒体 Cookie 即可
```

**利用条件与影响**

- 条件：拥有任意一个外部单位账号（角色已存在且现网启用），或拿到任意院内低权限账号。
- 影响：可批量读取全院所有项目的任务附件、取证截图/日志、表单提交（可能含姓名、电话、处置细节）、知识库原件。**"分级权限隔离"实际上只挡住了写和列表展示，没挡住按 ID 直取文件**，属于典型的失效的对象级授权（BOLA/IDOR）。对医院而言，泄露的是网络拓扑截图、系统账号处理过程、应急联系方式与演练短板。

**修复建议**

1. 为 `/uploads/*` 与所有文件下载接口建立**对象级授权**：文件 → 归属任务/项目 → 校验当前用户角色是否在该项目成员/任务角色内，外部单位仅允许其上报材料清单内的文件。
2. `GetTaskAttachments` / `GetTaskForms` / `GetEvidenceDownload` 增加与 `queryEvidence` 一致的外部过滤与项目成员校验；**禁止仅凭登录态按 ID 取数**。
3. 文件 URL 改为**带签名、短时效**的下载票据（如 `?sig=&exp=`），避免"知道路径就能拿"。
4. 任务号/项目号改为不可预测 ID（UUID/随机串），或在外部视图中隐藏。

---

### P0-2 上传 `.html/.svg` + `/uploads` 同源内联渲染 = 存储型 XSS

**漏洞点**

1. 附件上传只做**扩展名黑名单**，且黑名单里没有 `.html`、`.htm`、`.svg`、`.xml`、`.xhtml`、`.mhtml`：

```go
// server/internal/handler/missing_handlers.go:1490-1499
var dangerousAttExts = map[string]bool{
    ".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true,
    ".pif": true, ".msi": true, ".ps1": true, ".vbs": true, ".vbe": true,
    ".js": true, ".jse": true, ".wsf": true, ".wsh": true, ".hta": true,
    ".dll": true, ".cpl": true, ".jar": true, ".sh": true, ".bash": true,
    ".py": true, ".php": true, ".asp": true, ".aspx": true, ".jsp": true,
    ".reg": true, ".lnk": true, ".apk": true, ".bin": true, ".so": true,
    ".dylib": true, ".elf": true,
}
```

```go
// server/internal/handler/missing_handlers.go:1656-1658
if dangerousAttExts[strings.ToLower(filepath.Ext(fh.Filename))] { ... 拒绝 ... }
// 1673：按原始文件名/扩展落盘
rel, full, err := saveUploadedFile(uploadDir, "tasks", fh.Filename, fh)
// 1698：直接以 /uploads/<rel> 对外
"/uploads/" + saved[i].rel
```

知识库上传的黑名单更短（仅拦 `.exe/.ps1/...`），同样允许 `.html`：

```go
// server/internal/handler/content_write.go:279 附近 kbBadExt
```

2. `/uploads` 用 `http.ServeFile` 返回，**没有 `Content-Disposition: attachment`、没有强制 `text/plain`**，扩展名决定 MIME：

```go
// server/internal/router/router.go:441
http.ServeFile(c.Writer, c.Request, full)
```

3. 全站 CSP 允许内联脚本，XSS 直接可执行（现网实抓响应头）：

```http
HTTP/1.1 200 OK
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'self'; form-action 'self'
X-Content-Type-Options: nosniff
```

4. 前端令牌放在 `sessionStorage`，且知识库页面用 `target="_blank"` 打开原件：

```js
// public/system.js  ：登录后 sessionStorage.setItem('pzy_token', token)
// public/knowledge.html:297
'<a ... href="'+d.url+'" target="_blank">下载原件</a>'
```

**利用链（攻击者视角）**

1. 任一院内非外部账号（如 `clinical` 窗口收费、`duty` 值班、`netsec`）在自己负责的任务下上传 `演练材料.html`（内容为读取 `sessionStorage.getItem('pzy_token')` 或伪造登录框）。
2. 该文件落盘为 `/uploads/tasks/202611/<ts>-xxxx.html`，`http.ServeFile` 以 `text/html` 内联返回，CSP 允许内联脚本。
3. 攻击者把链接发到群聊/消息（聊天 `filePath` 字段可自由指定，见 `missing_handlers.go:958` 起的 PostChat），诱导管理员打开：
   - 若在**同一标签页**打开（粘贴地址/同页跳转），`sessionStorage` 中的 `pzy_token` 可被直接读取；
   - 若新标签打开，脚本在 `/uploads` 路径下仍可展示同源页面进行**口令钓鱼**（`form-action 'self'` 允许提交到 `/api/auth/login`），并通过顶层跳转把结果带出（CSP 不拦截顶层导航）；
   - 现代浏览器对 `<a target=_blank>` 默认隐式 `noopener`，会削弱 `window.opener` 取令牌这条路，因此"同标签页打开"是最稳的利用方式。
4. 拿到 `pzy_token` 即等价于平台管理员会话：下载全库备份（含口令散列与在线会话）、重置任意账号口令、上传 SQL 执行恢复等。

**证据**

- 代码链完整（上传 → 落盘 → 静态服务 → CSP）。
- 现网已确认：`/uploads` 路由存在且要求令牌，CSP 允许 `'unsafe-inline'`；`Content-Type` 由扩展名推导。
- **未验证**：未在现网用真实账号实际上传并触发（属写操作且无可用账号），因此标记为"代码证实、动态利用未执行"。

**修复建议（按优先级）**

1. **白名单**：附件/知识库只允许 `pdf/doc/docx/xls/xlsx/ppt/pptx/png/jpg/jpeg/gif/webp/zip/7z/txt/log/pcap` 等必要类型，拒绝一切 `text/html`、`image/svg+xml`、XML 派生类型。
2. `/uploads` 响应统一加 `Content-Disposition: attachment; filename*=UTF-8''...` + `X-Content-Type-Options: nosniff`，并对非图片类型强制 `application/octet-stream`；图片类型单独走独立域名或 `Content-Security-Policy: sandbox`。
3. 删除 CSP 的 `'unsafe-inline'`，改用 nonce/hash（当前全站内联脚本较多，需要同步改造）。
4. 上传后做**内容嗅探**（magic bytes）与图片重编码，防止伪造扩展名。

---

## 3. P1 级问题

### P1-1 匿名大屏泄露完整演练内幕（无需任何凭据）

**漏洞点**

`SCREEN_PUBLIC_VIEW` 默认 `true`（`server/internal/config/config.go:143`），现网已开启；`/api/screen/*` 挂 `OptionalAuth + AnonRateLimit`，不带令牌直接返回聚合数据：

```go
// server/internal/router/router.go:159-175 附近
scr := api.Group("/screen", authSvc.OptionalAuth(), middleware.AnonRateLimit())
scr.GET("/monitor", handler.ScreenMonitorHandler(database))
```

`MonitorContentHandler` 返回 `project/projects/tasks/timeline/seqFlow/chainNodes/score/impact/messages/roles/roleAgg`，其中任务、时间线、节点是**未脱敏的内部处置信息**：

```go
// server/internal/handler/monitor_content.go:115-127  tasks（id/role/title/status/level/完成时间）
// server/internal/handler/monitor_content.go:139-148  timeline（actor/role/action/detail）
```

**证据（现网原文，已截断）**

```http
GET http://192.168.97.131:8090/api/screen/monitor     （无 Authorization）
HTTP/1.1 200 OK     Content-Length ≈ 24KB

{
  "projects": [{"id": "P2026-001", "name": "2026年盘州市人民医院勒索病毒攻击演练", ...}],
  "tasks": [
    {"id":"T2026-001-01","role":"duty","level":"P0",
     "title":"HIS 数据库服务器数据文件被批量加密、数据库服务不可用","status":"已完成",
     "completedAt":"2026-11-07 10:33:14"},
    ...
  ],
  "timeline": [{"actor":"平台系统管理员","role":"platformadmin","action":"创建演练项目",
                "detail":"项目「2026年盘州市人民医院勒索病毒攻击演练」…已创建"}, ...]
}

GET /api/screen/summary   -> 200
GET /api/screen/notifs    -> 200
GET /api/auth/roles       -> 200，返回 12 个启用角色（含 external 标记）
```

**影响**

- 医院真实名称、演练主题、全部任务标题、责任岗位、完成时间、评分与短板、角色清单，全部可匿名获取；
- 直接为 P0-1 提供任务号枚举起点；
- 攻击者可据此绘制"这家医院遇到勒索病毒时谁负责什么、多久完成、哪里薄弱"，用于社工与定向攻击；
- 参与人员还能提前读到 `seqFlow/chainNodes` 的预期处置流程，影响演练公平性与考评有效性。

**修复建议**

1. 生产默认改为 `SCREEN_PUBLIC_VIEW=false`，大屏改用**独立的只读展示令牌**（可轮换、可撤销、权限仅 `/api/screen/*`）。
2. 匿名响应做字段裁剪：去掉 `tasks` 明细、`timeline.detail`、`seqFlow/chainNodes` 的预期动作，仅保留统计与脱敏标题。
3. 对匿名接口加来源 IP 白名单（大屏所在网段）。

---

### P1-2 图形验证码答案明文下发，自动化爆破/撞库防护失效

**漏洞点**

验证码是"算术题 SVG"，虽然加了轻微位移/旋转/干扰线和噪点，但**题目文字本身仍以明文写进 SVG 的 `<text>` 节点**，再整体 base64 返回：

```go
// server/internal/handler/captcha.go:74,87-88  题面 expr 直接写进 SVG，再整体 base64
expr = fmt.Sprintf("%d + %d", a.Int64(), b.Int64())
svg := captchaSVG(expr)
imageB64 = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
// server/internal/handler/captcha.go:189   <text ...>%s = ?</text>
// 验证码本身是一次性 + 绑定 IP（captcha.go:196-221），但这对"同一台机器自动解题"无效
```

**证据（现网复现）**

```http
GET /api/auth/captcha   -> 200
{"id":"...","image":"data:image/svg+xml;base64,PHN2Zy...."}

# base64 解码后直接是：
<svg ...><text ...>34 + 2 = ?</text></svg>
```

即：**不需要 OCR，脚本解码即可得到答案**。

**影响**

- 验证码对自动化完全无效；
- 现网登录锁定是"账号 + IP"维度（`server/internal/handler/login_guard.go`，`LOGIN_MAX_FAILURES` 默认 5 / 15 分钟），单个 IP 对单账号只能试 5 次，但**换账号不受限**：一个内网主机即可对医院全部员工账号做低速撞库/口令喷洒（实测 `/api/auth/captcha` 限速约 20 次/分钟，仍足以批量尝试）；
- 与 P2-2（验密前泄露账号状态/角色、且不计数）组合，可先精准筛出"存在 + 启用 + 持有某角色"的账号，再定向猜口令。

**修复建议**

1. 改用服务端生成的位图验证码（含扭曲/干扰线）或接入滑块/行为验证；**答案绝不能出现在响应中**。
2. 登录失败计数改为"账号 + IP + 全局"三档，并加入指数退避；对"账号不存在/停用/无该角色"也计入（防枚举）。
3. 对连续失败的来源 IP 做短时封禁与告警。

---

### P1-3 知识库/台账等内部资料对所有登录账号（含外部单位）无差别可读

**漏洞点**

`GET /api/content/knowledge`、`/api/content/ledger` 在 `content` 组下只要求登录，handler 直接把全表返回，未接收当前用户、未做任何过滤：

```go
// server/internal/router/router.go:267,269
cnt.GET("/knowledge", handler.GetKnowledge(database))
cnt.GET("/ledger", handler.GetLedger(database))
```

```go
// server/internal/handler/content_basic.go:66-69
func GetKnowledge(database *db.DB) gin.HandlerFunc {
    return func(c *gin.Context) { c.JSON(200, queryKnowledge(database)) }
}
// server/internal/handler/bootstrap.go:1485-1507 全表查询，返回 content 与 /uploads 原件地址
```

同一批数据里，时间线（`queryTimeline`）、取证列表（`queryEvidence`）都做了外部过滤，**知识库/台账却是全量**，属于同类接口的不一致遗漏。

**证据**

- 现网匿名访问 401（说明需要登录）；
- 代码证明登录后无角色/外部过滤；外部单位角色（`healthbureau/cac/cyberpolice`）走同一路由，`RequireNotExternalWrite` 只拦非 GET。
- **未验证**：无外部单位测试账号，未实拉。

**影响**

外部单位或任意低权限院内账号可读取全部知识库正文与附件原件（预案、制度、处置手册）、演练台账（名称、日期、事件、结果、评分、单位），超出"仅查看医院上报材料"的设计边界。

**修复建议**

同 P0-1：为 `knowledge`/`ledger` 增加外部过滤或项目成员校验；知识库按密级/角色分组授权。

---

### P1-4 生产 MySQL 3306 对内网直接开放

**漏洞点**

`deploy/docker-compose.yml:16` 与 `deploy/docker-dist/docker-compose.smoke.yml:23` 已改为 `127.0.0.1:3306:3306`，但现网实际仍把 3306 发布到所有网卡。

**证据（现网复现，只做 TCP 连接与读取握手，不登录、不写库）**

```text
Test-NetConnection 192.168.97.131 -Port 3306   -> TcpTestSucceeded = True
原始握手字节（ASCII 可打印部分）：
J....8.0.46..*..Zcb.4..................l].s.F.gUcjd.mysql_native_password.
```

即任何能到达 `192.168.97.131` 的内网主机都可以直接与 MySQL 8.0.46 建连，绕开应用层的全部鉴权、限流与审计。

**影响**

- 应用改了密码、踢了会话都没用：数据库层面直接暴露；
- 若现网沿用仓库本地 `.env` 中常见的弱口令组合（8 位、字母+年份形态，具体值不复述），等于整库可读写：口令散列、在线会话、全部演练数据一次拿走；
- 即使口令足够强，也把 MySQL 版本指纹、认证方式暴露给内网扫描者，并可被用于 DoS/漏洞利用；
- **未验证**：未尝试数据库登录（避免触碰生产库），因此无法确认口令强度。

**修复**

1. 立即把 3306 收敛为 `127.0.0.1:3306:3306`，或从 compose 中删除 `ports`，仅保留容器网络；用 `ss -lntp` 复核。
2. 数据库账号只允许应用容器网段；开启 `require_secure_transport`（内网可评估）。
3. 轮换数据库口令，并确认 `.env` 不在任何仓库/备份/共享目录中。

---

## 4. P2 级问题

### P2-1 现网 CORS 为通配 `*`

代码默认是"同源、不放开跨域"（`server/internal/config/config.go:148-149`），但现网实测返回通配：

```http
GET /api/screen/summary
Origin: http://evil.example
->
Access-Control-Allow-Origin: *
Access-Control-Allow-Headers: Content-Type, Authorization, X-Drill-Token
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
```

`POST /api/auth/login` 的预检也返回 `*` + 204。

**影响**：任意网站可跨域读取匿名接口、发起登录请求并读取响应。由于鉴权用 `Authorization` 头且令牌在 `sessionStorage`（跨源脚本拿不到），暂无"直接盗用会话"路径；但它扩大了匿名数据面的可达性，也不利于内网钓鱼页面伪装（攻击者页面可静默探测目标系统是否在线、版本与角色）。

**修复**：生产显式配置 `ALLOW_ORIGINS=http://192.168.97.131:8090`（或域名），不要用 `*`；同时去掉不必要的 `X-Drill-Token` 允许头。

### P2-2 登录接口在验密前泄露账号状态/角色持有情况，且不计入锁定

```go
// server/internal/handler/auth.go:70-93
if status != "启用" { ... "账号已停用" ... }          // 验密前
...
if roleID != "" {
    SELECT COUNT(*) FROM user_roles WHERE user_id=? AND role_id=?
    if held == 0 { ... "该账号不具备所选角色" ... }      // 验密前
}
// 96 行才开始 VerifyPassword，且上面两个分支都没有 loginGuardFailure
```

**影响**：未认证攻击者可无限次确认：账号是否存在、是否停用、持有哪些角色（遍历 `/api/auth/roles` 的 12 个角色）。配合 P1-2，可低成本筛出高价值账号（如持有 `platformadmin` 的账号），再定向钓鱼/猜解。审计里也只记录"不具备所选角色"，不触发锁定。

**修复**：把状态/角色判断移到验密之后；对外统一返回"账号或密码错误"；所有失败分支统一计数与延迟。

### P2-3 令牌可经 URL 传递，导出/审计链接把完整 JWT 写进地址栏与日志

```go
// server/internal/auth/auth.go:255-269
// 1. Authorization: Bearer   2. X-Drill-Token   3. ?t=   （文件下载场景）
```

```js
// public/api.js:77,85  项目脚本/通知导出
'/api/content/projects/'+id+'/script/export?fmt=...&t='+encodeURIComponent(sessionStorage.getItem('pzy_token'))
// public/admin.html:691 审计 CSV 导出
'/api/admin/audit/export?t='+encodeURIComponent(sessionStorage.getItem('pzy_token'))
```

**影响**：完整 JWT 进入浏览器历史、代理/访问日志、Referer（当前 `Referrer-Policy: same-origin` 已降低外泄面）、以及"复制链接分享"场景。任何拿到日志/历史的人可直接复用会话（`sessions` 表按令牌明文匹配）。

**修复**：下载类接口改用一次性短时效签名票据；服务端日志对 `t=` 参数脱敏；令牌统一只走 Header。

### P2-4 审计日志 CSV 公式注入

`AdminGetAuditExport` 直接把数据库字段写 CSV，未处理 `= + - @` 前缀：

```go
// server/internal/handler/admin.go:239-245
_ = w.Write([]string{ nullStr(tm), nullStr(usr), nullStr(roleID), nullStr(action), nullStr(ip) })
```

而 `usr` 来自登录请求体的 `username`（失败登录也会落库，`auth.go:65/107`），**未认证即可写入任意字符串**。管理员导出 CSV 用 Excel 打开时，`=cmd|' /C calc'!A0` 之类的公式会被执行（受 Excel 宏安全提示约束）。

**修复**：导出前对所有字段做 `=+-@` 前缀转义（前置 `'` 或空格），或输出 xlsx 并把单元格强制为文本。

### P2-5 备份=活会话+口令散列；恢复功能可执行任意 SQL

```go
// server/internal/auth/auth.go:227
INSERT INTO sessions (token,user_id,role_id,created_at,expires_at) VALUES (?,?,?,?,?)
```

会话令牌**明文**入库，`mysqldump` 全库备份即包含：全部 `pwd_hash/pwd_salt`、全部在线会话令牌、全部演练数据与文件元数据。平台管理员可下载（`/api/admin/backups/:file`），而 `POST /api/admin/restore-upload` 的校验只检查文件前 64KB 是否含 `mysql dump/create table/insert into` 关键字，之后交给 mysql 客户端执行——**等价于以 DB 账号执行任意 SQL**：

```go
// server/internal/handler/system_extra.go:235-257
if strings.Contains(lower, "mysql dump") || ... || strings.Contains(lower, "insert into") { return nil }
```

**影响**：admin 一旦失守（例如被 P0-2 拿到会话），可直接拿全库离线破解并长期驻留；恢复功能把"备份恢复"变成任意 SQL 执行原语。

**修复**：`sessions` 只存令牌哈希（SHA-256）并比对哈希；备份加密（如 age/gpg）；恢复只接受系统自产备份（带 HMAC 清单），并在隔离库校验后再切换；限制恢复 SQL 的语句类型。

### P2-6 `TRUSTED_PROXIES` 为空时 Gin 默认信任任意 `X-Forwarded-For`

```go
// server/internal/router/router.go 仅在 len>0 时 SetTrustedProxies
// gin v1.10 默认 trustedProxies = 0.0.0.0/0、::/0 → c.ClientIP() 取 XFF
```

代码默认配置下，登录锁定与各限速（按 `ClientIP` 计数）可被伪造 XFF 绕过，审计来源 IP 也可被污染。**现网实测未生效**：

- 固定 XFF 连续请求 `/api/auth/captcha`：约 20 次后开始 429；
- 轮换 XFF 请求 `/api/screen/summary` 70 次：前 62 次 200、随后 429（与"真实 IP 桶 + 1/s 回填"一致）。
说明现网大概率配置了 `TRUSTED_PROXIES`（或前置代理覆盖），但代码默认值仍是危险的。

**修复**：显式 `r.SetTrustedProxies(nil)`（不信任任何代理）或只填反代地址；不要把空值当成"自动信任"。

### P2-7 未认证失败登录无限写审计（日志放大/污染）

`loginGuardFailure` 是内存计数，而 `writeAuditRaw` 每次都落库（`auth.go:39/47/65/89/100/107`）。验证码可解（P1-2）后，攻击者可用脚本以 20 次/分钟持续写入无效用户名，审计表无上限增长，真事件被淹没，磁盘/查询性能受影响。

**修复**：对"账号不存在""验证码错误"类事件做聚合（按 IP+时间窗写入一条），并设置审计表保留策略/分区。

---

## 5. 已核查、未发现问题的点（避免误伤）

- **SQL 注入**：未发现用户输入拼接进 SQL。动态 UPDATE 的列名来自服务端常量/白名单（`server/internal/handler/admin.go:419`、`missing_handlers.go:344/430` 均为固定语句串），值全部参数化；`ORDER BY` 无用户可控字段。
- **路径穿越**：上传落盘使用 `sanitizeFileName`（`filepath.Base` + 替换 `\ / : * ? " < > |`），下载侧 `filepath.Clean + 逐段拒绝 ".."/隐藏段 + Rel/HasPrefix` 校验（`router.go:418-434`）；现网 `--path-as-is` 的 `/uploads/../.env`、`/%2e%2e/.env` 均为 401/404，未读到敏感文件。
- **JWT 伪造**：`Parse` 限制 HMAC 算法并校验 `exp`；且 `UserByToken` 还要求 `sessions.token` 精确匹配，**即使拿到 JWT 密钥也无法凭空造出可用令牌**（需要同时写库）。
- **CSRF**：业务接口只认 `Authorization/X-Drill-Token/?t`，不认 Cookie；`/uploads` 的 HttpOnly Cookie 仅 Path=/uploads、SameSite 默认 Lax，且只影响文件读取。未构成可利用 CSRF。
- **会话吊销**：改密（`auth_extra.go:218`）、停用账号（`admin.go:475`）、删除用户（`admin.go:519`）、删除角色（`admin.go:662`）均会清理 `sessions`，代码路径完整。
- **初始口令**：种子账号从环境变量或随机生成口令，`must_change=1` 强制改密；新建用户同样强制改密；改密策略 8 位以上且含字母+数字。
- **越权改分/结束演练**：`POST /api/content/evaluation/scores`、`PATCH /api/projects/:id` 等均挂 `RequirePlatformBoard`；`PostDrillDecide` 有角色与状态校验。
- **迁移器**：`cmd/migrate` 使用 MySQL advisory lock、事务执行、`reset` 需 `-yes` + `DRILL_ALLOW_RESET=1` 双确认，未见注入或误清库风险。
- **点击劫持**：`X-Frame-Options: SAMEORIGIN` + `frame-ancestors 'self'`，外部站点不能嵌套。

---

## 6. 若明天上线到真实医院内网：最可能被怎么打穿（前三名）

**第一名：从一个"窗口收费/值班"账号到平台管理员（P0-2 + P2-5）**

医院内网最大的现实是"人多、账号多、终端杂"。攻击者拿到任意一个最低权限账号（弱口令、共享工作站、离职账号未停用）→ 在自己任务下上传 `演练材料.html` → 把链接发到演练群/消息 → 管理员在同一标签页打开 → `pzy_token` 被读走 → 下载全库备份（口令散列 + 在线会话）→ 离线破解 + 恢复功能执行 SQL，完成持久化。**这条链不需要任何 0day，全部是配置与授权缺陷。**

**第二名：外部单位账号把全院演练底稿一次性打包（P0-1 + P1-3 + P1-1）**

卫健/网信/网安账号本应"只看医院上报材料"，但附件、取证下载、表单提交、知识库、台账都没有对象级授权；任务号又能从匿名大屏直接拿到（`T2026-001-01`）。一个外部账号可以在几分钟内枚举 `P2026-001` 下的所有任务并拉走附件与截图。**这是最容易被"合规审计/数据安全检查"直接定性为数据泄露的一条。**

**第三名：匿名面 + 验证码失效摸清家底后做定向钓鱼（P1-1 + P1-2 + P2-2）**

先匿名读大屏确认医院名称、演练主题、责任岗位与时间线；再解验证码、用登录接口在验密前枚举"哪些账号存在、持有哪个角色"；最后对信息科负责人/指挥部成员做定向口令猜测或伪造"演练通知"钓鱼。**它不一定立刻打穿，但能显著提高第一、二名的成功率。**

> 并列风险（可能比以上更快）：现网 3306 对全内网开放（P1-4）。任何一台能路由到该 VM 的机器都可以直接连 MySQL；若数据库口令延续了仓库 `.env` 的弱口令习惯，攻击者甚至不需要碰应用层，直接整库拖走/篡改——这是"最低成本、最高收益"的一条路径，建议上线前优先确认。

---

## 7. 修复顺序建议（投入产出）

1. **一周内**：`/uploads` 全量改 `Content-Disposition: attachment` + 上传白名单（P0-2 止血）；给附件/取证/表单/知识库/台账补齐对象级授权与外部过滤（P0-1/P1-3）。
2. **两周内**：关闭匿名大屏或改展示令牌；验证码换实现；登录失败统一计数并移到验密后（P1-1/P1-2/P2-2）。
3. **一个月内**：会话令牌哈希化、备份加密与签名、恢复流程隔离校验；CORS 收紧；`?t=` 改签名票据；CSV 公式转义；显式 `SetTrustedProxies`；审计表保留策略（P2 全部）。

---

## 8. 与历史评审的差集

> 本节在完成上述独立结论后，回看 `docs/security-hardening-2026-11.md`、`docs/system-review-engineering-2026-11.md`、`docs/system-review-business-2026-11.md` 后补写。只做对照，不改动前文结论。

### 8.1 上轮提过、本轮认为已修复或无法复现

| 上轮问题 | 上轮位置 | 本轮核查 |
|---|---|---|
| 匿名 `/api/auth/accounts` 返回 12 个账号（姓名/单位/角色） | 工程评审 P0-4、P1-10 | **已修复**：现网匿名请求返回 401；代码改为 `RequireAuth + RequireAdmin` |
| 登录页匿名账号选择器 | 工程评审 P0-4 | **已修复**：`index.html` 改为手输账号（代码） |
| `/uploads` 匿名可下载证据截图 | 工程评审 P1-10、加固文档第 3 节 | **已修复"匿名读"**：现网无令牌 401，媒体 Cookie 需先登录。⚠️但对象级授权仍缺失，见 P0-1 |
| 初始口令 + 强制改密未闭环 | 工程评审 P0-4、加固文档第 2 节 | **已修复（代码）**：受限 scope token、仅放行改密接口、改密吊销旧会话；现网未登录验证 |
| 低权限/外单位可写：删任务、启停项目、改影响面/舆情、写审计 | 工程评审 P0-3、P1-6 | **代码已修**：`guardTaskWrite` / `RequirePlatformBoard` / `RequireCommanderOrAdmin`；`PostAudit` 已忽略客户端条目。受只读约束未逐个发写请求复测 |
| 证据上传"假成功" | 工程评审 P0-1 | **已修复（代码）**：`PostEvidenceUpload` 已落盘+入库+magic 校验；`PostEvidenceLegacy` 接收 dataUrl |
| 备份上传恢复为空实现 | 工程评审 P0-2 | **已修复（代码）**：`AdminRestoreUpload` 已实现；但校验仍弱、且现网是否安装 mysql 客户端**未验证** |
| 大屏匿名泄露真实姓名 | 工程评审 P1-10、加固文档 5.2 | **姓名脱敏已生效**：现网 timeline 的 actor 为角色名；但任务/流程/任务号仍全量，见 P1-1 |
| 导出 HTML 未转义项目名（XSS） | 工程评审 P2-11 | **降级/未复现**：当前 `GetProjectScriptExport` 只返回项目名，且为附件下载；未在浏览器触发 |
| 审计客户端可注入任意 action | 工程评审 P1-6 | **已修复**：`PostAudit` 返回 ignored，审计改由服务端中间件写入 |

### 8.2 上轮提过、但本轮确认仍存在（含"代码改了、现网没落地"）

| 问题 | 上轮说法 | 本轮事实 |
|---|---|---|
| 验证码答案明文 | 加固文档第 6 节明确"仍是 SVG 文本算术题…本轮先做限流" | 仍存在，且现网可直接 base64 解码取答案 → P1-2 |
| MySQL 3306 暴露 | 加固文档称三套 compose 已改 `127.0.0.1` | **现网仍对全内网开放**（MySQL 8.0.46 握手可达）→ P1-4 |
| CORS 通配 | 加固文档称默认已留空、`.env` 已改 | **现网仍返回 `Access-Control-Allow-Origin: *`**（部署配置未更新）→ P2-1 |
| 令牌放 query `?t=` | 工程评审 P2-4 | 仍存在，且新增审计 CSV 导出也带 `t=` → P2-3 |
| 大屏匿名 | 工程评审列为"需裁决"的产品取舍 | 现网仍开启；本轮补充了"任务号成为 IDOR 枚举起点"的利用链 → P1-1 / P0-1 |
| 备份不含 `uploads/`、恢复校验弱 | 工程评审 P1-8、P0-2 | 恢复仅做关键字校验（代码），备份完整性**未验证**；另发现会话令牌明文入备份 → P2-5 |

### 8.3 上轮未提、本轮新发现

1. **P0-2 上传 `.html/.svg` → 同源存储型 XSS**：上轮只夸了"扩展名黑名单+大小限制"，未检查黑名单缺 HTML/SVG，也未检查 `/uploads` 无 `Content-Disposition`。这是本轮最关键的增量。
2. **P0-1 的完整利用链**：上轮把对象级授权写在 P1-10 的"修复建议"里（`/api/files/:id` 校验项目/角色），但实际只落地了登录鉴权；本轮确认附件、取证下载、表单提交、知识库、台账均无对象级过滤，且任务号可匿名获取。
3. **P1-3 知识库/台账对外部单位全量可读**：上轮未单列该接口（只提了时间线/取证过滤）。
4. **P2-2 登录验密前的账号状态/角色 oracle，且不计入锁定**：上轮关注"匿名账号清单"，未关注登录响应本身可枚举。
5. **P2-4 审计 CSV 公式注入**：审计导出是加固后新增功能，上轮评审时不存在。
6. **P2-5 `sessions.token` 明文入库 + 备份含在线会话 + 恢复=任意 SQL**：上轮备份问题聚焦"是否真的恢复"，未涉及令牌存储形态。
7. **P2-6 `TRUSTED_PROXIES` 为空时 Gin 默认信任任意 XFF**：上轮未提；本轮现网实测未生效，但代码默认值危险。
8. **P2-7 未认证失败登录无限写审计**：上轮只提验证码限流，未评估审计表写放大。

### 8.4 对上一轮"已修复"结论的保留意见

- 上一轮把 `/uploads` 的整改描述为"附件目录鉴权（P1-10）已完成"。本轮认为这只解决了"未登录能不能读"，**没有解决"登录后能不能读不该读的"**——在有多角色（含外部单位）的系统里，后者才是主要威胁，因此不能视为闭环。
- 上一轮把"扩展名黑名单"列为优点。本轮认为对"可被浏览器执行的静态文件"必须用**白名单 + 强制下载**，黑名单天然漏项（本轮就漏了 `.html/.svg/.xml`）。
