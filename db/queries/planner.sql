-- name: ListMealsRange :many
SELECT * FROM meal_plan
WHERE household_id = $1 AND plan_date >= sqlc.arg(from_date) AND plan_date <= sqlc.arg(to_date)
  AND deleted_at IS NULL
ORDER BY plan_date, plan_time, id;

-- name: ListUpcomingMeals :many
SELECT * FROM meal_plan
WHERE household_id = $1 AND plan_date >= sqlc.arg(from_date) AND deleted_at IS NULL
ORDER BY plan_date, plan_time, id
LIMIT 200;

-- name: ListAllMeals :many
SELECT * FROM meal_plan WHERE household_id = $1 ORDER BY plan_date, plan_time, id;

-- name: GetMeal :one
SELECT * FROM meal_plan WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListMealExtras :many
SELECT * FROM scheduled_meal_recipes
WHERE meal_id = ANY(sqlc.arg(meal_ids)::uuid[])
ORDER BY meal_id, sort_order, id;

-- name: CreateMeal :one
INSERT INTO meal_plan (household_id, plan_date, plan_time, food_id, servings, series_id, title,
                       notes, link_url, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
RETURNING id;

-- name: UpdateMeal :execrows
UPDATE meal_plan
SET plan_date = $4, plan_time = $5, food_id = $6, servings = $7, title = $8, notes = $9,
    link_url = $10, updated_by = $11, version = version + 1
WHERE id = $1 AND household_id = $2 AND version = $3 AND deleted_at IS NULL;

-- name: UpdateSeriesMeal :exec
-- Applies a series-wide edit to one occurrence, keeping its own date.
UPDATE meal_plan
SET plan_time = $3, food_id = $4, servings = $5, title = $6, notes = $7, link_url = $8,
    updated_by = $9, version = version + 1
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteMeal :execrows
UPDATE meal_plan SET deleted_at = now(), updated_by = $3
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListSeriesMeals :many
SELECT * FROM meal_plan
WHERE series_id = $1 AND household_id = $2 AND plan_date >= sqlc.arg(from_date) AND deleted_at IS NULL
ORDER BY plan_date;

-- name: SoftDeleteSeriesMeals :exec
UPDATE meal_plan SET deleted_at = now(), updated_by = $3
WHERE series_id = $1 AND household_id = $2 AND plan_date >= sqlc.arg(from_date) AND deleted_at IS NULL;

-- name: DeleteMealExtras :exec
DELETE FROM scheduled_meal_recipes WHERE meal_id = $1;

-- name: InsertMealExtra :exec
INSERT INTO scheduled_meal_recipes (meal_id, food_id, servings_override, sort_order, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $5);

-- name: CreateSeries :one
INSERT INTO meal_series (household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetSeries :one
SELECT * FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: TouchSeries :exec
UPDATE meal_series SET food_id = $3, plan_time = $4, servings = $5, version = version + 1
WHERE id = $1 AND household_id = $2;

-- name: ListAllSeries :many
SELECT * FROM meal_series WHERE household_id = $1 ORDER BY start_date, id;

-- name: ListAllMealExtras :many
SELECT r.*
FROM scheduled_meal_recipes r
JOIN meal_plan m ON m.id = r.meal_id
WHERE m.household_id = $1
ORDER BY r.meal_id, r.sort_order, r.id;

-- name: UpsertSeries :execrows
INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE
SET food_id = EXCLUDED.food_id, plan_time = EXCLUDED.plan_time, servings = EXCLUDED.servings,
    freq = EXCLUDED.freq, byweekday = EXCLUDED.byweekday, start_date = EXCLUDED.start_date,
    until_date = EXCLUDED.until_date, version = meal_series.version + 1
WHERE meal_series.household_id = EXCLUDED.household_id;

-- name: UpsertMeal :execrows
INSERT INTO meal_plan (id, household_id, plan_date, plan_time, food_id, servings, series_id, title,
                       notes, link_url, created_by, updated_by, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, $12)
ON CONFLICT (id) DO UPDATE
SET plan_date = EXCLUDED.plan_date, plan_time = EXCLUDED.plan_time, food_id = EXCLUDED.food_id,
    servings = EXCLUDED.servings, series_id = EXCLUDED.series_id, title = EXCLUDED.title,
    notes = EXCLUDED.notes, link_url = EXCLUDED.link_url, updated_by = EXCLUDED.updated_by,
    deleted_at = EXCLUDED.deleted_at, version = meal_plan.version + 1
WHERE meal_plan.household_id = EXCLUDED.household_id;
