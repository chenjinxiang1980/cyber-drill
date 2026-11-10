# 演练收尾与评分封存（2026-11）

依据：《演练业务与剧本专家评审》§6.3/§6.4/§6.5、《系统功能与工程质量评审》P0-03 / P1-2 相关条目。
本轮范围：只改代码 + 迁移脚本。未写现网库、未重建镜像、未重启容器、未推仓库。

## 1. 结束状态机

业务目标状态机：

```
演练中 ──结束核对──┬─> 已结束（完整）──归档──> 已归档
                    └─> 已结束（有缺项）──归档──> 已归档
```

落库映射（本轮）：

| 业务状态 | projects.status | project_end_seal.end_mode |
| --- | --- | --- |
| 演练中 | `演练中` | — |
| 已结束（完整） | `已结束` | `完整` |
| 已结束（有缺项） | `已结束` | `有缺项` |
| 已归档 | `已归档` | 保留原值 |

说明：`projects.status` 的取值被 `missing_handlers.go`（状态变更白名单、归档）、列表筛选、监控页
等多处按 `'已结束'` 精确匹配，本轮按约束不修改 `missing_handlers.go`，因此不新增
`已结束（完整）/已结束（有缺项）` 状态字符串，改用 `project_end_seal.end_mode` 表达两种结束方式。
对外展示（复盘页、报告）以 `end_mode` 为准，行为等价于目标状态机；后续统一上线时可再评估
是否把 `end_mode` 合并进 status 字段。

结束路径：

1. `POST /api/drill/finish`（唯一正式结束入口）：
   - `{projectId, dryRun:true}` → 只返回一次性核对清单，不写库；
   - `{projectId}` → 核心证据齐全且已有取证时正常结束（`完整`）；
   - `{projectId, exception:{reason,missingKeys[],owner,deadline,approver}}` → 有缺项结束（`有缺项`）。
2. 已结束项目再次调用：返回 400；已有封存返回 `sealed`，无封存返回 `needsSupplementalSeal:true`
   （不自动补封存）。

结束事务内顺序（`executeDrillFinish`）：

```
核对清单 → 缺项说明校验 → scoreInternal(forceEnded=true) 只算一次
  → score_snapshots(vN, 已封存)
  → project_end_exception（如有）
  → projects.status='已结束'（WHERE status='演练中' 乐观校验）
  → project_end_seal
  → timeline「演练结束」
  → audit_logs
  → commit → 站内通知
```

失败任一步整体回滚，不会出现“状态已结束但 timeline/快照缺失”。

### 与项目状态接口的关系

`PATCH /api/projects/:id/status` 改为复用本流程：目标状态为 `已结束` 时不再直接改库，而是调用
`executeDrillFinish`，因此同样产生评分快照、`project_end_seal`、timeline「演练结束」与审计。
请求体在 `status` 之外接受：

| 字段 | 说明 |
| --- | --- |
| `exception` | 有缺项结束说明，结构同 `/api/drill/finish`（`reason/missingKeys/owner/deadline/approver`） |
| `force` | 运维强制结束，仅平台管理员/指挥部/信息科负责人可用；必须同时提供 `reason` |
| `reason` | 强制结束原因（≥5 字），写入快照、timeline 与审计 |

- 证据齐全：直接结束并封存（`end_mode=完整`）；
- 核心证据缺项且未提供 `exception`：返回 400 `needsException`，响应附 `finishUrl` 与 `hint`，
  补全缺项说明后重试，或改走 `/api/drill/finish`，正当的「有缺项结束」路径不受影响；
- `force=true`：跳过核对门槛，快照标记 `snapshot_status=异常封存`、`end_mode=异常结束`，
  等级追加「（异常结束）」，并强制写入原因与审计；核心证据缺项仍登记到 `project_end_exception`。
- 允许从 `演练中` 或 `暂停` 结束；`未开始 → 已结束` 仍被状态机拒绝。

