<#
.SYNOPSIS
    统一重置全部账号的默认密码（含 admin），置 must_change=0。
.DESCRIPTION
    本脚本用于把 drill_system 数据库中所有账号的密码重置为部署方指定的口令。
    遵循三项安全铁律：
      1. 口令来源：-Password 参数 > 环境变量 DRILL_DEFAULT_PASSWORD，两者都缺失 -> 报错退出，绝不使用内置明文
      2. 先备份再改：data/drill.db 复制到 data/backups/drill-<yyyyMMdd-HHmmss>.db，校验副本可打开 + users 行数一致，失败则中止
      3. 双写比对：同时写 MySQL drill_system.users 和 SQLite data/drill.db 的 users，写后逐账号比对 pwd_salt / pwd_hash / must_change

    注意：本脚本依赖 users 表的 login_attempts / locked_until 列。这两列由后端任务的数据库迁移脚本添加。
    当前版本仅支持 dry-run（-Confirm 才真正执行）。脚本现在只写不跑，等后端迁移完成后再执行。
.PARAMETER Password
    新密码（所有账号统一使用）。未提供则读环境变量 DRILL_DEFAULT_PASSWORD。两者都没有直接报错退出。
    注意：口令不会被打印到控制台或日志，只显示长度。
.PARAMETER Confirm
    真正执行修改。默认不指定则 dry-run 只打印计划。
.PARAMETER MysqlHost
    MySQL 主机，默认 127.0.0.1。
.PARAMETER MysqlPort
    MySQL 端口，默认 3306。
.PARAMETER MysqlUser
    MySQL 业务账号，默认 drill_app。
.PARAMETER MysqlPwd
    MySQL 业务账号密码。未提供则读环境变量 DB_PASSWORD。
.PARAMETER MysqlDb
    MySQL 业务库名，默认 drill_system。
.PARAMETER SqlitePath
    SQLite 数据库文件路径，默认 data\drill.db（相对于项目根目录）。
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\reset_default_passwords.ps1 -Password "NewStrongP@ss123"

    powershell -ExecutionPolicy Bypass -File scripts\reset_default_passwords.ps1 -Password "NewStrongP@ss123" -Confirm
#>

param(
    [string]$Password    = $null,
    [switch]$Confirm     = $false,
    [string]$MysqlHost   = "127.0.0.1",
    [int]$MysqlPort      = 3306,
    [string]$MysqlUser   = "drill_app",
    [string]$MysqlPwd    = $null,
    [string]$MysqlDb     = "drill_system",
    [string]$SqlitePath  = $null
)

$ErrorActionPreference = "Stop"

# ==================== 路径解析 ====================
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $projectRoot

if (-not $SqlitePath) {
    $SqlitePath = Join-Path $projectRoot "data\drill.db"
}
$backupDir   = Join-Path $projectRoot "data\backups"
if (-not (Test-Path $backupDir)) { New-Item -ItemType Directory -Path $backupDir -Force | Out-Null }

# ==================== 口令来源校验 ====================
$passwordSource = ""
if ($Password -and $Password.Length -gt 0) {
    $passwordSource = "cli:-Password"
} elseif ($env:DRILL_DEFAULT_PASSWORD -and $env:DRILL_DEFAULT_PASSWORD.Length -gt 0) {
    $Password = $env:DRILL_DEFAULT_PASSWORD
    $passwordSource = "ENV:DRILL_DEFAULT_PASSWORD"
} else {
    Write-Error "ERROR: no -Password param and DRILL_DEFAULT_PASSWORD env empty. Refusing to use built-in default."
    Write-Host "  Usage:" -ForegroundColor Yellow
    Write-Host '    powershell -File scripts\reset_default_passwords.ps1 -Password "NewStrongP@ss123" -Confirm' -ForegroundColor DarkGray
    Write-Host '    $env:DRILL_DEFAULT_PASSWORD = "NewStrongP@ss123"' -ForegroundColor DarkGray
    Write-Host '    powershell -File scripts\reset_default_passwords.ps1 -Confirm' -ForegroundColor DarkGray
    exit 1
}

# 绝不打印口令明文
Write-Host ("[reset] password source  : {0}" -f $passwordSource) -ForegroundColor Cyan
Write-Host ("[reset] password length  : {0}  (>= 16 recommended)" -f $Password.Length) -ForegroundColor Cyan

