# Agent Instructions

This repository is `subcult-os`, a Go full-stack project with Vite React TypeScript, Postgres, and Docker, bootstrapped from `subculture-collective/project-template`.

## Priorities

1. Keep the project runnable and minimal.
2. Preserve Open Pilot issue and PR templates.
3. Keep generated caches, secrets, and local data out of git.
4. Update this file and `README.md` when stack conventions change.

Use the Subcults terminal design system in `docs/design-system.md`.
Edit `contracts/design/tokens.json`, then run `node scripts/design-tokens.mjs`;
do not edit generated client tokens independently. Prefer semantic colors and
shared controls. Keep event artwork and scanner surfaces immersive.
Brand assets in `web/public/` and the app icons in `mobile/assets/` come from the
Subcult OS pack in `subcult-tv` (`studio/branding/library/brands/subcult-os`).
Change the mark in Studio, then re-export with `python3 scripts/brand-assets.py
--studio-root <subcult-tv checkout>`; do not edit the exported files or
`docs/brand-provenance.json` by hand.

Private stage plots use a separate `MEDIA_S3_PRIVATE_BUCKET` whose policy denies
anonymous reads and public ACLs. Never reuse the public artwork bucket or return
object-store URLs. Keep tech sheets out of public DTOs, publication and projection;
see `docs/development/tech-sheets.md`. Removing an act retains its sheet until
confirmed deletion. Template/archive reuse does not copy sheets implicitly.

The optional `atproto-workers` Compose profile runs the revocation command from the same API image against the shared database. Keep it opt-in, run migrations before processing, and preserve secret-free status/error output. It may drain existing revocations even when new OAuth links are disabled.

The optional `mail-workers` profile runs `email-deliver -send -watch`. Sending additionally requires `MAIL_DELIVERY_ENABLED=true`, configured provider credentials and approved sender settings. Old/development outbox rows remain held. The default `email-deliver` command prints aggregate status only; never send live messages during automated qualification.

Resend feedback requires `RESEND_WEBHOOK_SECRET`. Brevo feedback requires bearer authentication with `BREVO_WEBHOOK_TOKEN`; enabled sending requires the selected provider's feedback secret. Select Brevo with `MAIL_PROVIDER=brevo` and configure `BREVO_API_KEY`. Keep signature checks on raw bytes and recipient suppression independent of webhook-supplied addresses. Feedback may continue while sending is disabled. Never roll back to a worker without suppression enforcement while sending remains enabled.

The optional `atproto-projection` profile runs `atproto-project -run`, an allowlisted, restart-safe mirror of the three admitted `tv.subcult.*` collections read from an external Jetstream-shaped stream (see `docs/development/projection.md`). Running it requires `AT_PROJECTION_ENABLED=true` and `AT_PROJECTION_SOURCE_URL`. It only ever writes to `at_projection_*` tables; never point it at the `cultural_*` tables or treat its output as authorized for publication.

The optional `announcement-workers` profile runs `email-deliver -announce -watch` (see `docs/development/announcements.md`, SIGNAL-01). It only claims due scheduled announcements and enqueues their recipient `email_outbox` rows from the existing `consent_grants` audience, minus suppression; it never contacts the mail provider and does not require `MAIL_DELIVERY_ENABLED`. Actual sending of the rows it enqueues still goes through the `mail-workers` profile and remains subject to `MAIL_DELIVERY_ENABLED`. Email remains the only implemented announcement channel; do not add an SMS or other provider without a documented pilot need and cost basis.

The optional `backline-workers` profile runs `backline-handoff -watch`, which delivers approved announcement handoffs to one connected Backline channel (see `docs/development/backline-handoff.md`, #221). It requires `BACKLINE_HANDOFF_URL` and `BACKLINE_HANDOFF_SECRET`; both default to empty and the worker refuses to run without them. Test against the synthetic receiver only; never point automated qualification at a real Backline or Discord channel.

The optional `listmonk-workers` profile runs `listmonk-relay -watch`, which sends announcement occurrences routed to a workspace's one bound listmonk list as per-recipient transactional messages (see `docs/development/listmonk-relay.md`, #220). It requires `LISTMONK_URL`, `LISTMONK_API_USER` and `LISTMONK_API_TOKEN` (all empty by default) and `MAIL_DELIVERY_ENABLED=true`; without the flag the relay refuses to run and leaves every row untouched. Never send a listmonk campaign to the list, never add anyone to a listmonk list or confirm a subscription from Subcult OS, never treat a Kork receipt alone as consent (a confirmed listmonk membership is the authority), never send one occurrence through both paths, and test against the synthetic `httptest` listmonk only.

## Verification

For isolated T3 development, use `bash scripts/dev-env.sh start` and the printed
preview URL. `seed` creates synthetic demo data through normal signup/verification;
`watch` restarts the backend on edits. `stop` removes the worktree's containers and
network and retains its data volumes.
`down` also deletes the worktree's volumes, including its development database.
`gc` lists development stacks whose checkout no longer exists; `gc --apply`
removes them. It never touches the deployment `subcult-os` project, projects using
`docker-compose.yml` or stacks with running containers. Volume-only stacks with
an unknown owner need `--orphans`; a named disposable stack whose checkout still
exists needs `--project <name>`. Remove any disposable stack you start by hand
(`docker compose -p <name> -f compose.dev.yml ...`) with
`docker compose -p <name> -f compose.dev.yml --profile test down -v --remove-orphans`
once its receipts are saved.
Use `bash scripts/dev-env.sh verify` to run the required verification and a
separate disposable DB gate with pinned toolchains. Never substitute the dev
database for the disposable test database. The repository disposable test DB
has a 2 GiB cap and bounded WAL for the complete migration suite; keep it separate from the
retained development database and from the installed shared T3 recipe.
The standalone `compose.dev.yml`
must not be merged with deployment Compose files or given production credentials.

Run:

```bash
make verify
```

Use narrower commands only when an issue explicitly asks for a smaller check.

CI provisions Node 24, pnpm 10.33.0 and Go 1.26.11 in the job rather than
assuming the runner image includes them. Keep toolchain changes explicit, in both
workflows, and distinguish local `make verify` from the actual hosted result.
Pull requests are verified on the GitHub organization mirror by
`.github/workflows/mirror-ci.yml` (`make verify`, `make test-db` and the web image
check), reported to Gitea as `GitHub CI / Mirror CI gate`. Gitea's
`.gitea/workflows/template-check.yml` verifies main and deploys the web image.
Both run the complete `make test-db` gate against a disposable PostgreSQL
service. Never point that gate at a retained application database.

## Cloned Dependency Source

Read-only dependency source repositories are available under
`.blacktower/clonedeps/repos/` for inspection. Do not edit these clones.

- `.blacktower/clonedeps/repos/bluesky-social__indigo/` — `bluesky-social/indigo` at `41278964ec8e3253e70d4e919dfb8e34211c543d`; use it to inspect the pinned unstable AT Protocol syntax and OAuth implementation.
