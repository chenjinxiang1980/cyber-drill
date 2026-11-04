<#
.SYNOPSIS
    Windows Docker Desktop 一键停止本系统容器

.DESCRIPTION
    -Mode HostMySQL → docker compose down（保留数据卷）
    -Mode Full      → docker compose down（保留 MySQL/app 数据卷）
    -Mode FullWipe  → docker compose down -v（删除所有数据卷，重置）

.NOTES
    本脚本不卸载 Docker Desktop、不删除镜像。
#>

param(
    [ValidateSet("HostMySQL","Full","FullWipe")]
    [string]$Mode = "HostMySQL"
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$desktopDir  = Join-Path $projectRoot "deploy\docker-desktop"

$composeFile = if ($Mode -eq "HostMySQL") {
    "docker-compose.yml"
} else {
    "docker-compose.full.yml"
}
$composePath = Join-Path $desktopDir $composeFile
Set-Location $desktopDir

Write-Host "[drill-docker] stopping Mode=$Mode (compose=$composeFile)" -ForegroundColor Cyan

$args = @('-f', $composeFile, 'down')
if ($Mode -eq "FullWipe") {
    $args += '-v'
    Write-Host "[drill-docker] FullWipe: 会删除 MySQL 数据卷！" -ForegroundColor Yellow
}

docker compose @args
if ($LASTEXITCODE -ne 0) {
    Write-Host "[drill-docker] down 失败（exit=$LASTEXITCODE），可能容器已停" -ForegroundColor Yellow
} else {
    Write-Host "[drill-docker] done" -ForegroundColor Green
}
