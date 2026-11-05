# 内置演练剧本内容更新 · 资料采集提示词模板

> 本轮只做前期准备：产出「给外部 AI 模型的资料采集提示词模板」，**不修改任何剧本数据、不改代码逻辑、不写现役库**。
> 代码核实时间：2026-11-05；基线真源：`node/src/scenario_baseline.js`；现役库表结构：`migrations/0001_init.up.sql`。

---

## 0. 结论式摘要

### 0.1 先纠正一个关键前提

任务描述中的 `drill_steps` **不是剧本步骤定义表**，它是「实战版处置记录」表（`migrations/0001_init.up.sql:132-162`）。真正的剧本步骤定义在 **`script_nodes`**（同文件 `99-129`），剧本级信息在 **`scenarios`**（`84-96`）。

因此本次内容更新要动的是：

1. `scenarios`：`name` / `type` / `level` / `descr` / `alert`（剧本级内容）
2. `script_nodes`：`title` / `short_title` / `descr` / `guide` / `keywords` / `action` / `parallel_desc` / `evidence_req`（节点级内容）
3. `drill_steps` **一行都不动**（运行记录，历史回查依赖它）

### 0.2 用户需要提供的资料分 5 类

| 类 | 内容 | 项数 | 用途 |
|---|---|---|---|
| A | 剧本基本信息 | 6 项 | 写 `scenarios` 的内容列 |
| B | 现有流程图结构清单 | 1 张表（系统导出，只读） | 锁定结构，防止改动 |
| C | 逐节点业务场景与真实感细节 | 每节点 3-5 句 | 写 `descr` / `guide` 的主体 |
| D | 评分与取证关注点 | 每节点 1-2 条 + 全局 1 条 | 写 `evidence_req`、校验 `keywords` |
| E | 术语、合规口径、禁用表述 | 各 1 组 | 统一文风与脱敏 |

### 0.3 三条硬结论（写进提示词模板的“最高优先级约束”）

1. **结构零改动**：`id / scenario_id / type / seq / deps / anchor_id / release_timing / target / receiver / flow_type / score / required / timeout / form / pos_x / pos_y` 一律不许改。
2. **只写内容层**：本次允许改的只有 `title / short_title / descr / guide / keywords / action / parallel_desc / evidence_req` 和 `scenarios` 的 5 个内容列。
3. **缺失写“待补充”**：任何字段拿不到真实依据，必须写 `待补充`，禁止编造机构名、IP、人名、时间。

### 0.4 现有 S01 的真实内容缺口（说明本次更新确有空间）

- S01 共 **18 个主流节点 + 34 个混淆节点**（`node/src/scenario_baseline.js:16-833`）。
- 18 个主流节点 **全部没有 `descr`**，也 **全部没有 `guide`**（脚本内 `descr:` 计数 = 0）。
- 34 个混淆节点 **都有 `guide`**，但写的是“为错误动作找理由”的话术，这正是实战版需要的“看似合理但会延误处置”。
- 18 个主流节点里 **16 个绑定了 `form`（F1/F2/F3/F4/F5/F7/F8/F9）**，其中 **F9 在表单模板中不存在**（`node/src/seed_data.js` 只到 F8），这是现网遗留问题。
- `parallel_desc` 在 S01 中 **全部为空**，并行节点在画布上只有“并行”标签，没有“这一组并行要达成什么”的说明。

---

## 1. 字段地图（逐字取自 DDL 与基线代码）

### 1.1 剧本级：`scenarios`

| 真实列名 | 类型/长度 | 必填/默认 | 语义与取值 | 本次是否可改 | 影响面 |
|---|---|---|---|---|---|
| `id` | VARCHAR(32) PK | 必填 | 剧本代号，如 `S01` | ❌ 结构 | 项目 `scenario_id` 外键、节点 `scenario_id`、所有按剧本查询 |
| `name` | VARCHAR(128) | 可空 | 剧本名称，如“勒索病毒攻击演练” | ✅ 内容 | 剧本库、项目关联展示、导出材料 |
| `type` | VARCHAR(32) | 可空 | 攻击类型；当前种子写入空串（`node/src/seed.js:338`） | ✅ 内容 | 剧本库筛选/展示；`GetProjectScript` 会返回 |
| `level` | VARCHAR(8) | 可空 | `重大` / `较大` / `一般` | ⚠️ 半结构 | 新建项目时决定任务优先级 P0/P1/P2（`drill_handler.go:349-356`）；已建项目不受影响 |
| `difficulty` | VARCHAR(8) | 默认 NULL | 难度；**种子数据从未写入** | ✅ 内容（需确认口径） | 仅 `bootstrap` 回显，当前无评分逻辑 |
| `status` | VARCHAR(8) | 可空 | `启用` / 停用 | ❌ 运维 | 剧本可否被选 |
| `version` | VARCHAR(16) | 可空 | `v1.0`；保存时自动 bump | ⚙️ 自动 | 版本追溯 |
| `updated` | VARCHAR(10) | 可空 | 最近更新日期 | ⚙️ 自动 | 展示 |
| `nodes` | INT | 可空 | 主流节点数量（自动 = 主流条数） | ⚙️ 自动 | 剧本库展示 |
| `descr` | VARCHAR(512) | 可空 | 剧本简介 | ✅ 内容 | 剧本库、项目页 |
| `alert` | TEXT | 可空 | 开局告警全文（T+0） | ✅ 内容 | 大屏/剧本页/实战版开场 |

