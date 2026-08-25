#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Uninstalls the wr-agent Windows Service. See docs/AGENT.md §17.
#>
param(
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
$installDir = "$env:ProgramFiles\WartungsRemote"
$exe = Join-Path $installDir "wr-agent.exe"

if (Test-Path $exe) {
    & $exe --service stop
    & $exe --service uninstall
}

Remove-Item -Path $installDir -Recurse -Force -ErrorAction SilentlyContinue

if ($RemoveData) {
    Remove-Item -Path "$env:ProgramData\WartungsRemote" -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "wr-agent uninstalled."
