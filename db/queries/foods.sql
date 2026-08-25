-- name: ListFoods :many
SELECT * FROM foods WHERE household_id = $1 ORDER BY lower(name);

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1 AND household_id = $2;

-- name: CreateFood :one
INSERT INTO foods (household_id, name, description, prep_time_min, cook_time_min, servings, default_unit, density_g_per_ml, density_source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateFood :execrows
-- Optimistic lock (FR16): only writes when the caller's expected_version still
-- matches. Returns the number of rows updated — 0 means a stale write. Scoped by
-- household so a cross-household id cannot be written.
UPDATE foods
SET name = $2, description = $3, prep_time_min = $4, cook_time_min = $5,
    servings = $6, default_unit = $7, density_g_per_ml = $8, density_source = $9,
    version = version + 1, updated_at = now()
WHERE id = $1 AND household_id = $11 AND version = $10;

-- name: DeleteFood :exec
DELETE FROM foods WHERE id = $1 AND household_id = $2;

-- name: ListAllTags :many
SELECT DISTINCT ft.tag FROM food_tags ft
JOIN foods f ON f.id = ft.food_id
WHERE f.household_id = $1
ORDER BY ft.tag;

-- name: ListTagsForFoods :many
SELECT ft.food_id, ft.tag FROM food_tags ft
JOIN foods f ON f.id = ft.food_id
WHERE f.household_id = $1
ORDER BY ft.food_id, ft.tag;

-- name: DeleteFoodTags :exec
DELETE FROM food_tags WHERE food_id = $1;

-- name: AddFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListComponentsForFoods :many
SELECT fc.* FROM food_components fc
JOIN foods f ON f.id = fc.parent_food_id
WHERE f.household_id = $1
ORDER BY fc.parent_food_id, fc.sort_order;

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
