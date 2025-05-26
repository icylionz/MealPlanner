-- name: CreateSchedule :one
WITH inserted_schedule AS (
  INSERT INTO schedules (food_id, scheduled_at, servings)
  VALUES ($1, $2, $3)
  RETURNING *
)
SELECT s.*, f.name as food_name
FROM inserted_schedule s
JOIN foods f ON f.id = s.food_id;

-- name: GetSchedulesInRange :many
SELECT s.*, f.name as food_name
FROM schedules s
JOIN foods f ON s.food_id = f.id
WHERE scheduled_at BETWEEN $1 AND $2
ORDER BY scheduled_at;

-- name: GetScheduleById :one
SELECT s.*, f.name as food_name
FROM schedules s
JOIN foods f ON s.food_id = f.id
WHERE s.id = $1;

-- name: UpdateSchedule :one
WITH updated_schedule AS (
  UPDATE schedules 
  SET food_id = $2, servings = $3, scheduled_at = $4, updated_at = NOW()
  WHERE schedules.id = $1
  RETURNING *
)
SELECT s.id, s.food_id, s.servings, s.scheduled_at, s.created_at, s.updated_at, f.name as food_name
FROM updated_schedule s
JOIN foods f ON f.id = s.food_id;

-- name: DeleteScheduleByIds :exec
DELETE FROM schedules
WHERE id = ANY($1::int[])
RETURNING id;

-- name: DeleteScheduleByDateRange :exec
DELETE FROM schedules
WHERE scheduled_at >= $1 AND scheduled_at <= $2
RETURNING id;