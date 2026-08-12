@echo off
setlocal EnableExtensions
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0..\..\common\verification-runtime\invoke-verification.ps1" -Stage "01"
exit /b %ERRORLEVEL%
