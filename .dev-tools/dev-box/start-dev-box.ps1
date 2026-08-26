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
$devRuntimeRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-runtime")
$boxRoot = Get-FullPath (Join-Path $devRuntimeRoot "eucli-box")
$boxExe = Join-Path $boxRoot "eucli-box.exe"

if (-not (Test-Path -LiteralPath $boxExe -PathType Leaf)) {
    throw "eucli-box.exe not found. Run prepare-dev-box first."
}

# 本体自治：实例根由本体自己推导（本体与 data\、programs\ 平级），零传递定位信息。
# 钥匙由本体首次启动时自行生成并记录；地址使用默认值。
Write-Host ""
Write-Host "eucli-box (current source) is ready."
Write-Host "    Address: http://127.0.0.1:8765"
Write-Host ""
Write-Host "Open the client and enter the address + key on the connection screen."
Write-Host "Press Ctrl+C to stop the box."
Write-Host ""

Push-Location $repositoryRoot
try {
    & $boxExe
    $exitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}
exit $exitCode
