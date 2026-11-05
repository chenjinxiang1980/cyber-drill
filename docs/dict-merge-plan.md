# 流程字典并入「系统设置」· 盘点与改造方案

- 版本：0.5.0（不升版本）
- 日期：2026-11-05
- 范围：`public/dictionary.html`、`server/internal/handler/system.go`、字典相关前端引用；不涉及 `public/app.js`、`workbench.html`、`drill_score.go`、`migrations/`、`deploy/`
- 状态：**盘点 + 设计方案 + 非破坏性改造已完成；删除/合并等待确认**

---

## 0. 结论摘要

1. **「决策与干扰项」不是一张独立数据表**，而是 `dict` 表里 `key='drill_action_meta'` 的一行 JSON（1414 字符）。Go 后端**完全没有读取**该键；现在实际使用的是剧本节点 `script_nodes.wrong`（混淆节点）机制。因此删除范围是「1 行字典数据 + 前台编辑界面」，**不存在需要删除的表或列**，`script_nodes.wrong` 必须保留。
2. 字典表共 **19 个 key**，其中 **9 个在当前 Go 生产后端中无任何读取**（详见 §1.3 判定表）：`drill_action_meta`、`drill_form_by_action`、`drill_level_by_scenario`、`fill_roles_by_role`、`managed_roles_by_role`、`kb_edit_roles`、`external_evidence_types`、`external_node_patterns`、`role_feed_mapping`；另有 `difficulties`、`drill_role_by_target`、`target_role_aliases` 在所有前端/后端均无读取。
3. 流程字典页只管理 19 个 key 中的 12 个；`levels / projTypes / eventTypes` 通过 bootstrap 全量下发到 `MOCK.DICT` 被前端读取，但**页面上没有编辑入口**；`difficulties` 等 4 个 key 无入口也无读取。
4. 「系统设置」已有 6 个模块，与字典重叠最明显的是：**权限矩阵 ↔ 角色范围/频道/知识库权限**、**表单模板 ↔ 动作→表单映射**、**通知与备份 ↔ 站内消息模板**。建议把字典页拆解并入这三个模块 + 新增一个「业务字典」页签，然后下线独立字典页。
5. 本次已完成的非破坏性改造：
   - 站内消息模板来源改为后端真实枚举下拉（4 项 + 占位）；
   - 每个事件按后端实际变量给出中文含义/示例，标题与正文各有一排「插入变量」按钮，按光标位置插入；
   - 标题/正文/来源必填校验；未知变量非阻断警告；删除已知事件模板时提示「服务端无兜底、将停止推送」；
   - 移除「决策与干扰项」前端的全部入口、编辑器和保存分支；
   - 后端 `PUT /api/system/dict/notif_templates` 增加来源枚举/必填/等级校验。
6. 待确认的删除项见 §4，**尚未执行任何删表、删列、删字典键、改菜单路由的操作**。

---

## 1. 盘点（证据）

### 1.1 流程字典各分组当前管理的键

页面：`public/dictionary.html`（`DICT_GROUPS`，改造后 5 组 12 项）