### 1.2 节点级：`script_nodes`（核心）

> 基线 JSON 键与列名的对应关系以 `node/src/scenario_baseline.js` 的 `apply()` 写入语句为准（`:4178`、`:4222-4224`、`:4264-4266`）。

| 层级 | 真实列名 | 基线 JSON 键 | 类型/长度 | 必填/默认 | 语义与取值 | 影响流程图？ |
|---|---|---|---|---|---|---|
| 🔒 结构 | `id` | `id` | VARCHAR(48) PK | 必填 | 节点唯一编号，主流 `S01-1`…`S01-18`，混淆 `DSS01-角色-序号` | 是：画布节点、`deps` 引用、评分绑定、历史 `matched_node` |
| 🔒 结构 | `scenario_id` | 由写入函数注入 | VARCHAR(32) | 必填 | 所属剧本 | 是 |
| 🔒 结构 | `type` | `type` | VARCHAR(16) | 可空 | `alert` 告警 / `task` 任务 / `parallel` 并行 / `report` 上报 / `end` 终止（`public/scenarios.html:240-246`） | 是：画布图标、`end` 过滤、任务生成过滤 |
| 🔒 结构 | `seq` | 数组下标 | INT | 可空 | 排序号；API 保存时按拓扑序重排 | 是：画布/大屏顺序 |
| 🔒 结构 | `deps` | `deps` | TEXT(JSON 数组) | 默认 `[]` | 前置节点 ID 列表 | 是：连线、解锁、评分起算 |
| 🔒 结构 | `anchor_id` | `anchor_id` | VARCHAR(48) | 可空 | 混淆节点锚定的主流节点 ID | 是：混淆下发门控 |
| 🔒 结构 | `release_timing` | `release_timing` | VARCHAR(16) | 默认 `before` | `before` = 锚点前置依赖完成即放行；`after` = 锚点完成后放行（`drill_score.go:252-280`、`drill_v1.go:113-133`） | 是 |
| 🔒 结构 | `target` | `target` | VARCHAR(32) | 可空 | **处置角色名**，必须精确匹配 `roles.name` 或 `roles.short`；否则模糊匹配，仍不命中则无人可办 | 是：角色工作台过滤、`canHandleRole` |
| 🔒 结构 | `receiver` | `receiver` | VARCHAR(256) | 可空 | 下一节点接收人，多选、顿号分隔（保存时由 `receiverStr` 规范化，`script_nodes_write.go:94-111`） | 是：实战版“接收人是否正确”扣 10%（`drill_score.go:590-597`） |
| 🔒 结构 | `flow_type` | `flow_type` | VARCHAR(16) | 默认 `correct` | `correct` 主流 / `distractor` 混淆 | 是：基础版过滤、评分口径 |
| 🔒 结构 | `score` | `score` | INT | 默认 0（混淆默认 10） | 该节点扣分权重；正确流程未完成扣全分，混淆被执行扣全分（`drill_score.go:632/666`） | 是：评分绑定 |
| 🔒 结构 | `required` | `required` | TINYINT(1) | 默认 1 | 是否必经；混淆节点为 0 | 是：遗漏判定 |
| 🔒 结构 | `timeout` | `timeout` | INT | 可空 | 处置时长（分钟）；超时扣本节点 10%（`drill_score.go:582-589`） | 是 |
| 🔒 结构 | `form` | `form` | VARCHAR(16) | 可空 | 表单模板 ID（现网 F1-F8 有模板，**F9 无模板**） | 是：表单/取证绑定 |
| 🔒 结构 | `pos_x` / `pos_y` | `posX` / `posY` | INT | 可空 | 画布坐标 | 是：布局 |
| ✍️ 内容 | `title` | `title` | VARCHAR(256) | 可空 | 节点标题（画布卡片、任务标题快照、导出口径） | 否（见 §1.3 专述） |
| ✍️ 内容 | `short_title` | `short_title` | VARCHAR(32) | 可空 | 节点简称，建议 ≤6 汉字；用于紧凑视图/大屏标签 | 否 |
| ✍️ 内容 | `descr` | `descr` | TEXT | 可空 | 处置说明/场景描述（当前 S01 全空） | 否 |
| ✍️ 内容 | `guide` | `guide` | TEXT | 可空 | 研判/引导说明。**实战版会下发给参演人**，必须只描述“怎么看、看什么”，不得暗示对错 | 否 |
| ✍️ 内容 | `keywords` | `keywords` | VARCHAR(512) | 可空 | 判定关键词，逗号分隔；编辑器标注“实战版匹配用” | 否（Go 版按 nodeId 绑定；Node 版启发式参与归属，见 §1.4） |
| ✍️ 内容 | `action` | `action` | VARCHAR(64) | 可空 | 动作标签，优先取字典 10 项（`seed_data.js:2325-2336`） | 否（同上，Node 版匹配 +3 分） |
| ✍️ 内容 | `parallel_desc` | `parallel_desc` | VARCHAR(256) | 可空 | 并行组共同目标说明（当前 S01 全空） | 否（仅展示） |
| ✍️ 内容 | `evidence_req` | 未在基线写入 | TEXT(JSON) | 默认 NULL→读为 `[]` | 取证要求数组，元素 `{type,name,note}`（`script_nodes_write.go:762-766`） | 否 |
| ✍️ 内容 | `wrong` | 未在基线写入 | TEXT(JSON) | 默认 NULL | 旧版干扰项；当前编辑器不再写入 | 否 |
| 📌 保留 | `time` | — | VARCHAR(16) | 空 | 旧字段，基线写入空串 | 否 |
| 📌 保留 | `att` | — | VARCHAR(256) | 空 | 旧字段 | 否 |
| 📌 保留 | `branches` | — | TEXT(JSON) | 空数组 | 旧字段，并行分支已由 `deps` 表达 | 否 |

