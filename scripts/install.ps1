#
# Agent Installer for Windows
# Usage: irm https://your-domain.com/install.ps1 | iex
#        .\install.ps1 -Server "ws://your-server:8080"
#

param(
    [string]$Server = "",
    [string]$Version = "latest",
    [string]$InstallDir = "$env:ProgramFiles\Avaropoint"
)

$ErrorActionPreference = "Stop"

# Configuration
$Repo = "avaropoint/rmm"
$ServiceName = "Agent"

function Write-Banner {
    Write-Host ""
    Write-Host "Agent Installer" -ForegroundColor Cyan
    Write-Host ""
}

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] " -ForegroundColor Cyan -NoNewline
    Write-Host $Message
}

function Write-Success {
    param([string]$Message)
    Write-Host "[OK] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-Architecture {
    if ([Environment]::Is64BitOperatingSystem) {
        return "amd64"
    } else {
        return "386"
    }
}

function Install-Binary {
    Write-Info "Downloading agent..."
    
    $arch = Get-Architecture
    
    if ($Version -eq "latest") {
        $downloadUrl = "https://github.com/$Repo/releases/latest/download/agent-windows-$arch.exe"
    } else {
        $downloadUrl = "https://github.com/$Repo/releases/download/$Version/agent-windows-$arch.exe"
    }
    
    # Check for local binary first (for testing)
    $localBin = ".\bin\agent-windows-$arch.exe"
    if (Test-Path $localBin) {
        Write-Info "Using local binary: $localBin"
        $sourcePath = $localBin
    } else {
        $tempPath = "$env:TEMP\agent.exe"
        
        try {
            Invoke-WebRequest -Uri $downloadUrl -OutFile $tempPath -UseBasicParsing
        } catch {
            Write-Error "Download failed: $_"
        }
        
        $sourcePath = $tempPath
    }
    
    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    
    # Copy binary
    $destPath = Join-Path $InstallDir "agent.exe"
    Copy-Item -Path $sourcePath -Destination $destPath -Force
    
    # Cleanup temp file
    if ($sourcePath -eq "$env:TEMP\agent.exe" -and (Test-Path $sourcePath)) {
        Remove-Item $sourcePath -Force
    }
    
    Write-Success "Installed to $destPath"
    return $destPath
}

function Add-ToPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    if ($currentPath -notlike "*$Directory*") {
        Write-Info "Adding to system PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$currentPath;$Directory", "Machine")
        $env:Path = "$env:Path;$Directory"
        Write-Success "Added to PATH"
    }
}

function Install-Service {
    param([string]$BinaryPath)
    
    if ([string]::IsNullOrEmpty($Server)) {
        Write-Warn "No server URL provided. Skipping service setup."
        Write-Warn "Run with: -Server ws://your-server:8080"
        return
    }
    
    Write-Info "Installing Windows service..."
    
    # Stop and remove existing service if present
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-Info "Removing existing service..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
    }
    
    # Create the service
    $binPathWithArgs = "`"$BinaryPath`" -server $Server"
    
    New-Service -Name $ServiceName `
        -BinaryPathName $binPathWithArgs `
        -DisplayName "Remote Desktop Agent" `
        -Description "Remote desktop agent service" `
        -StartupType Automatic | Out-Null
    
    # Start the service
    Start-Service -Name $ServiceName
    
    Write-Success "Service installed and started"
    Write-Info "Check status: Get-Service $ServiceName"
}

function Add-FirewallRule {
    Write-Info "Configuring firewall..."
    
    # Remove existing rule if present
    Remove-NetFirewallRule -DisplayName "Remote Desktop Agent" -ErrorAction SilentlyContinue
    
    # Add outbound rule (agent connects to server)
    New-NetFirewallRule -DisplayName "Remote Desktop Agent" `
        -Direction Outbound `
        -Program (Join-Path $InstallDir "agent.exe") `
        -Action Allow | Out-Null
    
    Write-Success "Firewall rule added"
}

function Write-Complete {
    Write-Host ""
    Write-Host "Installation complete." -ForegroundColor Green
    Write-Host ""
    Write-Host "Binary installed: $InstallDir\agent.exe"
    Write-Host ""
    
    if ([string]::IsNullOrEmpty($Server)) {
        Write-Host "To run manually:"
        Write-Host "  agent.exe -server ws://your-server:8080" -ForegroundColor Cyan
        Write-Host ""
        Write-Host "To install as a service, re-run with:"
        Write-Host "  .\install.ps1 -Server ws://your-server:8080" -ForegroundColor Cyan
    } else {
        Write-Host "Service status:"
        Get-Service $ServiceName | Format-Table Name, Status, StartType
    }
    Write-Host ""
}

# Main
Write-Banner

# Check for admin rights
if (-not (Test-Administrator)) {
    Write-Warn "Not running as Administrator. Some features may not work."
    Write-Warn "For full installation, run PowerShell as Administrator."
    Write-Host ""
}

$binaryPath = Install-Binary

if (Test-Administrator) {
    Add-ToPath -Directory $InstallDir
    Add-FirewallRule
    Install-Service -BinaryPath $binaryPath
}

Write-Complete
