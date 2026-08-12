Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$script:DisposableDirectories = @("inputs", "workspace", "environment", "work", "temp", "cache")
$script:AllowedRunEntries = @("evidence") + $script:DisposableDirectories
$script:ToolRuntimeRelative = ".dev-workspace\.dev-tools-runtime"
$script:ToolNameByStage = @{
    "01"  = "verify-release-build"
    "02"  = "verify-release-publish"
    "03"  = "verify-client-install"
    "04"  = "verify-tool-plugin-update"
    "dev" = "verify-dev-box"
}

function Get-NormalizedPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $pathRoot = [System.IO.Path]::GetPathRoot($fullPath)
    while ($fullPath.Length -gt $pathRoot.Length -and ($fullPath.EndsWith("\") -or $fullPath.EndsWith("/"))) {
        $fullPath = $fullPath.Substring(0, $fullPath.Length - 1)
    }
    return $fullPath
}

function Test-SamePath {
    param(
        [Parameter(Mandatory = $true)][string]$Left,
        [Parameter(Mandatory = $true)][string]$Right
    )

    return [string]::Equals(
        (Get-NormalizedPath -Path $Left),
        (Get-NormalizedPath -Path $Right),
        [StringComparison]::OrdinalIgnoreCase
    )
}

function Set-ObjectProperty {
    param(
        [Parameter(Mandatory = $true)][object]$Object,
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)]$Value
    )

    $Object | Add-Member -MemberType NoteProperty -Name $Name -Value $Value -Force
}

function Remove-ObjectProperty {
    param(
        [Parameter(Mandatory = $true)][object]$Object,
        [Parameter(Mandatory = $true)][string]$Name
    )

    if ($null -ne $Object.PSObject.Properties[$Name]) {
        [void]$Object.PSObject.Properties.Remove($Name)
    }
}

function Write-Report {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][object]$Report
    )

    $identifier = [Guid]::NewGuid().ToString("N")
    $temporaryPath = "$Path.$identifier.temporary"
    $backupPath = "$Path.$identifier.backup"
    $encoding = New-Object System.Text.UTF8Encoding($false)
    try {
        $payload = ($Report | ConvertTo-Json -Depth 20) + [Environment]::NewLine
        [System.IO.File]::WriteAllText($temporaryPath, $payload, $encoding)
        [System.IO.File]::Replace($temporaryPath, $Path, $backupPath)
    } finally {
        if ([System.IO.File]::Exists($temporaryPath)) {
            [System.IO.File]::Delete($temporaryPath)
        }
        if ([System.IO.File]::Exists($backupPath)) {
            [System.IO.File]::Delete($backupPath)
        }
    }
}

function Assert-Sequence {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Actual,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Expected,
        [Parameter(Mandatory = $true)][string]$Label
    )

    if ($Actual.Count -ne $Expected.Count) {
        throw "$Label does not match the cleanup contract."
    }
    for ($index = 0; $index -lt $Expected.Count; $index++) {
        if ([string]$Actual[$index] -ne $Expected[$index]) {
            throw "$Label does not match the cleanup contract."
        }
    }
}

function Assert-PlainDirectory {
    param([Parameter(Mandatory = $true)][string]$Path)

    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer) {
        throw "Expected a directory: $Path"
    }
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw [System.Security.SecurityException]::new("Directory must not be a reparse point: $Path")
    }
}

function Assert-PlainDirectoryChain {
    param([Parameter(Mandatory = $true)][string]$Path)

    $paths = New-Object System.Collections.Generic.List[string]
    $current = Get-NormalizedPath -Path $Path
    while ($true) {
        [void]$paths.Add($current)
        $parentValue = Split-Path -Parent $current
        if ([string]::IsNullOrWhiteSpace($parentValue)) {
            break
        }
        $parent = Get-NormalizedPath -Path $parentValue
        if (Test-SamePath -Left $parent -Right $current) {
            break
        }
        $current = $parent
    }
    for ($index = $paths.Count - 1; $index -ge 0; $index--) {
        Assert-PlainDirectory -Path $paths[$index]
    }
}

