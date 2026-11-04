# Local MIR folder and International Sales

This update re-enables MIR selection alongside manual entry at the user's request. It does not enable sales persistence, approval, invoicing or posting.

## Use the screens

1. Run the Go API and React frontend using the commands in the main README.
2. Open **International → Sales entry → MIR import**, or **International → MIR import**.
3. Enter an absolute local directory path on the machine running the Go API, for example `D:\MIR\Incoming`.
4. Choose **Save & lock folder**, review the path, then **Confirm & lock folder**. The directory must already exist and be accessible to the Go process.
5. Choose **From date** and **To date**, then **Fetch MIR files**. Both dates are inclusive. The filter uses **file modification dates in the API machine's local timezone**, not a guessed ticket issue/travel date. Subfolders are not scanned.
6. Select one ticket row, then **Proceed to sales entry**. The file hash is rechecked and the original file reparsed before the selected passenger's values are populated. Failed and duplicate rows cannot proceed.
7. Review missing values and source issues. Open **View source & issues** for raw text, original line bytes, and SHA-256. MIR fare total is kept separate from client total and supplier total; commercial prices and tax treatments are not inferred.
8. To correct a source field, click its pencil, enter a new value and a reason. Its badge becomes **Overridden**. Click its field badge to inspect original, previous and new values, time and reason. Repeated Proceed for an already-loaded ticket preserves its values and corrections.

## Locked configuration

The path is saved in `backend/.local/mir-source.json`, excluded from Git. The configuration records its creation time and lock event, survives API/browser restarts, and has no HTTP edit/delete/unlock operation. Submitting the same path again is idempotent; a different path is rejected. No source files are moved, renamed or deleted. This JSON file is application configuration, not a local transaction database.

MIR endpoints are restricted to loopback clients and recognized local application origins until real authentication/permissions are implemented. Keep the API bound to its default `127.0.0.1:8080`. This is a local preview integration, not an authenticated production deployment. Root configuration is resolved from `FINBOOK_ROOT`, or the repository root when the API is launched from `backend` using the supplied script.

## Existing parser runtime

The Go backend calls `backend/integrations/mir_bridge.py`, which imports the existing **unchanged** `parser.parse_mir` from `D:\Projects\IDC-MIR-Reader`. The bridge only maps parsed, passenger-linked fields into the UI contract. There is no replacement MIR parser and no LLM extraction.

Default interpreter: `D:\Projects\IDC-MIR-Reader\.venv\Scripts\python.exe`. If that environment is unavailable, set a working Python 3.10+ executable before starting the API:

```powershell
cd 'D:\Projects\Volta-Tours-Finance'
$env:MIR_PYTHON = 'C:\Path\To\Working\python.exe'
$env:MIR_PARSER_ROOT = 'D:\Projects\IDC-MIR-Reader'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-api.ps1
```

Replace the example interpreter path with a real executable. The parser itself uses only Python's standard library; Streamlit and pandas are not imported by this bridge. A regular Python installer or portable runtime is available from [Python's official Windows downloads](https://www.python.org/downloads/windows/). No global Python setting is changed by this project.

On this workstation, the existing virtual environment points to a Windows Store Python executable that could not be launched from the current execution sandbox. A portable-runtime download attempt was also blocked by network connectivity. **Live parsing with that runtime remains unverified.** The UI and Go folder/date/hash behavior have separate automated tests with controlled synthetic parser output. This limitation must not be described as a successful real-file integration test.

## Data and limits

- File extension: `.MIR`, case-insensitive; regular files directly inside the locked directory only. Symbolic file links are rejected.
- Limits: 2 MB per file, 100 matching files, 500 ticket rows per fetch. Narrow the range if a limit is exceeded. Parser output is bounded; request timeout is 60 seconds and each Python invocation is limited to 10 seconds.
- Identity: original file SHA-256 + parser record + passenger item. Identical source records are flagged as duplicates in a fetch. Proceed is read-only and cannot create a sale or posting.
- Source file changes between fetch and Proceed produce a conflict and require refetching.
- Only uniquely parsed fields are mapped. Unrecognized layouts, unsupported financial representations and parser issues remain visible; missing amounts stay blank.
- No whole-party fare is assigned to one passenger. No INR conversion, tax rate, commission, client sale total or supplier cost is invented.
- Source evidence remains in the original file and the unsaved sales state. Corrections are transient until database-backed saving and immutable audit controls are implemented; reloading/leaving the app discards the unsaved ticket. No passenger data or credentials are written to browser local storage.

## Validate on Windows

```powershell
cd 'D:\Projects\Volta-Tours-Finance'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-api.ps1
cd frontend
npm.cmd run build
$env:PLAYWRIGHT_CHANNEL = 'msedge'
npm.cmd run test:e2e -- tests/international-mir.spec.ts --workers=2
```

Go tests check configuration persistence, path locking, inclusive dates, file extension filtering, traversal rejection, source-hash conflicts and local access restrictions. Browser tests check the sidebar, locked source UI, date filtering, selection, population, unchanged financial blanks, source evidence, corrections and repeated Proceed on desktop and mobile. Browser fixtures never read real passenger files or alter your actual locked folder.
