# S05 处置文字质量规则清单（45 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S05 全部 45 个节点（correct 17 / distractor 28），剧本「核心存储故障系统宕机」
> 依据：现网节点 `title/descr/guide/keywords/evidence_req/form/score/timeout/target/receiver`，表单模板 F1–F17，评分口径 `server/internal/handler/score_config.go`，分档与原型见 `docs/quality-rules-framework.md`，格式与口径对齐 `docs/quality-rules-s01.md`
> **本轮只出文档：未执行 UPDATE/INSERT，未推仓库，未改动现网容器与数据库。字段值可直接用于后续批量落库审阅。**

---

## 0. 先明确规则是怎么被引擎消费的

这决定规则不能凭感觉写，以下 8 条是本次设计的硬约束：

1. **九个字段全部生效于处置文本**：引擎只拼接 `measure`（处置措施）、`result`（处置结果）、`found`（发现情况）、`evidence`（证据说明）四段文本；**表单填写值不参与关键词匹配**。所以必含词必须是参演人在叙述里自然会写的词，不能是只在下拉框里出现的选项。
2. **关键词是字面子串匹配**：`strings.Contains`，区分大小写，不处理同义词、不处理空格差异。带空格/罗马数字/大小写混排的词只能放加分词，不放必含词（如“10 分钟”“30 台”“Ⅱ级”“ACL 降权”）。
3. **必含词扣分按缺项比例折算**：缺 1 项 = 节点分 × `keywordPenaltyRatio` × 1/必含总数；加分词每命中 1 个可抵扣 1 项。本册统一取 3 项，分别对应处置对象、动作与判据，避免靠同义词堆叠凑数。
4. **长度按字符（rune）计**：`minMeasureLen` 只管“处置措施”，`minResultLen` 只管“处置结果”，不合并计算。
5. **要素只有四种**：`对象 / 动作 / 时间 / 结论`。引擎用正则判定，四类并非每类节点都适用：`结论`只给有可验证结果的节点，`时间`只给时间本身是验收条件的节点，外部指导/受理记录只要求`对象`。
6. **敷衍判定是整段判定**：去掉空白与标点后，若整段长度 ≤ `fillerMaxLen` 且能完全由 `fillerWords` 拼出来，才判为敷衍并返回 400。它不会因为正文里出现“已完成”就误伤长文本。
7. **所有节点都过文字质量闸门**：`drill_v1.go:470-476` 在 `isCorrect` 判定之前执行，所以**混淆节点与 0 分节点的规则同样会拦住提交**；只是扣分部分只对正确节点生效。这是混淆规则必须“可命中、可提交”的原因。
8. **单节点扣分合计上限 80%**：关键词、文字质量、证据、超时、接收人五项合计不超过节点分 80%。本清单的比例是在这个上限内分配的。

### 0.1 长度分档（本次实际使用的八档）

| 档 | 措施/结果 | 本册示例节点 |
|---|---|---|
| T1 轻量 | 15/10 | DSS05-clinical-13-1、DSS05-commander-14-1、DSS05-duty-1-2、DSS05-itlead-10-1 |
| T2 标准 | 20/12 | DSS05-clinical-13-2、DSS05-commander-14-2、DSS05-commander-5-1、DSS05-commander-5-2 |
| T3 记录 | 30/18 | S05-1、S05-10、S05-12、S05-16 |
| T4 关键 | 35/20 | S05-11、S05-14、S05-3、S05-4 |
| T5 复杂 | 40/25 | S05-13、S05-17、S05-2、S05-8 |
| T6 高复杂 | 45/28 | S05-15 |
| T7 取证 | 50/30 | — |

> 分档依据不是分值本身，而是该节点在验收上需要写清几个对象/判据：一个动作一个对象 → T1/T2；动作+对象+结论 → T3；多对象+顺序+例外 → T5/T6；证据链+时间 → T7；时间线+责任+整改 → T8。

### 0.2 比例档

| 节点档 | `textQualityRatio` | `keywordPenaltyRatio` | 说明 |
|---|---|---|---|
| 正确节点 8 分 | 0.25 | 0.30 | 关键处置，关键词是硬验收 |
| 正确节点 7 分 | 0.20 | 0.30 | 关键处置，关键词是硬验收 |
| 正确节点 6 分 / 0 分 | 0.20 | 0.25（0 分节点 0.30） | 常规节点，质量与关键词并重 |
| 正确节点 5 分 | 0.15 | 0.20 | 轻量记录类，从宽 |
| 混淆 8 分 | 0.15 | 0.25 | 选中已扣满分值，质量只作描述完整性约束 |
| 混淆 5 分 | 0.15 | 0.20 | 同上，取更轻档 |
| 混淆 3 分 | 0.10 | 0.15 | 最低档，避免双重惩罚 |

### 0.3 敷衍词档（按节点类型，不是一张表套全部）

