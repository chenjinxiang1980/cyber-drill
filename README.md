# 应急演练指挥系统（Cyber-Drill-System）

面向医院 / 企事业单位内网的 **桌面推演 + 实战演练** 指挥调度平台。内置 8 类高频安全事件剧本（勒索病毒、横向渗透、DDoS、钓鱼邮件、存储故障等），支持值班员、指挥员、管理员等多角色协同、剧本时序控制、处置步骤记录、1920×1080 监视大屏、自动考评与复盘报告。

> 当前版本：**0.8.3**（Go 版）
> 许可证：MIT © 2026 陈金祥

---

## 一、为什么重构为 Go（最重要的一节）

### 1.1 问题：纯 JS + HTML 的源码一旦公开，等于把底层业务规则一起交出去

本系统早期采用 “Node.js + 纯静态 HTML/JS” 实现，所有业务逻辑——包括剧本节点编排、评分口径、混淆节点的门控时机、任务下发条件、演练计时基准、站内消息播报规则——都写在可直接阅读、可直接修改的前端脚本里。

源码公开后，任何拿到仓库的人都可以：

- 直接读到每一分的评分公式与扣分规则，**按规则反向构造“满分操作”**；
- 在浏览器里改掉 `app.js` / `scenarios.html` 里的事件处理，**跳过 `before/after` 门控**，让混淆节点提前或延后触发；
- 伪造任务完成请求、绕过“仅演练中才下发任务”的项目状态门控；
- 改写计时起算点，让演练时长与处置时效失真；
- 改掉播报逻辑，让关键告警不播、重复播或只播标题。

换句话说：**前端可读 = 规则可读；前端可改 = 规则可绕过。** 对一套用于考核与复盘的演练系统，这比“代码被抄”更严重——它直接摧毁评分与流程的可信度。

### 1.2 方案：把规则收进服务端编译产物，前端只保留展示层

0.5.0 起，后端整体重构为 **Go（Gin）** 单体服务：

- 剧本编排、节点门控（`before` / `after`）、任务下发状态门控、计时基准（`projects.started_at`）、评分与扣分、消息播报过滤等**规则全部下沉到服务端**；
- 前端只负责渲染与服务端返回结果的展示，**不再持有判定逻辑**；
- 服务端以 Linux ELF / Docker 镜像交付，源码公开也不等于规则可被前端篡改或绕过；
- 同时保留 `node/` 目录作为**历史实现与行为基准**（仅用于对照，不参与部署）。

### 1.3 重构前 / 重构后对比

| 维度 | 重构前（Node + 纯前端 JS） | 重构后（Go 服务端 + 静态前端） |
|------|--------------------------|------------------------------|
| 规则位置 | 剧本编排、评分、门控、计时、播报均在前端脚本 | 全部在服务端编译产物内，前端只展示 |
| 前端可否篡改规则 | 可直接改 JS / 改请求绕过 | 改前端只影响显示，判定仍由服务端执行 |
| 可否绕过评分 | 可读公式、可伪造节点完成 | 评分与节点状态由服务端校验，前端无法自证 |
| 任务下发门控 | 前端按项目状态自行判断 | 服务端按项目状态强制门控 |
| 计时基准 | 前端本地计时，易被改写 | 服务端以 `projects.started_at` 为唯一基准 |
| 部署形态 | Node 运行时 + 依赖树 | 单个 Linux ELF 或 Docker 镜像，零运行时依赖 |

> 结论：**开源的是“系统”，不是“评分答案”。** 规则在服务端，源码公开不会暴露与绕过核心判定。

---

## 二、功能清单

- **演练监视大屏**：1920×1080 大屏，实时操作流、参演角色处置状态、告警与消息三栏滚动，自动重排避免末位错位。
- **角色工作台**：值班员 / 指挥员 / 管理员等角色工作台，处置弹窗、任务清单、业务系统影响态势。
- **剧本编排**：节点化剧本编辑、`before` / `after` 混淆门控、版本号策略（大版本 / 小版本 / 不更新）。
- **演练项目管理**：项目状态机（未开始 / 演练中 / 已暂停 / 已结束等）、基础版 / 实战版、参演单位与观察员。
- **复盘与考评**：按节点时序自动评分、扣分项说明、演练报告导出、时序回放。
- **站内消息**：按项目隔离的站内消息、未读计数、可选的语音播报（按消息 id 去重、只播未读、只播当前项目、只念正文）。
- **会商频道**：演练期间的多角色会商与消息留痕。
- **取证与台账**：处置证据归档、操作台账、知识库与字典。
- **通知与公告**：全局公告与项目消息分流，公告不触发语音播报。

---

## 三、架构与技术栈

