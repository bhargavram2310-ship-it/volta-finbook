# Local MySQL setup

The Go backend accepts local MySQL connection settings and applies the versioned SQL files under `backend/migrations/mysql` with a MySQL migration-history table. The DDL is not transactional: an interrupted migration is marked incomplete and requires deliberate recovery. The current schema is not proof that accounting posting, finance-approved rules, or all financial integrity controls are ready. Do not use this database for real financial records or posting yet.

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

4. From the project root, apply the migrations to the new, empty `finbook` schema:

   ```powershell
   Set-Location 'D:\Projects\Volta-Tours-Finance\backend'
   go run ./cmd/migrate -mode status
   go run ./cmd/migrate -mode plan
   go run ./cmd/migrate -mode apply
   go run ./cmd/migrate -mode status
   ```

   The plan command is read-only and lists pending versions and checksums. The final status should report all available migrations applied and `current: true`. The runner verifies migration sequence and checksums, refuses untracked existing application tables, and records an in-progress migration before executing DDL. Because MySQL DDL cannot be rolled back, do not delete migration history or blindly rerun SQL after an interrupted migration; investigate the incomplete version and recover the empty local schema deliberately. Do not run this against a database containing business data.

5. In another terminal, start the Go API from the project root:

   ```powershell
   Set-Location 'D:\Projects\Volta-Tours-Finance'
   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-api.ps1
   ```

   The API reads `DATABASE_URL` first, then `DATABASE_CREDENTIAL_FILE`, then `backend\.local\mysql.url`. If an existing `DATABASE_URL` environment variable is set, it takes precedence over the local file.

6. Test connectivity:

   ```powershell
   Invoke-RestMethod http://127.0.0.1:8080/healthz
   Invoke-RestMethod http://127.0.0.1:8080/api/v1/status
   ```

   `/api/v1/status` should report `database: "connected"` and `schema_ready: true`. `/readyz` indicates that the currently available schema migrations are applied; it does not mean financial posting or all ERP workflows are ready. A database with application tables but no recognized migration history is rejected. `scripts\complete-phase2.ps1` is the retired PostgreSQL migration workflow and must not be used for MySQL.

The local URL uses `tls=false` only for loopback connections. Remote MySQL URLs must set `tls=verify-full`; certificate verification cannot be disabled for remote hosts. Credentials must remain server-side and must not be placed in frontend `VITE_` variables or source control.

## Employee information screen

The employee workspace is available at `/app/administration/employees`, user management at `/app/administration/users`, and role management at `/app/administration/roles`. Any signed-in account can maintain employee, user, and role records. Users may have multiple roles; roles are organizational labels and do not grant application permissions. Create an employee before creating a user linked to it. Employee, user, role, and role-assignment changes are audited, and updates use record versions to reject stale edits.

After the migrations are current, run `go run ./cmd/bootstrap-admin` from `backend` once against a database with no users. It creates the initial linked employee and login **SuperAdmin / SuperAdmin** and requires a password change at first sign-in. Do not keep the default password. The command refuses to run if any users already exist. User accounts can be configured for username-only or email-only sign-in; existing users default to username sign-in.
