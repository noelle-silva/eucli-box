@echo off
setlocal EnableExtensions
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0invoke-verification.ps1" -Stage "dev" -Mode "default"
exit /b %ERRORLEVEL%
