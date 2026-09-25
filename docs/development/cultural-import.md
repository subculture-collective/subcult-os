# Cultural import preview

Status: partial IMPORT-01 implementation. The current parser validates one
bounded CSV dialect in memory. It has no HTTP route, database table, migration,
file storage, remote API fetch, canonical matching, apply action, rollback
action, consent change, or public publication.

The parser is `backend/internal/culturalimport`. It is intentionally separate
from the application package so a preview cannot create an event, occurrence,
place, profile, consent grant, outbox row, or audit entry. It does not contact
the asserted source. A source assertion is input metadata, not proof of rights,
ownership, or accuracy.

## Preview contract

`PreviewCSV` accepts a `SourceAssertion` with a source ID, source name, and
free-text assertion. All three are required and bounded. The result contains
the normalized source ID, a SHA-256 digest of the supplied bytes, accepted
candidates, and safe validation errors. The result has no database identity or
canonical match claim.

The accepted dialect is `subcult-occurrence-csv/v1`. Its header must match this
ordered list exactly:

```text
source_record_id,title,description,starts_at,ends_at,timezone,status,venue_name,locality,region,country
```

The parser rejects duplicate, unknown, reordered, and sensitive extra headers.
It also rejects malformed CSV, non-UTF-8 input, input larger than 256 KiB, and
more than 200 data rows. A malformed row or an over-limit input invalidates the
whole preview. Row-level field errors preserve other valid candidates, but a
later apply workflow must require review of every reported error.

The allowlisted fields describe a cultural occurrence only. They do not include
an email address, phone number, street address, access notes, attendee or staff
data, ticket or payment data, private notes, account IDs, consent, marketing
permission, or tokens. The parser emits structured JSON values. It does not
write a spreadsheet. It rejects text beginning with `=`, `+`, `-`, or `@`,
Unicode format characters, NUL, and other controls. Description alone may
contain ordinary newlines and tabs.

`starts_at` is required RFC 3339 input. `ends_at` is optional RFC 3339 input,
but when present it must be after `starts_at`. `timezone` is required, must be
an IANA identifier available from the embedded zone database, and cannot be
`Local`. `status` is one of `scheduled`, `rescheduled`, `postponed`, or
`cancelled`. The text limits are 200 characters for the source record ID, 1,000
for title, 12,000 for description, 600 for venue, and 400 each for locality
and region. Country is optional but, when supplied, must be two ASCII letters.
The preview trims text, normalizes timestamps to UTC RFC 3339, and uppercases
country.

Repeated `source_record_id` values are not silently merged. Every repeated row
receives a `duplicate_source_record_id` review error, and none of that source
ID's rows are returned as candidates, including a valid row paired with an
invalid one. The parser cannot identify a
duplicate or ambiguous canonical event because it has no database access.

Errors contain a row number, field name when applicable, and a stable code.
They never echo CSV text, source assertions, or parse-library messages.

## Remaining IMPORT-01 work

A later staged-import slice must persist the source assertion, normalized rows,
review decisions, and source provenance. It must define a workspace-scoped
importer permission and prevent cross-workspace target references. It must
show exact source-record duplicates and possible canonical matches for human
review without automatic merging.

An apply slice must create or correct canonical records only after an operator
chooses each candidate. It must not silently overwrite a canonical event. A
correction needs its target ID, a field diff, and the target's current revision
token. It must record append-only provenance.

A rollback slice must distinguish a newly created, unchanged and unreferenced
record from a record with later edits or dependent work. It may delete only the
former. Other outcomes need an explicit compensating correction or
cancellation. Neither importing nor correction may create consent, schedule an
announcement, or publish a public record.

A remote API adapter is out of scope until the operator selects a source and
defines its authority, credential, URL, rate-limit, pagination, failure, and
SSRF boundaries. The extraction manifest retains old importer behavior only as
fixtures; new implementation stays OS-native.

## Verification

Run the pure parser tests with:

```sh
cd backend
go test -race ./internal/culturalimport -count=1
```

The tests cover normalized DST instants, source assertions, malformed and
sensitive headers, invalid UTF-8, size and row limits, formula-like text,
invalid time/status/zone values, duplicate source IDs, and errors that do not
contain input text. These tests prove parser behavior only. They do not prove a
staged import, apply, correction, rollback, browser journey, provider flow, or
production qualification.
