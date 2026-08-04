@echo off
setlocal EnableExtensions
set "MODE=%~1"
if "%MODE%"=="" set "MODE=default"
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0invoke-verification.ps1" -Stage "03" -Mode "%MODE%"
exit /b %ERRORLEVEL%
