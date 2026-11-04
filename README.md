# VOLTA FinBook

Travel-agency sales, operations and accounting ERP. Employee and user master changes are supported when the MySQL application schema is provisioned. Financial entry, persistence and posting remain disabled.

**MySQL switch status:** the Go API can connect to a local MySQL 8.4 database, and a local bootstrap/configuration script is available. The PostgreSQL financial schema and its database-enforced integrity controls have not yet been ported, so migration is deliberately blocked and `/readyz` remains unavailable. Do not use the database for real financial records. See [local MySQL setup](docs/mysql-local-setup.md). The old PostgreSQL foundation is retained as [legacy documentation](docs/phase-2-database.md).

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\setup-mysql-local.ps1
```

## Employee and user setup

The **Admin Control → Employees**, **Users**, and **Roles** pages let signed-in users maintain employee records, login accounts, and named roles. Create roles and assign multiple roles to a user; roles are labels only and do not grant application permissions. Create an employee before creating a user linked to that employee. Each employee can have one user account; temporary passwords must be 6–72 characters and are changed at first sign-in. Account changes revoke the affected user's other sessions.

On an installed, empty application schema with migration `0005_user_login_method.sql` applied, run `go run ./cmd/bootstrap-admin` from `backend` once to create the initial linked account. Its initial login is **SuperAdmin / SuperAdmin**; the account is required to change its password on first login. Do not retain the default password. The bootstrap command refuses to run when any user account already exists.

In **Users**, choose whether each account signs in with its username or email. Email login requires the employee to have an official email address. Selecting an employee fills the display name, email and suggested username; the username can be changed before saving.

These pages require the MySQL application tables to be provisioned. The MySQL schema migration remains blocked in this project, so setup and live database saves depend on the existing local database schema; do not use it for real financial records. Passwords are hashed and excluded from API responses and audit records.

Validation: frontend production build, Go tests and `go vet`, plus desktop/mobile Playwright tests for employee entry, user creation/editing, employee mapping, server errors and authenticated access. Live MySQL integration is available through `FINBOOK_MYSQL_INTEGRATION=1` with `FINBOOK_ROOT` set to the repository root.

## What is included

- React + TypeScript + Vite interface, Tailwind tokens, shadcn-compatible local controls, Radix accessible dialogs and menus, locally bundled Inter.
- Split-screen login with secure server sessions, password changes, show/hide password and password-recovery information. Preview routes remain public and contain no business data.
- Pale blue collapsible navigation, larger fonts, responsive mobile menu, light/dark preferences, breadcrumbs, screen search with Ctrl+K, branch/year setup placeholders, notifications and user menu.
- FRD navigation with Overview directly under VOLTA FinBook, highlighted module labels, and normal-weight process links. The sidebar company/setup card, WORKSPACE heading and guide link have been removed. Dashboard values remain unavailable until ledger-backed reporting is connected.
- International Sales has **Manual entry** and **MIR import** tabs. Enter and lock a local MIR folder once; filter by inclusive From/To file-modified dates, choose a record and Proceed to populate source-backed fields. The Go API calls the existing deterministic Python parser. See [local MIR setup and verification limits](docs/mir-local-source.md).
- Per-screen search/status filters and saved view preferences stored in this browser. Empty, loading and recoverable error states. No financial records, passwords or sessions are saved in browser storage.
- Cash & bank → Payments → **Add record** opens an editable payment preview: payment number placeholder, payment date, posting date, Draft status, bank/cash account, Transfer/Cheque options, embedded multi-line entries, narration and exact INR totals. Masters → Bank & cash accounts has an **Add payment** shortcut. Save/post remain disabled until the database, account mapping, permissions, audit and idempotency controls exist.
- Go HTTP service with local MySQL configuration, verified TLS for remote connections, health/status endpoints, locked MIR-folder configuration, timeouts and graceful shutdown. MySQL schema migrations and financial writes remain disabled until the PostgreSQL integrity controls are ported.

TanStack Table, Recharts, real transaction forms and financial database operations arrive with their feature phases. There is no sample ledger or fake dashboard total.

## Run on Windows in VS Code

Prerequisites: Go 1.26 or newer and Node.js 22.12 or newer (a current Node LTS is suitable), including npm. Install Node from [nodejs.org](https://nodejs.org/en/download) if `node --version` is not recognized. Reopen VS Code after installation so its terminals pick up PATH. Go is already installed at `C:\Program Files\Go` on this workstation.

1. Open `D:\Projects\Volta-Tours-Finance` in VS Code using **File → Open Folder**.
2. Open **Terminal → New Terminal**, using PowerShell. Run:

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance\frontend'
node --version
npm.cmd install
npm.cmd run dev
```

