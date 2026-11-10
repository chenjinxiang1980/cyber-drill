# 网络安全应急演练系统 —— 独立工程与功能评审（2026-11，第二轮独立评审）

> 评审人视角：首次接触本系统，不沿用既有评审结论。所有结论均来自本轮代码阅读、静态检查和隔离环境实测。
> 现网：VMware 虚拟机 `192.168.97.131`，应用 0.8.3，MySQL 8 容器，迁移 47 条。
>
> **分级口径**
> - **P0**：会导致演练指挥信息错误、核心视图不可用、数据语义错误或部署/安全重大风险，正式演练前必须修复。
> - **P1**：核心功能受损但有绕行方式，或高风险运维/安全问题，应在上线前修复。
> - **P2**：质量、可维护性、可观测性与体验问题，可排期修复。

---

## 一、验证方式与隔离环境声明

为避免影响现网，本轮采用“临时库 + 临时进程”的方式在虚拟机内验证：

1. 用 `cmd/migrate` 在虚拟机 MySQL 容器内新建临时库 `drill_review_bb`，从空库执行 47 条迁移（迁移目录复制到临时目录后执行，见 P1-3）。
2. 用 `cmd/seed` 初始化基线账号，临时服务监听 `127.0.0.1:18090`，上传目录、数据目录均在 `/home/drill/drill-review/`。
3. 通过临时库完成：登录/强制改密、项目创建、项目启动、节点开始、决策提交、节点跳过、结束演练（dryRun/强制）、评分封存、报告查询、报告导出、取证上传、并发重复提交等实测。
4. 验证结束后已停止临时进程、`DROP DATABASE drill_review_bb`、删除 `/home/drill/drill-review`；复检现网 `projects=1`、`form_templates=19`、`schema_migrations=47`、`/api/health=200`，容器健康，未观察到现网数据被改动。

宿主机仅做代码编辑、`go vet`、Go 交叉编译（Linux）、`node --check`、SSH 与 HTTP 只读探测；未在宿主机运行任何 `go build` 产物。
临时库迁移期间，`0014` 因文件内硬编码 `USE drill_system;` 会切到现网库执行（见 P1-3）。本轮已确认该文件为纯 DML，断言失败自动回滚，现网 `F15/F14/模板数` 与迁移后基线一致。

**已跑通的验证结果（正向）**
- `go vet ./...` 通过；Linux 交叉编译通过；前端 `*.js` 语法检查通过。
- 登录 → 初始口令强制改密 → 受限令牌访问业务接口返回 403（`PASSWORD_CHANGE_REQUIRED`）。
- 实战版：创建 → 启动 → 开始节点 → 决策 → 跳过 → 结束（缺证据返回 `needsException`）→ 强制结束 → 封存快照 v1 → 报告读取封存分值 → 导出。
- 并发 5 次对同一节点提交决策：仅 1 次成功，其余 409，未产生重复处置记录。
- 取证上传：文件落盘、`evidence` 记录写入、可下载。
- 轻量并发（5 线程 × 10 次）：`/content/projects`、`/content/timeline`、`/admin/report`、`/screen/monitor` 在本机数据量下 p95 < 35ms，未见错误。

---

## 二、结论摘要（成熟度判断）

**能上生产吗？**
**不建议直接作为正式生产系统上线。** 系统“骨架”完整：项目/剧本/节点/任务/取证/决策/结束封存/报告/大屏/权限/审计都有实现，演练主流程在单项目、单人操作下可以跑通，封存快照与报告口径基本一致。但本轮实测发现多处“接口返回 200、实际没生效”的静默失败，且集中在演练指挥视图（当前项目、处置中状态、大屏项目选择）与部署迁移安全上。这些不是体验问题，而是会让指挥员看到空盘面或错误项目。

**最大的三个风险**
1. **指挥视图静默失真**：当前演练项目无法真正设置（P0-1），节点“处置中”状态永不落库（P0-2），大屏在多项目时按无排序 SQL 选中已结束项目（P0-3）。三者叠加，演练进行中大屏/协同页可能显示空盘面或上一场演练。实测已复现。
2. **部署与迁移不可重复、暴露面过大**：迁移文件硬编码 `USE drill_system;`，换库名会在错误库执行并在错误库记账（P1-3）；MySQL 3306 与 HTTP 8090 监听 `0.0.0.0`，无 TLS，大屏接口匿名可读（P1-4）。
3. **可运维性不足，故障不可查**：无访问日志，48 处 `_, _ =` 吞掉错误，多个关键接口“假成功”（P1-6）。出现数据不一致时，除写操作审计外几乎没有请求级证据，定位成本高。

