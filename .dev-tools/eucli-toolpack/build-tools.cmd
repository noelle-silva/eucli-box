@echo off
setlocal
set "TOOLROOT=%~dp0"
set "TOOLROOT=%TOOLROOT:~0,-1%"
set "REPO_ROOT=%~dp0..\.."
set "RUNTIME=%REPO_ROOT%\.dev-workspace\.dev-tools-runtime\eucli-toolpack"
set "TEMP=%RUNTIME%\temp"
set "TMP=%TEMP%"
set "GOTMPDIR=%RUNTIME%\temp\go"
mkdir "%RUNTIME%\temp" 2>nul
mkdir "%GOTMPDIR%" 2>nul
mkdir "%RUNTIME%\work" 2>nul

set "TOOL_PARAM="
:parseargs
if "%~1"=="" goto argsdone
if /i "%~1"=="-tool" (
    set "TOOL_PARAM=%~2"
    shift
    shift
    goto parseargs
)
shift
goto parseargs
:argsdone

set "ASSET_ROOT=%REPO_ROOT%\.dev-workspace\.release\verification\cache\assets"

if not defined TOOL_PARAM goto assets-prepared
call "%TOOLROOT%\prepare-assets.cmd" "%TOOL_PARAM%"
if errorlevel 1 (
    echo [FAIL] prepare assets for tool:%TOOL_PARAM%
    exit /b 1
)
:assets-prepared

pushd "%RUNTIME%\work" || exit /b 1
if defined TOOL_PARAM (
    go run devtools/eucli-toolpack %* -asset-root-dir "%ASSET_ROOT%\prepared"
) else (
    go run devtools/eucli-toolpack %*
)
set "STATUS=%ERRORLEVEL%"
popd
exit /b %STATUS%
