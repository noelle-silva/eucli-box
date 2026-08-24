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
$clientRoot = Get-FullPath (Join-Path $devRuntimeRoot "client")
$clientDataDir = Get-FullPath (Join-Path $clientRoot "data")
$clientTargetDir = Get-FullPath (Join-Path $clientRoot "target")
$clientDirectory = Get-FullPath (Join-Path $repositoryRoot "clients\eucli-studio")

foreach ($directory in @($devRuntimeRoot, $clientRoot, $clientDataDir, $clientTargetDir)) {
    [System.IO.Directory]::CreateDirectory($directory) | Out-Null
}

$env:FW_APP_DATA_DIR = $clientDataDir
$env:CARGO_TARGET_DIR = $clientTargetDir

Write-Host ""
Write-Host "eucli-studio (dev) is starting."
Write-Host "    Client data: $clientDataDir"
Write-Host ""
Write-Host "Open the window, enter the box address + key on the connection screen."
Write-Host "    Address: http://127.0.0.1:8765"
Write-Host "    Key:     see the box terminal (run-dev-box.cmd prints it)"
Write-Host ""

Push-Location $clientDirectory
try {
    & pnpm dev:app
    $exitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}
exit $exitCode
