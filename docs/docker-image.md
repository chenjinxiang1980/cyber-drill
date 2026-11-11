# Drill Server —— Docker 镜像使用文档

> 本镜像为**可分发形态**：用户 `docker pull` + 填写 `.env` 即可起完整系统（app + MySQL）。
> 镜像本身**不含任何凭据** —— 所有密钥/密码必须由使用者自行提供。

---

> **最终用户请直接看 [一键安装（Docker / openEuler）](install-docker-openeuler.md)**：不需要 Go、Node.js/npm，也不需要 clone 源码。
> 本文面向需要手工 compose、自建镜像或发布镜像的运维/开发者；手工方式需要 `deploy/docker-dist/` 下的文件，不属于一键部署路径。

---

## 目录

1. [读者 A0：最终用户（推荐：一键安装，无需源码）](#读者-a0最终用户推荐一键安装无需源码)
2. [读者 A1：手工 compose（可选）](#读者-a1手工-compose可选)
3. [读者 B：要自己构建的人（buildx 多架构）](#读者-b要自己构建的人buildx-多架构)
4. [读者 C：要发布到 Registry 的人](#读者-c要发布到-registry-的人)
5. [镜像内容快照声明（每次发布必填）](#镜像内容快照声明每次发布必填)
6. [安全声明](#安全声明)
7. [架构支持](#架构支持)
8. [国内拉取慢怎么办](#国内拉取慢怎么办)
9. [FAQ / Q&A](#faq--qa)

---

## 读者 A0：最终用户（推荐：一键安装，无需源码）

只需要 x86_64 Linux + Docker 环境，不需要 Go、Node.js/npm，也不需要 clone 源码：

- **在线**：下载约 270 KB 的安装器与基线快照，按镜像源选择 GHCR / 华为云 SWR；
- **离线**：Release 离线包约 451 MiB，解包后一条命令安装。

完整命令、前置条件、初始口令、重装与升级见 [一键安装（Docker / openEuler）](install-docker-openeuler.md)。

应用镜像：`ghcr.io/chenjinxiang1980/cyber-drill:1.0.2` 或 `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.2`；
MySQL 为 `mysql:8.0`（Docker Hub 不可达时安装器自动回退国内镜像源）。

下面的手工方式需要 compose 文件，面向运维定制与开发者，**不是**最终用户的一键部署路径。

---

## 读者 A1：手工 compose（可选）

### 前置条件
- Docker Engine ≥ 20.10
- Docker Compose v2（`docker compose`，不是旧版 `docker-compose`）
- 宿主能拉到 MySQL 镜像（或配置了 mirror）

### 步骤

```bash
# 1) 下载本目录（deploy/docker-dist/）的以下文件到本地任一目录：
#    docker-compose.yml + .env.example

# 2) 准备 .env
cp .env.example .env
# 编辑 .env —— 所有 __REPLACE_WITH__ 占位符必须替换成真实值
# 至少这几个不能留空：DB_USER / DB_PASSWORD / JWT_SECRET / SESSION_SECRET / MYSQL_ROOT_PASSWORD

# 生成 JWT_SECRET / SESSION_SECRET：
#   Linux/macOS:   openssl rand -hex 32
#   Windows(PowerShell): -join ((1..32 | % { $r=Get-Random; '{0:x2}' -f $r }) -join '')

# 3) 启动
docker compose up -d

# 查看状态
docker compose ps
# 等 mysql 变 healthy 后，app 会自动等 DB 就绪 → 跑 migrations → 起服务
# 整个启动约 30~60s（首次要建表）

# 访问 http://localhost:8090
```

### 端口 / 卷 / 网络

| 资源 | 说明 |
|------|------|
| `APP_HOST_PORT` (默认 8090) → 容器 8090 | HTTP 入口 |
| `drill-mysql-data` | MySQL 数据（持久化） |
| `drill-app-data` | `/opt/drill/data`（运行时数据） |
| `drill-app-uploads` | `/opt/drill/uploads`（用户上传） |
| `drill-net` | compose 内部 bridge 网络 |

### 停止 / 清理

```bash
docker compose down          # 停止，保留数据卷
docker compose down -v       # ⚠️ 彻底删除（含数据卷）—— 重置到空库
docker compose pull && docker compose up -d   # 更新镜像
```

---

## 读者 B：要自己构建的人（buildx 多架构）

### 构建

上下文是**仓库根**，Dockerfile 在 `deploy/docker-dist/Dockerfile`。

```bash
# amd64（本地默认）
DOCKER_BUILDKIT=1 docker build \
    -f deploy/docker-dist/Dockerfile \
    -t drill-server:local .

# arm64（buildx 跨平台，Docker Desktop 默认有 QEMU 模拟）
docker buildx build --platform linux/arm64 \
    -f deploy/docker-dist/Dockerfile \
    -t drill-server:local . --load

# 同时产出两个平台的 manifest（推送时用）
docker buildx build --platform linux/amd64,linux/arm64 \
    -t drill-server:local . --push   # --push 才会推到 registry，红线外
```

PowerShell 下可以用 `scripts/build-image.ps1`：

```powershell
.\scripts\build-image.ps1                       # amd64
.\scripts\build-image.ps1 -Arch arm64           # arm64
```

### 构建产物

| 产物 | 来源 | 说明 |
|------|------|------|
| `drill-server` 二进制 | `./server/main.go` | 主程序（静态，CGO_ENABLED=0） |
| `drill-migrate` 二进制 | `./server/cmd/migrate` | 幂等迁移工具（entrypoint 调用） |
| `public/` | 仓库根 | 前端纯静态（HTML+JS），直接 COPY |
| `migrations/*.up.sql` | 仓库根 | 建表 SQL |
| `docker-entrypoint.sh` | `deploy/docker-dist/` | 等 MySQL → 跑迁移 → exec 主程序 |

### 非 root 运行

镜像内主进程 uid=10001（drill），无特权：

```bash
docker run --rm drill-server:local id
# → uid=10001(drill) gid=10001(drill) groups=10001(drill)
```

### 零凭据内置

镜像**不会**写入任何默认密钥/密码。缺失必填配置会导致 `os.Exit(2)`，日志清晰打印缺失项。

---

## 读者 C：要发布到 Registry 的人

**⚠️ 红线：本项目发布流程不默认执行任何 push。** 需要发布时：

1. 你自己 `docker login` 到目标 registry
2. 你自己 `docker tag` + `docker push`
3. 用 `--push` 前确认仓库授权

### GitHub Container Registry (GHCR)

```bash
# 1. 创建 PAT（classic token，选 write:packages 权限）
#    https://github.com/settings/tokens
# 2. 登录
echo "YOUR_GITHUB_PAT" | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
# 3. 构建并推两个架构
docker buildx build --platform linux/amd64,linux/arm64 \
    -t ghcr.io/YOUR_ORG_OR_USER/drill-server:1.0.0 \
    -f deploy/docker-dist/Dockerfile --push .
# 4. 取 digest（推完后）
docker manifest inspect ghcr.io/YOUR_ORG_OR_USER/drill-server:1.0.0 \
    -v | grep digest
```

### 阿里云 ACR

```bash
docker login registry.cn-hangzhou.aliyuncs.com
docker buildx build --platform linux/amd64,linux/arm64 \
    -t registry.cn-hangzhou.aliyuncs.com/YOUR_NAMESPACE/drill-server:1.0.0 \
    -f deploy/docker-dist/Dockerfile --push .
```

### 腾讯云 TCR

```bash
docker login ccr.ccs.tencentyun.com
docker buildx build --platform linux/amd64,linux/arm64 \
    -t ccr.ccs.tencentyun.com/YOUR_NAMESPACE/drill-server:1.0.0 \
    -f deploy/docker-dist/Dockerfile --push .
```

### 华为云 SWR

```bash
# 先在华为云控制台创建组织，然后用临时登录命令
docker login swr.cn-north-4.myhuaweicloud.com
docker buildx build --platform linux/amd64,linux/arm64 \
    -t swr.cn-north-4.myhuaweicloud.com/YOUR_ORG/drill-server:1.0.0 \
    -f deploy/docker-dist/Dockerfile --push .
```

### 建议：固定 digest

发布后建议在 compose 里**把 image 从 tag 改成 digest**，防止供应链攻击或 tag 被覆盖：

```yaml
# 推荐
image: ghcr.io/your-org/drill-server:1.0.0@sha256:a1b2c3d4e5f6...
# 不推荐（tag 可能被重写）
image: ghcr.io/your-org/drill-server:1.0.0
```

---

## 安全声明

> **开源不等于开箱即安全。** 本镜像设计遵循以下原则，但最终安全性取决于部署者：

| 项目 | 状态 | 说明 |
|------|------|------|
| 默认凭据 | ❌ 无 | 镜像不内置任何口令 |
| 非 root 运行 | ✅ | uid=10001，无特权 |
| MySQL root 密码 | 由使用者提供 | MYSQL_ROOT_PASSWORD 必填 |
| HTTPS / TLS | 可选 | app 只监听明文 HTTP，**HTTPS 必须由外层反向代理（nginx/Caddy/Nginx Ingress 等）终止**。详见 `docs/security-guide.md` |
| 访问控制 | 需使用者配置 | compose 里建议注释掉 MySQL 端口暴露；生产建议用 docker network + 内部 DNS |
| 镜像签名 | 未启用 | 可配合 cosign 或 Docker Content Trust（未来） |
| 扫描 | 未默认集成 | 建议部署前用 trivy / grype 扫描 |
| 备份 | 需使用者配置 | `docker run --rm drill-server:local /opt/drill/bin/drill-migrate reset` 有破坏性（0.7.2 起必须同时设置 `DRILL_ALLOW_RESET=1` 并追加 `-yes`），生产前先 `docker run --rm -v drill-mysql-data:/var/lib/mysql mysql:8.0.42 mysqldump ...` |

---

## 架构支持

| 架构 | GOARCH | 支持状态 | 备注 |
|------|--------|----------|------|
| amd64 (x86_64) | `amd64` | ✅ 实测 | Windows Docker Desktop / Linux / macOS x86_64 |
| arm64 (aarch64) | `arm64` | ✅ 构建 | buildx QEMU 模拟可构建；建议在 ARM 物理机上最终验证 |
| loong64 (龙芯 LoongArch) | `loong64` | ⚠️ 未验证 | 理论可行（`CGO_ENABLED=0`），但本环境无龙芯硬件，**不能宣称可用**。用户自行用 `--build-arg TARGETARCH=loong64` 在龙芯机上构建 |

---

## 国内拉取慢怎么办

**只写方法，不写具体内网地址：**

1. **配置 Docker daemon 镜像加速器**：
   - Docker Desktop → Settings → Docker Engine → 加 `registry-mirrors`
   - Linux 编辑 `/etc/docker/daemon.json` → 重启 Docker
2. **buildx 使用代理**：
   ```bash
   export HTTPS_PROXY=http://your-proxy:port
   docker buildx build ... --build-arg HTTPS_PROXY=$HTTPS_PROXY
   ```
3. **MTU 问题（VMware NAT / VPN 下尤其常见）**：
   - 宿主 Docker daemon 加 `--mtu=1400`（或 1300）
   - `docker build` 时报错 `Client.Timeout exceeded` / `connection reset` 基本就是 MTU
   - Windows Docker Desktop 设置里搜 MTU

---

## FAQ / Q&A

### Q1: UOS Server 20 / RHEL 系 Docker 宿主常见坑

| 问题 | 解决 |
|------|------|
| `docker.socket` 与 `docker.service` 冲突导致 `/run/docker.sock` 消失 | `systemctl disable docker.socket && systemctl restart docker` |
| 拉取大镜像卡住 | daemon 加代理 + MTU 1400 |
| `docker-entrypoint-initdb.d` 宿主目录权限不对导致 MySQL 无限重启 | **本方案不用 initdb.d 挂载宿主目录**，entrypoint 内调 drill-migrate 执行迁移，规避此坑 |
| 旧 Docker builder（非 BuildKit）下 `COPY --chmod` 不生效 | **本方案全程不使用** `COPY --chmod` / `COPY --chown`，统一 COPY + RUN chown/chmod |

### Q2: entrypoint 为什么自己跑迁移，不用 MySQL 官方镜像的 initdb.d？

因为把 `migrations/` bind mount 到宿主目录时，宿主目录权限（如 `drill:drill`）不对会让 MySQL 容器无限重启。本方案把 migrations 内嵌到 app 镜像里，由 entrypoint 调用 Go 编的 `drill-migrate`（幂等：查 `schema_migrations` 表，已应用的跳过），彻底绕开宿主目录权限问题。

### Q3: 为什么 app 不直接用 root 连接 MySQL？

compose 里 MySQL 官方镜像通过 `MYSQL_ROOT_PASSWORD` / `MYSQL_USER` / `MYSQL_PASSWORD` 三个环境变量已经自动完成建库建用户，业务账号只有目标库的权限。drill-server / drill-migrate 都用业务账号 `DB_USER:DB_PASSWORD`，**不持有 root 凭据**，符合最小权限原则。

### Q4: `docker compose up` 报错 "配置缺失或未替换占位符"

就是 `.env` 里 `JWT_SECRET` / `SESSION_SECRET` / `DB_USER` 还留着 `__REPLACE_WITH__` 或空值。Go 程序 `config.Load()` 会在启动时严格校验，缺失直接 `os.Exit(2)` —— 这是故意的，**不允许静默降级**。

### Q5: 首次部署不想先建 MySQL 账号怎么办？

那用 bootstrap 子命令：

```bash
# compose 里临时给 mysql 加一个 init 卷，手工跑 bootstrap
# 旧版 bootstrap 路径若要求管理员初始口令，请临时传入自行生成的随机值，
# 不要在文档、脚本或镜像中写入固定口令。
docker compose run --rm -e DB_ROOT_PASSWORD=your_root_pw \
    -e ADMIN_INIT_PASSWORD="$(openssl rand -hex 16)" \
    app /opt/drill/bin/drill-server-bootstrap -root=/opt/drill
```

但一般不需要 —— compose 的 `MYSQL_USER` / `MYSQL_PASSWORD` 已经自动建好了。

---

## 镜像内容快照声明（每次发布必填）

> **为什么要有这一节**：镜像内 `public/` / `migrations/` / `server/` 都直接 COPY 自仓库，一旦发布后仓库又改了，使用者无法知道"拉到的镜像对应哪份源码"。每次发布必须记录下面这份快照，和镜像 tag / digest 一起写入 release notes。

### 快照模板（复制粘贴替换）

```markdown
### Source Snapshot — drill-server:TAG @ DIGEST

- **构建时间**：2026-11-04T10:30:00+08:00
- **Git commit**：$(git rev-parse HEAD) 或 "无（tarball 构建）"
- **构建机**：UOS Server 20 / Linux 6.8.0 / Docker 25.0.4 (legacy builder)
- **代理**：HTTP_PROXY=http://<代理地址>:7890

#### 关键文件 SHA-256（全部小写）

| 文件 | SHA-256 | Size |
|------|---------|------|
| server/internal/router/router.go | CBC3565F... | 14346 |
| server/internal/handler/missing_handlers.go | C8914F22... | 30397 |
| public/screen.html | 040959DA... | 54758 |
| public/index.html | 8133DBAA... | 13596 |
| public/app.js | ED9A3B00... | 31482 |
| migrations/0001_init.up.sql | E7B4C73A... | 18533 |
| migrations/0002_seed_impact.sql | 5375F2C5... | 1157 |
| migrations/0003_login_lock.up.sql | AEC18447... | 691 |

#### 路由注册校验（确认是否包含 XX 改动）

```
# router.go 三处关键行（行号可能变，按内容 grep）：
grep -n 'cleanup-test-accounts\|/content/evaluation\|/content/messages' server/internal/router/router.go
```

#### 前端与现役 VM 对比（若适用）

- VM 现役容器镜像：drill-app（docker-compose: drill-app → ???:latest）
- VM 宿主 bundle/public 与 repo public/ 差异：**UTF-8 BOM / captcha 换色逻辑 / styles.css 版本号**
- 结论：镜像内前端 **包含 / 不包含** 某修复

#### 写入镜像 label

构建时把 revision + version 注入镜像 label（Dockerfile 已声明 OCI label）：

```bash
docker build ... \
    --build-arg VERSION=1.0.0 \
    --build-arg VCS_REF=$(git rev-parse --short HEAD) \
    --build-arg BUILD_TIME="$(date -Iseconds)" \
    --label org.opencontainers.image.revision=$(git rev-parse HEAD) \
    --label drill.system.keyfiles.sha256="$SHA_STRING" \
    -t drill-server:1.0.0 .

# 发布后校验
docker image inspect drill-server:1.0.0 \
    --format '{{json .Config.Labels}}' | python -m json.tool
```

使用者拿到镜像后可以这样核对：

```bash
docker inspect drill-server:1.0.0 \
    --format '{{ .Config.Labels.org.opencontainers.image.revision }}'
```



## 目录结构

```
deploy/docker-dist/
├── Dockerfile                  # 多阶段构建
├── docker-entrypoint.sh        # 等 MySQL → 跑迁移 → exec
├── docker-compose.yml          # app + mysql 两服务
├── .env.example                # 所有必填项 + 占位符
├── .dockerignore                # 仓库根级，排除不该进镜像的东西
├── docs/
│   └── docker-image.md          # 本文件
└── scripts/
    ├── build-image.ps1          # Windows 本地构建
    └── build-image.sh           # Linux/macOS 本地构建
```
