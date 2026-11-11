# 安装

**最终用户不需要 Go、不需要 Node.js/npm，也不需要 clone 源码。** 只需要一台 x86_64 Linux（优先 openEuler 24.03 LTS-SP1）和 Docker 环境：

- **在线**：从 GHCR / 华为云 SWR 拉取镜像，只下载约 270 KB 的安装器与基线快照；
- **离线**：下载 Release 离线包（约 451 MiB），解包后一条命令安装，目标机无需外网。

## 前置条件

| 项 | 要求 |
|---|---|
| 操作系统 | x86_64 Linux；**优先 openEuler 24.03 LTS-SP1**（实机验证），**统信 UOS Server 20** 兼容可选 |
| 权限 | root（`sudo`） |
| 容器 | Docker Engine 20.10+ 与 Compose v2（`docker compose`）；未安装时在线用 openEuler 源、离线用包内 RPM |
| 内存 | ≥ 2 GB |
| 磁盘 | 安装目录所在分区 ≥ 3GB 可用（建议 ≥ 5GB）；离线包 472,867,328 字节（约 451 MiB / 473 MB） |
| 端口 | 8090（应用）空闲；3306 仅绑定 127.0.0.1；启用应用内 TLS 时 8443 |
| 目录 | 默认 `/opt/cyber-drill`；自定义 `--install-dir` 时**父目录必须已存在** |
| 网络 | 在线路径可达 GHCR 或华为云 SWR；离线路径不需要外网 |

## 路径一：在线安装（无需源码，推荐）

Gitee 优先（国内）：

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

- **四条 `curl` 缺一不可**：安装器优先读取脚本同级 `baseline/`，并用 `SHA256SUMS` 校验快照。
- GitHub 备选：把 4 个 URL 的 `https://gitee.com/habeas/cyber-drill/raw/main/` 换成 `https://raw.githubusercontent.com/chenjinxiang1980/cyber-drill/main/`。
- 已装 Docker/Compose 时，安装器只拉镜像、建库、恢复基线、起应用。

安装器会依次完成：前置检查 → 检查/安装 Docker 与 Compose → 选择镜像源并拉取镜像 → 生成随机强口令 `.env`（权限 600）→ 启动 MySQL → 恢复 1.0.4 基线快照（结构 + 参照数据 + 85 条 `schema_migrations`）→ 启动应用并健康检查 → 打印访问地址与一次性初始口令。

## 路径二：离线安装（Release 离线包，内网首选）

在可联网机器下载，把 tar 包拷到目标机：

```bash
curl -fL -o cyber-drill-offline-1.0.2.tar.gz \
  https://github.com/chenjinxiang1980/cyber-drill/releases/download/v1.0.2/cyber-drill-offline-1.0.2.tar.gz
tar -xzf cyber-drill-offline-1.0.2.tar.gz
cd cyber-drill-offline-1.0.2
sudo bash deploy/install/install.sh --offline-dir "$PWD"
```

离线包内容概要：

- Docker 引擎 RPM：`moby-engine` / `moby-client` 及依赖；
- Compose v2 独立二进制；
- 应用镜像 `cyber-drill-1.0.2` 与 MySQL 8.0 镜像；
- 基线快照 `drill-baseline-1.0.2.sql.gz`、`SHA256SUMS`、`manifest.env`；
- `deploy/install/install.sh` 与 `deploy/upgrade/` 升级工具。

目标机不需要外网，也不需要仓库源码。Gitee 发行版页 <https://gitee.com/habeas/cyber-drill/releases> 提供同版本说明；离线包体积较大，附件托管在 GitHub Release。

## 安装后：访问与初始口令

- 浏览器访问 `http://<服务器IP>:8090`（登录页是 `/`）。
- 健康检查：`curl -fsS http://127.0.0.1:8090/api/health`，应返回 `"version":"1.0.4"`。
- 初始账号：`admin`、`platformadmin`；口令在安装结束时**只打印一次**，同时写入 `/opt/cyber-drill/.env`（权限 600）。
- **首次登录必须立即修改初始口令**；改密后 `.env` 中的一次性口令失效。未改密前可用 `sudo bash install.sh --show-secrets` 再次打印。

## 镜像源选择

| 参数 | 说明 |
|---|---|
| `--image-source auto` | 默认，自动探测；两者均可达时交互选择 |
| `--image-source ghcr` | `ghcr.io/chenjinxiang1980/cyber-drill:<版本>` |
| `--image-source swr` | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:<版本>`（国内推荐） |

镜像 tag 与仓库根目录 `VERSION` 一致（当前 `1.0.4`），可用 `--version` 覆盖。

## 容器名与多实例

容器名跟随 compose 项目名（`--project`）：

| `--project` | 容器名 |
|---|---|
| `drill`（默认） | `drill-mysql` / `drill-app` |
| 任意其它名称（如 `drilltest`） | `<项目名>-mysql` / `<项目名>-app` |

同一台机器上跑第二套时，必须同时更换安装目录、项目名与端口，两套互不影响：

```bash
sudo bash deploy/install/install.sh \
  --install-dir /home/test/cyber-drill --project drilltest \
  --port 18090 --mysql-port 13306
```

`docker logs`、`docker exec` 等命令里的容器名请按上表替换。

## 已安装过：重装还是升级

检测到安装目录已存在且非空时，安装器直接停止（退出码 28），不会覆盖数据：

```bash
# A. 不保留数据：删除原目录、容器与数据卷后重装（交互输入 DELETE 确认）
sudo bash install.sh --reinstall

# 非交互环境必须显式提供确认词
sudo bash install.sh --reinstall --confirm DELETE

# B. 保留数据：用升级工具（备份 → 迁移 → 重建 app → 校验，失败回滚）
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

## 安装后检查

```bash
docker compose -f /opt/cyber-drill/docker-compose.yml ps
curl -fsS http://127.0.0.1:8090/api/health
```

容器均为 `healthy`、健康检查返回 `"version":"1.0.4"` 后，浏览器访问 `http://<服务器IP>:8090`。

## 开发者路径（不是部署路径）

最终用户部署不需要 Go、Node.js/npm 或源码。只有需要自行编译二进制、做二次开发或物理机 systemd 部署时，才看：

- [从源码构建与升级数据库](https://gitee.com/habeas/cyber-drill/blob/main/docs/build-from-source.md)
- [Linux 部署指南（systemd / 源码）](https://gitee.com/habeas/cyber-drill/blob/main/docs/linux-deployment.md)
- [一键安装完整参数与故障处置](https://gitee.com/habeas/cyber-drill/blob/main/docs/install-docker-openeuler.md)
