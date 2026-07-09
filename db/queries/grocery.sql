-- name: ListGroceryLists :many
SELECT * FROM grocery_lists ORDER BY created_at;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1;

-- name: CreateGroceryList :one
INSERT INTO grocery_lists (name) VALUES ($1) RETURNING *;

-- name: RenameGroceryList :exec
UPDATE grocery_lists SET name = $2 WHERE id = $1;

-- name: DeleteGroceryList :exec
DELETE FROM grocery_lists WHERE id = $1;

-- name: ListGroceryItems :many
SELECT * FROM grocery_items WHERE list_id = $1 ORDER BY sort_order, id;

-- name: ListAllGroceryItems :many
SELECT * FROM grocery_items ORDER BY list_id, sort_order, id;

-- name: GetGroceryItem :one
SELECT * FROM grocery_items WHERE id = $1;

-- name: CreateGroceryItem :one
INSERT INTO grocery_items (list_id, name, amount, unit, checked, note, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateGroceryItemAmount :exec
UPDATE grocery_items SET amount = $2, unit = $3 WHERE id = $1;

-- name: AddGroceryItemAmount :exec
UPDATE grocery_items SET amount = amount + $2 WHERE id = $1;

-- name: ToggleGroceryItem :exec
UPDATE grocery_items SET checked = NOT checked WHERE id = $1;

-- name: DeleteGroceryItem :exec
DELETE FROM grocery_items WHERE id = $1;

-- name: DeleteCheckedGroceryItems :exec
DELETE FROM grocery_items WHERE list_id = $1 AND checked;

-- name: MaxGrocerySortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM grocery_items WHERE list_id = $1;
