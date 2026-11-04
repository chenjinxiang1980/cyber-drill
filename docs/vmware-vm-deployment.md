# VMware Linux VM 部署指南

> ⚠️ **未实测声明**：本文档基于 `deploy/linux/` 交付包 + 作者对 VMware 网络与 Linux Docker 的常规认知编写，**作者手里没有这台 VM 的访问权限，也没有 WSL/Docker 可本地验证**。所有命令、网络判断、端口映射规则均属标准做法，但首次部署请严格按文末"首次部署检查表"逐项确认，遇到问题对照 `docs/linux-deployment.md` 常见故障章节排查。

---

## 目录

1. [前置条件](#1-前置条件)
2. [VM 网络：桥接 vs NAT](#2-vm-网络桥接-vs-nat)
3. [路径 A：Docker Compose](#3-路径-adocker-compose)
4. [路径 B：systemd 直接跑 ELF](#4-路径-bsystemd-直接跑-elf)
5. [数据导入：dump vs bootstrap](#5-数据导入dump-vs-bootstrap)
6. [首次启动顺序（完整版）](#6-首次启动顺序完整版)
7. [客户端可访问性验证](#7-客户端可访问性验证)
8. [常见问题](#8-常见问题)
9. [首次部署检查表](#9-首次部署检查表)

---

## 1. 前置条件

**VM 里需要**：
- Linux x86_64（推荐 Debian 12 / Ubuntu 22.04 / openEuler 24.03）
- 二选一：
  - **路径 A**：Docker + Docker Compose v2（`docker compose version` 能输出版本号）
  - **路径 B**：MySQL 8.x + systemd（几乎所有现代发行版都有）
- 非 root 用户（能用 sudo）
- SSH 或 VMware 控制台

**Windows 侧需要**：
- VMware Workstation / Player / vSphere（VMware Tools 建议装上，更好的剪贴板+时间同步）
- 能从旧 Windows 机器拷文件到 VM（scp / WinSCP / VMware 共享文件夹 / 网络共享任选）

**拷过去的文件**：
- `drill-linux-bundle.tar.gz` —— 打包脚本产出
- `drill_system-dump.sql` —— MySQL dump（`scripts/dump-mysql-for-vm.ps1` 产出）

---

## 2. VM 网络：**NAT（已确定）**

VMware 网络你已经选了 NAT——这意味着：

```
局域网其他电脑 / 大屏浏览器
    ↓ HTTP → Windows 宿主:8091 (阶段一) 或 8090 (阶段二)
    ↓ VMware NAT 端口转发（vmnetnat.conf）
VM (192.168.x.x:8090)
    ↓
Go 后端 → MySQL（容器内）
```

**访问地址的关键区别**：
- **桥接模式**：浏览器访问 `<VM-IP>:8090`（VM 直接拿到局域网 IP）
- **NAT 模式**：浏览器访问 `<宿主IP>:8091` 或 `<宿主IP>:8090`（走宿主，不是 VM IP）

### 2.1 vmnetnat.conf 位置

在 **Windows 宿主** 上，以**管理员**身份编辑：

```
C:\ProgramData\VMware\vmnetnat.conf
```

> 这个文件默认是隐藏的资源管理器里看不到，必须用 PowerShell/CMD 或 Notepad++ 直接打开路径。

### 2.2 两阶段端口方案（关键！）

**已知冲突**：宿主 Windows 上 **8090 端口正被现有 Go 服务占用**，VMware NAT 转发规则若写 `8090 = <VM-IP>:8090` 会撞宿主监听。分两阶段平滑过渡：

| 阶段 | 宿主 Go 状态 | 宿主端口 | vmnetnat.conf 里写 | 局域网访问地址 |
|------|-------------|---------|---------------------|---------------|
| **阶段一**（VM 先跑起来，宿主 Go 还在） | ✅ 跑着 | 8091 | `8091 = <VM-IP>:8090` | `http://<宿主IP>:8091` |
| **阶段二**（确认 VM 跑通，停掉宿主 Go） | ❌ 已停 | 8090 | `8090 = <VM-IP>:8090` | `http://<宿主IP>:8090` |

**前端零改动**：Go 后端返回的前端页面用的是**相对路径**（`/api/auth/captcha`、`/screen.html`），不管监听在哪个端口、转发到哪个端口，浏览器都只会访问当前 URL 同源。切阶段时浏览器不需要改任何地址，只是 URL 里端口号变了（从 8091 变 8090）。

### 2.3 vmnetnat.conf 示例片段（阶段一）

在文件里找到 `[incomingtcp]` 段，**追加**（不要删已有规则）：

```ini
# 示例：VM 的 NAT 网卡 IP 是 192.168.230.130（先在 VM 里 ip addr show 确认）
# 阶段一：宿主 8091 → VM 8090
8091 = 192.168.230.130:8090
# （可选）HTTPS 转发：宿主 443 → VM 443（VM 内 nginx/Caddy 终止 TLS 时用）
# 443 = 192.168.230.130:443
```

阶段二把第一行改成 `8090 = 192.168.230.130:8090` 即可。

### 2.4 改完 vmnetnat.conf 必须重启 VMware NAT Service

**这步需要管理员权限，由你在 Windows 宿主上执行（我不代跑）**：

```powershell
# 以管理员身份开 PowerShell
Restart-Service vmware-nat
# 可选：如果 DHCP 也改过（见下一节），一起重启
Restart-Service vmware-dhcp
# 验证服务起来了
Get-Service vmware-nat, vmware-dhcp
# 预期 Status=Running
```

VMware NAT Service 停止后宿主还能上网，但端口转发暂时失效——重启完立即恢复。**不会影响 VM 内部任何东西，不会断 SSH**。

### 2.5 VM 静态 IP（防止 NAT 转发规则失效）

NAT 下 VM 的 IP 由 VMware DHCP 分配，**重启 VM 可能变**，vmnetnat.conf 里写死的目标 IP 就失效了。两种固定办法：

**办法 A：VM 内设静态 IP（推荐，简单）**

```bash
# 在 VM 里（以 Debian/Ubuntu 为例）
sudo nmcli connection modify "Wired connection 1" \
    ipv4.method manual \
    ipv4.addresses 192.168.230.130/24 \
    ipv4.gateway 192.168.230.2 \
    ipv4.dns 192.168.230.2
sudo nmcli connection down "Wired connection 1"
sudo nmcli connection up   "Wired connection 1"
# VMware NAT 网关默认是 .2（230 是 VMware 默认 NAT 网段）
# 先在 VM 里 ip route show 看一下默认网关再填
```

**办法 B：宿主 vmnetdhcp.conf 按 MAC 绑定**

在 **Windows 宿主** 上编辑 `C:\ProgramData\VMware\vmnetdhcp.conf`：

```
host drill-vm {
    hardware ethernet 00:0c:29:ab:cd:ef;    # VM 网卡 MAC（在 VM 里 cat /sys/class/net/ens33/address 查）
    fixed-address     192.168.230.130;
}
```

改完同样 `Restart-Service vmware-dhcp`。VM 重启后 DHCP 就会给这个 MAC 固定 IP。

### 2.6 自检（三层，必须全过）

在**宿主 Windows 上**（PowerShell）：
```powershell
# 层 1：宿主自通（转发环回）
curl http://127.0.0.1:8091/api/health
# 预期 {"ok":true,"db":"up",...}

# 层 2：宿主自通（用宿主真实 IP，不是 127.0.0.1）
curl http://(Get-NetIPAddress -AddressFamily IPv4 | Where-Object {$_.IPAddress -ne "127.0.0.1"} | Select-Object -First 1).IPAddress:8091/api/health
# 预期 200

# 层 3：另一台局域网电脑（浏览器或 curl 都行）
curl http://<宿主IP>:8091/api/health
# 预期 200。**这步最关键**——只有这步过了，大屏/其他电脑才看得到
```

在 **VM 里**（分层排查用）：
```bash
# 层 0：VM 内部自通（绕过 NAT，直接连容器）
curl http://127.0.0.1:8090/api/health
# 预期 200。如果这步挂了，NAT 转发根本不是问题，是 Go/MySQL 容器没起好
```

**分层定位**：
- VM 内部 `curl 127.0.0.1:8090` 200 + 宿主 `curl 127.0.0.1:8091` 超时 → **vmnetnat.conf 没写/写错/服务没重启**
- VM 内部 OK + 宿主自通 OK + 局域网其他电脑不通 → **宿主 Windows 防火墙挡了 8091**（在 Windows Defender 防火墙入站规则里允许 TCP 8091）

### 2.7 HTTPS 落点（NAT 模式下）

NAT 转发**只转发明文 TCP 流**，不能在转发层终止 TLS。两种正确做法：

| 做法 | HTTPS 在哪终止 | NAT 转发 | 代价 |
|------|---------------|---------|------|
| **VM 内 nginx/Caddy**（推荐） | VM 内 | 宿主 8091→VM 8090（HTTP 开发期）；或宿主 443→VM 443（生产 HTTPS） | 多配一个 vmnetnat.conf 规则（443 = VM-IP:443） |
| Windows 宿主侧终止 | Windows（Nginx/Caddy/Node 反代） | 宿主 8091→VM 8090（仅 HTTP） | 宿主要装/起 Web 服务器；Go 的 TRUSTED_PROXIES 要加宿主真实 IP 段 |

**不要做**：在 vmnetnat.conf 的 8091 端口上直接期望 HTTPS 生效——转发层不理解 TLS，浏览器会报"连接被重置"或协议错误。

开发期（阶段一）建议直接用 **HTTP**（`http://<宿主IP>:8091`）跑通，阶段二确认稳定后再切 HTTPS（在 VM 内装 Caddy 最简单）。

---

## 3. 路径 A：Docker Compose

VM 里已有 Docker。**这条路径推荐**——隔离性好、回滚方便、不受宿主文件系统影响。**MySQL 用容器内实例**（已确定）。

### 3.1 准备 .env

```bash
cd deploy/linux
cp drill.env.example .env
vi .env
```

**必须改**（VMware NAT 阶段一）：
```ini
# ---- MySQL（compose 内部，写死不要动）----
DB_HOST=mysql
DB_PORT=3306
DB_NAME=drill_system
DB_USER=__REPLACE_WITH_DB_USER__
DB_PASSWORD=__REPLACE_WITH_DB_PASSWORD__
DB_ROOT_USER=root
DB_ROOT_PASSWORD=__REPLACE_WITH_DB_ROOT_PASSWORD__   # 必须和 compose.yml 里 MYSQL_ROOT_PASSWORD 一致

# ---- 关键：VMware NAT 阶段一设 8091 ----
APP_HOST_PORT=8091        # 阶段一（宿主 8090 被占）设 8091；阶段二改回 8090

# ---- 安全与代理 ----
JWT_SECRET=__REPLACE_WITH_JWT_SECRET__      # openssl rand -hex 32 生成
SESSION_SECRET=__REPLACE_WITH_SESSION_SECRET__
TRUSTED_PROXIES=127.0.0.1/32
```

**compose.yml 里的 MYSQL_* 值必须与 .env 里的 DB_* 一致**：
- `MYSQL_ROOT_PASSWORD` = `DB_ROOT_PASSWORD`
- `MYSQL_DATABASE`     = `DB_NAME`（已写死 `drill_system`，两边默认一致）
- `MYSQL_USER`         = `DB_USER`
- `MYSQL_PASSWORD`     = `DB_PASSWORD`

### 3.2 启动顺序（VMware NAT + 已有 dump）

```bash
# 1) 起 MySQL（先让它 ready）
docker compose up -d mysql

# 2) 等 MySQL healthy
docker compose ps mysql
# 预期: drill-mysql  healthy（10 秒级）

# 3) 导入旧数据（**只导一次**，然后跳过 bootstrap）
#    把 drill_system-dump.sql 放到 deploy/linux/ 目录（bundle 解压后就在一起）
docker compose exec -T mysql \
    mysql -uroot -p"${DB_ROOT_PASSWORD}" drill_system \
    < ../../drill_system-dump.sql

# 4) 验证数据
docker compose exec mysql mysql -uroot -p"${DB_ROOT_PASSWORD}" drill_system \
    -e "SELECT COUNT(*) FROM users; SELECT COUNT(*) FROM script_nodes;"
# users ≈ 12 / script_nodes ≈ 331

# 5) 起 app（**不要**跑 bootstrap——数据已经在库里了）
docker compose up -d app

# 6) 健康（VM 内部自通）
sleep 5
curl http://127.0.0.1:8090/api/health
# 预期 {"ok":true,"db":"up",...}
```

### 3.3 VMware NAT 宿主侧（你手动操作，我不代跑）

**以上步骤完成后，Windows 宿主上**：

```ini
# C:\ProgramData\VMware\vmnetnat.conf
# 追加到 [incomingtcp] 段
8091 = <VM-IP>:8090
```

```powershell
# 管理员 PowerShell
Restart-Service vmware-nat
```

然后在**另一台局域网电脑**上浏览器打开：`http://<宿主IP>:8091`。

### 3.4 阶段二切换（VM 跑通，宿主 Go 可以停了）

```bash
# VM 里：改 .env
vi .env          # APP_HOST_PORT=8091 → 8090（或直接删掉这行，默认就是 8090）

# VM 里：重起 app
docker compose up -d app    # 仅 app 重建，MySQL 不动

# Windows 宿主：改 vmnetnat.conf（管理员）
#   8091 = <VM-IP>:8090  →  8090 = <VM-IP>:8090
# 然后 Restart-Service vmware-nat

# 浏览器现在访问 http://<宿主IP>:8090 就是 Go 容器了
```

**宿主 Go 服务停不停由你决定**。VM Go 容器跑起来后，业务可以随时切到容器端（改 NAT 转发端口 + 浏览器刷新）。

### 3.5 可选：跑 bootstrap（仅空库、没有 dump 时）

```bash
docker compose down -v    # 清 MySQL 数据卷
docker compose up -d mysql
# bootstrap 服务的 depends_on 是 service_completed_successfully
# 直接 docker compose up -d 会自动起 bootstrap，跑完再起 app
docker compose up -d
```

### 3.6 镜像换国产发行版（可选）

```bash
# openEuler（验证用）
docker build -f Dockerfile \
    --build-arg RUNTIME_BASE=openeuler/openeuler:24.03-lts \
    -t drill-server:openeuler .

# 麒麟 V10（如果厂商给了 registry）
docker build -f Dockerfile \
    --build-arg RUNTIME_BASE=registry.kylinos.cn/kylin/v10-sp3-minimal:latest \
    -t drill-server:kylin .

# 详见 docs/linux-deployment.md 第 13 节
```

---

## 4. 路径 B：systemd 直接跑 ELF

VM 里没装 Docker 或不想用容器。MySQL 可以在 VM 内，也可以在外部。

### 4.1 安装 MySQL（如果没装）

```bash
# Debian/Ubuntu
sudo apt update && sudo apt install -y mysql-server
# openEuler / RHEL 系
sudo dnf install -y mysql-server
# UOS Server 20（**重点！**——用 dnf，没有 apt）
sudo dnf install -y mysql-server
# 启 MySQL 并开机自启
sudo systemctl enable --now mysqld  # openEuler/RHEL/UOS
sudo systemctl enable --now mysql   # Debian/Ubuntu
```

### 4.2 建库 + 导数据

```bash
# 1) 建库
sudo mysql -e "CREATE DATABASE drill_system DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# 2) 建业务账号
sudo mysql -e "CREATE USER 'drill'@'%' IDENTIFIED BY 'REPLACE_ME'; \
               GRANT ALL ON drill_system.* TO 'drill'@'%'; \
               FLUSH PRIVILEGES;"

# 3) 导入旧数据
sudo mysql drill_system < drill_system-dump.sql
```

### 4.3 跑 install.sh

```bash
# 解压 bundle 后
sudo bash deploy/linux/install.sh

# 写 .env
sudo cp deploy/linux/drill.env.example /etc/drill/drill.env
sudo vi /etc/drill/drill.env
# 改 DB_HOST=127.0.0.1（MySQL 在 VM 内）或远程 IP
# 改 DB_USER / DB_PASSWORD / DB_ROOT_* / JWT_SECRET / SESSION_SECRET

# 权限
sudo chown root:root /etc/drill/drill.env
sudo chmod 0640      /etc/drill/drill.env

# 启服务
sudo systemctl start drill-server
sudo systemctl enable drill-server

# 健康检查
curl http://127.0.0.1:8090/api/health
```

### 4.4 Nginx / Caddy（可选，生产 HTTPS）

```bash
# Nginx：
sudo cp deploy/linux/nginx-drill.conf /etc/nginx/conf.d/drill.conf
sudo sed -i 's/__REPLACE_WITH_DOMAIN__/drill.example.com/g' /etc/nginx/conf.d/drill.conf
# certbot 自动签证书，或手动放 /etc/letsencrypt/...
sudo nginx -t && sudo systemctl reload nginx

# Caddy（更简洁）：
sudo cp deploy/linux/Caddyfile.example /etc/caddy/Caddyfile
sudo sed -i 's/__REPLACE_WITH_DOMAIN__/drill.example.com/g' /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Go 进程的 `.env` 同步改：
```ini
TRUSTED_PROXIES=127.0.0.1/32   # Nginx/Caddy 在同机
```

---

## 5. 数据导入：dump vs bootstrap

**二选一，不要都做。**

| 场景 | 该做 | 不该做 |
|------|------|--------|
| 手里有旧系统的 `drill_system-dump.sql` | 导入 dump | 跑 bootstrap |
| 手里只有空 MySQL + migrations/ | 跑 bootstrap（建表 + 写 seed） | 导 dump（没数据可导） |
| 导了 dump 但没业务账号密码 | 先 `CREATE USER` 建账号，再导 dump（dump 里不含用户） | —— |

**怎么判断库里有多少数据**：
```bash
mysql -u root -p drill_system -e "
SELECT 'users' AS tbl, COUNT(*) AS cnt FROM users
UNION ALL SELECT 'roles', COUNT(*) FROM roles
UNION ALL SELECT 'script_nodes', COUNT(*) FROM script_nodes
UNION ALL SELECT 'projects', COUNT(*) FROM projects;
"
# users ≥ 12 说明旧数据在；script_nodes ≥ 300 说明剧本也在
```

**为什么不能都做**：bootstrap 会 DROP USER / CREATE DATABASE / 重写 schema，导了 dump 再跑 bootstrap 等于把你精心准备的数据清空重写。

---

## 6. 首次启动顺序（完整版，VMware NAT + 宿主 8090 冲突）

```
[0. VMware 静态 IP（防止 NAT 转发失效）]
# 在 VM 里查当前 NAT 网卡 IP 和 MAC
ip addr show
ip route show             # 记下默认网关（通常 192.168.230.2）
cat /sys/class/net/ens33/address   # MAC，给 vmnetdhcp.conf 绑定

[1. 拷文件]
scp drill-linux-bundle.tar.gz drill_system-dump.sql user@<VM-IP>:~/

[2. VM 里解压 + 起 MySQL]
ssh user@<VM-IP>
cd ~ && tar -xzf drill-linux-bundle.tar.gz
cd deploy/linux

# 写 .env（阶段一：APP_HOST_PORT=8091）
cp drill.env.example .env
vi .env
# 必须改：
#   DB_HOST=mysql（compose 内部 DNS，写死）
#   APP_HOST_PORT=8091（阶段一）
#   MYSQL_* 值 = DB_* 值（compose.yml 里）
#   JWT_SECRET / SESSION_SECRET（openssl rand -hex 32）

# 起 MySQL
docker compose up -d mysql
docker compose ps mysql    # 等 healthy

[3. 导数据]
docker compose exec -T mysql \
    mysql -uroot -p"${DB_ROOT_PASSWORD}" drill_system \
    < ../../drill_system-dump.sql
# 验证：users=12、script_nodes=331
docker compose exec mysql mysql -uroot -p"${DB_ROOT_PASSWORD}" drill_system \
    -e "SELECT COUNT(*) FROM users, COUNT(*) FROM script_nodes"

[4. 起 app]
docker compose up -d app

[5. VM 内健康（层 0）]
curl -s http://127.0.0.1:8090/api/health | python3 -m json.tool
# 预期 {"ok":true,"db":"up",...}  → 这步过了再去配 NAT 转发

[6. 宿主 Windows：配 NAT 转发（层 1-3）]
# 管理员 PowerShell
# C:\ProgramData\VMware\vmnetnat.conf [incomingtcp] 段追加：
#   8091 = <VM-IP>:8090
Restart-Service vmware-nat

# 宿主自通
curl http://127.0.0.1:8091/api/health
# 宿主真实 IP 通
curl http://(Get-NetIPAddress -AddressFamily IPv4 | Where-Object {$_.IPAddress -ne "127.0.0.1"} | Select-Object -First 1).IPAddress:8091/api/health

# 另一台局域网电脑验证（最关键，宿主 Windows 上做不了）
# 浏览器打开 http://<宿主IP>:8091
# 预期登录页 → 登录 → 大屏全显示 → api/health JSON OK

[7. 阶段二切换（可选，等你停掉宿主 Go 后）]
# VM 里：APP_HOST_PORT=8091 → 8090，docker compose up -d app
# 宿主：vmnetnat.conf 8091=VM-IP:8090 → 8090=VM-IP:8090，Restart-Service vmware-nat
# 浏览器地址变成 http://<宿主IP>:8090
```

---

## 7. 客户端可访问性验证（NAT 场景）

**访问的是宿主 IP + 8091**，不是 VM IP。

**桌面浏览器**（Windows 宿主上）：
- `http://127.0.0.1:8091/`（层 1：转发回环）
- `http://<宿主真实IP>:8091/`（层 2：宿主真实 IP）
- 两者都 200 才算转发通了

**其他电脑浏览器**（层 3，最关键）：
- 同局域网任何电脑直接打开 `http://<宿主IP>:8091`
- 登录后大屏正常渲染，F12 Console/Network 无红色报错

**如果只有宿主能访问、其他电脑不行**：宿主 Windows Defender 防火墙挡了 8091 入站。管理员 PowerShell：
```powershell
New-NetFirewallRule -DisplayName "Drill VM Phase1 8091" -Direction Inbound -Protocol TCP -LocalPort 8091 -Action Allow
```

**大屏 1920×1080 布局**：
- 浏览器窗口调成 1920×1080（Chrome DevTools Device Toolbar）
- 按 F11 全屏打开 `screen.html`
- 检查 KPI 卡片、时序图、告警色块的位置是否错位

---

## 8. 常见问题

### Q1：容器/Go 进程连不上 MySQL
```
# 路径 A（compose）：
docker compose exec mysql mysql -uroot -p -e "SELECT 1"
# 通 → MySQL OK，问题在 app 容器的 DB_HOST
# app 容器 DB_HOST 必须是 compose 内部 DNS 名：mysql

# 路径 B（systemd）：
mysql -h 127.0.0.1 -u root -p -e "SELECT 1"
# .env 里 DB_HOST=127.0.0.1
```

### Q2：VM 时钟漂移 → JWT/TLS 异常
```bash
# 检查
timedatectl
# 预期: NTP service: active, System clock synchronized: yes

# 同步
sudo timedatectl set-ntp true
# 或手动
sudo ntpdate -u time.windows.com
```

### Q3：VMware NAT 转发不通（最常见）
```
分层定位：
层 0（VM 内部）：curl 127.0.0.1:8090/api/health → 200 ？
  否 → MySQL/Go 容器没起好，与 NAT 无关
层 1（宿主回环）：curl 127.0.0.1:8091/api/health → 200 ？
  否 → vmnetnat.conf 没写、写错了、或 vmware-nat 没重启
层 2（宿主真实 IP）：curl <宿主IP>:8091 → 200 ？
  否 → 宿主防火墙挡了 8091 入站
层 3（其他电脑）：curl <宿主IP>:8091 → 200 ？
  否 → 同层 2；或交换机 VLAN 隔离
```

### Q4：宿主端口 8090 / 8091 冲突
```powershell
# Windows 上查
Get-NetTCPConnection -State Listen | Where-Object {$_.LocalPort -in 8090,8091,3306}

# 8090 被旧 Go 占着是正常的（阶段一就是绕开它用 8091）
# 8091 如果也被占 → 换 8092（APP_HOST_PORT + vmnetnat.conf 同步改）
```

### Q5：容器内连不上宿主 MySQL
```
Docker 桥接网络里容器要访问宿主：
  - compose 用 host.docker.internal（Docker Desktop 特有，VM 里 Docker 没有）
  - VM 内 Docker → 用宿主真实 IP（不是 127.0.0.1！）
  - MySQL bind-address 改成 0.0.0.0

docker run --rm busybox wget -qO- http://<宿主IP>:3306
# 通 → MySQL OK；不通 → bind-address 或防火墙问题
```

### Q6：nginx/Caddy 反向代理后接口返回 429
```
TRUSTED_PROXIES 没配对 → Gin 没信任反向代理，ClientIP 取了代理 IP
改成 TRUSTED_PROXIES=127.0.0.1/32（Nginx/Caddy 在同机）
```

### Q7：`docker compose up` 提示 docker daemon 未连接
```bash
sudo systemctl start docker
# openEuler/RHEL/UOS 系
sudo systemctl enable --now docker
```

### Q8：UOS Server 20 上 `systemctl start docker` 失败、或者 socket 文件丢失（UOS 特有）

**现象**：`docker ps` 报错 "Cannot connect to the Docker daemon at unix:///run/docker.sock"，`ls -la /run/docker.sock` 不存在，但 `systemctl status docker` 显示 active（或两个都 active）。

**根因**：UOS Server 20 的 systemd 对 Docker 有 **socket activation** 机制——`docker.socket` 和 `docker.service` 同时存在且都尝试绑定 `/run/docker.sock`，互相冲突导致 socket 文件创建失败。

**修复（永久）**：
```bash
# 1) 停掉两个
sudo systemctl stop docker docker.socket
# 2) 清掉可能残留的 socket 文件
sudo rm -f /run/docker.sock /var/run/docker.sock
# 3) 确保两个都 unmasked（masked 反而会导致 docker.service 启动时依赖检查失败）
sudo systemctl unmask docker docker.socket
# 4) 只启动 docker.service（它内部会自动 enable docker.socket 作为依赖）
sudo systemctl start docker.service
# 5) 验证
sudo systemctl is-active docker              # active
ls -la /run/docker.sock                      # srw-rw----, 存在
sudo docker version                          # Client + Server 都有输出
```

**不要做**：
- 不要 `systemctl mask docker.socket`——UOS 上 mask 后 docker.service 会因为 `Requires=docker.socket` 而启动失败
- 不要用 `dockerd &` 手跑——绕过 systemd 会丢失日志、健康检查、重启策略

### Q9：MySQL 容器 Permission denied on `/docker-entrypoint-initdb.d`

**现象**：compose 里挂载 `./migrations:/docker-entrypoint-initdb.d` 后，MySQL 容器第一次启动时退出码 2，日志里是 "Permission denied" 访问 initdb 目录。

**根因**：`docker-entrypoint-initdb.d` 下的脚本由 **mysql 用户**（UID 999）在容器内执行，但宿主上该目录的 owner 可能是 VM 的登录用户（drill）。Docker bind mount 的 UID/GID 直接穿透，容器内 mysql 用户没有读权限。

**修复方案二选一**：
```bash
# 方案 A：改宿主目录 owner（推荐，简单）
sudo chown -R 999:999 ./migrations
sudo chmod -R 755 ./migrations

# 方案 B：不挂 initdb，手动导 dump（更可控）
#   compose.yml 里删掉 docker-entrypoint-initdb.d 的 volume 映射
#   MySQL healthy 后手动：
sudo docker compose exec -T mysql \
    mysql -uroot -p"${DB_ROOT_PASSWORD}" drill_system \
    < drill_system-dump.sql
```

**本项目实测用方案 B**——migrations/ 目录只在首次 bootstrap 需要，有 dump 就不需要 initdb。

### Q10：Dockerfile 里 `COPY --chmod` 或 `RUN chmod +x` 不生效（legacy builder 问题）

**现象**：Windows 上交叉编译出的静态 ELF（`drill-server`）COPY 进镜像后，容器内运行报 "ExitCode 126: permission denied"。

**根因**：旧版 Docker (≤23.x) 默认用 legacy builder，对 `COPY --chmod=+x` 参数不识别（静默忽略）。即使写了 `RUN chmod +x /opt/drill/bin/drill-server`，如果文件权限是从宿主拷贝的（Windows 上 git clone 后可能是 644），`chmod` 也可能失败或产生不可预期的结果。

**修复**：
```dockerfile
# 方法 1：显式 RUN chmod（legacy builder 兼容）
COPY deploy-linux/drill-server /opt/drill/bin/drill-server
RUN chmod 755 /opt/drill/bin/drill-server

# 方法 2：用 buildx（推荐但需要安装）
# docker buildx build --platform linux/amd64 -t drill-server:latest .
```

**本项目实测用方法 1**（legacy builder + 显式 `RUN chmod +x`）。

### Q11：VMware NAT 下 `docker pull` 一直 EOF / timeout

**现象**：`docker pull mysql:8.0` 下载图层时卡在 6-7 层后 EOF/timeout，或 `curl -I https://registry-1.docker.io/v2/` 一直 EOF。

**根因**：
1. **MTU 问题**（最常见）：VMware NAT 默认 MTU 1500，TCP 大包（>1472 字节）在 NAT 层被分片后丢包
2. **docker.io 直连不通**：Docker Hub 的 token endpoint (`auth.docker.io`) 在国内经常 EOF，不走代理或镜像站拉不动
3. **DNS 被宿主 Clash/TUN 干扰**：返回 `198.18.x.x` 假 IP

**修复（三层递进）**：
```bash
# 层 1：先修 MTU（VM 内）
ip link set ens33 mtu 1400            # 临时（重启丢失）
# 永久（UOS nmcli）：
nmcli connection modify ens33 ethernet.mtu 1400 && nmcli connection up ens33

# 层 2：配 domestic mirror（VM 内，daemon.json）
sudo mkdir -p /etc/docker
sudo tee /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": ["https://docker.1ms.run", "https://docker.m.daocloud.io"],
  "dns": ["223.5.5.5", "119.29.29.29"]
}
EOF
sudo systemctl daemon-reload && sudo systemctl restart docker

# 层 3：如果走宿主代理（VMware NAT 出网）
sudo tee /etc/systemd/system/docker.service.d/http-proxy.conf <<'EOF'
[Service]
Environment="HTTP_PROXY=http://<宿主机代理地址>:7890"
Environment="HTTPS_PROXY=http://<宿主机代理地址>:7890"
Environment="NO_PROXY=localhost,127.0.0.1,mysql,app,<VM 所在网段>"
EOF
sudo systemctl daemon-reload && sudo systemctl restart docker

# 验证：
docker pull hello-world                # 快速测试
```

> **代理配置是部署期临时手段**——拉完镜像后 `/etc/systemd/system/docker.service.d/http-proxy.conf` 可以删掉（`rm` 后 `daemon-reload && restart docker`），不影响已拉下来的镜像。daemon.json 的 registry-mirrors 可以保留，长期加速。

### Q12：VMware NAT 下 VPN 代理干扰 DNS（`198.18.x.x` 假 IP）

**现象**：`getent hosts registry-1.docker.io` 返回 `198.18.x.x` 段 IP（不是真实公网 IP），`curl -I https://registry-1.docker.io/v2/` 返回 EOF。

**根因**：宿主 Windows 上的 Clash/Surge 等 VPN 在 TUN 模式下会劫持 DNS，返回 `198.18.0.0/15` 段的 fake-ip。VMware NAT 继承宿主 DNS 转发，VM 内 resolve.conf 里的 nameserver 可能是 VMware NAT 的 DNS 地址（`192.168.<NAT 网段>.2`），它缓存了 fake-ip。

**修复**：
```bash
# VM 内：改 DNS 直连公共解析（绕过 VMware NAT DNS）
# 备份后直接写（临时）
sudo cp /etc/resolv.conf /etc/resolv.conf.bak
echo -e "nameserver 223.5.5.5\nnameserver 119.29.29.29" | sudo tee /etc/resolv.conf

# 如果 NetworkManager 会覆盖，改 nmcli：
sudo nmcli connection show                      # 找到连接名（如 "Wired connection 1" 或 "ens33"）
sudo nmcli connection modify ens33 ipv4.dns "223.5.5.5 119.29.29.29"
sudo nmcli connection modify ens33 ipv4.ignore-auto-dns yes
sudo nmcli connection up ens33

# 验证：
getent hosts registry-1.docker.io              # 应该返回真实公网 IP（不是 198.18.x.x）
curl -sS -I --max-time 8 https://registry-1.docker.io/v2/  # 应该返回 401（未认证，正常）
```

**宿主侧（Windows）**：临时关掉 VPN TUN 模式（或 Clash 里改 DNS 配置），或者确保"允许局域网连接"已开启让 VM 能走宿主代理。

### Q13：容器重建后前端改动丢失（public/ 是 COPY 进镜像的）

**现象**：改了 `public/*.js` 或 `styles.css`，`docker compose up -d app` 生效了但浏览器看不到改动。

**根因**：Dockerfile 里 `COPY public/ /opt/drill/public/` 把前端文件打进了镜像，容器重建时用的是镜像里的版本——改宿主文件不影响已构建的镜像。

**修复**：把 public 改成 **只读 bind mount**（compose.yml 加一行）：
```yaml
services:
  app:
    # ...
    volumes:
      - drill-app-data:/opt/drill/data
      - drill-app-uploads:/opt/drill/uploads
      - ./bundle/public:/opt/drill/public:ro    # ← 加这一行
```
然后 `docker compose up -d --force-recreate app` 让挂载生效。**以后改 public/ 里的文件，浏览器刷新就能看到**（Ctrl+Shift+R 强制刷新避开缓存）。

> **生产可改回内置**：bind mount 方便迭代，但如果要交付纯镜像（不依赖外部目录），把这行 volumes 删掉、重建镜像即可。

### Q14：前端改动必须同步到 VM（唯一通道说明）

**核心事实**：仓库的 `e:\Trae\Emergency\public\` 和 VM 上的 `/home/drill/drill-deploy/bundle/public/` 是**两个独立目录**。compose 的 bind mount 只挂后者。改仓库文件**不会自动生效**到 VM。

**唯一同步通道**（不要手动 scp/sftp 拷贝）：

```powershell
# 1. 设 VM 登录密码（仅当前会话有效，不要写入文件）
$env:DRILL_PASS = '<VM guest 密码>'

# 2. 默认 dry-run：只 diff，不动文件
powershell -ExecutionPolicy Bypass -File scripts\sync-public-to-vm.ps1

# 3. 确认 diff 无误后 -Apply 真正执行
powershell -ExecutionPolicy Bypass -File scripts\sync-public-to-vm.ps1 -Apply
```

**脚本做了什么**：
1. 逐文件 md5 diff 仓库 `public/` vs VM `bundle/public/`
2. 差异文件先备份到 VM `bundle/public.bak-<HHMMSS>/`
3. SFTP 上传覆盖
4. 自动 strip UTF-8 BOM（浏览器渲染不需要，保留也无害）
5. 回读校验 md5 三方一致（仓库 = VM bundle = 容器内 mount）

**同步后不需要重启容器**——bind mount 是 live 的，Ctrl+Shift+R 强制刷新浏览器即可看到改动。

**SPA fallback 陷阱**：Go 路由的 NoRoute handler 对任何 `.html` 路径做 SPA 兜底——物理文件不存在时 serve `index.html`。所以：
- 删了 `public/foo.html` 后 `GET /foo.html` 返回 **200 + index.html 内容**，**不是 404**
- 这是设计行为（让前端裸相对链接如 `href="projects.html"` 能命中真实页面），删文件前全仓 grep 确认零引用再动手

**常见误操作**（不要做）：
- ❌ 手动 `scp` 拷文件 —— 容易漏文件、不备份
- ❌ `docker compose build` 重建镜像 —— compose 用的是 `drill-server:latest`（之前构建好的），COPY 进镜像的旧文件会覆盖 bind mount 吗？不会——bind mount 优先级更高，重建镜像无意义
- ❌ 重启 `drill-mysql` 或动数据卷 —— 前端同步和数据库无关

---

## 9. 首次部署检查表（VMware NAT 专项）

每一项**必须执行**，不跳过。"宿主"= Windows 机器，"VM"= Linux 虚拟机。

| # | 在哪 | 检查项 | 命令 | 预期输出 |
|---|------|--------|------|----------|
| **1** | VM | MySQL healthy | `docker compose ps mysql` | drill-mysql = healthy |
| **2** | VM | MySQL 有业务数据 | `docker compose exec mysql mysql -uroot -p -D drill_system -e "SELECT COUNT(*) FROM users; SELECT COUNT(*) FROM script_nodes"` | users ≥ 12, script_nodes ≥ 331 |
| **3** | VM | Go 容器 healthy | `curl -s http://127.0.0.1:8090/api/health` | `{"ok":true,...,"db":"up"}` |
| **4** | VM | VM 有 NAT IP | `ip addr show ens33 \| grep inet` | 192.168.x.x（VMware NAT 网段） |
| **5** | 宿主 | vmnetnat.conf 写了转发 | PowerShell: `Select-String -Path 'C:\ProgramData\VMware\vmnetnat.conf' -Pattern 'incomingtcp|8091'` | 看到 `[incomingtcp]` 和 `8091 = VM-IP:8090` |
| **6** | 宿主 | vmware-nat 服务 Running | `Get-Service vmware-nat` | Status=Running |
| **7** | 宿主 | 宿主自通（层 1） | `curl http://127.0.0.1:8091/api/health` | 200 + JSON |
| **8** | 宿主 | 宿主真实 IP 通（层 2） | `curl http://<宿主IP>:8091/api/health` | 200 |
| **9** | 局域网另一台电脑 | 可访问（层 3，最关键） | 浏览器打开 `http://<宿主IP>:8091/api/health` | 200 + JSON |
| **10** | 宿主 | 8091 入站防火墙允许 | `Get-NetFirewallRule -DisplayName *8091*` | 存在且 Enabled=True |
| **11** | 浏览器（宿主或其他电脑） | 登录页 | 打开 `http://<宿主IP>:8091` | 正常显示 + 能登录 + 大屏渲染完整 |
| **12** | VM | 时钟同步 | `timedatectl status` | `System clock synchronized: yes` |
| **13** | VM | 备份可跑 | `docker compose exec mysql mysqldump -uroot -p drill_system \| gzip > /tmp/test-backup.sql.gz` | 无报错、文件非空 |

**全部通过才算阶段一成功**。确认稳定后再决定是否做阶段二（停宿主 Go、转 8090）。

---

## 附录：bundle 内容与 SHA

```
bundle 解压后相对路径清单（打包脚本会自动校验）：

README-FIRST.md                              3 分钟上手
server/deploy-linux/drill-server             ELF64 x86_64, SHA=E82CCBA7...348676
public/                                      前端（无 .env）
migrations/                                  3 个 .sql
deploy/linux/Dockerfile                      多阶段 + 国产镜像支持
deploy/linux/docker-compose.yml              MySQL + app + bootstrap
deploy/linux/drill.env.example               env 模板（占位符）
deploy/linux/drill-server.service            systemd 单元
deploy/linux/install.sh                      幂等安装脚本
deploy/linux/nginx-drill.conf               Nginx 示例
deploy/linux/Caddyfile.example               Caddy 示例
docs/linux-deployment.md                     完整手册
docs/vmware-vm-deployment.md                 本文件
```
