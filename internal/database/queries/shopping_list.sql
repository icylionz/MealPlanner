-- name: CreateShoppingList :one
INSERT INTO shopping_lists (name, start_date, end_date)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetShoppingLists :many
SELECT * FROM shopping_lists
ORDER BY created_at DESC;

-- name: GetShoppingListById :one
SELECT * FROM shopping_lists WHERE id = $1;

-- name: DeleteShoppingList :exec
DELETE FROM shopping_lists WHERE id = $1;

-- name: CreateShoppingListItem :one
INSERT INTO shopping_list_items (shopping_list_id, food_id, quantity, unit)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetShoppingListItems :many
SELECT sli.*, f.name as food_name, f.unit_type, f.base_unit
FROM shopping_list_items sli
JOIN foods f ON sli.food_id = f.id
WHERE sli.shopping_list_id = $1
ORDER BY f.name;

-- name: DeleteShoppingListItem :exec
DELETE FROM shopping_list_items WHERE id = $1;

-- name: DeleteShoppingListItemsByListId :execresult
DELETE FROM shopping_list_items 
WHERE shopping_list_id = $1;

-- name: AddShoppingListMeal :exec
INSERT INTO shopping_list_meals (shopping_list_id, schedule_id)
VALUES ($1, $2);

-- name: GetShoppingListMeals :many
SELECT slm.*, s.scheduled_at, f.name as food_name
FROM shopping_list_meals slm
JOIN schedules s ON slm.schedule_id = s.id
JOIN foods f ON s.food_id = f.id
WHERE slm.shopping_list_id = $1
ORDER BY s.scheduled_at;

-- name: RemoveShoppingListMeal :exec
DELETE FROM shopping_list_meals WHERE shopping_list_id = $1 AND schedule_id = $2;

-- name: RecordItemPurchase :one
INSERT INTO shopping_list_purchases (shopping_list_item_id, actual_quantity, price)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetItemPurchases :many
SELECT * FROM shopping_list_purchases WHERE shopping_list_item_id = $1;

-- name: GenerateShoppingListItems :many
WITH RECURSIVE ingredients_needed AS (
    -- Base ingredients from meals directly in shopping list
    SELECT 
        slm.shopping_list_id,
        f.id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        1.0 as quantity,
        'serving' as unit,
        schedule_id,
        f.is_recipe
    FROM shopping_list_meals slm
    JOIN schedules s ON slm.schedule_id = s.id
    JOIN foods f ON s.food_id = f.id
    WHERE slm.shopping_list_id = $1
    
    UNION ALL
    
    -- Recursive extraction of ingredients from recipes
    SELECT 
        in_needed.shopping_list_id,
        f.id,
        f.name,
        f.unit_type,
        f.base_unit,
        (in_needed.quantity * ri.quantity) as quantity,
        ri.unit,
        in_needed.schedule_id,
        f.is_recipe
    FROM ingredients_needed in_needed
    JOIN recipes r ON in_needed.food_id = r.food_id
    JOIN recipe_ingredients ri ON r.food_id = ri.recipe_id
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE in_needed.is_recipe = true
)
SELECT 
    shopping_list_id,
    food_id,
    food_name,
    unit_type,
    base_unit,
    CAST(SUM(quantity) AS NUMERIC(10,2)) as total_quantity, -- Explicit NUMERIC with precision
    unit,
    array_agg(DISTINCT schedule_id) as schedule_ids
FROM ingredients_needed
WHERE is_recipe = false
GROUP BY shopping_list_id, food_id, food_name, unit_type, base_unit, unit
ORDER BY food_name;