## 2. 一次性核对清单

接口返回 `checklist`（`endChecklist`）：

| 字段 | 含义 |
| --- | --- |
| `requiredTotal / requiredDone / requiredMissing` | 必做节点（`required=1` 且非结束节点）完成情况与缺项 |
| `evidenceTotal / evidenceDone / evidenceRate` | 全部取证要求（core+support）归档率 |
| `coreTotal / coreDone / coreRate / corePending` | 核心证据完成情况 |
| `coreMissing[]` | 核心证据缺项：`key/nodeId/nodeTitle/target/name/suggestedDeadline` |
| `supportTotal / supportDone / supportPending` | 非核心证据，缺失只扣分、不阻断 |
| `zeroEvidence` | 有取证要求但一件未归档（0/N） |
| `canEndComplete / needsException` | 是否可直接结束 / 是否必须填写缺项说明 |
| `suggestedDeadline` | 建议补录时限（默认 now+72h，`drill_supplement_hours` 可配） |
| `hint` | 结论文案 |

前端入口：工作台「完成演练」先调用 `dryRun` 拿到清单并弹窗；核心证据未齐时展示缺项明细与
缺项说明表单，非核心缺失只做提示。

### 核心证据清单

- 平台内置默认：按剧本 `correct` 流程节点的 `evidence_req`，依证据名称优先级
  （告警 → 定级/研判 → 启动 → 隔离/封堵 → 备份/恢复 → 双报/上报/报告 → 回执 →
  取证/保全/镜像 → 结束/复盘）稳定排序，取前 10–12 项（`drill_core_evidence_max`，默认 12）。
- 项目级调整：项目创建时把默认清单复制到 `project_core_evidence`（`source='default'`）；
  后续可在该表内调整（`source='project'`、`adjusted_by/adjusted_at`）。老项目无项目级记录时
  自动回落到平台默认，无需刷数据。
- 本轮未新增“调整核心清单”的 HTTP 路由（router.go 由另一条线修改），调整能力先落在数据层。

## 3. 有缺项结束的数据结构

请求体：

```json
{
  "projectId": "P2026-001",
  "exception": {
    "reason": "演练时间窗口不足，核心取证未完成",
    "missingKeys": ["N1#0", "N5#1"],
    "owner": "张三（信息科）",
    "deadline": "2026-11-10 18:00",
    "approver": "李四（院应急指挥部）"
  }
}
```

校验：

- `reason` 5–500 字；`owner`、`approver` 必填（≤64 字）；
- `deadline` 支持 `YYYY-MM-DD HH:mm[:ss]`，必须晚于当前时间，落库统一为分钟；
- `missingKeys` 必须覆盖全部核心证据缺项；未传时服务端按清单自动补齐；
- `zeroEvidence`（0/N）同样必须走缺项说明，并按“有缺项”封存。

落库：

```sql
project_end_exception(
  project_id, snapshot_id, reason, missing_keys, missing_count,
  owner, deadline, approver, created_by, created_at
)
```

同时写入 `timeline`（action=`演练结束`，detail 含结束方式、核心证据完成率、缺项数、批准人、
责任人、补录时限）与 `audit_logs`。补录只允许后续追加补录记录，不回写已封存快照（本轮报告只读快照）。

## 4. 评分快照表与规则版本

### score_snapshots

| 字段 | 说明 |
| --- | --- |
| `project_id + version` | 唯一键；封存为 v1，重算追加 v2、v3… |
| `snapshot_status` | `已封存` / `重算` |
| `end_mode` | `完整` / `有缺项` |
| `score / grade / max_score / hit / total` | 结束时刻评分与等级（已做核心证据封顶） |
| `completeness / time_rate` | 流程完整率、时限达标率 |
| `evidence_complete_rate / core_complete_rate / core_total / core_done` | 证据与核心证据完整率 |
| `rule_version` | 规则版本，见下 |
| `scenario_version` | 剧本版本（`scenarios.version`） |
| `payload` | 快照全文 JSON：`ScoreResult`（逐节点结果 `nodes[]` + 扣分明细 `deductions[]`）+ 核对清单 + 缺项说明 |
| `recalc_from / recalc_reason` | 重算来源版本与原因 |
| `sealed_at / sealed_by` | 封存时间与封存人 |

