# 0.9.0 之后的一键升级工具 —— 设计方案

> 状态：**待评审**（本文只做设计，不含实现；评审通过后再落地）
> 适用对象：容器部署（docker compose）用户为主，源码自行编译的高级用户为辅
> 目标：用户只指定「从哪个版本 → 到哪个版本」，工具自动完成 **备份 → 数据库迁移 → 拉取镜像 → 重建容器 → 校验**；回滚只在升级失败时自动触发。
> 约束：本阶段不动现网；实现后在 VM 上用克隆库 + 0.8.9 / 0.9.0 两个镜像演练。

---

## 0. 结论摘要

| 决策点 | 结论 |
|---|---|
| 工具形态 | `deploy/upgrade/upgrade.sh`（纯 bash，除 python3 外无额外依赖）为主入口；另出 `drill-upgrade` 独立二进制给源码编译用户 |
| 版本清单 | `deploy/upgrade/manifest.json`，一个版本一条记录，链式 `previous` 指针；只加一条即可接入新版本 |
| 迁移时机 | **先用新镜像起临时容器迁移，再换成新镜像跑 app**（不会出现「新二进制 + 旧库」；「新库 + 旧二进制」窗口压到秒级） |
| 含 DDL 的版本 | 工具自动改为「停 app → 迁移 → 起 app」，并在开始时明确告知停机时长 |
| 回滚 | 仅失败自动触发：换回旧镜像 + 恢复整库（无 down 迁移，DDL 后只能整库恢复） |
| 幂等 | 每次升级一个 run 目录 + `STATE.json`；重跑复用已完成的阶段，迁移靠 `schema_migrations` 天然去重 |
| 安全 | 口令走 `--defaults-extra-file`（0600）/ `MYSQL_PWD`，不出现在命令行与日志；备份 0600 / 目录 0700 |
| compose 兼容 | 只改 `app` 服务的 `image:` 行，保留用户原有风格（本地 tag / registry tag / digest），改完先 `docker compose config -q` 校验 |

---

## 1. 现状与约束（设计前提）

| 事实 | 对设计的影响 |
|---|---|
| 现网 compose 里 `image: cyber-drill:0.9.0`（本地镜像 tag，非 registry digest） | 工具必须能改写任意形式的 image ref，且默认**保持用户原来的风格** |
| 容器 entrypoint 启动时会自动 `drill-migrate up` + `drill-seed` | 迁移可以在 app 容器内完成，但那样「新二进制 + 旧库」的失败窗口不可控；本设计改为**迁移前置到临时容器** |
| 迁移只有 `.up.sql`，**没有 down** | 回滚只能恢复整库；DDL 之后尤其如此 |
| MySQL 8 的 DDL 会隐式提交 | 含 DDL 的迁移失败会留下「半套结构」，因此这类版本默认停服迁移 |
| 迁移器已有 `GET_LOCK` 互斥 + `INSERT ... ON DUPLICATE KEY` 幂等 | 重复执行安全，重跑时自动 SKIP 已应用项 |
| 镜像内已带 `migrations/`（至 0.9.0 共 75 条）与 `mariadb-client`（含 mysqldump） | 迁移与备份都能在容器里完成，宿主机不需要装 MySQL 客户端 |
| 生产机存在 `public/` 的 bind mount（运维侧做法） | 工具需检测并**警告**「前端随镜像更新不会生效」，不擅自改挂载 |

---

## 2. 目录结构

```
repo/
├── deploy/
│   └── upgrade/
│       ├── upgrade.sh            # 主脚本（容器部署一键升级）
│       ├── manifest.json         # 版本清单（唯一事实来源，随镜像一起发布）
│       ├── lib/
│       │   ├── 00-common.sh      # 日志、锁、错误码、脱敏
│       │   ├── 10-manifest.sh    # 清单读取与链校验（jq 缺失时退化为 python3/内置解析）
│       │   ├── 20-backup.sh      # 备份、校验、保留策略
│       │   ├── 30-migrate.sh     # 临时容器迁移
│       │   ├── 40-compose.sh     # image 改写、recreate、健康检查
│       │   └── 50-rollback.sh    # 自动回滚
│       └── README.md             # 给运维的三屏速查（命令 + 失败码）
├── server/
│   └── cmd/
│       └── upgrade/              # 高级用户用的独立二进制（go:embed 同一份 manifest）
│           └── main.go
├── docs/
│   ├── upgrade-tool-design.md    # 本文（设计）
│   ├── upgrade.md                # 落地后：用户手册（普通用户）
│   └── build-from-source.md      # 落地后：源码编译 + 仅升级数据库
└── migrations/                   # 不变
```

落地后 `docs/linux-deployment.md` 顶部加一行指到 `docs/upgrade.md`，避免两套说法。

---

## 3. 版本清单（manifest）

### 3.1 结构

```jsonc
{
  "schema_version": 1,
  "updated_at": "2026-11-10",
  "default_registry": "ghcr",
  "registries": {
    "ghcr": "ghcr.io/chenjinxiang1980/cyber-drill",
    "swr":  "swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill"
  },
  "releases": [ /* 见 3.2，按 version 升序 */ ]
}
```

单条 release 的字段：