### 1.3 专述：节点“名称/标题”到底算不算内容层？

**结论：`title` 算内容层，可以改；但必须遵守 3 条边界。**

依据：

- **流程图拓扑**只认 `id + deps + anchor_id + release_timing`，不认标题。改标题不会动连线、解锁、归类（`script_nodes_write.go:145-186` 的拓扑排序只读 `deps`；画布渲染读 `title` 仅作显示）。
- **评分绑定**只认 `id`：`drill_steps.matched_node` 存的是节点 ID（`drill_score.go:190-193`、`drill_v1.go:319-320`），不按标题匹配。
- **历史回查**看两处：`drill_steps.title` 是执行时复制的标题快照（`drill_runtime.go:505-508`），`script_nodes.title` 是实时值；复盘页会把两者并列。因此改标题**不会篡改历史**，但会造成“历史记录里的旧标题 + 当前剧本里的新标题”并存。

边界要求：

1. 不得借改标题改变“这一步是干什么”的语义（否则评分口径与历史记录会对不上）。
2. 画布卡片宽 236px（`public/scenarios.html:39`），标题建议 ≤24 汉字；超长会截断。
3. 已建项目的 `tasks.title` 是创建时快照（`drill_handler.go:366-368`），改剧本标题不会同步旧任务；**不要为了“看起来一致”去批量改 `tasks.title`**，那会污染历史。

> 若希望彻底避免歧义，可采用“标题不动、只补 `descr/guide/short_title`”的保守方案；本轮模板默认允许改标题，但要求用户逐条确认。

### 1.4 两个与“内容更新”直接相关的代码事实（必须知道）

1. **基线 `descr` 实际不会写入库（Node 路径缺陷）**
   `node/src/scenario_baseline.js` 里有 216 处 `descr:`，但 `apply()` 写库时取的是 `o.desc`（`:4222`、`:4264`），全文 `desc:` 出现 0 次。也就是说：**通过 Node 基线重灌，`descr` 永远是空**。
   同理 `node/src/seed.js:217` 的 INSERT 只有 22 列，**丢掉了 `short_title / receiver / anchor_id / release_timing`**。
   → 所以“一键修正”不能只改基线 JS 再重灌，必须走 MySQL 定向 `UPDATE`（详见 §5）。

2. **`action` / `keywords` 在 Node 版参与“步骤归属”启发式**
   `node/src/routes/drill.js:141-149` 用 `action` 相等 +3、`keywords` 命中 +2 来猜测一条上报步骤属于哪个节点。Go 生产版改为按 `nodeId` 绑定（`drill_v1.go:412-419`），**当前不影响评分**；但为保证两版行为一致，内容更新时 `action`/`keywords` 应与节点语义对齐，不要随手改写。

### 1.5 参考：运行记录表 `drill_steps`（本轮禁止写入）

| 真实列名 | 类型 | 语义 |
|---|---|---|
| `id` | BIGINT AI PK | 记录 ID |
| `project_id` / `scenario_id` | VARCHAR(32) | 项目/剧本 |
| `role_id` / `user` | VARCHAR(32)/VARCHAR(64) | 执行角色/执行人 |
| `action` / `title` / `descr` | VARCHAR/… | 执行时快照 |
| `step_time` | VARCHAR(16) | HH:mm |
| `matched_node` | VARCHAR(48) | **绑定的剧本节点 ID** |
| `matched` / `ontime` / `correct` | TINYINT | 命中/按时/正确 |
| `choice` | VARCHAR(256) | 选择项 |
| `found_at` / `found` / `measure` / `result` / `evidence` / `forms` | … | 取证与处置内容 |
| `flow_type` / `score` | VARCHAR/DOUBLE | 记录时快照 |
| `receiver` / `receiver_ok` | VARCHAR/TINYINT | 实际接收人/是否正确 |
| `skipped` / `skip_reason` | TINYINT/VARCHAR(500) | 跳过与原因 |
| `created_at` | VARCHAR(19) | 创建时间 |

---

## 2. 资料采集提示词模板（可直接整段复制给其它 AI 模型）

> 使用方法：把 `【】` 内的内容替换为用户资料；**不要删掉“硬约束”“输出格式”“自检清单”三节**。

