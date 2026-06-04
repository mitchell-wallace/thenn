#Requires -Version 5.1

$ErrorActionPreference = "Stop"

$Repo = "mitchell-wallace/thenn"
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\thenn"

# Determine architecture
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default {
        Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

# Fetch latest release
$LatestUrl = "https://api.github.com/repos/$Repo/releases/latest"
$Release = Invoke-RestMethod -Uri $LatestUrl -UseBasicParsing
$Tag = $Release.tag_name

if (-not $Tag) {
    Write-Error "Failed to fetch latest release tag"
    exit 1
}

$Version = $Tag -replace '^v',''
$Asset = "thenn_${Version}_windows_${Arch}.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/$Asset"
$TempFile = Join-Path $env:TEMP $Asset

Write-Host "Downloading $Asset..."
Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile -UseBasicParsing

# Create install directory
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# Extract
Expand-Archive -Path $TempFile -DestinationPath $InstallDir -Force
Remove-Item -Path $TempFile -Force

Write-Host "Installed thenn.exe to $InstallDir"

# Update user PATH
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    $NewPath = "$UserPath;$InstallDir"
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    Write-Host "Added $InstallDir to user PATH"
} else {
    Write-Host "$InstallDir already in user PATH"
}

Write-Host "Installation complete. Restart your terminal to use thenn."