| 分组 | 键 | 页面中文名 | 被谁读取 | 页面入口 |
|---|---|---|---|---|
| 角色映射与权限矩阵 | `fill_roles_by_role` | 取证责任范围 | 仅 Node 历史实现 `node/src/routes/evidence.js:29`；Go 无读取 | 字典页 |
| 同上 | `managed_roles_by_role` | 统筹任务范围 | 仅 Node `node/src/routes/tasks.js:22`；Go 用硬编码 `bootstrap.go:490-495` | 字典页 |
| 同上 | `chat_channels` | 会商频道配置 | Go `missing_handlers.go:559,626-629`；前端 `collab.html:22,83`。但 `bootstrap.go:993-1002` 的聊天分组仍用硬编码角色表 | 字典页 + 演练导调 |
| 同上 | `kb_edit_roles` | 知识库编辑权限 | 仅 Node；Go 用硬编码 `content_write.go:274-276` | 字典页 |
| 站内消息模板 | `notif_templates` | 消息模板全集 | Go `exports.go` 的 `RenderNotifTemplate` + 各 `pushNotif` 调用点；前端字典页 | 字典页（本次改造） |
| 流程与表单 | `drill_form_by_action` | 动作→表单模板 | 仅 Node `node/src/drill_plan.js:3`；Go 只用 `script_nodes.form` | 字典页 |
| 同上 | `drill_level_by_scenario` | 演练等级映射 | 仅 Node `drill_plan.js:3`；Go 用硬编码 `drill_handler.go:349-356` | 字典页 |
| 同上 | `script_action_types` | 剧本处置动作 | 前端 `scenarios.html:249`（经 `MOCK.DICT`） | 字典页 + 剧本编排 |
| 证据字典 | `ev_types` | 证据类型枚举 | 前端 `scenarios.html:276`；Node `evidence_plan.js:16` | 字典页 |
| 同上 | `external_evidence_types` | 院外可见证据 | 仅 Node `evidence.js:50`；Go 硬编码 `bootstrap.go:1062` 附近 | 字典页 |
| 节点特征与工作台 | `external_node_patterns` | 院外节点识别 | **无任何读取** | 字典页 |
| 同上 | `role_feed_mapping` | 工作台默认模块 | **无任何读取**（`workbench.html` 未读取） | 字典页 |

### 1.2 页面未管理但存在于 `dict` 的 7 个键

| 键 | 实际读取方 | 判断 |
|---|---|---|
| `levels` | 前端 `scenarios.html:118,208`（经 `MOCK.DICT`） | 有效，但无可视化编辑入口 |
| `projTypes` | 前端 `projects.html:227` | 有效，但无可视化编辑入口 |
| `eventTypes` | 前端 `ledger.html:36` | 有效，但无可视化编辑入口 |
| `difficulties` | **无** | 冗余 |
| `drill_action_meta` | 仅 Node `drill_plan.js:35,153,329`；Go 无 | 已被混淆节点替代 |
| `drill_role_by_target` | 仅出现在注释 `drill_plan.js:3`，无代码读取 | 冗余 |
| `target_role_aliases` | **无** | 冗余 |

> `MOCK.DICT` 的来源：`bootstrap.go:64` 的 `queryDictMap`（`bootstrap.go:1521`）把 `dict` 全表下发，前端 `scenarios/projects/ledger` 从中取 `levels/projTypes/eventTypes/ev_types/script_action_types`。

### 1.3 重复 / 冗余 / 可合并判定表

| 键或功能 | 使用方 | 现状 | 判断 | 依据 |
|---|---|---|---|---|
| `notif_templates` | Go 渲染 + 字典页 | 有效；页面体验差 | **合并到系统设置「通知与备份」** | 与通知发送强绑定，和字典概念无关 |
| `chat_channels` | Go 频道接口 + 演练导调页 | 有效；但聊天分组权限另有硬编码，双源不一致 | **合并到「权限矩阵」** | 同一份权限配置不应分两处 |
| `fill_roles_by_role` | 仅 Node | Go 无读取 | **删除或合并到「权限矩阵」后由 Go 接入**（需确认） | Go 未实现，配置改了不生效 |
| `managed_roles_by_role` | 仅 Node | Go 无读取 | 同上 | `bootstrap.go:490` 硬编码 |
| `kb_edit_roles` | 仅 Node | Go 无读取 | 同上 | `content_write.go:274` 硬编码 |
| `external_evidence_types` | 仅 Node | Go 无读取 | 合并到「表单模板/证据枚举」或删除（需确认） | `bootstrap.go:1062` 硬编码 |
| `drill_form_by_action` | 仅 Node | Go 无读取 | **删除**（需确认） | Go 以 `script_nodes.form` 为唯一真源 |
| `drill_level_by_scenario` | 仅 Node | Go 无读取 | **删除**（需确认） | Go 在 `drill_handler.go:349-356` 硬编码 |
| `drill_action_meta` | 仅 Node | Go 无读取；前台编辑器已移除 | **删除该字典行** | 混淆节点 `script_nodes.wrong` 已替代 |
| `external_node_patterns` | 无 | 只写不读 | **删除** | 全仓库检索无消费 |
| `role_feed_mapping` | 无 | 只写不读 | **删除** | `workbench.html` 不读取 |
| `difficulties` | 无 | 只写不读 | **删除** | 全仓库检索无消费 |
| `drill_role_by_target` / `target_role_aliases` | 无 | 只写不读 | **删除** | 仅注释提及，无读取 |
| `levels` / `projTypes` / `eventTypes` | 前端 `MOCK.DICT` | 有效但无编辑入口 | **并入「业务字典」页签（只读或可编辑）** | 前端 UI 依赖 |
| `ev_types` / `script_action_types` | 前端 `MOCK.DICT` | 有效 | 并入「业务字典」页签 | 前端 UI 依赖 |
| 流程字典页本身 | 19 个 key 的编辑器 | 独立菜单，用户认为突兀 | **下线页面，能力并入系统设置** | 与系统设置职责重叠 |

