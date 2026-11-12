# Meals

A mobile-first meal planner for shared households: plan meals, keep one food
catalog of recipes and ingredients, build live grocery lists from the plan,
organise prep sessions, and move data in and out as JSON. It is the Meals app
of the **Household** design system and ships as a server-rendered PWA.

- Product requirements: [`docs/doc.md`](docs/doc.md)
- Architecture: [`docs/architecture.md`](docs/architecture.md)

## Stack

Go · Echo · templ · PostgreSQL · sqlc · golang-migrate · Tailwind (standalone
CLI) · HTMX (`hx-boost` only; every flow works without JavaScript).

## Layout

| Path | What |
| --- | --- |
| `cmd/server` | The web app. Wires dependencies, runs migrations on boot. |
| `cmd/seed-foods` | Optional: adds CoFID 2021 raw ingredients to the Starter Template. |
| `internal/web` | Routing, middleware (sessions, CSRF, roles), thin handlers. |
| `internal/core` | Business logic and transactions. The only caller of the queries. |
| `internal/units` | Unit conversion, including volume to weight by density. |
| `internal/db` | Pool, migrations (`migrations/`), generated queries (`sqlc/`, not committed). |
| `db/queries` | SQL for sqlc. |
| `internal/ui` | templ views and Phosphor icons. |
| `internal/assets` | Design-system CSS, fonts and static files, embedded in the binary. |

## Design system

The UI is built from the Household design system, vendored under
`internal/assets/css/`:

- `tokens.css` and `components.css` are copied verbatim from the system
  (colour, type, spacing, radius tokens and the `hh-*` components).
- `dark.css` applies the system's dark values when the device prefers dark,
  unless light or dark is pinned (`data-theme` on `<html>`).
- `app.css` is this app's layout (`mp-*`), written only with those tokens.

Form inputs, selects and the segmented control are not components in the
design system yet; `app.css` builds them from its tokens and usage rules.
Picking a theme, seasonal suggestions and the Softness dial belong to the
Household gateway app; this app only applies the theme the gateway hands it.

### Household theme

On each full page load the server asks the Household gateway for the signed-in
user's theme and applies it; if `HOUSEHOLD_THEME_URL` is unset, the gateway is
unreachable, or it answers with anything other than a valid theme, the built-in
"Oat and olive" theme is used. Results are cached for 5 minutes per user
(failures retried after 1 minute, keeping the last good theme), and a stale
theme is refreshed in the background.

- `HOUSEHOLD_THEME_URL`: http(s) URL. Called as `GET <url>?user=<sign-in email>`
  with a 1.5 s timeout.
- Response (200, JSON, up to 64 KB). Every key is optional, but it must set
  something:

  ```json
  {
    "mode": "system",
    "light": { "primary": "#2563EB", "surface-ground": "#F3F4F6" },
    "dark": { "primary": "#5B93F7" }
  }
  ```

  `mode` is `system`, `light` or `dark`; a light/dark pick in Settings →
  Appearance on the device wins. `light`/`dark` map Household token names (the
  list is `core.HouseholdThemeTokens`) to `#rrggbb` values; tokens left out keep
  the built-in value, unknown names are ignored, and a bad value or mode rejects
  the whole response.

## Run it

```sh
cp .env.example .env      # then set DATABASE_URL and SESSION_SECRET
make run                  # generates CSS, templ and sqlc code, builds, runs
```

Or the whole stack in containers: `make up` (Podman Compose, app on `:8080`).

`make check` regenerates, vets and tests. Generated files (`*_templ.go`,
`internal/db/sqlc/`, `internal/assets/static/app.css`) are not committed.

Seed the Starter Template once, before households are created:

```sh
./bin/seed-foods            # or: -file cofid.xlsx, -dry-run
```

## Configuration

| Variable | Purpose |
| --- | --- |
| `PORT` | Listen port (default 8080). |
| `BASE_PATH` | Mount path such as `/meals`; empty for root. |
| `DATABASE_URL` | PostgreSQL connection string. Required. |
| `SESSION_COOKIE_NAME` | Session cookie name; CSRF and flash cookies derive from it. |
| `SESSION_SECRET` | Signs CSRF tokens. Required in production. |
| `APP_ENV` | `production` turns on Secure cookies. |
| `APP_TIMEZONE` | Decides what "today" and "up next" mean (default UTC). |
| `LOGIN_THROTTLE_THRESHOLD`, `LOGIN_THROTTLE_WINDOW`, `LOGIN_THROTTLE_BLOCK_DURATION` | Failed-login throttle per IP and per email. |
| `TRUSTED_PROXY_CIDRS` | Proxies allowed to supply `X-Forwarded-For`. |
| `HOUSEHOLD_THEME_URL` | Household gateway theme endpoint; empty keeps the built-in theme. |

## Operations

- `GET /healthz` answers `ok`.
- `GET /metrics` exposes counters: login attempts, import failures, grocery
  generation duration, edit conflicts and invite acceptances.
- Logs are structured JSON with a request id per request.
- Migrations: `0001_init` is the schema in `docs/doc.md` section 6;
  `0002_member_colour` stores each member's design-system colour index.
  Both have down migrations.
