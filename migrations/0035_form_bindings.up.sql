-- 0035 标准表单绑定（P1-05 / D-03）
-- 把业务兜底、数据补录、恢复验证、舆情口径从通用表 F9 迁移到对应标准表单，
-- 避免 F6/F7/F16/F17 成为孤儿模板。同时按 D-03 把「网信办审核」改为「指导意见」。
-- 纯 DML，固定终值，可重复执行。

UPDATE script_nodes SET form = 'F16' WHERE id = 'S03-9';
UPDATE script_nodes SET form = 'F16' WHERE id = 'S05-8';
UPDATE script_nodes SET form = 'F17' WHERE id = 'S05-13';
UPDATE script_nodes SET form = 'F7'  WHERE id = 'S02-15';
UPDATE script_nodes SET form = 'F6'  WHERE id = 'S06-11';

UPDATE script_nodes
   SET descr = '结合市网信办的指导意见，由院新闻发言人统一对外发布事件口径与进展；网信办负责指导，不承担审核职责。未经院应急指挥部审批不得披露事件细节或患者信息。'
 WHERE id = 'S06-11';
