# S08 处置文字质量规则清单（44 节点）

> 身份：网络安全应急演练专家（医疗行业应急响应 + 桌面推演设计）
> 范围：现网 `script_nodes` 中 S08 全部 44 个节点（correct 16 / distractor 28），剧本「恶意代码批量感染办公网」
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
| T1 轻量 | 15/10 | DSS08-commander-14-1、DSS08-duty-1-1、DSS08-itlead-12-2、DSS08-itlead-3-1 |
| T2 标准 | 20/12 | DSS08-commander-14-2、DSS08-commander-5-1、DSS08-commander-5-2、DSS08-duty-1-2 |
| T3 记录 | 30/18 | S08-1、S08-12、S08-15 |
| T4 关键 | 35/20 | S08-10、S08-13、S08-14、S08-3 |
| T5 复杂 | 40/25 | S08-11、S08-16、S08-2、S08-5 |
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

## 1. 正确节点规则（16 个）

### S08-1 · 办公网多台终端杀毒告警

`correct` ｜ 类型 `alert` ｜ 表单 `无` ｜ 分值 0 ｜ 超时 5 min

- `requiredKeywords`：`恶意代码`、`感染终端`、`共享盘`
- `bonusKeywords`：`首次发现时间`、`样本哈希`、`未重启`、`影响科室`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、动作
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：首报只登记告警事实，取检出同一恶意代码、感染终端与共享盘传播迹象三条；0分节点，规则仅作提交闸门。

### S08-2 · 核实感染终端数量与传播方式

`correct` ｜ 类型 `task` ｜ 表单 `F12` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`感染终端`、`传播方式`、`样本哈希`
- `bonusKeywords`：`服务器`、`医疗设备`、`终端清单`、`共享盘`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、结论
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：核实要交出感染终端清单、样本哈希与传播方式，并确认是否波及服务器与医疗设备，三词正落在这三处。

### S08-3 · 预警研判：确认一般（Ⅳ级）事件

`correct` ｜ 类型 `task` ｜ 表单 `F3` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`定级依据|定级理由|分级依据|判定依据`、`业务网`、`一般`
- `bonusKeywords`：`感染终端数`、`全院预警`、`扩散趋势`、`Ⅳ级`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：定级要写明依据、是否波及业务网并给出一般等级词；Ⅳ级因罗马数字写法差异只放加分。

### S08-4 · 向院应急指挥部报告并建议启动预案

`correct` ｜ 类型 `report` ｜ 表单 `F4` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`指挥部`、`上报`、`事件等级|事件级别|事件分级`
- `bonusKeywords`：`启动`、`Ⅳ级`、`感染终端数`、`已采取措施`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：院内报告的核心是向指挥部上报事件等级，故取此三词；请求启动留在加分，不要求结论。

### S08-5 · 院应急指挥部批准启动Ⅳ级应急响应

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 8 ｜ 超时 10 min

- `requiredKeywords`：`响应级别|响应等级|响应分级`、`授权范围|授权权限|权限范围`、`启动`
- `bonusKeywords`：`Ⅳ级`、`启动时间`、`岗位分工`、`全员预警`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、动作
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：批复的可核查产物是响应级别与授权范围（隔离/查杀/取证/预警），故取这三词，启动时间作加分。

### S08-6 · 隔离感染终端与共享盘

`correct` ｜ 类型 `task` ｜ 表单 `F10` ｜ 分值 8 ｜ 超时 15 min

- `requiredKeywords`：`感染终端`、`隔离`、`共享盘写入`
- `bonusKeywords`：`隔离清单`、`核心服务器`、`保留运行状态`、`执行人`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.25 / 0.30**
- 设计理由：隔离必须写清感染终端、隔离动作与暂停共享盘写入三件事，保留运行状态用于取证放加分。

