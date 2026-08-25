DROP INDEX IF EXISTS meal_plan_series_idx;
ALTER TABLE meal_plan DROP COLUMN IF EXISTS series_id;
DROP TABLE IF EXISTS meal_series;
