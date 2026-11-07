# =============================================================================
# Cyber-Drill（应急演练指挥系统）0.8.3 —— Windows 一键安装脚本
#
# 功能：
#   1) 检测 Docker Desktop（区分"没装"与"装了没启动"），未装则给出 winget /
#      官网两条引导路径（含 WSL2 前置检查），不会静默安装
#   2) 选择源码来源：GitHub / Gitee / 已有本地目录
#   3) 选择镜像来源：GHCR（公开）/ SWR 华为云（公开或 AK 登录）/ 本地镜像
#   4) 生成 drill.env（缺失密钥自动随机生成；已有文件绝不覆盖）
#   5) docker compose up -d，轮询 /api/health 直到 200 或超时
#
# 用法（在 PowerShell 中）：
#   .\install-windows.ps1                  # 交互式
#   .\install-windows.ps1 --yes            # 全默认、免确认
#   .\install-windows.ps1 --dry-run        # 只打印命令，不做任何变更
#
# 常用参数：
#   --source github|gitee|local   源码来源
#   --registry ghcr|swr|local     镜像来源
#   --dir <路径>                  源码目录（默认 $HOME\cyber-drill）
#   --port <端口>                 应用宿主端口（默认 8090）
#   --mysql-port <端口>           MySQL 宿主端口（默认 3306）
#   --project <名称>              compose 项目名（默认 drill）
#   --env-file <文件名>           环境文件名（默认 drill.env）
#   --image-tar <文件>            本地镜像 tar
#
# 双击 install-windows.cmd 可直接运行（内部绕过 ExecutionPolicy）。
# 安全约定：脚本零硬编码凭据；SWR 口令只经 docker login --password-stdin。
# =============================================================================

# 原生命令（docker/wsl）会把进度写到 stderr，Stop 会误判为异常；
# 这里用 Continue，并显式检查 $LASTEXITCODE。
$ErrorActionPreference = 'Continue'

$Version      = '0.8.3'
# 安装器随 main 分支发布（v0.8.3 标签与安装器同源），
# 源码版本固定为 0.8.3；可用环境变量 DRILL_REF 覆盖分支/标签。
$Ref          = if ($env:DRILL_REF) { $env:DRILL_REF } else { 'main' }
$GithubRepo   = 'https://github.com/chenjinxiang1980/cyber-drill.git'
$GiteeRepo    = 'https://gitee.com/habeas/cyber-drill.git'
$GithubArchive = "https://github.com/chenjinxiang1980/cyber-drill/archive/refs/heads/$Ref.tar.gz"
$GiteeArchive  = "https://gitee.com/habeas/cyber-drill/repository/archive/$Ref.tar.gz"
$GhcrImage    = "ghcr.io/chenjinxiang1980/cyber-drill:$Version"
$SwrHost      = 'swr.cn-east-3.myhuaweicloud.com'
$SwrOrg       = 'chenjinxiang'

$Opts = @{
    DryRun    = $false
    Yes       = $false
    Source    = ''
    Registry  = ''
    Dir       = (Join-Path $HOME 'cyber-drill')
    Port      = 8090
    MysqlPort = 3306
    Project   = 'drill'
    EnvFile   = 'drill.env'
    ImageTar  = ''
    Bundle    = ''
    Update    = 'check'
    ForceReset = $false
}

