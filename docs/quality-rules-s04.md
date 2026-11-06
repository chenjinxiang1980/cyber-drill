# S04 处置文字质量规则清单（41 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S04 全部 41 个节点（correct 15 / distractor 26），剧本「钓鱼邮件跳板入侵」
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
| T1 轻量 | 15/10 | DSS04-commander-13-1、DSS04-duty-1-2、DSS04-duty-2-1、DSS04-itlead-12-1 |
| T2 标准 | 20/12 | DSS04-commander-13-2、DSS04-commander-5-1、DSS04-commander-5-2、DSS04-duty-1-1 |
| T3 记录 | 30/18 | S04-1、S04-12、S04-14、S04-5 |
| T4 关键 | 35/20 | S04-10、S04-13、S04-3、S04-4 |
| T5 复杂 | 40/25 | S04-15、S04-2、S04-9 |
| T6 高复杂 | 45/28 | S04-11 |
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

### S04-1 · 员工收到钓鱼邮件告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`钓鱼邮件`、`点击人数`、`提交凭据`
- `bonusKeywords`：`首封时间`、`发件人`、`涉及科室`、`收件人数`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报要给出三层数据：钓鱼邮件的收件面、点击人数与已提交凭据人数，故取这三项；首封时间与发件人放加分词，该节点 0 分只作提交闸门。

### S04-2 · 核实邮件内容与点击人员

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`仿冒域名`、`点击名单`、`失陷`
- `bonusKeywords`：`已提交凭据`、`附件类型`、`核实结论`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实必须落到仿冒域名、点击名单与是否已失陷三个可核查事实，才能形成可判定结论；附件类型与已提交凭据名单奖励查全的人。

### S04-3 · 预警研判：确认一般（Ⅳ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`提交凭据`、`终端失陷`、`一般`
- `bonusKeywords`：`定级依据`、`提交凭据`、`全院预警`、`Ⅳ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级判据是提交凭据的人数、终端失陷情况与最终等级，故取这三词；等级只用中文“一般”，Ⅳ级等写法只进加分词。

### S04-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`院应急指挥部`、`提交凭据`、`失陷`
- `bonusKeywords`：`Ⅳ级`、`收件人数`、`启动申请`、`点击人数`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：院内报告要报到指挥部、报清提交凭据人数与是否失陷，并同时提出启动申请；等级与各层人数放加分词。

### S04-5 · 院应急指挥部批准启动Ⅳ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`授权范围|授权权限|权限范围`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`Ⅳ级`、`全员预警`、`启动时间`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：审批节点的产物是启动令、授权范围与响应级别三件套，缺一项就证明不了授权边界；题面未含可判定的处置动词，故要素取对象+时间而非动作，避免合法提交被 400 拦。

### S04-6 · 隔离受影响终端

`correct` ｜ 类型 `task` ｜ 表单 `F10` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`受影响终端`、`隔离`、`共享盘`
- `bonusKeywords`：`IP/MAC`、`内存`、`浏览器记录`、`共享权限`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：隔离前记录 IP/MAC、隔离时保留内存与浏览器记录、同时关闭共享盘写入与核权限，故取受影响终端/隔离/共享盘；含符号的 IP/MAC 只放加分词。

### S04-7 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`初报`、`回执`
- `bonusKeywords`：`钓鱼邮件`、`一般`、`2小时`、`报送时间`、`同时报`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：双报的关键是报给谁（市卫生健康局）、报的是初报、以及是否留存回执；2 小时时限含数字只作加分词；“同时报”属报送质量表现，放加分词。

### S04-8 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`口径`、`回执`
- `bonusKeywords`：`初报`、`报送时间`、`接收人`、`市卫生健康局`、`同步报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：与卫健局同步报网信办，判据是两份口径一致且有回执，故取市网信办/口径/回执；接收人与报送时间奖励留痕完整的人；“同步报”属报送质量表现，放加分词。