```text
# 角色

你是“医院网络安全应急演练剧本内容编写专家”。你熟悉三甲医院信息科的真实业务（HIS/EMR/LIS/PACS、门诊挂号、收费、住院医嘱、医保结算、药房发药），熟悉《网络安全法》《数据安全法》《个人信息保护法》《医疗卫生机构网络安全管理办法》与等保 2.0 的常见表述，也熟悉勒索病毒、横向渗透、DDoS、钓鱼邮件、数据泄露、存储故障、边界突破、恶意代码等 8 类事件的处置顺序。

你的任务：为【剧本代号】编写“内容层资料”，供系统回填到演练剧本。你**只负责写文字内容**，不负责改流程图。

# 最高优先级硬约束（违反即整份作废）

1. 不得新增、删除、合并、拆分、重排任何节点。
2. 不得修改以下结构字段：id、scenario_id、type、seq、deps、anchor_id、release_timing、target（处置角色）、receiver（接收人）、flow_type、score、required、timeout、form、pos_x、pos_y。
3. 不得把“混淆流程”改成“正确流程”，也不得把“正确流程”改成“混淆流程”。
4. 只能填写以下内容字段：title、short_title、descr、guide、keywords、action、parallel_desc、evidence_req。
5. 缺失信息一律写“待补充”，禁止猜测、禁止编造。
6. 禁止出现任何真实机构名、真实域名、真实 IP、真实人员姓名、真实手机号、真实病历号；统一使用“某三甲医院”“门诊工作站”“10.x.x.x”等脱敏表述。
7. 不得出现“正确答案 / 错误答案 / 正确做法 / 错误做法 / 应该选 / 不要选 / 标准答案 / 解析”等提示性措辞。实战版是主流与混淆并列展示，任何一处暗示都会破坏演练。
8. 不得让混淆流程“一眼假”：它必须看起来合理、在真实医院里确实有人会这么做，但会延误处置或扩大影响。

# 输入资料（由用户提供；缺哪项就在对应输出里写“待补充”）

## A. 剧本基本信息

- 剧本代号：S__（必须与系统一致，不得改）
- 剧本名称：______
- 攻击类型：______（勒索病毒 / 横向渗透 / DDoS / 钓鱼邮件 / 数据泄露 / 存储故障 / 边界突破 / 恶意代码 / 其他）
- 演练目标：______（一句话，说明本次演练要检验什么能力）
- 适用单位类型：______（三甲综合 / 二级医院 / 专科医院 / 医联体 / 其他）
- 难度定位：______（基础版偏引导 / 实战版偏研判；说明期望的混淆强度）
- 计划时长：______ 分钟（仅用于校验各节点 timeout 之和是否合理，不写入结构字段）
- 事件定级：______（特别重大 / 重大 / 较大 / 一般）
- 开局告警（T+0）原文：______

## B. 现有流程图结构清单（只读，严禁改动）

请逐行照抄下表（由系统导出），它是唯一的结构依据：

| node_id | seq | type | target（处置角色） | flow_type | anchor_id | release_timing | timeout | form | required | score | deps |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 【逐行粘贴】 | | | | | | | | | | | |

补充说明（由用户填写）：
- 哪些节点是并行组：______
- 并行组共同目标：______
- 哪些混淆节点是成对出现的（同一锚点下 2 条）：______

## C. 逐节点业务场景与真实感细节（由用户填写）

对 B 表中每个 node_id，请用户给出 3-5 句：
1. 这个节点在真实医院里由谁、在什么时间、看到什么、先做什么；
2. 关键动作的可判定结果（做到什么程度算完成）；
3. 常见但会延误的错误做法（用于混淆节点）；
4. 需要留下什么证据/文书；
5. 该节点最容易出现的“真实细节”（如窗口排队、医保不能结算、临床催问、领导电话）。

格式：
- node_id：______
- 真实场景：______
- 可判定完成标准：______
- 常见延误做法：______
- 证据/文书：______
- 真实细节：______

## D. 评分与取证关注点

- 全局评分关注点（3-5 条）：______
- 每个节点的评分关注点（1-2 条）：______
- 必须留痕的材料类型（从以下选）：附件截图 / 操作日志 / 处置记录 / 样本摘要 / 表单文书 / 上报材料 / 报案材料 / 附件
- 哪些节点必须绑定表单（F1-F8，F9 当前无模板，不要新增 F9）：______

## E. 术语、合规口径与禁用表述

- 必须使用的术语：如 HIS、EMR、LIS、PACS、门诊挂号、收费窗口、住院医嘱、医保结算、等保 2.0、网络安全事件应急预案、Ⅱ级响应、信息科负责人、院应急指挥部
- 合规口径：______（如“1 小时内向市卫生健康局、市网信办双报”“重大事件同步向市网安大队报案”）
- 禁止出现的表述：真实机构名、真实 IP、真实人名、真实病历号、真实域名、真实系统厂商名、“正确答案/错误答案”等提示词
- 文风要求：中文、书面、面向大屏展示、短句为主、每条处置动作可执行可判定

# 输出格式（严格按此结构，一条节点一节，字段名不得改）

先输出一个 JSON 总对象，再按节点逐条重复同样的字段（便于人工核对）：

```json
{
  "scenario_id": "S01",
  "scenario_content": {
    "name": "……",
    "type": "……",
    "level": "重大",
    "descr": "……（≤120 字）",
    "alert": "……（T+0 告警全文，≤200 字）"
  },
  "nodes": [
    {
      "node_id": "S01-1",
      "title": "……（≤24 汉字）",
      "short_title": "……（≤6 汉字）",
      "descr": "……（2-4 句，写清楚‘谁在什么情况下做什么、做到什么程度’）",
      "guide": "……（仅写‘看什么、核什么’，不得暗示对错；主流节点也允许留空）",
      "keywords": ["……", "……"],
      "action": "告警核实",
      "parallel_desc": "……（仅 type=parallel 时填写，否则留空）",
      "evidence_req": [
        {"type": "附件截图", "name": "……", "note": "……"}
      ],
      "notes": "待补充项 / 需用户确认项"
    }
  ]
}
```

字段规则：
- `node_id` 必须与 B 表逐字一致，不得新增。
- `title` ≤24 汉字；`short_title` ≤6 汉字。
- `descr` 2-4 句、40-120 字；必须包含“可判定的完成标准”。
- `guide` 仅在确实需要提示“观察点”时填写；**不得出现动作结论**。
- `keywords` 3-6 个，逗号分隔（输出为数组），必须是名词或动作短语，不得写整句。
- `action` 只能从以下 10 项选：告警核实、研判分级、启动预案、隔离处置、处置恢复、取证留痕、上报审批、业务兜底、协同沟通、其他。
- `evidence_req` 每条含 `type/name/note`，每节点 0-4 条；无则输出 `[]`。
- `notes` 写“待补充/需确认”，无则写空字符串。
- 混淆节点（`flow_type=distractor`）的 `descr` 必须写成“一个看起来合理的处置动作 + 它为什么会延误”，`guide` 必须写成“当事人当时的合理化理由”，**不能出现‘这是错的’**。

# 写作要求（逐条检查）

1. 中文书面语，面向大屏展示：每条 ≤40 字，避免长定语。
2. 处置动作必须可执行、可判定：写“做什么 + 做到什么程度”，不写“加强”“尽快”“妥善”等空话。
3. 主流节点：写清“先核实什么、再处置什么、留什么证据、移交给谁”，但不得把接收人写成新角色（`receiver` 已锁定）。
4. 混淆节点：必须“看起来合理但会延误处置”，典型手法是“先观望 / 先等上级 / 先问一圈 / 只做一半 / 跳过验证 / 先恢复再取证”。
5. 术语一致：同一概念全文只用一个词（如统一用“隔离”，不要“断网/封禁/下线”混用）。
6. 合规口径一致：上报时限、上报对象、定级口径必须与用户给的 E 类资料一致。
7. 不写真实信息：所有 IP 用 `10.x.x.x`，所有机构用“某三甲医院/某市卫生健康局”，所有人名用“值班员/管理员”。
8. 不写答案提示：全文检索“正确/错误/答案/应该/不该/标准/解析”，出现即改写。

# 输出前自检清单（逐项打勾后再交付）

- [ ] B 表所有 node_id 是否 100% 覆盖，且没有新增/删除？
- [ ] 是否只输出了 8 个内容字段 + `notes`，没有输出任何结构字段的新值？
- [ ] 每个 `title` ≤24 汉字、`short_title` ≤6 汉字？
- [ ] 每个 `descr` 是否含“可判定的完成标准”？
- [ ] 混淆节点的 `descr/guide` 是否只讲“合理化理由”，没有出现“这是错的/不要选”？
- [ ] 全文是否出现“正确/错误/答案/应该/不该/标准答案/解析”？
- [ ] 是否出现真实机构名、真实 IP、真实人名、真实病历号、真实域名？
- [ ] `action` 是否全部在 10 项白名单内？
- [ ] `keywords` 是否都是 3-6 个短词，没有整句？
- [ ] `type=parallel` 的节点是否都填了 `parallel_desc`？非并行节点是否留空？
- [ ] 缺失项是否统一写“待补充”，而不是编造？
- [ ] 术语与合规口径是否与 E 类资料一致？

# 交付方式

1. 先给“完成度声明”：覆盖 X/Y 个节点，待补充 Z 项。
2. 再给 JSON 全文。
3. 最后给“待用户确认清单”（不超过 10 条）。
```

