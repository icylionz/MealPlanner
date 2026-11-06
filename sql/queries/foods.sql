-- name: ListFoods :many
-- Includes soft-deleted rows so historical references stay readable; callers
-- filter on deleted_at for new work.
SELECT * FROM foods WHERE household_id = $1 ORDER BY lower(name);

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1 AND household_id = $2;

-- name: ListHouseholdComponents :many
SELECT c.*
FROM food_components c
JOIN foods p ON p.id = c.parent_food_id
WHERE p.household_id = $1
ORDER BY c.parent_food_id, c.sort_order;

-- name: ListHouseholdAliases :many
SELECT a.*
FROM food_aliases a
JOIN foods f ON f.id = a.food_id
WHERE f.household_id = $1
ORDER BY a.alias;

-- name: ListHouseholdTags :many
SELECT t.*
FROM food_tags t
JOIN foods f ON f.id = t.food_id
WHERE f.household_id = $1
ORDER BY t.tag;

-- name: ListHouseholdSteps :many
SELECT s.*
FROM food_steps s
JOIN foods f ON f.id = s.food_id
WHERE f.household_id = $1
ORDER BY s.food_id, s.step_number;

-- name: ListFoodSteps :many
SELECT * FROM food_steps WHERE food_id = $1 ORDER BY step_number;

-- name: InsertFood :one
INSERT INTO foods (
    id, household_id, name, description, prep_time_min, cook_time_min, servings,
    yield_amount, default_unit, density_g_per_ml, density_source, source_url,
    source_last_imported_at, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14
)
RETURNING *;

-- name: UpdateFood :execrows
UPDATE foods SET
    name = $3,
    description = $4,
    prep_time_min = $5,
    cook_time_min = $6,
    servings = $7,
    yield_amount = $8,
    default_unit = $9,
    density_g_per_ml = $10,
    density_source = $11,
    source_url = $12,
    source_last_imported_at = $13,
    updated_by = $14,
    updated_at = now(),
    version = version + 1
WHERE id = $1 AND household_id = $15 AND version = $2 AND deleted_at IS NULL;

-- name: SoftDeleteFood :execrows
UPDATE foods SET deleted_at = now(), version = version + 1, updated_by = $3
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteFoodComponents :exec
DELETE FROM food_components WHERE parent_food_id = $1;

-- name: InsertFoodComponent :exec
INSERT INTO food_components (id, parent_food_id, child_food_id, amount, unit, variant_text, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: DeleteFoodAliases :exec
DELETE FROM food_aliases WHERE food_id = $1;

-- name: InsertFoodAlias :exec
INSERT INTO food_aliases (food_id, alias, created_by) VALUES ($1, $2, $3);

-- name: DeleteFoodTags :exec
DELETE FROM food_tags WHERE food_id = $1;

-- name: InsertFoodTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: DeleteFoodSteps :exec
DELETE FROM food_steps WHERE food_id = $1;

-- name: InsertFoodStep :exec
INSERT INTO food_steps (food_id, step_number, instruction) VALUES ($1, $2, $3);

-- name: FoodExistsAnywhere :one
SELECT household_id FROM foods WHERE id = $1;

-- name: LatestFoodCosts :many
-- Most recent recorded grocery cost per food in the household (FR19.2).
SELECT DISTINCT ON (gi.ingredient_id)
    gi.ingredient_id, gi.cost, gi.amount, gi.unit, gi.cost_recorded_at
FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1
  AND gi.ingredient_id IS NOT NULL
  AND gi.cost IS NOT NULL
  AND gi.amount > 0
ORDER BY gi.ingredient_id, gi.cost_recorded_at DESC NULLS LAST;
