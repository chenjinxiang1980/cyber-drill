# 重启前检查表（Docker Desktop 启用 WSL2/Hyper-V 大概率需要重启整机）

> **重启的原因**：启用 WSL2 或 Hyper-V 属于 Windows 功能级变更，99% 概率触发重启。重启后：
> 1. 依赖内存映像的 Go 进程会丢失（WMI 启动的 Go exe 随内存释放而消失）；
> 2. Go exe 落盘后可能再次被 360 QVM 运行时拦截（上次就是因为这个才转 Linux ELF）；
> 3. 你需要在 **Docker Desktop 就绪后** 才能决定是否要把 Go 搬进容器。
>
> 本清单帮助你在重启前把当前状态完整固化下来，不丢数据、不丢 PID、不丢凭据。

---

## 清单（按顺序，**每步必须做**）

| # | 动作 | 怎么做 | 关键产物 |
|---|------|--------|----------|
| **0** | **确认当前 Go 服务在跑** | 浏览器打开 `http://127.0.0.1:8090/api/health`，应返回 `ok:true` | — |
| **1** | **备份 MySQL 数据库** | ```powershell<br/>mysqldump -u drill_app -p drill_system \|<br/> Out-File -Encoding utf8 backup\mysql-pre-reboot-$(Get-Date -f yyyyMMdd-HHmmss).sql<br/>``` | `backup/mysql-pre-reboot-YYYYMMDD-HHMMSS.sql` |
| **2** | **备份 SQLite（如果历史用的 Node 版也在跑）** | ```powershell<br/>Copy-Item data\drill.db backup\drill.db-pre-reboot-$(Get-Date -f yyyyMMdd-HHmmss).db<br/>``` | `backup/drill.db-pre-reboot-*.db` |
| **3** | **备份 Go 启动期数据** | 如果 `data\backups\` 里有 Go 服务自动备份的 .sql / .json，打包整个目录：<br/>```powershell<br/>Compress-Archive data\backups backup\go-backups-pre-reboot.zip<br/>``` | `backup/go-backups-pre-reboot.zip` |
| **4** | **确认 `.env` 原件没丢** | ```powershell<br/>if (!(Test-Path .env)) { Write-Host ".env MISSING!" -ForegroundColor Red; exit 1 }<br/>Copy-Item .env backup\.env.pre-reboot.bak<br/>``` | `backup/.env.pre-reboot.bak`（含凭据，**不要传 git**） |
| **5** | **确认 Go exe 签名备份存在** | ```powershell<br/>Test-Path server\deploy\drill-server.exe.signed-bak<br/>``` | 预期 `True`（这是 Go 服务回滚的底线） |
| **6** | **记录当前 Go 服务 PID + 启动时间** | ```powershell<br/>$go = Get-Process -Name drill-server -ErrorAction SilentlyContinue<br/>if ($go) {<br/>"PID=$($go.Id) StartTime=$($go.StartTime)" \| Out-File backup\go-process-pre-reboot.txt<br/>Write-Host "Go PID=$($go.Id) started $($go.StartTime)"<br/>} else {<br/>Write-Host "⚠️  Go 进程未在运行？" -ForegroundColor Yellow<br/>}<br/>``` | `backup/go-process-pre-reboot.txt` |
| **7** | **记录端口占用** | ```powershell<br/>netstat -ano \| findstr ":8090" > backup\ports-pre-reboot.txt<br/>``` | `backup/ports-pre-reboot.txt` |
| **8** | **（可选但推荐）给 SQLite 原件做读锁标记** | ```powershell<br/># 只 touch，不读内容，不影响运行<br/>if (Test-Path data\drill.db) {<br/>Set-ItemProperty data\drill.db LastWriteTime (Get-Date)<br/>Write-Host "touched drill.db: $(Get-Item data\drill.db)"<br/>}<br/>``` | — |
| **9** | **确认 Node 兜底就绪** | ```powershell<br/>Test-Path node_modules<br/>``` | 预期 `True`（重启后 Go 起不来就能用 scripts\start-node-fallback.ps1） |
| **10** | **再做一遍 MySQL 备份校验** | 打开 backup/ 目录，确认最新 .sql 文件大小 ≥ 几 KB、能被文本编辑器打开看头部 10 行 | — |