### S04-9 · 全员预警、召回已投递邮件并封堵仿冒域名

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`全员预警`、`邮件召回`、`仿冒域名`
- `bonusKeywords`：`召回条数`、`网关拦截`、`点击人员`、`发件人`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：该节点三个动作不可只做一个：全员预警、召回已投递邮件、封堵仿冒域名，故三项各取一词；召回条数与网关规则放加分词，避免数量/条数写法差异误伤。

### S04-10 · 重置受影响账号口令

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`受影响账号`、`会话`、`自动转发`
- `bonusKeywords`：`取证完成`、`OAuth授权`、`异常项`、`口令策略`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：重置口令必须连带回收会话并检查自动转发规则，否则旧会话与众发规则仍可用；取证完成的先后顺序放加分词，不设硬门槛。

### S04-11 · 取证邮件样本与网关日志

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`邮件头`、`投递日志`、`哈希`
- `bonusKeywords`：`WHOIS`、`点击名单`、`终端内存`、`保全时间`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：取证要覆盖邮件头、网关投递日志与终端内存并逐项登记哈希，故取邮件头/投递日志/哈希；WHOIS 含大写字母只作加分词。

### S04-12 · 向市网安大队报案并提交材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案材料`、`受理回执|受案回执|回执单|回执`
- `bonusKeywords`：`流程证据`、`事件经过`、`代执行`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：报案验收的是报案对象、随案提交的报案材料与最终受理回执；外部单位只读由指挥部代执行，验收动词不在动作词表，故不放动作要素。

### S04-13 · 应急结束：院应急指挥部宣布Ⅳ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`无新增点击|无新增中招|无新增感染`、`终端验证`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`24小时`、`邮件召回`、`报案回执`、`Ⅳ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：结束的四项条件中，无新增点击、受影响终端验证、上报与报案闭环是三条可核查线，故取这三词；24 小时含数字放加分词。

### S04-14 · 接收市网安大队受理回执并归档

`correct` ｜ 类型 `report` ｜ 表单 `无` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`受理编号|受案编号|案件编号|回执编号`、`归档`
- `bonusKeywords`：`受理人`、`受理时间`、`报案材料`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：该节点只是接收并归档外部回执，验收点是回执本身、受理编号与归档动作，故只要求对象要素，避免把“收到”判成缺动作。

### S04-15 · 应急结束：复盘评估并形成总结报告

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 300 min

- `requiredKeywords`：`钓鱼邮件`、`整改清单`、`责任部门`
- `bonusKeywords`：`时间线`、`根因`、`复测`、`完成时限`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘要按钓鱼邮件处置全过程出整改清单并落到责任部门与完成时限；时间线、根因、复测放加分词，防止只写“已复盘”。

---

## 2. 混淆节点规则（26 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS04-commander-13-1 · 等所有终端重装一周后再结束

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`所有终端`、`重装`、`观察一周`
- `bonusKeywords`：`结束响应`、`后门`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：错误动作是“所有终端重装后观察一周才结束”，照此叙述必然出现所有终端、重装与观察一周；加分词奖励“确保没后门”这一理由。

### DSS04-commander-13-2 · 密码改了就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口令重置`、`结束条件`、`邮件召回`
- `bonusKeywords`：`邮件召回`、`终端失陷`、`上报闭环`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：以改密作为结束条件的写法会写口令重置、结束条件与“已安全”；被它省略的邮件召回与上报闭环，正是加分词要奖励的部分。

### DSS04-commander-5-1 · 让信息科先内部处理

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`信息科`、`自行处置`、`内部处理`
- `bonusKeywords`：`不启动`、`资源`、`跨部门`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：不启动预案的写法是交由信息科自行处置、先内部处理；跨部门协同缺失只作加分词，不拿“未授权”这类后果词设必含。

### DSS04-commander-5-2 · 等更多人中招了再说

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`暂不启动`、`更多人员`、`中招`
- `bonusKeywords`：`观望`、`扩散`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等待更多人出事的错误决策，其自然写法是暂不启动、等更多人员中招反馈；扩散风险只放加分词。

### DSS04-duty-1-1 · 等更多员工反馈后再处理

