-- name: ListRecipes :many
SELECT * FROM recipes ORDER BY lower(name);

-- name: GetRecipe :one
SELECT * FROM recipes WHERE id = $1;

-- name: CreateRecipe :one
INSERT INTO recipes (name, description, prep_time_min, cook_time_min, servings)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateRecipe :exec
UPDATE recipes
SET name = $2, description = $3, prep_time_min = $4, cook_time_min = $5,
    servings = $6, updated_at = now()
WHERE id = $1;

-- name: DeleteRecipe :exec
DELETE FROM recipes WHERE id = $1;

-- name: ListAllTags :many
SELECT DISTINCT tag FROM recipe_tags ORDER BY tag;

-- name: ListTagsForRecipes :many
SELECT recipe_id, tag FROM recipe_tags ORDER BY recipe_id, tag;

-- name: DeleteRecipeTags :exec
DELETE FROM recipe_tags WHERE recipe_id = $1;

-- name: AddRecipeTag :exec
INSERT INTO recipe_tags (recipe_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListIngredientsForRecipes :many
SELECT * FROM recipe_ingredients ORDER BY recipe_id, sort_order;

-- name: ListRecipeIngredients :many
SELECT * FROM recipe_ingredients WHERE recipe_id = $1 ORDER BY sort_order;

-- name: DeleteRecipeIngredients :exec
DELETE FROM recipe_ingredients WHERE recipe_id = $1;

-- name: AddRecipeIngredient :exec
INSERT INTO recipe_ingredients (recipe_id, name, amount, unit, sub_recipe_id, sort_order)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: CountSubRecipeUses :one
SELECT count(*) FROM recipe_ingredients WHERE sub_recipe_id = $1;

-- name: ListRecipeSteps :many
SELECT * FROM recipe_steps WHERE recipe_id = $1 ORDER BY step_number;

-- name: DeleteRecipeSteps :exec
DELETE FROM recipe_steps WHERE recipe_id = $1;

-- name: AddRecipeStep :exec
INSERT INTO recipe_steps (recipe_id, step_number, instruction) VALUES ($1, $2, $3);
