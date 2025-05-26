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

-- name: DeleteScheduleByIds :exec
DELETE FROM schedules
WHERE id = ANY($1::int[])
RETURNING id;

-- name: DeleteScheduleByDateRange :exec
DELETE FROM schedules
WHERE scheduled_at >= $1 AND scheduled_at <= $2
RETURNING id;