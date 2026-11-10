# S02 处置文字质量规则清单（42 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S02 全部 42 个节点（correct 16 / distractor 26），剧本「内网横向渗透/账号泄露」
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
| T1 轻量 | 15/10 | DSS02-commander-13-1、DSS02-duty-1-1、DSS02-duty-2-1、DSS02-itlead-12-1 |
| T2 标准 | 20/12 | DSS02-commander-13-2、DSS02-commander-5-1、DSS02-commander-5-2、DSS02-duty-1-2 |
| T3 记录 | 30/18 | S02-1、S02-12、S02-14、S02-5 |
| T4 关键 | 35/20 | S02-10、S02-13、S02-3、S02-4 |
| T5 复杂 | 40/25 | S02-15、S02-16、S02-2、S02-6 |
| T6 高复杂 | 45/28 | S02-11 |
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

## 1. 正确节点规则（16 个）

### S02-1 · 异常账号 10 分钟内多地同时登录告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`运维账号`、`同时登录`、`境外地址`
- `bonusKeywords`：`堡垒机`、`核心业务系统`、`登录终端`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报只需把被盗账号、多地同时登录与境外来源三个事实说清，三词均取自告警描述；0 分节点规则只作提交闸门。

### S02-2 · 核实异常登录来源与账号归属

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`账号归属`、`登录日志`、`堡垒机`
- `bonusKeywords`：`本人确认`、`异地登录`、`登录终端`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实的关键是账号归属与登录来源能不能对上，故取账号归属、登录日志、堡垒机三个可核查项；加分词奖励当面确认、异地登录排除与终端定位。

### S02-3 · 预警研判：确认较大（Ⅲ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`数据外泄`、`患者安全`、`较大`
- `bonusKeywords`：`Ⅲ级`、`扩散趋势`、`电子病历`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级必须给依据，故取数据外泄、患者安全两条硬依据与“较大”这一等级词，罗马数字只放加分以规避 Ⅲ/3 写法差异。

### S02-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`简报`、`失陷账号`、`启动`
- `bonusKeywords`：`Ⅲ级`、`渗透路径`、`10分钟`、`已采取措施`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：院内报告要有一页简报、点明失陷账号并提出启动请求；加分词奖励等级、渗透路径与已采取措施三项内容的完整度。

### S02-5 · 院应急指挥部批准启动Ⅲ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`授权范围|授权权限|权限范围`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`启动时间`、`停用账号`、`隔离主机`、`取证`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：审批的可核查产物是启动令、授权范围与响应级别，且 S02 的授权须覆盖停用账号、隔离主机、取证、对外上报四类。

### S02-6 · 禁用账号、踢出会话并回收授权

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`涉事账号`、`会话`、`授权`
- `bonusKeywords`：`回收授权`、`踢出会话`、`重置口令`、`操作记录`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：本节点判据是会话状态与授权范围而非主机，故取涉事账号/会话/授权；加分词奖励先踢会话后回收授权、重置口令与操作记录的顺序完整。

### S02-7 · 排查横向渗透路径与失陷主机

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`失陷主机`、`跳板机`、`域控`
- `bonusKeywords`：`数据库`、`访问日志`、`失陷清单`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：排查的判据是范围覆盖跳板机与域控，而不是只看最近一次登录的主机，故取失陷主机/跳板机/域控三个范围对象。

### S02-8 · 2 小时内报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`影响范围`、`回执`
- `bonusKeywords`：`2小时`、`已采取措施`、`先电话后书面`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：单报卫健局要写清接收单位、影响范围与回执；时限“2 小时”写法不稳只放加分，时间要素对应先电话后书面的报送时点。

### S02-9 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`舆情风险`、`回执`
- `bonusKeywords`：`同一时点`、`不等回复`、`攻击来源`、`事件编号`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：同报网信办的区分点是舆情风险与回执，故取市网信办/舆情风险/回执，把同一时点、不等回复放加分。

### S02-10 · 隔离失陷主机并封堵攻击源

`correct` ｜ 类型 `task` ｜ 表单 `F10` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`攻击源`、`封堵`、`失陷主机`
- `bonusKeywords`：`高风险网段`、`交换机端口`、`内存`、`变更记录`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：本节点要求先封堵后隔离，必含词分别锁定攻击源、封堵动作与失陷主机；加分词奖励网段范围、交换机端口与变更记录。