```
┌──────────────────────────────────────────────────────┐
│ 浏览器（纯静态 HTML / CSS / JS，无构建步骤）              │
│ index · workbench · screen(1920×1080) · admin · ...   │
│                    REST / JSON                       │
├──────────────────────────────────────────────────────┤
│ Go 后端（Gin）  server/main.go                        │
│ JWT + Session · RBAC · 限流 · 安全头 · 防注入 · 静态托管  │
├──────────────────────────────────────────────────────┤
│ MySQL 8.x（utf8mb4，单库多表）                          │
└──────────────────────────────────────────────────────┘
```

- **后端**：Go 1.22+（当前在 go1.27 工具链下构建），Gin、`database/sql` + MySQL driver；单文件编译，零外部运行时。
- **前端**：原生 HTML + CSS + JavaScript，无打包步骤；由 Go 服务直接托管 `public/`。
- **数据库**：MySQL 8.x，utf8mb4；迁移脚本在 `migrations/`。
- **大屏适配**：1920×1080 基准 + 缩放，手机端响应式布局。

---

## 四、目录结构

```
Emergency/
├── server/                  # Go 后端（当前生产实现）
│   ├── main.go              # 入口：配置、MySQL、路由、HTTP(S)、优雅关闭
│   ├── internal/
│   │   ├── auth/            # JWT + Session + RBAC
│   │   ├── config/          # .env 加载与必填校验
│   │   ├── db/              # MySQL 连接池
│   │   ├── handler/         # /api/* 业务 handler
│   │   ├── middleware/      # 安全头 / CORS / 限流 / BodyLimit
│   │   ├── router/          # 路由组装与安全静态文件服务
│   │   ├── util/
│   │   └── version/         # 版本号（0.8.3）
│   ├── cmd/                 # bootstrap / migrate / verify_* 工具
│   ├── winres/              # Windows 资源清单
│   └── deploy-linux/        # Linux 构建产物与签名文本（二进制不入库）
├── node/                    # 历史 Node 实现（行为基准，不参与部署）
├── public/                  # 前端静态页面（不构建，直接托管）
├── deploy/
│   ├── docker-dist/         # 多阶段 Docker 构建（推荐）
│   ├── linux/               # systemd / compose / nginx / caddy
│   └── docker-desktop/      # 本机开发用 compose
├── migrations/              # 0001_init / 0002_seed_impact / 0003_login_lock
├── scripts/                 # 构建、打包、启动、发版辅助脚本
├── docs/                    # 安全部署指南、Linux / VMware 部署、发版说明
├── .env.example             # 环境变量模板（只有占位符）
├── LICENSE                  # MIT（版权人：陈金祥）
└── README.md
```

> `server/deploy-linux/drill-server` 为交叉编译产物，**不进入仓库**；仓库只保留签名文本 `drill-server.sha256.txt`。

---

## 五、快速开始

> **推荐用 Linux 部署（含容器化）**：Linux 上是原生目标形态，交付物为 ELF 二进制或容器镜像，运行环境稳定、无终端安全软件干扰。
> 相比之下，Windows 原生编译产物（`.exe`）**容易被杀毒软件或安全管控终端（如 360 等）判定为可疑文件并拦截、隔离甚至删除**，导致服务无法稳定运行，且每次升级都可能被重新拦截。这不是“Windows 不能用”，而是原生二进制路径在企业终端上的误报与运维成本较高。
> 因此：**Windows 下推荐只运行 Docker Desktop 容器**（容器内是 Linux 镜像，不在本机落地 exe）；不建议在本机直接运行编译产物。
>
> 推荐顺序：① **Linux 一键安装（推荐）** → ② **Docker Compose（跨平台，含 Windows）** → ③ **离线包** → ④ **从源码编译（Linux 交叉编译优先）**。
> 完整说明（含参数、镜像对照、常见故障、从源码编译）见 [docs/install.md](docs/install.md)。

### 5.1 前置需求：Docker Engine 20.10+ / Compose v2

最低要求：**Docker Engine ≥ 20.10**，且必须使用 **Compose v2 插件**（`docker compose` 子命令）；旧版 `docker-compose`（v1）不受支持。可用 `docker version` 与 `docker compose version` 自检。

| 平台 | 安装方式（摘要） | 权威指南（逐一实测可达） |
|------|------------------|--------------------------|
| Linux（总览） | 按发行版选择官方仓库或脚本 | <https://docs.docker.com/engine/install/> |
| Ubuntu | 官方 apt 仓库安装 `docker-ce` + `docker-compose-plugin` | <https://docs.docker.com/engine/install/ubuntu/> |
| Debian | 官方 apt 仓库安装 | <https://docs.docker.com/engine/install/debian/> |
| RHEL | 官方 yum/dnf 仓库安装 | <https://docs.docker.com/engine/install/rhel/> |
| CentOS | 官方 yum/dnf 仓库安装 | <https://docs.docker.com/engine/install/centos/> |
| 通用脚本 | `curl -fsSL https://get.docker.com \| sh`（仅建议测试环境） | <https://get.docker.com> |
| Compose v2 | 安装 `docker-compose-plugin`，验证 `docker compose version` | <https://docs.docker.com/compose/install/linux/> |
| Windows | 安装 Docker Desktop（启用 WSL2 后端） | <https://docs.docker.com/desktop/setup/install/windows-install/> |
| Windows WSL2 | Docker Desktop 的 WSL2 后端前置说明 | <https://docs.docker.com/desktop/features/wsl/> |

