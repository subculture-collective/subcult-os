# Development execution log

## 2026-09-30 — discovery event-zone clocks and theme focus

Fixed projected occurrence cards/detail that formatted the viewer's local clock
then appended the event's zone. The same instant now displays Chicago 3:00 PM
and Tokyo 5:00 AM the following day. The instant's offset distinguishes repeated
fall-back wall times; missing or unusable zones show an explicit UTC fallback.
Fixed hardcoded black card focus rings and coordinate points to use semantic
theme tokens. No API contract, schema, projection or publication changes.

Full pinned `bash scripts/dev-env.sh verify` passed on the final source: 281 web
tests, 34 mobile tests, backend checks/build and all 421 top-level disposable DB
tests (599 including nested), zero failures/skips. The model's 13 tests also
passed under `TZ=Pacific/Honolulu` and `TZ=Asia/Tokyo`.

One earlier full gate lost its PostgreSQL checkpointer. Retained DB and kernel
logs prove a memory-cgroup OOM at the repository test DB's 768 MiB limit. A
subsequent unchanged DB suite passed at that cap, but the repository-only test
limit was raised to 1 GiB. README/AGENTS document the difference from the
installed shared T3 recipe; neither the retained dev DB nor the shared recipe
changed. The final full gate used the actual 1 GiB container cap; sampled cgroup
peak was about 1 GiB with zero `oom`/`oom_kill` events. Memory-limit reclaim was
observed. The disposable container was removed. Failed evidence is retained in
ignored `.cache/dev-env/discovery-zone-infra-db.log` and
`discovery-zone-oom-kernel.log`; final resource receipt is
`discovery-zone-db-resource-receipt.json`.

The desktop browser was T3 Code on Linux, Chrome 152.0.7977.130/Electron
44.4.2, with a 1402 × 876 CSS-pixel viewport.

Real desktop component-fixture checks rendered the actual discovery section
with synthetic intercepted public-list responses. Cards and the Tokyo detail
showed the event-local date rollover; unknown zone displayed UTC. Keyboard Tab
showed a 2px light outline on a dark card; Enter opened detail, Escape closed it
and returned focus. Coordinate fill was light on dark and dark on light.
Screenshots were inspected. The fixture root, interception and all helpers were
removed and the original private archive page restored. No external feed or
native/mobile/all-day behavior is claimed. Retained app data and disabled mail,
OAuth/projection settings were preserved; no backend restart or external action
was needed for these frontend/resource changes.

Issues #54/#57 had stale queued-CI wording reconciled against exact successful
jobs/merge records. Issues #50/#55/#58 now include qualified source receipts,
merged versus open status and remaining gates. Bodies were read back exactly;
no prerequisite link was removed, checkbox invented or issue closure attempted.

## 2026-09-30 — private archive permission and pending-write states

PR #174 exact head `67fb7967fdbd1e874b624e44d4c46f6452f03e80` passed
push 11045/job 19992 and PR 11046/job 19993, including the full disposable DB
gate (421 top-level tests, 599 including nested). Temporary runners exited 0,
were removed with credentials deleted, and zero repository registrations were
verified. Additional browser checks retained an editable draft on 400 and
cleared all inputs/rows on 401 plus failed refresh; helpers were removed.

Fixed the private archive approval page's retained-content behavior after a
permission rejection. Owner reads now gate all ledger and mutation controls;
401/403 responses clear rows, draft fields, correction selection and unavailable
reason. One pending write disables every editing action and a synchronous guard
rejects duplicate submits. Event changes/unmounts invalidate old responses.
Shared themed buttons replace this page's custom buttons. Successful private
archive responses are noncacheable; no schema or publication contract changes.

Full pinned `bash scripts/dev-env.sh verify` passed: 278 web tests, 34 mobile
tests, backend checks/build and the complete disposable database gate: 421
top-level tests (599 including nested), zero failures/skips. The earlier run
caught an obsolete route test that expected controls before the owner read;
that expectation was corrected. The disposable database was removed.

Real synthetic desktop browser checks held an actual correction's 201 response:
all draft fields and edit actions stayed disabled, and a second submit caused no
second request. A simulated lost response after another actual committed
correction blocked further changes; reload showed exactly one replacement and
cleared the draft, with no automatic replay. A simulated unavailable-write 403
removed all rows, draft inputs and reason. A denied reload kept them hidden;
restoring the transport and reloading recovered the three-entry correction
chain. All interception helpers were removed. Light/dark screenshots were
inspected. The server's private/no-store header was read from the live dev API.
These checks do not prove media rights or public publication readiness.

Restarted only this checkout's development API. Schema 29 and retained counts
were unchanged across restart: three events, five occurrences, seven access
revisions, two venues, three synthetic archive approval rows and four held
outbox rows. Sending, OAuth linking and projection remained disabled. The new
synthetic ended archive event was created through ordinary APIs; existing
lifecycle and development events were preserved. No production/provider action
occurred. Approval creation remains non-idempotent; uncertain responses require
ledger reconciliation before another change.

## 2026-09-30 — ACCESS-INFO occurrence venue comparison

PR #173 exact head `66472a21e205ff0a7507d8f3713695861ebe78c0` passed
both hosted checks: PR 11043/job 19990 and push 11044/job 19991, including the
full disposable database gate (421 top-level tests, 599 including nested).
Owned temporary runners exited 0 and were removed with credentials deleted;
the repository runner list returned to zero.

Added an owner-only, read-only comparison inside the event worksheet. It shows
the selected occurrence's linked venue beside the event's six recorded topics,
with distinct source/review/expiry labels and explicit Unknown states for a
missing venue or assertion. A repeatable-read transaction returns the occurrence
revision and both worksheets from one database snapshot and expiry clock. Owner
membership is rechecked after the transaction. Minimal occurrence options omit
creator identity, description and public record metadata. No source assertion is
copied, no occurrence verification is recorded, and no public DTO changes.

Full pinned `bash scripts/dev-env.sh verify` passed: 277 web tests, 34 mobile
tests, backend checks/build and complete disposable DB gate: 421 top-level tests,
599 including nested subtests, zero failures or skips. The separate race-enabled
access run passed 16 top-level tests, 32 including nested, with no race warnings.
Tests hold a worksheet table lock while changing the occurrence link/venue name
or revoking owner membership: the original comparison keeps one snapshot, the
next read sees the change, and revoked ownership returns 403 with no private
payload. Both disposable databases were removed.

Real synthetic desktop browser checks showed venue No beside event Yes with
separate provenance, six rows and a correct private venue link. An unlinked
occurrence displayed venue Unknown. Switching selection while an older actual
server response was held cleared the old rows; releasing that response did not
overwrite the new selection. An actual occurrence venue-link change appeared on
Refresh comparison; the QA fixture's original link was then restored. A
simulated comparison 403 cleared parent title, draft, history, cards and
comparison. Fetch interceptions were removed. Light and dark screenshots were
inspected. This is synthetic desktop proof, not actual venue conditions, a saved
occurrence-specific verification, full mobile/device evidence or user evaluation.

Restarted only this checkout's development API after qualification. Schema 29
and retained counts were preserved: two events, five occurrences, seven access
revisions, two venue references and four held outbox rows. No migration,
production deployment, mail delivery, provider call or public publication was
performed. #55 remains open for its unfinished acceptance gates.

## 2026-09-30 — ACCESS-INFO private venue observations

Extended the private access ledger to workspace-owned cultural places without
inheriting venue assertions into events. Owner-only venue references expose
names and IDs; named reference creation recovers an uncertain response with a
stable request key. Venue observations and event observations have distinct
source kinds and database scope checks. Topic corrections, conservative expiry,
exact replay, owner revocation checks and atomic audit use the shared contract.
Migration 29 preserves existing event revision IDs and payloads.

Full pinned `bash scripts/dev-env.sh verify` passed: 273 web tests, 34 mobile
tests, backend checks/build and the complete disposable DB gate: 419 top-level
tests, 595 including nested subtests, zero failures or skips. A separate
race-enabled access DB run passed 14 top-level tests, 28 including nested
subtests, with no race warnings. Both disposable databases were removed. The
initial rollback fixture incorrectly validated a rejecting constraint against
existing audit rows; it now rejects new writes without validating old fixtures.
The contract checker also required DTO fields on separate lines; this was fixed
before the final full gate.

Backed up retained development schema 28 and verified its dump catalog before
restarting only this checkout's API. Schema 29 preserved two events, five event
access revisions and four held outbox rows. Real desktop browser actions created
a venue, recorded expired Yes as effective Unknown, corrected it to No, retained
both history entries and reloaded revision 2. A real committed creation with a
simulated lost response recovered through the form's retry without creating a
third venue: two names and two creation requests remained. Simulated 403 responses
cleared both worksheet data and index names/draft; actual membership revocation,
including while waiting for a venue lock, passed in the DB tests. Browser fetch
interceptions were removed. Dark worksheet and light index screenshots were
inspected. This proves synthetic desktop behavior, not actual venue conditions,
full mobile behavior or intended-audience usability.

PR #171's exact head `39893d69` passed hosted push 11019/job 19932 and PR
11020/job 19933: 272 web, 34 mobile, 412 top-level DB tests (588 including nested),
zero failures or skips. Its body was updated and read back. PR #170 also has both
hosted checks green, targeting main after #169 merged at `375451dc`. The
merge-commit CI attempt failed when the temporary runner daemon exhausted its
256 MiB limit before tests. Owned orphaned job containers and the ephemeral
credential were cleaned, the daemon limit was raised to 1 GiB, and the unchanged
merge commit passed as job 19978: 405 top-level DB tests (567 including nested),
zero failures or skips. PR #172 at `7c772723` then passed hosted push
11037/job 19983 and PR 11038/job 19984, each with the same full 273 web/34 mobile/
419 top-level DB gate (595 including nested). The successful temporary runners
were removed, their credentials deleted, and zero repository registrations
verified. Its PR body was updated and read back. Shared
runner configuration and other worktrees were preserved.

#55 remains open. Explicit occurrence-scoped review of venue information,
public display/correction wording, accommodation-request privacy decisions and
meaningful accessibility-user evaluation remain separate acceptance work.

## 2026-09-30 — ACCESS-INFO private event worksheet

Added a six-topic, owner-only event worksheet with explicit Unknown states,
source kinds/references, review time and optional expiry. Organizer assertions,
event observations and external references remain distinct. Expired assertions
project to Unknown while preserving the original record. Corrections append
revisions with exact preconditions, bounded history and atomic audit; replay
binds event, topic, actor and normalized payload. Private identities/request
keys stay excluded from response DTOs and anonymous event/AT records.

The source slice follows the owner's expansion-development decision. It does
not infer demand or user evaluation. Venue assertion history, public event
verification/display, personal accommodation-request controls and meaningful
accessibility-user evaluation remain unfinished #55 acceptance work.

The first full `bash scripts/dev-env.sh verify` exited 0 with 272 web and 34
mobile tests, 412 top-level database passes (588 including subtests), zero
failures/skips. Focused database tests passed owner/member/outsider/revocation,
unknown/expiry, replay, stale/concurrent correction, audit rollback, pagination,
upgrade/replay preservation and public absence. A final full run follows the
last replay/pagination and error-state refinements; its result is recorded below.

Verified the preview API mounts this worktree and that mail, OAuth and
projection flags remain false. A private schema-27 dump and archive catalog
were saved before restarting only this worktree's API. Schema 28 applied and
health returned healthy. Counts remained two events, one ticket, two notices,
one review and four held outbox rows with zero attempts before browser edits.

In the real local desktop browser, a synthetic expired organizer assertion
became Unknown with review-expired guidance; a No/event-observation correction
then replaced it, followed by an Unknown withdrawal. All three revisions
persisted through reload with their original provenance. A simulated 403 cleared
the event title, worksheet, draft and history; the backend gate separately tests
actual owner revocation during an event-lock wait. Date automation initially
typed into the wrong focused input; corrected by setting the observed native
date controls and dispatching their normal input events before the real submit.
Light and dark screenshots were inspected and labeled controls checked. Full
viewport resizing still timed out; native/mobile and intended-user evaluation
remain unqualified. No live provider or production change occurred.

Final full verification exited 0: 272 web and 34 mobile tests, 412 top-level
DB passes (588 including subtests), zero failures/skips, disposable DB removed.
A separate race-enabled access gate passed all seven top-level tests (21 with
subtests), with no race warning; its disposable DB was also removed.

A lost-response browser check committed revision four but withheld its response.
A later synthetic correction created revision five. Refresh loaded that newer
record, and retry recovered revision four without replacing the current card or
creating a sixth revision. The editor showed the newer-revision warning.
Temporary fetch interception was removed. A 388 CSS-pixel same-origin iframe
showed all six cards and no horizontal overflow (document width 373); its
screenshot was inspected and the frame removed. This is narrow layout evidence,
not full viewport/device or intended-user validation.

## 2026-09-30 — LIFE-01 worklist clarity and refresh

The action list now separates an unapproved operational notice draft from a
completed local email queue. It displays destination, dispatch approval,
attempts, retry/completion timing and unknown-outcome reconciliation guidance.
A successful queue action says “Queued locally”; recipient delivery remains a
separate outcome. Known destinations use plain user-facing labels.

Notice approval triggers a parent worklist refresh without replacing the keyed
notice outcome/review widget. Owners also have a shared outlined refresh button.
Request guards reject stale reads and mutation responses after event/workspace
changes; access denial clears private state and invalidates pending requests.

In the real local desktop browser, a second synthetic cancelled occurrence
completed preview and queue approval. Its new queue action appeared immediately
alongside the original unapproved draft, while the recipient remained held with
zero attempts. A deliberately delayed worklist response was released after
switching to Development Night: no stale synthetic intent reappeared. The fetch
interception was removed after the check. No live provider call occurred.

Final `bash scripts/dev-env.sh verify` exited 0: full `make verify` with 264 web
and 34 mobile tests, followed by the complete disposable database gate with
405 top-level passes, 567 including subtests, zero failures/skips. The owned
disposable test database was removed. The dark desktop screenshot was inspected;
full viewport/mobile and live-provider qualification remain unfinished. Hosted
CI for this follow-up remains a separate gate.

## 2026-09-30 — PR #169 hosted qualification and browser recovery

PR #169 retains qualified head `b0172c053a789c19b5e1141168492108076ae9d0`.
Both hosted runs passed: push 10947/job 19785 and PR 10948/job 19786.
Each ran the complete baseline and disposable database gate: 261 web tests,
34 mobile tests, 405 top-level database passes (567 including subtests),
zero database failures/skips. The exact-head combined status is success.

The shared runner queue stalled behind unrelated jobs. A repository-scoped,
one-job ephemeral runner using pinned runner/job images completed the original
push job without changing the workflow, source or shared runner configuration.
The original PR job subsequently ran on the shared runner. A second temporary
runner was briefly started after that job had already been assigned; verified
idle, it was removed. Both temporary containers, repository runner registrations
and expired local credentials were cleaned up. Unrelated jobs were preserved.

T3 browser automation recovered. A separate synthetic local event completed
notice preview, explicit queue approval, owner review-note save and persistence
through reload in the real desktop UI. The inspected dark-mode screen was
readable. A temporary 388 CSS-pixel iframe had no horizontal overflow and was
removed after inspection; full viewport resizing still timed out. Native device
and full responsive journeys remain unqualified. Retained development data now
contains two events, one synthetic ticket, one notice, one review and three held
outbox rows with zero attempts. Mail, OAuth and projection remain disabled.

The browser check exposed stale parent worklist state after notice approval and
ambiguous identical labels for its original draft and completed local queue.
Follow-up source work begins on `t3code/lifecycle-outcomes-worklist`, preserving
PR #169's qualified head. Queue creation, provider feedback and owner notes
remain separate evidence. No provider activation or deployment occurred.

## 2026-09-29 — LIFE-01 private notice review log

Added an owner-only review endpoint and append-only review log for queued
listing notices, including superseded decisions. Each private note records a
server snapshot of recipient queue state, attempts and provider feedback. The
review and audit entry commit atomically. Exact request-key replay preserves the
original observation; changed bindings conflict. The endpoint never modifies
the mail queue or treats an owner's note as delivery evidence.

The outcome screen now summarizes queue states, explains uncertainty after
earlier attempts and uses the shared outlined control for saving reviews.
Historical observations remain separate from current delivery feedback. The
client retains request identity after an uncertain response and clears private
state on access denial. Provider identifiers and request keys stay excluded
from the response contract.

Final `bash scripts/dev-env.sh verify` exited 0: full `make verify` with 261 web
and 34 mobile tests, then the complete disposable database gate with 405
top-level passing tests, 567 including subtests, zero failures/skips. New
regressions cover owner privacy/revocation, validation, replay, concurrent
duplicates, frozen observations after later feedback, complete audit rollback,
unchanged outbox fields and migration/replay preservation. The first full run
found an older schema-24 reconstruction fixture that needed to drop the new
review table; the corrected full rerun passed.

T3 preview status reported no automation-capable tab. Both explicit open
attempts timed out. The rendered regressions and real backend database tests
passed; browser interaction and live-provider reconciliation remain unqualified.

Verified the API mounts this worktree and took a private custom-format backup
of the retained schema-26 development database, with archive catalog readback.
Restarted only this worktree's API; startup applied schema 27 and health returned
200. Retained counts remain one event, zero tickets, one held outbox row and
zero notices/reviews. Mail, OAuth and projection remain disabled. The API,
Postgres and web preview are healthy at `http://127.0.0.1:32880/`; the disposable
test database was removed after verification. Other worktrees were preserved.

## 2026-09-29 — LIFE-01 listing notice approval and mail guards

Added owner-only approval and outcome endpoints for saved listing-change notices.
Approval recomputes the reviewed digest under event/decision/owner/occurrence
locks and atomically inserts the notice, recipient ledger, outbox rows and a
succeeded local queue action. Exact request-key replay returns the same notice,
even after later listing edits. A new request or decision cannot create another
notice for the same occurrence revision. Suppressed recipients receive ledger
rows without outbox rows. Original decision-key replay remains compatible with
its initial draft action set.

The mail worker checks both owners, decision status, exact listing revision/CID,
approved content binding and the selected ticket/crew relationship immediately
before each provider call. First-attempt authority denial is withheld; denial
after a prior uncertain attempt is quarantined. Database errors preserve the
lease/body. Existing suppression, announcement consent and stable provider
idempotency/retry limits remain enforced. Queue success, provider acceptance and
delivery feedback have separate states.

The private UI provides explicit queue approval and per-recipient outcomes,
preserves request identity across a lost reply, requires re-preview after 409,
and allows outcome inspection after supersession. Shared contracts exclude
provider identifiers, leases, private reasons and request keys.

