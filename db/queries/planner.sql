-- name: ListMealsBetween :many
SELECT * FROM meal_plan
WHERE household_id = $1 AND plan_date >= $2 AND plan_date <= $3 AND deleted_at IS NULL
ORDER BY plan_date, plan_time;

-- name: ListMealDatesBetween :many
SELECT DISTINCT plan_date FROM meal_plan
WHERE household_id = $1 AND plan_date >= $2 AND plan_date <= $3 AND deleted_at IS NULL
ORDER BY plan_date;

-- name: GetMeal :one
SELECT * FROM meal_plan WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateMeal :one
INSERT INTO meal_plan AS m (household_id, plan_date, plan_time, food_id, servings, series_id, title, notes, created_by, updated_by)
SELECT sqlc.arg(household_id), sqlc.arg(plan_date), sqlc.arg(plan_time),
       sqlc.arg(food_id), sqlc.arg(servings), sqlc.arg(series_id),
       sqlc.arg(title), sqlc.arg(notes), sqlc.arg(created_by), sqlc.arg(created_by)
FROM foods f
WHERE f.id = sqlc.arg(food_id) AND f.household_id = sqlc.arg(household_id)
  AND f.deleted_at IS NULL
RETURNING m.*;

-- name: UpdateMeal :one
-- A non-series edit is accepted only at the version the editor loaded. Link
-- fields are part of the same write so a successful save is atomic (G13).
UPDATE meal_plan AS m
SET plan_date = sqlc.arg(plan_date), plan_time = sqlc.arg(plan_time),
    food_id = sqlc.arg(food_id), servings = sqlc.arg(servings),
    title = sqlc.arg(title), notes = sqlc.arg(notes),
    link_url = sqlc.arg(link_url), link_title = sqlc.arg(link_title),
    link_image_url = sqlc.arg(link_image_url), version = m.version + 1,
    updated_by = sqlc.arg(updated_by)
WHERE m.id = sqlc.arg(id) AND m.household_id = sqlc.arg(household_id)
  AND m.version = sqlc.arg(version) AND m.series_id IS NULL AND m.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM foods f
      WHERE f.id = sqlc.arg(food_id) AND f.household_id = sqlc.arg(household_id)
        AND f.deleted_at IS NULL
  )
RETURNING m.id;

-- name: UpdateAndDetachSeriesMeal :one
-- Editing one occurrence first claims the series token, then updates and
-- detaches the selected meal. A stale token leaves both rows unchanged.
WITH guarded_series AS (
    UPDATE meal_series AS s
    SET version = s.version + 1
    WHERE s.id = sqlc.arg(series_id) AND s.household_id = sqlc.arg(household_id)
      AND s.version = sqlc.arg(series_version)
      AND EXISTS (
          SELECT 1 FROM meal_plan selected
          WHERE selected.id = sqlc.arg(id)
            AND selected.household_id = sqlc.arg(household_id)
            AND selected.series_id = s.id
            AND selected.version = sqlc.arg(version)
            AND selected.deleted_at IS NULL
      )
    RETURNING s.version
)
UPDATE meal_plan AS m
SET plan_date = sqlc.arg(plan_date), plan_time = sqlc.arg(plan_time),
    food_id = sqlc.arg(food_id), servings = sqlc.arg(servings),
    title = sqlc.arg(title), notes = sqlc.arg(notes),
    link_url = sqlc.arg(link_url), link_title = sqlc.arg(link_title),
    link_image_url = sqlc.arg(link_image_url), series_id = NULL,
    version = m.version + 1, updated_by = sqlc.arg(updated_by)
FROM guarded_series
WHERE m.id = sqlc.arg(id) AND m.household_id = sqlc.arg(household_id)
  AND m.series_id = sqlc.arg(series_id) AND m.version = sqlc.arg(version)
  AND m.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM foods f
      WHERE f.id = sqlc.arg(food_id) AND f.household_id = sqlc.arg(household_id)
        AND f.deleted_at IS NULL
  )
