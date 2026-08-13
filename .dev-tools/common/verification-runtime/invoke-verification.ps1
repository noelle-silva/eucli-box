param(
    [Parameter(Mandatory = $true)]
    [string]$Tool,

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

# 每个验证工具自己声明允许的模式；工具名必须是描述性命名（verify-<对象>-<动作>）。
$modeRules = @{
    "verify-release-build"        = @{ AllowNoMode = $true;  Modes = @();             DefaultMode = "full" }
    "verify-release-publish"      = @{ AllowNoMode = $false; Modes = @("preflight", "remote"); DefaultMode = "" }
    "verify-client-install"       = @{ AllowNoMode = $true;  Modes = @("experience"); DefaultMode = "default" }
    "verify-tool-plugin-update"   = @{ AllowNoMode = $true;  Modes = @("experience"); DefaultMode = "default" }
    "verify-background-access"    = @{ AllowNoMode = $true;  Modes = @("experience"); DefaultMode = "default" }
    "verify-data-migration"       = @{ AllowNoMode = $true;  Modes = @();             DefaultMode = "default" }
    "verify-box-update"           = @{ AllowNoMode = $true;  Modes = @("experience"); DefaultMode = "default" }
    "verify-dev-box"              = @{ AllowNoMode = $true;  Modes = @();             DefaultMode = "default" }
}

if (-not $modeRules.ContainsKey($Tool)) {
    throw "Unknown verification tool: $Tool"
}
$rule = $modeRules[$Tool]
if ([string]::IsNullOrWhiteSpace($Mode)) {
    if (-not $rule.AllowNoMode) {
        throw "Usage: $Tool.cmd <" + ($rule.Modes -join "|") + ">"
    }
    $Mode = $rule.DefaultMode
} elseif ($Mode -ne $rule.DefaultMode -and $rule.Modes -notcontains $Mode) {
    throw "Usage: $Tool.cmd [" + ($rule.Modes -join "|") + "]"
}

$repositoryRoot = Get-FullPath (Join-Path $PSScriptRoot "..\..\..")
if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot "go.mod") -PathType Leaf)) {
    throw "Cannot locate the repository root."
}

$toolModule = "devtools/general-verification-tools/$Tool"

$runId = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssfffffffZ", [Globalization.CultureInfo]::InvariantCulture)
$runRoot = Join-Path $repositoryRoot ".dev-workspace\.dev-tools-runtime\$Tool\run-$runId"
if (Test-Path -LiteralPath $runRoot) {
    throw "Verification run already exists: $runRoot"
}

$verificationCacheRoot = Join-Path $repositoryRoot ".dev-workspace\.dev-tools-runtime\cache"

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

    Write-Host "$Tool $Mode verification root: $runRoot"
    $workDir = Join-Path $runRoot "work"
    [System.IO.Directory]::CreateDirectory($workDir) | Out-Null
    Push-Location $workDir
    try {
        $arguments = @("run", $toolModule, "-root", $repositoryRoot, "-run-root", $runRoot)
        if ($rule.AllowNoMode -or $rule.Modes.Count -gt 0) {
            $arguments += @("-mode", $Mode)
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
            -Tool $Tool `
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
    Write-Host "[FAIL] $Tool $Mode verification failed. Evidence retained at $runRoot"
    exit $verificationExitCode
}
if (-not $finalizationSucceeded) {
    Write-Host "[FAIL] $Tool $Mode finalization failed. Remaining evidence retained at $runRoot"
    exit 1
}
if ($manualCleanupRequired) {
    Write-Host "[PASS] $Tool $Mode verification checks passed. Manual cleanup required: $runRoot"
    Write-Host "Report: $runRoot\evidence\report.json"
    exit 0
}

Write-Host "[PASS] $Tool $Mode verification passed. Report: $runRoot\evidence\report.json"
exit 0
