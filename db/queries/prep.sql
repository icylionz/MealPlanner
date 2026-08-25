-- name: ListPrepSessions :many
SELECT * FROM prep_sessions WHERE household_id = $1 ORDER BY created_at;

-- name: GetPrepSession :one
SELECT * FROM prep_sessions WHERE id = $1 AND household_id = $2;

-- name: CreatePrepSession :one
INSERT INTO prep_sessions (household_id, name, session_date) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdatePrepSession :exec
UPDATE prep_sessions SET name = $2, session_date = $3 WHERE id = $1 AND household_id = $4;

-- name: DeletePrepSession :exec
DELETE FROM prep_sessions WHERE id = $1 AND household_id = $2;

-- name: ListPrepSessionMeals :many
SELECT * FROM prep_session_meals WHERE session_id = $1 ORDER BY sort_order;

-- name: ListAllPrepSessionMeals :many
SELECT psm.* FROM prep_session_meals psm
JOIN prep_sessions ps ON ps.id = psm.session_id
WHERE ps.household_id = $1
ORDER BY psm.session_id, psm.sort_order;

-- name: AddPrepSessionMeal :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, food_id) DO NOTHING;

-- name: RemovePrepSessionMeal :exec
DELETE FROM prep_session_meals WHERE session_id = $1 AND food_id = $2;

-- name: AdjustPrepSessionServings :exec
UPDATE prep_session_meals
SET servings = greatest(1, servings + $3)
WHERE session_id = $1 AND food_id = $2;

-- name: MaxPrepSortOrder :one
SELECT coalesce(max(sort_order), -1)::int FROM prep_session_meals WHERE session_id = $1;
