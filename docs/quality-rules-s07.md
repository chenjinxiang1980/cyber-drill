# S07 处置文字质量规则清单（41 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S07 全部 41 个节点（correct 15 / distractor 26），剧本「外网边界防火墙被突破」
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
| T1 轻量 | 15/10 | DSS07-commander-13-1、DSS07-duty-1-1、DSS07-itlead-11-1、DSS07-itlead-3-1 |
| T2 标准 | 20/12 | DSS07-commander-13-2、DSS07-commander-5-1、DSS07-commander-5-2、DSS07-duty-1-2 |
| T3 记录 | 30/18 | S07-1、S07-11、S07-14、S07-5 |
| T4 关键 | 35/20 | S07-12、S07-13、S07-3、S07-4 |
| T5 复杂 | 40/25 | S07-10、S07-15、S07-2、S07-6 |
| T6 高复杂 | 45/28 | — |
| T7 取证 | 50/30 | — |
| T8 综合报告 | 60/40 | — |

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

## 1. 正确节点规则（15 个）

### S07-1 · 边界防火墙异常策略告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`放通策略`、`异常登录`、`登录源`
- `bonusKeywords`：`策略编号`、`放通端口`、`管理账号`、`首次发现时间`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报的核心事实是新增未知放通策略与管理账号异常登录，第三个词落在可核查的登录源上，不取标题里的“边界防火墙”泛词；0 分节点只作提交闸门。

### S07-2 · 核实策略变更与登录来源

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`变更审批单`、`操作账号`、`越权`
- `bonusKeywords`：`变更时间`、`登录源`、`正常运维`、`审批单编号`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实的关键不是看过日志，而是拿变更审批单对照操作账号、判断是否越权；加分词奖励把变更时间与正常运维排除过程写出来的人。

### S07-3 · 预警研判：确认较大（Ⅲ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`定级依据|定级理由|分级依据|判定依据`、`外联通道|外联链路|互联网出口|出口链路`、`横向`
- `bonusKeywords`：`Ⅲ级`、`较大`、`边界资产`、`预案分级标准`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级成立与否取决于“是否已建立外联通道”与“内网是否被横向利用”，故取这两个判据加定级依据；题面动词不在引擎动作词表，要素只保留对象+结论（确认/形成定级），等级写法只放加分词。

### S07-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`异常策略`、`报告`、`影响范围`
- `bonusKeywords`：`Ⅲ级`、`攻击源`、`已采取措施`、`事件等级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：院内报告必须写清异常策略内容、已采取措施与影响范围并给出启动建议，缺影响范围就等于只报现象不报边界；不要求结论要素，避免把“建议启动”写成已完成。

### S07-5 · 院应急指挥部批准启动Ⅲ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`响应级别|响应等级|响应分级`、`授权范围|授权权限|权限范围`
- `bonusKeywords`：`Ⅲ级`、`启动时间`、`岗位分工`、`对外上报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：审批节点的可核查产物是签发的启动令、明确的响应级别与授权范围（断网隔离、数据恢复、对外上报），三者缺一即无法证明授权成立。

### S07-6 · 备份当前配置、阻断异常通道并全量核对策略基线

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`防火墙配置|防火墙策略|配置备份|策略备份|设备配置`、`阻断`、`策略基线|基线配置|配置基线`
- `bonusKeywords`：`快照`、`回滚点`、`外联通道`、`变更前`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：该节点是“先备份再阻断后核对”的顺序节点：对象是防火墙配置、动作是阻断外联通道、判据是按策略基线全量核对差异，只写阻断不写备份的人拿不到分。

### S07-7 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`上报`、`事件等级|事件级别|事件分级`
- `bonusKeywords`：`2小时`、`同一时点`、`处置进展`、`上报表`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：报卫健的验收点是接收单位、上报动作与上报内容，取事件等级作判据证明不是一句“已报”；“2小时”含数字与空格，只放加分词。

### S07-8 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`不等外部回复|无需等待|不等待`、`事件等级`
- `bonusKeywords`：`2小时`、`同一时点`、`受影响边界`、`双报`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：与 S07-7 刻意错开：网信这一路的特征是同一时点双报、不等外部回复即完成，故判据取“不等外部回复 + 事件等级”，防止两张上报单写成同一段话；“双报/同一时点”属报送质量表现，与 S08-7 一致放加分词。

