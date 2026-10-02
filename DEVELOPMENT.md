> For the product overview, see the [README](README.md).

# subcult-os

Transactional email uses an opt-in Brevo or Resend worker while Proton remains the inbox. See [the email runbook](docs/runbooks/transactional-email.md) for setup and qualification gates. Normal startup does not send mail; existing outbox rows remain held. The `mail-workers` Compose profile and `MAIL_DELIVERY_ENABLED=true` must both be deliberately configured for continuous delivery.

The one implemented announcement channel (verified email; see [`docs/development/announcements.md`](docs/development/announcements.md)) uses a separate `announcement-workers` Compose profile, running `email-deliver -announce -watch`. It only claims due scheduled announcements and enqueues their recipient `email_outbox` rows; it never contacts the mail provider itself, so it does not require `MAIL_DELIVERY_ENABLED=true` and can run independently of `mail-workers`. Actual sending of an announcement's enqueued rows still goes through `mail-workers`/`-send`, and is still subject to `MAIL_DELIVERY_ENABLED`.

Gitea verification installs Node 24, pnpm 10.33.0 and Go 1.26.6 inside each job.
The shared runner must provide Docker Compose for configuration validation. CI
starts a job-scoped PostgreSQL 17 service with disposable credentials and no host
port or persistent volume, then runs `make test-db`; it does not deploy the app.
Local `make verify` and hosted CI are
separate evidence. `make test-db` requires a disposable `TEST_DATABASE_URL` and
runs the complete app and AT adapter packages; the earlier seven failures and
their reconciliation are tracked in [issue #95](https://git.subcult.tv/subculture-collective/subcult-os/issues/95).

Full-stack SUBCULT OS project with a Go backend, Vite React TypeScript Tailwind public web frontend, Expo React Native mobile app, Postgres, Docker Compose, and Open Pilot conventions.

Bootstrapped from `subculture-collective/project-template`.

## Future development

Web and mobile share the [Subcult design system](docs/design-system.md), based
on the original Subcults terminal CSS: sharp corners, monospace typography,
purple actions and cyan focus. Edit
`contracts/design/tokens.json` and run `node scripts/design-tokens.mjs` to update
both clients. The isolated web preview provides a `/design-system` gallery in
development; `make verify` checks generated-token drift and text contrast in both
light and dark palettes. Choose Light, Dark, or System from the web Appearance
selector or mobile Settings → Appearance.

See the [Subcult.tv platform development handoff](docs/development/README.md), [AT Protocol kernel](docs/development/atproto-kernel.md), [Subcults cutover runbook](docs/runbooks/subcults-cutover.md), [ADR 0005](docs/adr/0005-subcult-os-platform-core.md), and [ADR 0006](docs/adr/0006-no-prototype-compatibility-contract.md). Subcult OS is the accepted receiving repository; selected Subcults capabilities are being rewritten behind OS-native boundaries rather than merged wholesale. Prototype API/schema compatibility is not required by default. Canonical identity, the minimal AT syntax kernel, encrypted identity-only OAuth persistence, confidential-client documents, authenticated start/one-time callback routes, and a link/list/local-unlink UI are implemented. Remote revocation, the minimal Lexicon contract and read-only projection are implemented. Live provider interoperability and repository publication remain unqualified or unimplemented.

## Quick start

### T3 worktree development

Prerequisites: Docker with Compose 2.32 or newer, Mise, Bash, curl, and flock.
The checked-in `t3.json` provides Setup Worktree, Start Dev, Watch Backend,
Preview URL, Seed Demo Data, Test, Test Database, Verify, Logs, and Stop Dev actions.
Import the actions from the project's **From t3.json** menu once per T3 environment.
The imported Setup Worktree action runs for new worktrees and waits for dependencies
before the agent starts. Start new worktrees from a branch containing these files;
older branches need this change merged or cherry-picked first. Existing worktrees
with these files can run the same commands manually:

```bash
bash scripts/dev-env.sh start
bash scripts/dev-env.sh seed
bash scripts/dev-env.sh watch
```

Start prints the actual loopback preview URL. The frontend reloads on edits;
Watch Backend restarts the Go service on backend changes while that terminal
remains open. Stop the watcher with Ctrl-C. Use `bash scripts/dev-env.sh stop`
to stop only this worktree's containers, retaining its database and build caches.
Start resumes it. Docker-assigned ports may change after a stop or recreation;
use Preview URL again. These URLs are local to the machine running T3.

Seed Demo Data creates `dev@example.test` with password `local-development-only`,
a Development Collective workspace, and a draft Development Night event. It
reads only that synthetic user's held verification mail and consumes the normal
verification endpoint. Repeating it preserves the existing demo workspace/event.
No external mail is sent.

```bash
bash scripts/dev-env.sh test      # backend, web, mobile, and QA helper tests
bash scripts/dev-env.sh test-db   # separate disposable PostgreSQL database
bash scripts/dev-env.sh verify    # make verify, followed by make test-db
```

The helpers pin Node 24.18.0, pnpm 10.33.0, and Go 1.26.6. Each worktree has
its own Compose project, network, development database, and Go caches. Test DB
containers use temporary storage and are removed after the run, including failed
runs. Test database logs are retained in ignored `.cache/dev-env/test-db.log`;
the container uses a 1 GiB limit. The growing migration suite exhausted the
previous 768 MiB limit and killed a PostgreSQL checkpointer; this repository
limit is independent of the installed shared T3 test recipe.
Test, Test Database and Verify share the installed T3 launcher's host test
lock, so verification does not overlap another worktree's checks. The API mounts
the current Lexicon contracts read-only for cultural record validation.
`make verify` includes formatting and can update Go files. Local results
do not replace hosted CI. The standalone `compose.dev.yml` does not load `.env`
or inherit deployment credentials; live email, AT OAuth, and projection workers
are disabled. Do not merge it with `docker-compose.yml`.

### Existing build and deployment commands

```bash
make verify
```

Useful local commands:

```bash
make help
make quick
make up-build
make urls
make smoke
make alpha-qa
make logs
make down
make reset-db
make dev-mobile
```

## Private venue and event access information

Owners can open **Access worksheet** from a saved event editor, or **Venue access
worksheets** from the workspace. The venue index lets owners add a named reference
and record a separate observation history. Each worksheet records six topics
with explicit Unknown states, source/review dates, conservative expiry and
retained correction history. Venue information never automatically becomes event
verification. The event worksheet also compares the linked venue and event
information for a selected occurrence, with separate source and expiry labels.
These owner-only worksheets do not publish venue claims or collect
personal accommodation requests. See [the worksheet contract](docs/development/event-access-information.md)
for the API, schema-29 boundary and remaining #55 acceptance work.

## First lifecycle slice

The first product slice proves:

1. Owner signs up and creates a Workspace.
2. Owner invites a Member by email; development stores invitation emails in `email_outbox` and exposes recent messages in the Workspace UI.
3. Invitee opens `/invite/{token}`, signs up or signs in with the invited email, and accepts Workspace membership.
4. Owner creates and publishes a direct-link Public Event Page.
5. Guest reserves a free Ticket with email and optional display name; the page shows an in-place confirmation and Ticket link.
6. Member runs mobile-friendly Door Check-In by manual lookup or exact Ticket code.
7. Owner runs End of Night and views the private Event Report.
8. People can belong to multiple Workspaces and switch the active operator home from the Workspace access panel.
9. Public discovery page at `/discover` lists published events without exposing private workspace, archive, staffing, settlement, or application data.
10. Owner-triggered reminder sweeps create private notification activity for due commitments and upcoming staffing without exposing free-text notes publicly.

Run locally:

```bash
make up-build
```

Then open:

- Web: http://localhost:38079
- API health: http://localhost:38080/api/health

The default host ports are intentionally high to avoid common local conflicts: `WEB_PORT=38079`, `API_PORT=38080`, and `POSTGRES_PORT=35432`.

Use `make urls` to print the actual published ports, `make smoke` to check the running Docker stack, and `make reset-db` to delete local Postgres data and rebuild if an old volume has stale credentials.

Recommended local QA loop:

```bash
make up-build
make smoke
make urls
make alpha-qa
make verify
```

If the app is already running, use the Workspace page to inspect development invite/ticket emails instead of connecting to Postgres directly.

## Mobile app

The Expo app in `mobile/` is the phone-first authenticated client for event-time flows. Public event pages, ticket links, invite links, and SEO-friendly discovery remain in the Vite web app.

`mobile/` is currently pinned to Expo SDK 54 so it can run in the App Store / Play Store version of Expo Go during fake-event and real-device testing. Revisit upgrading after switching to development builds.

Mobile styling uses Uniwind with Tailwind v4-compatible semantic tokens. Keep class names semantic (`bg-surface-panel`, `text-fg-primary`, `btn-primary`, `panel`) instead of hard-coding palette utilities in screens.

Start the API/web stack, then run Expo:

```bash
make up-build
make dev-mobile
```

For physical-device testing, point Expo at a backend URL reachable from your phone:

```bash
EXPO_PUBLIC_API_URL=http://<your-lan-ip>:38080 pnpm --dir mobile run start
```

Current mobile app routes include:

- attendee discovery, event detail, reservations, ticket lookup, and ticket wallet
- staff dashboard, workspace/event selector, scanner, door search/check-in, run-of-show, and live dashboard
- organizer event create/edit/publish with event image upload when S3-compatible media storage is configured

Event image uploads are backend-proxied to S3-compatible storage such as MinIO. Configure `MEDIA_S3_ENDPOINT`, `MEDIA_S3_ACCESS_KEY`, `MEDIA_S3_SECRET_KEY`, `MEDIA_S3_BUCKET`, and `MEDIA_PUBLIC_BASE_URL`; keep secrets out of git.

## Current alpha security notes

- Passwords are stored with bcrypt; unverified accounts cannot sign in.
- Email identity lookup uses keyed hashes and encrypted address material. The current `people.email` column remains an operational projection for existing workspace workflows, not the authentication lookup authority.
- Browser sessions use short-lived HttpOnly access cookies plus rotating refresh cookies. Native auth transport is isolated under `/api/mobile/auth/*` and returns separate access and refresh credentials for secure storage; ordinary browser auth responses never expose those credentials as JavaScript-readable headers. Replaying a rotated refresh credential revokes its session family.
- Account recovery revokes every existing session. Matching email text or DID never merges accounts automatically.
- AT OAuth request and session secrets use authenticated encryption under a protocol-specific derived key. The store accepts only identity-level `atproto` scope, one-time callback state, and a DID not owned by another local account. When explicitly enabled, the API publishes confidential-client metadata/JWKS and exposes authenticated start/list/local-unlink routes plus a state-bound callback. Discovery denies proxies, private IP connections and redirects. The operator home makes identity-only permissions and non-merging behavior explicit. The flow remains disabled by default pending bounded live interoperability and provider token-revocation qualification.
- Cookie-authenticated mutating API requests with an `Origin` header must come from the same host or `PUBLIC_WEB_URL`.
- Login attempts are lightly throttled per email/IP in process memory.
- Public free ticket reservations remain guest-accessible without account login.
- Private contacts and commitments help organizers remember scene relationships and promises; they are workspace-only and never shown on public discovery/event pages.
- Event templates let organizers repeat event planning fields privately; template notes are workspace-only and never copied to public event pages.

## Production readiness checklist

- Set `APP_ENV=production`.
- Set a real `DATABASE_URL`; production startup fails without it.
- Set a non-default `SESSION_SECRET` with at least 24 characters.
- Set `IDENTITY_PROTECTION_KEY` to a base64-encoded 32-byte key held in the deployment secret store. Distinct derived domains protect email identity and AT OAuth material. Losing it makes protected material unreadable; rotating it requires a designed data migration.
- Set `PUBLIC_WEB_URL` to the HTTPS web origin used by browsers. The web image build reads the same value (Compose passes it as a build argument) and writes it into the `og:image` address; an unset value builds `https://os.subcult.tv`. Rebuild the web image after changing the origin.
- Keep `ATPROTO_OAUTH_ENABLED=false` until bounded live authorization/callback, refresh and provider-revocation journeys are qualified. When enabling it, configure the exact HTTPS client-metadata, callback and JWKS URLs plus a secret-store-backed multibase P-256 client key; see the AT Protocol kernel document.
- Generate a compatible client key with `make generate-atproto-key` and send stdout directly into the deployment secret manager. Terminal history, logs, screenshots and committed env files are not secret managers.
- Stripe paid ticketing is optional until an Event uses paid pricing; when enabled, set `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET`.
- For event image uploads, configure S3-compatible media storage and expose `MEDIA_PUBLIC_BASE_URL` without auth so attendee/mobile clients can render images.
- Use `/api/health` for process health and `/api/ready` for DB-backed readiness.
- Review `docs/runbooks/database-migrations.md` before changing persisted schema.
- Keep applied migrations immutable. `schema.sql` is version 1; add later gap-free files under `backend/internal/app/migrations/` and use `make migrate` so the checksum ledger and startup compatibility gate are enforced.
- Review `docs/runbooks/deployment-checklist.md` before running outside local development.
- Use `docs/runbooks/subcults-cutover.md` for the original replacement of the legacy service at `subcults.subcult.tv` (now redirected to `os.subcult.tv`).

## AT OAuth revocation worker

Unlink immediately disables local access and queues encrypted credentials for remote revocation. After migrations and client secrets are configured, opt into the separate worker command with `docker compose --profile atproto-workers up -d atproto-revocations`. It uses the same API image and database, polls every 30 seconds and works even while new OAuth links are disabled. It does not apply migrations itself. This profile is off by default and is not live-qualified yet.

Use `docker compose exec api /app/atproto-revoke -status` for secret-free queue counts, or omit `-status` for one bounded batch. Completed credentials are erased; unsupported providers, invalid payloads, eight failed attempts or seven-day retention expiry produce a quarantined record with credentials removed. Quarantine does not prove provider revocation; investigate and revoke in the provider UI if necessary. See [the AT kernel](docs/development/atproto-kernel.md) for retries and recovery.

## AT record projection worker

An allowlisted, restart-safe mirror of the three admitted `tv.subcult.*` collections, read from an external Jetstream-shaped stream. After migrations are applied, opt into it with `docker compose --profile atproto-projection up -d atproto-projection`; it also requires `AT_PROJECTION_ENABLED=true` and `AT_PROJECTION_SOURCE_URL`. It never applies migrations and never writes into the `cultural_*` tables. This profile is off by default and is not live-qualified against a real Jetstream endpoint yet.

Use `docker compose exec api /app/atproto-project` (no flags) for secret-free status (stored cursor, record count, quarantine count); pass `-run` to actually consume the stream. See [projection.md](docs/development/projection.md) for the adapter choice, processor semantics and known limits.

## Paid ticketing local setup

For local paid-ticket testing, copy the optional Stripe env vars from `.env.example`, then run Stripe CLI webhook forwarding:

```bash
stripe listen --forward-to localhost:38080/api/stripe/webhook
```

The paid ticketing runbook is `docs/runbooks/stripe-local-testing.md`.

- `make alpha-qa` stays the default free-ticket lifecycle check.
- `make fake-event-qa` rehearses the organizer setup flow: draft event, test ticket, roles, staffing edits and door check-in.
- `make operations-qa` rehearses the operations panels: workspace invitations and switching, contacts, commitments, staffing assignments, event templates, roles/applications, and reminder-sweep boundaries (role gating, due-state gating, and sweep idempotency).
- All three rehearsal scripts require a loopback development API with mail delivery disabled, a matching disposable PostgreSQL database named `subcult_qa_*`, `QA_DATABASE_URL` pointing to it, and `QA_DISPOSABLE_DATABASE=1`. They also need `psql`, Node, Python and curl. Set `API_URL` explicitly when not using Compose. They read only held verification messages for their synthetic `example.test` accounts, then use the normal verification endpoint to establish sessions. This tests verification-token consumption, not email delivery. Never point them at retained application data; the opt-in acknowledges that they create synthetic accounts and events.
- `make alpha-qa-paid` runs the optional paid API QA when `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET` are set.

Stripe keys are not required unless you configure a paid Event. Use the Stripe test card `4242 4242 4242 4242`, and treat the webhook as the source of truth for fulfillment.

## Open Pilot

This repository keeps the base Open Pilot issue template and PR template.
Bootstrap labels after creating a repo from this template:

```bash
open-pilot labels bootstrap PatrickFanella/subcult-os
```

## License

GPL-3.0-only. See `LICENSE`.

https://www2.onnwee.me
