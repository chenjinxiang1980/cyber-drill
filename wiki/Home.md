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

当前版本：**1.0.1**。版本号唯一来源是仓库根目录 `VERSION`。

1.0.1 修复：系统设置上传 Logo 后无法显示（上传接口未落盘、`brand/` 资源匿名不可读、镜像内 `public/` 子目录权限被降为 0644）、升级工具在「迁移增量为 0」的补丁版本上误判 `brand_version` 并自动回滚，以及页面版本显示改为读 `/api/health` 返回的 `version`（取不到才回退 `settings.brand_version`，补丁版不再需要为此新增迁移）。详见 [Release v1.0.1（GitHub）](https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v1.0.1) ｜ [Gitee 发布页](https://gitee.com/habeas/cyber-drill/releases)。

运行环境：**优先 openEuler 24.03 LTS-SP1**（已在实机验证）；**统信 UOS Server 20** 为兼容可选项（同为 dnf 系，Docker 引擎需按现场仓库或离线 RPM 提供）。

## 获取帮助的顺序

1. 先看本 Wiki 的 [常见问题](FAQ)；
2. 再看 `docs/` 下对应主题文档；
3. 仍未解决时，附上 `docker compose ps`、`docker logs <项目名>-app`（默认 `drill-app`）的关键片段与版本号反馈。
