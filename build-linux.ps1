# Build script for app-manager Linux version
# 构建 app-manager 的 Linux 版本

param(
    [string]$Version = "1.0.0",
    [string]$OutputDir = "dist"
)

Write-Host "Building app-manager for Linux..." -ForegroundColor Green

# 设置环境变量
$env:GOOS = "linux"
$env:CGO_ENABLED = "0"

# 创建输出目录
if (!(Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force
}

# 构建信息
$BuildTime = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
$GitCommit = try { git rev-parse --short HEAD 2>$null } catch { "unknown" }

# 构建 amd64 版本
Write-Host "Building for linux/amd64..." -ForegroundColor Yellow
$env:GOARCH = "amd64"
$LdFlags = "-s -w -X 'main.Version=$Version' -X 'main.BuildTime=$BuildTime' -X 'main.GitCommit=$GitCommit'"

go build -ldflags $LdFlags -o "$OutputDir/app-manager-linux-amd64" .

if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] Linux amd64 build successful" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Linux amd64 build failed" -ForegroundColor Red
    exit 1
}

# 构建 arm64 版本
Write-Host "Building for linux/arm64..." -ForegroundColor Yellow
$env:GOARCH = "arm64"

go build -ldflags $LdFlags -o "$OutputDir/app-manager-linux-arm64" .

if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] Linux arm64 build successful" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Linux arm64 build failed" -ForegroundColor Red
    exit 1
}

Write-Host "All Linux builds completed successfully!" -ForegroundColor Green
Write-Host "Output files:" -ForegroundColor Cyan
Write-Host "  - $OutputDir/app-manager-linux-amd64" -ForegroundColor White
Write-Host "  - $OutputDir/app-manager-linux-arm64" -ForegroundColor White