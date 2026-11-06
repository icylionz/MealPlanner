-- name: ListMealsInRange :many
SELECT * FROM meal_plan
WHERE household_id = $1 AND plan_date BETWEEN sqlc.arg(from_date) AND sqlc.arg(to_date) AND deleted_at IS NULL
ORDER BY plan_date, plan_time, title;

-- name: GetMeal :one
SELECT * FROM meal_plan WHERE id = $1 AND household_id = $2;

-- name: ListMealsByIDs :many
SELECT * FROM meal_plan WHERE household_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListAllMeals :many
SELECT * FROM meal_plan WHERE household_id = $1 AND deleted_at IS NULL ORDER BY plan_date, plan_time;

-- name: ListMealRecipes :many
SELECT * FROM scheduled_meal_recipes
WHERE meal_id = ANY(sqlc.arg(meal_ids)::uuid[])
ORDER BY meal_id, sort_order;

-- name: InsertMeal :one
INSERT INTO meal_plan (
    id, household_id, plan_date, plan_time, food_id, servings, series_id, title, notes, link_url, created_by, updated_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING *;

-- name: UpdateMeal :execrows
UPDATE meal_plan SET
    plan_date = $3, plan_time = $4, food_id = $5, servings = $6, title = $7,
    notes = $8, link_url = $9, series_id = $10, updated_by = $11, version = version + 1
WHERE id = $1 AND household_id = $12 AND version = $2 AND deleted_at IS NULL;

-- name: UpdateMealContent :exec
-- Series-wide edit: applies shared fields but keeps each occurrence's date.
UPDATE meal_plan SET
    plan_time = $2, food_id = $3, servings = $4, title = $5, notes = $6,
    link_url = $7, updated_by = $8, version = version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteMeal :execrows
UPDATE meal_plan SET deleted_at = now(), updated_by = $3, version = version + 1
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteMealRecipes :exec
DELETE FROM scheduled_meal_recipes WHERE meal_id = $1;

-- name: InsertMealRecipe :exec
INSERT INTO scheduled_meal_recipes (id, meal_id, food_id, servings_override, sort_order, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6);

-- name: InsertSeries :one
INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetSeries :one
SELECT * FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: ListSeries :many
SELECT * FROM meal_series WHERE household_id = $1;

-- name: BumpSeriesVersion :execrows
UPDATE meal_series SET version = version + 1, food_id = $3, plan_time = $4, servings = $5
WHERE id = $1 AND version = $2;

-- name: ListSeriesMealsFrom :many
SELECT * FROM meal_plan
WHERE series_id = $1 AND plan_date >= sqlc.arg(from_date) AND deleted_at IS NULL
ORDER BY plan_date;
