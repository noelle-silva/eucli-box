@echo off
setlocal EnableExtensions
set "MODE=%~1"
if "%MODE%"=="" set "MODE=default"
if not "%MODE%"=="default" (
  echo Usage: verify-session-facts-regression.cmd [default] 1>&2
  exit /b 2
)
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0..\..\common\verification-runtime\invoke-verification.ps1" -Tool "verify-session-facts-regression" -Mode "%MODE%"
exit /b %ERRORLEVEL%
