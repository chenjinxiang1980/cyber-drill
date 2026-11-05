# 安装指南（0.5.1）

本文面向 **首次部署** 的运维人员，按推荐顺序覆盖以下路径：

1. **Linux 一键安装（推荐，含容器化）**：自动检测 Docker、选择源码与镜像来源、生成配置并启动；
2. **Docker Compose（跨平台，含 Windows Docker Desktop）**；
3. **离线安装**：离线源码包与离线镜像；
4. **镜像仓库对照**：GHCR 与华为 SWR 的公开状态、登录方式与实测结果；
5. **从源码自主编译安装**：Linux 交叉编译优先，含签名校验、systemd 安装与 Docker 最小构建命令。

> 版本固定为 **0.5.1**，镜像 tag 与 git 标签 `v0.5.1` 均不随安装器变化。

> **为什么推荐 Linux 部署**：Linux 是本项目的原生目标形态（ELF 二进制或 Linux 容器镜像），运行稳定、无终端安全软件干扰。
> Windows 原生编译产物（`.exe`）容易被杀毒软件与安全管控终端（如 360 等）判定为可疑文件并拦截、隔离甚至删除，导致服务无法稳定运行，升级后也可能再次被拦截。这不是“Windows 不能用”，而是原生二进制在企业终端上的误报与运维成本较高。
> 因此 Windows 下推荐**只运行 Docker Desktop 容器**（容器内为 Linux 镜像，不在本机落地 exe），不建议在本机直接运行编译产物。

---

## 一、一键安装

### 1.1 部署平台选择与前置条件

| 项目 | Linux（推荐） | Windows（降级推荐，仅容器） |
|------|---------------|------------------------------|
| 系统 | Debian 10+ / Ubuntu 20.04+ / RHEL 8+ / CentOS 8+ / UOS / Kylin | Windows 10/11 64 位，已启用 WSL2 |
| 部署形态 | ELF 二进制或 Docker 容器 | **仅** Docker Desktop 容器 |
| 原生 exe | 不涉及 | 不推荐：易被杀软 / 安全管控终端拦截 |
| Docker | docker-ce 或发行版自带 docker + Compose v2 插件 | Docker Desktop（含 Compose v2） |
| 资源 | 4 GB 内存、20 GB 磁盘（含镜像与数据卷） | 同上 |
| 网络 | 能访问 GHCR 或 SWR | 同左 |

**最低版本要求**：Docker Engine **20.10+**；必须使用 **Compose v2 插件**（`docker compose` 子命令），旧版 `docker-compose`（v1）不受支持。自检：

```bash
docker version           # Server 版本应 >= 20.10
docker compose version   # 应输出版本号；失败说明缺 v2 插件
```

**如何安装 Docker（摘要；以官方指南为准，勿照搬第三方教程）**

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

Linux 安装后建议：

```bash
sudo systemctl enable --now docker    # 开机自启并立即启动
sudo usermod -aG docker "$USER"       # 加入 docker 组，需重新登录生效
```

> 国内网络拉取慢时，可在 `/etc/docker/daemon.json` 配置 `registry-mirrors`，使用**本单位提供的可信镜像源**；不建议长期依赖来源不明的第三方加速地址。

安装器只做**引导**：检测到未装 Docker 时先展示命令并请求确认，不会静默安装；Windows 侧不会自动下载安装包。

### 1.2 Linux：分步操作（推荐）

**方式 A：本地执行（推荐）**

```bash
git clone --depth 1 https://gitee.com/habeas/cyber-drill.git cyber-drill
cd cyber-drill
bash install/install-linux.sh
```

**方式 B：下载脚本后本地执行**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh -o install-linux.sh
chmod +x ./install-linux.sh
bash ./install-linux.sh
```

**方式 C：`curl | bash`（管道模式没有交互输入，需带参数）**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh \
  | bash -s -- --source gitee --registry ghcr --yes
```

> GitHub 对应地址：
> `https://raw.githubusercontent.com/chenjinxiang1980/cyber-drill/main/install/install-linux.sh`

**默认安装（最省事）**

```bash
bash ./install-linux.sh --yes
```

`--yes` 默认：源码 GitHub、镜像 GHCR、目录 `~/cyber-drill`、应用端口 8090、MySQL 3306、项目名 `drill`、环境文件 `drill.env`。