| 档 | 名称 | 词表 | 上限 | 用于 |
|---|---|---|---|---|
| P1 | 核实 | 无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK | 8 | 核实 |
| P2 | 处置 | 无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK | 8 | 处置 |
| P3 | 研判 | 无、已研判、已定级、按预案、正常、已知悉、ok、OK | 8 | 研判 |
| P4 | 上报/指导记录 | 无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK | 10 | 报告、指导、报案、复盘、舆情 |
| P5 | 取证 | 无、已取证、已保全、已完成、正常、ok、OK | 8 | 取证 |
| P6 | 恢复 | 无、已恢复、已处理、正常、已完成、ok、OK | 8 | 恢复 |
| P7 | 观望/延误 | 无、已观察、等待中、已在处理、正常、ok、OK | 6 | 观望/延误 |
| P8 | 审批/结束 | 无、已审批、同意、已阅、收到、按要求处理、ok、OK | 8 | 审批/结束 |
| P9 | 补录 | 无、已补录、已处理、正常、已完成、ok、OK | 8 | 补录 |
| P10 | 兜底切换 | 无、已切换、已处理、正常、已完成、ok、OK | 8 | 兜底切换 |

> 报告/指导/报案档（P4）刻意不含“已上报、已报送、已报案”，因为对这类节点而言那是必须完成的动作，不是敷衍；但仍保留“已知悉、已阅、收到”等无实质内容的词。

---

## 1. 正确节点规则（17 个）

### S05-1 · 核心存储链路中断告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`存储链路`、`核实`、`控制器`
- `bonusKeywords`：`双链路`、`接口响应时间`、`磁盘故障`、`受影响清单`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报要写清哪条链路中断、是否伴随控制器或磁盘故障、业务接口是否变慢，故取存储链路/核实/控制器；该节点 0 分，规则只作提交闸门。

### S05-2 · 核实存储与业务影响范围

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`可用性`、`核实`、`只读`
- `bonusKeywords`：`受影响科室`、`窗口数量`、`不可用`、`逐项`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实的关键是逐系统给出可用/只读/不可用三种状态并统计科室与窗口数，故取可用性/核实/只读；加分词奖励窗口数量与不可用清单。

### S05-3 · 预警研判：确认重大（Ⅱ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`研判`、`定级依据|定级理由|分级依据|判定依据`、`纸质兜底`
- `bonusKeywords`：`重大`、`全不可用`、`受影响科室`、`扩散`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级必须落到两项判据——HIS 是否全不可用、是否需全面纸质兜底，故取研判/定级依据/纸质兜底；定级动作不在引擎动作词表，要素不用动作。

### S05-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`受影响系统`、`恢复窗口`、`启动`
- `bonusKeywords`：`业务中断`、`建议`、`指挥部`、`等级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：院内报告要同时给受影响系统清单与预计恢复窗口，并明确提出启动请求，故取受影响系统/恢复窗口/启动；不要求结论要素以免把建议写成已完成。

### S05-5 · 院应急指挥部批准启动Ⅱ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`厂商授权`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`兜底范围`、`对外口径`、`备案`、`启动时间`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：审批的可核查产物是启动令、厂商授权与响应级别三件，故取此三词；加分词奖励兜底覆盖范围与对外沟通口径。

### S05-6 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`初报`、`业务中断`
- `bonusKeywords`：`1小时`、`兜底措施`、`处置进展`、`上报表`、`同时报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报卫健的写法必须落到接收单位、初报动作与业务中断范围，故取市卫生健康局/初报/业务中断；1小时时限只放加分词，规避数字与空格写法差异；“同时报”属报送质量表现，放加分词。

### S05-7 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`上报`、`网络攻击`
- `bonusKeywords`：`故障性质`、`已采取措施`、`处置进展`、`同步`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报网信的核心是说明故障性质与是否排除网络攻击，故取市网信办/上报/网络攻击；加分词奖励已采取措施与处置进展这两段内容。

### S05-8 · 业务兜底：六大环节全面启用手工流程

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 25 min

- `requiredKeywords`：`手工流程`、`六大环节|六个环节|六环节|挂号`、`单据编号`
- `bonusKeywords`：`切换时间`、`责任人`、`挂号`、`回退`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、结论
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：兜底要写清六大环节全部切手工、单据编号规则与各环节责任人，故取手工流程/六大环节/单据编号；该节点验收动作是业务兜底，要素不要求动作。

### S05-9 · 切换备用存储或降级运行

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`备用存储`、`同步状态`、`切换`
- `bonusKeywords`：`容量`、`回退条件`、`切换时间`、`先取证`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：切换前必须先确认备用存储容量与数据同步状态，否则会造成数据缺口，故取备用存储/同步状态/切换；加分词奖励回退条件与先取证后切换的顺序。

### S05-10 · 联动存储厂商远程或现场支持

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 6 ｜ 超时 10 min

- `requiredKeywords`：`厂商`、`工单号`、`进展`
- `bonusKeywords`：`远程接入`、`到场时间`、`30分钟`、`处置建议`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：厂商协同的记录对象是工单与反馈节奏，不是技术动作，故取厂商/工单号/进展；30分钟反馈进展只放加分词，要素只需对象。

### S05-11 · 数据完整性校验

`correct` ｜ 类型 `task` ｜ 表单 `F7` ｜ 分值 7 ｜ 超时 25 min

- `requiredKeywords`：`总量比对|总量核对|总量校验`、`抽样校验|抽样核对|抽样检查`、`差异`
- `bonusKeywords`：`记录数`、`金额`、`清零`、`近期数据`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：完整性不能靠主观判断，必须按记录数与金额做总量比对并抽样，最终以差异清零收口，故取总量比对/抽样校验/差异。