function Write-Info([string]$msg) { Write-Host "[信息] $msg" }
function Write-Warn([string]$msg) { Write-Host "[提示] $msg" -ForegroundColor Yellow }
function Write-Err([string]$msg)  { Write-Host "[错误] $msg" -ForegroundColor Red }
function Write-Step([string]$msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

function Show-Usage {
    Write-Host @'
Cyber-Drill 0.8.3 Windows 一键安装

用法：
  powershell -NoProfile -ExecutionPolicy Bypass -File install-windows.ps1 [参数]

参数：
  --dry-run              只打印将执行的命令，不安装/不拉取/不启动
  --yes                  跳过交互确认（自动化）
  --source  github|gitee|local
  --registry ghcr|swr|local
  --dir <路径>          源码目录（默认 $HOME\cyber-drill）
  --port <端口>         应用端口（默认 8090）
  --mysql-port <端口>   MySQL 端口（默认 3306）
  --project <名称>      compose 项目名（默认 drill）
  --env-file <文件名>   环境文件名（默认 drill.env）
  --image-tar <文件>    本地镜像 tar（仅长参数）
  --bundle <tar.gz>     离线源码包（仅长参数，按内置 SHA256 校验）
  --update / -Update    检查远端 ref，一致跳过，不一致增量更新（默认）
  --no-update / -NoUpdate  只用本地源码，不联网检查
  --force-reset / -ForceReset  非 git / 有未提交改动时仍强制更新（先备份 drill.env）
'@
}

# ---------- 参数解析（多数参数同时支持 --xxx 与 -Xxx；--image-tar/--bundle 仅长参数） ----------
for ($i = 0; $i -lt $args.Count; $i++) {
    $key = [string]$args[$i]
    $val = ''
    switch ($key) {
        '--dry-run'  { $Opts.DryRun = $true }
        '-DryRun'    { $Opts.DryRun = $true }
        '--yes'      { $Opts.Yes = $true }
        '-Yes'       { $Opts.Yes = $true }
        '--source'   { $i++; $Opts.Source = [string]$args[$i] }
        '-Source'    { $i++; $Opts.Source = [string]$args[$i] }
        '--registry' { $i++; $Opts.Registry = [string]$args[$i] }
        '-Registry'  { $i++; $Opts.Registry = [string]$args[$i] }
        '--dir'      { $i++; $Opts.Dir = [string]$args[$i] }
        '-Dir'       { $i++; $Opts.Dir = [string]$args[$i] }
        '--port'     { $i++; $Opts.Port = [int]$args[$i] }
        '-Port'      { $i++; $Opts.Port = [int]$args[$i] }
        '--mysql-port' { $i++; $Opts.MysqlPort = [int]$args[$i] }
        '-MysqlPort'   { $i++; $Opts.MysqlPort = [int]$args[$i] }
        '--project'  { $i++; $Opts.Project = [string]$args[$i] }
        '-Project'   { $i++; $Opts.Project = [string]$args[$i] }
        '--env-file' { $i++; $Opts.EnvFile = [string]$args[$i] }
        '--image-tar' { $i++; $Opts.ImageTar = [string]$args[$i] }
        '--bundle'   { $i++; $Opts.Bundle = [string]$args[$i] }
        '--update'   { $Opts.Update = 'check' }
        '-Update'    { $Opts.Update = 'check' }
        '--no-update' { $Opts.Update = 'never' }
        '-NoUpdate'  { $Opts.Update = 'never' }
        '--force-reset' { $Opts.Update = 'check'; $Opts.ForceReset = $true }
        '-ForceReset'   { $Opts.Update = 'check'; $Opts.ForceReset = $true }
        '-h'         { Show-Usage; exit 0 }
        '--help'     { Show-Usage; exit 0 }
        default      { Write-Err "未知参数：$key"; Show-Usage; exit 1 }
    }
}

if (-not $Opts.Source -and $env:DRILL_SOURCE)     { $Opts.Source = $env:DRILL_SOURCE }
if (-not $Opts.Registry -and $env:DRILL_REGISTRY) { $Opts.Registry = $env:DRILL_REGISTRY }
if ($env:DRILL_DIR) { $Opts.Dir = $env:DRILL_DIR }
if ($env:DRILL_PORT) { $Opts.Port = [int]$env:DRILL_PORT }

function Confirm-Action([string]$prompt) {
    if ($Opts.Yes) { Write-Host "$prompt -> 自动确认（--yes）"; return $true }
    $ans = Read-Host "$prompt [y/N]"
    return ($ans -match '^(y|yes|Y|YES)$')
}

function Choose-Item([string]$title, [string]$o1, [string]$o2, [string]$o3, [string]$def) {
    Write-Host ""
    Write-Host $title
    Write-Host "  1) $o1"
    Write-Host "  2) $o2"
    Write-Host "  3) $o3"
    $ans = Read-Host "请选择 [$def]"
    if ([string]::IsNullOrWhiteSpace($ans)) { return $def }
    return $ans.Trim()
}

function New-RandomHex([int]$bytes) {
    $buf = New-Object byte[] $bytes
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($buf) } finally { $rng.Dispose() }
    return (($buf | ForEach-Object { $_.ToString('x2') }) -join '')
}

function Test-DockerCli {
    return [bool](Get-Command docker -ErrorAction SilentlyContinue)
}

function Test-DockerDaemon {
    if (-not (Test-DockerCli)) { return $false }
    & docker info 1>$null 2>$null
    return ($LASTEXITCODE -eq 0)
}

# ---------- 1. Docker 检测 ----------
function Show-WslCheck {
    Write-Host ""
    Write-Host "WSL2 前置检查（Docker Desktop 需要）："
    $wsl = Get-Command wsl -ErrorAction SilentlyContinue
    if (-not $wsl) {
        Write-Warn "未检测到 wsl 命令；请先在“启用或关闭 Windows 功能”里勾选 WSL 与虚拟机平台。"
        return
    }
    & wsl --status 1>$null 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  wsl --status：正常"
    } else {
        Write-Host "  wsl --status：未就绪（可执行 wsl --install，然后重启）"
    }
    & wsl -l -v 1>$null 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  wsl -l -v：已有发行版（请确认版本列为 2）"
    } else {
        Write-Host "  wsl -l -v：尚无已安装的发行版（可执行 wsl --install -d Ubuntu）"
    }
}

function Install-DockerGuidance {
    Write-Err "未检测到 Docker Desktop（docker 命令不存在）。"
    Write-Host ""
    Write-Host "请任选一条路径安装："
    Write-Host "  路径 A（推荐，winget）："
    Write-Host "    winget install -e --id Docker.DockerDesktop"
    Write-Host "  路径 B（官网下载）："
    Write-Host "    https://www.docker.com/products/docker-desktop/"
    Show-WslCheck
    Write-Host ""
    Write-Host "安装完成后："
    Write-Host "  1) 重启电脑（首次安装 WSL2 后必须重启）"
    Write-Host "  2) 重新打开终端，确认 docker info 能正常输出"
    Write-Host "  3) 重新运行本安装器"

    if ($Opts.DryRun) {
        Write-Host ""
        Write-Host "[dry-run] 仅展示引导，不执行 winget 安装。"
        return
    }
    if ($Opts.Yes) {
        Write-Warn "--yes 不自动安装 Docker Desktop，请按上述路径手工安装。"
        return
    }
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        if (Confirm-Action "是否现在用 winget 安装 Docker Desktop？") {
            & winget install -e --id Docker.DockerDesktop
            Write-Warn "安装命令已执行。请重启电脑并重新打开终端后，再运行本安装器。"
            return
        }
    }
    Write-Warn "已跳过自动安装。安装 Docker Desktop 后重新运行本安装器。"
}

