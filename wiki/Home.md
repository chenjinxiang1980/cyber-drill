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

当前版本：**1.0.3**。版本号唯一来源是仓库根目录 `VERSION`。

1.0.3 内容：新建演练项目时按所选剧本自动勾选参演角色；角色口径为剧本**全部节点**目标角色去重（含混淆节点），「是否已有账号」仅统计**启用状态**账号。本次无数据库迁移，累计 **80 条**，1.0.2 → 1.0.3 迁移增量 0 条。1.0.2 的剧本流程与封存校验整改（0124–0127、角色增至 20 个）见 [Release v1.0.2（GitHub）](https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v1.0.2) ｜ [Gitee 发布页](https://gitee.com/habeas/cyber-drill/releases)。

运行环境：**优先 openEuler 24.03 LTS-SP1**（已在实机验证）；**统信 UOS Server 20** 为兼容可选项（同为 dnf 系，Docker 引擎需按现场仓库或离线 RPM 提供）。

## 获取帮助的顺序

1. 先看本 Wiki 的 [常见问题](FAQ)；
2. 再看 `docs/` 下对应主题文档；
3. 仍未解决时，附上 `docker compose ps`、`docker logs <项目名>-app`（默认 `drill-app`）的关键片段与版本号反馈。
