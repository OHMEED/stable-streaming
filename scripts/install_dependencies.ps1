#Requires -RunAsAdministrator
# Windows Dependency Installer for Stable Stream Solutions
# Run this script in PowerShell as Administrator
# NOTE: This script uses DIRECT DOWNLOADS - no winget required!

$ErrorActionPreference = "Continue"

function Test-Command {
    param([string]$Command)
    try {
        Get-Command $Command -ErrorAction Stop | Out-Null
        return $true
    } catch {
        return $false
    }
}

function Refresh-Path {
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}

function Download-File {
    param([string]$Url, [string]$OutFile)
    Write-Host "  Downloading from $Url..." -ForegroundColor Gray
    try {
        # Try TLS 1.2 for older systems
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
        return $true
    } catch {
        Write-Host "  ERROR: Download failed: $_" -ForegroundColor Red
        return $false
    }
}

Write-Host "========================================" -ForegroundColor Green
Write-Host "Stable Stream Solutions - Windows Setup" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "This script will install dependencies using DIRECT DOWNLOADS." -ForegroundColor Cyan
Write-Host ""

# ============================================
# 1. Install Git
# ============================================
Write-Host "[1/7] Checking Git..." -ForegroundColor Cyan
if (-not (Test-Command "git")) {
    Write-Host "Installing Git..." -ForegroundColor Yellow
    $gitUrl = "https://github.com/git-for-windows/git/releases/download/v2.43.0.windows.1/Git-2.43.0-64-bit.exe"
    $gitInstaller = "$env:TEMP\git-installer.exe"
    
    if (Download-File $gitUrl $gitInstaller) {
        Write-Host "  Running Git installer (silent)..." -ForegroundColor Gray
        Start-Process -FilePath $gitInstaller -ArgumentList "/VERYSILENT /NORESTART /NOCANCEL /SP- /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS" -Wait
        Remove-Item $gitInstaller -Force -ErrorAction SilentlyContinue
        Refresh-Path
        Write-Host "  Git installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install Git manually from https://git-scm.com/download/win" -ForegroundColor Yellow
    }
} else {
    Write-Host "  Git already installed." -ForegroundColor Green
}

# ============================================
# 2. Install Go (1.23.x - latest stable)
# ============================================
Write-Host "[2/7] Checking Go..." -ForegroundColor Cyan
if (-not (Test-Command "go")) {
    Write-Host "Installing Go..." -ForegroundColor Yellow
    $goUrl = "https://go.dev/dl/go1.23.4.windows-amd64.msi"
    $goInstaller = "$env:TEMP\go-installer.msi"
    
    if (Download-File $goUrl $goInstaller) {
        Write-Host "  Running Go installer (silent)..." -ForegroundColor Gray
        Start-Process msiexec.exe -ArgumentList "/i `"$goInstaller`" /quiet /norestart" -Wait
        Remove-Item $goInstaller -Force -ErrorAction SilentlyContinue
        
        # Add Go to PATH manually if not there
        $goPath = "C:\Program Files\Go\bin"
        if (Test-Path $goPath) {
            $currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
            if ($currentPath -notlike "*$goPath*") {
                [Environment]::SetEnvironmentVariable("Path", "$currentPath;$goPath", "Machine")
            }
        }
        Refresh-Path
        Write-Host "  Go installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install Go manually from https://go.dev/dl/" -ForegroundColor Yellow
    }
} else {
    $goVer = go version
    Write-Host "  $goVer" -ForegroundColor Green
}

# ============================================
# 3. Install Node.js (LTS v20)
# ============================================
Write-Host "[3/7] Checking Node.js..." -ForegroundColor Cyan
if (-not (Test-Command "node")) {
    Write-Host "Installing Node.js LTS..." -ForegroundColor Yellow
    $nodeUrl = "https://nodejs.org/dist/v20.10.0/node-v20.10.0-x64.msi"
    $nodeInstaller = "$env:TEMP\node-installer.msi"
    
    if (Download-File $nodeUrl $nodeInstaller) {
        Write-Host "  Running Node.js installer (silent)..." -ForegroundColor Gray
        Start-Process msiexec.exe -ArgumentList "/i `"$nodeInstaller`" /quiet /norestart" -Wait
        Remove-Item $nodeInstaller -Force -ErrorAction SilentlyContinue
        Refresh-Path
        Write-Host "  Node.js installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install Node.js manually from https://nodejs.org/" -ForegroundColor Yellow
    }
} else {
    $nodeVer = node --version
    Write-Host "  Node.js $nodeVer" -ForegroundColor Green
}

