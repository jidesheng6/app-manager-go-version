# Build script for app-manager (Windows binary)
# Injects git info so 'app-manager status' / 'version' show which revision the binary came from.

param(
    [string]$Version = "",
    [string]$Description = "app-manager",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

Write-Host "Building app-manager for Windows..." -ForegroundColor Green

function Invoke-Git {
    param([string[]]$GitArgs)
    try {
        $out = & git @GitArgs 2>$null
        if ($LASTEXITCODE -ne 0) { return "" }
        return ($out | Out-String).Trim()
    } catch {
        return ""
    }
}

$GitCommit   = Invoke-Git @("rev-parse", "--short", "HEAD")
$GitBranch   = Invoke-Git @("rev-parse", "--abbrev-ref", "HEAD")
$GitDescribe = Invoke-Git @("describe", "--tags", "--always")
$GitStatus   = Invoke-Git @("status", "--porcelain")
$GitDirty    = if ($GitStatus -ne "") { "true" } else { "false" }

if ([string]::IsNullOrEmpty($GitCommit)) {
    Write-Host "[WARN] no git info (not a git repo or git missing); building without commit id" -ForegroundColor Yellow
    $GitCommit = "unknown"
    $GitBranch = "unknown"
    $GitDescribe = "unknown"
}

if ([string]::IsNullOrEmpty($Version)) {
    if (-not [string]::IsNullOrEmpty($GitDescribe) -and $GitDescribe -ne "unknown") {
        $Version = $GitDescribe
    } else {
        $Version = "dev-$GitCommit"
    }
}

# no spaces: this value ends up inside go -ldflags
$BuildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ss"
$Author    = "DengGuoWei-P7"

function Assert-NoSpace {
    param([string]$Name, [string]$Value)
    if ($Value -match '\s') {
        throw "$Name contains whitespace ('$Value') - go -ldflags splits on spaces, so values must not contain any."
    }
}

Assert-NoSpace "Version" $Version
Assert-NoSpace "Description" $Description
Assert-NoSpace "OutputDir" $OutputDir

# Guard against an empty -o target: 'go build -o ""' dumps the binary into
# ./<module-path> (e.g. ./app-manager-go-v1) which is easy to miss.
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    throw "OutputDir must not be empty (go build -o '' would dump the binary into ./app-manager-go-v1)"
}

Write-Host "  version:   $Version"
Write-Host "  git:       $GitBranch@$GitCommit (dirty=$GitDirty)"
Write-Host "  buildTime: $BuildTime"
Write-Host ""

# Run from the script's own directory so that both the go build target (".") and the
# output paths resolve against the repository, no matter where the script was invoked.
Set-Location -LiteralPath $PSScriptRoot

# Resolve OutputDir against the PowerShell location (not the process working directory).
# .NET APIs like [System.IO.File]::WriteAllText resolve relative paths against the process
# CWD, which is NOT necessarily where PowerShell is sitting (e.g. C:\WINDOWS\system32 when
# launched from Explorer) - that mismatch once wrote BUILD-INFO.txt to the wrong place.
$OutputDir = Join-Path $PWD.Path $OutputDir

if (!(Test-Path -LiteralPath $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

Write-Host "  outDir:    $OutputDir"

$LdFlags = @(
    "-s", "-w",
    "-X", "main.Version=$Version",
    "-X", "main.Date=$BuildTime",
    "-X", "main.Author=$Author",
    "-X", "main.Description=$Description",
    "-X", "main.GitCommit=$GitCommit",
    "-X", "main.GitBranch=$GitBranch",
    "-X", "main.GitDescribe=$GitDescribe",
    "-X", "main.GitDirty=$GitDirty",
    "-X", "app-manager-go-v1/internal/version.GitCommit=$GitCommit",
    "-X", "app-manager-go-v1/internal/version.GitBranch=$GitBranch",
    "-X", "app-manager-go-v1/internal/version.GitDescribe=$GitDescribe",
    "-X", "app-manager-go-v1/internal/version.GitDirty=$GitDirty",
    "-X", "app-manager-go-v1/internal/version.BuildTime=$BuildTime"
) -join " "

$ExeOut  = Join-Path $OutputDir "app-manager.exe"
$InfoOut = Join-Path $OutputDir "BUILD-INFO.txt"

go build -ldflags $LdFlags -o $ExeOut .
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Windows build failed" -ForegroundColor Red
    exit 1
}

$buildInfo = @(
    "version=$Version",
    "git_commit=$GitCommit",
    "git_branch=$GitBranch",
    "git_describe=$GitDescribe",
    "git_dirty=$GitDirty",
    "build_time=$BuildTime"
) -join "`n"
[System.IO.File]::WriteAllText($InfoOut, $buildInfo, (New-Object System.Text.UTF8Encoding($false)))

Write-Host "[OK] Windows build successful" -ForegroundColor Green
Write-Host "  - $ExeOut"
Write-Host "  - $InfoOut"
Write-Host ""
Write-Host "Run 'app-manager status' or 'app-manager version' to see this commit." -ForegroundColor Cyan
