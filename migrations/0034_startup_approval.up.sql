-- 0034 启动审批表与启动节点绑定（P1-04）
-- S02–S08 的启动审批原来挂在通用审批表 F9；统一改用《应急启动申请单》F14，
-- 并扩展 F14 支持Ⅰ–Ⅳ级启动/结束与启动令编号。
-- 纯 DML，固定终值，可重复执行。

UPDATE form_templates
   SET name = '应急启动/结束审批单',
       fields = '[{"label":"审批事项","type":"select","options":["启动Ⅰ级应急响应","启动Ⅱ级应急响应","启动Ⅲ级应急响应","启动Ⅳ级应急响应","结束Ⅰ级应急响应","结束Ⅱ级应急响应","结束Ⅲ级应急响应","结束Ⅳ级应急响应"],"required":true,"hint":"选择本节点要审批的事项，须与研判结论中的响应级别一致。"},{"label":"前置申请要点","type":"textarea","required":false,"hint":"自动带出前置节点提交的申请内容，可补充摘要；正式依据以上方只读区为准。"},{"label":"审批结论","type":"select","options":["同意","不同意","有条件同意"],"required":true,"hint":"根据前置申请与现场态势作出明确结论。"},{"label":"审批意见","type":"textarea","required":true,"hint":"写明同意/不同意的理由、附加条件与执行要求。"},{"label":"授权范围","type":"multiselect","options":["停机","断网隔离","数据恢复","对外上报","对外发布口径","全院预警"],"required":true,"hint":"勾选本次批准授权的处置范围（可多选）；未勾选的事项不得执行。"},{"label":"批准响应级别","type":"select","options":["Ⅰ级","Ⅱ级","Ⅲ级","Ⅳ级"],"required":true,"hint":"批准启动或结束的响应级别。"},{"label":"启动令编号","type":"text","required":false,"hint":"启动时填写院内启动令编号；结束后审批可留空。"},{"label":"审批人","type":"text","required":true,"hint":"填写审批人姓名与职务。"},{"label":"审批时间","type":"datetime","required":true,"hint":"填写审批完成时间。"}]'
 WHERE id = 'F14';

UPDATE script_nodes SET form = 'F14' WHERE id IN ('S02-5','S03-5','S04-5','S05-5','S06-5','S07-5','S08-5');
