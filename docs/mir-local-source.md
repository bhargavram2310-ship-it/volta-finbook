# MIR import with Go

The Go backend decodes MIR files directly using backend/internal/mir/decoder
and stages the results in MySQL. No external parser process or interpreter is
required. Run the API and React frontend using the commands in the main README.

## Import and review

1. Sign in and open Admin Control > MIR Report.
2. Upload MIR files. POST /api/v1/mir/import accepts up to 100 files, with a
   maximum size of 2 MiB per file and a 32 MiB request limit.
3. Alternatively, configure the source folder and click Pull from configured
   folder to manually extract its MIR files. Files stay in place, and repeated
   pulls identify already-imported records as duplicates. A pull supports up to
   100 regular MIR files and 30 MiB total; larger folders can use upload batches.
4. Filter staged records by MIR creation date (T50DTE). The first travel date
   (T50DFT) is stored separately.
5. Open a record to review its decoded sections, linked children, source bytes,
   warnings, and parser coverage. Unsupported or ambiguous layouts retain their
   original bytes and explicit issues rather than guessed values.

The import, folder-pull, listing, detail, and reprocessing endpoints require an authenticated
session. Source hashes identify duplicate imports. MIR staging does not create
sales, invoices, or financial postings.

## Optional folder setting

Admin Control > MIR File Reader Location retains the existing source-folder
setting. The path is stored in backend/.local/mir-source.json, excluded from
Git. The folder must exist on the API machine. Saving the same path is
idempotent; another path cannot replace the locked setting through the app.

Folder configuration is restricted to loopback clients and recognized local
origins. It is required for manual folder pulls and optional for uploads. The
retired folder-date preview and Proceed endpoints have been removed.

## Sales GDS Capture

Open Sales Workspace, choose a Sales type, and open GDS Capture. Choose From Date, To Date, and
either Processing Date (the default, first extraction timestamp in UTC) or
Issue Date (source A02DTE), then click Show extracted data. Both dates are
inclusive; no unfiltered list is loaded before the user applies a range. An import
can have older ticket issue dates, so Processing Date should be used to find
tickets extracted today. Empty Issue Date results offer a Processing Date action.

The grid shows one row per extracted passenger/ticket, with Ticket No, PNR No,
GDS, Staff, Client Reference, INV No, Branch, PAX Name, Issue Date, Sector,
Web Ref, Fare, and Proc Date. Visible columns can be toggled; only column
preferences are stored in browser storage. Search and pagination run on the API.

Staff and Branch show source ticketing-agent sign and ticketing-office PCC,
without guessing employee or branch-master mappings. Fares and sectors use
explicit passenger relationships. Client Reference is shown only when the
record has one unique customer-file reference. Unmapped Web Ref stays blank.
Missing or invalid issue dates are excluded from Issue Date filtering and can
still be reviewed using Processing Date. Capture is read-only and creates no
sales or financial postings.

Select one ticket using its radio button, then click Proceed to open the same
manual entry form with the selected passenger's source values. The API rechecks
the passenger and record identity and uses explicit fare/segment associations;
selections replaced by reprocessing must be refreshed. Ticket number, passenger,
PNR, airline, issue date, linked travel dates/sectors, and linked fare components
are populated when available. Exact unique airline/currency master matches are
selected automatically. Missing customer, supplier, and commercial amounts remain
editable and blank. Extracted values retain source lines, field identifiers, and
raw-byte evidence; corrections require a reason.

Proceed creates an unsaved in-memory draft. Selecting another ticket asks before
replacing existing unsaved values, while proceeding again with the same ticket
preserves corrections and entered accounting fields. Applying a new filter or
page clears the radio selection. No sales document is saved or posted by Proceed.

## Verification

Existing installations that have the older application tables without migration
history can install the separate MIR staging tables using the guarded setup
command. From `backend`, run `go run ./cmd/setup-mir -mode plan` to review the
missing tables, then `go run ./cmd/setup-mir -mode apply`. It creates only new
MIR staging tables and refuses partial staging schemas. Existing application
tables and foundation migration history are not rewritten. Installations with
recognized migration history use the normal migration runner through this
command.

Run go test ./... from backend for decoder, staging, configuration, and API
tests. To exercise imports against a temporary MySQL server without changing the
application database, run scripts/test-mir-isolated.ps1 from the repository root
with PowerShell and its process-scoped ExecutionPolicy Bypass option.

The isolated tests cover synthetic file imports, duplicate detection, MIR and
travel dates, exact decimal values, typed children, raw-byte preservation,
authentication, and reprocessing. See mir-staging-report.txt for detailed
coverage and known format limitations. Passing these checks does not establish
complete support for every MIR variant.