| 字段 | 类型 | 必填 | 含义 |
|---|---|---|---|
| `version` | string | 是 | 不带 `v` 的语义化版本，如 `0.9.0` |
| `previous` | string\|null | 是 | **链上前一个版本**；`null` 表示链起点（`0.8.3`） |
| `channel` | `stable`\|`internal` | 是 | `internal` 不在用户可选列表里，但仍可作跳板 |
| `published_at` | date | 是 | 发布日期 |
| `image.tags` | string[] | 是 | 该版本发布的 tag，如 `["0.9.0","latest"]` |
| `image.digest` | object | 是 | `{"ghcr":"sha256:…","swr":"sha256:…"}`；历史版本缺失时用 `null` + `"digest_unknown": true` |
| `image.size_bytes` / `layers` | int | 是 | 用于磁盘预估与日志 |
| `migrations.from_exclusive` | string | 是 | 上一版本 `through` 的名字（等于 `previous` 的 `through`） |
| `migrations.through` | string | 是 | 该版本应用到的最后一条迁移 |
| `migrations.count` | int | 是 | 区间内 `.up.sql` 条数（用于校验） |
| `migrations.contains_ddl` | bool | 是 | 是否含 DDL（决定是否默认停服迁移） |
| `migrations.backward_compatible` | bool | 是 | 迁移后旧二进制能否继续跑（决定能否不停服迁移） |
| `schema_migrations_total` | int | 是 | 升级到该版本后 `schema_migrations` 应有的行数 |
| `min_free_bytes` | int | 是 | 升级前磁盘硬下限（备份 + 镜像 + 余量） |
| `health.path` / `health.expect_version` / `health.timeout_seconds` | | 是 | 升级后校验口径 |
| `actions` | array | 否 | 中间版本需要的人工/自动动作（如环境变量改名、口令轮换），见 3.4 |
| `notes_url` | string | 否 | Release 链接 |

### 3.2 样例（覆盖 0.8.3 → 0.9.0 的真实历史）

```json
{
  "schema_version": 1,
  "updated_at": "2026-11-10",
  "default_registry": "ghcr",
  "registries": {
    "ghcr": "ghcr.io/chenjinxiang1980/cyber-drill",
    "swr": "swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill"
  },
  "releases": [
    {
      "version": "0.8.3",
      "previous": null,
      "channel": "stable",
      "published_at": "2026-11-06",
      "image": { "tags": ["0.8.3"], "digest": null, "digest_unknown": true, "size_bytes": 0, "layers": 0 },
      "migrations": { "from_exclusive": "0000_bootstrap", "through": "0090_distractor_rewrite_v2", "count": 47, "contains_ddl": true, "backward_compatible": true },
      "schema_migrations_total": 47,
      "min_free_bytes": 1610612736,
      "health": { "path": "/api/health", "expect_version": "0.8.3", "timeout_seconds": 90 },
      "notes_url": "https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v0.8.3"
    },
    {
      "version": "0.8.6",
      "previous": "0.8.3",
      "channel": "stable",
      "published_at": "2026-11-08",
      "image": {
        "tags": ["0.8.6"],
        "digest": { "ghcr": "sha256:cde759a249a9147b8308e004eb9b89b91256ed58fc5a7a3107d73acfa9f75dbf",
                    "swr":  "sha256:cde759a249a9147b8308e004eb9b89b91256ed58fc5a7a3107d73acfa9f75dbf" },
        "size_bytes": 79733766, "layers": 10
      },
      "migrations": { "from_exclusive": "0090_distractor_rewrite_v2", "through": "0104_brand_version_086", "count": 13, "contains_ddl": true, "backward_compatible": true },
      "schema_migrations_total": 60,
      "min_free_bytes": 1610612736,
      "health": { "path": "/api/health", "expect_version": "0.8.6", "timeout_seconds": 90 },
      "notes_url": "https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v0.8.6"
    },
    {
      "version": "0.8.7",
      "previous": "0.8.6",
      "channel": "internal",
      "published_at": "2026-11-08",
      "image": { "tags": ["0.8.7"], "digest": null, "digest_unknown": true, "size_bytes": 0, "layers": 0 },
      "migrations": { "from_exclusive": "0104_brand_version_086", "through": "0109_brand_version_087", "count": 5, "contains_ddl": false, "backward_compatible": true },
      "schema_migrations_total": 65,
      "min_free_bytes": 1610612736,
      "health": { "path": "/api/health", "expect_version": "0.8.7", "timeout_seconds": 90 }
    },
    {
      "version": "0.8.8",
      "previous": "0.8.7",
      "channel": "stable",
      "published_at": "2026-11-09",
      "image": { "tags": ["0.8.8"], "digest": null, "digest_unknown": true, "size_bytes": 0, "layers": 0 },
      "migrations": { "from_exclusive": "0109_brand_version_087", "through": "0114_brand_version_088", "count": 5, "contains_ddl": false, "backward_compatible": true },
      "schema_migrations_total": 70,
      "min_free_bytes": 1610612736,
      "health": { "path": "/api/health", "expect_version": "0.8.8", "timeout_seconds": 90 }
    },
    {
      "version": "0.8.9",
      "previous": "0.8.8",
      "channel": "stable",
      "published_at": "2026-11-09",
      "image": { "tags": ["0.8.9"], "digest": null, "digest_unknown": true, "size_bytes": 194000000, "layers": 9 },
      "migrations": { "from_exclusive": "0114_brand_version_088", "through": "0118_brand_version_089", "count": 1, "contains_ddl": false, "backward_compatible": true },
      "schema_migrations_total": 71,
      "min_free_bytes": 2147483648,
      "health": { "path": "/api/health", "expect_version": "0.8.9", "timeout_seconds": 90 }
    },
    {
      "version": "0.9.0",
      "previous": "0.8.9",
      "channel": "stable",
      "published_at": "2026-11-09",
      "image": {
        "tags": ["0.9.0", "latest"],
        "digest": { "ghcr": "sha256:105e638cf1cdf6de4b37e5bac3d33fccc57d6ce47d2362bf44adcf61a192c395",
                    "swr":  "sha256:105e638cf1cdf6de4b37e5bac3d33fccc57d6ce47d2362bf44adcf61a192c395" },
        "size_bytes": 194741696, "layers": 9
      },
      "migrations": { "from_exclusive": "0118_brand_version_089", "through": "0122_brand_version_090", "count": 4, "contains_ddl": false, "backward_compatible": true },
      "schema_migrations_total": 75,
      "min_free_bytes": 2147483648,
      "health": { "path": "/api/health", "expect_version": "0.9.0", "timeout_seconds": 90 },
      "notes_url": "https://github.com/chenjinxiang1980/cyber-drill/releases/tag/v0.9.0"
    }
  ]
}
```

