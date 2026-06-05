# build-windows.ps1 — Windows packaging stub for Sirsi Pantheon
# Future: MSIX or WiX installer
# Current: Copies built binaries into a versioned directory and creates a zip
#
# Usage: .\scripts\build-windows.ps1 [-Version "0.17.0"] [-Arch "amd64"]

param(
    [string]$Version = "0.17.0",
    [string]$Arch = "amd64"
)

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$BuildDir = Join-Path $ProjectRoot "bin"
$PackageName = "SirsiPantheon-${Version}-windows-${Arch}"
$PackageDir = Join-Path $BuildDir $PackageName
$ZipPath = Join-Path $BuildDir "${PackageName}.zip"

Write-Host "Building Sirsi Pantheon for Windows"
Write-Host "  Version: $Version"
Write-Host "  Arch:    $Arch"
Write-Host "  Output:  $ZipPath"
Write-Host ""

# --- Build binaries ---
Write-Host "Compiling sirsi CLI..."
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = $Arch
$LdFlags = "-s -w -X github.com/SirsiMaster/sirsi-pantheon/internal/version.Version=v${Version}"

go build -ldflags="$LdFlags" -o (Join-Path $BuildDir "sirsi.exe") ./cmd/sirsi/
if ($LASTEXITCODE -ne 0) { throw "Failed to build sirsi" }

Write-Host "Compiling sirsi-agent..."
go build -ldflags="$LdFlags" -o (Join-Path $BuildDir "sirsi-agent.exe") ./cmd/sirsi-agent/
if ($LASTEXITCODE -ne 0) { throw "Failed to build sirsi-agent" }

# --- Package ---
Write-Host "Creating package directory..."
if (Test-Path $PackageDir) { Remove-Item -Recurse -Force $PackageDir }
New-Item -ItemType Directory -Path $PackageDir | Out-Null

Copy-Item (Join-Path $BuildDir "sirsi.exe") $PackageDir
Copy-Item (Join-Path $BuildDir "sirsi-agent.exe") $PackageDir

# Include docs if available
$License = Join-Path $ProjectRoot "LICENSE"
if (Test-Path $License) { Copy-Item $License $PackageDir }

$Readme = Join-Path $ProjectRoot "README.md"
if (Test-Path $Readme) { Copy-Item $Readme $PackageDir }

# --- Create zip ---
Write-Host "Creating zip archive..."
if (Test-Path $ZipPath) { Remove-Item -Force $ZipPath }
Compress-Archive -Path $PackageDir -DestinationPath $ZipPath

Write-Host ""
Write-Host "Package created: $ZipPath"

# --- Future Plans ---
# TODO: MSIX installer for Windows Store distribution
#   - Requires Windows SDK and MakeAppx.exe
#   - AppxManifest.xml with Sirsi Technologies identity
#   - Code signing with Authenticode certificate
#
# TODO: WiX installer (.msi) for enterprise deployment
#   - WiX Toolset v4+
#   - Per-machine install to Program Files
#   - PATH registration
#   - Uninstall support via Add/Remove Programs
#
# TODO: Winget manifest for community distribution
#   - Submit to microsoft/winget-pkgs
#   - Automatic updates via winget upgrade