Final `bash scripts/dev-env.sh verify` passed: full `make verify`, including
254 web tests and 34 mobile tests, then the complete disposable DB gate with
402 top-level passing tests, 564 including subtests, zero failures/skips.
`GOFLAGS="-race -run=^(TestLifecycleNotice|TestEmail)" bash scripts/dev-env.sh test-db`
also passed: 21 top-level tests, 48 including subtests, no race reports or skips.
Regressions cover stale digest, duplicate approval, concurrent requests, complete
rollback after audit failure, upgrade/replay preservation, ticket/crew authority
loss, frozen retry identity/content, suppression and separate delivery feedback.

The T3 collaborative browser opened to a connection-error page and could not
navigate to the healthy preview server. The new approval journey is not browser
qualified; rendered UI regressions and real backend DB tests are separate proof.

Verified the existing API mounts this exact worktree with mail/OAuth/projection
disabled. Took a private custom-format PostgreSQL backup of its retained schema-24
development database and verified the archive catalog, then restarted only this
worktree's API. Startup applied migrations 25/26 and health returned 200. The
retained counts stayed one event, zero tickets and one held outbox message;
no notice was queued in that database. Postgres/web containers and other
worktrees were preserved. The backup catalog is an archive check, not a restore
drill or production rollback qualification.

Migration 26 adds notice/recipient ledgers and `withheld_authority`; older binaries
reject it. Logs and the private development backup are retained under
`.cache/dev-env/lifecycle-notice-approval/`. No live mail, hosted CI, remote push
or production deployment occurred. Live provider qualification, operator
reconciliation controls and coordinated operator-event changes remain open.

## 2026-09-29 — LIFE-01 listing notice previews

Added owner-only notice previews for an already saved listing cancellation or
reschedule. The server checks the active original decision owner, exact listing
revision/CID and matching status, renders listing-only content, and derives up
to 500 deduplicated ticket/assigned-crew recipients. Suppression is visible;
invalid mailboxes block review. Private decision reasons, staffing notes and
application messages never enter the notice. A review digest binds content,
audiences, recipient relationships and suppression from a consistent snapshot.

The private worklist exposes audience selection, message/recipient review and
stale-response errors. It clears a preview when its audiences change and clears
private state on access denial. No approval, queuing or sending control was
added. Schema remains 25; previews create no outbox rows or action attempts.

`bash scripts/dev-env.sh verify` passed against the final code: full
`make verify` (253 web tests, 34 mobile tests, configured Go checks, contracts,
build and Compose checks) and the full disposable database gate (395 top-level
passing tests, 544 including subtests; zero failures/skips). Three focused
notice-preview DB tests also passed under the race detector. The first web
rendering check failed because React SSR inserts text-separator comments; the
assertion now checks the rendered text after removing those comments.

The T3 browser checked desktop preview rendering, suppression labels, audience
change clearing and stale-listing errors against controlled synthetic responses.
A later narrow-screen check lost the browser connection with
`net::ERR_CONNECTION_REFUSED`, while host HTTP and the existing preview services
remained healthy. Responsive layout and browser access-denial behavior were not
qualified in this run. The real backend handlers were exercised by the DB gate.
Logs and the disposable browser fixture are retained locally under
`.cache/dev-env/lifecycle-notice-preview/`; the temporary served HTML was removed.
Existing preview containers and all other worktrees were preserved.

Approval/atomic queuing, send-time checks, per-recipient outcomes and
reconciliation remain the next LIFE-01 slice, described in
[the notice plan](../superpowers/plans/2026-09-29-lifecycle-notices.md).
No live messages, hosted CI, push or deployment were performed.

## 2026-09-29 — LIFE-01 destination-scoped dispatch infrastructure

Added an internal dispatcher with explicit dispatch approval, exact destination
selection, a finite adapter attempt budget, current owner and occurrence
revision/CID checks, fenced completion and independent action outcomes. Migration
000025 keeps every existing worklist action draft-only. Adapter errors and
malformed outcomes become unknown; bounded receipt references remain internal
for reconciliation. The private worklist exposes approval, retry and completion
metadata while preserving its exclusion of payloads, leases and provider references.

Local `make verify` passed (252 web tests, 34 mobile tests, backend checks,
contracts, builds and Compose validation). Full disposable `make test-db` passed
392 top-level tests, with zero failures/skips. The focused race gate
`^TestLifecycle(Action|Change|Intent|Dispatch)` passed 14 top-level tests / 29 with
subtests, with zero failures/skips or race reports. A separate attempt to run the
entire database suite under `-race` reached Go's ten-minute package timeout during
`TestNotificationLedgerAPI`; it did not complete and is not full-suite race proof.
Its output and both successful database logs were retained locally.

No runtime adapter, approval endpoint, worker, live email, public write or refund
is enabled. #50 remains open for approved notice content and relationship-derived
recipients, suppression, per-recipient delivery outcomes and destination
reconciliation. See [the lifecycle boundaries](event-lifecycle-changes.md).

## 2026-09-28 — IDENT-02 identity email app links

Verification and recovery email links now open the native app when it is
installed and the domain association verifies, and fall back to the existing
web pages otherwise. The API serves `/.well-known/apple-app-site-association`
and `/.well-known/assetlinks.json` from `MOBILE_APPLE_APP_IDS`,
`MOBILE_ANDROID_PACKAGE` and `MOBILE_ANDROID_CERT_SHA256`, claiming only
`/verify-email` and `/recover-password` with a token; each document is 404
until configured. `mobile/app.config.ts` adds iOS associated domains and an
Android `autoVerify` intent filter when built with `SUBCULT_APP_LINK_HOST`.
The app gains a recovery screen (request a link, or set a new password from
one) and a sign-in entry point; a rejected recovery token keeps the device
session. `scripts/device-link.sh` prints or opens the newest held identity
link for a synthetic account in a disposable stack.

Setup and the device checklist are in
`docs/development/mobile-app-links.md`. No Apple Team ID, signing
fingerprint, HTTPS host or physical device was exercised; the IDENT-02
real-device line remains open.

## 2026-09-24 — Expansion #50–71, first LIFE-01 slice

The owner promoted #50–71 for development ahead of the original roadmap order.
The work order is in `docs/superpowers/plans/2026-09-24-expansion-50-71.md`;
source work begins with #50 while deployment/provider gates remain separate.

Added the cancellation/reschedule state matrix in
`docs/development/event-lifecycle-changes.md`, separating public listings from
private event admission, provider tickets, money, notices and archive history.
Occurrence edits now support optional preview revision/CID preconditions and
an atomic update guard, reject invalid IANA zones and unordered intervals, and
retain explicit-offset DST instants. Embedded zone data supports the minimal
runtime image. No migration or coordinated cancellation worker was added.

Before the fix, focused tests showed stale-preview fields rejected as unknown
JSON (400 rather than a supported conflict check) and an invalid timezone
accepted with 200. The final seven occurrence/lifecycle tests passed with
`-race` in the installed T3 disposable PostgreSQL environment on Kvant, including
one-winner concurrent edits and cancellation preserving private event/ticket
rows and the email queue. `make verify && make test-db` passed: 306 top-level
database tests, 430 including subtests, zero failures or skips; web 221 and
mobile 27 passed. Documentation links and `git diff --check` passed. Test
containers were removed; the current worktree and other environments stayed
in place. These are local results, separate from hosted CI.

The full operator cancellation/reschedule workflow, notice failure/retry
recovery, remote repository CID enforcement and provider refund qualification
remain open. No browser, live provider, refund or deployment result is claimed.

## 2026-09-24 — SIGNAL-01 workflow recovery and review

Recovered Claude workflow `wf_72566bfc-c74`, task `w9uanh8za`, from the
"Merge Apps via Roadmap" thread. Implementation was committed at `0fa2efa`;
the review agent failed before reviewing because usage credits were exhausted.
The implementation commits were fast-forwarded into the existing
`t3code/email-announcement-scheduling` branch without moving worktrees.

Three disposable PostgreSQL regression tests reproduced duplicate delivery after
re-consent, invalid withdrawal links after session-secret rotation, and a
single-connection dispatch timeout. Dispatch now selects one grant per address,
prefers an active grant, reuses its transaction for consent queries, and issues
random per-message withdrawal tokens whose hashes remain valid independently of
session secrets. Migration 14 adds the token table and an announcement/address
unique constraint. The synthetic journey now uses the HTTP consent handlers.

The revised implementation passed focused consent/announcement/delivery tests
and `make verify && make test-db` on Kvant through the installed T3 environment.
For this run only, the test recipe declared its disposable PostgreSQL service
and `TEST_DATABASE_URL`; the installed profile files were unchanged. The full
database gate passed 302 top-level tests (426 including subtests), with zero
failures or skips. `make verify` passed 221 web and 27 mobile tests, non-DB Go
tests, lint, contracts, builds and Compose validation. The tested snapshot
matched all 21 changed source files. Test containers were removed afterward.

No browser preview, hosted CI, live mail delivery or pilot result is claimed by
these local checks. The original workflow's claims below are historical.

## 2026-09-24 — #24 original workflow implementation (superseded by review above)

Chose verified email through the existing Resend outbox as the one
announcement channel (`docs/development/announcements.md`): it is the
only channel with a qualified consent grant type, delivery ledger,
suppression and provider adapter already in place; SMS is explicitly not
added. The cost basis is configuration, `ANNOUNCEMENT_UNIT_COST_CENTS`
(default `0`, meaning unknown), not an invented number.

Added migration `backend/internal/app/migrations/000014_announcements.sql`
(`minimumSchemaVersion` moved to 14): `announcements` (workspace-scoped
draft/scheduled/cancelled/dispatching/dispatched state machine, scheduling
and cancellation audit columns, `recipient_count`/`withheld_count`/
`estimated_cost_cents` populated only at dispatch); `announcement_deliveries`
(links an announcement to each `email_outbox` row it produced, one row per
outbox row, no duplicated delivery/retry/suppression state); and a
nullable `consent_grants.withdraw_token_hash` with a partial unique index.

Implemented the withdraw-link design in `backend/internal/app/consent.go`:
`deriveWithdrawToken` computes a deterministic, per-grant public withdraw
token (HMAC-SHA256 keyed on the session secret, over the grant id) rather
than a randomly drawn one-time value, because `verification_token_hash`'s
raw token is never retained after the original confirm/unsubscribe email
and so cannot be reconstructed for a later announcement.
`mintWithdrawToken` derives it and returns its hash for storage;
`handleConfirmConsentGrant` mints and stores it at verification time, and
`handleWithdrawConsentGrant` now matches either `verification_token_hash`
or `withdraw_token_hash`, so both link generations resolve to the same
public withdraw route.

Added `manage_announcements` (owner and organizer, matching
`manage_consent`/`manage_delegations`) and, behind it,
`POST`/`GET /api/workspaces/{workspaceID}/announcements` (draft, list),
`GET .../announcements/{id}` (get, with delivery outcome counts joined
from `email_outbox.delivery_status` via `announcement_deliveries`),
`GET .../announcements/{id}/preview` (subject, body, current eligible
recipient count, estimated cost — never an address),
`POST .../announcements/{id}/schedule` (requires a future
`scheduledFor`), and `POST .../announcements/{id}/cancel` (allowed only
while draft or scheduled). `backend/internal/app/announcements.go`.

Implemented dispatch in `backend/internal/app/announcement_dispatch.go`
(`RunAnnouncementDispatch`, wired to a new `-announce` mode on
`backend/cmd/email-deliver`): claims due scheduled announcements with
`for update skip locked`, one per transaction; re-derives the raw
candidate audience fresh from every verified `announcement`-purpose grant
for the workspace (regardless of current withdrawn/suppressed state);
calls `checkSendPermission` per recipient as the actual final consent
gate (a grant withdrawn, or an address suppressed, since scheduling is
counted as withheld here, not silently dropped); enqueues one
`email_outbox` row per allowed recipient with `purpose = 'announcement'`
and `workspace_id` set, with the recipient's withdraw link appended to
the body; records `announcement_deliveries`; and sets
`recipient_count`/`withheld_count`/`estimated_cost_cents`/
`status = dispatched`, all before committing. It never contacts the mail
provider; actual sending, its bounded retries, suppression and feedback
stay entirely with the existing `processEmailDeliveries` worker, whose
existing send-time `checkSendPermission` recheck still applies.

Added the optional `announcement-workers` Compose profile
(`email-deliver -announce -watch`), documented in `AGENTS.md` and
`README.md` alongside the existing `mail-workers` profile, which is
otherwise unchanged.

**Verification actually run:** `go build ./...`, `go vet ./...`,
`gofmt -l .` (clean), and the full `internal/app` package test suite
against disposable PostgreSQL (`go test ./internal/app/...`; this did not
run the complete `make test-db` target), including
`TestAnnouncementSyntheticJourney` (grant two recipients, confirm both,
schedule into the past-due window, preview shows 2, withdraw one
recipient's grant, dispatch produces exactly one `email_outbox` row with
`withheld_count = 1`, then `processEmailDeliveries` with a fake sender
accepts the one allowed row and never sends to the withdrawn address),
`TestAnnouncementCancelBeforeDispatchProducesNoRows`,
`TestAnnouncementRequiresManageAnnouncementsPermission` (403 for a
member without `manage_announcements`), `TestAnnouncementScheduleRejectsPastAndPresent`,
and `TestAnnouncementWithdrawLinkTokenWorksThroughPublicRoute`. `make
verify` was also run; see its own note below for the actual result.