function Ensure-Docker {
    Write-Step "1/5 检测 Docker 环境"
    if (-not (Test-DockerCli)) {
        Install-DockerGuidance
        if (-not $Opts.DryRun) { exit 1 }
        return
    }
    if (-not (Test-DockerDaemon)) {
        Write-Warn "检测到 docker 命令，但 docker info 失败：Docker Desktop 已安装但未启动（或 WSL2 后端未就绪）。"
        Write-Host "  请启动 Docker Desktop，等待状态变为 Running 后重试。"
        if ($Opts.DryRun) {
            Write-Host "  [dry-run] 仅提示启动 Docker Desktop，不执行操作。"
            return
        }
        exit 1
    }
    Write-Info "Docker 已安装且守护进程可用。"

    & docker compose version 1>$null 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Err "未检测到 Docker Compose v2（docker compose version 失败）。Docker Desktop 通常自带，请升级后重试。"
        if (-not $Opts.DryRun) { exit 1 }
        return
    }
    Write-Info ("Docker Compose 可用：" + ((& docker compose version) | Select-Object -First 1))
}

# ---------- 2. 源码来源 ----------
function Get-Source {
    Write-Step "2/5 选择源码来源"
    if (-not $Opts.Source) {
        if ($Opts.Yes) {
            $Opts.Source = 'github'
        } else {
            $c = Choose-Item "请选择源码来源：" "GitHub（境外）" "Gitee（国内）" "已有本地目录" "1"
            switch ($c) {
                '1' { $Opts.Source = 'github' }
                '2' { $Opts.Source = 'gitee' }
                '3' { $Opts.Source = 'local' }
                default { Write-Err "无效选择：$c"; exit 1 }
            }
        }
    }

    $composeFile = Join-Path $Opts.Dir 'deploy\docker-dist\docker-compose.yml'
    if ($Opts.Source -eq 'local') {
        if ($Opts.DryRun) {
            Write-Host "  [dry-run] 检查本地目录 $($Opts.Dir)"
            return
        }
        if (-not (Test-Path $composeFile)) {
            Write-Err "本地目录中没有 deploy\docker-dist\docker-compose.yml：$($Opts.Dir)"
            exit 1
        }
        Write-Info "沿用本地源码目录：$($Opts.Dir)"
        return
    }

    if (Test-Path $composeFile) {
        if ($Opts.DryRun) {
            if ($Opts.Update -eq 'never') {
                Write-Host "  [dry-run] --no-update：跳过 git fetch，直接使用 $($Opts.Dir)"
                return
            }
            if (-not (Test-Path (Join-Path $Opts.Dir '.git'))) {
                Write-Host "  [dry-run] 检测到非 git 目录：默认不覆盖；加 --force-reset 将“备份目录 + 重新下载 + 恢复 drill.env”"
                return
            }
            Write-Host "  [dry-run] git -C $($Opts.Dir) fetch origin $Ref"
            $localShort = (& git -C $Opts.Dir rev-parse --short HEAD 2>$null | Out-String).Trim()
            $localFull = (& git -C $Opts.Dir rev-parse HEAD 2>$null | Out-String).Trim()
            $remotePeek = (& git -C $Opts.Dir ls-remote origin $Ref 2>$null | Out-String).Trim()
            if ($remotePeek) { $remotePeek = ($remotePeek -split '\s+')[0] }
            if ($localShort -and $remotePeek) {
                if ($localFull -eq $remotePeek) {
                    Write-Host "  [dry-run] 判定：源码已是最新（$localShort），将跳过下载"
                } else {
                    $rs = $remotePeek.Substring(0, [Math]::Min(8, $remotePeek.Length))
                    Write-Host "  [dry-run] 判定：检测到更新（$localShort -> $rs），将备份 drill.env 后 git reset --hard origin/$Ref"
                }
            } else {
                Write-Host "  [dry-run] 判定：git fetch 后比较 HEAD 与 origin/$Ref；一致跳过，不一致备份 drill.env 后增量更新"
            }
            if (Test-SourceDirty) { Write-Host "  [dry-run] 检测到未提交改动：默认不覆盖，需 --force-reset 才强更" }
            return
        }
        Write-Info "检测到已有源码目录：$($Opts.Dir)"
        Update-ExistingSource
        return
    }
    if ($Opts.DryRun) {
        Write-Host "  [dry-run] git clone --depth 1 --branch $Ref <$($Opts.Source)> $($Opts.Dir)（失败则下载归档）"
        return
    }

    if (-not (Invoke-FreshSource)) {
        Write-Err "源码下载/解压失败"
        exit 1
    }
}

# ---------- 2.1 源码更新辅助 ----------
function Get-EnvRelPath { return (Join-Path 'deploy\docker-dist' $Opts.EnvFile) }

function Backup-EnvFile {
    $envPath = Join-Path $Opts.Dir (Get-EnvRelPath)
    if (-not (Test-Path $envPath)) { return }
    $ts = Get-Date -Format 'yyyyMMdd-HHmmss'
    $bak = "$envPath.bak-$ts"
    Copy-Item -LiteralPath $envPath -Destination $bak -Force
    Write-Info "已备份环境文件：$bak"
}

