# Event settlement exports (EXPORT-01)

Status: private finance export covers the closeout settlement, its append-only
corrections, and the private event finance ledger. It does not implement an
accounting-provider adapter, a public export, a server-generated PDF, or a
general event-data export. Markdown and printable HTML render the same private
snapshot.

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
one event report reference, its event settlement and append-only adjustment
rows, plus finance-line history and current finance totals. Rows are ordered by
creation time and identifier where applicable for deterministic output.

The CSV uses a header row, one `settlement` row, then one `adjustment` row per
correction, one `finance_line_history` row per retained ledger entry, and one
`finance_current_total` row per currency/type/direction group. It includes the
report, settlement, adjustment and finance identifiers; UTC RFC3339 timestamps;
currency; integer-cent totals and counts; settlement status/finalization
provenance; and finance labels, reasons, actor identifiers and correction links.
Currency stays separate from integer cents. Gross, adjustment, net and finance
current totals are never converted or formatted through floating point.

Gross, adjustment and net total columns are populated only on the single
`settlement` row. They are blank on `adjustment` rows, where
`adjustment_amount_cents` is the only row-level money amount. Finance history
uses `finance_amount_cents`; grouped current ledger rows use the separate
`finance_current_total_cents` column. This prevents a spreadsheet column sum
from repeating settlement or ledger totals on every history row.

It does not include ticket buyer addresses or names, contacts, private notes,
staffing, archive participants, payment-provider references, or message data.

## Spreadsheet safety

Untrusted text is encoded with Go's CSV writer for delimiter, quote, newline
and Unicode correctness. Before writing, values whose first non-whitespace,
non-control character is `=`, `+`, `-` or `@` receive a literal apostrophe.
This keeps spreadsheet software from evaluating a title, correction label or
reason as a formula, including when the original text begins with whitespace
or a control character.

## Finance-line print boundary

Finance lines remain private operator records. Download
`settlement-print.html`, open the self-contained snapshot locally, then use the
browser's Print / Save as PDF function when a PDF is needed. The snapshot
contains retained manual-finance history and grouped current totals from the
same repeatable-read export as the settlement. It does not create a server-side
PDF or public artifact. Recorded obligations remain separate from ticket
settlement net totals.

## Accounting format and PDF check

No external accounting format is required as of 2026-10-02, so EXPORT-01's
conditional "selected accounting export when required" criterion is not
triggered. Add an adapter only for a named accounting requirement.

Print / Save as PDF was checked on 2026-10-02 against a local T3 preview. A
fresh synthetic free event was created, published, given one budget line and
closed with end-of-night. Its `settlement-print.html` returned 200 with
`attachment; filename="event-settlement-print.html"`. The saved file was
opened from disk in headless Chromium (Playwright `page.pdf()`, A4, print
media). This produced a 2-page, 26,342-byte PDF. `pdftotext` showed the
"Settlement report", "Report identity", "Stored settlement", "Corrections",
"Budgets", "Recorded obligations" and "Manual recorded payments" headings.
The budget line and its USD 123.45 total were present. This is a local
synthetic check, not production or accounting-user qualification.

## Remaining EXPORT-01 work

Accounting-user validation and production qualification remain future work.
