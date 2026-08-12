@echo off
setlocal
call "%~dp0prepare-dev-box.cmd" %*
if errorlevel 1 exit /b %ERRORLEVEL%
call "%~dp0start-dev-box.cmd" %*
exit /b %ERRORLEVEL%
exit /b %ERRORLEVEL%