安装完成后建议执行：

```bash
sudo systemctl enable --now docker    # Linux：开机自启并立即启动
sudo usermod -aG docker "$USER"       # Linux：加入 docker 组（需重新登录生效）
docker compose version                # 两个平台都应输出版本号
```

> 国内网络拉取慢时，可在 `/etc/docker/daemon.json` 配置 `registry-mirrors`，使用**本单位提供的可信镜像源**；不建议长期依赖来源不明的第三方加速地址。
> 本项目镜像同时发布在 GHCR 与华为云 SWR（均公开、可匿名拉取），可优先选择网络更通畅的一方。

### 5.2 Linux 一键安装（推荐）

**方式 A：下载脚本后本地执行（可加执行权限）**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh -o install-linux.sh
chmod +x ./install-linux.sh
bash ./install-linux.sh
```

**方式 B：`curl | bash`（管道模式没有交互输入，必须显式传参）**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh \
  | bash -s -- --source gitee --registry ghcr --yes
```

**最省事的默认安装**（`--yes` 时默认：源码 GitHub、镜像 GHCR、目录 `~/cyber-drill`、应用端口 8090、MySQL 3306、项目名 `drill`、环境文件 `drill.env`）：

```bash
bash ./install-linux.sh --yes
```

**自定义端口 + 国内源（完整示例）**：

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh \
  | bash -s -- \
      --source gitee --registry swr \
      --dir /opt/cyber-drill \
      --port 18090 --mysql-port 13306 \
      --project drill-prod --env-file drill.env \
      --yes
```

**Linux 常用参数（与 `install/install-linux.sh --help` 一致）**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--dry-run` | 关 | 只打印将执行的命令，不安装/不拉取/不启动 |
| `--yes`（别名 `-y`） | 关 | 跳过交互确认，使用默认值 |
| `--source github\|gitee\|local` | 交互选择；`--yes` 时 github | 源码来源 |
| `--registry ghcr\|swr\|local` | 交互选择；`--yes` 时 ghcr | 镜像来源 |
| `--dir <路径>` | `~/cyber-drill` | 源码目录 |
| `--port <端口>` | `8090` | 应用宿主端口 |
| `--mysql-port <端口>` | `3306` | MySQL 宿主端口 |
| `--project <名称>` | `drill` | compose 项目名（同时决定容器名与数据卷前缀） |
| `--env-file <文件名>` | `drill.env` | 环境文件名，生成在 `<dir>/deploy/docker-dist/` |
| `--image-tar <文件>` | 空 | 本地镜像 tar（`docker load`） |
| `--bundle <tar.gz>` | 空 | 离线源码包（按内置 SHA256 校验） |
| `--update` / `--no-update` | `--update` | 重跑时检查远端源码并增量更新；`--no-update` 只用本地、不联网 |
| `--force-reset` | 关 | 非 git/有未提交改动时仍强制更新（先备份 drill.env） |

环境变量等价参数：`DRILL_SOURCE`、`DRILL_REGISTRY`、`DRILL_DIR`、`DRILL_PORT`、`DRILL_REF`、`DRILL_UPDATE`。

> 未安装 Docker 时先按发行版展示安装命令，确认后才执行；已装但未启动会尝试 `systemctl enable --now docker`。
> 并行测试务必同时指定 `--project`、`--port`、`--mysql-port`，避免与现网冲突。

### 5.3 Windows 一键安装（Docker Desktop 容器方式）

**方式 A：双击运行（推荐）**

```
install\install-windows.cmd
```

`install-windows.cmd` 内部使用 `powershell -NoProfile -ExecutionPolicy Bypass -File`，无需修改系统 `ExecutionPolicy`。

**方式 B：PowerShell 直接调用**

```powershell
git clone --depth 1 https://gitee.com/habeas/cyber-drill.git cyber-drill
cd cyber-drill
powershell -NoProfile -ExecutionPolicy Bypass -File .\install\install-windows.ps1
```

**默认安装**（`-Yes` 时默认：源码 GitHub、镜像 GHCR、目录 `$HOME\cyber-drill`、应用端口 8090、MySQL 3306、项目名 `drill`、环境文件 `drill.env`）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install\install-windows.ps1 -Yes
```

**自定义端口 + 国内源（完整示例）**：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install\install-windows.ps1 `
  -Source gitee -Registry swr `
  -Dir D:\cyber-drill -Port 18090 -MysqlPort 13306 `
  -Project drill-prod -EnvFile drill.env -Yes
