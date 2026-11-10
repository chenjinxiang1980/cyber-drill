# S06 处置文字质量规则清单（41 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S06 全部 41 个节点（correct 15 / distractor 26），剧本「患者隐私数据泄露（舆情发酵）」
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
| T1 轻量 | 15/10 | DSS06-cac-11-1、DSS06-commander-13-1、DSS06-duty-1-1、DSS06-duty-2-1 |
| T2 标准 | 20/12 | DSS06-cac-11-2、DSS06-commander-13-2、DSS06-commander-5-1、DSS06-commander-5-2 |
| T3 记录 | 30/18 | S06-1、S06-10、S06-14、S06-5 |
| T4 关键 | 35/20 | S06-13、S06-3、S06-4、S06-6 |
| T5 复杂 | 40/25 | S06-11、S06-12、S06-15、S06-2 |
| T6 高复杂 | 45/28 | S06-9 |
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

### S06-1 · 互联网出现患者信息售卖帖告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 10 min

- `requiredKeywords`：`售卖帖`、`截图`、`哈希`
- `bonusKeywords`：`发布时间`、`字段名`、`样本量级`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报只做三件事：记录患者信息售卖帖、截图固证、计算哈希；URL 与 SHA-256 属写法敏感词只放加分，0 分节点规则仅作提交闸门。

### S06-2 · 核实泄露样本与数据量级

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`脱敏字段`、`条数`、`敏感标识`
- `bonusKeywords`：`隔离环境`、`样本哈希`、`时间范围`、`不转存`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实的关键是脱敏字段、泄露条数（数据量级）与是否含身份证号等敏感标识；guide 要求先固化哈希再在隔离环境比对，故加分词奖励这两步。

