<#
.SYNOPSIS
  为内核演练系统写入 Chromium 企业策略 OverrideSecurityRestrictionsOnInsecureOrigin，
  使 http://192.168.97.131:8090 被浏览器视为安全上下文（secure context），
  从而恢复 navigator.mediaDevices.getDisplayMedia 屏幕抓取能力。

.DESCRIPTION
  策略类型为 List of strings：注册表里每个地址是一个独立的字符串值，
  值名必须是 "1"、"2"、"3" …（不是分号拼接的一整串）。
  同时写入 Google Chrome 与 Microsoft Edge 两个策略位置。
  写入 HKLM 需要管理员权限；加 -CurrentUser 改写到 HKCU（仅当前用户，无需提权）。

.PARAMETER Remove
  撤销：删除两个浏览器下的该策略子键，并提示完全退出浏览器。

.PARAMETER Origin
  需要放行的 Origin 列表（必须带 scheme，可带端口，不能带路径）。
  默认 http://192.168.97.131:8090

.PARAMETER CurrentUser
  写到 HKCU\SOFTWARE\Policies 而不是 HKLM\SOFTWARE\Policies（无需管理员）。

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 -Remove

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 `
      -Origin http://192.168.97.131:8090,http://drill.example.local:8090 -CurrentUser
#>
[CmdletBinding()]
param(
    [switch]$Remove,
    [string[]]$Origin = @('http://192.168.97.131:8090'),
    [switch]$CurrentUser
)

$ErrorActionPreference = 'Stop'
$PolicyName = 'OverrideSecurityRestrictionsOnInsecureOrigin'

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Show-RestartHint {
    $running = @()
    foreach ($n in @('chrome', 'msedge')) {
        $p = Get-Process -Name $n -ErrorAction SilentlyContinue
        if ($p) {
            $cnt = @($p).Count
            $running += "$n.exe（$cnt 个进程）"
        }
    }
    Write-Host ''
    Write-Host '下一步（必须执行，否则策略不生效）：' -ForegroundColor Cyan
    Write-Host '  1) 完全退出浏览器：关闭所有窗口，并确认任务管理器里没有 chrome.exe / msedge.exe 残留'
    Write-Host '     （含右下角托盘常驻进程；也可执行：taskkill /f /im chrome.exe /im msedge.exe）'
    Write-Host '  2) 重新打开浏览器，地址栏访问 chrome://policy 或 edge://policy'
    Write-Host "     搜索 $PolicyName，状态应显示 OK"
    Write-Host '  3) 再访问演练系统，按 F12 在 Console 里执行：'
    Write-Host '     isSecureContext   // 期望 true'
    Write-Host '     typeof navigator.mediaDevices.getDisplayMedia   // 期望 "function"'
    if ($running.Count -gt 0) {
        Write-Host ''
        Write-Host ('[!] 检测到浏览器正在运行：' + ($running -join '、') + ' —— 现在必须全部退出后重开。') -ForegroundColor Yellow
    }
}

# ---------- 提权（写 HKLM 需要管理员） ----------
if (-not $CurrentUser -and -not (Test-Admin)) {
    $argLine = "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`""
    if ($Remove) { $argLine += ' -Remove' }
    $argLine += ' -Origin ' + (($Origin | ForEach-Object { '"' + $_ + '"' }) -join ',')
    Write-Host '[i] 写入 HKLM 需要管理员权限，正在请求提权（请在 UAC 弹窗点“是”）…' -ForegroundColor Yellow
    try {
        Start-Process -FilePath 'powershell.exe' -ArgumentList $argLine -Verb RunAs -Wait
    } catch {
        Write-Host '[X] 提权被取消，策略未写入。' -ForegroundColor Red
        exit 1
    }
    exit 0
}

$Root = if ($CurrentUser) { 'HKCU:\SOFTWARE\Policies' } else { 'HKLM:\SOFTWARE\Policies' }

$Targets = @(
    [pscustomobject]@{ Browser = 'Google Chrome'; Key = "$Root\Google\Chrome\$PolicyName" },
    [pscustomobject]@{ Browser = 'Microsoft Edge'; Key = "$Root\Microsoft\Edge\$PolicyName" }
)

Write-Host ('作用范围：' + $(if ($CurrentUser) { 'HKCU（当前用户）' } else { 'HKLM（本机所有用户）' }))
Write-Host ''

# ---------- 撤销 ----------
if ($Remove) {
    foreach ($t in $Targets) {
        if (Test-Path $t.Key) {
            Remove-Item -Path $t.Key -Recurse -Force
            Write-Host "[OK] 已删除 $($t.Browser) 策略：$PolicyName" -ForegroundColor Green
        } else {
            Write-Host "[--] $($t.Browser) 未配置该策略，跳过"
        }
    }
    Show-RestartHint
    Write-Host ''
    Write-Host '撤销完成。浏览器完全退出并重开后，对应地址将恢复为普通 HTTP（非安全上下文）。' -ForegroundColor Green
    exit 0
}

# ---------- 校验收参 ----------
$origins = @($Origin | Where-Object { $_ } | ForEach-Object { $_.Trim() } | Select-Object -Unique)
if ($origins.Count -eq 0) {
    Write-Host '[X] 未提供任何 Origin。' -ForegroundColor Red
    exit 1
}
foreach ($o in $origins) {
    if ($o -notmatch '^https?://') {
        Write-Host "[X] Origin 必须以 http:// 或 https:// 开头：$o" -ForegroundColor Red
        exit 1
    }
    if ($o -match '^https?://[^/]+/') {
        Write-Host "[X] 这里只能填 Origin（协议+主机+端口），不能带路径：$o" -ForegroundColor Red
        exit 1
    }
}

# ---------- 写入 ----------
foreach ($t in $Targets) {
    if (-not (Test-Path $t.Key)) {
        New-Item -Path $t.Key -Force | Out-Null
    }
    # 清掉旧的编号值，避免历史残留混入
    $old = (Get-Item -Path $t.Key).Property | Where-Object { $_ -match '^\d+$' }
    foreach ($v in $old) {
        Remove-ItemProperty -Path $t.Key -Name $v -Force
    }
    for ($i = 0; $i -lt $origins.Count; $i++) {
        New-ItemProperty -Path $t.Key -Name ([string]($i + 1)) -Value $origins[$i] `
            -PropertyType String -Force | Out-Null
    }
    Write-Host "[OK] $($t.Browser)：$($origins.Count) 条" -ForegroundColor Green
    foreach ($v in ((Get-Item -Path $t.Key).Property | Where-Object { $_ -match '^\d+$' } | Sort-Object { [int]$_ })) {
        Write-Host ("     `"{0}`" = `"{1}`"" -f $v, (Get-ItemProperty -Path $t.Key -Name $v).$v)
    }
}

Write-Host ''
Write-Host ("注册表位置：" + ($Targets | ForEach-Object { $_.Key } | Out-String).Trim())
Show-RestartHint
Write-Host ''
Write-Host "撤销方法：powershell -NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -Remove" -ForegroundColor Cyan
