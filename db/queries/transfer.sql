-- Full-fidelity export/import queries (FR15), scoped to one household. Exports
-- read the active household's entities ordered deterministically; imports upsert
-- by stable UUID into the active household so a re-import merges rather than
-- duplicates. Each upsert returns whether the row was newly inserted (xmax = 0)
-- so the import can report inserted vs updated counts. Membership/accounts are
-- not part of an archive — they are managed via invites, not import.

-- name: ExportFoods :many
SELECT * FROM foods WHERE household_id = $1 ORDER BY created_at, id;

-- name: ExportFoodTags :many
SELECT ft.food_id, ft.tag FROM food_tags ft
JOIN foods f ON f.id = ft.food_id
WHERE f.household_id = $1
ORDER BY ft.food_id, ft.tag;

-- name: ExportFoodComponents :many
SELECT fc.* FROM food_components fc
JOIN foods f ON f.id = fc.parent_food_id
WHERE f.household_id = $1
ORDER BY fc.parent_food_id, fc.sort_order, fc.id;

-- name: ExportFoodSteps :many
SELECT fs.* FROM food_steps fs
JOIN foods f ON f.id = fs.food_id
WHERE f.household_id = $1
ORDER BY fs.food_id, fs.step_number;

-- name: ExportMealSeries :many
SELECT * FROM meal_series WHERE household_id = $1 ORDER BY start_date, id;

-- name: ExportMealPlans :many
SELECT * FROM meal_plan WHERE household_id = $1 ORDER BY plan_date, plan_time, id;

-- name: ExportGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 ORDER BY created_at, id;

-- name: ExportGroceryItems :many
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1
ORDER BY gi.list_id, gi.sort_order, gi.id;

-- name: ExportPrepSessions :many
SELECT * FROM prep_sessions WHERE household_id = $1 ORDER BY session_date, id;

-- name: ExportPrepSessionMeals :many
SELECT psm.* FROM prep_session_meals psm
JOIN prep_sessions ps ON ps.id = psm.session_id
WHERE ps.household_id = $1
ORDER BY psm.session_id, psm.sort_order;

-- Which UUIDs already exist in this household, so an import can resolve foreign
-- keys without tripping constraints for sections the caller chose not to import.
-- name: ExistingFoodIDs :many
SELECT id FROM foods WHERE household_id = $1;

-- name: ExistingMealSeriesIDs :many
SELECT id FROM meal_series WHERE household_id = $1;

-- name: ImportFood :one
INSERT INTO foods (id, household_id, name, description, prep_time_min, cook_time_min, servings,
    default_unit, created_at, updated_at, density_g_per_ml, density_source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, description = excluded.description,
    prep_time_min = excluded.prep_time_min, cook_time_min = excluded.cook_time_min,
    servings = excluded.servings, default_unit = excluded.default_unit,
    updated_at = excluded.updated_at,
    density_g_per_ml = excluded.density_g_per_ml, density_source = excluded.density_source
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2)
ON CONFLICT (food_id, tag) DO NOTHING;

-- name: ImportFoodComponent :one
INSERT INTO food_components (id, parent_food_id, child_food_id, amount, unit, sort_order)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    parent_food_id = excluded.parent_food_id, child_food_id = excluded.child_food_id,
    amount = excluded.amount, unit = excluded.unit, sort_order = excluded.sort_order
RETURNING (xmax = 0) AS inserted;

-- name: ImportFoodStep :exec
INSERT INTO food_steps (food_id, step_number, instruction)
VALUES ($1, $2, $3)
ON CONFLICT (food_id, step_number) DO UPDATE SET instruction = excluded.instruction;

-- name: ImportMealSeries :one
INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    food_id = excluded.food_id, plan_time = excluded.plan_time,
    servings = excluded.servings, freq = excluded.freq, byweekday = excluded.byweekday,
    start_date = excluded.start_date, until_date = excluded.until_date
RETURNING (xmax = 0) AS inserted;

-- name: ImportMealPlan :one
INSERT INTO meal_plan (id, household_id, plan_date, plan_time, food_id, servings, series_id,
    link_url, link_title, link_image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET
    plan_date = excluded.plan_date, plan_time = excluded.plan_time,
    food_id = excluded.food_id, servings = excluded.servings, series_id = excluded.series_id,
    link_url = excluded.link_url, link_title = excluded.link_title,
    link_image_url = excluded.link_image_url
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryList :one
INSERT INTO grocery_lists (id, household_id, name, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = excluded.name
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryItem :one
INSERT INTO grocery_items (id, list_id, name, amount, unit, checked, note, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    list_id = excluded.list_id, name = excluded.name, amount = excluded.amount,
    unit = excluded.unit, checked = excluded.checked, note = excluded.note,
    sort_order = excluded.sort_order
RETURNING (xmax = 0) AS inserted;

-- name: ImportPrepSession :one
INSERT INTO prep_sessions (id, household_id, name, session_date, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET name = excluded.name, session_date = excluded.session_date
RETURNING (xmax = 0) AS inserted;

-- name: ImportPrepSessionMeal :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, food_id) DO UPDATE SET
    servings = excluded.servings, sort_order = excluded.sort_order;
