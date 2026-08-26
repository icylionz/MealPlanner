-- name: ListGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 AND deleted_at IS NULL ORDER BY created_at;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateGroceryList :one
INSERT INTO grocery_lists (household_id, name) VALUES ($1, $2) RETURNING *;

-- name: RenameGroceryList :exec
UPDATE grocery_lists SET name = $2 WHERE id = $1 AND household_id = $3 AND deleted_at IS NULL;

-- name: DeleteGroceryList :exec
-- Soft delete (G1). Items keep their own deleted_at untouched; the list filter
-- hides them, and un-deleting a list would restore its still-live items.
UPDATE grocery_lists SET deleted_at = now() WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListGroceryItems :many
SELECT * FROM grocery_items WHERE list_id = $1 AND deleted_at IS NULL ORDER BY sort_order, id;

-- name: ListAllGroceryItems :many
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1 AND gl.deleted_at IS NULL AND gi.deleted_at IS NULL
ORDER BY gi.list_id, gi.sort_order, gi.id;

-- name: GetGroceryItem :one
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gi.id = $1 AND gl.household_id = $2 AND gi.deleted_at IS NULL;

-- name: CreateGroceryItem :one
INSERT INTO grocery_items (list_id, name, amount, unit, checked, note, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateGroceryItemAmount :exec
UPDATE grocery_items SET amount = $2, unit = $3
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $4);

-- name: SetGroceryItemNote :exec
UPDATE grocery_items SET note = $2
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: AddGroceryItemAmount :exec
UPDATE grocery_items SET amount = amount + $2
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: ToggleGroceryItem :exec
UPDATE grocery_items SET checked = NOT checked
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteGroceryItem :exec
-- Soft delete (G1).
UPDATE grocery_items SET deleted_at = now()
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteCheckedGroceryItems :exec
-- Soft delete (G1).
UPDATE grocery_items SET deleted_at = now() WHERE list_id = $1 AND checked AND deleted_at IS NULL;

-- name: MaxGrocerySortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM grocery_items WHERE list_id = $1 AND deleted_at IS NULL;
