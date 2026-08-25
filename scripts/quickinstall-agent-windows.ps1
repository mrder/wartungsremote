<#
.SYNOPSIS
    WartungsRemote agent one-line installer for Windows. Looks up the
    latest signed agent-* GitHub release, downloads the Windows binary,
    and installs it as a Windows Service via install-agent.ps1
    (unmodified — this script only fetches what that one needs).

    Generated for you with server URL + token already filled in by the
    dashboard's "+ Add Device" panel.
.EXAMPLE
    $s = New-TemporaryFile
    Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/mrder/wartungsremote/main/scripts/quickinstall-agent-windows.ps1" -OutFile $s
    & $s -ServerUrl "https://remote.example.de" -Token "wr_enroll_XXXXXXXX"
#>
#Requires -RunAsAdministrator
param(
    [Parameter(Mandatory = $true)][string]$ServerUrl,
    [Parameter(Mandatory = $true)][string]$Token,
    [string]$Repo = "mrder/wartungsremote"
)

$ErrorActionPreference = "Stop"

Write-Host "Looking up the latest signed agent release..."
$releases = Invoke-RestMethod -UseBasicParsing -Uri "https://api.github.com/repos/$Repo/releases"
$release = $releases | Where-Object { $_.tag_name -like "agent-*" } | Select-Object -First 1
if (-not $release) {
    throw "could not find a published agent-* release under $Repo"
}
$tag = $release.tag_name
Write-Host "Installing $tag for windows/amd64"

$workDir = Join-Path $env:TEMP "wr-quickinstall-$([guid]::NewGuid())"
New-Item -ItemType Directory -Force -Path $workDir | Out-Null
try {
    $exePath = Join-Path $workDir "wr-agent.exe"
    $installScriptPath = Join-Path $workDir "install-agent.ps1"

    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$Repo/releases/download/$tag/wr-agent-windows-amd64.exe" -OutFile $exePath
    Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/$Repo/main/deployment/windows/install-agent.ps1" -OutFile $installScriptPath

    & $installScriptPath -ServerUrl $ServerUrl -Token $Token -BinaryPath $exePath

    Write-Host "Done. Check status with: Get-Service wartungsremote-agent"
}
finally {
    Remove-Item -Recurse -Force $workDir -ErrorAction SilentlyContinue
}
