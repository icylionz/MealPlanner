# PRD

## Implementation Authority

- `docs/architecture.md` is the implementation architecture source of truth.
- `docs/MealPlanner.html` and `docs/src/` are the UI source of truth.
- The prototype is not a suggestion. The shipped UI should match it and its design system strictly.
- Any UI deviation requires user approval first, together with the reason, impact, and implementation plan.

## 1. Summary

Build a mobile-first, responsive meal planner for shared households. Users can schedule meals, attach one or more recipes to a scheduled meal, generate grocery list snapshots from scheduled meals over a date range or selected meals/ingredients, import/export recipes and related data as JSON, and manage ingredient conversions including density-based volume↔weight conversion. The MVP is a server-hosted PWA + website built with a Go server-rendered stack.

## 2. Personas & Goals

### 2.1 Owner

- Creates or joins a household
- Manages recipes, ingredients, schedules, grocery list generation, tags, and household membership
- Wants fast weekly planning and reliable grocery generation
- Needs imports, scaling, nested recipe components, and cross-device access

### 2.2 Member

- Joins a household via invite
- Views household plans/recipes/lists
- Helps shop by checking/unchecking grocery items
- Can add ad-hoc grocery items to a grocery snapshot
- Does not edit recipes, schedules, ingredients, or household settings

## 3. Scope

### 3.1 MVP

- Authentication with username or email + password
- Household create/join flow
- Owner/member roles
- Agenda home view with date selector and continuous day-grouped scroll
- Scheduled meals with:
  - date + time
  - multiple recipes
  - per-meal servings
  - optional per-recipe servings override
  - notes
  - optional link with rich preview
  - recurrence
- Recipe library with tags
- Ingredient catalog with canonical ingredients
- Ingredient aliases/synonyms
- Ingredient variants/forms as attributes on usage lines
- Nested component recipes
- Yield/scaling across nested recipes
- Unit conversion:
  - same-dimension conversions
  - volume↔weight via density
  - starter density set + user override
- Grocery list generation by:
  - date range
  - selected meals
  - selected ingredients
- Grocery snapshots:
  - named
  - immutable after generation except snapshot edits
  - regeneration creates a new snapshot
  - pantry/purchased check state per snapshot
  - ad-hoc items
  - traceability to source meals/recipes/ingredients
- JSON import/export with schema version
- URL recipe import with parse/review/fix/re-import
- Optimistic locking
- Soft delete for referenced entities
- PWA installability
- Responsive desktop layout

### 3.2 Phase 2

- Nutrition
- Family preference profiles
- Password reset self-service
- Email verification
- Provider-specific recipe connectors
- Offline editing/sync
- Package-size shopping suggestions
- Global search page
- Advanced recurrence rules
- Role expansion beyond owner/member

## 4. Functional Requirements

### FR1. Account signup and login

Users can sign up with username or email and password, log in, and access their household data across devices.

#### Acceptance Criteria

1. Given a new user on the signup page, when they provide a valid username or email and password, then the system creates an account and proceeds to household create/join.
2. Given an existing user on the login page, when they submit valid credentials, then the system creates a session and redirects them to the agenda view.
3. Given invalid credentials, when the user submits the login form, then the system rejects the login with a generic error message and does not reveal whether the username/email exists.
4. Given repeated failed login attempts from the same IP or account, when the threshold is exceeded, then the system slows or blocks further attempts for a limited period.

### FR2. Household create or join

A user must create a new household or join an existing one via invite during onboarding.

#### Acceptance Criteria

1. Given a newly authenticated user without a household, when they choose "Create household", then the system creates a new household and makes them the owner.
2. Given a newly authenticated user without a household, when they enter a valid invite code or open a valid invite link, then the system joins them to that household as a member.
3. Given an expired or revoked invite, when a user attempts to join, then the system rejects the join and explains the invite is invalid.
4. Given a valid invite, when its configured expiration time passes, then the invite no longer permits joining.

### FR3. Household roles and permissions

Owners can edit household data and manage members; members can view data and interact with grocery snapshots only.

