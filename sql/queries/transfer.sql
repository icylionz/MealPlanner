-- Ownership lookups for JSON import: a stable ID may already exist in this
-- household (merge skips it) or in another household (conflict).

-- name: MealOwner :one
SELECT household_id FROM meal_plan WHERE id = $1;

-- name: SeriesOwner :one
SELECT household_id FROM meal_series WHERE id = $1;

-- name: GroceryListOwner :one
SELECT household_id FROM grocery_lists WHERE id = $1;

-- name: GroceryItemOwner :one
SELECT l.household_id FROM grocery_items i JOIN grocery_lists l ON l.id = i.list_id WHERE i.id = $1;

-- name: ComponentExists :one
SELECT EXISTS (SELECT 1 FROM food_components WHERE id = $1);

-- name: InsertSeriesFull :exec
INSERT INTO meal_series (id, household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertPrepSessionFull :exec
INSERT INTO prep_sessions (id, household_id, name, session_date, created_at)
VALUES ($1, $2, $3, $4, $5);
