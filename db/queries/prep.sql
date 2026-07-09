-- name: ListPrepSessions :many
SELECT * FROM prep_sessions ORDER BY created_at;

-- name: GetPrepSession :one
SELECT * FROM prep_sessions WHERE id = $1;

-- name: CreatePrepSession :one
INSERT INTO prep_sessions (name, session_date) VALUES ($1, $2) RETURNING *;

-- name: UpdatePrepSession :exec
UPDATE prep_sessions SET name = $2, session_date = $3 WHERE id = $1;

-- name: DeletePrepSession :exec
DELETE FROM prep_sessions WHERE id = $1;

-- name: ListPrepSessionMeals :many
SELECT * FROM prep_session_meals WHERE session_id = $1 ORDER BY sort_order;

-- name: ListAllPrepSessionMeals :many
SELECT * FROM prep_session_meals ORDER BY session_id, sort_order;

-- name: AddPrepSessionMeal :exec
INSERT INTO prep_session_meals (session_id, recipe_id, servings, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, recipe_id) DO NOTHING;

-- name: RemovePrepSessionMeal :exec
DELETE FROM prep_session_meals WHERE session_id = $1 AND recipe_id = $2;

-- name: AdjustPrepSessionServings :exec
UPDATE prep_session_meals
SET servings = greatest(1, servings + $3)
WHERE session_id = $1 AND recipe_id = $2;

-- name: MaxPrepSortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM prep_session_meals WHERE session_id = $1;
