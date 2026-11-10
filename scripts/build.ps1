<#
.SYNOPSIS
    Drill System Go 后端统一构建脚本（Windows）
.DESCRIPTION
    - 把 Go 构建缓存/临时目录收进项目内，避免 %LOCALAPPDATA%\go-build / C:\Temp 被 360 QVM 高权重扫描
    - 用 windres 生成 VERSIONINFO + ICON + MANIFEST，.syso 编译后立刻删除（中间产物易被杀软误判）
    - 固定输出 server\deploy\drill-server.exe，打印 SHA-256
.PARAMETER Version
    版本号，例如 "0.1.0"，默认 0.1.0
.PARAMETER Force
    忽略源文件时间戳，强制重编译
.EXAMPLE
    pwsh scripts/build.ps1
    pwsh scripts/build.ps1 -Version 0.2.0
    pwsh scripts/build.ps1 -Version 0.2.0 -Force
#>
param(
    [string]$Version = "0.1.0",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

# ---- Paths ----
$scriptDir    = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot  = Split-Path -Parent $scriptDir
$serverDir    = Join-Path $projectRoot "server"
$winresDir    = Join-Path $serverDir "winres"
$buildDir     = Join-Path $serverDir "deploy"
$exePath      = Join-Path $buildDir "drill-server.exe"
$sysoTempPath = Join-Path $serverDir "drill.syso"        # 给 go build 吃进去，用完即删

$gocache      = Join-Path $serverDir ".gocache"
$gotmpdir     = Join-Path $serverDir ".gotmp"

# ---- 工具检查 ----
Write-Host "[build] project root: $projectRoot" -ForegroundColor Cyan

$windres = Get-Command windres -ErrorAction SilentlyContinue
if (-not $windres) {
    Write-Host "[build] windres not found in PATH." -ForegroundColor Red
    Write-Host "        Expected: scoop install mingw / or use go-winres" -ForegroundColor Red
    Write-Host "        If using MinGW-w64 standalone, add its bin\ to PATH" -ForegroundColor Red
    exit 1
}
Write-Host "[build] windres: $($windres.Source)" -ForegroundColor Green

# ---- 缓存目录 ----
foreach ($d in @($gocache, $gotmpdir, $buildDir)) {
    if (-not (Test-Path $d)) { New-Item -ItemType Directory -Path $d -Force | Out-Null }
}
$env:GOCACHE   = $gocache
$env:GOTMPDIR  = $gotmpdir
Write-Host "[build] GOCACHE  = $gocache"   -ForegroundColor DarkGray
Write-Host "[build] GOTMPDIR = $gotmpdir"  -ForegroundColor DarkGray

# ---- 资源编译（.syso） ----
$rcFile   = Join-Path $winresDir "drill.rc"
$icoFile  = Join-Path $winresDir "drill.ico"
$manFile  = Join-Path $winresDir "app.manifest"

if (-not (Test-Path $rcFile)) {
    Write-Host "[build] drill.rc missing at $rcFile" -ForegroundColor Red; exit 1
}

Write-Host "[build] compiling resources (windres)..." -ForegroundColor Yellow
& $windres.Source --target=pe-x86-64 -i $rcFile -o $sysoTempPath 2>&1 | ForEach-Object { Write-Host "  windres: $_" }
if ($LASTEXITCODE -ne 0) { Write-Host "[build] windres FAIL" -ForegroundColor Red; exit 1 }
if (-not (Test-Path $sysoTempPath)) { Write-Host "[build] .syso not produced" -ForegroundColor Red; exit 1 }
Write-Host "[build] .syso ready: $((Get-Item $sysoTempPath).Length) bytes" -ForegroundColor Green

# ---- Go 编译 ----
Set-Location $serverDir

$srcLatest = (Get-ChildItem -Path $serverDir -Recurse -Include "*.go","*.syso" -ErrorAction SilentlyContinue `
               | Measure-Object LastWriteTime -Maximum).Maximum
$needBuild = $Force -or -not (Test-Path $exePath) -or
             ($srcLatest -and (Get-Item $exePath).LastWriteTime -lt $srcLatest)

if (-not $needBuild) {
    Write-Host "[build] sources unchanged, skipping go build (use -Force to override)" -ForegroundColor Yellow
    # 仍然删 .syso —— 它只是中间产物，留在磁盘上也会被 360 扫
    Remove-Item $sysoTempPath -Force -ErrorAction SilentlyContinue
    Write-Host "[build] result: REUSE" -ForegroundColor Green
    exit 0
}

Write-Host "[build] go build -trimpath -o $exePath ." -ForegroundColor Yellow
$sw = [System.Diagnostics.Stopwatch]::StartNew()
go build -trimpath -o $exePath . 2>&1 | ForEach-Object { Write-Host "  go: $_" }
$rc = $LASTEXITCODE
$sw.Stop()

# ---- 清理 .syso（关键：它只是编译输入，exe 生成后立刻删，避免被 360 误判）----
Remove-Item $sysoTempPath -Force -ErrorAction SilentlyContinue
Write-Host "[build] temp .syso removed (avoids AV heuristic trigger)" -ForegroundColor DarkGray

if ($rc -ne 0) { Write-Host "[build] go build FAIL (exit=$rc)" -ForegroundColor Red; exit 1 }

# ---- 结果 ----
$exe = Get-Item $exePath
$sha256 = (Get-FileHash $exePath -Algorithm SHA256).Hash.ToLower()
$vi = $exe.VersionInfo

Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  BUILD SUCCESS"                             -ForegroundColor Green
Write-Host "  Path:     $exePath"                        -ForegroundColor Green
Write-Host "  Size:     $([math]::Round($exe.Length/1MB,2)) MB" -ForegroundColor Green
Write-Host "  Time:     $([math]::Round($sw.Elapsed.TotalSeconds,1)) s" -ForegroundColor Green
Write-Host "  SHA-256:  $sha256"                         -ForegroundColor Green
Write-Host "  Company:  $($vi.CompanyName)"              -ForegroundColor Green
Write-Host "  Product:  $($vi.ProductName)"              -ForegroundColor Green
Write-Host "  FileDesc: $($vi.FileDescription)"          -ForegroundColor Green
Write-Host "  FileVer:  $($vi.FileVersion)"              -ForegroundColor Green
Write-Host "  ProdVer:  $($vi.ProductVersion)"           -ForegroundColor Green
Write-Host "  Copy:     $($vi.LegalCopyright)"          -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Write-Host ""
Write-Host "SHA-256 is needed for: 360 whitelist / false-positive appeal." -ForegroundColor DarkGray
Write-Host "Keep this output, it changes on every rebuild."                -ForegroundColor DarkGray
