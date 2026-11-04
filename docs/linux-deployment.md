# Linux 部署指南

> ⚠️ **未实测声明**：本指南的全部配置文件（systemd unit、nginx/Caddyfile、Dockerfile、docker-compose.yml、install.sh）已按 Linux 标准规范编写并经代码审查，但**作者当前在 Windows 开发机上无法直接在真实 Linux 主机上验证**（本机无 WSL/Docker/远程 Linux 跳板机）。首次部署时请严格按照"部署前检查表"逐项确认，遇到问题请对照"常见故障"章节排查。

---

## 目录

1. [交付物清单](#1-交付物清单)
2. [前置条件](#2-前置条件)
3. [交叉编译](#3-交叉编译)
4. [部署前检查表](#4-部署前检查表)
5. [物理机 + systemd 部署](#5-物理机--systemd-部署)
6. [HTTPS / 反向代理](#6-https--反向代理)
7. [开机自启](#7-开机自启)
8. [日志](#8-日志)
9. [备份](#9-备份)
10. [升级与回滚](#10-升级与回滚)
11. [常见故障](#11-常见故障)
12. [Docker Compose 部署（可选）](#12-docker-compose-部署可选)
13. [信创 / 国产化部署](#13-信创--国产化部署)

---

## 1. 交付物清单

| 文件 | 路径 | 说明 |
|------|------|------|
| `install.sh` | `deploy/linux/install.sh` | 幂等安装脚本（需 root） |
| `drill-server.service` | `deploy/linux/drill-server.service` | systemd 单元（非 root 运行、加固） |
| `drill.env.example` | `deploy/linux/drill.env.example` | Linux 环境变量模板（含路径注释） |
| `nginx-drill.conf` | `deploy/linux/nginx-drill.conf` | Nginx 反向代理 + TLS 示例 |
| `Caddyfile.example` | `deploy/linux/Caddyfile.example` | Caddy 自动 HTTPS 示例 |
| `Dockerfile` | `deploy/linux/Dockerfile` | 多阶段构建（alpine 编译 + runtime） |
| `docker-compose.yml` | `deploy/linux/docker-compose.yml` | 一键起 MySQL + app + bootstrap |
| `drill-server` | `server/deploy-linux/drill-server` | 已编译的 ELF64 x86_64（不覆盖不删除） |

---

## 2. 前置条件

**目标机器**：
- Linux x86_64（amd64），内核 ≥ 3.10（systemd 要求）
- systemd init system（Debian 10+ / Ubuntu 20.04+ / CentOS 8+ / Rocky 8+ 等主流发行版）
- 非 root 用户（deploy 期间只需要一次 sudo）
- 至少 512MB 内存（Go 二进制 + MySQL），2GB 推荐

**数据库**：
- MySQL 8.0+（utf8mb4）
- 本机或远程均可；远程 MySQL 需在 `.env` 里改 `DB_HOST`

**开放端口**：
- 后端默认监听 `0.0.0.0:8090`（仅内网需要）
- HTTPS 反向代理需 80 + 443
- MySQL 3306 仅本机或内网访问，不要直接暴露到公网

**工具**：
- `curl` 或 `wget`（健康检查）
- `openssl`（生成 JWT/Session 密钥）
- `tar`/`gzip`（备份）

**不需要**：
- **不需要**在目标机器上安装 Go 或 Docker（静态 ELF 直接运行）
- **不需要**关闭防火墙/SELinux/AppArmor（反而建议开启，见"SELinux"故障排查）

---

## 3. 交叉编译

### 3.1 在 Windows 上交叉编译（推荐）

Go 内置交叉编译，Windows → Linux 的二进制纯静态、零依赖：

```powershell
# 方法一：一行命令（推荐）
cd server
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -o deploy-linux/drill-server .

# 或用 go env 持久化（改回来用 go env -u）
go env -w GOOS=linux GOARCH=amd64 CGO_ENABLED=0
go build -trimpath -o deploy-linux/drill-server .
# 改回来（切回 Windows 编译）：
go env -u GOOS; go env -u GOARCH; go env -u CGO_ENABLED

# 一次性两行（不持久化 go env）
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'; go build -trimpath -o deploy-linux/drill-server .
```

验证产物（Windows 上 `file` 命令可用 Git Bash 或 WSL 的 `file.exe`）：

```bash
# 在 Git Bash / WSL 中
file server/deploy-linux/drill-server
# 预期: drill-server: ELF 64-bit LSB executable, x86-64 ... statically linked
```

**为什么 CGO_ENABLED=0 可以**：本项目所有 Go 依赖（gin、mysql driver、modernc.org/sqlite）都是纯 Go 实现，不依赖 libc。静态链接后产物可直接在任何 Linux x86_64（musl/glibc 均兼容）上运行。

### 3.2 在 Linux 上本地编译

```bash
cd server
CGO_ENABLED=0 go build -trimpath -o deploy-linux/drill-server .
```

---

## 4. 部署前检查表

部署前逐项确认，**不跳过**：

| # | 检查项 | 怎么查 | 正确状态 |
|---|--------|--------|----------|
| 1 | 架构匹配 | `file drill-server` 或 `uname -m` | 目标机器 `x86_64`，二进制 `ELF x86-64` |
| 2 | 端口 8090 空闲 | `ss -ltnp \| grep 8090` | 无输出 |
| 3 | MySQL 可连 | `mysql -h 127.0.0.1 -P 3306 -u root -p -e "SELECT VERSION();"` | 返回版本号 |
| 4 | `.env` 无真实口令外泄 | `grep -E 'PASSWORD\|SECRET' /etc/drill/drill.env` | 全部是 `__REPLACE_WITH__*` 或真实值但文件权限 0640 |
| 5 | 防火墙 | `sudo firewall-cmd --list-all` (firewalld) 或 `sudo ufw status` | 8090 / 80 / 443 按需开放 |
| 6 | SELinux（如果是 RHEL/CentOS/Rocky） | `getenforce` | 建议 `Enforcing`，不要设 `Disabled`（见故障排查） |
| 7 | 磁盘空间 | `df -h /opt /var/log` | 每分区剩余 ≥ 1GB |
| 8 | 证书可读（如果用 TLS） | `sudo -u drill test -r /etc/drill/certs/server.key && echo OK` | `OK` |

---

## 5. 物理机 + systemd 部署

### 5.1 拷贝文件到目标机器

```bash
# 在 Windows 上（Git Bash / WSL）
scp server/deploy-linux/drill-server \
      deploy/linux/install.sh \
      deploy/linux/drill-server.service \
      user@drill-server:/tmp/

# （可选）也把前端和 migrations 同步过去
scp -r public user@drill-server:/tmp/public
scp -r migrations user@drill-server:/tmp/migrations
```

### 5.2 执行安装脚本

```bash
ssh user@drill-server
sudo bash /tmp/install.sh
```

install.sh 会自动：
- 创建 `drill` 系统用户（no-login、无家目录）
- 建 `/opt/drill/{bin,public,data,uploads,migrations,logs,run}`
- 拷贝二进制到 `/opt/drill/bin/drill-server`（0755）
- 拷贝 systemd unit 到 `/etc/systemd/system/drill-server.service`（0644）
- 自动 `systemctl daemon-reload`
- **不**自动 start（等你放好 .env）

> install.sh 是幂等的——目录/文件已存在会覆盖更新，不会报错。

### 5.3 写环境变量

```bash
sudo cp deploy/linux/drill.env.example /etc/drill/drill.env
sudo vi /etc/drill/drill.env
```

**必须修改的键**（按优先级）：

| 键 | 改法 |
|----|------|
| `DB_HOST` | MySQL 在同机 → `127.0.0.1`；远程 MySQL → 远程 IP |
| `DB_USER` / `DB_PASSWORD` | MySQL 业务账号（bootstrap 会创建，或手动 CREATE） |
| `DB_ROOT_USER` / `DB_ROOT_PASSWORD` | 高权限账号（仅 bootstrap 用，服务不读） |
| `JWT_SECRET` | `openssl rand -hex 32` 生成 |
| `SESSION_SECRET` | `openssl rand -hex 32` 生成 |
| `TRUSTED_PROXIES` | Nginx 在同机 → `127.0.0.1/32`；多层代理 → 逗号分隔 CIDR |
| `PUBLIC_BASE_URL` | 部署后对外 URL，如 `https://drill.example.com` |

**不需要改的键**：
- 路径（`DATA_DIR=data`、`UPLOAD_ROOT=uploads`）——install.sh 把二进制放在 `/opt/drill/bin/`，`-root=/opt/drill`，所以相对路径自动解析正确
- MySQL root/Docker Compose 场景（MYSQL_* 会自动创建库和账号）

**权限**（关键！）：

```bash
sudo chown root:root /etc/drill/drill.env
sudo chmod 0640      /etc/drill/drill.env
# 验证
ls -l /etc/drill/drill.env
# 预期: -rw-r----- 1 root root ...
```

### 5.4 初始化数据库

首次部署必须跑 bootstrap，**只跑一次**：

```bash
# 方式一：用 Go 工具（目标机没装 Go 时，在开发机上编译 bootstrap 二进制拷过去）
go run ./server/cmd/bootstrap -root=/opt/drill

# 方式二：编译独立 bootstrap 二进制
cd server
CGO_ENABLED=0 go build -trimpath -o deploy-linux/drill-server-bootstrap ./cmd/bootstrap
# 然后拷到目标机，放在 /opt/drill/bin/ 下执行：
/opt/drill/bin/drill-server-bootstrap -root=/opt/drill
```

bootstrap 会用 `DB_ROOT_USER/DB_ROOT_PASSWORD` 高权限账号：
1. CREATE DATABASE `drill_system`
2. CREATE USER 业务账号 + GRANT `drill_system.*`
3. 执行 `migrations/0001_init.up.sql`（33 张表）
4. 写入测试 seed（含 testadmin）

### 5.5 启动服务

```bash
# 预演：先测试二进制能不能跑
/opt/drill/bin/drill-server -root=/opt/drill &
sleep 2
curl http://127.0.0.1:8090/api/health
kill %1

# 正式启动
sudo systemctl start drill-server
sudo systemctl enable drill-server     # 开机自启

# 验证
sudo systemctl status drill-server
curl http://127.0.0.1:8090/api/health
```

健康检查预期（DB 已连接）：

```json
{"ok":true,"service":"drill-server","uptime":"...","db":"up"}
```

---

## 6. HTTPS / 反向代理

Go 进程默认监听 `0.0.0.0:8090`。**生产强烈建议在前面套反向代理**，HTTPS 卸载在代理层处理——比 Go 进程直接起 TLS 更灵活（自动续期、HSTS、gzip、缓存）。

两种方案二选一：

### 方案 A：Nginx（传统、稳定）

```bash
# 安装
# Debian/Ubuntu
sudo apt install -y nginx certbot python3-certbot-nginx
# RHEL/CentOS/Rocky
sudo dnf install -y nginx certbot python3-certbot-nginx

# 拷贝配置（替换域名）
sudo cp deploy/linux/nginx-drill.conf /etc/nginx/conf.d/drill.conf
sudo sed -i 's/__REPLACE_WITH_DOMAIN__/drill.example.com/g' /etc/nginx/conf.d/drill.conf

# 申请证书（自动）
sudo certbot --nginx -d drill.example.com -d www.drill.example.com
# certbot 会自动修改 nginx 配置加 ssl_certificate 路径

# 重载
sudo nginx -t && sudo systemctl reload nginx
```

Go 进程端 `.env` 同步修改：

```bash
TRUSTED_PROXIES=127.0.0.1/32   # Nginx 在同机
```

**与 UPLOAD_MAX_MB 的对应**：nginx 配置里的 `client_max_body_size 20m;` 必须和 `.env` 里的 `UPLOAD_MAX_MB=20` 保持一致。如果改了 `UPLOAD_MAX_MB`，记得同步改 nginx 并重载。

### 方案 B：Caddy（更简洁、自动 HTTPS）

```bash
# 安装（apt 源配置见 deploy/linux/Caddyfile.example 注释）
sudo apt install -y caddy

# 写配置
sudo cp deploy/linux/Caddyfile.example /etc/caddy/Caddyfile
sudo sed -i 's/__REPLACE_WITH_DOMAIN__/drill.example.com/g' /etc/caddy/Caddyfile

# 重载
sudo systemctl reload caddy
```

Caddy 自动处理 Let's Encrypt 证书签发、续期、80→443 跳转、gzip、WebSocket 升级。

### Go 自带 TLS（仅无反向代理时使用）

```ini
TLS_ENABLED=true
HTTPS_CERT_FILE=/etc/drill/certs/server.crt
HTTPS_KEY_FILE=/etc/drill/certs/server.key
HTTP_REDIRECT_TO_HTTPS=true
```

⚠️ `TLS_ENABLED=true` + 证书缺失 = **直接拒绝启动**（exit code 2，不降级）。

---

## 7. 开机自启

已在 install.sh 里通过 `systemctl enable drill-server` 设置。验证：

```bash
systemctl is-enabled drill-server
# 预期: enabled

systemctl list-dependencies multi-user.target | grep drill
# 预期: └─drill-server.service
```

手动控制：

```bash
sudo systemctl start    drill-server
sudo systemctl stop     drill-server
sudo systemctl restart  drill-server
sudo systemctl status   drill-server
```

---

## 8. 日志

所有 stdout/stderr 自动进 **systemd journal**（不再单独写文件）：

```bash
# 实时跟踪（最常用）
sudo journalctl -u drill-server -f

# 今天的
sudo journalctl -u drill-server --since today

# 最近 100 行
sudo journalctl -u drill-server -n 100

# 只看错误
sudo journalctl -u drill-server -p err

# 导出到文件
sudo journalctl -u drill-server --since "2026-11-01" -o cat > drill-server.log
```

### 日志轮转

systemd journal 自带自动轮转，默认：
- 单日志 ≤ 4GB
- 总保留 ≤ 10% 文件系统
- 永久保留（除非磁盘满）

自定义（可选）：`sudo journalctl --vacuum-size=500M` 或改 `/etc/systemd/journald.conf`。

---

## 9. 备份

### 数据库备份（最关键）

```bash
# 完整备份（含所有表和数据）
mysqldump -u drill_app -p drill_system | gzip > backup-$(date +%Y%m%d-%H%M%S).sql.gz

# 只备份 schema（不含数据）
mysqldump -u drill_app -p --no-data drill_system | gzip > schema-$(date +%Y%m%d).sql.gz

# 恢复
gunzip -c backup-xxx.sql.gz | mysql -u drill_app -p drill_system
```

### 上传文件备份

```bash
tar -czf uploads-$(date +%Y%m%d).tar.gz /opt/drill/uploads/
```

### .env 备份

```bash
sudo cp /etc/drill/drill.env /etc/drill/drill.env.bak
# 别用 git 跟踪 .env 文件！
```

### 备份策略建议

- 每天一次 cron（crontab -e）
- 备份文件放远程位置（S3/OSS/NAS），不要只在本机
- 保留 30 天，周备份保留一年

---

## 10. 升级与回滚

### 升级

```bash
# 1) 备份（关键步骤！）
mysqldump -u drill_app -p drill_system | gzip > backup-pre-upgrade-$(date +%Y%m%d-%H%M%S).sql.gz

# 2) 停服务
sudo systemctl stop drill-server

# 3) 编译新版本（Windows 上），拷到目标机覆盖
cp /tmp/new-drill-server /opt/drill/bin/drill-server
sudo chmod 0755 /opt/drill/bin/drill-server

# 4) 跑新 migration（如果有新增）
# migrations/ 目录下 0003_login_lock.up.sql 等需要手动执行
mysql -u drill_app -p drill_system < /opt/drill/migrations/0003_login_lock.up.sql

# 5) 启动
sudo systemctl start drill-server

# 6) 验证
curl http://127.0.0.1:8090/api/health
```

### 回滚

```bash
# 1) 停服务
sudo systemctl stop drill-server

# 2) 恢复旧二进制
cp /opt/drill/bin/drill-server.bak /opt/drill/bin/drill-server

# 3) 数据库回滚（如果升级时 schema 变了，需要对应反向 SQL）
gunzip -c backup-pre-upgrade-xxx.sql.gz | mysql -u drill_app -p drill_system

# 4) 启动
sudo systemctl start drill-server
```

建议升级前先把当前二进制备份：
```bash
cp /opt/drill/bin/drill-server /opt/drill/bin/drill-server.bak-$(date +%Y%m%d-%H%M%S)
```

---

## 11. 常见故障

### 故障 1：MySQL 连不上

**症状**：`drill-server.service` 启动失败，journal 里有 `MySQL 连接失败: 1045 Access denied` 或 `unknown database drill_system`。

**排查**：
```bash
# 1) 检查 DB_HOST/DB_PORT 是否正确
mysql -h 127.0.0.1 -P 3306 -u drill_app -p -e "SELECT 1"
# 换成 .env 里的实际 DB_HOST 试试

# 2) 检查数据库是否存在
mysql -h 127.0.0.1 -u drill_app -p -e "SHOW DATABASES LIKE 'drill_system'"

# 3) 检查业务账号权限
mysql -h 127.0.0.1 -u root -p -e "SHOW GRANTS FOR 'drill_app'@'%'"

# 4) 远程 MySQL 要检查 bind-address 和防火墙
# /etc/mysql/mysql.conf.d/mysqld.cnf → bind-address = 0.0.0.0
# （生产只绑内网 IP，不要绑公网）
```

**修复**：重新跑 bootstrap 或手动 CREATE USER/GRANT。

### 故障 2：端口被占用

**症状**：`Bind: address already in use`。

```bash
# 查是谁占了 8090
ss -ltnp | grep 8090
# 或
sudo lsof -i :8090

# 如果是上次残留的 drill-server，kill 掉
sudo systemctl stop drill-server
sudo pkill -9 -f drill-server
```

### 故障 3：证书权限

**症状**：`TLS_ENABLED=true` 时启动失败，journal 里 `cannot read certificate file`。

```bash
# drill 用户要能读证书文件
sudo chown -R root:drill /etc/drill/certs
sudo chmod 0750 /etc/drill/certs
sudo chmod 0640 /etc/drill/certs/*

# 验证（以 drill 身份读）
sudo -u drill cat /etc/drill/certs/server.key > /dev/null && echo OK
```

### 故障 4：SELinux 拦截（RHEL/CentOS/Rocky）

**症状**：进程能起来但无法绑定端口、无法读/写文件。

**排查**：
```bash
getenforce
# 如果是 Enforcing，看是不是 SELinux 拦了
sudo ausearch -m AVC -ts recent
# 或查 /var/log/audit/audit.log
```

**修复（推荐保留 Enforcing，做正确的 SELinux 策略）**：

```bash
# 如果是自定义端口 8090 绑定
sudo semanage port -a -t http_port_t -p tcp 8090

# 如果是 Nginx 代理到后端
sudo setsebool -P httpd_can_network_connect 1

# 如果是 drill 进程需要写 /opt/drill/data（自定义目录）
sudo semanage fcontext -a -t httpd_sys_rw_content_t "/opt/drill/data(/.*)?"
sudo restorecon -Rv /opt/drill/data
```

**绝对不要**：`setenforce 0` 或修改 `/etc/selinux/config` 设 `SELINUX=disabled`。SELinux 开启能防止 Go 进程被利用后读写敏感系统文件。

### 故障 5：静态资源 /public 404

**症状**：HTML 页面能加载但图片/JS 全 404。

**排查**：
```bash
# 确认 public/ 存在
ls -la /opt/drill/public/

# 如果 install.sh 没找到源目录（你是手动拷的二进制），
# 手动把 public/ 同步过去
cp -ru ~/Emergency/public/. /opt/drill/public/
sudo chown -R drill:drill /opt/drill/public
```

### 故障 6：上传文件失败

**症状**：上传证据/知识库时报错、或保存的文件找不到。

```bash
# drill 用户要有写权限
sudo chown -R drill:drill /opt/drill/uploads
sudo chmod 0755 /opt/drill/uploads

# SELinux 同上，restorecon 一下
sudo restorecon -Rv /opt/drill/uploads

# 反向代理也要放行
# Nginx: client_max_body_size 和 UPLOAD_MAX_MB 对齐
# Caddy: client_max_body_size 20m
```

### 故障 7：Go 进程被 360/杀软拦截

**根因**：Windows 上编译的 `.exe` 会被 360 QVM 扫描，但**Linux ELF 不受 360 影响**（360 是 Windows 平台杀软）。

如果 Linux 上也有安全软件：
- Sophos/ClamAV 等企业杀软可能误报 Go 静态编译产物（因为没有签名）
- 把 `/opt/drill/bin/drill-server` 加到杀软白名单

---

## 12. Docker Compose 部署（可选）

适合快速搭建开发/测试环境，或不想在目标机装 Go/MySQL 的场景。

### 12.1 前置

- Docker + Docker Compose v2（`docker compose`，不是旧版 `docker-compose`）
- 目标机能拉取镜像（alpine、mysql）

### 12.2 启动

```bash
cd Emergency/

# 1) 准备 .env（DB_HOST 必须是 mysql，不是 127.0.0.1）
cp deploy/linux/drill.env.example deploy/linux/.env
# 改 deploy/linux/.env：
#   DB_HOST=mysql
#   MYSQL_ROOT_PASSWORD 必须与 DB_ROOT_PASSWORD 一致
#   其他占位符按实际填

# 2) 启动 MySQL（等它健康）
docker compose -f deploy/linux/docker-compose.yml up -d mysql
docker compose -f deploy/linux/docker-compose.yml logs -f mysql
# 看到 "ready for connections" 后 Ctrl+C

# 3) 构建 app 镜像
docker compose -f deploy/linux/docker-compose.yml build

# 4) 跑 bootstrap（一次性，初始化数据库 + 种子数据）
docker compose -f deploy/linux/docker-compose.yml run --rm bootstrap

# 5) 起 app
docker compose -f deploy/linux/docker-compose.yml up -d app

# 6) 健康检查
curl http://localhost:8090/api/health
```

### 12.3 运维

```bash
# 实时日志
docker compose -f deploy/linux/docker-compose.yml logs -f app

# 重启 app
docker compose -f deploy/linux/docker-compose.yml restart app

# 备份 MySQL
docker compose -f deploy/linux/docker-compose.yml exec mysql \
  mysqldump -uroot -p__YOUR_ROOT_PWD__ drill_system | gzip > backup-$(date +%Y%m%d).sql.gz

# 恢复
gunzip -c backup-xxx.sql.gz | docker compose -f deploy/linux/docker-compose.yml exec -T mysql \
  mysql -uroot -p__YOUR_ROOT_PWD__ drill_system

# 彻底停（含数据卷会丢！）
docker compose -f deploy/linux/docker-compose.yml down -v
```

### 12.4 Compose 与物理机的区别

| 项目 | 物理机 + systemd | Docker Compose |
|------|-----------------|----------------|
| MySQL 管理 | DBA 独立维护 | 命名卷自动持久化 |
| 日志 | journalctl | docker compose logs |
| 开机自启 | systemd enable | compose restart policy |
| 备份 | mysqldump 直接跑 | docker exec mysqldump |
| TLS | 外部代理（Nginx/Caddy） | 外部代理（compose 不内置 HTTPS） |
| 性能 | 最优 | 轻微开销（足够内网用） |
| 升级回滚 | 替换二进制 + 重启 | rebuild + recreate |

### 12.5 Compose 的 bootstrap 只跑一次

`docker-compose.yml` 里 `bootstrap` 服务的 `depends_on` 是 `service_completed_successfully`——compose 在 bootstrap 成功退出后才会启 app。**bootstrap 只需要第一次部署时跑一次**，后续 `docker compose up -d` 只会起 mysql 和 app，不会重跑 bootstrap。

如果你想重新初始化数据库（清空重建）：
```bash
docker compose -f deploy/linux/docker-compose.yml down -v   # 删数据卷
docker compose -f deploy/linux/docker-compose.yml up -d
docker compose -f deploy/linux/docker-compose.yml run --rm bootstrap
```

---

## 13. 信创 / 国产化部署

本程序**架构上天然适合国产信创环境**——`CGO_ENABLED=0` 静态编译，产物不依赖 glibc/musl，换基础镜像不需要改一行代码。本节告诉你哪些国产发行版有现成镜像、哪些需要自己导入 rootfs、哪些架构需要交叉编译。

### 13.1 各发行版适配结论

| 发行版 | 厂商 | Docker Hub 有官方镜像 | 推荐基础镜像 | 架构 | 备注 |
|--------|------|----------------------|--------------|------|------|
| **openEuler 22/24.03** | 华为欧拉 | ✅ 有 | `openeuler/openeuler:24.03-lts` | amd64 + arm64 | **首选**，Docker Hub 直接可拉，dnf 包管理器 |
| **龙蜥 Anolis 8/23** | 阿里巴巴 | ✅ 有 | `openanolis/anolisos:8.8` | amd64 + arm64 | 兼容 CentOS/RHEL 生态 |
| **银河麒麟 V10** | 麒麟软件 | ❌ 没有 | 厂商 registry 或 `docker import rootfs` | amd64 + arm64 + **loong64** | 见下文"麒麟/统信专项" |
| **统信 UOS V20** | 统信软件 | ❌ 没有 | 厂商 registry 或 `docker import rootfs` | amd64 + arm64 | Deepin 系 |
| **中标麒麟 V7/V7.5** | 中标软 | ❌ 老版本 | 厂商 registry | amd64 | 老系统，建议用 openEuler/Anolis 替代 |
| **LoongArch 龙芯** | — | — | 麒麟 V10 / openEuler LoongArch 版 | **loong64** | Windows Docker Desktop **无模拟层**，只能原生构建 |

### 13.2 架构约束（必须知道）

| 架构 | Go 编译参数 | Windows Docker Desktop 能跑？ | 说明 |
|------|------------|-------------------------------|------|
| **amd64 (x86_64)** | 默认，`GOARCH=amd64` | ✅ 直接运行 | 最常用 |
| **arm64 (aarch64)** | `GOARCH=arm64` | ✅ QEMU 模拟能跑但慢 2-5x | 鲲鹏 / 飞腾 CPU 服务器原生跑 arm64 不需要模拟 |
| **loong64 (龙芯 LoongArch)** | `GOARCH=loong64` | ❌ **没有模拟层** | 只能在龙芯物理机上 `docker build` 或 `go build` |

**结论**：信创场景如果是鲲鹏/飞腾服务器，在 **amd64 开发机上交叉编译出 arm64 镜像** 即可交付，Go 本身一条命令搞定。龙芯需要把构建过程搬到龙芯物理机上，或在 CI 里用 LoongArch runner。

### 13.3 两步法（推荐先验证再交付）

国产发行版镜像的细节差异（yum vs dnf、是否预装 procps、时区文件路径）可能导致 Dockerfile 里的 `RUN apk/dnf/yum` 行为不同。推荐两步：

**第一步：openEuler 验证功能（10 分钟）**

```bash
# openEuler 在 Docker Hub 有官方镜像，amd64 可以直接在 Windows Docker Desktop 里模拟跑
docker build -f deploy/linux/Dockerfile \
    --build-arg RUNTIME_BASE=openeuler/openeuler:24.03-lts \
    --build-arg BUILDER_BASE=golang:1.24-openeuler-24.03 \
    -t drill-server:openeuler .

# 跑起来
docker run --rm -p 8090:8090 drill-server:openeuler
# 浏览器开 http://127.0.0.1:8090/api/health
```

**第二步：切目标发行版做交付镜像**

```bash
# 假设目标是麒麟 V10，厂商给了 registry 地址
docker login registry.kylinos.cn
docker build -f deploy/linux/Dockerfile \
    --build-arg RUNTIME_BASE=registry.kylinos.cn/kylin/v10-sp3:latest \
    -t registry.kylinos.cn/your-team/drill-server:1.0.0 .
docker push registry.kylinos.cn/your-team/drill-server:1.0.0
```

### 13.4 麒麟 / 统信 专项（没有 Docker Hub 官方镜像）

#### 方式 A：厂商 registry 拉取（推荐）

向麒麟/统信销售或技术支持要：
- Docker 镜像 registry 地址（如 `registry.kylinos.cn`、`registry.deepin.com`）
- 登录凭据（可能是 OpenID Connect / 账号密码）
- 镜像 tag（如 `v10-sp3-minimal`、`v20-pro-x86_64`）

```bash
docker login registry.kylinos.cn
docker build -f deploy/linux/Dockerfile \
    --build-arg RUNTIME_BASE=registry.kylinos.cn/kylin/v10-sp3-minimal:latest \
    -t drill-server:kylin-v10 .
```

#### 方式 B：自己从 ISO 导入 rootfs

如果拿不到厂商 registry，可以从 ISO 里解 rootfs 出来：

```bash
# 1. 挂载麒麟 V10 ISO（Linux 上）
mkdir -p /mnt/kylin
mount -o loop Kylin-V10.iso /mnt/kylin

# 2. 找 rootfs（通常是 LiveOS/squashfs.img 里的 ext4.img 或直接 rootfs.tar.gz）
find /mnt/kylin -name "rootfs*" -o -name "*.tar.gz"

# 3. 导入为 Docker 镜像
docker import /path/to/rootfs.tar.gz kylin-v10-rootfs

# 4. 修改 Dockerfile 绕过 dnf/yum（因为 docker import 的镜像里没有包管理器）
#    deploy/linux/Dockerfile 的 runtime 段已经做了 shell -lc 探测，
#    如果没 apk/dnf/yum 会打印 WARNING 然后跳过装包——
#    但 HEALTHCHECK 的 wget 和运行时依赖还需要。两种选项：
#
#    选项 a) 用 openEuler/Anolis builder 阶段预装好 wget + tini + ca-certificates，
#           然后拷到麒麟 rootfs 镜像里：
#      # Dockerfile 里再加一个 stage:
#      FROM openeuler/openeuler:24.03-lts AS rtb
#      RUN dnf install -y wget tini ca-certificates && dnf clean all
#      ...
#      FROM kylin-v10-rootfs
#      COPY --from=rtb /usr/bin/wget /usr/bin/wget
#      COPY --from=rtb /usr/sbin/tini /usr/sbin/tini
#      COPY --from=rtb /etc/ssl/certs /etc/ssl/certs
#
#    选项 b) 用更激进的 distroless 思路：HEALTHCHECK 干脆去掉，
#           运行时不需要任何系统包（Go 二进制静态编译，自带网络栈）：
#      # Dockerfile 里删掉 apk/dnf/yum RUN 和 HEALTHCHECK
```

### 13.5 Dockerfile 已做的兼容性设计

`deploy/linux/Dockerfile` 里 runtime 阶段用 `SHELL ["/bin/bash", "-lc"]`，然后按顺序探测包管理器：

```bash
(command -v apk && ...) ||     # Alpine
(command -v dnf && ...) ||     # openEuler/Anolis/麒麟V10/统信UOS 新系统
(command -v yum && ...) ||     # RHEL7/麒麟V7 旧系统
echo "WARNING: no package manager found"
```

- 有哪个就用哪个，装出 `wget`（HEALTHCHECK）和时区文件
- **都没有**（纯 rootfs）→ 跳过装包，HEALTHCHECK 用 `|| true` 兜底
- 如果麒麟/UOS rootfs 连 `/bin/bash` 都没有，把第一行改成 `SHELL ["/bin/sh", "-c"]` 即可

### 13.6 Windows Docker Desktop 上测国产镜像的注意点

| 场景 | 怎么做 | 风险 |
|------|--------|------|
| openEuler / Anolis amd64 | 直接 `--build-arg RUNTIME_BASE=...` | 低，Docker Hub 镜像 Windows DD 能拉能跑 |
| openEuler / Anolis arm64 | `--build-arg GOARCH=arm64 --platform linux/arm64/v8` | 有 QEMU 模拟，慢 2-5x，开发验证够用 |
| 麒麟 V10 amd64 | 拿到镜像名后直接 build | 取决于麒麟镜像是否走标准 dnf |
| 麒麟 V10 arm64 | 同上 + `--build-arg GOARCH=arm64` | 建议在鲲鹏物理机上实测一次 |
| 龙芯 LoongArch | **Windows DD 跑不了** | 必须在龙芯物理机/国产 CI 上 `docker build` |

---

## 附录 A：文件权限速查表

| 路径 | 属主 | 权限 | 说明 |
|------|------|------|------|
| `/etc/drill/drill.env` | `root:root` | `0640` | 含口令，非 root 不能读 |
| `/etc/drill/certs/` | `root:drill` | `0750` | drill 进程需进入目录 |
| `/etc/drill/certs/*.crt` | `root:drill` | `0644` | 证书可公开 |
| `/etc/drill/certs/*.key` | `root:drill` | `0640` | 私钥仅限 drill 进程 |
| `/opt/drill/bin/` | `drill:drill` | `0755` | 二进制可执行 |
| `/opt/drill/data/` | `drill:drill` | `0755` | 运行时写（备份文件在此） |
| `/opt/drill/uploads/` | `drill:drill` | `0755` | 运行时写（上传文件在此） |
| `/opt/drill/migrations/` | `drill:drill` | `0755` | bootstrap 读 |
| `/opt/drill/public/` | `drill:drill` | `0755` | 静态前端只读 |

## 附录 B：systemd 加固选项说明

`drill-server.service` 里几个关键加固项：

- `NoNewPrivileges=true`：子进程不能通过 suid 二进制提权
- `ProtectSystem=strict`：只读 /usr /boot /etc，进程只能写 `/opt/drill/data`（通过 ReadWritePaths）
- `ProtectHome=true`：屏蔽 /home 下所有目录
- `PrivateTmp=true`：进程的 /tmp 是隔离的，避免 temp file 攻击
- `ProtectKernel*`：禁止修改内核参数、加载内核模块、访问 cgroup
- `LimitNOFILE=65535`：避免大量 file descriptor 耗尽（WebSocket + 上传场景）

如果需要修改，请在充分理解后果后调整。**不要为了"省事"去掉加固选项**。
