# Subcult OS

![Subcult OS: a tilted purple ticket stub reads Admit one, From the plan to the last ticket, beside a numbered run of plan, coordinate, run door, settle and archive.](docs/assets/readme/banner.png)

**19:00 doors, and the whole team on one sheet.**

Subcult OS is software for the people who run small events: independent venues,
collectives and crews. The plan, the staffing, the guest list, the door and the
end-of-night report live in one workspace the whole team reads. It is built
around free ticket reservations. It is in alpha development and has not been
used to run a real event yet.

Public event discovery: <https://os.subcult.tv/>. Product page:
<https://subcult.tv/products/subcult-os>.

## Features

**Plan**

- Workspaces with invited members, roles and a switcher for people who belong to
  more than one.
- Event drafts, reusable event templates and a public event page per event.
- Private availability polls on draft events, with each option shown in the
  viewer's time zone and the organizer's.
- A public discovery page (`/discover`) that lists published events and nothing
  from the private workspace.

**Coordinate**

- Staffing roles, applications and assignments. Eligible participants can claim
  and release capped shifts, and export their assigned shifts as an iCalendar
  file.
- A participant portal (`/participant`) with a person's own schedule and
  commitments.
- Private contacts, commitments and reminder sweeps.
- A run of show, and private act tech sheets combined into one printable input
  list.
- Private venue and event access worksheets that record observations, unknowns
  and review dates without publishing them.
- Email announcements to people who opted in. Approved announcements can also be
  handed to a connected Backline channel or a listmonk list (both optional).

**Door and close-out**

- Free ticket reservations with a ticket link and confirmation.
- Door check-in by name lookup or ticket code, and time-limited volunteer door
  links that admit guests without an account.
- End of Night close-out with a private event report and a settlement export
  (CSV, Markdown or printable HTML).

**Phone app**

The Expo app in `mobile/` carries the event-time tools: scanner, door search,
run of show and a live dashboard, plus attendee discovery, reservations and a
ticket wallet. It runs in Expo Go for development. It is not in an app store.

**Optional and not yet tested against live providers**

- Paid tickets through Stripe.
- A link to an outside ticket seller per event, limited to hostnames you allow.
- Event image uploads to S3-compatible storage.
- Transactional email through Resend or Brevo. Delivery is off by default.
- Linking an AT Protocol (Bluesky network) account. Off by default.

## Quick start

You need Docker with Compose, Go, Node 24 and pnpm.

```bash
cp .env.example .env    # sets PUBLIC_WEB_URL so email and ticket links point at the local web app
make up-build           # build and start Postgres, API and web
make urls               # print the local URLs
make smoke              # check API and web health
```

By default the web app is on <http://localhost:38079> and the API health check
on <http://localhost:38080/api/health>. Change `WEB_PORT`, `API_PORT` and
`POSTGRES_PORT` if those ports are taken.

Run the phone app against the local API:

```bash
pnpm --dir mobile install
EXPO_PUBLIC_API_URL=http://<your-lan-ip>:38080 pnpm --dir mobile run start
```

Tests and checks:

```bash
make help       # list every target
make quick      # format, lint and unit tests
make verify     # everything CI runs, except the database suite
make test-db    # database tests; needs TEST_DATABASE_URL for a throwaway database
make alpha-qa   # end-to-end free-ticket run against a disposable local stack
```

`make down` stops the stack. `make reset-db` deletes the local database and
starts again.

[DEVELOPMENT.md](DEVELOPMENT.md) has the longer guide: the event lifecycle,
mobile setup, paid-ticket testing with the Stripe CLI and the rehearsal scripts.

## Configuration

Settings are environment variables. `.env.example` lists them all with comments.
The main ones:

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | PostgreSQL connection string. Compose builds it from `POSTGRES_DB`, `POSTGRES_USER` and `POSTGRES_PASSWORD`. |
| `SESSION_SECRET` | Session signing secret. Production needs a non-default value of at least 24 characters. |
| `IDENTITY_PROTECTION_KEY` | Base64 32-byte key that encrypts stored identity data. Required in production. |
| `PUBLIC_WEB_URL` | The public HTTPS origin, used for links, the sitemap and link previews. |
| `APP_ENV`, `API_ADDR` | Environment name and API listen address. |
| `MAIL_DELIVERY_ENABLED`, `MAIL_PROVIDER`, `MAIL_FROM`, `MAIL_REPLY_TO` | Transactional email. Provider keys: `RESEND_API_KEY`, `RESEND_WEBHOOK_SECRET` or `BREVO_API_KEY`, `BREVO_WEBHOOK_TOKEN`. |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Paid tickets. Only needed for paid events. |
| `MEDIA_S3_*`, `MEDIA_PUBLIC_BASE_URL` | Event image storage. |
| `EXTERNAL_TICKET_ALLOWED_HOSTS` | Comma-separated hostnames allowed for outside ticket links. Empty denies all. |
| `BACKLINE_HANDOFF_URL`, `BACKLINE_HANDOFF_SECRET` | Optional Backline announcement handoff. |
| `LISTMONK_URL`, `LISTMONK_API_USER`, `LISTMONK_API_TOKEN` | Optional listmonk announcement relay. |
| `ATPROTO_OAUTH_*`, `AT_PROJECTION_*` | Optional AT Protocol account linking and record projection. |
| `MOBILE_APPLE_APP_IDS`, `MOBILE_ANDROID_PACKAGE`, `MOBILE_ANDROID_CERT_SHA256` | App links that open verification emails in the phone app. |

Keep secrets in your deployment's secret store, not in `.env` files you commit.

## Deploy

`docker-compose.yml` defines the full stack: `postgres`, `api` (Go) and `web`
(the built Vite app behind nginx). Background workers are Compose profiles you
turn on when you need them:

| Profile | Worker |
| --- | --- |
| `mail-workers` | Sends queued transactional email (also needs `MAIL_DELIVERY_ENABLED=true`) |
| `announcement-workers` | Queues scheduled announcements |
| `backline-workers` | Hands announcements to Backline |
| `listmonk-workers` | Relays announcements to listmonk |
| `atproto-workers`, `atproto-projection` | AT Protocol revocation and projection |

A typical deploy sets the production variables above, then runs
`docker compose up -d --build` with the profiles it needs. `make migrate` applies
database migrations, and the backend and web images can also be built on their
own from `backend/` and `web/`.

## Project layout

```text
backend/     Go API, migrations runner and workers (cmd/*)
web/         Vite + React + TypeScript web app
mobile/      Expo React Native app
contracts/   Shared API contracts and design tokens
migrations/  SQL migrations
scripts/     Dev environment, QA and check scripts
docs/        Development notes, ADRs and the design system
```

Both clients share one design system: edit `contracts/design/tokens.json`, then
run `node scripts/design-tokens.mjs`. See [docs/design-system.md](docs/design-system.md).

## Contributing

- Bugs and feature requests: [GitHub issues](https://github.com/subculture-collective/subcult-os/issues).
- Questions and ideas: [GitHub Discussions](https://github.com/subculture-collective/subcult-os/discussions).
- Code: open a pull request on GitHub against `main`. CI runs `make verify`
  (formatting, lint, contract checks, unit tests and builds), the database test
  suite and a web image build on every pull request. See
  [CONTRIBUTING.md](CONTRIBUTING.md).

Report security issues privately as described in [SECURITY.md](SECURITY.md).

## License

GNU General Public License v3.0 only. See [LICENSE](LICENSE).
