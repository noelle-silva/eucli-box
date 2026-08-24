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

function New-RandomKey {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return [Convert]::ToHexString($bytes).ToLowerInvariant()
}

$repositoryRoot = Get-RepositoryRoot
$devRuntimeRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-runtime")
$boxRoot = Get-FullPath (Join-Path $devRuntimeRoot "eucli-box")
$binDir = Get-FullPath (Join-Path $boxRoot "bin")
$boxExe = Join-Path $binDir "eucli-box.exe"

if (-not (Test-Path -LiteralPath $boxExe -PathType Leaf)) {
    throw "eucli-box.exe not found. Run prepare-dev-box first."
}

$dataDir = Get-FullPath (Join-Path $boxRoot "data")
[System.IO.Directory]::CreateDirectory($dataDir) | Out-Null

# 开发态钥匙与正式态同源：写 data/meta/box.key，首次启动生成一次，以后复用。
# 客户端连接屏只需配置一次地址+钥匙，重启业务端不再变化。
$metaDir = Get-FullPath (Join-Path $dataDir "meta")
[System.IO.Directory]::CreateDirectory($metaDir) | Out-Null
$keyFile = Join-Path $metaDir "box.key"
if (-not (Test-Path -LiteralPath $keyFile -PathType Leaf)) {
    $key = New-RandomKey
    [System.IO.File]::WriteAllText($keyFile, $key + "`n", [System.Text.Encoding]::ASCII)
}
$key = (Get-Content -LiteralPath $keyFile -Raw).Trim()

$env:EUCLI_BOX_ADDR = "127.0.0.1:8765"
$env:EUCLI_BOX_DATA_DIR = $dataDir

# Tool development source: the box reads local-built tool artifacts instead of
# the online release store when EUCLI_DEV_TOOL_SOURCE=1. The package root follows
# the build-tools output convention (<kind>-<id>/<version>/).
$devToolPackageRoot = Get-FullPath (Join-Path $boxRoot "package")
if (Test-Path -LiteralPath $devToolPackageRoot -PathType Container) {
    $env:EUCLI_DEV_TOOL_SOURCE = "1"
    $env:EUCLI_DEV_TOOL_PACKAGE_ROOT = $devToolPackageRoot
}

Write-Host ""
Write-Host "eucli-box (current source) is ready."
Write-Host "    Address: http://127.0.0.1:8765"
Write-Host "    Key:     $key"
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
