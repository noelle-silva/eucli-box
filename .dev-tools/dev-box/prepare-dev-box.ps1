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
$toolRuntimeRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-tools-runtime\dev-box")
$toolWorkRoot = Get-FullPath (Join-Path $toolRuntimeRoot "work")
$toolTempRoot = Get-FullPath (Join-Path $toolRuntimeRoot "temp")

foreach ($directory in @($devRuntimeRoot, $boxRoot, $toolWorkRoot, $toolTempRoot)) {
    [System.IO.Directory]::CreateDirectory($directory) | Out-Null
}

$env:TEMP = $toolTempRoot
$env:TMP = $toolTempRoot
$env:GOTMPDIR = Join-Path $toolTempRoot "go"
[System.IO.Directory]::CreateDirectory($env:GOTMPDIR) | Out-Null

# 本体自治：编译产物直接放到实例根（本体与 data\、programs\ 等平级），零传递定位信息。
$boxExe = Join-Path $boxRoot "eucli-box.exe"
Write-Host "Building current source eucli-box -> $boxExe"
Push-Location $repositoryRoot
try {
    & go build -o $boxExe ./cmd/eucli-box
    if ($LASTEXITCODE -ne 0) {
        throw "eucli-box build failed, exit code: $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
Write-Host "Build finished: $boxExe"
