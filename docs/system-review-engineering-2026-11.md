# 网络安全应急演练系统 功能与工程质量评审报告

- 评审日期：2026-11-07
- 评审版本：0.6.0（仓库 `server/internal/version/version.go` 与现网 `/api/health` 一致）
- 评审对象：Go + Gin + MySQL 8（容器）+ 纯静态 HTML/JS 前端；`docker compose` 部署于 UOS
- 评审方式：**只读**。阅读仓库代码与文档；在 192.168.97.131 上仅做匿名只读接口抽样；未登录业务页面、未写库、未改代码、未重启容器、未推仓库
- 与既有文档的关系：`docs/s0x-*-expert-review.md` 属剧本/内容层评审，本报告不重复其结论，只评功能与工程质量

---

## 0. 结论摘要

| 级别 | 数量 | 含义 |
|---|---|---|
| P0 | 4 | 缺陷 / 安全风险 / 会误导操作，须优先处置 |
| P1 | 11 | 应当修，影响可感知或影响数据可信度 |
| P2 | 13 | 优化建议，不阻塞上线 |

**整体判断：架构骨架是合格的，甚至有几处明显高于同类项目的工程基线（见第 5 节）。但"最后一公里"存在两类系统性问题：**

1. **假成功**：证据上传、备份上传恢复均返回成功却未真正执行，且界面明确提示"已归档/恢复成功"——这是本轮最需要立刻处理的风险。
2. **校验只做了一半**：实战版节点链路有完整的角色/项目状态/重复提交/质量规则校验；而基础版任务、项目启停、影响面/舆情/流程节点写入、审计写入仍是"登录即可写"，外单位只读角色也可操作。近期 S01 的外单位整改只覆盖了实战版路径。

**上线建议：** 可继续试运行，但 P0-1/P0-2/P0-3/P0-4 建议在 24–48 小时内处置或临时关闭入口（见第 7 节）。P1 中的"历史评分漂移"和"剧本保存覆盖"建议在本轮演练收尾前完成，否则报告结论不可复用。

---

## 1. 证据等级与现网抽样

| 等级 | 说明 | 本报告使用方式 |
|---|---|---|
| A | 现网已复现 | 仅限匿名接口（健康检查、账号清单、大屏态势） |
| B | 代码静态确认，路径明确 | 绝大多数缺陷，已定位到函数与行号 |
| C | 代码推断，需登录现网复核 | 已在条目中标注"需现网复核" |

现网只读抽样结果（2026-11-07）：

| 检查项 | 结果 |
|---|---|
| `GET http://192.168.97.131:8090/api/health` | 200，`version=0.6.0` |
| `GET /api/auth/accounts`（匿名） | **200，返回 12 个账号**，字段含 username / 姓名 / 所属单位 / 角色 |
| `GET /api/screen/monitor`（匿名） | **200，约 22 KB**，含项目信息、17 条任务、timeline（含操作人与动作）、消息、评分、影响面 |
| 响应安全头 | `X-Frame-Options: SAMEORIGIN`、CSP 存在（含 `unsafe-inline`） |
| 登录后页面 | 按纪律未操作；涉及鉴权后的结论均标注为代码证据 |

> 说明：`/api/screen/*` 的匿名可读由 `SCREEN_PUBLIC_VIEW` 控制（默认开启，`server/internal/config/config.go`）。这是产品取舍，列入第 6 节待裁决，不在 P0 中直接判错。

---

## 2. P0（缺陷 / 安全风险 / 会误导操作）

### P0-1 取证材料模块是半成品：上传不落盘、抓屏不入库、预填/需求接口恒空，但界面均提示成功

**现象**
1. **文件上传**：上传 PDF/图片提示"N 份材料已上传并归档"，当场列表可见；刷新后记录全部消失，返回的 `/uploads/evidence/...` 链接 404。服务端既没有落盘，也没有写 `evidence` 表。
2. **抓屏归档**：截图在弹窗里可见，点击"归档"提示"屏幕截图已归档"；实际只写入一条没有图片、没有备注、没有节点关联的记录，刷新后无法再查看截图。
3. **按剧本预填材料**：接口恒返回 `count: 0`，面板永远为空。
4. **证据需求清单**：接口恒返回 `[]`，"本角色需取证的材料"永远为空，导致导调无法核对取证完成度。

**证据**
- `server/internal/handler/missing_handlers.go:1637-1655`：`PostEvidenceUpload` 只拼出 `saved := "/uploads/evidence/<时间戳><后缀>"` 并返回，未调用 `saveUploadedFile`，无 `INSERT`。
- `server/internal/handler/missing_handlers.go:1594-1619`：`PostEvidenceLegacy` 的请求体只有 `type/name/size/auto/taskId/projectId`，**丢弃了前端提交的 `dataUrl`、`note`、`nodeId`、`reqKey`**，因此抓屏图片不落库、也无法与节点/取证要求关联。
- `server/internal/handler/missing_handlers.go:1621-1634`：`PostEvidencePrefill` 返回 `{ok:true,count:0}`；`GetEvidenceRequirements` 返回空数组。
- `public/api.js:298-310`：`uploadEvidence` 把接口返回的文件对象直接 `push` 进内存 `MOCK.EVIDENCE`，成功提示完全由前端给出。
- `public/evidence.html:226-229, 281-291`：上传与抓屏的成功提示均由前端 toast 给出。
- `server/internal/router/router.go:141`：`/uploads` 为静态目录；文件不存在即 404。

**复现步骤（代码路径即可复现）**
- 取证材料页 → 上传任意文件 → 看到"已上传并归档" → 刷新页面 → 列表为空；打开返回的 URL → 404。
- 取证材料页 → 抓屏 → 归档 → 刷新页面 → 记录还在但无图片可看。
- 取证材料页 → "按剧本预填材料"/"本角色取证要求" → 始终为空。

