-- name: ListFoods :many
-- Every food in the household, soft-deleted ones included, so historical
-- references stay readable.
SELECT * FROM foods WHERE household_id = $1 ORDER BY lower(name), id;

-- name: ListComponents :many
SELECT c.*
FROM food_components c
JOIN foods f ON f.id = c.parent_food_id
WHERE f.household_id = $1
ORDER BY c.parent_food_id, c.sort_order, c.id;

-- name: ListAliases :many
SELECT a.food_id, a.alias
FROM food_aliases a
JOIN foods f ON f.id = a.food_id
WHERE f.household_id = $1
ORDER BY a.created_at, a.id;

-- name: ListTags :many
SELECT t.food_id, t.tag
FROM food_tags t
JOIN foods f ON f.id = t.food_id
WHERE f.household_id = $1
ORDER BY t.tag;

-- name: ListSteps :many
SELECT step_number, instruction FROM food_steps WHERE food_id = $1 ORDER BY step_number;

-- name: ListAllSteps :many
SELECT s.food_id, s.step_number, s.instruction
FROM food_steps s
JOIN foods f ON f.id = s.food_id
WHERE f.household_id = $1
ORDER BY s.food_id, s.step_number;

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1 AND household_id = $2;

-- name: CreateFood :one
INSERT INTO foods (household_id, name, description, prep_time_min, cook_time_min, servings,
                   yield_amount, default_unit, density_g_per_ml, density_source, source_url,
                   source_last_imported_at, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
RETURNING id;

-- name: UpdateFood :execrows
UPDATE foods
SET name = $4, description = $5, prep_time_min = $6, cook_time_min = $7, servings = $8,
    yield_amount = $9, default_unit = $10, density_g_per_ml = $11, density_source = $12,
    source_url = $13, source_last_imported_at = $14, updated_by = $15,
    updated_at = now(), version = version + 1
WHERE id = $1 AND household_id = $2 AND version = $3 AND deleted_at IS NULL;

-- name: SoftDeleteFood :execrows
UPDATE foods SET deleted_at = now(), updated_by = $3, updated_at = now()
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteComponents :exec
DELETE FROM food_components WHERE parent_food_id = $1;

-- name: InsertComponent :exec
INSERT INTO food_components (parent_food_id, child_food_id, amount, unit, variant_text, sort_order)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpsertComponent :exec
INSERT INTO food_components (id, parent_food_id, child_food_id, amount, unit, variant_text, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE
SET child_food_id = EXCLUDED.child_food_id, amount = EXCLUDED.amount, unit = EXCLUDED.unit,
    variant_text = EXCLUDED.variant_text, sort_order = EXCLUDED.sort_order
WHERE food_components.parent_food_id = EXCLUDED.parent_food_id;

-- name: DeleteComponentsExcept :exec
DELETE FROM food_components WHERE parent_food_id = $1 AND NOT (id = ANY(sqlc.arg(keep)::uuid[]));

-- name: DeleteSteps :exec
DELETE FROM food_steps WHERE food_id = $1;

-- name: InsertStep :exec
INSERT INTO food_steps (food_id, step_number, instruction) VALUES ($1, $2, $3);

-- name: DeleteTags :exec
DELETE FROM food_tags WHERE food_id = $1;

-- name: InsertTag :exec
INSERT INTO food_tags (food_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: DeleteAliases :exec
DELETE FROM food_aliases WHERE food_id = $1;

-- name: InsertAlias :exec
INSERT INTO food_aliases (food_id, alias, created_by) VALUES ($1, $2, $3);

-- name: LatestCosts :many
-- The most recent recorded cost per food; earlier costs stay on their items.
SELECT DISTINCT ON (i.ingredient_id) i.ingredient_id, i.cost, i.amount, i.unit
FROM grocery_items i
JOIN grocery_lists l ON l.id = i.list_id
WHERE l.household_id = $1 AND i.cost IS NOT NULL AND i.ingredient_id IS NOT NULL
ORDER BY i.ingredient_id, i.cost_recorded_at DESC NULLS LAST;

-- name: UpsertFood :execrows
INSERT INTO foods (id, household_id, name, description, prep_time_min, cook_time_min, servings,
                   yield_amount, default_unit, density_g_per_ml, density_source, source_url,
                   source_last_imported_at, created_by, updated_by, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $15)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description,
    prep_time_min = EXCLUDED.prep_time_min, cook_time_min = EXCLUDED.cook_time_min,
    servings = EXCLUDED.servings, yield_amount = EXCLUDED.yield_amount,
    default_unit = EXCLUDED.default_unit, density_g_per_ml = EXCLUDED.density_g_per_ml,
    density_source = EXCLUDED.density_source, source_url = EXCLUDED.source_url,
    source_last_imported_at = EXCLUDED.source_last_imported_at, updated_by = EXCLUDED.updated_by,
    deleted_at = EXCLUDED.deleted_at, updated_at = now(), version = foods.version + 1
WHERE foods.household_id = EXCLUDED.household_id;