function Test-SourceDirty {
    if (-not (Test-Path (Join-Path $Opts.Dir '.git'))) { return $false }
    # 只看已跟踪文件；drill.env 等安装期生成文件是 untracked，不算脏
    $out = & git -C $Opts.Dir status --porcelain --untracked-files=no 2>$null
    return ((($out | Out-String).Trim()).Length -gt 0)
}

function Invoke-FreshSource {
    $composeFile = Join-Path $Opts.Dir 'deploy\docker-dist\docker-compose.yml'
    $url = if ($Opts.Source -eq 'gitee') { $GiteeRepo } else { $GithubRepo }
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
        Write-Info "浅克隆分支 $Ref（版本 $Version）：$url"
        & git clone --depth 1 --branch $Ref $url $Opts.Dir
        if ($LASTEXITCODE -eq 0 -and (Test-Path $composeFile)) { return $true }
        Write-Warn "git clone 失败，回退为归档下载。"
    } else {
        Write-Warn "未找到 git，回退为归档下载。"
    }

    $archiveUrl = if ($Opts.Source -eq 'gitee') { $GiteeArchive } else { $GithubArchive }
    $tmp = Join-Path $env:TEMP ("cyber-drill-" + $Version + ".tar.gz")
    Write-Info "下载归档：$archiveUrl"
    try {
        Invoke-WebRequest -Uri $archiveUrl -OutFile $tmp -UseBasicParsing -ErrorAction Stop
    } catch {
        Write-Warn "归档下载失败：$($_.Exception.Message)"
        return $false
    }
    $tar = Get-Command tar -ErrorAction SilentlyContinue
    if (-not $tar) { Write-Err "系统缺少 tar，无法解压归档。"; return $false }
    $tmpDir = Join-Path $env:TEMP ("cyber-drill-extract-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmpDir | Out-Null
    & tar -xzf $tmp -C $tmpDir
    if ($LASTEXITCODE -ne 0) { Write-Err "解压失败：$tmp"; return $false }
    $first = Get-ChildItem $tmpDir | Select-Object -First 1
    Move-Item $first.FullName $Opts.Dir
    Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    return (Test-Path $composeFile)
}

function Replace-NonGitSource {
    $ts = Get-Date -Format 'yyyyMMdd-HHmmss'
    $bk = "$($Opts.Dir).bak-$ts"
    Write-Info "备份原目录：$bk"
    Move-Item -LiteralPath $Opts.Dir -Destination $bk -Force
    if (-not (Invoke-FreshSource)) {
        Write-Err "重新下载失败，恢复原目录"
        Remove-Item -LiteralPath $Opts.Dir -Recurse -Force -ErrorAction SilentlyContinue
        Move-Item -LiteralPath $bk -Destination $Opts.Dir -Force
        return $false
    }
    $rel = Get-EnvRelPath
    $oldEnv = Join-Path $bk $rel
    if (Test-Path $oldEnv) {
        $dest = Join-Path $Opts.Dir $rel
        New-Item -ItemType Directory -Force -Path (Split-Path $dest -Parent) | Out-Null
        Copy-Item -LiteralPath $oldEnv -Destination $dest -Force
        Get-ChildItem "$oldEnv.bak-*" -ErrorAction SilentlyContinue | ForEach-Object {
            Copy-Item -LiteralPath $_.FullName -Destination (Split-Path $dest -Parent) -Force
        }
        Write-Info "已恢复原 drill.env（并保留备份文件）"
    }
    Remove-Item -LiteralPath $bk -Recurse -Force -ErrorAction SilentlyContinue
    Write-Info "源码已强制重新下载：$($Opts.Dir)"
    return $true
}

function Update-ExistingSource {
    if ($Opts.Update -eq 'never') {
        Write-Info "--no-update：跳过联网检查，使用本地源码 $($Opts.Dir)"
        return
    }
    if (-not (Test-Path (Join-Path $Opts.Dir '.git'))) {
        Write-Warn "目录不是 git 仓库：$($Opts.Dir)（默认不覆盖）"
        if ($Opts.ForceReset) { Replace-NonGitSource | Out-Null; return }
        if ($Opts.Yes) {
            Write-Info "非交互模式：保留本地目录，不覆盖（如需强更请加 --force-reset）"
            return
        }
        if (-not (Confirm-Action "是否备份并强制重新下载源码目录？")) {
            Write-Info "已保留本地目录：$($Opts.Dir)"
            return
        }
        Replace-NonGitSource | Out-Null
        return
    }

    Write-Info "检查远端更新：git -C $($Opts.Dir) fetch origin $Ref"
    & git -C $Opts.Dir fetch --quiet origin $Ref 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Warn "git fetch 失败（网络不通？），继续使用本地源码"
        return
    }
    $local = (& git -C $Opts.Dir rev-parse HEAD 2>$null | Out-String).Trim()
    $remote = (& git -C $Opts.Dir rev-parse FETCH_HEAD 2>$null | Out-String).Trim()
    $shortLocal = (& git -C $Opts.Dir rev-parse --short HEAD 2>$null | Out-String).Trim()
    if ($local -and ($local -eq $remote)) {
        Write-Info "源码已是最新（$shortLocal），跳过下载"
        return
    }
    $remoteShort = $remote
    if ($remote.Length -gt 8) { $remoteShort = $remote.Substring(0, 8) }
    Write-Info "检测到远端有更新：本地 $shortLocal -> 远端 $remoteShort"
    if (Test-SourceDirty) {
        Write-Warn "本地有未提交改动（默认不覆盖）"
        if (-not $Opts.ForceReset) {
            if ($Opts.Yes) {
                Write-Warn "非交互模式：保留本地改动，跳过本次更新（如需强更请加 --force-reset）"
                return
            }
            if (-not (Confirm-Action "是否丢弃本地已跟踪文件的改动并更新到 $Ref？")) {
                Write-Info "已保留本地源码与改动"
                return
            }
        }
    }
    Backup-EnvFile
    & git -C $Opts.Dir reset --hard "origin/$Ref" 2>$null
    if ($LASTEXITCODE -ne 0) { & git -C $Opts.Dir reset --hard FETCH_HEAD 2>$null }
    if ($LASTEXITCODE -ne 0) {
        Write-Err "增量更新失败，保留原有源码（未改动）"
        return
    }
    $newShort = (& git -C $Opts.Dir rev-parse --short HEAD 2>$null | Out-String).Trim()
    Write-Info "源码已增量更新：$shortLocal -> $newShort（drill.env 与数据卷均保留）"
}

