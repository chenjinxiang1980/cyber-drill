# Cyber-Drill-System Wiki

> 本 Wiki 已发布到 GitHub / Gitee Wiki；仓库内 `wiki/` 目录是唯一源文件，改完需要重新推送。

## 快速导航

- [安装](Installation)
- [升级](Upgrade)
- [配置项](Configuration)
- [常见问题](FAQ)
- [评分与剧本口径](Scoring-and-Scenarios)
- [备份与恢复](Backup-and-Restore)

## 系统定位

Cyber-Drill-System（应急演练指挥系统）是面向医院、政府与企事业单位内网的桌面推演 + 实战演练指挥调度平台。后端为 Go 单体服务，数据库为 MySQL 8.0，前端为静态页面，由服务端直接托管。

当前版本：**1.0.7**。版本号唯一来源是仓库根目录 `VERSION`。

1.0.7 内容：安全加固版本，无功能与数据库变更（累计仍为 **88 条**，末条 `0136_task_proxy_dispatch`）。修复外部单位账号经 `/api/bootstrap` 读取完整剧本答案（`desc/guide/action/wrong`）、内部会商、评分与消息的越权问题；外部角色仅能获取当前演练项目、自身任务节点与已上报材料。修复报告导出、项目切换器、台账/回放流程图的存储型 XSS 注入点；服务端导出的剧本/通知 HTML 增加转义与安全文件名；CSP 增加 `object-src 'none'`、`base-uri 'none'`；安装脚本不再把 MySQL 口令放入进程 argv，并对 docker-compose 下载做 sha256 校验。

1.0.6 内容：登录页恢复「多角色账号选身份」——账号密码校验通过后，若该账号绑定多个角色，先展示身份卡片供选择，再签发登录令牌；单角色账号保持一步登录。身份选择通过一次性票据（`POST /api/auth/login` → `needRole` + 票据，`POST /api/auth/login/role` → token）完成，票据短期有效且用后即废，审计记录所选身份。本次无数据库迁移，累计仍为 **88 条**，末条 `0136_task_proxy_dispatch`。

1.0.5 内容：删除退役剧本 S20「数据加密勒索」，并把原 S21「数据权限」改编号为 S20（剧本表 20 行、编号 S01–S20 连续、角色 18 个）；新增「未参演角色任务代填」——项目存在未勾选的参演角色时，任务下发/开始演练会把这些节点自动转交信息科负责人代填，工作台标记「代填·角色」并全程留痕。本次携带 3 条迁移（0133 删除 S20、0134 S21→S20、0136 代填字段与回填），累计 **88 条**，末条 `0136_task_proxy_dispatch`。1.0.4 的自动勾选参演角色见仓库提交记录与 [升级说明](Upgrade)。

运行环境：**优先 openEuler 24.03 LTS-SP1**（已在实机验证）；**统信 UOS Server 20** 为兼容可选项（同为 dnf 系，Docker 引擎需按现场仓库或离线 RPM 提供）。

## 获取帮助的顺序

1. 先看本 Wiki 的 [常见问题](FAQ)；
2. 再看 `docs/` 下对应主题文档；
3. 仍未解决时，附上 `docker compose ps`、`docker logs <项目名>-app`（默认 `drill-app`）的关键片段与版本号反馈。
