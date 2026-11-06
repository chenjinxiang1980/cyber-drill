# S01 处置文字质量规则清单（52 节点样板）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S01 全部 52 个节点（correct 18 / distractor 34），含 1 个 `end` 节点与 1 个 0 分告警节点
> 依据：现网节点 `title/descr/guide/keywords/evidence_req/form/score/timeout/target/receiver`，表单模板 F1–F17，评分口径 `server/internal/handler/score_config.go`，以及 `docs/s01-expert-review.md`、`docs/s01-gap-closure.md`
> **本轮只出文档：未执行 UPDATE/INSERT，未推仓库，未改动现网容器与数据库。字段值可直接用于后续批量落库审阅。**

---

## 0. 先明确规则是怎么被引擎消费的

这决定规则不能凭感觉写，以下 8 条是本次设计的硬约束：

1. **九个字段全部生效于处置文本**：引擎只拼接 `measure`（处置措施）、`result`（处置结果）、`found`（发现情况）、`evidence`（证据说明）四段文本；**表单填写值不参与关键词匹配**。所以必含词必须是参演人在叙述里自然会写的词，不能是只在下拉框里出现的选项。
2. **关键词是字面子串匹配**：`strings.Contains`，区分大小写，不处理同义词、不处理空格差异。带空格/罗马数字/大小写混排的词只能放加分词，不放必含词（如“10 分钟”“30 台”“Ⅱ级”“ACL 降权”）。
3. **必含词扣分按缺项比例折算**：缺 1 项 = 节点分 × `keywordPenaltyRatio` × 1/必含总数；加分词每命中 1 个可抵扣 1 项。本次统一取 3 项，分别对应处置对象、动作与判据，避免靠同义词堆叠凑数。
4. **长度按字符（rune）计**：`minMeasureLen` 只管“处置措施”，`minResultLen` 只管“处置结果”，不合并计算。
5. **要素只有四种**：`对象 / 动作 / 时间 / 结论`。引擎用正则判定，四类并非每类节点都适用：`结论`只给有可验证结果的节点，`时间`只给时间本身是验收条件的节点，外部指导记录只要求`对象`。
6. **敷衍判定是整段判定**：去掉空白与标点后，若整段长度 ≤ `fillerMaxLen` 且能完全由 `fillerWords` 拼出来，才判为敷衍并返回 400。它不会因为正文里出现“已完成”就误伤长文本。
7. **所有节点都过文字质量闸门**：`drill_v1.go:470-476` 在 `isCorrect` 判定之前执行，所以**混淆节点与 0 分节点的规则同样会拦住提交**；只是扣分部分只对正确节点生效。这是混淆规则必须“可命中、可提交”的原因。
8. **单节点扣分合计上限 80%**：关键词、文字质量、证据、超时、接收人五项合计不超过节点分 80%。本清单的比例是在这个上限内分配的。

### 0.1 长度分档（本次实际使用的八档）

| 档 | 措施/结果 | 适用节点 | S01 示例 |
|---|---|---|---|
| T1 轻量 | 15 / 10 | 低分混淆、单一动作 | DSS01-clinical-13-1、DSS01-clinical-13-2、DSS01-commander-16-1、DSS01-cyberpolice-15-2 |
| T2 标准 | 20 / 12 | 8 分混淆、单一动作叙述 | DSS01-cac-12-1、DSS01-cac-12-2、DSS01-commander-16-2、DSS01-commander-6-1 |
| T3 记录 | 30 / 18 | 首报、审批、指导、报案 | S01-1、S01-12、S01-13、S01-15 |
| T4 关键 | 35 / 20 | 院内报告、结束 | S01-17、S01-5、S01-6、S01-8 |
| T5 复杂 | 40 / 25 | 范围核实、备份校验、兜底、补录 | S01-10、S01-14、S01-18、S01-2 |
| T6 高复杂 | 45 / 25–28 | 隔离、定级、恢复 | S01-11 |
| T7 取证 | 50 / 30 | 证据固定 | — |
| T8 综合报告 | 60 / 40 | 复盘总结 | — |

> 分档依据不是分值本身，而是该节点在验收上需要写清几个对象/判据：一个动作一个对象 → T1/T2；动作+对象+结论 → T3；多对象+顺序+例外 → T5/T6；证据链+时间 → T7；时间线+责任+整改 → T8。

### 0.2 比例档

| 节点档 | `textQualityRatio` | `keywordPenaltyRatio` | 说明 |
|---|---|---|---|
| 正确节点 7 分 | 0.20 | 0.30 | 关键处置，关键词是硬验收 |
| 正确节点 6 分 / 0 分 | 0.20 | 0.25（0 分节点 0.30） | 常规节点，质量与关键词并重 |
| 混淆 8 分 | 0.15 | 0.25 | 选中已扣满分值，质量只作描述完整性约束 |
| 混淆 5 分 | 0.15 | 0.20 | 同上，取更轻档 |
| 混淆 3 分 | 0.10 | 0.15 | 最低档，避免双重惩罚 |

### 0.3 敷衍词档（按节点类型，不是一张表套全部）

| 档 | 名称 | 词表 | 上限 | 用于 |
|---|---|---|---|---|
| P1 | 核实 | 无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK | 8 | 核实 |
| P2 | 处置 | 无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK | 8 | 处置 |
| P3 | 研判 | 无、已研判、已定级、按预案、正常、已知悉、ok、OK | 8 | 研判 |
| P4 | 上报/指导记录 | 无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK | 10 | 报告、指导、报案（**刻意不含“已上报/已报案”**） |
| P5 | 取证 | 无、已取证、已保全、已完成、正常、ok、OK | 8 | 取证 |
| P6 | 恢复 | 无、已恢复、已处理、正常、已完成、ok、OK | 8 | 恢复 |
| P7 | 观望/延误 | 无、已观察、等待中、已在处理、正常、ok、OK | 6 | 拖延型混淆 |
| P8 | 审批/结束 | 无、已审批、同意、已阅、收到、按要求处理、ok、OK | 8 | 审批/结束 |
| P9 | 补录 | 无、已补录、已处理、正常、已完成、ok、OK | 8 | 补录 |
| P10 | 兜底切换 | 无、已切换、已处理、正常、已完成、ok、OK | 8 | 兜底切换 |

