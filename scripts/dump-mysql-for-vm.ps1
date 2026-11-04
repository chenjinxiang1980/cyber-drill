<#
.SYNOPSIS
    Dump drill_system MySQL to dist/drill_system-dump.sql

.DESCRIPTION
    - Reads DB_* credentials from root .env (never prints plaintext)
    - mysqldump with --single-transaction --set-gtid-purged=OFF --default-character-set=utf8mb4
    - Post-dump integrity check: 7 hardcoded table counts
    - Any count mismatch -> WARN (dump still written)

.NOTES
    No source code touched. No .env read except DB_* keys. No install.
#>

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

$distDir  = Join-Path $projectRoot "dist"
$envFile  = Join-Path $projectRoot ".env"
$dumpFile = Join-Path $distDir "drill_system-dump.sql"

if (-not (Test-Path $distDir)) { New-Item -ItemType Directory -Force -Path $distDir | Out-Null }

# ---------------------------------------------------------------
# 1. Load .env
# ---------------------------------------------------------------
Write-Host "[dump] 1. read .env..."
if (-not (Test-Path $envFile)) {
    Write-Host "[dump]   FAIL: .env not found ($envFile)" -ForegroundColor Red
    Write-Host "   Copy .env.example to .env and fill DB_* credentials first." -ForegroundColor Yellow
    exit 1
}

$envMap = @{}
Get-Content $envFile | ForEach-Object {
    $line = $_.Trim()
    if (-not $line -or $line.StartsWith('#')) { return }
    if ($line -match '^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(.*)$') {
        $k = $Matches[1]
        $v = $Matches[2].Trim('"', "'")
        $envMap[$k] = $v
    }
}

function Get-Env($key, $default) {
    if ($envMap.ContainsKey($key) -and $envMap[$key]) { return $envMap[$key] }
    return $default
}

$dbHost   = Get-Env "DB_HOST"              "127.0.0.1"
$dbPort   = Get-Env "DB_PORT"              "3306"
$dbName   = Get-Env "DB_NAME"              "drill_system"
$rootUser = Get-Env "DB_ROOT_USER"         "root"
$rootPwd  = Get-Env "DB_ROOT_PASSWORD"     ""

Write-Host "[dump]   host=$dbHost port=$dbPort db=$dbName root_user=$rootUser" -ForegroundColor DarkGray
Write-Host "[dump]   root_password=<hidden> (never printed)" -ForegroundColor DarkGray

# ---------------------------------------------------------------
# 2. mysqldump check
# ---------------------------------------------------------------
Write-Host "[dump] 2. check mysqldump..."
$md = Get-Command mysqldump -ErrorAction SilentlyContinue
if (-not $md) {
    Write-Host "[dump]   FAIL: mysqldump not in PATH" -ForegroundColor Red
    Write-Host "   Scoop: scoop install mysql  ; or install MySQL Installer." -ForegroundColor Yellow
    exit 1
}
Write-Host "[dump]   OK ($($md.Source))" -ForegroundColor Green

# ---------------------------------------------------------------
# 3. Run dump
# ---------------------------------------------------------------
Write-Host "[dump] 3. dumping $dbName..."
if (Test-Path $dumpFile) { Remove-Item -Force $dumpFile }

$prevPwd = $env:MYSQL_PWD
$env:MYSQL_PWD = $rootPwd
try {
    $tmpWarn = Join-Path $distDir "_dump-warn.log"
    & mysqldump --host=$dbHost --port=$dbPort --user=$rootUser `
        --single-transaction --set-gtid-purged=OFF --default-character-set=utf8mb4 `
        --routines --triggers --events `
        $dbName 2> $tmpWarn `
        | Out-File -FilePath $dumpFile -Encoding utf8
    $rc = $LASTEXITCODE
} finally {
    $env:MYSQL_PWD = $prevPwd
}

