Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-FullPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    return [System.IO.Path]::GetFullPath($Path)
}

function Get-RepositoryRoot {
    $candidate = Get-FullPath (Join-Path $PSScriptRoot "..\..")
    if (-not (Test-Path -LiteralPath (Join-Path $candidate "go.mod") -PathType Leaf)) {
        throw "Cannot locate the repository root."
    }
    return $candidate
}

$repositoryRoot = Get-RepositoryRoot
$protocolTool = Join-Path $repositoryRoot ".fast-window-dev-protocol\fast-window-dev-tool.mjs"
$boxRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-runtime\eucli-box")
$boxExe = Join-Path $boxRoot "eucli-box.exe"

# 开发装配走协议工具的开发装配动作：构建当前源码成品，并按散装形态把程序、清单与图标
# 落到实例根；data\ 与 programs\ 原样保留，宿主可直接以该可执行文件注册。
Write-Host "Staging current source eucli-box -> $boxRoot"
& node $protocolTool eucli-box-stage-dev
if ($LASTEXITCODE -ne 0) {
    throw "eucli-box dev staging failed, exit code: $LASTEXITCODE"
}
if (-not (Test-Path -LiteralPath $boxExe -PathType Leaf)) {
    throw "eucli-box.exe not found after staging: $boxExe"
}
Write-Host "Stage finished: $boxExe"