---

## 重启后怎么办（顺序很重要）

### 第一步：先验 Docker Desktop（而不是急着拉 Go）

```
1. 登录 Windows
2. 托盘 → Docker Desktop 图标 → 看状态：
     - "Docker Desktop is running" → 进入 B 路径
     - "Docker Desktop is starting..." → 等 1-2 分钟，还不行就重启 Docker Desktop 服务
     - "Docker Desktop stopped" / 图标灰 → 双击启动
3. 验证容器引擎真的能跑：
     docker run --rm hello-world
```

**如果 Docker Desktop 装了但跑不起来**（Hyper-V/WSL2 故障），别硬等：先退到 A 路径。

---

### A 路径：Go exe 被 360 杀 + Docker Desktop 起不来（兜底路径）

```powershell
# 1. 用 Node 版顶上（应急，SQLite）
powershell -File scripts\start-node-fallback.ps1
# 2. 浏览器打开 http://127.0.0.1:8080 确认能用
# 3. 业务继续运营，同时排查 Docker Desktop 问题（下次再装/启用）
```

---

### B 路径：Docker Desktop 正常（推荐路径）

```powershell
# 方案 B1：Go 进容器（推荐，容器里 Go 二进制不受 360 影响）
powershell -File scripts\docker-up.ps1 -Mode Full
#   （首次要先填 deploy/docker-desktop/.env 的占位符）

# 方案 B2：容器 Go + 宿主 MySQL（复用开发机已有数据库）
powershell -File scripts\docker-up.ps1 -Mode HostMySQL

# 方案 B3：宿主机直接跑 Go exe（风险最高，可能又被 360 拦）
powershell -File scripts\start.ps1 -Detach
#   跑起来后立刻用 start-node-fallback 顶一会儿，确认 Go 在容器里真能活
```

**容器 Go 跑起来后**：
```powershell
# 停掉可能在跑的 Node 版
Stop-Process -Name node -Force -ErrorAction SilentlyContinue
# 浏览器验证 http://127.0.0.1:8090/api/health
# 日志：docker compose -f deploy/docker-desktop/docker-compose*.yml logs -f app
```

---

### C 路径：360 还是拦截容器里的 Go

（**极低概率**——360 扫描的是宿主文件系统，容器文件系统隔离，通常扫不到）

如果真的中招：
1. 容器里 Go 进程能跑起来但立刻被杀 → 360 可能监控了 Docker Desktop 镜像内容
2. 解法：换国产发行版基础镜像（openEuler / Anolis）重构建，二进制路径变了，360 特征库可能匹配不上
3. `docker build -f deploy/linux/Dockerfile --build-arg RUNTIME_BASE=openeuler/openeuler:24.03 ...`

---

## 绝对不要做

| 不要做 | 为什么 |
|--------|--------|
| 不要**现在**手动 `docker compose up` 反复试 | 重启前 daemon 没 ready 会挂，白费时间 |
| 不要在 Node 兜底跑的时候让业务同时写 Node(SQLite) 和容器 Go(MySQL) | 两个数据源分叉，合并很麻烦 |
| 不要把 `.env` 或 MySQL dump 提交到 git | 含真实凭据，会泄漏 |
| 不要删 `server\deploy\drill-server.exe.signed-bak` | 回滚底线 |
| 不要在没做 MySQL dump 的情况下重启 | 万一重启后 MySQL 服务启动异常 + 没备份，数据恢复成本极高 |
| 不要在 Go 进程还在跑的时候手动 kill 后再启动 Node 兜底 | 要先等 Go 进程自然退出（正常 shutdown 走不到，因为重启是强杀） |

---

## 备份目录约定

所有重启前产生的备份都放 `backup/` 目录（仓库根下已存在）：

```
backup/
├── mysql-pre-reboot-20261103-220000.sql
├── drill.db-pre-reboot-20261103-220000.db
├── go-backups-pre-reboot.zip
├── .env.pre-reboot.bak
├── go-process-pre-reboot.txt
├── ports-pre-reboot.txt
└── (后续 Docker Desktop 启动失败时新增的诊断文件)
```

**保留策略**：`backup/` 目录至少保留 30 天，30 天后清理最旧的。