**影响面**
- 核心合规证据链断裂：演练结束导出报告时"人工上传证据"为空，但现场人员认为已归档。
- 抓屏图片不落库意味着"截图证据"只剩一行文字记录，无法在复盘/审计中出示。
- 取证需求与预填是外单位/岗位履职核对的基础，恒空会让导调误判"没人取证"或"已全部取证"。
- 属于"假成功"，比直接报错更危险。

**修复方案**
1. 复用已写好的上传逻辑：`server/internal/handler/missing_handlers.go:1147-1205` 的 `saveUploadedFile`（含扩展名黑名单、随机文件名、大小限制）。
2. `PostEvidenceUpload` 解析 `multipart` 的 `files`，逐个落盘到 `uploads/evidence/`，并 `INSERT INTO evidence(id,type,name,size,time,role_id,uploader,user_id,auto,task_id,project_id,url,file_path,note,node_id,req_key,created_at)`；任一步失败整批回滚（参考 `PostTaskAttachments` 的事务写法）。
3. `PostEvidenceLegacy` 补 `DataUrl/Note/NodeId/ReqKey` 字段：抓屏建议改为"先把 PNG 落盘再写 `url/file_path`"（不建议把 base64 直接塞进 MySQL，`evidence` 会迅速膨胀）；确实要存 base64 时单独建表。
4. 实现 `requirements`：按 `project_id + script_nodes.evidence` 要求与当前角色返回清单；实现 `prefill` 或**直接移除**该按钮与文案。
5. 返回值补齐 `id/url/size/name`；前端 `uploadEvidence` 改为使用服务端返回并重新拉取 `bootstrap`，禁止再写 `MOCK.EVIDENCE`。
6. 若本轮无法实现，**立即关闭入口**：路由返回 501，前端按钮置灰并说明"暂未开放"，避免继续产生"已归档"的错觉。
7. 回归用例：上传 → 刷新 → 记录仍在；下载返回 200 且字节一致；抓屏归档后刷新仍能查看图片；需求清单与剧本一致。

---

### P0-2 备份"上传恢复"是空实现；容器部署下备份/恢复命令缺失；备份产物与 UI 接受的格式不一致

**现象**
管理员页"从备份文件恢复"上传任意文件后提示"恢复成功，请重新登录"，实际数据库没有任何变化。另有两个连带问题：
1. 生产镜像未安装 `mysqldump`/`mysql` 客户端，容器部署下"立即备份"和"从备份列表恢复"会返回 500；
2. 系统生成的备份是 `.sql`，但上传控件只接受 `.db`，即使实现了恢复也选不中自家备份。

**证据**
- `server/internal/handler/missing_handlers.go:1737-1746`：`AdminRestoreUpload` 读取文件后直接 `return {ok:true,file:...}`，无任何恢复动作。
- `public/admin.html:136`：`accept=".db"`；`public/admin.html:746-754`：成功后提示"恢复成功，请重新登录"。
- `server/internal/handler/system_extra.go:80-138`：备份用 `mysqldump` 生成 `drill-backup-*.sql`；`system_extra.go:198-285`：恢复用 `mysql` CLI 导入。
- `deploy/docker-dist/Dockerfile:106`：运行时仅安装 `tini wget ca-certificates tzdata`，**未安装 mariadb-client**；`findMysqldump` 仅查 PATH 与固定路径。
- `docs/linux-deployment.md:634` 也承认 Docker 部署下备份需 `docker exec` 手工执行，与页面按钮的能力不一致。

**影响面**
- 灾备演练与真实回滚都会得到"成功"的错误结论，属于最高风险的功能缺陷。
- 演练前"一键备份"失败会直接阻断保障动作。

**修复方案**
1. **上传恢复**：实现"校验 → 落盘到备份目录 → 复用 `AdminRestore` 的导入逻辑"。校验文件头必须包含 `-- MySQL dump` 或 `CREATE TABLE`，拒绝非 SQL；文件名只用服务端生成名。
2. **镜像**：`deploy/docker-dist/Dockerfile:106` 增加 `mariadb-client`；或改为纯 Go 逻辑导出/导入，去掉外部命令依赖（更利于跨平台）。
3. **格式统一**：`accept=".sql"`，文案去掉 `.db`；若确需兼容旧 `.db`，明确提示"仅支持本系统导出的 .sql"。
4. 当外部命令不可用时返回 501 并在界面禁用按钮，**不得返回 ok**。
5. 回归用例：备份 → 删除一条 `tasks` → 从列表恢复 → 记录恢复；上传非法文件 → 400 且当前库不受影响。

> 该条中"容器内无 mysqldump"为代码 + Dockerfile 证据（B 级）；因未登录现网管理端，未在现网点击验证，列为**需现网复核**。

---

### P0-3 一批写接口只校验"已登录"，外单位只读角色也能改分、启停演练、删任务

**现象**
同一文件里，`PostTaskAttachments` 有完整的角色、项目状态、外单位拦截；但任务完成/删除/重置、任务表单提交、项目启停、项目成员、影响面、舆情、流程节点、审计写入都没有任何权限或状态校验。路由只挂了 `RequireAuth`。

**证据（接口 → 现有校验 → 后果）**

