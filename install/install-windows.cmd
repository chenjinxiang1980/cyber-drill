@echo off
setlocal
chcp 65001 >nul
rem ============================================================
rem  Cyber-Drill Windows one-click installer launcher
rem  Double-click to run. ExecutionPolicy is bypassed here so
rem  users are not blocked by "running scripts is disabled".
rem ============================================================

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-windows.ps1" %*
set "RC=%ERRORLEVEL%"

if not "%RC%"=="0" (
  echo.
  echo Installer exited with code %RC%.
)

if "%~1"=="" pause
exit /b %RC%