```

**Windows 常用参数（与脚本参数解析一致）**

| 参数（短/长写法） | 默认值 | 说明 |
|-------------------|--------|------|
| `-DryRun` / `--dry-run` | 关 | 只打印将执行的命令 |
| `-Yes` / `--yes` | 关 | 跳过交互确认（**无 `-y` 别名**） |
| `-Source` / `--source` `github\|gitee\|local` | 交互；`-Yes` 时 github | 源码来源 |
| `-Registry` / `--registry` `ghcr\|swr\|local` | 交互；`-Yes` 时 ghcr | 镜像来源 |
| `-Dir` / `--dir` | `$HOME\cyber-drill` | 源码目录 |
| `-Port` / `--port` | `8090` | 应用端口 |
| `-MysqlPort` / `--mysql-port` | `3306` | MySQL 端口 |
| `-Project` / `--project` | `drill` | compose 项目名 |
| `-EnvFile` / `--env-file` | `drill.env` | 环境文件名 |
| `--image-tar`（无短别名） | 空 | 本地镜像 tar |
| `--bundle`（无短别名） | 空 | 离线源码包 |
| `-Update` / `--update` | 开 | 重跑时检查远端源码并增量更新 |
| `-NoUpdate` / `--no-update` | 关 | 只用本地源码，不联网检查 |
| `-ForceReset` / `--force-reset` | 关 | 非 git/有未提交改动时仍强制更新（先备份 drill.env） |
| `-h` / `--help` | — | 显示帮助 |

> **重跑安装器的源码更新规则**（Linux/Windows 行为一致）：目录已存在时先 `git fetch` 并比较本地 `HEAD` 与 `DRILL_REF`（默认 `main`）。一致：打印“源码已是最新（`<短哈希>`），跳过下载”；不一致：先备份 `drill.env.bak-<时间戳>`，再 `git reset --hard origin/<ref>` 增量更新，**`drill.env` 与数据卷保留**。非 git 目录或本地有未提交改动时默认不覆盖，需 `--force-reset`；`--no-update` 纯本地运行；`--dry-run` 打印 fetch/compare 命令与判定结论。

> 未安装 Docker Desktop 时给出 `winget` 与官网两条引导路径（含 WSL2 检查），不会静默安装。
> **Windows 上只使用容器方式**：请勿在宿主机直接运行 `server\deploy-linux\` 下的二进制或自行编译的 exe，这类文件容易被终端安全软件误判拦截。

### 5.4 安装后验证、配置与停止/卸载

**验证（默认端口 8090）**

```bash
curl http://localhost:8090/api/health
# 期望：{"ok":true,"service":"cyber-drill-system","version":"0.8.3",...}
```

Windows PowerShell 可用：

```powershell
Invoke-WebRequest http://127.0.0.1:8090/api/health -UseBasicParsing
```

- **默认访问地址**：`http://localhost:8090`
- **管理员账号**：`admin` 与 `platformadmin`；**初始口令**由安装器为每个账号独立随机生成，在安装成功时由终端打印一次（同时写入环境文件），不会使用统一口令。
- **首次登录**：用初始口令登录后会强制跳转修改密码页，未改密前除改密接口外一律拒绝；改密成功后服务端会自动把环境文件中对应的 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD` 置空并标注失效，避免明文长期留驻。
- **交付客户时**：建议先完成首次改密并确认这两项已置空，再通过安全通道交付环境文件；不要把仍含有效初始口令的文件长期留在交付介质上。
- **环境文件位置**：`<源码目录>/deploy/docker-dist/drill.env`，默认即 `~/cyber-drill/deploy/docker-dist/drill.env`（Windows 为 `$HOME\cyber-drill\deploy\docker-dist\drill.env`）。
- **为什么必须备份**：该文件含数据库密码、MySQL root 密码、`JWT_SECRET`、`SESSION_SECRET` 与管理员初始口令；丢失后难以恢复数据库访问与登录态签名，重建会导致既有会话/密码失效。该文件权限为 `600`（Windows 用 `icacls` 仅保留当前用户），请离线备份，**不要提交到任何仓库**。

**首次启动初始化会创建什么、只删什么**

| 类别 | 行为 |
|------|------|
| 角色 | 幂等 upsert **12 个系统角色**（含 `platformadmin`，权限完整）；永久保留，初始化不删角色 |
| 剧本 | `INSERT IGNORE` 补齐 **8 个标准场景 / 268 个剧本节点**（S01 共 52 条）；只补缺失，不覆盖已编辑内容 |
| 流程字典 | `INSERT IGNORE` 补齐 **19 个字典键**（含 `notif_templates` 站内消息模板、`chat_channels`、`drill_action_meta`、`role_feed_mapping`）；只补缺失键，**不覆盖用户已修改的值** |
| 表单模板 | 补齐 8 类表单模板 |
| 管理员账号 | `admin` 与 `platformadmin` 不存在时创建，初始口令分别取安装器写入的 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD`；已存在则跳过，**绝不覆盖密码** |
| 删除 | 初始化**不删除任何角色、剧本、字典或账号**（不含 `DELETE` / `TRUNCATE`） |