### 1.4 「决策与干扰项」完整引用图

| 位置 | 内容 | 处理 |
|---|---|---|
| `dict` 表 `key='drill_action_meta'` | 每动作的告警/报告/一般问题 + 5 条干扰项 JSON | **待确认删除**（仅 1 行） |
| `migrations/0004_seed_baseline.up.sql:319` | 种子行 | 不新写迁移；历史迁移保持不动 |
| `node/src/seed_data.js:2103` | Node 种子 | 历史基线，不动 |
| `node/src/drill_plan.js:3,35,153,329` | Node 读取并生成决策/干扰 | 历史基线，不动 |
| `node/src/routes/system.js:108,137` | Node 字典白名单/校验 | 历史基线，不动 |
| `public/dictionary.html` | 导航项、编辑器、提示、保存分支 | **已全部删除** |
| Go `server/**` | **0 处引用** | 无需改动 |
| `migrations/0001_init.up.sql:116`、`script_nodes_write.go:544-582` | `script_nodes.wrong` + 混淆节点编号/锚点校验 | **保留，这是替代实现** |

**结论：没有「决策与干扰项」独立表/列。** 删除影响：前端不再展示该配置；Go 行为 0 变化；历史项目任务/评分不引用该字典行。

### 1.5 「系统设置」现有模块

页面：`public/admin.html`（页签在 56-64 行）

| 模块 | data-t | 主要接口 | 主要表 |
|---|---|---|---|
| 账号管理 | `users` | `/api/admin/users` | `users`、`roles` |
| 角色权限矩阵 | `matrix` | `/api/admin/roles` | `roles`（nav 权限数组） |
| 表单模板 | `forms` | `/api/admin/forms` | `form_templates` |
| 审计日志 | `audit` | `/api/admin/audit` | `audit_logs` |
| 通知与备份 | `ops` | `/api/admin/backup`、`/api/system/reset` | `notifs`、`settings`、备份文件 |
| 品牌设置 | `brand` | `/api/system/brand` | `settings` |

权限矩阵模块清单 `admin.html:51` 中仍有 `['dictionary','流程字典']` 一项；字典页下线时必须同步移除，否则会出现「可见但 404」的菜单。

---

## 2. 目标形态（建议）

### 2.1 模块划分

保留 6 个现有页签，新增 1 个「业务字典」，把字典能力按语义归位：

| 新位置 | 页签 | 收纳内容 |
|---|---|---|
| **通知与备份** | `ops` | `notif_templates`（站内消息模板，本次改造的完整界面） |
| **角色权限矩阵** | `matrix` | `fill_roles_by_role`、`managed_roles_by_role`、`kb_edit_roles`、`chat_channels`、`external_evidence_types` |
| **表单模板** | `forms` | `drill_form_by_action`（动作→表单映射） |
| **业务字典（新增）** | `dict` | `ev_types`、`script_action_types`、`levels`、`projTypes`、`eventTypes`、`drill_level_by_scenario`（若确认保留） |

- **字典页是否保留**：建议**下线** `public/dictionary.html`，由 `admin.html` 承接；如需过渡，保留页面但仅做只读跳转（301/链接到 `admin.html#dict`）。
- **接口不变**：继续使用 `GET /api/system/dict/:key`、`PUT /api/system/dict/:key`（后端已存在、已做 notif 校验）。合并是**前端信息架构调整**，不改 API 契约，风险最低。
- **权限点**：仍由 `RequireBrandAdmin` 控制；若希望「权限矩阵」只给平台管理员、「业务字典」可给指挥长，需要拆分中间件（见 §5 待确认 Q3）。

