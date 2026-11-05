<#
.SYNOPSIS
    Linux/amd64 发布件构建 + 签名文件自动重生成

.DESCRIPTION
    - 交叉编译 server -> server\deploy-linux\drill-server
      GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath
    - 同时交叉编译 drill-migrate / drill-seed 到 server\deploy-linux\（供
      Dockerfile.runtime 的 dist_local 一次取齐，避免缺文件导致构建失败）
    - 构建成功后重写 server\deploy-linux\drill-server.sha256.txt（沿用现有 9 行格式）：
        L1 标题 / L2 构建时间（取二进制 mtime）/ L3 构建方式
        L4 源码状态        （保留原文）
        L5 校验            （按实测更新）
        L6 上一线上版本    （保留原文，属部署历史）
        L7 字节数          （按实测更新）
        L8 部署记录        （保留原文，由人工在部署时更新）
        L9 <小写 sha256>  drill-server
    - 编码固定 UTF-8 无 BOM + LF 行尾，与既有文件一致。

.PARAMETER SignOnly
    不编译，仅按现有二进制重写签名文件。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-linux.ps1

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-linux.ps1 -SignOnly
#>
param(
    [switch]$SignOnly
)

$ErrorActionPreference = "Stop"

$scriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$serverDir   = Join-Path $projectRoot "server"
$outDir      = Join-Path $serverDir "deploy-linux"
$elfPath     = Join-Path $outDir "drill-server"
$sigPath     = Join-Path $outDir "drill-server.sha256.txt"

Write-Host "[build-linux] project root: $projectRoot" -ForegroundColor Cyan

if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }

if (-not $SignOnly) {
    Write-Host "[build-linux] go build (GOOS=linux GOARCH=amd64 CGO_ENABLED=0 -trimpath)..." -ForegroundColor Yellow
    Push-Location $serverDir
    try {
        $env:GOOS        = "linux"
        $env:GOARCH      = "amd64"
        $env:CGO_ENABLED = "0"
        go build -trimpath -o $elfPath .
        $rc = $LASTEXITCODE
        if ($rc -eq 0) {
            Write-Host "[build-linux] go build drill-migrate / drill-seed ..." -ForegroundColor Yellow
            go build -trimpath -ldflags "-s -w" -o (Join-Path $outDir "drill-migrate") ./cmd/migrate
            if ($LASTEXITCODE -ne 0) { $rc = $LASTEXITCODE }
        }
        if ($rc -eq 0) {
            go build -trimpath -ldflags "-s -w" -o (Join-Path $outDir "drill-seed") ./cmd/seed
            if ($LASTEXITCODE -ne 0) { $rc = $LASTEXITCODE }
        }
    } finally {
        Pop-Location
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    if ($rc -ne 0) {
        Write-Host "[build-linux] go build FAIL (exit=$rc)" -ForegroundColor Red
        exit 1
    }
}

if (-not (Test-Path $elfPath)) {
    Write-Host "[build-linux] FAIL: drill-server not found at $elfPath" -ForegroundColor Red
    exit 1
}

# ---- 签名文件重生成 ----
$elf   = Get-Item $elfPath
$hash  = (Get-FileHash -Algorithm SHA256 $elfPath).Hash.ToLower()
$size  = $elf.Length
$mtStr = $elf.LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss") + " (" + $elf.LastWriteTime.ToString("zzz") + ")"
$sizeN = $size.ToString("N0", [System.Globalization.CultureInfo]::InvariantCulture)
$hash8 = $hash.Substring(0, 8)

$oldLines = @()
if (Test-Path $sigPath) { $oldLines = @(Get-Content -Path $sigPath -Encoding UTF8) }

function Get-KeptLine($idx, $default) {
    if ($oldLines.Count -gt $idx -and $oldLines[$idx]) { return $oldLines[$idx] }
    return $default
}

# 下标 0 起：L4=3, L6=5, L8=7 —— 这三行属构建历史/部署历史，保留原文
$lines = @(
    "# drill-server Linux/amd64 构建签名",
    "# 构建时间: $mtStr",
    "# 构建方式: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o deploy-linux/drill-server .",
    (Get-KeptLine 3 "# 源码状态: 未记录"),
    "# 校验: 本文件末行哈希 = 二进制实测（Get-FileHash 大写）一致（$sizeN / $hash8…）",
    (Get-KeptLine 5 "# 上一线上版本: 未记录"),
    "# 字节数: $size",
    (Get-KeptLine 7 "# 部署记录: 未记录"),
    "$hash  drill-server"
)

$text = ($lines -join "`n") + "`n"
[System.IO.File]::WriteAllText($sigPath, $text, (New-Object System.Text.UTF8Encoding($false)))

Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  BUILD SIGNED"                            -ForegroundColor Green
Write-Host "  ELF   : $elfPath"                        -ForegroundColor Green
Write-Host "  Size  : $sizeN bytes"                    -ForegroundColor Green
Write-Host "  SHA256: $hash"                           -ForegroundColor Green
Write-Host "  Sig   : $sigPath"                        -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