> 说明：`0.8.7` 是内部迭代版本（品牌迁移 `0109`），标记 `channel=internal`，用户下拉里不出现，但跨版本链要靠它连通。
> 历史版本的 digest 在发布当时没有落盘（`0.8.6`、`0.9.0` 有），因此允许 `digest: null + digest_unknown: true`；工具在这种情况下退回「tag 校验 + 健康检查版本号」，并在输出里明确「该版本无 digest 可校验」。

### 3.3 新增一个版本（未来 1.0.0）

只做两件事：

1. 在 `releases` 末尾追加一条（`previous` 指向上一个发布版本）；
2. 如果是新发布的镜像，把两个 registry 的 digest 填上。

清单只在发布流程里改，工具不读远端、不联网发现新版本（内网可离线）。

### 3.4 `actions`（中间版本的动作）

多级跳时，某些中间版本除了迁移还有额外动作（改环境变量名、轮换密钥、文件权限等）。用声明式列表表达，工具按顺序执行并记录：

```jsonc
"actions": [
  { "type": "env_rename", "from": "OLD_KEY", "to": "NEW_KEY" },
  { "type": "env_require", "key": "PUBLIC_BASE_URL" },
  { "type": "warn", "text": "1.0.0 起 TLS 证书路径变更，升级后请核对 .env" }
]
```

首版只实现 `warn` 与 `env_require`（只读检查、不写文件），其余类型在真正出现需求时再加——避免过度设计。

---

## 4. 跨版本跳升

### 4.1 为什么用「链」而不是「任意跳」

数据库迁移本身是累积的：目标镜像里已经包含截止该版本的全部 `.up.sql`，所以 **0.8.6 → 0.9.0 只需要跑一次迁移**，不需要中间版本的镜像。但：

- 中间版本可能有 `actions`（配置项变更），这些不能被跳过；
- 必须有办法在动手前判断「我手上的清单足不足以支撑这次跳」。

因此清单用 `previous` 连成链，跳升 = 沿链收集 `actions` + 只跑目标镜像的迁移。

### 4.2 算法

```
resolve_from():            # 当前版本
  1) --from 指定 → 直接用
  2) 读运行中容器的 /api/health.version
  3) 读数据库 schema_migrations 的行数 → 反查 manifest 里 schema_migrations_total 相等的版本
  4) 都拿不到 → 报错退出（不猜）

build_chain(from, to):
  cur = to; chain = []
  while cur != from:
      e = manifest[cur]
      if e 不存在            → CHAIN_MISSING(cur)         # 清单里没有这个版本
      if e.previous == null  → CHAIN_BROKEN(cur)          # 链在 cur 处断了
      chain.prepend(e); cur = e.previous
      if len(chain) > 50     → CHAIN_CYCLE
  # 连续性校验：每一跳的 from_exclusive 必须等于上一跳的 through
  for i in 1..len(chain)-1:
      if chain[i].migrations.from_exclusive != chain[i-1].migrations.through
          → CHAIN_GAP(chain[i-1].version, chain[i].version)
```

三个校验都通过才继续；任一失败，**在动任何东西之前**退出。

### 4.3 缺环报错长什么样

```
[ERROR] 版本链不完整，已停止（未做任何变更）
        请求路径：0.8.6 → 0.9.0
        缺失环节：0.8.7、0.8.8
        清单内已知版本：0.8.3、0.8.6、0.9.0
        可用做法：
          1) 升级工具到最新（新版清单可能已补齐中间版本）
          2) 直接跳到清单内最近的更高版本：0.9.0（当前已在链上，但 0.8.6→0.9.0 需要 0.8.7/0.8.8）
          3) 联系支持，人工确认后使用 --force-chain（仅用于已知安全的场景，日志会留痕）
```

补充校验：

- 数据库实际 `schema_migrations` 行数必须等于 `from.schema_migrations_total`，否则报 `DB_STATE_MISMATCH`（数据库落后 → 提示先用当前镜像补跑迁移；数据库超前 → 拒绝，防止降级）；
- 目标版本必须 `channel=stable`，除非显式 `--allow-internal`。

---

## 5. 命令形态

### 5.1 常用

```bash
# 1) 最常用：只给目标版本，自动探测当前版本
sudo ./deploy/upgrade/upgrade.sh --to 0.9.0

# 2) 显式指定起止（多台机器批量执行时用）
sudo ./deploy/upgrade/upgrade.sh --from 0.8.6 --to 0.9.0

# 3) 空跑：只做检查、打印计划，不改任何东西
sudo ./deploy/upgrade/upgrade.sh --to 0.9.0 --dry-run

# 4) 内网无外网：用提前拷贝进来的镜像包
sudo ./deploy/upgrade/upgrade.sh --to 0.9.0 --offline --image-tar /opt/pkgs/cyber-drill-0.9.0.tar

# 5) 选仓库（默认 ghcr）
sudo ./deploy/upgrade/upgrade.sh --to 0.9.0 --registry swr

# 6) 非交互（自动化 / 批量）
sudo ./deploy/upgrade/upgrade.sh --to 0.9.0 --yes

# 7) 当前状态（版本、链、最近一次升级结果、可用备份）
sudo ./deploy/upgrade/upgrade.sh --status

# 8) 自动回滚也失败时的人工补救入口（不是正常流程的一部分）
sudo ./deploy/upgrade/upgrade.sh --recover-last
```

### 5.2 参数表

