# 从源码构建与升级数据库（开发者路径）

> **这是开发者路径，不是部署路径。** 最终用户部署不需要 Go、Node.js/npm 或源码，只需 Docker 环境 + 在线镜像源，或与 `install.sh` 同目录的镜像 tar，见 [一键安装（Docker / openEuler）](install-docker-openeuler.md)。
> 本文面向自己编译二进制、自己控数据库的高级用户；用容器部署的普通用户升级请直接用 [升级指南](upgrade.md) 里的 `deploy/upgrade/upgrade.sh`，不需要看这篇。

本篇三件事，按顺序做：

1. **版本注入**：把版本号、构建时间、提交号打进二进制；
2. **`GOOS` 交叉编译**：在任意平台产出 Linux/amd64 的 `drill-server` / `drill-migrate` / `drill-seed`（以及可选的 `drill-upgrade`）；
3. **`drill-migrate up`**：只升级数据库，不动容器。

---

## 0. 依赖

| 项 | 要求 | 说明 |
|---|---|---|
| Go | **1.27+** | `server/go.mod` 声明 `go 1.27` |
| CGO | 关闭 | 纯 Go 依赖，`CGO_ENABLED=0` 才能交叉编译出静态 ELF |
| 目标平台 | `linux/amd64`（ARM 服务器改 `GOARCH=arm64`） | 镜像基础层是 `alpine:3.20` |
| 数据库 | MySQL 8.0 | 迁移脚本按 8.0 语法写 |

代码根目录约定（下文以仓库根为当前目录）：

```
server/            Go 源码（go.mod 在这里）
migrations/        0000_bootstrap … 0127_script_target_roles 等 .up.sql
public/            前端静态资源
deploy/upgrade/    upgrade.sh + manifest.json（清单）
deploy/docker-dist/Dockerfile.runtime
```

---

## 1. 版本注入

版本信息在 `server/internal/version/version.go`，三个变量都可以用 `-ldflags -X` 覆盖：

| 变量 | 注入值 |
|---|---|
| `drill-system/internal/version.Version` | 版本号，必须与 `deploy/upgrade/manifest.json` 的 `version`、`/api/health` 返回的一致（例如 `1.0.0`，**不带 v**） |
| `drill-system/internal/version.BuildTime` | UTC 时间戳 |
| `drill-system/internal/version.GitCommit` | 短提交号 |

```bash
VERSION=1.0.7
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT=$(git rev-parse --short HEAD)
LDFLAGS="-X drill-system/internal/version.Version=${VERSION} \
         -X drill-system/internal/version.BuildTime=${BUILD_TIME} \
         -X drill-system/internal/version.GitCommit=${GIT_COMMIT}"
```

> 版本号写错（比如写成 `v1.0.0`）会让升级工具 P5 的 `health.version` 校验失败并触发回滚。

---

## 2. `GOOS` 交叉编译

在 `server/` 目录下执行（`-trimpath` 去掉本机路径，便于复现）：

```bash
cd server
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0

# 主服务：保留符号表，便于线上排查
go build -trimpath -ldflags "${LDFLAGS}" -o ../dist/drill-server .

# 迁移器与种子：可执行文件小一点
go build -trimpath -ldflags "-s -w ${LDFLAGS}" -o ../dist/drill-migrate ./cmd/migrate
go build -trimpath -ldflags "-s -w ${LDFLAGS}" -o ../dist/drill-seed    ./cmd/seed
```

Windows 上可直接用仓库脚本（同样行为，产出到 `server/deploy-linux/`）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-linux.ps1
```

### 2.1 可选：编译 `drill-upgrade`（只升级数据库的小工具）

`server/cmd/upgrade` 把版本清单 **内嵌（`go:embed`）** 进二进制，所以清单必须和 `main.go` 在**同一个包目录**。`go:embed` 不能引用上级目录，因此用 `go:generate` 复制一份：

```bash
cd server
go generate ./cmd/upgrade        # = cp ../../../deploy/upgrade/manifest.json manifest.json
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w ${LDFLAGS}" -o ../dist/drill-upgrade ./cmd/upgrade
```

Windows 上没有 `cp`，`go generate` 会报 `exec: "cp": executable file not found`，手工复制即可（效果相同）：

```powershell
cd server
Copy-Item ..\deploy\upgrade\manifest.json .\cmd\upgrade\manifest.json
```

> 手工构建忘了 `go generate` 时，`cmd/upgrade/manifest.json` 可能是旧副本——**升级完清单记得重新 `go generate`**。
> 排障时可以用 `--manifest /path/to/manifest.json` 临时指定外部清单，覆盖内嵌的那份。

### 2.2 可选：自建镜像

`Dockerfile.runtime` 只做 runtime 阶段，要求构建上下文里已经有编译好的 ELF：

```bash
# 约定：dist_local/ 里放三个二进制（drill-server 必需，migrate/seed 缺失则跳过）
mkdir -p dist_local
cp dist/drill-server dist/drill-migrate dist/drill-seed dist_local/