| 接口 | 路由 | 校验 | 后果 |
|---|---|---|---|
| `POST /api/tasks/:id/accept` | `router.go:330` | 无（`missing_handlers.go:996`） | 任意账号可把任务置为"进行中" |
| `POST /api/tasks/:id/complete` | `router.go:332` | 无（`missing_handlers.go:1022`） | 任意账号可"完成"任务，直接影响基础版评分 |
| `POST /api/tasks/:id/submit` | `router.go:331` | 无（`missing_handlers.go:1005`） | 为任意任务写入"(模拟)"证据（`auto=0`） |
| `DELETE /api/tasks/:id` | `router.go:333` | 无（`missing_handlers.go:1057`） | 硬删任务及附件/证据，历史报告口径变化 |
| `POST /api/tasks/reset-all` | `router.go:329` | 无（`missing_handlers.go:1069`） | 全局把所有项目的"已完成"任务重置为"待处理" |
| `POST /api/tasks/:id/forms` | `router.go:337` | 无（`missing_handlers.go:1382`） | 为任意任务写表单、证据、影响面 |
| `POST /api/projects/:id/activate` | `router.go:189` | 无（`missing_handlers.go:812`） | 任意账号切换大屏当前项目 |
| `PATCH /api/projects/:id/status` | `router.go:190` | 无（`missing_handlers.go:830`） | 任意账号开始/暂停/结束/归档演练，且无状态迁移白名单 |
| `PATCH /api/content/projects/:id/members` | `router.go:283` | 无（`missing_handlers.go:443`） | 任意账号改项目成员，不校验项目是否存在 |
| `PATCH /api/content/impact/:sys` | `router.go:295` | 无（`missing_handlers.go:215`） | 任意账号改大屏影响面 |
| `POST /api/content/opinion` | `router.go:293` | 无（`missing_handlers.go:258`） | 任意账号伪造舆情条目上大屏 |
| `POST /api/content/seqflow` | `router.go:290` | 无（`missing_handlers.go:178`） | 重复追加流程节点，且不清旧数据 |
| `POST /api/content/audit` | `router.go:291` | 无（`missing_handlers.go:240`） | 任意账号写入任意 `action`，审计可信度受损 |
| `POST /api/content/snapshots` | `router.go:289` | 无 | 任意账号写快照 |

对照组（说明这是遗漏而非设计）：`PostTaskAttachments` 在 `missing_handlers.go:1217-1237` 做了 `External` 拦截、`canManageTask` 校验和"已归档不可写"判断。

补充：`PatchImpact`（`missing_handlers.go:215-236`）只更新 `activeProjectID()` 指向的项目，且 `pct == 0` 时直接跳过、`UPDATE` 错误被 `_` 丢弃——即"把影响面改回 0%"是无效操作却仍返回 `ok`，属于典型的静默失败。

**影响面**
- 外单位（卫健/网信/网安）本应只读，却可通过接口改影响面、伪造舆情、结束演练——直接违背近期"外单位职责与表单整改"的目标。
- 已结束/已归档项目仍可被改写，导致复盘报告失真。
- 任意参演人员可把全部待办"完成"，基础版评分失去意义。

**修复方案**
1. 在 `server/internal/handler/` 抽出统一的 `guardTaskWrite(u, taskID) (project, task, error)`，内部完成：任务存在 → 所属项目未归档 → `canManageTask` → `!u.Role.External`。`accept/complete/submit/forms/delete` 全部改为调用它（`reset-all` 单独限定 `platformadmin/commander`）。
2. 项目级写操作增加 `guardProjectOperator(u, projectID)`：仅 `platformadmin/commander/itlead`；`status` 增加状态迁移表（如 `草稿→演练中→暂停→已结束→已归档`，禁止 `已归档→演练中`），非法迁移返回 409。
3. `impact/opinion/seqflow/audit/snapshots` 按"指挥/导调/平台管理员"收敛；`audit` 只允许服务端写，客户端接口删除或标记 `source=client`。
4. 在 `router.go` 增加路由级测试（可复用 `cmd/verify_routes` 的清单），对 12 个角色 × 全部写路由做 403/200 断言，防止回归。

---

### P0-4 初始口令 + 强制改密未闭环，配合匿名账号清单可直接登录未改密账号

**现象**
- 管理员新建/重置用户时，若未配置 `DRILL_INIT_PWD`，密码固定为 `CHANGE_ME`，并写入 `must_change=1`。
- 登录接口返回 `mustChange`，但**前端没有任何一处读取它**，后端也不阻断其他接口——强制改密实际不存在。
- 登录页匿名接口返回全部 12 个账号（含姓名、单位、角色），页面还明确提示"初始密码见 .env 的 DRILL_INIT_PWD"。

**证据**
- `server/internal/handler/admin.go:316-318`（新建默认 `CHANGE_ME`）、`admin.go:495-505`（重置默认值）。
- `server/internal/handler/auth.go:160`（登录响应含 `mustChange`）；全仓库 `public/` 检索 `mustChange` **0 命中**；后端 `RequireAuth` 不检查 `must_change`。
- `server/internal/router/router.go:174`：`GET /api/auth/accounts` 无鉴权；现网实测 200 且返回 12 个账号（A 级）。
- `public/index.html` 登录页注释/提示直接指向 `.env` 的 `DRILL_INIT_PWD`。

**影响面**
- 内网任意一台终端 → 打开登录页 → 拿到账号清单 → 用统一初始口令尝试；**若该账号仍在使用初始口令，可直接登录（含 `platformadmin`）**。
- 不满足等保对"初始口令强制修改""口令复杂度"的基本要求。

**修复方案**
1. **服务端闭环**：`Login` 成功且 `must_change=1` 时，只签发 scope=`password_change` 的受限 token；`RequireAuth` 对非改密接口返回 403 `需先修改初始密码`；前端登录后据此强制跳转改密页。改密成功后再签发正式 token。
2. 初始密码改为首次启动随机生成（写入 `.env` 或首次安装日志一次性展示），取消固定 `CHANGE_ME`；`admin` 创建/重置也走同一口令策略（当前 `AdminPostUsers` 未校验复杂度）。
3. `GET /api/auth/accounts` 收敛字段：只返回 `username + 显示名`（或改为"输入账号后按需校验"），不再暴露角色与单位；同时加验证码/限流。
4. 删除登录页"初始密码见 .env"的提示文案。
5. 登录成功/失败写入 `audit_logs`（当前仅 `slog`，见 P1-6）。
6. 回归用例：新建用户 → 首登未改密 → 调 `/api/tasks` 应 403；改密后正常。

