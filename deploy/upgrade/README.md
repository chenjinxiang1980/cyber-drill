# deploy/upgrade —— 一键升级（速查）

完整说明：[docs/upgrade.md](../../docs/upgrade.md)；设计规格：[docs/upgrade-tool-design.md](../../docs/upgrade-tool-design.md)。
源码自编译用户：[docs/build-from-source.md](../../docs/build-from-source.md)。

## 三条命令

```bash
sudo ./deploy/upgrade/upgrade.sh --status                 # 现在是什么版本、上一次升级结果
sudo ./deploy/upgrade/upgrade.sh --to 1.0.4 --dry-run     # 只检查 + 打印计划，零变更
sudo ./deploy/upgrade/upgrade.sh --to 1.0.4               # 备份 → 迁移 → 换镜像 → 校验；失败自动回滚
```

常用变体：

```bash
--from 0.8.6            # 当前版本探测不准时显式指定
--yes                   # 非交互
--dir /opt/drill        # 部署目录不在默认位置
--registry swr          # 显式换仓库（不带则保留 compose 里的仓库路径，只换 tag）
--offline --image-tar x.tar   # 内网无外网
--fast                  # 跳过备份的恢复演练（慢机器）
--keep-backups 3        # 成功备份保留份数（默认 3；0 = 不清理）
--recover-last          # 自动回滚也失败后，继续回滚
```

## 目录与文件

```
upgrade.sh              主入口（纯 bash）
manifest.json           版本清单（脚本与 drill-upgrade 共用；新增版本只加一条 release）
lib/00-common.sh        日志/出错/锁/STATE.json/脱敏
lib/10-manifest.sh      清单读取与链校验（需要 python3）
lib/20-backup.sh        整库备份、恢复演练、保留策略
lib/30-migrate.sh       用目标镜像的一次性容器跑 drill-migrate
lib/40-compose.sh       compose 解析、image 改写、重建 app、健康检查
lib/50-rollback.sh      自动回滚
```

运行期产生的文件都在**部署目录**下，不写仓库：

```
<部署目录>/.upgrade.lock                     并发锁（flock -n）
<部署目录>/.upgrade-runs/<from>-to-<to>-<ts>/  STATE.json（断点续跑）/ RESULT.json
<部署目录>/backups/upgrade-<from>-to-<to>-<ts>/ db.sql.gz / env.bak / compose.bak / META.json / SHA256SUMS
<部署目录>/logs/upgrade-<from>-to-<to>-<ts>.log
```

## 退出码

| 码 | 含义 | 数据是否已改 |
|---|---|---|
| 0 | 成功 / 已是目标版本 / dry-run 通过 | 否（除非真升级） |
| 10 | 前置检查失败（docker、compose<2.20、compose 解析、缺 python3、缺 root 口令、参数错误、非交互没加 --yes） | 否 |
| 11 | 链不完整/缺环、版本不在清单、库行数与清单不符、目标是 internal | 否 |
| 12 | 磁盘不足 | 否 |
| 13 | 备份或备份校验失败（坏备份已删） | 否 |
| 14 | 迁移/compose 改写/重建 app 失败 → 已回滚 | 已回滚 |
| 15 | 健康检查超时 → 已回滚 | 已回滚 |
| 16 | 版本号或 brand_version 不符 → 已回滚（期望值取清单 `brand_version`，未声明则按 `v<目标版本>`） | 已回滚 |
| 20 | 自动回滚失败，需人工（按终端打印的步骤做） | 需人工 |
| 30 | 已有升级在跑（锁冲突） | 否 |
| 40 | 镜像缺失/拉取失败/digest 不符/镜像内迁移数与清单不符 | 否 |

失败时终端会打印「卡在阶段 / 失败命令 / 原因 / 下一步 / 日志 / 备份」，同一份内容也写进日志文件。

## 依赖

bash、docker 20.10+、docker compose **v2.20+**、python3、flock、sha256sum、gzip、awk。
`.env` 里必须有 `DB_NAME`、`DB_USER`、`DB_PASSWORD`、`DB_ROOT_PASSWORD`（或 `MYSQL_ROOT_PASSWORD`）。

## 新增一个版本

在 `manifest.json` 的 `releases` 末尾追加一条，`previous` 指向上一个版本，`migrations.from_exclusive` 必须等于上一版本 `migrations.through`，`schema_migrations_total` = 上一版本累计 + 本次 `count`。链校验会在 `--dry-run` 阶段就拒绝缺环。