**总体成熟度**：功能覆盖约 80%，工程质量中上（参数化 SQL、事务、唯一键、封存快照、审计中间件都有），但“端到端正确性”与“可观测性”尚未达到生产标准。按当前状态，适合**单项目、受控小范围试运行**；修复 P0 与 P1-3/1-4 后可进入生产候选。

---

## 三、问题清单

### P0-1 当前演练项目设置链路整体失效，协同页“节点操作情况”永久空态

**现象**
- `POST /api/projects/:id/activate` 返回 `{"ok":true}`，但 `settings.active_project` 从未写入。
- `GET /api/content/drill-progress` 返回 `{"project":null,"nodes":[]}`，协同处置页始终显示“暂无进行中的演练项目”。
- 系统状态接口读取当前项目恒为空；剧本默认单位读取恒为空。

**证据**
- 写入口：[missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L1042-L1045)
  ```go
  _, _ = database.Exec(
      `INSERT INTO settings (key,value) VALUES ('active_project',?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, id)
  ```
  MySQL 8 中 `key` 作为列名未加反引号会触发 1064。实测（现网 MySQL 只读执行同类语句）：
  `ERROR 1064 (42000) ... near 'key,value) VALUES ("zz",1) ON DUPLICATE KEY UPDATE value=VALUES(value)'`。
- 同类只读语句也失败：`SELECT value FROM settings WHERE key=?` → `ERROR 1064 ... near 'key="brand_unit_name"'`。出现在：
  - [missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L112)（SystemStatus 读取当前项目）
  - [drill_handler.go](file:///e:/Trae/Emergency/server/internal/handler/drill_handler.go#L145)（默认单位）
- 正确写法在 [bootstrap.go](file:///e:/Trae/Emergency/server/internal/handler/bootstrap.go#L107)（使用反引号）和 [system.go](file:///e:/Trae/Emergency/server/internal/handler/system.go#L94)。
- 现网 `settings` 共 15 行，**没有 `active_project`**；本轮临时库调用 activate 返回 200 后仍无该行。
- 下游读取：[missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L604-L611) 直接以 `activeProjectID` 判空并返回空盘面；前端 [collab.html](file:///e:/Trae/Emergency/public/collab.html#L292-L303) 据此渲染“暂无进行中的演练项目”。
- 附加事实：前端未调用 `activateProject`（全局检索无使用点），即即使修好 SQL，也没有正常入口设置当前项目。

**影响**
协同处置页是参演角色查看“谁在处理哪个节点”的主视图，当前永久空态；依赖当前项目的实时看板/系统状态/默认单位均失真。多项目时后端无法确定“本场演练”，为 P0-3 的大屏错配埋下根因。

**复现**
1. 临时库执行 `POST /api/projects/P2026-001/activate` → 200。
2. 查 `settings` → 无 `active_project`。
3. 执行 `GET /api/content/drill-progress?channel=inner` → `project:null, nodes:[]`。
4. 打开 `collab.html` → 节点盘面空态。

**建议修改量级：小改**
- 3 处 SQL 的 `key` 加反引号；`activate` 必须检查并返回错误，不能吞。
- 项目创建/启动时自动写入 `active_project`，或在项目列表提供“设为当前”入口并调用 activate。
- `GetDrillProgress` 支持显式 `projectId`，缺省再回落 `active_project` → 最近“演练中”项目。

---

### P0-2 “开始处置”状态永不落库，处置中节点无法在大屏/协同页出现

**现象**
`POST /api/drill/nodes/:nodeId/start` 返回 200，但 `drill_node_state` 无任何新增行，协同页永远看不到“处置中”。

**证据**
- 写入口：[missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L3694-L3705)
  ```go
  activeID := activeProjectID(database)
  _, _ = database.Exec(
      `INSERT INTO drill_node_state (project_id,node_id,user,user_id,started_at) VALUES (?,?,?,?,?)`,
      activeID, nodeID, u.Name, u.ID, util.Now())
  ```
- 实际表结构（现网 `DESC drill_node_state`）只有 `project_id,node_id,role_id,user,started_at`，**没有 `user_id` 列**，该 INSERT 必然报错，且错误被丢弃。
- 同时 `activeID` 受 P0-1 影响为空字符串，即使列名修好，状态也会写到无项目归属的行。
- 本轮实测：对 S01-2 调用 start 返回 200，`SELECT * FROM drill_node_state` 为 0 行；而同一节点的决策（`/drill/decide`）可正常写入 `drill_steps`。

**影响**
- 协同页“处置中”永远不出现，导调无法判断哪个节点正在被处理。
- 与 P0-1 叠加，“节点操作情况”面板既无项目也无状态数据，实时性名存实亡。

**复现**
1. 启动项目，`POST /api/drill/nodes/S01-2/start {"projectId":"P2026-002"}` → 200。
2. 查 `drill_node_state` → 0 行。
3. 打开协同页 → 无“处置中”。

**建议修改量级：小改**
- 列名改为 `role_id`（或补齐 `user_id` 列并做迁移）；使用请求体/路径中的 `projectId`，不用全局当前项目。
- `Exec` 错误必须返回 500，并记录日志。

---

### P0-3 大屏与评估默认项目选择无排序，多项目时显示已结束演练

**现象**
数据库同时存在“已结束”的 P2026-001 与“演练中”的 P2026-002 时，大屏接口默认返回 P2026-001（已结束），而不是正在进行的演练。

**证据**
- [screen_public.go](file:///e:/Trae/Emergency/server/internal/handler/screen_public.go#L61-L68)
  ```go
  pid := strings.TrimSpace(c.Query("projectId"))
  if pid == "" {
      _ = database.QueryRow(`SELECT id FROM projects WHERE status != '未开始' LIMIT 1`).Scan(&activeID)
  ```
  无 `ORDER BY`、无“演练中优先”。
- [monitor_content.go](file:///e:/Trae/Emergency/server/internal/handler/monitor_content.go#L33-L40)、[evaluation.go](file:///e:/Trae/Emergency/server/internal/handler/evaluation.go#L20-L24) 同样写法。
- 本轮实测：匿名 `GET /api/screen/monitor`（不带 projectId）返回 P2026-001 的场景链（该场已结束），而此时 P2026-002 状态为“演练中”。
- 前端 [screen.html](file:///e:/Trae/Emergency/public/screen.html#L548-L580) 首次加载拿到项目后写入 `sessionStorage` 并固定，后续刷新继续使用错误项目。

**影响**
演练进行中，大屏可能整场显示上一场（甚至已结束）的进度、告警与得分，直接误导指挥席。该问题在单项目部署下不显现，一旦使用“项目/归档”能力就会触发。

**复现**
1. 建项目 A 并结束；建项目 B 并置为“演练中”。
2. 新开无痕窗口访问大屏（URL 不带 projectId）。
3. 大屏显示 A 的名称/进度。

**建议修改量级：小改**
- 统一项目选择函数：`active_project` > 状态“演练中” > “已暂停” > 最近更新时间；全部 SQL 加确定排序。
- 大屏首次加载不写死错误项目，或在页头提供项目切换。

---

### P1-1 “重新考评”按钮必现 404（路由参数名不一致）

**现象**
评估页点击“重新考评”返回 404 `{"error":"演练项目不存在"}`，封存项目无法重算/重封存。

**证据**
- 路由：[router.go](file:///e:/Trae/Emergency/server/internal/router/router.go#L332) `drill.POST("/score/:projectId/recalc", ...)`。
- 处理函数：[evaluation.go](file:///e:/Trae/Emergency/server/internal/handler/evaluation.go#L605-L615) 读取 `c.Param("id")`，恒为空，`Score()` 查不到项目直接 404。
- 前端调用：[evaluation.html](file:///e:/Trae/Emergency/public/evaluation.html#L416-L420) `API.recalcDrillScore`。
- 本轮实测：`POST /api/drill/score/P2026-001/recalc` → 404。

**影响**
评估/复盘页核心动作不可用；已封存项目无法生成新版本快照，也无法修正历史评分。与 P1-1 关联的“重新考评后重封存”设计因此完全失效。

**建议修改量级：小改**
- 改为 `c.Param("projectId")`，补回归测试。

---

### P1-2 时间线/台账/消息不分项目，多项目数据互相混入且前端无法过滤

**现象**
`GET /api/content/timeline`、`/api/content/ledger`、`/api/content/messages` 等返回全库数据；时间线响应项甚至不含 `projectId`，前端无法按项目过滤。

**证据**
- [bootstrap.go](file:///e:/Trae/Emergency/server/internal/handler/bootstrap.go#L1233-L1238)
  `SELECT t,actor,role_id,action,detail,level FROM timeline ORDER BY seq`（无 `project_id` 条件，也不返回 `project_id`）。
- [bootstrap.go](file:///e:/Trae/Emergency/server/internal/handler/bootstrap.go#L1426-L1427) 台账同样全表查询。
- [content_basic.go](file:///e:/Trae/Emergency/server/internal/handler/content_basic.go#L73-L83) 直接透传，不接受 `projectId`。
- 本轮实测：临时库两个项目各 2 条事件，`GET /api/content/timeline` 返回 4 条且无 `projectId` 字段。

**影响**
- 多项目场景下“演练回放/时间线/台账”会把历史项目事件混入当前演练，复盘结论错误。
- 外单位角色看到的是经关键词过滤的全库事件，可能读到其它项目（含未来项目）的信息。
- 现有 `timeline.project_id` 索引完全没用上，纯浪费。

**建议修改量级：中改**
- 上述查询按 `project_id` 过滤并返回 `projectId`；接口增加 `projectId` 参数；无参数时回落到当前项目而不是全表。

---

### P1-3 迁移脚本硬编码 `USE drill_system;`，换库名时在错误库执行/记账

**现象**
从空库执行迁移到临时库时，0014 的更新与断言实际跑在现网库 `drill_system` 上，并按现网状态断言失败，导致空库安装中断。

**证据**
- [0014_external_unit_form_fields.up.sql](file:///e:/Trae/Emergency/migrations/0014_external_unit_form_fields.up.sql#L24-L25) 第 25 行 `USE drill_system;`；同类硬编码还出现在 0015–0019 等共 6 个迁移文件（全局检索 `USE drill_system`）。
- 本轮实测：`DB_NAME=drill_review_bb` 执行 `drill-migrate up`，在 0014 处失败；原因是断言“模板总数 = 18”在现网（实际 19）判定失败，而临时库当时也是 18。证明语句被切到了现网库。
- 现网数据未被改动：0014 为纯 DML，[main.go](file:///e:/Trae/Emergency/server/cmd/migrate/main.go#L236-L253) 对纯 DML 文件包事务，失败回滚；复检现网 F15/F14/模板数一致。

**影响**
- 自定义数据库名/多环境部署时，迁移会读写错误库：轻则安装失败，重则在其它库执行 UPDATE 并提交，造成跨环境影响。
- 迁移链的“从 0 可重放”不成立，灾备重建/新环境交付不可信。

**建议修改量级：小改**
- 删除所有 `USE drill_system;`，完全依赖连接串的 `DB_NAME`；或由迁移器在执行前校验 `DATABASE()`。
- 增加“空库全量迁移 + 结构比对”作为发布门禁。

---

### P1-4 现网暴露面：MySQL 3306 对宿主机可达、应用 8090 明文 HTTP、大屏接口匿名

**现象**
- `drill-mysql` 的 3306 映射到 `0.0.0.0`，宿主机 `Test-NetConnection 192.168.97.131:3306` 成功。
- `drill-app` 8090 对宿主机可达，`http://192.168.97.131:8090/api/health` 返回 200，无 TLS/重定向。
- 匿名访问 `/api/screen/monitor`、`/api/screen/summary`、`/api/auth/roles` 均 200，大屏数据与角色说明对同网段任何人可读。

**证据**
- `docker ps` 端口映射；`ss -lntp` 仅见 8090/3306，无 443。
- 宿主机实测匿名接口返回 200 与业务数据（不在此列出内容）。

**影响**
- 登录口令、JWT 在 HTTP 明文传输；MySQL 暴露到局域网，凭据一旦泄露可直接读库。
- 演练项目名称、节点、告警文本等敏感信息对同网段匿名可读，与“内部演练”定位不符。

**建议修改量级：中改**
- 3306 绑定 `127.0.0.1` 或仅容器网络；应用前置 Nginx/Caddy 做 TLS 与安全头；非大屏终端限制 8090。
- 大屏匿名接口加只读令牌或 IP 白名单。

---

### P1-5 项目列表“结束演练”入口吞掉 `needsException`，点击无反馈

**现象**
在项目列表点“结束”时，若核心证据不足，后端返回 400 `{"needsException":true,...}`，前端没有 catch，页面无任何提示，用户会以为按钮失效。

**证据**
- [projects.html](file:///e:/Trae/Emergency/public/projects.html#L176-L183)：
  ```js
  API.updateProjectStatus(p.id, to).then(function(){ ... });
  ```
  无 `.catch`。
- 后端在 [missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L1051-L1065) 已将“已结束”转交 `executeDrillFinish`，缺证据时返回 400/`needsException`。
- 工作台有完整的“异常结束/补证”弹窗（说明同一条结束流程存在两套入口、行为不一致）。

**影响**
正式演练最常见的情形就是证据不全；此时在项目列表点击结束会静默失败，操作员无法从该入口完成结束，也不知道原因。

**建议修改量级：小改**
- 该入口 catch 400，跳转工作台结束弹窗或就地展示缺失清单；两处统一到同一结束组件。

---

### P1-6 48 处错误被丢弃、无访问日志，故障不可追溯

**现象**
- 全仓 `_, _ = ` 忽略错误 48 处（本轮统计，仅 `internal/handler`、`internal/auth`）。
- Gin 使用 `gin.New()` + `gin.Recovery()`，未挂 `gin.Logger()`，也没有自定义请求日志；本轮完整跑完登录、建项、决策、结束、报告、上传等流程后，`server.log` 只有极少量业务日志，没有任何请求行与 4xx/5xx 记录。

**证据**
- [router.go](file:///e:/Trae/Emergency/server/internal/router/router.go#L26-L27)。
- P0-1/P0-2/P1-5 的“200 假成功”均源于此模式。
- 写操作审计存在（`AuditWrites`），但读接口失败、panic 上下文、慢请求、参数错误的可追溯性不足；审计也无法还原“为什么返回 200 却没写入”。

**影响**
线上出现“数据没写进去/页面空白/评分不对”时，无法从日志还原请求与 SQL 失败原因，只能复现或查库倒推。

**建议修改量级：中改**
- 增加访问日志（路径、状态、耗时、用户、traceId）与错误日志；关键写路径禁止 `_ =`，至少 `slog.Error` + 500。
- 为 SQL 错误统一包装（表名/语句标识），避免暴露细节给前端但保留服务端上下文。

---

### P2-1 演练进行中实时分恒为 100/“优秀”，与完成度严重脱节

**现象**
实战版仅完成 1/19 个节点、另有 1 个必做节点被跳过时，工作台仍显示 `score=100, grade=优秀, completeness=5.3%`。

**证据**
- [drill_score.go](file:///e:/Trae/Emergency/server/internal/handler/drill_score.go#L713-L780) 仅在 `forceEnded` 时对未完成/跳过节点扣分；进行中只扣“错误/超时”等已发生项。
- 本轮实测：`GET /api/drill/workbench/P2026-001` 返回 `score=100, completeness=5.3, skippedCorrect=1, pending=18`。

**影响**
指挥席/大屏若直接展示实时分，会得出“满分优秀”的错误结论；虽属扣分制设计，但缺少“进行中、仅供参考”的显式语义。

**建议修改量级：小改**
- 进行中改展示“完成度/已扣分”或标注“过程分（未结束不计未完成项）”；结束前不显示等级。

---

### P2-2 任务“提交处置记录”会生成 9KB 的“模拟”证据记录（auto=0）

**现象**
调用 `POST /api/tasks/:id/submit` 会写入一条 `evidence`：名称 `任务处置记录-xxx(模拟)`、大小 `9 KB`、`auto=0`，但没有实体文件。

**证据**
- [missing_handlers.go](file:///e:/Trae/Emergency/server/internal/handler/missing_handlers.go#L1369-L1373)。
- 前端 [report.js](file:///e:/Trae/Emergency/public/report.js#L217-L219) 按名称含“模拟”标注为“模拟数据（不计人工）”，因此报告口径尚可自洽；但直接读 API/库的第三方会把它当人工证据。
- 本轮实测：临时库 evidence 中出现该记录。

**影响**
证据链可信度下降；若下游统计只看 `auto` 字段，会高估人工取证。

**建议修改量级：小改**
- 模拟记录写 `auto=1` 或新增 `simulated` 字段；不要伪造 `size`。

---

### P2-3 两条“结束演练”路径写分口径不一致（`scores.MODE` 缺失）

**现象**
`EndProject` 路径会写 `scores` 的 `MODE` 行；当前 UI 使用的 `executeDrillFinish`（封存）不写 `scores`，导致 `scores` 表与封存快照不一致，评估页“考评明细”全为“未评”。

**证据**
- 写 `scores.MODE`：[drill_runtime.go](file:///e:/Trae/Emergency/server/internal/handler/drill_runtime.go#L247-L249)、[drill_runtime.go](file:///e:/Trae/Emergency/server/internal/handler/drill_runtime.go#L616-L618)。
- 封存路径 `executeDrillFinish` 仅写 `score_snapshots/project_end_seal`，全文件无 `INSERT INTO scores`（全局检索确认）。
- 本轮实测：结束并封存后，`/admin/report` 的 `modeScore=null`、`scoreStats` 全部未评，而封存快照评分为 0。

**影响**
报告页面靠“实时分/封存分”兜底显示，暂未失真；但任何依赖 `scores` 表的导出、对接、二次统计都拿不到最终分，数据模型存在双写口径。

**建议修改量级：小改**
- `executeDrillFinish` 同步 upsert `scores.MODE`，或在读取端统一从封存快照取。

---

### P2-4 手工考评写入 `auto=1`，且未校验分值上限

**现象**
评估项（K1–K9）由人工填写，却以 `auto=1` 落库；分值未做 `0..max` 校验。

**证据**
- [evaluation.go](file:///e:/Trae/Emergency/server/internal/handler/evaluation.go#L179-L181) `VALUES (?,?,?,?,?,1,?) ... auto=1`。
- 同函数未对 `score` 与 `max` 做范围校验（仅 `max<=0` 时默认 10）。

**影响**
报告/审计无法区分人工与系统评分；可写入超过满分的数据，破坏统计口径。

**建议修改量级：小改**
- 人工项写 `auto=0`；服务端 clamp 分值并校验 `max`。

---

### P2-5 现网版本展示不一致：应用 0.8.3，页面品牌版本 v0.8.1

**现象**
`/api/health` 返回 0.8.3；`settings.brand_version` 为 `v0.8.1`，前端底部/关于页展示旧版本。

**证据**
- 现网 `/api/health` → `0.8.3`；现网 `settings` 中 `brand_version=v0.8.1`。

**影响**
运维核对版本困难，问题反馈容易错配版本。

**建议修改量级：小改**
- 品牌版本从构建信息读取，或发版脚本同步更新 `brand_version`。

---

### P2-6 现网已结束项目无封存、无评分（历史数据）

**现象**
现网唯一项目 P2026-001 状态“已结束”，但 `project_end_seal=0`、`score_snapshots=0`、`scores=0`。报告页对该项目显示“未考评/未封存”。

**证据**
- 只读查询：`SELECT COUNT(*) FROM project_end_seal/score_snapshots/scores` 均为 0；项目状态已结束。

**影响**
若该项目被视为正式演练记录，则缺少不可篡改评分快照，报告无法作为正式结论；属修复前的历史数据缺口。

**建议修改量级：小改（数据修复）**
- 对该项目补跑一次封存（评估页“重新考评”修好后即可）或按规程标注“历史数据未封存”。

---

### P2-7 `platformadmin` 账号仍处于“首次改密”状态且启用

**现象**
现网 13 个账号中，`platformadmin` 的 `must_change=1`，说明初始口令从未修改，账号仍启用。

**证据**
- 现网只读查询：`users` 中 `platformadmin.must_change=1`、`status=正常`。

**影响**
高权账号长期未使用且口令为安装期初始值；口令强度取决于部署环境变量（本轮未验证），存在被利用风险。

**建议修改量级：小改**
- 未启用的平台管理员应停用或删除；保留则强制首次登录改密并纳入巡检。

---

### P2-8 数据库无外键约束，依赖应用层清理

**现象**
`information_schema.referential_constraints` 中本库外键数为 0；`drill_steps/evidence/timeline` 等均以 `project_id` 逻辑关联。

**证据**
- 现网只读查询：外键约束 0。
- `DeleteProject` 已做了较完整的级联清理与事务，但仅覆盖该入口。

**影响**
任何非 `DeleteProject` 的删除/异常中断都可能留下孤儿数据；`timeline` 混项目问题会被放大。

**建议修改量级：中改**
- 关键表补外键（或定期一致性巡检任务）；至少为 `project_id` 建立孤儿检查 SQL 并纳入运维。

---

### P2-9 同一场景在基础版/实战版的取证要求数不一致（未验证成因）

**现象**
同一 S01 场景，实战版工作台证据清单要求 34 项，基础版为 38 项。

**证据**
- 本轮实测：`/drill/workbench` 的 `evidence.total` 实战版=34、基础版=38（同一 `scenarioId=S01`）。

**影响（未验证）**
若属设计（基础版把任务附件/表单重复计入），应在文档中说明；若属口径错误，会导致两模式评分与报告不可比。

**建议修改量级：待确认（小改或文档澄清）**

---

## 四、分维度结论

| 维度 | 结论 |
|---|---|
| 功能完整性 | 主流程齐全，单项目可跑通；但“当前项目/处置中/重新考评/结束入口”存在静默失败，完整性打折。 |
| 演练全流程可用性 | 实战版：建项→启动→节点→决策→跳过→结束→封存→报告可用；基础版任务闭环可完成。协同页实时状态不可用（P0-1/P0-2）。 |
| 前端交互一致性 | 未发现前端调用后端不存在的路由（逐条对齐通过），XSS 转义使用较规范；问题集中在错误分支未处理（P1-5）与实时分语义（P2-1）。 |
| 后端接口契约 | 路由与前端匹配良好；`recalc` 参数名不一致（P1-1）；`start` 忽略请求中的 projectId（P0-2）。 |
| 数据模型与迁移 | 表/索引基本齐全；无外键（P2-8）；迁移硬编码库名导致不可重放（P1-3）。 |
| 评分引擎 | 封存口径与实时口径分离清晰，快照可追溯；结束路径不写 `scores`（P2-3）、实时分误导（P2-1）。 |
| 大屏与实时数据 | 1920×1080 固定布局符合要求；默认项目选择与当前项目解析错误（P0-1/P0-3）。 |
| 报告与导出 | 前端生成 Word/HTML，读取封存快照，口径基本正确；导出接口仅记审计，不产出文件（设计如此）。多项目时间线混入会污染报告（P1-2）。 |
| 部署与运维 | 容器健康、带健康检查；MySQL/HTTP 暴露、无 TLS（P1-4）；版本展示不一致（P2-5）；无访问日志（P1-6）。 |
| 性能与并发 | 本机数据量下 p95 < 35ms，无明显瓶颈；并发决策由唯一键保护，幂等良好。大报告 N+1/全量取证哈希尚未压测（未验证）。 |
| 代码质量 | 分层清晰、参数化 SQL、事务与审计意识较好；`missing_handlers.go` 6800+ 行过大、48 处忽略错误、无请求日志。 |
| 错误处理与日志 | 最大短板：多处 200 假成功、前端无 catch、服务端无访问日志，故障定位证据不足。 |

---

## 五、与历史评审文档的差集对照

> 本节在形成上述独立结论后阅读历史文档（`system-review-engineering-2026-11.md`、`system-review-business-2026-11.md`、`security-hardening-2026-11.md`、`release-0.8.3.md`、`drill-finish-seal-2026-11.md`、`s01-a-stage-report.md` 等）得到。**未修改前文结论**。

### ① 上轮提出、但本轮未能复现或认为已修复的条目

| 上轮条目 | 本轮结果 | 依据 |
|---|---|---|
| P0-1 取证上传不落盘/抓屏不入库/预填接口恒空 | **未复现（上传已可用）** | 实测 `POST /api/evidence/upload` 返回 200，`uploads/` 有文件、`evidence` 有记录；抓屏流程代码已存在（未逐一实测抓屏） |
| P0-4 初始口令未强制修改、受限令牌可绕过 | **未复现（已修复）** | 初始口令登录返回受限令牌，访问 `/content/projects`、`/admin/users` 均 403 `PASSWORD_CHANGE_REQUIRED`；改密后正常 |
| P1-7 并发重复提交导致重复计分 | **未复现（已被唯一键拦截）** | 5 并发 decide：1×200、4×409；`drill_steps` 仅 1 条 |
| P0-2 备份上传/恢复空实现 | **未能确认已修复（代码已有实现，本轮未做破坏性实测）** | `system_extra.go` 已具备备份、恢复前自动备份、失败回滚与文件名校验；按约束未在现网执行恢复 |
| P0-3 外部单位可调用内部写接口 | **未实测外部账号；代码层已收紧** | content/drill/tasks/evidence 组均挂 `RequireNotExternalWrite`，项目状态变更要求平台领导小组；未创建外部账号做端到端验证 |
| P1-2 封存前后评分漂移 | **未复现（封存快照 + 读取快照已生效）** | 实测结束生成 `score_snapshots v1`，工作台与封存分均为 0，报告读封存值 |
| P1-5 大屏断连静默 | **未复现（页面有失败提示/重试）** | `screen.html` 轮询失败会显示“连接中断/重试”提示 |
| P1-6 审计可被前端伪造 | **未复现（已改为服务端中间件）** | `AuditWrites` 全量非 GET 写审计；`PostAudit` 明确丢弃客户端字段 |

### ② 上轮未提、本轮新发现的条目

| 新发现 | 级别 | 说明 |
|---|---|---|
| `settings.key` 未加反引号导致 activate/系统状态/默认单位 SQL 1064 | **P0**（P0-1） | 历史文档仅在 `s01-a-stage-report.md` 提到过“insert 少反引号”，主评审未列；本轮实测证明其连锁影响（当前项目、协同页空态） |
| `drill_node_state` 表无 `user_id` 列，但 start 接口写入该列 | **P0**（P0-2） | 历史 `phase0-inventory` 记录了正确列结构，但未发现代码写错列；本轮实测 0 行 |
| 大屏/评估默认项目 `status != '未开始' LIMIT 1` 无排序、无“演练中优先” | **P0**（P0-3） | 历史未提；实测大屏选中已结束项目 |
| 时间线/台账/消息接口不分项目、时间线响应无 projectId | **P1**（P1-2） | 历史未提；实测两个项目事件混出 4 条 |
| 项目列表“结束”入口不处理 `needsException`，无任何提示 | **P1**（P1-5） | 历史提过“结束”相关问题，但修复只落在工作台；项目列表仍是静默失败 |
| 封存路径不写 `scores.MODE`，与 EndProject 路径不一致 | **P2**（P2-3） | 历史未提；实测 `modeScore=null` |
| 手工考评写 `auto=1` 且无分值上限校验 | **P2**（P2-4） | 历史未提 |
| 现网 `platformadmin` 仍 `must_change=1` 且启用 | **P2**（P2-7） | 历史未提 |

### ③ 上轮已提、本轮确认仍然存在（未修复）

| 上轮条目 | 本轮状态 |
|---|---|
| 导调中心“节点操作情况”永久空态（P1-4） | **仍存在**，根因补充为 P0-1 + P0-2 |
| “重新考评”路由参数错（`s01-a-stage-report.md`） | **仍存在**，实测 404（P1-1） |
| 迁移硬编码 `USE drill_system`（工程评审 P1-9 / 阶段报告） | **仍存在**，实测换库名时切到现网库（P1-3） |
| MySQL 3306/应用 HTTP 暴露（P1-10） | **仍存在**，宿主机可直连 3306，8090 明文（P1-4） |
| 无访问日志/忽略错误（P1-6） | **仍存在**，48 处 `_, _ =`，仅 Recovery（P1-6） |
| 基础版模拟证据/一键完成刷分风险（P1-1） | **部分存在**：submit 仍写“9KB 模拟”记录（P2-2），但报告已标注“不计人工”，未再观察到直接刷分 |
| 版本展示 v0.8.1 未更新（release-0.8.3 运维项） | **仍存在**（P2-5） |

---

## 六、建议修复顺序（仅按依赖关系，不含工期）

1. **P0-1 + P0-2**：修 `settings` SQL、补 `active_project` 的唯一写入路径；修正 `drill_node_state` 写入列与项目来源。
2. **P0-3**：统一“当前项目”解析函数，大屏/评估/看板共用；前端首次进入大屏可手动选项目。
3. **P1-1**：修 recalc 参数名；回归“结束→封存→重算→重封存→报告”全链路。
4. **P1-3/P1-4**：迁移去掉 `USE`，补空库迁移门禁；收口 3306/8090、加 TLS 与大屏访问控制。
5. **P1-2/P1-5/P1-6**：项目维度过滤、结束入口错误处理、访问日志与错误治理。
6. P2 项按迭代排期。

---

## 七、附：关键证据命令（可复现）

```sql
-- settings 保留字导致语法错误（只读复现）
SELECT value FROM settings WHERE key='brand_unit_name';           -- ERROR 1064
INSERT INTO settings (key,value) VALUES ('zz',1)
  ON DUPLICATE KEY UPDATE value=VALUES(value);                    -- ERROR 1064
-- 现网状态
SELECT COUNT(*) FROM settings;                                    -- 15
SELECT `key`,`value` FROM settings WHERE `key`='active_project';  -- 0 行
DESC drill_node_state;                                            -- 无 user_id 列
SELECT COUNT(*) FROM drill_node_state;                            -- 0
```

```text
# 接口实测（临时库/临时进程）
POST /api/projects/P2026-001/activate            -> 200 {"ok":true}，settings 无 active_project
GET  /api/content/drill-progress?channel=inner   -> {"project":null,"nodes":[]}
POST /api/drill/nodes/S01-2/start                -> 200，drill_node_state 0 行
POST /api/drill/score/P2026-001/recalc           -> 404 {"error":"演练项目不存在"}
GET  /api/screen/monitor（匿名，不带 projectId）  -> 返回已结束项目 P2026-001
GET  /api/content/timeline                       -> 4 条，跨 2 个项目，无 projectId
```