---

## 3. P1（应当修）

### P1-1 评分口径三处不一致，基础版可"一键完成"刷分并伪造证据

**现象**
1. 考评页文案写"基础版……考评仅统计**处置时长达成率**（按时完成率 × 100 分）"，实际 `Score()` 是按节点分值扣分，两者不是同一算法。
2. 实战版对表单做必填 + 质量规则校验；基础版 `PostTaskForms` 只校验下拉选项，必填/质量规则完全不生效。
3. 基础版"完成本步骤"调用 `PostTaskSubmit`，服务端写入一条名为 `任务处置记录-xxx(模拟)` 的证据，且 `auto=0`，在报告中被计为"人工上传"。

**证据**
- `public/evaluation.html:84` 的说明文案；`server/internal/handler/drill_score.go:538-616` 的实际扣分逻辑。
- `server/internal/handler/missing_handlers.go:1417-1421`（仅选项校验）对比 `drill_v1.go` 中的必填 + `checkQuality`。
- `server/internal/handler/missing_handlers.go:1005-1019`（写入"(模拟)"证据、`auto=0`）；`public/report.js` 证据统计按 `auto` 计数。

**影响面**
- 同一场演练在考评页、报告页、大屏看到的分值解释不同，复盘时无法自证。
- 基础版无需任何处置记录即可满分，且报告显示"人工取证 1 件"，结论不可信。

**修复方案**
- 二选一统一口径：① 按文案实现"按时率 × 100"；② 保留扣分算法并修改 `evaluation.html:84` 文案为"按节点分值与超时扣分，满分 100"。
- 抽出 `validateFormSubmission(templateFields, values, qualityRules)`，`PostTaskForms` 与 `PostDrillDecide` 共用；基础版同样执行必填与质量规则。
- `PostTaskSubmit` 写入的证据改 `auto=1`，名称去掉"（模拟）"改为"系统引导记录"，报告统计口径区分"人工/系统"；若基础版不要求证据，则在评分中不计该条。

---

### P1-2 历史项目的评分与报告会随剧本修改而漂移

**现象**
某场演练已结束并导出报告；此后在剧本编辑器调整节点分值、删除节点或改依赖，再打开历史报告，分数与节点标题会变化。

**证据**
- `server/internal/handler/drill_score.go` 的 `Score()` 每次按 `project.scenario_id` 现读 `script_nodes` 计算。
- `server/internal/handler/script_nodes_write.go:1019-1034`：保存时按节点 id 增量更新，并删除"未出现在本次提交中"的节点（无软删、无版本校验）；`projects` 表无剧本快照字段（`migrations/0001_init.up.sql`）。

**影响面**
- 复盘报告、评比结果、上级报送材料不可复现；历史分值无法审计。
- 与"演练报告导出与复盘合并"的定位冲突。

**修复方案**
- 项目创建时把当前剧本节点快照到 `project_script_nodes`（新表，含 `project_id` + 节点全字段 + `scenario_version`），`Score()`、报告、节点详情一律读快照。
- 或在"结束演练"时写入 `project_score_snapshot`（总分、每节点得分、口径版本），历史报告只展示快照；剧本修改仅影响新项目。
- 迁移用 `0020_*.up.sql`，对存量项目回填当前剧本作为快照并标记 `snapshot_source=backfill`。

---

### P1-3 剧本保存以"本次提交的节点集合"为全量真相，无并发保护；节点数统计口径错误

**现象**
- 两名导调同时编辑同一剧本，后保存者会覆盖先保存者，且被删节点无提示、无恢复入口。
- 每次保存都会自动递增版本号（minor），但没有变更说明与差异记录，"批量改动的可追溯性"不成立。
- 保存成功后提示"共 N 个节点"，N 只统计非干扰项，导致编辑器里 52 个节点、列表显示 36 个。

**证据**
- `server/internal/handler/script_nodes_write.go:1019-1034`（按提交列表增量更新并删除未提交节点）、`script_nodes_write.go:600-606, 1054-1055`（`nodes = len(correctNodes)`，`correctNodes` 已排除 `flow_type=distractor`）。
- `server/internal/handler/script_nodes_write.go` 的 `resolveVersion`：默认 minor 自增，`UPDATE scenarios` 无 `WHERE version=?` 乐观锁。
- `public/scenarios.html:141` 用 `s.nodes` 渲染"个节点"。

**影响面**
- 剧本是演练的核心资产，覆盖后无法找回；节点数不一致会让导调误判演练规模。

**修复方案**
- 保存请求携带 `expectedVersion`，`UPDATE ... WHERE id=? AND version=?`，影响行数为 0 返回 409 并提示"他人已修改，请刷新比对"。
- 节点表增加 `deleted_at` 软删；保存前把旧版本写入 `script_revisions`（JSON），提供"查看/回滚上一版"。
- `nodes` 字段存**全部节点数**；若产品确实想展示"正确节点"，把字段改名为 `correct_nodes` 并同步前端文案。
- 保存时把 diff（新增/删除/改分/改依赖）写入 `audit_logs`，作为可追溯依据。

---

### P1-4 导调中心"节点操作情况"永久空态；非管理员打开协同待机页会整页报错

**现象**
1. 协同页（`collab.html`）承诺"演练节点操作情况盘面……实时显示处置中/已完成节点"，但该接口后端恒返回空数组，面板永远显示空态。
2. 协同页待机分支调用被覆盖后的 `API.getDict`，非 `platformadmin` 会 403，随后整页被替换为"数据加载失败"。

**证据**
- `server/internal/handler/missing_handlers.go:539-557`：`GetDrillProgress` 返回 `nodes: []`（后面直接 `c.JSON`）。
- `public/collab.html:50` 的能力描述；`collab.html:83` 使用 `API.getDict`。
- `public/api.js:428` 定义 `getDict → /content/dict`，`api.js:506` 用同名键重新定义为 `getDict → /system/dict/:key`（后者仅 `platformadmin` 可读）；`collab.html:22` 在无项目时调用它，`collab.html:53` 的 `catch` 直接把页面替换为错误态。

