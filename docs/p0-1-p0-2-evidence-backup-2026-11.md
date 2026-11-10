# P0-1 / P0-2 修复说明：取证材料链路与备份恢复

- 依据：`docs/system-review-engineering-2026-11.md` 的 P0-1、P0-2
- 本轮范围：只改代码与文档。未重建镜像、未重启容器、未写现网库、未推仓库、未新增数据库迁移。
- 涉及文件：
  - `server/internal/handler/missing_handlers.go`
  - `server/internal/handler/system_extra.go`
  - `server/internal/handler/bootstrap.go`（`queryEvidence` 增补 `nodeId` / `reqKey`）
  - `server/internal/handler/screen_public.go`（仅修复并行改动造成的编译阻断，见文末）
  - `server/internal/middleware/middleware.go`（仅放开 `restore-upload` 的请求体上限）
  - `public/workbench.html`
  - `public/admin.html`
  - `deploy/docker-dist/Dockerfile`、`deploy/docker-dist/Dockerfile.runtime`
  - 本文件

---

## 一、取证材料（evidence）数据流

### 1. 上传落盘与入库

入口：

- `POST /api/evidence/upload`（multipart，字段 `files`，兼容 `file`）
- `POST /api/evidence`（JSON，抓屏 `dataUrl`）
- 工作台任务附件：`POST /api/tasks/:id/attachments` → 同一物理文件同时镜像一条 `evidence` 记录

统一数据流：

```
浏览器选择文件 / canvas 抓屏
        │
        ├─ multipart ──────────────┐
        └─ data:image/...;base64 ──┤ 解码 + 文件头校验（PNG/JPEG/GIF/WEBP/BMP）
                                   ▼
                 uploads/evidence/YYYYMM/<时间戳>-<随机串>-<安全文件名>
                                   │
                                   ▼
        evidence 表：id / type / name / size / time / role_id / uploader / user_id
                     auto / task_id / project_id / node_id / req_key
                     url=/uploads/<相对路径> / file_path=<相对路径> / note / created_at
                                   │
                                   ▼
        GET /api/evidence 与 bootstrap 的 MOCK.EVIDENCE 返回同一批记录
```

关键点：

- 上传的文件真正落盘到 `UPLOAD_ROOT`（默认 `uploads`）+ `evidence/YYYYMM`，不再只返回一个假 URL。
- 抓屏图片按 `dataUrl` 的 base64 解码后落盘；扩展名以真实文件头为准，不信任客户端文件名。
- 任务绑定：`taskId` 存在时校验任务、项目状态与角色权限，并自动回填 `project_id` / `node_id`。
- 节点绑定：`nodeId` 必须存在且与项目剧本一致；只给节点时按节点所属剧本回填一个项目。
- 项目绑定：项目必须存在且未归档。
- 同一文件同时写入 `task_attachments`（有 `taskId` 时），因此工作台“证据附件”和“取证留痕”看到的是同一份文件。
- 物理文件写盘成功后若数据库事务失败，会删除已写文件，避免孤儿文件。

### 2. 权限口径

上传、删除与任务附件保持一致：

- 外部单位（`Role.External`）禁止上传、预填、导入模拟材料和删除。
- 任务绑定材料：平台管理员/指挥部/信息科负责人可代传；其他角色只能操作本角色任务。
- 删除：上传者本人，或 `platformadmin` / `commander` / `itlead`。
- 删除时先移除 `task_attachments` 镜像记录，再删除 `evidence`；只有确认没有其他记录引用同一 `file_path` 时才删除磁盘文件。
- 下载 `GET /api/evidence/:id/download` 返回真实文件字节流（`Content-Disposition: attachment`），不再返回 JSON。

### 3. 取证要求与预填

`GET /api/evidence/requirements?projectId=...`

- 按项目剧本节点 `script_nodes.evidence_req` 展开逐项要求。
- `evidence_req` 同时兼容对象数组 `[{type,name,note}]` 与旧版字符串数组。
- 每项返回：`key=nodeId#index`、`nodeId`、`nodeTitle`、`target`、`index`、`type`、`name`、`note`、`mine`、`done`、`evidenceId`、`doneAt`、`hasFile`。
- `mine` 由 `canHandleRole(当前角色, 节点 target)` 计算；`done` 由 `evidence.project_id + req_key` 判定。
- 返回总览：`total / done / pending / mineTotal / mineDone / minePending`。
- 不再恒空、不再返回 `[]`。

`POST /api/evidence/prefill`

- 前端把本角色负责且未归档的要求以 `{nodeId,index,name,note}` 提交。
- 服务端逐项校验节点、剧本归属、角色权限、`req_key` 是否已存在。
- 通过后写入 `evidence`（`auto=1`、`req_key=nodeId#index`、`project_id`、`node_id`），返回真实 `count` 与 `skipped` 明细。
- 重复提交不会重复入库。

