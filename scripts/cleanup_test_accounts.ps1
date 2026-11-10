<#
.SYNOPSIS
    一键清理演练过程中临时创建的测试账号，保留基线 12 个账号。
.DESCRIPTION
    本脚本删除 MySQL drill_system 数据库中不在基线白名单内的账号。
    遵循三项安全铁律：
      1. 永远不允许删除 admin - 脚本里硬性拦截
      2. 默认 dry-run - 不指定 -Execute 只打印计划
      3. 先删 user_roles 再删 users - 符合外键约束

    默认只保留 admin；其余基线账号由环境变量 ADMIN_BASELINE_USERS
    （逗号分隔）提供，仓库内不内置任何真实账号。

    注意：本脚本只操作 MySQL，不碰 SQLite data/drill.db。
.PARAMETER Execute
    真正执行删除。默认 dry-run 只打印将被删除的账号列表。
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
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\cleanup_test_accounts.ps1

    powershell -ExecutionPolicy Bypass -File scripts\cleanup_test_accounts.ps1 -Execute
#>

param(
    [switch]$Execute    = $false,
    [string]$MysqlHost  = "127.0.0.1",
    [int]$MysqlPort     = 3306,
    [string]$MysqlUser  = "drill_app",
    [string]$MysqlPwd   = $null,
    [string]$MysqlDb    = "drill_system"
)

$ErrorActionPreference = "Stop"

# ==================== 基线白名单 ====================
# 开源版不内置真实账号：默认只保留 admin；
# 其余基线账号通过环境变量 ADMIN_BASELINE_USERS（逗号分隔）提供。
$BASELINE_ACCOUNTS = @("admin")
if ($env:ADMIN_BASELINE_USERS) {
    $BASELINE_ACCOUNTS += ($env:ADMIN_BASELINE_USERS -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ })
}

$baselineSet = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
foreach ($a in $BASELINE_ACCOUNTS) { [void]$baselineSet.Add($a) }

Write-Host ("[cleanup] baseline whitelist: {0} accounts" -f $BASELINE_ACCOUNTS.Count) -ForegroundColor Cyan
$BASELINE_EXPECTED_USERS      = 12
$BASELINE_EXPECTED_USERROLES  = 15

# ==================== MySQL 凭据 ====================
if (-not $MysqlPwd -and $env:DB_PASSWORD) {
    $MysqlPwd = $env:DB_PASSWORD
}
if (-not $MysqlPwd) {
    Write-Error "ERROR: no MySQL password (use -MysqlPwd or DB_PASSWORD env)"; exit 1
}

# ==================== MySQL 辅助 ====================
# IMPORTANT: mysql 把 password 警告输出到 stderr。
# 在 $ErrorActionPreference="Stop" 下这会变成终止错误，
# 所以函数内部用 SilentlyContinue 屏蔽，只靠 LASTEXITCODE 判断。
function Invoke-Mysql([string]$sql) {
    $prevEAP = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    $prevPwd = $env:MYSQL_PWD
    $env:MYSQL_PWD = $MysqlPwd
    try {
        $argList = @("--host=$MysqlHost","--port=$MysqlPort","--user=$MysqlUser","--database=$MysqlDb","--batch","--raw","-e",$sql)
        $out = (& mysql @argList 2>$null)
        $ec = $LASTEXITCODE
        if ($ec -ne 0) {
            $e = (& mysql @argList 2>&1 | Out-String)
            throw "MySQL error (exit=$ec): $e"
        }
        # mysql --batch --raw returns header row first. Skip it.
        $lines = @($out) -join "`n" -split "`n" | Where-Object { $_ -ne "" }
        if ($lines.Count -gt 0) { $lines = $lines | Select-Object -Skip 1 }
        return $lines
    } finally {
        $ErrorActionPreference = $prevEAP
        if ($null -eq $prevPwd) { Remove-Item Env:MYSQL_PWD -ErrorAction SilentlyContinue }
        else { $env:MYSQL_PWD = $prevPwd }
    }
}

# ==================== 连接测试 ====================
$connLabel = "{0} at {1}:{2}/{3}" -f $MysqlUser, $MysqlHost, $MysqlPort, $MysqlDb
Write-Host ("[cleanup] connecting MySQL ({0})..." -f $connLabel) -ForegroundColor DarkGray
try {
    $null = Invoke-Mysql "SELECT 1;"
    Write-Host '[cleanup] MySQL OK' -ForegroundColor Green
} catch {
    Write-Error ("ERROR: MySQL connection failed - {0}" -f $_); exit 1
}

# ==================== 读取全量账号 ====================
Write-Host '[cleanup] reading all accounts...' -ForegroundColor DarkGray

try {
    $userRows = Invoke-Mysql "SELECT id, username, name FROM users ORDER BY username;"
} catch {
    Write-Error ("ERROR: cannot read users table - {0}" -f $_); exit 1
}