### S07-9 · 排查内网是否被横向利用

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`外联记录`、`外传痕迹`、`横向`
- `bonusKeywords`：`数据库`、`文件服务器`、`核心服务器`、`登录日志`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：横向排查必须覆盖边界外联记录、数据库与文件服务器外传痕迹、核心服务器登录日志三类证据，故取外联记录/外传痕迹/横向，只查登录日志不算完成。

### S07-10 · 取证防火墙日志与配置备份

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`策略快照`、`哈希`、`审计日志`
- `bonusKeywords`：`变更前后`、`保全位置`、`SHA-256`、`阻断前`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：取证要覆盖变更前后配置、策略快照与登录审计日志，并以哈希与保全位置收口；要求时间要素，用于证明固定发生在阻断之前。

### S07-11 · 向市网安大队报案并提交证据材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`证据清单`
- `bonusKeywords`：`先行`、`不等待`、`受理编号`、`补报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：报案节点考的是“不等外部研判意见先行报”加“提交证据材料与清单否则无法立案”，故取市网安大队/报案/证据清单；“先行”“不等待”属做得好才写得出，放加分词。

### S07-12 · 边界加固与全量策略复核

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 25 min

- `requiredKeywords`：`边界策略`、`全量复核|全面复核|全量核对|逐条核对`、`白名单|允许列表|放行清单`
- `bonusKeywords`：`最小放通`、`复核清单`、`回退点`、`管理面`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：加固的判据不是“已加固”，而是按白名单最小放通原则对边界策略做全量复核并形成清单与回退点；只复核对外策略的写法会被这三个词卡住。

### S07-13 · 应急结束：院应急指挥部宣布Ⅲ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`受理回执|受案回执|回执单|回执`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`策略基线`、`证据固定`、`横向排查`、`报备`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：结束的四项条件是策略基线恢复、横向排查完成、证据固定归档、上报与报案闭环；用“受理回执”把闭环落到可核查材料上，避免只写“业务已恢复即结束”。该节点验收动词是“宣布结束”，不在动作词表，故要素不含动作。

### S07-14 · 接收市网安大队报案受理回执

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`受理编号|受案编号|案件编号|回执编号`、`补证`
- `bonusKeywords`：`补证时限`、`对接人`、`缺件`、`归档`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：该节点只记录市网安大队回执内容，验收物是受理编号与补充材料要求（补证时限、缺件补齐），故只要求对象要素，不要求动作与结论。

### S07-15 · 总结经验教训

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 20 min

- `requiredKeywords`：`时间线`、`整改计划`、`责任人`
- `bonusKeywords`：`处置得失`、`复测`、`根因`、`时限`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘覆盖发现、研判、上报、报案、取证、阻断与加固七个环节，落笔必须有时间线、整改计划与责任人才算闭环；加分词奖励根因分析与复测安排。补录/复盘类动词不在动作词表，要素改用对象+时间+结论。

---

## 2. 混淆节点规则（26 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS07-commander-13-1 · 等全量复核通过一周后再宣布

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`全量复核`、`一周`、`加固到位`
- `bonusKeywords`：`加固到位`、`复发`、`稳妥`、`观察期`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该错误动作是“等全量复核通过满一周再宣布结束”，照此写会落到全量复核、一周与加固到位；5 分混淆按轻量档，只保证能提交。

### DSS07-commander-13-2 · 策略改回就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`策略`、`改回`、`结束`
- `bonusKeywords`：`回滚`、`未复核`、`告警消除`、`原版本`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“策略改回就结束”，其写法的三要素就是对象（策略）、动作（改回）与理由（告警消除即可结束）；不取“基线/闭环”等正确动作词，避免选混淆的人提交不了。

### DSS07-commander-5-1 · 让网安内部处理，指挥部不介入

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网安`、`内部处理`、`指挥部`
- `bonusKeywords`：`不介入`、`专业问题`、`自行处置`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误研判是“网安专业问题由网安内部处理、指挥部不介入”，写法必然带网安、内部处理与指挥部三方主语；不要求动作要素，防止专业动词不在词表被卡。

### DSS07-commander-5-2 · 等内网确实出问题了再说

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`内网`、`实质影响`、`再说`
- `bonusKeywords`：`暂不启动`、`等告警`、`边界`、`再评估`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等内网确实出问题再说”，按此叙述会写内网、实质影响与再说；不用正确动作的“启动/授权”做必含词。

