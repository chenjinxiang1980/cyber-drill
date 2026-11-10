# Cyber-Drill-System Wiki（草稿）

> 本目录是 Wiki 草稿，**尚未推送到 GitHub / Gitee Wiki**。正式发布前请按 `docs/pre-release-checklist-1.0.0.md` 的顺序执行。

## 快速导航

- [安装](Installation)
- [升级](Upgrade)
- [配置项](Configuration)
- [常见问题](FAQ)
- [评分与剧本口径](Scoring-and-Scenarios)
- [备份与恢复](Backup-and-Restore)

## 系统定位

Cyber-Drill-System（应急演练指挥系统）是面向医院、政府与企事业单位内网的桌面推演 + 实战演练指挥调度平台。后端为 Go 单体服务，数据库为 MySQL 8.0，前端为静态页面，由服务端直接托管。

当前版本：**1.0.0**。版本号唯一来源是仓库根目录 `VERSION`。

## 获取帮助的顺序

1. 先看本 Wiki 的 [常见问题](FAQ)；
2. 再看 `docs/` 下对应主题文档；
3. 仍未解决时，附上 `docker compose ps`、`docker logs drill-app` 的关键片段与版本号反馈。
