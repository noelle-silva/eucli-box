@echo off
setlocal
pwsh.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0start-dev-box.ps1" %*
exit /b %ERRORLEVEL%
