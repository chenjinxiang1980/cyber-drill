# S03 处置文字质量规则清单（42 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S03 全部 42 个节点（correct 16 / distractor 26），剧本「DDoS 攻击致门诊收费卡顿」
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
| T1 轻量 | 15/10 | DSS03-commander-13-1、DSS03-duty-1-1、DSS03-duty-1-2、DSS03-itlead-11-1 |
| T2 标准 | 20/12 | DSS03-commander-13-2、DSS03-commander-5-1、DSS03-commander-5-2、DSS03-itlead-11-2 |
| T3 记录 | 30/18 | S03-1、S03-11、S03-15、S03-5 |
| T4 关键 | 35/20 | S03-13、S03-3、S03-4 |
| T5 复杂 | 40/25 | S03-10、S03-12、S03-16、S03-2 |
| T6 高复杂 | 45/28 | S03-14 |
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

### S03-1 · 门诊收费系统访问缓慢告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`响应时间`、`并发连接`、`带宽曲线`
- `bonusKeywords`：`首次发现时间`、`昨日同时段`、`业务高峰`、`出口带宽`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报要拿出响应时间、并发连接与出口带宽曲线三组可比数据，并给出与昨日同时段的对比结论，故取这三词；该节点 0 分，规则只作提交闸门。

### S03-2 · 核实流量特征与受影响业务

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`连接状态`、`请求速率`、`受影响范围`
- `bonusKeywords`：`源地址`、`出口带宽`、`挂号`、`医保结算`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实节点要区分 SYN Flood、CC 与带宽占满，可核查判据落在连接状态与请求速率，并把挂号、收费、医保结算的可用性归到受影响范围。

### S03-3 · 预警研判：确认较大（Ⅲ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`中断时长`、`纸质兜底`、`较大`
- `bonusKeywords`：`门诊量`、`科室范围`、`Ⅲ级`、`区分理由`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级要写清中断时长、影响窗口与是否需要纸质兜底三项依据，并用中文“较大”而非罗马数字定档，避免 Ⅲ/III/3 写法漏命中。

### S03-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`峰值带宽`、`门诊量`、`启动`
- `bonusKeywords`：`事件等级`、`已采取措施`、`授权范围`、`10分钟`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：院内报告的硬信息是峰值带宽与受影响门诊量，落脚点是请求启动Ⅲ级响应并要授权（清洗/限速/兜底/上报）；不要求结论，避免把“建议启动”硬写成“已启动”。

### S03-5 · 院应急指挥部批准启动Ⅲ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`授权范围|授权权限|权限范围`、`启动时间`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`岗位职责`、`集合时限`、`清洗`、`限速`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：审批留痕的三件事是响应级别、启动时间与授权范围；加分词奖励把岗位职责与集合时限也写进去的人，防止只写一句“同意启动”。

### S03-6 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`中断时长`、`兜底措施`
- `bonusKeywords`：`2小时`、`门诊量`、`联系人`、`上报表`、`同时报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报卫健局必须落到中断时长与已启用的手工兜底措施，并写明联系人；时限“2 小时”因全半角与空格写法差异只作加分词；“同时报”属报送质量表现，与同类双报节点一致放加分词。

### S03-7 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`攻击类型`、`峰值流量`、`源地址|源IP|源IP段|攻击源`
- `bonusKeywords`：`清洗`、`封禁`、`上报表`、`续报`、`同时报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报网信办要讲清攻击类型、峰值流量与源地址三项；续报节奏属后续安排、“同时报”属报送质量，统一放加分词；源地址按 descr 的“源 IP 段特征”扩为同义组，清洗/封禁/上报表仍作加分。

### S03-8 · 启用流量清洗与限速

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`抓包`、`清洗`、`误封`
- `bonusKeywords`：`策略日志`、`先观察后生效`、`收费`、`策略清单`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：清洗节点的判据不是“洗过”，而是清洗前已抓包留证、策略先观察后生效且未误封正常患者，故取抓包/清洗/误封三词。

### S03-9 · 业务兜底：门诊收费启用手工流程

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`手工收费`、`医保补录|医保补传|医保补记`、`回切|回退|切回`
- `bonusKeywords`：`退费`、`流程指引`、`告知患者`、`启动阈值`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：门诊兜底要同时覆盖手工收费、退费与医保补录三张清单，并写清切换与回切时点；验收动作是“兜底”，要素不放动作，改用对象+时间+结论。

