/* 演练报告导出 · 复盘与报送栏目
 * 数据来源：GET /api/admin/report/:projectId（服务端聚合项目、剧本、任务、时序、评分、取证）
 * 导出格式：Word(.doc) / HTML / 打印另存 PDF，导出动作写入审计日志
 */
window.ReportPage = (function () {
  'use strict';
  function mount(page, role) {
    page.innerHTML =
      '<div class="page-head"><div><h1>演练报告导出</h1><div class="sub">按演练项目生成标准网络安全应急演练报告 · 支持 Word / HTML / PDF 导出</div></div>'
      + '<button class="btn" id="rpBack">'+App.icon('replay')+'时序回放</button></div>'
      + '<div class="sim-banner">'+App.icon('alert')+'<span>报告数据取自演练数据库（项目、剧本、任务、时序、评分、取证），内容为演练模拟数据，不含真实患者隐私信息。</span></div>'
      + '<div class="card"><div class="card-title">'+App.icon('file')+'导出标准网络安全应急演练报告</div>'
      +   '<div class="filters" style="margin-bottom:10px">'
      +     '<span class="small muted">演练项目：</span><select class="select" id="rpProj" style="min-width:280px"></select>'
      +     '<button class="btn primary" id="rpPreview">'+App.icon('eye')+'预览报告</button>'
      +     '<button class="btn" id="rpWord">'+App.icon('download')+'导出 Word</button>'
      +     '<button class="btn" id="rpHtml">'+App.icon('download')+'导出 HTML</button>'
      +     '<button class="btn" id="rpPrint">'+App.icon('file')+'打印 / 另存 PDF</button>'
      +   '</div>'
      +   '<div class="filters" style="gap:18px">'
      +     '<label class="small"><input type="checkbox" id="rpOptTl" checked> 处置时序表</label>'
      +     '<label class="small"><input type="checkbox" id="rpOptTask" checked> 任务明细表</label>'
      +     '<label class="small"><input type="checkbox" id="rpOptEv" checked> 取证材料清单</label>'
      +     '<label class="small"><input type="checkbox" id="rpOptAudit"> 审计留痕摘要</label>'
      +   '</div>'
      +   '<div class="tiny dim mt3" id="rpMeta">报告数据取自演练数据库（项目、剧本、任务、时序、评分、取证），导出操作将写入审计日志。</div></div>'
      + '<div class="card" style="margin-top:14px"><div class="card-title">'+App.icon('ledger')+'报告内容说明</div>'
      +   '<div class="tiny" style="line-height:2">报告共十一章：一、演练概况；二、演练组织与职责分工；三、演练准备；四、演练实施过程；五、演练任务完成情况；六、演练评估；七、存在问题；八、改进建议；九、取证与材料归档；十、演练结论；十一、附件。含封面（密级/编号）、编制与审核落款。</div></div>';

    var rpBack = document.getElementById('rpBack');
    if (role && role.nav && role.nav.indexOf('replay') < 0) rpBack.style.display = 'none';
    rpBack.onclick = function () {
      var sel = document.getElementById('rpProj');
      location.href = 'replay.html' + (sel && sel.value ? ('?projectId=' + encodeURIComponent(sel.value)) : '');
    };

  /* ---------- f) 演练报告导出 ---------- */
  var rpCache = {}, rpBusy = false;
  var rpProjSel = document.getElementById('rpProj');
  rpProjSel.innerHTML = (MOCK.PROJECTS || []).map(function(p){
    return '<option value="'+p.id+'">'+App.esc(p.name)+'（'+p.id+'）</option>';
  }).join('');
  var curProj = API.activeProject();
  if(curProj) rpProjSel.value = curProj.id;

  function rpOpts(){
    return {
      tl: document.getElementById('rpOptTl').checked,
      task: document.getElementById('rpOptTask').checked,
      ev: document.getElementById('rpOptEv').checked,
      audit: document.getElementById('rpOptAudit').checked
    };
  }
  function rpMeta(t){ var el=document.getElementById('rpMeta'); if(el) el.innerHTML = t; }
  function p2(n){ return n<10 ? '0'+n : String(n); }
  function ymd(d){ d = d || new Date(); return d.getFullYear()+p2(d.getMonth()+1)+p2(d.getDate()); }
  function gradeOf(pct){
    if(pct===null) return '未考评';
    if(pct>=90) return '优秀'; if(pct>=80) return '良好'; if(pct>=70) return '合格'; return '不合格';
  }
  function loadReport(){
    return new Promise(function(resolve, reject){
      var pid = rpProjSel.value;
      if(rpCache[pid]){ resolve(rpCache[pid]); return; }
      rpMeta('正在读取演练数据（项目 / 剧本 / 任务 / 时序 / 评分 / 取证）…');
      API.getDrillReport(pid).then(function(d){
        rpCache[pid] = d;
        rpMeta('已读取「'+App.esc(d.project.name)+'」：任务 '+d.stats.total+' 项 · 时序 '+d.timeline.length+' 条 · 取证 '+d.evidenceStats.total+' 件 · 评分 '+
          (d.modeScore ? (d.modeScore.score+'/'+d.modeScore.max+'（'+App.esc(d.project.mode||'基础版')+'自动考评）')
            : (d.scoreStats.pct===null ? '未考评' : d.scoreStats.sum+'/'+d.scoreStats.max+'（'+d.scoreStats.pct+'% · '+gradeOf(d.scoreStats.pct)+'）'))+'。');
        resolve(d);
      }).catch(reject);
    });
  }

  /* 根据数据推导问题与建议 */
  function rpIssues(d){
    var out = [];
    d.tasks.filter(function(t){ return t.status!=='已完成'; }).forEach(function(t){
      out.push('任务 '+t.id+'「'+t.title+'」（'+t.roleName+'，处置时长 '+(t.duration?t.duration+' 分钟':'未设置')+'）状态为「'+t.status+'」，未形成闭环。');
    });
    d.scores.forEach(function(s){
      if(s.score!==null && Number(s.score) < Number(s.max)) out.push('考评项「'+s.name+'」扣 '+(s.max-s.score)+' 分：'+(s.note||'未填写具体依据')+'。');
    });
    if(d.evidenceStats.total===0) out.push('本次演练未归档任何取证材料，证据链不完整。');
    else if(d.evidenceStats.auto>0 && d.evidenceStats.total===d.evidenceStats.auto) out.push('现有取证材料全部由系统自动生成，缺少人工现场取证记录。');
    if(d.scoreStats.pct===null && !d.modeScore) out.push('尚未执行系统自动考评，缺少量化评分依据。');
    return out;
  }
  function rpSugs(d){
    var out = [];
    if(d.stats.overdue>0) out.push('针对超时任务建立分级时限提醒与督办机制，超时自动推送至院应急指挥部并纳入下次演练重点考核。');
    if(d.stats.pending>0 || d.stats.doing>0) out.push('对未闭环任务补充处置记录与验证结论，确保"发现—处置—恢复—复盘"全流程留痕。');
    if(d.scores.some(function(s){ return s.score!==null && Number(s.score) < Number(s.max)*0.8; })) out.push('按考评扣分项逐条落实整改责任人与完成时限，下轮演练中复核整改效果。');
    if(d.evidenceStats.auto>0) out.push('加强人工取证（告警截图、日志片段、配置备份），与自动取证材料相互印证，满足事件溯源的证据要求。');
    if(d.scoreStats.pct===null && !d.modeScore) out.push('演练结束后及时在「复盘评估」中执行系统自动考评，形成量化评估结论。');
    out.push('定期更新应急预案与剧本节点，每半年至少开展一次同场景实战化演练，保持处置流程熟练度。');
    return out;
  }

  /* 生成标准报告（自包含 HTML，可直接打印/转 Word） */
  function buildReport(d, o){
    var p = d.project, sc = d.scenario, st = d.stats, ss = d.scoreStats;
    var today = new Date();
    var dateStr = today.getFullYear()+'年'+(today.getMonth()+1)+'月'+today.getDate()+'日';
    var no = 'DR-'+p.id+'-RPT-'+ymd(today);
    var grade = gradeOf(ss.pct);
    var memberRows = (p.members||[]).map(function(m){
      var r = (MOCK.ROLES||[]).filter(function(x){ return x.id===m.role; })[0] || {};
      return '<tr><td>'+App.esc(r.name||m.role)+'</td><td>'+App.esc(m.user||'—')+'</td><td>'+App.esc(r.desc||'按角色职责参演')+'</td></tr>';
    }).join('');
    var byRoleRows = d.byRole.map(function(r){
      return '<tr><td>'+App.esc(r.role)+'</td><td>'+r.total+'</td><td>'+r.done+'</td><td>'+r.doing+'</td><td>'+r.pending+'</td><td>'+r.overdue+'</td></tr>';
    }).join('');
    var taskRows = d.tasks.map(function(t){
      return '<tr><td>'+App.esc(t.id)+'</td><td>'+App.esc(t.title)+'</td><td>'+App.esc(t.roleName)+'</td><td>'+App.esc(t.level)+'</td><td>'+(t.duration?t.duration+' 分钟':'—')+'</td><td>'+App.esc(t.status)+'</td></tr>';
    }).join('');
    var tlRows = d.timeline.map(function(x){
      return '<tr><td>'+App.esc(x.t)+'</td><td>'+App.esc(x.action)+'</td><td>'+App.esc(x.actor||x.role||'—')+'</td><td>'+App.esc(x.detail||'')+'</td></tr>';
    }).join('');
    var seqRows = d.seqFlow.map(function(x){
      return '<tr><td>'+App.esc(x.time)+'</td><td>'+App.esc(x.name)+'</td><td>'+App.esc(x.role)+'</td><td>'+App.esc(x.state||'完成')+'</td></tr>';
    }).join('');
    var pmode = p.mode || '基础版';
    var decisionRows = (d.decisions||[]).map(function(x){
      return '<tr><td>'+App.esc(x.title)+'</td><td>'+App.esc(x.target||'—')+'</td><td>'+App.esc(x.choice||'—')+'</td>'
        + '<td>'+(x.correct?'正确':'有误')+'</td><td>'+App.esc(x.stepTime||'—')+'</td><td>'+App.esc(x.user||'—')+'</td>'
        + '<td>'+App.esc(x.found||'—')+'<br><span style="color:#444">措施：'+App.esc(x.measure||'—')+'</span><br><span style="color:#444">结果：'+App.esc(x.result||'—')+'</span></td></tr>';
    }).join('');
    var scoreRows = d.scores.map(function(s){
      return '<tr><td>'+App.esc(s.itemId)+' '+App.esc(s.name)+'</td><td>'+s.max+'</td><td>'+(s.score===null?'未评':s.score)+'</td><td>'+App.esc(s.note||s.rule||'—')+'</td></tr>';
    }).join('');
    var evTypeRows = Object.keys(d.evidenceStats.byType).map(function(k){
      return '<tr><td>'+App.esc(k)+'</td><td>'+d.evidenceStats.byType[k]+'</td></tr>';
    }).join('');
    var evRows = d.evidence.map(function(e){
      return '<tr><td>'+App.esc(e.id)+'</td><td>'+App.esc(e.type)+'</td><td>'+App.esc(e.name)+'</td><td>'+App.esc(e.size||'—')+'</td><td>'+App.esc(e.time||'—')+'</td><td>'+App.esc(e.taskId||'—')+'</td><td>'+(e.auto?'自动生成':'人工上传')+'</td></tr>';
    }).join('');
    var auditRows = (MOCK.AUDIT_LOGS||[]).slice(0,15).map(function(a){
      return '<tr><td>'+App.esc(a.time)+'</td><td>'+App.esc(a.user)+'</td><td>'+App.esc(a.action)+'</td><td>'+App.esc(a.ip||'—')+'</td></tr>';
    }).join('');
    var issues = rpIssues(d), sugs = rpSugs(d);
    var issueHtml = issues.length ? '<ol>'+issues.map(function(x){ return '<li>'+App.esc(x)+'</li>'; }).join('')+'</ol>' : '<p>本次演练各环节均按剧本完成，未发现明显问题。</p>';
    var sugHtml = '<ol>'+sugs.map(function(x){ return '<li>'+App.esc(x)+'</li>'; }).join('')+'</ol>';
    var evalScore = d.modeScore ? d.modeScore.score : ss.pct;
    var evalTxt = d.modeScore
      ? (d.modeScore.score+' / '+d.modeScore.max+' 分（演练模式综合评分，等级'+gradeOf(d.modeScore.score)+'）')
      : (ss.pct===null ? '未开展' : (ss.sum+' / '+ss.max+' 分（得分率 '+ss.pct+'%，等级'+grade+'）'));
    var conclusion = '本次演练以「'+(sc?sc.name:'—')+'」为场景，演练等级'+App.esc(p.level)+'，共设置处置任务 '+st.total+' 项，完成 '+st.done+
      ' 项，完成率 '+st.rate+'%'+(st.overdue?('，超时 '+st.overdue+' 项'):'')+'；系统自动考评为 '+evalTxt+
      '。'+(evalScore===null ? '总体完成情况尚可，建议补充考评后形成量化结论。' : (evalScore>=80 && st.rate>=90 ? '演练组织有序、职责清晰、处置及时，达到演练预期目标。' : '演练基本达到预期目标，但在时限控制、证据完备性与流程闭环方面仍需改进。'));

    var css = 'body{font-family:"宋体",SimSun,serif;font-size:11pt;color:#000;line-height:1.7;margin:0;background:#fff}'
      + '.page{padding:2.2cm 2cm;max-width:19cm;margin:0 auto}'
      + '.cover{height:24cm;position:relative;page-break-after:always;text-align:center;padding:2cm}'
      + '.cover .cls{position:absolute;top:1cm;right:1cm;font-size:10pt;text-align:right}'
      + '.cover .no{position:absolute;bottom:1.2cm;left:2cm;font-size:10pt}'
      + '.cover h1{font-family:"黑体",SimHei;font-size:28pt;letter-spacing:6px;margin:6cm 0 0}'
      + '.cover .sub{font-family:"黑体",SimHei;font-size:15pt;margin-top:14px}'
      + '.cover .line{width:60%;height:2px;background:#000;margin:18px auto}'
      + '.cover .meta{font-size:12pt;line-height:2.2;margin-top:2.2cm}'
      + 'h2{font-family:"黑体",SimHei;font-size:14pt;border-bottom:1.5px solid #000;padding-bottom:4px;margin:20px 0 10px}'
      + 'h3{font-family:"黑体",SimHei;font-size:12pt;margin:14px 0 6px}'
      + 'table{width:100%;border-collapse:collapse;font-size:10pt;margin:6px 0 10px}'
      + 'th,td{border:1px solid #444;padding:5px 6px;vertical-align:top;word-break:break-all}'
      + 'th{background:#eee;font-family:"黑体",SimHei;font-weight:normal}'
      + 'td.k{background:#f5f5f5;text-align:center;width:16%;font-family:"黑体",SimHei}'
      + 'ol,ul{padding-left:22px;margin:6px 0} li{margin-bottom:4px}'
      + '.sum{text-align:right;font-weight:700;margin-top:6px}'
      + '.sign{margin-top:2.2cm;font-size:11pt;line-height:2.4}'
      + '.foot{margin-top:26px;font-size:9pt;color:#555;border-top:1px solid #999;padding-top:6px}'
      + '.tag{display:inline-block;border:1px solid #000;padding:1px 8px;font-size:9pt}'
      + '@page{size:A4;margin:2cm}'
      + '@media print{.page{padding:0;max-width:none}}';

    var B = (window.DRILL && DRILL.brand()) || {};
    var brandUnit = B.unitName || '盘州市人民医院';
    var brandSystem = B.systemName || '网络安全应急演练系统';

    var cover = '<div class="cover"><div class="cls">密级：内部（演练专用）<br>编号：'+App.esc(no)+'</div>'
      + '<h1>网络安全应急演练报告</h1><div class="line"></div><div class="sub">'+App.esc(p.name)+'</div>'
      + '<div class="meta">演练类型：'+App.esc(p.type)+'<br>事件等级：'+App.esc(p.level)+'<br>演练日期：'+App.esc(p.date)+'<br>项目编号：'+App.esc(p.id)+'</div>'
      + '<div class="meta" style="margin-top:2.6cm">编制单位：'+App.esc(brandUnit)+'信息科<br>编制日期：'+dateStr+'<br>编制人：'+App.esc(d.generatedBy||'平台管理员')+'</div>'
      + '<div class="no">'+App.esc(brandUnit)+'　'+App.esc(brandSystem)+'</div></div>';

    var s1 = '<h2>一、演练概况</h2><table>'
      + '<tr><td class="k">项目名称</td><td colspan="3">'+App.esc(p.name)+'</td></tr>'
      + '<tr><td class="k">项目编号</td><td>'+App.esc(p.id)+'</td><td class="k">演练类型</td><td>'+App.esc(p.type)+'</td></tr>'
      + '<tr><td class="k">事件等级</td><td>'+App.esc(p.level)+'</td><td class="k">演练状态</td><td>'+App.esc(p.status)+'</td></tr>'
      + '<tr><td class="k">演练日期</td><td>'+App.esc(p.date)+'</td><td class="k">演练地点</td><td>院内演练环境（独立演练服务器，与生产网物理隔离）</td></tr>'
      + '<tr><td class="k">参演单位</td><td colspan="3">'+App.esc((p.units||[]).join('、')||'盘州市人民医院')+'</td></tr>'
      + '<tr><td class="k">演练目标</td><td colspan="3">'+App.esc(p.target||'—')+'</td></tr>'
      + '<tr><td class="k">场景剧本</td><td colspan="3">'+App.esc(sc?sc.name+'（'+sc.version+'）':'—')+(sc?('，事件类型'+App.esc(sc.type)+'，剧本节点 '+sc.nodes+' 个'):'')+'</td></tr>'
      + '</table>';

    var s2 = '<h2>二、演练组织与职责分工</h2><table><tr><th style="width:26%">参演角色</th><th style="width:20%">参演人员</th><th>主要职责</th></tr>'
      + (memberRows || '<tr><td colspan="3">未配置参演人员</td></tr>')
      + '<tr><td class="k">观察员</td><td colspan="2">'+App.esc((p.observers||[]).join('、')||'—')+'</td></tr></table>'
      + '<p>本次演练采用"院应急指挥部统一指挥、信息科牵头处置、医务与临床协同、外部单位按需报送"的组织模式，各角色按剧本节点接收任务并在平台内留痕。</p>';

    var s3 = '<h2>三、演练准备</h2><p>1. 场景与剧本：'+(sc?App.esc(sc.name)+'，'+App.esc(sc.descr||'')+'；剧本共 '+sc.nodes+' 个节点，明确各节点责任角色、处置时长与需提交表单。':'未关联场景剧本。')+'</p>'
      + '<p>2. 演练环境：系统部署于独立演练服务器/虚拟机，与医院 HIS、业务网及互联网物理隔离；全部处置动作在平台内模拟记录，不下发真实网络配置命令，不操作生产数据。</p>'
      + '<p>3. 数据准备：演练数据全部为虚拟测试数据，未录入任何真实患者隐私信息；告警、系统 IP、故障现象等由平台按场景自动填充，参演人员核对后确认。</p>'
      + '<p>4. 保障措施：平台账号按角色分配，登录采用口令+验证码；操作全过程写入审计日志（仅追加、不可删除）。</p>';

    var s4 = '<h2>四、演练实施过程</h2>';
    if(o.tl){
      s4 += '<h3>4.1 处置流程时序</h3><table><tr><th style="width:12%">时刻</th><th style="width:30%">处置环节</th><th style="width:22%">责任角色</th><th>状态</th></tr>'
        + (seqRows || '<tr><td colspan="4">暂无流程节点数据</td></tr>') + '</table>'
        + '<h3>4.2 关键操作记录</h3><table><tr><th style="width:12%">时间</th><th style="width:26%">动作</th><th style="width:20%">操作人/角色</th><th>说明</th></tr>'
        + (tlRows || '<tr><td colspan="4">暂无操作记录</td></tr>') + '</table>';
    }
    s4 += pmode === '实战版'
      ? '<p>演练按照"系统异常告警 → 参演人员自主研判 → 选择处置方案 → 分角色处置 → 业务兜底 → 上报取证 → 恢复验证 → 复盘评估"的闭环推进。系统不发布处置任务与工作提示，仅弹出系统异常故障警告，由参演人员在角色工作台自行选择处置方案并填写发现情况、处置措施与结果，全过程在平台内留痕。</p>'
      : '<p>演练按照"告警发现 → 核实研判 → 启动预案 → 分角色处置 → 业务兜底 → 上级上报 → 报案取证 → 恢复验证 → 复盘评估"的闭环流程推进，各节点任务在平台内完成接单、处置与归档。</p>';

    var s5 = '<h2>五、演练任务完成情况</h2>';
    if(pmode === '实战版'){
      var ds = d.decisionStats || { baseline:0, total:0, correct:0, wrong:0, ontime:0 };
      s5 += '<p>本项目采用实战版演练，系统不在角色工作台发布处置步骤与工作提示，仅展示已发现的系统故障并由参演人员自主研判处置。以下统计为依据角色工作台处置记录与剧本基线环节比对的结果。</p>'
        + '<table><tr><th>基线必经环节</th><th>已处置</th><th>决策正确</th><th>决策有误</th><th>时限内</th><th>处置覆盖率</th></tr>'
        + '<tr><td>'+ds.baseline+'</td><td>'+ds.total+'</td><td>'+ds.correct+'</td><td>'+ds.wrong+'</td><td>'+ds.ontime+'</td><td>'+(ds.baseline?Math.round(ds.total/ds.baseline*100):0)+'%</td></tr></table>'
        + '<h3>5.1 处置决策记录</h3><table><tr><th>处置环节</th><th style="width:12%">责任角色</th><th>选择方案</th><th style="width:7%">判定</th><th style="width:8%">处置时刻</th><th style="width:12%">处置人</th><th>发现情况 / 措施 / 结果</th></tr>'
        + (decisionRows || '<tr><td colspan="7">暂无实战处置记录</td></tr>') + '</table>';
    } else {
      s5 += '<table><tr><th>任务总数</th><th>已完成</th><th>处理中</th><th>待处理</th><th>超时</th><th>完成率</th></tr>'
        + '<tr><td>'+st.total+'</td><td>'+st.done+'</td><td>'+st.doing+'</td><td>'+st.pending+'</td><td>'+st.overdue+'</td><td>'+st.rate+'%</td></tr></table>'
        + '<h3>5.1 分角色完成情况</h3><table><tr><th>接收角色</th><th>任务数</th><th>已完成</th><th>处理中</th><th>待处理</th><th>超时</th></tr>'
        + (byRoleRows || '<tr><td colspan="6">暂无任务</td></tr>') + '</table>';
      if(o.task){
        s5 += '<h3>5.2 任务明细</h3><table><tr><th style="width:10%">编号</th><th>任务标题</th><th style="width:22%">接收角色</th><th style="width:8%">等级</th><th style="width:12%">处置时长</th><th style="width:10%">状态</th></tr>'
          + (taskRows || '<tr><td colspan="6">暂无任务</td></tr>') + '</table>';
      }
    }

    var s6 = '<h2>六、演练评估</h2>';
    var mode = (d.project && d.project.mode) || '基础版';
    s6 += '<p>演练模式：<b>'+App.esc(mode)+'</b>。'+(mode==='实战版'
      ? '本项目采用实战版（自主研判），角色工作台只展示已发现的系统故障，不发布处置步骤与引导提示；参演人员对已发现故障从混淆方案中自主选择处置动作，并实际填写发现时间、发现情况、处置措施与结果。系统以 100 分为基础，按与剧本基线偏离（每项20分）、处置超时（每项10分）、未处置（每项15分）及偏离剧本的额外操作（每项5分，上限20分）扣分。'
      : '本项目采用基础版（诱导演练），系统按剧本节点逐节点下发任务、给出引导提示并自动填充相关表单与截图，考评仅统计各节点的操作时限达成率。')+'</p>';
    if(d.modeScore){
      s6 += '<table><tr><th style="width:32%">模式化自动考评</th><th style="width:10%">上限</th><th style="width:10%">得分</th><th>计分依据</th></tr>'
        + '<tr><td>演练模式综合评分（'+App.esc(mode)+'）</td><td>'+d.modeScore.max+'</td><td>'+d.modeScore.score+'</td><td>'+App.esc(d.modeScore.note||'—')+'</td></tr></table>';
    }
    if(ss.graded===0){ if(!d.modeScore) s6 += '<p>本次演练尚未执行系统自动考评，暂无量化评分结果，建议演练结束后在「复盘评估」中完成考评。</p>'; }
    else {
      s6 += '<table><tr><th style="width:26%">考评指标</th><th style="width:8%">上限</th><th style="width:8%">得分</th><th>自动考评依据</th></tr>'
        + scoreRows + '</table><p class="sum">总分：'+ss.sum+' / '+ss.max+'　得分率：'+ss.pct+'%　评估等级：'+grade+'</p>';
    }

    var s7 = '<h2>七、存在问题</h2>' + issueHtml;
    var s8 = '<h2>八、改进建议</h2>' + sugHtml;
    var s9 = '<h2>九、取证与材料归档</h2><p>本次演练共归档取证材料 '+d.evidenceStats.total+' 件，其中系统自动生成 '+d.evidenceStats.auto+' 件，人工上传 '+(d.evidenceStats.total-d.evidenceStats.auto)+' 件。</p><table><tr><th style="width:40%">材料类型</th><th>数量</th></tr>'
      + (evTypeRows || '<tr><td colspan="2">暂无取证材料</td></tr>') + '</table>';
    if(o.ev){
      s9 += '<h3>9.1 取证材料清单</h3><table><tr><th style="width:10%">编号</th><th style="width:12%">类型</th><th>材料名称</th><th style="width:8%">大小</th><th style="width:10%">时间</th><th style="width:9%">关联任务</th><th style="width:10%">来源</th></tr>'
        + (evRows || '<tr><td colspan="7">暂无取证材料</td></tr>') + '</table>';
    }
    if(o.audit){
      s9 += '<h3>9.2 平台审计留痕摘要（最近 15 条）</h3><table><tr><th style="width:18%">时间</th><th style="width:12%">操作人</th><th>操作内容</th><th style="width:14%">来源 IP</th></tr>'
        + (auditRows || '<tr><td colspan="4">暂无审计记录</td></tr>') + '</table>';
    }

    var s10 = '<h2>十、演练结论</h2><p>'+App.esc(conclusion)+'</p>'
      + '<p>通过本次演练，检验了医院在「'+App.esc(sc?sc.type:'网络安全事件')+'」场景下的监测发现、研判决策、协同处置、业务兜底与上报取证能力，演练过程全程留痕、可回溯、可复盘。演练中形成的处置流程与改进建议，应纳入医院网络安全应急预案修订与年度培训计划。</p>';

    var s11 = '<h2>十一、附件</h2><ol>'
      + (o.tl ? '<li>附件1　处置流程时序表及关键操作记录（见第四章）</li>' : '')
      + (o.task ? '<li>附件2　演练任务明细表（见第五章 5.2）</li>' : '')
      + (o.ev ? '<li>附件3　取证材料清单（见第九章 9.1）</li>' : '')
      + '<li>附件4　平台审计日志（可在「系统基础管理 → 审计日志」导出 CSV）</li>'
      + '</ol>'
      + '<div class="sign">编制单位（盖章）：'+App.esc(brandUnit)+'信息科<br>编制人：'+App.esc(d.generatedBy||'—')+'　　编制日期：'+dateStr+'<br>审核：____________________　　批准：____________________<br>日期：______年____月____日</div>'
      + '<div class="foot">本报告由'+App.esc(brandUnit)+App.esc(brandSystem)+'依据演练数据库自动汇总生成，共 '+d.tasks.length+' 项任务、'+d.timeline.length+' 条操作记录、'+d.evidenceStats.total+' 件取证材料。报告内容为演练模拟数据，不含真实患者隐私信息，不作为真实网络安全事件的法定证据材料。</div>';

    var body = '<div class="page">'+s1+s2+s3+s4+s5+s6+s7+s8+s9+s10+s11+'</div>';
    return '<!DOCTYPE html><html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:w="urn:schemas-microsoft-com:office:word"><head><meta charset="UTF-8">'
      + '<title>网络安全应急演练报告-'+App.esc(p.id)+'</title><style>'+css+'</style></head><body>'+cover+body+'</body></html>';
  }

  function rpFilename(d, ext){
    return '网络安全应急演练报告-'+d.project.id+'-'+ymd()+'.'+ext;
  }
  function downloadFile(name, html, mime){
    var blob = new Blob(['\ufeff'+html], { type: mime });
    var a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name;
    document.body.appendChild(a);
    a.click();
    setTimeout(function(){ URL.revokeObjectURL(a.href); a.remove(); }, 3000);
  }
  function rpLog(d, fmt){
    API.markReportExported(d.project.id, fmt).catch(function(){});
  }

  function rpRun(action){
    if(rpBusy) return;
    rpBusy = true;
    loadReport().then(function(d){
      var o = rpOpts();
      var html = buildReport(d, o);
      var pname = d.project.name;
      if(action==='preview'){
        App.modal({ title:'演练报告预览 · '+pname, wide:true,
          body:'<iframe id="rpFrame" style="width:100%;height:70vh;border:1px solid var(--border2);border-radius:8px;background:#fff"></iframe>',
          foot:'<button class="btn" id="rpClose">关闭</button><button class="btn primary" id="rpDl">'+App.icon('download')+'导出 Word</button>',
          onOpen:function(ov){
            ov.querySelector('#rpFrame').srcdoc = html;
            ov.querySelector('#rpClose').onclick = ov.close;
            ov.querySelector('#rpDl').onclick = function(){
              downloadFile(rpFilename(d,'doc'), html, 'application/msword;charset=utf-8');
              rpLog(d,'Word');
              App.toast('已导出 Word：'+rpFilename(d,'doc'),'ok');
            };
          } });
      } else if(action==='word'){
        downloadFile(rpFilename(d,'doc'), html, 'application/msword;charset=utf-8');
        rpLog(d,'Word');
        App.toast('报告已导出（Word）：'+rpFilename(d,'doc'),'ok');
      } else if(action==='html'){
        downloadFile(rpFilename(d,'html'), html, 'text/html;charset=utf-8');
        rpLog(d,'HTML');
        App.toast('报告已导出（HTML）：'+rpFilename(d,'html'),'ok');
      } else if(action==='print'){
        var w = window.open('', '_blank');
        if(!w){ App.toast('浏览器拦截了打印窗口，请允许弹出窗口后重试','err'); rpBusy=false; return; }
        w.document.open(); w.document.write(html); w.document.close();
        rpLog(d,'PDF');
        setTimeout(function(){ try{ w.focus(); w.print(); }catch(e){} }, 500);
        App.toast('已打开打印预览，可选择"另存为 PDF"','ok');
      }
      rpBusy = false;
    }).catch(function(e){
      rpBusy = false;
      App.toast((e&&e.message)||'读取演练报告数据失败','err');
    });
  }

  document.getElementById('rpPreview').onclick = function(){ rpRun('preview'); };
  document.getElementById('rpWord').onclick = function(){ rpRun('word'); };
  document.getElementById('rpHtml').onclick = function(){ rpRun('html'); };
  document.getElementById('rpPrint').onclick = function(){ rpRun('print'); };
  rpProjSel.onchange = function(){ rpMeta('已切换项目，点击「预览报告」或导出按钮读取最新数据。'); };
  }
  return { mount: mount };
})();
