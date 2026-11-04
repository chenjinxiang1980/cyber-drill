# =============================================================================
# Windows + Docker Desktop —— 本系统本地运行
#
# 前置（用户已授权安装 Docker Desktop，**安装过程可能需要重启**）：
#   1. 启用 WSL2 或 Hyper-V（Windows 功能） → **大概率需要重启**
#   2. 安装 Docker Desktop for Windows
#   3. 启动 Docker Desktop，状态栏显示 "Docker Desktop is running"
#
# ---------- 两套 compose 二选一 ----------
#
# A) docker-compose.yml      —— 容器内 Go → **宿主 MySQL**（host.docker.internal）
#    适合：开发机上已经在跑 MySQL（如 MySQL Installer / XAMPP / 单独 mysqld）
#    优点：不重复起 MySQL，数据沿用开发机的 drill_system
#    注意：宿主 MySQL 必须监听 0.0.0.0（不能只绑 127.0.0.1），
#          且已创建业务账号 drill_app 并授权（或先跑 bootstrap）
#
# B) docker-compose.full.yml —— 容器内 Go + 容器内 MySQL（一键起）
#    适合：宿主机没装 MySQL，或不想污染开发机环境
#    优点：完全自包含，bootstrap 建库建账号全自动化
#    注意：MySQL 数据在命名卷 drill-desktop-mysql 里，重启不丢，删 -v 才丢
#
# ---------- 启动步骤 ----------
#
#   # 方式 A：宿主 MySQL 版
#   cp .env.example .env
#   # 把 DB_HOST 改成 host.docker.internal（关键！）
#   # DB_USER / DB_PASSWORD / DB_ROOT_* 填宿主 MySQL 的账号
#   docker compose -f docker-compose.yml up -d --build
#
#   # 方式 B：全容器版
#   cp .env.example .env
#   # DB_HOST=mysql（compose 内部 DNS，不要改）
#   # MYSQL_ROOT_PASSWORD 必须与 DB_ROOT_PASSWORD 一致
#   docker compose -f docker-compose.full.yml up -d --build
#
#   # 一键脚本（在 PowerShell 里）
#   powershell -File scripts\docker-up.ps1 -Mode HostMySQL
#   powershell -File scripts\docker-up.ps1 -Mode Full
#
# ---------- 限制（必须知道）----------
#
# 1) 宿主 MySQL 如果只绑 127.0.0.1，host.docker.internal 通常能转发到
#    宿主 loopback，但 **必须实测确认**：
#       docker run --rm alpine wget -qO- http://host.docker.internal:3306
#    不通则要么改 MySQL bind-address=0.0.0.0，要么改 compose 用容器 MySQL
#
# 2) Windows Docker Desktop 默认启用 amd64 + arm64 QEMU 模拟，
#    可以跑国产发行版（openEuler 等）但速度较慢。
#    龙芯 LoongArch 镜像 **无法在 Windows Docker Desktop 上运行**（无模拟层）。
#
# 3) 挂载的 public/ / data/ 用 volume mount，Windows Docker Desktop
#    首次挂载会弹窗请求文件系统权限（点击 "Share it"）。
#
# 4) Windows Docker Desktop 对容器里的 /etc/localtime 处理和 Linux 有差异，
#    已在 Dockerfile 里硬写 TZ=Asia/Shanghai，避免容器时区错 8 小时。
#
# 5) 本 compose **不**处理 360 QVM。如果宿主 Go exe 被 360 杀，
#    把 Go 逻辑搬到容器里运行（本 compose 正是这么做的）。
#
# ---------- 与 Linux 部署包的关系 ----------
#
# deploy/linux/ 是物理机 systemd 包；deploy/docker-desktop/ 是本地开发包。
# 两者独立、互补、不冲突。Dockerfile 共用 deploy/linux/Dockerfile（已做
# ARG BASE_IMAGE 可替换，默认 Alpine，openEuler/Anolis 国产镜像只需 --build-arg）。
