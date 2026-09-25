# Cultural import preview

Status: partial IMPORT-01 implementation. A workspace owner or organizer can
persist a bounded, review-only preview through the private operator API. The
preview records the source assertion, parser schema and digest, normalized
allowlisted candidates, safe parser error codes and bounded canonical-match
hints. It has no file storage, remote API fetch, apply action, correction,
rollback action, consent change or public publication.

The parser is `backend/internal/culturalimport`. It is intentionally separate
from the application package. The API uses it before writing a staged preview;
an invalid source assertion prevents every write. It does not contact the
asserted source. A source assertion is input metadata, not proof of rights,
ownership or accuracy.

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

## Persisted review previews

`POST /api/workspaces/{workspaceID}/cultural-imports/preview` accepts the
three source-assertion fields and a `csv` string. The JSON request is bounded
at 2 MiB to accommodate JSON escaping of the parser's 256 KiB decoded CSV
limit and its metadata.
Only the `owner` and `organizer` roles have `manage_imports`; every other role,
including `finance`, `door`, `crew` and legacy `member`, is denied. The handler
sets `Cache-Control: private, no-store` and `X-Content-Type-Options: nosniff`.
It checks authority again before returning a saved result.

Migration 000017 persists the source ID, source name, source assertion,
parser schema, content SHA-256, actor and timestamp. It persists only
normalized allowlisted candidate fields and the parser's row/field/code errors;
it never stores the raw CSV bytes. Error responses and persisted parser errors
do not echo source assertions or cell text. Candidate descriptions are
allowlisted untrusted text, not error messages or a rights claim.

For every accepted candidate, the preview records same-workspace occurrences
whose UTC start instant and case-folded trimmed title match exactly. This is a
conservative review hint, not an identity decision: zero, one or several
results never create a link or update. At most 20 matches are returned and
stored. A candidate reports `ambiguous` for more than one possible match and
`matchesTruncated` when more than 20 exist, so a bounded result is never shown
as uniquely matched. Cross-workspace occurrences are excluded by the query and
database foreign keys. If a canonical occurrence is later deleted, the live
match reference becomes null without blocking that deletion; the bounded
occurrence ID, event ID, title, start, status and revision-timestamp snapshot remains as preview
provenance.

The header, candidates, parser errors and match hints are written in one
transaction. A preview with parser errors can be retained for review when its
source assertion is valid. An invalid source assertion is rejected before a
preview header or any child row is created.

## Applying an explicit action

`GET /api/workspaces/{workspaceID}/cultural-imports/{importID}` reloads a saved
preview. `POST /api/workspaces/{workspaceID}/cultural-imports/apply` accepts a
candidate ID, one explicitly selected workspace event, and either `create` or
`correction`. Matches remain hints: the API never derives an event, occurrence,
or place from a title or timestamp.

A create makes one occurrence in the selected existing event with `place_id`
null. It cannot create an event or place. A correction requires the selected
occurrence in that selected event, a nonempty allowlisted field set, and its
`expectedUpdatedAt`; `expectedPublicCid` is an optional additional CAS token.
Only name, description, start, end, timezone, and status are writable. Public
URI/CID values are preserved, and the occurrence revision advances
monotonically. A preview with parser errors requires `acknowledgeErrors: true`.

The action transaction locks the candidate, selected event, and selected
occurrence before mutation, then stores the source ID/digest, candidate row,
selected-field diff, before/after snapshots, expected tokens, actor, and time
in `cultural_import_actions`. The candidate has a unique ledger action, so a
second apply is refused. Authority is checked at request entry, the write
boundary, and before the response. The action never changes consent,
announcements, payments, providers, or publication state.

`POST /api/workspaces/{workspaceID}/cultural-import-actions/{actionID}/rollback`
can delete only a created occurrence whose revision is still the import
revision, whose public URI/CID are empty, which has no credits, and which is
not referenced by another active import action. The ledger action remains with
the rollback actor, time, and outcome. Corrections are never deleted; they need
an explicit compensating correction.

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

The parser tests cover normalized DST instants, source assertions, malformed and
sensitive headers, invalid UTF-8, size and row limits, formula-like text,
invalid time/status/zone values, duplicate source IDs, and errors that do not
contain input text. PostgreSQL integration coverage additionally exercises
explicit create/correction actions, stale-revision rejection, one-action
idempotency, create rollback, correction rollback refusal, and the 18-to-19
upgrade path. It does not prove a public publication or provider workflow.