`rule_version = SHA-256(剧本节点全文 + 评分比例常量 + core key 清单 + salt)[:16]`，
salt 来自 `settings.drill_rule_version_salt`。`ScoreResult.Steps` 不写入快照（控制体积），
报告需要的节点明细在 `ScoreResult.Nodes` 中。

### 等级封顶

`sealedGrade(base, coreRate, coreTotal, complete)`：

- `coreRate < 100%`：不得“优秀”；
- `coreRate < 80%`：不得“良好”，最高“合格”；
- 有缺项结束：等级追加“（有缺项）”，如 `合格（有缺项）`；
- `coreTotal = 0`（剧本未配置取证要求）不做封顶。

P2026-001（0/35）会得到“合格（有缺项）”，分数保持不变（见 §6 口径说明）。

### project_end_seal

`project_id` 主键，记录 `end_mode / snapshot_id / snapshot_version / sealed_at / sealed_by /
exception_id`，是“报告默认展示哪个版本”的锚点。重算不会修改该表。

## 5. 读取路径

`GET /api/content/evaluation?projectId=X[&snapshotVersion=N]` 在原 `items/scores` 之外新增：

| 字段 | 说明 |
| --- | --- |
| `seal` | 封存记录（无封存为 null） |
| `snapshot` | 快照行 + `snapshot`（payload 解析后的全文） |
| `snapshots` | 全部版本摘要（版本、状态、分数、等级、重算原因） |
| `endException` | 缺项结束说明 |

默认版本规则：未指定 `snapshotVersion` 时，有封存取 `seal.snapshot_version`（封存版本），
无封存取最新一条。`snapshotVersion=N` 用于查看历史/重算版本。

消费方：

- `evaluation.html`（复盘）：并行拉取 workbench 与本接口；有快照时考评明细、汇总、建议、
  等级全部读快照，并在汇总卡展示“评分封存”区块（版本、结束方式、完整率、规则/剧本版本、
  缺项说明、历史版本）。
- `report.js`（报告）：并行拉取 `/admin/report/:id` 与本接口；第 6 章优先使用快照的
  `ScoreResult`、封存元数据与核心证据完整率；封面标注封存版本与结束方式；已结束但无封存时
  给出提示。历史版本仅在报告内以文字列出（默认仍展示封存版本）。
- `workbench.html`：`evidence` 新增 `core/coreDone/corePending/coreRate/support...`，
  结束按钮不再因取证缺失整体禁用。

## 6. 与现有评分口径的关系（重要）

本轮**没有改变 `Score()` 的扣分公式**（超时/接收人/关键词/文字质量/证据文本 20%，单节点上限
80%），也没有把“证据 20%”改为按 `evidence` 表 core/support 加权读取。原因：

- 改扣分基数会让所有在评项目与历史项目分数跳变；按“已结束”口径计算，P2026-001 的复盘
  `Score()` 为 82.6 分（2 个未完成节点计入遗漏），补封存快照与该值一致，等级按 core 0%
  降为 `合格（有缺项）`。若证据维度改读底表，其 0/34 会使分数进一步大幅下降，与既定处理
  口径冲突。
- 专家评审 §6.2 的证据维度重构属于独立评分口径改造，建议统一上线时单独立项、带历史重算方案。

因此本轮落实为：

- **结束门槛**：core 缺失阻断，support 缺失不阻断；
- **等级联动**：按 core 完整率封顶（<100% 不得优秀、<80% 不得良好），有缺项追加标识；
- **证据完整率**：在核对清单、快照、复盘页、报告中如实展示，作为扣分与整改依据；
- support 缺失的“只扣分”暂未新增独立扣分项，体现在完整率与等级封顶，待评分口径改造时
  正式并入。

