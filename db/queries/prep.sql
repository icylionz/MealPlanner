-- name: ListPrepSessions :many
SELECT p.*, (SELECT count(*) FROM prep_session_meals m WHERE m.session_id = p.id) AS food_count
FROM prep_sessions p
WHERE p.household_id = $1
ORDER BY p.session_date DESC, p.created_at DESC;

-- name: GetPrepSession :one
SELECT * FROM prep_sessions WHERE id = $1 AND household_id = $2;

-- name: CreatePrepSession :one
INSERT INTO prep_sessions (household_id, name, session_date) VALUES ($1, $2, $3) RETURNING id;

-- name: UpdatePrepSession :exec
UPDATE prep_sessions SET name = $3, session_date = $4 WHERE id = $1 AND household_id = $2;

-- name: DeletePrepSession :exec
DELETE FROM prep_sessions WHERE id = $1 AND household_id = $2;

-- name: ListPrepFoods :many
SELECT * FROM prep_session_meals WHERE session_id = $1 ORDER BY sort_order, food_id;

-- name: UpsertPrepFood :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
VALUES ($1, $2, $3,
        (SELECT coalesce(max(sort_order), 0) + 1 FROM prep_session_meals WHERE session_id = $1))
ON CONFLICT (session_id, food_id) DO UPDATE SET servings = EXCLUDED.servings;

-- name: DeletePrepFood :exec
DELETE FROM prep_session_meals WHERE session_id = $1 AND food_id = $2;

-- name: ListAllPrepFoods :many
SELECT m.*
FROM prep_session_meals m
JOIN prep_sessions p ON p.id = m.session_id
WHERE p.household_id = $1
ORDER BY m.session_id, m.sort_order;

-- name: UpsertPrepSession :execrows
INSERT INTO prep_sessions (id, household_id, name, session_date)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, session_date = EXCLUDED.session_date
WHERE prep_sessions.household_id = EXCLUDED.household_id;

-- name: ImportPrepFood :exec
INSERT INTO prep_session_meals (session_id, food_id, servings, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, food_id) DO UPDATE
SET servings = EXCLUDED.servings, sort_order = EXCLUDED.sort_order;
