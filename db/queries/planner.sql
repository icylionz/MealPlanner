-- name: ListMealsBetween :many
SELECT * FROM meal_plan
WHERE household_id = $1 AND plan_date >= $2 AND plan_date <= $3 AND deleted_at IS NULL
ORDER BY plan_date, plan_time;

-- name: ListMealDatesBetween :many
SELECT DISTINCT plan_date FROM meal_plan
WHERE household_id = $1 AND plan_date >= $2 AND plan_date <= $3 AND deleted_at IS NULL
ORDER BY plan_date;

-- name: GetMeal :one
SELECT * FROM meal_plan WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateMeal :one
INSERT INTO meal_plan (household_id, plan_date, plan_time, food_id, servings, series_id, title, notes, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING *;

-- name: UpdateMeal :exec
UPDATE meal_plan
SET plan_date = $2, plan_time = $3, food_id = $4, servings = $5, title = $8, notes = $9, version = version + 1, updated_by = $7
WHERE id = $1 AND household_id = $6 AND deleted_at IS NULL;

-- name: DetachMeal :exec
UPDATE meal_plan SET series_id = NULL WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteMeal :exec
-- Soft delete (G1): keep the occurrence as history instead of removing it.
UPDATE meal_plan SET deleted_at = now(), updated_by = $3 WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateSeries :one
INSERT INTO meal_series (household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSeries :one
SELECT * FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: ListSeriesMeals :many
SELECT * FROM meal_plan
WHERE series_id = $1 AND plan_date >= $2 AND deleted_at IS NULL
ORDER BY plan_date, plan_time;

-- name: UpdateSeriesMealsFrom :exec
UPDATE meal_plan
SET plan_time = $3, food_id = $4, servings = $5, title = $7, notes = $8, version = version + 1, updated_by = $6
WHERE series_id = $1 AND plan_date >= $2 AND deleted_at IS NULL;

-- name: DeleteSeriesMealsFrom :exec
-- Soft delete this-and-future occurrences of a series (G1).
UPDATE meal_plan SET deleted_at = now(), updated_by = $3
WHERE series_id = $1 AND plan_date >= $2 AND deleted_at IS NULL;

-- name: DeleteSeriesMeals :exec
-- Soft delete every occurrence of a series (scope=all, G1). The meal_series rule
-- row is left in place; a hard DELETE there would cascade-remove the occurrences
-- and defeat the soft-delete history.
UPDATE meal_plan SET deleted_at = now(), updated_by = $3
WHERE series_id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteSeries :exec
DELETE FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: CreateMealRecipe :exec
-- G4: attach an additional recipe to a scheduled meal, with an optional
-- per-recipe servings override (NULL => use the meal's servings).
INSERT INTO scheduled_meal_recipes (meal_id, food_id, servings_override, sort_order, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $5);

-- name: ListMealRecipes :many
SELECT * FROM scheduled_meal_recipes WHERE meal_id = $1 ORDER BY sort_order, id;

-- name: ListMealRecipesForMeals :many
-- Batch-load additional recipes for a set of meals (list/grocery views).
SELECT * FROM scheduled_meal_recipes WHERE meal_id = ANY($1::uuid[]) ORDER BY meal_id, sort_order, id;

-- name: DeleteMealRecipes :exec
-- Replace-on-edit: clear a meal's additional recipes before re-inserting.
DELETE FROM scheduled_meal_recipes WHERE meal_id = $1;

-- name: UpdateMealLink :exec
-- FR13: set/clear the external link and its preview on a single meal occurrence.
UPDATE meal_plan
SET link_url = $2, link_title = $3, link_image_url = $4, updated_by = $6
WHERE id = $1 AND household_id = $5 AND deleted_at IS NULL;