### S05-12 · 向市网安大队报案并提交材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`证据清单`
- `bonusKeywords`：`不等待`、`受理编号`、`事件经过`、`先行`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报案要先行、提交已保全的证据清单并保留受理编号，故取市网安大队/报案/证据清单；报案动作不在引擎动作词表，要素用对象加时间。

### S05-13 · 数据补录：六大环节手工单补录与稽核

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 30 min

- `requiredKeywords`：`补录`、`稽核`、`总量`
- `bonusKeywords`：`补录条数`、`差异条数`、`稽核人`、`停机前`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、结论
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：补录要覆盖六大环节手工单、逐项稽核，并以补录后总量与停机前一致作为收口，故取补录/稽核/总量。

### S05-14 · 应急结束：院应急指挥部宣布Ⅱ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`校验`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`无回退`、`补录完成`、`上报`、`宣布`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束必须逐条核对四项条件——连续无回退、完整性校验通过、补录完成、上报与报案闭环，故取结束条件/校验/闭环；外包给单句结束会被拦。

### S05-15 · 证据保全：存储控制器日志与厂商工单封存

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`控制器日志`、`封存`、`哈希`
- `bonusKeywords`：`双链路告警`、`保全时间`、`存放位置`、`可追溯`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：取证三件套是控制器日志、双链路告警与厂商工单，并以哈希与保全时间保证可追溯，故取控制器日志/封存/哈希；取证含时间要素用于证明封存在切换之前。

### S05-16 · 市网安大队受理回执与取证要求确认

`correct` ｜ 类型 `report` ｜ 表单 `无` ｜ 分值 5 ｜ 超时 15 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`取证要求`、`配合时限`
- `bonusKeywords`：`受理编号`、`取证边界`、`只读`、`代提交`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该节点只记录外部回执与要求，要素仅需对象；三个词锁定受理回执、取证要求与配合时限，避免把外部意见写成院内处置过程。

### S05-17 · 应急结束与复盘：Ⅱ级响应终止

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 30 min

- `requiredKeywords`：`复盘`、`时间线`、`整改项`
- `bonusKeywords`：`责任人`、`时限`、`总结报告`、`主管部门`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘要还原时间线、列整改项并明确责任人与时限，故取复盘/时间线/整改项；该节点兼结束与复盘，长度用 T6 不硬套 T8。

---

## 2. 混淆节点规则（28 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS05-clinical-13-1 · 让挂号收费集中时段补录

`distractor` ｜ 分值 5 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`补录`、`高峰时段`、`积压`
- `bonusKeywords`：`挂号收费`、`人力`、`效率`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是把补录压到挂号收费高峰时段，写法自然出现补录、高峰时段与积压；5 分混淆取轻量档，保证按提示可提交。

### DSS05-clinical-13-2 · 只补录大额单据

`distractor` ｜ 分值 8 ｜ 超时 30 min ｜ 表单 `无`

- `requiredKeywords`：`大额单据`、`小额`、`补录`
- `bonusKeywords`：`病历记录`、`优先`、`缺失`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P9 补录档，`fillerMaxLen` 8）：无、已补录、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只补大额单据的写法必然带大额单据、小额与补录三个词；8 分混淆取 20/12，区分度交给必含词而非长度。

### DSS05-commander-14-1 · 等业务恢复一周后再宣布

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`业务恢复`、`一周`、`结束`
- `bonusKeywords`：`稳定`、`观察`、`延迟`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：等一周再宣布的写法会写业务恢复、观察一周与结束；3 分混淆取 T1，要素只要对象避免动作正则卡提交。

### DSS05-commander-14-2 · 业务恢复了就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`业务恢复`、`结束`、`宣布`
- `bonusKeywords`：`校验通过`、`补录完成`、`上报闭环`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：业务恢复即结束的写法核心是业务恢复、宣布与结束；8 分混淆取 T2，被它省略的校验、补录与闭环只放加分词。

### DSS05-commander-5-1 · 让信息科先自己搞，指挥部不介入

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`信息科`、`自行处置`、`指挥部`
- `bonusKeywords`：`授权`、`医务处`、`资源调度`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：让信息科自行处置、指挥部不介入的写法会带信息科、自行处置、指挥部；8 分混淆取 T2，缺授权与资源调度放加分词。

### DSS05-commander-5-2 · 等 HIS 真的全停了再启动

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`完全停止`、`启动`、`兜底`
- `bonusKeywords`：`HIS`、`阈值`、`滞后`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等系统完全停止再启动的写法会写完全停止、启动与兜底；8 分混淆取 T2，系统英文缩写只放加分词规避大小写差异。

### DSS05-duty-1-1 · 先重启存储控制器看能不能恢复

`distractor` ｜ 分值 8 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`重启`、`存储控制器`、`恢复`
- `bonusKeywords`：`缓存`、`锁死`、`链路`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先重启控制器看能否恢复的写法会带重启、存储控制器、恢复；8 分混淆取 T2，不把缓存丢失这类后果词设必含。

