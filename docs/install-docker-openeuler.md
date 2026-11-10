# 一键安装（Docker / openEuler）

> 适用场景：**全新** x86_64 Linux 服务器，从零装出可登录的应急演练系统（Docker + MySQL + 应用）。
> 已在 openEuler 24.03 LTS-SP1 实机验证（验证机地址以 `<测试机 IP>` 代指）。
> 升级已有环境请用 [升级指南](upgrade.md)；源码编译、物理机 systemd 部署见 [Linux 部署指南](linux-deployment.md)。

> **🔒 对外交付必须上 TLS。** 面向非本机访问（局域网/专网/公网）时必须启用 HTTPS：
> 其二选一——应用内 `TLS_ENABLED=true` + 证书（安装器 `--tls-cert/--tls-key`），
> 或在前面套 Nginx/Caddy 反向代理（样例见 [`deploy/linux/nginx-drill.conf`](../deploy/linux/nginx-drill.conf)、[`deploy/linux/Caddyfile.example`](../deploy/linux/Caddyfile.example)）。
> 明文 HTTP 仅供本机联调。备份默认 AES-256-GCM 加密，恢复需要独立口令；详见第 10 节。

脚本：

| 脚本 | 作用 |
|------|------|
| [`deploy/install/build-baseline.sh`](../deploy/install/build-baseline.sh) | 从参照库导出 → 脱敏 → 校验，生成**可重复**的基线快照 |
| [`deploy/install/pack-offline.sh`](../deploy/install/pack-offline.sh) | 打离线包：RPM + compose + 镜像 + **基线快照** + 校验清单 |
| [`deploy/install/install.sh`](../deploy/install/install.sh) | 一键安装：Docker → 镜像 → 空库恢复快照 → 起应用 → 健康检查 |

---

## 0. 为什么全新安装走“基线快照”，而不是空库重放迁移

0.9.0 的空库逐条重放（0001 → 0122）在 `0099_business_batch2` 的前置守卫上失败：

```text
  APPLY 0098_security_file_idx ... OK
  APPLY 0099_business_batch2 ... FAIL: Error 1242 (21000): Subquery returns more than 1 row
```

逐项复算 0099 的断言（132 空库实测）：

| 断言 | 期望 | 空库实测 | 结论 |
|------|------|----------|------|
| `@p16` script_nodes 总数 | 353 | 353 | 一致 |
| `@p17` 干扰项数 | 220 | 220 | 一致 |
| `@p15` `settings.brand_version` | `v0.8.1` | 整行不存在 | 不一致 |
| 其余 p1–p14 | 满足 | 满足 | 一致 |

即：节点数没写错，是**基线数据缺口**——`settings.brand_version` 在现网是历史版本发布时于迁移之外补写的，
迁移链本身从未创建该行。同类“历史手工补录”在后续迁移（0101 一带）还会再次触发同样的断言失败。

因此 0.9.0 起的发布口径是：

- **全新安装 = 恢复基线快照**：一次导入结构 + 参照数据 + 76 条 `schema_migrations`，不做逐条重放。
- **迁移链只服务升级**：`upgrade.sh` 仍然按 `schema_migrations` 从旧版本往上走，行为不变。
- 守卫断言、迁移文件、S01–S08 已封存内容**一律不改**；快照只是把“现网本来就有、迁移里没写”的数据补齐。

---

## 1. 前置条件

| 项 | 要求 |
|----|------|
| 系统 | x86_64 Linux；openEuler 24.03 LTS-SP1 实测通过 |
| 权限 | root（装软件包、改 firewalld、写 /opt） |
| 内存 | ≥ 2 GB（实测 3.3 GB） |
| 磁盘 | 安装目录所在分区 ≥ 3 GB 可用；离线包约 0.7 GB |
| 端口 | 8090（应用）空闲；3306 仅绑定 127.0.0.1 |
| SELinux | 可保持 enforcing（openEuler 的 moby-engine 自带 container-selinux 策略） |
| 软件源 | 离线路径不需要外网；联网路径需要 openEuler 源与 ghcr.io |

---

## 2. 基线快照

### 2.1 内容