**自定义端口 + 国内源**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh \
  | bash -s -- \
      --source gitee --registry swr \
      --dir /opt/cyber-drill \
      --port 18090 --mysql-port 13306 \
      --project drill-prod --env-file drill.env \
      --yes
```

**只预览、不改系统**

```bash
bash install/install-linux.sh --dry-run --source gitee --registry ghcr
```

`--dry-run` 只打印将执行的命令（Docker 安装、git clone、docker pull、compose up、健康轮询），不会安装、拉取或启动任何东西。

**安装器会做什么**

| 阶段 | 行为 |
|------|------|
| Docker 检测 | `docker` 命令 + `docker info`；区分“未安装 / 已安装未启动 / 无权限” |
| Docker 安装 | 按 `/etc/os-release` 识别：Debian/Ubuntu 走官方脚本，RHEL/CentOS/UOS/Kylin 走 `dnf/yum install docker`；**先展示命令，确认后执行** |
| Compose 检测 | `docker compose version`；缺失时提示安装 `docker-compose-plugin` |
| 源码 | `git clone --depth 1 --branch main`（版本仍为 0.5.1；可用 `DRILL_REF` 覆盖分支/标签）；无 git 时回退归档下载 |
| 镜像 | GHCR 与 SWR 均公开匿名可拉；SWR 先做匿名 token 协商，失败再提示 AK |
| 配置 | 从 `.env.example` 生成 `drill.env`，缺失密钥随机生成（见 1.5） |
| 启动 | `docker compose -p <project> -f deploy/docker-dist/docker-compose.yml --env-file drill.env up -d` |
| 校验 | 轮询 `http://127.0.0.1:<port>/api/health`，最长 180 秒 |

**安装成功后**

```bash
curl http://127.0.0.1:8090/api/health
# {"ok":true,"service":"cyber-drill-system","version":"0.5.1",...}

# 停止（保留数据卷）
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env down

# 彻底清理（连数据卷一起删除，数据库清空，慎用）
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env down -v
```

默认访问地址 `http://localhost:8090`；管理员账号 `admin`，初始密码首次安装时打印（只显示一次）并写入 `deploy/docker-dist/drill.env`（含数据库密码、`JWT_SECRET`、`SESSION_SECRET`，务必离线备份且不要提交仓库）。

### 1.3 Windows：Docker Desktop 容器方式（降级推荐）

> **Windows 上只使用容器方式**。不要在宿主机直接运行 `server\deploy-linux\` 下的二进制，也不要自行编译 exe 作为交付物：这类原生程序容易被杀毒软件或终端安全管控（如 360）误判并拦截、隔离甚至删除。

**第 1 步：获取源码**

```powershell
git clone --depth 1 https://gitee.com/habeas/cyber-drill.git cyber-drill
cd cyber-drill
```

> 无法访问 Gitee 时换成 GitHub：
> `https://github.com/chenjinxiang1980/cyber-drill.git`

**第 2 步：双击运行（推荐）**

```
install\install-windows.cmd
```

`install-windows.cmd` 内部使用：

```bat
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-windows.ps1"
```

这样可绕过 `ExecutionPolicy` 限制，用户无需改注册表或组策略。

也可在 PowerShell 中直接运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File install\install-windows.ps1
```

**默认安装（最省事）**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install\install-windows.ps1 -Yes
```

`-Yes` 默认：源码 GitHub、镜像 GHCR、目录 `$HOME\cyber-drill`、应用端口 8090、MySQL 3306、项目名 `drill`、环境文件 `drill.env`。

**自定义端口 + 国内源**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install\install-windows.ps1 `
  -Source gitee -Registry swr `
  -Dir D:\cyber-drill -Port 18090 -MysqlPort 13306 `
  -Project drill-prod -EnvFile drill.env -Yes
```

**第 3 步：按提示完成四件事**

1. **Docker 检测**：未安装时展示两条路径（`winget install -e --id Docker.DockerDesktop` 或官网下载），并给出 WSL2 检查结果；已安装但未启动时要求先打开 Docker Desktop。
2. **源码来源**：`1) GitHub` / `2) Gitee` / `3) 已有本地目录`。
3. **镜像来源**：`1) GHCR` / `2) SWR` / `3) 本地镜像`。
4. **部署**：生成 `drill.env` → `docker compose up -d` → 轮询 `/api/health` → 打印访问地址与管理员信息。