### DSS05-duty-1-2 · 看到自动切换就报恢复，不再核对存储

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`自动切换`、`恢复`、`同步`
- `bonusKeywords`：`控制器日志`、`静默故障`、`双链路`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：以自动切换即判恢复的写法会写自动切换、恢复、同步；5 分混淆取 T1，双链路同步与控制器日志放加分词。

### DSS05-itlead-10-1 · 先试试自己修，修不好再叫厂商

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`自行维修`、`厂商`、`存储设备`
- `bonusKeywords`：`专业设备`、`扩大故障`、`再联系`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先自行维修、修不好再叫厂商的写法会带自行维修、厂商、存储设备；5 分混淆取 T1，只要求写出错误动作本身。

### DSS05-itlead-10-2 · 厂商收费贵，能远程就别让现场

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`远程支持`、`厂商`、`省钱`
- `bonusKeywords`：`现场`、`部件更换`、`延误`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只接受远程支持的写法会写远程支持、厂商与省钱；3 分混淆取 T1，厂商进场与部件更换延误放加分词。

### DSS05-itlead-12-1 · 不报，内部处理

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`内部处理`、`报案`、`网安大队`
- `bonusKeywords`：`技术问题`、`自行消化`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：不报案只内部处理的写法会写内部处理、报案与网安大队；3 分混淆取 T1，P4 档刻意不含已报案，避免把否定叙述误判为敷衍。

### DSS05-itlead-12-2 · 不保全任何证据就直接报案

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`报案`、`人为破坏`、`证据链`
- `bonusKeywords`：`日志`、`镜像`、`立案`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：怀疑人为破坏即报案但不保全的写法会带报案、人为破坏、证据链；5 分混淆取 T1，日志与镜像放加分词。

### DSS05-itlead-3-1 · 只看设备类型定级，不看业务中断范围

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`核心存储`、`定级`、`中断范围`
- `bonusKeywords`：`设备类型`、`兜底需求`、`依据不足`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：仅凭核心存储故障定级的写法会写核心存储、定级与中断范围；5 分混淆取 T1，罗马数字等级只放加分词。

### DSS05-itlead-3-2 · 等存储恢复了再说影响多大

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`存储恢复`、`评估影响`、`定级`
- `bonusKeywords`：`延迟`、`兜底`、`上报`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等存储恢复再评估影响的写法会带存储恢复、评估影响、定级；8 分混淆取 T2，延误导致的兜底与上报滞后放加分词。

### DSS05-itlead-4-1 · 等存储恢复方案出来再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`恢复方案`、`报告`、`指挥部`
- `bonusKeywords`：`启动窗口`、`延误`、`完整`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等恢复方案再报告的写法会写恢复方案、报告、指挥部；8 分混淆取 T2，不把报告内容写成已启动响应。

### DSS05-itlead-4-2 · 先报"存储告警"不说是链路中断

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`存储告警`、`链路中断`、`业务影响`
- `bonusKeywords`：`模糊`、`后续补齐`、`决策`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只报存储告警的写法会带存储告警、链路中断与业务影响三个词；5 分混淆取 T1，否定句式同样能命中。

### DSS05-itlead-6-1 · 等厂商给方案了再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`厂商方案`、`市卫生健康局`、`上报`
- `bonusKeywords`：`1小时`、`时限`、`完整`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等厂商方案再报卫健的写法会写厂商方案、市卫生健康局、上报；8 分混淆取 T2，1小时时限只放加分词。

### DSS05-itlead-6-2 · 先报"存储告警"

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`存储告警`、`先报`、`补齐`
- `bonusKeywords`：`信息不足`、`卫健`、`后续`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：先报存储告警后补齐的写法会带存储告警、先报、补齐；3 分混淆取 T1，信息不足的后果放加分词。

### DSS05-itlead-7-1 · 等卫健局那边意见出来了再同步报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市卫健局`、`意见`、`市网信办`
- `bonusKeywords`：`同步报`、`延误`、`一起报`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等卫健局意见再同步报网信的写法会写市卫健局、意见、市网信办；8 分混淆取 T2，两个接收单位是区分它与 S05-7 的关键。

### DSS05-itlead-7-2 · 只发初报，处置进展不再续报

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`初报`、`续报`、`处置进展`
- `bonusKeywords`：`闭环`、`接收单位`、`只发`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只发初报不续报的写法会带初报、续报与处置进展；5 分混淆取 T1，闭环情况与接收单位放加分词。

### DSS05-medlead-8-1 · 先只切核心收费环节

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`收费环节`、`手工`、`兜底`
- `bonusKeywords`：`挂号`、`住院`、`检验`、`药房`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只切收费环节的写法会写收费环节、手工与兜底；5 分混淆取 T1，未兜底的挂号住院检验药房放加分词。

### DSS05-medlead-8-2 · 继续等存储自己恢复

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`等待`、`自行恢复`、`门诊`
- `bonusKeywords`：`住院`、`数据链`、`断裂`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：继续等存储自愈的写法会带等待、自行恢复、门诊；8 分混淆取 T2，住院与数据链断裂风险放加分词。

### DSS05-sysadmin-11-1 · 让业务科室自己核对数据对不对

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`业务科室`、`自行核对`、`数据`
- `bonusKeywords`：`记录数`、`金额`、`总量比对`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：让业务科室自行核对的写法会写业务科室、自行核对、数据；5 分混淆取 T1，被省掉的总量比对与金额核对放加分词。