#### Acceptance Criteria

1. Given an owner, when they access recipes, ingredients, schedules, or settings, then edit controls are available.
2. Given a member, when they access recipes, ingredients, schedules, or settings, then edit controls are not available.
3. Given a member viewing a grocery snapshot, when they check/uncheck an item or add an ad-hoc item, then the system saves the change.
4. Given an owner, when they remove a member, then that member immediately loses household access.
5. Given an owner, when they transfer ownership to another member, then the new owner gains owner permissions and the previous owner loses them if demoted.

### FR4. Agenda home view

The home screen is an agenda view grouped by day with continuous scrolling and a date selector.

#### Acceptance Criteria

1. Given a signed-in user, when they open the app, then the default landing page is the agenda view.
2. Given scheduled meals across multiple days, when the user scrolls, then items appear grouped by day in chronological order.
3. Given the user selects a date, when the agenda updates, then the view jumps to or centers on the selected date's section.
4. Given a day has no scheduled meals, when that day is viewed, then the UI shows an empty state with an action to add a meal.

### FR5. Create and edit scheduled meals

Owners can create scheduled meals by choosing date/time first and then selecting one or more recipes.

#### Acceptance Criteria

1. Given an owner in the agenda view, when they choose to add a meal, then the app prompts for date and time before recipe selection.
2. Given selected recipes, when the owner saves the scheduled meal, then it appears in the agenda under the correct day and time.
3. Given a scheduled meal, when the owner adds notes or a URL, then those values persist and display in the meal detail view.
4. Given a scheduled meal with multiple recipes, when the owner sets a per-meal servings value, then all recipes inherit that value unless a per-recipe override is set.
5. Given a scheduled meal with per-recipe servings overrides, when the meal is saved, then the grocery calculations use the override for that recipe and the meal-level value for others.

### FR6. Recurring scheduled meals

Owners can create recurring meals with simple recurrence rules and edit one occurrence, future occurrences, or all occurrences.

#### Acceptance Criteria

1. Given an owner creating a scheduled meal, when they enable recurrence and choose a supported rule, then the system creates linked recurring occurrences.
2. Given a recurring scheduled meal occurrence, when the owner edits it, then the system offers "this occurrence", "this and future", and "all occurrences".
3. Given the owner chooses "this occurrence", when the save completes, then only that occurrence changes.
4. Given the owner chooses "this and future", when the save completes, then the selected occurrence and later occurrences change, and earlier ones do not.
5. Given the owner chooses "all occurrences", when the save completes, then every occurrence in the series changes.

### FR7. Recipe management

Owners can create, edit, tag, import, and organize recipes.

#### Acceptance Criteria

1. Given an owner, when they create a recipe with title, yield, ingredients, and steps, then the recipe is saved and available for scheduling.
2. Given recipes with tags, when the owner filters by a tag, then only matching recipes are shown.
3. Given a recipe is soft-deleted while referenced by schedules or lists, when those historical records are viewed, then the deleted recipe remains readable as a historical reference.
4. Given the owner searches in a recipe picker, when they type a query, then matches include recipe names and relevant aliases where supported.

### FR8. Nested component recipes

Recipes may include other recipes as components and leaf ingredients as ingredient lines.

#### Acceptance Criteria

1. Given an owner editing a recipe, when they add another recipe as a component, then the recipe graph stores that relationship with quantity/yield context.
2. Given a nested recipe graph, when the owner scales a parent recipe, then component quantities scale accordingly.
3. Given the configured nesting warning threshold is exceeded, when the owner saves, then the system warns but still allows saving.
4. Given a cyclic inclusion attempt, when a recipe is added as a component that would create a loop, then the system rejects the save.

### FR9. Ingredient catalog

Ingredients are first-class entities used across recipes.

#### Acceptance Criteria

1. Given an owner, when they create an ingredient, then it becomes selectable in recipe ingredient lines.
2. Given ingredient aliases/synonyms exist, when the owner searches in an ingredient picker, then the search matches the canonical name and aliases.
3. Given ingredient variants/forms are entered on a recipe line, when the recipe is saved, then the variant is stored as an attribute of that usage, not a new canonical ingredient.
4. Given an ingredient is soft-deleted while referenced historically, when old recipes or lists are viewed, then the historical reference remains readable.

