# 应急演练指挥系统（Cyber-Drill-System）

> **使用中遇到问题，请优先查阅 Wiki（国内推荐 Gitee）：**
> Gitee <https://gitee.com/habeas/cyber-drill/wiki> ｜ GitHub <https://github.com/chenjinxiang1980/cyber-drill/wiki>
> 安装、升级、配置、常见问题、评分口径、备份恢复都在 Wiki 里持续维护。

面向医院、政府与企事业单位**内网**的桌面推演 + 实战演练指挥调度平台：内置勒索病毒、横向渗透、DDoS、钓鱼邮件、存储故障等高频安全事件剧本，覆盖演练编排、多角色协同、时序控制、处置记录、监视大屏、自动考评与复盘报告全流程。

> 当前版本：**1.0.4** · 许可证：MIT © 2026 陈金祥 · 软件著作权登记：**未登记**（截至 1.0.4）

---

## 界面预览

截图使用内置演示数据渲染，不含真实账号与内网地址。

| 登录页 | 角色工作台 |
|---|---|
| ![登录页](docs/images/login.png) | ![角色工作台](docs/images/workbench.png) |

| 剧本编辑（可视化流程） | 演练监视大屏 |
|---|---|
| ![剧本编辑](docs/images/scenario-edit.png) | ![演练监视大屏](docs/images/monitor.png) |

| 台账与统计 | 复盘报告导出 |
|---|---|
| ![台账与统计](docs/images/ledger.png) | ![报告导出](docs/images/report.png) |

---

## 功能概览

- **演练监视大屏**：1920×1080 大屏，实时操作流、角色处置状态、告警与消息三栏滚动。
- **角色工作台**：值班员 / 指挥员 / 管理员等角色工作台，处置弹窗、任务清单、业务系统影响态势。
- **剧本编排**：节点化可视化编辑、`before` / `after` 混淆门控、剧本版本历史与大/小版本策略。
- **演练项目管理**：项目状态机、基础版 / 实战版、参演单位与观察员、演练计时基准。
- **复盘与考评**：按节点时序自动评分、扣分项说明、演练报告导出（Word / TXT / HTML）、时序回放。
- **取证与台账**：证据归档与哈希保管链、操作台账、知识库与字典。
- **站内消息与会商**：按项目隔离的消息、未读计数、可选语音播报；演练期间多角色会商留痕。
- **安全加固**：JWT + Session + RBAC、登录失败锁定、算术验证码、TLS、审计日志、加密备份。
- **交付形态**：预构建 Docker 镜像（Go 单体服务 + MySQL 8.0）；前端为静态资源，由服务端直接托管。使用方不需要 Go 或 Node/npm 工具链。

---

## 部署（Docker，二选一）

**最终用户不需要 Go、不需要 Node.js/npm，也不需要 clone 源码。** 只需要一台 x86_64 Linux（优先 openEuler 24.03 LTS-SP1）和 Docker 环境；应用镜像从 GHCR / 华为云 SWR 拉取，或使用 Release 离线包。

### 前置条件

| 项 | 要求 |
|---|---|
| 系统 | x86_64 Linux；优先 openEuler 24.03 LTS-SP1（实机验证），统信 UOS Server 20 兼容可选 |
| 权限 | root（`sudo`） |
| 容器 | Docker Engine 20.10+ 与 Compose v2（`docker compose`）；未安装时安装器会处理（在线用 openEuler 源，离线用包内 RPM） |
| 磁盘 | 安装目录所在分区 ≥ 3GB 可用（建议 ≥ 5GB） |
| 端口 | 8090（应用）空闲；3306 仅绑定 127.0.0.1；启用 TLS 时 8443 |
| 目录 | 默认安装到 `/opt/cyber-drill`；自定义 `--install-dir` 时**父目录必须已存在** |
| 网络 | 在线路径需可达 GHCR 或华为云 SWR；离线路径不需要外网 |

### 路径一：在线安装（无需源码，推荐）

安装器与基线快照合计约 270 KB。Gitee 优先：

```bash
sudo mkdir -p /opt/cyber-drill-installer/baseline
cd /opt/cyber-drill-installer
sudo curl -fsSL -o install.sh \
  https://gitee.com/habeas/cyber-drill/raw/main/deploy/install/install.sh
sudo curl -fsSL -o baseline/drill-baseline-1.0.4.sql.gz \
  https://gitee.com/habeas/cyber-drill/raw/main/deploy/install/baseline/drill-baseline-1.0.4.sql.gz
sudo curl -fsSL -o baseline/SHA256SUMS \
  https://gitee.com/habeas/cyber-drill/raw/main/deploy/install/baseline/SHA256SUMS
sudo curl -fsSL -o baseline/manifest.env \
  https://gitee.com/habeas/cyber-drill/raw/main/deploy/install/baseline/manifest.env
sudo bash install.sh --image-source swr
```