### S08-7 · 同时报市卫生健康局

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市卫生健康局|市卫健局|卫健委|卫生健康局`、`上报`、`感染范围`
- `bonusKeywords`：`双报`、`2小时`、`不等回复`、`已采取措施`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：报卫健的要点是接收单位、上报动作与感染范围；双报与2小时写法敏感，只进加分词。

### S08-8 · 同时报市网信办

`correct` ｜ 类型 `report` ｜ 表单 `F2` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`市网信办|网信办|市委网信办`、`上报`、`感染范围`
- `bonusKeywords`：`处置进展`、`2小时`、`不等回复`、`同一时点`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：与报卫健刻意错开，取市网信办/上报/感染范围；“同一时点”属报送质量表现，与 S07-7/S07-8 一致放加分词，处置进展作加分。

### S08-9 · 全院预警：禁止 U 盘、打开可疑文件

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 10 min

- `requiredKeywords`：`全院预警`、`禁用|禁止|封禁|不插入|不使用|不得使用|不要插入`、`回执`
- `bonusKeywords`：`OA`、`短信`、`病毒库`、`可疑文件`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：全院预警验收覆盖面与回执，故取全院预警/禁用/回执；U盘写法不统一，改用禁用并降为加分。

### S08-10 · 查杀恶意代码与清除启动项

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 20 min

- `requiredKeywords`：`查杀`、`启动项`、`复扫|复查|再次扫描|二次扫描`
- `bonusKeywords`：`计划任务`、`残留清单`、`重启`、`取证后`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：查杀节点取查杀/启动项/复扫，判据是复扫无残留而非杀过，计划任务与残留清单作加分。

### S08-11 · 取证样本与传播链

`correct` ｜ 类型 `task` ｜ 表单 `F13` ｜ 分值 7 ｜ 超时 25 min

- `requiredKeywords`：`恶意样本`、`样本哈希`、`共享盘访问日志`
- `bonusKeywords`：`启动项快照`、`隔离环境`、`禁止运行`、`保全位置`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：取证要覆盖恶意样本、样本哈希与共享盘访问日志，隔离环境保存与禁止运行是边界，放加分。

### S08-12 · 按规定评估并报市网安大队（满足条件即报）

`correct` ｜ 类型 `task` ｜ 表单 `F5` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`市网安大队|网安大队|公安网安|市公安网安`、`报案`、`受理编号|受案编号|案件编号|回执编号`
- `bonusKeywords`：`报案条件`、`证据材料`、`不等外部意见`、`后续补报`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：对象、时间
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：报案的可核查产物是受理编号，故取市网安大队/报案/受理编号；报案条件与不等待外部意见作加分。

### S08-13 · 补丁加固与全院终端复查

`correct` ｜ 类型 `task` ｜ 表单 `F9` ｜ 分值 7 ｜ 超时 25 min

- `requiredKeywords`：`补丁`、`复查`、`离线终端`
- `bonusKeywords`：`病毒库`、`医疗设备工作站`、`未覆盖清单`、`修复漏洞`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、动作、结论
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：加固复查要写补丁、复查与离线终端覆盖，医疗设备工作站与未覆盖清单是最易漏的边界。

### S08-14 · 应急结束：院应急指挥部宣布Ⅳ级响应结束

`correct` ｜ 类型 `report` ｜ 表单 `F8` ｜ 分值 7 ｜ 超时 15 min

- `requiredKeywords`：`结束条件|终止条件|结束标准|终止标准`、`复扫|复查|再次扫描|二次扫描`、`受理回执|受案回执|回执单|回执`
- `bonusKeywords`：`共享盘`、`离线终端`、`宣布结束`、`留存记录`
- `minMeasureLen` / `minResultLen`：**35 / 20**
- `requiredElements`：对象、结论
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.30**
- 设计理由：结束要逐条核对四类条件，取结束条件/复扫/受理回执；宣布动作不在引擎动作词表，故要素只留对象与结论。

### S08-15 · 接收市网安大队报案受理回执

`correct` ｜ 类型 `report` ｜ 表单 `F9` ｜ 分值 6 ｜ 超时 15 min

- `requiredKeywords`：`受理回执|受案回执|回执单|回执`、`受理编号|受案编号|案件编号|回执编号`、`补充材料`
- `bonusKeywords`：`补报时限`、`代录入`、`核对`
- `minMeasureLen` / `minResultLen`：**30 / 18**
- `requiredElements`：**无（纯接收/指导类，不要求要素）**
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：外部受理只读记录，必含受理回执/受理编号/补充材料，要素仅对象，避免把收到回执判成缺动作。

### S08-16 · 应急结束：Ⅳ级响应终止与复盘归档

`correct` ｜ 类型 `end` ｜ 表单 `无` ｜ 分值 6 ｜ 超时 10 min

- `requiredKeywords`：`响应终止`、`复盘`、`归档`
- `bonusKeywords`：`终止条件`、`时间线`、`整改`、`复测`
- `minMeasureLen` / `minResultLen`：**40 / 25**
- `requiredElements`：对象、时间、结论
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.20 / 0.25**
- 设计理由：结束与复盘合并节点，取响应终止/复盘/归档三词；时间线、整改与复测作加分，不硬套全部要件。

---

## 2. 混淆节点规则（28 个，按“该错误动作本身的合理写法”设计）

> 混淆节点的 `requiredElements` 统一只要求 `对象`。原因：混淆节点的质量规则同样会作为提交硬门槛（`drill_v1.go:470-476`），而引擎“动作/结论”正则只覆盖有限动词；如果按正确处置的动词设要素，选了混淆的人会因“动作词不在词表”被 400 卡住，反而拿不到“执行混淆扣全分”。区分度全部交给三个必含词。

### DSS08-commander-14-1 · 等全院终端复查通过一周后再宣布

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`全院终端`、`复查`、`宣布结束`
- `bonusKeywords`：`一周`、`闭环`、`确保干净`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误动作是等全院终端复查满一周再宣布，叙述必然出现全院终端、复查与宣布结束三词；5分混淆取15/10。

### DSS08-commander-14-2 · 病毒杀完就结束

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`病毒`、`结束`、`响应`
- `bonusKeywords`：`查杀`、`未取证`、`未复查`、`闭环`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该选项把病毒杀完直接当成响应结束，故取病毒/结束/响应三词；未取证未复查是它的代价，只放加分词。

### DSS08-commander-5-1 · 让信息科自己处理

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`信息科`、`自行处理`、`启动指令`
- `bonusKeywords`：`指挥体系`、`全院预警`、`缺位`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P8 审批/结束档，`fillerMaxLen` 8）：无、已审批、同意、已阅、收到、按要求处理、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误点是让信息科自行处理且不下正式启动指令，必含词正落在信息科/自行处理/启动指令，指挥体系缺位作加分。

### DSS08-commander-5-2 · 等感染更多了再说

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`感染`、`启动`、`观望`
- `bonusKeywords`：`扩散`、`等待`、`感染终端`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该选项主张先观望再启动，写法自然落在感染/启动/观望三个词；扩散与等待作加分，不取正确动作的词。

### DSS08-duty-1-1 · 只看病毒名，不核对哈希与传播方式

`distractor` ｜ 分值 5 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`病毒名`、`样本哈希`、`传播方式`
- `bonusKeywords`：`比对`、`同源变种`、`误判`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只比对病毒名而不核哈希，必含词取病毒名/样本哈希/传播方式，保证照descr写就能命中。

### DSS08-duty-1-2 · 等更多终端报告告警再处理

`distractor` ｜ 分值 8 ｜ 超时 5 min ｜ 表单 `无`

- `requiredKeywords`：`终端告警`、`等待`、`处置`
- `bonusKeywords`：`扩散`、`误报`、`更多`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等更多终端告警才处置，取终端告警/等待/处置；8分混淆取20/12，只作提交闸门。

### DSS08-itlead-12-1 · 不报，内部处理

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`内部处理`、`报案`、`刑事案件`
- `bonusKeywords`：`隐瞒`、`风险`、`网安大队`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：该选项主张不报案、内部处理，必含内部处理/报案/刑事案件，恰好覆盖它隐瞒的理由。

### DSS08-itlead-12-2 · 只口头报案，不提交样本与日志

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`口头报案`、`样本`、`日志`
- `bonusKeywords`：`立案`、`取证`、`书面材料`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只口头报案且不交证据，故取口头报案/样本/日志，立案与取证作为加分。

### DSS08-itlead-3-1 · 只看台数，不看波及范围

`distractor` ｜ 分值 5 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`感染台数`、`分级`、`业务网`
- `bonusKeywords`：`10 台`、`医疗设备`、`波及范围`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P3 研判档，`fillerMaxLen` 8）：无、已研判、已定级、按预案、正常、已知悉、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该选项只按感染台数套等级，必含感染台数/分级/业务网；10 台写法敏感，只能进加分词。

### DSS08-itlead-3-2 · 等查清楚传播方式再定

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`传播方式`、`定级`、`查清`
- `bonusKeywords`：`隔离`、`预警`、`滞后`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等查清传播方式再定级，取传播方式/定级/查清，隔离与预警滞后作加分。

### DSS08-itlead-4-1 · 等感染终端隔离完再报

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`感染终端`、`隔离`、`上报`
- `bonusKeywords`：`指挥部`、`启动`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等感染终端隔离完再上报，必含三词正好对应它的对象、动作与等待理由。

### DSS08-itlead-4-2 · 先报"办公网疑似中毒"

`distractor` ｜ 分值 3 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`办公网`、`疑似中毒`、`上报`
- `bonusKeywords`：`保守措辞`、`信息要素`、`及时`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该选项只报办公网疑似中毒这一保守措辞，固定写法就是办公网/疑似中毒/上报；3分取15/10。

### DSS08-itlead-7-1 · 等网信办那边同步了再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`市网信办`、`卫健`、`同步`
- `bonusKeywords`：`2小时`、`上报时限`、`延误`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等网信办同步再报卫健，用卫健而非全称，避免参演人写简称漏命中；2小时只进加分。

### DSS08-itlead-7-2 · 只发初步情况，不写等级与范围，后续不续报

`distractor` ｜ 分值 3 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`初步情况`、`事件等级`、`续报`
- `bonusKeywords`：`影响范围`、`简略`、`时限`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.10 / 0.15**
- 设计理由：该选项只发初步情况且后续不续报，故取初步情况/事件等级/续报，影响范围缺失作加分。

### DSS08-itlead-8-1 · 未经安全通道直接把活体样本邮件外发

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`活体样本`、`邮件外发`、`安全通道`
- `bonusKeywords`：`泄露`、`二次传播`、`风险`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是把活体样本用邮件直发，必含活体样本/邮件外发/安全通道，泄露与二次传播作加分。

### DSS08-itlead-8-2 · 等处置完再报

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`处置`、`上报`、`延误`
- `bonusKeywords`：`毁证`、`时限`、`处置完再报`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等处置完再报，取处置/上报/延误三词；毁证属于后果词，只放加分不设必含。

### DSS08-itlead-9-1 · 只发内部 IT 群，不覆盖临床科室

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`临床科室`、`通知`、`终端用户`
- `bonusKeywords`：`内部群`、`覆盖面`、`遗漏`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P4 上报/指导记录档，`fillerMaxLen` 10）：无、已知悉、已阅、收到、按要求处理、处理完毕、同上、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只在内部群通知，未覆盖的两类对象才是关键，故取临床科室/通知/终端用户。

### DSS08-itlead-9-2 · 等网信办确认病毒传播方式再通知

`distractor` ｜ 分值 8 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`网信办`、`传播方式`、`预警`
- `bonusKeywords`：`确认`、`延迟`、`终端`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等网信办确认传播方式再预警，取网信办/传播方式/预警，预警延迟作加分。

### DSS08-netsec-11-1 · 先处置完业务再说取证

`distractor` ｜ 分值 8 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`处置`、`取证`、`样本`
- `bonusKeywords`：`启动项`、`清除`、`溯源`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是先处置业务再取证，必含处置/取证/样本；启动项被清除导致无法溯源，只放加分。

### DSS08-netsec-11-2 · 让厂商帮忙取证

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`厂商`、`取证`、`保全`
- `bonusKeywords`：`代为`、`依赖外部`、`本院`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P5 取证档，`fillerMaxLen` 8）：无、已取证、已保全、已完成、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：该选项把取证外包给厂商，取厂商/取证/保全三词，强调本院保全不可替代，代为只放加分。

### DSS08-netsec-2-1 · 只看 EDR 分布图，不核实服务器与医疗设备

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`分布图`、`服务器`、`医疗设备`
- `bonusKeywords`：`EDR`、`受感染`、`核实`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只看EDR分布图，取分布图/服务器/医疗设备；EDR大小写混排，只能放在加分词。

### DSS08-netsec-2-2 · 只问当事人，不查 EDR 与共享盘日志

`distractor` ｜ 分值 5 ｜ 超时 10 min ｜ 表单 `无`

- `requiredKeywords`：`当事人`、`共享盘`、`传播方式`
- `bonusKeywords`：`EDR`、`访谈`、`访问日志`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P1 核实档，`fillerMaxLen` 8）：无、已处理、正常、已知悉、已阅、收到、无异常、已确认、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只访谈当事人，必含当事人/共享盘/传播方式，EDR与访问日志是它漏查的证据源。

### DSS08-netsec-6-1 · 等终端没人用了再隔离

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`终端`、`无人使用`、`隔离`
- `bonusKeywords`：`横向传播`、`等待`、`持续`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法是等终端无人使用再隔离，取终端/无人使用/隔离，横向传播作加分。

### DSS08-netsec-6-2 · 只改共享盘权限，终端不隔离

`distractor` ｜ 分值 8 ｜ 超时 15 min ｜ 表单 `无`

- `requiredKeywords`：`共享盘权限`、`隔离`、`终端`
- `bonusKeywords`：`横向传播`、`只改`、`仍可`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法只改共享盘权限而不隔离终端，必含三词正好暴露它只做了一半。

### DSS08-sysadmin-10-1 · 让终端上的员工自己用杀毒软件扫

`distractor` ｜ 分值 8 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`终端员工`、`杀毒软件`、`扫描`
- `bonusKeywords`：`责任下放`、`清除不彻底`、`自行`
- `minMeasureLen` / `minResultLen`：**20 / 12**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.25**
- 设计理由：错误做法把扫描责任下放给终端员工，取终端员工/杀毒软件/扫描，清除不彻底作加分。

### DSS08-sysadmin-10-2 · 直接重装系统，不留样本与启动项

`distractor` ｜ 分值 5 ｜ 超时 20 min ｜ 表单 `无`

- `requiredKeywords`：`重装系统`、`样本`、`启动项`
- `bonusKeywords`：`证据链`、`保全`、`彻底`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法直接重装系统，必含重装系统/样本/启动项，证据链破坏只放加分。

### DSS08-sysadmin-13-1 · 只给感染过的终端补补丁

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`补丁`、`离线终端`、`医疗设备`
- `bonusKeywords`：`感染终端`、`工作站`、`未覆盖`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P2 处置档，`fillerMaxLen` 8）：无、已处理、已完成、已隔离、正常、收到、处理完毕、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法只给感染过的终端打补丁，取补丁/离线终端/医疗设备，这三类正是它漏掉的覆盖对象。

### DSS08-sysadmin-13-2 · 等应急结束后再统一补补丁

`distractor` ｜ 分值 5 ｜ 超时 25 min ｜ 表单 `无`

- `requiredKeywords`：`应急结束`、`补丁`、`统一`
- `bonusKeywords`：`加固`、`延迟`、`可逆`
- `minMeasureLen` / `minResultLen`：**15 / 10**
- `requiredElements`：对象
- `fillerWords`（P7 观望/延误档，`fillerMaxLen` 6）：无、已观察、等待中、已在处理、正常、ok、OK
- `textQualityRatio` / `keywordPenaltyRatio`：**0.15 / 0.20**
- 设计理由：错误做法是等应急结束后再统一补丁，必含应急结束/补丁/统一，加固延迟可逆作加分。

---

## 3. 一页汇总表

| 节点 | flow | 表单 | 分 | 必含词 | 加分词 | 措施/结果 | 要素 | 敷衍档 | 质量/关键词 |
|---|---|---|---|---|---|---|---|---|---|
| S08-1 | 正确 | — | 0 | 恶意代码、感染终端、共享盘 | 首次发现时间、样本哈希、未重启、影响科室 | 30/18 | 对象、动作 | P1 | 0.20/0.30 |
| S08-2 | 正确 | F12 | 8 | 感染终端、传播方式、样本哈希 | 服务器、医疗设备、终端清单、共享盘 | 40/25 | 对象、结论 | P1 | 0.25/0.30 |
| S08-3 | 正确 | F3 | 8 | 定级依据\|定级理由\|分级依据\|判定依据、业务网、一般 | 感染终端数、全院预警、扩散趋势、Ⅳ级 | 35/20 | 对象、结论 | P3 | 0.25/0.30 |
| S08-4 | 正确 | F4 | 7 | 指挥部、上报、事件等级\|事件级别\|事件分级 | 启动、Ⅳ级、感染终端数、已采取措施 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S08-5 | 正确 | F9 | 8 | 响应级别\|响应等级\|响应分级、授权范围\|授权权限\|权限范围、启动 | Ⅳ级、启动时间、岗位分工、全员预警 | 40/25 | 对象、动作 | P8 | 0.25/0.30 |
| S08-6 | 正确 | F10 | 8 | 感染终端、隔离、共享盘写入 | 隔离清单、核心服务器、保留运行状态、执行人 | 35/20 | 对象、动作、结论 | P2 | 0.25/0.30 |
| S08-7 | 正确 | F2 | 7 | 市卫生健康局\|市卫健局\|卫健委\|卫生健康局、上报、感染范围 | 双报、2小时、不等回复、已采取措施 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S08-8 | 正确 | F2 | 7 | 市网信办\|网信办\|市委网信办、上报、感染范围 | 处置进展、2小时、不等回复、同一时点 | 35/20 | 对象、动作、时间 | P4 | 0.20/0.30 |
| S08-9 | 正确 | F9 | 7 | 全院预警、禁用\|禁止\|封禁\|不插入\|不使用\|不得使用\|不要插入、回执 | OA、短信、病毒库、可疑文件 | 40/25 | 对象、结论 | P4 | 0.20/0.30 |
| S08-10 | 正确 | F9 | 7 | 查杀、启动项、复扫\|复查\|再次扫描\|二次扫描 | 计划任务、残留清单、重启、取证后 | 35/20 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S08-11 | 正确 | F13 | 7 | 恶意样本、样本哈希、共享盘访问日志 | 启动项快照、隔离环境、禁止运行、保全位置 | 40/25 | 对象、时间、结论 | P5 | 0.20/0.30 |
| S08-12 | 正确 | F5 | 6 | 市网安大队\|网安大队\|公安网安\|市公安网安、报案、受理编号\|受案编号\|案件编号\|回执编号 | 报案条件、证据材料、不等外部意见、后续补报 | 30/18 | 对象、时间 | P4 | 0.20/0.25 |
| S08-13 | 正确 | F9 | 7 | 补丁、复查、离线终端 | 病毒库、医疗设备工作站、未覆盖清单、修复漏洞 | 35/20 | 对象、动作、结论 | P2 | 0.20/0.30 |
| S08-14 | 正确 | F8 | 7 | 结束条件\|终止条件\|结束标准\|终止标准、复扫\|复查\|再次扫描\|二次扫描、受理回执\|受案回执\|回执单\|回执 | 共享盘、离线终端、宣布结束、留存记录 | 35/20 | 对象、结论 | P8 | 0.20/0.30 |
| S08-15 | 正确 | F9 | 6 | 受理回执\|受案回执\|回执单\|回执、受理编号\|受案编号\|案件编号\|回执编号、补充材料 | 补报时限、代录入、核对 | 30/18 | 无 | P4 | 0.20/0.25 |
| S08-16 | 正确 | — | 6 | 响应终止、复盘、归档 | 终止条件、时间线、整改、复测 | 40/25 | 对象、时间、结论 | P4 | 0.20/0.25 |
| DSS08-commander-14-1 | 混淆 | — | 5 | 全院终端、复查、宣布结束 | 一周、闭环、确保干净 | 15/10 | 对象 | P7 | 0.15/0.20 |
| DSS08-commander-14-2 | 混淆 | — | 8 | 病毒、结束、响应 | 查杀、未取证、未复查、闭环 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS08-commander-5-1 | 混淆 | — | 8 | 信息科、自行处理、启动指令 | 指挥体系、全院预警、缺位 | 20/12 | 对象 | P8 | 0.15/0.25 |
| DSS08-commander-5-2 | 混淆 | — | 8 | 感染、启动、观望 | 扩散、等待、感染终端 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-duty-1-1 | 混淆 | — | 5 | 病毒名、样本哈希、传播方式 | 比对、同源变种、误判 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS08-duty-1-2 | 混淆 | — | 8 | 终端告警、等待、处置 | 扩散、误报、更多 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-itlead-12-1 | 混淆 | — | 8 | 内部处理、报案、刑事案件 | 隐瞒、风险、网安大队 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS08-itlead-12-2 | 混淆 | — | 5 | 口头报案、样本、日志 | 立案、取证、书面材料 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS08-itlead-3-1 | 混淆 | — | 5 | 感染台数、分级、业务网 | 10 台、医疗设备、波及范围 | 15/10 | 对象 | P3 | 0.15/0.20 |
| DSS08-itlead-3-2 | 混淆 | — | 8 | 传播方式、定级、查清 | 隔离、预警、滞后 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-itlead-4-1 | 混淆 | — | 8 | 感染终端、隔离、上报 | 指挥部、启动、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-itlead-4-2 | 混淆 | — | 3 | 办公网、疑似中毒、上报 | 保守措辞、信息要素、及时 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS08-itlead-7-1 | 混淆 | — | 8 | 市网信办、卫健、同步 | 2小时、上报时限、延误 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-itlead-7-2 | 混淆 | — | 3 | 初步情况、事件等级、续报 | 影响范围、简略、时限 | 15/10 | 对象 | P4 | 0.10/0.15 |
| DSS08-itlead-8-1 | 混淆 | — | 8 | 活体样本、邮件外发、安全通道 | 泄露、二次传播、风险 | 20/12 | 对象 | P4 | 0.15/0.25 |
| DSS08-itlead-8-2 | 混淆 | — | 8 | 处置、上报、延误 | 毁证、时限、处置完再报 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-itlead-9-1 | 混淆 | — | 5 | 临床科室、通知、终端用户 | 内部群、覆盖面、遗漏 | 15/10 | 对象 | P4 | 0.15/0.20 |
| DSS08-itlead-9-2 | 混淆 | — | 8 | 网信办、传播方式、预警 | 确认、延迟、终端 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-netsec-11-1 | 混淆 | — | 8 | 处置、取证、样本 | 启动项、清除、溯源 | 20/12 | 对象 | P5 | 0.15/0.25 |
| DSS08-netsec-11-2 | 混淆 | — | 5 | 厂商、取证、保全 | 代为、依赖外部、本院 | 15/10 | 对象 | P5 | 0.15/0.20 |
| DSS08-netsec-2-1 | 混淆 | — | 5 | 分布图、服务器、医疗设备 | EDR、受感染、核实 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS08-netsec-2-2 | 混淆 | — | 5 | 当事人、共享盘、传播方式 | EDR、访谈、访问日志 | 15/10 | 对象 | P1 | 0.15/0.20 |
| DSS08-netsec-6-1 | 混淆 | — | 8 | 终端、无人使用、隔离 | 横向传播、等待、持续 | 20/12 | 对象 | P7 | 0.15/0.25 |
| DSS08-netsec-6-2 | 混淆 | — | 8 | 共享盘权限、隔离、终端 | 横向传播、只改、仍可 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS08-sysadmin-10-1 | 混淆 | — | 8 | 终端员工、杀毒软件、扫描 | 责任下放、清除不彻底、自行 | 20/12 | 对象 | P2 | 0.15/0.25 |
| DSS08-sysadmin-10-2 | 混淆 | — | 5 | 重装系统、样本、启动项 | 证据链、保全、彻底 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS08-sysadmin-13-1 | 混淆 | — | 5 | 补丁、离线终端、医疗设备 | 感染终端、工作站、未覆盖 | 15/10 | 对象 | P2 | 0.15/0.20 |
| DSS08-sysadmin-13-2 | 混淆 | — | 5 | 应急结束、补丁、统一 | 加固、延迟、可逆 | 15/10 | 对象 | P7 | 0.15/0.20 |

---

## 4. 总量、分布与风险

### 4.1 S08 规则总量

- 节点 44：正确 16、混淆 28；规则条数 44，与节点一一对应，无遗漏无多余。
- 必含词合计 132 个，平均 3.0 个/节点；加分词合计 148 个。
- 长度档：15/10 共 13 个；20/12 共 15 个；30/18 共 3 个；35/20 共 8 个；40/25 共 5 个。
- 要素组合：对象 29 个；对象+动作 2 个；对象+动作+结论 3 个；对象+时间+结论 2 个；对象+时间 1 个；对象+结论 4 个；对象+动作+时间 3 个。
- 表单绑定（正确节点）：F10 1 个、F12 1 个、F13 1 个、F2 2 个、F3 1 个、F4 1 个、F5 1 个、F8 1 个、F9 5 个、无表单 2 个。

### 4.2 风险最高的几类节点

1. **混淆节点（28 个）**：它们的规则同时是提交硬门槛。若必含词与其错误动作的自然写法不符，参演人会选了混淆却提交不了，反而拿不到“执行混淆扣分”，演练设计失效。本轮已全部按其 `descr/guide` 的错误逻辑取词，并把长度压到 15/10–20/12、要素只留 `对象`。
2. `S08-5`（批准启动Ⅳ级，8 分）：要素含“动作”，题面的授权范围是“隔离/查杀/取证/预警”，其中 `隔离`、`查杀` 在动作词表内，`取证`、`预警` 不在；参演人若只写“批准启动、明确分工”，不写授权动作，会被 400 拦。
3. `S08-2`（核实感染终端与传播方式，8 分）：要素含“结论”，要求出现 `确认/完成/正常/成功` 这类收口词；只写“核查了终端清单与样本哈希”会被判缺要素。
4. `S08-9`（全院预警：禁止 U 盘、打开可疑文件）：`U 盘` 含空格且大小写敏感，只作加分词；必含词用稳定中文短语，但参演人若只写“已发通知”会缺两项。
5. `S08-12`（按规定评估并报案，6 分）：报案动词不在动作词表，要素取对象+时间；且“满足条件即报”的判断依据要写进文本，否则只剩对象一项支撑。
6. **写法敏感词**：本册涉及的罗马数字、时间、设备型号、英文缩写等一律只进加分词，必含词保持稳定中文短语；但参演人按加分词的写法作答时仍可能因全半角/空格差异漏命中，建议后续在引擎侧加一次全半角与空格归一化（属代码改动，本轮不做）。

---

## 附录 A：可直接落库的 `quality_rules` JSON（44 条）

> 仅文档内嵌，未写库。键为节点 `id`，值为该节点 `script_nodes.quality_rules` 的完整 JSON。

```json
{
  "S08-1": {
    "requiredKeywords": [
      "恶意代码",
      "感染终端",
      "共享盘"
    ],
    "bonusKeywords": [
      "首次发现时间",
      "样本哈希",
      "未重启",
      "影响科室"
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
  "S08-2": {
    "requiredKeywords": [
      "感染终端",
      "传播方式",
      "样本哈希"
    ],
    "bonusKeywords": [
      "服务器",
      "医疗设备",
      "终端清单",
      "共享盘"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
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
  "S08-3": {
    "requiredKeywords": [
      "定级依据|定级理由|分级依据|判定依据",
      "业务网",
      "一般"
    ],
    "bonusKeywords": [
      "感染终端数",
      "全院预警",
      "扩散趋势",
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
  "S08-4": {
    "requiredKeywords": [
      "指挥部",
      "上报",
      "事件等级|事件级别|事件分级"
    ],
    "bonusKeywords": [
      "启动",
      "Ⅳ级",
      "感染终端数",
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
  "S08-5": {
    "requiredKeywords": [
      "响应级别|响应等级|响应分级",
      "授权范围|授权权限|权限范围",
      "启动"
    ],
    "bonusKeywords": [
      "Ⅳ级",
      "启动时间",
      "岗位分工",
      "全员预警"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
      "动作"
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
  "S08-6": {
    "requiredKeywords": [
      "感染终端",
      "隔离",
      "共享盘写入"
    ],
    "bonusKeywords": [
      "隔离清单",
      "核心服务器",
      "保留运行状态",
      "执行人"
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
  "S08-7": {
    "requiredKeywords": [
      "市卫生健康局|市卫健局|卫健委|卫生健康局",
      "上报",
      "感染范围"
    ],
    "bonusKeywords": [
      "双报",
      "2小时",
      "不等回复",
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
  "S08-8": {
    "requiredKeywords": [
      "市网信办|网信办|市委网信办",
      "上报",
      "感染范围"
    ],
    "bonusKeywords": [
      "处置进展",
      "2小时",
      "不等回复",
      "同一时点"
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
  "S08-9": {
    "requiredKeywords": [
      "全院预警",
      "禁用|禁止|封禁|不插入|不使用|不得使用|不要插入",
      "回执"
    ],
    "bonusKeywords": [
      "OA",
      "短信",
      "病毒库",
      "可疑文件"
    ],
    "minMeasureLen": 40,
    "minResultLen": 25,
    "requiredElements": [
      "对象",
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
    "keywordPenaltyRatio": 0.3
  },
  "S08-10": {
    "requiredKeywords": [
      "查杀",
      "启动项",
      "复扫|复查|再次扫描|二次扫描"
    ],
    "bonusKeywords": [
      "计划任务",
      "残留清单",
      "重启",
      "取证后"
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
  "S08-11": {
    "requiredKeywords": [
      "恶意样本",
      "样本哈希",
      "共享盘访问日志"
    ],
    "bonusKeywords": [
      "启动项快照",
      "隔离环境",
      "禁止运行",
      "保全位置"
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
    "keywordPenaltyRatio": 0.3
  },
  "S08-12": {
    "requiredKeywords": [
      "市网安大队|网安大队|公安网安|市公安网安",
      "报案",
      "受理编号|受案编号|案件编号|回执编号"
    ],
    "bonusKeywords": [
      "报案条件",
      "证据材料",
      "不等外部意见",
      "后续补报"
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
  "S08-13": {
    "requiredKeywords": [
      "补丁",
      "复查",
      "离线终端"
    ],
    "bonusKeywords": [
      "病毒库",
      "医疗设备工作站",
      "未覆盖清单",
      "修复漏洞"
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
  "S08-14": {
    "requiredKeywords": [
      "结束条件|终止条件|结束标准|终止标准",
      "复扫|复查|再次扫描|二次扫描",
      "受理回执|受案回执|回执单|回执"
    ],
    "bonusKeywords": [
      "共享盘",
      "离线终端",
      "宣布结束",
      "留存记录"
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
    "keywordPenaltyRatio": 0.3
  },
  "S08-15": {
    "requiredKeywords": [
      "受理回执|受案回执|回执单|回执",
      "受理编号|受案编号|案件编号|回执编号",
      "补充材料"
    ],
    "bonusKeywords": [
      "补报时限",
      "代录入",
      "核对"
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
  "S08-16": {
    "requiredKeywords": [
      "响应终止",
      "复盘",
      "归档"
    ],
    "bonusKeywords": [
      "终止条件",
      "时间线",
      "整改",
      "复测"
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
  "DSS08-commander-14-1": {
    "requiredKeywords": [
      "全院终端",
      "复查",
      "宣布结束"
    ],
    "bonusKeywords": [
      "一周",
      "闭环",
      "确保干净"
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
  "DSS08-commander-14-2": {
    "requiredKeywords": [
      "病毒",
      "结束",
      "响应"
    ],
    "bonusKeywords": [
      "查杀",
      "未取证",
      "未复查",
      "闭环"
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
  "DSS08-commander-5-1": {
    "requiredKeywords": [
      "信息科",
      "自行处理",
      "启动指令"
    ],
    "bonusKeywords": [
      "指挥体系",
      "全院预警",
      "缺位"
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
  "DSS08-commander-5-2": {
    "requiredKeywords": [
      "感染",
      "启动",
      "观望"
    ],
    "bonusKeywords": [
      "扩散",
      "等待",
      "感染终端"
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
  "DSS08-duty-1-1": {
    "requiredKeywords": [
      "病毒名",
      "样本哈希",
      "传播方式"
    ],
    "bonusKeywords": [
      "比对",
      "同源变种",
      "误判"
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
  "DSS08-duty-1-2": {
    "requiredKeywords": [
      "终端告警",
      "等待",
      "处置"
    ],
    "bonusKeywords": [
      "扩散",
      "误报",
      "更多"
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
  "DSS08-itlead-12-1": {
    "requiredKeywords": [
      "内部处理",
      "报案",
      "刑事案件"
    ],
    "bonusKeywords": [
      "隐瞒",
      "风险",
      "网安大队"
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
  "DSS08-itlead-12-2": {
    "requiredKeywords": [
      "口头报案",
      "样本",
      "日志"
    ],
    "bonusKeywords": [
      "立案",
      "取证",
      "书面材料"
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
  "DSS08-itlead-3-1": {
    "requiredKeywords": [
      "感染台数",
      "分级",
      "业务网"
    ],
    "bonusKeywords": [
      "10 台",
      "医疗设备",
      "波及范围"
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
  "DSS08-itlead-3-2": {
    "requiredKeywords": [
      "传播方式",
      "定级",
      "查清"
    ],
    "bonusKeywords": [
      "隔离",
      "预警",
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
  "DSS08-itlead-4-1": {
    "requiredKeywords": [
      "感染终端",
      "隔离",
      "上报"
    ],
    "bonusKeywords": [
      "指挥部",
      "启动",
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
  "DSS08-itlead-4-2": {
    "requiredKeywords": [
      "办公网",
      "疑似中毒",
      "上报"
    ],
    "bonusKeywords": [
      "保守措辞",
      "信息要素",
      "及时"
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
  "DSS08-itlead-7-1": {
    "requiredKeywords": [
      "市网信办",
      "卫健",
      "同步"
    ],
    "bonusKeywords": [
      "2小时",
      "上报时限",
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
  "DSS08-itlead-7-2": {
    "requiredKeywords": [
      "初步情况",
      "事件等级",
      "续报"
    ],
    "bonusKeywords": [
      "影响范围",
      "简略",
      "时限"
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
  "DSS08-itlead-8-1": {
    "requiredKeywords": [
      "活体样本",
      "邮件外发",
      "安全通道"
    ],
    "bonusKeywords": [
      "泄露",
      "二次传播",
      "风险"
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
  "DSS08-itlead-8-2": {
    "requiredKeywords": [
      "处置",
      "上报",
      "延误"
    ],
    "bonusKeywords": [
      "毁证",
      "时限",
      "处置完再报"
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
  "DSS08-itlead-9-1": {
    "requiredKeywords": [
      "临床科室",
      "通知",
      "终端用户"
    ],
    "bonusKeywords": [
      "内部群",
      "覆盖面",
      "遗漏"
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
  "DSS08-itlead-9-2": {
    "requiredKeywords": [
      "网信办",
      "传播方式",
      "预警"
    ],
    "bonusKeywords": [
      "确认",
      "延迟",
      "终端"
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
  "DSS08-netsec-11-1": {
    "requiredKeywords": [
      "处置",
      "取证",
      "样本"
    ],
    "bonusKeywords": [
      "启动项",
      "清除",
      "溯源"
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
  "DSS08-netsec-11-2": {
    "requiredKeywords": [
      "厂商",
      "取证",
      "保全"
    ],
    "bonusKeywords": [
      "代为",
      "依赖外部",
      "本院"
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
  "DSS08-netsec-2-1": {
    "requiredKeywords": [
      "分布图",
      "服务器",
      "医疗设备"
    ],
    "bonusKeywords": [
      "EDR",
      "受感染",
      "核实"
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
  "DSS08-netsec-2-2": {
    "requiredKeywords": [
      "当事人",
      "共享盘",
      "传播方式"
    ],
    "bonusKeywords": [
      "EDR",
      "访谈",
      "访问日志"
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
  "DSS08-netsec-6-1": {
    "requiredKeywords": [
      "终端",
      "无人使用",
      "隔离"
    ],
    "bonusKeywords": [
      "横向传播",
      "等待",
      "持续"
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
  "DSS08-netsec-6-2": {
    "requiredKeywords": [
      "共享盘权限",
      "隔离",
      "终端"
    ],
    "bonusKeywords": [
      "横向传播",
      "只改",
      "仍可"
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
  "DSS08-sysadmin-10-1": {
    "requiredKeywords": [
      "终端员工",
      "杀毒软件",
      "扫描"
    ],
    "bonusKeywords": [
      "责任下放",
      "清除不彻底",
      "自行"
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
  "DSS08-sysadmin-10-2": {
    "requiredKeywords": [
      "重装系统",
      "样本",
      "启动项"
    ],
    "bonusKeywords": [
      "证据链",
      "保全",
      "彻底"
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
  "DSS08-sysadmin-13-1": {
    "requiredKeywords": [
      "补丁",
      "离线终端",
      "医疗设备"
    ],
    "bonusKeywords": [
      "感染终端",
      "工作站",
      "未覆盖"
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
  "DSS08-sysadmin-13-2": {
    "requiredKeywords": [
      "应急结束",
      "补丁",
      "统一"
    ],
    "bonusKeywords": [
      "加固",
      "延迟",
      "可逆"
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
