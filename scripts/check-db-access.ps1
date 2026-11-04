param(
    [string]$CredentialFile = 'D:\Projects\DB\postgresql.txt'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $goExecutable = $goCommand.Source }
elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') { $goExecutable = 'C:\Program Files\Go\bin\go.exe' }
else { throw 'Go is required. Reopen PowerShell after installing the Go SDK.' }
if (-not (Test-Path -LiteralPath $CredentialFile -PathType Leaf)) { throw 'The database connection file was not found.' }
$credentialPath = (Resolve-Path -LiteralPath $CredentialFile).ProviderPath
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
$env:GOMODCACHE = Join-Path $projectRoot '.cache\go-modules'
$env:GOTELEMETRY = 'off'
$env:GOFLAGS = '-mod=mod'
$reportFile = Join-Path $projectRoot '.cache\db-inspection.json'
# Remove the previous local report so a failed check cannot look successful.
if (Test-Path -LiteralPath $reportFile) { Remove-Item -LiteralPath $reportFile }
Write-Output 'Checking PostgreSQL credentials and visible schema metadata in a read-only transaction.'
Write-Output 'First run downloads the Go PostgreSQL driver. No credentials are sent to the package service.'
Push-Location (Join-Path $projectRoot 'tools\dbcheck')
try {
    & $goExecutable run . -credentials $credentialPath -report $reportFile
    if ($LASTEXITCODE -ne 0) { throw 'Database access check did not complete. No migrations were run.' }
} finally { Pop-Location }
