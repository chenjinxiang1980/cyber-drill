# Release 0.5.0 —— Go 版首发

**发布日期**：2026-11-05
**版本号**：`0.5.0`
**标签**：`v0.5.0`

---

## 一、为什么这次是“重构”而不是“升级”

0.5.0 的核心变化是把业务规则从可公开阅读的前端脚本迁移到服务端编译产物。

早期纯 JS + HTML 实现中，剧本编排、评分口径、混淆节点 `before/after` 门控、任务下发条件、计时基准、站内消息播报规则全部写在前端。源码公开即等于把规则公开：任何人都能读到评分公式、改掉门控、伪造任务完成、改写计时起点。

0.5.0 之后：

| 维度 | 重构前 | 重构后 |
|------|--------|--------|
| 规则位置 | 前端脚本 | 服务端 Go 二进制 |
| 前端可篡改 | 可以 | 只影响显示，不影响判定 |
| 绕过评分 | 可读公式、可伪造 | 服务端校验节点与状态 |
| 任务门控 | 前端自行判断 | 服务端按项目状态强制 |
| 计时基准 | 前端本地计时 | 服务端 `projects.started_at` |
| 部署形态 | Node 运行时 | 单个 ELF / Docker 镜像 |

`node/` 目录保留为历史实现与行为基准，不参与部署。

---

## 二、新增

- **Go 服务端首发**：Gin + MySQL 8，单文件编译；`server/internal/version` 统一版本号。
- **健康检查版本字段**：`GET /api/health` 返回 `{"ok":true,"service":"cyber-drill-system","version":"0.5.0","time":"..."}`。
- **版本注入**：Docker 构建通过 `-ldflags` 注入 `Version / BuildTime / GitCommit`，`deploy/docker-dist` 与 `deploy/linux` 构建链同步调整。
- **离线部署包**：`scripts/pack-linux-bundle.ps1` 读取签名文件末行哈希做校验后归档；输出 `dist/drill-linux-bundle.tar.gz` + `.sha256`。
- **一键初始化**：`go run ./cmd/bootstrap -root=..`（建库、建账号、跑 migrations、写种子）。
- **文档**：新增《安全部署指南》《Linux 部署》《VMware 虚拟机部署》《Docker 镜像构建》，README 重写。

## 三、修复

- **演练计时基准**：统一以项目开始时刻 `projects.started_at` 为基准，大屏 / 看板 / 工作台三处按“当前时间 − 开始时刻”重算；“启动演练”真正落库；`started_at` 已存在时顶部计时 chip 正常显示 `T+…`。
- **大屏滚动错位**：实时操作流 / 参演角色处置状态 / 告警与消息三处滚动模块末位错位修复。
- **处置弹窗**：角色工作台处置弹窗只允许“提交成功自动关闭”与“× 关闭”，避免误触关闭丢失内容。
- **任务门控**：基础版任务按项目状态门控，仅“演练中”下发，避免未开始项目提前出现任务。
- **语音播报**：按消息 id 去重、只播未读、只播当前项目、全局公告 0 次、文本只念正文（`descr`），不再拼标题。
- **影响态势去重**：角色工作台“业务系统影响态势”将全局模板行与项目实例行合并去重，避免同一系统重复显示。
- **删除项目清理**：删除演练项目时同步清理关联的消息、任务、影响态势等数据。
- **打包验签**：打包脚本改为读取 `drill-server.sha256.txt` 末行哈希（大小写无关整串比较），不一致直接失败；`scripts/build-linux.ps1` 构建成功后自动重签。
- **容器入口行尾（发布阻断已修复）**：`deploy/docker-dist/docker-entrypoint.sh` 在 Windows 侧以 CRLF 入库，容器内 shebang 变成 `#!/bin/sh\r`，`tini` 报 `exec /opt/drill/docker-entrypoint.sh failed: No such file or directory`，镜像无法启动。新增 `.gitattributes`（`*.sh` / `docker-entrypoint*` / `Dockerfile` 强制 `eol=lf`）后重建镜像，并同步重打离线包（`deploy/linux/install.sh` 同属受影响文件）。

## 四、安全