> 账号清理只允许删除**测试账号**：调用 `POST /api/admin/users/cleanup-test-accounts`，仅匹配 `test*/demo*/guest*/tmp*/try*/ceshi*` 的用户名可被删除；`ADMIN_BASELINE_USERS`（逗号分隔）用于追加保护账号，**默认内置保护 `admin` 与 `platformadmin`**，无需重复填写。

**停止 / 卸载（保留数据）**

```bash
cd ~/cyber-drill
docker compose -p drill -f deploy/docker-dist/docker-compose.yml \
  --env-file deploy/docker-dist/drill.env down
```

**彻底清理（连数据卷一起删，数据库将清空，慎用）**

```bash
cd ~/cyber-drill
docker compose -p drill -f deploy/docker-dist/docker-compose.yml \
  --env-file deploy/docker-dist/drill.env down -v
```

> 若安装时改了 `--project` 或 `--dir`，请把上面命令中的 `-p drill`、路径与 `--env-file` 换成实际值。

### 5.5 离线包

- Linux 离线包：`dist/drill-linux-bundle.tar.gz`，SHA256 `BB448855A4BB9F3F00ACD7803F4A0364C7D9091B936349BE8BB0D16FBA459B71`（systemd 安装，步骤见 [docs/install.md](docs/install.md)）。
- 离线镜像：`docker save drill-server:0.8.3` 后用 `install/install-linux.sh --registry local --image-tar <tar>`。

### 5.6 从源码自主编译安装（Linux 交叉编译优先）

优先产出 **Linux/amd64** 二进制（在 Linux 本机编译，或在 Windows 上交叉编译）；不建议把 Windows exe 作为交付物分发。完整步骤（编译、签名校验、systemd、`docker build`）见 **[docs/install.md](docs/install.md)** 第四节。

更完整的部署前检查表、HTTPS、代理、国产化基础镜像说明见：

- [docs/install.md](docs/install.md)
- [docs/linux-deployment.md](docs/linux-deployment.md)
- [docs/security-guide.md](docs/security-guide.md)
- [docs/vmware-vm-deployment.md](docs/vmware-vm-deployment.md)
- [docs/docker-image.md](docs/docker-image.md)

---

## 六、安全默认值与首次部署必改项

**仓库不预置任何可用凭证。** `.env.example` 中所有敏感项均为 `__REPLACE_WITH__` 占位符；服务启动时严格校验，缺失直接 `exit 2`，不静默降级。

| 必改项 | 说明 | 设置方式 |
|--------|------|----------|
| `JWT_SECRET` | JWT 签名密钥 | `openssl rand -hex 32` |
| `SESSION_SECRET` | Session 签名密钥 | `openssl rand -hex 32` |
| `DB_PASSWORD` / `MYSQL_ROOT_PASSWORD` | 数据库账号口令 | 自行设置强口令 |
| `CAPTCHA_ENABLED` | 登录图形验证码 | 公网部署保持 `true` |
| `ALLOW_ORIGINS` | CORS 白名单 | 生产改为明确域名，勿留 `*` |
| `TLS_ENABLED` + 证书 | HTTPS | 公网部署必须开启并挂载真实证书 |

> `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD` 无需手工设置：一键安装器会为两个管理员账号各生成独立随机口令并写入环境文件；手工部署留空即可，`drill-seed` 会随机生成并只在启动日志中输出一次。首次登录必须改密，改密后这两个值会被自动清空。

内置安全能力：

- **鉴权 / RBAC**：JWT + 服务端 Session 校验，角色分组鉴权（`server/internal/auth`）。
- **登录保护**：图形验证码（一次性、TTL 可配）+ 账号失败锁定 + 登录分组限流。
- **限流**：全局限流与 `/api/auth` 分组限流（`golang.org/x/time/rate`）。
- **防注入 / XSS / 越权**：参数化 SQL、输出转义、服务端角色校验；静态文件服务拒绝 `..` 与隐藏文件。
- **安全响应头 / BodyLimit**：`middleware.SecurityHeaders`、请求体大小上限。
- **默认零凭证**：未配置真实密钥时拒绝启动。

> 本项目不含 WebSocket 通道；实时刷新由前端轮询 REST 接口完成，因此不存在独立 WS 鉴权面。若后续引入 WS，需复用同一 JWT/Session 校验。

---

## 七、免责声明与合规使用

1. 本项目仅用于**授权的网络安全应急演练、教学与内部培训**。请勿用于未经授权的攻击、探测或任何违法用途。
2. 演练剧本中的攻击描述、节点与话术均为**模拟内容**，不构成真实攻击工具；使用者应确保演练环境与生产环境隔离。
3. 部署方对 `.env`、数据库、证书、账号口令的保管负全部责任；因配置不当（弱口令、公网暴露、关闭验证码等）造成的损失由部署方承担。
4. 本项目按 MIT 许可证“按原样”提供，作者不对可用性、适用性或任何直接/间接损失作出担保。
5. 使用本项目即表示你已阅读并同意上述条款，并遵守所在国家/地区的法律法规。