# ==================== MySQL 凭据 ====================
if (-not $MysqlPwd -and $env:DB_PASSWORD) {
    $MysqlPwd = $env:DB_PASSWORD
}
if (-not $MysqlPwd) {
    Write-Error "ERROR: no MySQL password (use -MysqlPwd or DB_PASSWORD env)"
    exit 1
}

# ==================== 工具检查 ====================
Write-Host '[reset] checking prerequisites...' -ForegroundColor DarkGray

try { $null = & python --version 2>&1 }
catch { Write-Error "ERROR: python not found (scrypt needs Python 3.6+)"; exit 1 }

$scryptOk = & python -c "import hashlib; print('scrypt-ok' if hasattr(hashlib, 'scrypt') else 'scrypt-missing')" 2>&1
if ($scryptOk -match 'scrypt-missing') { Write-Error "ERROR: Python hashlib.scrypt unavailable"; exit 1 }

$mysqlCmd = Get-Command mysql -ErrorAction SilentlyContinue
if (-not $mysqlCmd) { Write-Error "ERROR: mysql CLI not found"; exit 1 }

Write-Host '[reset] prerequisites OK (python scrypt + mysql CLI)' -ForegroundColor Green

# ==================== Python 哈希函数（单引号 here-string） ====================
$PY_SCRYPT = @'
import hashlib, sys
pw = sys.argv[1]
salt_hex = sys.argv[2]
h = hashlib.scrypt(
    pw.encode('utf-8'),
    salt=salt_hex.encode('utf-8'),
    n=16384, r=8, p=1, dklen=32
)
print(h.hex())
'@

function Get-ScryptHash([string]$password, [string]$saltHex) {
    $result = & python -c $PY_SCRYPT $password $saltHex 2>&1
    if ($LASTEXITCODE -ne 0) { throw "scrypt failed: $result" }
    return $result.Trim()
}

# ==================== Python SQLite 辅助（单引号 here-string） ====================
$PY_SQLITE = @'
import sqlite3, sys, base64
db = sys.argv[1]
sql = base64.b64decode(sys.argv[2]).decode('utf-8')
conn = sqlite3.connect(db)
cur = conn.cursor()
try:
    cur.execute(sql)
    rows = cur.fetchall()
    conn.commit()
    for r in rows:
        print('\t'.join(str(x) if x is not None else '' for x in r))
except Exception as e:
    print('ERROR:', e, file=sys.stderr)
    sys.exit(2)
finally:
    conn.close()
'@

function Invoke-Sqlite([string]$dbPath, [string]$sql) {
    $sqlB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sql))
    $result = & python -c $PY_SQLITE $dbPath $sqlB64 2>&1
    if ($LASTEXITCODE -eq 2) { throw "SQLite error: $result" }
    return $result
}

# ==================== MySQL 辅助 ====================
# NOTE: mysql outputs password warning to stderr.
# With $ErrorActionPreference="Stop" this becomes a terminating error.
# Suppress with SilentlyContinue inside function.
function Invoke-Mysql([string]$sql) {
    $argList = @("--host=$MysqlHost","--port=$MysqlPort","--user=$MysqlUser","--database=$MysqlDb","--batch","--raw","-e",$sql)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    $prevPwd = $env:MYSQL_PWD
    $env:MYSQL_PWD = $MysqlPwd
    try {
        $out = (& mysql @argList 2>$null)
        $ec = $LASTEXITCODE
        if ($ec -ne 0) {
            $e = (& mysql @argList 2>&1 | Out-String)
            throw "MySQL error (exit=$ec): $e"
        }
        # mysql --batch --raw always returns header row first. Skip it.
        $lines = @($out) -join "`n" -split "`n" | Where-Object { $_ -ne "" }
        if ($lines.Count -gt 0) { $lines = $lines | Select-Object -Skip 1 }
        return $lines
    } finally {
        $ErrorActionPreference = $prev
        if ($null -eq $prevPwd) { Remove-Item Env:MYSQL_PWD -ErrorAction SilentlyContinue }
        else { $env:MYSQL_PWD = $prevPwd }
    }
}

# ==================== MySQL 连接测试 ====================
$connLabel = "{0} at {1}:{2}/{3}" -f $MysqlUser, $MysqlHost, $MysqlPort, $MysqlDb
Write-Host ("[reset] testing MySQL connection ({0})..." -f $connLabel) -ForegroundColor DarkGray
try {
    $null = Invoke-Mysql "SELECT 1;"
    Write-Host '[reset] MySQL OK' -ForegroundColor Green
} catch {
    Write-Error ("ERROR: MySQL connection failed - {0}" -f $_); exit 1
}

