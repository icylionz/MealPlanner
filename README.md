# Backbone Plate (MealPlanner)

Meal planner for shared households, styled with the Backbone Tech design
system. Go SSR monolith: Echo · templ · PostgreSQL · sqlc · golang-migrate,
Tailwind built ahead of time, HTMX vendored, Node-free runtime.

See `docs/architecture.md` (implementation source of truth) and `docs/doc.md`
(PRD). Prototype artifacts belong in `docs/prototype/`, but no prototype files
are currently checked into that path.

## Screens

- **Login / Register** — email + password accounts backed by server-side,
  database-backed sessions (bcrypt hashes, `SameSite=Lax` session cookie)
- **Onboarding** — create a new household or join one by invite code
- **Today** — agenda for today with next-meal highlight
- **Plan** — mini calendar + week list, day list or week grid layout
- **Foods** — food catalog with search, tag filter; atomic ingredients and
  nested component recipes live in one library
- **Food detail** — ×1–×4 scaling, expandable component tree, unit conversion
- **New Food / editor** — every component is a search-and-select of an existing
  food; there is no free-text ingredient entry
- **Import** — URL/JSON/XML/MMF parse, then a reconcile screen maps each parsed
  ingredient line to a food (or creates one) before saving
- **Grocery** — lists, check-off, unit conversion, generation from planned
  meals / recipes / date ranges with ingredient aggregation
- **Prep** — prep sessions with aggregate ingredients and printable view
- **Household** — multi-tenant: each account belongs to one or more households,
  each owning its own foods/plans/grocery/prep; owners add members by email or
  regenerate the shareable invite code, and members switch the active household
- **Settings** — account self-service: change display name/email (email-unique)
  and password (verifies the current one, 8-char minimum)

## Development

Requirements: Go 1.26+, `templ`, `sqlc`, PostgreSQL, and the standalone Tailwind
CLI at `bin/tailwindcss`. `DATABASE_URL` is required when running the server
directly. Compose supplies `postgres://plate:plate@db:5432/plate?sslmode=disable`
to the app; its database service listens on container port `5432` and is not
published to a host port.

```sh
templ generate                                   # regenerate views
sqlc generate                                    # regenerate query code
./bin/tailwindcss -c tailwind.config.js \
  -i web/static/css/input.css -o web/static/css/app.css --minify
go build -o bin/server ./cmd/server
./bin/server                                     # migrates + seeds on boot
```

The data model is unified around a single `foods` table: every ingredient is a
food that is either *atomic* (no components) or a *recipe* (one or more
components, each referencing another food). The `0001`/`0002` migrations define
and seed this model with a small demo catalog (a handful of ingredients plus
example recipes).

To stock the catalog with real raw ingredients, run the CoFID seeder. It fetches
the live UK *Composition of Foods Integrated Dataset* (McCance and Widdowson,
Public Health England) and loads its raw-ingredient set (~790 whole foods —
vegetables, fruit, grains, meat, fish, dairy, eggs, nuts, fats, herbs/spices and
plain sweeteners; drinks, alcohol, snacks/confectionery and condiments are
excluded) into the seeded "Starter Template" household, so every household
created afterwards clones the enriched catalog. It is idempotent (foods already present by name are
skipped) and keeps the large ingredient list out of the repo — nothing is
committed but the ~200-line fetch/parse program in `internal/cofid`.

```sh
go run ./cmd/seed-foods                 # fetch live gov.uk dataset, seed template
go run ./cmd/seed-foods --file cofid.xlsx   # seed from a local workbook copy
go run ./cmd/seed-foods --dry-run           # report counts, write nothing
``` If you are upgrading a database created before the unified
model, wipe it first (the migrations were rewritten in place, so they will not
re-run on an already-migrated DB):

```sh
# drop + recreate the schema, then re-run the server to migrate + seed
psql "$DATABASE_URL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
```

Configuration comes from `.env` (see `internal/config`): `PORT`, `BASE_PATH`,
`DATABASE_URL`, `SESSION_COOKIE_NAME`, `SESSION_SECRET`, `APP_ENV`,
`LOGIN_THROTTLE_THRESHOLD`, `LOGIN_THROTTLE_WINDOW`,
`LOGIN_THROTTLE_BLOCK_DURATION`, and `TRUSTED_PROXY_CIDRS`. The last setting is
a comma-separated list of proxy CIDRs allowed to supply `X-Forwarded-For`; when
empty, client identity always comes from the direct socket peer.

## Deployment

```sh
podman compose up --build
```

One app container + one PostgreSQL container; reverse proxy/TLS is assumed to
sit outside the stack. `BASE_PATH` mounts the whole app under a sub-path.
