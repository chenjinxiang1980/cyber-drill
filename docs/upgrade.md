# 升级指南（容器部署）

> **迁移链的适用范围**：迁移链只服务本指南描述的“已部署实例升级”路径。
> 全新安装恢复基线快照，不重放迁移链；新增迁移在基线快照同步前**不会**进入全新安装。
> 口径与发布检查见 [install-docker-openeuler.md 第 0.1 节](install-docker-openeuler.md#01-迁移与安装的正式口径定稿)。

> 适用范围：用 `docker compose` 部署 Drill 的用户。
> 源码自行编译、只想升级数据库的用户看 [从源码构建与升级数据库](build-from-source.md)。
> 全新服务器从零安装（Docker / openEuler / 离线镜像 tar）见 [一键安装（Docker / openEuler）](install-docker-openeuler.md)。
> 工具的内部设计（阶段、清单格式、失败分支）见 [一键升级工具设计](upgrade-tool-design.md)。

一条命令完成 **备份 → 数据库迁移 → 拉取镜像 → 重建 app → 校验**；失败自动回滚：

```bash
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0
```

---

## 1. 运行前检查

| 项 | 要求 | 怎么确认 |
|---|---|---|
| 操作系统 | Linux（脚本用 bash、flock、sha256sum、gzip、awk）；**优先 openEuler 24.03 LTS-SP1**，统信 UOS Server 20 兼容可选 | `bash --version` |
| Docker | 20.10+ | `docker version` |
| Compose | **v2.20 及以上**（脚本用 `docker compose config --format json`） | `docker compose version --short` |
| Python | 3.x（清单是 JSON，用 python3 解析，不依赖 jq） | `python3 -V` |
| 权限 | 能执行 docker、能读写部署目录 | 通常用 `sudo` 跑 |
| 部署目录 | 含 `docker-compose.yml`、`.env`，且 `app`、`mysql` 两个服务已创建 | `docker compose ps` |
| 磁盘 | 空闲 ≥ 清单里该版本要求的 `min_free_bytes`（0.8.9+ 为 2 GiB） | `df -h <部署目录>` |

`.env` 里升级工具会用到这些变量（缺了会在 P0 直接报错退出，不会动任何东西）：

| 变量 | 用途 | 必需 |
|---|---|---|
| `DB_NAME` | 要备份/迁移的库，默认 `drill_system` | 建议显式写 |
| `DB_USER` / `DB_PASSWORD` | 迁移用（需要有建表/改表/写入权限） | 是 |
| `DB_ROOT_USER` | 整库备份/恢复用，默认 `root` | 否 |
| `DB_ROOT_PASSWORD` 或 `MYSQL_ROOT_PASSWORD` | 整库备份/恢复用 | **是** |
| `DB_HOST` | 容器内用 `mysql`；`.env` 里写 `127.0.0.1` 也没关系，工具会取 app 容器里的值 | 否 |

> 口令只写进临时的 0600 客户端配置并用 `docker cp` 放进 mysql 容器，不进命令行、不进日志。
> `drill-migrate` 启动时会按服务端配置校验 `JWT_SECRET` / `SESSION_SECRET`，所以 `--env-file` 请指向部署用的 `.env`（里面本来就有这两项），不要自己拼一个只有 DB_* 的文件。

---

## 2. 先看一眼：状态与空跑

```bash
# 当前版本、最新迁移、最近一次升级结果、可用备份数
sudo ./deploy/upgrade/upgrade.sh --status

# 只做检查并打印计划，不做任何变更（推荐每次升级前先跑）
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --dry-run
```

`--dry-run` 会打印：部署目录、compose、当前/目标镜像、**逐跳迁移链**（每跳迁移条数、DDL、是否兼容旧二进制）、是否停机、备份与日志落点。检查不通过会以非 0 退出并说明下一步该做什么。

---

## 3. 常用命令

```bash
# 1) 最常用：只给目标版本，当前版本自动探测（容器 /api/health → schema_migrations 行数）
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0

# 2) 显式指定起止（批量升级、或 health 不可用时）
sudo ./deploy/upgrade/upgrade.sh --from 0.8.6 --to 1.0.0

# 3) 空跑
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --dry-run

# 4) 非交互（脚本化 / 批量）
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --yes

# 5) 内网无外网：提前 docker save 好镜像包
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --offline --image-tar /opt/pkgs/cyber-drill-1.0.0.tar

# 6) 从指定仓库拉（默认 ghcr）
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --registry swr

# 7) 部署目录不在默认位置
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --dir /opt/drill

# 8) 自动回滚也没成功时的人工补救入口
sudo ./deploy/upgrade/upgrade.sh --recover-last
```

`--dir` 不写时按顺序找：脚本上一级的 `docker-dist/`、当前目录、脚本所在目录——**不确定就显式写 `--dir`**。

---

## 4. 参数表

| 参数 | 默认 | 说明 |
|---|---|---|
| `--to <ver>` | 必填 | 目标版本，`x.y.z` |
| `--from <ver>` | 自动探测 | 当前版本。**工具会相信你写的值**，写错会导致迁移区间判断错误 |
| `--dir <path>` | 自动查找 | 部署目录（含 `docker-compose.yml`） |
| `--compose <file>` | `<dir>/docker-compose.yml` | compose 文件 |
| `--env-file <file>` | `<dir>/.env` | 环境变量文件 |
| `--registry ghcr\|swr` | — | 显式换仓库：把 image 改写成清单里的该仓库地址（不带则保留 compose 原有仓库路径，只换 tag） |
| `--image-ref <ref>` | 按清单生成 | 直接指定目标镜像（本地构建/自建仓库） |
| `--offline` | 关 | 不访问 registry，要求本地已有镜像或配 `--image-tar` |
| `--image-tar <file>` | — | `docker load -i` 的镜像包；导入后 tag 与目标不符会自动打 tag |
| `--pin-digest` | 关 | 目标写成 `repo:ver@sha256:…`；清单没有该版本 digest 时只告警 |
| `--dry-run` | 关 | 只检查 + 打印计划 |
| `--yes` | 关 | 跳过交互确认（非交互环境必须加） |
| `--skip-backup` | 关 | 复用已有同版本、sha256 校验通过的备份；**必须与 `--yes` 同用** |
| `--fast` | 关 | 跳过备份的「恢复演练」（默认真的把 dump 导入临时库验一遍） |
| `--keep-backups <n>` | 3 | 保留最近 n 份**成功**备份；`0` 不清理 |
| `--timeout <sec>` | 180 | 等待新版本 healthy 的上限 |
| `--log-dir <path>` | `<dir>/logs` | 日志目录 |
| `--allow-internal` | 关 | 允许目标为内部迭代版（`channel=internal`，如 0.8.7） |
| `--allow-ahead` | 关 | 库已超前于 `--from` 版本时，若超出的迁移**全部**落在目标版本链的迁移区间内则放行（日志与 `STATE.json` 留痕）；其它库状态异常仍拦截 |
| `--force-chain` | 关 | 链不完整时强行继续，**必须与 `--yes` 同用**；日志留痕 |
| `--status` | — | 只打印当前状态 |
| `--recover-last` | — | 继续失败的回滚（见第 8 节） |

---

## 5. 退出码

| 码 | 含义 | 是否已改数据 |
|---|---|---|
| 0 | 成功；或本来就是目标版本（打印 noop）；或 `--dry-run` 通过 | 否（除非真升级） |
| 10 | 前置检查失败：docker/compose 版本、compose 解析、缺 `app` 服务、缺 `python3`、缺 root 口令、参数错误、非交互却没加 `--yes` | 否 |
| 11 | 版本链不完整/缺环、目标版本不在清单、库状态与清单不符（落后或超前）、目标为 internal | 否 |
| 12 | 磁盘不足 | 否 |
| 13 | 备份失败或备份校验失败（坏备份会删掉） | 否 |
| 14 | 迁移失败、compose 改写失败、重建 app 失败 → **已自动回滚** | 已回滚 |
| 15 | 新版本健康检查超时 → **已自动回滚** | 已回滚 |
| 16 | 升级后 health 版本号或 `brand_version` 不符 → **已自动回滚** | 已回滚 |
| 20 | **自动回滚也失败**，需要人工介入（终端会打印手工步骤） | 需人工 |
| 30 | 已有另一个升级进程持锁 | 否 |
| 40 | 目标镜像缺失、拉取失败、digest 不符、镜像内迁移数量与清单不符 | 否 |

---

## 6. 三个阶段最常出问题的地方

### 6.1 P0 预检（退出码 10/11/12/40）

工具在**做任何变更之前**完成：抢锁、compose 解析、探测当前版本、版本链校验、库状态对比、磁盘、目标镜像引用计算。

- 「版本链不完整，已停止」→ 说明升级工具太旧、清单里没有中间版本。**升级 `deploy/upgrade/` 目录到最新**（脚本 + `manifest.json` 要一起换），不要直接 `--force-chain`。
- 「数据库落后于当前版本」→ 上次启动迁移没跑完。先让当前版本正常启动一次（entrypoint 会自动补迁移），确认 `schema_migrations` 行数后再升级。
- 「数据库超前于当前版本」→ 分两种：
  - 工具会先逐条列出超出的迁移名及归属版本区间。**若这些迁移全部属于目标版本链**（常见于运维提前用目标镜像跑过迁移、或上次升级中途停过），确认后加 `--allow-ahead` 重跑即可，工具会在 P3 用目标镜像补齐剩余迁移；
  - 若超出的迁移里有**目标版本未覆盖**的条目，说明库结构比目标镜像新，任何参数都不放行，只能改用包含这些迁移的更新版本，或恢复升级前备份（第 7 节）。

### 6.2 P3 迁移（退出码 14）

工具用**目标镜像**起一次性容器执行迁移，绕开 entrypoint（不会等 MySQL、不会 seed）：

```bash
docker run --rm --network <app所在网络> --env-file <env文件> -e DB_HOST=<容器内DB主机> \
  --entrypoint /opt/drill/bin/drill-migrate <目标镜像> -root=/opt/drill up
```

失败会：停 app → 还原 compose → **整库恢复** → 起旧版 → 校验旧版 health 与迁移行数。日志里能直接看到失败的那条迁移文件名与前后的 `SELECT`/`UPDATE`。

### 6.3 P4/P5 换镜像与校验（退出码 14/15/16）

只重建 `app` 一个服务（`up -d --no-deps app`），`mysql` 不会被重建。校验项：

- `/api/health` 的 `version` == 目标版本；
- `settings.brand_version` == `v<目标版本>`（本版本迁移增量为 0 时跳过该项，仅打印实际值）；
- `schema_migrations` 行数 == 清单累计条数，末条 == 清单 `through`；
- 容器 `RestartCount`、最近 200 行日志里的 `PANIC/FATAL/[ERROR]` —— **只告警，不作为回滚依据**。

---

## 7. 备份、恢复演练与保留

每次升级在 `<部署目录>/backups/upgrade-<from>-to-<to>-<时间戳>/` 下产生：

| 文件 | 内容 |
|---|---|
| `db.sql.gz` | 整库 dump（`--single-transaction --quick --routines --triggers --events`） |
| `env.bak` | 升级前的 `.env`（0600） |
| `compose.bak` | 升级前的 `docker-compose.yml` |
| `META.json` | 源/目标版本、库名、排序规则、迁移行数、旧镜像引用与 id |
| `SHA256SUMS` | 上面文件的校验和 |
| `RESULT.json` | 本次结果（成功后才写入；保留策略只删「成功且过期」的备份） |

- 目录 `0700`、文件 `0600`。
- **默认做恢复演练**：把 dump 导入临时库 `drill_upgrade_verify_<时间戳>_<pid>`，比对表数与 `schema_migrations` 行数后删除临时库。慢机器/磁盘紧张可加 `--fast` 跳过（备份会被标记为未演练）。
- 默认保留最近 **3** 份成功备份；**失败或回滚过的备份永不自动删除**；`--keep-backups 0` 关闭清理。
- 校验备份：`cd <备份目录> && sha256sum -c SHA256SUMS`。

> **整库恢复会覆盖备份之后写入的业务数据。** 回滚是为了把系统拉回可用状态，不是无损时间机器。

手工恢复（自动回滚也失败时）：

```bash
# 1) 还原 compose
cp -p <备份目录>/compose.bak <部署目录>/docker-compose.yml
# 2) 整库恢复
docker cp <备份目录>/db.sql.gz <mysql容器>:/tmp/restore.sql.gz
docker exec <mysql容器> sh -c 'gunzip -c /tmp/restore.sql.gz | mysql -uroot -p "$DB_NAME"'
# 3) 起旧版
cd <部署目录> && docker compose up -d --no-deps app
# 4) 校验
docker exec <app容器> wget -qO- http://127.0.0.1:8090/api/health
```

---

## 8. 断点续跑、锁与人工补救

- **断点续跑**：每次升级一个目录 `<部署目录>/.upgrade-runs/<from>-to-<to>-<时间戳>/`，里面 `STATE.json` 每完成一个阶段写一次。中断后用**同样的 `--to`** 重跑即可：已完成的备份会通过 sha256 校验后复用，不重复备份；迁移靠 `schema_migrations` 天然去重。
- 如果上次死在回滚中途（`status=rolling_back`），重跑会**先把回滚做完**，不会继续往前走。
- **并发锁**：`<部署目录>/.upgrade.lock` + `flock -n`。另一个升级在跑时直接退出 30，并打印持锁信息。
- **`--recover-last`**：找出 `rolling_back` / `rollback_failed` 的记录，用记录里的备份继续回滚。它不重新升级。

---

## 9. 镜像三种写法都能用

工具**不强行改你的写法**，只把 tag 换成目标版本：

| 你的 compose | 目标引用怎么生成 | 需要做什么 |
|---|---|---|
| `image: cyber-drill:1.0.0`（本地 tag） | `cyber-drill:<to>`，只查本地 | 本地必须先有目标镜像（`docker load` 或自建）；否则加 `--registry`/`--image-tar` |
| `image: ghcr.io/org/cyber-drill:1.0.0` | `<原仓库地址>:<to>`（**只换 tag，保留你自己的仓库路径**，私有镜像站也能用） | 能访问 registry |
| `image: ghcr.io/org/cyber-drill:1.0.0@sha256:…` | 同写法 + 清单里该版本的 digest | 清单没有目标 digest 时**拒绝**，提示改回 tag 写法 |

`--registry swr`（或 `ghcr`）是**换仓库**：会把 image 改写成清单里的该仓库地址，用于从官方仓库/镜像站拉取；不写就只在原地址上换 tag。

- `--pin-digest`：把纯 tag 写法升级成 `tag@digest`；清单没有 digest 时只告警，按 tag 继续。
- **digest 校验**：从 registry 拉下来的镜像会比对 `RepoDigests`，不符直接拒绝（退出 40）。本地构建/`docker load` 导入的镜像没有 `RepoDigests`，无法比对，会标记 `UNVERIFIED` 后继续，由健康检查兜底。
- 历史版本（0.5.0/0.5.1/0.6.0/0.8.3）没有记录 digest，清单里是 `null`，只能用 tag + 健康检查校验。

---

## 10. 停机与已知限制

- **0.9.0 → 1.0.0**：迁移只改数据、向后兼容，app 不提前停；停机窗口仅为重建容器的数秒。
- **0.8.9 → 0.9.0（历史）**：迁移只改数据、向后兼容，app 不提前停；停机窗口仅为重建容器的数秒。
- **0.8.6 → 1.0.0 等跨版本**：链上 0.8.7 含 DDL，工具会**先停 app 再迁移**，停机时长≈迁移耗时。`--dry-run` 会提前告诉你。
- 入口版本 0.5.0/0.5.1/0.6.0 标了 `best_effort`：那时没有发布校验数据，升级后请人工抽查业务页面。
- `app` 如果 bind mount 了宿主机目录到 `/opt/drill/public`，前端静态资源不会随镜像更新。工具只**告警**，不会去改你的挂载，也不会删你改过的静态文件。
- 升级只重建 `app`；`mysql`、数据卷、网络一律不动（`drill-migrate` 也在一次性容器里跑，不落卷）。

---

## 11. 相关文档

- [从源码构建与升级数据库](build-from-source.md)：版本注入、`GOOS` 交叉编译、`drill-migrate up` 三步、`drill-upgrade` 用法
- [一键升级工具设计](upgrade-tool-design.md)：清单格式、链算法、阶段表、失败分支全表
- [Linux 部署](linux-deployment.md)：首次安装与日常运维
