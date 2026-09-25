# Event settlement exports (EXPORT-01)

Status: private finance export implemented as the first bounded EXPORT-01
slice. It exports the existing closeout settlement and its append-only
corrections. It does not implement a budget, accounts payable, an accounting
provider adapter, a public export, server-generated PDF, or a general event-data
export. Markdown and printable HTML render the same private settlement snapshot.

## Route and access

`GET /api/events/{eventID}/exports/settlement.csv` returns an attachment named
`event-settlement.csv` with `Content-Type: text/csv; charset=utf-8`,
`Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`.

`GET /api/events/{eventID}/exports/settlement.md` downloads a UTF-8 Markdown
report. `GET /api/events/{eventID}/exports/settlement-print.html` downloads a
self-contained printable HTML report. Open the HTML locally and use the
browser's Print / Save as PDF function when needed. This is not a server PDF
endpoint. Both formats preserve report, settlement and correction identifiers,
actor IDs, UTC dates, finalization status and exact monetary totals. Markdown
escapes untrusted text; HTML uses automatic template escaping and a restrictive
content security policy with no remote assets or scripts.

Every route requires the workspace `finance` permission. The current authority
matrix gives that permission to `owner` and `finance` roles. Organizer, crew,
door, inactive, revoked and unauthenticated sessions are denied. The routes are
not public and no public projection reads its data.

## Contents and consistency

The export is generated in a read-only repeatable-read transaction. It reads
one event report reference, its event settlement, and the settlement's
append-only adjustment rows. Adjustment rows are ordered by creation time and
identifier for deterministic output.

The CSV uses a header row, one `settlement` row, then one `adjustment` row per
correction. It includes the report, settlement and adjustment identifiers;
UTC RFC3339 timestamps; currency; integer-cent totals and counts; settlement
status/finalization provenance; and each adjustment's amount, label, reason,
creator identifier and timestamp. Currency stays separate from integer cents,
and the export includes gross, adjustment and net totals without rounding or
formatting money through floating point.

Gross, adjustment and net total columns are populated only on the single
`settlement` row. They are blank on `adjustment` rows, where
`adjustment_amount_cents` is the only row-level money amount. This prevents a
spreadsheet column sum from counting a settlement total once per correction.

It does not include ticket buyer addresses or names, contacts, private notes,
staffing, archive participants, payment-provider references, or message data.

## Spreadsheet safety

Untrusted text is encoded with Go's CSV writer for delimiter, quote, newline
and Unicode correctness. Before writing, values whose first non-whitespace,
non-control character is `=`, `+`, `-` or `@` receive a literal apostrophe.
This keeps spreadsheet software from evaluating a title, correction label or
reason as a formula, including when the original text begins with whitespace
or a control character.

## Remaining EXPORT-01 work

The current stored model contains actual paid-ticket settlement counts and
corrections. It does not contain a budget or payable-obligation ledger, so the
reports make no budget/payable claim. A selected accounting format, server PDF
generation, any separate operational report scope, accounting-user validation
and production qualification remain future work.
