# 1.0.0 发布说明

| 项目 | 内容 |
|---|---|
| 版本 | **1.0.0** |
| 发布日期 | 2026-11-10 |
| Git 标签 | `v1.0.0` |
| 上一版本 | `0.9.0` |
| 发布通道 | stable（`best_effort: false`） |
| 基线快照 | `deploy/install/baseline/drill-baseline-1.0.0.sql.gz` |
| 数据库迁移总量 | 76 条（`schema_migrations`） |
| 品牌版本 | `v1.0.0` |
| 交付形态 | Linux ELF（amd64 / arm64 / loong64，CGO 静态） + Docker 镜像 |

> 本说明描述的是 **1.0.0 工作区当前实际能力**。所有命令、参数、版本号以仓库内脚本与清单为准；设计文档中的历史记录（如 `docs/upgrade-tool-design.md`）不随本说明改写。

---

## 一、版本统一

1.0.0 将此前分散的版本标识统一到同一口径：

| 位置 | 1.0.0 取值 |
|---|---|
| `server/internal/version/version.go` | `1.0.0` |
| `deploy/upgrade/manifest.json` | `1.0.0`，`previous: 0.9.0` |
| 基线快照文件 | `drill-baseline-1.0.0.sql.gz` |
| 迁移脚本 | 新增 `0123_brand_version_100` |
| `settings.brand_version` | `v1.0.0` |
| `schema_migrations` | 76 |
| 镜像 tag | `1.0.0` |
| README / 安装 / 升级 / 安全文档 | 统一为 1.0.0 |

> 镜像 digest 与 size 在 1.0.0 构建推送后回填。`manifest.json` 中 1.0.0 的 `digest` 当前为 `null`、`size_bytes` 为 `0`，属于**发布前待回填**状态；使用 `--pin-digest` 时工具会告警并按 tag + 健康检查继续。

---

## 二、安装：基线快照

1.0.0 起，全新安装不再逐条重放 76 个迁移，而是导入 **1.0.0 基线快照**，再由 `drill-migrate` 校验并记录版本：

- 快照路径：`deploy/install/baseline/drill-baseline-1.0.0.sql.gz`
- 导入后：`schema_migrations = 76`、`brand_version = v1.0.0`
- 基线只包含结构与种子数据，**不包含任何生产账号、口令、业务数据或患者信息**
- 管理员账号由安装器/`drill-seed` 现场生成，首次登录强制改密

安装方式（按环境选择）：

```bash
# ① Docker 一键安装（唯一入口，自动选镜像源）
sudo bash deploy/install/install.sh

# ② 离线包安装
sudo bash deploy/install/install.sh --offline-dir /opt/cyber-drill-offline

# ③ Docker Compose 手动
cd deploy/docker-dist && docker compose up -d
```

TLS 参数（安装器）：

```bash
sudo bash deploy/install/install.sh \
  --tls-cert /etc/ssl/drill/fullchain.pem \
  --tls-key  /etc/ssl/drill/privkey.pem
```

> 公开部署必须启用 TLS：应用内 `TLS_ENABLED=true` + 证书，或反向代理卸载。TLS 证书缺失时应用**拒绝启动**（exit code 2），不降级为明文。

---

## 三、升级工具与升级矩阵

升级工具：`deploy/upgrade/upgrade.sh`（Linux）与 `drill-upgrade`。核心行为：

- 按 `manifest.json` 的 `previous` 链自动计算从当前版本到目标版本的升级路径；
- 升级前强制备份（默认 AES-256-GCM 加密），并校验备份 sha256；
- 迁移在一次性容器中执行，不落数据卷；
- 失败自动回滚；DDL 迁移的回滚为**整库恢复**（见已知限制）；
- 支持断点续跑、并发锁、`--dry-run`、`--pin-digest`。

### 3.1 升级到 1.0.0 的矩阵

