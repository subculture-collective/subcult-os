# Backline announcement handoff

Status: producer contract, organizer preview/handoff, opt-in delivery worker and
synthetic receiver tests implemented for issue #221 (migration 000032). No real
Backline receiver or Discord channel has received a handoff. The receiver side
is [Backline #88](https://git.subcult.tv/subculture-collective/backline/issues/88).

Subcult OS owns announcement content and approval
([`announcements.md`](announcements.md)). Backline owns durable channel
announcements in Discord. This slice lets an organizer post one approved
announcement to one connected Backline channel. It adds no email-list
transfer, direct messages, crossposting or second approval step.

## Pilot need and limits

The pilot need is one community that already runs Backline and wants workspace
announcements to appear in one Discord channel without copying the text by
hand. The slice is bounded to that:

- One active binding per workspace. Replacing it means revoking the old one.
- One handoff per announcement and binding.
- Content is at most 1,800 characters (subject, blank line, body), Backline's
  existing announcement limit.
- Mention policy is `none`. Backline announcements cannot notify users, roles
  or everyone, so the contract offers no other value. A later version may add
  a bounded policy once Backline supports one.
- Delivery window: Backline must not post after `expiresAt`, 24 hours after
  `deliverAt`, matching Backline's one-day delivery window.
- Cost: one signed HTTPS request per handoff, at most five automatic attempts,
  plus one cancel message when needed. Backline's Discord rate limits and job
  quotas apply on its side. There is no per-message provider fee, so
  `ANNOUNCEMENT_UNIT_COST_CENTS` does not apply.

Real Discord delivery remains a separately qualified pilot. It needs Backline
#88 deployed, a server administrator's opt-in, and a shared secret set in both
deployments' secret stores.

## Authority

- `manage_channel_bindings` (owner and organizer) connects or revokes the
  workspace's Backline binding. See
  [`authority-model.md`](authority-model.md).
- `manage_announcements` (owner and organizer) previews, hands off and retries.
- Approval is the existing announcement lifecycle: only a `scheduled`,
  `dispatching` or `dispatched` announcement can be handed off. A draft or
  cancelled announcement is refused with `409`.
- A binding from another workspace is `404`. A revoked binding is `409`.

## Binding

`POST /api/workspaces/{workspaceID}/backline-bindings` takes `{label, serverId,
channelId}`. The ids are the Discord server and channel snowflakes the Backline
administrator approved. The Backline side must hold a matching opt-in for the
binding id, server and channel.

`POST .../backline-bindings/{bindingID}/revoke` keeps the row for audit and
refuses new handoffs. In the same transaction it cancels that binding's unsent
handoffs and turns sent ones into cancel messages. `GET .../backline-bindings`
lists active and revoked bindings.

## Preview and handoff

`GET /api/workspaces/{workspaceID}/announcements/{announcementID}/backline-preview`
returns the subject, body, content revision, length and limit, mention policy,
destination binding, `deliverAt`/`expiresAt`, whether the adapter is configured,
the existing handoff and a `blockers` list (`announcement_not_approved`,
`no_active_binding`, `content_too_long`, `adapter_not_configured`,
`already_handed_off`). It creates nothing.

`POST .../announcements/{announcementID}/backline-handoff` with `{bindingId}`
creates one pending handoff and returns `201`. Repeating it returns the same
handoff with `200`. `deliverAt` is the announcement's scheduled time, or the
handoff time if that has passed.

Rescheduling moves an unsent handoff to the new time. Once a handoff may have
reached Backline, rescheduling is refused; cancel the announcement and draft a
new one. Cancelling the announcement withdraws its handoffs the same way
revocation does.

`GET .../announcements/{id}` returns the outcome ledger in `channelHandoffs`,
beside the email `deliveryStatusCounts`.

The web page `/workspace/{workspaceID}/announcements` connects a channel,
previews an announcement and hands it off.

## Contract, version 1

Files: [`contracts/backline-handoff.v1.schema.json`](../../contracts/backline-handoff.v1.schema.json)
and [`contracts/backline-handoff.v1.fixtures.json`](../../contracts/backline-handoff.v1.fixtures.json),
which holds examples and two signature vectors.

### Request

`POST {BACKLINE_HANDOFF_URL}` with JSON body:

| Field | Meaning |
| --- | --- |
| `version` | `1` |
| `kind` | `subcult-os.announcement-handoff` |
| `operation` | `publish` or `cancel` |
| `occurrenceKey` | `subcult-os/announcement/{announcementID}/binding/{bindingID}`; stable across retries and cancellation |
| `contentRevision` | `sha256:` + hex SHA-256 of the UTF-8 subject, one NUL byte, then the body |
| `binding` | `{id, serverId, channelId}` |
| `deliverAt`, `expiresAt` | RFC 3339 UTC |
| `mentionPolicy` | `none` |
| `content` | `{subject, body}`; publish only |
| `sentAt` | RFC 3339 UTC send time |

The payload carries content and identifiers only. It never includes email
addresses, consent, audience, membership, workspace or person data.

Headers:

- `X-Subcult-Handoff-Timestamp`: Unix seconds.
- `X-Subcult-Handoff-Signature`: `v1=` + hex HMAC-SHA256 with
  `BACKLINE_HANDOFF_SECRET` over `v1.handoff.{timestamp}.` followed by the raw
  body bytes.
- `Idempotency-Key`: the occurrence key (informational; the signed body is
  authoritative).

The receiver must reject a timestamp more than five minutes from its clock and
compare signatures in constant time.

### Receiver obligations

- Deduplicate on `occurrenceKey`. A repeated publish answers `duplicate` and
  posts nothing new.
- A cancel for an unknown key answers `not_found` and records a tombstone, so a
  later publish with that key is refused. This covers a cancel that overtakes
  an in-flight publish.
- Recheck its own binding opt-in, module state and permissions at delivery.
- Never post after `expiresAt`.

### Responses

`200` with `{version: 1, occurrenceKey, status, receiverReference?}`:

- publish: `accepted` or `duplicate`;
- cancel: `cancelled`, `not_found` or `already_delivered`.

A mismatched key or status is treated as uncertain. Other responses:

| Response | OS outcome |
| --- | --- |
| `429` or `503` | Not processed; retried with backoff |
| Other `4xx`/`3xx`, body `{"error": "code"}` | `rejected`, final; the code is stored |
| Other `5xx`, timeout, unreadable acknowledgement | `uncertain`, held |
| Connection refused before sending | Retried with backoff |

### Delivery receipt

Backline reports the later Discord outcome with
`POST /api/backline/receipts`:

```json
{"version":1,"kind":"subcult-os.announcement-receipt","occurrenceKey":"…","contentRevision":"sha256:…","outcome":"delivered","receiverReference":"…"}
```

`outcome` is `delivered`, `failed`, `uncertain` or `cancelled`. The headers are
the same, but the signature covers `v1.receipt.{timestamp}.` plus the body, so a
captured handoff cannot be replayed as a receipt. OS answers `200` (`recorded`
or `unchanged`), `401` for a bad signature or timestamp, `404` for an unknown
key, and `409` for a revision mismatch or a change to a final outcome. Only
`uncertain` may later resolve. The route is `404` while the adapter is not
configured.

## Handoff states

`announcement_channel_handoffs.status` describes the current operation:

- `pending`: queued; the worker sends it when `next_attempt_at` passes.
- `sending`: claimed with a two-minute lease. An expired lease becomes
  `uncertain`.
- `accepted`: acknowledged by the receiver; `receiver_status` keeps the reply.
- `rejected`, `failed` (five attempts used), `uncertain`: held and visible.
- `cancelled`: withdrawn before it was ever sent.
- `expired`: the delivery window closed before sending.

Uncertain and failed handoffs are never retried automatically. An organizer
checks the channel and uses
`POST .../announcements/{id}/channel-handoffs/{handoffID}/retry`, which resends
with the same key and revision, so the receiver posts at most once.

Cancellation and revocation leave a pending publish `cancelled`. A publish
that is `sending`, `accepted`, `uncertain` or `failed` becomes
`operation = cancel` with a fresh attempt count. A worker finishing the old
publish cannot overwrite that change.

## Worker and configuration

`backline-handoff [-watch] [-limit 1..100]` (`backend/cmd/backline-handoff`)
runs batches every 30 seconds with `-watch`. It refuses to start without
`BACKLINE_HANDOFF_URL` and `BACKLINE_HANDOFF_SECRET` (32 or more characters,
no whitespace), and handoff creation returns `409` without them. The URL must
be HTTPS in production. Both default to empty.

Run it with the optional `backline-workers` Compose profile. Output is
aggregate counts only.

The `channel` column on bindings is the seam for a later destination; only
`backline` is admitted today.

## Evidence

`backend/internal/app/backline_handoff_integration_test.go` runs against
disposable PostgreSQL and an `httptest` receiver that implements the contract:
journey and receipt readback, bad signature, uncertain result with a duplicate
retry, cancel before and after delivery, binding revocation, authority and
workspace scope, rescheduling, bounded retries and expiry.
`backline_handoff_test.go` checks the fixture signature vectors, payload fields
against the contract example and response classification.
`web/src/components/BacklineHandoffPanel.test.tsx` covers the preview and
outcome display.

Not yet done: a live receiver, Discord delivery, and a browser session against
a running fixture receiver.