| 参数 | 默认 | 说明 |
|---|---|---|
| `--to <ver>` | 必填 | 目标版本 |
| `--from <ver>` | 自动探测 | 当前版本 |
| `--dir <path>` | 脚本所在项目的部署目录 | 含 `docker-compose.yml` 与 `.env` |
| `--compose <file>` | `<dir>/docker-compose.yml` | 指定 compose 文件 |
| `--env-file <file>` | 从 compose 里读 | 环境变量文件 |
| `--registry ghcr\|swr` | `ghcr` | 拉取来源 |
| `--image-ref <ref>` | 由 manifest 生成 | 直接指定镜像引用（本地构建场景） |
| `--offline` | 关 | 不访问 registry；必须配 `--image-tar` 或用本地已有镜像 |
| `--image-tar <file>` | — | `docker load -i` 的镜像包 |
| `--pin-digest` | 关 | 写入 `repo:ver@sha256:…` 形式 |
| `--dry-run` | 关 | 只检查与打印计划 |
| `--yes` | 关 | 跳过交互确认 |
| `--skip-backup` | 关 | **仅**在已有同版本备份时允许，且必须 `--yes` |
| `--fast` | 关 | 跳过备份的「恢复演练」校验（默认做） |
| `--keep-backups <n>` | 3 | 保留最近 n 份成功升级的备份 |
| `--timeout <sec>` | 180 | 健康检查等待上限 |
| `--log-dir <path>` | `<dir>/logs` | 日志落盘目录 |
| `--allow-internal` | 关 | 允许目标为 `channel=internal` |
| `--force-chain` | 关 | 链不完整时强行继续（需二次确认，日志留痕） |

### 5.3 退出码

| 码 | 含义 |
|---|---|
| 0 | 升级成功 |
| 10 | 前置检查失败（docker/compose 版本、compose 解析、目录权限） |
| 11 | 版本链不完整 / 数据库状态与清单不符 |
| 12 | 磁盘不足 |
| 13 | 备份失败或备份校验失败（**未做任何变更**） |
| 14 | 迁移失败（已自动回滚） |
| 15 | 新容器健康检查失败（已自动回滚） |
| 16 | 升级后版本号不符（已自动回滚） |
| 20 | **自动回滚也失败**（需要人工介入，打印备份路径与手工步骤） |
| 30 | 已有另一个升级在执行（锁冲突） |
| 40 | 离线镜像缺失 / 镜像与清单 digest 不符 |

---

## 6. 执行步骤（含「先迁移还是先换镜像」的取舍）

### 6.1 阶段表

| 阶段 | 做什么 | 失败即停 | 日志 |
|---|---|---|---|
| P0 预检 | 抢锁；`docker`/`docker compose` 版本；解析 compose，确认 `app` 服务与 image 写法；探测当前版本；链校验；磁盘检查；DB 连通性；目标镜像可得性（在线 HEAD manifest / 离线检查 tar） | 是，**零变更** | `logs/upgrade-<from>-to-<to>-<ts>.log` |
| P1 备份 | 整库 dump、`.env`、`docker-compose.yml`、当前镜像 id/tag、`META.json`；生成 `SHA256SUMS`；默认做一次「恢复到临时库」的可用性演练 | 是，零变更 | 同上 |
| P2 取镜像 | `docker pull` 或 `docker load -i`；校验 digest（清单有则比对）；校验镜像内 `migrations/*.up.sql` 数量 == 清单该版本 `schema_migrations_total` | 是，零变更 | 同上 |
| P3 迁移 | 用**新镜像**起一次性容器跑 `drill-migrate -root=/opt/drill up`；含 DDL 或 `backward_compatible=false` 时先停 app | 是 → 进入回滚（若 app 已停先起旧版） | 同上 |
| P4 换镜像 | 原子改写 compose 的 `image:`；`docker compose config -q`；`docker compose up -d --no-deps app`（**只重建 app**） | 是 → 回滚 | 同上 |
| P5 校验 | `/api/health.version`、`settings.brand_version`、`schema_migrations` 行数与末条、容器 `RestartCount`、日志无 `ERROR/PANIC/FATAL` | 是 → 回滚 | 同上 |
| P6 收尾 | 写 `RESULT.json`；清理过期备份（保留 n 份）；打印摘要 | 否（只告警） | 同上 |

### 6.2 为什么是「先迁移，后换镜像」

| 顺序 | 窗口 | 失败后果 |
|---|---|---|
| **A. 先换镜像，entrypoint 内迁移**（当前做法） | 「新二进制 + 旧库」从容器启动到迁移完成（秒级到分钟级，取决于迁移量） | 迁移失败 → 新容器起不来 → **服务不可用**，且库可能半套结构 |
| **B. 先迁移（临时容器），再换镜像**（本设计） | 「新库 + 旧二进制」从迁移成功到 recreate 完成（通常 < 10s） | 迁移失败 → **旧版仍在线**，直接回滚备份 |
| C. 先停服，再迁移，再换镜像 | 零窗口 | 必然停机（迁移时长） |

选 **B**，并加两条硬约束来保证 B 的安全：

1. **迁移必须向后兼容**（manifest 的 `backward_compatible`）：只增表/列/行、不删不改语义、不改索引语义。窗口内旧二进制照常工作。
   - 本版 `0119`–`0122` 全部满足：`0119`–`0121` 是纯 `INSERT`，`0122` 只 `UPDATE settings`。
2. **含 DDL 或显式声明不兼容的版本，自动退化为 C**：manifest 里 `contains_ddl=true` 或 `backward_compatible=false` 时，工具先 `docker compose stop app` 再迁移，迁移完成立刻起新版；停机时长在 P0 就告知用户。

> 关键点：**永远不会出现「新二进制 + 旧库」**——新二进制只在迁移成功、且 `schema_migrations` 达到目标总数之后才启动；而「新库 + 旧二进制」被向后兼容约束限制为可运行。

### 6.3 临时容器怎么起

```bash
docker run --rm \
  --network "<compose项目名>_drill-net" \
  --env-file "<env-file>" \
  -e DB_HOST=mysql \
  --entrypoint /opt/drill/bin/drill-migrate \
  "<目标镜像>" -root=/opt/drill up
```