> 报告/指导/报案档（P4）刻意不含“已上报、已报送、已报案”，因为对这类节点而言那是必须完成的动作，不是敷衍；但仍保留“已知悉、已阅、收到”等无实质内容的词。

---

## 1. 正确节点规则（18 个）

### S01-1 · HIS 数据库服务器数据文件被批量加密、数据库服务不可用

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 10 min

- `requiredKeywords`：`勒索信`、`扩展名`、`共享盘`
- `bonusKeywords`：`受影响终端`、`首次发现时间`、`未重启`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报要把勒索特征与扩散面说清，必含词锁定勒索信、加密扩展名、共享盘三个事实；该节点 0 分，规则只作提交闸门不产生扣分。

### S01-2 · 15 分钟内确认数据库服务器与业务系统受影响范围

`correct` ｜ 类型 `parallel` ｜ 表单 `F1` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`受影响主机`、`仍在加密`、`网段`
- `bonusKeywords`：`共享目录访问告警`、`科室`、`IP`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：核实范围的关键不是看过告警，而是能给出主机清单、判断是否仍在加密并定位到网段/科室，故不取标题里的“15 分钟”。

### S01-3 · 断网隔离感染主机与共享目录

`correct` ｜ 类型 `parallel` ｜ 表单 `F10` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`共享目录`、`隔离`、`ACL|访问控制列表|访问控制策略`
- `bonusKeywords`：`核心服务器`、`隔离清单`、`交换机端口`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：隔离节点必须写清“关共享写入→隔离主机→核心服务器 ACL 降权”三段，必含词分别对应处置对象、动作与例外处理。

### S01-4 · 核对恢复点并抽验备份可读

`correct` ｜ 类型 `parallel` ｜ 表单 `F11` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`恢复点`、`RPO|恢复点目标|恢复时间目标`、`可读性`
- `bonusKeywords`：`抽验`、`全量`、`增量`、`异地副本`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：备份可用性不能只看任务成功，必须落到恢复点、RPO 与可读性三件事；加分词奖励真正做了抽验与异地副本核对的人。

### S01-5 · 预警研判：确认重大（Ⅱ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`定级依据|定级理由|分级依据|判定依据`、`患者安全`、`重大`
- `bonusKeywords`：`扩散趋势`、`数据量`、`Ⅱ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：定级要同时有依据、患者安全影响与明确等级；用“重大”而非罗马数字作为必含词，避免 Ⅱ/II/2 写法差异造成误判。

### S01-6 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 6 ｜ 超时 10 min

- `requiredKeywords`：`简报`、`启动`、`指挥部`
- `bonusKeywords`：`Ⅱ级`、`已采取措施`、`10分钟`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：院内报告要有一页简报、明确启动请求并报到指挥部；不要求“结论”要素，避免把“建议”硬写成结论而误伤。

### S01-7 · 院应急指挥部批准启动Ⅱ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F14` ｜ 分值 6 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`授权范围|授权权限|权限范围`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`Ⅱ级`、`启动时间`、`停机`、`数据恢复`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：审批节点的可核查产物是启动令、授权范围与响应级别；加分词奖励把停机/断网/恢复/上报四类授权写全的人。

### S01-8 · 同时报市卫生健康局和市网信办

`correct` ｜ 类型 `parallel` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`市网信办|网信办|市委网信办`、`回执`
- `bonusKeywords`：`1小时`、`时限依据`、`不等回复`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：双报的要点是“两家同时+时限+回执”，必含词直接锁定两个接收单位与回执，避免只写一句“已上报”就过关。

### S01-9 · 固定勒索样本与日志证据

`correct` ｜ 类型 `parallel` ｜ 表单 `F13` ｜ 分值 6 ｜ 超时 20 min

- `requiredKeywords`：`内存镜像`、`哈希`、`日志`
- `bonusKeywords`：`样本`、`勒索信`、`SHA-256`、`易失`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：取证要覆盖易失证据、样本与日志，并以哈希收口；要求“时间”要素，用于证明取证发生在恢复之前。

### S01-10 · 业务兜底：六大环节全面启用手工流程

`correct` ｜ 类型 `parallel` ｜ 表单 `F16` ｜ 分值 6 ｜ 超时 25 min

- `requiredKeywords`：`手工流程`、`单据号段|单据编号|单据号`、`回退|回滚|切回|回切`
- `bonusKeywords`：`挂号`、`收费`、`发药`、`检验申请`、`责任人`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：兜底不是“切了手工”，而是写清哪些环节、用什么单据号段、何时回退；加分词奖励四项关键环节与责任人。

### S01-11 · 从备份恢复业务并验证 HIS 可用

`correct` ｜ 类型 `parallel` ｜ 表单 `F7` ｜ 分值 6 ｜ 超时 30 min

- `requiredKeywords`：`恢复点`、`RPO|恢复点目标|恢复时间目标`、`业务用例`
- `bonusKeywords`：`隔离环境`、`二次加密`、`RTO`、`未恢复系统`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：恢复必须写恢复点、RPO 与业务用例三件事，否则无法证明恢复可用；加分词奖励隔离环境恢复与二次加密检查。

### S01-12 · 市卫生健康局研判指导

`correct` ｜ 类型 `parallel` ｜ 表单 `F15` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`响应级别|响应等级|响应分级`、`证据保全`、`回执`
- `bonusKeywords`：`上报时限`、`联系人`、`升级`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：外部指导节点记录的是“对方要求”，不是本院处置动作，要素只留“对象”，避免把指导意见写成处置过程而误判。

### S01-13 · 接收市网信办舆情与监管口径指导

`correct` ｜ 类型 `parallel` ｜ 表单 `F6` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`舆情`、`口径`、`复查`
- `bonusKeywords`：`监测`、`统一表述`、`监管要求`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：网信指导的产出是舆情口径、监管要求与是否复查，必含词与 S01-12 刻意错开，防止两张指导单写成同一段话。

### S01-14 · 数据补录：六大环节手工单补录与稽核

`correct` ｜ 类型 `task` ｜ 表单 `F17` ｜ 分值 6 ｜ 超时 30 min

- `requiredKeywords`：`补录`、`双人复核|双人核对|两人复核`、`差异`
- `bonusKeywords`：`封账`、`单据号`、`稽核`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：补录要写范围、双人复核与差异处理，封账才是收口；加分词奖励写清单据号与稽核规则的人。

