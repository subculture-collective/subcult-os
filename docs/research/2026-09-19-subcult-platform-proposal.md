# Subcult: shared infrastructure for independent culture

Research and business/product proposal · 19 September 2026 · Draft for discussion

## Executive proposal

Build one Subcult product that helps independent music and cultural communities find each other, maintain direct relationships, produce events, and carry their knowledge into the next gathering. Combine Subcults' scene discovery, touring, public identities, Signals, and AT Protocol foundation with Subcult OS's private event operations, staffing, commitments, door, settlement, and archives.

**Positioning:** Subcult connects the people, places, and work that keep independent culture alive.

**Product promise:** Find your scene. Bring people together. Make it happen. Keep the connection.

The initial customer remains independent venues, music collectives, artist teams, recurring promoters, community arts spaces, and small festivals. Broader capability should deepen service to those customers. It should not expand the initial sales effort into corporate conferences or every type of social network.

Recommend a single public brand, **Subcult**, with a participant experience and a private **Studio**. Within Studio, organizers have audience tools and event operations. Keep existing repositories intact during validation; integrate through explicit contracts before deciding whether to consolidate source code.

The commercial opportunity is recurring software for teams that maintain both a community and an event calendar. Public discovery and participation stay free; teams pay for reliable coordination, direct communication, integrations, reporting, and support. AT Protocol supplies portable public identity and publishing. It does not itself supply customers, private marketing permission, a payment system, or a ready-made shared workspace.

## Evidence and present capability

Inspected local sources:

- `subcults`, `/home/onnwee/Work/subcult/subcults`, branch `fix/main-regression-recovery`, revision `3cf88ec`; worktree clean when inspected.
- `subcult-os`, current research worktree, revision `abf3f50`; clean before this proposal.

This is source and documentation research. No deployment, provider delivery, real event, or new browser journey was qualified in this investigation. The prior competitor response is background; new external claims below use the linked sources checked for this proposal.

| Foundation | Source evidence | Implication |
| --- | --- | --- |
| Subcults public domain | Profiles/Acts, Places/Venues, Scenes, Events, Appearances, Tours; PostgreSQL touring repository | Reuse this richer identity and occurrence model |
| Subcults audience | Contact points, scoped consent and suppression, audience relationships, Signal revisions and dispatcher | Build audience tooling on these boundaries; existence is not deliverability proof |
| Subcults portability | Nine `tv.subcult.*` lexicons; confidential OAuth; PDS publication; URI/CID mappings; sync and reconciliation | Already a substantial AT integration, not a future checkbox |
| Subcult OS operations | Workspaces, roles/applications, staffing, commitments, tickets, check-in, settlements, archives | Bring these workflows into the shared product |
| Mobile | Subcult OS Expo routes for attendees and operators | Reuse selectively after unified identity and event contracts are defined |

Subcults' August 8 product plan says durable adapters and configured login are missing. Its later release document and current SQL/source modules show those local implementation gaps were subsequently addressed. Therefore that older paragraph is historical, not current source truth. The August 9 release contract still records frontend failures and unqualified provider/PDS operation; these are documented unresolved gates, not a fresh test result. Its exact failing-test count must be refreshed before budgeting release repair.

Source anchors: Subcults README (`subcults/README.md`), audience and touring plan (`subcults/docs/product/AUDIENCE_DROPS_AND_TOURING.md`), accepted relationship model (`subcults/docs/adr/0007-scene-signals-touring-relationship-model.md`), release contract (`subcults/docs/product/PUBLIC_BETA_RELEASE_STATUS.md`), AT operations (`subcults/docs/operations/ATPROTO_PDS.md`), publication source (`subcults/internal/atprotocol/publication.go`), Signal delivery source (`subcults/internal/signal/delivery.go`), [OS API routes](../../backend/internal/app/app.go), [OS schema](../../backend/internal/app/schema.sql).

