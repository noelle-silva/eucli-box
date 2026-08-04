param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("01", "02", "03", "dev")]
    [string]$Stage,

    [string]$Mode = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-FullPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    return [System.IO.Path]::GetFullPath($Path)
}

function Save-ProcessEnvironment {
    param([Parameter(Mandatory = $true)][string[]]$Names)

    $saved = @{}
    foreach ($name in $Names) {
        $saved[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
    }
    return $saved
}

function Restore-ProcessEnvironment {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Saved,
        [Parameter(Mandatory = $true)][string[]]$Names
    )

    foreach ($name in $Names) {
        [System.Environment]::SetEnvironmentVariable($name, $Saved[$name], "Process")
    }
}

if ($Stage -eq "01") {
    if (-not [string]::IsNullOrWhiteSpace($Mode)) {
        throw "Stage 01 does not accept a mode."
    }
    $Mode = "full"
} elseif ($Stage -eq "02") {
    if ($Mode -ne "preflight" -and $Mode -ne "remote") {
        throw "Usage: verify-stage-02.cmd <preflight|remote>"
    }
} elseif ($Stage -eq "03") {
    if ([string]::IsNullOrWhiteSpace($Mode)) {
        $Mode = "default"
    } elseif ($Mode -ne "default" -and $Mode -ne "experience") {
        throw "Usage: verify-stage-03.cmd [experience]"
    }
} elseif ($Stage -eq "dev") {
    if (-not [string]::IsNullOrWhiteSpace($Mode) -and $Mode -ne "default") {
        throw "Usage: verify-dev-box.cmd"
    }
    $Mode = "default"
} else {
    throw "Usage: verify-stage-02.cmd <preflight|remote>"
}

$repositoryRoot = Get-FullPath (Join-Path $PSScriptRoot "..\..")
if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot "go.mod") -PathType Leaf)) {
    throw "Cannot locate the repository root."
}

$runId = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssfffffffZ", [Globalization.CultureInfo]::InvariantCulture)
$runRoot = Join-Path $repositoryRoot ".dev-workspace\.release\verification\stage-$Stage\run-$runId"
if (Test-Path -LiteralPath $runRoot) {
    throw "Verification run already exists: $runRoot"
}

$verificationCacheRoot = Join-Path $repositoryRoot ".dev-workspace\.release\verification\cache"

[System.IO.Directory]::CreateDirectory((Join-Path $runRoot "temp")) | Out-Null
[System.IO.Directory]::CreateDirectory((Join-Path $runRoot "cache")) | Out-Null

$environmentNames = @("TEMP", "TMP", "GOCACHE", "GOMODCACHE", "GOTMPDIR", "GOTELEMETRY")
$savedEnvironment = Save-ProcessEnvironment -Names $environmentNames
$verificationExitCode = 1
$finalizationSucceeded = $false
$manualCleanupRequired = $false

try {
    $env:TEMP = Join-Path $runRoot "temp"
    $env:TMP = $env:TEMP
    $env:GOCACHE = Join-Path $verificationCacheRoot "go-build"
    $env:GOMODCACHE = Join-Path $verificationCacheRoot "go-mod"
    $env:GOTMPDIR = Join-Path $runRoot "temp\bootstrap-go"
    $env:GOTELEMETRY = "off"

    foreach ($directory in @($env:GOCACHE, $env:GOMODCACHE, $env:GOTMPDIR)) {
        [System.IO.Directory]::CreateDirectory($directory) | Out-Null
    }

    Write-Host "Stage $Stage $Mode verification root: $runRoot"
    Push-Location $repositoryRoot
    try {
        $arguments = @("run", "./cmd/eucli-release-verify", "stage-$Stage", "-root", $repositoryRoot, "-run-root", $runRoot)
        if ($Stage -eq "02" -or $Stage -eq "03") {
            $arguments += @("-mode", $Mode)
        }
        if ($Stage -eq "dev") {
            $arguments = @("run", "./cmd/eucli-release-verify", "dev-local-box", "-root", $repositoryRoot, "-run-root", $runRoot, "-mode", $Mode)
        }
        & go @arguments
        $verificationExitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    if ($verificationExitCode -eq 0) {
        & (Join-Path $PSScriptRoot "finalize-verification.ps1") `
            -RepositoryRoot $repositoryRoot `
            -RunRoot $runRoot `
            -Stage $Stage `
            -Mode $Mode
        $finalReportPath = Join-Path $runRoot "evidence\report.json"
        $finalReport = Get-Content -LiteralPath $finalReportPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $manualCleanupRequired = $finalReport.status -eq "passed" -and $finalReport.cleanup.status -eq "manual_required"
        $finalizationSucceeded = $true
    }
} catch {
    Write-Error $_
} finally {
    Restore-ProcessEnvironment -Saved $savedEnvironment -Names $environmentNames
}

if ($verificationExitCode -ne 0) {
    Write-Host "[FAIL] Stage $Stage $Mode verification failed. Evidence retained at $runRoot"
    exit $verificationExitCode
}
if (-not $finalizationSucceeded) {
    Write-Host "[FAIL] Stage $Stage $Mode finalization failed. Remaining evidence retained at $runRoot"
    exit 1
}
if ($manualCleanupRequired) {
    Write-Host "[PASS] Stage $Stage $Mode verification checks passed. Manual cleanup required: $runRoot"
    Write-Host "Report: $runRoot\evidence\report.json"
    exit 0
}

Write-Host "[PASS] Stage $Stage $Mode verification passed. Report: $runRoot\evidence\report.json"
exit 0
