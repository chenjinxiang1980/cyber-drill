-- 0038 干扰项分值梯度与主体纠偏（P1-11 / P1-12）
-- 影响判断的严重错误（不报/瞒报、证据灭失、越权、长期不结束）上调到 8 分；
-- 口头报案/材料不全下调到 5–6 分；同时纠正「就高不就低被判错」等主体混淆表述。
-- 纯 DML，固定终值，可重复执行。

-- 不报案/内部处理：3 -> 8
UPDATE script_nodes SET score=8 WHERE id IN ('DSS02-itlead-12-1','DSS03-itlead-11-1','DSS04-itlead-12-1','DSS05-itlead-12-1');

-- 口头报案、材料不全：8 -> 5（S04/S07/S08 原为 5，保持）
UPDATE script_nodes SET score=5 WHERE id IN ('DSS01-itlead-14-2','DSS02-itlead-12-2','DSS03-itlead-11-2');

-- 证据灭失/未保全即报案：5 -> 8
UPDATE script_nodes
   SET score=8,
       descr='怀疑勒索或人为破坏即向市网安大队报案，但未先固定日志与镜像，证据链缺失导致无法立案。'
 WHERE id='DSS05-itlead-12-2';

-- 全面停机等待勘验：3 -> 8
UPDATE script_nodes SET score=8 WHERE id='DSS01-cyberpolice-15-2';

-- 主体纠偏：把可取的低分行为改为明确的越权/过度动作
UPDATE script_nodes
   SET score=8, title='未核实影响即自行按Ⅰ级处置',
       descr='未核实业务影响与响应等级即自行按Ⅰ级调集全院资源，越权定级并挤占其他应急资源。'
 WHERE id='DSS01-sysadmin-4-2';

UPDATE script_nodes
   SET score=8, title='未核实影响即按Ⅰ级启动全院停诊',
       descr='未做影响评估即按最高等级Ⅰ级启动并全院停诊，扩大停机与上报范围。'
 WHERE id='DSS02-itlead-3-2';

UPDATE script_nodes
   SET score=8, title='无依据长期不结束响应',
       descr='无风险评估、无结束审批长期不宣布结束，响应状态与资源占用持续，复盘无法启动。'
 WHERE id='DSS01-commander-16-1';

UPDATE script_nodes
   SET title='等公安立案后再决定是否报案',
       descr='把公安立案作为报案前置，形成循环等待，延误报案与证据固定。'
 WHERE id='DSS07-itlead-11-2';

UPDATE script_nodes
   SET title='等公安立案后再决定是否上报',
       descr='把公安立案作为对外上报前置，延误 1 小时上报时限。'
 WHERE id='DSS06-itlead-10-2';

UPDATE script_nodes
   SET title='等安全厂商确认传播方式后再通知',
       descr='等待厂商确认病毒传播方式后再发全院预警，预警延迟，终端继续中招。'
 WHERE id='DSS08-itlead-9-2';
