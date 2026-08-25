-- name: ListMealsBetween :many
SELECT * FROM meal_plan
WHERE plan_date >= $1 AND plan_date <= $2
ORDER BY plan_date, plan_time;

-- name: ListMealDatesBetween :many
SELECT DISTINCT plan_date FROM meal_plan
WHERE plan_date >= $1 AND plan_date <= $2
ORDER BY plan_date;

-- name: GetMeal :one
SELECT * FROM meal_plan WHERE id = $1;

-- name: CreateMeal :one
INSERT INTO meal_plan (plan_date, plan_time, food_id, servings, series_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateMeal :exec
UPDATE meal_plan
SET plan_date = $2, plan_time = $3, food_id = $4, servings = $5
WHERE id = $1;

-- name: DetachMeal :exec
UPDATE meal_plan SET series_id = NULL WHERE id = $1;

-- name: DeleteMeal :exec
DELETE FROM meal_plan WHERE id = $1;

-- name: CreateSeries :one
INSERT INTO meal_series (food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSeries :one
SELECT * FROM meal_series WHERE id = $1;

-- name: ListSeriesMeals :many
SELECT * FROM meal_plan
WHERE series_id = $1 AND plan_date >= $2
ORDER BY plan_date, plan_time;

-- name: UpdateSeriesMealsFrom :exec
UPDATE meal_plan
SET plan_time = $3, food_id = $4, servings = $5
WHERE series_id = $1 AND plan_date >= $2;

-- name: DeleteSeriesMealsFrom :exec
DELETE FROM meal_plan
WHERE series_id = $1 AND plan_date >= $2;

-- name: DeleteSeries :exec
DELETE FROM meal_series WHERE id = $1;

-- name: UpdateMealLink :exec
-- FR13: set/clear the external link and its preview on a single meal occurrence.
UPDATE meal_plan
SET link_url = $2, link_title = $3, link_image_url = $4
WHERE id = $1;
