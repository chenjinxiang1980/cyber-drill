param(
    [switch]$Detach,
    [string]$Port = "8090"
)

$ErrorActionPreference = "Stop"

# ---- Paths ----
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $projectRoot
$serverDir   = Join-Path $projectRoot "server"
$buildDir    = Join-Path $serverDir "deploy"
$exePath     = Join-Path $buildDir "drill-server.exe"
$PORT        = $Port

Set-Location $projectRoot
$mode = if ($Detach) { "DETACHED (WMI)" } else { "FOREGROUND" }
Write-Host "[drill] mode=$mode" -ForegroundColor Cyan
Write-Host "[drill] project root: $projectRoot" -ForegroundColor Cyan

# ---- 1. Build ----
$needBuild = $false
if (-not (Test-Path $exePath)) {
    $needBuild = $true
} else {
    $srcLatest = (Get-ChildItem -Path $serverDir -Recurse -Filter "*.go" -ErrorAction SilentlyContinue | Measure-Object LastWriteTime -Maximum).Maximum
    if ($srcLatest) {
        $exeTime = (Get-Item $exePath).LastWriteTime
        if ($srcLatest -gt $exeTime) { $needBuild = $true }
    }
}

if ($needBuild) {
    Write-Host "[drill] building (via scripts/build.ps1)..." -ForegroundColor Yellow
    $buildScript = Join-Path $projectRoot "scripts" "build.ps1"
    if (Test-Path $buildScript) {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $buildScript
        $rc = $LASTEXITCODE
    } else {
        # legacy fallback (unlikely: scripts/build.ps1 is committed)
        if (-not (Test-Path $buildDir)) { New-Item -ItemType Directory -Path $buildDir -Force | Out-Null }
        Push-Location $serverDir
        go build -o $exePath . 2>&1 | ForEach-Object { Write-Host "  $_" }
        $rc = $LASTEXITCODE
        Pop-Location
    }
    if ($rc -ne 0) { Write-Host "[drill] build FAIL (exit=$rc)" -ForegroundColor Red; exit 1 }
    Write-Host "[drill] built: $exePath ($([math]::Round((Get-Item $exePath).Length/1MB,1)) MB)" -ForegroundColor Green
} else {
    Write-Host "[drill] reusing: $exePath" -ForegroundColor Green
}

# ---- 2. Port conflict (safe: only kill own drill) ----
$listener = Get-NetTCPConnection -LocalPort $PORT -State Listen -ErrorAction SilentlyContinue
if ($listener) {
    $lp = $listener | Select-Object -First 1
    $cp = $lp.OwningProcess
    $cpName = (Get-Process -Id $cp -ErrorAction SilentlyContinue).ProcessName
    $cpPath = (Get-CimInstance Win32_Process -Filter "ProcessId = $cp" -ErrorAction SilentlyContinue).ExecutablePath

    $ownExeLeaf = Split-Path $exePath -Leaf
    $cpExeLeaf  = Split-Path $cpPath -Leaf

    if ($cpName -eq 'drill-server' -and $cpExeLeaf -eq $ownExeLeaf -and $cpPath -like "$serverDir*") {
        Write-Host "[drill] port $PORT owned by previous drill (PID=$cp), killing..." -ForegroundColor DarkYellow
        Stop-Process -Id $cp -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
        $still = Get-NetTCPConnection -LocalPort $PORT -State Listen -ErrorAction SilentlyContinue
        if ($still) { Write-Host "[drill] port still busy after kill, wait more..." -ForegroundColor Yellow; Start-Sleep -Seconds 3 }
    } else {
        Write-Host "[drill] port $PORT occupied by NON-DRILL process: PID=$cp NAME=$cpName PATH=$cpPath" -ForegroundColor Red
        Write-Host "[drill] refusing to kill. Free the port manually and retry." -ForegroundColor Red
        exit 1
    }
}

# ---- 3. Launch ----
$cmdLine = "`"$exePath`" -root `"$projectRoot`""
$drillPid = $null