预期输出（成功时）：

```
[成功] 服务已启动。
  健康检查：{"ok":true,"service":"cyber-drill-system","version":"0.5.1",...}
  访问地址：http://localhost:8090
  管理员账号：admin
  管理员初始密码（只显示这一次，请立即保存）：<随机值>
  环境文件：...\deploy\docker-dist\drill.env（务必备份）
```

**第 4 步：验证**

浏览器打开 `http://localhost:8090`；或：

```powershell
Invoke-WebRequest http://127.0.0.1:8090/api/health -UseBasicParsing
```

### 1.4 安装器参数速查

两个平台语义一致，写法有差异：Linux 仅长参数；Windows 多数参数同时支持 `--xxx` 与 `-Xxx`（PowerShell 写法），但 **`--image-tar` 与 `--bundle` 只有长参数**。

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--dry-run`（Windows `-DryRun`） | 关 | 只打印命令，不产生任何变更 |
| `--yes`（Windows `-Yes`；Linux 别名 `-y`） | 关 | 跳过交互确认，使用默认值（自动化/CI） |
| `--source github\|gitee\|local` | 交互选择；`--yes` 时 github | 源码来源 |
| `--registry ghcr\|swr\|local` | 交互选择；`--yes` 时 ghcr | 镜像来源 |
| `--dir <路径>` | `~/cyber-drill` | 源码目录 |
| `--port <端口>` | `8090` | 应用宿主端口 |
| `--mysql-port <端口>` | `3306` | MySQL 宿主端口 |
| `--project <名称>` | `drill` | compose 项目名（数据卷前缀） |
| `--env-file <文件名>` | `drill.env` | 环境文件名 |
| `--image-tar <文件>` | 空 | 本地镜像 tar（`docker load`；Windows 无短别名） |
| `--bundle <tar.gz>` | 空 | 离线源码包，按已发布 SHA256 校验（Windows 无短别名） |
| `--update` / `--no-update` | `--update` | 重跑时是否检查远端源码：默认 fetch 后比较 HEAD，一致跳过，不一致增量更新；`--no-update` 只用本地、不联网 |
| `--force-reset`（Windows `-ForceReset`） | 关 | 非 git 目录或本地有未提交改动时，仍强制更新（先备份 `drill.env`） |

环境变量等价：`DRILL_SOURCE`、`DRILL_REGISTRY`、`DRILL_DIR`、`DRILL_PORT`、`DRILL_REF`、`DRILL_UPDATE`。

**重跑安装器的源码更新规则**

- 目录不存在：按 `--source` 下载（git 浅克隆，失败回退归档）。
- 目录存在且是 git 仓库：`git fetch origin <DRILL_REF>` → 比较本地 `HEAD` 与远端 ref。
  - 一致：打印“源码已是最新（`<短哈希>`），跳过下载”。
  - 不一致：备份 `drill.env.bak-<时间戳>` 后 `git reset --hard origin/<ref>`，**`drill.env` 与数据卷保持不变**。
- 目录不是 git 仓库，或本地有已跟踪文件的未提交改动：默认不覆盖；交互模式会询问，非交互（`--yes`）保留本地并跳过；只有 `--force-reset` 才强制更新。
- `--no-update`：不联网，直接使用本地源码；`--dry-run` 会打印 fetch/compare 命令与实际判定结论。

> 并行测试请同时指定 `--project`、`--port`、`--mysql-port`，否则容器名与端口会和现网冲突。

### 1.5 drill.env 与幂等规则

- 文件位置：`deploy/docker-dist/drill.env`（权限 `600`）。
- 首次生成时自动填充：
  - `JWT_SECRET` / `SESSION_SECRET`：32 字节随机（64 位十六进制）
  - `DB_PASSWORD` / `MYSQL_ROOT_PASSWORD`：16 字节随机
  - `ADMIN_INIT_PASSWORD`：交互输入，或直接回车随机生成（**只打印一次**）
- **已存在 drill.env 时绝不覆盖**：交互模式可选“沿用 / 备份后重建 / 退出”；`--yes` 默认沿用。
- **不删除任何数据卷**：重复执行不会执行 `down -v`。
- 脚本内零硬编码凭据；SWR 登录口令只经 `docker login --password-stdin` 传入。

### 1.6 首次启动初始化：创建什么、只删什么

容器入口按顺序执行：等待 MySQL → `drill-migrate up` → `drill-seed` → 启动服务。初始化完全幂等，可重复执行。

| 类别 | 行为 |
|------|------|
| 角色 | upsert **12 个系统角色**（含 `platformadmin`，具备完整权限）；任何情况下都不删除角色 |
| 剧本 | `INSERT IGNORE` 补齐 **8 个标准场景、268 个剧本节点**（S01 共 52 条：18 主流 + 34 混淆）；不覆盖用户已编辑内容 |
| 流程字典 | `INSERT IGNORE` 补齐 **19 个键**（`notif_templates` 站内消息模板、`chat_channels`、`drill_action_meta`、`role_feed_mapping` 等）；只补缺失键，**不覆盖用户已修改的值** |
| 表单模板 | 补齐 8 类模板 |
| 管理员账号 | `admin`、`platformadmin` 不存在时创建，初始密码为 `ADMIN_INIT_PASSWORD`；已存在则保留原密码 |
| 删除范围 | **仅测试账号**；初始化本身不执行任何 `DELETE` / `TRUNCATE` |

**账号清理的正确用法**

- 接口：`POST /api/admin/users/cleanup-test-accounts`，默认 `dryRun=true`，先看清单再执行。
- 只有用户名匹配 `test*/demo*/guest*/tmp*/try*/ceshi*` 的账号可被删除；业务账号（如 `zhangsan`）即使不在白名单也会被保护。
- `ADMIN_BASELINE_USERS`：逗号分隔的额外保护名单；**`admin` 与 `platformadmin` 已内置保护**，无需重复填写。留空即使用默认策略。
- 清理前后都会写日志：删除谁、原因、保留/保护了多少账号。

**已有数据升级**：再次启动只做“补齐缺失”，不会删除或重置自定义角色、场景、节点、字典与账号；升级前建议备份 `drill.env` 与数据库卷。

### 1.7 离线安装

**离线源码（本地目录）**

```bash
bash install/install-linux.sh --source local --dir /opt/cyber-drill --yes
```

目录中需包含 `deploy/docker-dist/docker-compose.yml`（即仓库或解压后的源码包）。

**离线源码包（tar.gz）**

```bash
sha256sum drill-linux-bundle.tar.gz
# 期望 BB448855A4BB9F3F00ACD7803F4A0364C7D9091B936349BE8BB0D16FBA459B71
bash install/install-linux.sh --source local --bundle ./drill-linux-bundle.tar.gz --yes
```

> 该离线包面向 **systemd 安装**（含二进制与 `deploy/linux/install.sh`），不含 `deploy/docker-dist`。使用该包请参考 4.4 的 systemd 步骤；安装器会校验哈希并给出提示。

**离线镜像（docker save）**

```bash
# 有网机器导出
docker save drill-server:0.5.1 | gzip > drill-server-0.5.1.tar.gz