- `--image-source swr`：华为云 SWR，国内推荐；`--image-source ghcr`：GHCR（国外网络）。不写则自动探测后选择。
- GitHub 备选：把上面 4 个 `https://gitee.com/habeas/cyber-drill/raw/main/...` 换成 `https://raw.githubusercontent.com/chenjinxiang1980/cyber-drill/main/...`。
- 已装 Docker/Compose 时，`install.sh` 只拉镜像、建库、恢复基线、起应用；未装时在线路径会从 openEuler 源安装 `moby-engine` 并下载 compose v2。

### 路径二：离线安装（Release 离线包）

在可联网机器下载，把 tar 包拷到目标机：

```bash
curl -fL -o cyber-drill-offline-1.0.2.tar.gz \
  https://github.com/chenjinxiang1980/cyber-drill/releases/download/v1.0.2/cyber-drill-offline-1.0.2.tar.gz
tar -xzf cyber-drill-offline-1.0.2.tar.gz
cd cyber-drill-offline-1.0.2
sudo bash deploy/install/install.sh --offline-dir "$PWD"
```

- 体积：**472,867,328 字节（约 451 MiB / 473 MB）**。
- 内容：Docker 引擎 RPM（`moby-engine` / `moby-client` 及依赖）、compose v2 二进制、应用镜像 `cyber-drill-1.0.2`、MySQL 8.0 镜像、基线快照与校验清单，以及安装/升级脚本。
- 目标机不需要外网、不需要源码；安装器优先使用包内 RPM、镜像和基线。
- Gitee 发行版页 <https://gitee.com/habeas/cyber-drill/releases> 提供同版本说明；离线包体积较大，附件托管在 GitHub Release。

### 安装后：访问与初始口令

- 浏览器访问 `http://<服务器IP>:8090`（登录页是 `/`）。
- 健康检查：`curl -fsS http://127.0.0.1:8090/api/health`，返回 `"version":"1.0.4"`。
- 初始账号：`admin` / `platformadmin`；口令在安装结束时**只打印一次**，同时写入 `/opt/cyber-drill/.env`（权限 600）。
- **首次登录必须立即修改初始口令**；改密后 `.env` 中的初始口令失效。若已遗失，可在安装器所在目录执行 `sudo bash install.sh --show-secrets`（仅在尚未改密时有效），或直接查看 `.env`。
- 常用参数：`--port 8090`、`--install-dir /opt/cyber-drill`、`--mysql-port 3306`、`--project drill`、`--tls-cert/--tls-key`；完整列表见 [安装文档](docs/install-docker-openeuler.md)。

### 已安装过：重装还是升级

安装器检测到安装目录已存在且非空时会直接停止（退出码 28），不会覆盖数据：

```bash
# A. 不保留数据：删除原目录、容器与数据卷后重装（交互输入 DELETE）
sudo bash install.sh --reinstall

# 非交互环境必须显式提供确认词
sudo bash install.sh --reinstall --confirm DELETE

# B. 保留数据：用升级工具（自动备份 → 迁移 → 重建 app → 校验，失败回滚）
sudo bash deploy/upgrade/upgrade.sh --to 1.0.4 --dir /opt/cyber-drill
```

升级工具不在 `install.sh` 内。用离线包安装的用户直接使用包内 `deploy/upgrade/`；只下载了在线安装器的用户，可从 Gitee 取该目录（8 个文件）：

```bash
sudo mkdir -p /opt/cyber-drill-upgrade/lib && cd /opt/cyber-drill-upgrade
for f in upgrade.sh manifest.json lib/00-common.sh lib/10-manifest.sh \
         lib/20-backup.sh lib/30-migrate.sh lib/40-compose.sh lib/50-rollback.sh; do
  sudo curl -fsSL -o "$f" "https://gitee.com/habeas/cyber-drill/raw/main/deploy/upgrade/$f"
done
sudo bash upgrade.sh --to 1.0.4 --dir /opt/cyber-drill
```

### 镜像源选择

