<#
.SYNOPSIS
    Windows Docker Desktop 一键启动本系统（Go 容器 + MySQL 容器 二选一）

.DESCRIPTION
    -Mode HostMySQL → deploy/docker-desktop/docker-compose.yml （Go 连宿主 MySQL）
    -Mode Full      → deploy/docker-desktop/docker-compose.full.yml（Go + MySQL 全容器）

    启动前检查：
      1) Docker Desktop 是否已就绪（docker info 能否返回）
      2) compose 文件和 .env 是否存在
      3) 构建失败时打印原因，不静默退出

.NOTES
    本脚本 **不** 安装 Docker Desktop（那是用户单独操作、可能重启整机）。
    如果你还没装 Docker Desktop 就跑这个脚本，会在第 1 步明确报错退出。
#>

param(
    [ValidateSet("HostMySQL","Full")]
    [string]$Mode = "HostMySQL"
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$desktopDir  = Join-Path $projectRoot "deploy\docker-desktop"

Write-Host "[drill-docker] Mode=$Mode" -ForegroundColor Cyan
Write-Host "[drill-docker] Project root: $projectRoot" -ForegroundColor Cyan

# ---------------------------------------------------------------
# 1. Docker 守护进程就绪检查
# ---------------------------------------------------------------
Write-Host "[drill-docker] checking Docker daemon..." -NoNewline
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host " FAIL (docker 不在 PATH)" -ForegroundColor Red
    Write-Host "  请先安装 Docker Desktop for Windows：" -ForegroundColor Red
    Write-Host "    https://www.docker.com/products/docker-desktop/" -ForegroundColor DarkGray
    Write-Host "  安装后大概率需要重启整机（启用 WSL2/Hyper-V）" -ForegroundColor Yellow
    exit 1
}

# docker info 在 daemon 未 ready 时会 hang 一阵子，给 10s 超时
try {
    $null = docker info 2>&1
    Write-Host " OK" -ForegroundColor Green
} catch {
    Write-Host " FAIL (daemon not reachable)" -ForegroundColor Red
    Write-Host "  启动 Docker Desktop，等托盘图标显示 'Docker Desktop is running' 后再试。" -ForegroundColor Yellow
    exit 1
}

# 确认 compose 子命令可用（Docker Desktop 默认自带 v2）
try {
    $null = docker compose version 2>&1
} catch {
    Write-Host "FAIL (docker compose 不可用)" -ForegroundColor Red
    Write-Host "  你的 Docker Desktop 版本可能过旧，升级一下。" -ForegroundColor Yellow
    exit 1
}

# ---------------------------------------------------------------
# 2. compose 文件 + .env 检查
# ---------------------------------------------------------------
$composeFile = if ($Mode -eq "Full") {
    "docker-compose.full.yml"
} else {
    "docker-compose.yml"
}
$composePath = Join-Path $desktopDir $composeFile
$envExample  = Join-Path $desktopDir ".env.example"
$envFile     = Join-Path $desktopDir ".env"

if (-not (Test-Path $composePath)) {
    Write-Host "[drill-docker] compose 不存在: $composePath" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path $envFile)) {
    Write-Host "[drill-docker] .env 不存在，从 .env.example 拷贝..." -ForegroundColor Yellow
    if (-not (Test-Path $envExample)) {
        Write-Host "[drill-docker] .env.example 也不存在！" -ForegroundColor Red
        exit 1
    }
    Copy-Item $envExample $envFile
    Write-Host "[drill-docker] 已生成 $envFile" -ForegroundColor Green
    Write-Host "[drill-docker] ⚠️  请打开 .env 替换所有 __REPLACE_WITH__ 占位符后再启动" -ForegroundColor Yellow
    Write-Host "    特别注意：" -ForegroundColor DarkGray
    if ($Mode -eq "HostMySQL") {
        Write-Host "    DB_HOST=host.docker.internal （已预填）" -ForegroundColor DarkGray
        Write-Host "    DB_USER / DB_PASSWORD / DB_ROOT_* = 宿主 MySQL 真实账号" -ForegroundColor DarkGray
    } else {
        Write-Host "    DB_HOST=mysql （compose 内部 DNS，不要改）" -ForegroundColor DarkGray
        Write-Host "    MYSQL_ROOT_PASSWORD 必须 = DB_ROOT_PASSWORD" -ForegroundColor DarkGray
        Write-Host "    DB_USER / DB_PASSWORD / MYSQL_DATABASE / MYSQL_USER / MYSQL_PASSWORD 保持一致" -ForegroundColor DarkGray
    }
    Write-Host ""
    Write-Host "[drill-docker] 请编辑完 .env 后再运行本脚本。" -ForegroundColor Yellow
    # 不自动启动，让用户先把 .env 改对
    exit 0
}