| 起点 | 迁移条数 | 是否含 DDL | 是否需提前停 app | 说明 |
|---|---:|---|---|---|
| `0.9.0` | 1 | 否 | 否 | 仅 `0123_brand_version_100`，向后兼容；容器重建秒级窗口 |
| `0.8.9` | 5 | 否 | 否 | 0.9.0 的 4 条 + 1.0.0 的 1 条 |
| `0.8.8` | 6 | 否 | 否 | 0.8.9、0.9.0、1.0.0 |
| `0.8.7` | 11 | 否 | 否 | 0.8.8 起无 DDL |
| `0.8.6` | 16 | 是 | 是 | 链上 0.8.7 含 DDL，工具先停 app 再迁移 |
| `0.8.3` | 29 | 是 | 是 | 含 0.8.6、0.8.7 两段 DDL，停机≈迁移耗时 |
| `0.6.0` | 64 | 是 | 是 | 跨多个 DDL 版本，建议先演练 |
| `0.5.1` | 73 | 是 | 是 | 跨多个 DDL 版本，建议先演练 |
| `0.5.0` | 76 | 是 | 是 | `best_effort`，升级后必须人工抽查业务页面 |

> 上表按 `manifest.json` 的 `migrations.count` 累加；实际执行以工具输出的计划为准。`--dry-run` 会提前打印停机点与迁移条数。

### 3.2 推荐命令

```bash
# 预演（不改动）
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --dry-run

# 正式升级
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0

# 指定镜像来源
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --registry swr

# 使用本地镜像 tar
sudo ./deploy/upgrade/upgrade.sh --to 1.0.0 --image-tar ./cyber-drill-1.0.0.tar
```

升级后校验：

```bash
curl -fsS http://127.0.0.1:8090/api/health
# 期望包含 "version":"1.0.0"

mysql -h127.0.0.1 -P3306 -u root -p -e \
  "SELECT COUNT(*) FROM schema_migrations; SELECT value FROM settings WHERE name='brand_version';"
# 期望 76 与 v1.0.0
```

---

## 四、安全加固

### 4.1 登录

- 先校验口令，再返回统一错误文案（账号不存在、口令错误、账号停用、角色不符不再可区分），避免账号枚举；
- 口令比较使用常量时间比较；不存在账号也执行一次 dummy 散列，缓解时序差异；
- 验证码错误单独提示，但**单独计数并计入锁定阈值**（阈值为密码阈值的 2 倍，默认 5 次 / 15 分钟，键为「账号 + 客户端 IP」）；
- 锁定期内即使口令正确也返回 423，防止绕过；
- 首次登录强制改密；口令使用 scrypt 加盐散列；
- `CAPTCHA_ENABLED` 默认 `true`，显式写出，避免默认值漂移。

### 4.2 备份

- 应用内备份为 **AES-256-GCM 分块加密**，文件格式 `DRLBK1`；
- 密钥由 `BACKUP_PASSPHRASE` 经 **PBKDF2-SHA256 600,000 次**派生；
- 口令不写日志、不写进备份文件名或 manifest；
- 备份文件权限 `600`、目录 `700`；
- 未配置 `BACKUP_PASSPHRASE` 时备份接口返回 503，拒绝生成明文备份。

### 4.3 恢复

- 恢复需要独立的 `RESTORE_PASSWORD`，与登录口令分离；
- 前端二次确认 + 后端口令校验双门禁；
- 恢复前自动生成加密 pre-restore 备份；
- 恢复失败自动回滚到 pre-restore 备份；
- 恢复后清空会话（强制重新登录）。

### 4.4 TLS

- 应用内 TLS：`TLS_ENABLED=true`、`TLS_ADDR`（默认 `:8443`）、`HTTPS_CERT_FILE`、`HTTPS_KEY_FILE`；
- HTTP 跳转：`HTTP_REDIRECT_TO_HTTPS=true` 时返回 301，`Location` 使用 **TLS 监听端口**，不会指回 HTTP 端口；
- 反向代理：提供 Nginx / Caddy 样例，支持 HSTS 与证书续期；
- 证书缺失或无法读取时拒绝启动。

