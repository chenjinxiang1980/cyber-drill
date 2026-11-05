# Release 0.5.1 —— 剧本/评分与影响联动增强，发布链路统一

**发布日期**：2026-11-06
**版本号**：`0.5.1`
**标签**：`v0.5.1`
**上一版本**：`0.5.0`（见 [release-0.5.0.md](release-0.5.0.md)）

---

## 一、本次变化

### 1.1 新增

- **S01 剧本与记录单改造**：S01 剧本内容、表单与评分口径调整；新增外部指导意见记录单与 S01 专用记录单，F3/F6/F8 字段补充。
- **业务影响联动与审议模式**（migration `0008`）：影响态势支持联动更新与审议流转。
- **大屏免登录只读观摩**：`SCREEN_PUBLIC_VIEW=true`（默认）时 `/api/screen/*` 允许匿名 GET，独立匿名限流；所有写方法返回 403，绝不落库。
- **播报收敛**：站内消息语音播报收敛到监视大屏，工作台仅提示新消息。
- **评分细化**：混淆节点按严重程度分档扣分（高 8 / 中 5 / 低 3）；S01 评分改为关键词与证据口径；接收人/超时权重与处置质量评分。
- **一键安装器**：`install/install-linux.sh`、`install/install-windows.ps1`（含 `.cmd` 启动器），支持源码来源（GitHub/Gitee/本地）、镜像来源（GHCR/SWR/本地）、compose 项目与端口隔离。

### 1.2 修复

- 脚本编排保存改为按 id 增量更新，避免整表删除重建覆盖节点；逐字段容错解析并修正 seq 重排。
- 业务影响更新 SQL 的 MySQL 1093 自引用错误；F7 恢复确认单按「状态」后缀识别业务系统。
- migration `0010` 修正 S01-9 / S01-10 执行顺序；初始化幂等补齐角色/剧本/字典，**不再删除角色**。
- 实战版演练中锁定脚本并支持强制查看扣分；登录页不再启动通知轮询与顶栏时钟。
- 大屏三处滚动模块末位错位、处置弹窗关闭行为、任务状态门控等 0.5.0 遗留问题继续收敛。

### 1.3 构建与发布

- 版本号统一为 `0.5.1`：服务端版本常量、`/api/health`、品牌默认版本、前端兜底版本、compose/Dockerfile/安装器默认值、文档。
- `.gitattributes` 强制 shell 脚本 LF，避免容器入口 CRLF 导致 `tini exec` 失败。
- 镜像同时发布 `0.5.1` 与 `latest` 两个 tag，GHCR 与华为云 SWR 双仓库。

---

## 二、升级说明（0.5.0 → 0.5.1）

1. 备份数据库（`mysqldump`）。
2. 拉取新镜像：`docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.5.1`（或 SWR 地址）。
3. 更新 compose 的 `image` / `VERSION`（或 `deploy/docker-dist/.env` 中的 `VERSION=0.5.1`）。
4. `docker compose up -d`。入口脚本会自动等待 MySQL 就绪并幂等执行 `drill-migrate up`（新增 0008/0009/0010 等迁移），随后 `drill-seed` 只增不删补齐角色/剧本/字典。
5. 验证：`GET /api/health` 返回 `version=0.5.1`；登录 → 工作台 → 大屏无 console 报错。

> `ADMIN_INIT_PASSWORD` 只影响首次创建管理员；已存在账号的密码不会被覆盖。

---

## 三、发布物

| 产物 | 说明 |
|------|------|
| 源码 | GitHub / Gitee `main` + `v0.5.1` 标签 |
| Docker 镜像 | `drill-server:0.5.1`，linux/amd64，66.5 MB，构建提交 `6480e15` |
| GHCR | `ghcr.io/chenjinxiang1980/cyber-drill:0.5.1` / `:latest` |
| SWR | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.1` / `:latest` |

**manifest digest**

| 仓库 | digest |
|------|--------|
| GHCR `0.5.1` / `latest` | `sha256:9297cc9063ca89e10d6da9a82bdabe233e1d04da5ef02b21d3ef3837b7160025` |
| SWR `0.5.1` / `latest` | `sha256:9297cc9063ca89e10d6da9a82bdabe233e1d04da5ef02b21d3ef3837b7160025` |

两个 tag 在同一仓库内指向同一 manifest；两仓库 digest 也一致（同一本地镜像 ID `a3a430b98b2d`）。
容器内 `/opt/drill/bin/drill-server` 为 11,534,496 B，SHA-256
`63eb81a95deea0ddaf4a8573f1b45a8d747b78b2a91f30f853ea6067d3860136`；`schema_migrations` 由 0010 迁移推进，0.5.0 为 10 条。

**可复现构建**

```bash
docker build -f deploy/docker-dist/Dockerfile \
  --build-arg VERSION=0.5.1 \
  --build-arg BUILD_TIME=<ISO8601> \
  --build-arg VCS_REF=6480e15 \
  --build-arg GIT_COMMIT=6480e15 \
  -t drill-server:0.5.1 .