### S06-3 · 预警研判：确认重大（Ⅱ级）事件与泄露来源

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`定级依据|定级理由|分级依据|判定依据`、`数据量级`、`重大`
- `bonusKeywords`：`敏感字段`、`舆情热度`、`扩散速度`、`Ⅱ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级要同时给出定级依据、数据量级与“重大”结论，guide 明确不得等抓到源头再定级；用“重大”而非罗马数字，规避 Ⅱ/II/2 写法差异。

### S06-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`指挥部`、`事件等级|事件级别|事件分级`、`启动`
- `bonusKeywords`：`泄露范围`、`Ⅱ级`、`同步提交`、`初步来源`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：院内报告的可核查产物是事件等级、启动申请与报送指挥部；不要求结论要素，避免把“建议启动”硬写成已完成启动。

### S06-5 · 院应急指挥部批准启动Ⅱ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`启动令`、`授权范围|授权权限|权限范围`、`响应级别|响应等级|响应分级`
- `bonusKeywords`：`固证优先`、`删帖`、`报案`、`Ⅱ级`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：审批节点的产物是启动令、授权范围与响应级别；guide 要求授权固证优先于删帖与接口关闭，加分词奖励把优先级写出来的人。

### S06-6 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`事件等级|事件级别|事件分级`、`回执`
- `bonusKeywords`：`1小时`、`泄露范围`、`舆情`、`同步上报`、`续报`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：报卫健局的关键是接收单位、事件等级与回执三项；一级时限只放加分词，防止全半角与空格差异导致必含词漏命中；guide 提到“初报后可续报”，续报属后续安排，放加分词。

### S06-7 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`哈希`、`事件等级|事件级别|事件分级`
- `bonusKeywords`：`1小时`、`舆情`、`先固定`、`泄露范围`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：报网信办比报卫健局多一道前置约束：样本先固定哈希再提交，故判据槽取哈希而非重复 S06-6 的回执。

### S06-8 · 关闭外泄接口并加固数据权限

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`外泄接口`、`关闭`、`最小权限`
- `bonusKeywords`：`访问日志`、`第三方`、`测试环境`、`先取证`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：处置对象是外泄接口、动作是关闭、边界是复核最小权限；guide 要求先取证后关闭，加分词奖励关闭前导出访问日志与排查第三方账号。

### S06-9 · 固定泄露证据与溯源

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 8 ｜ 超时 25 min

- `requiredKeywords`：`导出日志`、`固定`、`完整性校验|完整性检查|完整性核对`
- `bonusKeywords`：`访问日志`、`账号清单`、`时间线`、`帖子快照`
- `minMeasureLen` / `minResultLen`：**45 / 28**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：取证要覆盖 HIS 导出日志、固定的动作与完整性校验判据；要求时间要素，用于证明取证发生在删帖与关接口之前。

### S06-10 · 向市网安大队报案并提交证据材料

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`证据材料`
- `bonusKeywords`：`先报案后补`、`受理编号`、`不等待外部意见`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：报案节点验收的是向市网安大队报案并提交证据材料；不设“受理”为必含，因为受理结果不由本院决定，材料不全先报案后补放加分词。

### S06-11 · 统一对外口径与舆情回应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`口径`、`审批|审核|批准`、`患者信息`
- `bonusKeywords`：`发言人`、`法务`、`已核实事实`、`监测`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：舆情节点验收的是口径经审批后统一发布、且不披露患者信息；要素用对象锁定涉及系统、用结论锁定已核实与已发布，不套技术处置模板。

### S06-12 · 患者告知与法律评估

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 20 min

- `requiredKeywords`：`告知范围`、`法务`、`留痕`
- `bonusKeywords`：`短信`、`公告`、`求助渠道`、`不等待外部口径`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：告知节点要写清告知范围、法务把关与留痕三项，判据是“是否留痕可查”而非“是否逐人送达”；不得等外部统一口径，放加分词。

### S06-13 · 应急结束：院应急指挥部宣布Ⅱ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`证据固定`、`闭环|闭环管理|办结|归档`
- `bonusKeywords`：`外泄接口`、`患者告知`、`备案`、`不得以删帖`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束必须逐条对上结束条件、证据固定与上报报案闭环，guide 明确不得以删帖作为结束条件；五项条件写全才给满分。

### S06-14 · 市网安大队受理回执与取证要求确认（院应急指挥部代收）

`correct` ｜ 类型 `report` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`受理编号|受案编号|案件编号|回执编号`、`取证要求`、`回执`
- `bonusKeywords`：`受理人`、`取证清单`、`代收`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：外部受理节点只记录受理编号、取证要求与回执，本院不产生处置动作，故要素只留对象，区分度全交给三个必含词。

### S06-15 · 复盘总结与整改闭环

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`时间线`、`整改项`、`责任人`
- `bonusKeywords`：`告知覆盖率`、`复测`、`时限`、`归档`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：复盘要求时间线、整改项与责任人三件套，是本剧本最长档；加分词奖励告知覆盖率与复测，避免写成流水账。

---

## 2. 混淆节点规则（26 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS06-cac-11-1 · 让医院先内部自查，不对外发声

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`内部自查`、`对外发声`、`舆情`
- `bonusKeywords`：`自行核实`、`等结论`、`统一口径`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是“先自查不对外发声”，其合理写法必然出现内部自查、对外发声与否和舆情走向，故取这三词；5 分混淆按 T1 只作提交闸门。

### DSS06-cac-11-2 · 直接让医院发声明致歉

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`声明`、`致歉`、`法务`
- `bonusKeywords`：`未审批`、`公开`、`舆情风险`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：直接把声明致歉发出去，写法会落到声明、致歉与是否经过法务；加分词奖励“未经审批”这类自证其错的表述。

### DSS06-commander-13-1 · 等舆情平息一个月后再宣布

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`舆情平息`、`再宣布`、`结束`
- `bonusKeywords`：`观察期`、`无复发`、`延迟`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：延迟结束的写法会写“舆情平息”这一观察判据、再宣布的动作以及结束本身；“一个月”写法易变，只作隐性约束不入必含。

### DSS06-commander-13-2 · 帖子删了就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`帖子`、`宣布结束`、`固证`
- `bonusKeywords`：`未固证`、`未告知`、`外泄接口`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：以删帖作为结束条件的写法，对象是帖子、收口是宣布结束、漏项是固证；加分词奖励它自己跳过的告知与接口关闭。

### DSS06-commander-5-1 · 让信息科先处理技术问题

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`信息科`、`技术问题`、`舆情`
- `bonusKeywords`：`告知`、`报案`、`暂不启动`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：把启动缩成“信息科先处理技术问题”，必然出现信息科、技术问题与被搁置的舆情，错在启动决策没有覆盖全流程。

### DSS06-commander-5-2 · 等网信办介入再说

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`介入`、`启动`
- `bonusKeywords`：`响应空转`、`等待`、`暂不启动`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等外部介入再启动的写法，落点是网信办、介入与启动三者的先后关系，不取正确节点的授权范围与响应级别。

### DSS06-duty-1-1 · 只看条数，不截图固证

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`帖子`、`条数`、`量级`
- `bonusKeywords`：`未截图`、`未固证`、`时间点`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只统计条数的写法会出现帖子、条数与量级判断；刻意不设截图、哈希，否则参演人照 descr 写反而提交不了。

### DSS06-duty-1-2 · 让网信办先把帖子删掉再说

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`删帖`、`先行`
- `bonusKeywords`：`证据灭失`、`减少扩散`、`后续再说`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：要求先行删帖的写法会写网信办、删帖与先行；判据侧不要求证据，避免把这个错误动作写成正确固证。

### DSS06-duty-2-1 · 只对比时间点，不核对导出日志

`distractor` ｜ 分值 3 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`时间点`、`导出日志`、`账号`
- `bonusKeywords`：`HIS`、`吻合`、`来源`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只比对时间点的写法会落到时间点、导出日志与账号三个事实，正是漏查后两者才被判错。

### DSS06-itlead-10-1 · 先等材料齐全再报案

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`材料齐全`、`报案`、`时限`
- `bonusKeywords`：`先报案后补`、`立案`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等材料齐全再报案，写法必然出现材料齐全、报案与时限；“先报案后补材料”放加分词，奖励真正懂流程的人。

### DSS06-itlead-10-2 · 等网信办有结论了再决定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`结论`、`报案`
- `bonusKeywords`：`再决定`、`立案`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等外部结论再决定是否报案的写法，落点是网信办、结论与报案，不取受理编号这类正确材料作为判据。

### DSS06-itlead-3-1 · 只看数据类型，不看量级与舆情

`distractor` ｜ 分值 3 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`数据类型`、`分级`、`舆情`
- `bonusKeywords`：`Ⅱ级`、`量级`、`热度`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只看数据类型就分级的写法会出现数据类型、分级与舆情；量级与等级罗马数字只进加分词，避免写法差异误判。

### DSS06-itlead-3-2 · 等抓到泄露来源再定

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`泄露来源`、`定级`、`响应`
- `bonusKeywords`：`告知`、`源头`、`推迟`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等抓到来源再定级的写法会写泄露来源、定级与被拖住的响应；该节点错在时序，不在等级结论本身。

### DSS06-itlead-4-1 · 等网信办把帖子删了再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`删帖`、`上报`
- `bonusKeywords`：`1小时`、`证据灭失`、`时限`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等删帖再上报的写法落在网信办、删帖与上报三件事；一级时限只放加分词，防止“1 小时/1小时”写法差异漏命中。

### DSS06-itlead-4-2 · 先报"疑似数据泄露"留余地

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`疑似`、`上报`、`余地`
- `bonusKeywords`：`措辞`、`保守`、`后续核实`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：先报“疑似数据泄露”留余地的写法会出现疑似、上报与余地；属保守但及时的轻度过失，故 3 分轻度档。

### DSS06-itlead-6-1 · 等网信办那边结果出来了再同步

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`结果`、`同步`
- `bonusKeywords`：`1小时`、`卫健`、`时限`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等网信办结果再同步的写法落在网信办、结果与同步；加分辨的是“是否与卫健并列上报、是否卡时限”。

### DSS06-itlead-6-2 · 只发初报，不按1小时时限续报

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`初报`、`续报`、`时限`
- `bonusKeywords`：`1小时`、`初步情况`、`完整`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：只发初报不续报的写法会出现初报、续报与时限，判据是续报有没有卡住时限，而不是初报写得漂不漂亮。

### DSS06-itlead-7-1 · 未固定样本直接外发

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`样本`、`哈希`、`网信办`
- `bonusKeywords`：`二次扩散`、`未固定`、`脱敏`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：样本未固定哈希就外发的写法，对象是样本、判据是哈希、接收方是网信办；不取“脱敏”这个正确补充动作。

### DSS06-itlead-7-2 · 等删完帖再报完整情况

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`删完帖`、`完整情况`、`上报`
- `bonusKeywords`：`证据灭失`、`时限`、`先行`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等删完帖再报完整情况的写法落在删完帖、完整情况与上报；与 commander 侧的区别在于它强调“报完整情况”的时点。

### DSS06-medlead-12-1 · 先等网信办统一口径再告知患者

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`统一口径`、`告知`
- `bonusKeywords`：`患者`、`延迟`、`尚可解释`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：等网信办统一口径再告知的写法会出现网信办、统一口径与告知；延迟不等于不告知，故取 5 分而不到 8 分。

### DSS06-medlead-12-2 · 不告知，等患者找上门再说

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`患者`、`告知`、`被动`
- `bonusKeywords`：`法律风险`、`信任`、`主动联系`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：完全不告知的写法落在患者、告知与被动应对；不把“法律风险”设必含，避免只写后果不写做法。

### DSS06-netsec-2-2 · 只凭字段就认定源头，不查导出日志

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`字段`、`源头`、`导出日志`
- `bonusKeywords`：`HIS`、`表名`、`访问账号`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只凭字段认定源头的写法会出现字段、源头与导出日志，正因未查后者才判错；访问账号与表名放加分词。

### DSS06-netsec-8-1 · 只锁涉及泄露的那一个接口

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`同类接口`、`外泄`、`最小化`
- `bonusKeywords`：`最小化`、`外泄`、`其他接口`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：只锁一个接口的写法会落在同类接口、外泄与最小化，“同类接口”正是它漏处置的边界，不取最小权限这类正确表述。

### DSS06-netsec-8-2 · 等数据泄露源头抓到了再改权限

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`源头`、`权限`、`持续`
- `bonusKeywords`：`持续外泄`、`复核`、`删除权限`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：等抓到源头再改权限的写法落在源头、权限与持续，判据是权限在泄露期间是否保持原状。

### DSS06-netsec-9-1 · 先删帖再说，泄露源头以后追

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`删帖`、`溯源`、`先止损`
- `bonusKeywords`：`证据灭失`、`扩散`、`后续追`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：先删帖后溯源的写法会写删帖、溯源与“先止损”这一自述理由，不把证据灭失这种后果词设必含。

### DSS06-netsec-9-2 · 让网信办帮溯源

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`溯源`、`取证`
- `bonusKeywords`：`协助`、`责任`、`日志`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：把溯源整体委托网信办的写法会出现网信办、溯源与取证，判据是本院取证责任是否落地，而非对方是否配合。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S06-1 | 正确 | — | 0 | 售卖帖、截图、哈希 | 发布时间、字段名、样本量级 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S06-2 | 正确 | F12 | 8 | 脱敏字段、条数、敏感标识 | 隔离环境、样本哈希、时间范围、不转存 | 40/25 | 对象、动作、结论 | P1 | 0.25/0.30 |
| S06-3 | 正确 | F3 | 8 | 定级依据\|定级理由\|分级依据\|判定依据、数据量级、重大 | 敏感字段、舆情热度、扩散速度、Ⅱ级 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S06-4 | 正确 | F4 | 8 | 指挥部、事件等级\|事件级别\|事件分级、启动 | 泄露范围、Ⅱ级、同步提交、初步来源 | 35/20 | 对象、动作、时间 | P4 | 0.25/0.30 |
| S06-5 | 正确 | F9 | 8 | 启动令、授权范围\|授权权限\|权限范围、响应级别\|响应等级\|响应分级 | 固证优先、删帖、报案、Ⅱ级 | 30/18 | 对象、动作、时间 | P8 | 0.25/0.30 |
| S06-6 | 正确 | F2 | 8 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、事件等级\|事件级别\|事件分级、回执 | 1小时、泄露范围、舆情、同步上报、续报 | 35/20 | 对象、动作、时间 | P4 | 0.25/0.30 |
| S06-7 | 正确 | F2 | 8 | 市网信办\|网信办\|市委网信办、哈希、事件等级\|事件级别\|事件分级 | 1小时、舆情、先固定、泄露范围 | 35/20 | 对象、动作、时间 | P4 | 0.25/0.30 |
| S06-8 | 正确 | F9 | 8 | 外泄接口、关闭、最小权限 | 访问日志、第三方、测试环境、先取证 | 35/20 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S06-9 | 正确 | F13 | 8 | 导出日志、固定、完整性校验\|完整性检查\|完整性核对 | 访问日志、账号清单、时间线、帖子快照 | 45/28 | 对象、时间、结论 | P5 | 0.25/0.30 |
| S06-10 | 正确 | F5 | 8 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、证据材料 | 先报案后补、受理编号、不等待外部意见 | 30/18 | 对象、时间 | P4 | 0.25/0.30 |
| S06-11 | 正确 | F9 | 8 | 口径、审批\|审核\|批准、患者信息 | 发言人、法务、已核实事实、监测 | 40/25 | 结论 | P4 | 0.25/0.30 |
| S06-12 | 正确 | F9 | 8 | 告知范围、法务、留痕 | 短信、公告、求助渠道、不等待外部口径 | 40/25 | 无 | P4 | 0.25/0.30 |
| S06-13 | 正确 | F8 | 6 | 结束条件\|终止条件\|结束标准\|终止标准、证据固定、闭环\|闭环管理\|办结\|归档 | 外泄接口、患者告知、备案、不得以删帖 | 35/20 | 对象、动作、结论 | P8 | 0.20/0.25 |
| S06-14 | 正确 | — | 6 | 受理编号\|受案编号\|案件编号\|回执编号、取证要求、回执 | 受理人、取证清单、代收 | 30/18 | 无 | P4 | 0.20/0.25 |
| S06-15 | 正确 | — | 6 | 时间线、整改项、责任人 | 告知覆盖率、复测、时限、归档 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS06-cac-11-1 | 混淆 | — | 5 | 内部自查、对外发声、舆情 | 自行核实、等结论、统一口径 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS06-cac-11-2 | 混淆 | — | 8 | 声明、致歉、法务 | 未审批、公开、舆情风险 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS06-commander-13-1 | 混淆 | — | 3 | 舆情平息、再宣布、结束 | 观察期、无复发、延迟 | 15/10 | 对象 | P7 | 0.10/0.15 |
| DSS06-commander-13-2 | 混淆 | — | 8 | 帖子、宣布结束、固证 | 未固证、未告知、外泄接口 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS06-commander-5-1 | 混淆 | — | 8 | 信息科、技术问题、舆情 | 告知、报案、暂不启动 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS06-commander-5-2 | 混淆 | — | 8 | 网信办、介入、启动 | 响应空转、等待、暂不启动 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-duty-1-1 | 混淆 | — | 3 | 帖子、条数、量级 | 未截图、未固证、时间点 | 15/10 | 对象 | P1 | 0.10/0.15 |
| DSS06-duty-1-2 | 混淆 | — | 8 | 网信办、删帖、先行 | 证据灭失、减少扩散、后续再说 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS06-duty-2-1 | 混淆 | — | 3 | 时间点、导出日志、账号 | HIS、吻合、来源 | 15/10 | 对象 | P1 | 0.10/0.15 |
| DSS06-itlead-10-1 | 混淆 | — | 8 | 材料齐全、报案、时限 | 先报案后补、立案、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-itlead-10-2 | 混淆 | — | 8 | 网信办、结论、报案 | 再决定、立案、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-itlead-3-1 | 混淆 | — | 3 | 数据类型、分级、舆情 | Ⅱ级、量级、热度 | 15/10 | 对象 | P3 | 0.10/0.15 |
| DSS06-itlead-3-2 | 混淆 | — | 8 | 泄露来源、定级、响应 | 告知、源头、推迟 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-itlead-4-1 | 混淆 | — | 8 | 网信办、删帖、上报 | 1小时、证据灭失、时限 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-itlead-4-2 | 混淆 | — | 3 | 疑似、上报、余地 | 措辞、保守、后续核实 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS06-itlead-6-1 | 混淆 | — | 8 | 网信办、结果、同步 | 1小时、卫健、时限 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-itlead-6-2 | 混淆 | — | 3 | 初报、续报、时限 | 1小时、初步情况、完整 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS06-itlead-7-1 | 混淆 | — | 3 | 样本、哈希、网信办 | 二次扩散、未固定、脱敏 | 15/10 | 对象 | P2 | 0.10/0.15 |
| DSS06-itlead-7-2 | 混淆 | — | 8 | 删完帖、完整情况、上报 | 证据灭失、时限、先行 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-medlead-12-1 | 混淆 | — | 5 | 网信办、统一口径、告知 | 患者、延迟、尚可解释 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS06-medlead-12-2 | 混淆 | — | 8 | 患者、告知、被动 | 法律风险、信任、主动联系 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-netsec-2-2 | 混淆 | — | 5 | 字段、源头、导出日志 | HIS、表名、访问账号 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS06-netsec-8-1 | 混淆 | — | 5 | 同类接口、外泄、最小化 | 最小化、外泄、其他接口 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS06-netsec-8-2 | 混淆 | — | 8 | 源头、权限、持续 | 持续外泄、复核、删除权限 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS06-netsec-9-1 | 混淆 | — | 8 | 删帖、溯源、先止损 | 证据灭失、扩散、后续追 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS06-netsec-9-2 | 混淆 | — | 5 | 网信办、溯源、取证 | 协助、责任、日志 | 15/10 | 对象 | P4 | 0.15/0.20 |

---

## 4. 总量、分布与风险

### 4.1 S06 规则总量

- 节点 41：正确 15、混淆 26；规则条数 41，与节点一一对应，无遗漏无多余。
- 必含词合计 123 个，平均 3.0 个/节点；加分词合计 135 个。
- 长度档：15/10 共 12 个；20/12 共 14 个；30/18 共 4 个；35/20 共 6 个；40/25 共 4 个；45/28 共 1 个。
- 要素组合：对象 27 个；对象+动作 1 个；对象+时间 1 个；对象+结论 3 个；对象+动作+结论 3 个；对象+时间+结论 2 个；对象+动作+时间 4 个。
- 表单绑定（正确节点）：F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F8 1 个、F9 4 个、无表单 3 个。

### 4.2 风险最高的几类节点

1. **混淆节点（26 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S06-11`（统一对外口径，8 分）：文本是纯口径与审批表述，而引擎“对象”正则只认 `终端/主机/账号/系统/业务/邮箱/科室` 等具体名词或字母数字；参演人若通篇写“口径、发言人、不得披露”，会缺“对象”要素被 400 拦。这是本册风险最高的正确节点。
3. `DSS06-cac-11-1`（混淆“只做内部自查不对外发声”）：同上，且它是混淆节点——一旦 400，参演人连“执行混淆扣全分”都拿不到。建议演练提示里要求写清“涉及哪个系统/哪类患者数据”。
4. `S06-12`（患者告知与法律评估，8 分）：同样偏“沟通/法务”文本，对象要素依赖“患者/系统/数据范围”这类具体名词，提示里要给出可写的对象。
5. `S06-9`（固定泄露证据，8 分）：`售卖帖`、`样本哈希`、`截图` 是判据，写法较稳定；但“固定”不在动作词表，要素取对象+时间+结论，参演人需写出时间与结论收口。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（41 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S06-1": {
    "requiredKeywords": [
      "售卖帖",
      "截图",
      "哈希"
    ],
    "bonusKeywords": [
      "发布时间",
      "字段名",
      "样本量级"
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
  "S06-2": {
    "requiredKeywords": [
      "脱敏字段",
      "条数",
      "敏感标识"
    ],
    "bonusKeywords": [
      "隔离环境",
      "样本哈希",
      "时间范围",
      "不转存"
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
  "S06-3": {
    "requiredKeywords": [
      "定级依据|定级理由|分级依据|判定依据",
      "数据量级",
      "重大"
    ],
    "bonusKeywords": [
      "敏感字段",
      "舆情热度",
      "扩散速度",
      "Ⅱ级"
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
  "S06-4": {
    "requiredKeywords": [
      "指挥部",
      "事件等级|事件级别|事件分级",
      "启动"
    ],
    "bonusKeywords": [
      "泄露范围",
      "Ⅱ级",
      "同步提交",
      "初步来源"
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
  "S06-5": {
    "requiredKeywords": [
      "启动令",
      "授权范围|授权权限|权限范围",
      "响应级别|响应等级|响应分级"
    ],
    "bonusKeywords": [
      "固证优先",
      "删帖",
      "报案",
      "Ⅱ级"
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
  "S06-6": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "事件等级|事件级别|事件分级",
      "回执"
    ],
    "bonusKeywords": [
      "1小时",
      "泄露范围",
      "舆情",
      "同步上报",
      "续报"
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
  "S06-7": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "哈希",
      "事件等级|事件级别|事件分级"
    ],
    "bonusKeywords": [
      "1小时",
      "舆情",
      "先固定",
      "泄露范围"
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
  "S06-8": {
    "requiredKeywords": [
      "外泄接口",
      "关闭",
      "最小权限"
    ],
    "bonusKeywords": [
      "访问日志",
      "第三方",
      "测试环境",
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S06-9": {
    "requiredKeywords": [
      "导出日志",
      "固定",
      "完整性校验|完整性检查|完整性核对"
    ],
    "bonusKeywords": [
      "访问日志",
      "账号清单",
      "时间线",
      "帖子快照"
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
  "S06-10": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "证据材料"
    ],
    "bonusKeywords": [
      "先报案后补",
      "受理编号",
      "不等待外部意见"
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
  "S06-11": {
    "requiredKeywords": [
      "口径",
      "审批|审核|批准",
      "患者信息"
    ],
    "bonusKeywords": [
      "发言人",
      "法务",
      "已核实事实",
      "监测"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S06-12": {
    "requiredKeywords": [
      "告知范围",
      "法务",
      "留痕"
    ],
    "bonusKeywords": [
      "短信",
      "公告",
      "求助渠道",
      "不等待外部口径"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
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
    "textQualityRatio": 0.25,
    "keywordPenaltyRatio": 0.3
  },
  "S06-13": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "证据固定",
      "闭环|闭环管理|办结|归档"
    ],
    "bonusKeywords": [
      "外泄接口",
      "患者告知",
      "备案",
      "不得以删帖"
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
  "S06-14": {
    "requiredKeywords": [
      "受理编号|受案编号|案件编号|回执编号",
      "取证要求",
      "回执"
    ],
    "bonusKeywords": [
      "受理人",
      "取证清单",
      "代收"
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
  "S06-15": {
    "requiredKeywords": [
      "时间线",
      "整改项",
      "责任人"
    ],
    "bonusKeywords": [
      "告知覆盖率",
      "复测",
      "时限",
      "归档"
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
  "DSS06-cac-11-1": {
    "requiredKeywords": [
      "内部自查",
      "对外发声",
      "舆情"
    ],
    "bonusKeywords": [
      "自行核实",
      "等结论",
      "统一口径"
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
  "DSS06-cac-11-2": {
    "requiredKeywords": [
      "声明",
      "致歉",
      "法务"
    ],
    "bonusKeywords": [
      "未审批",
      "公开",
      "舆情风险"
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
  "DSS06-commander-13-1": {
    "requiredKeywords": [
      "舆情平息",
      "再宣布",
      "结束"
    ],
    "bonusKeywords": [
      "观察期",
      "无复发",
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
  "DSS06-commander-13-2": {
    "requiredKeywords": [
      "帖子",
      "宣布结束",
      "固证"
    ],
    "bonusKeywords": [
      "未固证",
      "未告知",
      "外泄接口"
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
  "DSS06-commander-5-1": {
    "requiredKeywords": [
      "信息科",
      "技术问题",
      "舆情"
    ],
    "bonusKeywords": [
      "告知",
      "报案",
      "暂不启动"
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
  "DSS06-commander-5-2": {
    "requiredKeywords": [
      "网信办",
      "介入",
      "启动"
    ],
    "bonusKeywords": [
      "响应空转",
      "等待",
      "暂不启动"
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
  "DSS06-duty-1-1": {
    "requiredKeywords": [
      "帖子",
      "条数",
      "量级"
    ],
    "bonusKeywords": [
      "未截图",
      "未固证",
      "时间点"
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
  "DSS06-duty-1-2": {
    "requiredKeywords": [
      "网信办",
      "删帖",
      "先行"
    ],
    "bonusKeywords": [
      "证据灭失",
      "减少扩散",
      "后续再说"
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
  "DSS06-duty-2-1": {
    "requiredKeywords": [
      "时间点",
      "导出日志",
      "账号"
    ],
    "bonusKeywords": [
      "HIS",
      "吻合",
      "来源"
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
  "DSS06-itlead-10-1": {
    "requiredKeywords": [
      "材料齐全",
      "报案",
      "时限"
    ],
    "bonusKeywords": [
      "先报案后补",
      "立案",
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
  "DSS06-itlead-10-2": {
    "requiredKeywords": [
      "网信办",
      "结论",
      "报案"
    ],
    "bonusKeywords": [
      "再决定",
      "立案",
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
  "DSS06-itlead-3-1": {
    "requiredKeywords": [
      "数据类型",
      "分级",
      "舆情"
    ],
    "bonusKeywords": [
      "Ⅱ级",
      "量级",
      "热度"
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
  "DSS06-itlead-3-2": {
    "requiredKeywords": [
      "泄露来源",
      "定级",
      "响应"
    ],
    "bonusKeywords": [
      "告知",
      "源头",
      "推迟"
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
  "DSS06-itlead-4-1": {
    "requiredKeywords": [
      "网信办",
      "删帖",
      "上报"
    ],
    "bonusKeywords": [
      "1小时",
      "证据灭失",
      "时限"
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
  "DSS06-itlead-4-2": {
    "requiredKeywords": [
      "疑似",
      "上报",
      "余地"
    ],
    "bonusKeywords": [
      "措辞",
      "保守",
      "后续核实"
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
  "DSS06-itlead-6-1": {
    "requiredKeywords": [
      "网信办",
      "结果",
      "同步"
    ],
    "bonusKeywords": [
      "1小时",
      "卫健",
      "时限"
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
  "DSS06-itlead-6-2": {
    "requiredKeywords": [
      "初报",
      "续报",
      "时限"
    ],
    "bonusKeywords": [
      "1小时",
      "初步情况",
      "完整"
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
  "DSS06-itlead-7-1": {
    "requiredKeywords": [
      "样本",
      "哈希",
      "网信办"
    ],
    "bonusKeywords": [
      "二次扩散",
      "未固定",
      "脱敏"
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
  },
  "DSS06-itlead-7-2": {
    "requiredKeywords": [
      "删完帖",
      "完整情况",
      "上报"
    ],
    "bonusKeywords": [
      "证据灭失",
      "时限",
      "先行"
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
  "DSS06-medlead-12-1": {
    "requiredKeywords": [
      "网信办",
      "统一口径",
      "告知"
    ],
    "bonusKeywords": [
      "患者",
      "延迟",
      "尚可解释"
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
  "DSS06-medlead-12-2": {
    "requiredKeywords": [
      "患者",
      "告知",
      "被动"
    ],
    "bonusKeywords": [
      "法律风险",
      "信任",
      "主动联系"
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
  "DSS06-netsec-2-2": {
    "requiredKeywords": [
      "字段",
      "源头",
      "导出日志"
    ],
    "bonusKeywords": [
      "HIS",
      "表名",
      "访问账号"
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
  "DSS06-netsec-8-1": {
    "requiredKeywords": [
      "同类接口",
      "外泄",
      "最小化"
    ],
    "bonusKeywords": [
      "最小化",
      "外泄",
      "其他接口"
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
  "DSS06-netsec-8-2": {
    "requiredKeywords": [
      "源头",
      "权限",
      "持续"
    ],
    "bonusKeywords": [
      "持续外泄",
      "复核",
      "删除权限"
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
  "DSS06-netsec-9-1": {
    "requiredKeywords": [
      "删帖",
      "溯源",
      "先止损"
    ],
    "bonusKeywords": [
      "证据灭失",
      "扩散",
      "后续追"
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
  "DSS06-netsec-9-2": {
    "requiredKeywords": [
      "网信办",
      "溯源",
      "取证"
    ],
    "bonusKeywords": [
      "协助",
      "责任",
      "日志"
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
  }
}
```