### FR10. Unit conversions and densities

The system supports unit conversions, including volume↔weight using density.

#### Acceptance Criteria

1. Given two units in the same dimension, when the system needs to aggregate quantities, then it converts to a chosen target unit using configured conversion factors.
2. Given a volume↔weight conversion is required and an ingredient density exists, when the conversion runs, then the system uses that density to calculate the converted amount.
3. Given an owner updates an ingredient density, when future calculations run, then the new density is used.
4. Given a density override differs from the starter density, when the ingredient is viewed, then the UI indicates the density is custom.
5. Given an ingredient lacks a required density for a requested conversion, when the system cannot convert volume↔weight, then it flags the item for review instead of silently guessing.

### FR11. Grocery list generation

Owners can generate grocery snapshots by date range, selected meals, and/or selected ingredients.

#### Acceptance Criteria

1. Given scheduled meals exist, when the owner chooses a date range and generates a list, then the system creates a new named grocery snapshot containing required ingredients.
2. Given the owner chooses specific meals instead of a date range, when the snapshot is generated, then only ingredients from those meals are included.
3. Given the owner chooses specific ingredients as a filter, when the snapshot is generated, then only matching ingredients are included.
4. Given the owner regenerates for the same criteria later, when generation completes, then a new snapshot is created and prior snapshots remain unchanged.

### FR12. Grocery aggregation, traceability, and snapshot editing

Generated grocery items are merged when possible, traceable to their sources, and editable at the snapshot level.

#### Acceptance Criteria

1. Given the same canonical ingredient appears across multiple included meals, when the snapshot is generated, then the system merges compatible quantities and performs needed unit conversions.
2. Given merged grocery items, when the user opens item details, then the UI shows which meals, recipes, and ingredient lines contributed to the total.
3. Given a snapshot item, when a household member checks or unchecks it, then the state is saved only on that snapshot.
4. Given a snapshot, when a household member adds an ad-hoc item, then the item appears in the snapshot and is marked as ad-hoc.
5. Given an owner edits quantities or removes an item in a snapshot, when the edit is saved, then that edit affects only the snapshot and does not alter source recipes.
6. Given a deleted recipe or ingredient contributed to a snapshot historically, when the snapshot is viewed later, then the contributing reference remains readable.

### FR13. Rich link previews on scheduled meals

Scheduled meal links may show fetched title and image metadata.

#### Acceptance Criteria

1. Given an owner enters a URL on a scheduled meal, when preview fetch succeeds, then the system stores and displays the preview title and image.
2. Given preview fetch fails, when the owner edits the scheduled meal, then they can keep the raw URL and manually enter fallback preview values.
3. Given the URL changes, when the owner requests a refresh, then the system re-fetches the preview metadata.

### FR14. URL recipe import

Owners can import recipes from supported public pages using structured recipe metadata and fix mappings before saving.

#### Acceptance Criteria

1. Given a URL that exposes parseable recipe metadata, when the owner imports it, then the system presents mapped fields for review before saving.
2. Given parsing produces incomplete or ambiguous data, when the review screen is shown, then the owner can edit the mapped fields before import completes.
3. Given the owner saves an imported recipe, when the import completes, then the recipe stores its source URL and imported timestamp.
4. Given an imported recipe already exists, when the owner triggers re-import, then the system fetches the source again and lets the owner review changes before applying them.

### FR15. JSON import/export

Users can export a full-fidelity JSON file and import it later with merge/selective section options.

#### Acceptance Criteria

1. Given household data exists, when the owner exports, then the system produces one JSON file containing the selected sections and schema_version.
2. Given a valid export file, when the owner imports with merge mode, then the system merges supported entities using stable IDs and household scoping rules.
3. Given a valid export file, when the owner chooses selective import, then only the selected sections are imported.
4. Given the import file schema_version is unsupported, when import is attempted, then the system rejects the import with a clear compatibility error.
5. Given imported entities conflict by ID, when the owner chooses merge, then the system applies deterministic merge rules and reports any skipped/conflicted entities.

