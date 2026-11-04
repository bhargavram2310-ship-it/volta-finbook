param([switch]$StatusOnly)
$ErrorActionPreference = 'Stop'
throw 'This is the retired PostgreSQL-only migration workflow. Do not run PostgreSQL migrations against MySQL. See docs\mysql-local-setup.md for the current local connection setup; MySQL schema migrations are not available yet.'

$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $goExecutable = $goCommand.Source }
elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') { $goExecutable = 'C:\Program Files\Go\bin\go.exe' }
else { throw 'Go is required.' }
$env:FINBOOK_ROOT = $projectRoot
$env:GOCACHE = Join-Path $projectRoot '.cache\go-build'
$env:GOMODCACHE = Join-Path $projectRoot '.cache\go-modules'
$env:GOTELEMETRY = 'off'
$testTemp = Join-Path $projectRoot '.cache\tmp'
New-Item -ItemType Directory -Force -Path $testTemp | Out-Null
$env:TEMP = $testTemp
$env:TMP = $testTemp
Push-Location (Join-Path $projectRoot 'backend')
try {
    if ($StatusOnly) {
        & $goExecutable run ./cmd/migrate -mode status
        if ($LASTEXITCODE -ne 0) { throw 'Could not inspect cloud migration status.' }
    } else {
        & $goExecutable test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Local tests failed. No migrations were attempted.' }
        & $goExecutable vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Static checks failed. No migrations were attempted.' }
        # Verify all SQL and controls in an isolated rollback-only cloud schema first.
        $previousIntegration = $env:FINBOOK_DB_INTEGRATION
        try {
            $env:FINBOOK_DB_INTEGRATION = '1'
            & $goExecutable test ./internal/database -run '^TestPostgreSQLIntegrity$' -count=1 -v
            if ($LASTEXITCODE -ne 0) { throw 'Cloud integrity tests failed. The real finbook schema was not migrated.' }
        } finally { $env:FINBOOK_DB_INTEGRATION = $previousIntegration }
        & $goExecutable run ./cmd/migrate -mode plan
        if ($LASTEXITCODE -ne 0) { throw 'Migration plan validation failed.' }
        & $goExecutable run ./cmd/migrate -mode apply
        if ($LASTEXITCODE -ne 0) { throw 'Migration failed. Its changes were rolled back.' }
        & $goExecutable run ./cmd/migrate -mode status
        if ($LASTEXITCODE -ne 0) { throw 'Post-migration verification failed.' }
        Write-Output 'Phase 2 cloud schema applied and verified. Existing public tables were not changed.'
    }
} finally { Pop-Location }
