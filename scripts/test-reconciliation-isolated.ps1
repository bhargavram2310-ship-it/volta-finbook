param([int]$Port = 33418)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$mysqlBin = Join-Path $env:ProgramFiles 'MySQL\MySQL Server 8.4\bin'
$serverExe = Join-Path $mysqlBin 'mysqld.exe'
$clientExe = Join-Path $mysqlBin 'mysql.exe'
if (-not (Test-Path -LiteralPath $serverExe)) { throw 'MySQL 8.4 server binary is required for isolated tests.' }
$testRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot ('.local\reconciliation-mysql-test-' + [Guid]::NewGuid().ToString('N'))))
$allowedRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot '.local')) + [IO.Path]::DirectorySeparatorChar
if (-not $testRoot.StartsWith($allowedRoot, [StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $testRoot) -notmatch '^reconciliation-mysql-test-[a-f0-9]{32}$') { throw 'Unsafe isolated test directory.' }
New-Item -ItemType Directory -Path $testRoot | Out-Null
$dataDir = Join-Path $testRoot 'data'
$serverProcess = $null
$testAuthenticated = $false
$previousURL = $env:DATABASE_URL
$previousRoot = $env:FINBOOK_ROOT
$previousIntegration = $env:FINBOOK_RECONCILIATION_INTEGRATION
try {
    $initializeArgs = @('--no-defaults', '--initialize-insecure', "--datadir=`"$dataDir`"", '--console')
    $initializer = Start-Process -FilePath $serverExe -ArgumentList $initializeArgs -Wait -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $testRoot 'initialize.out.log') -RedirectStandardError (Join-Path $testRoot 'initialize.err.log')
    if ($initializer.ExitCode -ne 0) { throw 'Isolated MySQL initialization failed.' }
    $serverArgs = @('--no-defaults', "--datadir=`"$dataDir`"", '--bind-address=127.0.0.1', "--port=$Port", '--mysqlx=0', '--console')
    $serverProcess = Start-Process -FilePath $serverExe -ArgumentList $serverArgs -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $testRoot 'server.out.log') -RedirectStandardError (Join-Path $testRoot 'server.err.log')
    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if ($serverProcess.HasExited) { throw 'Isolated MySQL exited before becoming ready.' }
        $probe = Start-Process -FilePath $clientExe -ArgumentList @('--no-defaults','--protocol=TCP','--host=127.0.0.1',"--port=$Port",'--user=root','--batch','--skip-column-names','--execute="SELECT 1"') -Wait -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $testRoot 'probe.out.log') -RedirectStandardError (Join-Path $testRoot 'probe.err.log')
        if ($probe.ExitCode -eq 0) { $ready = $true; break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'Isolated MySQL did not become ready.' }
    $testPassword = [Guid]::NewGuid().ToString('N')
    "ALTER USER 'root'@'localhost' IDENTIFIED BY '$testPassword';" | & $clientExe --no-defaults --protocol=TCP --host=127.0.0.1 --port=$Port --user=root --batch
    if ($LASTEXITCODE -ne 0) { throw 'Could not configure the temporary test user.' }
    $testAuthenticated = $true
    $env:DATABASE_URL = "mysql://root:$testPassword@127.0.0.1:$Port/mysql?tls=false"
    $env:FINBOOK_ROOT = $projectRoot
    $env:FINBOOK_RECONCILIATION_INTEGRATION = '1'
    Push-Location (Join-Path $projectRoot 'backend')
    try {
        & go test ./internal/reconciliation/... -v -count=1
        if ($LASTEXITCODE -ne 0) { throw 'Reconciliation isolated integration tests failed.' }
    } finally { Pop-Location }
} finally {
    $env:DATABASE_URL = $previousURL
    $env:FINBOOK_ROOT = $previousRoot
    $env:FINBOOK_RECONCILIATION_INTEGRATION = $previousIntegration
    if ($testAuthenticated) {
        $previousMySQLPassword = $env:MYSQL_PWD
        try {
            $env:MYSQL_PWD = $testPassword
            & $clientExe --no-defaults --protocol=TCP --host=127.0.0.1 --port=$Port --user=root --execute='SHUTDOWN'
        } finally { $env:MYSQL_PWD = $previousMySQLPassword }
    }
    $testPassword = $null
    if ($serverProcess -and -not $serverProcess.HasExited) { if (-not $serverProcess.WaitForExit(15000)) { Stop-Process -Id $serverProcess.Id -Force; $serverProcess.WaitForExit() } }
    # Delete only this script's newly created, checked workspace directory.
    if ($testRoot.StartsWith($allowedRoot, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $testRoot) -match '^reconciliation-mysql-test-[a-f0-9]{32}$') { Remove-Item -LiteralPath $testRoot -Recurse -Force }
}

