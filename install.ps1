# ==============================================================================
# Fortinet Auto-Login Universal Installer for Windows (PowerShell)
# Usage:
#   irm https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.ps1 | iex
# ==============================================================================

$ErrorActionPreference = "Stop"

$repo = "c2-07/fortinet-auto-login"
$installDir = "$HOME\.fortinet-autologin\bin"
$binName = "autologin.exe"
$targetPath = Join-Path $installDir $binName

Write-Host @"
    ____           __  _            __ 
   / __/___  _____/ /_(_)___  ___  / /_
  / /_/ __ \/ ___/ __/ / __ \/ _ \/ __/
 / __/ /_/ / /  / /_/ / / / /  __/ /_  
/_/  \____/_/   \__/_/_/ /_/\___/\__/  
      Captive Portal Auto-Login
"@ -ForegroundColor Cyan

Write-Host "[INFO] Detecting system architecture..." -ForegroundColor Cyan

# 1. Architecture Detection
$arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        "arm64"
    } else {
        "amd64"
    }
} else {
    Write-Error "[FAIL] 32-bit Windows is not supported. Please use a 64-bit OS."
    exit 1
}

$assetName = "fortinet-auto-login-windows-$arch.exe"
Write-Host "[OK]   Target detected: Windows/$arch ($assetName)" -ForegroundColor Green

# 2. Ensure install directory exists
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# 3. Download Release Asset
$downloadUrl = "https://github.com/$repo/releases/latest/download/$assetName"
$tempFile = [System.IO.Path]::GetTempFileName()

Write-Host "[INFO] Downloading latest release from GitHub ($downloadUrl)..." -ForegroundColor Cyan

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing
} catch {
    Write-Error "[FAIL] Failed to download release asset from $downloadUrl. Error: $_"
    if (Test-Path $tempFile) { Remove-Item -Force $tempFile }
    exit 1
}

if ((Get-Item $tempFile).Length -lt 1024) {
    Write-Error "[FAIL] Downloaded file is too small or corrupt."
    if (Test-Path $tempFile) { Remove-Item -Force $tempFile }
    exit 1
}

# 4. Safe replace existing binary
if (Test-Path $targetPath) {
    $oldPath = "$targetPath.old"
    if (Test-Path $oldPath) {
        Remove-Item -Force $oldPath -ErrorAction SilentlyContinue
    }
    try {
        Move-Item -Path $targetPath -Destination $oldPath -Force
    } catch {
        # Process might be holding lock; notify user
        Write-Warning "[WARN] Existing binary may be running. Attempting overwrite..."
    }
}

Move-Item -Path $tempFile -Destination $targetPath -Force
Write-Host "[OK]   Binary installed to: $targetPath" -ForegroundColor Green

# 5. Permanently add to Windows User PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathList = if ($userPath) { $userPath -split ';' | Where-Object { $_ -ne "" } } else { @() }

if ($pathList -notcontains $installDir) {
    $newPath = ($pathList + $installDir) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "[OK]   Added $installDir to persistent Windows User PATH." -ForegroundColor Green
} else {
    Write-Host "[OK]   $installDir is already in Windows User PATH." -ForegroundColor Green
}

# Update current session environment PATH
if (($env:Path -split ';') -notcontains $installDir) {
    $env:Path = "$env:Path;$installDir"
}

Write-Host "`nInstallation complete!" -ForegroundColor Green
Write-Host "[INFO] Run: autologin to log in to your Fortinet captive portal." -ForegroundColor Cyan
Write-Host "[INFO] Run: autologin -h to view available commands." -ForegroundColor Cyan
Write-Host "[INFO] To install background service, run: autologin -service install" -ForegroundColor Cyan
