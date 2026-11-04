$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) {
    $goExecutable = $goCommand.Source
} elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') {
    $goExecutable = 'C:\Program Files\Go\bin\go.exe'
} else {
    throw 'Go is required. Install Go from https://go.dev/dl/ and reopen PowerShell.'
}
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
$env:GOMODCACHE = Join-Path $projectRoot '.cache\go-modules'
$env:GOTELEMETRY = 'off'
if (-not $env:FINBOOK_ROOT) { $env:FINBOOK_ROOT = $projectRoot }
Push-Location (Join-Path $projectRoot 'backend')
try { & $goExecutable run ./cmd/api; if ($LASTEXITCODE -ne 0) { throw 'API exited with an error.' } } finally { Pop-Location }