### S02-11 · 取证与日志审计

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 8 ｜ 超时 25 min

- `requiredKeywords`：`登录日志`、`堡垒机`、`哈希`
- `bonusKeywords`：`内存`、`样本`、`保全位置`、`时间戳`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：取证要覆盖登录日志与堡垒机操作日志，并以哈希与保全位置收口；时间要素证明固定发生在隔离后、恢复前。

### S02-12 · 向市网安大队报案并提交材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`受理编号|受案编号|案件编号|回执编号`
- `bonusKeywords`：`不等待`、`事件经过`、`联系人`、`书面材料`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：报案的可核查产物是受理编号，且必须不等外部研判意见先行报案；事件经过、联系人与书面材料放加分。

### S02-13 · 应急结束：院应急指挥部宣布Ⅲ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`横向路径`、`全量复核|全面复核|全量核对|逐条核对`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`取证完成`、`报案闭环`、`逐条核对`、`Ⅲ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束的门槛是横向路径封堵、账号全量复核与上报闭环三项逐条核对，缺一项不得宣布结束。

### S02-14 · 接收市网安大队受理回执与取证要求

`correct` ｜ 类型 `report` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 10 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`受理编号|受案编号|案件编号|回执编号`、`材料清单`
- `bonusKeywords`：`取证要求`、`提交时限`、`对接人`、`转指挥部`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：该节点记录外部回执，验收物是受理回执、受理编号与材料清单三件实物；要素只留对象，避免把“转指挥部”判成缺动作。

### S02-15 · 恢复验证：账号权限与业务可用性确认

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`权限回收`、`会话`、`核心业务`
- `bonusKeywords`：`未恢复系统`、`遗留风险`、`挂号`、`收费`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：恢复验证的判据是权限回收、会话清零与核心业务可用，故不取“恢复”这一泛词；加分词奖励未恢复系统与遗留风险清单。

### S02-16 · 复盘总结与归档

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 30 min

- `requiredKeywords`：`时间线`、`整改`、`归档`
- `bonusKeywords`：`关键决策点`、`处置得失`、`责任人`、`闭环`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘交付物是时间线、整改责任清单与归档三件套，加分词奖励关键决策点与处置得失的还原。

---

## 2. 混淆节点规则（26 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS02-commander-13-1 · 等所有主机重装系统一周后再宣布结束

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`涉事主机`、`重装系统`、`一周`
- `bonusKeywords`：`后门残留`、`结束`、`观察期`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该混淆的写法是涉事主机全部重装、观察满一周再结束，故取涉事主机/重装系统/一周；七天观察属观望延误，用 P7。

### DSS02-commander-13-2 · 密码改了就宣布结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`账号密码`、`响应结束`、`验证`
- `bonusKeywords`：`Ⅲ级`、`取证`、`上报闭环`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆认为改完密码即可结束，故取账号密码/响应结束/验证；它省掉的取证与上报闭环只作加分。

### DSS02-commander-5-1 · 先让信息科内部自行处理，指挥部不直接介入

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`信息科`、`自行处置`、`指挥部`
- `bonusKeywords`：`启动令`、`授权范围`、`不介入`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆把处置责任推回信息科、指挥部不介入，故取信息科/自行处置/指挥部；启动令与授权范围是它缺的东西，放加分。

### DSS02-commander-5-2 · 等市网信办给出处置意见再启动

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`处置意见`、`启动`
- `bonusKeywords`：`Ⅲ级`、`等待`、`响应`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆等网信办意见再决定是否启动，故取市网信办/处置意见/启动；Ⅲ级写法不稳，只放加分。

### DSS02-duty-1-1 · 只看值班表确认是本人排班，不核查登录来源

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`值班表`、`本人操作`、`关闭告警`
- `bonusKeywords`：`源IP`、`登录终端`、`排班`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆以值班表证明本人操作后直接关告警，故取值班表/本人操作/关闭告警；源 IP 与登录终端只作加分。

### DSS02-duty-1-2 · 等账号再登录一次，抓包分析来源