**影响面**
- 导调中心看不到各岗位节点进度，只能口头询问；待机页对 11 个非管理员角色不可用。

**修复方案**
- 实现 `drill-progress`：按 `project_id` 关联 `script_nodes`、`drill_steps`、`drill_node_state`，返回 `nodeId/title/targetRole/status/startedAt/finishedAt/roleName`；无法实现则删除该面板与文案。
- `api.js` 中两个同名方法改为 `getContentDict` / `getSystemDict`，调用点同步（`admin.html:1132/1192/1231`、`collab.html:22`）。
- 待机页改用 `Promise.allSettled`，任一接口失败只降级对应区块，不再整页替换。

---

### P1-5 大屏轮询失败时静默保留旧数据，操作员无法察觉"已经断连"

**现象**
大屏 8 秒轮询；接口失败时 `catch` 为空，界面继续展示上一次数据，没有任何"连接中断/数据延迟"提示。断网 10 分钟后，大屏看起来仍在实时运行。

**证据**
- `public/screen.html:508-518`：`refresh()` 的 `catch(function(){})` 无任何状态变化；首屏 `load()` 失败仍会 `showMask`（`screen.html:496-506`），因此"能打开但断连"时反而无提示。

**影响面**
- 指挥中心据过期态势做决策，是本系统最典型的"误导操作"场景。

**修复方案**
- 记录 `lastSuccessAt`；连续 2 次失败显示顶部黄条"数据连接中断，最后更新 HH:mm:ss"，恢复后自动消失并提示"已恢复"。
- 底部状态条常驻显示服务器时间与最近刷新时间；失败时时间停止跳动即可被发现。

---

### P1-6 审计日志不能作为追溯依据：关键操作未记录，且客户端可注入任意条目

**现象**
- 服务端直接写 `audit_logs` 的地方仅 5 处左右；项目启停、剧本保存、用户/角色变更、备份恢复、任务删除等关键操作都没有服务端审计。
- `POST /api/content/audit` 接受任意登录用户提交的任意 `action`，以本人身份写入正式审计表（可伪造"某操作已执行"的事件记录）。
- 审计页只取最近 500 条，无分页、无筛选、无导出。

**证据**
- 全仓库 `INSERT INTO audit_logs` 命中很少；`missing_handlers.go:240-255`（客户端可写任意 action）；`bootstrap.go:1496-1500`（`LIMIT 500`，仅 `platformadmin` 可读）。
- `missing_handlers.go:812-860`（项目启停无审计）；`script_nodes_write.go`（剧本保存无审计）。

**影响面**
- 演练复盘无法回答"谁在何时结束了演练""谁改了剧本分值"；审计表混入客户端自报条目后整体不可信。

**修复方案**
- 增加 Gin 中间件 `AuditWrite()`：对所有非 GET 路由，在响应后写入 `audit_logs(actor, role, method, path, action, ip, result, duration, body_digest)`，敏感字段脱敏。
- 关键业务补充语义化 action（项目启停、剧本保存、用户变更、备份恢复、任务删除）。
- `POST /content/audit` 改为 `source=client` 的独立表或直接删除。
- 审计页增加时间/用户/动作筛选、分页与 CSV 导出。

---

### P1-7 同一节点并发提交可重复计分（无唯一约束）

**现象**
两个终端同时提交同一节点，`SELECT` 判重均通过，随后各插入一条 `drill_steps`，分数被计算两次（或出现重复节点行）。

**证据**
- `server/internal/handler/drill_v1.go:454` 判重后再插入，判重与插入不在同一事务、无锁。
- `migrations/0001_init.up.sql:160-161`：`drill_steps` 只有 `INDEX idx_ds_proj`、`INDEX idx_ds_proj_node`，**无 UNIQUE**。

**影响面**
- 分值虚高、复盘明细重复；"重复点击/断网重试"是演练现场高频动作。

**修复方案**
- 新增 `0020_drill_steps_unique.up.sql`：`ALTER TABLE drill_steps ADD UNIQUE KEY uk_ds_proj_node (project_id, matched_node)`（对存量重复数据先去重，保留最早一条）。
- 插入用 `INSERT ...` 并检查 `affected rows`；重复键返回 409 `该节点已提交，请刷新`。
- 前端提交按钮在请求期间禁用（`workbench.html` 已做二次确认，但仍需请求级幂等键）。

---

### P1-8 备份/恢复完整性不足

**现象**
1. 备份只导数据库，不含 `uploads/`，恢复后附件链接全部失效。
2. `mysqldump` 使用 `--skip-lock-tables` 但未加 `--single-transaction`，在有写入时导出可能不一致。
3. 恢复前的"安全备份"失败被忽略，直接继续导入。
4. 恢复期间不进入维护模式，应用继续写库，导入结果可能被覆盖。
5. 上传恢复（P0-2）不校验文件，也不备份当前库。

**证据**
- `server/internal/handler/system_extra.go:80-138`（仅 `mysqldump`）、`system_extra.go:227-272`（导入）。
- `system_extra.go:239-246`：`if err == nil { ... }`，失败只记日志。
- `system_extra.go:260-272`：导入前删除 `sessions`，无维护模式。

**修复方案**
- `--single-transaction --default-character-set=utf8mb4 --set-gtid-purged=OFF`；同时 `tar czf uploads-*.tar.gz -C <uploadDir> .`，备份清单写入 JSON（含版本、行数、文件数、sha256）。
- 恢复前备份失败即 500 中止；恢复期间设置 `settings.maintenance=1`，中间件对写请求返回 503。
- 恢复后校验关键表行数与 `schema_migrations` 版本，校验失败提示"已回滚到恢复前备份"。

