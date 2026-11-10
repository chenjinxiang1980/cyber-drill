# 安装

## 唯一入口

Linux + Docker 的唯一一键安装脚本：

```text
deploy/install/install.sh
```

仓库中不存在其他一键安装入口。`deploy/linux/install.sh` 仅用于物理机 systemd 手工部署（需自行编译二进制），不是一键安装入口。

## 前置条件

| 项 | 要求 |
|---|---|
| 操作系统 | Linux x86_64（openEuler / 统信 UOS / CentOS / Ubuntu 等） |
| 权限 | root（`sudo bash ...`） |
| 磁盘 | 安装目录所在分区 ≥ 3GB 可用 |
| 网络 | 可访问 GHCR 或华为云 SWR；否则使用离线包 |
| 容器 | Docker Engine 20.10+，Compose v2（`docker compose`） |

## 在线安装

```bash
git clone https://github.com/chenjinxiang1980/cyber-drill.git
cd cyber-drill
sudo bash deploy/install/install.sh
```

安装器会依次完成：

1. 前置检查（root、架构、磁盘、端口）；
2. 检查/安装 Docker 与 Compose；
3. 探测镜像仓库并选择镜像源，拉取应用与 MySQL 镜像；
4. 建安装目录，生成随机强口令 `.env`（权限 600）；
5. 启动 MySQL；
6. 恢复 1.0.0 基线快照（结构 + 参照数据 + 76 条 `schema_migrations`）；
7. 启动应用并做健康检查；
8. 生成 `admin` / `platformadmin` 初始口令；
9. 打印访问地址与初始口令。

## 镜像源选择

| 参数 | 说明 |
|---|---|
| `--image-source auto` | 默认，自动探测；两者均可达时交互选择 |
| `--image-source ghcr` | `ghcr.io/chenjinxiang1980/cyber-drill:<版本>` |
| `--image-source swr` | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:<版本>` |

镜像 tag 与仓库根目录 `VERSION` 一致，可用 `--version` 覆盖。

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


## 初始口令

安装结束时会明确打印并标注“仅显示一次”：

```text
admin 初始口令         : ********
platformadmin 初始口令 : ********
```

口令同时写入安装目录 `.env`（权限 600）。**首次登录必须立即修改初始口令**，修改后 `.env` 中的值会失效。

## 已安装过

检测到安装目录已存在且非空时，安装器直接停止，给出两条路径：

```bash
# A. 删除原目录、容器与数据卷后重装（交互输入 DELETE 确认）
sudo bash deploy/install/install.sh --reinstall

# B. 保留数据，改用升级脚本
sudo bash deploy/upgrade/upgrade.sh --to 1.0.0
```

非交互环境执行 A 分支：

```bash
sudo bash deploy/install/install.sh --reinstall --confirm DELETE
```

## 离线安装

```bash
sudo bash deploy/install/install.sh --offline-dir /opt/cyber-drill-offline
```

离线目录需包含 `rpms/`、`images/`、`baseline/`、`bin/docker-compose`。安装器会优先使用离线资源。

## 安装后检查

```bash
docker compose -f /opt/cyber-drill/docker-compose.yml ps
curl -s http://127.0.0.1:8090/api/health
```

健康检查返回 `status: ok` 后，浏览器访问 `http://<服务器IP>:8090`。