RETURNING m.id;

-- name: DeleteMeal :exec
-- Soft delete (G1): keep the occurrence as history instead of removing it.
UPDATE meal_plan SET deleted_at = now(), updated_by = $3 WHERE id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: CreateSeries :one
INSERT INTO meal_series AS s (household_id, food_id, plan_time, servings, freq, byweekday, start_date, until_date)
SELECT sqlc.arg(household_id), sqlc.arg(food_id), sqlc.arg(plan_time),
       sqlc.arg(servings), sqlc.arg(freq), sqlc.arg(byweekday),
       sqlc.arg(start_date), sqlc.arg(until_date)
FROM foods f
WHERE f.id = sqlc.arg(food_id) AND f.household_id = sqlc.arg(household_id)
  AND f.deleted_at IS NULL
RETURNING s.*;

-- name: GetSeries :one
SELECT * FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: BumpSeriesVersion :execrows
-- Any mutation that changes attached occurrence membership invalidates editors
-- opened against the previous series aggregate.
UPDATE meal_series
SET version = version + 1
WHERE id = sqlc.arg(id) AND household_id = sqlc.arg(household_id);

-- name: ListSeriesMeals :many
SELECT * FROM meal_plan
WHERE series_id = $1 AND plan_date >= $2 AND deleted_at IS NULL
ORDER BY plan_date, plan_time;

-- name: UpdateSeriesMealsFrom :many
-- The selected meal and series tokens are validated before any occurrence is
-- changed. Target rows deliberately have no individual version predicate: the
-- series token serializes overlapping future/all edits, so every still-attached
-- target is updated together and returned for transactional extra replacement.
WITH selected AS MATERIALIZED (
    SELECT m.series_id
    FROM meal_plan m
    WHERE m.id = sqlc.arg(id) AND m.household_id = sqlc.arg(household_id)
      AND m.series_id = sqlc.arg(series_id) AND m.version = sqlc.arg(version)
      AND m.deleted_at IS NULL
), guarded_series AS (
    UPDATE meal_series AS s
    SET version = s.version + 1
    FROM selected
    WHERE s.id = selected.series_id AND s.id = sqlc.arg(series_id)
      AND s.household_id = sqlc.arg(household_id)
      AND s.version = sqlc.arg(series_version)
    RETURNING s.id
), updated AS (
    UPDATE meal_plan AS m
    SET plan_time = sqlc.arg(plan_time), food_id = sqlc.arg(food_id),
        servings = sqlc.arg(servings), title = sqlc.arg(title),
        notes = sqlc.arg(notes),
        link_url = CASE WHEN m.id = sqlc.arg(id) THEN sqlc.arg(link_url) ELSE m.link_url END,
        link_title = CASE WHEN m.id = sqlc.arg(id) THEN sqlc.arg(link_title) ELSE m.link_title END,
        link_image_url = CASE WHEN m.id = sqlc.arg(id) THEN sqlc.arg(link_image_url) ELSE m.link_image_url END,
        version = m.version + 1, updated_by = sqlc.arg(updated_by)
    FROM guarded_series
    WHERE m.series_id = guarded_series.id
      AND m.household_id = sqlc.arg(household_id) AND m.deleted_at IS NULL
      AND (sqlc.arg(all_occurrences)::boolean OR m.plan_date >= sqlc.arg(from_date))
      AND EXISTS (
          SELECT 1 FROM foods f
          WHERE f.id = sqlc.arg(food_id) AND f.household_id = sqlc.arg(household_id)
            AND f.deleted_at IS NULL
      )
    RETURNING m.id
)
SELECT id FROM updated ORDER BY id;

-- name: DeleteSeriesMealsFrom :exec
-- Soft delete this-and-future occurrences of a series (G1).
UPDATE meal_plan SET deleted_at = now(), updated_by = sqlc.arg(updated_by)
WHERE series_id = sqlc.arg(series_id) AND household_id = sqlc.arg(household_id)
  AND plan_date >= sqlc.arg(plan_date) AND deleted_at IS NULL;

