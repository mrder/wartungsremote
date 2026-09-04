#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs wr-agent as a Windows Service. See docs/AGENT.md and docs/TODO.md Phase 26.
.EXAMPLE
    .\install-agent.ps1 -ServerUrl "https://remote.example.de" -Token "wr_enroll_XXXXXXXX"
#>
param(
    [Parameter(Mandatory = $true)][string]$ServerUrl,
    [string]$Token,
    [string]$BinaryPath = (Join-Path $PSScriptRoot "..\..\wr-agent.exe")
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BinaryPath)) {
    throw "wr-agent.exe not found at $BinaryPath (build it first: go build -o wr-agent.exe .\cmd\wr-agent)"
}

$installDir = "$env:ProgramFiles\WartungsRemote"
$configDir  = "$env:ProgramData\WartungsRemote\config"
$dataDir    = "$env:ProgramData\WartungsRemote\data"
$logDir     = "$env:ProgramData\WartungsRemote\logs"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$existingExe = Join-Path $installDir "wr-agent.exe"
if (Test-Path $existingExe) {
    Write-Host "Existing installation found, stopping service for upgrade..."
    & $existingExe --service stop
    & $existingExe --service uninstall
}

Copy-Item -Path $BinaryPath -Destination (Join-Path $installDir "wr-agent.exe") -Force

$configPath = Join-Path $configDir "agent.yaml"
if (-not (Test-Path $configPath)) {
@"
server_url: $ServerUrl
update_channel: stable
log_level: info
policy:
  terminal: true
  ssh_tunnel: true
  rdp_tunnel: true
  files_read: true
  files_write: true
  service_control: true
  process_terminate: true
  power_control: true
"@ | Set-Content -Path $configPath -Encoding utf8
} else {
    # Keep any hand-tuned policy flags, but never let server_url go stale on
    # a reinstall/upgrade — a customer moving to a new server, or re-running
    # this with a corrected URL, must actually take effect.
    (Get-Content -Path $configPath) -replace '^server_url:.*', "server_url: $ServerUrl" |
        Set-Content -Path $configPath -Encoding utf8
}

if ($Token) {
    Set-Content -Path (Join-Path $dataDir "enroll.token") -Value $Token -Encoding ascii -NoNewline
    # A freshly supplied token means "(re-)enroll this device" — an old
    # stored identity from a previous enrollment must not silently win and
    # make the new token look like it did nothing.
    $credentialFile = Join-Path $dataDir "device_credential.dat"
    if (Test-Path $credentialFile) {
        Write-Host "New token supplied, clearing previous device identity to force re-enrollment..."
        Remove-Item -Path $credentialFile -Force
    }
}

# Lock the ProgramData tree down to Administrators + SYSTEM; DPAPI protects
# the credential file itself in machine scope on top of this (docs/SECURITY.md §6).
icacls "$env:ProgramData\WartungsRemote" /inheritance:r | Out-Null
icacls "$env:ProgramData\WartungsRemote" /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null

$exe = Join-Path $installDir "wr-agent.exe"
& $exe --service install
& $exe --service start

Write-Host "wr-agent installed and started. Check status with: Get-Service wartungsremote-agent"