### DSS07-duty-1-1 · 不备份当前配置，直接重启回滚

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`重启`、`回滚`、`不备份`
- `bonusKeywords`：`配置覆盖`、`登录源证据`、`日志`、`证据灭失`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该错误动作本身就是“不备份配置与日志、直接重启回滚”，故取重启/回滚/不备份三个动作与理由词，参演人照 descr 写即命中；“证据灭失”是后果，放加分词。

### DSS07-duty-1-2 · 等更多异常告警出来再处理

`distractor` ｜ 分值 8 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`单点告警`、`篡改`、`再处理`
- `bonusKeywords`：`更多告警`、`重复出现`、`暂不处置`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：拖延型混淆的写法必然出现单点告警、篡改与再处理三个判断点；不把带数字的等待时长设必含，规避写法差异。

### DSS07-itlead-11-1 · 只口头报案，不提交任何证据材料

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`证据清单`、`立案`
- `bonusKeywords`：`电话`、`未提交`、`材料`、`无法立案`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是“只电话口头报案、不交证据材料”，其叙述落点是口头报案、证据清单与能否立案；5 分混淆按 15/10，保证能被提交出来。

### DSS07-itlead-11-2 · 等网信办建议再定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`建议`、`再定`
- `bonusKeywords`：`上级`、`研判意见`、`暂不报案`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等网信办建议再定是否报案”，写法落到网信办、建议与再定；不取“报案/受理”等正确动作词，避免硬门槛卡死。

### DSS07-itlead-3-1 · 只看设备类型，不看是否已外联

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`边界设备`、`篡改`、`定级`
- `bonusKeywords`：`Ⅲ级`、`外联通道`、`横向痕迹`、`未核查`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误研判是“仅凭边界设备被篡改就定级”，写法的对象是边界设备、动作是篡改、结论是定级；等级写法“Ⅲ级”属敏感写法，只放加分词。

### DSS07-itlead-3-2 · 等查清楚谁改的再定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`来源`、`查清`、`再定`
- `bonusKeywords`：`追查`、`暂不定级`、`等待`、`攻击源`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：“先搞清谁改的再定级”这一错误动作，其合理叙述必然出现来源、查清与再定；按 8 分混淆取 20/12，只考描述完整性。

### DSS07-itlead-4-1 · 等处置方案出来再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`处置方案`、`解决方案`、`上报`
- `bonusKeywords`：`暂缓报告`、`完善`、`指挥部`、`等方案`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“报告要有解决方案，等方案出来再报”，故意把上级机关要求的“先报情况”写成了拖延；必含词取处置方案、解决方案与上报，照 descr 写即可命中。

### DSS07-itlead-4-2 · 先报"防火墙告警"留余地

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`告警`、`措辞`、`留余地`
- `bonusKeywords`：`未定级`、`初步`、`防火墙`、`保守表述`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：错误动作是“先按防火墙告警上报、措辞保守留余地”，写法的落点是告警、措辞与留余地；3 分混淆取最轻档，区分度交给三个词。

### DSS07-itlead-7-1 · 等网信办那边同步了再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`同步`、`一起报`
- `bonusKeywords`：`更高效`、`等待`、`卫健`、`两小时`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等网信办同步后再一并报卫健”，其叙述必然写到网信办、同步与一起报；不把带数字的时限设必含，避免“2小时/2 小时”写法漏命中。

### DSS07-itlead-7-2 · 只发初步情况，不写等级与影响范围，后续不续报

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`初步情况`、`影响范围`、`续报`
- `bonusKeywords`：`事件等级`、`续报时点`、`不完整`、`首报`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：错误动作是“只发一句初步情况、不写等级与影响范围、也不约定续报”，写法的三词就落在初步情况、影响范围与续报上；3 分混淆按 15/10。

### DSS07-itlead-8-1 · 未经脱敏把完整配置与日志直接发外部邮箱

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`未脱敏`、`外部邮箱`、`登录日志`
- `bonusKeywords`：`内网地址`、`完整配置`、`泄露`、`篡改`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“未脱敏即把完整配置与登录日志发到外部邮箱”，写法的对象是登录日志与配置、动作落到未脱敏外发；不取正确动作的“脱敏通道/安全通道”。

### DSS07-itlead-8-2 · 等处置完了再报完整情况

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`处置`、`完整情况`、`再报`
- `bonusKeywords`：`等待`、`上报`、`闭环`、`暂缓`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等处置完了再报完整情况”，合理叙述会写处置、完整情况与再报三个词；“处置”取词干而非“处置完成”，兼容“处置完毕/处置结束”等写法。