| 类别 | 表 |
|------|----|
| 结构 | 全部 39 张表（`--no-data` 导出） |
| 参照数据 | `settings` `roles` `scenarios` `script_nodes` `form_templates` `dict` `knowledge` `score_items` `schema_migrations` |
| **不含**账号 | `users` `user_roles` 只建表、不导数据，**不携带任何现网口令散列** |
| **不含**运营数据 | `projects` `tasks` `form_submissions` `evidence` `audit_logs` `sessions` `messages` `notifs` `impact` `scores` `sim_data` 及上传文件 |

管理员账号由应用启动时的 `drill-seed` 创建：`admin` / `platformadmin`，口令取安装器写入 `.env` 的随机值，
`must_change=1`，**安装输出只打印一次**，首次登录必须改密；改密成功后应用会把 `.env` 里的明文就地清空并标注失效。

### 2.2 可重复生成

```bash
# 1) 从参照库导出（在能连参照库的机器上，只读）
mysqldump -h<host> -u<user> -p --single-transaction --no-tablespaces \
  --no-data drill_system > schema.sql
mysqldump -h<host> -u<user> -p --single-transaction --no-tablespaces \
  --no-create-info --skip-extended-insert --order-by-primary \
  drill_system settings roles scenarios script_nodes form_templates dict knowledge score_items schema_migrations > data.sql

# 2) 脱敏 + 校验 + 打包（-n 关闭 gzip 时间戳，保证同样输入得到同样哈希）
sudo bash deploy/install/build-baseline.sh --raw-sql ./dump.sql --version 1.0.0 --out ./baseline
```

`build-baseline.sh` 的固定动作：

- 去 `DEFINER=`、去 `AUTO_INCREMENT=` 计数器、把 `schema_migrations.initialized_at` 固定为 `1970-01-01 00:00:00`；
- 禁止出现运营表的 `INSERT`、禁止出现 bcrypt 散列（`$2a/$2b/$2y`）；
- 校验：`schema_migrations=76`、`brand_version=v1.0.0`、`scenarios=21`、`form_templates=22`、`dict=12`、
  `dict.eventTypes` 20 项、`script_nodes` 全量 1173 行（可见 1165，8 行软删）；
- 产出 `drill-baseline-<版本>.sql.gz`、`SHA256SUMS`、`manifest.env`（含各项期望值与生成时间）。

1.0.0 发布产物：

```text
baseline/drill-baseline-1.0.0.sql.gz   224K
sha256  2bc77cfe0e45a4397bdf705ab1885a55c39e7b978cb0fd10c915646eb4d02a54
```

同一份 dump 重跑两次得到相同哈希（已验证）；安装器在恢复前会用 `baseline/SHA256SUMS` 校验，
校验值同时写入 `manifest.env`、`/opt/cyber-drill/.restored` 与安装输出。

**基线快照的版本绑定规则（发布必做）**：

- 仓库内的基线固定放在 `deploy/install/baseline/`，文件名带版本号，例如 `drill-baseline-1.0.0.sql.gz`。
- **每个版本发布时必须重新生成一份**，不能复用上一个版本的快照；三项必须同时相等：
  `manifest.env` 的 `BASELINE_VERSION` = 镜像版本 = `deploy/upgrade/manifest.json` 中该版本的 `version`；
  `BASELINE_MIGRATIONS` = 该版本的 `schema_migrations_total`；
  `BASELINE_SHA256` = `SHA256SUMS` 中的哈希 = 实际文件哈希。
- 发布包（离线 `--out` 目录）里的 `baseline/` 必须与镜像同版本；安装器会拒绝“旧快照 + 新镜像”的组合。
- 生成新版本快照后，同步更新 `SHA256SUMS`、`manifest.env` 与本节示例，三者缺一不可。

---

## 3. 快速开始

### 3.1 离线安装（内网首选）

在**一台有网、有 docker、用 dnf 的 Linux** 上打包一次：

```bash
sudo bash deploy/install/pack-offline.sh \
  --out /opt/cyber-drill-offline --version 1.0.0 --baseline-dir ./baseline
```

离线目录结构：