# ---------- 3. 镜像来源 ----------
function Get-StatusCode([scriptblock]$req) {
    try {
        & $req | Out-Null
        return 200
    } catch {
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            return [int]$_.Exception.Response.StatusCode.value__
        }
        return 0
    }
}

function Probe-Ghcr {
    if ($Opts.DryRun) {
        Write-Host "  [dry-run] 匿名探测 GHCR manifest（期望 200）"
        return $true
    }
    try {
        $tok = (Invoke-RestMethod -Uri "https://ghcr.io/token?service=ghcr.io&scope=repository:chenjinxiang1980/cyber-drill:pull" -TimeoutSec 20 -ErrorAction Stop).token
    } catch {
        Write-Warn "GHCR 匿名 token 获取失败（网络不通或需要代理）。"
        return $false
    }
    if (-not $tok) { Write-Warn "GHCR 匿名 token 为空。"; return $false }
    $headers = @{
        Authorization = "Bearer $tok"
        Accept = 'application/vnd.oci.image.manifest.v1+json,application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.v2+json'
    }
    $code = Get-StatusCode { Invoke-WebRequest -Uri "https://ghcr.io/v2/chenjinxiang1980/cyber-drill/manifests/$Version" -Headers $headers -Method Head -UseBasicParsing -TimeoutSec 20 -ErrorAction Stop }
    Write-Info "GHCR 匿名 manifest 状态码：$code"
    return ($code -eq 200)
}

function Probe-Swr {
    if ($Opts.DryRun) {
        Write-Host "  [dry-run] 匿名探测 SWR manifest（含匿名 token 协商）：https://$SwrHost/v2/$SwrOrg/cyber-drill/manifests/$Version"
        return 'public'
    }
    $accept = 'application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json'
    $url = "https://$SwrHost/v2/$SwrOrg/cyber-drill/manifests/$Version"
    $code = 0
    $challenge = $null
    try {
        Invoke-WebRequest -Uri $url -Headers @{ Accept = $accept } -Method Get -UseBasicParsing -TimeoutSec 20 -ErrorAction Stop | Out-Null
        Write-Info "SWR 匿名 manifest 返回 200：仓库已公开，可匿名拉取，无需 AK。"
        return 'public'
    } catch {
        if (-not $_.Exception.Response) {
            Write-Err "无法连接 SWR：网络不通或 DNS 失败。"
            return 'network'
        }
        $code = [int]$_.Exception.Response.StatusCode.value__
        if ($code -eq 404) {
            Write-Err "SWR 返回 404：镜像 tag $Version 不存在。"
            return 'missing'
        }
        if ($code -ne 401 -and $code -ne 403) {
            Write-Warn "SWR 返回意外状态码 $code。"
            return 'auth'
        }
        try {
            $vals = @($_.Exception.Response.Headers.GetValues('WWW-Authenticate'))
            $challenge = $vals[0]
        } catch {
            $challenge = @($_.Exception.Response.Headers['WWW-Authenticate'])[0]
        }
    }

    # 华为 SWR 对公开仓库也会先返回 401 + Bearer challenge，
    # 需用 realm 换取匿名 token 后再判定是否真的可匿名拉取。
    if ($challenge) {
        $realm = $null; $service = ''; $scope = ''
        if ($challenge -match 'realm="([^"]+)"') { $realm = $matches[1] }
        if ($challenge -match 'service="([^"]+)"') { $service = $matches[1] }
        if ($challenge -match 'scope="([^"]+)"') { $scope = $matches[1] }
        if ($realm -and $scope) {
            $sep = if ($realm -match '\?') { '&' } else { '?' }
            $tokUrl = "$realm$sep" + "service=$service&scope=$([uri]::EscapeDataString($scope))"
            $tok = $null
            try { $tok = (Invoke-RestMethod -Uri $tokUrl -TimeoutSec 20 -ErrorAction Stop).token } catch { $tok = $null }
            if ($tok) {
                try {
                    Invoke-WebRequest -Uri $url -Headers @{ Authorization = "Bearer $tok"; Accept = $accept } -Method Get -UseBasicParsing -TimeoutSec 20 -ErrorAction Stop | Out-Null
                    Write-Info "SWR 匿名 token 协商成功（manifest 200）：仓库已公开，可匿名拉取，无需 AK。"
                    return 'public'
                } catch {
                    $code2 = 0
                    if ($_.Exception.Response) { $code2 = [int]$_.Exception.Response.StatusCode.value__ }
                    if ($code2 -eq 404) {
                        Write-Err "SWR 返回 404：镜像 tag $Version 不存在。"
                        return 'missing'
                    }
                    if ($code2 -eq 0) {
                        Write-Err "无法连接 SWR：网络不通或 DNS 失败。"
                        return 'network'
                    }
                }
            }
        }
    }
    Write-Warn "SWR 匿名鉴权未通过（HTTP $code）：仓库未公开或需要私有凭据（AK）。"
    return 'auth'
}

