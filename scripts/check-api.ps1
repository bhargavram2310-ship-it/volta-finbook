$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $goExecutable = $goCommand.Source }
elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') { $goExecutable = 'C:\Program Files\Go\bin\go.exe' }
else { throw 'Go is required. Install Go from https://go.dev/dl/.' }
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
$env:GOMODCACHE = Join-Path $projectRoot '.cache\go-modules'
$env:GOTELEMETRY = 'off'
$testTemp = Join-Path $projectRoot '.cache\tmp'
New-Item -ItemType Directory -Force -Path $testTemp | Out-Null
$env:TEMP = $testTemp
$env:TMP = $testTemp
Push-Location (Join-Path $projectRoot 'backend')
try {
    & $goExecutable test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & $goExecutable vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
} finally { Pop-Location }
