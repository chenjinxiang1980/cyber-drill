<#
.SYNOPSIS
    **应急入口**：Go 服务被 360 QVM 拦截 / 重启后 Go exe 起不来时，
    用旧的 Node.js 实现顶上（8080 + SQLite）。

.DESCRIPTION
    ⚠️  这是应急兜底，**不是**主入口。
    - 数据走 data\drill.db（SQLite），与 MySQL **分叉**
    - Go 服务恢复后，停止 Node 版，用 Node 备份导入到 MySQL（需手动操作）
    - 8080 端口避免撞 Go 进程的 8090
    - 自动检查 node.exe / node_modules / 8080 占用，缺依赖明确报错退出
    - 不自动 npm install（装包需要联网 + 可能被 360 扫）

.NOTES
    典型使用场景：
      1. 安装 Docker Desktop → 重启整机 → 回来发现 Go exe 被 360 杀了
      2. 先跑这个 Node 脚本顶上，保证业务不中断
      3. 等 Docker Desktop 就绪后，用 scripts\docker-up.ps1 把 Go 搬到容器里
      4. Node 版下线：Stop-Process -Name node -Force
#>

param(
    [int]$Port = 8080,
    [string]$DBFile = "",
    [string]$DrillHost = "0.0.0.0"
)

$ErrorActionPreference = "Stop"

# ---------- 路径 ----------
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

Write-Host "=============================================" -ForegroundColor Yellow
Write-Host "  DRILL NODE FALLBACK (emergency only)" -ForegroundColor Yellow
Write-Host "=============================================" -ForegroundColor Yellow
Write-Host "[fallback] project root: $projectRoot" -ForegroundColor DarkGray
Write-Host "[fallback] port       : $Port" -ForegroundColor DarkGray
Write-Host "[fallback] DB path    : $(if($DBFile){$DBFile}else{'(default: data\drill.db)'})" -ForegroundColor DarkGray
Write-Host ""
Write-Host "[fallback] ⚠️  数据走 SQLite (data\drill.db)" -ForegroundColor Yellow
Write-Host "[fallback] ⚠️  与 MySQL 版本数据会分叉" -ForegroundColor Yellow
Write-Host "[fallback] ⚠️  Go 服务恢复后停用 Node 版" -ForegroundColor Yellow
Write-Host ""

# ---------- 1. node.exe ----------
Write-Host "[fallback] checking node.exe..." -NoNewline
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host " FAIL" -ForegroundColor Red
    Write-Host "  本脚本依赖 Node.js。请先装 Node.js LTS（https://nodejs.org/）。" -ForegroundColor Red
    Write-Host "  或检查 PATH：where.exe node" -ForegroundColor Red
    exit 1
}
$nodeVersion = (node --version)
Write-Host " OK ($nodeVersion)" -ForegroundColor Green

# ---------- 2. node_modules ----------
Write-Host "[fallback] checking node_modules..." -NoNewline
$nodeModules = Join-Path $projectRoot "node_modules"
if (-not (Test-Path $nodeModules)) {
    Write-Host " FAIL" -ForegroundColor Red
    Write-Host "  node_modules 不存在，你需要先在项目根执行：" -ForegroundColor Red
    Write-Host "    npm install --no-audit --no-fund" -ForegroundColor DarkGray
    Write-Host "  然后再跑这个脚本。本脚本**不自动 npm install**（联网 + 装包可能被 360 QVM 扫）。" -ForegroundColor Yellow
    exit 1
}
Write-Host " OK" -ForegroundColor Green

# ---------- 3. 端口 ----------
Write-Host "[fallback] checking port $Port..." -NoNewline
$listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($listener) {
    $cp = $listener | Select-Object -First 1
    $cpName = (Get-Process -Id $cp.OwningProcess -ErrorAction SilentlyContinue).ProcessName
    $cpPath = (Get-CimInstance Win32_Process -Filter "ProcessId = $($cp.OwningProcess)" -ErrorAction SilentlyContinue).ExecutablePath
    Write-Host " FAIL" -ForegroundColor Red
    Write-Host "  Port $Port 已被占用：PID=$($cp.OwningProcess) NAME=$cpName" -ForegroundColor Red
    Write-Host "  路径：$cpPath" -ForegroundColor DarkGray
    Write-Host "  免费端口后重试。常见做法：" -ForegroundColor Yellow
    Write-Host "    Stop-Process -Id $($cp.OwningProcess) -Force" -ForegroundColor DarkGray
    Write-Host "    或换端口：scripts\start-node-fallback.ps1 -Port 8081" -ForegroundColor DarkGray
    exit 1
}
Write-Host " OK (free)" -ForegroundColor Green

# ---------- 4. SQLite 文件（只存在性检查，不读内容）----------
$dbFile = if ($DBFile) { $DBFile } else { (Join-Path $projectRoot "data\drill.db") }
Write-Host "[fallback] checking SQLite file..." -NoNewline
if (Test-Path $dbFile) {
    $size = [math]::Round((Get-Item $dbFile).Length / 1KB, 1)
    Write-Host " OK ($dbFile, ${size} KB)" -ForegroundColor Green
} else {
    Write-Host " MISSING" -ForegroundColor Yellow
    Write-Host "  SQLite 文件不存在，Node 版会自动初始化新库。" -ForegroundColor Yellow
    Write-Host "  如果你期望用现有数据，请把备份的 drill.db 放回来再启动。" -ForegroundColor Yellow
}

# ---------- 5. 启动 ----------
Write-Host ""
Write-Host "[fallback] launching Node server..." -ForegroundColor Cyan

$env:DRILL_PORT = "$Port"
$env:DRILL_HOST = $DrillHost
if ($DBFile) { $env:DRILL_DB = $dbFile }

$nodeProc = Start-Process -FilePath "node" `
                           -ArgumentList @("node\server.js") `
                           -WorkingDirectory $projectRoot `
                           -PassThru

# ---------- 6. 轮询健康检查 ----------
Write-Host "[fallback] health check... " -NoNewline
$healthUrl = "http://127.0.0.1:$Port/api/health"
$ok = $false
for ($i = 0; $i -lt 15; $i++) {
    Start-Sleep -Milliseconds 500
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
    Write-Host "  NODE FALLBACK RUNNING"                  -ForegroundColor Green
    Write-Host "  URL:     http://127.0.0.1:$Port"       -ForegroundColor Green
    Write-Host "  DB:      $dbFile"                       -ForegroundColor DarkGray
    Write-Host "  PID:     $($nodeProc.Id)"               -ForegroundColor Green
    Write-Host "  STOP:    Stop-Process -Id $($nodeProc.Id) -Force" -ForegroundColor Yellow
    Write-Host "=========================================" -ForegroundColor Green
} else {
    Write-Host " FAIL (15s timeout)" -ForegroundColor Red
    Write-Host "  Node 进程可能还在启动，或启动失败。" -ForegroundColor Yellow
    Write-Host "  手动查看：Get-Process -Id $($nodeProc.Id)" -ForegroundColor DarkGray
    Write-Host "  或直接前台跑：node node\server.js" -ForegroundColor DarkGray
    Stop-Process -Id $nodeProc.Id -Force -ErrorAction SilentlyContinue
    exit 1
}