function Invoke-SwrLogin {
    Write-Host ""
    Write-Host "SWR 私有仓库需要 AK 登录，口令只通过 --password-stdin 传入。"
    $h = Read-Host "区域 endpoint [$SwrHost]"
    if (-not [string]::IsNullOrWhiteSpace($h)) { $script:SwrHost = $h.Trim() }
    $o = Read-Host "组织名 [$SwrOrg]"
    if (-not [string]::IsNullOrWhiteSpace($o)) { $script:SwrOrg = $o.Trim() }
    $ak = Read-Host "AK 用户名（如 cn-east-3@XXXX）"
    if ([string]::IsNullOrWhiteSpace($ak)) { Write-Err "AK 用户名不能为空。"; return $false }
    $pass = $env:SWR_PASS
    if (-not $pass) {
        $secure = Read-Host "登录口令（输入不回显）" -AsSecureString
        $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        try { $pass = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr) }
        finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr) }
    }
    if (-not $pass) { Write-Err "登录口令为空。"; return $false }
    if ($Opts.DryRun) {
        Write-Host "  [dry-run] printf <口令> | docker login $SwrHost -u $ak --password-stdin"
        return $true
    }
    $pass | & docker login $script:SwrHost -u $ak --password-stdin
    if ($LASTEXITCODE -ne 0) {
        Write-Err "docker login 失败（401/403 多为 AK 或权限问题，超时多为网络问题）。"
        return $false
    }
    return $true
}

function Get-Image {
    Write-Step "3/5 选择镜像来源"
    if (-not $Opts.Registry) {
        if ($Opts.Yes) {
            $Opts.Registry = 'ghcr'
        } else {
            $c = Choose-Item "请选择镜像来源：" "GHCR（境外，公开，免登录）" "SWR 华为云（国内，公开或需 AK）" "本地已导入镜像 / 离线 tar" "1"
            switch ($c) {
                '1' { $Opts.Registry = 'ghcr' }
                '2' { $Opts.Registry = 'swr' }
                '3' { $Opts.Registry = 'local' }
                default { Write-Err "无效选择：$c"; exit 1 }
            }
        }
    }

    switch ($Opts.Registry) {
        'ghcr' {
            Write-Info "镜像来源：GHCR（公开，匿名可拉）"
            Probe-Ghcr | Out-Null
            if ($Opts.DryRun) {
                Write-Host "  [dry-run] docker pull $GhcrImage"
                Write-Host "  [dry-run] docker tag $GhcrImage drill-server:$Version"
                return
            }
            & docker pull $GhcrImage
            if ($LASTEXITCODE -ne 0) { Write-Err "拉取失败：$GhcrImage（检查网络/代理，公开仓库无需登录）"; exit 1 }
            & docker tag $GhcrImage "drill-server:$Version"
        }
        'swr' {
            Write-Info "镜像来源：SWR（$SwrHost/$SwrOrg/cyber-drill）"
            $state = Probe-Swr
            if ($state -eq 'missing' -or $state -eq 'network') { exit 1 }
            $image = "$SwrHost/$SwrOrg/cyber-drill:$Version"
            if ($state -eq 'auth') {
                if ($Opts.DryRun) {
                    Write-Host "  [dry-run] 匿名失败后进入 AK 登录分支"
                } else {
                    if (-not (Confirm-Action "匿名拉取失败，是否输入 AK 登录后重试？")) { exit 1 }
                    if (-not (Invoke-SwrLogin)) { exit 1 }
                }
            }
            if ($Opts.DryRun) {
                Write-Host "  [dry-run] docker pull $image"
                Write-Host "  [dry-run] docker tag $image drill-server:$Version"
                return
            }
            & docker pull $image
            if ($LASTEXITCODE -ne 0) {
                Write-Warn "拉取失败：$image（请区分：网络不通 / 仓库未公开需鉴权 / tag 不存在）"
                if (-not $Opts.DryRun) {
                    if (Confirm-Action "是否输入 AK 登录后重试？") {
                        if (-not (Invoke-SwrLogin)) { exit 1 }
                        & docker pull $image
                        if ($LASTEXITCODE -ne 0) { Write-Err "登录后仍拉取失败：$image"; exit 1 }
                    } else {
                        exit 1
                    }
                }
            }
            & docker tag $image "drill-server:$Version"
        }
        'local' {
            Write-Info "镜像来源：本地镜像 / 离线 tar"
            if ($Opts.ImageTar) {
                if ($Opts.DryRun) { Write-Host "  [dry-run] docker load -i $($Opts.ImageTar)" }
                else {
                    & docker load -i $Opts.ImageTar
                    if ($LASTEXITCODE -ne 0) { Write-Err "docker load 失败：$($Opts.ImageTar)"; exit 1 }
                }
            }
            if ($Opts.DryRun) {
                Write-Host "  [dry-run] 检查本地镜像 drill-server:$Version"
                return
            }
            & docker image inspect "drill-server:$Version" 1>$null 2>$null
            if ($LASTEXITCODE -ne 0) {
                Write-Err "本地没有 drill-server:$Version，请先导入镜像或用 --image-tar 指定。"
                exit 1
            }
        }
    }
}

