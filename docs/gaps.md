# Docs-vs-Reality Gap Backlog

Tracks where the shipped app diverges from `docs/doc.md` (PRD) and
`docs/architecture.md`. Each item is a self-contained work unit: implement the
code **or** amend the doc if the divergence is the intended design. Check the box
when done.

**Every unit must:** read `AGENTS.md` + `docs/architecture.md` first · add
reversible up/down migrations under `internal/platform/database/migrations/` ·
regen (`templ generate` + `sqlc generate`) · rebuild Tailwind if CSS changed ·
`go build ./... && go vet ./...` · verify end-to-end (`/verify` or `/run`) ·
reload podman (`podman compose up -d --build --force-recreate app`). Leave
changes staged; commit only when asked.

Terminology note: the app unified PRD's Recipe+Ingredient into one `foods` table
(atomic food vs recipe-with-components), renamed ScheduledMeal→`meal_plan`,
GrocerySnapshot→`grocery_lists`. Decide per item whether to keep app terms and
update the PRD, or honor the PRD term.

---

## G1. Soft delete (foundation) — do first
PRD 3.1, FR7.3, FR9.4, FR12.6, NFR5.3, §6.3 invariants.
No table has `deleted_at`; deletes are hard `ON DELETE CASCADE`.
- [x] Add `deleted_at timestamptz` to `foods`, `meal_plan`, `grocery_lists`, `grocery_items` (and any entity referenced historically).
- [x] Delete handlers set `deleted_at` instead of `DELETE`.
- [x] All list/detail queries filter `deleted_at IS NULL`, but historical references (past meals, grocery items pointing at a deleted food) still render the name.
- [x] Change FK cascade behavior so a soft-deleted food remains readable from old meals/lists. (No FK change needed: soft delete issues `UPDATE`, never `DELETE`, so the food row persists and old meals join to it. Meal/prep/grocery-gen views resolve names via `ListFoodsWithDeleted`; `CountComponentUses` now ignores soft-deleted parents. Series scope=all soft-deletes occurrences instead of a cascading `DELETE`.)

## G2. Authorship columns
PRD §6 (`created_by_user_id`/`updated_by_user_id` on most entities).
- [ ] Add `created_by`/`updated_by` (account UUID) to `foods`, `meal_plan`, `grocery_lists`, `grocery_items`, `grocery snapshots`.
- [ ] Populate from the request's account in services.
- [ ] Depends on nothing; pairs naturally with G1 in one migration.

## G3. Role enforcement + ownership transfer
PRD FR3 AC1/AC2/AC5. Today only household member add/remove/invite is owner-gated;
members can edit foods/meals/grocery/prep freely.
- [ ] Add owner-only guard to all mutating food/meal/grocery/prep handlers (middleware or per-handler `member.Role != "owner"` → 403).
- [ ] Members keep grocery check/uncheck + ad-hoc add (FR3 AC3).
- [ ] Hide edit controls in templ for members.
- [ ] Add "transfer ownership" flow: promote a member to owner, demote self (FR3 AC5).

## G4. Multi-recipe scheduled meals
PRD FR5 AC3–AC5, ScheduledMeal/ScheduledMealRecipe entities.
`meal_plan` holds one `food_id`; no notes/title, no per-recipe servings override.
- [ ] Add `scheduled_meal_recipes` join (meal → many foods, `servings_override`, `sort_order`).
- [ ] Add `title`, `notes`, `version` to `meal_plan`.
- [ ] Update add/edit meal UI to attach multiple recipes + per-recipe override.
- [ ] Grocery generation uses override when set, else meal-level servings.

## G5. Grocery item traceability
PRD FR12 AC1–AC2/AC6, GroceryItemSource.
Grocery stays live-editable lists (immutable-snapshot model dropped by decision);
no source table today.
- [ ] Add `grocery_item_sources` (item → meal/recipe/ingredient-line, `quantity_contributed`, `unit_contributed`).
- [ ] Item detail UI shows "why this is here" (contributing meals/recipes).
- [ ] Link `grocery_items.ingredient_id` (food UUID) + `source_type` (generated/adhoc).

## G6. Ingredient aliases, variants, source URL
PRD FR7.4, FR9.2, FR9.3, FR14 AC3.
- [ ] Add `food_aliases` (food → alias); picker search matches canonical + aliases.
- [ ] Store variant/form as an attribute on a component/usage line, not a new food.
- [ ] Add `source_url` + `source_last_imported_at` to `foods`; URL import stores them; support re-import review (FR14 AC4).

## G7. Invite lifecycle
PRD FR2 AC3/AC4, Invite entity.
`households.invite_code` is a single perpetual code.
- [ ] New `invites` table: `code`, `expires_at`, `revoked_at`, `max_uses`, `use_count`, `created_by`.
- [ ] Join checks expiry/revocation/uses; expired or revoked → clear error.
- [ ] Owner can revoke/regenerate; keep the simple share-code UX.

## G8. Login throttling
PRD FR1 AC4, NFR5.2.
- [ ] Throttle repeated failed logins per IP/account (in-memory or DB-backed counter); slow/block past a threshold for a window.

## G9. PWA installability
PRD 3.1, §7.
- [ ] Add `manifest.webmanifest` (name, icons, theme, display standalone) served under BASE_PATH.
- [ ] Add a minimal service worker (offline shell / cache static). Link both from `layout.templ` `<head>`.

## G10. Observability
PRD NFR5.6.
- [ ] Add request-ID middleware; include the ID in structured logs.
- [ ] Emit metrics: login attempts, import failures, snapshot generation duration, conflict rates, invite acceptance (Prometheus endpoint or structured metric logs).

## G11. Response contract — DOC FIX (likely)
`architecture.md` §Request and Response Contract claims HX-fragment + JSON tiers.
App is full-page + `hx-boost` only.
- [ ] Decide: build fragment/JSON tiers, or (recommended) rewrite the section to describe the actual SSR + hx-boost model. If doc-fix, do it and stop.

## G12. Doc reconciliation — DOC FIX
- [ ] Fix prototype paths: `docs/MealPlanner.html` + `docs/src/` → `docs/prototype/` (currently empty — re-add prototype files or note absence). Same in `README.md` (`Backbone Plate.html`).
- [ ] Reconcile PRD §8.1/§8.2 nav + screens to reality: Recipes+Ingredients→Foods, add Today + Prep, Settings sidebar-only.
- [ ] Add app-only features to PRD/README: Prep sessions, CoFID seeder, Settings (account name/email/password).
- [ ] Update PRD §6 data model to the shipped `foods`-unified schema, or mark it "target, superseded by implementation."
- [ ] Mark PRD FR11 immutable grocery snapshots as a deliberate non-goal — app uses live-editable grocery lists by decision. Do not re-add.
- [ ] Decide on `username` (FR1): implement, or relax PRD to email-only (app is email-only).
- [ ] README: reconcile local DB port `5433` vs compose `5432`.

## G13. Optimistic locking on meals
PRD FR16 + ScheduledMeal.version. Only `foods` has versioning today.
- [ ] Add `version` to `meal_plan` (folds into G4) and apply the same conflict screen (mirror FR16 foods impl).
