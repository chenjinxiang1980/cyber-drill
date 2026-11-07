/* 演练报告导出 · 已合并进「复盘评估」页面
 * 数据来源：GET /api/admin/report/:projectId（服务端聚合项目、剧本、任务、时序、取证、评分）
 * 评分口径：与复盘页同源，直接使用 /api/drill/workbench/:projectId 的 Score() 结果
 * 导出格式：Word(.doc) / HTML / 打印另存 PDF，导出动作写入审计日志
 */
window.ReportExport = (function () {
  'use strict';

  function arr(v) { return Array.isArray(v) ? v : []; }
  function objArr(v) { return Array.isArray(v) ? v.filter(function (x) { return x && typeof x === 'object'; }) : []; }
  function num(v, d) { var n = Number(v); return isFinite(n) ? n : (d || 0); }
  function obj(v) { return (v && typeof v === 'object' && !Array.isArray(v)) ? v : {}; }
  function p2(n) { return n < 10 ? '0' + n : String(n); }
  function isSimEvidence(e) { return !!(e && (e.simulated || /[（(]模拟[）)]\s*$/.test(String(e.name || '')))); }
  function ymd(d) { d = d || new Date(); return d.getFullYear() + p2(d.getMonth() + 1) + p2(d.getDate()); }
  function gradeOf(pct) {
    if (pct === null || pct === undefined || pct === '') return '未考评';
    if (pct >= 90) return '优秀'; if (pct >= 80) return '良好'; if (pct >= 70) return '合格'; return '不合格';
  }
  /* 分制口径：原始分一律配后端返回的 maxScore，百分比由 原始分/满分 换算，禁止硬编码 100 */
  function maxOf(s) { var m = Number(s && s.maxScore); return isFinite(m) && m > 0 ? m : 0; }
  function pctOf(score, max) {
    var sc = Number(score), mx = Number(max);
    if (!isFinite(sc) || !isFinite(mx) || mx <= 0) return null;
    /* 加极小量抵消浮点误差（82.6/112 应为 73.75% → 73.8%，而非 73.7%） */
    return Math.round(sc / mx * 1000 + 1e-6) / 10;
  }
  function scoreText(score, max) {
    var p = pctOf(score, max);
    if (score === null || score === undefined || score === '') return '未考评';
    return num(score) + (max > 0 ? (' / ' + max + ' 分') : ' 分') + (p === null ? '' : ('（得分率 ' + p + '%）'));
  }
  function todayCN() { var d = new Date(); return d.getFullYear() + '年' + (d.getMonth() + 1) + '月' + d.getDate() + '日'; }

  /* 兼容空响应/字段缺失：任何数组、对象、统计值都落到安全默认值 */
  function normalize(r, proj, wb, sealData) {
    r = obj(r);
    sealData = obj(sealData);
    var p = Object.assign({}, obj(proj), obj(r.project));
    p.units = arr(p.units);
    p.members = objArr(p.members);
    p.observers = arr(p.observers);
    p.mode = p.mode || '基础版';
    p.id = p.id || '';
    p.name = p.name || '演练项目';

    var tasks = objArr(r.tasks);
    var stats = Object.assign({}, obj(r.stats));
    stats.total = num(stats.total, tasks.length);
    stats.done = num(stats.done, tasks.filter(function (t) { return t && t.status === '已完成'; }).length);
    stats.doing = num(stats.doing, tasks.filter(function (t) { return t && t.status === '处理中'; }).length);
    stats.pending = num(stats.pending, tasks.filter(function (t) { return t && t.status === '待处理'; }).length);
    stats.overdue = num(stats.overdue, tasks.filter(function (t) { return t && t.status === '超时'; }).length);
    stats.rate = num(stats.rate, stats.total ? Math.round(stats.done / stats.total * 100) : 0);

    var byRole = objArr(r.byRole);
    if (!byRole.length && tasks.length) {
      var m = {}, order = [];
      tasks.forEach(function (t) {
        if (!t) return;
        var k = t.roleName || t.role || '—';
        if (!m[k]) { m[k] = { role: k, total: 0, done: 0, doing: 0, pending: 0, overdue: 0 }; order.push(k); }
        m[k].total++;
        if (t.status === '已完成') m[k].done++;
        else if (t.status === '处理中') m[k].doing++;
        else if (t.status === '超时') m[k].overdue++;
        else m[k].pending++;
      });
      byRole = order.map(function (k) { return m[k]; });
    }

    var evidence = objArr(r.evidence);
    var byType = Object.assign({}, obj(obj(r.evidenceStats).byType));
    if (!Object.keys(byType).length) {
      evidence.forEach(function (e) { if (e) { var k = e.type || '其他'; byType[k] = (byType[k] || 0) + 1; } });
    }
    var evStats = Object.assign({}, obj(r.evidenceStats));
    var isSim = isSimEvidence;
    evStats.total = num(evStats.total, evidence.length);
    evStats.simulated = evidence.length ? evidence.filter(isSim).length : num(evStats.simulated, 0);
    evStats.auto = evidence.length ? evidence.filter(function (e) { return e && e.auto && !isSim(e); }).length : num(evStats.auto, 0);
    evStats.manual = evidence.length ? Math.max(0, evStats.total - evStats.auto - evStats.simulated) : num(evStats.total - evStats.auto - evStats.simulated, 0);
    evStats.byType = byType;

    var scoreStats = Object.assign({ sum: 0, max: 0, pct: null, graded: 0, items: 0 }, obj(r.scoreStats));
    scoreStats.pct = (scoreStats.pct === null || scoreStats.pct === undefined || scoreStats.pct === '') ? null : num(scoreStats.pct);

    return {
      project: p,
      scenario: obj(r.scenario).id ? r.scenario : null,
      stats: stats,
      byRole: byRole,
      tasks: tasks,
      timeline: objArr(r.timeline),
      seqFlow: objArr(r.seqFlow),
      decisions: objArr(r.decisions),
      decisionStats: Object.assign({ baseline: 0, total: 0, correct: 0, wrong: 0, ontime: 0 }, obj(r.decisionStats)),
      evidence: evidence,
      evidenceStats: evStats,
      scores: objArr(r.scores),
      scoreStats: scoreStats,
      modeScore: obj(r.modeScore).max !== undefined ? r.modeScore : null,
      ledger: r.ledger || null,
      generatedBy: r.generatedBy || '',
      generatedAt: r.generatedAt || '',
      live: (wb && wb.score) ? wb : null,
      seal: sealData.seal || null,
      snapshot: (sealData.snapshot && sealData.snapshot.snapshot) ? sealData.snapshot : null,
      snapshots: arr(sealData.snapshots),
      endException: sealData.endException || null
    };
  }

  function liveScore(d) { return (d.live && d.live.score) ? d.live.score : null; }
  /* 封存/重算快照中的评分结果；有封存时报告一律读快照 */
  function sealedScore(d) { return (d.snapshot && d.snapshot.snapshot && d.snapshot.snapshot.score) ? d.snapshot.snapshot.score : null; }

  /* 与复盘页完全一致的改进建议口径 */
  function liveSuggestions(wb) {
    var s = wb.score || {}, nodes = objArr(s.nodes), lines = [];
    var missed = nodes.filter(function (n) { return !n.hit; });
    var late = nodes.filter(function (n) { return n.hit && !n.ontime; });
    var dedSum = function (arr) { return Math.round(arr.reduce(function (a, n) { return a + Math.abs(num(n.earned)); }, 0) * 10) / 10; };
    if (wb.mode === '基础版') {
      if (late.length) lines.push('1. 时限执行：' + late.map(function (n) { return '「' + n.title + '」'; }).join('、') + ' 未在处置时长内完成，建议明确各节点处置时长与责任人。');
      if (missed.length) lines.push((lines.length + 1) + '. 流程执行：' + missed.length + ' 项任务未按时闭环，建议在下一轮演练中重点跟踪。');
      if (!lines.length) lines.push('本次基础版演练各节点均在处置时长内完成，建议增加实战版无脚本抽测以验证真实处置能力。');
    } else {
      var wrong = nodes.filter(function (n) { return n.state === '决策有误'; });
      var omitted = nodes.filter(function (n) { return n.state === '遗漏'; });
      if (wrong.length) lines.push('1. 决策偏离：' + wrong.map(function (n) { return '「' + n.title + '」'; }).join('、') + ' 所选处置与剧本基线偏离（执行混淆流程，扣该流程全部分值，合计扣 ' + dedSum(wrong) + ' 分），建议围绕该环节开展专项研判训练。');
      if (late.length) lines.push((lines.length + 1) + '. 处置超时：' + late.map(function (n) { return '「' + n.title + '」'; }).join('、') + ' 超出处置时长（扣本节点分值的 20%，合计扣 ' + dedSum(late) + ' 分），建议优化响应流程与值班力量配置。');
      if (omitted.length) lines.push((lines.length + 1) + '. 环节未处置：' + omitted.map(function (n) { return '「' + n.title + '」（' + (n.plan || '—') + '）'; }).join('、') + ' 演练结束仍未完成（扣该流程全部分值，合计扣 ' + dedSum(omitted) + ' 分），建议明确责任岗位并补齐处置流程。');
      if (s.skippedDistract) lines.push((lines.length + 1) + '. 跳过混淆流程 ' + s.skippedDistract + ' 项（不扣分），建议复盘时说明判断依据。');
      if (!lines.length) lines.push('实战版演练全部基线环节决策正确且处置时长达标，无扣分项，建议固化处置经验并纳入标准剧本。');
    }
    return lines;
  }

  function liveIssues(d, score) {
    var out = [];
    var s = score || liveScore(d);
    if (s) {
      objArr(s.nodes).forEach(function (n) {
        if (n.state === '决策有误') out.push('「' + n.title + '」处置决策与剧本基线偏离（执行混淆流程，扣 ' + Math.abs(num(n.earned)) + ' 分）。');
        else if (n.state === '遗漏') out.push('「' + n.title + '」演练结束仍未处置（扣 ' + Math.abs(num(n.earned)) + ' 分）。');
        else if (n.late) out.push('「' + n.title + '」处置超出规定时长（扣本节点分值的 20%，扣 ' + Math.abs(num(n.earned)) + ' 分）。');
      });
    }
    d.tasks.forEach(function (t) {
      if (t && t.status !== '已完成') out.push('任务 ' + (t.id || '') + '「' + (t.title || '') + '」（' + (t.roleName || '—') + '）状态为「' + (t.status || '—') + '」，未形成闭环。');
    });
    if (d.evidenceStats.total === 0) out.push('本次演练未归档任何取证材料，证据链不完整。');
    else if (d.evidenceStats.simulated > 0) out.push('本项目为基础版演练，取证材料中有 ' + d.evidenceStats.simulated + ' 件为系统生成的模拟数据（已在明细中标注），不计入人工取证。');
    else if (d.evidenceStats.auto > 0 && d.evidenceStats.total === d.evidenceStats.auto) out.push('现有取证材料全部由系统自动生成，缺少人工现场取证记录。');
    return out;
  }

  function fallbackIssues(d) {
    var out = [];
    d.tasks.forEach(function (t) {
      if (t && t.status !== '已完成') out.push('任务 ' + (t.id || '') + '「' + (t.title || '') + '」（' + (t.roleName || '—') + '）状态为「' + (t.status || '—') + '」，未形成闭环。');
    });
    d.scores.forEach(function (s) {
      if (s && s.score !== null && s.score !== undefined && num(s.score) < num(s.max)) out.push('考评项「' + s.name + '」扣 ' + (num(s.max) - num(s.score)) + ' 分：' + (s.note || '未填写具体依据') + '。');
    });
    if (d.evidenceStats.total === 0) out.push('本次演练未归档任何取证材料，证据链不完整。');
    return out;
  }

  function fallbackSugs(d) {
    var out = [];
    if (d.stats.overdue > 0) out.push('针对超时任务建立分级时限提醒与督办机制，超时自动推送至院应急指挥部并纳入下次演练重点考核。');
    if (d.stats.pending > 0 || d.stats.doing > 0) out.push('对未闭环任务补充处置记录与验证结论，确保"发现—处置—恢复—复盘"全流程留痕。');
    if (d.evidenceStats.auto > 0) out.push('加强人工取证（告警截图、日志片段、配置备份），与自动取证材料相互印证，满足事件溯源的证据要求。');
    out.push('定期更新应急预案与剧本节点，每半年至少开展一次同场景实战化演练，保持处置流程熟练度。');
    return out;
  }

  function buildHTML(d, o) {
    o = obj(o);
    var p = d.project, sc = d.scenario, st = d.stats, ss = d.scoreStats;
    var live = sealedScore(d) || liveScore(d);
    var sealed = !!sealedScore(d);
    var sealGrade = (d.snapshot && d.snapshot.grade) ? d.snapshot.grade : null;
    var grade = gradeOf(ss.pct);
    var no = 'DR-' + p.id + '-RPT-' + ymd();
    var memberRows = objArr(p.members).map(function (m) {
      m = obj(m);
      var r = (window.MOCK ? arr(MOCK.ROLES) : []).filter(function (x) { return x.id === m.role; })[0] || {};
      return '<tr><td>' + App.esc(r.name || m.role) + '</td><td>' + App.esc(m.user || '—') + '</td><td>' + App.esc(r.desc || '按角色职责参演') + '</td></tr>';
    }).join('');
    var byRoleRows = d.byRole.map(function (r) {
      return '<tr><td>' + App.esc(r.role) + '</td><td>' + r.total + '</td><td>' + r.done + '</td><td>' + r.doing + '</td><td>' + r.pending + '</td><td>' + r.overdue + '</td></tr>';
    }).join('');
    var taskRows = d.tasks.map(function (t) {
      return '<tr><td>' + App.esc(t.id) + '</td><td>' + App.esc(t.title) + '</td><td>' + App.esc(t.roleName) + '</td><td>' + App.esc(t.level) + '</td><td>' + (t.duration ? t.duration + ' 分钟' : '—') + '</td><td>' + App.esc(t.status) + '</td></tr>';
    }).join('');
    var tlRows = d.timeline.map(function (x) {
      return '<tr><td>' + App.esc(x.t) + '</td><td>' + App.esc(x.action) + '</td><td>' + App.esc(x.actor || x.role || '—') + '</td><td>' + App.esc(x.detail || '') + '</td></tr>';
    }).join('');
    var seqRows = d.seqFlow.map(function (x) {
      return '<tr><td>' + App.esc(x.time) + '</td><td>' + App.esc(x.name) + '</td><td>' + App.esc(x.role) + '</td><td>' + App.esc(x.state || '完成') + '</td></tr>';
    }).join('');
    var pmode = p.mode || '基础版';
    var decisionRows = d.decisions.map(function (x) {
      return '<tr><td>' + App.esc(x.title) + '</td><td>' + App.esc(x.target || '—') + '</td><td>' + App.esc(x.choice || '—') + '</td>'
        + '<td>' + (x.correct ? '正确' : '有误') + '</td><td>' + App.esc(x.stepTime || '—') + '</td><td>' + App.esc(x.user || '—') + '</td>'
        + '<td>' + App.esc(x.found || '—') + '<br><span style="color:#444">措施：' + App.esc(x.measure || '—') + '</span><br><span style="color:#444">结果：' + App.esc(x.result || '—') + '</span></td></tr>';
    }).join('');
    var scoreRows = d.scores.map(function (s) {
      return '<tr><td>' + App.esc(s.itemId) + ' ' + App.esc(s.name) + '</td><td>' + (s.max === null || s.max === undefined ? '—' : s.max) + '</td><td>' + (s.score === null || s.score === undefined ? '未评' : s.score) + '</td><td>' + App.esc(s.note || s.rule || '—') + '</td></tr>';
    }).join('');
    var evTypeRows = Object.keys(d.evidenceStats.byType).map(function (k) {
      return '<tr><td>' + App.esc(k) + '</td><td>' + d.evidenceStats.byType[k] + '</td></tr>';
    }).join('');
    var evRows = d.evidence.map(function (e) {
      var sim = isSimEvidence(e);
      return '<tr><td>' + App.esc(e.id) + '</td><td>' + App.esc(e.type) + '</td><td>' + App.esc(e.name) + '</td><td>' + App.esc(e.size || '—') + '</td><td>' + App.esc(e.time || '—') + '</td><td>' + App.esc(e.taskId || '—') + '</td><td>' + (sim ? '<b style="color:#a15c00">模拟数据（不计人工）</b>' : (e.auto ? '系统自动生成' : '人工上传')) + '</td></tr>';
    }).join('');
    var auditRows = objArr(window.MOCK && MOCK.AUDIT_LOGS).slice(0, 15).map(function (a) {
      return '<tr><td>' + App.esc(a.time) + '</td><td>' + App.esc(a.user) + '</td><td>' + App.esc(a.action) + '</td><td>' + App.esc(a.ip || '—') + '</td></tr>';
    }).join('');

    var issues = live ? liveIssues(d, live) : fallbackIssues(d);
    var sugs = live ? liveSuggestions({ mode: pmode, score: live }) : fallbackSugs(d);
    var issueHtml = issues.length ? '<ol>' + issues.map(function (x) { return '<li>' + App.esc(x) + '</li>'; }).join('') + '</ol>' : '<p>本次演练各环节均按剧本完成，未发现明显问题。</p>';
    var sugHtml = '<ol>' + sugs.map(function (x) { return '<li>' + App.esc(x) + '</li>'; }).join('') + '</ol>';

    var liveMax = live ? (maxOf(live) || num(d.snapshot && d.snapshot.maxScore, 0)) : 0;
    var livePct = live ? pctOf(live.score, liveMax) : null;
    var evalScore, evalTxt, evalPct;
    if (live) {
      evalScore = num(live.score);
      evalPct = livePct;
      evalTxt = scoreText(live.score, liveMax) + '（' + (sealed ? '封存评分快照 v' + (d.snapshot.version || 1) : '复盘评估系统自动考评') + '，等级' + ((livePct !== null) ? gradeOf(livePct) : (sealGrade || gradeOf(evalScore))) + '）';
    } else if (d.modeScore) {
      evalScore = num(d.modeScore.score);
      var mm = num(d.modeScore.max, 0);
      evalPct = pctOf(evalScore, mm);
      evalTxt = scoreText(evalScore, mm) + '（演练模式综合评分，等级' + ((evalPct !== null) ? gradeOf(evalPct) : gradeOf(evalScore)) + '）';
    } else {
      evalScore = ss.pct;
      evalPct = ss.pct;
      evalTxt = ss.pct === null ? '未开展' : (ss.sum + ' / ' + (ss.max || '—') + ' 分（得分率 ' + ss.pct + '%，等级' + grade + '）');
    }

    var conclusion = '本次演练以「' + (sc ? sc.name : '—') + '」为场景，演练等级' + App.esc(p.level || '—') + '，共设置处置任务 ' + st.total + ' 项，完成 ' + st.done +
      ' 项，完成率 ' + st.rate + '%' + (st.overdue ? ('，超时 ' + st.overdue + ' 项') : '') + '；系统自动考评为 ' + evalTxt +
      '。' + (evalPct === null ? '总体完成情况尚可，建议补充考评后形成量化结论。' : (evalPct >= 80 && st.rate >= 90 ? '演练组织有序、职责清晰、处置及时，达到演练预期目标。' : '演练基本达到预期目标，但在时限控制、证据完备性与流程闭环方面仍需改进。'));

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
      + '@page{size:A4;margin:2cm}'
      + '@media print{.page{padding:0;max-width:none}}';

    var B = (window.DRILL && DRILL.brand && DRILL.brand()) || {};
    var brandUnit = B.unitName || '示例医院';
    var brandSystem = B.systemName || '网络安全应急演练系统';
    var dateStr = todayCN();

    var cover = '<div class="cover"><div class="cls">密级：内部（演练专用）<br>编号：' + App.esc(no) + '</div>'
      + '<h1>网络安全应急演练报告</h1><div class="line"></div><div class="sub">' + App.esc(p.name) + '</div>'
      + '<div class="meta">演练类型：' + App.esc(p.type || '—') + '<br>事件等级：' + App.esc(p.level || '—') + '<br>演练日期：' + App.esc(p.date || '—') + '<br>项目编号：' + App.esc(p.id)
      + (sealed ? ('<br>评分封存：v' + App.esc(d.snapshot.version) + ' · 核心证据完整率 ' + num(d.snapshot.coreCompleteRate) + '%（' + num(d.snapshot.coreDone) + '/' + num(d.snapshot.coreTotal) + '）· ' + App.esc(d.snapshot.endMode || '完整')) : '')
      + '</div>'
      + '<div class="meta" style="margin-top:2.6cm">编制单位：' + App.esc(brandUnit) + '信息科<br>编制日期：' + dateStr + '<br>编制人：' + App.esc(d.generatedBy || '—') + '</div>'
      + '<div class="no">' + App.esc(brandUnit) + '　' + App.esc(brandSystem) + '</div></div>';

    var s1 = '<h2>一、演练概况</h2><table>'
      + '<tr><td class="k">项目名称</td><td colspan="3">' + App.esc(p.name) + '</td></tr>'
      + '<tr><td class="k">项目编号</td><td>' + App.esc(p.id) + '</td><td class="k">演练类型</td><td>' + App.esc(p.type || '—') + '</td></tr>'
      + '<tr><td class="k">事件等级</td><td>' + App.esc(p.level || '—') + '</td><td class="k">演练状态</td><td>' + App.esc(p.status || '—') + '</td></tr>'
      + '<tr><td class="k">演练日期</td><td>' + App.esc(p.date || '—') + '</td><td class="k">演练地点</td><td>院内演练环境（独立演练服务器，与生产网物理隔离）</td></tr>'
      + '<tr><td class="k">参演单位</td><td colspan="3">' + App.esc(p.units.join('、') || '示例医院') + '</td></tr>'
      + '<tr><td class="k">演练目标</td><td colspan="3">' + App.esc(p.target || '—') + '</td></tr>'
      + '<tr><td class="k">场景剧本</td><td colspan="3">' + App.esc(sc ? (sc.name + '（' + (sc.version || '—') + '）') : '—') + (sc ? ('，剧本节点 ' + sc.nodes + ' 个') : '') + '</td></tr>'
      + '</table>';

    var s2 = '<h2>二、演练组织与职责分工</h2><table><tr><th style="width:26%">参演角色</th><th style="width:20%">参演人员</th><th>主要职责</th></tr>'
      + (memberRows || '<tr><td colspan="3">未配置参演人员</td></tr>')
      + '<tr><td class="k">观察员</td><td colspan="2">' + App.esc(p.observers.join('、') || '—') + '</td></tr></table>'
      + '<p>本次演练采用"院应急指挥部统一指挥、信息科牵头处置、医务与临床协同、外部单位按需报送"的组织模式，各角色按剧本节点接收任务并在平台内留痕。</p>';

    var s3 = '<h2>三、演练准备</h2><p>1. 场景与剧本：' + (sc ? App.esc(sc.name) + '，' + App.esc(sc.descr || '') + '；剧本共 ' + sc.nodes + ' 个节点，明确各节点责任角色、处置时长与需提交表单。' : '未关联场景剧本。') + '</p>'
      + '<p>2. 演练环境：系统部署于独立演练服务器/虚拟机，与医院 HIS、业务网及互联网物理隔离；全部处置动作在平台内模拟记录，不下发真实网络配置命令，不操作生产数据。</p>'
      + '<p>3. 数据准备：演练数据全部为虚拟测试数据，未录入任何真实患者隐私信息；告警、系统 IP、故障现象等由平台按场景自动填充，参演人员核对后确认。</p>'
      + '<p>4. 保障措施：平台账号按角色分配，登录采用口令+验证码；操作全过程写入审计日志（仅追加、不可删除）。</p>';

    var s4 = '<h2>四、演练实施过程</h2>';
    if (o.tl) {
      s4 += '<h3>4.1 处置流程时序</h3><table><tr><th style="width:12%">时刻</th><th style="width:30%">处置环节</th><th style="width:22%">责任角色</th><th>状态</th></tr>'
        + (seqRows || '<tr><td colspan="4">暂无流程节点数据</td></tr>') + '</table>'
        + '<h3>4.2 关键操作记录</h3><table><tr><th style="width:12%">时间</th><th style="width:26%">动作</th><th style="width:20%">操作人/角色</th><th>说明</th></tr>'
        + (tlRows || '<tr><td colspan="4">暂无操作记录</td></tr>') + '</table>';
    }
    s4 += pmode === '实战版'
      ? '<p>演练按照"系统异常告警 → 参演人员自主研判 → 选择处置方案 → 分角色处置 → 业务兜底 → 上报取证 → 恢复验证 → 复盘评估"的闭环推进。系统不发布处置任务与工作提示，仅弹出系统异常故障警告，由参演人员在角色工作台自行选择处置方案并填写发现情况、处置措施与结果，全过程在平台内留痕。</p>'
      : '<p>演练按照"告警发现 → 核实研判 → 启动预案 → 分角色处置 → 业务兜底 → 上级上报 → 报案取证 → 恢复验证 → 复盘评估"的闭环流程推进，各节点任务在平台内完成接单、处置与归档。</p>';

    var s5 = '<h2>五、演练任务完成情况</h2>';
    if (pmode === '实战版') {
      var ds = d.decisionStats;
      s5 += '<p>本项目采用实战版演练，系统不在角色工作台发布处置步骤与工作提示，仅展示已发现的系统故障并由参演人员自主研判处置。以下统计为依据角色工作台处置记录与剧本基线环节比对的结果。</p>'
        + '<table><tr><th>基线必经环节</th><th>已处置</th><th>决策正确</th><th>决策有误</th><th>时限内</th><th>处置覆盖率</th></tr>'
        + '<tr><td>' + ds.baseline + '</td><td>' + ds.total + '</td><td>' + ds.correct + '</td><td>' + ds.wrong + '</td><td>' + ds.ontime + '</td><td>' + (ds.baseline ? Math.round(ds.total / ds.baseline * 100) : 0) + '%</td></tr></table>'
        + '<h3>5.1 处置决策记录</h3><table><tr><th>处置环节</th><th style="width:12%">责任角色</th><th>选择方案</th><th style="width:7%">判定</th><th style="width:8%">处置时刻</th><th style="width:12%">处置人</th><th>发现情况 / 措施 / 结果</th></tr>'
        + (decisionRows || '<tr><td colspan="7">暂无实战处置记录</td></tr>') + '</table>';
    } else {
      s5 += '<table><tr><th>任务总数</th><th>已完成</th><th>处理中</th><th>待处理</th><th>超时</th><th>完成率</th></tr>'
        + '<tr><td>' + st.total + '</td><td>' + st.done + '</td><td>' + st.doing + '</td><td>' + st.pending + '</td><td>' + st.overdue + '</td><td>' + st.rate + '%</td></tr></table>'
        + '<h3>5.1 分角色完成情况</h3><table><tr><th>接收角色</th><th>任务数</th><th>已完成</th><th>处理中</th><th>待处理</th><th>超时</th></tr>'
        + (byRoleRows || '<tr><td colspan="6">暂无任务</td></tr>') + '</table>';
      if (o.task) {
        s5 += '<h3>5.2 任务明细</h3><table><tr><th style="width:10%">编号</th><th>任务标题</th><th style="width:22%">接收角色</th><th style="width:8%">等级</th><th style="width:12%">处置时长</th><th style="width:10%">状态</th></tr>'
          + (taskRows || '<tr><td colspan="6">暂无任务</td></tr>') + '</table>';
      }
    }

    var s6 = '<h2>六、演练评估</h2>';
    var maxLabel = liveMax > 0 ? ('剧本满分 ' + liveMax + ' 分') : '剧本节点分值合计';
    s6 += '<p>演练模式：<b>' + App.esc(pmode) + '</b>。' + (pmode === '实战版'
      ? '本项目采用实战版（自主研判），角色工作台只展示已发现的系统故障，不发布处置步骤与引导提示；参演人员对已发现故障从混淆方案中自主选择处置动作，并实际填写发现时间、发现情况、处置措施与结果。系统以' + maxLabel + '为基础：超时完成扣本节点分值的 20%，接收人选择错误扣 30%，处置关键词必含项缺失按缺失比例合计最多扣 30%，处置文本质量不达标扣 20%，证据材料未提交或未命中要求扣 20%，单节点合计扣分上限 80%；执行混淆流程扣该流程全部分值，演练结束仍未完成的正确流程扣全部分值，演练中强制查看脚本扣 10 分。'
      : '本项目采用基础版（诱导演练），系统按剧本节点逐节点下发任务、给出引导提示并自动填充相关表单与截图。考评以' + maxLabel + '为基础：超时完成扣本节点分值的 20%，演练结束仍未完成的正确流程扣该流程全部分值；基础版不设置质量规则与证据门槛，一键完成生成的模拟材料以“模拟数据”标注，不计入人工取证。') + '</p>';
    if (live) {
      s6 += '<table><tr><th style="width:32%">复盘评估自动考评</th><th style="width:10%">满分</th><th style="width:10%">得分</th><th>计分依据</th></tr>'
        + '<tr><td>演练模式综合评分（' + App.esc(pmode) + '）</td><td>' + (liveMax || '—') + '</td><td>' + live.score + (livePct !== null ? ('（得分率 ' + livePct + '%）') : '') + '</td><td>' + App.esc(live.formula || '—') + '</td></tr></table>';
      if (sealed && d.snapshot) {
        var sn = d.snapshot, ex6 = obj(d.endException);
        s6 += '<h3>6.1 评分封存</h3><table>'
          + '<tr><td class="k">封存版本</td><td>v' + App.esc(sn.version) + '（' + App.esc(sn.snapshotStatus) + '）</td><td class="k">结束方式</td><td>' + App.esc(sn.endMode || (d.seal && d.seal.endMode) || '完整') + '</td></tr>'
          + '<tr><td class="k">封存时间</td><td>' + App.esc(sn.sealedAt || '—') + '</td><td class="k">封存人</td><td>' + App.esc(sn.sealedBy || '—') + '</td></tr>'
          + '<tr><td class="k">核心证据完整率</td><td>' + num(sn.coreCompleteRate) + '%（' + num(sn.coreDone) + '/' + num(sn.coreTotal) + '）</td>'
          + '<td class="k">证据归档率</td><td>' + num(sn.evidenceCompleteRate) + '%</td></tr>'
          + '<tr><td class="k">规则版本</td><td>' + App.esc(sn.ruleVersion || '—') + '</td><td class="k">剧本版本</td><td>' + App.esc(sn.scenarioVersion || '—') + '</td></tr>';
        if (d.seal && d.seal.endMode === '有缺项') {
          s6 += '<tr><td class="k">缺项说明</td><td colspan="3">' + App.esc(ex6.reason || '—')
            + '<br>批准人：' + App.esc(ex6.approver || '—') + '　责任人：' + App.esc(ex6.owner || '—') + '　补录时限：' + App.esc(ex6.deadline || '—') + '</td></tr>';
        }
        s6 += '</table>';
        var others = arr(d.snapshots).filter(function (x) { return x.version !== sn.version; });
        if (others.length) {
          s6 += '<p style="font-size:9.5pt;color:#444">历史版本：' + others.map(function (x) {
            return 'v' + x.version + '（' + x.snapshotStatus + (x.recalcReason ? '·' + x.recalcReason : '') + '，' + (x.score) + ' / ' + (num(x.maxScore, 0) || '—') + ' 分）';
          }).join('；') + '。重算只追加新版本，不覆盖封存版本。</p>';
        }
      } else if (p.status === '已结束' || p.status === '已归档') {
        s6 += '<p style="color:#8a5a00">提示：本项目已结束，但未写入评分封存快照（历史经状态接口收口），当前展示实时考评结果，需按补封存流程处理后方可作为正式归档版本。</p>';
      }
      var nodeRows = objArr(live.nodes).map(function (n) {
        return '<tr><td>' + App.esc(n.title) + '</td><td>' + App.esc(n.plan || '—') + '</td><td>' + App.esc(n.actual || '—') + '</td><td>' + App.esc(n.state || '—') + '</td></tr>';
      }).join('');
      s6 += '<h3>' + (sealed ? '6.2' : '6.1') + ' 考评明细（' + (sealed ? '封存快照' : '与复盘评估页一致') + '）</h3><table><tr><th>考评单元</th><th style="width:16%">计划处置时长</th><th style="width:16%">实际处置</th><th style="width:10%">判定</th></tr>'
        + (nodeRows || '<tr><td colspan="4">剧本未配置考评单元</td></tr>') + '</table>'
        + '<p class="sum">总分：' + live.score + ' / ' + (liveMax || '—') + (livePct !== null ? ('（得分率 ' + livePct + '%）') : '') + '　评估等级：' + App.esc((livePct !== null) ? gradeOf(livePct) : (sealGrade || gradeOf(num(live.score)))) + '</p>';
    } else if (d.modeScore) {
      s6 += '<table><tr><th style="width:32%">模式化自动考评</th><th style="width:10%">满分</th><th style="width:10%">得分</th><th>计分依据</th></tr>'
        + '<tr><td>演练模式综合评分（' + App.esc(pmode) + '）</td><td>' + (num(d.modeScore.max, 0) || '—') + '</td><td>' + num(d.modeScore.score) + '</td><td>' + App.esc(d.modeScore.note || '—') + '</td></tr></table>';
    } else if (ss.graded === 0) {
      s6 += '<p>本次演练尚未执行系统自动考评，暂无量化评分结果，建议演练结束后在「复盘评估」中完成考评。</p>';
    } else {
      s6 += '<table><tr><th style="width:26%">考评指标</th><th style="width:8%">满分</th><th style="width:8%">得分</th><th>自动考评依据</th></tr>'
        + scoreRows + '</table><p class="sum">总分：' + ss.sum + ' / ' + ss.max + '　得分率：' + ss.pct + '%　评估等级：' + grade + '</p>';
    }

    var s7 = '<h2>七、存在问题</h2>' + issueHtml;
    var s8 = '<h2>八、改进建议</h2>' + sugHtml;
    var s9 = '<h2>九、取证与材料归档</h2>'
      + (d.evidenceStats.simulated > 0 ? '<p style="padding:6px 10px;border-left:3px solid #c77c02;background:#fff7e6;color:#7a4a00">注意：本项目为基础版演练，取证清单中包含 <b>' + d.evidenceStats.simulated + ' 件系统生成的模拟材料</b>（明细中已标注“模拟数据”），不计入人工取证，也不作为人工提交的证明。</p>' : '')
      + '<p>本次演练共归档取证材料 ' + d.evidenceStats.total + ' 件，其中系统自动生成 ' + d.evidenceStats.auto + ' 件、模拟数据 ' + d.evidenceStats.simulated + ' 件、人工上传 ' + d.evidenceStats.manual + ' 件。</p><table><tr><th style="width:40%">材料类型</th><th>数量</th></tr>'
      + (evTypeRows || '<tr><td colspan="2">暂无取证材料</td></tr>') + '</table>';
    if (o.ev) {
      s9 += '<h3>9.1 取证材料清单</h3><table><tr><th style="width:10%">编号</th><th style="width:12%">类型</th><th>材料名称</th><th style="width:8%">大小</th><th style="width:10%">时间</th><th style="width:9%">关联任务</th><th style="width:10%">来源</th></tr>'
        + (evRows || '<tr><td colspan="7">暂无取证材料</td></tr>') + '</table>';
    }
    if (o.audit) {
      s9 += '<h3>9.2 平台审计留痕摘要（最近 15 条）</h3><table><tr><th style="width:18%">时间</th><th style="width:12%">操作人</th><th>操作内容</th><th style="width:14%">来源 IP</th></tr>'
        + (auditRows || '<tr><td colspan="4">暂无审计记录</td></tr>') + '</table>';
    }

    var s10 = '<h2>十、演练结论</h2><p>' + App.esc(conclusion) + '</p>'
      + '<p>通过本次演练，检验了医院在「' + App.esc(sc ? sc.name : '本次演练') + '」场景下的监测发现、研判决策、协同处置、业务兜底与上报取证能力，演练过程全程留痕、可回溯、可复盘。演练中形成的处置流程与改进建议，应纳入医院网络安全应急预案修订与年度培训计划。</p>';

    var s11 = '<h2>十一、附件</h2><ol>'
      + (o.tl ? '<li>附件1　处置流程时序表及关键操作记录（见第四章）</li>' : '')
      + (o.task ? '<li>附件2　演练任务明细表（见第五章 5.2）</li>' : '')
      + (o.ev ? '<li>附件3　取证材料清单（见第九章 9.1）</li>' : '')
      + '<li>附件4　平台审计日志（可在「系统基础管理 → 审计日志」导出 CSV）</li>'
      + '</ol>'
      + '<div class="sign">编制单位（盖章）：' + App.esc(brandUnit) + '信息科<br>编制人：' + App.esc(d.generatedBy || '—') + '　　编制日期：' + dateStr + '<br>审核：____________________　　批准：____________________<br>日期：______年____月____日</div>'
      + '<div class="foot">本报告由' + App.esc(brandUnit) + App.esc(brandSystem) + '依据演练数据库自动汇总生成，共 ' + d.tasks.length + ' 项任务、' + d.timeline.length + ' 条操作记录、' + d.evidenceStats.total + ' 件取证材料。报告内容为演练模拟数据，不含真实患者隐私信息，不作为真实网络安全事件的法定证据材料。</div>';

    return '<!DOCTYPE html><html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:w="urn:schemas-microsoft-com:office:word"><head><meta charset="UTF-8">'
      + '<title>网络安全应急演练报告-' + App.esc(p.id) + '</title><style>' + css + '</style></head><body>'
      + cover + '<div class="page">' + s1 + s2 + s3 + s4 + s5 + s6 + s7 + s8 + s9 + s10 + s11 + '</div></body></html>';
  }

  function filename(d, ext) {
    var id = (d.project && d.project.id) || 'PROJECT';
    return '网络安全应急演练报告-' + id + '-' + ymd() + '.' + ext;
  }
  function download(name, html, mime) {
    var blob = new Blob(['\ufeff' + html], { type: mime });
    var a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name;
    document.body.appendChild(a);
    a.click();
    setTimeout(function () { URL.revokeObjectURL(a.href); a.remove(); }, 3000);
  }

  var busy = false;
  function run(action, ctx) {
    if (busy) return;
    var proj = ctx.project;
    if (!proj || !proj.id) { App.toast('请先选择演练项目', 'err'); return; }
    busy = true;
    setMeta(ctx.host, '正在读取演练数据（项目 / 剧本 / 任务 / 时序 / 评分 / 取证）…');
    var ready;
    try {
      ready = ctx.ensureWorkbench ? ctx.ensureWorkbench() : Promise.resolve(ctx.getWorkbench ? ctx.getWorkbench() : null);
    } catch (e) {
      ready = Promise.resolve(ctx.getWorkbench ? ctx.getWorkbench() : null);
    }
    Promise.resolve(ready).then(function (wb) {
      return Promise.all([
        API.getDrillReport(proj.id),
        DRILL.get('/content/evaluation?projectId=' + encodeURIComponent(proj.id)).catch(function () { return null; })
      ]).then(function (rr) {
        var raw = rr[0], sealData = rr[1];
        var d = normalize(raw, proj, wb, sealData);
      var html = buildHTML(d, options(ctx.host));
      var pname = d.project.name;
      if (action === 'preview') {
        App.modal({ title: '演练报告预览 · ' + pname, wide: true,
          body: '<iframe id="rpFrame" style="width:100%;height:70vh;border:1px solid var(--border2);border-radius:8px;background:#fff"></iframe>',
          foot: '<button class="btn" id="rpClose">关闭</button><button class="btn primary" id="rpDl">' + App.icon('download') + '导出 Word</button>',
          onOpen: function (ov) {
            ov.querySelector('#rpFrame').srcdoc = html;
            ov.querySelector('#rpClose').onclick = ov.close;
            ov.querySelector('#rpDl').onclick = function () {
              download(filename(d, 'doc'), html, 'application/msword;charset=utf-8');
              logExport(proj.id, 'Word');
              App.toast('已导出 Word：' + filename(d, 'doc'), 'ok');
            };
          } });
      } else if (action === 'word') {
        download(filename(d, 'doc'), html, 'application/msword;charset=utf-8');
        logExport(proj.id, 'Word');
        App.toast('报告已导出（Word）：' + filename(d, 'doc'), 'ok');
      } else if (action === 'html') {
        download(filename(d, 'html'), html, 'text/html;charset=utf-8');
        logExport(proj.id, 'HTML');
        App.toast('报告已导出（HTML）：' + filename(d, 'html'), 'ok');
      } else if (action === 'print') {
        var w = window.open('', '_blank');
        if (!w) { App.toast('浏览器拦截了打印窗口，请允许弹出窗口后重试', 'err'); busy = false; return; }
        w.document.open(); w.document.write(html); w.document.close();
        logExport(proj.id, 'PDF');
        setTimeout(function () { try { w.focus(); w.print(); } catch (e) {} }, 500);
        App.toast('已打开打印预览，可选择"另存为 PDF"', 'ok');
      }
      setMeta(ctx.host, '已读取「' + d.project.name + '」：任务 ' + d.stats.total + ' 项 · 时序 ' + d.timeline.length +
        ' 条 · 取证 ' + d.evidenceStats.total + ' 件 · 评分 ' + (d.snapshot ? ('封存 v' + d.snapshot.version + ' · ' + num(d.snapshot.score) + ' / ' + (num(d.snapshot.maxScore, 0) || '—')) :
        (d.live ? (num(d.live.score.score) + ' / ' + (maxOf(d.live.score) || '—') + '（复盘评估）') :
        (d.scoreStats.pct === null ? '未考评' : (d.scoreStats.sum + '/' + d.scoreStats.max + '（' + d.scoreStats.pct + '%）')))));
      busy = false;
      });
    }).catch(function (e) {
      busy = false;
      var msg = (e && e.message) ? e.message : '未知错误';
      setMeta(ctx.host, '报告数据读取失败：' + App.esc(msg));
      App.toast('报告数据读取失败：' + msg, 'err');
    });
  }

  function logExport(pid, fmt) {
    API.markReportExported(pid, fmt).then(function () {}).catch(function (e) {
      App.toast('导出留痕写入失败：' + ((e && e.message) || '请检查权限'), 'err');
    });
  }

  function options(host) {
    var q = function (id) { var el = host.querySelector('#' + id); return !!(el && el.checked); };
    return { tl: q('rptOptTl'), task: q('rptOptTask'), ev: q('rptOptEv'), audit: q('rptOptAudit') };
  }
  function setMeta(host, t) { var el = host.querySelector('#rptMeta'); if (el) el.innerHTML = t; }

  /* 复盘评估页内的导出卡片（无项目选择器，始终导出当前项目） */
  function panelHTML(project) {
    var pname = project ? (project.name + '（' + project.id + '）') : '—';
    return '<div class="card-title">' + App.icon('file') + '演练报告导出<span class="more">与上方复盘评估同源</span></div>'
      + '<div class="small dim" style="margin-bottom:10px">当前项目：<b>' + App.esc(pname) + '</b>。报告评分直接取复盘评估同一套自动考评结果，导出内容与页面展示一致。</div>'
      + '<div class="filters" style="margin-bottom:10px">'
      +   '<button class="btn primary" id="rptPreview">' + App.icon('eye') + '预览报告</button>'
      +   '<button class="btn" id="rptWord">' + App.icon('download') + '导出 Word</button>'
      +   '<button class="btn" id="rptHtml">' + App.icon('download') + '导出 HTML</button>'
      +   '<button class="btn" id="rptPrint">' + App.icon('file') + '打印 / 另存 PDF</button>'
      + '</div>'
      + '<div class="filters" style="gap:18px">'
      +   '<label class="small"><input type="checkbox" id="rptOptTl" checked> 处置时序表</label>'
      +   '<label class="small"><input type="checkbox" id="rptOptTask" checked> 任务明细表</label>'
      +   '<label class="small"><input type="checkbox" id="rptOptEv" checked> 取证材料清单</label>'
      +   '<label class="small"><input type="checkbox" id="rptOptAudit"> 审计留痕摘要</label>'
      + '</div>'
      + '<div class="tiny dim mt3" id="rptMeta">报告共十一章，导出操作将写入审计日志。</div>';
  }

  function bind(host, ctx) {
    if (!host) return;
    ctx = ctx || {};
    /* 调用方只传绑定容器时，run/options 里的 ctx.host 必须回落到同一容器，否则点击即抛错并卡死 busy */
    ctx.host = ctx.host || host;
    var btn = function (id) { return host.querySelector('#' + id); };
    var fire = function (action) {
      return function () {
        try { run(action, ctx); }
        catch (e) { busy = false; App.toast('报告导出失败：' + ((e && e.message) || e), 'err'); }
      };
    };
    if (btn('rptPreview')) btn('rptPreview').onclick = fire('preview');
    if (btn('rptWord')) btn('rptWord').onclick = fire('word');
    if (btn('rptHtml')) btn('rptHtml').onclick = fire('html');
    if (btn('rptPrint')) btn('rptPrint').onclick = fire('print');
  }

  return { panelHTML: panelHTML, bind: bind, buildHTML: buildHTML, normalize: normalize };
})();
