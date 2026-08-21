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

function Get-LatestDevelopmentPackage {
    param([Parameter(Mandatory = $true)][string]$PackageRoot)

    $candidates = foreach ($manifestFile in Get-ChildItem -LiteralPath $PackageRoot -Recurse -File -Filter "*.manifest.json") {
        try {
            $manifest = Get-Content -LiteralPath $manifestFile.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($manifest.verificationOnly -ne $true) {
                continue
            }
            if ([string]$manifest.artifact.kind -ne "eucli-box" -or [string]$manifest.platform -ne "windows-x64") {
                continue
            }
            $archiveName = [string]$manifest.archive.name
            $archivePath = Join-Path $manifestFile.DirectoryName $archiveName
            if ([string]::IsNullOrWhiteSpace($archiveName) -or -not (Test-Path -LiteralPath $archivePath -PathType Leaf)) {
                continue
            }
            [pscustomobject]@{
                ManifestPath = $manifestFile.FullName
                ArchivePath = $archivePath
                Version = [version][string]$manifest.version
                LastWriteTime = $manifestFile.LastWriteTime
            }
        }
        catch {
            continue
        }
    }

    return $candidates | Sort-Object Version, LastWriteTime -Descending | Select-Object -First 1
}

$repositoryRoot = Get-RepositoryRoot
$devRuntimeRoot = Get-FullPath (Join-Path $repositoryRoot ".dev-workspace\.dev-runtime")
$boxRoot = Get-FullPath (Join-Path $devRuntimeRoot "eucli-box")
$packageRoot = Get-FullPath (Join-Path $boxRoot "package")
$clientDataDir = Get-FullPath (Join-Path $devRuntimeRoot "client\data")
$clientDirectory = Get-FullPath (Join-Path $repositoryRoot "clients\eucli-studio")

[System.IO.Directory]::CreateDirectory($clientDataDir) | Out-Null
$package = Get-LatestDevelopmentPackage $packageRoot
if ($null -eq $package) {
    throw "没有找到开发业务端成品，请先运行 scripts\dev\prepare-dev-box.cmd。"
}

$env:EUCLI_DEV_BOX_SOURCE = "1"
$env:EUCLI_DEV_BOX_MANIFEST = $package.ManifestPath
$env:EUCLI_DEV_BOX_ARCHIVE = $package.ArchivePath
$env:EUCLI_DEV_BOX_BOX_ROOT = $boxRoot
$env:FW_APP_DATA_DIR = $clientDataDir

# 工具开发源：业务端安装 AI 工具时读取本地开发版工具成品，不访问线上发行仓库。
# 成品目录按 <kind>-<id>/<version>/ 组织，由 prepare-dev-box 之后运行 build-tools 产出。
$devToolPackageRoot = Get-FullPath (Join-Path $boxRoot "package")
$env:EUCLI_DEV_TOOL_SOURCE = "1"
$env:EUCLI_DEV_TOOL_PACKAGE_ROOT = $devToolPackageRoot

Write-Host "启动开发客户端，使用业务端版本：$($package.Version)"
Write-Host "开发客户端数据目录：$clientDataDir"
Push-Location $clientDirectory
try {
    & pnpm dev:app
    $clientExitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}
exit $clientExitCode
