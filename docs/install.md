# 安装指南（0.5.0）

本文面向 **首次部署** 的运维人员，覆盖三条路径：

1. **一键安装（推荐）**：Windows / Linux 自动检测 Docker、选择源码与镜像来源、生成配置并启动；
2. **镜像仓库对照**：GHCR 与华为 SWR 的公开状态、登录方式与实测结果；
3. **从源码自主编译安装**：Go 交叉编译、签名校验、systemd 安装与 Docker 最小构建命令。

> 版本固定为 **0.5.0**，镜像 tag 与 git 标签 `v0.5.0` 均不随安装器变化。

---

## 一、一键安装

### 1.1 前置条件

| 项目 | Windows | Linux |
|------|---------|-------|
| 系统 | Windows 10/11 64 位，已启用 WSL2 | Debian 10+ / Ubuntu 20.04+ / RHEL 8+ / CentOS 8+ / UOS / Kylin |
| Docker | Docker Desktop（含 Compose v2） | docker-ce 或发行版自带 docker + compose v2 插件 |
| 资源 | 4 GB 内存、20 GB 磁盘（含镜像与数据卷） | 同上 |
| 网络 | 能访问 GHCR 或 SWR | 同左 |

安装器只做**引导**：检测到未装 Docker 时先展示命令并请求确认，不会静默安装；Windows 侧不会自动下载安装包。

### 1.2 Windows：分步操作

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

**第 3 步：按提示完成四件事**

1. **Docker 检测**：未安装时展示两条路径（`winget install -e --id Docker.DockerDesktop` 或官网下载），并给出 WSL2 检查结果；已安装但未启动时要求先打开 Docker Desktop。
2. **源码来源**：`1) GitHub` / `2) Gitee` / `3) 已有本地目录`。
3. **镜像来源**：`1) GHCR` / `2) SWR` / `3) 本地镜像`。
4. **部署**：生成 `drill.env` → `docker compose up -d` → 轮询 `/api/health` → 打印访问地址与管理员信息。

预期输出（成功时）：

```
[成功] 服务已启动。
  健康检查：{"ok":true,"service":"cyber-drill-system","version":"0.5.0",...}
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

### 1.3 Linux：分步操作

**方式 A：本地执行（推荐）**

```bash
git clone --depth 1 https://gitee.com/habeas/cyber-drill.git cyber-drill
cd cyber-drill
bash install/install-linux.sh
```

**方式 B：`curl | bash`（管道模式没有交互输入，需带参数）**

```bash
curl -fsSL https://gitee.com/habeas/cyber-drill/raw/main/install/install-linux.sh \
  | bash -s -- --source gitee --registry ghcr --yes
```

> GitHub 对应地址：
> `https://raw.githubusercontent.com/chenjinxiang1980/cyber-drill/v0.5.0/install/install-linux.sh`

**方式 C：只预览、不改系统**

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
| 源码 | `git clone --depth 1 --branch main`（版本仍为 0.5.0；可用 `DRILL_REF` 覆盖分支/标签）；无 git 时回退归档下载 |
| 镜像 | GHCR 与 SWR 均公开匿名可拉；SWR 先做匿名 token 协商，失败再提示 AK |
| 配置 | 从 `.env.example` 生成 `drill.env`，缺失密钥随机生成（见 1.5） |
| 启动 | `docker compose -p <project> -f deploy/docker-dist/docker-compose.yml --env-file drill.env up -d` |
| 校验 | 轮询 `http://127.0.0.1:<port>/api/health`，最长 180 秒 |

**安装成功后**

```bash
curl http://127.0.0.1:8090/api/health
# {"ok":true,"service":"cyber-drill-system","version":"0.5.0",...}

# 停止（保留数据卷）
docker compose -p drill -f deploy/docker-dist/docker-compose.yml --env-file deploy/docker-dist/drill.env down
```

### 1.4 安装器参数速查