`distractor` ｜ 分值 8 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`观望`、`员工反馈`、`处置`
- `bonusKeywords`：`群发`、`点击数`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先观望是否有更多员工反馈再决定处置，三个词正对应这一错误链条；邮件仍在收件箱属后果描述，不设必含。

### DSS04-duty-1-2 · 让 IT 全员检查邮箱

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`邮箱`、`全员`、`自查`
- `bonusKeywords`：`全院`、`收件人`、`点击情况`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只让本方人员查邮箱的写法会落到邮箱、全员与自查三词；全院覆盖与点击情况是它漏掉的范围，放加分词。

### DSS04-duty-2-1 · 让当事人自己说点了什么链接

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`当事人`、`回忆`、`链接`
- `bonusKeywords`：`凭据`、`遗漏`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：依赖当事人回忆的错误做法，写法必然出现当事人、回忆与点开的链接；凭据提交情况与遗漏风险放加分词。

### DSS04-itlead-12-1 · 不报案，内部处理

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`钓鱼邮件`、`常见`、`内部处理`
- `bonusKeywords`：`网安大队`、`自行处置`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：不报案的理由是“钓鱼邮件太常见”，落到内部处理，三词对应其自述逻辑；网安大队只出现在否定句里，故不入必含。

### DSS04-itlead-12-2 · 仅口头报案，不提交报案材料与流程证据

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`电话`、`市网安大队`
- `bonusKeywords`：`报案材料`、`流程证据`、`补交`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：电话口头报案的写法必然出现口头报案、电话与市网安大队；未提交的材料与流程证据是缺口，放加分词。

### DSS04-itlead-3-1 · 只按中招人数定级，不核查是否已有终端失陷

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`中招人数`、`定级`、`规则`
- `bonusKeywords`：`一般`、`终端失陷`、`凭据泄露`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只按人数套规则的写法会出现中招人数、定级与规则三点；10 人阈值这类数字写法放加分词，避免格式差异。

### DSS04-itlead-3-2 · 等网信办建议再定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`建议`、`定级`
- `bonusKeywords`：`研判`、`上级`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：把定级权外推的写法是等市网信办给建议后再定级，三词即其行为链；院内不研判是对照项，只放加分词。

### DSS04-itlead-4-1 · 等全部中招人员排查清楚再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`中招人员`、`排查`、`报告`
- `bonusKeywords`：`Ⅳ级`、`预案启动`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等排查完再报的错误动作，写法落在中招人员、排查与报告三个词；等级与延误后果放加分词。

### DSS04-itlead-4-2 · 先报"收到可疑邮件"

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`可疑邮件`、`报告`、`跟进`
- `bonusKeywords`：`点击`、`凭据提交`、`钓鱼性质`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只报“收到可疑邮件”的写法必然出现可疑邮件与报告，后续跟进是它的托词；钓鱼性质、点击与凭据提交作为遗漏项放加分词。

### DSS04-itlead-7-1 · 等网信办意见出来再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`意见`、`市卫生健康局`
- `bonusKeywords`：`时限`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等外部意见再报卫健局的写法，两个单位名与“意见”都会出现；法定时限含数字只作加分词。

### DSS04-itlead-7-2 · 报"收到可疑邮件"不说是钓鱼

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`可疑邮件`、`上报`、`事件性质`
- `bonusKeywords`：`措辞`、`钓鱼邮件`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：刻意弱化定性的上报会写“只写收到可疑邮件”与弱化事件性质，故取可疑邮件/上报/事件性质；措辞保守放加分词。

### DSS04-itlead-8-1 · 只口头初报，不提交书面初报与续报

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`口头初报`、`续报`
- `bonusKeywords`：`书面初报`、`留痕`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：仅口头初报的写法是向市网信办口头初报、续报后续再说，三词自然出现；书面初报与留痕正是它缺的，放加分词。

### DSS04-itlead-8-2 · 等处置完了再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`处置结束`、`市网信办`、`上报`
- `bonusKeywords`：`初报时限`、`完整情况`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等处置结束再报的写法含处置结束、市网信办与上报三词；错过初报时限属后果描述，放加分词不设必含。