### DSS07-netsec-10-1 · 先恢复配置，取证留个描述

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`恢复配置`、`取证`、`描述`
- `bonusKeywords`：`优先`、`证据`、`事后`、`保全材料`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“先恢复配置、取证只留文字描述”，写法的顺序落点是恢复配置、取证与描述；不把“哈希/快照”等正确取证判据设必含，否则照 descr 写的人过不了。

### DSS07-netsec-10-2 · 让厂商帮忙做取证

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`厂商`、`取证`、`专业`
- `bonusKeywords`：`远程`、`专业`、`等待`、`支持`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是“让厂商帮忙取证、本院等结果”，写法落在厂商、取证与专业；5 分混淆取 15/10，避免提交门槛高于其扣分力度。

### DSS07-netsec-2-1 · 只看登录日志，不核对审批单与源地址

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`登录日志`、`审批单`、`越权`
- `bonusKeywords`：`登录源`、`核实范围`、`变更记录`、`未核对`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法是“只查登录日志、不核对变更审批单与源地址”，叙述必然出现登录日志、审批单与越权判定；不取“登录源 IP”这种含空格写法。

### DSS07-netsec-2-2 · 直接把策略改回原来的版本

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`策略`、`改回`、`恢复`
- `bonusKeywords`：`原版本`、`未核实`、`未备份`、`先进先出`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“直接把策略改回原版本先恢复正常”，写法的三要素是策略、改回与恢复；与 commander-13-2 的“改回即结束”用不同第三词区分文本指纹。

### DSS07-netsec-6-1 · 只改回那一条被篡改的策略

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`篡改`、`改回`、`最小化`
- `bonusKeywords`：`单条`、`未全量`、`基线`、`只改动`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是“只改回被篡改的那一条策略、最小化改动”，对象取“篡改”词干而非“篡改策略”，兼容“被篡改的那条策略”这类写法；不取“全量核对基线”等正确动作词。

### DSS07-netsec-6-2 · 等抓包确认没有流量再阻断

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`抓包`、`流量`、`再阻断`
- `bonusKeywords`：`阻断`、`确认`、`攻击停止`、`再处置`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等抓包确认没有流量再阻断”，写法落点是抓包、流量与再阻断；属观望型混淆，按 20/12 只作提交闸门。

### DSS07-netsec-9-1 · 只查登录日志，不查外联与数据库外传

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`登录日志`、`外联`、`外传`
- `bonusKeywords`：`数据库`、`文件服务器`、`排查范围`、`未核查`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法是“只查服务器登录日志、不查边界外联与数据库外传”，写法自然带登录日志、外联与外传三词；与 netsec-2-1 用审批单/越权区分。

### DSS07-netsec-9-2 · 等防火墙改回后再查内网

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`边界`、`恢复`、`再查`
- `bonusKeywords`：`内网`、`横向`、`等待`、`先后顺序`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误动作是“等防火墙改回、边界恢复后再查内网”，落点顺序是边界、恢复与再查；属“先恢复后排查”型延误混淆。

### DSS07-sysadmin-12-1 · 只复核对外相关策略，对内策略不动

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`对外`、`复核`、`对内`
- `bonusKeywords`：`重点`、`未全量`、`内网`、`策略清单`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法是“只复核对外相关策略、对内策略不动”，用“对外/对内”这一对范围词加复核作判据，比取“对外策略”更能兼容“对外的策略”等写法。