# 离线机器导入并安装
bash install/install-linux.sh --source local --registry local \
  --image-tar ./drill-server-0.5.1.tar.gz --dir /opt/cyber-drill --yes
```

---

## 二、镜像仓库对照

| 仓库 | 地址 | 可见性 | 登录 | 适用网络 |
|------|------|--------|------|----------|
| GHCR | `ghcr.io/chenjinxiang1980/cyber-drill:0.5.1` | **公开，匿名可拉** | 不需要 | 境外 / 有代理 |
| SWR 华为云 | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.1` | **公开，匿名可拉**（2026-11-05 实测） | 不需要；仅在匿名失败时才需 AK | 国内 |
| 本地 | `drill-server:0.5.1` | 本机镜像 | 不需要 | 离线 |

**实测记录（2026-11-05，VM 内标准通道）**

- GHCR 匿名 token + manifest GET：**200**，`docker pull` 成功（2.1s），digest `sha256:c7341909fbb9fcd3a6f79f6ffe19a361d9590f84a5a1bef1e02a105db1fc1e42`
- SWR 匿名 manifest GET：**401 + Bearer challenge**（Registry v2 正常挑战，不等于私有）；用 `realm` 换取匿名 token 后 manifest **200**
- SWR 匿名 `docker pull`（`DOCKER_CONFIG` 指向空目录、无任何凭据）：**成功**（0.6s），digest 与 GHCR 一致
- 安装器的 SWR 分支会先做匿名 token 协商，**200 即直接匿名拉取**；只有拿不到匿名 token 时才提示输入 AK

