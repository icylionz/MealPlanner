-- name: ListFoods :many
SELECT * FROM foods ORDER BY lower(name);

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1;

-- name: CreateFood :one
INSERT INTO foods (name, description, prep_time_min, cook_time_min, servings, default_unit)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateFood :exec
UPDATE foods
SET name = $2, description = $3, prep_time_min = $4, cook_time_min = $5,
    servings = $6, default_unit = $7, updated_at = now()
WHERE id = $1;

-- name: DeleteFood :exec
DELETE FROM foods WHERE id = $1;

-- name: ListAllTags :many
SELECT DISTINCT tag FROM food_tags ORDER BY tag;

-- name: ListTagsForFoods :many
SELECT food_id, tag FROM food_tags ORDER BY food_id, tag;

-- name: DeleteFoodTags :exec
DELETE FROM food_tags WHERE food_id = $1;

-- name: AddFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListComponentsForFoods :many
SELECT * FROM food_components ORDER BY parent_food_id, sort_order;

-- name: ListFoodComponents :many
SELECT * FROM food_components WHERE parent_food_id = $1 ORDER BY sort_order;

-- name: DeleteFoodComponents :exec
DELETE FROM food_components WHERE parent_food_id = $1;

-- name: AddFoodComponent :exec
INSERT INTO food_components (parent_food_id, child_food_id, amount, unit, sort_order)
VALUES ($1, $2, $3, $4, $5);

-- name: CountComponentUses :one
SELECT count(*) FROM food_components WHERE child_food_id = $1;

-- name: ListFoodSteps :many
SELECT * FROM food_steps WHERE food_id = $1 ORDER BY step_number;

-- name: DeleteFoodSteps :exec
DELETE FROM food_steps WHERE food_id = $1;

-- name: AddFoodStep :exec
INSERT INTO food_steps (food_id, step_number, instruction) VALUES ($1, $2, $3);