`distractor` ｜ 分值 8 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`抓包`、`下次登录`、`来源`
- `bonusKeywords`：`流量特征`、`不处置`、`等待`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆等下次登录再抓包分析来源、当前不动手，故取抓包/下次登录/来源；观望档用 P7。

### DSS02-duty-2-1 · 让账号本人先自查电脑有没有中毒

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`账号本人`、`自行检查`、`登录日志`
- `bonusKeywords`：`中毒`、`堡垒机`、`不再核查`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆把核实推给账号本人自查、不再看登录日志，故取账号本人/自行检查/登录日志；堡垒机来源放加分。

### DSS02-itlead-12-1 · 不报案，内部处理就行

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`内部处理`、`报案`、`市网安大队`
- `bonusKeywords`：`流程长`、`影响大`、`不报案`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该混淆选择内部处理、不向网安大队报案，故取内部处理/报案/市网安大队；流程长、影响大放加分。

### DSS02-itlead-12-2 · 只口头报案，不提交书面材料与证据

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`电话`、`材料`
- `bonusKeywords`：`联系人`、`补交`、`后续`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆只电话口头报案、材料后补，故取口头报案/电话/材料三个自述必现词；联系人与补交放加分。

### DSS02-itlead-3-1 · 只统计异常账号数量，不判断是否同一攻击源

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`异常账号`、`统计`、`规模`
- `bonusKeywords`：`数量`、`攻击源`、`失陷主机`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆只按异常账号个数判断事件规模，故取异常账号/统计/规模；攻击源与失陷主机是它没做关联的对象，放加分。

### DSS02-itlead-3-2 · 直接按最高等级Ⅰ级启动，确保全面应对

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`最高等级`、`影响评估`、`启动`
- `bonusKeywords`：`停机`、`上报范围`、`Ⅰ级`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该混淆跳过影响评估按最高等级启动，故取最高等级/影响评估/启动；Ⅰ级与扩大停机范围只放加分。

### DSS02-itlead-4-1 · 先做一份详细的账号清单和登录分析报告

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`账号清单`、`登录分析`、`指挥部`
- `bonusKeywords`：`详细`、`延误`、`上报时机`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆先做账号清单与登录分析长报告再上报，故取账号清单/登录分析/指挥部；用 P7 对应其延误性质。

### DSS02-itlead-4-2 · 先内部讨论一下影响范围再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`内部讨论`、`影响范围`、`上报`
- `bonusKeywords`：`指挥部`、`讨论清楚`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆以内部讨论影响范围为由推迟上报，故取内部讨论/影响范围/上报；不把“延误”这类后果词设必含。

### DSS02-itlead-8-1 · 等网信办那边的意见先出来再给卫健局报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`技术意见`、`市卫生健康局`
- `bonusKeywords`：`等待`、`一并报送`、`卫健`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆等网信办技术意见出来才报卫健局，故取市网信办/技术意见/市卫生健康局；两家单位名在正文自然出现，命中稳定。