## 7. P2026-001 补封存：已执行记录

执行时间 `2026-11-07 20:46:28`，由 `cmd/supplemental-seal` 走应用自身的补封存路径
（复用结束核对、评分、快照、缺项说明、timeline 与审计逻辑，不逐表手工写）。

执行前只读核对：`status=已结束`、`ended_at=2026-11-07 13:51:21`、`paused_at=NULL`、
`paused_sec=0`、`evidence 0/34`、`timeline` 仅创建记录、`score_snapshots/project_end_seal/
project_end_exception` 均为 0。

| 项目 | 结果 |
| --- | --- |
| 结束方式 | 有缺项（core 0/12，证据 0/34；必做 17/19） |
| 快照 | `SS20261107204628200`，v1，`已封存` |
| 分数 / 等级 | 82.6 / `合格（有缺项）`；复盘 `Score()` 82.6，分数一致 |
| 评分明细 | hit 17/19，max 112，completeness 89.5%，timeRate 68.4%，payload 19,481 字节含逐节点 `deductions` |
| `rule_version` | `4e72485dd3c39e7c` |
| `scenario_version` | `v2.0` |
| `sealed_at` / `sealed_by` | `2026-11-07 20:46:28` / `平台管理员` |
| 缺项说明 | 原因：演练由状态接口收口，结束核对未执行，取证 0/34；责任人：陈*祥（信息科负责人）；批准人：唐*旭（院应急指挥部）；补录时限：2026-11-10 20:46 |
| timeline | id 359，`演练结束（补封存）` |
| 审计 | `2026-11-07 20:46:28`，user=`平台管理员`，user_id=56 |

保护性校验：

- `projects` 行指纹与 `CHECKSUM TABLE projects/evidence/tasks/drill_steps` 前后完全一致，
  `status/started_at/ended_at/paused_sec` 未被改写；
- 写前整库备份：`/home/drill/backups/drill_system_before_P2026-001_20261107_204509.sql.gz`
  （102,836 字节，SHA-256 `56362978db0fee720bf2a0596686eae5fbd87703bb1cc7d1076e1c3ff2474181`）；
- 写后五处齐全：seal 1、snapshot 1、exception 1、timeline 新增 1、audit 新增 1。

回滚方式（如事后否决）：仅删除本次新增的 `score_snapshots` / `project_end_seal` /
`project_end_exception` 行及对应 timeline/audit，不动 `projects`；必要时用上述整库备份恢复。

影响：报告/复盘切换为读快照，默认展示 v1；原状态收口事实保留在审计中；补录只追加记录，
不回写快照；不删除任何既有记录。

## 8. 迁移

| 版本 | 文件 | 内容 |
| --- | --- | --- |
| 0020 | `0020_end_seal_tables.up.sql` | `project_core_evidence`、`project_end_seal`、`project_end_exception`、`score_snapshots` |
| 0021 | `0021_seal_policy.up.sql` | 策略默认值（`INSERT IGNORE`）：core 10/12、补录 72h、规则盐 |
| 0022 | `0022_evidence_req_idx.up.sql` | `evidence(project_id, req_key)` 索引，`information_schema` 判断保证幂等 |

全部为 `CREATE TABLE IF NOT EXISTS` / `INSERT IGNORE` / 条件 DDL，可重复执行；0023–0024、0025–0029 未使用。
文件名主干均 ≤32 字符（`schema_migrations.version` 限制）。

### 新镜像启动时是否自动应用

会。`deploy/docker-dist/Dockerfile` 把 `migrations/` 与 `drill-migrate` 一起打进镜像；
`docker-entrypoint.sh` 在 MySQL 就绪后执行 `drill-migrate -root=/opt/drill up`，0020–0022 与
历史迁移一样按 `schema_migrations` 差集自动执行。