Write-Host "[drill-docker] compose: $composeFile" -ForegroundColor DarkGray
Write-Host "[drill-docker] .env   : $envFile" -ForegroundColor DarkGray

# ---------------------------------------------------------------
# 3. 构建 + 启动
# ---------------------------------------------------------------
Set-Location $desktopDir

Write-Host ""
Write-Host "[drill-docker] docker compose build ..." -ForegroundColor Cyan
docker compose -f $composeFile build
if ($LASTEXITCODE -ne 0) {
    Write-Host "[drill-docker] 构建失败（exit=$LASTEXITCODE）" -ForegroundColor Red
    Write-Host "  常见原因：" -ForegroundColor Yellow
    Write-Host "    - 网络拉不到 golang:1.27-alpine（改镜像源或用 --build-arg BUILDER_BASE=... 指定国产 builder）" -ForegroundColor DarkGray
    Write-Host "    - Dockerfile 语法错误（已校验过但实际构建可能有平台差异）" -ForegroundColor DarkGray
    Write-Host "    - go mod 下载超时（重跑一次通常就好）" -ForegroundColor DarkGray
    exit 1
}

Write-Host ""
Write-Host "[drill-docker] docker compose up -d ..." -ForegroundColor Cyan
docker compose -f $composeFile up -d
if ($LASTEXITCODE -ne 0) {
    Write-Host "[drill-docker] up 失败（exit=$LASTEXITCODE）" -ForegroundColor Red
    Write-Host "  看详细日志：docker compose -f $composeFile logs" -ForegroundColor Yellow
    exit 1
}

# ---------------------------------------------------------------
# 4. 健康检查
# ---------------------------------------------------------------
Write-Host ""
Write-Host "[drill-docker] 等待健康检查..." -ForegroundColor Cyan
$healthUrl = "http://127.0.0.1:8090/api/health"
$ok = $false
for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Seconds 1
    try {
        $resp = Invoke-WebRequest -Uri $healthUrl -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) { $ok = $true; break }
    } catch {}
    Write-Host "." -NoNewline
}
if ($ok) {
    Write-Host " PASS" -ForegroundColor Green
    Write-Host ""
    Write-Host "=========================================" -ForegroundColor Green
    Write-Host "  RUNNING: http://127.0.0.1:8090"           -ForegroundColor Green
    Write-Host "  MODE:    $Mode"                            -ForegroundColor Green
    Write-Host "  LOGS:    docker compose -f $composeFile logs -f app" -ForegroundColor DarkGray
    Write-Host "  STOP:    powershell -File scripts\docker-down.ps1 -Mode $Mode" -ForegroundColor DarkGray
    Write-Host "=========================================" -ForegroundColor Green
} else {
    Write-Host " FAIL (20s timeout)" -ForegroundColor Red
    Write-Host "  容器可能还在启动 MySQL 初始化，再等一会儿或直接看日志：" -ForegroundColor Yellow
    Write-Host "  docker compose -f $composeFile logs" -ForegroundColor DarkGray
}
