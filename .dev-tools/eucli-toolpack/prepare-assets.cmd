@echo off
rem 准备一个工具发布所需的外部随包资产。
rem 独立成文件：资产准备会拉起 go 子进程，放在调用方脚本里执行会破坏调用方的批处理解析。
setlocal
set "REPO_ROOT=%~dp0..\.."
set "RUNTIME=%REPO_ROOT%\.dev-workspace\.dev-tools-runtime\eucli-toolpack"
set "TEMP=%RUNTIME%\temp"
set "TMP=%TEMP%"
set "GOTMPDIR=%RUNTIME%\temp\go"
set "ASSET_ROOT=%REPO_ROOT%\.dev-workspace\.release\verification\cache\assets"

if "%~1"=="" (
    echo [FAIL] prepare-assets 需要工具标识
    exit /b 1
)

pushd "%RUNTIME%\work" >nul 2>nul || exit /b 1
go run devtools/eucli-release-assets -target "tool:%~1" -root "%REPO_ROOT%" -output "%ASSET_ROOT%\prepared" -cache "%ASSET_ROOT%\cache" -temp "%ASSET_ROOT%\temp"
set "STATUS=%ERRORLEVEL%"
popd >nul 2>nul
exit /b %STATUS%