**SWR 登录（仅当匿名失败）**

```bash
# 口令从环境变量或交互输入，禁止写进命令行参数
printf '%s' "$SWR_PASS" | docker login swr.cn-east-3.myhuaweicloud.com \
  -u 'cn-east-3@<你的AK>' --password-stdin
```

安装器 SWR 分支的三种失败会被区分显示：

| 现象 | 判定 | 处理 |
|------|------|------|
| 连接超时 / DNS 失败 | 网络不通 | 检查代理、安全组、DNS |
| 401 / 403 且换不到匿名 token | 未公开或需鉴权 | 输入 AK 登录，或等仓库公开 |
| HTTP 404 | tag 不存在 | 核对 tag（应为 `0.5.1`） |

---

## 三、常见故障

### 3.1 Docker 相关

| 现象 | 原因 | 处理 |
|------|------|------|
| `docker info` 报 `Cannot connect to the Docker daemon` | 未启动 | Linux：`sudo systemctl enable --now docker`；Windows：启动 Docker Desktop |
| `permission denied ... /var/run/docker.sock` | 用户不在 docker 组 | `sudo usermod -aG docker $USER`，**注销后重新登录** |
| `docker compose` 不存在 | 缺 v2 插件 | `dnf/apt install docker-compose-plugin` |
| Windows 提示 `running scripts is disabled` | ExecutionPolicy | 用 `install-windows.cmd`，或 `powershell -ExecutionPolicy Bypass -File ...` |

### 3.2 部署相关

| 现象 | 原因 | 处理 |
|------|------|------|
| `port is already allocated` | 端口被占 | 改 `--port` / `--mysql-port`，或停掉占用进程 |
| 健康检查 180 秒超时 | MySQL 初始化慢 / 配置错 / 迁移失败 | 看 `docker compose ... logs --tail=100`；核对 `drill.env` 的 `DB_PASSWORD` 与 `MYSQL_ROOT_PASSWORD` |
| 容器反复重启 | `.env` 仍是 `__REPLACE_WITH__` 占位符 | 删除/重建 `drill.env`，或手工替换 |
| 已有 `drill.env` 导致旧配置被沿用 | 幂等保护 | 选择“备份后重建”，或手工改端口/密钥 |
| 空库无管理员账号 | 镜像未内置 bootstrap | 导入既有数据，或用 `server/cmd/bootstrap` 初始化 |

### 3.3 网络与拉取

| 现象 | 原因 | 处理 |
|------|------|------|
| GHCR 拉取超时 | 境外网络受限 | 配代理，或改用 SWR（国内、公开可匿名拉） |
| SWR 401 / 403 | 换不到匿名 token：仓库未公开或 AK 无效 | 按 2 节用 AK 登录 |
| Gitee 归档下载得到 HTML | Gitee 归档需登录 | 改用 `git clone`，或回退 GitHub 归档（安装器会自动回退） |
| `git clone` 失败 | 域名不可达 / 代理 | 改用另一仓库来源，或 `--source local` |

### 3.4 健康检查与回滚

```bash
# 查看状态
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env ps
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env logs --tail=100

# 回滚（保留数据卷）
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env down

# 彻底删除（含数据卷，慎用）
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env down -v
```

---

## 四、从源码自主编译安装（Linux 交叉编译优先）

> 一键安装失败、需要定制、或要做离线交付时使用本节。
> 交付物优先选择 **Linux/amd64** 二进制或容器镜像；不建议把 Windows exe 作为交付物（易被终端安全软件误判拦截）。

### 4.1 前置

- Go **1.22+**（`go version`）
- Node 不需要（前端为静态文件，直接托管 `public/`）
- MySQL 8.x（Docker 方式由 compose 自带）

### 4.2 编译 Linux/amd64 二进制（优先）

优先在 Linux 上直接编译：

