
-- name: CreateFood :one
INSERT INTO foods (name, unit_type, base_unit, density, is_recipe)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1;

-- name: ListFoods :many
SELECT * FROM foods ORDER BY name;

-- name: UpdateFood :one
UPDATE foods 
SET name = $2, unit_type = $3, base_unit = $4, density = $5, is_recipe = $6
WHERE id = $1
RETURNING *;

-- name: DeleteFood :exec
DELETE FROM foods WHERE id = $1;

-- name: GetRecipeWithIngredients :one
SELECT 
    r.*,
    COALESCE(
        json_agg(
            json_build_object(
                'food_id', ri.ingredient_id,
                'quantity', ri.quantity,
                'unit', ri.unit
            )
        ) FILTER (WHERE ri.ingredient_id IS NOT NULL),
        '[]'
    ) as ingredients
FROM recipes r
LEFT JOIN recipe_ingredients ri ON r.food_id = ri.recipe_id
WHERE r.food_id = $1
GROUP BY r.food_id;

-- name: CreateRecipe :one
INSERT INTO recipes (food_id, instructions, url, yield_quantity, yield_unit)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: AddRecipeIngredient :exec
INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit)
VALUES ($1, $2, $3, $4);

-- name: CreateSchedule :one
WITH inserted_schedule AS (
  INSERT INTO schedules (food_id, scheduled_at)
  VALUES ($1, $2)
  RETURNING *
)
SELECT s.*, f.name as food_name
FROM inserted_schedule s
JOIN foods f ON f.id = s.food_id;

-- name: GetSchedulesInRange :many
SELECT s.*, f.name as food_name
FROM schedules s
JOIN foods f ON s.food_id = f.id
WHERE scheduled_at BETWEEN $1 AND $2
ORDER BY scheduled_at;

-- name: DeleteScheduleByIds :exec
DELETE FROM schedules 
WHERE id = ANY($1::int[])
RETURNING id;

-- name: DeleteScheduleByDateRange :exec
DELETE FROM schedules 
WHERE scheduled_at >= $1 AND scheduled_at <= $2
RETURNING id;