# 1.0.0 发布前收尾清单

> 用途：在打 `v1.0.0`、创建 Release、推送镜像**之前**逐项人工确认。
> 用法：每项填写「负责人 / 完成时间 / 证据位置」，未完成不得进入发布。
> 本清单只列发布前必须确认的事项，不替代测试报告与安全评审。

| 项目 | 内容 |
|---|---|
| 目标版本 | 1.0.0 |
| 计划发布日期 | 2026-11-10 |
| 发布负责人 | 【待填写】 |
| 安全负责人 | 【待填写】 |
| 运维负责人 | 【待填写】 |

---

## 一、必须人工确认的五项

### 1. 131 现网 `platformadmin` 初始口令处理

- [ ] 确认 131 现网 `platformadmin` 账号的初始口令**未出现在**仓库、Wiki、聊天记录、部署脚本、截图或工单中。
- [ ] 确认该账号 `must_change = 1`，首次登录强制改密；如已改密，确认新口令由客户双人保管。
- [ ] 如初始口令曾以明文传递，**立即改密**，并检查 `audit_logs` 中该账号的登录记录是否异常。
- [ ] 确认不存在其他默认账号（如 `admin` / `test` / `demo`）可登录现网；如有，停用或改密。
- [ ] 确认 131 现网登录已启用验证码（`CAPTCHA_ENABLED=true`），且 TLS 已启用。

| 负责人 | 完成时间 | 证据 |
|---|---|---|
| 【待填写】 | 【待填写】 | 【待填写：审计截图 / 改密记录】 |

### 2. `upg-drill-a/b/c` 演练环境清理

- [ ] 停止并删除 `upg-drill-a`、`upg-drill-b`、`upg-drill-c` 相关容器、网络、数据卷与临时目录。
- [ ] 删除演练用的临时 MySQL 数据目录、备份目录、`.upgrade-runs`、`.upgrade.lock`。
- [ ] 确认演练环境未占用现网端口（8090 / 3306 / 8443 等）。
- [ ] 确认演练环境中的测试账号、测试数据、明文口令文件已删除。
- [ ] 如演练环境需要保留用于复盘，移入隔离目录并明确标注“非生产”，不得与发布产物混放。

| 负责人 | 完成时间 | 证据 |
|---|---|---|
| 【待填写】 | 【待填写】 | 【待填写：`docker ps -a` / 目录清理截图】 |

### 3. `docs/login-redesign/` 与 `verify_shots/` 的去留

- [ ] 确认 `docs/login-redesign/` 是**保留为设计记录**还是**发布前删除**。
  - 保留：移入 `docs/design/` 或 `docs/archive/`，确认不含口令、密钥、真实患者信息、现网截图敏感内容。
  - 删除：从工作区删除；如已进入 Git 历史，评估是否需要清理历史（注意：改写历史会影响所有协作者）。
- [ ] 确认 `verify_shots/` 是**保留为验收证据**还是**发布前删除**。
  - 保留：移入 `docs/verify/` 并脱敏（遮盖账号、IP、患者信息、令牌）。
  - 删除：从工作区删除；确认未提交到 Git。
- [ ] 确认上述目录中的截图不含 131/132 现网地址、账号、口令、患者信息。
- [ ] 确认 `.gitignore` 已覆盖本地产物（`temp/`、`verify_shots/`、`*.log`、`.env` 等）。

| 负责人 | 完成时间 | 证据 |
|---|---|---|
| 【待填写】 | 【待填写】 | 【待填写：目录清单 / 脱敏说明】 |

### 4. 132 上遗留的明文口令文件

- [ ] 在 132 上检索部署目录、`/tmp`、`/root`、`/home`、升级记录、脚本注释中的明文口令：`DB_PASSWORD`、`JWT_SECRET`、`SESSION_SECRET`、`BACKUP_PASSPHRASE`、`RESTORE_PASSWORD`、`platformadmin` 初始口令。
- [ ] 删除所有临时明文口令文件（如 `*.txt`、`*.md`、`*.sh.bak`、`drill.env.bak-*`、升级记录中的口令片段）。
- [ ] 如口令曾以明文落盘或出现在日志中，**全部轮换**：数据库口令、JWT/SESSION 密钥、备份口令、恢复口令、管理员口令。
- [ ] 确认最终口令只存在于权限 `600` 的 env 文件或客户密码管理系统中，且不在 shell history 中。
- [ ] 确认 132 不作为 1.0.0 发布源；如需保留为测试机，先完成清理与快照。

| 负责人 | 完成时间 | 证据 |
|---|---|---|
| 【待填写】 | 【待填写】 | 【待填写：检索命令输出 / 轮换记录】 |

### 5. Wiki 更新与 tag / Release / 镜像推送的执行顺序

推荐顺序（**不要跳步、不要移动已推送的 tag**）：

