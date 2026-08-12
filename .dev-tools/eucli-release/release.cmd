@echo off
setlocal
set "TOOLROOT=%~dp0"
set "TOOLROOT=%TOOLROOT:~0,-1%"
set "REPO_ROOT=%~dp0..\.."
set "RUNTIME=%REPO_ROOT%\.dev-workspace\.dev-tools-runtime\eucli-release"
set "TEMP=%RUNTIME%\temp"
set "TMP=%TEMP%"
set "GOTMPDIR=%RUNTIME%\temp\go"
mkdir "%RUNTIME%\temp" 2>nul
mkdir "%GOTMPDIR%" 2>nul
mkdir "%RUNTIME%\work" 2>nul
pushd "%RUNTIME%\work" || exit /b 1
go run devtools/eucli-release %*
set "STATUS=%ERRORLEVEL%"
popd
exit /b %STATUS%