### FR16. Optimistic locking and conflict handling

Concurrent edits are detected and surfaced.

#### Acceptance Criteria

1. Given two owners open the same editable record, when one user saves changes first and the second later attempts to save stale data, then the second save is rejected as a conflict.
2. Given a conflict occurs, when the second user is shown the conflict screen, then the UI presents the current saved version and the user's attempted changes.
3. Given the conflict screen is shown, when the user reapplies their changes and saves again, then the system persists the updated record if no newer conflict exists.

## 5. Non-Functional Requirements

### 5.1 Performance

- Initial agenda page render target: p95 under 1.5 seconds for a household with up to 90 days of scheduled meals cached server-side.
- Picker search target: p95 under 250 ms for common lookups within a household.
- Grocery snapshot generation target: complete under 5 seconds for a household with up to:
  - 500 recipes
  - 3,000 ingredient lines
  - 90 scheduled meals in range

### 5.2 Security

- Passwords must be stored using a strong password hashing approach; OWASP recommends modern password hashing guidance rather than reversible encryption. :contentReference[oaicite:0]{index=0}
- CSRF protection is required on all non-GET requests.
- Sessions must use Secure, HttpOnly, SameSite=Lax cookies.
- Minimal login throttling must be enabled.
- Authorization checks must enforce household scope on every read/write path.

### 5.3 Reliability

- Soft deletes must preserve historical readability.
- Migration-driven schema changes must be reversible or explicitly forward-only with operator notes.
- Import/export must be versioned.

### 5.4 Usability

- Mobile-first interaction model.
- All owner editing flows must be usable on small screens without horizontal scrolling.
- Day-grouped agenda and grocery checklist interactions must remain thumb-friendly.
- Empty states must always provide a next action.

### 5.5 Compliance

- No special compliance requirement declared for MVP.
- App must avoid exposing sensitive internals in error responses.

### 5.6 Observability

- Structured request logs with request IDs.
- Error logging for failed imports, preview fetches, snapshot generation, and auth failures.
- Metrics for:
  - login attempts
  - import failures
  - snapshot generation duration
  - conflict rates
  - invite acceptance

## 6. Data Model

### 6.1 Entities

#### User

- id (UUID)
- household_id (UUID)
- username (nullable if email used as login)
- email (nullable if username-only account)
- password_hash
- role (owner, member)
- status
- created_at
- updated_at
- deleted_at

#### Household

- id (UUID)
- name
- created_at
- updated_at
- deleted_at

#### Invite

- id (UUID)
- household_id
- code
- created_by_user_id
- expires_at
- revoked_at
- max_uses (nullable)
- use_count
- created_at

#### Recipe

- id (UUID)
- household_id
- title
- description
- yield_amount
- yield_unit
- source_url (nullable)
- source_last_imported_at (nullable)
- version
- created_by_user_id
- updated_by_user_id
- created_at
- updated_at
- deleted_at

#### RecipeTag

- id (UUID)
- household_id
- name
- created_at

#### RecipeTagAssignment

- recipe_id
- tag_id

#### Ingredient

- id (UUID)
- household_id
- canonical_name
- default_density_g_per_ml (nullable)
- density_source_type (starter, custom, none)
- note (nullable)
- created_at
- updated_at
- deleted_at

#### IngredientAlias

- id (UUID)
- ingredient_id
- alias
- created_by_user_id
- created_at

#### RecipeComponent

- id (UUID)
- parent_recipe_id
- component_recipe_id
- quantity
- unit
- sort_order

#### RecipeIngredientLine

- id (UUID)
- recipe_id
- ingredient_id
- quantity
- unit
- variant_text
- prep_note
- optional_flag
- sort_order

#### RecipeStep

- id (UUID)
- recipe_id
- step_number
- instruction

#### ScheduledMeal

