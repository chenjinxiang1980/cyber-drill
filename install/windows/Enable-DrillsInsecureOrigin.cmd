@echo off
setlocal
chcp 65001 >nul
rem ============================================================
rem  Cyber-Drill: enable Chromium policy
rem  OverrideSecurityRestrictionsOnInsecureOrigin
rem  for http://192.168.97.131:8090
rem
rem  Usage:
rem    Enable-DrillsInsecureOrigin.cmd            apply  (HKLM, UAC)
rem    Enable-DrillsInsecureOrigin.cmd -Remove    remove (HKLM, UAC)
rem
rem  After running, close the browser COMPLETELY and reopen it,
rem  then check chrome://policy / edge://policy.
rem ============================================================

set "PS1=%~dp0Enable-DrillsInsecureOrigin.ps1"

if not exist "%PS1%" (
  echo [X] Missing file: %PS1%
  pause
  exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -File "%PS1%" %*
set "RC=%ERRORLEVEL%"

echo.
if not "%RC%"=="0" echo [X] Script exited with code %RC%.
pause
exit /b %RC%