`POST /api/evidence/mock`

- 按节点 + 序号导入一条模拟材料记录（`auto=1`），用于演练预填。
- 已存在同 `req_key` 时返回 `deduped=true`，不重复写入。
- 仅登记记录，不伪造物理文件；`hasFile=false` 会在界面明确展示。

### 4. 前端

- `workbench.html`：任务附件上传成功后调用 `API.getEvidence()` 回读，刷新 `MOCK.EVIDENCE` 后重新渲染“证据附件”，确保附件区与“取证留痕”两处展示一致。
- `evidence.html` 原有入口已能消费新的 requirements / prefill / mock / upload / download 返回结构，本轮无需改动。
- 未改动 `api.js`、`screen.html`、`script_nodes_write.go`。

---

## 二、备份与恢复（P0-2）

### 1. 备份格式取舍：只用 `.sql`

结论：**统一使用 `.sql`（`mysqldump` 文本）**，不再保留 `.db` 分支。

- 系统生成的备份：`data/backups/drill-<YYYYMMDD-HHMMSS>.sql`
- 恢复前安全网：`data/backups/drill-pre-restore-<YYYYMMDD-HHMMSS>-<rand>.sql`
- 上传恢复落盘：`data/backups/drill-upload-<YYYYMMDD-HHMMSS>-<rand>.sql`
- 恢复过程日志：`data/backups/restore-<YYYYMMDD-HHMMSS>-<rand>.log`

影响：

- 旧 Node 版遗留的 SQLite `.db` 文件不再出现在备份列表，也不能通过“上传恢复”或“列表恢复”导入。
- 原因是 `.db` 与当前 MySQL 架构不兼容，继续接受只会产生“看似恢复成功、实际未恢复”的假象。
- 历史 `.db` 文件不会自动删除；需要归档时请人工转移，不要放进 `data/backups` 期望系统识别。

### 2. 恢复流程

`POST /api/admin/restore`（列表文件）与 `POST /api/admin/restore-upload`（上传 `.sql`）走同一个 `performRestore` 实现：

```
1. 文件名/扩展名/内容校验（必须是 mysqldump 形态的 .sql）
2. 恢复前自动备份当前库（mysqldump --single-transaction）
   └─ 失败 → 立即中止，当前库未被改动，返回 503
3. 记录恢复前行数快照（schema_migrations/roles/users/projects/tasks/evidence/script_nodes）
4. mysql/mariadb 客户端导入目标 .sql
   └─ 失败 → 自动导入第 2 步的安全网备份回滚
5. 恢复后再次读取关键表，校验数据库可读并生成行数快照
   └─ 校验失败 → 自动回滚
6. 清空 sessions（强制重新登录）
7. 写 restore-*.log 与 audit_logs，返回可核验结果
```

### 3. 失败语义（不再返回 ok）

| 场景 | HTTP | 行为 |
| --- | --- | --- |
| 非 `.sql` / 文件为空 / 不是 mysqldump 文本 | 400 | 直接拒绝，不触碰数据库 |
| 容器缺少 `mysqldump` | 503 | 恢复前备份失败，中止恢复 |
| 容器缺少 `mysql` / `mariadb` 客户端 | 500 | 导入失败，尝试回滚 |
| 导入 SQL 出错 | 500 | 自动回滚到恢复前备份，返回失败原因 |
| 恢复后关键表校验失败 | 500 | 自动回滚，返回失败原因 |
| 回滚也失败 | 500 | 返回安全网文件名，要求立即人工恢复 |
| 成功 | 200 | `ok=true`，附 `preBackup` / `before` / `after` / `verified` / `log` / `duration` |

成功响应示例：

```json
{
  "ok": true,
  "file": "drill-upload-20261107-101500-ab12cd.sql",
  "preBackup": "drill-pre-restore-20261107-101501-3f9a1c.sql",
  "before": { "users": 12, "tasks": 40 },
  "after": { "users": 12, "tasks": 41 },
  "verified": true,
  "log": "restore-20261107-101501-9d2e4f.log",
  "duration": "1.234s"
}
```

### 4. 可核验日志与审计

- 文件日志：`data/backups/restore-*.log`，逐行记录开始、来源、操作人/IP、前备份、导入、校验、回滚、成功/失败。
- 数据库审计：`audit_logs` 写入一条 `数据恢复成功/失败：...`，包含安全网文件名、耗时、关键表行数变化（失败时包含原因）。
- 应用日志：`slog.Info/Warn` 输出来源、操作人、安全网、日志名、耗时。

### 5. 容器依赖与请求体上限

