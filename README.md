# VOLTA FinBook

Travel-agency sales, operations and accounting ERP. Employee and user master changes are supported when the MySQL application schema is provisioned. Financial entry, persistence and posting remain disabled.

**MySQL status:** the Go API supports local MySQL 8.4, and the migration runner applies the versioned MySQL scripts while checking migration names and checksums. The MySQL schema is a foundation, not a fully verified financial ERP: posting and financial writes remain unavailable. Apply migrations only to a new empty application database, then verify the schema before using master-data features. Do not use the database for real financial records. See [local MySQL setup](docs/mysql-local-setup.md). The old PostgreSQL foundation is retained as [legacy documentation](docs/phase-2-database.md).

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\setup-mysql-local.ps1
```

## Shared sales workspace

Sales Workspace combines International, Domestic and Miscellaneous entry. Choose
the Sales type above the Manual entry / GDS Capture tabs. Classification stays
with the unsaved draft when switching tabs or loading a captured ticket.
Miscellaneous entry uses service and booking labels. Names and active availability
are maintained in Masters > Sales types. Switching type preserves entered values.
Existing category-specific entry links redirect to the shared workspace.

For an installation without migration history, run
`go run ./cmd/setup-sales-types -mode plan` from `backend`, then use `-mode apply`
to add the new master and its three defaults. Edited masters are not reseeded on
later setup runs. Installations with recognized history use the migration runner
through this command. Financial saving and posting remain unimplemented.

## Employee and user setup

The **Admin Control → Employees**, **Users**, and **Roles** pages let signed-in users maintain employee records, login accounts, and named roles. Create roles and assign multiple roles to a user; roles are labels only and do not grant application permissions. Create an employee before creating a user linked to that employee. Each employee can have one user account; temporary passwords must be 6–72 characters and are changed at first sign-in. Account changes revoke the affected user's other sessions.

After applying MySQL migrations to a new empty application schema, run `go run ./cmd/bootstrap-admin` from `backend` once to create the initial linked account. Its initial login is **SuperAdmin / SuperAdmin**; the account is required to change its password on first login. Do not retain the default password. The bootstrap command refuses to run when any user account already exists.

In **Users**, choose whether each account signs in with its username or email. Email login requires the employee to have an official email address. Selecting an employee fills the display name, email and suggested username; the username can be changed before saving.

These pages require the MySQL application tables to be provisioned. Passwords are hashed and excluded from API responses and audit records. Financial entry, persistence and posting remain disabled.

Validation: frontend production build, Go tests and `go vet`, plus desktop/mobile Playwright tests for employee entry, user creation/editing, employee mapping, server errors and authenticated access. Live MySQL integration is available through `FINBOOK_MYSQL_INTEGRATION=1` with `FINBOOK_ROOT` set to the repository root.

## What is included

- React + TypeScript + Vite interface, Tailwind tokens, shadcn-compatible local controls, Radix accessible dialogs and menus, locally bundled Inter.
- Split-screen login with secure server sessions, password changes, show/hide password and password-recovery information. Preview routes remain public and contain no business data.
- Pale blue collapsible navigation, larger fonts, responsive mobile menu, light/dark preferences, breadcrumbs, screen search with Ctrl+K, branch/year setup placeholders, notifications and user menu.
- FRD navigation with Overview directly under VOLTA FinBook, highlighted module labels, and normal-weight process links. The sidebar company/setup card, WORKSPACE heading and guide link have been removed. Dashboard values remain unavailable until ledger-backed reporting is connected.
- Admin Control > MIR Report provides manual folder pulls, uploads, source details and reprocessing. Sales Workspace combines International, Domestic and Miscellaneous entry with Manual entry and GDS Capture tabs; GDS Capture filters extracted passenger/ticket rows by Issue Date or Processing Date and supports column visibility preferences. MIR decoding runs in Go. See [MIR setup](docs/mir-local-source.md).
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

After the local MIR/sidebar update, the complete Edge desktop/mobile suite passed **31 tests with 1 desktop-inapplicable mobile test skipped**. The Go folder-configuration, filtering, source-change and duplicate-source tests also passed. The build and TypeScript checks passed. MIR browser tests use synthetic source responses. The native Go decoder and MySQL staging are covered by Go tests and the isolated integration script. See [MIR setup](docs/mir-local-source.md).

Verification on this workstation: Go tests, `go vet`, and backend compilation passed during the foundation work. A live Go server returned HTTP 200 for health and status and HTTP 404 for a sales write attempt. Node.js/npm and dependencies are now installed, `package-lock.json` exists, and the full TypeScript check and Vite production build passed after the payment UI update. The Edge desktop/mobile suite passed all 21 applicable checks across the initial run and corrected reruns (one desktop-inapplicable mobile-navigation check skipped). Two test-harness issues were corrected: an ambiguous status locator and the timeout for visiting every FRD destination. Desktop/mobile screenshots were reviewed with no document-width overflow. The build reports non-blocking dependency annotation and bundle-size warnings.

Manual review:

1. Open login, submit empty fields and check the messages. Toggle password visibility. Forgot password explains that accounts are not yet configured. Valid input never authenticates or sends credentials.
2. Explore the preview. Use Ctrl+K, type `International Sales entry`, and press Enter. Escape closes dialogs. Tab and Shift+Tab navigate controls.
3. Collapse the sidebar, switch theme, and refresh. Preferences persist. At a narrow browser width, open the mobile navigation; Escape closes it and returns focus.
4. Open a master screen, set a status filter, and save a named view. Reload and apply the view. Views are isolated by screen and stored only in this browser.
5. Check `http://127.0.0.1:5173/app/masters/customers?state=loading` and `?state=error` to review states. **Try again** returns to the empty screen. These query parameters are preview fixtures, not backend data states.
6. Confirm financial action buttons are disabled, balances show unavailable `— INR`, and roles/branches/years are not fabricated. Review International → Sales entry in both Manual entry and GDS Capture modes. [MIR setup](docs/mir-local-source.md) describes the Go import workflow and optional source-folder configuration.
7. Open **Cash & bank → Payments → Add record**. Select Bank, then Transfer or Cheque. Cheque shows cheque number/date; Cash hides bank-specific fields. Add two payment lines with `0.10` and `0.20`: the total must be `0.30 INR`. Remove a line, enter narration and use **Validate payment** to check required fields. Save/post are intentionally unavailable; validation does not save data. Cancel asks before discarding a modified form.