DOCKER_BUILDKIT=1 docker build \
  -f deploy/docker-dist/Dockerfile.runtime \
  -t cyber-drill:1.0.7 .
```

`dist_local/`、`public/`、`migrations/`、`deploy/docker-dist/docker-entrypoint.sh` 都必须在构建上下文根目录下。

---

## 3. `drill-migrate up`：只升级数据库

```bash
# 1) 先看：库现在到哪、目录里还有多少条没跑
./dist/drill-migrate -root=. status

# 2) 先备份（自己决定放哪）
mysqldump --single-transaction --quick --routines --triggers --events \
  --default-character-set=utf8mb4 drill_system | gzip -1 > backup-$(date +%Y%m%d-%H%M%S).sql.gz

# 3) 应用迁移（-root 指向含 migrations/ 与 .env 的目录）
./dist/drill-migrate -root=. up
```

要点：

- `-root` 是**项目根**，不是 `migrations/` 本身：目录下要有 `migrations/*.up.sql`，`.env`/环境变量提供 `DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_USER`/`DB_PASSWORD`。
- 迁移只跑 `schema_migrations` 里没有的条目，重复执行是安全的；**没有 down 脚本**，回退只能恢复整库备份。
- 只想确认版本链、不执行迁移时，用 `drill-upgrade verify`（见下一节），不要拿 `status` 反推版本链。

### 3.1 用 `drill-upgrade` 校验与执行（推荐）

`drill-upgrade` 认识「版本 → 迁移区间」的映射，能拒绝跨级缺环、能校验迁移后的行数与末条：

```bash
# 只读检查：当前版本、链是否完整、要跑哪几条、迁移后会到多少条
./dist/drill-upgrade verify --to 1.0.7 --env-file .env

# 备份 + 迁移 + 校验，一条龙
./dist/drill-upgrade db --to 1.0.7 --env-file .env \
  --backup-dir ./backups --root . --migrate-bin ./dist/drill-migrate --yes
```

| 参数 | 说明 |
|---|---|
| `--to` / `--from` | 目标版本；`--from` 不写时按 `schema_migrations` 行数反查 |
| `--env-file` | `DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_USER`/`DB_PASSWORD` 来源 |
| `--db-host` / `--db-port` | 覆盖环境文件里的值（在容器外连 `mysql` 服务名时用） |
| `--manifest` | 外部清单（默认用内嵌的） |
| `--backup-dir` | `db` 子命令的备份目录，默认 `./backups` |
| `--root` | `migrations/` 所在目录，默认 `.` |
| `--migrate-bin` | `drill-migrate` 路径（默认同目录，其次 `PATH`） |
| `--mysqldump` | `mysqldump` 路径（默认 `PATH`） |
| `--yes` | 跳过二次确认（`db` 必需） |
| `--json` | 机器可读输出（脚本调用用） |

退出码：`0` 成功；`1` 执行失败（连库、备份、迁移、校验）；`2` 参数错误或版本链错误。

> `drill-upgrade db` **只做数据库**：不拉镜像、不改 compose、不重建容器。容器部署请回到 [升级指南](upgrade.md)。

### 3.2 在「容器部署」里手工控迁移顺序

想自己控制「先迁移还是先换镜像」，但迁移器在镜像里：

```bash
docker run --rm \
  --network <compose项目名>_drill-net \
  --env-file .env -e DB_HOST=mysql \
  --entrypoint /opt/drill/bin/drill-migrate \
  cyber-drill:1.0.7 -root=/opt/drill up
```

`--entrypoint` 绕开 `docker-entrypoint.sh`，不会先等 MySQL 再 seed；迁移只写库，不挂数据卷。

---

## 4. 升级后自检

```bash
curl -fsS http://127.0.0.1:8090/api/health          # {"ok":true,...,"version":"1.0.7"}
docker exec <mysql容器> mysql -uroot -p -N -e \
  "select count(*), max(version) from schema_migrations" drill_system   # 80 / 0127_script_target_roles
```

---

## 5. 相关文档

- [升级指南（容器部署）](upgrade.md)
- [一键升级工具设计](upgrade-tool-design.md)：清单字段、链算法、失败分支
- [Linux 部署](linux-deployment.md)