```text
/opt/cyber-drill-offline/
├── rpms/                             # moby-engine、containerd、runc、container-selinux 等及依赖
├── bin/docker-compose                # compose v2 独立二进制
├── images/cyber-drill-1.0.0.tar.gz
├── images/mysql-8.0.tar.gz
├── baseline/drill-baseline-1.0.0.sql.gz
├── baseline/SHA256SUMS
├── baseline/manifest.env
└── SHA256SUMS
```

把整个目录拷到目标机，用仓库里的安装器指向该目录装完（离线包里不含 `install.sh`）：

```bash
sudo bash deploy/install/install.sh --offline-dir /opt/cyber-drill-offline
```

### 3.2 联网安装

```bash
sudo bash deploy/install/install.sh --mode online --version 1.0.0
```

联网路径从 openEuler 源装 `moby-engine`，应用镜像自动探测并选择 GHCR 或华为云 SWR（可用 `--image-source` 指定）；
Docker Hub 不可达时 MySQL 自动回退国内镜像源。
基线快照仍需随包提供（默认在脚本同级 `baseline/`）。

### 3.3 重复执行与已安装检测

安装目录已存在且非空时，安装器**直接停止**（退出码 28），不会覆盖 `.env`、不会覆盖数据库，并给出两条路径：

- A：`sudo bash deploy/install/install.sh --reinstall`，交互输入确认词 `DELETE` 后删除原目录/容器/数据卷再重装；非交互环境用 `--reinstall --confirm DELETE`；
- B：`sudo bash deploy/upgrade/upgrade.sh --to <版本>`，保留数据升级。

### 3.4 破坏性操作纪律（测试必读）

删除是不可逆的，以下三条为硬约束，`install.sh` / `upgrade.sh` 顶部注释已同步写明：

1. **只删匹配的对象**：容器只删 `com.docker.compose.project.working_dir == 当前安装目录` 的；
   数据卷只删 `com.docker.compose.project == 当前项目名` 且属于本安装器声明的三个卷
   （`<项目名>_drill-mysql-data`、`<项目名>_drill-app-data`、`<项目名>_drill-app-uploads`）。
   禁止按固定容器名（`drill-app`、`drill-mysql`）或固定卷名（`drill-deploy_*`）删除——
   同一台机器上可能同时存在多个实例。
   （注：compose 不会给数据卷打 `working_dir` 标签，所以卷这一侧只能靠“项目名 + 卷名”双重匹配。）
2. **测试必须隔离**：测安装/升级/重装请使用独立目录 + 独立项目名，例如

   ```bash
   sudo bash deploy/install/install.sh \
     --install-dir /home/test/opt/cyber-drill --project drilltest --port 18090 --mysql-port 13306
   ```

   禁止把测试指向 `/opt/cyber-drill` 或现网 compose 项目。
3. **删除前先看清单**：`--reinstall` 会先打印将删除的容器/数据卷数量与 ID，
   数量或名称不符合预期时立即中断（`Ctrl+C`），不要继续。

> 反例（真实事故）：曾用伪造的安装目录触发旧版清理逻辑，旧逻辑按固定容器名 `drill-app`/`drill-mysql`
> 删除，误删了同机现网实例的容器与数据卷。现版本已改为仅按工作目录标签清理，并在删除前打印清单。

这样可避免旧库口令与新 `.env` 不一致导致“装完却登不上”。

---