### DSS05-sysadmin-11-2 · 直接上线跑，有问题再回滚

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`上线`、`校验`、`回滚`
- `bonusKeywords`：`漏费`、`病历缺页`、`观察`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先上线有问题再回滚的写法会带上线、校验、回滚；8 分混淆取 T2，漏费与病历缺页作为后果放加分词。

### DSS05-sysadmin-2-1 · 只看光纤链路灯，不查业务与日志

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`指示灯`、`业务可用性`、`日志`
- `bonusKeywords`：`光纤`、`影响范围`、`硬件`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只看指示灯的写法会写指示灯、业务可用性与日志；5 分混淆取 T1，光纤与真实影响范围放加分词。

### DSS05-sysadmin-2-2 · 等厂商工程师到场再核实

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`厂商工程师`、`到场`、`核实`
- `bonusKeywords`：`等待`、`业务中断`、`外部`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等厂商到场再核实的写法会带厂商工程师、到场、核实；8 分混淆取 T2，核实被外部卡住的后果放加分词。

### DSS05-sysadmin-9-1 · 只切数据库，不验证同步与切换点

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`数据库实例`、`切换`、`同步状态`
- `bonusKeywords`：`切换时间点`、`数据缺口`、`未验证`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：直接切数据库的写法会写数据库实例、切换、同步状态；8 分混淆取 T2，切换时间点与数据缺口放加分词。

