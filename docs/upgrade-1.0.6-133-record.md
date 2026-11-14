# 133 现网 cyber-drill：1.0.5 → 1.0.6 升级留档

- 执行日期：2026-11-12（CST）
- 操作窗口：17:08 备份 → 17:08:39 app 重建启动 → 17:15 收尾复核
- 结论：**升级成功，未触发回滚**；现网 `drill` 栈运行 1.0.6，`mysql` 未重建
- 登录验证结论：**用户人工验证通过**（多角色账号会出现身份选择且两身份权限不同；单角色账号不出现身份选择）

---

## 1. 升级前后对照

| 项目 | 升级前（1.0.5） | 升级后（1.0.6） |
|---|---|---|
| compose `app` image | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill@sha256:8d24611e4d176177e613dc5dde317991bd47f56c63f29104097dbd747b7a0c04`（仅 digest，无 tag） | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.6@sha256:4b04fa1ea82076d7c359e9fa5a60fd386aab76606de62aa3d63ddde65581295c` |
| 运行镜像 ID | `sha256:69ca1db60b25245ef1af71c4a5c49ca285bbd4c05440ae9d29497ba5f971c80c` | `sha256:58ce99557a21d8a36ce3860da079ce7d9f3975d2f72ca06823c8798465ea5d51` |
| manifest digest | `sha256:8d24611e4d17…7a0c04` | `sha256:4b04fa1ea82076d7c359e9fa5a60fd386aab76606de62aa3d63ddde65581295c` |
| 容器创建时间 | `2026-11-11T15:25:05Z` | `2026-11-12T09:08:39Z` |
| 容器启动时间 | `2026-11-12T07:28:27Z` | `2026-11-12T09:08:39Z` |
| 健康状态 | `healthy` | `healthy`（重建后约 16s 转 healthy） |
| 重启次数 | 0 | 0 |
| `/api/health` version | `1.0.5` | `1.0.6` |
| 页脚 | `Cyber-Drill-System 1.0.5 …` | `Cyber-Drill-System 1.0.6 · © 2026 陈金祥` |
| `schema_migrations` | 88 条，末条 `0136_task_proxy_dispatch` | 88 条，末条 `0136_task_proxy_dispatch`（无新增迁移） |
| `drill-mysql` | `mysql:8.0`，ID `sha256:6cd09145…`，启动 `2026-11-12T07:28:27Z` | **完全未动**：同 ID、同启动时间、restarts=0 |

## 2. 备份

备份目录：`/opt/cyber-drill/backups/pre-1.0.6-20261112-170822/`

| 文件 | 大小 | 说明 |
|---|---|---|
| `db.sql.gz` | 290,859 B | 整库逻辑备份；`gzip -t` 通过；sha256 `e174518939c317cfbcfc36babe68545b8f940fe2426b4489a9356d5eba4b3152` |
| `docker-compose.yml.bak` | 2,085 B | 改前 compose（digest 固定 1.0.5） |
| `docker-compose.yml.switch-origin` | 2,085 B | 切换前同一份原件的第二副本 |
| `drill-app-inspect-before.json` | 16,934 B | 改前 `drill-app` 完整 inspect |
| `drill-mysql-inspect-before.json` | 13,769 B | 改前 `drill-mysql` 完整 inspect |
| `docker-ps-before.txt` | 191 B | 改前容器状态 |
| `image-1.0.5-digest.json` / `image-1.0.5-local.json` | 8,111 B ×2 | 回滚镜像的 digest 与本地记录 |

备份完整性：解压后 39 张表、23 条 `INSERT`、dump 尾标记完整。

回滚镜像保留（未删除）：`sha256:69ca1db60b25245ef1af71c4a5c49ca285bbd4c05440ae9d29497ba5f971c80c`，标签 `ghcr.io/…:1.0.5`、`swr…:1.0.5`、`cyber-drill:1.0.5-7bc1555` 均指向它。另有历史本地构建 `cyber-drill:1.0.5` = `sha256:0dc5445…`（1.0.5 早期构建，非本次回滚目标）。