---

## 八、版本与更新日志（0.8.3）

### 0.8.3（2026-11-08）

**评分口径与证据链**

- 实战版单节点扣分封顶 80%：超时 20%、接收人错误 30%、关键词必含项缺失按比例最多 30%、文本质量 20%、证据未提交或未命中 20%，各项独立计算后合计不超过节点分值的 80%。
- 接收人判定拆分为主送 / 抄送（migration `0083`）：`receiver` 为主送，必须全部命中，缺任一扣 30%；`receiver_cc` 为可选抄送，为空不扣分，非空且一个未选扣 10%。
- 证据链对齐结构化归档（migration `0082`）：`evidence` 增加 `sha256`、`custodian`、`sealed_at`、`chain_of_custody`，评分优先按 `evidence.req_key = 节点ID#序号` 命中，F13 填报哈希可与归档文件实际哈希比对。
- 回执节点降权（migration `0081`）：7 个回执节点由 5–7 分统一降到 2 分，未完成只扣 2 分，不再因外部回执不可控而重罚；S04-12 / S08-12 改为「报案必要性评估单」（F19）。
- 超时计时改为实战版按「前驱完成 / 任务下发时刻 + 节点 timeout，扣除暂停时长」实时判定，旧 `ontime` 值仅作兜底；基础版维持超时扣本节点 20%、结束未完成扣全部分值。
- `receiverOk` 改为三态：实战版为 true/false，基础版与未执行节点为 null，避免前端把基础版恒 false 误读为接收人全错。

**剧本与前端**

- 剧本版本统一为 `v2.0`，5 条「不报案 / 内部处理」干扰项按各剧本真实易犯错误差异化重写（migration `0090`）：S02 封号改密后不报案、S03 只让运营商清洗、S04 钓鱼即按Ⅰ级停诊、S05 硬件故障只报厂商、S08 业务未停不评估报案。
- 剧本编排 SVG 布局：自动布局改为「列=拓扑深度、列内按前驱/后继重心四轮排序、垂直居中」，列间连线控制点限制在列间空隙、同列/回连线从右侧绕行，避免穿过卡片；新增「适应画布」，首次打开的 `(0,0)` 占位坐标自动兜底并使拖拽后不再回弹。
- 复盘与导出合并：演练报告导出并入「复盘评估」页，按权限显示导出按钮；修复导出文件名与审计落库，导出动作统一写审计日志。
- 登录页角色卡改为纯展示并带职责面板；大屏节点进度隐藏滚动条自动滚动、中间态势面板毛玻璃与离线提示；基础版表单「只提示不拦」并标注模拟数据。

**构建与发布**

- 迁移新增 `0081`–`0083`、`0090` 共 4 个，迁移总数 43 → 47，末位 `0090_distractor_rewrite_v2`。
- 版本、品牌默认版本（系统设置显示 `v0.8.3`）与安装器统一为 `0.8.3`；镜像 `0.8.3` 与 `latest` 发布到 GHCR 与华为云 SWR。

### 0.8.2（2026-11-07）

**修复**

- 评分与考评页口径统一：扣分明细按节点真实得分计算，页面说明与后端 `Score.deductions` 一致，不再出现「页面写扣 10 分、实际按比例扣」的偏差。
- 导出与报告链路修复：报告数据取自项目、任务、时序、评分与取证的真实数据，导出失败时给出明确提示；证据完整率与核心证据完整率分开展示。
- 大屏与工作台细节修正：节点操作流与消息列对齐、超时节点纳入统计、刷新不叠加。

**构建与发布**

- 版本、品牌默认版本与安装器统一为 `0.8.2`；镜像 `0.8.2` 作为回滚点保留。

### 0.8.1（2026-11-07）

**剧本与业务**

- 新增 4 个独立节点：S02/S06/S08「等级升级研判」（影响扩大时按新等级续报）与 S03「运营商协同」拆分节点，节点总数 349 → 353，主流节点 129 → 133；依赖改为「研判 → 升级研判 → 报告」单链，无环。
- S04/S08 报告时限由 120 小时调整为 240 小时；S03-8A 接收人改为运营商，三个升级研判节点的证据要求补齐事实依据与续报回执。
- `script_nodes` 支持软删（`deleted_at`），全链路查询过滤已删节点；编排页新增「回收站」，可列出并恢复节点，恢复写入审计。
- 节点数口径拆分：`nodes` 为全量（含混淆，用于保存与一致性校验）、`displayNodes` 为主流展示口径、`distractors` 为混淆数；剧本库列表、编排页统计、保存提示与报告统一。

**交互**

- 基础版表单「只提示不拦」：提交照常落库，响应返回 `warnings`；正式版仍返回 400 与 `reasons`。
- 登录页角色卡改为纯展示：不再选中或带入账号，悬停 / Tab 聚焦 / 点击查看职责面板，Esc 取消固定。
- 大屏节点处置进度隐藏滚动条、超限自动滚动、悬停暂停、刷新不叠加。