### DSS07-sysadmin-12-2 · 等恢复一周后再做全量复核

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`恢复`、`一周`、`全量复核`
- `bonusKeywords`：`加固`、`延后`、`复发`、`观察`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是“等恢复满一周后再做全量复核”，叙述必然出现恢复、一周与全量复核三词；时间写作“一周”而非“7 天”，规避数字与空格差异。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S07-1 | 正确 | — | 0 | 放通策略、异常登录、登录源 | 策略编号、放通端口、管理账号、首次发现时间 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S07-2 | 正确 | F12 | 8 | 变更审批单、操作账号、越权 | 变更时间、登录源、正常运维、审批单编号 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S07-3 | 正确 | F3 | 8 | 定级依据\|定级理由\|分级依据\|判定依据、外联通道\|外联链路\|互联网出口\|出口链路、横向 | Ⅲ级、较大、边界资产、预案分级标准 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S07-4 | 正确 | F4 | 8 | 异常策略、报告、影响范围 | Ⅲ级、攻击源、已采取措施、事件等级 | 35/20 | 对象、动作、时间 | P4 | 0.25/0.30 |
| S07-5 | 正确 | F9 | 8 | 启动令、响应级别\|响应等级\|响应分级、授权范围\|授权权限\|权限范围 | Ⅲ级、启动时间、岗位分工、对外上报 | 30/18 | 对象、动作、时间 | P8 | 0.25/0.30 |
| S07-6 | 正确 | F9 | 7 | 防火墙配置\|防火墙策略\|配置备份\|策略备份\|设备配置、阻断、策略基线\|基线配置\|配置基线 | 快照、回滚点、外联通道、变更前 | 40/25 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S07-7 | 正确 | F2 | 7 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、上报、事件等级\|事件级别\|事件分级 | 2小时、同一时点、处置进展、上报表 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S07-8 | 正确 | F2 | 7 | 市网信办\|网信办\|市委网信办、不等外部回复\|无需等待\|不等待、事件等级 | 2小时、同一时点、受影响边界、双报 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S07-9 | 正确 | F9 | 8 | 外联记录、外传痕迹、横向 | 数据库、文件服务器、核心服务器、登录日志 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S07-10 | 正确 | F13 | 8 | 策略快照、哈希、审计日志 | 变更前后、保全位置、SHA-256、阻断前 | 40/25 | 对象、时间、结论 | P5 | 0.25/0.30 |
| S07-11 | 正确 | F5 | 8 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、证据清单 | 先行、不等待、受理编号、补报 | 30/18 | 对象、时间 | P4 | 0.25/0.30 |
| S07-12 | 正确 | F9 | 8 | 边界策略、全量复核\|全面复核\|全量核对\|逐条核对、白名单\|允许列表\|放行清单 | 最小放通、复核清单、回退点、管理面 | 35/20 | 对象、结论 | P2 | 0.25/0.30 |
| S07-13 | 正确 | F8 | 8 | 结束条件\|终止条件\|结束标准\|终止标准、受理回执\|受案回执\|回执单\|回执、闭环\|闭环管理\|办结\|归档 | 策略基线、证据固定、横向排查、报备 | 35/20 | 对象、结论 | P8 | 0.25/0.30 |
| S07-14 | 正确 | F9 | 7 | 受理回执\|受案回执\|回执单\|回执、受理编号\|受案编号\|案件编号\|回执编号、补证 | 补证时限、对接人、缺件、归档 | 30/18 | 无 | P4 | 0.20/0.30 |
| S07-15 | 正确 | — | 6 | 时间线、整改计划、责任人 | 处置得失、复测、根因、时限 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS07-commander-13-1 | 混淆 | — | 5 | 全量复核、一周、加固到位 | 加固到位、复发、稳妥、观察期 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS07-commander-13-2 | 混淆 | — | 8 | 策略、改回、结束 | 回滚、未复核、告警消除、原版本 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS07-commander-5-1 | 混淆 | — | 8 | 网安、内部处理、指挥部 | 不介入、专业问题、自行处置 | 20/12 | 对象 | P3 | 0.15/0.25 |
| DSS07-commander-5-2 | 混淆 | — | 8 | 内网、实质影响、再说 | 暂不启动、等告警、边界、再评估 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-duty-1-1 | 混淆 | — | 5 | 重启、回滚、不备份 | 配置覆盖、登录源证据、日志、证据灭失 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS07-duty-1-2 | 混淆 | — | 8 | 单点告警、篡改、再处理 | 更多告警、重复出现、暂不处置 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-itlead-11-1 | 混淆 | — | 5 | 口头报案、证据清单、立案 | 电话、未提交、材料、无法立案 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS07-itlead-11-2 | 混淆 | — | 8 | 网信办、建议、再定 | 上级、研判意见、暂不报案 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-itlead-3-1 | 混淆 | — | 5 | 边界设备、篡改、定级 | Ⅲ级、外联通道、横向痕迹、未核查 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS07-itlead-3-2 | 混淆 | — | 8 | 来源、查清、再定 | 追查、暂不定级、等待、攻击源 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-itlead-4-1 | 混淆 | — | 8 | 处置方案、解决方案、上报 | 暂缓报告、完善、指挥部、等方案 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-itlead-4-2 | 混淆 | — | 3 | 告警、措辞、留余地 | 未定级、初步、防火墙、保守表述 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS07-itlead-7-1 | 混淆 | — | 8 | 网信办、同步、一起报 | 更高效、等待、卫健、两小时 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-itlead-7-2 | 混淆 | — | 3 | 初步情况、影响范围、续报 | 事件等级、续报时点、不完整、首报 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS07-itlead-8-1 | 混淆 | — | 8 | 未脱敏、外部邮箱、登录日志 | 内网地址、完整配置、泄露、篡改 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS07-itlead-8-2 | 混淆 | — | 8 | 处置、完整情况、再报 | 等待、上报、闭环、暂缓 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-netsec-10-1 | 混淆 | — | 8 | 恢复配置、取证、描述 | 优先、证据、事后、保全材料 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS07-netsec-10-2 | 混淆 | — | 5 | 厂商、取证、专业 | 远程、专业、等待、支持 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS07-netsec-2-1 | 混淆 | — | 5 | 登录日志、审批单、越权 | 登录源、核实范围、变更记录、未核对 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS07-netsec-2-2 | 混淆 | — | 8 | 策略、改回、恢复 | 原版本、未核实、未备份、先进先出 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS07-netsec-6-1 | 混淆 | — | 5 | 篡改、改回、最小化 | 单条、未全量、基线、只改动 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS07-netsec-6-2 | 混淆 | — | 8 | 抓包、流量、再阻断 | 阻断、确认、攻击停止、再处置 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-netsec-9-1 | 混淆 | — | 5 | 登录日志、外联、外传 | 数据库、文件服务器、排查范围、未核查 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS07-netsec-9-2 | 混淆 | — | 8 | 边界、恢复、再查 | 内网、横向、等待、先后顺序 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS07-sysadmin-12-1 | 混淆 | — | 5 | 对外、复核、对内 | 重点、未全量、内网、策略清单 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS07-sysadmin-12-2 | 混淆 | — | 5 | 恢复、一周、全量复核 | 加固、延后、复发、观察 | 15/10 | 对象 | P7 | 0.15/0.20 |