需要注意的“守卫”与锁：

- **配置守卫**：`drill-migrate` 启动即调用 `config.Load`，`DB_USER / JWT_SECRET / SESSION_SECRET`
  缺失或仍为 `REPLACE_WITH` 时 `os.Exit(2)`，entrypoint 收到非 0 退出码后直接 `exit`，容器不会启动。
  上线前确认 `.env`（或 `DRILL_ENV_FILE`）已替换全部占位符即可，不会因 0020–0022 本身卡住。
- **并发锁**：迁移期间持有 MySQL advisory lock `drill_schema_migrations`（等待 60s）。拿到锁后才读取
  已应用版本，所以多副本同时启动时只有一个实例执行，其余等待后跳过；若 60s 内拿不到锁，该实例退出
  （由 `restart` 策略重试），属于并发争用而不是迁移失败。
- **DDL 隐式提交**：`applyFileTx` 用单事务包裹整个文件，但 MySQL 的 `CREATE TABLE / ALTER TABLE`
  会隐式提交。若 0020 或 0022 中途失败，已执行的 DDL 不会回滚、版本行也不会写入；由于三个文件都
  是幂等写法，直接重跑即可收敛。
- **镜像/挂载前提**：迁移文件必须来自新镜像。若沿用旧镜像、热修复镜像（`Dockerfile.hotfix`）
  或把旧 `migrations/` 目录挂载覆盖到 `/opt/drill/migrations`，0020–0022 不会被看到；
  `docker-dist` compose 未挂载 migrations，使用命名卷，正常重建镜像即可。

## 9. 改动文件

- `server/internal/handler/drill_seal.go`（新增）：core 清单、核对清单、缺项说明、快照读写、结束主流程。
- `server/internal/handler/drill_v1.go`：`PostDrillFinish` 改为核对/缺项/封存流程；`loadEvidenceSummary` 增加 core/support 分级。
- `server/internal/handler/drill_score.go`：抽出 `scoreInternal(forceEnded)`，结束前只计算一次。
- `server/internal/handler/drill_handler.go`：项目创建时复制核心证据清单。
- `server/internal/handler/drill_runtime.go`：重新考评在已封存项目上追加 `version+1` 快照。
- `server/internal/handler/evaluation.go`：复盘接口增加 `seal/snapshot/snapshots/endException`。
- `server/internal/handler/missing_handlers.go`：`PatchProjectStatus` 改为结束目标状态复用封存流程；
  新增 `exception/force/reason` 请求字段，支持运维强制结束（异常封存 + 审计）。
- `server/cmd/supplemental-seal`：补封存运维命令，供已结束但无快照的历史项目使用。
- `public/workbench.html`：结束核对弹窗、有缺项结束登记、按钮不再因取证缺失禁用。
- `public/evaluation.html`：封存状态展示、有快照时读快照。
- `public/report.js`：报告第 6 章与封面读快照，无封存时提示。
- `migrations/0020–0022`：见 §8。

## 10. 验证

- `go build ./...`：通过（全仓）。
- `go vet ./...`：无输出（通过）。
- `node --check public/report.js`：通过。
- `evaluation.html`、`workbench.html` 内联脚本抽取后 `node --check`：通过。
- 补封存执行：`cmd/supplemental-seal` 先 `-dry-run` 核对（19 必做/17 完成、证据 0/34、
  core 0/12、needsException=true），再单事务写入；写后五处齐全、分数一致、项目指纹不变。
- 未重启容器、未构建/替换镜像、未改动其它项目、未推送仓库。

## 11. 未完成 / 待统一上线

- “项目级核心证据清单调整”的 HTTP 入口（受 router.go 修改约束）；
- 归档前强制校验（归档时应要求已封存）；
- 证据维度按 `evidence` 表 core/support 加权的评分口径改造（见 §6）。
