# PRD

## Implementation Authority

- `docs/architecture.md` is the implementation architecture source of truth.
- Prototype artifacts belong under `docs/prototype/`; no prototype files are currently checked into that path.
- When restored, the prototype is not a suggestion. The shipped UI should match it and its design system strictly.
- Any UI deviation requires user approval first, together with the reason, impact, and implementation plan.

## 1. Summary

Build a mobile-first, responsive meal planner for shared households. Users can schedule meals, attach one or more foods/recipes to a scheduled meal, add aggregated ingredients to live grocery lists from a planned meal, food, or date range, organize prep sessions, import/export household data, and manage ingredient conversions including density-based volume↔weight conversion. The MVP is a server-hosted PWA + website built with a Go server-rendered stack.

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
- Checks and unchecks items on a live grocery list while shopping
- Does not edit recipes, schedules, ingredients, or household settings

## 3. Scope

### 3.1 MVP

- Authentication with email + password; usernames are not part of the account model
- Household create/join flow
- Owner/member roles
- Today landing view and Plan calendar/list views
- Scheduled meals with:
  - date + time
  - multiple recipes
  - per-meal servings
  - optional per-recipe servings override
  - notes
  - optional link with rich preview
  - recurrence
- Unified Foods library: atomic ingredients and recipes-with-components share one catalog
- Food tags and aliases/synonyms
- Ingredient variants/forms as attributes on usage lines
- Nested component recipes
- Yield/scaling across nested recipes
- Unit conversion:
  - same-dimension conversions
  - volume↔weight via density
  - starter density set + user override
- Grocery list generation by:
  - date range
  - one selected planned meal
  - one selected food/recipe with servings
- Live grocery lists:
  - named
  - directly editable rather than immutable snapshots
  - generation can add to an existing list or create a new list
  - pantry/purchased check state per list
  - traceability to source meals/recipes/ingredients
- Prep sessions with selected foods, per-food servings, aggregated ingredients, per-meal breakdowns, and print/export view
- Account Settings for display name, sign-in email, and password changes
- Optional CoFID seeder that enriches the Starter Template food catalog for subsequently created households
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

Users sign up and log in with an email address and password and can access their household data across devices. Accounts have a display name but no username credential.

#### Acceptance Criteria

1. Given a new user on the signup page, when they provide a display name, a unique valid email, and a valid password, then the system creates an account and proceeds to household create/join.
2. Given an existing user on the login page, when they submit valid credentials, then the system creates a session and redirects them to Today.
3. Given invalid credentials, when the user submits the login form, then the system rejects the login with a generic error message and does not reveal whether the email exists.
4. Given repeated failed login attempts from the same IP or account, when the threshold is exceeded, then the system slows or blocks further attempts for a limited period.

### FR2. Household create or join

A user must create a new household or join an existing one via invite during onboarding.

#### Acceptance Criteria

1. Given a newly authenticated user without a household, when they choose "Create household", then the system creates a new household and makes them the owner.
2. Given a newly authenticated user without a household, when they enter a valid invite code or open a valid invite link, then the system joins them to that household as a member.
3. Given an expired or revoked invite, when a user attempts to join, then the system rejects the join and explains the invite is invalid.
4. Given a valid invite, when its configured expiration time passes, then the invite no longer permits joining.

### FR3. Household roles and permissions

Owners can edit household data and manage members; members can view data and interact with live grocery lists only.

#### Acceptance Criteria

1. Given an owner, when they access Foods, Plan, grocery list management, Prep, or household administration, then the corresponding edit controls are available.
2. Given a member, when they access Foods, Plan, Grocery, Prep, or Household, then owner-only edit controls are not available.
3. Given a member viewing a grocery list, when they check or uncheck an item, then the system saves the change on that list.
4. Given an owner, when they remove a member, then that member immediately loses household access.
5. Given an owner, when they transfer ownership to another member, then the new owner gains owner permissions and the previous owner loses them if demoted.