### 2.2 迁移映射

| 键/功能 | 新位置 | 接口 | 权限点 | 迁移动作 |
|---|---|---|---|---|
| `notif_templates` | 系统设置 → 通知与备份 | `GET/PUT /api/system/dict/notif_templates` | platformadmin | 搬迁编辑器 DOM/JS；API 不变 |
| `chat_channels` | 系统设置 → 角色权限矩阵 | `GET/PUT /api/system/dict/chat_channels` | platformadmin | 搬迁编辑器 |
| `fill_roles_by_role` | 同上 | 同上模式 | platformadmin | 搬迁 + **待确认是否让 Go 接入** |
| `managed_roles_by_role` | 同上 | 同上 | platformadmin | 搬迁 + 待确认 |
| `kb_edit_roles` | 同上 | 同上 | platformadmin | 搬迁 + 待确认 |
| `external_evidence_types` | 同上 | 同上 | platformadmin | 搬迁或删除 |
| `drill_form_by_action` | 系统设置 → 表单模板 | 同上 | platformadmin | 搬迁（或删除） |
| `ev_types` / `script_action_types` | 系统设置 → 业务字典（新） | 同上 | platformadmin | 搬迁 |
| `levels` / `projTypes` / `eventTypes` | 业务字典（新，只读展示或可编辑） | 同上 | platformadmin | 新增入口 |
| `drill_level_by_scenario` | 业务字典（新） | 同上 | platformadmin | 待确认 |
| `drill_action_meta` | —— | —— | —— | **删除**（待确认） |
| `external_node_patterns` / `role_feed_mapping` / `difficulties` / `drill_role_by_target` / `target_role_aliases` | —— | —— | —— | **删除**（待确认） |

### 2.3 菜单与代码改造清单（方案批准后执行）

1. `public/app.js` 的 NAV 去掉 `dictionary`（**该文件当前由其他会话修改，本会话不动**）。
2. `public/admin.html:51` 的 `MODS` 去掉 `['dictionary','流程字典']`；新增「业务字典」页签 `data-t="dict"`。
3. 把 `dictionary.html` 的 `build/wire/collectValue` 抽成 `public/dict-panel.js`（或内联进 admin.html），按页签挂载。
4. `admin.html` 的「通知与备份」增加站内消息模板卡片（复用本次改造后的编辑器）。
5. 字典页下线：`dictionary.html` 改为 `<meta http-equiv="refresh" content="0;url=admin.html#dict">`，或直接删除；删除前确认无收藏夹/外部链接依赖。
6. 服务端不需要新接口；可选新增 `dict` key 白名单，拒绝未知 key（当前 Go `PutDrillDict` 无白名单，可写入任意 key）。

---

## 3. 删除清单（等待确认，未执行）

> 本清单中**没有**「删表 / 删列」。所有删除均为 `dict` 表行或前端代码。

### 3.1 数据（`dict` 表行）

| # | 删除对象 | 影响历史数据？ | 说明 |
|---|---|---|---|
| D1 | `drill_action_meta` | 否 | Go 无读取；历史任务的干扰项已固化在 `script_nodes.wrong` |
| D2 | `difficulties` | 否 | 无任何读取 |
| D3 | `drill_role_by_target` | 否 | 无读取 |
| D4 | `target_role_aliases` | 否 | 无读取 |
| D5 | `external_node_patterns` | 否 | 无读取 |
| D6 | `role_feed_mapping` | 否 | 无读取 |
| D7 | `drill_form_by_action` | 否 | Go 以 `script_nodes.form` 为准；历史节点已存 form |
| D8 | `drill_level_by_scenario` | 否 | 历史任务等级已写入 `tasks.level` |
| D9 | `fill_roles_by_role` / `managed_roles_by_role` / `kb_edit_roles` | 否（功能层面有影响） | 删除后这些权限范围只能改代码；若希望「可配置」，应先让 Go 接入再删（推荐） |
| D10 | `external_evidence_types` | 否 | Go 硬编码；删除后配置不再可改 |

