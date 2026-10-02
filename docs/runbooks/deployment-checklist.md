# Deployment Checklist

Use this checklist before running `subcult-os` outside local development.

## Required verification

Run from the repository root:

```bash
make verify
make build
make compose-config
```

For the local Docker stack, also run:

```bash
make up-build
make smoke
make alpha-qa
```

`make alpha-qa` is a local alpha lifecycle test. It creates throwaway users, a Workspace, an invite, an Event, a Ticket reservation, a Door Check-In, and an End of Night report.

## Required environment

Production must set:

- `APP_ENV=production`
- `DATABASE_URL` with the production Postgres connection string
- `SESSION_SECRET` as a non-default random value with at least 24 characters
- `PUBLIC_WEB_URL` as the HTTPS browser origin. It is also a web image build argument for the link preview address, so rebuild the web image when it changes
- `API_ADDR` for the bind address, usually `:8080` inside a container
- `IDENTITY_PROTECTION_KEY` as base64 for exactly 32 random bytes. See
  [key rotation](key-rotation.md) for ownership, backup expectations, and the
  rotation/re-encryption procedure, including the optional
  `IDENTITY_PROTECTION_KEY_PREVIOUS` transition variable.

When AT OAuth is enabled, production must also set:

- `ATPROTO_OAUTH_ENABLED=true`
- `ATPROTO_OAUTH_CLIENT_ID`, `ATPROTO_OAUTH_CALLBACK_URL`, and `ATPROTO_OAUTH_JWKS_URL` to same-origin HTTPS URLs
- `ATPROTO_OAUTH_CLIENT_PRIVATE_KEY` to a secret-store-backed multibase P-256 key
- `ATPROTO_OAUTH_CLIENT_KEY_ID` to the public key identifier

See [key rotation](key-rotation.md) for the signing-key rotation procedure,
including the optional `ATPROTO_OAUTH_CLIENT_PRIVATE_KEY_PREVIOUS` /
`ATPROTO_OAUTH_CLIENT_KEY_ID_PREVIOUS` transition variables.

Do not enable AT OAuth until bounded live start/callback, refresh, provider-revocation and replay-negative journeys have passed. Passing local handlers, browser UI and metadata/JWKS checks alone is not a qualified login flow.

Do not commit real secrets to `.env`, `.env.example`, docs, or compose files.

## Health checks

- `/api/health`: process is serving HTTP.
- `/api/ready`: API can reach the database.

Use `/api/ready` for load balancer or orchestrator readiness checks when available.

## Notification privacy and delivery

- Notification activity is operator-only; public discovery and public event pages never expose notification data.
- `email_outbox` rows contain recipient email addresses and email bodies, so treat outbox content as private workspace data.
- Contacts/commitments may contain sensitive free text; review logs, notification templates, and public routes before production launch.
- Review template private notes and copied event fields before production; templates must not copy applications, staffing notes, contacts, commitments, settlement, or archive notes.
- Reminder sweeps use email outbox rows and may contain recipient emails; review reminder copy, idempotency keys, and future scheduler credentials before production.
- Before enabling production delivery, review the mail provider for privacy, transport security, retention, and deliverability behavior.

## Database safety

Read `docs/runbooks/database-migrations.md` before changing `backend/internal/app/schema.sql`.

Before any non-additive schema change:

1. take a database backup,
2. test the migration against a copy of production-like data,
3. define rollback or forward-fix steps,
4. keep the previous deploy artifact available.

## Rollback notes

The current alpha uses a single Go API and static web build. Rollback means redeploying the previous API/web image or artifact. If schema changed, rollback may require restoring a database backup unless the change was additive and backward-compatible.

Production serves `https://os.subcult.tv`; `subcults.subcult.tv` redirects pages there. The original replacement of the legacy app followed the [cutover runbook](subcults-cutover.md). Do not stop or overwrite the legacy service as a staging mechanism.