- 只挂网络和环境变量，**不挂**数据卷（迁移只写库）；
- `--entrypoint` 绕开 `docker-entrypoint.sh`，避免先等 MySQL 再跑 seed；
- 退出码 0 且输出「APPLY ... OK」才继续；
- 迁移前把 `schema_migrations` 的行数写进日志（可与目标 `schema_migrations_total` 对比）。

### 6.4 校验口径（P5）

| 项 | 期望 |
|---|---|
| `/api/health` | HTTP 200，`version == 目标版本` |
| `settings.brand_version` | `v<目标版本>`（本版本迁移增量为 0 时跳过） |
| `schema_migrations` | 行数 == `schema_migrations_total`，末条 == `migrations.through` |
| 容器 | `RestartCount == 0`、`State.Health.Status == healthy` |
| 日志（最近 200 行） | 无 `PANIC` / `FATAL` / `[ERROR]`（**仅告警，不作为回滚依据**——业务日志里也有 ERROR 字样） |
| compose | `app` 服务 image 已是新引用，`mysql` 服务**未被重建** |

---

## 7. 备份与自动回滚

### 7.1 备份哪些对象

| 对象 | 形式 | 说明 |
|---|---|---|
| 数据库 | `db.sql.gz`（`mysqldump --single-transaction --routines --triggers --events --default-character-set=utf8mb4`） | 整库，含 `schema_migrations` |
| 环境文件 | `env.bak` | 原样拷贝，0600 |
| compose | `compose.bak` | 原样拷贝，便于人工比对 |
| 当前镜像 | `META.json` 里记 `image_id` / `repo_digests` / `tag` | 不导出镜像文件（旧镜像留在本地；如已被清理，提示用 registry 回拉旧 tag） |
| 元信息 | `META.json` + `SHA256SUMS` | 版本、digest、迁移条数、开始时间、执行用户 |

目录：`<deploy-dir>/backups/upgrade-<from>-to-<to>-<YYYYmmdd-HHMMSS>/`，权限 `0700`，文件 `0600`。

### 7.2 备份校验（默认开）

1. `gzip -t db.sql.gz`；
2. `sha256sum -c SHA256SUMS`；
3. **恢复演练**：把 dump 恢复进临时库 `drill_upgrade_verify_<ts>`，核对表数量与 `schema_migrations` 行数，然后 `DROP DATABASE`。
   - 这一步是「备份真的能用」的唯一证据；`--fast` 可跳过，但日志会标 `UNVERIFIED`。

### 7.3 自动回滚

**触发条件**（任一）：P3 迁移失败 / P4 recreate 失败 / P5 健康检查超时 / P5 版本号不符。

**执行顺序**（顺序不能反）：

```
1. docker compose stop app                      # 停掉可能半启动的新容器
2. 若 P4 已改 compose → 还原 compose.bak（或把 image 行改回旧引用）
3. 若 P3 已产生任何迁移（schema_migrations 行数 > 备份时行数）
      → 恢复整库：drop + create + 导入 db.sql.gz
   （DDL 已隐式提交，没有 down 脚本，唯一可靠的回滚就是整库恢复）
4. docker compose up -d --no-deps app           # 起旧镜像
5. 校验旧版本：/api/health.version == from，schema_migrations 行数 == 备份时行数
6. 写 RESULT.json（status=rolled_back），退出码 14/15/16
```

**回滚失败**（退出码 20）：保持 app 停止（不再乱试），打印：

- 备份目录绝对路径、`SHA256SUMS` 校验命令；
- 手工恢复三条命令（compose 还原、整库恢复、起容器）；
- 日志路径。

**必须写在文档里的风险**：

> 迁移一旦执行，**没有 down 脚本**。若迁移含 DDL，MySQL 已隐式提交，工具只能恢复整库——**备份之后产生的业务数据会丢失**。因此迁移窗口要选在业务低峰，且 `db.sql.gz` 的完整性必须在升级前校验通过（工具默认做恢复演练）。

---

## 8. 幂等与可重入

### 8.1 状态文件

每次升级一个 run 目录，`STATE.json` 记录阶段与产物：

```json
{
  "run_id": "20261110-0130-0.8.9-to-0.9.0",
  "from": "0.8.9", "to": "0.9.0",
  "phase": "migrate",
  "done": ["precheck", "backup", "image"],
  "artifacts": { "dump": "db.sql.gz", "dump_db_rows": 74, "old_image_id": "1e3e6094b6f2" },
  "started_at": "...", "updated_at": "..."
}
```

### 8.2 重跑规则

| 中断位置 | 重跑行为 |
|---|---|
| P0/P1 中途 | 复用同 run 目录；已有且校验通过的 dump 直接复用（不重复备份）；未完成的删除重做 |
| P2 中途 | 重新 `docker load` / `pull`（幂等） |
| P3 迁移中途 | `drill-migrate up` 依据 `schema_migrations` **SKIP 已应用项**，从断点继续；工具先打印「已应用 n / 目标 m」 |
| P4 中途 | compose 已是新引用则跳过改写；`up -d --no-deps app` 幂等 |
| P5 中途 | 只重跑校验 |
| 回滚中途 | `--recover-last` 从 `STATE.json` 的 `rollback` 阶段继续，不重复恢复已恢复的库 |

### 8.3 并发与锁

- `flock` 锁文件：`<deploy-dir>/.upgrade.lock`；拿不到 → 退出码 30，打印持锁 PID 与开始时间；
- 迁移器自身还有 MySQL `GET_LOCK('drill_schema_migrations')`，双保险；
- 锁文件不随进程异常退出残留（`flock` 随 fd 释放），无需「陈旧锁清理」。

---

## 9. 安全