---

## 4. 总量、分布与风险

### 4.1 S07 规则总量

- 节点 41：正确 15、混淆 26；规则条数 41，与节点一一对应，无遗漏无多余。
- 必含词合计 123 个，平均 3.0 个/节点；加分词合计 161 个。
- 长度档：15/10 共 12 个；20/12 共 14 个；30/18 共 4 个；35/20 共 6 个；40/25 共 5 个。
- 要素组合：对象 27 个；对象+动作 1 个；对象+时间+结论 2 个；对象+时间 1 个；对象+动作+结论 4 个；对象+结论 2 个；对象+动作+时间 4 个。
- 表单绑定（正确节点）：F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F8 1 个、F9 5 个、无表单 2 个。

### 4.2 风险最高的几类节点

1. **混淆节点（26 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S07-3`（研判定级，8 分）：必含 `定级依据`。参演人常写成“依据预案分级标准研判为Ⅲ级”，字面没有“定级依据”四个字，会缺一项；该节点题面动词只有“汇总/确认”，不在引擎动作词表，要素已去掉“动作”。
3. `S07-6`（配置备份与策略基线，7 分）：必含 `防火墙配置|防火墙策略|配置备份|策略备份|设备配置`（第二批已加同义组，第四批补 `设备配置`）；“当前配置”“原配置”仍不命中。
4. `S07-2`（核实策略变更与登录来源，8 分）：`策略变更`、`登录来源` 需要按原词写；只写“查了防火墙日志”会缺两项。
5. `S07-11`（报案，8 分）：报案与受理类动词不在动作词表，要素取对象+时间，参演人需写出受理时间或报案时间，否则缺要素。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（41 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S07-1": {
    "requiredKeywords": [
      "放通策略",
      "异常登录",
      "登录源"
    ],
    "bonusKeywords": [
      "策略编号",
      "放通端口",
      "管理账号",
      "首次发现时间"
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
  "S07-2": {
    "requiredKeywords": [
      "变更审批单",
      "操作账号",
      "越权"
    ],
    "bonusKeywords": [
      "变更时间",
      "登录源",
      "正常运维",
      "审批单编号"
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
  "S07-3": {
    "requiredKeywords": [
      "定级依据|定级理由|分级依据|判定依据",
      "外联通道|外联链路|互联网出口|出口链路",
      "横向"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "较大",
      "边界资产",
      "预案分级标准"
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
  "S07-4": {
    "requiredKeywords": [
      "异常策略",
      "报告",
      "影响范围"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "攻击源",
      "已采取措施",
      "事件等级"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-5": {
    "requiredKeywords": [
      "启动令",
      "响应级别|响应等级|响应分级",
      "授权范围|授权权限|权限范围"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "启动时间",
      "岗位分工",
      "对外上报"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-6": {
    "requiredKeywords": [
      "防火墙配置|防火墙策略|配置备份|策略备份|设备配置",
      "阻断",
      "策略基线|基线配置|配置基线"
    ],
    "bonusKeywords": [
      "快照",
      "回滚点",
      "外联通道",
      "变更前"
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
  "S07-7": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "上报",
      "事件等级|事件级别|事件分级"
    ],
    "bonusKeywords": [
      "2小时",
      "同一时点",
      "处置进展",
      "上报表"
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
  "S07-8": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "不等外部回复|无需等待|不等待",
      "事件等级"
    ],
    "bonusKeywords": [
      "2小时",
      "同一时点",
      "受影响边界",
      "双报"
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
  "S07-9": {
    "requiredKeywords": [
      "外联记录",
      "外传痕迹",
      "横向"
    ],
    "bonusKeywords": [
      "数据库",
      "文件服务器",
      "核心服务器",
      "登录日志"
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
  "S07-10": {
    "requiredKeywords": [
      "策略快照",
      "哈希",
      "审计日志"
    ],
    "bonusKeywords": [
      "变更前后",
      "保全位置",
      "SHA-256",
      "阻断前"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-11": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "证据清单"
    ],
    "bonusKeywords": [
      "先行",
      "不等待",
      "受理编号",
      "补报"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-12": {
    "requiredKeywords": [
      "边界策略",
      "全量复核|全面复核|全量核对|逐条核对",
      "白名单|允许列表|放行清单"
    ],
    "bonusKeywords": [
      "最小放通",
      "复核清单",
      "回退点",
      "管理面"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-13": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "受理回执|受案回执|回执单|回执",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "策略基线",
      "证据固定",
      "横向排查",
      "报备"
    ],
    "minMeasureLen": 35,
    "minResultLen": 20,
    "requiredElements": [
      "对象",
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S07-14": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "受理编号|受案编号|案件编号|回执编号",
      "补证"
    ],
    "bonusKeywords": [
      "补证时限",
      "对接人",
      "缺件",
      "归档"
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
    "keywordPenaltyRatio": 0.3
  },
  "S07-15": {
    "requiredKeywords": [
      "时间线",
      "整改计划",
      "责任人"
    ],
    "bonusKeywords": [
      "处置得失",
      "复测",
      "根因",
      "时限"
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
  "DSS07-commander-13-1": {
    "requiredKeywords": [
      "全量复核",
      "一周",
      "加固到位"
    ],
    "bonusKeywords": [
      "加固到位",
      "复发",
      "稳妥",
      "观察期"
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
  "DSS07-commander-13-2": {
    "requiredKeywords": [
      "策略",
      "改回",
      "结束"
    ],
    "bonusKeywords": [
      "回滚",
      "未复核",
      "告警消除",
      "原版本"
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
  "DSS07-commander-5-1": {
    "requiredKeywords": [
      "网安",
      "内部处理",
      "指挥部"
    ],
    "bonusKeywords": [
      "不介入",
      "专业问题",
      "自行处置"
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
  "DSS07-commander-5-2": {
    "requiredKeywords": [
      "内网",
      "实质影响",
      "再说"
    ],
    "bonusKeywords": [
      "暂不启动",
      "等告警",
      "边界",
      "再评估"
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
  "DSS07-duty-1-1": {
    "requiredKeywords": [
      "重启",
      "回滚",
      "不备份"
    ],
    "bonusKeywords": [
      "配置覆盖",
      "登录源证据",
      "日志",
      "证据灭失"
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
  "DSS07-duty-1-2": {
    "requiredKeywords": [
      "单点告警",
      "篡改",
      "再处理"
    ],
    "bonusKeywords": [
      "更多告警",
      "重复出现",
      "暂不处置"
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
  "DSS07-itlead-11-1": {
    "requiredKeywords": [
      "口头报案",
      "证据清单",
      "立案"
    ],
    "bonusKeywords": [
      "电话",
      "未提交",
      "材料",
      "无法立案"
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
  "DSS07-itlead-11-2": {
    "requiredKeywords": [
      "网信办",
      "建议",
      "再定"
    ],
    "bonusKeywords": [
      "上级",
      "研判意见",
      "暂不报案"
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
  "DSS07-itlead-3-1": {
    "requiredKeywords": [
      "边界设备",
      "篡改",
      "定级"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "外联通道",
      "横向痕迹",
      "未核查"
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
  "DSS07-itlead-3-2": {
    "requiredKeywords": [
      "来源",
      "查清",
      "再定"
    ],
    "bonusKeywords": [
      "追查",
      "暂不定级",
      "等待",
      "攻击源"
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
  "DSS07-itlead-4-1": {
    "requiredKeywords": [
      "处置方案",
      "解决方案",
      "上报"
    ],
    "bonusKeywords": [
      "暂缓报告",
      "完善",
      "指挥部",
      "等方案"
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
  "DSS07-itlead-4-2": {
    "requiredKeywords": [
      "告警",
      "措辞",
      "留余地"
    ],
    "bonusKeywords": [
      "未定级",
      "初步",
      "防火墙",
      "保守表述"
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
  "DSS07-itlead-7-1": {
    "requiredKeywords": [
      "网信办",
      "同步",
      "一起报"
    ],
    "bonusKeywords": [
      "更高效",
      "等待",
      "卫健",
      "两小时"
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
  "DSS07-itlead-7-2": {
    "requiredKeywords": [
      "初步情况",
      "影响范围",
      "续报"
    ],
    "bonusKeywords": [
      "事件等级",
      "续报时点",
      "不完整",
      "首报"
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
  "DSS07-itlead-8-1": {
    "requiredKeywords": [
      "未脱敏",
      "外部邮箱",
      "登录日志"
    ],
    "bonusKeywords": [
      "内网地址",
      "完整配置",
      "泄露",
      "篡改"
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
  "DSS07-itlead-8-2": {
    "requiredKeywords": [
      "处置",
      "完整情况",
      "再报"
    ],
    "bonusKeywords": [
      "等待",
      "上报",
      "闭环",
      "暂缓"
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
  "DSS07-netsec-10-1": {
    "requiredKeywords": [
      "恢复配置",
      "取证",
      "描述"
    ],
    "bonusKeywords": [
      "优先",
      "证据",
      "事后",
      "保全材料"
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
  "DSS07-netsec-10-2": {
    "requiredKeywords": [
      "厂商",
      "取证",
      "专业"
    ],
    "bonusKeywords": [
      "远程",
      "专业",
      "等待",
      "支持"
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
  "DSS07-netsec-2-1": {
    "requiredKeywords": [
      "登录日志",
      "审批单",
      "越权"
    ],
    "bonusKeywords": [
      "登录源",
      "核实范围",
      "变更记录",
      "未核对"
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
  "DSS07-netsec-2-2": {
    "requiredKeywords": [
      "策略",
      "改回",
      "恢复"
    ],
    "bonusKeywords": [
      "原版本",
      "未核实",
      "未备份",
      "先进先出"
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
  "DSS07-netsec-6-1": {
    "requiredKeywords": [
      "篡改",
      "改回",
      "最小化"
    ],
    "bonusKeywords": [
      "单条",
      "未全量",
      "基线",
      "只改动"
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
  "DSS07-netsec-6-2": {
    "requiredKeywords": [
      "抓包",
      "流量",
      "再阻断"
    ],
    "bonusKeywords": [
      "阻断",
      "确认",
      "攻击停止",
      "再处置"
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
  "DSS07-netsec-9-1": {
    "requiredKeywords": [
      "登录日志",
      "外联",
      "外传"
    ],
    "bonusKeywords": [
      "数据库",
      "文件服务器",
      "排查范围",
      "未核查"
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
  "DSS07-netsec-9-2": {
    "requiredKeywords": [
      "边界",
      "恢复",
      "再查"
    ],
    "bonusKeywords": [
      "内网",
      "横向",
      "等待",
      "先后顺序"
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
  "DSS07-sysadmin-12-1": {
    "requiredKeywords": [
      "对外",
      "复核",
      "对内"
    ],
    "bonusKeywords": [
      "重点",
      "未全量",
      "内网",
      "策略清单"
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
  "DSS07-sysadmin-12-2": {
    "requiredKeywords": [
      "恢复",
      "一周",
      "全量复核"
    ],
    "bonusKeywords": [
      "加固",
      "延后",
      "复发",
      "观察"
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
  }
}
```
