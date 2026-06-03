@echo off
setlocal
set "ROOT=%~dp0.."
pushd "%ROOT%" || exit /b 1
go run ./cmd/worktree-overlay %*
set "STATUS=%ERRORLEVEL%"
popd
exit /b %STATUS%
