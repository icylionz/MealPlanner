-- Full-fidelity export/import queries (FR15). Exports read every household
-- entity ordered deterministically; imports upsert by stable UUID so a re-import
-- merges rather than duplicates. Each upsert returns whether the row was newly
-- inserted (xmax = 0) so the import can report inserted vs updated counts.

-- name: ExportMembers :many
SELECT * FROM household_members ORDER BY created_at, id;

-- name: ExportFoods :many
SELECT * FROM foods ORDER BY created_at, id;

-- name: ExportFoodTags :many
SELECT food_id, tag FROM food_tags ORDER BY food_id, tag;

-- name: ExportFoodComponents :many
SELECT * FROM food_components ORDER BY parent_food_id, sort_order, id;

-- name: ExportFoodSteps :many
SELECT * FROM food_steps ORDER BY food_id, step_number;

-- name: ExportMealSeries :many
SELECT * FROM meal_series ORDER BY start_date, id;

-- name: ExportMealPlans :many
SELECT * FROM meal_plan ORDER BY plan_date, plan_time, id;

-- name: ExportGroceryLists :many
SELECT * FROM grocery_lists ORDER BY created_at, id;

-- name: ExportGroceryItems :many
SELECT * FROM grocery_items ORDER BY list_id, sort_order, id;

-- name: ExportPrepSessions :many
SELECT * FROM prep_sessions ORDER BY session_date, id;

-- name: ExportPrepSessionMeals :many
SELECT * FROM prep_session_meals ORDER BY session_id, sort_order;

-- Which UUIDs already exist, so an import can resolve foreign keys without
-- tripping constraints for sections the caller chose not to import.
-- name: ExistingFoodIDs :many
SELECT id FROM foods;

-- name: ExistingMealSeriesIDs :many
SELECT id FROM meal_series;

-- name: ImportMember :one
INSERT INTO household_members (id, name, role, initials, color, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name, role = excluded.role,
    initials = excluded.initials, color = excluded.color
RETURNING (xmax = 0) AS inserted;

-- name: ImportFood :one
INSERT INTO foods (id, name, description, prep_time_min, cook_time_min, servings,
    default_unit, created_at, updated_at, density_g_per_ml, density_source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
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
INSERT INTO meal_series (id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    food_id = excluded.food_id, plan_time = excluded.plan_time,
    servings = excluded.servings, freq = excluded.freq, byweekday = excluded.byweekday,
    start_date = excluded.start_date, until_date = excluded.until_date
RETURNING (xmax = 0) AS inserted;

-- name: ImportMealPlan :one
INSERT INTO meal_plan (id, plan_date, plan_time, food_id, servings, series_id,
    link_url, link_title, link_image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    plan_date = excluded.plan_date, plan_time = excluded.plan_time,
    food_id = excluded.food_id, servings = excluded.servings, series_id = excluded.series_id,
    link_url = excluded.link_url, link_title = excluded.link_title,
    link_image_url = excluded.link_image_url
RETURNING (xmax = 0) AS inserted;

-- name: ImportGroceryList :one
INSERT INTO grocery_lists (id, name, created_at)
VALUES ($1, $2, $3)
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
INSERT INTO prep_sessions (id, name, session_date, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = excluded.name, session_date = excluded.session_date
RETURNING (xmax = 0) AS inserted;

-- name: ImportPrepSessionMeal :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, food_id) DO UPDATE SET
    servings = excluded.servings, sort_order = excluded.sort_order;