### 3.2 前端代码

| # | 对象 | 状态 |
|---|---|---|
| C1 | `dictionary.html` 中「决策与干扰项」导航项/编辑器/提示/保存分支 | **已删除** |
| C2 | 全站 `drill_action_meta` 字样（public 目录） | **已清零**（grep 0 命中） |
| C3 | 字典页菜单/权限矩阵项 | 待批准（涉及 `app.js` 冻结文件） |

### 3.3 明确不删

- `script_nodes.wrong` 列：混淆流程真源。
- `notifs` 表及其历史行：消息是最终渲染文本，删除模板不影响历史消息展示。
- `dict` 表中的 `notif_templates`：本次要求保留并加强管理。

### 3.4 历史站内消息影响

- 删除某个 `notif_templates` 事件模板后：**未来该事件不再发送站内消息**（Go `pushNotif` 在模板不存在时返回 false，无内置兜底）。
- 已发送的历史 `notifs` 行保存的是渲染后的标题/正文，**不会因模板删除而消失或变空**。
- 删除 `chat_channels` 会导致演练导调中心频道不可见（`missing_handlers.go:629` 读取失败即无频道），不建议删除，只建议合并入口。

---

## 4. 风险与回滚

### 4.1 备份

1. **数据库快照**（执行删除前）：
   ```bash
   mysqldump --single-transaction --routines --triggers drill_system dict notifs > backup_dict_notifs_$(date +%Y%m%d%H%M).sql
   ```
   仅备份受影响的 `dict`、`notifs` 即可；如需全量，使用系统设置「通知与备份」的全量备份。
2. **代码快照**：删除前打 tag 或在本地 `git stash create`/分支保存；本会话不 commit、不 push。

### 4.2 回滚步骤

1. 代码回滚：恢复 `public/dictionary.html`、`admin.html`、`public/app.js`、`server/internal/handler/system.go` 到备份版本。
2. 数据回滚：
   ```bash
   mysql drill_system < backup_dict_notifs_YYYYMMDDHHMM.sql
   ```
   或按单键恢复：
   ```sql
   INSERT INTO dict (`key`, value) VALUES ('drill_action_meta', '<备份 JSON>')
     ON DUPLICATE KEY UPDATE value=VALUES(value);
   ```
3. 菜单回滚：恢复 NAV 与 `MODS` 后强刷浏览器缓存（静态资源带 `?v=` 版本号）。
4. 验证回滚：`GET /api/bootstrap` 能看到对应 key；`GET /api/content/dict/:key` 200；通知模板发送一条测试事件成功。

### 4.3 风险点

| 风险 | 等级 | 缓解 |
|---|---|---|
| 删除 Go 未读取的权限类 key 后，管理员误以为仍可配置 | 中 | 删除前在 UI 标注「当前版本由代码固定」或先让 Go 接入 |
| `app.js` 由其他会话修改，菜单合并冲突 | 中 | 合并前与 B 阶段会话对齐，最后统一改 NAV |
| 通知模板删除导致某事件静默不推送 | 中 | 删除弹窗已加醒目提示；后端保存校验必填 |
| Go 对未知字典 key 无白名单 | 低 | 可选增加 key 白名单，但会阻断历史自定义 key |
| 字典页下线后外部收藏链接 404 | 低 | 保留 `dictionary.html` 重定向到 `admin.html#dict` |

---

## 5. 需要用户确认的决定

1. **字典页是否彻底消失？**
   - 方案 A：删除页面，菜单与权限矩阵同步移除（推荐，最干净）。
   - 方案 B：保留 `dictionary.html` 作为 `admin.html#dict` 的 302 跳转（兼容旧链接）。
2. **合并后权限点是否变化？**
   - 方案 A：全部仍由平台管理员（`RequireBrandAdmin`）维护，不拆权限（推荐，零风险）。
   - 方案 B：拆成「权限矩阵（平台管理员）」与「业务字典（指挥长可读/可改）」两个权限点，需要新增中间件与角色校验。