### DSS05-sysadmin-9-2 · 直接重装 HIS 应用

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`重装`、`应用`、`耗时`
- `bonusKeywords`：`数据缺口`、`扩大`、`彻底`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：直接重装应用的写法会带重装、应用、耗时；8 分混淆取 T2，可能扩大的数据缺口只放加分词。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S05-1 | 正确 | — | 0 | 存储链路、核实、控制器 | 双链路、接口响应时间、磁盘故障、受影响清单 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S05-2 | 正确 | F12 | 8 | 可用性、核实、只读 | 受影响科室、窗口数量、不可用、逐项 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S05-3 | 正确 | F3 | 8 | 研判、定级依据\|定级理由\|分级依据\|判定依据、纸质兜底 | 重大、全不可用、受影响科室、扩散 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S05-4 | 正确 | F4 | 7 | 受影响系统、恢复窗口、启动 | 业务中断、建议、指挥部、等级 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S05-5 | 正确 | F9 | 7 | 启动令、厂商授权、响应级别\|响应等级\|响应分级 | 兜底范围、对外口径、备案、启动时间 | 30/18 | 对象、动作、时间 | P8 | 0.20/0.30 |
| S05-6 | 正确 | F2 | 6 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、初报、业务中断 | 1小时、兜底措施、处置进展、上报表、同时报 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S05-7 | 正确 | F2 | 6 | 市网信办\|网信办\|市委网信办、上报、网络攻击 | 故障性质、已采取措施、处置进展、同步 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S05-8 | 正确 | F9 | 7 | 手工流程、六大环节\|六个环节\|六环节\|挂号、单据编号 | 切换时间、责任人、挂号、回退 | 40/25 | 对象、结论 | P10 | 0.20/0.30 |
| S05-9 | 正确 | F9 | 7 | 备用存储、同步状态、切换 | 容量、回退条件、切换时间、先取证 | 35/20 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S05-10 | 正确 | F9 | 6 | 厂商、工单号、进展 | 远程接入、到场时间、30分钟、处置建议 | 30/18 | 无 | P4 | 0.20/0.25 |
| S05-11 | 正确 | F7 | 7 | 总量比对\|总量核对\|总量校验、抽样校验\|抽样核对\|抽样检查、差异 | 记录数、金额、清零、近期数据 | 35/20 | 对象、结论 | P6 | 0.20/0.30 |
| S05-12 | 正确 | F5 | 6 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、证据清单 | 不等待、受理编号、事件经过、先行 | 30/18 | 对象、时间 | P4 | 0.20/0.25 |
| S05-13 | 正确 | F9 | 7 | 补录、稽核、总量 | 补录条数、差异条数、稽核人、停机前 | 40/25 | 对象、结论 | P9 | 0.20/0.30 |
| S05-14 | 正确 | F8 | 6 | 结束条件\|终止条件\|结束标准\|终止标准、校验、闭环\|闭环管理\|办结\|归档 | 无回退、补录完成、上报、宣布 | 35/20 | 对象、动作、结论 | P8 | 0.20/0.25 |
| S05-15 | 正确 | F13 | 7 | 控制器日志、封存、哈希 | 双链路告警、保全时间、存放位置、可追溯 | 45/28 | 对象、时间、结论 | P5 | 0.20/0.30 |
| S05-16 | 正确 | — | 5 | 受理回执\|受案回执\|回执单\|回执、取证要求、配合时限 | 受理编号、取证边界、只读、代提交 | 30/18 | 无 | P4 | 0.15/0.20 |
| S05-17 | 正确 | — | 6 | 复盘、时间线、整改项 | 责任人、时限、总结报告、主管部门 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS05-clinical-13-1 | 混淆 | — | 5 | 补录、高峰时段、积压 | 挂号收费、人力、效率 | 15/10 | 对象 | P9 | 0.15/0.20 |
| DSS05-clinical-13-2 | 混淆 | — | 8 | 大额单据、小额、补录 | 病历记录、优先、缺失 | 20/12 | 对象 | P9 | 0.15/0.25 |
| DSS05-commander-14-1 | 混淆 | — | 3 | 业务恢复、一周、结束 | 稳定、观察、延迟 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS05-commander-14-2 | 混淆 | — | 8 | 业务恢复、结束、宣布 | 校验通过、补录完成、上报闭环 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS05-commander-5-1 | 混淆 | — | 8 | 信息科、自行处置、指挥部 | 授权、医务处、资源调度 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS05-commander-5-2 | 混淆 | — | 8 | 完全停止、启动、兜底 | HIS、阈值、滞后 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-duty-1-1 | 混淆 | — | 8 | 重启、存储控制器、恢复 | 缓存、锁死、链路 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS05-duty-1-2 | 混淆 | — | 5 | 自动切换、恢复、同步 | 控制器日志、静默故障、双链路 | 15/10 | 对象 | P6 | 0.15/0.20 |
| DSS05-itlead-10-1 | 混淆 | — | 5 | 自行维修、厂商、存储设备 | 专业设备、扩大故障、再联系 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS05-itlead-10-2 | 混淆 | — | 3 | 远程支持、厂商、省钱 | 现场、部件更换、延误 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS05-itlead-12-1 | 混淆 | — | 3 | 内部处理、报案、网安大队 | 技术问题、自行消化 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS05-itlead-12-2 | 混淆 | — | 5 | 报案、人为破坏、证据链 | 日志、镜像、立案 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS05-itlead-3-1 | 混淆 | — | 5 | 核心存储、定级、中断范围 | 设备类型、兜底需求、依据不足 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS05-itlead-3-2 | 混淆 | — | 8 | 存储恢复、评估影响、定级 | 延迟、兜底、上报 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-itlead-4-1 | 混淆 | — | 8 | 恢复方案、报告、指挥部 | 启动窗口、延误、完整 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS05-itlead-4-2 | 混淆 | — | 5 | 存储告警、链路中断、业务影响 | 模糊、后续补齐、决策 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS05-itlead-6-1 | 混淆 | — | 8 | 厂商方案、市卫生健康局、上报 | 1小时、时限、完整 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-itlead-6-2 | 混淆 | — | 3 | 存储告警、先报、补齐 | 信息不足、卫健、后续 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS05-itlead-7-1 | 混淆 | — | 8 | 市卫健局、意见、市网信办 | 同步报、延误、一起报 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-itlead-7-2 | 混淆 | — | 5 | 初报、续报、处置进展 | 闭环、接收单位、只发 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS05-medlead-8-1 | 混淆 | — | 5 | 收费环节、手工、兜底 | 挂号、住院、检验、药房 | 15/10 | 对象 | P10 | 0.15/0.20 |
| DSS05-medlead-8-2 | 混淆 | — | 8 | 等待、自行恢复、门诊 | 住院、数据链、断裂 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-sysadmin-11-1 | 混淆 | — | 5 | 业务科室、自行核对、数据 | 记录数、金额、总量比对 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS05-sysadmin-11-2 | 混淆 | — | 8 | 上线、校验、回滚 | 漏费、病历缺页、观察 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS05-sysadmin-2-1 | 混淆 | — | 5 | 指示灯、业务可用性、日志 | 光纤、影响范围、硬件 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS05-sysadmin-2-2 | 混淆 | — | 8 | 厂商工程师、到场、核实 | 等待、业务中断、外部 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS05-sysadmin-9-1 | 混淆 | — | 8 | 数据库实例、切换、同步状态 | 切换时间点、数据缺口、未验证 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS05-sysadmin-9-2 | 混淆 | — | 8 | 重装、应用、耗时 | 数据缺口、扩大、彻底 | 20/12 | 对象 | P2 | 0.15/0.25 |

---

## 4. 总量、分布与风险

### 4.1 S05 规则总量

- 节点 45：正确 17、混淆 28；规则条数 45，与节点一一对应，无遗漏无多余。
- 必含词合计 135 个，平均 3.0 个/节点；加分词合计 152 个。
- 长度档：15/10 共 14 个；20/12 共 14 个；30/18 共 7 个；35/20 共 5 个；40/25 共 4 个；45/28 共 1 个。
- 要素组合：对象 30 个；对象+动作 1 个；对象+结论 4 个；对象+时间 1 个；对象+动作+结论 3 个；对象+时间+结论 2 个；对象+动作+时间 4 个。
- 表单绑定（正确节点）：F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F7 1 个、F8 1 个、F9 5 个、无表单 3 个。

### 4.2 风险最高的几类节点