---

### P1-9 迁移与升级的运维风险：演练进行中升级会直接停服

**现象**
- `0019` 迁移内含"若存在演练中项目则失败"的守卫。`docker-entrypoint.sh` 在迁移失败时 `exit`，容器进入重启循环——升级窗口一旦撞上演练，旧版本也回不来。
- 迁移内硬编码 `USE drill_system`，`DB_NAME` 改为其他值会失败。
- `drill-migrate reset` 无二次确认，脚本化执行可直接清库。
- `applyFileTx` 声称"失败回滚"，但 MySQL DDL 自动提交，建表语句不会回滚。

**证据**
- `migrations/0019_s01_residual.up.sql:20-30`；`deploy/docker-dist/docker-entrypoint.sh:88-95`。
- `migrations/0014-0019` 均含 `USE drill_system`。
- `server/cmd/migrate/main.go:124-147`（`runReset` 无确认）、`main.go:188-210`（事务包裹 DDL）。

**影响面**
- 升级必须挑无演练时段，且失败后无法快速回退到旧镜像（旧镜像启动仍会跑新迁移）。

**修复方案**
- 迁移增加 `deferrable` 标记：`0019` 失败时记录 `deferred` 并告警，服务继续启动（该迁移只影响剧本内容，不影响运行）。
- 迁移 SQL 去掉 `USE`，由连接串选择库。
- `reset` 必须显式 `--yes` 且校验环境变量 `DRILL_ALLOW_RESET=1`。
- 文档与注释修正 DDL 回滚语义；发布说明增加"回滚到上一镜像时需同时回退 `schema_migrations` 到对应版本"的步骤。

---

### P1-10 暴露面与传输安全

**现象**
1. `docker compose` 将 MySQL `3306` 发布到所有网卡，局域网可直连数据库。
2. `/uploads` 全量静态开放，无需登录；证据截图一旦 URL 外泄即可匿名下载。
3. CORS 默认 `Allow-Origin: *` 且同时 `Allow-Credentials: true`（规范上不合法），生产未配置白名单。
4. 验证码接口未加独立限流（仅受全局 `/api/*` 300 次/分约束），且答案以明文写入 SVG 文本（`a + b = ?`），可脚本化无限求解；验证码内存表无上限、无主动清理。
5. 登录失败按账号锁定 15 分钟，可被用来恶意锁定他人账号。

**证据**
- `deploy/docker-dist/docker-compose.yml:40-41`；`server/internal/router/router.go:141`。
- `server/internal/middleware/middleware.go`：CORS 段与 `RateLimitByGroup` 仅挂在登录路由（`router.go:160`）。
- `server/internal/handler/captcha.go`：算术式直接进入 SVG 文本。

**修复方案**
- MySQL 端口改 `127.0.0.1:3306:3306`，或删除 `ports` 仅走容器网络。
- 附件改为鉴权下载（`GET /api/files/:id` 校验项目/角色）或使用短时签名 URL；`/uploads` 至少加 `X-Content-Type-Options: nosniff` 与不可执行策略。
- `ALLOW_ORIGINS` 默认改为空（不启用 CORS），确需跨域时显式白名单，并去掉 `Allow-Credentials`。
- 验证码加限流（如 60/分钟/IP）+ 内存上限（LRU 10000）；长期建议改为图片验证码或直接去掉验证码、保留账号锁定 + IP 退避。

---

### P1-11 基础版表单校验与实战版不一致

**现象**
实战版提交节点时校验必填字段、质量规则（最少字数、必含关键词、敷衍词）、证据要求；基础版提交表单只校验下拉选项，空文本可提交，质量规则完全不生效。

**证据**
- `server/internal/handler/missing_handlers.go:1417-1421`（仅校验 select 值是否在选项内）。
- `server/internal/handler/drill_v1.go` 的 `PostDrillDecide`：必填 + `checkQuality` + 证据要求。

**影响面**
- 同一模板在基础版/实战版产出质量不同；基础版评分与报告口径失真。

**修复方案**
- 抽 `validateFormSubmission` 公共服务，两处共用；错误信息返回字段级明细，前端定位到具体输入框。
- 基础版若产品上不要求质量规则，需在考评页明确写"基础版不校验质量规则"，避免用户误解。

---

## 4. P2（优化建议）

