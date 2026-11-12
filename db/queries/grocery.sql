-- name: ListGroceryLists :many
SELECT l.id, l.name, l.created_at,
       count(i.id) AS total,
       count(i.id) FILTER (WHERE i.checked) AS checked,
       coalesce(sum(i.cost), 0)::double precision AS cost
FROM grocery_lists l
LEFT JOIN grocery_items i ON i.list_id = l.id AND i.deleted_at IS NULL
WHERE l.household_id = $1 AND l.deleted_at IS NULL
GROUP BY l.id
ORDER BY l.created_at DESC, l.id;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateGroceryList :one
INSERT INTO grocery_lists (household_id, name, created_by, updated_by)
VALUES ($1, $2, $3, $3)
RETURNING id;

-- name: RenameGroceryList :exec
UPDATE grocery_lists SET name = $3, updated_by = $4
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteGroceryList :exec
UPDATE grocery_lists SET deleted_at = now(), updated_by = $3
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListGroceryItems :many
SELECT i.*, (SELECT count(*) FROM grocery_item_sources s WHERE s.grocery_item_id = i.id) AS source_count
FROM grocery_items i
WHERE i.list_id = $1 AND i.deleted_at IS NULL
ORDER BY i.sort_order, i.id;

-- name: GetGroceryItem :one
SELECT i.*, l.name AS list_name
FROM grocery_items i
JOIN grocery_lists l ON l.id = i.list_id
WHERE i.id = $1 AND l.household_id = $2 AND i.deleted_at IS NULL AND l.deleted_at IS NULL;

-- name: CreateGroceryItem :one
INSERT INTO grocery_items (list_id, ingredient_id, name, amount, unit, variant_text, note,
                           sort_order, source_type, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7,
        (SELECT coalesce(max(sort_order), 0) + 1 FROM grocery_items WHERE list_id = $1),
        $8, $9, $9)
RETURNING id;

-- name: AddGroceryItemAmount :exec
UPDATE grocery_items SET amount = amount + $2, updated_by = $3 WHERE id = $1;

-- name: UpdateGroceryItem :exec
UPDATE grocery_items
SET amount = $2, unit = $3, note = $4, cost = $5, cost_recorded_at = $6, updated_by = $7
WHERE id = $1;

-- name: SetGroceryItemChecked :exec
UPDATE grocery_items SET checked = $2, updated_by = $3 WHERE id = $1;

-- name: SoftDeleteGroceryItem :exec
UPDATE grocery_items SET deleted_at = now(), updated_by = $2 WHERE id = $1;

-- name: InsertGroceryItemSource :exec
INSERT INTO grocery_item_sources (grocery_item_id, scheduled_meal_id, recipe_id,
                                  recipe_ingredient_line_id, quantity_contributed,
                                  unit_contributed, variant_text, line_recipe_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListGroceryItemSources :many
-- Joins ignore soft deletes on purpose: provenance stays readable.
SELECT s.*, m.plan_date, m.plan_time, m.title AS meal_title, mf.name AS meal_food_name,
       rf.name AS recipe_name
FROM grocery_item_sources s
LEFT JOIN meal_plan m ON m.id = s.scheduled_meal_id
LEFT JOIN foods mf ON mf.id = m.food_id
LEFT JOIN foods rf ON rf.id = s.recipe_id
WHERE s.grocery_item_id = $1
ORDER BY m.plan_date NULLS LAST, m.plan_time, s.id;

-- name: ListAllGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 ORDER BY created_at, id;

-- name: ListAllGroceryItems :many
SELECT i.*
FROM grocery_items i
JOIN grocery_lists l ON l.id = i.list_id
WHERE l.household_id = $1
ORDER BY i.list_id, i.sort_order, i.id;

-- name: ListAllGroceryItemSources :many
SELECT s.*
FROM grocery_item_sources s
JOIN grocery_items i ON i.id = s.grocery_item_id
JOIN grocery_lists l ON l.id = i.list_id
WHERE l.household_id = $1
ORDER BY s.grocery_item_id, s.id;

-- name: UpsertGroceryList :execrows
INSERT INTO grocery_lists (id, household_id, name, created_by, updated_by, deleted_at)
VALUES ($1, $2, $3, $4, $4, $5)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, updated_by = EXCLUDED.updated_by, deleted_at = EXCLUDED.deleted_at
WHERE grocery_lists.household_id = EXCLUDED.household_id;

-- name: UpsertGroceryItem :execrows
INSERT INTO grocery_items (id, list_id, ingredient_id, name, amount, unit, variant_text, note,
                           sort_order, cost, cost_recorded_at, checked, source_type, created_by,
                           updated_by, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $15)
ON CONFLICT (id) DO UPDATE
SET ingredient_id = EXCLUDED.ingredient_id, name = EXCLUDED.name, amount = EXCLUDED.amount,
    unit = EXCLUDED.unit, variant_text = EXCLUDED.variant_text, note = EXCLUDED.note,
    sort_order = EXCLUDED.sort_order, cost = EXCLUDED.cost,
    cost_recorded_at = EXCLUDED.cost_recorded_at, checked = EXCLUDED.checked,
    source_type = EXCLUDED.source_type, updated_by = EXCLUDED.updated_by,
    deleted_at = EXCLUDED.deleted_at
WHERE grocery_items.list_id = EXCLUDED.list_id;

-- name: DeleteGroceryItemSources :exec
DELETE FROM grocery_item_sources WHERE grocery_item_id = $1;
