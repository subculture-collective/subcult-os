# listmonk announcement relay

Status: binding, Kork receipt import, single-path dispatch, opt-in relay worker
and synthetic contract tests implemented for issue #220 (migration 000033). No
real listmonk instance has received a request, and no real message has been
sent.

Subcult OS owns announcement content, approval and consent
([`announcements.md`](announcements.md), [`consent.md`](consent.md)). This
slice lets a workspace deliver its approved announcements through one listmonk
list it runs, instead of through Subcult OS's own outbox. It adds no second
dispatcher, no list campaign, no audience pooling and no SMS.

## Pilot need and limits

The pilot need is one community that already runs listmonk for its mailing
list and wants workspace announcements to arrive from that list's sender, with
its listmonk template and unsubscribe handling, without exporting addresses by
hand. Some of its subscribers signed up through a Kork sign-up block that adds
confirmed addresses to the same list (Kork #35, merged in Kork PR #73).

The slice is bounded to that:

- One active listmonk binding per workspace: one list, one transactional
  template and one sender address. Replacing it means revoking the old one.
- One API user and token per Subcult OS deployment (`LISTMONK_*`), so the
  pilot assumes one listmonk instance. Subcult OS checks that the user can read
  the bound list, but cannot tell which workspace owns a list. See
  [API user permissions](#api-user-permissions).
- At most five automatic attempts per recipient, with backoff from one minute
  up to 30 minutes, inside a 23-hour delivery window (the native outbox
  window).
- `MAIL_DELIVERY_ENABLED=true` is still required, and it is the emergency
  stop at send time too. Without it dispatched listmonk rows stay `held`,
  exactly like native outbox rows, and the relay refuses to run: it touches no
  row, so rows already `pending` or requeued by a retry wait until the flag is
  back. The existing live-mail qualification (#7) still gates real sends.
- The same flag makes startup validation require the native provider settings
  (`MAIL_PROVIDER`, its API key and webhook secret, `MAIL_FROM`). They are not
  decoupled for listmonk: native consent grants are confirmed by a native
  confirmation email, so a deployment that relays through listmonk still needs
  the native provider.

## List ownership

The workspace owns the listmonk list and the listmonk instance. listmonk is the
record of who unsubscribed from that list or is blocklisted there. Subcult OS
remains the authority for whether a given announcement may go to a given
person: listmonk never chooses the audience.

The relay never sends a campaign to the list. A campaign would reach every
subscriber, including people who joined through Kork or listmonk's own forms
and never authorized Subcult OS announcements. Instead each authorized
recipient gets one transactional message.

The relay never adds anyone to the list or changes a list membership. A
person who confirmed only Subcult OS announcements gets a listmonk subscriber
record with no lists, which is all `/api/tx` needs (its default
`subscriber_mode` requires that "recipients must exist as subscribers in the
database"; [transactional API](https://listmonk.app/docs/apis/transactional/)).
They never receive the list owner's campaigns because of Subcult OS. An earlier
draft of this slice added such people to the bound list as confirmed; that code
path is gone and never ran outside tests, so no deployment holds members it
added. The subscriber record stays in the list owner's listmonk after an OS
withdrawal, still with no lists; Subcult OS does not delete it, and the list
owner should not add it to a list by hand.

## Consent-receiver policy

A binding records which consent counts for it:

- `disclosureVersion`: a native grant (the existing `consent_grants` flow,
  confirmed by the recipient) counts only when it was confirmed under this
  disclosure version. Use a version whose text names the listmonk list and
  sender. Grants confirmed under an older version are withheld; those people
  need a fresh confirmation through the existing consent flow.
- `korkPolicy` (optional): the exact `listUrl`, `sender`, `purpose` and
  `disclosure` that a Kork receipt must carry. Without it the binding refuses
  Kork receipts.

Membership, ticket reservations, contacts and Kork sign-ups never imply
permission. Only a grant row counts, and dispatch and the relay check it again.

Kork-origin grants have their own unique index per (binding, address), apart
from the native one-active-grant-per-address index, so an address can hold a
native grant and a Kork grant at once and a Kork grant never blocks native
consent. A withdrawal through Subcult OS (the withdraw link in any announcement,
or an operator-recorded withdrawal) withdraws every active announcement grant
the address holds in that workspace, native and Kork alike. It does not touch
listmonk: someone who joined the list through Kork stays on it until they
unsubscribe there.

### Kork receipts

Kork stores a version-1 receipt with each sign-up (`signup_contacts.consent`,
written by `signupConsent` in Kork's `internal/server/signup.go`):

```json
{"v":1,"list":"listmonk:https://lists.example#7","sender":"Page title","page":"https://…","purpose":"Block title","disclosure":"Once you confirm, your address is added to the page owner's mailing list at lists.example, which they run with listmonk.","at":"2026-11-06T12:00:00Z"}
```

The receipt is supporting evidence, not the authority. Kork writes it when
the visitor *requests* a sign-up, before its double opt-in; it carries no
address and no confirmed flag; its fields equal what the organizer typed into
the binding policy; and its `at` is whatever the supplier chose. The authority
is listmonk: Kork pushes an address to the list only after the person confirms
(`deliverSignup` runs from `handleSubscribeConfirm` after
`store.ConfirmSignup`, or from the retry job for confirmed sign-ups), so an enabled subscriber with a
`confirmed` membership of the bound list is the evidence that the person opted
in. `page` is not checked because the policy does not name a page.

`POST /api/workspaces/{workspaceID}/listmonk-bindings/{bindingID}/kork-grants`
with `{recipientAddress, receipt}` (`manage_consent`) looks the address up in
listmonk and records a verified grant with `source = 'kork_receipt'`, scoped
to that binding, storing the receipt. It never creates, adds or confirms a
listmonk subscriber. It is refused when:

| Response | Reason |
| --- | --- |
| `422 receipt_invalid` | Not exactly the version-1 shape: unknown or extra fields, another version, missing time |
| `422 receipt_wrong_list` | `list` is not `listmonk:{korkPolicy.listUrl}#{listId}`, including Kork-local or Brevo lists |
| `422 receipt_wrong_sender` | `sender` differs from the policy |
| `422 receipt_wrong_purpose` | `purpose` differs from the policy |
| `422 receipt_wrong_disclosure` | `disclosure` differs from the policy, for example edited text |
| `422 receipt_expired` | `at` is more than 180 days old or more than five minutes in the future |
| `422 listmonk_membership_unconfirmed` | listmonk has no such subscriber, or their membership of the bound list is missing or `unconfirmed` (for example a Kork sign-up that was never confirmed) |
| `409 listmonk_subscriber_blocked` | The listmonk subscriber is `blocklisted` or `disabled` |
| `409 listmonk_unsubscribed` | The subscriber unsubscribed from the bound list |
| `409 binding_accepts_no_kork_receipts` | The binding has no Kork policy |
| `409 grant_withdrawn` | The workspace recorded a withdrawal of this address's announcement consent at or after `at`, or at any time after the binding was created. A fresh `at` never overrides a withdrawal. Withdrawals made by a binding revoke (`listmonk_binding_revoked`) don't count: they were not the person's decision |
| `409 recipient_suppressed` | The address is in `email_suppressions` |
| `409` | Revoked binding, relay not configured, or an active Kork grant already exists for this binding and address |
| `502` | listmonk could not be reached or rejected the API user |

A Kork-origin grant authorizes this binding only. Native email ignores it
(`checkSendPermission` requires `listmonk_binding_id is null`). Revoking the
binding withdraws its Kork grants (reason `listmonk_binding_revoked`), so a
replacement binding for another sender cannot use them. Before every send the
relay checks the listmonk membership again and withholds the message
(`listmonk_membership_unconfirmed`) when it is no longer confirmed.

Limits of this check: Kork does not sign receipts and has no API that exports
them; Kork deletes a contact row once listmonk accepts it. An organizer obtains
the receipt from the Kork page owner. Subcult OS cannot prove a receipt came
from Kork. The confirmed listmonk membership proves the person opted in to the
list owner's list, which may also have been through listmonk's own form rather
than Kork; the receipt and its policy match are what tie the consent to this
sender and purpose. The import is audited
(`consent_grant.kork_receipt_imported`). The 180-day window is a pilot choice,
not a legal rule. A signed Kork export would close the remaining gap and is not
part of this slice.

## Binding

- `POST /api/workspaces/{workspaceID}/listmonk-bindings`
  (`manage_channel_bindings`, owner and organizer) takes `{label, listId,
  templateId, senderAddress, disclosureVersion, korkPolicy?}`. The Kork
  `listUrl` is normalized the way Kork normalizes it before writing the
  receipt: an `https` URL without credentials, query or fragment, host
  lowercased, path case kept, one trailing slash removed. It returns `409`
  when the relay is not configured or a listmonk binding is already active,
  `422` when listmonk has no such list, and `502` when listmonk rejects the API
  user or cannot be reached. It calls `GET /api/lists/{listId}` before saving.
- `GET .../listmonk-bindings` (`manage_announcements`) lists active and revoked
  bindings.
- `POST .../listmonk-bindings/{bindingID}/revoke` keeps the row for audit,
  marks the binding's `held` and `pending` deliveries `withheld`
  (`binding_revoked`), withdraws its Kork grants, and makes later dispatches
  use the native path. The binding never added anyone to the listmonk list, so
  nothing there needs undoing.

Bindings share `announcement_channel_bindings` with Backline through its
`channel` column. The Backline routes only see `backline` rows.

## Preview, schedule and cancel

Drafting, scheduling and cancelling are unchanged. The existing
`GET .../announcements/{id}/preview` now also returns `deliveryPath` (the path
dispatch would choose now), `withheldCount` and `listmonkConfigured`.
`recipientCount` and `withheldCount` count verified, unwithdrawn grants that
path would use or withhold. Neither preview nor any other response includes a
recipient address. Schedule-time counts authorize nothing.

## Single-path routing

Dispatch (`email-deliver -announce`, the `announcement-workers` profile)
decides the path once per occurrence. If the workspace has an active listmonk
binding when the announcement is claimed, every recipient goes through
listmonk; otherwise every recipient goes through the native outbox. The choice
is stored in `announcements.delivery_path`. `announcement_deliveries` (native)
and `announcement_listmonk_deliveries` each carry a constant path and a foreign
key to `(announcement id, delivery_path)`, so the database refuses a second
path for the same occurrence. A trigger on `email_outbox` also refuses an
`announcement` row for an occurrence whose path is `listmonk`, so the outbox
can't gain one without its delivery link either. `announcement_listmonk_deliveries` is unique on
`(announcement_id, recipient_address)`, and dispatch claims an announcement
once, so repeated dispatch adds nothing.

On the listmonk path, dispatch re-derives the audience from `consent_grants`,
checks each grant against the binding's policy and current suppression, mints
a withdraw link bound to that grant (the same link the native path appends),
and stores one row per allowed recipient. It contacts nobody.

## Relay worker

`listmonk-relay [-watch] [-limit 1..100]` (`backend/cmd/listmonk-relay`), run
with the optional `listmonk-workers` Compose profile, sends due rows. Per row:

1. Re-check the grant, the binding and `email_suppressions`.
2. Look the subscriber up: `GET /api/subscribers?query=subscribers.email = '…'`
   ([subscribers API](https://listmonk.app/docs/apis/subscribers/)).
   - Blocklisted or disabled: `suppressed` (`provider_blocklisted`,
     `provider_disabled`).
   - Unsubscribed from the bound list: `suppressed` (`provider_unsubscribed`),
     and the grant is withdrawn with reason `listmonk_unsubscribed`.
   - Kork grant: the membership of the bound list must be `confirmed`.
     Otherwise, including an unknown address or an `unconfirmed` membership,
     the row is `withheld` (`listmonk_membership_unconfirmed`). Nothing is
     created, added or confirmed.
   - Native grant, unknown address: `POST /api/subscribers` with
     `status: "enabled"` and no lists. A `409` retries the lookup on the next
     attempt. An existing subscriber's list memberships are left as they are.
3. Re-check the grant, binding and suppression again (withdrawal before send).
4. `POST /api/tx` ([transactional API](https://listmonk.app/docs/apis/transactional/))
   with `subscriber_email`, `template_id`, `from_email`, `subject`,
   `data.body` (the body plus the withdraw link) and `content_type: "plain"`.
   The bound template must render `{{ .Tx.Data.body }}`.

Requests use `Authorization: token {user}:{token}`
([API auth](https://listmonk.app/docs/apis/apis/)), a 15-second timeout and no
redirects. Each recipient's calls share a 90-second deadline, shorter than the
two-minute lease, and the outcome is written even while the worker is stopping,
so an interrupted `/api/tx` is recorded as `uncertain`. Each claim gets a fresh
`claim_token`; an outcome is written only for the claim it belongs to, so a
worker that stalled past its lease can't overwrite a later claim. The relay never re-subscribes an address listmonk has as
unsubscribed or blocklisted. A withdrawal committed after step 3 but before
listmonk accepts the message can still go out; the native worker has the same
window between its check and the provider call.

### Outcomes

`announcement_listmonk_deliveries.status`:

| Status | Meaning |
| --- | --- |
| `held` | `MAIL_DELIVERY_ENABLED` was off at dispatch |
| `pending` | Due for an attempt; also after a refused connection, `429`, `503`, or any failure before `/api/tx` |
| `sending` | Claimed with a two-minute lease; an expired lease becomes `uncertain` |
| `accepted` | listmonk accepted the transactional message |
| `uncertain` | `/api/tx` timed out, returned another `5xx` or lost its lease. listmonk has no idempotency key, so the message may have gone out. Held |
| `failed` | Five attempts used, or listmonk answered `401`, `403` or `408`. Held |
| `rejected` | Another `4xx` from listmonk. Final |
| `withheld` | Grant withdrawn, no longer accepted by the binding, binding revoked, or (Kork grants) listmonk membership no longer confirmed |
| `suppressed` | Suppressed in Subcult OS, or blocklisted, disabled or unsubscribed in listmonk |
| `expired` | The 23-hour window closed first |

`last_error` stores a code such as `listmonk_http_500`. Uncertain and failed
rows are never retried automatically.
`POST .../announcements/{id}/listmonk-deliveries/retry`
(`manage_announcements`) re-queues them while the binding is active. Check
listmonk's logs first: retrying an uncertain message can send it twice.

The worker prints aggregate counts only.

## Ledger

`GET .../announcements/{id}` returns `deliveryPath` and `listmonkStatusCounts`
beside the native `deliveryStatusCounts` and the Backline `channelHandoffs`.
`recipientCount` and `withheldCount` on the announcement are the dispatch-time
counts for whichever path ran.

The web page `/workspace/{workspaceID}/announcements` connects a list, shows
the path and counts for the selected announcement, the outcome counts, and the
retry action.

## Configuration

| Variable | Meaning |
| --- | --- |
| `LISTMONK_URL` | Base URL of the listmonk instance; HTTPS in production |
| `LISTMONK_API_USER` | listmonk API user (Admin → Users) |
| `LISTMONK_API_TOKEN` | That user's token, from the deployment secret store |

All three default to empty. Set together or not at all; startup validation
refuses a partial set, an HTTP URL in production, a URL with credentials or a
query, and whitespace in the token. While unset, binding and Kork import return
`409` and the worker refuses to start. The worker also refuses to start while
`MAIL_DELIVERY_ENABLED` is not `true`.

### API user permissions

Create a listmonk user role for the relay's API user with
([roles and permissions](https://listmonk.app/docs/roles-and-permissions/)):

- `subscribers:get_all`, to read the looked-up subscriber and its lists;
- `subscribers:manage`, to create a list-less subscriber for a native-grant
  recipient;
- `subscribers:sql_query`, because the documented API has no exact-email lookup
  other than the `query` SQL expression on `GET /api/subscribers`. listmonk runs
  it read-only, but it can read every list and subscriber, so give this role
  to no other user;
- `tx:send`, for `POST /api/tx`;
- read access to the bound list, either a list role with `get` on that list or
  `lists:get_all`, for the binding check `GET /api/lists/{id}`.

The relay never needs list management, campaigns, imports or settings. Kork's
own API user for the same list (Kork PR #73) uses the same lookup.

## Cost basis

- listmonk is free, AGPLv3 software that runs as a single binary with
  PostgreSQL. Hosting it is the workspace's cost and is not measured here.
- listmonk sends through the SMTP server or messenger configured in listmonk.
  That backend's per-message price, sending limits and reputation are unknown
  for the pilot. Set `ANNOUNCEMENT_UNIT_COST_CENTS` to its per-message cost; the
  preview and the dispatch record use it for the listmonk path too, and it
  defaults to `0` (unknown).
- Per recipient the relay makes two or three HTTP requests to listmonk
  (lookup, a subscriber create for a new native-grant recipient, the message),
  plus retries. A Kork import makes one lookup.
- listmonk's own rate and concurrency settings apply on its side and are
  unknown for the pilot instance.

## Evidence

`backend/internal/app/listmonk_relay_integration_test.go` runs against
disposable PostgreSQL and an `httptest` listmonk that implements the endpoints
above: the journey (preview counts without addresses, single dispatch, one
transactional message, ledger, working withdraw link), Kork receipt validation
(valid, wrong sender, wrong purpose, wrong list, tampered disclosure, extra
field, version, expired, future, withdrawn, suppressed, authority), a
wrong-sender grant after rebinding, withdrawal after dispatch and during the
relay's own listmonk calls, local and provider suppression with reconciliation,
single-path routing in both directions, uncertain and throttled outcomes with
the retry, binding authority and configuration, and held rows plus revocation.
`listmonk_relay_test.go` covers receipt validation against Kork's exact shape,
response classification and configuration validation.
`web/src/components/ListmonkRelayPanel.test.tsx` covers the binding form,
preview counts and outcome display.

Review fixes on PR #234 added tests for: a forged or freshly stamped receipt
for an address listmonk doesn't hold as confirmed, unconfirmed, blocklisted,
disabled and unsubscribed members, and a withdrawal newer than the binding
(`TestListmonkKorkImportNeedsConfirmedListmonkMembership`); Kork sends withheld
when the membership is no longer confirmed, with no create or list change
(`TestListmonkKorkSendNeedsConfirmedListmonkMembership`); OS withdrawal leaving
list membership alone (`TestListmonkOSWithdrawalLeavesListMembershipAlone`);
the kill switch at relay time (`TestListmonkKillSwitchAtRelayTime`); a grant
from another workspace (`TestListmonkGrantFromAnotherWorkspace`); Kork grants
withdrawn on revoke while native consent keeps working
(`TestListmonkKorkGrantsAndNativeConsentCoexist`); a stale outcome write
(`TestListmonkStaleOutcomeCannotOverwriteNewClaim`); and Kork's URL
normalization (`TestListmonkKorkPolicyURLMatchesKorkNormalization`,
`TestNormalizeKorkListURL`).

Not yet done: a run against a real listmonk (version-specific API details such
as the `409` message or tx behavior for a blocklisted subscriber are unverified),
a browser session against a running fixture, a signed Kork receipt export, and
listmonk bounce feedback into `email_suppressions`.