---

## 3. 完整可用示例（取自 S01 真实基线）

> 以下 `title / short_title / target / form / deps / score / anchor_id / release_timing / pos_x / pos_y` 均逐字取自 `node/src/scenario_baseline.js`。
> `descr / evidence_req` 在 S01 基线中为空或未写入，示例中的取值是**模板演示值，不是现网内容**，回填前必须由用户确认。

### 3.1 结构输入（用户照抄给模型的部分）

| node_id | seq | type | target | flow_type | anchor_id | release_timing | timeout | form | required | score | deps |
|---|---|---|---|---|---|---|---|---|---|---|---|
| S01-1 | 0 | alert | 信息科值班员 | correct | — | — | 10 | — | 1 | 0 | [] |
| S01-2 | 1 | parallel | 信息科值班员 | correct | — | — | 10 | F1 | 1 | 7 | ["S01-1"] |
| S01-3 | 2 | parallel | 网络安全管理员 | correct | — | — | 15 | F9 | 1 | 7 | ["S01-1"] |
| S01-5 | 4 | task | 系统管理员 | correct | — | — | 15 | F3 | 1 | 7 | ["S01-2","S01-3","S01-4"] |
| DSS01-duty-0-1 | 18 | alert | 信息科值班员 | distractor | S01-1 | before | 10 | — | 0 | 10 | [] |

> 说明：S01-1 的 `score=0`、`receiver="信息科值班员、网络安全管理员、数据安全管理员"`；S01-2 的 `receiver="系统管理员"`；S01-3 的 `receiver="系统管理员"`；S01-5 的 `receiver="信息科负责人"`（均见 `scenario_baseline.js:20-103`）。

### 3.2 模型应输出的内容（示例）

