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
- **Foods** — recipe library with search, tag filter, nested component recipes
- **Recipe detail** — ×1–×4 scaling, expandable sub-recipe tree, unit conversion
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

Configuration comes from `.env` (see `internal/config`): `PORT`, `BASE_PATH`,
`DATABASE_URL`, `SESSION_COOKIE_NAME`, `SESSION_SECRET`, `APP_ENV`.

## Deployment

```sh
podman compose up --build
```

One app container + one PostgreSQL container; reverse proxy/TLS is assumed to
sit outside the stack. `BASE_PATH` mounts the whole app under a sub-path.