### DSS04-itlead-9-1 · 先发内部群通知大家注意

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`内部群`、`通知`、`提醒`
- `bonusKeywords`：`邮件召回`、`仿冒域名`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只发群通知的做法，写法是内部群、通知与提醒三点；未做的邮件召回与域名封堵放加分词，奖励写清边界的人。

### DSS04-itlead-9-2 · 等网信办的钓鱼样本分析再封堵

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`样本分析`、`封堵`、`空窗期`
- `bonusKeywords`：`封堵空窗`、`继续投递`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等外部样本分析再封堵的写法，落点是样本分析、封堵与空窗期；空窗期内继续投递是后果词，只放加分词。

### DSS04-netsec-11-1 · 只保存那封钓鱼邮件的样本

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`邮件`、`样本`、`保存`
- `bonusKeywords`：`邮件头`、`投递日志`、`点击名单`、`终端内存`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只留那封邮件的错误取证，写法只会出现邮件、样本与保存；被遗漏的邮件头、投递日志与内存放加分词。

### DSS04-netsec-11-2 · 先处置业务，取证留个描述就行

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`恢复业务`、`文字描述`、`取证`
- `bonusKeywords`：`证据固定`、`内存`、`日志`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：以文字描述代替证据固定的写法含恢复业务、文字描述与取证三词；已灭失的内存与日志放加分词，提示其代价。

### DSS04-netsec-2-2 · 只看收件人数，不核对点击与凭据提交名单

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`收件人数`、`影响面`、`统计`
- `bonusKeywords`：`点击名单`、`提交凭据`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只看发件量的核实写法是统计收件人数、认为影响面已明；未核对的点击名单与提交凭据放加分词。

### DSS04-netsec-6-1 · 只改密码，不断网

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`重置口令`、`终端`、`隔离`
- `bonusKeywords`：`外联`、`横向移动`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：只改密不断网的写法会出现重置口令、终端与隔离（否定式）三词；外联与横向移动是后果，放加分词。

### DSS04-netsec-6-2 · 先备份终端数据再隔离

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`备份`、`终端`、`隔离`
- `bonusKeywords`：`内存`、`易失证据`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：先备份再隔离的写法就是备份、终端、隔离三词；备份过程破坏内存等易失证据，作为加分词提示其风险。

### DSS04-sysadmin-10-1 · 等他们下一次登录再强制改

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`下次登录`、`改密`、`账号`
- `bonusKeywords`：`强制`、`空窗`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：推迟改密的写法是等下次登录再要求改密，账号在空窗期仍可用，故取下次登录、改密与账号；空窗风险放加分词。

