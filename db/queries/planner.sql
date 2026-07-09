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
INSERT INTO meal_plan (plan_date, plan_time, recipe_id, servings)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteMeal :exec
DELETE FROM meal_plan WHERE id = $1;
