# Local MySQL setup

The backend accepts local MySQL connection settings. MySQL table DDL is being ported under `backend/migrations/mysql`, but it is not yet a complete or tracked migration system: the files are not safe to rerun, and PostgreSQL posting, audit, and immutability controls are not yet implemented for MySQL. The migration command deliberately refuses to execute PostgreSQL SQL against MySQL. Do not use this database for real financial records or posting yet.

## Configure a fresh local MySQL 8.4 database

1. Install MySQL Community Server 8.4 and complete the elevated MySQL Configurator. Choose a strong MySQL `root` password, set the server to run as a Windows service, and start it. The password stays in the configurator; do not send it in chat.
2. In PowerShell, create a protected local admin login path. Use the full path if MySQL is not on `PATH`. The command prompts for the MySQL admin password; it does not put the password in the command line:

   ```powershell
   & 'C:\Program Files\MySQL\MySQL Server 8.4\bin\mysql_config_editor.exe' set --login-path=finbook-admin --host=127.0.0.1 --user=root --password
   ```

3. From the project root, run the bootstrap script:

   ```powershell
   Set-Location 'D:\Projects\Volta-Tours-Finance'
   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\setup-mysql-local.ps1
   ```

   The script creates the empty `finbook` database using `utf8mb4`, creates a dedicated `finbook_app` account restricted to the local host, grants it rights only on `finbook`, and saves the URL to the ignored `backend\.local\mysql.url` file. It prompts for the app password and applies user-only permissions to that credential file. The admin password is stored by MySQL's login-path utility, not in this repository. The setup script locates the standard MySQL 8.4 installation even if `mysql.exe` is not on `PATH`.

4. Start the Go API:

   ```powershell
   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-api.ps1
   ```

   The API reads `DATABASE_URL` first, then `DATABASE_CREDENTIAL_FILE`, then `backend\.local\mysql.url`. If an existing `DATABASE_URL` environment variable is set, it takes precedence over the local file.

5. Test connectivity:

   ```powershell
   Invoke-RestMethod http://127.0.0.1:8080/healthz
   Invoke-RestMethod http://127.0.0.1:8080/api/v1/status
   ```

   An empty database reports pending MySQL DDL. A database with application tables but no recognized migration history is reported as schema-incompatible; do not rerun the non-idempotent SQL files to try to repair it. `/readyz` remains unavailable until the MySQL migrations and financial controls are tracked and verified. `scripts\complete-phase2.ps1` is the retired PostgreSQL migration workflow and must not be used for MySQL.

The local URL uses `tls=false` only for loopback connections. Remote MySQL URLs must set `tls=verify-full`; certificate verification cannot be disabled for remote hosts. Credentials must remain server-side and must not be placed in frontend `VITE_` variables or source control.

## Employee information screen

The employee workspace is available at `/app/administration/employees`, user management at `/app/administration/users`, and role management at `/app/administration/roles`. Any signed-in account can maintain employee, user, and role records. Users may have multiple roles; roles are organizational labels and do not grant application permissions. Create an employee before creating a user linked to it. Employee, user, role, and role-assignment changes are audited, and updates use record versions to reject stale edits.

After installing the MySQL application schema, apply `backend\migrations\mysql\0005_user_login_method.sql` once using the MySQL client (after `0004_employee_access.sql`):

```powershell
Get-Content -Raw .\backend\migrations\mysql\0005_user_login_method.sql |
  & 'C:\Program Files\MySQL\MySQL Server 8.4\bin\mysql.exe' --login-path=finbook-admin finbook
```

Then run `go run ./cmd/bootstrap-admin` from `backend` once against a database with no users. It creates the initial linked employee and login **SuperAdmin / SuperAdmin** and requires a password change at first sign-in. Do not keep the default password. The command refuses to run if any users already exist. User accounts can be configured for username-only or email-only sign-in; existing users default to username sign-in.
