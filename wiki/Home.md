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

当前版本：**1.0.5**。版本号唯一来源是仓库根目录 `VERSION`。

1.0.5 内容：删除退役剧本 S20「数据加密勒索」，并把原 S21「数据权限」改编号为 S20（剧本表 20 行、编号 S01–S20 连续、角色 18 个）；新增「未参演角色任务代填」——项目存在未勾选的参演角色时，任务下发/开始演练会把这些节点自动转交信息科负责人代填，工作台标记「代填·角色」并全程留痕。本次携带 3 条迁移（0133 删除 S20、0134 S21→S20、0136 代填字段与回填），累计 **88 条**，末条 `0136_task_proxy_dispatch`。1.0.4 的自动勾选参演角色见 [Release v1.0.4（GitHub）](https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v1.0.4) ｜ [Gitee 发布页](https://gitee.com/habeas/cyber-drill/releases)。

运行环境：**优先 openEuler 24.03 LTS-SP1**（已在实机验证）；**统信 UOS Server 20** 为兼容可选项（同为 dnf 系，Docker 引擎需按现场仓库或离线 RPM 提供）。

## 获取帮助的顺序

1. 先看本 Wiki 的 [常见问题](FAQ)；
2. 再看 `docs/` 下对应主题文档；
3. 仍未解决时，附上 `docker compose ps`、`docker logs <项目名>-app`（默认 `drill-app`）的关键片段与版本号反馈。
