# S01 剧本缺口补齐与评分口径变更（2026-11-05）

依据：`docs/s01-expert-review.md` 与 `.trae/tmp/s01-review-gap-audit.md`。本轮只动 S01，S02–S08 未改。

## 1. 评分口径（正确节点）

参数集中在 `server/internal/handler/score_config.go`，节点级文字质量规则存 `script_nodes.quality_rules`（迁移 `migrations/0011_script_node_quality_rules.up.sql`，只新增列）。

| 项 | 常量 / 字段 | 比例 |
|---|---|---|
| 接收人选择错误 | `ReceiverErrPenaltyRatio` | 本节点分 30% |
| 超时完成 | `TimeoutPenaltyRatio` | 本节点分 20% |
| 必含关键词缺失 | `quality_rules.requiredKeywords` / `KeywordPenaltyRatio` | 默认 30%，按缺项比例折算 |
| 处置文字质量不达标 | `quality_rules.minMeasureLen` / `minResultLen` / `requiredElements` / `TextQualityRatio` | 默认 20% |
| 证据要求未提交或未命中 | `EvidencePenaltyRatio` | 扣 20%（节点无取证要求时不扣） |
| 单节点合计上限 | `NodeMaxPenaltyRatio` | 80% |

- 关键词只匹配处置文本：处置措施、处置结果、发现情况、证据说明；**表单填写值不参与匹配**，防止把关键词塞进表单刷分。
- 关键词分级：配置了「必含关键词」时，缺一项按 `关键词比例 × 缺项数 / 必含总数` 扣分，扣分理由逐项列出缺失词；「加分关键词」每命中 1 个可抵扣 1 个缺项比例。未配置必含项时回落到旧口径（`keywords` 命中 ≥2 即通过）。
- 文字质量门槛：处置措施/结果有最小字数（默认 15 / 10 字），并要求「对象 + 动作 + 结论」三类要素（可配置为对象/动作/时间/结论任意组合）；不达标标记为无效文本并给出原因。
- 敷衍短文本：`无 / 已处理 / 正常 / 已知悉` 等黑名单词拼成、且去标点后不超过默认 8 字的文本视为未实质填写，关键词按 0 命中计算并叠加文字质量扣分；提交接口直接返回 400 并说明原因。
- `evidence_req` 为空串或 `"[]"` 视为本节点无取证要求，不再扣 20%。
- 基础版（无提交文本）不套用关键词/质量/证据规则，避免建项目即扣分。
- 明细逐条生成：「超时完成（扣本节点20%）」「接收人选择错误（扣本节点30%）」「处置关键词必含项缺失：…」「处置文本质量不达标（…）」「证据材料未提交或未命中要求…」。
- 老节点没有 `quality_rules`（348 个已有 `keywords` 的节点）全部走默认口径，不会因缺字段一次性扣分。
- 起始 100 分、`maxScore`（S01 正确节点合计 100）不变。

## 2. 表单与服务端校验

- 表单必填保持服务端全量硬校验：缺项 400，文案列出「中文名(key)」。
- F5 增加 `accept_officer`（受理人，选填）与 `evidence_hold`（证据保全，必填）。
- F6 增加 `regulator_requirement`（监管要求，必填）与 `review_required`（是否需复查，必填）。
- F8 增加 `incident_level`（定级结论）、`response_timeline`（处置时间线）、`rectify_owner`（整改责任人）、`retest_result`（复测结论），均必填。

## 3. 只读与展示

- 外部单位：`GET /api/drill/flows/:projectId` 不再 403，可查看被指派节点与表单字段；提交/跳过仍 403，由院应急指挥部代执行。
- 工作台外部角色显示「外部单位只读视图」，不展示提交按钮，不标记处置中。
- 大屏链路节点：`name`/`shortTitle` 优先取节点 `title`/`short_title`，`role` 仍为 `target`。

## 4. 数据变更（单事务，现役库）

| 对象 | 字段 | 改前 → 改后 |
|---|---|---|
| `S01-1` | title | 终端批量文件被加密告警 → 门诊终端文件被批量加密、扩展名被改并出现勒索信 |
| `S01-15` | title | 上报市网安大队 → 向市网安大队报案 |
| `S01-13` | timeout | 60 → 15（与相邻节点一致） |
| `DSS01-duty-1-1` | anchor_id | S01-1 → S01-2 |
| `role_mvnaewi63am` | short | 门诊部主任 → 数据安全员 |
| `F5/F6/F8` | fields | 新增字段见第 2 节 |

## 5. 未决

- S01-12 仍无下游节点：建议新增 `S01-19`「落实市卫健局研判意见并回执」（deps=S01-12），或在 S01-17 的 deps 增加 S01-12；本轮只给方案未写库。
- 其余 33 条混淆节点 `release_timing` 保持 `before`。
- 历史 `drill_steps` 快照不回填；新口径只影响之后产生的提交。