# ---------- 4. 环境文件 ----------
function Write-EnvFile([string]$template, [string]$target) {
    if ($Opts.DryRun) {
        Write-Host "  [dry-run] 从 $template 生成 $target（随机生成密钥）"
        return
    }
    if (-not (Test-Path $template)) { throw "缺少环境模板：$template" }

    $jwt = New-RandomHex 32
    $sess = New-RandomHex 32
    $dbpw = New-RandomHex 16
    $rootpw = New-RandomHex 16

    # 管理员初始口令由安装器为每个账号独立随机生成（前缀保证同时含字母与数字），
    # 只写入环境文件并在安装输出打印一次；首次登录强制改密。
    $adminpw = 'Ad7' + (New-RandomHex 16)
    $platpw  = 'Pd7' + (New-RandomHex 16)
    $script:GeneratedAdminPw = $adminpw
    $script:GeneratedPlatformPw = $platpw

    $text = Get-Content -Path $template -Raw -Encoding UTF8
    $text = $text.Replace('__REPLACE_WITH_DB_USER__', 'drill')
    $text = $text.Replace('__REPLACE_WITH_DB_PASSWORD__', $dbpw)
    $text = $text.Replace('__REPLACE_WITH_ROOT_PW__', $rootpw)
    $text = $text.Replace('__REPLACE_WITH_HEX32_JWT_SECRET__', $jwt)
    $text = $text.Replace('__REPLACE_WITH_HEX32_SESSION_SECRET__', $sess)
    $text = $text.Replace('__REPLACE_WITH_ADMIN_INIT_PASSWORD__', $adminpw)
    $text = $text.Replace('__REPLACE_WITH_PLATFORMADMIN_INIT_PASSWORD__', $platpw)
    $text = $text -replace '(?m)^VERSION=.*$', "VERSION=$Version"

    $append = @"

# ---------- 一键安装器追加 ----------
APP_HOST_PORT=$($Opts.Port)
MYSQL_HOST_PORT=$($Opts.MysqlPort)
DRILL_ENV_FILE=$($Opts.EnvFile)
APP_CONTAINER_NAME=$($Opts.Project)-app
MYSQL_CONTAINER_NAME=$($Opts.Project)-mysql
PUBLIC_BASE_URL=http://localhost:$($Opts.Port)
"@
    $text = $text + $append
    [IO.File]::WriteAllText($target, $text, (New-Object Text.UTF8Encoding($false)))
    Restrict-EnvFileAcl $target
}

# 环境文件仅保留当前用户访问（等价于 Unix 600），避免明文扩散。
function Restrict-EnvFileAcl([string]$path) {
    try {
        & icacls.exe $path /inheritance:r /grant:r "$($env:USERNAME):(F)" *> $null
        if ($LASTEXITCODE -ne 0) { Write-Warn "无法收紧环境文件权限，请手工检查：$path" }
    } catch {
        Write-Warn "无法收紧环境文件权限，请手工检查：$path"
    }
}