### S03-10 · 溯源攻击源并封禁 IP

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`攻击源`、`封禁`、`合作方`
- `bonusKeywords`：`归属`、`峰值时间线`、`解封条件`、`设备日志`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：溯源封禁的可核查点是攻击源清单、按段封禁，以及封禁前核对合作方与医保专线出口这一边界，故三词落在攻击源/封禁/合作方。

### S03-11 · 向市网安大队报案并提交证据材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`受理编号|受案编号|案件编号|回执编号`
- `bonusKeywords`：`先行`、`抓包`、`哈希`、`补充材料`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：报案要先行提交事件经过与源 IP 清单，并以受理编号作为受理凭据；要素只留对象与时间，避免把“已受理”当成参演人能保证的结论。

### S03-12 · 业务恢复验证与数据核对

`correct` ｜ 类型 `task` ｜ 表单 `F7` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`交易流水`、`逐笔核对|逐笔比对|逐笔校验`、`漏费错费`
- `bonusKeywords`：`医保补传`、`并发恢复曲线`、`责任人`、`时限`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：恢复验证的核心是高峰时段交易流水逐笔核对并出漏费错费清单，医保补传与并发恢复曲线是补强项；缺这两份清单就不能判定恢复通过。

### S03-13 · 应急结束：院应急指挥部宣布Ⅲ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`业务验证`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`攻击停止`、`证据固定`、`复盘`、`报备`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束要逐条对上四项条件：攻击停止满 2 小时、业务验证通过、证据固定完成、上报与报案闭环；用“业务验证/闭环”规避时长写法差异。

### S03-14 · 证据保全：抓包、清洗设备日志与源 IP 清单

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`抓包`、`日志`、`哈希`
- `bonusKeywords`：`镜像`、`源地址清单`、`时间线`、`保管人`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：取证必须在清洗与封禁之前完成抓包与清洗设备、防火墙日志导出，并用哈希逐项收口；源 IP 清单因大小写与空格写法不一，只放加分词。

### S03-15 · 市网安大队受理回执与补充材料要求

`correct` ｜ 类型 `report` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`受理编号|受案编号|案件编号|回执编号`、`取证要求`
- `bonusKeywords`：`受理人`、`补充材料`、`报案闭环`、`签收`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：该节点记录的是对方回执：受理编号、受理人与取证要求，再按清单补齐流量证据；文本天然缺本院处置动作，故只要求对象要素。

### S03-16 · 复盘总结：攻击特征、处置得失与整改

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 300 min

- `requiredKeywords`：`时间线`、`取证顺序`、`整改`
- `bonusKeywords`：`根因`、`复测`、`责任人`、`得失`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘按时间线还原发现与上报时延，重点检讨“先止损还是先取证”的顺序，并把问题落到整改项与责任人，区别于只写“整改计划”的空泛总结。

---

## 2. 混淆节点规则（26 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS03-commander-13-1 · 等业务完全恢复 24 小时再宣布

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`观察`、`二次攻击`、`结束`
- `bonusKeywords`：`24小时`、`过度延迟`、`应急资源`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：错误动作是强行观察一天再结束，按其自述必然出现观察期、无二次攻击的判断与结束动作；3 分混淆取 T1，保证照 descr 写就能提交。

### DSS03-commander-13-2 · 业务恢复了就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`业务恢复`、`结束`、`攻击停止`
- `bonusKeywords`：`证据固定`、`上报闭环`、`二次攻击`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：“能用就结束”的合理写法只讲业务恢复与宣布结束，最多补一句攻击已停；证据固定与上报闭环是被它省略的后果，只作加分。