```json
{
  "scenario_id": "S01",
  "scenario_content": {
    "name": "勒索病毒攻击演练",
    "type": "勒索病毒",
    "level": "重大",
    "descr": "内网终端中招、文件加密、勒索提示，评估 HIS 服务器感染风险，推演断网隔离、备份恢复与上报流程。",
    "alert": "T+0 监控平台推送紧急告警：HIS 门诊工作站与共享文件服务器出现大量文件被加密（扩展名变为 .locked），桌面弹出勒索信；已有 3 台终端无法打开业务系统，疑似勒索病毒正在院内网横向扩散。"
  },
  "nodes": [
    {
      "node_id": "S01-1",
      "title": "终端批量文件被加密告警",
      "short_title": "终端告警",
      "descr": "值班员在监控平台看到多台终端文件扩展名被批量改为 .locked，桌面出现勒索信。先确认告警时间、受影响终端数量与所在网段，再把告警截图和终端清单留档，随即通知网络安全管理员与系统管理员到场。",
      "guide": "先看告警时间、受影响终端数量和网段分布，再核对是否已有业务系统无法登录。",
      "keywords": ["勒索", "加密", "告警"],
      "action": "告警核实",
      "parallel_desc": "",
      "evidence_req": [
        {"type": "附件截图", "name": "告警界面截图", "note": "含告警时间、受影响终端数量"},
        {"type": "处置记录", "name": "值班核实记录", "note": "记录首次发现时间与通知对象"}
      ],
      "notes": ""
    },
    {
      "node_id": "S01-3",
      "title": "断网隔离感染主机与共享目录",
      "short_title": "隔离感染主机",
      "descr": "网络安全管理员按终端清单逐台断开网络，关闭共享目录写入权限，确认隔离后 10 分钟内无新增加密告警；记录每台主机的 IP、MAC、隔离时间与操作人，避免误隔离业务服务器。",
      "guide": "隔离前先记录主机 IP 与 MAC；隔离后持续观察是否仍有新增加密告警。",
      "keywords": ["隔离", "断网", "封堵", "共享"],
      "action": "隔离处置",
      "parallel_desc": "在核实感染范围的同时，并行完成断网隔离、备份可用性检查与预警研判三路处置。",
      "evidence_req": [
        {"type": "操作日志", "name": "网络设备/防火墙操作日志", "note": "含隔离对象与时间"},
        {"type": "处置记录", "name": "隔离处置记录", "note": "记录主机 IP、MAC、操作人与观察结果"}
      ],
      "notes": ""
    },
    {
      "node_id": "DSS01-duty-0-1",
      "title": "先等 10 分钟观察告警是否自动消失，再考虑召集其他人员应急",
      "short_title": "先观望",
      "descr": "值班员认为勒索加密有时是一次性触发，先观察十分钟，看告警是否自行恢复，再决定是否通知其他岗位。这样可以把人力留给更确定的故障。",
      "guide": "勒索病毒首次加密后常需要手动触发或网络激活，先观望一轮可以排除偶发告警。",
      "keywords": ["观望", "等待", "暂不通知"],
      "action": "告警核实",
      "parallel_desc": "",
      "evidence_req": [],
      "notes": "short_title/keywords 为演示补写，基线未提供，需用户确认"
    }
  ]
}
```

> 例中 S01-1/S01-3 的 `title/short_title/target/form/deps/score/keywords/action` 均逐字取自基线；`descr/evidence_req` 为演示补写。混淆节点 `DSS01-duty-0-1` 的 `title/guide/anchor_id/release_timing` 逐字取自基线 `scenario_baseline.js:325-338`，`short_title/keywords` 为演示补写；它示范了“看起来合理、实则延误”的写法。

---

## 4. 多剧本批量采集方案（一次准备 N 个剧本）

### 4.1 组织方式（推荐）

**一次只做一个剧本，但一次给足该剧本的全部结构清单。**

原因：混淆节点与锚点是成对关系，跨剧本混在一起时模型极易把 `S02` 的锚点写成 `S01-5`；一次一个剧本可以把结构表压到 20-60 行，模型不容易丢行。

如果必须并行，按“一个剧本一个会话/一个任务”拆开，**不要在一个上下文里同时产出两个剧本的 JSON**。

### 4.2 分片策略（单剧本节点过多时）

- 单剧本 ≤30 个节点：一次给全量结构表 + 全量逐节点资料，一次产出。
- 单剧本 31-60 个节点：按主流节点顺序切成 2 片（如 S01-1～S01-9 / S01-10～S01-18），每片都附**完整结构表**（只读）和本片的逐节点资料；混淆节点跟随其锚点所在片。
- 单剧本 >60 个节点：按“每片 8-10 个主流节点 + 其锚定的混淆节点”切，最多 5 片。
- 每片都重复同一段“硬约束 + 输出格式 + 自检清单”，避免模型在第二片开始“自由发挥”。

### 4.3 合并与验收（必须机器校验 + 人工抽检）

1. 合并前先做**覆盖率校验**：输出 `node_id` 集合与结构表 `node_id` 集合必须完全相等（无新增、无遗漏）。
2. 做**字段白名单校验**：输出 JSON 只能出现 `node_id/title/short_title/descr/guide/keywords/action/parallel_desc/evidence_req/notes`；出现任何结构字段即打回。
3. 做**结构快照 diff**：入库前对 `script_nodes` 导出结构指纹（见 §5.4），diff 必须为空。
4. 做**禁用词扫描**：`正确|错误|答案|应该|不该|标准|解析|真实医院名|真实IP|1[3-9]\d{9}`。
5. 做**长度校验**：`title` ≤24 汉字、`short_title` ≤6 汉字、`descr` 40-120 字、`keywords` 3-6 个。
6. 人工抽检：每个剧本至少抽 3 个主流节点 + 3 个混淆节点，检查“混淆是否真的会延误、主流是否可判定”。