# ==================== SQLite 文件检查 ====================
if (-not (Test-Path $SqlitePath)) {
    Write-Error ("ERROR: SQLite not found at {0}" -f $SqlitePath); exit 1
}
Write-Host ("[reset] SQLite OK: {0}" -f $SqlitePath) -ForegroundColor Green

# ==================== 检查 login_attempts / locked_until 列 ====================
Write-Host '[reset] checking login_attempts / locked_until columns...' -ForegroundColor DarkGray

$mysqlColumns = @{}
try {
    $colRows = Invoke-Mysql ("SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA='{0}' AND TABLE_NAME='users';" -f $MysqlDb)
    foreach ($line in $colRows) {
        if ($line -match '^(\S+)') { $mysqlColumns[$Matches[1].ToLower()] = $true }
    }
} catch {
    Write-Error "ERROR: cannot query MySQL schema - $_"; exit 1
}

$needCols = @("login_attempts", "locked_until")
$missingCols = $needCols | Where-Object { -not $mysqlColumns.ContainsKey($_) }
if ($missingCols) {
    Write-Error ("ERROR: MySQL users table missing required columns: {0}" -f ($missingCols -join ', '))
    Write-Error "       These columns are added by the backend migration. Run that first."
    exit 1
}
Write-Host '[reset] required columns present: login_attempts, locked_until' -ForegroundColor Green

# ==================== SQLite 列检查（双写前置条件） ====================
Write-Host '[reset] checking SQLite columns...' -ForegroundColor DarkGray
try {
    $sqliteSchema = Invoke-Sqlite $SqlitePath "PRAGMA table_info(users);"
    $sqliteCols = @{}
    foreach ($line in $sqliteSchema) {
        if ($line -match '^\d+\|([^|]+)\|') { $sqliteCols[$Matches[1].ToLower()] = $true }
    }
} catch {
    Write-Error "ERROR: cannot read SQLite schema - $_"; exit 1
}
$sqliteMissing = $needCols | Where-Object { -not $sqliteCols.ContainsKey($_) }
if ($sqliteMissing) {
    Write-Error ("ERROR: SQLite users table missing columns: {0}" -f ($sqliteMissing -join ', '))
    Write-Error "       MySQL users already updated (Step 3), but SQLite cannot be written."
    Write-Error "       Add columns manually: python -c 'import sqlite3; c=sqlite3.connect(); c.execute(\"ALTER TABLE users ADD COLUMN login_attempts INTEGER DEFAULT 0\"); c.execute(\"ALTER TABLE users ADD COLUMN locked_until DATETIME\"); c.commit(); c.close()'"
    Write-Error "       Or restore SQLite from backup and retry."
    exit 1
}
Write-Host '[reset] SQLite columns OK: login_attempts, locked_until' -ForegroundColor Green

# ==================== 读取账号 ====================
Write-Host '[reset] reading current accounts...' -ForegroundColor DarkGray

$mysqlUsers = @()
try {
    $userRows = Invoke-Mysql "SELECT id, username FROM users ORDER BY id;"
    foreach ($line in $userRows) {
        $parts = $line -split "`t"
        if ($parts.Count -ge 2) {
            $mysqlUsers += [PSCustomObject]@{ Id = $parts[0]; Username = $parts[1] }
        }
    }
} catch {
    Write-Error "ERROR: cannot read MySQL users - $_"; exit 1
}

$sqliteCount = (Invoke-Sqlite $SqlitePath "SELECT COUNT(*) FROM users;").Trim()
$mysqlCount  = $mysqlUsers.Count

if ($sqliteCount -ne $mysqlCount) {
    Write-Warning "[reset] WARNING: users row count differs - MySQL=$mysqlCount vs SQLite=$sqliteCount"
}

Write-Host ("[reset] found {0} accounts" -f $mysqlCount) -ForegroundColor Green

# ==================== 备份路径 ====================
$backupPath = Join-Path $backupDir ("drill-" + (Get-Date -Format "yyyyMMdd-HHmmss") + ".db")