### FR4. Today and Plan views

Today is the default landing screen and shows the current day's agenda. Plan provides calendar navigation with day-list and week-grid planning views.

#### Acceptance Criteria

1. Given a signed-in user with an active household, when they open the app, then the root route redirects to Today.
2. Given meals scheduled today, when Today opens, then they appear in chronological order with the next meal highlighted.
3. Given the user opens Plan, when they select a date or week, then the day-list or week-grid view shows the corresponding scheduled meals.
4. Given a viewed day has no scheduled meals, then the UI shows an empty state with an owner action to add a meal.

### FR5. Create and edit scheduled meals

Owners can create scheduled meals by choosing date/time first and then selecting one or more recipes.

#### Acceptance Criteria

1. Given an owner in Today or Plan, when they choose to add a meal, then the app prompts for date and time before recipe selection.
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

Owners add aggregated ingredients to a live grocery list from one planned meal, one food/recipe, or a date range of planned meals. Immutable point-in-time grocery snapshots are a deliberate non-goal: lists remain editable, and generation may merge into a selected existing list or create a new live list.

#### Acceptance Criteria

1. Given scheduled meals exist, when the owner chooses a valid date range, then ingredients from every meal in the range are aggregated for insertion.
2. Given the owner chooses a planned meal, then the primary and additional recipes are expanded using the meal servings and any per-recipe override.
3. Given the owner chooses a food/recipe and servings, then its leaf ingredients are aggregated for insertion.
4. Given the owner selects an existing list, when generation completes, then compatible generated lines merge into that live list and retain source traceability.
5. Given no existing target list is selected, when generation completes, then the system creates a new live list named "Generated list".

### FR12. Grocery aggregation, traceability, and live-list editing

Generated grocery items are merged when possible, traceable to their sources, and editable on their live list.

#### Acceptance Criteria

1. Given the same canonical ingredient and normalized variant appear across included sources, when ingredients are added, then the system merges compatible quantities and performs supported unit conversions.
2. Given merged grocery items, when the user opens item details, then the UI shows which meals, recipes, and ingredient lines contributed to the total.
3. Given a list item, when a household member checks or unchecks it, then the state is saved on that live list.
4. Given an owner converts or removes an item in a list, then that edit affects only the list and does not alter source foods or planned meals.
5. Given a deleted recipe or ingredient contributed to a generated item historically, when item details are viewed later, then the contributing reference remains readable.

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

### FR17. Prep sessions

Owners can group foods/recipes into dated prep sessions, adjust servings, and review the resulting prep work. Members have read-only access.

#### Acceptance Criteria

1. Given an owner, when they create, rename, re-date, or delete a prep session, then the session list reflects the change.
2. Given an owner editing a prep session, when they add or remove a food or adjust its servings, then the session updates accordingly.
3. Given a session with foods, when it is viewed, then the app shows aggregated leaf ingredients and a per-food ingredient/step breakdown.
4. Given any household member viewing a session, when they open its print/export route, then a printable preparation view is rendered.

### FR18. Account settings

Authenticated users can maintain their own account profile and password from Settings. Settings is linked from the desktop sidebar and is not a mobile bottom-tab destination.

#### Acceptance Criteria

1. Given an authenticated user, when they save a non-empty display name and unique valid email, then the account profile is updated.
2. Given an authenticated user, when they provide the correct current password and a new password of at least eight characters, then the password is changed.
3. Given an invalid current password or an email already in use, then the app preserves the submitted profile context and shows an error.

## 5. Non-Functional Requirements

### 5.1 Performance

- Initial agenda page render target: p95 under 1.5 seconds for a household with up to 90 days of scheduled meals cached server-side.
- Picker search target: p95 under 250 ms for common lookups within a household.
- Grocery ingredient generation target: complete under 5 seconds for a household with up to:
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
- Error logging for failed imports, preview fetches, grocery generation, and auth failures.
- Metrics for:
  - login attempts
  - import failures
  - grocery generation duration
  - conflict rates
  - invite acceptance

