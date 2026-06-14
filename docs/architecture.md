# MealPlanner Architecture

## Summary

MealPlanner is implemented as a single Go monolith using Echo, templ, PostgreSQL, sqlc, and golang-migrate. The application is SSR-first and multipage-first, with HTMX used to reduce full page reloads by swapping only the relevant HTML fragments when JavaScript is available. The application must remain fully usable when JavaScript is disabled.

The current prototype in `docs/MealPlanner.html` and `docs/src/` is the authoritative UI target. It is not a loose reference. Implementation work must match the prototype and its design system unless the user explicitly approves a deviation first.

## Technology Decisions

- HTTP server and routing: Echo
- HTML rendering: templ
- Database: PostgreSQL
- Query generation: sqlc
- Schema migrations: golang-migrate
- Configuration: `.env`-driven environment variables
- Dependency injection: manual constructor-based DI
- Local and deployment orchestration: Docker Compose, run locally with Podman
- CSS: Tailwind generated ahead of time with the standalone CLI
- JavaScript enhancement: HTMX plus small custom scripts where needed

## Request and Response Contract

The same feature routes support full-page HTML, partial HTML, and JSON depending on the request.

1. Direct browser navigation returns a full HTML page.
2. Requests with `HX-Request: true` return only the relevant HTML fragment for the target swap.
3. Requests with `Accept: application/json` return JSON and take precedence over HTMX fragment responses.

The app must support standard browser links and form submissions first. HTMX is an enhancement layer and must not become a hard dependency for core flows.

## Application Structure

Use a single Go binary with clear internal boundaries.

- `cmd/...`
  - application entrypoint and dependency wiring
- `internal/config`
  - env loading, normalization, validation
- `internal/platform`
  - shared infrastructure such as database, sessions, renderer, jobs, logging, and time helpers
- `internal/http`
  - Echo router, middleware, handlers, request parsing, content negotiation
- `internal/auth`
  - accounts, login, logout, session ownership, authorization checks
- `internal/households`
  - household membership, roles, invite flows
- `internal/recipes`
  - recipes, recipe components, imports, tags
- `internal/ingredients`
  - ingredient catalog, aliases, unit conversions, density rules
- `internal/planner`
  - scheduled meals, recurrence, agenda behavior
- `internal/grocery`
  - snapshot generation, contributor traceability, snapshot edits
- `internal/imports`
  - import and export workflows, import review, re-import support

Exact package names can change, but the architectural boundary must remain: handlers stay thin, services own business logic, and data access stays below the service layer.

## Dependency Injection

Use manual constructor-based dependency injection only.

- Wire dependencies in the application entrypoint.
- Pass concrete dependencies through constructors.
- Depend on interfaces only where a boundary actually benefits from substitution, such as sessions, jobs, rendering, or repositories.
- Do not introduce runtime DI containers.

## Data Access

PostgreSQL is the source of truth for all persistent state.

- sqlc-generated queries are used for database access.
- sqlc usage should stay inside repository or query-layer packages, not handlers.
- Multi-step domain operations should execute inside service-owned transaction boundaries.
- SQL migrations are the source of truth for schema evolution and must stay in sync with sqlc query definitions.

## Sessions and Authentication

Authentication uses server-side, database-backed sessions.

- The cookie stores only a session identifier.
- Session records live in PostgreSQL.
- Role and household authorization is enforced on the server for every protected request.
- This architecture is preferred over JWT for the current SSR-first, web-first application shape.

## Jobs and Slow Work

Slow or asynchronous work runs through an in-process job runner backed by PostgreSQL.

- The initial deployment remains a single app process plus PostgreSQL.
- A `jobs` table should hold queued work, attempts, status, and scheduling metadata.
- Work claiming should use database locking patterns appropriate for concurrent workers, such as `FOR UPDATE SKIP LOCKED`.
- Initial job candidates include recipe URL import processing and rich link preview refresh.

If scale later justifies it, the same job runner can be moved into a separate worker process without redesigning the domain layer.

## Configuration

Configuration is environment-driven and loaded from `.env` locally.

At minimum, support:

- `PORT`
- `BASE_PATH`
- `DATABASE_URL`
- `SESSION_COOKIE_NAME`
- `SESSION_SECRET`
- `APP_ENV`

### Base Path Rules

`BASE_PATH` is a first-class deployment setting.

- All routes must mount beneath it.
- Generated links, redirects, form actions, HTMX endpoints, and asset URLs must honor it.
- An empty `BASE_PATH` means root deployment.

Host and scheme should be inferred from the incoming request and trusted forwarded headers. A separate public URL setting is not required by default.

## Delivery and Deployment

The initial deployment topology is:

- one application container
- one PostgreSQL container

Use Docker Compose for local orchestration and for deployment packaging, with Podman used locally to run the stack.

Assume any reverse proxy or TLS termination sits outside this compose stack.

## Frontend Asset Strategy

The runtime stays Node-free.

- Serve static assets from the Go application.
- Vendor or locally serve HTMX rather than loading it from a runtime CDN dependency.
- Generate Tailwind CSS ahead of time with the standalone Tailwind CLI.
- Keep custom JavaScript small and focused on progressive enhancement needs.

Do not use the Tailwind CDN script in production. The generated CSS must be part of the build output so styling remains reproducible and compatible with containerized deployment.

## UI Source of Truth

The prototype is authoritative.

- `docs/MealPlanner.html` and `docs/src/` define the required UI.
- The prototype's design system must be followed strictly.
- Layout, hierarchy, screens, states, spacing, controls, and interaction intent must match the prototype.
- templ, Echo, HTMX, and Tailwind are implementation tools only. They do not authorize redesign.

### Deviations

No visual or structural deviation is allowed without explicit user approval first.

If a deviation appears necessary, stop and present:

1. the reason for the deviation,
2. the expected impact on fidelity or behavior,
3. the implementation plan for how the change will be made.

Possible reasons may include accessibility, SSR or HTMX runtime constraints, or required responsive behavior, but they still require approval before implementation.

## Implementation Guardrails

- Build SSR-first pages first, then add HTMX enhancement where it improves UX.
- Prefer full-page routes plus fragment templates rather than separate feature implementations for SSR and HTMX.
- Keep feature logic in services so HTML, HTMX fragments, and JSON can share the same use-case layer.
- Keep the app fully functional without JavaScript.
- Treat the prototype as the acceptance target for UI fidelity.
