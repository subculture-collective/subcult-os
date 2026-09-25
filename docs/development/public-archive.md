# Private archive approvals

The archive approval ledger is the first source slice of issue #58. An active
workspace owner can open **Manage future public archive** in an ended event's
editor, or visit `/events/{eventID}/public-archive`. Every entry remains private
and unpublished. This page is separate from the operational archive.

An entry records a credit or external link, attribution, intended use, a rights
assertion and an optional evidence reference. A rights assertion records an
owner's claim; it does not establish permission to copy, upload, rehost or deliver
media. URLs are stored without fetching them. Only HTTP and HTTPS URLs without
credentials or control characters are accepted.

Correcting an approved entry atomically marks the predecessor corrected and
creates a replacement in the same event and workspace. Marking an approved entry
unavailable retains it with a bounded public-safe reason. These operations do
not alter private archive notes, tickets, finance, staffing or consent. No public
read endpoint or publication worker exists for this ledger.

Migration 18 adds `event_public_archive_items` and enforces event/workspace scope
for both the owning event and correction references. Apply migrations before
starting the new binary. Preserve the ledger when rolling back application
code; do not delete approvals or correction history to downgrade a schema.

Qualification uses synthetic ended events and disposable PostgreSQL. Focused
tests cover owner authority, cross-workspace denial, revoked ownership, invalid
URLs, correction history and the historical schema upgrade. The required full
gate remains `make verify` and `make test-db`; a local pass does not establish
media rights, scene consent or public publication readiness.