# ==================== DRY-RUN ====================
if (-not $Confirm) {
    Write-Host ""
    Write-Host "============================================================" -ForegroundColor Yellow
    Write-Host "  DRY-RUN MODE  --  printing plan only, no changes" -ForegroundColor Yellow
    Write-Host "============================================================" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "[reset] Plan:" -ForegroundColor Cyan
    Write-Host "  1. Backup SQLite:" -ForegroundColor White
    Write-Host ("     Copy-Item {0} -> {1}" -f $SqlitePath, $backupPath) -ForegroundColor DarkGray
    Write-Host ("     Verify: readable + users rows = {0}" -f $mysqlCount) -ForegroundColor DarkGray
    Write-Host ""
    Write-Host ("  2. Generate scrypt hashes for {0} accounts:" -f $mysqlCount) -ForegroundColor White
    Write-Host "     - New 32-hex salt per account (crypto-random)" -ForegroundColor DarkGray
    Write-Host "     - scrypt(N=16384, r=8, p=1, dkLen=32)" -ForegroundColor DarkGray
    Write-Host "     - Set must_change=0, login_attempts=0, locked_until=NULL" -ForegroundColor DarkGray
    Write-Host ""
    Write-Host "  3. MySQL write: UPDATE users SET pwd_salt=?, pwd_hash=?, must_change=0," -ForegroundColor White
    Write-Host "                 login_attempts=0, locked_until=NULL WHERE id=?" -ForegroundColor DarkGray
    Write-Host ""
    Write-Host "  4. SQLite write: same UPDATE pattern" -ForegroundColor White
    Write-Host ""
    Write-Host "  5. Cross-check: compare pwd_salt / pwd_hash / must_change" -ForegroundColor White
    Write-Host "     for every account between MySQL and SQLite" -ForegroundColor DarkGray
    Write-Host ""
    Write-Host ("  Account list ({0} accounts):" -f $mysqlCount) -ForegroundColor White
    foreach ($u in $mysqlUsers) {
        Write-Host ("     [{0}] {1}" -f $u.Id, $u.Username) -ForegroundColor DarkGray
    }
    Write-Host ""
    Write-Host "[reset] Use -Confirm flag to actually execute." -ForegroundColor Yellow
    exit 0
}

# ==================== 真正执行 ====================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Red
Write-Host "  EXECUTION MODE  --  modifying database now" -ForegroundColor Red
Write-Host "============================================================" -ForegroundColor Red

# --- Step 1.1: 复制 ---
Write-Host ""
Write-Host '[reset] Step 1: backup SQLite...' -ForegroundColor Cyan
try {
    Copy-Item $SqlitePath $backupPath -Force
    Write-Host ("  backed up: {0}" -f $backupPath) -ForegroundColor Green
} catch {
    Write-Error ("ERROR: backup failed - {0}" -f $_); exit 1
}

# --- Step 1.2: 校验可打开 ---
try {
    Invoke-Sqlite $backupPath "SELECT 1;" | Out-Null
    Write-Host "  backup readable: OK" -ForegroundColor Green
} catch {
    Write-Error ("ERROR: backup not readable - {0}" -f $_); exit 1
}

# --- Step 1.3: 校验行数 ---
$backupCount = (Invoke-Sqlite $backupPath "SELECT COUNT(*) FROM users;").Trim()
if ($backupCount -ne $mysqlCount) {
    Write-Error ("ERROR: backup verification failed - backup users={0} vs MySQL={1}" -f $backupCount, $mysqlCount)
    Write-Error "       Original untouched. Script aborted."
    exit 1
}
Write-Host ("  backup row count: {0} (matches MySQL)" -f $backupCount) -ForegroundColor Green
Write-Host '[reset] backup verified OK' -ForegroundColor Green

# ==================== Step 2: 生成 salt + scrypt 哈希 ====================
Write-Host ""
Write-Host ("[reset] Step 2: generating scrypt hashes for {0} accounts..." -f $mysqlCount) -ForegroundColor Cyan

$accountUpdates = @()
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()

foreach ($u in $mysqlUsers) {
    $saltBytes = New-Object byte[] 16
    $rng.GetBytes($saltBytes)
    $saltHex = -join ($saltBytes | ForEach-Object { $_.ToString("x2") })
    $hashHex = Get-ScryptHash $Password $saltHex

    $accountUpdates += [PSCustomObject]@{
        Id         = $u.Id
        Username   = $u.Username
        Salt       = $saltHex
        Hash       = $hashHex
        MustChange = 0
    }
}

Write-Host '[reset] hashes generated OK' -ForegroundColor Green