- id (UUID)
- household_id
- title (optional)
- scheduled_at
- notes
- link_url (nullable)
- link_title (nullable)
- link_image_url (nullable)
- recurrence_series_id (nullable)
- created_by_user_id
- updated_by_user_id
- version
- created_at
- updated_at
- deleted_at

#### ScheduledMealRecipe

- id (UUID)
- scheduled_meal_id
- recipe_id
- servings_override_amount (nullable)
- sort_order

#### RecurrenceSeries

- id (UUID)
- household_id
- rule_type
- interval_value
- by_weekday (nullable)
- start_at
- end_at (nullable)
- created_at
- updated_at

#### GrocerySnapshot

- id (UUID)
- household_id
- name
- generation_mode (date_range, meals, ingredients, mixed)
- generation_filter_json
- created_by_user_id
- created_at

#### GroceryItem

- id (UUID)
- snapshot_id
- ingredient_id (nullable for ad-hoc)
- display_name
- quantity
- unit
- checked
- source_type (generated, adhoc)
- note (nullable)
- created_by_user_id
- updated_by_user_id
- created_at
- updated_at
- deleted_at

#### GroceryItemSource

- id (UUID)
- grocery_item_id
- scheduled_meal_id (nullable)
- recipe_id (nullable)
- recipe_ingredient_line_id (nullable)
- quantity_contributed
- unit_contributed

#### ImportJob

- id (UUID)
- household_id
- type (url_recipe, json_merge, json_selective)
- status
- source_url (nullable)
- payload_json (nullable)
- report_json (nullable)
- created_by_user_id
- created_at
- completed_at (nullable)

### 6.2 Relationships

- Household 1..n Users
- Household 1..n Recipes
- Household 1..n Ingredients
- Household 1..n ScheduledMeals
- Household 1..n GrocerySnapshots
- Recipe n..n RecipeTags
- Recipe 1..n RecipeIngredientLines
- Recipe 1..n RecipeComponents
- ScheduledMeal 1..n ScheduledMealRecipes
- GrocerySnapshot 1..n GroceryItems
- GroceryItem 1..n GroceryItemSources

### 6.3 Invariants

- Every household-scoped record must belong to exactly one household.
- Members cannot mutate non-grocery household data.
- Recipe component graph must be acyclic.
- Soft-deleted records may not be used in new schedules/import links, but historical references remain visible.
- Snapshot generation never mutates older snapshots.
- Import/export IDs are UUIDs and must remain stable across exports/imports.

## 7. Integrations

- Public web pages for recipe URL import using structured recipe metadata
- PWA install/browser capabilities
- Optional metadata fetch for rich link previews

## 8. UX Requirements

### 8.1 Mobile navigation

Top-level navigation:

- Plan
- Grocery
- Recipes
- Ingredients
- Settings

### 8.2 Core screens

- Login
- Signup
- Create/join household
- Agenda view
- Scheduled meal create/edit
- Scheduled meal detail
- Recipe list
- Recipe detail/edit
- Ingredient list
- Ingredient detail/edit
- Grocery snapshot list
- Grocery snapshot detail
- URL import review/fix
- JSON import/export
- Household members/invites
- Conflict resolution screen

### 8.3 Key UX behaviors

- Agenda grouped by day, infinite-ish scroll by pagination/loading
- Pickers support inline search
- Grocery item detail explains "why this is here"
- Conflict UI shows saved version vs attempted version
- Recurring edit flow offers occurrence scope explicitly
- Preview fetch failures degrade gracefully to raw URL + manual fields

## 9. Out of Scope

- Offline editing
- Self-service password reset
- Email verification
- Nutrition
- Package-size optimization
- Provider-specific recipe integrations
- Advanced collaboration editing by members
- Global search page
- Full calendar-rule recurrence engine
- Real-time collaboration

## 10. Open Questions (remaining but non-blocking)

- Whether recurrence MVP is weekly-only or daily+weekly
- Whether invite links should have unlimited uses or configurable max uses
- Exact search match scope for scheduled meal link metadata
- Exact merge rules for JSON import field-level conflicts
