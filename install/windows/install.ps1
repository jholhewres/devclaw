# DevClaw installer — Windows PowerShell
# Usage: iwr -useb https://raw.githubusercontent.com/jholhewres/devclaw/master/install/windows/install.ps1 | iex
#
# Options:
#   -Name <name>    Service name and namespace (default: devclaw)
#                   Affects install dir (%LOCALAPPDATA%\<name>) and PM2 process name
#   -Port <port>    Server port (default: 47716)
#   -Version <tag>  Install specific version (default: latest)
#   -NoPrompt       Non-interactive mode
#
# Examples:
#   # Default install
#   .\install.ps1
#
#   # Second instance
#   .\install.ps1 -Name devclaw-2 -Port 47718
#
param(
    [string]$Name = "devclaw",
    [string]$Port = "47716",
    [string]$Version = "",
    [switch]$NoPrompt
)

$ErrorActionPreference = "Stop"

$Repo = "jholhewres/devclaw"
$Binary = "devclaw"
$ServiceName = $Name
$InstallDir = "$env:LOCALAPPDATA\$ServiceName"

function Info($msg) { Write-Host "[info]  $msg" -ForegroundColor Cyan }
function Ok($msg)   { Write-Host "[ok]    $msg" -ForegroundColor Green }
function Warn($msg) { Write-Host "[warn]  $msg" -ForegroundColor Yellow }
function Err($msg)  { Write-Host "[error] $msg" -ForegroundColor Red; exit 1 }

# Banner
Write-Host ""
Write-Host "  +======================================+" -ForegroundColor Cyan
Write-Host "  |   DevClaw — AI Agent for Tech Teams  |" -ForegroundColor Cyan
Write-Host "  +======================================+" -ForegroundColor Cyan
Write-Host ""

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { Err "32-bit Windows not supported" }

Info "Detected: windows/$Arch"
Info "Service name: $ServiceName"
Info "Install dir: $InstallDir"
Info "Port: $Port"

# Get version
if ($Version -eq "") {
    Info "Fetching latest version..."
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $Release.tag_name
    Info "Latest version: $Version"
} else {
    Info "Version: $Version (specified)"
}

# Confirm
if (-not $NoPrompt) {
    Write-Host ""
    $confirm = Read-Host "Continue with installation? [y/N]"
    if ($confirm -notmatch '^[Yy]$') {
        Info "Installation cancelled"
        exit 0
    }
}

# Download
$Archive = "${Binary}_$($Version.TrimStart('v'))_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Archive"
$TmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path "$_.dir" }
$TmpFile = Join-Path $TmpDir $Archive

Info "Downloading $Url..."
Invoke-WebRequest -Uri $Url -OutFile $TmpFile -UseBasicParsing

# Extract
Info "Extracting..."
Expand-Archive -Path $TmpFile -DestinationPath $TmpDir -Force

# Install
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Create subdirectories
foreach ($dir in @("data", "sessions", "logs")) {
    $dirPath = Join-Path $InstallDir $dir
    if (-not (Test-Path $dirPath)) {
        New-Item -ItemType Directory -Path $dirPath -Force | Out-Null
    }
}

Copy-Item -Path (Join-Path $TmpDir "$Binary.exe") -Destination (Join-Path $InstallDir "$Binary.exe") -Force
Ok "Installed to $InstallDir\$Binary.exe"

# Add to PATH
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Info "Added $InstallDir to user PATH"
    Info "Restart your terminal for PATH changes to take effect"
}

# PM2 setup (if Node.js and PM2 are available)
$hasPm2 = $false
try { $null = Get-Command pm2 -ErrorAction Stop; $hasPm2 = $true } catch {}

if ($hasPm2) {
    Info "PM2 detected, configuring..."

    # Stop existing process if running
    try { pm2 describe $ServiceName 2>$null | Out-Null; pm2 delete $ServiceName 2>$null | Out-Null } catch {}

    $ecosystemFile = Join-Path $InstallDir "ecosystem.config.js"
    $ecosystemContent = @"
const path = require('path');
const installDir = '$($InstallDir -replace '\\','\\\\')';
const port = '$Port';

module.exports = {
  apps: [{
    name: '$ServiceName',
    script: path.join(installDir, 'devclaw.exe'),
    args: 'serve',
    cwd: installDir,
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    kill_timeout: 5000,
    wait_ready: true,
    listen_timeout: 10000,
    time: true,
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    error_file: path.join(installDir, 'logs', 'error.log'),
    out_file: path.join(installDir, 'logs', 'out.log'),
    merge_logs: true,
    env: {
      NODE_ENV: 'production',
      DEVCLAW_STATE_DIR: installDir,
      PORT: port
    }
  }]
};
"@
    Set-Content -Path $ecosystemFile -Value $ecosystemContent -Encoding UTF8
    Ok "PM2 config generated"

    Info "Starting $ServiceName with PM2..."
    Push-Location $InstallDir
    pm2 start ecosystem.config.js
    pm2 save
    Pop-Location
    Ok "$ServiceName started with PM2"
} else {
    Warn "PM2 not found — skipping service setup"
    Info "To run manually: $Binary serve"
    Info "To install PM2: npm install -g pm2"
}

# Cleanup
Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue

# Success
Write-Host ""
Ok "$ServiceName installed successfully!"
Write-Host ""
Write-Host "  Installation:"
Write-Host "    Service:    $ServiceName"
Write-Host "    Binary:     $InstallDir\$Binary.exe"
Write-Host "    Config:     $InstallDir\config.yaml"
Write-Host "    Data:       $InstallDir\data\"
Write-Host "    Logs:       $InstallDir\logs\"
Write-Host ""
Write-Host "  Commands:"
Write-Host "    $Binary --version              # Check version"
Write-Host "    $Binary serve                  # Start server (foreground)"
if ($hasPm2) {
    Write-Host "    pm2 status                     # Check PM2 status"
    Write-Host "    pm2 logs $ServiceName               # View logs"
    Write-Host "    pm2 restart $ServiceName            # Restart service"
    Write-Host "    pm2 stop $ServiceName               # Stop service"
}
Write-Host ""
Write-Host "  Web UI:"
Write-Host "    http://0.0.0.0:$Port"
Write-Host "    http://0.0.0.0:$Port/setup"
Write-Host ""