## 6. Data Model

This section describes the shipped PostgreSQL schema through migration `0018`.
It supersedes the earlier target model that used separate Recipe, Ingredient,
RecipeIngredientLine, and immutable GrocerySnapshot entities. Database table and
column names below are the implementation contract.

### 6.1 Identity and households

#### Account (`accounts`)

- `id` (UUID)
- `email` (required, case-insensitively unique; the only login identifier)
- `password_hash`
- `name` (display name)
- `created_at`

#### Household (`households`)

- `id` (UUID)
- `name`
- `is_template` (marks the hidden Starter Template)
- `created_at`

#### HouseholdMember (`household_members`)

- `id` (UUID)
- `household_id`, `account_id` (unique pair)
- `role` (`owner`, `member`)
- `initials`, `color`
- `created_at`

#### Session (`sessions`)

- `token`
- `account_id`
- `active_household_id` (nullable)
- `created_at`, `expires_at`

#### Invite (`invites`)

- `id`, `household_id`, `code`
- `expires_at`, `revoked_at` (nullable)
- `max_uses` (nullable), `use_count`
- `created_by` (nullable account UUID), `created_at`

### 6.2 Unified foods

#### Food (`foods`)

A food is atomic when it has no components and acts as a recipe when it has one
or more components. Both forms share the same catalog and identifier space.

- `id`, `household_id`, `name`, `description`
- `prep_time_min`, `cook_time_min`, `servings`, `default_unit`
- `density_g_per_ml`, `density_source` (`starter`, `custom`, `none`)
- `source_url`, `source_last_imported_at` (nullable)
- `version`
- `created_by`, `updated_by` (nullable account UUIDs)
- `created_at`, `updated_at`, `deleted_at` (nullable)

#### FoodAlias (`food_aliases`)

- `id`, `food_id`, `alias`
- `created_by` (nullable), `created_at`

#### FoodTag (`food_tags`)

- `food_id`, `tag` (composite primary key)

#### FoodComponent (`food_components`)

- `id`, `parent_food_id`, `child_food_id`
- `amount`, `unit`, `variant_text`, `sort_order`

Each component references another food; component lines are never free text.
The same table represents recipe-to-recipe nesting and recipe-to-atomic-food
ingredient usage.

#### FoodStep (`food_steps`)

- `food_id`, `step_number` (composite primary key)
- `instruction`

### 6.3 Planning

#### MealSeries (`meal_series`)

- `id`, `household_id`, `food_id`
- `plan_time`, `servings`
- `freq` (`daily`, `weekly`), `byweekday`
- `start_date`, `until_date`
- `version` (series-wide optimistic-lock token)

#### ScheduledMeal (`meal_plan`)

- `id`, `household_id`, `plan_date`, `plan_time`
- `food_id` (the primary food/recipe), `servings`
- `series_id` (nullable)
- `title`, `notes`
- `link_url`, `link_title`, `link_image_url`
- `version`
- `created_by`, `updated_by` (nullable account UUIDs)
- `deleted_at` (nullable)

#### ScheduledMealRecipe (`scheduled_meal_recipes`)

- `id`, `meal_id`, `food_id`
- `servings_override` (nullable; falls back to meal servings)
- `sort_order`
- `created_by`, `updated_by` (nullable account UUIDs)

The primary recipe remains on `meal_plan.food_id`; this table stores only
additional attached recipes.

### 6.4 Grocery lists

#### GroceryList (`grocery_lists`)

- `id`, `household_id`, `name`
- `created_by`, `updated_by` (nullable account UUIDs)
- `created_at`, `deleted_at` (nullable)

#### GroceryItem (`grocery_items`)

- `id`, `list_id`
- `ingredient_id` (nullable food UUID)
- `name`, `amount`, `unit`, `variant_text`, `note`, `sort_order`
- `checked`
- `source_type` (`generated`, `adhoc`)
- `created_by`, `updated_by` (nullable account UUIDs)
- `deleted_at` (nullable)

