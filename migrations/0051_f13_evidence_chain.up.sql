-- 0051 F13 证据保管链与逐条明细（P1-07 表单侧）
-- 依据：评审 P1-07 —— F13《证据保全清单》一张表只有一条证据名称、一个哈希值，
-- 无法覆盖一个节点的多条证据（日志、样本、内存镜像、截图、流量包），
-- 也缺少保管人、封存介质与移交记录。本迁移只新增字段并改提示文案，
-- 保留原有 8 个字段与选项（历史提交按 label 回显），不删除任何已被引用的选项。
-- evidence 表的 sha256/custodian/sealed_at/chain_of_custody 与写入链路在取证/备份线，
-- 本轮不改（见 0051 说明与文档「被约束卡住的条目」）。
-- 纯 DML，固定终值，可重复执行。

UPDATE form_templates
   SET fields = '[{"label":"证据名称","hint":"逐项填写保全的证据名称，如堡垒机操作日志、内存镜像。","required":true,"type":"text"},{"label":"证据类型","hint":"选择证据的类型。","required":true,"type":"select","options":["日志","样本文件","内存镜像","截图","流量包","其他"]},{"label":"来源对象","hint":"证据来源的系统、主机、账号或设备。","required":true,"type":"text"},{"label":"保全时间","hint":"完成证据固定的时间，精确到分钟。","required":true,"type":"datetime"},{"label":"存放位置","hint":"证据的存放介质、目录或平台位置。","required":true,"type":"text"},{"label":"哈希值","hint":"单条证据的 SHA256 或 MD5 值，用于完整性校验；同一节点有多条证据时，请在「证据明细（逐条）」中逐行填写。","required":true,"type":"text"},{"label":"保全人","hint":"执行证据保全的人员姓名与岗位。","required":true,"type":"text"},{"label":"完整性校验","hint":"保全后是否完成哈希复核。","required":true,"type":"select","options":["通过","不通过","未校验"]},{"label":"证据编号","hint":"院内证据编号（如 EVD-2026-001），便于与证据归档一一对应。","required":false,"type":"text"},{"label":"证据明细（逐条）","hint":"一条证据一行，格式：编号｜名称｜类型｜来源｜保全时间｜存放位置｜SHA-256｜保管人。一个节点有多条证据时逐行填写，不只写一条汇总。","required":true,"type":"textarea"},{"label":"保管人","hint":"负责保管证据的人员与岗位；与保全人不同时，注明移交时间。","required":true,"type":"text"},{"label":"移交记录","hint":"记录证据移交的时间、移交人、接收人、用途与签收方式；无移交可留空。","required":false,"type":"textarea"},{"label":"哈希算法","hint":"优先使用 SHA-256；MD5 仅用于历史材料兼容。","required":true,"type":"select","options":["SHA-256","SHA-1","MD5"]}]'
 WHERE id = 'F13';
