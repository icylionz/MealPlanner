-- name: ListGroceryLists :many
SELECT * FROM grocery_lists WHERE household_id = $1 AND deleted_at IS NULL ORDER BY created_at;

-- name: GetGroceryList :one
SELECT * FROM grocery_lists WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: LockGroceryList :one
SELECT * FROM grocery_lists
WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: CreateGroceryList :one
INSERT INTO grocery_lists (household_id, name, created_by, updated_by) VALUES ($1, $2, $3, $3) RETURNING *;

-- name: RenameGroceryList :exec
UPDATE grocery_lists SET name = $2, updated_by = $4 WHERE id = $1 AND household_id = $3 AND deleted_at IS NULL;

-- name: DeleteGroceryList :exec
-- Soft delete (G1). Items keep their own deleted_at untouched; the list filter
-- hides them, and un-deleting a list would restore its still-live items.
UPDATE grocery_lists SET deleted_at = now(), updated_by = $3 WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: ListGroceryItems :many
SELECT * FROM grocery_items WHERE list_id = $1 AND deleted_at IS NULL ORDER BY sort_order, id;

-- name: ListAllGroceryItems :many
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
LEFT JOIN foods ingredient ON ingredient.id = gi.ingredient_id
    AND ingredient.household_id = gl.household_id
WHERE gl.household_id = $1 AND gl.deleted_at IS NULL AND gi.deleted_at IS NULL
  AND (gi.ingredient_id IS NULL OR ingredient.id IS NOT NULL)
ORDER BY gi.list_id, gi.sort_order, gi.id;

-- name: GetGroceryItem :one
SELECT gi.* FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
LEFT JOIN foods ingredient ON ingredient.id = gi.ingredient_id
    AND ingredient.household_id = gl.household_id
WHERE gi.id = $1 AND gl.household_id = $2
  AND gl.deleted_at IS NULL AND gi.deleted_at IS NULL
  AND (gi.ingredient_id IS NULL OR ingredient.id IS NOT NULL);

-- name: CreateGroceryItem :one
INSERT INTO grocery_items (list_id, name, amount, unit, checked, note, sort_order,
    ingredient_id, source_type, variant_text, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING *;

-- name: CreateGroceryItemSource :one
-- Every referenced source must belong to the grocery item's household. The
-- INSERT affects no rows when any supplied reference crosses that boundary.
INSERT INTO grocery_item_sources (grocery_item_id, scheduled_meal_id, recipe_id,
    recipe_ingredient_line_id, quantity_contributed, unit_contributed,
    variant_text, line_recipe_name)
SELECT $1, $2, $3, $4, $5, $6, $7, $8
FROM grocery_items gi
JOIN grocery_lists gl ON gl.id = gi.list_id
WHERE gi.id = $1 AND gi.deleted_at IS NULL
  AND gl.household_id = $9 AND gl.deleted_at IS NULL
  AND ($2::uuid IS NULL OR EXISTS (
      SELECT 1
      FROM meal_plan mp
      JOIN foods meal_food ON meal_food.id = mp.food_id
      WHERE mp.id = $2 AND mp.household_id = gl.household_id
        AND meal_food.household_id = gl.household_id
  ))
  AND ($3::uuid IS NULL OR EXISTS (
      SELECT 1 FROM foods recipe
      WHERE recipe.id = $3 AND recipe.household_id = gl.household_id
  ))
  AND ($4::uuid IS NULL OR EXISTS (
      SELECT 1
      FROM food_components line
      JOIN foods parent_food ON parent_food.id = line.parent_food_id
      JOIN foods child_food ON child_food.id = line.child_food_id
      WHERE line.id = $4
        AND parent_food.household_id = gl.household_id
        AND child_food.household_id = gl.household_id
  ))
RETURNING id;

-- name: GroceryIngredientBelongsToHousehold :one
SELECT EXISTS (
    SELECT 1 FROM foods WHERE id = $1 AND household_id = $2
);

-- name: ListGroceryItemSources :many
-- No deleted_at filters are applied to meals or foods: soft-deleted source
-- records remain readable as grocery history (FR12.6).
SELECT
    gis.id,
    gis.scheduled_meal_id,
    gis.recipe_id,
    gis.recipe_ingredient_line_id,
    gis.quantity_contributed,
    gis.unit_contributed,
    COALESCE(NULLIF(mp.title, ''), meal_food.name, '')::text AS meal_name,
    COALESCE(to_char(mp.plan_date, 'YYYY-MM-DD'), '')::text AS meal_date,
    COALESCE(mp.plan_time, '')::text AS meal_time,
    COALESCE(recipe.name, '')::text AS recipe_name,
    line_recipe.id AS line_recipe_id,
    COALESCE(NULLIF(gis.line_recipe_name, ''), line_recipe.name, '')::text AS line_recipe_name,
    gis.variant_text
FROM grocery_item_sources gis
JOIN grocery_items gi ON gi.id = gis.grocery_item_id
JOIN grocery_lists gl ON gl.id = gi.list_id
LEFT JOIN meal_plan mp ON mp.id = gis.scheduled_meal_id
    AND mp.household_id = gl.household_id
LEFT JOIN foods meal_food ON meal_food.id = mp.food_id
    AND meal_food.household_id = gl.household_id
LEFT JOIN foods recipe ON recipe.id = gis.recipe_id
    AND recipe.household_id = gl.household_id
LEFT JOIN food_components line ON line.id = gis.recipe_ingredient_line_id
LEFT JOIN foods line_recipe ON line_recipe.id = line.parent_food_id
    AND line_recipe.household_id = gl.household_id
LEFT JOIN foods line_ingredient ON line_ingredient.id = line.child_food_id
    AND line_ingredient.household_id = gl.household_id
WHERE gis.grocery_item_id = $1 AND gl.household_id = $2
  AND gl.deleted_at IS NULL AND gi.deleted_at IS NULL
  AND (gis.scheduled_meal_id IS NULL
       OR (mp.id IS NOT NULL AND meal_food.id IS NOT NULL))
  AND (gis.recipe_id IS NULL OR recipe.id IS NOT NULL)
  AND (gis.recipe_ingredient_line_id IS NULL
       OR (line_recipe.id IS NOT NULL AND line_ingredient.id IS NOT NULL))
ORDER BY mp.plan_date NULLS LAST, mp.plan_time NULLS LAST, gis.id;

-- name: UpdateGroceryItemAmount :exec
UPDATE grocery_items SET amount = $2, unit = $3, updated_by = $5
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $4);

-- name: SetGroceryItemNote :exec
UPDATE grocery_items SET note = $2, updated_by = $4
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: AddGroceryItemAmount :exec
UPDATE grocery_items SET amount = amount + $2, updated_by = $4
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $3);

-- name: ToggleGroceryItem :exec
UPDATE grocery_items SET checked = NOT checked, updated_by = $3
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteGroceryItem :exec
-- Soft delete (G1).
UPDATE grocery_items SET deleted_at = now(), updated_by = $3
WHERE grocery_items.id = $1 AND grocery_items.deleted_at IS NULL AND grocery_items.list_id IN (SELECT gl.id FROM grocery_lists gl WHERE gl.household_id = $2);

-- name: DeleteCheckedGroceryItems :exec
-- Soft delete (G1).
UPDATE grocery_items SET deleted_at = now(), updated_by = $2 WHERE list_id = $1 AND checked AND deleted_at IS NULL;

-- name: MaxGrocerySortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM grocery_items WHERE list_id = $1 AND deleted_at IS NULL;