| 项 | 做法 |
|---|---|
| 口令来源 | 从 compose 的 `env_file` / `--env-file` 读，**不写命令行参数**；`mysqldump` 用 `--defaults-extra-file=<0600临时文件>`（用完删），或 `MYSQL_PWD` 环境变量 |
| 口令落日志 | 统一日志函数对 `PASSWORD` / `SECRET` / `TOKEN` 关键字做 `***` 替换；`set +x`，禁止 `bash -x` |
| 备份权限 | 目录 `0700`、文件 `0600`，属主为执行用户（root）；`SHA256SUMS` 同权限 |
| 临时文件 | `mktemp -d` 于备份目录内，`trap` 清理（含异常退出） |
| 旧备份清理 | 默认保留最近 **3** 份**成功**升级的备份；**失败/回滚过的备份永不自动删**；`--keep-backups 0` 关闭清理 |
| 镜像来源 | 清单里有 digest 就必须比对；不符直接拒绝（退出码 40），不提供忽略开关 |
| 命令注入 | 所有变量加引号；版本号白名单正则 `^[0-9]+\.[0-9]+\.[0-9]+$`；`--image-ref` 校验 `^[a-zA-Z0-9._/:@-]+$` |

---

## 10. 高级用户路径（源码自编译）

### 10.1 只升级数据库的工具形态

**推荐：独立二进制 `drill-upgrade`**（不复用 `drill-migrate` 的原因：`drill-migrate` 只认目录里的 SQL，不认识「版本 → 区间」的映射；把清单逻辑塞进去会让迁移器职责变混）。

```
server/cmd/upgrade/main.go        # //go:generate cp ../../../deploy/upgrade/manifest.json manifest.json + //go:embed manifest.json
```

子命令：

```bash
# 只升级数据库（不碰容器）。默认先备份再迁移。
./drill-upgrade db --to 0.9.0 --env-file /opt/drill/etc/drill.env --backup-dir /opt/drill/backups

# 只检查：库当前版本、链是否完整、需要跑哪几条迁移（只读）
./drill-upgrade verify --to 0.9.0 --env-file /opt/drill/etc/drill.env

# 用镜像内的迁移器（容器部署、但想自己控制顺序时）
docker run --rm --network drill-net --env-file .env -e DB_HOST=mysql \
  --entrypoint /opt/drill/bin/drill-migrate cyber-drill:0.9.0 -root=/opt/drill up
```

`drill-upgrade db` 与 `upgrade.sh` 共用同一份 manifest 与同一套备份/校验逻辑（Go 实现一份，脚本调用二进制时走 `--json` 输出）。

### 10.2 编译说明落点

新增 `docs/build-from-source.md`，「落地」时写，内容固定为这几步（与当前发布流程一致）：

```bash
# 0) 依赖：Go 1.22+、make 可无、CGO 关闭
cd server
# 1) 版本注入（版本号、构建时间、提交号）
VERSION=0.9.0
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT=$(git rev-parse --short HEAD)
LDFLAGS="-X drill-system/internal/version.Version=${VERSION} \
         -X drill-system/internal/version.BuildTime=${BUILD_TIME} \
         -X drill-system/internal/version.GitCommit=${GIT_COMMIT}"
# 2) 交叉编译（server 不带 -s -w；migrate/seed 带 -s -w）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o ../dist/drill-server .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${LDFLAGS}" -o ../dist/drill-migrate ./cmd/migrate
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${LDFLAGS}" -o ../dist/drill-seed    ./cmd/seed
# 3) 只升级数据库
../dist/drill-migrate -root=.. up        # 或 ./drill-upgrade db --to 0.9.0 --env-file ...
# 4) 自建镜像（可选）
docker build -f deploy/docker-dist/Dockerfile.runtime -t cyber-drill:0.9.0 .
```

文档里同时写清：`public/` 是纯静态、`migrations/` 必须整目录带齐、`-root` 指向含 `.env` 与 `migrations/` 的目录。

---

## 11. 兼容现状：compose 里 image 的改写

### 11.1 现网写法（本地 tag）

```yaml
services:
  app:
    image: cyber-drill:0.9.0
```

工具的处理：

1. 解析当前 ref → `repo=cyber-drill`、`tag=0.9.0`、`registry=无`；
2. 目标 ref 生成规则：
   - 当前 ref **无 registry 前缀** → 目标也保持无前缀：`cyber-drill:0.9.1`，并要求镜像已存在于本地（`--image-tar` 或事先 `docker load`）；否则报 `IMAGE_NOT_FOUND` 并提示加 `--registry` 或 `--image-ref`；
   - 当前 ref 有 registry 前缀 → 用 `--registry`（默认 ghcr）拼 `ghcr.io/.../cyber-drill:<ver>`；
3. 只在 `app` 服务下替换 `image:` 行，其余内容（`build:`、`environment:`、`volumes:`）**不动**。

### 11.2 将来改成 digest 固定

```yaml
image: ghcr.io/chenjinxiang1980/cyber-drill:0.9.0@sha256:105e638c…
```

- 检测到 ref 带 `@sha256:` → 目标 ref 自动带上清单里的目标 digest；
- 反向兼容：`--pin-digest` 可把纯 tag 写法升级成 `tag@digest`；
- 若用户写了 `@sha256:` 但清单没有该版本 digest（历史版本）→ 拒绝并提示用 tag 形式。

### 11.3 改写实现

无 `yq` 依赖。做法：

1. 用 `docker compose config --format json` 读出 `services.app.image`（只读，不落盘）；
2. 用「按服务定位 + 精确匹配 image 行」的 `awk`/`sed` 替换，生成 `docker-compose.yml.new`；
3. `docker compose -f docker-compose.yml.new config -q` 校验；
4. 校验通过 → `mv` 覆盖（同目录同一文件系统，原子）；
5. 任何一步失败 → 删除 `.new`，原文件保持不动。

> 备选方案（待裁决）：直接依赖 `python3 -c` 做 YAML 解析改写。**建议不用**——目标机不一定有 pyyaml，而 awk 定点替换 + `compose config` 校验已经足够安全。

### 11.4 bind mount 的 `public/`

若 `app` 服务存在把宿主机目录挂到 `/opt/drill/public` 的 bind mount：