| # | 问题 | 证据 | 建议 |
|---|---|---|---|
| P2-1 | HTTP 服务未设置 `ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout`；优雅退出用 `Close()` 而非带超时的 `Shutdown()` | `server/cmd/prodserver/main.go` | 设置 5s/30s/60s/120s；用 `Shutdown(ctx)` 并等待 |
| P2-2 | 无访问日志、无请求 ID、无指标端点；排障只能靠业务 `slog` | `server/internal/router/router.go:38-60` | 增加访问日志中间件（含 request_id、状态码、耗时）；暴露 `/api/metrics` 或至少 pprof 内网白名单 |
| P2-3 | 限流器按 IP 的 `rateStore.limiters` 只增不减（注释称"每分钟扫一次"但未实现）；账号锁定可被用于 DoS | `middleware/middleware.go:74-100`；`handler/auth.go` 锁定逻辑 | 定时清理过期桶；锁定改为"账号 + IP"双维度并支持管理员解锁 |
| P2-4 | 导出/下载把 token 放 query（`?t=`） | `public/api.js` 导出相关函数 | 改为一次性下载票据或 `Authorization` + `fetch` 后 `Blob` |
| P2-5 | 修改密码不撤销其他会话 | `server/internal/handler/auth_extra.go` | 改密后 `DELETE FROM sessions WHERE user_id=?`，保留当前会话或强制重登 |
| P2-6 | 表单填写无草稿，刷新/断网即丢失 | `public/workbench.html`（无 localStorage 草稿） | 输入 2 秒防抖存 localStorage，提交成功清除；提示"已保存草稿" |
| P2-7 | 4K 屏上采样导致大屏文字发虚；触屏一体机被硬拦截 | `public/screen.html:412-420`（`scale` 可 >1）、`screen.html:378-381` | `Math.min(1, s)` 或提供"4K 原生布局"；触屏场景用 `pointer: coarse` 区分，或提供开关 |
| P2-8 | 语音播报在 UOS 无中文音色时回退默认音色，中文可能读不出 | `public/app.js` 语音选择逻辑 | 检测 `zh` 音色，缺失时界面提示"未安装中文语音包"并给出安装命令 |
| P2-9 | 全局 BodyLimit 等于上传上限，multipart 余量会导致临界文件失败 | `router.go:38-60` | 上传路由单独放宽为 `maxMB+1`，或 BodyLimit 按 1.1 倍计算 |
| P2-10 | 删除任务只删行，不删物理附件，产生孤儿文件 | `missing_handlers.go:1057-1068` | 删除前收集文件路径，事务提交后清理；或做定期孤儿扫描 |
| P2-11 | 导出 HTML 未转义项目名与路径参数，打开导出文件存在 XSS 风险 | `missing_handlers.go:457-472`（`GetProjectScriptExport`）、`missing_handlers.go:523-536`（`GetProjectNotifyExport`） | 统一 `html.EscapeString`；`fmt=html` 改为强制附件下载 |
| P2-12 | 无 CI；handler 单测仅 4 个 `*_test.go`，`verify_*` 靠人工执行 | `server/cmd/verify_*` | 把 `verify_routes` 纳入 CI；补权限矩阵测试与迁移幂等测试 |
| P2-13 | 存在两套部署说明（宿主 nginx 反代 vs Docker compose），端口/路径/备份方式不一致 | `deploy/linux/nginx-drill.conf`、`deploy/docker-dist/docker-compose.yml`、`docs/linux-deployment.md` | 明确"现网唯一部署方式"，另一套标注"历史/可选"并同步备份章节 |

---

## 5. 已经做得好的地方（不建议改动）

这些是本项目相对同类系统的优势，修复上述问题时**不要顺手重构掉**：

1. **迁移器**：`GET_LOCK('drill_schema_migrations', 60)` 防并发、`schema_migrations` 版本表、单文件执行、失败即停；0014–0019 还写了"守卫表 + 前置断言"避免脏数据。`server/cmd/migrate/main.go:73-108`。
2. **实战版节点状态机**：`PostDrillDecide` 依次校验演练是否在运行、节点是否属于该剧本、依赖是否完成、岗位是否匹配、是否重复提交、必填与质量规则，并对 `External` 角色硬拦截。这是全系统最完整的一条写路径，应作为其余写接口的模板。`server/internal/handler/drill_v1.go`。
3. **附件上传**：`PostTaskAttachments` 同时校验角色、项目状态、扩展名黑名单、大小，并在事务中写库+落盘，失败清理。`server/internal/handler/missing_handlers.go:1210-1330`。
4. **评分单一来源**：`Score()` 统一产出 `total/items/rate`，大屏、报告、考评页共用，避免了"三处各算一套"。`server/internal/handler/drill_score.go`。
5. **鉴权基础**：bcrypt、常量时间比较、JWT + 服务端 `sessions`（可撤销）、失败计数与锁定、`CurrentUser` 从服务端会话取角色（不信任 token 内角色）。`server/internal/auth/auth.go`。
6. **前端 XSS 纪律**：`app.js` 提供 `esc()`，抽查 `workbench/report/collab/screen` 的 `innerHTML` 均做了转义；CSP 已启用。不是"零风险"，但基线明显好于常见静态页项目。
7. **移动端**：`styles.css` 的 768/480 断点覆盖了导航、左右分栏、表格横向滚动、表单单列、按钮触控尺寸；普通页在手机上可读可操作（这是明确优点，改动布局时请保留断点行为）。
8. **大屏语音**：去重、队列、跨标签锁、watchdog、用户手势解锁、播报开关持久化——考虑得比功能本身更细。`public/app.js:194-300`。
9. **容器基线**：非 root 用户、`tini` 收尸、healthcheck、数据卷持久化、`.env` 注入。`deploy/docker-dist/Dockerfile`、`docker-compose.yml`。
10. **版本与文档一致**：`version.go`、`docs/release-0.6.0.md`、现网 `/api/health` 三者一致；发布说明包含备份与验证步骤。

---

## 6. 需裁决的取舍（不自行判定）

| # | 取舍 | 现状与事实 | 选项 | 影响 |
|---|---|---|---|---|
| A | 大屏是否允许匿名访问 | `SCREEN_PUBLIC_VIEW` 默认开启；匿名 `/api/screen/monitor` 返回约 22 KB，含操作人姓名、timeline、消息 | ① 保持匿名（便于观摩）② 改为一次性大屏 token ③ 内网 IP 白名单 + 匿名 | 安全合规 ↔ 演练现场"开机即看"的便利 |
| B | 登录页账号选择器是否保留 | 匿名 `/api/auth/accounts` 返回 12 个账号的姓名/单位/角色 | ① 保留（降低现场输入成本）② 只留账号+显示名 ③ 完全取消，输账号登录 | 现场效率 ↔ 账号枚举风险 |
| C | 初始口令策略 | 当前 `.env` 固定初始密码 + `must_change` 未强制 | ① 首登强制改密（推荐）② 每账号随机初始密码 ③ 保持统一口令但限制内网 | 交付效率 ↔ 安全合规 |
| D | 基础版是否纳入质量规则/证据要求 | 当前基础版不校验、不要求证据，且"一键完成"生成系统记录 | ① 与实战版对齐 ② 明确标注"基础版不校验" | 改变历史评分口径，需提前告知使用单位 |
| E | 是否冻结剧本快照 | 当前历史分数随剧本变化 | ① 项目级快照（存储增量小）② 仅结束演练时存总分快照 | 报告可追溯 ↔ 改动成本 |
| F | 演练中存在"禁止执行"迁移时是否允许跳过 | 当前迁移失败即停服 | ① 可延迟迁移（推荐）② 维持严格，升级窗口人工避让 | 升级安全 ↔ 服务连续性 |

