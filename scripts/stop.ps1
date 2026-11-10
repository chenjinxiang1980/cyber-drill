param(
    [string]$Port = "8090"
)

$ErrorActionPreference = "SilentlyContinue"
$killed = 0

# Strategy 1: match by exe name
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $projectRoot
$serverBin = Join-Path (Join-Path $projectRoot "server") "deploy"
$all = Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'drill-server.exe' } -ErrorAction SilentlyContinue
foreach ($p in $all) {
    $procId = $p.ProcessId
    Write-Host "[drill-stop] found PID=$procId PPID=$($p.ParentProcessId) EXE=$($p.ExecutablePath)" -ForegroundColor Yellow
    # Only kill if under project server/bin to be safe
    if ($p.ExecutablePath -and $p.ExecutablePath -like "$serverBin*") {
        try { Stop-Process -Id $procId -Force; Write-Host "[drill-stop] killed PID=$procId" -ForegroundColor Green; $killed++ }
        catch { Write-Host "[drill-stop] fail PID=$procId - $_" -ForegroundColor Red }
    } else {
        Write-Host "[drill-stop] skipping (not under $serverBin)" -ForegroundColor DarkGray
    }
}

# Strategy 2: match by port (fallback)
if ($killed -eq 0) {
    Write-Host "[drill-stop] no process by name, scanning port $Port..." -ForegroundColor DarkGray
    $listeners = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    foreach ($lp in $listeners) {
        $owningPid = $lp.OwningProcess
        $owningProc = Get-CimInstance Win32_Process -Filter "ProcessId = $owningPid" -ErrorAction SilentlyContinue
        Write-Host "[drill-stop] port listener PID=$owningPid NAME=$($owningProc.Name) EXE=$($owningProc.ExecutablePath)" -ForegroundColor Yellow
        if ($owningProc.Name -eq 'drill-server') {
            try { Stop-Process -Id $owningPid -Force; Write-Host "[drill-stop] killed PID=$owningPid" -ForegroundColor Green; $killed++ }
            catch { Write-Host "[drill-stop] fail PID=$owningPid - $_" -ForegroundColor Red }
        } else {
            Write-Host "[drill-stop] skipping non-drill process" -ForegroundColor DarkGray
        }
    }
}

Start-Sleep -Seconds 1
$stillCount = (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Measure-Object).Count
if ($stillCount -gt 0) {
    Write-Host "[drill-stop] port $Port still LISTENING" -ForegroundColor Red
} else {
    Write-Host "[drill-stop] port $Port freed" -ForegroundColor Green
}
Write-Host "[drill-stop] total killed: $killed"
