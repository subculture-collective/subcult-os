# Development backlog

Status: original engineering slices for the Subcult OS platform core. The [Gitea master roadmap](https://git.subcult.tv/subculture-collective/subcult-os/issues/1) now tracks 83 work items, including deferred and exploratory work, with native dependencies. No items were queued or assigned by roadmap publication. Sizes below are rough historical engineering ranges, not commitments.

## Dependency order

BASE-01 → API-01 + DB-01 + INV-01.
DB-01 + INV-01 → IDENT-01 + AT-01.
IDENT-01 + AT-01 → MODEL-01 → DISC-01 → PUB-01 → UX-01 → QUAL-01.
CONSENT-01 is a boundary task, not authorization to send. COMMONS-01 follows demonstrated conformance reuse. The owner promoted issues #50–71 for development on 2026-09-24; see the [expansion work order](../superpowers/plans/2026-09-24-expansion-50-71.md). Device, provider and production gates remain separate.

Review [decisions](decisions.md), [architecture](architecture.md), and the [extraction inventory](extraction-inventory.md) before implementation.

For the September 29 checkout and Gitea reconciliation, use the
[current delivery work order](current-delivery-work-order.md). Historical
implementation statuses below do not establish device, provider or deployment
qualification.

## BASE-01 — Capture current behavior and source evidence

- Repository: OS target; Subcults read-only
- Priority: P0
- Depends on: none
- Status: complete — baseline captured 2026-09-20; see execution-log.md
- Size: complete baseline slice

**Acceptance:** Record exact revisions, dirty state, tools and real check outcomes; distinguish current source evidence from historic release claims.

**Verification:** Run documented baseline commands; preserve failing output and prerequisite blockers.

**Out of scope:** No deployment, source import, dependency upgrade or test deletion.

**Rollback:** Documentation-only baseline; preserve prior files and source revisions.

## API-01 — Separate public event projection from operator DTO

- Repository: OS
- Priority: P0
- Depends on: BASE-01
- Status: implemented; focused privacy tests and subsequent aggregate `make verify` passed. Earlier dependency-policy failures are historical evidence, not the current gate.
- Size: implemented slice

**Acceptance:** Explicit public allowlist; private workspace/attendance/staffing properties rejected; necessary public UI retained.

**Verification:** Contract checker, serializer sentinel test, DB-backed anonymous route test, web/mobile type checks and focused suites.

**Out of scope:** No authentication replacement, AT dependency or public-data expansion.

**Rollback:** The prototype endpoint may change or disappear during consolidation, but no replacement may expose operational DTOs as a shortcut.

## DB-01 — Establish a clean ordered OS schema

- Repository: OS
- Priority: P0
- Depends on: BASE-01
- Status: implemented and focused-test-verified; no external database was reset or migrated
- Size: implemented foundation slice

**Acceptance:** Approve runner and clean platform schema; ledger each migration once; serialize concurrent runners; block incompatible startup; inventory every existing database before any reset and add legacy migration only for specifically retained real data.

**Verification:** New fresh, replay, concurrent-runner, failed-migration, backup and restore tests on disposable PostgreSQL. Add populated legacy-upgrade fixtures only if INV-01 finds data that must survive.

**Out of scope:** No Subcults migration import, live migration or unapproved destructive reset.

**Rollback:** Before replacing any non-empty database, classify and back up it read-only. For clean development databases, recreate from ordered migrations; do not build compatibility merely to restore prototype APIs.

## INV-01 — Complete selective Subcults extraction audit

- Repository: OS documentation; Subcults read-only
- Priority: P0
- Depends on: BASE-01
- Status: complete — file-level manifest recorded at the audited clean revision
- Size: complete audit slice

**Acceptance:** Classify candidate files as adapt, rewrite, fixture-only, reference-only or reject; record product need, revision, license/provenance, generated status, dependencies and coupled features.

**Verification:** Independent inventory review; every accepted candidate maps to a current journey and a focused test that does not require the old application.

**Out of scope:** No repository merge, source copy, migration import or cleanup of Subcults.

**Rollback:** Inventory changes are reversible documentation; rejected candidates remain untouched in Subcults.

## IDENT-01 — Build the canonical OS identity and session foundation

- Repository: OS
- Priority: P0
- Depends on: DB-01, INV-01
- Status: implemented and focused-test-verified; native-device and full recovery-browser qualification remain open
- Size: implemented foundation slice

**Acceptance:** Verified email identities, protected lookup material, rotating session families, revoke-one/revoke-all, recovery and additive DID-link slots; API, web and mobile move to the new model together.

**Verification:** Token replay/family revocation, expiry, account ambiguity, cross-account claim and web/mobile session fixtures. Add legacy-password/account migration cases only for inventoried real accounts.

**Out of scope:** No automatic email-equality merge, creator authority inference, Subcults account import or PDS provisioning.

**Rollback:** Recreate clean development state from migrations. If retained real accounts are discovered, specify a bounded migration and rollback before touching them; otherwise do not add dual-read or legacy-session code.

## AT-01 — Establish the minimal Go AT Protocol kernel

- Repository: OS
- Priority: P0
- Depends on: DB-01, INV-01
- Status: syntax, encrypted identity-only persistence, confidential-client metadata/JWKS, hardened resolver, start/callback and link/list/local-unlink UI implemented. Remote revocation, real provider qualification and Lexicon admission remain open in Gitea #9, #10 and #11.
- Size: active multi-slice item

**Acceptance:** Pinned minimal Indigo surface; canonical syntax/DID/handle/URI/CID validation; reviewed Lexicon subset; identity-only OAuth link/unlink; no publication scope by default.

**Verification:** Shared valid/invalid fixture corpus passes Go and TypeScript `@atproto/lex`; bounded resolver and OAuth state/replay/revocation tests.

**Out of scope:** No full Indigo fork, PDS hosting, record publication, Jetstream/Tap service or all old Lexicons.

**Rollback:** Module remains disabled behind internal interfaces; remove it without changing existing OS event/ticket flows.

## MODEL-01 — Add the minimum cultural record model

- Repository: OS
- Priority: P1
- Depends on: IDENT-01, AT-01
- Status: data model, workspace-scoped CRUD API and public-preview projection implemented 2026-09-23; see [cultural-model.md](cultural-model.md) and execution-log.md. Publication (writing to a PDS), projection/discovery ingestion and UI remain open in DISC-01/PUB-01/UX-01.
- Size: 5–8 engineering days

**Acceptance:** Define only journey-required Profile/Act, Place/Venue, public Event occurrence and private operator-event relation; preserve public/private location and time semantics.

**Verification:** Fresh/upgrade DB cases, ownership and cross-workspace negatives, duplicate occurrence, DST/reschedule, protected location and serializer sentinels.

**Out of scope:** No wholesale touring/social schema, streaming, alliances, posts or reputation.

**Rollback:** Additive tables and nullable relationships; unlinked operator events retain existing behavior.

## DISC-01 — Build validated public projection and discovery

- Repository: OS
- Priority: P1
- Depends on: MODEL-01
- Status: allowlisted, restart-safe Jetstream projection into `at_projection_*`
  implemented 2026-09-24 (migration 000011); see [projection.md](projection.md)
  and execution-log.md. Discovery UI, backfill tooling and reconciliation
  with `cultural_*` remain open in PUB-01/UX-01.
- Size: 4–7 engineering days

**Acceptance:** Accepted collections only; restart-safe cursor; URI/CID provenance; idempotent projection; delete/unavailable state; bounded backfill and quarantine.

**Verification:** Fixture stream replay, restart/cursor, out-of-order revision, deletion, malformed/oversized record, private-network resolution and recovery tests.

**Out of scope:** No complete AppView, every historical collection, ranking system or early broker/microservice split.

**Rollback:** Projections are rebuildable; disabling ingestion does not delete private operations or canonical PDS records.

## PUB-01 — Add authorized publication and reconciliation

- Repository: OS
- Priority: P1
- Depends on: IDENT-01, MODEL-01, DISC-01
- Status: proposed
- Size: 6–10 engineering days

**Acceptance:** Explicit preview/approval; workspace plus creator authority; separately scoped OAuth; payload allowlist/digest; stable idempotency; CID preconditions; exact observed outcome; bounded retries and quarantine.

**Verification:** Unauthorized actor, revoked grant, duplicate intent, stale CID, timeout-after-write, lost response, restart/replay and delayed projection tests against disposable authoritative fixtures.

**Out of scope:** No publish-on-save, broad service impersonation, exactly-once claim, new Lexicon field without review or production write.

**Rollback:** Stopping code cannot undo a PDS write; use reviewed compensating writes or preserve and reconcile the record with fresh authority.

## UX-01 — Deliver the unified account, operator and attendee journeys

- Repository: OS
- Priority: P2
- Depends on: DISC-01, PUB-01
- Status: proposed
- Size: 5–8 engineering days

**Acceptance:** One account/session; operator public-record preview and state; accessible pending/conflict UI; public discovery/reservation handoff; guests do not require an AT account.

**Verification:** Browser journeys, mobile real-device smoke, keyboard/accessibility checks, deep links, session refresh and PDS/indexing outage behavior.

**Out of scope:** No complete visual redesign or mandatory native app.

**Rollback:** Existing direct public URL and local event lifecycle stay usable while new cultural/publication UI is disabled.

## CONSENT-01 — Define delivery consent without attendance inference

- Repository: OS
- Priority: P2
- Depends on: IDENT-01
- Status: grant schema, `checkSendPermission` and send-time recheck implemented 2026-09-24 (migration 000013, issue #23); see execution-log.md and docs/development/consent.md. No audit-redaction fixture exists yet. SIGNAL-01 (issue #24, below) is the implemented announcement send path.
- Size: 2–4 engineering days

**Acceptance:** Separate transactional notices from marketing; define sender/channel/purpose/scope/verification/suppression; no ticket/contact import grants consent.

**Verification:** No-consent/no-send, revoked-before-send, wrong-purpose, suppression and audit-redaction fixtures.

**Out of scope:** No autonomous marketing, SMS/social DM automation or legacy audience import.

**Rollback:** Delivery remains transactional-only and fail-closed until an accepted consent contract exists.

## SIGNAL-01 — Implement one scoped announcement channel and delivery worker

- Repository: OS
- Priority: P2
- Depends on: CONSENT-01
- Status: verified email through the existing Resend outbox chosen as the one channel; migration 000014 (`announcements`, `announcement_deliveries`, `consent_grants.withdraw_token_hash`); draft/preview/schedule/cancel/list/get endpoints behind a new `manage_announcements` permission; `-announce` dispatch mode re-deriving the audience and enqueueing per-recipient `email_outbox` rows; synthetic grant/schedule/withdraw/dispatch/send journey implemented 2026-09-24 (issue #24); see execution-log.md and docs/development/announcements.md. No live deliverability test or permissioned pilot has been run (blocked on #7); no SMS or second channel.
- Size: 3–5 engineering days

**Acceptance:** Choose one verified channel using pilot need and observed cost; explicit scheduling with preview, cancellation and final consent checks; record provider delivery outcomes, bounded retries, cost and suppression; verify synthetic grant/schedule/revoke/send journey before a limited permissioned pilot.

**Verification:** Synthetic PostgreSQL journey (grant, confirm, schedule, preview, withdraw, dispatch, send with a fake sender); permission-boundary and past-scheduling rejection fixtures.

**Out of scope:** No SMS/social DM channel, no live send test, no UI, no re-send or per-recipient personalization beyond the withdraw link.

**Rollback:** Stop invoking `-announce`; existing transactional sending and CONSENT-01's grant/withdraw endpoints are unaffected. Roll back the application only; do not drop the additive migration while any dispatched announcement's delivery ledger must be retained for audit.

## QUAL-01 — Qualify the consolidated protected pilot

- Repository: OS
- Priority: P2
- Depends on: UX-01, CONSENT-01
- Status: proposed
- Size: determined by observation windows

**Acceptance:** Exact artifact/revision evidence; clean install and restore; provider/PDS/browser proof; privacy fixtures; outage behavior; independent review; rollback rehearsal. Add legacy-data migration evidence only if retained data exists.

**Verification:** Complete release evidence matrix without substituting health, registration or synthetic bypasses for natural behavior.

**Out of scope:** No public launch, destructive cleanup, external-state mutation or real-user migration without separate authorization.

**Rollback:** Preserve immutable migration ledgers and external-write evidence. A previous service/traffic return procedure is required only if a deployed consumer is found during qualification.

## COMMONS-01 — Publish reusable AT application conformance fixtures

- Repository: OS or a rights-reviewed extracted package
- Priority: P3
- Depends on: AT-01, PUB-01
- Status: proposed
- Size: 3–6 engineering days plus external review

**Acceptance:** Independent implementation can reproduce supported syntax, Lexicon, OAuth, publication and reconciliation cases without private Subcult services.

**Verification:** Second-language or second-implementation reproduction report with exact versions and known gaps.

**Out of scope:** No automatic standard, namespace governance claim, copied secrets/data or unreviewed relicensing.

**Rollback:** Keep package private until rights, maintenance and disclosure review pass.

## LIFE-01 — Specify cancellation and rescheduling before refunds

- Repository: OS
- Priority: active development by owner request, 2026-09-24
- Depends on: QUAL-01 for rollout; independent source work promoted
- Status: [state matrix, occurrence edit safeguards and private owner worklist](event-lifecycle-changes.md) implemented, including durable decision-key replay and unsent-only supersession. Destination-scoped dispatch infrastructure is locally qualified with synthetic adapters; owner-only listing notice previews, atomic approval/queuing, send-time guards and per-recipient outcomes are implemented locally. Live delivery, operator reconciliation controls and coordinated event changes remain open. Merged worklist baseline `8166559` has passing hosted CI run 10160, rechecked 2026-09-29.
- Size: bounded multi-slice work

**Acceptance:** State matrix covers public record, operator plan, tickets, notice, refund policy, projection and archive continuity.

**Verification:** State-machine, stale-CID, provider test-mode and participant-notice cases before implementation completion.

**Out of scope:** No assumed automatic refund, deletion or live charge.

**Rollback:** Preserve prior published/ticket state and require explicit compensating action for external writes.

## OFFLINE-01 — Research disconnected door operations

- Repository: OS
- Priority: active source research by owner request, 2026-09-24
- Depends on: QUAL-01 for rollout; independent research promoted
- Status: [synthetic research slice implemented](../research/offline-door-experiment-2026-09-25.md), with Go merge-model tests and two separate Node client processes. The harness passed again 2026-09-29. Physical-device partition/reconnect, scanner, persistence and manual-fallback qualification remain open; disconnected admission remains unavailable.
- Size: research slice complete; device qualification required before a product slice

**Acceptance:** Define snapshot expiry, revocation, duplicate check-in conflict, reconnect merge and device loss for a bounded pilot.

**Verification:** Two-device partition/reconnect experiments and real-device plan.

**Out of scope:** No production offline guarantee or permanent ticket cache.

**Rollback:** Offline capability remains disabled; server-authoritative door flow stays intact.

## Parked options

These options were closed as no-go for now on 2026-10-02. None has the demand,
partner or prerequisite evidence its issue requires. Each entry names the
evidence that would justify a new issue. Reopening one does not admit it to
the delivery order; it still needs its own acceptance criteria and checks.

### COMMONS-NS — Namespace migration and reader compatibility examples

- Problem: show old/new reader behavior, rollback and deletion propagation when a published namespace or schema changes.
- Reopen when: COMMONS-01 (#47) is complete and a real schema change has two independent readers to exercise.
- Source: [upstream.md](upstream.md).
- Parked 2026-10-02; Gitea issue #48 closed as no-go for now.

### CAL-SYNC — Calendar subscription and recurrence adapters

- Problem: one iCalendar adapter with stable occurrence IDs, recurrence exceptions, timezones and cancellation.
- Reopen when: operators ask for a specific calendar integration after CAL-01 (#22) and the pilot (#36).
- Source: [first-draft plan](../research/first-draft-plan.md); [calendar interoperability](../research/Subcult%20Research%20Dossier/04%20AT%20Protocol/Subcult%20Calendar%20Interoperability.md).
- Parked 2026-10-02; Gitea issue #59 closed as no-go for now.

### TOUR-01 — Tours, appearances and multi-host planning

- Problem: Tour/Appearance aggregates and multi-host permissions across occurrences.
- Reopen when: a pilot operator shows a recurring regional-tour need that MODEL-01 occurrences cannot handle.
- Source: [extraction manifest](subcults-extraction-manifest.md); [platform proposal](../research/2026-09-19-subcult-platform-proposal.md).
- Parked 2026-10-02; Gitea issue #60 closed as no-go for now.

### DIRECTORY-01 — Permissioned contacts, skills and venue availability

- Problem: private venue, vendor, artist, gear and skills directories with explicitly approved sharing.
- Reopen when: pilot collectives ask for this discovery and the privacy review (#15) covers shared contact records. No public reliability scoring ([ADR 0008](../adr/0008-excluded-social-ranking-reputation.md)).
- Source: [first-draft plan](../research/first-draft-plan.md).
- Parked 2026-10-02; Gitea issue #62 closed as no-go for now.

### GEAR-01 — Equipment lending and logistics commitments

- Problem: shared gear requests, custody, condition and return tracking.
- Reopen when: DIRECTORY-01 exists and a pilot partner commits to using and maintaining the workflow.
- Source: [first-draft plan](../research/first-draft-plan.md).
- Parked 2026-10-02; Gitea issue #63 closed as no-go for now.

### COOP-01 — Multi-collective and cooperative revenue coordination

- Problem: shared events across organizations, with separate attribution, money and contact access.
- Reopen when: a real cross-organization workflow is documented and its settlement, legal and accounting requirements are known. No pooled funds in the initial release.
- Source: [first-draft plan](../research/first-draft-plan.md).
- Parked 2026-10-02; Gitea issue #64 closed as no-go for now.

### ARTS-REPORT — Arts-organization grant reporting exports

- Problem: reproducible event and financial summaries for a funder report without attendee surveillance.
- Reopen when: an operator brings a specific funder's reporting requirements, after EXPORT-01 (#54) and METRICS-01 (#37).
- Source: [first-draft plan](../research/first-draft-plan.md).
- Parked 2026-10-02; Gitea issue #65 closed as no-go for now.

### CHANNELS-01 — SMS and additional announcement channels

- Problem: channels beyond SIGNAL-01 email, each with sender verification and suppression.
- Reopen when: pilot operators show demand for a specific channel and its provider unit cost is known. No autonomous marketing or social DM blasts.
- Source: [privacy and consent](../research/Subcult%20Research%20Dossier/03%20Product/Subcult%20Privacy%20and%20Consent.md); [announcements](announcements.md).
- Parked 2026-10-02; Gitea issue #66 closed as no-go for now.

### PDS-01 — Bounded PDS invitations and hosted accounts

- Problem: invitation-based hosted PDS accounts with expiry, migration, key custody and incident duties.
- Reopen when: users need hosted accounts and someone accepts abuse, recovery and maintenance ownership, after AT-LIVE (#10). A new operational ADR and threat model are also required ([extraction manifest](subcults-extraction-manifest.md)).
- Source: [PDS invite research](../research/Subcult%20Research%20Dossier/04%20AT%20Protocol/Subcult%20PDS%20Invite%20Research.md).
- Parked 2026-10-02; Gitea issue #69 closed as no-go for now.

### AUDIO-01 — Optional live audio and streaming

- Problem: live audio as an optional integration, outside the core event workflow.
- Reopen when: recurring paid demand is shown and moderation, recording-rights, accessibility and provider costs are estimated. LiveKit/streaming stays out of the core dependency chain ([ADR 0008](../adr/0008-excluded-social-ranking-reputation.md)).
- Source: [development brief](development-brief.md).
- Parked 2026-10-02; Gitea issue #70 closed as no-go for now.

### COMMERCE-EXTRA — Memberships, marketplace and advanced commerce

- Problem: paid memberships, resale, dynamic pricing, vendor/festival commerce and invoicing, each a separate hypothesis.
- Reopen when: one of these has demonstrated demand after COMMERCE-01 (#53) and the pilot, with provider and regulatory burden compared. No all-in-one marketplace.
- Source: [MVP scope](../research/Subcult%20Research%20Dossier/03%20Product/Subcult%20MVP%20Scope.md).
- Parked 2026-10-02; Gitea issue #71 closed as no-go for now.

## Portfolio items outside Subcult OS

These proposals belong to the wider Subcult.tv portfolio, not to this
repository. Their substance lives in the Funding Kit under
`docs/research/Subcult Funding Kit/`. They are tracked outside this
repository's delivery tracker; Gitea issues #73–#82 are not Subcult OS work.

- #73 PATCHWORK-01, Patchwork public-resource pilot: [Patchwork Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Patchwork%20Proposal.md).
- #74 ORG-COMMONS, organization authority and publication boundaries: [Organization Commons Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Organization%20Commons%20Proposal.md).
- #75 RESOURCE-COMMONS, public-resource provenance and correction fixtures: [Protocol Commons Roadmap](../research/Subcult%20Funding%20Kit/01%20House/Protocol%20Commons%20Roadmap.md).
- #76 COMMUNITY-OPS, community operations as a Commons consumer: [Community Operations Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Community%20Operations%20Proposal.md).
- #77 MEMBER-COMMS, membership communications: [Membership Communications Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Membership%20Communications%20Proposal.md).
- #78 MEETING-LAB, public meeting artifacts with private deliberation: [Open Meeting Lab Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Open%20Meeting%20Lab%20Proposal.md).
- #79 FORUM-01, community forum feasibility: [Community Forum Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Community%20Forum%20Proposal.md).
- #80 CIVIC-LAB, source receipts and civic evidence tooling: [Civic Evidence Lab Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Civic%20Evidence%20Lab%20Proposal.md).
- #81 SCAFFOLD-01, scaffold reserves and DSA-suite options: [Scaffold Reserve Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/Scaffold%20Reserve%20Proposal.md) and [DSA Suite Proposal](../research/Subcult%20Funding%20Kit/03%20Proposals/DSA%20Suite%20Proposal.md).
- #82 MEDIA-01, adjacent media and evidence projects: [Portfolio and Boundaries](../research/Subcult%20Funding%20Kit/01%20House/Portfolio%20and%20Boundaries.md).
