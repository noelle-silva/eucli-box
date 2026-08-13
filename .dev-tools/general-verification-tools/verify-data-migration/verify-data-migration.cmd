@echo off
setlocal EnableExtensions
set "MODE=%~1"
if "%MODE%"=="" set "MODE=default"
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0..\..\common\verification-runtime\invoke-verification.ps1" -Tool "verify-data-migration" -Mode "%MODE%"
exit /b %ERRORLEVEL%
