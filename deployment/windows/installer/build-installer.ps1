<#
.SYNOPSIS
    Builds wr-agent.exe for windows/amd64 and packages it into a single
    self-contained graphical installer (WartungsRemoteAgentSetup.exe) that
    asks the technician for the server URL and enrollment token, then
    installs and starts the agent as a Windows Service.

    Run this from a repo checkout with Go and the .NET SDK installed.
.EXAMPLE
    .\deployment\windows\installer\build-installer.ps1
#>
param(
    [string]$OutputDir = (Join-Path $PSScriptRoot "..\..\..\dist")
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..\..")
$projectDir = Join-Path $PSScriptRoot "WrAgentInstaller"
$payloadDir = Join-Path $projectDir "Payload"

Write-Host "Building wr-agent.exe (windows/amd64)..."
New-Item -ItemType Directory -Force -Path $payloadDir | Out-Null
Push-Location $repoRoot
try {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $payloadDir "wr-agent.exe") .\cmd\wr-agent
}
finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

Write-Host "Publishing WartungsRemoteAgentSetup.exe..."
Push-Location $projectDir
try {
    dotnet publish -c Release
}
finally {
    Pop-Location
}

$publishDir = Join-Path $projectDir "bin\Release\net10.0-windows\win-x64\publish"
$builtExe = Join-Path $publishDir "WartungsRemoteAgentSetup.exe"
if (-not (Test-Path $builtExe)) {
    throw "build did not produce $builtExe"
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$destExe = Join-Path $OutputDir "WartungsRemoteAgentSetup.exe"
Copy-Item -Path $builtExe -Destination $destExe -Force

Write-Host "Done: $destExe"