**构建与发布**

- 迁移 0070–0073、0080 共 5 个，迁移总数 38 → 43；版本与安装器统一 `0.8.1`，库内 `settings.brand_version` 同步 `v0.8.1`。
- 保留 `0.8.0` 镜像作为回滚点。

### 0.8.0（2026-11-07）

**新增 / 修复**

- **评分引擎**：加分词不再参与证据文本匹配，且加分抵扣设置上限（缺失必填扣分不得被加分归零），避免「缺报送对象 + 证据含回执」被白拿满分。
- **业务表单**（migration 0050–0052）：F18 结束条件扩至五项并补齐网络/邮件/终端业务恢复选项；F13 证据保全新增逐条明细、证据编号、保管人、移交记录与哈希算法；S06-12 补齐个人信息保护影响评估与三类证据。
- **工程 P1**（migration 0060–0061）：节点计分唯一索引与重复步骤去重、剧本版本历史与回滚接口、基础版表单必填校验、账号/角色/剧本保存/审计导出审计留痕、恢复期维护模式 503、备份附件归档与 manifest、`migrate reset` 双重确认、`drill-progress` 真实进度。
- **监视大屏**：修复首屏偶发不显示（请求 12 秒超时 + 递增重试）、超时完成节点纳入统计、操作流去掉来源板块、语音播报在切换窗口/后台时不再中断。

**构建与发布**

- 版本、品牌默认版本（系统设置显示 `v0.8.0`）与安装器统一为 `0.8.0`；现网库内 `settings.brand_version` 同步改为 `v0.8.0`。
- 镜像内含 0001–0061 全部迁移共 38 个文件；保留 `0.7.1` 镜像作为回滚点。

### 0.7.1（2026-11-07）

**修复 / 加固**

- **S01-8B 质量规则**（migration 0041）：为「向市网信办报送事件（1 小时内）」补齐节点级 `quality_rules`，必含报送对象（网信办）、事件基本情况、已采取措施三组关键词，加分项为 1 小时内 / 回执 / 研判；纯套话与缺报送对象的文本不再白拿该 6 分节点。
- **同秒登录 500**：JWT 增加 128 位随机 `jti`，同一账号同秒多次签发不再生成相同令牌，`sessions.token` 主键冲突与改密后立刻重登报 500 的问题消除。
- **静态资源权限归一化**：安装脚本固定 `umask 022` 并将 `public/` 目录与文件归一到 755/644，同步脚本上传后同样归一，避免严格 umask 下 640 导致页面 403。

**构建与发布**

- 版本、品牌默认版本（系统设置显示 `v0.7.1`）与安装器统一为 `0.7.1`；现网库内 `settings.brand_version` 同步改为 `v0.7.1`。
- 镜像内含 0001–0041 全部迁移共 33 个文件，现网升级自动应用 0041。

完整说明见 [docs/release-0.7.1.md](docs/release-0.7.1.md)。

### 0.7.0（2026-11-07）

**新增 / 增强**

- **结束封存**（migration 0020–0022）：新增 `project_core_evidence`、`project_end_seal`、`project_end_exception`、`score_snapshots` 四张表，结束演练时固化核心证据与评分快照；封存后状态接口不再绕过，仅可按例外流程留痕处理。
- **缺项结束与强制结束**：结束前核对必填项与证据链，缺项可登记缺项原因后结束；强制结束按权限留痕，timeline 记录「演练结束」。
- **取证与备份**：取证材料上传落盘并可下载，支持抓屏入库与 `requirements`/`prefill`；新增真实导出与上传恢复，恢复失败自动回滚（P0-1 / P0-2）。
- **剧本业务一致性**（migration 0030–0039）：S01 拆分为市卫健委 / 市网信办双报节点；结束依赖双报并行；节点指标与评分文案取真实扣分明细；上报时限、启动审批、表单绑定、接收方语义、岗位职责、干扰项与表单升级口径统一；节点数改为含干扰项/终止项。
- **安全加固**：`/uploads` 改为鉴权访问（匿名大屏走只读白名单）；安装期为主管账号各自生成随机初始口令；首次改密成功后失效旧会话；表单提交增加下拉值白名单与请求体 UTF-8 校验；迁移器加 MySQL advisory lock 并将迁移 SQL 与版本行收进同一事务。

**构建与发布**

- 版本、品牌默认版本（系统设置显示 `v0.7.0`）与安装器统一为 `0.7.0`。
- 镜像内含 0001–0039 全部迁移，新环境首次启动自动建表；现网升级自动应用 0020–0022、0030–0039 共 13 个文件。

完整说明见 [docs/release-0.7.0.md](docs/release-0.7.0.md)。

### 0.6.0（2026-11-06）

**新增 / 增强**