## Laylo and the competitive opening

Laylo competes directly with the proposed audience product. It markets branded Drop pages, verified signups, presaves, tour promotion, SMS/email/Instagram contact channels, segmentation, and integrations. Its first-party comparison pages advertise Pro at $25/month plus messaging with unlimited contacts. The dedicated pricing page did not expose readable terms in this research, so confirm checkout terms before quoting this as a contractual price. Treat Laylo's conversion multipliers as vendor marketing, not independently established performance. [Drop pages](https://laylo.com/features/drops), [Laylo's own feature/pricing comparison](https://laylo.com/compare/cobrand).

Laylo also advertises data import/export, ticket and commerce integrations, and campaign measurement. Consequently, claiming that Subcult uniquely lets creators export their audience would be weak. The stronger distinction is an interoperable public record of cultural activity connected to participant-controlled interests and practical production workflows. [Laylo's event-marketing comparison](https://laylo.com/compare/hive).

| Competitor/category | Established strength | Implication for Subcult |
| --- | --- | --- |
| Laylo | Artist audience capture and activation around releases, tours, tickets and merchandise | Match a small reliable communication journey; differentiate through scene context and operations |
| Luma | Registration, calendars, community memberships and guest communication | Public event creation must feel easy; deeper obligations justify an additional tool |
| Eventeny | Vendors, volunteers, maps, tickets and festival coordination | Integrated operations already exist; target smaller recurring cultural teams with simpler adoption |
| Opendate / Prism.fm | Professional live-event booking, financial and venue operations | Validate the paying buyer while competing on segment fit and workflow simplicity |
| Smoke Signal / community calendar ecosystem | AT-native events and RSVPs | Collaborate on shared schemas and interoperability instead of inventing another isolated event format |
| Spreadsheets + messaging + ticketing | Familiar, flexible, already adopted | Import existing records and coexist with their ticket seller from day one |

Current primary references: [Luma memberships](https://help.luma.com/p/calendar-memberships), [Eventeny](https://www.eventeny.com/why-eventeny/), [Opendate](https://www.opendate.io/pricing), [Prism](https://prism.fm/), [Smoke Signal documentation](https://docs.smokesignal.events/).

This is a positioning assessment, not a claim that competitors lack every overlapping feature. Audience acquisition, deliverability, integrations and operational polish remain substantial incumbent advantages.

## The combined experience

### Discover and belong

Participants browse a map or list by a chosen city, date, scene and interest. Artist home territory remains context; a show appears where it happens. Profiles connect artists, venues, collectives and curators to appearances, tours, releases and public archives. People can save a city without disclosing where they live or their device location.

Scene membership, following an artist, saving an event, buying a ticket, joining a crew, and requesting announcements remain distinct relationships. Community membership need not be public. A person can participate through a guest ticket or verified email without first understanding AT Protocol.

### Reach people who asked to hear from you

An artist creates a Signal for a release, tour, presale or broadcast. A person chooses the artist, subject, city/date and delivery channel they want. Studio shows eligible recipients and why they qualify. Start with email and web push; add SMS through a provider after messaging costs, delivery and opt-out operations are proven.

Tour interest is a declared preference, not a prediction of attendance or a ticket sale. Messaging permission does not transfer automatically between an artist, label, venue and allied scene. Give participants one comprehensible preference center and a visible source for each subscription.

Laylo sets a high standard here. The initial product needs one excellent opt-in → verified contact → scheduled announcement → delivery → withdrawal journey, including a pre-send permission check and duplicate-send protection. Existing dispatcher code is a foundation, not proof of those complete guarantees.

### Collaborate and produce

An organizer opens an Event Operations Record from the same public event. They assign crew, receive role applications, track commitments and load-in requirements, run the door, reconcile income and expenses, record payout obligations, and complete the archive. A touring artist sees the appearances they participate in; they do not automatically gain access to the host's private workspace.

Later, scenes can deliberately share opportunities, equipment availability, calls for performers and introductions. Use explicit offers and requests; do not infer private reliability ratings from past participation. Live audio and streams remain an optional later way to participate between events, subject to the existing runtime qualification gap.

### Carry the work forward

After an event, the private report preserves commitments, staffing and financial closeout. A separately approved public archive credits consenting contributors and publishes selected artwork, recordings and links. The next event inherits a reusable plan. An attendee may separately request future announcements.

The reinforcing loop is: discovery → chosen connection → announcement → participation → coordinated production → archive → next gathering. Value can accrue at each step without requiring every customer to adopt every module.

### Example: a regional tour

A Chicago artist announces dates in Milwaukee and Detroit. Each local show belongs to its host and appears in that city's discovery results. A participant selects only Detroit presale updates. The Detroit host opens the linked operations record, assigns sound and door, and confirms the artist's set. Ticketing can stay with an external seller. A reschedule updates the canonical public occurrence and triggers the appropriate transactional workflow; promotional delivery still needs consent. After settlement, the host retains private money records while the artist and host approve public credits. The next local booking can reuse the operational plan and the participant's still-valid subscription scope.

## Reconcile the two domain models before merging systems

| Concept | Proposed meaning and authority |
| --- | --- |
| Person/account | Internal person ID, optionally linked to verified AT DIDs and separately verified contact points |
| Profile/Act | Public identity and creative project; controller/publisher must be explicit |
| Scene | Community and cultural context; it is not automatically a billing account or legal entity |
| Workspace | Private team, permissions and commercial subscription |
| Event | One actual occurrence and public identity; stable mapping to an operations record |
| Host | Multiple public credits are possible; one owning workspace controls each private operations record |
| Appearance | An Act's participation in an Event; belongs to a Tour when appropriate |
| Signal | Versioned invitation to act; delivery authorization remains private |
| Ticket/order | Transactional entitlement with its own lifecycle and system of record |
| Archive | Private continuity plus separately consented public material |

Do not equate an email match with proof of account ownership. Require authenticated linking. Do not deduplicate occurrences merely by similar titles and dates. Preserve source IDs, conflicts and the authority of corrections.

The OS `draft/published/end_of_night` lifecycle and Subcults' announced/postponed/cancelled/completed vocabulary describe different concerns. Keep publication state, public occurrence status, private operations state and ticket validity separate, with an explicit transition table. Public co-host credit never grants private access.

Recommended technical direction: Subcults supplies public identity, occurrence discovery and AT publishing; OS supplies operations behind a service boundary. Start with a stable event mapping and authenticated server-to-server access. Keep financial writes in the transactional database; publish public changes through an idempotent outbox with reconciliation. A delayed AT projection must not undo a successful payment or suggest that an accepted PDS write failed. Resolve conflicting public edits by authority and revision, never by blindly copying whichever system changed last.

## AT Protocol's role and limits

Use DIDs for portable account identity, PDS repositories for intentionally public records, OAuth for exact collection/action authorization, and a rebuildable application projection for search and maps. Custom lexicons describe our domain but do not automatically appear in Bluesky feeds or another app's interface. Interoperability requires another application to understand the schema. [Protocol overview](https://atproto.com/guides/overview), [OAuth](https://atproto.com/specs/oauth), [permissions](https://atproto.com/specs/permission).

| Data | Initial home | Portability promise |
| --- | --- | --- |
| Published profiles, scenes, events, tours, appearances, safe public assertions | Creator-authorized PDS records + indexed projection | Repository export/migration and compatible third-party readers |
| Private drafts, staffing, settlement, contact points, consent, suppression, precise protected locations | Access-controlled application storage | Explicit authorized exports and future interoperable adapters |
| Payments and ticket validity | Transactional application/provider records | Reconciled exports; never a public payment/contact ledger |
| Experimental shared workspace records | Isolated AT Spaces test environment | Research evidence only until the extension is qualified |

The official August 20, 2026 Spaces announcement describes an alpha for non-public repositories and sync, controlled by a space authority. It explicitly warns against production use. Spaces provides access control, not end-to-end confidentiality: authorized parties can read the content. Use synthetic crew/venue data for experiments; keep real sensitive records in the existing private system. [AT Spaces alpha](https://atproto.com/blog/atproto-spaces-alpha), [permissioned-data proposal](https://github.com/bluesky-social/proposals/blob/main/0016-permissioned-data/README.md).

Public data can be copied. Deleting or withdrawing a record cannot guarantee deletion from every downstream archive. Do not publish precise underground locations, private attendance, email/phone hashes, marketing permissions or safety dossiers. A signature proves authorship/integrity under an identity, not that an event claim is true. Moderation, authorized representation and disputed claims remain application responsibilities.

## Contribution agenda: useful beyond Subcult

Propose small reusable work with fixtures and an independent consumer. Separate core protocol changes, reference implementation improvements, shared lexicons and SDKs. Adoption by upstream maintainers is uncertain and must not block the commercial pilot.

### 1. Shared event compatibility and occurrence fixtures — first priority

**Layer:** community lexicons and adapters. **Collaborators to approach later:** Lexicon Community and Smoke Signal maintainers.

Existing `community.lexicon.calendar.event` records and related RSVP work predate this proposal. Subcults currently emits only `tv.subcult.*`; adding public calendar compatibility therefore needs an explicit namespace/migration decision, not silent dual publication. [Smoke Signal's community-lexicon history](https://docs.smokesignal.events/blog/), [calendar schema discussion](https://discourse.lexicon.community/t/event-header-images/39).

Deliver a field-by-field compatibility RFC and fixtures for a local show, touring appearance, festival program, online broadcast, cancellation, reschedule, timezone/DST boundary and coarse-location event. Reuse the common event when it fits; represent tours and appearances as linked extensions. Test unknown fields and safe loss of unsupported detail. Preserve one canonical identity with provenance when importing; avoid creating competing editable copies.

**Public benefit:** calendars, volunteer apps and cultural directories exchange usable occurrences. **Subcult benefit:** events reach other AT applications. **Acceptance:** an independent consumer renders the same occurrence and cancellation correctly without exposing a protected location. This is proposed work, not a tested compatibility claim.

### 2. Reliable projection/rebuild conformance kit — first priority

**Layer:** open-source tooling, documentation and targeted Indigo/Tap fixes where reproduced.

Package fixtures and a small reference harness for accepted writes awaiting projection, duplicate delivery, deletes, stale CIDs, out-of-order observations, PDS migration, interrupted backfill, and a rebuild from authoritative records. Document when to dereference the latest URI and when a strong reference deliberately pins a specific version.

**Public benefit:** any AT app can test whether its search/index view stays faithful to source records. **Subcult benefit:** fewer stale dates and contradictory tour pages. **Acceptance:** rebuild yields the expected projection after injected interruptions, without reviving withdrawn records. Review upstream facilities first; contribute missing tests instead of creating a competing sync stack.

### 3. Private-workspace interoperability tests for Spaces — experimental

**Layer:** feedback, reproducible cases and tests against the evolving Spaces proposal/reference implementations.

Exercise temporary crew membership, expiry, removing a team member, app authorization withdrawal, account migration, authority recovery, and partitioned permissions between general crew and finance. Define and measure how long revoked principals can continue reading through caches or sessions. Distinguish denying future reads from erasing copies already obtained.

**Public benefit:** associations, classrooms, volunteer groups and publications need these behaviors. **Subcult benefit:** a future portable collaboration layer. **Acceptance:** two implementations agree on specified membership/revocation cases using synthetic data. Treat gaps as questions for maintainers; do not imply present Spaces behavior guarantees row-level roles or automatic group administration.

### 4. Organizational delegation and attribution — medium priority

**Layer:** application convention/SDK first; protocol proposal only for demonstrated missing authority semantics.

Describe a collective identity with multiple human editors: who granted each role, what records/actions it covers, when it expires, how it is revoked, and how succession works. Existing OAuth repo permissions already restrict collections/actions; requesting scopes is not a complete multi-person organization governance system. [Current permissions specification](https://atproto.com/specs/permission).

Start with workspace RBAC, private audit logs and explicit publisher control. A public delegation claim alone cannot authorize writes to someone else's PDS. Any standard must explain the enforcing server and authenticated actor, not just a JSON shape.

**Public benefit:** bands, newsrooms, clubs and nonprofits can maintain continuity when people leave. **Subcult benefit:** a promoter can schedule appearances without controlling the artist's whole account. **Acceptance:** a removed editor cannot perform new authorized writes, and a receiver can distinguish original author from delegated actor without exposing private team membership unnecessarily.

### 5. Consent-preserving audience transfer toolkit — medium priority

**Layer:** private interchange format, validation library and adapter fixtures; potentially Spaces research later.

Define an authorized transfer bundle for sender identity, channel, purpose, scope, contact verification, disclosure version, evidence, grant/revocation and suppression. Export contacts only through an approved private channel. Never publish contact hashes or consent records to the public firehose. A destination must preserve restrictions and refuse activation if evidence is missing; importing data does not itself establish messaging permission.

**Public benefit:** creators and subscribers can switch providers with fewer lost preferences and accidental resubscriptions. **Subcult benefit:** credible audience portability and easier onboarding. **Acceptance:** an imported revoked/suppressed contact stays unsendable, and revocation after scheduling prevents dispatch. This is a technical evidence contract, not a promise that consent is legally transferable in every circumstance.

### 6. Bounded PDS invitations — small reference-implementation candidate

**Layer:** reference PDS lifecycle/test improvement, not a new social protocol primitive.

Subcults' runbook identifies ten-minute invite expiry as a signup blocker and mentions missing upstream revocation. Current upstream account-manager source and the `com.atproto.admin.disableInviteCodes` procedure already expose invite disabling; do not pitch revocation as a missing feature without checking the deployed version and endpoint. [Reference PDS account manager](https://github.com/bluesky-social/atproto/blob/main/packages/pds/src/account-manager/account-manager.ts), [invite-disabling lexicon](https://github.com/bluesky-social/atproto/blob/main/lexicons/com/atproto/admin/disableInviteCodes.json).

Investigate server-enforced `expiresAt`, atomic redemption checks and expiry/revocation race fixtures. If an existing implementation meets the requirement, contribute documentation or integration tests. A cleanup job that eventually disables a code is not strict expiry at redemption.

**Public benefit:** safer bounded onboarding for small hosts. **Subcult benefit:** one concrete public-signup gate can be addressed. **Acceptance:** an expired invite cannot create an account even when the cleanup worker is stopped. Capacity, restore and synchronization gates still remain.

Allocate approximately 10–15% of engineering capacity to the first two contributions and bounded Spaces experiments; this is a proposed allocation. Require useful local tests and an independent user before expanding standards work. Do not make a new protocol fork, global reputation system, ticket-payment protocol or universal private CRM the initial contribution.

## Commercial plan

Begin with recurring organizers in one city and a small connected touring corridor. Choose the geography where the founding team can recruit and support real operators. Anchor customers are venues and collectives that coordinate multiple collaborators and events every month. Artists, guests and occasional crew can join without becoming paying seats.

Test a subscription with a limited core workspace included and separately metered communication delivery:

| Offer hypothesis | Price to test | Scope |
| --- | --- | --- |
| Public participation | Free | Discovery, public profiles, guest participation and preference controls |
| Collective Studio | $39/month | Shared event planning, commitments, archive and basic Signals |
| Venue Studio | $99/month | More active events, permission controls, reconciliation and integrations |
| Network Studio | $249+/month | Multiple workspaces, aggregate reporting and assisted onboarding |

These are research hypotheses, not adopted prices or revenue forecasts. Specify event/storage/send limits after observing use; avoid unlimited SMS or support. Revenue should pay for coordination and service reliability, not sale of behavioral data or ranking influence. Offer clear ticket-processing costs if native commerce is enabled. Revenue shares and payment routing need a separate design before launch.

Illustrative subscription arithmetic: 50 collective customers at $39 plus 20 venues at $99 equals $3,930 MRR/$47,160 ARR; 250 plus 100 equals $19,650 MRR/$235,800 ARR. These are scenarios with no acquisition or retention evidence. Model delivery, infrastructure, onboarding, event-night support, disputes and churn separately before estimating profit or funding needs. Laylo's advertised $25 entry price means a more expensive Subcult plan must prove additional coordination value.

Go to market through five to ten known teams, onboarding their existing ticket seller and spreadsheet. Deliver the first event personally, then measure whether they voluntarily use and pay for the next. Add touring partners through actual appearances and co-host invitations; avoid requiring a large public network before the software is useful. Seek infrastructure grants for shared interoperability work only as a separate funding opportunity, not assumed operating revenue.

## Delivery and validation plan

Indicative sequence, not a fixed release promise; staffing, operating budget and current runtime qualification remain to be established.

| Stage | Deliverable | Exit evidence |
| --- | --- | --- |
| 0: source/runtime refresh and model alignment | Current test/release inventory, entity/lifecycle crosswalk, account linking and event authority ADR | One documented owner for every state transition; refreshed blocker list |
| 1: one combined vertical slice | Public occurrence → private operations record → crew/commitments → closeout | Same event through both surfaces; tenant isolation; restart and browser/device evidence |
| 2: direct relationship loop | Artist/city Signal → verified opt-in → email/push → preference withdrawal | Real bounded provider delivery; post-revocation suppression; retries do not duplicate sends |
| 3: repeat-event pilot | External ticket import, financial reconciliation, reusable archive and permissioned public credits | Five teams each complete two events; three pay to continue without bespoke feature commitments |
| 4: interoperability | Community-calendar adapter and public projection harness | Independent reader and repeatable sync/rebuild cases |
| 5: selective expansion | Additional provider, stronger ticketing, touring collaboration, optional audio | Usage and support economics justify each module; Spaces remains isolated until qualified |

Measure organizer time to publish and staff an event, unowned obligations at doors-open, check-in failures, time to closeout, repeat usage, paid retention, support hours per event and contribution margin after delivery costs. Track opt-out failures, unwanted messages, protected-location exposure and duplicate notifications as guardrails. Report conversion attribution with a disclosed method; a click followed by a purchase is not proof of incremental sales.

The initial product milestone should be: **a participant discovers an out-of-town artist locally, chooses one announcement, attends an event whose host coordinates and closes it in Subcult, and both sides return for another gathering.** That demonstrates the combined value better than separate feature demos.

## Decisions proposed for review

1. Adopt Subcult as the umbrella product; place OS capabilities inside Studio.
2. Use Subcults' public domain and AT publishing as the initial integration foundation, retaining a distinct private operations authority.
3. Pilot with recurring cultural teams and an existing ticket provider; fund native commerce expansion from demonstrated demand.
4. Make scoped communication and a useful post-event archive part of the initial joined journey.
5. Pursue shared calendar compatibility and sync tests first; make Spaces a bounded research contribution.
6. Refresh existing release evidence before committing dates or publishing production claims.

No code merge, namespace migration, deployment, upstream submission or outreach is performed by this proposal. The business thesis remains a testable one: recurring cultural teams will pay for continuity across community, communications and production, while open public records make the surrounding ecosystem stronger.