1. **混淆节点（28 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S05-2`（核实影响范围，8 分）：必含 `只读`。参演人只写“存储全部不可用”，漏掉“只读/不可写/离线”三种状态中的“只读”，会缺一项。
3. `S05-3`（研判定级，8 分）：必含 `定级依据` 与 `纸质兜底` 同时成立才像样，实际提交常只写“定为重大事件”，两项都缺，是本册 8 分节点里最容易失分的一个。
4. `S05-8`（六大环节兜底，7 分）：必含词落在环节与单据上，参演人若只写“已切换手工流程”而不列环节，长度和关键词两道闸门都会拦。
5. `S05-16`（受理回执，5 分）：无表单、只记录对方意见，要素仅 `对象`，长度按 T3 30/18 取（与 S01 外部受理节点一致），不要按 5 分档压到 15/10，否则记录类文本容易被判过短。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（45 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S05-1": {
    "requiredKeywords": [
      "存储链路",
      "核实",
      "控制器"
    ],
    "bonusKeywords": [
      "双链路",
      "接口响应时间",
      "磁盘故障",
      "受影响清单"
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
  "S05-2": {
    "requiredKeywords": [
      "可用性",
      "核实",
      "只读"
    ],
    "bonusKeywords": [
      "受影响科室",
      "窗口数量",
      "不可用",
      "逐项"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S05-3": {
    "requiredKeywords": [
      "研判",
      "定级依据|定级理由|分级依据|判定依据",
      "纸质兜底"
    ],
    "bonusKeywords": [
      "重大",
      "全不可用",
      "受影响科室",
      "扩散"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S05-4": {
    "requiredKeywords": [
      "受影响系统",
      "恢复窗口",
      "启动"
    ],
    "bonusKeywords": [
      "业务中断",
      "建议",
      "指挥部",
      "等级"
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-5": {
    "requiredKeywords": [
      "启动令",
      "厂商授权",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "兜底范围",
      "对外口径",
      "备案",
      "启动时间"
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-6": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "初报",
      "业务中断"
    ],
    "bonusKeywords": [
      "1小时",
      "兜底措施",
      "处置进展",
      "上报表",
      "同时报"
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
  "S05-7": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "上报",
      "网络攻击"
    ],
    "bonusKeywords": [
      "故障性质",
      "已采取措施",
      "处置进展",
      "同步"
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
  "S05-8": {
    "requiredKeywords": [
      "手工流程",
      "六大环节|六个环节|六环节|挂号",
      "单据编号"
    ],
    "bonusKeywords": [
      "切换时间",
      "责任人",
      "挂号",
      "回退"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-9": {
    "requiredKeywords": [
      "备用存储",
      "同步状态",
      "切换"
    ],
    "bonusKeywords": [
      "容量",
      "回退条件",
      "切换时间",
      "先取证"
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
  "S05-10": {
    "requiredKeywords": [
      "厂商",
      "工单号",
      "进展"
    ],
    "bonusKeywords": [
      "远程接入",
      "到场时间",
      "30分钟",
      "处置建议"
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
  "S05-11": {
    "requiredKeywords": [
      "总量比对|总量核对|总量校验",
      "抽样校验|抽样核对|抽样检查",
      "差异"
    ],
    "bonusKeywords": [
      "记录数",
      "金额",
      "清零",
      "近期数据"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-12": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "证据清单"
    ],
    "bonusKeywords": [
      "不等待",
      "受理编号",
      "事件经过",
      "先行"
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
  "S05-13": {
    "requiredKeywords": [
      "补录",
      "稽核",
      "总量"
    ],
    "bonusKeywords": [
      "补录条数",
      "差异条数",
      "稽核人",
      "停机前"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-14": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "校验",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "无回退",
      "补录完成",
      "上报",
      "宣布"
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
  "S05-15": {
    "requiredKeywords": [
      "控制器日志",
      "封存",
      "哈希"
    ],
    "bonusKeywords": [
      "双链路告警",
      "保全时间",
      "存放位置",
      "可追溯"
    ],
    "minMeasureLen": 45,
    "minResultLen": 28,
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
    "keywordPenaltyRatio": 0.3
  },
  "S05-16": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "取证要求",
      "配合时限"
    ],
    "bonusKeywords": [
      "受理编号",
      "取证边界",
      "只读",
      "代提交"
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
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "S05-17": {
    "requiredKeywords": [
      "复盘",
      "时间线",
      "整改项"
    ],
    "bonusKeywords": [
      "责任人",
      "时限",
      "总结报告",
      "主管部门"
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
  "DSS05-clinical-13-1": {
    "requiredKeywords": [
      "补录",
      "高峰时段",
      "积压"
    ],
    "bonusKeywords": [
      "挂号收费",
      "人力",
      "效率"
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
  "DSS05-clinical-13-2": {
    "requiredKeywords": [
      "大额单据",
      "小额",
      "补录"
    ],
    "bonusKeywords": [
      "病历记录",
      "优先",
      "缺失"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
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
    "keywordPenaltyRatio": 0.25
  },
  "DSS05-commander-14-1": {
    "requiredKeywords": [
      "业务恢复",
      "一周",
      "结束"
    ],
    "bonusKeywords": [
      "稳定",
      "观察",
      "延迟"
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
  "DSS05-commander-14-2": {
    "requiredKeywords": [
      "业务恢复",
      "结束",
      "宣布"
    ],
    "bonusKeywords": [
      "校验通过",
      "补录完成",
      "上报闭环"
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
  "DSS05-commander-5-1": {
    "requiredKeywords": [
      "信息科",
      "自行处置",
      "指挥部"
    ],
    "bonusKeywords": [
      "授权",
      "医务处",
      "资源调度"
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
  "DSS05-commander-5-2": {
    "requiredKeywords": [
      "完全停止",
      "启动",
      "兜底"
    ],
    "bonusKeywords": [
      "HIS",
      "阈值",
      "滞后"
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
  "DSS05-duty-1-1": {
    "requiredKeywords": [
      "重启",
      "存储控制器",
      "恢复"
    ],
    "bonusKeywords": [
      "缓存",
      "锁死",
      "链路"
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
  "DSS05-duty-1-2": {
    "requiredKeywords": [
      "自动切换",
      "恢复",
      "同步"
    ],
    "bonusKeywords": [
      "控制器日志",
      "静默故障",
      "双链路"
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
  "DSS05-itlead-10-1": {
    "requiredKeywords": [
      "自行维修",
      "厂商",
      "存储设备"
    ],
    "bonusKeywords": [
      "专业设备",
      "扩大故障",
      "再联系"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-itlead-10-2": {
    "requiredKeywords": [
      "远程支持",
      "厂商",
      "省钱"
    ],
    "bonusKeywords": [
      "现场",
      "部件更换",
      "延误"
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
  "DSS05-itlead-12-1": {
    "requiredKeywords": [
      "内部处理",
      "报案",
      "网安大队"
    ],
    "bonusKeywords": [
      "技术问题",
      "自行消化"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS05-itlead-12-2": {
    "requiredKeywords": [
      "报案",
      "人为破坏",
      "证据链"
    ],
    "bonusKeywords": [
      "日志",
      "镜像",
      "立案"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-itlead-3-1": {
    "requiredKeywords": [
      "核心存储",
      "定级",
      "中断范围"
    ],
    "bonusKeywords": [
      "设备类型",
      "兜底需求",
      "依据不足"
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
  "DSS05-itlead-3-2": {
    "requiredKeywords": [
      "存储恢复",
      "评估影响",
      "定级"
    ],
    "bonusKeywords": [
      "延迟",
      "兜底",
      "上报"
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
  "DSS05-itlead-4-1": {
    "requiredKeywords": [
      "恢复方案",
      "报告",
      "指挥部"
    ],
    "bonusKeywords": [
      "启动窗口",
      "延误",
      "完整"
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
  "DSS05-itlead-4-2": {
    "requiredKeywords": [
      "存储告警",
      "链路中断",
      "业务影响"
    ],
    "bonusKeywords": [
      "模糊",
      "后续补齐",
      "决策"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-itlead-6-1": {
    "requiredKeywords": [
      "厂商方案",
      "市卫生健康局",
      "上报"
    ],
    "bonusKeywords": [
      "1小时",
      "时限",
      "完整"
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
  "DSS05-itlead-6-2": {
    "requiredKeywords": [
      "存储告警",
      "先报",
      "补齐"
    ],
    "bonusKeywords": [
      "信息不足",
      "卫健",
      "后续"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS05-itlead-7-1": {
    "requiredKeywords": [
      "市卫健局",
      "意见",
      "市网信办"
    ],
    "bonusKeywords": [
      "同步报",
      "延误",
      "一起报"
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
  "DSS05-itlead-7-2": {
    "requiredKeywords": [
      "初报",
      "续报",
      "处置进展"
    ],
    "bonusKeywords": [
      "闭环",
      "接收单位",
      "只发"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-medlead-8-1": {
    "requiredKeywords": [
      "收费环节",
      "手工",
      "兜底"
    ],
    "bonusKeywords": [
      "挂号",
      "住院",
      "检验",
      "药房"
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
  "DSS05-medlead-8-2": {
    "requiredKeywords": [
      "等待",
      "自行恢复",
      "门诊"
    ],
    "bonusKeywords": [
      "住院",
      "数据链",
      "断裂"
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
  "DSS05-sysadmin-11-1": {
    "requiredKeywords": [
      "业务科室",
      "自行核对",
      "数据"
    ],
    "bonusKeywords": [
      "记录数",
      "金额",
      "总量比对"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-sysadmin-11-2": {
    "requiredKeywords": [
      "上线",
      "校验",
      "回滚"
    ],
    "bonusKeywords": [
      "漏费",
      "病历缺页",
      "观察"
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
  "DSS05-sysadmin-2-1": {
    "requiredKeywords": [
      "指示灯",
      "业务可用性",
      "日志"
    ],
    "bonusKeywords": [
      "光纤",
      "影响范围",
      "硬件"
    ],
    "minMeasureLen": 15,
    "minResultLen": 10,
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
    "keywordPenaltyRatio": 0.2
  },
  "DSS05-sysadmin-2-2": {
    "requiredKeywords": [
      "厂商工程师",
      "到场",
      "核实"
    ],
    "bonusKeywords": [
      "等待",
      "业务中断",
      "外部"
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
  "DSS05-sysadmin-9-1": {
    "requiredKeywords": [
      "数据库实例",
      "切换",
      "同步状态"
    ],
    "bonusKeywords": [
      "切换时间点",
      "数据缺口",
      "未验证"
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
  "DSS05-sysadmin-9-2": {
    "requiredKeywords": [
      "重装",
      "应用",
      "耗时"
    ],
    "bonusKeywords": [
      "数据缺口",
      "扩大",
      "彻底"
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
  }
}
```