3. Open **http://127.0.0.1:5173** in your browser. Choose **Explore interface preview** on the login screen. You do not need a username or password. Keep this terminal running; Ctrl+C stops it.
4. Open a second terminal for the Go API:

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-api.ps1
```

This command uses the installed Go path even when Go is missing from PATH. The execution-policy flag applies only to that PowerShell process; it does not change the machine's policy. Alternatively run `go run ./cmd/api` from `backend` if Go is on PATH. Database connectivity is optional for the interface preview; configure MySQL using the [local setup guide](docs/mysql-local-setup.md). Use Ctrl+C to stop the API.

Check the running API in a third terminal:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:5173/api/v1/status
```

Expected: health `status: ok`; service status `phase: 2`, `entry_mode: manual_and_mir_preview`, `mir: local_folder`, `authentication: not_implemented`. When configured, a reachable MySQL instance reports `connected_schema_pending`; migration and readiness cannot complete until the MySQL schema port is implemented. `/readyz` returns 503. The API must run for database status and MIR configuration/fetching.

VS Code also provides **Terminal → Run Task → FinBook: Frontend / Go API / Check API / Build frontend** after npm installation.

## Test Phase 1

Backend tests and static checks:

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-api.ps1
```

Frontend build and browser smoke tests (internet is needed to install dependencies and the test browser):

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance\frontend'
npm.cmd run build
npx.cmd playwright install chromium
npm.cmd run test:e2e
```

The Playwright configuration starts Vite automatically or reuses a running development server. Tests cover login validation without credential transmission, keyboard search, every navigation destination, persistent theme/saved views, error recovery and mobile navigation. Go rule tests will replace pytest for tax, margin, ageing, posting and duplicate prevention as those Go features are built. Browser transaction tests will be added with working flows.

After the local MIR/sidebar update, the complete Edge desktop/mobile suite passed **31 tests with 1 desktop-inapplicable mobile test skipped**. The Go folder-configuration, filtering, source-change and duplicate-source tests also passed. The build and TypeScript checks passed. MIR browser tests use synthetic source responses; the existing Windows Store Python runtime could not launch in this sandbox, so real-file parser execution remains unverified. See [local MIR setup](docs/mir-local-source.md).

Verification on this workstation: Go tests, `go vet`, and backend compilation passed during the foundation work. A live Go server returned HTTP 200 for health and status and HTTP 404 for a sales write attempt. Node.js/npm and dependencies are now installed, `package-lock.json` exists, and the full TypeScript check and Vite production build passed after the payment UI update. The Edge desktop/mobile suite passed all 21 applicable checks across the initial run and corrected reruns (one desktop-inapplicable mobile-navigation check skipped). Two test-harness issues were corrected: an ambiguous status locator and the timeout for visiting every FRD destination. Desktop/mobile screenshots were reviewed with no document-width overflow. The build reports non-blocking dependency annotation and bundle-size warnings.

Manual review:

1. Open login, submit empty fields and check the messages. Toggle password visibility. Forgot password explains that accounts are not yet configured. Valid input never authenticates or sends credentials.
2. Explore the preview. Use Ctrl+K, type `International Sales entry`, and press Enter. Escape closes dialogs. Tab and Shift+Tab navigate controls.
3. Collapse the sidebar, switch theme, and refresh. Preferences persist. At a narrow browser width, open the mobile navigation; Escape closes it and returns focus.
4. Open a master screen, set a status filter, and save a named view. Reload and apply the view. Views are isolated by screen and stored only in this browser.
5. Check `http://127.0.0.1:5173/app/masters/customers?state=loading` and `?state=error` to review states. **Try again** returns to the empty screen. These query parameters are preview fixtures, not backend data states.
6. Confirm financial action buttons are disabled, balances show unavailable `— INR`, and roles/branches/years are not fabricated. Review International → Sales entry in both Manual entry and MIR import modes. [MIR setup](docs/mir-local-source.md) includes the working Python-runtime requirement and source-folder locking instructions.
7. Open **Cash & bank → Payments → Add record**. Select Bank, then Transfer or Cheque. Cheque shows cheque number/date; Cash hides bank-specific fields. Add two payment lines with `0.10` and `0.20`: the total must be `0.30 INR`. Remove a line, enter narration and use **Validate payment** to check required fields. Save/post are intentionally unavailable; validation does not save data. Cancel asks before discarding a modified form.

To run browser tests with the installed Microsoft Edge instead of downloading Chromium:

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance\frontend'
$env:PLAYWRIGHT_CHANNEL = 'msedge'
npm.cmd run test:e2e -- --workers=2
```

## Configuration and phase boundaries

The PostgreSQL access-check scripts and prior connection report are retained for historical reference only; they do not configure or validate the current MySQL backend. Follow [local MySQL setup](docs/mysql-local-setup.md) for the active database setup.

- Go configuration reads `DATABASE_URL`, then `DATABASE_CREDENTIAL_FILE`, then `backend\.local\mysql.url`. Keep credentials on the server; `backend/.env.example` is documentation, not an automatic dotenv loader. Never put a DB password in a `VITE_` variable or source control.
- One company. Branches, financial years, roles and thresholds remain unconfigured until setup. No permissive role defaults are assumed.
- Proposed invoice timing: generate the official invoice **after successful posting** through a separate action, with its own idempotency protection. Phase 1 does not generate numbers, invoices, entries or audit events.
- All tax rules: **to be verified by finance**. No rates or applicability rules are embedded. Finance approval and effective dates must be part of later configuration.
- Unknown legacy meanings (CN?, CD, Credit, Sub A/C, Matching, Knock-off, Transfer and Copy) are not implemented as actions or financial rules. The cash/bank transfer navigation is only an empty screen.
- No financial write endpoints exist yet. Future phases must enforce validation, audit logging, authorization, atomic posting and idempotency before enabling writes. Posted records use reversal/adjustment instead of editing or deletion.

## Source and architecture

`FRD/VOLTA_Functional_Requirements_Sales_Text_Only.docx` contains v1.1 and the v1.0 baseline in Appendix A. Your explicit phase sequence and later instructions take precedence over the FRD's suggested phases and original MIR priority.

```text
frontend/     React interface, design tokens and Playwright smoke tests
backend/      Go HTTP service, MySQL connection layer and PostgreSQL legacy migrations
scripts/      Windows launch and verification scripts
docs/         Phase decisions and design-system reference
FRD/          Original requirement document, unchanged
.vscode/      Windows development tasks
```

Frontend implementation references: [Vite](https://vite.dev/guide/), [Tailwind Vite integration](https://tailwindcss.com/docs/installation/using-vite), [shadcn Vite setup](https://ui.shadcn.com/docs/installation/vite), [React Hook Form](https://react-hook-form.com/get-started), [Playwright web server](https://playwright.dev/docs/test-webserver).

Phase 2 source now includes the normalized schema, immutable/audited source models, posting integrity constraints, reporting views, Go integration and migrations. Cloud SQL execution and migration application remain pending until the completion command succeeds. The local MIR integration is still read-only for ticket data; authenticated services, persistent corrections, master validation and duplicate checks against saved sales must be connected in later phases before enabling saves/posting. Go replaces FastAPI/SQLAlchemy/Alembic.
