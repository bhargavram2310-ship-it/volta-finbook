# Phase 1 design system

Source of truth: `frontend/src/styles.css`. Components use semantic CSS custom properties, also exposed to Tailwind through `@theme inline`. Theme selection applies `data-theme` to the document root and persists as a browser preference.

| Token | Light | Purpose |
| --- | --- | --- |
| `--sidebar` | `#edf3fb` | Light blue navigation surface |
| `--sidebar-text` | `#2d425e` | Sidebar labels and brand text |
| `--sidebar-line` | `#d4dfed` | Sidebar dividers and borders |
| `--primary` | `#2563eb` | Main actions and focus context |
| `--background` | `#f5f7fb` | Workspace |
| `--surface` | `#ffffff` | Cards, tables, dialogs |
| `--foreground` | `#18263d` | Primary text |
| `--muted` | `#647187` | Secondary text |
| `--teal` | `#158574` | Positive/posted context |
| `--amber` | `#976315` | Pending/review context |
| `--red` | `#c33848` | Validation and cancelled/failed context |

Inter is bundled locally. All numbers use tabular figures; money columns align right and carry currency in headers and values. Unknown balances show `— INR`, never zero. Screen titles use 26–30 px, body text 15 px, navigation 13–14 px, and supporting metadata 11–13 px. The light theme uses a pale blue sidebar; the dark theme uses a softer slate sidebar with contrasting labels. Sidebar width is increased to accommodate larger labels. At narrow widths, grids stack and tables scroll horizontally. Dense layouts still retain visible focus and semantic labels.

Shared controls include Button (CVA/Slot), Radix-backed Dialog, Radix DropdownMenu, cmdk command navigation, StatusBadge and Empty/Loading/Error states. `components.json` supplies aliases for future shadcn controls. TanStack Table will be used for real sortable, paginated grids when data exists; Recharts will be introduced with ledger-backed reporting.

The bank/cash payment preview uses a wide dialog with an embedded line grid, sticky validation actions, document narration and conditional Transfer/Cheque details. Amounts are parsed as integer paise for exact INR addition. Draft status and the not-yet-assigned payment number are read-only. The form is transient: no business record, document number or ledger entry is created. Dirty forms require confirmation before discard.

The sidebar now places Overview immediately below the brand, omits the company/setup card and WORKSPACE heading, and removes its guide link. Module buttons use a bold label, pale blue background and blue left border; process links retain normal weight. International includes MIR import, and Sales entry includes keyboard-operable Manual entry/MIR import tabs. The picker displays a locked folder path, inclusive date filters, source records and Proceed. Populated fields carry MIR/Manual/Overridden badges with source line and correction-history access.

Status labels: Draft (neutral), Verified (blue), Approved/Posted (teal), Cancelled/Failed (red), Refunded (blue), Duplicate (amber). Status text accompanies color. These are presentation tokens, not permission or transition rules.

Ctrl+K opens search; arrow keys select a destination and Enter navigates. Radix handles dialog focus and Escape. Mobile navigation traps Tab and restores focus on Escape. The shell includes a skip link, route headings and breadcrumbs. Reduced-motion preference disables animations.

Saved views persist only the screen filter name, text query and selected status. Theme and sidebar preferences are also local. There are no browser-stored business records or auth tokens. Search values can contain user text: users should avoid real passenger identifiers during the preview, since a saved filter stores that query on this browser.