Linux 侧实测（一次性容器，非现网服务）：

```text
HTTP/1.1 301 Moved Permanently
Content-Type: text/html; charset=utf-8
Location: https://127.0.0.1:18443/api/health
Date: Tue, 10 Nov 2026 01:56:37 GMT
Content-Length: 69
```

HTTPS 健康检查返回：

```json
{"ok":true,"service":"cyber-drill-system","time":"2026-11-10 01:56:37","version":"1.0.0"}
```

### 4.5 审计保留

- `audit_logs` 只追加；
- `AUDIT_RETENTION_DAYS` 默认 `365` 天，后台每 6 小时清理一次；
- 设为 `0` 表示不自动清理；
- 每次清理会写入一条 `system` 自身的审计记录，记录删除条数与阈值；
- 登录、权限变更、备份、恢复、重置、审计清理等敏感操作均写审计。

### 4.6 默认值显式化

| 变量 | 默认值 | 说明 |
|---|---|---|
| `CAPTCHA_ENABLED` | `true` | 登录验证码默认开启 |
| `SCREEN_PUBLIC_VIEW` | `true` | 大屏匿名只读默认开启；不需要时显式设为 `false` |
| `JWT_TTL` | `480` 分钟 | 会话令牌有效期 |
| `LOGIN_MAX_FAILURES` | `5` | 登录失败锁定阈值 |
| `LOGIN_LOCK_MINUTES` | `15` | 锁定时长 |
| `AUDIT_RETENTION_DAYS` | `365` | 审计保留天数，0 为不清理 |
| `UPLOAD_MAX_MB` | `20` | 上传大小上限 |

---

## 五、文档与合规交付物

1.0.0 新增/更新以下交付材料：

| 文档 | 用途 |
|---|---|
| [隐私与数据处理说明](privacy-data-processing.md) | 面向医院/政府交付；字段、目的、法律依据、留存、删除、共享、出境、责任人 |
| [第三方许可证清单](../THIRD-PARTY-NOTICES.md) | Go 依赖、前端、运行时组件、MySQL 的许可证与风险提示 |
| [发布前收尾清单](pre-release-checklist-1.0.0.md) | 发 1.0.0 前必须人工确认的事项 |
| [升级指南](upgrade.md) | 操作示例统一到 1.0.0 |
| [一键安装（Docker / openEuler）](install-docker-openeuler.md) | 1.0.0 基线快照与 TLS 参数 |
| [Linux 部署](linux-deployment.md) | TLS、备份加密、恢复校验 |
| [安全加固指南](security-guide.md) | 登录、备份、TLS、审计留存 |

---

## 六、已知限制（发布前必须知悉）

### 6.1 大屏匿名只读

- `SCREEN_PUBLIC_VIEW` 默认 `true`，`/screen.html` 及大屏只读接口允许**未登录访问**，仅用于 1920×1080 监视大屏。
- 匿名访问能看到的范围限于大屏展示数据，但仍可能包含参演单位、角色、处置进度、消息标题等敏感内容。
- **必须**通过网络隔离、反向代理访问控制、防火墙白名单或显式设置 `SCREEN_PUBLIC_VIEW=false` 限制访问；不要把大屏直接暴露到互联网。

### 6.2 验证码依赖

- 验证码为服务端内存态 SVG，默认开启（`CAPTCHA_ENABLED=true`）。
- 多实例部署时验证码不共享；进程重启后未使用的验证码失效。
- 关闭验证码（`CAPTCHA_ENABLED=false`）会显著降低登录抗暴力破解能力，仅建议在完全隔离的联调环境使用。
- 当前未接入短信/邮件/硬件令牌等第二因素；高安全要求场景应通过反向代理、VPN、堡垒机或统一身份认证补充。