# ============================================
# 4. Install Python (for websockify)
# ============================================
Write-Host "[4/7] Checking Python..." -ForegroundColor Cyan
if (-not (Test-Command "python")) {
    Write-Host "Installing Python 3.12..." -ForegroundColor Yellow
    $pythonUrl = "https://www.python.org/ftp/python/3.12.1/python-3.12.1-amd64.exe"
    $pythonInstaller = "$env:TEMP\python-installer.exe"
    
    if (Download-File $pythonUrl $pythonInstaller) {
        Write-Host "  Running Python installer (silent)..." -ForegroundColor Gray
        # InstallAllUsers=1 adds to PATH for all users
        Start-Process -FilePath $pythonInstaller -ArgumentList "/quiet InstallAllUsers=1 PrependPath=1 Include_test=0" -Wait
        Remove-Item $pythonInstaller -Force -ErrorAction SilentlyContinue
        Refresh-Path
        Write-Host "  Python installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install Python manually from https://www.python.org/downloads/" -ForegroundColor Yellow
    }
} else {
    $pyVer = python --version
    Write-Host "  $pyVer" -ForegroundColor Green
}

# Install websockify via pip
Write-Host "  Installing websockify..." -ForegroundColor Gray
if (Test-Command "python") {
    python -m pip install --upgrade pip --quiet 2>$null
    python -m pip install websockify --quiet 2>$null
    Write-Host "  websockify installed." -ForegroundColor Green
}

# ============================================
# 5. Install OBS Studio
# ============================================
Write-Host "[5/7] Checking OBS Studio..." -ForegroundColor Cyan
$obsPath = "C:\Program Files\obs-studio\bin\64bit\obs64.exe"
if (-not (Test-Path $obsPath)) {
    Write-Host "Installing OBS Studio..." -ForegroundColor Yellow
    $obsUrl = "https://cdn-fastly.obsproject.com/downloads/OBS-Studio-30.0.2-Full-Installer-x64.exe"
    $obsInstaller = "$env:TEMP\obs-installer.exe"
    
    if (Download-File $obsUrl $obsInstaller) {
        Write-Host "  Running OBS installer (silent)..." -ForegroundColor Gray
        Start-Process -FilePath $obsInstaller -ArgumentList "/S" -Wait
        Remove-Item $obsInstaller -Force -ErrorAction SilentlyContinue
        Write-Host "  OBS Studio installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install OBS manually from https://obsproject.com/download" -ForegroundColor Yellow
    }
} else {
    Write-Host "  OBS Studio already installed." -ForegroundColor Green
}