### DSS03-commander-5-1 · 先让网络安全管理员处理，指挥部等结果

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网络安全管理员`、`指挥部`、`不启动`
- `bonusKeywords`：`不调度`、`等待结果`、`责任`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆的写法是把网络问题交给网络安全管理员、指挥部暂不启动，必含取这两个主体加“不启动”这一消极动作；不把“失职”这类评价词设必含。

### DSS03-commander-5-2 · 等收费真的全停了再启动

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`收费全停`、`启动`、`阈值`
- `bonusKeywords`：`延误`、`业务兜底`、`再等等`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：以“收费全停”为启动阈值的写法，必然同时出现阈值口径、收费状态与启动动作；8 分混淆仍按 T2，不因分值上调长度。

### DSS03-duty-1-1 · 先重启收费服务器看能不能缓解

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`收费服务器`、`重启`、`卡顿`
- `bonusKeywords`：`攻击痕迹`、`二次中断`、`快速恢复`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先重启的写法就是“对收费服务器重启以缓解卡顿”，三词都在正常的处置叙述里；不取“掩盖痕迹”这类只有专家才会补写的后果。

### DSS03-duty-1-2 · 先让各窗口反馈"还能用吗"

`distractor` ｜ 分值 3 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`窗口`、`口头询问`、`可用`
- `bonusKeywords`：`监控数据`、`主观`、`反馈`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只问窗口的写法会写“向收费窗口口头询问是否可用”，不含任何监控数据；3 分混淆取 T1，避免字数门槛卡死提交。

### DSS03-itlead-11-1 · 不报案，DDoS 很常见

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`不报案`、`常见`、`网安大队`
- `bonusKeywords`：`立案`、`舆情`、`自行处置`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该混淆的落点是“攻击很常见、不向网安大队报案”，判据是常见与否；不把 DDoS 大小写写法设必含，避免字面漏命中。

### DSS03-itlead-11-2 · 仅口头报案，不提交流量证据材料

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`电话`、`材料`
- `bonusKeywords`：`流量证据`、`日志`、`立案`、`取证链`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：仅口头报案的写法是电话说一声、材料以后再补，必含落在电话/口头报案/材料三个词；证据类词留作加分，奖励写清缺什么的人。

### DSS03-itlead-3-1 · 只按中断时长定级，不看影响范围与兜底需求

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`收费中断`、`单一条件`、`时长`
- `bonusKeywords`：`影响窗口`、`门诊量`、`纸质兜底`、`2小时`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只按时段判定的写法会写“按收费中断这一单一条件卡时长定级”，三词都是其自述用语；影响窗口与纸质兜底是它漏掉的项，放加分词。

### DSS03-itlead-3-2 · 等网信办给判断再定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`定级`、`等待`
- `bonusKeywords`：`响应空转`、`外部判断`、`准确`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：把定级权外移的写法是“由网信办判断、等待回复”，三词分别覆盖对象、动作与状态；不把“空转”这种评价词设必含。

### DSS03-itlead-4-1 · 等流量缓解一些再报，显得更专业

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`延后上报`、`处置进展`、`时限`
- `bonusKeywords`：`启动`、`已采取措施`、`更完整`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等有进展再报的写法必然出现延后上报、处置进展与时限三个词；加分词奖励写出“有已采取措施更完整”这一自我合理化理由。

### DSS03-itlead-4-2 · 先报"网络故障"不明确说是 DDoS

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网络故障`、`笼统`、`上报`
- `bonusKeywords`：`隐瞒`、`研判`、`支援`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先报网络故障的写法是“笼统上报网络故障、原因待查”，必含只取笼统/网络故障/上报；不取攻击类型字样，因为那正是被隐去的内容。

### DSS03-itlead-6-1 · 等网信办的意见出来了再同步报卫健局

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`卫健局`、`意见`
- `bonusKeywords`：`2小时`、`前置条件`、`一起报`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该混淆的写法是等网信办技术意见出来再一起报卫健局，两个单位加“意见”即可命中；时限与前置条件留作加分。

### DSS03-itlead-6-2 · 只报"收费系统故障"不说是 DDoS

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`收费系统故障`、`事件性质`、`上报`
- `bonusKeywords`：`卫健局`、`弱化`、`补报`、`失真`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只报“收费系统故障”的写法会写上报对象、所报口径与未说明事件性质；不把攻击类型设必含，因为它正是被隐去的内容。

### DSS03-itlead-7-1 · 只发初报，后续进展不再续报

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`首次上报`、`续报`、`进展`
- `bonusKeywords`：`2小时`、`交差`、`恢复情况`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只发一次上报的错误动作，其合理写法会出现首次上报、续报与进展三个词；5 分混淆取 T1，避免与“选错扣全分”形成双重惩罚。

### DSS03-itlead-7-2 · 等处置完了再一次性报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`处置结束`、`一次性上报`、`滞后`
- `bonusKeywords`：`时限`、`完整情况`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等打完再一次性上报的写法，对象是处置结束这个时点，判据是信息滞后；不把“错过时限”设必含，那是后果评价而非其自述。

### DSS03-medlead-9-1 · 先通知只收现金，等刷卡恢复

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`现金`、`退费`、`医保补录`
- `bonusKeywords`：`刷卡`、`兜底`、`窗口`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只收现金的写法会写“通知窗口只收现金、退费与医保补录另行处理”，三词都在其叙述里；不把“不完整”这种评价词设必含。

