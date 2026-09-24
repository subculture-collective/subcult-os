# Agent Instructions

This repository is `subcult-os`, a Go full-stack project with Vite React TypeScript, Postgres, and Docker, bootstrapped from `subculture-collective/project-template`.

## Priorities

1. Keep the project runnable and minimal.
2. Preserve Open Pilot issue and PR templates.
3. Keep generated caches, secrets, and local data out of git.
4. Update this file and `README.md` when stack conventions change.

The optional `atproto-workers` Compose profile runs the revocation command from the same API image against the shared database. Keep it opt-in, run migrations before processing, and preserve secret-free status/error output. It may drain existing revocations even when new OAuth links are disabled.

The optional `mail-workers` profile runs `email-deliver -send -watch`. Sending additionally requires `MAIL_DELIVERY_ENABLED=true`, configured Resend credentials and approved sender settings. Old/development outbox rows remain held. The default `email-deliver` command prints aggregate status only; never send live messages during automated qualification.

Resend feedback requires `RESEND_WEBHOOK_SECRET`; enabled sending also requires this secret. Keep signature checks on raw bytes and recipient suppression independent of webhook-supplied addresses. Feedback may continue while sending is disabled. Never roll back to a worker without suppression enforcement while sending remains enabled.

The optional `atproto-projection` profile runs `atproto-project -run`, an allowlisted, restart-safe mirror of the three admitted `tv.subcult.*` collections read from an external Jetstream-shaped stream (see `docs/development/projection.md`). Running it requires `AT_PROJECTION_ENABLED=true` and `AT_PROJECTION_SOURCE_URL`. It only ever writes to `at_projection_*` tables; never point it at the `cultural_*` tables or treat its output as authorized for publication.

## Verification

For isolated T3 development, use `bash scripts/dev-env.sh start` and the printed
preview URL. `seed` creates synthetic demo data through normal signup/verification;
`watch` restarts the backend on edits. `stop` retains the worktree's data.
Use `bash scripts/dev-env.sh verify` to run the required verification and a
separate disposable DB gate with pinned toolchains. Never substitute the dev
database for the disposable test database. The standalone `compose.dev.yml`
must not be merged with deployment Compose files or given production credentials.

Run:

```bash
make verify
```

Use narrower commands only when an issue explicitly asks for a smaller check.

Gitea CI provisions Node 24, pnpm 10.33.0 and Go 1.26.6 in the job rather than
assuming the shared runner image includes them. Keep toolchain changes explicit
and distinguish local `make verify` from the actual hosted result.
The workflow runs the complete `make test-db` gate against its disposable
PostgreSQL service. Never point that gate at a retained application database.

## Cloned Dependency Source

Read-only dependency source repositories are available under
`.blacktower/clonedeps/repos/` for inspection. Do not edit these clones.

- `.blacktower/clonedeps/repos/bluesky-social__indigo/` — `bluesky-social/indigo` at `41278964ec8e3253e70d4e919dfb8e34211c543d`; use it to inspect the pinned unstable AT Protocol syntax and OAuth implementation.