### 4.4 可用性检查清单（逐项通过才算可入库）

- [ ] 覆盖 100%，无孤立节点、无重复 node_id
- [ ] 结构字段零改动（机器 diff 为空）
- [ ] 8 个内容字段齐全；缺失项均为“待补充”
- [ ] 主流节点 `descr` 有“完成标准”；混淆节点 `descr` 有“合理化理由”
- [ ] `guide` 不含动作结论
- [ ] `action` 全在 10 项白名单
- [ ] `parallel_desc` 仅并行节点有值
- [ ] `evidence_req` 元素结构为 `{type,name,note}`
- [ ] 禁用词扫描 0 命中
- [ ] 术语/合规口径与 E 类资料一致
- [ ] 用户已逐条确认“待补充”和“需确认”项

---

## 5. 后续“一键修正”执行预案（只写方案，本轮不执行）

### 5.1 改哪个数据源？

| 数据源 | 位置 | 是否现役 | 本轮建议 |
|---|---|---|---|
| Node 基线 | `node/src/scenario_baseline.js` | 历史/重建用 | **同步更新**，保证未来重建不丢内容；但注意 §1.4 的 `descr` 缺陷 |
| Node 种子剧本元数据 | `node/src/seed_data.js:412-485` | 历史/重建用 | 同步更新 `name/type/level/descr/alert` |
| MySQL 现役 `script_nodes` / `scenarios` | `migrations/0001_init.up.sql` 定义 | **生产真源** | **定向 `UPDATE` 内容列**（首选） |
| Migration/seed | `migrations/` | 建库用 | 新增 `0004_scenario_content_s01.up.sql`，只写内容列，作为可追溯变更 |
| 保存 API | `POST /api/content/script-nodes` | 生产写入口 | **不推荐**用于纯内容更新：它会 `DELETE` 全剧本再重插（`script_nodes_write.go:534`），并按拓扑序重排 `seq`、重推 `receiver`、自动生成 `evidence_req`（`:556-580`），容易引入结构漂移 |

**结论：生产库走“定向 UPDATE + 结构 diff 校验”；基线 JS 同步改；migration 留档。**

### 5.2 更新顺序（务必按序）

1. **冻结**：确认没有项目处于“演练中”；通知参演单位暂停新建项目。
2. **备份**（见 §5.6）。
3. **校验产出**：按 §4.4 检查清单过一遍内容 JSON。
4. **生成补丁**：只生成 `UPDATE script_nodes SET title=?, short_title=?, descr=?, guide=?, keywords=?, action=?, parallel_desc=?, evidence_req=? WHERE id=? AND scenario_id=?`；`scenarios` 只更新 `name/type/level/descr/alert`（`level` 需用户确认，因为它影响新建项目任务优先级）。
5. **事务执行**：单事务内执行全部 `UPDATE`，任一行受影响行数 ≠ 1 即回滚。
6. **结构校验**：导出更新前后结构指纹，`diff` 必须为空（见 §5.4）。
7. **内容校验**：抽查 3 个主流 + 3 个混淆节点；调用 `GET /api/content/projects/:id/script` 与 `GET /api/drill/flows/:projectId` 看返回是否为新内容。
8. **同步基线**：把确认后的内容写回 `node/src/scenario_baseline.js` 与 `seed_data.js`；同时记录“`apply()` 取 `o.desc` 而非 `o.descr`”的缺陷（是否修复留待用户决定，本轮不动代码）。
9. **留档**：把本次内容 JSON、SQL 补丁、前后指纹、备份路径写入变更记录。

### 5.3 影响范围（必须先告知用户）

- **已创建项目会“继承”内容**：`GetProjectScript` 与 `GetDrillFlows` 都实时读 `script_nodes`（`drill_handler.go:579`、`missing_handlers.go:1476`），所以**内容改完，进行中的项目立刻看到新文案**。
- **但任务不会变**：基础版 `tasks` 是建项目时按节点快照生成的（`drill_handler.go:306-372`），`tasks.title/descr` 仍是旧值。会出现“剧本页新标题、任务中心旧标题”。
- **历史记录不会变**：`drill_steps.title` 是执行时快照（`drill_runtime.go:505-508`），已完成/已归档项目的记录保持原样；复盘页可能出现“旧记录标题 + 新节点标题”并存。
- **新建项目完全继承新内容**。
- **评分不受影响**：评分按 `id + score + timeout + flow_type`，这些都在锁定字段内。

> 建议：只在“无演练中项目”的窗口执行；执行后向用户说明“旧任务/旧记录标题不变”是预期行为，不做批量回写。

### 5.4 如何证明“流程图零改动”（推荐做法）

**结构指纹 SQL（更新前、更新后各跑一次，导出同一格式）：**

```sql
SELECT id, seq, deps, anchor_id, release_timing, target, receiver,
       flow_type, score, required, timeout, form, type, pos_x, pos_y
FROM script_nodes
WHERE scenario_id = 'S01'
ORDER BY id;
```

