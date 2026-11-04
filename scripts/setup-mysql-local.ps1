param(
    [string]$LoginPath = 'finbook-admin',
    [string]$MySQL = 'mysql'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$credentialDirectory = Join-Path $projectRoot 'backend\.local'
$credentialFile = Join-Path $credentialDirectory 'mysql.url'
$databaseScript = Join-Path $PSScriptRoot 'mysql-local.sql'

$mysqlExe = (Get-Command $MySQL -ErrorAction SilentlyContinue).Source

if (-not $mysqlExe -and $MySQL -eq 'mysql') {
    $installedClient = Join-Path $env:ProgramFiles 'MySQL\MySQL Server 8.4\bin\mysql.exe'

    if (Test-Path -LiteralPath $installedClient -PathType Leaf) {
        $mysqlExe = $installedClient
    }
}

if (-not $mysqlExe) {
    throw 'MySQL client was not found. Install MySQL 8.4 or pass its mysql.exe path with -MySQL.'
}
$securePassword = Read-Host 'Choose a password for the local finbook_app account' -AsSecureString
$plainPassword = $null
try {
    $plainPassword = [System.Net.NetworkCredential]::new('', $securePassword).Password
    if ([string]::IsNullOrWhiteSpace($plainPassword)) {
        throw 'A non-empty MySQL application password is required.'
    }

    $passwordHex = [BitConverter]::ToString([System.Text.Encoding]::UTF8.GetBytes($plainPassword)).Replace('-', '')
    $bootstrapSQL = Get-Content -LiteralPath $databaseScript -Raw
    $sql = @"
$bootstrapSQL
SET @finbook_password = CONVERT(UNHEX('$passwordHex') USING utf8mb4);
SET @finbook_user_sql = CONCAT('CREATE USER IF NOT EXISTS ''finbook_app''@''127.0.0.1'' IDENTIFIED BY ', QUOTE(@finbook_password));
PREPARE finbook_user_statement FROM @finbook_user_sql;
EXECUTE finbook_user_statement;
DEALLOCATE PREPARE finbook_user_statement;
SET @finbook_user_sql = CONCAT('ALTER USER ''finbook_app''@''127.0.0.1'' IDENTIFIED BY ', QUOTE(@finbook_password));
PREPARE finbook_user_statement FROM @finbook_user_sql;
EXECUTE finbook_user_statement;
DEALLOCATE PREPARE finbook_user_statement;
GRANT ALL PRIVILEGES ON finbook.* TO 'finbook_app'@'127.0.0.1';
"@

$sql | & $mysqlExe --login-path=$LoginPath --protocol=TCP --host=127.0.0.1 --port=3306 --batch --skip-column-names
    if ($LASTEXITCODE -ne 0) {
        throw 'MySQL initialization failed. Check that MySQL 8.4 is running and the configured admin login path is valid.'
    }

    New-Item -ItemType Directory -Force -Path $credentialDirectory | Out-Null
    $encodedPassword = [Uri]::EscapeDataString($plainPassword)
    $connectionURL = "mysql://finbook_app:$encodedPassword@127.0.0.1:3306/finbook?tls=false"
    Set-Content -LiteralPath $credentialFile -Value $connectionURL -Encoding ascii -NoNewline

    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    $credentialAcl = [System.Security.AccessControl.FileSecurity]::new()
    $credentialAcl.SetAccessRuleProtection($true, $false)
    $credentialRule = [System.Security.AccessControl.FileSystemAccessRule]::new(
        $identity,
        [System.Security.AccessControl.FileSystemRights]::Modify,
        [System.Security.AccessControl.AccessControlType]::Allow
    )
    $credentialAcl.SetAccessRule($credentialRule)
    try {
        Set-Acl -LiteralPath $credentialFile -AclObject $credentialAcl
    } catch {
        Remove-Item -LiteralPath $credentialFile -ErrorAction SilentlyContinue
        throw 'Could not restrict permissions on the local database credential file.'
    }
}
finally {
    $plainPassword = $null
    $securePassword.Dispose()
}

Write-Output 'Local MySQL database and application account are configured.'
Write-Output 'Connection settings were saved in backend\.local\mysql.url with user-only file permissions.'
Write-Output 'This script configures the MySQL database and application credentials; it does not apply the application schema.'
Write-Output 'Migration history and financial posting controls are not yet implemented for MySQL, so readiness remains pending.'