- **零预置凭证**：仓库只提供 `.env.example`，全部为 `__REPLACE_WITH__` 占位符；`JWT_SECRET / SESSION_SECRET / DB_USER` 缺失或仍为占位符时启动即 `exit 2`。
- **仓库卫生**：`.gitignore` 排除 `.env` 与 `.env.*`、`*.pem/*.crt/*.key`、`*.exe`、`server/deploy-linux/drill-server`、`*.tar.gz`、`dist/`、`*.bak*`、`public.bak*`、SQL 备份、日志、`.trae/tmp/`、截图与作者本机内网运维脚本。
- **命令行口令暴露修复**：mysqldump / mysql CLI 调用由 `--password=<明文>` 改为 `MYSQL_PWD` 环境变量，避免口令出现在进程列表。
- **静态文件防护**：拒绝 `..` 路径穿越与以 `.` 开头的隐藏文件；限制文件服务根目录。
- **鉴权与限流**：JWT + 服务端 Session、RBAC 角色分组、验证码一次性校验、账号失败锁定、全局与登录分组限流。
- **文档脱敏**：移除文档与脚本中的真实内网地址、宿主机路径；发布二进制不入库，仅保留签名文本。

## 五、升级说明

1. 备份数据库。
2. 更新 `.env`：新增/确认 `JWT_SECRET`、`SESSION_SECRET`、`DB_USER`、`DB_PASSWORD`。
3. 执行 `migrations/` 中尚未应用的脚本（`drill-server-migrate` 或 `cmd/migrate`）。
4. 替换服务端二进制 / 镜像，重启服务。
5. 验证：`GET /api/health` 返回 `version=0.5.0`；登录 → 工作台 → 大屏无 console 报错。

## 六、发布物

| 产物 | 说明 |
|------|------|
| 源码 | GitHub / Gitee 默认分支 + `v0.5.0` 标签 |
| Linux ELF | `server/deploy-linux/drill-server`（随离线包或 Release 附件，仓库不入库） |
| 离线包 | `dist/drill-linux-bundle.tar.gz` + `.sha256` |
| Docker 镜像 | `drill-server:0.5.0`（48.2 MB / 9 层，linux/amd64），manifest digest `sha256:763d980465879ed509279d183a1d63be263e4ca95118213f7946e7961e51b8fc` |

镜像构建参数（可复现）：

```bash
docker build -f deploy/docker-dist/Dockerfile \
  -t drill-server:0.5.0 \
  --build-arg VERSION=0.5.0 \
  --build-arg BUILD_TIME=<ISO8601> \
  --build-arg VCS_REF=f1a65bb \
  --build-arg GIT_COMMIT=f1a65bb .
```

---

## 七、镜像拉取与运行

### 7.1 GHCR（GitHub Container Registry）

```bash
docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.5.0
docker pull ghcr.io/chenjinxiang1980/cyber-drill:latest    # 与 0.5.0 同一 digest
```

两个 tag 指向同一 manifest：`sha256:763d980465879ed509279d183a1d63be263e4ca95118213f7946e7961e51b8fc`。
该包已设为 **public**，无需登录即可匿名拉取。

包若为私有，先登录（token 只经管道传入，不进 shell 历史）：

```bash
echo "$GH_TOKEN" | docker login ghcr.io -u <github 用户名> --password-stdin
```

### 7.2 华为云 SWR（区域 cn-east-3）

```bash
docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.0
docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:latest

# 仅在匿名拉取失败时才需要登录（用户名形如 cn-east-3@<AK>，口令交互输入）
docker login swr.cn-east-3.myhuaweicloud.com
```

组织名为 `chenjinxiang`；两个 tag 与 GHCR 同一 manifest digest
`sha256:763d980465879ed509279d183a1d63be263e4ca95118213f7946e7961e51b8fc`。该仓库为**公开**，无需登录即可匿名拉取（2026-11-05 实测通过）。

`<组织名>` 需在 SWR 控制台预先创建（命名空间与 AK 同名的组织通常已存在）；推送失败先确认
命名空间存在、帐号有 `swr:repository:push` 权限。

### 7.3 docker compose 运行

```bash
git clone https://github.com/chenjinxiang1980/cyber-drill.git
cd cyber-drill/deploy/docker-dist
cp .env.example .env      # 然后替换全部 __REPLACE_WITH__ 占位符
docker compose up -d
```