- `deploy/docker-dist/Dockerfile` 与 `Dockerfile.runtime` 的 `apk add` 增加 `mariadb-client`，提供 `mysqldump` / `mysql`（兼容 `mariadb-dump` / `mariadb`）。
- `middleware.BodyLimit` 仅对 `/api/admin/restore-upload` 放宽到 2GB，其余接口仍使用 `UPLOAD_MAX_MB`。否则整库 `.sql` 会被 20MB 全局上限直接 413。
- 本轮未构建镜像；上线时必须重新构建带 `mariadb-client` 的运行时镜像，否则恢复会在“恢复前自动备份”阶段显式失败。

---

## 三、验证步骤

> 本轮未连接现网库。以下步骤使用隔离库 + 临时实例，上线后按同一顺序复验。

### A. 静态检查（已执行）

```powershell
cd server
go vet ./...          # 无输出
go build ./...        # 通过
```

前端内联脚本：

```powershell
# 从 workbench.html / admin.html / evidence.html 抽取内联 <script> 后逐个 node --check
# 本轮结果为 OK
```

### B. 取证链路（隔离库）

1. 用独立库（如 `drill_test`）启动临时实例，`UPLOAD_ROOT` 指向临时目录。
2. 以有角色账号登录，调用 `POST /api/evidence/upload`（带 `projectId`、`nodeId`、`reqKey`）：
   - 数据库出现 `evidence` 行，`file_path` / `url` / `node_id` / `req_key` 非空；
   - 临时 `uploads/evidence/YYYYMM/` 出现物理文件；
   - `GET /api/evidence` 能看到该记录；
   - `GET /api/evidence/:id/download` 返回文件字节（非 JSON）。
3. 用 `POST /api/evidence` 提交 `dataUrl` 截图：
   - 物理文件出现，文件头与扩展名一致；
   - `evidence.hasFile=true`。
4. 工作台给任务上传附件：
   - `task_attachments` 与 `evidence` 各出现一条记录，`file_path` 相同；
   - 工作台“证据附件”与“取证留痕”均显示该材料。
5. `GET /api/evidence/requirements?projectId=...`：
   - 返回剧本节点的逐项要求，`total > 0`；
   - 已上传项 `done=true`，其余 `done=false`。
6. `POST /api/evidence/prefill`：
   - 返回 `count > 0`；
   - 再次调用同一批，`count=0` 且 `skipped` 包含“已归档”。
7. 删除：
   - 上传者删除成功，物理文件消失；
   - 非上传者且非管理角色删除返回 403。

### C. 备份恢复（隔离库 + 临时实例）

1. 在临时实例上调用备份接口，确认生成非空 `.sql`，首行包含 `MySQL dump` / `MariaDB dump`。
2. 记录当前 `users` / `tasks` / `evidence` 行数。
3. 修改一条可识别数据（如新增测试任务），再上传刚才的 `.sql` 恢复：
   - 返回 `ok=true`、`verified=true`；
   - `before` / `after` 行数与预期一致；
   - `data/backups` 出现 `drill-upload-*.sql` 与 `drill-pre-restore-*.sql`；
   - 出现 `restore-*.log`，`audit_logs` 有成功记录；
   - `sessions` 已清空。
4. 人为制造失败：
   - 上传一个内容非 SQL 的 `.sql` → 400，数据库不变；
   - 上传一个中途报错的 SQL（如引用不存在表）→ 500，自动回滚，返回失败原因与安全网文件名；
   - 在缺少 `mysqldump` 的镜像中恢复 → 503，明确提示缺少客户端，数据库不变。
5. 旧 `.db` 文件：
   - 备份列表中不显示；
   - 上传 `.db` → 400，提示仅支持 `.sql`。

### D. 本轮未执行

- 未构建镜像、未重启容器、未执行真实恢复、未写现网库。
- `go vet` / `go build` / 前端内联脚本语法检查已通过。

---

## 四、设计取舍与遗留说明

1. **备份格式二选一**：选择 `.sql`，历史 `.db` 不再兼容。若现场确实存在需要保留的 `.db`，需要单独做一次性数据迁移，不能直接用于本系统恢复。
2. **恢复期间未做全站维护模式**：当前通过全局互斥锁防止并发恢复，但恢复过程中其他读请求仍可能看到中间状态。该问题属于评审 P1-8 范围，本轮未扩大改动。
3. **`screen_public.go` 的编译阻断**：本轮执行 `go build ./...` 时发现并行改动中的 `screen_public.go` 存在变量遮蔽（`title`/`descr` 以 `sql.NullString` 声明后被 `:=` 重声明为 `string`），会阻断 `go vet ./...`。为保证本轮验收，做了最小修复：改用 `titleText` / `descrText` 局部变量，不改变原有逻辑。该文件不在本轮取证/备份功能范围内。
