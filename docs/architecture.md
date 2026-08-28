# MealPlanner Architecture

## Summary

MealPlanner is implemented as a single Go monolith using Echo, templ, PostgreSQL, sqlc, and golang-migrate. The application is SSR-first and multipage-first. Every page request renders a complete HTML document; global HTMX `hx-boost` progressively enhances ordinary links and forms by replacing the page body when JavaScript is available. The application must remain fully usable when JavaScript is disabled.

Prototype artifacts belong under `docs/prototype/`. No prototype artifact is currently checked into that path, so the repository does not presently contain an inspectable prototype source of truth. A restored prototype remains the authoritative UI target and must be followed unless the user explicitly approves a deviation first.

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

HTML page routes use one response model:

1. GET routes render a complete HTML document through the shared templ layout.
2. Successful state-changing form submissions generally use Post/Redirect/Get and return `303 See Other` to an app-local page route. Editor sub-actions that must show refreshed in-form state may render the complete page directly.
3. The shared `<body hx-boost="true">` lets HTMX intercept eligible links and forms. Boosted requests receive the same complete HTML document as direct requests; HTMX extracts and replaces the body and maintains browser history.
4. Handlers do not branch on `HX-Request` and do not provide a feature-route JSON representation based on `Accept`.

Purpose-specific static asset and file download routes, such as data export, may
return their declared non-HTML representation; they are not alternate
representations of page routes.

The app must support standard browser links and form submissions first. HTMX is an optional transport enhancement, not a separate fragment API and not a dependency for core flows.

## Application Structure

Use a single Go binary with clear internal boundaries: the entrypoint under
`cmd/` wires dependencies, HTTP handlers stay thin, services own business logic,
and data access stays below the service layer. Package layout can evolve freely
as long as that boundary holds.

## Dependency Injection

Use manual constructor-based dependency injection only.

- Wire dependencies in the application entrypoint.
- Pass concrete dependencies through constructors.
- Depend on interfaces only where a boundary actually benefits from substitution, such as sessions, rendering, or repositories.
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

## Configuration

Configuration is environment-driven and loaded from `.env` locally.

At minimum, support:

- `PORT`
- `BASE_PATH`
- `DATABASE_URL`
- `SESSION_COOKIE_NAME`
- `SESSION_SECRET`
- `APP_ENV`
- `TRUSTED_PROXY_CIDRS` (optional, comma-separated CIDRs)

### Base Path Rules

`BASE_PATH` is a first-class deployment setting.

- All routes must mount beneath it.
- Generated links, redirects, form actions, HTMX endpoints, and asset URLs must honor it.
- An empty `BASE_PATH` means root deployment.

Host and scheme should be inferred from the incoming request. Forwarded client
addresses are honored only when the direct peer is covered by
`TRUSTED_PROXY_CIDRS`; direct-peer extraction is the default. A separate public
URL setting is not required by default.

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

The prototype is authoritative when its artifacts are present.

- `docs/prototype/` is the canonical location for prototype artifacts.
- That path currently contains no checked-in prototype files. Do not claim fidelity to, or infer missing details from, an unavailable artifact.
- When prototype files are restored, they define the required UI.
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
- Keep one full-page HTML implementation for direct and `hx-boost` requests; do not add fragment or JSON response tiers without an explicit architecture change.
- Keep feature logic in services and out of handlers.
- Keep the app fully functional without JavaScript.
- Treat a restored prototype under `docs/prototype/` as the acceptance target for UI fidelity.