function Get-FileSystemInfosRecursive {
    param([Parameter(Mandatory = $true)][string]$Path)

    $directory = New-Object System.IO.DirectoryInfo($Path)
    foreach ($entry in $directory.GetFileSystemInfos()) {
        if (($entry.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw [System.Security.SecurityException]::new("Cleanup directory contains a reparse point: $($entry.FullName)")
        }
        $entry
        if (($entry.Attributes -band [System.IO.FileAttributes]::Directory) -ne 0) {
            Get-FileSystemInfosRecursive -Path $entry.FullName
        }
    }
}

function Assert-CleanupTreeHasNoReparsePoints {
    param([Parameter(Mandatory = $true)][string]$Path)

    Assert-PlainDirectory -Path $Path
    $longPath = "\\?\" + (Get-NormalizedPath -Path $Path)
    Get-FileSystemInfosRecursive -Path $longPath | Out-Null
}

function Remove-LongPathEntry {
    param([Parameter(Mandatory = $true)][string]$Path)

    $longPath = "\\?\" + (Get-NormalizedPath -Path $Path)
    if ([System.IO.File]::Exists($longPath)) {
        [System.IO.File]::Delete($longPath)
        return
    }
    if (-not [System.IO.Directory]::Exists($longPath)) {
        throw "Cleanup target does not exist: $Path"
    }
    Remove-LongPathTree -Path $longPath
}

function Remove-LongPathTree {
    param([Parameter(Mandatory = $true)][string]$Path)

    $directory = New-Object System.IO.DirectoryInfo($Path)
    if (($directory.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw [System.Security.SecurityException]::new("Cleanup directory contains a reparse point: $Path")
    }
    foreach ($entry in $directory.GetFileSystemInfos()) {
        if (($entry.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw [System.Security.SecurityException]::new("Cleanup directory contains a reparse point: $($entry.FullName)")
        }
        if (($entry.Attributes -band [System.IO.FileAttributes]::Directory) -ne 0) {
            Remove-LongPathTree -Path $entry.FullName
        } else {
            [System.IO.File]::Delete($entry.FullName)
        }
    }
    [System.IO.Directory]::Delete($Path)
}

function Get-CleanupState {
    param([Parameter(Mandatory = $true)][string]$RunRoot)

    $completed = New-Object System.Collections.Generic.List[string]
    $pending = New-Object System.Collections.Generic.List[string]
    foreach ($name in $script:DisposableDirectories) {
        $target = Get-NormalizedPath -Path (Join-Path $RunRoot $name)
        if (-not (Test-SamePath -Left (Split-Path -Parent $target) -Right $RunRoot)) {
            throw "Cleanup target is outside the verification run."
        }
        if ([System.IO.Directory]::Exists($target) -or [System.IO.File]::Exists($target)) {
            [void]$pending.Add($name)
        } else {
            [void]$completed.Add($name)
        }
    }
    return [PSCustomObject]@{
        Completed = [string[]]$completed.ToArray()
        Pending = [string[]]$pending.ToArray()
    }
}

function Set-FailedReport {
    param(
        [Parameter(Mandatory = $true)][object]$Report,
        [Parameter(Mandatory = $true)][string]$RunRoot,
        [Parameter(Mandatory = $true)][string]$Message
    )

    $state = Get-CleanupState -RunRoot $RunRoot
    $finishedAt = [DateTime]::UtcNow.ToString("o", [Globalization.CultureInfo]::InvariantCulture)
    $Report.status = "failed"
    Set-ObjectProperty -Object $Report -Name "finishedAt" -Value $finishedAt
    Set-ObjectProperty -Object $Report -Name "error" -Value $Message
    $Report.cleanup.status = "retained"
    $Report.cleanup.completedDirectories = [object[]]$state.Completed
    $Report.cleanup.pendingDirectories = [object[]]$state.Pending
    Set-ObjectProperty -Object $Report.cleanup -Name "finishedAt" -Value $finishedAt
    Set-ObjectProperty -Object $Report.cleanup -Name "error" -Value $Message
    Remove-ObjectProperty -Object $Report.cleanup -Name "message"
}

function Set-ManualCleanupReport {
    param(
        [Parameter(Mandatory = $true)][object]$Report,
        [Parameter(Mandatory = $true)][string]$RunRoot,
        [Parameter(Mandatory = $true)][string]$Message
    )

    $state = Get-CleanupState -RunRoot $RunRoot
    $finishedAt = [DateTime]::UtcNow.ToString("o", [Globalization.CultureInfo]::InvariantCulture)
    $Report.status = "passed"
    Set-ObjectProperty -Object $Report -Name "finishedAt" -Value $finishedAt
    Remove-ObjectProperty -Object $Report -Name "error"
    $Report.cleanup.status = "manual_required"
    $Report.cleanup.completedDirectories = [object[]]$state.Completed
    $Report.cleanup.pendingDirectories = [object[]]$state.Pending
    Set-ObjectProperty -Object $Report.cleanup -Name "finishedAt" -Value $finishedAt
    Set-ObjectProperty -Object $Report.cleanup -Name "message" -Value $Message
    Remove-ObjectProperty -Object $Report.cleanup -Name "error"
}

function Set-CleanupPassedReport {
    param(
        [Parameter(Mandatory = $true)][object]$Report,
        [Parameter(Mandatory = $true)][object]$State
    )

    $finishedAt = [DateTime]::UtcNow.ToString("o", [Globalization.CultureInfo]::InvariantCulture)
    $Report.status = "passed"
    Set-ObjectProperty -Object $Report -Name "finishedAt" -Value $finishedAt
    Remove-ObjectProperty -Object $Report -Name "error"
    $Report.cleanup.status = "passed"
    $Report.cleanup.completedDirectories = [object[]]$State.Completed
    $Report.cleanup.pendingDirectories = [object[]]$State.Pending
    Set-ObjectProperty -Object $Report.cleanup -Name "finishedAt" -Value $finishedAt
    Remove-ObjectProperty -Object $Report.cleanup -Name "error"
    Remove-ObjectProperty -Object $Report.cleanup -Name "message"
}

function Save-FailedFinalization {
    param(
        [Parameter(Mandatory = $true)][object]$Report,
        [Parameter(Mandatory = $true)][string]$RunRoot,
        [Parameter(Mandatory = $true)][string]$ReportPath,
        [Parameter(Mandatory = $true)][string]$Message
    )

    try {
        Set-FailedReport -Report $Report -RunRoot $RunRoot -Message $Message
        Write-Report -Path $ReportPath -Report $Report
    } catch {
        throw "Verification finalization failed and its report could not be updated: $Message; report error: $($_.Exception.Message)"
    }
}

function Get-CleanupTargets {
    param([Parameter(Mandatory = $true)][string]$RunRoot)

    foreach ($entry in Get-ChildItem -LiteralPath $RunRoot -Force) {
        if (($entry.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw [System.Security.SecurityException]::new("Verification run contains a reparse point: $($entry.FullName)")
        }
        if ($script:AllowedRunEntries -notcontains $entry.Name) {
            throw "Verification run contains an unexpected entry: $($entry.Name)"
        }
    }

    $targets = New-Object System.Collections.Generic.List[object]
    foreach ($name in $script:DisposableDirectories) {
        $target = Get-NormalizedPath -Path (Join-Path $RunRoot $name)
        if (-not (Test-SamePath -Left (Split-Path -Parent $target) -Right $RunRoot)) {
            throw "Cleanup target is outside the verification run."
        }
        Assert-PlainDirectory -Path $target
        [void]$targets.Add([PSCustomObject]@{ Name = $name; Path = $target })
    }
    return [object[]]$targets.ToArray()
}

function Complete-VerificationRun {
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryRoot,
        [Parameter(Mandatory = $true)][string]$RunRoot,
        [Parameter(Mandatory = $true)][ValidateSet("01", "02", "03", "04", "dev")][string]$Stage,
        [Parameter(Mandatory = $true)][string]$Mode
    )

    $repositoryRoot = Get-NormalizedPath -Path $RepositoryRoot
    $runRoot = Get-NormalizedPath -Path $RunRoot
    $toolName = $script:ToolNameByStage[$Stage]
    $expectedParent = Get-NormalizedPath -Path (Join-Path $repositoryRoot (Join-Path $script:ToolRuntimeRelative $toolName))
    $actualParent = Get-NormalizedPath -Path (Split-Path -Parent $runRoot)
    $runName = Split-Path -Leaf $runRoot

    if (-not (Test-SamePath -Left $expectedParent -Right $actualParent) -or $runName -notmatch '^run-[A-Za-z0-9][A-Za-z0-9._-]*$') {
        throw "Run root is outside the tool verification boundary."
    }
    if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot "go.mod") -PathType Leaf)) {
        throw "Repository root is invalid."
    }
    foreach ($path in @(
        $repositoryRoot,
        (Join-Path $repositoryRoot $script:ToolRuntimeRelative),
        $expectedParent,
        $runRoot,
        (Join-Path $runRoot "evidence")
    )) {
        Assert-PlainDirectoryChain -Path $path
    }

    $reportPath = Join-Path $runRoot "evidence\report.json"
    if (-not (Test-Path -LiteralPath $reportPath -PathType Leaf)) {
        throw "Verification report is missing: $reportPath"
    }
    $reportItem = Get-Item -LiteralPath $reportPath -Force
    if (($reportItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Verification report must not be a reparse point."
    }
    $report = Get-Content -LiteralPath $reportPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($report.stage -ne $Stage -or $report.mode -ne $Mode -or -not (Test-SamePath -Left $report.runRoot -Right $runRoot)) {
        throw "Verification report identity does not match the requested run."
    }
    if ($report.status -ne "cleanup_pending" -or $report.cleanup.status -ne "pending") {
        throw "Verification report is not waiting for cleanup."
    }
    Assert-Sequence -Actual @($report.cleanup.completedDirectories) -Expected @() -Label "Completed directories"
    Assert-Sequence -Actual @($report.cleanup.pendingDirectories) -Expected $script:DisposableDirectories -Label "Pending directories"

    try {
        $cleanupTargets = @(Get-CleanupTargets -RunRoot $runRoot)
    } catch {
        $message = $_.Exception.Message
        Save-FailedFinalization -Report $report -RunRoot $runRoot -ReportPath $reportPath -Message $message
        throw "Verification finalization failed: $message"
    }

    $cleanableTargets = New-Object System.Collections.Generic.List[object]
    $manualCleanupMessages = New-Object System.Collections.Generic.List[string]
    foreach ($target in $cleanupTargets) {
        try {
            Assert-CleanupTreeHasNoReparsePoints -Path $target.Path
            [void]$cleanableTargets.Add($target)
        } catch [System.Security.SecurityException] {
            $message = $_.Exception.Message
            Save-FailedFinalization -Report $report -RunRoot $runRoot -ReportPath $reportPath -Message $message
            throw "Verification safety failure: $message"
        } catch {
            [void]$manualCleanupMessages.Add("$($target.Name): $($_.Exception.Message)")
        }
    }

    $report.cleanup.status = "in_progress"
    Write-Report -Path $reportPath -Report $report

    foreach ($target in $cleanableTargets) {
        try {
            Assert-CleanupTreeHasNoReparsePoints -Path $target.Path
        } catch [System.Security.SecurityException] {
            $message = $_.Exception.Message
            Save-FailedFinalization -Report $report -RunRoot $runRoot -ReportPath $reportPath -Message $message
            throw "Verification safety failure: $message"
        } catch {
            [void]$manualCleanupMessages.Add("$($target.Name): $($_.Exception.Message)")
            continue
        }

        try {
            Remove-LongPathEntry -Path $target.Path
            if (Test-Path -LiteralPath $target.Path) {
                throw "Cleanup target still exists: $($target.Path)"
            }
        } catch {
            [void]$manualCleanupMessages.Add("$($target.Name): $($_.Exception.Message)")
        }
    }

    $state = Get-CleanupState -RunRoot $runRoot
    if ($manualCleanupMessages.Count -gt 0 -or $state.Pending.Count -gt 0) {
        if ($manualCleanupMessages.Count -eq 0) {
            [void]$manualCleanupMessages.Add("Automatic cleanup left disposable directories behind.")
        }
        $message = [string]::Join("; ", $manualCleanupMessages.ToArray())
        Set-ManualCleanupReport -Report $report -RunRoot $runRoot -Message $message
        Write-Report -Path $reportPath -Report $report
        Write-Host "[WARN] Verification checks passed, but manual cleanup is required: $runRoot"
        return
    }

    $remainingEntries = @(Get-ChildItem -LiteralPath $runRoot -Force | Where-Object { $_.Name -ne "evidence" })
    if ($remainingEntries.Count -ne 0) {
        $message = "Verification run contains unexpected entries after cleanup."
        Save-FailedFinalization -Report $report -RunRoot $runRoot -ReportPath $reportPath -Message $message
        throw "Verification finalization failed: $message"
    }
    Assert-Sequence -Actual @($state.Completed) -Expected $script:DisposableDirectories -Label "Completed directories"
    Assert-Sequence -Actual @($state.Pending) -Expected @() -Label "Pending directories"
    Set-CleanupPassedReport -Report $report -State $state
    Write-Report -Path $reportPath -Report $report
}

Export-ModuleMember -Function "Complete-VerificationRun"
