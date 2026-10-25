/* 盘州市人民医院网络安全应急演练系统 · 统一 Mock 数据源（全部为虚拟演练数据，不含真实患者信息） */
window.MOCK = (function(){
"use strict";

var ROLES = [
  { id:'commander', name:'院应急指挥部', short:'指挥部', desc:'分管院长牵头：研判决策、启动/终止预案、审批上报与口径', nav:['dashboard','workbench','collab','projects','evidence','replay','evaluation','ledger','opinion','knowledge'], color:'#2f81f7' },
  { id:'duty', name:'信息科值班员', short:'值班员', desc:'接收模拟告警、核实记录、第一时间上报，无封堵/系统操作权限', nav:['workbench','collab','knowledge'], color:'#58a6ff' },
  { id:'netsec', name:'网络安全管理员', short:'网安员', desc:'网络侧模拟处置：IP封堵、ACL调整、断网隔离、流量排查、样本上传', nav:['workbench','collab','evidence','knowledge'], color:'#a371f7' },
  { id:'sysadmin', name:'系统管理员', short:'系统员', desc:'核心系统模拟处置：HIS/EMR/LIS/PACS停机、灾备切换、备份校验、恢复', nav:['workbench','collab','evidence','knowledge'], color:'#3fb950' },
  { id:'itlead', name:'信息科负责人', short:'信息科长', desc:'科内统筹、组织研判、汇总简报、向指挥部汇报、分配技术任务', nav:['dashboard','workbench','collab','evidence','replay','knowledge'], color:'#d29922' },
  { id:'medlead', name:'医务部/门诊部负责人', short:'医务部', desc:'业务影响评估、组织纸质兜底流程、患者风险评估、临床工作指引', nav:['workbench','collab','knowledge'], color:'#f85149' },
  { id:'clinical', name:'临床科室/收费窗口', short:'临床窗口', desc:'患者沟通解释、公告张贴、诉求投诉登记、业务卡点上报', nav:['workbench','collab','knowledge'], color:'#e3b341' },
  { id:'healthbureau', name:'市卫生健康局', short:'卫健局', desc:'行业主管：接收上报、督办处置、统筹区域资源、查看上报文书（仅报送材料）', nav:['workbench','collab','knowledge'], external:true, color:'#56d364' },
  { id:'cac', name:'市网信办', short:'网信办', desc:'舆情管控指导、依法监管、接收事件通报、口径指导（仅报送材料）', nav:['workbench','collab','opinion','knowledge'], external:true, color:'#39c5cf' },
  { id:'cyberpolice', name:'市网安大队', short:'网安大队', desc:'接收报案材料、登记报案、取证指引、固定证据、反馈受理状态（仅报送材料）', nav:['workbench','collab','evidence','knowledge'], external:true, color:'#ff7b72' },
  { id:'platformadmin', name:'平台系统管理员', short:'平台管理员', desc:'账号/角色/剧本/字典维护、日志审计、数据备份，不参与业务推演', nav:['scenarios','projects','ledger','admin','knowledge'], color:'#8f9aa8' }
];

var USERS = [
  { name:'张副院长', role:'commander', org:'盘州市人民医院应急指挥部' },
  { name:'李明', role:'duty', org:'信息科监控值班室' },
  { name:'王强', role:'netsec', org:'信息科网络安全组' },
  { name:'赵工', role:'sysadmin', org:'信息科系统运维组' },
  { name:'陈主任', role:'itlead', org:'信息科' },
  { name:'刘主任', role:'medlead', org:'医务部' },
  { name:'孙护士长', role:'clinical', org:'门诊收费窗口' },
  { name:'周科员', role:'healthbureau', org:'盘州市卫生健康局' },
  { name:'吴处长', role:'cac', org:'盘州市委网信办' },
  { name:'郑警官', role:'cyberpolice', org:'盘州市公安局网安大队' },
  { name:'平台管理员', role:'platformadmin', org:'信息科平台运维' }
];

var SCENARIOS = [
  { id:'S01', name:'勒索病毒攻击演练', type:'勒索病毒', level:'重大', status:'启用', version:'v2.1', updated:'2026-09-12', nodes:14, desc:'内网终端中招、文件加密、勒索提示，评估HIS服务器感染风险，推演断网隔离、备份恢复与上报流程。', alert:'T+0 监控平台推送紧急告警：HIS 门诊工作站与共享文件服务器出现大量文件被加密（扩展名变为 .locked），桌面弹出勒索信；已有 3 台终端无法打开业务系统，疑似勒索病毒正在院内网横向扩散。' },
  { id:'S02', name:'内网横向渗透/账号泄露', type:'内网渗透', level:'较大', status:'启用', version:'v1.3', updated:'2026-08-25', nodes:11, desc:'弱口令入侵后横向移动，电子病历数据遭非法访问，推演账号排查、终端隔离与取证。', alert:'T+0 监测发现电子病历系统存在异常账号登录与批量查询行为：同一账号在 10 分钟内从 4 台不同终端访问病历，疑似弱口令被利用并横向渗透，部分病历数据可能已被非法访问。' },
  { id:'S03', name:'DDoS攻击致门诊收费卡顿', type:'DDoS攻击', level:'较大', status:'启用', version:'v1.1', updated:'2026-07-30', nodes:9, desc:'门诊收费系统遭流量攻击卡顿，推演流量清洗协调、窗口纸质兜底与业务恢复确认。', alert:'T+0 门诊收费系统出现大面积卡顿并间歇性超时，外网出口流量异常激增（峰值约为日常的 30 倍），收费窗口排队积压，疑似遭受 DDoS 流量攻击。' },
  { id:'S04', name:'钓鱼邮件跳板入侵', type:'钓鱼邮件', level:'一般', status:'启用', version:'v2.0', updated:'2026-09-02', nodes:10, desc:'信息科人员点击木马邮件，攻击者获取跳板，推演邮件排查、全员通报与账号重置。', alert:'T+0 信息科多名人员收到伪装为“医保结算通知”的邮件并点击附件，随后终端出现可疑外联行为与异常进程，疑似钓鱼木马已在办公终端取得跳板权限。' },
  { id:'S05', name:'核心存储故障系统宕机', type:'系统宕机', level:'重大', status:'启用', version:'v1.5', updated:'2026-08-18', nodes:13, desc:'核心存储/数据库故障致HIS、EMR整体宕机，推演灾备切换、兜底流程与恢复验证。', alert:'T+0 核心存储阵列发出硬件告警，HIS、EMR 等业务系统整体无法访问，数据库连接持续失败，门诊挂号、收费与住院医嘱业务中断，需立即启动应急与业务兜底。' },
  { id:'S06', name:'患者隐私数据泄露（舆情发酵）', type:'数据泄露', level:'重大', status:'启用', version:'v1.2', updated:'2026-09-20', nodes:15, desc:'虚拟患者数据在外部论坛售卖引发舆情，推演上报、口径审批、舆情管控与报案取证。', alert:'T+0 监测发现虚拟患者数据在外部论坛被公开售卖，样本包含姓名、就诊科室与就诊记录；相关截图已在社交媒体传播，存在舆情快速发酵与监管问责风险。' },
  { id:'S07', name:'外网边界防火墙被突破', type:'边界突破', level:'较大', status:'启用', version:'v1.0', updated:'2026-06-28', nodes:12, desc:'边界设备被突破、多业务系统并发告警，推演边界收敛、策略调整与全网排查。', alert:'T+0 外网边界防火墙检测到异常穿透与策略篡改行为，多个业务系统同时出现异常访问告警，疑似边界设备已被突破并建立外联通道，需立即收敛边界并开展全网排查。' },
  { id:'S08', name:'恶意代码批量感染办公网', type:'恶意代码', level:'一般', status:'停用', version:'v1.4', updated:'2026-05-15', nodes:8, desc:'办公网终端批量感染恶意代码，业务网暂未波及，推演批量断网、样本摘要提取与查杀。', alert:'T+0 办公网多台终端批量弹出异常进程提示并生成可疑文件，终端资源占用异常升高；业务网暂未发现波及，疑似恶意代码在办公网批量感染。' }
];

var PROJECTS = [
  { id:'P2026-012', name:'2026年勒索病毒专项应急演练', type:'实战协同演练', status:'演练中', date:'2026-10-23 09:00', scenarioId:'S01', level:'重大', units:['盘州市人民医院','盘州市卫生健康局'], target:'检验勒索病毒场景下30分钟内断网隔离、2小时内完成上报的闭环能力', members:[{role:'commander',user:'张副院长'},{role:'duty',user:'李明'},{role:'netsec',user:'王强'},{role:'sysadmin',user:'赵工'},{role:'itlead',user:'陈主任'},{role:'medlead',user:'刘主任'},{role:'clinical',user:'孙护士长'},{role:'healthbureau',user:'周科员'}], observers:['院办 杨主任'] },
  { id:'P2026-011', name:'数据泄露舆情联合演练（卫健牵头）', type:'联合演练', status:'已结束', date:'2026-09-18 14:00', scenarioId:'S06', level:'重大', units:['盘州市人民医院','市卫健局','市网信办','市网安大队'], target:'多单位协同：上报、口径审批、报案取证、舆情管控全链路', members:[{role:'commander',user:'张副院长'},{role:'cac',user:'吴处长'},{role:'cyberpolice',user:'郑警官'}], observers:['纪检室 马主任'] },
  { id:'P2026-010', name:'门诊收费系统DDoS桌面推演', type:'桌面推演', status:'未开始', date:'2026-11-05 15:00', scenarioId:'S03', level:'较大', units:['盘州市人民医院'], target:'桌面推演验证窗口纸质兜底流程可操作性', members:[{role:'itlead',user:'陈主任'}], observers:[] },
  { id:'P2026-009', name:'钓鱼邮件防范季度演练', type:'桌面推演', status:'已归档', date:'2026-07-10 09:30', scenarioId:'S04', level:'一般', units:['盘州市人民医院'], target:'检验信息科值班员核实上报与全员通报时效', members:[{role:'duty',user:'李明'}], observers:[] }
];

var TASKS = [
  { id:'T101', projectId:'P2026-012', role:'duty', title:'核实终端异常加密告警', desc:'监控平台推送【演练模拟】HIS终端批量文件重命名告警，请核实并填写《事件初步核实单》', deadline:'09:15', status:'超时', level:'P0', forms:['F1'], atts:[] },
  { id:'T102', projectId:'P2026-012', role:'duty', title:'向信息科负责人上报初步情况', desc:'值班员第一时间上报，附初步截图证据', deadline:'09:20', status:'待处理', level:'P0', forms:[], atts:['告警截图.png'] },
  { id:'T103', projectId:'P2026-012', role:'netsec', title:'模拟封堵恶意IP并隔离感染终端', desc:'对10.20.x.x段恶意C2地址执行模拟封堵，隔离3台感染终端，提交网络处置记录', deadline:'09:40', status:'处理中', level:'P0', forms:['F2'], atts:[] },
  { id:'T104', projectId:'P2026-012', role:'sysadmin', title:'HIS数据库服务器模拟隔离与备份校验', desc:'确认勒索软件未扩散至数据库，执行备份有效性校验记录', deadline:'09:55', status:'待处理', level:'P1', forms:[], atts:[] },
  { id:'T105', projectId:'P2026-012', role:'itlead', title:'汇总技术研判并提交阶段性简报', desc:'组织研判事件等级，形成《网络安全事件上报表》提交院应急指挥部', deadline:'10:10', status:'待处理', level:'P0', forms:['F2'], atts:[] },
  { id:'T106', projectId:'P2026-012', role:'commander', title:'审批启动网络安全应急预案Ⅱ级响应', desc:'听取汇报后下达应急启动指令，审批对外口径', deadline:'10:20', status:'待处理', level:'P0', forms:['F4'], atts:[] },
  { id:'T107', projectId:'P2026-012', role:'medlead', title:'评估门诊/住院业务影响并启用纸质兜底', desc:'勾选受影响业务，登记纸质处方/登记/人工收费启动情况', deadline:'10:30', status:'待处理', level:'P1', forms:['F3'], atts:[] },
  { id:'T108', projectId:'P2026-012', role:'clinical', title:'窗口公告张贴与患者诉求登记', desc:'张贴系统维护公告，记录患者咨询与投诉，反馈纸质流程卡点', deadline:'10:40', status:'处理中', level:'P2', forms:[], atts:[] },
  { id:'T109', projectId:'P2026-012', role:'commander', title:'批准向市卫健局上报事件情况', desc:'审批《网络安全事件上报表》并流转至卫健局工作台', deadline:'11:00', status:'待处理', level:'P0', forms:['F2'], atts:[] },
  { id:'T110', projectId:'P2026-012', role:'healthbureau', title:'接收医院上报并下达督办意见', desc:'查看医院报送材料，填写督办意见（外部单位仅见报送材料）', deadline:'11:30', status:'待处理', level:'P1', forms:[], atts:[] },
  { id:'T111', projectId:'P2026-012', role:'sysadmin', title:'业务恢复验证并提交恢复确认单', desc:'模拟系统恢复完成，提交《业务恢复确认单》', deadline:'12:00', status:'待处理', level:'P2', forms:['F7'], atts:[] },
  { id:'T112', projectId:'P2026-011', role:'cyberpolice', title:'登记报案材料并出具受理回执', desc:'接收医院《报案登记表》，固定证据摘要，反馈受理状态', deadline:'15:30', status:'已完成', level:'P1', forms:['F5'], atts:['报案登记表.pdf'] },
  { id:'T113', projectId:'P2026-011', role:'cac', title:'舆情风险研判并下达管控要求', desc:'查看通报材料，填写《舆情风险研判表》并下达口径指导', deadline:'15:00', status:'已完成', level:'P1', forms:['F6'], atts:[] }
];

var MESSAGES = [
  { id:'M1', time:'09:02', level:'P0', source:'态势感知平台（模拟）', text:'【演练模拟】内网10.20.13.45等3台终端检出可疑加密进程，文件批量重命名为 .locked 后缀', targets:['duty','itlead'] },
  { id:'M2', time:'09:05', level:'P0', source:'态势感知平台（模拟）', text:'【演练模拟】检测到终端向境外IP 185.2xx.xx.77 的异常外联（疑似C2通信）', targets:['duty','netsec'] },
  { id:'M3', time:'09:18', level:'P1', source:'信息科值班员 李明', text:'已核实为勒索病毒特征，正在填写初步核实单并上报', targets:['itlead','commander'] },
  { id:'M4', time:'09:31', level:'P0', source:'网络安全管理员 王强', text:'【模拟】已对C2地址执行封堵，3台感染终端断网隔离', targets:['itlead','commander'] },
  { id:'M5', time:'09:47', level:'P1', source:'系统管理员 赵工', text:'数据库服务器未检出感染，备份校验中', targets:['itlead'] },
  { id:'M6', time:'10:05', level:'P0', source:'信息科负责人 陈主任', text:'综合研判为较大及以上事件，建议启动Ⅱ级响应并向指挥部汇报', targets:['commander'] },
  { id:'M7', time:'10:22', level:'P0', source:'院应急指挥部 张副院长', text:'同意启动应急预案Ⅱ级响应，请各科室按预案执行，同步准备上报材料', targets:['*'] },
  { id:'M8', time:'10:35', level:'P1', source:'医务部 刘主任', text:'门诊、收费受影响，已启用纸质处方与人工收费兜底流程', targets:['commander','clinical'] },
  { id:'M9', time:'11:02', level:'P1', source:'系统模拟推送', text:'超时提醒：值班员上报任务T101超时未完成，已提醒信息科负责人', targets:['itlead'] }
];

var CHAT = {
  inner: [ // 院内会商组
    { sender:'李明', role:'duty', time:'09:06', text:'收到告警，我先远程看一下终端情况，截图取证。' },
    { sender:'陈主任', role:'itlead', time:'09:08', text:'注意不要重启终端，保留现场，先做初步核实单。' },
    { sender:'王强', role:'netsec', time:'09:12', text:'我在防火墙上看到同段多个终端有异常外联，建议先断网。', att:'异常外联流量截图.png' },
    { sender:'赵工', role:'sysadmin', time:'09:15', text:'HIS数据库这边确认一下，存储快照是今早6点的，可用于恢复。' },
    { sender:'张副院长', role:'commander', time:'10:20', text:'启动Ⅱ级响应。信息科每30分钟报一次处置进展，医务部同步兜底情况。' },
    { sender:'系统', role:'sys', time:'10:21', text:'【演练模拟】院应急指挥部下达指令：启动Ⅱ级应急响应' }
  ],
  external: [ // 院外上报沟通组（外部单位与院内报送角色）
    { sender:'周科员', role:'healthbureau', time:'11:10', text:'收到医院上报材料。请保持处置进展同步，如需区域资源支持可提出。' },
    { sender:'陈主任', role:'itlead', time:'11:15', text:'目前感染已控制在办公网3台终端，业务网正在恢复验证。' },
    { sender:'郑警官', role:'cyberpolice', time:'11:22', text:'报案材料请包含：勒索信截图、样本哈希摘要、加密文件清单（虚拟数据）。', att:'取证指引单.pdf' },
    { sender:'吴处长', role:'cac', time:'11:28', text:'暂未监测到相关舆情扩散，对外口径请统一经指挥部审批后发布。' },
    { sender:'系统', role:'sys', time:'11:30', text:'【演练提示】外部单位仅可查看医院主动上报的材料，不可查看内网原始日志。' }
  ]
};

/* 内置表单模板（离线兜底；在线时以服务端 /api/bootstrap 下发为准） */
var FORM_TEMPLATES = [
  { id:'F1', name:'事件初步核实单', fields:[
    { label:'发现时间', hint:'以监控平台或值班记录中首次发现的时间为准，精确到分钟。', required:true, type:'datetime' },
    { label:'发现方式', hint:'如安全告警系统、值班巡检、临床科室报障、外部单位通报。', required:true, type:'select', options:['安全告警系统','值班巡检','临床科室报障','外部单位通报','第三方监测','其他'] },
    { label:'报告人', hint:'首先发现并报告的人员姓名及所属科室。', required:true, type:'text' },
    { label:'受影响系统', hint:'出现异常的业务系统名称，如 HIS、EMR、LIS、PACS。', required:true, type:'text' },
    { label:'IP/域名', hint:'涉及的主机 IP、域名或访问地址，多个用顿号分隔。', required:true, type:'text' },
    { label:'异常现象', hint:'客观描述观察到的现象，如文件被加密、异常外联、登录失败激增。', required:true, type:'textarea' },
    { label:'初步判断', hint:'结合告警与现象初判事件类型和等级，注明为模拟核实结论。', required:true, type:'textarea' },
    { label:'初步影响范围', hint:'已确认受影响的科室、业务和主机数量，未确认的标注「待评估」。', required:true, type:'textarea' },
    { label:'已采取临时措施', hint:'如暂时断开网线、暂停共享目录写入，注明执行人与时间。', required:true, type:'textarea' },
    { label:'核实人', hint:'完成初步核实的信息科值班人员姓名。', required:true, type:'text' },
    { label:'核实时间', hint:'完成初步核实的时间，精确到分钟。', required:true, type:'datetime' }
  ] },
  { id:'F2', name:'网络安全事件上报表', fields:[
    { label:'上报单位', hint:'填写医院全称，与系统单位名称一致。', required:true, type:'text' },
    { label:'事件类型', hint:'如勒索病毒、DDoS 攻击、数据泄露、系统宕机等。', required:true, type:'select', options:['勒索病毒','DDoS 攻击','数据泄露','系统宕机','钓鱼邮件','内网渗透','其他'] },
    { label:'事件等级', hint:'按预案分为特别重大/重大/较大/一般，与研判结论一致。', required:true, type:'select', options:['特别重大','重大','较大','一般'] },
    { label:'发生时间', hint:'事件实际发生时间；无法确认时填写首次发现时间并注明。', required:true, type:'datetime' },
    { label:'发现时间', hint:'首次发现事件的时间，精确到分钟。', required:true, type:'datetime' },
    { label:'受攻击系统', hint:'被攻击或受影响的业务系统名称。', required:true, type:'text' },
    { label:'IP/域名', hint:'受攻击资产 IP、域名或端口，多个用顿号分隔。', required:true, type:'text' },
    { label:'事件基本情况', hint:'按「何时、何地、何事、如何发现」叙述事件经过。', required:true, type:'textarea' },
    { label:'影响范围', hint:'受影响业务系统、科室、患者服务与数据范围（演练虚拟数据）。', required:true, type:'textarea' },
    { label:'已采取措施', hint:'已执行的隔离、封堵、停机、备份等处置动作及时间。', required:true, type:'textarea' },
    { label:'处置进展', hint:'当前处置阶段与预计完成时间。', required:true, type:'textarea' },
    { label:'下一步计划', hint:'拟开展的溯源、恢复、整改等计划安排。', required:true, type:'textarea' },
    { label:'报送人/联系电话', hint:'报送人姓名 + 虚拟联系电话。', required:true, type:'text' },
    { label:'报送时间', hint:'实际报送时间，精确到分钟。', required:true, type:'datetime' }
  ] },
  { id:'F3', name:'业务影响评估表', fields:[
    { label:'受影响业务系统', hint:'受影响的业务系统清单，多个用顿号分隔。', required:true, type:'text' },
    { label:'业务中断起止时间', hint:'格式如 2026-10-24 09:00 至 10:30；仍在中断填「持续中」。', required:true, type:'text' },
    { label:'受影响科室', hint:'受影响的临床、医技与窗口科室。', required:true, type:'textarea' },
    { label:'受影响患者数（虚拟）', hint:'仅填写演练虚拟数量，不得录入真实患者信息。', required:true, type:'text' },
    { label:'关键业务兜底方案', hint:'如纸质处方、人工挂号收费、手工登记，注明启动条件。', required:true, type:'textarea' },
    { label:'预计恢复时间', hint:'预计业务恢复时间。', required:true, type:'datetime' },
    { label:'评估人', hint:'完成评估的医务/门诊负责人。', required:true, type:'text' },
    { label:'评估时间', hint:'评估完成时间。', required:true, type:'datetime' }
  ] },
  { id:'F4', name:'应急启动审批单', fields:[
    { label:'事件摘要', hint:'简述事件事实、等级与当前影响。', required:true, type:'textarea' },
    { label:'建议响应等级', hint:'按预案填写 Ⅰ级/Ⅱ级/Ⅲ级/Ⅳ级。', required:true, type:'select', options:['Ⅰ级','Ⅱ级','Ⅲ级','Ⅳ级'] },
    { label:'启动依据', hint:'引用预案条款与研判结论。', required:true, type:'textarea' },
    { label:'拟采取措施', hint:'拟启动的响应措施与责任分工。', required:true, type:'textarea' },
    { label:'申请部门/申请人', hint:'提出启动申请的单位与人员。', required:true, type:'text' },
    { label:'审批人', hint:'具备启动审批权限的院领导。', required:true, type:'text' },
    { label:'审批意见', hint:'同意/不同意及补充要求。', required:true, type:'textarea' },
    { label:'审批时间', hint:'审批完成时间。', required:true, type:'datetime' }
  ] },
  { id:'F5', name:'报案登记表', fields:[
    { label:'报案单位', hint:'报案单位全称。', required:true, type:'text' },
    { label:'报案人/联系电话', hint:'报案人姓名 + 虚拟联系电话。', required:true, type:'text' },
    { label:'事件发现时间', hint:'首次发现事件的时间。', required:true, type:'datetime' },
    { label:'事件经过', hint:'按时间顺序叙述发现与处置经过。', required:true, type:'textarea' },
    { label:'受攻击系统', hint:'受攻击系统名称。', required:true, type:'text' },
    { label:'IP/域名', hint:'涉案 IP、域名与端口。', required:true, type:'text' },
    { label:'损失情况', hint:'仅描述演练虚拟损失，如中断时长、受影响终端数。', required:true, type:'textarea' },
    { label:'证据清单', hint:'已固定的日志、样本摘要、截图等材料名称。', required:true, type:'textarea' },
    { label:'报案时间', hint:'提交报案时间。', required:true, type:'datetime' }
  ] },
  { id:'F6', name:'舆情风险研判表', fields:[
    { label:'监测渠道', hint:'如社交媒体、论坛、短视频平台、上级通报。', required:true, type:'text' },
    { label:'舆情摘要', hint:'舆情主要内容与关键截图描述。', required:true, type:'textarea' },
    { label:'传播趋势', hint:'扩散速度、主要传播节点与当前热度。', required:true, type:'textarea' },
    { label:'风险等级', hint:'按研判结论填写 高/中/低。', required:true, type:'select', options:['高','中','低'] },
    { label:'口径建议', hint:'统一对外答复口径，不得透露真实患者信息。', required:true, type:'textarea' },
    { label:'研判人', hint:'参与研判的人员姓名。', required:true, type:'text' },
    { label:'研判时间', hint:'研判完成时间。', required:true, type:'datetime' }
  ] },
  { id:'F7', name:'业务恢复确认单', fields:[
    { label:'恢复系统清单', hint:'已完成恢复的系统清单。', required:true, type:'textarea' },
    { label:'数据完整性校验', hint:'备份校验方式、校验结果与 RPO 达成情况。', required:true, type:'textarea' },
    { label:'业务功能验证结果', hint:'逐项记录 HIS/EMR/LIS/PACS 验证结果。', required:true, type:'textarea' },
    { label:'恢复时间', hint:'业务恢复完成时间。', required:true, type:'datetime' },
    { label:'确认人', hint:'业务与信息科双确认人员。', required:true, type:'text' },
    { label:'确认时间', hint:'确认完成时间。', required:true, type:'datetime' }
  ] },
  { id:'F8', name:'网络和数据安全事件反馈单', fields:[
    { label:'受攻击系统', hint:'被攻击或受影响的系统名称及版本，多个用顿号分隔。', required:true, type:'textarea' },
    { label:'IP/域名', hint:'受攻击资产 IP、域名、端口，多个用顿号分隔。', required:true, type:'textarea' },
    { label:'事件基本情况', hint:'简述发生时间、发现方式、攻击手法与事件等级。', required:true, type:'textarea' },
    { label:'事件影响范围', hint:'受影响业务、科室、数据与患者服务范围（演练虚拟数据）。', required:true, type:'textarea' },
    { label:'现场处置过程', hint:'按时间顺序记录隔离、封堵、取证、恢复等动作及执行人。', required:true, type:'textarea' },
    { label:'事件处置结果', hint:'处置成效、业务恢复情况与残留风险。', required:true, type:'textarea' },
    { label:'下一步整改及工作计划', hint:'整改措施、责任部门、完成时限与复查安排。', required:true, type:'textarea' }
  ] }
];

var EVIDENCE = [
  { id:'E1', type:'操作日志', name:'值班员核实操作记录.log', uploader:'李明', role:'duty', time:'09:07', size:'12 KB', auto:true },
  { id:'E2', type:'附件截图', name:'告警截图.png', uploader:'李明', role:'duty', time:'09:09', size:'214 KB', auto:false },
  { id:'E3', type:'处置记录', name:'网络处置单-IP封堵', uploader:'王强', role:'netsec', time:'09:31', size:'8 KB', auto:false },
  { id:'E4', type:'样本摘要', name:'勒索信文本摘要.txt', uploader:'王强', role:'netsec', time:'09:36', size:'3 KB', auto:false },
  { id:'E5', type:'聊天记录', name:'院内会商组聊天记录', uploader:'系统', role:'sys', time:'10:30', size:'26 KB', auto:true },
  { id:'E6', type:'表单文书', name:'网络安全事件上报表(已审批)', uploader:'陈主任', role:'itlead', time:'10:58', size:'15 KB', auto:false },
  { id:'E7', type:'状态变更', name:'HIS系统状态切换记录', uploader:'赵工', role:'sysadmin', time:'11:40', size:'6 KB', auto:true }
];

var SNAPSHOTS = [
  { id:'SP1', name:'告警发现时刻', time:'09:02', by:'系统', note:'初始3台终端告警状态' },
  { id:'SP2', name:'应急启动决策', time:'10:20', by:'张副院长', note:'事件等级较大、Ⅱ级响应' },
  { id:'SP3', name:'兜底流程启用', time:'10:35', by:'刘主任', note:'门诊纸质流程登记' }
];

var TIMELINE = [
  { t:'09:02', actor:'态势感知平台', role:'sys', action:'推送模拟告警', detail:'3台终端检出可疑加密进程', level:'P0' },
  { t:'09:06', actor:'李明', role:'duty', action:'接收告警', detail:'值班员确认接收，开始核实' },
  { t:'09:15', actor:'李明', role:'duty', action:'超时未上报', detail:'核实单提交超时，系统提醒信息科负责人', level:'P1' },
  { t:'09:18', actor:'李明', role:'duty', action:'提交初步核实单', detail:'判定勒索病毒特征，附告警截图' },
  { t:'09:20', actor:'陈主任', role:'itlead', action:'组织研判', detail:'召集网安员、系统员线上会商' },
  { t:'09:31', actor:'王强', role:'netsec', action:'模拟IP封堵', detail:'封堵境外C2地址，隔离3台感染终端' },
  { t:'09:47', actor:'赵工', role:'sysadmin', action:'数据库隔离', detail:'确认DB未感染，校验备份有效性' },
  { t:'10:05', actor:'陈主任', role:'itlead', action:'提交研判简报', detail:'建议判定为较大事件，启动Ⅱ级响应' },
  { t:'10:20', actor:'张副院长', role:'commander', action:'启动应急预案', detail:'审批Ⅱ级响应，统筹全院资源' },
  { t:'10:35', actor:'刘主任', role:'medlead', action:'启用业务兜底', detail:'纸质处方、人工收费、窗口公告' },
  { t:'11:02', actor:'张副院长', role:'commander', action:'批准上报', detail:'《网络安全事件上报表》流转市卫健局' },
  { t:'11:10', actor:'周科员', role:'healthbureau', action:'接收并督办', detail:'查看报送材料，下达督办意见' },
  { t:'11:22', actor:'郑警官', role:'cyberpolice', action:'报案登记', detail:'接收报案登记表，出具取证指引' },
  { t:'11:40', actor:'赵工', role:'sysadmin', action:'系统恢复', detail:'HIS/EMR模拟恢复完成，业务验证通过' },
  { t:'11:55', actor:'孙护士长', role:'clinical', action:'窗口恢复确认', detail:'收费窗口恢复线上，纸质流程撤销' },
  { t:'12:05', actor:'张副院长', role:'commander', action:'终止演练', detail:'确认闭环完成，演练结束进入复盘' }
];

var SEQ_FLOW = [
  { name:'告警发现', time:'09:02', role:'态势平台', state:'done' },
  { name:'核实研判', time:'09:18', role:'值班员/信息科', state:'done' },
  { name:'启动预案', time:'10:20', role:'指挥部', state:'done' },
  { name:'网络处置', time:'09:31', role:'网安员', state:'done' },
  { name:'业务兜底', time:'10:35', role:'医务部/窗口', state:'done' },
  { name:'上报主管', time:'11:02', role:'指挥部→卫健', state:'done' },
  { name:'报案取证', time:'11:22', role:'网安大队', state:'done' },
  { name:'恢复上线', time:'11:40', role:'系统管理员', state:'done' },
  { name:'复盘终止', time:'12:05', role:'全院', state:'done' }
];

var SCORE_ITEMS = [
  { id:'K1', name:'响应时效', weight:15, rule:'告警发现后10分钟内完成核实上报；超时每分钟扣1分' },
  { id:'K2', name:'上报合规性', weight:15, rule:'使用标准《网络安全事件上报表》，要素齐全；缺必要表单扣5分/项' },
  { id:'K3', name:'处置步骤完整性', weight:15, rule:'隔离→封堵→排查→恢复顺序正确；处置顺序错误扣5分/次' },
  { id:'K4', name:'业务兜底到位', weight:10, rule:'纸质流程在业务中断30分钟内启用并登记' },
  { id:'K5', name:'跨部门协同', weight:10, rule:'会商响应及时、指令闭环反馈' },
  { id:'K6', name:'上报主管部门及时性', weight:10, rule:'研判后2小时内完成卫健上报' },
  { id:'K7', name:'取证留痕完整性', weight:10, rule:'关键节点均有日志/截图/表单归档' },
  { id:'K8', name:'舆情管控', weight:8, rule:'对外口径经审批，无擅自发布' },
  { id:'K9', name:'恢复验证', weight:7, rule:'恢复后提交业务恢复确认单并完成验证' }
];

var SCORES = [
  { itemId:'K1', score:11, max:15, note:'值班员T101超时3分钟上报，扣2分；后续响应及时' },
  { itemId:'K2', score:14, max:15, note:'上报表要素齐全，联系人字段漏填扣1分' },
  { itemId:'K3', score:15, max:15, note:'处置顺序规范，隔离与封堵并行得当' },
  { itemId:'K4', score:9, max:10, note:'纸质兜底25分钟内启用，登记完整' },
  { itemId:'K5', score:8, max:10, note:'联合会商响应总体及时，网安大队问询回复略慢' },
  { itemId:'K6', score:10, max:10, note:'研判后57分钟完成上报，时效优秀' },
  { itemId:'K7', score:8, max:10, note:'样本摘要缺少哈希值字段扣2分' },
  { itemId:'K8', score:7, max:8, note:'公告口径走审批，扣1分：张贴版与审批版存在措辞差异' },
  { itemId:'K9', score:7, max:7, note:'恢复验证完整' }
];

var LEDGER = [
  { id:'P2026-012', name:'2026年勒索病毒专项应急演练', date:'2026-10-23', type:'实战协同演练', event:'勒索病毒', level:'重大', result:'进行中', score:'-', org:'院内自主' },
  { id:'P2026-011', name:'数据泄露舆情联合演练', date:'2026-09-18', type:'联合演练', event:'数据泄露', level:'重大', result:'良好', score:'89.0', org:'卫健牵头' },
  { id:'P2026-009', name:'钓鱼邮件防范季度演练', date:'2026-07-10', type:'桌面推演', event:'钓鱼邮件', level:'一般', result:'合格', score:'76.5', org:'院内自主' },
  { id:'P2026-008', name:'HIS宕机灾备切换演练', date:'2026-06-15', type:'实战协同演练', event:'系统宕机', level:'重大', result:'良好', score:'85.0', org:'院内自主' },
  { id:'P2026-007', name:'内网渗透排查专项演练', date:'2026-04-22', type:'桌面推演', event:'内网渗透', level:'较大', result:'优秀', score:'92.0', org:'院内自主' },
  { id:'P2025-015', name:'DDoS攻击兜底演练', date:'2025-12-08', type:'实战协同演练', event:'DDoS攻击', level:'较大', result:'合格', score:'78.0', org:'院内自主' }
];

var OPINION = [
  { id:'O1', time:'10:50', channel:'模拟微博监测', title:'【演练模拟】网传“盘州医院患者信息被倒卖”', heat:326, risk:'较大', status:'待研判', source:'网信办推送' },
  { id:'O2', time:'11:05', channel:'模拟论坛', title:'【演练模拟】某论坛发帖出售“医院挂号数据”样本', heat:158, risk:'重大', status:'研判中', source:'网信办推送' },
  { id:'O3', time:'11:20', channel:'医院公众号', title:'院方公告文稿：关于系统维护的情况说明（待审批）', heat:'-', risk:'—', status:'审批中', source:'院办提交' },
  { id:'O4', time:'11:45', channel:'模拟微博监测', title:'【演练模拟】话题热度回落，未发现本地媒体跟进', heat:42, risk:'一般', status:'已归档', source:'网信办推送' }
];

var KNOWLEDGE = [
  { id:'KB1', name:'盘州市人民医院网络安全事件应急预案（2026版）', cat:'综合预案', updated:'2026-03-01', size:'1.2 MB', pages:48 },
  { id:'KB2', name:'勒索病毒专项处置预案', cat:'专项预案', updated:'2026-04-12', size:'680 KB', pages:22 },
  { id:'KB3', name:'网络安全事件上报流程图（卫健/网信/网安）', cat:'上报流程', updated:'2026-05-20', size:'320 KB', pages:3 },
  { id:'KB4', name:'业务中断纸质兜底操作流程（处方/登记/收费）', cat:'兜底流程', updated:'2026-06-02', size:'540 KB', pages:15 },
  { id:'KB5', name:'应急通讯录（院内+主管单位，虚拟号码）', cat:'通讯录', updated:'2026-09-01', size:'96 KB', pages:4 },
  { id:'KB6', name:'数据泄露事件报案取证指引', cat:'专项预案', updated:'2026-07-18', size:'410 KB', pages:12 }
];

var AUDIT_LOGS = [
  { time:'2026-10-23 08:55', user:'平台管理员', role:'platformadmin', action:'登录系统', ip:'10.30.1.8' },
  { time:'2026-10-23 08:58', user:'平台管理员', role:'platformadmin', action:'启用剧本《勒索病毒攻击演练》v2.1', ip:'10.30.1.8' },
  { time:'2026-10-23 09:00', user:'张副院长', role:'commander', action:'启动演练项目 P2026-012', ip:'10.30.2.15' },
  { time:'2026-10-23 09:31', user:'王强', role:'netsec', action:'执行模拟处置：IP封堵 185.2xx.xx.77', ip:'10.30.3.22' },
  { time:'2026-10-23 10:20', user:'张副院长', role:'commander', action:'审批应急启动（Ⅱ级响应）', ip:'10.30.2.15' },
  { time:'2026-10-23 10:58', user:'陈主任', role:'itlead', action:'提交《网络安全事件上报表》', ip:'10.30.3.11' },
  { time:'2026-10-23 11:02', user:'张副院长', role:'commander', action:'批准上报市卫健局', ip:'10.30.2.15' },
  { time:'2026-10-23 11:10', user:'周科员', role:'healthbureau', action:'查看医院报送材料', ip:'172.18.5.31' }
];

var NOTIFS = [
  { id:'N1', time:'09:02', title:'P0告警：终端可疑加密进程', desc:'态势感知平台（模拟）推送，请值班员立即核实', level:'P0', read:false },
  { id:'N2', time:'09:31', title:'处置记录已提交', desc:'网络安全管理员王强提交IP封堵记录', level:'P1', read:false },
  { id:'N3', time:'10:20', title:'指挥部指令', desc:'启动Ⅱ级应急响应，各科室按预案执行', level:'P0', read:false },
  { id:'N4', time:'11:02', title:'上报材料待接收', desc:'医院《网络安全事件上报表》已流转至市卫健局', level:'P1', read:false },
  { id:'N5', time:'11:30', title:'超时提醒', desc:'任务T101超时已提醒信息科负责人', level:'P2', read:false }
];

var IMPACT = [
  { sys:'门诊收费系统', state:'已恢复', pct:100 },
  { sys:'HIS住院系统', state:'已恢复', pct:100 },
  { sys:'电子病历EMR', state:'恢复验证中', pct:85 },
  { sys:'LIS检验系统', state:'正常', pct:100 },
  { sys:'PACS影像', state:'灾备切换', pct:60 },
  { sys:'办公OA', state:'隔离中', pct:20 }
];

var DICT = {
  levels:['一般','较大','重大','特别重大'],
  projTypes:['桌面推演','实战协同演练','联合演练'],
  eventTypes:['勒索病毒','内网渗透','DDoS攻击','钓鱼邮件','系统宕机','数据泄露','边界突破','恶意代码'],
  difficulties:['基础版（引导式）','实战版（无引导）']
};

/* 剧本节点（S01 勒索病毒，供编排器与看板时间轴使用） */
var SCRIPT_NODES = [
  { id:'N1', type:'alert', title:'T+0 模拟告警推送', target:'值班员', time:'09:00', timeout:10, desc:'推送终端加密行为告警（实战版仅原始告警）' },
  { id:'N2', type:'task', title:'异常核实与登记', target:'值班员', time:'09:05', timeout:10, form:'F1', att:'告警截图', desc:'填写事件初步核实单' },
  { id:'N3', type:'task', title:'初步上报', target:'值班员→信息科负责人', time:'09:15', timeout:5, desc:'上报核实结果' },
  { id:'N4', type:'branch', title:'事件等级判定', target:'信息科负责人', time:'09:25', desc:'分支：一般→科内处置；较大/重大→上报指挥部', branches:['一般事件','较大及以上'] },
  { id:'N5', type:'task', title:'网络侧处置', target:'网络安全管理员', time:'09:30', timeout:15, form:'F2', desc:'模拟IP封堵、终端隔离、流量排查' },
  { id:'N6', type:'task', title:'核心系统处置', target:'系统管理员', time:'09:45', timeout:20, desc:'数据库隔离、备份校验' },
  { id:'N7', type:'task', title:'应急启动审批', target:'院应急指挥部', time:'10:15', timeout:10, form:'F4', desc:'研判等级、下达启动指令' },
  { id:'N8', type:'task', title:'业务兜底启用', target:'医务部/临床窗口', time:'10:30', timeout:20, form:'F3', desc:'纸质流程启用与登记' },
  { id:'N9', type:'report', title:'上报主管部门', target:'指挥部→卫健局', time:'11:00', timeout:15, form:'F2', desc:'审批后流转上报表' },
  { id:'N10', type:'report', title:'报案取证', target:'指挥部→网安大队', time:'11:20', form:'F5', desc:'报案登记与证据固定' },
  { id:'N11', type:'task', title:'系统恢复与验证', target:'系统管理员', time:'11:40', timeout:30, form:'F7', desc:'恢复上线、业务验证' },
  { id:'N12', type:'end', title:'演练终止与复盘', target:'指挥部', time:'12:05', desc:'终止指令、进入评估' }
];

/* 演练事件模拟数据字典：按场景 type 键，供任务表单自动预填与自动取证截图使用（全部为虚拟演练数据，不含真实患者信息） */
var SIM_DATA = {
  '勒索病毒': {
    sys:'HIS服务器 10.20.15.66 / EMR 10.20.15.70 / 办公终端 10.20.13.45',
    fault:'10.20.13.45 等3台终端检出可疑加密进程，文件批量重命名为 .locked 后缀，桌面出现勒索信 README_RANSOM.txt',
    alert:'态势感知平台（模拟）：内网3台终端检出可疑加密进程，并向境外IP异常外联（疑似C2通信）',
    ip:'疑似C2外联地址 185.2xx.xx.77（模拟境外恶意IP）',
    sample:'勒索信 README_RANSOM.txt；加密样本哈希（模拟）SHA256: 3f9a…c21e',
    scope:'办公网3台终端文件被加密，HIS/EMR业务网暂未波及，数据库服务器未检出感染',
    action:'感染终端断网隔离、模拟封堵C2地址 185.2xx.xx.77、全网杀毒特征库升级排查',
    progress:'感染已控制在办公网3台终端；HIS数据库备份校验通过，恢复验证进行中',
    backup:'今晨06:00存储快照可用于恢复，备份有效性校验通过',
    impact:'门诊收费与电子病历读写短时受限，已启用纸质兜底流程',
    measure:'纸质处方/人工收费/登记本兜底，系统恢复后在线补录'
  },
  '内网渗透': {
    sys:'HIS 10.20.15.66 / EMR 10.20.15.70 / 失陷终端 10.20.16.28',
    fault:'攻击者利用弱口令账号横向移动至10.20.16.x段，EMR出现异常账号越权查看虚拟患者记录',
    alert:'态势感知平台（模拟）：检出多次登录失败与异常SMB横向移动（源 10.20.16.28）',
    ip:'异常横向移动源 10.20.16.28（模拟失陷终端）',
    sample:'异常登录脚本 psexec_tmp.exe（模拟样本哈希 8ab4…1d0f）',
    scope:'EMR系统被非法账号短时访问，未检出数据外传，业务网未进一步扩散',
    action:'禁用失陷账号、隔离终端10.20.16.28、全院账号口令重置与登录审计',
    progress:'失陷账号已禁用，横向移动路径已确认，取证日志提取中',
    backup:'数据库审计日志已导出归档，备份快照未受影响',
    impact:'EMR短时限制访问，门诊住院业务正常',
    measure:'病历权限人工核对，必要时转纸质登记'
  },
  'DDoS攻击': {
    sys:'门诊收费对外门户 172.18.9.20 / 边界防火墙 10.20.10.1',
    fault:'门诊收费系统遭模拟大流量攻击，页面响应超30秒，部分收费操作超时失败',
    alert:'边界防火墙（模拟）：入站流量突增至基线8倍，源集中于 45.155.x.x 段',
    ip:'攻击源 45.155.x.x 段（模拟DDoS源IP）',
    sample:'流量抓取摘要 ddos_0920.pcap（模拟）',
    scope:'门诊收费窗口与自助机受影响，住院/HIS内网业务正常',
    action:'协调运营商模拟流量清洗、边界限速策略、启用窗口纸质收费兜底',
    progress:'清洗策略生效后流量回落至正常，收费业务已恢复',
    backup:'收费流水临时本地缓存，恢复后核对无丢单',
    impact:'门诊收费排队时长增加，自助机暂停服务',
    measure:'纸质收据+人工收费，系统恢复后在线补录'
  },
  '钓鱼邮件': {
    sys:'OA邮件服务器 10.20.18.10 / 受害终端 172.18.22.31',
    fault:'员工点击"发票报销"钓鱼邮件下载木马，终端出现异常远程连接行为',
    alert:'邮件网关（模拟）：检出3封同附件哈希钓鱼邮件，其中1封已被点击',
    ip:'钓鱼链接域名 pay-sim.top（模拟）/ C2 45.9.x.11',
    sample:'附件《10月报销明细.xlsx.exe》（模拟木马dropper，哈希 5c7e…9a2b）',
    scope:'办公网1台终端失陷，业务网未受影响',
    action:'网关清除钓鱼邮件、隔离终端172.18.22.31、账号重置并全员通报',
    progress:'邮件清理与终端隔离完成，账号重置进行中',
    backup:'邮件日志已导出归档取证',
    impact:'OA短时暂停外收邮件，临床业务不受影响',
    measure:'OA暂停期间使用院内通知单与纸质传阅'
  },
  '系统宕机': {
    sys:'HIS 10.20.15.66 / EMR 10.20.15.70 / 核心存储 10.20.14.10',
    fault:'核心存储模拟磁盘阵列故障，数据库无法挂载，HIS/EMR整体宕机',
    alert:'运维监控平台（模拟）：存储 10.20.14.10 多块磁盘离线，数据库连接池耗尽',
    ip:'存储管理地址 10.20.14.10（模拟故障设备）',
    sample:'存储硬件错误日志 storage_err.log（模拟）',
    scope:'HIS、EMR、收费业务全面中断，LIS/PACS经备用链路运行',
    action:'切换灾备节点、通知厂商应急到场、启动纸质兜底流程',
    progress:'灾备切换完成，业务恢复验证进行中',
    backup:'以06:00快照+归档日志恢复，数据完整性校验通过',
    impact:'门诊与住院业务中断约40分钟（模拟），纸质流程接管',
    measure:'纸质处方/登记/人工收费，恢复后在线补录'
  },
  '数据泄露': {
    sys:'EMR数据库 10.20.15.70 / 外发审计系统 172.18.7.5',
    fault:'外部论坛（模拟）出现售卖"医院挂号数据"样本，涉及虚拟患者身份与就诊记录',
    alert:'舆情监测（模拟）+ 卫健通报：外部平台出现涉院数据售卖样本',
    ip:'疑似外传目的地 103.x.x.x（模拟外网上传地址）',
    sample:'泄露数据样本片段 sample_001.csv（虚拟数据，无真实患者）',
    scope:'初判为历史导出数据外泄，现网数据库未受损，涉及范围核实中',
    action:'上报卫健/网信/网安、封禁外传账号、舆情监测与对外口径审批',
    progress:'上报材料已流转，取证摘要固定，舆情热度回落',
    backup:'数据库审计日志与导出记录已导出取证',
    impact:'业务未中断，以舆情与合规风险为主',
    measure:'对外口径统一经指挥部审批发布，患者问询登记解释'
  },
  '边界突破': {
    sys:'边界防火墙 10.20.10.1 / DMZ Web服务器 172.18.8.15',
    fault:'边界设备模拟漏洞被利用，DMZ服务器出现异常登录与外联回连，多业务系统并发告警',
    alert:'边界防火墙（模拟）：检出针对DMZ 172.18.8.15 的非授权访问与异常外联',
    ip:'攻击源 193.x.x.x（模拟境外IP）',
    sample:'Webshell摘要 shell.jsp（模拟，已清除归档）',
    scope:'DMZ互联网业务受影响，内网业务网经访问控制核查未扩散',
    action:'暂停DMZ对外服务、边界策略收敛、隔离失陷服务器、全网排查',
    progress:'边界策略已收敛，失陷服务器隔离，排查持续进行',
    backup:'防火墙配置已备份回滚，攻击日志归档取证',
    impact:'预约挂号/在线咨询等互联网服务短时停用，院内业务正常',
    measure:'预约转电话/窗口办理，恢复后验证放行'
  },
  '恶意代码': {
    sys:'办公网 172.18.20.x 段 / 终端管理系统 10.20.18.20',
    fault:'办公网终端批量检出恶意代码告警，出现蠕虫式横向传播尝试行为',
    alert:'终端管理平台（模拟）：12台办公终端检出同一恶意代码家族（模拟哈希 77e0…4b1a）',
    ip:'内网扩散源 172.18.20.66（模拟零号病人终端）',
    sample:'恶意代码样本摘要（哈希+行为报告，模拟）',
    scope:'办公网12台终端感染，业务网未波及',
    action:'批量断网隔离、查杀特征库全网升级、共享目录写权限关闭',
    progress:'零号终端已定位，批量查杀进行中',
    backup:'终端资产清单已导出，查杀日志归档',
    impact:'OA与办公文件共享短时受限，临床业务正常',
    measure:'文件共享受限期间纸质流转'
  },
  '通用': {
    sys:'院内业务系统（模拟地址 10.20.x.x）',
    fault:'模拟故障现象（通用兜底，请按实际修改）',
    alert:'态势感知平台（模拟）：推送安全告警',
    ip:'10.20.x.x（模拟可疑IP）',
    sample:'模拟样本摘要（占位）',
    scope:'模拟范围（无真实业务影响）',
    action:'按预案模拟处置',
    progress:'按剧本推进中（模拟）',
    backup:'备份有效性校验（模拟）',
    impact:'无真实业务影响（模拟）',
    measure:'按预案启用纸质/人工兜底（模拟）'
  }
};

return { ROLES:ROLES, USERS:USERS, SCENARIOS:SCENARIOS, PROJECTS:PROJECTS, TASKS:TASKS, MESSAGES:MESSAGES,
  CHAT:CHAT, FORM_TEMPLATES:FORM_TEMPLATES, EVIDENCE:EVIDENCE, SNAPSHOTS:SNAPSHOTS, TIMELINE:TIMELINE,
  SEQ_FLOW:SEQ_FLOW, SCORE_ITEMS:SCORE_ITEMS, SCORES:SCORES, LEDGER:LEDGER, OPINION:OPINION, KNOWLEDGE:KNOWLEDGE,
  AUDIT_LOGS:AUDIT_LOGS, NOTIFS:NOTIFS, IMPACT:IMPACT, DICT:DICT, SCRIPT_NODES:SCRIPT_NODES, SIM_DATA:SIM_DATA };
})();