-- name: DeleteSeriesMeals :exec
-- Soft delete every occurrence of a series (scope=all, G1). The meal_series rule
-- row is left in place; a hard DELETE there would cascade-remove the occurrences
-- and defeat the soft-delete history.
UPDATE meal_plan SET deleted_at = now(), updated_by = $3
WHERE series_id = $1 AND household_id = $2 AND deleted_at IS NULL;

-- name: DeleteSeries :exec
DELETE FROM meal_series WHERE id = $1 AND household_id = $2;

-- name: CreateMealRecipe :execrows
-- G4: attach an additional recipe to a scheduled meal, with an optional
-- per-recipe servings override (NULL => use the meal's servings). Both parent
-- and food are household-scoped so posted UUIDs cannot cross tenant boundaries.
INSERT INTO scheduled_meal_recipes (meal_id, food_id, servings_override, sort_order, created_by, updated_by)
SELECT sqlc.arg(meal_id), sqlc.arg(food_id), sqlc.arg(servings_override),
       sqlc.arg(sort_order), sqlc.arg(created_by), sqlc.arg(created_by)
FROM meal_plan m, foods f
WHERE m.id = sqlc.arg(meal_id) AND m.household_id = sqlc.arg(household_id)
  AND m.deleted_at IS NULL AND f.id = sqlc.arg(food_id)
  AND f.household_id = sqlc.arg(household_id) AND f.deleted_at IS NULL;

-- name: ListMealRecipes :many
SELECT * FROM scheduled_meal_recipes WHERE meal_id = $1 ORDER BY sort_order, id;

-- name: ListMealRecipesForMeals :many
-- Batch-load additional recipes for a set of meals (list/grocery views).
SELECT * FROM scheduled_meal_recipes WHERE meal_id = ANY($1::uuid[]) ORDER BY meal_id, sort_order, id;

-- name: DeleteMealRecipes :exec
-- Replace-on-edit: clear a meal's additional recipes before re-inserting.
DELETE FROM scheduled_meal_recipes WHERE meal_id = $1;

-- name: UpdateMealLink :one
-- FR13/G13: preview sub-actions on a non-series meal cannot bypass locking.
UPDATE meal_plan AS m
SET link_url = sqlc.arg(link_url), link_title = sqlc.arg(link_title),
    link_image_url = sqlc.arg(link_image_url), version = m.version + 1,
    updated_by = sqlc.arg(updated_by)
WHERE m.id = sqlc.arg(id) AND m.household_id = sqlc.arg(household_id)
  AND m.version = sqlc.arg(version) AND m.series_id IS NULL AND m.deleted_at IS NULL
RETURNING m.id;

-- name: UpdateSeriesMealLink :one
-- A preview action keeps an occurrence attached, but claims the series token so
-- it cannot be overwritten by an overlapping future/all edit.
WITH guarded_series AS (
    UPDATE meal_series AS s
    SET version = s.version + 1
    WHERE s.id = sqlc.arg(series_id) AND s.household_id = sqlc.arg(household_id)
      AND s.version = sqlc.arg(series_version)
      AND EXISTS (
          SELECT 1 FROM meal_plan selected
          WHERE selected.id = sqlc.arg(id)
            AND selected.household_id = sqlc.arg(household_id)
            AND selected.series_id = s.id
            AND selected.version = sqlc.arg(version)
            AND selected.deleted_at IS NULL
      )
    RETURNING s.id
)
UPDATE meal_plan AS m
SET link_url = sqlc.arg(link_url), link_title = sqlc.arg(link_title),
    link_image_url = sqlc.arg(link_image_url), version = m.version + 1,
    updated_by = sqlc.arg(updated_by)
FROM guarded_series
WHERE m.id = sqlc.arg(id) AND m.household_id = sqlc.arg(household_id)
  AND m.series_id = guarded_series.id AND m.version = sqlc.arg(version)
  AND m.deleted_at IS NULL
RETURNING m.id;