### DSS02-itlead-8-2 · 直接让网络安全管理员和卫健局技术岗对接

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网络安全管理员`、`技术岗`、`对接`
- `bonusKeywords`：`指挥链`、`绕过`、`报送`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆让管理员绕过指挥链与卫健局技术岗直接对接，故取网络安全管理员/技术岗/对接；指挥链放加分。

### DSS02-itlead-9-1 · 只报账号被盗，不报横向渗透风险与已采取措施

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`账号被盗`、`上报`、`风险`
- `bonusKeywords`：`影响范围`、`横向渗透`、`初报`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆只报账号被盗、不谈横向渗透与已采取措施，故取账号被盗/上报/风险三个自述必现词；影响范围与横向渗透放加分。

### DSS02-itlead-9-2 · 等账号都改好密码了再一次性报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`改密`、`市网信办`、`报送`
- `bonusKeywords`：`一次性`、`等待`、`全部完成`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆等全部改密完成才一次性报送网信办，故取改密/市网信办/报送；不做初报的时点问题放加分。

### DSS02-netsec-10-1 · 只改密码，不做主机隔离

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`密码`、`隔离`、`主机`
- `bonusKeywords`：`重置密码`、`外联通道`、`未隔离`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆只改账号密码不隔离主机，故取密码/隔离/主机三个自述里必现的词；外联通道与未隔离放加分。

### DSS02-netsec-10-2 · 把失陷主机上的重要数据先拷出来再隔离

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`重要数据`、`拷出`、`隔离`
- `bonusKeywords`：`失陷主机`、`易失证据`、`拷贝`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆先拷数据再隔离、破坏易失证据，故取重要数据/拷出/隔离；证据被破坏的后果只作加分。

### DSS02-netsec-11-1 · 只保存登录日志，事件发生后其他日志意义不大

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`登录日志`、`设备日志`、`保全`
- `bonusKeywords`：`堡垒机`、`主机日志`、`暂不`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆只保账号登录日志、弃设备日志，故取登录日志/设备日志/保全；堡垒机与主机日志放加分。

### DSS02-netsec-11-2 · 先恢复业务，取证留个事件经过描述就行

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`事件经过`、`恢复`、`日志`
- `bonusKeywords`：`样本`、`后续补`、`取证`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆先恢复业务、只留事件经过描述，故取事件经过/恢复/日志；样本保全与后续补证放加分。

### DSS02-netsec-2-2 · 只按 IP 归属地判断为异地登录，不核查 VPN 与堡垒机来源

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`归属地`、`异地登录`、`堡垒机`
- `bonusKeywords`：`终端`、`认证链路`、`不核对`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆仅凭归属地判异地登录，故取归属地/异地登录/堡垒机；IP 与 VPN 写法大小写不稳，只放加分。

### DSS02-netsec-7-1 · 只查该账号最近一次登录的主机，其他主机不查

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`最近一次`、`主机`、`跳板机`
- `bonusKeywords`：`数据库`、`域控`、`历史登录`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆只查最近一次登录主机，故取最近一次/主机/跳板机；数据库与域控是它漏掉的排查范围，放加分。

### DSS02-netsec-7-2 · 用入侵检测系统告警直接定位

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`入侵检测`、`告警`、`失陷主机`
- `bonusKeywords`：`访问日志`、`登录日志`、`直接定位`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该混淆只靠入侵检测告警定位失陷主机，故取入侵检测/告警/失陷主机；不核对登录与访问日志放加分。

### DSS02-sysadmin-6-1 · 等账号登录时再强制下线改密

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`会话`、`下次登录`、`强制下线`
- `bonusKeywords`：`不停用`、`改密`、`等待`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆把停用推迟到下次登录，故取会话/下次登录/强制下线；等待型用 P7，不设时限写法。

### DSS02-sysadmin-6-2 · 把所有运维账号的密码都重置一遍

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`运维账号`、`重置`、`涉事账号`
- `bonusKeywords`：`统一`、`全部`、`不区分`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该混淆不区分账号、对全部运维账号统一重置，故取运维账号/重置/涉事账号；一刀切的边界问题放加分。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S02-1 | 正确 | — | 0 | 运维账号、同时登录、境外地址 | 堡垒机、核心业务系统、登录终端 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S02-2 | 正确 | F12 | 8 | 账号归属、登录日志、堡垒机 | 本人确认、异地登录、登录终端 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S02-3 | 正确 | F3 | 8 | 数据外泄、患者安全、较大 | Ⅲ级、扩散趋势、电子病历 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S02-4 | 正确 | F4 | 7 | 简报、失陷账号、启动 | Ⅲ级、渗透路径、10分钟、已采取措施 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S02-5 | 正确 | F9 | 8 | 启动令、授权范围\|授权权限\|权限范围、响应级别\|响应等级\|响应分级 | 启动时间、停用账号、隔离主机、取证 | 30/18 | 对象、时间 | P8 | 0.25/0.30 |
| S02-6 | 正确 | F9 | 7 | 涉事账号、会话、授权 | 回收授权、踢出会话、重置口令、操作记录 | 40/25 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S02-7 | 正确 | F9 | 7 | 失陷主机、跳板机、域控 | 数据库、访问日志、失陷清单 | 40/25 | 对象、动作、结论 | P1 | 0.20/0.30 |
| S02-8 | 正确 | F2 | 6 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、影响范围、回执 | 2小时、已采取措施、先电话后书面 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S02-9 | 正确 | F2 | 6 | 市网信办\|网信办\|市委网信办、舆情风险、回执 | 同一时点、不等回复、攻击来源、事件编号 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S02-10 | 正确 | F10 | 8 | 攻击源、封堵、失陷主机 | 高风险网段、交换机端口、内存、变更记录 | 35/20 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S02-11 | 正确 | F13 | 8 | 登录日志、堡垒机、哈希 | 内存、样本、保全位置、时间戳 | 45/28 | 对象、时间、结论 | P5 | 0.25/0.30 |
| S02-12 | 正确 | F5 | 7 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、受理编号\|受案编号\|案件编号\|回执编号 | 不等待、事件经过、联系人、书面材料 | 30/18 | 对象、时间 | P4 | 0.20/0.30 |
| S02-13 | 正确 | F8 | 6 | 横向路径、全量复核\|全面复核\|全量核对\|逐条核对、闭环\|闭环管理\|办结\|归档 | 取证完成、报案闭环、逐条核对、Ⅲ级 | 35/20 | 对象、结论 | P8 | 0.20/0.25 |
| S02-14 | 正确 | — | 6 | 受理回执\|受案回执\|回执单\|回执、受理编号\|受案编号\|案件编号\|回执编号、材料清单 | 取证要求、提交时限、对接人、转指挥部 | 30/18 | 无 | P4 | 0.20/0.25 |
| S02-15 | 正确 | F9 | 8 | 权限回收、会话、核心业务 | 未恢复系统、遗留风险、挂号、收费 | 40/25 | 对象、动作、结论 | P6 | 0.25/0.30 |
| S02-16 | 正确 | — | 6 | 时间线、整改、归档 | 关键决策点、处置得失、责任人、闭环 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS02-commander-13-1 | 混淆 | — | 3 | 涉事主机、重装系统、一周 | 后门残留、结束、观察期 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS02-commander-13-2 | 混淆 | — | 8 | 账号密码、响应结束、验证 | Ⅲ级、取证、上报闭环 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS02-commander-5-1 | 混淆 | — | 8 | 信息科、自行处置、指挥部 | 启动令、授权范围、不介入 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS02-commander-5-2 | 混淆 | — | 8 | 市网信办、处置意见、启动 | Ⅲ级、等待、响应 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-duty-1-1 | 混淆 | — | 5 | 值班表、本人操作、关闭告警 | 源IP、登录终端、排班 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS02-duty-1-2 | 混淆 | — | 8 | 抓包、下次登录、来源 | 流量特征、不处置、等待 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-duty-2-1 | 混淆 | — | 5 | 账号本人、自行检查、登录日志 | 中毒、堡垒机、不再核查 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS02-itlead-12-1 | 混淆 | — | 3 | 内部处理、报案、市网安大队 | 流程长、影响大、不报案 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS02-itlead-12-2 | 混淆 | — | 8 | 口头报案、电话、材料 | 联系人、补交、后续 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS02-itlead-3-1 | 混淆 | — | 5 | 异常账号、统计、规模 | 数量、攻击源、失陷主机 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS02-itlead-3-2 | 混淆 | — | 3 | 最高等级、影响评估、启动 | 停机、上报范围、Ⅰ级 | 15/10 | 对象 | P3 | 0.10/0.15 |
| DSS02-itlead-4-1 | 混淆 | — | 5 | 账号清单、登录分析、指挥部 | 详细、延误、上报时机 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS02-itlead-4-2 | 混淆 | — | 8 | 内部讨论、影响范围、上报 | 指挥部、讨论清楚、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-itlead-8-1 | 混淆 | — | 8 | 市网信办、技术意见、市卫生健康局 | 等待、一并报送、卫健 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-itlead-8-2 | 混淆 | — | 5 | 网络安全管理员、技术岗、对接 | 指挥链、绕过、报送 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS02-itlead-9-1 | 混淆 | — | 5 | 账号被盗、上报、风险 | 影响范围、横向渗透、初报 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS02-itlead-9-2 | 混淆 | — | 8 | 改密、市网信办、报送 | 一次性、等待、全部完成 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-netsec-10-1 | 混淆 | — | 8 | 密码、隔离、主机 | 重置密码、外联通道、未隔离 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS02-netsec-10-2 | 混淆 | — | 5 | 重要数据、拷出、隔离 | 失陷主机、易失证据、拷贝 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS02-netsec-11-1 | 混淆 | — | 5 | 登录日志、设备日志、保全 | 堡垒机、主机日志、暂不 | 15/10 | 对象 | P5 | 0.15/0.20 |
| DSS02-netsec-11-2 | 混淆 | — | 8 | 事件经过、恢复、日志 | 样本、后续补、取证 | 20/12 | 对象 | P6 | 0.15/0.25 |
| DSS02-netsec-2-2 | 混淆 | — | 5 | 归属地、异地登录、堡垒机 | 终端、认证链路、不核对 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS02-netsec-7-1 | 混淆 | — | 5 | 最近一次、主机、跳板机 | 数据库、域控、历史登录 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS02-netsec-7-2 | 混淆 | — | 5 | 入侵检测、告警、失陷主机 | 访问日志、登录日志、直接定位 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS02-sysadmin-6-1 | 混淆 | — | 8 | 会话、下次登录、强制下线 | 不停用、改密、等待 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS02-sysadmin-6-2 | 混淆 | — | 3 | 运维账号、重置、涉事账号 | 统一、全部、不区分 | 15/10 | 对象 | P2 | 0.10/0.15 |

---

## 4. 总量、分布与风险

### 4.1 S02 规则总量

- 节点 42：正确 16、混淆 26；规则条数 42，与节点一一对应，无遗漏无多余。
- 必含词合计 126 个，平均 3.0 个/节点；加分词合计 137 个。
- 长度档：15/10 共 15 个；20/12 共 11 个；30/18 共 6 个；35/20 共 4 个；40/25 共 5 个；45/28 共 1 个。
- 要素组合：对象 27 个；对象+动作 1 个；对象+动作+结论 5 个；对象+时间+结论 2 个；对象+时间 2 个；对象+结论 2 个；对象+动作+时间 3 个。
- 表单绑定（正确节点）：F10 1 个、F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F8 1 个、F9 4 个、无表单 3 个。

### 4.2 风险最高的几类节点

1. **混淆节点（26 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S02-3`（研判定级，8 分）：必含词是 `定级依据`/`外联通道`/`横向`。参演人若只写“研判为较大（Ⅲ级）事件”，既没写“是否已建立外联通道”也没写“内网是否被横向利用”，会一次缺两项。等级写法（Ⅲ级/较大）只放加分词，是对的，但演练提示里应明确要求写“外联通道”和“横向”这两个判据。
3. `S02-13`（应急结束，6 分）：必含词含 `全量复核`、`闭环` 这类复合词，写成“账号已复核”“上报已完成”不会命中。结束节点文本往往很短，这是本册最容易“写对了却不计分”的节点。
4. `S02-1`（0 分告警首报）：不产生扣分，但规则仍是提交闸门，缺词照旧 400；它的必含词 `异常登录`/`堡垒机`/`境外地址` 与标题里的“异常账号多地同时登录”不完全一致，提示中需点明三个事实。
5. `S02-14`（受理回执，无表单）：记录的是“市网安大队的要求”，不是本院动作，要素已降为仅 `对象`；若参演人只写“已收到回执”，长度与要素两道闸门都会拦。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（42 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S02-1": {
    "requiredKeywords": [
      "运维账号",
      "同时登录",
      "境外地址"
    ],
    "bonusKeywords": [
      "堡垒机",
      "核心业务系统",
      "登录终端"
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
  "S02-2": {
    "requiredKeywords": [
      "账号归属",
      "登录日志",
      "堡垒机"
    ],
    "bonusKeywords": [
      "本人确认",
      "异地登录",
      "登录终端"
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
  "S02-3": {
    "requiredKeywords": [
      "数据外泄",
      "患者安全",
      "较大"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "扩散趋势",
      "电子病历"
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
  "S02-4": {
    "requiredKeywords": [
      "简报",
      "失陷账号",
      "启动"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "渗透路径",
      "10分钟",
      "已采取措施"
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
  "S02-5": {
    "requiredKeywords": [
      "启动令",
      "授权范围|授权权限|权限范围",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "启动时间",
      "停用账号",
      "隔离主机",
      "取证"
    ],
    "minMeasureLen": 30,
    "minResultLen": 18,
    "requiredElements": [
      "对象",
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
  "S02-6": {
    "requiredKeywords": [
      "涉事账号",
      "会话",
      "授权"
    ],
    "bonusKeywords": [
      "回收授权",
      "踢出会话",
      "重置口令",
      "操作记录"
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
  "S02-7": {
    "requiredKeywords": [
      "失陷主机",
      "跳板机",
      "域控"
    ],
    "bonusKeywords": [
      "数据库",
      "访问日志",
      "失陷清单"
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
  "S02-8": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "影响范围",
      "回执"
    ],
    "bonusKeywords": [
      "2小时",
      "已采取措施",
      "先电话后书面"
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
  "S02-9": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "舆情风险",
      "回执"
    ],
    "bonusKeywords": [
      "同一时点",
      "不等回复",
      "攻击来源",
      "事件编号"
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
  "S02-10": {
    "requiredKeywords": [
      "攻击源",
      "封堵",
      "失陷主机"
    ],
    "bonusKeywords": [
      "高风险网段",
      "交换机端口",
      "内存",
      "变更记录"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S02-11": {
    "requiredKeywords": [
      "登录日志",
      "堡垒机",
      "哈希"
    ],
    "bonusKeywords": [
      "内存",
      "样本",
      "保全位置",
      "时间戳"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S02-12": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "受理编号|受案编号|案件编号|回执编号"
    ],
    "bonusKeywords": [
      "不等待",
      "事件经过",
      "联系人",
      "书面材料"
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
    "keywordPenaltyRatio": 0.3
  },
  "S02-13": {
    "requiredKeywords": [
      "横向路径",
      "全量复核|全面复核|全量核对|逐条核对",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "取证完成",
      "报案闭环",
      "逐条核对",
      "Ⅲ级"
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
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.25
  },
  "S02-14": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "受理编号|受案编号|案件编号|回执编号",
      "材料清单"
    ],
    "bonusKeywords": [
      "取证要求",
      "提交时限",
      "对接人",
      "转指挥部"
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
  "S02-15": {
    "requiredKeywords": [
      "权限回收",
      "会话",
      "核心业务"
    ],
    "bonusKeywords": [
      "未恢复系统",
      "遗留风险",
      "挂号",
      "收费"
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
      "已恢复",
      "已处理",
      "正常",
      "已完成",
      "ok",
      "OK"
    ],
    "fillerMaxLen": 8,
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S02-16": {
    "requiredKeywords": [
      "时间线",
      "整改",
      "归档"
    ],
    "bonusKeywords": [
      "关键决策点",
      "处置得失",
      "责任人",
      "闭环"
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
  "DSS02-commander-13-1": {
    "requiredKeywords": [
      "涉事主机",
      "重装系统",
      "一周"
    ],
    "bonusKeywords": [
      "后门残留",
      "结束",
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS02-commander-13-2": {
    "requiredKeywords": [
      "账号密码",
      "响应结束",
      "验证"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "取证",
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
  "DSS02-commander-5-1": {
    "requiredKeywords": [
      "信息科",
      "自行处置",
      "指挥部"
    ],
    "bonusKeywords": [
      "启动令",
      "授权范围",
      "不介入"
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
  "DSS02-commander-5-2": {
    "requiredKeywords": [
      "市网信办",
      "处置意见",
      "启动"
    ],
    "bonusKeywords": [
      "Ⅲ级",
      "等待",
      "响应"
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
  "DSS02-duty-1-1": {
    "requiredKeywords": [
      "值班表",
      "本人操作",
      "关闭告警"
    ],
    "bonusKeywords": [
      "源IP",
      "登录终端",
      "排班"
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
  "DSS02-duty-1-2": {
    "requiredKeywords": [
      "抓包",
      "下次登录",
      "来源"
    ],
    "bonusKeywords": [
      "流量特征",
      "不处置",
      "等待"
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
  "DSS02-duty-2-1": {
    "requiredKeywords": [
      "账号本人",
      "自行检查",
      "登录日志"
    ],
    "bonusKeywords": [
      "中毒",
      "堡垒机",
      "不再核查"
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
  "DSS02-itlead-12-1": {
    "requiredKeywords": [
      "内部处理",
      "报案",
      "市网安大队"
    ],
    "bonusKeywords": [
      "流程长",
      "影响大",
      "不报案"
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
  "DSS02-itlead-12-2": {
    "requiredKeywords": [
      "口头报案",
      "电话",
      "材料"
    ],
    "bonusKeywords": [
      "联系人",
      "补交",
      "后续"
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
  "DSS02-itlead-3-1": {
    "requiredKeywords": [
      "异常账号",
      "统计",
      "规模"
    ],
    "bonusKeywords": [
      "数量",
      "攻击源",
      "失陷主机"
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
  "DSS02-itlead-3-2": {
    "requiredKeywords": [
      "最高等级",
      "影响评估",
      "启动"
    ],
    "bonusKeywords": [
      "停机",
      "上报范围",
      "Ⅰ级"
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS02-itlead-4-1": {
    "requiredKeywords": [
      "账号清单",
      "登录分析",
      "指挥部"
    ],
    "bonusKeywords": [
      "详细",
      "延误",
      "上报时机"
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
  "DSS02-itlead-4-2": {
    "requiredKeywords": [
      "内部讨论",
      "影响范围",
      "上报"
    ],
    "bonusKeywords": [
      "指挥部",
      "讨论清楚",
      "延误"
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
  "DSS02-itlead-8-1": {
    "requiredKeywords": [
      "市网信办",
      "技术意见",
      "市卫生健康局"
    ],
    "bonusKeywords": [
      "等待",
      "一并报送",
      "卫健"
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
  "DSS02-itlead-8-2": {
    "requiredKeywords": [
      "网络安全管理员",
      "技术岗",
      "对接"
    ],
    "bonusKeywords": [
      "指挥链",
      "绕过",
      "报送"
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
  "DSS02-itlead-9-1": {
    "requiredKeywords": [
      "账号被盗",
      "上报",
      "风险"
    ],
    "bonusKeywords": [
      "影响范围",
      "横向渗透",
      "初报"
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
  "DSS02-itlead-9-2": {
    "requiredKeywords": [
      "改密",
      "市网信办",
      "报送"
    ],
    "bonusKeywords": [
      "一次性",
      "等待",
      "全部完成"
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
  "DSS02-netsec-10-1": {
    "requiredKeywords": [
      "密码",
      "隔离",
      "主机"
    ],
    "bonusKeywords": [
      "重置密码",
      "外联通道",
      "未隔离"
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
  "DSS02-netsec-10-2": {
    "requiredKeywords": [
      "重要数据",
      "拷出",
      "隔离"
    ],
    "bonusKeywords": [
      "失陷主机",
      "易失证据",
      "拷贝"
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
  "DSS02-netsec-11-1": {
    "requiredKeywords": [
      "登录日志",
      "设备日志",
      "保全"
    ],
    "bonusKeywords": [
      "堡垒机",
      "主机日志",
      "暂不"
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
    "textQualityRatio": 0.15,
    "keywordPenaltyRatio": 0.2
  },
  "DSS02-netsec-11-2": {
    "requiredKeywords": [
      "事件经过",
      "恢复",
      "日志"
    ],
    "bonusKeywords": [
      "样本",
      "后续补",
      "取证"
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
  "DSS02-netsec-2-2": {
    "requiredKeywords": [
      "归属地",
      "异地登录",
      "堡垒机"
    ],
    "bonusKeywords": [
      "终端",
      "认证链路",
      "不核对"
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
  "DSS02-netsec-7-1": {
    "requiredKeywords": [
      "最近一次",
      "主机",
      "跳板机"
    ],
    "bonusKeywords": [
      "数据库",
      "域控",
      "历史登录"
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
  "DSS02-netsec-7-2": {
    "requiredKeywords": [
      "入侵检测",
      "告警",
      "失陷主机"
    ],
    "bonusKeywords": [
      "访问日志",
      "登录日志",
      "直接定位"
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
  "DSS02-sysadmin-6-1": {
    "requiredKeywords": [
      "会话",
      "下次登录",
      "强制下线"
    ],
    "bonusKeywords": [
      "不停用",
      "改密",
      "等待"
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
  "DSS02-sysadmin-6-2": {
    "requiredKeywords": [
      "运维账号",
      "重置",
      "涉事账号"
    ],
    "bonusKeywords": [
      "统一",
      "全部",
      "不区分"
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  }
}
```
