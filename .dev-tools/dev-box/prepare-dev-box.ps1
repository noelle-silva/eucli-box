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
$packageRoot = Get-FullPath (Join-Path $boxRoot "package")
$prepareWorkRoot = Get-FullPath (Join-Path $boxRoot "work\prepare")
$prepareEvidenceRoot = Get-FullPath (Join-Path $boxRoot "logs\prepare-evidence")
$resultFile = Get-FullPath (Join-Path $boxRoot "work\latest-build-result.json")
$versionFile = Join-Path $repositoryRoot "internal\boxrelease\release.json"
$toolRuntimeRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-tools-runtime\dev-box")
$toolWorkRoot = Get-FullPath (Join-Path $toolRuntimeRoot "work")
$toolTempRoot = Get-FullPath (Join-Path $toolRuntimeRoot "temp")

foreach ($directory in @($devRuntimeRoot, $boxRoot, $packageRoot, $prepareWorkRoot, $prepareEvidenceRoot, $toolWorkRoot, $toolTempRoot)) {
    [System.IO.Directory]::CreateDirectory($directory) | Out-Null
}

$env:TEMP = $toolTempRoot
$env:TMP = $toolTempRoot
$env:GOTMPDIR = Join-Path $toolTempRoot "go"
[System.IO.Directory]::CreateDirectory($env:GOTMPDIR) | Out-Null

$release = Get-Content -LiteralPath $versionFile -Raw -Encoding UTF8 | ConvertFrom-Json
$version = [string]$release.version
$targetDirectory = Join-Path $packageRoot (Join-Path "eucli-box" $version)
if (Test-Path -LiteralPath $targetDirectory) {
    Remove-Item -LiteralPath $targetDirectory -Recurse -Force -ErrorAction Stop
    if (Test-Path -LiteralPath $targetDirectory) {
        throw "无法清理旧开发成品目录：$targetDirectory"
    }
}

Write-Host "制作当前源码业务端开发成品：$version"
Push-Location $toolWorkRoot
try {
    & go run devtools/eucli-release build `
        -root $repositoryRoot `
        -target eucli-box `
        -verification-only `
        -work-root $prepareWorkRoot `
        -output-root $packageRoot `
        -evidence-root $prepareEvidenceRoot `
        -result-file $resultFile
    if ($LASTEXITCODE -ne 0) {
        throw "当前源码成品制作失败，退出码：$LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
}
finally {
    Pop-Location
}

if (-not (Test-Path -LiteralPath $resultFile -PathType Leaf)) {
    throw "当前源码成品制作结果缺失。"
}

Write-Host "开发成品制作完成：$targetDirectory"