To run browser tests with the installed Microsoft Edge instead of downloading Chromium:

```powershell
Set-Location 'D:\Projects\Volta-Tours-Finance\frontend'
$env:PLAYWRIGHT_CHANNEL = 'msedge'
npm.cmd run test:e2e -- --workers=2
```

## Sales Workspace ticket review

Sales Type is in the compact page header. International and Domestic have GDS Capture, Entry, Refund and Cancellation tabs. Miscellaneous has Entry, Refund and Cancellation because the current GDS pipeline extracts airline tickets. Cancellation retains its name: the ticket schema distinguishes cancellation from voiding; these processing workflows remain unconnected.

GDS Capture uses **Search Tickets** with Imported Date or Ticket Issue Date, From/To dates, prefix search for ticket/PNR/passenger, Reset, and 25-row server pagination. Configure `BUSINESS_TIMEZONE` on the Go API with an IANA name such as `Asia/Kolkata`. Its default is UTC, preserving the previous convention. The UI shows the server timezone and local current date. Imported-date filters convert inclusive local dates into a half-open UTC timestamp range, including daylight-saving transitions. Issue dates remain date-only source values. The existing API value `date_basis=processing` and its UTC `processing_date` response field remain compatible; `imported_at` is an explicit UTC timestamp used for business-timezone display.

Primary columns include ticket, passenger, PNR, GDS, route, issue date and imported fare, followed by status and actions. Visible Columns exposes secondary source fields and remembers choices in session storage. Amount is explicitly identified as imported fare and currency, never inferred as client revenue or supplier cost. Ticket links open the existing dialog pattern with passenger/itinerary details, linked fare/taxes, validation issues, customer-mapping information and any existing sale.

