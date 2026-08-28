# Generates the local secret files referenced by deployment/docker/docker-compose.yml.
# Run once before the first `docker compose up`. These files are gitignored.
$ErrorActionPreference = "Stop"
$dir = Join-Path $PSScriptRoot "..\deployment\docker\secrets"
New-Item -ItemType Directory -Force -Path $dir | Out-Null

function New-RandomBytesFile([string]$path, [int]$bytes) {
    if (-not (Test-Path $path)) {
        $buf = New-Object byte[] $bytes
        [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
        [System.IO.File]::WriteAllBytes($path, $buf)
    }
}

$dbPasswordPath = Join-Path $dir "db_password.txt"
if (-not (Test-Path $dbPasswordPath)) {
    $buf = New-Object byte[] 24
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
    [Convert]::ToBase64String($buf) | Set-Content -Path $dbPasswordPath -NoNewline -Encoding ascii
}

$databaseUrlPath = Join-Path $dir "database_url.txt"
if (-not (Test-Path $databaseUrlPath)) {
    $dbPass = Get-Content $dbPasswordPath -Raw
    "postgres://wartungsremote:$dbPass@postgres:5432/wartungsremote?sslmode=disable" | Set-Content -Path $databaseUrlPath -NoNewline -Encoding ascii
}

# Least-privilege runtime role (docs/DEPLOYMENT.md §5a) - db-init creates/
# updates it on every stack startup from this same password.
$appRolePasswordPath = Join-Path $dir "app_role_password.txt"
if (-not (Test-Path $appRolePasswordPath)) {
    $buf = New-Object byte[] 24
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
    [Convert]::ToBase64String($buf) | Set-Content -Path $appRolePasswordPath -NoNewline -Encoding ascii
}
$runtimeDatabaseUrlPath = Join-Path $dir "runtime_database_url.txt"
if (-not (Test-Path $runtimeDatabaseUrlPath)) {
    $appPass = Get-Content $appRolePasswordPath -Raw
    "postgres://wartungsremote_app:$appPass@postgres:5432/wartungsremote?sslmode=disable" | Set-Content -Path $runtimeDatabaseUrlPath -NoNewline -Encoding ascii
}

New-RandomBytesFile (Join-Path $dir "session_pepper.bin") 32
New-RandomBytesFile (Join-Path $dir "totp_key.bin") 32

$adminPasswordPath = Join-Path $dir "admin_password.txt"
if (-not (Test-Path $adminPasswordPath)) {
    $buf = New-Object byte[] 18
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($buf)
    [Convert]::ToBase64String($buf) | Set-Content -Path $adminPasswordPath -NoNewline -Encoding ascii
}

Write-Host "Secrets generated in $dir"
Write-Host "First admin account password: $adminPasswordPath (used by the createadmin step in the README - change it after first login)"