3. **`drill_action_meta` 是否确认删除？**（Go 0 读取，替代实现为 `script_nodes.wrong`；建议删除）
4. **`drill_form_by_action`、`drill_level_by_scenario` 是否删除？**（Go 已硬编码/以节点为准；建议删除，但如未来要恢复「可配置」，需先改 Go 读取）
5. **`fill_roles_by_role`、`managed_roles_by_role`、`kb_edit_roles`、`external_evidence_types` 怎么处理？**
   - A. 直接删除（承认当前不可配置）；
   - B. 搬到权限矩阵，并让 Go 改为读取字典（工作量中等，行为会随配置变化）；
   - 建议 B，至少 `chat_channels` 已双源，应统一。
6. **`external_node_patterns`、`role_feed_mapping`、`difficulties`、`drill_role_by_target`、`target_role_aliases` 是否确认删除？**（建议全部删除）
7. **`levels / projTypes / eventTypes` 是否需要在「业务字典」里可视化编辑？** 还是保持种子常量、只读展示？
8. **删除执行时机**：是否等 B 阶段合并完成后再执行，避免同一时段改 `dict` 数据？

---

## 6. 本次已实施（非破坏性）记录

### 6.1 代码改动

| 文件 | 改动 |
|---|---|
| `public/dictionary.html` | ① 删除「决策与干扰项」导航项、编辑器、提示与保存分支；② 站内消息模板来源改下拉（占位 + 4 个真实来源 + 历史值兼容）；③ 事件元数据表（8 事件、23 变量中文含义/示例）；④ 标题/正文两排「插入变量」按钮，按光标位置插入；⑤ 必填与未知变量校验；⑥ 新增模板只允许 8 个真实事件键；⑦ 删除已知事件模板加入「停止推送」确认弹窗；⑧ 修复本页签内重复绑定（`render()` 已含 `wire()`，避免一次点击开两个弹窗） |
| `server/internal/handler/system.go` | `PUT /api/system/dict/notif_templates` 增加服务端校验：标题/正文/来源必填、来源必须在 4 个合法值内、等级 P0/P1/P2 |

未改动：`api.js`、`app.js`、`workbench.html`、`drill_score.go`、`migrations/`、`deploy/`、Node 历史实现。

### 6.2 实测结果

> ⚠️ 本表为**宿主机 Windows exe** 上取得的旧数据。按项目长期约束（宿主机装有 360 安全管控终端，会杀 Go 编译产物），**本轮已全部作废**；VM 内重做结果见 7.5。

| 项 | 结果（已作废，仅存档） |
|---|---|
| 模板读取 | `GET /api/system/dict/notif_templates` → 200，8 个事件 |
| 来源枚举 | 下拉 5 项：`请选择消息来源` + 演练调度 / 角色工作台 / 演练导调中心 / 复盘评估（每个带说明） |
| 保存 | `PUT` → 200 `{"ok":true}` |
| 非法来源 | `PUT` → **400**，`{"error":"模板 project_created 的消息来源「非法来源」非法，必须是：演练调度 / 角色工作台 / 演练导调中心 / 复盘评估"}` |
| 缺标题 | `PUT` → **400**，`{"error":"模板 project_created 缺少标题"}` |
| 新增/删除 | 删除 `auto_eval` 后 GET 无该键；重新新增后恢复 8 个事件，均 200 |
| 变量插入 | 标题插入 `{mode}`、`{project}`；正文插入 `{scenario}`、`{date}`；落库值见下 |
| 渲染替换 | 创建项目后消息原文：`【实战版】站内模板变量验证项目（P2026-003）已创建` / `项目「站内模板变量验证项目」关联剧本「未关联剧本」，计划 2026-11-05；目标：验证模板变量替换。` |
| 未知变量 | 保存 200，警告「未知变量 {foo} 不会被替换」（非阻断） |
| 必填校验 | 清空标题后阻止保存，提示「请补全必填项：演练开始/继续 的标题」 |
| 决策与干扰项残留 | `public/` 内 `drill_action_meta` / `决策与干扰项` / `dictMetaAdd` **0 命中** |
| 三视口 | 1920 / 393 / 412：console error **0**，`scrollWidth == clientWidth`（无横向溢出） |
| 构建 | `go build ./...`、`go vet ./...` 均 exit 0 |
| 截图 | 宿主机产物已删除；VM 内截图为 `docs/verify/admin-dict-*-vm.png` |

