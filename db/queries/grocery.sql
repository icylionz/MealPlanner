-- name: ListGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 ORDER BY created_at;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1 AND household_id = $2;

-- name: CreateGroceryList :one
INSERT INTO grocery_lists (household_id, name) VALUES ($1, $2) RETURNING *;

-- name: RenameGroceryList :exec
UPDATE grocery_lists SET name = $2 WHERE id = $1 AND household_id = $3;

-- name: DeleteGroceryList :exec
DELETE FROM grocery_lists WHERE id = $1 AND household_id = $2;

-- name: ListGroceryItems :many
SELECT * FROM grocery_items WHERE list_id = $1 ORDER BY sort_order, id;

-- name: ListAllGroceryItems :many
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gl.household_id = $1
ORDER BY gi.list_id, gi.sort_order, gi.id;

-- name: GetGroceryItem :one
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gi.id = $1 AND gl.household_id = $2;

-- name: CreateGroceryItem :one
INSERT INTO grocery_items (list_id, name, amount, unit, checked, note, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateGroceryItemAmount :exec
UPDATE grocery_items SET amount = $2, unit = $3
WHERE grocery_items.id = $1 AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $4);

-- name: SetGroceryItemNote :exec
UPDATE grocery_items SET note = $2
WHERE grocery_items.id = $1 AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: AddGroceryItemAmount :exec
UPDATE grocery_items SET amount = amount + $2
WHERE grocery_items.id = $1 AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: ToggleGroceryItem :exec
UPDATE grocery_items SET checked = NOT checked
WHERE grocery_items.id = $1 AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteGroceryItem :exec
DELETE FROM grocery_items
WHERE grocery_items.id = $1 AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteCheckedGroceryItems :exec
DELETE FROM grocery_items WHERE list_id = $1 AND checked;

-- name: MaxGrocerySortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM grocery_items WHERE list_id = $1;
