# Backbone Plate (MealPlanner)

Meal planner for shared households, styled with the Backbone Tech design
system. Go SSR monolith: Echo · templ · PostgreSQL · sqlc · golang-migrate,
Tailwind built ahead of time, HTMX vendored, Node-free runtime.

See `docs/architecture.md` (implementation source of truth) and `docs/doc.md`
(PRD). The UI implements the `Backbone Plate.html` prototype from the
claude.ai/design MealPlanner project.

## Screens

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
- **Household** — member profiles with active-profile switching

## Development

Requirements: Go 1.26+, `templ`, `sqlc`, PostgreSQL (defaults expect
`postgres://plate:plate@localhost:5433/plate`), the standalone Tailwind CLI at
`bin/tailwindcss`.

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
and seed this model. If you are upgrading a database created before the unified
model, wipe it first (the migrations were rewritten in place, so they will not
re-run on an already-migrated DB):

```sh
# drop + recreate the schema, then re-run the server to migrate + seed
psql "$DATABASE_URL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
```

Configuration comes from `.env` (see `internal/config`): `PORT`, `BASE_PATH`,
`DATABASE_URL`, `SESSION_COOKIE_NAME`, `SESSION_SECRET`, `APP_ENV`.

## Deployment

```sh
podman compose up --build
```

One app container + one PostgreSQL container; reverse proxy/TLS is assumed to
sit outside the stack. `BASE_PATH` mounts the whole app under a sub-path.