## 3. 回滚命令

### 3.1 仅回滚应用（首选，`mysql` 不动）

```sh
cd /opt/cyber-drill
cp docker-compose.yml docker-compose.yml.rollback-$(date +%Y%m%d-%H%M%S)

# 把 app 的 image 换回 1.0.5 digest
sed -i 's#image: swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.6@sha256:4b04fa1ea82076d7c359e9fa5a60fd386aab76606de62aa3d63ddde65581295c#image: swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.5@sha256:8d24611e4d176177e613dc5dde317991bd47f56c63f29104097dbd747b7a0c04#' docker-compose.yml

docker compose -p drill config -q          # 语法自检
docker compose -p drill up -d --no-deps app
docker inspect drill-app --format '{{.Image}} {{.State.Health.Status}} {{.RestartCount}}'
curl -s http://127.0.0.1:8090/api/health
```

要求：镜像 ID 回到 `sha256:69ca1db60b25…`，`/api/health` 报 `1.0.5`。**不要执行 `docker compose down`**，否则 `mysql` 也会被停掉。

### 3.2 需要连带恢复数据库时

```sh
cd /opt/cyber-drill

# 1) 先停 app，避免恢复期间有写入
docker compose -p drill stop app

# 2) 恢复整库（口令从 .env 读取，不进 argv）
set -a; . ./.env; set +a
gunzip -c backups/pre-1.0.6-20261112-170822/db.sql.gz \
  | docker exec -i -e MYSQL_PWD="${MYSQL_ROOT_PASSWORD}" drill-mysql \
      mysql -uroot "${DB_NAME}"

# 3) 按 3.1 回滚 app 镜像后启动
docker compose -p drill up -d --no-deps app
```

恢复后复核：`schema_migrations` 88 条 / 末条 `0136_task_proxy_dispatch`、`P2026-001` 24 条任务、`tasks.is_proxy=1` 3 条。

## 4. 升级后验证（实际输出）

| 检查项 | 实际输出 |
|---|---|
| `Config.Image` | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.6@sha256:4b04fa1ea82076d7c359e9fa5a60fd386aab76606de62aa3d63ddde65581295c` |
| 运行镜像 ID | `sha256:58ce99557a21d8a36ce3860da079ce7d9f3975d2f72ca06823c8798465ea5d51` |
| 容器健康 | `HEALTH=healthy RESTARTS=0 STARTED=2026-11-12T09:08:39Z` |
| `/api/health` | `{"ok":true,"service":"cyber-drill-system","time":"2026-11-12 17:15:32","version":"1.0.6"}` |
| 页脚 | `Cyber-Drill-System 1.0.6 · © 2026 陈金祥`；`COPYRIGHT_WORD=absent`（无软著字样） |
| CSP | `CSP_HEADER_HASHES=17 INLINE=1 MISS=0`（页内 1 条内联脚本哈希在响应头中命中） |
| 验证码 | `HTTP=200`、`SVG_BYTES=1875`、`<svg` 合法、生产环境无 `testCode` |
| `schema_migrations` | `migrations_total=88`、`migrations_last=0136_task_proxy_dispatch` |
| `P2026-001` 任务 | `tasks_P2026-001=24` |
| `is_proxy=1` | `tasks_is_proxy_1=3` |
| `drill-mysql` | 同 ID、`STARTED=2026-11-12T07:28:27Z`、`RESTARTS=0`（未重建） |
| compose 语法 | `docker compose -p drill config -q` → `CONFIG_OK` |
| 两步登录（多角色/单角色） | **用户人工验证通过** |

## 5. 约束与未做事项

- 未打 git tag、未创建 Release、未上传 Release 资产
- 未再推送镜像（1.0.6 在发版阶段已推送 GHCR 与 SWR，digest 一致）
- 未修改任何账号口令；未创建 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD`
- 临时凭据文件 `/opt/cyber-drill/.logincheck.env` **从未创建**（收尾时确认 `CRED_FILE_ABSENT`）；临时验证脚本已从 133 `/tmp` 删除