## 4. 参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `--offline-dir <目录>` | 脚本同级 `offline/` 或 `/opt/cyber-drill-offline` | 离线资源目录（含 `baseline/`） |
| `--snapshot <文件>` | 自动查找 | 指定基线快照 `.sql.gz` |
| `--mode auto\|offline\|online` | `auto` | 有离线镜像包时优先离线 |
| `--version <版本>` | 读取仓库 `VERSION` | 应用镜像版本，禁止写死 |
| `--image-source auto\|ghcr\|swr` | `auto` | 应用镜像来源；`auto` 探测后选择，国内推荐 `swr` |
| `--registry <ghcr\|swr>` | — | `--image-source` 的别名 |
| `--install-dir <目录>` | `/opt/cyber-drill` | 安装目录（compose、.env、日志、基线副本） |
| `--port <端口>` | `8090` | 应用对外端口 |
| `--mysql-port <端口>` | `3306` | MySQL 宿主机回环端口 |
| `--project <名称>` | `drill` | compose 项目名；容器名跟随项目名：默认 `drill-app` / `drill-mysql`，其它项目名为 `<项目名>-app` / `<项目名>-mysql` |
| `--force-env` | 关 | 备份旧 `.env` 后重新生成口令（不删数据卷） |
| `--show-secrets` | 关 | 结束时再打印一次初始口令 |
| `--reinstall` | 关 | 已存在安装目录时删除重装（必须输入确认词） |
| `--confirm DELETE` | — | 非交互环境下 `--reinstall` 的确认词 |
| `--skip-firewall` | 关 | 不修改 firewalld |
| `--tls-cert <证书>` | 关 | 启用应用内 HTTPS（PEM 全链，如 `fullchain.pem`）；必须与 `--tls-key` 成对 |
| `--tls-key <私钥>` | 关 | 应用内 HTTPS 私钥（PEM）；与 `--tls-cert` 成对，缺一即退出码 10 |

---

## 5. 安装脚本做了什么

1. **前置检查**：root、x86_64、磁盘 ≥ 3 GB、8090 占用、识别离线/联网来源与访问 IP。
2. **安装引擎**：离线用自带 RPM，联网用 `moby-engine`；安装 compose v2 并启动 docker。
3. **准备镜像**：优先导入离线 tar，缺失时按配置拉取并统一打标签。
4. **定位快照**：按 `--offline-dir/baseline` → 脚本同级 `baseline/` → 安装目录顺序查找，校验 `SHA256SUMS`，读取 `manifest.env` 的期望值。
5. **创建目录与 compose**：MySQL 仅绑回环、命名卷持久化；`.env` 以单文件挂进应用容器（用于首登改密后清空一次性口令）。
6. **生成 env**：随机数据库口令、JWT/SESSION 密钥、两个管理员初始口令、备份加密口令 `BACKUP_PASSPHRASE`、恢复口令 `RESTORE_PASSWORD`；权限 `600`、属主 `10001`（容器用户），已有文件不覆盖。
7. **初始化数据库**：见第 6 节判定 → 空库恢复快照 → 逐项校验 76/21/1165/22/12/20 → 写 `.restored`。
8. **启动应用**：等 healthy，再请求 `/api/health` 校验版本；`drill-seed` 创建管理员（`must_change=1`）。
9. **收尾**：firewalld 放行端口，写 `.installed`，打印版本、口径、访问地址与一次性初始口令。

0.9.0 在验证机实测：镜像已在本地时，整条命令 **44 秒**完成（含 MySQL 初始化与快照恢复）。

---

## 6. 空库判定与覆盖保护

| 探测结果 | 行为 |
|----------|------|
| `drill_system` 表数 = 0 | 恢复基线快照 |
| 有表 + 存在本安装器 `.restored` 标记 | 视为断点续装，跳过恢复，只做口径校验与启动 |
| 有表 + 无标记 | **拒绝覆盖**，退出码 26，提示改用升级工具或先清空数据卷 |

不存在“看起来像空库就重建”的判断：只有表数为 0 才恢复。第 3 种情况是**升级环境**，
请用 [升级指南](upgrade.md) 的 `deploy/upgrade/upgrade.sh`。

---

## 7. 验收口径（1.0.0）

```bash
curl -fsS http://127.0.0.1:8090/api/health
# {"ok":true,"service":"cyber-drill-system","version":"1.0.0",...}

PW="$(sed -n 's/^MYSQL_ROOT_PASSWORD=//p' /opt/cyber-drill/.env)"
docker exec drill-mysql mysql -uroot -p"$PW" -N -B drill_system -e "
 SELECT (SELECT COUNT(*) FROM schema_migrations),
        (SELECT value FROM settings WHERE \`key\`='brand_version'),
        (SELECT COUNT(*) FROM scenarios),
        (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL),
        (SELECT COUNT(*) FROM script_nodes WHERE deleted_at IS NULL AND flow_type='distractor'),
        (SELECT COUNT(*) FROM form_templates),
        (SELECT COUNT(*) FROM dict),
        (SELECT JSON_LENGTH(value) FROM dict WHERE \`key\`='eventTypes');"
# 76  v1.0.0  21  1165  750  22  12  20
```

