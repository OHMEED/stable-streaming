# Windows Service Downloader for Stable Stream Solutions
# Downloads MediaMTX and builds go-irl

$ErrorActionPreference = "Continue"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$binDir = Join-Path $projectRoot "bin"

# Ensure bin directory exists
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}

# Ensure TLS 1.2 for downloads
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

Write-Host "========================================" -ForegroundColor Green
Write-Host "Downloading Streaming Services" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# ============================================
# 1. Download MediaMTX
# ============================================
$MMTX_VER = "v1.9.3"
$MMTX_URL = "https://github.com/bluenviron/mediamtx/releases/download/$MMTX_VER/mediamtx_${MMTX_VER}_windows_amd64.zip"
$mmtxZip = Join-Path $binDir "mediamtx.zip"
$mmtxExe = Join-Path $binDir "mediamtx.exe"

if (-not (Test-Path $mmtxExe)) {
    Write-Host "Downloading MediaMTX $MMTX_VER..." -ForegroundColor Cyan
    
    try {
        Invoke-WebRequest -Uri $MMTX_URL -OutFile $mmtxZip -UseBasicParsing
        
        Write-Host "Extracting MediaMTX..." -ForegroundColor Cyan
        Expand-Archive -Path $mmtxZip -DestinationPath $binDir -Force
        Remove-Item $mmtxZip -Force
        
        if (Test-Path $mmtxExe) {
            Write-Host "MediaMTX installed to $mmtxExe" -ForegroundColor Green
        } else {
            Write-Host "ERROR: MediaMTX extraction failed." -ForegroundColor Red
        }
    } catch {
        Write-Host "ERROR: Failed to download MediaMTX: $_" -ForegroundColor Red
    }
} else {
    Write-Host "MediaMTX already exists at $mmtxExe" -ForegroundColor Green
}

# ============================================
# 2. Build STABLE-SRTLA
# ============================================
$srtlaExe = Join-Path $binDir "go-irl.exe"

Write-Host ""
Write-Host "Building go-irl..." -ForegroundColor Cyan

# Check if git is available
$gitAvailable = $false
try {
    $gitVersion = git --version 2>&1
    if ($LASTEXITCODE -eq 0) {
        $gitAvailable = $true
    }
} catch {
    $gitAvailable = $false
}

# Check if go is available
$goAvailable = $false
try {
    $goVersion = go version 2>&1
    if ($LASTEXITCODE -eq 0) {
        $goAvailable = $true
    }
} catch {
    $goAvailable = $false
}

if (-not $gitAvailable) {
    Write-Host "WARNING: Git is not available in PATH." -ForegroundColor Yellow
    Write-Host "  You may need to close and reopen PowerShell after installing Git." -ForegroundColor Yellow
    Write-Host "  Or install Git manually from https://git-scm.com/download/win" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "SKIPPING go-irl build - will use fallback method..." -ForegroundColor Yellow
    
    Write-Host "Attempting to download go-irl source as zip..." -ForegroundColor Cyan
    $srtlaZipUrl = "https://github.com/e04/go-irl/archive/refs/heads/main.zip"
    $srtlaZip = Join-Path $env:TEMP "go-irl-main.zip"
    $srtlaExtract = Join-Path $env:TEMP "go-irl-extract"
    
    try {
        Invoke-WebRequest -Uri $srtlaZipUrl -OutFile $srtlaZip -UseBasicParsing
        
        # Clean up old extract
        if (Test-Path $srtlaExtract) {
            Remove-Item $srtlaExtract -Recurse -Force
        }
        
        Expand-Archive -Path $srtlaZip -DestinationPath $srtlaExtract -Force
        Remove-Item $srtlaZip -Force
        
        # Find the extracted folder
        $srtlaDir = Get-ChildItem $srtlaExtract -Directory | Select-Object -First 1
        
        if ($srtlaDir -and $goAvailable) {
            Write-Host "Compiling go-irl..." -ForegroundColor Cyan
            Push-Location $srtlaDir.FullName
            
            $env:CGO_ENABLED = "0"
            $buildResult = & go build -o $srtlaExe . 2>&1
            
            Pop-Location
            
            if (Test-Path $srtlaExe) {
                Write-Host "go-irl installed to $srtlaExe" -ForegroundColor Green
            } else {
                Write-Host "ERROR: go-irl build failed." -ForegroundColor Red
                Write-Host $buildResult -ForegroundColor Red
            }
        } elseif (-not $goAvailable) {
            Write-Host "ERROR: Go is not available. Cannot build go-irl." -ForegroundColor Red
            Write-Host "  Close and reopen PowerShell, or install Go manually." -ForegroundColor Yellow
        }
        
        # Cleanup
        Remove-Item $srtlaExtract -Recurse -Force -ErrorAction SilentlyContinue
        
    } catch {
        Write-Host "ERROR: Failed to download go-irl source: $_" -ForegroundColor Red
    }
    
    Write-Host "ERROR: Go is not installed. Cannot build go-irl." -ForegroundColor Red
    Write-Host "Please run install_dependencies.ps1 first, then close and reopen PowerShell." -ForegroundColor Yellow
} else {
    # Both git and go are available - use normal clone method
    $tempDir = Join-Path $env:TEMP "go-irl-build"
    
    try {
        # Clone repository
        if (Test-Path $tempDir) {
            Remove-Item $tempDir -Recurse -Force
        }
        
        Write-Host "Cloning go-irl repository..." -ForegroundColor Cyan
        $cloneResult = & git clone https://github.com/e04/go-irl.git $tempDir 2>&1
        
        if (-not (Test-Path $tempDir)) {
            throw "Clone failed: $cloneResult"
        }
        
        # Build
        Write-Host "Compiling go-irl..." -ForegroundColor Cyan
        Push-Location $tempDir
        
        $env:CGO_ENABLED = "0"
        $buildOutput = & go build -o $srtlaExe . 2>&1
        
        Pop-Location
        
        if (Test-Path $srtlaExe) {
            Write-Host "go-irl installed to $srtlaExe" -ForegroundColor Green
        } else {
            Write-Host "ERROR: go-irl build failed:" -ForegroundColor Red
            Write-Host $buildOutput -ForegroundColor Red
        }
        
        # Cleanup
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        
    } catch {
        Write-Host "ERROR: Failed to build go-irl: $_" -ForegroundColor Red
        Pop-Location -ErrorAction SilentlyContinue
    }
}

# ============================================
# Summary
# ============================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "Service Download Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Services in $binDir :"
Write-Host "  - MediaMTX: $(if (Test-Path $mmtxExe) { 'OK' } else { 'MISSING' })"
Write-Host "  - go-irl: $(if (Test-Path $srtlaExe) { 'OK' } else { 'MISSING' })"

if (-not (Test-Path $srtlaExe)) {
    Write-Host ""
    Write-Host "go-irl is missing. To fix:" -ForegroundColor Yellow
    Write-Host "  1. Close and reopen PowerShell (to refresh PATH)" -ForegroundColor White
    Write-Host "  2. Run: .\scripts\download_services.ps1" -ForegroundColor White
}