| 参数 | 行为 |
|---|---|
| `--image-source auto` | 默认。自动探测；两者均可达时交互选择，非交互环境按本地时区选择 |
| `--image-source ghcr` | 使用 `ghcr.io/chenjinxiang1980/cyber-drill:<版本>`（国外网络） |
| `--image-source swr` | 使用 `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:<版本>`（国内推荐） |

镜像 tag 取仓库根目录 `VERSION`（当前 `1.0.4`），可用 `--version` 覆盖。

### Windows

Windows 不提供原生一键安装脚本。推荐使用 **Docker Desktop + WSL2**，参考 [`deploy/docker-desktop/README.md`](deploy/docker-desktop/README.md)。不建议在 Windows 本机直接运行编译产物（终端安全软件可能误报拦截）。

---

## 升级

容器部署使用升级脚本，自动完成备份 → 数据库迁移 → 拉取镜像 → 重建应用 → 健康校验，失败自动回滚。以下命令在含 `deploy/upgrade/` 的目录（离线包解包目录或源码仓库）执行；只下载了在线安装器的用户，先按上一节获取升级工具。

```bash
sudo bash deploy/upgrade/upgrade.sh --status
sudo bash deploy/upgrade/upgrade.sh --to 1.0.4 --dry-run
sudo bash deploy/upgrade/upgrade.sh --to 1.0.4 --dir /opt/cyber-drill
```

版本链与升级矩阵见 [`docs/upgrade.md`](docs/upgrade.md) 与 [`deploy/upgrade/manifest.json`](deploy/upgrade/manifest.json)。

---

## 从源码编译（开发者路径，部署不需要）

最终用户部署**不需要** Go、Node.js/npm 或源码；以下仅面向需要自行产出二进制、做二次开发或只升级数据库的开发者。

要求 Go 1.27+、`CGO_ENABLED=0`，目标平台 `linux/amd64`：

```bash
cd server
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
VERSION=$(cat ../VERSION)
go build -trimpath -ldflags "-s -w -X drill-system/internal/version.Version=${VERSION}" -o ../dist/drill-server .
go build -trimpath -ldflags "-s -w -X drill-system/internal/version.Version=${VERSION}" -o ../dist/drill-migrate ./cmd/migrate
go build -trimpath -ldflags "-s -w -X drill-system/internal/version.Version=${VERSION}" -o ../dist/drill-seed ./cmd/seed
```

完整说明（版本注入、交叉编译、只升级数据库）见 [`docs/build-from-source.md`](docs/build-from-source.md)。

---

## 目录结构

```text
Emergency/
├── server/                  # Go 后端（Gin + MySQL）
├── public/                  # 前端静态页面（无构建步骤，由服务端托管）
├── migrations/              # 0000_bootstrap … 0132_s01_end_deps（1.0.4 共 85 条）
├── deploy/
│   ├── install/             # ★ 唯一一键安装入口（install.sh + 基线快照）
│   ├── upgrade/             # 升级脚本与版本清单
│   ├── docker-dist/         # 多阶段镜像构建
│   ├── docker-desktop/      # 跨平台 / 本机开发 compose
│   └── linux/               # 物理机 systemd / nginx / caddy 参考
├── docs/                    # 部署、安全、合规、发版文档与截图
├── scripts/                 # 构建辅助脚本
├── VERSION                  # 版本号单一来源
├── THIRD-PARTY-NOTICES.md   # 第三方组件与许可证
└── LICENSE                  # MIT
```

---

## 文档与常见问题

| 主题 | 入口 |
|---|---|
| 安装（openEuler / Docker） | [docs/install-docker-openeuler.md](docs/install-docker-openeuler.md) |
| 升级与回滚 | [docs/upgrade.md](docs/upgrade.md) |
| 源码编译（开发者路径） | [docs/build-from-source.md](docs/build-from-source.md) |
| Linux / systemd / 反向代理（开发者路径） | [docs/linux-deployment.md](docs/linux-deployment.md) |
| 安全加固与登录策略 | [docs/security-guide.md](docs/security-guide.md) |
| 个人信息处理说明（医院/政府交付） | [docs/privacy-data-processing.md](docs/privacy-data-processing.md) |
| 1.0.0 发版说明与已知限制 | [docs/release-1.0.0.md](docs/release-1.0.0.md) |
| 第三方组件与许可证 | [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) |
| 常见问题、评分与剧本口径、备份恢复 | **Wiki**（见页首链接） |

Wiki 主要页面：安装、升级、配置项、常见问题、评分与剧本口径、备份与恢复。

---

## 许可证

MIT License，版权人：陈金祥（© 2026）。第三方组件许可见 [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md)。
