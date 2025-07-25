
-- name: CreateFood :one
INSERT INTO foods (name, unit_type, base_unit, density, is_recipe)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1;

-- name: SearchFoods :many
SELECT * FROM foods
WHERE 
    CASE 
        WHEN COALESCE(TRIM($1), '') = '' THEN TRUE
        ELSE (name ILIKE '%' || $1 || '%') OR (CAST(id AS TEXT) LIKE '%' || $1 || '%')
    END
ORDER BY name
LIMIT $2 OFFSET $3;

-- name: SearchFoodsAutocomplete :many
SELECT id, name, unit_type, base_unit, is_recipe, density FROM foods
WHERE name ILIKE $1 || '%'
ORDER BY 
    CASE WHEN name ILIKE $1 || '%' THEN 1 ELSE 2 END,
    LENGTH(name),
    name
LIMIT $2;

-- name: GetRecentFoods :many  
SELECT id, name, unit_type, base_unit, is_recipe, density FROM foods
ORDER BY updated_at DESC
LIMIT $1;
-- name: CountSearchFoods :one
SELECT COUNT(*) FROM foods
WHERE 
    CASE 
        WHEN COALESCE(TRIM($1), '') = '' THEN TRUE
        ELSE (name ILIKE '%' || $1 || '%') OR (CAST(id AS TEXT) LIKE '%' || $1 || '%')
    END;
    
-- name: SearchFoodsWithDependencies :many
WITH RECURSIVE recipe_tree AS (
    -- Base case: foods matching id or name
    SELECT 
        f.id,
        f.name,
        f.unit_type,
        f.base_unit,
        f.density,
        f.is_recipe,
        0 as depth,
        ARRAY[f.id] as path,
        CAST(NULL AS NUMERIC) as quantity,
        CAST(NULL AS TEXT) as unit
    FROM foods f
    WHERE 
        CASE 
            WHEN @search_id::int > 0 THEN f.id = @search_id
            WHEN COALESCE(TRIM(@search_name), '') <> '' THEN f.name ILIKE '%' || @search_name::text || '%'
            ELSE TRUE
        END
    
    UNION ALL
    
    -- Recursive case: get ingredients of recipes
    SELECT 
        f.id,
        f.name,
        f.unit_type,
        f.base_unit,
        f.density,
        f.is_recipe,
        rt.depth + 1,
        rt.path || f.id,
        ri.quantity,
        ri.unit
    FROM recipe_tree rt
    JOIN recipe_ingredients ri ON rt.id = ri.recipe_id
    JOIN foods f ON ri.ingredient_id = f.id
    WHERE NOT f.id = ANY(rt.path)  -- Prevent circular dependencies
    AND (@max_depth::int <= 0 OR rt.depth < @max_depth)  -- Check max depth if specified
    AND rt.depth < 15              -- Enforce max depth
)
SELECT DISTINCT ON (rt.depth, f.id)
    f.id,
    f.name,
    f.unit_type,
    f.base_unit,
    f.density,
    f.is_recipe,
    rt.depth,
    rt.quantity,
    rt.unit,
    r.instructions,
    r.url,
    r.yield_quantity
FROM recipe_tree rt
JOIN foods f ON rt.id = f.id
LEFT JOIN recipes r ON f.id = r.food_id
ORDER BY rt.depth, f.id, f.name;

-- name: UpdateFood :one
UPDATE foods
SET name = $2, unit_type = $3, base_unit = $4, density = $5, is_recipe = $6
WHERE id = $1
RETURNING *;

-- name: UpdateFoodWithRecipe :one
WITH updated_food AS (
    UPDATE foods
    SET 
        name = $2,
        unit_type = $3,
        base_unit = $4,
        density = $5,
        is_recipe = $6,
        updated_at = NOW()
    WHERE id = $1
    RETURNING *
),
deleted_ingredients AS (
    DELETE FROM recipe_ingredients
    WHERE recipe_id = $1
    RETURNING recipe_id
),
updated_recipe AS (
    UPDATE recipes
    SET 
        instructions = $7,
        url = $8,
        yield_quantity = $9,
        updated_at = NOW()
    WHERE food_id = $1
    RETURNING *
)
SELECT 
    f.*,
    COALESCE(
        json_build_object(
            'instructions', r.instructions,
            'url', r.url,
            'yield_quantity', r.yield_quantity
        ),
        NULL
    ) as recipe,
    (SELECT COUNT(*) FROM deleted_ingredients) as deleted_count
FROM updated_food f
LEFT JOIN updated_recipe r ON f.id = r.food_id;-- name: DeleteFood :exec
DELETE FROM foods WHERE id = $1;

-- name: GetRecipeWithIngredients :one
SELECT
    r.*,
    COALESCE(
        json_agg(
            json_build_object(
                'food_id', ri.ingredient_id,
                'quantity', ri.quantity,
                'unit', ri.unit
            )
        ) FILTER (WHERE ri.ingredient_id IS NOT NULL),
        '[]'
    ) as ingredients
FROM recipes r
LEFT JOIN recipe_ingredients ri ON r.food_id = ri.recipe_id
WHERE r.food_id = $1
GROUP BY r.food_id;

-- name: CreateRecipe :one
INSERT INTO recipes (food_id, instructions, url, yield_quantity)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: AddRecipeIngredient :exec
INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, unit)
VALUES ($1, $2, $3, $4);