#### GroceryItemSource (`grocery_item_sources`)

- `id`, `grocery_item_id`
- `scheduled_meal_id`, `recipe_id`, `recipe_ingredient_line_id` (nullable)
- `quantity_contributed`, `unit_contributed`
- `variant_text`, `line_recipe_name`

`line_recipe_name` and source variant values are display snapshots used to keep
provenance readable if a component line later changes. There is no
GrocerySnapshot table or generation-filter record.

### 6.5 Prep

#### PrepSession (`prep_sessions`)

- `id`, `household_id`, `name`, `session_date`, `created_at`

#### PrepSessionMeal (`prep_session_meals`)

- `session_id`, `food_id` (composite primary key)
- `servings`, `sort_order`

### 6.6 Relationships

- Accounts join one or more households through HouseholdMember; a Session may select one active household.
- Household owns Foods, MealSeries, ScheduledMeals, GroceryLists, and PrepSessions.
- Food has aliases, tags, steps, and child Foods through FoodComponent.
- ScheduledMeal has one primary Food and zero or more additional Foods through ScheduledMealRecipe.
- GroceryList has GroceryItems; generated GroceryItems have GroceryItemSources.
- PrepSession has Foods through PrepSessionMeal.

### 6.7 Invariants

- Every household-scoped record and every referenced child record must remain within one household.
- Members cannot mutate foods, plans, grocery list structure/items, prep sessions, imports, or household administration; they may check and uncheck grocery items.
- The food component graph must be acyclic.
- Soft-deleted foods, meals, lists, and items are unavailable to new work, while historical meal and grocery provenance remains readable.
- Grocery lists are live and mutable. Generation may merge into an existing list; the system does not promise immutable historical snapshots.
- Import/export IDs are UUIDs and remain stable across exports/imports. Import processing is request-scoped; there is no persisted ImportJob table.

## 7. Integrations

- Public web pages for recipe URL import using structured recipe metadata
- UK CoFID 2021 workbook, fetched by the optional `cmd/seed-foods` operator command or supplied as a local file; the idempotent seeder populates the Starter Template so future households clone the enriched raw-food catalog
- PWA install/browser capabilities
- Optional metadata fetch for rich link previews

## 8. UX Requirements

### 8.1 Navigation

The mobile bottom tab bar contains exactly:

- Today
- Plan
- Foods
- Grocery
- Prep

The desktop sidebar contains those five destinations plus Data, Household, and
Settings. Settings is sidebar-only navigation and does not appear in the mobile
bottom tab bar. Atomic ingredients and recipes are both reached through Foods;
there are no separate Recipes and Ingredients navigation destinations.

### 8.2 Core screens

- Login
- Register
- Create/join household
- Today agenda
- Plan calendar with day-list and week-grid layouts
- Scheduled meal create/edit
- Foods list, food detail, and food editor for both atomic foods and recipes
- URL/file food import and reconcile/review
- Live grocery lists, grocery generation, and grocery item provenance detail
- Prep session list/detail and printable prep view
- JSON import/export
- Household switching, members, ownership transfer, and invite lifecycle
- Account Settings for display name, email, and password
- Food and scheduled-meal optimistic-lock conflict states

### 8.3 Key UX behaviors

- Today orders the current day's meals chronologically; Plan provides explicit date and week navigation
- Pickers support inline search
- Grocery item detail explains "why this is here"
- Food and scheduled-meal conflict UI shows saved version vs attempted version
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
- Immutable or versioned grocery snapshots; grocery lists are intentionally live and mutable

## 10. Open Questions (remaining but non-blocking)

- Whether recurrence MVP is weekly-only or daily+weekly
- Whether invite links should have unlimited uses or configurable max uses
- Exact search match scope for scheduled meal link metadata
- Exact merge rules for JSON import field-level conflicts