### S01-15 · 向市网安大队报案

`correct` ｜ 类型 `parallel` ｜ 表单 `F5` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`报案`、`受理`、`证据材料`
- `bonusKeywords`：`先行`、`不等待`、`书面材料`、`受理编号`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报案要“先行+书面材料+受理”，敷衍词表刻意不含“已上报/已报案”，避免把合规动作当敷衍内容处理。

### S01-16 · 市网安大队处置指导

`correct` ｜ 类型 `task` ｜ 表单 `F15` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`受理`、`取证要求`、`保护现场`
- `bonusKeywords`：`受理编号`、`非涉案业务`、`不得重装`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：网安指导的可核查产出是受理、取证要求与保护现场边界；加分词奖励“非涉案业务可恢复”这一最易漏掉的边界。

### S01-17 · 应急结束：院应急指挥部宣布Ⅱ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F14` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`二次加密|再次加密|复加密`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`业务恢复`、`证据固定`、`复盘`、`逐条核对`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束节点必须逐条核对四项条件，“二次加密+闭环”是区别于“业务恢复即结束”的关键，故设为必含。

### S01-18 · 总结经验教训

`correct` ｜ 类型 `end` ｜ 表单 `F8` ｜ 分值 6 ｜ 超时 300 min

- `requiredKeywords`：`时间线`、`整改`、`责任人`
- `bonusKeywords`：`根因`、`复测`、`时限`、`得失`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘报告篇幅最长，要求时间线、整改与责任人三要素；加分词奖励根因、复测与得失，避免写成流水账。该 end 节点当前不进实战 flows，规则为结构变更后预留。

---

## 2. 混淆节点规则（34 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS01-cac-12-1 · 单系统事件先院内处置，达到上报标准再报

`distractor` ｜ 分值 8 ｜ 超时 60 min ｜ 表单 `无`

- `requiredKeywords`：`单系统`、`上报标准`、`时限`
- `bonusKeywords`：`1小时`、`监管`、`先行处置`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等达标再报”，其合理写法必然出现单系统、上报标准与时限；按 8 分混淆档取 20/12。

### DSS01-cac-12-2 · 建议院方把所有受感染服务器重新装系统

`distractor` ｜ 分值 8 ｜ 超时 60 min ｜ 表单 `无`

- `requiredKeywords`：`重装`、`服务器`、`证据`
- `bonusKeywords`：`全部服务器`、`残留后门`、`恢复时间`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：重装动作的写法要素是对象（服务器）、动作（重装）与后果（证据灭失），不把“彻底”设必含，避免照抄标题即过。

### DSS01-clinical-13-1 · 让挂号收费窗口集中时段再补录

`distractor` ｜ 分值 5 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`集中补录`、`晚间`、`积压`
- `bonusKeywords`：`人力`、`收费`、`发药`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：集中补录的合理写法会带时段、积压现状与补录方式；5 分混淆按轻量档，保证能被提交出来。

### DSS01-clinical-13-2 · 先补急诊与住院，普通门诊后续批量补

`distractor` ｜ 分值 5 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`急诊`、`住院`、`门诊`
- `bonusKeywords`：`批量补录`、`对平`、`先急后缓`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：“先急后缓”必须落到急诊/住院/门诊三类对象才写得具体，加分词奖励“对平”这一财务后果。

### DSS01-commander-16-1 · 等所有业务环节恢复一周后再宣布结束

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`观察一周`、`无复发`、`结束`
- `bonusKeywords`：`二次加密`、`资源占用`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：观察一周的错误动作，其合理写法会写观察期与“无复发”判据；延误型混淆天然要求时间要素。

### DSS01-commander-16-2 · 核心业务恢复即可结束，剩余系统并行恢复

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`核心业务`、`结束`、`并行恢复`
- `bonusKeywords`：`证据固定`、`上报闭环`、`剩余系统`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该错误判断的写法会写核心业务、剩余系统与并行恢复；加分词奖励被它省略的证据固定与上报闭环。

### DSS01-commander-6-1 · 先小范围口头启动，正式启动令待处置完成后补签

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`口头启动`、`补签`、`启动令`
- `bonusKeywords`：`避免惊动`、`小范围`、`授权`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：口头启动后补签的写法必然落到口头、补签与启动令三个动作，不把“授权缺失”这种后果词设必含。

### DSS01-commander-6-2 · 维持现有响应级别，待业务大面积中断后再升级

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`维持`、`响应级别`、`大面积中断`
- `bonusKeywords`：`升级`、`资源`、`可控`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：维持级别的写法会写当前级别与升级触发条件；加分词奖励“资源占用/可控”这一犹豫理由。

### DSS01-cyberpolice-15-1 · 先让院方自行恢复业务，取证由警方处理

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`自行恢复`、`取证`、`现场`
- `bonusKeywords`：`警方`、`覆盖`、`残留日志`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先恢复后取证的写法会写自行恢复、取证与现场；加分词奖励“覆盖/残留日志”这类后果描述。

### DSS01-cyberpolice-15-2 · 要求院方全面停机等待，不得做任何恢复准备

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`全面停机`、`现场`、`等待`
- `bonusKeywords`：`不得恢复`、`业务中断`、`勘验`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：全面停机等待的写法会写停机范围、现场保护与等待；3 分混淆取最低档，避免提交被硬门槛卡死。

### DSS01-datasafe-3-1 · 核对备份任务成功日志，确认无失败即视为可用

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`备份日志`、`成功`、`可用`
- `bonusKeywords`：`无需验证`、`任务状态`、`容量`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只看日志的写法会写日志、任务成功与“即可用”判断；加分词奖励“无需验证”这一关键错误理由。

### DSS01-datasafe-3-2 · 联系数据存储厂商远程协助验证恢复可行性，等待厂商确认

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`厂商`、`远程`、`等待`
- `bonusKeywords`：`30分钟`、`60分钟`、`恢复可行性`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：等厂商的写法会写厂商、远程与等待；时间要素对应 30–60 分钟的拖延代价。

### DSS01-duty-0-1 · 先观察 10 分钟，确认告警是否重复出现

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`观察`、`重复`、`误报`
- `bonusKeywords`：`10分钟`、`采集抖动`、`全员空跑`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：观望型混淆的写法必然带观察动作与重复/误报判据；不把带空格的“10 分钟”设必含，规避写法差异。