$allUsers = @()
foreach ($line in $userRows) {
    $parts = $line -split "`t"
    if ($parts.Count -ge 2) {
        $nm = ""
        if ($parts.Count -ge 3) { $nm = $parts[2] }
        $allUsers += [PSCustomObject]@{
            Id       = [int]$parts[0]
            Username = $parts[1]
            Name     = $nm
        }
    }
}

# ==================== admin 硬拦截 ====================
foreach ($u in $allUsers) {
    if ($u.Username -ieq "admin" -and -not $baselineSet.Contains("admin")) {
        Write-Error "FATAL: admin NOT in baseline! Data anomaly. Script aborted."; exit 1
    }
}

# ==================== 分类 ====================
$toKeep   = @()
$toDelete = @()

foreach ($u in $allUsers) {
    if ($baselineSet.Contains($u.Username)) {
        $toKeep += $u
    } else {
        $toDelete += $u
    }
}

Write-Host ("[cleanup] total accounts: {0}" -f $allUsers.Count) -ForegroundColor Cyan
Write-Host ("[cleanup] keep:   {0}  (baseline whitelist)" -f $toKeep.Count) -ForegroundColor Green
Write-Host ("[cleanup] delete: {0}  (non-baseline)" -f $toDelete.Count) -ForegroundColor Yellow

# ==================== admin 安全拦截 ====================
$adminInDelete = $toDelete | Where-Object { $_.Username -ieq "admin" }
if ($adminInDelete) {
    Write-Error "FATAL: admin is in delete list! Hard block. Script aborted."; exit 1
}
Write-Host '[cleanup] admin protection check: OK (admin will not be deleted)' -ForegroundColor Green

# ==================== 读取将被删除账号的 user_roles ====================
$deleteUserIds = ""
if ($toDelete.Count -gt 0) {
    $deleteUserIds = ($toDelete | ForEach-Object { $_.Id }) -join ","
}

$affectedRoles = @()
if ($deleteUserIds) {
    try {
        $roleSql = "SELECT ur.user_id, u.username, ur.role_id, r.name FROM user_roles ur JOIN users u ON u.id = ur.user_id JOIN roles r ON r.id = ur.role_id WHERE ur.user_id IN ($deleteUserIds) ORDER BY u.username, ur.role_id;"
        $roleRows = Invoke-Mysql $roleSql
        foreach ($line in $roleRows) {
            $parts = $line -split "`t"
            if ($parts.Count -ge 4) {
                $affectedRoles += [PSCustomObject]@{
                    UserId   = [int]$parts[0]
                    Username = $parts[1]
                    RoleId   = $parts[2]
                    RoleName = $parts[3]
                }
            }
        }
    } catch {
        Write-Error ("ERROR: cannot read user_roles - {0}" -f $_); exit 1
    }
}

Write-Host ("[cleanup] user_roles rows to delete: {0}" -f $affectedRoles.Count) -ForegroundColor Yellow

# ==================== DRY-RUN ====================
if (-not $Execute) {
    Write-Host ""
    Write-Host "============================================================" -ForegroundColor Yellow
    Write-Host "  DRY-RUN MODE  --  printing plan only, no deletes" -ForegroundColor Yellow
    Write-Host "============================================================" -ForegroundColor Yellow
    Write-Host ""

    Write-Host ("Accounts to delete ({0}):" -f $toDelete.Count) -ForegroundColor Red
    if ($toDelete.Count -eq 0) {
        Write-Host "  (none - all accounts in baseline)" -ForegroundColor Green
    } else {
        foreach ($u in $toDelete) {
            Write-Host ("  [{0}] {1}  ({2})" -f $u.Id, $u.Username, $u.Name) -ForegroundColor DarkGray
        }
    }

    Write-Host ""
    Write-Host ("user_roles rows to delete ({0}):" -f $affectedRoles.Count) -ForegroundColor Red
    if ($affectedRoles.Count -eq 0) {
        Write-Host "  (none)" -ForegroundColor Green
    } else {
        foreach ($r in $affectedRoles) {
            Write-Host ("  user_id={0} ({1})  role_id={2} ({3})" -f $r.UserId, $r.Username, $r.RoleId, $r.RoleName) -ForegroundColor DarkGray
        }
    }

    Write-Host ""
    Write-Host ("Baseline accounts kept ({0}):" -f $toKeep.Count) -ForegroundColor Green
    foreach ($u in $toKeep) {
        Write-Host ("  [{0}] {1}  ({2})" -f $u.Id, $u.Username, $u.Name) -ForegroundColor DarkGray
    }

    Write-Host ""
    Write-Host "[cleanup] Use -Execute flag to actually delete." -ForegroundColor Yellow
    Write-Host "          Order: DELETE user_roles first, then users" -ForegroundColor Yellow
    exit 0
}

