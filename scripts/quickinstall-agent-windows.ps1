<#
.SYNOPSIS
    WartungsRemote agent one-line installer for Windows. Looks up the
    latest signed agent-* GitHub release, downloads the Windows binary,
    and installs it as a Windows Service via install-agent.ps1
    (unmodified — this script only fetches what that one needs).

    Generated for you with server URL + token already filled in by the
    dashboard's "+ Add Device" panel.
.PARAMETER Channel
    "stable" (default) installs the newest agent-* release that is NOT
    marked as a GitHub pre-release. "beta" installs the newest agent-*
    release regardless of pre-release status.
.EXAMPLE
    $s = New-TemporaryFile | Rename-Item -NewName { $_.Name + ".ps1" } -PassThru
    Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/mrder/wartungsremote/main/scripts/quickinstall-agent-windows.ps1" -OutFile $s
    powershell -ExecutionPolicy Bypass -File $s -ServerUrl "https://remote.example.de" -Token "wr_enroll_XXXXXXXX"
#>
#Requires -RunAsAdministrator
param(
    [Parameter(Mandatory = $true)][string]$ServerUrl,
    [Parameter(Mandatory = $true)][string]$Token,
    [string]$Repo = "mrder/wartungsremote",
    [ValidateSet("stable", "beta")][string]$Channel = "stable"
)

$ErrorActionPreference = "Stop"

Write-Host "Looking up the latest signed agent release (channel: $Channel)..."
$releases = Invoke-RestMethod -UseBasicParsing -Uri "https://api.github.com/repos/$Repo/releases"
$candidates = $releases | Where-Object { $_.tag_name -like "agent-*" }
if ($Channel -eq "stable") {
    $candidates = $candidates | Where-Object { -not $_.prerelease }
}
$release = $candidates | Select-Object -First 1
if (-not $release) {
    throw "could not find a published agent-* release (channel: $Channel) under $Repo"
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

    powershell -ExecutionPolicy Bypass -File $installScriptPath -ServerUrl $ServerUrl -Token $Token -BinaryPath $exePath

    Write-Host "Done. Check status with: Get-Service wartungsremote-agent"
}
finally {
    Remove-Item -Recurse -Force $workDir -ErrorAction SilentlyContinue
}