if ($rc -ne 0 -or -not (Test-Path $dumpFile) -or (Get-Item $dumpFile).Length -eq 0) {
    Write-Host "[dump]   FAIL (exit=$rc)" -ForegroundColor Red
    if (Test-Path $tmpWarn) {
        Write-Host "   errors:" -ForegroundColor Yellow
        Get-Content $tmpWarn | ForEach-Object { Write-Host "   $_" -ForegroundColor Yellow }
        Remove-Item $tmpWarn -Force
    }
    exit 1
}

$dumpSize = [math]::Round((Get-Item $dumpFile).Length / 1KB, 1)
Write-Host "[dump]   OK ($dumpSize KB)" -ForegroundColor Green

# Cleanup warn log if only contains password-on-command-line warning
if (Test-Path $tmpWarn) {
    $w = Get-Content $tmpWarn -Raw
    if ($w -match 'Using a password on the command line') { Remove-Item $tmpWarn -Force }
    else { Write-Host "[dump]   WARNING: non-empty mysqldump warnings, kept at $tmpWarn" -ForegroundColor Yellow }
}

# ---------------------------------------------------------------
# 4. Integrity: 7 checks
# ---------------------------------------------------------------
Write-Host "[dump] 4. integrity checks (7 tables)..."
$checks = @(
    @{ Table = "users";          Expect = 12 },
    @{ Table = "user_roles";     Expect = 15 },
    @{ Table = "projects";       Expect = 1  },
    @{ Table = "script_nodes";   Expect = 331 },
    @{ Table = "impact";         Expect = 6;  Cond = "WHERE project_id=''" },
    @{ Table = "roles";          Expect = 12 },
    @{ Table = "form_templates"; Expect = 9  }
)

$mysqlCli = Get-Command mysql -ErrorAction SilentlyContinue
if (-not $mysqlCli) {
    Write-Host "[dump]   SKIP checks: mysql CLI not available" -ForegroundColor Yellow
    Write-Host "   .sql generated; verify counts after import into VM." -ForegroundColor Yellow
} else {
    $allOk = $true
    foreach ($c in $checks) {
        $cond = if ($c.ContainsKey("Cond")) { $c.Cond } else { "" }
        $sql  = "SELECT COUNT(*) FROM $($c.Table) $cond"
        $prevPwd2 = $env:MYSQL_PWD
        $env:MYSQL_PWD = $rootPwd
        try {
            $result = (& mysql --host=$dbHost --port=$dbPort --user=$rootUser --batch --raw --skip-column-names `
                $dbName -e $sql 2>$null)
        } finally {
            $env:MYSQL_PWD = $prevPwd2
        }
        $actual = [int]($result -as [string])
        $pad = $c.Table.PadRight(18)
        if ($actual -eq $c.Expect) {
            Write-Host "   PASS  $pad = $actual  (expect $($c.Expect))" -ForegroundColor Green
        } else {
            Write-Host "   FAIL  $pad = $actual  (expect $($c.Expect))" -ForegroundColor Red
            $allOk = $false
        }
    }
    if (-not $allOk) {
        Write-Host ""
        Write-Host "[dump]   !! Some table counts are unexpected." -ForegroundColor Yellow
        Write-Host "   Dump still generated, but verify manually before importing to VM." -ForegroundColor Yellow
    }
}

# ---------------------------------------------------------------
# 5. Done
# ---------------------------------------------------------------
Write-Host ""
Write-Host "[dump] 5. done" -ForegroundColor Green
Write-Host "   file : $dumpFile"
Write-Host "   size : $dumpSize KB"
Write-Host "   host : ${dbHost}:${dbPort} / $dbName"
Write-Host ""
Write-Host "Next:" -ForegroundColor Cyan
Write-Host "   Copy dist\drill-linux-bundle.tar.gz + dist\drill_system-dump.sql to VM" -ForegroundColor DarkGray
Write-Host "   Import: mysql -u root -p drill_system < drill_system-dump.sql" -ForegroundColor DarkGray
Write-Host "   Then run bundle compose/systemd" -ForegroundColor DarkGray
