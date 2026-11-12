# 安装

**最终用户不需要 Go、不需要 Node.js/npm，也不需要 clone 源码。** 只需要一台 x86_64 Linux（优先 openEuler 24.03 LTS-SP1）和 Docker 环境：

- **在线**：只下载一个约 350 KB 的 `install.sh`（基线快照已内嵌），镜像从 GHCR / 华为云 SWR 拉取；
- **离线**：把 `install.sh` 与两个镜像 tar 放同一目录，一条命令安装，目标机无需外网。

## 前置条件

| 项 | 要求 |
|---|---|
| 操作系统 | x86_64 Linux；**优先 openEuler 24.03 LTS-SP1**（实机验证），**统信 UOS Server 20** 兼容可选 |
| 权限 | root（`sudo`） |
| 容器 | Docker Engine 20.10+ 与 Compose v2（`docker compose`）；未安装时在线用 openEuler 源，离线用 `pack-offline.sh --with-rpms` 打出的 RPM |
| 内存 | ≥ 2 GB |
| 磁盘 | 安装目录所在分区 ≥ 3GB 可用（建议 ≥ 5GB）；离线场景另需两个镜像 tar（约 450 MB） |
| 端口 | 8090（应用）空闲；3306 仅绑定 127.0.0.1；启用应用内 TLS 时 8443 |
| 目录 | 默认 `/opt/cyber-drill`；自定义 `--install-dir` 时**父目录必须已存在** |
| 网络 | 在线路径可达 GHCR 或华为云 SWR；离线路径不需要外网 |

## 路径一：在线安装（只下一个文件，推荐）

国内（Gitee）：

```bash
sudo mkdir -p /opt/cyber-drill-installer
cd /opt/cyber-drill-installer
sudo curl -fsSL -o install.sh \
  https://gitee.com/habeas/cyber-drill/raw/main/deploy/install/install.sh
sudo bash install.sh --image-source swr
```

国外（GitHub）：

```bash
sudo mkdir -p /opt/cyber-drill-installer
cd /opt/cyber-drill-installer
sudo curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/chenjinxiang1980/cyber-drill/main/deploy/install/install.sh
sudo bash install.sh --image-source ghcr
```

- 基线快照已内嵌在 `install.sh` 里，**不需要再下载 baseline 相关文件**；`bash install.sh --help` 末尾会打印内嵌版本与 SHA256。
- 已装 Docker/Compose 时，安装器只拉镜像、建库、恢复基线、起应用。

安装器会依次完成：前置检查 → 检查/安装 Docker 与 Compose → 选择镜像源并拉取镜像 → 生成随机强口令 `.env`（权限 600）→ 启动 MySQL → 恢复 1.0.6 基线快照（结构 + 参照数据 + 88 条 `schema_migrations`）→ 启动应用并健康检查 → 打印访问地址与一次性初始口令。

## 路径二：离线安装（install.sh 与镜像 tar 同目录，内网首选）

目标机目录里只放三个文件：

```text
install.sh
cyber-drill-1.0.6.tar.gz
mysql-8.0.tar.gz
```

然后执行：

```bash
sudo bash install.sh --mode offline
```

镜像 tar 可由任意镜像源获取，例如在能联网的机器上导出：

```bash
docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.6
docker tag  swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:1.0.6 cyber-drill:1.0.6
docker save cyber-drill:1.0.6 | gzip -1 > cyber-drill-1.0.6.tar.gz
docker pull mysql:8.0
docker save mysql:8.0 | gzip -1 > mysql-8.0.tar.gz
```

目标机不需要外网，也不需要仓库源码；安装器自动发现同目录 tar 并导入。未装 Docker 时需先安装引擎与 Compose v2，或使用 `pack-offline.sh --with-rpms` 打出的含 `rpms/` 的离线目录（配合 `--offline-dir`）。

## 安装后：访问与初始口令

- 浏览器访问 `http://<服务器IP>:8090`（登录页是 `/`）。
- 健康检查：`curl -fsS http://127.0.0.1:8090/api/health`，应返回 `"version":"1.0.6"`。
- 初始账号：`admin`、`platformadmin`；口令在安装结束时**只打印一次**，同时写入 `/opt/cyber-drill/.env`（权限 600）。
- **首次登录必须立即修改初始口令**；改密后 `.env` 中的一次性口令失效。未改密前可用 `sudo bash install.sh --show-secrets` 再次打印。

## 镜像源选择

| 参数 | 说明 |
|---|---|
| `--image-source auto` | 默认，自动探测；交互三选一（1=SWR 回车即走），非交互国内优先 SWR |
| `--image-source ghcr` | `ghcr.io/chenjinxiang1980/cyber-drill:<版本>`（国外网络） |
| `--image-source swr` | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:<版本>`（国内推荐） |
| `--image-source local` | 使用本地镜像 tar（等价 `--mode offline`），不访问镜像仓库 |

镜像 tag 与仓库根目录 `VERSION` 一致（当前 `1.0.6`），可用 `--version` 覆盖。

## 容器名与多实例

容器名跟随 compose 项目名（`--project`）：

| `--project` | 容器名 |
|---|---|
| `drill`（默认） | `drill-mysql` / `drill-app` |
| 任意其它名称（如 `drilltest`） | `<项目名>-mysql` / `<项目名>-app` |

同一台机器上跑第二套时，必须同时更换安装目录、项目名与端口，两套互不影响：

```bash
sudo bash install.sh \
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
sudo bash deploy/upgrade/upgrade.sh --to 1.0.6 --dir /opt/cyber-drill
```

升级工具不在 `install.sh` 内，从 Gitee 取该目录（8 个文件）：

```bash
sudo mkdir -p /opt/cyber-drill-upgrade/lib && cd /opt/cyber-drill-upgrade
for f in upgrade.sh manifest.json lib/00-common.sh lib/10-manifest.sh \
         lib/20-backup.sh lib/30-migrate.sh lib/40-compose.sh lib/50-rollback.sh; do
  sudo curl -fsSL -o "$f" "https://gitee.com/habeas/cyber-drill/raw/main/deploy/upgrade/$f"
done
sudo bash upgrade.sh --to 1.0.6 --dir /opt/cyber-drill
```

## 安装后检查

```bash
docker compose -f /opt/cyber-drill/docker-compose.yml ps
curl -fsS http://127.0.0.1:8090/api/health
```

容器均为 `healthy`、健康检查返回 `"version":"1.0.6"` 后，浏览器访问 `http://<服务器IP>:8090`。

## 开发者路径（不是部署路径）

最终用户部署不需要 Go、Node.js/npm 或源码。只有需要自行编译二进制、做二次开发或物理机 systemd 部署时，才看：

- [从源码构建与升级数据库](https://gitee.com/habeas/cyber-drill/blob/main/docs/build-from-source.md)
- [Linux 部署指南（systemd / 源码）](https://gitee.com/habeas/cyber-drill/blob/main/docs/linux-deployment.md)
- [一键安装完整参数与故障处置](https://gitee.com/habeas/cyber-drill/blob/main/docs/install-docker-openeuler.md)
