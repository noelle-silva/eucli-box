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

if defined TOOL_PARAM (
    pushd "%RUNTIME%\work" >nul 2>nul
    go run devtools/eucli-release-assets -target "tool:%TOOL_PARAM%" -root "%REPO_ROOT%" -output "%ASSET_ROOT%\prepared" -cache "%ASSET_ROOT%\cache" -temp "%ASSET_ROOT%\temp"
    set "ASSET_STATUS=%ERRORLEVEL%"
    popd >nul 2>nul
    if not "%ASSET_STATUS%"=="0" (
        echo [FAIL] prepare assets for tool:%TOOL_PARAM%
        exit /b %ASSET_STATUS%
    )
)

pushd "%RUNTIME%\work" || exit /b 1
if defined TOOL_PARAM (
    go run devtools/eucli-toolpack %* -asset-root-dir "%ASSET_ROOT%\prepared"
) else (
    go run devtools/eucli-toolpack %*
)
set "STATUS=%ERRORLEVEL%"
popd
exit /b %STATUS%
