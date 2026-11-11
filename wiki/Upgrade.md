# 升级

容器部署统一使用 `deploy/upgrade/upgrade.sh`。升级流程：备份 → 数据库迁移 → 拉取镜像 → 重建 app → 健康校验；失败自动回滚。

## 查看状态与预演

```bash
sudo bash deploy/upgrade/upgrade.sh --status
sudo bash deploy/upgrade/upgrade.sh --to 1.0.2 --dry-run
```

## 执行升级

```bash
sudo bash deploy/upgrade/upgrade.sh --to 1.0.2
```

可显式指定来源版本：

```bash
sudo bash deploy/upgrade/upgrade.sh --from 1.0.1 --to 1.0.2
```

## 升级矩阵（摘要）

| 路径 | 迁移条数 | 含 DDL | 说明 |
|---|---|---|---|
| 1.0.1 → 1.0.2 | 4 | 否 | 仅数据修正（剧本节点/目标角色），工具自动备份后迁移，失败回滚 |
| 1.0.0 → 1.0.2 | 5 | 否 | 同上，含 1.0.1 的 0 迁移补丁 |
| 1.0.0 → 1.0.1 | 0 | 否 | 无数据库迁移，停机窗口仅为重建容器的数秒 |
| 0.9.0 → 1.0.0 | 1 | 否 | 停机窗口仅为重建容器的数秒 |
| 0.8.6 → 1.0.0 | 16 | 是 | 需按备份 → 迁移 → 校验流程 |
| 0.8.3 → 1.0.0 | 29 | 是 | 同上 |
| 0.5.0 → 1.0.0 | 76 | 是 | best effort，建议先整库备份 |

完整版本链见 `deploy/upgrade/manifest.json` 与 `docs/upgrade.md`。

> 1.0.2 携带 4 条数据修正迁移（0124–0127，累计 80 条）：升级后 `schema_migrations` 末条为 `0127_script_target_roles`，角色总数由 15 增至 20，`settings.brand_version` 仍保持 `v1.0.0`（沿用 1.0.1 起读 `/api/health` 的 `version` 口径，页面显示 **v1.0.2**）。升级工具以 `/api/health` 的 `version` 判定成功。

## 回滚

- 升级工具在迁移失败时自动回滚；
- 含 DDL 的迁移**不提供逐条 down 脚本**，回滚方式是整库恢复到升级前备份；
- 整库恢复会丢失备份时点之后写入的数据，并可能使已删除数据重新出现。

## 升级前检查

1. 确认已配置 `BACKUP_PASSPHRASE`，备份可正常生成；
2. 确认磁盘空间足够存放备份与新旧镜像；
3. 记录当前版本（`/api/health` 或 `--status`）；
4. 避开演练进行中时段。