# ============================================
# 6. Install TightVNC Server
# ============================================
Write-Host "[6/7] Checking TightVNC..." -ForegroundColor Cyan
$tightvncPath = "C:\Program Files\TightVNC\tvnserver.exe"
if (-not (Test-Path $tightvncPath)) {
    Write-Host "Installing TightVNC Server..." -ForegroundColor Yellow
    $tightvncUrl = "https://www.tightvnc.com/download/2.8.85/tightvnc-2.8.85-gpl-setup-64bit.msi"
    $tightvncInstaller = "$env:TEMP\tightvnc-setup.msi"
    
    if (Download-File $tightvncUrl $tightvncInstaller) {
        Write-Host "  Running TightVNC installer (silent)..." -ForegroundColor Gray
        # Silent install with default password for development
        Start-Process msiexec.exe -ArgumentList "/i `"$tightvncInstaller`" /quiet /norestart SET_USEVNCAUTHENTICATION=1 VALUE_OF_USEVNCAUTHENTICATION=1 SET_PASSWORD=1 VALUE_OF_PASSWORD=password SET_USECONTROLAUTHENTICATION=1 VALUE_OF_USECONTROLAUTHENTICATION=1 SET_CONTROLPASSWORD=1 VALUE_OF_CONTROLPASSWORD=password" -Wait
        Remove-Item $tightvncInstaller -Force -ErrorAction SilentlyContinue
        Write-Host "  TightVNC installed." -ForegroundColor Green
    } else {
        Write-Host "  Please install TightVNC manually from https://www.tightvnc.com/download.php" -ForegroundColor Yellow
    }
} else {
    Write-Host "  TightVNC already installed." -ForegroundColor Green
}

# ============================================
# 7. Download MediaMTX and STABLE-SRTLA
# ============================================
Write-Host "[7/7] Installing streaming services..." -ForegroundColor Cyan

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$downloadScript = Join-Path $scriptDir "download_services.ps1"

if (Test-Path $downloadScript) {
    & $downloadScript
} else {
    Write-Host "  WARNING: download_services.ps1 not found. Run it manually." -ForegroundColor Yellow
}

# ============================================
# Configure Windows Firewall
# ============================================
Write-Host ""
Write-Host "Configuring Windows Firewall rules..." -ForegroundColor Cyan

$firewallRules = @(
    @{Name="StableStream-WebUI"; Port=8080; Protocol="TCP"; Description="Stable Stream Web UI"},
    @{Name="StableStream-VNC"; Port=5900; Protocol="TCP"; Description="VNC Server"},
    @{Name="StableStream-noVNC"; Port=6080; Protocol="TCP"; Description="noVNC WebSocket"},
    @{Name="StableStream-OBSWebSocket"; Port=4455; Protocol="TCP"; Description="OBS WebSocket"}
)

foreach ($rule in $firewallRules) {
    $existing = Get-NetFirewallRule -DisplayName $rule.Name -ErrorAction SilentlyContinue
    if (-not $existing) {
        New-NetFirewallRule -DisplayName $rule.Name -Direction Inbound -Protocol $rule.Protocol -LocalPort $rule.Port -Action Allow -Description $rule.Description | Out-Null
        Write-Host "  Created firewall rule: $($rule.Name) (port $($rule.Port))" -ForegroundColor Green
    } else {
        Write-Host "  Firewall rule exists: $($rule.Name)" -ForegroundColor Gray
    }
}

# ============================================
# Summary
# ============================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "Installation Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Installed Components:" -ForegroundColor Cyan
Write-Host "  - Git: $(if (Test-Command 'git') { 'OK' } else { 'MISSING' })"
Write-Host "  - Go: $(if (Test-Command 'go') { 'OK' } else { 'MISSING' })"
Write-Host "  - Node.js: $(if (Test-Command 'node') { 'OK' } else { 'MISSING' })"
Write-Host "  - Python: $(if (Test-Command 'python') { 'OK' } else { 'MISSING' })"
Write-Host "  - OBS Studio: $(if (Test-Path $obsPath) { 'OK' } else { 'MISSING' })"
Write-Host "  - TightVNC: $(if (Test-Path $tightvncPath) { 'OK' } else { 'MISSING' })"
Write-Host ""

# Check for any missing components
$missing = @()
if (-not (Test-Command 'git')) { $missing += "Git" }
if (-not (Test-Command 'go')) { $missing += "Go" }
if (-not (Test-Command 'node')) { $missing += "Node.js" }
if (-not (Test-Command 'python')) { $missing += "Python" }
if (-not (Test-Path $obsPath)) { $missing += "OBS Studio" }
if (-not (Test-Path $tightvncPath)) { $missing += "TightVNC" }

if ($missing.Count -gt 0) {
    Write-Host "MISSING COMPONENTS: $($missing -join ', ')" -ForegroundColor Red
    Write-Host "You may need to:" -ForegroundColor Yellow
    Write-Host "  1. Close and reopen PowerShell to refresh PATH" -ForegroundColor Yellow
    Write-Host "  2. Install missing components manually" -ForegroundColor Yellow
    Write-Host ""
}

Write-Host "Next Steps:" -ForegroundColor Yellow
Write-Host "  1. CLOSE AND REOPEN PowerShell (to refresh PATH)" -ForegroundColor Yellow
Write-Host "  2. Run .\start.ps1 to build and start the server" -ForegroundColor White
Write-Host "  3. Open http://localhost:8080 in your browser" -ForegroundColor White
Write-Host "  4. Login with default credentials (admin/password)" -ForegroundColor White
Write-Host ""

Write-Host "SECURITY NOTE:" -ForegroundColor Red
Write-Host "  TightVNC was installed with default password 'password'." -ForegroundColor Yellow
Write-Host "  Change this in production via TightVNC configuration!" -ForegroundColor Yellow
