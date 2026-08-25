@echo off
rem Build command-analyzer release binary into the development runtime slot.
rem Usage: run from anywhere; it locates the repo root relative to this file.
setlocal EnableExtensions

set "HERE=%~dp0"
set "REPO=%~dp0..\..\.."
pushd "%REPO%" >nul
if not exist go.mod (
    echo FAIL: repo root not found from %HERE%
    popd
    exit /b 1
)

set "VER=0.1.0"
set "SLOT=.dev-workspace\.dev-tools-runtime\shell-command-analyzer\%VER%"
if not exist "%SLOT%" mkdir "%SLOT%"

set "CARGO_TARGET_DIR=%REPO%\%SLOT%\cargo-target"
cargo build --release --manifest-path "%REPO%\tools\shell_command\analyzer\Cargo.toml"
if errorlevel 1 (
    popd
    exit /b 1
)

if not exist "%REPO%\%SLOT%\cargo-target\release\command-analyzer.exe" (
    echo FAIL: build product missing
    popd
    exit /b 1
)
copy /y "%REPO%\%SLOT%\cargo-target\release\command-analyzer.exe" "%REPO%\%SLOT%\command-analyzer.exe" >nul
popd
echo OK: command-analyzer %VER% ready at %SLOT%\command-analyzer.exe
exit /b 0
