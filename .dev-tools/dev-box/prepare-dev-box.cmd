@echo off
setlocal
pwsh.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0prepare-dev-box.ps1" %*
exit /b %ERRORLEVEL%