- 处置文字质量规则下沉到节点级（migration 0011）：`quality_rules` 支持必含/加分关键词、最小字数、要素要求、敷衍词与扣分比例；关键词只匹配文本类字段，表单填写值不再参与匹配。
- 匹配容错：全半角、空白、大小写与数字体系统一归一化；必含关键词同义词组；敷衍短文本在提交接口直接 400。
- 证据要求为 `[]` 或空串的节点不再无条件扣 20%；老节点无 `quality_rules` 时按默认口径评分。
- 表单模板展示编号三位补零（migration 0012，`F001…`），修复系统设置模板列表与「关联表单」下拉排序错乱；模板保存改为兼容后端仅返回 `{ok:true}`，不再因读 `r.template.id` 崩溃。
- 大屏：语音播报开关、匿名观摩接口收敛；工作台处置弹窗左右分栏并修正前置节点展示；剧本编排与管理页交互修正。

**构建与发布**

- 版本、品牌默认版本（系统设置显示 `v0.6.0`）与安装器统一为 `0.6.0`。
- 镜像 `0.6.0` 与 `latest` 同步发布到 GHCR 与华为云 SWR，公开匿名可拉。

完整说明见 [docs/release-0.6.0.md](docs/release-0.6.0.md)。

### 0.5.1（2026-11-06）

**新增**

- S01 剧本内容、记录表单与评分口径改造；新增外部指导意见记录单与 S01 专用记录单。
- 业务影响联动与审议模式（migration 0008）。
- 监视大屏免登录只读观摩（匿名限流）；站内消息播报收敛到大屏，工作台仅提示新消息。
- 混淆节点按严重程度分档扣分（高 8 / 中 5 / 低 3），评分改为关键词与证据口径。
- Windows / Linux 一键安装器（`install/`），支持源码与镜像来源选择、compose 项目隔离。

**修复**

- 脚本编排保存按 id 增量更新，避免整表删除重建覆盖节点；逐字段容错解析并修正 seq 重排。
- 业务影响更新 SQL 的 MySQL 1093 自引用错误；F7 恢复确认单按「状态」后缀识别业务系统。
- migration 0010 修正 S01-9 / S01-10 执行顺序；初始化幂等补齐角色/剧本/字典，不再删除角色。
- 实战版演练中锁定脚本并支持强制查看扣分；登录页不再启动通知轮询与顶栏时钟。

**构建与发布**

- 镜像升级到 `0.5.1`（`latest` 同步），GHCR 与华为云 SWR 双仓库发布，公开匿名可拉。
- `.gitattributes` 强制 shell 脚本 LF，修复容器入口 CRLF 导致 tini exec 失败。

完整说明见 [docs/release-0.5.1.md](docs/release-0.5.1.md)。

### 0.5.0（2026-11-05）

**新增**

- Go 版服务端首发：规则、评分、门控、计时全部下沉服务端。
- `/api/health` 增加 `version` 字段；Docker 构建注入 `internal/version`。
- Linux 离线部署包与 SHA256 验签流程（`scripts/pack-linux-bundle.ps1`）。

**修复**

- 演练计时基准统一为项目开始时刻 `projects.started_at`，大屏 / 看板 / 工作台一致重算；启动演练真正落库。
- 监视大屏三处滚动模块末位错位修复。
- 角色工作台处置弹窗仅允许“提交成功自动关闭”和“× 关闭”。
- 基础版任务按项目状态门控，仅“演练中”下发。
- 站内消息语音播报：按消息 id 去重、只播未读、只播当前项目、全局公告不播、只念正文。
- 角色工作台“业务系统影响态势”重复行合并去重。
- 打包脚本改为读取签名文件末行哈希；`build-linux.ps1` 构建后自动重签。
- 删除项目时清理关联数据。

**安全**

- 仓库零预置凭证；`.env.example` 全占位符，启动强校验。
- `.gitignore` 覆盖 `.env*`、证书私钥、构建二进制、归档包、备份、日志与临时目录。
- mysqldump / mysql CLI 调用改用 `MYSQL_PWD` 环境变量，避免口令出现在进程命令行。
- 静态文件服务路径穿越与隐藏文件防护。
- 文档与脚本中的内网地址、真实账号信息已脱敏；发布二进制不入库。

完整说明见 [docs/release-0.5.0.md](docs/release-0.5.0.md)。

---

## 九、License 与致谢

- 许可证：**MIT**，版权人 **陈金祥**，见 [LICENSE](LICENSE)。
- 致谢：感谢所有参与演练脚本设计、内网部署验证与安全评审的同事；感谢 Gin、MySQL 及开源社区。

---

## English Summary

Cyber-Drill-System is an intranet tabletop/field exercise command platform for hospitals and enterprises. In v0.5.0 the backend was rewritten in Go so that **scoring rules, node gating, task gating, timing and notification rules live in the server binary**, not in publicly readable frontend scripts. The frontend remains plain static HTML/CSS/JS and is display-only. Requires Go 1.22+ and MySQL 8.x, or Docker. No credentials are preconfigured. Licensed under MIT © 2026 Chen Jinxiang.
