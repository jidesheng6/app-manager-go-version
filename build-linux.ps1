# Build script for app-manager (Linux binaries)
# Injects git info (commit/branch/describe/dirty) into the binary so that
# 'app-manager status' and 'app-manager version' can show which revision is deployed.

param(
    [string]$Version = "",
    # NOTE: keep this free of spaces - the value is passed through go -ldflags,
    # where whitespace separates tokens, so "a b" breaks the linker flags.
    [string]$Description = "app-manager",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

# -ldflags is whitespace separated: any space inside a value would split it into
# extra (bogus) linker flags. Guard instead of producing a confusing link error.
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

Write-Host "Building app-manager for Linux..." -ForegroundColor Green

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

# ---- collect git info ----
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

# NOTE: no spaces - this value ends up inside go -ldflags
$BuildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ss"
$Author    = "DengGuoWei-P7"

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

$env:GOOS = "linux"
$env:CGO_ENABLED = "0"

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
) -join " " | ForEach-Object {
    # 最终兜底：任何仍带空格的取值都会把 ldflags 拆坏，直接拦下
    $flagTokens = $_ -split ' '
    for ($i = 0; $i -lt $flagTokens.Count; $i++) {
        if ($flagTokens[$i] -eq "-X" -and ($i + 1) -lt $flagTokens.Count) {
            if ($flagTokens[$i + 1] -notmatch '=') {
                throw "ldflags 被空格拆坏了: '$($flagTokens[$i + 1])'（检查 Version/Description/BuildTime 是否含空格）"
            }
        }
    }
    $_
}

# name => GOARCH（产物名用 aarch64 便于和运维习惯对齐）
$Targets = @(
    @{ Name = "amd64";    Arch = "amd64" },
    @{ Name = "aarch64";  Arch = "arm64" },
    @{ Name = "mips64le"; Arch = "mips64le" },
    @{ Name = "mips64";   Arch = "mips64" }
)

$built = @()
foreach ($t in $Targets) {
    $out = Join-Path $OutputDir ("app-manager-linux-" + $t.Name)
    Write-Host ("Building for linux/{0} -> {1}..." -f $t.Arch, $out) -ForegroundColor Yellow
    $env:GOARCH = $t.Arch
    go build -ldflags $LdFlags -o $out .
    if ($LASTEXITCODE -ne 0) {
        Write-Host ("[ERROR] Linux {0} build failed" -f $t.Arch) -ForegroundColor Red
        exit 1
    }
    Write-Host ("[OK] Linux {0}" -f $t.Name) -ForegroundColor Green
    $built += $out
}

$InfoOut = Join-Path $OutputDir "BUILD-INFO.txt"
$Sw64Out = Join-Path $OutputDir "SW64.txt"
$buildInfo = @(
    "version=$Version",
    "git_commit=$GitCommit",
    "git_branch=$GitBranch",
    "git_describe=$GitDescribe",
    "git_dirty=$GitDirty",
    "build_time=$BuildTime",
    "arches=amd64,aarch64,mips64le,mips64",
    "sw64=unsupported_by_official_go_use_shenwei_toolchain"
) -join "`n"
[System.IO.File]::WriteAllText($InfoOut, $buildInfo, (New-Object System.Text.UTF8Encoding($false)))

$sw64Note = @"
SW64 / 申威
===========
官方 golang.org 工具链不提供 GOARCH=sw64，本脚本无法交叉编译申威二进制。
请在申威机器上使用厂商 Go 工具链本地编译：

  export CGO_ENABLED=0
  go build -o app-manager-linux-sw64 .

详见 README「申威 SW64」。
"@
[System.IO.File]::WriteAllText($Sw64Out, $sw64Note, (New-Object System.Text.UTF8Encoding($false)))

Write-Host ""
Write-Host "All Linux builds completed successfully!" -ForegroundColor Green
Write-Host "Output files:"
foreach ($f in $built) { Write-Host "  - $f" }
Write-Host "  - $InfoOut"
Write-Host "  - $Sw64Out"
Write-Host ""
Write-Host "After deploy, run 'app-manager status' or 'app-manager version' to see this commit." -ForegroundColor Cyan