```bash
cd server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X drill-system/internal/version.Version=0.5.1" \
  -o deploy-linux/drill-server .
```

没有 Linux 构建环境时，可在 Windows 上交叉编译出**同一个 Linux 产物**：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-linux.ps1
```

脚本行为（`scripts\build-linux.ps1`）：

1. 在 `server/` 下执行 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o deploy-linux/drill-server .`
2. 产物：`server/deploy-linux/drill-server`
3. 自动重写签名文件 `server/deploy-linux/drill-server.sha256.txt`（9 行，末行为小写 SHA256）

只重签、不编译：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-linux.ps1 -SignOnly
```

> 不建议编译或分发 Windows exe：这类原生程序容易被杀毒软件 / 终端管控软件（如 360）误判拦截。Windows 环境请使用 1.3 的 Docker Desktop 容器方式。

### 4.3 签名校验

签名文件末行格式：`<sha256 小写>  drill-server`。

Windows：

```powershell
$expected = (Get-Content server\deploy-linux\drill-server.sha256.txt | Select-Object -Last 1).Split(' ')[0]
$actual   = (Get-FileHash server\deploy-linux\drill-server -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw "签名不匹配" } else { "OK: $actual" }
```

Linux：

```bash
cd server/deploy-linux
expected=$(grep -Eo '^[0-9a-f]{64}' drill-server.sha256.txt | tail -n 1)
actual=$(sha256sum drill-server | awk '{print $1}')
[ "$actual" = "$expected" ] && echo "OK: $actual" || echo "签名不匹配"
```

### 4.4 systemd 安装（无 Docker）

```bash
# 1) 把二进制与部署脚本拷到目标机
scp -r server/deploy-linux/drill-server deploy/linux/install.sh deploy/linux/drill-server.service user@host:/tmp/drill/

# 2) 安装（幂等；只安装不启动）
sudo bash /tmp/drill/install.sh

# 3) 配置（所有 __REPLACE_WITH__ 必须替换）
sudo vi /etc/drill/drill.env

# 4) 启动并验证
sudo systemctl enable --now drill-server
curl http://127.0.0.1:8090/api/health
```

`install.sh` 会创建 `drill` 用户、`/opt/drill` 目录、`/etc/drill` 配置目录，并安装 `drill-server.service`，但**不会自动启动**（等配置就绪）。

### 4.5 Docker 最小构建命令

```bash
DOCKER_BUILDKIT=1 docker build \
  -f deploy/docker-dist/Dockerfile \
  --build-arg VERSION=0.5.1 \
  -t drill-server:0.5.1 .
```

如需构建代理：

```bash
DOCKER_BUILDKIT=1 docker build \
  -f deploy/docker-dist/Dockerfile \
  --build-arg HTTP_PROXY=http://<proxy>:<port> \
  --build-arg HTTPS_PROXY=http://<proxy>:<port> \
  --build-arg VERSION=0.5.1 \
  -t drill-server:0.5.1 .
```

### 4.6 构建后自检清单

```bash
# 1) 版本注入
docker run --rm drill-server:0.5.1 /opt/drill/bin/drill-server --version || true
curl http://127.0.0.1:8090/api/health   # version 应为 0.5.1

# 2) 入口脚本可执行（容器内）
docker run --rm --entrypoint /bin/sh drill-server:0.5.1 -c \
  'ls -l /opt/drill/docker-entrypoint.sh; head -1 /opt/drill/docker-entrypoint.sh | od -c'
# 首行必须是 #!/bin/sh\n，不能出现 \r
```

### 4.7 镜像分发

```bash
docker tag drill-server:0.5.1 <registry>/<org>/cyber-drill:0.5.1
printf '%s' "$REGISTRY_PASS" | docker login <registry> -u <user> --password-stdin
docker push <registry>/<org>/cyber-drill:0.5.1
```

> 不要覆盖已发布的 `0.5.1` 标签，除非明确要替换该版本；常规发版请用新 tag。

---

## 五、相关文档

- [Docker 镜像说明](docker-image.md)
- [Linux 部署](linux-deployment.md)
- [VMware 虚拟机部署](vmware-vm-deployment.md)
- [安全部署指南](security-guide.md)
- [0.5.1 发版说明](release-0.5.1.md)
- [0.5.0 发版说明](release-0.5.0.md)