Statuses come from stored extraction validation, required source fields, active airline/currency mappings, and actual ticket-to-sale links. **Ready for Sale** means the source can populate a draft; staff still review customer and supplier accounting in Entry. **Needs Review** lists missing/invalid source information. **Sale Created** reflects an existing `sales_lines` / `sales_documents` link. Loading another draft for a sold ticket returns HTTP 409. Existing unique keys on canonical ticket numbers and original ticket sales remain the write-time duplicate protection, including cancelled and soft-deleted documents. No schema changes or financial write endpoints are introduced.

Draft preparation preserves the existing authenticated read-only policy. Viewing a saved sale additionally requires an active assigned role with the explicit permission `module=sales`, `screen=workspace`, `action=view`; no grants are seeded. Save, approval and posting remain disabled because those backend workflows are not connected. Create Sale opens the existing prefilled, unsaved draft for review and does not persist a sale. Automatic customer mapping is not inferred from an unverified source client reference. Bulk sale creation is not exposed.

Verification includes the existing Go suite, isolated MySQL MIR integration tests for timezone boundaries, status/master validation, permissions and duplicate protection, and Playwright desktop/mobile checks for the header, conditional tabs, columns, filters, loading/retry/reset, details and draft handoff. Layout screenshots are written to `.local/sales-workspace-desktop.png` and `.local/sales-workspace-mobile.png` by the browser tests. `go run ./cmd/capture-schema` inspects only relevant table metadata for compatibility checks.

## Reconciliation statement uploads

Open **Upload for reconciliation** in the sidebar. Choose Bank or Airline / supplier, select a searchable PDF, Excel `.xlsx` workbook, or UTF-8 CSV, then choose **Upload, extract & save**. Older `.xls` workbooks must be saved as `.xlsx`; scanned PDFs require OCR before upload. Limits: 10 MiB per file, 10,000 nonempty rows, 256 columns, and 200 PDF pages.

The authenticated API saves the original file, its SHA-256, uploader, statement type, and extracted source rows in `reconciliation_statements` and `reconciliation_statement_rows`. Excel worksheets and PDF pages retain source locations. Headers and balances are preserved; dates and money are not inferred or rounded. PDF column boundaries are based on text positions and need review. Uploading prepares source data for reconciliation; transaction matching and ledger posting are separate work. Identical file bytes return the existing import, even if renamed. Parent records and extracted rows are inserted in one transaction.

From `backend`, run `go run ./cmd/setup-reconciliation -mode plan` to inspect setup, then `go run ./cmd/setup-reconciliation -mode apply` to add the tables. This supports the existing installation without foundation migration history; tracked databases use the normal migration runner. Restart the Go API after updating its source. The screen lists the latest 50 imports and reads saved rows in pages of 100.

Verification: `go test ./...`, `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-reconciliation-isolated.ps1` from the project root, and `npx.cmd playwright test tests/reconciliation-upload.spec.ts` from `frontend`. The isolated MySQL script creates a temporary server and database, exercises authenticated PDF/Excel/CSV uploads, duplicate detection and rollback, then removes its test data.

## Configuration and phase boundaries

The PostgreSQL access-check scripts and prior connection report are retained for historical reference only; they do not configure or validate the current MySQL backend. Follow [local MySQL setup](docs/mysql-local-setup.md) for the active database setup.

- Go configuration reads `DATABASE_URL`, then `DATABASE_CREDENTIAL_FILE`, then `backend\.local\mysql.url`. Keep credentials on the server; `backend/.env.example` is documentation, not an automatic dotenv loader. Never put a DB password in a `VITE_` variable or source control.
- Preview, inspect or apply MySQL migrations from `backend` with `go run ./cmd/migrate -mode plan`, `go run ./cmd/migrate -mode status`, or `go run ./cmd/migrate -mode apply`. The migration runner refuses untracked existing application tables and stops on incomplete migrations; because MySQL DDL is not transactional, an interrupted migration requires deliberate recovery rather than an automatic rerun.
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
