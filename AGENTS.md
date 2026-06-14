# Agent Instructions

Read these before making product, architecture, or UI decisions:

1. `docs/doc.md` is the product requirements document.
2. `docs/architecture.md` is the implementation architecture source of truth.

## UI Rules

- The prototype is the required implementation target, not a suggestion.
- Stick strictly to the prototype's design system, layout, information hierarchy, spacing, components, and interaction patterns.
- Do not redesign, simplify, restyle, or "improve" the UI without explicit user approval.
- If a deviation appears necessary, stop first and ask for permission.
- Any proposed deviation must include:
  - the reason it is needed,
  - the impact on fidelity or behavior,
  - the implementation plan for the change.

## Architecture Rules

- Follow the stack and request contract documented in `docs/architecture.md`.
- Keep the app SSR-first and multipage-first. HTMX enhances flows but must not be required for core usage.
- Keep handlers thin, business logic in services, and data access behind repository or query boundaries.
