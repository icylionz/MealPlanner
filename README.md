# MealPlanner (Backbone Plate)

Mobile-first household meal planner: schedule meals, keep a unified foods
catalog (ingredients and nested recipes), generate live grocery lists, plan prep
sessions, and move data in and out as JSON. Requirements live in
`docs/doc.md`; architecture in `docs/architecture.md`.

Go monolith: Echo, templ, PostgreSQL, sqlc, golang-migrate, HTMX (`hx-boost`
only — every flow works without JavaScript), Tailwind built ahead of time. The
UI reuses the Backbone design system shell from RHWallet (`shell.css`, fonts,
Lucide icons) plus `mp.css` for planner-specific pieces.

## Layout

```
cmd/server            web app entrypoint (manual DI, migrations on boot)
cmd/seed-foods        optional CoFID 2021 seeder for the Starter Template
internal/config       .env-driven settings
internal/platform/database  pool, embedded migrations, transactions
sql/queries           sqlc query sources → internal/repo/db (generated)
internal/service      business logic: auth, households, foods catalog/graph,
                      planner, grocery, prep, URL import, JSON transfer
internal/units        unit normalization and volume↔weight conversion
internal/http         thin Echo handlers, middleware (CSRF, sessions, logs)
internal/view         templ pages (generated *_templ.go is not committed)
internal/assets       static files; app.css is built by build-css.sh
```

## Develop

Requires Go 1.26, `templ` v0.3.1020, `sqlc` v1.31, and Postgres.

```sh
cp .env.example .env        # then edit DATABASE_URL / SESSION_SECRET
make gen                    # Tailwind CSS + templ + sqlc
make run                    # http://localhost:8080
make check                  # gen + vet + test
```

Generated code (templ, sqlc, `app.css`) is gitignored and rebuilt by `make gen`
and the Dockerfile.

## Run with Compose (Podman)

```sh
SESSION_SECRET=$(openssl rand -hex 32) podman compose up -d --build
```

Set `BASE_PATH=/mealplanner` to mount under a sub-path; links, redirects,
assets, cookies and the PWA manifest all honour it. Set `TRUSTED_PROXY_CIDRS`
when running behind a reverse proxy so client IPs (for login throttling) come
from `X-Forwarded-For`.

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `PORT` | `8080` | |
| `BASE_PATH` | empty | e.g. `/mealplanner` |
| `DATABASE_URL` | — | required |
| `SESSION_COOKIE_NAME` | `mealplanner_session` | |
| `SESSION_SECRET` | dev value | required when `APP_ENV=production` |
| `APP_ENV` | `development` | `production` marks cookies `Secure` |
| `APP_TIMEZONE` | `UTC` | decides what "today" is |
| `TRUSTED_PROXY_CIDRS` | empty | comma-separated CIDRs |
| `LOGIN_THROTTLE_THRESHOLD` / `_WINDOW` / `_BLOCK_DURATION` | `5` / `10m` / `15m` | per IP and per email |

## Operations

- `GET /healthz` — database ping. `GET /metrics` — Prometheus text: login
  attempts, import failures, grocery generation duration/failures, conflicts,
  invite acceptances.
- Logs are JSON with a `request_id` (also returned as `X-Request-ID`).
- Migrations run on boot and are reversible (`*.down.sql`).
- `seed-foods [-file cofid.xlsx] [-dry-run]` adds CoFID raw ingredients to the
  Starter Template; households created afterwards get them.

## Behaviour notes

- New households clone the Starter Template catalog (40 staples with densities).
- Volume↔weight conversions use a food's density; without one the grocery line
  is kept separate and flagged "Needs review" rather than guessed.
- Grocery generation merges into unchecked, unpriced lines of the same food and
  variant; every contribution is recorded for "why is this here".
- Food cost estimates use the latest recorded grocery cost per food and roll up
  through nested recipes with an "N of M ingredients priced" coverage note.
- JSON import merge rules: an ID already in this household is kept (reported as
  existing); an ID owned by another household is skipped and reported; records
  referencing missing foods are skipped.
