param(
    [Parameter(Mandatory = $true)]
    [string]$RepositoryRoot,

    [Parameter(Mandatory = $true)]
    [string]$RunRoot,

    [Parameter(Mandatory = $true)]
    [string]$Tool,

    [Parameter(Mandatory = $true)]
    [string]$Mode
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Import-Module (Join-Path $PSScriptRoot "verification-finalization.psm1") -Force -ErrorAction Stop
Complete-VerificationRun -RepositoryRoot $RepositoryRoot -RunRoot $RunRoot -Tool $Tool -Mode $Mode