### DSS04-sysadmin-10-2 · 让账号自己改，我们不强制

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`用户`、`自行改密`、`重置`
- `bonusKeywords`：`会话`、`自行修改`、`不强制`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：把改密责任推给用户的写法会出现用户、自行改密与重置（否定式“不强制重置”）；会话回收缺失放加分词。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S04-1 | 正确 | — | 0 | 钓鱼邮件、点击人数、提交凭据 | 首封时间、发件人、涉及科室、收件人数 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S04-2 | 正确 | F12 | 8 | 仿冒域名、点击名单、失陷 | 已提交凭据、附件类型、核实结论 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S04-3 | 正确 | F3 | 8 | 提交凭据、终端失陷、一般 | 定级依据、提交凭据、全院预警、Ⅳ级 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S04-4 | 正确 | F4 | 8 | 院应急指挥部、提交凭据、失陷 | Ⅳ级、收件人数、启动申请、点击人数 | 35/20 | 对象、动作、时间 | P4 | 0.25/0.30 |
| S04-5 | 正确 | F9 | 8 | 启动令、授权范围\|授权权限\|权限范围、响应级别\|响应等级\|响应分级 | Ⅳ级、全员预警、启动时间 | 30/18 | 对象、时间 | P8 | 0.25/0.30 |
| S04-6 | 正确 | F10 | 8 | 受影响终端、隔离、共享盘 | IP/MAC、内存、浏览器记录、共享权限 | 35/20 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S04-7 | 正确 | F2 | 7 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、初报、回执 | 钓鱼邮件、一般、2小时、报送时间、同时报 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S04-8 | 正确 | F2 | 7 | 市网信办\|网信办\|市委网信办、口径、回执 | 初报、报送时间、接收人、市卫生健康局、同步报 | 30/18 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S04-9 | 正确 | F9 | 8 | 全员预警、邮件召回、仿冒域名 | 召回条数、网关拦截、点击人员、发件人 | 40/25 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S04-10 | 正确 | F9 | 8 | 受影响账号、会话、自动转发 | 取证完成、OAuth授权、异常项、口令策略 | 35/20 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S04-11 | 正确 | F13 | 8 | 邮件头、投递日志、哈希 | WHOIS、点击名单、终端内存、保全时间 | 45/28 | 对象、时间、结论 | P5 | 0.25/0.30 |
| S04-12 | 正确 | F5 | 8 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案材料、受理回执\|受案回执\|回执单\|回执 | 流程证据、事件经过、代执行 | 30/18 | 对象、时间 | P4 | 0.25/0.30 |
| S04-13 | 正确 | F8 | 7 | 无新增点击\|无新增中招\|无新增感染、终端验证、闭环\|闭环管理\|办结\|归档 | 24小时、邮件召回、报案回执、Ⅳ级 | 35/20 | 对象、动作、结论 | P8 | 0.20/0.30 |
| S04-14 | 正确 | — | 7 | 受理回执\|受案回执\|回执单\|回执、受理编号\|受案编号\|案件编号\|回执编号、归档 | 受理人、受理时间、报案材料 | 30/18 | 无 | P4 | 0.20/0.30 |
| S04-15 | 正确 | — | 6 | 钓鱼邮件、整改清单、责任部门 | 时间线、根因、复测、完成时限 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS04-commander-13-1 | 混淆 | — | 3 | 所有终端、重装、观察一周 | 结束响应、后门 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS04-commander-13-2 | 混淆 | — | 8 | 口令重置、结束条件、邮件召回 | 邮件召回、终端失陷、上报闭环 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS04-commander-5-1 | 混淆 | — | 8 | 信息科、自行处置、内部处理 | 不启动、资源、跨部门 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS04-commander-5-2 | 混淆 | — | 8 | 暂不启动、更多人员、中招 | 观望、扩散 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-duty-1-1 | 混淆 | — | 8 | 观望、员工反馈、处置 | 群发、点击数 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-duty-1-2 | 混淆 | — | 5 | 邮箱、全员、自查 | 全院、收件人、点击情况 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS04-duty-2-1 | 混淆 | — | 5 | 当事人、回忆、链接 | 凭据、遗漏 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS04-itlead-12-1 | 混淆 | — | 3 | 钓鱼邮件、常见、内部处理 | 网安大队、自行处置 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS04-itlead-12-2 | 混淆 | — | 5 | 口头报案、电话、市网安大队 | 报案材料、流程证据、补交 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS04-itlead-3-1 | 混淆 | — | 5 | 中招人数、定级、规则 | 一般、终端失陷、凭据泄露 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS04-itlead-3-2 | 混淆 | — | 8 | 市网信办、建议、定级 | 研判、上级 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-itlead-4-1 | 混淆 | — | 8 | 中招人员、排查、报告 | Ⅳ级、预案启动、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-itlead-4-2 | 混淆 | — | 3 | 可疑邮件、报告、跟进 | 点击、凭据提交、钓鱼性质 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS04-itlead-7-1 | 混淆 | — | 8 | 市网信办、意见、市卫生健康局 | 时限、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-itlead-7-2 | 混淆 | — | 5 | 可疑邮件、上报、事件性质 | 措辞、钓鱼邮件 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS04-itlead-8-1 | 混淆 | — | 5 | 市网信办、口头初报、续报 | 书面初报、留痕 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS04-itlead-8-2 | 混淆 | — | 8 | 处置结束、市网信办、上报 | 初报时限、完整情况 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-itlead-9-1 | 混淆 | — | 5 | 内部群、通知、提醒 | 邮件召回、仿冒域名 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS04-itlead-9-2 | 混淆 | — | 8 | 样本分析、封堵、空窗期 | 封堵空窗、继续投递 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-netsec-11-1 | 混淆 | — | 5 | 邮件、样本、保存 | 邮件头、投递日志、点击名单、终端内存 | 15/10 | 对象 | P5 | 0.15/0.20 |
| DSS04-netsec-11-2 | 混淆 | — | 8 | 恢复业务、文字描述、取证 | 证据固定、内存、日志 | 20/12 | 对象 | P5 | 0.15/0.25 |
| DSS04-netsec-2-2 | 混淆 | — | 5 | 收件人数、影响面、统计 | 点击名单、提交凭据 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS04-netsec-6-1 | 混淆 | — | 8 | 重置口令、终端、隔离 | 外联、横向移动 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS04-netsec-6-2 | 混淆 | — | 5 | 备份、终端、隔离 | 内存、易失证据 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS04-sysadmin-10-1 | 混淆 | — | 8 | 下次登录、改密、账号 | 强制、空窗 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS04-sysadmin-10-2 | 混淆 | — | 8 | 用户、自行改密、重置 | 会话、自行修改、不强制 | 20/12 | 对象 | P2 | 0.15/0.25 |