---

## 7. 修复顺序与回归清单

### 第 1 波（24–48 小时，止血）
1. P0-1 证据上传：实现落盘+入库，或立刻关闭入口返回 501。
2. P0-2 备份上传恢复：实现或禁用；镜像补 `mariadb-client`；`.db` 改 `.sql`。
3. P0-3 写接口越权：先按"外单位只读 + 归档只读"两条最小规则拦截（可先在路由层加中间件），再逐接口补 `guardTaskWrite`。
4. P0-4 强制改密：服务端拦截 + 前端跳转；`/auth/accounts` 收敛字段。

### 第 2 波（本周，恢复数据可信度）
5. P1-1 评分口径统一 + 模拟证据标记。
6. P1-2 历史评分快照。
7. P1-3 剧本乐观锁 + 软删 + 节点数修正。
8. P1-6 服务端审计中间件。
9. P1-7 `drill_steps` 唯一索引。
10. P1-4 节点盘实现/下线 + `getDict` 重命名。

### 第 3 波（迭代，工程化）
11. P1-5 大屏断连提示、P1-8 备份完整性、P1-9 迁移可延迟、P1-10 暴露面收敛。
12. P2 全量。

### 回归清单（建议直接落成用例）
- [ ] 外单位账号：`POST /api/projects/:id/status`、`/api/tasks/:id/complete`、`PATCH /api/content/impact` 均 403
- [ ] 已归档项目：任何写接口 409，前端按钮置灰
- [ ] 证据上传：上传 → 刷新 → 记录仍在 → 下载 200
- [ ] 备份恢复：备份 → 删一条数据 → 恢复 → 数据回来；非法文件 400 且库不变
- [ ] 新用户首登未改密：除改密外全部接口 403
- [ ] 双终端同时提交同一节点：仅 1 条 `drill_steps`，第二次 409
- [ ] 修改剧本后打开历史项目：分数与报告不变
- [ ] 非 `platformadmin` 打开协同待机页：不出现整页"数据加载失败"
- [ ] 大屏断网 30 秒：出现"连接中断/最后更新时间"，恢复后自动消失
- [ ] 演练进行中执行 `0019`：服务仍能启动并给出明确告警

---

## 附录 A：评审范围对照

| 评审范围 | 结论位置 |
|---|---|
| 登录鉴权、失败锁定、验证码 | P0-4、P1-10、第 5 节第 5 条 |
| RBAC 与越权 | P0-3（核心） |
| 待办下发、节点提交与表单 | P0-3、P1-1、P1-11 |
| 证据附件、抓屏 | P0-1、P2-10 |
| 站内消息与语音 | 第 5 节第 8 条、P2-8 |
| 大屏展示 | P1-5、P2-7、待裁决 A |
| 报告与复盘导出 | P1-1、P1-2、P2-11 |
| 系统设置与表单模板编辑 | P1-3、P2-11 |
| 审计日志 | P1-6 |
| 备份恢复 | P0-2、P1-8 |
| 前后端字段契约 | P1-4（`getDict` 覆盖）、P1-1 |
| 错误处理（静默失败/半保存/空态） | P0-1、P0-2、P1-4、P1-5 |
| 评分口径一致性 | P1-1、P1-2 |
| 历史数据兼容 | P1-2、P1-9 |
| 演练流程异常（刷新/断网/重复/超时） | P1-5、P1-7、P2-6 |
| 剧本编辑体验 | P1-3 |
| 迁移机制/日志/密钥/容器/版本/回滚 | P1-9、P2-1/2/13、第 5 节第 1/9/10 条 |
| 安全与合规（上传/XSS/默认凭据/匿名面） | P0-4、P1-10、P2-11、待裁决 A/B |
| 手机端/浏览器/大屏缩放/无障碍 | 第 5 节第 7 条、P2-7 |

## 附录 B：关键证据索引

| 主题 | 文件:行 |
|---|---|
| 写路由仅 RequireAuth | `server/internal/router/router.go:189-190, 283-295, 324-338` |
| 证据上传空实现 | `server/internal/handler/missing_handlers.go:1637-1655` |
| 备份上传空实现 | `server/internal/handler/missing_handlers.go:1737-1746` |
| 备份/恢复命令 | `server/internal/handler/system_extra.go:80-138, 198-285` |
| 镜像缺客户端 | `deploy/docker-dist/Dockerfile:106` |
| 初始口令 | `server/internal/handler/admin.go:316-318, 495-505` |
| 强制改密未消费 | `server/internal/handler/auth.go:160`；`public/` 无 `mustChange` |
| 匿名账号清单 | `server/internal/router/router.go:174`；现网 200/12 |
| 节点判重竞态 | `server/internal/handler/drill_v1.go:454`；`migrations/0001_init.up.sql:160-161` |
| 剧本全量重建 | `server/internal/handler/script_nodes_write.go:1027-1031, 1054-1055` |
| 节点盘空实现 | `server/internal/handler/missing_handlers.go:539-557` |
| getDict 覆盖 | `public/api.js:428, 506`；`public/collab.html:22, 53` |
| 大屏静默失败 | `public/screen.html:517` |
| 审计覆盖 | `server/internal/handler/missing_handlers.go:240-255`；`bootstrap.go:1496-1500` |
| 迁移守卫 | `migrations/0019_s01_residual.up.sql:20-30`；`deploy/docker-dist/docker-entrypoint.sh:88-95` |
| DB 端口/CORS/静态上传 | `deploy/docker-dist/docker-compose.yml:40-41`；`middleware/middleware.go`；`router.go:141` |
