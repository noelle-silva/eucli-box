@echo off
setlocal
pwsh.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0run-client.ps1" %*
exit /b %ERRORLEVEL%
