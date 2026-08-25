#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Uninstalls the wr-core Windows Service (native, non-Docker deployment).
#>
param(
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
$installDir = "$env:ProgramFiles\WartungsRemoteCore"
$exe = Join-Path $installDir "wr-core.exe"

if (Test-Path $exe) {
    & $exe --service stop
    & $exe --service uninstall
}

Remove-Item -Path $installDir -Recurse -Force -ErrorAction SilentlyContinue

if ($RemoveData) {
    Remove-Item -Path "$env:ProgramData\WartungsRemoteCore" -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "wr-core uninstalled. The PostgreSQL database was not touched."