---

## 4. 总量、分布与风险

### 4.1 S04 规则总量

- 节点 41：正确 15、混淆 26；规则条数 41，与节点一一对应，无遗漏无多余。
- 必含词合计 123 个，平均 3.0 个/节点；加分词合计 119 个。
- 长度档：15/10 共 13 个；20/12 共 13 个；30/18 共 5 个；35/20 共 6 个；40/25 共 3 个；45/28 共 1 个。
- 要素组合：对象 27 个；对象+动作 1 个；对象+动作+结论 5 个；对象+时间+结论 2 个；对象+时间 2 个；对象+结论 1 个；对象+动作+时间 3 个。
- 表单绑定（正确节点）：F10 1 个、F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F8 1 个、F9 3 个、无表单 3 个。

### 4.2 风险最高的几类节点

1. **混淆节点（26 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S04-3`（研判定级，8 分）：必含 `终端失陷`。只写“按预案定级为一般事件”的参演人会缺这一项，因为“终端失陷”是定级的关键事实，但标题里没出现，提示需点明。
3. `DSS04-duty-1-2`（混淆，全员排查）：必含 `全员`。参演人选了这条混淆、只写“让 IT 人员查邮箱”，就会因缺词提交不了，反而拿不到“执行混淆扣全分”，演练设计失效。
4. `S04-5`（批准启动Ⅳ级，8 分）：该节点题面只有“批准/签发/明确”，没有引擎动作词表内的动词，要素已改为 `对象`+`时间`；参演人若不写“立即/已于/时刻”这类时间表述，会被 400 拦。这是本册最需要演练前自测的节点。
5. `S04-9`（全员预警/召回邮件/封堵域名，8 分）：`仿冒域名`、`网关` 等词写法敏感度低，但“召回”不在动作词表，要素未放动作，区分度全部落在必含词上。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（41 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S04-1": {
    "requiredKeywords": [
      "钓鱼邮件",
      "点击人数",
      "提交凭据"
    ],
    "bonusKeywords": [
      "首封时间",
      "发件人",
      "涉及科室",
      "收件人数"
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
  "S04-2": {
    "requiredKeywords": [
      "仿冒域名",
      "点击名单",
      "失陷"
    ],
    "bonusKeywords": [
      "已提交凭据",
      "附件类型",
      "核实结论"
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
  "S04-3": {
    "requiredKeywords": [
      "提交凭据",
      "终端失陷",
      "一般"
    ],
    "bonusKeywords": [
      "定级依据",
      "提交凭据",
      "全院预警",
      "Ⅳ级"
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
  "S04-4": {
    "requiredKeywords": [
      "院应急指挥部",
      "提交凭据",
      "失陷"
    ],
    "bonusKeywords": [
      "Ⅳ级",
      "收件人数",
      "启动申请",
      "点击人数"
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
  "S04-5": {
    "requiredKeywords": [
      "启动令",
      "授权范围|授权权限|权限范围",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "Ⅳ级",
      "全员预警",
      "启动时间"
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
  "S04-6": {
    "requiredKeywords": [
      "受影响终端",
      "隔离",
      "共享盘"
    ],
    "bonusKeywords": [
      "IP/MAC",
      "内存",
      "浏览器记录",
      "共享权限"
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
  "S04-7": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "初报",
      "回执"
    ],
    "bonusKeywords": [
      "钓鱼邮件",
      "一般",
      "2小时",
      "报送时间",
      "同时报"
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
  "S04-8": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "口径",
      "回执"
    ],
    "bonusKeywords": [
      "初报",
      "报送时间",
      "接收人",
      "市卫生健康局",
      "同步报"
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
    "keywordPenaltyRatio": 0.3
  },
  "S04-9": {
    "requiredKeywords": [
      "全员预警",
      "邮件召回",
      "仿冒域名"
    ],
    "bonusKeywords": [
      "召回条数",
      "网关拦截",
      "点击人员",
      "发件人"
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
  "S04-10": {
    "requiredKeywords": [
      "受影响账号",
      "会话",
      "自动转发"
    ],
    "bonusKeywords": [
      "取证完成",
      "OAuth授权",
      "异常项",
      "口令策略"
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
  "S04-11": {
    "requiredKeywords": [
      "邮件头",
      "投递日志",
      "哈希"
    ],
    "bonusKeywords": [
      "WHOIS",
      "点击名单",
      "终端内存",
      "保全时间"
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
  "S04-12": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案材料",
      "受理回执|受案回执|回执单|回执"
    ],
    "bonusKeywords": [
      "流程证据",
      "事件经过",
      "代执行"
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
  "S04-13": {
    "requiredKeywords": [
      "无新增点击|无新增中招|无新增感染",
      "终端验证",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "24小时",
      "邮件召回",
      "报案回执",
      "Ⅳ级"
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
    "keywordPenaltyRatio": 0.3
  },
  "S04-14": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "受理编号|受案编号|案件编号|回执编号",
      "归档"
    ],
    "bonusKeywords": [
      "受理人",
      "受理时间",
      "报案材料"
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
  "S04-15": {
    "requiredKeywords": [
      "钓鱼邮件",
      "整改清单",
      "责任部门"
    ],
    "bonusKeywords": [
      "时间线",
      "根因",
      "复测",
      "完成时限"
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
  "DSS04-commander-13-1": {
    "requiredKeywords": [
      "所有终端",
      "重装",
      "观察一周"
    ],
    "bonusKeywords": [
      "结束响应",
      "后门"
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
  "DSS04-commander-13-2": {
    "requiredKeywords": [
      "口令重置",
      "结束条件",
      "邮件召回"
    ],
    "bonusKeywords": [
      "邮件召回",
      "终端失陷",
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
  "DSS04-commander-5-1": {
    "requiredKeywords": [
      "信息科",
      "自行处置",
      "内部处理"
    ],
    "bonusKeywords": [
      "不启动",
      "资源",
      "跨部门"
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
  "DSS04-commander-5-2": {
    "requiredKeywords": [
      "暂不启动",
      "更多人员",
      "中招"
    ],
    "bonusKeywords": [
      "观望",
      "扩散"
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
  "DSS04-duty-1-1": {
    "requiredKeywords": [
      "观望",
      "员工反馈",
      "处置"
    ],
    "bonusKeywords": [
      "群发",
      "点击数"
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
  "DSS04-duty-1-2": {
    "requiredKeywords": [
      "邮箱",
      "全员",
      "自查"
    ],
    "bonusKeywords": [
      "全院",
      "收件人",
      "点击情况"
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
  "DSS04-duty-2-1": {
    "requiredKeywords": [
      "当事人",
      "回忆",
      "链接"
    ],
    "bonusKeywords": [
      "凭据",
      "遗漏"
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
  "DSS04-itlead-12-1": {
    "requiredKeywords": [
      "钓鱼邮件",
      "常见",
      "内部处理"
    ],
    "bonusKeywords": [
      "网安大队",
      "自行处置"
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
  "DSS04-itlead-12-2": {
    "requiredKeywords": [
      "口头报案",
      "电话",
      "市网安大队"
    ],
    "bonusKeywords": [
      "报案材料",
      "流程证据",
      "补交"
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
  "DSS04-itlead-3-1": {
    "requiredKeywords": [
      "中招人数",
      "定级",
      "规则"
    ],
    "bonusKeywords": [
      "一般",
      "终端失陷",
      "凭据泄露"
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
  "DSS04-itlead-3-2": {
    "requiredKeywords": [
      "市网信办",
      "建议",
      "定级"
    ],
    "bonusKeywords": [
      "研判",
      "上级"
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
  "DSS04-itlead-4-1": {
    "requiredKeywords": [
      "中招人员",
      "排查",
      "报告"
    ],
    "bonusKeywords": [
      "Ⅳ级",
      "预案启动",
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
  "DSS04-itlead-4-2": {
    "requiredKeywords": [
      "可疑邮件",
      "报告",
      "跟进"
    ],
    "bonusKeywords": [
      "点击",
      "凭据提交",
      "钓鱼性质"
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
  "DSS04-itlead-7-1": {
    "requiredKeywords": [
      "市网信办",
      "意见",
      "市卫生健康局"
    ],
    "bonusKeywords": [
      "时限",
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
  "DSS04-itlead-7-2": {
    "requiredKeywords": [
      "可疑邮件",
      "上报",
      "事件性质"
    ],
    "bonusKeywords": [
      "措辞",
      "钓鱼邮件"
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
  "DSS04-itlead-8-1": {
    "requiredKeywords": [
      "市网信办",
      "口头初报",
      "续报"
    ],
    "bonusKeywords": [
      "书面初报",
      "留痕"
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
  "DSS04-itlead-8-2": {
    "requiredKeywords": [
      "处置结束",
      "市网信办",
      "上报"
    ],
    "bonusKeywords": [
      "初报时限",
      "完整情况"
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
  "DSS04-itlead-9-1": {
    "requiredKeywords": [
      "内部群",
      "通知",
      "提醒"
    ],
    "bonusKeywords": [
      "邮件召回",
      "仿冒域名"
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
  "DSS04-itlead-9-2": {
    "requiredKeywords": [
      "样本分析",
      "封堵",
      "空窗期"
    ],
    "bonusKeywords": [
      "封堵空窗",
      "继续投递"
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
  "DSS04-netsec-11-1": {
    "requiredKeywords": [
      "邮件",
      "样本",
      "保存"
    ],
    "bonusKeywords": [
      "邮件头",
      "投递日志",
      "点击名单",
      "终端内存"
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
  "DSS04-netsec-11-2": {
    "requiredKeywords": [
      "恢复业务",
      "文字描述",
      "取证"
    ],
    "bonusKeywords": [
      "证据固定",
      "内存",
      "日志"
    ],
    "minMeasureLen": 20,
    "minResultLen": 12,
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
    "keywordPenaltyRatio": 0.25
  },
  "DSS04-netsec-2-2": {
    "requiredKeywords": [
      "收件人数",
      "影响面",
      "统计"
    ],
    "bonusKeywords": [
      "点击名单",
      "提交凭据"
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
  "DSS04-netsec-6-1": {
    "requiredKeywords": [
      "重置口令",
      "终端",
      "隔离"
    ],
    "bonusKeywords": [
      "外联",
      "横向移动"
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
  "DSS04-netsec-6-2": {
    "requiredKeywords": [
      "备份",
      "终端",
      "隔离"
    ],
    "bonusKeywords": [
      "内存",
      "易失证据"
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
  "DSS04-sysadmin-10-1": {
    "requiredKeywords": [
      "下次登录",
      "改密",
      "账号"
    ],
    "bonusKeywords": [
      "强制",
      "空窗"
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
  "DSS04-sysadmin-10-2": {
    "requiredKeywords": [
      "用户",
      "自行改密",
      "重置"
    ],
    "bonusKeywords": [
      "会话",
      "自行修改",
      "不强制"
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
