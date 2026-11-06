-- name: ListGroceryLists :many
SELECT l.*,
    (SELECT count(*) FROM grocery_items i WHERE i.list_id = l.id AND i.deleted_at IS NULL)::int AS item_count,
    (SELECT count(*) FROM grocery_items i WHERE i.list_id = l.id AND i.deleted_at IS NULL AND i.checked)::int AS checked_count
FROM grocery_lists l
WHERE l.household_id = $1 AND l.deleted_at IS NULL
ORDER BY l.created_at DESC;

-- name: ListAllGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 AND deleted_at IS NULL ORDER BY created_at;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: InsertGroceryList :one
INSERT INTO grocery_lists (id, household_id, name, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
RETURNING *;

-- name: RenameGroceryList :execrows
UPDATE grocery_lists SET name = $3, updated_by = $4
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteGroceryList :execrows
UPDATE grocery_lists SET deleted_at = now(), updated_by = $3
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListGroceryItems :many
SELECT * FROM grocery_items WHERE list_id = $1 AND deleted_at IS NULL ORDER BY checked, sort_order, lower(name);

-- name: GetGroceryItem :one
SELECT i.* FROM grocery_items i
JOIN grocery_lists l ON l.id = i.list_id
WHERE i.id = $1 AND l.household_id = $2 AND i.deleted_at IS NULL AND l.deleted_at IS NULL;

-- name: InsertGroceryItem :one
INSERT INTO grocery_items (
    id, list_id, ingredient_id, name, amount, unit, variant_text, note, sort_order, checked, source_type,
    cost, cost_recorded_at, created_by, updated_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
RETURNING *;

-- name: NextGrocerySort :one
SELECT coalesce(max(sort_order), 0)::int + 1 FROM grocery_items WHERE list_id = $1;

-- name: UpdateGroceryItemAmount :exec
UPDATE grocery_items SET amount = $2, unit = $3, note = $4, updated_by = $5 WHERE id = $1;

-- name: EditGroceryItem :exec
UPDATE grocery_items SET name = $2, amount = $3, unit = $4, variant_text = $5, note = $6, updated_by = $7
WHERE id = $1;

-- name: SetGroceryItemChecked :exec
UPDATE grocery_items SET checked = $2, updated_by = $3 WHERE id = $1;

-- name: SetGroceryItemCost :exec
UPDATE grocery_items SET cost = $2, cost_recorded_at = $3, updated_by = $4 WHERE id = $1;

-- name: SoftDeleteGroceryItem :exec
UPDATE grocery_items SET deleted_at = now(), updated_by = $2 WHERE id = $1;

-- name: InsertGroceryItemSource :exec
INSERT INTO grocery_item_sources (
    id, grocery_item_id, scheduled_meal_id, recipe_id, recipe_ingredient_line_id,
    quantity_contributed, unit_contributed, variant_text, line_recipe_name
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListGroceryItemSources :many
SELECT * FROM grocery_item_sources WHERE grocery_item_id = $1 ORDER BY id;

-- name: ListGrocerySourcesForList :many
SELECT s.* FROM grocery_item_sources s
JOIN grocery_items i ON i.id = s.grocery_item_id
WHERE i.list_id = $1;

-- name: ListAllGroceryItems :many
SELECT i.* FROM grocery_items i
JOIN grocery_lists l ON l.id = i.list_id
WHERE l.household_id = $1 AND l.deleted_at IS NULL AND i.deleted_at IS NULL
ORDER BY i.list_id, i.sort_order;

-- name: ListAllGrocerySources :many
SELECT s.* FROM grocery_item_sources s
JOIN grocery_items i ON i.id = s.grocery_item_id
JOIN grocery_lists l ON l.id = i.list_id
WHERE l.household_id = $1 AND l.deleted_at IS NULL AND i.deleted_at IS NULL;
