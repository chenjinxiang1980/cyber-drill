# =============================================================================
# build-image.ps1 —— Windows 本地构建脚本
#
# 用法：
#   .\scripts\build-image.ps1                     # 默认 amd64
#   .\scripts\build-image.ps1 -Arch arm64         # 跨架构（buildx）
#   .\scripts\build-image.ps1 -Arch amd64 -Version 1.0.0
#
# 注意：
#   - **绝不** push 到任何 registry（红线）。push 命令只作为注释示例。
#   - 需要 Docker Desktop 启用 buildx（默认已启用）。
#   - 构建上下文是仓库根（从 deploy/docker-dist/Dockerfile 向上两级）。
# =============================================================================

param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",
    [string]$Tag  = "drill-server:0.5.1",
    [string]$Version = "0.5.1"
)

$ErrorActionPreference = "Stop"

# --- 构建上下文：仓库根 ---
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$Dockerfile = Join-Path $RepoRoot "deploy\docker-dist\Dockerfile"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host " Drill Server Image Builder (Windows)" -ForegroundColor Cyan
Write-Host "   Architecture : $Arch" -ForegroundColor Gray
Write-Host "   Image        : $Tag" -ForegroundColor Gray
Write-Host "   Version      : $Version" -ForegroundColor Gray
Write-Host "   Context      : $RepoRoot" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan

# --- 时间戳 ---
$buildStart = Get-Date

# --- 构建 ---
# 强制 BuildKit（Docker Desktop 默认开；但旧版本关闭时会导致 COPY --chmod 不生效）
$env:DOCKER_BUILDKIT = "1"

$buildArgs = @(
    "buildx", "build",
    "--platform", "linux/$Arch",
    "-f", $Dockerfile,
    "-t", $Tag,
    "--build-arg", "VERSION=$Version",
    "--build-arg", "BUILD_TIME=$($buildStart.ToString('yyyy-MM-dd HH:mm:ss'))",
    "--load",   # 加载到本地 Docker daemon（buildx 默认只导出，--load 把结果入本地）
    "."
)

Write-Host "`n[1/3] docker $($buildArgs -join ' ')`n" -ForegroundColor Yellow

& docker @buildArgs
if ($LASTEXITCODE -ne 0) {
    Write-Host "`n❌ 构建失败 (exit=$LASTEXITCODE)" -ForegroundColor Red
    exit $LASTEXITCODE
}

$buildEnd = Get-Date
$elapsed = ($buildEnd - $buildStart).ToString("mm\:ss")
Write-Host "`n✅ 构建完成，耗时 $elapsed" -ForegroundColor Green

# --- 镜像信息 ---
Write-Host "`n[2/3] 镜像信息" -ForegroundColor Yellow
& docker images --format "table {{.Repository}}`t{{.Tag}}`t{{.ID}}`t{{.Size}}" | Select-String -Pattern "drill-server"

$imageId = (& docker images -q --filter "reference=$Tag") | Select-Object -First 1
if ($imageId) {
    Write-Host "`nImage ID   : $imageId"
    $digest = (& docker inspect --format "{{index .RepoDigests 0}}" $imageId 2>$null)
    if ($digest) { Write-Host "Image Digest: $digest" }
}

# --- 不 push（红线）---
Write-Host "`n[3/3] 本地冒烟测试（--rm 临时容器）" -ForegroundColor Yellow
Write-Host "  注意：冒烟测试只启动容器确认镜像能跑。" -ForegroundColor Gray
Write-Host "  要真正起完整服务请用 docker compose up -d。`n" -ForegroundColor Gray

# 冒烟：用临时容器启 drill-migrate 验证二进制存在，立即退出
$smokeId = (& docker run --rm --entrypoint /opt/drill/bin/drill-migrate $Tag --help 2>&1 | Out-String)
# 不关心 exit code（--help 可能返回 1），只关心能启动
Write-Host "  drill-migrate 输出（前 5 行）:" -ForegroundColor Gray
$smokeId -split "`n" | Select-Object -First 5 | ForEach-Object { Write-Host "    $_" }

Write-Host "`n============================================================" -ForegroundColor Green
Write-Host "  构建完成！镜像：$Tag" -ForegroundColor Green
Write-Host "  架构   : linux/$Arch" -ForegroundColor Green
Write-Host "  耗时   : $elapsed" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green

# =============================================================================
# 如何 push（**需要用户自行 docker login 并确认授权后执行**）：
#
#   # 1. 登录 registry
#   docker login ghcr.io                    # GitHub Container Registry
#   docker login registry.cn-hangzhou.aliyuncs.com   # 阿里云 ACR
#   docker login swr.cn-north-4.myhuaweicloud.com    # 华为云 SWR
#
#   # 2. tag
#   docker tag $Tag ghcr.io/your-org/drill-server:$Version
#
#   # 3. push
#   docker push ghcr.io/your-org/drill-server:$Version
#
#   # 4. 多架构 manifest（buildx 推荐方式）
#   docker buildx build --platform linux/amd64,linux/arm64 `
#       -t ghcr.io/your-org/drill-server:$Version `
#       -f deploy/docker-dist/Dockerfile --push .
# =============================================================================
