# Design Contract — 盘州市人民医院网络安全应急演练系统原型

## 技术栈与交付形态
- vanilla HTML + CSS + JS，纯静态、零依赖、无外部CDN/字体/图标库，双击 index.html 即可运行
- 项目目录：e:\Trae\Emergency\cyber-drill-prototype\
- 多页面：每模块一个 .html，共享 styles.css / app.js / mock.js / api.js（相对路径引用）

## 风格层级
tech-dark 指挥中心风格（深海军蓝 + 蓝色主色 + 状态色），贴合医院安全应急态势场景。

## 设计令牌（styles.css :root）
- 背景: --bg:#0a1220; --panel:#111c2e; --elev:#16263d; --hover:#1b2e49
- 边框: --border:#1f3350; --border2:#2c4a72
- 文本: --text:#e6edf7; --text2:#8fa3bf; --text3:#5c7396
- 主色: --accent:#2f81f7; --accent2:#4d9bff; --accent-dim:rgba(47,129,247,.15)
- 语义: --ok:#3fb950; --warn:#d29922; --err:#f85149; --info:#58a6ff; --purple:#a371f7
- 等级: P0=err P1=warn P2=info
- 圆角: 6/10/14; 间距: 4/8/12/16/24/32; 阴影: 0 8px 24px rgba(0,0,0,.35)
- 字体: "Microsoft YaHei","PingFang SC",system-ui; 等宽: Consolas,monospace

## App Shell（唯一骨架，app.js 渲染，禁止各页自写导航）
- body 属性: data-page="模块key"；app.js DOMContentLoaded 时 App.boot(pageKey)
- 结构: .sidebar(232px 固定左) + .main-wrap(.topbar 56px + #page 内容区 padding 24px)
- sidebar: 品牌区(盾形logo+系统名) → 导航分组(演练执行/编排与资源/复盘与报送/系统管理) → 底部用户卡(角色名+退出)
- topbar: 左=当前演练项目选择器；中=态势徽标(演练进行中/等级)；右=时钟+消息铃(未读数)+角色chip+退出
- 导航项按 ROLES[cur].nav 过滤；active 规则: pageKey===navKey 加 .active
- 外部角色(healthbureau/cac/cyberpolice) nav 仅含受限模块 → 天然看不到技术日志/内网原始数据页

## 页面清单（14 页）
1. index.html 登录：角色选择(12卡片)+账号密码+验证码+锁定提示；写 sessionStorage 后跳转 dashboard.html
2. dashboard.html 态势总看板：KPI/事件等级/剧本时间轴进度/角色任务完成矩阵/上报文书清单/业务影响/消息流（commander/itlead/evaluator/platformadmin 可见）
3. tasks.html 任务协同中心：待办列表(状态/超时标记)+任务详情抽屉(处置记录/附件/表单/流转)+人工触发消息+超时提醒演示
4. workbench.html 分角色工作台：按当前角色渲染差异化面板(12种)，所有处置动作=模拟记录弹窗，无真实命令
5. collab.html 会商中心：院内会商组/跨部门联合会商组 双页签聊天，文字+附件，归档提示
6. scenarios.html 场景剧本库：8内置场景卡片+搜索筛选+版本/启停/复制另存/导入导出+新建自定义
7. script-editor.html 可视化剧本编排：节点画布(告警/任务/分支/上报/兜底/恢复)+拖拽排序+节点配置抽屉(消息/接收角色/附件/表单/超时)+难度(基础/实战)+分支判断
8. projects.html 演练项目管理：项目列表+新建向导(名称/类型/单位/剧本/考评规则)+编组(角色-人员/观察员/考评员)+状态机(未开始→演练中→暂停→已结束→已归档)+启停控制+预演
9. evidence.html 取证与材料归档：自动采集流(操作日志/聊天/表单/附件)+手动取证包上传(类型大小校验/禁恶意样本)+关键节点快照+一键打包档案
10. replay.html 时序回放：时间轴+播放控制(倍速/暂停/定位)+回放事件流+自动生成处置时序图(告警→核实→上报→研判→处置→兜底→上报→报案→恢复→终止)
11. evaluation.html 复盘评估：指标库打分(定性+扣分规则)+评语/改进建议+评分汇总+演练报告预览(概况/时序/问题/建议/评分/附件)+导出Word/PDF(模拟)
12. ledger.html 台账统计：历史演练检索(时间/类型/事件/结果)+统计图(年度次数/事件频次/短板/角色平均响应)+导出台账Excel(模拟)
13. opinion.html 公告与舆情模拟：舆情消息流→网信办角色接收+院方公告文稿提交→指挥部审批→口径审核记录+处置动作登记
14. knowledge.html 预案知识库：预案/流程图/兜底流程/通讯录文档列表+在线预览抽屉+全角色可查
15. admin.html 系统管理：用户账号/角色权限矩阵/组织机构/表单模板管理/审计日志(仅查看不可删)/消息通知配置/数据备份

## Mock schema（mock.js 全局 MOCK 对象）
- ROLES: 12角色 {id,name,short,desc,nav[],external?,color}
  id: commander/duty/netsec/sysadmin/itlead/medlead/clinical/healthbureau/cac/cyberpolice/evaluator/platformadmin
- USERS, SCENARIOS(8,含nodes[]), PROJECTS(4,含状态机与编组), TASKS(按角色,含超时), MESSAGES(告警流), CHAT(两组), FORM_TEMPLATES(7), EVIDENCE, SNAPSHOTS, TIMELINE(回放事件), SEQ_FLOW(时序图节点), SCORE_ITEMS(9指标), SCORES, LEDGER(6), OPINION, KNOWLEDGE, AUDIT_LOGS, NOTIFS, IMPACT(业务影响), DICT(事件等级/类型)
- 全部虚拟测试数据，无真实患者信息；文案标注"演练模拟"

## API 桩（api.js 全局 API）
- 会话: login/logout/curRole()/curUser()（sessionStorage key: pzy_role, pzy_user）
- 数据: 各 getXxx(roleId) 返回 Promise(setTimeout 120-300ms 模拟延迟)
- 写操作: submitTask/actSim(模拟处置)/sendChat/addEvidence/score/saveScript/... 内存态变更+Toast反馈，关键状态(任务完成/项目状态)持久 localStorage
- 守卫: App.boot 未登录跳回 index.html；无权限页显示 403 面板

## 组件规范（styles.css 类）
btn(primary/ghost/outline/danger/sm/icon) input select textarea card panel table tag(5色) badge modal drawer tabs kpi timeline chat-bubble progress form-grid empty toast section-title mono divider dot
图标: app.js ICONS 内联SVG(约22个) 无外部库；图表: 手写SVG柱/环/折线 helper 于 app.js

## 反AI-slop
- 不用emoji当图标；不堆渐变霓虹；文案用医院/卫健真实语域(上报文书/督办/兜底)；每页有真实业务密度数据