# ==================== 真正执行 ====================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Red
Write-Host ("  EXECUTION MODE  --  deleting {0} accounts" -f $toDelete.Count) -ForegroundColor Red
Write-Host "============================================================" -ForegroundColor Red

if ($toDelete.Count -eq 0) {
    Write-Host "[cleanup] nothing to delete, already at baseline" -ForegroundColor Green
} else {
    $beforeUsersCount = [int]((Invoke-Mysql "SELECT COUNT(*) FROM users;").Trim())
    $beforeRolesCount = [int]((Invoke-Mysql "SELECT COUNT(*) FROM user_roles;").Trim())
    Write-Host ("[cleanup] before: users={0}, user_roles={1}" -f $beforeUsersCount, $beforeRolesCount) -ForegroundColor DarkGray

    # Step 1: 先删 user_roles
    Write-Host ""
    Write-Host '[cleanup] Step 1: DELETE FROM user_roles WHERE user_id IN (...)' -ForegroundColor Cyan
    try {
        $del = (Invoke-Mysql ("DELETE FROM user_roles WHERE user_id IN ($deleteUserIds); SELECT ROW_COUNT();")).Trim()
        Write-Host ("  deleted {0} user_roles rows" -f $del) -ForegroundColor Green
    } catch {
        Write-Error ("ERROR: user_roles delete failed - {0}" -f $_); exit 1
    }

    # Step 2: 再删 users
    Write-Host ""
    Write-Host '[cleanup] Step 2: DELETE FROM users WHERE id IN (...)' -ForegroundColor Cyan
    try {
        $del = (Invoke-Mysql ("DELETE FROM users WHERE id IN ($deleteUserIds); SELECT ROW_COUNT();")).Trim()
        Write-Host ("  deleted {0} users rows" -f $del) -ForegroundColor Green
    } catch {
        Write-Error ("ERROR: users delete failed - {0}" -f $_)
        Write-Host '       user_roles already deleted. Check or restore.' -ForegroundColor Red
        exit 1
    }

    # Step 3: 删除后计数 + 基线比对
    $afterUsersCount = [int]((Invoke-Mysql "SELECT COUNT(*) FROM users;").Trim())
    $afterRolesCount = [int]((Invoke-Mysql "SELECT COUNT(*) FROM user_roles;").Trim())

    Write-Host ""
    Write-Host ("[cleanup] after: users={0}, user_roles={1}" -f $afterUsersCount, $afterRolesCount) -ForegroundColor DarkGray
    Write-Host ("[cleanup] removed: users={0}, user_roles={1}" -f ($beforeUsersCount - $afterUsersCount), ($beforeRolesCount - $afterRolesCount)) -ForegroundColor DarkGray
    Write-Host ""
    Write-Host '[cleanup] baseline verification:' -ForegroundColor Cyan

    $usersMatch = ($afterUsersCount -eq $BASELINE_EXPECTED_USERS)
    $rolesMatch = ($afterRolesCount -eq $BASELINE_EXPECTED_USERROLES)

    if ($usersMatch) {
        Write-Host ("  users baseline  : {0} / {1}  [OK]" -f $afterUsersCount, $BASELINE_EXPECTED_USERS) -ForegroundColor Green
    } else {
        Write-Host ("  users baseline  : {0} / {1}  [MISMATCH]" -f $afterUsersCount, $BASELINE_EXPECTED_USERS) -ForegroundColor Red
        Write-Error ("FATAL: user count differs from baseline {0}. Manual check required." -f $BASELINE_EXPECTED_USERS)
        exit 1
    }

    if ($rolesMatch) {
        Write-Host ("  user_roles count: {0} / {1}  [OK]" -f $afterRolesCount, $BASELINE_EXPECTED_USERROLES) -ForegroundColor Green
    } else {
        Write-Host ("  user_roles count: {0} / {1}  [MISMATCH]" -f $afterRolesCount, $BASELINE_EXPECTED_USERROLES) -ForegroundColor Red
        Write-Error ("FATAL: user_roles count differs from baseline {0}. Manual check required." -f $BASELINE_EXPECTED_USERROLES)
        exit 1
    }
}

Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host "  CLEANUP COMPLETE" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host ("  deleted users: {0}" -f $toDelete.Count) -ForegroundColor DarkGray
Write-Host ("  kept users   : {0}" -f $toKeep.Count) -ForegroundColor DarkGray
Write-Host ("  mysql        : {0}:{1}/{2}" -f $MysqlHost, $MysqlPort, $MysqlDb) -ForegroundColor DarkGray
Write-Host "  admin        : PROTECTED (never deleted)" -ForegroundColor DarkGray
Write-Host "============================================================" -ForegroundColor Green