if ($Detach) {
    Write-Host "[drill] launching via WMI (detached)..." -ForegroundColor Cyan
    $wmiArgs = @{
        CommandLine     = $cmdLine
        CurrentDirectory = $projectRoot
    }
    $result = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments $wmiArgs
    if ($result.ReturnValue -ne 0) {
        Write-Host "[drill] WMI Create FAIL (ReturnValue=$($result.ReturnValue))" -ForegroundColor Red
        exit 1
    }
    $drillPid = $result.ProcessId

    $ppid = (Get-CimInstance Win32_Process -Filter "ProcessId = $drillPid").ParentProcessId
    $pproc = Get-CimInstance Win32_Process -Filter "ProcessId = $ppid" -ErrorAction SilentlyContinue
    Write-Host "[drill] PID=$drillPid PPID=$pppid ($($pproc.Name))" -ForegroundColor Green
    Write-Host "[drill] parent is WmiPrvSE.exe - shell can exit freely" -ForegroundColor DarkGray
} else {
    Write-Host "[drill] launching via Start-Process (foreground)..." -ForegroundColor Cyan
    $drill = Start-Process -FilePath $exePath `
                           -ArgumentList @('-root', $projectRoot) `
                           -WorkingDirectory $projectRoot `
                           -WindowStyle Minimized `
                           -PassThru
    $drillPid = $drill.Id
    Write-Host "[drill] PID=$drillPid parent=PowerShell (this window)" -ForegroundColor Green
}

# ---- 4. Health check ----
$baseUrl = "http://127.0.0.1:$PORT/api/health"
Write-Host "[drill] health check... " -NoNewline -ForegroundColor Cyan
$ok = $false
for ($i = 0; $i -lt 15; $i++) {
    Start-Sleep -Milliseconds 500
    try {
        $resp = [System.Net.WebRequest]::Create($baseUrl).GetResponse()
        if ($resp.StatusCode -eq 'OK') { $ok = $true; $resp.Close(); break }
        $resp.Close()
    } catch {}
    Write-Host "." -NoNewline
}

if ($ok) {
    Write-Host " PASS" -ForegroundColor Green
    Write-Host ""
    Write-Host "==========================================" -ForegroundColor Green
    Write-Host "  RUNNING:  http://127.0.0.1:$PORT"         -ForegroundColor Green
    Write-Host "  CRED:     see .env (ADMIN_INIT_PASSWORD / DRILL_INIT_PWD)" -ForegroundColor DarkGray
    Write-Host "  MONITOR:  /public/screen.html"             -ForegroundColor Green
    Write-Host "  PID:      $drillPid"                       -ForegroundColor Green
    if ($Detach) {
        Write-Host "  MODE:     DETACHED (survives this window)" -ForegroundColor Green
        Write-Host "  STOP:     Stop-Process -Name drill-server -Force" -ForegroundColor Yellow
    } else {
        Write-Host "  MODE:     FOREGROUND (dies with this window)" -ForegroundColor Green
        Write-Host "  Ctrl+C to stop gracefully"                -ForegroundColor Yellow
    }
    Write-Host "==========================================" -ForegroundColor Green
} else {
    Write-Host " FAIL (15s timeout)" -ForegroundColor Red
    Stop-Process -Id $drillPid -Force -ErrorAction SilentlyContinue
    exit 1
}

# ---- 5. Keepalive loop (foreground only) ----
if (-not $Detach) {
    Write-Host "[drill] keepalive loop started. Ctrl+C to exit." -ForegroundColor DarkGray
    try {
        while ($true) {
            Start-Sleep -Seconds 30
            try { $null = [System.Net.WebRequest]::Create($baseUrl).GetResponse() } catch {
                if ($drill.HasExited) {
                    Write-Host "[drill] unexpected exit (code=$($drill.ExitCode))" -ForegroundColor Red
                    break
                }
            }
        }
    } finally {
        Write-Host ""
        Write-Host "[drill] cleaning up PID=$drillPid..." -ForegroundColor Yellow
        try { Stop-Process -Id $drillPid -Force -ErrorAction SilentlyContinue } catch {}
        Write-Host "[drill] stopped" -ForegroundColor Green
    }
}
# Detach mode: script exits here, drill keeps running under WmiPrvSE.exe