应用层：

- 登录页在 **`/`**（`/login` 返回 404），标题“登录 · 示例医院网络安全应急演练系统”。
- 验证码走 `GET /api/auth/captcha`，返回 `{id,image}`，`image` 为 SVG 算术题（矢量绘制，无明文题干）。
- `POST /api/auth/login` 入参 `{username,password,captchaId,captchaCode}`；用一次性初始口令登录成功时
  `user.mustChange=true`，且签发的是**受限令牌**：除 `/api/auth/me`、`/api/auth/change-password`、
  `/api/auth/logout`、`/api/health` 外一律 `403 PASSWORD_CHANGE_REQUIRED`。
- `POST /api/auth/change-password` 入参 `{oldPassword,newPassword}`（≥ 8 位且含字母与数字），成功后
  `must_change=0`、旧会话全部吊销、重新签发正式令牌，并把 `.env` 中对应的一次性口令清空并标注失效。

重启验证：

```bash
systemctl reboot
# 开机后
docker ps --format '{{.Names}} {{.Status}}'   # drill-mysql / drill-app 均为 healthy
curl -fsS http://127.0.0.1:8090/api/health
```

---

## 8. 失败处置

### 8.1 退出码

| 退出码 | 阶段 | 常见原因 |
|--------|------|----------|
| 10 | 前置检查 | 非 root、架构不符、磁盘不足、端口被占、离线目录无镜像包 |
| 20 | Docker | 离线 RPM 缺失、源不可用、compose 下载失败、docker 启动失败 |
| 21 | 镜像 | 离线 tar 缺失/损坏、仓库不可达 |
| 23 | MySQL | 初始化慢（180 s 超时）、端口冲突、`MYSQL_*` 与 `.env` 不一致 |
| 25 | 应用 | 启动失败、240 s 内未 healthy、`/api/health` 无版本号 |
| 26 | 已有库保护 | 探测到非本安装器创建的库，拒绝覆盖 |
| 27 | 基线快照 | 快照缺失、`SHA256SUMS` 不匹配、恢复失败、口径校验不一致 |

### 8.2 快照恢复失败

- 日志：`/opt/cyber-drill/logs/restore-<时间>.log`，脚本会同时打印末尾 20 行。
- 快照校验不匹配：重新用 `build-baseline.sh` 生成并核对 `SHA256SUMS`，不要手改快照。
- 恢复中途失败：`docker compose -p drill -f /opt/cyber-drill/docker-compose.yml down -v` 清空数据卷后重装
  （此时 `.restored` 尚未写入，重跑不会误判为已恢复）。

### 8.3 口径校验不一致

退出码 27，错误行会给出“期望 / 实际”。先看 `manifest.env` 的期望值是否与该版本口径一致，
再确认快照是不是对应版本产物；**不要**通过改 `manifest.env` 或删表来“凑数”。

### 8.4 MySQL 未就绪

```bash
docker logs drill-mysql --tail 80
```

重点检查 `.env` 中 `DB_USER` / `DB_PASSWORD` 与容器实际 `MYSQL_USER` / `MYSQL_PASSWORD` 是否对应（脚本已自动映射）。

### 8.5 compose 下载失败（联网路径）

联网路径会依次尝试多个下载入口；全部失败时，把任意来源的 compose v2 二进制放到
`<offline-dir>/bin/docker-compose` 后重跑。

---

## 9. 卸载与重装

### 9.1 停止但保留数据

```bash
docker compose -p drill -f /opt/cyber-drill/docker-compose.yml stop
```

### 9.2 彻底删除（含数据，不可恢复）

```bash
docker compose -p drill -f /opt/cyber-drill/docker-compose.yml down -v
rm -rf /opt/cyber-drill
```

### 9.3 仅重置环境文件

```bash
sudo bash install.sh --force-env
```