### DSS03-medlead-9-2 · 先不切手工，让窗口继续尝试

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`手工流程`、`继续尝试`、`窗口`
- `bonusKeywords`：`刷卡`、`积压`、`患者聚集`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：不切手工的写法是让窗口继续尝试刷卡、暂不启用手工流程；积压与患者聚集是后果词，只作加分，避免迫使人写未发生的事。

### DSS03-netsec-10-1 · 先把攻击流量清洗掉，溯源以后再说

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`先清洗`、`溯源`、`样本`
- `bonusKeywords`：`源地址`、`灭失`、`业务优先`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先救业务的写法是“先清洗、溯源以后再说”，并可能提到样本灭失风险；必含取动作顺序与对象，不取“以后”这类虚词。

### DSS03-netsec-10-2 · 只让运营商封 IP，自己不抓包不留证

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`运营商`、`封禁`、`上游`
- `bonusKeywords`：`抓包`、`清洗设备日志`、`源地址清单`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：把封禁交给运营商的写法会写上游封禁与运营商配合，内网抓包与日志是其省略项，只进加分；必含三词都能由其自述自然写出。

### DSS03-netsec-2-1 · 只看带宽占用，不核对协议特征与业务可用性

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`带宽曲线`、`流量`、`占用`
- `bonusKeywords`：`协议`、`收费可用`、`并发`、`流量采集`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只凭 NTOP 看带宽的写法会写“看带宽曲线、出口流量占用升高”，三词都在正常叙述里；协议特征与收费可用性是被省略的项，只作加分。

### DSS03-netsec-2-2 · 重启防火墙让缓存清一下

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`边界防火墙`、`重启`、`缓存`
- `bonusKeywords`：`业务中断`、`无效`、`卡顿`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：重启防火墙清缓存的写法，设备、动作与理由三词都在其自述里；不把“对 DDoS 无效”设必含，那是它没意识到的判断。

### DSS03-netsec-8-1 · 直接封禁当前占带宽最高的 IP

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`带宽`、`封禁`、`最高`
- `bonusKeywords`：`误封`、`合作方`、`可逆性`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：“谁占带宽封谁”的写法会写封禁带宽占用最高的地址；误封与合作方出口是它没考虑的边界，只放加分词。

### DSS03-netsec-8-2 · 让业务走备用线路绕过去

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`备用线路`、`业务`、`带宽`
- `bonusKeywords`：`拖延`、`未解决`、`切换`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P10 兜底切换档，`fillerMaxLen` 8）：无、已切换、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：让业务走备用线路的写法，三词落在备用线路、业务切换与备用带宽有限这一理由上；不把“未解决攻击”设必含。

### DSS03-sysadmin-12-1 · 让收费窗口自己试一下能不能刷

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`试刷`、`窗口`、`恢复`
- `bonusKeywords`：`冒烟`、`数据一致`、`全量`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：让窗口试刷的写法会写“由窗口试刷确认恢复”，冒烟与全量一致性是其省略项，只作加分；三词均可自然写出。

