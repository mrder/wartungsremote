#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs wr-core as a Windows Service for native (non-Docker) deployment.
    Requires a reachable PostgreSQL instance; see docs/DEPLOYMENT.md.
.EXAMPLE
    .\install-core.ps1 -DatabaseUrl "postgres://wruser:pass@localhost:5432/wartungsremote"
#>
param(
    [Parameter(Mandatory = $true)][string]$DatabaseUrl,
    [string]$BinaryPath = (Join-Path $PSScriptRoot "..\..\wr-core.exe"),
    [string]$Mode = "production"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BinaryPath)) {
    throw "wr-core.exe not found at $BinaryPath (build it first: go build -o wr-core.exe .\cmd\wr-core)"
}

$installDir = "$env:ProgramFiles\WartungsRemoteCore"
$configDir  = "$env:ProgramData\WartungsRemoteCore\config"
$secretsDir = "$env:ProgramData\WartungsRemoteCore\secrets"
$logDir     = "$env:ProgramData\WartungsRemoteCore\logs"

New-Item -ItemType Directory -Force -Path $installDir, $configDir, $secretsDir, $logDir | Out-Null

Copy-Item -Path $BinaryPath -Destination (Join-Path $installDir "wr-core.exe") -Force

$configPath = Join-Path $configDir "server.yaml"
if (-not (Test-Path $configPath)) {
    Copy-Item -Path (Join-Path $PSScriptRoot "..\..\deployment\docker\server.example.yaml") -Destination $configPath -Force
}

$dbSecretPath = Join-Path $secretsDir "database_url"
Set-Content -Path $dbSecretPath -Value $DatabaseUrl -Encoding ascii -NoNewline

function New-RandomSecretFile([string]$path, [int]$bytes) {
    if (-not (Test-Path $path)) {
        $buf = New-Object byte[] $bytes
        [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
        [System.IO.File]::WriteAllBytes($path, $buf)
    }
}
New-RandomSecretFile (Join-Path $secretsDir "session_pepper") 32
New-RandomSecretFile (Join-Path $secretsDir "totp_key") 32

icacls "$env:ProgramData\WartungsRemoteCore" /inheritance:r | Out-Null
icacls "$env:ProgramData\WartungsRemoteCore" /grant:r "SYSTEM:(OI)(CI)F" "Administrators:(OI)(CI)F" | Out-Null

# Secret file paths are baked into the service's own command line at install
# time (see cmd/wr-core --database-url-file etc.), not set as machine
# environment variables: Windows services do not reliably observe env
# changes made after the SCM started without a reboot.
$sessionPepperPath = Join-Path $secretsDir "session_pepper"
$totpKeyPath = Join-Path $secretsDir "totp_key"

$exe = Join-Path $installDir "wr-core.exe"
& $exe --service install `
    --config $configPath `
    --database-url-file $dbSecretPath `
    --session-pepper-file $sessionPepperPath `
    --totp-key-file $totpKeyPath
& $exe --service start

Write-Host "wr-core installed and started. Check status with: Get-Service wartungsremote-core"
Write-Host "Create the first super_admin with:"
Write-Host "  & '$exe' createadmin --username admin --password-file <path> --config '$configPath'"