### DSS01-duty-0-2 · 按科室逐一确认业务是否异常，汇总后再统一处置

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`科室`、`确认`、`汇总`
- `bonusKeywords`：`通讯录`、`20分钟`、`误判`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：逐科室确认的写法会写科室、确认与汇总；与 duty-1-1 用不同必含词区分两条“问一圈”型混淆。

### DSS01-duty-1-1 · 按科室通讯录批量外呼，等反馈齐后再处置

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`外呼`、`反馈`、`汇总`
- `bonusKeywords`：`通讯录`、`15分钟`、`科室`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：批量外呼的写法要素是外呼动作、反馈来源与汇总判断，与 duty-0-2 的“逐科室确认”形成不同文本指纹。

### DSS01-healthbureau-11-1 · 建议院方以业务为重，先保障业务，后续再考虑溯源取证

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`业务`、`溯源`、`取证`
- `bonusKeywords`：`患者安全`、`恢复后`、`现场`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：“先保业务后溯源”的写法会写业务、溯源与取证三项安排；加分词奖励患者安全与现场覆盖判断。

### DSS01-healthbureau-11-2 · 建议由医院自行定级，卫健局不参与研判

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`自行定级`、`卫健局`、`研判`
- `bonusKeywords`：`行业口径`、`偏低`、`层级`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：自行定级的写法会写定级主体、卫健局角色与研判安排；加分词奖励“行业口径缺失、可能偏低”这一后果。

### DSS01-itlead-14-1 · 先内部核查，确认攻击来源后再报案

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`自查`、`来源`、`报案`
- `bonusKeywords`：`数小时`、`错过`、`立案`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先自查再报案的写法会写自查、来源与报案；要求时间要素，对应“自查数小时”的延误本质。

### DSS01-itlead-14-2 · 仅电话口头报案，不提交书面材料与证据

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`书面材料`、`证据`
- `bonusKeywords`：`受理号`、`后补`、`立案`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：口头报案的写法会写口头方式、书面材料与证据缺口；加分词奖励“受理号/后补材料”这一自认为聪明的理由。

### DSS01-itlead-5-1 · 先在信息科内部对报告内容做一次完整审议再上报

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`内部审议`、`统一口径`、`上报`
- `bonusKeywords`：`30分钟`、`避免追问`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：科内审议的写法会写审议范围、统一口径与上报；轻量档，时间要素对应约 30 分钟的延迟。

### DSS01-itlead-5-2 · 等备份恢复结果出来后再一并上报

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`恢复结果`、`一并上报`、`等待`
- `bonusKeywords`：`30分钟`、`信息齐全`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：等信息再报的写法会写恢复结果、一并上报与等待；加分词奖励“信息齐全/避免多次打扰”的自我合理化。