**Remaining limits:** no SMS/second channel; `ANNOUNCEMENT_UNIT_COST_CENTS`
defaults to `0` (unknown, not free); no UI; no re-send, schedule-time
editing or per-recipient personalization beyond the withdraw link; no
throttling beyond the existing delivery worker's batch/lease behavior; no
live deliverability test or permissioned pilot has been run (blocked on
issue #7). See `docs/development/announcements.md` for the full list.

## 2026-09-24 — #23 verified channel consent and suppression semantics (CONSENT-01)

Added migration `backend/internal/app/migrations/000013_consent_grants.sql`
(`minimumSchemaVersion` moved to 12): a new `consent_grants` table (sender
`workspace_id`, `channel` currently constrained to `'email'`,
`recipient_address` stored plaintext like other private recipient columns,
`purpose` `announcement`/`transactional`, `scope`, verification token hash
plus `verified_at`, `disclosure_version`, `granted_at`,
`withdrawn_at`/`withdrawal_reason`, `source`, `created_by_person_id`, a
partial unique index enforcing at most one active grant per
`(workspace_id, channel, recipient_address, purpose)`); two additive
columns on the existing `email_outbox` table (`purpose` defaulting every
existing/legacy row to `'transactional'`, `workspace_id` nullable); and a
widened `email_outbox.delivery_status` check constraint adding the new
terminal `'withheld_consent'` status. No existing row's status, purpose or
send behavior changes.

Implemented the central `checkSendPermission(ctx, workspaceID, channel,
recipient, purpose)` (`backend/internal/app/consent.go`): suppression in
`email_suppressions` denies unconditionally regardless of purpose or
grant; `transactional` always passes; `announcement` requires a verified,
unwithdrawn `consent_grants` row matching workspace, channel, recipient
and purpose exactly. The function's only two queries read
`email_suppressions` and `consent_grants`; it never joins tickets,
contacts, `event_role_applications`, atproto identity links or
`workspace_members`.
`TestCheckSendPermissionNeverConsultsUnrelatedTables` plants a ticket
holder, a contact and a workspace member sharing one address and proves an
announcement to that address is still denied without a real grant.

Wired the same check into `processEmailDeliveries`
(`backend/internal/app/email_delivery.go`) as a send-time recheck: every
claimed row is rechecked immediately after leasing and before the
provider is called; a denial marks the row `withheld_consent` with
`last_error_code` `consent_required` or `recipient_suppressed`, clears its
body, and never calls the provider or retries. Added operator endpoints
`POST`/`GET /api/workspaces/{workspaceID}/consent-grants` (new
`manage_consent` permission, granted to owner and organizer like
`manage_delegations`) and public tokenized
`POST /api/public/consent/{token}/confirm` and
`POST /api/public/consent/{token}/withdraw` (no session, generic 404
for an unknown or withdrawn token, no other grant field ever revealed).
Creating a grant enqueues a `transactional` verification email (reusing
`enqueueEmail`), so establishing a grant never itself requires one.

Verification: `go build ./...`, `go vet ./...`, and the disposable-Postgres
`make test-db` suite (Go 1.26.6, PATH override per AGENTS.md) — the full
existing suite passes unchanged against the new migration, plus new tests
in `backend/internal/app/consent_integration_test.go` covering unknown
grant, wrong purpose, unverified grant, verified-grant success,
withdrawn-grant denial, suppression overriding a valid grant, the
unrelated-tables non-derivation proof above, the full operator/public
HTTP lifecycle (create → 409 on duplicate → confirm → list → withdraw →
idempotent re-withdraw → confirm-after-withdrawal fails closed), unknown
public tokens revealing nothing, and the send-time recheck (accepts a
verified grant, withholds on revoke-before-send, withholds on wrong
purpose, withholds on unknown grant, and leaves transactional rows
unaffected).

Limits: no code path enqueues an `announcement`-purpose message in this
slice — this is the permission boundary only, not a send feature
(SIGNAL-01, issue #24, is the future work that sends). No audit-redaction
fixture exists. `consent_grants` is not yet wired into the access-export
or account-deletion behavior in `data-lifecycle.md`, because that pipeline
itself is not implemented. `disclosure_version` is an opaque label with no
disclosure-text storage or re-consent workflow. `sms` is named in the
channel design but not implemented.
## 2026-09-24 — #17 review fixes: migration renumbering, bounded backfill/rebuild, status_mismatch repair

Three review findings against the initial #17 slice, fixed on the same
branch:

1. **Migration renumbered `000013` → `000012`.** The initial slice assumed
   an unmerged, parallel `000012` slice would land first and deliberately
   left a gap; that slice never merged, so the gap-free migration loader
   failed `make test-db` (`migration sequence: got version 13, want 12`).
   Renumbered the file and `minimumSchemaVersion` (`db.go`) to `12` and
   updated `migrations/README.md`, `projection.md` and `decisions.md`
   accordingly, so this branch is self-consistent without depending on
   unrelated, unmerged work.
2. **Bounded backfill no longer runs its deletion sweep.** `backfillOnce`
   could exit a collection's page loop via `backfillMaxRecordsPerAuthority`
   or `backfillMaxPagesPerCollection` while the source still reported a
   non-empty cursor, then unconditionally mark every unseen local record
   `deleted` — silently dropping public rows from any authority with more
   records than one bounded pass could reach, every time. `backfillOnce`
   and `buildProjectionShadowState` now skip the deletion
   sweep / `extra` classification for a collection or DID whose listing
   exited with a non-empty cursor, and the run/diff is reported as a new
   `bounded` outcome instead of `completed`
   (`TestRunProjectionBackfillBoundedListingSkipsDeletionSweep`,
   `TestRunProjectionRebuildBoundedListingDoesNotReportExtra`). Added the
   `bounded` outcome to the `at_projection_runs` check constraint.
3. **Reconcile now repairs a `status_mismatch` whose CID is unchanged.**
   `applyCommitEvent`'s same-CID duplicate short-circuit fired regardless of
   the stored row's status, so a record a delete commit had marked
   `deleted` (CID untouched) stayed `deleted` forever even though the
   authority still listed it active with the same CID — reconcile reported
   `Duplicate`, not a write, and the run was still recorded `completed`.
   The short-circuit now also requires the stored row to already be
   `active`; replaying the authority's current record for a
   `status_mismatch` case reactivates it as intended
   (`TestRunProjectionReconcileRepairsStatusMismatchBackToActive`). Backfill
   shares the same fix for a locally-deleted record the authority still
   holds.

Verification: `go vet ./...` and `go test ./internal/app/... ./internal/atproto/...`
(disposable PostgreSQL via `TEST_DATABASE_URL`) both pass — 388 tests, 0
failures — with the migration now numbered `000012` and no renumbering
needed. Updated `projection.md` (backfill, rebuild/compare, reconcile,
run-ledger sections) to describe the `bounded` outcome and the
`status_mismatch` repair.

## 2026-09-24 — #17 AT projection backfill, rebuild and reconciliation

Added migration `backend/internal/app/migrations/000012_at_projection_recovery.sql`
(`minimumSchemaVersion` moved to 12): `at_projection_authorities` (`did`
primary key, `approved_by_person_id`, `approved_at`, `revoked_at`, `note`)
and `at_projection_runs` (`kind` backfill/rebuild/reconcile, `authority`
nullable, `started_at`, `finished_at`, `outcome`
running/completed/failed/gap, `counts jsonb`, `error`). Both tables are
additive and unreferenced by any prior code path. This slice was originally
authored as `000013` on the assumption that a separate, parallel `000012`
slice would land first; that slice was never merged, so review flagged the
gap-free migration loader failure it caused. Renumbered to `000012` (with
`minimumSchemaVersion` set to 12) so this branch is self-consistent and
does not depend on unmerged, unrelated work landing first.

Added `backend/internal/atproto/record_list.go`: `RecordLister` interface
plus `IdentityRecordLister`, the production `com.atproto.repo.listRecords`
client. Like the existing `IdentityRecordFetcher`, it resolves the
authority through the hardened identity directory and issues requests
through the shared public-only, no-proxy HTTP client
(`ssrf.PublicOnlyTransport`), so a PDS endpoint that resolves to a private,
loopback or link-local address is refused
(`TestIdentityRecordListerRefusesPrivatePDSEndpoint`). Listing is
cursor-paged with a bounded page size (100) and a bounded per-record size
(64 KiB, `RecordListMaxRecordBytes`, matching the stream path's bound).

Added `backend/internal/app/atproto_backfill.go`:
`ApproveProjectionAuthority`/`RevokeProjectionAuthority`/
`ListApprovedProjectionAuthorities` manage the allowlist.
`RunProjectionBackfill` lists all three admitted collections from an
approved authority's PDS and feeds every record through the existing
`ProjectionProcessor.ProcessEvent`, so Lexicon validation, the collection
allowlist, and quarantine are shared with the stream path; it writes its
own audit cursor under a distinct `at_projection_cursor` source row
(`"backfill"`) so it can never overwrite the live `"jetstream"` cursor, and
marks any record the authority no longer lists as `deleted` while
preserving provenance. `RunProjectionRebuild` builds an in-memory shadow
state from every approved authority and reports a `ProjectionDiff`
(`missing`/`extra`/`cid_mismatch`/`status_mismatch` URIs, never record
bodies) against `at_projection_records` without writing.
`RunProjectionReconcile` rebuilds the same shadow state and applies it,
scoped to approved authorities only. `RunProjectionMetrics` reports stream
lag (derived from the stored Jetstream `time_us` cursor), quarantine count,
records by status, the last run per kind, and the approved-authority count.
Every run is recorded as one `at_projection_runs` row.

Updated `backend/cmd/atproto-project/main.go`: added `-backfill did`,
`-rebuild`, `-reconcile`, `-approve-authority did -approved-by person-id
[-note text]` and `-revoke-authority did`; the no-flag default now prints
`RunProjectionMetrics` instead of the narrower prior status. `-run` is
unchanged.

Verification: `go build ./...`, `go vet ./...`, and
`go test ./internal/atproto/... ./internal/app/...` (full package,
disposable PostgreSQL via `TEST_DATABASE_URL`) all passed against the
`000012`-numbered migration in this worktree. New tests:
`record_list_test.go` (paging, oversize record, private-endpoint refusal,
no-directory failure) and `atproto_backfill_integration_test.go` (authority
approval gate, backfill storing/marking-deleted/account-migration/
source-outage/cursor-gap, rebuild diff, reconcile applying the diff scoped
to approved authorities, and a metrics-JSON no-private-data assertion).

Known limits: backfill always performs a full listing per invocation (no
persisted per-authority incremental resume cursor), so
`ErrProjectionCursorGap` is exercised only by fixture tests; the bundled
lister never returns it itself. `IdentityRecordLister` is exercised against
`httptest` fixtures only, never a live PDS. Metrics are command-output
only; no HTTP route exposes them in this change. Rebuild/reconcile hold the
full shadow state for all approved authorities in memory per call.
## 2026-09-24 — #18 anonymous cultural discovery and reservation handoff (UX-01)

No migration; `minimumSchemaVersion` stays 11. Added
`backend/internal/app/public_discovery_occurrences.go`: anonymous
`GET /api/public/discovery/occurrences` (list, excludes projection status
`deleted`/`unavailable`, optional `locality`/`limit`/`offset`) and
`GET /api/public/discovery/occurrences/{uri...}` (detail, still returns a
deleted/unavailable record with that status flagged) over the existing
DISC-01 `at_projection_records` mirror. Each item carries source (authority
DID + `at://` URI; no handle — the projection schema stores none), the
record's own status plus the projection status, start/end with timezone,
and a safe public location built only from the projected `tv.subcult.place`
record's own JSON (name/locality/region/country/coarse coordinates — the
Lexicon has no street-address field to leak).

Reservation handoff resolves through `event_public_links`: `fresh`/`changed`
mappings to a published local event return `{"kind": "local", "eventSlug",
"reservationPath"}` (the existing `tickets.go` reservation route); a
missing, stale (`invalid`/`unavailable`/`deleted`), or unresolvable mapping
always returns `{"kind": "none", "reason"}` and never falls back to a
different event —
`TestDiscoveryHandoffResolvesLocalReservationAndNeverCrossesEvents` plants
two occurrences sharing a display title, each correctly mapped to a
different local event, and asserts neither resolves to the other's slug.
External ticket handoff (`TICKET_HANDOFF_ALLOWED_HOSTS`) is not implemented:
the admitted `tv.subcult.event.occurrence` Lexicon has no ticket-URL field,
so there is nothing for a host allowlist to gate in this slice.

Added `contracts/api.schema.json`'s `PublicDiscoveryOccurrenceDTO` (required
fields plus a `forbidden` list covering ticket/contact email, street
address, access notes, staffing, settlement, invitation/OAuth tokens and
member role) and matching TypeScript interfaces in `web/src/domain.ts` and
`mobile/src/api/types.ts`; `scripts/check-contracts.mjs` passes.

Web: `web/src/views/DiscoverView.tsx` gained a `DiscoveryOccurrencesSection`
(list, a dependency-free inline-SVG coordinate plot, and a keyboard-operable
detail view with Escape/backdrop/button close and an explicit
reserve-or-unavailable handoff button), backed by pure helpers in
`web/src/modules/discovery/discoveryOccurrenceModel.ts`.

Verification actually run: `go build`/`go vet` clean;
`TEST_DATABASE_URL= go test ./...` and, against a disposable
`docker compose -p subcult-wf-18` PostgreSQL 17,
`go test ./internal/app ./internal/atproto -count=1` (all pass, 116.8s/6.6s);
`node scripts/check-contracts.mjs`; `pnpm --dir web run format`, `run lint`,
`run test` (221 tests, including the new discovery model and component
tests); `pnpm --dir mobile run lint`, `run test` (27 tests, unaffected). Full
`make verify` run recorded separately below by exit code.

Known limits: the `document`-level Escape-to-close listener and real
360px-viewport CSS behavior are not exercised by the vitest component tests
(this repository's existing component-test convention has no
jsdom/testing-library, so these need a real-browser check, not claimed
here); location/handoff resolution does one extra query per occurrence
(no batched join, acceptable at this slice's scale); `locality` filtering is
a substring match, not geocoded or bounding-box; `source.handle` is always
absent (the projection schema has no handle column); no native mobile UI
ships, only the matching contract. See
[`discovery-ux.md`](discovery-ux.md) for full detail.

### Fix — 2026-09-24: `locality` filter was dead SQL

The list route's `locality` clause compared `o.record ->> 'place'` (which
returns the `{"cid":...,"uri":...}` strong-ref JSON as text, never a bare
URI) against `p.uri`, so the exists-subquery could never match, and it never
compared `p.record ->> 'locality'` at all — `locality=Chicago` silently
degraded to the plain name-substring fallback. Fixed the join to
`o.record -> 'place' ->> 'uri'` and added the missing
`lower(p.record ->> 'locality') = lower($2)` comparison in
`public_discovery_occurrences.go`. Added
`TestDiscoveryListFiltersByPlaceLocalityNotOccurrenceName`, which plants an
occurrence named "Signal Night" at a Chicago place and one named "Denver
Night" at a Denver place, and asserts `?locality=Chicago` includes the
former (whose name never mentions Chicago) and excludes the latter. Updated
`discovery-ux.md`'s two locality-filtering descriptions to match the actual
place-locality join instead of the previously-documented (and unimplemented)
substring-only behavior.

Review follow-up: the initial send-time recheck in `processEmailDeliveries`
treated any non-nil `checkSendPermission` error as a consent denial, so a
transient database failure, a cancelled context, or (for an
`announcement` row with a null `workspace_id`) the resulting driver encode
error was recorded as a permanent `withheld_consent` row with its body
erased — a regression against existing transactional messages (identity
verification/recovery, ticket confirmations, invitations) staying
unchanged. Fixed by making `checkSendPermission` return
`ErrConsentGrantRequired` explicitly when `purpose` is `announcement` and
`workspaceID` is empty, and by having `processEmailDeliveries` withhold
only on `errors.Is(permErr, ErrConsentGrantRequired) ||
errors.Is(permErr, ErrConsentSuppressed)`; any other error is returned
to the caller and the row's lease is left to expire for retry, matching
the existing claim-failure path. Added
`TestProcessEmailDeliveriesInfraErrorDoesNotWithholdConsent` (a
test-only `App.consentCheckOverride` seam simulates the infra failure)
and `TestCheckSendPermissionRejectsAnnouncementWithoutWorkspace`.

## 2026-09-24 — #16 allowlisted, restart-safe AT record projection (DISC-01)

Added migration `backend/internal/app/migrations/000011_at_projection.sql`
(`minimumSchemaVersion` moved to 12): `at_projection_records` (`uri` primary
key, `did`, `collection`, `rkey`, `cid`, `rev`, `record jsonb`, `size_bytes`,
`status` active/deleted/unavailable, `first_seen_at`, `updated_at`,
`source_cursor`), `at_projection_cursor` (one row per source name), and
`at_projection_quarantine` (bounded/truncated payload, reason, cursor). None
of the three tables is referenced by any existing code path or written to by
the CRUD/public-preview code from MODEL-01.

Compared the raw firehose, Jetstream and Tap as the upstream stream adapter
against the pinned Indigo commit (`v0.0.0-20260903211445-41278964ec8e`, no
Tap client); chose a Jetstream-shaped JSON websocket consumer
(`atproto_projection_jetstream.go`, using `golang.org/x/net/websocket`,
already an indirect Indigo dependency and now promoted to direct — Indigo
itself was not upgraded) because it lets the server filter to the three
admitted collections and needs no Indigo surface. The stream client sits
behind a `StreamSource` interface; all processor tests use an in-memory
implementation, so no test opens a network connection. Full reasoning is in
[`projection.md`](projection.md).

`ProjectionProcessor.ProcessEvent` (`atproto_projection.go`) commits the
record write and the cursor advance in one transaction. Implemented and
tested: collection allowlisting (others advance the cursor without being
stored), Lexicon validation via the existing `atproto.ValidateAdmittedRecord`
(structural plus the public-field allowlist, catching a private-field leak
the same way MODEL-01's public-preview path would), a 64 KiB per-record
bound with a separately bounded quarantine payload, malformed-JSON
quarantine, duplicate `(uri, cid)` replay as a no-op, out-of-order `rev`
rejection, delete-marks-deleted-with-preserved-provenance (including a
delete arriving before any create), account deactivated/takendown/suspended
marking every record for that DID unavailable, terminal account deletion
that survives a later reactivation event, and bounded exponential-backoff
reconnect (`ProjectionProcessor.Run`). A dedicated crash/replay test injects
a failure after the row write but before commit, then resumes a fresh
processor from the stored cursor and proves no duplicate and no missing row.

Added `backend/cmd/atproto-project` (default: secret-free JSON status
mirroring `atproto-revoke`/`email-deliver`; `-run` consumes the stream,
gated by `AT_PROJECTION_ENABLED`) and the opt-in `atproto-projection`
Compose profile mirroring `atproto-workers`. `decisions.md`'s D10 row is now
A11 (Accepted); publication/reconciliation (D8, D9) and discovery UI
(UX-01) remain open.

### Migration numbering

This slice was built in parallel with #13, #14 and #15 and originally carried
migration `000012`. At integration it was rebased onto the merged #13 and #14
branches and renumbered to `000011`, because the privacy slice (#15) added
no schema change. `minimumSchemaVersion` is 11.

### Verification passed
- `go build ./...`, `go vet ./...`, `gofmt -l .` (clean) from `backend/`
- After the rebase and renumbering, `TEST_DATABASE_URL=<disposable> make test-db` on a disposable PostgreSQL: `internal/atproto` ok; `internal/app` 248 passed with one failure, `TestAuthorityMigrationWidensRoleAndAddsColumns`, which hardcoded schema version 10; it now asserts `minimumSchemaVersion`. The 17 projection tests (8 unit, 9 integration including the crash-mid-batch/replay-from-stored-cursor test) pass. The orchestrator's final counts are recorded on the pull request.
- `make verify` (deps, fmt, lint, `check-contracts`, backend/web/mobile/QA-script tests, `build`, `compose-config`, `open-pilot-check`): exit 0 on the pre-rebase branch; rerun after the rebase, result on the pull request.

### Remaining
`RunWithConnector` re-dials a fresh `JetstreamSource` at the last committed
cursor after a transient failure with bounded backoff; the adapter itself is
exercised by compile-time and in-memory tests only, not against a live
Jetstream endpoint, so its wire handling is unverified until a source is
qualified. No
backfill tooling, and no reconciliation between `at_projection_*` and
`cultural_*` — that remains PUB-01/UX-01, per D8-D9.
## 2026-09-23 — #15 privacy boundary audit and lifecycle spec (SEC-15)

Audited every anonymous/capability-token route in `backend/internal/app/app.go`, the MODEL-01 public preview, exports, request logging and telemetry against `docs/development/data-boundaries.md`; recorded the route-by-route trace in new [`privacy-audit-2026-09-23.md`](privacy-audit-2026-09-23.md). No export endpoint and no telemetry SDK exist in this codebase. Confirmed findings: `GET /api/dev/email-outbox` is not scoped to the caller's own workspace(s) in non-production environments (a cross-tenant plaintext-email leak in non-production environments, gated only by `AppEnv` and a session, not by workspace membership); fixed in this slice by scoping the query to the caller's own email, identity challenges and workspaces, pinned by `TestDevEmailOutboxScopedToCaller` because `email_outbox` has no `workspace_id` column to scope by without a migration, and the acceptance criteria called for an audit and fixes for leaks the criteria name, not a general route-hardening pass — recorded rather than silently dropped. No other anonymous route, the public preview, or request logging was found to leak a contact, staffing, attendance, consent, precise-location, incident-note, finance or plaintext-email field.

Added a reusable private-field sentinel fixture (`privacy_sentinel_test.go`, `plantPrivacySentinels`/`assertNoSentinelLeak`) that plants a unique sentinel in every private column class this repository stores today — contact email/notes, a role applicant's email/message, a ticket holder's email/display name, staffing notes, and MODEL-01's `cultural_place_protected_details` (street address, access notes) — and asserts none of them appear in `GET /api/public/events`, `GET /api/public/events/{slug}`, `GET /api/public/events/{slug}/roles`, the MODEL-01 occurrence public preview, or captured `log` output from a request whose query string and body both carry a sentinel email (`TestRequestLoggerNeverRecordsQueryOrBody`, pinning that `requestLogger` records only `method`/`route`/`status`/`duration`, never the URL or body). `TestCulturalPlaceProtectedDetailsRequireWorkspaceMembership` confirms `cultural_place_protected_details` is unreachable anonymously (401) and by a person outside the owning workspace (403), and readable only by an authenticated member of that workspace (criterion c); no code change was needed here because `handleListCulturalPlaces`/`handleCreateCulturalPlace`/`handleUpdateCulturalPlace` already gated on `requireWorkspaceRole(..., "owner", "member")` and the public projection already never joins the protected table — the tests close the previously-untested gap the brief identified.

Added [`data-lifecycle.md`](data-lifecycle.md): retention periods per data class, an access-export shape (what a requester can get and what is excluded because it belongs to someone else or is a system integrity record), account deletion (deleted vs. anonymized vs. retained-for-finance rows), correction, and an explicit statement that authoritative deletion of a local row or a future published AT record cannot erase downstream copies already fetched by relays/app views/other operators, plus what the system does instead (tombstone/status, stop projecting, notify via the protocol's own delete operation where applicable). No export/deletion pipeline is implemented; the document specifies the target behavior only, per the brief. `data-boundaries.md` now links both new documents. No migration was added: no test in this slice required a schema change, and the brief made `000011` conditional on that need.

### Verification passed
- `go build ./...`, `go vet ./...` from `backend/` (clean)
- `TEST_DATABASE_URL=<disposable> go test ./internal/app -count=1`: full package passes (146.9s), including the 3 new tests (`TestPrivacySentinelsNeverLeakIntoAnonymousRoutes`, `TestCulturalPlaceProtectedDetailsRequireWorkspaceMembership`, `TestRequestLoggerNeverRecordsQueryOrBody`)
- Disposable PostgreSQL used `COMPOSE_PROJECT_NAME=subcult-wf-15`/`POSTGRES_PORT=47015`, torn down with `docker compose -p subcult-wf-15 down -v` after the run
- `make verify` from the repository root after the toolchain `PATH`/`CI` export: exit 0

### Remaining
`GET /api/debug/mobile-auth` still has no environment gate; it exposes only the caller's own person id and is recorded in `privacy-audit-2026-09-23.md`. No consent table exists yet (`CONSENT-01` open), so the sentinel fixture has no consent column to plant. No access-export or account-deletion code was written; `data-lifecycle.md` is a specification for that future work.
## 2026-09-23 — #14 creator delegation and granular workspace authority (AUTH-01)

Added migration `backend/internal/app/migrations/000010_workspace_authority.sql`
(schema version 9 → 10): widens `workspace_members.role` from
`('owner', 'member')` to `('owner', 'organizer', 'finance', 'door', 'crew',
'member')`, keeping `'member'` as a permanent legacy alias for `'crew'` so
every existing row and every existing literal `"owner"`/`"member"` call site
keeps working unmodified; adds `expires_at`, `revoked_at` and
`revoked_by_person_id` to `workspace_members`; and adds `creator_delegations`
(a cultural profile's scoped, time-boxed grant of the right to act for it to
a workspace), carrying the same composite `(id, workspace_id)` foreign key
pattern as migration 000008 so a delegation can never reference a cultural
profile from a different workspace. `minimumSchemaVersion` moved to 10.

Added `authority.go`: a least-privilege permission matrix
(`rolePermissions`) over five roles plus the legacy alias; the central
`authorize(ctx, personID, workspaceID, permission)` function and its
`activeMembership` helper, which treat a removed, revoked, or expired
membership identically as absent; `requireWorkspaceRole` rewritten on top of
`activeMembership` so every pre-existing call site's literal role strings
keep working while gaining expiry/revocation enforcement; and
`authorizePublicWrite(ctx, actorPersonID, workspaceID, profileID)`, the hook
the future PUB-AUTH (#19) publication outbox will call, denying on revoked/
expired membership, missing `publish` permission, or a missing/expired/
revoked creator delegation. Added `authority_handlers.go`: member role/
expiry change and revocation endpoints (owner-only, with a transaction-locked
last-owner guard blocking demotion, revocation, or removal of the sole
active owner), and creator delegation list/create/revoke endpoints
(owner/organizer via `manage_delegations`, delegation creation rejects a
cultural profile from another workspace).

`docs/development/authority-model.md` records the four separated concepts
(account control, workspace permission, creator delegation, real-world
organization claim), the permission matrix, expiry/revocation/owner-
departure/recovery rules, and known limits — most importantly that
pre-AUTH-01 endpoints still gate on literal `"owner"`/`"member"` role
strings. `requireWorkspaceRole` treats the literal `"member"` as any active
role carrying `operate`, so a member promoted to `organizer`/`finance`/`door`
keeps baseline access to the existing surface, but finer-grained enforcement
on that surface (for example keeping `door` out of settlement routes) waits
for each call site to move to permission-based checks in a follow-on slice. `decisions.md`'s D8 row now points to this doc as
partial evidence toward its recommended starting point (workspace
permission and creator delegation); D8 remains Open, and current scoped
OAuth (the third element of D8) is not implemented here.

`backend/internal/app/cultural_model_integration_test.go`'s
`TestCulturalModelFreshMigrationCreatesTables` asserted schema version `== 8`
after a fresh migration; changed to `>= 8` since later slices (including
this one) legitimately advance the version further.

### Verification passed
- `go build ./...`, `go vet ./...` from `backend/`
- `TEST_DATABASE_URL=<disposable> make test-db`: 213 tests, 0 failures,
  both `internal/app` and `internal/atproto` `ok`, including 7 new tests
  (fresh-migration role/column check, revoked-member-denied-everywhere,
  expired-membership-denied-everywhere, last-owner-cannot-be-removed-
  demoted-or-revoked, authorizePublicWrite-denies-each-reason, role-change-
  restricted-to-owner, delegation-endpoints-restricted-and-scoped)
- `make verify`: deps, fmt, lint, `check-contracts`, `test`
  (backend/web/mobile/qa-scripts), `build`, `compose-config`,
  `open-pilot-check` all passed; web 200/200, mobile 27/27; exit 0
- Disposable PostgreSQL used `COMPOSE_PROJECT_NAME=subcult-wf-14`/
  `POSTGRES_PORT=47014`, torn down with `docker compose -p subcult-wf-14
  down -v` after the run. A local-only placeholder
  `000009_placeholder_local_test_only.sql` (a no-op `select 1;`) was used
  transiently to satisfy the gap-free migration sequence check while this
  worktree lacks the parallel slice that actually owns version 9; it was
  deleted before committing and is not part of any commit.

### Remaining
Every pre-AUTH-01 endpoint (events, contacts, tickets, staffing, cultural
CRUD, etc.) still checks literal `"owner"`/`"member"` role strings rather
than routing through `authorize`/`requirePermission`; `organizer`, `finance`,
and `door` are only meaningfully usable through the new endpoints and direct
`authorize` calls until that migration happens. There is no account-recovery
flow beyond promoting a second owner before the original steps down. No
publication outbox exists yet for `authorizePublicWrite` to be wired into
(PUB-AUTH, #19). No web/mobile UI was added for any of this.
## 2026-09-23 — #13 private event-to-public-occurrence links (LINK-01)

Added migration `backend/internal/app/migrations/000009_event_public_links.sql`
(schema version 8 → 9): `event_public_links`, one row per private operator
`events` row linked to a public `tv.subcult.event.occurrence` AT record,
carrying `public_uri`, `observed_cid`, the resolved authority DID and
freshness fields (`status` in `fresh`/`changed`/`unavailable`/`deleted`,
`observed_at`, `last_checked_at`, `last_error`). Composite
`(event_id, workspace_id)` foreign key back to `events (id, workspace_id)`,
matching migration 000008's pattern; `unique (event_id, public_uri)` makes
repeated attach idempotent at the database level. `minimumSchemaVersion`
moved to 9.

Added `backend/internal/atproto/record_fetch.go`: a `RecordFetcher`
interface plus `IdentityRecordFetcher`, the production implementation,
which resolves a record's authority through the existing hardened
`identity.Directory` and fetches it through `com.atproto.repo.getRecord`
using the same public-only outbound HTTP client and identity hardening AT
OAuth already uses (`hardenIdentityDirectory`/`publicOnlyHTTPClient`).
Tests supply a fixture `RecordFetcher`; nothing in this repository's test
suite makes a live network call for this feature.

Added `backend/internal/app/event_public_links.go`: workspace-scoped
`POST .../preview` (resolves, Lexicon-validates and returns source
identity/CID/event fields without persisting anything), `POST
.../public-links` (attach, idempotent via `on conflict do update` no-op),
`GET .../public-links` (list), `DELETE .../public-links/{linkID}`
(detach), and `POST .../public-links/{linkID}/refresh` (re-fetches and
updates only this row's status/freshness columns: same CID → `fresh`,
different CID → `changed`, `RecordNotFoundError` → `deleted`, any other
fetch failure → `unavailable` with `last_error` recorded). All five routes
reuse the existing `requireWorkspaceRole(..., "owner", "member")` gate, so a
person outside the event's workspace gets `403`, matching the MODEL-01
cultural routes. Full route/table detail, the fetcher boundary and known
limits are in [`public-links.md`](public-links.md).

### Verification actually run

- Go 1.26.6 `go build ./...` and `go vet ./...` (backend).
- Disposable PostgreSQL (`docker compose -p subcult-wf-13`, port 47013):
  `go test ./internal/app ./internal/atproto -count=1 -v`, including the
  new `backend/internal/app/event_public_links_integration_test.go`
  (preview does not persist; preview rejects a record missing required
  Lexicon fields; attach is idempotent across two identical requests;
  cross-workspace access to preview/list/attach/refresh/detach all return
  `403`; list/detach; refresh detects a changed CID, an unavailable
  fetcher error, and a deleted (`RecordNotFoundError`) record, and confirms
  event editing stays usable after a link is marked `deleted`; preview
  rejects a handle-authority URI). All passed; container removed after the
  run (`docker compose -p subcult-wf-13 down -v`).
- Full `make verify`; see below.

### Remaining limits

- API-only; no web/mobile UI.
- Refresh is manual (`POST .../refresh`); no background scheduler
  re-checks links automatically.
- `authority_did` reflects the identity resolved at attach/refresh time; it
  is not re-verified against identity rotation except on the next refresh.

## 2026-09-23 — #12 minimal cultural record model (MODEL-01)

Added migration `backend/internal/app/migrations/000008_cultural_model.sql`
(schema version 7 → 8): `cultural_profiles` (a `kind` column of
`creator`/`collective`/`act` stands in for a separate Act table, per D6's
recommended starting point), `cultural_places` (public fields only) plus
`cultural_place_protected_details` (street address/access notes in a
separate table the public serializer never joins), `event_occurrences`
(relates to an existing private `events` row; one event may have more than
one occurrence; nullable `public_uri`/`public_cid` left unset), and
`event_occurrence_profiles` (multi-host attribution with role and sort
order). Every new table carries a composite `(id, workspace_id)` foreign key
back to its parent so a cross-workspace reference is rejected by PostgreSQL
itself. `events` gained `unique (id, workspace_id)` to support this.
`minimumSchemaVersion` moved to 8.

Added workspace/event-scoped CRUD handlers
(`cultural_profiles.go`, `cultural_places.go`, `cultural_occurrences.go`)
reusing the existing `requireWorkspaceRole` ownership gate, and a public
projection (`cultural_public_projection.go`) that builds the exact
`tv.subcult.profile`/`.place`/`event.occurrence` record shapes from
public-only columns and validates them with the existing
`atproto.ValidateAdmittedRecord`, exposed read-only at
`GET .../occurrences/{id}/public-preview`. Full table/route inventory,
the DST/reschedule/sentinel reasoning and known limits are in
[`cultural-model.md`](cultural-model.md). `decisions.md`'s D6/D7 rows now
note that this slice implements their recommended starting points; D6/D7
remain Open. D5/ADR 0007 (Lexicon admission itself) was accepted the same
day as A10. No web/mobile UI and no contract-schema DTO were added.

### Verification passed
- `go build ./...`, `go vet ./...`, `gofmt -l .` (clean) from `backend/`
- `TEST_DATABASE_URL=<disposable> go test ./internal/app ./internal/atproto -count=1 -v`: 205 tests, 0 failures, both packages `ok`, including the 11 new tests (fresh migration, upgrade path preserving a pre-existing event, profile/place CRUD, duplicate occurrences + multi-host credit attach/detach, reschedule-does-not-touch-tickets, cross-workspace rejection, DST round-trip, public-preview sentinel leak check, public-preview-requires-a-credited-profile)
- `make verify`: deps, fmt, lint, `check-contracts`, `test` (backend/web/mobile/qa-scripts), `build`, `compose-config`, `open-pilot-check` all passed; web 200/200, mobile 27/27
- Disposable PostgreSQL used `COMPOSE_PROJECT_NAME=subcult-os-model`/`POSTGRES_PORT=47432`, torn down with `docker compose -p subcult-os-model down -v` after the run

### Remaining
Publication (writing any record to a PDS), projection/discovery ingestion,
reconciliation, and any web/mobile UI for profiles/places/occurrences are
still open (DISC-01/PUB-01/UX-01). Review embedded the admitted Lexicon
documents into the binary (`backend/internal/atproto/lexicons/`, kept
byte-identical to `contracts/lexicons/` by a test) so `/public-preview`
works in the production image without a contracts directory on disk;
`LEXICON_CONTRACT_DIR` remains an optional development override.

## 2026-09-23 — #11 Lexicon admission accepted (D5 → A10)

The repository owner accepted ADR 0007 on 2026-09-23, following its recommendation to admit the independently authored minimal `tv.subcult.*` chain (profile, place, event occurrence) rather than adopt the community calendar schemas wholesale. D5 moved from the Open table to the Accepted table as A10 in `decisions.md`; the ADR status, `atproto-kernel.md`, `lexicon-contract.md` and the architecture contract table were updated to say admitted instead of proposed. No schema, corpus, validator or runtime code changed in this step. Publication of any `tv.subcult.*` record still depends on D7 through D10 and on the MODEL-01, DISC-01 and PUB-AUTH slices.

## 2026-09-23 — #46 Indigo outbound-policy contribution package

New [Indigo outbound-policy contribution package](../upstream/indigo-outbound-policy/README.md), prepared from a disposable copy under `/tmp`, outside the repository. Source came read-only from the local Go module cache (`.blacktower/clonedeps` was absent in this worktree); nothing under the module cache was modified. Against the exact pinned commit `41278964ec8e3253e70d4e919dfb8e34211c543d` (`v0.0.0-20260903211445-41278964ec8e`, matching `backend/go.mod`), eight new standalone tests across two files (`oauth_outbound_policy_test.go`, `identity_outbound_policy_test.go`) demonstrate that all four reviewed request kinds — OAuth server metadata discovery (`oauth.Resolver.Client`), the OAuth PAR/token endpoint (`oauth.ClientApp.Client`), handle HTTPS well-known resolution and did:web (both `identity.BaseDirectory.HTTPClient`), and did:plc (`identity.BaseDirectory.PLCClient`) — follow a same-origin-unchecked redirect and honor an ambient `HTTP_PROXY`, using loopback `httptest` servers and no real network calls. `PLCClient` was additionally found to have no `Transport` set at all in `identity.DefaultDirectory()`, so it inherits `http.DefaultTransport` with no public-IP dial restriction, unlike the other three clients. All eight tests, plus the existing package suites, pass under `go test -race`.

Evaluated two proposal shapes: constructor/option-struct fields across three packages, versus a documented strict profile assembled from already-exported fields (the same technique Subcult OS already applies in `backend/internal/atproto/oauth_flow.go`). Recommended the latter, plus one small additive patch: `ssrf.StrictPublicOnlyTransport()` (`fix.patch`), which is `PublicOnlyTransport()` with `Proxy` forced to `nil` and changes no existing default. The patch applies cleanly via `patch -p1` (this worktree's git-operation guard blocks `git apply` outside the assigned worktree) and the patched `util/ssrf` package builds, vets and passes its existing test.

Searched `bluesky-social/indigo` issues and PRs for `CheckRedirect` and `ProxyFromEnvironment` (0 results each) and for `SSRF`/`PublicOnlyTransport` (13/2 results): no existing issue or PR addresses this proxy/redirect gap. The closest prior work — PR #1452 "harden identity package" and PR #1451 "util/ssrf: small improvements" (both merged 2026-09-01, already present in the pin) and open issue #1461 on reusable "safe" client singletons — covers dial-level SSRF and client-reuse ergonomics, not proxy or redirect policy. Fetched `main`'s `util/ssrf/ssrf.go` and `atproto/auth/oauth/resolver.go` directly: both match the pinned commit's behavior, so the gap is present on current `main`, not just the pin.

The contribution package README records this evidence, the proposal and compatibility analysis, the upstream search results with URLs, a submitter checklist, an AI-assistance disclosure, and a PR description draft. Nothing was filed, posted, or opened upstream. `docs/development/upstream.md`'s candidate bullet now links the package. Subcult's existing hardening (`newHardenedIndigoClient`, `hardenIdentityDirectory` in `oauth_flow.go`) already applies the equivalent strict profile in production and was not changed.

Remaining: all human-only submission steps in the package's checklist (authorization, GitHub account, DCO/CLA check, opening the issue first, final re-reproduction against then-current `main`, deciding between the two proposed shapes with maintainers). No upstream issue or PR was opened.

## 2026-09-23 — #45 Indigo persistence contribution reproduction

Re-reproduced the prepared [Indigo persistence contribution package](../upstream/indigo-persistence/README.md) from a disposable copy under `/tmp`, outside the repository. Source came read-only from the local Go module cache (`.blacktower/clonedeps` was absent in this worktree); nothing under the module cache was modified. Against the exact pinned commit `41278964ec8e3253e70d4e919dfb8e34211c543d` (`v0.0.0-20260903211445-41278964ec8e`, matching `backend/go.mod`), the existing synthetic test `TestStartAuthFlowRejectsUnstoredState` failed with `error=<nil>; want wrapped persistence failure`. After applying the existing `fix.patch` to a second disposable copy of the same commit, `go test -race ./atproto/auth/oauth -count=1` passed in full. Neither the test nor the patch needed changes.

Searched `bluesky-social/indigo` issues and PRs (open and closed) for `SaveAuthRequestInfo`, `StartAuthFlow persist`, `ClientAuthStore`, and `oauth persist error`: no issue or PR addresses this defect. One open PR (#1164) adds unrelated `state`-stashing behavior; one merged PR (#1159) adds a duplicate-state guard inside `MemStore` but does not check the error at the `StartAuthFlow` call site. Fetched `main`'s `atproto/auth/oauth/oauth.go` directly from GitHub: the unchecked `app.Store.SaveAuthRequestInfo(ctx, *info)` call is still present on `main`, not just at the pinned commit. The repository has no root `CONTRIBUTING.md`; the README's "Contributions" section is the operative guidance, and states no AI-assistance disclosure policy.

The contribution package README now records this reproduction, the upstream search results with URLs, a submitter checklist (authorization to publish, account, DCO/CLA recheck, issue-first, re-reproduce against current `main`, no internal references), an AI-assistance disclosure statement, and a PR description draft. Nothing was filed, posted, or opened upstream. Subcult's local fail-closed capture wrapper in `backend/internal/atproto/oauth_flow.go` was confirmed unchanged and its regression test (`TestOAuthFlowSurfacesIgnoredUpstreamPersistenceError`) still passes; `docs/development/upstream.md` now states it stays until an upgraded, separately verified Indigo dependency is pinned.

Remaining: all human-only submission steps in the package's checklist (authorization, GitHub account, DCO/CLA check, opening the issue first, final re-reproduction against then-current `main`). No upstream issue or PR was opened.

## 2026-09-23 — #11 minimal Lexicon admission proposal

Reviewed `community.lexicon.calendar.event`/`.rsvp` and `community.lexicon.location.address`/`.geo` (Lexicon Community, MIT License) as prior art; the `events.smokesignal.*` source repository returned HTTP 404 when fetched directly, so only unverified indirect descriptions of it exist and none were relied on. Recommended, and drafted, a minimal independently authored `tv.subcult.*` chain (`tv.subcult.profile`, `tv.subcult.place`, `tv.subcult.event.occurrence`) rather than adopting `community.lexicon` as-is, because it has no profile record, no independently addressable place record, and no bounds or public/private field distinction. Full reasoning, license and evidence are in [ADR 0007](../adr/0007-minimal-lexicon-admission.md) (status Proposed); field allowlists, bounds, and public time/location semantics are in [`lexicon-contract.md`](lexicon-contract.md).

Added `contracts/lexicons/*.json` (three record Lexicons) and `contracts/atproto-lexicon.fixtures.json` (`tv.subcult.profile`: 2 valid/8 invalid; `tv.subcult.place`: 2 valid/10 invalid; `tv.subcult.event.occurrence`: 3 valid/13 invalid; 38 cases total). Both `backend/internal/atproto/lexicon.go` (using the pinned Indigo `atproto/lexicon` package, confirmed present at the pinned commit) and `web/src/atprotoLexiconConformance.test.ts` (using `@atproto/lexicon@0.7.6`, added as a pinned `web/package.json` devDependency and installed into `web/pnpm-lock.yaml`) run the same corpus and agree on every case. Both validators additionally enforce a field allowlist derived from the Lexicon JSON's own `properties`, and an explicit-UTC-offset datetime check, because official Lexicon validation intentionally allows additive unknown fields and (in `@atproto/lexicon`'s case) a missing datetime offset; this discrepancy is recorded in ADR 0007 and `lexicon-contract.md` rather than papered over.

Updated `atproto-kernel.md`'s Lexicon boundary section and `decisions.md`'s D5 row to point at the ADR and contract doc. D5 is **not** marked Accepted; this slice is a complete, reviewable proposal; the repository owner accepts it by flipping ADR 0007 to Accepted. No database, migration or runtime handler changed.

### Verification passed
- `node scripts/check-contracts.mjs`
- `go test ./internal/atproto -run TestSharedLexiconConformanceFixture -count=1` (42 cases)
- `go test ./internal/atproto -count=1` and `go vet ./...`
- `pnpm run test -- atprotoLexiconConformance` (40 cases; 200/200 across the whole web suite)
- `make verify` (see PR/commit for the exact final tail)

## 2026-09-23 — #5 operations-panel API rehearsal

New `scripts/qa-operations.sh` (`make operations-qa`) rehearses the QUAL-BASE second acceptance bullet: workspace invitations and switching, contacts, commitments, staffing assignments, event templates, roles/applications, and reminder-sweep boundaries. Built and ran against a fresh isolated Compose stack (`subcult-os-qual`, ports 45432/48080/48079, disposable database `subcult_qa_operations_c97f07a`) from revision `c97f07a58a70c2b20090cc6809bc056329f84342`. The script produced 36 PASS/0 FAIL across two consecutive runs, exit code 0, covering: invite-then-accept plus client-side workspace switching by ID; contact create/list/update; commitment create/status-validation/completion; an owner-only staffing create/assign/PATCH boundary (member gets 403); template create/apply-to-draft plus a 409 once the target event is published; public role application submit/review with an invalid-status 400 and an owner-only decision boundary; and a reminder sweep that is role-gated (403 for members), state-gated (only overdue open commitments produce a reminder; a 30-day-future commitment produces none), and idempotent (a repeat sweep creates zero additional reminders).

This is API-level evidence only — no browser or native-device rehearsal was run for these panels. No product defects were found; all authorization and validation boundaries matched the code as read. Three bounded follow-ups are recorded in `docs/qa/operations-panels-2026-09-23.md`: the API has no recipient-level consent flag for reminders (only state/role gating), workspace "switching" is confirmed client-side-only with no server session state (documentation note, not a defect), and broader operations-panel browser/device coverage remains open for a future headed rehearsal. `make verify` passed after adding the script and docs.

## 2026-09-23 — #8 identity/AT signing key rotation

`IDENTITY_PROTECTION_KEY` (verified email encryption/lookup) and the AT OAuth
confidential client signing key now support a bounded rotation window rather
than an all-or-nothing swap. `identityProtector` and `atproto.OAuthStore`
each accept an optional previous key, try the current key first and the
previous key second on both decrypt and lookup-hash matching, and always
write under the current key. A new resumable `identity-rekey` command
(`backend/cmd/identity-rekey`, `backend/internal/app/identity_rekey.go`,
`backend/internal/atproto/oauth_rekey.go`) re-encrypts `email_identities`,
`atproto_oauth_sessions` and `atproto_oauth_revocations` with a keyset
cursor in bounded `FOR UPDATE SKIP LOCKED` transactions, is idempotent, and
prints aggregate counts only. Review added a test that a batch limit smaller
than the table still rewrites every previous-key row in one run. The AT OAuth JWKS
(`backend/internal/atproto/oauth_client.go`) can publish a previous public
key alongside the current one during a signing-key transition; the private
key that signs new assertions is always the current one.

Verified: unit tests for wrong-key rejection, tampered-ciphertext rejection,
current-only vs. current+previous decrypt and lookup during rotation, and
re-encryption idempotency (`backend/internal/app/identity_crypto_test.go`);
production config validation failing closed with a named-variable message
when `IDENTITY_PROTECTION_KEY` is missing or malformed, and accepting a
well-formed previous key (`backend/internal/app/app_test.go`); JWKS
containing both keys during a transition and only the current key afterward,
plus rejection of malformed/colliding previous-key settings
(`backend/internal/atproto/oauth_client_test.go`). Started a disposable
`docker compose up -d postgres` (throwaway database, not a retained
application database) and ran `go test ./internal/app ./internal/atproto
-count=1` with `TEST_DATABASE_URL` set: all 271 cases passed, including a new
end-to-end test that seeds email and AT OAuth rows under a previous key,
runs `identity-rekey`, confirms the previous key can then be removed while
reads still succeed, and confirms a second run changes nothing. `make verify`
passed locally.

Open and out of reach here: provisioning either key through a real deployed
secret store, a live rehearsal of a deployed rotation (this environment has
no deployed instance to rotate), and observing an actual AT Protocol
resource server accept a token signed against the previous key during a
JWKS transition (covered here only by asserting the JWKS document shape).
`atproto_oauth_requests` rows are deliberately not re-encrypted; they expire
in 10 minutes and are documented in `docs/runbooks/key-rotation.md` as a
bounded, self-clearing exception. The pre-existing plaintext `people.email`
column (a known prototype leftover, unrelated to this key) was left
untouched — protecting it was out of scope for this issue.

## 2026-09-22 — #121 verified rehearsal accounts

The real-backend checkpoint at `f206b4f` found both legacy rehearsal scripts failing with 401 after signup: they assumed signup issued a session. They now require explicit loopback/disposable-database opt-in, read the held verification message for their own synthetic recipient, and consume its challenge through the normal verification endpoint. They never mark identities verified directly, print tokens, or send mail. The shared helper rejects retained/remote targets and propagates database, missing-message and HTTP failures; its network-free tests run in `make verify`.

After correcting a column-name error in the first helper attempt, both free API rehearsals passed against `subcult_qa_batch_f206b4f` and the unchanged Go binary built from `f206b4f`. Coverage includes verified signup, invitation acceptance, publish, capacity rejection, door search/idempotent check-in, end-of-night counts, role visibility/review and staffing edits. These API checks do not replace served browser journeys, native-device tests or live provider qualification. The earlier 401 results remain evidence of the inherited script defect.

## 2026-09-22 — #119 payment-aware ticket presentation

Admission messages now require free or paid status. Pending, cancelled and unknown payment values never claim readiness or granted access, even when a prior check-in is recorded. Codes remain available for support with an explicit statement that they do not bypass payment. Cancelled-payment copy no longer invents a resumable checkout. Already scanned free/paid tickets say "Already checked in" rather than implying a new grant of access.

Full local `make verify` passed. Tests cover all four payment values plus unknown input across reserved/checked-in states, and rendered pending/cancelled pages reject conflicting ready-for-entry text. A browser with a synthetic pending ticket confirmed the corrected warning, payment explanation and support-code copy. No live payment, admission or deployment was performed. The five-issue batch now enters combined verification and hosted CI review before merge.

## 2026-09-22 — #117 ticket recovery

Ticket pages now provide an in-page read retry/refresh. A failed refresh retains the last loaded pass and explicitly warns that payment/check-in status may have changed. Changing ticket codes clears the prior ticket and QR; cancelled asynchronous loads cannot replace the new result. QR generation failure shows manual-code instructions instead of an indefinite preparation message.

Full local `make verify`, render regressions for QR association/failure and initial/refresh errors, docs validation and diff checks passed. A real browser with a synthetic API confirmed initial outage recovery, pending-ticket/QR rendering and retained pass plus warning after a failed refresh. QR failure rendering is unit-tested, not a browser-induced canvas failure. No reservation, payment, check-in or live email was issued; hosted CI and actual backend qualification remain separate.

## 2026-09-22 — #115 saved-state lifecycle transitions

Publish and end-of-night now require a clean, saved event form and explain why the action is disabled when edits remain. Save and lifecycle handlers reject overlapping actions, and event detail inputs are locked during their requests so a response cannot replace edits made in flight. No unsaved content is automatically published.

Full local `make verify` passed after correcting a type annotation in the new test fixture; the first failed check is retained in the local verification log. Rendering regressions cover both lifecycle actions with dirty forms and both busy states. A browser against the synthetic API showed publish enabled for a clean draft, disabled with the save-first explanation after a title edit, and enabled again when the saved title was restored. No publish, close, payment or live mail request was made during that browser check. Hosted verification is a separate gate.

## 2026-09-22 — #113 event editor read recovery

The report loader treats only a 404 as an absent report. Workspace authority failures are explicit rather than silently hiding owner tools. Both reads have independent error state and scoped retries; neither retry reloads the event form. Authority-dependent controls remain unavailable until the workspace read succeeds. An already known report snapshot is retained during a refresh, with any refresh error still visible.

Local `make verify` passed, including status/transport regression cases. Browser checks against a disposable synthetic API confirmed an authority outage, recovery of owner tools while an unsaved title stayed intact, and report outage/retry recovery. The fixture recorded no event refetch during authority recovery and only the report endpoint during report retry. These are controlled UI checks, not production or real-backend qualification. No live email or deployment was performed.

## 2026-09-22 — #111 workspace loading boundaries

Workspace selection now falls back only after an explicit 403 or 404. Server, authentication and transport failures remain errors, and fallback membership is loaded from the authorized workspace endpoint rather than fabricated from the account summary. Event and archive outages no longer become empty lists. The dashboard is committed only after its required overview loads, with a retry action for initial failures.

Local `make verify` passed. Fifteen loader cases cover selection errors, authorized fallback, overview outages and private-panel denial. A real browser against a disposable synthetic API confirmed event/archive error states, successful retry into the chosen workspace, no fallback on a selection outage, and an explicit notice when access denial permits fallback. This verifies controlled UI behavior, not production or a real backend lifecycle. Hosted CI remains a separate PR gate. No schema, live mail or deployment changes were made.

## 2026-09-22 — #105 signed email feedback

Added a disabled-by-default Resend webhook, raw-body signature verification and duplicate-safe minimal receipts. Tests use the independent published Svix vector and locally signed synthetic requests. Early receipts correlate after acknowledgement; adverse outcomes cannot be cleared by late delivery events. Suppression uses only our stored recipient, and workers exclude both durable suppression and unprocessed adverse receipts. Generic provider failures do not suppress a recipient.

The owner deferred live Resend setup. No account, DNS, live delivery or production changes are included. Full local and hosted verification precede the batch merge checkpoint. Minimal receipt retention and audited unsuppression remain follow-ups; no cleanup silently enables sending.

Hosted worker runs 9254/9255 exposed a test-fixture race: the version-five fixture executed the database-wide extension creation outside the migration advisory lock while AT integration tests ran in another package. The fixture now uses the same lock and one transaction. Production migration SQL and assertions are unchanged. The failed hosted results remain part of the record; verification is repeated against a fresh disposable database as well as the existing synthetic test database.

## 2026-09-22 — #104 durable transactional delivery

Added an opt-in delivery command and additive version-6 ledger. Historical and disabled-mode inserts remain held; enabled inserts freeze the sender and reply-to. Leased claims use row locking and fenced acknowledgements. Retries retain one provider idempotency key, stop after eight attempts or 23 hours, and respect identity-challenge expiry/consumption. Terminal outcomes clear message bodies. Aggregate status does not contact Resend.

Focused disposable-PostgreSQL tests passed under the race detector, including concurrent claims, stale acknowledgements, crash recovery, terminal failures and identity deadlines. The full verification and database gates are rerun before publication. Live Resend setup is deferred at the owner's request; no credentials, DNS, live mail or production migration is part of this change. Approved reply-to and controlled test recipient: `info@subcult.tv`.

## 2026-09-20 — Five-issue delivery batch after merge checkpoint

### #103 — Resend provider adapter

Added one standard-library Go HTTPS adapter with stable message idempotency, bounded responses/timeouts, no redirects/proxy and typed redacted errors. Synthetic transport cases cover successful acceptance, malformed/oversized responses, credential rejection, both idempotency conflict classes, throttling, provider failure and invalid input. See `docs/runbooks/transactional-email.md` for sourced contracts and setup boundaries. This does not activate a worker or live sending.

The owner approved batches of five to ten issues, followed by combined verification and merging before further feature work. Baseline: `966a111` on main, verified by hosted runs 8929/8930 and local database/race tests. Current batch: #101 reservation inventory, #102 protected-session expiry, #103 Resend adapter, #104 durable delivery, #105 signed delivery feedback/suppression. Each receives a stacked PR. Parent roadmap issues remain open where live-provider, device or deployment acceptance is still missing.

### #101 — Reservation inventory

Free RSVP now reloads authoritative event inventory instead of leaving pre-reservation counts or decrementing locally. A failed refresh preserves the issued ticket and labels availability unknown. Unit checks cover confirmed, failed-refresh and rejected-reservation cases. `make verify` passed (123 web and 27 mobile tests). A real Chromium browser against the disposable PostgreSQL/API/Vite runtime reserved the last ticket: both availability indicators changed to sold out while the confirmation and ticket link remained visible. This was synthetic local data, not delivery or production qualification.

### #102 — Protected operator session expiry

Workspace/event namespaces now authenticate before resource authorization: missing or expired sessions return 401, while authenticated foreign-workspace denial stays 403. The request-scoped identity avoids redundant authentication and preserves the server-owned logging route template. Anonymous media requests now require authentication before exposing adapter availability; the media test covers both that boundary and authenticated 503 behavior.

The complete database suite passed, including expired access, refresh, denied mutation and cross-workspace regression. Web tests prove one refresh/retry with the unchanged mutation payload and no retry for real 403. Chromium loaded an event after controlled access expiry and saved an allowed location edit after a second expiry; session generations rotated. A disallowed published-title change remained 409 and was not falsely called successful. No natural expiry soak, native device or production behavior is claimed.

## 2026-09-20 — ARCH-01 protocol failure isolation

Reused the complete local-event lifecycle assertion for a second scenario with an enabled AT link flow that returns a synthetic provider error. The AT request returns 502, then local create/publish/free-reserve/duplicate-check-in/closeout succeeds with one settlement/archive. The provider is called exactly once and no DID link is created. Focused and complete database gates plus `make verify` pass. This tests the existing application seam, not a live network partition or offline mobile mode.

The architecture document now distinguishes implemented account/session and AT seams from design-only cultural validator/projection/publication contracts. It removes obsolete prototype identity/status claims and explicitly defers unused interface packages until a real caller exists. No public schema, record mapping or PDS publication was added. Hosted baseline runs 8911/8912 passed at `226fdf4`; later PostgreSQL service qualification is still queued.

## 2026-09-20 — Hosted database verification gate

With the full local database suite restored, Gitea now declares a job-scoped PostgreSQL 17 service and runs the complete `make test-db` after `make verify`. The service has disposable test-only credentials, readiness checks, no host port and no retained volume. Test cases use private schemas. No application database, runner settings or deployment service is targeted.

Actionlint and local `make verify` pass; the immediately preceding expanded local database gate passes on isolated PostgreSQL 18. Hosted service-network and PostgreSQL 17 execution remain pending until this revision runs. Parent run 8911 has successfully provisioned Node/pnpm/Go and reached dependency/build verification, clearing the earlier missing-pnpm step; it is not yet a completed green run.

## 2026-09-20 — QUAL-BASE full database gate restored

Resolved all seven isolated failures from #95. Two product defects were confirmed: malformed event IDs reached PostgreSQL UUID conversion and returned 500, and publication preserved the database slug but incorrectly returned/audited the ID-derived fallback. Both normal and locking event loaders now reject malformed UUIDs as not found before querying; publication uses `RETURNING public_slug` so storage, response and audit agree. Regression tests retain the seeded-slug scenario and explicitly compare all three values.

The remaining setup corrections preserve behavior assertions: closed/private public applications are explicitly denied, then synthetic private rows perturb source state to prove archive immutability and roster filtering; paid-event privacy checks assert free-RSVP rejection and use a paid ticket fixture; member staffing authorization is checked through the supported PATCH method. Public-event privacy is observed before close and closed lookup must return 404. No public admission/payment policy was relaxed and no test was removed or skipped.

The full app database suite passed twice in one process: 336 passing test/subtest events, zero failures/skips. The expanded `make test-db` then passed for the complete app and AT packages; it no longer hides lifecycle cases behind a name filter. `make verify` passed (120 web, 27 mobile, configured Go/vet/build/contracts/Compose). Original failed ledgers remain unchanged; successful local ledgers are `/tmp/subcult-lifecycle-contracts-full.jsonl` and `/tmp/subcult-test-db-expanded.log`.

A separate actual Chromium rehearsal created a workspace and event, published, reserved a free ticket, opened ticket lookup, checked in through Door, and generated End of Night with reserved/check-in/no-show counts 1/1/0 and zero-dollar settlement. This rehearsal used the earlier running `07b83ea` API binary and current web sources (unchanged since `c7817d2`), not a rebuilt/deployed claim for these backend fixes. Broad API coverage qualifies additional workspace/privacy/staffing/template/role/reminder boundaries; physical-device and full browser coverage of those panels remain open under #5. The date field required DOM input-event entry because the preview typing helper misfocused the native date input; this is not native date-picker qualification.

## 2026-09-20 — Hosted CI toolchain setup

Run 8907/job 16364 failed immediately at `make deps-web`: `pnpm: command not found`. The workflow previously assumed Go/pnpm existed in the shared runner image. It now provisions Node 24 (matching the frontend image major), pnpm 10.33.0 (the web package pin), and locally qualified Go 1.26.6, reports versions, and retains the unchanged `make verify` gate. Added actions use verified upstream commit pins; no runner or global host configuration changed. Dependency caching is disabled for Go until the Gitea cache path is independently qualified. A 20-minute job bound and explicit CI environment prevent unbounded setup and interactive dependency prompts.

Local `make verify` and actionlint pass. Actionlint initially hit an unset mise shim; explicitly selecting its installed Go 1.25.13 tool environment passed without changing global defaults. Hosted execution must be read back after publication; this entry does not claim hosted success or full database qualification.

## 2026-09-20 — QUAL-BASE test isolation and retained failures

The broad app database run at `c7817d2` produced 151 passing / 11 failing test events. Unlike the identity tests, lifecycle fixtures shared the public schema and persisted data across runs. Each lifecycle fixture now owns a disposable schema through the existing migration helper. Secondary cross-workspace actors explicitly share the first application's database/session authority; they remain valid authenticated users, so access-denial checks do not degrade into invalid-session tests. The events-table migration check is also schema-local.

Isolation removes four failures (notification/reminder static keys and discovery counts): the same broad suite reaches 155 passing / seven failing events. Issue #95 retains the seven archive, paid-privacy, staffing, slug and private-role cases for behavioral reconciliation. Original local JSONL ledgers remain at `/tmp/subcult-lifecycle-baseline.jsonl` and `/tmp/subcult-lifecycle-isolated.jsonl`; no failing test was deleted or skipped. A new fixture test proves private schemas and valid authenticated cross-workspace denial. That test and the four repaired cases pass twice in one process. `make verify` passes; the full database suite remains red and the maintained narrow target is not presented as full coverage.

Hosted runs now finish but fail before tests: run 8907/job 16364 at `c7817d2` reports `pnpm: command not found`. CI toolchain setup is the next independent fix; local verification is not hosted success. No runner service was modified.

## 2026-09-20 — AUTH-RETURN same-origin navigation

Issue #93 records a browser-confirmed URL-normalization gap: the previous `next` check allowed slash/backslash and slash/control/slash inputs that Chromium resolves off-site. The new pure return-path validator rejects those forms, checks the parsed origin, and rejects network-path results after dot-segment normalization. It preserves valid invitation/workspace paths and query/fragment data. No cookie disclosure is claimed; this addresses post-authentication off-site navigation.

Seventeen hostile/valid-path cases pass. Actual browser sign-in with the malicious slash/backslash query remains on the local application origin and lands at the operator home. `make verify` passes (120 web and 27 mobile tests plus the full configured Go/build/contracts/Compose checks). This is a local fix; no production deployment occurred.

## 2026-09-20 — IDENT-02 web recovery and challenge qualification

The real Chromium/Vite Strict Mode journey reproduced two verification POSTs on one page load: one 200 and one rejected 401 replay. Verification now requires an explicit form submission, with an immediate in-flight guard; mounting the page does not consume the challenge. Successful verification replaces the token URL. Recovery success clears its query token and password field and removes the completed form. Recovery inputs have accessible names and result/error messages expose status/alert semantics.

On the disposable Unix-socket PostgreSQL 18 database, browser signup created no session before confirmation; one confirmation created exactly one session and reached the operator home. Recovery through the browser invalidated the existing session, rejected the old password, and accepted the new password. With only the synthetic account's access expiration advanced in the database, a reload refreshed the cookie session, advanced its generation from 0 to 1 and retained the authenticated workspace. This is controlled-expiration evidence, not a 15-minute natural soak. Development outbox reads bridged unsent emails; no external email was delivered. Tokens/passwords and account rows are not included in this record.

`make verify` passed (103 web tests, 27 mobile tests, Go/vet/build/contracts/Compose); `make test-db` passed including replay descendant revocation, revoke-one/all, recovery and account non-merging. Handler tests cover explicit submission, concurrent-submit suppression and missing tokens; actual browser behavior was checked separately. Issue #6 stays open: physical-device secure storage, restarts/deep links and native logout require real-device evidence. Hosted CI, production email and production deployment are not claimed.

## 2026-09-20 — UP-STATE reduced upstream reproduction

Prepared an original no-network test against Indigo's actual `StartAuthFlow`, rather than only a fake application runner. The pinned/current upstream commit ignores a failing store and returns nil error; the test reproduces that failure. A two-line error-propagation patch applied to a disposable source export makes the complete OAuth race suite pass. The inspected read-only dependency clone and application module pin remain unchanged. Current README contribution guidance requests issue discussion before an upstream PR; the package includes a submission draft and AI-assistance disclosure, but no external maintainer was contacted.

## 2026-09-20 — Request log privacy follow-up

Issue #89 records a concrete leak observed during lifecycle testing: the shared request logger wrote invitation/ticket values and linked DIDs from raw URL paths. It now writes only server-owned route templates, method, status and duration; unmatched, method-mismatch and pre-routing-denied requests use a constant marker. Query strings and path values are never a fallback. Focused race tests capture actual log output across those cases, and `make verify` passes. This changes future application logs only; production rollout and historic log retention are separate work.

## 2026-09-20 — AT-REVOKE worker and provider adapter

Added bounded leased processing, acknowledgement fencing, exponential retries, terminal quarantine and credential erasure. The SDK-backed adapter revokes access and refresh tokens with confidential-client assertions and DPoP nonce handling through the hardened public-only transport. Raw provider errors never enter the queue or command output. An opt-in worker command supports status, one-shot and watch modes using the same API image/database; disabling new OAuth links does not prevent draining existing work.

`make verify`, `make test-db`, the complete AT package race suite, the optional Compose profile render and a real CLI watch/SIGTERM smoke passed. Tests cover transient backoff, terminal exhaustion, unsupported providers, malformed encrypted payloads, retention expiry, crash recovery including the eighth attempt, concurrent workers and late-rotation fencing. The CLI produced aggregate counts with new links disabled. Docker became inactive after the earlier PostgreSQL 17 storage checks; worker tests used a signature-verified standalone PostgreSQL 18.6 package on a private Unix socket with no TCP listener. No shared daemon was started. The image was not built or deployed in that environment; real provider acceptance and production worker qualification remain open.

## 2026-09-20 — AT-REVOKE durable storage slice

Migration 5 adds an encrypted revocation outbox. Local unlink atomically transfers every active session payload to the outbox, revokes the DID and removes active sessions. DID-scoped transaction locks serialize unlink and session persistence. A late refresh cannot reactivate the link: it updates pending encrypted revocation material and invalidates the older worker lease. Active-session reads also require a current active DID link.

Disposable PostgreSQL race tests prove failed enqueue rolls back unlink, secrets are not recognizable plaintext, late rotation fences stale work, and concurrent unlink/refresh leaves no active session. This slice intentionally does not execute network revocation; the next stacked PR adds bounded processing, provider handling and operator-visible status. Production OAuth remains off pending the real-provider gate.

## 2026-09-20 — BASE-01 reconciliation and stacked review

The main checkout moved to `Work/Subcult/subcult-os`; `git worktree repair` corrected this worktree's stale lowercase pointer without changing commits or user files. Remote main remains `abf3f50`; twelve local commits through `06816d2` contain the research and platform foundations. Research is separated at `13f88b7` into the first review branch, with the platform branch stacked above it. The Gitea roadmap now tracks the remaining work and supersedes the old statement that no hosted issues exist.

Reconciled implemented migration/identity decisions and stale AT HTTP/UI and aggregate-test statuses. `make verify` passed using the installed Go 1.26.6 toolchain with system pnpm on PATH (99 web tests, 27 mobile tests, Go tests/vet/build and Compose validation). Two earlier environment-only attempts failed because Go was absent from PATH or unset in mise; no dependency or package-script policy was weakened. All three documentation validators pass. Live-provider, physical-device and cutover gates remain open; no deployment is implied by PR publication.

## 2026-09-20 — Live legacy deployment reconciliation

Read-only host inspection corrected the historical deployment assumption. The NUC has no running Subcults containers or listeners on 3024/3025. Active Almaz Caddy instead routes Subcults API/health traffic to Dozor `10.0.0.57:3025` and frontend traffic to `10.0.0.57:3024`. Dozor runs the six-service `subcults` Compose project from `/srv/containers/subcults`; all roles were running with zero restarts, and exact current image IDs are recorded in the cutover runbook.

The live PostgreSQL database is at migration 46 with `dirty=false`. Row-count-only queries found zero users, events, profiles, OAuth links, OAuth sessions and OAuth requests. No credential values or personal rows were read. This evidence removes the need to design a retained-user migration for the currently deployed database, but it does not authorize deletion and must be reconfirmed at cutover. The unprivileged account could not inspect the protected backup directory, so a fresh backup plus isolated restore remains mandatory. No service, image, proxy, database or configuration was changed.

## 2026-09-20 — AT identity link lifecycle UI

Authenticated users can now list active AT Protocol links, start an identity-only authorization from the operator home, and unlink through a two-step confirmation. The UI says explicitly that a DID proves account control but grants no workspace membership or publishing authority. Callback result copy is fixed locally and never reflects provider text. Disabled installations receive no panel because the capability routes remain behind `ATPROTO_OAUTH_ENABLED`.

Local unlink transactionally marks only the current person's DID revoked, deletes every matching encrypted OAuth session, and records an `atproto_did_unlinked` audit event. This fails closed across accounts and does not depend on provider availability. It does not yet revoke tokens at the provider, so the production flag remains off until a bounded live revocation design is qualified. `make generate-atproto-key` now emits the required multibase P-256 secret for direct secret-manager capture.

A named disposable PostgreSQL 17 container passed the link-list/unlink database tests and focused race runs, then was removed. An independently named disposable Compose stack passed a headed Chromium journey for signup, verification, login, panel rendering, callback-success copy, invalid-identifier feedback, a synthetic linked-state fixture, two-step unlink, and UI/DB audit readback. The synthetic DB fixture qualified presentation and local unlink only; no external resolver, PDS, OAuth grant or provider revocation was exercised. The browser, containers, network and disposable volume were removed afterward.

## 2026-09-20 — AT OAuth start and callback boundary

Subcult OS now exposes an authenticated `POST /api/v1/auth/atproto/start` and state-bound `GET /api/v1/auth/atproto/callback` when AT OAuth is explicitly enabled. Start accepts only a normalized handle or DID and binds the already-authenticated local person into the encrypted request. Callback delegates PAR, PKCE, DPoP, issuer, subject and token processing to the pinned Indigo client, while the OS store enforces one-time state, exact identity-only scope and non-merging DID ownership. Browser completion uses a fixed `/workspace?atproto=` landing and does not reflect untrusted provider error descriptions.

The adapter discovered that pinned Indigo ignores the error returned by `SaveAuthRequestInfo` in `StartAuthFlow`. An isolated capture wrapper now fails closed if persistence fails or never occurs; the regression is recorded as an upstream candidate. The adapter also replaces SDK HTTP defaults with public-IP-only, no-proxy, no-redirect clients across OAuth and identity discovery to close environment-proxy and redirect rebinding gaps.

Non-database tests and focused race tests passed. A named disposable PostgreSQL 17 container passed `make test-db`, including authenticated-person binding, plus race-enabled OAuth store tests; that container was removed. An existing local Compose database rejected the documented default password, so it was neither reset nor inspected and was returned to its prior stopped state. No external resolver, PDS, OAuth provider, credential, grant, DNS, proxy or deployed service was touched. The flow remains disabled by default pending UI and bounded live interoperability.

## 2026-09-20 — Production origin and OAuth client identity

The product owner selected `subcults.subcult.tv` as the replacement origin and authorized eventual controlled replacement of the legacy Subcults application. A read-only public inventory confirmed that the hostname currently serves the legacy app and exposes client metadata, JWKS and callback routes under `/api/v1/auth/atproto/`. The public metadata describes a confidential ES256 client with DPoP and broader repository scopes; no private secrets were read or recorded.

Subcult OS now models the replacement as a confidential web client and can serve the exact existing metadata and JWKS URLs when explicitly enabled. Configuration requires same-origin HTTPS client ID/callback/JWKS URLs, a P-256 private key and key ID, and rejects malformed enable flags. The generated public documents expose only the public JWK and request exactly `atproto`; they do not inherit the legacy repository scopes. Focused adapter, handler and configuration tests pass using generated ephemeral keys.

The callback and authorization-start flows are not implemented, so AT OAuth remains disabled by default. No live proxy, DNS, service, database, credential, OAuth grant or user data was changed. The [cutover runbook](../runbooks/subcults-cutover.md) requires current live-host inventory, proven backups/restores, an isolated candidate database, full API/browser qualification and an atomic proxy rollback before the legacy app may be stopped or replaced.

## 2026-09-20 — AT-01 encrypted OAuth persistence

Migration 4 adds AT OAuth requests and sessions without exposing protocol secrets as searchable plaintext. The OS-owned Indigo `ClientAuthStore` adapter binds each start request to an authenticated local person, HMAC-indexes state, encrypts request/session payloads under a protocol-specific derived key, atomically claims callbacks once, applies a ten-minute expiry, and provides expired-request cleanup.

Session creation accepts only the identity-level `atproto` scope. It atomically links the verified DID, stores authenticated-encrypted session material, consumes the request and writes an auth audit event. Existing sessions can persist rotated tokens only while their DID link remains active. Database uniqueness and a conditional conflict path reject a DID already owned by another local account; no email, handle or DID match merges people.

Focused PostgreSQL 17 tests passed for migration 4, concurrent callback claim, expiry and cleanup, recognizable-plaintext rejection, encrypted round trips, token rotation, audit creation, revoked-link refusal, over-scoped response refusal and cross-account DID rejection; the package also passed the race detector. The pinned OAuth import expands the Go transitive graph to the SDK's JWT, identity, CID/multibase and metrics dependencies. No OAuth HTTP route, live resolver request, external credential, PDS operation or repository scope was enabled.

## 2026-09-20 — Pinned Indigo OAuth implementation review

The exact Indigo source commit behind the Go module pin was cloned into the ignored read-only dependency workspace and registered in `.blacktower/clonedeps.json`. Review was limited to its AT OAuth package; no dependency scripts were run and no upstream source was edited or copied into Subcult OS.

The implementation supplies PAR, PKCE, DPoP, nonce retry, callback issuer and token subject validation, public-only HTTP transports, refresh and revocation. OS remains responsible for encrypted durable storage, expiry and replay handling, local-person link intent, scope policy and stricter redirect/resolver policy. No OAuth route, external request, credential, repository scope or DID link was created.

## 2026-09-20 — AT-01 syntax foundation

The first AT-01 slice pins Indigo at `v0.0.0-20260903211445-41278964ec8e` and limits production imports to `atproto/syntax` behind the OS-owned `backend/internal/atproto` adapter. It parses normalized account identifiers and collection NSIDs, exact-record AT URIs and DID-authority strong references. Application packages receive plain strings rather than unstable Indigo types.

The web dev toolchain independently pins `@atproto/syntax` `0.7.6`. Go and TypeScript consume the same provenance-tagged JSON corpus reduced from current public protocol specifications. Focused Go and TypeScript conformance tests pass. No Subcults source, fixture or Lexicon was copied.

This is syntax qualification only. AT OAuth still requires a reviewed implementation of PKCE, PAR, DPoP/nonces, client metadata, issuer/resource discovery and `sub`/scope validation. No handle/DID resolution, network call, OAuth link, PDS write, Lexicon publication or repository permission exists. The missing Subcults license continues to block copying the legacy schemas or OAuth implementation; `T-LEX` remains open until a minimal schema is independently authored and approved.

## 2026-09-20 — IDENT-01

Subcult OS now owns the canonical account/session foundation. Signup creates an unverified email identity and one-time verification challenge; verification is required before login. Authentication lookup uses an HMAC of normalized email, while the address is encrypted with a deployment key. The existing `people.email` value remains a plaintext operational projection for current workspace workflows and is not used as the authentication lookup authority.

Sessions use 15-minute access credentials and rotating 30-day refresh families. Browser transport uses scoped HttpOnly cookies. Native auth endpoints are isolated under `/api/mobile/auth/*` and use separate access and refresh headers backed by secure storage; ordinary browser auth responses do not expose JavaScript-readable token headers. Refresh replay revokes the whole descendant family. Logout, logout-all, expiry, password recovery, email normalization conflicts, DID uniqueness, challenge expiry and account non-merging have database-backed coverage. Web and mobile signup now stop at a verification-required state; both clients have verification entry points, and the web client includes recovery request/completion surfaces. Neither client receives session credentials in a JSON DTO.

Migration 2 refuses populated prototype accounts before creating the canonical identity tables. Migration 3 refuses populated prototype sessions before removing that table. This repository has no authorized retained-account migration, legacy password reader or dual-session compatibility path. Any discovered retained database must be inventoried and backed up before a separately reviewed migration is written.

Verification passed for the focused identity and migration suites, the repository's configured lifecycle database tests, Go non-database tests/vet/build, shared contracts, web TypeScript/ESLint/tests/build, mobile TypeScript/tests, Compose configuration and aggregate `make verify` under noninteractive CI mode. pnpm still reports that the `esbuild@0.27.7` install script is ignored; no build-script approval or supply-chain policy was changed. The broader historical database suite reaches several pre-existing lifecycle assertions outside IDENT-01 that conflict with current end-of-night/public behavior; those failures are recorded as non-identity follow-up rather than weakened. No deployment, external email, real-user migration or PDS operation was performed.

A built disposable stack also passed a headed Chromium signup and email-verification journey with synthetic data, landing in the authenticated operator home. Response-header inspection confirmed that normal browser login emits two HttpOnly cookies and no access/refresh token headers, while the native login route emits the two native token headers. The local outbox row was inspected only to bridge the deliberately unsent development verification email. The browser, containers, test databases and disposable volume were removed afterward. Native-device and full browser recovery qualification remain open.

The next eligible unit is AT-01. Its implementation remains an OS-native rewrite: no Subcults source or Lexicon may be copied until the recorded rights/license gate is resolved.

## 2026-09-20 — DB-01 and INV-01

### DB-01 result

Subcult OS now embeds a gap-free ordered migration set. `schema.sql` is immutable version 1; later files use `backend/internal/app/migrations/NNNNNN_name.sql`. The runner applies migrations in one PostgreSQL transaction under a transaction-scoped advisory lock and records version, name, SHA-256 checksum and time in `schema_migrations`. It rejects changed applied migrations and any database whose ledger is ahead of the binary. Startup and the explicit `cmd/migrate` path use the same runner; the Make target no longer pipes `schema.sql` directly into `psql`.

Focused tests on the named disposable `subcult-os-db01-postgres` PostgreSQL 17 container passed for fresh/replay, concurrent runners, checksum tamper, forced SQL failure rollback, database-ahead rejection and the current create/publish/free-door/end-of-night and capacity/door lifecycle journeys. The focused migration suite also passed under Go's race detector. A logical dump of the populated disposable database restored into `db01_restore`; both source and restore reported schema version 1, two events, four people and two workspaces. The backend Dockerfile built both binaries, and `/app/migrate` replayed the populated schema successfully from the built image.

No external database was inspected, reset or migrated. No compatibility path was added because no retained real database has been identified. The test container and its temporary databases were removed after verification.

### INV-01 result

The clean Subcults checkout remained at `3cf88ec66dffa52160331ecf2e24aee29d66e741` and was not modified. The [file-level extraction manifest](subcults-extraction-manifest.md) classifies all scoped AT, identity, indexer, audience, signal, touring, Lexicon and selected migration candidates.

No package qualified for wholesale import. The roughly 9,045-line indexer is particularly coupled to Jetstream, PostgreSQL, Prometheus, OpenTelemetry and legacy scene/post/alliance/geo domains. Useful reuse is primarily privacy, session-family, consent, stream-recovery and publication-failure contracts and fixtures. The audited checkout had no root `LICENSE`, `COPYING` or `NOTICE` file, and scoped history includes owner aliases and Copilot bot identities; distributable copying therefore remains blocked on explicit rights/license review.

### Next eligible work

IDENT-01 and AT-01 are now eligible as separate units. Both must be OS-native rewrites with provenance-tagged fixtures; AT-01 does not include publication or PDS provisioning, and IDENT-01 does not infer account equality from email or DID.

## 2026-09-20 — BASE-01 and API-01

### BASE-01 result
Subcult OS baseline: branch `t3code/research-event-app-competitors`, HEAD `abf3f500364d2cce60879c449cb6711346cd675f`. Existing research and proposal files were preserved.

Subcults baseline: clean `fix/main-regression-recovery` at `3cf88ec66dffa52160331ecf2e24aee29d66e741`. Fresh focused checks passed for Go module integrity; `internal/atprotocol`, `internal/touring`, `internal/audience`, and `internal/signal`; nine canonical Lexicons; frontend i18n/lint/build; and mocked deploy recovery. Full Vitest, whole Go/race, PostGIS, browser, PDS/provider, restore and parity qualification were not run. The dated release status is stale for schema evidence: the checkout requires migration 47.

Local tool detail: the default Go shim was unconfigured; Go 1.26.6 was invoked from the installed pinned toolchain. The first `pnpm` on PATH was a hanging wrapper; `/usr/bin/pnpm` selected the package-manager version pinned by each project.

### API-01 defect and change
Before API-01, `GET /api/public/events` used an explicit allowlist, but `GET /api/public/events/{slug}` embedded internal `eventDTO`. Anonymous detail responses therefore included the private workspace identifier, ticket allocation, raw reservation/check-in aggregates and staffing-count keys. The route did not expose ticket-holder identity, staffing items, notes, settlements or archives.

API-01 now:
- serializes detail through a standalone allowlisted Go DTO;
- omits workspace, raw attendance/capacity and staffing aggregates;
- restores detail/list image parity by loading `image_url`;
- defines a narrow web and mobile `PublicEventDTO` rather than inheriting/copying `EventDTO`;
- makes the contract checker reject forbidden public client properties;
- adds Go serialization sentinel/absence coverage and anonymous endpoint regression coverage;
- updates stale discovery test fixtures to use the free-reservation contract and the fixture's actual public base URL.

No database schema, endpoint URL, authentication behavior, reservation workflow, AT record or deployed service changed.

### Verification passed
- `/usr/bin/node scripts/check-contracts.mjs`
- Go 1.26.6 non-DB `go test ./... -count=1`
- Go 1.26.6 `go vet ./...`
- Go 1.26.6 backend build
- disposable PostgreSQL selected lifecycle, discovery, public serializer and migration tests
- web TypeScript check, ESLint, 76 tests and production build
- mobile TypeScript check and 27 tests, invoked directly from installed binaries because the dependency-policy check blocks the package script
- `docker compose -p subcult-os config --quiet`

The disposable `subcult-os-api01-postgres` container used only test credentials and an isolated database. It was removed after verification.

### Remaining gate
The aggregate `make verify` is not green: mobile dependency installation rejects an unapproved `esbuild` build script. No build approval or supply-chain policy was changed. Full browser/device behavior and a running application stack were outside API-01. API-01 is implemented and focused-test-verified, not runtime- or production-verified.

### Next eligible work
DB-01 and INV-01 can proceed independently. IDENT-01 and AT-01 remain blocked until ordered OS migrations and the selective extraction audit establish their boundaries.

## 2026-09-20 — Architecture direction changed

The product owner selected Subcult OS as the receiving Subcult.tv repository. The older Subcults application will remain read-only source material while useful capabilities are evaluated and extracted selectively. The prior permanent cross-repository bridge, separate-account and separate-database proposal is superseded by ADR 0005.

Planning artifacts now require one Go API, one identity authority and one PostgreSQL database in OS, with modular ownership and a Go/TypeScript AT conformance boundary. No source, migration, test-suite or Git-history merge was performed. Subcults was not modified.

### Discovery modal keyboard containment — 2026-09-30

From qualified PR #175 (`9af914d`), replaced the custom occurrence overlay with a
native modal dialog and added a scoped Tab boundary handler for Close/Reserve.
The first full local verification passed before the final Tab handler. The final
source verification is recorded in `.cache/dev-env/discovery-native-dialog-verify.log`.
Browser component fixtures verified native background inertness, forward/reverse
Tab for one/two controls, Escape/Close focus return, outer-dialog click handling,
themed surfaces, and unmount cleanup. Browser screenshots:
`browser-screenshot-127-0-0-1-muntx6dy-eb493ed7.png` (light),
`browser-screenshot-127-0-0-1-muntxsjf-18a16d64.png` (dark).
No live provider, publication, projection, or native-device claims were added.

### Qualified native dialog and named discovery controls — 2026-09-30

PR #176, `2f3c0e6adae2bcb062014b6c96dc22e52a7d663a`, passed hosted
push 11053/job 20001 and PR 11054/job 20002. Each ran 282 web/34 mobile tests
and the full DB gate (421 top-level, 599 including nested), no failures/skips.
Its final local gate also passed. Owned runners exited 0, were removed, and
registration credentials were deleted; zero repository registrations verified.
The issue #26 receipt was updated without closing acceptance or dependencies.

The next discovery controls slice captures SVG openers, names each plotted
occurrence, uses group semantics, preserves atomic result-status regions and
uses theme-aware search focus. Public browse copy no longer exposes projection
mechanics. Final local gate: 284 web/34 mobile tests, 421 top-level DB tests
(599 including nested), no failures/skips. Browser evidence is in
`discovery-ux.md`; receipts remain in `.cache/dev-env/discovery-controls-*`.
Screenshots `browser-screenshot-127-0-0-1-munupt2m-b7ca4b53.png` (light) and
`browser-screenshot-127-0-0-1-munupt7v-4e2bc361.png` (dark) were inspected.
Screen-reader speech, native/device, live feed and full journey remain separate.

### Qualified discovery controls and narrow long-content repair — 2026-09-30

PR #177 (`ade02d9a1d5a84454e4dd63ca0ace1eb7bf1f143`) passed push
11057/job 20006 and PR 11058/job 20007, each with 284 web/34 mobile tests
and 421 top-level DB tests (599 including nested), no failures/skips. Owned
runners exited 0, containers/registration credentials were removed, and zero
repository registrations were verified. The issue #26 receipt was reconciled.

The next slice fixes reproduced discovery text overflow and displaced Close at
360px. Same-origin iframe geometry, actual native keyboard, long published
fields and synthetic error proof are recorded in `discovery-ux.md` and ignored
`.cache/dev-env/discovery-long-content-*` receipts. Final local full gate passed
284 web/34 mobile tests and 421/599 DB tests, no failures/skips; disposable DB
removed. Browser fixtures were removed and original page/appearance preserved.
Physical/native devices, screen-reader speech and complete journeys remain open.

### Qualified discovery reflow and public booking context — 2026-09-30

PR #178 (`702d2f8b6793632d1240866f2ac9d3d6edba9005`) passed push
11061/job 20010 and PR 11062/job 20011: 284 web/34 mobile tests and
421 top-level DB tests (599 including nested), no failures/skips. Owned runners
exited 0, containers and registration credentials were removed, and zero
repository registrations were verified. The issue #26 receipt was updated.

The next slice repairs a reproduced public event component state leak: a failed
new-slug read left the previous event and its enabled booking form visible.
Keyed guest state, current-read gating, unmount callback invalidation and
synchronous submission guards now pass the full local gate: 286 web/34 mobile
tests and 421/599 DB tests, no failures/skips. Strict Mode browser fixtures cover
old free/role successes, paid redirects/errors, A → B → A, repeated submits and
current confirmations. Seven writes were simulated; actual development inventory
remained nine before/after. See [public booking context](public-booking-context.md)
for source decisions, inspected light/dark screenshots and evidence limits.

### Public booking context qualified and reflow repaired — 2026-09-30

PR #179 (`2d0dca74cc8f204f0394b451b5e244c09176715d`) passed push
11065/job 20015 and PR 11066/job 20016. Each passed 286 web/34 mobile tests
and 421/599 DB tests, no failures/skips. Owned runners exited 0 and were removed;
registration credentials deleted and zero repository registrations verified.
The PR body and issue #26 were reconciled and read back.

The next three-class public booking repair fixes page overflow reproduced at
360px: scroll/client width6682/345 now345/345. Long event, role, guest, email
and error text fit; the capacity badge and native keyboard booking/application
controls remain reachable. Light/dark focus screenshots inspected. Three writes
were intercepted synthetic responses, and all helpers/interception/iframe were
removed. Full local gate passed 286 web/34 mobile tests and 421/599 DB tests,
no failures/skips. See [booking context and reflow](public-booking-context.md)
for exact geometry and remaining journey/device/provider limits.

### Booking reflow qualified; owner role setup added — 2026-09-30

PR #180 (`e25312ef23807aef861579ec916bb07965289ad6`) passed PR11069/job20019
and push11070/job20020: 286 web/34 mobile tests and 421/599 DB tests, no
failures/skips. The existing kvant instance runner handled both jobs; no owned
runner or credentials were created. Shared runner preserved; zero repository
registrations verified. PR/issue #26 receipts were reconciled and read back.

A new disposable operator rehearsal completed browser signup/verification,
workspace and event creation, then found no owner role-creation control. The new
panel uses the existing endpoint, defaults explicitly to private, preserves
application/assignment separation and fences pending/uncertain writes. Real
private/public creation, committed-response loss and reload proof passed;
synthetic validation/denial and stale-context fixtures passed. Final full local
gate: 296 web/34 mobile, DB421/599, no failures/skips. Narrow light/dark layout
screenshots inspected; inactive-preview keyboard-focus proof remains open.
See [role setup](participation-role-setup.md) and the
[ongoing operator rehearsal](../qa/operator-journey-2026-09-30.md).


### Role setup qualified; invitation receipt corrected — 2026-09-30

PR #181 (`6ed01556ff91b51b7997f74543b77e6802b26731`) passed push11075/job20026
and PR11076/job20027: 296 web/34 mobile and DB421/599, no failures/skips.
Existing kvant runner preserved; no owned runner or credentials created. PR and
issue #25 receipts were read back. Normal invitation acceptance and member
read-only role UI passed; an unauthorized creation returned403 with roles4→4.
An anonymous public application persisted as submitted, with staffing0.

The next small correction replaces “Invite sent” with a created/queued receipt
and unconfirmed delivery. The existing transactional API supplies creation and
queue evidence only. The joined rehearsal and browser connection limit are
recorded in [the operator journey](../qa/operator-journey-2026-09-30.md).
Continue owner application review, staffing/commitments, participant views, free
ticket/door, finance, closeout/template reuse and paired timing qualification.

The invitation-copy candidate passed full pinned local verification: 296 web/34
mobile tests, backend checks/builds and the complete disposable DB gate (exit0).
The test database was removed. Its DB output capture was truncated by the tool
output budget, so this receipt does not derive complete DB test counts from it.
Changed-copy browser and screen-reader proof is pending the preview connection
recovery; no live email was sent. Hosted qualification follows publication.


### Invitation receipt qualified; owner controls corrected — 2026-09-30

PR #182 (`7d9563c3368e04282070df953d02375bae66c02d`) passed push11077/job20028
and PR11078/job20029: 296 web/34 mobile tests, DB421/599, no failures/skips.
Existing kvant runner preserved; no owned credentials or runner created; qualified
PR body and head were read back. The disposable API operations rehearsal passed
36/36 using separate synthetic accounts/events and held mail.

The next UI correction removes the owner-only invitation form and its guidance
entry point from non-owner roles, with a matching submit guard. Focused API member
creation was rejected403 with invitations2→2 and outbox9→9. Scope and evidence
limits are recorded in [the ongoing operator journey](../qa/operator-journey-2026-09-30.md).
Full local verification passed: 304 web/34 mobile, DB421/599, no failures/skips;
test DB removed. Changed-form browser/SR proof still requires
preview connection recovery. Continue the original browser event through owner
application review, assignment/participant views, ticket/door, finance and reuse.


### Owner invitation controls qualified; door rehearsal repaired — 2026-09-30

PR #183 (`b4369fa5a806133961a4c3dc061b7d011928c888`) passed push11081/job20033
and PR11082/job20034: 304 web/34 mobile, DB421/599, no failures/skips. Existing
kvant runner preserved; qualified PR body/head read back, no owned runner needed.

The free-ticket API rehearsal found a stale baseline-member door assumption.
The current server correctly rejected it403. The repaired script proves that
denial, grants the synthetic membership role `door` through the owner API,
checks search/idempotent check-in, then changes the role to `crew` and proves
write denial403. Final report is reserved1/checked-in1/no-shows0; script exit0.
No provider or browser/device claim. See [the operator report](../qa/operator-journey-2026-09-30.md).
The suspected invitation context leak is unconfirmed through normal navigation,
which uses full-page workspace links. Continue joined browser qualification
when the preview connection recovers; owner role-management UI is also not yet
qualified by this API-only role grant. Full local verification passed304 web/34
mobile, DB421/599, no failures/skips; disposable test DB removed. Final script
rehearsal, Bash syntax and ShellCheck with sourced files also passed.


### Door rehearsal qualified; membership access roster added — 2026-09-30

PR #184 (`40a55c4bac0e53ce9efd06e48998342c7ef887df`) passed push11083/job20035
and PR11084/job20036: 304 web/34 mobile, DB421/599, no failures/skips. Existing
kvant runner preserved; qualified PR body/head read back, no owned runner needed.

The next slice makes roster roles and server-derived access status truthful
before owner role-management UI. Private current/scoped workspace reads expose
active/expired/revoked state and timestamps, use no-store and omit removed rows.
No migration or authority mutation change. Web labels all six roles accurately;
inactive/unknown membership does not imply usable permissions. Shared mobile
role types match the server. Full local gate315 web/34 mobile, DB422/606, no
failures/skips. Normal owner expiry/revoke API proof passed in the separate
alpha workspace, with original actor/event state preserved and all21 mail held.
See [the operator report](../qa/operator-journey-2026-09-30.md) for scope and limits.
Continue owner role-management controls and the joined browser work when preview
connection recovers; finance, private closeout/reuse, paired timings, native
devices, screen readers and provider/deployment gates remain separate.


## Owner member-role assignment

PR #185 (`32a474ec77ca2e188332f9e5f40930921719c7b9`) passed both hosted
checks: push11089/job20041 and PR11090/job20042. Each passed315 web/34 mobile
and DB422 top-level/606 including nested, with no failures/skips. The existing
kvant runner was preserved; no owned runner or credentials were created and
repository runner registrations remain0. Qualified PR body/head read back.

Owners now have a dedicated Member roles page linked from their workspace.
It offers Owner, Organizer, Finance, Door and Crew with capability descriptions
before assignment. The legacy member role defaults to its effective Crew bundle;
unchanged selections do not write. Nonowners and inactive, unknown-role or
unresolved memberships remain read-only. The request contains only the selected
role and targets the membership row ID, preserving expiry/revocation metadata.
The server still decides permissions and rejects last-active-owner demotion.
No authority policy, schema or access-restoration control changed.

A synchronous pending guard fences duplicate submissions across the page.
Matched receipts trigger a fresh canonical roster read; permissions are never
applied optimistically. Denied writes clear the private roster. Unknown outcomes
fence further writes until a fresh read, without replaying the write. Known
validation/last-owner rejections remain editable. Unmounted callbacks cannot
update the departed page. Separate route instances are keyed by workspace ID.

Eleven transport cases cover encoded membership paths, role-only payloads,
metadata preservation, invalid roles, mismatched receipts, server rejections and
workspace identity. Eleven static-render cases cover all five nonowner roles,
owner choices/review, inactive/unresolved rows, unknown roles and missing private
workspace data. These do not prove browser callback timing or native interaction.
Changed-page visual, keyboard and screen-reader proof remains pending T3 preview
connection recovery. No provider, native-device or deployment claim.


A normal API rehearsal on separate synthetic alpha workspace
`3449e469-cbde-44bf-9a32-3a3e1996bb1e`, membership
`eec8aa9c-8fc2-4dbb-9c2d-3100efcd60d4`, proved role-only Door/Crew changes
preserve a future expiry, canonical refreshed roster state, nonowner change403
and last-owner demotion409. The original Crew/no-expiry state was restored through
normal owner API calls. Original browser actors/event were untouched. This proves
the existing API boundary, not the page's actual browser interaction.


Full pinned local verification passed337 web/34 mobile tests, backend checks and
builds, DB422 top-level/606 including nested, no failures/skips. Disposable test
DB removed. The initial TypeScript fixture-cast failure was corrected before the
complete rerun. Scoped review, edited Markdown links and diffcheck passed.


## Owner role controls qualified; finance/closeout API rehearsal

PR #186 (`dc095ffad2d7b639ed2cae5a91a5bc068bd8ec32`) passed push11093/job20046
and PR11094/job20047:337 web/34 mobile, DB422 top-level/606 including nested,
no failures/skips. Existing kvant runner preserved; no owned runner/credentials,
repository registrations0. Qualified PR body/head and issue #25 read back.

On separate disposable alpha workspace3449e469-cbde-44bf-9a32-3a3e1996bb1e,
closed free event46d57735-6a5a-403a-9689-66056e5ab73a, normal API calls retained
four finance rows: budget10000 cents, payable8000, actual3000 corrected to2500.
Current categories remain separate; ticket gross stays0. Same request-key replay
returned the same correction ID; changed payload with that key returned409.
Crew reads403, assigned Finance reads200, restored Crew reads403. No provider.

Settlement finalized with gross0; late adjustment rejected409. Private archive
was available to owner and denied anonymous401. Private note remains in archive.
Reuse seeded draft36a59dda-e344-4703-8624-af9e5dc91aad once; retry returned the
same ID. Tickets/check-ins, roles, staffing, finance lines and archive were not
copied. Private templatefc7e7cd7-16b8-4a23-b5d3-7a20843a71ae applied while
draft; after synthetic local publication, applying again returned409. Public
response omits private finance/archive/template sentinel text and fields.
The first ad-hoc lookup used ended instead of source-owned end_of_night and
stopped before mutations; corrected before proof. Do not rerun this ad-hoc script
against the finalized event; its initial conditions are no longer present.

API source remained185/Go1.26.6; frontend186. Original browser actor/event state
stays roles4/applications1/staffing0/tickets0; aggregate10people/5workspaces/
7events/3tickets, all21 outbox held. Retained development stack/data untouched.
T3 status/open retry still reaches chrome-error with no application root, while
host web/health return200. This is API proof, not joined browser, intended-user,
paired-timing, keyboard/SR/device, provider or deployment acceptance.

## Private finance read and write session

The finance panel previously rendered its editor after a failed/denied ledger
read and guarded submissions using rendered busy state. A new event-owned session
requires a valid event-scoped private read before editing, admits one write
synchronously, and validates receipt event/type/direction/amount/currency and
correction/payable references before displaying it. Denial or uncertain write
outcomes clear private lines/draft and fence further writes until a fresh read;
refresh never replays the mutation. Known400/409 rejections remain editable and
retain an unchanged request key. Successful writes retain existing manual-record
and category-separation behavior. No payment execution or authority policy change.

Keyed inner panels isolate event lifetimes; permission loss unmounts the private
panel. Layout cleanup invalidates session and view lifetimes, including reactivation
before an old response arrives. Loading does not claim the ledger is empty.
Shared Button and a persistent atomic status region cover save/recovery controls.
Nineteen transport/session cases cover pending read/write fences, identity,
400/409 retry,401/403/500 denial/uncertainty, disconnects, mismatched receipts and
departed/restarted lifetimes. Five static-render cases cover absent permission,
loading, unavailable recovery, confirmed empty data and retained payable history.
These fixtures do not prove actual React callback timing or browser/SR interaction.


Full pinned local gate passed361 web/34 mobile tests, backend checks/builds,
DB422 top-level/606 including nested, no failures/skips; test DB removed.
Initial key-generator typing and fixture React-import/401-refresh failures were
corrected before the final complete rerun. All attempts retained as local logs.
Scoped review, edited Markdown links and diffcheck passed. Browser proof remains
pending; no live provider/deployment/retained-state action.


## Finance panel qualified; repeatable free API rehearsal

PR #187 (`1cdd075f4c7eed9344a92e55ab89317cdb07538e`) passed push11097/job20050
and PR11098/job20051:361 web/34 mobile, DB422 top-level/606 including nested,
no failures/skips. Existing kvant runner preserved; no owned runner/credentials,
repository registrations0. Qualified PR body/head/base and issue #25 read back.

Read-only exports on the earlier synthetic finalized event passed owner200,
Crew403 and anonymous401 for CSV, Markdown and printable HTML, with no-store,
UTF-8, retained4 history rows, current budget10000/payable8000/actual2500 cents,
superseded actual exclusion and UTC timestamp. Unrelated archive/template note
sentinels are absent. Initial ad-hoc assertions used an incorrect CSV row name
and false rather than its blank noncurrent flag; corrected against source before
proof. No data mutation or browser Print/PDF qualification.

The new finance-closeout-qa harness turns the precondition-dependent ad-hoc run
into fresh-record normal API proof. Shell and Python entrypoints both enforce
existing disposable-target guards. Private temp cookies are removed on exit;
output contains named checks and bounded errors, not tokens/capabilities/DTOs.
Fresh verified owner/crew actors cover ledger corrections/key replay, Finance
permission removal, free door/closeout, private notes, seed retry/no copied data,
private template reuse and scoped CSV/Markdown/HTML exports with exact cents/UTC.
See [the harness guide](../qa/finance-closeout-rehearsal.md).

The first run stopped401 before workspace creation: urllib uses localhost.local
for a single-label host, while curl stored host-only cookies under localhost.
The helper now normalizes only those host-only cookies for a localhost API.
Both subsequent localhost runs passed all27 checks. Six unsafe wrapper/helper
invocations (remote API, missing opt-in, retained DB name) rejected before cookie
or API use. Bash syntax, ShellCheck with sourced files and Python syntax passed.
All attempts retained. Original browser event remains roles4/applications1/
staffing0/tickets0. After these runs:16people/7workspaces/11events/5tickets and
all31 outbox rows held. Owned runtime has only API/PG/web, mail/OAuth/projection
flags false, backend source185/Go1.26.6. Retained development data/stack untouched.
No live provider/device/deployment or broader acceptance claim.


A third27-check run through127.0.0.1 also passed. Read-only persisted checks show
three distinct fresh rehearsal workspaces, each with2 members/2 events/4 finance
history rows/1 ticket/1 private archive. Post-run aggregate18people/8workspaces/
13events/6tickets, all35 outbox held. The original browser vector is unchanged.
Full pinned local verification passed361 web/34 mobile, DB422 top-level/606
including nested, no failures/skips; disposable gate DB removed. This gate is
separate from the retained owned rehearsal DB. Edited Markdown links, syntax,
ShellCheck and scoped review passed; no broader acceptance closure.


## Exact event start preservation during unrelated edits

PR #188 (`fee51e77b254dcc43cbbe284f0b3dc294426aaf6`) passed both hosted
checks: push11101/job20055 and PR11102/job20056, with361 web/34 mobile tests
and DB422 top-level/606 including nested, no failures/skips. The existing kvant
runner was preserved; no owned runner or credentials, repository registrations0.
Qualified PR body/head/base and issue #25 receipts were read back.

Both event editors reconstructed every saved start from minute-level local text.
That truncated server seconds/fractions and could select the other instant during
a repeated local hour. On a published synthetic event with one reserved ticket,
the actual web and mobile builders converted2026-11-01T07:30:45.123456Z into
2026-11-01T06:30:00.000Z under America/Chicago. Both unrelated description
PATCH requests returned409. An exact-start control succeeded200.

Hydrated forms now retain the original timestamp privately. An unchanged displayed
start uses that original value; a changed input uses the existing parser. The
metadata never enters the write payload. Both actual fixed builders passed normal
API description edits200 and retained the exact start and one reservation.
The first post-fix rehearsal reused an identical description and correctly hit
the server's no-op400; distinct per-client descriptions resolved the harness
assertion without changing server validation. Supplemental actual-model runs
passed under Chicago, New York, Honolulu and Tokyo. Five regression cases per
client cover fractional precision, repeated-hour instants, offset input, deliberate
changes and invalid source fallback. The red gate failed the three preservation
cases before the implementation; the complete green pinned gate passed366 web/
39 mobile and DB422/606, with no failures/skips and disposable gate DB removed.

This is model and owned synthetic API evidence. Actual browser/native editing,
legacy event-zone persistence and deliberate ambiguous/nonexistent wall-time
selection remain open. No live provider, deployment or retained-data action.


## Browser recovery and staffing assignment identity

The earlier stack through #188 is now merged into main (observed main52c5ad7).
PR #189 ataa13a855dadaf271e155c656b0bb46da74893d62 targets main and is linked
to the thread. Its push11160/job20126 and PR11161/job20127 are queued behind
merge runs; the shared kvant runner is online/busy and was left intact. Normal
web login and description-only Save event on the precision fixture preserved
2026-11-01T07:30:45.123456Z and one reservation, returning No changes. This
qualifies the web edit path for that fixture, not native or deliberate DST editing.

The collaborative preview resumed with an actual application root. The original
owner/browser event progressed: its public application was accepted and a private
setup task created. Acceptance did not create an assignment. Two defects appeared:
the accepted participant roster/options stayed empty until full navigation, and
assigning the Crew workspace member returned400 because the selector submitted a
membership row ID as assignedPersonId. Both identities existed before the roster
metadata work; the staffing contract expects the person's identity.

Private MemberDTO now includes personId while retaining id for membership role
management. Web staffing options use personId only for explicitly active members;
missing identity or expired/revoked/unresolved access cannot substitute a membership
ID. Mobile's private DTO gains the same optional field for older API compatibility.
Application-list updates refresh the canonical participant roster, clearing stale
options while loading. Seven model cases cover identity and inactive/missing-field
boundaries; existing DB roster cases assert distinct person/membership identities.

After restarting only the owned rehearsal API (Postgres/data/retained stack left
intact), the browser assigned the existing task to Crew successfully. Under review
removed the participant/options; Accepted restored both without navigation. One
assigned task remains; participant requirements were saved separately from the
private operator note. Crew portal proof remains pending: the preserved synthetic
crew login rejected the credential recorded in the continuation context. No reset
or credential change was performed; the earlier owner session was signed out.

The first gate failed six static-route fixtures because a new positional state hook
shifted their overrides. The final implementation refreshes from application state
without adding that hook. Full pinned verification then passed373 web/39 mobile,
DB422 top-level/606 including nested, no failures/skips; gate database removed.
Backend source and mounted frontend are this staffing candidate; no migrations,
live provider, deployment, intended-user or broad acceptance closure.


## Merged source; crew portal, commitment and free admission browser proof

PRs #189 and #190 are merged. Observed main717b79a3e2f055311e4bd4b3f7500bfac565652d
has the same files as the locally qualified staffing candidate. Gitea's updated
PR heads6d78fb6 (#189) and20e0cee (#190), their original qualified-local heads,
and main still have queued hosted checks. Merge status does not establish hosted
qualification. Existing shared runner/queued work was preserved.

The owned runtime identity was rechecked: same API/PG containers and tmpfs QA DB;
mail delivery, AT OAuth and projection flags false. The synthetic Crew account
was recovered through the normal browser request/held-email link/password form,
not a direct database update. Its one-use challenge was consumed and removed from
the URL. Only this example.test fixture credential changed; no provider contacted.
Normal login then showed its assigned task and participant requirements in the
participant portal, with the private operator note absent.

The original owner/browser event now has one completed private commitment, created
and marked done through normal controls. An anonymous public page omitted both
that private text and the staffing note. A new synthetic guest reserved one free
ticket; remaining10 became9. Its ticket page displayed the guest and QR pass.
Owner Door search found the guest, Check in succeeded, and the disabled Checked in
control appeared. Read-only DB proof confirms tickets1/checked-in1, commitments1/
done1, staffing1/assigned-to-Crew1. These are real browser stages of the same
original event, separate from earlier fresh API-only harness runs. They do not
qualify physical scanning, offline/reconnect, paired timings or intended-user use.

Recovery exposed another delivery-copy defect: web claimed a link had been sent
and mobile claimed it was on its way, although the API receipt does not confirm
provider delivery and the rehearsal message was held. Both notices now keep
account existence conditional and state delivery is unconfirmed. Mobile action
and guidance describe requesting the link. API, sending policy and account
privacy behavior are unchanged. Web handler fixtures cover accepted and rejected
requests; mobile notice regression retains the conditional wording. A real web
request for a missing synthetic account showed the corrected generic notice.
Full pinned local gate passed375 web/39 mobile, DB422 top-level/606 including
nested, no failures/skips; separate disposable gate DB removed. All40 rehearsal
outbox rows are held. Private receipts contain no ticket/recovery capability.

Remaining original-event browser stages include timed shifts, finance/closeout,
private archive/template reuse and paired workflow measurements. Physical device,
provider, deployment and issue #25 acceptance gates remain open.


## Subcults terminal design foundation — 2026-09-30

The user selected the old Subcults CSS design system. The reference is the separate
`subcults` repository at `93a13af`, `web/src/index.css`: Space Mono, black and
charcoal surfaces, purple actions, neon status accents and square controls. This
supersedes the mobile-derived monochrome foundation. Existing Light/Dark/System
preferences remain in place; light mode uses the same structure with darker
status colors. The shared JSON tokens remain the source for both clients.

Web now self-hosts licensed Space Mono 400/700, uses square shared controls and
visible cyan focus in dark mode, and updates the design gallery. Native themed
StyleSheet factories use platform monospace and square corners, preserving
avatars. Exact native Space Mono loading and isolated inline styles still need
device review. Font source version and hashes are recorded beside the assets.

The token generator passes its admitted foreground/surface contrast checks. The
final pinned full gate passed375 web/39 mobile tests and the complete disposable
database gate (426 top-level / 610 including nested), with no failures or skips. The T3
preview repeatedly loads `chrome-error://chromewebdata/` although the host serves
`/design-system` with HTTP200. This slice has no rendered browser or native device
qualification. Retained application and synthetic journey data remain preserved.

Resume rendered light/dark and narrow/wide review when the T3 preview connects,
then exact native font loading and device review. The original event's timed
shifts, finance/closeout, archive/template reuse and paired measurements remain
open; the user-directed design change is the current source priority.

## Subcults terminal adoption pass — 2026-09-30

Applied the terminal foundation to screens that still used the old styling. On
web, removed pill and arbitrary rounding, soft shadows and weights above bold
from the views. Mapped the amber, emerald, rose and fuchsia tone helpers to
semantic status tokens. Destructive actions now use the danger outline instead
of purple. Links styled as actions share the uppercase `btn-*` labels. Field
text stays regular weight. On native, `terminalStyles()` now also squares
per-corner radii and applies monospace to color-only text styles. Placeholder
and icon colors now come from tokens. Source weights above bold were reduced to
bold.

Headless Chromium screenshots were taken of the gallery, login, signed-in
workspace and event editor at 390 and 1280 px in light and dark. Native device
review was not done. `bash scripts/dev-env.sh verify` exited 0 with 375 web
tests, 42 mobile tests and 426 top-level disposable database tests, and no
failures or skips.

Follow-up, 2026-10-01: web buttons, links and fields now use the shared
`btn-primary`/`btn-secondary`/`btn-ghost`/`btn-danger` and `field` utilities.
The exceptions are selected-state tabs and card links. `Button` gained a danger
variant. Staffing and commitment counters size by container so monospace labels
fit side columns. On native, `PrimaryButton` (primary/secondary/danger, icon),
`Pill` (all tones) and a new `Field` are adopted by 14 screens. Links,
selection toggles, icon-only controls, the run-of-show row actions and
immersive surfaces stay local. Expo web export succeeded for 20 routes. Expo web
screenshots at 390 px show the avatar circle kept and the navigation dot
square. Other round indicators need event data or a camera and were not
rendered. A statically exported page first loaded in dark mode renders
StyleSheet colors light, which is not yet fixed. The full pinned gate exited 0
with 375 web, 42 mobile and 426 top-level disposable database tests.

Second follow-up, 2026-10-01:
- Native `app.json` set `userInterfaceStyle` to `light`, which locked iOS and
  Android to light so System appearance never followed a dark device. It is now
  `automatic`.
- `useThemeTokens()` returns light tokens during web hydration through
  `useSyncExternalStore`'s server snapshot, then switches. A statically exported
  page first loaded in dark now renders fully dark. Login and settings were
  checked from prerendered markup in a fresh Expo web export.
- The public event page no longer shows an empty grey block when an event has
  no artwork. "Discover more events" is a link rather than a status chip.
- Long ticket codes wrap on the ticket page at 390 px.
- Selected pricing cards and the active workspace use a purple frame instead of
  the disabled background. Cards with a hidden radio show a focus outline.
  Pricing cards size by container.
- Unchanged hover states now strengthen the border.
- The full pinned gate exited 0 with 375 web, 42 mobile and 426 top-level
  disposable database tests.

## Subcults terminal polish pass — 2026-10-01

Full-screen review of every web route at 390 and 1280 px in light and dark.
- Fixed a horizontal overflow on phones that PR 195 introduced. The workspace
  scrolled sideways by 277 px and the editor by 40 px, because unclamped
  `auto-fit` grids inflated their sidebar's minimum width. Grid minimums now use
  `min(…, 100%)` and sidebars have `min-w-0`. This regression is live in
  production until the next web release.
- The event editor is widened to `max-w-6xl`. Its sidebar keeps only the
  contextual panels: create help, draft checklist, templates, live event, actions
  and workspace link. Finance, settlement, roles, applications, participants,
  staffing, notifications, reminders, archive and commitments move to a
  two-column area below. This removes the empty form column.
- Uppercase labels use a single letter-spacing. Panels no longer use status
  borders or label colors as decoration. List item titles are bold. Text links
  are underlined with a shared offset.
- Venue access, cultural imports, lifecycle intents and access information share
  the framed header used by member roles. Their bare labels, buttons and forms
  now use shared field, button and panel styles.
- Recover, verify-email and email-preference pages show the brand bar.
- Workspace stats sit three across on phones. Archive search wraps instead of
  squeezing its input.

No overflow on any captured route at 390 px.

## Production origin moved to os.subcult.tv — 2026-10-02

The owner moved the canonical origin from `subcults.subcult.tv` to `os.subcult.tv`. DNS and the Cloudflare tunnel already covered `*.subcult.tv`, and no PDS handle used `os`. Almaz Caddy serves the new host. The old host keeps `/api`, health and the legacy service-worker retirement, and 301-redirects every other path with its query, so already-sent email links keep working. The analytics proxy maps both hosts to the same site. Production `PUBLIC_WEB_URL` and the disabled AT OAuth URLs point to the new host. The API and email worker were recreated with unchanged images. Sessions are host-only, so operators sign in again on the new host. Verified: new-host pages return 200 and `/api/ready` reports ready. The API allows the new origin. Old-host page links redirect with path and query preserved. Backups and rollback are on Almaz under `/opt/server/management/backups/os-subcult-tv-*`.

## Issue closure documentation — 2026-10-02

Documentation only; no code, schema or contract changed.
- [ADR 0008](../adr/0008-excluded-social-ranking-reputation.md) records the
  excluded alliance, social-post, feed, ranking, streaming and reputation
  concepts for SOCIAL-01 (#72). The ADR index now also lists ADR 0007.
- The [backlog](backlog.md#parked-options) gains a "Parked options" section
  for #48, #59, #60, #62–#66 and #69–#71. Each entry names its reopening
  evidence and source material. A "Portfolio items outside Subcult OS"
  section points #73–#82 to their Funding Kit proposals.
- [Project maintenance](../project-maintenance.md) documents the current
  pnpm build-script state, the owner-only trusted-package rule and Indigo and
  TypeScript bump procedures for MAINT-01 (#84).
- [Event exports](event-exports.md) records that no accounting format is
  currently required for EXPORT-01 (#54). It also records a local Print/Save
  as PDF check: a synthetic event's settlement print HTML, opened in headless
  Chromium, produced a 2-page A4 PDF with the expected headings.