落库实际值（隔离库 `drill_dictui.dict`）：

```
project_created.title  = 演练{mode}项目已创建{project}
project_created.desc   = {scenario}关联剧本，计划日期。{date}
project_created.source = 演练调度
dict 模板数 = 8（auto_eval/drill_end/drill_end_score/drill_start/node_done/node_skip/node_submit/project_created）
```

渲染后的 `notifs` 行（隔离库）：

```
N1793851735359_419 | 【实战版】站内模板变量验证项目（P2026-003）已创建 | 项目「站内模板变量验证项目」关联剧本「未关联剧本」，计划 2026-11-05；目标：验证模板变量替换。 | P1 | 演练调度 | internal
```

> 说明：模板整行内容在界面保存阶段被替换为验证文案，属于隔离库测试数据，不影响现役库。

### 6.3 已知遗留

- `public/dictionary.html` 中不同分组编辑器仍共用 `id="dictSaveBtn"`（同页多组会重复 ID），本次通过 `#edit-notif` 作用域规避，未做全页重构；建议在并入 `admin.html` 时一并消除。
- `bootstrap.go:993-1002` 的聊天频道分组权限仍为硬编码，与 `dict.chat_channels` 双源；属合并阶段的必修项。
- 本次未改 `public/app.js`（其他会话占用），因此「流程字典」菜单仍指向旧页；页面内的「决策与干扰项」已消失。


## 七、本轮落地（用户授权后执行，v0.5.0 界面合并）

### 7.1 页面结构（系统设置 admin.html）

| 页签 | data-t | 承载的字典能力 |
|---|---|---|
| 账号管理 | users | 不变 |
| 角色权限矩阵 | matrix | 权限矩阵 + 会商频道配置（chat_channels） |
| 表单模板 | forms | 不变 |
| 审计日志 | audit | 不变 |
| 通知与备份 | ops | 原有备份 + 站内消息模板（notif_templates） |
| 业务字典 | dict | levels / projTypes / eventTypes / ev_types / script_action_types |
| 基本信息设置 | brand | 不变 |

- 字典页 public/dictionary.html 下线，改为跳转页；server/internal/router/router.go 的 NoRoute 顶部对 /dictionary.html 返回 **302** → /admin.html#dict。
- public/app.js 导航移除「流程字典」；admin.html 的 MODS 移除 ['dictionary','流程字典']。
- 各编辑器使用独立 ID：ntSaveBtn / chSaveBtn / bdSaveBtn，dictSaveBtn 在 public/ 内 0 命中。

### 7.2 数据删除（迁移 migrations/0006_remove_dead_dict_keys.up.sql）

已删除 7 个键：drill_action_meta、drill_form_by_action、drill_level_by_scenario、external_node_patterns、difficulties、drill_role_by_target、target_role_aliases。

**复核更正**：role_feed_mapping 并非 0 读取 —— public/workbench.html 按角色读取它作为默认工作台模块，故**未删除**，数据保留。

### 7.3 保留数据但从界面移除（待裁决：是否接入 Go）

fill_roles_by_role、managed_roles_by_role、kb_edit_roles、external_evidence_types：Go 端为硬编码，改字典不生效，留界面会误导，因此不迁移到新页签、数据保留。

### 7.4 待裁决

1. 上述 4 个键是否改由 Go 读取（需改 bootstrap.go / content_write.go / evidence 权限逻辑）。
2. chat_channels 双源：bootstrap.go 的频道分组成员仍硬编码，界面已加说明「分组成员当前由服务端策略决定」，是否统一为字典驱动。
3. role_feed_mapping 是否需要在「业务字典」补编辑入口。

### 7.5 验证（全部在 VMware 虚拟机内完成）

**验证环境**