- 工具**不改挂载**；
- 升级完成后打印警告：「检测到 public 为 bind mount，本次前端更新未生效；如需同步，执行 `rsync -a --delete <镜像内 public 备份>/ <宿主机目录>/` 并保持 755/644」；
- manifest 里不预置自动同步，避免误删用户自定义静态文件。

---

## 12. 失败分支表

| # | 场景 | 判定依据 | 工具动作 | 退出码 | 用户补救 |
|---|---|---|---|---|---|
| 1 | 无 docker / compose 版本过低 | `docker compose version` < v2.20 | 退出，零变更 | 10 | 升级 Docker |
| 2 | compose 解析失败 / 无 `app` 服务 | `compose config` 报错 | 退出 | 10 | 修 compose |
| 3 | 版本链缺环 | 4.2 算法 | 退出，打印缺哪一环 | 11 | 升级工具 / `--force-chain` |
| 4 | 库状态与清单不符（落后） | `schema_migrations` 行数 < `from.total` | 退出，提示先补迁移 | 11 | 用当前镜像跑一次 `migrate up` |
| 5 | 库超前（要降级） | 行数 > `from.total` | 拒绝 | 11 | 不支持降级；需恢复备份 |
| 6 | 磁盘不足 | 可用空间 < `min_free_bytes` 且 < dump×1.5+镜像×3 | 退出 | 12 | 清理旧备份/镜像 |
| 7 | dump 失败 | `mysqldump` 非 0 | 退出，零变更 | 13 | 查库连通/权限 |
| 8 | 备份校验失败 | `gzip -t` / `sha256sum -c` / 恢复演练失败 | 删除坏备份，退出 | 13 | 查磁盘/内存，重跑 |
| 9 | 镜像拉取失败 | `docker pull` 非 0 | 退出，零变更 | 40 | 换 `--registry` / 用 `--image-tar` |
| 10 | digest 不符 | 拉取后 `RepoDigests` ≠ 清单 | 拒绝使用该镜像 | 40 | 检查仓库/清单 |
| 11 | 迁移失败（DML） | migrate 非 0，事务已回滚 | 回滚：起旧版 + 整库恢复 | 14 | 看日志里失败的迁移文件名 |
| 12 | 迁移失败（DDL，半套结构） | 同上 + 含 DDL | 回滚：**必须**整库恢复（已完成） | 14 | 同上，需修正迁移后重发 |
| 13 | recreate 失败 | `compose up` 非 0 | 还原 compose → 恢复整库 → 起旧版 | 14 | 查端口占用/镜像损坏 |
| 14 | 健康检查超时 | 180s 内未 healthy | 自动回滚 | 15 | 看新容器日志 |
| 15 | 版本号不符 | `health.version != to` | 自动回滚 | 16 | 查镜像 tag 是否错配 |
| 16 | 回滚失败 | 恢复或起旧版失败 | 停止 app，打印手工步骤 | 20 | 按输出手工恢复 |
| 17 | 重复执行 | 锁被占 | 退出 | 30 | 等前一个跑完 |
| 18 | 中断后重跑 | `STATE.json` 存在 | 从断点继续 | 0 | — |

---

## 13. 拿不准的地方（请裁决）

| # | 选项 | 建议 | 理由 |
|---|---|---|---|
| 1 | 主脚本纯 bash / 改成 Go 二进制 | **bash** | 用户机上跑未知二进制心理成本高；bash 可读可改，便于内网审计 |
| 2 | 迁移时机 A/B/C（见 6.2） | **B + 条件退 C** | 唯一不会出现「新二进制 + 旧库」的组合，且失败时旧版仍在线 |
| 3 | 备份恢复演练默认开还是关 | **默认开**，`--fast` 关 | 备份不能用等于没备份；代价是多花一次导入时间 |
| 4 | 备份保留份数 | **3** | 0.9.0 镜像 186 MiB、整库 dump 通常 < 100 MB，3 份占用可控 |
| 5 | 是否默认写 digest（pin） | **跟随用户原写法**，`--pin-digest` 显式开 | 现网是纯 tag，突然改写法会让运维 diff 很吵 |
| 6 | 0.6.0 这类更老版本是否纳入链 | **纳入**（`previous` 连到 0.8.3） | 一次性把历史补齐，避免以后又断环 |
| 7 | 历史版本 digest 缺失怎么处理 | 允许 `null`，退回 tag+健康检查校验 | 当时没记录，无法追溯；不做假数据 |
| 8 | 是否提供 GUI / 交互菜单 | **不提供** | 命令行 + `--dry-run` 已够；GUI 是另一个维护面 |

---

## 14. 落地计划与验收（设计通过后）

1. 实现 `deploy/upgrade/`（脚本 + manifest）与 `server/cmd/upgrade`；
2. 补 `docs/upgrade.md`、`docs/build-from-source.md`；
3. 在 VM 上演练（**只用克隆库与临时 compose 项目，不碰 `drill-app` / `drill-mysql`**）：

| 用例 | 期望 |
|---|---|
| 0.8.9 → 0.9.0（正常） | 迁移 4 条、单次 recreate、health 0.9.0、退出 0 |
| 0.8.6 → 0.9.0（多级跳） | 链走 0.8.7/0.8.8/0.8.9，迁移 15 条，退出 0 |
| 人为让迁移失败 | 旧版恢复在线、整库恢复、退出 14 |
| 迁移中途 kill -9 后重跑 | 从断点继续，不重复备份、不重复应用 |
| 清单删掉 0.8.8 | 退出 11，报「缺 0.8.8」 |
| `--offline --image-tar` | 不访问网络，成功 |
| compose 为 digest 写法 | 自动带 digest |

4. 演练通过后再更新 Wiki「升级与回滚」页与 Release 说明。

---

## 15. 与现有文档的关系

| 文档 | 变化 |
|---|---|
| `docs/linux-deployment.md` | 顶部加一句「升级请用 `deploy/upgrade/upgrade.sh`，见 `docs/upgrade.md`」 |
| `docs/install.md` | 首次安装流程不变 |
| Wiki「升级与回滚」 | 落地后改写为「一键升级 + 失败回滚」口径，保留手工步骤作为兜底 |