两个平台参数语义一致（Windows 用 `--xxx` 或 `-Xxx`）：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--dry-run` | 关 | 只打印命令，不产生任何变更 |
| `--yes` | 关 | 跳过交互确认，使用默认值（自动化/CI） |
| `--source github\|gitee\|local` | 交互选择；`--yes` 时 github | 源码来源 |
| `--registry ghcr\|swr\|local` | 交互选择；`--yes` 时 ghcr | 镜像来源 |
| `--dir <路径>` | `~/cyber-drill` | 源码目录 |
| `--port <端口>` | `8090` | 应用宿主端口 |
| `--mysql-port <端口>` | `3306` | MySQL 宿主端口 |
| `--project <名称>` | `drill` | compose 项目名（数据卷前缀） |
| `--env-file <文件名>` | `drill.env` | 环境文件名 |
| `--image-tar <文件>` | 空 | 本地镜像 tar（`docker load`） |
| `--bundle <tar.gz>` | 空 | 离线源码包，按已发布 SHA256 校验 |

环境变量等价：`DRILL_SOURCE`、`DRILL_REGISTRY`、`DRILL_DIR`、`DRILL_PORT`。

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

### 1.6 离线安装

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

> 该离线包面向 **systemd 安装**（含二进制与 `deploy/linux/install.sh`），不含 `deploy/docker-dist`。使用该包请参考 3.3 的 systemd 步骤；安装器会校验哈希并给出提示。

**离线镜像（docker save）**

```bash
# 有网机器导出
docker save drill-server:0.5.0 | gzip > drill-server-0.5.0.tar.gz

# 离线机器导入并安装
bash install/install-linux.sh --source local --registry local \
  --image-tar ./drill-server-0.5.0.tar.gz --dir /opt/cyber-drill --yes
```

---

## 二、镜像仓库对照

| 仓库 | 地址 | 可见性 | 登录 | 适用网络 |
|------|------|--------|------|----------|
| GHCR | `ghcr.io/chenjinxiang1980/cyber-drill:0.5.0` | **公开，匿名可拉** | 不需要 | 境外 / 有代理 |
| SWR 华为云 | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.0` | **公开，匿名可拉**（2026-11-05 实测） | 不需要；仅在匿名失败时才需 AK | 国内 |
| 本地 | `drill-server:0.5.0` | 本机镜像 | 不需要 | 离线 |

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
| HTTP 404 | tag 不存在 | 核对 tag（应为 `0.5.0`） |

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

## 四、从源码自主编译安装

> 一键安装失败、需要定制、或要做离线交付时使用本节。

### 4.1 前置

- Go **1.22+**（`go version`）
- Node 不需要（前端为静态文件，直接托管 `public/`）
- MySQL 8.x（Docker 方式由 compose 自带）

### 4.2 交叉编译 Linux/amd64 二进制

在 Windows 上：

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

在 Linux 上直接编译：

```bash
cd server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X drill-system/internal/version.Version=0.5.0" \
  -o deploy-linux/drill-server .
```

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
  --build-arg VERSION=0.5.0 \
  -t drill-server:0.5.0 .
```

如需构建代理：

```bash
DOCKER_BUILDKIT=1 docker build \
  -f deploy/docker-dist/Dockerfile \
  --build-arg HTTP_PROXY=http://<proxy>:<port> \
  --build-arg HTTPS_PROXY=http://<proxy>:<port> \
  --build-arg VERSION=0.5.0 \
  -t drill-server:0.5.0 .
```

### 4.6 构建后自检清单

```bash
# 1) 版本注入
docker run --rm drill-server:0.5.0 /opt/drill/bin/drill-server --version || true
curl http://127.0.0.1:8090/api/health   # version 应为 0.5.0

# 2) 入口脚本可执行（容器内）
docker run --rm --entrypoint /bin/sh drill-server:0.5.0 -c \
  'ls -l /opt/drill/docker-entrypoint.sh; head -1 /opt/drill/docker-entrypoint.sh | od -c'
# 首行必须是 #!/bin/sh\n，不能出现 \r
```

### 4.7 镜像分发

```bash
docker tag drill-server:0.5.0 <registry>/<org>/cyber-drill:0.5.0
printf '%s' "$REGISTRY_PASS" | docker login <registry> -u <user> --password-stdin
docker push <registry>/<org>/cyber-drill:0.5.0
```

> 不要覆盖已发布的 `0.5.0` 标签，除非明确要替换该版本；常规发版请用新 tag。

---

## 五、相关文档

- [Docker 镜像说明](docker-image.md)
- [Linux 部署](linux-deployment.md)
- [VMware 虚拟机部署](vmware-vm-deployment.md)
- [安全部署指南](security-guide.md)
- [0.5.0 发版说明](release-0.5.0.md)