| 项 | 值 |
|---|---|
| VM | VMware `192.168.97.131`（UOS Server 20，账号 `drill`） |
| 数据库 | 容器 `drill-dictui-mysql`（`127.0.0.1:13310`，隔离库 `drill_dictui`） |
| 被测服务 | 宿主交叉编译的 Linux ELF（`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath`），在 VM 内运行于 `127.0.0.1:18096`，`-root=/home/drill/dictui/root` |
| 二进制有效性 | 宿主编译产物与 VM 运行产物 sha256 一致：`02eb1299d75179218f70b75825291326a364c827a8f1ebaef772309cfafb738a` |
| 浏览器 | VM 内 Chromium 131.0.6778.33 + Playwright 1.49.1（Node 20.18.1） |
| 现网隔离 | 未触碰 `drill-app`(8090) / `drill-mysql`(3306)，未部署、未重建镜像、未写现役库 |

**数字结果**（原始记录 `docs/verify/admin-dict-report-vm.json`）

| 项 | 结果 |
|---|---|
| 三视口 1920×1080 | console error 0 / page error 0 / HTTP 4xx-5xx 0 / 横向溢出 0（scrollWidth 1920 = clientWidth 1920） |
| 三视口 393×873 | console error 0 / page error 0 / HTTP 4xx-5xx 0 / 横向溢出 0（393 = 393） |
| 三视口 412×915 | console error 0 / page error 0 / HTTP 4xx-5xx 0 / 横向溢出 0（412 = 412） |
| 页签遍历（7 个） | users / matrix / forms / audit / ops / dict / brand 全部 `on=true` 且 pane 有内容；新增 console error 0、HTTP 4xx-5xx 0 |
| 页签渲染计数 | admTabs=7、ntCards=8、chCards=2、bdItems=4、`#dictSaveBtn` 命中 0 |
| notif_templates | 读取 8 个事件、来源下拉 5 项（占位 + 4 合法来源）；保存 `PUT` 200；回读 200 |
| chat_channels | 读取 2 个频道、角色勾选 24 项；保存 `PUT` 200；回读 200（2 频道） |
| 业务字典 5 键 | levels 4 / projTypes 3 / eventTypes 8 / ev_types 8 / script_action_types 10 项，逐个保存 `PUT` 200 + 回读 200 |
| 变量回写 | 标题写入 `{projectId} · {nodeName}` → 保存 200，回读两个占位符原样保留（`projectIdIntact=true`、`nodeNameIntact=true`），随后恢复原值 |
| 服务端校验（system.go） | 非法来源 400「模板 auto_eval 的消息来源「不存在的来源」非法…」；缺标题 400；缺来源 400；等级 P9 400；合法含 `{projectId}` 保存 200 |
| 旧地址 | `GET /dictionary.html` → **302**，`Location: /admin.html#dict`；浏览器访问最终落 `admin.html#dict` |
| 导航残留 | dashboard.html 内 `a[href*="dictionary.html"]` 0、页面文本「流程字典」0 |
| 隔离库 dict 键数 | 12（19 − 7），`drill_action_meta` 等 7 键均不存在，`role_feed_mapping` 保留 |
| `go build`（Linux ELF 交叉编译） | exit 0（宿主仅编译不运行） |
| `go vet ./...` | exit 0 |
| `node --check`（验证脚本 / admin.html 内联脚本） | exit 0 |
| `public/` 残留 grep | `流程字典` 3（全部在跳转页说明文案）、`dictionary.html` 0、`决策与干扰项` 0、`drill_action_meta` 0、`id="dictSaveBtn"` 0 |

**截图**：`docs/verify/admin-dict-1920x1080-vm.png`、`admin-dict-393x873-vm.png`、`admin-dict-412x915-vm.png`、`admin-ops-1920x1080-vm.png`、`admin-matrix-1920x1080-vm.png`（均由 VM 内 Chromium 生成）。

**作废重做清单**：上一轮在宿主机 Windows 上运行 `drill-server-test.exe` 得到的接口断言、三视口数字、隔离库写入结果全部作废，已在上表用 VM 内服务重做；宿主侧仅保留 `go build`/`go vet`/`node --check`/静态 grep 类结论。

**服务端行为变化**：本轮为零。唯一后端改动是 `NoRoute` 新增 `/dictionary.html` → 302 跳转（纯页面路由，不改变任何业务接口）；`PUT /api/system/dict/notif_templates` 的入参校验属上一轮改动，随下次发版生效。字典读取路径、通知发送、频道分组、权限点均未改动。