```

构建机：UOS Server 20 / Docker 25.0.4（legacy builder，无 BuildKit）；构建需代理时追加
`--build-arg HTTP_PROXY=... --build-arg HTTPS_PROXY=...`。

**镜像内容快照（关键文件 SHA-256，小写）**

| 文件 | SHA-256 | Size |
|------|---------|------|
| server/internal/version/version.go | `b033d8acd3f5c0a535c9cb2e849c3cb36c40805ee89840ef70e018ca8884bf1c` | 379 |
| public/app.js | `f242736f116c8e53784912349e8491abd0b2738be6eb5f4505df40a226c5191f` | 53945 |
| public/screen.html | `91b44cfcf6acab01e1eafb8f688a2e72269a12bdaaf01c9d7216d71d49c5a968` | 59043 |
| public/index.html | `fa25b1e54f25a3441966e6282d11a366c4372d2aa041e771e5e9ed8697fe994c` | 13586 |
| deploy/docker-dist/Dockerfile | `2a8906fb5d59f75be8fcc319e1341c15c6c5718a4707ea5a7c3bd2f8c14de683` | 7504 |
| install/install-linux.sh | `f2aaf083aa462f14f748781b4763c092a02cc338ff86152d51539df97fe9c487` | 41531 |
| install/install-windows.ps1 | `ba00d6171c64394e0492693aafd413f8c2eaa013f34e65ea0d1c4805e813f420` | 39380 |

---

## 四、镜像拉取与运行

### 4.1 公开仓库，匿名可拉

```bash
# GHCR
docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.5.1

# 华为云 SWR（cn-east-3）
docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.1
```

两个仓库均为 **public**，无需登录。仅当匿名拉取失败（401/403 且换不到匿名 token）时才需要登录：

```bash
printf '%s' "$REGISTRY_PASS" | docker login swr.cn-east-3.myhuaweicloud.com -u '<cn-east-3@AK>' --password-stdin
```

### 4.2 docker compose

```bash
git clone https://github.com/chenjinxiang1980/cyber-drill.git
cd cyber-drill/deploy/docker-dist
cp .env.example .env      # 替换全部 __REPLACE_WITH__ 占位符
docker compose up -d      # 默认 VERSION=0.5.1
```

使用已发布镜像时，把 `image` 改为 `ghcr.io/chenjinxiang1980/cyber-drill:0.5.1` 并注释 `build:` 段。

### 4.3 必改环境变量

与 0.5.0 一致：`JWT_SECRET`、`SESSION_SECRET`、`DB_USER` / `DB_PASSWORD`、`MYSQL_ROOT_PASSWORD`、`ADMIN_INIT_PASSWORD`；
生产建议收敛 `ALLOW_ORIGINS`、保持 `CAPTCHA_ENABLED=true`、按需设置 `SCREEN_PUBLIC_VIEW`。

---

## 五、发布验证记录

| 项目 | 结果 |
|------|------|
| `/api/health` | 200，`{"ok":true,...,"version":"0.5.1"}` |
| `/api/auth/captcha` | 200，返回 `id` / SVG `image` / `ttl` |
| `/screen.html` | 200，59043 B（仓库与容器一致） |
| 容器身份 | `uid=10001(drill) gid=10001(drill)` |
| 大屏匿名接口 | GET `/api/screen/monitor` 200；POST 403 |
| 登录 | 带算术验证码 `admin` 登录 200，`/api/auth/me` 200 |
| 冒烟隔离 | 独立项目 `drill051smoke`、端口 18091/13307，验后容器与卷已清理 |

---

## 六、GitHub / Gitee Release 建议文案

**标题**：`v0.5.1 · 剧本与评分增强，双仓库镜像发布`

**正文**：

```markdown
## v0.5.1

### 主要变化
- S01 剧本/记录单/评分口径改造，新增外部指导意见记录单
- 业务影响联动与审议模式（migration 0008）
- 大屏免登录只读观摩（匿名限流，写方法 403）
- 播报收敛到大屏；混淆节点分档扣分；脚本编排增量保存等修复
- 版本统一 0.5.1，新增 Windows/Linux 一键安装器

### 镜像
- `docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.5.1`
- `docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.5.1`
- 两仓库均公开，匿名可拉；`latest` 与 `0.5.1` 同一 digest

### 升级
备份数据库 → 拉取新镜像 → `docker compose up -d`（入口自动跑迁移）。
验证：`curl -s http://127.0.0.1:8090/api/health` → `"version":"0.5.1"`。

完整说明见 docs/release-0.5.1.md。MIT License © 2026 陈金祥。
```