执行方式（示例）：

```sql
-- 会话内先建临时快照表，便于逐行 diff
CREATE TABLE _snap_s01_before AS
SELECT id, seq, deps, anchor_id, release_timing, target, receiver,
       flow_type, score, required, timeout, form, type, pos_x, pos_y
FROM script_nodes WHERE scenario_id='S01';
```

更新后：

```sql
SELECT COUNT(*) AS diff_rows
FROM _snap_s01_before b
LEFT JOIN script_nodes a ON a.id = b.id
WHERE a.id IS NULL
   OR a.seq <> b.seq
   OR a.deps <> b.deps
   OR COALESCE(a.anchor_id,'') <> COALESCE(b.anchor_id,'')
   OR COALESCE(a.release_timing,'') <> COALESCE(b.release_timing,'')
   OR a.target <> b.target
   OR COALESCE(a.receiver,'') <> COALESCE(b.receiver,'')
   OR COALESCE(a.flow_type,'') <> COALESCE(b.flow_type,'')
   OR COALESCE(a.score,0) <> COALESCE(b.score,0)
   OR COALESCE(a.required,1) <> COALESCE(b.required,1)
   OR COALESCE(a.timeout,0) <> COALESCE(b.timeout,0)
   OR COALESCE(a.form,'') <> COALESCE(b.form,'')
   OR COALESCE(a.type,'') <> COALESCE(b.type,'')
   OR COALESCE(a.pos_x,0) <> COALESCE(b.pos_x,0)
   OR COALESCE(a.pos_y,0) <> COALESCE(b.pos_y,0);
```

**验收标准：`diff_rows = 0`，且节点总数 = 更新前总数。** 任一不为 0，立即回滚。

> 注意：用 `LEFT JOIN` 只能发现“旧行变了/被删”，还要反向再查一次“新增行”：
> `SELECT COUNT(*) FROM script_nodes a LEFT JOIN _snap_s01_before b ON b.id=a.id WHERE a.scenario_id='S01' AND b.id IS NULL;` 也必须为 0。

### 5.5 需要用户确认的事项（执行前）

1. `level` 是否保持不变（影响新建项目的任务优先级 P0/P1/P2）。
2. 是否允许改 `title`（已有项目任务/历史记录仍是旧标题，会出现并存）。
3. `action` 是否严格收敛到 10 项字典（现基线存在 `对外上报`、`应急结束` 两个字典外取值）。
4. `F9` 无表单模板：是保持现状（无表单），还是本次一并决定只允许 F1-F8。
5. 是否存在“演练中”项目需要先结束。

### 5.6 回滚方式

1. **SQL 备份（首选）**

```bash
mysqldump -h <host> -u <user> -p <db> \
  scenarios script_nodes form_templates \
  > backup_scenario_20261105.sql
```

2. **表级快照（会话内快速回滚）**

```sql
CREATE TABLE _snap_scenarios_20261105 AS SELECT * FROM scenarios;
CREATE TABLE _snap_script_nodes_20261105 AS SELECT * FROM script_nodes;
-- 回滚：
UPDATE script_nodes a JOIN _snap_script_nodes_20261105 b ON a.id=b.id
SET a.title=b.title, a.short_title=b.short_title, a.descr=b.descr,
    a.guide=b.guide, a.keywords=b.keywords, a.action=b.action,
    a.parallel_desc=b.parallel_desc, a.evidence_req=b.evidence_req;
```

3. **基线文件快照**：复制 `node/src/scenario_baseline.js`、`node/src/seed_data.js` 到 `data/backups/scenario-content-20261105/`（与现有 `apply()` 的备份目录一致）。

4. **回滚后验证**：重复 §5.4 的结构 diff（必须为 0），并抽查内容是否恢复。

---

## 6. 待用户确认的问题（≤5）

1. **`level` 是否允许调整？** 它会影响之后新建项目的任务优先级（P0/P1/P2），不影响已建项目。
2. **`title` 是否允许改写？** 允许则画布/任务标题会变，但历史 `drill_steps.title` 与已建 `tasks.title` 仍是旧快照，会出现并存。
3. **`action` 是否收敛到 10 项字典？** 基线现有 `对外上报`、`应急结束` 不在字典内（`seed_data.js:2325-2336`），是否需要本次一并统一。
4. **`F9` 怎么处理？** 现基线有 9 处引用 F9（全部在 S01），但 `form_templates` 只有 F1-F8，F9 节点在实战版没有表单字段。
5. **本次覆盖范围是只做 S01，还是 8 个剧本全做？** 若全做，按 §4 一次一个剧本推进，并先确认每个剧本的 `level` 与合规口径。

---

## 7. 文件位置

- 本文件：`E:\Trae\Emergency\docs\script-authoring-prompt.md`
- 基线真源：`E:\Trae\Emergency\node\src\scenario_baseline.js`
- 剧本元数据：`E:\Trae\Emergency\node\src\seed_data.js`
- 生产表结构：`E:\Trae\Emergency\migrations\0001_init.up.sql`
- 剧本下发：`E:\Trae\Emergency\server\internal\handler\drill_handler.go`
- 实战版流程：`E:\Trae\Emergency\server\internal\handler\missing_handlers.go`
- 评分：`E:\Trae\Emergency\server\internal\handler\drill_score.go`
- 剧本保存 API：`E:\Trae\Emergency\server\internal\handler\script_nodes_write.go`