默认由 compose 本地构建镜像；若要用已发布镜像，把 `docker-compose.yml` 的 `image` 改成
`ghcr.io/chenjinxiang1980/cyber-drill:0.5.0`，并把 `build:` 段整段注释掉。

### 7.4 必须修改的环境变量

| 变量 | 必改 | 说明 |
|------|------|------|
| `JWT_SECRET` | 是 | `openssl rand -hex 32`；占位符未替换时启动 `exit 2` |
| `SESSION_SECRET` | 是 | 同上 |
| `DB_USER` / `DB_PASSWORD` | 是 | 业务库账号，不要用 root |
| `MYSQL_ROOT_PASSWORD` | 是 | 仅 MySQL 容器首次初始化使用 |
| `ADMIN_INIT_PASSWORD` | 是 | 管理员初始口令（bootstrap 用） |
| `ADMIN_BASELINE_USERS` | 建议 | 账号清理白名单，逗号分隔且不含 `admin`；留空只保留 `admin` |
| `CAPTCHA_ENABLED` | 建议 | 保持 `true`；`false` 仅供内部联调 |
| `PUBLIC_BASE_URL` | 建议 | 反向代理后的对外地址 |
| `TLS_ENABLED` | 视情况 | 开启后证书缺失即拒绝启动，不会静默降级 |
| `ALLOW_ORIGINS` | 视情况 | 生产应收敛为具体域名，不建议留 `*` |

### 7.5 验证版本

```bash
curl -s http://127.0.0.1:8090/api/health
# {"ok":true,"service":"cyber-drill-system","version":"0.5.0","time":"..."}
```

镜像内也可用启动日志确认（不连数据库也会先打印版本行）：

```bash
docker run --rm --entrypoint /opt/drill/bin/drill-server \
  ghcr.io/chenjinxiang1980/cyber-drill:0.5.0 -root=/opt/drill
# INFO 启动 version=0.5.0 buildTime=<ISO8601> gitCommit=f1a65bb
```

---

## 八、GitHub / Gitee Release 建议文案（待确认后发布）

**标题**：

```
v0.5.0 · Go 版首发：把演练规则收进服务端
```

**正文**：

```markdown
## v0.5.0 · Go 版首发

这次不是普通升级，而是把演练系统的“判定权”从前端移到服务端。

### 为什么重构
旧版纯 JS + HTML 的源码公开后，剧本编排、评分公式、节点门控、任务下发条件、
计时基准和播报规则都能被直接阅读和修改——任何人都能绕过评分或伪造处置。
0.5.0 起，这些规则全部进入 Go 服务端编译产物，前端只负责展示。

### 主要变化
- 新增：Go/Gin 服务端、/api/health 版本字段、离线包与验签流程
- 修复：计时基准、大屏滚动错位、处置弹窗、任务门控、语音播报去重与只念正文、
  影响态势去重、删除项目清理、打包验签
- 安全：仓库零预置凭证、.gitignore 补强、命令行口令改用 MYSQL_PWD、
  文档与脚本内网信息脱敏

### 部署
- Docker（48.2 MB 单镜像，9 层，linux/amd64）：
  - `docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.5.0`
  - SWR（cn-east-3）：`docker pull swr.cn-east-3.myhuaweicloud.com/<组织名>/cyber-drill:0.5.0`
  - compose 用法见 `deploy/docker-dist/docker-compose.yml`
- Linux：`scripts/build-linux.ps1` 交叉编译 + `dist/drill-linux-bundle.tar.gz`

### 注意
仓库不含任何真实凭证。首次部署必须自行设置 ADMIN_INIT_PASSWORD、
JWT_SECRET、SESSION_SECRET、DB_USER、DB_PASSWORD，保持 CAPTCHA_ENABLED=true，
并按需设置 ADMIN_BASELINE_USERS（留空则只保留 admin）。

验证：`curl -s http://127.0.0.1:8090/api/health` → `"version":"0.5.0"`。

完整说明（含拉取命令、compose、必改环境变量）见 docs/release-0.5.0.md。MIT License © 2026 陈金祥。
```