---

## 16. 实现偏差（落地时按「更安全的一侧」修正）

本文是设计规格；落地实现与本文不一致的地方以本节为准，本文正文已同步修正可见的错漏。

| # | 设计原文 | 实际实现 | 原因 / 影响 |
|---|---|---|---|
| 1 | 0.8.8 迁移 `count: 6` | `count: 5`（0110–0114） | 设计样例笔误；累计 70 条不变，链校验不受影响 |
| 2 | P2「读镜像内 `/opt/drill/VERSION` 与清单一致」 | 校验镜像内 `migrations/*.up.sql` 数量 == `schema_migrations_total` | `Dockerfile.runtime` 不生成 `VERSION` 文件；迁移文件数能同样发现 tag 错配 |
| 3 | `//go:embed ../../deploy/upgrade/manifest.json` | `//go:generate cp ../../../deploy/upgrade/manifest.json manifest.json` + `//go:embed manifest.json` | `go:embed` 不能跨包目录向上引用；编译前需 `go generate ./cmd/upgrade` |
| 4 | 「纯 bash，零额外依赖」 | 依赖 `python3` 解析 JSON（不依赖 jq / pyyaml） | 手写 JSON 解析太脆弱；`python3` 在主流发行版默认可用，缺失时 P0 直接报错退出 10 |
| 5 | 口令来源「从 compose 的 env_file 读」 | 读 `--env-file`（默认 `<dir>/.env`），并要求 `DB_ROOT_PASSWORD` 或 `MYSQL_ROOT_PASSWORD` | 整库 dump 需要 root 级权限；缺口令在 P0 退出 10，不做任何变更 |
| 6 | `-e DB_HOST=mysql` 写死 | 取 **app 容器 `Config.Env` 里的 `DB_HOST`**，其次 `.env`，兜底 `mysql` | 用户 `.env` 常写 `DB_HOST=127.0.0.1`（宿主机语义），在容器网络里不可用 |
| 7 | digest 不符一律拒绝 | 从 registry 拉取：`RepoDigests` 不含清单 digest → 拒绝（40）；本地构建 / `docker load` 导入：无 `RepoDigests` → 告警 `UNVERIFIED` 后继续 | 本地 tag 写法本来就没有 `RepoDigests`，硬拒绝会让「本地 tag」这条被裁决支持的路径不可用；能比对时必须比对 |
| 8 | P5「日志无 ERROR/PANIC/FATAL」作为校验项 | 只告警，不触发回滚 | 业务日志也会出现 `[ERROR]`，按它回滚会造成误回滚（更安全的一侧是不回滚） |
| 9 | `--env-file` 默认「从 compose 里读」 | 默认 `<dir>/.env` | compose `env_file` 可能是相对路径且可多个，直接用部署目录的 `.env` 与用户手工执行一致 |
| 10 | `--force-chain`「强行继续」 | 仅跳过清单里缺失的版本，且必须 `--yes`；日志留痕 | 不提供「忽略连续性」的开关，避免把错误清单当成正确链 |
| 11 | 中断后重跑「从断点继续」 | `status=rolling_back` 的记录重跑时**先完成回滚**，不再继续升级 | 回滚中途中断时继续往前走会放大不一致 |
| 12 | 「保留最近 3 份备份」 | 只删 `RESULT.json status=success` 的过期备份；失败/回滚过的永不自动删 | 失败备份是排障证据 |
| 13 | 0.5.0 / 0.5.1 / 0.6.0 纳入链 | 已纳入，`best_effort: true`；升级路径含此类版本时打印告警 | 当时无发布校验数据，无法保证端到端验证 |
| 14 | `drill-upgrade` 未定义退出码 | `0` 成功 / `1` 执行失败 / `2` 参数或版本链错误 | 与 `upgrade.sh` 的 10/11/… 区分：这是「只升库」工具，不涉及容器阶段 |
| 15 | `--skip-backup`「仅在已有同版本备份时允许」 | 必须 `--yes`；按 `META.json.from` 匹配，并用 `sha256sum -c` 校验通过才复用 | 避免复用一个坏备份 |
| 16 | registry 写法「用清单里的仓库地址替换」 | **保留用户 compose 里的仓库路径，只换 tag**；只有显式 `--registry` 才改写成清单里的仓库 | 用户可能用私有镜像站（`registry.corp/drill/cyber-drill`），替换成 ghcr 会直接拉不到 |
| 17 | `--dry-run` 也建运行目录 | 用 `mktemp -d` 临时目录，不写 `.upgrade-runs`、不参与断点续跑 | 空跑不应留下可被复用的「运行中」记录 |
| 18 | 中断后重跑即续跑 | 仅当记录里有备份（`artifacts.backup_dir`）或状态为 `rolling_back` 时才续跑；P0/P1 早期失败按新升级处理 | 没产生任何变更的失败记录续跑只会带来误判 |
| 19 | `--recover-last` 无记录时按失败处理 | 打印「无需处理」并退出 0 | 没有待回滚记录不是故障，返回 20 会误导脚本 |
| 20 | `--fast` 只跳过演练 | 跳过恢复演练，备份仍做 gzip + sha256 校验并标记未演练；`--skip-backup` 允许复用未演练但 sha256 通过的备份 | 演练是为了验证可导入性，sha256 已能保证文件完整 |
| 21 | 备份命令未提 tablespace | 加 `--no-tablespaces` | MySQL 8 下非 root 备份会因缺 `PROCESS` 权限报错 |
| 22 | 迁移由工具内嵌执行 | 由目标镜像里的 `drill-migrate` 子进程执行，它按服务端配置校验 `JWT_SECRET`/`SESSION_SECRET` | 因此 `--env-file` 必须是部署用的 `.env`；`drill-upgrade` 自身不依赖这两项 |