### DSS03-sysadmin-12-2 · 直接上线跑，有问题窗口会反馈

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`直接上线`、`核对`、`回滚`
- `bonusKeywords`：`漏费错费`、`窗口反馈`、`风险`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P6 恢复档，`fillerMaxLen` 8）：无、已恢复、已处理、正常、已完成、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先上线有问题再回滚的写法必然出现直接上线与回滚，核对是被跳过的一步；三词都在参演人解释自己为什么这么做的叙述里。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S03-1 | 正确 | — | 0 | 响应时间、并发连接、带宽曲线 | 首次发现时间、昨日同时段、业务高峰、出口带宽 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S03-2 | 正确 | F12 | 8 | 连接状态、请求速率、受影响范围 | 源地址、出口带宽、挂号、医保结算 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S03-3 | 正确 | F3 | 8 | 中断时长、纸质兜底、较大 | 门诊量、科室范围、Ⅲ级、区分理由 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S03-4 | 正确 | F4 | 7 | 峰值带宽、门诊量、启动 | 事件等级、已采取措施、授权范围、10分钟 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S03-5 | 正确 | F9 | 8 | 授权范围\|授权权限\|权限范围、启动时间、响应级别\|响应等级\|响应分级 | 岗位职责、集合时限、清洗、限速 | 30/18 | 对象、动作、时间 | P8 | 0.25/0.30 |
| S03-6 | 正确 | F2 | 6 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、中断时长、兜底措施 | 2小时、门诊量、联系人、上报表、同时报 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S03-7 | 正确 | F2 | 6 | 攻击类型、峰值流量、源地址\|源IP\|源IP段\|攻击源 | 清洗、封禁、上报表、续报、同时报 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.25 |
| S03-8 | 正确 | F9 | 8 | 抓包、清洗、误封 | 策略日志、先观察后生效、收费、策略清单 | 40/25 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S03-9 | 正确 | F9 | 7 | 手工收费、医保补录\|医保补传\|医保补记、回切\|回退\|切回 | 退费、流程指引、告知患者、启动阈值 | 40/25 | 对象、时间、结论 | P10 | 0.20/0.30 |
| S03-10 | 正确 | F9 | 8 | 攻击源、封禁、合作方 | 归属、峰值时间线、解封条件、设备日志 | 40/25 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S03-11 | 正确 | F5 | 7 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、受理编号\|受案编号\|案件编号\|回执编号 | 先行、抓包、哈希、补充材料 | 30/18 | 对象、时间 | P4 | 0.20/0.30 |
| S03-12 | 正确 | F7 | 7 | 交易流水、逐笔核对\|逐笔比对\|逐笔校验、漏费错费 | 医保补传、并发恢复曲线、责任人、时限 | 40/25 | 对象、动作、结论 | P6 | 0.20/0.30 |
| S03-13 | 正确 | F8 | 6 | 结束条件\|终止条件\|结束标准\|终止标准、业务验证、闭环\|闭环管理\|办结\|归档 | 攻击停止、证据固定、复盘、报备 | 35/20 | 对象、动作、结论 | P8 | 0.20/0.25 |
| S03-14 | 正确 | F13 | 8 | 抓包、日志、哈希 | 镜像、源地址清单、时间线、保管人 | 45/28 | 对象、时间、结论 | P5 | 0.25/0.30 |
| S03-15 | 正确 | — | 6 | 受理回执\|受案回执\|回执单\|回执、受理编号\|受案编号\|案件编号\|回执编号、取证要求 | 受理人、补充材料、报案闭环、签收 | 30/18 | 无 | P4 | 0.20/0.25 |
| S03-16 | 正确 | — | 6 | 时间线、取证顺序、整改 | 根因、复测、责任人、得失 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS03-commander-13-1 | 混淆 | — | 3 | 观察、二次攻击、结束 | 24小时、过度延迟、应急资源 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS03-commander-13-2 | 混淆 | — | 8 | 业务恢复、结束、攻击停止 | 证据固定、上报闭环、二次攻击 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS03-commander-5-1 | 混淆 | — | 8 | 网络安全管理员、指挥部、不启动 | 不调度、等待结果、责任 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-commander-5-2 | 混淆 | — | 8 | 收费全停、启动、阈值 | 延误、业务兜底、再等等 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-duty-1-1 | 混淆 | — | 5 | 收费服务器、重启、卡顿 | 攻击痕迹、二次中断、快速恢复 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS03-duty-1-2 | 混淆 | — | 3 | 窗口、口头询问、可用 | 监控数据、主观、反馈 | 15/10 | 对象 | P1 | 0.10/0.15 |
| DSS03-itlead-11-1 | 混淆 | — | 3 | 不报案、常见、网安大队 | 立案、舆情、自行处置 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS03-itlead-11-2 | 混淆 | — | 8 | 口头报案、电话、材料 | 流量证据、日志、立案、取证链 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS03-itlead-3-1 | 混淆 | — | 5 | 收费中断、单一条件、时长 | 影响窗口、门诊量、纸质兜底、2小时 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS03-itlead-3-2 | 混淆 | — | 8 | 网信办、定级、等待 | 响应空转、外部判断、准确 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-itlead-4-1 | 混淆 | — | 8 | 延后上报、处置进展、时限 | 启动、已采取措施、更完整 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-itlead-4-2 | 混淆 | — | 5 | 网络故障、笼统、上报 | 隐瞒、研判、支援 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS03-itlead-6-1 | 混淆 | — | 8 | 网信办、卫健局、意见 | 2小时、前置条件、一起报 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-itlead-6-2 | 混淆 | — | 5 | 收费系统故障、事件性质、上报 | 卫健局、弱化、补报、失真 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS03-itlead-7-1 | 混淆 | — | 5 | 首次上报、续报、进展 | 2小时、交差、恢复情况 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS03-itlead-7-2 | 混淆 | — | 8 | 处置结束、一次性上报、滞后 | 时限、完整情况、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-medlead-9-1 | 混淆 | — | 5 | 现金、退费、医保补录 | 刷卡、兜底、窗口 | 15/10 | 对象 | P10 | 0.15/0.20 |
| DSS03-medlead-9-2 | 混淆 | — | 8 | 手工流程、继续尝试、窗口 | 刷卡、积压、患者聚集 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS03-netsec-10-1 | 混淆 | — | 5 | 先清洗、溯源、样本 | 源地址、灭失、业务优先 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS03-netsec-10-2 | 混淆 | — | 8 | 运营商、封禁、上游 | 抓包、清洗设备日志、源地址清单 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS03-netsec-2-1 | 混淆 | — | 5 | 带宽曲线、流量、占用 | 协议、收费可用、并发、流量采集 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS03-netsec-2-2 | 混淆 | — | 8 | 边界防火墙、重启、缓存 | 业务中断、无效、卡顿 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS03-netsec-8-1 | 混淆 | — | 5 | 带宽、封禁、最高 | 误封、合作方、可逆性 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS03-netsec-8-2 | 混淆 | — | 5 | 备用线路、业务、带宽 | 拖延、未解决、切换 | 15/10 | 对象 | P10 | 0.15/0.20 |
| DSS03-sysadmin-12-1 | 混淆 | — | 5 | 试刷、窗口、恢复 | 冒烟、数据一致、全量 | 15/10 | 对象 | P6 | 0.15/0.20 |
| DSS03-sysadmin-12-2 | 混淆 | — | 8 | 直接上线、核对、回滚 | 漏费错费、窗口反馈、风险 | 20/12 | 对象 | P6 | 0.15/0.25 |

