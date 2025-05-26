-- Shopping List CRUD Operations
-- name: CreateShoppingList :one
INSERT INTO shopping_lists (name, notes)
VALUES ($1, $2)
RETURNING *;

-- name: GetShoppingLists :many
SELECT * FROM shopping_lists
ORDER BY updated_at DESC;

-- name: GetShoppingListById :one
SELECT * FROM shopping_lists 
WHERE id = $1;

-- name: UpdateShoppingList :one
UPDATE shopping_lists 
SET name = $2, notes = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteShoppingList :exec
DELETE FROM shopping_lists WHERE id = $1;

-- Shopping List Item Operations
-- name: CreateShoppingListItem :one
INSERT INTO shopping_list_items (
    shopping_list_id, food_id, food_name, quantity, unit, unit_type, notes
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetShoppingListItems :many
SELECT * FROM shopping_list_items
WHERE shopping_list_id = $1
ORDER BY food_name;

-- name: GetShoppingListItemsWithSources :many
SELECT 
    sli.*,
    slis.shopping_list_source_id as source_id,
    slis.contributed_quantity
FROM shopping_list_items sli
LEFT JOIN shopping_list_item_sources slis ON sli.id = slis.shopping_list_item_id
WHERE sli.shopping_list_id = $1
ORDER BY sli.food_name, slis.shopping_list_source_id;

-- name: FindCompatibleShoppingListItem :one
SELECT * FROM shopping_list_items
WHERE shopping_list_id = $1 
  AND food_id = $2 
  AND unit = $3
LIMIT 1;

-- name: UpdateShoppingListItemQuantity :exec
UPDATE shopping_list_items 
SET quantity = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateShoppingListItemNotes :exec
UPDATE shopping_list_items 
SET notes = $2, updated_at = NOW()
WHERE id = $1;

-- name: MarkShoppingListItemPurchased :exec
UPDATE shopping_list_items 
SET purchased = $2, actual_quantity = $3, actual_price = $4, updated_at = NOW()
WHERE id = $1;

-- name: DeleteShoppingListItem :exec
DELETE FROM shopping_list_items WHERE id = $1;

-- name: DeleteOrphanedShoppingListItems :exec
DELETE FROM shopping_list_items 
WHERE id NOT IN (
    SELECT DISTINCT shopping_list_item_id 
    FROM shopping_list_item_sources
);

-- Shopping List Source Operations
-- name: CreateShoppingListSource :one
INSERT INTO shopping_list_sources (
    shopping_list_id, source_type, source_id, source_name, servings
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetShoppingListSources :many
SELECT * FROM shopping_list_sources
WHERE shopping_list_id = $1
ORDER BY added_at DESC;

-- name: DeleteShoppingListSource :exec
DELETE FROM shopping_list_sources WHERE id = $1;

-- name: DeleteShoppingListSourcesByType :exec
DELETE FROM shopping_list_sources 
WHERE shopping_list_id = $1 AND source_type = $2;

-- Shopping List Item Source Linking
-- name: CreateShoppingListItemSource :exec
INSERT INTO shopping_list_item_sources (
    shopping_list_item_id, shopping_list_source_id, contributed_quantity
)
VALUES ($1, $2, $3);

-- name: GetShoppingListItemSources :many
SELECT * FROM shopping_list_item_sources
WHERE shopping_list_item_id = $1;

-- name: DeleteShoppingListItemSources :exec
DELETE FROM shopping_list_item_sources 
WHERE shopping_list_item_id = $1;

-- name: DeleteShoppingListItemSourcesBySource :exec
DELETE FROM shopping_list_item_sources 
WHERE shopping_list_source_id = $1;

-- Advanced Queries for Item Management
-- name: GetShoppingListWithItemCounts :many
SELECT 
    sl.*,
    COUNT(sli.id) as item_count,
    COUNT(CASE WHEN sli.purchased THEN 1 END) as purchased_count,
    COUNT(DISTINCT sls.id) as source_count
FROM shopping_lists sl
LEFT JOIN shopping_list_items sli ON sl.id = sli.shopping_list_id
LEFT JOIN shopping_list_sources sls ON sl.id = sls.shopping_list_id
GROUP BY sl.id, sl.name, sl.notes, sl.created_at, sl.updated_at
ORDER BY sl.updated_at DESC;

-- name: GetItemsBySource :many
SELECT 
    sli.*,
    slis.contributed_quantity,
    sls.source_name,
    sls.source_type
FROM shopping_list_items sli
JOIN shopping_list_item_sources slis ON sli.id = slis.shopping_list_item_id
JOIN shopping_list_sources sls ON slis.shopping_list_source_id = sls.id
WHERE sls.shopping_list_id = $1 AND sls.id = $2
ORDER BY sli.food_name;

-- Complex ingredient extraction for recipes
-- name: ExtractRecipeIngredients :many
WITH RECURSIVE recipe_ingredients AS (
    -- Base case: direct ingredients of the recipe
    SELECT 
        ri.ingredient_id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        ri.quantity,
        ri.unit,
        f.is_recipe,
        1 as depth,
        ARRAY[f.id] as path
    FROM recipe_ingredients ri
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE ri.recipe_id = $1
    
    UNION ALL
    
    -- Recursive case: ingredients of recipe ingredients
    SELECT 
        ri.ingredient_id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        (rec_ing.quantity * ri.quantity) as quantity,
        ri.unit,
        f.is_recipe,
        rec_ing.depth + 1,
        rec_ing.path || f.id
    FROM recipe_ingredients rec_ing
    JOIN recipe_ingredients ri ON rec_ing.food_id = ri.recipe_id
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE rec_ing.is_recipe = true
      AND NOT f.id = ANY(rec_ing.path)  -- Prevent cycles
      AND rec_ing.depth < 15            -- Max depth limit
)
SELECT 
    food_id,
    food_name,
    unit_type,
    base_unit,
    SUM(quantity) as total_quantity,
    unit,
    depth
FROM recipe_ingredients
WHERE is_recipe = false  -- Only return base ingredients
GROUP BY food_id, food_name, unit_type, base_unit, unit, depth
ORDER BY food_name;

-- name: GetScheduleIngredients :many
WITH RECURSIVE schedule_ingredients AS (
    -- Base case: if scheduled item is a basic food
    SELECT 
        s.id as schedule_id,
        f.id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        s.servings as quantity,
        f.base_unit as unit,
        false as is_recipe,
        0 as depth,
        ARRAY[f.id] as path
    FROM schedules s
    JOIN foods f ON s.food_id = f.id
    WHERE s.id = ANY($1::int[])
      AND f.is_recipe = false
    
    UNION ALL
    
    -- Recursive case: if scheduled item is a recipe
    SELECT 
        s.id as schedule_id,
        ri.ingredient_id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        (s.servings * ri.quantity / r.yield_quantity) as quantity,
        ri.unit,
        f.is_recipe,
        1 as depth,
        ARRAY[s.food_id, f.id] as path
    FROM schedules s
    JOIN recipes r ON s.food_id = r.food_id
    JOIN recipe_ingredients ri ON r.food_id = ri.recipe_id
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE s.id = ANY($1::int[])
    
    UNION ALL
    
    -- Continue recursion for nested recipes
    SELECT 
        si.schedule_id,
        ri.ingredient_id as food_id,
        f.name as food_name,
        f.unit_type,
        f.base_unit,
        (si.quantity * ri.quantity / r.yield_quantity) as quantity,
        ri.unit,
        f.is_recipe,
        si.depth + 1,
        si.path || f.id
    FROM schedule_ingredients si
    JOIN recipes r ON si.food_id = r.food_id
    JOIN recipe_ingredients ri ON r.food_id = ri.recipe_id
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE si.is_recipe = true
      AND NOT f.id = ANY(si.path)
      AND si.depth < 15
)
SELECT 
    schedule_id,
    food_id,
    food_name,
    unit_type,
    base_unit,
    SUM(quantity) as total_quantity,
    unit
FROM schedule_ingredients
WHERE is_recipe = false
GROUP BY schedule_id, food_id, food_name, unit_type, base_unit, unit
ORDER BY food_name;

-- Statistics and reporting queries
-- name: GetShoppingListStats :one
SELECT 
    COUNT(*) as total_items,
    COUNT(CASE WHEN purchased THEN 1 END) as purchased_items,
    SUM(CASE WHEN actual_price IS NOT NULL THEN actual_price ELSE 0 END) as total_spent,
    COUNT(DISTINCT 
        CASE WHEN unit_type = 'mass' THEN food_id END
    ) as mass_items,
    COUNT(DISTINCT 
        CASE WHEN unit_type = 'volume' THEN food_id END
    ) as volume_items,
    COUNT(DISTINCT 
        CASE WHEN unit_type = 'count' THEN food_id END
    ) as count_items
FROM shopping_list_items
WHERE shopping_list_id = $1;

-- name: GetShoppingListProgress :many
SELECT 
    sls.source_name,
    sls.source_type,
    COUNT(sli.id) as total_items,
    COUNT(CASE WHEN sli.purchased THEN 1 END) as purchased_items,
    ROUND(
        (COUNT(CASE WHEN sli.purchased THEN 1 END) * 100.0 / NULLIF(COUNT(sli.id), 0)), 
        1
    ) as completion_percentage
FROM shopping_list_sources sls
LEFT JOIN shopping_list_item_sources slis ON sls.id = slis.shopping_list_source_id
LEFT JOIN shopping_list_items sli ON slis.shopping_list_item_id = sli.id
WHERE sls.shopping_list_id = $1
GROUP BY sls.id, sls.source_name, sls.source_type
ORDER BY sls.added_at;

-- Cleanup and maintenance queries
-- name: DeleteEmptyShoppingLists :exec
DELETE FROM shopping_lists 
WHERE id NOT IN (
    SELECT DISTINCT shopping_list_id 
    FROM shopping_list_items
)
AND created_at < NOW() - INTERVAL '30 days';

-- name: ArchiveOldShoppingLists :exec
-- This would be for a future archiving feature
UPDATE shopping_lists 
SET notes = COALESCE(notes || ' ', '') || '[ARCHIVED]'
WHERE updated_at < $1 
  AND notes NOT LIKE '%[ARCHIVED]%';

-- Search and filtering
-- name: SearchShoppingLists :many
SELECT sl.*, 
       COUNT(sli.id) as item_count
FROM shopping_lists sl
LEFT JOIN shopping_list_items sli ON sl.id = sli.shopping_list_id
WHERE ($1 = '' OR sl.name ILIKE '%' || $1 || '%')
   OR ($1 = '' OR sl.notes ILIKE '%' || $1 || '%')
GROUP BY sl.id, sl.name, sl.notes, sl.created_at, sl.updated_at
ORDER BY sl.updated_at DESC;

-- name: GetShoppingListsByDateRange :many
SELECT * FROM shopping_lists
WHERE created_at BETWEEN $1 AND $2
ORDER BY created_at DESC;
