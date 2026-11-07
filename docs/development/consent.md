# Channel consent and suppression (CONSENT-01, Issue #23)

Status: consent grant schema, a central send-permission check and its
send-time recheck, and operator/public consent-grant endpoints implemented
2026-09-24 (migration 000013). This is the boundary task named in
[`data-boundaries.md`](data-boundaries.md)'s "Consent" section and
[`data-lifecycle.md`](data-lifecycle.md)'s consent row; it defines
permission semantics only. It does not itself send an announcement: no
code path in this slice enqueues an `announcement`-purpose message.
SIGNAL-01 (issue #24, [`announcements.md`](announcements.md)) is the
feature that does, calling `checkSendPermission` before enqueueing exactly
as `processEmailDeliveries` calls it before sending.

## What this is not

`CONSENT-01` is a boundary task, not authorization to send
(`docs/development/backlog.md`). This slice does not add any marketing
send path, campaign, template or scheduling. It only defines: what a grant
is, what never counts as one, and where permission is checked.

## The two purposes

- **`transactional`** — a message the recipient's own action requires:
  identity verification/recovery, a ticket confirmation, a workspace
  invitation. These never need a grant and are unaffected by this slice;
  every existing `email_outbox` row defaults to `purpose = 'transactional'`
  and keeps sending exactly as before.
- **`announcement`** — anything else sent to an audience because they are
  presumed interested (a newsletter, a lineup update, a marketing message).
  This purpose requires a verified, unwithdrawn `consent_grants` row that
  matches the exact workspace, channel, recipient and purpose being sent.

A grant recorded for one purpose never authorizes the other. A
`transactional` grant is, in fact, pointless to record at all —
`checkSendPermission` never consults `consent_grants` for a transactional
send — so `POST /api/workspaces/{workspaceID}/consent-grants` only accepts
`purpose: "announcement"`.

## What never implies consent

Holding a ticket, being recorded as a contact, submitting a role
application, linking an AT Protocol identity, or being a workspace member
never implies permission to receive an announcement at that address, no
matter how well the workspace otherwise knows it. `checkSendPermission`
(`backend/internal/app/consent.go`) enforces this structurally: its only
queries are against `email_suppressions` and `consent_grants`; it contains
no join, subquery or fallback that reads `tickets`, `contacts`,
`event_role_applications`, `did_links`/atproto identity tables or
`workspace_members`. A regression test
(`TestCheckSendPermissionNeverConsultsUnrelatedTables`) plants a ticket
holder, a contact and a workspace member that all share one address in one
workspace and proves an announcement to that address is still denied
without a real grant.

## The `consent_grants` table (migration 000013)

| Column | Meaning |
| --- | --- |
| `workspace_id` | The sender: which workspace this grant authorizes announcements on behalf of. Not nullable — every grant in this slice is workspace-scoped; there is no platform-level sender today, so a nullable "platform" sender would be speculative. |
| `channel` | `'email'` today; the check constraint can be widened to add `'sms'` later (same pattern migration 000010 used for `workspace_members.role`), without touching any row. |
| `recipient_address` | Stored as plaintext, matching the existing convention for other private recipient columns in this codebase (for example `tickets.email`, `contacts.email`). This is a private column: never included in any public projection. |
| `purpose` | `'announcement'` or `'transactional'`; see above. |
| `scope` | Free-text operator-supplied label for what the recipient agreed to receive (for example `"monthly newsletter"`). Not machine-enforced in this slice. |
| `verification_token_hash` | SHA-256 hash of a random token, the same construction `identity_challenges.token_hash` uses (`newToken`/`tokenHash` in `auth.go`). The raw token is never stored; it is only ever present in the one-time confirm/unsubscribe email link. It doubles as the unsubscribe credential (see below). |
| `verified_at` | Set once the recipient clicks the confirm link. `null` means the grant does not yet authorize anything. |
| `disclosure_version` | Opaque operator-supplied label identifying which disclosure text the recipient agreed to. Recorded so a future audit or re-consent campaign can tell which recipients agreed to an older disclosure. This slice does not interpret the value. |
| `granted_at` | When the grant was created (not when it was verified). |
| `withdrawn_at` / `withdrawal_reason` | Set once, never cleared. A withdrawn grant is permanently inactive; recording a new grant for the same tuple requires the old one to already be withdrawn (see the unique index below). |
| `source` | `'explicit_form'` (recipient submitted a public form themselves) or `'operator_recorded'` (an operator recorded a grant obtained elsewhere, e.g. a paper sign-up sheet). Both still require the same verification step before the grant is active. `'kork_receipt'` (migration 000033) is a Kork double opt-in receipt that matched a listmonk binding's receiver policy; it is scoped to that binding through `listmonk_binding_id`, never authorizes native email and is described in [`listmonk-relay.md`](listmonk-relay.md). |
| `created_by_person_id` | The operator who recorded the grant. |

**Unique active grant**: a partial unique index on
`(workspace_id, channel, recipient_address, purpose) where withdrawn_at is
null` means at most one non-withdrawn row per tuple, whether or not it is
yet verified. Recording a second grant for an address that already has an
active one is a `409`; the caller must withdraw the existing grant first.

**Private, never projected**: no public route in this codebase reads
`consent_grants`, and none should ever be added to the allowlisted
projection builders described in `data-boundaries.md`'s "Public payload
rule".

## `checkSendPermission`

`backend/internal/app/consent.go` defines the single function every send
path must call:

```go
func (a *App) checkSendPermission(ctx context.Context, workspaceID, channel, recipient, purpose string) error
```

Decision order:

1. **Suppression always wins.** If `recipient` (normalized the same way
   `email_suppressions.recipient_email` already is) is suppressed, return
   `ErrConsentSuppressed` — regardless of purpose, and regardless of any
   grant. A bounce or complaint suppresses the address for every purpose,
   including transactional mail, matching the existing suppression
   behavior `processEmailDeliveries` already had before this slice.
2. **Transactional passes.** If `purpose == "transactional"`, return `nil`
   once suppression is cleared.
3. **Announcement requires a live grant.** Otherwise, require a
   `consent_grants` row matching `workspace_id`, `channel`, `recipient`
   and `purpose = "announcement"` with `verified_at is not null` and
   `withdrawn_at is null`. Anything else — no grant, a grant for the wrong
   purpose, an unverified grant, or a withdrawn grant — returns
   `ErrConsentGrantRequired`. These are deliberately not distinguished
   further: an unverified or withdrawn grant authorizes nothing, exactly
   like no grant at all.

Listing-change notices use transactional purpose with a separate, owner-approved
operational audience derived from tickets and assigned crew. They do not infer
announcement grants. The mail worker additionally checks the approved listing,
message and recipient relationship before sending; see
[listing notice approval](event-lifecycle-changes.md#listing-notice-approval-and-outcomes).
Suppression still applies to every purpose.

## Send-time recheck

Consent can be withdrawn, or an address suppressed, after a message is
enqueued and before it leaves the system. `processEmailDeliveries`
(`backend/internal/app/email_delivery.go`) calls `checkSendPermission` for
every claimed row, immediately after leasing it and before handing it to
the mail provider. A denial marks the row `delivery_status =
'withheld_consent'` (a new terminal status added to the existing check
constraint from migration 000006) with `last_error_code` set to
`consent_required` or `recipient_suppressed`, clears its body the same way
every other terminal status already does, and never calls the provider.
`withheld_consent` rows are never retried by any of this worker's existing
retry/expiry logic (they are already terminal, exactly like `failed` and
`quarantined`).

`email_outbox` gained two columns to support this (migration 000013):

- **`purpose`** — `'transactional'` by default, so every existing and
  legacy-binary row keeps its meaning and keeps sending unchanged.
- **`workspace_id`** — nullable, unset by every existing enqueue call site
  (`enqueueEmail`'s four callers are all transactional and never set it).
  It exists so a future `announcement`-purpose row carries the workspace
  `checkSendPermission` needs; nothing in this slice enqueues such a row.

Tests cover: an announcement with a valid, verified grant sends
(`TestProcessEmailDeliveriesSendsVerifiedAnnouncementGrant`); withdrawing
that grant between enqueue and worker run withholds it and the provider is
never called (`TestProcessEmailDeliveriesRevokeBeforeSend`); a grant
recorded for the wrong purpose does not authorize the announcement
(`TestProcessEmailDeliveriesWithholdsWrongPurposeGrant`); an address with
no grant at all is withheld
(`TestProcessEmailDeliveriesWithholdsUnknownGrant`); and every existing
transactional row keeps sending exactly as before
(`TestProcessEmailDeliveriesUnaffectedForTransactionalRows`).

## Endpoints

All four are new in this slice; nothing else changes route behavior.

- **`POST /api/workspaces/{workspaceID}/consent-grants`** — operator-only
  (`permManageConsent`: owner or organizer, see
  [`authority-model.md`](authority-model.md)). Body:
  `{channel, recipientAddress, purpose, scope, disclosureVersion, source}`.
  `purpose` must be `"announcement"`. Creates an unverified grant and
  enqueues a `transactional` verification email (`related_type =
  "consent_verification"`) containing the confirm link — recording a grant
  never itself requires a grant, because the grant it establishes does not
  exist yet. Returns `409` if an active grant already exists for the same
  tuple.
- **`GET /api/workspaces/{workspaceID}/consent-grants`** — operator-only,
  same permission. Lists every grant for the workspace, newest first.
  `verification_token_hash` and the raw token are never included in the
  response.
- **`POST /api/public/consent/{token}/confirm`** — public, no session.
  Marks `verified_at` (idempotent: confirming an already-verified grant
  succeeds without changing anything). An unrecognized or already-withdrawn
  token returns a generic `404`; the response never reveals the recipient
  address, workspace or any other grant field.
- **`POST /api/public/consent/{token}/withdraw`** — the unsubscribe
  route, public and tokenized, no session. It is `POST` only: the emailed
  link opens the web page `/consent/withdraw?token=…`, which posts on the
  recipient's click, so a link-prefetching mail scanner cannot withdraw on
  their behalf. Sets `withdrawn_at` and `withdrawal_reason =
  "recipient_requested"` (idempotent, never overwrites an existing
  reason). The same token serves confirm and withdraw, since both are
  private, per-grant secrets equally sensitive to leak. An unrecognized
  token returns a generic `404` and reveals nothing else. The confirm link
  likewise opens `/consent/confirm?token=…`, which posts to the confirm
  route.
- **`POST /api/workspaces/{workspaceID}/consent-grants/{grantID}/withdraw`**
  — operator-only (`permManageConsent`), for withdrawals received out of
  band (reply email, phone, in person). Scoped to the workspace in the
  path; a grant id from another workspace is a `404`. Records
  `withdrawal_reason = "operator_recorded"` and an audit entry.

`manage_consent` is a new permission, granted to `owner` and `organizer`
(the same set as `manage_delegations`), not `manage_members`: recording or
withdrawing a grant does not change anyone's workspace authority. See
[`authority-model.md`](authority-model.md).

## Disclosure versioning

`disclosure_version` is an opaque, operator-supplied string identifying
which version of a disclosure/consent-request text a recipient agreed to.
This codebase does not render or validate disclosure text; it only records
which label the recipient agreed to at grant time, so a future audit can
identify recipients whose consent predates a disclosure-text change. There
is no re-consent workflow in this slice: reconciling old-disclosure grants
against a new disclosure version is left to a future feature. The one place
the label is compared is a listmonk binding, which uses only grants confirmed
under its own `disclosureVersion` (see [`listmonk-relay.md`](listmonk-relay.md)).

## Suppression precedence

`email_suppressions` (migration 000007) already suppresses an address for
every message regardless of purpose, once a bounce, complaint or explicit
suppression event is recorded. This slice does not change that table or
its precedence; `checkSendPermission` checks it first and unconditionally,
so a suppressed address is denied even if it also holds a verified,
unwithdrawn announcement grant.

## Remaining limits

- This slice defines the permission boundary only; SIGNAL-01 (issue #24,
  [`announcements.md`](announcements.md)) is the implemented feature that
  queues announcement sends, calling `checkSendPermission` at dispatch and
  relying on the same `processEmailDeliveries` recheck at send time.
- `recipient_address` is plaintext, matching the existing convention for
  other private recipient columns in this codebase; it is not
  additionally encrypted at rest in this slice.
- `disclosure_version` is an opaque label; no disclosure-text storage,
  rendering or re-consent workflow exists.
- `sms` is named in the channel design but not implemented: `channel`
  currently accepts only `'email'`.
- Consent grants are not included in the access-export/deletion pipeline
  described in `data-lifecycle.md`, because that pipeline itself is not
  implemented yet (see that document's "Remaining limits"). A future
  implementation of that pipeline must include a recipient's own
  `consent_grants` rows (matched by verified email/account, not an
  unverified `recipientAddress` claim) in export, and must withdraw rather
  than silently delete a grant on account deletion, so the withdrawal is
  itself auditable.