---

## 4. 总量、分布与风险

### 4.1 S03 规则总量

- 节点 42：正确 16、混淆 26；规则条数 42，与节点一一对应，无遗漏无多余。
- 必含词合计 126 个，平均 3.0 个/节点；加分词合计 146 个。
- 长度档：15/10 共 14 个；20/12 共 12 个；30/18 共 6 个；35/20 共 3 个；40/25 共 6 个；45/28 共 1 个。
- 要素组合：对象 27 个；对象+动作 1 个；对象+动作+结论 5 个；对象+时间 1 个；对象+时间+结论 3 个；对象+结论 1 个；对象+动作+时间 4 个。
- 表单绑定（正确节点）：F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F7 1 个、F8 1 个、F9 4 个、无表单 3 个。

### 4.2 风险最高的几类节点

1. **混淆节点（26 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S03-3`（研判定级，8 分）：等级词只能用中文“较大”，必含词里不含 `Ⅲ级`。照标题写“Ⅲ级事件”而不写“较大”的参演人会缺一项，演练前需说明等级用中文表述。
3. `S03-9`（业务兜底，7 分）：必含 `回切|回退|切回`（第二批已加同义组）。“恢复系统”仍不命中，建议提示中给出“回切/回退/切回”三种写法之一。
4. `S03-8`（流量清洗与限速，8 分）：清洗动作在引擎动作词表里只有 `阻断`/`拦截`/`限速` 不是词表词，要素未放“动作”；参演人若只描述“开启清洗”不写对象，仍可能缺要素。
5. `S03-14`（证据保全，8 分）：`源 IP 清单` 这类写法含空格与字母，只作加分词；必含词用 `抓包`/`设备日志`/`证据清单` 等稳定中文短语，但参演人若只写“已保全证据”会缺两项。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（42 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S03-1": {
    "requiredKeywords": [
      "响应时间",
      "并发连接",
      "带宽曲线"
    ],
    "bonusKeywords": [
      "首次发现时间",
      "昨日同时段",
      "业务高峰",
      "出口带宽"
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
  "S03-2": {
    "requiredKeywords": [
      "连接状态",
      "请求速率",
      "受影响范围"
    ],
    "bonusKeywords": [
      "源地址",
      "出口带宽",
      "挂号",
      "医保结算"
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
  "S03-3": {
    "requiredKeywords": [
      "中断时长",
      "纸质兜底",
      "较大"
    ],
    "bonusKeywords": [
      "门诊量",
      "科室范围",
      "Ⅲ级",
      "区分理由"
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
  "S03-4": {
    "requiredKeywords": [
      "峰值带宽",
      "门诊量",
      "启动"
    ],
    "bonusKeywords": [
      "事件等级",
      "已采取措施",
      "授权范围",
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
    "keywordPenaltyRatio": 0.3
  },
  "S03-5": {
    "requiredKeywords": [
      "授权范围|授权权限|权限范围",
      "启动时间",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "岗位职责",
      "集合时限",
      "清洗",
      "限速"
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
  "S03-6": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "中断时长",
      "兜底措施"
    ],
    "bonusKeywords": [
      "2小时",
      "门诊量",
      "联系人",
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
  "S03-7": {
    "requiredKeywords": [
      "攻击类型",
      "峰值流量",
      "源地址|源IP|源IP段|攻击源"
    ],
    "bonusKeywords": [
      "清洗",
      "封禁",
      "上报表",
      "续报",
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
  "S03-8": {
    "requiredKeywords": [
      "抓包",
      "清洗",
      "误封"
    ],
    "bonusKeywords": [
      "策略日志",
      "先观察后生效",
      "收费",
      "策略清单"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S03-9": {
    "requiredKeywords": [
      "手工收费",
      "医保补录|医保补传|医保补记",
      "回切|回退|切回"
    ],
    "bonusKeywords": [
      "退费",
      "流程指引",
      "告知患者",
      "启动阈值"
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
  "S03-10": {
    "requiredKeywords": [
      "攻击源",
      "封禁",
      "合作方"
    ],
    "bonusKeywords": [
      "归属",
      "峰值时间线",
      "解封条件",
      "设备日志"
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S03-11": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "受理编号|受案编号|案件编号|回执编号"
    ],
    "bonusKeywords": [
      "先行",
      "抓包",
      "哈希",
      "补充材料"
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
  "S03-12": {
    "requiredKeywords": [
      "交易流水",
      "逐笔核对|逐笔比对|逐笔校验",
      "漏费错费"
    ],
    "bonusKeywords": [
      "医保补传",
      "并发恢复曲线",
      "责任人",
      "时限"
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
    "textQualityRatio": 0.2,
    "keywordPenaltyRatio": 0.3
  },
  "S03-13": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "业务验证",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "攻击停止",
      "证据固定",
      "复盘",
      "报备"
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
  "S03-14": {
    "requiredKeywords": [
      "抓包",
      "日志",
      "哈希"
    ],
    "bonusKeywords": [
      "镜像",
      "源地址清单",
      "时间线",
      "保管人"
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
  "S03-15": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "受理编号|受案编号|案件编号|回执编号",
      "取证要求"
    ],
    "bonusKeywords": [
      "受理人",
      "补充材料",
      "报案闭环",
      "签收"
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
  "S03-16": {
    "requiredKeywords": [
      "时间线",
      "取证顺序",
      "整改"
    ],
    "bonusKeywords": [
      "根因",
      "复测",
      "责任人",
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
  "DSS03-commander-13-1": {
    "requiredKeywords": [
      "观察",
      "二次攻击",
      "结束"
    ],
    "bonusKeywords": [
      "24小时",
      "过度延迟",
      "应急资源"
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
  "DSS03-commander-13-2": {
    "requiredKeywords": [
      "业务恢复",
      "结束",
      "攻击停止"
    ],
    "bonusKeywords": [
      "证据固定",
      "上报闭环",
      "二次攻击"
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
  "DSS03-commander-5-1": {
    "requiredKeywords": [
      "网络安全管理员",
      "指挥部",
      "不启动"
    ],
    "bonusKeywords": [
      "不调度",
      "等待结果",
      "责任"
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
  "DSS03-commander-5-2": {
    "requiredKeywords": [
      "收费全停",
      "启动",
      "阈值"
    ],
    "bonusKeywords": [
      "延误",
      "业务兜底",
      "再等等"
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
  "DSS03-duty-1-1": {
    "requiredKeywords": [
      "收费服务器",
      "重启",
      "卡顿"
    ],
    "bonusKeywords": [
      "攻击痕迹",
      "二次中断",
      "快速恢复"
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
  "DSS03-duty-1-2": {
    "requiredKeywords": [
      "窗口",
      "口头询问",
      "可用"
    ],
    "bonusKeywords": [
      "监控数据",
      "主观",
      "反馈"
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
    "textQualityRatio": 0.1,
    "keywordPenaltyRatio": 0.15
  },
  "DSS03-itlead-11-1": {
    "requiredKeywords": [
      "不报案",
      "常见",
      "网安大队"
    ],
    "bonusKeywords": [
      "立案",
      "舆情",
      "自行处置"
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
  "DSS03-itlead-11-2": {
    "requiredKeywords": [
      "口头报案",
      "电话",
      "材料"
    ],
    "bonusKeywords": [
      "流量证据",
      "日志",
      "立案",
      "取证链"
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
  "DSS03-itlead-3-1": {
    "requiredKeywords": [
      "收费中断",
      "单一条件",
      "时长"
    ],
    "bonusKeywords": [
      "影响窗口",
      "门诊量",
      "纸质兜底",
      "2小时"
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
  "DSS03-itlead-3-2": {
    "requiredKeywords": [
      "网信办",
      "定级",
      "等待"
    ],
    "bonusKeywords": [
      "响应空转",
      "外部判断",
      "准确"
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
  "DSS03-itlead-4-1": {
    "requiredKeywords": [
      "延后上报",
      "处置进展",
      "时限"
    ],
    "bonusKeywords": [
      "启动",
      "已采取措施",
      "更完整"
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
  "DSS03-itlead-4-2": {
    "requiredKeywords": [
      "网络故障",
      "笼统",
      "上报"
    ],
    "bonusKeywords": [
      "隐瞒",
      "研判",
      "支援"
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
  "DSS03-itlead-6-1": {
    "requiredKeywords": [
      "网信办",
      "卫健局",
      "意见"
    ],
    "bonusKeywords": [
      "2小时",
      "前置条件",
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
  "DSS03-itlead-6-2": {
    "requiredKeywords": [
      "收费系统故障",
      "事件性质",
      "上报"
    ],
    "bonusKeywords": [
      "卫健局",
      "弱化",
      "补报",
      "失真"
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
  "DSS03-itlead-7-1": {
    "requiredKeywords": [
      "首次上报",
      "续报",
      "进展"
    ],
    "bonusKeywords": [
      "2小时",
      "交差",
      "恢复情况"
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
  "DSS03-itlead-7-2": {
    "requiredKeywords": [
      "处置结束",
      "一次性上报",
      "滞后"
    ],
    "bonusKeywords": [
      "时限",
      "完整情况",
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
  "DSS03-medlead-9-1": {
    "requiredKeywords": [
      "现金",
      "退费",
      "医保补录"
    ],
    "bonusKeywords": [
      "刷卡",
      "兜底",
      "窗口"
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
  "DSS03-medlead-9-2": {
    "requiredKeywords": [
      "手工流程",
      "继续尝试",
      "窗口"
    ],
    "bonusKeywords": [
      "刷卡",
      "积压",
      "患者聚集"
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
  "DSS03-netsec-10-1": {
    "requiredKeywords": [
      "先清洗",
      "溯源",
      "样本"
    ],
    "bonusKeywords": [
      "源地址",
      "灭失",
      "业务优先"
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
  "DSS03-netsec-10-2": {
    "requiredKeywords": [
      "运营商",
      "封禁",
      "上游"
    ],
    "bonusKeywords": [
      "抓包",
      "清洗设备日志",
      "源地址清单"
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
  "DSS03-netsec-2-1": {
    "requiredKeywords": [
      "带宽曲线",
      "流量",
      "占用"
    ],
    "bonusKeywords": [
      "协议",
      "收费可用",
      "并发",
      "流量采集"
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
  "DSS03-netsec-2-2": {
    "requiredKeywords": [
      "边界防火墙",
      "重启",
      "缓存"
    ],
    "bonusKeywords": [
      "业务中断",
      "无效",
      "卡顿"
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
  "DSS03-netsec-8-1": {
    "requiredKeywords": [
      "带宽",
      "封禁",
      "最高"
    ],
    "bonusKeywords": [
      "误封",
      "合作方",
      "可逆性"
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
  "DSS03-netsec-8-2": {
    "requiredKeywords": [
      "备用线路",
      "业务",
      "带宽"
    ],
    "bonusKeywords": [
      "拖延",
      "未解决",
      "切换"
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
  "DSS03-sysadmin-12-1": {
    "requiredKeywords": [
      "试刷",
      "窗口",
      "恢复"
    ],
    "bonusKeywords": [
      "冒烟",
      "数据一致",
      "全量"
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
  "DSS03-sysadmin-12-2": {
    "requiredKeywords": [
      "直接上线",
      "核对",
      "回滚"
    ],
    "bonusKeywords": [
      "漏费错费",
      "窗口反馈",
      "风险"
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
  }
}
```
