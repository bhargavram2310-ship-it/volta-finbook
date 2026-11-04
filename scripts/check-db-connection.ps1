param(
    [string]$CredentialFile = 'D:\Projects\DB\postgresql.txt'
)

$ErrorActionPreference = 'Stop'
$dbSocket = $null
$dbTls = $null
try {
    $dbSetting = (Get-Content -LiteralPath $CredentialFile -Raw).Trim()
    $dbConnectionValue = ($dbSetting -replace '^\s*DATABASE_URL\s*=\s*', '').Trim().Trim('"').Trim("'")
    $dbUri = $null
    if (-not [Uri]::TryCreate($dbConnectionValue, [UriKind]::Absolute, [ref]$dbUri) -or $dbUri.Scheme -notin @('postgres', 'postgresql')) {
        throw [InvalidOperationException]::new('Invalid PostgreSQL URL format.')
    }
    $dbTargetPort = if ($dbUri.Port -gt 0) { $dbUri.Port } else { 5432 }
    $dbAddresses = [Net.Dns]::GetHostAddresses($dbUri.DnsSafeHost)
    if ($dbAddresses.Count -eq 0) { throw [InvalidOperationException]::new('No database addresses found.') }
    Write-Output 'Database hostname resolved.'
    $dbSocket = [Net.Sockets.TcpClient]::new()
    $dbConnect = $dbSocket.ConnectAsync($dbUri.DnsSafeHost, $dbTargetPort)
    if (-not $dbConnect.Wait(8000)) { throw [TimeoutException]::new('Database TCP connection timed out.') }
    Write-Output 'Database TCP connection succeeded.'
    $dbStream = $dbSocket.GetStream()
    $dbStream.ReadTimeout = 8000
    $dbStream.WriteTimeout = 8000
    # PostgreSQL SSLRequest: length 8, request code 80877103. No credentials sent.
    $dbSslRequest = [byte[]]@(0, 0, 0, 8, 4, 210, 22, 47)
    $dbStream.Write($dbSslRequest, 0, $dbSslRequest.Length)
    if ($dbStream.ReadByte() -ne 83) { throw [InvalidOperationException]::new('Server did not accept PostgreSQL TLS negotiation.') }
    $dbTls = [Net.Security.SslStream]::new($dbStream, $false)
    $dbTlsHandshake = $dbTls.AuthenticateAsClientAsync($dbUri.DnsSafeHost)
    if (-not $dbTlsHandshake.Wait(8000)) { throw [TimeoutException]::new('Database TLS negotiation timed out.') }
    Write-Output 'Database TLS connection succeeded; the server certificate was validated.'
    Write-Output 'Connectivity check only. Password authentication and schema access have not been tested.'
} catch {
    $dbFailure = $_.Exception.GetBaseException()
    # Do not print exception messages: they can contain connection details.
    if ($dbFailure -is [Net.Sockets.SocketException]) {
        Write-Output ('Database connection failed: ' + $dbFailure.SocketErrorCode + ' (Windows ' + $dbFailure.NativeErrorCode + ').')
    } else {
        Write-Output ('Database connectivity check failed: ' + $dbFailure.GetType().Name + '. Check the connection file, network and TLS setup.')
    }
    exit 1
} finally {
    if ($null -ne $dbTls) { $dbTls.Dispose() }
    if ($null -ne $dbSocket) { $dbSocket.Dispose() }
}