function Prepare-Env {
    Write-Step "4/5 准备环境文件"
    $script:ComposeFile = Join-Path $Opts.Dir 'deploy\docker-dist\docker-compose.yml'
    $script:EnvPath = Join-Path $Opts.Dir ("deploy\docker-dist\" + $Opts.EnvFile)
    $template = Join-Path $Opts.Dir 'deploy\docker-dist\.env.example'

    if ($Opts.DryRun) {
        Write-Host "  [dry-run] 环境文件：$($script:EnvPath)"
        if (Test-Path $script:EnvPath) { Write-Info "检测到已存在环境文件，将沿用（不覆盖）。" }
        else { Write-Info "环境文件不存在，将先生成随机密钥。" }
        return
    }
    if (-not (Test-Path $script:ComposeFile)) { throw "缺少 compose 文件：$($script:ComposeFile)" }

    if (Test-Path $script:EnvPath) {
        Write-Warn "检测到已有环境文件：$($script:EnvPath)"
        if ($Opts.Yes) {
            Write-Info "沿用已有环境文件（--yes 不覆盖）。"
        } else {
            Write-Host "  1) 沿用（推荐）"
            Write-Host "  2) 备份后重建（旧文件改名 .bak.<时间戳>）"
            Write-Host "  3) 退出"
            $c = Read-Host "请选择 [1]"
            if ([string]::IsNullOrWhiteSpace($c)) { $c = '1' }
            switch ($c) {
                '1' { Write-Info "沿用已有环境文件。" }
                '2' {
                    $bak = "$($script:EnvPath).bak.$(Get-Date -Format 'yyyyMMddHHmmss')"
                    Copy-Item $script:EnvPath $bak
                    Write-Info "已备份到：$bak"
                    Write-EnvFile $template $script:EnvPath
                }
                default { Write-Info "用户退出，未做任何变更。"; exit 0 }
            }
        }
    } else {
        Write-EnvFile $template $script:EnvPath
        Write-Info "已生成环境文件：$($script:EnvPath)"
    }

    # 兼容已存在的环境文件：收紧权限，供首次改密后回写清空初始口令。
    Restrict-EnvFileAcl $script:EnvPath

    $filePort = (Get-Content $script:EnvPath | Where-Object { $_ -match '^APP_HOST_PORT=' } | Select-Object -Last 1)
    if ($filePort) {
        $Opts.Port = [int]($filePort -replace '^APP_HOST_PORT=', '')
        Write-Info "沿用环境文件中的 APP_HOST_PORT=$($Opts.Port) 做健康检查。"
    }
}

# ---------- 5. 部署 ----------
function Invoke-Deploy {
    Write-Step "5/5 启动服务"
    Write-Info "compose 项目名：$($Opts.Project)"
    Write-Info "容器名：$($Opts.Project)-app / $($Opts.Project)-mysql"
    Write-Info "端口：应用 $($Opts.Port)，MySQL $($Opts.MysqlPort)"

    if ($Opts.DryRun) {
        Write-Host "  [dry-run] docker compose -p $($Opts.Project) -f $($script:ComposeFile) --env-file $($script:EnvPath) up -d"
        Write-Host "  [dry-run] 轮询 http://127.0.0.1:$($Opts.Port)/api/health 直到 200"
        return
    }

    $env:DRILL_ENV_FILE = $Opts.EnvFile
    & docker compose -p $Opts.Project -f $script:ComposeFile --env-file $script:EnvPath up -d
    if ($LASTEXITCODE -ne 0) {
        Write-Err "docker compose up -d 失败，以下为最近日志："
        & docker compose -p $Opts.Project -f $script:ComposeFile --env-file $script:EnvPath logs --tail=100
        Write-Err "回滚建议：docker compose -p $($Opts.Project) -f $($script:ComposeFile) --env-file $($script:EnvPath) down"
        exit 1
    }

    Write-Info "等待 /api/health 就绪（最多 180 秒）…"
    $deadline = (Get-Date).AddSeconds(180)
    $body = $null
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$($Opts.Port)/api/health" -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
            if ($resp.StatusCode -eq 200) { $body = $resp.Content; break }
        } catch { }
        Start-Sleep -Seconds 3
    }

    if ($body) {
        Write-Host ""
        Write-Host "[成功] 服务已启动。" -ForegroundColor Green
        Write-Host "  健康检查：$body"
        Write-Host "  访问地址：http://localhost:$($Opts.Port)"
        Write-Host "  管理员账号：admin / platformadmin"
        if ($script:GeneratedAdminPw) {
            Write-Host "  管理员初始口令（只显示这一次，已写入环境文件，首次登录必须改密）：" -ForegroundColor Yellow
            Write-Host "    admin          $($script:GeneratedAdminPw)" -ForegroundColor Yellow
            Write-Host "    platformadmin  $($script:GeneratedPlatformPw)" -ForegroundColor Yellow
        } else {
            Write-Host "  管理员初始口令：沿用 drill.env 中已有值；若为空，首次创建时会随机生成并写入启动日志"
        }
        Write-Host "  环境文件：$($script:EnvPath)（务必备份）"
        Write-Host ""
        Write-Host "注意：首次启动会自动创建 12 个角色、8 个场景/剧本节点与管理员账号（admin / platformadmin），无需手工 bootstrap。"
        Write-Host "停止服务：docker compose -p $($Opts.Project) -f $($script:ComposeFile) down（保留数据卷）"
        return
    }

    Write-Err "健康检查超时（180 秒内 http://127.0.0.1:$($Opts.Port)/api/health 未返回 200）。"
    Write-Host "最近 100 行日志："
    & docker compose -p $Opts.Project -f $script:ComposeFile --env-file $script:EnvPath logs --tail=100
    Write-Err "回滚建议：docker compose -p $($Opts.Project) -f $($script:ComposeFile) --env-file $($script:EnvPath) down"
    exit 1
}

# ---------- 主流程 ----------
Write-Host "Cyber-Drill $Version Windows 一键安装"
Write-Host "仓库：$GithubRepo"
Write-Host "      $GiteeRepo"
if ($Opts.DryRun) { Write-Host "模式：dry-run（只打印命令，不做实际变更）" }
if ($Opts.Yes)    { Write-Host "模式：--yes（跳过交互确认）" }

# 参数校验
if ($Opts.Port -lt 1 -or $Opts.Port -gt 65535) { Write-Err "--port 越界：$($Opts.Port)"; exit 1 }
if ($Opts.MysqlPort -lt 1 -or $Opts.MysqlPort -gt 65535) { Write-Err "--mysql-port 越界：$($Opts.MysqlPort)"; exit 1 }
if ($Opts.Port -eq $Opts.MysqlPort) { Write-Err "应用端口与 MySQL 端口不能相同"; exit 1 }
if ($Opts.Source -and $Opts.Source -notin @('github','gitee','local')) { Write-Err "--source 只能是 github / gitee / local"; exit 1 }
if ($Opts.Registry -and $Opts.Registry -notin @('ghcr','swr','local')) { Write-Err "--registry 只能是 ghcr / swr / local"; exit 1 }
if ([string]::IsNullOrWhiteSpace($Opts.Dir)) { Write-Err "--dir 不能为空"; exit 1 }
if ([string]::IsNullOrWhiteSpace($Opts.EnvFile)) { Write-Err "--env-file 不能为空"; exit 1 }

Ensure-Docker
Get-Source
Get-Image
Prepare-Env
Invoke-Deploy

if ($Opts.DryRun) {
    Write-Host ""
    Write-Host "[dry-run] 完成。未安装 Docker、未拉取镜像、未写文件、未启动容器。"
}