# ==================== Step 3: MySQL 写入 ====================
Write-Host ""
Write-Host ("[reset] Step 3: updating MySQL {0}.users..." -f $MysqlDb) -ForegroundColor Cyan

foreach ($upd in $accountUpdates) {
    $sql = "UPDATE users SET pwd_salt='$($upd.Salt)', pwd_hash='$($upd.Hash)', must_change=0, login_attempts=0, locked_until=NULL WHERE id=$($upd.Id);"
    try {
        Invoke-Mysql $sql | Out-Null
        Write-Host ("  [{0}] {1}  updated" -f $upd.Id, $upd.Username) -ForegroundColor DarkGray
    } catch {
        Write-Error ("ERROR: MySQL update failed - [{0}] {1}: {2}" -f $upd.Id, $upd.Username, $_)
        Write-Error "       Some accounts written, some not. Restore from backup."
        exit 1
    }
}

Write-Host ("[reset] MySQL update complete ({0} accounts)" -f $mysqlCount) -ForegroundColor Green

# ==================== Step 4: SQLite 写入 ====================
Write-Host ""
Write-Host ("[reset] Step 4: updating SQLite {0} ..." -f $SqlitePath) -ForegroundColor Cyan

foreach ($upd in $accountUpdates) {
    $sql = "UPDATE users SET pwd_salt='$($upd.Salt)', pwd_hash='$($upd.Hash)', must_change=0, login_attempts=0, locked_until=NULL WHERE id=$($upd.Id);"
    try {
        Invoke-Sqlite $SqlitePath $sql | Out-Null
        Write-Host ("  [{0}] {1}  updated" -f $upd.Id, $upd.Username) -ForegroundColor DarkGray
    } catch {
        Write-Error ("ERROR: SQLite update failed - [{0}] {1}: {2}" -f $upd.Id, $upd.Username, $_)
        Write-Error "       MySQL already written. Check or restore from backup."
        exit 1
    }
}

Write-Host ("[reset] SQLite update complete ({0} accounts)" -f $mysqlCount) -ForegroundColor Green

# ==================== Step 5: 逐账号比对 ====================
Write-Host ""
Write-Host '[reset] Step 5: cross-checking MySQL vs SQLite...' -ForegroundColor Cyan

$mismatches = @()
foreach ($upd in $accountUpdates) {
    $mysqlRow  = Invoke-Mysql   ("SELECT pwd_salt, pwd_hash, must_change FROM users WHERE id={0};" -f $upd.Id)
    $sqliteRow = Invoke-Sqlite  $SqlitePath ("SELECT pwd_salt, pwd_hash, must_change FROM users WHERE id={0};" -f $upd.Id)

    $mParts = ($mysqlRow  | Select-Object -First 1) -split "`t"
    $sParts = ($sqliteRow | Select-Object -First 1) -split "`t"

    $matchSalt   = ($mParts[0] -eq $sParts[0])
    $matchHash   = ($mParts[1] -eq $sParts[1])
    $matchMustCh = ($mParts[2] -eq $sParts[2])

    if (-not ($matchSalt -and $matchHash -and $matchMustCh)) {
        $mismatches += ("  [{0}] {1}: Salt={2}, Hash={3}, MustCh={4}" -f $upd.Id, $upd.Username, $matchSalt, $matchHash, $matchMustCh)
    }
}

if ($mismatches.Count -gt 0) {
    Write-Error ("ERROR: MySQL vs SQLite mismatches:" + ($mismatches -join "`n"))
    Write-Error ("       All accounts saved in backup copy {0}" -f $backupPath)
    exit 1
}

Write-Host ("[reset] cross-check OK - all {0} accounts consistent" -f $mysqlCount) -ForegroundColor Green

# ==================== 完成 ====================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host ("  RESET COMPLETE  --  {0} accounts updated" -f $mysqlCount) -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host ("  backup    : {0}" -f $backupPath) -ForegroundColor DarkGray
Write-Host ("  mysql     : {0}:{1}/{2}.users" -f $MysqlHost, $MysqlPort, $MysqlDb) -ForegroundColor DarkGray
Write-Host ("  sqlite    : {0}" -f $SqlitePath) -ForegroundColor DarkGray
Write-Host "  must_change: 0 (no forced password change)" -ForegroundColor DarkGray
Write-Host ("  password  : *** (not logged - length {0}) ***" -f $Password.Length) -ForegroundColor DarkGray
Write-Host "============================================================" -ForegroundColor Green