旧 `.env` 备份为 `.env.bak.<时间>`；数据卷与数据库不受影响，但数据库口令随之改变，
MySQL 容器会因口令不一致无法登录——非必要不要单独重置 `.env`。

### 9.4 重装

按 9.2 清空后重新执行安装命令即可；Docker 引擎无需重复安装，脚本自动跳过。

---

## 10. TLS / 备份加密 / 恢复（1.0.0 安全项）

### 10.1 TLS（对外交付必须）

**默认不强制，但对外交付（任何非本机访问）必须开启。**

| 方式 | 做法 | 适用 |
|------|------|------|
| 应用内 HTTPS | 安装时加 `--tls-cert /path/fullchain.pem --tls-key /path/privkey.pem`；容器内固定 `/opt/drill/tls/`，宿主 `8443` → 容器 `8443` | 无反向代理、单机直连 |
| 反向代理卸载 | Go 保持 `TLS_ENABLED=false` 只监听 8090，代理层终止 TLS | 推荐；可自动续期、HSTS、缓存 |

反向代理样例：

- Nginx：[`deploy/linux/nginx-drill.conf`](../deploy/linux/nginx-drill.conf)（80→443 跳转、HSTS、`client_max_body_size` 与 `UPLOAD_MAX_MB` 对齐）
- Caddy：[`deploy/linux/Caddyfile.example`](../deploy/linux/Caddyfile.example)（自动签发/续期证书、自动 80→443）

应用内 TLS 相关变量：

```ini
TLS_ENABLED=true
TLS_ADDR=:8443
HTTPS_CERT_FILE=/opt/drill/tls/fullchain.pem
HTTPS_KEY_FILE=/opt/drill/tls/privkey.pem
HTTP_REDIRECT_TO_HTTPS=true   # 同时监听 HTTP 并 301 跳转；不需要就保持 false
```

`TLS_ENABLED=true` 但证书缺失 → **拒绝启动（exit 2）**，不静默降级。
反向代理层还需把来源网段写进 `TRUSTED_PROXIES`，否则审计里的客户端 IP 会是代理 IP。

### 10.2 备份加密与权限

- 应用内备份（后台“备份/恢复”页、`/api/admin/backup`）产出的 dump 与上传归档**默认 AES-256-GCM 加密**，文件名为 `*.sql.enc` / `*.uploads.tar.gz.enc`。
- 口令来源：`BACKUP_PASSPHRASE`（优先）或只读挂载的 `BACKUP_PASSPHRASE_FILE`。**口令不写日志、不写进备份文件**；安装器生成 32 字节随机值写入 `.env`（600）。
- 未配置口令时备份接口直接拒绝（503），**不会退化为明文备份**。
- 备份文件权限 `600`、备份目录 `700`，由应用启动时自动设置。
- **遗失 `BACKUP_PASSPHRASE` = 旧加密备份无法恢复**，请与 `.env` 一起纳入密钥管理。

### 10.3 恢复（高危）

- 后台恢复需要两个独立输入：二次确认词 `RESTORE` + `RESTORE_PASSWORD`（环境变量，与登录口令分离）；未配置 `RESTORE_PASSWORD` 时恢复接口直接拒绝。
- **恢复会覆盖整库并清空全部登录会话**，所有用户需重新登录；恢复前应用会自动生成一份加密备份（`drill-pre-restore-*.sql.enc`），导入失败自动回滚。
- `multiStatements=true` 意味着恢复等于执行任意 SQL，**仅限管理员在本机操作**：不要把恢复接口暴露到公网，不要在恢复窗口内处理不可信文件。
- 恢复后应校验 `schema_migrations = 76`、`settings.brand_version = 'v1.0.0'`。

### 10.4 审计保留

`AUDIT_RETENTION_DAYS` 控制 `audit_logs` 保留天数（默认 365，`0` = 不清理）。后台每 6 小时清理一次，并写一条自身的审计记录（操作人 `system`）。该清理不新建表。

---

## 11. 相关文档

- [升级指南（容器部署）](upgrade.md)
- [Linux 部署指南（systemd / 源码）](linux-deployment.md)
- [从源码构建与升级数据库](build-from-source.md)
- [Docker 镜像说明](docker-image.md)