1. **Wiki 内容定稿**：在草稿区更新 1.0.0 安装、升级、TLS、备份恢复、已知限制；暂不对外发布。
2. **候选镜像构建**：从发布提交构建镜像，推送到临时 tag，记录 `RepoDigest` 与 size；用候选镜像完成安装/升级/回滚演练。
3. **回填 manifest**：把 digest、size、发布日期回填到 `deploy/upgrade/manifest.json`；更新 `docs/release-1.0.0.md` 中的镜像信息。
4. **最终验证**：`go build`、迁移校验、登录/备份/TLS/审计实测、基线快照校验、隐私与许可证文档复核。
5. **打 tag**：在最终提交上创建 `v1.0.0` 并推送；确认 tag 指向的提交包含回填后的 manifest。
6. **推送正式镜像**：用候选镜像的同一 digest 打 `1.0.0`（如需要再打 `latest`），推送到 GHCR 与 SWR；核对 registry 返回的 digest 与 manifest 一致。
7. **创建 Release**：附 `docs/release-1.0.0.md`、基线快照、镜像 tar（如提供）、SHA256 校验值；不要只写“修复若干问题”。
8. **Wiki 正式发布 + 公告**：确认安装入口、升级命令、已知限制与 Release 一致后，再对外公告。
9. **发布后验证**：匿名拉取镜像、全新安装、0.9.0 → 1.0.0 升级、健康检查 `version=1.0.0`。

- [ ] Wiki 草稿已定稿并与 Release 口径一致。
- [ ] 候选镜像 digest 已记录，manifest 已回填。
- [ ] tag `v1.0.0` 已创建并推送，指向最终提交。
- [ ] 正式镜像已推送 GHCR 与 SWR，digest 与 manifest 一致。
- [ ] Release 已创建，包含 release notes、基线快照、校验值。
- [ ] Wiki 已正式发布，公告已发出。

| 负责人 | 完成时间 | 证据 |
|---|---|---|
| 【待填写】 | 【待填写】 | 【待填写：tag / digest / Release URL】 |

---

## 二、发布前技术核对（建议逐项打勾）

### 2.1 版本与构建

- [ ] `server/internal/version/version.go` 为 `1.0.0`。
- [ ] `deploy/upgrade/manifest.json` 中 1.0.0 的 `previous` 为 `0.9.0`、`schema_migrations_total` 为 `76`。
- [ ] `drill-baseline-1.0.0.sql.gz` 可导入，导入后 `schema_migrations=76`、`brand_version=v1.0.0`。
- [ ] `go build ./...` 通过；`go vet ./...` 无阻断问题。
- [ ] 二进制 `-ldflags` 注入版本为 `1.0.0`；`/api/health` 返回 `version=1.0.0`。
- [ ] 镜像 OCI 标签 `org.opencontainers.image.version=1.0.0`、`licenses=MIT`。

### 2.2 安全

- [ ] 登录统一文案：账号不存在 / 口令错误 / 账号停用 / 角色不符不再可区分；验证码错误单独提示。
- [ ] 验证码错误计入锁定；连续失败达到阈值后返回 423；锁定期内正确口令仍 423。
- [ ] 备份生成 `.sql.enc`，无明文 `.sql` 残留；文件权限 600、目录 700。
- [ ] 备份接口在未配置 `BACKUP_PASSPHRASE` 时返回 503。
- [ ] 恢复在缺少/错误 `RESTORE_PASSWORD` 时被拒绝；恢复前生成 pre-restore 加密备份。
- [ ] TLS 健康检查通过；`curl -D- http://<host>:<http端口>/api/health` 返回 301，`Location` 指向 **TLS 端口**。
- [ ] `AUDIT_RETENTION_DAYS` 已按客户要求设置；清理后写入 `system` 自身审计。
- [ ] `.env` / `drill.env` 未被 Git 跟踪；仓库中无明文密钥、口令、证书私钥。

### 2.3 文档与合规

- [ ] `README.md` 版本为 1.0.0，版权行与版本号一致。
- [ ] `public/index.html` 底部版权行已更新；确认是否需要重建镜像（静态资源内置时需重建/重启容器）。
- [ ] `docs/privacy-data-processing.md` 已由客户法务/合规复核，方括号占位符已填写。
- [ ] `THIRD-PARTY-NOTICES.md` 已复核；MPL-2.0 与 GPL-2.0 组件说明已确认。
- [ ] `docs/release-1.0.0.md` 与 `docs/upgrade.md`、`install-docker-openeuler.md`、`linux-deployment.md`、`security-guide.md` 的版本、命令、参数一致。
- [ ] 设计文档中的历史记录未被误改（`docs/upgrade-tool-design.md` 等）。

### 2.4 发布物

- [ ] 基线快照 SHA256 已记录并在 Release 中提供。
- [ ] 镜像 tar（如提供）SHA256 已记录。
- [ ] 升级工具 `upgrade.sh` / `drill-upgrade` 可执行权限正确。
- [ ] 离线包内含 `install.sh`、`docker-compose.yml`、`drill.env.example`、基线快照、镜像 tar（如适用）。
- [ ] 发布目录中无 `temp/`、`verify_shots/`、测试脚本、明文口令、数据库 dump。

---

## 三、签字

| 角色 | 姓名 | 签字 | 日期 |
|---|---|---|---|
| 发布负责人 | 【待填写】 |  |  |
| 安全负责人 | 【待填写】 |  |  |
| 运维负责人 | 【待填写】 |  |  |
| 客户代表（如适用） | 【待填写】 |  |  |

> 本清单完成后，方可执行 tag、Release 与镜像推送。任何一项未完成，应延期发布，不得以“先发后补”方式跳过安全与合规项。