### 6.3 登录锁定为内存态，重启清零

- 登录失败计数与锁定状态保存在**进程内存**，不落库。
- 应用重启、容器重建或滚动发布后，锁定计数与锁定状态清零；攻击者可能通过反复触发重启绕过锁定。
- 计数键为「账号 + 客户端 IP」，多实例之间不共享；水平扩容会削弱锁定效果。
- 因此 1.0.0 定位为**单实例部署**；如需高可用/多实例，应在网关层增加统一限流与账号锁定，或改造为共享存储。

### 6.4 备份恢复需口令

- 备份必须配置 `BACKUP_PASSPHRASE`；恢复必须配置 `RESTORE_PASSWORD`。
- 口令丢失时，**已生成的加密备份无法恢复**（AES-256-GCM + PBKDF2，无后门、无托管密钥）。
- 备份文件本身不足以完成恢复，必须同时保管口令；建议口令与备份分离保管、双人控制。
- 恢复会覆盖当前数据库；恢复前的 pre-restore 备份也使用同一备份口令加密。

### 6.5 DDL 回滚只能整库恢复

- 升级工具对**含 DDL 的迁移**不提供逐条 `down` 脚本；回滚方式是**整库恢复到升级前备份**。
- 整库恢复会丢失备份时点之后写入的业务数据，并可能使已删除数据重新出现。
- 跨 DDL 版本升级（如 0.8.3 → 1.0.0）停机时间≈迁移耗时，必须在维护窗口执行并提前演练。
- 无 DDL 的升级（0.9.0 → 1.0.0）可不停 app，仅有容器重建的秒级窗口。

### 6.6 其他限制

- **单实例定位**：会话、验证码、登录锁定均为单实例设计；未内置分布式会话/共享缓存。
- **用户无硬删除界面**：账号只能停用；物理删除需数据库运维按变更单执行。
- **容器日志未内置轮转**：`docker-compose.yml` 未设置 `logging.max-size`，长期运行需客户配置 Docker 日志轮转或 logrotate。
- **大屏分辨率**：以 1920×1080 为基准缩放，非标准分辨率可能出现滚动或留白。
- **1.0.0 镜像 digest 待回填**：`manifest.json` 中 `digest: null`、`size_bytes: 0`，构建推送后需回填；回填前 `--pin-digest` 只能告警并按 tag 校验。

---

## 七、发布与验收

发布前请逐项完成 [发布前收尾清单](pre-release-checklist-1.0.0.md)，至少确认：

- [ ] 1.0.0 基线快照可导入，`schema_migrations=76`、`brand_version=v1.0.0`；
- [ ] 0.9.0 → 1.0.0 升级演练通过（1 条迁移、无 DDL、健康检查 `1.0.0`）；
- [ ] 登录统一文案（账号不存在 / 口令错误 / 账号停用 / 角色不符）、验证码错误计入锁定、锁定期 423；
- [ ] 加密备份可生成、可恢复；错误口令被拒绝；pre-restore 备份存在；
- [ ] TLS 健康检查通过，HTTP→HTTPS 301 的 `Location` 指向 TLS 端口；
- [ ] 审计保留清理生效并写自身审计；
- [ ] 隐私说明、许可证清单、README 版权行已更新；
- [ ] tag / Release / 镜像推送顺序已确认。

---

## 八、相关文档

- [README](../README.md)
- [升级指南](upgrade.md)
- [从源码构建与升级数据库](build-from-source.md)
- [一键安装（Docker / openEuler）](install-docker-openeuler.md)
- [Linux 部署](linux-deployment.md)
- [安全加固指南](security-guide.md)
- [隐私与数据处理说明](privacy-data-processing.md)
- [第三方许可证清单](../THIRD-PARTY-NOTICES.md)
- [发布前收尾清单](pre-release-checklist-1.0.0.md)