### DSS01-itlead-7-1 · 先报市卫生健康局，等他们给意见再同步报网信办

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市卫生健康局`、`意见`、`网信办`
- `bonusKeywords`：`分批上报`、`1小时`、`时限`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：分批上报的写法会写两个接收单位与“等意见”；加分词奖励 1 小时时限，暴露其合规风险。

### DSS01-itlead-7-2 · 让网络安全管理员先写一份详细技术报告再上报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`技术报告`、`成文`、`上报`
- `bonusKeywords`：`攻击链`、`影响面`、`1小时`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先成文再报的写法会写技术报告、成文与上报；加分词奖励攻击链/影响面这类“写得越全越晚”的内容。

### DSS01-medlead-8-1 · 仅把最核心的收费、发药环节切手工，其他医疗环节等待系统恢复

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`收费`、`发药`、`手工`
- `bonusKeywords`：`门诊医嘱`、`检验申请`、`滞留`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只切两条线的写法会写收费/发药与手工切换；加分词奖励被遗漏的门诊医嘱、检验申请与患者滞留后果。

### DSS01-medlead-8-2 · 先维持 HIS 运行，仅对报障科室做局部手工登记

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`局部`、`手工登记`、`HIS`
- `bonusKeywords`：`报障科室`、`住院`、`检验`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：局部登记的写法会写局部范围、手工登记与 HIS 现状；加分词奖励覆盖不足的住院/检验环节。

### DSS01-netsec-1-2 · 只看防火墙日志，不核查终端，判断无需扩大隔离

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`防火墙日志`、`边界`、`不扩大`
- `bonusKeywords`：`孤立事件`、`终端`、`抽样`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只看边界的写法会写防火墙日志、边界判断与不扩大隔离；加分词奖励“孤立事件”这一错误结论。

### DSS01-netsec-2-1 · 只隔离已加密的主机，其他疑似被感染主机先观察

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`隔离`、`已加密`、`疑似`
- `bonusKeywords`：`观察`、`业务连续性`、`共享`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只隔已加密的写法会写隔离对象（已加密/疑似）与观察安排；加分词奖励“业务连续性”这一犹豫理由。

### DSS01-netsec-9-1 · 先恢复业务，恢复后再补做取证

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`恢复`、`取证`、`证据`
- `bonusKeywords`：`重启`、`覆盖`、`内存`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先恢复后取证的写法会写恢复、取证与证据；加分词奖励重启/内存覆盖等“证据已灭失”的具体原因。

### DSS01-netsec-9-2 · 取证完成后删除加密样本与内存镜像，释放存储

`distractor` ｜ 分值 3 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`删除`、`样本`、`存储`
- `bonusKeywords`：`归档`、`哈希`、`内存镜像`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：删样本的写法会写删除对象（样本）、动作（删除）与目的（释放存储）；3 分混淆取最低档。

### DSS01-sysadmin-10-1 · 直接用最新快照全量回滚，不校验快照完整性

`distractor` ｜ 分值 8 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`快照`、`回滚`、`完整性`
- `bonusKeywords`：`未校验`、`生产`、`损坏`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：快照回滚的写法会写快照、回滚与完整性判断；加分词奖励“未校验/带毒快照”这一被忽略的风险。

### DSS01-sysadmin-10-2 · 数据恢复后立即上线，边运行边观察

`distractor` ｜ 分值 5 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`上线`、`验证`、`观察`
- `bonusKeywords`：`边运行`、`用例`、`异常`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：恢复即上线的写法会写上线、验证缺口与观察安排；加分词奖励“边运行/用例缺失”的具体说法。

### DSS01-sysadmin-2-2 · 先对全部主机做整盘镜像，镜像完成后再统一隔离

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`整盘镜像`、`隔离`、`取证`
- `bonusKeywords`：`1-2小时`、`联网`、`扩散`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先镜像后隔离的写法会写整盘镜像、隔离顺序与取证目的；时间要素对应 1–2 小时的延误窗口。

### DSS01-sysadmin-4-1 · 按加密主机数量对照预案阈值，超过 30 台即定Ⅱ级

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`阈值`、`加密主机`、`快速定级`
- `bonusKeywords`：`30台`、`Ⅱ级`、`数量`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：套阈值的写法会写阈值、加密主机与快速定级；不把“30 台”设必含，避免空格与数字写法差异导致无法命中。

### DSS01-sysadmin-4-2 · 按Ⅰ级先行处置，定级后续再修正

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`过度升级`、`定级`、`追责`
- `bonusKeywords`：`先高后低`、`Ⅰ级`、`资源`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先按Ⅰ级的写法会写过度升级、定级与“避免追责”的理由；不把罗马数字设必含，规避 Ⅰ/I/1 的写法差异。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S01-1 | 正确 | — | 0 | 勒索信、扩展名、共享盘 | 受影响终端、首次发现时间、未重启 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S01-2 | 正确 | F1 | 7 | 受影响主机、仍在加密、网段 | 共享目录访问告警、科室、IP | 40/25 | 对象、动作、结论 | P1 | 0.20/0.30 |
| S01-3 | 正确 | F10 | 7 | 共享目录、隔离、ACL\|访问控制列表\|访问控制策略 | 核心服务器、隔离清单、交换机端口 | 40/25 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S01-4 | 正确 | F11 | 7 | 恢复点、RPO\|恢复点目标\|恢复时间目标、可读性 | 抽验、全量、增量、异地副本 | 40/25 | 对象、动作、结论 | P1 | 0.20/0.30 |
| S01-5 | 正确 | F3 | 7 | 定级依据\|定级理由\|分级依据\|判定依据、患者安全、重大 | 扩散趋势、数据量、Ⅱ级 | 35/20 | 对象、动作、结论 | P3 | 0.20/0.30 |
| S01-6 | 正确 | F4 | 6 | 简报、启动、指挥部 | Ⅱ级、已采取措施、10分钟 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S01-7 | 正确 | F14 | 6 | 启动令、授权范围\|授权权限\|权限范围、响应级别\|响应等级\|响应分级 | Ⅱ级、启动时间、停机、数据恢复 | 30/18 | 对象、动作、时间 | P8 | 0.20/0.25 |
| S01-8 | 正确 | F2 | 6 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、市网信办\|网信办\|市委网信办、回执 | 1小时、时限依据、不等回复 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S01-9 | 正确 | F13 | 6 | 内存镜像、哈希、日志 | 样本、勒索信、SHA-256、易失 | 40/25 | 对象、时间、结论 | P5 | 0.20/0.25 |
| S01-10 | 正确 | F16 | 6 | 手工流程、单据号段\|单据编号\|单据号、回退\|回滚\|切回\|回切 | 挂号、收费、发药、检验申请、责任人 | 40/25 | 对象、动作、结论 | P10 | 0.20/0.25 |
| S01-11 | 正确 | F7 | 6 | 恢复点、RPO\|恢复点目标\|恢复时间目标、业务用例 | 隔离环境、二次加密、RTO、未恢复系统 | 45/28 | 对象、动作、结论 | P6 | 0.20/0.25 |
| S01-12 | 正确 | F15 | 6 | 响应级别\|响应等级\|响应分级、证据保全、回执 | 上报时限、联系人、升级 | 30/18 | 无 | P4 | 0.20/0.25 |
| S01-13 | 正确 | F6 | 6 | 舆情、口径、复查 | 监测、统一表述、监管要求 | 30/18 | 无 | P4 | 0.20/0.25 |
| S01-14 | 正确 | F17 | 6 | 补录、双人复核\|双人核对\|两人复核、差异 | 封账、单据号、稽核 | 40/25 | 对象、时间、结论 | P9 | 0.20/0.25 |
| S01-15 | 正确 | F5 | 6 | 报案、受理、证据材料 | 先行、不等待、书面材料、受理编号 | 30/18 | 对象、时间 | P4 | 0.20/0.25 |
| S01-16 | 正确 | F15 | 6 | 受理、取证要求、保护现场 | 受理编号、非涉案业务、不得重装 | 30/18 | 无 | P4 | 0.20/0.25 |
| S01-17 | 正确 | F14 | 6 | 结束条件\|终止条件\|结束标准\|终止标准、二次加密\|再次加密\|复加密、闭环\|闭环管理\|办结\|归档 | 业务恢复、证据固定、复盘、逐条核对 | 35/20 | 对象、动作、结论 | P8 | 0.20/0.25 |
| S01-18 | 正确 | F8 | 6 | 时间线、整改、责任人 | 根因、复测、时限、得失 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS01-cac-12-1 | 混淆 | — | 8 | 单系统、上报标准、时限 | 1小时、监管、先行处置 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS01-cac-12-2 | 混淆 | — | 8 | 重装、服务器、证据 | 全部服务器、残留后门、恢复时间 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS01-clinical-13-1 | 混淆 | — | 5 | 集中补录、晚间、积压 | 人力、收费、发药 | 15/10 | 对象 | P9 | 0.15/0.20 |
| DSS01-clinical-13-2 | 混淆 | — | 5 | 急诊、住院、门诊 | 批量补录、对平、先急后缓 | 15/10 | 对象 | P9 | 0.15/0.20 |
| DSS01-commander-16-1 | 混淆 | — | 5 | 观察一周、无复发、结束 | 二次加密、资源占用 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-commander-16-2 | 混淆 | — | 8 | 核心业务、结束、并行恢复 | 证据固定、上报闭环、剩余系统 | 20/12 | 对象 | P3 | 0.15/0.25 |
| DSS01-commander-6-1 | 混淆 | — | 8 | 口头启动、补签、启动令 | 避免惊动、小范围、授权 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS01-commander-6-2 | 混淆 | — | 8 | 维持、响应级别、大面积中断 | 升级、资源、可控 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS01-cyberpolice-15-1 | 混淆 | — | 8 | 自行恢复、取证、现场 | 警方、覆盖、残留日志 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS01-cyberpolice-15-2 | 混淆 | — | 3 | 全面停机、现场、等待 | 不得恢复、业务中断、勘验 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS01-datasafe-3-1 | 混淆 | — | 8 | 备份日志、成功、可用 | 无需验证、任务状态、容量 | 20/12 | 对象 | P1 | 0.15/0.25 |
| DSS01-datasafe-3-2 | 混淆 | — | 5 | 厂商、远程、等待 | 30分钟、60分钟、恢复可行性 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-duty-0-1 | 混淆 | — | 8 | 观察、重复、误报 | 10分钟、采集抖动、全员空跑 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS01-duty-0-2 | 混淆 | — | 5 | 科室、确认、汇总 | 通讯录、20分钟、误判 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-duty-1-1 | 混淆 | — | 5 | 外呼、反馈、汇总 | 通讯录、15分钟、科室 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-healthbureau-11-1 | 混淆 | — | 8 | 业务、溯源、取证 | 患者安全、恢复后、现场 | 20/12 | 对象 | P3 | 0.15/0.25 |
| DSS01-healthbureau-11-2 | 混淆 | — | 5 | 自行定级、卫健局、研判 | 行业口径、偏低、层级 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS01-itlead-14-1 | 混淆 | — | 8 | 自查、来源、报案 | 数小时、错过、立案 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS01-itlead-14-2 | 混淆 | — | 8 | 口头报案、书面材料、证据 | 受理号、后补、立案 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS01-itlead-5-1 | 混淆 | — | 5 | 内部审议、统一口径、上报 | 30分钟、避免追问 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-itlead-5-2 | 混淆 | — | 5 | 恢复结果、一并上报、等待 | 30分钟、信息齐全 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS01-itlead-7-1 | 混淆 | — | 8 | 市卫生健康局、意见、网信办 | 分批上报、1小时、时限 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS01-itlead-7-2 | 混淆 | — | 8 | 技术报告、成文、上报 | 攻击链、影响面、1小时 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS01-medlead-8-1 | 混淆 | — | 8 | 收费、发药、手工 | 门诊医嘱、检验申请、滞留 | 20/12 | 对象 | P10 | 0.15/0.25 |
| DSS01-medlead-8-2 | 混淆 | — | 5 | 局部、手工登记、HIS | 报障科室、住院、检验 | 15/10 | 对象 | P10 | 0.15/0.20 |
| DSS01-netsec-1-2 | 混淆 | — | 8 | 防火墙日志、边界、不扩大 | 孤立事件、终端、抽样 | 20/12 | 对象 | P1 | 0.15/0.25 |
| DSS01-netsec-2-1 | 混淆 | — | 8 | 隔离、已加密、疑似 | 观察、业务连续性、共享 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS01-netsec-9-1 | 混淆 | — | 8 | 恢复、取证、证据 | 重启、覆盖、内存 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS01-netsec-9-2 | 混淆 | — | 3 | 删除、样本、存储 | 归档、哈希、内存镜像 | 15/10 | 对象 | P5 | 0.10/0.15 |
| DSS01-sysadmin-10-1 | 混淆 | — | 8 | 快照、回滚、完整性 | 未校验、生产、损坏 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS01-sysadmin-10-2 | 混淆 | — | 5 | 上线、验证、观察 | 边运行、用例、异常 | 15/10 | 对象 | P6 | 0.15/0.20 |
| DSS01-sysadmin-2-2 | 混淆 | — | 8 | 整盘镜像、隔离、取证 | 1-2小时、联网、扩散 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS01-sysadmin-4-1 | 混淆 | — | 5 | 阈值、加密主机、快速定级 | 30台、Ⅱ级、数量 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS01-sysadmin-4-2 | 混淆 | — | 5 | 过度升级、定级、追责 | 先高后低、Ⅰ级、资源 | 15/10 | 对象 | P3 | 0.15/0.20 |

---

## 4. 总量、分布与风险

### 4.1 S01 规则总量

- 节点 52：正确 18（含 `end` 1、0 分告警 1）、混淆 34；规则条数 52，无遗漏。
- 必含词合计 156 个，平均 3.0 个/节点；加分词合计 162 个。
- 长度档：15/10 共 15 个；20/12 共 19 个；30/18 共 6 个；35/20 共 4 个；40/25 共 7 个；45/28 共 1 个。
- 要素：三类（对象/动作/结论）13 个；两类 3 个；四类 0 个；仅对象 36 个。
- 表单绑定（正确节点）：无表单 1（S01-1）、F1/F2/F3/F4/F5/F6/F7/F8/F10/F11/F13/F16/F17 各 1、F14 2、F15 2。

### 4.2 风险最高的几类节点

1. **混淆节点（34 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12。
2. **表单密集型正确节点（S01-2/4/11/14/16）**：参演人容易把内容写进表单、处置文本只写一句“已按表单完成”。必含词因此取自叙述中必须交代的对象与判据，而不是下拉选项；但仍建议演练前用一份样例文本做命中自测。
3. **外部指导节点（S01-12/13/16）**：文本是“记录对方意见”，不是本院处置，容易出现没有动作/结论词的情况。本轮把要素降为仅“对象”或“对象+动作”，把区分度交给必含词。
4. **0 分与 `end` 节点（S01-1、S01-18）**：S01-1 不产生扣分但会 400 拦截；S01-18 当前不进实战 flows、不生成基础版任务，规则处于“已配置但不执行”状态。若结构不改，S01-18 的规则不会影响任何评分。
5. **写法敏感词**：`Ⅱ级`、`Ⅰ级`、`30 台`、`1 小时`、`10 分钟`、`ACL 降权`、`RPO` 等容易因全半角、空格、罗马数字写法差异漏命中。本轮已将其中大部分降为加分词，必含词只保留写法稳定的中文短语；仍建议后续在引擎侧加一次全半角/空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（52 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S01-1": {
    "requiredKeywords": [
      "勒索信",
      "扩展名",
      "共享盘"
    ],
    "bonusKeywords": [
      "受影响终端",
      "首次发现时间",
      "未重启"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [
      "对象",
      "动作"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "正常",
      "已知悉",
      "已阅",
      "收到",
      "无异常",
      "已确认",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S01-2": {
    "requiredKeywords": [
      "受影响主机",
      "仍在加密",
      "网段"
    ],
    "bonusKeywords": [
      "共享目录访问告警",
      "科室",
      "IP"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "正常",
      "已知悉",
      "已阅",
      "收到",
      "无异常",
      "已确认",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S01-3": {
    "requiredKeywords": [
      "共享目录",
      "隔离",
      "ACL|访问控制列表|访问控制策略"
    ],
    "bonusKeywords": [
      "核心服务器",
      "隔离清单",
      "交换机端口"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "已完成",
      "已隔离",
      "正常",
      "收到",
      "处理完毕",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S01-4": {
    "requiredKeywords": [
      "恢复点",
      "RPO|恢复点目标|恢复时间目标",
      "可读性"
    ],
    "bonusKeywords": [
      "抽验",
      "全量",
      "增量",
      "异地副本"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "正常",
      "已知悉",
      "已阅",
      "收到",
      "无异常",
      "已确认",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S01-5": {
    "requiredKeywords": [
      "定级依据|定级理由|分级依据|判定依据",
      "患者安全",
      "重大"
    ],
    "bonusKeywords": [
      "扩散趋势",
      "数据量",
      "Ⅱ级"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S01-6": {
    "requiredKeywords": [
      "简报",
      "启动",
      "指挥部"
    ],
    "bonusKeywords": [
      "Ⅱ级",
      "已采取措施",
      "10分钟"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
      "动作",
      "时间"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-7": {
    "requiredKeywords": [
      "启动令",
      "授权范围|授权权限|权限范围",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "Ⅱ级",
      "启动时间",
      "停机",
      "数据恢复"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [
      "对象",
      "动作",
      "时间"
    ],
    "fillerWords": [
      "无",
      "已审批",
      "同意",
      "已阅",
      "收到",
      "按要求处理",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-8": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "市网信办|网信办|市委网信办",
      "回执"
    ],
    "bonusKeywords": [
      "1小时",
      "时限依据",
      "不等回复"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
      "动作",
      "时间"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-9": {
    "requiredKeywords": [
      "内存镜像",
      "哈希",
      "日志"
    ],
    "bonusKeywords": [
      "样本",
      "勒索信",
      "SHA-256",
      "易失"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "时间",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已取证",
      "已保全",
      "已完成",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-10": {
    "requiredKeywords": [
      "手工流程",
      "单据号段|单据编号|单据号",
      "回退|回滚|切回|回切"
    ],
    "bonusKeywords": [
      "挂号",
      "收费",
      "发药",
      "检验申请",
      "责任人"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已切换",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-11": {
    "requiredKeywords": [
      "恢复点",
      "RPO|恢复点目标|恢复时间目标",
      "业务用例"
    ],
    "bonusKeywords": [
      "隔离环境",
      "二次加密",
      "RTO",
      "未恢复系统"
    ],
    "minMeasureLen": 45,
    "minResultLen": 28,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-12": {
    "requiredKeywords": [
      "响应级别|响应等级|响应分级",
      "证据保全",
      "回执"
    ],
    "bonusKeywords": [
      "上报时限",
      "联系人",
      "升级"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-13": {
    "requiredKeywords": [
      "舆情",
      "口径",
      "复查"
    ],
    "bonusKeywords": [
      "监测",
      "统一表述",
      "监管要求"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-14": {
    "requiredKeywords": [
      "补录",
      "双人复核|双人核对|两人复核",
      "差异"
    ],
    "bonusKeywords": [
      "封账",
      "单据号",
      "稽核"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "时间",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已补录",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-15": {
    "requiredKeywords": [
      "报案",
      "受理",
      "证据材料"
    ],
    "bonusKeywords": [
      "先行",
      "不等待",
      "书面材料",
      "受理编号"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [
      "对象",
      "时间"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-16": {
    "requiredKeywords": [
      "受理",
      "取证要求",
      "保护现场"
    ],
    "bonusKeywords": [
      "受理编号",
      "非涉案业务",
      "不得重装"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-17": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "二次加密|再次加密|复加密",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "业务恢复",
      "证据固定",
      "复盘",
      "逐条核对"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
      "动作",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已审批",
      "同意",
      "已阅",
      "收到",
      "按要求处理",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S01-18": {
    "requiredKeywords": [
      "时间线",
      "整改",
      "责任人"
    ],
    "bonusKeywords": [
      "根因",
      "复测",
      "时限",
      "得失"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "时间",
      "结论"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-cac-12-1": {
    "requiredKeywords": [
      "单系统",
      "上报标准",
      "时限"
    ],
    "bonusKeywords": [
      "1小时",
      "监管",
      "先行处置"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-cac-12-2": {
    "requiredKeywords": [
      "重装",
      "服务器",
      "证据"
    ],
    "bonusKeywords": [
      "全部服务器",
      "残留后门",
      "恢复时间"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "已完成",
      "已隔离",
      "正常",
      "收到",
      "处理完毕",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-clinical-13-1": {
    "requiredKeywords": [
      "集中补录",
      "晚间",
      "积压"
    ],
    "bonusKeywords": [
      "人力",
      "收费",
      "发药"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已补录",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-clinical-13-2": {
    "requiredKeywords": [
      "急诊",
      "住院",
      "门诊"
    ],
    "bonusKeywords": [
      "批量补录",
      "对平",
      "先急后缓"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已补录",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-commander-16-1": {
    "requiredKeywords": [
      "观察一周",
      "无复发",
      "结束"
    ],
    "bonusKeywords": [
      "二次加密",
      "资源占用"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-commander-16-2": {
    "requiredKeywords": [
      "核心业务",
      "结束",
      "并行恢复"
    ],
    "bonusKeywords": [
      "证据固定",
      "上报闭环",
      "剩余系统"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-commander-6-1": {
    "requiredKeywords": [
      "口头启动",
      "补签",
      "启动令"
    ],
    "bonusKeywords": [
      "避免惊动",
      "小范围",
      "授权"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已审批",
      "同意",
      "已阅",
      "收到",
      "按要求处理",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-commander-6-2": {
    "requiredKeywords": [
      "维持",
      "响应级别",
      "大面积中断"
    ],
    "bonusKeywords": [
      "升级",
      "资源",
      "可控"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-cyberpolice-15-1": {
    "requiredKeywords": [
      "自行恢复",
      "取证",
      "现场"
    ],
    "bonusKeywords": [
      "警方",
      "覆盖",
      "残留日志"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-cyberpolice-15-2": {
    "requiredKeywords": [
      "全面停机",
      "现场",
      "等待"
    ],
    "bonusKeywords": [
      "不得恢复",
      "业务中断",
      "勘验"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS01-datasafe-3-1": {
    "requiredKeywords": [
      "备份日志",
      "成功",
      "可用"
    ],
    "bonusKeywords": [
      "无需验证",
      "任务状态",
      "容量"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "正常",
      "已知悉",
      "已阅",
      "收到",
      "无异常",
      "已确认",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-datasafe-3-2": {
    "requiredKeywords": [
      "厂商",
      "远程",
      "等待"
    ],
    "bonusKeywords": [
      "30分钟",
      "60分钟",
      "恢复可行性"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-duty-0-1": {
    "requiredKeywords": [
      "观察",
      "重复",
      "误报"
    ],
    "bonusKeywords": [
      "10分钟",
      "采集抖动",
      "全员空跑"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-duty-0-2": {
    "requiredKeywords": [
      "科室",
      "确认",
      "汇总"
    ],
    "bonusKeywords": [
      "通讯录",
      "20分钟",
      "误判"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-duty-1-1": {
    "requiredKeywords": [
      "外呼",
      "反馈",
      "汇总"
    ],
    "bonusKeywords": [
      "通讯录",
      "15分钟",
      "科室"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-healthbureau-11-1": {
    "requiredKeywords": [
      "业务",
      "溯源",
      "取证"
    ],
    "bonusKeywords": [
      "患者安全",
      "恢复后",
      "现场"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-healthbureau-11-2": {
    "requiredKeywords": [
      "自行定级",
      "卫健局",
      "研判"
    ],
    "bonusKeywords": [
      "行业口径",
      "偏低",
      "层级"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-itlead-14-1": {
    "requiredKeywords": [
      "自查",
      "来源",
      "报案"
    ],
    "bonusKeywords": [
      "数小时",
      "错过",
      "立案"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-itlead-14-2": {
    "requiredKeywords": [
      "口头报案",
      "书面材料",
      "证据"
    ],
    "bonusKeywords": [
      "受理号",
      "后补",
      "立案"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-itlead-5-1": {
    "requiredKeywords": [
      "内部审议",
      "统一口径",
      "上报"
    ],
    "bonusKeywords": [
      "30分钟",
      "避免追问"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-itlead-5-2": {
    "requiredKeywords": [
      "恢复结果",
      "一并上报",
      "等待"
    ],
    "bonusKeywords": [
      "30分钟",
      "信息齐全"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-itlead-7-1": {
    "requiredKeywords": [
      "市卫生健康局",
      "意见",
      "网信办"
    ],
    "bonusKeywords": [
      "分批上报",
      "1小时",
      "时限"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已知悉",
      "已阅",
      "收到",
      "按要求处理",
      "处理完毕",
      "同上",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 10,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-itlead-7-2": {
    "requiredKeywords": [
      "技术报告",
      "成文",
      "上报"
    ],
    "bonusKeywords": [
      "攻击链",
      "影响面",
      "1小时"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已观察",
      "等待中",
      "已在处理",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 6,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-medlead-8-1": {
    "requiredKeywords": [
      "收费",
      "发药",
      "手工"
    ],
    "bonusKeywords": [
      "门诊医嘱",
      "检验申请",
      "滞留"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已切换",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-medlead-8-2": {
    "requiredKeywords": [
      "局部",
      "手工登记",
      "HIS"
    ],
    "bonusKeywords": [
      "报障科室",
      "住院",
      "检验"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已切换",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-netsec-1-2": {
    "requiredKeywords": [
      "防火墙日志",
      "边界",
      "不扩大"
    ],
    "bonusKeywords": [
      "孤立事件",
      "终端",
      "抽样"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "正常",
      "已知悉",
      "已阅",
      "收到",
      "无异常",
      "已确认",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-netsec-2-1": {
    "requiredKeywords": [
      "隔离",
      "已加密",
      "疑似"
    ],
    "bonusKeywords": [
      "观察",
      "业务连续性",
      "共享"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "已完成",
      "已隔离",
      "正常",
      "收到",
      "处理完毕",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-netsec-9-1": {
    "requiredKeywords": [
      "恢复",
      "取证",
      "证据"
    ],
    "bonusKeywords": [
      "重启",
      "覆盖",
      "内存"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-netsec-9-2": {
    "requiredKeywords": [
      "删除",
      "样本",
      "存储"
    ],
    "bonusKeywords": [
      "归档",
      "哈希",
      "内存镜像"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已取证",
      "已保全",
      "已完成",
      "正常",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS01-sysadmin-10-1": {
    "requiredKeywords": [
      "快照",
      "回滚",
      "完整性"
    ],
    "bonusKeywords": [
      "未校验",
      "生产",
      "损坏"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-sysadmin-10-2": {
    "requiredKeywords": [
      "上线",
      "验证",
      "观察"
    ],
    "bonusKeywords": [
      "边运行",
      "用例",
      "异常"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-sysadmin-2-2": {
    "requiredKeywords": [
      "整盘镜像",
      "隔离",
      "取证"
    ],
    "bonusKeywords": [
      "1-2小时",
      "联网",
      "扩散"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已处理",
      "已完成",
      "已隔离",
      "正常",
      "收到",
      "处理完毕",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.25
  },
  "DSS01-sysadmin-4-1": {
    "requiredKeywords": [
      "阈值",
      "加密主机",
      "快速定级"
    ],
    "bonusKeywords": [
      "30台",
      "Ⅱ级",
      "数量"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS01-sysadmin-4-2": {
    "requiredKeywords": [
      "过度升级",
      "定级",
      "追责"
    ],
    "bonusKeywords": [
      "先高后低",
      "Ⅰ级",
      "资源"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
    "requiredElements": [
      "对象"
    ],
    "fillerWords": [
      "无",
      "已研判",
      "已定级",
      "按预案",
      "正常",
      "已知悉",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  }
}
```
